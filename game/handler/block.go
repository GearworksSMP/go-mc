package handler

import (
	"log"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// faceOffsets maps block face index to the XYZ offset for block placement.
var faceOffsets = [6][3]int{
	{0, -1, 0}, // 0: Bottom (-Y)
	{0, 1, 0},  // 1: Top (+Y)
	{0, 0, -1}, // 2: North (-Z)
	{0, 0, 1},  // 3: South (+Z)
	{-1, 0, 0}, // 4: West (-X)
	{1, 0, 0},  // 5: East (+X)
}

// BlockHandler processes block break/place packets and creative inventory.
type BlockHandler struct {
	World   game.World
	Manager *game.PlayerManager
	Logger  *log.Logger
}

// HandlePacket processes a single packet for the given player.
// Returns true if the packet was handled.
func (h *BlockHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundPlayerAction:
		h.handlePlayerAction(player, p)
		return true

	case packetid.ServerboundUseItemOn:
		h.handleUseItemOn(player, p)
		return true

	case packetid.ServerboundSetCreativeModeSlot:
		if player.GameMode == 1 { // only in creative
			h.handleCreativeSlot(player, p)
		}
		return true

	case packetid.ServerboundSetCarriedItem:
		h.handleSetCarriedItem(player, p)
		return true
	}

	return false
}

// handlePlayerAction handles ServerboundPlayerAction.
// In creative mode, action=0 causes instant break.
// In survival mode, action=0 starts digging, action=1 cancels, action=2 finishes.
// Actions 3/4 are Q-key drops (all game modes).
func (h *BlockHandler) handlePlayerAction(player *game.Player, p pk.Packet) {
	var action pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&action, &pos, &face, &sequence); err != nil {
		return
	}

	// Q-key drops (all game modes)
	switch action {
	case 3: // drop item
		h.dropFromHotbar(player, false)
		return
	case 4: // drop item stack (ctrl+Q)
		h.dropFromHotbar(player, true)
		return
	}

	if player.GameMode == 1 { // creative — instant break
		if action == 0 {
			h.breakBlock(player, pos.X, pos.Y, pos.Z, int32(sequence))
		}
		return
	}

	// Survival mode
	switch action {
	case 0: // started_digging
		CancelEating(player)
		player.Digging = true
		player.DigX, player.DigY, player.DigZ = pos.X, pos.Y, pos.Z
		player.DigStartTime = time.Now()
		// Broadcast break progress stage 0
		h.broadcastBlockDestruction(player.EID, pos.X, pos.Y, pos.Z, 0)
	case 1: // cancelled_digging
		player.Digging = false
		// Reset break progress (stage 10+)
		h.broadcastBlockDestruction(player.EID, player.DigX, player.DigY, player.DigZ, 10)
	case 2: // finished_digging
		if player.Digging && player.DigX == pos.X && player.DigY == pos.Y && player.DigZ == pos.Z {
			if h.validateBreakTime(player, pos.X, pos.Y, pos.Z) {
				h.breakBlock(player, pos.X, pos.Y, pos.Z, int32(sequence))
			} else {
				// Reject: send ack only, don't break
				h.sendAck(player, int32(sequence))
			}
		}
		player.Digging = false
	}
}

// dropFromHotbar drops items from the player's currently held hotbar slot.
// If dropAll is true, drops the entire stack; otherwise drops one item.
func (h *BlockHandler) dropFromHotbar(player *game.Player, dropAll bool) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID == 0 || invItem.Count <= 0 {
		return
	}

	if dropAll {
		*invItem = game.ItemStack{}
	} else {
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
	}
	SendSlotUpdate(player, slot)
}

// handleUseItemOn handles ServerboundUseItemOn (block placement).
func (h *BlockHandler) handleUseItemOn(player *game.Player, p pk.Packet) {
	var hand pk.VarInt
	var pos pk.Position
	var face pk.VarInt
	var cursorX, cursorY, cursorZ pk.Float
	var insideBlock pk.Boolean
	var sequence pk.VarInt
	if err := p.Scan(&hand, &pos, &face, &cursorX, &cursorY, &cursorZ, &insideBlock, &sequence); err != nil {
		return
	}

	_ = hand

	if int(face) < 0 || int(face) >= len(faceOffsets) {
		return
	}

	// Check if the clicked block is a crafting table (and player isn't sneaking)
	if !player.Sneaking {
		stateID, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
		if err == nil {
			blockName := BlockNameFromState(int(stateID))
			if blockName == "crafting_table" {
				h.openCraftingTable(player)
				h.sendAck(player, int32(sequence))
				return
			}
		}
	}

	offset := faceOffsets[face]
	placeX := pos.X + offset[0]
	placeY := pos.Y + offset[1]
	placeZ := pos.Z + offset[2]

	stateID := h.heldBlockState(player)
	if stateID < 0 {
		// Not holding a placeable block
		h.sendAck(player, int32(sequence))
		return
	}

	h.placeBlock(player, placeX, placeY, placeZ, level.BlocksState(stateID), int32(sequence))
}

// openCraftingTable opens a 3x3 crafting window for the player.
func (h *BlockHandler) openCraftingTable(player *game.Player) {
	player.OpenWindowID = 1
	// Clear crafting grid
	for i := range player.CraftingGrid {
		player.CraftingGrid[i] = game.ItemStack{}
	}

	// Send ClientboundOpenScreen: windowID=1, type=12 (crafting), title
	title := chat.Text("Crafting")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(1),  // window ID
		pk.VarInt(12), // menu type: crafting (3x3)
		title,
	))

	// Send initial window content
	SendCraftingWindowContent(player)
}

// handleCreativeSlot handles ServerboundSetCreativeModeSlot.
func (h *BlockHandler) handleCreativeSlot(player *game.Player, p pk.Packet) {
	var slotNumber pk.VarInt
	var count pk.VarInt
	if err := p.Scan(&slotNumber, &count); err != nil {
		return
	}

	if count == 0 {
		player.SetCreativeSlot(int16(slotNumber), 0)
		return
	}

	// Read item ID (next VarInt after count)
	var itemID pk.VarInt
	if err := p.Scan(&slotNumber, &count, &itemID); err != nil {
		return
	}

	player.SetCreativeSlot(int16(slotNumber), int32(itemID))
}

// handleSetCarriedItem handles ServerboundSetCarriedItem (hotbar slot selection).
func (h *BlockHandler) handleSetCarriedItem(player *game.Player, p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
	CancelEating(player)
	player.SetHeldSlot(int16(slot))
}

// heldBlockState returns the block state ID for the item the player is holding,
// or -1 if the player isn't holding a placeable block.
func (h *BlockHandler) heldBlockState(player *game.Player) int32 {
	itemID := player.HeldItemID()
	if itemID <= 0 {
		return -1
	}

	it, ok := item.ByID[item.ID(itemID)]
	if !ok {
		return -1
	}

	b, ok := block.FromID["minecraft:"+it.Name]
	if !ok {
		return -1
	}

	sid, ok := block.ToStateID[b]
	if !ok {
		return -1
	}

	return int32(sid)
}

// breakBlock removes a block (sets to air) and broadcasts the change.
func (h *BlockHandler) breakBlock(player *game.Player, x, y, z int, sequence int32) {
	oldState, err := h.World.SetBlock(x, y, z, 0) // 0 = air
	if err != nil {
		h.logf("Error breaking block at (%d,%d,%d): %v", x, y, z, err)
		return
	}

	h.broadcastBlockUpdate(x, y, z, 0)
	h.sendAck(player, sequence)

	// Drop item in survival mode
	if player.GameMode == 0 && oldState > 0 {
		h.dropBlockItem(player, int(oldState))
		// Decrement tool durability
		h.decrementToolDurability(player)
	}

	h.logf("Player %s broke block at (%d, %d, %d)", player.Name, x, y, z)
}

// dropBlockItem adds the broken block's item to the player's inventory and sends the slot update.
func (h *BlockHandler) dropBlockItem(player *game.Player, stateID int) {
	if stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	blockName := block.StateList[stateID].ID()
	itemID, ok := blockToItemID(blockName)
	if !ok {
		return
	}
	slot := player.Inventory.AddItem(itemID, 1)
	if slot < 0 {
		return // inventory full
	}
	SendSlotUpdate(player, slot)
}

// validateBreakTime checks whether the player has spent enough time mining a block.
// Returns true if the break is allowed. Unknown blocks always pass.
func (h *BlockHandler) validateBreakTime(player *game.Player, x, y, z int) bool {
	stateID, err := h.World.GetBlock(x, y, z)
	if err != nil {
		return true // can't look up — allow
	}
	blockName := BlockNameFromState(int(stateID))
	if blockName == "" {
		return true
	}
	heldName := ItemNameByID(player.Inventory[player.HeldSlot+36].ID)
	expected := CalculateBreakTime(blockName, heldName)
	if expected <= 0 {
		return true // instant break or unknown
	}
	if expected < 0 {
		return false // unbreakable
	}
	elapsed := time.Since(player.DigStartTime).Seconds()
	// 20% tolerance for network latency
	return elapsed >= expected*0.8
}

// blockToItemID maps a block name (e.g. "minecraft:dirt") to its item ID.
func blockToItemID(blockName string) (int32, bool) {
	// Strip "minecraft:" prefix for matching against item names
	name := blockName
	if len(name) > 10 && name[:10] == "minecraft:" {
		name = name[10:]
	}
	for id, itm := range item.ByID {
		if itm.Name == name {
			return int32(id), true
		}
	}
	return 0, false
}

// placeBlock places a block and broadcasts the change.
func (h *BlockHandler) placeBlock(player *game.Player, x, y, z int, state level.BlocksState, sequence int32) {
	_, err := h.World.SetBlock(x, y, z, state)
	if err != nil {
		h.logf("Error placing block at (%d,%d,%d): %v", x, y, z, err)
		h.sendAck(player, sequence)
		return
	}

	h.broadcastBlockUpdate(x, y, z, int32(state))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	h.logf("Player %s placed block at (%d, %d, %d) state=%d", player.Name, x, y, z, state)
}

// broadcastBlockDestruction sends ClientboundBlockDestruction to all players.
// stage 0-9 = progress, 10+ = reset.
func (h *BlockHandler) broadcastBlockDestruction(entityID int32, x, y, z int, stage int8) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockDestruction,
		pk.VarInt(entityID),
		pk.Position{X: x, Y: y, Z: z},
		pk.Byte(stage),
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// broadcastBlockUpdate sends ClientboundBlockUpdate to all connected players.
func (h *BlockHandler) broadcastBlockUpdate(x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// sendAck sends ClientboundBlockChangedAck to the player.
func (h *BlockHandler) sendAck(player *game.Player, sequence int32) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))
}

// decrementToolDurability reduces the held tool's durability by 1 after breaking a block.
// If durability reaches 0, the tool breaks (slot is cleared).
func (h *BlockHandler) decrementToolDurability(player *game.Player) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.MaxDurability <= 0 {
		return // not a tool
	}
	invItem.Durability--
	if invItem.Durability <= 0 {
		*invItem = game.ItemStack{} // tool breaks
	}
	SendSlotUpdate(player, slot)
}

func (h *BlockHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
