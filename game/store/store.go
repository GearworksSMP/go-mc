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
	ID            int32            `json:"id"`
	Count         int32            `json:"count"`
	Durability    int32            `json:"durability,omitempty"`
	MaxDurability int32            `json:"max_durability,omitempty"`
	Enchantments  map[string]int32 `json:"enchantments,omitempty"`
	DisplayName   string           `json:"display_name,omitempty"`
	PotionType    string           `json:"potion_type,omitempty"`
}

// EffectData represents an active status effect for persistence.
type EffectData struct {
	ID       int32 `json:"id"`
	Level    int32 `json:"level"`
	Duration int32 `json:"duration"`
	Ambient  bool  `json:"ambient,omitempty"`
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
	Exhaustion      float32
	Inventory       []ItemSlot
	EnderChest      []ItemSlot
	Effects         []EffectData
	Experience      float32
	ExperienceLevel int32
	ExperienceTotal int32
	SpawnX          float64
	SpawnY          float64
	SpawnZ          float64
	HasSpawnPoint   bool
	Advancements    []string `json:"advancements,omitempty"`
	UnlockedRecipes []string `json:"unlocked_recipes,omitempty"`
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

// MobData holds the persistent state of a mob entity.
type MobData struct {
	Dimension string  `json:"dimension"`
	TypeID    int32   `json:"type_id"`
	X, Y, Z   float64 `json:"x"`
	Yaw       float32 `json:"yaw"`
	Health    float32 `json:"health"`
	MaxHealth float32 `json:"max_health"`
	Extra     []byte  `json:"extra,omitempty"` // JSON for type-specific fields
}

// MobStore provides persistent storage for mob entities.
type MobStore interface {
	LoadMobs(ctx context.Context, dimension string) ([]MobData, error)
	SaveMobs(ctx context.Context, dimension string, mobs []MobData) error
}
