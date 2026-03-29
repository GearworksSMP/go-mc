package handler

import (
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// SendExperience sends the XP bar, level, and total to the player.
func SendExperience(player *game.Player) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetExperience,
		pk.Float(player.Experience),
		pk.VarInt(player.ExperienceLevel),
		pk.VarInt(player.ExperienceTotal),
	))
}

// AddExperience adds XP to a player and recalculates their level.
func AddExperience(player *game.Player, amount int32) {
	addExperienceInternal(player, amount, nil)
}

// AddExperienceWithSound adds XP and plays pickup/levelup sounds.
func AddExperienceWithSound(manager *game.PlayerManager, player *game.Player, amount int32) {
	addExperienceInternal(player, amount, manager)
}

func addExperienceInternal(player *game.Player, amount int32, manager *game.PlayerManager) {
	if amount <= 0 {
		return
	}
	oldLevel := player.ExperienceLevel
	player.ExperienceTotal += amount

	// Recalculate level and bar progress from total XP
	totalXP := player.ExperienceTotal
	level := int32(0)
	for {
		needed := xpForNextLevel(level)
		if totalXP < needed {
			break
		}
		totalXP -= needed
		level++
	}
	player.ExperienceLevel = level
	needed := xpForNextLevel(level)
	if needed > 0 {
		player.Experience = float32(totalXP) / float32(needed)
	} else {
		player.Experience = 0
	}

	SendExperience(player)

	// Sound effects
	if manager != nil {
		px, py, pz := player.Position()
		// XP pickup sound (random pitch for variety)
		BroadcastSound(manager, SoundXPPickup, SoundCategoryPlayer, px, py, pz, 0.1, 0.5+float32(amount%5)*0.1)
		// Level up sound
		if player.ExperienceLevel > oldLevel {
			BroadcastSound(manager, SoundLevelUp, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
		}
	}
}

// xpForNextLevel returns the XP needed to go from level to level+1.
// Vanilla formula: levels 0-16 = 2*level+7, 17-31 = 5*level-38, 32+ = 9*level-158.
func xpForNextLevel(level int32) int32 {
	switch {
	case level < 16:
		return 2*level + 7
	case level < 31:
		return 5*level - 38
	default:
		return 9*level - 158
	}
}

// entityTypeXPOrb is the entity type ID for experience orbs (26.1-snapshot-2).
const entityTypeXPOrb int32 = 49

// XPOrb represents a floating experience orb entity.
type XPOrb struct {
	EID       int32
	X, Y, Z   float64
	Amount    int32
	SpawnTick int64
}

// XPOrbManager tracks all experience orb entities in the world.
type XPOrbManager struct {
	Manager *game.PlayerManager
	mu      sync.Mutex
	Orbs    map[int32]*XPOrb
	tick    int64
}

// NewXPOrbManager creates a new XPOrbManager.
func NewXPOrbManager(manager *game.PlayerManager) *XPOrbManager {
	return &XPOrbManager{
		Manager: manager,
		Orbs:    make(map[int32]*XPOrb),
	}
}

// SpawnXPOrbs spawns visible XP orb entities at the given position.
// Splits totalXP into 1-3 orbs for visual effect.
func (m *XPOrbManager) SpawnXPOrbs(x, y, z float64, totalXP int32) {
	if totalXP <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Split into 1-3 orbs with slight position jitter
	orbCount := 1
	if totalXP >= 7 {
		orbCount = 3
	} else if totalXP >= 3 {
		orbCount = 2
	}

	remaining := totalXP
	for i := 0; i < orbCount; i++ {
		amount := remaining
		if i < orbCount-1 {
			amount = remaining / int32(orbCount-i)
		}
		remaining -= amount
		if amount <= 0 {
			continue
		}

		ox := x + (rand.Float64()-0.5)*0.5
		oy := y + rand.Float64()*0.3
		oz := z + (rand.Float64()-0.5)*0.5

		eid := m.Manager.NextEntityID()
		orb := &XPOrb{
			EID:       eid,
			X:         ox,
			Y:         oy,
			Z:         oz,
			Amount:    amount,
			SpawnTick: m.tick,
		}
		m.Orbs[eid] = orb

		// Broadcast spawn
		entityUUID := uuid.New()
		addPkt := pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(eid),
			pk.UUID(entityUUID),
			pk.VarInt(entityTypeXPOrb),
			pk.Double(ox),
			pk.Double(oy),
			pk.Double(oz),
			pk.UnsignedByte(0), // LpVec3 zero velocity
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(amount), // data = XP amount
		)
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(addPkt)
		})
	}
}

// Tick processes XP orb movement, collection, and despawn.
func (m *XPOrbManager) Tick(tick int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tick = tick

	var toRemove []int32

	for eid, orb := range m.Orbs {
		// Despawn after 5 minutes
		if tick-orb.SpawnTick > 6000 {
			toRemove = append(toRemove, eid)
			m.broadcastRemove(eid)
			continue
		}

		// Skip collection for the first 10 ticks (pickup delay)
		if tick-orb.SpawnTick < 10 {
			continue
		}

		// Find nearest player within 8 blocks
		var nearest *game.Player
		nearestDist := 8.0
		m.Manager.ForEach(func(p *game.Player) {
			if nearest != nil || p.Dead || p.GameMode == 3 {
				return
			}
			px, py, pz := p.Position()
			dx := px - orb.X
			dy := (py + 0.9) - orb.Y
			dz := pz - orb.Z
			d := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if d < nearestDist {
				nearestDist = d
				nearest = p
			}
		})

		if nearest == nil {
			continue
		}

		// Collect if within 1.5 blocks
		if nearestDist <= 1.5 {
			// Mending: repair equipped items before adding XP
			remaining := applyMending(nearest, orb.Amount)
			if remaining > 0 {
				AddExperienceWithSound(m.Manager, nearest, remaining)
			} else {
				// Still play pickup sound even if all XP went to mending
				px, py, pz := nearest.Position()
				BroadcastSound(m.Manager, SoundXPPickup, SoundCategoryPlayer, px, py, pz, 0.1, 0.5)
			}
			// Send pickup animation
			pickupPkt := pk.Marshal(
				packetid.ClientboundTakeItemEntity,
				pk.VarInt(eid),
				pk.VarInt(nearest.EID),
				pk.VarInt(orb.Amount),
			)
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(eid),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(pickupPkt)
				p.WritePacket(removePkt)
			})
			toRemove = append(toRemove, eid)
			continue
		}

		// Move toward nearest player (0.5 blocks/tick)
		px, py, pz := nearest.Position()
		py += 0.9
		dx := px - orb.X
		dy := py - orb.Y
		dz := pz - orb.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > 0.01 {
			speed := 0.5
			orb.X += dx / dist * speed
			orb.Y += dy / dist * speed
			orb.Z += dz / dist * speed

			// Broadcast position update
			movePkt := pk.Marshal(
				packetid.ClientboundTeleportEntity,
				pk.VarInt(eid),
				pk.Double(orb.X),
				pk.Double(orb.Y),
				pk.Double(orb.Z),
				pk.Double(0), pk.Double(0), pk.Double(0),
				pk.Float(0), pk.Float(0),
				pk.Int(0),
				pk.Boolean(false),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(movePkt)
			})
		}
	}

	for _, eid := range toRemove {
		delete(m.Orbs, eid)
	}
}

func (m *XPOrbManager) broadcastRemove(eid int32) {
	pkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// Ore XP amounts (fixed values for simplicity).
var oreXP = map[string]int32{
	"coal_ore":     1,
	"diamond_ore":  5,
	"lapis_ore":    3,
	"redstone_ore": 2,
	"emerald_ore":  5,
	"copper_ore":   0,
	"iron_ore":     0,
	"gold_ore":     0,
}

// GetOreXP returns the XP awarded for mining an ore block.
func GetOreXP(blockName string) int32 {
	return oreXP[blockName]
}

// applyMending checks equipped items with Mending enchantment and repairs them
// using XP. Each 1 XP = 2 durability. Returns remaining XP not used for repair.
func applyMending(player *game.Player, xpAmount int32) int32 {
	// Eligible slots: offhand (45), armor (5-8), held item (HeldSlot+36)
	candidateSlots := []int{45, 5, 6, 7, 8, int(player.HeldSlot) + 36}
	var mendableSlots []int
	for _, slot := range candidateSlots {
		if slot < 0 || slot >= len(player.Inventory) {
			continue
		}
		item := &player.Inventory[slot]
		if item.ID <= 0 || item.MaxDurability <= 0 {
			continue
		}
		if enchant.GetLevel(item.Enchantments, enchant.Mending) <= 0 {
			continue
		}
		if item.Durability >= item.MaxDurability {
			continue // not damaged
		}
		mendableSlots = append(mendableSlots, slot)
	}
	if len(mendableSlots) == 0 {
		return xpAmount
	}
	// Pick a random mendable item (vanilla behavior)
	slot := mendableSlots[rand.Intn(len(mendableSlots))]
	item := &player.Inventory[slot]
	// Each XP point repairs 2 durability
	repairNeeded := item.MaxDurability - item.Durability
	xpToUse := (repairNeeded + 1) / 2 // ceil(repairNeeded/2)
	if xpToUse > xpAmount {
		xpToUse = xpAmount
	}
	item.Durability += xpToUse * 2
	if item.Durability > item.MaxDurability {
		item.Durability = item.MaxDurability
	}
	SendSlotUpdate(player, slot)
	return xpAmount - xpToUse
}
