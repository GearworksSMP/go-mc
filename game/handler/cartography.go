package handler

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// CartographyWindowID is the container window ID for the cartography table UI.
const CartographyWindowID = 24

// CartographyManager handles cartography table interactions: clone, zoom out, lock maps.
type CartographyManager struct {
	Manager *game.PlayerManager
	MapMgr  *MapManager
	Logger  *log.Logger
}

// NewCartographyManager creates a new CartographyManager.
func NewCartographyManager(manager *game.PlayerManager, mapMgr *MapManager, logger *log.Logger) *CartographyManager {
	return &CartographyManager{
		Manager: manager,
		MapMgr:  mapMgr,
		Logger:  logger,
	}
}

// OpenCartographyTable opens the cartography table UI for a player.
// Menu type 11 = cartography_table in the Minecraft registry.
func (cm *CartographyManager) OpenCartographyTable(player *game.Player) {
	player.OpenWindowID = CartographyWindowID

	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(CartographyWindowID),
		pk.VarInt(11), // menu type: cartography_table
		chat.Text("Cartography Table"),
	))
}

// HandleCartographyClick processes a click in the cartography table output slot.
// Slot layout: 0 = map input, 1 = second input (empty_map/paper/glass_pane), 2 = output.
func (cm *CartographyManager) HandleCartographyClick(player *game.Player, slotID int) {
	if player.OpenWindowID != CartographyWindowID || slotID != 2 {
		return
	}

	// Determine input pair: one slot must hold a filled_map, the other the material.
	// We check the main hand and offhand (slot 45) for the pair.
	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]
	if held.ID <= 0 || held.Count <= 0 {
		return
	}
	heldName := ItemNameByID(held.ID)

	offhand := &player.Inventory[45]
	offhandName := ""
	if offhand.ID > 0 && offhand.Count > 0 {
		offhandName = ItemNameByID(offhand.ID)
	}

	mapSlot, materialSlot := -1, -1
	var materialName string

	if heldName == "filled_map" {
		mapSlot = heldSlot
		if offhandName != "" {
			materialSlot = 45
			materialName = offhandName
		}
	} else if offhandName == "filled_map" {
		mapSlot = 45
		materialSlot = heldSlot
		materialName = heldName
	}

	if mapSlot < 0 {
		return
	}

	switch materialName {
	case "map":
		cm.cloneMap(player, mapSlot, materialSlot)
	case "paper":
		cm.zoomOut(player, mapSlot, materialSlot)
	case "glass_pane":
		cm.lockMap(player, mapSlot, materialSlot)
	}
}

// cloneMap duplicates an existing map.
func (cm *CartographyManager) cloneMap(player *game.Player, mapSlot, materialSlot int) {
	if cm.MapMgr == nil {
		return
	}

	srcMap := cm.findHeldMap(player, mapSlot)
	if srcMap == nil {
		return
	}

	cm.MapMgr.mu.Lock()
	newID := cm.MapMgr.nextMapID
	cm.MapMgr.nextMapID++
	clone := &MapData{
		ID:      newID,
		CenterX: srcMap.CenterX,
		CenterZ: srcMap.CenterZ,
		Scale:   srcMap.Scale,
		Locked:  srcMap.Locked,
		Dirty:   true,
	}
	copy(clone.Pixels[:], srcMap.Pixels[:])
	cm.MapMgr.maps[newID] = clone
	cm.MapMgr.mu.Unlock()

	consumeOneItem(player, materialSlot)
	giveFilledMap(player)
	cm.MapMgr.SendMapData(player, newID)
}

// zoomOut creates a new map with scale+1 (max 4).
func (cm *CartographyManager) zoomOut(player *game.Player, mapSlot, materialSlot int) {
	if cm.MapMgr == nil {
		return
	}

	srcMap := cm.findHeldMap(player, mapSlot)
	if srcMap == nil {
		return
	}

	if srcMap.Scale >= 4 {
		return
	}

	cm.MapMgr.mu.Lock()
	newID := cm.MapMgr.nextMapID
	cm.MapMgr.nextMapID++
	zoomed := &MapData{
		ID:      newID,
		CenterX: srcMap.CenterX,
		CenterZ: srcMap.CenterZ,
		Scale:   srcMap.Scale + 1,
		Locked:  srcMap.Locked,
		Dirty:   true,
	}
	cm.MapMgr.maps[newID] = zoomed
	cm.MapMgr.mu.Unlock()

	cm.MapMgr.renderMap(zoomed)

	consumeOneItem(player, materialSlot)
	consumeOneItem(player, mapSlot)
	giveFilledMap(player)
	cm.MapMgr.SendMapData(player, newID)
}

// lockMap sets the map's Locked flag to true, preventing further updates.
func (cm *CartographyManager) lockMap(player *game.Player, mapSlot, materialSlot int) {
	if cm.MapMgr == nil {
		return
	}

	srcMap := cm.findHeldMap(player, mapSlot)
	if srcMap == nil || srcMap.Locked {
		return
	}

	cm.MapMgr.mu.Lock()
	srcMap.Locked = true
	srcMap.Dirty = true
	cm.MapMgr.mu.Unlock()

	consumeOneItem(player, materialSlot)
	cm.MapMgr.SendMapData(player, srcMap.ID)
}

// findHeldMap returns the MapData for the map item in the given slot.
// Returns the first map in the manager as a placeholder (real impl would use item NBT map ID).
func (cm *CartographyManager) findHeldMap(player *game.Player, slot int) *MapData {
	item := &player.Inventory[slot]
	if item.ID <= 0 || item.Count <= 0 {
		return nil
	}
	if ItemNameByID(item.ID) != "filled_map" {
		return nil
	}

	cm.MapMgr.mu.Lock()
	defer cm.MapMgr.mu.Unlock()
	for _, md := range cm.MapMgr.maps {
		return md
	}
	return nil
}

// giveFilledMap adds one filled_map item to the player's inventory.
func giveFilledMap(player *game.Player) {
	filledMapID := itemIDByName("filled_map")
	if filledMapID > 0 {
		addSlot := player.Inventory.AddItem(filledMapID, 1)
		if addSlot >= 0 {
			SendSlotUpdate(player, addSlot)
		}
	}
}

// consumeOneItem decrements the item count in the given slot by 1.
func consumeOneItem(player *game.Player, slot int) {
	if slot < 0 || slot >= len(player.Inventory) {
		return
	}
	item := &player.Inventory[slot]
	item.Count--
	if item.Count <= 0 {
		*item = game.ItemStack{}
	}
	SendSlotUpdate(player, slot)
}
