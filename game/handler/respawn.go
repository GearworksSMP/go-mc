package handler

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// RespawnHandler handles the death screen respawn button.
type RespawnHandler struct {
	Manager *game.PlayerManager
	World   game.World
	SpawnY  float64
	MinY    int
	Logger  *log.Logger
}

// HandlePacket processes ServerboundClientCommand (respawn request).
// Returns true if the packet was handled.
func (h *RespawnHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundClientCommand {
		return false
	}

	var action pk.VarInt
	if err := p.Scan(&action); err != nil {
		return true
	}

	if action == 0 { // perform_respawn
		h.handleRespawn(player)
	}
	return true
}

func (h *RespawnHandler) handleRespawn(player *game.Player) {
	if !player.Dead {
		return
	}

	// Reset survival state
	player.Health = 20
	player.Food = 20
	player.Saturation = 5
	player.Exhaustion = 0
	player.Dead = false
	player.FallStartY = -999

	// Send ClientboundRespawn with same dimension info
	player.WritePacket(pk.Marshal(
		packetid.ClientboundRespawn,
		pk.VarInt(0),                             // dimension type index
		pk.Identifier("minecraft:overworld"),      // dimension name
		pk.Long(0),                                // hashed seed
		pk.UnsignedByte(0),                        // gamemode: survival
		pk.Byte(-1),                               // previous gamemode: none
		pk.Boolean(false),                         // is debug
		pk.Boolean(true),                          // is flat
		pk.Boolean(false),                         // has death location
		pk.VarInt(0),                              // portal cooldown
		pk.VarInt(63),                             // sea level
		pk.Byte(0),                                // dataKept: nothing kept
	))

	// Teleport to spawn
	spawnX, spawnZ := 0.5, 0.5
	player.SetPosition(spawnX, h.SpawnY, spawnZ)

	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(2),          // teleport ID
		pk.Double(spawnX),
		pk.Double(h.SpawnY),
		pk.Double(spawnZ),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(0), pk.Float(0), // yaw, pitch
		pk.Int(0),   // flags: all absolute
	))

	// Clear loaded chunks so they get re-sent
	player.LoadedChunks = make(map[game.ChunkPos]bool)

	// Send spawn sequence
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDefaultSpawnPosition,
		pk.Identifier("minecraft:overworld"),
		pk.Position{X: 0, Y: int(h.SpawnY), Z: 0},
		pk.Float(0), pk.Float(0),
	))

	player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), pk.Float(0),
	))

	// Re-send chunks
	cs := &ChunkSender{World: h.World, MinY: h.MinY}
	cs.SendInitialChunks(player)

	// Send health update
	SendSetHealth(player)

	// Broadcast metadata reset to other players
	BroadcastEntityFlags(h.Manager, player)

	h.logf("Player %s respawned", player.Name)
}

func (h *RespawnHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
