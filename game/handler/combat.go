package handler

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// CombatHandler handles player-vs-player and player-vs-mob attacks via ServerboundInteract.
type CombatHandler struct {
	Manager         *game.PlayerManager
	SurvivalHandler *SurvivalHandler
	MobManager      *MobManager
	VillagerMgr     *VillagerManager
	BoatMgr         *BoatManager
	MinecartMgr     *MinecartManager
	EffectMgr       *EffectManager
	Logger          *log.Logger
}

// HandlePacket processes combat-related packets.
// Returns true if the packet was handled.
func (h *CombatHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundInteract {
		return false
	}

	var entityID pk.VarInt
	var action pk.VarInt
	if err := p.Scan(&entityID, &action); err != nil {
		return true
	}

	if action == 1 { // attack
		h.handleAttack(player, int32(entityID))
	} else if action == 0 { // interact (right-click)
		h.handleInteract(player, int32(entityID))
	}
	return true
}

func (h *CombatHandler) handleInteract(player *game.Player, targetEID int32) {
	if player.Dead {
		return
	}

	// Check for boat interaction (right-click to mount)
	if h.BoatMgr != nil && h.BoatMgr.IsBoat(targetEID) {
		h.BoatMgr.MountBoat(player, targetEID)
		return
	}

	// Check for minecart interaction (right-click to mount)
	if h.MinecartMgr != nil && h.MinecartMgr.IsMinecart(targetEID) {
		h.MinecartMgr.MountMinecart(player, targetEID)
		return
	}

	// Check for villager interaction (right-click to trade)
	if h.VillagerMgr != nil && h.VillagerMgr.IsVillager(targetEID) {
		h.VillagerMgr.OpenMerchantUI(player, targetEID)
		return
	}

	// Check for passive mob feeding
	if h.MobManager != nil && h.MobManager.IsPassiveMob(targetEID) {
		mobType := h.MobManager.GetMobType(targetEID)
		heldSlot := int(player.HeldSlot) + 36
		heldItem := &player.Inventory[heldSlot]
		if heldItem.ID > 0 {
			heldName := ItemNameByID(heldItem.ID)
			if isBreedingFood(mobType, heldName) {
				if h.MobManager.FeedMob(targetEID) {
					// Consume one food item in survival
					if player.GameMode == 0 {
						heldItem.Count--
						if heldItem.Count <= 0 {
							*heldItem = game.ItemStack{}
						}
						SendSlotUpdate(player, heldSlot)
					}
				}
			}
		}
	}
}

func (h *CombatHandler) handleAttack(attacker *game.Player, targetEID int32) {
	if attacker.Dead {
		return
	}

	// Look up weapon damage from held item
	heldName := ItemNameByID(attacker.Inventory[attacker.HeldSlot+36].ID)
	baseDamage := GetWeaponDamage(heldName)

	// Compute attack strength from cooldown (vanilla formula)
	cooldownPeriod := GetWeaponCooldown(heldName)
	timeSince := time.Since(attacker.LastAttackTime).Seconds()
	strength := timeSince / cooldownPeriod
	if strength > 1 {
		strength = 1
	}
	if strength < 0 {
		strength = 0
	}

	// Sharpness enchantment: +0.5*level + 0.5 damage
	heldItem := &attacker.Inventory[attacker.HeldSlot+36]
	if heldItem.Enchantments != nil {
		if sharpLvl := heldItem.Enchantments["sharpness"]; sharpLvl > 0 {
			baseDamage += float32(sharpLvl)*0.5 + 0.5
		}
	}

	// Strength/Weakness effect modifiers
	if h.EffectMgr != nil {
		baseDamage += h.EffectMgr.GetDamageModifier(attacker)
		if baseDamage < 0 {
			baseDamage = 0
		}
	}

	// Vanilla damage formula: damage = baseDamage * (0.2 + strength^2 * 0.8)
	damage := baseDamage * float32(0.2+strength*strength*0.8)

	attacker.LastAttackTime = time.Now()

	// Try boat target
	if h.BoatMgr != nil && h.BoatMgr.IsBoat(targetEID) {
		h.BoatMgr.DamageBoat(attacker.EID, targetEID, damage)
		h.logf("Player %s attacked boat EID=%d (damage=%.1f)", attacker.Name, targetEID, damage)
		return
	}

	// Try minecart target
	if h.MinecartMgr != nil && h.MinecartMgr.IsMinecart(targetEID) {
		h.MinecartMgr.DamageMinecart(attacker.EID, targetEID, damage)
		h.logf("Player %s attacked minecart EID=%d (damage=%.1f)", attacker.Name, targetEID, damage)
		return
	}

	// Try mob target first
	if h.MobManager != nil && h.MobManager.DamageMob(attacker, targetEID, damage) {
		attacker.Exhaustion += 0.1
		h.applyDurabilityLoss(attacker, heldName, strength)
		h.logf("Player %s attacked mob EID=%d (strength=%.2f, damage=%.1f)", attacker.Name, targetEID, strength, damage)
		return
	}

	// Try player target
	target := h.Manager.GetByEID(targetEID)
	if target == nil || target.Dead {
		return
	}

	if h.SurvivalHandler != nil {
		h.SurvivalHandler.ApplyDamageFrom(h.Manager, target, damage, h.SurvivalHandler.AttackDamageTypeID, attacker.Name)
	}

	// Add exhaustion for attacking
	attacker.Exhaustion += 0.1

	// Knockback: push target away from attacker (reduced if blocking with shield)
	ax, _, az := attacker.Position()
	tx, _, tz := target.Position()
	dx := tx - ax
	dz := tz - az
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist > 0 {
		scale := 4000.0 / dist
		knockbackMult := 1.0
		if target.Blocking {
			knockbackMult = 0.2
		}

		// Netherite knockback resistance: each netherite armor piece gives 0.1 resistance
		totalKBResist := 0.0
		for _, slot := range []int{5, 6, 7, 8} {
			totalKBResist += GetKnockbackResistance(ItemNameByID(target.Inventory[slot].ID))
		}
		if totalKBResist > 1.0 {
			totalKBResist = 1.0
		}
		knockbackMult *= (1 - totalKBResist)

		velX := int16(dx * scale * knockbackMult)
		velY := int16(float64(3000) * knockbackMult)
		velZ := int16(dz * scale * knockbackMult)

		target.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(target.EID),
			pk.Short(velX),
			pk.Short(velY),
			pk.Short(velZ),
		))
	}

	// Thorns enchantment: check target's armor for thorns, reflect damage back to attacker
	h.applyThorns(attacker, target)

	h.applyDurabilityLoss(attacker, heldName, strength)
	h.logf("Player %s attacked %s (strength=%.2f, damage=%.1f)", attacker.Name, target.Name, strength, damage)
}

// applyDurabilityLoss reduces tool durability after a near-full-charge attack.
func (h *CombatHandler) applyDurabilityLoss(attacker *game.Player, heldName string, strength float64) {
	if strength >= 0.9 {
		slot := int(attacker.HeldSlot) + 36
		invItem := &attacker.Inventory[slot]
		if invItem.MaxDurability > 0 {
			itemName := ItemNameByID(invItem.ID)
			if IsSword(itemName) {
				invItem.Durability--
			} else {
				invItem.Durability -= 2
			}
			if invItem.Durability <= 0 {
				*invItem = game.ItemStack{} // tool breaks
			}
			SendSlotUpdate(attacker, slot)
		}
	}
}

// applyThorns checks the target's armor for thorns enchantments and reflects damage to attacker.
// Vanilla formula: each armor piece with thorns has a (level * 15)% chance to trigger,
// dealing (level * 0.5 + 1) damage back to the attacker. Only the highest-level trigger applies.
func (h *CombatHandler) applyThorns(attacker, target *game.Player) {
	if target.Dead {
		return
	}

	highestDamage := float32(0)
	triggered := false
	for _, slot := range []int{5, 6, 7, 8} {
		item := &target.Inventory[slot]
		if item.ID <= 0 || item.Enchantments == nil {
			continue
		}
		thornsLvl := item.Enchantments["thorns"]
		if thornsLvl <= 0 {
			continue
		}
		// level * 15% chance to trigger
		chance := float64(thornsLvl) * 0.15
		if rand.Float64() < chance {
			thornsDmg := float32(thornsLvl)*0.5 + 1.0
			if thornsDmg > highestDamage {
				highestDamage = thornsDmg
			}
			triggered = true
		}
	}

	if triggered && highestDamage > 0 && h.SurvivalHandler != nil {
		// Play thorns hit sound at the attacker's position
		px, py, pz := attacker.Position()
		BroadcastSound(h.Manager, SoundThornsHit, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

		attacker.LastDamageMessage = attacker.Name + " was killed trying to hurt " + target.Name
		h.SurvivalHandler.ApplyDamage(h.Manager, attacker, highestDamage, h.SurvivalHandler.AttackDamageTypeID)
		h.logf("Thorns reflected %.1f damage from %s back to %s", highestDamage, target.Name, attacker.Name)
	}
}

func (h *CombatHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
