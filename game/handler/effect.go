package handler

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Effect IDs (minecraft registry).
const (
	EffectSpeed          int32 = 1
	EffectSlowness       int32 = 2
	EffectHaste          int32 = 3
	EffectMiningFatigue  int32 = 4
	EffectStrength       int32 = 5
	EffectInstantHealth  int32 = 6
	EffectInstantDamage  int32 = 7
	EffectJumpBoost      int32 = 8
	EffectNausea         int32 = 9
	EffectRegeneration   int32 = 10
	EffectResistance     int32 = 11
	EffectFireResistance int32 = 12
	EffectWaterBreathing int32 = 13
	EffectInvisibility   int32 = 14
	EffectBlindness      int32 = 15
	EffectNightVision    int32 = 16
	EffectHunger         int32 = 17
	EffectWeakness       int32 = 18
	EffectPoison         int32 = 19
	EffectWither         int32 = 20
	EffectAbsorption     int32 = 22
	EffectGlowing        int32 = 24
	EffectLevitation     int32 = 25
	EffectSlowFalling    int32 = 28
)

// effectNameToID maps effect names to IDs for /effect command.
var effectNameToID = map[string]int32{
	"speed": EffectSpeed, "slowness": EffectSlowness, "haste": EffectHaste,
	"mining_fatigue": EffectMiningFatigue, "strength": EffectStrength,
	"instant_health": EffectInstantHealth, "instant_damage": EffectInstantDamage,
	"jump_boost": EffectJumpBoost, "nausea": EffectNausea,
	"regeneration": EffectRegeneration, "resistance": EffectResistance,
	"fire_resistance": EffectFireResistance, "water_breathing": EffectWaterBreathing,
	"invisibility": EffectInvisibility, "blindness": EffectBlindness,
	"night_vision": EffectNightVision, "hunger": EffectHunger,
	"weakness": EffectWeakness, "poison": EffectPoison, "wither": EffectWither,
	"absorption": EffectAbsorption, "glowing": EffectGlowing,
	"levitation": EffectLevitation, "slow_falling": EffectSlowFalling,
}

// EffectIDByName returns the effect ID for a name, or -1 if unknown.
func EffectIDByName(name string) int32 {
	if id, ok := effectNameToID[name]; ok {
		return id
	}
	return -1
}

// EffectManager manages status effects for all players.
type EffectManager struct {
	Manager  *game.PlayerManager
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// NewEffectManager creates a new EffectManager.
func NewEffectManager(manager *game.PlayerManager, survival *SurvivalHandler, logger *log.Logger) *EffectManager {
	return &EffectManager{
		Manager:  manager,
		Survival: survival,
		Logger:   logger,
	}
}

// ApplyEffect adds or replaces an effect on a player.
// For instant effects (InstantHealth, InstantDamage), the effect is applied immediately
// and not stored as an ongoing effect.
func (em *EffectManager) ApplyEffect(player *game.Player, effectID, level, durationTicks int32, ambient bool) {
	// Handle instant effects
	if effectID == EffectInstantHealth {
		heal := float32(4 * (level + 1))
		player.Health += heal
		if player.Health > 20 {
			player.Health = 20
		}
		SendSetHealth(player)
		BroadcastHealthTag(em.Manager, player)
		em.logf("Applied Instant Health %d to %s (healed %.1f)", level+1, player.Name, heal)
		return
	}
	if effectID == EffectInstantDamage {
		damage := float32(6 * (level + 1))
		if em.Survival != nil {
			em.Survival.ApplyDamage(em.Manager, player, damage, em.Survival.AttackDamageTypeID)
		}
		em.logf("Applied Instant Damage %d to %s (%.1f damage)", level+1, player.Name, damage)
		return
	}

	// Initialize effects map if needed
	if player.Effects == nil {
		player.Effects = make(map[int32]*game.ActiveEffect)
	}

	effect := &game.ActiveEffect{
		ID:       effectID,
		Level:    level,
		Duration: durationTicks,
		Ambient:  ambient,
	}
	player.Effects[effectID] = effect

	// Handle absorption: grant extra absorption hearts on apply
	if effectID == EffectAbsorption {
		player.Absorption = float32(4 * (level + 1))
	}

	// Send effect packet to the player
	em.sendUpdateEffect(player, effect)

	em.logf("Applied %s %d to %s (duration=%d ticks)", effectName(effectID), level+1, player.Name, durationTicks)
}

// RemoveEffect removes an effect from a player and notifies the client.
func (em *EffectManager) RemoveEffect(player *game.Player, effectID int32) {
	if player.Effects == nil {
		return
	}
	if _, ok := player.Effects[effectID]; !ok {
		return
	}
	delete(player.Effects, effectID)

	// If removing absorption, clear the absorption HP pool
	if effectID == EffectAbsorption {
		player.Absorption = 0
	}

	// Send remove effect packet
	player.WritePacket(pk.Marshal(
		packetid.ClientboundRemoveMobEffect,
		pk.VarInt(player.EID),
		pk.VarInt(effectID),
	))
}

// ClearAllEffects removes all effects from a player.
func (em *EffectManager) ClearAllEffects(player *game.Player) {
	if player.Effects == nil {
		return
	}
	for effectID := range player.Effects {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundRemoveMobEffect,
			pk.VarInt(player.EID),
			pk.VarInt(effectID),
		))
	}
	player.Effects = nil
	player.Absorption = 0
	em.logf("Cleared all effects from %s", player.Name)
}

// Tick processes all active effects for all players. Called every tick.
func (em *EffectManager) Tick(tick int64) {
	em.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.Effects == nil {
			return
		}

		var expired []int32
		for effectID, effect := range p.Effects {
			// Decrement duration (skip infinite effects)
			if effect.Duration > 0 {
				effect.Duration--
				if effect.Duration <= 0 {
					expired = append(expired, effectID)
					continue
				}
			}

			// Apply per-tick effects
			switch effectID {
			case EffectRegeneration:
				// Heal 1 HP every 50/(level+1) ticks
				interval := int32(50) / (effect.Level + 1)
				if interval < 1 {
					interval = 1
				}
				if int32(tick)%interval == 0 {
					if p.Health < 20 {
						p.Health += 1
						if p.Health > 20 {
							p.Health = 20
						}
						SendSetHealth(p)
						BroadcastHealthTag(em.Manager, p)
					}
				}

			case EffectPoison:
				// Damage 1 HP every 25/(level+1) ticks (cannot kill, minimum 1 HP)
				interval := int32(25) / (effect.Level + 1)
				if interval < 1 {
					interval = 1
				}
				if int32(tick)%interval == 0 {
					if p.Health > 1 {
						p.Health -= 1
						if p.Health < 1 {
							p.Health = 1
						}
						SendSetHealth(p)
						BroadcastHealthTag(em.Manager, p)
						// Show hurt animation
						hurtPkt := pk.Marshal(
							packetid.ClientboundHurtAnimation,
							pk.VarInt(p.EID),
							pk.Float(0),
						)
						em.Manager.ForEach(func(other *game.Player) {
							other.WritePacket(hurtPkt)
						})
					}
				}

			case EffectWither:
				// Wither: like poison but CAN kill. Damage 1 HP every 40/(level+1) ticks.
				interval := int32(40) / (effect.Level + 1)
				if interval < 1 {
					interval = 1
				}
				if int32(tick)%interval == 0 {
					if em.Survival != nil && !p.IsInvulnerable() {
						p.LastDamageMessage = p.Name + " withered away"
						em.Survival.ApplyDamage(em.Manager, p, 1, em.Survival.AttackDamageTypeID)
					}
				}

			case EffectHunger:
				// Increase exhaustion by 0.005*(level+1) per tick
				p.Exhaustion += 0.005 * float32(effect.Level+1)
			}
		}

		// Remove expired effects
		for _, effectID := range expired {
			em.RemoveEffect(p, effectID)
		}
	})
}

// GetSpeedModifier returns the movement speed multiplier from Speed/Slowness effects.
// Returns 1.0 if no speed effects are active.
func (em *EffectManager) GetSpeedModifier(player *game.Player) float64 {
	if player.Effects == nil {
		return 1.0
	}
	mod := 1.0
	if eff, ok := player.Effects[EffectSpeed]; ok {
		mod += 0.2 * float64(eff.Level+1)
	}
	if eff, ok := player.Effects[EffectSlowness]; ok {
		mod -= 0.15 * float64(eff.Level+1)
	}
	if mod < 0 {
		mod = 0
	}
	return mod
}

// GetDamageModifier returns the additive damage modifier from Strength/Weakness effects.
// Positive = Strength bonus, negative = Weakness penalty.
func (em *EffectManager) GetDamageModifier(player *game.Player) float32 {
	if player.Effects == nil {
		return 0
	}
	var mod float32
	if eff, ok := player.Effects[EffectStrength]; ok {
		mod += 3 * float32(eff.Level+1)
	}
	if eff, ok := player.Effects[EffectWeakness]; ok {
		mod -= 4 * float32(eff.Level+1)
	}
	return mod
}

// GetResistanceReduction returns the damage reduction fraction from the Resistance effect.
// e.g. Resistance I = 0.2 (20% reduction), Resistance II = 0.4, etc.
func (em *EffectManager) GetResistanceReduction(player *game.Player) float32 {
	if player.Effects == nil {
		return 0
	}
	eff, ok := player.Effects[EffectResistance]
	if !ok {
		return 0
	}
	reduction := 0.2 * float32(eff.Level+1)
	if reduction > 1.0 {
		reduction = 1.0
	}
	return reduction
}

// HasEffect returns true if the player has the given effect active.
func (em *EffectManager) HasEffect(player *game.Player, effectID int32) bool {
	if player.Effects == nil {
		return false
	}
	_, ok := player.Effects[effectID]
	return ok
}

// sendUpdateEffect sends a ClientboundUpdateMobEffect packet.
func (em *EffectManager) sendUpdateEffect(player *game.Player, eff *game.ActiveEffect) {
	var flags pk.Byte
	if eff.Ambient {
		flags |= 0x01
	}
	flags |= 0x02 // show particles
	flags |= 0x04 // show icon

	player.WritePacket(pk.Marshal(
		packetid.ClientboundUpdateMobEffect,
		pk.VarInt(player.EID),
		pk.VarInt(eff.ID),
		pk.Byte(byte(eff.Level)),
		pk.VarInt(eff.Duration),
		flags,
	))
}

// effectName returns a human-readable name for an effect ID (for logging).
func effectName(id int32) string {
	switch id {
	case EffectSpeed:
		return "Speed"
	case EffectSlowness:
		return "Slowness"
	case EffectHaste:
		return "Haste"
	case EffectMiningFatigue:
		return "Mining Fatigue"
	case EffectStrength:
		return "Strength"
	case EffectInstantHealth:
		return "Instant Health"
	case EffectInstantDamage:
		return "Instant Damage"
	case EffectJumpBoost:
		return "Jump Boost"
	case EffectRegeneration:
		return "Regeneration"
	case EffectResistance:
		return "Resistance"
	case EffectFireResistance:
		return "Fire Resistance"
	case EffectWaterBreathing:
		return "Water Breathing"
	case EffectInvisibility:
		return "Invisibility"
	case EffectNightVision:
		return "Night Vision"
	case EffectWeakness:
		return "Weakness"
	case EffectPoison:
		return "Poison"
	case EffectWither:
		return "Wither"
	case EffectAbsorption:
		return "Absorption"
	case EffectSlowFalling:
		return "Slow Falling"
	default:
		return "Unknown"
	}
}

func (em *EffectManager) logf(format string, args ...any) {
	if em.Logger != nil {
		em.Logger.Printf(format, args...)
	}
}
