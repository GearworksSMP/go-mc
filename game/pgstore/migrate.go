package pgstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrations is an ordered list of SQL statements. Each index is the version number.
var migrations = []string{
	// Version 0 → 1: initial schema
	`CREATE TABLE IF NOT EXISTS chunks (
		dimension  TEXT NOT NULL DEFAULT 'overworld',
		chunk_x    INTEGER NOT NULL,
		chunk_z    INTEGER NOT NULL,
		data       BYTEA NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (dimension, chunk_x, chunk_z)
	);

	CREATE TABLE IF NOT EXISTS players (
		uuid       UUID PRIMARY KEY,
		name       TEXT NOT NULL,
		x          DOUBLE PRECISION NOT NULL,
		y          DOUBLE PRECISION NOT NULL,
		z          DOUBLE PRECISION NOT NULL,
		yaw        REAL NOT NULL,
		pitch      REAL NOT NULL,
		dimension  TEXT NOT NULL DEFAULT 'overworld',
		game_mode  INTEGER NOT NULL DEFAULT 1,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	);
	INSERT INTO schema_version (version) VALUES (1);`,

	// Version 1 → 2: add health, food, saturation columns
	`ALTER TABLE players ADD COLUMN IF NOT EXISTS health REAL NOT NULL DEFAULT 20;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS food INTEGER NOT NULL DEFAULT 20;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS saturation REAL NOT NULL DEFAULT 5;
	UPDATE schema_version SET version = 2;`,

	// Version 2 → 3: add inventory column
	`ALTER TABLE players ADD COLUMN IF NOT EXISTS inventory JSONB;
	UPDATE schema_version SET version = 3;`,

	// Version 3 → 4: add block_entities table and XP columns
	`CREATE TABLE IF NOT EXISTS block_entities (
		dimension TEXT NOT NULL DEFAULT 'overworld',
		x INTEGER NOT NULL,
		y INTEGER NOT NULL,
		z INTEGER NOT NULL,
		type TEXT NOT NULL,
		data JSONB NOT NULL,
		PRIMARY KEY (dimension, x, y, z)
	);
	ALTER TABLE players ADD COLUMN IF NOT EXISTS experience REAL NOT NULL DEFAULT 0;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS experience_level INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS experience_total INTEGER NOT NULL DEFAULT 0;
	UPDATE schema_version SET version = 4;`,

	// Version 4 → 5: add spawn point columns
	`ALTER TABLE players ADD COLUMN IF NOT EXISTS spawn_x DOUBLE PRECISION NOT NULL DEFAULT 0;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS spawn_y DOUBLE PRECISION NOT NULL DEFAULT 0;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS spawn_z DOUBLE PRECISION NOT NULL DEFAULT 0;
	ALTER TABLE players ADD COLUMN IF NOT EXISTS has_spawn_point BOOLEAN NOT NULL DEFAULT false;
	UPDATE schema_version SET version = 5;`,

	// Version 5 → 6: add mobs table
	`CREATE TABLE IF NOT EXISTS mobs (
		id SERIAL PRIMARY KEY,
		dimension TEXT NOT NULL DEFAULT 'overworld',
		type_id INTEGER NOT NULL,
		x DOUBLE PRECISION NOT NULL,
		y DOUBLE PRECISION NOT NULL,
		z DOUBLE PRECISION NOT NULL,
		yaw REAL NOT NULL DEFAULT 0,
		health REAL NOT NULL,
		max_health REAL NOT NULL,
		extra JSONB NOT NULL DEFAULT '{}'
	);
	UPDATE schema_version SET version = 6;`,
}

// Migrate runs all pending schema migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure schema_version table exists (might not on first run)
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("pgstore: create schema_version: %w", err)
	}

	var current int
	err = pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil {
		return fmt.Errorf("pgstore: read schema version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		if _, err := pool.Exec(ctx, migrations[i]); err != nil {
			return fmt.Errorf("pgstore: migration %d: %w", i+1, err)
		}
	}

	return nil
}
