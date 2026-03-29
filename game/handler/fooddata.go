package handler

// FoodInfo describes the nutritional value of a food item.
type FoodInfo struct {
	Nutrition  int32
	Saturation float32
}

// LookupFood returns the FoodInfo for an item name (without minecraft: prefix),
// or nil if the item is not food.
func LookupFood(itemName string) *FoodInfo {
	if info, ok := foodTable[itemName]; ok {
		return &info
	}
	return nil
}

var foodTable = map[string]FoodInfo{
	// Fruits
	"apple":           {4, 2.4},
	"golden_apple":    {4, 9.6},
	"enchanted_golden_apple": {4, 9.6},
	"chorus_fruit":    {4, 2.4},
	"melon_slice":     {2, 1.2},
	"sweet_berries":   {2, 0.4},
	"glow_berries":    {2, 0.4},

	// Vegetables
	"carrot":          {3, 3.6},
	"golden_carrot":   {6, 14.4},
	"potato":          {1, 0.6},
	"baked_potato":    {5, 6.0},
	"poisonous_potato": {2, 1.2},
	"beetroot":        {1, 1.2},
	"dried_kelp":      {1, 0.6},

	// Meat (raw)
	"beef":            {3, 1.8},
	"porkchop":        {3, 1.8},
	"mutton":          {2, 1.2},
	"chicken":         {2, 1.2},
	"rabbit":          {3, 1.8},
	"cod":             {2, 0.4},
	"salmon":          {2, 0.4},
	"tropical_fish":   {1, 0.2},
	"pufferfish":      {1, 0.2},

	// Meat (cooked)
	"cooked_beef":     {8, 12.8},
	"cooked_porkchop": {8, 12.8},
	"cooked_mutton":   {6, 9.6},
	"cooked_chicken":  {6, 7.2},
	"cooked_rabbit":   {5, 6.0},
	"cooked_cod":      {5, 6.0},
	"cooked_salmon":   {6, 9.6},

	// Baked goods
	"bread":           {5, 6.0},
	"cookie":          {2, 0.4},
	"pumpkin_pie":     {8, 4.8},
	"cake":            {2, 0.4}, // per slice, simplified

	// Soups/stews
	"mushroom_stew":   {6, 7.2},
	"beetroot_soup":   {6, 7.2},
	"rabbit_stew":     {10, 12.0},
	"suspicious_stew": {6, 7.2},

	// Other
	"rotten_flesh":    {4, 0.8},
	"spider_eye":      {2, 3.2},
	"honey_bottle":    {6, 1.2},
}
