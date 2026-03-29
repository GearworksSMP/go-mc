package gen

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// StructurePlacer generates simple structures during terrain generation.
type StructurePlacer struct {
	Seed int64

	// Block state IDs looked up at init time.
	cobblestoneID      level.BlocksState
	oakPlanksID        level.BlocksState
	chestID            level.BlocksState
	craftingTableID    level.BlocksState
	mossyCobblestoneID level.BlocksState
	spawnerID          level.BlocksState
	torchID            level.BlocksState
	waterID            level.BlocksState

	// Whether each block lookup succeeded.
	hasSpawner bool
	hasTorch   bool

	// Sub-placers for additional structure types.
	templePlacer          *TemplePlacer
	strongholdPlacer      *StrongholdPlacer
	mineshaftPlacer       *MineshaftPlacer
	villagePlacer         *VillagePlacer
	oceanMonumentPlacer   *OceanMonumentPlacer
	witchHutPlacer        *WitchHutPlacer
	pillagerOutpostPlacer *PillagerOutpostPlacer
	shipwreckPlacer        *ShipwreckPlacer
	buriedTreasurePlacer   *BuriedTreasurePlacer
	ruinedPortalPlacer     *RuinedPortalPlacer
	woodlandMansionPlacer  *WoodlandMansionPlacer
	underwaterRuinsPlacer  *UnderwaterRuinsPlacer
}

// NewStructurePlacer creates a StructurePlacer with resolved block state IDs.
func NewStructurePlacer(seed int64, waterID level.BlocksState) *StructurePlacer {
	sp := &StructurePlacer{
		Seed:    seed,
		waterID: waterID,
	}

	sp.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	sp.oakPlanksID, _ = block.ToStateID[block.OakPlanks{}]
	sp.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	sp.craftingTableID, _ = block.ToStateID[block.CraftingTable{}]
	sp.mossyCobblestoneID, _ = block.ToStateID[block.MossyCobblestone{}]

	var ok bool
	sp.spawnerID, ok = block.ToStateID[block.Spawner{}]
	sp.hasSpawner = ok

	sp.torchID, ok = block.ToStateID[block.Torch{}]
	sp.hasTorch = ok

	// Initialize sub-placers.
	sp.templePlacer = NewTemplePlacer(seed)
	sp.strongholdPlacer = NewStrongholdPlacer(seed)
	sp.mineshaftPlacer = NewMineshaftPlacer(seed)
	sp.villagePlacer = NewVillagePlacer(seed)
	sp.oceanMonumentPlacer = NewOceanMonumentPlacer(seed, waterID)
	sp.witchHutPlacer = NewWitchHutPlacer(seed)
	sp.pillagerOutpostPlacer = NewPillagerOutpostPlacer(seed)
	sp.shipwreckPlacer = NewShipwreckPlacer(seed)
	sp.buriedTreasurePlacer = NewBuriedTreasurePlacer(seed)
	sp.ruinedPortalPlacer = NewRuinedPortalPlacer(seed)
	sp.woodlandMansionPlacer = NewWoodlandMansionPlacer(seed)
	sp.underwaterRuinsPlacer = NewUnderwaterRuinsPlacer(seed, waterID)

	return sp
}

// PlaceStructures is called after terrain generation to place structures in a chunk.
func (sp *StructurePlacer) PlaceStructures(chunk *level.Chunk, chunkX, chunkZ int, gen *TerrainGenerator) {
	pos := game.ChunkPos{X: chunkX, Z: chunkZ}
	heights, biomes := gen.computeHeights(pos)

	// Try dungeon placement (2% chance).
	dHash := structureHash(chunkX, chunkZ, sp.Seed, 0xDEAD)
	if abs64(dHash)%50 < 1 {
		// Pick a local position in the interior (avoid edges for the 5x5 structure).
		lx := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xBEEF)) % 12)
		lz := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xCAFE)) % 12)
		// Underground Y between 20 and 50.
		dungeonY := 20 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xF00D))%31)

		// Only place if there's solid stone ground at this position.
		if sp.isSolidStone(chunk, lx, dungeonY, lz, gen) {
			sp.placeDungeon(chunk, lx, dungeonY, lz, gen)
		}
	}

	centerBiome := biomes[8*16+8]

	// Try village placement (3% chance in plains biome).
	housePlaced := false
	hHash := structureHash(chunkX, chunkZ, sp.Seed, 0xAAAA)
	if abs64(hHash)%100 < 3 {
		if centerBiome == BiomePlains {
			lx := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xBBBB))%8)
			lz := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xCCCC))%8)
			surfaceY := heights[lz*16+lx]
			if surfaceY >= gen.SeaLevel {
				// Vary building type based on hash.
				buildHash := abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x1234))
				buildType := int(buildHash % 5)

				if buildType == 0 {
					// Small house (original).
					sp.placeVillageHouse(chunk, lx, surfaceY, lz, gen)
				} else {
					// Enhanced buildings (medium house, large house, blacksmith, church).
					sp.villagePlacer.PlaceVillageBuildings(chunk, lx, surfaceY, lz, gen, buildType)
				}
				housePlaced = true
			}
		}
	}

	// Try village well and farm placement (only if a house was placed in this chunk).
	if housePlaced {
		wHash := structureHash(chunkX, chunkZ, sp.Seed, 0xDDDD)
		lx := 2 + int(abs64(wHash)%12)
		lz := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xEEEE))%12)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.placeVillageWell(chunk, lx, surfaceY, lz, gen)
		}

		// Place a farm near the village (50% chance).
		farmHash := structureHash(chunkX, chunkZ, sp.Seed, 0xFA12)
		if abs64(farmHash)%2 == 0 {
			fX := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xFA13))%10)
			fZ := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xFA14))%10)
			farmY := heights[fZ*16+fX]
			if farmY >= gen.SeaLevel {
				sp.villagePlacer.PlaceVillageFarm(chunk, fX, farmY, fZ, gen)
			}
		}

		// Place a road between the house and well.
		houseX := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xBBBB))%8)
		houseZ := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xCCCC))%8)
		wellX := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xDDDD))%12)
		wellZ := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xEEEE))%12)
		roadY := heights[houseZ*16+houseX]
		sp.villagePlacer.PlaceVillageRoad(chunk, houseX, houseZ, wellX, wellZ, roadY, gen)
	}

	// Try desert temple placement (2% chance in desert biome).
	tHash := structureHash(chunkX, chunkZ, sp.Seed, 0xD351)
	if abs64(tHash)%50 < 1 && centerBiome == BiomeDesert {
		lx := 3 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD352))%4)
		lz := 3 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD353))%4)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.templePlacer.PlaceDesertTemple(chunk, lx, surfaceY, lz, gen)
		}
	}

	// Try jungle temple placement (2% chance in forest biome).
	jHash := structureHash(chunkX, chunkZ, sp.Seed, 0xF0E5)
	if abs64(jHash)%50 < 1 && centerBiome == BiomeForest {
		lx := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xF0E6))%5)
		lz := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xF0E7))%5)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.templePlacer.PlaceJungleTemple(chunk, lx, surfaceY, lz, gen)
		}
	}

	// Try mineshaft placement (1% chance, underground).
	sp.mineshaftPlacer.PlaceMineshaft(chunk, chunkX, chunkZ, gen)

	// Try stronghold placement (at deterministic positions).
	sp.strongholdPlacer.PlaceStronghold(chunk, chunkX, chunkZ, gen)

	// Try ocean monument placement (1% chance in ocean biome).
	omHash := structureHash(chunkX, chunkZ, sp.Seed, 0x0CE4)
	if abs64(omHash)%100 < 1 && centerBiome == BiomeOcean {
		lx := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x0CE5)) % 2)
		lz := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x0CE6)) % 2)
		// Place underwater: use sea level minus the monument height.
		monumentY := gen.SeaLevel - 12
		sp.oceanMonumentPlacer.PlaceOceanMonument(chunk, lx, monumentY, lz, gen)
	}

	// Try witch hut placement (2% chance in taiga biome).
	// NOTE: Uses BiomeTaiga as a stand-in; should be BiomeSwamp when that constant is added.
	whHash := structureHash(chunkX, chunkZ, sp.Seed, 0xB17C)
	if abs64(whHash)%50 < 1 && centerBiome == BiomeTaiga {
		lx := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xB17D))%8)
		lz := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xB17E))%8)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.witchHutPlacer.PlaceWitchHut(chunk, lx, surfaceY, lz, gen)
		}
	}

	// Try pillager outpost placement (1.5% chance in plains or savanna biome).
	poHash := structureHash(chunkX, chunkZ, sp.Seed, 0xF117)
	if abs64(poHash)%200 < 3 && (centerBiome == BiomePlains || centerBiome == BiomeSavanna) {
		lx := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xF118))%6)
		lz := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xF119))%6)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.pillagerOutpostPlacer.PlaceOutpost(chunk, lx, surfaceY, lz, gen)
		}
	}

	// Try shipwreck placement (2% chance in ocean biome).
	swHash := structureHash(chunkX, chunkZ, sp.Seed, 0x5B10)
	if abs64(swHash)%50 < 1 && centerBiome == BiomeOcean {
		lx := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x5B11))%4)
		lz := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x5B12))%4)
		// Place on the ocean floor.
		floorY := gen.SeaLevel - 5 - int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0x5B13))%8)
		sp.shipwreckPlacer.PlaceShipwreck(chunk, lx, floorY, lz, gen)
	}

	// Try underwater ruins placement (1.5% chance in ocean biome).
	urHash := structureHash(chunkX, chunkZ, sp.Seed, 0xD170)
	if abs64(urHash)%200 < 3 && centerBiome == BiomeOcean {
		lx := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD175))%6)
		lz := 1 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD176))%6)
		// Place on the ocean floor.
		floorY := gen.SeaLevel - 5 - int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD177))%8)
		sp.underwaterRuinsPlacer.PlaceUnderwaterRuins(chunk, lx, floorY, lz, gen)
	}

	// Try buried treasure placement (3% chance at chunks bordering ocean).
	btHash := structureHash(chunkX, chunkZ, sp.Seed, 0xB710)
	if abs64(btHash)%100 < 3 {
		// Check if any edge biome is ocean (beach-adjacent).
		hasOceanEdge := biomes[0] == BiomeOcean || biomes[15] == BiomeOcean ||
			biomes[15*16] == BiomeOcean || biomes[15*16+15] == BiomeOcean
		if hasOceanEdge && centerBiome != BiomeOcean {
			lx := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xB711))%8)
			lz := 4 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xB712))%8)
			surfaceY := heights[lz*16+lx]
			if surfaceY >= gen.SeaLevel {
				sp.buriedTreasurePlacer.PlaceBuriedTreasure(chunk, lx, surfaceY, lz, gen)
			}
		}
	}

	// Try ruined portal placement (1% chance, any biome).
	rpHash := structureHash(chunkX, chunkZ, sp.Seed, 0xD0A0)
	if abs64(rpHash)%100 < 1 {
		lx := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD0A1))%8)
		lz := 2 + int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xD0A2))%8)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.ruinedPortalPlacer.PlaceRuinedPortal(chunk, lx, surfaceY, lz, gen)
		}
	}

	// Try woodland mansion placement (0.5% chance in dark forest biome).
	wmHash := structureHash(chunkX, chunkZ, sp.Seed, 0xAD00)
	if abs64(wmHash)%200 < 1 && centerBiome == BiomeDarkForest {
		lx := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xAD01)) % 2)
		lz := int(abs64(structureHash(chunkX, chunkZ, sp.Seed, 0xAD02)) % 2)
		surfaceY := heights[lz*16+lx]
		if surfaceY >= gen.SeaLevel {
			sp.woodlandMansionPlacer.PlaceWoodlandMansion(chunk, lx, surfaceY, lz, gen)
		}
	}
}

// placeDungeon places a 5x5x4 cobblestone dungeon room underground.
func (sp *StructurePlacer) placeDungeon(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	// 5x5x4 room: walls and floor are cobblestone, interior is air.
	// baseY is the floor level. Room extends from baseY to baseY+3 (4 blocks tall).
	for dy := 0; dy <= 3; dy++ {
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 5; dx++ {
				bx := localX + dx
				bz := localZ + dz
				by := baseY + dy

				// Skip out-of-chunk-bounds blocks.
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 4
				isFloor := dy == 0
				isCeiling := dy == 3

				if isFloor || isCeiling || isWall {
					sp.setBlock(chunk, bx, by, bz, sp.cobblestoneID, gen)
				} else {
					// Interior is air (state 0).
					sp.setBlock(chunk, bx, by, bz, 0, gen)
				}
			}
		}
	}

	// Place monster spawner (or mossy cobblestone fallback) in center of room floor.
	centerX := localX + 2
	centerZ := localZ + 2
	if centerX >= 0 && centerX < 16 && centerZ >= 0 && centerZ < 16 {
		if sp.hasSpawner {
			sp.setBlock(chunk, centerX, baseY+1, centerZ, sp.spawnerID, gen)
		} else {
			sp.setBlock(chunk, centerX, baseY+1, centerZ, sp.mossyCobblestoneID, gen)
		}
	}

	// Place 1-2 chests against walls.
	chestCount := 1
	ch := structureHash(localX, localZ, sp.Seed, 0x7777)
	if abs64(ch)%2 == 0 {
		chestCount = 2
	}

	// First chest: against the north wall (dz=1 side), interior position.
	cx1 := localX + 1
	cz1 := localZ + 1
	if cx1 >= 0 && cx1 < 16 && cz1 >= 0 && cz1 < 16 {
		sp.setBlock(chunk, cx1, baseY+1, cz1, sp.chestID, gen)
	}

	// Second chest: against the east wall (dx=3 side), interior position.
	if chestCount >= 2 {
		cx2 := localX + 3
		cz2 := localZ + 3
		if cx2 >= 0 && cx2 < 16 && cz2 >= 0 && cz2 < 16 {
			sp.setBlock(chunk, cx2, baseY+1, cz2, sp.chestID, gen)
		}
	}
}

// placeVillageHouse places a 5x5x5 village house on the surface.
func (sp *StructurePlacer) placeVillageHouse(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	// baseY is the terrain surface. The house sits on top of it.
	// Floor at baseY+1, walls from baseY+1 to baseY+4, roof at baseY+5.
	floorY := baseY + 1

	for dy := 0; dy <= 4; dy++ {
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 5; dx++ {
				bx := localX + dx
				bz := localZ + dz
				by := floorY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 4
				isFloor := dy == 0
				isRoof := dy == 4

				if isFloor {
					// Cobblestone floor.
					sp.setBlock(chunk, bx, by, bz, sp.cobblestoneID, gen)
				} else if isRoof {
					// Oak planks roof.
					sp.setBlock(chunk, bx, by, bz, sp.oakPlanksID, gen)
				} else if isWall {
					// Door opening: one block gap on the south wall center.
					if dz == 4 && dx == 2 && dy >= 1 && dy <= 2 {
						sp.setBlock(chunk, bx, by, bz, 0, gen) // air for door opening
					} else {
						sp.setBlock(chunk, bx, by, bz, sp.oakPlanksID, gen)
					}
				} else {
					// Interior is air.
					sp.setBlock(chunk, bx, by, bz, 0, gen)
				}
			}
		}
	}

	// Crafting table in a corner (interior position 1,1).
	ctX := localX + 1
	ctZ := localZ + 1
	if ctX >= 0 && ctX < 16 && ctZ >= 0 && ctZ < 16 {
		sp.setBlock(chunk, ctX, floorY+1, ctZ, sp.craftingTableID, gen)
	}

	// Torch on the floor near the wall.
	if sp.hasTorch {
		tX := localX + 2
		tZ := localZ + 1
		if tX >= 0 && tX < 16 && tZ >= 0 && tZ < 16 {
			sp.setBlock(chunk, tX, floorY+1, tZ, sp.torchID, gen)
		}
	}
}

// placeVillageWell places a 3x3 cobblestone well on the surface.
func (sp *StructurePlacer) placeVillageWell(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	floorY := baseY + 1

	// Dig down 3 blocks from floor for the water column.
	centerX := localX + 1
	centerZ := localZ + 1
	if centerX >= 0 && centerX < 16 && centerZ >= 0 && centerZ < 16 {
		for dy := -2; dy <= 0; dy++ {
			sp.setBlock(chunk, centerX, floorY+dy, centerZ, sp.waterID, gen)
		}
	}

	// Build 3x3 cobblestone ring walls, 3 blocks high.
	for dy := 0; dy < 3; dy++ {
		for dz := 0; dz < 3; dz++ {
			for dx := 0; dx < 3; dx++ {
				bx := localX + dx
				bz := localZ + dz
				by := floorY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isEdge := dx == 0 || dx == 2 || dz == 0 || dz == 2
				isCenter := dx == 1 && dz == 1

				if isEdge {
					sp.setBlock(chunk, bx, by, bz, sp.cobblestoneID, gen)
				} else if isCenter && dy > 0 {
					// Interior above floor is air (water is below floor).
					sp.setBlock(chunk, bx, by, bz, 0, gen)
				}
			}
		}
	}

	// Cobblestone blocks on top corners.
	corners := [][2]int{{0, 0}, {0, 2}, {2, 0}, {2, 2}}
	for _, c := range corners {
		bx := localX + c[0]
		bz := localZ + c[1]
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			sp.setBlock(chunk, bx, floorY+3, bz, sp.cobblestoneID, gen)
		}
	}
}

// setBlock sets a block in the chunk at local coordinates (x 0-15, z 0-15, worldY).
func (sp *StructurePlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}

// isSolidStone checks if the block at the given position is stone.
func (sp *StructurePlacer) isSolidStone(chunk *level.Chunk, x, worldY, z int, gen *TerrainGenerator) bool {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return false
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	state := chunk.Sections[secIdx].GetBlock(idx)
	return state == gen.stoneID
}

// structureHash returns a deterministic hash for structure placement decisions.
func structureHash(x, z int, seed int64, salt int64) int64 {
	h := seed ^ (int64(x)*341873128712 + int64(z)*132897987541 + salt)
	h = h ^ (h >> 16)
	h = h * 0x45d9f3b
	h = h ^ (h >> 16)
	return h
}

// abs64 returns the absolute value of an int64.
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
