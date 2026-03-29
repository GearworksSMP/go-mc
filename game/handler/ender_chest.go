package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// EnderChestWindowID is the window ID used for ender chest windows.
const EnderChestWindowID = 19

// OpenEnderChest opens the ender chest window for the player.
func OpenEnderChest(player *game.Player) {
	player.OpenWindowID = EnderChestWindowID

	title := chat.Text("Ender Chest")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(EnderChestWindowID), // window ID
		pk.VarInt(2),                  // menu type: generic_9x3
		title,
	))

	SendEnderChestContent(player)
}

// SendEnderChestContent sends the full ender chest window content to the player.
// Layout: 63 slots = 27 ender chest + 27 main inv + 9 hotbar.
func SendEnderChestContent(player *game.Player) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 63)
	// Slots 0-26: ender chest items
	for i := 0; i < 27; i++ {
		slots[i] = player.EnderItems[i].ToSlot()
	}
	// Main inventory: window slots 27-53 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[27+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: window slots 54-62 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[54+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(EnderChestWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// EnderChestSlot returns a pointer to the item stack for an ender chest window slot.
func EnderChestSlot(player *game.Player, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 26:
		return &player.EnderItems[windowSlot]
	case windowSlot >= 27 && windowSlot <= 53:
		return &player.Inventory[windowSlot-27+9]
	case windowSlot >= 54 && windowSlot <= 62:
		return &player.Inventory[windowSlot-54+36]
	}
	return nil
}

// ---------------------------------------------------------------------------
// Ender chest click handlers (mirrors chest handlers with per-player storage)
// ---------------------------------------------------------------------------

func (h *InventoryHandler) handleEnderChestClick(player *game.Player, slot, button int) {
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

	invItem := EnderChestSlot(player, slot)
	if invItem == nil {
		return
	}

	genericClick(player, invItem, button)
}

func (h *InventoryHandler) handleEnderChestShiftClick(player *game.Player, slot int) {
	src := EnderChestSlot(player, slot)
	if src == nil || src.ID == 0 || src.Count <= 0 {
		return
	}

	// Single chest layout: 0-26 = ender chest, 27-53 = main, 54-62 = hotbar
	if slot >= 0 && slot <= 26 {
		moveToPlayerInv(player, src)
	} else if slot >= 27 && slot <= 53 {
		if !moveToEnderChest(player, src) {
			moveToRange(player, src, 36, 44)
		}
	} else if slot >= 54 && slot <= 62 {
		if !moveToEnderChest(player, src) {
			moveToRange(player, src, 9, 35)
		}
	}
}

func (h *InventoryHandler) handleEnderChestNumberKey(player *game.Player, slot, button int) {
	hotbarSlot := button + 36
	if hotbarSlot < 36 || hotbarSlot > 44 {
		return
	}
	src := EnderChestSlot(player, slot)
	if src == nil {
		return
	}
	*src, player.Inventory[hotbarSlot] = player.Inventory[hotbarSlot], *src
}

func (h *InventoryHandler) handleEnderChestDrop(player *game.Player, slot, button int) {
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
	src := EnderChestSlot(player, slot)
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

// moveToEnderChest tries to move src into the player's ender chest. Returns true if fully moved.
func moveToEnderChest(player *game.Player, src *game.ItemStack) bool {
	// First try stacking
	for i := 0; i < 27; i++ {
		if player.EnderItems[i].ID == src.ID && player.EnderItems[i].Count > 0 && player.EnderItems[i].Count < 64 {
			space := int32(64) - player.EnderItems[i].Count
			if src.Count <= space {
				player.EnderItems[i].Count += src.Count
				*src = game.ItemStack{}
				return true
			}
			player.EnderItems[i].Count = 64
			src.Count -= space
		}
	}
	// Then find empty slot
	for i := 0; i < 27; i++ {
		if player.EnderItems[i].ID == 0 || player.EnderItems[i].Count <= 0 {
			player.EnderItems[i] = *src
			*src = game.ItemStack{}
			return true
		}
	}
	return false
}
