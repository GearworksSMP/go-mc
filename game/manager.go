package game

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// PlayerManager is a thread-safe registry of connected players.
type PlayerManager struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*Player
	nextEID atomic.Int32
}

// NewPlayerManager creates a new PlayerManager.
func NewPlayerManager() *PlayerManager {
	pm := &PlayerManager{
		players: make(map[uuid.UUID]*Player),
	}
	pm.nextEID.Store(1) // entity ID 0 is reserved
	return pm
}

// NextEntityID allocates and returns the next entity ID.
func (pm *PlayerManager) NextEntityID() int32 {
	return pm.nextEID.Add(1) - 1
}

// Add registers a player. Returns false if the UUID is already registered.
func (pm *PlayerManager) Add(p *Player) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, exists := pm.players[p.UUID]; exists {
		return false
	}
	pm.players[p.UUID] = p
	return true
}

// Remove unregisters a player by UUID.
func (pm *PlayerManager) Remove(id uuid.UUID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.players, id)
}

// Get returns a player by UUID, or nil if not found.
func (pm *PlayerManager) Get(id uuid.UUID) *Player {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.players[id]
}

// Count returns the number of connected players.
func (pm *PlayerManager) Count() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.players)
}

// GetByEID returns a player by entity ID, or nil if not found.
func (pm *PlayerManager) GetByEID(eid int32) *Player {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.players {
		if p.EID == eid {
			return p
		}
	}
	return nil
}

// GetByName returns a player by name (case-insensitive), or nil if not found.
func (pm *PlayerManager) GetByName(name string) *Player {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.players {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// ForEach calls fn for each registered player. Do not call Add/Remove inside fn.
func (pm *PlayerManager) ForEach(fn func(*Player)) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.players {
		fn(p)
	}
}
