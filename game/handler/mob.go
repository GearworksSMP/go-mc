package handler

import (
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Mob type IDs (entity type for AddEntity packet).
const (
	MobTypeZombie   int32 = 120
	MobTypeSkeleton int32 = 87
	MobTypeCreeper  int32 = 20
	MobTypeSpider   int32 = 96

	MobTypeEnderman int32 = 30
	MobTypeWitch    int32 = 114
	MobTypeSlime    int32 = 89
	MobTypePhantom  int32 = 72

	MobTypeCow     int32 = 19
	MobTypePig     int32 = 73
	MobTypeSheep   int32 = 83
	MobTypeChicken int32 = 16

	// New mob types
	MobTypeBlaze            int32 = 5
	MobTypeGhast            int32 = 42
	MobTypeIronGolem        int32 = 51
	MobTypeSnowGolem        int32 = 91
	MobTypeGuardian         int32 = 44
	MobTypeElderGuardian    int32 = 27
	MobTypeDrowned          int32 = 24
	MobTypeHusk             int32 = 48
	MobTypeStray            int32 = 99
	MobTypeCaveSpider       int32 = 13
	MobTypeSilverfish       int32 = 86
	MobTypeEndermite        int32 = 31
	MobTypeMagmaCube        int32 = 62
	MobTypePiglin           int32 = 74
	MobTypeZombifiedPiglin  int32 = 121
	MobTypeHoglin           int32 = 46
	MobTypeStrider          int32 = 100
	MobTypeWitherSkeleton   int32 = 116
	MobTypeShulker          int32 = 84
	MobTypePillager         int32 = 75
	MobTypeVindicator       int32 = 107
	MobTypeEvoker           int32 = 32
	MobTypeVex              int32 = 106
	MobTypeRavager          int32 = 80
	MobTypeBee              int32 = 4
	MobTypeFox              int32 = 40
	MobTypeRabbit           int32 = 79
	MobTypeBat              int32 = 3
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

	// Creeper fuse
	FuseStart    int64 // tick when fuse started (0 = not fusing)
	FuseDuration int64 // ticks until explosion (30 = 1.5s)

	// Skeleton ranged attack
	ShootCooldown int64

	// Passive mob AI
	FleeX, FleeZ float64 // flee target position
	FleeTicks     int64   // ticks remaining to flee
	LoveTicks     int64   // ticks remaining in "love" mode (breeding)
	BreedCooldown int64   // ticks until can breed again

	// Enderman
	TeleportCooldown int64

	// Slime
	SlimeSize int32 // 1=small, 2=medium, 4=large

	// Phantom
	FlyTargetY float64
	SwoopPhase int32  // 0=circling, 1=diving
	SwoopTick  int64  // tick when current phase started
	CircleAngle float64 // current angle around target for circling

	// Pathfinding
	Path          []PathStep // computed A* path
	PathIndex     int        // current step along path
	PathRecalcTick int64     // tick when path was last recalculated

	// Villager data (nil for non-villagers)
	VillagerData *VillagerData

	// Tameable mob data (nil for non-tameable)
	TameData *TameableMobData
}

// MobManager handles mob spawning, AI, and lifecycle.
type MobManager struct {
	Manager      *game.PlayerManager
	TimeMgr      *TimeManager
	WeatherMgr   *WeatherManager
	World        game.World
	MinY         int
	Survival     *SurvivalHandler
	ItemEntities *ItemEntityManager
	ArrowMgr     *ArrowManager
	AdvMgr       *AdvancementManager
	mu           sync.Mutex
	Mobs         map[int32]*Mob
	maxMobs      int
	maxPassive   int
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
		maxMobs:      20,
		maxPassive:   15,
	}
}

// Tick processes mob spawning, AI, and despawning.
func (m *MobManager) Tick(tick int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Spawn hostile mobs every 100 ticks (5 seconds)
	if tick%100 == 0 {
		m.trySpawn()
		m.trySpawnSlime()
	}

	// Spawn phantoms every 200 ticks (10 seconds)
	if tick%200 == 0 {
		m.trySpawnPhantom(tick)
	}

	// Spawn passive mobs every 200 ticks (10 seconds)
	if tick%200 == 0 {
		m.trySpawnPassive()
	}

	// AI tick for each mob
	for _, mob := range m.Mobs {
		m.tickMob(mob, tick)
	}

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

// trySpawn attempts to spawn hostile mobs near players at night.
func (m *MobManager) trySpawn() {
	if !m.TimeMgr.IsNight() {
		return
	}
	hostileCount := 0
	for _, mob := range m.Mobs {
		if mob.Hostile {
			hostileCount++
		}
	}
	if hostileCount >= m.maxMobs {
		return
	}

	// Pick a random player
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

	// Pick random position 24-48 blocks from player
	angle := rand.Float64() * 2 * math.Pi
	dist := 24 + rand.Float64()*24
	spawnX := px + math.Cos(angle)*dist
	spawnZ := pz + math.Sin(angle)*dist

	// Find surface Y
	spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
	if spawnY < m.MinY {
		return
	}

	// Pick mob type: 40% zombie, 20% skeleton, 15% spider, 10% creeper, 5% enderman, 5% witch, 5% spider(extra)
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

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    typeID,
		X:         spawnX + 0.5,
		Y:         float64(spawnY),
		Z:         spawnZ + 0.5,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     speed,
		WanderYaw: rand.Float32() * 360,
		Hostile:   true,
	}
	m.Mobs[eid] = mob

	// Broadcast spawn to all players
	m.broadcastSpawn(mob)
}

// trySpawnPassive attempts to spawn passive mobs on grass during daytime.
func (m *MobManager) trySpawnPassive() {
	if m.TimeMgr.IsNight() {
		return
	}
	passiveCount := 0
	for _, mob := range m.Mobs {
		if !mob.Hostile {
			passiveCount++
		}
	}
	if passiveCount >= m.maxPassive {
		return
	}

	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	player := players[rand.Intn(len(players))]
	px, _, pz := player.Position()

	// Pick random position 8-32 blocks from player
	angle := rand.Float64() * 2 * math.Pi
	dist := 8 + rand.Float64()*24
	spawnX := px + math.Cos(angle)*dist
	spawnZ := pz + math.Sin(angle)*dist

	// Find surface Y
	spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
	if spawnY < m.MinY {
		return
	}

	// Check that the block below is grass_block
	belowState, err := m.World.GetBlock(int(spawnX), spawnY-1, int(spawnZ))
	if err != nil {
		return
	}
	blockName := BlockNameFromState(int(belowState))
	if blockName != "grass_block" {
		return
	}

	// Pick mob type: 20% cow, 20% pig, 14% sheep, 14% chicken, 8% villager, 8% wolf, 6% cat, 6% horse, 4% parrot
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
	default:
		typeID, health = MobTypeParrot, 6
	}

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:          eid,
		TypeID:       typeID,
		X:            spawnX + 0.5,
		Y:            float64(spawnY),
		Z:            spawnZ + 0.5,
		Health:       health,
		MaxHealth:    health,
		Damage:       0,
		Speed:        0.1,
		WanderYaw:    rand.Float32() * 360,
		Hostile:      false,
		VillagerData: vdata,
		TameData:     tdata,
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
	if mob.Health <= 0 {
		return
	}

	if mob.AttackCooldown > 0 {
		mob.AttackCooldown--
	}

	// Dispatch to specialized AI
	switch {
	case mob.TypeID == MobTypeCreeper:
		m.tickCreeper(mob, tick)
		return
	case mob.TypeID == MobTypeSkeleton:
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
	case !mob.Hostile:
		m.tickPassive(mob, tick)
		return
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

// applyGravity applies simple gravity to a mob.
func (m *MobManager) applyGravity(mob *Mob) {
	bx := int(math.Floor(mob.X))
	by := int(math.Floor(mob.Y))
	bz := int(math.Floor(mob.Z))
	below, err := m.World.GetBlock(bx, by-1, bz)
	if err != nil {
		return
	}
	if !isSolidBlock(below) {
		mob.Y -= 0.1
	}
}

// tickHostile runs standard melee hostile AI (zombie, spider).
func (m *MobManager) tickHostile(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Find nearest player within 32 blocks
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
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
		}

		m.broadcastMoveEntity(mob)
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

	m.broadcastMoveEntity(mob)

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
				m.Survival.ApplyDamage(m.Manager, p, damage, m.Survival.AttackDamageTypeID)
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

	// Maintain 5-10 block distance
	if dist < 5.0 {
		// Back up — direct movement (no pathfinding needed for retreating)
		nx := -dx / dist * mob.Speed
		nz := -dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	} else if dist > 10.0 {
		// Move closer using pathfinding
		m.moveWithPathfinding(mob, nearest, tick)
	}
	m.broadcastMoveEntity(mob)

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
			m.broadcastMoveEntity(mob)
			return
		}
		mob.FleeTicks = 0
	}

	// Love mode countdown
	if mob.LoveTicks > 0 {
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
			Health:    mob.MaxHealth / 2,
			MaxHealth: mob.MaxHealth,
			Speed:     mob.Speed,
			WanderYaw: rand.Float32() * 360,
			Hostile:   false,
		}
		m.Mobs[eid] = baby
		m.broadcastSpawn(baby)

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

// tickWander makes a mob wander randomly.
func (m *MobManager) tickWander(mob *Mob, tick int64) {
	if tick-mob.WanderTick > int64(40+rand.Intn(40)) {
		mob.WanderTick = tick
		mob.WanderYaw += (rand.Float32() - 0.5) * 90
	}
	rad := float64(mob.WanderYaw) * math.Pi / 180
	nx := math.Cos(rad) * mob.Speed * 0.3
	nz := math.Sin(rad) * mob.Speed * 0.3
	if !m.tryMove(mob, nx, nz) {
		// Blocked: pick a new direction
		mob.WanderYaw += (rand.Float32()-0.5)*180 + 90
	}
	mob.Yaw = mob.WanderYaw
	m.broadcastMoveEntity(mob)
}

// moveWithPathfinding moves a mob toward a target player using A* pathfinding.
// Recalculates path every 20 ticks or when the target has moved significantly.
// Falls back to direct-line movement if no path is found.
func (m *MobManager) moveWithPathfinding(mob *Mob, target *game.Player, tick int64) {
	px, py, pz := target.Position()

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
			nx := dx / dist * mob.Speed
			nz := dz / dist * mob.Speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
	} else {
		// Fallback: direct-line movement
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.5 {
			nx := dx / dist * mob.Speed
			nz := dz / dist * mob.Speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
	}
}

// DamageMob applies damage to a mob from a player attack.
// Returns true if the mob was found and damaged.
func (m *MobManager) DamageMob(attacker *game.Player, targetEID int32, damage float32) bool {
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
		pk.VarInt(m.Survival.AttackDamageTypeID),
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

	if mob.Health <= 0 {
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
		pk.VarInt(m.Survival.AttackDamageTypeID),
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
			m.broadcastMoveEntity(mob)
		}
	}

	// Fire Aspect: set mob on fire metadata
	if fireAspectLevel > 0 {
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pk.Marshal(
				packetid.ClientboundSetEntityData,
				pk.VarInt(mob.EID),
				pk.UnsignedByte(0),
				pk.VarInt(0),
				pk.Byte(0x01), // on fire
				pk.UnsignedByte(0xFF),
			))
		})
		// Apply 1 fire tick damage immediately
		mob.Health -= 1.0
		if mob.Health < 0 {
			mob.Health = 0
		}
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

	if mob.Health <= 0 {
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
	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

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

	go func() {
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(mob.EID),
		)
		m.mu.Lock()
		delete(m.Mobs, mob.EID)
		m.mu.Unlock()
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})
	}()

	if killer != nil {
		if mob.Hostile {
			AddExperience(killer, 5)
		} else {
			AddExperience(killer, int32(1+rand.Intn(3)))
		}
	}
}

// killMob handles mob death: animation, removal, drops, XP.
func (m *MobManager) killMob(mob *Mob, killer *game.Player) {
	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

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

	// Remove after brief delay (use goroutine for simplicity)
	go func() {
		// Remove entity
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(mob.EID),
		)
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})

		m.mu.Lock()
		delete(m.Mobs, mob.EID)
		m.mu.Unlock()
	}()

	// Advancement check
	if killer != nil && m.AdvMgr != nil {
		m.AdvMgr.CheckMobKill(killer, mob.TypeID)
	}

	// Award XP to killer
	if killer != nil {
		if mob.Hostile {
			AddExperience(killer, 5)
		} else {
			AddExperience(killer, int32(1+rand.Intn(3)))
		}
	}
}

// dropMobLoot drops item entities for a killed mob.
func (m *MobManager) dropMobLoot(mob *Mob) {
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
		drops = []drop{
			{"beef", 1, 3},
			{"leather", 0, 2},
		}
	case MobTypePig:
		drops = []drop{
			{"porkchop", 1, 3},
		}
	case MobTypeSheep:
		drops = []drop{
			{"white_wool", 1, 1},
		}
	case MobTypeChicken:
		drops = []drop{
			{"chicken", 1, 1},
			{"feather", 0, 2},
		}
	case MobTypeZombie:
		drops = []drop{
			{"rotten_flesh", 0, 2},
		}
	case MobTypeSkeleton:
		drops = []drop{
			{"bone", 0, 2},
			{"arrow", 0, 2},
		}
	case MobTypeVillager:
		drops = []drop{
			{"emerald", 0, 1},
		}
	case MobTypeEnderman:
		if rand.Float64() < 0.5 {
			drops = []drop{{"ender_pearl", 1, 1}}
		}
	case MobTypeWitch:
		if rand.Float64() < 0.33 {
			drops = append(drops, drop{"glass_bottle", 0, 2})
		}
		if rand.Float64() < 0.33 {
			drops = append(drops, drop{"redstone", 0, 2})
		}
		if rand.Float64() < 0.33 {
			drops = append(drops, drop{"glowstone_dust", 0, 2})
		}
	case MobTypeSlime:
		if mob.SlimeSize <= 1 {
			drops = []drop{{"slime_ball", 0, 2}}
		}
		// Medium and large slimes don't drop items (they split instead)
	case MobTypePhantom:
		if rand.Float64() < 0.5 {
			drops = []drop{{"phantom_membrane", 1, 1}}
		}
	case MobTypeWolf:
		// Wolves don't drop items
	case MobTypeCat:
		if rand.Float64() < 0.5 {
			drops = []drop{{"string", 0, 2}}
		}
	case MobTypeHorse:
		drops = []drop{{"leather", 0, 2}}
	case MobTypeParrot:
		drops = []drop{{"feather", 1, 2}}
	}

	for _, d := range drops {
		count := d.minCount
		if d.maxCount > d.minCount {
			count += rand.Int31n(d.maxCount - d.minCount + 1)
		}
		if count <= 0 {
			continue
		}
		itemID := itemIDByName(d.name)
		if itemID <= 0 {
			continue
		}
		m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, count, 10)
	}
}

// dropMobLootWithLooting drops loot with Looting enchantment bonus.
// Each Looting level adds 0-1 extra items per drop.
func (m *MobManager) dropMobLootWithLooting(mob *Mob, lootingLevel int32) {
	if lootingLevel <= 0 {
		m.dropMobLoot(mob)
		return
	}
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
		drops = []drop{{"white_wool", 1, 1}}
	case MobTypeChicken:
		drops = []drop{{"chicken", 1, 1}, {"feather", 0, 2}}
	case MobTypeZombie:
		drops = []drop{{"rotten_flesh", 0, 2}}
	case MobTypeSkeleton:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
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
	case MobTypeDrowned:
		drops = []drop{{"rotten_flesh", 0, 2}}
	case MobTypeHusk:
		drops = []drop{{"rotten_flesh", 0, 2}}
	case MobTypeStray:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
	case MobTypeCaveSpider:
		drops = []drop{{"string", 0, 2}, {"spider_eye", 0, 1}}
	case MobTypeMagmaCube:
		drops = []drop{{"magma_cream", 0, 1}}
	case MobTypePiglin:
		drops = []drop{{"gold_ingot", 0, 1}}
	case MobTypeHoglin:
		drops = []drop{{"porkchop", 2, 4}, {"leather", 0, 2}}
	case MobTypePillager:
		drops = []drop{{"arrow", 0, 2}}
	case MobTypeVindicator:
		drops = []drop{{"emerald", 0, 1}}
	case MobTypeEvoker:
		drops = []drop{{"totem_of_undying", 1, 1}}
	case MobTypeRabbit:
		drops = []drop{{"rabbit", 0, 1}, {"rabbit_hide", 0, 1}}
	case MobTypeIronGolem:
		drops = []drop{{"iron_ingot", 3, 5}, {"poppy", 0, 2}}
	case MobTypeSnowGolem:
		drops = []drop{{"snowball", 0, 15}}
	default:
		m.dropMobLoot(mob)
		return
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
		itemID := itemIDByName(d.name)
		if itemID <= 0 {
			continue
		}
		m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, count, 10)
	}
}

// despawnFarMobs removes mobs too far from any player.
func (m *MobManager) despawnFarMobs() {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 {
			continue
		}
		// Never despawn tamed mobs
		if mob.TameData != nil && mob.TameData.Tamed {
			continue
		}
		nearPlayer := false
		m.Manager.ForEach(func(p *game.Player) {
			if nearPlayer {
				return
			}
			px, py, pz := p.Position()
			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			if dx*dx+dy*dy+dz*dz < 128*128 {
				nearPlayer = true
			}
		})
		if !nearPlayer {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		m.removeMobEntity(m.Mobs[eid])
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
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Send slime size metadata (index 16 = VarInt size)
	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
		var w MetadataWriter
		w.writeIndex(16, metaSerializerInt)
		writeVarIntBuf(&w.buf, mob.SlimeSize)
		data := w.Bytes()
		m.Manager.ForEach(func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}

	// Send tameable metadata
	if mob.TameData != nil && mob.TameData.Tamed {
		m.broadcastTameableMetadata(mob)
	}
}

// broadcastMoveEntity sends a teleport update for a mob.
func (m *MobManager) broadcastMoveEntity(mob *Mob) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(mob.EID),
		pk.Double(mob.X),
		pk.Double(mob.Y),
		pk.Double(mob.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(mob.Yaw),
		pk.Float(mob.Pitch),
		pk.Boolean(true), // on ground
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SendExistingMobs sends all current mobs to a newly joined player.
func (m *MobManager) SendExistingMobs(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mob := range m.Mobs {
		if mob.Health <= 0 {
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

		// Send slime size metadata
		if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
			var w MetadataWriter
			w.writeIndex(16, metaSerializerInt)
			writeVarIntBuf(&w.buf, mob.SlimeSize)
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
			m.Survival.ApplyDamage(m.Manager, mob.Target, mob.Damage, m.Survival.AttackDamageTypeID)
		}

		m.broadcastMoveEntity(mob)
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
			m.broadcastMoveEntity(mob)
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
	m.broadcastMoveEntity(mob)

	// Attack every 60 ticks when target in range (10 blocks)
	if nearestDist <= 10.0 && mob.ShootCooldown <= 0 {
		mob.ShootCooldown = 60 // 3 seconds
		nearest.LastDamageMessage = nearest.Name + " was killed by Witch"
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
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
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
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

	m.broadcastMoveEntity(mob)
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

		eid := m.Manager.NextEntityID()
		baby := &Mob{
			EID:       eid,
			TypeID:    MobTypeSlime,
			X:         mob.X + (rand.Float64()-0.5)*2,
			Y:         mob.Y,
			Z:         mob.Z + (rand.Float64()-0.5)*2,
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
	hostileCount := 0
	for _, mob := range m.Mobs {
		if mob.Hostile {
			hostileCount++
		}
	}
	if hostileCount >= m.maxMobs {
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
		m.broadcastMoveEntity(mob)
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
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
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

	m.broadcastMoveEntity(mob)
}

// trySpawnPhantom spawns phantoms for players who haven't slept for 3+ in-game days.
func (m *MobManager) trySpawnPhantom(tick int64) {
	if !m.TimeMgr.IsNight() {
		return
	}
	hostileCount := 0
	for _, mob := range m.Mobs {
		if mob.Hostile {
			hostileCount++
		}
	}
	if hostileCount >= m.maxMobs {
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
