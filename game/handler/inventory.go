package handler

import (
	"bytes"
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// doubleClickScanOrder is the slot scan order for mode 6 (double-click collect):
// main inventory (9-35), hotbar (36-44), crafting/armor (1-8), offhand (45).
var doubleClickScanOrder = func() []int {
	order := make([]int, 0, 45)
	for i := 9; i <= 35; i++ {
		order = append(order, i)
	}
	for i := 36; i <= 44; i++ {
		order = append(order, i)
	}
	for i := 1; i <= 8; i++ {
		order = append(order, i)
	}
	return append(order, 45)
}()

// InventoryHandler processes inventory interaction packets.
type InventoryHandler struct {
	Logger           *log.Logger
	Chests           *ChestManager
	Furnaces         *FurnaceManager
	BrewingMgr       *BrewingStandManager
	HopperMgr        *HopperManager
	DispenserMgr     *DispenserManager
	EnchantMgr       *EnchantManager
	AnvilMgr         *AnvilManager
	VillagerMgr      *VillagerManager
	BarrelMgr        *BarrelManager
	GrindstoneMgr    *GrindstoneManager
	StonecutterMgr   *StonecutterManager
	SmokerMgr        *SmokerManager
	BlastFurnaceMgr  *BlastFurnaceManager
	ShulkerBoxMgr    *ShulkerBoxManager
	SmithingMgr      *SmithingTableManager
	BeaconMgr        *BeaconManager
	LoomMgr          *LoomManager
	CrafterMgr       *CrafterManager
	OnContainerClose func(containerType string, pos [3]int) // called when chest/furnace closed
}

// HandlePacket processes a single packet for the given player.
// Returns true if the packet was handled.
func (h *InventoryHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundContainerClick:
		h.handleContainerClick(player, p)
		return true
	case packetid.ServerboundContainerClose:
		h.handleContainerClose(player, p)
		return true
	case packetid.ServerboundContainerButtonClick:
		h.handleContainerButtonClick(player, p)
		return true
	case packetid.ServerboundRenameItem:
		HandleRenameItem(player, p)
		return true
	case packetid.ServerboundSelectTrade:
		if h.VillagerMgr != nil {
			return h.VillagerMgr.HandleSelectTrade(player, p)
		}
		return false
	case packetid.ServerboundSetBeacon:
		if h.BeaconMgr != nil {
			h.BeaconMgr.HandleBeaconUpdate(player, p)
			return true
		}
		return false
	}
	return false
}

// SendFullInventory sends ClientboundContainerSetContent to sync the full inventory.
func SendFullInventory(player *game.Player) {
	stateID := player.NextStateID()
	slots := player.InventorySlots()
	cursor := player.CursorItem.ToSlot()

	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(0), // window ID 0 = player inventory
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// handleContainerClick processes ServerboundContainerClick.
// We parse the packet manually since it has variable-length changed-slots.
func (h *InventoryHandler) handleContainerClick(player *game.Player, p pk.Packet) {
	r := bytes.NewReader(p.Data)

	var windowID pk.UnsignedByte
	var stateID pk.VarInt
	var slotNum pk.Short
	var button pk.Byte
	var mode pk.VarInt
	if _, err := windowID.ReadFrom(r); err != nil {
		return
	}
	if _, err := stateID.ReadFrom(r); err != nil {
		return
	}
	if _, err := slotNum.ReadFrom(r); err != nil {
		return
	}
	if _, err := button.ReadFrom(r); err != nil {
		return
	}
	if _, err := mode.ReadFrom(r); err != nil {
		return
	}

	// Read and skip changed slots array
	var changedCount pk.VarInt
	if _, err := changedCount.ReadFrom(r); err != nil {
		return
	}
	for i := 0; i < int(changedCount); i++ {
		var s pk.Short
		var slot game.Slot261
		if _, err := s.ReadFrom(r); err != nil {
			return
		}
		if _, err := slot.ReadFrom(r); err != nil {
			return
		}
	}

	// Read carried item (cursor after click)
	var carriedItem game.Slot261
	if _, err := carriedItem.ReadFrom(r); err != nil {
		return
	}

	slot := int(slotNum)

	if windowID == 1 && player.OpenWindowID == 1 {
		// Crafting table window
		switch mode {
		case 0:
			h.handleCraftingTableClick(player, slot, int(button))
		case 1:
			h.handleCraftingTableShiftClick(player, slot)
		case 2:
			h.handleCraftingTableNumberKey(player, slot, int(button))
		case 4:
			h.handleCraftingTableDrop(player, slot, int(button))
		}
		updateCraftingResult3x3(player)
		SendCraftingWindowContent(player)
		return
	}

	if int(windowID) == EnderChestWindowID && player.OpenWindowID == EnderChestWindowID {
		switch mode {
		case 0:
			h.handleEnderChestClick(player, slot, int(button))
		case 1:
			h.handleEnderChestShiftClick(player, slot)
		case 2:
			h.handleEnderChestNumberKey(player, slot, int(button))
		case 4:
			h.handleEnderChestDrop(player, slot, int(button))
		}
		SendEnderChestContent(player)
		return
	}

	if windowID == 2 && player.OpenWindowID == 2 && h.Chests != nil {
		cs := h.Chests.Get(player.OpenChestPos[0], player.OpenChestPos[1], player.OpenChestPos[2])
		if cs != nil {
			switch mode {
			case 0:
				h.handleChestClick(player, cs, slot, int(button))
			case 1:
				h.handleChestShiftClick(player, cs, slot)
			case 2:
				h.handleChestNumberKey(player, cs, slot, int(button))
			case 4:
				h.handleChestDrop(player, cs, slot, int(button))
			}
			if cs.Partner != nil {
				SendDoubleChestWindowContent(player, cs)
			} else {
				SendChestWindowContent(player, cs)
			}
		}
		return
	}

	if windowID == 3 && player.OpenWindowID == 3 && h.Furnaces != nil {
		fs := h.Furnaces.GetOrCreate(player.OpenFurnacePos[0], player.OpenFurnacePos[1], player.OpenFurnacePos[2])
		switch mode {
		case 0:
			h.handleFurnaceClick(player, fs, slot, int(button))
		case 1:
			h.handleFurnaceShiftClick(player, fs, slot)
		case 2:
			h.handleFurnaceNumberKey(player, fs, slot, int(button))
		case 4:
			h.handleFurnaceDrop(player, fs, slot, int(button))
		}
		SendFurnaceWindowContent(player, fs)
		return
	}

	if int(windowID) == BrewingWindowID && player.OpenWindowID == BrewingWindowID && h.BrewingMgr != nil {
		bs := h.BrewingMgr.Get(player.OpenBrewingStandPos[0], player.OpenBrewingStandPos[1], player.OpenBrewingStandPos[2])
		if bs != nil {
			switch mode {
			case 0:
				h.handleBrewingClick(player, bs, slot, int(button))
			case 1:
				h.handleBrewingShiftClick(player, bs, slot)
			case 2:
				h.handleBrewingNumberKey(player, bs, slot, int(button))
			case 4:
				h.handleBrewingDrop(player, bs, slot, int(button))
			}
			SendBrewingWindowContent(player, bs)
		}
		return
	}

	if player.AnvilSession != nil && int(windowID) == player.AnvilSession.WindowID && player.OpenWindowID == player.AnvilSession.WindowID {
		switch mode {
		case 0:
			h.handleAnvilClick(player, slot, int(button))
		case 1:
			h.handleAnvilShiftClick(player, slot)
		case 2:
			h.handleAnvilNumberKey(player, slot, int(button))
		case 4:
			h.handleAnvilDrop(player, slot, int(button))
		}
		ComputeAnvilResult(player)
		SendAnvilWindowContent(player)
		return
	}

	if int(windowID) == HopperWindowID && player.OpenWindowID == HopperWindowID && h.HopperMgr != nil {
		hs := h.HopperMgr.Get(player.OpenHopperPos[0], player.OpenHopperPos[1], player.OpenHopperPos[2])
		if hs != nil {
			switch mode {
			case 0:
				h.handleHopperClick(player, hs, slot, int(button))
			case 1:
				h.handleHopperShiftClick(player, hs, slot)
			case 2:
				h.handleHopperNumberKey(player, hs, slot, int(button))
			case 4:
				h.handleHopperDrop(player, hs, slot, int(button))
			}
			SendHopperWindowContent(player, hs)
		}
		return
	}

	if int(windowID) == DispenserWindowID && player.OpenWindowID == DispenserWindowID && h.DispenserMgr != nil {
		ds := h.DispenserMgr.Get(player.OpenDispenserPos[0], player.OpenDispenserPos[1], player.OpenDispenserPos[2])
		if ds != nil {
			switch mode {
			case 0:
				h.handleDispenserClick(player, ds, slot, int(button))
			case 1:
				h.handleDispenserShiftClick(player, ds, slot)
			case 2:
				h.handleDispenserNumberKey(player, ds, slot, int(button))
			case 4:
				h.handleDispenserDrop(player, ds, slot, int(button))
			}
			SendDispenserWindowContent(player, ds)
		}
		return
	}

	if int(windowID) == CrafterWindowID && player.OpenWindowID == CrafterWindowID && h.CrafterMgr != nil {
		cs := h.CrafterMgr.Get(player.OpenCrafterPos[0], player.OpenCrafterPos[1], player.OpenCrafterPos[2])
		if cs != nil {
			switch mode {
			case 0:
				h.CrafterMgr.HandleCrafterClick(player, cs, slot, int(button))
			case 1:
				h.CrafterMgr.HandleCrafterShiftClick(player, cs, slot)
			case 2:
				h.CrafterMgr.HandleCrafterNumberKey(player, cs, slot, int(button))
			case 4:
				h.CrafterMgr.HandleCrafterDrop(player, cs, slot, int(button))
			}
			SendCrafterWindowContent(player, cs)
		}
		return
	}

	if int(windowID) == MerchantWindowID && player.OpenWindowID == MerchantWindowID && h.VillagerMgr != nil {
		h.handleMerchantClick(player, slot, int(button), int(mode))
		return
	}

	if int(windowID) == 12 && player.OpenWindowID == 12 && h.BarrelMgr != nil {
		bs := h.BarrelMgr.Get(player.OpenChestPos[0], player.OpenChestPos[1], player.OpenChestPos[2])
		if bs != nil {
			switch mode {
			case 0:
				h.handleBarrelClick(player, bs, slot, int(button))
			case 1:
				h.handleBarrelShiftClick(player, bs, slot)
			}
			SendBarrelWindowContent(player, bs)
		}
		return
	}

	if int(windowID) == SmokerWindowID && player.OpenWindowID == SmokerWindowID && h.SmokerMgr != nil {
		ss := h.SmokerMgr.GetOrCreate(player.OpenFurnacePos[0], player.OpenFurnacePos[1], player.OpenFurnacePos[2])
		switch mode {
		case 0:
			h.handleSmokerClick(player, ss, slot, int(button))
		case 1:
			h.handleSmokerShiftClick(player, ss, slot)
		}
		SendSmokerWindowContent(player, ss)
		return
	}

	if int(windowID) == BlastFurnaceWindowID && player.OpenWindowID == BlastFurnaceWindowID && h.BlastFurnaceMgr != nil {
		bfs := h.BlastFurnaceMgr.GetOrCreate(player.OpenFurnacePos[0], player.OpenFurnacePos[1], player.OpenFurnacePos[2])
		switch mode {
		case 0:
			h.handleBlastFurnaceClick(player, bfs, slot, int(button))
		case 1:
			h.handleBlastFurnaceShiftClick(player, bfs, slot)
		}
		SendBlastFurnaceWindowContent(player, bfs)
		return
	}

	if int(windowID) == ShulkerBoxWindowID && player.OpenWindowID == ShulkerBoxWindowID && h.ShulkerBoxMgr != nil {
		sbs := h.ShulkerBoxMgr.Get(player.OpenChestPos[0], player.OpenChestPos[1], player.OpenChestPos[2])
		if sbs != nil {
			switch mode {
			case 0:
				h.handleShulkerBoxClick(player, sbs, slot, int(button))
			case 1:
				h.handleShulkerBoxShiftClick(player, sbs, slot)
			}
			SendShulkerBoxWindowContent(player, sbs)
		}
		return
	}

	if windowID != 0 {
		// Unknown window — resync
		SendFullInventory(player)
		return
	}

	// Snapshot armor slots before click for change detection
	var armorBefore [4]int32
	for i, s := range []int{5, 6, 7, 8} {
		armorBefore[i] = player.Inventory[s].ID
	}

	switch mode {
	case 0: // Normal click
		h.handleNormalClick(player, slot, int(button))
	case 1: // Shift-click
		h.handleShiftClick(player, slot)
	case 2: // Number key swap
		h.handleNumberKey(player, slot, int(button))
	case 4: // Drop
		h.handleDrop(player, slot, int(button))
	case 5: // Drag
		h.handleDrag(player, slot, int(button))
	case 6: // Double-click (collect matching items to cursor)
		h.handleDoubleClick(player, slot)
	default:
		// Mode 3 (clone) — just resync
	}

	updateCraftingResult(player)
	SendFullInventory(player)

	// If any armor slot changed, broadcast updated attributes
	armorChanged := false
	for i, s := range []int{5, 6, 7, 8} {
		if player.Inventory[s].ID != armorBefore[i] {
			armorChanged = true
			break
		}
	}
	if armorChanged {
		if mgr := h.playerManager(); mgr != nil {
			BroadcastAttributes(mgr, player)
		}
	}
}

// handleContainerClose handles ServerboundContainerClose.
func (h *InventoryHandler) handleContainerClose(player *game.Player, p pk.Packet) {
	// Return cursor item to inventory
	if player.CursorItem.ID > 0 && player.CursorItem.Count > 0 {
		player.Inventory.AddItem(player.CursorItem.ID, player.CursorItem.Count)
		player.CursorItem = game.ItemStack{}
	}

	switch player.OpenWindowID {
	case 1:
		// Closing crafting table — return 3x3 grid items to inventory
		for i := range player.CraftingGrid {
			itm := &player.CraftingGrid[i]
			if itm.ID > 0 && itm.Count > 0 {
				player.Inventory.AddItem(itm.ID, itm.Count)
				*itm = game.ItemStack{}
			}
		}
	case 2:
		// Closing chest
		h.playBlockSound(SoundChestClose, player.OpenChestPos)
		if h.OnContainerClose != nil {
			h.OnContainerClose("chest", player.OpenChestPos)
		}
	case 3:
		// Closing furnace — notify persistence layer
		if h.OnContainerClose != nil {
			h.OnContainerClose("furnace", player.OpenFurnacePos)
		}
	case BrewingWindowID:
		// Closing brewing stand — notify persistence layer
		if h.OnContainerClose != nil {
			h.OnContainerClose("brewing_stand", player.OpenBrewingStandPos)
		}
	case HopperWindowID:
		// Closing hopper — notify persistence layer
		if h.OnContainerClose != nil {
			h.OnContainerClose("hopper", player.OpenHopperPos)
		}
	case DispenserWindowID:
		// Closing dispenser/dropper — notify persistence layer
		if h.OnContainerClose != nil {
			h.OnContainerClose("dispenser", player.OpenDispenserPos)
		}
	case CrafterWindowID:
		// Closing crafter — notify persistence layer
		if h.OnContainerClose != nil {
			h.OnContainerClose("crafter", player.OpenCrafterPos)
		}
	case 12:
		// Closing barrel
		h.playBlockSound(SoundBarrelClose, player.OpenChestPos)
		if h.OnContainerClose != nil {
			h.OnContainerClose("barrel", player.OpenChestPos)
		}
	case 13:
		// Closing grindstone — no state to return
	case 14:
		// Closing stonecutter — no state to return
	case SmokerWindowID:
		// Closing smoker
		if h.OnContainerClose != nil {
			h.OnContainerClose("smoker", player.OpenFurnacePos)
		}
	case BlastFurnaceWindowID:
		// Closing blast furnace
		if h.OnContainerClose != nil {
			h.OnContainerClose("blast_furnace", player.OpenFurnacePos)
		}
	case ShulkerBoxWindowID:
		// Closing shulker box
		h.playBlockSound(SoundShulkerBoxClose, player.OpenChestPos)
		if h.OnContainerClose != nil {
			h.OnContainerClose("shulker_box", player.OpenChestPos)
		}
	case SmithingWindowID:
		// Closing smithing table — no state to return
	case EnderChestWindowID:
		// Closing ender chest — no shared state to save (per-player storage)
	case 10:
		// Closing enchanting table — clear session
		player.EnchantSession = nil
	case MerchantWindowID:
		// Closing merchant window
		if h.VillagerMgr != nil {
			h.VillagerMgr.OnMerchantClose(player)
		}
	case 11:
		// Closing anvil — return input/material items to inventory
		if player.AnvilSession != nil {
			if player.AnvilSession.Input.ID > 0 && player.AnvilSession.Input.Count > 0 {
				player.Inventory.AddItem(player.AnvilSession.Input.ID, player.AnvilSession.Input.Count)
			}
			if player.AnvilSession.Material.ID > 0 && player.AnvilSession.Material.Count > 0 {
				player.Inventory.AddItem(player.AnvilSession.Material.ID, player.AnvilSession.Material.Count)
			}
			player.AnvilSession = nil
		}
	default:
		// Closing player inventory — return 2x2 crafting grid items (slots 1-4)
		for i := 1; i <= 4; i++ {
			itm := &player.Inventory[i]
			if itm.ID > 0 && itm.Count > 0 {
				player.Inventory.AddItem(itm.ID, itm.Count)
				*itm = game.ItemStack{}
			}
		}
		player.Inventory[0] = game.ItemStack{} // clear crafting result
	}

	player.OpenWindowID = 0
	SendFullInventory(player)
}

// playerManager returns the PlayerManager via the chest manager, or nil.
func (h *InventoryHandler) playerManager() *game.PlayerManager {
	if h.Chests != nil {
		return h.Chests.Manager
	}
	return nil
}

// playBlockSound broadcasts a sound at the center of a block position.
func (h *InventoryHandler) playBlockSound(soundID int32, pos [3]int) {
	if h.Chests == nil || h.Chests.Manager == nil {
		return
	}
	BroadcastSound(h.Chests.Manager, soundID, SoundCategoryBlock,
		float64(pos[0])+0.5, float64(pos[1])+0.5, float64(pos[2])+0.5, 1.0, 1.0)
}

// playPlayerSound broadcasts a sound at the player's position.
func (h *InventoryHandler) playPlayerSound(player *game.Player, soundID int32) {
	if h.Chests == nil || h.Chests.Manager == nil {
		return
	}
	px, py, pz := player.Position()
	BroadcastSound(h.Chests.Manager, soundID, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
}

// updateCraftingResult checks the 2x2 crafting grid (slots 1-4) and sets the
// result in slot 0.
func updateCraftingResult(player *game.Player) {
	grid := make([]int32, 4)
	for i := 0; i < 4; i++ {
		grid[i] = player.Inventory[i+1].ID
	}
	resultID, count := MatchRecipe(grid, 2)
	if resultID > 0 {
		player.Inventory[0] = NewItemStack(resultID, count)
	} else {
		player.Inventory[0] = game.ItemStack{}
	}
}

// handleCraftingResultClick handles clicking on the crafting result slot (slot 0).
// Gives the result to the cursor and consumes one of each ingredient.
func (h *InventoryHandler) handleCraftingResultClick(player *game.Player) {
	result := player.Inventory[0]
	if result.ID == 0 || result.Count <= 0 {
		return
	}

	// Can only pick up result if cursor is empty or same item with room
	if player.CursorItem.ID != 0 && player.CursorItem.ID != result.ID {
		return
	}
	if player.CursorItem.ID == result.ID && player.CursorItem.Count+result.Count > 64 {
		return
	}

	// Give result to cursor
	if player.CursorItem.ID == 0 {
		player.CursorItem = result
	} else {
		player.CursorItem.Count += result.Count
	}

	// Consume one of each non-empty ingredient (slots 1-4)
	consumeCraftingIngredients(player)

	// Play crafting click sound
	h.playPlayerSound(player, SoundUIButtonClick)

	// Recompute result
	updateCraftingResult(player)
}

// handleCraftingResultShiftClick handles shift-clicking the crafting result slot.
// Moves result to main/hotbar inventory and consumes ingredients.
func (h *InventoryHandler) handleCraftingResultShiftClick(player *game.Player) {
	result := player.Inventory[0]
	if result.ID == 0 || result.Count <= 0 {
		return
	}

	// Move result to main/hotbar
	addedSlot := player.Inventory.AddItem(result.ID, result.Count)
	if addedSlot < 0 {
		return // inventory full
	}

	// Consume ingredients
	consumeCraftingIngredients(player)

	// Play crafting click sound
	h.playPlayerSound(player, SoundUIButtonClick)

	// Recompute result
	updateCraftingResult(player)
}

// consumeCraftingIngredients decrements each non-empty crafting grid slot (1-4) by 1.
func consumeCraftingIngredients(player *game.Player) {
	for i := 1; i <= 4; i++ {
		item := &player.Inventory[i]
		if item.ID > 0 && item.Count > 0 {
			item.Count--
			if item.Count <= 0 {
				*item = game.ItemStack{}
			}
		}
	}
}
