package main

import (
	"log"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler"
	"github.com/Tnze/go-mc/game/pgstore"
	"github.com/Tnze/go-mc/game/store"

	"go.opentelemetry.io/otel/trace"
)

// managersResult holds all managers created by createManagers that are needed
// outside of the gamePlay struct (e.g. for the tick loop or server setup).
type managersResult struct {
	gp              *gamePlay
	survHandler     *handler.SurvivalHandler
	gameRules       *handler.GameRules
	spawnerMgr      *handler.SpawnerManager
	lightningMgr    *handler.LightningManager
	collisionMgr    *handler.CollisionManager
	villageMgr      *handler.VillageManager
	raidMgr         *handler.RaidManager
	sculkMgr        *handler.SculkManager
	turtleMgr       *handler.TurtleManager
	conduitMgr      *handler.ConduitManager
	advancementMgr  *handler.AdvancementManager
}

// createManagers instantiates all gameplay managers and wires their
// cross-dependencies. It returns a managersResult containing the gamePlay
// struct and tick-loop managers that are not stored on gamePlay.
func createManagers(
	logger *log.Logger,
	players *game.PlayerManager,
	world game.World,
	sections, minY int,
	spawnY float64,
	isFlat bool,
	playerStore store.PlayerStore,
	pg *pgstore.PGStore,
	tracer trace.Tracer,
) managersResult {

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
	loomMgr := handler.NewLoomManager()
	spawnerMgr := handler.NewSpawnerManager(mobMgr, players, world)
	enchantMgr := handler.NewEnchantManager(world)
	anvilMgr := handler.NewAnvilManager(world)
	villagerMgr := handler.NewVillagerManager(mobMgr, players)
	villageMgr := handler.NewVillageManager(mobMgr, players, world, logger)
	fluidMgr := handler.NewFluidManager(world, players)
	fallingMgr := handler.NewFallingBlockManager(world, players)
	blockUpdateMgr := handler.NewBlockUpdateManager(world, players)
	blockUpdateMgr.FallingMgr = fallingMgr
	blockUpdateMgr.FluidMgr = fluidMgr
	blockUpdateMgr.ItemEntities = itemEntities
	treeMgr := handler.NewTreeGrowthManager(world, players)
	cropMgr := handler.NewCropManager(world, players)
	weatherMgr := handler.NewWeatherManager(players)
	weatherMgr.Rules = gameRules
	mobMgr.WeatherMgr = weatherMgr
	mobMgr.Rules = gameRules

	hiveMgr := handler.NewHiveManager(mobMgr, players, itemEntities, survHandler, world, logger)
	mobMgr.HiveMgr = hiveMgr

	collisionMgr := &handler.CollisionManager{
		Manager:    players,
		MobManager: mobMgr,
	}

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
	crafterMgr := handler.NewCrafterManager(players, world, itemEntities)

	// Wire redstone sub-managers
	redstoneMgr.WireMgr = wireMgr
	redstoneMgr.PistonMgr = pistonMgr
	redstoneMgr.DispenserMgr = dispenserMgr
	redstoneMgr.CrafterMgr = crafterMgr
	redstoneMgr.TNTMgr = tntMgr
	redstoneMgr.TimeMgr = timeMgr
	hopperMgr.Chests = chestMgr
	hopperMgr.Furnaces = furnaceMgr
	hopperMgr.CrafterMgr = crafterMgr
	dispenserMgr.ArrowMgr = arrowMgr
	hopperMgr.WireMgr = wireMgr
	hopperMgr.RedstoneMgr = redstoneMgr
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
	bookMgr := handler.NewBookManager()
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
	sculkMgr := handler.NewSculkManager(players, world)
	sculkMgr.MobMgr = mobMgr
	sculkMgr.EffectMgr = effectMgr
	mobMgr.SculkMgr = sculkMgr
	turtleMgr := handler.NewTurtleManager(mobMgr, players, world, logger)
	mobMgr.TurtleMgr = turtleMgr
	conduitMgr := handler.NewConduitManager(players, world, effectMgr, mobMgr, logger)
	raidMgr := handler.NewRaidManager(players, mobMgr, world, effectMgr, logger)

	survHandler.ItemEntities = itemEntities
	survHandler.Rules = gameRules
	survHandler.EffectMgr = effectMgr

	permMgr := handler.NewPermissionManager("ops.json", "whitelist.json")

	banMgr, err := handler.LoadBans("banned-players.json")
	if err != nil {
		logger.Printf("Warning: could not load bans: %v", err)
		banMgr = nil
	}

	scoreboardMgr := handler.NewScoreboardManager(players)
	worldBorderMgr := handler.NewWorldBorderManager(players)
	worldBorderMgr.SurvivalHandler = survHandler
	worldBorderMgr.Logger = logger

	copperMgr := &handler.CopperManager{
		Manager: players,
		World:   world,
	}

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
		blockUpdateMgr:  blockUpdateMgr,
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
		crafterMgr:      crafterMgr,
		barrelMgr:       barrelMgr,
		grindstoneMgr:   grindstoneMgr,
		stonecutterMgr:  stonecutterMgr,
		smokerMgr:       smokerMgr,
		blastFurnaceMgr: blastFurnaceMgr,
		shulkerBoxMgr:   shulkerBoxMgr,
		smithingMgr:     smithingMgr,
		loomMgr:         loomMgr,
		jukeboxMgr:      jukeboxMgr,
		lecternMgr:      lecternMgr,
		bookMgr:         bookMgr,
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
		copperMgr:        copperMgr,
		advancementMgr:   advancementMgr,
		permMgr:          permMgr,
		banMgr:           banMgr,
		scoreboardMgr:    scoreboardMgr,
		worldBorderMgr:  worldBorderMgr,
		gameRules:        gameRules,
		tracer:           tracer,
	}

	return managersResult{
		gp:             gp,
		survHandler:    survHandler,
		gameRules:      gameRules,
		spawnerMgr:     spawnerMgr,
		lightningMgr:   lightningMgr,
		collisionMgr:   collisionMgr,
		villageMgr:     villageMgr,
		raidMgr:        raidMgr,
		sculkMgr:       sculkMgr,
		turtleMgr:      turtleMgr,
		conduitMgr:     conduitMgr,
		advancementMgr: advancementMgr,
	}
}
