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

func (c *CommandExecutor) cmdTitle(player *game.Player, args []string) {
	if len(args) < 2 {
		c.sendSystemMsg(player, "Usage: /title <targets> <title|subtitle|actionbar|times|clear|reset> ...", "red")
		return
	}

	targets := resolveTargets(player, args[0], c.Manager)
	if len(targets) == 0 {
		c.sendSystemMsg(player, fmt.Sprintf("No targets found for: %s", args[0]), "red")
		return
	}

	subCmd := strings.ToLower(args[1])
	switch subCmd {
	case "title":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /title <targets> title <text>", "red")
			return
		}
		text := strings.Join(args[2:], " ")
		pkt := pk.Marshal(packetid.ClientboundSetTitleText, chat.Text(text))
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Showed title '%s' to %d player(s)", text, len(targets)), "green")

	case "subtitle":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /title <targets> subtitle <text>", "red")
			return
		}
		text := strings.Join(args[2:], " ")
		pkt := pk.Marshal(packetid.ClientboundSetSubtitleText, chat.Text(text))
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set subtitle '%s' for %d player(s)", text, len(targets)), "green")

	case "actionbar":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /title <targets> actionbar <text>", "red")
			return
		}
		text := strings.Join(args[2:], " ")
		pkt := pk.Marshal(packetid.ClientboundSetActionBarText, chat.Text(text))
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Showed action bar '%s' to %d player(s)", text, len(targets)), "green")

	case "times":
		if len(args) < 5 {
			c.sendSystemMsg(player, "Usage: /title <targets> times <fadeIn> <stay> <fadeOut> (in ticks)", "red")
			return
		}
		fadeIn, err1 := strconv.Atoi(args[2])
		stay, err2 := strconv.Atoi(args[3])
		fadeOut, err3 := strconv.Atoi(args[4])
		if err1 != nil || err2 != nil || err3 != nil {
			c.sendSystemMsg(player, "Times must be integers (in ticks)", "red")
			return
		}
		pkt := pk.Marshal(packetid.ClientboundSetTitlesAnimation,
			pk.Int(int32(fadeIn)),
			pk.Int(int32(stay)),
			pk.Int(int32(fadeOut)),
		)
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Set title times to %d/%d/%d for %d player(s)", fadeIn, stay, fadeOut, len(targets)), "green")

	case "clear":
		pkt := pk.Marshal(packetid.ClientboundClearTitles, pk.Boolean(false))
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Cleared titles for %d player(s)", len(targets)), "green")

	case "reset":
		pkt := pk.Marshal(packetid.ClientboundClearTitles, pk.Boolean(true))
		for _, t := range targets {
			t.WritePacket(pkt)
		}
		c.sendSystemMsg(player, fmt.Sprintf("Reset title times for %d player(s)", len(targets)), "green")

	default:
		c.sendSystemMsg(player, "Usage: /title <targets> <title|subtitle|actionbar|times|clear|reset> ...", "red")
	}
}
