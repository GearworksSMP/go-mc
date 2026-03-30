package handler

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
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
	DragonMgr       *EnderDragonManager
	WitherMgr       *WitherManager
	ArmorStandMgr   *ArmorStandManager
	ItemFrameMgr    *ItemFrameManager
	PaintingMgr     *PaintingManager
	LeashMgr        *LeashManager
	Rules           *GameRules
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

	// Spectators cannot interact with entities
	if IsSpectator(player) {
		return
	}

	// Check for boat interaction (right-click to mount)
	if h.BoatMgr != nil && h.BoatMgr.IsBoat(targetEID) {
		h.BoatMgr.MountBoat(player, targetEID)
		return
	}

	// Check for minecart interaction (right-click to mount or interact with variant)
	if h.MinecartMgr != nil && h.MinecartMgr.IsMinecart(targetEID) {
		if !h.MinecartMgr.MountMinecart(player, targetEID) {
			h.MinecartMgr.InteractMinecart(player, targetEID)
		}
		return
	}

	// Check for armor stand interaction (right-click to equip)
	if h.ArmorStandMgr != nil && h.ArmorStandMgr.IsArmorStand(targetEID) {
		h.ArmorStandMgr.HandleInteract(player, targetEID)
		return
	}

	// Check for item frame interaction (right-click to place item/rotate)
	if h.ItemFrameMgr != nil && h.ItemFrameMgr.IsItemFrame(targetEID) {
		h.ItemFrameMgr.HandleInteract(player, targetEID)
		return
	}

	// Check for leash knot interaction (right-click to attach leashed mobs)
	if h.LeashMgr != nil && h.LeashMgr.IsLeashKnot(targetEID) {
		h.LeashMgr.HandleKnotInteract(player, targetEID)
		return
	}

	// Check if player is using a lead on a mob
	if h.LeashMgr != nil && h.MobManager != nil {
		heldSlot := int(player.HeldSlot) + 36
		heldItem := &player.Inventory[heldSlot]
		if heldItem.ID > 0 && ItemNameByID(heldItem.ID) == "lead" {
			if h.MobManager.IsMob(targetEID) {
				h.LeashMgr.AttachLeash(player, targetEID)
				return
			}
		}
	}

	// Check for villager interaction (right-click to trade)
	if h.VillagerMgr != nil && h.VillagerMgr.IsVillager(targetEID) {
		h.VillagerMgr.OpenMerchantUI(player, targetEID)
		return
	}

	// Check for tameable mob interaction (taming, sit toggle, horse mount)
	if h.MobManager != nil && h.MobManager.TryTame(player, targetEID) {
		return
	}

	// Check for nametag use on mob
	if h.MobManager != nil {
		heldSlot := int(player.HeldSlot) + 36
		heldItem := &player.Inventory[heldSlot]
		if heldItem.ID > 0 && ItemNameByID(heldItem.ID) == "name_tag" && heldItem.DisplayName != "" {
			if h.MobManager.NameMob(targetEID, heldItem.DisplayName) {
				if player.GameMode == 0 {
					heldItem.Count--
					if heldItem.Count <= 0 {
						*heldItem = game.ItemStack{}
					}
					SendSlotUpdate(player, heldSlot)
				}
				return
			}
		}
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

// isUndeadMob returns true if the mob type is undead (for Smite enchantment).
func isUndeadMob(typeID int32) bool {
	switch typeID {
	case MobTypeZombie, MobTypeSkeleton, MobTypePhantom,
		MobTypeDrowned, MobTypeHusk, MobTypeStray,
		MobTypeWitherSkeleton, MobTypeZombifiedPiglin:
		return true
	}
	return false
}

// isArthropodMob returns true if the mob type is an arthropod (for Bane of Arthropods).
func isArthropodMob(typeID int32) bool {
	switch typeID {
	case MobTypeSpider, MobTypeCaveSpider, MobTypeSilverfish, MobTypeEndermite, MobTypeBee:
		return true
	}
	return false
}

func (h *CombatHandler) handleAttack(attacker *game.Player, targetEID int32) {
	if attacker.Dead {
		return
	}

	// Spectators cannot attack
	if IsSpectator(attacker) {
		return
	}

	// Check for armor stand attack (destroy it)
	if h.ArmorStandMgr != nil && h.ArmorStandMgr.IsArmorStand(targetEID) {
		h.ArmorStandMgr.RemoveArmorStand(targetEID)
		return
	}

	// Check for item frame attack (pop item or break frame)
	if h.ItemFrameMgr != nil && h.ItemFrameMgr.IsItemFrame(targetEID) {
		h.ItemFrameMgr.HandleAttack(attacker, targetEID)
		return
	}

	// Check for painting attack (break it)
	if h.PaintingMgr != nil && h.PaintingMgr.IsPainting(targetEID) {
		h.PaintingMgr.RemovePainting(targetEID)
		return
	}

	// Check for leash knot attack (break it)
	if h.LeashMgr != nil && h.LeashMgr.IsLeashKnot(targetEID) {
		h.LeashMgr.BreakKnot(targetEID)
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
	if sharpLvl := enchant.GetLevel(heldItem.Enchantments, enchant.Sharpness); sharpLvl > 0 {
		baseDamage += float32(sharpLvl)*0.5 + 0.5
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

	// Critical hit: player must be falling (not on ground), full charge, not sprinting, not blind
	isCritical := !attacker.OnGround && strength >= 1.0 && !attacker.Sprinting && !attacker.Blocking
	if isCritical {
		damage *= 1.5
	}

	attacker.LastAttackTime = time.Now()

	// Broadcast attack indicator to trigger cooldown bar on clients
	attackIndicatorPkt := pk.Marshal(
		packetid.ClientboundEntityEvent,
		pk.Int(attacker.EID),
		pk.Byte(30), // attack indicator
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(attackIndicatorPkt)
	})

	// Determine mob type for type-specific enchantments
	var targetMobType int32
	if h.MobManager != nil {
		targetMobType = h.MobManager.GetMobType(targetEID)
	}

	// Smite: +2.5 * level vs undead mobs
	if targetMobType > 0 {
		if smiteLvl := enchant.GetLevel(heldItem.Enchantments, enchant.Smite); smiteLvl > 0 && isUndeadMob(targetMobType) {
			damage += float32(smiteLvl) * 2.5
		}
		// Bane of Arthropods: +2.5 * level vs arthropods
		if baneLvl := enchant.GetLevel(heldItem.Enchantments, enchant.BaneOfArthropods); baneLvl > 0 && isArthropodMob(targetMobType) {
			damage += float32(baneLvl) * 2.5
		}
	}

	// Knockback enchantment level (adds to base knockback)
	knockbackLevel := enchant.GetLevel(heldItem.Enchantments, enchant.Knockback)
	// Sprinting adds +1 knockback level equivalent
	if attacker.Sprinting {
		knockbackLevel++
	}

	// Fire Aspect: levels for setting targets on fire
	fireAspectLevel := enchant.GetLevel(heldItem.Enchantments, enchant.FireAspect)

	// Looting enchantment level (used for mob drops)
	lootingLevel := enchant.GetLevel(heldItem.Enchantments, enchant.Looting)

	// Broadcast critical hit particle effect
	if isCritical {
		critPkt := pk.Marshal(
			packetid.ClientboundAnimate,
			pk.VarInt(targetEID),
			pk.UnsignedByte(4), // critical effect
		)
		h.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(critPkt)
		})
	}

	// Damage indicator particle at target position (resolved later per-target type)
	h.emitDamageParticles(targetEID, isCritical, len(heldItem.Enchantments) > 0)

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

	// Try ender dragon target
	if h.DragonMgr != nil && h.DragonMgr.IsDragonEID(targetEID) {
		h.DragonMgr.DamageDragon(attacker, damage)
		attacker.Exhaustion += 0.1
		h.applyDurabilityLoss(attacker, heldName, strength)
		h.logf("Player %s attacked Ender Dragon (strength=%.2f, damage=%.1f, crit=%v)", attacker.Name, strength, damage, isCritical)
		return
	}

	// Try wither target
	if h.WitherMgr != nil && h.WitherMgr.IsWitherEID(targetEID) {
		h.WitherMgr.DamageWither(targetEID, attacker, damage)
		attacker.Exhaustion += 0.1
		h.applyDurabilityLoss(attacker, heldName, strength)
		h.logf("Player %s attacked Wither (strength=%.2f, damage=%.1f, crit=%v)", attacker.Name, strength, damage, isCritical)
		return
	}

	// Try mob target
	if h.MobManager != nil && h.MobManager.DamageMobEx(attacker, targetEID, damage, knockbackLevel, fireAspectLevel, lootingLevel) {
		attacker.Exhaustion += 0.1

		// Sweep attack: if full charge, standing still, and using a sword,
		// deal reduced damage to nearby mobs
		if strength >= 1.0 && IsSword(heldName) && !attacker.Sprinting {
			h.applySweepAttack(attacker, targetEID, baseDamage)
		}

		h.applyDurabilityLoss(attacker, heldName, strength)
		h.logf("Player %s attacked mob EID=%d (strength=%.2f, damage=%.1f, crit=%v)", attacker.Name, targetEID, strength, damage, isCritical)
		return
	}

	// Try player target
	target := h.Manager.GetByEID(targetEID)
	if target == nil || target.Dead {
		return
	}

	// PvP check: if pvp is disabled, don't allow player-vs-player damage
	if h.Rules != nil && !h.Rules.GetPvP() {
		return
	}

	if h.SurvivalHandler != nil {
		h.SurvivalHandler.ApplyDamageFrom(h.Manager, target, damage, h.SurvivalHandler.AttackDamageTypeID, attacker.Name)
	}

	// Fire Aspect: set target player on fire (4 seconds per level)
	if fireAspectLevel > 0 {
		fireTicks := fireAspectLevel * 80 // 4 seconds per level at 20 TPS
		h.setPlayerOnFire(target, fireTicks)
	}

	// Add exhaustion for attacking
	attacker.Exhaustion += 0.1

	// Knockback: push target away from attacker
	h.applyKnockbackToPlayer(attacker, target, knockbackLevel)

	// Thorns enchantment: check target's armor for thorns, reflect damage back to attacker
	h.applyThorns(attacker, target)

	// Sweep attack for PvP
	if strength >= 1.0 && IsSword(heldName) && !attacker.Sprinting {
		h.applySweepAttackPvP(attacker, target, baseDamage)
	}

	// Axe disables shield for 5 seconds
	if IsAxe(heldName) && target.Blocking {
		target.Blocking = false
		target.ShieldCooldownUntil = time.Now().Add(5 * time.Second)
		tx, ty, tz := target.Position()
		BroadcastSound(h.Manager, SoundShieldBlock, SoundCategoryPlayer, tx, ty, tz, 1.0, 1.0)
	}

	h.applyDurabilityLoss(attacker, heldName, strength)
	h.logf("Player %s attacked %s (strength=%.2f, damage=%.1f, crit=%v)", attacker.Name, target.Name, strength, damage, isCritical)
}

// applyKnockbackToPlayer pushes the target player away from the attacker.
func (h *CombatHandler) applyKnockbackToPlayer(attacker, target *game.Player, kbLevel int32) {
	ax, _, az := attacker.Position()
	tx, _, tz := target.Position()
	dx := tx - ax
	dz := tz - az
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist <= 0 {
		return
	}

	// Base knockback + enchantment bonus (each level adds ~3 blocks of velocity)
	baseScale := 4000.0
	enchantBonus := float64(kbLevel) * 3000.0
	scale := (baseScale + enchantBonus) / dist

	knockbackMult := 1.0
	if target.Blocking {
		knockbackMult = 0.2
	}

	// Netherite knockback resistance
	totalKBResist := 0.0
	for _, slot := range []int{5, 6, 7, 8} {
		totalKBResist += GetKnockbackResistance(ItemNameByID(target.Inventory[slot].ID))
	}
	if totalKBResist > 1.0 {
		totalKBResist = 1.0
	}
	knockbackMult *= (1 - totalKBResist)

	velX := int16(dx * scale * knockbackMult)
	velY := int16((3000.0 + float64(kbLevel)*1500.0) * knockbackMult)
	velZ := int16(dz * scale * knockbackMult)

	target.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityMotion,
		pk.VarInt(target.EID),
		pk.Short(velX),
		pk.Short(velY),
		pk.Short(velZ),
	))
}

// setPlayerOnFire sets fire ticks on a player and broadcasts the visual fire state.
func (h *CombatHandler) setPlayerOnFire(target *game.Player, fireTicks int32) {
	target.FireTicks = fireTicks
	broadcastFireMetadata(h.Manager, target.EID, true)
}

// broadcastFireMetadata sends the on-fire entity flag to all players.
func broadcastFireMetadata(manager *game.PlayerManager, eid int32, onFire bool) {
	flags := byte(0)
	if onFire {
		flags = 0x01
	}
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityData,
			pk.VarInt(eid),
			pk.UnsignedByte(0), // index 0: base entity flags
			pk.VarInt(0),       // type: byte
			pk.Byte(flags),
			pk.UnsignedByte(0xFF),
		))
	})
}

// applySweepAttack deals reduced damage to mobs near the primary target.
// Vanilla: 1 + Sweeping Edge enchant bonus damage, in a 1-block radius around the target.
func (h *CombatHandler) applySweepAttack(attacker *game.Player, primaryEID int32, baseDamage float32) {
	if h.MobManager == nil {
		return
	}

	sweepDamage := float32(1.0)
	heldItem := &attacker.Inventory[attacker.HeldSlot+36]
	if seLvl := enchant.GetLevel(heldItem.Enchantments, enchant.SweepingEdge); seLvl > 0 {
		sweepDamage += baseDamage * float32(seLvl) / float32(seLvl+1)
	}

	h.MobManager.DamageMobsNearExcept(attacker, primaryEID, sweepDamage, 1.5)

	// Play sweep sound
	px, py, pz := attacker.Position()
	BroadcastSound(h.Manager, SoundSweepAttack, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Sweep particle
	sweepPkt := pk.Marshal(
		packetid.ClientboundAnimate,
		pk.VarInt(attacker.EID),
		pk.UnsignedByte(5), // sweep attack
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(sweepPkt)
	})
}

// applySweepAttackPvP deals reduced sweep damage to other players near the primary target.
func (h *CombatHandler) applySweepAttackPvP(attacker, primaryTarget *game.Player, baseDamage float32) {
	sweepDamage := float32(1.0)
	heldItem := &attacker.Inventory[attacker.HeldSlot+36]
	if seLvl := enchant.GetLevel(heldItem.Enchantments, enchant.SweepingEdge); seLvl > 0 {
		sweepDamage += baseDamage * float32(seLvl) / float32(seLvl+1)
	}

	tx, ty, tz := primaryTarget.Position()
	h.Manager.ForEach(func(p *game.Player) {
		if p.EID == attacker.EID || p.EID == primaryTarget.EID || p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - tx
		dy := py - ty
		dz := pz - tz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist <= 1.5 {
			if h.SurvivalHandler != nil {
				h.SurvivalHandler.ApplyDamageFrom(h.Manager, p, sweepDamage, h.SurvivalHandler.AttackDamageTypeID, attacker.Name)
			}
		}
	})
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
		if item.ID <= 0 {
			continue
		}
		thornsLvl := enchant.GetLevel(item.Enchantments, enchant.Thorns)
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

// emitDamageParticles sends damage indicator (and crit/enchanted hit) particles at the target entity.
func (h *CombatHandler) emitDamageParticles(targetEID int32, isCritical, enchanted bool) {
	var x, y, z float64
	if h.MobManager != nil {
		h.MobManager.mu.Lock()
		if mob, ok := h.MobManager.Mobs[targetEID]; ok {
			x, y, z = mob.X, mob.Y+1.0, mob.Z
		}
		h.MobManager.mu.Unlock()
	}
	if x == 0 && y == 0 && z == 0 {
		if target := h.Manager.GetByEID(targetEID); target != nil {
			x, y, z = target.Position()
			y += 1.0
		}
	}
	if x == 0 && y == 0 && z == 0 {
		return
	}

	BroadcastParticle(h.Manager, ParticleDamageIndicator, x, y, z, 0.1, 0.2, 0.1, 0.2, 3)

	if isCritical {
		BroadcastParticle(h.Manager, ParticleCrit, x, y, z, 0.3, 0.5, 0.3, 0.4, 8)
	}
	if enchanted {
		BroadcastParticle(h.Manager, ParticleEnchantedHit, x, y, z, 0.3, 0.3, 0.3, 0.5, 10)
	}
}

func (h *CombatHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
