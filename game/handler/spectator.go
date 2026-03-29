package handler

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// SpectatorManager handles spectator mode behaviors: camera control,
// visibility toggling, and gamemode transition effects.
type SpectatorManager struct {
	Manager *game.PlayerManager
	Logger  *log.Logger
}

// IsSpectator returns true if the player is in spectator mode (gamemode 3).
func IsSpectator(p *game.Player) bool {
	return p.GameMode == 3
}

// SpectateEntity sends ClientboundSetCamera to make the player view from
// another entity's perspective.
func (s *SpectatorManager) SpectateEntity(p *game.Player, entityID int32) {
	if !IsSpectator(p) {
		return
	}
	p.WritePacket(pk.Marshal(
		packetid.ClientboundSetCamera,
		pk.VarInt(entityID),
	))
}

// StopSpectating resets the camera back to the player's own entity.
func (s *SpectatorManager) StopSpectating(p *game.Player) {
	p.WritePacket(pk.Marshal(
		packetid.ClientboundSetCamera,
		pk.VarInt(p.EID),
	))
}

// HandleSpectatorTeleport teleports a spectator to the target player.
func (s *SpectatorManager) HandleSpectatorTeleport(p *game.Player, targetUUID uuid.UUID) {
	if !IsSpectator(p) {
		return
	}
	target := s.Manager.Get(targetUUID)
	if target == nil {
		return
	}
	tx, ty, tz := target.Position()
	p.SetPosition(tx, ty, tz)
	p.TeleportPending = true
	p.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(101),    // teleport ID
		pk.Double(tx),
		pk.Double(ty),
		pk.Double(tz),
		pk.Double(0),      // vel_x
		pk.Double(0),      // vel_y
		pk.Double(0),      // vel_z
		pk.Float(0),       // yaw (relative)
		pk.Float(0),       // pitch (relative)
		pk.Int(0x08|0x10), // flags: yaw+pitch relative
	))
}

// OnGamemodeChange handles visibility updates when a player's gamemode changes.
// Entering spectator hides the player from non-spectators; leaving spectator
// re-spawns the player entity for nearby players.
func (s *SpectatorManager) OnGamemodeChange(p *game.Player, oldMode, newMode int32) {
	if oldMode == newMode {
		return
	}

	if newMode == 3 {
		s.hideFromOthers(p)
		s.StopSpectating(p)
	} else if oldMode == 3 {
		s.showToOthers(p)
	}
}

// hideFromOthers sends RemoveEntities for p to all non-spectator players
// that currently have p visible.
func (s *SpectatorManager) hideFromOthers(p *game.Player) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(p.EID),
	)
	s.Manager.ForEach(func(other *game.Player) {
		if other.UUID == p.UUID || IsSpectator(other) {
			return
		}
		if other.VisiblePlayers[p.UUID] {
			other.WritePacket(removePkt)
			delete(other.VisiblePlayers, p.UUID)
		}
	})
}

// showToOthers re-spawns the player entity for nearby non-spectator players.
func (s *SpectatorManager) showToOthers(p *game.Player) {
	px, _, pz := p.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange
	s.Manager.ForEach(func(other *game.Player) {
		if other.UUID == p.UUID {
			return
		}
		ox, _, oz := other.Position()
		dx := px - ox
		dz := pz - oz
		if dx*dx+dz*dz <= r2 {
			SendSpawnPlayer(other, p)
			SendFullPlayerMetadata(other, p)
			SendEquipment(other, p)
			other.VisiblePlayers[p.UUID] = true
		}
	})
}

// SendSpectatorMenu sends a chat-based list of online non-spectator players.
func (s *SpectatorManager) SendSpectatorMenu(p *game.Player) {
	if !IsSpectator(p) {
		return
	}
	s.Manager.ForEach(func(other *game.Player) {
		if other.UUID == p.UUID || IsSpectator(other) {
			return
		}
		p.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			chat.Message{Text: other.Name, Color: "aqua"},
			pk.Boolean(false),
		))
	})
}

// ShouldHideFromPlayer reports whether subject should be hidden from viewer.
// Spectators are invisible to non-spectators.
func ShouldHideFromPlayer(viewer, subject *game.Player) bool {
	return IsSpectator(subject) && !IsSpectator(viewer)
}
