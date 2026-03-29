package dbworld

import (
	"context"
	"log"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/level"
)

// flushRequest is a batch of serialized chunks to save asynchronously.
type flushRequest struct {
	batch []store.ChunkData
}

// AsyncFlusher serializes and saves dirty chunks in a background goroutine
// so the main tick loop is not blocked by I/O.
type AsyncFlusher struct {
	store  store.ChunkStore
	ch     chan flushRequest
	done   chan struct{}
	logger *log.Logger
}

// NewAsyncFlusher creates an AsyncFlusher and starts its background worker.
// The worker processes flush requests until Close is called.
func NewAsyncFlusher(s store.ChunkStore, logger *log.Logger) *AsyncFlusher {
	af := &AsyncFlusher{
		store:  s,
		ch:     make(chan flushRequest, 8),
		done:   make(chan struct{}),
		logger: logger,
	}
	go af.run()
	return af
}

// FlushDirty takes a snapshot of dirty chunks from the world, serializes them,
// and sends the data to the background worker for saving. This method does not
// block on I/O; it only blocks on serialization.
func (af *AsyncFlusher) FlushDirty(w *World) {
	snapshot := w.DirtySnapshot()
	if len(snapshot) == 0 {
		return
	}

	batch := make([]store.ChunkData, 0, len(snapshot))
	for pos, chunk := range snapshot {
		data, err := serializeChunk(chunk, int32(pos.X), int32(pos.Z), int32(w.minY/16))
		if err != nil {
			if af.logger != nil {
				af.logger.Printf("async flush serialize (%d,%d): %v", pos.X, pos.Z, err)
			}
			// Re-mark as dirty so it will be retried.
			w.mu.Lock()
			w.dirty[pos] = true
			w.mu.Unlock()
			continue
		}
		batch = append(batch, store.ChunkData{
			Dimension: w.dim,
			X:         pos.X,
			Z:         pos.Z,
			Data:      data,
		})
	}

	if len(batch) == 0 {
		return
	}

	// Non-blocking send; drop if channel is full (will retry next interval).
	select {
	case af.ch <- flushRequest{batch: batch}:
	default:
		if af.logger != nil {
			af.logger.Printf("async flush: channel full, skipping %d chunks", len(batch))
		}
		// Re-mark as dirty.
		w.mu.Lock()
		for pos := range snapshot {
			w.dirty[pos] = true
		}
		w.mu.Unlock()
	}
}

// Close signals the worker to drain remaining requests and stop.
// It blocks until the worker has finished.
func (af *AsyncFlusher) Close() {
	close(af.ch)
	<-af.done
}

func (af *AsyncFlusher) run() {
	defer close(af.done)
	for req := range af.ch {
		if err := af.store.SaveChunks(context.Background(), req.batch); err != nil {
			if af.logger != nil {
				af.logger.Printf("async flush save error: %v", err)
			}
		}
	}
}

// DirtySnapshot returns a copy of the dirty chunk references and clears the
// dirty flags. The returned map can be serialized without holding the world lock.
func (w *World) DirtySnapshot() map[game.ChunkPos]*level.Chunk {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.dirty) == 0 {
		return nil
	}

	snapshot := make(map[game.ChunkPos]*level.Chunk, len(w.dirty))
	for pos := range w.dirty {
		if c, ok := w.chunks[pos]; ok {
			snapshot[pos] = c
		}
	}
	w.dirty = make(map[game.ChunkPos]bool)
	return snapshot
}
