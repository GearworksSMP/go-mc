package handler

import (
	"bytes"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
)

// SendPlayerInfo sends ClientboundPlayerInfoUpdate (add_player + initialize_chat + gamemode + listed)
// for 'about' to 'target'.
func SendPlayerInfo(target, about *game.Player) {
	// Actions bitset: bit 0 = add_player, bit 1 = init_chat, bit 2 = gamemode, bit 3 = listed
	actions := pk.NewFixedBitSet(6)
	actions.Set(0, true) // add player
	actions.Set(1, true) // initialize chat
	actions.Set(2, true) // update gamemode
	actions.Set(3, true) // update listed

	props := make([]user.Property, len(about.Properties))
	copy(props, about.Properties)

	// Build packet manually since initialize_chat has conditional fields
	var buf bytes.Buffer
	actions.WriteTo(&buf)
	pk.VarInt(1).WriteTo(&buf)          // count = 1
	pk.UUID(about.UUID).WriteTo(&buf)   // player UUID

	// Action 0: add_player
	pk.String(about.Name).WriteTo(&buf) // player name
	pk.Array(props).WriteTo(&buf)       // properties (skin textures)

	// Action 1: initialize_chat
	if about.ProfileKey != nil {
		pk.Boolean(true).WriteTo(&buf)                // has session
		pk.UUID(about.ChatSessionID).WriteTo(&buf)    // session UUID
		about.ProfileKey.WriteTo(&buf)                // public key (expiry + key + sig)
	} else {
		pk.Boolean(false).WriteTo(&buf) // no session (offline mode)
	}

	// Action 2: update gamemode
	pk.VarInt(about.GameMode).WriteTo(&buf)

	// Action 3: update listed
	pk.Boolean(true).WriteTo(&buf)

	target.WritePacket(pk.Packet{
		ID:   int32(packetid.ClientboundPlayerInfoUpdate),
		Data: buf.Bytes(),
	})
}

// SendSpawnPlayer sends ClientboundAddEntity (type=124, Player) for 'about' to 'target'.
// 26.1 format: eid, uuid, type, x, y, z, LpVec3(movement), pitch, yaw, headYaw, data.
// Velocity is encoded as LpVec3: a single 0x00 byte means zero velocity.
func SendSpawnPlayer(target, about *game.Player) {
	x, y, z := about.Position()
	yaw, pitch := about.Rotation()

	target.WritePacket(pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(about.EID),        // entity ID
		pk.UUID(about.UUID),         // entity UUID
		pk.VarInt(155),              // entity type = Player (26.1-snapshot-2)
		pk.Double(x),                // x
		pk.Double(y),                // y
		pk.Double(z),                // z
		pk.UnsignedByte(0),          // LpVec3 zero velocity (single 0x00 byte)
		pk.Angle(degToAngle(pitch)), // xRot (pitch)
		pk.Angle(degToAngle(yaw)),   // yRot (yaw)
		pk.Angle(degToAngle(yaw)),   // yHeadRot (head yaw)
		pk.VarInt(0),                // data
	))
}

// PlayerTrackingRange is the distance (in blocks) within which players can see each other.
const PlayerTrackingRange = 64.0

// BroadcastPlayerJoin sends PlayerInfoUpdate + AddEntity + metadata for 'joined' to all OTHER players
// within tracking range, and broadcasts a join message to all.
func BroadcastPlayerJoin(manager *game.PlayerManager, joined *game.Player) {
	joinMsg := chat.Message{Text: joined.Name + " joined the game", Color: "yellow"}
	joinPkt := pk.Marshal(
		packetid.ClientboundSystemChat,
		joinMsg,
		pk.Boolean(false),
	)
	jx, _, jz := joined.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange
	manager.ForEach(func(p *game.Player) {
		if p.UUID == joined.UUID {
			return
		}
		// Always send join message and tab list info
		SendPlayerInfo(p, joined)
		p.WritePacket(joinPkt)

		// Only spawn entity if within tracking range
		px, _, pz := p.Position()
		dx := px - jx
		dz := pz - jz
		if dx*dx+dz*dz <= r2 {
			SendSpawnPlayer(p, joined)
			SendFullPlayerMetadata(p, joined)
			SendEquipment(p, joined)
			p.VisiblePlayers[joined.UUID] = true
		}
	})
}

// SendExistingPlayers sends PlayerInfoUpdate + AddEntity + metadata for all existing players to 'newPlayer'.
// Only spawns entities for players within tracking range.
func SendExistingPlayers(manager *game.PlayerManager, newPlayer *game.Player) {
	nx, _, nz := newPlayer.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange
	manager.ForEach(func(p *game.Player) {
		if p.UUID == newPlayer.UUID {
			return
		}
		// Always send tab list info
		SendPlayerInfo(newPlayer, p)

		// Only spawn entity if within tracking range
		px, _, pz := p.Position()
		dx := px - nx
		dz := pz - nz
		if dx*dx+dz*dz <= r2 {
			SendSpawnPlayer(newPlayer, p)
			SendFullPlayerMetadata(newPlayer, p)
			SendEquipment(newPlayer, p)
			newPlayer.VisiblePlayers[p.UUID] = true
		}
	})
}

// UpdatePlayerVisibility checks all other players and spawns/despawns entities
// as they enter/exit tracking range. Call this when a player crosses a chunk boundary.
func UpdatePlayerVisibility(manager *game.PlayerManager, player *game.Player) {
	px, _, pz := player.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange
	manager.ForEach(func(other *game.Player) {
		if other.UUID == player.UUID {
			return
		}
		ox, _, oz := other.Position()
		dx := px - ox
		dz := pz - oz
		inRange := dx*dx+dz*dz <= r2

		wasVisible := player.VisiblePlayers[other.UUID]
		if inRange && !wasVisible {
			// Other player entered our range — spawn them for us
			SendSpawnPlayer(player, other)
			SendFullPlayerMetadata(player, other)
			SendEquipment(player, other)
			player.VisiblePlayers[other.UUID] = true

			// Also spawn us for the other player
			otherSeesUs := other.VisiblePlayers[player.UUID]
			if !otherSeesUs {
				SendSpawnPlayer(other, player)
				SendFullPlayerMetadata(other, player)
				SendEquipment(other, player)
				other.VisiblePlayers[player.UUID] = true
			}
		} else if !inRange && wasVisible {
			// Other player left our range — despawn them for us
			player.WritePacket(pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(other.EID),
			))
			delete(player.VisiblePlayers, other.UUID)

			// Also despawn us for the other player
			if other.VisiblePlayers[player.UUID] {
				other.WritePacket(pk.Marshal(
					packetid.ClientboundRemoveEntities,
					pk.VarInt(1),
					pk.VarInt(player.EID),
				))
				delete(other.VisiblePlayers, player.UUID)
			}
		}
	})
}

// BroadcastPlayerLeave sends RemoveEntities + PlayerInfoRemove for 'left' to all remaining players,
// and broadcasts a leave message.
func BroadcastPlayerLeave(manager *game.PlayerManager, left *game.Player) {
	leaveMsg := chat.Message{Text: left.Name + " left the game", Color: "yellow"}
	leavePkt := pk.Marshal(
		packetid.ClientboundSystemChat,
		leaveMsg,
		pk.Boolean(false),
	)
	manager.ForEach(func(p *game.Player) {
		if p.UUID == left.UUID {
			return
		}
		// Clean up visibility tracking
		delete(p.VisiblePlayers, left.UUID)

		// RemoveEntities
		p.WritePacket(pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),            // count
			pk.VarInt(left.EID),     // entity ID
		))
		// PlayerInfoRemove
		p.WritePacket(pk.Marshal(
			packetid.ClientboundPlayerInfoRemove,
			pk.VarInt(1),            // count
			pk.UUID(left.UUID),      // UUID
		))
		p.WritePacket(leavePkt)
	})
}

// SendEquipment sends ClientboundSetEquipment for 'about' to 'target'.
// Equipment slots: 0=mainhand, 1=offhand, 2=boots, 3=leggings, 4=chestplate, 5=helmet.
func SendEquipment(target, about *game.Player) {
	type equipEntry struct {
		wireSlot byte
		invSlot  int
	}
	entries := []equipEntry{
		{0, int(about.HeldSlot) + 36}, // mainhand
		{1, 45},                        // offhand
		{2, 8},                         // boots
		{3, 7},                         // leggings
		{4, 6},                         // chestplate
		{5, 5},                         // helmet
	}

	var buf []byte
	for i, e := range entries {
		slotByte := e.wireSlot
		if i < len(entries)-1 {
			slotByte |= 0x80 // set MSB for all except last
		}
		buf = append(buf, slotByte)
		s := about.Inventory[e.invSlot].ToSlot()
		slotBytes := encodeSlot261(s)
		buf = append(buf, slotBytes...)
	}

	target.WritePacket(pk.Marshal(
		packetid.ClientboundSetEquipment,
		pk.VarInt(about.EID),
		pk.PluginMessageData(buf),
	))
}

// encodeSlot261 encodes a Slot261 to bytes.
func encodeSlot261(s game.Slot261) []byte {
	var buf bytes.Buffer
	s.WriteTo(&buf)
	return buf.Bytes()
}

// BroadcastEquipment sends equipment of 'player' to all other players within tracking range.
func BroadcastEquipment(manager *game.PlayerManager, player *game.Player) {
	px, _, pz := player.Position()
	manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			SendEquipment(p, player)
		}
	})
}

// degToAngle converts degrees to a protocol Angle (1/256 of a full turn).
func degToAngle(deg float32) int8 {
	return int8(int32(deg*256.0/360.0) & 0xFF)
}
