// Package pgstore implements store.ChunkStore and store.PlayerStore using PostgreSQL.
package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tnze/go-mc/game/store"
)

// PGStore implements both store.ChunkStore and store.PlayerStore using a pgx pool.
type PGStore struct {
	pool *pgxpool.Pool
}

// New creates a PGStore with a connection pool to the given database URL.
func New(ctx context.Context, databaseURL string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgstore: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	return &PGStore{pool: pool}, nil
}

// Pool returns the underlying pgx pool (useful for migrations).
func (s *PGStore) Pool() *pgxpool.Pool { return s.pool }

// Close shuts down the connection pool.
func (s *PGStore) Close() { s.pool.Close() }

// LoadChunk returns the serialized chunk data, or nil if not found.
func (s *PGStore) LoadChunk(ctx context.Context, dimension string, x, z int) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT data FROM chunks WHERE dimension=$1 AND chunk_x=$2 AND chunk_z=$3`,
		dimension, x, z,
	).Scan(&data)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: load chunk (%s,%d,%d): %w", dimension, x, z, err)
	}
	return data, nil
}

// SaveChunks batch-upserts multiple chunks in a single transaction.
func (s *PGStore) SaveChunks(ctx context.Context, chunks []store.ChunkData) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, c := range chunks {
		_, err := tx.Exec(ctx,
			`INSERT INTO chunks (dimension, chunk_x, chunk_z, data, updated_at)
			 VALUES ($1, $2, $3, $4, now())
			 ON CONFLICT (dimension, chunk_x, chunk_z)
			 DO UPDATE SET data = EXCLUDED.data, updated_at = EXCLUDED.updated_at`,
			c.Dimension, c.X, c.Z, c.Data,
		)
		if err != nil {
			return fmt.Errorf("pgstore: save chunk (%s,%d,%d): %w", c.Dimension, c.X, c.Z, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgstore: commit: %w", err)
	}
	return nil
}

// LoadPlayer returns the player state, or nil if not found.
func (s *PGStore) LoadPlayer(ctx context.Context, id uuid.UUID) (*store.PlayerState, error) {
	ps := &store.PlayerState{}
	err := s.pool.QueryRow(ctx,
		`SELECT uuid, name, dimension, x, y, z, yaw, pitch, game_mode
		 FROM players WHERE uuid=$1`, id,
	).Scan(&ps.UUID, &ps.Name, &ps.Dimension, &ps.X, &ps.Y, &ps.Z, &ps.Yaw, &ps.Pitch, &ps.GameMode)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: load player %x: %w", id, err)
	}
	return ps, nil
}

// SavePlayer upserts the player state.
func (s *PGStore) SavePlayer(ctx context.Context, state *store.PlayerState) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO players (uuid, name, dimension, x, y, z, yaw, pitch, game_mode, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		 ON CONFLICT (uuid)
		 DO UPDATE SET name=EXCLUDED.name, dimension=EXCLUDED.dimension,
		   x=EXCLUDED.x, y=EXCLUDED.y, z=EXCLUDED.z,
		   yaw=EXCLUDED.yaw, pitch=EXCLUDED.pitch,
		   game_mode=EXCLUDED.game_mode, updated_at=EXCLUDED.updated_at`,
		state.UUID, state.Name, state.Dimension, state.X, state.Y, state.Z,
		state.Yaw, state.Pitch, state.GameMode,
	)
	if err != nil {
		return fmt.Errorf("pgstore: save player %s: %w", state.Name, err)
	}
	return nil
}
