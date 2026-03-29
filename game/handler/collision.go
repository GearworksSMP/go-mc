package handler

import (
	"math"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

const (
	playerHitboxWidth    = 0.6    // XZ width in blocks
	mobDefaultHitboxWidth = 0.6   // XZ width in blocks
	pushStrength         = 0.05   // blocks/tick push force
	collisionQueryRadius = 2.0    // spatial query radius in blocks
	velocityScale        = 8000.0 // protocol velocity units per block/tick
)

// CollisionManager handles entity push-apart physics each tick.
type CollisionManager struct {
	Manager    *game.PlayerManager
	MobManager *MobManager
}

// collisionSnap is a snapshot of a player's position for collision checks.
type collisionSnap struct {
	player *game.Player
	x, z   float64
	eid    int32
}

// Tick runs collision checks for all entity pairs.
func (c *CollisionManager) Tick() {
	if c.Manager == nil {
		return
	}

	var players []collisionSnap
	c.Manager.ForEach(func(p *game.Player) {
		if p.Dead || IsSpectator(p) || p.RidingEntityEID != 0 {
			return
		}
		x, _, z := p.Position()
		players = append(players, collisionSnap{player: p, x: x, z: z, eid: p.EID})
	})

	c.pushPlayers(players)

	if c.MobManager != nil {
		c.pushMobsAndPlayers(players)
	}
}

// pushPlayers applies push-apart forces between overlapping players.
func (c *CollisionManager) pushPlayers(players []collisionSnap) {
	minDist := playerHitboxWidth
	for i := 0; i < len(players); i++ {
		for j := i + 1; j < len(players); j++ {
			a := &players[i]
			b := &players[j]
			dx := b.x - a.x
			dz := b.z - a.z
			dist := math.Sqrt(dx*dx + dz*dz)
			if dist >= minDist || dist < 1e-6 {
				continue
			}

			force := (minDist - dist) / minDist * pushStrength
			nx := dx / dist
			nz := dz / dist

			velAX := int16(-nx * force * velocityScale)
			velAZ := int16(-nz * force * velocityScale)
			velBX := int16(nx * force * velocityScale)
			velBZ := int16(nz * force * velocityScale)

			a.player.WritePacket(pk.Marshal(
				packetid.ClientboundSetEntityMotion,
				pk.VarInt(a.eid),
				pk.Short(velAX),
				pk.Short(0),
				pk.Short(velAZ),
			))
			b.player.WritePacket(pk.Marshal(
				packetid.ClientboundSetEntityMotion,
				pk.VarInt(b.eid),
				pk.Short(velBX),
				pk.Short(0),
				pk.Short(velBZ),
			))
		}
	}
}

// pushMobsAndPlayers handles mob-to-player and mob-to-mob collisions.
func (c *CollisionManager) pushMobsAndPlayers(players []collisionSnap) {
	c.MobManager.mu.Lock()
	defer c.MobManager.mu.Unlock()

	minDist := (playerHitboxWidth + mobDefaultHitboxWidth) / 2

	checkedMobs := make(map[int32]bool)

	for _, ps := range players {
		candidates := c.MobManager.Spatial.QueryRange(ps.x, ps.z, collisionQueryRadius)
		for _, mobEID := range candidates {
			mob := c.MobManager.Mobs[mobEID]
			if mob == nil || mob.Health <= 0 {
				continue
			}

			dx := ps.x - mob.X
			dz := ps.z - mob.Z
			dist := math.Sqrt(dx*dx + dz*dz)
			if dist < minDist && dist >= 1e-6 {
				force := (minDist - dist) / minDist * pushStrength
				nx := dx / dist
				nz := dz / dist

				velX := int16(nx * force * velocityScale)
				velZ := int16(nz * force * velocityScale)
				ps.player.WritePacket(pk.Marshal(
					packetid.ClientboundSetEntityMotion,
					pk.VarInt(ps.eid),
					pk.Short(velX),
					pk.Short(0),
					pk.Short(velZ),
				))

				mobVelX := int16(-nx * force * velocityScale)
				mobVelZ := int16(-nz * force * velocityScale)
				mobPkt := pk.Marshal(
					packetid.ClientboundSetEntityMotion,
					pk.VarInt(mobEID),
					pk.Short(mobVelX),
					pk.Short(0),
					pk.Short(mobVelZ),
				)
				c.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
					p.WritePacket(mobPkt)
				})

				mob.X -= nx * force
				mob.Z -= nz * force
			}

			if !checkedMobs[mobEID] {
				checkedMobs[mobEID] = true
				c.pushMobPairs(mobEID, mob, candidates)
			}
		}
	}
}

// pushMobPairs pushes apart mobs that overlap with the given mob.
func (c *CollisionManager) pushMobPairs(eid int32, mob *Mob, neighbors []int32) {
	minDist := mobDefaultHitboxWidth
	for _, otherEID := range neighbors {
		if otherEID == eid {
			continue
		}
		other := c.MobManager.Mobs[otherEID]
		if other == nil || other.Health <= 0 {
			continue
		}

		dx := other.X - mob.X
		dz := other.Z - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist >= minDist || dist < 1e-6 {
			continue
		}

		force := (minDist - dist) / minDist * pushStrength * 0.5
		nx := dx / dist
		nz := dz / dist

		mob.X -= nx * force
		mob.Z -= nz * force
		other.X += nx * force
		other.Z += nz * force

		velA := pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(eid),
			pk.Short(int16(-nx*force*velocityScale)),
			pk.Short(0),
			pk.Short(int16(-nz*force*velocityScale)),
		)
		velB := pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(otherEID),
			pk.Short(int16(nx*force*velocityScale)),
			pk.Short(0),
			pk.Short(int16(nz*force*velocityScale)),
		)
		midX := (mob.X + other.X) / 2
		midZ := (mob.Z + other.Z) / 2
		c.Manager.ForEachNearby(midX, midZ, PlayerTrackingRange, func(p *game.Player) {
			p.WritePacket(velA)
			p.WritePacket(velB)
		})
	}
}
