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

	MobTypeCow     int32 = 19
	MobTypePig     int32 = 73
	MobTypeSheep   int32 = 83
	MobTypeChicken int32 = 16
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

	// Villager data (nil for non-villagers)
	VillagerData *VillagerData
}

// MobManager handles mob spawning, AI, and lifecycle.
type MobManager struct {
	Manager      *game.PlayerManager
	TimeMgr      *TimeManager
	World        game.World
	MinY         int
	Survival     *SurvivalHandler
	ItemEntities *ItemEntityManager
	ArrowMgr     *ArrowManager
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

	// Pick mob type: 50% zombie, 25% skeleton, 15% spider, 10% creeper
	var typeID int32
	var health, damage float32
	var speed float64
	roll := rand.Float64()
	switch {
	case roll < 0.5:
		typeID, health, damage, speed = MobTypeZombie, 20, 3, 0.115
	case roll < 0.75:
		typeID, health, damage, speed = MobTypeSkeleton, 20, 2, 0.1
	case roll < 0.9:
		typeID, health, damage, speed = MobTypeSpider, 16, 2, 0.15
	default:
		typeID, health, damage, speed = MobTypeCreeper, 20, 0, 0.1
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

	// Pick mob type: 25% cow, 25% pig, 18% sheep, 18% chicken, 14% villager
	var typeID int32
	var health float32
	var vdata *VillagerData
	roll := rand.Float64()
	switch {
	case roll < 0.25:
		typeID, health = MobTypeCow, 10
	case roll < 0.50:
		typeID, health = MobTypePig, 10
	case roll < 0.68:
		typeID, health = MobTypeSheep, 8
	case roll < 0.86:
		typeID, health = MobTypeChicken, 4
	default:
		typeID, health = MobTypeVillager, 20
		vdata = NewVillagerData(RandomProfession())
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
	case mob.TypeID == MobTypeSkeleton && mob.Hostile:
		m.tickSkeleton(mob, tick)
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
		// Move toward target
		px, _, pz := nearest.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist > 1.5 {
			nx := dx / dist * mob.Speed
			nz := dz / dist * mob.Speed
			m.tryMove(mob, nx, nz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}

		// Attack if within range
		if nearestDist <= 1.5 && mob.AttackCooldown <= 0 && mob.Damage > 0 {
			mob.AttackCooldown = 30 // 1.5 seconds
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
		}

		m.broadcastMoveEntity(mob)
	} else {
		mob.Target = nil
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
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest

	// Move toward target
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.5 {
		nx := dx / dist * mob.Speed
		nz := dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
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
	return ok && !mob.Hostile
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

// despawnFarMobs removes mobs too far from any player.
func (m *MobManager) despawnFarMobs() {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 {
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
	}
}

// IsMob returns true if the entity ID belongs to a mob.
func (m *MobManager) IsMob(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Mobs[eid]
	return ok
}
