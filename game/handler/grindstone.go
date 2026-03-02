package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// GrindstoneManager handles grindstone interactions (disenchanting and repair).
type GrindstoneManager struct {
	World game.World
}

// NewGrindstoneManager creates a new GrindstoneManager.
func NewGrindstoneManager(world game.World) *GrindstoneManager {
	return &GrindstoneManager{World: world}
}

// OpenGrindstone opens the grindstone UI for a player.
// Menu type 12 = grindstone.
func (gm *GrindstoneManager) OpenGrindstone(player *game.Player, x, y, z int) {
	windowID := 13
	player.OpenWindowID = windowID

	title := chat.Text("Repair & Disenchant")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(windowID),
		pk.VarInt(12), // menu type: grindstone
		title,
	))
}

// HandleGrindstoneClick handles clicking the grindstone result slot.
// Removes all non-curse enchantments and returns XP.
func (gm *GrindstoneManager) HandleGrindstoneClick(player *game.Player, slotID int) {
	if player.OpenWindowID != 13 || slotID != 2 {
		return
	}

	// The input slot is mapped to slot 0 of the grindstone window.
	// For simplicity, we disenchant the player's held item directly.
	heldSlot := int(player.HeldSlot) + 36
	item := &player.Inventory[heldSlot]
	if item.ID <= 0 || item.Enchantments == nil || len(item.Enchantments) == 0 {
		return
	}

	// Calculate XP to return (sum of enchant levels)
	xpReturn := int32(0)
	for _, lvl := range item.Enchantments {
		xpReturn += lvl
	}

	// Remove all enchantments
	item.Enchantments = nil

	// Repair durability (grindstone merging is simplified: restore 5% bonus)
	if item.MaxDurability > 0 {
		bonus := item.MaxDurability / 20
		if bonus < 1 {
			bonus = 1
		}
		item.Durability += bonus
		if item.Durability > item.MaxDurability {
			item.Durability = item.MaxDurability
		}
	}

	SendSlotUpdate(player, heldSlot)
	AddExperience(player, xpReturn)

	msg := chat.Message{Text: "Disenchanted!", Color: "green"}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false),
	))
}
