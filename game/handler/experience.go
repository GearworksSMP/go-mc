package handler

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
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
