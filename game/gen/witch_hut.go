package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// WitchHutPlacer generates witch hut structures in taiga/swamp biomes.
type WitchHutPlacer struct {
	Seed int64

	sprucePlanksID  level.BlocksState
	oakFenceID      level.BlocksState
	cauldronID      level.BlocksState
	craftingTableID level.BlocksState
	flowerPotID     level.BlocksState
	airID           level.BlocksState
}

// NewWitchHutPlacer creates a WitchHutPlacer with resolved block state IDs.
func NewWitchHutPlacer(seed int64) *WitchHutPlacer {
	p := &WitchHutPlacer{Seed: seed, airID: 0}

	p.sprucePlanksID, _ = block.ToStateID[block.SprucePlanks{}]
	p.oakFenceID, _ = block.ToStateID[block.OakFence{}]
	p.cauldronID, _ = block.ToStateID[block.Cauldron{}]
	p.craftingTableID, _ = block.ToStateID[block.CraftingTable{}]
	p.flowerPotID, _ = block.ToStateID[block.FlowerPot{}]

	return p
}

// PlaceWitchHut places a 7x7x6 spruce wood hut on oak fence stilts.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
func (p *WitchHutPlacer) PlaceWitchHut(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	const (
		width     = 7
		depth     = 7
		stiltH    = 3 // stilts are 3 blocks tall
		hutH      = 4 // hut interior + walls + roof
		floorY    = stiltH // relative to baseY
		interiorH = 2      // 2 blocks of interior space
	)

	// Place stilts: oak fence posts at the four corners.
	stiltPositions := [4][2]int{{0, 0}, {0, depth - 1}, {width - 1, 0}, {width - 1, depth - 1}}
	for _, sp := range stiltPositions {
		bx := localX + sp[0]
		bz := localZ + sp[1]
		if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
			continue
		}
		for dy := 0; dy < stiltH; dy++ {
			p.setBlock(chunk, bx, baseY+dy, bz, p.oakFenceID, gen)
		}
	}

	// Build the hut: floor, walls, and roof.
	for dy := 0; dy < hutH; dy++ {
		by := baseY + floorY + dy
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
				isRoof := dy == hutH-1

				switch {
				case isFloor || isRoof:
					p.setBlock(chunk, bx, by, bz, p.sprucePlanksID, gen)
				case isWall:
					// Door opening on the south side, centered.
					if dz == depth-1 && dx == width/2 && dy <= interiorH {
						p.setBlock(chunk, bx, by, bz, p.airID, gen)
					} else {
						p.setBlock(chunk, bx, by, bz, p.sprucePlanksID, gen)
					}
				default:
					p.setBlock(chunk, bx, by, bz, p.airID, gen)
				}
			}
		}
	}

	// Interior furnishings: cauldron, crafting table, flower pot.
	interiorY := baseY + floorY + 1

	// Cauldron in the NW corner.
	cx := localX + 1
	cz := localZ + 1
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setBlock(chunk, cx, interiorY, cz, p.cauldronID, gen)
	}

	// Crafting table in the NE corner.
	ctx := localX + width - 2
	ctz := localZ + 1
	if ctx >= 0 && ctx < 16 && ctz >= 0 && ctz < 16 {
		p.setBlock(chunk, ctx, interiorY, ctz, p.craftingTableID, gen)
	}

	// Flower pot in the SE corner.
	fpx := localX + width - 2
	fpz := localZ + depth - 2
	if fpx >= 0 && fpx < 16 && fpz >= 0 && fpz < 16 {
		p.setBlock(chunk, fpx, interiorY, fpz, p.flowerPotID, gen)
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *WitchHutPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
