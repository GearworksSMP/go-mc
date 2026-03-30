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

// HopperWindowID is the window ID used for hoppers.
const HopperWindowID = 8

// HopperState holds the state of a single hopper block.
type HopperState struct {
	Slots    [5]game.ItemStack
	Pos      [3]int
	Cooldown int // ticks until next transfer (8-tick interval)
}

// HopperManager tracks all hoppers in the world and handles item transfer.
type HopperManager struct {
	Hoppers    map[[3]int]*HopperState
	mu         sync.RWMutex
	Manager    *game.PlayerManager
	World      game.World
	Chests     *ChestManager
	Furnaces   *FurnaceManager
	CrafterMgr *CrafterManager
	WireMgr    *WireManager        // for comparator notifications on item transfers
	RedstoneMgr *RedstoneManager   // for checking if hopper is locked by redstone
}

// NewHopperManager creates a new HopperManager.
func NewHopperManager(manager *game.PlayerManager, world game.World) *HopperManager {
	return &HopperManager{
		Hoppers: make(map[[3]int]*HopperState),
		Manager: manager,
		World:   world,
	}
}

// GetOrCreate returns the hopper state at the given position, creating it if needed.
func (hm *HopperManager) GetOrCreate(x, y, z int) *HopperState {
	pos := [3]int{x, y, z}
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if hs, ok := hm.Hoppers[pos]; ok {
		return hs
	}
	hs := &HopperState{Pos: pos}
	hm.Hoppers[pos] = hs
	return hs
}

// Get returns the hopper state at the given position, or nil.
func (hm *HopperManager) Get(x, y, z int) *HopperState {
	pos := [3]int{x, y, z}
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.Hoppers[pos]
}

// OpenHopper opens a hopper window for the player.
func (hm *HopperManager) OpenHopper(player *game.Player, x, y, z int) {
	hs := hm.GetOrCreate(x, y, z)

	player.OpenWindowID = HopperWindowID
	player.OpenHopperPos = [3]int{x, y, z}

	title := chat.Text("Hopper")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(HopperWindowID),
		pk.VarInt(13), // menu type: hopper (5 slots)
		title,
	))

	SendHopperWindowContent(player, hs)
}

// SendHopperWindowContent sends the full hopper window content.
// Layout: 41 slots = 5 hopper + 27 main inv + 9 hotbar
func SendHopperWindowContent(player *game.Player, hs *HopperState) {
	stateID := player.NextStateID()
	totalSlots := 5 + 27 + 9
	buf := make([]game.Slot261, totalSlots)

	// Hopper slots 0-4
	for i := 0; i < 5; i++ {
		buf[i] = hs.Slots[i].ToSlot()
	}
	// Main inventory (player slots 9-35) → window slots 5-31
	for i := 9; i <= 35; i++ {
		buf[5+i-9] = player.Inventory[i].ToSlot()
	}
	// Hotbar (player slots 36-44) → window slots 32-40
	for i := 36; i <= 44; i++ {
		buf[32+i-36] = player.Inventory[i].ToSlot()
	}

	slots := game.Slot261Array(buf)
	cursor := player.CursorItem.ToSlot()

	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(HopperWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// HopperSlot returns a pointer to the ItemStack for a window slot.
func HopperSlot(player *game.Player, hs *HopperState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot < 5:
		return &hs.Slots[windowSlot]
	case windowSlot >= 5 && windowSlot <= 31:
		return &player.Inventory[9+windowSlot-5]
	case windowSlot >= 32 && windowSlot <= 40:
		return &player.Inventory[36+windowSlot-32]
	}
	return nil
}

// SaveAll serializes all hopper states for persistence.
func (hm *HopperManager) SaveAll(dim string) []store.BlockEntityData {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	var result []store.BlockEntityData
	for pos, hs := range hm.Hoppers {
		items := containerItemSlots(hs.Slots[:])
		data, err := json.Marshal(map[string]interface{}{"items": items})
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "hopper", Data: data,
		})
	}
	return result
}

// LoadAll restores hopper states from persisted data.
func (hm *HopperManager) LoadAll(entities []store.BlockEntityData) {
	for _, e := range entities {
		if e.Type != "hopper" {
			continue
		}
		var raw struct {
			Items []persistedItem `json:"items"`
		}
		if err := json.Unmarshal(e.Data, &raw); err != nil {
			continue
		}
		hs := hm.GetOrCreate(e.X, e.Y, e.Z)
		loadItemSlots(hs.Slots[:], raw.Items)
	}
}

// Tick processes hopper item transfers every 8 ticks.
func (hm *HopperManager) Tick(tick int64) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for _, hs := range hm.Hoppers {
		// Redstone-powered hoppers are locked
		if hm.RedstoneMgr != nil && hm.RedstoneMgr.GetPowerLevel(hs.Pos[0], hs.Pos[1], hs.Pos[2]) > 0 {
			continue
		}

		if hs.Cooldown > 0 {
			hs.Cooldown--
			continue
		}

		transferred := false

		// Pull from container above (y+1)
		if !transferred {
			transferred = hm.tryPullFrom(hs, hs.Pos[0], hs.Pos[1]+1, hs.Pos[2])
		}

		// Push to container below (y-1)
		if !transferred {
			transferred = hm.tryPushTo(hs, hs.Pos[0], hs.Pos[1]-1, hs.Pos[2])
		}

		if transferred {
			hs.Cooldown = 8
			if hm.WireMgr != nil {
				hm.WireMgr.notifyNeighborComponents(hs.Pos[0], hs.Pos[1], hs.Pos[2])
			}
		}
	}
}

// tryPullFrom pulls one item from a container above the hopper.
func (hm *HopperManager) tryPullFrom(hs *HopperState, x, y, z int) bool {
	// Check what block is above
	stateID, err := hm.World.GetBlock(x, y, z)
	if err != nil || stateID == 0 {
		return false
	}
	blockName := BlockNameFromState(int(stateID))

	switch blockName {
	case "chest":
		if hm.Chests == nil {
			return false
		}
		cs := hm.Chests.Get(x, y, z)
		if cs == nil {
			return false
		}
		// Pull from first non-empty chest slot
		for i := range cs.Items {
			if cs.Items[i].ID > 0 && cs.Items[i].Count > 0 {
				if hm.addToHopper(hs, cs.Items[i].ID, 1) {
					cs.Items[i].Count--
					if cs.Items[i].Count <= 0 {
						cs.Items[i] = game.ItemStack{}
					}
					return true
				}
			}
		}
	case "furnace":
		if hm.Furnaces == nil {
			return false
		}
		fs := hm.Furnaces.Get(x, y, z)
		if fs == nil {
			return false
		}
		// Pull from furnace output slot
		if fs.Output.ID > 0 && fs.Output.Count > 0 {
			if hm.addToHopper(hs, fs.Output.ID, 1) {
				fs.Output.Count--
				if fs.Output.Count <= 0 {
					fs.Output = game.ItemStack{}
				}
				return true
			}
		}
	case "hopper":
		other := hm.Hoppers[[3]int{x, y, z}]
		if other == nil {
			return false
		}
		for i := range other.Slots {
			if other.Slots[i].ID > 0 && other.Slots[i].Count > 0 {
				if hm.addToHopper(hs, other.Slots[i].ID, 1) {
					other.Slots[i].Count--
					if other.Slots[i].Count <= 0 {
						other.Slots[i] = game.ItemStack{}
					}
					return true
				}
			}
		}
	case "crafter":
		if hm.CrafterMgr == nil {
			return false
		}
		cs := hm.CrafterMgr.Get(x, y, z)
		if cs == nil {
			return false
		}
		for i := 0; i < 9; i++ {
			if cs.Slots[i].ID > 0 && cs.Slots[i].Count > 0 {
				if hm.addToHopper(hs, cs.Slots[i].ID, 1) {
					cs.Slots[i].Count--
					if cs.Slots[i].Count <= 0 {
						cs.Slots[i] = game.ItemStack{}
					}
					return true
				}
			}
		}
	}
	return false
}

// tryPushTo pushes one item from the hopper to a container below.
func (hm *HopperManager) tryPushTo(hs *HopperState, x, y, z int) bool {
	stateID, err := hm.World.GetBlock(x, y, z)
	if err != nil || stateID == 0 {
		return false
	}
	blockName := BlockNameFromState(int(stateID))

	// Find first non-empty hopper slot
	srcSlot := -1
	for i := range hs.Slots {
		if hs.Slots[i].ID > 0 && hs.Slots[i].Count > 0 {
			srcSlot = i
			break
		}
	}
	if srcSlot < 0 {
		return false
	}

	itemID := hs.Slots[srcSlot].ID

	switch blockName {
	case "chest":
		if hm.Chests == nil {
			return false
		}
		cs := hm.Chests.GetOrCreate(x, y, z)
		// Try to add to existing matching stack or empty slot
		for i := range cs.Items {
			if cs.Items[i].ID == itemID && cs.Items[i].Count < 64 {
				cs.Items[i].Count++
				hs.Slots[srcSlot].Count--
				if hs.Slots[srcSlot].Count <= 0 {
					hs.Slots[srcSlot] = game.ItemStack{}
				}
				return true
			}
		}
		for i := range cs.Items {
			if cs.Items[i].ID == 0 {
				cs.Items[i] = game.ItemStack{ID: itemID, Count: 1}
				hs.Slots[srcSlot].Count--
				if hs.Slots[srcSlot].Count <= 0 {
					hs.Slots[srcSlot] = game.ItemStack{}
				}
				return true
			}
		}
	case "furnace":
		if hm.Furnaces == nil {
			return false
		}
		fs := hm.Furnaces.GetOrCreate(x, y, z)
		// Push into furnace input slot
		if fs.Input.ID == 0 {
			fs.Input = game.ItemStack{ID: itemID, Count: 1}
			hs.Slots[srcSlot].Count--
			if hs.Slots[srcSlot].Count <= 0 {
				hs.Slots[srcSlot] = game.ItemStack{}
			}
			return true
		}
		if fs.Input.ID == itemID && fs.Input.Count < 64 {
			fs.Input.Count++
			hs.Slots[srcSlot].Count--
			if hs.Slots[srcSlot].Count <= 0 {
				hs.Slots[srcSlot] = game.ItemStack{}
			}
			return true
		}
	case "hopper":
		other := hm.Hoppers[[3]int{x, y, z}]
		if other == nil {
			other = &HopperState{Pos: [3]int{x, y, z}}
			hm.Hoppers[[3]int{x, y, z}] = other
		}
		if hm.addToHopper(other, itemID, 1) {
			hs.Slots[srcSlot].Count--
			if hs.Slots[srcSlot].Count <= 0 {
				hs.Slots[srcSlot] = game.ItemStack{}
			}
			return true
		}
	case "crafter":
		if hm.CrafterMgr == nil {
			return false
		}
		cs := hm.CrafterMgr.GetOrCreate(x, y, z)
		if hm.CrafterMgr.AddItemToSlot(cs, itemID, 1) {
			hs.Slots[srcSlot].Count--
			if hs.Slots[srcSlot].Count <= 0 {
				hs.Slots[srcSlot] = game.ItemStack{}
			}
			return true
		}
	}
	return false
}

// addToHopper tries to add count items of itemID to a hopper. Returns true on success.
func (hm *HopperManager) addToHopper(hs *HopperState, itemID int32, count int32) bool {
	// Try existing matching stack
	for i := range hs.Slots {
		if hs.Slots[i].ID == itemID && hs.Slots[i].Count < 64 {
			space := int32(64) - hs.Slots[i].Count
			if count <= space {
				hs.Slots[i].Count += count
				return true
			}
		}
	}
	// Try empty slot
	for i := range hs.Slots {
		if hs.Slots[i].ID == 0 {
			hs.Slots[i] = game.ItemStack{ID: itemID, Count: count}
			return true
		}
	}
	return false
}
