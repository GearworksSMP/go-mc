package game

import (
	"math"
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

	// EIDAllocFunc overrides the default entity ID allocation when set.
	// Used by cluster.EIDAllocator to provide range-based allocation.
	EIDAllocFunc func() int32
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
	if pm.EIDAllocFunc != nil {
		return pm.EIDAllocFunc()
	}
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

// ForEachNearby calls fn for each player within radius blocks (2D horizontal distance)
// of the point (x, z). Do not call Add/Remove inside fn.
func (pm *PlayerManager) ForEachNearby(x, z float64, radius float64, fn func(*Player)) {
	r2 := radius * radius
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.players {
		px, _, pz := p.Position()
		dx := px - x
		dz := pz - z
		if dx*dx+dz*dz <= r2 {
			fn(p)
		}
	}
}

// DistanceSq2D returns the squared 2D horizontal distance between two points.
func DistanceSq2D(x1, z1, x2, z2 float64) float64 {
	dx := x1 - x2
	dz := z1 - z2
	return dx*dx + dz*dz
}

// Distance2D returns the 2D horizontal distance between two points.
func Distance2D(x1, z1, x2, z2 float64) float64 {
	return math.Sqrt(DistanceSq2D(x1, z1, x2, z2))
}
