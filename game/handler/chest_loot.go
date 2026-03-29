package handler

import (
	"math/rand"

	"github.com/Tnze/go-mc/game"
)

// LootEntry represents a possible item in a loot table.
type LootEntry struct {
	ItemName           string
	MinCount, MaxCount int32
	Weight             int
	Enchantments       map[string]int32 // optional enchantments to apply
}

// LootTable defines a set of possible loot items and how many slots to fill.
type LootTable struct {
	MinItems, MaxItems int
	Entries            []LootEntry
}

// Loot tables for various structure types.
var lootTables = map[string]*LootTable{
	"dungeon": {
		MinItems: 2, MaxItems: 8,
		Entries: []LootEntry{
			{"string", 1, 4, 10, nil},
			{"gunpowder", 1, 4, 10, nil},
			{"wheat", 1, 4, 10, nil},
			{"bread", 1, 1, 10, nil},
			{"name_tag", 1, 1, 5, nil},
			{"saddle", 1, 1, 5, nil},
			{"iron_ingot", 1, 4, 10, nil},
			{"gold_ingot", 1, 4, 5, nil},
			{"redstone", 1, 4, 5, nil},
			{"coal", 1, 4, 10, nil},
			{"bone", 1, 4, 10, nil},
			{"rotten_flesh", 1, 4, 10, nil},
			{"music_disc_13", 1, 1, 1, nil},
			{"music_disc_cat", 1, 1, 1, nil},
		},
	},
	"mineshaft": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"rail", 4, 8, 10, nil},
			{"torch", 1, 16, 10, nil},
			{"iron_ingot", 1, 5, 8, nil},
			{"gold_ingot", 1, 3, 4, nil},
			{"lapis_lazuli", 4, 9, 5, nil},
			{"bread", 1, 3, 8, nil},
			{"melon_seeds", 2, 4, 5, nil},
			{"pumpkin_seeds", 2, 4, 5, nil},
			{"iron_pickaxe", 1, 1, 3, nil},
			{"name_tag", 1, 1, 3, nil},
			{"golden_apple", 1, 1, 1, nil},
		},
	},
	"temple": {
		MinItems: 2, MaxItems: 7,
		Entries: []LootEntry{
			{"diamond", 1, 3, 3, nil},
			{"emerald", 1, 3, 3, nil},
			{"gold_ingot", 2, 7, 8, nil},
			{"iron_ingot", 1, 5, 8, nil},
			{"bone", 4, 6, 10, nil},
			{"rotten_flesh", 3, 7, 10, nil},
			{"spider_eye", 1, 3, 5, nil},
			{"enchanted_golden_apple", 1, 1, 1, nil},
			{"saddle", 1, 1, 3, nil},
			{"iron_horse_armor", 1, 1, 2, nil},
			{"golden_horse_armor", 1, 1, 1, nil},
			{"diamond_horse_armor", 1, 1, 1, nil},
		},
	},
	"village": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"bread", 1, 4, 10, nil},
			{"apple", 1, 3, 8, nil},
			{"potato", 1, 7, 8, nil},
			{"carrot", 1, 7, 8, nil},
			{"wheat", 1, 4, 10, nil},
			{"oak_sapling", 1, 2, 5, nil},
			{"iron_ingot", 1, 3, 3, nil},
			{"gold_ingot", 1, 1, 2, nil},
			{"emerald", 1, 1, 2, nil},
			{"coal", 1, 3, 5, nil},
			{"oak_planks", 1, 4, 10, nil},
		},
	},
	"stronghold": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"ender_pearl", 1, 1, 5, nil},
			{"diamond", 1, 3, 3, nil},
			{"iron_ingot", 1, 5, 8, nil},
			{"gold_ingot", 1, 3, 5, nil},
			{"book", 1, 3, 8, nil},
			{"apple", 1, 3, 8, nil},
			{"bread", 1, 3, 8, nil},
			{"iron_pickaxe", 1, 1, 3, nil},
			{"iron_sword", 1, 1, 3, nil},
			{"iron_boots", 1, 1, 2, nil},
			{"iron_chestplate", 1, 1, 2, nil},
		},
	},
	"nether_fortress": {
		MinItems: 2, MaxItems: 7,
		Entries: []LootEntry{
			{"gold_ingot", 1, 3, 10, nil},
			{"iron_ingot", 1, 5, 8, nil},
			{"nether_wart", 3, 7, 5, nil},
			{"saddle", 1, 1, 5, nil},
			{"golden_horse_armor", 1, 1, 3, nil},
			{"iron_horse_armor", 1, 1, 4, nil},
			{"diamond_horse_armor", 1, 1, 1, nil},
			{"diamond", 1, 3, 3, nil},
			{"flint_and_steel", 1, 1, 3, nil},
			{"obsidian", 2, 4, 2, nil},
			{"blaze_rod", 1, 2, 2, nil},
		},
	},
	"bastion": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"gold_ingot", 2, 8, 10, nil},
			{"gold_block", 1, 2, 3, nil},
			{"iron_ingot", 1, 6, 8, nil},
			{"crossbow", 1, 1, 3, nil},
			{"spectral_arrow", 4, 12, 5, nil},
			{"magma_cream", 2, 6, 5, nil},
			{"string", 4, 6, 8, nil},
			{"iron_sword", 1, 1, 3, nil},
			{"golden_sword", 1, 1, 5, nil},
			{"golden_axe", 1, 1, 3, nil},
			{"golden_boots", 1, 1, 2, nil},
			{"ancient_debris", 1, 1, 1, nil},
		},
	},
	"end_city": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"diamond", 2, 7, 5, nil},
			{"iron_ingot", 4, 8, 8, nil},
			{"gold_ingot", 2, 7, 8, nil},
			{"emerald", 2, 6, 5, nil},
			{"beetroot_seeds", 1, 10, 5, nil},
			{"diamond_sword", 1, 1, 2, map[string]int32{"sharpness": 3}},
			{"diamond_pickaxe", 1, 1, 2, map[string]int32{"efficiency": 3}},
			{"diamond_shovel", 1, 1, 2, map[string]int32{"efficiency": 3}},
			{"diamond_chestplate", 1, 1, 2, map[string]int32{"protection": 3}},
			{"diamond_boots", 1, 1, 2, map[string]int32{"protection": 3, "feather_falling": 3}},
			{"diamond_helmet", 1, 1, 2, nil},
			{"diamond_leggings", 1, 1, 2, nil},
			{"iron_leggings", 1, 1, 3, nil},
			{"iron_boots", 1, 1, 3, nil},
			{"elytra", 1, 1, 1, nil},
		},
	},
	"buried_treasure": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"heart_of_the_sea", 1, 1, 10, nil},
			{"iron_ingot", 1, 4, 8, nil},
			{"gold_ingot", 1, 4, 6, nil},
			{"tnt", 1, 2, 3, nil},
			{"emerald", 4, 8, 5, nil},
			{"diamond", 1, 2, 3, nil},
			{"cooked_cod", 2, 4, 5, nil},
			{"cooked_salmon", 2, 4, 5, nil},
			{"prismarine_crystals", 1, 5, 5, nil},
			{"iron_sword", 1, 1, 2, nil},
			{"leather_tunic", 1, 1, 2, nil},
		},
	},
	"shipwreck_treasure": {
		MinItems: 3, MaxItems: 6,
		Entries: []LootEntry{
			{"iron_ingot", 1, 5, 10, nil},
			{"iron_nugget", 1, 10, 8, nil},
			{"emerald", 1, 5, 5, nil},
			{"diamond", 1, 1, 3, nil},
			{"lapis_lazuli", 1, 10, 8, nil},
			{"gold_ingot", 1, 5, 5, nil},
			{"gold_nugget", 1, 10, 8, nil},
		},
	},
	"shipwreck_supply": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"paper", 1, 12, 10, nil},
			{"wheat", 8, 21, 8, nil},
			{"carrot", 4, 8, 5, nil},
			{"potato", 2, 6, 5, nil},
			{"poisonous_potato", 2, 6, 3, nil},
			{"coal", 2, 8, 8, nil},
			{"tnt", 1, 2, 2, nil},
			{"gunpowder", 1, 5, 5, nil},
			{"enchanted_golden_apple", 1, 1, 1, nil},
			{"bamboo", 1, 3, 3, nil},
			{"pumpkin", 1, 3, 3, nil},
		},
	},
	"igloo": {
		MinItems: 2, MaxItems: 5,
		Entries: []LootEntry{
			{"golden_apple", 1, 1, 5, nil},
			{"coal", 1, 4, 10, nil},
			{"apple", 1, 3, 8, nil},
			{"wheat", 2, 3, 8, nil},
			{"stone_axe", 1, 1, 3, nil},
		},
	},
	"woodland_mansion": {
		MinItems: 3, MaxItems: 8,
		Entries: []LootEntry{
			{"diamond", 1, 3, 3, nil},
			{"iron_ingot", 1, 4, 8, nil},
			{"gold_ingot", 1, 4, 5, nil},
			{"diamond_chestplate", 1, 1, 1, nil},
			{"diamond_sword", 1, 1, 1, nil},
			{"iron_sword", 1, 1, 3, nil},
			{"bread", 1, 4, 8, nil},
			{"bone", 1, 8, 10, nil},
			{"gunpowder", 1, 8, 10, nil},
			{"string", 1, 8, 10, nil},
			{"bucket", 1, 1, 3, nil},
			{"redstone", 1, 4, 5, nil},
			{"name_tag", 1, 1, 3, nil},
			{"lead", 1, 1, 3, nil},
			{"music_disc_13", 1, 1, 1, nil},
			{"music_disc_cat", 1, 1, 1, nil},
		},
	},
	"ruined_portal": {
		MinItems: 2, MaxItems: 6,
		Entries: []LootEntry{
			{"obsidian", 1, 2, 10, nil},
			{"flint_and_steel", 1, 1, 5, nil},
			{"iron_nugget", 9, 18, 8, nil},
			{"gold_nugget", 4, 24, 8, nil},
			{"golden_sword", 1, 1, 3, nil},
			{"golden_axe", 1, 1, 3, nil},
			{"golden_apple", 1, 1, 2, nil},
			{"enchanted_golden_apple", 1, 1, 1, nil},
			{"fire_charge", 1, 1, 5, nil},
			{"golden_horse_armor", 1, 1, 2, nil},
			{"golden_boots", 1, 1, 2, nil},
		},
	},
}

// GenerateChestLoot fills a chest's items array with random loot from a table.
func GenerateChestLoot(tableType string, chest *ChestState) {
	table, ok := lootTables[tableType]
	if !ok {
		return
	}

	// Calculate total weight
	totalWeight := 0
	for _, e := range table.Entries {
		totalWeight += e.Weight
	}
	if totalWeight <= 0 {
		return
	}

	// Determine how many items to place
	numItems := table.MinItems
	if table.MaxItems > table.MinItems {
		numItems += rand.Intn(table.MaxItems - table.MinItems + 1)
	}

	// Pick random slots (27 slots in a chest)
	slots := rand.Perm(27)
	if numItems > 27 {
		numItems = 27
	}

	for i := 0; i < numItems; i++ {
		// Weighted random selection
		entry := weightedPick(table.Entries, totalWeight)
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
		if len(entry.Enchantments) > 0 {
			stack.Enchantments = make(map[string]int32, len(entry.Enchantments))
			for k, v := range entry.Enchantments {
				stack.Enchantments[k] = v
			}
		}
		chest.Items[slots[i]] = stack
	}
}

func weightedPick(entries []LootEntry, totalWeight int) LootEntry {
	r := rand.Intn(totalWeight)
	for _, e := range entries {
		r -= e.Weight
		if r < 0 {
			return e
		}
	}
	return entries[len(entries)-1]
}
