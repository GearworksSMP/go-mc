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
}

// pressurePlateName returns the pressure plate item name for a plank type.
func pressurePlateName(plank string) string {
	// e.g. "oak_planks" → "oak_pressure_plate"
	// Strip "_planks" suffix and add "_pressure_plate"
	base := plank[:len(plank)-len("_planks")]
	return base + "_pressure_plate"
}

var allRecipes []Recipe
