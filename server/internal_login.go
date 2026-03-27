package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
)

// InternalLoginHandler handles login for connections from a trusted proxy.
// No encryption, no Mojang authentication — the proxy has already authenticated the player.
type InternalLoginHandler struct {
	// Threshold is the compression threshold. Set to -1 to disable.
	Threshold int
}

var _ LoginHandler = (*InternalLoginHandler)(nil)

func (h *InternalLoginHandler) AcceptLogin(conn *net.Conn, protocol int32) (name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, err error) {
	// Read LoginStart — trust the name and UUID from the proxy.
	var p pk.Packet
	err = conn.ReadPacket(&p)
	if err != nil {
		return
	}
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundLoginHello {
		err = wrongPacketErr{expect: int32(packetid.ServerboundLoginHello), get: p.ID}
		return
	}

	err = p.Scan(
		(*pk.String)(&name),
		(*pk.UUID)(&id),
	)
	if err != nil {
		return
	}

	// Set compression
	if h.Threshold >= 0 {
		err = conn.WritePacket(pk.Marshal(
			packetid.ClientboundLoginLoginCompression,
			pk.VarInt(h.Threshold),
		))
		if err != nil {
			return
		}
		conn.SetThreshold(h.Threshold)
	}

	// Send LoginSuccess
	err = conn.WritePacket(pk.Marshal(
		packetid.ClientboundLoginGameProfile,
		pk.UUID(id),
		pk.String(name),
		pk.Array(properties),
	))
	if err != nil {
		return
	}

	// Read LoginAcknowledged
	err = conn.ReadPacket(&p)
	if err == nil && packetid.ServerboundPacketID(p.ID) != packetid.ServerboundLoginLoginAcknowledged {
		err = wrongPacketErr{expect: int32(packetid.ServerboundLoginLoginAcknowledged), get: p.ID}
	}
	return
}
