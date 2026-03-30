package handler

import (
	"math"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// sqDist3 returns the squared distance given dx,dy,dz (utility to avoid repeating).
func sqDist3(dx, dy, dz float64) float64 {
	return dx*dx + dy*dy + dz*dz
}

// broadcastEntityEvent sends an entity event packet to all players.
func (m *MobManager) broadcastEntityEvent(mob *Mob, event byte) {
	pkt := pk.Marshal(
		packetid.ClientboundEntityEvent,
		pk.Int(mob.EID),
		pk.Byte(event),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// mobsInRange returns all living mobs within radius blocks of (x, z),
// using the spatial index for efficient lookup.
func (m *MobManager) mobsInRange(x, z, radius float64) []*Mob {
	return m.Spatial.MobsInRange(x, z, radius, m.Mobs)
}

// nearestPlayerPos finds the closest player to (x, z) within maxDist blocks.
// Returns the player and distance, or (nil, 0) if none found.
func (m *MobManager) nearestPlayerPos(x, z, maxDist float64) (*game.Player, float64) {
	var best *game.Player
	bestDist := maxDist + 1
	m.Manager.ForEachNearby(x, z, maxDist, func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, _, pz := p.Position()
		dx := px - x
		dz := pz - z
		d := math.Sqrt(dx*dx + dz*dz)
		if d < bestDist {
			bestDist = d
			best = p
		}
	})
	if best == nil {
		return nil, 0
	}
	return best, bestDist
}
