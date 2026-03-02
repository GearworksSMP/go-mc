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
	Items   [27]game.ItemStack
	Pos     [3]int
	Partner *ChestState // nil = single chest, non-nil = linked double chest
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

// GetOrCreate returns the chest state at the given position, creating it if needed.
func (cm *ChestManager) GetOrCreate(x, y, z int) *ChestState {
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
	cs := cm.GetOrCreate(x, y, z)

	player.OpenWindowID = 2
	player.OpenChestPos = [3]int{x, y, z}

	if cs.Partner != nil {
		// Double chest: generic_9x6 (menu type 5), 54 chest slots + 36 player = 90
		title := chat.Text("Large Chest")
		player.WritePacket(pk.Marshal(
			packetid.ClientboundOpenScreen,
			pk.VarInt(2),  // window ID
			pk.VarInt(5),  // menu type: generic_9x6
			title,
		))
		SendDoubleChestWindowContent(player, cs)
	} else {
		// Single chest: generic_9x3 (menu type 2)
		title := chat.Text("Chest")
		player.WritePacket(pk.Marshal(
			packetid.ClientboundOpenScreen,
			pk.VarInt(2),  // window ID
			pk.VarInt(2),  // menu type: generic_9x3
			title,
		))
		SendChestWindowContent(player, cs)
	}
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
	if cs.Partner != nil {
		return DoubleChestSlot(player, cs, windowSlot)
	}
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

// DoubleChestSlot returns a pointer to the item stack for a double chest window slot.
// Layout: 90 slots = 54 chest (27 left + 27 right) + 27 main inv + 9 hotbar.
func DoubleChestSlot(player *game.Player, cs *ChestState, windowSlot int) *game.ItemStack {
	left, right := doubleChestOrder(cs)
	switch {
	case windowSlot >= 0 && windowSlot <= 26:
		return &left.Items[windowSlot]
	case windowSlot >= 27 && windowSlot <= 53:
		return &right.Items[windowSlot-27]
	case windowSlot >= 54 && windowSlot <= 80:
		return &player.Inventory[windowSlot-54+9] // main inv: 9..35
	case windowSlot >= 81 && windowSlot <= 89:
		return &player.Inventory[windowSlot-81+36] // hotbar: 36..44
	}
	return nil
}

// doubleChestOrder returns (left, right) where left has the lower coordinate.
func doubleChestOrder(cs *ChestState) (*ChestState, *ChestState) {
	p := cs.Partner
	if p == nil {
		return cs, cs
	}
	// Lower X first, then lower Z
	if cs.Pos[0] < p.Pos[0] || (cs.Pos[0] == p.Pos[0] && cs.Pos[2] < p.Pos[2]) {
		return cs, p
	}
	return p, cs
}

// SendDoubleChestWindowContent sends the full double chest window content.
// Layout: 90 slots = 54 chest + 27 main inv + 9 hotbar.
func SendDoubleChestWindowContent(player *game.Player, cs *ChestState) {
	stateID := player.NextStateID()
	left, right := doubleChestOrder(cs)

	slots := make(game.Slot261Array, 90)
	// Left chest: slots 0-26
	for i := 0; i < 27; i++ {
		slots[i] = left.Items[i].ToSlot()
	}
	// Right chest: slots 27-53
	for i := 0; i < 27; i++ {
		slots[27+i] = right.Items[i].ToSlot()
	}
	// Main inventory: slots 54-80 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[54+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: slots 81-89 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[81+(i-36)] = player.Inventory[i].ToSlot()
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

// TryLinkDouble checks 4 horizontal neighbors for a single chest and links them as a double.
func (cm *ChestManager) TryLinkDouble(x, y, z int, world game.World, manager *game.PlayerManager) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cs := cm.Chests[[3]int{x, y, z}]
	if cs == nil {
		cs = &ChestState{Pos: [3]int{x, y, z}}
		cm.Chests[[3]int{x, y, z}] = cs
	}
	if cs.Partner != nil {
		return // already linked
	}

	neighbors := [][3]int{
		{x - 1, y, z},
		{x + 1, y, z},
		{x, y, z - 1},
		{x, y, z + 1},
	}

	for _, npos := range neighbors {
		state, err := world.GetBlock(npos[0], npos[1], npos[2])
		if err != nil {
			continue
		}
		name := BlockNameFromState(int(state))
		if name != "chest" {
			continue
		}
		neighbor := cm.Chests[npos]
		if neighbor == nil {
			continue
		}
		if neighbor.Partner != nil {
			continue // neighbor already doubled
		}
		// Link
		cs.Partner = neighbor
		neighbor.Partner = cs
		return
	}
}

// UnlinkChest unlinks a double chest when one half is broken.
func (cm *ChestManager) UnlinkChest(x, y, z int, world game.World, manager *game.PlayerManager) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cs := cm.Chests[[3]int{x, y, z}]
	if cs == nil {
		return
	}
	if cs.Partner != nil {
		cs.Partner.Partner = nil
		cs.Partner = nil
	}
	delete(cm.Chests, [3]int{x, y, z})
}
