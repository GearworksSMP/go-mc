package handler

import (
	"strings"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// BlockUpdateManager handles neighbor notifications when a block changes.
// It checks support-dependent blocks, multi-block structures, and connection
// blocks, breaking or updating them as needed.
type BlockUpdateManager struct {
	World        game.World
	Manager      *game.PlayerManager
	FallingMgr   *FallingBlockManager
	FluidMgr     *FluidManager
	ItemEntities *ItemEntityManager
}

// NewBlockUpdateManager creates a new BlockUpdateManager.
func NewBlockUpdateManager(world game.World, manager *game.PlayerManager) *BlockUpdateManager {
	return &BlockUpdateManager{
		World:   world,
		Manager: manager,
	}
}

// maxUpdateDepth limits recursive neighbor notifications to prevent stack overflow
// from cascading breaks (e.g., a tall sugar cane column).
const maxUpdateDepth = 64

// NotifyNeighbors is called when a block at (x,y,z) changes. It checks all 6
// adjacent neighbors for blocks that depend on support and updates connection blocks.
func (m *BlockUpdateManager) NotifyNeighbors(x, y, z int) {
	m.notifyNeighbors(x, y, z, 0)
}

func (m *BlockUpdateManager) notifyNeighbors(x, y, z int, depth int) {
	if depth >= maxUpdateDepth {
		return
	}
	for _, off := range adjacentOffsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		state, err := m.World.GetBlock(nx, ny, nz)
		if err != nil || state == 0 {
			continue
		}
		name := BlockNameFromState(int(state))
		if name == "" {
			continue
		}

		// Check support-dependent blocks
		if m.checkSupportDependent(nx, ny, nz, state, name, depth) {
			continue
		}

		// Check multi-block structures
		m.checkMultiBlock(nx, ny, nz, state, name, depth)

		// Check connection blocks
		m.updateConnections(nx, ny, nz, state, name)

		// Concrete powder adjacent to water converts to concrete
		m.checkConcretePowder(nx, ny, nz, name)
	}
}

// checkSupportDependent checks if a block at (nx,ny,nz) has lost its support.
// Returns true if the block was broken.
func (m *BlockUpdateManager) checkSupportDependent(nx, ny, nz int, state block.StateID, name string, depth int) bool {
	if !m.needsSupport(name) {
		return false
	}
	if m.hasSupport(nx, ny, nz, state, name) {
		return false
	}
	m.breakAndDropAt(nx, ny, nz, state, name, depth)
	return true
}

// needsSupport returns true if the named block requires a support block.
func (m *BlockUpdateManager) needsSupport(name string) bool {
	switch {
	case name == "torch", name == "soul_torch":
		return true
	case name == "wall_torch", name == "soul_wall_torch", name == "redstone_wall_torch":
		return true
	case name == "lantern", name == "soul_lantern":
		return true
	case name == "ladder":
		return true
	case isLeverOrButton(name):
		return true
	case isStandingSign(name):
		return true
	case isWallSign(name):
		return true
	case isStandingBanner(name):
		return true
	case isWallBanner(name):
		return true
	case name == "vine":
		return true
	case name == "snow":
		return true
	case isCarpet(name):
		return true
	case isPressurePlate(name):
		return true
	case isRail(name):
		return true
	case name == "redstone_wire":
		return true
	case isSaplingOrFlower(name):
		return true
	case isCropOnFarmland(name):
		return true
	case name == "sugar_cane":
		return true
	case name == "cactus":
		return true
	case name == "redstone_torch":
		return true
	}
	return false
}

// hasSupport checks if the block at (nx,ny,nz) still has its required support.
func (m *BlockUpdateManager) hasSupport(nx, ny, nz int, state block.StateID, name string) bool {
	switch {
	case name == "torch", name == "soul_torch", name == "redstone_torch":
		return m.isSolidAt(nx, ny-1, nz)

	case name == "wall_torch", name == "soul_wall_torch", name == "redstone_wall_torch":
		return m.wallTorchHasSupport(nx, ny, nz, state)

	case name == "lantern", name == "soul_lantern":
		return m.lanternHasSupport(nx, ny, nz, state)

	case name == "ladder":
		return m.wallAttachedHasSupport(nx, ny, nz, state)

	case isLeverOrButton(name):
		return m.faceAttachedHasSupport(nx, ny, nz, state)

	case isStandingSign(name), isStandingBanner(name):
		return m.isSolidAt(nx, ny-1, nz)

	case isWallSign(name):
		return m.wallAttachedHasSupport(nx, ny, nz, state)

	case isWallBanner(name):
		return m.wallBannerHasSupport(nx, ny, nz, state)

	case name == "vine":
		return m.isSolidAt(nx, ny+1, nz)

	case name == "snow", isCarpet(name), isPressurePlate(name), isRail(name),
		name == "redstone_wire":
		return m.isSolidAt(nx, ny-1, nz)

	case isSaplingOrFlower(name):
		return m.isDirtLike(nx, ny-1, nz)

	case isCropOnFarmland(name):
		return m.isFarmlandAt(nx, ny-1, nz)

	case name == "sugar_cane":
		return m.sugarCaneHasSupport(nx, ny, nz)

	case name == "cactus":
		return m.cactusHasSupport(nx, ny, nz)
	}
	return true
}

// wallTorchHasSupport checks if a wall torch has the block it's attached to.
func (m *BlockUpdateManager) wallTorchHasSupport(nx, ny, nz int, state block.StateID) bool {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return true
	}
	// Try all wall torch types
	var facing block.Direction
	switch t := block.StateList[state].(type) {
	case block.WallTorch:
		facing = t.Facing
	case block.SoulWallTorch:
		facing = t.Facing
	case block.RedstoneWallTorch:
		facing = t.Facing
	default:
		return true
	}
	dx, dz := dirToOffset(facing)
	return m.isSolidAt(nx-dx, ny, nz-dz)
}

// lanternHasSupport checks if a lantern has the block it hangs from or sits on.
func (m *BlockUpdateManager) lanternHasSupport(nx, ny, nz int, state block.StateID) bool {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return true
	}
	var hanging bool
	switch t := block.StateList[state].(type) {
	case block.Lantern:
		hanging = bool(t.Hanging)
	case block.SoulLantern:
		hanging = bool(t.Hanging)
	default:
		return true
	}
	if hanging {
		return m.isSolidAt(nx, ny+1, nz)
	}
	return m.isSolidAt(nx, ny-1, nz)
}

// wallAttachedHasSupport checks if a wall-attached block (ladder, wall sign) has support.
func (m *BlockUpdateManager) wallAttachedHasSupport(nx, ny, nz int, state block.StateID) bool {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return true
	}
	var facing block.Direction
	switch t := block.StateList[state].(type) {
	case block.OakWallSign:
		facing = t.Facing
	case block.SpruceWallSign:
		facing = t.Facing
	case block.BirchWallSign:
		facing = t.Facing
	case block.AcaciaWallSign:
		facing = t.Facing
	case block.CherryWallSign:
		facing = t.Facing
	case block.JungleWallSign:
		facing = t.Facing
	case block.DarkOakWallSign:
		facing = t.Facing
	case block.MangroveWallSign:
		facing = t.Facing
	case block.BambooWallSign:
		facing = t.Facing
	case block.CrimsonWallSign:
		facing = t.Facing
	case block.WarpedWallSign:
		facing = t.Facing
	case block.OakWallHangingSign:
		facing = t.Facing
	case block.SpruceWallHangingSign:
		facing = t.Facing
	case block.BirchWallHangingSign:
		facing = t.Facing
	case block.AcaciaWallHangingSign:
		facing = t.Facing
	case block.CherryWallHangingSign:
		facing = t.Facing
	case block.JungleWallHangingSign:
		facing = t.Facing
	case block.DarkOakWallHangingSign:
		facing = t.Facing
	case block.MangroveWallHangingSign:
		facing = t.Facing
	case block.CrimsonWallHangingSign:
		facing = t.Facing
	case block.WarpedWallHangingSign:
		facing = t.Facing
	case block.BambooWallHangingSign:
		facing = t.Facing
	case block.Ladder:
		facing = t.Facing
	default:
		return true
	}
	dx, dz := dirToOffset(facing)
	return m.isSolidAt(nx-dx, ny, nz-dz)
}

// wallBannerHasSupport checks if a wall banner has the block it's attached to.
func (m *BlockUpdateManager) wallBannerHasSupport(nx, ny, nz int, state block.StateID) bool {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return true
	}
	var facing block.Direction
	switch t := block.StateList[state].(type) {
	case block.WhiteWallBanner:
		facing = t.Facing
	case block.OrangeWallBanner:
		facing = t.Facing
	case block.MagentaWallBanner:
		facing = t.Facing
	case block.LightBlueWallBanner:
		facing = t.Facing
	case block.YellowWallBanner:
		facing = t.Facing
	case block.LimeWallBanner:
		facing = t.Facing
	case block.PinkWallBanner:
		facing = t.Facing
	case block.GrayWallBanner:
		facing = t.Facing
	case block.LightGrayWallBanner:
		facing = t.Facing
	case block.CyanWallBanner:
		facing = t.Facing
	case block.PurpleWallBanner:
		facing = t.Facing
	case block.BlueWallBanner:
		facing = t.Facing
	case block.BrownWallBanner:
		facing = t.Facing
	case block.GreenWallBanner:
		facing = t.Facing
	case block.RedWallBanner:
		facing = t.Facing
	case block.BlackWallBanner:
		facing = t.Facing
	default:
		return true
	}
	dx, dz := dirToOffset(facing)
	return m.isSolidAt(nx-dx, ny, nz-dz)
}

// faceAttachedHasSupport checks if a lever/button has the block it's attached to.
func (m *BlockUpdateManager) faceAttachedHasSupport(nx, ny, nz int, state block.StateID) bool {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return true
	}
	var face block.AttachFace
	var facing block.Direction
	switch t := block.StateList[state].(type) {
	case block.Lever:
		face = t.Face
		facing = t.Facing
	case block.StoneButton:
		face = t.Face
		facing = t.Facing
	case block.OakButton:
		face = t.Face
		facing = t.Facing
	case block.SpruceButton:
		face = t.Face
		facing = t.Facing
	case block.BirchButton:
		face = t.Face
		facing = t.Facing
	case block.JungleButton:
		face = t.Face
		facing = t.Facing
	case block.AcaciaButton:
		face = t.Face
		facing = t.Facing
	case block.CherryButton:
		face = t.Face
		facing = t.Facing
	case block.DarkOakButton:
		face = t.Face
		facing = t.Facing
	case block.MangroveButton:
		face = t.Face
		facing = t.Facing
	case block.BambooButton:
		face = t.Face
		facing = t.Facing
	case block.CrimsonButton:
		face = t.Face
		facing = t.Facing
	case block.WarpedButton:
		face = t.Face
		facing = t.Facing
	case block.PolishedBlackstoneButton:
		face = t.Face
		facing = t.Facing
	default:
		return true
	}
	switch face {
	case block.AttachFaceFloor:
		return m.isSolidAt(nx, ny-1, nz)
	case block.AttachFaceCeiling:
		return m.isSolidAt(nx, ny+1, nz)
	case block.AttachFaceWall:
		dx, dz := dirToOffset(facing)
		return m.isSolidAt(nx-dx, ny, nz-dz)
	}
	return true
}

// sugarCaneHasSupport checks if sugar cane has a valid support block.
func (m *BlockUpdateManager) sugarCaneHasSupport(nx, ny, nz int) bool {
	below, err := m.World.GetBlock(nx, ny-1, nz)
	if err != nil {
		return false
	}
	belowName := BlockNameFromState(int(below))
	switch belowName {
	case "sugar_cane":
		return true
	case "sand", "red_sand", "dirt", "coarse_dirt", "podzol",
		"grass_block", "mycelium", "mud", "rooted_dirt", "moss_block":
		return true
	}
	return false
}

// cactusHasSupport checks if a cactus has valid support and no adjacent solid blocks.
func (m *BlockUpdateManager) cactusHasSupport(nx, ny, nz int) bool {
	// Must be on cactus or sand
	below, err := m.World.GetBlock(nx, ny-1, nz)
	if err != nil {
		return false
	}
	belowName := BlockNameFromState(int(below))
	if belowName != "cactus" && belowName != "sand" && belowName != "red_sand" {
		return false
	}
	// No adjacent solid blocks (horizontal)
	for _, off := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		adj, err := m.World.GetBlock(nx+off[0], ny, nz+off[1])
		if err != nil {
			continue
		}
		if isSolidBlock(adj) {
			return false
		}
	}
	return true
}

// checkMultiBlock checks multi-block structures and breaks the other part if needed.
func (m *BlockUpdateManager) checkMultiBlock(nx, ny, nz int, state block.StateID, name string, depth int) {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return
	}
	if isDoor(name) {
		m.checkDoor(nx, ny, nz, state, depth)
		return
	}
	if isTallPlant(name) {
		m.checkTallPlant(nx, ny, nz, state, name, depth)
		return
	}
}

// checkDoor breaks a door half if the other half is missing.
func (m *BlockUpdateManager) checkDoor(nx, ny, nz int, state block.StateID, depth int) {
	s := block.StateList[state]
	var half block.DoubleBlockHalf
	switch t := s.(type) {
	case block.OakDoor:
		half = t.Half
	case block.IronDoor:
		half = t.Half
	case block.SpruceDoor:
		half = t.Half
	case block.BirchDoor:
		half = t.Half
	case block.JungleDoor:
		half = t.Half
	case block.AcaciaDoor:
		half = t.Half
	case block.CherryDoor:
		half = t.Half
	case block.DarkOakDoor:
		half = t.Half
	case block.MangroveDoor:
		half = t.Half
	case block.BambooDoor:
		half = t.Half
	case block.CrimsonDoor:
		half = t.Half
	case block.WarpedDoor:
		half = t.Half
	case block.CopperDoor:
		half = t.Half
	case block.ExposedCopperDoor:
		half = t.Half
	case block.WeatheredCopperDoor:
		half = t.Half
	case block.OxidizedCopperDoor:
		half = t.Half
	case block.WaxedCopperDoor:
		half = t.Half
	case block.WaxedExposedCopperDoor:
		half = t.Half
	case block.WaxedWeatheredCopperDoor:
		half = t.Half
	case block.WaxedOxidizedCopperDoor:
		half = t.Half
	default:
		return
	}

	otherY := ny
	if half == block.DoubleBlockHalfUpper {
		otherY = ny - 1
	} else {
		otherY = ny + 1
	}

	otherState, err := m.World.GetBlock(nx, otherY, nz)
	if err != nil {
		return
	}
	otherName := BlockNameFromState(int(otherState))
	if !isDoor(otherName) {
		// Other half is gone — break this half
		m.breakAndDropAt(nx, ny, nz, state, BlockNameFromState(int(state)), depth)
	}
}

// checkTallPlant breaks a tall plant half if the other half is missing.
func (m *BlockUpdateManager) checkTallPlant(nx, ny, nz int, state block.StateID, name string, depth int) {
	s := block.StateList[state]
	var half block.DoubleBlockHalf
	switch t := s.(type) {
	case block.Sunflower:
		half = t.Half
	case block.Lilac:
		half = t.Half
	case block.RoseBush:
		half = t.Half
	case block.Peony:
		half = t.Half
	case block.TallGrass:
		half = t.Half
	case block.LargeFern:
		half = t.Half
	default:
		return
	}

	otherY := ny
	if half == block.DoubleBlockHalfUpper {
		otherY = ny - 1
	} else {
		otherY = ny + 1
	}

	otherState, err := m.World.GetBlock(nx, otherY, nz)
	if err != nil {
		return
	}
	otherName := BlockNameFromState(int(otherState))
	if !isTallPlant(otherName) {
		// Other half is gone — break this half too
		m.breakAndDropAt(nx, ny, nz, state, name, depth)
	}
}

// updateConnections updates connection properties for fences, walls, glass panes, iron bars.
func (m *BlockUpdateManager) updateConnections(nx, ny, nz int, state block.StateID, name string) {
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return
	}
	switch {
	case isFence(name):
		m.updateFenceConnections(nx, ny, nz, state, name)
	case isWall(name):
		m.updateWallConnections(nx, ny, nz, state, name)
	case isPaneOrBars(name):
		m.updatePaneConnections(nx, ny, nz, state, name)
	}
}

// updateFenceConnections updates fence NSEW connection properties.
func (m *BlockUpdateManager) updateFenceConnections(nx, ny, nz int, state block.StateID, name string) {
	north := m.shouldFenceConnect(nx, ny, nz-1)
	south := m.shouldFenceConnect(nx, ny, nz+1)
	east := m.shouldFenceConnect(nx+1, ny, nz)
	west := m.shouldFenceConnect(nx-1, ny, nz)

	s := block.StateList[state]
	var newState block.Block
	switch t := s.(type) {
	case block.OakFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.SpruceFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.BirchFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.JungleFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.AcaciaFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.CherryFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.DarkOakFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.MangroveFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.BambooFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.NetherBrickFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.CrimsonFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.WarpedFence:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	default:
		return
	}
	m.setBlockState(nx, ny, nz, newState)
}

// updateWallConnections updates wall NSEW connection properties.
func (m *BlockUpdateManager) updateWallConnections(nx, ny, nz int, state block.StateID, name string) {
	north := m.wallSideFor(nx, ny, nz-1)
	south := m.wallSideFor(nx, ny, nz+1)
	east := m.wallSideFor(nx+1, ny, nz)
	west := m.wallSideFor(nx-1, ny, nz)
	up := m.isSolidAt(nx, ny+1, nz) || (north != block.WallSideNone && south != block.WallSideNone) ||
		(east != block.WallSideNone && west != block.WallSideNone)

	s := block.StateList[state]
	var newState block.Block
	switch t := s.(type) {
	case block.CobblestoneWall:
		t.North = north
		t.South = south
		t.East = east
		t.West = west
		t.Up = block.Boolean(up)
		newState = t
	case block.MossyCobblestoneWall:
		t.North = north
		t.South = south
		t.East = east
		t.West = west
		t.Up = block.Boolean(up)
		newState = t
	default:
		// Many wall types share the same pattern; handle generically via name match
		return
	}
	m.setBlockState(nx, ny, nz, newState)
}

// updatePaneConnections updates glass pane / iron bars NSEW connections.
func (m *BlockUpdateManager) updatePaneConnections(nx, ny, nz int, state block.StateID, name string) {
	north := m.shouldPaneConnect(nx, ny, nz-1)
	south := m.shouldPaneConnect(nx, ny, nz+1)
	east := m.shouldPaneConnect(nx+1, ny, nz)
	west := m.shouldPaneConnect(nx-1, ny, nz)

	s := block.StateList[state]
	var newState block.Block
	switch t := s.(type) {
	case block.IronBars:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	case block.GlassPane:
		t.North = block.Boolean(north)
		t.South = block.Boolean(south)
		t.East = block.Boolean(east)
		t.West = block.Boolean(west)
		newState = t
	default:
		// Stained glass panes
		return
	}
	m.setBlockState(nx, ny, nz, newState)
}

// breakAndDrop removes a block, broadcasts the change, and spawns an item drop.
// depth tracks recursion depth to prevent stack overflow.
func (m *BlockUpdateManager) breakAndDropAt(x, y, z int, state block.StateID, name string, depth int) {
	m.World.SetBlock(x, y, z, 0)
	broadcastBlockUpdateDirect(m.Manager, x, y, z, 0)

	// Spawn break particles
	BroadcastLevelEvent(m.Manager, 2001, x, y, z, int32(state))

	// Spawn item entity for the dropped block
	if m.ItemEntities != nil && name != "" {
		dropName := blockDropName(name)
		if id := itemIDByName(dropName); id > 0 {
			m.ItemEntities.SpawnItem(m.Manager,
				float64(x)+0.5, float64(y)+0.5, float64(z)+0.5,
				id, 1, 10)
		}
	}

	// Check for falling blocks above the now-empty space
	if m.FallingMgr != nil {
		m.FallingMgr.CheckAndSpawnFalling(x, y+1, z)
	}

	// Schedule fluid flow into the space
	if m.FluidMgr != nil {
		m.FluidMgr.CheckFlowDown(x, y+1, z)
	}

	// Recursively notify neighbors of this broken block
	m.notifyNeighbors(x, y, z, depth+1)
}

// checkConcretePowder converts concrete powder to concrete if adjacent to water.
func (m *BlockUpdateManager) checkConcretePowder(nx, ny, nz int, name string) {
	if !strings.HasSuffix(name, "_concrete_powder") {
		return
	}
	if !m.hasAdjacentWater(nx, ny, nz) {
		return
	}
	// Convert to concrete (same color)
	concreteName := strings.TrimSuffix(name, "_powder")
	newBlock, ok := block.FromID["minecraft:"+concreteName]
	if !ok {
		return
	}
	newID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}
	m.World.SetBlock(nx, ny, nz, newID)
	broadcastBlockUpdateDirect(m.Manager, nx, ny, nz, int32(newID))
}

// --- Helper functions ---

// isSolidAt returns true if the block at (x,y,z) is solid.
func (m *BlockUpdateManager) isSolidAt(x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	return isSolidBlock(state)
}

// isDirtLike returns true if the block at (x,y,z) is dirt/grass/similar.
func (m *BlockUpdateManager) isDirtLike(x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	switch name {
	case "dirt", "coarse_dirt", "rooted_dirt", "grass_block", "podzol",
		"mycelium", "mud", "muddy_mangrove_roots", "moss_block", "farmland":
		return true
	}
	return false
}

// isFarmlandAt returns true if the block at (x,y,z) is farmland.
func (m *BlockUpdateManager) isFarmlandAt(x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	return BlockNameFromState(int(state)) == "farmland"
}

// hasAdjacentWater checks if any of the 6 neighbors is water.
func (m *BlockUpdateManager) hasAdjacentWater(x, y, z int) bool {
	for _, off := range adjacentOffsets {
		state, err := m.World.GetBlock(x+off[0], y+off[1], z+off[2])
		if err != nil {
			continue
		}
		name := BlockNameFromState(int(state))
		if name == "water" {
			return true
		}
	}
	return false
}

// shouldFenceConnect returns true if a fence should connect to the block at (x,y,z).
func (m *BlockUpdateManager) shouldFenceConnect(x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	if isFence(name) || isWall(name) {
		return true
	}
	return isSolidBlock(state)
}

// shouldPaneConnect returns true if a pane/bars should connect to the block at (x,y,z).
func (m *BlockUpdateManager) shouldPaneConnect(x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	if isPaneOrBars(name) {
		return true
	}
	return isSolidBlock(state)
}

// wallSideFor returns the WallSide for a wall connecting to the block at (x,y,z).
func (m *BlockUpdateManager) wallSideFor(x, y, z int) block.WallSide {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return block.WallSideNone
	}
	name := BlockNameFromState(int(state))
	if isWall(name) || isSolidBlock(state) {
		return block.WallSideLow
	}
	return block.WallSideNone
}

// setBlockState sets a block from a block.Block value.
func (m *BlockUpdateManager) setBlockState(x, y, z int, b block.Block) {
	newID, ok := block.ToStateID[b]
	if !ok {
		return
	}
	m.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newID))
}

// dirToOffset returns the (dx, dz) offset for a facing direction.
// The offset points in the direction the block is facing.
func dirToOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, -1
	case block.South:
		return 0, 1
	case block.East:
		return 1, 0
	case block.West:
		return -1, 0
	}
	return 0, 0
}

// blockDropName maps block names to their dropped item name.
func blockDropName(name string) string {
	// Most blocks drop themselves
	switch name {
	case "wall_torch", "soul_wall_torch":
		return strings.TrimPrefix(name, "wall_")
	case "redstone_wall_torch":
		return "redstone_torch"
	}
	// Wall signs drop the sign item
	if strings.HasPrefix(name, "wall_") {
		return strings.TrimPrefix(name, "wall_")
	}
	if strings.Contains(name, "_wall_sign") {
		return strings.Replace(name, "_wall_sign", "_sign", 1)
	}
	if strings.Contains(name, "_wall_banner") {
		return strings.Replace(name, "_wall_banner", "_banner", 1)
	}
	if strings.Contains(name, "_wall_hanging_sign") {
		return strings.Replace(name, "_wall_hanging_sign", "_hanging_sign", 1)
	}
	return name
}

// --- Block category helpers ---

func isLeverOrButton(name string) bool {
	if name == "lever" {
		return true
	}
	return strings.HasSuffix(name, "_button")
}

func isStandingSign(name string) bool {
	if strings.HasSuffix(name, "_sign") && !strings.Contains(name, "wall") && !strings.Contains(name, "hanging") {
		return true
	}
	return false
}

func isWallSign(name string) bool {
	return strings.Contains(name, "_wall_sign") || strings.Contains(name, "_wall_hanging_sign")
}

func isStandingBanner(name string) bool {
	return strings.HasSuffix(name, "_banner") && !strings.Contains(name, "wall")
}

func isWallBanner(name string) bool {
	return strings.Contains(name, "_wall_banner")
}

func isCarpet(name string) bool {
	return strings.HasSuffix(name, "_carpet")
}

func isPressurePlate(name string) bool {
	return strings.HasSuffix(name, "_pressure_plate")
}

func isRail(name string) bool {
	switch name {
	case "rail", "powered_rail", "detector_rail", "activator_rail":
		return true
	}
	return false
}

func isSaplingOrFlower(name string) bool {
	switch name {
	case "oak_sapling", "spruce_sapling", "birch_sapling", "jungle_sapling",
		"acacia_sapling", "cherry_sapling", "dark_oak_sapling",
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet",
		"red_tulip", "orange_tulip", "white_tulip", "pink_tulip",
		"oxeye_daisy", "cornflower", "lily_of_the_valley", "wither_rose",
		"torchflower", "short_grass", "fern", "dead_bush",
		"red_mushroom", "brown_mushroom", "sweet_berry_bush":
		return true
	}
	return false
}

func isCropOnFarmland(name string) bool {
	switch name {
	case "wheat", "carrots", "potatoes", "beetroots", "torchflower_crop":
		return true
	}
	return false
}

func isDoor(name string) bool {
	return strings.HasSuffix(name, "_door")
}

func isTallPlant(name string) bool {
	switch name {
	case "sunflower", "lilac", "rose_bush", "peony", "tall_grass", "large_fern":
		return true
	}
	return false
}

func isFence(name string) bool {
	return strings.HasSuffix(name, "_fence") && !strings.HasSuffix(name, "_fence_gate")
}

func isWall(name string) bool {
	return strings.HasSuffix(name, "_wall") && !strings.Contains(name, "sign") &&
		!strings.Contains(name, "banner") && !strings.Contains(name, "torch")
}

func isPaneOrBars(name string) bool {
	if name == "iron_bars" || name == "glass_pane" {
		return true
	}
	return strings.HasSuffix(name, "_stained_glass_pane")
}
