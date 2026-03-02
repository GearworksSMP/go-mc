package handler

import (
	"log"
	"math"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// EndPortalManager handles end portal frame interaction and End dimension teleportation.
type EndPortalManager struct {
	World        game.World    // overworld
	EndWorld     game.World    // the end
	Manager      *game.PlayerManager
	DimensionMgr *DimensionManager
	Logger       *log.Logger
}

// HandleFrameClick handles a player right-clicking an end_portal_frame with an ender_eye.
// Returns true if the interaction was handled.
func (m *EndPortalManager) HandleFrameClick(player *game.Player, x, y, z int, stateID int) bool {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	frame, ok := block.StateList[stateID].(block.EndPortalFrame)
	if !ok {
		return false
	}

	// Only interact if the frame doesn't already have an eye
	if frame.Eye {
		return false
	}

	// Check if player is holding ender_eye (item ID 1003)
	heldID := player.HeldItemID()
	if heldID != 1003 {
		return false
	}

	// Set the eye on the frame
	frame.Eye = true
	newStateID, found := block.ToStateID[frame]
	if !found {
		return false
	}

	m.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateStatic(m.Manager, x, y, z, int32(newStateID))

	// Consume one ender eye from inventory (survival mode)
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		if player.Inventory[slot].ID == 1003 && player.Inventory[slot].Count > 0 {
			player.Inventory[slot].Count--
			if player.Inventory[slot].Count <= 0 {
				player.Inventory[slot] = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Check if a complete portal ring has been formed
	if m.checkAndActivatePortal(x, y, z) {
		m.logf("End portal activated at (%d, %d, %d) by %s", x, y, z, player.Name)
	}

	return true
}

// checkAndActivatePortal checks if the frame at (x, y, z) is part of a complete
// 3x3 end portal ring. If so, it fills the 3x3 interior with end_portal blocks.
// Returns true if a portal was activated.
func (m *EndPortalManager) checkAndActivatePortal(x, y, z int) bool {
	// An end portal frame ring consists of 12 frames in a 5x5 footprint:
	//   . F F F .
	//   F . . . F
	//   F . . . F
	//   F . . . F
	//   . F F F .
	// The 3x3 interior gets filled with end_portal blocks.
	//
	// We try all possible positions where (x,z) could be part of the ring.
	// The ring is at a fixed Y.

	// Possible portal ring offsets relative to the "corner" (NW corner of 5x5 footprint)
	type ringPos struct {
		dx, dz int
		facing block.Direction
	}

	// All 12 frame positions relative to the NW corner of the 5x5 bounding box
	ringPositions := []ringPos{
		// South-facing (top row, Z = corner.Z)
		{1, 0, block.South}, {2, 0, block.South}, {3, 0, block.South},
		// North-facing (bottom row, Z = corner.Z + 4)
		{1, 4, block.North}, {2, 4, block.North}, {3, 4, block.North},
		// East-facing (left column, X = corner.X)
		{0, 1, block.East}, {0, 2, block.East}, {0, 3, block.East},
		// West-facing (right column, X = corner.X + 4)
		{4, 1, block.West}, {4, 2, block.West}, {4, 3, block.West},
	}

	// Try each ring position as the one containing our frame
	for _, rp := range ringPositions {
		cornerX := x - rp.dx
		cornerZ := z - rp.dz

		if m.validatePortalRing(cornerX, y, cornerZ) {
			m.fillPortalInterior(cornerX, y, cornerZ)
			return true
		}
	}

	return false
}

// validatePortalRing checks if all 12 frames at the given corner position have eyes.
func (m *EndPortalManager) validatePortalRing(cornerX, y, cornerZ int) bool {
	type ringPos struct {
		dx, dz int
	}

	// All 12 frame positions
	allPositions := []ringPos{
		{1, 0}, {2, 0}, {3, 0},
		{1, 4}, {2, 4}, {3, 4},
		{0, 1}, {0, 2}, {0, 3},
		{4, 1}, {4, 2}, {4, 3},
	}

	for _, rp := range allPositions {
		fx := cornerX + rp.dx
		fz := cornerZ + rp.dz
		state, err := m.World.GetBlock(fx, y, fz)
		if err != nil {
			return false
		}
		if int(state) < 0 || int(state) >= len(block.StateList) || block.StateList[state] == nil {
			return false
		}
		frame, ok := block.StateList[state].(block.EndPortalFrame)
		if !ok {
			return false
		}
		if !frame.Eye {
			return false
		}
	}

	return true
}

// fillPortalInterior fills the 3x3 interior of the portal ring with end_portal blocks.
func (m *EndPortalManager) fillPortalInterior(cornerX, y, cornerZ int) {
	portalStateID, ok := block.ToStateID[block.EndPortal{}]
	if !ok {
		return
	}

	// Interior is at (cornerX+1..cornerX+3, y, cornerZ+1..cornerZ+3)
	for dz := 1; dz <= 3; dz++ {
		for dx := 1; dx <= 3; dx++ {
			px := cornerX + dx
			pz := cornerZ + dz
			m.World.SetBlock(px, y, pz, portalStateID)
			broadcastBlockUpdateStatic(m.Manager, px, y, pz, int32(portalStateID))
		}
	}

	// Play level event sound (end portal activation)
	BroadcastSound(m.Manager, SoundEndPortalAmbient, SoundCategoryBlock,
		float64(cornerX+2), float64(y), float64(cornerZ+2), 1.0, 1.0)
}

// TeleportToEnd sends a player from the Overworld to the End dimension.
func (m *EndPortalManager) TeleportToEnd(player *game.Player) {
	if m.DimensionMgr == nil {
		return
	}
	m.DimensionMgr.TeleportToEnd(player)
}

// TeleportFromEnd sends a player from the End back to the Overworld.
func (m *EndPortalManager) TeleportFromEnd(player *game.Player) {
	if m.DimensionMgr == nil {
		return
	}
	m.DimensionMgr.TeleportFromEnd(player)
}

// isEndPortalState returns true if the block state is an end portal block.
func isEndPortalState(stateID int) bool {
	return BlockNameFromState(stateID) == "end_portal"
}

// SoundEndPortalAmbient is the sound event for end portal activation.
const SoundEndPortalAmbient int32 = 55 // approximate; end portal ambient

// CheckPlayerInEndPortal checks if a player is standing in an end_portal block
// and teleports them to/from the End. Called from the dimension manager tick.
func (m *EndPortalManager) CheckPlayerInEndPortal(player *game.Player, world game.World) bool {
	if player.Dead || player.PortalCooldown > 0 {
		return false
	}

	px, py, pz := player.Position()
	footX := int(math.Floor(px))
	footY := int(math.Floor(py))
	footZ := int(math.Floor(pz))

	state, err := world.GetBlock(footX, footY, footZ)
	if err != nil {
		return false
	}

	if !isEndPortalState(int(state)) {
		return false
	}

	if player.Dimension == "minecraft:the_end" {
		m.TeleportFromEnd(player)
	} else {
		m.TeleportToEnd(player)
	}
	player.PortalCooldown = 300 // 15 seconds

	return true
}

func (m *EndPortalManager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}
