package game

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level"
	pk "github.com/Tnze/go-mc/net/packet"
)

// WriteChunkPacket261 encodes a level.Chunk into a 26.1 LevelChunkWithLight packet.
// The level.Chunk.WriteTo now produces 26.1 format natively (no VarInt data prefix,
// map codec heightmaps, no Trust Edges in light data).
func WriteChunkPacket261(pos ChunkPos, chunk *level.Chunk, minY int) (pk.Packet, error) {
	return pk.Marshal(
		packetid.ClientboundLevelChunkWithLight,
		pk.Int(int32(pos.X)), pk.Int(int32(pos.Z)),
		chunk,
	), nil
}
