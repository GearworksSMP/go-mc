package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/cluster"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// PlayerSession manages one player's proxy session — bidirectional forwarding
// between the client and a backend region server.
type PlayerSession struct {
	Name       string
	UUID       uuid.UUID
	Properties []user.Property
	Protocol   int32
	Client     *net.Conn
	Backend    *net.Conn
	ServerID   string
	Config     *cluster.ClusterConfig
	Redis      *redis.Client
	Logger     *log.Logger

	// Player position tracked by snooping movement packets
	mu   sync.Mutex
	x, z float64
}

// Run starts the bidirectional forwarding loop. Blocks until the session ends.
func (s *PlayerSession) Run() {
	done := make(chan struct{}, 2)

	// client → backend (snoop movement packets)
	go func() {
		defer func() { done <- struct{}{} }()
		s.forwardClientToBackend()
	}()

	// backend → client
	go func() {
		defer func() { done <- struct{}{} }()
		s.forwardBackendToClient()
	}()

	// Wait for either direction to finish
	<-done
}

// forwardClientToBackend reads packets from the client and sends them to the backend.
// It snoops on movement packets to detect region boundary crossings.
func (s *PlayerSession) forwardClientToBackend() {
	for {
		var p pk.Packet
		if err := s.Client.ReadPacket(&p); err != nil {
			return
		}

		// Snoop movement packets
		sid := packetid.ServerboundPacketID(p.ID)
		if sid == packetid.ServerboundMovePlayerPos || sid == packetid.ServerboundMovePlayerPosRot {
			s.handleMovement(p)
		}

		if err := s.Backend.WritePacket(p); err != nil {
			return
		}
	}
}

// forwardBackendToClient reads packets from the backend and sends them to the client.
func (s *PlayerSession) forwardBackendToClient() {
	for {
		var p pk.Packet
		if err := s.Backend.ReadPacket(&p); err != nil {
			return
		}

		if err := s.Client.WritePacket(p); err != nil {
			return
		}
	}
}

// handleMovement extracts position from a movement packet and checks for region crossings.
func (s *PlayerSession) handleMovement(p pk.Packet) {
	var x, y, z pk.Double
	if err := p.Scan(&x, &y, &z); err != nil {
		return
	}

	s.mu.Lock()
	oldX, oldZ := s.x, s.z
	s.x, s.z = float64(x), float64(z)
	s.mu.Unlock()

	oldRegionX := blockToRegion(oldX)
	oldRegionZ := blockToRegion(oldZ)
	newRegionX := blockToRegion(float64(x))
	newRegionZ := blockToRegion(float64(z))

	if oldRegionX != newRegionX || oldRegionZ != newRegionZ {
		newServer := s.Config.ServerForBlock("minecraft:overworld", int(math.Floor(float64(x))), int(math.Floor(float64(z))))
		if newServer != s.ServerID {
			s.Logger.Printf("Player %s crossing to region (%d,%d) → server %s",
				s.Name, newRegionX, newRegionZ, newServer)
			s.sendSystemChat(fmt.Sprintf("[Cluster] Transferring to %s (region %d,%d)...", newServer, newRegionX, newRegionZ), "yellow")
			go s.initiateTransfer(newServer, float64(x), float64(y), float64(z))
		}
	}
}

func blockToRegion(coord float64) int {
	return int(math.Floor(coord / 512.0))
}

// initiateTransfer handles moving the player from the current backend to a new one.
func (s *PlayerSession) initiateTransfer(newServerID string, x, y, z float64) {
	ctx := context.Background()

	state := cluster.PlayerTransferState{
		Name:       s.Name,
		UUID:       s.UUID,
		X:          x,
		Y:          y,
		Z:          z,
		FromServer: s.ServerID,
		ToServer:   newServerID,
	}

	stateJSON, _ := json.Marshal(state)
	s.Logger.Printf("Transfer: %s", stateJSON)

	// 1. Send ClientboundStartConfiguration to the client (re-enter config phase)
	if err := s.Client.WritePacket(pk.Marshal(packetid.ClientboundStartConfiguration)); err != nil {
		s.Logger.Printf("Transfer failed (start config): %v", err)
		return
	}

	// 2. Read ConfigurationAcknowledged from client
	var ack pk.Packet
	if err := s.Client.ReadPacket(&ack); err != nil {
		s.Logger.Printf("Transfer failed (config ack): %v", err)
		return
	}

	// 3. Close old backend
	s.Backend.Close()

	// 4. Connect to new backend
	newBackend, err := connectBackend(s.Config, newServerID, s.Name, s.UUID, s.Properties, s.Protocol)
	if err != nil {
		s.Logger.Printf("Transfer failed (connect %s): %v", newServerID, err)
		return
	}

	// 5. Relay the new backend's config phase to the client
	if err := relayConfigPhase(s.Client, newBackend); err != nil {
		s.Logger.Printf("Transfer failed (relay config): %v", err)
		newBackend.Close()
		return
	}

	// 6. Update session state
	s.mu.Lock()
	s.Backend = newBackend
	s.ServerID = newServerID
	s.x = x
	s.z = z
	s.mu.Unlock()

	// 7. Update Redis routing
	s.Redis.Set(ctx, cluster.PlayerKey(s.UUID), newServerID, 24*time.Hour)

	s.Logger.Printf("Transfer complete: %s now on %s", s.Name, newServerID)
	s.sendSystemChat(fmt.Sprintf("[Cluster] Now on %s", newServerID), "green")
}

// sendSystemChat sends a system chat message to the client for debugging.
func (s *PlayerSession) sendSystemChat(text string, color string) {
	msg := chat.Message{Text: text, Color: color}
	s.Client.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false), // not action bar
	))
}
