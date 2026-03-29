package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level/block"
)

// CheckFrostWalker turns water source blocks near the player's feet into frosted ice.
// Radius is 2 + enchantment level. Only affects still water at y-1.
func CheckFrostWalker(manager *game.PlayerManager, world game.World, x, y, z int, level int32) {
	if level <= 0 {
		return
	}
	radius := int(2 + level)
	iceY := y - 1
	frostedID, ok := block.ToStateID[block.FrostedIce{}]
	if !ok {
		return
	}

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			if dx*dx+dz*dz > radius*radius {
				continue
			}
			bx := x + dx
			bz := z + dz
			state, err := world.GetBlock(bx, iceY, bz)
			if err != nil {
				continue
			}
			if int(state) < len(block.StateList) && block.StateList[state] != nil {
				if w, isWater := block.StateList[state].(block.Water); isWater && int(w.Level) == 0 {
					world.SetBlock(bx, iceY, bz, frostedID)
					broadcastBlockUpdateDirect(manager, bx, iceY, bz, int32(frostedID))
				}
			}
		}
	}
}

// DepthStriderSpeedMultiplier returns the speed multiplier bonus from Depth Strider.
// At level 3, returns 1.0 (full land speed). At level 1, returns ~0.333.
func DepthStriderSpeedMultiplier(level int32) float64 {
	if level <= 0 {
		return 0
	}
	if level > 3 {
		level = 3
	}
	return float64(level) / 3.0
}

// CheckSoulSpeed checks if the block below the player is soul sand or soul soil.
// If active, has a 4% chance per call to consume 1 boot durability.
func CheckSoulSpeed(player *game.Player, world game.World, x, y, z int, level int32) bool {
	if level <= 0 {
		return false
	}
	state, err := world.GetBlock(x, y-1, z)
	if err != nil {
		return false
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return false
	}
	switch block.StateList[state].(type) {
	case block.SoulSand, block.SoulSoil:
		if rand.Float32() < 0.04 {
			boots := &player.Inventory[8]
			if boots.Count > 0 && boots.MaxDurability > 0 && boots.Durability > 0 {
				boots.Durability--
			}
		}
		return true
	}
	return false
}

// SoulSpeedBonus returns the movement speed bonus from Soul Speed.
func SoulSpeedBonus(level int32) float64 {
	return 0.0105 * float64(level)
}

// CheckAquaAffinity returns true if the player's helmet (slot 5) has Aqua Affinity,
// which removes the underwater mining speed penalty.
func CheckAquaAffinity(player *game.Player) bool {
	return enchant.HasEnchant(player.Inventory[5].Enchantments, enchant.AquaAffinity)
}

// SwiftSneakSpeedFraction returns the fraction of walking speed when sneaking
// with Swift Sneak. Vanilla sneaking is 0.3; each level adds 0.15, capped at 1.0.
func SwiftSneakSpeedFraction(level int32) float64 {
	result := 0.3 + 0.15*float64(level)
	if result > 1.0 {
		return 1.0
	}
	return result
}

// getBootsEnchantLevel returns the enchantment level on the player's boots (slot 8).
func getBootsEnchantLevel(player *game.Player, name string) int32 {
	return enchant.GetLevel(player.Inventory[8].Enchantments, name)
}

// getLeggingsEnchantLevel returns the enchantment level on the player's leggings (slot 7).
func getLeggingsEnchantLevel(player *game.Player, name string) int32 {
	return enchant.GetLevel(player.Inventory[7].Enchantments, name)
}

// checkMovementEnchantments runs all movement-related enchantment checks.
// Called every ~4 position updates for performance.
func (h *MovementHandler) checkMovementEnchantments(player *game.Player, x, y, z float64) {
	if player.Dead || player.GameMode == 1 || player.GameMode == 3 {
		return
	}

	w := h.worldForPlayer(player)
	bx := int(math.Floor(x))
	by := int(math.Floor(y))
	bz := int(math.Floor(z))

	// Frost Walker
	fwLevel := getBootsEnchantLevel(player, enchant.FrostWalker)
	if fwLevel > 0 && player.OnGround {
		CheckFrostWalker(h.Manager, w, bx, by, bz, fwLevel)
	}

	// Depth Strider
	dsLevel := getBootsEnchantLevel(player, enchant.DepthStrider)
	if dsLevel > 0 && player.InWater {
		player.DepthStriderLevel = dsLevel
	} else {
		player.DepthStriderLevel = 0
	}

	// Soul Speed
	ssLevel := getBootsEnchantLevel(player, enchant.SoulSpeed)
	player.SoulSpeedActive = CheckSoulSpeed(player, w, bx, by, bz, ssLevel)

	// Swift Sneak
	snLevel := getLeggingsEnchantLevel(player, enchant.SwiftSneak)
	if snLevel > 0 && player.Sneaking {
		player.SwiftSneakLevel = snLevel
	} else {
		player.SwiftSneakLevel = 0
	}
}
