package game

import (
	"github.com/Tnze/go-mc/level"
)

// ChunkGenerator produces chunks for positions that don't exist yet.
type ChunkGenerator interface {
	Generate(pos ChunkPos) *level.Chunk
}
