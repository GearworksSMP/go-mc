package handler

import (
	"github.com/Tnze/go-mc/data/item"
)

// Recipe describes a crafting recipe that works in both 2x2 and 3x3 grids.
type Recipe struct {
	Shaped      bool
	Width       int      // shaped only: pattern width
	Height      int      // shaped only: pattern height
	Pattern     []string // shaped: row-major item names, "" = empty (Width*Height entries)
	Inputs      []string // shapeless: required item names (unordered)
	ResultName  string
	ResultCount int32
}

// MatchRecipe checks a crafting grid and returns the result item ID and count.
// grid contains item IDs in row-major order, gridWidth is 2 or 3.
// Returns (0, 0) if no recipe matches.
func MatchRecipe(grid []int32, gridWidth int) (resultID, count int32) {
	gridHeight := len(grid) / gridWidth

	// Convert grid IDs to item names
	names := make([]string, len(grid))
	for i, id := range grid {
		if id > 0 {
			if it, ok := item.ByID[item.ID(id)]; ok {
				names[i] = it.Name
			}
		}
	}

	for _, r := range allRecipes {
		if r.Shaped {
			if r.Width > gridWidth || r.Height > gridHeight {
				continue
			}
			if matchShapedGrid(names, gridWidth, gridHeight, r) {
				return itemIDByName(r.ResultName), r.ResultCount
			}
		} else {
			if matchShapelessGrid(names, r.Inputs) {
				return itemIDByName(r.ResultName), r.ResultCount
			}
		}
	}
	return 0, 0
}

// matchShapedGrid tries all valid offsets (dx, dy) where the pattern fits,
// including horizontal mirror.
func matchShapedGrid(grid []string, gridW, gridH int, r Recipe) bool {
	for dy := 0; dy <= gridH-r.Height; dy++ {
		for dx := 0; dx <= gridW-r.Width; dx++ {
			if matchAtOffset(grid, gridW, gridH, r, dx, dy, false) {
				return true
			}
			if matchAtOffset(grid, gridW, gridH, r, dx, dy, true) {
				return true
			}
		}
	}
	return false
}

// matchAtOffset checks if the pattern matches at offset (dx, dy), optionally mirrored.
func matchAtOffset(grid []string, gridW, gridH int, r Recipe, dx, dy int, mirror bool) bool {
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			gridItem := grid[gy*gridW+gx]

			// Check if this grid cell is within the pattern area
			py := gy - dy
			px := gx - dx
			if py >= 0 && py < r.Height && px >= 0 && px < r.Width {
				patX := px
				if mirror {
					patX = r.Width - 1 - px
				}
				expected := r.Pattern[py*r.Width+patX]
				if gridItem != expected {
					return false
				}
			} else {
				// Outside pattern — must be empty
				if gridItem != "" {
					return false
				}
			}
		}
	}
	return true
}

// matchShapelessGrid checks if the grid contains exactly the required inputs (unordered).
func matchShapelessGrid(grid []string, inputs []string) bool {
	var gridItems []string
	for _, name := range grid {
		if name != "" {
			gridItems = append(gridItems, name)
		}
	}
	if len(gridItems) != len(inputs) {
		return false
	}
	used := make([]bool, len(inputs))
	for _, g := range gridItems {
		found := false
		for j, inp := range inputs {
			if !used[j] && g == inp {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func itemIDByName(name string) int32 {
	for id, it := range item.ByID {
		if it.Name == name {
			return int32(id)
		}
	}
	return 0
}

// Helper constructors

func shapeless1(input, result string, count int32) Recipe {
	return Recipe{Inputs: []string{input}, ResultName: result, ResultCount: count}
}

func shapeless2(a, b, result string, count int32) Recipe {
	return Recipe{Inputs: []string{a, b}, ResultName: result, ResultCount: count}
}

func shaped(w, h int, pattern []string, result string, count int32) Recipe {
	return Recipe{Shaped: true, Width: w, Height: h, Pattern: pattern, ResultName: result, ResultCount: count}
}

// Tool recipe helpers
func toolRecipe(pattern []string, w, h int, material, result string) Recipe {
	expanded := make([]string, len(pattern))
	for i, p := range pattern {
		switch p {
		case "M":
			expanded[i] = material
		case "S":
			expanded[i] = "stick"
		default:
			expanded[i] = ""
		}
	}
	return shaped(w, h, expanded, result, 1)
}

var plankTypes = []string{
	"oak_planks", "spruce_planks", "birch_planks", "jungle_planks",
	"acacia_planks", "dark_oak_planks", "cherry_planks", "mangrove_planks",
}

var logPairs = [][2]string{
	{"oak_log", "oak_planks"}, {"spruce_log", "spruce_planks"},
	{"birch_log", "birch_planks"}, {"jungle_log", "jungle_planks"},
	{"acacia_log", "acacia_planks"}, {"dark_oak_log", "dark_oak_planks"},
	{"cherry_log", "cherry_planks"}, {"mangrove_log", "mangrove_planks"},
	{"stripped_oak_log", "oak_planks"}, {"stripped_spruce_log", "spruce_planks"},
	{"stripped_birch_log", "birch_planks"}, {"stripped_jungle_log", "jungle_planks"},
	{"stripped_acacia_log", "acacia_planks"}, {"stripped_dark_oak_log", "dark_oak_planks"},
	{"stripped_cherry_log", "cherry_planks"}, {"stripped_mangrove_log", "mangrove_planks"},
}

var buttonPairs = [][2]string{
	{"oak_planks", "oak_button"}, {"spruce_planks", "spruce_button"},
	{"birch_planks", "birch_button"}, {"jungle_planks", "jungle_button"},
	{"acacia_planks", "acacia_button"}, {"dark_oak_planks", "dark_oak_button"},
	{"cherry_planks", "cherry_button"}, {"mangrove_planks", "mangrove_button"},
}

// Tool tiers: material → tool name prefix
var toolTiers = []struct {
	material string
	prefix   string
}{
	{"oak_planks", "wooden"},
	{"cobblestone", "stone"},
	{"iron_ingot", "iron"},
	{"gold_ingot", "golden"},
	{"diamond", "diamond"},
}

func init() {
	// Logs → Planks
	for _, lp := range logPairs {
		allRecipes = append(allRecipes, shapeless1(lp[0], lp[1], 4))
	}

	// Sticks: 2 planks vertically (1x2)
	for _, p := range plankTypes {
		allRecipes = append(allRecipes, shaped(1, 2, []string{p, p}, "stick", 4))
	}

	// Crafting table: 4 planks in 2x2
	for _, p := range plankTypes {
		allRecipes = append(allRecipes, shaped(2, 2, []string{p, p, p, p}, "crafting_table", 1))
	}

	// Buttons: 1 plank → 4 buttons
	for _, bp := range buttonPairs {
		allRecipes = append(allRecipes, shapeless1(bp[0], bp[1], 4))
	}

	// Stone button
	allRecipes = append(allRecipes, shapeless1("stone", "stone_button", 1))

	// Torches: coal + stick (1x2 shaped)
	allRecipes = append(allRecipes, shaped(1, 2, []string{"coal", "stick"}, "torch", 4))
	allRecipes = append(allRecipes, shaped(1, 2, []string{"charcoal", "stick"}, "torch", 4))

	// Pressure plates: 2 planks horizontal (2x1)
	for _, p := range plankTypes {
		allRecipes = append(allRecipes, shaped(2, 1, []string{p, p}, pressurePlateName(p), 1))
	}

	// Stone pressure plate
	allRecipes = append(allRecipes, shaped(2, 1, []string{"stone", "stone"}, "stone_pressure_plate", 1))

	// 3x3 Tool recipes
	for _, tier := range toolTiers {
		m := tier.material
		pfx := tier.prefix
		// Pickaxe: MMM / _S_ / _S_
		allRecipes = append(allRecipes, toolRecipe([]string{"M", "M", "M", "", "S", "", "", "S", ""}, 3, 3, m, pfx+"_pickaxe"))
		// Axe: MM_ / MS_ / _S_ (mirror handles the other variant)
		allRecipes = append(allRecipes, toolRecipe([]string{"M", "M", "", "M", "S", "", "", "S", ""}, 3, 3, m, pfx+"_axe"))
		// Shovel: _M_ / _S_ / _S_
		allRecipes = append(allRecipes, toolRecipe([]string{"M", "S", "S"}, 1, 3, m, pfx+"_shovel"))
		// Hoe: MM_ / _S_ / _S_ (mirror handles the other variant)
		allRecipes = append(allRecipes, toolRecipe([]string{"M", "M", "", "S", "", "S"}, 2, 3, m, pfx+"_hoe"))
		// Sword: _M_ / _M_ / _S_
		allRecipes = append(allRecipes, toolRecipe([]string{"M", "M", "S"}, 1, 3, m, pfx+"_sword"))
	}

	// Furnace: 8 cobblestone ring
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"cobblestone", "cobblestone", "cobblestone",
		"cobblestone", "", "cobblestone",
		"cobblestone", "cobblestone", "cobblestone",
	}, "furnace", 1))

	// Chest: 8 planks ring (use oak_planks; each plank type gets one)
	for _, p := range plankTypes {
		allRecipes = append(allRecipes, shaped(3, 3, []string{
			p, p, p,
			p, "", p,
			p, p, p,
		}, "chest", 1))
	}

	// Bowl: planks V-shape (3x2)
	for _, p := range plankTypes {
		allRecipes = append(allRecipes, shaped(3, 2, []string{
			p, "", p,
			"", p, "",
		}, "bowl", 4))
	}

	// Bucket: iron_ingot V-shape (3x2)
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"iron_ingot", "", "iron_ingot",
		"", "iron_ingot", "",
	}, "bucket", 1))

	// -----------------------------------------------------------------------
	// Armor recipes (4 tiers × 4 pieces)
	// -----------------------------------------------------------------------
	armorMaterials := []struct {
		material string
		prefix   string
	}{
		{"leather", "leather"},
		{"iron_ingot", "iron"},
		{"gold_ingot", "golden"},
		{"diamond", "diamond"},
	}
	for _, am := range armorMaterials {
		m := am.material
		pfx := am.prefix
		// Helmet: MMM / M_M (3x2)
		allRecipes = append(allRecipes, shaped(3, 2, []string{m, m, m, m, "", m}, pfx+"_helmet", 1))
		// Chestplate: M_M / MMM / MMM (3x3)
		allRecipes = append(allRecipes, shaped(3, 3, []string{m, "", m, m, m, m, m, m, m}, pfx+"_chestplate", 1))
		// Leggings: MMM / M_M / M_M (3x3)
		allRecipes = append(allRecipes, shaped(3, 3, []string{m, m, m, m, "", m, m, "", m}, pfx+"_leggings", 1))
		// Boots: M_M / M_M (3x2)
		allRecipes = append(allRecipes, shaped(3, 2, []string{m, "", m, m, "", m}, pfx+"_boots", 1))
	}

	// -----------------------------------------------------------------------
	// Wood products (per plank type)
	// -----------------------------------------------------------------------
	for _, p := range plankTypes {
		base := p[:len(p)-len("_planks")]
		// Door: MM / MM / MM (2x3) → 3
		allRecipes = append(allRecipes, shaped(2, 3, []string{p, p, p, p, p, p}, base+"_door", 3))
		// Trapdoor: MMM / MMM (3x2) → 2
		allRecipes = append(allRecipes, shaped(3, 2, []string{p, p, p, p, p, p}, base+"_trapdoor", 2))
		// Fence: MSM / MSM (3x2) → 3
		allRecipes = append(allRecipes, shaped(3, 2, []string{p, "stick", p, p, "stick", p}, base+"_fence", 3))
		// Fence gate: SMS / SMS (3x2) → 1
		allRecipes = append(allRecipes, shaped(3, 2, []string{"stick", p, "stick", "stick", p, "stick"}, base+"_fence_gate", 1))
		// Sign: MMM / MMM / _S_ (3x3) → 3
		allRecipes = append(allRecipes, shaped(3, 3, []string{p, p, p, p, p, p, "", "stick", ""}, base+"_sign", 3))
		// Slab: MMM (3x1) → 6
		allRecipes = append(allRecipes, shaped(3, 1, []string{p, p, p}, base+"_slab", 6))
		// Stairs: M__ / MM_ / MMM (3x3) → 4
		allRecipes = append(allRecipes, shaped(3, 3, []string{p, "", "", p, p, "", p, p, p}, base+"_stairs", 4))
		// Boat: M_M / MMM (3x2) → 1
		allRecipes = append(allRecipes, shaped(3, 2, []string{p, "", p, p, p, p}, base+"_boat", 1))
	}

	// -----------------------------------------------------------------------
	// Stone products
	// -----------------------------------------------------------------------
	// Stone slab: SSS (3x1) → 6
	allRecipes = append(allRecipes, shaped(3, 1, []string{"cobblestone", "cobblestone", "cobblestone"}, "cobblestone_slab", 6))
	// Stone stairs: S__ / SS_ / SSS (3x3) → 4
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"cobblestone", "", "",
		"cobblestone", "cobblestone", "",
		"cobblestone", "cobblestone", "cobblestone",
	}, "cobblestone_stairs", 4))

	// -----------------------------------------------------------------------
	// Miscellaneous recipes
	// -----------------------------------------------------------------------
	// Ladder: S_S / SSS / S_S (3x3) → 3
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stick", "", "stick",
		"stick", "stick", "stick",
		"stick", "", "stick",
	}, "ladder", 3))

	// Bed: WWW / PPP (3x2) → 1 (white wool + oak planks)
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"white_wool", "white_wool", "white_wool",
		"oak_planks", "oak_planks", "oak_planks",
	}, "white_bed", 1))

	// Iron bars: III / III (3x2) → 16
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"iron_ingot", "iron_ingot", "iron_ingot",
		"iron_ingot", "iron_ingot", "iron_ingot",
	}, "iron_bars", 16))

	// Glass pane: GGG / GGG (3x2) → 16
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"glass", "glass", "glass",
		"glass", "glass", "glass",
	}, "glass_pane", 16))

	// Compass: _I_ / IRI / _I_ (3x3) → 1
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "iron_ingot", "",
		"iron_ingot", "redstone", "iron_ingot",
		"", "iron_ingot", "",
	}, "compass", 1))

	// Clock: _G_ / GRG / _G_ (3x3) → 1
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "gold_ingot", "",
		"gold_ingot", "redstone", "gold_ingot",
		"", "gold_ingot", "",
	}, "clock", 1))

	// Shield: PIP / PPP / _P_ (3x3) → 1
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "iron_ingot", "oak_planks",
		"oak_planks", "oak_planks", "oak_planks",
		"", "oak_planks", "",
	}, "shield", 1))

	// Arrow: F / S / E (1x3) → 4
	allRecipes = append(allRecipes, shaped(1, 3, []string{"flint", "stick", "feather"}, "arrow", 4))

	// Bread: WWW (3x1) → 1
	allRecipes = append(allRecipes, shaped(3, 1, []string{"wheat", "wheat", "wheat"}, "bread", 1))

	// Paper: SSS (3x1) → 3
	allRecipes = append(allRecipes, shaped(3, 1, []string{"sugar_cane", "sugar_cane", "sugar_cane"}, "paper", 3))

	// Book: shapeless(paper, paper, paper, leather) → 1
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"paper", "paper", "paper", "leather"},
		ResultName:  "book",
		ResultCount: 1,
	})

	// -----------------------------------------------------------------------
	// Crop/food recipes
	// -----------------------------------------------------------------------
	// Sugar cane → Sugar (shapeless 1:1)
	allRecipes = append(allRecipes, shapeless1("sugar_cane", "sugar", 1))

	// Pumpkin pie: shapeless(pumpkin, sugar, egg) → 1
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"pumpkin", "sugar", "egg"},
		ResultName:  "pumpkin_pie",
		ResultCount: 1,
	})

	// Cookie: wheat + cocoa_beans + wheat (3x1) → 8
	allRecipes = append(allRecipes, shaped(3, 1, []string{"wheat", "cocoa_beans", "wheat"}, "cookie", 8))

	// Melon block: 9 melon slices (3x3) → 1
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"melon_slice", "melon_slice", "melon_slice",
		"melon_slice", "melon_slice", "melon_slice",
		"melon_slice", "melon_slice", "melon_slice",
	}, "melon", 1))

	// Cake: MMM / SES / WWW (3x3) where M=milk_bucket, S=sugar, E=egg, W=wheat
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"milk_bucket", "milk_bucket", "milk_bucket",
		"sugar", "egg", "sugar",
		"wheat", "wheat", "wheat",
	}, "cake", 1))

	// -----------------------------------------------------------------------
	// Missing utility recipes
	// -----------------------------------------------------------------------
	// String ×4 → Wool (2x2)
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"string", "string",
		"string", "string",
	}, "white_wool", 1))

	// Flint and steel: shapeless(iron_ingot, flint) → 1
	allRecipes = append(allRecipes, shapeless2("iron_ingot", "flint", "flint_and_steel", 1))

	// Golden apple: GGG / GAG / GGG (3x3) where G=gold_ingot, A=apple
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"gold_ingot", "gold_ingot", "gold_ingot",
		"gold_ingot", "apple", "gold_ingot",
		"gold_ingot", "gold_ingot", "gold_ingot",
	}, "golden_apple", 1))

	// Eye of ender: shapeless(blaze_powder, ender_pearl) → 1
	allRecipes = append(allRecipes, shapeless2("blaze_powder", "ender_pearl", "ender_eye", 1))

	// TNT: G S G / S G S / G S G (3x3) where G=gunpowder, S=sand
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"gunpowder", "sand", "gunpowder",
		"sand", "gunpowder", "sand",
		"gunpowder", "sand", "gunpowder",
	}, "tnt", 1))

	// Redstone torch: redstone + stick (1x2)
	allRecipes = append(allRecipes, shaped(1, 2, []string{"redstone", "stick"}, "redstone_torch", 1))

	// Beacon: GGG / GNG / OOO (3x3) where G=glass, N=nether_star, O=obsidian
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"glass", "glass", "glass",
		"glass", "nether_star", "glass",
		"obsidian", "obsidian", "obsidian",
	}, "beacon", 1))

	// -----------------------------------------------------------------------
	// Rail/minecart recipes
	// -----------------------------------------------------------------------
	// Rail: I_I / ISI / I_I (3x3) where I=iron_ingot, S=stick → 16
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_ingot", "", "iron_ingot",
		"iron_ingot", "stick", "iron_ingot",
		"iron_ingot", "", "iron_ingot",
	}, "rail", 16))

	// Minecart: I_I / III (3x2) where I=iron_ingot → 1
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"iron_ingot", "", "iron_ingot",
		"iron_ingot", "iron_ingot", "iron_ingot",
	}, "minecart", 1))

	// -----------------------------------------------------------------------
	// Missing block recipes
	// -----------------------------------------------------------------------
	// Stone bricks: SSSS (2x2) where S=stone → 4
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"stone", "stone",
		"stone", "stone",
	}, "stone_bricks", 4))

	// -----------------------------------------------------------------------
	// Storage block recipes (compress 9 → 1 block + decompress 1 → 9)
	// -----------------------------------------------------------------------
	storageBlocks := [][2]string{
		{"iron_ingot", "iron_block"}, {"gold_ingot", "gold_block"},
		{"diamond", "diamond_block"}, {"emerald", "emerald_block"},
		{"lapis_lazuli", "lapis_block"}, {"redstone", "redstone_block"},
		{"coal", "coal_block"}, {"copper_ingot", "copper_block"},
		{"raw_iron", "raw_iron_block"}, {"raw_gold", "raw_gold_block"},
		{"raw_copper", "raw_copper_block"}, {"netherite_ingot", "netherite_block"},
		{"slime_ball", "slime_block"}, {"dried_kelp", "dried_kelp_block"},
		{"wheat", "hay_block"}, {"bone_meal", "bone_block"},
	}
	for _, sb := range storageBlocks {
		allRecipes = append(allRecipes, shaped(3, 3, []string{
			sb[0], sb[0], sb[0],
			sb[0], sb[0], sb[0],
			sb[0], sb[0], sb[0],
		}, sb[1], 1))
		allRecipes = append(allRecipes, shapeless1(sb[1], sb[0], 9))
	}
	// Ingot/nugget conversions
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_nugget", "iron_nugget", "iron_nugget",
		"iron_nugget", "iron_nugget", "iron_nugget",
		"iron_nugget", "iron_nugget", "iron_nugget",
	}, "iron_ingot", 1))
	allRecipes = append(allRecipes, shapeless1("iron_ingot", "iron_nugget", 9))
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"gold_nugget", "gold_nugget", "gold_nugget",
		"gold_nugget", "gold_nugget", "gold_nugget",
		"gold_nugget", "gold_nugget", "gold_nugget",
	}, "gold_ingot", 1))
	allRecipes = append(allRecipes, shapeless1("gold_ingot", "gold_nugget", 9))

	// -----------------------------------------------------------------------
	// Redstone component recipes
	// -----------------------------------------------------------------------
	// Repeater: RTR / SSS
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"redstone_torch", "redstone", "redstone_torch",
		"stone", "stone", "stone",
	}, "repeater", 1))
	// Comparator: _T_ / TQT / SSS
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "redstone_torch", "",
		"redstone_torch", "quartz", "redstone_torch",
		"stone", "stone", "stone",
	}, "comparator", 1))
	// Observer: CCC / RRQ / CCC
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"cobblestone", "cobblestone", "cobblestone",
		"redstone", "redstone", "quartz",
		"cobblestone", "cobblestone", "cobblestone",
	}, "observer", 1))
	// Piston: PPP / CIC / CRC
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "oak_planks", "oak_planks",
		"cobblestone", "iron_ingot", "cobblestone",
		"cobblestone", "redstone", "cobblestone",
	}, "piston", 1))
	// Sticky piston: slime_ball + piston (1x2)
	allRecipes = append(allRecipes, shaped(1, 2, []string{"slime_ball", "piston"}, "sticky_piston", 1))
	// Hopper: I_I / ICI / _I_
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_ingot", "", "iron_ingot",
		"iron_ingot", "chest", "iron_ingot",
		"", "iron_ingot", "",
	}, "hopper", 1))
	// Dropper: CCC / C_C / CRC
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"cobblestone", "cobblestone", "cobblestone",
		"cobblestone", "", "cobblestone",
		"cobblestone", "redstone", "cobblestone",
	}, "dropper", 1))
	// Dispenser: CCC / CBC / CRC
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"cobblestone", "cobblestone", "cobblestone",
		"cobblestone", "bow", "cobblestone",
		"cobblestone", "redstone", "cobblestone",
	}, "dispenser", 1))
	// Note block: PPP / PRP / PPP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "oak_planks", "oak_planks",
		"oak_planks", "redstone", "oak_planks",
		"oak_planks", "oak_planks", "oak_planks",
	}, "note_block", 1))
	// Redstone lamp: _R_ / RGR / _R_
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "redstone", "",
		"redstone", "glowstone", "redstone",
		"", "redstone", "",
	}, "redstone_lamp", 1))
	// Tripwire hook: I / S / P (1x3)
	allRecipes = append(allRecipes, shaped(1, 3, []string{"iron_ingot", "stick", "oak_planks"}, "tripwire_hook", 2))
	// Lever: stick + cobblestone (1x2)
	allRecipes = append(allRecipes, shaped(1, 2, []string{"stick", "cobblestone"}, "lever", 1))
	// Trapped chest: tripwire_hook + chest
	allRecipes = append(allRecipes, shapeless2("tripwire_hook", "chest", "trapped_chest", 1))
	// Daylight detector: GGG / QQQ / SSS
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"glass", "glass", "glass",
		"quartz", "quartz", "quartz",
		"oak_slab", "oak_slab", "oak_slab",
	}, "daylight_detector", 1))
	// Target: R_R / _H_ / R_R (redstone + hay_block)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "redstone", "",
		"redstone", "hay_block", "redstone",
		"", "redstone", "",
	}, "target", 1))
	// Weighted pressure plates
	allRecipes = append(allRecipes, shaped(2, 1, []string{"iron_ingot", "iron_ingot"}, "heavy_weighted_pressure_plate", 1))
	allRecipes = append(allRecipes, shaped(2, 1, []string{"gold_ingot", "gold_ingot"}, "light_weighted_pressure_plate", 1))

	// -----------------------------------------------------------------------
	// Workstation & utility recipes
	// -----------------------------------------------------------------------
	// Enchanting table: _B_ / DOD / OOO
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "book", "",
		"diamond", "obsidian", "diamond",
		"obsidian", "obsidian", "obsidian",
	}, "enchanting_table", 1))
	// Anvil: BBB / _I_ / III
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_block", "iron_block", "iron_block",
		"", "iron_ingot", "",
		"iron_ingot", "iron_ingot", "iron_ingot",
	}, "anvil", 1))
	// Brewing stand: _B_ / CCC
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"", "blaze_rod", "",
		"cobblestone", "cobblestone", "cobblestone",
	}, "brewing_stand", 1))
	// Blast furnace: III / IFI / SSS
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_ingot", "iron_ingot", "iron_ingot",
		"iron_ingot", "furnace", "iron_ingot",
		"smooth_stone", "smooth_stone", "smooth_stone",
	}, "blast_furnace", 1))
	// Smoker: _L_ / LFL / _L_
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "oak_log", "",
		"oak_log", "furnace", "oak_log",
		"", "oak_log", "",
	}, "smoker", 1))
	// Grindstone: SSS / P_P
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"stick", "stone_slab", "stick",
		"oak_planks", "", "oak_planks",
	}, "grindstone", 1))
	// Stonecutter: _I_ / SSS
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"", "iron_ingot", "",
		"stone", "stone", "stone",
	}, "stonecutter", 1))
	// Smithing table: II / PP / PP
	allRecipes = append(allRecipes, shaped(2, 3, []string{
		"iron_ingot", "iron_ingot",
		"oak_planks", "oak_planks",
		"oak_planks", "oak_planks",
	}, "smithing_table", 1))
	// Campfire: _S_ / SCS / LLL
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "stick", "",
		"stick", "coal", "stick",
		"oak_log", "oak_log", "oak_log",
	}, "campfire", 1))
	// Soul campfire
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "stick", "",
		"stick", "soul_sand", "stick",
		"oak_log", "oak_log", "oak_log",
	}, "soul_campfire", 1))
	// Lantern: III / ICI / III (chain + torches — actually: NNN / NTN / NNN where N=iron_nugget, T=torch)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_nugget", "iron_nugget", "iron_nugget",
		"iron_nugget", "torch", "iron_nugget",
		"iron_nugget", "iron_nugget", "iron_nugget",
	}, "lantern", 1))
	// Soul lantern
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_nugget", "iron_nugget", "iron_nugget",
		"iron_nugget", "soul_torch", "iron_nugget",
		"iron_nugget", "iron_nugget", "iron_nugget",
	}, "soul_lantern", 1))
	// Soul torch: coal + stick + soul_sand (1x3) — actually: C / S where C is coal on soul_sand not quite
	// Vanilla: shapeless(coal/charcoal, stick, soul_sand) but actually it's shaped: C / S / SS
	// Actually: shaped 1x2 coal+stick = torch, soul torch = shaped(1,3, [charcoal, stick, soul_sand])
	// Let's just do: coal_or_charcoal + stick + soul_sand/soul_soil
	allRecipes = append(allRecipes, shaped(1, 3, []string{"coal", "stick", "soul_sand"}, "soul_torch", 4))
	allRecipes = append(allRecipes, shaped(1, 3, []string{"charcoal", "stick", "soul_sand"}, "soul_torch", 4))
	allRecipes = append(allRecipes, shaped(1, 3, []string{"coal", "stick", "soul_soil"}, "soul_torch", 4))
	// Chain: N / I / N (1x3)
	allRecipes = append(allRecipes, shaped(1, 3, []string{"iron_nugget", "iron_ingot", "iron_nugget"}, "chain", 1))
	// Lightning rod: CCC (1x3) where C=copper_ingot
	allRecipes = append(allRecipes, shaped(1, 3, []string{"copper_ingot", "copper_ingot", "copper_ingot"}, "lightning_rod", 1))
	// Spyglass: ACA (1x3) where A=amethyst_shard, C=copper_ingot — actually _A_ / _C_ / _C_
	allRecipes = append(allRecipes, shaped(1, 3, []string{"amethyst_shard", "copper_ingot", "copper_ingot"}, "spyglass", 1))
	// Cauldron: I_I / I_I / III
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"iron_ingot", "", "iron_ingot",
		"iron_ingot", "", "iron_ingot",
		"iron_ingot", "iron_ingot", "iron_ingot",
	}, "cauldron", 1))
	// Composter: P_P / P_P / PPP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_slab", "", "oak_slab",
		"oak_slab", "", "oak_slab",
		"oak_slab", "oak_slab", "oak_slab",
	}, "composter", 1))
	// Lectern: SSS / _B_ / _S_  (slab on top, bookshelf in middle, slab at bottom)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_slab", "oak_slab", "oak_slab",
		"", "bookshelf", "",
		"", "oak_slab", "",
	}, "lectern", 1))
	// Loom: SS / PP (2x2)
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"string", "string",
		"oak_planks", "oak_planks",
	}, "loom", 1))
	// Cartography table: PP / PP / __ (paper on top, planks below — actually: PP / _P / _P with paper+plank)
	allRecipes = append(allRecipes, shaped(2, 3, []string{
		"paper", "paper",
		"oak_planks", "oak_planks",
		"oak_planks", "oak_planks",
	}, "cartography_table", 1))
	// Fletching table: FF / PP / PP
	allRecipes = append(allRecipes, shaped(2, 3, []string{
		"flint", "flint",
		"oak_planks", "oak_planks",
		"oak_planks", "oak_planks",
	}, "fletching_table", 1))
	// Barrel: PSP / P_P / PSP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "oak_slab", "oak_planks",
		"oak_planks", "", "oak_planks",
		"oak_planks", "oak_slab", "oak_planks",
	}, "barrel", 1))

	// -----------------------------------------------------------------------
	// Decoration & misc
	// -----------------------------------------------------------------------
	// Bookshelf: PPP / BBB / PPP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "oak_planks", "oak_planks",
		"book", "book", "book",
		"oak_planks", "oak_planks", "oak_planks",
	}, "bookshelf", 1))
	// Painting: SSS / SWS / SSS
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stick", "stick", "stick",
		"stick", "white_wool", "stick",
		"stick", "stick", "stick",
	}, "painting", 1))
	// Item frame: SSS / SLS / SSS
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stick", "stick", "stick",
		"stick", "leather", "stick",
		"stick", "stick", "stick",
	}, "item_frame", 1))
	// Armor stand: S_S / _S_ / SAS (stick + stone_slab base)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stick", "", "stick",
		"", "stick", "",
		"stick", "smooth_stone_slab", "stick",
	}, "armor_stand", 1))
	// Jukebox: PPP / PDP / PPP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"oak_planks", "oak_planks", "oak_planks",
		"oak_planks", "diamond", "oak_planks",
		"oak_planks", "oak_planks", "oak_planks",
	}, "jukebox", 1))
	// Flower pot: B_B / _B_ (brick)
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"brick", "", "brick",
		"", "brick", "",
	}, "flower_pot", 1))
	// Glass bottle: G_G / _G_ (3x2)
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"glass", "", "glass",
		"", "glass", "",
	}, "glass_bottle", 3))
	// Tinted glass: _A_ / AGA / _A_
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "amethyst_shard", "",
		"amethyst_shard", "glass", "amethyst_shard",
		"", "amethyst_shard", "",
	}, "tinted_glass", 2))
	// Lead: SS_ / SR_ / __S (string + slime_ball)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"string", "string", "",
		"string", "slime_ball", "",
		"", "", "string",
	}, "lead", 2))
	// Map: PPP / PCP / PPP
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"paper", "paper", "paper",
		"paper", "compass", "paper",
		"paper", "paper", "paper",
	}, "map", 1))
	// Bone meal from bone
	allRecipes = append(allRecipes, shapeless1("bone", "bone_meal", 3))
	// Brick block from bricks
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"brick", "brick",
		"brick", "brick",
	}, "bricks", 1))
	// Nether brick block
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"nether_brick", "nether_brick",
		"nether_brick", "nether_brick",
	}, "nether_bricks", 1))
	// Quartz block
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"quartz", "quartz",
		"quartz", "quartz",
	}, "quartz_block", 1))
	// Blaze powder from blaze rod
	allRecipes = append(allRecipes, shapeless1("blaze_rod", "blaze_powder", 2))
	// Magma cream
	allRecipes = append(allRecipes, shapeless2("blaze_powder", "slime_ball", "magma_cream", 1))
	// Fire charge: blaze_powder + coal + gunpowder
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"blaze_powder", "coal", "gunpowder"},
		ResultName:  "fire_charge",
		ResultCount: 3,
	})
	// Ender chest: OEO / OEO / OOO — actually: OEO / OEO / OOO (obsidian + ender_eye)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"obsidian", "obsidian", "obsidian",
		"obsidian", "ender_eye", "obsidian",
		"obsidian", "obsidian", "obsidian",
	}, "ender_chest", 1))

	// -----------------------------------------------------------------------
	// Weapons & tools
	// -----------------------------------------------------------------------
	// Bow: _SM / S_M / _SM (stick + string — mirror handles other variant)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "stick", "string",
		"stick", "", "string",
		"", "stick", "string",
	}, "bow", 1))
	// Crossbow: SIS / THT / _S_ (stick, iron_ingot, tripwire_hook, string)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stick", "iron_ingot", "stick",
		"string", "tripwire_hook", "string",
		"", "stick", "",
	}, "crossbow", 1))
	// Fishing rod: __S / _SL / S_L (stick + string)
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"", "", "stick",
		"", "stick", "string",
		"stick", "", "string",
	}, "fishing_rod", 1))
	// Shears: _I / I_ (2x2)
	allRecipes = append(allRecipes, shaped(2, 2, []string{
		"", "iron_ingot",
		"iron_ingot", "",
	}, "shears", 1))

	// -----------------------------------------------------------------------
	// Food recipes
	// -----------------------------------------------------------------------
	// Mushroom stew: shapeless(red_mushroom, brown_mushroom, bowl)
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"red_mushroom", "brown_mushroom", "bowl"},
		ResultName:  "mushroom_stew",
		ResultCount: 1,
	})
	// Beetroot soup: shapeless(beetroot ×6, bowl) — actually shaped: BBB / BBB / _O_ where O=bowl
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"beetroot", "beetroot", "beetroot",
		"beetroot", "beetroot", "beetroot",
		"", "bowl", "",
	}, "beetroot_soup", 1))
	// Rabbit stew: shapeless(cooked_rabbit, carrot, baked_potato, brown_mushroom, bowl)
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"cooked_rabbit", "carrot", "baked_potato", "brown_mushroom", "bowl"},
		ResultName:  "rabbit_stew",
		ResultCount: 1,
	})
	// Golden carrot: GGG / GCG / GGG where G=gold_nugget, C=carrot
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"gold_nugget", "gold_nugget", "gold_nugget",
		"gold_nugget", "carrot", "gold_nugget",
		"gold_nugget", "gold_nugget", "gold_nugget",
	}, "golden_carrot", 1))
	// Glistering melon slice: GGG / GMG / GGG where G=gold_nugget, M=melon_slice
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"gold_nugget", "gold_nugget", "gold_nugget",
		"gold_nugget", "melon_slice", "gold_nugget",
		"gold_nugget", "gold_nugget", "gold_nugget",
	}, "glistering_melon_slice", 1))
	// Fermented spider eye: shapeless(spider_eye, sugar, brown_mushroom)
	allRecipes = append(allRecipes, Recipe{
		Inputs:      []string{"spider_eye", "sugar", "brown_mushroom"},
		ResultName:  "fermented_spider_eye",
		ResultCount: 1,
	})

	// -----------------------------------------------------------------------
	// Dye & colored blocks
	// -----------------------------------------------------------------------
	dyeColors := []struct{ dye, color string }{
		{"white_dye", "white"}, {"orange_dye", "orange"}, {"magenta_dye", "magenta"},
		{"light_blue_dye", "light_blue"}, {"yellow_dye", "yellow"}, {"lime_dye", "lime"},
		{"pink_dye", "pink"}, {"gray_dye", "gray"}, {"light_gray_dye", "light_gray"},
		{"cyan_dye", "cyan"}, {"purple_dye", "purple"}, {"blue_dye", "blue"},
		{"brown_dye", "brown"}, {"green_dye", "green"}, {"red_dye", "red"},
		{"black_dye", "black"},
	}
	for _, dc := range dyeColors {
		// Dye + white wool → colored wool
		allRecipes = append(allRecipes, shapeless2(dc.dye, "white_wool", dc.color+"_wool", 1))
		// Dye + white bed → colored bed
		allRecipes = append(allRecipes, shapeless2(dc.dye, "white_bed", dc.color+"_bed", 1))
		// Dye + glass → stained glass (8 around dye)
		allRecipes = append(allRecipes, shaped(3, 3, []string{
			"glass", "glass", "glass",
			"glass", dc.dye, "glass",
			"glass", "glass", "glass",
		}, dc.color+"_stained_glass", 8))
		// Dye + terracotta → colored terracotta (8 around dye)
		allRecipes = append(allRecipes, shaped(3, 3, []string{
			"terracotta", "terracotta", "terracotta",
			"terracotta", dc.dye, "terracotta",
			"terracotta", "terracotta", "terracotta",
		}, dc.color+"_terracotta", 8))
		// Concrete powder: dye + 4 sand + 4 gravel
		allRecipes = append(allRecipes, Recipe{
			Inputs:      []string{dc.dye, "sand", "sand", "sand", "sand", "gravel", "gravel", "gravel", "gravel"},
			ResultName:  dc.color + "_concrete_powder",
			ResultCount: 8,
		})
	}
	// Common dye recipes
	allRecipes = append(allRecipes, shapeless1("bone_meal", "white_dye", 1))
	allRecipes = append(allRecipes, shapeless1("ink_sac", "black_dye", 1))
	allRecipes = append(allRecipes, shapeless1("cocoa_beans", "brown_dye", 1))
	allRecipes = append(allRecipes, shapeless1("lapis_lazuli", "blue_dye", 1))
	allRecipes = append(allRecipes, shapeless2("blue_dye", "white_dye", "light_blue_dye", 2))
	allRecipes = append(allRecipes, shapeless2("red_dye", "white_dye", "pink_dye", 2))
	allRecipes = append(allRecipes, shapeless2("green_dye", "white_dye", "lime_dye", 2))
	allRecipes = append(allRecipes, shapeless2("black_dye", "white_dye", "gray_dye", 2))
	allRecipes = append(allRecipes, shapeless2("gray_dye", "white_dye", "light_gray_dye", 2))
	allRecipes = append(allRecipes, shapeless2("blue_dye", "red_dye", "purple_dye", 2))
	allRecipes = append(allRecipes, shapeless2("red_dye", "yellow_dye", "orange_dye", 2))
	allRecipes = append(allRecipes, shapeless2("blue_dye", "green_dye", "cyan_dye", 2))
	allRecipes = append(allRecipes, shapeless2("purple_dye", "pink_dye", "magenta_dye", 2))

	// -----------------------------------------------------------------------
	// Additional stone/mineral recipes
	// -----------------------------------------------------------------------
	// Stone slab
	allRecipes = append(allRecipes, shaped(3, 1, []string{"stone", "stone", "stone"}, "stone_slab", 6))
	// Stone brick slab
	allRecipes = append(allRecipes, shaped(3, 1, []string{"stone_bricks", "stone_bricks", "stone_bricks"}, "stone_brick_slab", 6))
	// Stone brick stairs
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"stone_bricks", "", "",
		"stone_bricks", "stone_bricks", "",
		"stone_bricks", "stone_bricks", "stone_bricks",
	}, "stone_brick_stairs", 4))
	// Cobblestone wall
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"cobblestone", "cobblestone", "cobblestone",
		"cobblestone", "cobblestone", "cobblestone",
	}, "cobblestone_wall", 6))
	// Stone brick wall
	allRecipes = append(allRecipes, shaped(3, 2, []string{
		"stone_bricks", "stone_bricks", "stone_bricks",
		"stone_bricks", "stone_bricks", "stone_bricks",
	}, "stone_brick_wall", 6))
	// Smooth stone slab
	allRecipes = append(allRecipes, shaped(3, 1, []string{"smooth_stone", "smooth_stone", "smooth_stone"}, "smooth_stone_slab", 6))
	// Polished granite/diorite/andesite (2x2)
	allRecipes = append(allRecipes, shaped(2, 2, []string{"granite", "granite", "granite", "granite"}, "polished_granite", 4))
	allRecipes = append(allRecipes, shaped(2, 2, []string{"diorite", "diorite", "diorite", "diorite"}, "polished_diorite", 4))
	allRecipes = append(allRecipes, shaped(2, 2, []string{"andesite", "andesite", "andesite", "andesite"}, "polished_andesite", 4))
	// Polished deepslate
	allRecipes = append(allRecipes, shaped(2, 2, []string{"cobbled_deepslate", "cobbled_deepslate", "cobbled_deepslate", "cobbled_deepslate"}, "polished_deepslate", 4))
	// Cut copper
	allRecipes = append(allRecipes, shaped(2, 2, []string{"copper_block", "copper_block", "copper_block", "copper_block"}, "cut_copper", 4))
	// Sandstone
	allRecipes = append(allRecipes, shaped(2, 2, []string{"sand", "sand", "sand", "sand"}, "sandstone", 1))
	// Red sandstone
	allRecipes = append(allRecipes, shaped(2, 2, []string{"red_sand", "red_sand", "red_sand", "red_sand"}, "red_sandstone", 1))
	// Prismarine
	allRecipes = append(allRecipes, shaped(2, 2, []string{"prismarine_shard", "prismarine_shard", "prismarine_shard", "prismarine_shard"}, "prismarine", 1))
	// Prismarine bricks
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"prismarine_shard", "prismarine_shard", "prismarine_shard",
		"prismarine_shard", "prismarine_shard", "prismarine_shard",
		"prismarine_shard", "prismarine_shard", "prismarine_shard",
	}, "prismarine_bricks", 1))
	// Sea lantern
	allRecipes = append(allRecipes, shaped(3, 3, []string{
		"prismarine_shard", "prismarine_crystals", "prismarine_shard",
		"prismarine_crystals", "prismarine_crystals", "prismarine_crystals",
		"prismarine_shard", "prismarine_crystals", "prismarine_shard",
	}, "sea_lantern", 1))
	// End stone bricks
	allRecipes = append(allRecipes, shaped(2, 2, []string{"end_stone", "end_stone", "end_stone", "end_stone"}, "end_stone_bricks", 4))
}

// pressurePlateName returns the pressure plate item name for a plank type.
func pressurePlateName(plank string) string {
	// e.g. "oak_planks" → "oak_pressure_plate"
	// Strip "_planks" suffix and add "_pressure_plate"
	base := plank[:len(plank)-len("_planks")]
	return base + "_pressure_plate"
}

var allRecipes []Recipe
