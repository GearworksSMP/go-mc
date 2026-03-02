package handler

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// PistonManager handles piston push/pull mechanics.
type PistonManager struct {
	Manager *game.PlayerManager
	World   game.World
}

// NewPistonManager creates a new piston manager.
func NewPistonManager(mgr *game.PlayerManager, world game.World) *PistonManager {
	return &PistonManager{Manager: mgr, World: world}
}

// maxPistonPush is the maximum number of blocks a piston can push.
const maxPistonPush = 12

// TryActivate attempts to extend a piston when it receives power.
func (pm *PistonManager) TryActivate(x, y, z int) {
	stateID, err := pm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var facing block.Direction
	var sticky bool

	switch p := block.StateList[stateID].(type) {
	case block.Piston:
		if bool(p.Extended) {
			return
		}
		facing = p.Facing
		sticky = false
	case block.StickyPiston:
		if bool(p.Extended) {
			return
		}
		facing = p.Facing
		sticky = true
	default:
		return
	}

	dx, dy, dz := directionOffset(facing)

	// Collect blocks to push
	blocks := pm.collectPushChain(x+dx, y+dy, z+dz, dx, dy, dz)
	if blocks == nil {
		return // can't push (immovable block or too many)
	}

	// Move blocks: start from the farthest and work back
	for i := len(blocks) - 1; i >= 0; i-- {
		bx, by, bz := blocks[i][0], blocks[i][1], blocks[i][2]
		state, err := pm.World.GetBlock(bx, by, bz)
		if err != nil {
			continue
		}
		// Move block forward
		pm.World.SetBlock(bx+dx, by+dy, bz+dz, state)
		broadcastBlockUpdateDirect(pm.Manager, bx+dx, by+dy, bz+dz, int32(state))
		pm.World.SetBlock(bx, by, bz, 0)
		broadcastBlockUpdateDirect(pm.Manager, bx, by, bz, 0)
	}

	// Set piston to extended state
	pm.setExtended(x, y, z, facing, sticky, true)

	// Place piston head
	pm.placePistonHead(x+dx, y+dy, z+dz, facing, sticky)

	_ = sticky // used by placePistonHead
}

// TryRetract retracts a piston when it loses power.
func (pm *PistonManager) TryRetract(x, y, z int) {
	stateID, err := pm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var facing block.Direction
	var sticky bool

	switch p := block.StateList[stateID].(type) {
	case block.Piston:
		if !bool(p.Extended) {
			return
		}
		facing = p.Facing
		sticky = false
	case block.StickyPiston:
		if !bool(p.Extended) {
			return
		}
		facing = p.Facing
		sticky = true
	default:
		return
	}

	dx, dy, dz := directionOffset(facing)

	// Remove piston head
	pm.World.SetBlock(x+dx, y+dy, z+dz, 0)
	broadcastBlockUpdateDirect(pm.Manager, x+dx, y+dy, z+dz, 0)

	// Sticky piston: pull block back
	if sticky {
		pullX, pullY, pullZ := x+dx*2, y+dy*2, z+dz*2
		pullState, err := pm.World.GetBlock(pullX, pullY, pullZ)
		if err == nil && pullState != 0 {
			pullName := BlockNameFromState(int(pullState))
			if !isImmovable(pullName) {
				pm.World.SetBlock(x+dx, y+dy, z+dz, pullState)
				broadcastBlockUpdateDirect(pm.Manager, x+dx, y+dy, z+dz, int32(pullState))
				pm.World.SetBlock(pullX, pullY, pullZ, 0)
				broadcastBlockUpdateDirect(pm.Manager, pullX, pullY, pullZ, 0)
			}
		}
	}

	// Set piston to retracted state
	pm.setExtended(x, y, z, facing, sticky, false)
}

// collectPushChain collects the chain of blocks that would be pushed.
// Returns nil if the chain is too long or contains an immovable block.
func (pm *PistonManager) collectPushChain(startX, startY, startZ, dx, dy, dz int) [][3]int {
	var chain [][3]int
	x, y, z := startX, startY, startZ

	for i := 0; i < maxPistonPush; i++ {
		state, err := pm.World.GetBlock(x, y, z)
		if err != nil || state == 0 {
			return chain // air = end of chain
		}

		name := BlockNameFromState(int(state))
		if isImmovable(name) {
			return nil // can't push
		}

		chain = append(chain, [3]int{x, y, z})
		x += dx
		y += dy
		z += dz
	}

	// Check the block beyond the chain
	state, err := pm.World.GetBlock(x, y, z)
	if err != nil || state != 0 {
		return nil // too many blocks or blocked
	}
	return chain
}

// isImmovable returns true if a block can't be moved by pistons.
func isImmovable(name string) bool {
	switch name {
	case "bedrock", "obsidian", "end_portal_frame", "end_portal",
		"spawner", "enchanting_table", "ender_chest", "beacon",
		"command_block", "barrier", "reinforced_deepslate":
		return true
	}
	return false
}

// directionOffset returns the XYZ offset for a block.Direction.
func directionOffset(d block.Direction) (dx, dy, dz int) {
	switch d {
	case block.Up:
		return 0, 1, 0
	case block.Down:
		return 0, -1, 0
	case block.North:
		return 0, 0, -1
	case block.South:
		return 0, 0, 1
	case block.East:
		return 1, 0, 0
	case block.West:
		return -1, 0, 0
	}
	return 0, 0, 0
}

// setExtended updates the piston block to extended/retracted state.
func (pm *PistonManager) setExtended(x, y, z int, facing block.Direction, sticky, extended bool) {
	var newBlock block.Block
	if sticky {
		newBlock = block.StickyPiston{Extended: block.Boolean(extended), Facing: facing}
	} else {
		newBlock = block.Piston{Extended: block.Boolean(extended), Facing: facing}
	}
	if newID, ok := block.ToStateID[newBlock]; ok {
		pm.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(pm.Manager, x, y, z, int32(newID))
	}
}

// placePistonHead places a piston head block.
func (pm *PistonManager) placePistonHead(x, y, z int, facing block.Direction, sticky bool) {
	headType := block.PistonTypeNormal
	if sticky {
		headType = block.PistonTypeSticky
	}
	head := block.PistonHead{Facing: facing, Short: false, Type: headType}
	if newID, ok := block.ToStateID[head]; ok {
		pm.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(pm.Manager, x, y, z, int32(newID))
	}
}
