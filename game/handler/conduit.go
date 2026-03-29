package handler

import (
	"log"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// EffectConduitPower is defined in effect.go

// ConduitManager tracks conduit block positions and applies Conduit Power effects.
type ConduitManager struct {
	Manager   *game.PlayerManager
	World     game.World
	EffectMgr *EffectManager
	MobMgr    *MobManager
	Logger    *log.Logger

	mu       sync.Mutex
	conduits map[[3]int]struct{} // tracked conduit positions
}

// NewConduitManager creates a new ConduitManager.
func NewConduitManager(manager *game.PlayerManager, world game.World, effectMgr *EffectManager, mobMgr *MobManager, logger *log.Logger) *ConduitManager {
	return &ConduitManager{
		Manager:   manager,
		World:     world,
		EffectMgr: effectMgr,
		MobMgr:    mobMgr,
		Logger:    logger,
		conduits:  make(map[[3]int]struct{}),
	}
}

// RegisterConduit tracks a conduit block placement.
func (cm *ConduitManager) RegisterConduit(x, y, z int) {
	cm.mu.Lock()
	cm.conduits[[3]int{x, y, z}] = struct{}{}
	cm.mu.Unlock()
}

// UnregisterConduit removes a tracked conduit (e.g., when broken).
func (cm *ConduitManager) UnregisterConduit(x, y, z int) {
	cm.mu.Lock()
	delete(cm.conduits, [3]int{x, y, z})
	cm.mu.Unlock()
}

// Tick runs conduit logic every 40 ticks.
func (cm *ConduitManager) Tick(tick int64) {
	if tick%40 != 0 {
		return
	}

	cm.mu.Lock()
	positions := make([][3]int, 0, len(cm.conduits))
	for pos := range cm.conduits {
		positions = append(positions, pos)
	}
	cm.mu.Unlock()

	for _, pos := range positions {
		// Verify the conduit still exists
		if !cm.isConduitBlock(pos[0], pos[1], pos[2]) {
			cm.UnregisterConduit(pos[0], pos[1], pos[2])
			continue
		}
		cm.activateConduit(pos[0], pos[1], pos[2])
	}
}

func (cm *ConduitManager) isConduitBlock(x, y, z int) bool {
	state, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) < len(block.StateList) && block.StateList[state] != nil {
		_, ok := block.StateList[state].(block.Conduit)
		return ok
	}
	return false
}

// activateConduit checks the prismarine frame and applies effects if valid.
func (cm *ConduitManager) activateConduit(cx, cy, cz int) {
	prismarineCount := cm.countPrismarineFrame(cx, cy, cz)
	if prismarineCount < 16 {
		return
	}

	rings := prismarineCount / 7
	if rings < 1 {
		rings = 1
	}
	effectRange := float64(rings) * 32.0
	if effectRange > 96.0 {
		effectRange = 96.0
	}
	effectRangeSq := effectRange * effectRange

	condX := float64(cx) + 0.5
	condY := float64(cy) + 0.5
	condZ := float64(cz) + 0.5

	cm.Manager.ForEach(func(p *game.Player) {
		if p.Dead {
			return
		}
		px, py, pz := p.Position()
		distSq := sqDist3(px-condX, py-condY, pz-condZ)
		if distSq <= effectRangeSq {
			cm.EffectMgr.ApplyEffect(p, EffectConduitPower, 0, 260, true)
		}
	})

	if cm.MobMgr != nil {
		cm.damageNearbyHostiles(condX, condY, condZ)
	}
}

// countPrismarineFrame counts valid prismarine-type blocks in the 5x5x5 shell
// around the conduit (excluding the inner 3x3x3 core).
func (cm *ConduitManager) countPrismarineFrame(cx, cy, cz int) int {
	count := 0
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			for dz := -2; dz <= 2; dz++ {
				if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 && dz >= -1 && dz <= 1 {
					continue
				}
				if cm.isPrismarineType(cx+dx, cy+dy, cz+dz) {
					count++
				}
			}
		}
	}
	return count
}

func (cm *ConduitManager) isPrismarineType(x, y, z int) bool {
	state, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return false
	}
	switch block.StateList[state].(type) {
	case block.Prismarine, block.PrismarineBricks, block.DarkPrismarine, block.SeaLantern:
		return true
	}
	return false
}

// damageNearbyHostiles deals 4 damage to hostile mobs within 8 blocks of the conduit.
func (cm *ConduitManager) damageNearbyHostiles(cx, cy, cz float64) {
	cm.MobMgr.mu.Lock()
	defer cm.MobMgr.mu.Unlock()

	for _, mob := range cm.MobMgr.Mobs {
		if !mob.Hostile || mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		if sqDist3(mob.X-cx, mob.Y-cy, mob.Z-cz) <= 64.0 { // 8^2
			mob.Health -= 4.0
			if mob.Health <= 0 {
				mob.Health = 0
			}
		}
	}
}
