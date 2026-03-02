package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// TemplePlacer generates desert and jungle temples.
type TemplePlacer struct {
	Seed int64

	// Block state IDs looked up at init time.
	sandstoneID          level.BlocksState
	cutSandstoneID       level.BlocksState
	orangeTerracottaID   level.BlocksState
	cobblestoneID        level.BlocksState
	mossyCobblestoneID   level.BlocksState
	chestID              level.BlocksState
	tntID                level.BlocksState
	stonePressurePlateID level.BlocksState
	mossyStoneBricksID   level.BlocksState
	stoneID              level.BlocksState
	leverID              level.BlocksState
	airID                level.BlocksState
}

// NewTemplePlacer creates a TemplePlacer with resolved block state IDs.
func NewTemplePlacer(seed int64) *TemplePlacer {
	tp := &TemplePlacer{Seed: seed}

	tp.sandstoneID, _ = block.ToStateID[block.Sandstone{}]
	tp.cutSandstoneID, _ = block.ToStateID[block.CutSandstone{}]
	tp.orangeTerracottaID, _ = block.ToStateID[block.OrangeTerracotta{}]
	tp.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	tp.mossyCobblestoneID, _ = block.ToStateID[block.MossyCobblestone{}]
	tp.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	tp.tntID, _ = block.ToStateID[block.Tnt{Unstable: false}]
	tp.stonePressurePlateID, _ = block.ToStateID[block.StonePressurePlate{Powered: false}]
	tp.mossyStoneBricksID, _ = block.ToStateID[block.MossyStoneBricks{}]
	tp.stoneID, _ = block.ToStateID[block.Stone{}]
	tp.leverID, _ = block.ToStateID[block.Lever{Face: block.AttachFaceWall, Facing: block.North, Powered: false}]
	tp.airID = 0

	return tp
}

// PlaceDesertTemple places a 9x9x8 sandstone pyramid structure.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
func (tp *TemplePlacer) PlaceDesertTemple(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	// The temple is 9 wide, 9 deep, 8 tall.
	// Layers 0-3: full 9x9 walls tapering inward.
	// Layer 4: orange terracotta band.
	// Layers 5-7: top pyramid narrowing.
	// Below floor: basement with TNT trap.

	for dy := 0; dy <= 7; dy++ {
		by := baseY + dy

		// Determine inset for pyramid taper.
		inset := 0
		if dy >= 5 {
			inset = dy - 4
		}

		width := 9 - inset*2
		if width <= 0 {
			break
		}

		for dz := 0; dz < width; dz++ {
			for dx := 0; dx < width; dx++ {
				bx := localX + inset + dx
				bz := localZ + inset + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isEdge := dx == 0 || dx == width-1 || dz == 0 || dz == width-1
				isFloor := dy == 0
				isRoof := dy == 7

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = tp.cutSandstoneID
				case isRoof:
					stateID = tp.sandstoneID
				case dy == 4 && isEdge:
					// Orange terracotta band at middle height.
					stateID = tp.orangeTerracottaID
				case isEdge:
					stateID = tp.sandstoneID
				default:
					// Interior is air.
					stateID = tp.airID
				}

				tp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Door opening on the south side, centered.
	doorX := localX + 4
	doorZ := localZ + 8
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		for dy := 1; dy <= 2; dy++ {
			tp.setBlock(chunk, doorX, baseY+dy, doorZ, tp.airID, gen)
		}
	}

	// Basement: 3x3 room below the floor at center.
	basementCX := localX + 4
	basementCZ := localZ + 4
	basementY := baseY - 4 // basement floor

	for dy := 0; dy <= 3; dy++ {
		for dz := -1; dz <= 1; dz++ {
			for dx := -1; dx <= 1; dx++ {
				bx := basementCX + dx
				bz := basementCZ + dz
				by := basementY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == -1 || dx == 1 || dz == -1 || dz == 1
				isFloor := dy == 0
				isCeiling := dy == 3

				if isFloor || isCeiling {
					tp.setBlock(chunk, bx, by, bz, tp.sandstoneID, gen)
				} else if isWall {
					tp.setBlock(chunk, bx, by, bz, tp.sandstoneID, gen)
				} else {
					tp.setBlock(chunk, bx, by, bz, tp.airID, gen)
				}
			}
		}
	}

	// TNT under pressure plate in the center of the basement.
	if basementCX >= 0 && basementCX < 16 && basementCZ >= 0 && basementCZ < 16 {
		tp.setBlock(chunk, basementCX, basementY+1, basementCZ, tp.tntID, gen)
		tp.setBlock(chunk, basementCX, basementY+2, basementCZ, tp.stonePressurePlateID, gen)
	}

	// 4 chests in the basement corners.
	chestOffsets := [4][2]int{{-1, -1}, {-1, 1}, {1, -1}, {1, 1}}
	for _, off := range chestOffsets {
		cx := basementCX + off[0]
		cz := basementCZ + off[1]
		if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
			tp.setBlock(chunk, cx, basementY+1, cz, tp.chestID, gen)
		}
	}
}

// PlaceJungleTemple places a 7x7x8 cobblestone/mossy cobblestone structure.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
func (tp *TemplePlacer) PlaceJungleTemple(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	for dy := 0; dy <= 7; dy++ {
		by := baseY + dy
		for dz := 0; dz < 7; dz++ {
			for dx := 0; dx < 7; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isEdge := dx == 0 || dx == 6 || dz == 0 || dz == 6
				isFloor := dy == 0
				isRoof := dy == 7

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = tp.cobblestoneID
				case isRoof:
					stateID = tp.mossyCobblestoneID
				case isEdge:
					// Alternate cobblestone and mossy cobblestone for vine-covered look.
					h := structureHash(dx, dz, tp.Seed, 0x4444)
					if abs64(h)%3 == 0 {
						stateID = tp.mossyCobblestoneID
					} else {
						stateID = tp.cobblestoneID
					}
				default:
					stateID = tp.airID
				}

				tp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Door opening on the south side.
	doorX := localX + 3
	doorZ := localZ + 6
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		for dy := 1; dy <= 2; dy++ {
			tp.setBlock(chunk, doorX, baseY+dy, doorZ, tp.airID, gen)
		}
	}

	// 2 chests inside.
	chest1X := localX + 1
	chest1Z := localZ + 1
	if chest1X >= 0 && chest1X < 16 && chest1Z >= 0 && chest1Z < 16 {
		tp.setBlock(chunk, chest1X, baseY+1, chest1Z, tp.chestID, gen)
	}

	chest2X := localX + 5
	chest2Z := localZ + 5
	if chest2X >= 0 && chest2X < 16 && chest2Z >= 0 && chest2Z < 16 {
		tp.setBlock(chunk, chest2X, baseY+1, chest2Z, tp.chestID, gen)
	}

	// Lever on the east wall as a puzzle hint.
	leverX := localX + 6
	leverZ := localZ + 3
	if leverX >= 0 && leverX < 16 && leverZ >= 0 && leverZ < 16 {
		tp.setBlock(chunk, leverX, baseY+2, leverZ, tp.leverID, gen)
	}
}

// setBlock sets a block in the chunk at local coordinates (x 0-15, z 0-15, worldY).
func (tp *TemplePlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
