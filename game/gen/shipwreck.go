package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// ShipwreckPlacer generates shipwreck structures in ocean biomes.
type ShipwreckPlacer struct {
	Seed int64

	oakPlanksID    level.BlocksState
	oakLogID       level.BlocksState
	sprucePlanksID level.BlocksState
	chestID        level.BlocksState
	airID          level.BlocksState
}

// NewShipwreckPlacer creates a ShipwreckPlacer with resolved block state IDs.
func NewShipwreckPlacer(seed int64) *ShipwreckPlacer {
	p := &ShipwreckPlacer{Seed: seed, airID: 0}

	p.oakPlanksID, _ = block.ToStateID[block.OakPlanks{}]
	p.oakLogID, _ = block.ToStateID[block.OakLog{Axis: block.Z}]
	p.sprucePlanksID, _ = block.ToStateID[block.SprucePlanks{}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]

	return p
}

// PlaceShipwreck places a shipwreck structure.
// localX, localZ are the NW corner in chunk-local coords. baseY is the ocean floor level.
func (p *ShipwreckPlacer) PlaceShipwreck(chunk *level.Chunk, localX, baseY, localZ int, gen *TerrainGenerator) {
	variantHash := abs64(structureHash(localX, localZ, p.Seed, 0x5B1C))
	variant := int(variantHash % 3) // 0=full, 1=stern, 2=bow

	rotHash := abs64(structureHash(localX, localZ, p.Seed, 0x5B1D))
	rot := int(rotHash % 4) // 0=north, 1=east, 2=south, 3=west

	var shipLen, shipW, shipH int
	switch variant {
	case 0: // full ship
		shipLen, shipW, shipH = 15, 5, 7
	case 1: // half-buried stern
		shipLen, shipW, shipH = 10, 5, 7
	default: // upright bow
		shipLen, shipW, shipH = 8, 5, 7
	}

	// Place the hull.
	for dy := 0; dy < shipH; dy++ {
		for dl := 0; dl < shipLen; dl++ {
			for dw := 0; dw < shipW; dw++ {
				// Determine local block position based on rotation.
				dx, dz := p.rotate(dl, dw, rot, shipLen, shipW)
				bx := localX + dx
				bz := localZ + dz
				by := baseY + dy

				if bx < 0 || bx >= 16 || bz < 0 || bz >= 16 {
					continue
				}

				// Hull shape: V-shaped bottom, straight walls above.
				isFloor := dy == 0
				isWall := dw == 0 || dw == shipW-1
				isDeck := dy == 3
				isRail := dy == 4 && isWall
				isBow := dl >= shipLen-3
				isStern := dl <= 2

				// Taper the hull bottom into a V shape.
				if dy < 2 {
					halfW := shipW / 2
					dist := dw - halfW
					if dist < 0 {
						dist = -dist
					}
					if dist > dy+1 {
						continue
					}
				}

				// Narrow the bow.
				if isBow && dy < 4 {
					halfW := shipW / 2
					dist := dw - halfW
					if dist < 0 {
						dist = -dist
					}
					narrowing := dl - (shipLen - 3)
					if dist > halfW-narrowing {
						continue
					}
				}

				// Skip above deck except for rails and mast area.
				if dy > 4 {
					// Mast at center of the ship.
					midL := shipLen / 2
					midW := shipW / 2
					if dl == midL && dw == midW {
						p.setBlock(chunk, bx, by, bz, p.oakLogID, gen)
					}
					continue
				}

				var stateID level.BlocksState
				switch {
				case isFloor:
					stateID = p.oakPlanksID
				case isDeck:
					stateID = p.sprucePlanksID
				case isRail:
					stateID = p.oakLogID
				case isWall && dy < 4:
					stateID = p.oakPlanksID
				case isStern && dy == 1 && dw == shipW/2:
					// Stern decoration.
					stateID = p.oakLogID
				default:
					continue
				}

				p.setBlock(chunk, bx, by, bz, stateID, gen)
			}
		}
	}

	// Place 1-3 loot chests inside the hull.
	chestHash := abs64(structureHash(localX, localZ, p.Seed, 0x5B1E))
	chestCount := 1 + int(chestHash%3)

	chestPositions := [][2]int{{2, shipW / 2}, {shipLen / 2, 1}, {shipLen - 4, shipW - 2}}
	for i := 0; i < chestCount && i < len(chestPositions); i++ {
		dl := chestPositions[i][0]
		dw := chestPositions[i][1]
		dx, dz := p.rotate(dl, dw, rot, shipLen, shipW)
		bx := localX + dx
		bz := localZ + dz
		if bx >= 0 && bx < 16 && bz >= 0 && bz < 16 {
			p.setBlock(chunk, bx, baseY+1, bz, p.chestID, gen)
		}
	}
}

// rotate converts length/width offsets into x/z based on rotation.
func (p *ShipwreckPlacer) rotate(dl, dw, rot, maxL, maxW int) (dx, dz int) {
	switch rot {
	case 0: // north: length=Z, width=X
		return dw, dl
	case 1: // east: length=X, width=Z
		return dl, dw
	case 2: // south: length=Z(reversed), width=X(reversed)
		return maxW - 1 - dw, maxL - 1 - dl
	default: // west: length=X(reversed), width=Z(reversed)
		return maxL - 1 - dl, maxW - 1 - dw
	}
}

// setBlock sets a block in the chunk at local coordinates.
func (p *ShipwreckPlacer) setBlock(chunk *level.Chunk, x, worldY, z int, state level.BlocksState, gen *TerrainGenerator) {
	secIdx := (worldY - gen.MinY) / 16
	if secIdx < 0 || secIdx >= gen.Sections {
		return
	}
	localY := (worldY - gen.MinY) % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}
