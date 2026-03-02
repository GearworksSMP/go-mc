// Package store defines interfaces for persistent storage of chunks and player state.
package store

import (
	"context"

	"github.com/google/uuid"
)

// ChunkData holds a serialized chunk and its position for batch operations.
type ChunkData struct {
	Dimension string
	X, Z      int
	Data      []byte
}

// ChunkStore provides persistent storage for serialized chunk data.
type ChunkStore interface {
	// LoadChunk returns the serialized chunk data, or nil if not found.
	LoadChunk(ctx context.Context, dimension string, x, z int) ([]byte, error)

	// SaveChunks batch-upserts multiple chunks in a single transaction.
	SaveChunks(ctx context.Context, chunks []ChunkData) error
}

// ItemSlot represents a single inventory slot for persistence.
type ItemSlot struct {
	ID            int32 `json:"id"`
	Count         int32 `json:"count"`
	Durability    int32 `json:"durability,omitempty"`
	MaxDurability int32 `json:"max_durability,omitempty"`
}

// PlayerState holds the persistent state of a player.
type PlayerState struct {
	UUID            uuid.UUID
	Name            string
	Dimension       string
	X, Y, Z         float64
	Yaw             float32
	Pitch           float32
	GameMode        int
	Health          float32
	Food            int32
	Saturation      float32
	Inventory       []ItemSlot
	Experience      float32
	ExperienceLevel int32
	ExperienceTotal int32
	SpawnX          float64
	SpawnY          float64
	SpawnZ          float64
	HasSpawnPoint   bool
}

// BlockEntityData holds a serialized block entity and its position.
type BlockEntityData struct {
	Dimension string
	X, Y, Z   int
	Type      string
	Data      []byte // JSON
}

// PlayerStore provides persistent storage for player state.
type PlayerStore interface {
	// LoadPlayer returns the player state, or nil if not found.
	LoadPlayer(ctx context.Context, id uuid.UUID) (*PlayerState, error)

	// SavePlayer upserts the player state.
	SavePlayer(ctx context.Context, state *PlayerState) error
}
