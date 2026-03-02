package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/server/command"
)

// CommandExecutor dispatches and executes player commands.
type CommandExecutor struct {
	Manager         *game.PlayerManager
	Logger          *log.Logger
	SurvivalHandler *SurvivalHandler
	TimeMgr         *TimeManager
	WeatherMgr      *WeatherManager
	KeepInventory   *bool
	PermMgr         *PermissionManager
}

// Execute parses and dispatches a command line (without the leading /).
func (c *CommandExecutor) Execute(player *game.Player, cmdLine string) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	// Check permissions for admin commands
	if pm := c.PermMgr; pm != nil {
		switch cmd {
		case "gamemode", "gm", "tp", "teleport", "give", "kill", "time",
			"clear", "difficulty", "weather", "xp", "experience", "enchant", "gamerule":
			if pm.OpLevel(player.UUID) < 2 {
				c.sendSystemMsg(player, "You don't have permission to use this command", "red")
				return
			}
		case "op", "deop", "whitelist":
			if pm.OpLevel(player.UUID) < 3 {
				c.sendSystemMsg(player, "You don't have permission to use this command", "red")
				return
			}
		}
	}

	switch cmd {
	case "gamemode", "gm":
		c.cmdGamemode(player, args)
	case "tp", "teleport":
		c.cmdTeleport(player, args)
	case "kill":
		c.cmdKill(player)
	case "say":
		c.cmdSay(player, args)
	case "time":
		c.cmdTime(player, args)
	case "give":
		c.cmdGive(player, args)
	case "clear":
		c.cmdClear(player, args)
	case "difficulty":
		c.cmdDifficulty(player, args)
	case "weather":
		c.cmdWeather(player, args)
	case "xp", "experience":
		c.cmdXP(player, args)
	case "enchant":
		c.cmdEnchant(player, args)
	case "gamerule":
		c.cmdGamerule(player, args)
	case "help":
		c.cmdHelp(player)
	case "op":
		c.cmdOp(player, args)
	case "deop":
		c.cmdDeop(player, args)
	case "whitelist":
		c.cmdWhitelist(player, args)
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown command: /%s. Type /help for a list of commands.", cmd), "red")
	}
}

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

func (c *CommandExecutor) cmdKill(player *game.Player) {
	c.SurvivalHandler.Kill(c.Manager, player)
	c.Logger.Printf("%s killed themselves", player.Name)
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

func (c *CommandExecutor) cmdDifficulty(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /difficulty <peaceful|easy|normal|hard>", "red")
		return
	}

	var level int32
	var name string
	switch strings.ToLower(args[0]) {
	case "peaceful", "0":
		level, name = 0, "Peaceful"
	case "easy", "1":
		level, name = 1, "Easy"
	case "normal", "2":
		level, name = 2, "Normal"
	case "hard", "3":
		level, name = 3, "Hard"
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown difficulty: %s", args[0]), "red")
		return
	}

	pkt := pk.Marshal(
		packetid.ClientboundChangeDifficulty,
		pk.UnsignedByte(level),
		pk.Boolean(false), // not locked
	)
	c.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	c.sendSystemMsg(player, fmt.Sprintf("Set difficulty to %s", name), "green")
	c.Logger.Printf("%s set difficulty to %s", player.Name, name)
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

func (c *CommandExecutor) cmdEnchant(player *game.Player, args []string) {
	validEnchants := map[string]bool{
		"sharpness": true, "protection": true, "efficiency": true,
		"unbreaking": true, "knockback": true,
	}

	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /enchant <enchantment> [level]", "red")
		c.sendSystemMsg(player, "  Valid: sharpness, protection, efficiency, unbreaking, knockback", "")
		return
	}

	enchName := strings.ToLower(args[0])
	if !validEnchants[enchName] {
		c.sendSystemMsg(player, fmt.Sprintf("Unknown enchantment: %s", args[0]), "red")
		return
	}

	level := int32(1)
	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 1 || n > 5 {
			c.sendSystemMsg(player, "Level must be between 1 and 5", "red")
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

	if invItem.Enchantments == nil {
		invItem.Enchantments = make(map[string]int32)
	}
	invItem.Enchantments[enchName] = level

	c.sendSystemMsg(player, fmt.Sprintf("Applied %s %d to held item", enchName, level), "green")
	c.Logger.Printf("%s enchanted held item with %s %d", player.Name, enchName, level)
}

func (c *CommandExecutor) cmdGamerule(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /gamerule <rule> [value]", "red")
		c.sendSystemMsg(player, "  Available rules: keepInventory", "")
		return
	}

	rule := strings.ToLower(args[0])
	switch rule {
	case "keepinventory":
		if len(args) < 2 {
			val := "false"
			if c.KeepInventory != nil && *c.KeepInventory {
				val = "true"
			}
			c.sendSystemMsg(player, fmt.Sprintf("keepInventory = %s", val), "green")
			return
		}
		switch strings.ToLower(args[1]) {
		case "true":
			if c.KeepInventory != nil {
				*c.KeepInventory = true
			}
			c.sendSystemMsg(player, "Set keepInventory to true", "green")
		case "false":
			if c.KeepInventory != nil {
				*c.KeepInventory = false
			}
			c.sendSystemMsg(player, "Set keepInventory to false", "green")
		default:
			c.sendSystemMsg(player, "Value must be true or false", "red")
		}
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown gamerule: %s", rule), "red")
	}
}

func (c *CommandExecutor) cmdHelp(player *game.Player) {
	lines := []string{
		"Available commands:",
		"  /gamemode <mode> — Change game mode (survival/creative/adventure/spectator)",
		"  /tp <x> <y> <z> — Teleport to coordinates",
		"  /tp <player> — Teleport to a player",
		"  /kill — Kill yourself",
		"  /say <message> — Broadcast a server announcement",
		"  /time set <value> — Set world time (day/noon/night/midnight/<ticks>)",
		"  /give <player> <item> [count] — Give items to a player",
		"  /clear [player] — Clear inventory",
		"  /difficulty <level> — Set difficulty (peaceful/easy/normal/hard)",
		"  /weather <clear|rain|thunder> [duration_ticks] — Set weather",
		"  /xp <add|set> <player> <amount> [levels|points] — Manage XP",
		"  /enchant <enchantment> [level] — Enchant held item",
		"  /gamerule <rule> [value] — View/set game rules",
		"  /op <player> — Make a player a server operator",
		"  /deop <player> — Remove a player's operator status",
		"  /whitelist <add|remove|on|off|list> [player] — Manage whitelist",
		"  /help — Show this help message",
	}
	for _, line := range lines {
		c.sendSystemMsg(player, line, "")
	}
}

func (c *CommandExecutor) cmdOp(player *game.Player, args []string) {
	if c.PermMgr == nil {
		c.sendSystemMsg(player, "Permissions system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /op <player>", "red")
		return
	}
	target := c.Manager.GetByName(args[0])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
		return
	}
	c.PermMgr.SetOp(target.UUID, target.Name, 4)
	c.sendSystemMsg(player, fmt.Sprintf("Made %s a server operator", target.Name), "green")
	c.sendSystemMsg(target, "You are now a server operator", "green")
	c.Logger.Printf("%s opped %s", player.Name, target.Name)
}

func (c *CommandExecutor) cmdDeop(player *game.Player, args []string) {
	if c.PermMgr == nil {
		c.sendSystemMsg(player, "Permissions system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /deop <player>", "red")
		return
	}
	target := c.Manager.GetByName(args[0])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
		return
	}
	c.PermMgr.SetOp(target.UUID, target.Name, 0)
	c.sendSystemMsg(player, fmt.Sprintf("Removed %s as server operator", target.Name), "green")
	c.sendSystemMsg(target, "You are no longer a server operator", "yellow")
	c.Logger.Printf("%s de-opped %s", player.Name, target.Name)
}

func (c *CommandExecutor) cmdWhitelist(player *game.Player, args []string) {
	if c.PermMgr == nil {
		c.sendSystemMsg(player, "Permissions system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /whitelist <add|remove|on|off|list> [player]", "red")
		return
	}
	switch strings.ToLower(args[0]) {
	case "on":
		c.PermMgr.SetWhitelistEnabled(true)
		c.sendSystemMsg(player, "Whitelist enabled", "green")
	case "off":
		c.PermMgr.SetWhitelistEnabled(false)
		c.sendSystemMsg(player, "Whitelist disabled", "green")
	case "add":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /whitelist add <player>", "red")
			return
		}
		target := c.Manager.GetByName(args[1])
		if target == nil {
			c.sendSystemMsg(player, fmt.Sprintf("Player not found (must be online): %s", args[1]), "red")
			return
		}
		c.PermMgr.AddWhitelist(target.UUID, target.Name)
		c.sendSystemMsg(player, fmt.Sprintf("Added %s to whitelist", target.Name), "green")
	case "remove":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /whitelist remove <player>", "red")
			return
		}
		target := c.Manager.GetByName(args[1])
		if target == nil {
			c.sendSystemMsg(player, fmt.Sprintf("Player not found (must be online): %s", args[1]), "red")
			return
		}
		c.PermMgr.RemoveWhitelist(target.UUID)
		c.sendSystemMsg(player, fmt.Sprintf("Removed %s from whitelist", target.Name), "green")
	case "list":
		names := c.PermMgr.WhitelistNames()
		if len(names) == 0 {
			c.sendSystemMsg(player, "Whitelist is empty", "")
		} else {
			c.sendSystemMsg(player, fmt.Sprintf("Whitelisted players: %s", strings.Join(names, ", ")), "")
		}
	default:
		c.sendSystemMsg(player, "Usage: /whitelist <add|remove|on|off|list> [player]", "red")
	}
}

// sendSystemMsg sends a system chat message to a player.
// If color is empty, default (white) is used.
func (c *CommandExecutor) sendSystemMsg(player *game.Player, text, color string) {
	msg := chat.Message{Text: text}
	if color != "" {
		msg.Color = color
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false), // not action bar
	))
}

// BuildCommandGraph creates the command tree for tab-completion hints.
func BuildCommandGraph() *command.Graph {
	noop := func(_ context.Context, _ []command.ParsedData) error { return nil }

	g := command.NewGraph()

	// /help
	g.AppendLiteral(g.Literal("help").HandleFunc(noop))

	// /kill
	g.AppendLiteral(g.Literal("kill").HandleFunc(noop))

	// /gamemode <mode>
	g.AppendLiteral(
		g.Literal("gamemode").
			AppendArgument(g.Argument("mode", command.GamemodeParser{}).HandleFunc(noop)).
			Unhandle(),
	)

	// /gm <mode> (alias)
	g.AppendLiteral(
		g.Literal("gm").
			AppendArgument(g.Argument("mode", command.GamemodeParser{}).HandleFunc(noop)).
			Unhandle(),
	)

	// /tp <location> — vec3 is greedy so it handles "x y z"
	// Also handles /tp <player> as a single-word entity selector
	g.AppendLiteral(
		g.Literal("tp").
			AppendArgument(g.Argument("destination", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
			Unhandle(),
	)

	// /teleport (alias)
	g.AppendLiteral(
		g.Literal("teleport").
			AppendArgument(g.Argument("destination", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
			Unhandle(),
	)

	// /say <message>
	g.AppendLiteral(
		g.Literal("say").
			AppendArgument(g.Argument("message", command.MessageParser{}).HandleFunc(noop)).
			Unhandle(),
	)

	// /time set <value>
	g.AppendLiteral(
		g.Literal("time").
			AppendLiteral(
				g.Literal("set").
					AppendArgument(g.Argument("value", command.StringParser(0)).HandleFunc(noop)).
					Unhandle(),
			).
			Unhandle(),
	)

	// /give <player> <item> [count]
	g.AppendLiteral(
		g.Literal("give").
			AppendArgument(
				g.Argument("player", command.EntityParser{Flags: 0x01}).
					AppendArgument(
						g.Argument("item", command.StringParser(0)).
							AppendArgument(g.Argument("count", command.StringParser(0)).HandleFunc(noop)).
							HandleFunc(noop),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	// /clear [player]
	g.AppendLiteral(
		g.Literal("clear").
			AppendArgument(g.Argument("player", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
			HandleFunc(noop),
	)

	// /difficulty <level>
	g.AppendLiteral(
		g.Literal("difficulty").
			AppendArgument(g.Argument("difficulty", command.StringParser(0)).HandleFunc(noop)).
			Unhandle(),
	)

	// /weather <type>
	g.AppendLiteral(
		g.Literal("weather").
			AppendArgument(g.Argument("type", command.StringParser(0)).HandleFunc(noop)).
			Unhandle(),
	)

	// /xp <add|set> <player> <amount> [levels|points]
	g.AppendLiteral(
		g.Literal("xp").
			AppendLiteral(
				g.Literal("add").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(g.Argument("amount", command.StringParser(0)).HandleFunc(noop)).
							Unhandle(),
					).
					Unhandle(),
			).
			AppendLiteral(
				g.Literal("set").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(g.Argument("amount", command.StringParser(0)).HandleFunc(noop)).
							Unhandle(),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	// /enchant <enchantment> [level]
	g.AppendLiteral(
		g.Literal("enchant").
			AppendArgument(
				g.Argument("enchantment", command.StringParser(0)).
					AppendArgument(g.Argument("level", command.StringParser(0)).HandleFunc(noop)).
					HandleFunc(noop),
			).
			Unhandle(),
	)

	// /gamerule <rule> [value]
	g.AppendLiteral(
		g.Literal("gamerule").
			AppendArgument(
				g.Argument("rule", command.StringParser(0)).
					AppendArgument(g.Argument("value", command.StringParser(0)).HandleFunc(noop)).
					HandleFunc(noop),
			).
			Unhandle(),
	)

	// /experience (alias for /xp)
	g.AppendLiteral(
		g.Literal("experience").
			AppendLiteral(
				g.Literal("add").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(g.Argument("amount", command.StringParser(0)).HandleFunc(noop)).
							Unhandle(),
					).
					Unhandle(),
			).
			AppendLiteral(
				g.Literal("set").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(g.Argument("amount", command.StringParser(0)).HandleFunc(noop)).
							Unhandle(),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	// /op <player>
	g.AppendLiteral(
		g.Literal("op").
			AppendArgument(g.Argument("player", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
			Unhandle(),
	)

	// /deop <player>
	g.AppendLiteral(
		g.Literal("deop").
			AppendArgument(g.Argument("player", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
			Unhandle(),
	)

	// /whitelist <add|remove|on|off|list> [player]
	g.AppendLiteral(
		g.Literal("whitelist").
			AppendLiteral(g.Literal("on").HandleFunc(noop)).
			AppendLiteral(g.Literal("off").HandleFunc(noop)).
			AppendLiteral(g.Literal("list").HandleFunc(noop)).
			AppendLiteral(
				g.Literal("add").
					AppendArgument(g.Argument("player", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
					Unhandle(),
			).
			AppendLiteral(
				g.Literal("remove").
					AppendArgument(g.Argument("player", command.EntityParser{Flags: 0x01}).HandleFunc(noop)).
					Unhandle(),
			).
			Unhandle(),
	)

	return g
}
