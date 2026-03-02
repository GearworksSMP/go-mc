package gen

import (
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// NetherStructurePlacer generates nether fortress and bastion remnant structures.
type NetherStructurePlacer struct {
	Seed int64

	// Block state IDs
	netherBricksID       level.BlocksState
	netherBrickFenceID   level.BlocksState
	netherBrickStairsID  level.BlocksState
	soulSandID           level.BlocksState
	netherWartID         level.BlocksState
	blazeSpawnerID       level.BlocksState
	chestID              level.BlocksState
	lavaID               level.BlocksState
	airID                level.BlocksState
	blackstoneID         level.BlocksState
	polishedBlackstoneID level.BlocksState
	gildedBlackstoneID   level.BlocksState
	blackstoneBricksID   level.BlocksState
	goldBlockID          level.BlocksState
	magmaBlockID         level.BlocksState
	lanternID            level.BlocksState
	chainID              level.BlocksState

	hasBlackstoneBricks bool
	hasGilded           bool
	hasLantern          bool
	hasChain            bool
}

// NewNetherStructurePlacer creates a new nether structure placer with resolved block IDs.
func NewNetherStructurePlacer(seed int64) *NetherStructurePlacer {
	p := &NetherStructurePlacer{Seed: seed}

	p.netherBricksID, _ = block.ToStateID[block.NetherBricks{}]
	p.netherBrickFenceID, _ = block.ToStateID[block.NetherBrickFence{
		North: false, South: false, East: false, West: false, Waterlogged: false,
	}]
	p.netherBrickStairsID, _ = block.ToStateID[block.NetherBrickStairs{
		Facing: block.North, Half: block.Bottom, Shape: block.StairsShapeStraight, Waterlogged: false,
	}]
	p.soulSandID, _ = block.ToStateID[block.SoulSand{}]
	p.netherWartID, _ = block.ToStateID[block.NetherWart{Age: 0}]
	p.chestID, _ = block.ToStateID[block.Chest{Facing: block.North, Type: block.ChestTypeSingle, Waterlogged: false}]
	p.lavaID, _ = block.ToStateID[block.Lava{Level: 0}]
	p.airID = 0

	// Spawner for blazes
	p.blazeSpawnerID, _ = block.ToStateID[block.Spawner{}]

	// Bastion blocks
	p.blackstoneID, _ = block.ToStateID[block.Blackstone{}]
	p.polishedBlackstoneID, _ = block.ToStateID[block.PolishedBlackstone{}]
	p.goldBlockID, _ = block.ToStateID[block.GoldBlock{}]
	p.magmaBlockID, _ = block.ToStateID[block.MagmaBlock{}]

	var ok bool
	p.blackstoneBricksID, ok = block.ToStateID[block.PolishedBlackstoneBricks{}]
	p.hasBlackstoneBricks = ok

	p.gildedBlackstoneID, ok = block.ToStateID[block.GildedBlackstone{}]
	p.hasGilded = ok

	p.lanternID, ok = block.ToStateID[block.Lantern{Hanging: true, Waterlogged: false}]
	p.hasLantern = ok

	p.chainID, ok = block.ToStateID[block.Chain{Axis: block.Y, Waterlogged: false}]
	p.hasChain = ok

	return p
}

// PlaceNetherStructures is called after nether terrain generation.
func (p *NetherStructurePlacer) PlaceNetherStructures(chunk *level.Chunk, chunkX, chunkZ int) {
	// Nether fortress: 1.5% chance per chunk, spread apart
	fHash := structureHash(chunkX, chunkZ, p.Seed, 0xFE55)
	// Only place fortress at grid-aligned positions (every 8 chunks) for spacing
	if chunkX%8 == 0 && chunkZ%8 == 0 && abs64(fHash)%67 < 1 {
		p.placeFortressCorridor(chunk, chunkX, chunkZ)
	}

	// Bastion remnant: 1% chance per chunk
	bHash := structureHash(chunkX, chunkZ, p.Seed, 0xBA51)
	if chunkX%6 == 3 && chunkZ%6 == 3 && abs64(bHash)%100 < 1 {
		p.placeBastionRemnant(chunk, chunkX, chunkZ)
	}
}

// setNetherBlock sets a block in the nether chunk (minY=0, 8 sections).
func (p *NetherStructurePlacer) setNetherBlock(chunk *level.Chunk, x, y, z int, state level.BlocksState) {
	if x < 0 || x >= 16 || z < 0 || z >= 16 || y < 0 || y >= 128 {
		return
	}
	secIdx := y / 16
	if secIdx >= len(chunk.Sections) {
		return
	}
	localY := y % 16
	idx := localY*16*16 + z*16 + x
	chunk.Sections[secIdx].SetBlock(idx, state)
}

// placeFortressCorridor places a nether fortress section: corridors, blaze spawner, nether wart room.
func (p *NetherStructurePlacer) placeFortressCorridor(chunk *level.Chunk, chunkX, chunkZ int) {
	baseY := 48 + int(abs64(structureHash(chunkX, chunkZ, p.Seed, 0xFE60))%16) // Y 48-63

	// Main corridor: 3 wide, 4 tall, runs the full 16 blocks along X
	for lz := 6; lz <= 9; lz++ {
		for lx := 0; lx < 16; lx++ {
			for dy := 0; dy <= 4; dy++ {
				y := baseY + dy
				isFloor := dy == 0
				isCeiling := dy == 4
				isWall := lz == 6 || lz == 9

				if isFloor || isCeiling {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else if isWall {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else {
					p.setNetherBlock(chunk, lx, y, lz, p.airID)
				}
			}
		}
	}

	// Fence pillars on top of the corridor walls every 4 blocks
	for lx := 0; lx < 16; lx += 4 {
		for _, lz := range []int{6, 9} {
			p.setNetherBlock(chunk, lx, baseY+5, lz, p.netherBrickFenceID)
		}
	}

	// Cross corridor: runs along Z at x=8
	for lz := 0; lz < 16; lz++ {
		for lx := 7; lx <= 9; lx++ {
			for dy := 0; dy <= 4; dy++ {
				y := baseY + dy
				isFloor := dy == 0
				isCeiling := dy == 4
				isWall := lx == 7 || lx == 9

				// Don't overwrite the main corridor interior
				if lz >= 6 && lz <= 9 {
					continue
				}

				if isFloor || isCeiling {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else if isWall {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else {
					p.setNetherBlock(chunk, lx, y, lz, p.airID)
				}
			}
		}
	}

	// Blaze spawner room (7x7) at one corner
	spawnHash := structureHash(chunkX, chunkZ, p.Seed, 0xFE61)
	if abs64(spawnHash)%2 == 0 {
		p.placeBlazeSpawnerRoom(chunk, 0, baseY, 0)
	} else {
		p.placeBlazeSpawnerRoom(chunk, 9, baseY, 0)
	}

	// Nether wart room at the other end
	wartHash := structureHash(chunkX, chunkZ, p.Seed, 0xFE62)
	if abs64(wartHash)%2 == 0 {
		p.placeNetherWartRoom(chunk, 0, baseY, 10)
	} else {
		p.placeNetherWartRoom(chunk, 9, baseY, 10)
	}

	// Staircase section: connects to a lower corridor level
	stairY := baseY - 5
	if stairY > 10 {
		for dy := 0; dy < 6; dy++ {
			sx := 3 + dy
			if sx < 16 {
				p.setNetherBlock(chunk, sx, stairY+dy, 7, p.netherBrickStairsID)
				p.setNetherBlock(chunk, sx, stairY+dy, 8, p.netherBrickStairsID)
				// Walls on sides
				p.setNetherBlock(chunk, sx, stairY+dy, 6, p.netherBricksID)
				p.setNetherBlock(chunk, sx, stairY+dy, 9, p.netherBricksID)
				// Clear above stairs
				for clearY := 1; clearY <= 3; clearY++ {
					p.setNetherBlock(chunk, sx, stairY+dy+clearY, 7, p.airID)
					p.setNetherBlock(chunk, sx, stairY+dy+clearY, 8, p.airID)
				}
			}
		}
	}

	// Lava well in center of cross corridor intersection
	p.setNetherBlock(chunk, 8, baseY, 7, p.lavaID)
	p.setNetherBlock(chunk, 8, baseY, 8, p.lavaID)
}

// placeBlazeSpawnerRoom places a 6x6x5 room with a blaze spawner in the center.
func (p *NetherStructurePlacer) placeBlazeSpawnerRoom(chunk *level.Chunk, startX, baseY, startZ int) {
	for dz := 0; dz < 6; dz++ {
		for dx := 0; dx < 6; dx++ {
			for dy := 0; dy <= 5; dy++ {
				lx := startX + dx
				lz := startZ + dz
				y := baseY + dy

				if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
					continue
				}

				isFloor := dy == 0
				isCeiling := dy == 5
				isWall := dx == 0 || dx == 5 || dz == 0 || dz == 5

				if isFloor || isCeiling {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else if isWall {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else {
					p.setNetherBlock(chunk, lx, y, lz, p.airID)
				}
			}
		}
	}

	// Place blaze spawner on a 1-block pedestal in center
	cx, cz := startX+3, startZ+3
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setNetherBlock(chunk, cx, baseY+1, cz, p.netherBricksID)
		p.setNetherBlock(chunk, cx, baseY+2, cz, p.blazeSpawnerID)
	}

	// Fence railing around the edge of the room at floor+1
	for dx := 1; dx < 5; dx++ {
		for _, dz := range []int{1, 4} {
			lx := startX + dx
			lz := startZ + dz
			if lx >= 0 && lx < 16 && lz >= 0 && lz < 16 {
				p.setNetherBlock(chunk, lx, baseY+1, lz, p.netherBrickFenceID)
			}
		}
	}
	for dz := 2; dz < 4; dz++ {
		for _, dx := range []int{1, 4} {
			lx := startX + dx
			lz := startZ + dz
			if lx >= 0 && lx < 16 && lz >= 0 && lz < 16 {
				p.setNetherBlock(chunk, lx, baseY+1, lz, p.netherBrickFenceID)
			}
		}
	}
}

// placeNetherWartRoom places a 5x5x4 room with soul sand and nether wart.
func (p *NetherStructurePlacer) placeNetherWartRoom(chunk *level.Chunk, startX, baseY, startZ int) {
	for dz := 0; dz < 5; dz++ {
		for dx := 0; dx < 5; dx++ {
			for dy := 0; dy <= 4; dy++ {
				lx := startX + dx
				lz := startZ + dz
				y := baseY + dy

				if lx < 0 || lx >= 16 || lz < 0 || lz >= 16 {
					continue
				}

				isFloor := dy == 0
				isCeiling := dy == 4
				isWall := dx == 0 || dx == 4 || dz == 0 || dz == 4

				if isFloor || isCeiling {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else if isWall {
					p.setNetherBlock(chunk, lx, y, lz, p.netherBricksID)
				} else {
					p.setNetherBlock(chunk, lx, y, lz, p.airID)
				}
			}
		}
	}

	// Soul sand floor with nether wart growing on it (interior 3x3)
	for dz := 1; dz <= 3; dz++ {
		for dx := 1; dx <= 3; dx++ {
			lx := startX + dx
			lz := startZ + dz
			if lx >= 0 && lx < 16 && lz >= 0 && lz < 16 {
				p.setNetherBlock(chunk, lx, baseY+1, lz, p.soulSandID)
				p.setNetherBlock(chunk, lx, baseY+2, lz, p.netherWartID)
			}
		}
	}

	// Chest with loot in corner
	cx, cz := startX+1, startZ+1
	if cx >= 0 && cx < 16 && cz >= 0 && cz < 16 {
		p.setNetherBlock(chunk, cx, baseY+1, cz, p.chestID)
	}
}

// placeBastionRemnant places a bastion remnant structure.
func (p *NetherStructurePlacer) placeBastionRemnant(chunk *level.Chunk, chunkX, chunkZ int) {
	baseY := 40 + int(abs64(structureHash(chunkX, chunkZ, p.Seed, 0xBA52))%20)

	wallBlock := p.blackstoneID
	if p.hasBlackstoneBricks {
		wallBlock = p.blackstoneBricksID
	}
	floorBlock := p.polishedBlackstoneID
	if floorBlock == 0 {
		floorBlock = p.blackstoneID
	}
	accentBlock := p.gildedBlackstoneID
	if !p.hasGilded {
		accentBlock = p.goldBlockID
	}

	// Main treasure room: 10x10x7
	for dz := 3; dz < 13; dz++ {
		for dx := 3; dx < 13; dx++ {
			for dy := 0; dy <= 7; dy++ {
				if dx >= 16 || dz >= 16 {
					continue
				}
				y := baseY + dy
				isFloor := dy == 0
				isCeiling := dy == 7
				isWall := dx == 3 || dx == 12 || dz == 3 || dz == 12

				if isFloor {
					p.setNetherBlock(chunk, dx, y, dz, floorBlock)
				} else if isCeiling {
					p.setNetherBlock(chunk, dx, y, dz, wallBlock)
				} else if isWall {
					// Alternate gilded blackstone into walls
					h := posHash(dx, dy, dz, p.Seed+0xBA53)
					if h%5 == 0 {
						p.setNetherBlock(chunk, dx, y, dz, accentBlock)
					} else {
						p.setNetherBlock(chunk, dx, y, dz, wallBlock)
					}
				} else {
					p.setNetherBlock(chunk, dx, y, dz, p.airID)
				}
			}
		}
	}

	// Entrance opening (south wall center, 3 wide x 3 tall)
	for dx := 7; dx <= 9; dx++ {
		for dy := 1; dy <= 3; dy++ {
			p.setNetherBlock(chunk, dx, baseY+dy, 12, p.airID)
		}
	}

	// Gold block treasure pile in center (3x2x3 mound)
	for dz := 7; dz <= 9; dz++ {
		for dx := 7; dx <= 9; dx++ {
			p.setNetherBlock(chunk, dx, baseY+1, dz, p.goldBlockID)
		}
	}
	// Peak
	p.setNetherBlock(chunk, 8, baseY+2, 8, p.goldBlockID)

	// Magma block hazards around the treasure
	hazards := [][2]int{{6, 7}, {6, 9}, {10, 7}, {10, 9}, {7, 6}, {9, 6}, {7, 10}, {9, 10}}
	for _, h := range hazards {
		if h[0] < 16 && h[1] < 16 {
			p.setNetherBlock(chunk, h[0], baseY+1, h[1], p.magmaBlockID)
		}
	}

	// Chest with loot
	p.setNetherBlock(chunk, 5, baseY+1, 5, p.chestID)
	p.setNetherBlock(chunk, 10, baseY+1, 5, p.chestID)

	// Hanging lanterns (if available)
	if p.hasLantern && p.hasChain {
		lanternPos := [][2]int{{5, 5}, {5, 10}, {10, 5}, {10, 10}}
		for _, lp := range lanternPos {
			if lp[0] < 16 && lp[1] < 16 {
				p.setNetherBlock(chunk, lp[0], baseY+6, lp[1], p.chainID)
				p.setNetherBlock(chunk, lp[0], baseY+5, lp[1], p.lanternID)
			}
		}
	}

	// Bridge walkway extending from entrance
	for dz := 13; dz < 16; dz++ {
		for dx := 7; dx <= 9; dx++ {
			p.setNetherBlock(chunk, dx, baseY, dz, wallBlock)
			// Clear above bridge
			for dy := 1; dy <= 3; dy++ {
				p.setNetherBlock(chunk, dx, baseY+dy, dz, p.airID)
			}
		}
		// Fence railings
		if dz < 16 {
			p.setNetherBlock(chunk, 6, baseY+1, dz, p.netherBrickFenceID)
			if 10 < 16 {
				p.setNetherBlock(chunk, 10, baseY+1, dz, p.netherBrickFenceID)
			}
		}
	}

	// Piglin spawner area (small room off the main chamber)
	for dz := 3; dz <= 5; dz++ {
		for dx := 0; dx <= 2; dx++ {
			for dy := 0; dy <= 4; dy++ {
				y := baseY + dy
				isFloor := dy == 0
				isCeiling := dy == 4
				isWall := dx == 0 || dz == 3

				if isFloor || isCeiling {
					p.setNetherBlock(chunk, dx, y, dz, wallBlock)
				} else if isWall {
					p.setNetherBlock(chunk, dx, y, dz, wallBlock)
				} else {
					p.setNetherBlock(chunk, dx, y, dz, p.airID)
				}
			}
		}
	}
}
