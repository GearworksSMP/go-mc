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

// BowManager handles bow drawing and arrow shooting.
type BowManager struct {
	Manager  *game.PlayerManager
	ArrowMgr *ArrowManager
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// HandleUseItem processes ServerboundUseItem to start drawing a bow.
// Returns true if the packet was handled (bow-related).
func (bm *BowManager) HandleUseItem(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundUseItem {
		return false
	}

	var hand pk.VarInt
	var sequence pk.VarInt
	if err := p.Scan(&hand, &sequence); err != nil {
		return false
	}

	if player.Dead {
		return false
	}

	// Only check main hand (hand=0)
	if int(hand) != 0 {
		return false
	}

	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return false
	}

	itemName := ItemNameByID(invItem.ID)
	if itemName != "bow" {
		return false
	}

	// Check if player has arrows in inventory
	if findArrowSlot(player) < 0 {
		// Check for infinity enchant (infinity doesn't require arrows in vanilla,
		// but the player still needs at least one arrow to shoot)
		return false
	}

	player.DrawingBow = true
	player.BowDrawStart = time.Now()

	// Send acknowledge with sequence
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))

	return true
}

// HandlePlayerAction processes ServerboundPlayerAction for bow release (status 5).
// Returns true if the packet was handled (bow-related).
func (bm *BowManager) HandlePlayerAction(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundPlayerAction {
		return false
	}

	var action pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&action, &pos, &face, &sequence); err != nil {
		return false
	}

	// Action 5 = release use item (finish drawing bow)
	if action != 5 {
		return false
	}

	if !player.DrawingBow {
		return false
	}

	// Check that the player still holds a bow before consuming the DrawingBow state
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return false
	}
	itemName := ItemNameByID(invItem.ID)
	if itemName != "bow" {
		return false // not a bow, let other handlers process
	}

	player.DrawingBow = false

	if player.Dead {
		return true
	}

	// Calculate charge (0.0 to 1.0)
	charge := time.Since(player.BowDrawStart).Seconds()
	if charge > 1.0 {
		charge = 1.0
	}
	if charge < 0.1 {
		return true // too short, ignore
	}

	// Check if player has arrows
	arrowSlot := findArrowSlot(player)
	if arrowSlot < 0 {
		return true
	}

	// Base damage = 1 + charge*5 (so max 6 at full charge)
	damage := 1.0 + charge*5.0

	// Check bow enchantments
	hasInfinity := false
	punchLevel := int32(0)
	onFire := false
	// Power enchantment: damage *= (1 + 0.25*level)
	if powerLvl := enchant.GetLevel(invItem.Enchantments, enchant.Power); powerLvl > 0 {
		damage *= 1.0 + 0.25*float64(powerLvl)
	}
	// Infinity enchantment: don't consume arrows
	if enchant.HasEnchant(invItem.Enchantments, enchant.Infinity) {
		hasInfinity = true
	}
	// Punch enchantment: extra knockback on hit
	punchLevel = enchant.GetLevel(invItem.Enchantments, enchant.Punch)
	// Flame enchantment: arrow sets target on fire
	if enchant.HasEnchant(invItem.Enchantments, enchant.Flame) {
		onFire = true
	}

	// Consume one arrow (unless infinity)
	if !hasInfinity && player.GameMode == 0 {
		player.Inventory[arrowSlot].Count--
		if player.Inventory[arrowSlot].Count <= 0 {
			player.Inventory[arrowSlot] = game.ItemStack{}
		}
		SendSlotUpdate(player, arrowSlot)
	}

	// Calculate direction from player yaw/pitch
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	// Spawn arrow at eye level
	px, py, pz := player.Position()
	bm.ArrowMgr.SpawnPlayerArrow(player.EID, px, py+1.62, pz, dirX, dirY, dirZ, damage, punchLevel, onFire)

	// Reduce bow durability in survival mode
	if player.GameMode == 0 && invItem.MaxDurability > 0 {
		// Unbreaking enchant: skip durability loss with probability level/(level+1)
		unbreakLvl := enchant.GetLevel(invItem.Enchantments, enchant.Unbreaking)
		shouldReduce := true
		if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
			shouldReduce = false
		}
		if shouldReduce {
			invItem.Durability--
			if invItem.Durability <= 0 {
				*invItem = game.ItemStack{} // bow breaks
			}
			SendSlotUpdate(player, slot)
			BroadcastEquipment(bm.Manager, player)
		}
	}

	// Play arrow shoot sound
	BroadcastSound(bm.Manager, SoundArrowShoot, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	bm.logf("Player %s shot arrow (charge=%.2f, damage=%.1f)", player.Name, charge, damage)
	return true
}

// findArrowSlot scans the player's inventory for an arrow item.
// Returns the slot index (9-44 main + hotbar, or 45 offhand), or -1 if not found.
func findArrowSlot(player *game.Player) int {
	// Check offhand first
	if ItemNameByID(player.Inventory[45].ID) == "arrow" && player.Inventory[45].Count > 0 {
		return 45
	}
	// Check main inventory and hotbar
	for i := 9; i <= 44; i++ {
		if ItemNameByID(player.Inventory[i].ID) == "arrow" && player.Inventory[i].Count > 0 {
			return i
		}
	}
	return -1
}

func (bm *BowManager) logf(format string, args ...any) {
	if bm.Logger != nil {
		bm.Logger.Printf(format, args...)
	}
}
