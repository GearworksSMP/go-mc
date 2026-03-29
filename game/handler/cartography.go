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
// The player takes from slot 2 to perform the operation.
func (cm *CartographyManager) HandleCartographyClick(player *game.Player, slotID int) {
	if player.OpenWindowID != CartographyWindowID || slotID != 2 {
		return
	}

	// Read input items from inventory (using the first two slots after the player's main inv)
	// In the cartography table container, slots 0 and 1 are the two inputs.
	// For simplicity, we check the player's held item and cursor/hotbar to determine the operation.
	// In practice, the server tracks container slots. Here we use the player's hotbar items
	// to determine what operation was intended.

	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]
	if held.ID <= 0 || held.Count <= 0 {
		return
	}
	heldName := ItemNameByID(held.ID)

	// Determine operation based on what the player is holding
	// The cartography table needs a map + a second item. We simulate by checking
	// the held item and the offhand (slot 45) for the pair.
	offhand := &player.Inventory[45]
	offhandName := ""
	if offhand.ID > 0 && offhand.Count > 0 {
		offhandName = ItemNameByID(offhand.ID)
	}

	// Try both orientations: held=map + offhand=material, or held=material + offhand=map
	mapSlot, materialSlot := -1, -1
	var mapName, materialName string

	if heldName == "filled_map" {
		mapSlot = heldSlot
		mapName = heldName
		if offhandName != "" {
			materialSlot = 45
			materialName = offhandName
		}
	} else if offhandName == "filled_map" {
		mapSlot = 45
		mapName = offhandName
		materialSlot = heldSlot
		materialName = heldName
	}

	if mapSlot < 0 || mapName != "filled_map" {
		return
	}

	switch materialName {
	case "map":
		// Clone operation: duplicate the map
		cm.cloneMap(player, mapSlot, materialSlot)
	case "paper":
		// Zoom out: increase scale by 1 (max 4)
		cm.zoomOut(player, mapSlot, materialSlot)
	case "glass_pane":
		// Lock: prevent further map updates
		cm.lockMap(player, mapSlot, materialSlot)
	}
}

// cloneMap duplicates an existing map.
func (cm *CartographyManager) cloneMap(player *game.Player, mapSlot, materialSlot int) {
	if cm.MapMgr == nil {
		return
	}

	// For now we use a simple map ID scheme (held item NBT would normally have the map ID).
	// Get the first available map as the source.
	srcMap := cm.findHeldMap(player, mapSlot)
	if srcMap == nil {
		return
	}

	// Create a clone with identical data
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

	// Consume the empty map
	consumeOneItem(player, materialSlot)

	// Give the cloned map
	filledMapID := itemIDByName("filled_map")
	if filledMapID > 0 {
		addSlot := player.Inventory.AddItem(filledMapID, 1)
		if addSlot >= 0 {
			SendSlotUpdate(player, addSlot)
		}
	}

	// Send the cloned map data to the player
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
		return // already at max zoom
	}

	// Create a new map with increased scale
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

	// Re-render at new scale
	cm.MapMgr.renderMap(zoomed)

	// Consume paper and the original map
	consumeOneItem(player, materialSlot)
	consumeOneItem(player, mapSlot)

	// Give the zoomed-out map
	filledMapID := itemIDByName("filled_map")
	if filledMapID > 0 {
		addSlot := player.Inventory.AddItem(filledMapID, 1)
		if addSlot >= 0 {
			SendSlotUpdate(player, addSlot)
		}
	}

	cm.MapMgr.SendMapData(player, newID)
}

// lockMap sets the map's Locked flag to true, preventing further updates.
func (cm *CartographyManager) lockMap(player *game.Player, mapSlot, materialSlot int) {
	if cm.MapMgr == nil {
		return
	}

	srcMap := cm.findHeldMap(player, mapSlot)
	if srcMap == nil || srcMap.Locked {
		return // already locked
	}

	// Lock the map in place
	cm.MapMgr.mu.Lock()
	srcMap.Locked = true
	srcMap.Dirty = true
	cm.MapMgr.mu.Unlock()

	// Consume the glass pane
	consumeOneItem(player, materialSlot)

	// Re-send the map data with Locked=true
	cm.MapMgr.SendMapData(player, srcMap.ID)
}

// findHeldMap returns the MapData for the map item in the given slot.
// In a full implementation, the map ID would be stored in item NBT.
// Here we return the first map in the manager as a placeholder.
func (cm *CartographyManager) findHeldMap(player *game.Player, slot int) *MapData {
	item := &player.Inventory[slot]
	if item.ID <= 0 || item.Count <= 0 {
		return nil
	}
	name := ItemNameByID(item.ID)
	if name != "filled_map" {
		return nil
	}

	// Return the first available map (simplified; real impl uses item NBT map ID)
	cm.MapMgr.mu.Lock()
	defer cm.MapMgr.mu.Unlock()
	for _, md := range cm.MapMgr.maps {
		return md
	}
	return nil
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
