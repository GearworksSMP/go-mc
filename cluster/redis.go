package cluster

import (
	"fmt"

	"github.com/google/uuid"
)

// Redis key patterns and channel names for the cluster.
const (
	// PlayerKeyPrefix is the Redis key prefix for player routing: "player:{uuid}" → serverID.
	PlayerKeyPrefix = "player:"

	// ChatChannel is the Redis pub/sub channel for cross-server chat.
	ChatChannel = "chat:global"

	// BorderChannelPrefix is the Redis pub/sub channel prefix for border entity sync.
	// Full channel: "border:{serverID}".
	BorderChannelPrefix = "border:"

	// TransferChannelPrefix is the Redis pub/sub channel for transfer signals.
	// Full channel: "transfer:{uuid}".
	TransferChannelPrefix = "transfer:"
)

// PlayerKey returns the Redis key for a player's routing entry.
func PlayerKey(id uuid.UUID) string {
	return PlayerKeyPrefix + id.String()
}

// BorderChannel returns the Redis pub/sub channel for a server's border entities.
func BorderChannel(serverID string) string {
	return BorderChannelPrefix + serverID
}

// TransferChannel returns the Redis pub/sub channel for a player's transfer signal.
func TransferChannel(id uuid.UUID) string {
	return TransferChannelPrefix + id.String()
}

// PlayerTransferState holds the data passed between servers during a player transfer.
type PlayerTransferState struct {
	Name       string    `json:"name"`
	UUID       uuid.UUID `json:"uuid"`
	Dimension  string    `json:"dimension"`
	X, Y, Z    float64   `json:"x,y,z"`
	Yaw, Pitch float32   `json:"yaw,pitch"`
	GameMode   int32     `json:"game_mode"`
	Health     float32   `json:"health"`
	Food       int32     `json:"food"`
	Saturation float32   `json:"saturation"`
	FromServer string    `json:"from_server"`
	ToServer   string    `json:"to_server"`
}

func (s PlayerTransferState) String() string {
	return fmt.Sprintf("Transfer{%s→%s player=%s pos=(%.1f,%.1f,%.1f)}",
		s.FromServer, s.ToServer, s.Name, s.X, s.Y, s.Z)
}
