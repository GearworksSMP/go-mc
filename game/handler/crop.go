package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// CropManager handles crop growth simulation.
type CropManager struct {
	World   game.World
	Manager *game.PlayerManager

	mu    sync.Mutex
	crops map[[3]int]struct{} // positions of registered crops
}

// NewCropManager creates a new CropManager.
func NewCropManager(world game.World, manager *game.PlayerManager) *CropManager {
	return &CropManager{
		World:   world,
		Manager: manager,
		crops:   make(map[[3]int]struct{}),
	}
}

// RegisterCrop registers a crop for growth ticking.
func (c *CropManager) RegisterCrop(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.crops[[3]int{x, y, z}] = struct{}{}
}

// UnregisterCrop removes a crop from growth ticking.
func (c *CropManager) UnregisterCrop(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.crops, [3]int{x, y, z})
}

// Tick processes crop growth. Called every server tick.
// Growth checks happen every 100 ticks (~5 seconds).
func (c *CropManager) Tick(tick int64) {
	if tick%100 != 0 {
		return
	}

	c.mu.Lock()
	positions := make([][3]int, 0, len(c.crops))
	for pos := range c.crops {
		positions = append(positions, pos)
	}
	c.mu.Unlock()

	for _, pos := range positions {
		c.tickCrop(pos[0], pos[1], pos[2])
	}
}

func (c *CropManager) tickCrop(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterCrop(x, y, z)
		return
	}

	// Verify block below is farmland
	belowState, err := c.World.GetBlock(x, y-1, z)
	if err != nil {
		return
	}
	belowName := BlockNameFromState(int(belowState))
	if belowName != "farmland" {
		// Farmland destroyed — break crop, drop as item
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterCrop(x, y, z)
		return
	}

	// Growth chance: 1/25 base, doubled if hydrated farmland
	chance := 25
	if c.isHydrated(x, y-1, z) {
		chance = 12
	}
	if rand.Intn(chance) != 0 {
		return
	}

	c.growCropState(x, y, z)
}

// growCropState attempts to advance a crop's age by 1.
func (c *CropManager) growCropState(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var newBlock block.Block
	atMax := false

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Wheat{Age: block.Integer(age + 1)}
		}
	case block.Carrots:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Carrots{Age: block.Integer(age + 1)}
		}
	case block.Potatoes:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Potatoes{Age: block.Integer(age + 1)}
		}
	case block.Beetroots:
		age := int(b.Age)
		if age >= 3 {
			atMax = true
		} else {
			newBlock = block.Beetroots{Age: block.Integer(age + 1)}
		}
	default:
		c.UnregisterCrop(x, y, z)
		return
	}

	if atMax {
		return // already mature
	}

	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}

	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
}

// BoneMealCrop advances a crop by 2-5 growth stages (random, capped at max).
// Returns true if the crop was advanced.
func (c *CropManager) BoneMealCrop(x, y, z int) bool {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	var newBlock block.Block
	var changed bool

	advance := 2 + rand.Intn(4) // 2-5 stages

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Wheat{Age: block.Integer(newAge)}
		changed = true
	case block.Carrots:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Carrots{Age: block.Integer(newAge)}
		changed = true
	case block.Potatoes:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Potatoes{Age: block.Integer(newAge)}
		changed = true
	case block.Beetroots:
		age := int(b.Age)
		if age >= 3 {
			return false
		}
		newAge := age + advance
		if newAge > 3 {
			newAge = 3
		}
		newBlock = block.Beetroots{Age: block.Integer(newAge)}
		changed = true
	}

	if !changed {
		return false
	}

	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return false
	}

	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
	return true
}

// isHydrated checks if farmland at (x, y, z) is hydrated (water within 4 blocks).
func (c *CropManager) isHydrated(x, y, z int) bool {
	// Check farmland moisture property
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		if fl, ok := block.StateList[stateID].(block.Farmland); ok {
			if int(fl.Moisture) > 0 {
				return true
			}
		}
	}

	// Check for nearby water (4 blocks horizontally, same Y or Y+1)
	for dx := -4; dx <= 4; dx++ {
		for dz := -4; dz <= 4; dz++ {
			for dy := 0; dy <= 1; dy++ {
				waterState, err := c.World.GetBlock(x+dx, y+dy, z+dz)
				if err != nil {
					continue
				}
				if int(waterState) < len(block.StateList) && block.StateList[waterState] != nil {
					if _, ok := block.StateList[waterState].(block.Water); ok {
						return true
					}
				}
			}
		}
	}
	return false
}
