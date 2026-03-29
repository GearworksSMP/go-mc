package gen

import (
	"math"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// EndCityPlacer generates end city structures on outer End islands.
type EndCityPlacer struct {
	Seed int64

	purpurBlockID  level.BlocksState
	purpurPillarID level.BlocksState
	purpurSlabID   level.BlocksState
	endRodID       level.BlocksState
	chestID        level.BlocksState

	hasEndRod bool
}

// NewEndCityPlacer creates an EndCityPlacer with resolved block state IDs.
func NewEndCityPlacer(seed int64) *EndCityPlacer {
	p := &EndCityPlacer{Seed: seed}

	p.purpurBlockID, _ = block.ToStateID[block.PurpurBlock{}]
	p.purpurPillarID, _ = block.ToStateID[block.PurpurPillar{Axis: block.Y}]
	p.purpurSlabID, _ = block.ToStateID[block.PurpurSlab{Type: block.SlabTypeBottom}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle}]

	var ok bool
	p.endRodID, ok = block.ToStateID[block.EndRod{Facing: block.Up}]
	p.hasEndRod = ok

	return p
}

// PlaceEndCity attempts to place an end city in the given chunk if conditions are met.
func (p *EndCityPlacer) PlaceEndCity(chunk *level.Chunk, pos game.ChunkPos, g *EndGenerator) {
	centerX := pos.X*16 + 8
	centerZ := pos.Z*16 + 8

	distSq := float64(centerX*centerX + centerZ*centerZ)
	if distSq < 1000*1000 {
		return
	}

	h := structureHash(pos.X, pos.Z, p.Seed, 0xEC17)
	if abs64(h)%100 != 0 {
		return
	}

	baseY := g.findEndStoneSurface(chunk, 8, 8)
	if baseY < 0 {
		return
	}

	heightHash := abs64(structureHash(pos.X, pos.Z, p.Seed, 0xEC18))
	towerHeight := 10 + int(heightHash%6)

	p.placeTower(chunk, 4, baseY, 4, towerHeight, g)

	secondHash := abs64(structureHash(pos.X, pos.Z, p.Seed, 0xEC19))
	if secondHash%2 == 0 {
		height2 := 10 + int(abs64(structureHash(pos.X, pos.Z, p.Seed, 0xEC1A))%6)
		p.placeTower(chunk, 10, baseY, 10, height2, g)
		p.placeBridge(chunk, 4, baseY+towerHeight/2, 4, 10, baseY+height2/2, 10, g)
	}

	shipHash := abs64(structureHash(pos.X, pos.Z, p.Seed, 0xEC1B))
	if shipHash%2 == 0 {
		shipY := baseY + towerHeight + 5
		p.placeEndShip(chunk, 2, shipY, 5, g)
	}
}

// placeTower places a 5x5 purpur tower with interior slab floors and end rod lighting.
func (p *EndCityPlacer) placeTower(chunk *level.Chunk, lx, baseY, lz, height int, g *EndGenerator) {
	floors := 3
	floorSpacing := height / floors

	for dy := 0; dy <= height; dy++ {
		for dz := 0; dz < 5; dz++ {
			for dx := 0; dx < 5; dx++ {
				bx := lx + dx
				bz := lz + dz
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				by := baseY + dy
				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 4
				isFloorLevel := dy == 0 || dy == height
				isInteriorFloor := false
				for f := 1; f < floors; f++ {
					if dy == f*floorSpacing {
						isInteriorFloor = true
						break
					}
				}

				if isFloorLevel {
					g.setBlock(chunk, bx, by, bz, p.purpurBlockID)
				} else if isInteriorFloor {
					if isWall {
						g.setBlock(chunk, bx, by, bz, p.purpurBlockID)
					} else {
						g.setBlock(chunk, bx, by, bz, p.purpurSlabID)
					}
				} else if isWall {
					// Leave a door opening on one side.
					if dz == 4 && dx == 2 && dy >= 1 && dy <= 2 {
						continue
					}
					g.setBlock(chunk, bx, by, bz, p.purpurBlockID)
				}
			}
		}
	}

	if p.hasEndRod {
		rodX := lx + 2
		rodZ := lz + 2
		if rodX >= 0 && rodX < 16 && rodZ >= 0 && rodZ < 16 {
			for f := 0; f <= floors; f++ {
				rodY := baseY + f*floorSpacing + 1
				if f == floors {
					rodY = baseY + height + 1
				}
				g.setBlock(chunk, rodX, rodY, rodZ, p.endRodID)
			}
		}
	}
}

// placeBridge places a 3-wide purpur bridge between two tower positions.
func (p *EndCityPlacer) placeBridge(chunk *level.Chunk, x1, y1, z1, x2, y2, z2 int, g *EndGenerator) {
	sx := x1 + 2
	sz := z1 + 2
	ex := x2 + 2
	ez := z2 + 2
	bridgeY := (y1 + y2) / 2

	steps := max(absInt(ex-sx), absInt(ez-sz))
	if steps == 0 {
		return
	}

	widenAlongZ := absInt(ex-sx) >= absInt(ez-sz)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := sx + int(math.Round(t*float64(ex-sx)))
		cz := sz + int(math.Round(t*float64(ez-sz)))

		for w := -1; w <= 1; w++ {
			var bx, bz int
			if widenAlongZ {
				bx = cx
				bz = cz + w
			} else {
				bx = cx + w
				bz = cz
			}

			if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
				g.setBlock(chunk, bx, bridgeY, bz, p.purpurBlockID)
				if w != 0 {
					g.setBlock(chunk, bx, bridgeY+1, bz, p.purpurSlabID)
				}
			}
		}
	}
}

// placeEndShip places a simplified end ship (7x5x3 hull) with a loot chest.
func (p *EndCityPlacer) placeEndShip(chunk *level.Chunk, lx, baseY, lz int, g *EndGenerator) {
	const shipLen, shipW, shipH = 7, 5, 3

	for dy := 0; dy < shipH; dy++ {
		for dl := 0; dl < shipLen; dl++ {
			for dw := 0; dw < shipW; dw++ {
				bx := lx + dl
				bz := lz + dw
				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				isWall := dw == 0 || dw == shipW-1
				isBow := dl >= shipLen-2

				// Taper the bow section.
				if isBow {
					halfW := shipW / 2
					dist := dw - halfW
					if dist < 0 {
						dist = -dist
					}
					if dist > halfW-(dl-(shipLen-2)) {
						continue
					}
				}

				var stateID level.BlocksState
				switch {
				case dy == 0 || dy == shipH-1:
					stateID = p.purpurBlockID
				case isWall:
					stateID = p.purpurPillarID
				case dl <= 1 && dy == 1 && dw == shipW/2:
					stateID = p.purpurPillarID
				default:
					continue
				}

				g.setBlock(chunk, bx, baseY+dy, bz, stateID)
			}
		}
	}

	chestX := lx + 3
	chestZ := lz + 2
	if chestX >= 0 && chestX < 16 && chestZ >= 0 && chestZ < 16 {
		g.setBlock(chunk, chestX, baseY+1, chestZ, p.chestID)
	}
}

// absInt returns the absolute value of an integer.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
