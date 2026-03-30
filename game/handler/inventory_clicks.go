package handler

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
)

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
	var equipArmorMaterial string
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
		equipArmorMaterial = armorInfo.Material
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

	// Play equip sound if armor was placed into an armor slot
	if equipArmorMaterial != "" {
		h.playPlayerSound(player, ArmorEquipSound(equipArmorMaterial))
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
			h.playPlayerSound(player, ArmorEquipSound(armorInfo.Material))
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
