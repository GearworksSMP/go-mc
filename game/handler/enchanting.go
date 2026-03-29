package handler

import (
	"encoding/binary"
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
	World   game.World
	lapisID int32 // cached item ID for lapis_lazuli
}

// NewEnchantManager creates a new EnchantManager.
func NewEnchantManager(world game.World) *EnchantManager {
	return &EnchantManager{World: world, lapisID: itemIDByName("lapis_lazuli")}
}

// initEnchantSeed sets the player's enchant seed from their UUID if not yet set.
func initEnchantSeed(player *game.Player) {
	if player.EnchantSeed != 0 {
		return
	}
	hi := binary.BigEndian.Uint64(player.UUID[:8])
	lo := binary.BigEndian.Uint64(player.UUID[8:])
	player.EnchantSeed = int32(hi ^ lo)
	if player.EnchantSeed == 0 {
		player.EnchantSeed = 1 // avoid re-init on zero UUID
	}
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

	bookshelves := em.countBookshelves(x, y, z)
	if bookshelves > 15 {
		bookshelves = 15
	}

	initEnchantSeed(player)
	offers := generateOffers(bookshelves, player.EnchantSeed)

	player.EnchantSession = &game.EnchantSessionData{
		TablePos: [3]int{x, y, z},
		WindowID: windowID,
		Offers: [3]game.EnchantOfferData{
			{RequiredLevel: offers[0].RequiredLevel, EnchantID: offers[0].EnchantID, EnchantLevel: offers[0].EnchantLevel},
			{RequiredLevel: offers[1].RequiredLevel, EnchantID: offers[1].EnchantID, EnchantLevel: offers[1].EnchantLevel},
			{RequiredLevel: offers[2].RequiredLevel, EnchantID: offers[2].EnchantID, EnchantLevel: offers[2].EnchantLevel},
		},
	}

	// Send enchantment slot levels via container properties (0,1,2)
	for i := 0; i < 3; i++ {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundContainerSetData,
			pk.UnsignedByte(byte(windowID)),
			pk.Short(int16(i)),
			pk.Short(int16(offers[i].RequiredLevel)),
		))
	}
	// Properties 4-6: enchantment IDs, 7-9: enchantment levels (hidden)
	for i := 4; i <= 9; i++ {
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

	if player.ExperienceLevel < offer.RequiredLevel {
		return
	}

	heldSlot := int(player.HeldSlot) + 36
	heldItem := &player.Inventory[heldSlot]
	if heldItem.ID <= 0 || heldItem.Count <= 0 {
		return
	}

	lapisCost := int32(buttonID + 1)

	// Find lapis lazuli in inventory (slots 9-44: main + hotbar)
	lapisSlot := -1
	if em.lapisID > 0 {
		for i := 9; i <= 44; i++ {
			if player.Inventory[i].ID == em.lapisID && player.Inventory[i].Count >= lapisCost {
				lapisSlot = i
				break
			}
		}
		if lapisSlot < 0 {
			msg := chat.Message{Text: "Not enough lapis lazuli!", Color: "red"}
			player.WritePacket(pk.Marshal(
				packetid.ClientboundSystemChat, msg, pk.Boolean(false),
			))
			return
		}
	}

	// Consume XP levels
	player.ExperienceLevel -= lapisCost
	if player.ExperienceLevel < 0 {
		player.ExperienceLevel = 0
	}
	SendExperience(player)

	// Consume lapis lazuli
	if lapisSlot >= 0 {
		player.Inventory[lapisSlot].Count -= lapisCost
		if player.Inventory[lapisSlot].Count <= 0 {
			player.Inventory[lapisSlot] = game.ItemStack{}
		}
		SendSlotUpdate(player, lapisSlot)
	}

	// Apply enchantment
	heldItem.Enchantments = enchant.ApplyToItem(heldItem.Enchantments, offer.EnchantID, offer.EnchantLevel)
	SendSlotUpdate(player, heldSlot)

	msg := chat.Message{
		Text:  "Enchanted with " + offer.EnchantID + "!",
		Color: "green",
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat, msg, pk.Boolean(false),
	))

	// Reroll seed so the next table open shows different offers
	player.EnchantSeed ^= int32(offer.RequiredLevel*31 + offer.EnchantLevel*7)
	if player.EnchantSeed == 0 {
		player.EnchantSeed = 1
	}
	player.EnchantSession = nil
}

// countBookshelves counts bookshelves within the standard enchanting table range.
// Vanilla checks a 5x5 ring at distance 2, at table height and one block above,
// with an air gap between the table and the bookshelf.
func (em *EnchantManager) countBookshelves(tx, ty, tz int) int {
	count := 0
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			if dx > -2 && dx < 2 && dz > -2 && dz < 2 {
				continue
			}
			for dy := 0; dy <= 1; dy++ {
				bx, by, bz := tx+dx, ty+dy, tz+dz
				state, err := em.World.GetBlock(bx, by, bz)
				if err != nil {
					continue
				}
				if BlockNameFromState(int(state)) == "bookshelf" {
					count++
				}
			}
		}
	}
	return count
}

// generateOffers creates 3 deterministic enchantment offers from a seed and bookshelf count.
func generateOffers(bookshelves int, seed int32) [3]EnchantOffer {
	rng := rand.New(rand.NewSource(int64(seed)))
	var offers [3]EnchantOffer

	type enchDef struct {
		id       string
		maxLevel int32
	}
	pool := []enchDef{
		{enchant.Sharpness, enchant.MaxLevel(enchant.Sharpness)},
		{enchant.Smite, enchant.MaxLevel(enchant.Smite)},
		{enchant.BaneOfArthropods, enchant.MaxLevel(enchant.BaneOfArthropods)},
		{enchant.Knockback, enchant.MaxLevel(enchant.Knockback)},
		{enchant.FireAspect, enchant.MaxLevel(enchant.FireAspect)},
		{enchant.Looting, enchant.MaxLevel(enchant.Looting)},
		{enchant.SweepingEdge, enchant.MaxLevel(enchant.SweepingEdge)},
		{enchant.Protection, enchant.MaxLevel(enchant.Protection)},
		{enchant.FireProtection, enchant.MaxLevel(enchant.FireProtection)},
		{enchant.BlastProtection, enchant.MaxLevel(enchant.BlastProtection)},
		{enchant.ProjectileProtection, enchant.MaxLevel(enchant.ProjectileProtection)},
		{enchant.Thorns, enchant.MaxLevel(enchant.Thorns)},
		{enchant.FeatherFalling, enchant.MaxLevel(enchant.FeatherFalling)},
		{enchant.Respiration, enchant.MaxLevel(enchant.Respiration)},
		{enchant.AquaAffinity, enchant.MaxLevel(enchant.AquaAffinity)},
		{enchant.Efficiency, enchant.MaxLevel(enchant.Efficiency)},
		{enchant.Unbreaking, enchant.MaxLevel(enchant.Unbreaking)},
		{enchant.Fortune, enchant.MaxLevel(enchant.Fortune)},
		{enchant.SilkTouch, enchant.MaxLevel(enchant.SilkTouch)},
		{enchant.Power, enchant.MaxLevel(enchant.Power)},
		{enchant.Punch, enchant.MaxLevel(enchant.Punch)},
		{enchant.Flame, enchant.MaxLevel(enchant.Flame)},
		{enchant.Infinity, enchant.MaxLevel(enchant.Infinity)},
		{enchant.Lure, enchant.MaxLevel(enchant.Lure)},
		{enchant.LuckOfTheSea, enchant.MaxLevel(enchant.LuckOfTheSea)},
	}

	// Vanilla-style base level: rand(1, power/3+1) + 1 + rand(0, power/3)
	power := bookshelves
	b := rng.Intn(power/3+1) + 1 + rng.Intn(power/3+1)

	slotCosts := [3]int32{
		max(1, int32(b/3)),               // slot 0: lowest tier
		int32(b*2/3 + 1),                 // slot 1: middle tier
		max(int32(b), int32(power*2)),     // slot 2: highest tier
	}
	for i := range slotCosts {
		slotCosts[i] = max(1, min(30, slotCosts[i]))
	}

	for i := 0; i < 3; i++ {
		ench := pool[rng.Intn(len(pool))]
		enchLevel := int32(1)
		if ench.maxLevel > 1 {
			enchLevel = 1 + int32(rng.Intn(int(ench.maxLevel)))
		}
		if slotCosts[i] >= 20 && enchLevel < ench.maxLevel {
			enchLevel++
		}
		enchLevel = min(enchLevel, ench.maxLevel)

		offers[i] = EnchantOffer{
			RequiredLevel: slotCosts[i],
			EnchantID:     ench.id,
			EnchantLevel:  enchLevel,
		}
	}

	return offers
}
