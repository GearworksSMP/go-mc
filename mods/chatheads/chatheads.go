// Package chatheads provides server-side support for the Chat Heads client mod.
// It sends ClientboundPlayerChat with the sender's UUID so the client mod can
// display player head icons next to chat messages.
package chatheads

import (
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/chat/sign"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// ChatBroadcaster sends ClientboundPlayerChat packets with the sender UUID,
// enabling the Chat Heads client mod to render player head icons.
type ChatBroadcaster struct {
	Manager *game.PlayerManager
}

// BroadcastChat sends a ClientboundPlayerChat with the sender's UUID to all players.
func (b *ChatBroadcaster) BroadcastChat(sender *game.Player, message string) {
	now := time.Now()
	senderName := chat.Text(sender.Name)
	body := &sign.PackedMessageBody{
		PlainMsg:  message,
		Timestamp: now,
		Salt:      0,
		LastSeen:  nil,
	}
	filterMask := &sign.FilterMask{Type: 0}
	chatType := &chat.Type{ID: 0, SenderName: senderName}

	b.Manager.ForEach(func(p *game.Player) {
		pkt := pk.Marshal(
			packetid.ClientboundPlayerChat,
			pk.VarInt(p.NextChatIndex()), // globalIndex (1.21.5+)
			pk.UUID(sender.UUID),         // sender
			pk.VarInt(0),                 // index (no chain tracking)
			pk.Boolean(false),            // no signature
			body,                         // message body
			pk.Boolean(false),            // no unsigned content override
			filterMask,                   // PASS_THROUGH
			chatType,                     // chat type with sender name
		)
		p.WritePacket(pkt)
	})
}
