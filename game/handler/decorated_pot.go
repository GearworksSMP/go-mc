package handler

import (
	"sync"

	"github.com/Tnze/go-mc/game"
)

// PotData stores the item held inside a decorated pot.
type PotData struct {
	StoredItem game.ItemStack
}

// DecoratedPotManager manages decorated pot blocks that can store a single item stack.
type DecoratedPotManager struct {
	Manager      *game.PlayerManager
	ItemEntities *ItemEntityManager
	mu           sync.Mutex
	pots         map[[3]int]*PotData
}

// NewDecoratedPotManager creates a new DecoratedPotManager.
func NewDecoratedPotManager(manager *game.PlayerManager, itemEntities *ItemEntityManager) *DecoratedPotManager {
	return &DecoratedPotManager{
		Manager:      manager,
		ItemEntities: itemEntities,
		pots:         make(map[[3]int]*PotData),
	}
}

// UsePot handles right-click interaction with a decorated pot.
// Returns true if any interaction happened.
func (dm *DecoratedPotManager) UsePot(player *game.Player, x, y, z int) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	pos := [3]int{x, y, z}
	pot, exists := dm.pots[pos]
	if !exists {
		pot = &PotData{}
		dm.pots[pos] = pot
	}

	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]

	if pot.StoredItem.ID > 0 && pot.StoredItem.Count > 0 {
		// Pot has an item — try to give it to the player
		slot := player.Inventory.AddItem(pot.StoredItem.ID, pot.StoredItem.Count)
		if slot < 0 {
			return false // inventory full
		}
		pot.StoredItem = game.ItemStack{}
		SendSlotUpdate(player, slot)
		return true
	}

	if held.ID > 0 && held.Count > 0 {
		// Player is holding something — store it in the pot
		pot.StoredItem = *held
		*held = game.ItemStack{}
		SendSlotUpdate(player, heldSlot)
		return true
	}

	return false
}

// BreakPot handles breaking a decorated pot block, dropping its contents and itself.
func (dm *DecoratedPotManager) BreakPot(x, y, z int) {
	dm.mu.Lock()
	pos := [3]int{x, y, z}
	pot, exists := dm.pots[pos]
	if exists {
		delete(dm.pots, pos)
	}
	dm.mu.Unlock()

	fx, fy, fz := float64(x)+0.5, float64(y)+0.5, float64(z)+0.5

	// Drop stored item if any
	if exists && pot.StoredItem.ID > 0 && pot.StoredItem.Count > 0 && dm.ItemEntities != nil {
		dm.ItemEntities.SpawnItem(dm.Manager, fx, fy, fz, pot.StoredItem.ID, pot.StoredItem.Count, 10)
	}

	// Drop the decorated pot itself
	if dm.ItemEntities != nil {
		potItemID := itemIDByName("decorated_pot")
		if potItemID > 0 {
			dm.ItemEntities.SpawnItem(dm.Manager, fx, fy, fz, potItemID, 1, 10)
		}
	}
}
