// Package pgstore implements store.ChunkStore and store.PlayerStore using PostgreSQL.
package pgstore

import (
	"context"
	"encoding/json"
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
	var invJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT uuid, name, dimension, x, y, z, yaw, pitch, game_mode, health, food, saturation, inventory,
		        COALESCE(experience, 0), COALESCE(experience_level, 0), COALESCE(experience_total, 0),
		        COALESCE(spawn_x, 0), COALESCE(spawn_y, 0), COALESCE(spawn_z, 0), COALESCE(has_spawn_point, false)
		 FROM players WHERE uuid=$1`, id,
	).Scan(&ps.UUID, &ps.Name, &ps.Dimension, &ps.X, &ps.Y, &ps.Z, &ps.Yaw, &ps.Pitch, &ps.GameMode,
		&ps.Health, &ps.Food, &ps.Saturation, &invJSON,
		&ps.Experience, &ps.ExperienceLevel, &ps.ExperienceTotal,
		&ps.SpawnX, &ps.SpawnY, &ps.SpawnZ, &ps.HasSpawnPoint)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pgstore: load player %x: %w", id, err)
	}
	if len(invJSON) > 0 {
		if err := json.Unmarshal(invJSON, &ps.Inventory); err != nil {
			return nil, fmt.Errorf("pgstore: unmarshal inventory for %x: %w", id, err)
		}
	}
	return ps, nil
}

// SavePlayer upserts the player state.
func (s *PGStore) SavePlayer(ctx context.Context, state *store.PlayerState) error {
	var invJSON []byte
	if len(state.Inventory) > 0 {
		var err error
		invJSON, err = json.Marshal(state.Inventory)
		if err != nil {
			return fmt.Errorf("pgstore: marshal inventory for %s: %w", state.Name, err)
		}
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO players (uuid, name, dimension, x, y, z, yaw, pitch, game_mode, health, food, saturation, inventory, experience, experience_level, experience_total, spawn_x, spawn_y, spawn_z, has_spawn_point, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, now())
		 ON CONFLICT (uuid)
		 DO UPDATE SET name=EXCLUDED.name, dimension=EXCLUDED.dimension,
		   x=EXCLUDED.x, y=EXCLUDED.y, z=EXCLUDED.z,
		   yaw=EXCLUDED.yaw, pitch=EXCLUDED.pitch,
		   game_mode=EXCLUDED.game_mode, health=EXCLUDED.health,
		   food=EXCLUDED.food, saturation=EXCLUDED.saturation,
		   inventory=EXCLUDED.inventory,
		   experience=EXCLUDED.experience, experience_level=EXCLUDED.experience_level,
		   experience_total=EXCLUDED.experience_total,
		   spawn_x=EXCLUDED.spawn_x, spawn_y=EXCLUDED.spawn_y, spawn_z=EXCLUDED.spawn_z,
		   has_spawn_point=EXCLUDED.has_spawn_point,
		   updated_at=EXCLUDED.updated_at`,
		state.UUID, state.Name, state.Dimension, state.X, state.Y, state.Z,
		state.Yaw, state.Pitch, state.GameMode, state.Health, state.Food, state.Saturation, invJSON,
		state.Experience, state.ExperienceLevel, state.ExperienceTotal,
		state.SpawnX, state.SpawnY, state.SpawnZ, state.HasSpawnPoint,
	)
	if err != nil {
		return fmt.Errorf("pgstore: save player %s: %w", state.Name, err)
	}
	return nil
}

// SaveBlockEntity upserts a block entity (chest, furnace) at the given position.
func (s *PGStore) SaveBlockEntity(ctx context.Context, dimension string, x, y, z int, entityType string, data json.RawMessage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO block_entities (dimension, x, y, z, type, data)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (dimension, x, y, z)
		 DO UPDATE SET type=EXCLUDED.type, data=EXCLUDED.data`,
		dimension, x, y, z, entityType, data,
	)
	if err != nil {
		return fmt.Errorf("pgstore: save block entity at (%d,%d,%d): %w", x, y, z, err)
	}
	return nil
}

// LoadBlockEntities loads all block entities (returns dimension, x, y, z, type, data).
func (s *PGStore) LoadBlockEntities(ctx context.Context, dimension string) ([]store.BlockEntityData, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT x, y, z, type, data FROM block_entities WHERE dimension=$1`, dimension)
	if err != nil {
		return nil, fmt.Errorf("pgstore: load block entities: %w", err)
	}
	defer rows.Close()

	var entities []store.BlockEntityData
	for rows.Next() {
		var e store.BlockEntityData
		e.Dimension = dimension
		if err := rows.Scan(&e.X, &e.Y, &e.Z, &e.Type, &e.Data); err != nil {
			return nil, fmt.Errorf("pgstore: scan block entity: %w", err)
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

// DeleteBlockEntity removes a block entity at the given position.
func (s *PGStore) DeleteBlockEntity(ctx context.Context, dimension string, x, y, z int) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM block_entities WHERE dimension=$1 AND x=$2 AND y=$3 AND z=$4`,
		dimension, x, y, z,
	)
	if err != nil {
		return fmt.Errorf("pgstore: delete block entity at (%d,%d,%d): %w", x, y, z, err)
	}
	return nil
}

// SaveMobs deletes all mobs for the dimension and batch-inserts the new set.
func (s *PGStore) SaveMobs(ctx context.Context, dimension string, mobs []store.MobData) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgstore: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM mobs WHERE dimension=$1`, dimension); err != nil {
		return fmt.Errorf("pgstore: delete mobs: %w", err)
	}

	for _, m := range mobs {
		extra := m.Extra
		if len(extra) == 0 {
			extra = []byte("{}")
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO mobs (dimension, type_id, x, y, z, yaw, health, max_health, extra)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			dimension, m.TypeID, m.X, m.Y, m.Z, m.Yaw, m.Health, m.MaxHealth, extra,
		)
		if err != nil {
			return fmt.Errorf("pgstore: insert mob: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// LoadMobs loads all mobs for the given dimension.
func (s *PGStore) LoadMobs(ctx context.Context, dimension string) ([]store.MobData, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT type_id, x, y, z, yaw, health, max_health, extra FROM mobs WHERE dimension=$1`, dimension)
	if err != nil {
		return nil, fmt.Errorf("pgstore: load mobs: %w", err)
	}
	defer rows.Close()

	var mobs []store.MobData
	for rows.Next() {
		var m store.MobData
		m.Dimension = dimension
		if err := rows.Scan(&m.TypeID, &m.X, &m.Y, &m.Z, &m.Yaw, &m.Health, &m.MaxHealth, &m.Extra); err != nil {
			return nil, fmt.Errorf("pgstore: scan mob: %w", err)
		}
		mobs = append(mobs, m)
	}
	return mobs, rows.Err()
}
