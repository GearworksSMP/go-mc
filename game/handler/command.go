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
	WorldBorderMgr  *WorldBorderManager
	TPSHandler      *TPSManager
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
			"summon", "setblock", "fill", "effect", "title", "scoreboard", "worldborder":
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
	case "worldborder":
		c.cmdWorldBorder(player, args)
	case "tps":
		if c.TPSHandler != nil {
			c.TPSHandler.HandleTPSCommand(player)
		}
	default:
		c.sendSystemMsg(player, fmt.Sprintf("Unknown command: /%s. Type /help for a list of commands.", cmd), "red")
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
		"  /worldborder <center|set|add|get|damage|warning> — Manage world border",
		"  /tps — Show server TPS, MSPT, and memory usage",
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

	// /tps
	g.AppendLiteral(g.Literal("tps").HandleFunc(noop))

	return g
}
