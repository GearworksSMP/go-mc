package main

import (
	"fmt"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
)

// relayConfigPhase relays configuration packets from the backend to the client,
// and client responses back to the backend. This is used during player transfers
// when the client re-enters the configuration phase.
//
// The flow is:
// 1. Backend sends config packets (registry data, known packs, tags, etc.)
// 2. Client may respond to some (known packs selection, custom payload)
// 3. Backend sends FinishConfiguration
// 4. Client sends FinishConfiguration ack
// 5. Backend enters play phase
func relayConfigPhase(client *net.Conn, backend *net.Conn) error {
	// Relay packets from backend to client until FinishConfiguration.
	for {
		var p pk.Packet
		if err := backend.ReadPacket(&p); err != nil {
			return fmt.Errorf("read backend config packet: %w", err)
		}

		pid := packetid.ClientboundPacketID(p.ID)

		// Forward the packet to the client
		if err := client.WritePacket(p); err != nil {
			return fmt.Errorf("write config packet to client: %w", err)
		}

		// If it's SelectKnownPacks, we need to relay the client's response back
		if pid == packetid.ClientboundConfigSelectKnownPacks {
			if err := relayClientResponse(client, backend); err != nil {
				return fmt.Errorf("relay known packs response: %w", err)
			}
		}

		// FinishConfiguration ends the config phase
		if pid == packetid.ClientboundConfigFinishConfiguration {
			// Read the client's ack and forward to backend
			if err := relayClientResponse(client, backend); err != nil {
				return fmt.Errorf("relay finish config ack: %w", err)
			}
			return nil
		}
	}
}

// relayClientResponse reads one packet from the client and forwards it to the backend.
// It skips CustomPayload packets from the client (brand, NeoForge, etc.) and relays
// the first non-CustomPayload response.
func relayClientResponse(client *net.Conn, backend *net.Conn) error {
	for {
		var p pk.Packet
		if err := client.ReadPacket(&p); err != nil {
			return fmt.Errorf("read client response: %w", err)
		}

		// Forward to backend
		if err := backend.WritePacket(p); err != nil {
			return fmt.Errorf("write client response to backend: %w", err)
		}

		// CustomPayload packets (brand, etc.) may precede the actual response.
		// Keep relaying until we get a non-CustomPayload packet.
		if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundConfigCustomPayload {
			return nil
		}
	}
}
