package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// MineshaftPlacer generates abandoned mineshaft corridors underground.
type MineshaftPlacer struct {
	Seed int64

	// Block state IDs looked up at init time.
	oakPlanksID level.BlocksState
	oakFenceID  level.BlocksState
	railID      level.BlocksState
	torchID     level.BlocksState
	cobwebID    level.BlocksState
	spawnerID   level.BlocksState
	airID       level.BlocksState

	hasSpawner bool
}

// NewMineshaftPlacer creates a MineshaftPlacer with resolved block state IDs.
func NewMineshaftPlacer(seed int64) *MineshaftPlacer {
	mp := &MineshaftPlacer{Seed: seed}

	mp.oakPlanksID, _ = block.ToStateID[block.OakPlanks{}]
	mp.oakFenceID, _ = block.ToStateID[block.OakFence{}]
	mp.railID, _ = block.ToStateID[block.Rail{Shape: block.RailShapeNorthSouth, Waterlogged: false}]
	mp.torchID, _ = block.ToStateID[block.Torch{}]
	mp.cobwebID, _ = block.ToStateID[block.Cobweb{}]
	mp.airID = 0

	var ok bool
	mp.spawnerID, ok = block.ToStateID[block.Spawner{}]
	mp.hasSpawner = ok

	return mp
}

// PlaceMineshaft generates a mineshaft corridor in the chunk if conditions are met.
// Returns true if a mineshaft was placed.
func (mp *MineshaftPlacer) PlaceMineshaft(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) bool {
	// 1% chance per chunk.
	h := structureHash(chunkX, chunkZ, mp.Seed, 0xA1A1)
	if abs64(h)%100 != 0 {
		return false
	}

	// Determine corridor direction: N-S (along Z axis) or E-W (along X axis).
	dirHash := structureHash(chunkX, chunkZ, mp.Seed, 0xA2A2)
	isNorthSouth := abs64(dirHash)%2 == 0

	// Corridor length: 8-14 blocks.
	lenHash := structureHash(chunkX, chunkZ, mp.Seed, 0xA3A3)
	corridorLen := 8 + int(abs64(lenHash)%7)

	// Y level: between MinY+10 and 40.
	yHash := structureHash(chunkX, chunkZ, mp.Seed, 0xA4A4)
	corridorY := 10 + int(abs64(yHash)%31) // Y 10-40

	// Starting position within chunk (avoid edges for the 3-wide corridor).
	startHash := structureHash(chunkX, chunkZ, mp.Seed, 0xA5A5)

	var startX, startZ int
	if isNorthSouth {
		startX = 2 + int(abs64(startHash)%11) // 2..12 to keep 3-wide within 0..15
		startZ = 1
		if corridorLen > 14 {
			corridorLen = 14
		}
	} else {
		startX = 1
		startZ = 2 + int(abs64(startHash)%11)
		if corridorLen > 14 {
			corridorLen = 14
		}
	}

	// Rail state depends on direction.
	railState := mp.railID
	if !isNorthSouth {
		railState, _ = block.ToStateID[block.Rail{Shape: block.RailShapeEastWest, Waterlogged: false}]
	}

	// Generate the corridor.
	for i := 0; i < corridorLen; i++ {
		var cx, cz int
		if isNorthSouth {
			cx = startX
			cz = startZ + i
		} else {
			cx = startX + i
			cz = startZ
		}

		// Place 3-wide corridor cross-section.
		for w := -1; w <= 1; w++ {
			var bx, bz int
			if isNorthSouth {
				bx = cx + w
				bz = cz
			} else {
				bx = cx
				bz = cz + w
			}

			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}

			// Floor: oak planks.
			mp.setBlock(chunk, bx, corridorY, bz, mp.oakPlanksID, gen)

			// Air for the corridor interior (3 blocks high).
			for dy := 1; dy <= 3; dy++ {
				mp.setBlock(chunk, bx, corridorY+dy, bz, mp.airID, gen)
			}

			// Ceiling: oak planks.
			mp.setBlock(chunk, bx, corridorY+4, bz, mp.oakPlanksID, gen)
		}

		// Support pillars every 4 blocks (oak fence from floor+1 to floor+3).
		if i%4 == 0 {
			for w := -1; w <= 1; w += 2 { // only at the edges
				var px, pz int
				if isNorthSouth {
					px = cx + w
					pz = cz
				} else {
					px = cx
					pz = cz + w
				}

				if px < 0 || px >= 16 || pz < 0 || pz >= 16 {
					continue
				}

				for dy := 1; dy <= 3; dy++ {
					mp.setBlock(chunk, px, corridorY+dy, pz, mp.oakFenceID, gen)
				}
			}

			// Cross-beam at top between pillars.
			var bx, bz int
			if isNorthSouth {
				bx = cx
				bz = cz
			} else {
				bx = cx
				bz = cz
			}
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				mp.setBlock(chunk, bx, corridorY+3, bz, mp.oakPlanksID, gen)
			}
		}

		// Rails on the center of the floor.
		var rx, rz int
		if isNorthSouth {
			rx = cx
			rz = cz
		} else {
			rx = cx
			rz = cz
		}
		if rx >= 0 && rx < 16 && rz >= 0 && rz < 16 {
			mp.setBlock(chunk, rx, corridorY+1, rz, railState, gen)
		}

		// Occasional cobwebs (15% chance per position).
		webHash := structureHash(cx+i, cz, mp.Seed, 0xA6A6)
		if abs64(webHash)%100 < 15 {
			// Place cobweb on the ceiling edge.
			var wx, wz int
			if isNorthSouth {
				wx = cx + 1
				if abs64(webHash)%2 == 0 {
					wx = cx - 1
				}
				wz = cz
			} else {
				wx = cx
				wz = cz + 1
				if abs64(webHash)%2 == 0 {
					wz = cz - 1
				}
			}
			if wx >= 0 && wx < 16 && wz >= 0 && wz < 16 {
				mp.setBlock(chunk, wx, corridorY+3, wz, mp.cobwebID, gen)
			}
		}

		// Occasional torches (10% chance per position).
		torchHash := structureHash(cx, cz+i, mp.Seed, 0xA7A7)
		if abs64(torchHash)%100 < 10 {
			var tx, tz int
			if isNorthSouth {
				tx = cx - 1
				tz = cz
			} else {
				tx = cx
				tz = cz - 1
			}
			if tx >= 0 && tx < 16 && tz >= 0 && tz < 16 {
				mp.setBlock(chunk, tx, corridorY+1, tz, mp.torchID, gen)
			}
		}
	}

	// Rare cave spider spawner (10% of mineshaft chunks).
	spawnerHash := structureHash(chunkX, chunkZ, mp.Seed, 0xA8A8)
	if abs64(spawnerHash)%10 == 0 && mp.hasSpawner {
		sx := startX
		var sz int
		if isNorthSouth {
			sz = startZ + corridorLen/2
		} else {
			sz = startZ
			sx = startX + corridorLen/2
		}
		if sx >= 0 && sx < 16 && sz >= 0 && sz < 16 {
			mp.setBlock(chunk, sx, corridorY+1, sz, mp.spawnerID, gen)
		}
	}

	return true
}

// setBlock sets a block in the chunk at local coordinates (x 0-15, z 0-15, worldY).
func (mp *MineshaftPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
