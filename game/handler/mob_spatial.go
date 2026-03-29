package handler

import (
	"math"
)

const (
	// spatialCellSize is the width of each grid cell in blocks.
	// 16 aligns with Minecraft chunk boundaries.
	spatialCellSize = 16

	// spatialRebuildInterval is the number of ticks between full index rebuilds.
	spatialRebuildInterval = 100

	// entityCullDistance is the maximum distance (in blocks) from any player
	// before a mob's full AI tick is skipped.
	entityCullDistance = 128.0
)

// spatialKey is a 2D grid cell coordinate.
type spatialKey struct {
	cx, cz int
}

// MobSpatialIndex provides chunk-aligned 2D spatial lookups for mobs.
// It uses a simple grid where each cell is spatialCellSize x spatialCellSize blocks.
type MobSpatialIndex struct {
	// cells maps grid coordinates to sets of entity IDs in that cell.
	cells map[spatialKey]map[int32]struct{}
	// pos tracks the last known position for each entity ID, so we can
	// remove it from the correct cell on update/remove.
	pos map[int32]spatialKey
}

// NewMobSpatialIndex creates an empty spatial index.
func NewMobSpatialIndex() *MobSpatialIndex {
	return &MobSpatialIndex{
		cells: make(map[spatialKey]map[int32]struct{}),
		pos:   make(map[int32]spatialKey),
	}
}

// cellFor returns the grid cell for a world position.
func cellFor(x, z float64) spatialKey {
	return spatialKey{
		cx: int(math.Floor(x / spatialCellSize)),
		cz: int(math.Floor(z / spatialCellSize)),
	}
}

// Insert adds a mob to the spatial index at the given position.
func (s *MobSpatialIndex) Insert(eid int32, x, z float64) {
	key := cellFor(x, z)
	s.pos[eid] = key
	cell := s.cells[key]
	if cell == nil {
		cell = make(map[int32]struct{})
		s.cells[key] = cell
	}
	cell[eid] = struct{}{}
}

// Remove removes a mob from the spatial index.
func (s *MobSpatialIndex) Remove(eid int32) {
	key, ok := s.pos[eid]
	if !ok {
		return
	}
	delete(s.pos, eid)
	if cell := s.cells[key]; cell != nil {
		delete(cell, eid)
		if len(cell) == 0 {
			delete(s.cells, key)
		}
	}
}

// Update moves a mob to a new position in the index.
func (s *MobSpatialIndex) Update(eid int32, x, z float64) {
	newKey := cellFor(x, z)
	if oldKey, ok := s.pos[eid]; ok && oldKey == newKey {
		return // same cell, nothing to do
	}
	s.Remove(eid)
	s.Insert(eid, x, z)
}

// QueryRange returns all mob entity IDs in grid cells overlapping a circle
// of the given radius centered on (cx, cz). Results are coarse; callers
// should do a precise distance check.
func (s *MobSpatialIndex) QueryRange(cx, cz, radius float64) []int32 {
	cellRadius := int(math.Ceil(radius/spatialCellSize)) + 1
	center := cellFor(cx, cz)

	var result []int32
	for dcx := -cellRadius; dcx <= cellRadius; dcx++ {
		for dcz := -cellRadius; dcz <= cellRadius; dcz++ {
			key := spatialKey{cx: center.cx + dcx, cz: center.cz + dcz}
			cell := s.cells[key]
			if cell == nil {
				continue
			}
			for eid := range cell {
				result = append(result, eid)
			}
		}
	}
	return result
}

// NearestInRange finds the nearest mob within maxDist blocks of (cx, cz),
// using the provided mob map for exact positions. Returns the EID and
// distance, or (-1, 0) if none found.
func (s *MobSpatialIndex) NearestInRange(cx, cz, maxDist float64, mobs map[int32]*Mob) (int32, float64) {
	candidates := s.QueryRange(cx, cz, maxDist)
	bestEID := int32(-1)
	bestDist := maxDist + 1

	for _, eid := range candidates {
		mob := mobs[eid]
		if mob == nil || mob.Health <= 0 {
			continue
		}
		dx := mob.X - cx
		dz := mob.Z - cz
		d := math.Sqrt(dx*dx + dz*dz)
		if d < bestDist {
			bestDist = d
			bestEID = eid
		}
	}

	if bestEID == -1 {
		return -1, 0
	}
	return bestEID, bestDist
}

// Rebuild clears the index and re-inserts all mobs from the provided map.
func (s *MobSpatialIndex) Rebuild(mobs map[int32]*Mob) {
	s.cells = make(map[spatialKey]map[int32]struct{})
	s.pos = make(map[int32]spatialKey)

	for eid, mob := range mobs {
		if mob.Health > 0 && mob.DeathTick == 0 {
			s.Insert(eid, mob.X, mob.Z)
		}
	}
}

// MobsInRange returns pointers to all living mobs within radius blocks
// of (cx, cz), using the mob map for position lookups.
func (s *MobSpatialIndex) MobsInRange(cx, cz, radius float64, mobs map[int32]*Mob) []*Mob {
	candidates := s.QueryRange(cx, cz, radius)
	r2 := radius * radius
	var result []*Mob
	for _, eid := range candidates {
		mob := mobs[eid]
		if mob == nil || mob.Health <= 0 {
			continue
		}
		dx := mob.X - cx
		dz := mob.Z - cz
		if dx*dx+dz*dz <= r2 {
			result = append(result, mob)
		}
	}
	return result
}

// isNearAnyPlayer returns true if the position (x, z) is within dist blocks
// of any player position in the provided slice.
func isNearAnyPlayer(x, z float64, players [][2]float64, dist float64) bool {
	d2 := dist * dist
	for _, p := range players {
		dx := x - p[0]
		dz := z - p[1]
		if dx*dx+dz*dz <= d2 {
			return true
		}
	}
	return false
}
