package handler

import (
	"encoding/json"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	pk "github.com/Tnze/go-mc/net/packet"
)

// BrewingState holds the state of a single brewing stand block.
type BrewingState struct {
	Bottles     [3]game.ItemStack // slots 0-2: potion output slots
	Ingredient  game.ItemStack    // slot 3: ingredient
	Fuel        game.ItemStack    // slot 4: fuel (blaze powder)
	FuelCharges int32             // remaining brews from current fuel (0-20)
	BrewTime    int32             // remaining brew ticks (400 = 20 seconds)
	Pos         [3]int
}

// BrewingStandManager tracks all brewing stands in the world.
type BrewingStandManager struct {
	Stands  map[[3]int]*BrewingState
	mu      sync.RWMutex
	Manager *game.PlayerManager
}

// BrewingWindowID is the window ID used for brewing stand windows.
const BrewingWindowID = 7

// NewBrewingStandManager creates a new BrewingStandManager.
func NewBrewingStandManager(manager *game.PlayerManager) *BrewingStandManager {
	return &BrewingStandManager{
		Stands:  make(map[[3]int]*BrewingState),
		Manager: manager,
	}
}

// GetOrCreate returns the brewing stand state at the given position, creating it if needed.
func (bm *BrewingStandManager) GetOrCreate(x, y, z int) *BrewingState {
	pos := [3]int{x, y, z}
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if bs, ok := bm.Stands[pos]; ok {
		return bs
	}
	bs := &BrewingState{Pos: pos}
	bm.Stands[pos] = bs
	return bs
}

// Get returns the brewing stand state at the given position, or nil.
func (bm *BrewingStandManager) Get(x, y, z int) *BrewingState {
	pos := [3]int{x, y, z}
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.Stands[pos]
}

// OpenBrewingStand opens a brewing stand window for the player.
func (bm *BrewingStandManager) OpenBrewingStand(player *game.Player, x, y, z int) {
	bs := bm.GetOrCreate(x, y, z)

	player.OpenWindowID = BrewingWindowID
	player.OpenBrewingStandPos = [3]int{x, y, z}

	title := chat.Text("Brewing Stand")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(BrewingWindowID), // window ID
		pk.VarInt(12),             // menu type: brewing_stand
		title,
	))

	SendBrewingWindowContent(player, bs)
	sendBrewingProperties(player, bs)
}

// SendBrewingWindowContent sends the full brewing stand window content.
// Layout: 41 slots = 5 brewing stand + 27 main inv + 9 hotbar
func SendBrewingWindowContent(player *game.Player, bs *BrewingState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 41)
	// Slots 0-2: bottle output slots
	slots[0] = bs.Bottles[0].ToSlot()
	slots[1] = bs.Bottles[1].ToSlot()
	slots[2] = bs.Bottles[2].ToSlot()
	// Slot 3: ingredient
	slots[3] = bs.Ingredient.ToSlot()
	// Slot 4: fuel (blaze powder)
	slots[4] = bs.Fuel.ToSlot()
	// Main inventory: window slots 5-31 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[5+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: window slots 32-40 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[32+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(BrewingWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// sendBrewingProperties sends the 2 brewing stand data properties to the player.
func sendBrewingProperties(player *game.Player, bs *BrewingState) {
	// Property 0: Brew time (0-400)
	sendContainerData(player, BrewingWindowID, 0, int16(bs.BrewTime))
	// Property 1: Fuel charges (0-20)
	sendContainerData(player, BrewingWindowID, 1, int16(bs.FuelCharges))
}

// BrewingSlot returns a pointer to the item stack for a brewing stand window slot.
func BrewingSlot(player *game.Player, bs *BrewingState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 2:
		return &bs.Bottles[windowSlot]
	case windowSlot == 3:
		return &bs.Ingredient
	case windowSlot == 4:
		return &bs.Fuel
	case windowSlot >= 5 && windowSlot <= 31:
		return &player.Inventory[windowSlot-5+9]
	case windowSlot >= 32 && windowSlot <= 40:
		return &player.Inventory[windowSlot-32+36]
	}
	return nil
}

// persistedBrewingState is the JSON format for brewing stand persistence.
type persistedBrewingState struct {
	Bottles    []persistedItem `json:"bottles"`
	Ingredient persistedItem   `json:"ingredient"`
	Fuel       persistedItem   `json:"fuel"`
	BrewTime   int32           `json:"brew_time"`
	FuelCharge int32           `json:"fuel_charges"`
}

// SaveAll serializes all brewing stand states for persistence.
func (bm *BrewingStandManager) SaveAll(dim string) []store.BlockEntityData {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	var result []store.BlockEntityData
	for pos, bs := range bm.Stands {
		pbs := persistedBrewingState{
			Bottles:    containerItemSlots(bs.Bottles[:]),
			BrewTime:   bs.BrewTime,
			FuelCharge: bs.FuelCharges,
		}
		if bs.Ingredient.ID > 0 {
			pbs.Ingredient = persistedItem{ID: bs.Ingredient.ID, Count: bs.Ingredient.Count}
		}
		if bs.Fuel.ID > 0 {
			pbs.Fuel = persistedItem{ID: bs.Fuel.ID, Count: bs.Fuel.Count}
		}
		data, err := json.Marshal(pbs)
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "brewing_stand", Data: data,
		})
	}
	return result
}

// LoadAll restores brewing stand states from persisted data.
func (bm *BrewingStandManager) LoadAll(entities []store.BlockEntityData) {
	for _, e := range entities {
		if e.Type != "brewing_stand" {
			continue
		}
		var pbs persistedBrewingState
		if err := json.Unmarshal(e.Data, &pbs); err != nil {
			continue
		}
		bs := bm.GetOrCreate(e.X, e.Y, e.Z)
		loadItemSlots(bs.Bottles[:], pbs.Bottles)
		if pbs.Ingredient.ID > 0 {
			bs.Ingredient = game.ItemStack{ID: pbs.Ingredient.ID, Count: pbs.Ingredient.Count}
		}
		if pbs.Fuel.ID > 0 {
			bs.Fuel = game.ItemStack{ID: pbs.Fuel.ID, Count: pbs.Fuel.Count}
		}
		bs.BrewTime = pbs.BrewTime
		bs.FuelCharges = pbs.FuelCharge
	}
}

// Tick processes brewing logic for all active brewing stands.
func (bm *BrewingStandManager) Tick() {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for _, bs := range bm.Stands {
		changed := false

		// If no fuel charges and fuel slot has blaze_powder, consume 1 and set charges
		if bs.FuelCharges <= 0 && bs.Fuel.ID > 0 && bs.Fuel.Count > 0 {
			fuelName := ItemNameByID(bs.Fuel.ID)
			if fuelName == "blaze_powder" {
				bs.Fuel.Count--
				if bs.Fuel.Count <= 0 {
					bs.Fuel = game.ItemStack{}
				}
				bs.FuelCharges = 20
				changed = true
			}
		}

		// Check if we have an ingredient and at least one brewable bottle
		ingredientName := ""
		if bs.Ingredient.ID > 0 && bs.Ingredient.Count > 0 {
			ingredientName = ItemNameByID(bs.Ingredient.ID)
		}

		canBrew := false
		if ingredientName != "" && bs.FuelCharges > 0 {
			if recipes, ok := brewingRecipes[ingredientName]; ok {
				for i := 0; i < 3; i++ {
					if bs.Bottles[i].ID > 0 && bs.Bottles[i].Count > 0 {
						bottleName := ItemNameByID(bs.Bottles[i].ID)
						if _, ok := recipes[bottleName]; ok {
							canBrew = true
							break
						}
					}
				}
			}
		}

		if canBrew {
			// Start or continue brewing
			if bs.BrewTime <= 0 {
				bs.BrewTime = 400
				changed = true
			}

			bs.BrewTime--
			changed = true

			if bs.BrewTime <= 0 {
				// Brewing complete — apply recipes to all valid bottles
				recipes := brewingRecipes[ingredientName]
				for i := 0; i < 3; i++ {
					if bs.Bottles[i].ID > 0 && bs.Bottles[i].Count > 0 {
						bottleName := ItemNameByID(bs.Bottles[i].ID)
						if recipe, ok := recipes[bottleName]; ok {
							resultID := itemIDByName(recipe.OutputItem)
							if resultID > 0 {
								potionType := recipe.PotionType
								if potionType == "" {
									// Modifier ingredients keep existing potion type
									potionType = bs.Bottles[i].PotionType
								}
								bs.Bottles[i] = game.ItemStack{
									ID:         resultID,
									Count:      1,
									PotionType: potionType,
								}
							}
						}
					}
				}

				// Consume 1 ingredient
				bs.Ingredient.Count--
				if bs.Ingredient.Count <= 0 {
					bs.Ingredient = game.ItemStack{}
				}

				// Consume 1 fuel charge
				bs.FuelCharges--

				bs.BrewTime = 0
			}
		} else {
			// No valid recipe — reset brew progress
			if bs.BrewTime > 0 {
				bs.BrewTime = 0
				changed = true
			}
		}

		if changed {
			bm.updateViewers(bs)
		}
	}
}

// updateViewers sends container data and slot updates to players viewing this brewing stand.
func (bm *BrewingStandManager) updateViewers(bs *BrewingState) {
	bm.Manager.ForEach(func(p *game.Player) {
		if p.OpenWindowID == BrewingWindowID && p.OpenBrewingStandPos == bs.Pos {
			sendBrewingProperties(p, bs)
			// Update the 5 brewing stand slots
			for i := 0; i < 3; i++ {
				sendBrewingSlot(p, i, bs.Bottles[i])
			}
			sendBrewingSlot(p, 3, bs.Ingredient)
			sendBrewingSlot(p, 4, bs.Fuel)
		}
	})
}

func sendBrewingSlot(player *game.Player, slot int, item game.ItemStack) {
	s := item.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetSlot,
		pk.UnsignedByte(BrewingWindowID),
		pk.VarInt(0),
		pk.Short(int16(slot)),
		s,
	))
}

// brewRecipe defines what a brewing ingredient produces.
type brewRecipe struct {
	OutputItem string // output item name (usually "potion")
	PotionType string // potion type set on the output
}

// Brewing recipes: ingredient name → map of input item name → recipe output.
var brewingRecipes = map[string]map[string]brewRecipe{
	"nether_wart":            {"potion": {OutputItem: "potion", PotionType: "awkward"}},
	"sugar":                  {"potion": {OutputItem: "potion", PotionType: "swiftness"}},
	"blaze_powder":           {"potion": {OutputItem: "potion", PotionType: "strength"}},
	"ghast_tear":             {"potion": {OutputItem: "potion", PotionType: "regeneration"}},
	"glistering_melon_slice": {"potion": {OutputItem: "potion", PotionType: "healing"}},
	"spider_eye":             {"potion": {OutputItem: "potion", PotionType: "poison"}},
	"golden_carrot":          {"potion": {OutputItem: "potion", PotionType: "night_vision"}},
	"magma_cream":            {"potion": {OutputItem: "potion", PotionType: "fire_resistance"}},
	"rabbit_foot":            {"potion": {OutputItem: "potion", PotionType: "leaping"}},
	"pufferfish":             {"potion": {OutputItem: "potion", PotionType: "water_breathing"}},
	"fermented_spider_eye":   {"potion": {OutputItem: "potion", PotionType: "weakness"}},
	"redstone":               {"potion": {OutputItem: "potion", PotionType: ""}},   // extends duration (keeps type)
	"glowstone_dust":         {"potion": {OutputItem: "potion", PotionType: ""}},   // amplifies (keeps type)
	"gunpowder":              {"potion": {OutputItem: "splash_potion", PotionType: ""}}, // potion → splash (keeps type)
	"dragon_breath":          {"splash_potion": {OutputItem: "lingering_potion", PotionType: ""}},
}

// MatchTippedArrowRecipe checks if the 3x3 crafting grid contains the tipped arrow recipe:
// 8 arrows surrounding 1 lingering potion in the center.
// grid is a 9-element slice of ItemStack (row-major 3x3). Returns the tipped arrow
// item ID, count (8), and the PotionType from the lingering potion. Returns (0,0,"") if no match.
func MatchTippedArrowRecipe(grid []game.ItemStack) (resultID, count int32, potionType string) {
	if len(grid) != 9 {
		return 0, 0, ""
	}

	// Center slot (index 4) must be a lingering potion
	centerName := ItemNameByID(grid[4].ID)
	if centerName != "lingering_potion" || grid[4].Count <= 0 {
		return 0, 0, ""
	}

	// All 8 surrounding slots must be arrows
	arrowID := itemIDByName("arrow")
	if arrowID <= 0 {
		return 0, 0, ""
	}
	for i := 0; i < 9; i++ {
		if i == 4 {
			continue
		}
		if grid[i].ID != arrowID || grid[i].Count <= 0 {
			return 0, 0, ""
		}
	}

	tippedID := itemIDByName("tipped_arrow")
	if tippedID <= 0 {
		return 0, 0, ""
	}
	return tippedID, 8, grid[4].PotionType
}
