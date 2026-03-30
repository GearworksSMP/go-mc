package handler

import (
	"math"
)

// blockNameAtMob returns the block name at a mob's foot position.
func (m *MobManager) blockNameAtMob(mob *Mob) string {
	bx := int(math.Floor(mob.X))
	by := int(math.Floor(mob.Y))
	bz := int(math.Floor(mob.Z))
	state, err := m.World.GetBlock(bx, by, bz)
	if err != nil {
		return ""
	}
	return BlockNameFromState(int(state))
}

// convertMob converts a mob from one type to another, preserving position and health.
// It removes the old entity, changes the type, and re-spawns with the new type.
func (m *MobManager) convertMob(mob *Mob, newType int32) {
	// Remove old entity from clients
	m.removeMobEntity(mob)

	// Update mob type and reset defaults for new type
	mob.TypeID = newType
	newHealth, newDamage, newSpeed, newHostile := mobDefaults(newType)
	if mob.Health > newHealth {
		mob.Health = newHealth
	}
	mob.MaxHealth = newHealth
	mob.Damage = newDamage
	mob.Speed = newSpeed
	mob.Hostile = newHostile
	mob.ConversionTicks = 0

	// Reset AI state
	mob.Target = nil
	mob.Path = nil
	mob.AttackCooldown = 0

	// Spawn new entity (reuses EID)
	m.broadcastSpawn(mob)

	// Play conversion sound and particles
	BroadcastSound(m.Manager, MobDeathSound(newType), MobSoundCategory(newType),
		mob.X, mob.Y, mob.Z, 1.0, 1.0)
	BroadcastParticle(m.Manager, ParticleSplash,
		mob.X, mob.Y+1.0, mob.Z, 0.5, 0.5, 0.5, 0.1, 20)
}

// tickConversion checks if a mob is standing in triggerBlock and counts down
// ConversionTicks (starting at 600 = 30s). When the timer expires, converts
// the mob to newType. Returns true if conversion happened this tick.
func (m *MobManager) tickConversion(mob *Mob, triggerBlock string, newType int32) bool {
	if m.blockNameAtMob(mob) == triggerBlock {
		if mob.ConversionTicks == 0 {
			mob.ConversionTicks = 600
		}
		mob.ConversionTicks--
		if mob.ConversionTicks <= 0 {
			m.convertMob(mob, newType)
			return true
		}
	} else {
		mob.ConversionTicks = 0
	}
	return false
}

// tickZombieConversion checks if a zombie/husk should convert due to water submersion.
func (m *MobManager) tickZombieConversion(mob *Mob) bool {
	target := MobTypeDrowned
	if mob.TypeID == MobTypeHusk {
		target = MobTypeZombie
	}
	return m.tickConversion(mob, "water", target)
}

// tickSkeletonConversion checks if a skeleton should convert to a stray in powder snow.
func (m *MobManager) tickSkeletonConversion(mob *Mob) bool {
	return m.tickConversion(mob, "powder_snow", MobTypeStray)
}
