package handler

import (
	"encoding/json"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	pk "github.com/Tnze/go-mc/net/packet"
)

// BarrelState holds the items stored in a barrel at a world position.
type BarrelState struct {
	Items [27]game.ItemStack
	Pos   [3]int
}

// BarrelManager tracks all barrels in the world.
type BarrelManager struct {
	Barrels map[[3]int]*BarrelState
	mu      sync.RWMutex
}

// NewBarrelManager creates a new BarrelManager.
func NewBarrelManager() *BarrelManager {
	return &BarrelManager{
		Barrels: make(map[[3]int]*BarrelState),
	}
}

// GetOrCreate returns the barrel state at the given position, creating it if needed.
func (bm *BarrelManager) GetOrCreate(x, y, z int) *BarrelState {
	pos := [3]int{x, y, z}
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if bs, ok := bm.Barrels[pos]; ok {
		return bs
	}
	bs := &BarrelState{Pos: pos}
	bm.Barrels[pos] = bs
	return bs
}

// Get returns the barrel state at the given position, or nil.
func (bm *BarrelManager) Get(x, y, z int) *BarrelState {
	pos := [3]int{x, y, z}
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.Barrels[pos]
}

// OpenBarrel opens a barrel window for the player.
// A barrel is functionally identical to a single chest (27 slots, menu type 2 = generic_9x3).
func (bm *BarrelManager) OpenBarrel(player *game.Player, x, y, z int) {
	bs := bm.GetOrCreate(x, y, z)

	player.OpenWindowID = 12 // distinct window ID
	player.OpenChestPos = [3]int{x, y, z}

	title := chat.Text("Barrel")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(12),
		pk.VarInt(2), // menu type: generic_9x3
		title,
	))

	SendBarrelWindowContent(player, bs)
}

// SendBarrelWindowContent sends the full barrel window content.
// Layout: 63 slots = 27 barrel + 27 main inv + 9 hotbar (same as single chest).
func SendBarrelWindowContent(player *game.Player, bs *BarrelState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 63)
	for i := 0; i < 27; i++ {
		slots[i] = bs.Items[i].ToSlot()
	}
	for i := 9; i <= 35; i++ {
		slots[27+(i-9)] = player.Inventory[i].ToSlot()
	}
	for i := 36; i <= 44; i++ {
		slots[54+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(12),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// BarrelSlot returns a pointer to the item stack for a barrel window slot.
func BarrelSlot(player *game.Player, bs *BarrelState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 26:
		return &bs.Items[windowSlot]
	case windowSlot >= 27 && windowSlot <= 53:
		return &player.Inventory[windowSlot-27+9]
	case windowSlot >= 54 && windowSlot <= 62:
		return &player.Inventory[windowSlot-54+36]
	}
	return nil
}

// SaveAll serializes all barrel states for persistence.
func (bm *BarrelManager) SaveAll(dim string) []store.BlockEntityData {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	var result []store.BlockEntityData
	for pos, bs := range bm.Barrels {
		items := containerItemSlots(bs.Items[:])
		data, err := json.Marshal(map[string]interface{}{"items": items})
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "barrel", Data: data,
		})
	}
	return result
}

// LoadAll restores barrel states from persisted data.
func (bm *BarrelManager) LoadAll(entities []store.BlockEntityData) {
	for _, e := range entities {
		if e.Type != "barrel" {
			continue
		}
		var raw struct {
			Items []persistedItem `json:"items"`
		}
		if err := json.Unmarshal(e.Data, &raw); err != nil {
			continue
		}
		bs := bm.GetOrCreate(e.X, e.Y, e.Z)
		loadItemSlots(bs.Items[:], raw.Items)
	}
}

// Remove removes a barrel at the given position.
func (bm *BarrelManager) Remove(x, y, z int) {
	pos := [3]int{x, y, z}
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.Barrels, pos)
}
