package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (c *CommandExecutor) cmdTime(player *game.Player, args []string) {
	if c.TimeMgr == nil {
		c.sendSystemMsg(player, "Time management is not available", "red")
		return
	}
	if len(args) < 2 {
		c.sendSystemMsg(player, fmt.Sprintf("Current time: %d", c.TimeMgr.GetDayTime()), "green")
		return
	}
	switch strings.ToLower(args[0]) {
	case "set":
		var t int64
		switch strings.ToLower(args[1]) {
		case "day":
			t = 1000
		case "noon":
			t = 6000
		case "sunset", "evening":
			t = 12000
		case "night":
			t = 13000
		case "midnight":
			t = 18000
		default:
			val, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				c.sendSystemMsg(player, "Usage: /time set <day|noon|night|midnight|<ticks>>", "red")
				return
			}
			t = val
		}
		c.TimeMgr.SetDayTime(t)
		c.sendSystemMsg(player, fmt.Sprintf("Set time to %d", t), "green")
	default:
		c.sendSystemMsg(player, "Usage: /time set <day|noon|night|midnight|<ticks>>", "red")
	}
}

func (c *CommandExecutor) cmdWeather(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /weather <clear|rain|thunder> [duration_ticks]", "red")
		return
	}

	var state WeatherState
	var label string
	switch strings.ToLower(args[0]) {
	case "clear":
		state, label = WeatherClear, "Clear"
	case "rain":
		state, label = WeatherRain, "Rain"
	case "thunder":
		state, label = WeatherThunder, "Thunder"
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown weather: %s", args[0]), "red")
		return
	}

	var duration int64
	if len(args) >= 2 {
		n, err := strconv.ParseInt(args[1], 10, 64)
		if err == nil && n > 0 {
			duration = n
		}
	}

	if c.WeatherMgr != nil {
		c.WeatherMgr.SetWeather(state, duration)
	} else {
		// Fallback: just send the packet directly (old behavior)
		var eventType byte
		switch state {
		case WeatherClear:
			eventType = 1
		case WeatherRain:
			eventType = 2
		case WeatherThunder:
			eventType = 2
		}
		pkt := pk.Marshal(
			packetid.ClientboundGameEvent,
			pk.UnsignedByte(eventType),
			pk.Float(0),
		)
		c.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}

	c.sendSystemMsg(player, fmt.Sprintf("Set weather to %s", label), "green")
	c.Logger.Printf("%s set weather to %s", player.Name, label)
}

func (c *CommandExecutor) cmdKill(player *game.Player) {
	c.SurvivalHandler.Kill(c.Manager, player)
	c.Logger.Printf("%s killed themselves", player.Name)
}

func (c *CommandExecutor) cmdClear(player *game.Player, args []string) {
	target := player
	if len(args) >= 1 {
		target = c.Manager.GetByName(args[0])
		if target == nil {
			c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
			return
		}
	}

	cleared := 0
	// Clear main inventory (9-44), armor (5-8), offhand (45)
	for i := 5; i <= 45; i++ {
		if target.Inventory[i].ID > 0 {
			target.Inventory[i] = game.ItemStack{}
			cleared++
		}
	}
	SendFullInventory(target)
	BroadcastEquipment(c.Manager, target)
	c.sendSystemMsg(player, fmt.Sprintf("Cleared %d items from %s", cleared, target.Name), "green")
	c.Logger.Printf("%s cleared %d items from %s", player.Name, cleared, target.Name)
}

func (c *CommandExecutor) cmdEnchant(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /enchant <enchantment> [level]", "red")
		c.sendSystemMsg(player, "  Valid: any enchantment from the registry (e.g. sharpness, protection, efficiency)", "")
		return
	}

	enchName := strings.ToLower(args[0])
	if enchant.MaxLevel(enchName) == 0 {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown enchantment: %s", args[0]), "red")
		return
	}

	level := int32(1)
	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 || int32(n) > enchant.MaxLevel(enchName) {
			c.sendSystemMsg(player, fmt.Sprintf("Level must be between 1 and %d", enchant.MaxLevel(enchName)), "red")
			return
		}
		level = int32(n)
	}

	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		c.sendSystemMsg(player, "You must be holding an item", "red")
		return
	}

	invItem.Enchantments = enchant.ApplyToItem(invItem.Enchantments, enchName, level)

	c.sendSystemMsg(player, fmt.Sprintf("Applied %s %d to held item", enchName, level), "green")
	c.Logger.Printf("%s enchanted held item with %s %d", player.Name, enchName, level)
}

func (c *CommandExecutor) cmdSummon(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /summon <entity_type> [x y z]", "red")
		return
	}

	if c.MobMgr == nil {
		c.sendSystemMsg(player, "Mob spawning not available", "red")
		return
	}

	entityName := strings.ToLower(args[0])
	px, py, pz := player.Position()

	var x, y, z float64
	if len(args) >= 4 {
		var err1, err2, err3 error
		x, err1 = parseCoordRelative(args[1], px)
		y, err2 = parseCoordRelative(args[2], py)
		z, err3 = parseCoordRelative(args[3], pz)
		if err1 != nil || err2 != nil || err3 != nil {
			c.sendSystemMsg(player, "Invalid coordinates", "red")
			return
		}
	} else {
		x, y, z = px, py, pz
	}

	// Look up mob type by name
	typeID := MobTypeByName(entityName)
	if typeID < 0 {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown entity: %s", entityName), "red")
		return
	}

	c.MobMgr.SpawnMobAt(typeID, x, y, z)
	c.sendSystemMsg(player, fmt.Sprintf("Summoned %s at %.1f %.1f %.1f", entityName, x, y, z), "green")
	c.Logger.Printf("%s summoned %s at %.1f %.1f %.1f", player.Name, entityName, x, y, z)
}

func (c *CommandExecutor) cmdSetblock(player *game.Player, args []string) {
	if len(args) < 4 {
		c.sendSystemMsg(player, "Usage: /setblock <x> <y> <z> <block>", "red")
		return
	}

	if c.World == nil {
		c.sendSystemMsg(player, "World not available", "red")
		return
	}

	px, py, pz := player.Position()
	x, err1 := parseCoordRelative(args[0], px)
	y, err2 := parseCoordRelative(args[1], py)
	z, err3 := parseCoordRelative(args[2], pz)
	if err1 != nil || err2 != nil || err3 != nil {
		c.sendSystemMsg(player, "Invalid coordinates", "red")
		return
	}

	blockName := strings.ToLower(args[3])
	stateID := blockStateFromName(blockName)
	if stateID < 0 {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown block: %s", blockName), "red")
		return
	}

	bx, by, bz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	c.World.SetBlock(bx, by, bz, level.BlocksState(stateID))
	broadcastBlockUpdateDirect(c.Manager, bx, by, bz, int32(stateID))
	c.sendSystemMsg(player, fmt.Sprintf("Set block at %d %d %d to %s", bx, by, bz, blockName), "green")
}

func (c *CommandExecutor) cmdFill(player *game.Player, args []string) {
	if len(args) < 7 {
		c.sendSystemMsg(player, "Usage: /fill <x1> <y1> <z1> <x2> <y2> <z2> <block> [replace|hollow|outline]", "red")
		return
	}

	if c.World == nil {
		c.sendSystemMsg(player, "World not available", "red")
		return
	}

	px, py, pz := player.Position()
	x1, err1 := parseCoordRelative(args[0], px)
	y1, err2 := parseCoordRelative(args[1], py)
	z1, err3 := parseCoordRelative(args[2], pz)
	x2, err4 := parseCoordRelative(args[3], px)
	y2, err5 := parseCoordRelative(args[4], py)
	z2, err6 := parseCoordRelative(args[5], pz)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
		c.sendSystemMsg(player, "Invalid coordinates", "red")
		return
	}

	blockName := strings.ToLower(args[6])
	stateID := blockStateFromName(blockName)
	if stateID < 0 {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown block: %s", blockName), "red")
		return
	}

	ix1, iy1, iz1 := int(math.Floor(x1)), int(math.Floor(y1)), int(math.Floor(z1))
	ix2, iy2, iz2 := int(math.Floor(x2)), int(math.Floor(y2)), int(math.Floor(z2))

	// Normalize
	if ix1 > ix2 { ix1, ix2 = ix2, ix1 }
	if iy1 > iy2 { iy1, iy2 = iy2, iy1 }
	if iz1 > iz2 { iz1, iz2 = iz2, iz1 }

	total := (ix2-ix1+1) * (iy2-iy1+1) * (iz2-iz1+1)
	if total > 32768 {
		c.sendSystemMsg(player, fmt.Sprintf("Too many blocks: %d (max 32768)", total), "red")
		return
	}

	mode := "replace"
	if len(args) >= 8 {
		mode = strings.ToLower(args[7])
	}

	count := 0
	for bx := ix1; bx <= ix2; bx++ {
		for by := iy1; by <= iy2; by++ {
			for bz := iz1; bz <= iz2; bz++ {
				place := true
				switch mode {
				case "hollow":
					isEdge := bx == ix1 || bx == ix2 || by == iy1 || by == iy2 || bz == iz1 || bz == iz2
					if !isEdge {
						// Fill interior with air
						c.World.SetBlock(bx, by, bz, 0)
						broadcastBlockUpdateDirect(c.Manager, bx, by, bz, 0)
						count++
						place = false
					}
				case "outline":
					isEdge := bx == ix1 || bx == ix2 || by == iy1 || by == iy2 || bz == iz1 || bz == iz2
					if !isEdge {
						place = false
					}
				}
				if place {
					c.World.SetBlock(bx, by, bz, level.BlocksState(stateID))
					broadcastBlockUpdateDirect(c.Manager, bx, by, bz, int32(stateID))
					count++
				}
			}
		}
	}

	c.sendSystemMsg(player, fmt.Sprintf("Filled %d blocks with %s", count, blockName), "green")
	c.Logger.Printf("%s filled %d blocks with %s", player.Name, count, blockName)
}

func (c *CommandExecutor) cmdEffect(player *game.Player, args []string) {
	if len(args) < 2 {
		c.sendSystemMsg(player, "Usage: /effect <give|clear> <player> [effect] [duration] [amplifier]", "red")
		return
	}

	if c.EffectMgr == nil {
		c.sendSystemMsg(player, "Effect system not available", "red")
		return
	}

	subCmd := strings.ToLower(args[0])
	targetName := args[1]

	var target *game.Player
	if targetName == "@s" {
		target = player
	} else {
		target = c.Manager.GetByName(targetName)
	}
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", targetName), "red")
		return
	}

	switch subCmd {
	case "give":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /effect give <player> <effect> [duration] [amplifier]", "red")
			return
		}
		effectName := strings.ToLower(args[2])
		effectID := EffectIDByName(effectName)
		if effectID < 0 {
			c.sendSystemMsg(player, fmt.Sprintf("Unknown effect: %s", effectName), "red")
			return
		}

		duration := int32(600) // default 30s = 600 ticks
		if len(args) >= 4 {
			d, err := strconv.Atoi(args[3])
			if err != nil {
				c.sendSystemMsg(player, "Invalid duration", "red")
				return
			}
			duration = int32(d) * 20 // seconds to ticks
		}

		amplifier := int32(0)
		if len(args) >= 5 {
			a, err := strconv.Atoi(args[4])
			if err != nil {
				c.sendSystemMsg(player, "Invalid amplifier", "red")
				return
			}
			amplifier = int32(a)
		}

		c.EffectMgr.ApplyEffect(target, effectID, amplifier, duration, false)
		c.sendSystemMsg(player, fmt.Sprintf("Applied %s %d to %s for %ds", effectName, amplifier+1, target.Name, duration/20), "green")

	case "clear":
		if len(args) >= 3 {
			effectName := strings.ToLower(args[2])
			effectID := EffectIDByName(effectName)
			if effectID < 0 {
				c.sendSystemMsg(player, fmt.Sprintf("Unknown effect: %s", effectName), "red")
				return
			}
			c.EffectMgr.RemoveEffect(target, effectID)
			c.sendSystemMsg(player, fmt.Sprintf("Removed %s from %s", effectName, target.Name), "green")
		} else {
			c.EffectMgr.ClearAllEffects(target)
			c.sendSystemMsg(player, fmt.Sprintf("Cleared all effects from %s", target.Name), "green")
		}

	default:
		c.sendSystemMsg(player, "Usage: /effect <give|clear> <player> ...", "red")
	}
}
