// Package mem provides an in-memory World implementation backed by a map.
package mem

import (
	"fmt"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// World is an in-memory World implementation with a generator fallback.
type World struct {
	mu       sync.RWMutex
	chunks   map[game.ChunkPos]*level.Chunk
	gen      game.ChunkGenerator
	sections int // number of sections per chunk (24 for overworld)
	minY     int // minimum Y coordinate (-64 for overworld)
}

// NewWorld creates an in-memory world with the given generator.
// sections is the number of 16-block sections per chunk (24 for overworld).
// minY is the minimum Y coordinate (-64 for overworld).
func NewWorld(gen game.ChunkGenerator, sections, minY int) *World {
	return &World{
		chunks:   make(map[game.ChunkPos]*level.Chunk),
		gen:      gen,
		sections: sections,
		minY:     minY,
	}
}

// LoadChunk returns the chunk at the given position, generating it if needed.
func (w *World) LoadChunk(pos game.ChunkPos) (*level.Chunk, error) {
	w.mu.RLock()
	if c, ok := w.chunks[pos]; ok {
		w.mu.RUnlock()
		return c, nil
	}
	w.mu.RUnlock()

	// Generate the chunk
	w.mu.Lock()
	defer w.mu.Unlock()
	// Double-check after acquiring write lock
	if c, ok := w.chunks[pos]; ok {
		return c, nil
	}
	c := w.gen.Generate(pos)
	w.chunks[pos] = c
	return c, nil
}

// SetBlock sets a block at the given world coordinates.
func (w *World) SetBlock(x, y, z int, state level.BlocksState) (level.BlocksState, error) {
	pos := game.BlockToChunk(x, z)
	chunk, err := w.LoadChunk(pos)
	if err != nil {
		return 0, err
	}

	secIdx := (y - w.minY) / 16
	if secIdx < 0 || secIdx >= w.sections {
		return 0, fmt.Errorf("Y=%d out of range [%d, %d)", y, w.minY, w.minY+w.sections*16)
	}

	localX := ((x % 16) + 16) % 16
	localY := ((y - w.minY) % 16 + 16) % 16
	localZ := ((z % 16) + 16) % 16
	idx := localY*16*16 + localZ*16 + localX

	w.mu.Lock()
	defer w.mu.Unlock()
	old := chunk.Sections[secIdx].GetBlock(idx)
	chunk.Sections[secIdx].SetBlock(idx, state)

	// Update heightmaps
	w.updateHeightmaps(chunk, x&0xF, y, z&0xF, state)

	return old, nil
}

// GetBlock returns the block state at the given world coordinates.
func (w *World) GetBlock(x, y, z int) (level.BlocksState, error) {
	pos := game.BlockToChunk(x, z)

	w.mu.RLock()
	chunk, ok := w.chunks[pos]
	w.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("chunk (%d,%d) not loaded", pos.X, pos.Z)
	}

	secIdx := (y - w.minY) / 16
	if secIdx < 0 || secIdx >= w.sections {
		return 0, fmt.Errorf("Y=%d out of range [%d, %d)", y, w.minY, w.minY+w.sections*16)
	}

	localX := ((x % 16) + 16) % 16
	localY := ((y - w.minY) % 16 + 16) % 16
	localZ := ((z % 16) + 16) % 16
	idx := localY*16*16 + localZ*16 + localX

	return chunk.Sections[secIdx].GetBlock(idx), nil
}

// Sections returns the number of sections per chunk.
func (w *World) Sections() int { return w.sections }

// MinY returns the minimum Y coordinate.
func (w *World) MinY() int { return w.minY }

// updateHeightmaps updates heightmap entries for the given block change.
func (w *World) updateHeightmaps(chunk *level.Chunk, localX, worldY, localZ int, state level.BlocksState) {
	hmIdx := localZ*16 + localX
	absY := worldY - w.minY + 1 // heightmap stores (Y - minY + 1), 0 means no block

	// Update WorldSurface (not air) and MotionBlocking (not air, simplified)
	if !block.IsAir(state) {
		if chunk.HeightMaps.WorldSurface != nil {
			cur := chunk.HeightMaps.WorldSurface.Get(hmIdx)
			if absY > cur {
				chunk.HeightMaps.WorldSurface.Set(hmIdx, absY)
			}
		}
		if chunk.HeightMaps.MotionBlocking != nil {
			cur := chunk.HeightMaps.MotionBlocking.Get(hmIdx)
			if absY > cur {
				chunk.HeightMaps.MotionBlocking.Set(hmIdx, absY)
			}
		}
		if chunk.HeightMaps.MotionBlockingNoLeaves != nil {
			cur := chunk.HeightMaps.MotionBlockingNoLeaves.Get(hmIdx)
			if absY > cur {
				chunk.HeightMaps.MotionBlockingNoLeaves.Set(hmIdx, absY)
			}
		}
	}
}
