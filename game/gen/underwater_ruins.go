package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// UnderwaterRuinsPlacer generates underwater ruin structures in ocean biomes.
type UnderwaterRuinsPlacer struct {
	Seed int64

	stoneBricksID      level.BlocksState
	mossyStoneBricksID level.BlocksState
	crackedStoneBricksID level.BlocksState
	chestID level.BlocksState
	waterID level.BlocksState
}

// NewUnderwaterRuinsPlacer creates an UnderwaterRuinsPlacer with resolved block state IDs.
func NewUnderwaterRuinsPlacer(seed int64, waterID level.BlocksState) *UnderwaterRuinsPlacer {
	p := &UnderwaterRuinsPlacer{Seed: seed, waterID: waterID}

	p.stoneBricksID, _ = block.ToStateID[block.StoneBricks{}]
	p.mossyStoneBricksID, _ = block.ToStateID[block.MossyStoneBricks{}]
	p.crackedStoneBricksID, _ = block.ToStateID[block.CrackedStoneBricks{}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: true}]

	return p
}

// PlaceUnderwaterRuins places an underwater ruin at the given position.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ocean floor level.
func (p *UnderwaterRuinsPlacer) PlaceUnderwaterRuins(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	// Determine variant: small (60%) or large (40%).
	variantHash := abs64(structureHash(localX, localZ, p.Seed, 0xD170))
	if variantHash%5 < 3 {
		p.placeSmallRuin(chunk, localX, baseY, localZ, gen)
	} else {
		p.placeLargeRuin(chunk, localX, baseY, localZ, gen)
	}
}

// placeSmallRuin places a 5x5x4 partially collapsed stone brick structure.
func (p *UnderwaterRuinsPlacer) placeSmallRuin(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	collapseHash := abs64(structureHash(localX, localZ, p.Seed, 0xD171))

	for dy := 0; dy < 4; dy++ {
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 5; dx++ {
				bx := localX + dx
				bz := localZ + dz
				by := baseY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 4
				isFloor := dy == 0
				isCeiling := dy == 3

				if !isFloor && !isCeiling && !isWall {
					// Interior: fill with water.
					p.setBlock(chunk, bx, by, bz, p.waterID, gen)
					continue
				}

				// Random collapse: ~30% of wall/ceiling blocks are missing.
				blockHash := abs64(structureHash(dx+bx, dy+by, p.Seed+collapseHash, 0xD172))
				if !isFloor && blockHash%10 < 3 {
					continue
				}

				p.setBlock(chunk, bx, by, bz, p.ruinBlock(blockHash), gen)
			}
		}
	}

	// Place a chest inside (center of floor).
	cx := localX + 2
	cz := localZ + 2
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setBlock(chunk, cx, baseY+1, cz, p.chestID, gen)
	}
}

// placeLargeRuin places a 9x9x6 ruin with multiple rooms, columns, and chests.
func (p *UnderwaterRuinsPlacer) placeLargeRuin(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	collapseHash := abs64(structureHash(localX, localZ, p.Seed, 0xD173))

	for dy := 0; dy < 6; dy++ {
		for dz := 0; dz < 9; dz++ {
			for dx := 0; dx < 9; dx++ {
				bx := localX + dx
				bz := localZ + dz
				by := baseY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isOuterWall := dx == 0 || dx == 8 || dz == 0 || dz == 8
				isInnerWall := dx == 4 || dz == 4
				isFloor := dy == 0
				isMiddleFloor := dy == 3
				isCeiling := dy == 5
				isColumn := (dx == 2 || dx == 6) && (dz == 2 || dz == 6)

				isStructural := isFloor || isCeiling || isMiddleFloor || isOuterWall || isInnerWall || isColumn

				if !isStructural {
					// Interior: fill with water.
					p.setBlock(chunk, bx, by, bz, p.waterID, gen)
					continue
				}

				// Inner walls have doorway openings.
				if isInnerWall && !isFloor && !isCeiling && !isMiddleFloor {
					// Doorways at center of inner walls.
					if dx == 4 && (dz == 2 || dz == 6) && dy >= 1 && dy <= 2 {
						p.setBlock(chunk, bx, by, bz, p.waterID, gen)
						continue
					}
					if dz == 4 && (dx == 2 || dx == 6) && dy >= 1 && dy <= 2 {
						p.setBlock(chunk, bx, by, bz, p.waterID, gen)
						continue
					}
				}

				// Random collapse: ~25% of non-floor blocks.
				blockHash := abs64(structureHash(dx+bx, dy+by, p.Seed+collapseHash, 0xD174))
				if !isFloor && !isColumn && blockHash%4 == 0 {
					continue
				}

				p.setBlock(chunk, bx, by, bz, p.ruinBlock(blockHash), gen)
			}
		}
	}

	// Place chests in two rooms.
	chestPositions := [][2]int{{2, 2}, {6, 6}}
	for _, cp := range chestPositions {
		cx := localX + cp[0]
		cz := localZ + cp[1]
		if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
			p.setBlock(chunk, cx, baseY+1, cz, p.chestID, gen)
		}
	}
}

// ruinBlock picks a weathered stone brick variant based on a hash value.
// Mossy 40%, cracked 20%, regular 40%.
func (p *UnderwaterRuinsPlacer) ruinBlock(hash int64) level.BlocksState {
	switch hash % 5 {
	case 0, 1:
		return p.mossyStoneBricksID
	case 2:
		return p.crackedStoneBricksID
	default:
		return p.stoneBricksID
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *UnderwaterRuinsPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
