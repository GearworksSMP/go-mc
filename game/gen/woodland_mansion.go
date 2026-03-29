package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// WoodlandMansionPlacer generates simplified woodland mansion structures in dark forest biomes.
type WoodlandMansionPlacer struct {
	Seed int64

	darkOakPlanksID level.BlocksState
	darkOakLogID    level.BlocksState
	oakPlanksID     level.BlocksState
	cobblestoneID   level.BlocksState
	bookshelfID     level.BlocksState
	redCarpetID     level.BlocksState
	whiteCarpetID   level.BlocksState
	grayCarpetID    level.BlocksState
	chestID         level.BlocksState
	whiteBannerID   level.BlocksState
	airID           level.BlocksState
}

// NewWoodlandMansionPlacer creates a WoodlandMansionPlacer with resolved block state IDs.
func NewWoodlandMansionPlacer(seed int64) *WoodlandMansionPlacer {
	p := &WoodlandMansionPlacer{Seed: seed, airID: 0}

	p.darkOakPlanksID, _ = block.ToStateID[block.DarkOakPlanks{}]
	p.darkOakLogID, _ = block.ToStateID[block.DarkOakLog{Axis: block.Y}]
	p.oakPlanksID, _ = block.ToStateID[block.OakPlanks{}]
	p.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	p.bookshelfID, _ = block.ToStateID[block.Bookshelf{}]
	p.redCarpetID, _ = block.ToStateID[block.RedCarpet{}]
	p.whiteCarpetID, _ = block.ToStateID[block.WhiteCarpet{}]
	p.grayCarpetID, _ = block.ToStateID[block.GrayCarpet{}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	p.whiteBannerID, _ = block.ToStateID[block.WhiteBanner{Rotation: 0}]

	return p
}

// PlaceWoodlandMansion places a simplified woodland mansion structure.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
// The mansion is 15 wide x 13 deep x 15 tall (simplified to fit within a chunk).
func (p *WoodlandMansionPlacer) PlaceWoodlandMansion(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	const (
		width  = 15
		depth  = 13
		height = 15
	)

	floorY := baseY + 1

	// Build the exterior shell.
	p.buildExterior(chunk, localX, floorY, localZ, width, depth, height, gen)

	// Place interior rooms.
	p.placeEntranceHall(chunk, localX, floorY, localZ, width, gen)
	p.placeDiningRoom(chunk, localX, floorY, localZ, gen)
	p.placeLibrary(chunk, localX, floorY, localZ, width, gen)
	p.placeBedroom(chunk, localX, floorY, localZ, gen)
	p.placeAttic(chunk, localX, floorY, localZ, width, depth, height, gen)
}

// buildExterior constructs the mansion's outer walls, floors, and roof.
func (p *WoodlandMansionPlacer) buildExterior(chunk *level.Chunk, localX, floorY, localZ, w, d, h int, gen *TerrainGenerator) {
	// Foundation.
	for dz := 0; dz < d; dz++ {
		for dx := 0; dx < w; dx++ {
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}
			p.setBlock(chunk, bx, floorY-1, bz, p.cobblestoneID, gen)
		}
	}

	// Two floors: ground floor (dy 0-6) and upper floor (dy 7-13), roof at dy 14.
	for dy := 0; dy < h; dy++ {
		by := floorY + dy
		for dz := 0; dz < d; dz++ {
			for dx := 0; dx < w; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWallX := dx == 0 || dx == w-1
				isWallZ := dz == 0 || dz == d-1
				isWall := isWallX || isWallZ
				isFloor := dy == 0 || dy == 7 // ground floor and upper floor
				isRoof := dy == h-1

				// Corner pillars: dark oak logs.
				isCorner := (dx == 0 || dx == w-1) && (dz == 0 || dz == d-1)
				// Mid-wall pillars for structural detail.
				isMidPillar := isWall && (dx == w/2 || dz == d/2)

				switch {
				case isCorner:
					p.setBlock(chunk, bx, by, bz, p.darkOakLogID, gen)
				case isMidPillar && (dy%7 != 0):
					p.setBlock(chunk, bx, by, bz, p.darkOakLogID, gen)
				case isRoof:
					p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
				case isFloor:
					p.setBlock(chunk, bx, by, bz, p.oakPlanksID, gen)
				case isWall:
					// Door opening on the south wall center, ground floor.
					if dz == d-1 && dx >= w/2-1 && dx <= w/2+1 && dy >= 1 && dy <= 3 {
						p.setBlock(chunk, bx, by, bz, p.airID, gen)
					} else {
						p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
					}
				default:
					// Interior air.
					p.setBlock(chunk, bx, by, bz, p.airID, gen)
				}
			}
		}
	}
}

// placeEntranceHall adds carpet and banners to the entrance area (ground floor south).
func (p *WoodlandMansionPlacer) placeEntranceHall(chunk *level.Chunk, localX, floorY, localZ, w int, gen *TerrainGenerator) {
	// Red carpet runner down the center of the entrance.
	centerX := localX + w/2
	if centerX < 0 || centerX >= 16 {
		return
	}
	for dz := 1; dz < 6; dz++ {
		bz := localZ + 12 - dz // from door inward
		if bz < 0 || bz >= 16 {
			continue
		}
		p.setBlock(chunk, centerX, floorY+1, bz, p.redCarpetID, gen)
	}

	// Banners flanking the entrance.
	for _, dx := range []int{w/2 - 2, w/2 + 2} {
		bx := localX + dx
		if bx < 0 || bx >= 16 {
			continue
		}
		bz := localZ + 11
		if bz < 0 || bz >= 16 {
			continue
		}
		p.setBlock(chunk, bx, floorY+3, bz, p.whiteBannerID, gen)
	}
}

// placeDiningRoom adds a table and carpet to the NW corner of the ground floor.
func (p *WoodlandMansionPlacer) placeDiningRoom(chunk *level.Chunk, localX, floorY, localZ int, gen *TerrainGenerator) {
	// Table: oak planks in a 3x2 area.
	for dx := 2; dx <= 4; dx++ {
		for dz := 2; dz <= 3; dz++ {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, floorY+1, bz, p.oakPlanksID, gen)
			}
		}
	}

	// Gray carpet along the table edges.
	for dx := 1; dx <= 5; dx++ {
		for _, dz := range []int{1, 4} {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, floorY+1, bz, p.grayCarpetID, gen)
			}
		}
	}
}

// placeLibrary adds bookshelves to the NE corner of the ground floor.
func (p *WoodlandMansionPlacer) placeLibrary(chunk *level.Chunk, localX, floorY, localZ, w int, gen *TerrainGenerator) {
	// Bookshelves along the north and east interior walls, 3 blocks high.
	for dy := 1; dy <= 3; dy++ {
		// North wall bookshelves.
		for dx := w - 6; dx <= w-2; dx++ {
			bx := localX + dx
			bz := localZ + 1
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, floorY+dy, bz, p.bookshelfID, gen)
			}
		}
		// East wall bookshelves.
		bx := localX + w - 2
		for dz := 1; dz <= 5; dz++ {
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, floorY+dy, bz, p.bookshelfID, gen)
			}
		}
	}

	// Chest with loot in the library corner.
	cx := localX + w - 3
	cz := localZ + 2
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setBlock(chunk, cx, floorY+1, cz, p.chestID, gen)
	}
}

// placeBedroom adds a carpet area and chest to the upper floor (SW corner).
func (p *WoodlandMansionPlacer) placeBedroom(chunk *level.Chunk, localX, floorY, localZ int, gen *TerrainGenerator) {
	upperFloor := floorY + 8 // first block above upper floor slab

	// White carpet area for the bed.
	for dx := 2; dx <= 4; dx++ {
		for dz := 7; dz <= 9; dz++ {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, upperFloor, bz, p.whiteCarpetID, gen)
			}
		}
	}

	// Chest at the foot of the bed area.
	cx := localX + 3
	cz := localZ + 10
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setBlock(chunk, cx, upperFloor, cz, p.chestID, gen)
	}
}

// placeAttic adds a peaked roof and some storage to the top of the mansion.
func (p *WoodlandMansionPlacer) placeAttic(chunk *level.Chunk, localX, floorY, localZ, w, d, h int, gen *TerrainGenerator) {
	roofBase := floorY + h

	// Simple peaked roof: narrows from each side by 1 block per layer.
	for dy := 0; dy < w/2; dy++ {
		by := roofBase + dy
		inset := dy + 1
		rw := w - inset*2
		if rw <= 0 {
			break
		}
		for dz := 0; dz < d; dz++ {
			// Only the edges of the narrowing roof.
			for _, dx := range []int{inset, w - 1 - inset} {
				bx := localX + dx
				bz := localZ + dz
				if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
					p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
				}
			}
		}
		// Cap each layer.
		if rw <= 2 {
			for dz := 0; dz < d; dz++ {
				for dx := inset; dx <= w-1-inset; dx++ {
					bx := localX + dx
					bz := localZ + dz
					if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
						p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
					}
				}
			}
		}
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *WoodlandMansionPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
