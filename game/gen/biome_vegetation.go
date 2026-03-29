package gen

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
)

// getSurfaceBlock returns the block state at the surface height, or 0 if out of bounds.
func (g *TerrainGenerator) getSurfaceBlock(chunk *level.Chunk, x, ty, z int) level.BlocksState {
	secIdx := (ty - g.MinY) / 16
	if secIdx < 0 || secIdx >= g.Sections {
		return 0
	}
	localY := (ty - g.MinY) % 16
	idx := localY*16*16 + z*16 + x
	return chunk.Sections[secIdx].GetBlock(idx)
}

// placeJungleTree places a tall jungle tree (8-12 block trunk) with vines.
func (g *TerrainGenerator) placeJungleTree(chunk *level.Chunk, x, ty, z int, rng *uint64) {
	trunkH := 8 + int(*rng%5)
	*rng = (*rng >> 3) | (*rng << 61)

	for dy := 1; dy <= trunkH; dy++ {
		g.setBlock(chunk, x, ty+dy, z, g.jungleLogID)
	}

	topY := ty + trunkH
	for dy := -2; dy <= 1; dy++ {
		radius := 2
		if dy == 1 || dy == -2 {
			radius = 1
		}
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dz*dz > radius*radius+1 {
					continue
				}
				lx, lz := x+dx, z+dz
				if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
					continue
				}
				if dx == 0 && dz == 0 && dy <= 0 {
					continue
				}
				g.setBlock(chunk, lx, topY+dy, lz, g.jungleLeavesID)
			}
		}
	}

	if g.vineID == 0 {
		return
	}
	for dy := 2; dy <= trunkH-1; dy++ {
		vineY := ty + dy
		if x > 0 {
			g.setBlock(chunk, x-1, vineY, z, g.vineID)
		}
		if x < 15 {
			g.setBlock(chunk, x+1, vineY, z, g.vineID)
		}
		if z > 0 {
			g.setBlock(chunk, x, vineY, z-1, g.vineID)
		}
		if z < 15 {
			g.setBlock(chunk, x, vineY, z+1, g.vineID)
		}
	}
}

// placeSwampTree places a short oak tree with vines.
func (g *TerrainGenerator) placeSwampTree(chunk *level.Chunk, x, ty, z int, rng *uint64) {
	// Reuse the standard tree placement, then add vines
	g.placeTree(chunk, x, ty, z, g.oakLogID, g.oakLeavesID, rng)

	if g.vineID == 0 {
		return
	}
	// Estimate trunk height (placeTree uses 4 + rng%3, but rng was already advanced)
	// Hang vines around the leaf area
	for dy := 3; dy <= 5; dy++ {
		vineY := ty + dy
		for _, d := range [][2]int{{-2, 0}, {2, 0}, {0, -2}, {0, 2}} {
			lx, lz := x+d[0], z+d[1]
			if lx >= 0 && lx < 16 && lz >= 0 && lz < 16 {
				g.setBlock(chunk, lx, vineY, lz, g.vineID)
			}
		}
	}
}

// placeDarkOakTree places a dark oak tree with a 2x2 trunk.
func (g *TerrainGenerator) placeDarkOakTree(chunk *level.Chunk, x, ty, z int, rng *uint64) {
	trunkH := 5 + int(*rng%3)
	*rng = (*rng >> 3) | (*rng << 61)

	if x >= 15 || z >= 15 {
		return
	}

	for dy := 1; dy <= trunkH; dy++ {
		g.setBlock(chunk, x, ty+dy, z, g.darkOakLogID)
		g.setBlock(chunk, x+1, ty+dy, z, g.darkOakLogID)
		g.setBlock(chunk, x, ty+dy, z+1, g.darkOakLogID)
		g.setBlock(chunk, x+1, ty+dy, z+1, g.darkOakLogID)
	}

	topY := ty + trunkH
	for dy := -1; dy <= 1; dy++ {
		radius := 3
		if dy == 1 {
			radius = 1
		}
		for dz := -radius; dz <= radius; dz++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dz*dz > radius*radius+1 {
					continue
				}
				lx, lz := x+dx, z+dz
				if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
					continue
				}
				if dy <= 0 && (dx == 0 || dx == 1) && (dz == 0 || dz == 1) {
					continue
				}
				g.setBlock(chunk, lx, topY+dy, lz, g.darkOakLeavesID)
			}
		}
	}
}

// placeHugeMushroom places a huge red or brown mushroom.
func (g *TerrainGenerator) placeHugeMushroom(chunk *level.Chunk, x, ty, z int, rng *uint64) {
	stemH := 4 + int(*rng%3)
	isRed := *rng%2 == 0
	*rng = (*rng >> 3) | (*rng << 61)

	for dy := 1; dy <= stemH; dy++ {
		g.setBlock(chunk, x, ty+dy, z, g.mushroomStemID)
	}

	capID := g.brownMushroomBlockID
	if isRed {
		capID = g.redMushroomBlockID
	}

	capY := ty + stemH + 1
	capRadius := 1
	if !isRed {
		capRadius = 2
	}

	for dz := -capRadius; dz <= capRadius; dz++ {
		for dx := -capRadius; dx <= capRadius; dx++ {
			if dx*dx+dz*dz > capRadius*capRadius+capRadius {
				continue
			}
			lx, lz := x+dx, z+dz
			if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
				continue
			}
			g.setBlock(chunk, lx, capY, lz, capID)
		}
	}
}

// placeBiomeVegetation places biome-specific ground vegetation.
func (g *TerrainGenerator) placeBiomeVegetation(chunk *level.Chunk, pos game.ChunkPos, heights [256]int, biomes [256]BiomeType) {
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			biome := biomes[z*16+x]
			ty := heights[z*16+x]

			worldX := pos.X*16 + x
			worldZ := pos.Z*16 + z
			rng := posHash(worldX, ty, worldZ, g.Seed+500)

			switch biome {
			case BiomeFlowerForest:
				if ty >= g.SeaLevel {
					g.placeFlowerForestVegetation(chunk, x, ty, z, rng)
				}
			case BiomeDarkForest:
				if ty >= g.SeaLevel {
					g.placeDarkForestVegetation(chunk, x, ty, z, rng)
				}
			case BiomeSwamp:
				g.placeSwampVegetation(chunk, x, ty, z, rng)
			case BiomeMushroom:
				if ty >= g.SeaLevel {
					g.placeMushroomVegetation(chunk, x, ty, z, rng)
				}
			case BiomeBadlands:
				if ty >= g.SeaLevel {
					g.placeBadlandsVegetation(chunk, x, ty, z, rng)
				}
			}
		}
	}
}

// placeFlowerForestVegetation places varied flower patches.
func (g *TerrainGenerator) placeFlowerForestVegetation(chunk *level.Chunk, x, ty, z int, rng uint64) {
	if rng%6 != 0 {
		return
	}
	if g.getSurfaceBlock(chunk, x, ty, z) != g.grassID {
		return
	}

	flowers := [4]level.BlocksState{g.poppyID, g.dandelionID, g.azureBluetID, g.oxeyeDaisyID}
	flower := flowers[rng/6%4]
	if flower == 0 {
		return
	}
	g.setBlock(chunk, x, ty+1, z, flower)
}

// placeDarkForestVegetation places mushrooms on the ground.
func (g *TerrainGenerator) placeDarkForestVegetation(chunk *level.Chunk, x, ty, z int, rng uint64) {
	if rng%12 != 0 {
		return
	}
	if g.getSurfaceBlock(chunk, x, ty, z) != g.grassID {
		return
	}

	if rng%2 == 0 && g.brownMushroomID != 0 {
		g.setBlock(chunk, x, ty+1, z, g.brownMushroomID)
	} else if g.redMushroomID != 0 {
		g.setBlock(chunk, x, ty+1, z, g.redMushroomID)
	}
}

// placeSwampVegetation places lily pads on water surfaces.
func (g *TerrainGenerator) placeSwampVegetation(chunk *level.Chunk, x, ty, z int, rng uint64) {
	if g.lilyPadID == 0 || ty >= g.SeaLevel || rng%8 != 0 {
		return
	}

	// Verify water at sea level before placing lily pad on top
	if g.getSurfaceBlock(chunk, x, g.SeaLevel, z) != g.waterID {
		return
	}
	g.setBlock(chunk, x, g.SeaLevel+1, z, g.lilyPadID)
}

// placeMushroomVegetation places small mushrooms on mycelium.
func (g *TerrainGenerator) placeMushroomVegetation(chunk *level.Chunk, x, ty, z int, rng uint64) {
	if rng%8 != 0 {
		return
	}
	if g.getSurfaceBlock(chunk, x, ty, z) != g.myceliumID {
		return
	}

	if rng%2 == 0 && g.brownMushroomID != 0 {
		g.setBlock(chunk, x, ty+1, z, g.brownMushroomID)
	} else if g.redMushroomID != 0 {
		g.setBlock(chunk, x, ty+1, z, g.redMushroomID)
	}
}

// placeBadlandsVegetation places dead bushes on red sand.
func (g *TerrainGenerator) placeBadlandsVegetation(chunk *level.Chunk, x, ty, z int, rng uint64) {
	if rng%10 != 0 || g.deadBushID == 0 {
		return
	}
	if g.getSurfaceBlock(chunk, x, ty, z) != g.redSandID {
		return
	}
	g.setBlock(chunk, x, ty+1, z, g.deadBushID)
}
