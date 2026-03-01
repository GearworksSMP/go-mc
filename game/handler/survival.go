package handler

import (
	"log"
	"math"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SurvivalHandler manages health, damage, hunger, and related survival mechanics.
type SurvivalHandler struct {
	Logger           *log.Logger
	FallDamageTypeID int32
	AttackDamageTypeID int32
	VoidDamageTypeID int32
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

	// Update health HUD
	SendSetHealth(player)

	if player.Health <= 0 {
		s.handleDeath(manager, player)
	}
}

// handleDeath handles player death: sends combat kill and death animation.
func (s *SurvivalHandler) handleDeath(manager *game.PlayerManager, player *game.Player) {
	player.Dead = true
	player.Health = 0

	// Send combat kill to the dying player
	deathMsg := chat.Text(player.Name + " died")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerCombatKill,
		pk.VarInt(player.EID),
		deathMsg,
	))

	// Broadcast entity event (death animation = event 3) to all players
	// EntityEvent uses Int (not VarInt) for entity ID and Byte for event
	eidBytes := [4]byte{
		byte(player.EID >> 24), byte(player.EID >> 16),
		byte(player.EID >> 8), byte(player.EID),
	}
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(player.EID),
			pk.Byte(3), // death
		))
	})
	_ = eidBytes

	s.logf("Player %s died", player.Name)
}

// Kill forces a player death regardless of game mode.
func (s *SurvivalHandler) Kill(manager *game.PlayerManager, player *game.Player) {
	player.Health = 0
	s.handleDeath(manager, player)
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
			s.Kill(manager, p)
			s.logf("Player %s fell out of the world", p.Name)
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
	Logger *log.Logger
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

	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return true
	}

	itemName := ItemNameByID(invItem.ID)
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
