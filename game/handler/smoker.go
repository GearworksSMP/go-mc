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

// SmokerWindowID is the window ID used for smoker windows.
const SmokerWindowID = 15

// SmokerState holds the state of a single smoker block.
// A smoker is a furnace variant that smelts food items twice as fast (100 ticks).
type SmokerState struct {
	Input         game.ItemStack
	Fuel          game.ItemStack
	Output        game.ItemStack
	BurnTime      int32
	MaxBurnTime   int32
	CookTime      int32
	CookTimeTotal int32
	Pos           [3]int
}

// SmokerManager tracks all smokers in the world.
type SmokerManager struct {
	Smokers map[[3]int]*SmokerState
	mu      sync.RWMutex
	Manager *game.PlayerManager
}

// NewSmokerManager creates a new SmokerManager.
func NewSmokerManager(manager *game.PlayerManager) *SmokerManager {
	return &SmokerManager{
		Smokers: make(map[[3]int]*SmokerState),
		Manager: manager,
	}
}

// GetOrCreate returns the smoker state at the given position, creating it if needed.
func (sm *SmokerManager) GetOrCreate(x, y, z int) *SmokerState {
	pos := [3]int{x, y, z}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if ss, ok := sm.Smokers[pos]; ok {
		return ss
	}
	ss := &SmokerState{Pos: pos, CookTimeTotal: 100}
	sm.Smokers[pos] = ss
	return ss
}

// Get returns the smoker state at the given position, or nil.
func (sm *SmokerManager) Get(x, y, z int) *SmokerState {
	pos := [3]int{x, y, z}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.Smokers[pos]
}

// OpenSmoker opens a smoker window for the player.
func (sm *SmokerManager) OpenSmoker(player *game.Player, x, y, z int) {
	ss := sm.GetOrCreate(x, y, z)

	player.OpenWindowID = SmokerWindowID
	player.OpenFurnacePos = [3]int{x, y, z} // reuse furnace pos field

	title := chat.Text("Smoker")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(SmokerWindowID),
		pk.VarInt(22), // menu type: smoker
		title,
	))

	SendSmokerWindowContent(player, ss)
	sendSmokerProperties(player, ss)
}

// SendSmokerWindowContent sends the full smoker window content.
// Layout: 39 slots = 3 smoker + 27 main inv + 9 hotbar
func SendSmokerWindowContent(player *game.Player, ss *SmokerState) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 39)
	slots[0] = ss.Input.ToSlot()
	slots[1] = ss.Fuel.ToSlot()
	slots[2] = ss.Output.ToSlot()
	for i := 9; i <= 35; i++ {
		slots[3+(i-9)] = player.Inventory[i].ToSlot()
	}
	for i := 36; i <= 44; i++ {
		slots[30+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(SmokerWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// sendSmokerProperties sends the furnace-style property data for the smoker.
func sendSmokerProperties(player *game.Player, ss *SmokerState) {
	sendContainerData(player, SmokerWindowID, 0, int16(ss.BurnTime))
	sendContainerData(player, SmokerWindowID, 1, int16(ss.MaxBurnTime))
	sendContainerData(player, SmokerWindowID, 2, int16(ss.CookTime))
	sendContainerData(player, SmokerWindowID, 3, int16(ss.CookTimeTotal))
}

// SmokerSlot returns a pointer to the item stack for a smoker window slot.
func SmokerSlot(player *game.Player, ss *SmokerState, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot == 0:
		return &ss.Input
	case windowSlot == 1:
		return &ss.Fuel
	case windowSlot == 2:
		return &ss.Output
	case windowSlot >= 3 && windowSlot <= 29:
		return &player.Inventory[windowSlot-3+9]
	case windowSlot >= 30 && windowSlot <= 38:
		return &player.Inventory[windowSlot-30+36]
	}
	return nil
}

// SaveAll serializes all smoker states for persistence.
func (sm *SmokerManager) SaveAll(dim string) []store.BlockEntityData {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var result []store.BlockEntityData
	for pos, ss := range sm.Smokers {
		pfs := persistedFurnaceState{
			BurnTime: ss.BurnTime,
			CookTime: ss.CookTime,
		}
		if ss.Input.ID > 0 {
			pfs.Input = persistedItem{ID: ss.Input.ID, Count: ss.Input.Count}
		}
		if ss.Fuel.ID > 0 {
			pfs.Fuel = persistedItem{ID: ss.Fuel.ID, Count: ss.Fuel.Count}
		}
		if ss.Output.ID > 0 {
			pfs.Output = persistedItem{ID: ss.Output.ID, Count: ss.Output.Count}
		}
		data, err := json.Marshal(pfs)
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "smoker", Data: data,
		})
	}
	return result
}

// LoadAll restores smoker states from persisted data.
func (sm *SmokerManager) LoadAll(entities []store.BlockEntityData) {
	for _, e := range entities {
		if e.Type != "smoker" {
			continue
		}
		var pfs persistedFurnaceState
		if err := json.Unmarshal(e.Data, &pfs); err != nil {
			continue
		}
		ss := sm.GetOrCreate(e.X, e.Y, e.Z)
		if pfs.Input.ID > 0 {
			ss.Input = game.ItemStack{ID: pfs.Input.ID, Count: pfs.Input.Count}
		}
		if pfs.Fuel.ID > 0 {
			ss.Fuel = game.ItemStack{ID: pfs.Fuel.ID, Count: pfs.Fuel.Count}
		}
		if pfs.Output.ID > 0 {
			ss.Output = game.ItemStack{ID: pfs.Output.ID, Count: pfs.Output.Count}
		}
		ss.BurnTime = pfs.BurnTime
		ss.CookTime = pfs.CookTime
	}
}

// smokerSmeltable returns the output item name for a given input in the smoker.
// Smokers can only smelt food items.
func smokerSmeltable(inputName string) string {
	switch inputName {
	case "beef":
		return "cooked_beef"
	case "porkchop":
		return "cooked_porkchop"
	case "chicken":
		return "cooked_chicken"
	case "mutton":
		return "cooked_mutton"
	case "rabbit":
		return "cooked_rabbit"
	case "cod":
		return "cooked_cod"
	case "salmon":
		return "cooked_salmon"
	case "potato":
		return "baked_potato"
	case "kelp":
		return "dried_kelp"
	default:
		return ""
	}
}

// TickSmokers advances smelting for all active smokers.
func (sm *SmokerManager) TickSmokers() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, ss := range sm.Smokers {
		if ss.Input.ID <= 0 && ss.BurnTime <= 0 {
			continue
		}

		wasBurning := ss.BurnTime > 0

		if ss.BurnTime > 0 {
			ss.BurnTime--
		}

		inputName := ""
		if ss.Input.ID > 0 {
			inputName = ItemNameByID(ss.Input.ID)
		}
		canSmelt := inputName != "" && smokerSmeltable(inputName) != ""
		if canSmelt {
			outputName := smokerSmeltable(inputName)
			outputID := itemIDByName(outputName)
			canSmelt = outputID > 0 && (ss.Output.ID == 0 || (ss.Output.ID == outputID && ss.Output.Count < 64))
		}

		if ss.BurnTime <= 0 && canSmelt && ss.Fuel.ID > 0 {
			burnTicks, ok := fuelBurnTicks[ItemNameByID(ss.Fuel.ID)]
			if ok && burnTicks > 0 {
				ss.BurnTime = burnTicks
				ss.MaxBurnTime = burnTicks
				ss.Fuel.Count--
				if ss.Fuel.Count <= 0 {
					ss.Fuel = game.ItemStack{}
				}
			}
		}

		if ss.BurnTime > 0 && canSmelt {
			ss.CookTime++
			if ss.CookTime >= ss.CookTimeTotal {
				ss.CookTime = 0
				outputName := smokerSmeltable(inputName)
				outputID := itemIDByName(outputName)
				if ss.Output.ID == 0 {
					ss.Output = game.ItemStack{ID: outputID, Count: 1}
				} else {
					ss.Output.Count++
				}
				ss.Input.Count--
				if ss.Input.Count <= 0 {
					ss.Input = game.ItemStack{}
				}
			}
		} else if !canSmelt {
			ss.CookTime = 0
		}

		if wasBurning || ss.BurnTime > 0 {
			sm.broadcastSmokerUpdate(ss)
		}
	}
}

// broadcastSmokerUpdate sends smoker progress to all players viewing this smoker.
func (sm *SmokerManager) broadcastSmokerUpdate(ss *SmokerState) {
	if sm.Manager == nil {
		return
	}
	sm.Manager.ForEach(func(p *game.Player) {
		if p.OpenWindowID == SmokerWindowID && p.OpenFurnacePos == ss.Pos {
			sendSmokerProperties(p, ss)
		}
	})
}
