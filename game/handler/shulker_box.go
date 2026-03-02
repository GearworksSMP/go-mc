package handler

import (
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// ShulkerBoxWindowID is the window ID used for shulker box windows.
const ShulkerBoxWindowID = 17

// ShulkerBoxState holds the items stored in a shulker box at a world position.
type ShulkerBoxState struct {
	Items [27]game.ItemStack
	Pos   [3]int
	Color string // color variant, e.g. "white", "orange", "" for undyed
}

// ShulkerBoxManager tracks all shulker boxes in the world.
type ShulkerBoxManager struct {
	ShulkerBoxes map[[3]int]*ShulkerBoxState
	mu           sync.RWMutex
}

// NewShulkerBoxManager creates a new ShulkerBoxManager.
func NewShulkerBoxManager() *ShulkerBoxManager {
	return &ShulkerBoxManager{
		ShulkerBoxes: make(map[[3]int]*ShulkerBoxState),
	}
}

// GetOrCreate returns the shulker box state at the given position, creating it if needed.
func (sbm *ShulkerBoxManager) GetOrCreate(x, y, z int) *ShulkerBoxState {
	pos := [3]int{x, y, z}
	sbm.mu.Lock()
	defer sbm.mu.Unlock()
	if sbs, ok := sbm.ShulkerBoxes[pos]; ok {
		return sbs
	}
	sbs := &ShulkerBoxState{Pos: pos}
	sbm.ShulkerBoxes[pos] = sbs
	return sbs
}

// Get returns the shulker box state at the given position, or nil.
func (sbm *ShulkerBoxManager) Get(x, y, z int) *ShulkerBoxState {
	pos := [3]int{x, y, z}
	sbm.mu.RLock()
	defer sbm.mu.RUnlock()
	return sbm.ShulkerBoxes[pos]
}

// OpenShulkerBox opens a shulker box window for the player.
func (sbm *ShulkerBoxManager) OpenShulkerBox(player *game.Player, x, y, z int) {
	sbs := sbm.GetOrCreate(x, y, z)

	player.OpenWindowID = ShulkerBoxWindowID
	player.OpenChestPos = [3]int{x, y, z} // reuse chest pos field

	title := chat.Text("Shulker Box")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(ShulkerBoxWindowID),
		pk.VarInt(20), // menu type: shulker_box
		title,
	))

	SendShulkerBoxWindowContent(player, sbs)
}

// SendShulkerBoxWindowContent sends the full shulker box window content.
// Layout: 63 slots = 27 shulker + 27 main inv + 9 hotbar.
func SendShulkerBoxWindowContent(player *game.Player, sbs *ShulkerBoxState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 63)
	for i := 0; i < 27; i++ {
		slots[i] = sbs.Items[i].ToSlot()
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
		pk.UnsignedByte(ShulkerBoxWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// ShulkerBoxSlot returns a pointer to the item stack for a shulker box window slot.
func ShulkerBoxSlot(player *game.Player, sbs *ShulkerBoxState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 26:
		return &sbs.Items[windowSlot]
	case windowSlot >= 27 && windowSlot <= 53:
		return &player.Inventory[windowSlot-27+9]
	case windowSlot >= 54 && windowSlot <= 62:
		return &player.Inventory[windowSlot-54+36]
	}
	return nil
}

// Remove removes a shulker box at the given position.
func (sbm *ShulkerBoxManager) Remove(x, y, z int) *ShulkerBoxState {
	pos := [3]int{x, y, z}
	sbm.mu.Lock()
	defer sbm.mu.Unlock()
	sbs := sbm.ShulkerBoxes[pos]
	delete(sbm.ShulkerBoxes, pos)
	return sbs
}

// IsShulkerBox returns true if the block name is any shulker box variant.
func IsShulkerBox(blockName string) bool {
	switch blockName {
	case "shulker_box",
		"white_shulker_box", "orange_shulker_box", "magenta_shulker_box",
		"light_blue_shulker_box", "yellow_shulker_box", "lime_shulker_box",
		"pink_shulker_box", "gray_shulker_box", "light_gray_shulker_box",
		"cyan_shulker_box", "purple_shulker_box", "blue_shulker_box",
		"brown_shulker_box", "green_shulker_box", "red_shulker_box",
		"black_shulker_box":
		return true
	}
	return false
}
