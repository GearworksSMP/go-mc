package handler

import (
	"fmt"
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

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
