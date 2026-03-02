package handler

import (
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// BlastFurnaceWindowID is the window ID used for blast furnace windows.
const BlastFurnaceWindowID = 16

// BlastFurnaceState holds the state of a single blast furnace block.
// A blast furnace smelts ores and metal items twice as fast (100 ticks).
type BlastFurnaceState struct {
	Input         game.ItemStack
	Fuel          game.ItemStack
	Output        game.ItemStack
	BurnTime      int32
	MaxBurnTime   int32
	CookTime      int32
	CookTimeTotal int32
	Pos           [3]int
}

// BlastFurnaceManager tracks all blast furnaces in the world.
type BlastFurnaceManager struct {
	BlastFurnaces map[[3]int]*BlastFurnaceState
	mu            sync.RWMutex
	Manager       *game.PlayerManager
}

// NewBlastFurnaceManager creates a new BlastFurnaceManager.
func NewBlastFurnaceManager(manager *game.PlayerManager) *BlastFurnaceManager {
	return &BlastFurnaceManager{
		BlastFurnaces: make(map[[3]int]*BlastFurnaceState),
		Manager:       manager,
	}
}

// GetOrCreate returns the blast furnace state at the given position, creating it if needed.
func (bfm *BlastFurnaceManager) GetOrCreate(x, y, z int) *BlastFurnaceState {
	pos := [3]int{x, y, z}
	bfm.mu.Lock()
	defer bfm.mu.Unlock()
	if bfs, ok := bfm.BlastFurnaces[pos]; ok {
		return bfs
	}
	bfs := &BlastFurnaceState{Pos: pos, CookTimeTotal: 100}
	bfm.BlastFurnaces[pos] = bfs
	return bfs
}

// Get returns the blast furnace state at the given position, or nil.
func (bfm *BlastFurnaceManager) Get(x, y, z int) *BlastFurnaceState {
	pos := [3]int{x, y, z}
	bfm.mu.RLock()
	defer bfm.mu.RUnlock()
	return bfm.BlastFurnaces[pos]
}

// OpenBlastFurnace opens a blast furnace window for the player.
func (bfm *BlastFurnaceManager) OpenBlastFurnace(player *game.Player, x, y, z int) {
	bfs := bfm.GetOrCreate(x, y, z)

	player.OpenWindowID = BlastFurnaceWindowID
	player.OpenFurnacePos = [3]int{x, y, z} // reuse furnace pos field

	title := chat.Text("Blast Furnace")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(BlastFurnaceWindowID),
		pk.VarInt(10), // menu type: blast_furnace
		title,
	))

	SendBlastFurnaceWindowContent(player, bfs)
	sendBlastFurnaceProperties(player, bfs)
}

// SendBlastFurnaceWindowContent sends the full blast furnace window content.
// Layout: 39 slots = 3 furnace + 27 main inv + 9 hotbar
func SendBlastFurnaceWindowContent(player *game.Player, bfs *BlastFurnaceState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 39)
	slots[0] = bfs.Input.ToSlot()
	slots[1] = bfs.Fuel.ToSlot()
	slots[2] = bfs.Output.ToSlot()
	for i := 9; i <= 35; i++ {
		slots[3+(i-9)] = player.Inventory[i].ToSlot()
	}
	for i := 36; i <= 44; i++ {
		slots[30+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(BlastFurnaceWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// sendBlastFurnaceProperties sends the furnace-style property data for the blast furnace.
func sendBlastFurnaceProperties(player *game.Player, bfs *BlastFurnaceState) {
	sendContainerData(player, BlastFurnaceWindowID, 0, int16(bfs.BurnTime))
	sendContainerData(player, BlastFurnaceWindowID, 1, int16(bfs.MaxBurnTime))
	sendContainerData(player, BlastFurnaceWindowID, 2, int16(bfs.CookTime))
	sendContainerData(player, BlastFurnaceWindowID, 3, int16(bfs.CookTimeTotal))
}

// BlastFurnaceSlot returns a pointer to the item stack for a blast furnace window slot.
func BlastFurnaceSlot(player *game.Player, bfs *BlastFurnaceState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot == 0:
		return &bfs.Input
	case windowSlot == 1:
		return &bfs.Fuel
	case windowSlot == 2:
		return &bfs.Output
	case windowSlot >= 3 && windowSlot <= 29:
		return &player.Inventory[windowSlot-3+9]
	case windowSlot >= 30 && windowSlot <= 38:
		return &player.Inventory[windowSlot-30+36]
	}
	return nil
}

// blastFurnaceSmeltable returns the output item name for a given input.
// Blast furnaces can only smelt ores, raw metals, and metal/chain armor.
func blastFurnaceSmeltable(inputName string) string {
	switch inputName {
	case "iron_ore", "deepslate_iron_ore", "raw_iron":
		return "iron_ingot"
	case "gold_ore", "deepslate_gold_ore", "raw_gold", "nether_gold_ore":
		return "gold_ingot"
	case "copper_ore", "deepslate_copper_ore", "raw_copper":
		return "copper_ingot"
	case "diamond_ore", "deepslate_diamond_ore":
		return "diamond"
	case "emerald_ore", "deepslate_emerald_ore":
		return "emerald"
	case "lapis_ore", "deepslate_lapis_ore":
		return "lapis_lazuli"
	case "redstone_ore", "deepslate_redstone_ore":
		return "redstone"
	case "coal_ore", "deepslate_coal_ore":
		return "coal"
	case "ancient_debris":
		return "netherite_scrap"
	case "iron_helmet", "iron_chestplate", "iron_leggings", "iron_boots",
		"iron_sword", "iron_pickaxe", "iron_axe", "iron_shovel", "iron_hoe":
		return "iron_nugget"
	case "golden_helmet", "golden_chestplate", "golden_leggings", "golden_boots",
		"golden_sword", "golden_pickaxe", "golden_axe", "golden_shovel", "golden_hoe":
		return "gold_nugget"
	case "chainmail_helmet", "chainmail_chestplate", "chainmail_leggings", "chainmail_boots":
		return "iron_nugget"
	default:
		return ""
	}
}

// TickBlastFurnaces advances smelting for all active blast furnaces.
func (bfm *BlastFurnaceManager) TickBlastFurnaces() {
	bfm.mu.RLock()
	defer bfm.mu.RUnlock()

	for _, bfs := range bfm.BlastFurnaces {
		if bfs.Input.ID <= 0 && bfs.BurnTime <= 0 {
			continue
		}

		wasBurning := bfs.BurnTime > 0

		if bfs.BurnTime > 0 {
			bfs.BurnTime--
		}

		inputName := ""
		if bfs.Input.ID > 0 {
			inputName = ItemNameByID(bfs.Input.ID)
		}
		canSmelt := inputName != "" && blastFurnaceSmeltable(inputName) != ""
		if canSmelt {
			outputName := blastFurnaceSmeltable(inputName)
			outputID := itemIDByName(outputName)
			canSmelt = outputID > 0 && (bfs.Output.ID == 0 || (bfs.Output.ID == outputID && bfs.Output.Count < 64))
		}

		if bfs.BurnTime <= 0 && canSmelt && bfs.Fuel.ID > 0 {
			burnTicks, ok := fuelBurnTicks[ItemNameByID(bfs.Fuel.ID)]
			if ok && burnTicks > 0 {
				bfs.BurnTime = burnTicks
				bfs.MaxBurnTime = burnTicks
				bfs.Fuel.Count--
				if bfs.Fuel.Count <= 0 {
					bfs.Fuel = game.ItemStack{}
				}
			}
		}

		if bfs.BurnTime > 0 && canSmelt {
			bfs.CookTime++
			if bfs.CookTime >= bfs.CookTimeTotal {
				bfs.CookTime = 0
				outputName := blastFurnaceSmeltable(inputName)
				outputID := itemIDByName(outputName)
				if bfs.Output.ID == 0 {
					bfs.Output = game.ItemStack{ID: outputID, Count: 1}
				} else {
					bfs.Output.Count++
				}
				bfs.Input.Count--
				if bfs.Input.Count <= 0 {
					bfs.Input = game.ItemStack{}
				}
			}
		} else if !canSmelt {
			bfs.CookTime = 0
		}

		if wasBurning || bfs.BurnTime > 0 {
			bfm.broadcastBlastFurnaceUpdate(bfs)
		}
	}
}

// broadcastBlastFurnaceUpdate sends blast furnace progress to all players viewing it.
func (bfm *BlastFurnaceManager) broadcastBlastFurnaceUpdate(bfs *BlastFurnaceState) {
	if bfm.Manager == nil {
		return
	}
	bfm.Manager.ForEach(func(p *game.Player) {
		if p.OpenWindowID == BlastFurnaceWindowID && p.OpenFurnacePos == bfs.Pos {
			sendBlastFurnaceProperties(p, bfs)
		}
	})
}
