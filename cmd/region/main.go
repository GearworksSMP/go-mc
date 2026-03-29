// Command region is a Minecraft region server that runs as part of a distributed cluster.
// It trusts the proxy for authentication and uses range-based entity IDs to avoid collisions.
//
// Environment variables:
//   - SERVER_ID: this server's ID in the cluster config (required)
//   - LISTEN_ADDR: address to listen on (default ":25566")
//   - DATABASE_URL: PostgreSQL connection string
//   - REDIS_URL: Redis connection string
//   - CLUSTER_CONFIG_FILE or CLUSTER_CONFIG: cluster topology
//   - WORLD_TYPE: "flat" for superflat, default is terrain
//   - WORLD_SEED: seed for terrain generation
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
	"sync"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/cluster"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/dbworld"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/game/handler"
	"github.com/Tnze/go-mc/game/mem"
	"github.com/Tnze/go-mc/game/pgstore"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/server/command"
	"github.com/Tnze/go-mc/server/vanilla"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func main() {
	serverID := os.Getenv("SERVER_ID")
	if serverID == "" {
		log.Fatal("SERVER_ID environment variable is required")
	}

	logger := log.New(os.Stdout, fmt.Sprintf("[%s] ", serverID), log.LstdFlags|log.Lmsgprefix)

	cfg, err := cluster.LoadConfigFromEnv()
	if err != nil {
		logger.Fatalf("Failed to load cluster config: %v", err)
	}

	// Determine server index for EID allocation
	serverIndex := 0
	i := 0
	for id := range cfg.Servers {
		if id == serverID {
			serverIndex = i
			break
		}
		i++
	}
	eidAlloc := cluster.NewEIDAllocator(serverIndex)
	logger.Printf("EID allocator: server index %d", serverIndex)

	// Connect to Redis
	var rdb *redis.Client
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			logger.Fatalf("Invalid REDIS_URL: %v", err)
		}
		rdb = redis.NewClient(opts)
		defer rdb.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rdb.Ping(ctx).Err(); err != nil {
			logger.Fatalf("Redis ping failed: %v", err)
		}
		cancel()
		logger.Println("Connected to Redis")
	}

	// World setup
	players := game.NewPlayerManager()
	players.EIDAllocFunc = eidAlloc.AllocFunc()

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
	} else {
		seed := int64(12345)
		if s := os.Getenv("WORLD_SEED"); s != "" {
			for _, c := range s {
				seed = seed*31 + int64(c)
			}
		}
		tGen := gen.NewTerrainGenerator(seed)
		worldGen = tGen
		sections = tGen.Sections
		minY = tGen.MinY
		spawnY = tGen.SpawnY()
	}

	var world game.World
	var playerStore store.PlayerStore
	var pg *pgstore.PGStore
	var dbw *dbworld.World

	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		logger.Printf("Connecting to PostgreSQL...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

		dbw = dbworld.NewWorld(pg, worldGen, sections, minY, "overworld", logger)
		world = dbw
		playerStore = pg
	} else {
		world = mem.NewWorld(worldGen, sections, minY)
	}

	regs := buildRegistries()

	survHandler := &handler.SurvivalHandler{
		Logger:             logger,
		FallDamageTypeID:   2,
		AttackDamageTypeID: 3,
		VoidDamageTypeID:   4,
	}

	gameRules := handler.NewGameRules()
	chestMgr := handler.NewChestManager()
	chestMgr.Manager = players
	itemEntities := handler.NewItemEntityManager(players)
	furnaceMgr := handler.NewFurnaceManager(players)
	brewingMgr := handler.NewBrewingStandManager(players)
	timeMgr := &handler.TimeManager{}
	arrowMgr := handler.NewArrowManager(players, survHandler, world)
	mobMgr := handler.NewMobManager(players, timeMgr, world, minY, survHandler, itemEntities)
	mobMgr.ArrowMgr = arrowMgr
	mobMgr.Logger = logger
	if pg != nil {
		mobMgr.MobStore = pg
	}
	advancementMgr := handler.NewAdvancementManager(players)
	mobMgr.AdvMgr = advancementMgr

	cropMgr := handler.NewCropManager(world, players)
	weatherMgr := handler.NewWeatherManager(players)
	mobMgr.WeatherMgr = weatherMgr

	bedMgr := &handler.BedManager{
		Manager:    players,
		MobManager: mobMgr,
		TimeMgr:    timeMgr,
		WeatherMgr: weatherMgr,
		World:      world,
		Logger:     logger,
		AdvMgr:     advancementMgr,
	}

	boatMgr := handler.NewBoatManager(players, world, itemEntities, logger)
	tntMgr := handler.NewTNTManager(players, world, survHandler, itemEntities, logger)
	redstoneMgr := handler.NewRedstoneManager(players, world, logger)
	minecartMgr := handler.NewMinecartManager(players, world, redstoneMgr, itemEntities, logger)

	fluidMgr := handler.NewFluidManager(world, players)
	fallingMgr := handler.NewFallingBlockManager(world, players)
	treeMgr := handler.NewTreeGrowthManager(world, players)
	fishingMgr := handler.NewFishingManager(players, itemEntities, world, logger)
	enchantMgr := handler.NewEnchantManager(world)
	anvilMgr := handler.NewAnvilManager(world)
	effectMgr := handler.NewEffectManager(players, survHandler, logger)
	potionMgr := handler.NewPotionManager(players, effectMgr, survHandler, logger)
	fireMgr := handler.NewFireManager(players, world, survHandler, logger)

	survHandler.ItemEntities = itemEntities
	survHandler.Rules = gameRules
	survHandler.EffectMgr = effectMgr

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
	tridentMgr := &handler.TridentManager{
		Manager:    players,
		ArrowMgr:   arrowMgr,
		Survival:   survHandler,
		WeatherMgr: weatherMgr,
		Logger:     logger,
	}
	elytraMgr := &handler.ElytraManager{
		Manager:  players,
		Survival: survHandler,
		Logger:   logger,
	}

	gp := &regionGamePlay{
		serverID:        serverID,
		clusterCfg:      cfg,
		redis:           rdb,
		logger:          logger,
		world:           world,
		players:         players,
		sections:        sections,
		minY:            minY,
		spawnY:          spawnY,
		isFlat:          isFlat,
		playerStore:     playerStore,
		survivalHandler: survHandler,
		commandGraph:    handler.BuildCommandGraph(),
		chests:          chestMgr,
		itemEntities:    itemEntities,
		furnaces:        furnaceMgr,
		brewingMgr:      brewingMgr,
		timeMgr:         timeMgr,
		mobMgr:          mobMgr,
		arrowMgr:        arrowMgr,
		fluidMgr:        fluidMgr,
		fallingMgr:      fallingMgr,
		treeMgr:         treeMgr,
		cropMgr:         cropMgr,
		weatherMgr:      weatherMgr,
		enchantMgr:      enchantMgr,
		anvilMgr:        anvilMgr,
		bedMgr:          bedMgr,
		bowMgr:          bowMgr,
		crossbowMgr:     crossbowMgr,
		tridentMgr:      tridentMgr,
		elytraMgr:       elytraMgr,
		fishingMgr:      fishingMgr,
		boatMgr:         boatMgr,
		minecartMgr:     minecartMgr,
		effectMgr:       effectMgr,
		potionMgr:       potionMgr,
		tntMgr:          tntMgr,
		fireMgr:         fireMgr,
		redstoneMgr:     redstoneMgr,
	}

	if pg != nil {
		mobMgr.LoadSavedMobs("overworld")
	}

	srv := server.Server{
		Logger:          logger,
		ListPingHandler: &pingHandler{players: players},
		LoginHandler:    &server.InternalLoginHandler{Threshold: 256},
		ConfigHandler: &server.Configurations{
			Registries: regs,
			KnownPacks: []server.KnownPack{
				{Namespace: "minecraft", ID: "core", Version: server.ProtocolName},
			},
			KnownPackEntries: vanilla.RegistryKeys(),
			Tags:             vanilla.ConfigTags(),
			Logger:           logger,
		},
		GamePlay: gp,
	}

	// Ghost entity manager for border sync (initialized below if Redis available)
	var ghostMgr *ghostEntityManager

	// Start tick loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	foodHandler := &handler.FoodHandler{Logger: logger, FishingMgr: fishingMgr, PotionMgr: potionMgr}
	gp.foodHandler = foodHandler
	tickLoop := game.NewTickLoop(
		game.TickHandlerFunc(func(tick int64) {
			gp.keepalive.Tick(tick, players)
			survHandler.HungerTick(players, tick)
			survHandler.VoidDamageTick(players, minY)
			foodHandler.Tick(players)
			itemEntities.Tick(tick)
			furnaceMgr.Tick()
			brewingMgr.Tick()
			timeMgr.Tick(tick, players)
			mobMgr.Tick(tick)
			arrowMgr.Tick(tick)
			fishingMgr.Tick(tick)
			fluidMgr.Tick(tick)
			fallingMgr.Tick(tick)
			treeMgr.Tick(tick)
			cropMgr.Tick(tick)
			weatherMgr.Tick(tick)
			bedMgr.Tick(tick)
			boatMgr.Tick(tick)
			minecartMgr.Tick(tick)
			effectMgr.Tick(tick)
			potionMgr.Tick(tick)
			tntMgr.Tick(tick)
			fireMgr.Tick(tick)
			redstoneMgr.Tick(tick)
			tridentMgr.Tick(tick)
			elytraMgr.Tick(tick)
			if rdb != nil {
				publishBorderEntities(tick, serverID, cfg, players, rdb)
			}
			if ghostMgr != nil && tick%100 == 0 {
				ghostMgr.cleanup()
			}
		}),
	)

	if dbw != nil {
		tickLoop.AddHandler(dbw.FlushTick(600))
	}

	go tickLoop.Run(ctx)

	// Subscribe to Redis channels if available
	if rdb != nil {
		go subscribeChatChannel(rdb, serverID, players, logger)

		ghostMgr = newGhostEntityManager(players, logger)
		go subscribeBorderChannel(rdb, serverID, ghostMgr, logger)
		logger.Printf("Border entity sync enabled for channel %s", cluster.BorderChannel(serverID))
	}
	gp.ghostMgr = ghostMgr

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Printf("Shutting down region server %s...", serverID)
		logger.Println("Shutdown: saving mobs...")
		mobMgr.SaveAllMobs("overworld")

		if playerStore != nil {
			logger.Println("Shutdown: saving player states...")
			players.ForEach(func(p *game.Player) {
				sctx, scancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := playerStore.SavePlayer(sctx, buildPlayerState(p))
				scancel()
				if err != nil {
					logger.Printf("Shutdown: failed to save player %s: %v", p.Name, err)
				} else {
					logger.Printf("Shutdown: saved player %s", p.Name)
				}
			})
		}

		if dbw != nil {
			logger.Println("Shutdown: flushing dirty chunks...")
			n, err := dbw.FlushDirty(context.Background())
			if err != nil {
				logger.Printf("Shutdown: flush error: %v", err)
			} else if n > 0 {
				logger.Printf("Shutdown: flushed %d dirty chunks", n)
			}
		}

		if rdb != nil {
			logger.Println("Shutdown: cleaning Redis routing keys...")
			players.ForEach(func(p *game.Player) {
				rdb.Del(context.Background(), cluster.PlayerKey(p.UUID))
			})
		}

		logger.Printf("Shutdown complete for %s", serverID)
		cancel()
		os.Exit(0)
	}()

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":25566"
	}
	logger.Printf("Region server %s listening on %s", serverID, addr)
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

func (p *pingHandler) Name() string                            { return server.ProtocolName }
func (p *pingHandler) Protocol(int32) int                      { return server.ProtocolVersion }
func (p *pingHandler) MaxPlayer() int                          { return 20 }
func (p *pingHandler) OnlinePlayer() int                       { return p.players.Count() }
func (p *pingHandler) PlayerSamples() []server.PlayerSample    { return nil }
func (p *pingHandler) Description() *chat.Message {
	msg := chat.Text("Region Server")
	return &msg
}
func (p *pingHandler) FavIcon() string { return "" }

// ---------------------------------------------------------------------------
// GamePlay
// ---------------------------------------------------------------------------

type regionGamePlay struct {
	serverID    string
	clusterCfg  *cluster.ClusterConfig
	redis       *redis.Client
	logger      *log.Logger
	world       game.World
	players     *game.PlayerManager
	sections    int
	minY        int
	spawnY      float64
	isFlat      bool
	playerStore store.PlayerStore

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
	fluidMgr        *handler.FluidManager
	fallingMgr      *handler.FallingBlockManager
	treeMgr         *handler.TreeGrowthManager
	cropMgr         *handler.CropManager
	weatherMgr      *handler.WeatherManager
	enchantMgr      *handler.EnchantManager
	anvilMgr        *handler.AnvilManager
	bedMgr          *handler.BedManager
	bowMgr          *handler.BowManager
	crossbowMgr     *handler.CrossbowManager
	tridentMgr      *handler.TridentManager
	elytraMgr       *handler.ElytraManager
	fishingMgr      *handler.FishingManager
	signMgr         *handler.SignManager
	boatMgr         *handler.BoatManager
	minecartMgr     *handler.MinecartManager
	effectMgr       *handler.EffectManager
	potionMgr       *handler.PotionManager
	tntMgr          *handler.TNTManager
	fireMgr         *handler.FireManager
	redstoneMgr     *handler.RedstoneManager
	ghostMgr        *ghostEntityManager
}

func (g *regionGamePlay) logf(format string, args ...any) {
	g.logger.Printf(format, args...)
}

// buildPlayerState creates a store.PlayerState snapshot for saving to the database.
func buildPlayerState(p *game.Player) *store.PlayerState {
	px, py, pz := p.Position()
	pyaw, ppitch := p.Rotation()

	invSlots := make([]store.ItemSlot, len(p.Inventory))
	for i, s := range p.Inventory {
		invSlots[i] = store.ItemSlot{
			ID: s.ID, Count: s.Count,
			Durability: s.Durability, MaxDurability: s.MaxDurability,
			Enchantments: s.Enchantments, DisplayName: s.DisplayName,
			PotionType: s.PotionType,
		}
	}
	enderSlots := make([]store.ItemSlot, len(p.EnderItems))
	for i, s := range p.EnderItems {
		enderSlots[i] = store.ItemSlot{
			ID: s.ID, Count: s.Count,
			Durability: s.Durability, MaxDurability: s.MaxDurability,
			Enchantments: s.Enchantments, DisplayName: s.DisplayName,
			PotionType: s.PotionType,
		}
	}
	var effects []store.EffectData
	for _, e := range p.Effects {
		if e != nil {
			effects = append(effects, store.EffectData{
				ID: e.ID, Level: e.Level, Duration: e.Duration, Ambient: e.Ambient,
			})
		}
	}

	return &store.PlayerState{
		UUID: p.UUID, Name: p.Name, Dimension: p.Dimension,
		X: px, Y: py, Z: pz, Yaw: pyaw, Pitch: ppitch,
		GameMode: int(p.GameMode), Health: p.Health,
		Food: p.Food, Saturation: p.Saturation, Exhaustion: p.Exhaustion,
		Inventory: invSlots, EnderChest: enderSlots, Effects: effects,
		Experience: p.Experience, ExperienceLevel: p.ExperienceLevel,
		ExperienceTotal: p.ExperienceTotal,
		SpawnX: p.SpawnX, SpawnY: p.SpawnY, SpawnZ: p.SpawnZ,
		HasSpawnPoint: p.HasSpawnPoint,
	}
}

func (g *regionGamePlay) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	eid := g.players.NextEntityID()
	player := game.NewPlayer(name, id, eid, conn)
	player.Properties = properties

	spawnX, spawnYVal, spawnZ := 0.5, g.spawnY, 0.5
	var spawnYaw, spawnPitch float32

	// Load saved position
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
			player.HasSpawnPoint = ps.HasSpawnPoint
			player.SpawnX = ps.SpawnX
			player.SpawnY = ps.SpawnY
			player.SpawnZ = ps.SpawnZ
			for i, slot := range ps.Inventory {
				if i >= len(player.Inventory) {
					break
				}
				player.Inventory[i] = game.ItemStack{
					ID:            slot.ID,
					Count:         slot.Count,
					Durability:    slot.Durability,
					MaxDurability: slot.MaxDurability,
					Enchantments:  slot.Enchantments,
					DisplayName:   slot.DisplayName,
					PotionType:    slot.PotionType,
				}
			}
			for i, slot := range ps.EnderChest {
				if i >= len(player.EnderItems) {
					break
				}
				player.EnderItems[i] = game.ItemStack{
					ID:            slot.ID,
					Count:         slot.Count,
					Durability:    slot.Durability,
					MaxDurability: slot.MaxDurability,
					Enchantments:  slot.Enchantments,
					DisplayName:   slot.DisplayName,
					PotionType:    slot.PotionType,
				}
			}
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
		}
	}

	player.SetPosition(spawnX, spawnYVal, spawnZ)
	player.SetRotation(spawnYaw, spawnPitch)
	player.ViewDistance = 10

	g.players.Add(player)
	g.logf("Player %s (%s) joined [eid=%d]", name, id, eid)

	defer func() {
		g.fishingMgr.CleanupPlayer(player)
		if player.Sleeping && g.bedMgr != nil {
			g.bedMgr.WakePlayer(player)
		}
		if player.RidingEntityEID != 0 {
			if g.minecartMgr != nil && g.minecartMgr.IsMinecart(player.RidingEntityEID) {
				g.minecartMgr.DismountMinecart(player)
			} else if g.boatMgr != nil {
				g.boatMgr.DismountBoat(player)
			}
		}
		handler.RemovePlayerScore(g.players, player.Name)
		handler.BroadcastPlayerLeave(g.players, player)

		if g.playerStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := g.playerStore.SavePlayer(ctx, buildPlayerState(player))
			cancel()
			if err != nil {
				g.logf("Warning: failed to save player %s state: %v", name, err)
			}
		}

		g.players.Remove(id)
		g.logf("Player %s (%s) left", name, id)
	}()

	// JoinGame
	if err := g.sendJoinGame(conn, eid); err != nil {
		g.logf("Error sending JoinGame to %s: %v", name, err)
		return
	}

	// PlayerAbilities
	conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerAbilities,
		pk.Byte(0x00),
		pk.Float(0.05),
		pk.Float(0.1),
	))

	// PlayerPosition
	conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(1),
		pk.Double(spawnX), pk.Double(spawnYVal), pk.Double(spawnZ),
		pk.Double(0), pk.Double(0), pk.Double(0),
		pk.Float(spawnYaw), pk.Float(spawnPitch),
		pk.Int(0),
	))

	if err := g.sendSpawnSequence(player); err != nil {
		g.logf("Error sending spawn sequence to %s: %v", name, err)
		return
	}

	handler.SendFullInventory(player)

	conn.WritePacket(pk.Marshal(
		packetid.ClientboundSetHealth,
		pk.Float(player.Health),
		pk.VarInt(player.Food),
		pk.Float(player.Saturation),
	))

	handler.SendExperience(player)
	g.timeMgr.SendTime(player)
	if g.weatherMgr != nil {
		g.weatherMgr.SendWeather(player)
	}

	conn.WritePacket(pk.Marshal(
		packetid.ClientboundServerData,
		chat.Text(""),
		pk.Boolean(false),
	))
	conn.WritePacket(pk.Marshal(
		packetid.ClientboundCustomPayload,
		pk.Identifier("minecraft:brand"),
		pk.String("gearworks-region"),
	))
	conn.WritePacket(pk.Marshal(
		packetid.ClientboundCommands, g.commandGraph,
	))

	handler.SendPlayerInfo(player, player)
	handler.BroadcastPlayerJoin(g.players, player)
	handler.SendExistingPlayers(g.players, player)
	g.itemEntities.SendExistingItems(player)
	g.mobMgr.SendExistingMobs(player)
	g.arrowMgr.SendExistingArrows(player)
	g.fishingMgr.SendExistingBobbers(player)
	g.boatMgr.SendExistingBoats(player)
	g.minecartMgr.SendExistingMinecarts(player)
	g.tntMgr.SendExistingTNTs(player)
	g.potionMgr.SendExistingPotions(player)
	handler.SendScoreboard(g.players, player)

	g.packetLoop(player)
}

func (g *regionGamePlay) sendJoinGame(conn *net.Conn, eid int32) error {
	dimensionNames := []pk.Identifier{"minecraft:overworld", "minecraft:the_nether", "minecraft:the_end"}
	return conn.WritePacket(pk.Marshal(
		packetid.ClientboundLogin,
		pk.Int(eid),
		pk.Boolean(false),
		pk.Array(dimensionNames),
		pk.VarInt(20),
		pk.VarInt(10),
		pk.VarInt(10),
		pk.Boolean(false),
		pk.Boolean(true),
		pk.Boolean(false),
		pk.VarInt(0),
		pk.Identifier("minecraft:overworld"),
		pk.Long(0),
		pk.UnsignedByte(0),
		pk.Byte(-1),
		pk.Boolean(false),
		pk.Boolean(g.isFlat),
		pk.Boolean(false),
		pk.VarInt(0),
		pk.VarInt(63),
		pk.Boolean(false),
	))
}

func (g *regionGamePlay) sendSpawnSequence(player *game.Player) error {
	px, py, pz := player.Position()

	err := player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDefaultSpawnPosition,
		pk.Identifier("minecraft:overworld"),
		pk.Position{X: int(px), Y: int(py), Z: int(pz)},
		pk.Float(0), pk.Float(0),
	))
	if err != nil {
		return err
	}

	err = player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), pk.Float(0),
	))
	if err != nil {
		return err
	}

	cs := &handler.ChunkSender{
		World: g.world,
		MinY:  g.minY,
	}
	return cs.SendInitialChunks(player)
}

func (g *regionGamePlay) packetLoop(player *game.Player) {
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
	}

	// Wire up region crossing callback
	if g.redis != nil {
		movHandler.OnRegionCrossing = func(p *game.Player, newRegion game.RegionPos) {
			newServer := g.clusterCfg.ServerForRegion(p.Dimension, newRegion.X, newRegion.Z)
			if newServer != g.serverID {
				g.logf("Player %s crossed to region (%d,%d) → server %s", p.Name, newRegion.X, newRegion.Z, newServer)
				// Publish transfer signal to Redis
				state := cluster.PlayerTransferState{
					Name:       p.Name,
					UUID:       p.UUID,
					Dimension:  p.Dimension,
					X:          p.X,
					Y:          p.Y,
					Z:          p.Z,
					FromServer: g.serverID,
					ToServer:   newServer,
				}
				data, _ := json.Marshal(state)
				g.redis.Publish(context.Background(), cluster.TransferChannel(p.UUID), string(data))
			}
		}
	}

	blockHandler := &handler.BlockHandler{
		World:        g.world,
		Manager:      g.players,
		Logger:       g.logger,
		Chests:       g.chests,
		ItemEntities: g.itemEntities,
		Furnaces:     g.furnaces,
		BrewingMgr:   g.brewingMgr,
		EnchantMgr:   g.enchantMgr,
		AnvilMgr:     g.anvilMgr,
		BedMgr:       g.bedMgr,
		BoatMgr:      g.boatMgr,
		MinecartMgr:  g.minecartMgr,
		FluidMgr:     g.fluidMgr,
		FallingMgr:   g.fallingMgr,
		TNTMgr:       g.tntMgr,
		FireMgr:      g.fireMgr,
		RedstoneMgr:  g.redstoneMgr,
		TreeMgr:      g.treeMgr,
		CropMgr:      g.cropMgr,
	}

	invHandler := &handler.InventoryHandler{
		Logger:     g.logger,
		Chests:     g.chests,
		Furnaces:   g.furnaces,
		BrewingMgr: g.brewingMgr,
		EnchantMgr: g.enchantMgr,
		AnvilMgr:   g.anvilMgr,
	}

	cmdExecutor := &handler.CommandExecutor{
		Manager:         g.players,
		Logger:          g.logger,
		SurvivalHandler: g.survivalHandler,
		TimeMgr:         g.timeMgr,
		WeatherMgr:      g.weatherMgr,
	}

	chatHandler := &handler.ChatHandler{
		Manager:  g.players,
		Logger:   g.logger,
		Commands: cmdExecutor,
	}
	// Wire Redis chat broadcast if available
	if g.redis != nil {
		chatHandler.Broadcaster = &redisChatBroadcaster{
			serverID: g.serverID,
			manager:  g.players,
			rdb:      g.redis,
		}
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
		BoatMgr:         g.boatMgr,
		MinecartMgr:     g.minecartMgr,
		EffectMgr:       g.effectMgr,
		Logger:          g.logger,
	}
	respawnHandler := &handler.RespawnHandler{
		Manager: g.players,
		World:   g.world,
		SpawnY:  g.spawnY,
		MinY:    g.minY,
		Logger:  g.logger,
	}

	// Keepalive sender goroutine
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
			break
		}

		if movHandler.HandlePacket(player, p) {
			continue
		}
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
		if g.elytraMgr.HandleUseItem(player, p) {
			continue
		}
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

		switch packetid.ServerboundPacketID(p.ID) {
		case packetid.ServerboundAcceptTeleportation:
		case packetid.ServerboundChunkBatchReceived:
		default:
		}
	}
}

// ---------------------------------------------------------------------------
// Redis Chat
// ---------------------------------------------------------------------------

func subscribeChatChannel(rdb *redis.Client, ownServerID string, players *game.PlayerManager, logger *log.Logger) {
	ctx := context.Background()
	sub := rdb.Subscribe(ctx, cluster.ChatChannel)
	defer sub.Close()

	ch := sub.Channel()
	for msg := range ch {
		var data struct {
			ServerID string `json:"server_id"`
			Name     string `json:"name"`
			Message  string `json:"message"`
		}
		if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
			continue
		}
		// Skip messages originating from this server to avoid self-echo.
		if data.ServerID == ownServerID {
			continue
		}

		chatMsg := chat.Text(fmt.Sprintf("<%s> %s", data.Name, data.Message))
		pkt := pk.Marshal(
			packetid.ClientboundDisguisedChat,
			chatMsg,
			pk.VarInt(0),
			chat.Text(data.Name),
			pk.Boolean(false),
		)
		players.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}
}

// ---------------------------------------------------------------------------
// Registries (same as server261)
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

	return regs
}

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

// redisChatBroadcaster implements handler.ChatBroadcaster by publishing to Redis
// and also sending to local players.
type redisChatBroadcaster struct {
	serverID string
	manager  *game.PlayerManager
	rdb      *redis.Client
}

func (b *redisChatBroadcaster) BroadcastChat(sender *game.Player, message string) {
	// Local broadcast
	pkt := pk.Marshal(
		packetid.ClientboundDisguisedChat,
		chat.Text(message),
		&chat.Type{ID: 0, SenderName: chat.Text(sender.Name)},
	)
	b.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Cross-server broadcast via Redis (include server_id so receivers can skip self-echo)
	msg, _ := json.Marshal(map[string]string{"server_id": b.serverID, "name": sender.Name, "message": message})
	b.rdb.Publish(context.Background(), cluster.ChatChannel, string(msg))
}

// ---------------------------------------------------------------------------
// Border Entity Sync — ghost entities from adjacent region servers
// ---------------------------------------------------------------------------

const (
	borderRange     = 64.0  // blocks from region boundary to scan
	borderPublishHz = 10    // publish every N ticks
	ghostTimeout    = 5 * time.Second
)

// ghostEntity is a phantom entity rendered from an adjacent server's border broadcast.
type ghostEntity struct {
	EID      int32
	TypeID   int32
	X, Y, Z  float64
	Yaw      float32
	Pitch    float32
	Name     string
	UUID     uuid.UUID
	LastSeen time.Time
}

// ghostEntityManager tracks phantom entities from other servers.
type ghostEntityManager struct {
	mu       sync.Mutex
	ghosts   map[string]map[int32]*ghostEntity // sourceServer → eid → ghost
	players  *game.PlayerManager
	logger   *log.Logger
}

func newGhostEntityManager(players *game.PlayerManager, logger *log.Logger) *ghostEntityManager {
	return &ghostEntityManager{
		ghosts:  make(map[string]map[int32]*ghostEntity),
		players: players,
		logger:  logger,
	}
}

// applyUpdate processes a border entity update from another server.
func (gm *ghostEntityManager) applyUpdate(update cluster.BorderEntityUpdate) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	serverGhosts, ok := gm.ghosts[update.SourceServer]
	if !ok {
		serverGhosts = make(map[int32]*ghostEntity)
		gm.ghosts[update.SourceServer] = serverGhosts
	}

	// Track which EIDs are in this update
	seen := make(map[int32]bool, len(update.Entities))
	now := time.Now()

	for _, e := range update.Entities {
		seen[e.EID] = true
		existing, exists := serverGhosts[e.EID]
		if exists {
			// Update position — send teleport to nearby players
			existing.X, existing.Y, existing.Z = e.X, e.Y, e.Z
			existing.Yaw = e.Yaw
			existing.Pitch = e.Pitch
			existing.LastSeen = now

			teleportPkt := pk.Marshal(
				packetid.ClientboundTeleportEntity,
				pk.VarInt(e.EID),
				pk.Double(e.X),
				pk.Double(e.Y),
				pk.Double(e.Z),
				pk.UnsignedByte(0), // LpVec3 zero velocity
				pk.Angle(degToAngle(e.Yaw)),
				pk.Angle(degToAngle(e.Pitch)),
				pk.Boolean(true), // onGround
			)
			gm.players.ForEachNearby(e.X, e.Z, handler.PlayerTrackingRange, func(p *game.Player) {
				p.WritePacket(teleportPkt)
			})
		} else {
			// New ghost — spawn for nearby players
			ghost := &ghostEntity{
				EID: e.EID, TypeID: e.TypeID,
				X: e.X, Y: e.Y, Z: e.Z,
				Yaw: e.Yaw, Pitch: e.Pitch,
				Name: e.Name, UUID: e.UUID,
				LastSeen: now,
			}
			serverGhosts[e.EID] = ghost

			spawnPkt := pk.Marshal(
				packetid.ClientboundAddEntity,
				pk.VarInt(e.EID),
				pk.UUID(e.UUID),
				pk.VarInt(e.TypeID),
				pk.Double(e.X),
				pk.Double(e.Y),
				pk.Double(e.Z),
				pk.UnsignedByte(0), // LpVec3 zero velocity
				pk.Angle(degToAngle(e.Pitch)),
				pk.Angle(degToAngle(e.Yaw)),
				pk.Angle(degToAngle(e.Yaw)), // head yaw
				pk.VarInt(0),
			)
			gm.players.ForEachNearby(e.X, e.Z, handler.PlayerTrackingRange, func(p *game.Player) {
				p.WritePacket(spawnPkt)
			})
		}
	}

	// Remove ghosts not in this update (they left the border)
	for eid, ghost := range serverGhosts {
		if !seen[eid] {
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(eid),
			)
			gm.players.ForEachNearby(ghost.X, ghost.Z, handler.PlayerTrackingRange, func(p *game.Player) {
				p.WritePacket(removePkt)
			})
			delete(serverGhosts, eid)
		}
	}
}

// cleanup removes stale ghosts that haven't been updated recently.
func (gm *ghostEntityManager) cleanup() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	cutoff := time.Now().Add(-ghostTimeout)
	for srv, ghosts := range gm.ghosts {
		for eid, ghost := range ghosts {
			if ghost.LastSeen.Before(cutoff) {
				removePkt := pk.Marshal(
					packetid.ClientboundRemoveEntities,
					pk.VarInt(1),
					pk.VarInt(eid),
				)
				gm.players.ForEachNearby(ghost.X, ghost.Z, handler.PlayerTrackingRange, func(p *game.Player) {
					p.WritePacket(removePkt)
				})
				delete(ghosts, eid)
			}
		}
		if len(ghosts) == 0 {
			delete(gm.ghosts, srv)
		}
	}
}

// publishBorderEntities scans for players near region boundaries and publishes them.
func publishBorderEntities(tick int64, serverID string, cfg *cluster.ClusterConfig, players *game.PlayerManager, rdb *redis.Client) {
	if tick%borderPublishHz != 0 {
		return
	}

	// Gather entities near boundaries destined for each adjacent server
	updates := make(map[string][]cluster.BorderEntity) // targetServerID → entities

	players.ForEach(func(p *game.Player) {
		px, py, pz := p.Position()
		yaw, pitch := p.Rotation()

		// Check all 4 cardinal region boundaries
		bx := int(px)
		bz := int(pz)
		regionX := blockToRegionCoord(bx)
		regionZ := blockToRegionCoord(bz)

		// Region boundaries in block coords
		minRX := regionX * 512
		maxRX := minRX + 512
		minRZ := regionZ * 512
		maxRZ := minRZ + 512

		type adj struct{ rx, rz int }
		var adjacents []adj

		if float64(bx)-float64(minRX) < borderRange {
			adjacents = append(adjacents, adj{regionX - 1, regionZ})
		}
		if float64(maxRX)-float64(bx) < borderRange {
			adjacents = append(adjacents, adj{regionX + 1, regionZ})
		}
		if float64(bz)-float64(minRZ) < borderRange {
			adjacents = append(adjacents, adj{regionX, regionZ - 1})
		}
		if float64(maxRZ)-float64(bz) < borderRange {
			adjacents = append(adjacents, adj{regionX, regionZ + 1})
		}

		for _, a := range adjacents {
			targetSrv := cfg.ServerForRegion(p.Dimension, a.rx, a.rz)
			if targetSrv != serverID {
				updates[targetSrv] = append(updates[targetSrv], cluster.BorderEntity{
					EID:    p.EID,
					TypeID: 155, // Player
					X:      px,
					Y:      py,
					Z:      pz,
					Yaw:    yaw,
					Pitch:  pitch,
					Name:   p.Name,
					UUID:   p.UUID,
				})
			}
		}
	})

	ctx := context.Background()
	for targetSrv, entities := range updates {
		update := cluster.BorderEntityUpdate{
			SourceServer: serverID,
			Entities:     entities,
		}
		data, _ := json.Marshal(update)
		rdb.Publish(ctx, cluster.BorderChannel(targetSrv), string(data))
	}
}

// subscribeBorderChannel listens for border entity updates from adjacent servers.
func subscribeBorderChannel(rdb *redis.Client, serverID string, ghostMgr *ghostEntityManager, logger *log.Logger) {
	ctx := context.Background()
	sub := rdb.Subscribe(ctx, cluster.BorderChannel(serverID))
	defer sub.Close()

	ch := sub.Channel()
	for msg := range ch {
		var update cluster.BorderEntityUpdate
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			logger.Printf("border: invalid message: %v", err)
			continue
		}
		ghostMgr.applyUpdate(update)
	}
}

func blockToRegionCoord(coord int) int {
	if coord < 0 {
		return (coord - 511) / 512
	}
	return coord / 512
}

func degToAngle(deg float32) int8 {
	return int8(int32(deg*256.0/360.0) & 0xFF)
}

// Suppress unused import warnings
var _ = rand.Intn
