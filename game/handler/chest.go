package handler

import (
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// ChestState holds the items stored in a chest at a world position.
type ChestState struct {
	Items [27]game.ItemStack
	Pos   [3]int
}

// ChestManager tracks all chests in the world.
type ChestManager struct {
	Chests map[[3]int]*ChestState
	mu     sync.RWMutex
}

// NewChestManager creates a new ChestManager.
func NewChestManager() *ChestManager {
	return &ChestManager{
		Chests: make(map[[3]int]*ChestState),
	}
}

// getOrCreate returns the chest state at the given position, creating it if needed.
func (cm *ChestManager) getOrCreate(x, y, z int) *ChestState {
	pos := [3]int{x, y, z}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cs, ok := cm.Chests[pos]; ok {
		return cs
	}
	cs := &ChestState{Pos: pos}
	cm.Chests[pos] = cs
	return cs
}

// Get returns the chest state at the given position, or nil.
func (cm *ChestManager) Get(x, y, z int) *ChestState {
	pos := [3]int{x, y, z}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Chests[pos]
}

// OpenChest opens a chest window for the player.
func (cm *ChestManager) OpenChest(player *game.Player, x, y, z int) {
	cs := cm.getOrCreate(x, y, z)

	player.OpenWindowID = 2
	player.OpenChestPos = [3]int{x, y, z}

	// Send OpenScreen: windowID=2, menuType=2 (generic_9x3 = chest), title
	title := chat.Text("Chest")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(2),  // window ID
		pk.VarInt(2),  // menu type: generic_9x3
		title,
	))

	SendChestWindowContent(player, cs)
}

// SendChestWindowContent sends the full chest window content to a player.
// Layout: 63 slots = 27 chest + 27 main inv + 9 hotbar
func SendChestWindowContent(player *game.Player, cs *ChestState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 63)
	// Chest slots 0-26
	for i := 0; i < 27; i++ {
		slots[i] = cs.Items[i].ToSlot()
	}
	// Main inventory: window slots 27-53 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[27+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: window slots 54-62 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[54+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(2), // window ID 2
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// ChestSlot returns a pointer to the item stack for a chest window slot.
// Returns nil if the slot is out of range.
func ChestSlot(player *game.Player, cs *ChestState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 26:
		return &cs.Items[windowSlot]
	case windowSlot >= 27 && windowSlot <= 53:
		return &player.Inventory[windowSlot-27+9] // main inv: 9..35
	case windowSlot >= 54 && windowSlot <= 62:
		return &player.Inventory[windowSlot-54+36] // hotbar: 36..44
	}
	return nil
}
