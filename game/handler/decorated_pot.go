package handler

import (
	"encoding/json"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
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

// SaveAll serializes all decorated pot states for persistence.
func (dm *DecoratedPotManager) SaveAll(dim string) []store.BlockEntityData {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	var result []store.BlockEntityData
	for pos, pot := range dm.pots {
		var items []persistedItem
		if pot.StoredItem.ID > 0 && pot.StoredItem.Count > 0 {
			items = append(items, persistedItem{ID: pot.StoredItem.ID, Count: pot.StoredItem.Count, Slot: 0})
		}
		data, err := json.Marshal(map[string]interface{}{"items": items})
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "decorated_pot", Data: data,
		})
	}
	return result
}

// LoadAll restores decorated pot states from persisted data.
func (dm *DecoratedPotManager) LoadAll(entities []store.BlockEntityData) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, e := range entities {
		if e.Type != "decorated_pot" {
			continue
		}
		var raw struct {
			Items []persistedItem `json:"items"`
		}
		if err := json.Unmarshal(e.Data, &raw); err != nil {
			continue
		}
		pot := &PotData{}
		if len(raw.Items) > 0 && raw.Items[0].ID > 0 {
			pot.StoredItem = game.ItemStack{ID: raw.Items[0].ID, Count: raw.Items[0].Count}
		}
		dm.pots[[3]int{e.X, e.Y, e.Z}] = pot
	}
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
