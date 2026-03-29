package gen

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// Ocean vegetation block IDs, resolved once at init.
type oceanVegetationBlocks struct {
	kelpID      level.BlocksState
	kelpPlantID level.BlocksState
	seagrassID  level.BlocksState
	tallSeagrassLowerID level.BlocksState
	tallSeagrassUpperID level.BlocksState

	// Coral blocks
	tubeCoralBlockID   level.BlocksState
	brainCoralBlockID  level.BlocksState
	bubbleCoralBlockID level.BlocksState
	fireCoralBlockID   level.BlocksState
	hornCoralBlockID   level.BlocksState

	// Coral fans (floor-placed)
	tubeCoralFanID   level.BlocksState
	brainCoralFanID  level.BlocksState
	bubbleCoralFanID level.BlocksState
	fireCoralFanID   level.BlocksState
	hornCoralFanID   level.BlocksState

	// Sea pickle
	seaPickle1ID level.BlocksState
	seaPickle2ID level.BlocksState
	seaPickle3ID level.BlocksState
	seaPickle4ID level.BlocksState
}

// newOceanVegetationBlocks resolves all ocean vegetation block state IDs.
// Called once during TerrainGenerator initialization.
func newOceanVegetationBlocks() oceanVegetationBlocks {
	var b oceanVegetationBlocks

	b.kelpID, _ = block.ToStateID[block.Kelp{Age: 0}]
	b.kelpPlantID, _ = block.ToStateID[block.KelpPlant{}]
	b.seagrassID, _ = block.ToStateID[block.Seagrass{}]
	b.tallSeagrassLowerID, _ = block.ToStateID[block.TallSeagrass{Half: block.DoubleBlockHalfLower}]
	b.tallSeagrassUpperID, _ = block.ToStateID[block.TallSeagrass{Half: block.DoubleBlockHalfUpper}]

	b.tubeCoralBlockID, _ = block.ToStateID[block.TubeCoralBlock{}]
	b.brainCoralBlockID, _ = block.ToStateID[block.BrainCoralBlock{}]
	b.bubbleCoralBlockID, _ = block.ToStateID[block.BubbleCoralBlock{}]
	b.fireCoralBlockID, _ = block.ToStateID[block.FireCoralBlock{}]
	b.hornCoralBlockID, _ = block.ToStateID[block.HornCoralBlock{}]

	b.tubeCoralFanID, _ = block.ToStateID[block.TubeCoralFan{Waterlogged: true}]
	b.brainCoralFanID, _ = block.ToStateID[block.BrainCoralFan{Waterlogged: true}]
	b.bubbleCoralFanID, _ = block.ToStateID[block.BubbleCoralFan{Waterlogged: true}]
	b.fireCoralFanID, _ = block.ToStateID[block.FireCoralFan{Waterlogged: true}]
	b.hornCoralFanID, _ = block.ToStateID[block.HornCoralFan{Waterlogged: true}]

	b.seaPickle1ID, _ = block.ToStateID[block.SeaPickle{Pickles: 1, Waterlogged: true}]
	b.seaPickle2ID, _ = block.ToStateID[block.SeaPickle{Pickles: 2, Waterlogged: true}]
	b.seaPickle3ID, _ = block.ToStateID[block.SeaPickle{Pickles: 3, Waterlogged: true}]
	b.seaPickle4ID, _ = block.ToStateID[block.SeaPickle{Pickles: 4, Waterlogged: true}]

	return b
}

// coralBlocks returns the 5 coral block IDs as a slice for random selection.
func (b *oceanVegetationBlocks) coralBlocks() [5]level.BlocksState {
	return [5]level.BlocksState{
		b.tubeCoralBlockID, b.brainCoralBlockID, b.bubbleCoralBlockID,
		b.fireCoralBlockID, b.hornCoralBlockID,
	}
}

// coralFans returns the 5 coral fan IDs as a slice for random selection.
func (b *oceanVegetationBlocks) coralFans() [5]level.BlocksState {
	return [5]level.BlocksState{
		b.tubeCoralFanID, b.brainCoralFanID, b.bubbleCoralFanID,
		b.fireCoralFanID, b.hornCoralFanID,
	}
}

// seaPickle returns a sea pickle block with the given count (1-4).
func (b *oceanVegetationBlocks) seaPickle(count int) level.BlocksState {
	switch count {
	case 2:
		return b.seaPickle2ID
	case 3:
		return b.seaPickle3ID
	case 4:
		return b.seaPickle4ID
	default:
		return b.seaPickle1ID
	}
}

// decorateOceanFloor places kelp, seagrass, and coral reefs on ocean floor blocks.
// Called from terrain.go after chunk generation for ocean biomes.
func (g *TerrainGenerator) decorateOceanFloor(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	ob := &g.oceanBlocks

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			biome := biomes[z*16+x]
			if biome != BiomeOcean {
				continue
			}

			floorY := heights[z*16+x]
			if floorY >= g.SeaLevel {
				continue // not underwater
			}

			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z
			rng := posHash(worldX, floorY, worldZ, g.Seed+700)

			// Check if this is a warm ocean area for coral reefs (use biome noise).
			warmth := g.biomeNoise.Noise2D(float64(worldX)*0.01, float64(worldZ)*0.01)
			isWarmOcean := warmth > 0.3

			// Check floor block is a valid substrate.
			floorBlock := g.getBlock(chunk, x, floorY, z)
			isValidFloor := floorBlock == g.gravelID || floorBlock == g.sandID || floorBlock == g.dirtID

			if !isValidFloor {
				continue
			}

			// Ensure there is water above the floor.
			if g.getBlock(chunk, x, floorY+1, z) != g.waterID {
				continue
			}

			if isWarmOcean {
				// Warm ocean: coral reefs and sea pickles.
				g.placeCoralReef(chunk, x, floorY, z, ob, rng)
			} else {
				// Normal ocean: kelp and seagrass.
				g.placeKelpOrSeagrass(chunk, x, floorY, z, ob, rng)
			}
		}
	}
}

// placeKelpOrSeagrass places kelp columns or seagrass at a single ocean floor position.
func (g *TerrainGenerator) placeKelpOrSeagrass(chunk *level.Chunk, x, floorY, z int, ob *oceanVegetationBlocks, rng uint64) {
	// Kelp: ~15% chance.
	if rng%100 < 15 {
		waterDepth := g.SeaLevel - floorY - 1
		if waterDepth < 2 {
			return
		}
		maxHeight := waterDepth - 1 // leave top block as water
		if maxHeight > 26 {
			maxHeight = 26
		}
		if maxHeight < 2 {
			maxHeight = 2
		}
		height := 2 + int((rng/100)%uint64(maxHeight-1))

		// Place kelp_plant blocks for the stem, kelp on top.
		for dy := 1; dy < height; dy++ {
			g.setBlock(chunk, x, floorY+dy, z, ob.kelpPlantID)
		}
		g.setBlock(chunk, x, floorY+height, z, ob.kelpID)
		return
	}

	// Seagrass: ~10% chance.
	if (rng/100)%100 < 10 {
		waterDepth := g.SeaLevel - floorY - 1
		// Tall seagrass needs at least 2 water blocks.
		if waterDepth >= 2 && (rng/10000)%3 == 0 {
			g.setBlock(chunk, x, floorY+1, z, ob.tallSeagrassLowerID)
			g.setBlock(chunk, x, floorY+2, z, ob.tallSeagrassUpperID)
		} else if waterDepth >= 1 {
			g.setBlock(chunk, x, floorY+1, z, ob.seagrassID)
		}
	}
}

// placeCoralReef places a coral cluster with fans and sea pickles at an ocean floor position.
func (g *TerrainGenerator) placeCoralReef(chunk *level.Chunk, x, floorY, z int, ob *oceanVegetationBlocks, rng uint64) {
	// Coral cluster: ~12% chance at each position.
	if rng%100 >= 12 {
		// Even outside a cluster, scatter sea pickles (3% chance).
		if (rng/100)%100 < 3 {
			count := 1 + int((rng/10000)%4)
			g.setBlock(chunk, x, floorY+1, z, ob.seaPickle(count))
		}
		return
	}

	coralBlocks := ob.coralBlocks()
	coralFans := ob.coralFans()

	// Cluster of 3-8 coral blocks.
	clusterSize := 3 + int((rng/100)%6)
	placed := 0
	rngState := rng

	for i := 0; i < clusterSize; i++ {
		rngState = (rngState >> 5) | (rngState << 59)
		dx := int(rngState%3) - 1
		rngState = (rngState >> 5) | (rngState << 59)
		dz := int(rngState%3) - 1
		rngState = (rngState >> 5) | (rngState << 59)
		dy := int(rngState % 2)

		bx := x + dx
		bz := z + dz
		by := floorY + 1 + dy

		if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
			continue
		}

		// Only place in water.
		if g.getBlock(chunk, bx, by, bz) != g.waterID {
			continue
		}

		// Pick a random coral type.
		coralType := int((rngState / 2) % 5)
		g.setBlock(chunk, bx, by, bz, coralBlocks[coralType])
		placed++

		// Place a coral fan on top (50% chance).
		if (rngState/10)%2 == 0 && by+1 < g.SeaLevel {
			if g.getBlock(chunk, bx, by+1, bz) == g.waterID {
				fanType := int((rngState / 20) % 5)
				g.setBlock(chunk, bx, by+1, bz, coralFans[fanType])
			}
		}
	}

	// Place sea pickles for light near the cluster.
	if placed > 0 && (rngState/100)%2 == 0 {
		pickleCount := 1 + int((rngState/200)%4)
		g.setBlock(chunk, x, floorY+1, z, ob.seaPickle(pickleCount))
	}
}
