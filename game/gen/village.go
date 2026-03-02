package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// VillagePlacer generates enhanced village structures beyond the basic house and well.
type VillagePlacer struct {
	Seed int64

	// Block state IDs looked up at init time.
	oakPlanksID     level.BlocksState
	cobblestoneID   level.BlocksState
	oakLogID        level.BlocksState
	dirtPathID      level.BlocksState
	chestID         level.BlocksState
	craftingTableID level.BlocksState
	furnaceID       level.BlocksState
	torchID         level.BlocksState
	oakFenceID      level.BlocksState
	wheatID         level.BlocksState
	farmlandID      level.BlocksState
	waterID         level.BlocksState
	lavaID          level.BlocksState
	airID           level.BlocksState
}

// NewVillagePlacer creates a VillagePlacer with resolved block state IDs.
func NewVillagePlacer(seed int64) *VillagePlacer {
	vp := &VillagePlacer{Seed: seed}

	vp.oakPlanksID, _ = block.ToStateID[block.OakPlanks{}]
	vp.cobblestoneID, _ = block.ToStateID[block.Cobblestone{}]
	vp.oakLogID, _ = block.ToStateID[block.OakLog{Axis: block.Y}]
	vp.dirtPathID, _ = block.ToStateID[block.DirtPath{}]
	vp.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	vp.craftingTableID, _ = block.ToStateID[block.CraftingTable{}]
	vp.furnaceID, _ = block.ToStateID[block.Furnace{Facing: block.North, Lit: false}]
	vp.torchID, _ = block.ToStateID[block.Torch{}]
	vp.oakFenceID, _ = block.ToStateID[block.OakFence{}]
	vp.wheatID, _ = block.ToStateID[block.Wheat{Age: 7}] // fully grown
	vp.farmlandID, _ = block.ToStateID[block.Farmland{Moisture: 7}]
	vp.waterID, _ = block.ToStateID[block.Water{Level: 0}]
	vp.lavaID, _ = block.ToStateID[block.Lava{Level: 0}]
	vp.airID = 0

	return vp
}

// PlaceVillageBuildings places a village building of the given type.
//
//	buildType 0: Small house (5x5) - existing pattern, handled in structures.go
//	buildType 1: Medium house (7x7, two floors)
//	buildType 2: Large house (9x7, L-shaped)
//	buildType 3: Blacksmith (7x7, outdoor forge)
//	buildType 4: Church (7x11, tall tower)
func (vp *VillagePlacer) PlaceVillageBuildings(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator, buildType int) {
	switch buildType {
	case 1:
		vp.placeMediumHouse(chunk, localX, baseY, localZ, gen)
	case 2:
		vp.placeLargeHouse(chunk, localX, baseY, localZ, gen)
	case 3:
		vp.placeBlacksmith(chunk, localX, baseY, localZ, gen)
	case 4:
		vp.placeChurch(chunk, localX, baseY, localZ, gen)
	}
}

// placeMediumHouse places a 7x7 two-story house.
func (vp *VillagePlacer) placeMediumHouse(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	floorY := baseY + 1

	// Two floors, each 4 blocks high (floor + 3 air + ceiling).
	for floor := 0; floor < 2; floor++ {
		fy := floorY + floor*4
		for dy := 0; dy <= 3; dy++ {
			by := fy + dy
			for dz := 0; dz < 7; dz++ {
				for dx := 0; dx < 7; dx++ {
					bx := localX + dx
					bz := localZ + dz
					if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
						continue
					}

					isCorner := (dx == 0 || dx == 6) && (dz == 0 || dz == 6)
					isWall := dx == 0 || dx == 6 || dz == 0 || dz == 6
					isFloor := dy == 0
					isCeiling := dy == 3

					var stateID level.BlocksState
					switch {
					case isFloor:
						stateID = vp.cobblestoneID
					case isCeiling:
						stateID = vp.oakPlanksID
					case isCorner:
						stateID = vp.oakLogID
					case isWall:
						// Windows: air gaps on the second block up, centered on each wall.
						if dy == 2 && ((dx == 3 && (dz == 0 || dz == 6)) || (dz == 3 && (dx == 0 || dx == 6))) {
							stateID = vp.airID
						} else {
							stateID = vp.oakPlanksID
						}
					default:
						stateID = vp.airID
					}

					vp.setBlock(chunk, bx, by, bz, stateID, gen)
				}
			}
		}
	}

	// Roof (flat oak planks on top).
	for dz := 0; dz < 7; dz++ {
		for dx := 0; dx < 7; dx++ {
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}
			vp.setBlock(chunk, bx, floorY+8, bz, vp.oakPlanksID, gen)
		}
	}

	// Door opening (south wall, ground floor).
	doorX := localX + 3
	doorZ := localZ + 6
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		vp.setBlock(chunk, doorX, floorY+1, doorZ, vp.airID, gen)
		vp.setBlock(chunk, doorX, floorY+2, doorZ, vp.airID, gen)
	}

	// Torch inside ground floor.
	tX := localX + 3
	tZ := localZ + 1
	if tX >= 0 && tX < 16 && tZ >= 0 && tZ < 16 {
		vp.setBlock(chunk, tX, floorY+1, tZ, vp.torchID, gen)
	}

	// Torch inside upper floor.
	tX2 := localX + 3
	tZ2 := localZ + 1
	if tX2 >= 0 && tX2 < 16 && tZ2 >= 0 && tZ2 < 16 {
		vp.setBlock(chunk, tX2, floorY+5, tZ2, vp.torchID, gen)
	}
}

// placeLargeHouse places a 9x7 L-shaped house with multiple rooms.
func (vp *VillagePlacer) placeLargeHouse(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	floorY := baseY + 1

	// Main section: 9x5 (the long part).
	for dy := 0; dy <= 4; dy++ {
		by := floorY + dy
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 9; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isCorner := (dx == 0 || dx == 8) && (dz == 0 || dz == 4)
				isWall := dx == 0 || dx == 8 || dz == 0 || dz == 4
				isFloor := dy == 0
				isRoof := dy == 4

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = vp.cobblestoneID
				case isRoof:
					stateID = vp.oakPlanksID
				case isCorner:
					stateID = vp.oakLogID
				case isWall:
					stateID = vp.oakPlanksID
				default:
					stateID = vp.airID
				}

				vp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// L-extension: 5x4 attached to the east side (the short part).
	for dy := 0; dy <= 4; dy++ {
		by := floorY + dy
		for dz := 0; dz < 4; dz++ {
			for dx := 0; dx < 5; dx++ {
				bx := localX + 5 + dx // starts at column 5 of the main section
				bz := localZ + 4 + dz // starts where the main section ends
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 3
				isFloor := dy == 0
				isRoof := dy == 4

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = vp.cobblestoneID
				case isRoof:
					stateID = vp.oakPlanksID
				case isWall:
					stateID = vp.oakPlanksID
				default:
					stateID = vp.airID
				}

				vp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Opening between main room and L-extension.
	for dy := 1; dy <= 3; dy++ {
		bx := localX + 6
		bz := localZ + 4
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			vp.setBlock(chunk, bx, floorY+dy, bz, vp.airID, gen)
		}
		bx2 := localX + 7
		if bx2 >= 0 && bx2 < 16 && bz >= 0 && bz < 16 {
			vp.setBlock(chunk, bx2, floorY+dy, bz, vp.airID, gen)
		}
	}

	// Interior wall dividing the main section into two rooms.
	divX := localX + 4
	for dz := 1; dz < 4; dz++ {
		bz := localZ + dz
		if divX >= 0 && divX < 16 && bz >= 0 && bz < 16 {
			for dy := 1; dy <= 3; dy++ {
				vp.setBlock(chunk, divX, floorY+dy, bz, vp.oakPlanksID, gen)
			}
		}
	}
	// Door in the dividing wall.
	if divX >= 0 && divX < 16 && localZ+2 >= 0 && localZ+2 < 16 {
		vp.setBlock(chunk, divX, floorY+1, localZ+2, vp.airID, gen)
		vp.setBlock(chunk, divX, floorY+2, localZ+2, vp.airID, gen)
	}

	// External door on south wall.
	doorX := localX + 2
	doorZ := localZ + 4
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		vp.setBlock(chunk, doorX, floorY+1, doorZ, vp.airID, gen)
		vp.setBlock(chunk, doorX, floorY+2, doorZ, vp.airID, gen)
	}

	// Crafting table and furnace in first room.
	ctX := localX + 1
	ctZ := localZ + 1
	if ctX >= 0 && ctX < 16 && ctZ >= 0 && ctZ < 16 {
		vp.setBlock(chunk, ctX, floorY+1, ctZ, vp.craftingTableID, gen)
	}
	fX := localX + 2
	fZ := localZ + 1
	if fX >= 0 && fX < 16 && fZ >= 0 && fZ < 16 {
		vp.setBlock(chunk, fX, floorY+1, fZ, vp.furnaceID, gen)
	}

	// Torches.
	for _, pos := range [][2]int{{1, 3}, {7, 1}, {7, 6}} {
		tX := localX + pos[0]
		tZ := localZ + pos[1]
		if tX >= 0 && tX < 16 && tZ >= 0 && tZ < 16 {
			vp.setBlock(chunk, tX, floorY+1, tZ, vp.torchID, gen)
		}
	}
}

// placeBlacksmith places a 7x7 blacksmith building with an outdoor forge area.
func (vp *VillagePlacer) placeBlacksmith(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	floorY := baseY + 1

	// Main building: 7x5 enclosed area.
	for dy := 0; dy <= 4; dy++ {
		by := floorY + dy
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 7; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isCorner := (dx == 0 || dx == 6) && (dz == 0 || dz == 4)
				isWall := dx == 0 || dx == 6 || dz == 0 || dz == 4
				isFloor := dy == 0
				isRoof := dy == 4

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = vp.cobblestoneID
				case isRoof:
					stateID = vp.cobblestoneID // stone roof for the forge
				case isCorner:
					stateID = vp.oakLogID
				case isWall:
					stateID = vp.cobblestoneID
				default:
					stateID = vp.airID
				}

				vp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Door opening.
	doorX := localX + 3
	doorZ := localZ + 4
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		vp.setBlock(chunk, doorX, floorY+1, doorZ, vp.airID, gen)
		vp.setBlock(chunk, doorX, floorY+2, doorZ, vp.airID, gen)
	}

	// Outdoor forge area: 7x2 extending south of the building.
	for dz := 5; dz < 7; dz++ {
		for dx := 0; dx < 7; dx++ {
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}
			// Cobblestone floor.
			vp.setBlock(chunk, bx, floorY, bz, vp.cobblestoneID, gen)
		}
	}

	// Lava pool in forge area (2x1).
	lavaX := localX + 2
	lavaZ := localZ + 5
	if lavaX >= 0 && lavaX < 16 && lavaZ >= 0 && lavaZ < 16 {
		vp.setBlock(chunk, lavaX, floorY+1, lavaZ, vp.lavaID, gen)
	}
	lavaX2 := localX + 3
	if lavaX2 >= 0 && lavaX2 < 16 && lavaZ >= 0 && lavaZ < 16 {
		vp.setBlock(chunk, lavaX2, floorY+1, lavaZ, vp.lavaID, gen)
	}

	// Fence border around forge area.
	for dx := 0; dx < 7; dx++ {
		bx := localX + dx
		bz := localZ + 6
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			vp.setBlock(chunk, bx, floorY+1, bz, vp.oakFenceID, gen)
		}
	}
	for dz := 5; dz <= 6; dz++ {
		for _, dx := range []int{0, 6} {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				vp.setBlock(chunk, bx, floorY+1, bz, vp.oakFenceID, gen)
			}
		}
	}

	// Chest with loot inside.
	chestX := localX + 1
	chestZ := localZ + 1
	if chestX >= 0 && chestX < 16 && chestZ >= 0 && chestZ < 16 {
		vp.setBlock(chunk, chestX, floorY+1, chestZ, vp.chestID, gen)
	}

	// Furnace inside.
	fX := localX + 5
	fZ := localZ + 1
	if fX >= 0 && fX < 16 && fZ >= 0 && fZ < 16 {
		vp.setBlock(chunk, fX, floorY+1, fZ, vp.furnaceID, gen)
	}

	// Crafting table inside.
	ctX := localX + 5
	ctZ := localZ + 2
	if ctX >= 0 && ctX < 16 && ctZ >= 0 && ctZ < 16 {
		vp.setBlock(chunk, ctX, floorY+1, ctZ, vp.craftingTableID, gen)
	}
}

// placeChurch places a 7x11 church with a tall tower.
func (vp *VillagePlacer) placeChurch(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	floorY := baseY + 1

	// Main nave: 7x7, 6 blocks tall.
	for dy := 0; dy <= 5; dy++ {
		by := floorY + dy
		for dz := 0; dz < 7; dz++ {
			for dx := 0; dx < 7; dx++ {
				bx := localX + dx
				bz := localZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isCorner := (dx == 0 || dx == 6) && (dz == 0 || dz == 6)
				isWall := dx == 0 || dx == 6 || dz == 0 || dz == 6
				isFloor := dy == 0
				isRoof := dy == 5

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = vp.cobblestoneID
				case isRoof:
					stateID = vp.cobblestoneID
				case isCorner:
					stateID = vp.oakLogID
				case isWall:
					// Window openings on east and west walls.
					if dy == 3 && (dx == 0 || dx == 6) && dz == 3 {
						stateID = vp.airID
					} else {
						stateID = vp.cobblestoneID
					}
				default:
					stateID = vp.airID
				}

				vp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Tower: 3x3 extending from the back (north side), 10 blocks tall total.
	towerBaseX := localX + 2
	towerBaseZ := localZ - 3 // extends north of the main nave
	if towerBaseZ < 0 {
		towerBaseZ = 0
	}

	towerHeight := 10
	for dy := 0; dy < towerHeight; dy++ {
		by := floorY + dy
		for dz := 0; dz < 3; dz++ {
			for dx := 0; dx < 3; dx++ {
				bx := towerBaseX + dx
				bz := towerBaseZ + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isEdge := dx == 0 || dx == 2 || dz == 0 || dz == 2
				isFloor := dy == 0
				isTop := dy == towerHeight-1

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = vp.cobblestoneID
				case isTop:
					stateID = vp.cobblestoneID
				case isEdge:
					stateID = vp.cobblestoneID
				default:
					stateID = vp.airID
				}

				vp.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Opening between tower and nave.
	openX := towerBaseX + 1
	openZ := localZ
	if openX >= 0 && openX < 16 && openZ >= 0 && openZ < 16 {
		for dy := 1; dy <= 3; dy++ {
			vp.setBlock(chunk, openX, floorY+dy, openZ, vp.airID, gen)
		}
	}

	// Door on south wall of nave.
	doorX := localX + 3
	doorZ := localZ + 6
	if doorX >= 0 && doorX < 16 && doorZ >= 0 && doorZ < 16 {
		vp.setBlock(chunk, doorX, floorY+1, doorZ, vp.airID, gen)
		vp.setBlock(chunk, doorX, floorY+2, doorZ, vp.airID, gen)
	}

	// Torches inside the nave.
	for _, pos := range [][2]int{{1, 1}, {5, 1}, {1, 5}, {5, 5}} {
		tX := localX + pos[0]
		tZ := localZ + pos[1]
		if tX >= 0 && tX < 16 && tZ >= 0 && tZ < 16 {
			vp.setBlock(chunk, tX, floorY+1, tZ, vp.torchID, gen)
		}
	}

	// Torch at top of tower.
	topTorchX := towerBaseX + 1
	topTorchZ := towerBaseZ + 1
	if topTorchX >= 0 && topTorchX < 16 && topTorchZ >= 0 && topTorchZ < 16 {
		vp.setBlock(chunk, topTorchX, floorY+towerHeight, topTorchZ, vp.torchID, gen)
	}
}

// PlaceVillageRoad places dirt_path blocks along a line between two points on the ground.
func (vp *VillagePlacer) PlaceVillageRoad(chunk *level.Chunk, x1, z1, x2, z2, baseY int, gen *TerrainGenerator) {
	// Bresenham-like line drawing for the path.
	dx := x2 - x1
	dz := z2 - z1
	if dx < 0 {
		dx = -dx
	}
	if dz < 0 {
		dz = -dz
	}

	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sz := 1
	if z1 > z2 {
		sz = -1
	}

	err := dx - dz
	cx, cz := x1, z1

	for {
		if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
			vp.setBlock(chunk, cx, baseY, cz, vp.dirtPathID, gen)
		}

		if cx == x2 && cz == z2 {
			break
		}

		e2 := 2 * err
		if e2 > -dz {
			err -= dz
			cx += sx
		}
		if e2 < dx {
			err += dx
			cz += sz
		}
	}
}

// PlaceVillageFarm places a 5x5 farm plot with farmland, water in center, and wheat crops.
func (vp *VillagePlacer) PlaceVillageFarm(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	for dz := 0; dz < 5; dz++ {
		for dx := 0; dx < 5; dx++ {
			bx := localX + dx
			bz := localZ + dz
			if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
				continue
			}

			isCenter := dx == 2 && dz == 2

			if isCenter {
				// Water in center.
				vp.setBlock(chunk, bx, baseY, bz, vp.waterID, gen)
			} else {
				// Farmland.
				vp.setBlock(chunk, bx, baseY, bz, vp.farmlandID, gen)
				// Wheat on top.
				vp.setBlock(chunk, bx, baseY+1, bz, vp.wheatID, gen)
			}
		}
	}

	// Fence around the farm.
	for dx := -1; dx <= 5; dx++ {
		for _, dz := range []int{-1, 5} {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				vp.setBlock(chunk, bx, baseY+1, bz, vp.oakFenceID, gen)
			}
		}
	}
	for dz := 0; dz < 5; dz++ {
		for _, dx := range []int{-1, 5} {
			bx := localX + dx
			bz := localZ + dz
			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				vp.setBlock(chunk, bx, baseY+1, bz, vp.oakFenceID, gen)
			}
		}
	}
}

// setBlock sets a block in the chunk at local coordinates (x 0-15, z 0-15, worldY).
func (vp *VillagePlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
