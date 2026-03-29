package handler

import (
	"math/rand"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
)

// LootFunction identifies a post-processing function applied to a loot entry.
type LootFunction int

const (
	LootFuncNone           LootFunction = iota
	LootFuncEnchantRandomly             // apply a random enchantment at a random level
	LootFuncEnchantLevels               // apply enchantments as if from an enchanting table
	LootFuncSetDamage                   // set item durability to a random fraction
)

// LootEntry represents a possible item in a loot table.
type LootEntry struct {
	ItemName           string
	MinCount, MaxCount int32
	Weight             int
	Quality            int              // effective weight adjustment: effectiveWeight = Weight + Quality * luck
	Enchantments       map[string]int32 // optional fixed enchantments to apply

	// Post-processing function and its parameter.
	// For LootFuncEnchantLevels: FuncParam is the enchanting level.
	// For LootFuncSetDamage: FuncParam is max durability percentage (0-100).
	Function  LootFunction
	FuncParam int32
}

// LootPool is a group of entries rolled independently. Each pool has its own
// min/max roll count.
type LootPool struct {
	MinRolls, MaxRolls int
	Entries            []LootEntry
}

// LootTable defines a set of loot pools. Each pool is rolled independently,
// allowing a single chest to generate items from multiple independent groups
// (e.g., common items in one pool, rare items in another).
type LootTable struct {
	Pools []LootPool

	// Legacy single-pool fields kept for backward compatibility with existing
	// table definitions. If Pools is empty, a single pool is constructed from
	// these fields.
	MinItems, MaxItems int
	Entries            []LootEntry
}

// pools returns the effective pool list for this table.
func (lt *LootTable) pools() []LootPool {
	if len(lt.Pools) > 0 {
		return lt.Pools
	}
	if len(lt.Entries) > 0 {
		return []LootPool{{MinRolls: lt.MinItems, MaxRolls: lt.MaxItems, Entries: lt.Entries}}
	}
	return nil
}

// Loot tables for various structure types.
var lootTables = map[string]*LootTable{
	"dungeon": {
		MinItems: 2, MaxItems: 8,
		Entries: []LootEntry{
			{"string", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"gunpowder", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"wheat", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"bread", 1, 1, 10, 0, nil, LootFuncNone, 0},
			{"name_tag", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"saddle", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 4, 5, 0, nil, LootFuncNone, 0},
			{"redstone", 1, 4, 5, 0, nil, LootFuncNone, 0},
			{"coal", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"bone", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"rotten_flesh", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"music_disc_13", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"music_disc_cat", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"mineshaft": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"rail", 4, 8, 10, 0, nil, LootFuncNone, 0},
			{"torch", 1, 16, 10, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 5, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 3, 4, 0, nil, LootFuncNone, 0},
			{"lapis_lazuli", 4, 9, 5, 0, nil, LootFuncNone, 0},
			{"bread", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"melon_seeds", 2, 4, 5, 0, nil, LootFuncNone, 0},
			{"pumpkin_seeds", 2, 4, 5, 0, nil, LootFuncNone, 0},
			{"iron_pickaxe", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"name_tag", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"golden_apple", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"temple": {
		MinItems: 2, MaxItems: 7,
		Entries: []LootEntry{
			{"diamond", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"emerald", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 2, 7, 8, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 5, 8, 0, nil, LootFuncNone, 0},
			{"bone", 4, 6, 10, 0, nil, LootFuncNone, 0},
			{"rotten_flesh", 3, 7, 10, 0, nil, LootFuncNone, 0},
			{"spider_eye", 1, 3, 5, 0, nil, LootFuncNone, 0},
			{"enchanted_golden_apple", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"saddle", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"iron_horse_armor", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"golden_horse_armor", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"diamond_horse_armor", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"village": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"bread", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"apple", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"potato", 1, 7, 8, 0, nil, LootFuncNone, 0},
			{"carrot", 1, 7, 8, 0, nil, LootFuncNone, 0},
			{"wheat", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"oak_sapling", 1, 2, 5, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"emerald", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"coal", 1, 3, 5, 0, nil, LootFuncNone, 0},
			{"oak_planks", 1, 4, 10, 0, nil, LootFuncNone, 0},
		},
	},
	"stronghold": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"ender_pearl", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"diamond", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 5, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 3, 5, 0, nil, LootFuncNone, 0},
			{"book", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"apple", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"bread", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"iron_pickaxe", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"iron_sword", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"iron_boots", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"iron_chestplate", 1, 1, 2, 0, nil, LootFuncNone, 0},
		},
	},
	"nether_fortress": {
		MinItems: 2, MaxItems: 7,
		Entries: []LootEntry{
			{"gold_ingot", 1, 3, 10, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 5, 8, 0, nil, LootFuncNone, 0},
			{"nether_wart", 3, 7, 5, 0, nil, LootFuncNone, 0},
			{"saddle", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"golden_horse_armor", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"iron_horse_armor", 1, 1, 4, 0, nil, LootFuncNone, 0},
			{"diamond_horse_armor", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"diamond", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"flint_and_steel", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"obsidian", 2, 4, 2, 0, nil, LootFuncNone, 0},
			{"blaze_rod", 1, 2, 2, 0, nil, LootFuncNone, 0},
		},
	},
	"bastion": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"gold_ingot", 2, 8, 10, 0, nil, LootFuncNone, 0},
			{"gold_block", 1, 2, 3, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 6, 8, 0, nil, LootFuncNone, 0},
			{"crossbow", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"spectral_arrow", 4, 12, 5, 0, nil, LootFuncNone, 0},
			{"magma_cream", 2, 6, 5, 0, nil, LootFuncNone, 0},
			{"string", 4, 6, 8, 0, nil, LootFuncNone, 0},
			{"iron_sword", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"golden_sword", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"golden_axe", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"golden_boots", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"ancient_debris", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"end_city": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"diamond", 2, 7, 5, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 4, 8, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 2, 7, 8, 0, nil, LootFuncNone, 0},
			{"emerald", 2, 6, 5, 0, nil, LootFuncNone, 0},
			{"beetroot_seeds", 1, 10, 5, 0, nil, LootFuncNone, 0},
			{"diamond_sword", 1, 1, 2, 0, map[string]int32{"sharpness": 3}, LootFuncNone, 0},
			{"diamond_pickaxe", 1, 1, 2, 0, map[string]int32{"efficiency": 3}, LootFuncNone, 0},
			{"diamond_shovel", 1, 1, 2, 0, map[string]int32{"efficiency": 3}, LootFuncNone, 0},
			{"diamond_chestplate", 1, 1, 2, 0, map[string]int32{"protection": 3}, LootFuncNone, 0},
			{"diamond_boots", 1, 1, 2, 0, map[string]int32{"protection": 3, "feather_falling": 3}, LootFuncNone, 0},
			{"diamond_helmet", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"diamond_leggings", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"iron_leggings", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"iron_boots", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"elytra", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"buried_treasure": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"heart_of_the_sea", 1, 1, 10, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 4, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 4, 6, 0, nil, LootFuncNone, 0},
			{"tnt", 1, 2, 3, 0, nil, LootFuncNone, 0},
			{"emerald", 4, 8, 5, 0, nil, LootFuncNone, 0},
			{"diamond", 1, 2, 3, 0, nil, LootFuncNone, 0},
			{"cooked_cod", 2, 4, 5, 0, nil, LootFuncNone, 0},
			{"cooked_salmon", 2, 4, 5, 0, nil, LootFuncNone, 0},
			{"prismarine_crystals", 1, 5, 5, 0, nil, LootFuncNone, 0},
			{"iron_sword", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"leather_tunic", 1, 1, 2, 0, nil, LootFuncNone, 0},
		},
	},
	"shipwreck_treasure": {
		MinItems: 3, MaxItems: 6,
		Entries: []LootEntry{
			{"iron_ingot", 1, 5, 10, 0, nil, LootFuncNone, 0},
			{"iron_nugget", 1, 10, 8, 0, nil, LootFuncNone, 0},
			{"emerald", 1, 5, 5, 0, nil, LootFuncNone, 0},
			{"diamond", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"lapis_lazuli", 1, 10, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 5, 5, 0, nil, LootFuncNone, 0},
			{"gold_nugget", 1, 10, 8, 0, nil, LootFuncNone, 0},
		},
	},
	"shipwreck_supply": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"paper", 1, 12, 10, 0, nil, LootFuncNone, 0},
			{"wheat", 8, 21, 8, 0, nil, LootFuncNone, 0},
			{"carrot", 4, 8, 5, 0, nil, LootFuncNone, 0},
			{"potato", 2, 6, 5, 0, nil, LootFuncNone, 0},
			{"poisonous_potato", 2, 6, 3, 0, nil, LootFuncNone, 0},
			{"coal", 2, 8, 8, 0, nil, LootFuncNone, 0},
			{"tnt", 1, 2, 2, 0, nil, LootFuncNone, 0},
			{"gunpowder", 1, 5, 5, 0, nil, LootFuncNone, 0},
			{"enchanted_golden_apple", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"bamboo", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"pumpkin", 1, 3, 3, 0, nil, LootFuncNone, 0},
		},
	},
	"igloo": {
		MinItems: 2, MaxItems: 5,
		Entries: []LootEntry{
			{"golden_apple", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"coal", 1, 4, 10, 0, nil, LootFuncNone, 0},
			{"apple", 1, 3, 8, 0, nil, LootFuncNone, 0},
			{"wheat", 2, 3, 8, 0, nil, LootFuncNone, 0},
			{"stone_axe", 1, 1, 3, 0, nil, LootFuncNone, 0},
		},
	},
	"woodland_mansion": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"diamond", 1, 3, 3, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 4, 8, 0, nil, LootFuncNone, 0},
			{"gold_ingot", 1, 4, 5, 0, nil, LootFuncNone, 0},
			{"diamond_chestplate", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"diamond_sword", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"iron_sword", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"bread", 1, 4, 8, 0, nil, LootFuncNone, 0},
			{"bone", 1, 8, 10, 0, nil, LootFuncNone, 0},
			{"gunpowder", 1, 8, 10, 0, nil, LootFuncNone, 0},
			{"string", 1, 8, 10, 0, nil, LootFuncNone, 0},
			{"bucket", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"redstone", 1, 4, 5, 0, nil, LootFuncNone, 0},
			{"name_tag", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"lead", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"music_disc_13", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"music_disc_cat", 1, 1, 1, 0, nil, LootFuncNone, 0},
		},
	},
	"ruined_portal": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"obsidian", 1, 2, 10, 0, nil, LootFuncNone, 0},
			{"flint_and_steel", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"iron_nugget", 9, 18, 8, 0, nil, LootFuncNone, 0},
			{"gold_nugget", 4, 24, 8, 0, nil, LootFuncNone, 0},
			{"golden_sword", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"golden_axe", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"golden_apple", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"enchanted_golden_apple", 1, 1, 1, 0, nil, LootFuncNone, 0},
			{"fire_charge", 1, 1, 5, 0, nil, LootFuncNone, 0},
			{"golden_horse_armor", 1, 1, 2, 0, nil, LootFuncNone, 0},
			{"golden_boots", 1, 1, 2, 0, nil, LootFuncNone, 0},
		},
	},

	// --- New structure loot tables ---

	"ancient_city": {
		Pools: []LootPool{
			{ // Common items
				MinRolls: 3, MaxRolls: 6,
				Entries: []LootEntry{
					{"sculk_sensor", 1, 3, 8, 0, nil, LootFuncNone, 0},
					{"sculk_catalyst", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"sculk_shrieker", 1, 2, 3, 0, nil, LootFuncNone, 0},
					{"candle", 1, 4, 8, 0, nil, LootFuncNone, 0},
					{"bone", 1, 15, 10, 0, nil, LootFuncNone, 0},
					{"soul_torch", 1, 15, 10, 0, nil, LootFuncNone, 0},
					{"coal", 6, 15, 8, 0, nil, LootFuncNone, 0},
					{"amethyst_shard", 1, 15, 5, 0, nil, LootFuncNone, 0},
				},
			},
			{ // Rare items
				MinRolls: 1, MaxRolls: 3,
				Entries: []LootEntry{
					{"echo_shard", 1, 3, 5, 2, nil, LootFuncNone, 0},
					{"disc_fragment_5", 1, 3, 3, 1, nil, LootFuncNone, 0},
					{"music_disc_otherside", 1, 1, 1, 1, nil, LootFuncNone, 0},
					{"enchanted_golden_apple", 1, 1, 1, 0, nil, LootFuncNone, 0},
					{"diamond", 1, 3, 3, 1, nil, LootFuncNone, 0},
					{"diamond_hoe", 1, 1, 2, 1, nil, LootFuncEnchantLevels, 30},
					{"diamond_leggings", 1, 1, 2, 1, nil, LootFuncEnchantLevels, 30},
					{"book", 1, 1, 3, 2, nil, LootFuncEnchantLevels, 30},
					{"book", 1, 1, 1, 0, map[string]int32{"swift_sneak": 3}, LootFuncNone, 0},
				},
			},
		},
	},
	"trail_ruins": {
		Pools: []LootPool{
			{ // Common archaeology finds
				MinRolls: 2, MaxRolls: 5,
				Entries: []LootEntry{
					{"brick", 1, 4, 10, 0, nil, LootFuncNone, 0},
					{"emerald", 1, 3, 5, 0, nil, LootFuncNone, 0},
					{"wheat", 1, 3, 8, 0, nil, LootFuncNone, 0},
					{"coal", 1, 3, 8, 0, nil, LootFuncNone, 0},
					{"blue_dye", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"light_blue_dye", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"orange_dye", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"white_dye", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"yellow_dye", 1, 2, 5, 0, nil, LootFuncNone, 0},
					{"dead_bush", 1, 1, 3, 0, nil, LootFuncNone, 0},
				},
			},
			{ // Rare pottery sherds and items
				MinRolls: 1, MaxRolls: 2,
				Entries: []LootEntry{
					{"wayfinder_armor_trim_smithing_template", 1, 1, 2, 1, nil, LootFuncNone, 0},
					{"raiser_armor_trim_smithing_template", 1, 1, 2, 1, nil, LootFuncNone, 0},
					{"shaper_armor_trim_smithing_template", 1, 1, 2, 1, nil, LootFuncNone, 0},
					{"host_armor_trim_smithing_template", 1, 1, 2, 1, nil, LootFuncNone, 0},
					{"gold_ingot", 1, 1, 3, 0, nil, LootFuncNone, 0},
					{"music_disc_relic", 1, 1, 1, 1, nil, LootFuncNone, 0},
				},
			},
		},
	},
	"trial_chambers": {
		Pools: []LootPool{
			{ // Common trial chamber loot
				MinRolls: 2, MaxRolls: 5,
				Entries: []LootEntry{
					{"arrow", 2, 8, 10, 0, nil, LootFuncNone, 0},
					{"baked_potato", 1, 3, 8, 0, nil, LootFuncNone, 0},
					{"iron_ingot", 1, 4, 5, 0, nil, LootFuncNone, 0},
					{"honeycomb", 2, 8, 5, 0, nil, LootFuncNone, 0},
					{"tuff", 5, 10, 8, 0, nil, LootFuncNone, 0},
					{"scaffolding", 2, 6, 5, 0, nil, LootFuncNone, 0},
					{"glow_berries", 2, 10, 5, 0, nil, LootFuncNone, 0},
				},
			},
			{ // Rare trial chamber loot
				MinRolls: 1, MaxRolls: 2,
				Entries: []LootEntry{
					{"trial_key", 1, 1, 5, 2, nil, LootFuncNone, 0},
					{"breeze_rod", 1, 3, 3, 1, nil, LootFuncNone, 0},
					{"diamond", 1, 2, 2, 1, nil, LootFuncNone, 0},
					{"flow_armor_trim_smithing_template", 1, 1, 1, 1, nil, LootFuncNone, 0},
					{"bolt_armor_trim_smithing_template", 1, 1, 1, 1, nil, LootFuncNone, 0},
					{"diamond_chestplate", 1, 1, 1, 1, nil, LootFuncEnchantLevels, 20},
					{"diamond_axe", 1, 1, 1, 1, nil, LootFuncEnchantLevels, 20},
					{"book", 1, 1, 2, 1, nil, LootFuncEnchantRandomly, 0},
				},
			},
		},
	},
}

// GenerateChestLoot fills a chest's items array with random loot from a table.
func GenerateChestLoot(tableType string, chest *ChestState) {
	GenerateChestLootWithLuck(tableType, chest, 0)
}

// GenerateChestLootWithLuck fills a chest's items array with random loot,
// applying a luck modifier that boosts entries with positive Quality values.
func GenerateChestLootWithLuck(tableType string, chest *ChestState, luck float64) {
	table, ok := lootTables[tableType]
	if !ok {
		return
	}

	pools := table.pools()
	if len(pools) == 0 {
		return
	}

	// Collect all generated items across pools.
	var items []game.ItemStack
	for _, pool := range pools {
		items = append(items, generatePool(pool, luck)...)
	}

	// Distribute items into random chest slots.
	slots := rand.Perm(27)
	n := len(items)
	if n > 27 {
		n = 27
	}
	for i := 0; i < n; i++ {
		chest.Items[slots[i]] = items[i]
	}
}

// generatePool rolls a single loot pool and returns the generated item stacks.
func generatePool(pool LootPool, luck float64) []game.ItemStack {
	numRolls := pool.MinRolls
	if pool.MaxRolls > pool.MinRolls {
		numRolls += rand.Intn(pool.MaxRolls - pool.MinRolls + 1)
	}

	// Calculate total effective weight.
	totalWeight := effectiveTotalWeight(pool.Entries, luck)
	if totalWeight <= 0 {
		return nil
	}

	var result []game.ItemStack
	for i := 0; i < numRolls; i++ {
		entry := weightedPickWithLuck(pool.Entries, totalWeight, luck)
		count := entry.MinCount
		if entry.MaxCount > entry.MinCount {
			count += rand.Int31n(entry.MaxCount - entry.MinCount + 1)
		}
		if count <= 0 {
			continue
		}
		itemID := itemIDByName(entry.ItemName)
		if itemID <= 0 {
			continue
		}
		stack := game.ItemStack{
			ID:    itemID,
			Count: count,
		}
		// Copy fixed enchantments.
		if len(entry.Enchantments) > 0 {
			stack.Enchantments = make(map[string]int32, len(entry.Enchantments))
			for k, v := range entry.Enchantments {
				stack.Enchantments[k] = v
			}
		}
		// Apply loot functions.
		applyLootFunction(&stack, entry)
		result = append(result, stack)
	}
	return result
}

// effectiveTotalWeight sums the effective weights of all entries with luck applied.
func effectiveTotalWeight(entries []LootEntry, luck float64) float64 {
	var total float64
	for _, e := range entries {
		w := float64(e.Weight) + float64(e.Quality)*luck
		if w > 0 {
			total += w
		}
	}
	return total
}

// weightedPickWithLuck selects an entry using quality-adjusted weights.
func weightedPickWithLuck(entries []LootEntry, totalWeight float64, luck float64) LootEntry {
	r := rand.Float64() * totalWeight
	for _, e := range entries {
		w := float64(e.Weight) + float64(e.Quality)*luck
		if w <= 0 {
			continue
		}
		r -= w
		if r < 0 {
			return e
		}
	}
	return entries[len(entries)-1]
}

// applyLootFunction performs post-processing on a generated item stack.
func applyLootFunction(stack *game.ItemStack, entry LootEntry) {
	switch entry.Function {
	case LootFuncEnchantRandomly:
		applyRandomEnchantment(stack)
	case LootFuncEnchantLevels:
		applyEnchantWithLevels(stack, entry.FuncParam)
	case LootFuncSetDamage:
		applySetDamage(stack, entry.FuncParam)
	}
}

// enchantNames is a cached list of non-alias enchantment names for random selection.
var enchantNames []string

func init() {
	for name := range enchant.Registry {
		if name == enchant.Sweeping {
			continue
		}
		enchantNames = append(enchantNames, name)
	}
}

// applyRandomEnchantment picks a random enchantment from the registry and
// applies it at a random level between 1 and its max.
func applyRandomEnchantment(stack *game.ItemStack) {
	if len(enchantNames) == 0 {
		return
	}
	chosen := enchantNames[rand.Intn(len(enchantNames))]
	maxLvl := enchant.MaxLevel(chosen)
	if maxLvl <= 0 {
		maxLvl = 1
	}
	level := int32(1) + rand.Int31n(maxLvl)
	stack.Enchantments = enchant.ApplyToItem(stack.Enchantments, chosen, level)
}

// applyEnchantWithLevels simulates enchanting-table results at the given level.
// It picks 1-3 compatible enchantments scaled by the level parameter.
func applyEnchantWithLevels(stack *game.ItemStack, level int32) {
	if level <= 0 {
		level = 1
	}
	if len(enchantNames) == 0 {
		return
	}

	// Number of enchantments scales with level: 1 at low, up to 3 at high.
	numEnch := 1
	if level >= 15 {
		numEnch = 2
	}
	if level >= 25 {
		numEnch = 1 + rand.Intn(3) // 1-3
	}

	for i := 0; i < numEnch; i++ {
		chosen := enchantNames[rand.Intn(len(enchantNames))]
		maxLvl := enchant.MaxLevel(chosen)
		if maxLvl <= 0 {
			continue
		}
		// Scale level: higher enchanting level means higher enchantment level.
		enchLvl := int32(1)
		if maxLvl > 1 {
			scaled := int32(float64(level) / 30.0 * float64(maxLvl))
			if scaled < 1 {
				scaled = 1
			}
			if scaled > maxLvl {
				scaled = maxLvl
			}
			enchLvl = scaled
		}
		// Check compatibility with existing enchantments.
		compatible := true
		for existing := range stack.Enchantments {
			if !enchant.IsCompatible(chosen, existing) {
				compatible = false
				break
			}
		}
		if !compatible {
			continue
		}
		stack.Enchantments = enchant.ApplyToItem(stack.Enchantments, chosen, enchLvl)
	}
}

// applySetDamage sets random durability on a tool/armor item. The FuncParam
// represents the item's max durability. The item gets a random remaining
// durability between 10% and 100% of max.
func applySetDamage(stack *game.ItemStack, maxDurability int32) {
	if maxDurability <= 0 {
		return
	}
	stack.MaxDurability = maxDurability
	// 10% to 100% remaining
	minDur := maxDurability / 10
	if minDur < 1 {
		minDur = 1
	}
	stack.Durability = minDur + rand.Int31n(maxDurability-minDur+1)
}

// mobDropTable maps mob type IDs to their loot pools.
var mobDropTable = map[int32][]LootPool{
	MobTypeZombie: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"rotten_flesh", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 1, 1, 1, nil, LootFuncNone, 0},
			{"carrot", 1, 1, 1, 1, nil, LootFuncNone, 0},
			{"potato", 1, 1, 1, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeSkeleton: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"bone", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"arrow", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeCreeper: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"gunpowder", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeSpider: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"string", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"spider_eye", 0, 1, 3, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeEnderman: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"ender_pearl", 0, 1, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeBlaze: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"blaze_rod", 0, 1, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeWitherSkeleton: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"coal", 0, 1, 10, 0, nil, LootFuncNone, 0},
			{"bone", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"wither_skeleton_skull", 1, 1, 1, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeGhast: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"ghast_tear", 0, 1, 10, 0, nil, LootFuncNone, 0},
			{"gunpowder", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeSlime: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"slime_ball", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeMagmaCube: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"magma_cream", 0, 1, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeWitch: {{
		MinRolls: 1, MaxRolls: 3,
		Entries: []LootEntry{
			{"glass_bottle", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"glowstone_dust", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"gunpowder", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"redstone", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"spider_eye", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"sugar", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"stick", 0, 2, 5, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeCow: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"leather", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"beef", 1, 3, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypePig: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"porkchop", 1, 3, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeSheep: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"mutton", 1, 2, 10, 0, nil, LootFuncNone, 0},
			{"white_wool", 1, 1, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeChicken: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"chicken", 1, 1, 10, 0, nil, LootFuncNone, 0},
			{"feather", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypePhantom: {{
		MinRolls: 1, MaxRolls: 1,
		Entries: []LootEntry{
			{"phantom_membrane", 0, 1, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeDrowned: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"rotten_flesh", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"copper_ingot", 1, 1, 2, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeHusk: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"rotten_flesh", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"iron_ingot", 1, 1, 1, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeStray: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"bone", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"arrow", 0, 2, 10, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeCaveSpider: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"string", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"spider_eye", 0, 1, 3, 1, nil, LootFuncNone, 0},
		},
	}},
	MobTypeGuardian: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"prismarine_shard", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"raw_cod", 0, 1, 5, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeElderGuardian: {{
		MinRolls: 1, MaxRolls: 3,
		Entries: []LootEntry{
			{"prismarine_shard", 0, 2, 10, 0, nil, LootFuncNone, 0},
			{"prismarine_crystals", 0, 2, 5, 0, nil, LootFuncNone, 0},
			{"wet_sponge", 1, 1, 3, 0, nil, LootFuncNone, 0},
			{"raw_cod", 0, 1, 5, 0, nil, LootFuncNone, 0},
		},
	}},
	MobTypeIronGolem: {{
		MinRolls: 1, MaxRolls: 2,
		Entries: []LootEntry{
			{"iron_ingot", 3, 5, 10, 0, nil, LootFuncNone, 0},
			{"poppy", 0, 2, 5, 0, nil, LootFuncNone, 0},
		},
	}},
}

// GenerateMobDrops returns item stacks that a mob should drop on death.
// killedByPlayer gates player-only drops (like spider eyes, ender pearls).
// lootingLevel increases drop quantities (each level adds one potential extra).
func GenerateMobDrops(mobTypeID int32, killedByPlayer bool, lootingLevel int32) []game.ItemStack {
	pools, ok := mobDropTable[mobTypeID]
	if !ok {
		return nil
	}

	var result []game.ItemStack
	for _, pool := range pools {
		numRolls := pool.MinRolls
		if pool.MaxRolls > pool.MinRolls {
			numRolls += rand.Intn(pool.MaxRolls - pool.MinRolls + 1)
		}

		totalWeight := effectiveTotalWeight(pool.Entries, 0)
		if totalWeight <= 0 {
			continue
		}

		for i := 0; i < numRolls; i++ {
			entry := weightedPickWithLuck(pool.Entries, totalWeight, 0)

			// Player-only drops: quality > 0 entries require a player kill.
			if entry.Quality > 0 && !killedByPlayer {
				continue
			}

			count := entry.MinCount
			if entry.MaxCount > entry.MinCount {
				count += rand.Int31n(entry.MaxCount - entry.MinCount + 1)
			}

			// Looting bonus: each level adds 0-1 extra.
			if lootingLevel > 0 {
				count += rand.Int31n(lootingLevel + 1)
			}

			if count <= 0 {
				continue
			}

			itemID := itemIDByName(entry.ItemName)
			if itemID <= 0 {
				continue
			}

			result = append(result, game.ItemStack{
				ID:    itemID,
				Count: count,
			})
		}
	}
	return result
}
