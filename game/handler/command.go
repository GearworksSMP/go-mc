package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"math"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
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
	Rules           *GameRules
	PermMgr         *PermissionManager
	World           game.World
	MobMgr          *MobManager
	EffectMgr       *EffectManager
	BanMgr          *BanManager
	ScoreboardMgr   *ScoreboardManager
	SpectatorMgr    *SpectatorManager
}

// Execute parses and dispatches a command line (without the leading /).
func (c *CommandExecutor) Execute(player *game.Player, cmdLine string) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	if player.SessionEvents != nil {
		player.SessionEvents.OnCommand(cmdLine)
	}

	// Check permissions for admin commands
	if pm := c.PermMgr; pm != nil {
		switch cmd {
		case "gamemode", "gm", "tp", "teleport", "give", "kill", "time",
			"clear", "difficulty", "weather", "xp", "experience", "enchant", "gamerule",
			"summon", "setblock", "fill", "effect", "title", "scoreboard":
			if pm.OpLevel(player.UUID) < 2 {
				c.sendSystemMsg(player, "You don't have permission to use this command", "red")
				return
			}
		case "op", "deop", "whitelist", "kick", "ban", "pardon", "banlist":
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
	case "summon":
		c.cmdSummon(player, args)
	case "setblock":
		c.cmdSetblock(player, args)
	case "fill":
		c.cmdFill(player, args)
	case "effect":
		c.cmdEffect(player, args)
	case "kick":
		c.cmdKick(player, args)
	case "ban":
		c.cmdBan(player, args)
	case "pardon":
		c.cmdPardon(player, args)
	case "banlist":
		c.cmdBanList(player)
	case "title":
		c.cmdTitle(player, args)
	case "msg", "tell", "w":
		c.cmdMsg(player, args)
	case "scoreboard":
		c.cmdScoreboard(player, args)
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

	oldMode := player.GameMode
	player.GameMode = mode

	if c.SpectatorMgr != nil {
		c.SpectatorMgr.OnGamemodeChange(player, oldMode, mode)
	}

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
	if c.Rules != nil {
		c.Rules.SetDifficulty(level)
	}
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

func (c *CommandExecutor) cmdGamerule(player *game.Player, args []string) {
	if c.Rules == nil {
		c.sendSystemMsg(player, "Game rules not available", "red")
		return
	}

	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /gamerule <rule> [true|false]", "red")
		c.sendSystemMsg(player, "Rules: "+strings.Join(RuleNames, ", "), "")
		return
	}

	ruleName := strings.ToLower(args[0])

	// Query mode
	if len(args) < 2 {
		val, ok := c.Rules.GetRule(ruleName)
		if !ok {
			c.sendSystemMsg(player, fmt.Sprintf("Unknown gamerule: %s", args[0]), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("%s = %v", args[0], val), "green")
		return
	}

	// Set mode
	switch strings.ToLower(args[1]) {
	case "true":
		if !c.Rules.SetRule(ruleName, true) {
			c.sendSystemMsg(player, fmt.Sprintf("Unknown gamerule: %s", args[0]), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set %s to true", args[0]), "green")
	case "false":
		if !c.Rules.SetRule(ruleName, false) {
			c.sendSystemMsg(player, fmt.Sprintf("Unknown gamerule: %s", args[0]), "red")
			return
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set %s to false", args[0]), "green")
	default:
		c.sendSystemMsg(player, "Value must be true or false", "red")
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
		"  /summon <entity> [x y z] — Summon an entity",
		"  /setblock <x> <y> <z> <block> — Set a block",
		"  /fill <x1 y1 z1> <x2 y2 z2> <block> [mode] — Fill region with blocks",
		"  /effect <give|clear> <player> [effect] [duration] [amplifier] — Manage effects",
		"  /kick <player> [reason] — Kick a player",
		"  /ban <player> [reason] — Ban a player",
		"  /pardon <player> — Unban a player",
		"  /banlist — List all banned players",
		"  /title <targets> <title|subtitle|actionbar|times|clear|reset> — Manage titles",
		"  /msg <player> <message> — Send a private message (/tell, /w)",
		"  /scoreboard objectives <add|remove|setdisplay> — Manage objectives",
		"  /scoreboard players <set|add|remove|reset> — Manage scores",
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

// blockStateFromName returns the default block state ID for a block name (without minecraft: prefix).
// Returns -1 if the block is unknown.
func blockStateFromName(name string) int {
	b, ok := block.FromID["minecraft:"+name]
	if !ok {
		return -1
	}
	sid, ok := block.ToStateID[b]
	if !ok {
		return -1
	}
	return int(sid)
}

// parseCoordRelative parses a coordinate that may be relative (~offset from player pos).
func parseCoordRelative(s string, playerPos float64) (float64, error) {
	if strings.HasPrefix(s, "~") {
		if s == "~" {
			return playerPos, nil
		}
		offset, err := strconv.ParseFloat(s[1:], 64)
		if err != nil {
			return 0, err
		}
		return playerPos + offset, nil
	}
	return strconv.ParseFloat(s, 64)
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

func (c *CommandExecutor) cmdKick(player *game.Player, args []string) {
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /kick <player> [reason]", "red")
		return
	}

	target := c.Manager.GetByName(args[0])
	if target == nil {
		c.sendSystemMsg(player, fmt.Sprintf("Player not found: %s", args[0]), "red")
		return
	}

	reason := "Kicked by operator"
	if len(args) >= 2 {
		reason = strings.Join(args[1:], " ")
	}

	// Send disconnect packet
	kickMsg := chat.Message{Text: reason}
	target.WritePacket(pk.Marshal(
		packetid.ClientboundDisconnect,
		kickMsg,
	))
	target.Conn.Close()

	c.sendSystemMsg(player, fmt.Sprintf("Kicked %s: %s", target.Name, reason), "green")
	c.Logger.Printf("%s kicked %s: %s", player.Name, target.Name, reason)
}

func (c *CommandExecutor) cmdBan(player *game.Player, args []string) {
	if c.BanMgr == nil {
		c.sendSystemMsg(player, "Ban system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /ban <player> [reason]", "red")
		return
	}

	target := c.Manager.GetByName(args[0])
	targetName := args[0]
	targetUUID := ""
	reason := "Banned by operator"
	if len(args) >= 2 {
		reason = strings.Join(args[1:], " ")
	}

	if target != nil {
		targetName = target.Name
		targetUUID = target.UUID.String()
		// Kick the banned player
		target.WritePacket(pk.Marshal(
			packetid.ClientboundDisconnect,
			chat.Message{Text: "You have been banned: " + reason},
		))
		target.Conn.Close()
	}

	c.BanMgr.Ban(targetName, targetUUID, reason, player.Name)
	c.sendSystemMsg(player, fmt.Sprintf("Banned %s: %s", targetName, reason), "green")
	c.Logger.Printf("%s banned %s: %s", player.Name, targetName, reason)
}

func (c *CommandExecutor) cmdPardon(player *game.Player, args []string) {
	if c.BanMgr == nil {
		c.sendSystemMsg(player, "Ban system not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /pardon <player>", "red")
		return
	}

	banned, _ := c.BanMgr.IsBanned(args[0])
	if !banned {
		c.sendSystemMsg(player, fmt.Sprintf("%s is not banned", args[0]), "red")
		return
	}

	c.BanMgr.Pardon(args[0])
	c.sendSystemMsg(player, fmt.Sprintf("Pardoned %s", args[0]), "green")
	c.Logger.Printf("%s pardoned %s", player.Name, args[0])
}

func (c *CommandExecutor) cmdBanList(player *game.Player) {
	if c.BanMgr == nil {
		c.sendSystemMsg(player, "Ban system not available", "red")
		return
	}

	bans := c.BanMgr.ListBans()
	if len(bans) == 0 {
		c.sendSystemMsg(player, "No players are banned", "")
		return
	}

	c.sendSystemMsg(player, fmt.Sprintf("Banned players (%d):", len(bans)), "")
	for _, b := range bans {
		line := fmt.Sprintf("  %s — %s (by %s, %s)", b.PlayerName, b.Reason, b.BannedBy, b.BannedAt)
		c.sendSystemMsg(player, line, "")
	}
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

	// /gamerule <rule> [true|false]
	gameruleNode := g.Literal("gamerule")
	for _, ruleName := range RuleNames {
		gameruleNode.AppendLiteral(
			g.Literal(ruleName).
				AppendLiteral(g.Literal("true").HandleFunc(noop)).
				AppendLiteral(g.Literal("false").HandleFunc(noop)).
				HandleFunc(noop),
		)
	}
	g.AppendLiteral(gameruleNode.Unhandle())

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

	// /summon <entity_type> [x y z]
	g.AppendLiteral(
		g.Literal("summon").
			AppendArgument(
				g.Argument("entity", command.StringParser(0)).
					AppendArgument(
						g.Argument("pos", command.StringParser(2)).HandleFunc(noop),
					).
					HandleFunc(noop),
			).
			Unhandle(),
	)

	// /setblock <x> <y> <z> <block>
	g.AppendLiteral(
		g.Literal("setblock").
			AppendArgument(
				g.Argument("pos", command.StringParser(2)).
					AppendArgument(g.Argument("block", command.StringParser(0)).HandleFunc(noop)).
					Unhandle(),
			).
			Unhandle(),
	)

	// /fill <x1> <y1> <z1> <x2> <y2> <z2> <block> [mode]
	g.AppendLiteral(
		g.Literal("fill").
			AppendArgument(
				g.Argument("from", command.StringParser(2)).
					AppendArgument(
						g.Argument("to_and_block", command.StringParser(2)).HandleFunc(noop),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	// /effect <give|clear> <player> [effect] [duration] [amplifier]
	g.AppendLiteral(
		g.Literal("effect").
			AppendLiteral(
				g.Literal("give").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(
								g.Argument("effect", command.StringParser(0)).
									AppendArgument(
										g.Argument("duration", command.StringParser(0)).
											AppendArgument(g.Argument("amplifier", command.StringParser(0)).HandleFunc(noop)).
											HandleFunc(noop),
									).
									HandleFunc(noop),
							).
							Unhandle(),
					).
					Unhandle(),
			).
			AppendLiteral(
				g.Literal("clear").
					AppendArgument(
						g.Argument("player", command.EntityParser{Flags: 0x01}).
							AppendArgument(g.Argument("effect", command.StringParser(0)).HandleFunc(noop)).
							HandleFunc(noop),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	// /kick <player> [reason]
	g.AppendLiteral(
		g.Literal("kick").
			AppendArgument(
				g.Argument("player", command.EntityParser{Flags: 0x01}).
					AppendArgument(g.Argument("reason", command.MessageParser{}).HandleFunc(noop)).
					HandleFunc(noop),
			).
			Unhandle(),
	)

	// /ban <player> [reason]
	g.AppendLiteral(
		g.Literal("ban").
			AppendArgument(
				g.Argument("player", command.EntityParser{Flags: 0x01}).
					AppendArgument(g.Argument("reason", command.MessageParser{}).HandleFunc(noop)).
					HandleFunc(noop),
			).
			Unhandle(),
	)

	// /pardon <player>
	g.AppendLiteral(
		g.Literal("pardon").
			AppendArgument(g.Argument("player", command.StringParser(0)).HandleFunc(noop)).
			Unhandle(),
	)

	// /banlist
	g.AppendLiteral(g.Literal("banlist").HandleFunc(noop))

	// /title <targets> <action> ...
	g.AppendLiteral(
		g.Literal("title").
			AppendArgument(
				g.Argument("targets", command.EntityParser{Flags: 0x01}).
					AppendLiteral(
						g.Literal("title").
							AppendArgument(g.Argument("text", command.MessageParser{}).HandleFunc(noop)).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("subtitle").
							AppendArgument(g.Argument("text", command.MessageParser{}).HandleFunc(noop)).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("actionbar").
							AppendArgument(g.Argument("text", command.MessageParser{}).HandleFunc(noop)).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("times").
							AppendArgument(g.Argument("fadeIn_stay_fadeOut", command.StringParser(2)).HandleFunc(noop)).
							Unhandle(),
					).
					AppendLiteral(g.Literal("clear").HandleFunc(noop)).
					AppendLiteral(g.Literal("reset").HandleFunc(noop)).
					Unhandle(),
			).
			Unhandle(),
	)

	// /msg, /tell, /w — private messaging
	for _, alias := range []string{"msg", "tell", "w"} {
		g.AppendLiteral(
			g.Literal(alias).
				AppendArgument(
					g.Argument("player", command.EntityParser{Flags: 0x01}).
						AppendArgument(g.Argument("message", command.MessageParser{}).HandleFunc(noop)).
						Unhandle(),
				).
				Unhandle(),
		)
	}

	// /scoreboard <objectives|players> ...
	g.AppendLiteral(
		g.Literal("scoreboard").
			AppendLiteral(
				g.Literal("objectives").
					AppendLiteral(
						g.Literal("add").
							AppendArgument(
								g.Argument("name", command.StringParser(0)).
									AppendArgument(
										g.Argument("criteria", command.StringParser(0)).
											AppendArgument(g.Argument("displayName", command.MessageParser{}).HandleFunc(noop)).
											HandleFunc(noop),
									).
									Unhandle(),
							).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("remove").
							AppendArgument(g.Argument("name", command.StringParser(0)).HandleFunc(noop)).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("setdisplay").
							AppendArgument(
								g.Argument("slot", command.StringParser(0)).
									AppendArgument(g.Argument("objective", command.StringParser(0)).HandleFunc(noop)).
									Unhandle(),
							).
							Unhandle(),
					).
					Unhandle(),
			).
			AppendLiteral(
				g.Literal("players").
					AppendLiteral(
						g.Literal("set").
							AppendArgument(
								g.Argument("targets", command.EntityParser{Flags: 0x01}).
									AppendArgument(
										g.Argument("objective", command.StringParser(0)).
											AppendArgument(g.Argument("score", command.StringParser(0)).HandleFunc(noop)).
											Unhandle(),
									).
									Unhandle(),
							).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("add").
							AppendArgument(
								g.Argument("targets", command.EntityParser{Flags: 0x01}).
									AppendArgument(
										g.Argument("objective", command.StringParser(0)).
											AppendArgument(g.Argument("score", command.StringParser(0)).HandleFunc(noop)).
											Unhandle(),
									).
									Unhandle(),
							).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("remove").
							AppendArgument(
								g.Argument("targets", command.EntityParser{Flags: 0x01}).
									AppendArgument(
										g.Argument("objective", command.StringParser(0)).
											AppendArgument(g.Argument("score", command.StringParser(0)).HandleFunc(noop)).
											Unhandle(),
									).
									Unhandle(),
							).
							Unhandle(),
					).
					AppendLiteral(
						g.Literal("reset").
							AppendArgument(
								g.Argument("targets", command.EntityParser{Flags: 0x01}).
									AppendArgument(g.Argument("objective", command.StringParser(0)).HandleFunc(noop)).
									HandleFunc(noop),
							).
							Unhandle(),
					).
					Unhandle(),
			).
			Unhandle(),
	)

	return g
}
