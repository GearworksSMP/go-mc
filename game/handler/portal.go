package handler

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Portal frame constraints (Minecraft vanilla rules).
const (
	portalMinWidth  = 2  // minimum interior width
	portalMaxWidth  = 21 // maximum interior width
	portalMinHeight = 3  // minimum interior height
	portalMaxHeight = 21 // maximum interior height
)

// detectPortalFrame checks whether the given obsidian position is part of a valid
// nether portal frame. It tries both X-axis and Z-axis orientations.
// Returns the list of interior positions to fill with portal blocks, the axis, and ok=true
// if a valid frame was found.
func detectPortalFrame(world game.World, clickX, clickY, clickZ int) (portalBlocks [][3]int, axis block.Axis, ok bool) {
	// Try X-axis portal (frame spans along X, portal faces east/west)
	if blocks, found := tryDetectFrame(world, clickX, clickY, clickZ, 1, 0); found {
		return blocks, block.X, true
	}
	// Try Z-axis portal (frame spans along Z, portal faces north/south)
	if blocks, found := tryDetectFrame(world, clickX, clickY, clickZ, 0, 1); found {
		return blocks, block.Z, true
	}
	return nil, 0, false
}

// tryDetectFrame attempts to detect a portal frame from a clicked obsidian block,
// scanning along the given horizontal axis (dx, dz).
// For an X-axis portal: dx=1, dz=0. For a Z-axis portal: dx=0, dz=1.
//
// The clicked obsidian can be any part of the frame (bottom row, top row, left column,
// right column, or a corner). The strategy is:
// 1. Find the lowest obsidian in this column (potential bottom row Y).
// 2. Try each Y from there upward as a candidate bottom row.
// 3. At each candidate Y, find the leftmost obsidian along the axis direction.
// 4. Try each horizontal position as a candidate left column.
func tryDetectFrame(world game.World, startX, startY, startZ, dx, dz int) ([][3]int, bool) {
	// Find the lowest obsidian in this column (scanning down).
	bottomY := startY
	for i := 0; i < portalMaxHeight+2; i++ {
		s, err := world.GetBlock(startX, bottomY-1, startZ)
		if err != nil || !isObsidianState(int(s)) {
			break
		}
		bottomY--
	}

	// Try each Y from bottomY up to startY as a candidate base row.
	for baseY := bottomY; baseY <= startY; baseY++ {
		// Find leftmost obsidian along the horizontal axis at this Y level.
		leftX, leftZ := startX, startZ
		for i := 0; i < portalMaxWidth+2; i++ {
			nx, nz := leftX-dx, leftZ-dz
			s, err := world.GetBlock(nx, baseY, nz)
			if err != nil || !isObsidianState(int(s)) {
				break
			}
			leftX, leftZ = nx, nz
		}

		// Try each horizontal position as a candidate left column.
		for offset := 0; offset < portalMaxWidth+2; offset++ {
			frameX := leftX + offset*dx
			frameZ := leftZ + offset*dz
			s, err := world.GetBlock(frameX, baseY, frameZ)
			if err != nil || !isObsidianState(int(s)) {
				break
			}
			if blocks, valid := validateFrame(world, frameX, baseY, frameZ, dx, dz); valid {
				return blocks, true
			}
		}
	}

	return nil, false
}

// validateFrame checks if there's a valid portal frame with the left column at
// (frameX, baseY, frameZ) and the bottom row at baseY. dx,dz define the horizontal axis.
//
// Frame layout (2-wide x 3-tall interior example):
//
//	O O O O    <- top row at baseY+4 (all obsidian)
//	O . . O    <- baseY+3: left/right columns obsidian, interior air
//	O . . O    <- baseY+2: left/right columns obsidian, interior air
//	O . . O    <- baseY+1: left/right columns obsidian, interior air
//	O O O O    <- bottom row at baseY (all obsidian)
//
// frameX,frameZ is the left column position. Interior columns are at offsets 1..width.
// Right column is at offset width+1.
//
// Returns interior positions and true if valid.
func validateFrame(world game.World, frameX, baseY, frameZ, dx, dz int) ([][3]int, bool) {
	// Verify bottom-left corner is obsidian
	state, err := world.GetBlock(frameX, baseY, frameZ)
	if err != nil || !isObsidianState(int(state)) {
		return nil, false
	}

	// Determine interior width by scanning the row at baseY+1 (first interior row).
	// The left column (frameX) should be obsidian. Interior blocks should be air.
	// The right column is the first obsidian after the air run.
	leftColState, err := world.GetBlock(frameX, baseY+1, frameZ)
	if err != nil || !isObsidianState(int(leftColState)) {
		return nil, false
	}

	width := 0
	for w := 1; w <= portalMaxWidth+1; w++ {
		bx := frameX + w*dx
		bz := frameZ + w*dz
		s, err := world.GetBlock(bx, baseY+1, bz)
		if err != nil {
			return nil, false
		}
		if isObsidianState(int(s)) {
			width = w - 1 // interior width
			break
		}
		if !isAirOrPortalState(int(s)) {
			return nil, false // non-air, non-obsidian block in the interior
		}
	}

	if width < portalMinWidth || width > portalMaxWidth {
		return nil, false
	}

	// Determine interior height by scanning up the left column.
	// At each interior row, the left column must be obsidian.
	// The first row where the left column is obsidian AND the block next to it
	// (the interior) is also obsidian marks the top row.
	height := 0
	for h := 1; h <= portalMaxHeight+1; h++ {
		leftState, err := world.GetBlock(frameX, baseY+h, frameZ)
		if err != nil {
			return nil, false
		}
		if !isObsidianState(int(leftState)) {
			return nil, false // left column must be obsidian at every row
		}
		// Check the first interior block at this height
		intState, err := world.GetBlock(frameX+dx, baseY+h, frameZ+dz)
		if err != nil {
			return nil, false
		}
		if isObsidianState(int(intState)) {
			// This is the top row
			height = h - 1
			break
		}
		if !isAirOrPortalState(int(intState)) {
			return nil, false
		}
	}

	if height < portalMinHeight || height > portalMaxHeight {
		return nil, false
	}

	// Verify bottom row: all obsidian
	for w := 0; w <= width+1; w++ {
		s, err := world.GetBlock(frameX+w*dx, baseY, frameZ+w*dz)
		if err != nil || !isObsidianState(int(s)) {
			return nil, false
		}
	}

	// Verify top row: all obsidian at baseY+height+1
	topY := baseY + height + 1
	for w := 0; w <= width+1; w++ {
		s, err := world.GetBlock(frameX+w*dx, topY, frameZ+w*dz)
		if err != nil || !isObsidianState(int(s)) {
			return nil, false
		}
	}

	// Verify right column: obsidian from baseY+1 to baseY+height
	rightX := frameX + (width+1)*dx
	rightZ := frameZ + (width+1)*dz
	for h := 1; h <= height; h++ {
		s, err := world.GetBlock(rightX, baseY+h, rightZ)
		if err != nil || !isObsidianState(int(s)) {
			return nil, false
		}
	}

	// Verify interior is all air or portal blocks
	var interior [][3]int
	for h := 1; h <= height; h++ {
		for w := 1; w <= width; w++ {
			ix := frameX + w*dx
			iz := frameZ + w*dz
			iy := baseY + h
			s, err := world.GetBlock(ix, iy, iz)
			if err != nil {
				return nil, false
			}
			if !isAirOrPortalState(int(s)) {
				return nil, false
			}
			interior = append(interior, [3]int{ix, iy, iz})
		}
	}

	if len(interior) == 0 {
		return nil, false
	}

	return interior, true
}

// isObsidianState returns true if the block state represents obsidian.
func isObsidianState(stateID int) bool {
	name := BlockNameFromState(stateID)
	return name == "obsidian"
}

// isAirOrPortalState returns true if the block state is air or nether_portal.
func isAirOrPortalState(stateID int) bool {
	if stateID == 0 {
		return true // air
	}
	name := BlockNameFromState(stateID)
	return name == "air" || name == "cave_air" || name == "void_air" || name == "nether_portal"
}

// isPortalState returns true if the block state is a nether portal.
func isPortalState(stateID int) bool {
	return BlockNameFromState(stateID) == "nether_portal"
}

// removeConnectedPortals removes all nether_portal blocks connected to the given position.
// Uses flood-fill to find and remove adjacent portal blocks.
func removeConnectedPortals(world game.World, manager *game.PlayerManager, startX, startY, startZ int) {
	type pos struct{ x, y, z int }
	visited := make(map[pos]bool)
	queue := []pos{{startX, startY, startZ}}

	var toRemove []pos

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		if visited[p] {
			continue
		}
		visited[p] = true

		state, err := world.GetBlock(p.x, p.y, p.z)
		if err != nil || !isPortalState(int(state)) {
			continue
		}

		toRemove = append(toRemove, p)

		// Check all 6 adjacent blocks
		for _, off := range [][3]int{
			{1, 0, 0}, {-1, 0, 0},
			{0, 1, 0}, {0, -1, 0},
			{0, 0, 1}, {0, 0, -1},
		} {
			np := pos{p.x + off[0], p.y + off[1], p.z + off[2]}
			if !visited[np] {
				queue = append(queue, np)
			}
		}
	}

	// Remove all found portal blocks
	for _, p := range toRemove {
		world.SetBlock(p.x, p.y, p.z, 0) // set to air
		broadcastBlockUpdateStatic(manager, p.x, p.y, p.z, 0)
	}
}

// removeAdjacentPortals checks if a broken block is adjacent to any nether_portal blocks,
// and if so, removes all connected portal blocks. Called when obsidian or portal blocks are broken.
func removeAdjacentPortals(world game.World, manager *game.PlayerManager, x, y, z int) {
	for _, off := range [][3]int{
		{1, 0, 0}, {-1, 0, 0},
		{0, 1, 0}, {0, -1, 0},
		{0, 0, 1}, {0, 0, -1},
	} {
		ax, ay, az := x+off[0], y+off[1], z+off[2]
		state, err := world.GetBlock(ax, ay, az)
		if err != nil {
			continue
		}
		if isPortalState(int(state)) {
			removeConnectedPortals(world, manager, ax, ay, az)
		}
	}
}

// broadcastBlockUpdateStatic is a package-level helper that sends ClientboundBlockUpdate
// to all connected players without requiring a BlockHandler receiver.
func broadcastBlockUpdateStatic(manager *game.PlayerManager, x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
