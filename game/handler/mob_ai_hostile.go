package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/game"
)

// tickGuardian runs guardian AI: swims, beam attack.
func (m *MobManager) tickGuardian(mob *Mob, tick int64) {
	// Guardians are underwater mobs — don't apply normal gravity
	// Simple float behavior
	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 24.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		// Swim randomly
		mob.X += (rand.Float64() - 0.5) * 0.15
		mob.Z += (rand.Float64() - 0.5) * 0.15
		mob.Y += (rand.Float64() - 0.5) * 0.1
		m.broadcastMobMove(mob)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dy := py - mob.Y
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Swim toward target, maintain ~8 block distance
	if dist > 8 {
		mob.X += dx / dist * 0.15
		mob.Y += dy / dist * 0.1
		mob.Z += dz / dist * 0.15
	}

	m.broadcastMobMove(mob)

	// Beam attack: 2 seconds charge (40 ticks), 6 damage
	if nearestDist <= 15.0 && mob.ShootCooldown <= 0 {
		mob.ShootCooldown = 80 // 4 second total cycle

		damage := float32(6)
		if mob.TypeID == MobTypeElderGuardian {
			damage = 8
		}
		nearest.LastDamageMessage = nearest.Name + " was zapped by Guardian"
		m.Survival.ApplyDamage(m.Manager, nearest, damage, m.Survival.MobDamageTypeID)
	}
}

// tickIronGolem runs iron golem AI: patrols village, attacks hostile mobs near players.
func (m *MobManager) tickIronGolem(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Iron golems attack hostile mobs targeting players
	var targetMob *Mob
	targetDist := 16.0
	for _, other := range m.mobsInRange(mob.X, mob.Z, 16.0) {
		if other.EID == mob.EID || !other.Hostile {
			continue
		}
		dx := other.X - mob.X
		dy := other.Y - mob.Y
		dz := other.Z - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < targetDist {
			targetDist = d
			targetMob = other
		}
	}

	if targetMob != nil {
		dx := targetMob.X - mob.X
		dz := targetMob.Z - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

		if dist > 1.5 {
			nx := dx / dist * mob.Speed
			nz := dz / dist * mob.Speed
			m.tryMove(mob, nx, nz)
		}

		// Melee attack — iron golems hit hard and launch mobs upward
		if targetDist <= 2.5 && mob.AttackCooldown <= 0 {
			mob.AttackCooldown = 20
			targetMob.Health -= 10
			targetMob.Y += 0.5 // knockup
			if targetMob.Health <= 0 {
				targetMob.Health = 0
				m.killMob(targetMob, nil)
			}
			BroadcastSound(m.Manager, SoundIronGolemAttack, SoundCategoryNeutral, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}

		m.broadcastMobMove(mob)
		return
	}

	// Wander peacefully
	m.tickWander(mob, tick)
}

// tickShulker runs shulker AI: stationary, shoots homing projectiles.
func (m *MobManager) tickShulker(mob *Mob, tick int64) {
	// Shulkers are stationary — no movement

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 16.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		return
	}

	// Shoot shulker bullet every 4 seconds
	if nearestDist <= 16.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 80
		px, py, pz := nearest.Position()
		m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+0.5, mob.Z, px, py+1.0, pz, 1.0)
	}
}

// tickMagmaCube runs magma cube AI: like slime but deals fire damage on contact.
func (m *MobManager) tickMagmaCube(mob *Mob, tick int64) {
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 16.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest

	// Jump toward target every 40 ticks
	if tick%40 == 0 {
		px, _, pz := nearest.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 1.0 {
			jumpDist := mob.Speed * 3
			if jumpDist > dist {
				jumpDist = dist
			}
			mob.X += dx / dist * jumpDist
			mob.Z += dz / dist * jumpDist
			mob.Y += 0.5 // hop
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
	}

	// Contact damage
	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		nearest.LastDamageMessage = nearest.Name + " was squished by Magma Cube"
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
	}

	m.broadcastMobMove(mob)
}

// tickPillager runs pillager AI: ranged crossbow attack, patrols.
func (m *MobManager) tickPillager(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 32.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Maintain 8-15 block distance (ranged attacker)
	if dist < 8 {
		nx := -dx / dist * mob.Speed
		nz := -dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	} else if dist > 15 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	m.broadcastMobMove(mob)

	// Crossbow attack every 2.5 seconds
	if nearestDist <= 20.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 50
		m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.5, mob.Z, px, py+1.0, pz, 3.0)
		BroadcastSound(m.Manager, SoundCrossbowShoot, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// tickVindicator runs vindicator AI: charges and attacks with axe.
func (m *MobManager) tickVindicator(mob *Mob, tick int64) {
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 24.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest

	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	// Vindicators sprint when close (1.5x speed)
	if dist > 1.5 {
		sprintMob := *mob
		if dist < 10 {
			sprintMob.Speed *= 1.5
		}
		m.moveWithPathfinding(&sprintMob, nearest, tick)
		mob.X = sprintMob.X
		mob.Y = sprintMob.Y
		mob.Z = sprintMob.Z
		mob.Path = sprintMob.Path
		mob.PathIndex = sprintMob.PathIndex
		mob.PathRecalcTick = sprintMob.PathRecalcTick
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
	}

	// Heavy axe attack: 13 damage on hard
	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	m.broadcastMobMove(mob)
}

// tickEvoker runs evoker AI: summons vexes, casts fang attack.
func (m *MobManager) tickEvoker(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 24.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Keep distance, retreat if too close
	if dist < 6 {
		nx := -dx / dist * mob.Speed
		nz := -dz / dist * mob.Speed
		m.tryMove(mob, nx, nz)
	} else if dist > 16 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	m.broadcastMobMove(mob)

	// Evoker fangs attack: line of damage toward target
	if nearestDist <= 16.0 && mob.ShootCooldown <= 0 {
		mob.ShootCooldown = 100 // 5 second cooldown

		// Spawn fang damage along line toward player
		fangDist := dist
		if fangDist > 10 {
			fangDist = 10
		}
		for i := 1.0; i <= fangDist; i += 1.0 {
			fx := mob.X + dx/dist*i
			fz := mob.Z + dz/dist*i

			// Check if player is near any fang
			pdx := px - fx
			pdz := pz - fz
			if pdx*pdx+pdz*pdz < 1.5 {
				nearest.LastDamageMessage = nearest.Name + " was killed by Evoker Fangs"
				m.Survival.ApplyDamage(m.Manager, nearest, 6, m.Survival.MobDamageTypeID)
				break
			}
		}

		BroadcastSound(m.Manager, SoundEvokerCast, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// tickVex runs vex AI: flying melee mob spawned by evokers.
func (m *MobManager) tickVex(mob *Mob, tick int64) {
	// Vexes fly — no gravity

	var nearest *game.Player
	nearestDist := 24.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		// Drift
		mob.X += (rand.Float64() - 0.5) * 0.3
		mob.Y += (rand.Float64() - 0.5) * 0.2
		mob.Z += (rand.Float64() - 0.5) * 0.3
		m.broadcastMobMove(mob)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dy := py + 1.0 - mob.Y // aim for head height
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Fly directly toward target (vexes go through blocks)
	if dist > 0.5 {
		speed := mob.Speed * 1.5
		mob.X += dx / dist * speed
		mob.Y += dy / dist * speed
		mob.Z += dz / dist * speed
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
	}

	// Melee attack
	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	m.broadcastMobMove(mob)
}

// tickRavager runs ravager AI: charges at players, breaks crops.
func (m *MobManager) tickRavager(mob *Mob, tick int64) {
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 32.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 2.0 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	// Ravagers deal heavy damage and knockback
	if nearestDist <= 2.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 30
		nearest.LastDamageMessage = nearest.Name + " was trampled by Ravager"
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	m.broadcastMobMove(mob)
}

// tickDrowned runs drowned AI: underwater zombie with trident throw.
func (m *MobManager) tickDrowned(mob *Mob, tick int64) {
	// Drowned can be underwater — simplified gravity
	m.applyGravity(mob)

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 24.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.5 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	// Melee attack when close
	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 30
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	// Some drowned throw tridents (30% chance to be a trident drowned)
	if nearestDist > 3 && nearestDist <= 15 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		if mob.EID%3 == 0 { // deterministic "has trident" based on EID
			mob.ShootCooldown = 60
			m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.5, mob.Z, px, py+1.0, pz, 3.0)
		}
	}

	m.broadcastMobMove(mob)
}

// tickStray runs stray AI: like skeleton but applies slowness.
func (m *MobManager) tickStray(mob *Mob, tick int64) {
	// Stray uses skeleton AI — it's a skeleton variant
	m.tickSkeleton(mob, tick)
}

// tickHusk runs husk AI: like zombie but applies hunger.
func (m *MobManager) tickHusk(mob *Mob, tick int64) {
	// Husk→Zombie conversion when submerged in water
	if m.tickZombieConversion(mob) {
		return
	}

	// Husk uses standard hostile AI (zombie behavior) with hunger on hit
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 32.0
	mobEyeY := mob.Y + 1.5
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		ppx, ppy, ppz := p.Position()
		d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
		if d < nearestDist && m.hasLineOfSight(mob.X, mobEyeY, mob.Z, ppx, ppy+1.62, ppz) {
			nearestDist = d
			nearest = p
		}
	})

	if nearest != nil {
		mob.Target = nearest
		px, _, pz := nearest.Position()
		dx := px - mob.X
		dz := pz - mob.Z
		dist := math.Sqrt(dx*dx + dz*dz)

		if dist > 1.5 {
			m.moveWithPathfinding(mob, nearest, tick)
		}

		if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
			mob.AttackCooldown = 30
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
			m.broadcastArmSwing(mob)
			// Apply hunger effect
			if m.Survival.EffectMgr != nil {
				m.Survival.EffectMgr.ApplyEffect(nearest, 17, 0, 140, false) // hunger = 17, 7 seconds
			}
		}

		m.broadcastMobMove(mob)
	} else {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
	}
}

// tickCaveSpider runs cave spider AI: chase player, melee attack with poison.
// Smaller than regular spiders (0.7 blocks tall) with shorter attack range (1.0 vs 1.5).
func (m *MobManager) tickCaveSpider(mob *Mob, tick int64) {
	m.applyGravity(mob)

	var nearest *game.Player
	nearestDist := 32.0
	mobEyeY := mob.Y + 0.35
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		dx := px - mob.X
		dy := py - mob.Y
		dz := pz - mob.Z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d < nearestDist && m.hasLineOfSight(mob.X, mobEyeY, mob.Z, px, py+1.62, pz) {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.0 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	if nearestDist <= 1.0 && mob.AttackCooldown <= 0 && mob.Damage > 0 {
		mob.AttackCooldown = 30
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
		// Poison duration scales with difficulty: 7s Normal, 15s Hard
		if m.Survival.EffectMgr != nil {
			duration := int32(140)
			if m.Rules != nil && m.Rules.GetDifficulty() >= 3 {
				duration = 300
			}
			m.Survival.EffectMgr.ApplyEffect(nearest, 19, 0, duration, false)
		}
	}

	m.broadcastMobMove(mob)
}

// Sound constants for new mob types (only those not already in sound.go).
const (
	SoundIronGolemAttack = 551
	SoundEvokerCast      = 333
)

// tickWarden runs warden AI: vibration detection, 3-strike anger, melee + sonic boom.
func (m *MobManager) tickWarden(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Initialize per-player tracking maps on first tick.
	if mob.WardenPlayerAnger == nil {
		mob.WardenPlayerAnger = make(map[int32]int32)
		mob.WardenPrevPos = make(map[int32][3]float64)
	}

	// --- Vibration detection: every 20 ticks, scan for players within 16 blocks ---
	if tick-mob.WardenSniffTick >= 20 {
		mob.WardenSniffTick = tick

		m.Manager.ForEach(func(p *game.Player) {
			if p.Dead || p.GameMode != 0 {
				return
			}
			px, py, pz := p.Position()
			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist > 16 {
				return
			}

			eid := p.EID
			prev, hasPrev := mob.WardenPrevPos[eid]
			mob.WardenPrevPos[eid] = [3]float64{px, py, pz}

			if !hasPrev {
				return
			}

			// Check if player moved since last scan.
			mdx := px - prev[0]
			mdy := py - prev[1]
			mdz := pz - prev[2]
			moveDist := math.Sqrt(mdx*mdx + mdy*mdy + mdz*mdz)
			if moveDist < 0.1 {
				return
			}

			// Sneaking players generate much less anger.
			angerInc := int32(10)
			if p.Sneaking {
				angerInc = 1
			}
			mob.WardenPlayerAnger[eid] += angerInc

			// Emit vibration for sculk sensors when warden detects movement.
			if m.SculkMgr != nil {
				m.SculkMgr.EmitVibration(int(px), int(py), int(pz), VibrationStep, eid, 0)
			}
		})

		// Find highest-anger player and set as target.
		var bestAnger int32
		var bestEID int32 = -1
		for eid, anger := range mob.WardenPlayerAnger {
			if anger > bestAnger {
				bestAnger = anger
				bestEID = eid
			}
		}
		mob.WardenAnger = bestAnger
		mob.WardenTarget = bestEID

		// Anger decay: reduce each player's anger by 1.
		for eid, anger := range mob.WardenPlayerAnger {
			if anger <= 1 {
				delete(mob.WardenPlayerAnger, eid)
				delete(mob.WardenPrevPos, eid)
			} else {
				mob.WardenPlayerAnger[eid] = anger - 1
			}
		}
	}

	// --- Resolve target player ---
	var target *game.Player
	if mob.WardenTarget >= 0 {
		m.Manager.ForEach(func(p *game.Player) {
			if p.EID == mob.WardenTarget && !p.Dead && p.GameMode == 0 {
				target = p
			}
		})
	}

	// --- Heartbeat sound based on anger tier ---
	var heartbeatInterval int64
	switch {
	case mob.WardenAnger >= 80:
		heartbeatInterval = 10
	case mob.WardenAnger >= 40:
		heartbeatInterval = 30
	default:
		heartbeatInterval = 60
	}
	if tick-mob.WardenLastHeartbeat >= heartbeatInterval {
		mob.WardenLastHeartbeat = tick
		BroadcastSound(m.Manager, SoundWardenHeartbeat, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}

	// --- Tier behavior ---
	switch {
	case mob.WardenAnger >= 80:
		// Enraged: chase and attack target.
		if target != nil {
			px, py, pz := target.Position()
			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

			// Melee attack: within 2 blocks, 20-tick cooldown.
			if dist <= 2.0 && mob.AttackCooldown <= 0 {
				mob.AttackCooldown = 20
				target.LastDamageMessage = target.Name + " was slain by Warden"
				m.Survival.ApplyDamage(m.Manager, target, mob.Damage, m.Survival.MobDamageTypeID)
			}

			// Sonic boom: 5-15 blocks, 100-tick cooldown.
			if dist >= 5.0 && dist <= 15.0 && tick-mob.WardenRoarTick >= 100 {
				mob.WardenRoarTick = tick
				BroadcastSound(m.Manager, SoundWardenSonicBoom, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)

				// Broadcast particles along the line from warden to target.
				steps := int(dist * 2)
				if steps < 4 {
					steps = 4
				}
				for i := 0; i <= steps; i++ {
					t := float64(i) / float64(steps)
					lx := mob.X + dx*t
					ly := mob.Y + 1.0 + dy*t
					lz := mob.Z + dz*t
					BroadcastParticle(m.Manager, ParticleSonicBoom, lx, ly, lz, 0, 0, 0, 0, 1)
				}

				// Sonic boom deals 10 damage that bypasses armor (use mob damage type).
				target.LastDamageMessage = target.Name + " was obliterated by Warden's sonic boom"
				m.Survival.ApplyDamage(m.Manager, target, 10, m.Survival.MobDamageTypeID)
			}

			// Chase target.
			if dist > 2.0 {
				m.moveWithPathfinding(mob, target, tick)
			}
		} else {
			// Sniff around when angry but no visible target.
			m.tickWander(mob, tick)
		}

	case mob.WardenAnger >= 40:
		// Alert: move toward target, apply Darkness effect to nearby players.
		if m.EffectMgr != nil {
			m.Manager.ForEach(func(p *game.Player) {
				if p.Dead || p.GameMode != 0 {
					return
				}
				px, py, pz := p.Position()
				d := math.Sqrt(sqDist3(px-mob.X, py-mob.Y, pz-mob.Z))
				if d <= 20.0 {
					m.EffectMgr.ApplyEffect(p, EffectDarkness, 0, 260, true) // ~13 seconds, re-applied
				}
			})
		}
		if target != nil {
			m.moveWithPathfinding(mob, target, tick)
		} else {
			m.tickWander(mob, tick)
		}

	default:
		// Idle: sniffing animation, wander slowly.
		if tick%100 == 0 {
			BroadcastSound(m.Manager, SoundWardenSniff, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}
		m.tickWander(mob, tick)
	}

	if mob.AttackCooldown > 0 {
		mob.AttackCooldown--
	}

	m.broadcastMobMove(mob)
}
