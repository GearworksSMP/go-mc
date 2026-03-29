package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// OceanMonumentPlacer generates simplified ocean monument structures.
type OceanMonumentPlacer struct {
	Seed int64

	prismarineID     level.BlocksState
	darkPrismarineID level.BlocksState
	seaLanternID     level.BlocksState
	spongeID         level.BlocksState
	spawnerID        level.BlocksState
	waterID          level.BlocksState
	airID            level.BlocksState

	hasSpawner bool
}

// NewOceanMonumentPlacer creates an OceanMonumentPlacer with resolved block state IDs.
func NewOceanMonumentPlacer(seed int64, waterID level.BlocksState) *OceanMonumentPlacer {
	p := &OceanMonumentPlacer{Seed: seed, waterID: waterID, airID: 0}

	p.prismarineID, _ = block.ToStateID[block.Prismarine{}]
	p.darkPrismarineID, _ = block.ToStateID[block.DarkPrismarine{}]
	p.seaLanternID, _ = block.ToStateID[block.SeaLantern{}]
	p.spongeID, _ = block.ToStateID[block.Sponge{}]

	var ok bool
	p.spawnerID, ok = block.ToStateID[block.Spawner{}]
	p.hasSpawner = ok

	return p
}

// PlaceOceanMonument places a simplified 15x15x10 prismarine monument.
// The full vanilla monument is 58x58; this is a recognizable miniature version
// that fits within a single chunk. localX, localZ are the NW corner.
func (p *OceanMonumentPlacer) PlaceOceanMonument(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	const (
		width  = 15
		depth  = 15
		height = 10
	)

	// Build the main structure: dark prismarine frame with prismarine fill.
	for dy := 0; dy < height; dy++ {
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
				isFloor := dy == 0
				isRoof := dy == height-1

				// Frame edges: where two axes meet.
				isFrame := (isEdgeX && isEdgeZ) || (isEdgeX && isFloor) || (isEdgeZ && isFloor) ||
					(isEdgeX && isRoof) || (isEdgeZ && isRoof)

				isWall := isEdgeX || isEdgeZ

				var stateID level.BlocksState
				switch {
				case isFrame:
					stateID = p.darkPrismarineID
				case isFloor || isRoof || isWall:
					stateID = p.prismarineID
				default:
					// Interior filled with water.
					stateID = p.waterID
				}

				p.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Sea lanterns at the four top corners.
	corners := [4][2]int{{1, 1}, {1, depth - 2}, {width - 2, 1}, {width - 2, depth - 2}}
	for _, c := range corners {
		bx := localX + c[0]
		bz := localZ + c[1]
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			p.setBlock(chunk, bx, baseY+height-1, bz, p.seaLanternID, gen)
			// Also place lanterns at mid-height for interior lighting.
			p.setBlock(chunk, bx, baseY+height/2, bz, p.seaLanternID, gen)
		}
	}

	// Sponge room: small 3x3x3 chamber in one corner of the interior.
	spongeBaseX := localX + 2
	spongeBaseZ := localZ + 2
	spongeBaseY := baseY + 1
	for dy := 0; dy < 3; dy++ {
		for dz := 0; dz < 3; dz++ {
			for dx := 0; dx < 3; dx++ {
				bx := spongeBaseX + dx
				bz := spongeBaseZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}
				isEdge := dx == 0 || dx == 2 || dz == 0 || dz == 2 || dy == 0 || dy == 2
				if isEdge {
					p.setBlock(chunk, bx, spongeBaseY+dy, bz, p.spongeID, gen)
				}
			}
		}
	}

	// Elder guardian spawner room: center of the monument.
	centerX := localX + width/2
	centerZ := localZ + depth/2
	spawnerY := baseY + height/2
	if centerX >= 0 && centerX < 16 && centerZ >= 0 && centerZ < 16 {
		if p.hasSpawner {
			p.setBlock(chunk, centerX, spawnerY, centerZ, p.spawnerID, gen)
		} else {
			p.setBlock(chunk, centerX, spawnerY, centerZ, p.darkPrismarineID, gen)
		}
		// Clear space around the spawner.
		for _, off := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			sx := centerX + off[0]
			sz := centerZ + off[1]
			if sx >= 0 && sx < 16 && sz >= 0 && sz < 16 {
				p.setBlock(chunk, sx, spawnerY, sz, p.waterID, gen)
			}
		}
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *OceanMonumentPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
