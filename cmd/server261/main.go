// Command server261 is a Minecraft 26.1-snapshot-2 server with a superflat world,
// 20 TPS tick loop, player movement tracking, and view-distance chunk streaming.
//
// Usage:
//
//	go run ./cmd/server261
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/dbworld"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/game/handler"
	"github.com/Tnze/go-mc/game/mem"
	"github.com/Tnze/go-mc/game/pgstore"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/server"
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

	// Create all gameplay managers
	mgrs := createManagers(logger, players, world, sections, minY, spawnY, isFlat, playerStore, pg, tp.Tracer("gearworks-mc"))
	gp := mgrs.gp

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
		AdvMgr:             mgrs.advancementMgr,
	}

	// Create Ender Dragon manager
	dragonMgr := handler.NewEnderDragonManager(endWorld, players, gp.mobMgr, mgrs.survHandler, logger)

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
	gp.mobMgr.LoadSavedMobs("overworld")

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
		MobMgr:  gp.mobMgr,
	}
	metrics.StartMetricsServer(MetricsAddr())
	metrics.SetReady()
	logger.Printf("Metrics server listening on %s", MetricsAddr())
	tabListMgr := &handler.TabListManager{Players: players, TPS: metrics}

	tpsMgr := &handler.TPSManager{
		Manager: players,
		Metrics: metrics,
		Logger:  logger,
		PermMgr: gp.permMgr,
	}
	gp.tpsMgr = tpsMgr

	// Start tick loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	foodHandler := &handler.FoodHandler{Logger: logger, FishingMgr: gp.fishingMgr, PotionMgr: gp.potionMgr, EffectMgr: gp.effectMgr}
	gp.foodHandler = foodHandler
	tickLoop := game.NewTickLoop(
		game.TickHandlerFunc(func(tick int64) {
			tickStart := time.Now()

			gp.keepalive.Tick(tick, players)
			mgrs.survHandler.HungerTick(players, tick)
			mgrs.survHandler.VoidDamageTick(players, minY)
			mgrs.survHandler.FireTick(players)
			mgrs.survHandler.WaterTick(players, world)
			mgrs.survHandler.EnvironmentDamageTick(players, world, tick)
			foodHandler.Tick(players)
			gp.itemEntities.Tick(tick)
			gp.furnaces.Tick()
			gp.smokerMgr.TickSmokers()
			gp.blastFurnaceMgr.TickBlastFurnaces()
			gp.brewingMgr.Tick()
			gp.timeMgr.Tick(tick, players)
			gp.mobMgr.Tick(tick)
			mgrs.spawnerMgr.Tick(tick)
			gp.arrowMgr.Tick(tick)
			gp.arrowMgr.TickWindCharges(tick)
			gp.xpOrbMgr.Tick(tick)
			gp.fishingMgr.Tick(tick)
			gp.fluidMgr.Tick(tick)
			gp.fallingMgr.Tick(tick)
			gp.treeMgr.Tick(tick)
			gp.cropMgr.Tick(tick)
			gp.weatherMgr.Tick(tick)
			mgrs.lightningMgr.Tick(tick)
			gp.bedMgr.Tick(tick)
			gp.boatMgr.Tick(tick)
			gp.minecartMgr.Tick(tick)
			gp.effectMgr.Tick(tick)
			gp.potionMgr.Tick(tick)
			gp.tntMgr.Tick(tick)
			gp.fireMgr.Tick(tick)
			gp.redstoneMgr.Tick(tick)
			gp.wireMgr.Tick(tick)
			gp.hopperMgr.Tick(tick)
			gp.tridentMgr.Tick(tick)
			gp.elytraMgr.Tick(tick)
			dimensionMgr.Tick(tick)
			dragonMgr.Tick(tick)
			gp.beaconMgr.Tick(tick)
			gp.witherMgr.Tick(tick)
			gp.leashMgr.Tick(tick)
			mgrs.raidMgr.Tick(tick)
			mgrs.sculkMgr.Tick(tick)
			mgrs.turtleMgr.TickTurtleEggs(tick)
			mgrs.conduitMgr.Tick(tick)
			gp.worldBorderMgr.Tick(tick)
			gp.villagerMgr.TickRestock(tick, gp.timeMgr)
			mgrs.villageMgr.Tick(tick)
			tabListMgr.Tick(tick)
			mgrs.collisionMgr.Tick()

			if tick%1200 == 0 {
				gp.copperMgr.TickWeathering()
			}

			tickDur := time.Since(tickStart)
			metrics.RecordTick(tickDur)
			tpsMgr.Tick(tickDur)
		}),
	)

	// Pair each DB-backed world with an async flusher.
	type dimWorld struct {
		world   *dbworld.World
		flusher *dbworld.AsyncFlusher
	}
	var dbWorlds []dimWorld
	allDBWorlds := []*dbworld.World{dbw, netherDBW, endDBW}
	for _, dw := range allDBWorlds {
		if dw != nil {
			dbWorlds = append(dbWorlds, dimWorld{
				world:   dw,
				flusher: dbworld.NewAsyncFlusher(pg, logger),
			})
		}
	}

	// Periodic chunk management: tick update, eviction, and async flush.
	evictionPolicy := dbworld.DefaultEvictionPolicy()
	tickLoop.AddHandler(game.TickHandlerFunc(func(tick int64) {
		for _, dw := range dbWorlds {
			dw.world.SetTick(tick)
		}

		// Chunk eviction every 1200 ticks.
		if tick%1200 == 0 && tick > 0 && len(dbWorlds) > 0 {
			var positions [][2]int
			vd := 10
			players.ForEach(func(p *game.Player) {
				px, _, pz := p.Position()
				positions = append(positions, [2]int{int(px) >> 4, int(pz) >> 4})
				if p.ViewDistance > vd {
					vd = p.ViewDistance
				}
			})
			for _, dw := range dbWorlds {
				n := dbworld.EvictUnused(dw.world, positions, vd, tick, evictionPolicy)
				if n > 0 {
					logger.Printf("Evicted %d idle chunks (loaded=%d)", n, dw.world.ChunkCount())
				}
			}
		}

		// Async flush every 6000 ticks.
		if tick%6000 == 0 && tick > 0 {
			for _, dw := range dbWorlds {
				dw.flusher.FlushDirty(dw.world)
			}
		}
	}))

	// Auto-save all online players every 5 minutes (6000 ticks)
	if playerStore != nil {
		tickLoop.AddHandler(game.TickHandlerFunc(func(tick int64) {
			if tick%6000 != 0 || tick == 0 {
				return
			}
			players.ForEach(func(p *game.Player) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := playerStore.SavePlayer(ctx, buildPlayerState(p, mgrs.advancementMgr)); err != nil {
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
				if err := playerStore.SavePlayer(ctx, buildPlayerState(p, mgrs.advancementMgr)); err != nil {
					logger.Printf("Shutdown save failed for %s: %v", p.Name, err)
				}
				cancel()
			})
			logger.Printf("Shutdown: saved all online players")
		}

		// Save all block entities
		gp.saveAllBlockEntities()

		// Save mobs for all dimensions
		gp.mobMgr.SaveAllMobs("overworld")

		// Close async flushers (drain pending writes), then flush remaining dirty chunks.
		for _, dw := range dbWorlds {
			dw.flusher.Close()
			n, err := dw.world.FlushDirty(context.Background())
			if err != nil {
				logger.Printf("Shutdown flush error: %v", err)
			} else if n > 0 {
				logger.Printf("Shutdown: flushed %d dirty chunks", n)
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
