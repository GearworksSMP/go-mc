package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// tickGhast runs ghast AI: flies around, shoots fireballs at players.
func (m *MobManager) tickGhast(mob *Mob, tick int64) {
	// Ghasts float — no gravity

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 64.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		px, py, pz := p.Position()
		d := math.Sqrt(sqDist3(px-mob.X, py-mob.Y, pz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		// Drift randomly
		if tick%40 == 0 {
			mob.FlyTargetY = mob.Y + (rand.Float64()*6 - 3)
		}
		mob.X += (rand.Float64() - 0.5) * 0.2
		mob.Z += (rand.Float64() - 0.5) * 0.2
		if mob.Y < mob.FlyTargetY {
			mob.Y += 0.05
		} else if mob.Y > mob.FlyTargetY {
			mob.Y -= 0.05
		}
		m.broadcastMoveEntity(mob)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()

	// Face target
	dx := px - mob.X
	dz := pz - mob.Z
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Maintain ~20 block distance and altitude
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist < 15 {
		mob.X -= dx / dist * 0.1
		mob.Z -= dz / dist * 0.1
	} else if dist > 25 {
		mob.X += dx / dist * 0.1
		mob.Z += dz / dist * 0.1
	}
	// Float above target
	targetY := py + 10
	if mob.Y < targetY-1 {
		mob.Y += 0.08
	} else if mob.Y > targetY+1 {
		mob.Y -= 0.08
	}

	m.broadcastMoveEntity(mob)

	// Shoot fireball every 3 seconds when within range
	if nearestDist <= 64.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 60
		m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.0, mob.Z, px, py+1.0, pz, 1.5)
		BroadcastSound(m.Manager, SoundGhastShoot, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// tickBlaze runs blaze AI: floats, shoots fireballs in bursts.
func (m *MobManager) tickBlaze(mob *Mob, tick int64) {
	// Blazes float
	m.applyGravity(mob)
	// But also hover
	if mob.Y < mob.FlyTargetY {
		mob.Y += 0.05
	}

	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	var nearest *game.Player
	nearestDist := 48.0
	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}
		bpx, bpy, bpz := p.Position()
		d := math.Sqrt(sqDist3(bpx-mob.X, bpy-mob.Y, bpz-mob.Z))
		if d < nearestDist {
			nearestDist = d
			nearest = p
		}
	})

	if nearest == nil {
		mob.Target = nil
		// Float up gently
		if tick%60 == 0 {
			mob.FlyTargetY = mob.Y + rand.Float64()*3
		}
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Maintain 8-16 block distance
	if dist < 8 {
		mob.X -= dx / dist * 0.1
		mob.Z -= dz / dist * 0.1
	} else if dist > 16 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	// Rise to be slightly above target
	targetAlt := py + 3
	if mob.Y < targetAlt {
		mob.Y += 0.06
	}
	mob.FlyTargetY = targetAlt

	m.broadcastMoveEntity(mob)

	// Shoot 3 fireballs in quick succession, then cooldown
	if nearestDist <= 48.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 60 // 3 second cooldown after burst
		for i := 0; i < 3; i++ {
			// Slight spread
			spread := (rand.Float64() - 0.5) * 2
			m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.0, mob.Z, px+spread, py+1.0, pz+spread, 2.0)
		}
		BroadcastSound(m.Manager, SoundBlazeShoot, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

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
		m.broadcastMoveEntity(mob)
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

	m.broadcastMoveEntity(mob)

	// Beam attack: 2 seconds charge (40 ticks), 6 damage
	if nearestDist <= 15.0 && mob.ShootCooldown <= 0 {
		mob.ShootCooldown = 80 // 4 second total cycle

		damage := float32(6)
		if mob.TypeID == MobTypeElderGuardian {
			damage = 8
		}
		nearest.LastDamageMessage = nearest.Name + " was zapped by Guardian"
		m.Survival.ApplyDamage(m.Manager, nearest, damage, m.Survival.AttackDamageTypeID)
	}
}

// tickIronGolem runs iron golem AI: patrols village, attacks hostile mobs near players.
func (m *MobManager) tickIronGolem(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Iron golems attack hostile mobs targeting players
	var targetMob *Mob
	targetDist := 16.0
	for _, other := range m.Mobs {
		if other.EID == mob.EID || other.Health <= 0 || !other.Hostile {
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

		m.broadcastMoveEntity(mob)
		return
	}

	// Wander peacefully
	m.tickWander(mob, tick)
}

// tickPiglin runs piglin AI: hostile unless player wears gold armor, barters with gold ingots.
func (m *MobManager) tickPiglin(mob *Mob, tick int64) {
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

	// Check if player is wearing gold armor — if so, piglin is neutral
	if playerHasGoldArmor(nearest) {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	mob.Target = nearest
	mob.Hostile = true

	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.5 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
}

// playerHasGoldArmor checks if a player has any gold armor equipped.
func playerHasGoldArmor(p *game.Player) bool {
	// Armor slots are 36-39 in player inventory (feet, legs, chest, head)
	for slot := 36; slot <= 39; slot++ {
		if slot < len(p.Inventory) && p.Inventory[slot].ID != 0 {
			name := ItemNameByID(p.Inventory[slot].ID)
			switch name {
			case "golden_helmet", "golden_chestplate", "golden_leggings", "golden_boots":
				return true
			}
		}
	}
	return false
}

// tickZombifiedPiglin runs zombified piglin AI: neutral until attacked, then swarms.
func (m *MobManager) tickZombifiedPiglin(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// If not hostile, just wander
	if !mob.Hostile {
		m.tickWander(mob, tick)
		return
	}

	// Hostile — chase target
	if mob.Target == nil || mob.Target.Dead {
		mob.Hostile = false
		mob.Target = nil
		mob.Path = nil
		return
	}

	px, _, pz := mob.Target.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 40 {
		// Lost interest
		mob.Hostile = false
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
		return
	}

	if dist > 1.5 {
		m.moveWithPathfinding(mob, mob.Target, tick)
	}

	targetDist := math.Sqrt(sqDist3(px-mob.X, mob.Target.Y-mob.Y, pz-mob.Z))
	if targetDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		m.Survival.ApplyDamage(m.Manager, mob.Target, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
}

// AggroZombifiedPiglins makes all zombified piglins near a position hostile toward a player.
func (m *MobManager) AggroZombifiedPiglins(attacker *game.Player, x, y, z float64) {
	for _, mob := range m.Mobs {
		if mob.TypeID != MobTypeZombifiedPiglin || mob.Health <= 0 {
			continue
		}
		d := math.Sqrt(sqDist3(mob.X-x, mob.Y-y, mob.Z-z))
		if d <= 32 {
			mob.Hostile = true
			mob.Target = attacker
			mob.Path = nil
		}
	}
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
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

	m.broadcastMoveEntity(mob)

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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
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

	m.broadcastMoveEntity(mob)

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
				m.Survival.ApplyDamage(m.Manager, nearest, 6, m.Survival.AttackDamageTypeID)
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
		m.broadcastMoveEntity(mob)
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
}

// tickHoglin runs hoglin AI: hostile, attacks players, flees from warped fungus.
func (m *MobManager) tickHoglin(mob *Mob, tick int64) {
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
	px, _, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 1.5 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	// Hoglin attacks with knockup
	if nearestDist <= 2.0 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	m.broadcastMoveEntity(mob)
}

// tickWitherSkeleton runs wither skeleton AI: melee with wither effect.
func (m *MobManager) tickWitherSkeleton(mob *Mob, tick int64) {
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

	if dist > 1.5 {
		m.moveWithPathfinding(mob, nearest, tick)
	}

	// Wither skeleton attacks apply wither effect
	if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
		mob.AttackCooldown = 20
		nearest.LastDamageMessage = nearest.Name + " withered away"
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
		// Apply wither effect (10 seconds of damage over time)
		if m.Survival.EffectMgr != nil {
			m.Survival.EffectMgr.ApplyEffect(nearest, 20, 0, 200, false) // wither = 20
		}
	}

	m.broadcastMoveEntity(mob)
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
	}

	// Some drowned throw tridents (30% chance to be a trident drowned)
	if nearestDist > 3 && nearestDist <= 15 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		if mob.EID%3 == 0 { // deterministic "has trident" based on EID
			mob.ShootCooldown = 60
			m.ArrowMgr.SpawnArrow(mob.EID, mob.X, mob.Y+1.5, mob.Z, px, py+1.0, pz, 3.0)
		}
	}

	m.broadcastMoveEntity(mob)
}

// tickStray runs stray AI: like skeleton but applies slowness.
func (m *MobManager) tickStray(mob *Mob, tick int64) {
	// Stray uses skeleton AI — it's a skeleton variant
	m.tickSkeleton(mob, tick)
}

// tickHusk runs husk AI: like zombie but applies hunger.
func (m *MobManager) tickHusk(mob *Mob, tick int64) {
	// Husk uses standard hostile AI (zombie behavior) with hunger on hit
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
			m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
			// Apply hunger effect
			if m.Survival.EffectMgr != nil {
				m.Survival.EffectMgr.ApplyEffect(nearest, 17, 0, 140, false) // hunger = 17, 7 seconds
			}
		}

		m.broadcastMoveEntity(mob)
	} else {
		mob.Target = nil
		mob.Path = nil
		m.tickWander(mob, tick)
	}
}

// tickBee runs bee AI: passive unless provoked, flies around flowers.
func (m *MobManager) tickBee(mob *Mob, tick int64) {
	// Bees fly
	if mob.Y < mob.FlyTargetY {
		mob.Y += 0.05
	} else if mob.Y > mob.FlyTargetY+0.5 {
		mob.Y -= 0.03
	}

	if tick%80 == 0 {
		mob.FlyTargetY = mob.Y + (rand.Float64()*4 - 2)
	}

	if mob.Hostile {
		// Bee was provoked — sting attack
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

		if nearest != nil {
			px, py, pz := nearest.Position()
			dx := px - mob.X
			dy := py + 1 - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist > 0.5 {
				speed := mob.Speed * 2
				mob.X += dx / dist * speed
				mob.Y += dy / dist * speed
				mob.Z += dz / dist * speed
				mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
			}

			if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
				mob.AttackCooldown = 20
				m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.AttackDamageTypeID)
				// Bee dies after stinging (like real bee)
				mob.Health = 0
				m.killMob(mob, nil)
				return
			}
		}
	} else {
		// Passive flying
		mob.X += (rand.Float64() - 0.5) * 0.15
		mob.Z += (rand.Float64() - 0.5) * 0.15
	}

	m.broadcastMoveEntity(mob)
}

// sqDist3 returns the squared distance given dx,dy,dz (utility to avoid repeating).
func sqDist3(dx, dy, dz float64) float64 {
	return dx*dx + dy*dy + dz*dz
}

// Sound constants for new mob types (only those not already in sound.go).
const (
	SoundIronGolemAttack = 551
	SoundEvokerCast      = 333
)

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
