package gen

import (
	"math/bits"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// TerrainGenerator creates terrain with simplex noise, caves, ores, and trees.
type TerrainGenerator struct {
	Seed     int64
	SeaLevel int
	MinY     int
	Sections int

	heightNoise *SimplexNoise
	caveNoise   *SimplexNoise
	oreNoise    *SimplexNoise

	bedrockID, stoneID, dirtID, grassID, airID level.BlocksState
	coalOreID, ironOreID, copperOreID           level.BlocksState
	goldOreID, diamondOreID, lapisOreID         level.BlocksState
	redstoneOreID                                level.BlocksState
	oakLogID, oakLeavesID                        level.BlocksState
	waterID                                      level.BlocksState
	sandID, sandstoneID                          level.BlocksState
	gravelID                                     level.BlocksState
	birchLogID, birchLeavesID                    level.BlocksState
	spruceLogID, spruceLeavesID                  level.BlocksState
	cactusID                                     level.BlocksState
	biomeNoise                                   *SimplexNoise
	structurePlacer                              *StructurePlacer
}

// NewTerrainGenerator creates a terrain generator with the given seed.
func NewTerrainGenerator(seed int64) *TerrainGenerator {
	g := &TerrainGenerator{
		Seed:     seed,
		SeaLevel: 64,
		MinY:     -64,
		Sections: 24,

		heightNoise: NewSimplexNoise(seed),
		caveNoise:   NewSimplexNoise(seed + 1),
		oreNoise:    NewSimplexNoise(seed + 2),
	}

	g.bedrockID, _ = block.ToStateID[block.Bedrock{}]
	g.stoneID, _ = block.ToStateID[block.Stone{}]
	g.dirtID, _ = block.ToStateID[block.Dirt{}]
	g.grassID, _ = block.ToStateID[block.GrassBlock{Snowy: false}]
	g.oakLogID, _ = block.ToStateID[block.OakLog{Axis: block.Y}]
	g.oakLeavesID, _ = block.ToStateID[block.OakLeaves{Distance: 1, Persistent: true, Waterlogged: false}]

	g.coalOreID, _ = block.ToStateID[block.CoalOre{}]
	g.ironOreID, _ = block.ToStateID[block.IronOre{}]
	g.copperOreID, _ = block.ToStateID[block.CopperOre{}]
	g.goldOreID, _ = block.ToStateID[block.GoldOre{}]
	g.diamondOreID, _ = block.ToStateID[block.DiamondOre{}]
	g.lapisOreID, _ = block.ToStateID[block.LapisOre{}]
	g.redstoneOreID, _ = block.ToStateID[block.RedstoneOre{Lit: false}]
	g.waterID, _ = block.ToStateID[block.Water{Level: 0}]

	g.sandID, _ = block.ToStateID[block.Sand{}]
	g.sandstoneID, _ = block.ToStateID[block.Sandstone{}]
	g.gravelID, _ = block.ToStateID[block.Gravel{}]
	g.birchLogID, _ = block.ToStateID[block.BirchLog{Axis: block.Y}]
	g.birchLeavesID, _ = block.ToStateID[block.BirchLeaves{Distance: 1, Persistent: true, Waterlogged: false}]
	g.spruceLogID, _ = block.ToStateID[block.SpruceLog{Axis: block.Y}]
	g.spruceLeavesID, _ = block.ToStateID[block.SpruceLeaves{Distance: 1, Persistent: true, Waterlogged: false}]
	g.cactusID, _ = block.ToStateID[block.Cactus{Age: 0}]
	g.biomeNoise = NewSimplexNoise(seed + 3)
	g.structurePlacer = NewStructurePlacer(seed, g.waterID)

	return g
}

// BiomeType represents a terrain biome.
type BiomeType int

const (
	BiomePlains    BiomeType = 0
	BiomeForest    BiomeType = 1
	BiomeDesert    BiomeType = 2
	BiomeMountains BiomeType = 3
	BiomeOcean     BiomeType = 4
)

// biomeAt returns the biome type for a world position using large-scale noise.
func (g *TerrainGenerator) biomeAt(x, z int) BiomeType {
	val := g.biomeNoise.Noise2D(float64(x)*0.002, float64(z)*0.002)
	switch {
	case val < -0.3:
		return BiomeOcean
	case val < 0.1:
		return BiomePlains
	case val < 0.35:
		return BiomeForest
	case val < 0.6:
		return BiomeDesert
	default:
		return BiomeMountains
	}
}

// Generate creates a terrain chunk at the given position.
func (g *TerrainGenerator) Generate(pos game.ChunkPos) *level.Chunk {
	chunk := level.EmptyChunk(g.Sections)

	heights, biomes := g.computeHeights(pos)

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z
			h := heights[z*16+x]
			biome := biomes[z*16+x]

			for worldY := g.MinY; worldY < g.MinY+g.Sections*16; worldY++ {
				var stateID level.BlocksState
				switch {
				case worldY == g.MinY:
					stateID = g.bedrockID
				case worldY < h-3:
					stateID = g.stoneID
				case worldY <= h:
					stateID = g.surfaceBlock(biome, h-worldY, worldY)
				case worldY <= g.SeaLevel && worldY > h:
					stateID = g.waterID
				default:
					continue // air
				}

				// Cave carving (only in stone, not in desert sand or surface)
				if stateID == g.stoneID && worldY > g.MinY+1 && worldY < h-2 {
					cv := g.caveNoise.Noise3D(float64(worldX)/16.0, float64(worldY)/16.0, float64(worldZ)/16.0)
					if cv > 0.6 {
						continue
					}
				}

				// Ore placement (only replace stone)
				if stateID == g.stoneID {
					if ore := g.getOre(worldX, worldY, worldZ); ore != 0 {
						stateID = ore
					}
				}

				g.setBlockInChunk(chunk, x, worldY, z)
				secIdx := (worldY - g.MinY) / 16
				localY := (worldY - g.MinY) % 16
				idx := localY*16*16 + z*16 + x
				chunk.Sections[secIdx].SetBlock(idx, stateID)
			}
		}
	}

	g.placeBiomeTrees(chunk, pos, heights, biomes)
	g.structurePlacer.PlaceStructures(chunk, pos.X, pos.Z, g)
	g.computeHeightmaps(chunk)

	return chunk
}

// computeHeights returns a 16x16 array of terrain heights and biome types for a chunk.
func (g *TerrainGenerator) computeHeights(pos game.ChunkPos) ([256]int, [256]BiomeType) {
	var heights [256]int
	var biomes [256]BiomeType
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z
			fWorldX := float64(worldX)
			fWorldZ := float64(worldZ)

			biome := g.biomeAt(worldX, worldZ)
			biomes[z*16+x] = biome

			// Base terrain noise
			n := g.heightNoise.Octave2D(fWorldX/64.0, fWorldZ/64.0, 3, 2.0, 0.5)

			var h int
			switch biome {
			case BiomePlains:
				h = 64 + int(n*8)
			case BiomeForest:
				h = 66 + int(n*10)
			case BiomeDesert:
				h = 62 + int(n*6)
			case BiomeMountains:
				h = 72 + int(n*32)
			case BiomeOcean:
				h = 45 + int(n*8)
			default:
				h = 64 + int(n*16)
			}

			if h < g.MinY+5 {
				h = g.MinY + 5
			}
			if h > g.MinY+g.Sections*16-2 {
				h = g.MinY + g.Sections*16 - 2
			}
			heights[z*16+x] = h
		}
	}
	return heights, biomes
}

// setBlockInChunk is a no-op placeholder to ensure the section exists.
func (g *TerrainGenerator) setBlockInChunk(_ *level.Chunk, _, _, _ int) {}

// surfaceBlock returns the block for a surface layer at a given depth and biome.
func (g *TerrainGenerator) surfaceBlock(biome BiomeType, depthFromSurface, worldY int) level.BlocksState {
	switch biome {
	case BiomeDesert:
		if depthFromSurface <= 3 {
			return g.sandID
		}
		return g.sandstoneID
	case BiomeOcean:
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.gravelID
			}
			return g.grassID
		}
		return g.dirtID
	case BiomeMountains:
		if worldY > 90 {
			return g.stoneID // exposed stone at high altitude
		}
		if depthFromSurface == 0 {
			return g.grassID
		}
		return g.dirtID
	default: // Plains, Forest
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.dirtID
			}
			return g.grassID
		}
		return g.dirtID
	}
}

// getOre returns an ore block state ID for the given position, or 0 if no ore.
func (g *TerrainGenerator) getOre(x, y, z int) level.BlocksState {
	// Use deterministic hash based on position
	h := posHash(x, y, z, g.Seed)

	worldY := y

	// Check each ore type
	type oreEntry struct {
		id      level.BlocksState
		minY    int
		maxY    int
		chance  uint64 // 1 in N chance
	}

	ores := [...]oreEntry{
		{g.coalOreID, 0, 128, 50},
		{g.ironOreID, -16, 72, 80},
		{g.copperOreID, -16, 112, 70},
		{g.goldOreID, -64, 32, 200},
		{g.diamondOreID, -64, 16, 500},
		{g.lapisOreID, -32, 32, 300},
		{g.redstoneOreID, -64, 16, 200},
	}

	for _, ore := range ores {
		if worldY >= ore.minY && worldY <= ore.maxY {
			if h%ore.chance == 0 {
				return ore.id
			}
		}
		// Rotate hash bits to get different values for each ore type
		h = (h >> 7) | (h << 57)
	}

	return 0
}

// posHash returns a deterministic hash for a world position.
func posHash(x, y, z int, seed int64) uint64 {
	h := uint64(seed)
	h ^= uint64(x) * 0x9E3779B97F4A7C15
	h ^= uint64(y) * 0x517CC1B727220A95
	h ^= uint64(z) * 0x6C62272E07BB0142
	h = (h ^ (h >> 30)) * 0xBF58476D1CE4E5B9
	h = (h ^ (h >> 27)) * 0x94D049BB133111EB
	h = h ^ (h >> 31)
	return h
}

// placeBiomeTrees scatters biome-appropriate trees on suitable surfaces within the chunk.
func (g *TerrainGenerator) placeBiomeTrees(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	rng := posHash(pos.X, 0, pos.Z, g.Seed+42)

	for z := 2; z < 14; z++ {
		for x := 2; x < 14; x++ {
			biome := biomes[z*16+x]
			ty := heights[z*16+x]
			if ty < g.SeaLevel {
				continue
			}

			// Check surface is appropriate
			secIdx := (ty - g.MinY) / 16
			if secIdx < 0 || secIdx >= g.Sections {
				continue
			}
			localY := (ty - g.MinY) % 16
			idx := localY*16*16 + z*16 + x
			surfState := chunk.Sections[secIdx].GetBlock(idx)

			// Determine tree chance and type based on biome
			treeRng := posHash(pos.X*16+x, ty, pos.Z*16+z, g.Seed+99)

			switch biome {
			case BiomePlains:
				if surfState != g.grassID || treeRng%40 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.oakLogID, g.oakLeavesID, &rng)
			case BiomeForest:
				if surfState != g.grassID || treeRng%8 != 0 {
					continue
				}
				if treeRng%3 == 0 {
					g.placeTree(chunk, x, ty, z, g.birchLogID, g.birchLeavesID, &rng)
				} else {
					g.placeTree(chunk, x, ty, z, g.oakLogID, g.oakLeavesID, &rng)
				}
			case BiomeDesert:
				// Cacti on sand (rare)
				if surfState != g.sandID || treeRng%30 != 0 || g.cactusID == 0 {
					continue
				}
				// Place 1-3 block tall cactus
				cactusH := 1 + int(treeRng/30)%3
				for dy := 1; dy <= cactusH; dy++ {
					g.setBlock(chunk, x, ty+dy, z, g.cactusID)
				}
			case BiomeMountains:
				if surfState != g.grassID || ty > 90 || treeRng%20 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.spruceLogID, g.spruceLeavesID, &rng)
			case BiomeOcean:
				continue // no trees in ocean
			}
		}
	}
}

// placeTree places a single tree at the given position.
func (g *TerrainGenerator) placeTree(chunk *level.Chunk, x, ty, z int, logID, leavesID level.BlocksState, rng *uint64) {
	trunkH := 4 + int(*rng%3)
	*rng = (*rng >> 3) | (*rng << 61)

	for dy := 1; dy <= trunkH; dy++ {
		g.setBlock(chunk, x, ty+dy, z, logID)
	}

	topY := ty + trunkH
	for dy := -1; dy <= 1; dy++ {
		radius := 1
		if dy == 1 {
			radius = 0
		}
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				lx, ly, lz := x+dx, topY+dy, z+dz
				if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
					continue
				}
				if dx == 0 && dz == 0 && dy <= 0 {
					continue
				}
				g.setBlock(chunk, lx, ly, lz, leavesID)
			}
		}
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (g *TerrainGenerator) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState) {
	secIdx := (worldY - g.MinY) / 16
	if secIdx < 0 || secIdx >= g.Sections {
		return
	}
	localY := (worldY - g.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}

// computeHeightmaps calculates all heightmaps for the chunk.
func (g *TerrainGenerator) computeHeightmaps(chunk *level.Chunk) {
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
					if !mbFound && isMotionBlocking(state) {
						chunk.HeightMaps.MotionBlocking.Set(hmIdx, absY)
						mbFound = true
					}
					if !mbnlFound && isMotionBlockingNoLeaves(state) {
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

// isMotionBlocking returns true if the block blocks entity motion.
func isMotionBlocking(state level.BlocksState) bool {
	if block.IsAir(state) {
		return false
	}
	// Leaves are motion blocking
	return true
}

// isMotionBlockingNoLeaves returns true if the block blocks motion and isn't leaves.
func isMotionBlockingNoLeaves(state level.BlocksState) bool {
	if block.IsAir(state) {
		return false
	}
	if int(state) >= 0 && int(state) < len(block.StateList) && block.StateList[state] != nil {
		name := block.StateList[state].ID()
		// Check for leaves
		for _, suffix := range []string{"_leaves"} {
			if len(name) > len(suffix)+10 && name[len(name)-len(suffix):] == suffix {
				return false
			}
		}
	}
	return true
}

// TopBlockY returns the approximate spawn height for the terrain.
func (g *TerrainGenerator) TopBlockY() int {
	return g.SeaLevel // approximate
}

// SpawnY returns the Y coordinate for spawning.
func (g *TerrainGenerator) SpawnY() float64 {
	// Use noise to compute actual height at spawn
	n := g.heightNoise.Octave2D(0, 0, 3, 2.0, 0.5)
	h := 64 + int(n*16)
	return float64(h) + 1
}
