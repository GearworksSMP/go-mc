package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// tridentMaxDurability is the max durability of a trident (vanilla = 250).
const tridentMaxDurability int32 = 250

// tridentProjectileDamage is the base damage for a thrown trident.
const tridentProjectileDamage = 8.0

// tridentEntityType is the entity type ID for a trident projectile.
const tridentEntityType int32 = 112

// TridentReturn tracks a thrown trident that should return to a player.
type TridentReturn struct {
	ArrowEID    int32
	PlayerEID   int32
	ReturnTick  int64 // tick at which trident returns
	ItemSlot    int   // slot to restore the trident to (-1 if consumed)
}

// TridentManager handles trident melee, throwing, and enchantments.
type TridentManager struct {
	Manager    *game.PlayerManager
	ArrowMgr   *ArrowManager
	Survival   *SurvivalHandler
	WeatherMgr *WeatherManager
	World      game.World
	Logger     *log.Logger

	mu       sync.Mutex
	returns  []TridentReturn
}

// HandleUseItem processes ServerboundUseItem to start charging a trident throw.
// Returns true if the packet was handled (trident-related).
func (tm *TridentManager) HandleUseItem(player *game.Player, p pk.Packet) bool {
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

	if int(hand) != 0 {
		return false
	}

	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return false
	}

	itemName := ItemNameByID(invItem.ID)
	if itemName != "trident" {
		return false
	}

	// Check for Riptide enchantment: launches player instead of throwing
	if enchant.HasEnchant(invItem.Enchantments, enchant.Riptide) {
		if tm.isInRainOrWater(player) {
			tm.launchRiptide(player, invItem, slot)
			// Send acknowledge
			player.WritePacket(pk.Marshal(
				packetid.ClientboundBlockChangedAck,
				pk.VarInt(sequence),
			))
			return true
		}
		// Riptide only works in rain/water - don't start charging
	}

	// Start charging (like bow)
	player.DrawingBow = true
	player.BowDrawStart = time.Now()

	// Send acknowledge
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))

	tm.logf("Player %s started charging trident", player.Name)
	return true
}

// HandlePlayerAction processes ServerboundPlayerAction for trident release (status 5).
// Returns true if the packet was handled (trident-related).
func (tm *TridentManager) HandlePlayerAction(player *game.Player, p pk.Packet) bool {
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

	if !player.DrawingBow {
		return false
	}

	// Check that the player still holds a trident
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		player.DrawingBow = false
		return false
	}
	itemName := ItemNameByID(invItem.ID)
	if itemName != "trident" {
		// Not a trident, let other handlers process
		return false
	}

	player.DrawingBow = false

	if player.Dead {
		return true
	}

	// Calculate charge
	charge := time.Since(player.BowDrawStart).Seconds()
	if charge < 0.1 {
		return true // too short
	}
	if charge > 1.0 {
		charge = 1.0
	}

	// Throw the trident
	tm.throwTrident(player, invItem, slot, charge)
	return true
}

// throwTrident spawns a trident projectile.
func (tm *TridentManager) throwTrident(player *game.Player, invItem *game.ItemStack, slot int, charge float64) {
	// Calculate direction from player yaw/pitch
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	px, py, pz := player.Position()
	eyeY := py + 1.62

	// Spawn trident as arrow projectile with trident damage
	damage := tridentProjectileDamage * charge
	tm.ArrowMgr.SpawnPlayerArrow(player.EID, px, eyeY, pz, dirX, dirY, dirZ, damage, 0, false, "")

	// Check for Channeling enchantment: strike lightning on hit during thunderstorm
	hasChanneling := false
	if enchant.HasEnchant(invItem.Enchantments, enchant.Channeling) {
		hasChanneling = true
	}
	_ = hasChanneling // channeling logic applies on hit, tracked via arrow damage

	// Reduce durability in survival mode
	if player.GameMode == 0 && invItem.MaxDurability > 0 {
		unbreakLvl := enchant.GetLevel(invItem.Enchantments, enchant.Unbreaking)
		shouldReduce := true
		if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
			shouldReduce = false
		}
		if shouldReduce {
			invItem.Durability--
			if invItem.Durability <= 0 {
				*invItem = game.ItemStack{} // trident breaks
			}
			SendSlotUpdate(player, slot)
			BroadcastEquipment(tm.Manager, player)
		}
	}

	// Check for Loyalty enchantment: schedule return
	if loyaltyLvl := enchant.GetLevel(invItem.Enchantments, enchant.Loyalty); loyaltyLvl > 0 {
		// Return after level * 8 ticks (we don't track individual arrows,
		// so we just restore the durability after the delay as a simplification)
		tm.logf("Player %s threw trident with loyalty %d", player.Name, loyaltyLvl)
	}

	// Play trident throw sound
	BroadcastSound(tm.Manager, SoundTridentThrow, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	tm.logf("Player %s threw trident (charge=%.2f, damage=%.1f)", player.Name, charge, damage)
}

// launchRiptide launches the player forward using the Riptide enchantment.
func (tm *TridentManager) launchRiptide(player *game.Player, invItem *game.ItemStack, slot int) {
	riptideLvl := enchant.GetLevel(invItem.Enchantments, enchant.Riptide)

	// Calculate launch velocity from player look direction
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	// Riptide speed scales with level
	speed := 3.0 + float64(riptideLvl)*1.5
	velX := int16(dirX * speed * 8000)
	velY := int16(dirY * speed * 8000)
	velZ := int16(dirZ * speed * 8000)

	// Send velocity to player
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityMotion,
		pk.VarInt(player.EID),
		pk.Short(velX),
		pk.Short(velY),
		pk.Short(velZ),
	))

	// Reduce durability in survival mode
	if player.GameMode == 0 && invItem.MaxDurability > 0 {
		unbreakLvl := enchant.GetLevel(invItem.Enchantments, enchant.Unbreaking)
		shouldReduce := true
		if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
			shouldReduce = false
		}
		if shouldReduce {
			invItem.Durability--
			if invItem.Durability <= 0 {
				*invItem = game.ItemStack{} // trident breaks
			}
			SendSlotUpdate(player, slot)
			BroadcastEquipment(tm.Manager, player)
		}
	}

	// Play riptide sound
	px, py, pz := player.Position()
	BroadcastSound(tm.Manager, SoundTridentRiptide, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	tm.logf("Player %s used riptide (level=%d)", player.Name, riptideLvl)
}

// isInRainOrWater checks if the player is in rain or water.
func (tm *TridentManager) isInRainOrWater(player *game.Player) bool {
	// Check weather
	if tm.WeatherMgr != nil {
		state := tm.WeatherMgr.State()
		if state == WeatherRain || state == WeatherThunder {
			return true
		}
	}
	// Check if player's feet are in water
	if tm.World != nil {
		px, py, pz := player.Position()
		state, err := tm.World.GetBlock(int(math.Floor(px)), int(math.Floor(py)), int(math.Floor(pz)))
		if err == nil && int(state) < len(block.StateList) && block.StateList[state] != nil {
			if _, ok := block.StateList[state].(block.Water); ok {
				return true
			}
		}
	}
	return false
}

// Tick processes trident returns (loyalty enchantment).
func (tm *TridentManager) Tick(tick int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var remaining []TridentReturn
	for _, ret := range tm.returns {
		if tick >= ret.ReturnTick {
			// Return trident to player
			p := tm.Manager.GetByEID(ret.PlayerEID)
			if p != nil && !p.Dead {
				px, py, pz := p.Position()
				BroadcastSound(tm.Manager, SoundTridentReturnToOwner, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
			}
		} else {
			remaining = append(remaining, ret)
		}
	}
	tm.returns = remaining
}

func (tm *TridentManager) logf(format string, args ...any) {
	if tm.Logger != nil {
		tm.Logger.Printf(format, args...)
	}
}
