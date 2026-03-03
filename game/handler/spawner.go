package handler

import (
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
)

// SpawnerConfig holds the configuration for a mob spawner block.
type SpawnerConfig struct {
	Pos            [3]int
	MobType        int32   // entity type ID to spawn
	SpawnRange     int     // horizontal spawn radius (default 4)
	SpawnCount     int     // mobs per activation (default 4)
	MaxNearby      int     // max mobs of this type nearby before stopping (default 6)
	RequiredPlayer float64 // max player distance to activate (default 16)
	MinDelay       int     // min delay between spawns in ticks (default 200)
	MaxDelay       int     // max delay between spawns in ticks (default 800)
	CurrentDelay   int     // current countdown ticks
}

// SpawnerManager manages mob spawner blocks in the world.
type SpawnerManager struct {
	MobMgr   *MobManager
	Manager  *game.PlayerManager
	World    game.World
	mu       sync.Mutex
	Spawners map[[3]int]*SpawnerConfig
}

// NewSpawnerManager creates a new SpawnerManager.
func NewSpawnerManager(mobMgr *MobManager, manager *game.PlayerManager, world game.World) *SpawnerManager {
	return &SpawnerManager{
		MobMgr:   mobMgr,
		Manager:  manager,
		World:    world,
		Spawners: make(map[[3]int]*SpawnerConfig),
	}
}

// Register adds a spawner at the given position.
func (sm *SpawnerManager) Register(x, y, z int, mobType int32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.Spawners[[3]int{x, y, z}] = &SpawnerConfig{
		Pos:            [3]int{x, y, z},
		MobType:        mobType,
		SpawnRange:     4,
		SpawnCount:     4,
		MaxNearby:      6,
		RequiredPlayer: 16,
		MinDelay:       200,
		MaxDelay:       800,
		CurrentDelay:   200 + rand.Intn(600),
	}
}

// Remove removes a spawner at the given position.
func (sm *SpawnerManager) Remove(x, y, z int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.Spawners, [3]int{x, y, z})
}

// Tick processes all spawners.
func (sm *SpawnerManager) Tick(tick int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, spawner := range sm.Spawners {
		sm.tickSpawner(spawner)
	}
}

// tickSpawner processes a single spawner.
func (sm *SpawnerManager) tickSpawner(spawner *SpawnerConfig) {
	sx, sy, sz := float64(spawner.Pos[0])+0.5, float64(spawner.Pos[1])+0.5, float64(spawner.Pos[2])+0.5

	// Check if any player is in range
	playerInRange := false
	sm.Manager.ForEach(func(p *game.Player) {
		px, py, pz := p.Position()
		dx := px - sx
		dy := py - sy
		dz := pz - sz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist <= spawner.RequiredPlayer {
			playerInRange = true
		}
	})
	if !playerInRange {
		return
	}

	// Count down
	spawner.CurrentDelay--
	if spawner.CurrentDelay > 0 {
		return
	}

	// Reset delay
	spawner.CurrentDelay = spawner.MinDelay + rand.Intn(spawner.MaxDelay-spawner.MinDelay+1)

	// Count nearby mobs of this type
	nearbyCount := 0
	sm.MobMgr.mu.Lock()
	for _, mob := range sm.MobMgr.Mobs {
		if mob.TypeID != spawner.MobType {
			continue
		}
		dx := mob.X - sx
		dy := mob.Y - sy
		dz := mob.Z - sz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist <= float64(spawner.SpawnRange)*2 {
			nearbyCount++
		}
	}
	sm.MobMgr.mu.Unlock()

	if nearbyCount >= spawner.MaxNearby {
		return
	}

	// Spawn mobs
	spawned := 0
	for attempt := 0; attempt < spawner.SpawnCount*3 && spawned < spawner.SpawnCount; attempt++ {
		// Random position within spawn range
		spX := sx + float64(rand.Intn(spawner.SpawnRange*2+1)-spawner.SpawnRange)
		spZ := sz + float64(rand.Intn(spawner.SpawnRange*2+1)-spawner.SpawnRange)
		spY := sy - 1 // spawn at spawner level

		// Basic check: must have solid ground below
		groundState, err := sm.World.GetBlock(int(math.Floor(spX)), int(spY)-1, int(math.Floor(spZ)))
		if err != nil || groundState == 0 {
			continue
		}

		mob := sm.MobMgr.createMobOfType(spawner.MobType, spX, spY, spZ)
		if mob == nil {
			continue
		}

		sm.MobMgr.mu.Lock()
		sm.MobMgr.Mobs[mob.EID] = mob
		sm.MobMgr.mu.Unlock()
		sm.MobMgr.broadcastSpawn(mob)

		spawned++
	}
}

// createMobOfType creates a mob of the given type at the specified position.
// Returns nil if the mob type is not supported.
func (m *MobManager) createMobOfType(typeID int32, x, y, z float64) *Mob {
	mob := &Mob{
		EID:    m.Manager.NextEntityID(),
		TypeID: typeID,
		X:      x,
		Y:      y,
		Z:      z,
		PrevX:  x,
		PrevY:  y,
		PrevZ:  z,
		Yaw:    float32(rand.Intn(360)),
		Speed:  0.23,
	}

	switch typeID {
	case MobTypeZombie:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 3
		mob.Hostile = true
	case MobTypeSkeleton:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 2
		mob.Hostile = true
		mob.ShootCooldown = 40
	case MobTypeSpider:
		mob.Health = 16
		mob.MaxHealth = 16
		mob.Damage = 2
		mob.Hostile = true
		mob.Speed = 0.3
	case MobTypeCaveSpider:
		mob.Health = 12
		mob.MaxHealth = 12
		mob.Damage = 2
		mob.Hostile = true
		mob.Speed = 0.3
	case MobTypeCreeper:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 0
		mob.Hostile = true
		mob.FuseDuration = 30
	case MobTypeBlaze:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 6
		mob.Hostile = true
	case MobTypeSilverfish:
		mob.Health = 8
		mob.MaxHealth = 8
		mob.Damage = 1
		mob.Hostile = true
		mob.Speed = 0.25
	case MobTypeGhast:
		mob.Health = 10
		mob.MaxHealth = 10
		mob.Damage = 6
		mob.Hostile = true
		mob.ShootCooldown = 60
		mob.FlyTargetY = y + 10
	case MobTypeGuardian:
		mob.Health = 30
		mob.MaxHealth = 30
		mob.Damage = 6
		mob.Hostile = true
		mob.ShootCooldown = 80
	case MobTypeElderGuardian:
		mob.Health = 80
		mob.MaxHealth = 80
		mob.Damage = 8
		mob.Hostile = true
		mob.ShootCooldown = 80
	case MobTypeIronGolem:
		mob.Health = 100
		mob.MaxHealth = 100
		mob.Damage = 10
		mob.Hostile = false
		mob.Speed = 0.25
	case MobTypeDrowned:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 3
		mob.Hostile = true
		mob.ShootCooldown = 60
	case MobTypeHusk:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 3
		mob.Hostile = true
	case MobTypeStray:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 2
		mob.Hostile = true
		mob.ShootCooldown = 40
	case MobTypeMagmaCube:
		mob.Health = 16
		mob.MaxHealth = 16
		mob.Damage = 6
		mob.Hostile = true
		mob.SlimeSize = 4
	case MobTypePiglin:
		mob.Health = 16
		mob.MaxHealth = 16
		mob.Damage = 5
		mob.Hostile = true
	case MobTypeZombifiedPiglin:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 5
		mob.Hostile = false // neutral until attacked
	case MobTypeHoglin:
		mob.Health = 40
		mob.MaxHealth = 40
		mob.Damage = 6
		mob.Hostile = true
	case MobTypeWitherSkeleton:
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 8
		mob.Hostile = true
		mob.Speed = 0.25
	case MobTypeShulker:
		mob.Health = 30
		mob.MaxHealth = 30
		mob.Damage = 4
		mob.Hostile = true
		mob.ShootCooldown = 80
	case MobTypePillager:
		mob.Health = 24
		mob.MaxHealth = 24
		mob.Damage = 4
		mob.Hostile = true
		mob.ShootCooldown = 50
	case MobTypeVindicator:
		mob.Health = 24
		mob.MaxHealth = 24
		mob.Damage = 13
		mob.Hostile = true
		mob.Speed = 0.35
	case MobTypeEvoker:
		mob.Health = 24
		mob.MaxHealth = 24
		mob.Damage = 6
		mob.Hostile = true
		mob.ShootCooldown = 100
	case MobTypeVex:
		mob.Health = 14
		mob.MaxHealth = 14
		mob.Damage = 9
		mob.Hostile = true
		mob.Speed = 0.3
	case MobTypeRavager:
		mob.Health = 100
		mob.MaxHealth = 100
		mob.Damage = 12
		mob.Hostile = true
		mob.Speed = 0.3
	case MobTypeEndermite:
		mob.Health = 8
		mob.MaxHealth = 8
		mob.Damage = 2
		mob.Hostile = true
		mob.Speed = 0.25
	case MobTypeBee:
		mob.Health = 10
		mob.MaxHealth = 10
		mob.Damage = 2
		mob.Hostile = false
		mob.FlyTargetY = y + 2
	default:
		// Generic hostile mob
		mob.Health = 20
		mob.MaxHealth = 20
		mob.Damage = 3
		mob.Hostile = true
	}

	return mob
}
