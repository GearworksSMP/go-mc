package handler

import (
	"github.com/Tnze/go-mc/game"
)

// persistedItem is the JSON-serializable format for a single container item slot.
type persistedItem struct {
	ID    int32 `json:"id"`
	Count int32 `json:"count"`
	Slot  int   `json:"slot"`
}

// containerItemSlots converts a slice of ItemStack to persistedItem entries,
// skipping empty slots.
func containerItemSlots(items []game.ItemStack) []persistedItem {
	var result []persistedItem
	for i, it := range items {
		if it.ID <= 0 || it.Count <= 0 {
			continue
		}
		result = append(result, persistedItem{ID: it.ID, Count: it.Count, Slot: i})
	}
	return result
}

// loadItemSlots populates a fixed-size item array from persisted slot data.
func loadItemSlots(items []game.ItemStack, slots []persistedItem) {
	for _, s := range slots {
		if s.Slot < 0 || s.Slot >= len(items) {
			continue
		}
		items[s.Slot] = game.ItemStack{ID: s.ID, Count: s.Count}
	}
}
