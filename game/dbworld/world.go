// Package dbworld provides a World implementation backed by a database store,
// with in-memory caching and dirty-chunk tracking for periodic flush.
package dbworld

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/save"
)

// World is a chunk cache backed by a ChunkStore with dirty tracking.
// Access path: in-memory map → database → generator.
type World struct {
	mu         sync.RWMutex
	chunks     map[game.ChunkPos]*level.Chunk
	dirty      map[game.ChunkPos]bool
	lastAccess map[game.ChunkPos]int64
	store      store.ChunkStore
	gen        game.ChunkGenerator
	sections   int
	minY       int
	dim        string // dimension key for store, e.g. "overworld"
	logger     *log.Logger

	// currentTick is set atomically from the main tick loop.
	currentTick atomic.Int64
}

// NewWorld creates a DB-backed world.
func NewWorld(cs store.ChunkStore, gen game.ChunkGenerator, sections, minY int, dimension string, logger *log.Logger) *World {
	return &World{
		chunks:     make(map[game.ChunkPos]*level.Chunk),
		dirty:      make(map[game.ChunkPos]bool),
		lastAccess: make(map[game.ChunkPos]int64),
		store:      cs,
		gen:        gen,
		sections:   sections,
		minY:       minY,
		dim:        dimension,
		logger:     logger,
	}
}

// SetTick updates the current tick counter (called from the main tick loop).
func (w *World) SetTick(tick int64) { w.currentTick.Store(tick) }

// LoadChunk returns the chunk at pos: memory → DB → generate.
func (w *World) LoadChunk(pos game.ChunkPos) (*level.Chunk, error) {
	tick := w.currentTick.Load()

	// Fast path: already in memory
	w.mu.RLock()
	if c, ok := w.chunks[pos]; ok {
		w.mu.RUnlock()
		w.touchAccess(pos, tick)
		return c, nil
	}
	w.mu.RUnlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double-check
	if c, ok := w.chunks[pos]; ok {
		w.lastAccess[pos] = tick
		return c, nil
	}

	// Try loading from DB
	data, err := w.store.LoadChunk(context.Background(), w.dim, pos.X, pos.Z)
	if err != nil {
		return nil, fmt.Errorf("dbworld: load chunk (%d,%d): %w", pos.X, pos.Z, err)
	}

	if data != nil {
		c, err := deserializeChunk(data, w.sections, int32(w.minY/16))
		if err != nil {
			return nil, fmt.Errorf("dbworld: deserialize chunk (%d,%d): %w", pos.X, pos.Z, err)
		}
		w.chunks[pos] = c
		w.lastAccess[pos] = tick
		return c, nil
	}

	// Generate new chunk, mark dirty so it gets persisted
	c := w.gen.Generate(pos)
	w.chunks[pos] = c
	w.dirty[pos] = true
	w.lastAccess[pos] = tick
	return c, nil
}

// SetBlock sets a block and marks the chunk dirty.
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
	w.updateHeightmaps(chunk, x&0xF, y, z&0xF, state)
	w.dirty[pos] = true
	w.lastAccess[pos] = w.currentTick.Load()
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

// FlushDirty serializes all dirty chunks and saves them to the store.
// Returns the number of chunks flushed.
func (w *World) FlushDirty(ctx context.Context) (int, error) {
	w.mu.Lock()
	if len(w.dirty) == 0 {
		w.mu.Unlock()
		return 0, nil
	}

	// Snapshot dirty set and clear it
	dirtyPos := make([]game.ChunkPos, 0, len(w.dirty))
	for pos := range w.dirty {
		dirtyPos = append(dirtyPos, pos)
	}
	w.dirty = make(map[game.ChunkPos]bool)
	w.mu.Unlock()

	// Serialize outside the lock
	batch := make([]store.ChunkData, 0, len(dirtyPos))
	for _, pos := range dirtyPos {
		w.mu.RLock()
		chunk := w.chunks[pos]
		w.mu.RUnlock()
		if chunk == nil {
			continue
		}

		data, err := serializeChunk(chunk, int32(pos.X), int32(pos.Z), int32(w.minY/16))
		if err != nil {
			// Re-mark as dirty so we retry next flush
			w.mu.Lock()
			w.dirty[pos] = true
			w.mu.Unlock()
			return 0, fmt.Errorf("dbworld: serialize chunk (%d,%d): %w", pos.X, pos.Z, err)
		}
		batch = append(batch, store.ChunkData{
			Dimension: w.dim,
			X:         pos.X,
			Z:         pos.Z,
			Data:      data,
		})
	}

	if err := w.store.SaveChunks(ctx, batch); err != nil {
		// Re-mark all as dirty
		w.mu.Lock()
		for _, pos := range dirtyPos {
			w.dirty[pos] = true
		}
		w.mu.Unlock()
		return 0, fmt.Errorf("dbworld: save batch: %w", err)
	}

	return len(batch), nil
}

// FlushTick returns a TickHandler that calls FlushDirty every intervalTicks.
func (w *World) FlushTick(intervalTicks int64) game.TickHandlerFunc {
	return func(tick int64) {
		if tick%intervalTicks != 0 || tick == 0 {
			return
		}
		n, err := w.FlushDirty(context.Background())
		if err != nil {
			if w.logger != nil {
				w.logger.Printf("Flush error: %v", err)
			}
			return
		}
		if n > 0 && w.logger != nil {
			w.logger.Printf("Flushed %d dirty chunks", n)
		}
	}
}

// touchAccess updates the lastAccess timestamp for a chunk position.
func (w *World) touchAccess(pos game.ChunkPos, tick int64) {
	w.mu.Lock()
	w.lastAccess[pos] = tick
	w.mu.Unlock()
}

// UnloadChunk flushes the chunk to the store if dirty, then removes it from memory.
func (w *World) UnloadChunk(pos game.ChunkPos) {
	w.mu.Lock()
	if w.dirty[pos] {
		chunk := w.chunks[pos]
		w.mu.Unlock()

		if chunk != nil {
			w.flushPositions([]game.ChunkPos{pos})
		}

		w.mu.Lock()
	}
	delete(w.chunks, pos)
	delete(w.dirty, pos)
	delete(w.lastAccess, pos)
	w.mu.Unlock()
}

// ChunkCount returns the number of chunks currently loaded in memory.
func (w *World) ChunkCount() int {
	w.mu.RLock()
	n := len(w.chunks)
	w.mu.RUnlock()
	return n
}

// emptyListNBT is a RawMessage representing an empty NBT list (TagList with 0 elements).
var emptyListNBT = nbt.RawMessage{
	Type: nbt.TagList,
	Data: []byte{nbt.TagEnd, 0, 0, 0, 0}, // element type End, length 0
}

// emptyCompoundNBT is a RawMessage representing an empty NBT compound (TagCompound with 0 entries).
var emptyCompoundNBT = nbt.RawMessage{
	Type: nbt.TagCompound,
	Data: []byte{nbt.TagEnd}, // just the end tag
}

// serializeChunk converts a level.Chunk to save format bytes (zlib compressed).
func serializeChunk(c *level.Chunk, chunkX, chunkZ, yPos int32) ([]byte, error) {
	sc := &save.Chunk{
		XPos:           chunkX,
		ZPos:           chunkZ,
		YPos:           yPos,
		DataVersion:    1,
		Status:         "full",
		BlockTicks:     emptyListNBT,
		FluidTicks:     emptyListNBT,
		PostProcessing: emptyListNBT,
		Structures:     emptyCompoundNBT,
	}
	if err := level.ChunkToSave(c, sc); err != nil {
		return nil, err
	}
	return sc.Data(2) // zlib compression
}

// deserializeChunk converts save format bytes back to a level.Chunk.
func deserializeChunk(data []byte, sections int, yPos int32) (*level.Chunk, error) {
	sc := &save.Chunk{YPos: yPos}
	if err := sc.Load(data); err != nil {
		return nil, err
	}
	return level.ChunkFromSave(sc)
}

// updateHeightmaps updates heightmap entries for the given block change.
func (w *World) updateHeightmaps(chunk *level.Chunk, localX, worldY, localZ int, state level.BlocksState) {
	hmIdx := localZ*16 + localX
	absY := worldY - w.minY + 1

	if !block.IsAir(state) {
		if chunk.HeightMaps.WorldSurface != nil {
			if absY > chunk.HeightMaps.WorldSurface.Get(hmIdx) {
				chunk.HeightMaps.WorldSurface.Set(hmIdx, absY)
			}
		}
		if chunk.HeightMaps.MotionBlocking != nil {
			if absY > chunk.HeightMaps.MotionBlocking.Get(hmIdx) {
				chunk.HeightMaps.MotionBlocking.Set(hmIdx, absY)
			}
		}
		if chunk.HeightMaps.MotionBlockingNoLeaves != nil {
			if absY > chunk.HeightMaps.MotionBlockingNoLeaves.Get(hmIdx) {
				chunk.HeightMaps.MotionBlockingNoLeaves.Set(hmIdx, absY)
			}
		}
	}
}
