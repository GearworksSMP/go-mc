package mem

import (
	"testing"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/level/block"
)

func TestWorldLoadChunkGenerates(t *testing.T) {
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(sfGen, 24, -64)

	pos := game.ChunkPos{X: 5, Z: 10}
	chunk, err := w.LoadChunk(pos)
	if err != nil {
		t.Fatal(err)
	}
	if chunk == nil {
		t.Fatal("LoadChunk returned nil")
	}

	// Verify superflat blocks at bottom
	bedrockID, _ := block.ToStateID[block.Bedrock{}]
	b := chunk.Sections[0].GetBlock(0) // Y=-64, localY=0, x=0, z=0
	if b != bedrockID {
		t.Errorf("Block at Y=-64 = %d, want bedrock (%d)", b, bedrockID)
	}
}

func TestWorldLoadChunkCaches(t *testing.T) {
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(sfGen, 24, -64)

	pos := game.ChunkPos{X: 0, Z: 0}
	c1, _ := w.LoadChunk(pos)
	c2, _ := w.LoadChunk(pos)

	if c1 != c2 {
		t.Error("LoadChunk should return the same chunk instance on second call")
	}
}

func TestWorldGetSetBlock(t *testing.T) {
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(sfGen, 24, -64)

	// Load the chunk first
	w.LoadChunk(game.ChunkPos{X: 0, Z: 0})

	stoneID, _ := block.ToStateID[block.Stone{}]

	// Set a block at Y=0 (section 4, localY=0)
	old, err := w.SetBlock(5, 0, 5, stoneID)
	if err != nil {
		t.Fatal(err)
	}
	// Y=0 is above the superflat layers (bedrock at -64, dirt -63..-62, grass -61),
	// so the old value should be air (0)
	if old != 0 {
		t.Errorf("Old block at Y=0 = %d, want 0 (air)", old)
	}

	// Read it back
	got, err := w.GetBlock(5, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if got != stoneID {
		t.Errorf("GetBlock = %d, want stone (%d)", got, stoneID)
	}
}

func TestWorldGetBlockUnloaded(t *testing.T) {
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(sfGen, 24, -64)

	// GetBlock on unloaded chunk should return error
	_, err := w.GetBlock(100, 0, 100)
	if err == nil {
		t.Error("GetBlock on unloaded chunk should return error")
	}
}
