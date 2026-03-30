package handler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Entity metadata serializer IDs (26.1-snapshot-2).
// See https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata
const (
	metaSerializerByte    = 0
	metaSerializerInt     = 1
	metaSerializerFloat   = 3
	metaSerializerOptChat = 6
	metaSerializerBoolean     = 8
	metaSerializerPose        = 20
	metaSerializerVillagerData = 22
)

// MetadataWriter builds entity metadata entries.
type MetadataWriter struct {
	buf bytes.Buffer
}

func (w *MetadataWriter) writeIndex(index uint8, serializerID int32) {
	w.buf.WriteByte(index)
	writeVarIntBuf(&w.buf, serializerID)
}

// WriteByte writes a BYTE metadata entry.
func (w *MetadataWriter) WriteByte(index uint8, value int8) {
	w.writeIndex(index, metaSerializerByte)
	w.buf.WriteByte(byte(value))
}

// WriteVarInt writes a VARINT metadata entry.
func (w *MetadataWriter) WriteVarInt(index uint8, value int32) {
	w.writeIndex(index, metaSerializerInt)
	writeVarIntBuf(&w.buf, value)
}

// WriteFloat writes a FLOAT metadata entry.
func (w *MetadataWriter) WriteFloat(index uint8, value float32) {
	w.writeIndex(index, metaSerializerFloat)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], math.Float32bits(value))
	w.buf.Write(b[:])
}

// WriteBoolean writes a BOOLEAN metadata entry.
func (w *MetadataWriter) WriteBoolean(index uint8, value bool) {
	w.writeIndex(index, metaSerializerBoolean)
	if value {
		w.buf.WriteByte(1)
	} else {
		w.buf.WriteByte(0)
	}
}

// WriteOptChat writes an Optional Chat metadata entry.
// If msg is non-empty, writes Boolean(true) + chat JSON as NBT.
func (w *MetadataWriter) WriteOptChat(index uint8, msg chat.Message) {
	w.writeIndex(index, metaSerializerOptChat)
	if msg.Text == "" {
		w.buf.WriteByte(0) // not present
		return
	}
	w.buf.WriteByte(1) // present
	// Write chat as NBT compound (simplified: just string tag)
	msg.WriteTo(&w.buf)
}

// WritePose writes a POSE metadata entry (VarInt enum).
func (w *MetadataWriter) WritePose(index uint8, pose int32) {
	w.writeIndex(index, metaSerializerPose)
	writeVarIntBuf(&w.buf, pose)
}

// WriteVillagerData writes a VILLAGER_DATA metadata entry (type, profession, level as VarInts).
func (w *MetadataWriter) WriteVillagerData(index uint8, villagerType, profession, level int32) {
	w.writeIndex(index, metaSerializerVillagerData)
	writeVarIntBuf(&w.buf, villagerType)
	writeVarIntBuf(&w.buf, profession)
	writeVarIntBuf(&w.buf, level)
}

// Bytes returns the metadata bytes with the 0xFF terminator appended.
func (w *MetadataWriter) Bytes() []byte {
	data := make([]byte, w.buf.Len()+1)
	copy(data, w.buf.Bytes())
	data[len(data)-1] = 0xFF
	return data
}

// writeVarIntBuf writes a VarInt to a bytes.Buffer.
func writeVarIntBuf(buf *bytes.Buffer, v int32) {
	val := uint32(v)
	for val >= 0x80 {
		buf.WriteByte(byte(val&0x7F) | 0x80)
		val >>= 7
	}
	buf.WriteByte(byte(val))
}

// EntityFlags builds the entity flags byte from player state.
func EntityFlags(p *game.Player) int8 {
	var flags int8
	if p.Sneaking {
		flags |= 0x02
	}
	if p.Sprinting {
		flags |= 0x08
	}
	return flags
}

// SendEntityMetadata sends ClientboundSetEntityData to target.
func SendEntityMetadata(target *game.Player, entityID int32, data []byte) {
	target.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(entityID),
		pk.PluginMessageData(data),
	))
}

// SendFullPlayerMetadata sends initial metadata for a player to target.
// Includes: entity flags (index 0), custom name (index 2), custom name visible (index 3),
// pose (index 6), and skin parts (index 16).
func SendFullPlayerMetadata(target, about *game.Player) {
	var w MetadataWriter
	w.WriteByte(0, EntityFlags(about))          // entity flags
	w.WriteVarInt(1, about.AirTicks)            // air supply
	w.WriteOptChat(2, healthDisplayText(about)) // custom name
	w.WriteBoolean(3, true)                     // custom name visible
	w.WritePose(6, playerPose(about))           // pose
	w.WriteByte(16, int8(about.SkinParts))      // displayed skin parts
	SendEntityMetadata(target, about.EID, w.Bytes())
}

// healthDisplayText returns a chat message showing the player's health.
func healthDisplayText(p *game.Player) chat.Message {
	hearts := int(p.Health + 0.5)
	if hearts < 0 {
		hearts = 0
	}
	return chat.Message{
		Text:  fmt.Sprintf("%s  %d", p.Name, hearts),
		Color: "red",
	}
}

// BroadcastHealthTag sends updated health display to nearby players.
func BroadcastHealthTag(manager *game.PlayerManager, player *game.Player) {
	var w MetadataWriter
	w.WriteOptChat(2, healthDisplayText(player))
	w.WriteBoolean(3, true)
	data := w.Bytes()

	px, _, pz := player.Position()
	manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			SendEntityMetadata(p, player.EID, data)
		}
	})
}

// BroadcastAirSupply sends updated air supply metadata to the player and nearby players.
// This makes the client render the air bubble bar when underwater.
func BroadcastAirSupply(manager *game.PlayerManager, player *game.Player) {
	var w MetadataWriter
	w.WriteVarInt(1, player.AirTicks)
	data := w.Bytes()

	// Send to the player themselves (for their own bubble bar)
	SendEntityMetadata(player, player.EID, data)

	// Send to nearby players
	px, _, pz := player.Position()
	manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			SendEntityMetadata(p, player.EID, data)
		}
	})
}

// BroadcastEntityFlags broadcasts updated entity flags + pose to nearby players.
func BroadcastEntityFlags(manager *game.PlayerManager, player *game.Player) {
	var w MetadataWriter
	w.WriteByte(0, EntityFlags(player))
	w.WritePose(6, playerPose(player))
	data := w.Bytes()

	px, _, pz := player.Position()
	manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			SendEntityMetadata(p, player.EID, data)
		}
	})
}

// playerPose returns the pose enum value for a player.
func playerPose(p *game.Player) int32 {
	if p.Sleeping {
		return 2 // Sleeping
	}
	if p.Sneaking {
		return 5 // Sneaking
	}
	return 0 // Standing
}
