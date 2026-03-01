package game

import (
	"testing"

	"github.com/google/uuid"
)

func TestPlayerManagerAddRemove(t *testing.T) {
	pm := NewPlayerManager()

	id1 := uuid.New()
	p1 := NewPlayer("Alice", id1, pm.NextEntityID(), nil)

	if !pm.Add(p1) {
		t.Fatal("Add should succeed for new player")
	}
	if pm.Count() != 1 {
		t.Fatalf("Count = %d, want 1", pm.Count())
	}

	// Duplicate add should fail
	if pm.Add(p1) {
		t.Fatal("Add should fail for duplicate UUID")
	}

	// Get should return the player
	got := pm.Get(id1)
	if got != p1 {
		t.Fatal("Get returned wrong player")
	}

	// Remove
	pm.Remove(id1)
	if pm.Count() != 0 {
		t.Fatalf("Count = %d after remove, want 0", pm.Count())
	}
	if pm.Get(id1) != nil {
		t.Fatal("Get should return nil after remove")
	}
}

func TestPlayerManagerEntityIDs(t *testing.T) {
	pm := NewPlayerManager()

	id1 := pm.NextEntityID()
	id2 := pm.NextEntityID()
	id3 := pm.NextEntityID()

	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Errorf("Entity IDs should be unique: %d, %d, %d", id1, id2, id3)
	}
	if id1 < 1 {
		t.Errorf("First entity ID should be >= 1, got %d", id1)
	}
}

func TestPlayerManagerForEach(t *testing.T) {
	pm := NewPlayerManager()

	for i := 0; i < 5; i++ {
		p := NewPlayer("Player", uuid.New(), pm.NextEntityID(), nil)
		pm.Add(p)
	}

	count := 0
	pm.ForEach(func(p *Player) {
		count++
	})

	if count != 5 {
		t.Errorf("ForEach visited %d players, want 5", count)
	}
}
