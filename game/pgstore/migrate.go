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
