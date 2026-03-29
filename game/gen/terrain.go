package gen

import (
	"math"
	"math/bits"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
)

// TerrainGenerator creates terrain with simplex noise, caves, ores, and trees.
type TerrainGenerator struct {
	Seed     int64
	SeaLevel int
	MinY     int
	Sections int

	heightNoise  *SimplexNoise
	caveNoise    *SimplexNoise
	caveNoise2   *SimplexNoise
	ravineNoise  *SimplexNoise
	oreNoise     *SimplexNoise

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
	acaciaLogID, acaciaLeavesID                  level.BlocksState
	cactusID                                     level.BlocksState
	sugarCaneID                                  level.BlocksState
	pumpkinID                                    level.BlocksState
	snowLayerID                                  level.BlocksState
	iceID                                        level.BlocksState
	snowyGrassID                                 level.BlocksState
	deepslateID                                  level.BlocksState
	dsCoalOreID, dsIronOreID, dsCopperOreID      level.BlocksState
	dsGoldOreID, dsDiamondOreID, dsLapisOreID    level.BlocksState
	dsRedstoneOreID                              level.BlocksState
	// New biome blocks
	vineID                                       level.BlocksState
	lilyPadID                                    level.BlocksState
	myceliumID                                   level.BlocksState
	orangeTerracottaID                           level.BlocksState
	redTerracottaID                              level.BlocksState
	yellowTerracottaID                           level.BlocksState
	brownTerracottaID                            level.BlocksState
	redSandID                                    level.BlocksState
	darkOakLogID                                 level.BlocksState
	darkOakLeavesID                              level.BlocksState
	jungleLogID                                  level.BlocksState
	jungleLeavesID                               level.BlocksState
	redMushroomBlockID                           level.BlocksState
	brownMushroomBlockID                         level.BlocksState
	mushroomStemID                               level.BlocksState
	brownMushroomID                              level.BlocksState
	redMushroomID                                level.BlocksState
	deadBushID                                   level.BlocksState
	poppyID                                      level.BlocksState
	dandelionID                                  level.BlocksState
	azureBluetID                                 level.BlocksState
	oxeyeDaisyID                                 level.BlocksState
	clayID                                       level.BlocksState

	biomeNoise                                   *SimplexNoise
	humidNoise                                   *SimplexNoise
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
		caveNoise2:  NewSimplexNoise(seed + 5),
		ravineNoise: NewSimplexNoise(seed + 6),
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
	g.acaciaLogID, _ = block.ToStateID[block.AcaciaLog{Axis: block.Y}]
	g.acaciaLeavesID, _ = block.ToStateID[block.AcaciaLeaves{Distance: 1, Persistent: true, Waterlogged: false}]
	g.cactusID, _ = block.ToStateID[block.Cactus{Age: 0}]
	g.sugarCaneID, _ = block.ToStateID[block.SugarCane{Age: 0}]
	g.pumpkinID, _ = block.ToStateID[block.Pumpkin{}]
	g.snowLayerID, _ = block.ToStateID[block.Snow{Layers: 1}]
	g.iceID, _ = block.ToStateID[block.Ice{}]
	g.snowyGrassID, _ = block.ToStateID[block.GrassBlock{Snowy: true}]
	g.deepslateID, _ = block.ToStateID[block.Deepslate{Axis: block.Y}]
	g.dsCoalOreID, _ = block.ToStateID[block.DeepslateCoalOre{}]
	g.dsIronOreID, _ = block.ToStateID[block.DeepslateIronOre{}]
	g.dsCopperOreID, _ = block.ToStateID[block.DeepslateCopperOre{}]
	g.dsGoldOreID, _ = block.ToStateID[block.DeepslateGoldOre{}]
	g.dsDiamondOreID, _ = block.ToStateID[block.DeepslateDiamondOre{}]
	g.dsLapisOreID, _ = block.ToStateID[block.DeepslateLapisOre{}]
	g.dsRedstoneOreID, _ = block.ToStateID[block.DeepslateRedstoneOre{Lit: false}]
	// New biome blocks
	g.vineID, _ = block.ToStateID[block.Vine{South: true}]
	g.lilyPadID, _ = block.ToStateID[block.LilyPad{}]
	g.myceliumID, _ = block.ToStateID[block.Mycelium{Snowy: false}]
	g.orangeTerracottaID, _ = block.ToStateID[block.OrangeTerracotta{}]
	g.redTerracottaID, _ = block.ToStateID[block.RedTerracotta{}]
	g.yellowTerracottaID, _ = block.ToStateID[block.YellowTerracotta{}]
	g.brownTerracottaID, _ = block.ToStateID[block.BrownTerracotta{}]
	g.redSandID, _ = block.ToStateID[block.RedSand{}]
	g.darkOakLogID, _ = block.ToStateID[block.DarkOakLog{Axis: block.Y}]
	g.darkOakLeavesID, _ = block.ToStateID[block.DarkOakLeaves{Distance: 1, Persistent: true, Waterlogged: false}]
	g.jungleLogID, _ = block.ToStateID[block.JungleLog{Axis: block.Y}]
	g.jungleLeavesID, _ = block.ToStateID[block.JungleLeaves{Distance: 1, Persistent: true, Waterlogged: false}]
	g.redMushroomBlockID, _ = block.ToStateID[block.RedMushroomBlock{Down: true, East: true, North: true, South: true, Up: true, West: true}]
	g.brownMushroomBlockID, _ = block.ToStateID[block.BrownMushroomBlock{Down: true, East: true, North: true, South: true, Up: true, West: true}]
	g.mushroomStemID, _ = block.ToStateID[block.MushroomStem{Down: true, East: true, North: true, South: true, Up: true, West: true}]
	g.brownMushroomID, _ = block.ToStateID[block.BrownMushroom{}]
	g.redMushroomID, _ = block.ToStateID[block.RedMushroom{}]
	g.deadBushID, _ = block.ToStateID[block.DeadBush{}]
	g.poppyID, _ = block.ToStateID[block.Poppy{}]
	g.dandelionID, _ = block.ToStateID[block.Dandelion{}]
	g.azureBluetID, _ = block.ToStateID[block.AzureBluet{}]
	g.oxeyeDaisyID, _ = block.ToStateID[block.OxeyeDaisy{}]
	g.clayID, _ = block.ToStateID[block.Clay{}]

	g.biomeNoise = NewSimplexNoise(seed + 3)
	g.humidNoise = NewSimplexNoise(seed + 4)
	g.structurePlacer = NewStructurePlacer(seed, g.waterID)

	return g
}

// BiomeType represents a terrain biome.
type BiomeType int

const (
	BiomePlains      BiomeType = 0
	BiomeForest      BiomeType = 1
	BiomeDesert      BiomeType = 2
	BiomeMountains   BiomeType = 3
	BiomeOcean       BiomeType = 4
	BiomeTaiga       BiomeType = 5
	BiomeSnowyPlains BiomeType = 6
	BiomeSnowyTaiga  BiomeType = 7
	BiomeBirchForest BiomeType = 8
	BiomeSavanna     BiomeType = 9
	BiomeJungle      BiomeType = 10
	BiomeSwamp       BiomeType = 11
	BiomeDarkForest  BiomeType = 12
	BiomeFlowerForest BiomeType = 13
	BiomeMushroom    BiomeType = 14
	BiomeBadlands    BiomeType = 15
)

// biomeRegistryID maps internal BiomeType to the biome registry ID used on the wire.
func biomeRegistryID(b BiomeType) biome.Type {
	switch b {
	case BiomePlains:
		return 1 // minecraft:plains
	case BiomeForest:
		return 8 // minecraft:forest
	case BiomeDesert:
		return 5 // minecraft:desert
	case BiomeMountains:
		return 19 // minecraft:windswept_hills
	case BiomeOcean:
		return 43 // minecraft:ocean
	case BiomeTaiga:
		return 15 // minecraft:taiga
	case BiomeSnowyPlains:
		return 3 // minecraft:snowy_plains
	case BiomeSnowyTaiga:
		return 16 // minecraft:snowy_taiga
	case BiomeBirchForest:
		return 10 // minecraft:birch_forest
	case BiomeSavanna:
		return 17 // minecraft:savanna
	case BiomeJungle:
		return 22 // minecraft:jungle
	case BiomeSwamp:
		return 6 // minecraft:swamp
	case BiomeDarkForest:
		return 11 // minecraft:dark_forest
	case BiomeFlowerForest:
		return 9 // minecraft:flower_forest
	case BiomeMushroom:
		return 48 // minecraft:mushroom_fields
	case BiomeBadlands:
		return 25 // minecraft:badlands
	default:
		return 1
	}
}

// biomeAt returns the biome type for a world position using temperature + humidity noise.
func (g *TerrainGenerator) biomeAt(x, z int) BiomeType {
	temp := g.biomeNoise.Noise2D(float64(x)*0.002, float64(z)*0.002)
	humid := g.humidNoise.Noise2D(float64(x)*0.002, float64(z)*0.002)

	// Mushroom island: rare special biome when both noise values are extreme
	if temp > 0.6 && humid > 0.6 {
		return BiomeMushroom
	}

	switch {
	case temp < -0.3:
		// Cold biomes
		if humid > 0 {
			return BiomeSnowyPlains
		}
		return BiomeSnowyTaiga
	case temp < 0:
		// Cool biomes
		if humid > 0.4 {
			return BiomeDarkForest
		}
		return BiomeTaiga
	case temp < 0.25:
		// Temperate biomes
		if humid > 0.3 {
			return BiomeForest
		}
		if humid > 0.1 {
			return BiomeFlowerForest
		}
		if humid < -0.1 {
			return BiomeBirchForest
		}
		return BiomePlains
	case temp < 0.5:
		// Warm biomes
		if humid > 0.4 {
			return BiomeSwamp
		}
		if humid > 0.2 {
			return BiomeForest
		}
		if humid < -0.2 {
			return BiomeSavanna
		}
		return BiomePlains
	default:
		// Hot biomes
		if humid > 0.3 {
			return BiomeJungle
		}
		if humid > 0.1 {
			return BiomeSavanna
		}
		if humid < -0.2 {
			return BiomeBadlands
		}
		return BiomeDesert
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
					if g.isCave(worldX, worldY, worldZ) {
						continue
					}
				}

				// Deepslate below Y=0
				if stateID == g.stoneID && worldY < 0 {
					stateID = g.deepslateID
				}

				// Ore placement (replace stone or deepslate)
				if stateID == g.stoneID || stateID == g.deepslateID {
					if ore := g.getOre(worldX, worldY, worldZ); ore != 0 {
						if stateID == g.deepslateID {
							ore = g.deepslateOre(ore)
						}
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

	// Write biome data to each section (4x4x4 grid = 64 entries per section)
	for secIdx := 0; secIdx < g.Sections; secIdx++ {
		for by := 0; by < 4; by++ {
			for bz := 0; bz < 4; bz++ {
				for bx := 0; bx < 4; bx++ {
					wx := pos.X*16 + bx*4 + 2 // sample center of 4-block region
					wz := pos.Z*16 + bz*4 + 2
					b := g.biomeAt(wx, wz)
					idx := by*16 + bz*4 + bx
					chunk.Sections[secIdx].Biomes.Set(idx, biomeRegistryID(b))
				}
			}
		}
	}

	g.placeSnowAndIce(chunk, pos, heights, biomes)
	g.placeBiomeTrees(chunk, pos, heights, biomes)
	g.placeBiomeVegetation(chunk, pos, heights, biomes)
	g.placeSugarCane(chunk, pos, heights, biomes)
	g.placePumpkins(chunk, pos, heights, biomes)
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
			case BiomeForest, BiomeBirchForest:
				h = 66 + int(n*10)
			case BiomeDesert:
				h = 62 + int(n*6)
			case BiomeMountains:
				h = 72 + int(n*32)
			case BiomeOcean:
				h = 45 + int(n*8)
			case BiomeTaiga, BiomeSnowyTaiga:
				h = 66 + int(n*12)
			case BiomeSnowyPlains:
				h = 64 + int(n*6)
			case BiomeSavanna:
				h = 64 + int(n*10)
			case BiomeJungle:
				h = 66 + int(n*10)
			case BiomeSwamp:
				h = 62 + int(n*4) // Flat and low, near water level
			case BiomeDarkForest:
				h = 66 + int(n*8)
			case BiomeFlowerForest:
				h = 66 + int(n*8)
			case BiomeMushroom:
				h = 64 + int(n*6)
			case BiomeBadlands:
				h = 68 + int(n*16) // Hilly terracotta terrain
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
			return g.stoneID
		}
		if depthFromSurface == 0 {
			return g.grassID
		}
		return g.dirtID
	case BiomeSnowyPlains, BiomeSnowyTaiga:
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.dirtID
			}
			return g.snowyGrassID
		}
		return g.dirtID
	case BiomeSavanna:
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.dirtID
			}
			return g.grassID
		}
		return g.dirtID
	case BiomeBadlands:
		if depthFromSurface == 0 {
			return g.redSandID
		}
		// Terracotta layers at different depths
		switch depthFromSurface % 4 {
		case 1:
			return g.orangeTerracottaID
		case 2:
			return g.redTerracottaID
		case 3:
			return g.yellowTerracottaID
		default:
			return g.brownTerracottaID
		}
	case BiomeMushroom:
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.dirtID
			}
			return g.myceliumID
		}
		return g.dirtID
	case BiomeSwamp:
		if depthFromSurface == 0 {
			if worldY < g.SeaLevel {
				return g.dirtID
			}
			// Clay patches near water level (hash uses worldY as proxy for position variation)
			if worldY <= g.SeaLevel+2 {
				h := posHash(worldY, depthFromSurface, worldY*37, g.Seed)
				if h%4 == 0 {
					return g.clayID
				}
			}
			return g.grassID
		}
		return g.dirtID
	default: // Plains, Forest, Birch Forest, Taiga, Jungle, Dark Forest, Flower Forest
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

// isCave returns true if the given position should be carved out as a cave.
// Uses spaghetti caves (dual-offset noise), cheese caves (large caverns below Y=30),
// and ravines (2D ridge noise for surface-to-deep cuts).
func (g *TerrainGenerator) isCave(x, y, z int) bool {
	fx, fy, fz := float64(x), float64(y), float64(z)

	// Spaghetti caves: two offset noise fields creating narrow winding tunnels
	sa := g.caveNoise.Noise3D(fx*0.03, fy*0.03, fz*0.03)
	sb := g.caveNoise2.Noise3D(fx*0.03, fy*0.03, fz*0.03)
	if sa*sa+sb*sb < 0.02 {
		return true
	}

	// Cheese caves: large open caverns below Y=30
	if y < 30 {
		cheese := g.caveNoise.Noise3D(fx*0.015, fy*0.02, fz*0.015)
		if cheese > 0.55 {
			return true
		}
	}

	// Ravines: 2D ridge noise creates surface-to-deep vertical cuts
	ridge := 1.0 - math.Abs(g.ravineNoise.Noise2D(fx*0.005, fz*0.005))
	if ridge > 0.95 && y < 50 {
		// Width decreases with depth
		widthNoise := g.ravineNoise.Noise2D(fx*0.02, fz*0.02)
		if math.Abs(widthNoise) < 0.15 {
			return true
		}
	}

	return false
}

// deepslateOre converts a regular ore state ID to its deepslate variant.
func (g *TerrainGenerator) deepslateOre(ore level.BlocksState) level.BlocksState {
	switch ore {
	case g.coalOreID:
		return g.dsCoalOreID
	case g.ironOreID:
		return g.dsIronOreID
	case g.copperOreID:
		return g.dsCopperOreID
	case g.goldOreID:
		return g.dsGoldOreID
	case g.diamondOreID:
		return g.dsDiamondOreID
	case g.lapisOreID:
		return g.dsLapisOreID
	case g.redstoneOreID:
		return g.dsRedstoneOreID
	default:
		return ore
	}
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
			case BiomeBirchForest:
				if surfState != g.grassID || treeRng%8 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.birchLogID, g.birchLeavesID, &rng)
			case BiomeDesert:
				if surfState != g.sandID || treeRng%30 != 0 || g.cactusID == 0 {
					continue
				}
				cactusH := 1 + int(treeRng/30)%3
				for dy := 1; dy <= cactusH; dy++ {
					g.setBlock(chunk, x, ty+dy, z, g.cactusID)
				}
			case BiomeMountains:
				if surfState != g.grassID || ty > 90 || treeRng%20 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.spruceLogID, g.spruceLeavesID, &rng)
			case BiomeTaiga, BiomeSnowyTaiga:
				isSnowyGrass := surfState == g.snowyGrassID
				if (surfState != g.grassID && !isSnowyGrass) || treeRng%10 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.spruceLogID, g.spruceLeavesID, &rng)
			case BiomeSnowyPlains:
				if surfState != g.snowyGrassID || treeRng%60 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.spruceLogID, g.spruceLeavesID, &rng)
			case BiomeSavanna:
				if surfState != g.grassID || treeRng%15 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.acaciaLogID, g.acaciaLeavesID, &rng)
			case BiomeJungle:
				if surfState != g.grassID || treeRng%6 != 0 {
					continue
				}
				g.placeJungleTree(chunk, x, ty, z, &rng)
			case BiomeSwamp:
				if surfState != g.grassID || treeRng%12 != 0 {
					continue
				}
				g.placeSwampTree(chunk, x, ty, z, &rng)
			case BiomeDarkForest:
				if surfState != g.grassID || treeRng%6 != 0 {
					continue
				}
				g.placeDarkOakTree(chunk, x, ty, z, &rng)
			case BiomeFlowerForest:
				if surfState != g.grassID || treeRng%10 != 0 {
					continue
				}
				g.placeTree(chunk, x, ty, z, g.oakLogID, g.oakLeavesID, &rng)
			case BiomeMushroom:
				if surfState != g.myceliumID || treeRng%10 != 0 {
					continue
				}
				g.placeHugeMushroom(chunk, x, ty, z, &rng)
			case BiomeBadlands:
				// No trees in badlands; occasional cactus
				if surfState != g.redSandID || treeRng%30 != 0 || g.cactusID == 0 {
					continue
				}
				cactusH := 1 + int(treeRng/30)%3
				for dy := 1; dy <= cactusH; dy++ {
					g.setBlock(chunk, x, ty+dy, z, g.cactusID)
				}
			case BiomeOcean:
				continue
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

// placeSnowAndIce adds snow layers and ice in snowy biomes.
func (g *TerrainGenerator) placeSnowAndIce(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	if g.snowLayerID == 0 {
		return
	}
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			biome := biomes[z*16+x]
			if biome != BiomeSnowyPlains && biome != BiomeSnowyTaiga {
				continue
			}
			ty := heights[z*16+x]
			if ty < g.SeaLevel {
				// Water surface: place ice instead of snow
				if g.iceID != 0 {
					g.setBlock(chunk, x, g.SeaLevel, z, g.iceID)
				}
				continue
			}
			// Place snow layer on top of surface
			g.setBlock(chunk, x, ty+1, z, g.snowLayerID)
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

// placeSugarCane places sugar cane near water-adjacent positions.
func (g *TerrainGenerator) placeSugarCane(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	if g.sugarCaneID == 0 {
		return
	}

	for z := 1; z < 15; z++ {
		for x := 1; x < 15; x++ {
			biome := biomes[z*16+x]
			ty := heights[z*16+x]

			// Only place on grass or sand adjacent to water, above sea level.
			if ty < g.SeaLevel {
				continue
			}
			// Skip desert mountains and ocean.
			if biome == BiomeMountains || biome == BiomeOcean {
				continue
			}

			// Check if this block's surface is grass or sand.
			secIdx := (ty - g.MinY) / 16
			if secIdx < 0 || secIdx >= g.Sections {
				continue
			}
			localY := (ty - g.MinY) % 16
			idx := localY*16*16 + z*16 + x
			surfState := chunk.Sections[secIdx].GetBlock(idx)
			if surfState != g.grassID && surfState != g.sandID {
				continue
			}

			// Check if there is water adjacent (check neighboring heights).
			hasWaterNearby := false
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, nz := x+d[0], z+d[1]
				if nx < 0 || nx >= 16 || nz < 0 || nz >= 16 {
					continue
				}
				nh := heights[nz*16+nx]
				if nh < g.SeaLevel {
					hasWaterNearby = true
					break
				}
			}
			if !hasWaterNearby {
				continue
			}

			// 2% chance.
			rng := posHash(pos.X*16+x, ty, pos.Z*16+z, g.Seed+200)
			if rng%50 != 0 {
				continue
			}

			// Place 1-3 blocks of sugar cane.
			caneHeight := 1 + int(rng/50)%3
			for dy := 1; dy <= caneHeight; dy++ {
				g.setBlock(chunk, x, ty+dy, z, g.sugarCaneID)
			}
		}
	}
}

// placePumpkins places rare pumpkin blocks on grass surfaces in plains biome.
func (g *TerrainGenerator) placePumpkins(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	if g.pumpkinID == 0 {
		return
	}

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			biome := biomes[z*16+x]
			ty := heights[z*16+x]

			// Only place in plains biome above sea level
			if biome != BiomePlains || ty < g.SeaLevel {
				continue
			}

			// Check if surface is grass
			secIdx := (ty - g.MinY) / 16
			if secIdx < 0 || secIdx >= g.Sections {
				continue
			}
			localY := (ty - g.MinY) % 16
			idx := localY*16*16 + z*16 + x
			surfState := chunk.Sections[secIdx].GetBlock(idx)
			if surfState != g.grassID {
				continue
			}

			// 1% chance
			rng := posHash(pos.X*16+x, ty, pos.Z*16+z, g.Seed+300)
			if rng%100 != 0 {
				continue
			}

			// Place pumpkin on top of grass
			g.setBlock(chunk, x, ty+1, z, g.pumpkinID)
		}
	}
}
