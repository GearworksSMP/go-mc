package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// TreeGrowthManager handles sapling growth into trees.
type TreeGrowthManager struct {
	World   game.World
	Manager *game.PlayerManager
	mu      sync.Mutex
	// saplings maps position → tick when registered
	saplings map[[3]int]int64
}

// NewTreeGrowthManager creates a new TreeGrowthManager.
func NewTreeGrowthManager(world game.World, manager *game.PlayerManager) *TreeGrowthManager {
	return &TreeGrowthManager{
		World:    world,
		Manager:  manager,
		saplings: make(map[[3]int]int64),
	}
}

// RegisterSapling registers a sapling at the given position for growth tracking.
func (t *TreeGrowthManager) RegisterSapling(x, y, z int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.saplings[[3]int{x, y, z}] = 0
}

// UnregisterSapling removes a sapling from growth tracking.
func (t *TreeGrowthManager) UnregisterSapling(x, y, z int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.saplings, [3]int{x, y, z})
}

// Tick processes sapling growth. Called every tick.
// Every 100 ticks, each sapling has a ~1/60 chance of growing (~30 seconds avg).
func (t *TreeGrowthManager) Tick(tick int64) {
	if tick%100 != 0 {
		return
	}

	t.mu.Lock()
	// Collect saplings that should try to grow
	var toGrow [][3]int
	for pos := range t.saplings {
		if rand.Intn(60) == 0 {
			toGrow = append(toGrow, pos)
		}
	}
	t.mu.Unlock()

	for _, pos := range toGrow {
		if t.GrowTree(pos[0], pos[1], pos[2]) {
			t.mu.Lock()
			delete(t.saplings, pos)
			t.mu.Unlock()
		}
	}
}

// GrowTree attempts to grow a tree at the given position.
// Returns true if the tree was successfully grown.
func (t *TreeGrowthManager) GrowTree(x, y, z int) bool {
	// Verify sapling still exists
	state, err := t.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	blockName := BlockNameFromState(int(state))
	if !isSapling(blockName) {
		return false
	}

	// Pick trunk height 4-6
	trunkH := 4 + rand.Intn(3)

	// Check clearance: trunk + 2 for leaves
	for dy := 1; dy <= trunkH+2; dy++ {
		checkY := y + dy
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				if dx == 0 && dz == 0 && dy <= trunkH {
					// Trunk position — only needs to be air or replaceable
					s, err := t.World.GetBlock(x, checkY, z)
					if err != nil {
						return false
					}
					if s != 0 && !isReplaceable(BlockNameFromState(int(s))) {
						return false
					}
				} else if dy > trunkH-2 {
					// Leaf area — check it's air or replaceable
					s, err := t.World.GetBlock(x+dx, checkY, z+dz)
					if err != nil {
						continue // out of world — skip
					}
					if s != 0 && !isReplaceable(BlockNameFromState(int(s))) {
						return false
					}
				}
			}
		}
	}

	// Get block state IDs for oak log and leaves
	oakLog, ok1 := block.ToStateID[block.OakLog{Axis: block.Y}]
	oakLeaves, ok2 := block.ToStateID[block.OakLeaves{
		Distance:    block.Integer(1),
		Persistent:  block.Boolean(false),
		Waterlogged: block.Boolean(false),
	}]
	if !ok1 || !ok2 {
		return false
	}

	// Remove sapling
	t.World.SetBlock(x, y, z, 0)
	t.broadcastBlockUpdate(x, y, z, 0)

	// Place trunk
	for dy := 1; dy <= trunkH; dy++ {
		t.World.SetBlock(x, y+dy, z, oakLog)
		t.broadcastBlockUpdate(x, y+dy, z, int32(oakLog))
	}

	// Place leaves (3x3x2 crown + 1x1x1 top, matching terrain.go)
	topY := y + trunkH
	for dy := -1; dy <= 1; dy++ {
		radius := 1
		if dy == 1 {
			radius = 0 // top layer is 1x1
		}
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				lx, ly, lz := x+dx, topY+dy, z+dz
				if dx == 0 && dz == 0 && dy <= 0 {
					continue // trunk position
				}
				t.World.SetBlock(lx, ly, lz, oakLeaves)
				t.broadcastBlockUpdate(lx, ly, lz, int32(oakLeaves))
			}
		}
	}

	return true
}

// isReplaceable returns true if the block can be replaced by tree growth.
func isReplaceable(name string) bool {
	switch name {
	case "air", "short_grass", "tall_grass", "fern", "large_fern",
		"dead_bush", "snow", "dandelion", "poppy",
		"oak_sapling", "spruce_sapling", "birch_sapling",
		"jungle_sapling", "acacia_sapling", "dark_oak_sapling",
		"cherry_sapling", "oak_leaves", "":
		return true
	}
	return false
}

func (t *TreeGrowthManager) broadcastBlockUpdate(x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	t.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
