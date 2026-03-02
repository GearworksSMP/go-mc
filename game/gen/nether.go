package gen

import (
	"math/bits"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// NetherGenerator creates Nether terrain with netherrack, lava seas, and 3D caves.
type NetherGenerator struct {
	Seed     int64
	MinY     int // 0
	Sections int // 8 (128 blocks height)

	heightNoise, caveNoise, patchNoise *SimplexNoise

	// Block state IDs
	netherrackID      level.BlocksState
	lavaID            level.BlocksState
	soulSandID        level.BlocksState
	glowstoneID       level.BlocksState
	netherQuartzOreID level.BlocksState
	bedrockID         level.BlocksState
	airID             level.BlocksState
	magmaBlockID      level.BlocksState
	gravelID          level.BlocksState
	netherGoldOreID   level.BlocksState
}

// NewNetherGenerator creates a nether terrain generator with the given seed.
func NewNetherGenerator(seed int64) *NetherGenerator {
	g := &NetherGenerator{
		Seed:     seed,
		MinY:     0,
		Sections: 8, // 128 blocks height

		heightNoise: NewSimplexNoise(seed + 100),
		caveNoise:   NewSimplexNoise(seed + 101),
		patchNoise:  NewSimplexNoise(seed + 102),
	}

	g.bedrockID, _ = block.ToStateID[block.Bedrock{}]
	g.netherrackID, _ = block.ToStateID[block.Netherrack{}]
	g.lavaID, _ = block.ToStateID[block.Lava{Level: 0}]
	g.soulSandID, _ = block.ToStateID[block.SoulSand{}]
	g.glowstoneID, _ = block.ToStateID[block.Glowstone{}]
	g.netherQuartzOreID, _ = block.ToStateID[block.NetherQuartzOre{}]
	g.magmaBlockID, _ = block.ToStateID[block.MagmaBlock{}]
	g.gravelID, _ = block.ToStateID[block.Gravel{}]
	// air is state 0 by default
	g.airID = 0

	// Nether gold ore may or may not exist in block registry
	g.netherGoldOreID, _ = block.ToStateID[block.GoldOre{}]

	return g
}

// SpawnY returns the Y coordinate for spawning in the nether.
func (g *NetherGenerator) SpawnY() float64 {
	return 65
}

// Generate creates a nether chunk at the given position.
func (g *NetherGenerator) Generate(pos game.ChunkPos) *level.Chunk {
	chunk := level.EmptyChunk(g.Sections)

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z
			fWorldX := float64(worldX)
			fWorldZ := float64(worldZ)

			// Use noise to vary the floor and ceiling height
			floorNoise := g.heightNoise.Octave2D(fWorldX/48.0, fWorldZ/48.0, 3, 2.0, 0.5)
			ceilNoise := g.heightNoise.Octave2D(fWorldX/48.0+100, fWorldZ/48.0+100, 3, 2.0, 0.5)

			floorH := 32 + int(floorNoise*12)  // floor surface varies 20-44
			ceilH := 100 + int(ceilNoise*16)    // ceiling varies 84-116

			if floorH < 5 {
				floorH = 5
			}
			if ceilH > 125 {
				ceilH = 125
			}
			if ceilH < floorH+10 {
				ceilH = floorH + 10
			}

			for worldY := 0; worldY < g.Sections*16; worldY++ {
				var stateID level.BlocksState

				switch {
				case worldY == 0:
					// Bedrock floor
					stateID = g.bedrockID
				case worldY <= 4:
					// Bedrock mixed layer at bottom
					h := posHash(worldX, worldY, worldZ, g.Seed+200)
					if h%uint64(worldY+1) == 0 {
						stateID = g.bedrockID
					} else {
						stateID = g.netherrackID
					}
				case worldY >= 123:
					// Bedrock mixed layer at top
					h := posHash(worldX, worldY, worldZ, g.Seed+201)
					if h%uint64(128-worldY) == 0 {
						stateID = g.bedrockID
					} else {
						stateID = g.netherrackID
					}
				case worldY == 127:
					// Bedrock ceiling
					stateID = g.bedrockID
				case worldY <= floorH:
					// Solid netherrack floor
					stateID = g.netherrackID
				case worldY >= ceilH:
					// Solid netherrack ceiling
					stateID = g.netherrackID
				default:
					// Open area between floor and ceiling - carve caves
					cv := g.caveNoise.Noise3D(fWorldX/20.0, float64(worldY)/20.0, fWorldZ/20.0)

					if cv > 0.1 {
						// Open space (cave/air)
						if worldY <= 31 {
							// Lava sea level
							stateID = g.lavaID
						} else {
							continue // air
						}
					} else {
						// Solid netherrack
						stateID = g.netherrackID
					}
				}

				// Apply ore/patch replacements for netherrack blocks
				if stateID == g.netherrackID {
					stateID = g.applyNetherFeatures(worldX, worldY, worldZ, stateID)
				}

				secIdx := worldY / 16
				localY := worldY % 16
				idx := localY*16*16 + z*16 + x
				chunk.Sections[secIdx].SetBlock(idx, stateID)
			}
		}
	}

	// Add glowstone clusters on the ceiling
	g.placeGlowstone(chunk, pos)

	g.computeHeightmaps(chunk)

	return chunk
}

// applyNetherFeatures replaces netherrack with soul sand, quartz, magma, or gravel.
func (g *NetherGenerator) applyNetherFeatures(x, y, z int, base level.BlocksState) level.BlocksState {
	h := posHash(x, y, z, g.Seed+300)

	// Soul sand patches at Y=32-40
	if y >= 32 && y <= 40 {
		sv := g.patchNoise.Noise2D(float64(x)*0.05, float64(z)*0.05)
		if sv > 0.4 && h%3 == 0 {
			return g.soulSandID
		}
	}

	// Magma blocks near lava level (Y=28-34)
	if y >= 28 && y <= 34 {
		if h%12 == 0 {
			return g.magmaBlockID
		}
	}

	// Gravel patches at Y=60-80
	if y >= 60 && y <= 80 {
		gv := g.patchNoise.Noise2D(float64(x)*0.04+50, float64(z)*0.04+50)
		if gv > 0.5 && h%4 == 0 {
			return g.gravelID
		}
	}

	// Nether quartz ore scattered at Y=10-117
	if y >= 10 && y <= 117 {
		if h%40 == 0 {
			return g.netherQuartzOreID
		}
	}

	// Nether gold ore (rare) at Y=15-115
	if y >= 15 && y <= 115 && g.netherGoldOreID != 0 {
		h2 := (h >> 7) | (h << 57)
		if h2%120 == 0 {
			return g.netherGoldOreID
		}
	}

	return base
}

// placeGlowstone adds glowstone clusters on the ceiling.
func (g *NetherGenerator) placeGlowstone(chunk *level.Chunk, pos game.ChunkPos) {
	for z := 1; z < 15; z++ {
		for x := 1; x < 15; x++ {
			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z

			h := posHash(worldX, 0, worldZ, g.Seed+400)
			if h%25 != 0 {
				continue
			}

			// Find the ceiling (scan down from top)
			ceilY := -1
			for y := 120; y >= 50; y-- {
				secIdx := y / 16
				localY := y % 16
				idx := localY*16*16 + z*16 + x
				state := chunk.Sections[secIdx].GetBlock(idx)
				if state == g.netherrackID {
					ceilY = y
					break
				}
			}

			if ceilY < 50 {
				continue
			}

			// Place a small cluster of glowstone hanging from the ceiling
			clusterSize := 1 + int(h/25)%3
			for dy := 0; dy < clusterSize; dy++ {
				gy := ceilY - dy
				if gy < 0 {
					break
				}
				secIdx := gy / 16
				localY := gy % 16
				idx := localY*16*16 + z*16 + x
				chunk.Sections[secIdx].SetBlock(idx, g.glowstoneID)

				// Small spread on the bottom piece
				if dy == clusterSize-1 {
					for _, off := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
						nx, nz := x+off[0], z+off[1]
						if nx < 0 || nx >= 16 || nz < 0 || nz >= 16 {
							continue
						}
						h2 := posHash(worldX+off[0], gy, worldZ+off[1], g.Seed+401)
						if h2%3 == 0 {
							idx2 := localY*16*16 + nz*16 + nx
							chunk.Sections[secIdx].SetBlock(idx2, g.glowstoneID)
						}
					}
				}
			}
		}
	}
}

// computeHeightmaps calculates heightmaps for the nether chunk.
func (g *NetherGenerator) computeHeightmaps(chunk *level.Chunk) {
	bitsForHeight := bits.Len(uint(g.Sections)*16 + 1)
	chunk.HeightMaps.WorldSurface = level.NewBitStorage(bitsForHeight, 16*16, nil)
	chunk.HeightMaps.MotionBlocking = level.NewBitStorage(bitsForHeight, 16*16, nil)
	chunk.HeightMaps.MotionBlockingNoLeaves = level.NewBitStorage(bitsForHeight, 16*16, nil)

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			hmIdx := z*16 + x
			wsFound, mbFound, mbnlFound := false, false, false
			for secIdx := g.Sections - 1; secIdx >= 0; secIdx-- {
				for localY := 15; localY >= 0; localY-- {
					idx := localY*16*16 + z*16 + x
					state := chunk.Sections[secIdx].GetBlock(idx)
					if block.IsAir(state) {
						continue
					}
					absY := secIdx*16 + localY + 1
					if !wsFound {
						chunk.HeightMaps.WorldSurface.Set(hmIdx, absY)
						wsFound = true
					}
					if !mbFound {
						chunk.HeightMaps.MotionBlocking.Set(hmIdx, absY)
						mbFound = true
					}
					if !mbnlFound {
						chunk.HeightMaps.MotionBlockingNoLeaves.Set(hmIdx, absY)
						mbnlFound = true
					}
					if wsFound && mbFound && mbnlFound {
						goto nextCol
					}
				}
			}
		nextCol:
		}
	}
}
