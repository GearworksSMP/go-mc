package handler

import (
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// KeepaliveHandler sends keepalive packets and handles responses.
type KeepaliveHandler struct{}

// Tick sends a keepalive packet every 300 ticks (15 seconds).
func (k *KeepaliveHandler) Tick(tick int64, players *game.PlayerManager) {
	if tick%300 != 0 {
		return
	}
	keepAliveID := rand.Int63()
	players.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundKeepAlive,
			pk.Long(keepAliveID),
		))
	})
}

// HandlePacket processes keepalive response packets.
// Returns true if the packet was handled.
func (k *KeepaliveHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) == packetid.ServerboundKeepAlive {
		// Keepalive response — acknowledged, no action needed
		return true
	}
	return false
}
