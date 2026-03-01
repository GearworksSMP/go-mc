package gen

import (
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
)

// VoidGenerator generates empty chunks (all air).
type VoidGenerator struct {
	Sections int // number of sections (24 for overworld)
}

// Generate creates an empty chunk at the given position.
func (v *VoidGenerator) Generate(pos game.ChunkPos) *level.Chunk {
	return level.EmptyChunk(v.Sections)
}
