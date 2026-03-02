package handler

import (
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Arrow entity type ID for AddEntity.
const arrowEntityType int32 = 3

// Arrow represents a flying arrow projectile.
type Arrow struct {
	EID                  int32
	ShooterEID           int32
	X, Y, Z              float64
	VelX, VelY, VelZ     float64
	Damage               float32
	LifeTick             int64
	Stuck                bool
}

// ArrowManager manages arrow projectiles.
type ArrowManager struct {
	Manager  *game.PlayerManager
	Survival *SurvivalHandler
	World    game.World
	mu       sync.Mutex
	Arrows   map[int32]*Arrow
}

// NewArrowManager creates a new ArrowManager.
func NewArrowManager(manager *game.PlayerManager, survival *SurvivalHandler, world game.World) *ArrowManager {
	return &ArrowManager{
		Manager:  manager,
		Survival: survival,
		World:    world,
		Arrows:   make(map[int32]*Arrow),
	}
}

// SpawnArrow creates and broadcasts a new arrow from shooter toward target.
func (am *ArrowManager) SpawnArrow(shooterEID int32, sx, sy, sz, tx, ty, tz float64, damage float32) {
	dx := tx - sx
	dy := ty - sy
	dz := tz - sz
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist < 0.01 {
		return
	}

	speed := 1.6
	velX := dx / dist * speed
	velY := dy / dist * speed
	velZ := dz / dist * speed

	eid := am.Manager.NextEntityID()
	arrow := &Arrow{
		EID:        eid,
		ShooterEID: shooterEID,
		X:          sx,
		Y:          sy,
		Z:          sz,
		VelX:       velX,
		VelY:       velY,
		VelZ:       velZ,
		Damage:     damage,
	}

	am.mu.Lock()
	am.Arrows[eid] = arrow
	am.mu.Unlock()

	// Broadcast spawn
	id := uuid.New()
	data := shooterEID + 1 // data field = shooter entity ID + 1
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(id),
		pk.VarInt(arrowEntityType),
		pk.Double(sx),
		pk.Double(sy),
		pk.Double(sz),
		pk.UnsignedByte(0), // LpVec3 zero velocity (handled by teleport)
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(data),
	)
	am.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SpawnPlayerArrow creates and broadcasts an arrow shot by a player with a given direction and damage.
func (am *ArrowManager) SpawnPlayerArrow(shooterEID int32, x, y, z, dirX, dirY, dirZ, damage float64) {
	speed := 3.0
	velX := dirX * speed
	velY := dirY * speed
	velZ := dirZ * speed

	eid := am.Manager.NextEntityID()
	arrow := &Arrow{
		EID:        eid,
		ShooterEID: shooterEID,
		X:          x,
		Y:          y,
		Z:          z,
		VelX:       velX,
		VelY:       velY,
		VelZ:       velZ,
		Damage:     float32(damage),
	}

	am.mu.Lock()
	am.Arrows[eid] = arrow
	am.mu.Unlock()

	// Broadcast spawn
	id := uuid.New()
	data := shooterEID + 1 // data field = shooter entity ID + 1
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(id),
		pk.VarInt(arrowEntityType),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0),
		pk.Angle(0), // pitch
		pk.Angle(0), // yaw
		pk.Angle(0), // head yaw
		pk.VarInt(data),
	)
	am.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// Tick processes all arrows (movement, collision, despawn).
func (am *ArrowManager) Tick(tick int64) {
	am.mu.Lock()
	defer am.mu.Unlock()

	var toRemove []int32

	for eid, arrow := range am.Arrows {
		if arrow.Stuck {
			arrow.LifeTick++
			if arrow.LifeTick > 1200 {
				toRemove = append(toRemove, eid)
			}
			continue
		}

		arrow.LifeTick++
		if arrow.LifeTick > 100 { // 5 second max flight
			toRemove = append(toRemove, eid)
			continue
		}

		// Move
		arrow.X += arrow.VelX
		arrow.Y += arrow.VelY
		arrow.Z += arrow.VelZ
		arrow.VelY -= 0.05 // gravity

		// Check block collision
		bx, by, bz := int(math.Floor(arrow.X)), int(math.Floor(arrow.Y)), int(math.Floor(arrow.Z))
		state, err := am.World.GetBlock(bx, by, bz)
		if err == nil && state != 0 {
			arrow.Stuck = true
			arrow.LifeTick = 0
			continue
		}

		// Check player collision
		hitPlayer := false
		deflected := false
		am.Manager.ForEach(func(p *game.Player) {
			if hitPlayer || deflected || p.Dead || p.IsInvulnerable() {
				return
			}
			// Skip the shooter (player arrows should not hit themselves)
			if p.EID == arrow.ShooterEID && arrow.LifeTick < 5 {
				return
			}
			px, py, pz := p.Position()
			dx := px - arrow.X
			dy := (py + 0.9) - arrow.Y // center of player
			dz := pz - arrow.Z
			d := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if d < 1.0 {
				// Shield deflection: if player is blocking and arrow is coming from the front
				if p.Blocking {
					// Compute player's look direction from yaw/pitch
					yaw, pitch := p.Rotation()
					yawRad := float64(yaw) * math.Pi / 180.0
					pitchRad := float64(pitch) * math.Pi / 180.0
					lookX := -math.Sin(yawRad) * math.Cos(pitchRad)
					lookZ := math.Cos(yawRad) * math.Cos(pitchRad)

					// Arrow direction (normalized)
					arrowSpeed := math.Sqrt(arrow.VelX*arrow.VelX + arrow.VelZ*arrow.VelZ)
					if arrowSpeed > 0.001 {
						arrowDirX := arrow.VelX / arrowSpeed
						arrowDirZ := arrow.VelZ / arrowSpeed

						// Dot product > 0 means arrow is coming from the front
						dot := arrowDirX*lookX + arrowDirZ*lookZ
						if dot > 0 {
							// Deflect: reverse arrow velocity
							arrow.VelX = -arrow.VelX
							arrow.VelY = -arrow.VelY * 0.5
							arrow.VelZ = -arrow.VelZ
							// Play shield block sound
							BroadcastSound(am.Manager, SoundShieldBlock, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
							// Reduce shield durability
							reduceShieldDurability(p)
							deflected = true
							return
						}
					}
				}

				// Calculate damage (2-5 for mob arrows, uncapped for player arrows)
				damage := arrow.Damage
				if damage < 2 {
					damage = 2
				}
				// Find shooter name for death message
				shooterName := "Skeleton"
				shooter := am.Manager.GetByEID(arrow.ShooterEID)
				if shooter != nil {
					shooterName = shooter.Name
					// Player arrows use full damage (already calculated with charge + enchants)
				} else {
					// Mob arrows: cap at 5
					if damage > 5 {
						damage = 5
					}
				}
				p.LastDamageMessage = p.Name + " was shot by " + shooterName
				am.Survival.ApplyDamage(am.Manager, p, damage, am.Survival.AttackDamageTypeID)
				hitPlayer = true
			}
		})
		if hitPlayer {
			toRemove = append(toRemove, eid)
			continue
		}
		if deflected {
			continue // arrow was deflected, keep it alive for next tick
		}

		// Broadcast position update
		am.broadcastArrowMove(arrow)
	}

	// Remove arrows
	for _, eid := range toRemove {
		am.removeArrow(eid)
	}
}

// broadcastArrowMove sends a teleport update for an arrow.
func (am *ArrowManager) broadcastArrowMove(arrow *Arrow) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(arrow.EID),
		pk.Double(arrow.X),
		pk.Double(arrow.Y),
		pk.Double(arrow.Z),
		pk.Double(arrow.VelX), pk.Double(arrow.VelY), pk.Double(arrow.VelZ),
		pk.Float(0), pk.Float(0),
		pk.Boolean(false),
	)
	am.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// removeArrow removes an arrow and broadcasts entity removal.
func (am *ArrowManager) removeArrow(eid int32) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	am.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
	delete(am.Arrows, eid)
}

// SendExistingArrows sends all current arrows to a newly joined player.
func (am *ArrowManager) SendExistingArrows(player *game.Player) {
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, arrow := range am.Arrows {
		id := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(arrow.EID),
			pk.UUID(id),
			pk.VarInt(arrowEntityType),
			pk.Double(arrow.X),
			pk.Double(arrow.Y),
			pk.Double(arrow.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(arrow.ShooterEID+1),
		))
	}
}
