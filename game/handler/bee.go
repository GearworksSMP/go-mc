package handler

import (
	"log"
	"math"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// HiveManager tracks beehive/bee_nest blocks and their honey levels.
type HiveManager struct {
	MobMgr       *MobManager
	Players      *game.PlayerManager
	ItemEntities *ItemEntityManager
	Survival     *SurvivalHandler
	World        game.World
	Logger       *log.Logger
	mu           sync.Mutex
	hives        map[[3]int]*HiveData
}

// HiveData stores the state of a single beehive or bee_nest block.
type HiveData struct {
	BeeCount   int // bees inside (max 3)
	HoneyLevel int // 0-5
}

// NewHiveManager creates a new HiveManager.
func NewHiveManager(mobMgr *MobManager, players *game.PlayerManager, itemEntities *ItemEntityManager, survival *SurvivalHandler, world game.World, logger *log.Logger) *HiveManager {
	return &HiveManager{
		MobMgr:       mobMgr,
		Players:      players,
		ItemEntities: itemEntities,
		Survival:     survival,
		World:        world,
		Logger:       logger,
		hives:        make(map[[3]int]*HiveData),
	}
}

// RegisterHive adds a beehive or bee_nest to the tracked hives map.
func (h *HiveManager) RegisterHive(x, y, z int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pos := [3]int{x, y, z}
	if _, ok := h.hives[pos]; !ok {
		h.hives[pos] = &HiveData{}
	}
}

// GetHive returns the hive data at the given position, or nil.
func (h *HiveManager) GetHive(x, y, z int) *HiveData {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hives[[3]int{x, y, z}]
}

// IncrementHoney increases the honey level of a hive (capped at 5)
// and updates the block state in the world.
func (h *HiveManager) IncrementHoney(x, y, z int) {
	h.mu.Lock()
	hd := h.hives[[3]int{x, y, z}]
	if hd == nil {
		h.mu.Unlock()
		return
	}
	if hd.HoneyLevel < 5 {
		hd.HoneyLevel++
	}
	level := hd.HoneyLevel
	h.mu.Unlock()
	h.updateHiveBlockState(x, y, z, level)
}

// AddBeeToHive attempts to add a bee to the hive at the given position.
// Returns true if the bee was accepted (hive exists and has room).
func (h *HiveManager) AddBeeToHive(x, y, z int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	hd := h.hives[[3]int{x, y, z}]
	if hd == nil {
		// Auto-register
		hd = &HiveData{}
		h.hives[[3]int{x, y, z}] = hd
	}
	if hd.BeeCount >= 3 {
		return false
	}
	hd.BeeCount++
	return true
}

// ReleaseBee decrements the bee count in a hive. Called when a bee exits.
func (h *HiveManager) ReleaseBee(x, y, z int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hd := h.hives[[3]int{x, y, z}]
	if hd != nil && hd.BeeCount > 0 {
		hd.BeeCount--
	}
}

// updateHiveBlockState sets the honey_level property on the beehive/bee_nest block.
func (h *HiveManager) updateHiveBlockState(x, y, z, honeyLevel int) {
	if h.World == nil {
		return
	}
	stateID, err := h.World.GetBlock(x, y, z)
	if err != nil || int(stateID) < 0 || int(stateID) >= len(block.StateList) {
		return
	}
	b := block.StateList[stateID]
	if b == nil {
		return
	}
	hl := block.Integer(honeyLevel)
	var newBlock block.Block
	switch v := b.(type) {
	case block.Beehive:
		v.Honey_level = hl
		newBlock = v
	case block.BeeNest:
		v.Honey_level = hl
		newBlock = v
	default:
		return
	}
	if newID, ok := block.ToStateID[newBlock]; ok {
		h.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(h.Players, x, y, z, int32(newID))
	}
}

// UseHive handles a player right-clicking a beehive or bee_nest.
// Returns true if the interaction was consumed.
func (h *HiveManager) UseHive(player *game.Player, x, y, z int) bool {
	h.mu.Lock()
	hd := h.hives[[3]int{x, y, z}]
	if hd == nil {
		// Auto-register hive on first interaction
		hd = &HiveData{}
		h.hives[[3]int{x, y, z}] = hd
	}
	honeyLevel := hd.HoneyLevel
	h.mu.Unlock()

	if honeyLevel < 5 {
		return false
	}

	slot := int(player.HeldSlot) + 36
	heldName := ItemNameByID(player.Inventory[slot].ID)

	if heldName == "glass_bottle" {
		// Consume glass bottle, give honey bottle, reset honey level
		h.mu.Lock()
		hd.HoneyLevel = 0
		h.mu.Unlock()
		h.updateHiveBlockState(x, y, z, 0)

		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)

		honeyBottleID := itemIDByName("honey_bottle")
		if honeyBottleID > 0 {
			addedSlot := player.Inventory.AddItem(honeyBottleID, 1)
			if addedSlot >= 0 {
				SendSlotUpdate(player, addedSlot)
			}
		}
		return true
	}

	if heldName == "shears" {
		// Drop 3 honeycomb, reset honey, anger nearby bees
		h.mu.Lock()
		hd.HoneyLevel = 0
		h.mu.Unlock()
		h.updateHiveBlockState(x, y, z, 0)

		// Drop honeycomb items
		if h.ItemEntities != nil {
			honeycombID := itemIDByName("honeycomb")
			if honeycombID > 0 {
				h.ItemEntities.SpawnItem(h.Players, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, honeycombID, 3, 10)
			}
		}

		// Anger nearby bees within 16 blocks
		h.angerNearbyBees(x, y, z)
		return true
	}

	return false
}

// angerNearbyBees sets BeeAngryTicks on all bees within 16 blocks of the position.
func (h *HiveManager) angerNearbyBees(x, y, z int) {
	h.MobMgr.mu.Lock()
	defer h.MobMgr.mu.Unlock()
	for _, mob := range h.MobMgr.Mobs {
		if mob.TypeID != MobTypeBee {
			continue
		}
		dx := mob.X - float64(x)
		dy := mob.Y - float64(y)
		dz := mob.Z - float64(z)
		if math.Sqrt(dx*dx+dy*dy+dz*dz) <= 16 {
			mob.BeeAngryTicks = 400
			mob.Hostile = true
		}
	}
}

// flowerBlocks is the set of flower block names that bees can pollinate.
var flowerBlocks = map[string]bool{
	"dandelion":          true,
	"poppy":              true,
	"blue_orchid":        true,
	"allium":             true,
	"azure_bluet":        true,
	"red_tulip":          true,
	"orange_tulip":       true,
	"white_tulip":        true,
	"pink_tulip":         true,
	"oxeye_daisy":        true,
	"cornflower":         true,
	"lily_of_the_valley": true,
	"sunflower":          true,
	"lilac":              true,
	"rose_bush":          true,
	"peony":              true,
	"wither_rose":        true,
	"torchflower":        true,
}

