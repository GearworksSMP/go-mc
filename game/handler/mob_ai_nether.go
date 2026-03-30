package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/game"
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
		m.broadcastMobMove(mob)
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

	m.broadcastMobMove(mob)

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

	m.broadcastMobMove(mob)

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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	m.broadcastMobMove(mob)
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
		m.Survival.ApplyDamage(m.Manager, mob.Target, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
	}

	m.broadcastMobMove(mob)
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
	}

	m.broadcastMobMove(mob)
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
		m.Survival.ApplyDamage(m.Manager, nearest, mob.Damage, m.Survival.MobDamageTypeID)
		m.broadcastArmSwing(mob)
		// Apply wither effect (10 seconds of damage over time)
		if m.Survival.EffectMgr != nil {
			m.Survival.EffectMgr.ApplyEffect(nearest, 20, 0, 200, false) // wither = 20
		}
	}

	m.broadcastMobMove(mob)
}

// tickBreeze runs breeze AI: hovers above ground, shoots wind charges at players, evades when close.
func (m *MobManager) tickBreeze(mob *Mob, tick int64) {
	if mob.ShootCooldown > 0 {
		mob.ShootCooldown--
	}

	groundY := m.findGroundY(mob.X, mob.Y, mob.Z)
	hoverY := groundY + 3.5
	if mob.Y < hoverY-0.5 {
		mob.Y += 0.08
	} else if mob.Y > hoverY+0.5 {
		mob.Y -= 0.04
	}

	var nearest *game.Player
	nearestDist := 24.0
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
		if tick%40 == 0 {
			mob.FlyTargetY = mob.Y + (rand.Float64()*4 - 2)
		}
		mob.X += (rand.Float64() - 0.5) * 0.15
		mob.Z += (rand.Float64() - 0.5) * 0.15
		m.broadcastMobMove(mob)
		return
	}

	mob.Target = nearest
	px, py, pz := nearest.Position()
	dx := px - mob.X
	dz := pz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	if dist < 4 && dist > 0.1 {
		evadeX := -dx / dist * 0.8
		evadeZ := -dz / dist * 0.8
		mob.X += evadeX
		mob.Z += evadeZ
		mob.Y += 0.3
		m.broadcastMobMove(mob)
		return
	}

	if dist < 8 {
		mob.X -= dx / dist * 0.12
		mob.Z -= dz / dist * 0.12
	} else if dist > 16 {
		mob.X += dx / dist * 0.12
		mob.Z += dz / dist * 0.12
	}

	m.broadcastMobMove(mob)

	if nearestDist <= 24.0 && mob.ShootCooldown <= 0 && m.ArrowMgr != nil {
		mob.ShootCooldown = 60
		m.ArrowMgr.SpawnWindCharge(mob.EID, mob.X, mob.Y+1.0, mob.Z, px, py+1.0, pz)
		BroadcastSound(m.Manager, SoundBreezeShoot, SoundCategoryHostile, mob.X, mob.Y, mob.Z, 1.0, 1.0)
	}
}

// findGroundY scans downward from pos to find the first solid block.
func (m *MobManager) findGroundY(x, y, z float64) float64 {
	bx, bz := int(math.Floor(x)), int(math.Floor(z))
	for by := int(math.Floor(y)); by > -64; by-- {
		state, err := m.World.GetBlock(bx, by, bz)
		if err == nil && state != 0 {
			return float64(by + 1)
		}
	}
	return -64
}
