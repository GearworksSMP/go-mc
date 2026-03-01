package handler

import (
	"log"
	"math"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// CombatHandler handles player-vs-player attacks via ServerboundInteract.
type CombatHandler struct {
	Manager         *game.PlayerManager
	SurvivalHandler *SurvivalHandler
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
	}
	return true
}

func (h *CombatHandler) handleAttack(attacker *game.Player, targetEID int32) {
	if attacker.Dead {
		return
	}

	target := h.Manager.GetByEID(targetEID)
	if target == nil || target.Dead {
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

	// Vanilla damage formula: damage = baseDamage * (0.2 + strength^2 * 0.8)
	damage := baseDamage * float32(0.2+strength*strength*0.8)

	attacker.LastAttackTime = time.Now()

	if h.SurvivalHandler != nil {
		h.SurvivalHandler.ApplyDamage(h.Manager, target, damage, h.SurvivalHandler.AttackDamageTypeID)
	}

	// Add exhaustion for attacking
	attacker.Exhaustion += 0.1

	// Knockback: push target away from attacker
	ax, _, az := attacker.Position()
	tx, _, tz := target.Position()
	dx := tx - ax
	dz := tz - az
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist > 0 {
		scale := 4000.0 / dist
		velX := int16(dx * scale)
		velY := int16(3000)
		velZ := int16(dz * scale)

		target.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(target.EID),
			pk.Short(velX),
			pk.Short(velY),
			pk.Short(velZ),
		))
	}

	// Only apply durability loss when strength >= 0.9 (near-full charge)
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

	h.logf("Player %s attacked %s (strength=%.2f, damage=%.1f)", attacker.Name, target.Name, strength, damage)
}

func (h *CombatHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
