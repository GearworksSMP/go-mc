package main

import (
	"fmt"

	"github.com/Tnze/go-mc/cluster"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
)

// connectBackend dials a backend region server and completes the handshake, login,
// and configuration phases as a Minecraft client. Returns a connection ready for
// play-phase packet forwarding.
func connectBackend(cfg *cluster.ClusterConfig, serverID string, name string, id uuid.UUID, properties []user.Property, protocol int32) (*net.Conn, error) {
	sc, ok := cfg.Servers[serverID]
	if !ok {
		return nil, fmt.Errorf("unknown server: %s", serverID)
	}

	conn, err := net.DialMC(sc.MCAddr)
	if err != nil {
		return nil, fmt.Errorf("dial %s (%s): %w", serverID, sc.MCAddr, err)
	}

	// Send handshake
	if err := sendHandshake(conn, sc.MCAddr, protocol); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake: %w", err)
	}

	// Login phase
	if err := doLogin(conn, name, id); err != nil {
		conn.Close()
		return nil, fmt.Errorf("login: %w", err)
	}

	// Configuration phase — read and discard all config packets from the backend
	if err := skipConfigPhase(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("config: %w", err)
	}

	return conn, nil
}

// sendHandshake sends the handshake packet with login intention (2).
func sendHandshake(conn *net.Conn, addr string, protocol int32) error {
	// Parse host:port from addr. For simplicity, use the full addr as the host
	// since the backend ignores it.
	return conn.WritePacket(pk.Marshal(
		0x00, // Handshake packet ID
		pk.VarInt(server.ProtocolVersion),
		pk.String(addr),
		pk.UnsignedShort(25565),
		pk.VarInt(2), // Login intention
	))
}

// doLogin performs the login phase as a client with the backend (offline mode).
func doLogin(conn *net.Conn, name string, id uuid.UUID) error {
	// Send LoginStart (ServerboundLoginHello)
	if err := conn.WritePacket(pk.Marshal(
		int32(packetid.ServerboundLoginHello),
		pk.String(name),
		pk.UUID(id),
	)); err != nil {
		return fmt.Errorf("send login start: %w", err)
	}

	// Read login response packets
	for {
		var p pk.Packet
		if err := conn.ReadPacket(&p); err != nil {
			return fmt.Errorf("read login packet: %w", err)
		}

		switch packetid.ClientboundPacketID(p.ID) {
		case packetid.ClientboundLoginLoginCompression:
			var threshold pk.VarInt
			if err := p.Scan(&threshold); err != nil {
				return fmt.Errorf("scan compression: %w", err)
			}
			conn.SetThreshold(int(threshold))

		case packetid.ClientboundLoginGameProfile:
			// Login success — send LoginAcknowledged
			if err := conn.WritePacket(pk.Marshal(int32(packetid.ServerboundLoginLoginAcknowledged))); err != nil {
				return fmt.Errorf("send login ack: %w", err)
			}
			return nil

		case packetid.ClientboundLoginLoginDisconnect:
			return fmt.Errorf("backend disconnected during login")

		default:
			// Ignore other login packets (cookie requests, etc.)
		}
	}
}

// skipConfigPhase reads and discards all configuration packets from the backend,
// responding to SelectKnownPacks and FinishConfiguration as needed.
func skipConfigPhase(conn *net.Conn) error {
	for {
		var p pk.Packet
		if err := conn.ReadPacket(&p); err != nil {
			return fmt.Errorf("read config packet: %w", err)
		}

		pid := packetid.ClientboundPacketID(p.ID)

		switch pid {
		case packetid.ClientboundConfigSelectKnownPacks:
			// Respond with empty known packs (we don't cache anything)
			var packs []pk.FieldEncoder
			if err := conn.WritePacket(pk.Marshal(
				int32(packetid.ServerboundConfigSelectKnownPacks),
				pk.VarInt(0), // no known packs
			)); err != nil {
				return fmt.Errorf("send known packs response: %w", err)
			}
			_ = packs

		case packetid.ClientboundConfigFinishConfiguration:
			// Send ack
			if err := conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigFinishConfiguration))); err != nil {
				return fmt.Errorf("send finish config ack: %w", err)
			}
			return nil

		case packetid.ClientboundConfigKeepAlive:
			// Echo keepalive
			if err := conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigKeepAlive), pk.Long(0))); err != nil {
				return err
			}

		default:
			// Discard other config packets (registry data, tags, etc.)
		}
	}
}
