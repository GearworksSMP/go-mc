package handler

import (
	"bytes"
	"fmt"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// TPSProvider is satisfied by MetricsServer.
type TPSProvider interface {
	GetTPS() float64
}

// TabListManager sends periodic tab list header/footer updates and
// latency pings to all connected players.
type TabListManager struct {
	Players *game.PlayerManager
	TPS     TPSProvider // optional; may be nil
}

// GearworksHeader returns the styled header chat.Message for the tab list.
func GearworksHeader() chat.Message {
	return chat.Message{
		Text:  "Gearworks\n",
		Color: "gold",
		Bold:  true,
		Extra: []chat.Message{
			{Text: "Minecraft 26.1", Color: "gray"},
		},
	}
}

// footer builds the tab list footer with player count and optional TPS.
func (t *TabListManager) footer() chat.Message {
	count := t.Players.Count()
	text := fmt.Sprintf("\n%d player(s) online", count)
	if t.TPS != nil {
		text += fmt.Sprintf(" | TPS: %.1f", t.TPS.GetTPS())
	}
	return chat.Message{Text: text, Color: "gray"}
}

// Tick is called every game tick. Every 60 ticks it refreshes the tab list
// header/footer and broadcasts latency updates.
func (t *TabListManager) Tick(tick int64) {
	if tick%60 != 0 {
		return
	}
	header := GearworksHeader()
	footer := t.footer()

	t.Players.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundTabList,
			header, footer,
		))
	})

	t.broadcastLatency()
}

// broadcastLatency sends a ClientboundPlayerInfoUpdate with action=0x10
// (update_latency) for every online player to every online player.
func (t *TabListManager) broadcastLatency() {
	var uuids []pk.UUID
	t.Players.ForEach(func(p *game.Player) {
		uuids = append(uuids, pk.UUID(p.UUID))
	})
	if len(uuids) == 0 {
		return
	}

	// Build the packet once: actions=0x10 (update_latency only)
	var buf bytes.Buffer
	actions := pk.NewFixedBitSet(6)
	actions.Set(4, true) // bit 4 = update_latency
	actions.WriteTo(&buf)
	pk.VarInt(len(uuids)).WriteTo(&buf)
	for _, id := range uuids {
		id.WriteTo(&buf)
		pk.VarInt(50).WriteTo(&buf) // default 50ms
	}
	data := append([]byte(nil), buf.Bytes()...)

	pkt := pk.Packet{
		ID:   int32(packetid.ClientboundPlayerInfoUpdate),
		Data: data,
	}
	t.Players.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
