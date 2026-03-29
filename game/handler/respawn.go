package handler

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// RespawnHandler handles the death screen respawn button.
type RespawnHandler struct {
	Manager      *game.PlayerManager
	World        game.World
	SpawnY       float64
	MinY         int
	Logger       *log.Logger
	DimensionMgr *DimensionManager // optional; when set, handles cross-dimension respawn
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
	player.AirTicks = 300
	if player.SessionEvents != nil {
		player.SessionEvents.OnRespawn()
	}

	// Determine if dimension switch is needed (nether → overworld on death)
	needsDimSwitch := player.Dimension == "minecraft:the_nether"

	// Always respawn in overworld
	dimTypeID := int32(0) // overworld
	dimName := "minecraft:overworld"
	isFlat := false
	if h.DimensionMgr != nil {
		isFlat = h.DimensionMgr.IsFlat
	}

	// Send ClientboundRespawn with overworld dimension
	player.WritePacket(pk.Marshal(
		packetid.ClientboundRespawn,
		pk.VarInt(dimTypeID),            // dimension type index
		pk.Identifier(dimName),          // dimension name
		pk.Long(0),                      // hashed seed
		pk.UnsignedByte(0),              // gamemode: survival
		pk.Byte(-1),                     // previous gamemode: none
		pk.Boolean(false),               // is debug
		pk.Boolean(isFlat),              // is flat
		pk.Boolean(false),               // has death location
		pk.VarInt(0),                    // portal cooldown
		pk.VarInt(63),                   // sea level
		pk.Byte(0),                      // dataKept: nothing kept
	))

	// Switch player to overworld if they were in the nether
	if needsDimSwitch {
		player.Dimension = "minecraft:overworld"
		player.PortalCooldown = 0
		player.PortalTicks = 0
	}

	// Determine spawn location (bed or world spawn)
	var spawnX, spawnYVal, spawnZ float64
	if player.HasSpawnPoint {
		spawnX, spawnYVal, spawnZ = player.SpawnX, player.SpawnY, player.SpawnZ
	} else {
		spawnX, spawnZ = 0.5, 0.5
		spawnYVal = h.SpawnY
	}

	player.SetPosition(spawnX, spawnYVal, spawnZ)
	player.TeleportPending = true

	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(2),             // teleport ID
		pk.Double(spawnX),
		pk.Double(spawnYVal),
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
		pk.Position{X: int(spawnX), Y: int(spawnYVal), Z: int(spawnZ)},
		pk.Float(0), pk.Float(0),
	))

	player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), pk.Float(0),
	))

	// Re-send chunks from the overworld
	var cs *ChunkSender
	if h.DimensionMgr != nil {
		cs = h.DimensionMgr.OverEncoder
	} else {
		cs = &ChunkSender{World: h.World, MinY: h.MinY}
	}
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
