package handler

import "strings"

// ToolType identifies a tool category.
type ToolType int

const (
	ToolNone ToolType = iota
	ToolPickaxe
	ToolAxe
	ToolShovel
	ToolHoe
	ToolSword
)

// ToolTier identifies a tool material tier.
type ToolTier int

const (
	TierWood ToolTier = iota
	TierGold
	TierStone
	TierIron
	TierDiamond
	TierNetherite
)

// ToolInfo describes a tool item's type and tier.
type ToolInfo struct {
	Type ToolType
	Tier ToolTier
}

// GetToolInfo returns the ToolInfo for an item name (without minecraft: prefix),
// or nil if the item is not a tool.
func GetToolInfo(itemName string) *ToolInfo {
	if info, ok := toolItems[itemName]; ok {
		return &info
	}
	return nil
}

// GetMaxDurability returns the max durability for a tool tier.
func GetMaxDurability(tier ToolTier) int32 {
	return maxDurability[tier]
}

// GetMaxDurabilityByItem returns the max durability for a tool item name,
// or 0 if the item is not a tool.
func GetMaxDurabilityByItem(itemName string) int32 {
	info := GetToolInfo(itemName)
	if info == nil {
		return 0
	}
	return maxDurability[info.Tier]
}

// CalculateBreakTime returns the expected break time in seconds for a block,
// given the held item name. Returns 0 for instant-break or unknown blocks.
func CalculateBreakTime(blockName, heldItemName string) float64 {
	hardness, ok := blockHardness[blockName]
	if !ok {
		return 0 // unknown block = allow instant break
	}
	if hardness < 0 {
		return -1 // unbreakable (bedrock, etc.)
	}
	if hardness == 0 {
		return 0 // instant break
	}

	speedMul := 1.0
	canHarvest := true

	tool := GetToolInfo(heldItemName)
	pref, needsTool := preferredTool[blockName]

	if tool != nil && pref == tool.Type {
		speedMul = tierSpeed[tool.Tier]
	}

	if needsTool && (tool == nil || pref != tool.Type) {
		canHarvest = false
	}

	var damage float64
	if canHarvest {
		damage = speedMul / hardness / 30
	} else {
		damage = speedMul / hardness / 100
	}

	if damage >= 1 {
		return 0 // instant break
	}

	return 1.0 / damage / 20.0 // ticks → seconds
}

var blockHardness = map[string]float64{
	// Stone variants
	"stone":              1.5,
	"granite":            1.5,
	"diorite":            1.5,
	"andesite":           1.5,
	"cobblestone":        2.0,
	"mossy_cobblestone":  2.0,
	"stone_bricks":       1.5,
	"deepslate":          3.0,
	"cobbled_deepslate":  3.5,

	// Ores
	"coal_ore":           3.0,
	"iron_ore":           3.0,
	"gold_ore":           3.0,
	"diamond_ore":        3.0,
	"lapis_ore":          3.0,
	"redstone_ore":       3.0,
	"emerald_ore":        3.0,
	"copper_ore":         3.0,

	// Earth
	"dirt":               0.5,
	"grass_block":        0.6,
	"sand":               0.5,
	"gravel":             0.6,
	"clay":               0.6,
	"farmland":           0.6,
	"dirt_path":          0.65,
	"soul_sand":          0.5,
	"soul_soil":          0.5,
	"mycelium":           0.6,
	"podzol":             0.5,
	"mud":                0.5,
	"snow_block":         0.2,

	// Wood
	"oak_log":            2.0,
	"spruce_log":         2.0,
	"birch_log":          2.0,
	"jungle_log":         2.0,
	"acacia_log":         2.0,
	"dark_oak_log":       2.0,
	"cherry_log":         2.0,
	"mangrove_log":       2.0,
	"oak_planks":         2.0,
	"spruce_planks":      2.0,
	"birch_planks":       2.0,
	"jungle_planks":      2.0,
	"acacia_planks":      2.0,
	"dark_oak_planks":    2.0,
	"cherry_planks":      2.0,
	"mangrove_planks":    2.0,
	"crafting_table":     2.5,

	// Hard blocks
	"obsidian":           50.0,
	"crying_obsidian":    50.0,
	"iron_block":         5.0,
	"gold_block":         3.0,
	"diamond_block":      5.0,
	"netherite_block":    50.0,
	"anvil":              5.0,
	"enchanting_table":   5.0,

	// Misc
	"bookshelf":          1.5,
	"glass":              0.3,
	"ice":                0.5,
	"packed_ice":         0.5,
	"blue_ice":           2.8,
	"glowstone":          0.3,
	"terracotta":         1.25,
	"bricks":             2.0,
	"sandstone":          0.8,

	// Unbreakable
	"bedrock":            -1,
	"end_portal_frame":   -1,
	"barrier":            -1,
	"command_block":      -1,

	// Leaves and plants
	"oak_leaves":         0.2,
	"spruce_leaves":      0.2,
	"birch_leaves":       0.2,
	"jungle_leaves":      0.2,
	"acacia_leaves":      0.2,
	"dark_oak_leaves":    0.2,
}

// preferredTool maps block names to their preferred tool type.
// Blocks in this map require the correct tool for drops.
var preferredTool = map[string]ToolType{
	// Pickaxe blocks
	"stone":              ToolPickaxe,
	"granite":            ToolPickaxe,
	"diorite":            ToolPickaxe,
	"andesite":           ToolPickaxe,
	"cobblestone":        ToolPickaxe,
	"mossy_cobblestone":  ToolPickaxe,
	"stone_bricks":       ToolPickaxe,
	"deepslate":          ToolPickaxe,
	"cobbled_deepslate":  ToolPickaxe,
	"coal_ore":           ToolPickaxe,
	"iron_ore":           ToolPickaxe,
	"gold_ore":           ToolPickaxe,
	"diamond_ore":        ToolPickaxe,
	"lapis_ore":          ToolPickaxe,
	"redstone_ore":       ToolPickaxe,
	"emerald_ore":        ToolPickaxe,
	"copper_ore":         ToolPickaxe,
	"obsidian":           ToolPickaxe,
	"crying_obsidian":    ToolPickaxe,
	"iron_block":         ToolPickaxe,
	"gold_block":         ToolPickaxe,
	"diamond_block":      ToolPickaxe,
	"netherite_block":    ToolPickaxe,
	"anvil":              ToolPickaxe,
	"enchanting_table":   ToolPickaxe,
	"bricks":             ToolPickaxe,
	"sandstone":          ToolPickaxe,
	"terracotta":         ToolPickaxe,
	"glowstone":          ToolPickaxe,

	// Axe blocks
	"oak_log":            ToolAxe,
	"spruce_log":         ToolAxe,
	"birch_log":          ToolAxe,
	"jungle_log":         ToolAxe,
	"acacia_log":         ToolAxe,
	"dark_oak_log":       ToolAxe,
	"cherry_log":         ToolAxe,
	"mangrove_log":       ToolAxe,
	"oak_planks":         ToolAxe,
	"spruce_planks":      ToolAxe,
	"birch_planks":       ToolAxe,
	"jungle_planks":      ToolAxe,
	"acacia_planks":      ToolAxe,
	"dark_oak_planks":    ToolAxe,
	"cherry_planks":      ToolAxe,
	"mangrove_planks":    ToolAxe,
	"crafting_table":     ToolAxe,
	"bookshelf":          ToolAxe,

	// Shovel blocks
	"dirt":               ToolShovel,
	"grass_block":        ToolShovel,
	"sand":               ToolShovel,
	"gravel":             ToolShovel,
	"clay":               ToolShovel,
	"farmland":           ToolShovel,
	"dirt_path":          ToolShovel,
	"soul_sand":          ToolShovel,
	"soul_soil":          ToolShovel,
	"mycelium":           ToolShovel,
	"podzol":             ToolShovel,
	"mud":                ToolShovel,
	"snow_block":         ToolShovel,

	// Hoe blocks
	"oak_leaves":         ToolHoe,
	"spruce_leaves":      ToolHoe,
	"birch_leaves":       ToolHoe,
	"jungle_leaves":      ToolHoe,
	"acacia_leaves":      ToolHoe,
	"dark_oak_leaves":    ToolHoe,
}

var tierSpeed = map[ToolTier]float64{
	TierWood:      2,
	TierGold:      12,
	TierStone:     4,
	TierIron:      6,
	TierDiamond:   8,
	TierNetherite: 9,
}

var maxDurability = map[ToolTier]int32{
	TierWood:      59,
	TierGold:      32,
	TierStone:     131,
	TierIron:      250,
	TierDiamond:   1561,
	TierNetherite: 2031,
}

var toolItems = map[string]ToolInfo{
	// Pickaxes
	"wooden_pickaxe":    {ToolPickaxe, TierWood},
	"stone_pickaxe":     {ToolPickaxe, TierStone},
	"iron_pickaxe":      {ToolPickaxe, TierIron},
	"golden_pickaxe":    {ToolPickaxe, TierGold},
	"diamond_pickaxe":   {ToolPickaxe, TierDiamond},
	"netherite_pickaxe": {ToolPickaxe, TierNetherite},

	// Axes
	"wooden_axe":    {ToolAxe, TierWood},
	"stone_axe":     {ToolAxe, TierStone},
	"iron_axe":      {ToolAxe, TierIron},
	"golden_axe":    {ToolAxe, TierGold},
	"diamond_axe":   {ToolAxe, TierDiamond},
	"netherite_axe": {ToolAxe, TierNetherite},

	// Shovels
	"wooden_shovel":    {ToolShovel, TierWood},
	"stone_shovel":     {ToolShovel, TierStone},
	"iron_shovel":      {ToolShovel, TierIron},
	"golden_shovel":    {ToolShovel, TierGold},
	"diamond_shovel":   {ToolShovel, TierDiamond},
	"netherite_shovel": {ToolShovel, TierNetherite},

	// Hoes
	"wooden_hoe":    {ToolHoe, TierWood},
	"stone_hoe":     {ToolHoe, TierStone},
	"iron_hoe":      {ToolHoe, TierIron},
	"golden_hoe":    {ToolHoe, TierGold},
	"diamond_hoe":   {ToolHoe, TierDiamond},
	"netherite_hoe": {ToolHoe, TierNetherite},

	// Swords
	"wooden_sword":    {ToolSword, TierWood},
	"stone_sword":     {ToolSword, TierStone},
	"iron_sword":      {ToolSword, TierIron},
	"golden_sword":    {ToolSword, TierGold},
	"diamond_sword":   {ToolSword, TierDiamond},
	"netherite_sword": {ToolSword, TierNetherite},
}

// IsSword returns true if the item is a sword.
func IsSword(itemName string) bool {
	return strings.HasSuffix(itemName, "_sword")
}

// GetWeaponDamage returns the attack damage for a weapon item name.
// Returns 1.0 (bare hand) for non-weapon items.
func GetWeaponDamage(itemName string) float32 {
	if d, ok := weaponDamage[itemName]; ok {
		return d
	}
	return 1.0
}

var weaponDamage = map[string]float32{
	// Swords: wood=4, stone=5, iron=6, gold=4, diamond=7, netherite=8
	"wooden_sword":    4,
	"stone_sword":     5,
	"iron_sword":      6,
	"golden_sword":    4,
	"diamond_sword":   7,
	"netherite_sword": 8,

	// Axes: wood=7, stone=9, iron=9, gold=7, diamond=9, netherite=10
	"wooden_axe":    7,
	"stone_axe":     9,
	"iron_axe":      9,
	"golden_axe":    7,
	"diamond_axe":   9,
	"netherite_axe": 10,
}
