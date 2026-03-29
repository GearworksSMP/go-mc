package handler

import (
	"log"
	"math"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// DimensionManager handles cross-dimension teleportation between the Overworld, Nether, and End.
type DimensionManager struct {
	Manager       *game.PlayerManager
	OverWorld     game.World
	NetherWorld   game.World
	EndWorld      game.World
	OverEncoder   *ChunkSender
	NetherEncoder *ChunkSender
	EndEncoder    *ChunkSender
	Logger        *log.Logger
	AdvMgr        *AdvancementManager

	// NetherDimTypeID is the registry index for minecraft:the_nether dimension type.
	// Default: 3 (overworld=0, overworld_caves=1, the_end=2, the_nether=3).
	NetherDimTypeID int32

	// OverworldDimTypeID is the registry index for minecraft:overworld dimension type.
	// Default: 0.
	OverworldDimTypeID int32

	// EndDimTypeID is the registry index for minecraft:the_end dimension type.
	// Default: 2.
	EndDimTypeID int32

	// SpawnY is the overworld spawn Y coordinate.
	OverworldSpawnY float64

	// IsFlat indicates whether the overworld is a flat world (for respawn packet).
	IsFlat bool

	// EndPortalMgr handles end portal checks (optional).
	EndPortalMgr *EndPortalManager

	// DragonMgr manages the ender dragon boss (optional).
	DragonMgr *EnderDragonManager
}

// Tick updates portal cooldowns and checks if players are standing in portals.
// Called every server tick (20 TPS).
func (dm *DimensionManager) Tick(tick int64) {
	dm.Manager.ForEach(func(player *game.Player) {
		// Decrement portal cooldown
		if player.PortalCooldown > 0 {
			player.PortalCooldown--
		}

		// Check if player is standing in a nether portal
		if player.Dead || player.PortalCooldown > 0 {
			player.PortalTicks = 0
			return
		}

		world := dm.worldForPlayer(player)
		if world == nil {
			return
		}

		// Check block at player's feet
		px, py, pz := player.Position()
		footX := int(math.Floor(px))
		footY := int(math.Floor(py))
		footZ := int(math.Floor(pz))

		state, err := world.GetBlock(footX, footY, footZ)
		if err != nil {
			player.PortalTicks = 0
			return
		}

		// Check for end portal (instant teleport)
		if isEndPortalState(int(state)) {
			if dm.EndPortalMgr != nil {
				dm.EndPortalMgr.CheckPlayerInEndPortal(player, world)
			}
			return
		}

		if isPortalState(int(state)) {
			player.PortalTicks++
			if player.PortalTicks >= 80 { // 4 seconds at 20 TPS
				if player.Dimension == "minecraft:the_nether" {
					dm.TeleportToOverworld(player)
				} else if player.Dimension != "minecraft:the_end" {
					dm.TeleportToNether(player)
				}
				player.PortalTicks = 0
			}
		} else {
			player.PortalTicks = 0
		}
	})
}

// TeleportToNether sends a player from the Overworld to the Nether.
func (dm *DimensionManager) TeleportToNether(player *game.Player) {
	px, _, pz := player.Position()

	// Nether coordinates: overworld / 8
	netherX := px / 8
	netherZ := pz / 8

	// Find a safe Y in the nether
	netherY := dm.findSafeY(dm.NetherWorld, int(math.Floor(netherX)), int(math.Floor(netherZ)), 0, 128)

	dm.sendRespawn(player, dm.NetherDimTypeID, "minecraft:the_nether", false)

	player.Dimension = "minecraft:the_nether"
	if player.SessionEvents != nil {
		player.SessionEvents.OnDimensionChange("minecraft:overworld", "minecraft:the_nether")
	}
	if dm.AdvMgr != nil {
		dm.AdvMgr.CheckDimensionChange(player, "minecraft:the_nether")
	}
	player.LoadedChunks = make(map[game.ChunkPos]bool)
	player.SetPosition(netherX, float64(netherY), netherZ)
	player.FallStartY = -999
	player.PortalCooldown = 300 // 15 seconds

	dm.sendPlayerPosition(player, netherX, float64(netherY), netherZ)
	dm.sendSpawnSequenceForDimension(player, dm.NetherEncoder, "minecraft:the_nether")

	dm.logf("Player %s teleported to Nether at (%.1f, %d, %.1f)", player.Name, netherX, netherY, netherZ)
}

// TeleportToOverworld sends a player from the Nether to the Overworld.
func (dm *DimensionManager) TeleportToOverworld(player *game.Player) {
	px, _, pz := player.Position()

	// Overworld coordinates: nether * 8
	owX := px * 8
	owZ := pz * 8

	// Find a safe Y in the overworld
	owY := dm.findSafeY(dm.OverWorld, int(math.Floor(owX)), int(math.Floor(owZ)), -64, 320)

	dm.sendRespawn(player, dm.OverworldDimTypeID, "minecraft:overworld", dm.IsFlat)

	prevDim := player.Dimension
	player.Dimension = "minecraft:overworld"
	if player.SessionEvents != nil {
		player.SessionEvents.OnDimensionChange(prevDim, "minecraft:overworld")
	}
	player.LoadedChunks = make(map[game.ChunkPos]bool)
	player.SetPosition(owX, float64(owY), owZ)
	player.FallStartY = -999
	player.PortalCooldown = 300 // 15 seconds

	dm.sendPlayerPosition(player, owX, float64(owY), owZ)
	dm.sendSpawnSequenceForDimension(player, dm.OverEncoder, "minecraft:overworld")

	dm.logf("Player %s teleported to Overworld at (%.1f, %d, %.1f)", player.Name, owX, owY, owZ)
}

// sendRespawn sends the ClientboundRespawn packet to switch the player's dimension.
func (dm *DimensionManager) sendRespawn(player *game.Player, dimTypeID int32, dimName string, isFlat bool) {
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
		pk.Byte(0x01),                   // dataKept: keep entity data
	))
}

// sendPlayerPosition sends a position teleport to the player.
func (dm *DimensionManager) sendPlayerPosition(player *game.Player, x, y, z float64) {
	player.TeleportPending = true
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(3),        // teleport ID
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(0), pk.Float(0), // yaw, pitch
		pk.Int(0), // flags: all absolute
	))
}

// sendSpawnSequenceForDimension sends the spawn sequence for a dimension change.
func (dm *DimensionManager) sendSpawnSequenceForDimension(player *game.Player, encoder *ChunkSender, dimName string) {
	px, py, pz := player.Position()

	// Set default spawn position
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDefaultSpawnPosition,
		pk.Identifier(dimName),
		pk.Position{X: int(px), Y: int(py), Z: int(pz)},
		pk.Float(0), pk.Float(0),
	))

	// Game event: start waiting for level chunks
	player.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), pk.Float(0),
	))

	// Send chunks
	if encoder != nil {
		encoder.SendInitialChunks(player)
	}

	// Re-send health
	SendSetHealth(player)
}

// findSafeY finds a safe Y coordinate for spawning in a world.
// It scans upward to find a 2-block-tall air pocket.
func (dm *DimensionManager) findSafeY(world game.World, x, z, minY, maxY int) int {
	// First ensure the chunk is loaded by getting a block
	for y := minY + 1; y < maxY-2; y++ {
		stateBelow, err := world.GetBlock(x, y-1, z)
		if err != nil {
			continue
		}
		stateFeet, err := world.GetBlock(x, y, z)
		if err != nil {
			continue
		}
		stateHead, err := world.GetBlock(x, y+1, z)
		if err != nil {
			continue
		}

		// Need solid below, air at feet and head
		belowName := BlockNameFromState(int(stateBelow))
		feetName := BlockNameFromState(int(stateFeet))
		headName := BlockNameFromState(int(stateHead))

		isSolid := belowName != "" && belowName != "air" && belowName != "cave_air" &&
			belowName != "void_air" && belowName != "lava" && belowName != "water"
		isAirFeet := feetName == "" || feetName == "air" || feetName == "cave_air" || feetName == "void_air"
		isAirHead := headName == "" || headName == "air" || headName == "cave_air" || headName == "void_air"

		if isSolid && isAirFeet && isAirHead {
			return y
		}
	}

	// Fallback: spawn above the surface
	if maxY > 128 {
		return 70 // overworld fallback
	}
	return 65 // nether fallback
}

// TeleportToEnd sends a player from the Overworld to the End dimension.
func (dm *DimensionManager) TeleportToEnd(player *game.Player) {
	if dm.EndWorld == nil {
		return
	}

	// End spawn: obsidian platform at 100, 49, 0
	endX := 100.5
	endY := 49.0
	endZ := 0.5

	// Build obsidian platform at spawn point
	obsidianID, _ := block.ToStateID[block.Obsidian{}]
	for dz := -2; dz <= 2; dz++ {
		for dx := -2; dx <= 2; dx++ {
			dm.EndWorld.SetBlock(100+dx, 48, dz, obsidianID)
			// Clear blocks above the platform
			dm.EndWorld.SetBlock(100+dx, 49, dz, 0) // air
			dm.EndWorld.SetBlock(100+dx, 50, dz, 0) // air
			dm.EndWorld.SetBlock(100+dx, 51, dz, 0) // air
		}
	}

	dm.sendRespawn(player, dm.EndDimTypeID, "minecraft:the_end", false)

	prevDim := player.Dimension
	player.Dimension = "minecraft:the_end"
	if player.SessionEvents != nil {
		player.SessionEvents.OnDimensionChange(prevDim, "minecraft:the_end")
	}
	if dm.AdvMgr != nil {
		dm.AdvMgr.CheckDimensionChange(player, "minecraft:the_end")
	}
	player.LoadedChunks = make(map[game.ChunkPos]bool)
	player.SetPosition(endX, endY, endZ)
	player.FallStartY = -999
	player.PortalCooldown = 300 // 15 seconds

	dm.sendPlayerPosition(player, endX, endY, endZ)
	dm.sendSpawnSequenceForDimension(player, dm.EndEncoder, "minecraft:the_end")

	// Spawn dragon if not already active
	if dm.DragonMgr != nil {
		dm.DragonMgr.SpawnDragon()
		dm.DragonMgr.SendDragonToPlayer(player)
	}

	dm.logf("Player %s teleported to the End at (%.1f, %.1f, %.1f)", player.Name, endX, endY, endZ)
}

// TeleportFromEnd sends a player from the End back to the Overworld spawn.
func (dm *DimensionManager) TeleportFromEnd(player *game.Player) {
	owX := 0.5
	owY := dm.OverworldSpawnY
	owZ := 0.5

	// Remove boss bar if present
	if dm.DragonMgr != nil {
		dm.DragonMgr.RemoveBossBarFromPlayer(player)
	}

	dm.sendRespawn(player, dm.OverworldDimTypeID, "minecraft:overworld", dm.IsFlat)

	player.Dimension = "minecraft:overworld"
	if player.SessionEvents != nil {
		player.SessionEvents.OnDimensionChange("minecraft:the_end", "minecraft:overworld")
	}
	player.LoadedChunks = make(map[game.ChunkPos]bool)
	player.SetPosition(owX, owY, owZ)
	player.FallStartY = -999
	player.PortalCooldown = 300 // 15 seconds

	dm.sendPlayerPosition(player, owX, owY, owZ)
	dm.sendSpawnSequenceForDimension(player, dm.OverEncoder, "minecraft:overworld")

	dm.logf("Player %s teleported from End to Overworld at (%.1f, %.1f, %.1f)", player.Name, owX, owY, owZ)
}

// worldForPlayer returns the world the player is currently in.
func (dm *DimensionManager) worldForPlayer(player *game.Player) game.World {
	switch player.Dimension {
	case "minecraft:the_nether":
		return dm.NetherWorld
	case "minecraft:the_end":
		return dm.EndWorld
	default:
		return dm.OverWorld
	}
}

// WorldForPlayer is a public accessor that returns the world the player is currently in.
func (dm *DimensionManager) WorldForPlayer(player *game.Player) game.World {
	return dm.worldForPlayer(player)
}

// EncoderForPlayer returns the chunk encoder for the player's current dimension.
func (dm *DimensionManager) EncoderForPlayer(player *game.Player) *ChunkSender {
	switch player.Dimension {
	case "minecraft:the_nether":
		return dm.NetherEncoder
	case "minecraft:the_end":
		return dm.EndEncoder
	default:
		return dm.OverEncoder
	}
}

func (dm *DimensionManager) logf(format string, args ...any) {
	if dm.Logger != nil {
		dm.Logger.Printf(format, args...)
	}
}
