package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (c *CommandExecutor) cmdGamemode(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /gamemode <survival|creative|adventure|spectator>", "red")
		return
	}

	var mode int32
	var modeName string
	switch strings.ToLower(args[0]) {
	case "survival", "s", "0":
		mode, modeName = 0, "Survival"
	case "creative", "c", "1":
		mode, modeName = 1, "Creative"
	case "adventure", "a", "2":
		mode, modeName = 2, "Adventure"
	case "spectator", "sp", "3":
		mode, modeName = 3, "Spectator"
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown game mode: %s", args[0]), "red")
		return
	}

	player.GameMode = mode

	// Send GameEvent (event 3 = change game mode)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(3),
		pk.Float(float32(mode)),
	))

	// Send PlayerAbilities with appropriate flags
	var flags byte
	switch mode {
	case 0, 2: // survival, adventure
		flags = 0x00
	case 1: // creative: invulnerable | allow_flying | creative_mode
		flags = 0x0D
	case 3: // spectator: invulnerable | flying | allow_flying
		flags = 0x07
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerAbilities,
		pk.Byte(flags),
		pk.Float(0.05), // fly speed
		pk.Float(0.1),  // fov modifier
	))

	c.sendSystemMsg(player, fmt.Sprintf("Set own game mode to %s", modeName), "green")
	c.Logger.Printf("%s changed game mode to %s", player.Name, modeName)
}

func (c *CommandExecutor) cmdTeleport(player *game.Player, args []string) {
	switch len(args) {
	case 3: // /tp x y z
		x, err1 := strconv.ParseFloat(args[0], 64)
		y, err2 := strconv.ParseFloat(args[1], 64)
		z, err3 := strconv.ParseFloat(args[2], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			c.sendSystemMsg(player, "Usage: /tp <x> <y> <z> — coordinates must be numbers", "red")
			return
		}
		player.SetPosition(x, y, z)
		player.TeleportPending = true
		player.WritePacket(pk.Marshal(
			packetid.ClientboundPlayerPosition,
			pk.VarInt(99),  // teleport ID
			pk.Double(x),   // x
			pk.Double(y),   // y
			pk.Double(z),   // z
			pk.Double(0),   // vel_x
			pk.Double(0),   // vel_y
			pk.Double(0),   // vel_z
			pk.Float(0),    // yaw
			pk.Float(0),    // pitch
			pk.Int(0),      // flags: all absolute
		))
		c.sendSystemMsg(player, fmt.Sprintf("Teleported to %.1f, %.1f, %.1f", x, y, z), "green")
		c.Logger.Printf("%s teleported to %.1f, %.1f, %.1f", player.Name, x, y, z)

	case 1: // /tp <player>
		target := c.Manager.GetByName(args[0])
		if target == nil {
			c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
			return
		}
		tx, ty, tz := target.Position()
		player.SetPosition(tx, ty, tz)
		player.TeleportPending = true
		player.WritePacket(pk.Marshal(
			packetid.ClientboundPlayerPosition,
			pk.VarInt(99),  // teleport ID
			pk.Double(tx),  // x
			pk.Double(ty),  // y
			pk.Double(tz),  // z
			pk.Double(0),   // vel_x
			pk.Double(0),   // vel_y
			pk.Double(0),   // vel_z
			pk.Float(0),    // yaw
			pk.Float(0),    // pitch
			pk.Int(0),      // flags: all absolute
		))
		c.sendSystemMsg(player, fmt.Sprintf("Teleported to %s", target.Name), "green")
		c.Logger.Printf("%s teleported to %s", player.Name, target.Name)

	default:
		c.sendSystemMsg(player, "Usage: /tp <x> <y> <z> or /tp <player>", "red")
	}
}

func (c *CommandExecutor) cmdSay(player *game.Player, args []string) {
	if len(args) == 0 {
		c.sendSystemMsg(player, "Usage: /say <message>", "red")
		return
	}
	message := strings.Join(args, " ")
	announcement := fmt.Sprintf("[%s] %s", player.Name, message)

	pkt := pk.Marshal(
		packetid.ClientboundSystemChat,
		chat.Text(announcement),
		pk.Boolean(false), // not action bar
	)
	c.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	c.Logger.Printf("[%s] %s", player.Name, message)
}

func (c *CommandExecutor) cmdGive(player *game.Player, args []string) {
	if len(args) < 2 {
		c.sendSystemMsg(player, "Usage: /give <player> <item> [count]", "red")
		return
	}

	target := c.Manager.GetByName(args[0])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
		return
	}

	itemName := strings.TrimPrefix(args[1], "minecraft:")
	itemID := itemIDByName(itemName)
	if itemID <= 0 {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown item: %s", args[1]), "red")
		return
	}

	count := int32(1)
	if len(args) >= 3 {
		n, err := strconv.Atoi(args[2])
		if err != nil || n < 1 || n > 64 {
			c.sendSystemMsg(player, "Count must be between 1 and 64", "red")
			return
		}
		count = int32(n)
	}
	stack := NewItemStack(itemID, count)
	slot := target.Inventory.AddItem(stack.ID, stack.Count)
	if slot >= 0 {
		// Copy durability info if the stack has it
		if stack.MaxDurability > 0 {
			target.Inventory[slot].Durability = stack.Durability
			target.Inventory[slot].MaxDurability = stack.MaxDurability
		}
	}
	SendFullInventory(target)
	c.sendSystemMsg(player, fmt.Sprintf("Gave %d %s to %s", count, itemName, target.Name), "green")
	c.Logger.Printf("%s gave %d %s to %s", player.Name, count, itemName, target.Name)
}

func (c *CommandExecutor) cmdXP(player *game.Player, args []string) {
	if len(args) < 3 {
		c.sendSystemMsg(player, "Usage: /xp <add|set> <player> <amount> [levels|points]", "red")
		return
	}

	action := strings.ToLower(args[0])
	target := c.Manager.GetByName(args[1])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[1]), "red")
		return
	}

	amount, err := strconv.Atoi(args[2])
	if err != nil {
		c.sendSystemMsg(player, "Amount must be a number", "red")
		return
	}

	unit := "points"
	if len(args) >= 4 {
		unit = strings.ToLower(args[3])
	}

	switch action {
	case "add":
		if unit == "levels" {
			target.ExperienceLevel += int32(amount)
			if target.ExperienceLevel < 0 {
				target.ExperienceLevel = 0
			}
			SendExperience(target)
		} else {
			AddExperience(target, int32(amount))
		}
	case "set":
		if unit == "levels" {
			target.ExperienceLevel = int32(amount)
			if target.ExperienceLevel < 0 {
				target.ExperienceLevel = 0
			}
			SendExperience(target)
		} else {
			target.Experience = float32(amount) / float32(xpForNextLevel(target.ExperienceLevel))
			if target.Experience > 1 {
				target.Experience = 1
			}
			if target.Experience < 0 {
				target.Experience = 0
			}
			SendExperience(target)
		}
	default:
		c.sendSystemMsg(player, "Usage: /xp <add|set> <player> <amount> [levels|points]", "red")
		return
	}

	c.sendSystemMsg(player, fmt.Sprintf("Set %s XP for %s: %s %d %s", action, target.Name, action, amount, unit), "green")
	c.Logger.Printf("%s %s %d %s XP for %s", player.Name, action, amount, unit, target.Name)
}

func (c *CommandExecutor) cmdMsg(player *game.Player, args []string) {
	if len(args) < 2 {
		c.sendSystemMsg(player, "Usage: /msg <player> <message>", "red")
		return
	}

	target := c.Manager.GetByName(args[0])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
		return
	}

	message := strings.Join(args[1:], " ")

	// Send to target
	targetMsg := chat.Message{
		Text:   fmt.Sprintf("[%s -> You] %s", player.Name, message),
		Color:  "gray",
		Italic: true,
	}
	target.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		targetMsg,
		pk.Boolean(false),
	))

	// Send confirmation to sender
	senderMsg := chat.Message{
		Text:   fmt.Sprintf("[You -> %s] %s", target.Name, message),
		Color:  "gray",
		Italic: true,
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		senderMsg,
		pk.Boolean(false),
	))
}

func (c *CommandExecutor) cmdScoreboard(player *game.Player, args []string) {
	if c.ScoreboardMgr == nil {
		c.sendSystemMsg(player, "Scoreboard system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /scoreboard <objectives|players> ...", "red")
		return
	}

	switch strings.ToLower(args[0]) {
	case "objectives":
		c.cmdScoreboardObjectives(player, args[1:])
	case "players":
		c.cmdScoreboardPlayers(player, args[1:])
	default:
		c.sendSystemMsg(player, "Usage: /scoreboard <objectives|players> ...", "red")
	}
}

func (c *CommandExecutor) cmdScoreboardObjectives(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /scoreboard objectives <add|remove|setdisplay> ...", "red")
		return
	}

	switch strings.ToLower(args[0]) {
	case "add":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /scoreboard objectives add <name> dummy [displayName]", "red")
			return
		}
		name := args[1]
		displayName := name
		if len(args) >= 4 {
			displayName = strings.Join(args[3:], " ")
		}
		if err := c.ScoreboardMgr.AddObjective(name, displayName); err != nil {
			c.sendSystemMsg(player, err.Error(), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("Added objective '%s'", name), "green")

	case "remove":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /scoreboard objectives remove <name>", "red")
			return
		}
		if err := c.ScoreboardMgr.RemoveObjective(args[1]); err != nil {
			c.sendSystemMsg(player, err.Error(), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("Removed objective '%s'", args[1]), "green")

	case "setdisplay":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /scoreboard objectives setdisplay <slot> <name>", "red")
			return
		}
		slot := args[1]
		name := args[2]
		if err := c.ScoreboardMgr.SetDisplay(slot, name); err != nil {
			c.sendSystemMsg(player, err.Error(), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set display slot '%s' to objective '%s'", slot, name), "green")

	default:
		c.sendSystemMsg(player, "Usage: /scoreboard objectives <add|remove|setdisplay> ...", "red")
	}
}

func (c *CommandExecutor) cmdScoreboardPlayers(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /scoreboard players <set|add|remove|reset> ...", "red")
		return
	}

	switch strings.ToLower(args[0]) {
	case "set":
		if len(args) < 4 {
			c.sendSystemMsg(player, "Usage: /scoreboard players set <targets> <objective> <score>", "red")
			return
		}
		targets := resolveTargetNames(player, args[1], c.Manager)
		score, err := strconv.Atoi(args[3])
		if err != nil {
			c.sendSystemMsg(player, "Score must be a number", "red")
			return
		}
		for _, name := range targets {
			c.ScoreboardMgr.SetScore(name, args[2], int32(score))
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set score of %s for %d player(s) to %d", args[2], len(targets), score), "green")

	case "add":
		if len(args) < 4 {
			c.sendSystemMsg(player, "Usage: /scoreboard players add <targets> <objective> <score>", "red")
			return
		}
		targets := resolveTargetNames(player, args[1], c.Manager)
		score, err := strconv.Atoi(args[3])
		if err != nil {
			c.sendSystemMsg(player, "Score must be a number", "red")
			return
		}
		for _, name := range targets {
			c.ScoreboardMgr.AddScore(name, args[2], int32(score))
		}
		c.sendSystemMsg(player, fmt.Sprintf("Added %d to %s for %d player(s)", score, args[2], len(targets)), "green")

	case "remove":
		if len(args) < 4 {
			c.sendSystemMsg(player, "Usage: /scoreboard players remove <targets> <objective> <score>", "red")
			return
		}
		targets := resolveTargetNames(player, args[1], c.Manager)
		score, err := strconv.Atoi(args[3])
		if err != nil {
			c.sendSystemMsg(player, "Score must be a number", "red")
			return
		}
		for _, name := range targets {
			c.ScoreboardMgr.AddScore(name, args[2], -int32(score))
		}
		c.sendSystemMsg(player, fmt.Sprintf("Removed %d from %s for %d player(s)", score, args[2], len(targets)), "green")

	case "reset":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /scoreboard players reset <targets> [objective]", "red")
			return
		}
		targets := resolveTargetNames(player, args[1], c.Manager)
		objective := ""
		if len(args) >= 3 {
			objective = args[2]
		}
		for _, name := range targets {
			c.ScoreboardMgr.ResetScores(name, objective)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Reset scores for %d player(s)", len(targets)), "green")

	default:
		c.sendSystemMsg(player, "Usage: /scoreboard players <set|add|remove|reset> ...", "red")
	}
}

// resolveTargetNames resolves a selector to player names (for scoreboard, which uses names not player objects).
func resolveTargetNames(executor *game.Player, selector string, manager *game.PlayerManager) []string {
	targets := resolveTargets(executor, selector, manager)
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.Name
	}
	return names
}
