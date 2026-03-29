package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// RuinedPortalPlacer generates ruined nether portal structures.
type RuinedPortalPlacer struct {
	Seed int64

	obsidianID       level.BlocksState
	cryingObsidianID level.BlocksState
	netherrackID     level.BlocksState
	magmaBlockID     level.BlocksState
	chestID          level.BlocksState
	airID            level.BlocksState

	hasCryingObsidian bool
}

// NewRuinedPortalPlacer creates a RuinedPortalPlacer with resolved block state IDs.
func NewRuinedPortalPlacer(seed int64) *RuinedPortalPlacer {
	p := &RuinedPortalPlacer{Seed: seed, airID: 0}

	p.obsidianID, _ = block.ToStateID[block.Obsidian{}]
	p.netherrackID, _ = block.ToStateID[block.Netherrack{}]
	p.magmaBlockID, _ = block.ToStateID[block.MagmaBlock{}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]

	var ok bool
	p.cryingObsidianID, ok = block.ToStateID[block.CryingObsidian{}]
	p.hasCryingObsidian = ok

	return p
}

// PlaceRuinedPortal places a ruined nether portal structure.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ground level.
func (p *RuinedPortalPlacer) PlaceRuinedPortal(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	// Determine if the portal is half-buried (50% chance).
	buriedHash := abs64(structureHash(localX, localZ, p.Seed, 0xD0A1))
	halfBuried := buriedHash%2 == 0
	portalY := baseY
	if halfBuried {
		portalY = baseY - 2
	}

	// Place netherrack + magma block base (5x3 area around the portal).
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 5; dx++ {
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}
			// Alternate netherrack and magma blocks.
			h := abs64(structureHash(dx, dz, p.Seed, 0xD0A2))
			if h%3 == 0 {
				p.setBlock(chunk, bx, portalY, bz, p.magmaBlockID, gen)
			} else {
				p.setBlock(chunk, bx, portalY, bz, p.netherrackID, gen)
			}
		}
	}

	// Build the obsidian portal frame: 4 wide x 5 tall.
	// Frame blocks are the border of the 4x5 rectangle.
	frameBlocks := make([][2]int, 0, 14)
	for dy := 0; dy < 5; dy++ {
		for dx := 0; dx < 4; dx++ {
			isEdge := dx == 0 || dx == 3 || dy == 0 || dy == 4
			if isEdge {
				frameBlocks = append(frameBlocks, [2]int{dx, dy})
			}
		}
	}

	// Determine which blocks to remove (2-4 random blocks from the frame).
	removeHash := abs64(structureHash(localX, localZ, p.Seed, 0xD0A3))
	removeCount := 2 + int(removeHash%3)
	removed := make(map[int]bool)
	for i := 0; i < removeCount; i++ {
		rh := abs64(structureHash(localX+i, localZ, p.Seed, 0xD0A4+int64(i)))
		idx := int(rh % int64(len(frameBlocks)))
		removed[idx] = true
	}

	// Determine which obsidian blocks get replaced with crying obsidian (1-2).
	cryHash := abs64(structureHash(localX, localZ, p.Seed, 0xD0A5))
	cryCount := 1 + int(cryHash%2)
	crying := make(map[int]bool)
	if p.hasCryingObsidian {
		for i := 0; i < cryCount; i++ {
			ch := abs64(structureHash(localX, localZ+i, p.Seed, 0xD0A6+int64(i)))
			idx := int(ch % int64(len(frameBlocks)))
			if !removed[idx] {
				crying[idx] = true
			}
		}
	}

	// Place the frame.
	for i, fb := range frameBlocks {
		if removed[i] {
			continue
		}
		bx := localX + fb[0]
		by := portalY + 1 + fb[1]
		bz := localZ
		if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
			continue
		}

		if crying[i] {
			p.setBlock(chunk, bx, by, bz, p.cryingObsidianID, gen)
		} else {
			p.setBlock(chunk, bx, by, bz, p.obsidianID, gen)
		}
	}

	// Clear the portal interior (2x3 air blocks).
	for dy := 1; dy <= 3; dy++ {
		for dx := 1; dx <= 2; dx++ {
			bx := localX + dx
			bz := localZ
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				p.setBlock(chunk, bx, portalY+1+dy, bz, p.airID, gen)
			}
		}
	}

	// Place a loot chest adjacent to the portal.
	chestX := localX + 4
	chestZ := localZ
	if chestX >= 0 && chestX < 16 && chestZ >= 0 && chestZ < 16 {
		p.setBlock(chunk, chestX, portalY+1, chestZ, p.chestID, gen)
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *RuinedPortalPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
