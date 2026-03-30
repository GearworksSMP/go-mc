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
	"github.com/google/uuid"
)

// Mob type IDs (entity type for AddEntity packet, 26.1-snapshot-2 registry).
const (
	MobTypeZombie   int32 = 150
	MobTypeSkeleton int32 = 115
	MobTypeCreeper  int32 = 32
	MobTypeSpider   int32 = 124

	MobTypeEnderman int32 = 41
	MobTypeWitch    int32 = 144
	MobTypeSlime    int32 = 117
	MobTypePhantom  int32 = 99

	MobTypeCow     int32 = 30
	MobTypePig     int32 = 100
	MobTypeSheep   int32 = 111
	MobTypeChicken int32 = 26

	MobTypeBlaze            int32 = 14
	MobTypeGhast            int32 = 57
	MobTypeIronGolem        int32 = 70
	MobTypeSnowGolem        int32 = 121
	MobTypeGuardian         int32 = 63
	MobTypeElderGuardian    int32 = 40
	MobTypeDrowned          int32 = 38
	MobTypeHusk             int32 = 67
	MobTypeStray            int32 = 128
	MobTypeCaveSpider       int32 = 22
	MobTypeSilverfish       int32 = 114
	MobTypeEndermite        int32 = 42
	MobTypeMagmaCube        int32 = 80
	MobTypePiglin           int32 = 101
	MobTypeZombifiedPiglin  int32 = 154
	MobTypeHoglin           int32 = 64
	MobTypeStrider          int32 = 129
	MobTypeWitherSkeleton   int32 = 146
	MobTypeShulker          int32 = 112
	MobTypePillager         int32 = 103
	MobTypeVindicator       int32 = 140
	MobTypeEvoker           int32 = 46
	MobTypeVex              int32 = 138
	MobTypeRavager          int32 = 109
	MobTypeBee              int32 = 11
	MobTypeFox              int32 = 54
	MobTypeRabbit           int32 = 108
	MobTypeBat              int32 = 10
	MobTypeWarden           int32 = 132
	MobTypeFrog             int32 = 55
	MobTypeAxolotl          int32 = 7
	MobTypeAllay            int32 = 2
	MobTypeSniffer          int32 = 119
	MobTypeTurtle           int32 = 131
)

// mobNameToType maps entity names to type IDs for /summon.
var mobNameToType = map[string]int32{
	"zombie": MobTypeZombie, "skeleton": MobTypeSkeleton, "creeper": MobTypeCreeper,
	"spider": MobTypeSpider, "enderman": MobTypeEnderman, "witch": MobTypeWitch,
	"slime": MobTypeSlime, "phantom": MobTypePhantom, "cow": MobTypeCow,
	"pig": MobTypePig, "sheep": MobTypeSheep, "chicken": MobTypeChicken,
	"blaze": MobTypeBlaze, "ghast": MobTypeGhast, "iron_golem": MobTypeIronGolem,
	"snow_golem": MobTypeSnowGolem, "guardian": MobTypeGuardian,
	"elder_guardian": MobTypeElderGuardian, "drowned": MobTypeDrowned,
	"husk": MobTypeHusk, "stray": MobTypeStray, "cave_spider": MobTypeCaveSpider,
	"silverfish": MobTypeSilverfish, "endermite": MobTypeEndermite,
	"magma_cube": MobTypeMagmaCube, "piglin": MobTypePiglin,
	"zombified_piglin": MobTypeZombifiedPiglin, "hoglin": MobTypeHoglin,
	"strider": MobTypeStrider, "wither_skeleton": MobTypeWitherSkeleton,
	"shulker": MobTypeShulker, "pillager": MobTypePillager,
	"vindicator": MobTypeVindicator, "evoker": MobTypeEvoker,
	"vex": MobTypeVex, "ravager": MobTypeRavager,
	"bee": MobTypeBee, "fox": MobTypeFox, "rabbit": MobTypeRabbit, "bat": MobTypeBat,
	"warden": MobTypeWarden, "frog": MobTypeFrog, "axolotl": MobTypeAxolotl,
	"allay": MobTypeAllay, "sniffer": MobTypeSniffer,
	"turtle": MobTypeTurtle,
	"llama": MobTypeLlama, "trader_llama": MobTypeTraderLlama,
}

// MobTypeByName returns the entity type ID for a mob name, or -1 if unknown.
func MobTypeByName(name string) int32 {
	if id, ok := mobNameToType[name]; ok {
		return id
	}
	return -1
}

// mobDefaults returns default health, damage, speed, and hostile flag for a mob type.
func mobDefaults(typeID int32) (health, damage float32, speed float64, hostile bool) {
	switch typeID {
	case MobTypeZombie:
		return 20, 3, 0.115, true
	case MobTypeSkeleton:
		return 20, 2, 0.1, true
	case MobTypeCreeper:
		return 20, 0, 0.1, true
	case MobTypeSpider:
		return 16, 2, 0.15, true
	case MobTypeEnderman:
		return 40, 7, 0.15, true
	case MobTypeWitch:
		return 26, 3, 0.1, true
	case MobTypeSlime:
		return 16, 3, 0.1, true
	case MobTypeCow, MobTypePig, MobTypeSheep:
		return 10, 0, 0.1, false
	case MobTypeChicken:
		return 4, 0, 0.1, false
	case MobTypeIronGolem:
		return 100, 15, 0.1, false
	case MobTypeBlaze:
		return 20, 6, 0.1, true
	case MobTypeGhast:
		return 10, 6, 0.05, true
	case MobTypeWitherSkeleton:
		return 20, 8, 0.1, true
	case MobTypeCaveSpider:
		return 12, 2, 0.15, true
	case MobTypeWarden:
		return 500, 30, 0.3, true
	case MobTypeFrog:
		return 10, 0, 0.1, false
	case MobTypeAxolotl:
		return 14, 2, 0.1, false
	case MobTypeAllay:
		return 20, 0, 0.08, false
	case MobTypeSniffer:
		return 14, 0, 0.09, false
	case MobTypeTurtle:
		return 30, 0, 0.1, false
	case MobTypeLlama, MobTypeTraderLlama:
		return 22, 1, 0.1, false
	default:
		return 20, 3, 0.1, true
	}
}

// SpawnMobAt spawns a mob of the given type at the specified position and returns its EID.
func (m *MobManager) SpawnMobAt(typeID int32, x, y, z float64) int32 {
	health, damage, speed, hostile := mobDefaults(typeID)
	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    typeID,
		X:         x,
		Y:         y,
		Z:         z,
		PrevX:     x,
		PrevY:     y,
		PrevZ:     z,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     speed,
		WanderYaw: rand.Float32() * 360,
		Hostile:   hostile,
	}
	if typeID == MobTypeSheep {
		mob.WoolColor = 0 // default white for summoned sheep
	}
	if isLlamaType(typeID) {
		mob.TameData = &TameableMobData{
			CarpetColor:   -1,
			LlamaStrength: 1 + rand.Int31n(5),
		}
	}
	m.mu.Lock()
	m.Mobs[eid] = mob
	m.Spatial.Insert(eid, x, z)
	m.mu.Unlock()
	m.broadcastSpawn(mob)
	return eid
}

// isSlimeType returns true if the mob type uses index 16 for slime size (not baby flag).
func isSlimeType(typeID int32) bool {
	return typeID == MobTypeSlime || typeID == MobTypeMagmaCube
}

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


// grantBadOmen increments a player's Bad Omen level when they kill a patrol captain.
func (m *MobManager) grantBadOmen(killer *game.Player) {
	killer.BadOmen++
	if killer.BadOmen > 7 {
		killer.BadOmen = 7
	}
	if m.EffectMgr != nil {
		m.EffectMgr.ApplyEffect(killer, EffectBadOmen, killer.BadOmen-1, 120000, false)
	}
	if m.Logger != nil {
		m.Logger.Printf("Player %s killed patrol captain, Bad Omen level %d", killer.Name, killer.BadOmen)
	}
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

// sweepDeadMobs removes mobs whose death animation has finished.
func (m *MobManager) sweepDeadMobs(tick int64) {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.DeathTick > 0 && tick-mob.DeathTick >= 20 {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		mob := m.Mobs[eid]
		if mob != nil {
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(mob.EID),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(removePkt)
			})
		}
		m.Spatial.Remove(eid)
		delete(m.Mobs, eid)
	}
}

// mobExtraData holds type-specific mob fields for JSON persistence.
type mobExtraData struct {
	SlimeSize  int32   `json:"slime_size,omitempty"`
	Hostile    bool    `json:"hostile,omitempty"`
	Damage     float32 `json:"damage,omitempty"`
	Speed      float64 `json:"speed,omitempty"`
	CustomName string  `json:"custom_name,omitempty"`
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

const (
	// Per-player mob caps (vanilla-inspired).
	hostileCapPerPlayer = 70
	passiveCapPerPlayer = 10
)

// countMobsNear counts mobs within radiusSq (squared distance) of (x, z).
// If hostile is true, counts only hostile mobs; if false, counts only passive mobs.
func (m *MobManager) countMobsNear(x, z, radiusSq float64, hostile bool) int {
	count := 0
	for _, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		if mob.Hostile != hostile {
			continue
		}
		dx := mob.X - x
		dz := mob.Z - z
		if dx*dx+dz*dz <= radiusSq {
			count++
		}
	}
	return count
}

// trySpawn attempts to spawn hostile mobs near each survival player.
// Uses per-player mob caps and dimension-aware mob selection:
// - Overworld: night-only, standard hostile mobs
// - Nether: always spawns, nether-specific mobs
// - End: always spawns, endermen only
func (m *MobManager) trySpawn() {
	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && p.GameMode == 0 {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	for _, player := range players {
		px, _, pz := player.Position()

		// Per-player cap check: count hostile mobs within 128 blocks
		nearbyHostile := m.countMobsNear(px, pz, 128*128, true)
		if nearbyHostile >= hostileCapPerPlayer {
			continue
		}

		spawnX, spawnY, spawnZ, ok := m.pickSpawnPos(player)
		if !ok {
			continue
		}

		// Dimension-aware spawning
		switch player.Dimension {
		case "minecraft:the_nether":
			m.spawnNetherHostileAt(spawnX, float64(spawnY), spawnZ)
		case "minecraft:the_end":
			m.spawnEndHostileAt(spawnX, float64(spawnY), spawnZ)
		default:
			if !m.TimeMgr.IsNight() {
				continue
			}
			m.spawnOverworldHostileAt(spawnX, float64(spawnY), spawnZ)
		}
	}
}

// pickSpawnPos picks a random spawn position 24-48 blocks from the player and
// returns the coordinates. ok is false if no valid surface was found.
func (m *MobManager) pickSpawnPos(player *game.Player) (x float64, y int, z float64, ok bool) {
	px, _, pz := player.Position()
	angle := rand.Float64() * 2 * math.Pi
	dist := 24 + rand.Float64()*24
	x = px + math.Cos(angle)*dist
	z = pz + math.Sin(angle)*dist
	y = m.findSurfaceY(int(x), int(z))
	ok = y >= m.MinY
	return
}

// spawnOverworldHostileAt spawns a hostile mob at the given position using overworld mob table.
func (m *MobManager) spawnOverworldHostileAt(x, y, z float64) {
	var typeID int32
	var health, damage float32
	var speed float64
	roll := rand.Float64()
	switch {
	case roll < 0.40:
		typeID, health, damage, speed = MobTypeZombie, 20, 3, 0.115
	case roll < 0.60:
		typeID, health, damage, speed = MobTypeSkeleton, 20, 2, 0.1
	case roll < 0.75:
		typeID, health, damage, speed = MobTypeSpider, 16, 2, 0.15
	case roll < 0.85:
		typeID, health, damage, speed = MobTypeCreeper, 20, 0, 0.1
	case roll < 0.90:
		typeID, health, damage, speed = MobTypeEnderman, 40, 7, 0.15
	case roll < 0.95:
		typeID, health, damage, speed = MobTypeWitch, 26, 3, 0.1
	default:
		typeID, health, damage, speed = MobTypeSpider, 16, 2, 0.15
	}

	m.spawnHostileMob(x+0.5, y, z+0.5, typeID, health, damage, speed)
}

// spawnNetherHostileAt spawns a nether-specific hostile mob at the given position.
func (m *MobManager) spawnNetherHostileAt(x, y, z float64) {
	var typeID int32
	var health, damage float32
	var speed float64
	roll := rand.Float64()
	switch {
	case roll < 0.30:
		typeID, health, damage, speed = MobTypeZombifiedPiglin, 20, 5, 0.1
	case roll < 0.50:
		typeID, health, damage, speed = MobTypePiglin, 16, 5, 0.1
	case roll < 0.65:
		typeID, health, damage, speed = MobTypeMagmaCube, 16, 3, 0.1
	case roll < 0.80:
		typeID, health, damage, speed = MobTypeGhast, 10, 6, 0.05
	case roll < 0.90:
		typeID, health, damage, speed = MobTypeWitherSkeleton, 20, 8, 0.1
	default:
		typeID, health, damage, speed = MobTypeBlaze, 20, 6, 0.1
	}

	m.spawnHostileMob(x+0.5, y, z+0.5, typeID, health, damage, speed)
}

// spawnEndHostileAt spawns an enderman at the given position in the End.
func (m *MobManager) spawnEndHostileAt(x, y, z float64) {
	m.spawnHostileMob(x+0.5, y, z+0.5, MobTypeEnderman, 40, 7, 0.15)
}

// spawnHostileMob creates a hostile mob at the given position and broadcasts it.
func (m *MobManager) spawnHostileMob(x, y, z float64, typeID int32, health, damage float32, speed float64) {
	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    typeID,
		X:         x,
		Y:         y,
		Z:         z,
		PrevX:     x,
		PrevY:     y,
		PrevZ:     z,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     speed,
		WanderYaw: rand.Float32() * 360,
		Hostile:   true,
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
}

// trySpawnPassive attempts to spawn passive mobs on grass during daytime.
// Since passive mobs no longer despawn, spawning is conservative:
// only spawns if fewer than passiveCapPerPlayer passive mobs are near the target player.
func (m *MobManager) trySpawnPassive() {
	if m.TimeMgr.IsNight() {
		return
	}

	// Only spawn passive mobs in the overworld
	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && (p.Dimension == "" || p.Dimension == "minecraft:overworld") {
			players = append(players, p)
		}
	})

	for _, player := range players {
		px, _, pz := player.Position()

		// Per-player cap: only spawn if fewer than passiveCapPerPlayer passive mobs nearby
		nearbyPassive := m.countMobsNear(px, pz, 128*128, false)
		if nearbyPassive >= passiveCapPerPlayer {
			continue
		}

		// Pick random position 8-32 blocks from player
		angle := rand.Float64() * 2 * math.Pi
		dist := 8 + rand.Float64()*24
		spawnX := px + math.Cos(angle)*dist
		spawnZ := pz + math.Sin(angle)*dist

		// Find surface Y
		spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
		if spawnY < m.MinY {
			continue
		}

		// Check that the block below is grass_block
		belowState, err := m.World.GetBlock(int(spawnX), spawnY-1, int(spawnZ))
		if err != nil {
			continue
		}
		blockName := BlockNameFromState(int(belowState))
		if blockName != "grass_block" {
			continue
		}

		m.spawnPassiveMobAt(spawnX+0.5, float64(spawnY), spawnZ+0.5)
	}
}

// spawnPassiveMobAt creates a random passive mob at the given position.
func (m *MobManager) spawnPassiveMobAt(x, y, z float64) {
	var typeID int32
	var health float32
	var vdata *VillagerData
	var tdata *TameableMobData
	roll := rand.Float64()
	switch {
	case roll < 0.20:
		typeID, health = MobTypeCow, 10
	case roll < 0.40:
		typeID, health = MobTypePig, 10
	case roll < 0.54:
		typeID, health = MobTypeSheep, 8
	case roll < 0.68:
		typeID, health = MobTypeChicken, 4
	case roll < 0.76:
		typeID, health = MobTypeVillager, 20
		vdata = NewVillagerData(RandomProfession())
	case roll < 0.84:
		typeID, health = MobTypeWolf, 8
	case roll < 0.90:
		typeID, health = MobTypeCat, 10
	case roll < 0.96:
		typeID, health = MobTypeHorse, 15 + float32(rand.Intn(16))
		tdata = &TameableMobData{
			HorseSpeed:   0.1 + rand.Float64()*0.05,
			HorseJump:    0.4 + rand.Float64()*0.3,
			HorseVariant: rand.Int31n(7)*256 + rand.Int31n(5),
		}
	case roll < 0.98:
		typeID, health = MobTypeLlama, 22
		tdata = &TameableMobData{
			CarpetColor:   -1,
			LlamaStrength: 1 + rand.Int31n(5),
		}
	default:
		typeID, health = MobTypeParrot, 6
	}

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:          eid,
		TypeID:       typeID,
		X:            x,
		Y:            y,
		Z:            z,
		PrevX:        x,
		PrevY:        y,
		PrevZ:        z,
		Health:       health,
		MaxHealth:    health,
		Damage:       0,
		Speed:        0.1,
		WanderYaw:    rand.Float32() * 360,
		Hostile:      false,
		VillagerData: vdata,
		TameData:     tdata,
	}
	// Assign random wool color for sheep (vanilla distribution)
	if typeID == MobTypeSheep {
		r := rand.Float64()
		switch {
		case r < 0.8184: // white (81.84%)
			mob.WoolColor = 0
		case r < 0.8184+0.05: // orange
			mob.WoolColor = 1
		case r < 0.8184+0.10: // magenta
			mob.WoolColor = 2
		case r < 0.8184+0.15: // light blue
			mob.WoolColor = 3
		case r < 0.9684: // black (3%)
			mob.WoolColor = 15
		case r < 0.9884: // gray (2%)
			mob.WoolColor = 7
		case r < 0.9984: // brown (1%)
			mob.WoolColor = 12
		default: // pink (0.16%)
			mob.WoolColor = 6
		}
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
}

// findSurfaceY returns the Y coordinate of the surface at (x, z).
func (m *MobManager) findSurfaceY(x, z int) int {
	// Scan from top down
	maxY := m.MinY + 24*16 - 1
	for y := maxY; y >= m.MinY; y-- {
		state, err := m.World.GetBlock(x, y, z)
		if err != nil {
			continue
		}
		if state != 0 { // not air
			// Check block above is air
			above, err := m.World.GetBlock(x, y+1, z)
			if err == nil && above == 0 {
				above2, err2 := m.World.GetBlock(x, y+2, z)
				if err2 == nil && above2 == 0 {
					return y + 1
				}
			}
		}
	}
	return m.MinY - 1
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
	case mob.TypeID == MobTypeTurtle:
		if m.TurtleMgr != nil {
			m.TurtleMgr.tickTurtle(mob, tick)
		} else {
			m.tickPassive(mob, tick)
		}
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

// tickPassive runs passive mob AI with flee and breeding behavior.
func (m *MobManager) tickPassive(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Flee behavior: move toward flee target at 1.5x speed
	if mob.FleeTicks > 0 {
		mob.FleeTicks--
		dx := mob.FleeX - mob.X
		dz := mob.FleeZ - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 1.0 {
			nx := dx / dist * mob.Speed * 1.5
			nz := dz / dist * mob.Speed * 1.5
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
			m.broadcastMobMove(mob)
			return
		}
		mob.FleeTicks = 0
	}

	// Baby growth: count down and convert to adult
	if mob.Baby && mob.BabyAge > 0 {
		mob.BabyAge--
		if mob.BabyAge <= 0 {
			mob.Baby = false
			mob.Speed /= 1.5 // revert baby speed boost
			// Send adult metadata (isBaby = false)
			var w MetadataWriter
			w.WriteBoolean(16, false)
			data := w.Bytes()
			m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
				SendEntityMetadata(p, mob.EID, data)
			})
		}
	}

	// Love mode countdown (babies can't breed)
	if !mob.Baby && mob.LoveTicks > 0 {
		mob.LoveTicks--
		// Check for nearby mob in love mode for breeding
		if mob.LoveTicks > 0 && mob.BreedCooldown <= 0 {
			m.tryBreed(mob)
		}
	}

	// Breed cooldown
	if mob.BreedCooldown > 0 {
		mob.BreedCooldown--
	}

	m.tickWander(mob, tick)
}

// isBreedingFood returns true if the item is food for the given mob type.
func isBreedingFood(mobType int32, itemName string) bool {
	switch mobType {
	case MobTypeCow, MobTypeSheep:
		return itemName == "wheat"
	case MobTypePig:
		return itemName == "carrot" || itemName == "potato" || itemName == "beetroot"
	case MobTypeChicken:
		return itemName == "wheat_seeds" || itemName == "melon_seeds" ||
			itemName == "pumpkin_seeds" || itemName == "beetroot_seeds"
	case MobTypeWolf:
		return itemName == "cooked_beef" || itemName == "cooked_porkchop" ||
			itemName == "cooked_chicken" || itemName == "cooked_mutton"
	case MobTypeCat:
		return itemName == "cod" || itemName == "salmon" ||
			itemName == "raw_cod" || itemName == "raw_salmon"
	case MobTypeHorse:
		return itemName == "golden_carrot" || itemName == "golden_apple"
	case MobTypeParrot:
		return false // parrots don't breed
	case MobTypeTurtle:
		return itemName == "seagrass"
	}
	return false
}

// FeedMob attempts to put a mob into love mode for breeding.
// Returns true if the mob was fed successfully.
func (m *MobManager) FeedMob(targetEID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 || mob.Hostile {
		return false
	}
	if mob.Baby {
		// Feeding a baby accelerates growth by 10% (2400 ticks)
		if mob.BabyAge > 0 {
			mob.BabyAge -= 2400
			if mob.BabyAge < 0 {
				mob.BabyAge = 0
			}
		}
		return true
	}
	if mob.LoveTicks > 0 || mob.BreedCooldown > 0 {
		return false
	}

	mob.LoveTicks = 600 // 30 seconds to find a mate

	// Broadcast heart particles
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(18), // love mode hearts
		))
	})

	return true
}

// tryBreed checks for a nearby mob of the same type in love mode and breeds them.
func (m *MobManager) tryBreed(mob *Mob) {
	for _, other := range m.Mobs {
		if other.EID == mob.EID || other.TypeID != mob.TypeID {
			continue
		}
		if other.Health <= 0 || other.LoveTicks <= 0 || other.BreedCooldown > 0 {
			continue
		}
		dx := other.X - mob.X
		dz := other.Z - mob.Z
		if dx*dx+dz*dz > 64 { // within 8 blocks
			continue
		}

		// Breed! Spawn baby at midpoint
		babyX := (mob.X + other.X) / 2
		babyZ := (mob.Z + other.Z) / 2

		eid := m.Manager.NextEntityID()
		baby := &Mob{
			EID:       eid,
			TypeID:    mob.TypeID,
			X:         babyX,
			Y:         mob.Y,
			Z:         babyZ,
			PrevX:     babyX,
			PrevY:     mob.Y,
			PrevZ:     babyZ,
			Health:    mob.MaxHealth / 2,
			MaxHealth: mob.MaxHealth,
			Speed:     mob.Speed * 1.5, // babies move faster
			WanderYaw: rand.Float32() * 360,
			Hostile:   false,
			Baby:      true,
			BabyAge:   24000, // 20 minutes to grow up
		}
		m.Mobs[eid] = baby
		m.broadcastSpawn(baby)

		// Heart particles at both parent positions
		BroadcastParticle(m.Manager, ParticleHeart, mob.X, mob.Y+1.0, mob.Z, 0.3, 0.3, 0.3, 0.0, 7)
		BroadcastParticle(m.Manager, ParticleHeart, other.X, other.Y+1.0, other.Z, 0.3, 0.3, 0.3, 0.0, 7)

		// Reset parents
		mob.LoveTicks = 0
		mob.BreedCooldown = 6000 // 5 minutes
		other.LoveTicks = 0
		other.BreedCooldown = 6000

		return
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

// DamageMob applies damage to a mob from a player attack.
// Returns true if the mob was found and damaged.
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

// DamageMobByArrow applies arrow damage to a mob. Returns whether the mob died and its type ID.
func (m *MobManager) DamageMobByArrow(shooterEID, targetEID int32, damage float32) (killed bool, typeID int32) {
	m.mu.Lock()
	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		m.mu.Unlock()
		return false, 0
	}
	typeID = mob.TypeID
	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
	}

	// Broadcast hurt animation
	hurtPkt := pk.Marshal(packetid.ClientboundHurtAnimation, pk.VarInt(mob.EID), pk.Float(0))
	m.Manager.ForEach(func(p *game.Player) { p.WritePacket(hurtPkt) })

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	if mob.Health <= 0 {
		m.mu.Unlock()
		m.killMob(mob, nil) // no player killer for mob-on-mob
		return true, typeID
	}
	m.mu.Unlock()
	return false, typeID
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

func (m *MobManager) DamageMob(attacker *game.Player, targetEID int32, damage float32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		return false
	}

	// Apply horse armor damage reduction
	if mob.TypeID == MobTypeHorse {
		reduction := horseArmorDamageReduction(mob)
		damage -= reduction
		if damage < 0 {
			damage = 0
		}
	}

	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
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

	// Broadcast damage event
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(mob.EID),
		pk.VarInt(m.Survival.MobDamageTypeID),
		pk.VarInt(attacker.EID+1), // cause entity (+1 because 0 = none)
		pk.VarInt(attacker.EID+1), // direct entity
		pk.Boolean(false),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Enderman teleport on hit
	if mob.TypeID == MobTypeEnderman && mob.TeleportCooldown <= 0 && mob.Health > 0 {
		m.endermanTeleport(mob)
		mob.TeleportCooldown = 20
		mob.Target = attacker // aggro on the attacker
	}

	// Zombified piglin swarm aggro
	if mob.TypeID == MobTypeZombifiedPiglin {
		mob.Hostile = true
		mob.Target = attacker
		m.AggroZombifiedPiglins(attacker, mob.X, mob.Y, mob.Z)
	}

	// Passive mobs flee when hit
	if !mob.Hostile {
		px, _, pz := attacker.Position()
		dx := mob.X - px
		dz := mob.Z - pz
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			mob.FleeX = mob.X + dx/dist*16
			mob.FleeZ = mob.Z + dz/dist*16
		} else {
			mob.FleeX = mob.X + (rand.Float64()-0.5)*16
			mob.FleeZ = mob.Z + (rand.Float64()-0.5)*16
		}
		mob.FleeTicks = 60 // 3 seconds
	}

	m.applyVillagerDamageGossip(mob, attacker)

	if mob.Health <= 0 {
		m.applyVillagerKillGossip(mob, attacker)
		m.killMob(mob, attacker)
	}

	return true
}

// DamageMobEx applies damage to a mob with combat enchant effects.
// knockbackLevel: extra knockback (from enchant + sprint).
// fireAspectLevel: sets mob on fire visual (4s per level).
// lootingLevel: extra loot drops on kill.
func (m *MobManager) DamageMobEx(attacker *game.Player, targetEID int32, damage float32, knockbackLevel, fireAspectLevel, lootingLevel int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		return false
	}

	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
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

	// Broadcast damage event
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(mob.EID),
		pk.VarInt(m.Survival.MobDamageTypeID),
		pk.VarInt(attacker.EID+1),
		pk.VarInt(attacker.EID+1),
		pk.Boolean(false),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Knockback enchantment
	if knockbackLevel > 0 {
		ax, _, az := attacker.Position()
		dx := mob.X - ax
		dz := mob.Z - az
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			kbDist := float64(knockbackLevel) * 0.5
			mob.X += dx / dist * kbDist
			mob.Z += dz / dist * kbDist
			m.broadcastMobTeleport(mob)
		}
	}

	// Fire Aspect: set mob on fire with ticking damage
	if fireAspectLevel > 0 {
		mob.FireTicks = fireAspectLevel * 80 // 4 seconds per level
		broadcastFireMetadata(m.Manager, mob.EID, true)
	}

	// Enderman teleport on hit
	if mob.TypeID == MobTypeEnderman && mob.TeleportCooldown <= 0 && mob.Health > 0 {
		m.endermanTeleport(mob)
		mob.TeleportCooldown = 20
		mob.Target = attacker
	}

	// Passive mobs flee when hit
	if !mob.Hostile {
		px, _, pz := attacker.Position()
		dx := mob.X - px
		dz := mob.Z - pz
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			mob.FleeX = mob.X + dx/dist*16
			mob.FleeZ = mob.Z + dz/dist*16
		} else {
			mob.FleeX = mob.X + (rand.Float64()-0.5)*16
			mob.FleeZ = mob.Z + (rand.Float64()-0.5)*16
		}
		mob.FleeTicks = 60
	}

	m.applyVillagerDamageGossip(mob, attacker)

	if mob.Health <= 0 {
		m.applyVillagerKillGossip(mob, attacker)
		m.killMobWithLooting(mob, attacker, lootingLevel)
	}

	return true
}

// DamageMobsNearExcept deals sweep damage to mobs within radius of the target mob.
func (m *MobManager) DamageMobsNearExcept(attacker *game.Player, primaryEID int32, damage float32, radius float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	primary, ok := m.Mobs[primaryEID]
	if !ok {
		return
	}

	for _, mob := range m.Mobs {
		if mob.EID == primaryEID || mob.Health <= 0 {
			continue
		}
		dx := mob.X - primary.X
		dy := mob.Y - primary.Y
		dz := mob.Z - primary.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > radius {
			continue
		}

		mob.Health -= damage
		if mob.Health < 0 {
			mob.Health = 0
		}

		hurtPkt := pk.Marshal(
			packetid.ClientboundHurtAnimation,
			pk.VarInt(mob.EID),
			pk.Float(0),
		)
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(hurtPkt)
		})

		if mob.Health <= 0 {
			m.killMob(mob, attacker)
		}
	}
}

// killMobWithLooting handles mob death with Looting enchantment bonus drops.
func (m *MobManager) killMobWithLooting(mob *Mob, killer *game.Player, lootingLevel int32) {
	if mob.DeathTick > 0 {
		return // already dying
	}
	mob.Health = 0
	mob.DeathTick = m.currentTick
	if mob.DeathTick == 0 {
		mob.DeathTick = 1
	}

	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Death particles (poof + smoke)
	BroadcastParticle(m.Manager, ParticlePoof, mob.X, mob.Y+0.5, mob.Z, 0.3, 0.5, 0.3, 0.05, 10)
	BroadcastParticle(m.Manager, ParticleSmoke, mob.X, mob.Y+0.5, mob.Z, 0.2, 0.4, 0.2, 0.02, 5)

	// Death animation
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(3),
		))
	})

	// Drop loot with looting bonus
	m.dropMobLootWithLooting(mob, lootingLevel)

	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 1 {
		m.slimeSplit(mob)
	}

	// Spawn XP orbs at mob death location
	if killer != nil {
		if mob.TypeID == MobTypePillager && mob.IsPatrolCaptain {
			m.grantBadOmen(killer)
		}
		m.spawnMobXPOrbs(mob)
	}
}

// killMob handles mob death: animation, drops, XP. Entity removal is deferred
// to sweepDeadMobs (20 ticks later) to allow the death animation to play.
func (m *MobManager) killMob(mob *Mob, killer *game.Player) {
	if mob.DeathTick > 0 {
		return // already dying
	}
	mob.Health = 0
	mob.DeathTick = m.currentTick
	if mob.DeathTick == 0 {
		mob.DeathTick = 1 // ensure nonzero so sweep detects it
	}

	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Death particles (poof + smoke)
	BroadcastParticle(m.Manager, ParticlePoof, mob.X, mob.Y+0.5, mob.Z, 0.3, 0.5, 0.3, 0.05, 10)
	BroadcastParticle(m.Manager, ParticleSmoke, mob.X, mob.Y+0.5, mob.Z, 0.2, 0.4, 0.2, 0.02, 5)

	// Death animation
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(3), // death
		))
	})

	// Drop loot
	m.dropMobLoot(mob)

	// Slime split on death: spawn smaller slimes
	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 1 {
		m.slimeSplit(mob)
	}

	// Advancement check
	if killer != nil && m.AdvMgr != nil {
		m.AdvMgr.CheckMobKill(killer, mob.TypeID)
	}

	// Award XP to killer
	// Spawn XP orbs at mob death location
	if killer != nil {
		if mob.TypeID == MobTypePillager && mob.IsPatrolCaptain {
			m.grantBadOmen(killer)
		}
		m.spawnMobXPOrbs(mob)
	}
}

// spawnMobXPOrbs spawns XP orb entities at a mob's death location.
func (m *MobManager) spawnMobXPOrbs(mob *Mob) {
	if m.XPOrbMgr == nil {
		return
	}
	xp := mobXPAmount(mob)
	if xp > 0 {
		m.XPOrbMgr.SpawnXPOrbs(mob.X, mob.Y+0.5, mob.Z, xp)
	}
}

// mobXPAmount returns the vanilla XP value for a mob type.
func mobXPAmount(mob *Mob) int32 {
	switch mob.TypeID {
	case MobTypeBlaze:
		return 10
	case MobTypeSlime, MobTypeMagmaCube:
		if mob.SlimeSize >= 4 {
			return 4
		}
		if mob.SlimeSize >= 2 {
			return 2
		}
		return 1
	case MobTypeCow, MobTypePig, MobTypeSheep, MobTypeChicken:
		return int32(1 + rand.Intn(3))
	case MobTypeBat:
		return 0
	default:
		if mob.Hostile {
			return 5
		}
		return int32(1 + rand.Intn(3))
	}
}

// dropMobLoot drops item entities for a killed mob.
func (m *MobManager) dropMobLoot(mob *Mob) {
	m.dropMobLootWithLooting(mob, 0)
}

// woolColorToItem returns the wool item name for a given dye color ID.
func woolColorToItem(color int32) string {
	switch color {
	case 0:
		return "white_wool"
	case 1:
		return "orange_wool"
	case 2:
		return "magenta_wool"
	case 3:
		return "light_blue_wool"
	case 4:
		return "yellow_wool"
	case 5:
		return "lime_wool"
	case 6:
		return "pink_wool"
	case 7:
		return "gray_wool"
	case 8:
		return "light_gray_wool"
	case 9:
		return "cyan_wool"
	case 10:
		return "purple_wool"
	case 11:
		return "blue_wool"
	case 12:
		return "brown_wool"
	case 13:
		return "green_wool"
	case 14:
		return "red_wool"
	case 15:
		return "black_wool"
	default:
		return "white_wool"
	}
}

// cookedMeats maps raw meat items to their cooked variants (for fire kills).
var cookedMeats = map[string]string{
	"beef":     "cooked_beef",
	"porkchop": "cooked_porkchop",
	"mutton":   "cooked_mutton",
	"chicken":  "cooked_chicken",
	"rabbit":   "cooked_rabbit",
	"cod":      "cooked_cod",
	"salmon":   "cooked_salmon",
}

// dropMobLootWithLooting drops loot with Looting enchantment bonus.
// Each Looting level adds 0-1 extra items per drop.
func (m *MobManager) dropMobLootWithLooting(mob *Mob, lootingLevel int32) {
	if m.ItemEntities == nil {
		return
	}

	type drop struct {
		name     string
		minCount int32
		maxCount int32
	}

	var drops []drop
	switch mob.TypeID {
	case MobTypeCow:
		drops = []drop{{"beef", 1, 3}, {"leather", 0, 2}}
	case MobTypePig:
		drops = []drop{{"porkchop", 1, 3}}
	case MobTypeSheep:
		// Drop colored wool based on sheep's wool color
		woolName := woolColorToItem(mob.WoolColor)
		drops = []drop{{woolName, 1, 1}, {"mutton", 1, 2}}
	case MobTypeChicken:
		drops = []drop{{"chicken", 1, 1}, {"feather", 0, 2}}
	case MobTypeZombie:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Rare drops: 2.5% base + 1% per looting level
		rareChance := 0.025 + float64(lootingLevel)*0.01
		if rand.Float64() < rareChance {
			switch rand.Intn(3) {
			case 0:
				drops = append(drops, drop{"iron_ingot", 1, 1})
			case 1:
				drops = append(drops, drop{"carrot", 1, 1})
			case 2:
				drops = append(drops, drop{"potato", 1, 1})
			}
		}
	case MobTypeSkeleton:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
		// 8.5% chance to drop a bow (+ 1% per looting level)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"bow", 1, 1})
		}
	case MobTypeSpider:
		drops = []drop{{"string", 0, 2}}
		// Spider eye only drops when killed by player (always true here)
		if rand.Float64() < 0.33+float64(lootingLevel)*0.015 {
			drops = append(drops, drop{"spider_eye", 1, 1})
		}
	case MobTypeCreeper:
		drops = []drop{{"gunpowder", 0, 2}}
	case MobTypeEnderman:
		drops = []drop{{"ender_pearl", 0, 1}}
	case MobTypeBlaze:
		drops = []drop{{"blaze_rod", 0, 1}}
	case MobTypeWitherSkeleton:
		drops = []drop{{"bone", 0, 2}, {"coal", 0, 1}}
		if rand.Float64() < 0.025+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"wither_skeleton_skull", 1, 1})
		}
	case MobTypeGuardian:
		drops = []drop{{"prismarine_shard", 0, 2}}
		if rand.Float64() < 0.4 {
			drops = append(drops, drop{"cod", 0, 1})
		}
	case MobTypeElderGuardian:
		drops = []drop{{"prismarine_shard", 0, 2}, {"wet_sponge", 1, 1}}
		if rand.Float64() < 0.5 {
			drops = append(drops, drop{"cod", 0, 2})
		}
	case MobTypeDrowned:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Rare drop: copper ingot (5% + 2% per looting)
		if rand.Float64() < 0.05+float64(lootingLevel)*0.02 {
			drops = append(drops, drop{"copper_ingot", 1, 1})
		}
	case MobTypeHusk:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Same rare drops as zombie
		rareChance := 0.025 + float64(lootingLevel)*0.01
		if rand.Float64() < rareChance {
			switch rand.Intn(3) {
			case 0:
				drops = append(drops, drop{"iron_ingot", 1, 1})
			case 1:
				drops = append(drops, drop{"carrot", 1, 1})
			case 2:
				drops = append(drops, drop{"potato", 1, 1})
			}
		}
	case MobTypeStray:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
		// Stray drops tipped arrows of slowness (50% + looting)
		if rand.Float64() < 0.5+float64(lootingLevel)*0.05 {
			drops = append(drops, drop{"tipped_arrow", 1, 1})
		}
	case MobTypeCaveSpider:
		drops = []drop{{"string", 0, 2}}
		if rand.Float64() < 0.33+float64(lootingLevel)*0.015 {
			drops = append(drops, drop{"spider_eye", 1, 1})
		}
	case MobTypeMagmaCube:
		drops = []drop{{"magma_cream", 0, 1}}
	case MobTypeGhast:
		drops = []drop{{"ghast_tear", 0, 1}, {"gunpowder", 0, 2}}
	case MobTypePiglin:
		drops = []drop{{"gold_ingot", 0, 1}}
	case MobTypeZombifiedPiglin:
		drops = []drop{{"rotten_flesh", 0, 1}, {"gold_nugget", 0, 1}}
		// Rare drop: gold ingot (2.5% + looting)
		if rand.Float64() < 0.025+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"gold_ingot", 1, 1})
		}
	case MobTypeHoglin:
		drops = []drop{{"porkchop", 2, 4}, {"leather", 1, 3}}
	case MobTypeStrider:
		drops = []drop{{"string", 2, 5}}
	case MobTypePillager:
		drops = []drop{{"arrow", 0, 2}}
		// Rare: crossbow (8.5% + looting)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"crossbow", 1, 1})
		}
	case MobTypeVindicator:
		drops = []drop{{"emerald", 0, 1}}
		// Rare: iron axe (8.5% + looting)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"iron_axe", 1, 1})
		}
	case MobTypeEvoker:
		drops = []drop{{"totem_of_undying", 1, 1}, {"emerald", 0, 1}}
	case MobTypeRavager:
		drops = []drop{{"saddle", 1, 1}}
	case MobTypeShulker:
		// 50% chance + 6.25% per looting level
		if rand.Float64() < 0.5+float64(lootingLevel)*0.0625 {
			drops = []drop{{"shulker_shell", 1, 1}}
		}
	case MobTypeRabbit:
		drops = []drop{{"rabbit", 0, 1}, {"rabbit_hide", 0, 1}}
		// Rare: rabbit's foot (10% + looting)
		if rand.Float64() < 0.1+float64(lootingLevel)*0.03 {
			drops = append(drops, drop{"rabbit_foot", 1, 1})
		}
	case MobTypeIronGolem:
		drops = []drop{{"iron_ingot", 3, 5}, {"poppy", 0, 2}}
	case MobTypeSnowGolem:
		drops = []drop{{"snowball", 0, 15}}
	case MobTypeVillager:
		// Villagers don't drop items in vanilla
	case MobTypeWitch:
		// Witch drops 1-3 of these categories randomly
		witchItems := []drop{
			{"glass_bottle", 0, 2},
			{"redstone", 0, 2},
			{"glowstone_dust", 0, 2},
			{"gunpowder", 0, 2},
			{"sugar", 0, 2},
			{"spider_eye", 0, 2},
			{"stick", 0, 2},
		}
		count := 1 + rand.Intn(3) // drop 1-3 categories
		for i := 0; i < count && i < len(witchItems); i++ {
			idx := rand.Intn(len(witchItems))
			drops = append(drops, witchItems[idx])
		}
	case MobTypeSlime:
		if mob.SlimeSize <= 1 {
			drops = []drop{{"slime_ball", 0, 2}}
		}
	case MobTypePhantom:
		drops = []drop{{"phantom_membrane", 0, 1}}
	case MobTypeCat:
		if rand.Float64() < 0.5 {
			drops = []drop{{"string", 0, 2}}
		}
	case MobTypeHorse:
		drops = []drop{{"leather", 0, 2}}
		if mob.TameData != nil {
			if mob.TameData.ArmorItemID > 0 {
				if armorName := ItemNameByID(mob.TameData.ArmorItemID); armorName != "" {
					drops = append(drops, drop{armorName, 1, 1})
				}
			}
			if mob.TameData.HasSaddle {
				drops = append(drops, drop{"saddle", 1, 1})
			}
		}
	case MobTypeLlama, MobTypeTraderLlama:
		drops = []drop{{"leather", 0, 2}}
		// Drop chest contents
		if mob.TameData != nil && mob.TameData.HasChest {
			for _, item := range mob.TameData.ChestInventory {
				if item.ID > 0 {
					itemName := ItemNameByID(item.ID)
					if itemName != "" {
						drops = append(drops, drop{itemName, item.Count, item.Count})
					}
				}
			}
		}
	case MobTypeParrot:
		drops = []drop{{"feather", 1, 2}}
	case MobTypeBee:
		// Bees drop nothing in vanilla
	case MobTypeWarden:
		drops = []drop{{"sculk_catalyst", 1, 1}}
	case MobTypeFrog:
		// Frogs drop nothing in vanilla (froglights come from tongue attack)
	case MobTypeAxolotl:
		// Axolotls drop nothing in vanilla
	case MobTypeAllay:
		// Allays drop nothing in vanilla (but drop their held item)
	case MobTypeSniffer:
		drops = []drop{{"moss_block", 1, 1}}
	case MobTypeTurtle:
		if !mob.Baby {
			drops = []drop{{"seagrass", 0, 2}}
		}
		// Baby turtles drop scute on growth, not on death
	}

	for _, d := range drops {
		count := d.minCount
		if d.maxCount > d.minCount {
			count += rand.Int31n(d.maxCount - d.minCount + 1)
		}
		// Looting bonus: add 0 to lootingLevel extra items
		if lootingLevel > 0 {
			count += rand.Int31n(lootingLevel + 1)
		}
		if count <= 0 {
			continue
		}
		// Cook meat if mob died while on fire
		itemName := d.name
		if mob.FireTicks > 0 {
			if cooked, ok := cookedMeats[itemName]; ok {
				itemName = cooked
			}
		}
		itemID := itemIDByName(itemName)
		if itemID <= 0 {
			continue
		}
		m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, count, 10)
	}
}

// despawnFarMobs removes hostile mobs using vanilla-like tiered despawn rules.
// Passive mobs never naturally despawn. Named and tamed mobs never despawn.
func (m *MobManager) despawnFarMobs() {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		// Never despawn tamed mobs
		if mob.TameData != nil && mob.TameData.Tamed {
			continue
		}
		// Never despawn named mobs (nametag)
		if mob.CustomName != "" {
			continue
		}
		// Passive mobs never naturally despawn (vanilla behavior)
		if !mob.Hostile {
			continue
		}

		// Find squared distance to nearest player
		minDistSq := math.MaxFloat64
		m.Manager.ForEach(func(p *game.Player) {
			px, _, pz := p.Position()
			dx := px - mob.X
			dz := pz - mob.Z
			distSq := dx*dx + dz*dz
			if distSq < minDistSq {
				minDistSq = distSq
			}
		})

		// Tiered despawn:
		// > 128 blocks: instant despawn
		// 32-128 blocks: ~22% chance per check (approximates vanilla 1/800 per tick over 200 ticks)
		// < 32 blocks: never despawn
		if minDistSq > 128*128 {
			toRemove = append(toRemove, eid)
		} else if minDistSq > 32*32 && rand.Float64() < 0.22 {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		m.removeMobEntity(m.Mobs[eid])
		m.Spatial.Remove(eid)
		delete(m.Mobs, eid)
	}
}

// sunlightDamage applies damage to zombies and skeletons in sunlight.
func (m *MobManager) sunlightDamage() {
	for _, mob := range m.Mobs {
		if mob.Health <= 0 {
			continue
		}
		if mob.TypeID != MobTypeZombie && mob.TypeID != MobTypeSkeleton {
			continue
		}
		mob.Health -= 1
		if mob.Health <= 0 {
			mob.Health = 0
			m.killMob(mob, nil)
		} else {
			// Broadcast hurt
			hurtPkt := pk.Marshal(
				packetid.ClientboundHurtAnimation,
				pk.VarInt(mob.EID),
				pk.Float(0),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(hurtPkt)
			})
			BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}
	}
}

// removeMobEntity sends RemoveEntities for a mob.
func (m *MobManager) removeMobEntity(mob *Mob) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(mob.EID),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
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

// broadcastSpawn sends AddEntity for a mob to all players.
func (m *MobManager) broadcastSpawn(mob *Mob) {
	id := uuid.New()
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(mob.EID),
		pk.UUID(id),
		pk.VarInt(mob.TypeID),
		pk.Double(mob.X),
		pk.Double(mob.Y),
		pk.Double(mob.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(degToAngle(mob.Pitch)),
		pk.Angle(degToAngle(mob.Yaw)),
		pk.Angle(degToAngle(mob.Yaw)), // head yaw
		pk.VarInt(0),                  // data
	)
	m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Send slime size metadata (index 16 = VarInt size)
	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
		var w MetadataWriter
		w.writeIndex(16, metaSerializerInt)
		writeVarIntBuf(&w.buf, mob.SlimeSize)
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}

	// Send baby metadata (AgeableMob index 16, Boolean)
	if mob.Baby && !isSlimeType(mob.TypeID) {
		var w MetadataWriter
		w.WriteBoolean(16, true) // isBaby = true
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}

	// Send tameable metadata
	if mob.TameData != nil && mob.TameData.Tamed {
		m.broadcastTameableMetadata(mob)
		// Send horse armor equipment
		if mob.TypeID == MobTypeHorse && mob.TameData.ArmorItemID > 0 {
			m.broadcastHorseEquipment(mob)
		}
		// Send llama metadata (chest, carpet)
		if isLlamaType(mob.TypeID) {
			m.broadcastLlamaMetadata(mob)
		}
	}

	// Send custom name metadata
	if mob.CustomName != "" {
		m.broadcastCustomName(mob)
	}

	// Send villager data metadata (profession, type, level)
	if mob.TypeID == MobTypeVillager && mob.VillagerData != nil {
		var w MetadataWriter
		w.WriteVillagerData(18, 0, villagerProfessionID(mob.VillagerData.Profession), 1)
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}
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

// SendExistingMobs sends all current mobs to a newly joined player.
func (m *MobManager) SendExistingMobs(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	px, _, pz := player.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange

	for _, mob := range m.Mobs {
		if mob.Health <= 0 || mob.InsideHive {
			continue
		}
		// Only send mobs within tracking range
		dx := mob.X - px
		dz := mob.Z - pz
		if dx*dx+dz*dz > r2 {
			continue
		}

		id := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(mob.EID),
			pk.UUID(id),
			pk.VarInt(mob.TypeID),
			pk.Double(mob.X),
			pk.Double(mob.Y),
			pk.Double(mob.Z),
			pk.UnsignedByte(0),
			pk.Angle(degToAngle(mob.Pitch)),
			pk.Angle(degToAngle(mob.Yaw)),
			pk.Angle(degToAngle(mob.Yaw)),
			pk.VarInt(0),
		))

		// Send head rotation
		player.WritePacket(pk.Marshal(
			packetid.ClientboundRotateHead,
			pk.VarInt(mob.EID),
			pk.Angle(degToAngle(mob.Yaw)),
		))

		// Send slime size metadata
		if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
			var w MetadataWriter
			w.writeIndex(16, metaSerializerInt)
			writeVarIntBuf(&w.buf, mob.SlimeSize)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}

		// Send baby metadata
		if mob.Baby && !isSlimeType(mob.TypeID) {
			var w MetadataWriter
			w.WriteBoolean(16, true)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}

		// Send villager data metadata
		if mob.TypeID == MobTypeVillager && mob.VillagerData != nil {
			var w MetadataWriter
			w.WriteVillagerData(18, 0, villagerProfessionID(mob.VillagerData.Profession), 1)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}
	}
}

// IsMob returns true if the entity ID belongs to a mob.
func (m *MobManager) IsMob(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Mobs[eid]
	return ok
}

// applyVillagerDamageGossip adds minor negative gossip when a villager is hit.
// Must be called with m.mu already held.
func (m *MobManager) applyVillagerDamageGossip(mob *Mob, attacker *game.Player) {
	if mob.TypeID != MobTypeVillager || mob.VillagerData == nil {
		return
	}
	mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipMinorNegative, attacker.UUID, 5, m.currentTick)
	m.spreadVillagerGossip(mob, GossipMinorNegative, attacker.UUID, 5)
}

// applyVillagerKillGossip adds major negative gossip when a villager is killed.
// Must be called with m.mu already held.
func (m *MobManager) applyVillagerKillGossip(mob *Mob, attacker *game.Player) {
	if mob.TypeID != MobTypeVillager || mob.VillagerData == nil {
		return
	}
	mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipMajorNegative, attacker.UUID, 25, m.currentTick)
	m.spreadVillagerGossip(mob, GossipMajorNegative, attacker.UUID, 25)
}

// spreadVillagerGossip propagates a gossip entry to nearby villagers within 16 blocks.
// Must be called with m.mu already held.
func (m *MobManager) spreadVillagerGossip(source *Mob, gtype string, playerUUID uuid.UUID, value int) {
	for _, other := range m.Mobs {
		if other.EID == source.EID || other.TypeID != MobTypeVillager || other.Health <= 0 || other.VillagerData == nil {
			continue
		}
		dx := other.X - source.X
		dz := other.Z - source.Z
		if dx*dx+dz*dz <= 16*16 {
			other.VillagerData.Gossip = AddGossip(other.VillagerData.Gossip, gtype, playerUUID, value/2, m.currentTick)
		}
	}
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

// slimeSplit spawns 2-4 smaller slimes when a slime dies.
func (m *MobManager) slimeSplit(mob *Mob) {
	var newSize int32
	switch mob.SlimeSize {
	case 4:
		newSize = 2
	case 2:
		newSize = 1
	default:
		return // small slimes don't split
	}

	count := 2 + rand.Int31n(3) // 2-4 smaller slimes
	for i := int32(0); i < count; i++ {
		var health float32
		var damage float32
		switch newSize {
		case 1:
			health, damage = 1, 0
		case 2:
			health, damage = 4, 2
		}

		babyX := mob.X + (rand.Float64()-0.5)*2
		babyZ := mob.Z + (rand.Float64()-0.5)*2
		eid := m.Manager.NextEntityID()
		baby := &Mob{
			EID:       eid,
			TypeID:    MobTypeSlime,
			X:         babyX,
			Y:         mob.Y,
			Z:         babyZ,
			PrevX:     babyX,
			PrevY:     mob.Y,
			PrevZ:     babyZ,
			Health:    health,
			MaxHealth: health,
			Damage:    damage,
			Speed:     0.1,
			WanderYaw: rand.Float32() * 360,
			Hostile:   true,
			SlimeSize: newSize,
		}
		m.Mobs[eid] = baby
		m.broadcastSpawn(baby)
	}
}

// trySpawnSlime attempts to spawn slimes (1% chance per 100 ticks).
func (m *MobManager) trySpawnSlime() {
	if rand.Float64() > 0.01 {
		return
	}

	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && p.GameMode == 0 {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	player := players[rand.Intn(len(players))]
	px, _, pz := player.Position()

	// Per-player cap check
	if m.countMobsNear(px, pz, 128*128, true) >= hostileCapPerPlayer {
		return
	}

	angle := rand.Float64() * 2 * math.Pi
	dist := 24 + rand.Float64()*24
	spawnX := px + math.Cos(angle)*dist
	spawnZ := pz + math.Sin(angle)*dist

	spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
	if spawnY < m.MinY {
		return
	}

	// Slimes spawn below Y=40 (simulating swamp/deep biome)
	if spawnY > 40 {
		return
	}

	// Pick random size
	var slimeSize int32
	var health, damage float32
	sizeRoll := rand.Float64()
	switch {
	case sizeRoll < 0.5:
		slimeSize, health, damage = 1, 1, 0
	case sizeRoll < 0.8:
		slimeSize, health, damage = 2, 4, 2
	default:
		slimeSize, health, damage = 4, 16, 4
	}

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    MobTypeSlime,
		X:         spawnX + 0.5,
		Y:         float64(spawnY),
		Z:         spawnZ + 0.5,
		PrevX:     spawnX + 0.5,
		PrevY:     float64(spawnY),
		PrevZ:     spawnZ + 0.5,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     0.1,
		WanderYaw: rand.Float32() * 360,
		Hostile:   true,
		SlimeSize: slimeSize,
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
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

// trySpawnPhantom spawns phantoms for players who haven't slept for 3+ in-game days.
func (m *MobManager) trySpawnPhantom(tick int64) {
	if !m.TimeMgr.IsNight() {
		return
	}

	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}

		// Check if player hasn't slept for 3+ in-game days (72000 ticks)
		if tick-p.LastSleepTick < 72000 {
			return
		}
		// 5% chance per check
		if rand.Float64() > 0.05 {
			return
		}

		px, py, pz := p.Position()

		// Per-player cap check
		if m.countMobsNear(px, pz, 128*128, true) >= hostileCapPerPlayer {
			return
		}
		spawnY := py + 20 + rand.Float64()*20
		spawnAngle := rand.Float64() * 2 * math.Pi
		spawnDist := 5 + rand.Float64()*10
		spawnX := px + math.Cos(spawnAngle)*spawnDist
		spawnZ := pz + math.Sin(spawnAngle)*spawnDist

		eid := m.Manager.NextEntityID()
		mob := &Mob{
			EID:         eid,
			TypeID:      MobTypePhantom,
			X:           spawnX,
			Y:           spawnY,
			Z:           spawnZ,
			PrevX:       spawnX,
			PrevY:       spawnY,
			PrevZ:       spawnZ,
			Health:      20,
			MaxHealth:   20,
			Damage:      6,
			Speed:       0.3,
			WanderYaw:   rand.Float32() * 360,
			Hostile:     true,
			SwoopPhase:  0,
			SwoopTick:   tick,
			FlyTargetY:  rand.Float64()*5 + 5,
			CircleAngle: rand.Float64() * 2 * math.Pi,
		}
		m.Mobs[eid] = mob
		m.broadcastSpawn(mob)
		BroadcastSound(m.Manager, SoundPhantomAmbient, SoundCategoryHostile, spawnX, spawnY, spawnZ, 1.0, 1.0)
	})
}
