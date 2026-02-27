package dbworld

import (
	"context"
	"sync"
	"testing"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/level/block"
)

// mockChunkStore is an in-memory implementation of store.ChunkStore for testing.
type mockChunkStore struct {
	mu     sync.Mutex
	chunks map[string][]byte // key: "dim:x:z"
	saved  []store.ChunkData // track SaveChunks calls
}

func newMockStore() *mockChunkStore {
	return &mockChunkStore{chunks: make(map[string][]byte)}
}

func chunkKey(dim string, x, z int) string {
	return dim + ":" + string(rune(x)) + ":" + string(rune(z))
}

func (m *mockChunkStore) LoadChunk(_ context.Context, dim string, x, z int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chunks[chunkKey(dim, x, z)], nil
}

func (m *mockChunkStore) SaveChunks(_ context.Context, chunks []store.ChunkData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range chunks {
		m.chunks[chunkKey(c.Dimension, c.X, c.Z)] = c.Data
	}
	m.saved = append(m.saved, chunks...)
	return nil
}

func TestLoadChunkGenerates(t *testing.T) {
	ms := newMockStore()
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(ms, sfGen, sfGen.Sections, sfGen.MinY, "overworld", nil)

	pos := game.ChunkPos{X: 0, Z: 0}
	chunk, err := w.LoadChunk(pos)
	if err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}
	if chunk == nil {
		t.Fatal("LoadChunk returned nil chunk")
	}

	// Check that it's marked dirty
	w.mu.RLock()
	dirty := w.dirty[pos]
	w.mu.RUnlock()
	if !dirty {
		t.Error("newly generated chunk should be marked dirty")
	}
}

func TestFlushDirty(t *testing.T) {
	ms := newMockStore()
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(ms, sfGen, sfGen.Sections, sfGen.MinY, "overworld", nil)

	// Generate a chunk
	pos := game.ChunkPos{X: 3, Z: 5}
	if _, err := w.LoadChunk(pos); err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}

	// Flush
	n, err := w.FlushDirty(context.Background())
	if err != nil {
		t.Fatalf("FlushDirty: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 flushed chunk, got %d", n)
	}

	// Verify chunk was saved to the mock store
	ms.mu.Lock()
	if len(ms.saved) != 1 {
		t.Errorf("expected 1 saved chunk, got %d", len(ms.saved))
	}
	if ms.saved[0].X != 3 || ms.saved[0].Z != 5 {
		t.Errorf("saved chunk at wrong position: (%d,%d)", ms.saved[0].X, ms.saved[0].Z)
	}
	ms.mu.Unlock()

	// Second flush should be empty
	n, err = w.FlushDirty(context.Background())
	if err != nil {
		t.Fatalf("FlushDirty: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 flushed chunks after clean flush, got %d", n)
	}
}

func TestLoadChunkFromStore(t *testing.T) {
	ms := newMockStore()
	sfGen := gen.DefaultSuperflat()

	// First world: generate and flush
	w1 := NewWorld(ms, sfGen, sfGen.Sections, sfGen.MinY, "overworld", nil)
	pos := game.ChunkPos{X: 1, Z: 2}
	if _, err := w1.LoadChunk(pos); err != nil {
		t.Fatalf("LoadChunk w1: %v", err)
	}
	if _, err := w1.FlushDirty(context.Background()); err != nil {
		t.Fatalf("FlushDirty w1: %v", err)
	}

	// Second world: should load from store, not generate
	w2 := NewWorld(ms, sfGen, sfGen.Sections, sfGen.MinY, "overworld", nil)
	chunk, err := w2.LoadChunk(pos)
	if err != nil {
		t.Fatalf("LoadChunk w2: %v", err)
	}

	// Chunk should NOT be dirty (loaded from DB, not generated)
	w2.mu.RLock()
	dirty := w2.dirty[pos]
	w2.mu.RUnlock()
	if dirty {
		t.Error("chunk loaded from DB should not be dirty")
	}

	// Verify grass block at Y=-60 (top of superflat = minY + 3)
	grassID, _ := block.ToStateID[block.GrassBlock{Snowy: false}]
	secIdx := (-60 - sfGen.MinY) / 16 // section 0
	localY := (-60 - sfGen.MinY) % 16  // localY=4 but actually let's compute
	_ = localY
	// Top layer is at minY+3 = -61. Check section 0, localY=3 (minY=-64, so -61 is offset 3)
	topY := sfGen.MinY + 3 // -61: grass
	secIdx = (topY - sfGen.MinY) / 16
	ly := (topY - sfGen.MinY) % 16
	idx := ly*16*16 + 0*16 + 0
	state := chunk.Sections[secIdx].GetBlock(idx)
	if state != grassID {
		t.Errorf("expected grass block (state=%d) at top layer, got state=%d", grassID, state)
	}
}

func TestSetBlockMarksDirty(t *testing.T) {
	ms := newMockStore()
	sfGen := gen.DefaultSuperflat()
	w := NewWorld(ms, sfGen, sfGen.Sections, sfGen.MinY, "overworld", nil)

	// Load then flush to clear dirty
	if _, err := w.LoadChunk(game.ChunkPos{X: 0, Z: 0}); err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}
	if _, err := w.FlushDirty(context.Background()); err != nil {
		t.Fatalf("FlushDirty: %v", err)
	}

	// SetBlock should mark dirty
	stoneID, _ := block.ToStateID[block.Stone{}]
	if _, err := w.SetBlock(5, 10, 5, stoneID); err != nil {
		t.Fatalf("SetBlock: %v", err)
	}

	w.mu.RLock()
	dirty := w.dirty[game.ChunkPos{X: 0, Z: 0}]
	w.mu.RUnlock()
	if !dirty {
		t.Error("chunk should be dirty after SetBlock")
	}
}
