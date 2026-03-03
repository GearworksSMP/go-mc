package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

const scoreboardObjective = "health"

// SendScoreboard sends the scoreboard objective and all current player scores to a newly joined player.
func SendScoreboard(manager *game.PlayerManager, player *game.Player) {
	// Create objective: action=0, name="health", displayName="♥", renderType=1 (hearts)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetObjective,
		pk.String(scoreboardObjective),
		pk.Byte(0), // action: create
		chat.Text("♥"),
		pk.VarInt(1),     // render type: hearts
		pk.Boolean(false), // no number format
	))

	// Display objective: slot=2 (below_name)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDisplayObjective,
		pk.VarInt(2), // slot: below_name
		pk.String(scoreboardObjective),
	))

	// Send all online players' health scores
	manager.ForEach(func(p *game.Player) {
		sendScorePacket(player, p.Name, int32(p.Health))
	})
}

// UpdateHealthScore broadcasts an updated health score for a player to all online players.
func UpdateHealthScore(manager *game.PlayerManager, player *game.Player) {
	manager.ForEach(func(p *game.Player) {
		sendScorePacket(p, player.Name, int32(player.Health))
	})
}

// RemovePlayerScore broadcasts a score removal for a leaving player to all online players.
func RemovePlayerScore(manager *game.PlayerManager, playerName string) {
	pkt := pk.Marshal(
		packetid.ClientboundResetScore,
		pk.String(playerName),
		pk.Boolean(true), // has objective
		pk.String(scoreboardObjective),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// sendScorePacket sends a SetScore packet to a single recipient.
func sendScorePacket(recipient *game.Player, entityName string, value int32) {
	recipient.WritePacket(pk.Marshal(
		packetid.ClientboundSetScore,
		pk.String(entityName),
		pk.String(scoreboardObjective),
		pk.VarInt(value),
		pk.Boolean(false), // no display name
		pk.Boolean(false), // no number format
	))
}
