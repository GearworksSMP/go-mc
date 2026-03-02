package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// DispenserWindowID is the window ID used for dispensers/droppers.
const DispenserWindowID = 9

// DispenserState holds the state of a single dispenser or dropper block.
type DispenserState struct {
	Slots    [9]game.ItemStack
	Pos      [3]int
	IsDroper bool // true if dropper, false if dispenser
}

// DispenserManager tracks all dispensers and droppers in the world.
type DispenserManager struct {
	Dispensers   map[[3]int]*DispenserState
	mu           sync.RWMutex
	Manager      *game.PlayerManager
	World        game.World
	ItemEntities *ItemEntityManager
	ArrowMgr     *ArrowManager
}

// NewDispenserManager creates a new DispenserManager.
func NewDispenserManager(manager *game.PlayerManager, world game.World, itemEntities *ItemEntityManager) *DispenserManager {
	return &DispenserManager{
		Dispensers:   make(map[[3]int]*DispenserState),
		Manager:      manager,
		World:        world,
		ItemEntities: itemEntities,
	}
}

// GetOrCreate returns the dispenser state at the given position, creating it if needed.
func (dm *DispenserManager) GetOrCreate(x, y, z int, isDropper bool) *DispenserState {
	pos := [3]int{x, y, z}
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if ds, ok := dm.Dispensers[pos]; ok {
		return ds
	}
	ds := &DispenserState{Pos: pos, IsDroper: isDropper}
	dm.Dispensers[pos] = ds
	return ds
}

// Get returns the dispenser state at the given position, or nil.
func (dm *DispenserManager) Get(x, y, z int) *DispenserState {
	pos := [3]int{x, y, z}
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.Dispensers[pos]
}

// OpenDispenser opens a dispenser/dropper window for the player.
func (dm *DispenserManager) OpenDispenser(player *game.Player, x, y, z int, isDropper bool) {
	ds := dm.GetOrCreate(x, y, z, isDropper)

	player.OpenWindowID = DispenserWindowID
	player.OpenDispenserPos = [3]int{x, y, z}

	title := chat.Text("Dispenser")
	if isDropper {
		title = chat.Text("Dropper")
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(DispenserWindowID),
		pk.VarInt(6), // menu type: generic_3x3
		title,
	))

	SendDispenserWindowContent(player, ds)
}

// SendDispenserWindowContent sends the full dispenser window content.
// Layout: 45 slots = 9 dispenser + 27 main inv + 9 hotbar
func SendDispenserWindowContent(player *game.Player, ds *DispenserState) {
	stateID := player.NextStateID()
	totalSlots := 9 + 27 + 9
	buf := make([]game.Slot261, totalSlots)

	// Dispenser slots 0-8
	for i := 0; i < 9; i++ {
		buf[i] = ds.Slots[i].ToSlot()
	}
	// Main inventory (player slots 9-35) → window slots 9-35
	for i := 9; i <= 35; i++ {
		buf[i] = player.Inventory[i].ToSlot()
	}
	// Hotbar (player slots 36-44) → window slots 36-44
	for i := 36; i <= 44; i++ {
		buf[i] = player.Inventory[i].ToSlot()
	}

	slots := game.Slot261Array(buf)
	cursor := player.CursorItem.ToSlot()

	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(DispenserWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// DispenserSlot returns a pointer to the ItemStack for a window slot.
func DispenserSlot(player *game.Player, ds *DispenserState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot < 9:
		return &ds.Slots[windowSlot]
	case windowSlot >= 9 && windowSlot <= 35:
		return &player.Inventory[windowSlot] // direct mapping
	case windowSlot >= 36 && windowSlot <= 44:
		return &player.Inventory[windowSlot] // direct mapping
	}
	return nil
}

// Activate fires or drops one item from a dispenser/dropper when powered by redstone.
func (dm *DispenserManager) Activate(x, y, z int) {
	dm.mu.RLock()
	ds := dm.Dispensers[[3]int{x, y, z}]
	dm.mu.RUnlock()

	if ds == nil {
		return
	}

	// Find a random non-empty slot
	var occupied []int
	for i := range ds.Slots {
		if ds.Slots[i].ID > 0 && ds.Slots[i].Count > 0 {
			occupied = append(occupied, i)
		}
	}
	if len(occupied) == 0 {
		// Click sound for empty dispenser
		BroadcastSound(dm.Manager, 555, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.2)
		return
	}

	slotIdx := occupied[rand.Intn(len(occupied))]
	item := &ds.Slots[slotIdx]
	itemName := ItemNameByID(item.ID)

	// Get facing direction
	facing := dm.getFacing(x, y, z)
	dx, dy, dz := directionOffset(facing)
	outX := float64(x) + float64(dx) + 0.5
	outY := float64(y) + float64(dy) + 0.5
	outZ := float64(z) + float64(dz) + 0.5

	if ds.IsDroper {
		// Dropper: always drop as item entity
		if dm.ItemEntities != nil {
			dm.ItemEntities.SpawnItem(dm.Manager, outX, outY, outZ, item.ID, 1, 10)
		}
		item.Count--
		if item.Count <= 0 {
			*item = game.ItemStack{}
		}
		// Dropper click sound
		BroadcastSound(dm.Manager, 555, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
		return
	}

	// Dispenser: shoot projectiles or drop items
	switch itemName {
	case "arrow":
		if dm.ArrowMgr != nil {
			dm.ArrowMgr.SpawnPlayerArrow(0, outX, outY, outZ,
				float64(dx), float64(dy), float64(dz), 2.0)
		}
		item.Count--
		if item.Count <= 0 {
			*item = game.ItemStack{}
		}
		BroadcastSound(dm.Manager, SoundArrowShoot, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	default:
		// Drop as item entity for any other item
		if dm.ItemEntities != nil {
			dm.ItemEntities.SpawnItem(dm.Manager, outX, outY, outZ, item.ID, 1, 10)
		}
		item.Count--
		if item.Count <= 0 {
			*item = game.ItemStack{}
		}
		BroadcastSound(dm.Manager, 555, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	}
}

// getFacing returns the facing direction for a dispenser/dropper at the given position.
func (dm *DispenserManager) getFacing(x, y, z int) block.Direction {
	stateID, err := dm.World.GetBlock(x, y, z)
	if err != nil {
		return block.North
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return block.North
	}
	switch d := block.StateList[stateID].(type) {
	case block.Dispenser:
		return d.Facing
	case block.Dropper:
		return d.Facing
	}
	return block.North
}
