package main

import (
	"context"
	"log"
	"time"

	"github.com/Tnze/go-mc/cluster"
	"github.com/Tnze/go-mc/net"
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
}

func (gp *ProxyGamePlay) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	ctx := context.Background()

	// Determine which backend to connect to.
	serverID := gp.Config.DefaultServer
	if stored, err := gp.Redis.Get(ctx, cluster.PlayerKey(id)).Result(); err == nil {
		serverID = stored
	}

	gp.Logger.Printf("Player %s (%s) → backend %s", name, id, serverID)

	backendConn, err := connectBackend(gp.Config, serverID, name, id, properties, protocol)
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

	session.Run()
}
