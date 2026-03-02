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

	// Interactive blocks
	"oak_door":           3.0,
	"spruce_door":        3.0,
	"birch_door":         3.0,
	"jungle_door":        3.0,
	"acacia_door":        3.0,
	"dark_oak_door":      3.0,
	"cherry_door":        3.0,
	"mangrove_door":      3.0,
	"bamboo_door":        3.0,
	"crimson_door":       3.0,
	"warped_door":        3.0,
	"iron_door":          5.0,
	"lever":              0.5,
	"stone_button":       0.5,
	"oak_button":         0.5,
	"spruce_button":      0.5,
	"birch_button":       0.5,
	"jungle_button":      0.5,
	"acacia_button":      0.5,
	"dark_oak_button":    0.5,

	// Chest, furnace, brewing stand, bed
	"chest":              2.5,
	"furnace":            3.5,
	"brewing_stand":      0.5,
	"red_bed":            0.2,

	// Redstone components
	"redstone_wire":      0,
	"repeater":           0,
	"comparator":         0,
	"piston":             1.5,
	"sticky_piston":      1.5,
	"piston_head":        1.5,
	"hopper":             3.0,
	"dispenser":          3.5,
	"dropper":            3.5,
	"observer":           3.5,
	"redstone_torch":     0,
	"redstone_wall_torch": 0,

	// TNT and fire
	"tnt":                0,
	"fire":               0,
	"soul_fire":          0,

	// Unbreakable
	"bedrock":            -1,
	"end_portal_frame":   -1,
	"barrier":            -1,
	"command_block":      -1,

	// Crops (instant break)
	"wheat":              0,
	"carrots":            0,
	"potatoes":           0,
	"beetroots":          0,
	"pumpkin_stem":        0,
	"melon_stem":          0,
	"attached_pumpkin_stem": 0,
	"attached_melon_stem":   0,
	"sugar_cane":          0,
	"nether_wart":         0,
	"cocoa":               0.2,
	"sweet_berry_bush":    0,
	"bamboo":              1.0,
	"bamboo_sapling":      0,

	// Fruit blocks
	"pumpkin":             1.0,
	"melon":               1.0,

	// Trapdoors
	"oak_trapdoor":       3.0,
	"spruce_trapdoor":    3.0,
	"birch_trapdoor":     3.0,
	"jungle_trapdoor":    3.0,
	"acacia_trapdoor":    3.0,
	"cherry_trapdoor":    3.0,
	"dark_oak_trapdoor":  3.0,
	"mangrove_trapdoor":  3.0,
	"bamboo_trapdoor":    3.0,
	"crimson_trapdoor":   3.0,
	"warped_trapdoor":    3.0,
	"iron_trapdoor":      5.0,

	// Fence gates
	"oak_fence_gate":       2.0,
	"spruce_fence_gate":    2.0,
	"birch_fence_gate":     2.0,
	"jungle_fence_gate":    2.0,
	"acacia_fence_gate":    2.0,
	"cherry_fence_gate":    2.0,
	"dark_oak_fence_gate":  2.0,
	"mangrove_fence_gate":  2.0,
	"bamboo_fence_gate":    2.0,
	"crimson_fence_gate":   2.0,
	"warped_fence_gate":    2.0,

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

// GetWeaponCooldown returns the attack cooldown period in seconds for a weapon.
// Vanilla values: sword=0.625, axe=1.0, shovel=1.0, pickaxe=0.833,
// hoe=varies by tier, other/hand=0.25.
func GetWeaponCooldown(itemName string) float64 {
	if c, ok := weaponCooldown[itemName]; ok {
		return c
	}
	return 0.25 // bare hand / non-weapon
}

var weaponCooldown = map[string]float64{
	// Swords: 0.625s (1.6 attacks/s)
	"wooden_sword": 0.625, "stone_sword": 0.625, "iron_sword": 0.625,
	"golden_sword": 0.625, "diamond_sword": 0.625, "netherite_sword": 0.625,
	// Axes: 1.0s (1.0 attacks/s)
	"wooden_axe": 1.0, "stone_axe": 1.0, "iron_axe": 1.0,
	"golden_axe": 1.0, "diamond_axe": 1.0, "netherite_axe": 1.0,
	// Shovels: 1.0s
	"wooden_shovel": 1.0, "stone_shovel": 1.0, "iron_shovel": 1.0,
	"golden_shovel": 1.0, "diamond_shovel": 1.0, "netherite_shovel": 1.0,
	// Pickaxes: 0.833s (1.2 attacks/s)
	"wooden_pickaxe": 0.833, "stone_pickaxe": 0.833, "iron_pickaxe": 0.833,
	"golden_pickaxe": 0.833, "diamond_pickaxe": 0.833, "netherite_pickaxe": 0.833,
	// Hoes: varies by tier
	"wooden_hoe": 1.0, "stone_hoe": 0.5, "iron_hoe": 0.333,
	"golden_hoe": 1.0, "diamond_hoe": 0.25, "netherite_hoe": 0.25,
	// Trident: 0.9s (1.1 attacks/s)
	"trident": 0.9,
}

// CanHarvestBlock returns true if the held item can harvest the block (gets drops).
// Blocks with a minimum tier requirement need a pickaxe of at least that tier.
func CanHarvestBlock(blockName, heldItemName string) bool {
	minTier, needsTier := minTierForDrop[blockName]
	if !needsTier {
		// Check if it needs the correct tool type at all
		pref, needsTool := preferredTool[blockName]
		if !needsTool {
			return true // no tool requirement
		}
		tool := GetToolInfo(heldItemName)
		if tool == nil || tool.Type != pref {
			return false // wrong tool or bare hand
		}
		return true
	}
	// Needs a pickaxe of at least minTier
	tool := GetToolInfo(heldItemName)
	if tool == nil || tool.Type != ToolPickaxe {
		return false
	}
	return tierLevel(tool.Tier) >= minTier
}

// tierLevel returns a numeric level for tier comparison.
func tierLevel(t ToolTier) int {
	switch t {
	case TierWood, TierGold:
		return 0
	case TierStone:
		return 1
	case TierIron:
		return 2
	case TierDiamond:
		return 3
	case TierNetherite:
		return 4
	}
	return 0
}

// GetBlockDropItemName returns the item name that a block drops.
// Returns ("", false) if the block should drop nothing.
func GetBlockDropItemName(blockName string) (string, bool) {
	if drop, ok := blockDropOverrides[blockName]; ok {
		if drop == "" {
			return "", false // drops nothing
		}
		return drop, true
	}
	return blockName, true // drops itself
}

// minTierForDrop maps blocks to minimum pickaxe tier level needed to get drops.
// Level: 0=wood/gold, 1=stone, 2=iron, 3=diamond, 4=netherite.
var minTierForDrop = map[string]int{
	// Any pickaxe (tier 0)
	"stone": 0, "cobblestone": 0, "mossy_cobblestone": 0,
	"granite": 0, "diorite": 0, "andesite": 0,
	"stone_bricks": 0, "deepslate": 0, "cobbled_deepslate": 0,
	"sandstone": 0, "terracotta": 0, "bricks": 0,
	"coal_ore": 0, "copper_ore": 0,
	"iron_block": 0, "gold_block": 0,
	// Stone+ (tier 1)
	"iron_ore": 1, "lapis_ore": 1,
	// Iron+ (tier 2)
	"gold_ore": 2, "diamond_ore": 2, "emerald_ore": 2, "redstone_ore": 2,
	"diamond_block": 2,
	// Diamond+ (tier 3)
	"obsidian": 3, "crying_obsidian": 3, "netherite_block": 3,
}

// blockDropOverrides maps blocks to the item they drop instead of themselves.
// An empty string means the block drops nothing.
var blockDropOverrides = map[string]string{
	"stone":        "cobblestone",
	"coal_ore":     "coal",
	"diamond_ore":  "diamond",
	"iron_ore":     "raw_iron",
	"gold_ore":     "raw_gold",
	"copper_ore":   "raw_copper",
	"lapis_ore":    "lapis_lazuli",
	"redstone_ore": "redstone",
	"emerald_ore":  "emerald",
	"grass_block":  "dirt",
	"grass":        "", // tall grass drops nothing
	"tall_grass":   "", // tall grass drops nothing
	"fire":         "", // fire drops nothing
	"soul_fire":    "", // soul fire drops nothing
	// Crops are handled separately by dropCropItems
	"wheat":                "",
	"carrots":              "",
	"potatoes":             "",
	"beetroots":            "",
	"pumpkin_stem":         "",
	"melon_stem":           "",
	"attached_pumpkin_stem": "",
	"attached_melon_stem":  "",
	"sugar_cane":           "",
	"nether_wart":          "",
	"cocoa":                "",
	"sweet_berry_bush":     "",
	"bamboo":               "",
	"bamboo_sapling":       "",
	// Melon is handled separately in dropBlockItem (3-7 melon_slices)
	"melon":                "",
}

// ArmorType identifies an armor piece slot.
type ArmorType int

const (
	ArmorHelmet ArmorType = iota
	ArmorChestplate
	ArmorLeggings
	ArmorBoots
)

// ArmorInfo describes an armor item's type and material.
type ArmorInfo struct {
	Type     ArmorType
	Material string // "leather", "iron", "gold", "diamond", "netherite"
}

// GetArmorInfo returns the ArmorInfo for an item name, or nil if not armor.
func GetArmorInfo(itemName string) *ArmorInfo {
	if info, ok := armorItems[itemName]; ok {
		return &info
	}
	return nil
}

// IsArmor returns true if the item is an armor piece.
func IsArmor(itemName string) bool {
	_, ok := armorItems[itemName]
	return ok
}

// ArmorSlotFor returns the inventory slot index for an armor type.
// Helmet=5, Chestplate=6, Leggings=7, Boots=8.
func ArmorSlotFor(armorType ArmorType) int {
	switch armorType {
	case ArmorHelmet:
		return 5
	case ArmorChestplate:
		return 6
	case ArmorLeggings:
		return 7
	case ArmorBoots:
		return 8
	}
	return -1
}

// GetArmorProtection returns the defense points for an armor item name.
func GetArmorProtection(itemName string) int {
	if p, ok := armorProtection[itemName]; ok {
		return p
	}
	return 0
}

// GetArmorDurability returns the max durability for an armor item name, or 0 if not armor.
func GetArmorDurability(itemName string) int32 {
	if d, ok := armorMaxDurability[itemName]; ok {
		return d
	}
	return 0
}

// GetKnockbackResistance returns the knockback resistance for an armor item.
// Each netherite armor piece provides 0.1 (10%) knockback resistance.
// Returns 0 for non-netherite or non-armor items.
func GetKnockbackResistance(itemName string) float64 {
	info := GetArmorInfo(itemName)
	if info != nil && info.Material == "netherite" {
		return 0.1
	}
	return 0
}

var armorItems = map[string]ArmorInfo{
	"leather_helmet":      {ArmorHelmet, "leather"},
	"leather_chestplate":  {ArmorChestplate, "leather"},
	"leather_leggings":    {ArmorLeggings, "leather"},
	"leather_boots":       {ArmorBoots, "leather"},
	"golden_helmet":       {ArmorHelmet, "gold"},
	"golden_chestplate":   {ArmorChestplate, "gold"},
	"golden_leggings":     {ArmorLeggings, "gold"},
	"golden_boots":        {ArmorBoots, "gold"},
	"iron_helmet":         {ArmorHelmet, "iron"},
	"iron_chestplate":     {ArmorChestplate, "iron"},
	"iron_leggings":       {ArmorLeggings, "iron"},
	"iron_boots":          {ArmorBoots, "iron"},
	"diamond_helmet":      {ArmorHelmet, "diamond"},
	"diamond_chestplate":  {ArmorChestplate, "diamond"},
	"diamond_leggings":    {ArmorLeggings, "diamond"},
	"diamond_boots":       {ArmorBoots, "diamond"},
	"netherite_helmet":      {ArmorHelmet, "netherite"},
	"netherite_chestplate":  {ArmorChestplate, "netherite"},
	"netherite_leggings":    {ArmorLeggings, "netherite"},
	"netherite_boots":       {ArmorBoots, "netherite"},
}

var armorProtection = map[string]int{
	"leather_helmet": 1, "leather_chestplate": 3, "leather_leggings": 2, "leather_boots": 1,
	"golden_helmet": 2, "golden_chestplate": 5, "golden_leggings": 3, "golden_boots": 1,
	"iron_helmet": 2, "iron_chestplate": 6, "iron_leggings": 5, "iron_boots": 2,
	"diamond_helmet": 3, "diamond_chestplate": 8, "diamond_leggings": 6, "diamond_boots": 3,
	"netherite_helmet": 3, "netherite_chestplate": 8, "netherite_leggings": 6, "netherite_boots": 3,
}

var armorMaxDurability = map[string]int32{
	"leather_helmet": 55, "leather_chestplate": 80, "leather_leggings": 75, "leather_boots": 65,
	"golden_helmet": 77, "golden_chestplate": 112, "golden_leggings": 105, "golden_boots": 91,
	"iron_helmet": 165, "iron_chestplate": 240, "iron_leggings": 225, "iron_boots": 195,
	"diamond_helmet": 363, "diamond_chestplate": 528, "diamond_leggings": 495, "diamond_boots": 429,
	"netherite_helmet": 407, "netherite_chestplate": 592, "netherite_leggings": 555, "netherite_boots": 481,
}

// shieldMaxDurability is the max durability of a shield.
const shieldMaxDurability int32 = 336

// bowMaxDurability is the max durability of a bow (vanilla = 384).
const bowMaxDurability int32 = 384

// bucketActions maps bucket items to their fluid type or "pickup" for empty buckets.
var bucketActions = map[string]string{
	"water_bucket": "water",
	"lava_bucket":  "lava",
	"bucket":       "pickup",
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

	// Trident: 9 base damage
	"trident": 9,
}
