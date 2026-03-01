// Package game provides the core game server framework for Minecraft 26.1.
//
// It defines interfaces for world access, chunk generation, and tick processing,
// along with player management and 26.1-specific chunk encoding.
package game

import (
	"github.com/Tnze/go-mc/level"
)

// ChunkPos identifies a chunk by its X and Z coordinates.
type ChunkPos struct {
	X, Z int
}

// RegionPos identifies a region (32x32 chunks) by its X and Z coordinates.
type RegionPos struct {
	X, Z int
}

// ChunkToRegion returns the region containing the given chunk.
func ChunkToRegion(pos ChunkPos) RegionPos {
	// Arithmetic right shift for negative coordinates
	rx := pos.X >> 5
	rz := pos.Z >> 5
	return RegionPos{X: rx, Z: rz}
}

// World provides access to chunks and block state.
type World interface {
	// LoadChunk returns the chunk at the given position, generating it if needed.
	LoadChunk(pos ChunkPos) (*level.Chunk, error)

	// SetBlock sets a block at the given world coordinates and returns the previous state.
	SetBlock(x, y, z int, state level.BlocksState) (level.BlocksState, error)

	// GetBlock returns the block state at the given world coordinates.
	GetBlock(x, y, z int) (level.BlocksState, error)
}

// BlockToChunk converts world block coordinates to chunk coordinates.
// Uses floor division (not truncation) so negative coordinates work correctly.
func BlockToChunk(x, z int) ChunkPos {
	return ChunkPos{X: floorDiv(x, 16), Z: floorDiv(z, 16)}
}

// floorDiv returns the floor of a/b (rounds toward negative infinity).
func floorDiv(a, b int) int {
	d := a / b
	if (a^b) < 0 && d*b != a {
		d--
	}
	return d
}
