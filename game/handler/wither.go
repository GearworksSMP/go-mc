package handler

import (
	"log"
	"math"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Wither entity and projectile type IDs (26.1-snapshot-2 registry, 0-indexed).
const (
	MobTypeWither      int32 = 125
	MobTypeWitherSkull int32 = 127
)

// Wither sound event IDs.
const (
	SoundWitherAmbient int32 = 1538
	SoundWitherDeath   int32 = 1540
	SoundWitherHurt    int32 = 1541
	SoundWitherShoot   int32 = 1542
	SoundWitherSpawn   int32 = 1547
)

// Wither boss phases.
const (
	WitherPhaseInvulnerable = 0
	WitherPhaseAttack       = 1
	WitherPhaseArmored      = 2 // health ≤ 150, 50% damage reduction
)

// ItemNetherStar is the item ID for nether_star.
const ItemNetherStar int32 = 1110

// WitherBoss represents a single wither boss entity.
type WitherBoss struct {
	EID         int32
	UUID        uuid.UUID
	BossBarUUID uuid.UUID
	Health      float32
	MaxHealth   float32
	X, Y, Z    float64
	Yaw, Pitch  float32
	Phase       int
	PhaseTick   int64 // tick when current phase started
	InvulnTicks int32 // countdown for invulnerability phase
	ShootTimer  int32 // ticks until next skull shot
	Alive       bool
}

// WitherSkull represents a wither skull projectile.
type WitherSkull struct {
	EID        int32
	UUID       uuid.UUID
	X, Y, Z   float64
	VX, VY, VZ float64 // velocity per tick
	LifeTicks  int32
	OwnerEID   int32 // wither that shot it
}

// WitherManager manages wither boss entities.
type WitherManager struct {
	mu sync.Mutex

	World      game.World
	Manager    *game.PlayerManager
	Survival   *SurvivalHandler
	EffectMgr  *EffectManager
	ItemMgr    *ItemEntityManager
	Logger     *log.Logger

	Withers map[int32]*WitherBoss  // keyed by EID
	Skulls  map[int32]*WitherSkull // keyed by EID
}

// NewWitherManager creates a new WitherManager.
func NewWitherManager(world game.World, manager *game.PlayerManager, survival *SurvivalHandler, effectMgr *EffectManager, itemMgr *ItemEntityManager, logger *log.Logger) *WitherManager {
	return &WitherManager{
		World:     world,
		Manager:   manager,
		Survival:  survival,
		EffectMgr: effectMgr,
		ItemMgr:   itemMgr,
		Logger:    logger,
		Withers:   make(map[int32]*WitherBoss),
		Skulls:    make(map[int32]*WitherSkull),
	}
}

// CheckWitherSummon checks if placing a wither_skeleton_skull at (x,y,z) completes
// the T-shape summoning pattern. Called from block placement handler.
func (wm *WitherManager) CheckWitherSummon(x, y, z int) {
	// The T-shape pattern (skulls on top, soul_sand body):
	//    S S S    (y)
	//    B B B    (y-1)
	//      B      (y-2)
	// Check both orientations: E-W (along X) and N-S (along Z).

	// The placed skull could be at any of the 3 skull positions.
	// Try each possibility.

	for _, dx := range []int{-1, 0, 1} {
		cx := x - dx // center X of the skull row
		if wm.checkPattern(cx, y, z, true) { // E-W orientation
			wm.summonWither(cx, y, z, true)
			return
		}
	}

	for _, dz := range []int{-1, 0, 1} {
		cz := z - dz // center Z of the skull row
		if wm.checkPattern(x, y, cz, false) { // N-S orientation
			wm.summonWither(x, y, cz, false)
			return
		}
	}
}

// checkPattern checks if the T-shape pattern exists at the given center position.
// eastWest=true means skulls along X axis, false means along Z axis.
func (wm *WitherManager) checkPattern(cx, sy, cz int, eastWest bool) bool {
	// sy = skull Y level, sy-1 = soul sand arm, sy-2 = soul sand pillar

	// Check 3 skulls at (cx±offset, sy, cz) or (cx, sy, cz±offset)
	for _, offset := range []int{-1, 0, 1} {
		var sx, sz int
		if eastWest {
			sx, sz = cx+offset, cz
		} else {
			sx, sz = cx, cz+offset
		}
		if !wm.isWitherSkull(sx, sy, sz) {
			return false
		}
	}

	// Check 3 soul_sand at arm level (sy-1)
	for _, offset := range []int{-1, 0, 1} {
		var bx, bz int
		if eastWest {
			bx, bz = cx+offset, cz
		} else {
			bx, bz = cx, cz+offset
		}
		if !wm.isSoulSand(bx, sy-1, bz) {
			return false
		}
	}

	// Check soul_sand pillar at (cx, sy-2, cz)
	if !wm.isSoulSand(cx, sy-2, cz) {
		return false
	}

	return true
}

func (wm *WitherManager) isWitherSkull(x, y, z int) bool {
	state, err := wm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	return name == "wither_skeleton_skull" || name == "wither_skeleton_wall_skull"
}

func (wm *WitherManager) isSoulSand(x, y, z int) bool {
	state, err := wm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	return BlockNameFromState(int(state)) == "soul_sand"
}

// summonWither removes the pattern blocks and spawns a wither.
func (wm *WitherManager) summonWither(cx, sy, cz int, eastWest bool) {
	airID, _ := block.ToStateID[block.Air{}]

	// Remove skulls
	for _, offset := range []int{-1, 0, 1} {
		var sx, sz int
		if eastWest {
			sx, sz = cx+offset, cz
		} else {
			sx, sz = cx, cz+offset
		}
		wm.World.SetBlock(sx, sy, sz, airID)
		broadcastBlockUpdateStatic(wm.Manager, sx, sy, sz, int32(airID))
	}

	// Remove soul_sand arm
	for _, offset := range []int{-1, 0, 1} {
		var bx, bz int
		if eastWest {
			bx, bz = cx+offset, cz
		} else {
			bx, bz = cx, cz+offset
		}
		wm.World.SetBlock(bx, sy-1, bz, airID)
		broadcastBlockUpdateStatic(wm.Manager, bx, sy-1, bz, int32(airID))
	}

	// Remove soul_sand pillar
	wm.World.SetBlock(cx, sy-2, cz, airID)
	broadcastBlockUpdateStatic(wm.Manager, cx, sy-2, cz, int32(airID))

	// Spawn wither at the center of the pattern
	spawnX := float64(cx) + 0.5
	spawnY := float64(sy) + 1.0 // above the skull position
	spawnZ := float64(cz) + 0.5

	wm.SpawnWither(spawnX, spawnY, spawnZ)
}

// SpawnWither spawns a wither boss at the given position.
func (wm *WitherManager) SpawnWither(x, y, z float64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	eid := wm.Manager.NextEntityID()
	witherUUID := uuid.New()
	bossBarUUID := uuid.New()

	wither := &WitherBoss{
		EID:         eid,
		UUID:        witherUUID,
		BossBarUUID: bossBarUUID,
		Health:      300,
		MaxHealth:   300,
		X:           x,
		Y:           y,
		Z:           z,
		Phase:       WitherPhaseInvulnerable,
		InvulnTicks: 200, // 10 seconds invulnerability
		ShootTimer:  0,
		Alive:       true,
	}
	wm.Withers[eid] = wither

	// Broadcast AddEntity
	spawnPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(witherUUID),
		pk.VarInt(MobTypeWither),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0), // velocity encoding
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(spawnPkt)
	})

	// Play spawn sound
	BroadcastSound(wm.Manager, SoundWitherSpawn, SoundCategoryHostile, x, y, z, 1.0, 1.0)

	// Send boss bar to all players
	wm.sendBossBarAdd(wither)

	wm.logf("Wither spawned at (%.1f, %.1f, %.1f) EID=%d", x, y, z, eid)
}

// Tick updates all wither bosses. Called every server tick.
func (wm *WitherManager) Tick(tick int64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for _, w := range wm.Withers {
		if !w.Alive {
			continue
		}
		wm.tickWither(w, tick)
	}

	wm.tickSkulls(tick)

	// Cleanup dead withers
	for eid, w := range wm.Withers {
		if !w.Alive {
			delete(wm.Withers, eid)
		}
	}
}

func (wm *WitherManager) tickWither(w *WitherBoss, tick int64) {
	switch w.Phase {
	case WitherPhaseInvulnerable:
		wm.tickInvulnerable(w, tick)
	case WitherPhaseAttack:
		wm.tickAttack(w, tick)
	case WitherPhaseArmored:
		wm.tickArmored(w, tick)
	}

	// Ambient sound every ~3 seconds
	if tick%60 == 0 {
		BroadcastSound(wm.Manager, SoundWitherAmbient, SoundCategoryHostile, w.X, w.Y, w.Z, 1.0, 1.0)
	}

	// Update boss bar every second
	if tick%20 == 0 {
		wm.sendBossBarProgress(w)
	}

	// Broadcast position
	wm.broadcastPosition(w)
}

func (wm *WitherManager) tickInvulnerable(w *WitherBoss, tick int64) {
	w.InvulnTicks--

	// Rise slowly during invulnerability
	w.Y += 0.1

	if w.InvulnTicks <= 0 {
		w.Phase = WitherPhaseAttack
		w.PhaseTick = tick
		w.ShootTimer = 40

		// Explosion effect at end of invulnerability
		wm.broadcastExplosion(w.X, w.Y, w.Z, 7.0)

		wm.logf("Wither entered attack phase (EID=%d)", w.EID)
	}
}

func (wm *WitherManager) tickAttack(w *WitherBoss, tick int64) {
	// Check phase transition
	if w.Health <= 150 {
		w.Phase = WitherPhaseArmored
		w.PhaseTick = tick
		wm.logf("Wither entered armored phase (EID=%d, health=%.1f)", w.EID, w.Health)
	}

	wm.witherFlyAndShoot(w, tick)
}

func (wm *WitherManager) tickArmored(w *WitherBoss, tick int64) {
	// In armored phase, wither flies lower and is more aggressive
	wm.witherFlyAndShoot(w, tick)
}

// witherFlyAndShoot handles wither movement and skull shooting.
func (wm *WitherManager) witherFlyAndShoot(w *WitherBoss, tick int64) {
	// Find nearest player
	target := wm.findNearestPlayer(w)
	if target == nil {
		return
	}

	px, py, pz := target.Position()

	// Fly toward player but stay ~8 blocks above
	targetY := py + 8
	dx := px - w.X
	dy := targetY - w.Y
	dz := pz - w.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	speed := 0.6
	if dist > speed {
		w.X += dx / dist * speed
		w.Y += dy / dist * speed
		w.Z += dz / dist * speed
	}

	w.Yaw = float32(math.Atan2(dz, dx)*180/math.Pi) - 90

	// Shoot skulls
	w.ShootTimer--
	if w.ShootTimer <= 0 {
		w.ShootTimer = 40 // shoot every 2 seconds
		if w.Phase == WitherPhaseArmored {
			w.ShootTimer = 20 // faster in armored phase
		}
		wm.shootSkull(w, target)
	}
}

func (wm *WitherManager) shootSkull(w *WitherBoss, target *game.Player) {
	px, py, pz := target.Position()
	dx := px - w.X
	dy := (py + 1) - w.Y // aim at body
	dz := pz - w.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist < 0.1 {
		return
	}

	speed := 0.8
	vx := dx / dist * speed
	vy := dy / dist * speed
	vz := dz / dist * speed

	eid := wm.Manager.NextEntityID()
	skullUUID := uuid.New()

	skull := &WitherSkull{
		EID:       eid,
		UUID:      skullUUID,
		X:         w.X,
		Y:         w.Y + 2, // shoot from head height
		Z:         w.Z,
		VX:        vx,
		VY:        vy,
		VZ:        vz,
		LifeTicks: 100, // 5 seconds max
		OwnerEID:  w.EID,
	}
	wm.Skulls[eid] = skull

	// Broadcast skull spawn
	spawnPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(skullUUID),
		pk.VarInt(MobTypeWitherSkull),
		pk.Double(skull.X),
		pk.Double(skull.Y),
		pk.Double(skull.Z),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.VarInt(0),
	)
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(spawnPkt)
	})

	BroadcastSound(wm.Manager, SoundWitherShoot, SoundCategoryHostile, w.X, w.Y, w.Z, 1.0, 1.0)
}

func (wm *WitherManager) tickSkulls(tick int64) {
	var expired []int32

	for eid, skull := range wm.Skulls {
		skull.X += skull.VX
		skull.Y += skull.VY
		skull.Z += skull.VZ
		skull.LifeTicks--

		if skull.LifeTicks <= 0 {
			expired = append(expired, eid)
			continue
		}

		// Check collision with players
		hit := false
		wm.Manager.ForEach(func(p *game.Player) {
			if hit || p.Dead || p.IsInvulnerable() {
				return
			}
			px, py, pz := p.Position()
			dx := px - skull.X
			dy := (py + 1) - skull.Y
			dz := pz - skull.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < 1.5 {
				hit = true
				// Deal damage
				if wm.Survival != nil {
					p.LastDamageMessage = p.Name + " was shot by a wither skull"
					wm.Survival.ApplyDamage(wm.Manager, p, 8, wm.Survival.MobDamageTypeID)
				}
				// Apply Wither II effect for 10 seconds (200 ticks)
				if wm.EffectMgr != nil {
					wm.EffectMgr.ApplyEffect(p, EffectWither, 1, 200, false)
				}
			}
		})

		if hit {
			expired = append(expired, eid)
			// Small explosion effect
			wm.broadcastExplosion(skull.X, skull.Y, skull.Z, 1.0)
			continue
		}

		// Check collision with blocks
		state, err := wm.World.GetBlock(int(math.Floor(skull.X)), int(math.Floor(skull.Y)), int(math.Floor(skull.Z)))
		if err == nil && state != 0 {
			name := BlockNameFromState(int(state))
			if name != "air" && name != "cave_air" && name != "void_air" {
				expired = append(expired, eid)
				wm.broadcastExplosion(skull.X, skull.Y, skull.Z, 1.0)
				continue
			}
		}

		// Broadcast position update
		pkt := pk.Marshal(
			packetid.ClientboundTeleportEntity,
			pk.VarInt(skull.EID),
			pk.Double(skull.X),
			pk.Double(skull.Y),
			pk.Double(skull.Z),
			pk.Double(skull.VX), pk.Double(skull.VY), pk.Double(skull.VZ),
			pk.Float(0), pk.Float(0),
			pk.Int(0),
			pk.Boolean(false),
		)
		wm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}

	// Remove expired skulls
	for _, eid := range expired {
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(eid),
		)
		wm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})
		delete(wm.Skulls, eid)
	}
}

// DamageWither deals damage to a wither from a player attack.
func (wm *WitherManager) DamageWither(eid int32, player *game.Player, damage float32) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.Withers[eid]
	if !ok || !w.Alive {
		return
	}

	// Invulnerable phase: no damage
	if w.Phase == WitherPhaseInvulnerable {
		return
	}

	// Armored phase: 50% damage reduction
	if w.Phase == WitherPhaseArmored {
		damage *= 0.5
	}

	w.Health -= damage
	if w.Health < 0 {
		w.Health = 0
	}

	wm.logf("Wither (EID=%d) took %.1f damage from %s (health=%.1f)", w.EID, damage, player.Name, w.Health)

	BroadcastSound(wm.Manager, SoundWitherHurt, SoundCategoryHostile, w.X, w.Y, w.Z, 1.0, 1.0)

	// Hurt animation
	hurtPkt := pk.Marshal(
		packetid.ClientboundEntityEvent,
		pk.Int(w.EID),
		pk.Byte(2),
	)
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(hurtPkt)
	})

	if w.Health <= 0 {
		wm.killWither(w, player)
	}
}

// IsWitherEID returns true if the given entity ID belongs to a living wither.
func (wm *WitherManager) IsWitherEID(eid int32) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	w, ok := wm.Withers[eid]
	return ok && w.Alive
}

func (wm *WitherManager) killWither(w *WitherBoss, killer *game.Player) {
	w.Alive = false

	wm.logf("Wither killed by %s!", killer.Name)

	BroadcastSound(wm.Manager, SoundWitherDeath, SoundCategoryHostile, w.X, w.Y, w.Z, 1.0, 1.0)

	// Remove wither entity
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(w.EID),
	)
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})

	// Remove boss bar
	wm.sendBossBarRemove(w)

	// Cosmetic explosion
	wm.broadcastExplosion(w.X, w.Y, w.Z, 7.0)

	// Drop nether_star
	if wm.ItemMgr != nil {
		wm.ItemMgr.SpawnItem(wm.Manager, w.X, w.Y, w.Z, ItemNetherStar, 1, 10)
	}

	// Award 50 XP
	killer.ExperienceTotal += 50
	killer.ExperienceLevel += 2
	killer.Experience = 0
	SendExperience(killer)
}

func (wm *WitherManager) findNearestPlayer(w *WitherBoss) *game.Player {
	var nearest *game.Player
	nearestDist := math.MaxFloat64

	wm.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - w.X
		dy := py - w.Y
		dz := pz - w.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < nearestDist && dist < 64 {
			nearestDist = dist
			nearest = p
		}
	})

	return nearest
}

// broadcastPosition sends entity teleport to all players.
func (wm *WitherManager) broadcastPosition(w *WitherBoss) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(w.EID),
		pk.Double(w.X),
		pk.Double(w.Y),
		pk.Double(w.Z),
		pk.Double(0), pk.Double(0), pk.Double(0),
		pk.Float(w.Yaw),
		pk.Float(w.Pitch),
		pk.Int(0),
		pk.Boolean(false),
	)
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SoundGenericExplode is the sound event ID for entity.generic.explode.
const SoundGenericExplode int32 = 576

// broadcastExplosion sends a cosmetic explosion effect (sound only).
func (wm *WitherManager) broadcastExplosion(x, y, z, power float64) {
	_ = power
	BroadcastSound(wm.Manager, SoundGenericExplode, SoundCategoryHostile, x, y, z, 1.0, 1.0)
}

// sendBossBarAdd sends the boss bar "add" action to all players.
func (wm *WitherManager) sendBossBarAdd(w *WitherBoss) {
	title := chat.Message{Text: "Wither", Bold: true}

	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundBossEvent,
			pk.UUID(w.BossBarUUID),
			pk.VarInt(0), // action: add
			title,
			pk.Float(w.Health/w.MaxHealth),
			pk.VarInt(5),       // color: purple
			pk.VarInt(0),       // division: no notches
			pk.UnsignedByte(0), // flags
		))
	})
}

// sendBossBarProgress updates the boss bar health progress.
func (wm *WitherManager) sendBossBarProgress(w *WitherBoss) {
	progress := w.Health / w.MaxHealth
	if progress < 0 {
		progress = 0
	}
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundBossEvent,
			pk.UUID(w.BossBarUUID),
			pk.VarInt(2), // action: update progress
			pk.Float(progress),
		))
	})
}

// sendBossBarRemove removes the boss bar from all players.
func (wm *WitherManager) sendBossBarRemove(w *WitherBoss) {
	wm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundBossEvent,
			pk.UUID(w.BossBarUUID),
			pk.VarInt(1), // action: remove
		))
	})
}

// SendWitherToPlayer sends all active wither entities and boss bars to a player.
func (wm *WitherManager) SendWitherToPlayer(player *game.Player) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for _, w := range wm.Withers {
		if !w.Alive {
			continue
		}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(w.EID),
			pk.UUID(w.UUID),
			pk.VarInt(MobTypeWither),
			pk.Double(w.X),
			pk.Double(w.Y),
			pk.Double(w.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(0),
		))

		title := chat.Message{Text: "Wither", Bold: true}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundBossEvent,
			pk.UUID(w.BossBarUUID),
			pk.VarInt(0), // add
			title,
			pk.Float(w.Health/w.MaxHealth),
			pk.VarInt(5),
			pk.VarInt(0),
			pk.UnsignedByte(0),
		))
	}
}

func (wm *WitherManager) logf(format string, args ...any) {
	if wm.Logger != nil {
		wm.Logger.Printf(format, args...)
	}
}
