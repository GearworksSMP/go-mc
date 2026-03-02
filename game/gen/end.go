package gen

import (
	"math"
	"math/bits"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// EndGenerator creates End dimension terrain with a central end stone island,
// obsidian pillars, and a void world everywhere else.
type EndGenerator struct {
	Seed     int64
	MinY     int // -64 (same as overworld)
	Sections int // 24

	shapeNoise *SimplexNoise

	// Block state IDs
	endStoneID  level.BlocksState
	obsidianID  level.BlocksState
	bedrockID   level.BlocksState
	torchID     level.BlocksState
	hasTorch    bool
}

// NewEndGenerator creates an End terrain generator with the given seed.
func NewEndGenerator(seed int64) *EndGenerator {
	g := &EndGenerator{
		Seed:     seed,
		MinY:     -64,
		Sections: 24,

		shapeNoise: NewSimplexNoise(seed + 500),
	}

	g.endStoneID, _ = block.ToStateID[block.EndStone{}]
	g.obsidianID, _ = block.ToStateID[block.Obsidian{}]
	g.bedrockID, _ = block.ToStateID[block.Bedrock{}]

	var ok bool
	g.torchID, ok = block.ToStateID[block.Torch{}]
	g.hasTorch = ok

	return g
}

// SpawnY returns the Y coordinate for spawning in the End.
func (g *EndGenerator) SpawnY() float64 {
	return 100
}

// pillar describes an obsidian pillar on the End island.
type pillar struct {
	cx, cz int     // center position
	radius int     // pillar radius
	height int     // pillar height above base Y=64
}

// endPillars returns the 10 obsidian pillars placed around the origin.
// The pillars are deterministically generated from the seed.
func (g *EndGenerator) endPillars() []pillar {
	pillars := make([]pillar, 10)
	for i := 0; i < 10; i++ {
		angle := float64(i) * 2 * math.Pi / 10
		dist := 40.0 + float64(posHash(i, 0, 0, g.Seed+600)%41) // 40-80 from origin
		pillars[i] = pillar{
			cx:     int(math.Round(math.Cos(angle) * dist)),
			cz:     int(math.Round(math.Sin(angle) * dist)),
			radius: 3,
			height: 40 + int(posHash(i, 1, 0, g.Seed+601)%61), // 40-100
		}
	}
	return pillars
}

// Generate creates an End chunk at the given position.
func (g *EndGenerator) Generate(pos game.ChunkPos) *level.Chunk {
	chunk := level.EmptyChunk(g.Sections)

	// The End island is centered around (0, 0) with radius ~90 blocks.
	// Base Y = 64, which maps to section index (64 - MinY) / 16.
	baseY := 64

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z

			distFromOrigin := math.Sqrt(float64(worldX*worldX + worldZ*worldZ))

			// End island: radius ~90 blocks with noise-based edge variation
			if distFromOrigin < 95 {
				// Compute island surface height with noise
				edgeFactor := 1.0
				if distFromOrigin > 70 {
					edgeFactor = math.Max(0, (95-distFromOrigin)/25)
				}

				noise := g.shapeNoise.Noise2D(float64(worldX)*0.03, float64(worldZ)*0.03)
				surfaceHeight := baseY + int(noise*4*edgeFactor)

				// Fill with end stone from a few blocks below base up to surface
				bottomY := baseY - 5 - int(3*edgeFactor)
				if bottomY < baseY-8 {
					bottomY = baseY - 8
				}

				for worldY := bottomY; worldY <= surfaceHeight; worldY++ {
					g.setBlock(chunk, x, worldY, z, g.endStoneID)
				}
			}
		}
	}

	// Place obsidian pillars
	g.placePillars(chunk, pos)

	// Place return portal base at origin (bedrock + end portal frame below)
	g.placeReturnPortalBase(chunk, pos)

	g.computeHeightmaps(chunk)

	return chunk
}

// placePillars places obsidian pillars that intersect this chunk.
func (g *EndGenerator) placePillars(chunk *level.Chunk, pos game.ChunkPos) {
	baseY := 64
	pillars := g.endPillars()

	chunkMinX := pos.X * 16
	chunkMaxX := chunkMinX + 15
	chunkMinZ := pos.Z * 16
	chunkMaxZ := chunkMinZ + 15

	for _, p := range pillars {
		// Check if this pillar intersects this chunk
		if p.cx-p.radius > chunkMaxX || p.cx+p.radius < chunkMinX {
			continue
		}
		if p.cz-p.radius > chunkMaxZ || p.cz+p.radius < chunkMinZ {
			continue
		}

		// Place pillar blocks within this chunk
		for dz := -p.radius; dz <= p.radius; dz++ {
			for dx := -p.radius; dx <= p.radius; dx++ {
				// Circular cross-section
				if dx*dx+dz*dz > p.radius*p.radius {
					continue
				}
				wx := p.cx + dx
				wz := p.cz + dz
				if wx < chunkMinX || wx > chunkMaxX || wz < chunkMinZ || wz > chunkMaxZ {
					continue
				}
				lx := wx - chunkMinX
				lz := wz - chunkMinZ

				for worldY := baseY; worldY <= baseY+p.height; worldY++ {
					g.setBlock(chunk, lx, worldY, lz, g.obsidianID)
				}
			}
		}

		// Place torch on top center of pillar if it falls in this chunk
		if p.cx >= chunkMinX && p.cx <= chunkMaxX && p.cz >= chunkMinZ && p.cz <= chunkMaxZ {
			if g.hasTorch {
				lx := p.cx - chunkMinX
				lz := p.cz - chunkMinZ
				g.setBlock(chunk, lx, baseY+p.height+1, lz, g.torchID)
			}
		}
	}
}

// placeReturnPortalBase places a bedrock pedestal at origin (0, 64, 0).
// The full return portal is placed when the dragon is killed.
func (g *EndGenerator) placeReturnPortalBase(chunk *level.Chunk, pos game.ChunkPos) {
	chunkMinX := pos.X * 16
	chunkMinZ := pos.Z * 16

	// Bedrock 3x3 platform at 0,64,0
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			wx := dx
			wz := dz
			lx := wx - chunkMinX
			lz := wz - chunkMinZ
			if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
				continue
			}
			g.setBlock(chunk, lx, 64, lz, g.bedrockID)
		}
	}

	// Center bedrock pillar up to Y=68
	if 0 >= chunkMinX && 0 <= chunkMinX+15 && 0 >= chunkMinZ && 0 <= chunkMinZ+15 {
		lx := 0 - chunkMinX
		lz := 0 - chunkMinZ
		for y := 64; y <= 68; y++ {
			g.setBlock(chunk, lx, y, lz, g.bedrockID)
		}
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (g *EndGenerator) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState) {
	secIdx := (worldY - g.MinY) / 16
	if secIdx < 0 || secIdx >= g.Sections {
		return
	}
	localY := (worldY - g.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}

// computeHeightmaps calculates heightmaps for the End chunk.
func (g *EndGenerator) computeHeightmaps(chunk *level.Chunk) {
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
