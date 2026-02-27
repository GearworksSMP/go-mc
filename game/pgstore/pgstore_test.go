package pgstore_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Tnze/go-mc/game/pgstore"
	"github.com/Tnze/go-mc/game/store"
)

func getTestDB(t *testing.T) *pgstore.PGStore {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pg, err := pgstore.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgstore.New: %v", err)
	}
	if err := pgstore.Migrate(ctx, pg.Pool()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { pg.Close() })
	return pg
}

func TestChunkRoundTrip(t *testing.T) {
	pg := getTestDB(t)
	ctx := context.Background()

	data := []byte{1, 2, 3, 4, 5}
	err := pg.SaveChunks(ctx, []store.ChunkData{
		{Dimension: "test_dim", X: 10, Z: 20, Data: data},
	})
	if err != nil {
		t.Fatalf("SaveChunks: %v", err)
	}

	got, err := pg.LoadChunk(ctx, "test_dim", 10, 20)
	if err != nil {
		t.Fatalf("LoadChunk: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("LoadChunk = %v, want %v", got, data)
	}

	// Not found
	got, err = pg.LoadChunk(ctx, "test_dim", 99, 99)
	if err != nil {
		t.Fatalf("LoadChunk not found: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing chunk, got %v", got)
	}
}

func TestPlayerRoundTrip(t *testing.T) {
	pg := getTestDB(t)
	ctx := context.Background()

	id := uuid.New()
	ps := &store.PlayerState{
		UUID:      id,
		Name:      "testplayer",
		Dimension: "overworld",
		X:         1.5,
		Y:         64.0,
		Z:         -3.5,
		Yaw:       90.0,
		Pitch:     -10.0,
		GameMode:  1,
	}

	if err := pg.SavePlayer(ctx, ps); err != nil {
		t.Fatalf("SavePlayer: %v", err)
	}

	got, err := pg.LoadPlayer(ctx, id)
	if err != nil {
		t.Fatalf("LoadPlayer: %v", err)
	}
	if got == nil {
		t.Fatal("LoadPlayer returned nil")
	}
	if got.X != ps.X || got.Y != ps.Y || got.Z != ps.Z {
		t.Errorf("position mismatch: got (%.1f,%.1f,%.1f), want (%.1f,%.1f,%.1f)",
			got.X, got.Y, got.Z, ps.X, ps.Y, ps.Z)
	}
	if got.Yaw != ps.Yaw || got.Pitch != ps.Pitch {
		t.Errorf("rotation mismatch: got (%.1f,%.1f), want (%.1f,%.1f)",
			got.Yaw, got.Pitch, ps.Yaw, ps.Pitch)
	}

	// Not found
	got, err = pg.LoadPlayer(ctx, uuid.New())
	if err != nil {
		t.Fatalf("LoadPlayer not found: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing player, got %+v", got)
	}
}
