package handler

import (
	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SendSlotUpdate sends a ClientboundContainerSetSlot to sync one inventory slot.
func SendSlotUpdate(player *game.Player, slot int) {
	s := player.Inventory[slot].ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetSlot,
		pk.UnsignedByte(0),    // window ID 0 = player inventory
		pk.VarInt(0),          // state ID
		pk.Short(int16(slot)), // slot index
		s,
	))
}

// ItemNameByID returns the item name (without minecraft: prefix) for an item ID,
// or "" if not found.
func ItemNameByID(id int32) string {
	if it, ok := item.ByID[item.ID(id)]; ok {
		return it.Name
	}
	return ""
}

// BlockNameFromState returns the block name (without minecraft: prefix) for a state ID,
// or "" if the state is unknown.
func BlockNameFromState(stateID int) string {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return ""
	}
	name := block.StateList[stateID].ID()
	if len(name) > 10 && name[:10] == "minecraft:" {
		return name[10:]
	}
	return name
}

// NewItemStack creates an ItemStack, auto-setting durability if the item is a tool, armor, or shield.
func NewItemStack(id, count int32) game.ItemStack {
	s := game.ItemStack{ID: id, Count: count}
	name := ItemNameByID(id)
	if maxDur := GetMaxDurabilityByItem(name); maxDur > 0 {
		s.Durability = maxDur
		s.MaxDurability = maxDur
	} else if maxDur := GetArmorDurability(name); maxDur > 0 {
		s.Durability = maxDur
		s.MaxDurability = maxDur
	} else if name == "shield" {
		s.Durability = shieldMaxDurability
		s.MaxDurability = shieldMaxDurability
	} else if name == "bow" {
		s.Durability = bowMaxDurability
		s.MaxDurability = bowMaxDurability
	}
	return s
}
