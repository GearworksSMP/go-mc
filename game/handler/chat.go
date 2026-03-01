package handler

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/chat/sign"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// ChatHandler processes chat messages and broadcasts them to all players.
type ChatHandler struct {
	Manager  *game.PlayerManager
	Logger   *log.Logger
	Commands *CommandExecutor
}

// HandlePacket processes chat packets.
// Returns true if the packet was handled.
func (c *ChatHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundChat:
		c.handleChat(player, p)
		return true
	case packetid.ServerboundChatCommand:
		c.handleChatCommand(player, p)
		return true
	case packetid.ServerboundChatAck:
		return true
	case packetid.ServerboundChatSessionUpdate:
		return true
	}
	return false
}

func (c *ChatHandler) handleChat(player *game.Player, p pk.Packet) {
	var (
		message      pk.String
		timestamp    pk.Long
		salt         pk.Long
		hasSignature pk.Boolean
	)
	historyUpdate := sign.HistoryUpdate{
		Acknowledged: pk.NewFixedBitSet(20),
	}

	if err := p.Scan(&message, &timestamp, &salt, &hasSignature, &historyUpdate); err != nil {
		c.Logger.Printf("Failed to parse chat from %s: %v", player.Name, err)
		return
	}

	msg := string(message)
	if len(msg) == 0 || len(msg) > 256 {
		return
	}

	c.Logger.Printf("<%s> %s", player.Name, msg)
	c.broadcastChat(player, msg)
}

func (c *ChatHandler) handleChatCommand(player *game.Player, p pk.Packet) {
	var command pk.String
	if err := p.Scan(&command); err != nil {
		c.Logger.Printf("Failed to parse chat command from %s: %v", player.Name, err)
		return
	}
	c.Logger.Printf("%s issued command: /%s", player.Name, string(command))
	if c.Commands != nil {
		c.Commands.Execute(player, string(command))
	}
}

func (c *ChatHandler) broadcastChat(player *game.Player, message string) {
	pkt := pk.Marshal(
		packetid.ClientboundDisguisedChat,
		chat.Text(message),
		&chat.Type{ID: 0, SenderName: chat.Text(player.Name)},
	)
	c.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
