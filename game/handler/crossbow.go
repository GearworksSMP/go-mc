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

// crossbowMaxDurability is the max durability of a crossbow (vanilla = 465).
const crossbowMaxDurability int32 = 465

// crossbowLoadTime is the base load time for a crossbow in seconds.
const crossbowLoadTime = 1.25

// CrossbowManager handles crossbow loading, charging, and firing.
type CrossbowManager struct {
	Manager  *game.PlayerManager
	ArrowMgr *ArrowManager
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// HandleUseItem processes ServerboundUseItem for crossbow actions.
// If the crossbow is loaded, it fires. Otherwise it starts loading.
// Returns true if the packet was handled (crossbow-related).
func (cm *CrossbowManager) HandleUseItem(player *game.Player, p pk.Packet) bool {
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
	if itemName != "crossbow" {
		return false
	}

	// Send acknowledge with sequence
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))

	// If crossbow is already loaded, fire it
	if player.CrossbowLoaded {
		cm.fireCrossbow(player, invItem, slot)
		return true
	}

	// Check if player has arrows in inventory
	if findArrowSlot(player) < 0 {
		return true
	}

	// Start loading
	player.LoadingCrossbow = true
	player.CrossbowLoadStart = time.Now()

	cm.logf("Player %s started loading crossbow", player.Name)
	return true
}

// HandlePlayerAction processes ServerboundPlayerAction for crossbow release (status 5).
// Returns true if the packet was handled (crossbow-related).
func (cm *CrossbowManager) HandlePlayerAction(player *game.Player, p pk.Packet) bool {
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

	// Action 5 = release use item
	if action != 5 {
		return false
	}

	if !player.LoadingCrossbow {
		return false
	}
	player.LoadingCrossbow = false

	if player.Dead {
		return false
	}

	// Check that the player still holds a crossbow
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return true
	}
	itemName := ItemNameByID(invItem.ID)
	if itemName != "crossbow" {
		return true
	}

	// Calculate load time with Quick Charge enchantment
	loadTime := crossbowLoadTime
	if qcLvl := enchant.GetLevel(invItem.Enchantments, enchant.QuickCharge); qcLvl > 0 {
		loadTime -= 0.25 * float64(qcLvl)
		if loadTime < 0 {
			loadTime = 0
		}
	}

	// Check if enough time has passed to finish loading
	elapsed := time.Since(player.CrossbowLoadStart).Seconds()
	if elapsed < loadTime {
		cm.logf("Player %s released crossbow too early (%.2f/%.2fs)", player.Name, elapsed, loadTime)
		return true
	}

	// Crossbow is now loaded
	player.CrossbowLoaded = true

	// Play crossbow loading done sound
	px, py, pz := player.Position()
	BroadcastSound(cm.Manager, SoundCrossbowLoadingEnd, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	cm.logf("Player %s loaded crossbow (%.2fs)", player.Name, elapsed)
	return true
}

// fireCrossbow fires the loaded crossbow, spawning arrows.
func (cm *CrossbowManager) fireCrossbow(player *game.Player, invItem *game.ItemStack, slot int) {
	player.CrossbowLoaded = false

	// Check if player has arrows
	arrowSlot := findArrowSlot(player)
	if arrowSlot < 0 {
		return
	}

	// Base arrow damage for crossbow: 6-11 (average 8.5)
	damage := 8.0

	// Calculate direction from player yaw/pitch
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	px, py, pz := player.Position()
	eyeY := py + 1.62

	// Check for Multishot enchantment
	multishotCount := 1
	if enchant.HasEnchant(invItem.Enchantments, enchant.Multishot) {
		multishotCount = 3
	}

	// Check for Piercing enchantment
	piercingLvl := enchant.GetLevel(invItem.Enchantments, enchant.Piercing)

	// Consume one arrow (multishot still only consumes one)
	if player.GameMode == 0 {
		player.Inventory[arrowSlot].Count--
		if player.Inventory[arrowSlot].Count <= 0 {
			player.Inventory[arrowSlot] = game.ItemStack{}
		}
		SendSlotUpdate(player, arrowSlot)
	}

	// Spawn arrows
	for i := 0; i < multishotCount; i++ {
		dx, dy, dz := dirX, dirY, dirZ
		if multishotCount == 3 && i != 1 {
			// Offset by +/-10 degrees for multishot side arrows
			angleOffset := 10.0 * math.Pi / 180.0
			if i == 0 {
				angleOffset = -angleOffset
			}
			cos := math.Cos(angleOffset)
			sin := math.Sin(angleOffset)
			// Rotate direction around Y axis
			dx = dirX*cos - dirZ*sin
			dz = dirX*sin + dirZ*cos
		}
		cm.ArrowMgr.SpawnPlayerArrow(player.EID, px, eyeY, pz, dx, dy, dz, damage, 0, false, piercingLvl)
	}

	// Reduce crossbow durability in survival mode
	if player.GameMode == 0 && invItem.MaxDurability > 0 {
		unbreakLvl := enchant.GetLevel(invItem.Enchantments, enchant.Unbreaking)
		shouldReduce := true
		if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
			shouldReduce = false
		}
		if shouldReduce {
			invItem.Durability--
			if invItem.Durability <= 0 {
				*invItem = game.ItemStack{} // crossbow breaks
			}
			SendSlotUpdate(player, slot)
			BroadcastEquipment(cm.Manager, player)
		}
	}

	// Play crossbow shoot sound
	BroadcastSound(cm.Manager, SoundCrossbowShoot, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	cm.logf("Player %s fired crossbow (arrows=%d, damage=%.1f)", player.Name, multishotCount, damage)
}

func (cm *CrossbowManager) logf(format string, args ...any) {
	if cm.Logger != nil {
		cm.Logger.Printf(format, args...)
	}
}
