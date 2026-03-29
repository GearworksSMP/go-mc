package handler

import (
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// LoomWindowID is the container window ID for the loom UI.
const LoomWindowID = 25

// MaxBannerLayers is the maximum number of pattern layers on a banner.
const MaxBannerLayers = 6

// loomPatterns lists all vanilla banner patterns available in the loom.
// The index in this slice corresponds to the pattern selection button ID.
var loomPatterns = []string{
	"base",
	"stripe_bottom",
	"stripe_top",
	"stripe_left",
	"stripe_right",
	"stripe_center",
	"stripe_middle",
	"stripe_downright",
	"stripe_downleft",
	"small_stripes",
	"cross",
	"straight_cross",
	"triangle_bottom",
	"triangle_top",
	"triangles_bottom",
	"triangles_top",
	"diagonal_left",
	"diagonal_right",
	"diagonal_up_left",
	"diagonal_up_right",
	"half_vertical",
	"half_vertical_right",
	"half_horizontal",
	"half_horizontal_bottom",
	"square_bottom_left",
	"square_bottom_right",
	"square_top_left",
	"square_top_right",
	"rhombus",
	"circle",
	"border",
	"curly_border",
	"bricks",
	"gradient",
	"gradient_up",
	"creeper",
	"skull",
	"flower",
	"mojang",
	"globe",
	"piglin",
}

// specialPatternItems maps banner pattern items to the pattern they unlock.
// These patterns require the corresponding item in the pattern slot.
var specialPatternItems = map[string]string{
	"creeper_banner_pattern": "creeper",
	"skull_banner_pattern":   "skull",
	"flower_banner_pattern":  "flower",
	"mojang_banner_pattern":  "mojang",
	"globe_banner_pattern":   "globe",
	"piglin_banner_pattern":  "piglin",
}

// dyeNameToColor maps dye item names to their color name string.
var dyeNameToColor = map[string]string{
	"white_dye":      "white",
	"orange_dye":     "orange",
	"magenta_dye":    "magenta",
	"light_blue_dye": "light_blue",
	"yellow_dye":     "yellow",
	"lime_dye":       "lime",
	"pink_dye":       "pink",
	"gray_dye":       "gray",
	"light_gray_dye": "light_gray",
	"cyan_dye":       "cyan",
	"purple_dye":     "purple",
	"blue_dye":       "blue",
	"brown_dye":      "brown",
	"green_dye":      "green",
	"red_dye":        "red",
	"black_dye":      "black",
}

// LoomManager handles loom block interactions for applying banner patterns.
type LoomManager struct{}

// NewLoomManager creates a new LoomManager.
func NewLoomManager() *LoomManager {
	return &LoomManager{}
}

// OpenLoom opens the loom UI for a player.
// Menu type 14 = loom in the Minecraft registry.
func (lm *LoomManager) OpenLoom(player *game.Player) {
	player.OpenWindowID = LoomWindowID

	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(LoomWindowID),
		pk.VarInt(14), // menu type: loom
		chat.Text("Loom"),
	))
}

// HandleLoomSelect processes a pattern selection button click in the loom.
// The buttonID corresponds to the index in loomPatterns.
//
// Loom slot layout:
//
//	0 = banner input
//	1 = dye input
//	2 = pattern item (optional, for special patterns)
//	3 = output
func (lm *LoomManager) HandleLoomSelect(player *game.Player, buttonID int) {
	if player.OpenWindowID != LoomWindowID {
		return
	}

	if buttonID < 0 || buttonID >= len(loomPatterns) {
		return
	}

	patternName := loomPatterns[buttonID]

	bannerSlot, bannerItem := lm.findBannerInHand(player)
	if bannerSlot < 0 {
		return
	}

	dyeSlot, dyeColor := lm.findDyeInHand(player, bannerSlot)
	if dyeSlot < 0 {
		return
	}

	// Special patterns (creeper, skull, etc.) require a banner pattern item
	if isSpecialPattern(patternName) {
		if lm.findPatternItem(player, patternName, bannerSlot, dyeSlot) < 0 {
			return
		}
	}

	existingPatterns := bannerItem.BannerPatterns
	if len(existingPatterns) >= MaxBannerLayers {
		return
	}

	outputPatterns := make([]game.BannerLayer, len(existingPatterns)+1)
	copy(outputPatterns, existingPatterns)
	outputPatterns[len(existingPatterns)] = game.BannerLayer{
		Pattern: "minecraft:" + patternName,
		Color:   dyeColor,
	}

	bannerItemID := bannerItem.ID
	consumeOneItem(player, bannerSlot)
	consumeOneItem(player, dyeSlot)

	lm.giveOutputBanner(player, bannerItemID, outputPatterns)
}

// findBannerInHand looks for a banner item in the player's held slot or offhand.
// Returns the slot index and item pointer, or -1 if not found.
func (lm *LoomManager) findBannerInHand(player *game.Player) (int, *game.ItemStack) {
	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]
	if held.ID > 0 && held.Count > 0 && isBannerItem(ItemNameByID(held.ID)) {
		return heldSlot, held
	}

	offhand := &player.Inventory[45]
	if offhand.ID > 0 && offhand.Count > 0 && isBannerItem(ItemNameByID(offhand.ID)) {
		return 45, offhand
	}

	return -1, nil
}

// findDyeInHand looks for a dye item in the player's hands, excluding the given slot.
// Returns the slot index and the color name, or -1 if not found.
func (lm *LoomManager) findDyeInHand(player *game.Player, excludeSlot int) (int, string) {
	if excludeSlot != 45 {
		offhand := &player.Inventory[45]
		if offhand.ID > 0 && offhand.Count > 0 {
			if color, ok := dyeNameToColor[ItemNameByID(offhand.ID)]; ok {
				return 45, color
			}
		}
	}

	heldSlot := int(player.HeldSlot) + 36
	if heldSlot != excludeSlot {
		held := &player.Inventory[heldSlot]
		if held.ID > 0 && held.Count > 0 {
			if color, ok := dyeNameToColor[ItemNameByID(held.ID)]; ok {
				return heldSlot, color
			}
		}
	}

	for i := 36; i <= 44; i++ {
		if i == excludeSlot {
			continue
		}
		item := &player.Inventory[i]
		if item.ID > 0 && item.Count > 0 {
			if color, ok := dyeNameToColor[ItemNameByID(item.ID)]; ok {
				return i, color
			}
		}
	}

	return -1, ""
}

// findPatternItem looks for the banner pattern item that unlocks the given pattern.
// Returns the slot index or -1 if not found.
func (lm *LoomManager) findPatternItem(player *game.Player, patternName string, excludeSlots ...int) int {
	excluded := make(map[int]bool, len(excludeSlots))
	for _, s := range excludeSlots {
		excluded[s] = true
	}

	targetItem := ""
	for itemName, pName := range specialPatternItems {
		if pName == patternName {
			targetItem = itemName
			break
		}
	}
	if targetItem == "" {
		return -1
	}

	if !excluded[45] {
		offhand := &player.Inventory[45]
		if offhand.ID > 0 && offhand.Count > 0 && ItemNameByID(offhand.ID) == targetItem {
			return 45
		}
	}

	heldSlot := int(player.HeldSlot) + 36
	if !excluded[heldSlot] {
		held := &player.Inventory[heldSlot]
		if held.ID > 0 && held.Count > 0 && ItemNameByID(held.ID) == targetItem {
			return heldSlot
		}
	}

	for i := 9; i <= 44; i++ {
		if excluded[i] {
			continue
		}
		item := &player.Inventory[i]
		if item.ID > 0 && item.Count > 0 && ItemNameByID(item.ID) == targetItem {
			return i
		}
	}

	return -1
}

// isSpecialPattern returns true if the pattern requires a banner pattern item.
func isSpecialPattern(patternName string) bool {
	for _, pName := range specialPatternItems {
		if pName == patternName {
			return true
		}
	}
	return false
}

// giveOutputBanner adds a banner with the given patterns to the player's inventory.
func (lm *LoomManager) giveOutputBanner(player *game.Player, bannerItemID int32, patterns []game.BannerLayer) {
	addSlot := player.Inventory.AddItem(bannerItemID, 1)
	if addSlot >= 0 {
		player.Inventory[addSlot].BannerPatterns = make([]game.BannerLayer, len(patterns))
		copy(player.Inventory[addSlot].BannerPatterns, patterns)
		SendSlotUpdate(player, addSlot)
	}
}

// isBannerItem returns true if the item name is a banner (not a banner_pattern item).
func isBannerItem(name string) bool {
	return strings.HasSuffix(name, "_banner") && !strings.Contains(name, "banner_pattern")
}
