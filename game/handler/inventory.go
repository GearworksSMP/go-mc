package handler

import (
	"bytes"
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
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
}

// handleNormalClick handles mode 0 (left/right click).
func (h *InventoryHandler) handleNormalClick(player *game.Player, slot, button int) {
	if slot == -999 {
		// Click outside window — drop cursor item
		if button == 0 {
			// Left click: drop entire stack
			player.CursorItem = game.ItemStack{}
		} else {
			// Right click: drop one
			if player.CursorItem.Count > 1 {
				player.CursorItem.Count--
			} else {
				player.CursorItem = game.ItemStack{}
			}
		}
		return
	}

	if slot < 0 || slot >= 46 {
		return
	}

	// Crafting result slot — special handling
	if slot == 0 {
		h.handleCraftingResultClick(player)
		return
	}

	invItem := &player.Inventory[slot]

	// Curse of Binding: prevent removing cursed armor in survival mode
	if slot >= 5 && slot <= 8 && invItem.ID > 0 && player.GameMode != 1 {
		if enchant.HasEnchant(invItem.Enchantments, enchant.CurseOfBinding) {
			return
		}
	}

	// Armor slot restrictions: only allow correct armor type in slots 5-8
	if slot >= 5 && slot <= 8 && player.CursorItem.ID > 0 {
		cursorName := ItemNameByID(player.CursorItem.ID)
		armorInfo := GetArmorInfo(cursorName)
		if armorInfo == nil || ArmorSlotFor(armorInfo.Type) != slot {
			// Not the correct armor piece for this slot — only allow picking up
			if button == 0 && invItem.ID > 0 {
				player.CursorItem, *invItem = *invItem, player.CursorItem
			}
			return
		}
	}

	if button == 0 { // Left click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			// Pick up stack
			player.CursorItem = *invItem
			*invItem = game.ItemStack{}
		} else if invItem.ID == 0 {
			// Place stack
			*invItem = player.CursorItem
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.ID == invItem.ID {
			// Merge stacks
			space := int32(64) - invItem.Count
			if space >= player.CursorItem.Count {
				invItem.Count += player.CursorItem.Count
				player.CursorItem = game.ItemStack{}
			} else {
				invItem.Count = 64
				player.CursorItem.Count -= space
			}
		} else {
			// Swap cursor and slot
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	} else { // Right click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			// Pick up half
			half := (invItem.Count + 1) / 2
			player.CursorItem = game.ItemStack{ID: invItem.ID, Count: half}
			invItem.Count -= half
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
		} else if invItem.ID == 0 || invItem.ID == player.CursorItem.ID {
			// Place one
			if invItem.ID == 0 {
				*invItem = game.ItemStack{ID: player.CursorItem.ID, Count: 0}
			}
			if invItem.Count < 64 {
				invItem.Count++
				player.CursorItem.Count--
				if player.CursorItem.Count <= 0 {
					player.CursorItem = game.ItemStack{}
				}
			}
		} else {
			// Swap
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	}
}

// handleShiftClick handles mode 1 (shift-click).
// Moves items between hotbar (36-44) and main inventory (9-35).
func (h *InventoryHandler) handleShiftClick(player *game.Player, slot int) {
	if slot < 0 || slot >= 46 {
		return
	}

	// Shift-click crafting result: move result to inventory and consume ingredients
	if slot == 0 {
		h.handleCraftingResultShiftClick(player)
		return
	}

	src := &player.Inventory[slot]
	if src.ID == 0 || src.Count <= 0 {
		return
	}

	// Auto-equip armor on shift-click
	srcName := ItemNameByID(src.ID)
	if armorInfo := GetArmorInfo(srcName); armorInfo != nil && slot >= 9 {
		targetSlot := ArmorSlotFor(armorInfo.Type)
		if targetSlot >= 5 && targetSlot <= 8 {
			dst := &player.Inventory[targetSlot]
			if dst.ID == 0 || dst.Count <= 0 {
				*dst = *src
				*src = game.ItemStack{}
			} else {
				// Swap with existing armor
				*src, *dst = *dst, *src
			}
			return
		}
	}

	// Shield goes to offhand slot 45
	if srcName == "shield" && slot >= 9 && slot != 45 {
		dst := &player.Inventory[45]
		if dst.ID == 0 || dst.Count <= 0 {
			*dst = *src
			*src = game.ItemStack{}
		} else {
			*src, *dst = *dst, *src
		}
		return
	}

	if slot >= 36 && slot <= 44 {
		// Hotbar → main inventory (9-35)
		h.moveItem(player, slot, 9, 35)
	} else if slot >= 9 && slot <= 35 {
		// Main inventory → hotbar (36-44)
		h.moveItem(player, slot, 36, 44)
	} else if slot >= 1 && slot <= 8 {
		// Crafting area/armor → main/hotbar (9-44)
		h.moveItem(player, slot, 9, 44)
	}
}

// moveItem moves the item from srcSlot into the range [dstStart, dstEnd].
func (h *InventoryHandler) moveItem(player *game.Player, srcSlot, dstStart, dstEnd int) {
	src := &player.Inventory[srcSlot]

	// Try stacking first
	for i := dstStart; i <= dstEnd; i++ {
		dst := &player.Inventory[i]
		if dst.ID == src.ID && dst.Count > 0 && dst.Count < 64 {
			space := int32(64) - dst.Count
			if space >= src.Count {
				dst.Count += src.Count
				*src = game.ItemStack{}
				return
			}
			dst.Count = 64
			src.Count -= space
		}
	}

	// Then find empty slot
	for i := dstStart; i <= dstEnd; i++ {
		dst := &player.Inventory[i]
		if dst.ID == 0 || dst.Count <= 0 {
			*dst = *src
			*src = game.ItemStack{}
			return
		}
	}
}

// handleNumberKey handles mode 2 (number key swap).
// Swaps clicked slot with hotbar slot (button = 0-8 → inventory 36-44).
func (h *InventoryHandler) handleNumberKey(player *game.Player, slot, button int) {
	if slot < 0 || slot >= 46 {
		return
	}
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}
	player.Inventory[slot], player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], player.Inventory[slot]
}

// handleDrop handles mode 4 (drop key in inventory screen).
func (h *InventoryHandler) handleDrop(player *game.Player, slot, button int) {
	if slot == -999 {
		// Drop from cursor
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	if slot < 0 || slot >= 46 {
		return
	}
	invItem := &player.Inventory[slot]
	if invItem.ID == 0 || invItem.Count <= 0 {
		return
	}

	if button == 0 {
		// Drop one
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
	} else if button == 1 {
		// Drop entire stack
		*invItem = game.ItemStack{}
	}
}

// handleDrag handles mode 5 (drag to distribute items across slots).
// The drag protocol sends multiple ContainerClick packets:
//   - Start: button 0/4/8, slot -999
//   - Add slot: button 1/5/9, slot = target
//   - End: button 2/6/10, slot -999
func (h *InventoryHandler) handleDrag(player *game.Player, slot, button int) {
	switch button {
	case 0, 4, 8:
		player.DragSlots = nil
	case 1, 5, 9:
		// Add slot to drag
		if slot >= 0 && slot < 46 {
			player.DragSlots = append(player.DragSlots, int16(slot))
		}
	case 2:
		// End left drag — divide evenly
		h.endLeftDrag(player)
		player.DragSlots = nil
	case 6:
		// End right drag — place one each
		h.endRightDrag(player)
		player.DragSlots = nil
	case 10:
		// End middle drag — creative clone
		h.endMiddleDrag(player)
		player.DragSlots = nil
	}
}

// endLeftDrag distributes the cursor item evenly across all drag slots.
func (h *InventoryHandler) endLeftDrag(player *game.Player) {
	if player.CursorItem.ID == 0 || player.CursorItem.Count <= 0 {
		return
	}
	slots := player.DragSlots
	if len(slots) == 0 {
		return
	}

	// Filter to valid target slots (empty or same item type with room)
	var targets []int16
	for _, s := range slots {
		if s < 0 || int(s) >= 46 {
			continue
		}
		inv := &player.Inventory[s]
		if inv.ID == 0 || (inv.ID == player.CursorItem.ID && inv.Count < 64) {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 {
		return
	}

	perSlot := player.CursorItem.Count / int32(len(targets))
	if perSlot <= 0 {
		return
	}

	remaining := player.CursorItem.Count
	for _, s := range targets {
		inv := &player.Inventory[s]
		give := perSlot
		if inv.ID == player.CursorItem.ID {
			// Stack — cap at 64
			space := int32(64) - inv.Count
			if give > space {
				give = space
			}
			inv.Count += give
		} else {
			// Empty slot
			if give > 64 {
				give = 64
			}
			*inv = game.ItemStack{ID: player.CursorItem.ID, Count: give}
		}
		remaining -= give
	}

	if remaining > 0 {
		player.CursorItem.Count = remaining
	} else {
		player.CursorItem = game.ItemStack{}
	}
}

// endRightDrag places exactly 1 item from cursor into each drag slot.
func (h *InventoryHandler) endRightDrag(player *game.Player) {
	if player.CursorItem.ID == 0 || player.CursorItem.Count <= 0 {
		return
	}
	slots := player.DragSlots
	if len(slots) == 0 {
		return
	}

	for _, s := range slots {
		if player.CursorItem.Count <= 0 {
			break
		}
		if s < 0 || int(s) >= 46 {
			continue
		}
		inv := &player.Inventory[s]
		if inv.ID == 0 {
			*inv = game.ItemStack{ID: player.CursorItem.ID, Count: 1}
			player.CursorItem.Count--
		} else if inv.ID == player.CursorItem.ID && inv.Count < 64 {
			inv.Count++
			player.CursorItem.Count--
		}
	}

	if player.CursorItem.Count <= 0 {
		player.CursorItem = game.ItemStack{}
	}
}

// endMiddleDrag clones the cursor stack into each drag slot (creative mode only).
func (h *InventoryHandler) endMiddleDrag(player *game.Player) {
	if player.GameMode != 1 {
		return // middle drag is creative-only
	}
	if player.CursorItem.ID == 0 {
		return
	}
	for _, s := range player.DragSlots {
		if s < 0 || int(s) >= 46 {
			continue
		}
		player.Inventory[s] = game.ItemStack{
			ID:    player.CursorItem.ID,
			Count: player.CursorItem.Count,
		}
	}
}

// handleDoubleClick handles mode 6 (double-click to collect matching items to cursor).
// Scans all accessible slots and transfers items matching the cursor's item ID
// until the cursor reaches max stack size (64).
func (h *InventoryHandler) handleDoubleClick(player *game.Player, slot int) {
	if player.CursorItem.ID == 0 || player.CursorItem.Count <= 0 {
		return
	}
	if player.CursorItem.Count >= 64 {
		return
	}

	targetID := player.CursorItem.ID

	for _, i := range doubleClickScanOrder {
		if player.CursorItem.Count >= 64 {
			break
		}
		inv := &player.Inventory[i]
		if inv.ID != targetID || inv.Count <= 0 {
			continue
		}
		// Transfer as much as possible
		space := int32(64) - player.CursorItem.Count
		take := inv.Count
		if take > space {
			take = space
		}
		player.CursorItem.Count += take
		inv.Count -= take
		if inv.Count <= 0 {
			*inv = game.ItemStack{}
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
		// Closing chest — play close sound + notify persistence layer
		if h.Chests != nil && h.Chests.Manager != nil {
			pos := player.OpenChestPos
			BroadcastSound(h.Chests.Manager, SoundChestClose, SoundCategoryBlock,
				float64(pos[0])+0.5, float64(pos[1])+0.5, float64(pos[2])+0.5, 1.0, 1.0)
		}
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
	case 12:
		// Closing barrel
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

// ---------------------------------------------------------------------------
// Crafting table (3x3) window handlers
// ---------------------------------------------------------------------------

// Crafting table window slot layout:
// 0     = crafting result
// 1-9   = CraftingGrid[0..8]
// 10-36 = Inventory[9..35] (main inventory)
// 37-45 = Inventory[36..44] (hotbar)

// craftingTableSlot returns a pointer to the storage backing a crafting table window slot.
func craftingTableSlot(player *game.Player, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 1 && windowSlot <= 9:
		return &player.CraftingGrid[windowSlot-1]
	case windowSlot >= 10 && windowSlot <= 36:
		return &player.Inventory[windowSlot-10+9] // main inv: 9..35
	case windowSlot >= 37 && windowSlot <= 45:
		return &player.Inventory[windowSlot-37+36] // hotbar: 36..44
	}
	return nil
}

func (h *InventoryHandler) handleCraftingTableClick(player *game.Player, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else {
			if player.CursorItem.Count > 1 {
				player.CursorItem.Count--
			} else {
				player.CursorItem = game.ItemStack{}
			}
		}
		return
	}

	if slot == 0 {
		h.handleCraftingTableResultClick(player)
		return
	}

	invItem := craftingTableSlot(player, slot)
	if invItem == nil {
		return
	}

	if button == 0 { // Left click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
			*invItem = game.ItemStack{}
		} else if invItem.ID == 0 {
			*invItem = player.CursorItem
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.ID == invItem.ID {
			space := int32(64) - invItem.Count
			if space >= player.CursorItem.Count {
				invItem.Count += player.CursorItem.Count
				player.CursorItem = game.ItemStack{}
			} else {
				invItem.Count = 64
				player.CursorItem.Count -= space
			}
		} else {
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	} else { // Right click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			half := (invItem.Count + 1) / 2
			player.CursorItem = game.ItemStack{ID: invItem.ID, Count: half,
				Durability: invItem.Durability, MaxDurability: invItem.MaxDurability}
			invItem.Count -= half
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
		} else if invItem.ID == 0 || invItem.ID == player.CursorItem.ID {
			if invItem.ID == 0 {
				*invItem = game.ItemStack{ID: player.CursorItem.ID, Count: 0}
			}
			if invItem.Count < 64 {
				invItem.Count++
				player.CursorItem.Count--
				if player.CursorItem.Count <= 0 {
					player.CursorItem = game.ItemStack{}
				}
			}
		} else {
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	}
}

func (h *InventoryHandler) handleCraftingTableShiftClick(player *game.Player, slot int) {
	if slot == 0 {
		h.handleCraftingTableResultShiftClick(player)
		return
	}

	src := craftingTableSlot(player, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	// Auto-equip armor on shift-click from grid or inventory areas
	srcName := ItemNameByID(src.ID)
	if armorInfo := GetArmorInfo(srcName); armorInfo != nil {
		targetSlot := ArmorSlotFor(armorInfo.Type)
		if targetSlot >= 5 && targetSlot <= 8 {
			dst := &player.Inventory[targetSlot]
			if dst.ID == 0 || dst.Count <= 0 {
				*dst = *src
				*src = game.ItemStack{}
				return
			}
		}
	}

	if slot >= 1 && slot <= 9 {
		// Grid → main/hotbar (inv slots 9-44)
		moveCraftingTableItem(player, src, 9, 44)
	} else if slot >= 10 && slot <= 36 {
		// Main inv → hotbar
		moveCraftingTableItem(player, src, 36, 44)
	} else if slot >= 37 && slot <= 45 {
		// Hotbar → main inv
		moveCraftingTableItem(player, src, 9, 35)
	}
}

// moveCraftingTableItem moves items from src into player.Inventory[dstStart..dstEnd].
func moveCraftingTableItem(player *game.Player, src *game.ItemStack, dstStart, dstEnd int) {
	// Try stacking first
	for i := dstStart; i <= dstEnd; i++ {
		dst := &player.Inventory[i]
		if dst.ID == src.ID && dst.Count > 0 && dst.Count < 64 {
			space := int32(64) - dst.Count
			if space >= src.Count {
				dst.Count += src.Count
				*src = game.ItemStack{}
				return
			}
			dst.Count = 64
			src.Count -= space
		}
	}
	// Then find empty slot
	for i := dstStart; i <= dstEnd; i++ {
		dst := &player.Inventory[i]
		if dst.ID == 0 || dst.Count <= 0 {
			*dst = *src
			*src = game.ItemStack{}
			return
		}
	}
}

func (h *InventoryHandler) handleCraftingTableNumberKey(player *game.Player, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}

	src := craftingTableSlot(player, slot)
	if src == nil {
		return
	}

	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleCraftingTableDrop(player *game.Player, slot, button int) {
	if slot == -999 {
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	src := craftingTableSlot(player, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

func (h *InventoryHandler) handleCraftingTableResultClick(player *game.Player) {
	result := craftingTableResult(player)
	if result.ID == 0 || result.Count <= 0 {
		return
	}

	if player.CursorItem.ID != 0 && player.CursorItem.ID != result.ID {
		return
	}
	if player.CursorItem.ID == result.ID && player.CursorItem.Count+result.Count > 64 {
		return
	}

	if player.CursorItem.ID == 0 {
		player.CursorItem = result
	} else {
		player.CursorItem.Count += result.Count
	}

	// Consume one of each non-empty ingredient
	for i := range player.CraftingGrid {
		itm := &player.CraftingGrid[i]
		if itm.ID > 0 && itm.Count > 0 {
			itm.Count--
			if itm.Count <= 0 {
				*itm = game.ItemStack{}
			}
		}
	}
}

func (h *InventoryHandler) handleCraftingTableResultShiftClick(player *game.Player) {
	result := craftingTableResult(player)
	if result.ID == 0 || result.Count <= 0 {
		return
	}

	addedSlot := player.Inventory.AddItem(result.ID, result.Count)
	if addedSlot < 0 {
		return
	}

	for i := range player.CraftingGrid {
		itm := &player.CraftingGrid[i]
		if itm.ID > 0 && itm.Count > 0 {
			itm.Count--
			if itm.Count <= 0 {
				*itm = game.ItemStack{}
			}
		}
	}
}

// craftingTableResult computes the crafting result for the 3x3 grid.
func craftingTableResult(player *game.Player) game.ItemStack {
	grid := make([]int32, 9)
	for i := range player.CraftingGrid {
		grid[i] = player.CraftingGrid[i].ID
	}
	resultID, count := MatchRecipe(grid, 3)
	if resultID > 0 {
		return NewItemStack(resultID, count)
	}
	return game.ItemStack{}
}

// updateCraftingResult3x3 updates the crafting table result after grid changes.
func updateCraftingResult3x3(player *game.Player) {
	// Result is computed on-the-fly in SendCraftingWindowContent
	// Nothing to store — result is derived from grid state
}

// SendCraftingWindowContent sends the full crafting table window content.
// Layout: 46 slots = 1 result + 9 grid + 27 main + 9 hotbar
func SendCraftingWindowContent(player *game.Player) {
	stateID := player.NextStateID()

	// Compute crafting result
	result := craftingTableResult(player)

	// Build 46 slots
	slots := make(game.Slot261Array, 46)
	slots[0] = result.ToSlot()
	for i := 0; i < 9; i++ {
		slots[1+i] = player.CraftingGrid[i].ToSlot()
	}
	for i := 9; i <= 35; i++ {
		slots[1+9+(i-9)] = player.Inventory[i].ToSlot()
	}
	for i := 36; i <= 44; i++ {
		slots[1+9+27+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(1), // window ID 1
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// ---------------------------------------------------------------------------
// Chest (window ID 2) handlers
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleChestClick(player *game.Player, cs *ChestState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := ChestSlot(player, cs, slot)
	if invItem == nil {
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleChestShiftClick(player *game.Player, cs *ChestState, slot int) {
	src := ChestSlot(player, cs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	if cs.Partner != nil {
		// Double chest: 0-53 = chest, 54-80 = main, 81-89 = hotbar
		if slot >= 0 && slot <= 53 {
			moveToPlayerInv(player, src)
		} else if slot >= 54 && slot <= 80 {
			if !moveToDoubleChest(cs, src) {
				moveToRange(player, src, 36, 44)
			}
		} else if slot >= 81 && slot <= 89 {
			if !moveToDoubleChest(cs, src) {
				moveToRange(player, src, 9, 35)
			}
		}
	} else {
		// Single chest: 0-26 = chest, 27-53 = main, 54-62 = hotbar
		if slot >= 0 && slot <= 26 {
			moveToPlayerInv(player, src)
		} else if slot >= 27 && slot <= 53 {
			if !moveToChest(cs, src) {
				moveToRange(player, src, 36, 44)
			}
		} else if slot >= 54 && slot <= 62 {
			if !moveToChest(cs, src) {
				moveToRange(player, src, 9, 35)
			}
		}
	}
}

func (h *InventoryHandler) handleChestNumberKey(player *game.Player, cs *ChestState, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}
	src := ChestSlot(player, cs, slot)
	if src == nil {
		return
	}
	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleChestDrop(player *game.Player, cs *ChestState, slot, button int) {
	if slot == -999 {
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}
	src := ChestSlot(player, cs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// moveToChest tries to move src into the chest. Returns true if fully moved.
func moveToChest(cs *ChestState, src *game.ItemStack) bool {
	// Try stacking first
	for i := 0; i < 27; i++ {
		dst := &cs.Items[i]
		if dst.ID == src.ID && dst.Count > 0 && dst.Count < 64 && dst.MaxDurability == 0 {
			space := int32(64) - dst.Count
			if space >= src.Count {
				dst.Count += src.Count
				*src = game.ItemStack{}
				return true
			}
			dst.Count = 64
			src.Count -= space
		}
	}
	for i := 0; i < 27; i++ {
		dst := &cs.Items[i]
		if dst.ID == 0 || dst.Count <= 0 {
			*dst = *src
			*src = game.ItemStack{}
			return true
		}
	}
	return false
}

// moveToDoubleChest tries to move src into a double chest (both halves). Returns true if fully moved.
func moveToDoubleChest(cs *ChestState, src *game.ItemStack) bool {
	left, right := doubleChestOrder(cs)
	// Try left half first, then right
	for _, half := range []*ChestState{left, right} {
		for i := 0; i < 27; i++ {
			dst := &half.Items[i]
			if dst.ID == src.ID && dst.Count > 0 && dst.Count < 64 && dst.MaxDurability == 0 {
				space := int32(64) - dst.Count
				if space >= src.Count {
					dst.Count += src.Count
					*src = game.ItemStack{}
					return true
				}
				dst.Count = 64
				src.Count -= space
			}
		}
	}
	for _, half := range []*ChestState{left, right} {
		for i := 0; i < 27; i++ {
			dst := &half.Items[i]
			if dst.ID == 0 || dst.Count <= 0 {
				*dst = *src
				*src = game.ItemStack{}
				return true
			}
		}
	}
	return false
}

// moveToPlayerInv moves src into player inventory slots 9-44.
func moveToPlayerInv(player *game.Player, src *game.ItemStack) {
	moveToRange(player, src, 9, 44)
}

// moveToRange moves src into player.Inventory[start..end].
func moveToRange(player *game.Player, src *game.ItemStack, start, end int) {
	// Try stacking
	for i := start; i <= end; i++ {
		dst := &player.Inventory[i]
		if dst.ID == src.ID && dst.Count > 0 && dst.Count < 64 && dst.MaxDurability == 0 {
			space := int32(64) - dst.Count
			if space >= src.Count {
				dst.Count += src.Count
				*src = game.ItemStack{}
				return
			}
			dst.Count = 64
			src.Count -= space
		}
	}
	for i := start; i <= end; i++ {
		dst := &player.Inventory[i]
		if dst.ID == 0 || dst.Count <= 0 {
			*dst = *src
			*src = game.ItemStack{}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Furnace (window ID 3) handlers
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleFurnaceClick(player *game.Player, fs *FurnaceState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := FurnaceSlot(player, fs, slot)
	if invItem == nil {
		return
	}

	// Slot 2 (output) is take-only
	if slot == 2 {
		if invItem.ID == 0 || invItem.Count <= 0 {
			return
		}
		if player.CursorItem.ID != 0 && player.CursorItem.ID != invItem.ID {
			return
		}
		if player.CursorItem.ID == invItem.ID && player.CursorItem.Count+invItem.Count > 64 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
		} else {
			player.CursorItem.Count += invItem.Count
		}
		*invItem = game.ItemStack{}
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleFurnaceShiftClick(player *game.Player, fs *FurnaceState, slot int) {
	src := FurnaceSlot(player, fs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot == 0 || slot == 1 || slot == 2:
		// Furnace slot → player inventory
		moveToPlayerInv(player, src)
	case slot >= 3 && slot <= 29:
		// Main inv → try to put smeltable in input, fuel in fuel slot, else hotbar
		itemName := ItemNameByID(src.ID)
		if _, ok := smeltingRecipes[itemName]; ok {
			putInFurnaceSlot(&fs.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&fs.Fuel, src)
		} else {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 30 && slot <= 38:
		// Hotbar → try smeltable/fuel, else main inv
		itemName := ItemNameByID(src.ID)
		if _, ok := smeltingRecipes[itemName]; ok {
			putInFurnaceSlot(&fs.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&fs.Fuel, src)
		} else {
			moveToRange(player, src, 9, 35)
		}
	}
}

func (h *InventoryHandler) handleFurnaceNumberKey(player *game.Player, fs *FurnaceState, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}
	src := FurnaceSlot(player, fs, slot)
	if src == nil {
		return
	}
	// Don't allow swapping into output slot
	if slot == 2 {
		return
	}
	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleFurnaceDrop(player *game.Player, fs *FurnaceState, slot, button int) {
	if slot == -999 {
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}
	src := FurnaceSlot(player, fs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// putInFurnaceSlot tries to put src into a furnace slot (input or fuel).
func putInFurnaceSlot(dst *game.ItemStack, src *game.ItemStack) {
	if dst.ID == 0 || dst.Count <= 0 {
		*dst = *src
		*src = game.ItemStack{}
	} else if dst.ID == src.ID && dst.Count < 64 {
		space := int32(64) - dst.Count
		if space >= src.Count {
			dst.Count += src.Count
			*src = game.ItemStack{}
		} else {
			dst.Count = 64
			src.Count -= space
		}
	}
}

// ---------------------------------------------------------------------------
// Brewing stand (window ID 7) handlers
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleBrewingClick(player *game.Player, bs *BrewingState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := BrewingSlot(player, bs, slot)
	if invItem == nil {
		return
	}

	// Slots 0-2 (bottle outputs) are take-only when brewing is active
	if slot >= 0 && slot <= 2 {
		if invItem.ID == 0 || invItem.Count <= 0 {
			// Empty bottle slot — allow placing bottles/potions in
			genericClick(player, invItem, button)
			return
		}
		// Allow taking from bottle slots
		if player.CursorItem.ID != 0 && player.CursorItem.ID != invItem.ID {
			// Swap items
			player.CursorItem, *invItem = *invItem, player.CursorItem
			return
		}
		if player.CursorItem.ID == invItem.ID && player.CursorItem.Count+invItem.Count > 64 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
		} else {
			player.CursorItem.Count += invItem.Count
		}
		*invItem = game.ItemStack{}
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleBrewingShiftClick(player *game.Player, bs *BrewingState, slot int) {
	src := BrewingSlot(player, bs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot >= 0 && slot <= 4:
		// Brewing stand slot → player inventory
		moveToPlayerInv(player, src)
	case slot >= 5 && slot <= 31:
		// Main inv → try to put in brewing stand slots, else hotbar
		itemName := ItemNameByID(src.ID)
		if isBrewingIngredient(itemName) {
			putInFurnaceSlot(&bs.Ingredient, src)
		} else if itemName == "blaze_powder" {
			putInFurnaceSlot(&bs.Fuel, src)
		} else if isBottleOrPotion(itemName) {
			putInBrewingBottleSlot(bs, src)
		} else {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 32 && slot <= 40:
		// Hotbar → try brewing stand slots, else main inv
		itemName := ItemNameByID(src.ID)
		if isBrewingIngredient(itemName) {
			putInFurnaceSlot(&bs.Ingredient, src)
		} else if itemName == "blaze_powder" {
			putInFurnaceSlot(&bs.Fuel, src)
		} else if isBottleOrPotion(itemName) {
			putInBrewingBottleSlot(bs, src)
		} else {
			moveToRange(player, src, 9, 35)
		}
	}
}

func (h *InventoryHandler) handleBrewingNumberKey(player *game.Player, bs *BrewingState, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}
	src := BrewingSlot(player, bs, slot)
	if src == nil {
		return
	}
	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleBrewingDrop(player *game.Player, bs *BrewingState, slot, button int) {
	if slot == -999 {
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}
	src := BrewingSlot(player, bs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// isBrewingIngredient returns true if the item can be used as a brewing ingredient.
func isBrewingIngredient(itemName string) bool {
	_, ok := brewingRecipes[itemName]
	return ok
}

// isBottleOrPotion returns true if the item is a bottle or potion.
func isBottleOrPotion(itemName string) bool {
	switch itemName {
	case "potion", "splash_potion", "lingering_potion", "glass_bottle":
		return true
	}
	return false
}

// putInBrewingBottleSlot tries to put src into the first empty bottle slot (0-2).
func putInBrewingBottleSlot(bs *BrewingState, src *game.ItemStack) {
	for i := 0; i < 3; i++ {
		if bs.Bottles[i].ID == 0 || bs.Bottles[i].Count <= 0 {
			bs.Bottles[i] = *src
			*src = game.ItemStack{}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Generic click handler for chest/furnace slots (shared logic)
// ---------------------------------------------------------------------------

func genericClick(player *game.Player, invItem *game.ItemStack, button int) {
	if button == 0 { // Left click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
			*invItem = game.ItemStack{}
		} else if invItem.ID == 0 {
			*invItem = player.CursorItem
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.ID == invItem.ID && invItem.MaxDurability == 0 {
			space := int32(64) - invItem.Count
			if space >= player.CursorItem.Count {
				invItem.Count += player.CursorItem.Count
				player.CursorItem = game.ItemStack{}
			} else {
				invItem.Count = 64
				player.CursorItem.Count -= space
			}
		} else {
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	} else { // Right click
		if player.CursorItem.ID == 0 && invItem.ID == 0 {
			return
		}
		if player.CursorItem.ID == 0 {
			half := (invItem.Count + 1) / 2
			player.CursorItem = game.ItemStack{ID: invItem.ID, Count: half,
				Durability: invItem.Durability, MaxDurability: invItem.MaxDurability}
			invItem.Count -= half
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
		} else if invItem.ID == 0 || invItem.ID == player.CursorItem.ID {
			if invItem.ID == 0 {
				*invItem = game.ItemStack{ID: player.CursorItem.ID, Count: 0}
			}
			if invItem.Count < 64 {
				invItem.Count++
				player.CursorItem.Count--
				if player.CursorItem.Count <= 0 {
					player.CursorItem = game.ItemStack{}
				}
			}
		} else {
			player.CursorItem, *invItem = *invItem, player.CursorItem
		}
	}
}

// handleContainerButtonClick handles ServerboundContainerButtonClick.
// Used for enchanting table button clicks.
func (h *InventoryHandler) handleContainerButtonClick(player *game.Player, p pk.Packet) {
	var windowID pk.VarInt
	var buttonID pk.VarInt
	if err := p.Scan(&windowID, &buttonID); err != nil {
		return
	}
	if h.EnchantMgr != nil && player.EnchantSession != nil && int(windowID) == player.EnchantSession.WindowID {
		h.EnchantMgr.HandleEnchantButton(player, int(buttonID))
	}
	if h.StonecutterMgr != nil && int(windowID) == 14 && player.OpenWindowID == 14 {
		h.StonecutterMgr.HandleStonecutterSelect(player, int(buttonID))
	}
	if h.GrindstoneMgr != nil && int(windowID) == 13 && player.OpenWindowID == 13 {
		h.GrindstoneMgr.HandleGrindstoneClick(player, int(buttonID))
	}
	if h.SmithingMgr != nil && int(windowID) == SmithingWindowID && player.OpenWindowID == SmithingWindowID {
		h.SmithingMgr.HandleSmithingClick(player, int(buttonID))
	}
}

func (h *InventoryHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}

// ---------------------------------------------------------------------------
// Merchant (window ID 20) handlers
// ---------------------------------------------------------------------------

// handleMerchantClick processes clicks in the merchant (villager) window.
// The merchant window has 3 virtual slots (0=input1, 1=input2, 2=result)
// plus the player's main inventory (3-29) and hotbar (30-38).
// The actual trade execution is handled server-side via ExecuteTrade.
func (h *InventoryHandler) handleMerchantClick(player *game.Player, slot, button, mode int) {
	if h.VillagerMgr == nil {
		return
	}

	// Clicking on result slot (2) triggers trade execution
	if slot == 2 && (mode == 0 || mode == 1) {
		// Find the first non-exhausted trade to execute.
		// In practice, the client should have sent ServerboundSelectTrade before clicking.
		idx := h.VillagerMgr.GetOpenTradeIndex(player)
		if idx >= 0 {
			h.VillagerMgr.ExecuteTrade(player, idx)
		}
		return
	}

	// For player inventory slots (3-38), handle normally
	invItem := MerchantSlot(player, slot)
	if invItem == nil {
		// Virtual slots 0-1 or out of range: ignore clicks on input slots
		// (Minecraft's merchant window doesn't let players put items in manually through clicks)
		return
	}

	switch mode {
	case 0:
		genericClick(player, invItem, button)
	case 1:
		// Shift-click: move items between main/hotbar
		if invItem.ID == 0 || invItem.Count <= 0 {
			return
		}
		if slot >= 3 && slot <= 29 {
			moveToRange(player, invItem, 36, 44)
		} else if slot >= 30 && slot <= 38 {
			moveToRange(player, invItem, 9, 35)
		}
	case 2:
		// Number key swap
		hotbarSlot := button + 36
		if hotbarSlot >= 36 && hotbarSlot <= 44 {
			*invItem, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *invItem
		}
	case 4:
		// Drop
		if invItem.ID == 0 || invItem.Count <= 0 {
			return
		}
		if button == 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
		} else if button == 1 {
			*invItem = game.ItemStack{}
		}
	}

	SendMerchantWindowContent(player)
}

// ---------------------------------------------------------------------------
// Anvil (window ID 11) handlers
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleAnvilClick(player *game.Player, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	// Slot 2 (output) — take-only, requires XP
	if slot == 2 {
		h.handleAnvilOutputClick(player)
		return
	}

	invItem := AnvilSlot(player, slot)
	if invItem == nil {
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleAnvilOutputClick(player *game.Player) {
	if player.AnvilSession == nil {
		return
	}

	output := player.AnvilSession.Output
	if output.ID <= 0 || output.Count <= 0 {
		return
	}

	cost := player.AnvilSession.RepairCost
	if cost <= 0 {
		return
	}

	// Check XP
	if player.ExperienceLevel < cost {
		return
	}

	// Can the cursor accept this item?
	if player.CursorItem.ID != 0 && player.CursorItem.ID != output.ID {
		return
	}
	if player.CursorItem.ID == output.ID && player.CursorItem.Count+output.Count > 64 {
		return
	}

	// Consume XP
	player.ExperienceLevel -= cost
	if player.ExperienceLevel < 0 {
		player.ExperienceLevel = 0
	}
	SendExperience(player)

	// Give result to cursor
	if player.CursorItem.ID == 0 {
		player.CursorItem = output
	} else {
		player.CursorItem.Count += output.Count
	}

	// Consume input and material
	player.AnvilSession.Input = game.ItemStack{}
	player.AnvilSession.Material = game.ItemStack{}
	player.AnvilSession.Output = game.ItemStack{}
	player.AnvilSession.RepairCost = 0
	player.AnvilSession.RenameText = ""

	// Play anvil use sound
	if h.Chests != nil && h.Chests.Manager != nil {
		px, py, pz := player.Position()
		BroadcastSound(h.Chests.Manager, SoundAnvilUse, SoundCategoryBlock, px, py, pz, 1.0, 1.0)
	}
}

func (h *InventoryHandler) handleAnvilShiftClick(player *game.Player, slot int) {
	if slot == 2 {
		h.handleAnvilOutputShiftClick(player)
		return
	}

	src := AnvilSlot(player, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	if slot == 0 || slot == 1 {
		// Anvil input/material → player inventory
		moveToPlayerInv(player, src)
	} else if slot >= 3 && slot <= 29 {
		// Main inv → try anvil input first, then material, else hotbar
		if player.AnvilSession != nil {
			if player.AnvilSession.Input.ID == 0 || player.AnvilSession.Input.Count <= 0 {
				player.AnvilSession.Input = *src
				*src = game.ItemStack{}
				return
			}
			if player.AnvilSession.Material.ID == 0 || player.AnvilSession.Material.Count <= 0 {
				player.AnvilSession.Material = *src
				*src = game.ItemStack{}
				return
			}
		}
		moveToRange(player, src, 36, 44)
	} else if slot >= 30 && slot <= 38 {
		// Hotbar → try anvil input first, then material, else main inv
		if player.AnvilSession != nil {
			if player.AnvilSession.Input.ID == 0 || player.AnvilSession.Input.Count <= 0 {
				player.AnvilSession.Input = *src
				*src = game.ItemStack{}
				return
			}
			if player.AnvilSession.Material.ID == 0 || player.AnvilSession.Material.Count <= 0 {
				player.AnvilSession.Material = *src
				*src = game.ItemStack{}
				return
			}
		}
		moveToRange(player, src, 9, 35)
	}
}

func (h *InventoryHandler) handleAnvilOutputShiftClick(player *game.Player) {
	if player.AnvilSession == nil {
		return
	}

	output := player.AnvilSession.Output
	if output.ID <= 0 || output.Count <= 0 {
		return
	}

	cost := player.AnvilSession.RepairCost
	if cost <= 0 {
		return
	}

	// Check XP
	if player.ExperienceLevel < cost {
		return
	}

	// Try to move to inventory
	addedSlot := player.Inventory.AddItem(output.ID, output.Count)
	if addedSlot < 0 {
		return // inventory full
	}

	// Consume XP
	player.ExperienceLevel -= cost
	if player.ExperienceLevel < 0 {
		player.ExperienceLevel = 0
	}
	SendExperience(player)

	// Consume input and material
	player.AnvilSession.Input = game.ItemStack{}
	player.AnvilSession.Material = game.ItemStack{}
	player.AnvilSession.Output = game.ItemStack{}
	player.AnvilSession.RepairCost = 0
	player.AnvilSession.RenameText = ""
}

func (h *InventoryHandler) handleAnvilNumberKey(player *game.Player, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}

	// Don't allow number-key swap into the output slot
	if slot == 2 {
		return
	}

	src := AnvilSlot(player, slot)
	if src == nil {
		return
	}

	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleAnvilDrop(player *game.Player, slot, button int) {
	if slot == -999 {
		if button == 0 && player.CursorItem.Count > 0 {
			player.CursorItem.Count--
			if player.CursorItem.Count <= 0 {
				player.CursorItem = game.ItemStack{}
			}
		} else if button == 1 {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	// Don't allow dropping from output slot
	if slot == 2 {
		return
	}

	src := AnvilSlot(player, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// --- Hopper handlers ---

func (h *InventoryHandler) handleHopperClick(player *game.Player, hs *HopperState, slot, button int) {
	if slot < 0 || slot > 40 {
		return
	}
	src := HopperSlot(player, hs, slot)
	if src == nil {
		return
	}
	genericClick(player, src, button)
}

func (h *InventoryHandler) handleHopperShiftClick(player *game.Player, hs *HopperState, slot int) {
	if slot < 0 || slot > 40 {
		return
	}
	src := HopperSlot(player, hs, slot)
	if src == nil || src.ID == 0 {
		return
	}
	if slot < 5 {
		// Move from hopper to player inventory
		player.Inventory.AddItem(src.ID, src.Count)
		*src = game.ItemStack{}
	} else {
		// Move from player to hopper
		for i := 0; i < 5; i++ {
			if hs.Slots[i].ID == src.ID && hs.Slots[i].Count < 64 {
				space := int32(64) - hs.Slots[i].Count
				if src.Count <= space {
					hs.Slots[i].Count += src.Count
					*src = game.ItemStack{}
					return
				}
				hs.Slots[i].Count = 64
				src.Count -= space
			}
		}
		for i := 0; i < 5; i++ {
			if hs.Slots[i].ID == 0 {
				hs.Slots[i] = *src
				*src = game.ItemStack{}
				return
			}
		}
	}
}

func (h *InventoryHandler) handleHopperNumberKey(player *game.Player, hs *HopperState, slot, button int) {
	if slot < 0 || slot > 40 || button < 0 || button > 8 {
		return
	}
	src := HopperSlot(player, hs, slot)
	if src == nil {
		return
	}
	hotbarSlot := 36 + button
	src2 := &player.Inventory[hotbarSlot]
	*src, *src2 = *src2, *src
}

func (h *InventoryHandler) handleHopperDrop(player *game.Player, hs *HopperState, slot, button int) {
	if slot < 0 || slot > 40 {
		return
	}
	src := HopperSlot(player, hs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// --- Dispenser/Dropper handlers ---

func (h *InventoryHandler) handleDispenserClick(player *game.Player, ds *DispenserState, slot, button int) {
	if slot < 0 || slot > 44 {
		return
	}
	src := DispenserSlot(player, ds, slot)
	if src == nil {
		return
	}
	genericClick(player, src, button)
}

func (h *InventoryHandler) handleDispenserShiftClick(player *game.Player, ds *DispenserState, slot int) {
	if slot < 0 || slot > 44 {
		return
	}
	src := DispenserSlot(player, ds, slot)
	if src == nil || src.ID == 0 {
		return
	}
	if slot < 9 {
		// Move from dispenser to player inventory
		player.Inventory.AddItem(src.ID, src.Count)
		*src = game.ItemStack{}
	} else {
		// Move from player to dispenser
		for i := 0; i < 9; i++ {
			if ds.Slots[i].ID == src.ID && ds.Slots[i].Count < 64 {
				space := int32(64) - ds.Slots[i].Count
				if src.Count <= space {
					ds.Slots[i].Count += src.Count
					*src = game.ItemStack{}
					return
				}
				ds.Slots[i].Count = 64
				src.Count -= space
			}
		}
		for i := 0; i < 9; i++ {
			if ds.Slots[i].ID == 0 {
				ds.Slots[i] = *src
				*src = game.ItemStack{}
				return
			}
		}
	}
}

func (h *InventoryHandler) handleDispenserNumberKey(player *game.Player, ds *DispenserState, slot, button int) {
	if slot < 0 || slot > 44 || button < 0 || button > 8 {
		return
	}
	src := DispenserSlot(player, ds, slot)
	if src == nil {
		return
	}
	hotbarSlot := 36 + button
	src2 := &player.Inventory[hotbarSlot]
	*src, *src2 = *src2, *src
}

func (h *InventoryHandler) handleDispenserDrop(player *game.Player, ds *DispenserState, slot, button int) {
	if slot < 0 || slot > 44 {
		return
	}
	src := DispenserSlot(player, ds, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}
	if button == 0 {
		src.Count--
		if src.Count <= 0 {
			*src = game.ItemStack{}
		}
	} else if button == 1 {
		*src = game.ItemStack{}
	}
}

// ---------------------------------------------------------------------------
// Barrel (window ID 12) handlers — same layout as single chest (27 slots)
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleBarrelClick(player *game.Player, bs *BarrelState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := BarrelSlot(player, bs, slot)
	if invItem == nil {
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleBarrelShiftClick(player *game.Player, bs *BarrelState, slot int) {
	src := BarrelSlot(player, bs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot >= 0 && slot <= 26:
		moveToPlayerInv(player, src)
	case slot >= 27 && slot <= 53:
		if !moveToBarrel(bs, src) {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 54 && slot <= 62:
		if !moveToBarrel(bs, src) {
			moveToRange(player, src, 9, 35)
		}
	}
}

func moveToBarrel(bs *BarrelState, src *game.ItemStack) bool {
	for i := 0; i < 27; i++ {
		if bs.Items[i].ID == src.ID && bs.Items[i].Count < 64 {
			space := int32(64) - bs.Items[i].Count
			if src.Count <= space {
				bs.Items[i].Count += src.Count
				*src = game.ItemStack{}
				return true
			}
			bs.Items[i].Count = 64
			src.Count -= space
		}
	}
	for i := 0; i < 27; i++ {
		if bs.Items[i].ID == 0 {
			bs.Items[i] = *src
			*src = game.ItemStack{}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Shulker Box (window ID 17) handlers — same layout as barrel/chest (27 slots)
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleShulkerBoxClick(player *game.Player, sbs *ShulkerBoxState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	// Shulker boxes cannot contain other shulker boxes
	if slot >= 0 && slot <= 26 && player.CursorItem.ID > 0 {
		cursorName := ItemNameByID(player.CursorItem.ID)
		if IsShulkerBox(cursorName) {
			return
		}
	}

	invItem := ShulkerBoxSlot(player, sbs, slot)
	if invItem == nil {
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleShulkerBoxShiftClick(player *game.Player, sbs *ShulkerBoxState, slot int) {
	src := ShulkerBoxSlot(player, sbs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot >= 0 && slot <= 26:
		moveToPlayerInv(player, src)
	case slot >= 27 && slot <= 53:
		srcName := ItemNameByID(src.ID)
		if IsShulkerBox(srcName) {
			moveToRange(player, src, 36, 44)
			return
		}
		if !moveToShulkerBox(sbs, src) {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 54 && slot <= 62:
		srcName := ItemNameByID(src.ID)
		if IsShulkerBox(srcName) {
			moveToRange(player, src, 9, 35)
			return
		}
		if !moveToShulkerBox(sbs, src) {
			moveToRange(player, src, 9, 35)
		}
	}
}

func moveToShulkerBox(sbs *ShulkerBoxState, src *game.ItemStack) bool {
	for i := 0; i < 27; i++ {
		if sbs.Items[i].ID == src.ID && sbs.Items[i].Count < 64 {
			space := int32(64) - sbs.Items[i].Count
			if src.Count <= space {
				sbs.Items[i].Count += src.Count
				*src = game.ItemStack{}
				return true
			}
			sbs.Items[i].Count = 64
			src.Count -= space
		}
	}
	for i := 0; i < 27; i++ {
		if sbs.Items[i].ID == 0 {
			sbs.Items[i] = *src
			*src = game.ItemStack{}
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Smoker (window ID 15) handlers — same layout as furnace (3 slots)
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleSmokerClick(player *game.Player, ss *SmokerState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := SmokerSlot(player, ss, slot)
	if invItem == nil {
		return
	}

	if slot == 2 {
		if invItem.ID == 0 || invItem.Count <= 0 {
			return
		}
		if player.CursorItem.ID != 0 && player.CursorItem.ID != invItem.ID {
			return
		}
		if player.CursorItem.ID == invItem.ID && player.CursorItem.Count+invItem.Count > 64 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
		} else {
			player.CursorItem.Count += invItem.Count
		}
		*invItem = game.ItemStack{}
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleSmokerShiftClick(player *game.Player, ss *SmokerState, slot int) {
	src := SmokerSlot(player, ss, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot == 0 || slot == 1 || slot == 2:
		moveToPlayerInv(player, src)
	case slot >= 3 && slot <= 29:
		itemName := ItemNameByID(src.ID)
		if smokerSmeltable(itemName) != "" {
			putInFurnaceSlot(&ss.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&ss.Fuel, src)
		} else {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 30 && slot <= 38:
		itemName := ItemNameByID(src.ID)
		if smokerSmeltable(itemName) != "" {
			putInFurnaceSlot(&ss.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&ss.Fuel, src)
		} else {
			moveToRange(player, src, 9, 35)
		}
	}
}

// ---------------------------------------------------------------------------
// Blast Furnace (window ID 16) handlers — same layout as furnace (3 slots)
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleBlastFurnaceClick(player *game.Player, bfs *BlastFurnaceState, slot, button int) {
	if slot == -999 {
		if button == 0 {
			player.CursorItem = game.ItemStack{}
		} else if player.CursorItem.Count > 1 {
			player.CursorItem.Count--
		} else {
			player.CursorItem = game.ItemStack{}
		}
		return
	}

	invItem := BlastFurnaceSlot(player, bfs, slot)
	if invItem == nil {
		return
	}

	if slot == 2 {
		if invItem.ID == 0 || invItem.Count <= 0 {
			return
		}
		if player.CursorItem.ID != 0 && player.CursorItem.ID != invItem.ID {
			return
		}
		if player.CursorItem.ID == invItem.ID && player.CursorItem.Count+invItem.Count > 64 {
			return
		}
		if player.CursorItem.ID == 0 {
			player.CursorItem = *invItem
		} else {
			player.CursorItem.Count += invItem.Count
		}
		*invItem = game.ItemStack{}
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleBlastFurnaceShiftClick(player *game.Player, bfs *BlastFurnaceState, slot int) {
	src := BlastFurnaceSlot(player, bfs, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	switch {
	case slot == 0 || slot == 1 || slot == 2:
		moveToPlayerInv(player, src)
	case slot >= 3 && slot <= 29:
		itemName := ItemNameByID(src.ID)
		if blastFurnaceSmeltable(itemName) != "" {
			putInFurnaceSlot(&bfs.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&bfs.Fuel, src)
		} else {
			moveToRange(player, src, 36, 44)
		}
	case slot >= 30 && slot <= 38:
		itemName := ItemNameByID(src.ID)
		if blastFurnaceSmeltable(itemName) != "" {
			putInFurnaceSlot(&bfs.Input, src)
		} else if _, ok := fuelBurnTicks[itemName]; ok {
			putInFurnaceSlot(&bfs.Fuel, src)
		} else {
			moveToRange(player, src, 9, 35)
		}
	}
}
