package gen

import (
	"math"

	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// StrongholdPlacer generates stronghold rooms underground.
type StrongholdPlacer struct {
	Seed int64

	// Block state IDs looked up at init time.
	stoneBricksID      level.BlocksState
	mossyStoneBricksID level.BlocksState
	cobblestoneID      level.BlocksState
	chestID            level.BlocksState
	ironBarsID         level.BlocksState
	bookshelfID        level.BlocksState
	torchID            level.BlocksState
	airID              level.BlocksState

	// End portal frame block IDs for each facing direction.
	endPortalFrameNorth level.BlocksState
	endPortalFrameSouth level.BlocksState
	endPortalFrameEast  level.BlocksState
	endPortalFrameWest  level.BlocksState

	// End portal frame with Eye=true for each direction.
	endPortalFrameNorthEye level.BlocksState
	endPortalFrameSouthEye level.BlocksState
	endPortalFrameEastEye  level.BlocksState
	endPortalFrameWestEye  level.BlocksState

	lavaID level.BlocksState

	// Cached stronghold positions (3 per world).
	positions [][2]int // [chunkX, chunkZ]
}

// NewStrongholdPlacer creates a StrongholdPlacer with resolved block state IDs.
func NewStrongholdPlacer(seed int64) *StrongholdPlacer {
	shp := &StrongholdPlacer{Seed: seed}

	shp.stoneBricksID, _ = block.ToStateID[block.StoneBricks{}]
	shp.mossyStoneBricksID, _ = block.ToStateID[block.MossyStoneBricks{}]
	shp.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	shp.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	shp.ironBarsID, _ = block.ToStateID[block.IronBars{}]
	shp.bookshelfID, _ = block.ToStateID[block.Bookshelf{}]
	shp.torchID, _ = block.ToStateID[block.Torch{}]
	shp.lavaID, _ = block.ToStateID[block.Lava{Level: 0}]
	shp.airID = 0

	// End portal frames facing each direction (without eye).
	shp.endPortalFrameNorth, _ = block.ToStateID[block.EndPortalFrame{Eye: false, Facing: block.North}]
	shp.endPortalFrameSouth, _ = block.ToStateID[block.EndPortalFrame{Eye: false, Facing: block.South}]
	shp.endPortalFrameEast, _ = block.ToStateID[block.EndPortalFrame{Eye: false, Facing: block.East}]
	shp.endPortalFrameWest, _ = block.ToStateID[block.EndPortalFrame{Eye: false, Facing: block.West}]

	// End portal frames facing each direction (with eye).
	shp.endPortalFrameNorthEye, _ = block.ToStateID[block.EndPortalFrame{Eye: true, Facing: block.North}]
	shp.endPortalFrameSouthEye, _ = block.ToStateID[block.EndPortalFrame{Eye: true, Facing: block.South}]
	shp.endPortalFrameEastEye, _ = block.ToStateID[block.EndPortalFrame{Eye: true, Facing: block.East}]
	shp.endPortalFrameWestEye, _ = block.ToStateID[block.EndPortalFrame{Eye: true, Facing: block.West}]

	shp.positions = getStrongholdPositions(seed)

	return shp
}

// getStrongholdPositions returns 3 stronghold chunk positions for a given seed.
// Strongholds are placed 1000-2700 blocks from the origin, evenly spaced around a ring.
func getStrongholdPositions(seed int64) [][2]int {
	positions := make([][2]int, 3)

	for i := 0; i < 3; i++ {
		// Angle: evenly spaced around a circle with seed-based offset.
		angle := (2.0 * math.Pi * float64(i) / 3.0) + float64(structureHash(i, 0, seed, 0x5701D))*0.0001
		// Distance: 1000-2700 blocks from origin.
		dist := 1000.0 + float64(abs64(structureHash(i, 1, seed, 0x5701D))%1700)

		worldX := int(math.Round(math.Cos(angle) * dist))
		worldZ := int(math.Round(math.Sin(angle) * dist))

		positions[i] = [2]int{worldX >> 4, worldZ >> 4} // convert to chunk coords
	}

	return positions
}

// PlaceStronghold checks if this chunk is a stronghold position and generates the end portal room.
func (shp *StrongholdPlacer) PlaceStronghold(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) {
	for _, pos := range shp.positions {
		if pos[0] == chunkX && pos[1] == chunkZ {
			shp.placeEndPortalRoom(chunk, chunkX, chunkZ, gen)
			shp.placeLibrary(chunk, chunkX, chunkZ, gen)
			shp.placeCorridor(chunk, chunkX, chunkZ, gen)
			return
		}
	}
}

// placeEndPortalRoom creates a 9x9x6 end portal room at Y=30.
func (shp *StrongholdPlacer) placeEndPortalRoom(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) {
	// Room centered at chunk local (7,7), occupying (3..11, 3..11).
	roomBaseY := 30
	roomOriginX := 3
	roomOriginZ := 3

	// Build the room shell.
	for dy := 0; dy <= 5; dy++ {
		by := roomBaseY + dy
		for dz := 0; dz < 9; dz++ {
			for dx := 0; dx < 9; dx++ {
				bx := roomOriginX + dx
				bz := roomOriginZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == 8 || dz == 0 || dz == 8
				isFloor := dy == 0
				isCeiling := dy == 5

				var stateID level.BlocksState
				switch {
				case isFloor || isCeiling:
					stateID = shp.stoneBricksID
				case isWall:
					// Mix stone bricks and mossy stone bricks.
					h := structureHash(dx, dz+dy, shp.Seed, 0x5555)
					if abs64(h)%4 == 0 {
						stateID = shp.mossyStoneBricksID
					} else {
						stateID = shp.stoneBricksID
					}
				default:
					stateID = shp.airID
				}

				shp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Place lava pool in the center of the room floor.
	centerX := roomOriginX + 4
	centerZ := roomOriginZ + 4
	if centerX >= 0 && centerX < 16 && centerZ >= 0 && centerZ < 16 {
		shp.setBlock(chunk, centerX, roomBaseY+1, centerZ, shp.lavaID, gen)
	}

	// Place end portal frame in a ring (12 blocks).
	// The portal is a 5x5 ring (minus corners) centered in the room.
	// Frame Y is at roomBaseY+1 (one above the floor).
	portalY := roomBaseY + 1
	portalCX := roomOriginX + 4 // center of the room
	portalCZ := roomOriginZ + 4

	// Determine which frames have ender eyes based on seed (typically 0-2 out of 12).
	eyeHash := abs64(structureHash(chunkX, chunkZ, shp.Seed, 0x6666))
	eyeCount := int(eyeHash % 3) // 0, 1, or 2 eyes

	type framePos struct {
		dx, dz     int
		noEye, eye level.BlocksState
	}

	frames := []framePos{
		// North side (facing south): dz=-2
		{-1, -2, shp.endPortalFrameSouth, shp.endPortalFrameSouthEye},
		{0, -2, shp.endPortalFrameSouth, shp.endPortalFrameSouthEye},
		{1, -2, shp.endPortalFrameSouth, shp.endPortalFrameSouthEye},
		// South side (facing north): dz=+2
		{-1, 2, shp.endPortalFrameNorth, shp.endPortalFrameNorthEye},
		{0, 2, shp.endPortalFrameNorth, shp.endPortalFrameNorthEye},
		{1, 2, shp.endPortalFrameNorth, shp.endPortalFrameNorthEye},
		// East side (facing west): dx=+2
		{2, -1, shp.endPortalFrameWest, shp.endPortalFrameWestEye},
		{2, 0, shp.endPortalFrameWest, shp.endPortalFrameWestEye},
		{2, 1, shp.endPortalFrameWest, shp.endPortalFrameWestEye},
		// West side (facing east): dx=-2
		{-2, -1, shp.endPortalFrameEast, shp.endPortalFrameEastEye},
		{-2, 0, shp.endPortalFrameEast, shp.endPortalFrameEastEye},
		{-2, 1, shp.endPortalFrameEast, shp.endPortalFrameEastEye},
	}

	for i, f := range frames {
		fx := portalCX + f.dx
		fz := portalCZ + f.dz
		if fx < 0 || fx >= 16 || fz < 0 || fz >= 16 {
			continue
		}

		var stateID level.BlocksState
		if i < eyeCount {
			stateID = f.eye
		} else {
			stateID = f.noEye
		}
		shp.setBlock(chunk, fx, portalY, fz, stateID, gen)
	}

	// Place torches in room corners.
	torchPositions := [][2]int{
		{roomOriginX + 1, roomOriginZ + 1},
		{roomOriginX + 7, roomOriginZ + 1},
		{roomOriginX + 1, roomOriginZ + 7},
		{roomOriginX + 7, roomOriginZ + 7},
	}
	for _, tp := range torchPositions {
		if tp[0] >= 0 && tp[0] < 16 && tp[1] >= 0 && tp[1] < 16 {
			shp.setBlock(chunk, tp[0], roomBaseY+1, tp[1], shp.torchID, gen)
		}
	}
}

// placeLibrary creates a 9x9x8 library room offset from the portal room.
func (shp *StrongholdPlacer) placeLibrary(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) {
	// Library is placed in the eastern portion of the chunk (origin at 3, 26..34 Z).
	// Offset from portal room to avoid overlap.
	libBaseY := 30
	libOriginX := 3
	libOriginZ := 12 // adjacent to portal room (portal room goes 3..11)

	// Only place if it fits in the chunk.
	if libOriginZ+5 > 16 {
		// Reduce to fit: make it 3 blocks deep instead.
		libOriginZ = 12
	}
	libWidth := 9
	libDepth := 4 // keep it small to fit in the chunk
	libHeight := 6

	for dy := 0; dy < libHeight; dy++ {
		by := libBaseY + dy
		for dz := 0; dz < libDepth; dz++ {
			for dx := 0; dx < libWidth; dx++ {
				bx := libOriginX + dx
				bz := libOriginZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == libWidth-1 || dz == 0 || dz == libDepth-1
				isFloor := dy == 0
				isCeiling := dy == libHeight-1

				var stateID level.BlocksState
				switch {
				case isFloor || isCeiling:
					stateID = shp.stoneBricksID
				case isWall:
					stateID = shp.stoneBricksID
				default:
					stateID = shp.airID
				}

				shp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Place bookshelves along the walls (interior side).
	for dy := 1; dy <= 3; dy++ {
		for dx := 1; dx < libWidth-1; dx++ {
			bx := libOriginX + dx
			// North wall (dz=1 interior).
			bz1 := libOriginZ + 1
			if bx >= 0 && bx < 16 && bz1 >= 0 && bz1 < 16 && dx != libWidth/2 {
				shp.setBlock(chunk, bx, libBaseY+dy, bz1, shp.bookshelfID, gen)
			}
		}
	}

	// Chest in the library.
	chestX := libOriginX + 4
	chestZ := libOriginZ + 2
	if chestX >= 0 && chestX < 16 && chestZ >= 0 && chestZ < 16 {
		shp.setBlock(chunk, chestX, libBaseY+1, chestZ, shp.chestID, gen)
	}

	// Torches.
	for _, dx := range []int{2, 6} {
		tx := libOriginX + dx
		tz := libOriginZ + 2
		if tx >= 0 && tx < 16 && tz >= 0 && tz < 16 {
			shp.setBlock(chunk, tx, libBaseY+1, tz, shp.torchID, gen)
		}
	}
}

// placeCorridor creates a 3-wide hallway connecting the portal room and library.
func (shp *StrongholdPlacer) placeCorridor(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) {
	corridorY := 30
	corridorX := 6 // centered in the room (rooms are at X offset 3, center at 7)
	corridorStartZ := 11 // end of portal room
	corridorEndZ := 12   // start of library

	for dz := corridorStartZ; dz <= corridorEndZ; dz++ {
		for dx := 0; dx < 3; dx++ {
			bx := corridorX + dx
			if bx < 0 || bx >= 16 || dz < 0 || dz >= 16 {
				continue
			}
			// Floor.
			shp.setBlock(chunk, bx, corridorY, dz, shp.stoneBricksID, gen)
			// Air interior.
			for dy := 1; dy <= 3; dy++ {
				shp.setBlock(chunk, bx, corridorY+dy, dz, shp.airID, gen)
			}
			// Ceiling.
			shp.setBlock(chunk, bx, corridorY+4, dz, shp.stoneBricksID, gen)
		}
	}
}

// setBlock sets a block in the chunk at local coordinates (x 0-15, z 0-15, worldY).
func (shp *StrongholdPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
