package handler

import (
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// CrafterWindowID is the window ID used for crafter blocks.
const CrafterWindowID = 25

// CrafterState holds the state of a single crafter block.
type CrafterState struct {
	Slots        [9]game.ItemStack
	DisabledMask uint16 // bitmask: bit i set means slot i is disabled
	Pos          [3]int
}

// IsDisabled returns whether the given slot index is disabled.
func (cs *CrafterState) IsDisabled(slot int) bool {
	return cs.DisabledMask&(1<<uint(slot)) != 0
}

// ToggleDisabled toggles the disabled state of the given slot index.
func (cs *CrafterState) ToggleDisabled(slot int) {
	cs.DisabledMask ^= 1 << uint(slot)
}

// CrafterManager tracks all crafter blocks in the world and handles auto-crafting.
type CrafterManager struct {
	Crafters     map[[3]int]*CrafterState
	mu           sync.RWMutex
	Manager      *game.PlayerManager
	World        game.World
	ItemEntities *ItemEntityManager
}

// NewCrafterManager creates a new CrafterManager.
func NewCrafterManager(manager *game.PlayerManager, world game.World, itemEntities *ItemEntityManager) *CrafterManager {
	return &CrafterManager{
		Crafters:     make(map[[3]int]*CrafterState),
		Manager:      manager,
		World:        world,
		ItemEntities: itemEntities,
	}
}

// GetOrCreate returns the crafter state at the given position, creating it if needed.
func (cm *CrafterManager) GetOrCreate(x, y, z int) *CrafterState {
	pos := [3]int{x, y, z}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cs, ok := cm.Crafters[pos]; ok {
		return cs
	}
	cs := &CrafterState{Pos: pos}
	cm.Crafters[pos] = cs
	return cs
}

// Get returns the crafter state at the given position, or nil.
func (cm *CrafterManager) Get(x, y, z int) *CrafterState {
	pos := [3]int{x, y, z}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Crafters[pos]
}

// Remove removes the crafter state at the given position.
func (cm *CrafterManager) Remove(x, y, z int) {
	pos := [3]int{x, y, z}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.Crafters, pos)
}

// OpenCrafter opens a crafter window for the player.
func (cm *CrafterManager) OpenCrafter(player *game.Player, x, y, z int) {
	cs := cm.GetOrCreate(x, y, z)

	player.OpenWindowID = CrafterWindowID
	player.OpenCrafterPos = [3]int{x, y, z}

	title := chat.Text("Crafter")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(CrafterWindowID),
		pk.VarInt(7), // menu type: crafter_3x3 (index 7)
		title,
	))

	SendCrafterWindowContent(player, cs)
}

// SendCrafterWindowContent sends the full crafter window content.
// Layout: 45 slots = 9 crafter + 27 main inv + 9 hotbar.
func SendCrafterWindowContent(player *game.Player, cs *CrafterState) {
	stateID := player.NextStateID()
	totalSlots := 9 + 27 + 9
	buf := make([]game.Slot261, totalSlots)

	// Crafter slots 0-8
	for i := 0; i < 9; i++ {
		buf[i] = cs.Slots[i].ToSlot()
	}
	// Main inventory (player slots 9-35) -> window slots 9-35
	for i := 9; i <= 35; i++ {
		buf[i] = player.Inventory[i].ToSlot()
	}
	// Hotbar (player slots 36-44) -> window slots 36-44
	for i := 36; i <= 44; i++ {
		buf[i] = player.Inventory[i].ToSlot()
	}

	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.VarInt(CrafterWindowID),
		pk.VarInt(stateID),
		pk.VarInt(int32(totalSlots)),
		pk.Array(buf),
		player.CursorItem.ToSlot(),
	))

	// Send disabled slot data via container data property.
	// For crafter_3x3, property 0 = triggered (crafting), properties 1-9 = slot disabled flags.
	for i := 0; i < 9; i++ {
		val := int16(0)
		if cs.IsDisabled(i) {
			val = 1
		}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.VarInt(CrafterWindowID),
			pk.Short(int16(i+1)), // property: 1-9 for slots 0-8 disabled state
			pk.Short(val),
		))
	}
}

// CrafterSlot maps a window slot index to the actual ItemStack pointer.
func CrafterSlot(player *game.Player, cs *CrafterState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot < 9:
		return &cs.Slots[windowSlot]
	case windowSlot >= 9 && windowSlot <= 44:
		return &player.Inventory[windowSlot]
	}
	return nil
}

// HandleCrafterClick processes a click on a crafter slot.
// If a player clicks an empty crafter slot with an empty cursor, toggle the disabled state.
func (cm *CrafterManager) HandleCrafterClick(player *game.Player, cs *CrafterState, slot, button int) {
	if slot < 0 || slot > 44 {
		return
	}

	// Clicking on a crafter grid slot (0-8) with empty cursor on empty slot toggles disabled.
	if slot < 9 && player.CursorItem.ID == 0 && cs.Slots[slot].ID == 0 {
		cs.ToggleDisabled(slot)
		// Send disabled state update to client
		val := int16(0)
		if cs.IsDisabled(slot) {
			val = 1
		}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.VarInt(CrafterWindowID),
			pk.Short(int16(slot+1)),
			pk.Short(val),
		))
		return
	}

	if slot < 9 && cs.IsDisabled(slot) {
		return
	}

	src := CrafterSlot(player, cs, slot)
	if src == nil {
		return
	}
	genericClick(player, src, button)
}

// HandleCrafterShiftClick handles shift-click for crafter windows.
func (cm *CrafterManager) HandleCrafterShiftClick(player *game.Player, cs *CrafterState, slot int) {
	if slot < 0 || slot > 44 {
		return
	}
	src := CrafterSlot(player, cs, slot)
	if src == nil || src.ID == 0 {
		return
	}
	if slot < 9 {
		// Move from crafter to player inventory
		player.Inventory.AddItem(src.ID, src.Count)
		*src = game.ItemStack{}
	} else {
		// Move from player inventory to crafter (skip disabled slots)
		for i := 0; i < 9; i++ {
			if cs.IsDisabled(i) {
				continue
			}
			if cs.Slots[i].ID == src.ID && cs.Slots[i].Count < 64 {
				space := int32(64) - cs.Slots[i].Count
				if src.Count <= space {
					cs.Slots[i].Count += src.Count
					*src = game.ItemStack{}
					return
				}
				cs.Slots[i].Count = 64
				src.Count -= space
			}
		}
		for i := 0; i < 9; i++ {
			if cs.IsDisabled(i) {
				continue
			}
			if cs.Slots[i].ID == 0 {
				cs.Slots[i] = *src
				*src = game.ItemStack{}
				return
			}
		}
	}
}

// HandleCrafterNumberKey handles number key swaps for crafter windows.
func (cm *CrafterManager) HandleCrafterNumberKey(player *game.Player, cs *CrafterState, slot, button int) {
	if slot < 0 || slot > 44 || button < 0 || button > 8 {
		return
	}
	if slot < 9 && cs.IsDisabled(slot) {
		return
	}
	src := CrafterSlot(player, cs, slot)
	if src == nil {
		return
	}
	hotbarSlot := 36 + button
	src2 := &player.Inventory[hotbarSlot]
	*src, *src2 = *src2, *src
}

// HandleCrafterDrop handles dropping items from crafter windows.
func (cm *CrafterManager) HandleCrafterDrop(player *game.Player, cs *CrafterState, slot, button int) {
	if slot < 0 || slot > 44 {
		return
	}
	src := CrafterSlot(player, cs, slot)
	if src == nil || src.ID == 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else {
		*src = game.ItemStack{}
	}
}

// Activate is called when a crafter receives a redstone pulse.
// It attempts to craft using the current grid contents and ejects the result from the front face.
func (cm *CrafterManager) Activate(x, y, z int) {
	cm.mu.RLock()
	cs := cm.Crafters[[3]int{x, y, z}]
	cm.mu.RUnlock()

	if cs == nil {
		return
	}

	// Build a 3x3 grid of item IDs for recipe matching.
	// Disabled slots act as empty for pattern matching.
	var grid [9]int32
	for i := 0; i < 9; i++ {
		if !cs.IsDisabled(i) && cs.Slots[i].ID > 0 && cs.Slots[i].Count > 0 {
			grid[i] = cs.Slots[i].ID
		}
	}

	resultID, resultCount := MatchRecipe(grid[:], 3)
	if resultID == 0 {
		// No matching recipe; play fail sound
		BroadcastSound(cm.Manager, 555, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.2)
		return
	}

	// Consume one of each input item
	for i := 0; i < 9; i++ {
		if cs.IsDisabled(i) {
			continue
		}
		if cs.Slots[i].ID > 0 && cs.Slots[i].Count > 0 {
			cs.Slots[i].Count--
			if cs.Slots[i].Count <= 0 {
				cs.Slots[i] = game.ItemStack{}
			}
		}
	}

	// Determine the front face direction from the crafter block state
	facing := cm.getFacing(x, y, z)
	dx, dy, dz := directionOffset(facing)
	outX := float64(x) + float64(dx) + 0.5
	outY := float64(y) + float64(dy) + 0.5
	outZ := float64(z) + float64(dz) + 0.5

	// Eject crafted item from the front face (like a dropper)
	if cm.ItemEntities != nil {
		cm.ItemEntities.SpawnItem(cm.Manager, outX, outY, outZ, resultID, resultCount, 10)
	}

	// Play crafting sound
	BroadcastSound(cm.Manager, 555, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)

	// Update crafter window for any player viewing it
	cm.Manager.ForEach(func(p *game.Player) {
		if p.OpenWindowID == CrafterWindowID && p.OpenCrafterPos == cs.Pos {
			SendCrafterWindowContent(p, cs)
		}
	})
}

// getFacing returns the front direction of the crafter at (x, y, z).
func (cm *CrafterManager) getFacing(x, y, z int) block.Direction {
	stateID, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return block.North
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return block.North
	}
	if crafter, ok := block.StateList[stateID].(block.Crafter); ok {
		front, _ := crafter.Orientation.Directions()
		return front
	}
	return block.North
}

// AddItemToSlot tries to add an item to the crafter, used for hopper integration.
// Returns true if the item was added.
func (cm *CrafterManager) AddItemToSlot(cs *CrafterState, itemID int32, count int32) bool {
	// Try to stack with existing matching item
	for i := 0; i < 9; i++ {
		if cs.IsDisabled(i) {
			continue
		}
		if cs.Slots[i].ID == itemID && cs.Slots[i].Count < 64 {
			space := int32(64) - cs.Slots[i].Count
			if count <= space {
				cs.Slots[i].Count += count
				return true
			}
		}
	}
	// Try to place in empty slot
	for i := 0; i < 9; i++ {
		if cs.IsDisabled(i) {
			continue
		}
		if cs.Slots[i].ID == 0 {
			cs.Slots[i] = game.ItemStack{ID: itemID, Count: count}
			return true
		}
	}
	return false
}

