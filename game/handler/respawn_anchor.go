package handler

import (
	"log"
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// RespawnAnchorManager handles respawn anchor block interactions.
type RespawnAnchorManager struct {
	Manager  *game.PlayerManager
	World    game.World
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// UseRespawnAnchor handles right-click on a respawn anchor block.
// Returns true if the interaction was handled.
func (m *RespawnAnchorManager) UseRespawnAnchor(player *game.Player, x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return false
	}

	anchor, ok := block.StateList[state].(block.RespawnAnchor)
	if !ok {
		return false
	}

	charges := int(anchor.Charges)
	held := &player.Inventory[int(player.HeldSlot)+36]
	heldName := ItemNameByID(held.ID)

	// Charge with glowstone
	if heldName == "glowstone" && charges < 4 {
		anchor.Charges = block.Integer(charges + 1)
		newID, ok := block.ToStateID[anchor]
		if !ok {
			return false
		}
		m.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newID))
		held.Count--
		if held.Count <= 0 {
			held.ID = 0
			held.Count = 0
		}
		SendSlotUpdate(player, int(player.HeldSlot)+36)
		return true
	}

	// Set spawn in the Nether
	if player.Dimension == "minecraft:the_nether" && charges > 0 && heldName != "glowstone" {
		player.SpawnX = float64(x) + 0.5
		player.SpawnY = float64(y) + 1.0
		player.SpawnZ = float64(z) + 0.5
		player.HasSpawnPoint = true
		anchor.Charges = block.Integer(charges - 1)
		newID, ok := block.ToStateID[anchor]
		if !ok {
			return false
		}
		m.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newID))

		msg := chat.Message{Text: "Respawn point set", Color: "green"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
		return true
	}

	// Explode outside the Nether (vanilla "Intentional Game Design")
	if player.Dimension != "minecraft:the_nether" {
		m.explode(x, y, z)
		return true
	}

	return false
}

// explode creates an explosion at the respawn anchor position (stronger than TNT).
func (m *RespawnAnchorManager) explode(x, y, z int) {
	m.World.SetBlock(x, y, z, 0)
	broadcastBlockUpdateDirect(m.Manager, x, y, z, 0)

	cx := float64(x) + 0.5
	cy := float64(y) + 0.5
	cz := float64(z) + 0.5
	const blockRadius = 5
	const entityRadius = 8.0

	BroadcastSound(m.Manager, SoundExplode, SoundCategoryBlock, cx, cy, cz, 4.0, 1.0)

	for dx := -blockRadius; dx <= blockRadius; dx++ {
		for dy := -blockRadius; dy <= blockRadius; dy++ {
			for dz := -blockRadius; dz <= blockRadius; dz++ {
				dist := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
				if dist > float64(blockRadius) {
					continue
				}
				bx := x + dx
				by := y + dy
				bz := z + dz

				state, err := m.World.GetBlock(bx, by, bz)
				if err != nil || state == 0 {
					continue
				}

				blockName := BlockNameFromState(int(state))
				if blockName == "" || isBlastResistant(blockName) {
					continue
				}

				destructionChance := 1.0 - (dist / float64(blockRadius+1))
				if rand.Float64() > destructionChance {
					continue
				}

				m.World.SetBlock(bx, by, bz, 0)
				broadcastBlockUpdateDirect(m.Manager, bx, by, bz, 0)
				BroadcastLevelEvent(m.Manager, 2001, bx, by, bz, int32(state))
			}
		}
	}

	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - cx
		dy := (py + 0.9) - cy
		dz := pz - cz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > entityRadius {
			return
		}

		damage := float32(math.Max(1, (1-dist/entityRadius)*50))
		blastProtTotal := int32(0)
		for _, slot := range []int{5, 6, 7, 8} {
			blastProtTotal += enchant.GetLevel(p.Inventory[slot].Enchantments, enchant.BlastProtection)
		}
		if blastProtTotal > 0 {
			reduction := float32(blastProtTotal) * 0.08
			if reduction > 0.8 {
				reduction = 0.8
			}
			damage *= (1 - reduction)
		}

		p.LastDamageMessage = p.Name + " was killed by [Intentional Game Design]" // vanilla death message
		if m.Survival != nil {
			m.Survival.ApplyDamage(m.Manager, p, damage, m.Survival.AttackDamageTypeID)
		}

		if dist > 0.01 {
			scale := 7000.0 * (1 - dist/entityRadius) / dist
			velX := int16(dx * scale)
			velY := int16(3500 * (1 - dist/entityRadius))
			velZ := int16(dz * scale)
			p.WritePacket(pk.Marshal(
				packetid.ClientboundSetEntityMotion,
				pk.VarInt(p.EID),
				pk.Short(velX),
				pk.Short(velY),
				pk.Short(velZ),
			))
		}
	})

	if m.Logger != nil {
		m.Logger.Printf("Respawn anchor exploded at (%d, %d, %d)", x, y, z)
	}
}
