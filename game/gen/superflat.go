// Package gen provides chunk generators for different world types.
package gen

import (
	"math/bits"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// Layer defines a horizontal layer of blocks in a superflat world.
type Layer struct {
	Block  level.BlocksState // block state ID
	Height int               // number of Y levels
}

// SuperflatGenerator generates flat chunks with configurable layers.
type SuperflatGenerator struct {
	Sections int     // number of sections (24 for overworld)
	MinY     int     // minimum Y coordinate (-64 for overworld)
	Layers   []Layer // bottom to top, starting at MinY
}

// DefaultSuperflat returns a superflat generator with bedrock(1) + dirt(2) + grass_block(1),
// matching the vanilla superflat preset. Uses 24 sections for overworld.
func DefaultSuperflat() *SuperflatGenerator {
	bedrockID, _ := block.ToStateID[block.Bedrock{}]
	dirtID, _ := block.ToStateID[block.Dirt{}]
	grassID, _ := block.ToStateID[block.GrassBlock{Snowy: false}]

	return &SuperflatGenerator{
		Sections: 24,
		MinY:     -64,
		Layers: []Layer{
			{Block: bedrockID, Height: 1},
			{Block: dirtID, Height: 2},
			{Block: grassID, Height: 1},
		},
	}
}

// Generate creates a superflat chunk at the given position.
func (g *SuperflatGenerator) Generate(pos game.ChunkPos) *level.Chunk {
	chunk := level.EmptyChunk(g.Sections)

	// Fill blocks layer by layer starting from MinY
	y := g.MinY
	for _, layer := range g.Layers {
		for dy := 0; dy < layer.Height; dy++ {
			g.fillYLevel(chunk, y, layer.Block)
			y++
		}
	}

	// Compute heightmaps
	g.computeHeightmaps(chunk)

	return chunk
}

// fillYLevel fills an entire Y level with the given block state.
func (g *SuperflatGenerator) fillYLevel(chunk *level.Chunk, worldY int, state level.BlocksState) {
	secIdx := (worldY - g.MinY) / 16
	if secIdx < 0 || secIdx >= g.Sections {
		return
	}
	localY := (worldY - g.MinY) % 16

	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			idx := localY*16*16 + z*16 + x
			chunk.Sections[secIdx].SetBlock(idx, state)
		}
	}
}

// computeHeightmaps recalculates all heightmaps for the chunk.
func (g *SuperflatGenerator) computeHeightmaps(chunk *level.Chunk) {
	bitsForHeight := bits.Len(uint(g.Sections)*16 + 1)
	chunk.HeightMaps.WorldSurface = level.NewBitStorage(bitsForHeight, 16*16, nil)
	chunk.HeightMaps.MotionBlocking = level.NewBitStorage(bitsForHeight, 16*16, nil)
	chunk.HeightMaps.MotionBlockingNoLeaves = level.NewBitStorage(bitsForHeight, 16*16, nil)

	// Scan from top down to find highest non-air block
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			hmIdx := z*16 + x
			for secIdx := g.Sections - 1; secIdx >= 0; secIdx-- {
				for localY := 15; localY >= 0; localY-- {
					idx := localY*16*16 + z*16 + x
					state := chunk.Sections[secIdx].GetBlock(idx)
					if !block.IsAir(state) {
						// Heightmap value = (worldY - minY) + 1
						absY := secIdx*16 + localY + 1
						chunk.HeightMaps.WorldSurface.Set(hmIdx, absY)
						chunk.HeightMaps.MotionBlocking.Set(hmIdx, absY)
						chunk.HeightMaps.MotionBlockingNoLeaves.Set(hmIdx, absY)
						goto nextColumn
					}
				}
			}
		nextColumn:
		}
	}
}

// TopBlockY returns the world Y coordinate of the highest block in the superflat preset.
func (g *SuperflatGenerator) TopBlockY() int {
	total := 0
	for _, layer := range g.Layers {
		total += layer.Height
	}
	return g.MinY + total - 1
}

// SpawnY returns the Y coordinate a player should spawn at (above the top block).
func (g *SuperflatGenerator) SpawnY() float64 {
	return float64(g.TopBlockY()) + 1
}
