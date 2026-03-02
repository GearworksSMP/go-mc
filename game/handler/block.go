package handler

import (
	"log"
	"math/rand"
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
	World        game.World
	Manager      *game.PlayerManager
	Logger       *log.Logger
	ItemEntities *ItemEntityManager
	Chests       *ChestManager
	Furnaces     *FurnaceManager
	TimeMgr      *TimeManager
	FluidMgr     *FluidManager
	FallingMgr   *FallingBlockManager
	TreeMgr      *TreeGrowthManager
	CropMgr      *CropManager
	EnchantMgr   *EnchantManager
	AnvilMgr     *AnvilManager
	OnBlockBreak func(blockName string, x, y, z int) // called when a block is broken
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

	// Q-key drops and offhand swap (all game modes)
	switch action {
	case 3: // drop item
		h.dropFromHotbar(player, false)
		return
	case 4: // drop item stack (ctrl+Q)
		h.dropFromHotbar(player, true)
		return
	case 6: // swap item with offhand
		main := int(player.HeldSlot) + 36
		player.Inventory[main], player.Inventory[45] = player.Inventory[45], player.Inventory[main]
		SendSlotUpdate(player, main)
		SendSlotUpdate(player, 45)
		BroadcastEquipment(h.Manager, player)
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
		player.Blocking = false
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

	dropCount := int32(1)
	if dropAll {
		dropCount = invItem.Count
	}

	itemID := invItem.ID

	if dropAll {
		*invItem = game.ItemStack{}
	} else {
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
	}
	SendSlotUpdate(player, slot)

	// Spawn item entity at player position
	if h.ItemEntities != nil {
		px, py, pz := player.Position()
		h.ItemEntities.SpawnItem(h.Manager, px, py+1.3, pz, itemID, dropCount, 40)
	}
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

	// Check if the clicked block is interactive (and player isn't sneaking)
	if !player.Sneaking {
		stateID, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
		if err == nil {
			blockName := BlockNameFromState(int(stateID))
			switch blockName {
			case "crafting_table":
				h.openCraftingTable(player)
				h.sendAck(player, int32(sequence))
				return
			case "chest":
				if h.Chests != nil {
					h.Chests.OpenChest(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "furnace":
				if h.Furnaces != nil {
					h.Furnaces.OpenFurnace(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "enchanting_table":
				if h.EnchantMgr != nil {
					h.EnchantMgr.OpenEnchantingTable(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "anvil", "chipped_anvil", "damaged_anvil":
				if h.AnvilMgr != nil {
					h.AnvilMgr.OpenAnvil(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			default:
				if h.handleBlockInteraction(player, pos.X, pos.Y, pos.Z, int(stateID)) {
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}
	}

	offset := faceOffsets[face]
	placeX := pos.X + offset[0]
	placeY := pos.Y + offset[1]
	placeZ := pos.Z + offset[2]

	// Check if holding a door or bed item (needs special two-block placement)
	heldItemID := player.HeldItemID()
	if heldItemID > 0 {
		heldName := ItemNameByID(heldItemID)
		if isDoorItem(heldName) {
			h.placeDoor(player, placeX, placeY, placeZ, heldName, int32(sequence))
			return
		}
		if heldName == "red_bed" {
			h.placeBed(player, placeX, placeY, placeZ, int32(sequence))
			return
		}
		// Bone meal on sapling → instant tree growth
		if heldName == "bone_meal" && h.TreeMgr != nil {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && isSapling(BlockNameFromState(int(clickedState))) {
				if h.TreeMgr.GrowTree(pos.X, pos.Y, pos.Z) {
					// Consume bone meal in survival
					if player.GameMode == 0 {
						slot := int(player.HeldSlot) + 36
						player.Inventory[slot].Count--
						if player.Inventory[slot].Count <= 0 {
							player.Inventory[slot] = game.ItemStack{}
						}
						SendSlotUpdate(player, slot)
					}
					// Bone meal particles
					BroadcastLevelEvent(h.Manager, 1505, pos.X, pos.Y, pos.Z, 0)
				}
				h.sendAck(player, int32(sequence))
				return
			}
		}

		// Bone meal on crops → advance growth
		if heldName == "bone_meal" && h.CropMgr != nil {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil {
				clickedName := BlockNameFromState(int(clickedState))
				if isCropBlock(clickedName) {
					if h.CropMgr.BoneMealCrop(pos.X, pos.Y, pos.Z) {
						if player.GameMode == 0 {
							slot := int(player.HeldSlot) + 36
							player.Inventory[slot].Count--
							if player.Inventory[slot].Count <= 0 {
								player.Inventory[slot] = game.ItemStack{}
							}
							SendSlotUpdate(player, slot)
						}
						BroadcastLevelEvent(h.Manager, 1505, pos.X, pos.Y, pos.Z, 0)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}

		// Flint and steel on obsidian → light nether portal
		if heldName == "flint_and_steel" {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && isObsidianState(int(clickedState)) {
				if portalBlocks, axis, ok := detectPortalFrame(h.World, pos.X, pos.Y, pos.Z); ok {
					portalBlock := block.NetherPortal{Axis: axis}
					if portalStateID, found := block.ToStateID[portalBlock]; found {
						for _, pb := range portalBlocks {
							h.World.SetBlock(pb[0], pb[1], pb[2], portalStateID)
							h.broadcastBlockUpdate(pb[0], pb[1], pb[2], int32(portalStateID))
						}
						// Consume durability in survival mode
						if player.GameMode == 0 {
							h.decrementToolDurability(player)
						}
						h.logf("Player %s lit nether portal at (%d, %d, %d) axis=%s", player.Name, pos.X, pos.Y, pos.Z, axis)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}

		// Seed planting on farmland
		if isSeedItem(heldName) {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && int(face) == 1 { // top face only
				clickedName := BlockNameFromState(int(clickedState))
				if clickedName == "farmland" {
					if h.plantSeed(player, pos.X, pos.Y+1, pos.Z, heldName, int32(sequence)) {
						return
					}
				}
			}
		}

		// Hoe on dirt/grass_block → farmland
		if isHoeItem(heldName) {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && int(face) == 1 { // top face only
				clickedName := BlockNameFromState(int(clickedState))
				if clickedName == "dirt" || clickedName == "grass_block" {
					farmland := block.Farmland{Moisture: block.Integer(0)}
					if farmID, ok := block.ToStateID[farmland]; ok {
						h.World.SetBlock(pos.X, pos.Y, pos.Z, farmID)
						h.broadcastBlockUpdate(pos.X, pos.Y, pos.Z, int32(farmID))
						if player.GameMode == 0 {
							h.decrementToolDurability(player)
						}
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}
	}

	stateID := h.heldBlockState(player)
	if stateID < 0 {
		// Not holding a placeable block — check for bucket usage
		if heldItemID > 0 {
			heldName := ItemNameByID(heldItemID)
			if action, ok := bucketActions[heldName]; ok {
				h.handleBucket(player, placeX, placeY, placeZ, pos.X, pos.Y, pos.Z, action, heldName, int32(sequence))
				return
			}
		}
		h.sendAck(player, int32(sequence))
		return
	}

	// Check if placement would collide with any player
	if h.wouldCollideWithPlayer(placeX, placeY, placeZ) {
		h.broadcastBlockUpdate(placeX, placeY, placeZ, 0) // resync client
		h.sendAck(player, int32(sequence))
		return
	}

	orientedState := orientBlock(player, int(stateID), int(face))
	h.placeBlock(player, placeX, placeY, placeZ, level.BlocksState(orientedState), int32(sequence))
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
	player.Blocking = false
	player.SetHeldSlot(int16(slot))
	BroadcastEquipment(h.Manager, player)
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

	// Block break particles + sound
	if oldState > 0 {
		BroadcastLevelEvent(h.Manager, 2001, x, y, z, int32(oldState))
	}

	// Drop item in survival mode
	if player.GameMode == 0 && oldState > 0 {
		h.dropBlockItem(player, int(oldState), x, y, z)
		// Decrement tool durability
		h.decrementToolDurability(player)
		// Award ore XP
		blockName := BlockNameFromState(int(oldState))
		if xp := GetOreXP(blockName); xp > 0 {
			AddExperience(player, xp)
		}
		// Mining exhaustion
		player.Exhaustion += 0.005
	}

	// If breaking a bed, also remove the other half
	h.breakBedOtherHalf(int(oldState), x, y, z)

	// If breaking a nether_portal or obsidian, remove connected portal blocks
	if oldState > 0 {
		oldBlockName := BlockNameFromState(int(oldState))
		if oldBlockName == "nether_portal" {
			// The portal block itself was broken; flood-fill remove all connected portals
			removeConnectedPortals(h.World, h.Manager, x, y, z)
		} else if oldBlockName == "obsidian" {
			// Frame obsidian was broken; check adjacent blocks for portal blocks
			removeAdjacentPortals(h.World, h.Manager, x, y, z)
		}
	}

	// Check for falling blocks above
	if h.FallingMgr != nil {
		h.FallingMgr.CheckAndSpawnFalling(x, y+1, z)
	}

	// Check if fluid is above the broken block — schedule downward flow
	if h.FluidMgr != nil {
		h.FluidMgr.CheckFlowDown(x, y+1, z)
	}

	// Unregister sapling if broken
	if h.TreeMgr != nil {
		blockName := BlockNameFromState(int(oldState))
		if isSapling(blockName) {
			h.TreeMgr.UnregisterSapling(x, y, z)
		}
	}

	// Unregister crop if broken
	if h.CropMgr != nil {
		blockName := BlockNameFromState(int(oldState))
		if isCropBlock(blockName) {
			h.CropMgr.UnregisterCrop(x, y, z)
		}
	}

	// Notify persistence layer (for chest/furnace deletion)
	if h.OnBlockBreak != nil {
		blockName := BlockNameFromState(int(oldState))
		h.OnBlockBreak(blockName, x, y, z)
	}

	// Unlink double chests if breaking a chest
	if h.Chests != nil {
		blockName := BlockNameFromState(int(oldState))
		if blockName == "chest" {
			h.Chests.UnlinkChest(x, y, z, h.World, h.Manager)
		}
	}

	h.logf("Player %s broke block at (%d, %d, %d)", player.Name, x, y, z)
}

// dropBlockItem spawns a dropped item entity for the broken block, or adds directly
// to inventory if no ItemEntityManager is available.
func (h *BlockHandler) dropBlockItem(player *game.Player, stateID int, x, y, z int) {
	blockName := BlockNameFromState(stateID)
	if blockName == "" {
		return
	}

	// Crop blocks have special drop logic based on age
	if isCropBlock(blockName) {
		h.dropCropItems(stateID, x, y, z)
		return
	}

	// Check tool requirements
	heldName := ItemNameByID(player.Inventory[player.HeldSlot+36].ID)
	if !CanHarvestBlock(blockName, heldName) {
		return // wrong tool — no drop
	}

	// Get the drop item name (may differ from block name)
	dropName, drops := GetBlockDropItemName(blockName)
	if !drops {
		return // block drops nothing
	}

	itemID := itemIDByName(dropName)
	if itemID <= 0 {
		return
	}

	if h.ItemEntities != nil {
		h.ItemEntities.SpawnItem(h.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, itemID, 1, 10)
	} else {
		slot := player.Inventory.AddItem(itemID, 1)
		if slot < 0 {
			return
		}
		SendSlotUpdate(player, slot)
	}
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
	heldSlot := &player.Inventory[player.HeldSlot+36]
	heldName := ItemNameByID(heldSlot.ID)
	expected := CalculateBreakTime(blockName, heldName)
	if expected <= 0 {
		return true // instant break or unknown
	}
	if expected < 0 {
		return false // unbreakable
	}
	// Efficiency enchantment: multiply speed by (1 + level^2)
	if heldSlot.Enchantments != nil {
		if effLvl := heldSlot.Enchantments["efficiency"]; effLvl > 0 {
			expected /= float64(1 + effLvl*effLvl)
		}
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

// orientBlock adjusts a block's state based on player facing and clicked face.
// Returns the oriented block state ID, or the original if no orientation applies.
func orientBlock(player *game.Player, state int, face int) int {
	if state < 0 || state >= len(block.StateList) || block.StateList[state] == nil {
		return state
	}

	yaw, pitch := player.Rotation()
	facing := yawToDirection(yaw)

	b := block.StateList[state]
	var oriented block.Block

	switch v := b.(type) {
	// Stairs: face toward player, half based on click position
	case block.OakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobblestoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneBrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SpruceStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BirchStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.JungleStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.AcaciaStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.DarkOakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SandstoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobbledDeepslateStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v

	// Logs: axis based on clicked face
	case block.OakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.SpruceLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.BirchLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.JungleLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.AcaciaLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.DarkOakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.CherryLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.MangroveLog:
		v.Axis = faceToAxis(face)
		oriented = v

	// Slabs: type based on click position
	case block.StoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobblestoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.StoneBrickSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SandstoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.OakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SpruceSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.BirchSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.JungleSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.AcaciaSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.DarkOakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobbledDeepslateSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v

	// Furnace, chest facing
	case block.Furnace:
		v.Facing = facing
		oriented = v
	case block.Chest:
		v.Facing = facing
		oriented = v

	// Pumpkin/carved pumpkin
	case block.CarvedPumpkin:
		v.Facing = facing
		oriented = v

	// Glazed terracotta
	case block.WhiteGlazedTerracotta:
		v.Facing = facing
		oriented = v

	// Observer
	case block.Observer:
		v.Facing = yawPitchToFacing6(yaw, pitch)
		oriented = v

	default:
		return state
	}

	if newID, ok := block.ToStateID[oriented]; ok {
		return int(newID)
	}
	return state
}

// faceToAxis converts a clicked face index to a log axis.
func faceToAxis(face int) block.Axis {
	switch face {
	case 0, 1: // bottom, top
		return block.Y
	case 2, 3: // north, south
		return block.Z
	case 4, 5: // west, east
		return block.X
	}
	return block.Y
}

// yawPitchToFacing6 converts yaw+pitch to a 6-direction facing (includes up/down).
func yawPitchToFacing6(yaw, pitch float32) block.Direction {
	if pitch > 45 {
		return block.Down
	}
	if pitch < -45 {
		return block.Up
	}
	return yawToDirection(yaw)
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

	// Register sapling for tree growth
	if h.TreeMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isSapling(placedName) {
			h.TreeMgr.RegisterSapling(x, y, z)
		}
	}

	// Check if placed block should fall
	if h.FallingMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isFallingBlock(placedName) {
			// Check if block below is air
			belowState, err := h.World.GetBlock(x, y-1, z)
			if err == nil && belowState == 0 {
				h.FallingMgr.CheckAndSpawnFalling(x, y, z)
			}
		}
	}

	// Try linking adjacent chests as double chest
	if h.Chests != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "chest" {
			h.Chests.TryLinkDouble(x, y, z, h.World, h.Manager)
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
// Unbreaking enchantment: skip durability loss with probability level/(level+1).
// If durability reaches 0, the tool breaks (slot is cleared).
func (h *BlockHandler) decrementToolDurability(player *game.Player) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.MaxDurability <= 0 {
		return // not a tool
	}
	if invItem.Enchantments != nil {
		if unbreakLvl := invItem.Enchantments["unbreaking"]; unbreakLvl > 0 {
			if rand.Int31n(unbreakLvl+1) > 0 {
				return // unbreaking saved this durability point
			}
		}
	}
	invItem.Durability--
	if invItem.Durability <= 0 {
		*invItem = game.ItemStack{} // tool breaks
	}
	SendSlotUpdate(player, slot)
}

// handleBlockInteraction handles right-click interactions with doors, levers, and buttons.
// Returns true if the interaction was handled (prevents item placement).
func (h *BlockHandler) handleBlockInteraction(player *game.Player, x, y, z int, stateID int) bool {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	b := block.StateList[stateID]

	switch door := b.(type) {
	case block.OakDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.SpruceDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.BirchDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.JungleDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.AcaciaDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.DarkOakDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.CherryDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.MangroveDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.BambooDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.CrimsonDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.WarpedDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.IronDoor:
		return true // iron doors require redstone, no hand interaction
	case block.Lever:
		door.Powered = !door.Powered
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.StoneButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "stone_button", 30)
		return true
	case block.OakButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "oak_button", 20)
		return true
	case block.SpruceButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "spruce_button", 20)
		return true
	case block.BirchButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "birch_button", 20)
		return true
	case block.JungleButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "jungle_button", 20)
		return true
	case block.AcaciaButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "acacia_button", 20)
		return true
	case block.DarkOakButton:
		h.pressButton(x, y, z, door.Face, door.Facing, "dark_oak_button", 20)
		return true
	case block.RedBed:
		h.interactBed(player, x, y, z)
		return true

	// Trapdoors
	case block.OakTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.SpruceTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BirchTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.JungleTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.AcaciaTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CherryTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.DarkOakTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.MangroveTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BambooTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CrimsonTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.WarpedTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true

	// Fence gates
	case block.OakFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.SpruceFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BirchFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.JungleFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.AcaciaFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CherryFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.DarkOakFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.MangroveFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BambooFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CrimsonFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.WarpedFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	}

	return false
}

// toggleDoor updates both halves of a door.
func (h *BlockHandler) toggleDoor(x, y, z int, clickedBlock block.Block, half block.DoubleBlockHalf, facing block.Direction, hinge block.DoorHingeSide, open, powered bool) {
	// Update clicked half
	if newID, ok := block.ToStateID[clickedBlock]; ok {
		h.World.SetBlock(x, y, z, newID)
		h.broadcastBlockUpdate(x, y, z, int32(newID))
	}

	// Update other half
	otherY := y + 1
	if half == block.DoubleBlockHalfUpper {
		otherY = y - 1
	}
	otherHalf := block.DoubleBlockHalfUpper
	if half == block.DoubleBlockHalfUpper {
		otherHalf = block.DoubleBlockHalfLower
	}

	// Get the other half's current state and update its Open property
	otherState, err := h.World.GetBlock(x, otherY, z)
	if err != nil {
		return
	}
	if int(otherState) >= len(block.StateList) || block.StateList[otherState] == nil {
		return
	}

	// Create matching other half using the block name lookup
	otherBlock := block.StateList[otherState]
	switch d := otherBlock.(type) {
	case block.OakDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.SpruceDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.BirchDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.JungleDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.AcaciaDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.DarkOakDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.CherryDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.MangroveDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.BambooDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.CrimsonDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.WarpedDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	}
}

// pressButton sets a button to powered and schedules it to reset.
func (h *BlockHandler) pressButton(x, y, z int, face block.AttachFace, facing block.Direction, buttonType string, _ int) {
	var pressedState block.Block
	switch buttonType {
	case "stone_button":
		pressedState = block.StoneButton{Face: face, Facing: facing, Powered: true}
	case "oak_button":
		pressedState = block.OakButton{Face: face, Facing: facing, Powered: true}
	case "spruce_button":
		pressedState = block.SpruceButton{Face: face, Facing: facing, Powered: true}
	case "birch_button":
		pressedState = block.BirchButton{Face: face, Facing: facing, Powered: true}
	case "jungle_button":
		pressedState = block.JungleButton{Face: face, Facing: facing, Powered: true}
	case "acacia_button":
		pressedState = block.AcaciaButton{Face: face, Facing: facing, Powered: true}
	case "dark_oak_button":
		pressedState = block.DarkOakButton{Face: face, Facing: facing, Powered: true}
	default:
		return
	}

	if newID, ok := block.ToStateID[pressedState]; ok {
		h.World.SetBlock(x, y, z, newID)
		h.broadcastBlockUpdate(x, y, z, int32(newID))
	}

	// Schedule reset (done via tick system — for now, reset after a goroutine delay)
	go func() {
		var resetState block.Block
		switch buttonType {
		case "stone_button":
			resetState = block.StoneButton{Face: face, Facing: facing, Powered: false}
		case "oak_button":
			resetState = block.OakButton{Face: face, Facing: facing, Powered: false}
		case "spruce_button":
			resetState = block.SpruceButton{Face: face, Facing: facing, Powered: false}
		case "birch_button":
			resetState = block.BirchButton{Face: face, Facing: facing, Powered: false}
		case "jungle_button":
			resetState = block.JungleButton{Face: face, Facing: facing, Powered: false}
		case "acacia_button":
			resetState = block.AcaciaButton{Face: face, Facing: facing, Powered: false}
		case "dark_oak_button":
			resetState = block.DarkOakButton{Face: face, Facing: facing, Powered: false}
		}
		// Wait 1.5s for stone, 1s for wood
		delay := 1500 * time.Millisecond
		if buttonType != "stone_button" {
			delay = 1000 * time.Millisecond
		}
		time.Sleep(delay)
		if newID, ok := block.ToStateID[resetState]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
	}()
}

// placeDoor places both halves of a door at the given position.
func (h *BlockHandler) placeDoor(player *game.Player, x, y, z int, doorName string, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	var lowerBlock, upperBlock block.Block

	switch doorName {
	case "oak_door":
		lowerBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "spruce_door":
		lowerBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "birch_door":
		lowerBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "jungle_door":
		lowerBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "acacia_door":
		lowerBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "dark_oak_door":
		lowerBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "cherry_door":
		lowerBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "mangrove_door":
		lowerBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "bamboo_door":
		lowerBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "crimson_door":
		lowerBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "warped_door":
		lowerBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "iron_door":
		lowerBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	default:
		return
	}

	lowerID, ok1 := block.ToStateID[lowerBlock]
	upperID, ok2 := block.ToStateID[upperBlock]
	if !ok1 || !ok2 {
		return
	}

	h.World.SetBlock(x, y, z, lowerID)
	h.World.SetBlock(x, y+1, z, upperID)
	h.broadcastBlockUpdate(x, y, z, int32(lowerID))
	h.broadcastBlockUpdate(x, y+1, z, int32(upperID))
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
}

// yawToDirection converts player yaw to a block.Direction for placement.
func yawToDirection(yaw float32) block.Direction {
	// Normalize yaw to 0-360
	y := float64(yaw)
	for y < 0 {
		y += 360
	}
	for y >= 360 {
		y -= 360
	}
	// Player faces: 0=south, 90=west, 180=north, 270=east
	switch {
	case y >= 315 || y < 45:
		return block.South
	case y >= 45 && y < 135:
		return block.West
	case y >= 135 && y < 225:
		return block.North
	default:
		return block.East
	}
}

// isDoorItem returns true if the item name is a door item.
func isDoorItem(name string) bool {
	switch name {
	case "oak_door", "spruce_door", "birch_door", "jungle_door",
		"acacia_door", "dark_oak_door", "cherry_door", "mangrove_door",
		"bamboo_door", "crimson_door", "warped_door", "iron_door":
		return true
	}
	return false
}

// placeBed places both halves of a red bed at the given position.
func (h *BlockHandler) placeBed(player *game.Player, x, y, z int, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	foot := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartFoot}
	head := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartHead}

	footID, ok1 := block.ToStateID[foot]
	headID, ok2 := block.ToStateID[head]
	if !ok1 || !ok2 {
		h.sendAck(player, sequence)
		return
	}

	// Head block offset based on facing
	hx, hz := bedHeadOffset(facing)
	headX, headZ := x+hx, z+hz

	// Check that head position is air
	headState, err := h.World.GetBlock(headX, y, headZ)
	if err != nil || headState != 0 {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, footID)
	h.World.SetBlock(headX, y, headZ, headID)
	h.broadcastBlockUpdate(x, y, z, int32(footID))
	h.broadcastBlockUpdate(headX, y, headZ, int32(headID))
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
}

// interactBed handles right-clicking a bed: set spawn at night, message during day.
func (h *BlockHandler) interactBed(player *game.Player, x, y, z int) {
	if h.TimeMgr != nil && h.TimeMgr.IsNight() {
		player.HasSpawnPoint = true
		player.SpawnX = float64(x) + 0.5
		player.SpawnY = float64(y) + 0.6 // slightly above bed
		player.SpawnZ = float64(z) + 0.5
		msg := chat.Message{Text: "Respawn point set", Color: "green"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
	} else {
		msg := chat.Message{Text: "You can only sleep at night", Color: "red"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
	}
}

// breakBedOtherHalf removes the other half of a bed when one half is broken.
func (h *BlockHandler) breakBedOtherHalf(stateID int, x, y, z int) {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	bed, ok := block.StateList[stateID].(block.RedBed)
	if !ok {
		return
	}

	ox, oz := bedHeadOffset(bed.Facing)
	if bed.Part == block.BedPartHead {
		// This was the head, remove the foot (opposite direction)
		ox, oz = -ox, -oz
	}

	otherX, otherZ := x+ox, z+oz
	otherState, err := h.World.GetBlock(otherX, y, otherZ)
	if err != nil {
		return
	}
	if int(otherState) < len(block.StateList) && block.StateList[otherState] != nil {
		if _, isBed := block.StateList[otherState].(block.RedBed); isBed {
			h.World.SetBlock(otherX, y, otherZ, 0)
			h.broadcastBlockUpdate(otherX, y, otherZ, 0)
		}
	}
}

// bedHeadOffset returns the (dx, dz) offset from foot to head for a given facing.
func bedHeadOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, -1
	case block.South:
		return 0, 1
	case block.West:
		return -1, 0
	case block.East:
		return 1, 0
	}
	return 0, 1
}

// wouldCollideWithPlayer checks if placing a block at (bx, by, bz) would overlap any player's hitbox.
func (h *BlockHandler) wouldCollideWithPlayer(bx, by, bz int) bool {
	var collides bool
	h.Manager.ForEach(func(p *game.Player) {
		px, py, pz := p.Position()
		// Player hitbox: 0.6 wide, 1.8 tall, centered on X/Z
		if float64(bx+1) > px-0.3 && float64(bx) < px+0.3 &&
			float64(by+1) > py && float64(by) < py+1.8 &&
			float64(bz+1) > pz-0.3 && float64(bz) < pz+0.3 {
			collides = true
		}
	})
	return collides
}

// handleBucket handles placing/picking up water or lava with buckets.
func (h *BlockHandler) handleBucket(player *game.Player, placeX, placeY, placeZ, clickX, clickY, clickZ int, action, heldName string, sequence int32) {
	slot := int(player.HeldSlot) + 36

	switch action {
	case "water":
		// Place water source block
		waterState, ok := block.ToStateID[block.Water{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, waterState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(waterState))
		// Replace held item with empty bucket
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		// Schedule fluid flow
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "water")
		}

	case "lava":
		// Place lava source block
		lavaState, ok := block.ToStateID[block.Lava{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, lavaState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(lavaState))
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "lava")
		}

	case "pickup":
		// Pick up source block at clicked position
		stateID, err := h.World.GetBlock(clickX, clickY, clickZ)
		if err != nil {
			h.sendAck(player, sequence)
			return
		}
		blockName := BlockNameFromState(int(stateID))
		var bucketName string
		switch blockName {
		case "water":
			// Only pick up source blocks (level=0)
			if b, ok := block.StateList[stateID].(block.Water); ok && int(b.Level) == 0 {
				bucketName = "water_bucket"
			}
		case "lava":
			if b, ok := block.StateList[stateID].(block.Lava); ok && int(b.Level) == 0 {
				bucketName = "lava_bucket"
			}
		}
		if bucketName == "" {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(clickX, clickY, clickZ, 0) // remove fluid
		h.broadcastBlockUpdate(clickX, clickY, clickZ, 0)
		if player.GameMode == 0 {
			bucketID := itemIDByName(bucketName)
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourceRemoved(clickX, clickY, clickZ)
		}
	}

	h.sendAck(player, sequence)
}

// plantSeed places a crop block on top of farmland and consumes the seed item.
func (h *BlockHandler) plantSeed(player *game.Player, x, y, z int, seedName string, sequence int32) bool {
	// Check that target position is air
	targetState, err := h.World.GetBlock(x, y, z)
	if err != nil || targetState != 0 {
		h.sendAck(player, sequence)
		return false
	}

	var cropBlock block.Block
	switch seedName {
	case "wheat_seeds":
		cropBlock = block.Wheat{Age: block.Integer(0)}
	case "carrot":
		cropBlock = block.Carrots{Age: block.Integer(0)}
	case "potato":
		cropBlock = block.Potatoes{Age: block.Integer(0)}
	case "beetroot_seeds":
		cropBlock = block.Beetroots{Age: block.Integer(0)}
	default:
		return false
	}

	cropStateID, ok := block.ToStateID[cropBlock]
	if !ok {
		return false
	}

	h.World.SetBlock(x, y, z, cropStateID)
	h.broadcastBlockUpdate(x, y, z, int32(cropStateID))
	h.sendAck(player, sequence)

	// Consume seed in survival
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Register crop for growth
	if h.CropMgr != nil {
		h.CropMgr.RegisterCrop(x, y, z)
	}

	return true
}

// isCropBlock returns true if the block name is a crop.
func isCropBlock(name string) bool {
	switch name {
	case "wheat", "carrots", "potatoes", "beetroots":
		return true
	}
	return false
}

// isSeedItem returns true if the item name is a seed that can be planted.
func isSeedItem(name string) bool {
	switch name {
	case "wheat_seeds", "carrot", "potato", "beetroot_seeds":
		return true
	}
	return false
}

// isHoeItem returns true if the item name is a hoe.
func isHoeItem(name string) bool {
	info := GetToolInfo(name)
	return info != nil && info.Type == ToolHoe
}

// dropCropItems spawns crop-specific drops based on the crop's age.
func (h *BlockHandler) dropCropItems(stateID int, x, y, z int) {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var cropName string
	var age int
	var maxAge int

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		cropName = "wheat"
		age = int(b.Age)
		maxAge = 7
	case block.Carrots:
		cropName = "carrots"
		age = int(b.Age)
		maxAge = 7
	case block.Potatoes:
		cropName = "potatoes"
		age = int(b.Age)
		maxAge = 7
	case block.Beetroots:
		cropName = "beetroots"
		age = int(b.Age)
		maxAge = 3
	default:
		return
	}

	if h.ItemEntities == nil {
		return
	}

	fx, fy, fz := float64(x)+0.5, float64(y)+0.5, float64(z)+0.5

	if age >= maxAge {
		// Mature crop drops
		switch cropName {
		case "wheat":
			wheatID := itemIDByName("wheat")
			seedID := itemIDByName("wheat_seeds")
			if wheatID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, wheatID, 1, 10)
			}
			if seedID > 0 {
				seedCount := int32(1 + rand.Intn(3)) // 1-3 seeds
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, seedID, seedCount, 10)
			}
		case "carrots":
			carrotID := itemIDByName("carrot")
			if carrotID > 0 {
				count := int32(1 + rand.Intn(4)) // 1-4 carrots
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, carrotID, count, 10)
			}
		case "potatoes":
			potatoID := itemIDByName("potato")
			if potatoID > 0 {
				count := int32(1 + rand.Intn(4)) // 1-4 potatoes
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, potatoID, count, 10)
			}
		case "beetroots":
			beetrootID := itemIDByName("beetroot")
			seedID := itemIDByName("beetroot_seeds")
			if beetrootID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, beetrootID, 1, 10)
			}
			if seedID > 0 {
				seedCount := int32(1 + rand.Intn(3)) // 1-3 seeds
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, seedID, seedCount, 10)
			}
		}
	} else {
		// Immature crop: drop seeds only
		switch cropName {
		case "wheat":
			seedID := itemIDByName("wheat_seeds")
			if seedID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, seedID, 1, 10)
			}
		case "beetroots":
			seedID := itemIDByName("beetroot_seeds")
			if seedID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, seedID, 1, 10)
			}
		// carrots and potatoes: immature drops the seed item itself
		case "carrots":
			carrotID := itemIDByName("carrot")
			if carrotID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, carrotID, 1, 10)
			}
		case "potatoes":
			potatoID := itemIDByName("potato")
			if potatoID > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, potatoID, 1, 10)
			}
		}
	}
}

func (h *BlockHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
