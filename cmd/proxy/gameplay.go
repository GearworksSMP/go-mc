package main

import (
	"context"
	"log"
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

// ProxyGamePlay implements server.GamePlay. For each player it dials a backend
// region server and runs a bidirectional packet-forwarding loop.
type ProxyGamePlay struct {
	Config *cluster.ClusterConfig
	Redis  *redis.Client
	Logger *log.Logger
	Health *HealthChecker

	mu       sync.Mutex
	sessions map[uuid.UUID]*PlayerSession
	wg       sync.WaitGroup
}

func (gp *ProxyGamePlay) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	ctx := context.Background()

	// Determine which backend to connect to.
	serverID := gp.Config.DefaultServer
	if stored, err := gp.Redis.Get(ctx, cluster.PlayerKey(id)).Result(); err == nil {
		serverID = stored
	}

	// If the chosen server is unhealthy, fall back to the default server.
	if gp.Health != nil && !gp.Health.IsHealthy(serverID) {
		gp.Logger.Printf("Backend %s unhealthy for %s, falling back to %s", serverID, name, gp.Config.DefaultServer)
		serverID = gp.Config.DefaultServer
	}

	gp.Logger.Printf("Player %s (%s) → backend %s", name, id, serverID)

	backendConn, err := connectBackendWithRetry(gp.Config, serverID, name, id, properties, protocol, gp.Logger)
	if err != nil {
		gp.Logger.Printf("Failed to connect %s to backend %s: %v", name, serverID, err)
		return
	}
	defer backendConn.Close()

	// Store routing in Redis
	gp.Redis.Set(ctx, cluster.PlayerKey(id), serverID, 24*time.Hour)
	defer gp.Redis.Del(ctx, cluster.PlayerKey(id))

	session := &PlayerSession{
		Name:       name,
		UUID:       id,
		Properties: properties,
		Protocol:   protocol,
		Client:     conn,
		Backend:    backendConn,
		ServerID:   serverID,
		Config:     gp.Config,
		Redis:      gp.Redis,
		Logger:     gp.Logger,
	}

	gp.addSession(id, session)
	defer gp.removeSession(id)

	session.Run()
}

func (gp *ProxyGamePlay) addSession(id uuid.UUID, s *PlayerSession) {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	if gp.sessions == nil {
		gp.sessions = make(map[uuid.UUID]*PlayerSession)
	}
	gp.sessions[id] = s
	gp.wg.Add(1)
}

func (gp *ProxyGamePlay) removeSession(id uuid.UUID) {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	delete(gp.sessions, id)
	gp.wg.Done()
}

// DisconnectAll sends a disconnect packet to every active session and closes connections.
func (gp *ProxyGamePlay) DisconnectAll(reason string) {
	gp.mu.Lock()
	sessions := make([]*PlayerSession, 0, len(gp.sessions))
	for _, s := range gp.sessions {
		sessions = append(sessions, s)
	}
	gp.mu.Unlock()

	msg := chat.Text(reason)
	for _, s := range sessions {
		s.Client.WritePacket(pk.Marshal(packetid.ClientboundDisconnect, msg))
		s.Client.Close()
		s.Backend.Close()
	}
}

// Wait blocks until all active sessions have finished.
func (gp *ProxyGamePlay) Wait() {
	gp.wg.Wait()
}
