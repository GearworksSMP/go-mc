package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// PillagerOutpostPlacer generates pillager outpost towers.
type PillagerOutpostPlacer struct {
	Seed int64

	darkOakPlanksID level.BlocksState
	cobblestoneID   level.BlocksState
	oakFenceID      level.BlocksState
	chestID         level.BlocksState
	whiteBannerID   level.BlocksState
	airID           level.BlocksState
}

// NewPillagerOutpostPlacer creates a PillagerOutpostPlacer with resolved block state IDs.
func NewPillagerOutpostPlacer(seed int64) *PillagerOutpostPlacer {
	p := &PillagerOutpostPlacer{Seed: seed, airID: 0}

	p.darkOakPlanksID, _ = block.ToStateID[block.DarkOakPlanks{}]
	p.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	p.oakFenceID, _ = block.ToStateID[block.OakFence{}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	p.whiteBannerID, _ = block.ToStateID[block.WhiteBanner{Rotation: 0}]

	return p
}

// PlaceOutpost places a 7x7x12 dark oak plank tower with cobblestone base.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
func (p *PillagerOutpostPlacer) PlaceOutpost(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	const (
		width  = 7
		depth  = 7
		baseH  = 3 // cobblestone base height
		totalH = 12
	)

	// Build the tower.
	for dy := 0; dy < totalH; dy++ {
		by := baseY + dy
		for dz := 0; dz < depth; dz++ {
			for dx := 0; dx < width; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isEdgeX := dx == 0 || dx == width-1
				isEdgeZ := dz == 0 || dz == depth-1
				isWall := isEdgeX || isEdgeZ
				isFloor := dy == 0
				isRoof := dy == totalH-1

				switch {
				case isFloor:
					p.setBlock(chunk, bx, by, bz, p.cobblestoneID, gen)
				case isRoof:
					p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
				case dy < baseH && isWall:
					p.setBlock(chunk, bx, by, bz, p.cobblestoneID, gen)
				case dy < baseH:
					// Interior of cobblestone base is air.
					p.setBlock(chunk, bx, by, bz, p.airID, gen)
				case isWall:
					// Door opening on the south side, at ground level of the plank section.
					if dz == depth-1 && dx == width/2 && dy >= baseH && dy <= baseH+1 {
						p.setBlock(chunk, bx, by, bz, p.airID, gen)
					} else {
						p.setBlock(chunk, bx, by, bz, p.darkOakPlanksID, gen)
					}
				default:
					p.setBlock(chunk, bx, by, bz, p.airID, gen)
				}
			}
		}
	}

	// Corner pillars: oak fence extending 2 blocks above the roof.
	pillarPositions := [4][2]int{{0, 0}, {0, depth - 1}, {width - 1, 0}, {width - 1, depth - 1}}
	for _, pp := range pillarPositions {
		bx := localX + pp[0]
		bz := localZ + pp[1]
		if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
			continue
		}
		for dy := totalH; dy < totalH+2; dy++ {
			p.setBlock(chunk, bx, baseY+dy, bz, p.oakFenceID, gen)
		}
	}

	// Banners on the exterior walls (one per side, at mid-tower height).
	bannerY := baseY + totalH - 2
	bannerPositions := [4][2]int{
		{width / 2, 0},         // north
		{width / 2, depth - 1}, // south
		{0, depth / 2},         // west
		{width - 1, depth / 2}, // east
	}
	for _, bp := range bannerPositions {
		bx := localX + bp[0]
		bz := localZ + bp[1]
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			p.setBlock(chunk, bx, bannerY, bz, p.whiteBannerID, gen)
		}
	}

	// Chest with loot inside, on the ground floor.
	chestX := localX + width/2
	chestZ := localZ + depth/2
	if chestX >= 0 && chestX < 16 && chestZ >= 0 && chestZ < 16 {
		p.setBlock(chunk, chestX, baseY+1, chestZ, p.chestID, gen)
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *PillagerOutpostPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
