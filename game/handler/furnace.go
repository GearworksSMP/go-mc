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

// FurnaceState holds the state of a single furnace block.
type FurnaceState struct {
	Input         game.ItemStack // slot 0
	Fuel          game.ItemStack // slot 1
	Output        game.ItemStack // slot 2
	BurnTime      int32          // remaining burn ticks
	MaxBurnTime   int32          // total burn ticks for current fuel
	CookTime      int32          // current smelting progress (0-199)
	CookTimeTotal int32          // total cook time (200)
	Pos           [3]int
}

// FurnaceManager tracks all furnaces in the world.
type FurnaceManager struct {
	Furnaces map[[3]int]*FurnaceState
	mu       sync.RWMutex
	Manager  *game.PlayerManager
}

// NewFurnaceManager creates a new FurnaceManager.
func NewFurnaceManager(manager *game.PlayerManager) *FurnaceManager {
	return &FurnaceManager{
		Furnaces: make(map[[3]int]*FurnaceState),
		Manager:  manager,
	}
}

// GetOrCreate returns the furnace state at the given position, creating it if needed.
func (fm *FurnaceManager) GetOrCreate(x, y, z int) *FurnaceState {
	pos := [3]int{x, y, z}
	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fs, ok := fm.Furnaces[pos]; ok {
		return fs
	}
	fs := &FurnaceState{Pos: pos, CookTimeTotal: 200}
	fm.Furnaces[pos] = fs
	return fs
}

// Get returns the furnace state at the given position, or nil.
func (fm *FurnaceManager) Get(x, y, z int) *FurnaceState {
	pos := [3]int{x, y, z}
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.Furnaces[pos]
}

// OpenFurnace opens a furnace window for the player.
func (fm *FurnaceManager) OpenFurnace(player *game.Player, x, y, z int) {
	fs := fm.GetOrCreate(x, y, z)

	player.OpenWindowID = 3
	player.OpenFurnacePos = [3]int{x, y, z}

	title := chat.Text("Furnace")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(3),  // window ID
		pk.VarInt(14), // menu type: furnace
		title,
	))

	SendFurnaceWindowContent(player, fs)
	sendFurnaceProperties(player, fs)
}

// SendFurnaceWindowContent sends the full furnace window content.
// Layout: 39 slots = 3 furnace + 27 main inv + 9 hotbar
func SendFurnaceWindowContent(player *game.Player, fs *FurnaceState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 39)
	slots[0] = fs.Input.ToSlot()
	slots[1] = fs.Fuel.ToSlot()
	slots[2] = fs.Output.ToSlot()
	// Main inventory: window slots 3-29 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[3+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: window slots 30-38 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[30+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(3), // window ID 3
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// sendFurnaceProperties sends the 4 furnace data properties to the player.
func sendFurnaceProperties(player *game.Player, fs *FurnaceState) {
	// Property 0: BurnTime remaining
	sendContainerData(player, 3, 0, int16(fs.BurnTime))
	// Property 1: MaxBurnTime
	sendContainerData(player, 3, 1, int16(fs.MaxBurnTime))
	// Property 2: CookTime progress
	sendContainerData(player, 3, 2, int16(fs.CookTime))
	// Property 3: CookTimeTotal
	sendContainerData(player, 3, 3, int16(fs.CookTimeTotal))
}

func sendContainerData(player *game.Player, windowID int, property int, value int16) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.UnsignedByte(windowID),
		pk.Short(int16(property)),
		pk.Short(value),
	))
}

// FurnaceSlot returns a pointer to the item stack for a furnace window slot.
func FurnaceSlot(player *game.Player, fs *FurnaceState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot == 0:
		return &fs.Input
	case windowSlot == 1:
		return &fs.Fuel
	case windowSlot == 2:
		return &fs.Output
	case windowSlot >= 3 && windowSlot <= 29:
		return &player.Inventory[windowSlot-3+9]
	case windowSlot >= 30 && windowSlot <= 38:
		return &player.Inventory[windowSlot-30+36]
	}
	return nil
}

// persistedFurnaceState is the JSON format for furnace persistence.
type persistedFurnaceState struct {
	Input    persistedItem `json:"input"`
	Fuel     persistedItem `json:"fuel"`
	Output   persistedItem `json:"output"`
	BurnTime int32         `json:"burn_time"`
	CookTime int32         `json:"cook_time"`
}

// SaveAll serializes all furnace states for persistence.
func (fm *FurnaceManager) SaveAll(dim string) []store.BlockEntityData {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	var result []store.BlockEntityData
	for pos, fs := range fm.Furnaces {
		pfs := persistedFurnaceState{
			BurnTime: fs.BurnTime,
			CookTime: fs.CookTime,
		}
		if fs.Input.ID > 0 {
			pfs.Input = persistedItem{ID: fs.Input.ID, Count: fs.Input.Count}
		}
		if fs.Fuel.ID > 0 {
			pfs.Fuel = persistedItem{ID: fs.Fuel.ID, Count: fs.Fuel.Count}
		}
		if fs.Output.ID > 0 {
			pfs.Output = persistedItem{ID: fs.Output.ID, Count: fs.Output.Count}
		}
		data, err := json.Marshal(pfs)
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "furnace", Data: data,
		})
	}
	return result
}

// LoadAll restores furnace states from persisted data.
func (fm *FurnaceManager) LoadAll(entities []store.BlockEntityData) {
	for _, e := range entities {
		if e.Type != "furnace" {
			continue
		}
		var pfs persistedFurnaceState
		if err := json.Unmarshal(e.Data, &pfs); err != nil {
			continue
		}
		fs := fm.GetOrCreate(e.X, e.Y, e.Z)
		if pfs.Input.ID > 0 {
			fs.Input = game.ItemStack{ID: pfs.Input.ID, Count: pfs.Input.Count}
		}
		if pfs.Fuel.ID > 0 {
			fs.Fuel = game.ItemStack{ID: pfs.Fuel.ID, Count: pfs.Fuel.Count}
		}
		if pfs.Output.ID > 0 {
			fs.Output = game.ItemStack{ID: pfs.Output.ID, Count: pfs.Output.Count}
		}
		fs.BurnTime = pfs.BurnTime
		fs.CookTime = pfs.CookTime
	}
}

// Tick processes smelting logic for all active furnaces.
func (fm *FurnaceManager) Tick() {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	for _, fs := range fm.Furnaces {
		changed := false

		// Determine if input has a valid recipe
		inputName := ""
		if fs.Input.ID > 0 && fs.Input.Count > 0 {
			inputName = ItemNameByID(fs.Input.ID)
		}
		resultName, hasRecipe := smeltingRecipes[inputName]

		// Check if output slot can accept the result
		canOutput := false
		if hasRecipe {
			resultID := itemIDByName(resultName)
			if resultID > 0 {
				if fs.Output.ID == 0 || fs.Output.Count <= 0 {
					canOutput = true
				} else if fs.Output.ID == resultID && fs.Output.Count < 64 {
					canOutput = true
				}
			}
		}

		// Decrement burn time
		wasBurning := fs.BurnTime > 0
		if fs.BurnTime > 0 {
			fs.BurnTime--
			changed = true
		}

		if hasRecipe && canOutput {
			// Try to start burning if not already
			if fs.BurnTime <= 0 && fs.Fuel.ID > 0 && fs.Fuel.Count > 0 {
				fuelName := ItemNameByID(fs.Fuel.ID)
				if burnTicks, ok := fuelBurnTicks[fuelName]; ok {
					fs.BurnTime = burnTicks
					fs.MaxBurnTime = burnTicks
					fs.Fuel.Count--
					if fs.Fuel.Count <= 0 {
						fs.Fuel = game.ItemStack{}
					}
					changed = true
				}
			}

			// Progress cooking while burning
			if fs.BurnTime > 0 {
				fs.CookTime++
				changed = true

				if fs.CookTime >= 200 {
					// Smelting complete
					resultID := itemIDByName(resultName)
					if fs.Output.ID == 0 || fs.Output.Count <= 0 {
						fs.Output = game.ItemStack{ID: resultID, Count: 1}
					} else {
						fs.Output.Count++
					}
					fs.Input.Count--
					if fs.Input.Count <= 0 {
						fs.Input = game.ItemStack{}
					}
					fs.CookTime = 0

					// Award smelting XP to the nearest viewer
					xp := getSmeltingXP(resultName)
					if xp > 0 {
						fm.Manager.ForEach(func(p *game.Player) {
							if p.OpenWindowID == 3 && p.OpenFurnacePos == fs.Pos {
								AddExperience(p, xp)
							}
						})
					}
				}
			}
		} else {
			// No valid recipe or output full — reset cook progress
			if fs.CookTime > 0 {
				fs.CookTime = 0
				changed = true
			}
		}

		// If state changed, update viewers
		if changed || wasBurning {
			fm.updateViewers(fs)
		}
	}
}

// updateViewers sends container data and slot updates to players viewing this furnace.
func (fm *FurnaceManager) updateViewers(fs *FurnaceState) {
	fm.Manager.ForEach(func(p *game.Player) {
		if p.OpenWindowID == 3 && p.OpenFurnacePos == fs.Pos {
			sendFurnaceProperties(p, fs)
			// Update the 3 furnace slots
			sendFurnaceSlot(p, 0, fs.Input)
			sendFurnaceSlot(p, 1, fs.Fuel)
			sendFurnaceSlot(p, 2, fs.Output)
		}
	})
}

func sendFurnaceSlot(player *game.Player, slot int, item game.ItemStack) {
	s := item.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetSlot,
		pk.UnsignedByte(3),
		pk.VarInt(0),
		pk.Short(int16(slot)),
		s,
	))
}

// Smelting recipes: input item name → output item name
var smeltingRecipes = map[string]string{
	// Raw ores
	"raw_iron":   "iron_ingot",
	"raw_gold":   "gold_ingot",
	"raw_copper": "copper_ingot",
	// Ore blocks
	"iron_ore":   "iron_ingot",
	"gold_ore":   "gold_ingot",
	"copper_ore": "copper_ingot",
	// Stone
	"cobblestone": "stone",
	"sand":        "glass",
	"clay_ball":   "brick",
	// Logs → charcoal
	"oak_log":      "charcoal",
	"spruce_log":   "charcoal",
	"birch_log":    "charcoal",
	"jungle_log":   "charcoal",
	"acacia_log":   "charcoal",
	"dark_oak_log": "charcoal",
	"cherry_log":   "charcoal",
	"mangrove_log": "charcoal",
	// Food
	"raw_beef":    "cooked_beef",
	"raw_porkchop": "cooked_porkchop",
	"raw_chicken": "cooked_chicken",
	"raw_mutton":  "cooked_mutton",
	"raw_cod":     "cooked_cod",
	"raw_salmon":  "cooked_salmon",
	"potato":      "baked_potato",
}

// Fuel burn times in ticks
var fuelBurnTicks = map[string]int32{
	"coal":        1600,
	"charcoal":    1600,
	"coal_block":  16000,
	"lava_bucket": 20000,
	"blaze_rod":   2400,
	// Planks
	"oak_planks":      300,
	"spruce_planks":   300,
	"birch_planks":    300,
	"jungle_planks":   300,
	"acacia_planks":   300,
	"dark_oak_planks": 300,
	"cherry_planks":   300,
	"mangrove_planks": 300,
	// Logs
	"oak_log":      300,
	"spruce_log":   300,
	"birch_log":    300,
	"jungle_log":   300,
	"acacia_log":   300,
	"dark_oak_log": 300,
	"cherry_log":   300,
	"mangrove_log": 300,
	// Sticks
	"stick": 100,
	// Wooden tools/weapons
	"wooden_pickaxe": 200,
	"wooden_axe":     200,
	"wooden_shovel":  200,
	"wooden_hoe":     200,
	"wooden_sword":   200,
}

// smeltingXP maps result item name to XP awarded per smelt (integer approximation).
var smeltingXP = map[string]int32{
	"iron_ingot":   1,
	"gold_ingot":   1,
	"copper_ingot": 1,
	"stone":        1,
	"glass":        1,
	"brick":        1,
	"charcoal":     1,
}

// getSmeltingXP returns the XP to award for smelting the given result item.
func getSmeltingXP(resultName string) int32 {
	if xp, ok := smeltingXP[resultName]; ok {
		return xp
	}
	return 0
}
