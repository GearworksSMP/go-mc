package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// StonecutterRecipe maps an input item to possible outputs.
type StonecutterRecipe struct {
	Input   string
	Outputs []StonecutterOutput
}

// StonecutterOutput is a single stonecutter recipe result.
type StonecutterOutput struct {
	Name  string
	Count int32
}

// stonecutterRecipes defines all stonecutter recipes.
var stonecutterRecipes = []StonecutterRecipe{
	{"stone", []StonecutterOutput{
		{"stone_slab", 2}, {"stone_stairs", 1}, {"stone_bricks", 1},
		{"stone_brick_slab", 2}, {"stone_brick_stairs", 1}, {"stone_brick_wall", 1},
		{"chiseled_stone_bricks", 1},
	}},
	{"cobblestone", []StonecutterOutput{
		{"cobblestone_slab", 2}, {"cobblestone_stairs", 1}, {"cobblestone_wall", 1},
	}},
	{"granite", []StonecutterOutput{
		{"granite_slab", 2}, {"granite_stairs", 1}, {"granite_wall", 1},
		{"polished_granite", 1}, {"polished_granite_slab", 2}, {"polished_granite_stairs", 1},
	}},
	{"diorite", []StonecutterOutput{
		{"diorite_slab", 2}, {"diorite_stairs", 1}, {"diorite_wall", 1},
		{"polished_diorite", 1}, {"polished_diorite_slab", 2}, {"polished_diorite_stairs", 1},
	}},
	{"andesite", []StonecutterOutput{
		{"andesite_slab", 2}, {"andesite_stairs", 1}, {"andesite_wall", 1},
		{"polished_andesite", 1}, {"polished_andesite_slab", 2}, {"polished_andesite_stairs", 1},
	}},
	{"sandstone", []StonecutterOutput{
		{"sandstone_slab", 2}, {"sandstone_stairs", 1}, {"sandstone_wall", 1},
		{"cut_sandstone", 1}, {"cut_sandstone_slab", 2}, {"chiseled_sandstone", 1},
	}},
	{"red_sandstone", []StonecutterOutput{
		{"red_sandstone_slab", 2}, {"red_sandstone_stairs", 1}, {"red_sandstone_wall", 1},
		{"cut_red_sandstone", 1}, {"cut_red_sandstone_slab", 2}, {"chiseled_red_sandstone", 1},
	}},
	{"quartz_block", []StonecutterOutput{
		{"quartz_slab", 2}, {"quartz_stairs", 1}, {"quartz_bricks", 1},
		{"quartz_pillar", 1}, {"chiseled_quartz_block", 1},
	}},
	{"prismarine", []StonecutterOutput{
		{"prismarine_slab", 2}, {"prismarine_stairs", 1}, {"prismarine_wall", 1},
	}},
	{"prismarine_bricks", []StonecutterOutput{
		{"prismarine_brick_slab", 2}, {"prismarine_brick_stairs", 1},
	}},
	{"dark_prismarine", []StonecutterOutput{
		{"dark_prismarine_slab", 2}, {"dark_prismarine_stairs", 1},
	}},
	{"bricks", []StonecutterOutput{
		{"brick_slab", 2}, {"brick_stairs", 1}, {"brick_wall", 1},
	}},
	{"nether_bricks", []StonecutterOutput{
		{"nether_brick_slab", 2}, {"nether_brick_stairs", 1}, {"nether_brick_wall", 1},
		{"chiseled_nether_bricks", 1},
	}},
	{"blackstone", []StonecutterOutput{
		{"blackstone_slab", 2}, {"blackstone_stairs", 1}, {"blackstone_wall", 1},
		{"polished_blackstone", 1}, {"polished_blackstone_slab", 2},
		{"polished_blackstone_stairs", 1}, {"polished_blackstone_wall", 1},
		{"polished_blackstone_bricks", 1}, {"polished_blackstone_brick_slab", 2},
		{"polished_blackstone_brick_stairs", 1}, {"polished_blackstone_brick_wall", 1},
	}},
	{"deepslate", []StonecutterOutput{
		{"cobbled_deepslate_slab", 2}, {"cobbled_deepslate_stairs", 1},
		{"cobbled_deepslate_wall", 1}, {"polished_deepslate", 1},
		{"polished_deepslate_slab", 2}, {"polished_deepslate_stairs", 1},
		{"polished_deepslate_wall", 1}, {"deepslate_bricks", 1},
		{"deepslate_brick_slab", 2}, {"deepslate_brick_stairs", 1},
		{"deepslate_brick_wall", 1}, {"deepslate_tiles", 1},
		{"deepslate_tile_slab", 2}, {"deepslate_tile_stairs", 1},
		{"deepslate_tile_wall", 1}, {"chiseled_deepslate", 1},
	}},
	{"copper_block", []StonecutterOutput{
		{"cut_copper", 4}, {"cut_copper_slab", 8}, {"cut_copper_stairs", 4},
	}},
}

// StonecutterManager handles stonecutter interactions.
type StonecutterManager struct {
	World game.World
}

// NewStonecutterManager creates a new StonecutterManager.
func NewStonecutterManager(world game.World) *StonecutterManager {
	return &StonecutterManager{World: world}
}

// OpenStonecutter opens the stonecutter UI for a player.
// Menu type 13 = stonecutter.
func (sm *StonecutterManager) OpenStonecutter(player *game.Player, x, y, z int) {
	windowID := 14
	player.OpenWindowID = windowID

	title := chat.Text("Stonecutter")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(windowID),
		pk.VarInt(13), // menu type: stonecutter
		title,
	))
}

// HandleStonecutterSelect handles selecting a stonecutter recipe.
func (sm *StonecutterManager) HandleStonecutterSelect(player *game.Player, recipeIndex int) {
	if player.OpenWindowID != 14 {
		return
	}

	// Get input from held slot
	heldSlot := int(player.HeldSlot) + 36
	item := &player.Inventory[heldSlot]
	if item.ID <= 0 || item.Count <= 0 {
		return
	}
	inputName := ItemNameByID(item.ID)

	// Find matching recipes
	for _, recipe := range stonecutterRecipes {
		if recipe.Input != inputName {
			continue
		}
		if recipeIndex < 0 || recipeIndex >= len(recipe.Outputs) {
			return
		}
		output := recipe.Outputs[recipeIndex]
		outputID := itemIDByName(output.Name)
		if outputID <= 0 {
			return
		}

		// Consume 1 input
		item.Count--
		if item.Count <= 0 {
			*item = game.ItemStack{}
		}
		SendSlotUpdate(player, heldSlot)

		// Give output
		addSlot := player.Inventory.AddItem(outputID, output.Count)
		if addSlot >= 0 {
			SendSlotUpdate(player, addSlot)
		}
		return
	}
}
