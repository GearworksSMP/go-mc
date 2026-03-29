package handler

import (
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

const LecternWindowID = 21

// LecternManager handles lectern book placement and reading.
type LecternManager struct {
	Manager      *game.PlayerManager
	World        game.World
	ItemEntities *ItemEntityManager
	mu           sync.Mutex
	lecterns     map[[3]int]*LecternData
}

// LecternData holds the state of a lectern with a book.
type LecternData struct {
	BookItemID  int32
	PageCount   int
	CurrentPage int
}

// NewLecternManager creates a new LecternManager.
func NewLecternManager(mgr *game.PlayerManager, world game.World, items *ItemEntityManager) *LecternManager {
	return &LecternManager{
		Manager:      mgr,
		World:        world,
		ItemEntities: items,
		lecterns:     make(map[[3]int]*LecternData),
	}
}

// InteractLectern handles right-click on a lectern.
func (lm *LecternManager) InteractLectern(player *game.Player, x, y, z int) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	pos := [3]int{x, y, z}

	// Check block state for has_book
	stateID, err := lm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	lec, ok := block.StateList[stateID].(block.Lectern)
	if !ok {
		return false
	}

	if bool(lec.Has_book) {
		// Open lectern with book — send lectern window
		data := lm.lecterns[pos]
		if data == nil {
			return false
		}
		lm.openLectern(player, x, y, z, data)
		return true
	}

	// Try to place a book from player's hand
	slot := player.HeldSlot
	held := &player.Inventory[36+slot]
	if held.ID == 0 || held.Count == 0 {
		return false
	}
	itemName := ItemNameByID(held.ID)
	if itemName != "written_book" && itemName != "writable_book" {
		return false
	}

	// Place the book
	lm.lecterns[pos] = &LecternData{
		BookItemID:  held.ID,
		PageCount:   15, // default page count (actual parsing deferred)
		CurrentPage: 0,
	}

	// Consume item
	held.Count--
	if held.Count <= 0 {
		*held = game.ItemStack{}
	}
	SendSlotUpdate(player, 36+int(slot))

	// Update block state
	lec.Has_book = true
	if newID, ok2 := block.ToStateID[lec]; ok2 {
		lm.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(lm.Manager, x, y, z, int32(newID))
	}

	// Play book place sound (use generic block place sound)
	BroadcastSound(lm.Manager, BlockPlaceSound("lectern"), SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)

	return true
}

// openLectern sends the lectern window to the player.
func (lm *LecternManager) openLectern(player *game.Player, x, y, z int, data *LecternData) {
	player.OpenWindowID = LecternWindowID

	// Menu type 23 = lectern in 26.1 registry
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(LecternWindowID),
		pk.VarInt(23), // lectern menu type
		pk.String(`{"text":"Lectern"}`),
	))

	// Send book in slot 0
	bookSlot := game.Slot261{
		Count:  1,
		ItemID: data.BookItemID,
	}
	player.StateID++
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.VarInt(LecternWindowID),
		pk.VarInt(player.StateID),
		pk.VarInt(1), // 1 slot
		bookSlot,
		game.Slot261{}, // carried item
	))

	// Send current page via container data
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.VarInt(LecternWindowID),
		pk.Short(0), // property 0 = page
		pk.Short(int16(data.CurrentPage)),
	))
}

// OnLecternBreak should be called when a lectern block is broken to drop any book.
func (lm *LecternManager) OnLecternBreak(x, y, z int) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	pos := [3]int{x, y, z}
	data := lm.lecterns[pos]
	if data == nil {
		return
	}
	delete(lm.lecterns, pos)

	// Drop the book as item entity
	if lm.ItemEntities != nil && data.BookItemID > 0 {
		lm.ItemEntities.SpawnItem(lm.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, data.BookItemID, 1, 10)
	}
}

// TakeBook removes the book from the lectern (for player sneak+click interactions).
func (lm *LecternManager) TakeBook(player *game.Player, x, y, z int) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	pos := [3]int{x, y, z}
	data := lm.lecterns[pos]
	if data == nil {
		return false
	}

	// Give book to player
	slot := player.Inventory.AddItem(data.BookItemID, 1)
	if slot >= 0 {
		SendSlotUpdate(player, slot)
	}

	delete(lm.lecterns, pos)

	// Update block state
	stateID, err := lm.World.GetBlock(x, y, z)
	if err != nil {
		return true
	}
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		if lec, ok := block.StateList[stateID].(block.Lectern); ok {
			lec.Has_book = false
			if newID, ok2 := block.ToStateID[lec]; ok2 {
				lm.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(lm.Manager, x, y, z, int32(newID))
			}
		}
	}

	return true
}
