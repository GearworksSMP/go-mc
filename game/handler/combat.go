package handler

import (
	"log"
	"math"

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

	// Apply 1.0 damage (bare hand)
	if h.SurvivalHandler != nil {
		h.SurvivalHandler.ApplyDamage(h.Manager, target, 1.0, h.SurvivalHandler.AttackDamageTypeID)
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
		// Normalize and scale to knockback velocity
		// Minecraft velocity: 1 block/tick = 8000 units
		scale := 4000.0 / dist
		velX := int16(dx * scale)
		velY := int16(3000) // upward knockback
		velZ := int16(dz * scale)

		target.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityMotion,
			pk.VarInt(target.EID),
			pk.Short(velX),
			pk.Short(velY),
			pk.Short(velZ),
		))
	}

	h.logf("Player %s attacked %s", attacker.Name, target.Name)
}

func (h *CombatHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
