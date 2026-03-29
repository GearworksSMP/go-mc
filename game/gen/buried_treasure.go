package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// BuriedTreasurePlacer generates buried treasure structures at beach/ocean edges.
type BuriedTreasurePlacer struct {
	Seed int64

	chestID level.BlocksState
	sandID  level.BlocksState
	tntID   level.BlocksState
}

// NewBuriedTreasurePlacer creates a BuriedTreasurePlacer with resolved block state IDs.
func NewBuriedTreasurePlacer(seed int64) *BuriedTreasurePlacer {
	p := &BuriedTreasurePlacer{Seed: seed}

	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	p.sandID, _ = block.ToStateID[block.Sand{}]
	p.tntID, _ = block.ToStateID[block.Tnt{Unstable: false}]

	return p
}

// PlaceBuriedTreasure places a buried chest with optional TNT trap.
// localX, localZ are chunk-local coords. surfaceY is the terrain surface height.
func (p *BuriedTreasurePlacer) PlaceBuriedTreasure(chunk *level.Chunk, localX, surfaceY, localZ int, gen *TerrainGenerator) {
	chestY := surfaceY - 3
	if chestY < gen.MinY+1 {
		return
	}

	if localX < 0 || localX >= 16 || localZ < 0 || localZ >= 16 {
		return
	}

	p.setBlock(chunk, localX, chestY, localZ, p.chestID, gen)

	// Cover the chest with sand.
	for dy := 1; dy <= 3; dy++ {
		p.setBlock(chunk, localX, chestY+dy, localZ, p.sandID, gen)
	}

	// TNT trap below the chest (50% chance).
	trapHash := abs64(structureHash(localX, localZ, p.Seed, 0xB712))
	if trapHash%2 == 0 {
		p.setBlock(chunk, localX, chestY-1, localZ, p.tntID, gen)
	}

	// Sand around the chest to blend with terrain.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}
			for dy := 0; dy <= 3; dy++ {
				p.setBlock(chunk, bx, chestY+dy, bz, p.sandID, gen)
			}
		}
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *BuriedTreasurePlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
