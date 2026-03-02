package handler

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SurvivalHandler manages health, damage, hunger, and related survival mechanics.
type SurvivalHandler struct {
	Logger             *log.Logger
	FallDamageTypeID   int32
	AttackDamageTypeID int32
	VoidDamageTypeID   int32
	ItemEntities       *ItemEntityManager
	KeepInventory      *bool
	EffectMgr          *EffectManager // status effects (set after construction)
}

// SendSetHealth sends the health/food/saturation HUD update to a player.
func SendSetHealth(player *game.Player) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetHealth,
		pk.Float(player.Health),
		pk.VarInt(player.Food),
		pk.Float(player.Saturation),
	))
}

// ApplyDamage reduces player health, broadcasts hurt animation, and handles death.
func (s *SurvivalHandler) ApplyDamage(manager *game.PlayerManager, player *game.Player, damage float32, damageTypeID int32) {
	if player.Dead || player.IsInvulnerable() {
		return
	}

	// Shield blocking: absorbs all damage (with 250ms cooldown between blocks)
	if player.Blocking && isHoldingShield(player) && time.Now().After(player.ShieldCooldownUntil) {
		reduceShieldDurability(player)
		player.ShieldCooldownUntil = time.Now().Add(250 * time.Millisecond)
		// Play shield block sound
		px, py, pz := player.Position()
		BroadcastSound(manager, SoundShieldBlock, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
		return
	}

	// Calculate armor protection
	armorPoints := 0
	for _, slot := range []int{5, 6, 7, 8} {
		armorPoints += GetArmorProtection(ItemNameByID(player.Inventory[slot].ID))
	}
	if armorPoints > 0 {
		reduction := float32(armorPoints) / 25.0
		if reduction > 0.8 {
			reduction = 0.8
		}
		damage *= (1 - reduction)
	}

	// Protection enchantment: sum from all armor pieces, 4% reduction per level (cap 80%)
	protectionTotal := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		if player.Inventory[slot].Enchantments != nil {
			protectionTotal += player.Inventory[slot].Enchantments["protection"]
		}
	}
	if protectionTotal > 0 {
		protReduction := float32(protectionTotal) * 0.04
		if protReduction > 0.8 {
			protReduction = 0.8
		}
		damage *= (1 - protReduction)
	}

	// TODO: Fire Protection enchantment — when fire damage types are implemented,
	// sum "fire_protection" levels from all armor pieces and apply 8% reduction per level (cap 80%).
	// TODO: Blast Protection enchantment — when explosion damage types are implemented,
	// sum "blast_protection" levels from all armor pieces and apply 8% reduction per level (cap 80%).

	// Resistance effect: 20% reduction per level (cap 100%)
	if s.EffectMgr != nil {
		resReduction := s.EffectMgr.GetResistanceReduction(player)
		if resReduction > 0 {
			damage *= (1 - resReduction)
		}
	}

	// Absorption: absorb damage from extra HP pool first
	if player.Absorption > 0 {
		if damage <= player.Absorption {
			player.Absorption -= damage
			damage = 0
		} else {
			damage -= player.Absorption
			player.Absorption = 0
		}
	}

	player.Health -= damage
	if player.Health < 0 {
		player.Health = 0
	}

	// Add exhaustion from taking damage
	player.Exhaustion += 0.1

	// Send hurt animation to all players
	hurtPkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(player.EID),
		pk.Float(0), // yaw
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(hurtPkt)
	})

	// Send damage event to all players
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(player.EID),
		pk.VarInt(damageTypeID),
		pk.VarInt(0), // no cause entity
		pk.VarInt(0), // no direct entity
		pk.Boolean(false), // no source position
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play player hurt sound
	px, py, pz := player.Position()
	BroadcastSound(manager, SoundPlayerHurt, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Update health HUD
	SendSetHealth(player)

	// Update health display above player
	BroadcastHealthTag(manager, player)

	// Reduce armor durability (unbreaking enchant: skip with probability level/(level+1))
	armorChanged := false
	for _, slot := range []int{5, 6, 7, 8} {
		if player.Inventory[slot].MaxDurability > 0 && player.Inventory[slot].ID > 0 {
			unbreakLvl := int32(0)
			if player.Inventory[slot].Enchantments != nil {
				unbreakLvl = player.Inventory[slot].Enchantments["unbreaking"]
			}
			if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
				continue // unbreaking saved this durability point
			}
			player.Inventory[slot].Durability--
			if player.Inventory[slot].Durability <= 0 {
				player.Inventory[slot] = game.ItemStack{} // armor breaks
			}
			SendSlotUpdate(player, slot)
			armorChanged = true
		}
	}
	if armorChanged {
		BroadcastEquipment(manager, player)
	}

	if player.Health <= 0 {
		s.handleDeath(manager, player)
	}
}

// ApplyDamageFrom reduces player health from a named attacker and handles death with a PvP message.
func (s *SurvivalHandler) ApplyDamageFrom(manager *game.PlayerManager, player *game.Player, damage float32, damageTypeID int32, attackerName string) {
	player.LastDamageMessage = player.Name + " was slain by " + attackerName
	s.ApplyDamage(manager, player, damage, damageTypeID)
}

// handleDeath handles player death: sends combat kill and death animation.
func (s *SurvivalHandler) handleDeath(manager *game.PlayerManager, player *game.Player) {
	msg := player.Name + " died"
	if player.LastDamageMessage != "" {
		msg = player.LastDamageMessage
		player.LastDamageMessage = ""
	}
	s.handleDeathWithMessage(manager, player, msg)
}

// dropPlayerInventory drops all inventory items as entities and resets XP.
func (s *SurvivalHandler) dropPlayerInventory(manager *game.PlayerManager, player *game.Player) {
	if s.KeepInventory != nil && *s.KeepInventory {
		return
	}
	px, py, pz := player.Position()
	for i := 5; i <= 45; i++ { // armor(5-8) + main(9-35) + hotbar(36-44) + offhand(45)
		item := &player.Inventory[i]
		if item.ID <= 0 || item.Count <= 0 {
			continue
		}
		if s.ItemEntities != nil {
			s.ItemEntities.SpawnItem(manager, px, py+1, pz, item.ID, item.Count, 40)
		}
		*item = game.ItemStack{}
	}
	// Reset XP
	player.Experience = 0
	player.ExperienceLevel = 0
	player.ExperienceTotal = 0
}

// handleDeathWithMessage handles player death with a custom death message.
func (s *SurvivalHandler) handleDeathWithMessage(manager *game.PlayerManager, player *game.Player, deathMessage string) {
	// Drop inventory before marking dead
	s.dropPlayerInventory(manager, player)

	// Clear all status effects on death
	if s.EffectMgr != nil {
		s.EffectMgr.ClearAllEffects(player)
	}

	player.Dead = true
	player.Health = 0

	// Send combat kill to the dying player
	deathMsg := chat.Text(deathMessage)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerCombatKill,
		pk.VarInt(player.EID),
		deathMsg,
	))

	// Broadcast entity event (death animation = event 3) to all players
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(player.EID),
			pk.Byte(3), // death
		))
	})

	// Play player death sound
	px, py, pz := player.Position()
	BroadcastSound(manager, SoundPlayerDeath, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Broadcast death message to all players
	broadcastMsg := chat.Message{Text: deathMessage, Color: "red"}
	deathChatPkt := pk.Marshal(
		packetid.ClientboundSystemChat,
		broadcastMsg,
		pk.Boolean(false),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(deathChatPkt)
	})

	s.logf("Player %s died: %s", player.Name, deathMessage)
}

// Kill forces a player death regardless of game mode.
func (s *SurvivalHandler) Kill(manager *game.PlayerManager, player *game.Player) {
	player.Health = 0
	s.handleDeath(manager, player)
}

// KillWithMessage forces a player death with a custom death message.
func (s *SurvivalHandler) KillWithMessage(manager *game.PlayerManager, player *game.Player, message string) {
	player.Health = 0
	s.handleDeathWithMessage(manager, player, message)
}

// HungerTick processes hunger mechanics for all players.
// Called every tick from the tick loop.
func (s *SurvivalHandler) HungerTick(manager *game.PlayerManager, tick int64) {
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}

		// Process exhaustion → saturation → food depletion
		changed := false
		for p.Exhaustion >= 4.0 {
			p.Exhaustion -= 4.0
			if p.Saturation > 0 {
				p.Saturation -= 1.0
				if p.Saturation < 0 {
					p.Saturation = 0
				}
				changed = true
			} else if p.Food > 0 {
				p.Food--
				changed = true
			}
		}

		// Natural regeneration: when food >= 18, heal 1 HP every 80 ticks
		if p.Food >= 18 && p.Health < 20 && tick%80 == 0 {
			p.Health += 1
			if p.Health > 20 {
				p.Health = 20
			}
			p.Exhaustion += 6.0
			changed = true
		}

		// Starvation: when food = 0, damage 1 HP every 80 ticks (min 1 HP)
		if p.Food == 0 && tick%80 == 0 && p.Health > 1 {
			p.Health -= 1
			changed = true
			// Broadcast hurt animation for starvation
			hurtPkt := pk.Marshal(
				packetid.ClientboundHurtAnimation,
				pk.VarInt(p.EID),
				pk.Float(0),
			)
			manager.ForEach(func(other *game.Player) {
				other.WritePacket(hurtPkt)
			})
		}

		if changed {
			SendSetHealth(p)
			BroadcastHealthTag(manager, p)
		}
	})
}

// VoidDamageTick damages players below the world. Called every tick.
// minY is the world's minimum Y coordinate (e.g. -64).
func (s *SurvivalHandler) VoidDamageTick(manager *game.PlayerManager, minY int) {
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		_, y, _ := p.Position()
		if y < float64(minY)-64 {
			s.KillWithMessage(manager, p, p.Name+" fell out of the world")
		} else if y < float64(minY) {
			s.ApplyDamage(manager, p, 0.2, s.VoidDamageTypeID)
		}
	})
}

// AddSprintExhaustion adds exhaustion for sprint distance.
func AddSprintExhaustion(player *game.Player, dx, dz float64) {
	if !player.Sprinting || player.Dead || player.IsInvulnerable() {
		return
	}
	dist := math.Sqrt(dx*dx + dz*dz)
	player.Exhaustion += float32(dist) * 0.1
}

func (s *SurvivalHandler) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

// FoodHandler handles eating food via ServerboundUseItem with a 1.6s eating animation.
type FoodHandler struct {
	Logger     *log.Logger
	FishingMgr *FishingManager
	PotionMgr  *PotionManager
}

// HandlePacket processes ServerboundUseItem to start eating.
// Returns true if the packet was handled.
func (h *FoodHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundUseItem {
		return false
	}

	var hand pk.VarInt
	var sequence pk.VarInt
	if err := p.Scan(&hand, &sequence); err != nil {
		return true
	}

	if player.GameMode != 0 || player.Dead {
		return true
	}

	// Check offhand for shield when hand=1
	slot := int(player.HeldSlot) + 36
	if int(hand) == 1 {
		slot = 45
	}
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return true
	}

	itemName := ItemNameByID(invItem.ID)

	// Shield use: start blocking
	if itemName == "shield" {
		player.Blocking = true
		return true
	}

	// Fishing rod use: cast or reel in
	if itemName == "fishing_rod" && h.FishingMgr != nil {
		h.FishingMgr.CastRod(player)
		return true
	}

	// Potion and milk bucket handling
	if h.PotionMgr != nil {
		if itemName == "milk_bucket" || itemName == "potion" {
			h.PotionMgr.HandleDrinkPotion(player, itemName)
			return true
		}
		if itemName == "splash_potion" {
			h.PotionMgr.ThrowSplashPotion(player, "healing")
			return true
		}
	}

	food := LookupFood(itemName)
	if food == nil {
		return true // not food
	}

	// Can't eat when food is full (except golden apples)
	if player.Food >= 20 && itemName != "golden_apple" && itemName != "enchanted_golden_apple" {
		return true
	}

	// Start eating animation — actual consumption happens in Tick after 1.6s
	player.EatingStart = time.Now()
	return true
}

// Tick processes eating for all players. Called every tick from the tick loop.
func (h *FoodHandler) Tick(manager *game.PlayerManager) {
	manager.ForEach(func(player *game.Player) {
		if player.EatingStart.IsZero() || player.Dead {
			return
		}

		if time.Since(player.EatingStart) < 1600*time.Millisecond {
			return // still eating
		}

		// Eating complete — consume food
		player.EatingStart = time.Time{}

		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.ID <= 0 || invItem.Count <= 0 {
			return
		}

		itemName := ItemNameByID(invItem.ID)
		food := LookupFood(itemName)
		if food == nil {
			return
		}

		// Apply nutrition
		player.Food += food.Nutrition
		if player.Food > 20 {
			player.Food = 20
		}

		// Apply saturation (capped at food level)
		player.Saturation += food.Saturation
		if player.Saturation > float32(player.Food) {
			player.Saturation = float32(player.Food)
		}

		// Consume item
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)

		// Update health HUD
		SendSetHealth(player)

		h.logf("Player %s ate %s (food=%d, sat=%.1f)", player.Name, itemName, player.Food, player.Saturation)
	})
}

// CancelEating cancels any in-progress eating for a player.
func CancelEating(player *game.Player) {
	player.EatingStart = time.Time{}
}

func (h *FoodHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}

// isHoldingShield returns true if the player has a shield in mainhand or offhand.
func isHoldingShield(player *game.Player) bool {
	offhand := ItemNameByID(player.Inventory[45].ID)
	if offhand == "shield" {
		return true
	}
	mainhand := ItemNameByID(player.Inventory[player.HeldSlot+36].ID)
	return mainhand == "shield"
}

// reduceShieldDurability reduces shield durability by 1.
func reduceShieldDurability(player *game.Player) {
	// Check offhand first, then mainhand
	for _, slot := range []int{45, int(player.HeldSlot) + 36} {
		if ItemNameByID(player.Inventory[slot].ID) == "shield" {
			if player.Inventory[slot].MaxDurability > 0 {
				player.Inventory[slot].Durability--
				if player.Inventory[slot].Durability <= 0 {
					player.Inventory[slot] = game.ItemStack{}
					player.Blocking = false
				}
				SendSlotUpdate(player, slot)
			}
			return
		}
	}
}
