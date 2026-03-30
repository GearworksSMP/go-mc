package handler

import (
	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// blockToItemID maps a block name (e.g. "minecraft:dirt") to its item ID.
func blockToItemID(blockName string) (int32, bool) {
	// Strip "minecraft:" prefix for matching against item names
	name := blockName
	if len(name) > 10 && name[:10] == "minecraft:" {
		name = name[10:]
	}
	for id, itm := range item.ByID {
		if itm.Name == name {
			return int32(id), true
		}
	}
	return 0, false
}

// orientBlock adjusts a block's state based on player facing and clicked face.
// Returns the oriented block state ID, or the original if no orientation applies.
func orientBlock(player *game.Player, state int, face int) int {
	if state < 0 || state >= len(block.StateList) || block.StateList[state] == nil {
		return state
	}

	yaw, pitch := player.Rotation()
	facing := yawToDirection(yaw)

	b := block.StateList[state]
	var oriented block.Block

	switch v := b.(type) {
	// Stairs: face toward player, half based on click position
	case block.OakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobblestoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneBrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SpruceStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BirchStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.JungleStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.AcaciaStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.DarkOakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SandstoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobbledDeepslateStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v

	// Logs: axis based on clicked face
	case block.OakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.SpruceLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.BirchLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.JungleLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.AcaciaLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.DarkOakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.CherryLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.MangroveLog:
		v.Axis = faceToAxis(face)
		oriented = v

	// Slabs: type based on click position
	case block.StoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobblestoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.StoneBrickSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SandstoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.OakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SpruceSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.BirchSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.JungleSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.AcaciaSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.DarkOakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobbledDeepslateSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v

	// Furnace, chest facing
	case block.Furnace:
		v.Facing = facing
		oriented = v
	case block.Chest:
		v.Facing = facing
		oriented = v

	// Pumpkin/carved pumpkin
	case block.CarvedPumpkin:
		v.Facing = facing
		oriented = v

	// Glazed terracotta
	case block.WhiteGlazedTerracotta:
		v.Facing = facing
		oriented = v

	// Observer
	case block.Observer:
		v.Facing = yawPitchToFacing6(yaw, pitch)
		oriented = v

	default:
		return state
	}

	if newID, ok := block.ToStateID[oriented]; ok {
		return int(newID)
	}
	return state
}

// faceToAxis converts a clicked face index to a log axis.
func faceToAxis(face int) block.Axis {
	switch face {
	case 0, 1: // bottom, top
		return block.Y
	case 2, 3: // north, south
		return block.Z
	case 4, 5: // west, east
		return block.X
	}
	return block.Y
}

// yawPitchToFacing6 converts yaw+pitch to a 6-direction facing (includes up/down).
func yawPitchToFacing6(yaw, pitch float32) block.Direction {
	if pitch > 45 {
		return block.Down
	}
	if pitch < -45 {
		return block.Up
	}
	return yawToDirection(yaw)
}

// placeBlock places a block and broadcasts the change.
func (h *BlockHandler) placeBlock(player *game.Player, x, y, z int, state level.BlocksState, sequence int32) {
	_, err := h.World.SetBlock(x, y, z, state)
	if err != nil {
		h.logf("Error placing block at (%d,%d,%d): %v", x, y, z, err)
		h.sendAck(player, sequence)
		return
	}

	h.broadcastBlockUpdate(x, y, z, int32(state))
	h.sendAck(player, sequence)

	// Play block place sound
	placedBlockName := BlockNameFromState(int(state))
	BroadcastSound(h.Manager, BlockPlaceSound(placedBlockName), SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 0.8)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Register sapling for tree growth
	if h.TreeMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isSapling(placedName) {
			h.TreeMgr.RegisterSapling(x, y, z)
		}
	}

	// Check if placed block should fall
	if h.FallingMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isFallingBlock(placedName) {
			// Check if block below is air
			belowState, err := h.World.GetBlock(x, y-1, z)
			if err == nil && belowState == 0 {
				h.FallingMgr.CheckAndSpawnFalling(x, y, z)
			}
		}
	}

	// Try linking adjacent chests as double chest
	if h.Chests != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "chest" {
			h.Chests.TryLinkDouble(x, y, z, h.World, h.Manager)
		}
	}

	// Notify observers of block change
	if h.RedstoneMgr != nil {
		h.RedstoneMgr.NotifyBlockChange(x, y, z)
	}

	// Track daylight detector placement
	if h.RedstoneMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "daylight_detector" {
			h.RedstoneMgr.TrackDaylightDetector(x, y, z)
		}
	}

	// Track beacon placement
	if h.BeaconMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "beacon" {
			h.BeaconMgr.TrackBeacon(x, y, z)
		}
	}

	// Check wither summoning when placing wither_skeleton_skull
	if h.WitherMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "wither_skeleton_skull" || placedName == "wither_skeleton_wall_skull" {
			h.WitherMgr.CheckWitherSummon(x, y, z)
		}
	}

	// Notify neighbors for connection updates and support checks
	if h.BlockUpdateMgr != nil {
		h.BlockUpdateMgr.NotifyNeighbors(x, y, z)
	}

	h.logf("Player %s placed block at (%d, %d, %d) state=%d", player.Name, x, y, z, state)
}

// yawToDirection converts player yaw to a block.Direction for placement.
func yawToDirection(yaw float32) block.Direction {
	// Normalize yaw to 0-360
	y := float64(yaw)
	for y < 0 {
		y += 360
	}
	for y >= 360 {
		y -= 360
	}
	// Player faces: 0=south, 90=west, 180=north, 270=east
	switch {
	case y >= 315 || y < 45:
		return block.South
	case y >= 45 && y < 135:
		return block.West
	case y >= 135 && y < 225:
		return block.North
	default:
		return block.East
	}
}

// isDoorItem returns true if the item name is a door item.
func isDoorItem(name string) bool {
	switch name {
	case "oak_door", "spruce_door", "birch_door", "jungle_door",
		"acacia_door", "dark_oak_door", "cherry_door", "mangrove_door",
		"bamboo_door", "crimson_door", "warped_door", "iron_door":
		return true
	}
	return false
}

// placeDoor places both halves of a door at the given position.
func (h *BlockHandler) placeDoor(player *game.Player, x, y, z int, doorName string, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	var lowerBlock, upperBlock block.Block

	switch doorName {
	case "oak_door":
		lowerBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "spruce_door":
		lowerBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "birch_door":
		lowerBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "jungle_door":
		lowerBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "acacia_door":
		lowerBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "dark_oak_door":
		lowerBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "cherry_door":
		lowerBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "mangrove_door":
		lowerBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "bamboo_door":
		lowerBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "crimson_door":
		lowerBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "warped_door":
		lowerBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "iron_door":
		lowerBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	default:
		return
	}

	lowerID, ok1 := block.ToStateID[lowerBlock]
	upperID, ok2 := block.ToStateID[upperBlock]
	if !ok1 || !ok2 {
		return
	}

	h.World.SetBlock(x, y, z, lowerID)
	h.World.SetBlock(x, y+1, z, upperID)
	h.broadcastBlockUpdate(x, y, z, int32(lowerID))
	h.broadcastBlockUpdate(x, y+1, z, int32(upperID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}
}

// placeSign handles sign placement. Standing signs are placed on top of blocks
// (face 1), wall signs on side faces (face 2-5).
func (h *BlockHandler) placeSign(player *game.Player, x, y, z int, itemName string, face int, sequence int32) {
	// Check for player collision
	if h.wouldCollideWithPlayer(x, y, z) {
		h.broadcastBlockUpdate(x, y, z, 0)
		h.sendAck(player, sequence)
		return
	}

	var signBlock block.Block

	if face >= 2 && face <= 5 {
		// Wall sign: placed on the side of a block
		dir, ok := faceToWallSignDirection(face)
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		wallName := wallSignBlockForItem(itemName)
		if wallName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = wallSignWithFacing(wallName, dir)
	} else {
		// Standing sign: placed on top of a block
		yaw, _ := player.Rotation()
		rotation := yawToSignRotation(yaw)
		standingName := signBlockForItem(itemName)
		if standingName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = standingSignWithRotation(standingName, rotation)
	}

	if signBlock == nil {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := block.ToStateID[signBlock]
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Open sign editor
	h.SignMgr.PlaceSign(player, x, y, z)

	h.logf("Player %s placed sign at (%d, %d, %d)", player.Name, x, y, z)
}

// standingSignWithRotation creates a standing sign block with the given rotation.
func standingSignWithRotation(blockName string, rotation int) block.Block {
	rot := block.Integer(rotation)
	switch blockName {
	case "minecraft:oak_sign":
		return block.OakSign{Rotation: rot}
	case "minecraft:spruce_sign":
		return block.SpruceSign{Rotation: rot}
	case "minecraft:birch_sign":
		return block.BirchSign{Rotation: rot}
	case "minecraft:jungle_sign":
		return block.JungleSign{Rotation: rot}
	case "minecraft:acacia_sign":
		return block.AcaciaSign{Rotation: rot}
	case "minecraft:cherry_sign":
		return block.CherrySign{Rotation: rot}
	case "minecraft:dark_oak_sign":
		return block.DarkOakSign{Rotation: rot}
	case "minecraft:mangrove_sign":
		return block.MangroveSign{Rotation: rot}
	case "minecraft:bamboo_sign":
		return block.BambooSign{Rotation: rot}
	case "minecraft:crimson_sign":
		return block.CrimsonSign{Rotation: rot}
	case "minecraft:warped_sign":
		return block.WarpedSign{Rotation: rot}
	}
	return nil
}

// wallSignWithFacing creates a wall sign block with the given facing direction.
func wallSignWithFacing(blockName string, facing block.Direction) block.Block {
	switch blockName {
	case "minecraft:oak_wall_sign":
		return block.OakWallSign{Facing: facing}
	case "minecraft:spruce_wall_sign":
		return block.SpruceWallSign{Facing: facing}
	case "minecraft:birch_wall_sign":
		return block.BirchWallSign{Facing: facing}
	case "minecraft:jungle_wall_sign":
		return block.JungleWallSign{Facing: facing}
	case "minecraft:acacia_wall_sign":
		return block.AcaciaWallSign{Facing: facing}
	case "minecraft:cherry_wall_sign":
		return block.CherryWallSign{Facing: facing}
	case "minecraft:dark_oak_wall_sign":
		return block.DarkOakWallSign{Facing: facing}
	case "minecraft:mangrove_wall_sign":
		return block.MangroveWallSign{Facing: facing}
	case "minecraft:bamboo_wall_sign":
		return block.BambooWallSign{Facing: facing}
	case "minecraft:crimson_wall_sign":
		return block.CrimsonWallSign{Facing: facing}
	case "minecraft:warped_wall_sign":
		return block.WarpedWallSign{Facing: facing}
	}
	return nil
}

// placeHangingSign handles hanging sign placement. Ceiling hanging signs are placed
// below a block (face 0 = bottom), wall hanging signs on side faces (face 2-5).
func (h *BlockHandler) placeHangingSign(player *game.Player, x, y, z int, itemName string, face int, sequence int32) {
	if h.wouldCollideWithPlayer(x, y, z) {
		h.broadcastBlockUpdate(x, y, z, 0)
		h.sendAck(player, sequence)
		return
	}

	var signBlock block.Block

	if face >= 2 && face <= 5 {
		// Wall hanging sign: attached to the side of a block
		dir, ok := faceToWallSignDirection(face)
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		wallName := wallHangingSignBlockForItem(itemName)
		if wallName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = wallHangingSignWithFacing(wallName, dir)
	} else {
		// Ceiling hanging sign: attached below a block, rotation from player yaw
		yaw, _ := player.Rotation()
		rotation := yawToSignRotation(yaw)
		ceilingName := hangingSignBlockForItem(itemName)
		if ceilingName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = ceilingHangingSignWithRotation(ceilingName, rotation)
	}

	if signBlock == nil {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := block.ToStateID[signBlock]
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Open sign editor
	h.SignMgr.PlaceSign(player, x, y, z)

	h.logf("Player %s placed hanging sign at (%d, %d, %d)", player.Name, x, y, z)
}

// ceilingHangingSignWithRotation creates a ceiling hanging sign block with the given rotation.
func ceilingHangingSignWithRotation(blockName string, rotation int) block.Block {
	rot := block.Integer(rotation)
	switch blockName {
	case "minecraft:oak_hanging_sign":
		return block.OakHangingSign{Rotation: rot}
	case "minecraft:spruce_hanging_sign":
		return block.SpruceHangingSign{Rotation: rot}
	case "minecraft:birch_hanging_sign":
		return block.BirchHangingSign{Rotation: rot}
	case "minecraft:jungle_hanging_sign":
		return block.JungleHangingSign{Rotation: rot}
	case "minecraft:acacia_hanging_sign":
		return block.AcaciaHangingSign{Rotation: rot}
	case "minecraft:cherry_hanging_sign":
		return block.CherryHangingSign{Rotation: rot}
	case "minecraft:dark_oak_hanging_sign":
		return block.DarkOakHangingSign{Rotation: rot}
	case "minecraft:mangrove_hanging_sign":
		return block.MangroveHangingSign{Rotation: rot}
	case "minecraft:bamboo_hanging_sign":
		return block.BambooHangingSign{Rotation: rot}
	case "minecraft:crimson_hanging_sign":
		return block.CrimsonHangingSign{Rotation: rot}
	case "minecraft:warped_hanging_sign":
		return block.WarpedHangingSign{Rotation: rot}
	}
	return nil
}

// wallHangingSignWithFacing creates a wall hanging sign block with the given facing direction.
func wallHangingSignWithFacing(blockName string, facing block.Direction) block.Block {
	switch blockName {
	case "minecraft:oak_wall_hanging_sign":
		return block.OakWallHangingSign{Facing: facing}
	case "minecraft:spruce_wall_hanging_sign":
		return block.SpruceWallHangingSign{Facing: facing}
	case "minecraft:birch_wall_hanging_sign":
		return block.BirchWallHangingSign{Facing: facing}
	case "minecraft:jungle_wall_hanging_sign":
		return block.JungleWallHangingSign{Facing: facing}
	case "minecraft:acacia_wall_hanging_sign":
		return block.AcaciaWallHangingSign{Facing: facing}
	case "minecraft:cherry_wall_hanging_sign":
		return block.CherryWallHangingSign{Facing: facing}
	case "minecraft:dark_oak_wall_hanging_sign":
		return block.DarkOakWallHangingSign{Facing: facing}
	case "minecraft:mangrove_wall_hanging_sign":
		return block.MangroveWallHangingSign{Facing: facing}
	case "minecraft:bamboo_wall_hanging_sign":
		return block.BambooWallHangingSign{Facing: facing}
	case "minecraft:crimson_wall_hanging_sign":
		return block.CrimsonWallHangingSign{Facing: facing}
	case "minecraft:warped_wall_hanging_sign":
		return block.WarpedWallHangingSign{Facing: facing}
	}
	return nil
}

// placeBed places both halves of a red bed at the given position.
func (h *BlockHandler) placeBed(player *game.Player, x, y, z int, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	foot := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartFoot}
	head := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartHead}

	footID, ok1 := block.ToStateID[foot]
	headID, ok2 := block.ToStateID[head]
	if !ok1 || !ok2 {
		h.sendAck(player, sequence)
		return
	}

	// Head block offset based on facing
	hx, hz := bedHeadOffset(facing)
	headX, headZ := x+hx, z+hz

	// Check that head position is air
	headState, err := h.World.GetBlock(headX, y, headZ)
	if err != nil || headState != 0 {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, footID)
	h.World.SetBlock(headX, y, headZ, headID)
	h.broadcastBlockUpdate(x, y, z, int32(footID))
	h.broadcastBlockUpdate(headX, y, headZ, int32(headID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}
}

// bedHeadOffset returns the (dx, dz) offset from foot to head for a given facing.
func bedHeadOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, -1
	case block.South:
		return 0, 1
	case block.West:
		return -1, 0
	case block.East:
		return 1, 0
	}
	return 0, 1
}

// wouldCollideWithPlayer checks if placing a block at (bx, by, bz) would overlap any player's hitbox.
func (h *BlockHandler) wouldCollideWithPlayer(bx, by, bz int) bool {
	var collides bool
	h.Manager.ForEach(func(p *game.Player) {
		px, py, pz := p.Position()
		// Player hitbox: 0.6 wide, 1.8 tall, centered on X/Z
		if float64(bx+1) > px-0.3 && float64(bx) < px+0.3 &&
			float64(by+1) > py && float64(by) < py+1.8 &&
			float64(bz+1) > pz-0.3 && float64(bz) < pz+0.3 {
			collides = true
		}
	})
	return collides
}

// handleBucket handles placing/picking up water or lava with buckets.
func (h *BlockHandler) handleBucket(player *game.Player, placeX, placeY, placeZ, clickX, clickY, clickZ int, action, heldName string, sequence int32) {
	slot := int(player.HeldSlot) + 36

	switch action {
	case "water":
		// Place water source block
		waterState, ok := block.ToStateID[block.Water{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, waterState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(waterState))
		// Replace held item with empty bucket
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		// Schedule fluid flow
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "water")
		}

	case "lava":
		// Place lava source block
		lavaState, ok := block.ToStateID[block.Lava{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, lavaState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(lavaState))
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "lava")
		}

	case "pickup":
		// Pick up source block at clicked position
		stateID, err := h.World.GetBlock(clickX, clickY, clickZ)
		if err != nil {
			h.sendAck(player, sequence)
			return
		}
		blockName := BlockNameFromState(int(stateID))
		var bucketName string
		switch blockName {
		case "water":
			// Only pick up source blocks (level=0)
			if b, ok := block.StateList[stateID].(block.Water); ok && int(b.Level) == 0 {
				bucketName = "water_bucket"
			}
		case "lava":
			if b, ok := block.StateList[stateID].(block.Lava); ok && int(b.Level) == 0 {
				bucketName = "lava_bucket"
			}
		}
		if bucketName == "" {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(clickX, clickY, clickZ, 0) // remove fluid
		h.broadcastBlockUpdate(clickX, clickY, clickZ, 0)
		if player.GameMode == 0 {
			bucketID := itemIDByName(bucketName)
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourceRemoved(clickX, clickY, clickZ)
		}
	}

	h.sendAck(player, sequence)
}

// plantSeed places a crop block on top of farmland and consumes the seed item.
func (h *BlockHandler) plantSeed(player *game.Player, x, y, z int, seedName string, sequence int32) bool {
	// Check that target position is air
	targetState, err := h.World.GetBlock(x, y, z)
	if err != nil || targetState != 0 {
		h.sendAck(player, sequence)
		return false
	}

	var cropBlock block.Block
	isStem := false
	switch seedName {
	case "wheat_seeds":
		cropBlock = block.Wheat{Age: block.Integer(0)}
	case "carrot":
		cropBlock = block.Carrots{Age: block.Integer(0)}
	case "potato":
		cropBlock = block.Potatoes{Age: block.Integer(0)}
	case "beetroot_seeds":
		cropBlock = block.Beetroots{Age: block.Integer(0)}
	case "pumpkin_seeds":
		cropBlock = block.PumpkinStem{Age: block.Integer(0)}
		isStem = true
	case "melon_seeds":
		cropBlock = block.MelonStem{Age: block.Integer(0)}
		isStem = true
	default:
		return false
	}

	cropStateID, ok := block.ToStateID[cropBlock]
	if !ok {
		return false
	}

	h.World.SetBlock(x, y, z, cropStateID)
	h.broadcastBlockUpdate(x, y, z, int32(cropStateID))
	h.sendAck(player, sequence)

	// Consume seed in survival
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Register crop for growth
	if h.CropMgr != nil {
		if isStem {
			h.CropMgr.RegisterStem(x, y, z)
		} else {
			h.CropMgr.RegisterCrop(x, y, z)
		}
	}

	return true
}

// placeNonFarmlandPlant handles placing sugar cane, bamboo, nether wart, and sweet berries.
func (h *BlockHandler) placeNonFarmlandPlant(player *game.Player, x, y, z int, itemName string, sequence int32) bool {
	// Check that target position is air
	targetState, err := h.World.GetBlock(x, y, z)
	if err != nil || targetState != 0 {
		h.sendAck(player, sequence)
		return false
	}

	// Check block below
	belowState, err := h.World.GetBlock(x, y-1, z)
	if err != nil {
		h.sendAck(player, sequence)
		return false
	}
	belowName := BlockNameFromState(int(belowState))

	var plantBlock block.Block

	switch itemName {
	case "sugar_cane":
		// Must be on dirt, sand, grass_block, or another sugar_cane
		if belowName != "dirt" && belowName != "sand" && belowName != "grass_block" && belowName != "sugar_cane" {
			return false
		}
		// If placing on ground (not on another sugar cane), check for adjacent water
		if belowName != "sugar_cane" {
			if !h.hasAdjacentWater(x, y-1, z) {
				return false
			}
		}
		plantBlock = block.SugarCane{Age: 0}

	case "bamboo":
		// Must be on grass_block, dirt, or sand (or another bamboo)
		if belowName != "grass_block" && belowName != "dirt" && belowName != "sand" && belowName != "bamboo" && belowName != "bamboo_sapling" {
			return false
		}
		plantBlock = block.Bamboo{Age: 0, Leaves: block.BambooLeavesNone, Stage: 0}

	case "nether_wart":
		// Must be on soul_sand
		if belowName != "soul_sand" {
			return false
		}
		plantBlock = block.NetherWart{Age: block.Integer(0)}

	case "sweet_berries":
		// Must be on grass_block or dirt
		if belowName != "grass_block" && belowName != "dirt" {
			return false
		}
		plantBlock = block.SweetBerryBush{Age: block.Integer(0)}

	default:
		return false
	}

	plantStateID, ok := block.ToStateID[plantBlock]
	if !ok {
		return false
	}

	h.World.SetBlock(x, y, z, plantStateID)
	h.broadcastBlockUpdate(x, y, z, int32(plantStateID))
	h.sendAck(player, sequence)

	// Consume item in survival
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Register for growth ticking
	if h.CropMgr != nil {
		switch itemName {
		case "sugar_cane":
			h.CropMgr.RegisterSugarCane(x, y, z)
		case "bamboo":
			h.CropMgr.RegisterBamboo(x, y, z)
		case "nether_wart":
			h.CropMgr.RegisterNetherWart(x, y, z)
		case "sweet_berries":
			h.CropMgr.RegisterBerry(x, y, z)
		}
	}

	return true
}

// hasAdjacentWater checks if there is water within 1 block horizontally at the given Y level.
func (h *BlockHandler) hasAdjacentWater(x, y, z int) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			ws, err := h.World.GetBlock(x+dx, y, z+dz)
			if err != nil {
				continue
			}
			if int(ws) < len(block.StateList) && block.StateList[ws] != nil {
				if _, ok := block.StateList[ws].(block.Water); ok {
					return true
				}
			}
		}
	}
	return false
}

// isCropBlock returns true if the block name is a crop (includes all crop types).
func isCropBlock(name string) bool {
	switch name {
	case "wheat", "carrots", "potatoes", "beetroots",
		"pumpkin_stem", "melon_stem", "attached_pumpkin_stem", "attached_melon_stem",
		"sugar_cane", "bamboo", "bamboo_sapling",
		"nether_wart", "cocoa", "sweet_berry_bush":
		return true
	}
	return false
}

// isSeedItem returns true if the item name is a seed that can be planted on farmland.
func isSeedItem(name string) bool {
	switch name {
	case "wheat_seeds", "carrot", "potato", "beetroot_seeds",
		"pumpkin_seeds", "melon_seeds":
		return true
	}
	return false
}

// isNonFarmlandPlantItem returns true if the item is a plant that goes on non-farmland blocks.
func isNonFarmlandPlantItem(name string) bool {
	switch name {
	case "sugar_cane", "bamboo", "nether_wart", "sweet_berries":
		return true
	}
	return false
}

// isHoeItem returns true if the item name is a hoe.
func isHoeItem(name string) bool {
	info := GetToolInfo(name)
	return info != nil && info.Type == ToolHoe
}

// placeBoat handles placing a boat item on water or on the top face of a block.
func (h *BlockHandler) placeBoat(player *game.Player, clickX, clickY, clickZ, face int, woodType int32, sequence int32) {
	// Determine spawn position: place on the clicked block's top surface or on water
	spawnX := float64(clickX) + 0.5
	spawnY := float64(clickY) + 1.0
	spawnZ := float64(clickZ) + 0.5

	// If the clicked block is water, place boat on the water surface
	clickedState, err := h.World.GetBlock(clickX, clickY, clickZ)
	if err == nil {
		if int(clickedState) < len(block.StateList) && block.StateList[clickedState] != nil {
			if _, ok := block.StateList[clickedState].(block.Water); ok {
				// Find water surface (topmost water block)
				surfaceY := clickY
				for sy := clickY + 1; sy < clickY+10; sy++ {
					aboveState, err := h.World.GetBlock(clickX, sy, clickZ)
					if err != nil {
						break
					}
					if int(aboveState) < len(block.StateList) && block.StateList[aboveState] != nil {
						if _, ok := block.StateList[aboveState].(block.Water); ok {
							surfaceY = sy
							continue
						}
					}
					break
				}
				spawnY = float64(surfaceY) + 0.5625 // boat sits slightly above water
			}
		}
	}

	h.BoatMgr.SpawnBoat(spawnX, spawnY, spawnZ, woodType)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}

// placeMinecart places a minecart entity on the clicked rail block.
func (h *BlockHandler) placeMinecart(player *game.Player, clickX, clickY, clickZ, face int, sequence int32, itemName string) {
	// The minecart should be placed on top of the clicked block
	spawnX := float64(clickX) + 0.5
	spawnY := float64(clickY) + 0.0625 // slightly above the block
	spawnZ := float64(clickZ) + 0.5

	// Check if clicked block is a rail
	clickedState, err := h.World.GetBlock(clickX, clickY, clickZ)
	if err == nil && int(clickedState) < len(block.StateList) && block.StateList[clickedState] != nil {
		switch block.StateList[clickedState].(type) {
		case block.Rail, block.PoweredRail, block.DetectorRail, block.ActivatorRail:
			// Good, place on the rail
		default:
			// If we clicked the top of a block, check the block above for a rail
			if face == 1 {
				aboveState, err := h.World.GetBlock(clickX, clickY+1, clickZ)
				if err == nil && int(aboveState) < len(block.StateList) && block.StateList[aboveState] != nil {
					switch block.StateList[aboveState].(type) {
					case block.Rail, block.PoweredRail, block.DetectorRail, block.ActivatorRail:
						spawnY = float64(clickY+1) + 0.0625
					default:
						h.sendAck(player, sequence)
						return // not on a rail
					}
				} else {
					h.sendAck(player, sequence)
					return
				}
			} else {
				h.sendAck(player, sequence)
				return // not on a rail
			}
		}
	} else {
		h.sendAck(player, sequence)
		return
	}

	h.MinecartMgr.SpawnMinecartVariant(spawnX, spawnY, spawnZ, minecartVariantForItem(itemName))

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}

// placeRail places a rail block with auto-curving using the MinecartManager.
func (h *BlockHandler) placeRail(player *game.Player, x, y, z int, railName string, sequence int32) {
	// Check that the target position is empty (air)
	existing, err := h.World.GetBlock(x, y, z)
	if err == nil && existing != 0 {
		h.sendAck(player, sequence)
		return
	}

	// Check that there is a solid block below (rails need support)
	belowState, err := h.World.GetBlock(x, y-1, z)
	if err != nil || belowState == 0 {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := h.MinecartMgr.PlaceRail(x, y, z, railName)
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}
