// Command server261 is a Minecraft 26.1-snapshot-2 server with a superflat world,
// 20 TPS tick loop, player movement tracking, and view-distance chunk streaming.
//
// Usage:
//
//	go run ./cmd/server261
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/server/command"
	"github.com/Tnze/go-mc/game/dbworld"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/game/handler"
	"github.com/Tnze/go-mc/mods/chatheads"
	"github.com/Tnze/go-mc/game/mem"
	"github.com/Tnze/go-mc/game/pgstore"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	logger := log.New(os.Stdout, "[Server] ", log.LstdFlags)

	tp, tracerShutdown := initTracer(logger)
	defer tracerShutdown()

	players := game.NewPlayerManager()

	// Choose world generator: terrain (default) or superflat
	var worldGen game.ChunkGenerator
	var sections, minY int
	var spawnY float64
	var isFlat bool

	if os.Getenv("WORLD_TYPE") == "flat" {
		sfGen := gen.DefaultSuperflat()
		worldGen = sfGen
		sections = sfGen.Sections
		minY = sfGen.MinY
		spawnY = sfGen.SpawnY()
		isFlat = true
		logger.Printf("Using superflat world generator")
	} else {
		seed := int64(12345)
		if s := os.Getenv("WORLD_SEED"); s != "" {
			// Simple string→int64 hash
			for _, c := range s {
				seed = seed*31 + int64(c)
			}
		}
		tGen := gen.NewTerrainGenerator(seed)
		worldGen = tGen
		sections = tGen.Sections
		minY = tGen.MinY
		spawnY = tGen.SpawnY()
		logger.Printf("Using terrain world generator (seed=%d)", seed)
	}

	var world game.World
	var playerStore store.PlayerStore
	var pg *pgstore.PGStore
	var dbw *dbworld.World // nil when using mem.World

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		logger.Printf("Connecting to PostgreSQL...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var err error
		pg, err = pgstore.New(ctx, dbURL)
		cancel()
		if err != nil {
			logger.Fatalf("Database connection failed: %v", err)
		}
		defer pg.Close()

		ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		if err := pgstore.Migrate(ctx2, pg.Pool()); err != nil {
			cancel2()
			logger.Fatalf("Database migration failed: %v", err)
		}
		cancel2()
		logger.Printf("Database ready")

		dbw = dbworld.NewWorld(pg, worldGen, sections, minY, "overworld", logger)
		world = dbw
		playerStore = pg
	} else {
		world = mem.NewWorld(worldGen, sections, minY)
	}

	// Create Nether and End worlds (persistent if DB available, otherwise in-memory)
	var overworldSeed int64 = 12345
	if s := os.Getenv("WORLD_SEED"); s != "" {
		overworldSeed = 12345
		for _, c := range s {
			overworldSeed = overworldSeed*31 + int64(c)
		}
	}
	netherGen := gen.NewNetherGenerator(overworldSeed)
	var netherWorld game.World
	var netherDBW *dbworld.World
	if pg != nil {
		netherDBW = dbworld.NewWorld(pg, netherGen, netherGen.Sections, netherGen.MinY, "the_nether", logger)
		netherWorld = netherDBW
		logger.Printf("Nether world initialized with DB persistence (sections=%d, minY=%d)", netherGen.Sections, netherGen.MinY)
	} else {
		netherWorld = mem.NewWorld(netherGen, netherGen.Sections, netherGen.MinY)
		logger.Printf("Nether world initialized in-memory (sections=%d, minY=%d)", netherGen.Sections, netherGen.MinY)
	}

	endGen := gen.NewEndGenerator(overworldSeed)
	var endWorld game.World
	var endDBW *dbworld.World
	if pg != nil {
		endDBW = dbworld.NewWorld(pg, endGen, endGen.Sections, endGen.MinY, "the_end", logger)
		endWorld = endDBW
		logger.Printf("End world initialized with DB persistence (sections=%d, minY=%d)", endGen.Sections, endGen.MinY)
	} else {
		endWorld = mem.NewWorld(endGen, endGen.Sections, endGen.MinY)
		logger.Printf("End world initialized in-memory (sections=%d, minY=%d)", endGen.Sections, endGen.MinY)
	}

	// Build minimal registries for 26.1-snapshot-2
	regs := buildRegistries()

	survHandler := &handler.SurvivalHandler{
		Logger:             logger,
		FallDamageTypeID:   2, // minecraft:fall (3rd registered)
		AttackDamageTypeID: 3, // minecraft:player_attack (4th registered)
		MobDamageTypeID:    7, // minecraft:mob_attack (8th registered)
		VoidDamageTypeID:   4, // minecraft:out_of_world (5th registered)
		FireDamageTypeID:   5, // minecraft:on_fire (6th registered)
		DrownDamageTypeID:  6, // minecraft:drown (7th registered)
	}

	gameRules := handler.NewGameRules()

	chestMgr := handler.NewChestManager()
	chestMgr.Manager = players
	itemEntities := handler.NewItemEntityManager(players)
	xpOrbMgr := handler.NewXPOrbManager(players)
	furnaceMgr := handler.NewFurnaceManager(players)
	brewingMgr := handler.NewBrewingStandManager(players)
	timeMgr := &handler.TimeManager{Rules: gameRules}
	arrowMgr := handler.NewArrowManager(players, survHandler, world)
	mobMgr := handler.NewMobManager(players, timeMgr, world, minY, survHandler, itemEntities)
	arrowMgr.MobMgr = mobMgr
	arrowMgr.ItemEntities = itemEntities
	mobMgr.ArrowMgr = arrowMgr
	mobMgr.XPOrbMgr = xpOrbMgr
	mobMgr.Logger = logger
	if pg != nil {
		mobMgr.MobStore = pg
	}
	advancementMgr := handler.NewAdvancementManager(players)
	mobMgr.AdvMgr = advancementMgr

	barrelMgr := handler.NewBarrelManager()
	smokerMgr := handler.NewSmokerManager(players)
	blastFurnaceMgr := handler.NewBlastFurnaceManager(players)
	shulkerBoxMgr := handler.NewShulkerBoxManager()
	grindstoneMgr := handler.NewGrindstoneManager(world)
	stonecutterMgr := handler.NewStonecutterManager(world)
	smithingMgr := handler.NewSmithingTableManager(world)
	spawnerMgr := handler.NewSpawnerManager(mobMgr, players, world)
	enchantMgr := handler.NewEnchantManager(world)
	anvilMgr := handler.NewAnvilManager(world)
	villagerMgr := handler.NewVillagerManager(mobMgr, players)
	villageMgr := handler.NewVillageManager(mobMgr, players, world, logger)
	fluidMgr := handler.NewFluidManager(world, players)
	fallingMgr := handler.NewFallingBlockManager(world, players)
	treeMgr := handler.NewTreeGrowthManager(world, players)
	cropMgr := handler.NewCropManager(world, players)
	weatherMgr := handler.NewWeatherManager(players)
	weatherMgr.Rules = gameRules
	mobMgr.WeatherMgr = weatherMgr
	mobMgr.Rules = gameRules

	hiveMgr := handler.NewHiveManager(mobMgr, players, itemEntities, survHandler, world, logger)
	mobMgr.HiveMgr = hiveMgr

	bedMgr := &handler.BedManager{
		Manager:    players,
		MobManager: mobMgr,
		TimeMgr:    timeMgr,
		WeatherMgr: weatherMgr,
		World:      world,
		Logger:     logger,
		AdvMgr:     advancementMgr,
	}

	bowMgr := &handler.BowManager{
		Manager:  players,
		ArrowMgr: arrowMgr,
		Survival: survHandler,
		Logger:   logger,
	}

	crossbowMgr := &handler.CrossbowManager{
		Manager:  players,
		ArrowMgr: arrowMgr,
		Survival: survHandler,
		Logger:   logger,
	}

	lightningMgr := handler.NewLightningManager(players, weatherMgr, mobMgr, survHandler, logger)

	tridentMgr := &handler.TridentManager{
		Manager:    players,
		ArrowMgr:   arrowMgr,
		Survival:   survHandler,
		WeatherMgr: weatherMgr,
		World:      world,
		Logger:     logger,
	}

	elytraMgr := &handler.ElytraManager{
		Manager:  players,
		Survival: survHandler,
		Logger:   logger,
	}

	fishingMgr := handler.NewFishingManager(players, itemEntities, world, logger)
	signMgr := handler.NewSignManager(players, world, logger)
	decoratedPotMgr := handler.NewDecoratedPotManager(players, itemEntities)
	boatMgr := handler.NewBoatManager(players, world, itemEntities, logger)
	tntMgr := handler.NewTNTManager(players, world, survHandler, itemEntities, logger)
	fireMgr := handler.NewFireManager(players, world, survHandler, logger)
	fireMgr.Rules = gameRules
	redstoneMgr := handler.NewRedstoneManager(players, world, logger)
	wireMgr := handler.NewWireManager(players, world)
	pistonMgr := handler.NewPistonManager(players, world)
	hopperMgr := handler.NewHopperManager(players, world)
	dispenserMgr := handler.NewDispenserManager(players, world, itemEntities)

	// Wire redstone sub-managers
	redstoneMgr.WireMgr = wireMgr
	redstoneMgr.PistonMgr = pistonMgr
	redstoneMgr.DispenserMgr = dispenserMgr
	redstoneMgr.TNTMgr = tntMgr
	redstoneMgr.TimeMgr = timeMgr
	hopperMgr.Chests = chestMgr
	hopperMgr.Furnaces = furnaceMgr
	dispenserMgr.ArrowMgr = arrowMgr
	hopperMgr.WireMgr = wireMgr
	wireMgr.ChestMgr = chestMgr
	wireMgr.FurnaceMgr = furnaceMgr
	wireMgr.HopperMgr = hopperMgr
	wireMgr.BarrelMgr = barrelMgr
	wireMgr.BrewingMgr = brewingMgr

	minecartMgr := handler.NewMinecartManager(players, world, redstoneMgr, itemEntities, logger)

	effectMgr := handler.NewEffectManager(players, survHandler, logger)
	potionMgr := handler.NewPotionManager(players, effectMgr, survHandler, logger)
	jukeboxMgr := handler.NewJukeboxManager(players, world, itemEntities)
	lecternMgr := handler.NewLecternManager(players, world, itemEntities)
	bannerMgr := handler.NewBannerManager(players, world)
	mapMgr := handler.NewMapManager(players, world)
	composterMgr := handler.NewComposterManager(players, world, logger)
	cauldronMgr := handler.NewCauldronManager(players, world, logger)
	beaconMgr := handler.NewBeaconManager(players, world, effectMgr, logger)
	witherMgr := handler.NewWitherManager(world, players, survHandler, effectMgr, itemEntities, logger)
	armorStandMgr := handler.NewArmorStandManager(players, itemEntities, logger)
	itemFrameMgr := handler.NewItemFrameManager(players, itemEntities, logger)
	paintingMgr := handler.NewPaintingManager(players, world, itemEntities, logger)
	leashMgr := handler.NewLeashManager(players, mobMgr, itemEntities, logger)
	respawnAnchorMgr := &handler.RespawnAnchorManager{
		Manager:  players,
		World:    world,
		TNTMgr:   tntMgr,
		Survival: survHandler,
		Logger:   logger,
	}
	mobMgr.EffectMgr = effectMgr
	turtleMgr := handler.NewTurtleManager(mobMgr, players, world, logger)
	mobMgr.TurtleMgr = turtleMgr
	conduitMgr := handler.NewConduitManager(players, world, effectMgr, mobMgr, logger)
	raidMgr := handler.NewRaidManager(players, mobMgr, world, effectMgr, logger)

	survHandler.ItemEntities = itemEntities
	survHandler.Rules = gameRules
	survHandler.EffectMgr = effectMgr

	permMgr := handler.NewPermissionManager("ops.json", "whitelist.json")

	gp := &gamePlay{
		logger:          logger,
		world:           world,
		players:         players,
		sections:        sections,
		minY:            minY,
		spawnY:          spawnY,
		isFlat:          isFlat,
		playerStore:     playerStore,
		pgStore:         pg,
		survivalHandler: survHandler,
		commandGraph:    handler.BuildCommandGraph(),
		chests:          chestMgr,
		itemEntities:    itemEntities,
		furnaces:        furnaceMgr,
		brewingMgr:      brewingMgr,
		timeMgr:         timeMgr,
		mobMgr:          mobMgr,
		arrowMgr:        arrowMgr,
		xpOrbMgr:        xpOrbMgr,
		fluidMgr:        fluidMgr,
		fallingMgr:      fallingMgr,
		treeMgr:         treeMgr,
		cropMgr:         cropMgr,
		weatherMgr:      weatherMgr,
		enchantMgr:      enchantMgr,
		anvilMgr:        anvilMgr,
		villagerMgr:     villagerMgr,
		bedMgr:          bedMgr,
		bowMgr:          bowMgr,
		crossbowMgr:     crossbowMgr,
		tridentMgr:      tridentMgr,
		elytraMgr:       elytraMgr,
		fishingMgr:      fishingMgr,
		signMgr:         signMgr,
		decoratedPotMgr: decoratedPotMgr,
		boatMgr:         boatMgr,
		minecartMgr:     minecartMgr,
		effectMgr:       effectMgr,
		potionMgr:       potionMgr,
		tntMgr:          tntMgr,
		fireMgr:         fireMgr,
		redstoneMgr:     redstoneMgr,
		wireMgr:         wireMgr,
		pistonMgr:       pistonMgr,
		hopperMgr:       hopperMgr,
		dispenserMgr:    dispenserMgr,
		barrelMgr:       barrelMgr,
		grindstoneMgr:   grindstoneMgr,
		stonecutterMgr:  stonecutterMgr,
		smokerMgr:       smokerMgr,
		blastFurnaceMgr: blastFurnaceMgr,
		shulkerBoxMgr:   shulkerBoxMgr,
		smithingMgr:     smithingMgr,
		jukeboxMgr:      jukeboxMgr,
		lecternMgr:      lecternMgr,
		bannerMgr:       bannerMgr,
		mapMgr:          mapMgr,
		composterMgr:    composterMgr,
		cauldronMgr:     cauldronMgr,
		beaconMgr:       beaconMgr,
		witherMgr:       witherMgr,
		armorStandMgr:   armorStandMgr,
		itemFrameMgr:    itemFrameMgr,
		paintingMgr:     paintingMgr,
		leashMgr:         leashMgr,
		respawnAnchorMgr: respawnAnchorMgr,
		hiveMgr:          hiveMgr,
		advancementMgr:   advancementMgr,
		permMgr:         permMgr,
		gameRules:       gameRules,
		tracer:          tp.Tracer("gearworks-mc"),
	}

	// Create DimensionManager for cross-dimension teleportation
	dimensionMgr := &handler.DimensionManager{
		Manager:     players,
		OverWorld:   world,
		NetherWorld: netherWorld,
		EndWorld:    endWorld,
		OverEncoder: &handler.ChunkSender{
			World: world,
			MinY:  minY,
		},
		NetherEncoder: &handler.ChunkSender{
			World: netherWorld,
			MinY:  netherGen.MinY,
		},
		EndEncoder: &handler.ChunkSender{
			World: endWorld,
			MinY:  endGen.MinY,
		},
		Logger:             logger,
		NetherDimTypeID:    3, // overworld=0, overworld_caves=1, the_end=2, the_nether=3
		OverworldDimTypeID: 0,
		EndDimTypeID:       2,
		OverworldSpawnY:    spawnY,
		IsFlat:             isFlat,
		AdvMgr:             advancementMgr,
	}

	// Create Ender Dragon manager
	dragonMgr := handler.NewEnderDragonManager(endWorld, players, mobMgr, survHandler, logger)

	// Create End Portal manager
	endPortalMgr := &handler.EndPortalManager{
		World:        world, // overworld
		EndWorld:     endWorld,
		Manager:      players,
		DimensionMgr: dimensionMgr,
		Logger:       logger,
	}

	// Wire End managers into DimensionManager
	dimensionMgr.EndPortalMgr = endPortalMgr
	dimensionMgr.DragonMgr = dragonMgr

	gp.dimensionMgr = dimensionMgr
	gp.endPortalMgr = endPortalMgr
	gp.dragonMgr = dragonMgr

	// Load block entities (chests, furnaces) from DB
	gp.loadBlockEntities()

	// Load persisted mobs
	mobMgr.LoadSavedMobs("overworld")

	srv := server.Server{
		Logger:          logger,
		ListPingHandler: &pingHandler{players: players},
		LoginHandler: &server.MojangLoginHandler{
			OnlineMode: true,
			Threshold:  256,
		},
		ConfigHandler: &server.Configurations{
			Registries: regs,
			KnownPacks: []server.KnownPack{
				{Namespace: "minecraft", ID: "core", Version: server.ProtocolName},
			},
			KnownPackEntries: vanillaRegistryKeys(),
			Tags:             vanillaConfigTags(),
			Logger:           logger,
		},
		GamePlay: gp,
	}

	// Start metrics/health HTTP server
	metrics := &MetricsServer{
		Players: players,
		MobMgr:  mobMgr,
	}
	metrics.StartMetricsServer(MetricsAddr())
	metrics.SetReady()
	logger.Printf("Metrics server listening on %s", MetricsAddr())

	// Start tick loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	foodHandler := &handler.FoodHandler{Logger: logger, FishingMgr: fishingMgr, PotionMgr: potionMgr, EffectMgr: effectMgr}
	gp.foodHandler = foodHandler
	tickLoop := game.NewTickLoop(
		game.TickHandlerFunc(func(tick int64) {
			tickStart := time.Now()

			gp.keepalive.Tick(tick, players)
			survHandler.HungerTick(players, tick)
			survHandler.VoidDamageTick(players, minY)
			survHandler.FireTick(players)
			survHandler.WaterTick(players, world)
			survHandler.EnvironmentDamageTick(players, world, tick)
			foodHandler.Tick(players)
			itemEntities.Tick(tick)
			furnaceMgr.Tick()
			smokerMgr.TickSmokers()
			blastFurnaceMgr.TickBlastFurnaces()
			brewingMgr.Tick()
			timeMgr.Tick(tick, players)
			mobMgr.Tick(tick)
			spawnerMgr.Tick(tick)
			arrowMgr.Tick(tick)
			xpOrbMgr.Tick(tick)
			fishingMgr.Tick(tick)
			fluidMgr.Tick(tick)
			fallingMgr.Tick(tick)
			treeMgr.Tick(tick)
			cropMgr.Tick(tick)
			weatherMgr.Tick(tick)
			lightningMgr.Tick(tick)
			bedMgr.Tick(tick)
			boatMgr.Tick(tick)
			minecartMgr.Tick(tick)
			effectMgr.Tick(tick)
			potionMgr.Tick(tick)
			tntMgr.Tick(tick)
			fireMgr.Tick(tick)
			redstoneMgr.Tick(tick)
			wireMgr.Tick(tick)
			hopperMgr.Tick(tick)
			tridentMgr.Tick(tick)
			elytraMgr.Tick(tick)
			dimensionMgr.Tick(tick)
			dragonMgr.Tick(tick)
			beaconMgr.Tick(tick)
			witherMgr.Tick(tick)
			leashMgr.Tick(tick)
			raidMgr.Tick(tick)
			turtleMgr.TickTurtleEggs(tick)
			conduitMgr.Tick(tick)
			villagerMgr.TickRestock(tick, timeMgr)
			villageMgr.Tick(tick)

			metrics.RecordTick(time.Since(tickStart))
		}),
	)

	// Add dirty-chunk flush handlers (every 30s = 600 ticks)
	if dbw != nil {
		tickLoop.AddHandler(dbw.FlushTick(600))
	}
	if netherDBW != nil {
		tickLoop.AddHandler(netherDBW.FlushTick(600))
	}
	if endDBW != nil {
		tickLoop.AddHandler(endDBW.FlushTick(600))
	}

	// Auto-save all online players every 5 minutes (6000 ticks)
	if playerStore != nil {
		tickLoop.AddHandler(game.TickHandlerFunc(func(tick int64) {
			if tick%6000 != 0 || tick == 0 {
				return
			}
			players.ForEach(func(p *game.Player) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := playerStore.SavePlayer(ctx, buildPlayerState(p, advancementMgr)); err != nil {
					logger.Printf("Auto-save failed for %s: %v", p.Name, err)
				}
				cancel()
			})
		}))
	}

	go tickLoop.Run(ctx)

	// Graceful shutdown: flush dirty chunks on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Printf("Shutting down...")
		tracerShutdown()

		// Save all online players
		if playerStore != nil {
			players.ForEach(func(p *game.Player) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := playerStore.SavePlayer(ctx, buildPlayerState(p, advancementMgr)); err != nil {
					logger.Printf("Shutdown save failed for %s: %v", p.Name, err)
				}
				cancel()
			})
			logger.Printf("Shutdown: saved all online players")
		}

		// Save all block entities
		gp.saveAllBlockEntities()

		// Save mobs for all dimensions
		mobMgr.SaveAllMobs("overworld")

		// Flush dirty chunks for all dimensions
		for _, dw := range []*dbworld.World{dbw, netherDBW, endDBW} {
			if dw != nil {
				n, err := dw.FlushDirty(context.Background())
				if err != nil {
					logger.Printf("Shutdown flush error: %v", err)
				} else if n > 0 {
					logger.Printf("Shutdown: flushed %d dirty chunks", n)
				}
			}
		}

		cancel()
		os.Exit(0)
	}()

	addr := ":25565"
	logger.Printf("Starting server on %s (protocol %s / %d)", addr, server.ProtocolName, server.ProtocolVersion)
	if err := srv.Listen(addr); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListPingHandler
// ---------------------------------------------------------------------------

type pingHandler struct {
	players *game.PlayerManager
}

func (p *pingHandler) Name() string                    { return server.ProtocolName }
func (p *pingHandler) Protocol(int32) int              { return server.ProtocolVersion }
func (p *pingHandler) MaxPlayer() int                  { return 20 }
func (p *pingHandler) OnlinePlayer() int               { return p.players.Count() }
func (p *pingHandler) PlayerSamples() []server.PlayerSample { return nil }
func (p *pingHandler) Description() *chat.Message {
	msg := chat.Text("A Go-MC 26.1-snapshot-2 Server")
	return &msg
}
func (p *pingHandler) FavIcon() string { return "" }

// ---------------------------------------------------------------------------
// GamePlay
// ---------------------------------------------------------------------------

type gamePlay struct {
	logger      *log.Logger
	world       game.World
	players     *game.PlayerManager
	sections    int
	minY        int
	spawnY      float64
	isFlat      bool
	playerStore store.PlayerStore
	pgStore     *pgstore.PGStore // nil when not using DB

	keepalive       handler.KeepaliveHandler
	survivalHandler *handler.SurvivalHandler
	foodHandler     *handler.FoodHandler
	commandGraph    *command.Graph
	chests          *handler.ChestManager
	itemEntities    *handler.ItemEntityManager
	furnaces        *handler.FurnaceManager
	brewingMgr      *handler.BrewingStandManager
	timeMgr         *handler.TimeManager
	mobMgr          *handler.MobManager
	arrowMgr        *handler.ArrowManager
	xpOrbMgr        *handler.XPOrbManager
	fluidMgr        *handler.FluidManager
	fallingMgr      *handler.FallingBlockManager
	treeMgr         *handler.TreeGrowthManager
	cropMgr         *handler.CropManager
	weatherMgr      *handler.WeatherManager
	enchantMgr      *handler.EnchantManager
	anvilMgr        *handler.AnvilManager
	villagerMgr     *handler.VillagerManager
	bedMgr          *handler.BedManager
	bowMgr          *handler.BowManager
	crossbowMgr     *handler.CrossbowManager
	tridentMgr      *handler.TridentManager
	elytraMgr       *handler.ElytraManager
	fishingMgr      *handler.FishingManager
	signMgr         *handler.SignManager
	decoratedPotMgr *handler.DecoratedPotManager
	boatMgr         *handler.BoatManager
	minecartMgr     *handler.MinecartManager
	effectMgr       *handler.EffectManager
	potionMgr       *handler.PotionManager
	tntMgr          *handler.TNTManager
	fireMgr          *handler.FireManager
	redstoneMgr     *handler.RedstoneManager
	wireMgr         *handler.WireManager
	pistonMgr       *handler.PistonManager
	hopperMgr       *handler.HopperManager
	dispenserMgr    *handler.DispenserManager
	dimensionMgr    *handler.DimensionManager
	endPortalMgr    *handler.EndPortalManager
	dragonMgr       *handler.EnderDragonManager
	barrelMgr       *handler.BarrelManager
	grindstoneMgr   *handler.GrindstoneManager
	stonecutterMgr  *handler.StonecutterManager
	smokerMgr       *handler.SmokerManager
	blastFurnaceMgr *handler.BlastFurnaceManager
	shulkerBoxMgr   *handler.ShulkerBoxManager
	smithingMgr     *handler.SmithingTableManager
	jukeboxMgr      *handler.JukeboxManager
	lecternMgr      *handler.LecternManager
	bannerMgr       *handler.BannerManager
	mapMgr          *handler.MapManager
	composterMgr    *handler.ComposterManager
	cauldronMgr     *handler.CauldronManager
	beaconMgr       *handler.BeaconManager
	witherMgr       *handler.WitherManager
	armorStandMgr   *handler.ArmorStandManager
	itemFrameMgr    *handler.ItemFrameManager
	paintingMgr     *handler.PaintingManager
	leashMgr         *handler.LeashManager
	respawnAnchorMgr *handler.RespawnAnchorManager
	hiveMgr          *handler.HiveManager
	gameRules        *handler.GameRules
	advancementMgr  *handler.AdvancementManager
	permMgr         *handler.PermissionManager
	tracer          trace.Tracer
}

func (g *gamePlay) logf(format string, args ...any) {
	if g.logger != nil {
		g.logger.Printf(format, args...)
	}
}

// saveBlockEntity serializes and saves a chest or furnace to PostgreSQL.
func (g *gamePlay) saveBlockEntity(containerType string, pos [3]int) {
	if g.pgStore == nil {
		return
	}
	var data []byte
	var err error
	switch containerType {
	case "chest":
		cs := g.chests.Get(pos[0], pos[1], pos[2])
		if cs == nil {
			return
		}
		data, err = json.Marshal(cs.Items)
	case "furnace":
		fs := g.furnaces.Get(pos[0], pos[1], pos[2])
		if fs == nil {
			return
		}
		data, err = json.Marshal(map[string]interface{}{
			"input":      fs.Input,
			"fuel":       fs.Fuel,
			"output":     fs.Output,
			"burn_time":  fs.BurnTime,
			"max_burn":   fs.MaxBurnTime,
			"cook_time":  fs.CookTime,
		})
	default:
		return
	}
	if err != nil {
		g.logf("Failed to marshal %s at %v: %v", containerType, pos, err)
		return
	}
	if err := g.pgStore.SaveBlockEntity(context.Background(), "overworld", pos[0], pos[1], pos[2], containerType, data); err != nil {
		g.logf("Failed to save %s at %v: %v", containerType, pos, err)
	}
}

// loadBlockEntities loads all block entities from DB and populates managers.
func (g *gamePlay) loadBlockEntities() {
	if g.pgStore == nil {
		return
	}
	entities, err := g.pgStore.LoadBlockEntities(context.Background(), "overworld")
	if err != nil {
		g.logf("Failed to load block entities: %v", err)
		return
	}
	g.chests.LoadAll(entities)
	g.furnaces.LoadAll(entities)
	g.barrelMgr.LoadAll(entities)
	g.shulkerBoxMgr.LoadAll(entities)
	g.hopperMgr.LoadAll(entities)
	g.brewingMgr.LoadAll(entities)
	g.smokerMgr.LoadAll(entities)
	g.blastFurnaceMgr.LoadAll(entities)
	g.signMgr.LoadAll(entities)
	g.bannerMgr.LoadAll(entities)
	g.hiveMgr.LoadAll(entities)
	g.decoratedPotMgr.LoadAll(entities)
	g.logf("Loaded %d block entities from database", len(entities))
}

// saveAllBlockEntities collects all block entity state and batch-saves to PostgreSQL.
func (g *gamePlay) saveAllBlockEntities() {
	if g.pgStore == nil {
		return
	}
	dim := "overworld"
	var all []store.BlockEntityData
	all = append(all, g.chests.SaveAll(dim)...)
	all = append(all, g.furnaces.SaveAll(dim)...)
	all = append(all, g.barrelMgr.SaveAll(dim)...)
	all = append(all, g.shulkerBoxMgr.SaveAll(dim)...)
	all = append(all, g.hopperMgr.SaveAll(dim)...)
	all = append(all, g.brewingMgr.SaveAll(dim)...)
	all = append(all, g.smokerMgr.SaveAll(dim)...)
	all = append(all, g.blastFurnaceMgr.SaveAll(dim)...)
	all = append(all, g.signMgr.SaveAll(dim)...)
	all = append(all, g.bannerMgr.SaveAll(dim)...)
	all = append(all, g.hiveMgr.SaveAll(dim)...)
	all = append(all, g.decoratedPotMgr.SaveAll(dim)...)
	if len(all) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := g.pgStore.SaveBlockEntities(ctx, all); err != nil {
		g.logf("Failed to save block entities: %v", err)
	} else {
		g.logf("Shutdown: saved %d block entities", len(all))
	}
}

func (g *gamePlay) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	// Whitelist enforcement
	if g.permMgr != nil && !g.permMgr.IsWhitelisted(id) {
		disconnectPkt := pk.Marshal(
			packetid.ClientboundDisconnect,
			chat.Text("You are not whitelisted on this server."),
		)
		_ = conn.WritePacket(disconnectPkt)
		return
	}

	eid := g.players.NextEntityID()

	// Start root session span (gives each player session a unique trace ID)
	tracer := g.tracer
	if tracer == nil {
		tracer = trace.NewNoopTracerProvider().Tracer("")
	}
	ctx, sessionSpan := tracer.Start(context.Background(), "player.session",
		trace.WithAttributes(
			attribute.String("player.name", name),
			attribute.String("player.uuid", id.String()),
			attribute.Int("player.eid", int(eid)),
		),
	)
	defer sessionSpan.End()

	player := game.NewPlayer(name, id, eid, conn)
	if sessionSpan.IsRecording() {
		player.SessionEvents = &otelSessionEvents{span: sessionSpan}
	}
	_ = ctx // used for child spans below
	player.Properties = properties
	player.ProfileKey = profilePubKey
	player.ChatSessionID = uuid.New()
	spawnX, spawnYVal, spawnZ := 0.5, g.spawnY, 0.5
	var spawnYaw, spawnPitch float32

	// Load saved position if DB is available
	if g.playerStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ps, err := g.playerStore.LoadPlayer(ctx, id)
		cancel()
		if err != nil {
			g.logf("Warning: failed to load player %s state: %v", name, err)
		} else if ps != nil {
			spawnX, spawnYVal, spawnZ = ps.X, ps.Y, ps.Z
			spawnYaw, spawnPitch = ps.Yaw, ps.Pitch
			player.Health = ps.Health
			player.Food = ps.Food
			player.Saturation = ps.Saturation
			player.Exhaustion = ps.Exhaustion
			player.GameMode = int32(ps.GameMode)
			player.Experience = ps.Experience
			player.ExperienceLevel = ps.ExperienceLevel
			player.ExperienceTotal = ps.ExperienceTotal
			// Restore spawn point
			player.HasSpawnPoint = ps.HasSpawnPoint
			player.SpawnX = ps.SpawnX
			player.SpawnY = ps.SpawnY
			player.SpawnZ = ps.SpawnZ
			// Restore inventory
			for i, slot := range ps.Inventory {
				if i >= len(player.Inventory) {
					break
				}
				player.Inventory[i] = slotToItemStack(slot)
			}
			// Restore ender chest
			for i, slot := range ps.EnderChest {
				if i >= len(player.EnderItems) {
					break
				}
				player.EnderItems[i] = slotToItemStack(slot)
			}
			// Restore effects
			if len(ps.Effects) > 0 {
				player.Effects = make(map[int32]*game.ActiveEffect)
				for _, e := range ps.Effects {
					player.Effects[e.ID] = &game.ActiveEffect{
						ID:       e.ID,
						Level:    e.Level,
						Duration: e.Duration,
						Ambient:  e.Ambient,
					}
				}
			}
			// Restore advancements
			g.advancementMgr.ImportGranted(name, ps.Advancements)
			// Restore unlocked recipes
			player.UnlockedRecipes = ps.UnlockedRecipes
			g.logf("Loaded saved state for %s: pos=(%.1f, %.1f, %.1f) gm=%d", name, spawnX, spawnYVal, spawnZ, ps.GameMode)
		}
	}

	player.SetPosition(spawnX, spawnYVal, spawnZ)
	player.SetRotation(spawnYaw, spawnPitch)
	player.ViewDistance = 10

	g.players.Add(player)
	g.logf("Player %s (%s) joined [protocol=%d, eid=%d]", name, id, protocol, eid)

	defer func() {
		// Clean up fishing bobber on disconnect
		g.fishingMgr.CleanupPlayer(player)

		// Wake player from bed on disconnect so other players' sleep check updates
		if player.Sleeping && g.bedMgr != nil {
			g.bedMgr.WakePlayer(player)
		}

		// Remove dragon boss bar on disconnect
		if g.dragonMgr != nil {
			g.dragonMgr.RemoveBossBarFromPlayer(player)
		}

		// Release leashes on disconnect
		if g.leashMgr != nil {
			g.leashMgr.OnPlayerDisconnect(player.UUID)
		}

		// Dismount vehicle if riding one
		if player.RidingEntityEID != 0 {
			if g.minecartMgr != nil && g.minecartMgr.IsMinecart(player.RidingEntityEID) {
				g.minecartMgr.DismountMinecart(player)
			} else if g.boatMgr != nil {
				g.boatMgr.DismountBoat(player)
			}
		}

		// Remove scoreboard score before leave broadcast
		handler.RemovePlayerScore(g.players, player.Name)

		// Broadcast leave before removing from manager
		handler.BroadcastPlayerLeave(g.players, player)

		// Save player state on disconnect
		if g.playerStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := g.playerStore.SavePlayer(ctx, buildPlayerState(player, g.advancementMgr))
			cancel()
			if err != nil {
				g.logf("Warning: failed to save player %s state: %v", name, err)
			} else {
				sx, sy, sz := player.Position()
				g.logf("Saved state for %s: (%.1f, %.1f, %.1f)", name, sx, sy, sz)
			}
		}
		// Record disconnect position on the session span
		if sessionSpan.IsRecording() {
			px, py, pz := player.Position()
			sessionSpan.AddEvent("player.disconnect", trace.WithAttributes(
				attribute.Float64("x", px),
				attribute.Float64("y", py),
				attribute.Float64("z", pz),
			))
		}

		g.players.Remove(id)
		g.logf("Player %s (%s) left", name, id)
	}()

	// Join span: covers the entire join sequence (JoinGame → initial chunks → tab list)
	_, joinSpan := tracer.Start(ctx, "player.join",
		trace.WithAttributes(
			attribute.Float64("spawn.x", spawnX),
			attribute.Float64("spawn.y", spawnYVal),
			attribute.Float64("spawn.z", spawnZ),
		),
	)

	if err := g.sendJoinGame(conn, eid); err != nil {
		g.logf("Error sending JoinGame to %s: %v", name, err)
		return
	}

	// PlayerAbilities — survival mode: no special abilities
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerAbilities,
		pk.Byte(0x00),    // flags: none (survival)
		pk.Float(0.05),   // fly speed
		pk.Float(0.1),    // field of view modifier
	)); err != nil {
		g.logf("Error sending PlayerAbilities to %s: %v", name, err)
		return
	}

	// PlayerPosition — must come BEFORE chunks
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(1),             // teleport ID
		pk.Double(spawnX),        // x
		pk.Double(spawnYVal),     // y
		pk.Double(spawnZ),        // z
		pk.Double(0),             // vel_x
		pk.Double(0),             // vel_y
		pk.Double(0),             // vel_z
		pk.Float(spawnYaw),       // yaw
		pk.Float(spawnPitch),     // pitch
		pk.Int(0),                // flags: all absolute
	)); err != nil {
		g.logf("Error sending PlayerPosition to %s: %v", name, err)
		return
	}
	player.TeleportPending = true

	if err := g.sendSpawnSequence(player); err != nil {
		g.logf("Error sending spawn sequence to %s: %v", name, err)
		return
	}

	// Send full inventory contents so the client sees any items
	handler.SendFullInventory(player)

	// Send initial health/food/saturation HUD
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundSetHealth,
		pk.Float(player.Health),
		pk.VarInt(player.Food),
		pk.Float(player.Saturation),
	)); err != nil {
		g.logf("Error sending SetHealth to %s: %v", name, err)
		return
	}

	// Send initial experience
	handler.SendExperience(player)

	// Send current world time
	g.timeMgr.SendTime(player)

	// Send current weather state
	if g.weatherMgr != nil {
		g.weatherMgr.SendWeather(player)
	}

	// ServerData — MOTD + optional icon (enforcesSecureChat was removed in 1.20.5+)
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundServerData,
		chat.Text(""),      // MOTD
		pk.Boolean(false),  // has icon
	)); err != nil {
		g.logf("Error sending ServerData to %s: %v", name, err)
		return
	}

	// Server brand — shows in F3 debug screen
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundCustomPayload,
		pk.Identifier("minecraft:brand"),
		pk.String("gearworks-go"),
	)); err != nil {
		g.logf("Error sending brand to %s: %v", name, err)
		return
	}

	// Command tree for tab-completion hints
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundCommands, g.commandGraph,
	)); err != nil {
		g.logf("Error sending Commands to %s: %v", name, err)
		return
	}

	// Send player's own PlayerInfo first (needed for tab list + Chat Heads mod)
	handler.SendPlayerInfo(player, player)

	// Broadcast new player to existing players & send existing players to new player
	// Must happen AFTER JoinGame + chunks so the client's level is initialized.
	handler.BroadcastPlayerJoin(g.players, player)
	handler.SendExistingPlayers(g.players, player)

	// Send existing item entities, mobs, and arrows to the new player
	g.itemEntities.SendExistingItems(player)
	g.mobMgr.SendExistingMobs(player)
	g.arrowMgr.SendExistingArrows(player)
	g.fishingMgr.SendExistingBobbers(player)
	g.boatMgr.SendExistingBoats(player)
	g.minecartMgr.SendExistingMinecarts(player)
	g.tntMgr.SendExistingTNTs(player)
	g.potionMgr.SendExistingPotions(player)
	g.armorStandMgr.SendExistingStands(player)
	g.itemFrameMgr.SendExistingFrames(player)
	g.paintingMgr.SendExistingPaintings(player)
	g.leashMgr.SendExistingKnots(player)

	// Scoreboard: health below names
	handler.SendScoreboard(g.players, player)

	// Tab list header/footer
	tabHeader := chat.Message{Text: "Gearworks", Color: "gold", Bold: true}
	tabFooter := chat.Message{Text: fmt.Sprintf("\n%d player(s) online", g.players.Count()), Color: "gray"}
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundTabList,
		tabHeader, tabFooter,
	)); err != nil {
		g.logf("Error sending TabList to %s: %v", name, err)
		return
	}

	// Send advancements
	g.advancementMgr.SendAdvancementsOnJoin(player)

	joinSpan.End() // join sequence complete

	// Packet read loop with handlers
	g.packetLoop(player)
}

func (g *gamePlay) sendJoinGame(conn *net.Conn, eid int32) error {
	dimensionNames := []pk.Identifier{"minecraft:overworld", "minecraft:the_nether", "minecraft:the_end"}

	return conn.WritePacket(pk.Marshal(
		packetid.ClientboundLogin,
		pk.Int(eid),                // entity ID
		pk.Boolean(false),          // not hardcore
		pk.Array(dimensionNames),   // dimension names
		pk.VarInt(20),              // max players
		pk.VarInt(10),              // view distance
		pk.VarInt(10),              // simulation distance
		pk.Boolean(false),          // reduced debug info
		pk.Boolean(true),           // enable respawn screen
		pk.Boolean(false),          // do limited crafting
		pk.VarInt(0),               // dimension type = index 0
		pk.Identifier("minecraft:overworld"),
		pk.Long(0),                 // hashed seed
		pk.UnsignedByte(0),         // gamemode: survival
		pk.Byte(-1),                // previous gamemode: none
		pk.Boolean(false),          // is debug
		pk.Boolean(g.isFlat),       // is flat
		pk.Boolean(false),          // has death location
		pk.VarInt(0),               // portal cooldown
		pk.VarInt(63),              // sea level
		pk.Boolean(false),          // enforces secure chat (false: unsigned PlayerChat works without "can't be verified" toast)
	))
}

func (g *gamePlay) sendSpawnSequence(player *game.Player) error {
	px, py, pz := player.Position()

	// 1. Set default spawn position
	err := player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDefaultSpawnPosition,
		pk.Identifier("minecraft:overworld"),
		pk.Position{X: int(px), Y: int(py), Z: int(pz)},
		pk.Float(0), pk.Float(0),
	))
	if err != nil {
		return fmt.Errorf("spawn position: %w", err)
	}

	// 2. Game event: start waiting for level chunks
	err = player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), pk.Float(0),
	))
	if err != nil {
		return fmt.Errorf("game event: %w", err)
	}

	// 3. Send initial chunks from the world
	cs := &handler.ChunkSender{
		World: g.world,
		MinY:  g.minY,
	}
	if err := cs.SendInitialChunks(player); err != nil {
		return fmt.Errorf("initial chunks: %w", err)
	}

	return nil
}

func (g *gamePlay) packetLoop(player *game.Player) {
	cs := &handler.ChunkSender{
		World: g.world,
		MinY:  g.minY,
	}
	movHandler := &handler.MovementHandler{
		World:           g.world,
		Manager:         g.players,
		Logger:          g.logger,
		Encoder:         cs,
		SurvivalHandler: g.survivalHandler,
		CropMgr:         g.cropMgr,
		BedMgr:          g.bedMgr,
		BoatMgr:         g.boatMgr,
		MinecartMgr:     g.minecartMgr,
		RedstoneMgr:     g.redstoneMgr,
		DimensionMgr:    g.dimensionMgr,
	}
	blockHandler := &handler.BlockHandler{
		World:        g.world,
		Manager:      g.players,
		Logger:       g.logger,
		ItemEntities: g.itemEntities,
		XPOrbMgr:     g.xpOrbMgr,
		Chests:       g.chests,
		Furnaces:     g.furnaces,
		BrewingMgr:   g.brewingMgr,
		TimeMgr:      g.timeMgr,
		FluidMgr:     g.fluidMgr,
		FallingMgr:   g.fallingMgr,
		TreeMgr:      g.treeMgr,
		CropMgr:      g.cropMgr,
		EnchantMgr:   g.enchantMgr,
		AnvilMgr:     g.anvilMgr,
		BedMgr:       g.bedMgr,
		SignMgr:         g.signMgr,
		DecoratedPotMgr: g.decoratedPotMgr,
		BoatMgr:         g.boatMgr,
		MinecartMgr:  g.minecartMgr,
		TNTMgr:       g.tntMgr,
		FireMgr:      g.fireMgr,
		RedstoneMgr:  g.redstoneMgr,
		WireMgr:      g.wireMgr,
		PistonMgr:    g.pistonMgr,
		HopperMgr:    g.hopperMgr,
		DispenserMgr: g.dispenserMgr,
		DimensionMgr:    g.dimensionMgr,
		EndPortalMgr:    g.endPortalMgr,
		BarrelMgr:       g.barrelMgr,
		GrindstoneMgr:   g.grindstoneMgr,
		StonecutterMgr:  g.stonecutterMgr,
		SmokerMgr:       g.smokerMgr,
		BlastFurnaceMgr: g.blastFurnaceMgr,
		ShulkerBoxMgr:   g.shulkerBoxMgr,
		SmithingMgr:     g.smithingMgr,
		ComposterMgr:    g.composterMgr,
		CauldronMgr:     g.cauldronMgr,
		BeaconMgr:       g.beaconMgr,
		WitherMgr:       g.witherMgr,
		ArmorStandMgr:   g.armorStandMgr,
		ItemFrameMgr:    g.itemFrameMgr,
		PaintingMgr:     g.paintingMgr,
		LeashMgr:        g.leashMgr,
		JukeboxMgr:      g.jukeboxMgr,
		LecternMgr:      g.lecternMgr,
		BannerMgr:        g.bannerMgr,
		RespawnAnchorMgr: g.respawnAnchorMgr,
		HiveMgr:          g.hiveMgr,
		CopperMgr: &handler.CopperManager{
			Manager: g.players,
			World:   g.world,
		},
	}
	if g.pgStore != nil {
		blockHandler.OnBlockBreak = func(blockName string, x, y, z int) {
			if blockName == "chest" || blockName == "furnace" || blockName == "brewing_stand" || blockName == "hopper" || blockName == "dispenser" || blockName == "dropper" {
				go g.pgStore.DeleteBlockEntity(context.Background(), "overworld", x, y, z)
			}
		}
	}
	invHandler := &handler.InventoryHandler{
		Logger:          g.logger,
		Chests:          g.chests,
		Furnaces:        g.furnaces,
		BrewingMgr:      g.brewingMgr,
		HopperMgr:       g.hopperMgr,
		DispenserMgr:    g.dispenserMgr,
		EnchantMgr:      g.enchantMgr,
		AnvilMgr:        g.anvilMgr,
		VillagerMgr:     g.villagerMgr,
		BarrelMgr:       g.barrelMgr,
		GrindstoneMgr:   g.grindstoneMgr,
		StonecutterMgr:  g.stonecutterMgr,
		SmokerMgr:       g.smokerMgr,
		BlastFurnaceMgr: g.blastFurnaceMgr,
		ShulkerBoxMgr:   g.shulkerBoxMgr,
		SmithingMgr:     g.smithingMgr,
		BeaconMgr:       g.beaconMgr,
	}
	if g.pgStore != nil {
		invHandler.OnContainerClose = func(containerType string, pos [3]int) {
			go g.saveBlockEntity(containerType, pos)
		}
	}
	cmdExecutor := &handler.CommandExecutor{
		Manager:         g.players,
		Logger:          g.logger,
		SurvivalHandler: g.survivalHandler,
		TimeMgr:         g.timeMgr,
		WeatherMgr:      g.weatherMgr,
		Rules:           g.gameRules,
		PermMgr:         g.permMgr,
		World:           g.world,
		MobMgr:          g.mobMgr,
		EffectMgr:       g.effectMgr,
	}
	chatHandler := &handler.ChatHandler{
		Manager:     g.players,
		Logger:      g.logger,
		Commands:    cmdExecutor,
		Broadcaster: &chatheads.ChatBroadcaster{Manager: g.players},
	}
	animHandler := &handler.AnimationHandler{
		Manager:   g.players,
		BedMgr:    g.bedMgr,
		ElytraMgr: g.elytraMgr,
	}
	combatHandler := &handler.CombatHandler{
		Manager:         g.players,
		SurvivalHandler: g.survivalHandler,
		MobManager:      g.mobMgr,
		VillagerMgr:     g.villagerMgr,
		BoatMgr:         g.boatMgr,
		MinecartMgr:     g.minecartMgr,
		EffectMgr:       g.effectMgr,
		DragonMgr:       g.dragonMgr,
		WitherMgr:       g.witherMgr,
		ArmorStandMgr:   g.armorStandMgr,
		ItemFrameMgr:    g.itemFrameMgr,
		PaintingMgr:     g.paintingMgr,
		LeashMgr:        g.leashMgr,
		Rules:           g.gameRules,
		Logger:          g.logger,
	}
	respawnHandler := &handler.RespawnHandler{
		Manager:      g.players,
		World:        g.world,
		SpawnY:       g.spawnY,
		MinY:         g.minY,
		Logger:       g.logger,
		DimensionMgr: g.dimensionMgr,
	}

	// Keepalive sender goroutine (sends every 15s)
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				keepAliveID := rand.Int63()
				player.WritePacket(pk.Marshal(
					packetid.ClientboundKeepAlive,
					pk.Long(keepAliveID),
				))
			}
		}
	}()

	var p pk.Packet
	for {
		if err := player.Conn.ReadPacket(&p); err != nil {
			g.logf("Player %s disconnected: %v", player.Name, err)
			return
		}

		// Try each handler
		if movHandler.HandlePacket(player, p) {
			continue
		}
		// Bow/crossbow/trident release (action=5) must be checked before blockHandler claims all PlayerAction packets
		if g.bowMgr.HandlePlayerAction(player, p) {
			continue
		}
		if g.crossbowMgr.HandlePlayerAction(player, p) {
			continue
		}
		if g.tridentMgr.HandlePlayerAction(player, p) {
			continue
		}
		if blockHandler.HandlePacket(player, p) {
			continue
		}
		// Elytra firework boost (UseItem with firework_rocket while gliding) must be checked first
		if g.elytraMgr.HandleUseItem(player, p) {
			continue
		}
		// Bow/crossbow/trident draw (UseItem) must be checked before food handler
		if g.bowMgr.HandleUseItem(player, p) {
			continue
		}
		if g.crossbowMgr.HandleUseItem(player, p) {
			continue
		}
		if g.tridentMgr.HandleUseItem(player, p) {
			continue
		}
		if g.foodHandler.HandlePacket(player, p) {
			continue
		}
		if invHandler.HandlePacket(player, p) {
			continue
		}
		if g.keepalive.HandlePacket(player, p) {
			continue
		}
		if animHandler.HandlePacket(player, p) {
			continue
		}
		if combatHandler.HandlePacket(player, p) {
			continue
		}
		if g.boatMgr.HandleMoveVehicle(player, p) {
			continue
		}
		if g.boatMgr.HandlePaddleBoat(player, p) {
			continue
		}
		if g.minecartMgr.HandleMoveVehicle(player, p) {
			continue
		}
		if respawnHandler.HandlePacket(player, p) {
			continue
		}
		if chatHandler.HandlePacket(player, p) {
			continue
		}

		// Handle other known packets
		switch packetid.ServerboundPacketID(p.ID) {
		case packetid.ServerboundAcceptTeleportation:
			// Acknowledged teleport — no action needed
		case packetid.ServerboundChunkBatchReceived:
			// Client ACK for chunk batch — no action needed
		default:
			// Silently discard unknown packets
		}
	}
}

// ---------------------------------------------------------------------------
// Registry data
// ---------------------------------------------------------------------------

func buildRegistries() registry.Registries {
	regs := registry.NewNetworkCodec()

	regs.DimensionType.Put("minecraft:overworld", registry.Dimension{
		HasSkylight:        true,
		HasCeiling:         false,
		Ultrawarm:          false,
		Natural:            true,
		CoordinateScale:    1.0,
		BedWorks:           true,
		RespawnAnchorWorks: 0,
		MinY:               -64,
		Height:             384,
		LogicalHeight:      384,
		InfiniteBurn:       "#minecraft:infiniburn_overworld",
		Effects:            "minecraft:overworld",
		AmbientLight:       0.0,
		PiglinSafe:         0,
		HasRaids:           1,
		MonsterSpawnLightLevel: mustNBTRaw(map[string]any{
			"type": "minecraft:uniform",
			"value": map[string]any{
				"min_inclusive": int32(0),
				"max_inclusive": int32(7),
			},
		}),
		MonsterSpawnBlockLightLimit: 0,
	})

	regs.WorldGenBiome.Put("minecraft:plains", mustNBTRaw(map[string]any{
		"has_precipitation": byte(1),
		"temperature":      float32(0.8),
		"downfall":         float32(0.4),
		"effects": map[string]any{
			"sky_color":       int32(7907327),
			"water_fog_color": int32(329011),
			"fog_color":       int32(12638463),
			"water_color":     int32(4159204),
		},
	}))

	regs.ChatType.Put("minecraft:chat", registry.ChatType{
		Chat: chat.Decoration{
			TranslationKey: "chat.type.text",
			Parameters:     []string{"sender", "content"},
		},
		Narration: chat.Decoration{
			TranslationKey: "chat.type.text.narrate",
			Parameters:     []string{"sender", "content"},
		},
	})

	regs.DamageType.Put("minecraft:generic", registry.DamageType{
		MessageID:  "generic",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:generic_kill", registry.DamageType{
		MessageID:  "genericKill",
		Scaling:    "never",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:fall", registry.DamageType{
		MessageID:  "fall",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:player_attack", registry.DamageType{
		MessageID:  "player",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.1,
	})
	regs.DamageType.Put("minecraft:out_of_world", registry.DamageType{
		MessageID:  "outOfWorld",
		Scaling:    "never",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:on_fire", registry.DamageType{
		MessageID:  "onFire",
		Scaling:    "never",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:drown", registry.DamageType{
		MessageID:  "drown",
		Scaling:    "never",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:mob_attack", registry.DamageType{
		MessageID:  "mob",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.1,
	})

	return regs
}

// mustNBTRaw encodes v as an nbt.RawMessage suitable for embedding.
func mustNBTRaw(v any) nbt.RawMessage {
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)
	if err := enc.Encode(v, ""); err != nil {
		panic(fmt.Sprintf("nbt.Encode: %v", err))
	}
	data := buf.Bytes()
	return nbt.RawMessage{
		Type: data[0],
		Data: append([]byte(nil), data[1:]...),
	}
}

// buildHeightmapBytes and packHeightmapLongs are kept for backward compatibility
// with existing tests.

// buildHeightmapBytes builds the raw bytes for a heightmap map with 3 entries.
func buildHeightmapBytes(value int32) []byte {
	longs := packHeightmapLongs(value)

	var buf bytes.Buffer
	writeVarInt(&buf, 3)
	writeVarInt(&buf, 1)
	writeLongArray(&buf, longs)
	writeVarInt(&buf, 4)
	writeLongArray(&buf, longs)
	writeVarInt(&buf, 5)
	writeLongArray(&buf, longs)

	return buf.Bytes()
}

func packHeightmapLongs(value int32) []int64 {
	const bitsPerEntry = 9
	const valsPerLong = 64 / bitsPerEntry
	numLongs := (256 + valsPerLong - 1) / valsPerLong

	longs := make([]int64, numLongs)
	for i := 0; i < 256; i++ {
		longIdx := i / valsPerLong
		bitOffset := uint(i%valsPerLong) * bitsPerEntry
		longs[longIdx] |= int64(value) << bitOffset
	}
	return longs
}

func writeVarInt(buf *bytes.Buffer, v int32) {
	val := uint32(v)
	for val >= 0x80 {
		buf.WriteByte(byte(val&0x7F) | 0x80)
		val >>= 7
	}
	buf.WriteByte(byte(val))
}

func writeLongArray(buf *bytes.Buffer, longs []int64) {
	writeVarInt(buf, int32(len(longs)))
	for _, l := range longs {
		buf.WriteByte(byte(l >> 56))
		buf.WriteByte(byte(l >> 48))
		buf.WriteByte(byte(l >> 40))
		buf.WriteByte(byte(l >> 32))
		buf.WriteByte(byte(l >> 24))
		buf.WriteByte(byte(l >> 16))
		buf.WriteByte(byte(l >> 8))
		buf.WriteByte(byte(l))
	}
}

// slotToItemStack converts a store.ItemSlot to a game.ItemStack.
func slotToItemStack(slot store.ItemSlot) game.ItemStack {
	return game.ItemStack{
		ID:            slot.ID,
		Count:         slot.Count,
		Durability:    slot.Durability,
		MaxDurability: slot.MaxDurability,
		Enchantments:  slot.Enchantments,
		DisplayName:   slot.DisplayName,
		PotionType:    slot.PotionType,
	}
}

// itemStackToSlot converts a game.ItemStack to a store.ItemSlot.
func itemStackToSlot(s game.ItemStack) store.ItemSlot {
	return store.ItemSlot{
		ID:            s.ID,
		Count:         s.Count,
		Durability:    s.Durability,
		MaxDurability: s.MaxDurability,
		Enchantments:  s.Enchantments,
		DisplayName:   s.DisplayName,
		PotionType:    s.PotionType,
	}
}

// buildPlayerState creates a store.PlayerState from a live player.
func buildPlayerState(player *game.Player, advMgr *handler.AdvancementManager) *store.PlayerState {
	px, py, pz := player.Position()
	pyaw, ppitch := player.Rotation()

	invSlots := make([]store.ItemSlot, len(player.Inventory))
	for i, s := range player.Inventory {
		invSlots[i] = itemStackToSlot(s)
	}

	var enderSlots []store.ItemSlot
	for _, s := range player.EnderItems {
		if s.ID != 0 {
			enderSlots = append(enderSlots, itemStackToSlot(s))
		}
	}
	// Preserve index-based storage for ender chest
	if len(enderSlots) > 0 {
		enderSlots = make([]store.ItemSlot, len(player.EnderItems))
		for i, s := range player.EnderItems {
			enderSlots[i] = itemStackToSlot(s)
		}
	}

	var effects []store.EffectData
	for _, e := range player.Effects {
		if e != nil {
			effects = append(effects, store.EffectData{
				ID:       e.ID,
				Level:    e.Level,
				Duration: e.Duration,
				Ambient:  e.Ambient,
			})
		}
	}

	return &store.PlayerState{
		UUID:            player.UUID,
		Name:            player.Name,
		Dimension:       player.Dimension,
		X:               px,
		Y:               py,
		Z:               pz,
		Yaw:             pyaw,
		Pitch:           ppitch,
		GameMode:        int(player.GameMode),
		Health:          player.Health,
		Food:            player.Food,
		Saturation:      player.Saturation,
		Exhaustion:      player.Exhaustion,
		Inventory:       invSlots,
		EnderChest:      enderSlots,
		Effects:         effects,
		Experience:      player.Experience,
		ExperienceLevel: player.ExperienceLevel,
		ExperienceTotal: player.ExperienceTotal,
		SpawnX:          player.SpawnX,
		SpawnY:          player.SpawnY,
		SpawnZ:          player.SpawnZ,
		HasSpawnPoint:   player.HasSpawnPoint,
		Advancements:    advMgr.ExportGranted(player.Name),
		UnlockedRecipes: player.UnlockedRecipes,
	}
}
