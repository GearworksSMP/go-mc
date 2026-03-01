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
}

// Execute parses and dispatches a command line (without the leading /).
func (c *CommandExecutor) Execute(player *game.Player, cmdLine string) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "gamemode", "gm":
		c.cmdGamemode(player, args)
	case "tp", "teleport":
		c.cmdTeleport(player, args)
	case "kill":
		c.cmdKill(player)
	case "say":
		c.cmdSay(player, args)
	case "help":
		c.cmdHelp(player)
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

func (c *CommandExecutor) cmdHelp(player *game.Player) {
	lines := []string{
		"Available commands:",
		"  /gamemode <mode> — Change game mode (survival/creative/adventure/spectator)",
		"  /tp <x> <y> <z> — Teleport to coordinates",
		"  /tp <player> — Teleport to a player",
		"  /kill — Kill yourself",
		"  /say <message> — Broadcast a server announcement",
		"  /help — Show this help message",
	}
	for _, line := range lines {
		c.sendSystemMsg(player, line, "")
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

	return g
}
