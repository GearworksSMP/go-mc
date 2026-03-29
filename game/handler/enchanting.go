package handler

import (
	"math/rand"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	pk "github.com/Tnze/go-mc/net/packet"
)

// EnchantOffer represents one of the three enchantment offers.
type EnchantOffer struct {
	RequiredLevel int32
	EnchantID     string
	EnchantLevel  int32
}

// EnchantSession tracks an active enchanting table interaction.
type EnchantSession struct {
	TablePos [3]int
	WindowID int
	Offers   [3]EnchantOffer
}

// EnchantManager handles enchanting table interactions.
type EnchantManager struct {
	World game.World
}

// NewEnchantManager creates a new EnchantManager.
func NewEnchantManager(world game.World) *EnchantManager {
	return &EnchantManager{World: world}
}

// OpenEnchantingTable opens the enchanting table UI for a player.
func (em *EnchantManager) OpenEnchantingTable(player *game.Player, x, y, z int) {
	windowID := 10 // use a distinct window ID for enchanting
	player.OpenWindowID = windowID

	title := chat.Text("Enchant")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(windowID),
		pk.VarInt(13), // menu type: enchantment
		title,
	))

	// Count nearby bookshelves (within 2 blocks, 1 block air gap)
	bookshelves := em.countBookshelves(x, y, z)
	if bookshelves > 15 {
		bookshelves = 15
	}

	// Generate 3 enchantment offers based on bookshelf count
	offers := em.generateOffers(bookshelves)

	// Store session data on player
	player.EnchantSession = &game.EnchantSessionData{
		TablePos: [3]int{x, y, z},
		WindowID: windowID,
		Offers: [3]game.EnchantOfferData{
			{RequiredLevel: offers[0].RequiredLevel, EnchantID: offers[0].EnchantID, EnchantLevel: offers[0].EnchantLevel},
			{RequiredLevel: offers[1].RequiredLevel, EnchantID: offers[1].EnchantID, EnchantLevel: offers[1].EnchantLevel},
			{RequiredLevel: offers[2].RequiredLevel, EnchantID: offers[2].EnchantID, EnchantLevel: offers[2].EnchantLevel},
		},
	}

	// Send enchantment slot levels via container properties
	// Property 0,1,2 = required levels for slots 0,1,2
	for i := 0; i < 3; i++ {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.UnsignedByte(byte(windowID)),
			pk.Short(int16(i)),
			pk.Short(int16(offers[i].RequiredLevel)),
		))
	}
	// Property 4,5,6 = enchantment IDs (we'll send -1 for "hidden")
	for i := 4; i <= 6; i++ {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.UnsignedByte(byte(windowID)),
			pk.Short(int16(i)),
			pk.Short(-1),
		))
	}
	// Property 7,8,9 = enchantment levels
	for i := 7; i <= 9; i++ {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.UnsignedByte(byte(windowID)),
			pk.Short(int16(i)),
			pk.Short(-1),
		))
	}
}

// HandleEnchantButton handles a player clicking an enchant button (0, 1, or 2).
func (em *EnchantManager) HandleEnchantButton(player *game.Player, buttonID int) {
	if player.EnchantSession == nil || buttonID < 0 || buttonID > 2 {
		return
	}

	offer := player.EnchantSession.Offers[buttonID]
	if offer.RequiredLevel <= 0 {
		return
	}

	// Check player has enough XP levels
	if player.ExperienceLevel < offer.RequiredLevel {
		return
	}

	// Check player has an item in the enchanting slot
	heldSlot := int(player.HeldSlot) + 36
	heldItem := &player.Inventory[heldSlot]
	if heldItem.ID <= 0 || heldItem.Count <= 0 {
		return
	}

	// Consume XP levels (cost = button index + 1)
	lapisCost := int32(buttonID + 1)
	player.ExperienceLevel -= lapisCost
	if player.ExperienceLevel < 0 {
		player.ExperienceLevel = 0
	}
	SendExperience(player)

	// Apply enchantment
	heldItem.Enchantments = enchant.ApplyToItem(heldItem.Enchantments, offer.EnchantID, offer.EnchantLevel)

	SendSlotUpdate(player, heldSlot)

	// Send success message
	msg := chat.Message{
		Text:  "Enchanted with " + offer.EnchantID + "!",
		Color: "green",
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false),
	))

	// Clear session
	player.EnchantSession = nil
}

// countBookshelves counts bookshelves within the standard enchanting table range.
// Vanilla checks a 5x5 ring at distance 2, at table height and one block above,
// with an air gap between the table and the bookshelf.
func (em *EnchantManager) countBookshelves(tx, ty, tz int) int {
	count := 0
	// Check 5x5 area around table, at table height and 1 block above
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			// Only check the outer ring (distance 2 in either axis)
			if dx > -2 && dx < 2 && dz > -2 && dz < 2 {
				continue
			}
			for dy := 0; dy <= 1; dy++ {
				bx, by, bz := tx+dx, ty+dy, tz+dz
				state, err := em.World.GetBlock(bx, by, bz)
				if err != nil {
					continue
				}
				name := BlockNameFromState(int(state))
				if name == "bookshelf" {
					count++
				}
			}
		}
	}
	return count
}

// generateOffers creates 3 enchantment offers based on bookshelf count.
func (em *EnchantManager) generateOffers(bookshelves int) [3]EnchantOffer {
	var offers [3]EnchantOffer

	// Pool of enchantments with max levels
	type enchDef struct {
		id       string
		maxLevel int32
	}
	pool := []enchDef{
		// Melee
		{enchant.Sharpness, enchant.MaxLevel(enchant.Sharpness)},
		{enchant.Smite, enchant.MaxLevel(enchant.Smite)},
		{enchant.BaneOfArthropods, enchant.MaxLevel(enchant.BaneOfArthropods)},
		{enchant.Knockback, enchant.MaxLevel(enchant.Knockback)},
		{enchant.FireAspect, enchant.MaxLevel(enchant.FireAspect)},
		{enchant.Looting, enchant.MaxLevel(enchant.Looting)},
		{enchant.SweepingEdge, enchant.MaxLevel(enchant.SweepingEdge)},
		// Armor
		{enchant.Protection, enchant.MaxLevel(enchant.Protection)},
		{enchant.FireProtection, enchant.MaxLevel(enchant.FireProtection)},
		{enchant.BlastProtection, enchant.MaxLevel(enchant.BlastProtection)},
		{enchant.ProjectileProtection, enchant.MaxLevel(enchant.ProjectileProtection)},
		{enchant.Thorns, enchant.MaxLevel(enchant.Thorns)},
		{enchant.FeatherFalling, enchant.MaxLevel(enchant.FeatherFalling)},
		{enchant.Respiration, enchant.MaxLevel(enchant.Respiration)},
		{enchant.AquaAffinity, enchant.MaxLevel(enchant.AquaAffinity)},
		// Tools
		{enchant.Efficiency, enchant.MaxLevel(enchant.Efficiency)},
		{enchant.Unbreaking, enchant.MaxLevel(enchant.Unbreaking)},
		{enchant.Fortune, enchant.MaxLevel(enchant.Fortune)},
		{enchant.SilkTouch, enchant.MaxLevel(enchant.SilkTouch)},
		// Bows
		{enchant.Power, enchant.MaxLevel(enchant.Power)},
		{enchant.Punch, enchant.MaxLevel(enchant.Punch)},
		{enchant.Flame, enchant.MaxLevel(enchant.Flame)},
		{enchant.Infinity, enchant.MaxLevel(enchant.Infinity)},
		// Fishing
		{enchant.Lure, enchant.MaxLevel(enchant.Lure)},
		{enchant.LuckOfTheSea, enchant.MaxLevel(enchant.LuckOfTheSea)},
	}

	for i := 0; i < 3; i++ {
		// Required level scales with slot and bookshelves
		// Slot 0: low cost, Slot 1: medium, Slot 2: high
		base := (i + 1) * (bookshelves + 1) / 3
		if base < 1 {
			base = 1
		}
		maxLevel := (i + 1) * 10
		if maxLevel > 30 {
			maxLevel = 30
		}
		reqLevel := int32(base + rand.Intn(maxLevel-base+1))
		if reqLevel < 1 {
			reqLevel = 1
		}

		// Pick random enchantment
		ench := pool[rand.Intn(len(pool))]
		enchLevel := int32(1)
		if ench.maxLevel > 1 {
			enchLevel = 1 + int32(rand.Intn(int(ench.maxLevel)))
		}
		// Scale enchant level with required XP level
		if reqLevel >= 20 && enchLevel < ench.maxLevel {
			enchLevel++
		}
		if enchLevel > ench.maxLevel {
			enchLevel = ench.maxLevel
		}

		offers[i] = EnchantOffer{
			RequiredLevel: reqLevel,
			EnchantID:     ench.id,
			EnchantLevel:  enchLevel,
		}
	}

	return offers
}
