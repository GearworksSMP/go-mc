// Package dbworld chunk eviction support.
package dbworld

import (
	"context"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/level"
)

// EvictionPolicy configures chunk eviction behavior.
type EvictionPolicy struct {
	// MinChunks is the minimum number of chunks to keep loaded regardless of idle time.
	MinChunks int

	// MaxIdleTicks is the maximum number of ticks since last access before a chunk
	// becomes eligible for eviction. Default 6000 (5 minutes at 20 TPS).
	MaxIdleTicks int64
}

// DefaultEvictionPolicy returns an EvictionPolicy with sensible defaults.
func DefaultEvictionPolicy() EvictionPolicy {
	return EvictionPolicy{
		MinChunks:    256,
		MaxIdleTicks: 6000,
	}
}

// EvictUnused removes idle chunks from the world that are not within any
// player's view distance. It flushes dirty chunks to the store before removal.
// Returns the number of evicted chunks.
func EvictUnused(w *World, playerPositions [][2]int, viewDistance int, currentTick int64, policy EvictionPolicy) int {
	w.mu.Lock()
	loaded := len(w.chunks)
	if loaded <= policy.MinChunks {
		w.mu.Unlock()
		return 0
	}

	// Build list of candidates while holding the lock.
	type candidate struct {
		pos   game.ChunkPos
		dirty bool
	}
	var candidates []candidate

	for pos := range w.chunks {
		// Check if within any player's view distance.
		if isNearAnyPlayer(pos, playerPositions, viewDistance) {
			continue
		}

		// Check idle time.
		lastAccess, ok := w.lastAccess[pos]
		if ok && (currentTick-lastAccess) < policy.MaxIdleTicks {
			continue
		}

		candidates = append(candidates, candidate{pos: pos, dirty: w.dirty[pos]})
	}

	// Respect MinChunks: don't evict more than would bring us below the minimum.
	maxEvict := loaded - policy.MinChunks
	if maxEvict <= 0 {
		w.mu.Unlock()
		return 0
	}
	if len(candidates) > maxEvict {
		candidates = candidates[:maxEvict]
	}

	// Separate dirty chunks that need flushing.
	var dirtyToFlush []game.ChunkPos
	for _, c := range candidates {
		if c.dirty {
			dirtyToFlush = append(dirtyToFlush, c.pos)
		}
	}
	w.mu.Unlock()

	// Flush dirty candidates before evicting.
	if len(dirtyToFlush) > 0 {
		w.flushPositions(dirtyToFlush)
	}

	// Remove evicted chunks.
	w.mu.Lock()
	evicted := 0
	for _, c := range candidates {
		// Re-check that the chunk still exists and hasn't been re-accessed.
		if _, ok := w.chunks[c.pos]; !ok {
			continue
		}
		if la, ok := w.lastAccess[c.pos]; ok && (currentTick-la) < policy.MaxIdleTicks {
			continue // re-accessed while we were flushing
		}
		delete(w.chunks, c.pos)
		delete(w.dirty, c.pos)
		delete(w.lastAccess, c.pos)
		evicted++
	}
	w.mu.Unlock()

	return evicted
}

// isNearAnyPlayer returns true if the chunk position is within viewDistance
// chunks of any player position.
func isNearAnyPlayer(pos game.ChunkPos, playerPositions [][2]int, viewDistance int) bool {
	for _, pp := range playerPositions {
		dx := pos.X - pp[0]
		dz := pos.Z - pp[1]
		if dx < 0 {
			dx = -dx
		}
		if dz < 0 {
			dz = -dz
		}
		if dx <= viewDistance && dz <= viewDistance {
			return true
		}
	}
	return false
}

// flushPositions serializes and saves specific dirty chunks to the store.
func (w *World) flushPositions(positions []game.ChunkPos) {
	// Snapshot chunks under a single read lock.
	type snapshot struct {
		pos   game.ChunkPos
		chunk *level.Chunk
	}
	w.mu.RLock()
	toSerialize := make([]snapshot, 0, len(positions))
	for _, pos := range positions {
		if w.dirty[pos] {
			if c := w.chunks[pos]; c != nil {
				toSerialize = append(toSerialize, snapshot{pos: pos, chunk: c})
			}
		}
	}
	w.mu.RUnlock()

	if len(toSerialize) == 0 {
		return
	}

	// Serialize outside any lock.
	batch := make([]store.ChunkData, 0, len(toSerialize))
	var savedPositions []game.ChunkPos
	for _, s := range toSerialize {
		data, err := serializeChunk(s.chunk, int32(s.pos.X), int32(s.pos.Z), int32(w.minY/16))
		if err != nil {
			if w.logger != nil {
				w.logger.Printf("eviction flush serialize (%d,%d): %v", s.pos.X, s.pos.Z, err)
			}
			continue
		}
		batch = append(batch, store.ChunkData{
			Dimension: w.dim,
			X:         s.pos.X,
			Z:         s.pos.Z,
			Data:      data,
		})
		savedPositions = append(savedPositions, s.pos)
	}

	if len(batch) == 0 {
		return
	}

	if err := w.store.SaveChunks(context.Background(), batch); err != nil {
		if w.logger != nil {
			w.logger.Printf("eviction flush save: %v", err)
		}
		return
	}

	w.mu.Lock()
	for _, pos := range savedPositions {
		delete(w.dirty, pos)
	}
	w.mu.Unlock()
}
