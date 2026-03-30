package handler

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Mob represents a mob entity (hostile or passive).
type Mob struct {
	EID            int32
	TypeID         int32
	X, Y, Z        float64
	Yaw, Pitch     float32
	Health         float32
	MaxHealth      float32
	Damage         float32
	Speed          float64
	Target         *game.Player
	AttackCooldown int64
	WanderTick     int64
	WanderYaw      float32
	Hostile        bool

	// Previous position/yaw for relative movement packets
	PrevX, PrevY, PrevZ float64
	PrevYaw             float32

	// Creeper fuse
	FuseStart    int64 // tick when fuse started (0 = not fusing)
	FuseDuration int64 // ticks until explosion (30 = 1.5s)

	// Skeleton ranged attack
	ShootCooldown    int64
	StrafeDirection  int8  // -1 = left, 0 = none, 1 = right
	StrafeChangeTick int64 // tick when strafe direction was last changed

	// Passive mob AI
	FleeX, FleeZ float64 // flee target position
	FleeTicks     int64   // ticks remaining to flee
	LoveTicks     int64   // ticks remaining in "love" mode (breeding)
	BreedCooldown int64   // ticks until can breed again
	Baby          bool    // true if this is a baby mob
	BabyAge       int64   // ticks remaining until growth to adult (24000 = 20 min)

	// Sheep wool color (0=white, 1=orange, ..., 15=black). Used for drop color.
	WoolColor int32

	// Enderman
	TeleportCooldown int64

	// Slime
	SlimeSize int32 // 1=small, 2=medium, 4=large

	// Phantom
	FlyTargetY float64
	SwoopPhase int32  // 0=circling, 1=diving
	SwoopTick  int64  // tick when current phase started
	CircleAngle float64 // current angle around target for circling

	// Velocity-based gravity
	VelY float64

	// Pathfinding
	Path          []PathStep // computed A* path
	PathIndex     int        // current step along path
	PathRecalcTick int64     // tick when path was last recalculated

	// Bee fields
	BeeHivePos    *[3]int // home hive, nil if homeless
	BeeFlowerPos  *[3]int // current flower target
	BeePollinated bool    // carrying pollen
	BeePollTimer  int64   // ticks spent at flower
	BeeAngryTicks int64   // angry duration remaining
	InsideHive    bool    // true when bee is inside a hive (hidden from world)
	HiveExitTick  int64   // tick when bee should leave the hive

	// Patrol captain flag (pillagers)
	IsPatrolCaptain bool

	// Villager data (nil for non-villagers)
	VillagerData *VillagerData

	// Tameable mob data (nil for non-tameable)
	TameData *TameableMobData

	// Custom name from nametag. Named mobs never naturally despawn.
	CustomName string

	// Fire ticks remaining (0 = not on fire). Decrements each tick, deals 1 damage every 20 ticks.
	FireTicks int32

	// Warden-specific fields
	WardenAnger     int32          // anger level: 0-39 idle, 40-79 alert, 80+ enraged
	WardenTarget    int32          // entity ID of highest-anger source
	WardenSniffTick int64          // last tick vibration scan ran
	WardenRoarTick  int64          // last tick sonic boom fired
	WardenLastHeartbeat int64      // last tick heartbeat sound played
	WardenPlayerAnger map[int32]int32 // per-player anger tracking (player EID -> anger)
	WardenPrevPos   map[int32][3]float64 // previous player positions for movement detection

	// Frog fields
	FrogVariant    int32 // 0=temperate, 1=warm, 2=cold
	FrogJumpTick   int64 // next tick to jump
	FrogTongueTick int64 // cooldown for tongue attack

	// Axolotl fields
	AxolotlPlayingDead bool  // true when playing dead
	AxolotlPlayDeadEnd int64 // tick when play-dead ends
	AxolotlVariant     int32 // 0-4 color variants

	// Allay fields
	AllayHeldItem  string // item name the allay is collecting (empty = none)
	AllayDeliverTo *[3]int // position of note block to deliver to (nil = follow player)

	// Turtle fields
	TurtleEggCooldown int64 // ticks until can lay eggs again

	// Sniffer fields
	SnifferSniffTick int64 // next tick to sniff
	SnifferDigging   bool  // currently digging
	SnifferDigEnd    int64 // tick when digging finishes

	// Conversion ticks: counts down to 0 when a mob is converting to another type.
	// Zombie→Drowned (in water), Husk→Zombie (in water), Skeleton→Stray (in powder snow).
	// 0 = not converting. Set to 600 (30 seconds) when conversion starts.
	ConversionTicks int

	// Death animation: tick when killed (0 = alive). Entity is removed after 20 ticks.
	DeathTick int64

	// Ambient sound cooldown: ticks until next ambient sound (0 = play now).
	AmbientSoundCooldown int64
}

// MobManager handles mob spawning, AI, and lifecycle.
type MobManager struct {
	Manager      *game.PlayerManager
	TimeMgr      *TimeManager
	WeatherMgr   *WeatherManager
	Rules        *GameRules
	World        game.World
	MinY         int
	Survival     *SurvivalHandler
	ItemEntities *ItemEntityManager
	ArrowMgr     *ArrowManager
	XPOrbMgr     *XPOrbManager
	AdvMgr       *AdvancementManager
	EffectMgr    *EffectManager
	SculkMgr     *SculkManager
	MobStore     store.MobStore
	HiveMgr      *HiveManager
	TurtleMgr    *TurtleManager
	Logger       *log.Logger
	Spatial      *MobSpatialIndex
	mu           sync.Mutex
	Mobs         map[int32]*Mob
	currentTick  int64 // set at the start of each Tick() call
}

// NewMobManager creates a new mob manager.
func NewMobManager(manager *game.PlayerManager, timeMgr *TimeManager, world game.World, minY int, survival *SurvivalHandler, itemEntities *ItemEntityManager) *MobManager {
	return &MobManager{
		Manager:      manager,
		TimeMgr:      timeMgr,
		World:        world,
		MinY:         minY,
		Survival:     survival,
		ItemEntities: itemEntities,
		Mobs:         make(map[int32]*Mob),
		Spatial:      NewMobSpatialIndex(),
	}
}

// MobCount returns the number of tracked mobs.
func (m *MobManager) MobCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Mobs)
}

// IsMobAlive returns true if a mob with the given EID exists and has positive health.
func (m *MobManager) IsMobAlive(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	return ok && mob.Health > 0
}

// Tick processes mob spawning, AI, and despawning.
func (m *MobManager) Tick(tick int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentTick = tick

	// Only spawn mobs if doMobSpawning is enabled
	mobSpawning := m.Rules == nil || m.Rules.GetDoMobSpawning()

	if mobSpawning {
		// Spawn hostile mobs every 100 ticks (5 seconds)
		if tick%100 == 0 {
			m.trySpawn()
			m.trySpawnSlime()
		}

		// Spawn phantoms every 200 ticks (10 seconds)
		if tick%200 == 0 {
			m.trySpawnPhantom(tick)
		}

		// Spawn passive mobs every 600 ticks (30 seconds) — slower since they no longer despawn
		if tick%600 == 0 {
			m.trySpawnPassive()
		}
	}

	// Rebuild spatial index periodically
	if tick%spatialRebuildInterval == 0 {
		m.Spatial.Rebuild(m.Mobs)
	}

	// Collect player positions for entity culling
	var playerPositions [][2]float64
	m.Manager.ForEach(func(p *game.Player) {
		px, _, pz := p.Position()
		playerPositions = append(playerPositions, [2]float64{px, pz})
	})

	// AI tick for each mob, with entity culling
	for _, mob := range m.Mobs {
		// Skip full AI for mobs far from all players
		if len(playerPositions) > 0 && !isNearAnyPlayer(mob.X, mob.Z, playerPositions, entityCullDistance) {
			// Still decrement cooldowns so they don't stall
			if mob.AttackCooldown > 0 {
				mob.AttackCooldown--
			}
			if mob.ShootCooldown > 0 {
				mob.ShootCooldown--
			}
			if mob.FireTicks > 0 {
				mob.FireTicks--
			}
			continue
		}
		m.tickMob(mob, tick)
	}

	// Sweep dead mobs after death animation (20 ticks = 1 second)
	m.sweepDeadMobs(tick)

	// Despawn check every 200 ticks
	if tick%200 == 0 {
		m.despawnFarMobs()
	}

	// Sunlight damage every 20 ticks
	if tick%20 == 0 && !m.TimeMgr.IsNight() {
		m.sunlightDamage()
	}

	// Enderman rain damage every 20 ticks
	if tick%20 == 0 {
		m.endermanRainDamage()
	}
}

// SaveAllMobs serializes all living mobs and persists them.
func (m *MobManager) SaveAllMobs(dimension string) {
	if m.MobStore == nil {
		return
	}
	m.mu.Lock()
	var mobs []store.MobData
	for _, mob := range m.Mobs {
		if mob.Health <= 0 {
			continue
		}
		extra, _ := json.Marshal(mobExtraData{
			SlimeSize:  mob.SlimeSize,
			Hostile:    mob.Hostile,
			Damage:     mob.Damage,
			Speed:      mob.Speed,
			CustomName: mob.CustomName,
		})
		mobs = append(mobs, store.MobData{
			Dimension: dimension,
			TypeID:    mob.TypeID,
			X:         mob.X,
			Y:         mob.Y,
			Z:         mob.Z,
			Yaw:       mob.Yaw,
			Health:    mob.Health,
			MaxHealth: mob.MaxHealth,
			Extra:     extra,
		})
	}
	m.mu.Unlock()

	if err := m.MobStore.SaveMobs(context.Background(), dimension, mobs); err != nil {
		if m.Logger != nil {
			m.Logger.Printf("Failed to save mobs: %v", err)
		}
	} else if m.Logger != nil {
		m.Logger.Printf("Saved %d mobs to database", len(mobs))
	}
}

// LoadSavedMobs loads mobs from persistent storage and spawns them.
func (m *MobManager) LoadSavedMobs(dimension string) {
	if m.MobStore == nil {
		return
	}
	mobs, err := m.MobStore.LoadMobs(context.Background(), dimension)
	if err != nil {
		if m.Logger != nil {
			m.Logger.Printf("Failed to load mobs: %v", err)
		}
		return
	}

	m.mu.Lock()
	for _, md := range mobs {
		var extra mobExtraData
		if len(md.Extra) > 0 {
			json.Unmarshal(md.Extra, &extra)
		}
		eid := m.Manager.NextEntityID()
		mob := &Mob{
			EID:       eid,
			TypeID:    md.TypeID,
			X:         md.X,
			Y:         md.Y,
			Z:         md.Z,
			PrevX:     md.X,
			PrevY:     md.Y,
			PrevZ:     md.Z,
			Yaw:       md.Yaw,
			Health:    md.Health,
			MaxHealth: md.MaxHealth,
			WanderYaw: md.Yaw,
			Hostile:   extra.Hostile,
			Damage:    extra.Damage,
			Speed:      extra.Speed,
			SlimeSize:  extra.SlimeSize,
			CustomName: extra.CustomName,
		}
		if mob.Speed == 0 {
			mob.Speed = 0.1
		}
		m.Mobs[eid] = mob
	}
	m.mu.Unlock()

	// Broadcast spawns to all online players
	m.mu.Lock()
	for _, mob := range m.Mobs {
		m.broadcastSpawn(mob)
	}
	m.mu.Unlock()

	if m.Logger != nil {
		m.Logger.Printf("Loaded %d mobs from database", len(mobs))
	}
}

// NameMob applies a custom name to a mob via nametag. Returns true if the mob was found.
func (m *MobManager) NameMob(eid int32, name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	if !ok || mob.Health <= 0 || mob.DeathTick > 0 {
		return false
	}
	mob.CustomName = name
	m.broadcastCustomName(mob)
	return true
}

// broadcastCustomName sends entity metadata for custom name to all players.
func (m *MobManager) broadcastCustomName(mob *Mob) {
	nameJSON := `{"text":"` + mob.CustomName + `"}`

	// Build metadata bytes: index 2 = Optional Chat (custom name), index 3 = Boolean (visible)
	var meta []byte
	// Index 2, type 7 (Optional Chat), present=true, value=JSON string
	meta = append(meta, 2)
	meta = appendVarInt(meta, 7)       // type: Optional Chat
	meta = append(meta, 1)             // present = true
	meta = appendVarInt(meta, len(nameJSON))
	meta = append(meta, []byte(nameJSON)...)
	// Index 3, type 8 (Boolean), value=true
	meta = append(meta, 3)
	meta = appendVarInt(meta, 8) // type: Boolean
	meta = append(meta, 1)      // true
	// Terminator
	meta = append(meta, 0xFF)

	pkt := pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(mob.EID),
		pk.PluginMessageData(meta),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// tickMob runs AI for a single mob.
func (m *MobManager) tickMob(mob *Mob, tick int64) {
	if mob.Health <= 0 || mob.DeathTick > 0 {
		return
	}

	// Fire tick damage
	if mob.FireTicks > 0 {
		mob.FireTicks--
		if mob.FireTicks%20 == 0 { // 1 damage per second
			mob.Health -= 1.0
			if mob.Health <= 0 {
				mob.Health = 0
				m.killMob(mob, nil)
				return
			}
			// Broadcast hurt animation
			hurtPkt := pk.Marshal(
				packetid.ClientboundHurtAnimation,
				pk.VarInt(mob.EID),
				pk.Float(0),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(hurtPkt)
			})
		}
		if mob.FireTicks == 0 {
			broadcastFireMetadata(m.Manager, mob.EID, false)
		}
	}

	if mob.AttackCooldown > 0 {
		mob.AttackCooldown--
	}

	// Ambient sound tick
	if mob.AmbientSoundCooldown <= 0 {
		// First tick or cooldown expired — set a random cooldown
		mob.AmbientSoundCooldown = 80 + int64(rand.Intn(121))
	} else {
		mob.AmbientSoundCooldown--
		if mob.AmbientSoundCooldown == 0 {
			if soundID := MobAmbientSound(mob.TypeID); soundID >= 0 {
				cat := MobSoundCategory(mob.TypeID)
				BroadcastSound(m.Manager, soundID, cat, mob.X, mob.Y, mob.Z, 1.0, 0.8+rand.Float32()*0.4)
			}
			mob.AmbientSoundCooldown = 80 + int64(rand.Intn(121))
		}
	}

	// Dispatch to specialized AI
	switch {
	case mob.TypeID == MobTypeCreeper:
		m.tickCreeper(mob, tick)
		return
	case mob.TypeID == MobTypeSkeleton:
		if m.tickSkeletonConversion(mob) {
			return
		}
		m.tickSkeleton(mob, tick)
		return
	case mob.TypeID == MobTypeStray:
		m.tickStray(mob, tick)
		return
	case mob.TypeID == MobTypeEnderman:
		m.tickEnderman(mob, tick)
		return
	case mob.TypeID == MobTypeWitch:
		m.tickWitch(mob, tick)
		return
	case mob.TypeID == MobTypeSlime:
		m.tickSlime(mob, tick)
		return
	case mob.TypeID == MobTypeMagmaCube:
		m.tickMagmaCube(mob, tick)
		return
	case mob.TypeID == MobTypePhantom:
		m.tickPhantom(mob, tick)
		return
	case mob.TypeID == MobTypeGhast:
		m.tickGhast(mob, tick)
		return
	case mob.TypeID == MobTypeBlaze:
		m.tickBlaze(mob, tick)
		return
	case mob.TypeID == MobTypeGuardian, mob.TypeID == MobTypeElderGuardian:
		m.tickGuardian(mob, tick)
		return
	case mob.TypeID == MobTypeIronGolem:
		m.tickIronGolem(mob, tick)
		return
	case mob.TypeID == MobTypePiglin:
		m.tickPiglin(mob, tick)
		return
	case mob.TypeID == MobTypeZombifiedPiglin:
		m.tickZombifiedPiglin(mob, tick)
		return
	case mob.TypeID == MobTypeShulker:
		m.tickShulker(mob, tick)
		return
	case mob.TypeID == MobTypePillager:
		m.tickPillager(mob, tick)
		return
	case mob.TypeID == MobTypeVindicator:
		m.tickVindicator(mob, tick)
		return
	case mob.TypeID == MobTypeEvoker:
		m.tickEvoker(mob, tick)
		return
	case mob.TypeID == MobTypeVex:
		m.tickVex(mob, tick)
		return
	case mob.TypeID == MobTypeRavager:
		m.tickRavager(mob, tick)
		return
	case mob.TypeID == MobTypeHoglin:
		m.tickHoglin(mob, tick)
		return
	case mob.TypeID == MobTypeWitherSkeleton:
		m.tickWitherSkeleton(mob, tick)
		return
	case mob.TypeID == MobTypeDrowned:
		m.tickDrowned(mob, tick)
		return
	case mob.TypeID == MobTypeHusk:
		m.tickHusk(mob, tick)
		return
	case mob.TypeID == MobTypeBee:
		m.tickBee(mob, tick)
		return
	case mob.TypeID == MobTypeWolf:
		m.tickWolf(mob, tick)
		return
	case mob.TypeID == MobTypeCat:
		m.tickCat(mob, tick)
		return
	case mob.TypeID == MobTypeHorse:
		m.tickHorse(mob, tick)
		return
	case mob.TypeID == MobTypeParrot:
		m.tickParrot(mob, tick)
		return
	case isLlamaType(mob.TypeID):
		m.tickLlama(mob, tick)
		return
	case mob.TypeID == MobTypeCaveSpider:
		m.tickCaveSpider(mob, tick)
		return
	case mob.TypeID == MobTypeWarden:
		m.tickWarden(mob, tick)
		return
	case mob.TypeID == MobTypeFrog:
		m.tickFrog(mob, tick)
		return
	case mob.TypeID == MobTypeAxolotl:
		m.tickAxolotl(mob, tick)
		return
	case mob.TypeID == MobTypeAllay:
		m.tickAllay(mob, tick)
		return
	case mob.TypeID == MobTypeSniffer:
		m.tickSniffer(mob, tick)
		return
	case mob.TypeID == MobTypeVillager:
		m.tickVillager(mob, tick)
		return
	case mob.TypeID == MobTypeTurtle:
		if m.TurtleMgr != nil {
			m.TurtleMgr.tickTurtle(mob, tick)
		} else {
			m.tickPassive(mob, tick)
		}
		return
	case mob.TypeID == MobTypeBreeze:
		m.tickBreeze(mob, tick)
		return
	case !mob.Hostile:
		m.tickPassive(mob, tick)
		return
	}

	// Zombie water conversion check
	if mob.TypeID == MobTypeZombie {
		if m.tickZombieConversion(mob) {
			return
		}
	}

	// Default hostile AI (zombie, spider)
	m.tickHostile(mob, tick)
}

// isWalkable checks if a mob can move to the given position (feet and head clear, ground below).
func (m *MobManager) isWalkable(x, y, z float64) bool {
	bx, by, bz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	// Feet and head must be non-solid
	for dy := 0; dy <= 1; dy++ {
		state, err := m.World.GetBlock(bx, by+dy, bz)
		if err != nil {
			return false
		}
		if isSolidBlock(state) {
			return false
		}
	}
	// Must have ground below (max 1-block drop)
	ground, err := m.World.GetBlock(bx, by-1, bz)
	if err != nil {
		return false
	}
	if !isSolidBlock(ground) {
		ground2, err := m.World.GetBlock(bx, by-2, bz)
		if err != nil || !isSolidBlock(ground2) {
			return false
		}
	}
	return true
}

// isCliff returns true if there is no solid block within 3 blocks below the given position.
func (m *MobManager) isCliff(x, y, z float64) bool {
	bx, by, bz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	for dy := 1; dy <= 3; dy++ {
		state, err := m.World.GetBlock(bx, by-dy, bz)
		if err != nil {
			continue
		}
		if isSolidBlock(state) {
			return false
		}
	}
	return true
}

// hasLineOfSight checks if there is a clear line between two points (no solid blocks).
// Raycasts every 0.5 blocks, max 64 checks.
func (m *MobManager) hasLineOfSight(fromX, fromY, fromZ float64, toX, toY, toZ float64) bool {
	dx := toX - fromX
	dy := toY - fromY
	dz := toZ - fromZ
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist < 0.5 {
		return true
	}
	steps := int(dist / 0.5)
	if steps > 64 {
		steps = 64
	}
	stepX := dx / float64(steps)
	stepY := dy / float64(steps)
	stepZ := dz / float64(steps)
	cx, cy, cz := fromX, fromY, fromZ
	for i := 0; i < steps; i++ {
		cx += stepX
		cy += stepY
		cz += stepZ
		bx := int(math.Floor(cx))
		by := int(math.Floor(cy))
		bz := int(math.Floor(cz))
		state, err := m.World.GetBlock(bx, by, bz)
		if err != nil {
			continue
		}
		if isSolidBlock(state) {
			return false
		}
	}
	return true
}

// tryMove attempts to move a mob, checking collision. Returns true if moved.
func (m *MobManager) tryMove(mob *Mob, nx, nz float64) bool {
	newX := mob.X + nx
	newZ := mob.Z + nz

	if m.isWalkable(newX, mob.Y, newZ) {
		mob.X = newX
		mob.Z = newZ
		return true
	}
	// Try step-up
	if m.isWalkable(newX, mob.Y+1, newZ) {
		mob.X = newX
		mob.Y += 1
		mob.Z = newZ
		return true
	}
	return false
}

// applyGravity applies velocity-based gravity to a mob.
func (m *MobManager) applyGravity(mob *Mob) {
	bx := int(math.Floor(mob.X))
	by := int(math.Floor(mob.Y))
	bz := int(math.Floor(mob.Z))
	below, err := m.World.GetBlock(bx, by-1, bz)
	if err != nil {
		return
	}
	if isSolidBlock(below) && mob.VelY <= 0 {
		// On ground — snap to block top, reset velocity
		groundY := float64(by)
		if mob.Y < groundY {
			mob.Y = groundY
		}
		mob.VelY = 0
		return
	}
	// In air — apply gravity and drag
	mob.VelY -= 0.08
	mob.VelY *= 0.98
	if mob.VelY < -3.0 {
		mob.VelY = -3.0
	}
	mob.Y += mob.VelY
}

// tickHostile runs standard melee hostile AI (zombie, spider).
func (m *MobManager) tickHostile(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Find nearest player within 32 blocks with line of sight
	var nearest *game.Player
	nearestDist := 32.0
	mobEyeY := mob.Y + 1.5
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist && m.hasLineOfSight(mob.X, mobEyeY, mob.Z, px, py+1.62, pz) {
			nearestDist = d
			nearest = p
		}
	})

	if nearest != nil {
		mob.Target = nearest

		px, _, pz := nearest.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist > 1.5 {
			m.moveWithPathfinding(mob, nearest, tick)
		}

		// Attack if within range
		if nearestDist <= 1.5 && mob.AttackCooldown <= 0 && mob.Damage > 0 {
			mob.AttackCooldown = 30 // 1.5 seconds
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
			// Arm swing animation
			m.broadcastArmSwing(mob)
		}

		m.broadcastMobMove(mob)
	} else {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
	}
}

// tickCreeper runs creeper-specific AI: approach, fuse, explode.
func (m *MobManager) tickCreeper(mob *Mob, tick int64) {
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 32.0
	mobEyeY := mob.Y + 1.5
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist && m.hasLineOfSight(mob.X, mobEyeY, mob.Z, px, py+1.62, pz) {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.FuseStart = 0
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest

	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.5 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	m.broadcastMobMove(mob)

	// Fuse logic
	if nearestDist <= 3.0 {
		if mob.FuseStart == 0 {
			// Start fuse
			mob.FuseStart = tick
			mob.FuseDuration = 30 // 1.5 seconds
			BroadcastSound(m.Manager, SoundCreeperPrime, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}
		// Check if fuse complete
		if tick-mob.FuseStart >= mob.FuseDuration {
			m.creeperExplode(mob)
		}
	} else {
		// Cancel fuse if target moved away
		mob.FuseStart = 0
	}
}

// creeperExplode handles creeper explosion: damage, block destruction, sound.
func (m *MobManager) creeperExplode(mob *Mob) {
	cx, cy, cz := mob.X, mob.Y, mob.Z

	// Kill creeper (no XP)
	m.removeMobEntity(mob)
	m.Spatial.Remove(mob.EID)
	delete(m.Mobs, mob.EID)

	// Explosion sound
	BroadcastSound(m.Manager, SoundExplode, SoundCategoryHostile, cx, cy, cz, 1.0, 1.0)

	// Damage entities within 7 blocks (linear falloff, max 12 damage)
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - cx
		dy := py - cy
		dz := pz - cz
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < 7.0 {
			damage := float32(12.0 * (1.0 - d/7.0))
			if damage > 0 {
				p.LastDamageMessage = p.Name + " was blown up by Creeper"
				m.Survival.ApplyDamage(m.Manager, p, damage, m.Survival.MobDamageTypeID)
			}
		}
	})

	// Damage other mobs within 7 blocks
	for _, other := range m.Mobs {
		if other.Health <= 0 || other.EID == mob.EID {
			continue
		}
		dx := other.X - cx
		dy := other.Y - cy
		dz := other.Z - cz
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < 7.0 {
			damage := float32(12.0 * (1.0 - d/7.0))
			other.Health -= damage
			if other.Health <= 0 {
				other.Health = 0
				m.killMob(other, nil)
			}
		}
	}

	// Destroy blocks within 3-block radius
	radius := 3
	bx, by, bz := int(cx), int(cy), int(cz)
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				if dx*dx+dy*dy+dz*dz > radius*radius {
					continue
				}
				wx, wy, wz := bx+dx, by+dy, bz+dz
				state, err := m.World.GetBlock(wx, wy, wz)
				if err != nil || state == 0 {
					continue
				}
				blockName := BlockNameFromState(int(state))
				if blockName == "bedrock" || blockName == "obsidian" {
					continue
				}
				m.World.SetBlock(wx, wy, wz, 0)
				m.broadcastBlockUpdate(wx, wy, wz, 0)
			}
		}
	}
}

// tickSkeleton runs skeleton ranged AI: keep distance, shoot arrows.
func (m *MobManager) tickSkeleton(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 32.0
	mobEyeY := mob.Y + 1.5
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist && m.hasLineOfSight(mob.X, mobEyeY, mob.Z, px, py+1.62, pz) {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Positioning: back up if too close, approach if too far, strafe at ideal range
	if dist < 5.0 {
		// Back up — direct movement (no pathfinding needed for retreating)
		nx := -dx / dist * mob.Speed
		nz := -dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	} else if dist > 15.0 {
		// Too far — move closer using pathfinding
		m.moveWithPathfinding(mob, nearest, tick)
	} else {
		// Ideal shooting range (5-15 blocks) — strafe laterally
		if mob.StrafeDirection == 0 || tick >= mob.StrafeChangeTick {
			if rand.Intn(2) == 0 {
				mob.StrafeDirection = 1
			} else {
				mob.StrafeDirection = -1
			}
			mob.StrafeChangeTick = tick + 40 + int64(rand.Intn(21))
		}
		perpX := -dz / dist * mob.Speed * 0.6 * float64(mob.StrafeDirection)
		perpZ := dx / dist * mob.Speed * 0.6 * float64(mob.StrafeDirection)
		m.tryMove(mob, perpX, perpZ)
	}
	m.broadcastMobMove(mob)

	// Shoot if within 15 blocks and cooldown ready
	if nearestDist <= 15.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 40 // 2 seconds
		_, tpy, _ := nearest.Position()
		m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.5, mob.Z, px, tpy+1.0, pz, 3.0)
		BroadcastSound(m.Manager, SoundSkeletonShoot, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// IsPassiveMob returns true if the entity ID belongs to a passive mob.
func (m *MobManager) IsPassiveMob(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	if !ok {
		return false
	}
	// Tameable mobs are handled separately via TryTame
	if isTameableType(mob.TypeID) {
		return false
	}
	return !mob.Hostile
}

// GetMobType returns the mob type ID for the given entity, or -1 if not found.
func (m *MobManager) GetMobType(eid int32) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	if !ok {
		return -1
	}
	return mob.TypeID
}

// tickWander makes a mob wander randomly with cliff avoidance.
func (m *MobManager) tickWander(mob *Mob, tick int64) {
	if tick-mob.WanderTick > int64(40+rand.Intn(40)) {
		mob.WanderTick = tick
		mob.WanderYaw += (rand.Float32() - 0.5) * 90
	}
	rad := float64(mob.WanderYaw) * math.Pi / 180
	nx := math.Cos(rad) * mob.Speed * 0.3
	nz := math.Sin(rad) * mob.Speed * 0.3

	// Cliff avoidance: don't walk off edges while wandering
	destX := mob.X + nx
	destZ := mob.Z + nz
	if m.isCliff(destX, mob.Y, destZ) {
		mob.WanderYaw += (rand.Float32()-0.5)*180 + 90
		mob.Yaw = mob.WanderYaw
		m.broadcastMobMove(mob)
		return
	}

	if !m.tryMove(mob, nx, nz) {
		// Blocked: pick a new direction
		mob.WanderYaw += (rand.Float32()-0.5)*180 + 90
	}
	mob.Yaw = mob.WanderYaw
	m.broadcastMobMove(mob)
}

// moveWithPathfinding moves a mob toward a target player using A* pathfinding.
// Recalculates path every 20 ticks or when the target has moved significantly.
// Falls back to direct-line movement if no path is found.
func (m *MobManager) moveWithPathfinding(mob *Mob, target *game.Player, tick int64) {
	px, py, pz := target.Position()

	// Sprint: 1.5x speed when target is far away
	speed := mob.Speed
	tdx := px - mob.X
	tdz := pz - mob.Z
	targetDist := math.Sqrt(tdx*tdx + tdz*tdz)
	if targetDist > 16 {
		speed *= 1.5
	}

	// Recalculate path every 20 ticks or if path is empty
	needsRecalc := mob.Path == nil ||
		mob.PathIndex >= len(mob.Path) ||
		tick-mob.PathRecalcTick >= 20

	if needsRecalc {
		sx := int(math.Floor(mob.X))
		sy := int(math.Floor(mob.Y))
		sz := int(math.Floor(mob.Z))
		gx := int(math.Floor(px))
		gy := int(math.Floor(py))
		gz := int(math.Floor(pz))

		mob.Path = m.pathfind(sx, sy, sz, gx, gy, gz)
		mob.PathIndex = 0
		mob.PathRecalcTick = tick
	}

	if mob.Path != nil && mob.PathIndex < len(mob.Path) {
		// Move toward next path step
		step := mob.Path[mob.PathIndex]
		tx := float64(step.X) + 0.5
		tz := float64(step.Z) + 0.5
		dx := tx - mob.X
		dz := tz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist < 0.3 {
			// Reached this step, advance
			mob.PathIndex++
			if mob.PathIndex < len(mob.Path) {
				step = mob.Path[mob.PathIndex]
				tx = float64(step.X) + 0.5
				tz = float64(step.Z) + 0.5
				dx = tx - mob.X
				dz = tz - mob.Z
				dist = math.Sqrt(dx*dx + dz*dz)
			}
		}

		if dist > 0.05 {
			nx := dx / dist * speed
			nz := dz / dist * speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
	} else {
		// Fallback: direct-line movement
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.5 {
			nx := dx / dist * speed
			nz := dz / dist * speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
	}
}

// FindMobNear returns the EID of the first living mob within radius of (x,y,z),
// excluding excludeEID. Returns -1 if none found.
func (m *MobManager) FindMobNear(x, y, z, radius float64, excludeEID int32) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	r2 := radius * radius
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		if eid == excludeEID {
			continue
		}
		dx := mob.X - x
		dy := (mob.Y + 0.9) - y
		dz := mob.Z - z
		if dx*dx+dy*dy+dz*dz < r2 {
			return eid
		}
	}
	return -1
}

// KnockbackMobsNear pushes all mobs within radius away from (cx, cy, cz).
func (m *MobManager) KnockbackMobsNear(cx, cy, cz, radius, strength float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r2 := radius * radius
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		dx := mob.X - cx
		dy := (mob.Y + 0.9) - cy
		dz := mob.Z - cz
		d2 := dx*dx + dy*dy + dz*dz
		if d2 >= r2 || d2 < 0.0001 {
			continue
		}
		d := math.Sqrt(d2)
		scale := (1.0 - d/radius) * strength
		kbX := dx / d * scale
		kbY := 0.4 * scale
		kbZ := dz / d * scale
		pkt := pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(eid),
			pk.Short(int16(kbX*8000)),
			pk.Short(int16(kbY*8000)),
			pk.Short(int16(kbZ*8000)),
		)
		m.Manager.ForEachNearby(mob.X, mob.Z, 64, func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}
}

// IsMobOfType checks if the given EID is a living mob of the specified type.
func (m *MobManager) IsMobOfType(eid int32, typeID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	return ok && mob.TypeID == typeID
}

// GetMobPos returns the position of a mob by EID. Returns (0,0,0) if not found.
func (m *MobManager) GetMobPos(eid int32) (x, y, z float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mob, ok := m.Mobs[eid]
	if !ok {
		return 0, 0, 0
	}
	return mob.X, mob.Y, mob.Z
}

// IsMob returns true if the entity ID belongs to a mob.
func (m *MobManager) IsMob(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Mobs[eid]
	return ok
}

// broadcastMobMove sends smooth relative movement packets for a mob.
// Uses MoveEntityPosRot + RotateHead when deltas fit in int16,
// otherwise falls back to TeleportEntity.
func (m *MobManager) broadcastMobMove(mob *Mob) {
	dx := (mob.X - mob.PrevX) * 4096
	dy := (mob.Y - mob.PrevY) * 4096
	dz := (mob.Z - mob.PrevZ) * 4096

	// Check if deltas fit in int16 range (-32768..32767)
	if dx >= -32768 && dx <= 32767 && dy >= -32768 && dy <= 32767 && dz >= -32768 && dz <= 32767 {
		aYaw := pk.Angle(degToAngle(mob.Yaw))
		aPitch := pk.Angle(degToAngle(mob.Pitch))
		movPkt := pk.Marshal(
			packetid.ClientboundMoveEntityPosRot,
			pk.VarInt(mob.EID),
			pk.Short(int16(dx)), pk.Short(int16(dy)), pk.Short(int16(dz)),
			aYaw, aPitch,
			pk.Boolean(true), // on ground
		)
		headPkt := pk.Marshal(
			packetid.ClientboundRotateHead,
			pk.VarInt(mob.EID),
			aYaw,
		)
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			p.WritePacket(movPkt)
			p.WritePacket(headPkt)
		})
	} else {
		m.broadcastMobTeleport(mob)
	}

	mob.PrevX = mob.X
	mob.PrevY = mob.Y
	mob.PrevZ = mob.Z
	mob.PrevYaw = mob.Yaw
}

// broadcastArmSwing broadcasts an arm swing animation for a mob's melee attack.
func (m *MobManager) broadcastArmSwing(mob *Mob) {
	pkt := pk.Marshal(
		packetid.ClientboundAnimate,
		pk.VarInt(mob.EID),
		pk.UnsignedByte(0), // 0 = swing main arm
	)
	m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// broadcastMobTeleport sends an absolute teleport for a mob.
// Used for enderman teleportation and large knockback jumps.
func (m *MobManager) broadcastMobTeleport(mob *Mob) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(mob.EID),
		pk.Double(mob.X),
		pk.Double(mob.Y),
		pk.Double(mob.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(mob.Yaw),
		pk.Float(mob.Pitch),
		pk.Int(0), // relative flags (all absolute)
		pk.Boolean(true), // on ground
	)
	headPkt := pk.Marshal(
		packetid.ClientboundRotateHead,
		pk.VarInt(mob.EID),
		pk.Angle(degToAngle(mob.Yaw)),
	)
	m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
		p.WritePacket(pkt)
		p.WritePacket(headPkt)
	})

	mob.PrevX = mob.X
	mob.PrevY = mob.Y
	mob.PrevZ = mob.Z
	mob.PrevYaw = mob.Yaw
}

// broadcastBlockUpdate sends ClientboundBlockUpdate to all players.
func (m *MobManager) broadcastBlockUpdate(x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// --- Enderman AI ---

// tickEnderman runs enderman AI: aggro on look, melee attack, teleport.
func (m *MobManager) tickEnderman(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TeleportCooldown > 0 {
		mob.TeleportCooldown--
	}

	// Check if any player is looking at the enderman (aggro on look)
	if mob.Target == nil {
		m.Manager.ForEach(func(p *game.Player) {
			if mob.Target != nil || p.Dead || p.GameMode != 0 {
				return
			}
			px, py, pz := p.Position()
			dx := mob.X - px
			dy := (mob.Y + 1.5) - (py + 1.62) // eye heights
			dz := mob.Z - pz
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist > 64 || dist < 0.5 {
				return
			}

			// Get player look direction
			yaw, pitch := p.Rotation()
			yawRad := float64(-yaw) * math.Pi / 180
			pitchRad := float64(-pitch) * math.Pi / 180
			lookX := math.Sin(yawRad) * math.Cos(pitchRad)
			lookY := math.Sin(pitchRad)
			lookZ := math.Cos(yawRad) * math.Cos(pitchRad)

			// Normalize direction to enderman
			ndx := dx / dist
			ndy := dy / dist
			ndz := dz / dist

			// Dot product = cos(angle)
			dot := lookX*ndx + lookY*ndy + lookZ*ndz
			angle := math.Acos(dot) * 180 / math.Pi
			if angle < 5 {
				mob.Target = p
			}
		})
	}

	if mob.Target != nil && (mob.Target.Dead || mob.Target.GameMode != 0) {
		mob.Target = nil
	}

	if mob.Target != nil {
		px, _, pz := mob.Target.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist > 1.5 {
			nx := dx / dist * mob.Speed * 2 // enderman is fast
			nz := dz / dist * mob.Speed * 2
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}

		// Calculate 3D distance for attack range
		tpx, tpy, tpz := mob.Target.Position()
		ddx := tpx - mob.X
		ddy := tpy - mob.Y
		ddz := tpz - mob.Z
		totalDist := math.Sqrt(ddx*ddx + ddy*ddy + ddz*ddz)

		if totalDist <= 1.5 && mob.AttackCooldown <= 0 && mob.Damage > 0 {
			mob.AttackCooldown = 30
			mob.Target.LastDamageMessage = mob.Target.Name + " was slain by Enderman"
			m.Survival.ApplyDamage(m.Manager, mob.Target, mob.Damage, m.Survival.MobDamageTypeID)
			m.broadcastArmSwing(mob)
		}

		m.broadcastMobMove(mob)
	} else {
		m.tickWander(mob, tick)
	}
}

// endermanTeleport teleports an enderman to a random position within 16 blocks.
func (m *MobManager) endermanTeleport(mob *Mob) {
	for i := 0; i < 10; i++ {
		nx := mob.X + (rand.Float64()-0.5)*32
		nz := mob.Z + (rand.Float64()-0.5)*32
		ny := m.findSurfaceY(int(nx), int(nz))
		if ny < m.MinY {
			continue
		}
		if m.isWalkable(nx, float64(ny), nz) {
			mob.X = nx
			mob.Y = float64(ny)
			mob.Z = nz
			BroadcastSound(m.Manager, SoundEndermanTeleport, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
			m.broadcastMobTeleport(mob)
			return
		}
	}
}

// endermanRainDamage damages endermen in rain.
func (m *MobManager) endermanRainDamage() {
	if m.WeatherMgr == nil {
		return
	}
	state := m.WeatherMgr.State()
	if state == WeatherClear {
		return
	}
	for _, mob := range m.Mobs {
		if mob.Health <= 0 || mob.TypeID != MobTypeEnderman {
			continue
		}
		mob.Health -= 1
		if mob.Health <= 0 {
			mob.Health = 0
			m.killMob(mob, nil)
		} else {
			hurtPkt := pk.Marshal(
				packetid.ClientboundHurtAnimation,
				pk.VarInt(mob.EID),
				pk.Float(0),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(hurtPkt)
			})
			BroadcastSound(m.Manager, SoundEndermanHurt, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
			// Enderman also teleports to escape rain
			if mob.TeleportCooldown <= 0 {
				m.endermanTeleport(mob)
				mob.TeleportCooldown = 20
			}
		}
	}
}

// --- Witch AI ---

// tickWitch runs witch AI: ranged attack, maintain distance.
func (m *MobManager) tickWitch(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 32.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Maintain 8-10 block distance
	if dist < 8.0 {
		// Back up
		nx := -dx / dist * mob.Speed
		nz := -dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	} else if dist > 10.0 {
		// Move closer
		nx := dx / dist * mob.Speed
		nz := dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	}
	m.broadcastMobMove(mob)

	// Attack every 60 ticks when target in range (10 blocks)
	if nearestDist <= 10.0 && mob.ShootCooldown <= 0 {
		mob.ShootCooldown = 60 // 3 seconds
		nearest.LastDamageMessage = nearest.Name + " was killed by Witch"
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		BroadcastSound(m.Manager, SoundWitchAmbient, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// --- Slime AI ---

// tickSlime runs slime AI: bouncing movement toward nearest player.
func (m *MobManager) tickSlime(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Bounce interval depends on size: every 20-40 ticks
	bounceInterval := int64(30)
	if mob.SlimeSize >= 4 {
		bounceInterval = 20
	} else if mob.SlimeSize <= 1 {
		bounceInterval = 40
	}

	if tick%bounceInterval != 0 {
		return // only act on bounce ticks
	}

	var nearest *game.Player
	nearestDist := 16.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest != nil {
		mob.Target = nearest
		px, _, pz := nearest.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.5 {
			speed := 0.1 * float64(mob.SlimeSize)
			nx := dx / dist * speed
			nz := dz / dist * speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}

		// Attack if within range and size > 1 (small slimes don't deal damage)
		if nearestDist <= 1.5 && mob.AttackCooldown <= 0 && mob.Damage > 0 {
			mob.AttackCooldown = 30
			nearest.LastDamageMessage = nearest.Name + " was squished by Slime"
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		}
	} else {
		mob.Target = nil
		// Random hop
		mob.WanderYaw += (rand.Float32() - 0.5) * 90
		rad := float64(mob.WanderYaw) * math.Pi / 180
		speed := 0.08 * float64(mob.SlimeSize)
		nx := math.Cos(rad) * speed
		nz := math.Sin(rad) * speed
		m.tryMove(mob, nx, nz)
	}

	// Play bounce sound
	if mob.SlimeSize <= 1 {
		BroadcastSound(m.Manager, SoundSlimeSquishSmall, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	} else {
		BroadcastSound(m.Manager, SoundSlimeSquish, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}

	m.broadcastMobMove(mob)
}

// --- Phantom AI ---

// tickPhantom runs phantom AI: circling and swooping.
func (m *MobManager) tickPhantom(mob *Mob, tick int64) {
	// Phantoms don't have gravity - they fly
	// No m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 48.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		// Hover in place, slowly drift
		mob.CircleAngle += 0.02
		m.broadcastMobMove(mob)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()

	phaseDuration := tick - mob.SwoopTick

	if mob.SwoopPhase == 0 {
		// Circling phase: fly in circle around target at Y+10-15
		targetY := py + 10 + mob.FlyTargetY
		mob.CircleAngle += 0.05
		circleRadius := 8.0
		targetX := px + math.Cos(mob.CircleAngle)*circleRadius
		targetZ := pz + math.Sin(mob.CircleAngle)*circleRadius

		dx := targetX - mob.X
		dy := targetY - mob.Y
		dz := targetZ - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > 0.5 {
			speed := 0.3
			mob.X += dx / dist * speed
			mob.Y += dy / dist * speed
			mob.Z += dz / dist * speed
		}
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

		// Switch to swoop after 60-100 ticks (3-5 seconds)
		if phaseDuration > int64(60+rand.Intn(40)) {
			mob.SwoopPhase = 1
			mob.SwoopTick = tick
			BroadcastSound(m.Manager, SoundPhantomSwoop, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}
	} else {
		// Swoop phase: dive toward target
		dx := px - mob.X
		dy := (py + 1.0) - mob.Y
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist > 1.0 {
			speed := 0.5
			mob.X += dx / dist * speed
			mob.Y += dy / dist * speed
			mob.Z += dz / dist * speed
		}
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		mob.Pitch = float32(math.Atan2(-dy, math.Sqrt(dx*dx+dz*dz)) * 180 / math.Pi)

		// Attack on contact
		if dist <= 2.0 && mob.AttackCooldown <= 0 {
			mob.AttackCooldown = 20
			nearest.LastDamageMessage = nearest.Name + " was slain by Phantom"
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
			BroadcastSound(m.Manager, SoundPhantomBite, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}

		// After swooping or 40 ticks, climb back up and circle
		if phaseDuration > 40 || dist <= 2.0 {
			mob.SwoopPhase = 0
			mob.SwoopTick = tick
			mob.FlyTargetY = rand.Float64()*5 + 5 // 5-10 blocks above target
			mob.Pitch = 0
		}
	}

	m.broadcastMobMove(mob)
}
