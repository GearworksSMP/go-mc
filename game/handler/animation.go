package handler

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// AnimationHandler handles arm swings, sprint start/stop, and client commands.
type AnimationHandler struct {
	Manager   *game.PlayerManager
	BedMgr    *BedManager
	ElytraMgr *ElytraManager
}

// HandlePacket processes animation and player-command packets.
// Returns true if the packet was handled.
func (h *AnimationHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundSwing:
		h.handleSwing(player, p)
		return true

	case packetid.ServerboundPlayerCommand:
		h.handlePlayerCommand(player, p)
		return true
	}
	return false
}

// handleSwing broadcasts arm swing animation to all other players.
func (h *AnimationHandler) handleSwing(player *game.Player, p pk.Packet) {
	var hand pk.VarInt
	if err := p.Scan(&hand); err != nil {
		return
	}

	// animation: 0 = swing main hand, 3 = swing offhand
	var animation pk.UnsignedByte
	if hand == 1 {
		animation = 3
	}

	pkt := pk.Marshal(
		packetid.ClientboundAnimate,
		pk.VarInt(player.EID),
		animation,
	)
	h.Manager.ForEach(func(p *game.Player) {
		if p.UUID != player.UUID {
			p.WritePacket(pkt)
		}
	})
}

// handlePlayerCommand handles sprint start/stop, leave bed, and other player commands.
func (h *AnimationHandler) handlePlayerCommand(player *game.Player, p pk.Packet) {
	var entityID pk.VarInt
	var action pk.VarInt
	if err := p.Scan(&entityID, &action); err != nil {
		return
	}

	switch action {
	case 1: // start sprint
		player.Sprinting = true
		BroadcastEntityFlags(h.Manager, player)
	case 2: // stop sprint
		player.Sprinting = false
		BroadcastEntityFlags(h.Manager, player)
	case 8: // START_FALL_FLYING (elytra)
		if h.ElytraMgr != nil {
			h.ElytraMgr.handleElytraActivation(player)
		}
	}

	// Wake player from bed on any player command action
	if player.Sleeping && h.BedMgr != nil {
		h.BedMgr.WakePlayer(player)
	}
}
