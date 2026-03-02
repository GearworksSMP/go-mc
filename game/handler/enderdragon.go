package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// MobTypeEnderDragon is the entity type ID for the ender dragon.
const MobTypeEnderDragon int32 = 28

// Dragon flight phases.
const (
	DragonPhaseCircling  = 0
	DragonPhaseStrafing  = 1
	DragonPhasePerching  = 2
	DragonPhaseTakeoff   = 3
)

// EnderDragonManager manages the ender dragon boss entity in the End dimension.
type EnderDragonManager struct {
	mu sync.Mutex

	World      game.World // End world
	Manager    *game.PlayerManager
	MobManager *MobManager
	Survival   *SurvivalHandler
	Logger     *log.Logger

	// Dragon state
	DragonEID   int32
	DragonUUID  uuid.UUID
	BossBarUUID uuid.UUID
	Alive       bool
	Killed      bool // true after the dragon has been killed (return portal placed)
	Health      float32
	MaxHealth   float32

	// Position and movement
	X, Y, Z    float64
	Yaw, Pitch float32

	// AI state
	Phase        int
	PhaseTick    int64 // tick when current phase started
	CircleAngle  float64
	PerchTimer   int64 // ticks until next perch
	StrafeTick   int64 // ticks into strafing
	StrafeTarget *game.Player
}

// NewEnderDragonManager creates a new ender dragon manager.
func NewEnderDragonManager(world game.World, manager *game.PlayerManager, mobMgr *MobManager, survival *SurvivalHandler, logger *log.Logger) *EnderDragonManager {
	return &EnderDragonManager{
		World:      world,
		Manager:    manager,
		MobManager: mobMgr,
		Survival:   survival,
		Logger:     logger,
		MaxHealth:  200,
		BossBarUUID: uuid.New(),
	}
}

// SpawnDragon spawns the ender dragon when a player enters the End.
func (dm *EnderDragonManager) SpawnDragon() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.Alive || dm.Killed {
		return
	}

	dm.DragonEID = dm.Manager.NextEntityID()
	dm.DragonUUID = uuid.New()
	dm.Alive = true
	dm.Health = dm.MaxHealth
	dm.X = 0
	dm.Y = 70
	dm.Z = 0
	dm.Phase = DragonPhaseCircling
	dm.CircleAngle = 0
	dm.PerchTimer = 1200 // ~60 seconds until first perch

	// Broadcast AddEntity for the dragon
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(dm.DragonEID),
		pk.UUID(dm.DragonUUID),
		pk.VarInt(MobTypeEnderDragon),
		pk.Double(dm.X),
		pk.Double(dm.Y),
		pk.Double(dm.Z),
		pk.UnsignedByte(0), // velocity encoding
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(pkt)
		}
	})

	// Send boss bar to all End players
	dm.sendBossBarAdd()

	dm.logf("Ender Dragon spawned (EID=%d)", dm.DragonEID)
}

// Tick updates the ender dragon AI. Called every server tick.
func (dm *EnderDragonManager) Tick(tick int64) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.Alive {
		return
	}

	// Check if any players are in the End
	hasEndPlayers := false
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" && !p.Dead {
			hasEndPlayers = true
		}
	})
	if !hasEndPlayers {
		return
	}

	switch dm.Phase {
	case DragonPhaseCircling:
		dm.tickCircling(tick)
	case DragonPhaseStrafing:
		dm.tickStrafing(tick)
	case DragonPhasePerching:
		dm.tickPerching(tick)
	case DragonPhaseTakeoff:
		dm.tickTakeoff(tick)
	}

	// Broadcast position update
	dm.broadcastPosition()

	// Check for melee damage from nearby players when perching
	if dm.Phase == DragonPhasePerching {
		dm.checkMeleeDamage()
	}

	// Update boss bar progress
	if tick%20 == 0 {
		dm.sendBossBarProgress()
	}
}

// tickCircling makes the dragon fly in circles around the origin.
func (dm *EnderDragonManager) tickCircling(tick int64) {
	dm.CircleAngle += 0.015 // radians per tick (~5.5 degrees/sec)
	if dm.CircleAngle > 2*math.Pi {
		dm.CircleAngle -= 2 * math.Pi
	}

	radius := 80.0
	targetX := math.Cos(dm.CircleAngle) * radius
	targetZ := math.Sin(dm.CircleAngle) * radius
	targetY := 70.0

	// Smoothly move toward target
	speed := 1.5
	dx := targetX - dm.X
	dz := targetZ - dm.Z
	dy := targetY - dm.Y
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	if dist > speed {
		dm.X += dx / dist * speed
		dm.Y += dy / dist * speed
		dm.Z += dz / dist * speed
	} else {
		dm.X = targetX
		dm.Y = targetY
		dm.Z = targetZ
	}

	dm.Yaw = float32(math.Atan2(dz, dx) * 180 / math.Pi)

	// Transition to strafing occasionally
	dm.PerchTimer--
	if dm.PerchTimer <= 0 {
		dm.Phase = DragonPhasePerching
		dm.PhaseTick = tick
		dm.PerchTimer = 1200 + int64(rand.Intn(600)) // 60-90 seconds
		dm.logf("Dragon entering perch phase")
		return
	}

	// Strafe dive toward nearest player every ~15 seconds
	if tick%300 == 0 {
		dm.StrafeTarget = dm.findNearestEndPlayer()
		if dm.StrafeTarget != nil {
			dm.Phase = DragonPhaseStrafing
			dm.PhaseTick = tick
			dm.StrafeTick = 0
			dm.logf("Dragon strafing toward %s", dm.StrafeTarget.Name)
		}
	}
}

// tickStrafing makes the dragon dive toward a target player and shoot a fireball.
func (dm *EnderDragonManager) tickStrafing(tick int64) {
	dm.StrafeTick++

	if dm.StrafeTarget == nil || dm.StrafeTarget.Dead || dm.StrafeTarget.Dimension != "minecraft:the_end" {
		dm.Phase = DragonPhaseCircling
		dm.PhaseTick = tick
		return
	}

	// Dive toward the player
	px, py, pz := dm.StrafeTarget.Position()
	dx := px - dm.X
	dy := (py + 2) - dm.Y // aim slightly above player
	dz := pz - dm.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	speed := 2.0
	if dist > speed {
		dm.X += dx / dist * speed
		dm.Y += dy / dist * speed
		dm.Z += dz / dist * speed
	}

	dm.Yaw = float32(math.Atan2(dz, dx) * 180 / math.Pi)

	// Deal damage in a 4-block radius when close
	if dist < 6 {
		dm.dealAreaDamage(6.0, 4.0)
	}

	// Return to circling after 3 seconds
	if dm.StrafeTick >= 60 {
		dm.Phase = DragonPhaseCircling
		dm.PhaseTick = tick
	}
}

// tickPerching makes the dragon land at the origin and stay vulnerable.
func (dm *EnderDragonManager) tickPerching(tick int64) {
	// Move toward origin at Y=64
	targetX := 0.0
	targetY := 66.0
	targetZ := 0.0

	dx := targetX - dm.X
	dy := targetY - dm.Y
	dz := targetZ - dm.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	speed := 1.0
	if dist > speed {
		dm.X += dx / dist * speed
		dm.Y += dy / dist * speed
		dm.Z += dz / dist * speed
	}

	// Stay perched for 5 seconds (100 ticks) after arriving
	elapsed := tick - dm.PhaseTick
	if elapsed > 200 { // 10 seconds total (including flight time)
		dm.Phase = DragonPhaseTakeoff
		dm.PhaseTick = tick
		dm.logf("Dragon taking off from perch")
	}
}

// tickTakeoff makes the dragon fly back up to circling altitude.
func (dm *EnderDragonManager) tickTakeoff(tick int64) {
	targetY := 70.0
	dy := targetY - dm.Y
	speed := 1.5

	if math.Abs(dy) > speed {
		dm.Y += speed
	} else {
		dm.Y = targetY
		dm.Phase = DragonPhaseCircling
		dm.PhaseTick = tick
	}

	// Also move outward from origin
	dm.CircleAngle += 0.02
	dm.X = math.Cos(dm.CircleAngle) * 40
	dm.Z = math.Sin(dm.CircleAngle) * 40
}

// DamageDragon deals damage to the dragon from a player attack.
func (dm *EnderDragonManager) DamageDragon(player *game.Player, damage float32) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.Alive {
		return
	}

	dm.Health -= damage
	if dm.Health < 0 {
		dm.Health = 0
	}

	dm.logf("Dragon took %.1f damage from %s (health=%.1f)", damage, player.Name, dm.Health)

	// Play hurt sound
	BroadcastSound(dm.Manager, SoundDragonHurt, SoundCategoryHostile, dm.X, dm.Y, dm.Z, 1.0, 1.0)

	// Send entity event (hurt animation)
	hurtPkt := pk.Marshal(
		packetid.ClientboundEntityEvent,
		pk.Int(dm.DragonEID),
		pk.Byte(2), // hurt animation
	)
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(hurtPkt)
		}
	})

	if dm.Health <= 0 {
		dm.killDragon(player)
	}
}

// IsDragonEID returns true if the given entity ID belongs to the ender dragon.
func (dm *EnderDragonManager) IsDragonEID(eid int32) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.Alive && dm.DragonEID == eid
}

// killDragon handles dragon death: XP, return portal, boss bar removal.
func (dm *EnderDragonManager) killDragon(killer *game.Player) {
	dm.Alive = false
	dm.Killed = true

	dm.logf("Ender Dragon killed by %s!", killer.Name)

	// Play death sound
	BroadcastSound(dm.Manager, SoundDragonDeath, SoundCategoryHostile, dm.X, dm.Y, dm.Z, 1.0, 1.0)

	// Remove dragon entity
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(dm.DragonEID),
	)
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(removePkt)
		}
	})

	// Remove boss bar
	dm.sendBossBarRemove()

	// Award XP to killer (12000 XP total in vanilla, simplified here)
	killer.ExperienceTotal += 12000
	killer.ExperienceLevel += 68 // approximate levels for 12000 XP
	killer.Experience = 0
	SendExperience(killer)

	// Place return portal at origin
	dm.placeReturnPortal()
}

// placeReturnPortal creates the end return portal at the origin (0, 64, 0).
func (dm *EnderDragonManager) placeReturnPortal() {
	bedrockID, _ := block.ToStateID[block.Bedrock{}]
	portalID, _ := block.ToStateID[block.EndPortal{}]
	dragonEggID, _ := block.ToStateID[block.DragonEgg{}]

	// Bedrock pillar from Y=64 to Y=68
	for y := 64; y <= 68; y++ {
		dm.World.SetBlock(0, y, 0, bedrockID)
		broadcastBlockUpdateStatic(dm.Manager, 0, y, 0, int32(bedrockID))
	}

	// 3x3 bedrock base at Y=64 (extend to 5x5 edges)
	for dz := -2; dz <= 2; dz++ {
		for dx := -2; dx <= 2; dx++ {
			if dx == 0 && dz == 0 {
				continue // already placed center
			}
			// Only place corners/edges at the right pattern
			dist := abs(dx) + abs(dz)
			if dist <= 3 {
				dm.World.SetBlock(dx, 64, dz, bedrockID)
				broadcastBlockUpdateStatic(dm.Manager, dx, 64, dz, int32(bedrockID))
			}
		}
	}

	// End portal blocks in the 3x3 area around center at Y=64
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dz == 0 {
				continue // center is bedrock pillar
			}
			dm.World.SetBlock(dx, 64, dz, portalID)
			broadcastBlockUpdateStatic(dm.Manager, dx, 64, dz, int32(portalID))
		}
	}

	// Dragon egg on top of the bedrock pillar
	if dragonEggID != 0 {
		dm.World.SetBlock(0, 69, 0, dragonEggID)
		broadcastBlockUpdateStatic(dm.Manager, 0, 69, 0, int32(dragonEggID))
	}

	dm.logf("Return portal placed at origin")
}

// dealAreaDamage deals damage to all End players within the given radius.
func (dm *EnderDragonManager) dealAreaDamage(damage float32, radius float64) {
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension != "minecraft:the_end" || p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - dm.X
		dy := py - dm.Y
		dz := pz - dm.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < radius {
			if dm.Survival != nil {
				p.LastDamageMessage = p.Name + " was roasted in dragon's breath"
				dm.Survival.ApplyDamage(dm.Manager, p, damage, dm.Survival.AttackDamageTypeID)
			}
		}
	})
}

// checkMeleeDamage checks if any player is close enough to deal melee damage while perching.
// This is not the player attacking the dragon - that's handled via CombatHandler.
// This deals contact damage from the dragon to nearby players.
func (dm *EnderDragonManager) checkMeleeDamage() {
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension != "minecraft:the_end" || p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - dm.X
		dy := py - dm.Y
		dz := pz - dm.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < 5 {
			// Contact damage
			if dm.Survival != nil {
				p.LastDamageMessage = p.Name + " was squashed by Ender Dragon"
				dm.Survival.ApplyDamage(dm.Manager, p, 5, dm.Survival.AttackDamageTypeID)
			}
		}
	})
}

// findNearestEndPlayer returns the nearest living player in the End, or nil.
func (dm *EnderDragonManager) findNearestEndPlayer() *game.Player {
	var nearest *game.Player
	nearestDist := math.MaxFloat64

	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension != "minecraft:the_end" || p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - dm.X
		dy := py - dm.Y
		dz := pz - dm.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < nearestDist {
			nearestDist = dist
			nearest = p
		}
	})

	return nearest
}

// broadcastPosition sends entity teleport to all End players.
func (dm *EnderDragonManager) broadcastPosition() {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(dm.DragonEID),
		pk.Double(dm.X),
		pk.Double(dm.Y),
		pk.Double(dm.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Angle(degToAngle(dm.Yaw)),
		pk.Angle(degToAngle(dm.Pitch)),
		pk.Boolean(false), // on ground
	)
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(pkt)
		}
	})

	// Also send head rotation
	headPkt := pk.Marshal(
		packetid.ClientboundRotateHead,
		pk.VarInt(dm.DragonEID),
		pk.Angle(degToAngle(dm.Yaw)),
	)
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(headPkt)
		}
	})
}

// sendBossBarAdd sends the boss bar "add" action to all End players.
func (dm *EnderDragonManager) sendBossBarAdd() {
	title := chat.Message{Text: "Ender Dragon", Color: "light_purple", Bold: true}

	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.DragonBossBarID = dm.BossBarUUID
			p.WritePacket(pk.Marshal(
				packetid.ClientboundBossEvent,
				pk.UUID(dm.BossBarUUID),
				pk.VarInt(0), // action: add
				title,
				pk.Float(dm.Health/dm.MaxHealth), // progress
				pk.VarInt(2),                      // color: pink
				pk.VarInt(0),                      // division: no notches
				pk.UnsignedByte(0),                // flags
			))
		}
	})
}

// sendBossBarProgress updates the boss bar health progress for all End players.
func (dm *EnderDragonManager) sendBossBarProgress() {
	progress := dm.Health / dm.MaxHealth
	if progress < 0 {
		progress = 0
	}

	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(pk.Marshal(
				packetid.ClientboundBossEvent,
				pk.UUID(dm.BossBarUUID),
				pk.VarInt(2), // action: update progress
				pk.Float(progress),
			))
		}
	})
}

// sendBossBarRemove removes the boss bar from all players.
func (dm *EnderDragonManager) sendBossBarRemove() {
	dm.Manager.ForEach(func(p *game.Player) {
		if p.Dimension == "minecraft:the_end" {
			p.WritePacket(pk.Marshal(
				packetid.ClientboundBossEvent,
				pk.UUID(dm.BossBarUUID),
				pk.VarInt(1), // action: remove
			))
			p.DragonBossBarID = uuid.UUID{}
		}
	})
}

// SendDragonToPlayer sends the dragon entity and boss bar to a newly arrived End player.
func (dm *EnderDragonManager) SendDragonToPlayer(player *game.Player) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.Alive {
		return
	}

	// Send AddEntity
	player.WritePacket(pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(dm.DragonEID),
		pk.UUID(dm.DragonUUID),
		pk.VarInt(MobTypeEnderDragon),
		pk.Double(dm.X),
		pk.Double(dm.Y),
		pk.Double(dm.Z),
		pk.UnsignedByte(0),
		pk.Angle(degToAngle(dm.Pitch)),
		pk.Angle(degToAngle(dm.Yaw)),
		pk.Angle(degToAngle(dm.Yaw)),
		pk.VarInt(0),
	))

	// Send boss bar
	title := chat.Message{Text: "Ender Dragon", Color: "light_purple", Bold: true}
	player.DragonBossBarID = dm.BossBarUUID
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBossEvent,
		pk.UUID(dm.BossBarUUID),
		pk.VarInt(0), // add
		title,
		pk.Float(dm.Health/dm.MaxHealth),
		pk.VarInt(2),       // pink
		pk.VarInt(0),       // no notches
		pk.UnsignedByte(0), // flags
	))
}

// RemoveBossBarFromPlayer removes the boss bar from a player leaving the End.
func (dm *EnderDragonManager) RemoveBossBarFromPlayer(player *game.Player) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if player.DragonBossBarID != (uuid.UUID{}) {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundBossEvent,
			pk.UUID(dm.BossBarUUID),
			pk.VarInt(1), // remove
		))
		player.DragonBossBarID = uuid.UUID{}
	}
}

// Dragon sound constants.
const (
	SoundDragonAmbient int32 = 323 // entity.ender_dragon.ambient
	SoundDragonDeath   int32 = 324 // entity.ender_dragon.death
	SoundDragonHurt    int32 = 327 // entity.ender_dragon.hurt
	SoundDragonGrowl   int32 = 326 // entity.ender_dragon.growl
	SoundDragonFlap    int32 = 325 // entity.ender_dragon.flap
)

func (dm *EnderDragonManager) logf(format string, args ...any) {
	if dm.Logger != nil {
		dm.Logger.Printf(format, args...)
	}
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
