package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// campfireCookingRecipes maps raw food item names to their cooked equivalents.
var campfireCookingRecipes = map[string]string{
	"raw_beef":    "cooked_beef",
	"raw_porkchop": "cooked_porkchop",
	"raw_chicken": "cooked_chicken",
	"raw_mutton":  "cooked_mutton",
	"raw_cod":     "cooked_cod",
	"raw_salmon":  "cooked_salmon",
	"potato":      "baked_potato",
	"kelp":        "dried_kelp",
}

// campfireCookTime is the number of ticks to cook an item on a campfire (30 seconds).
const campfireCookTime = 600

// CampfireSlot holds a single cooking slot on a campfire.
type CampfireSlot struct {
	ItemID   int32
	OutputID int32
	Progress int32 // ticks elapsed
	MaxTime  int32 // total cook time
}

// CampfireData holds the state of a single campfire block.
type CampfireData struct {
	Slots [4]*CampfireSlot
}

// CampfireManager tracks all campfires and their cooking state.
type CampfireManager struct {
	Manager  *game.PlayerManager
	World    game.World
	ItemMgr  *ItemEntityManager
	mu       sync.Mutex
	campfires map[[3]int]*CampfireData
}

// NewCampfireManager creates a new CampfireManager.
func NewCampfireManager(manager *game.PlayerManager, world game.World, itemMgr *ItemEntityManager) *CampfireManager {
	return &CampfireManager{
		Manager:   manager,
		World:     world,
		ItemMgr:   itemMgr,
		campfires: make(map[[3]int]*CampfireData),
	}
}

// RegisterCampfire begins tracking a campfire at the given position.
func (cm *CampfireManager) RegisterCampfire(x, y, z int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if _, ok := cm.campfires[[3]int{x, y, z}]; !ok {
		cm.campfires[[3]int{x, y, z}] = &CampfireData{}
	}
}

// UnregisterCampfire stops tracking a campfire at the given position.
func (cm *CampfireManager) UnregisterCampfire(x, y, z int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.campfires, [3]int{x, y, z})
}

// PlaceItem attempts to place the player's held item on the campfire.
// Returns true if the item was placed.
func (cm *CampfireManager) PlaceItem(player *game.Player, x, y, z int) bool {
	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]
	if held.ID <= 0 || held.Count <= 0 {
		return false
	}

	itemName := ItemNameByID(held.ID)
	outputName, ok := campfireCookingRecipes[itemName]
	if !ok {
		return false
	}
	outputID := itemIDByName(outputName)
	if outputID <= 0 {
		return false
	}

	// Verify the campfire is lit
	stateID, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		switch cf := block.StateList[stateID].(type) {
		case block.Campfire:
			if !bool(cf.Lit) {
				return false
			}
		case block.SoulCampfire:
			if !bool(cf.Lit) {
				return false
			}
		default:
			return false
		}
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, exists := cm.campfires[[3]int{x, y, z}]
	if !exists {
		data = &CampfireData{}
		cm.campfires[[3]int{x, y, z}] = data
	}

	// Find first empty slot
	for i := range data.Slots {
		if data.Slots[i] == nil {
			data.Slots[i] = &CampfireSlot{
				ItemID:   held.ID,
				OutputID: outputID,
				Progress: 0,
				MaxTime:  campfireCookTime,
			}
			// Consume one item from held stack
			held.Count--
			if held.Count <= 0 {
				*held = game.ItemStack{}
			}
			SendSlotUpdate(player, heldSlot)
			return true
		}
	}

	return false // all slots occupied
}

// RemoveItems drops all items from a campfire when it is broken.
func (cm *CampfireManager) RemoveItems(x, y, z int) {
	cm.mu.Lock()
	data, exists := cm.campfires[[3]int{x, y, z}]
	if !exists {
		cm.mu.Unlock()
		return
	}
	// Copy slots and remove from map
	var slots [4]*CampfireSlot
	copy(slots[:], data.Slots[:])
	delete(cm.campfires, [3]int{x, y, z})
	cm.mu.Unlock()

	// Drop items as entities
	if cm.ItemMgr == nil {
		return
	}
	cx, cy, cz := float64(x)+0.5, float64(y)+1.0, float64(z)+0.5
	for _, slot := range slots {
		if slot != nil && slot.ItemID > 0 {
			cm.ItemMgr.SpawnItem(cm.Manager, cx, cy, cz, slot.ItemID, 1, 10)
		}
	}
}

// cookedDrop is an item to spawn after releasing the lock.
type cookedDrop struct {
	x, y, z float64
	itemID  int32
}

// Tick advances cooking on all tracked campfires.
func (cm *CampfireManager) Tick(tick int64) {
	cm.mu.Lock()

	var drops []cookedDrop
	var smokePositions [][3]int

	for pos, data := range cm.campfires {
		hasItem := false
		for i, slot := range data.Slots {
			if slot == nil {
				continue
			}
			hasItem = true
			slot.Progress++
			if slot.Progress >= slot.MaxTime {
				drops = append(drops, cookedDrop{
					x: float64(pos[0]) + 0.5, y: float64(pos[1]) + 1.0, z: float64(pos[2]) + 0.5,
					itemID: slot.OutputID,
				})
				data.Slots[i] = nil
			}
		}
		if hasItem && tick%40 == 0 {
			smokePositions = append(smokePositions, pos)
		}
	}

	cm.mu.Unlock()

	// Spawn cooked items and particles outside the lock
	if cm.ItemMgr != nil {
		for _, d := range drops {
			cm.ItemMgr.SpawnItem(cm.Manager, d.x, d.y, d.z, d.itemID, 1, 10)
		}
	}
	for _, pos := range smokePositions {
		BroadcastParticle(cm.Manager, ParticleSmoke,
			float64(pos[0])+0.5, float64(pos[1])+0.8, float64(pos[2])+0.5,
			0.2, 0.3, 0.2, 0.01, 2)
	}
}

// ---------------------------------------------------------------------------
// Farmland Trampling (exported for use by movement handler)
// ---------------------------------------------------------------------------

// TrampleFarmland checks if the block at (x, y, z) is farmland and converts it to dirt.
// Returns true if the block was trampled.
func TrampleFarmland(world game.World, manager *game.PlayerManager, x, y, z int) bool {
	stateID, err := world.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	blockName := BlockNameFromState(int(stateID))
	if blockName != "farmland" {
		return false
	}
	dirtID, ok := block.ToStateID[block.Dirt{}]
	if !ok {
		return false
	}
	world.SetBlock(x, y, z, dirtID)
	broadcastBlockUpdateDirect(manager, x, y, z, int32(dirtID))
	return true
}

// ---------------------------------------------------------------------------
// Chorus Fruit Teleport
// ---------------------------------------------------------------------------

// isAirBlock returns true if the block name represents an air-type block.
func isAirBlock(name string) bool {
	return name == "air" || name == "cave_air" || name == "void_air"
}

// isPassableBlock returns true if the block cannot support a player standing on it.
func isPassableBlock(name string) bool {
	return isAirBlock(name) || name == "water" || name == "lava"
}

// HandleChorusFruitEat teleports the player to a random valid position within 8 blocks.
// A valid position has a solid block below and air at both feet and head level.
// Tries up to 16 random positions, teleports to the first valid one.
func HandleChorusFruitEat(player *game.Player, world game.World, manager *game.PlayerManager) bool {
	px, py, pz := player.Position()

	for attempt := 0; attempt < 16; attempt++ {
		// Random offset: -8 to +8 horizontally and vertically
		tx := px + (rand.Float64()*16 - 8)
		ty := py + (rand.Float64()*16 - 8)
		tz := pz + (rand.Float64()*16 - 8)

		bx := int(tx)
		by := int(ty)
		bz := int(tz)

		if by < -64 || by > 319 {
			continue
		}

		// Feet and head must be air, block below must be solid
		feetState, err := world.GetBlock(bx, by, bz)
		if err != nil || !isAirBlock(BlockNameFromState(int(feetState))) {
			continue
		}
		headState, err := world.GetBlock(bx, by+1, bz)
		if err != nil || !isAirBlock(BlockNameFromState(int(headState))) {
			continue
		}
		belowState, err := world.GetBlock(bx, by-1, bz)
		if err != nil || isPassableBlock(BlockNameFromState(int(belowState))) {
			continue
		}

		// Valid position found — teleport
		destX, destY, destZ := float64(bx)+0.5, float64(by), float64(bz)+0.5
		player.SetPosition(destX, destY, destZ)
		player.TeleportPending = true

		// Send teleport packet
		player.WritePacket(pk.Marshal(
			packetid.ClientboundPlayerPosition,
			pk.VarInt(99),            // teleport ID
			pk.Double(destX),
			pk.Double(destY),
			pk.Double(destZ),
			pk.Double(0), pk.Double(0), pk.Double(0), // velocity
			pk.Float(0), pk.Float(0), // yaw, pitch (relative)
			pk.Int(0x08|0x10), // flags: yaw+pitch relative, position absolute
		))

		// Play teleport sound and particles at both origin and destination
		BroadcastParticle(manager, ParticlePoof, px, py+0.5, pz, 0.3, 0.5, 0.3, 0.05, 10)
		BroadcastParticle(manager, ParticlePoof, float64(bx)+0.5, float64(by)+0.5, float64(bz)+0.5, 0.3, 0.5, 0.3, 0.05, 10)

		return true
	}
	return false
}
