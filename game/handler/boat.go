package handler

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Boat entity type ID for AddEntity (26.1-snapshot-2: oak_boat = 89).
const boatEntityType int32 = 89

// Wood type constants for boat variants.
const (
	BoatWoodOak      int32 = 0
	BoatWoodSpruce   int32 = 1
	BoatWoodBirch    int32 = 2
	BoatWoodJungle   int32 = 3
	BoatWoodAcacia   int32 = 4
	BoatWoodCherry   int32 = 5
	BoatWoodDarkOak  int32 = 6
	BoatWoodMangrove int32 = 7
)

// Boat represents a boat entity in the world.
type Boat struct {
	EID      int32
	X, Y, Z  float64
	Yaw      float32
	Health   float32
	WoodType int32
	RiderEID int32 // 0 = empty
	VelX     float64
	VelY     float64
	VelZ     float64
	TickAge  int64
}

// BoatManager manages boat entities in the world.
type BoatManager struct {
	Manager      *game.PlayerManager
	World        game.World
	ItemEntities *ItemEntityManager
	Logger       *log.Logger
	mu           sync.Mutex
	boats        map[int32]*Boat
}

// NewBoatManager creates a new BoatManager.
func NewBoatManager(mgr *game.PlayerManager, world game.World, itemEntities *ItemEntityManager, logger *log.Logger) *BoatManager {
	return &BoatManager{
		Manager:      mgr,
		World:        world,
		ItemEntities: itemEntities,
		Logger:       logger,
		boats:        make(map[int32]*Boat),
	}
}

// SpawnBoat creates a new boat entity and broadcasts it to all players.
// Returns the entity ID of the new boat.
func (bm *BoatManager) SpawnBoat(x, y, z float64, woodType int32) int32 {
	eid := bm.Manager.NextEntityID()
	boat := &Boat{
		EID:      eid,
		X:        x,
		Y:        y,
		Z:        z,
		Health:   40,
		WoodType: woodType,
	}

	bm.mu.Lock()
	bm.boats[eid] = boat
	bm.mu.Unlock()

	// Broadcast spawn to all players
	entityUUID := uuid.New()
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(boatEntityType),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Broadcast boat type metadata (entity data index 11, VarInt serializer)
	bm.broadcastBoatMetadata(boat)

	bm.logf("Spawned boat EID=%d type=%d at (%.1f, %.1f, %.1f)", eid, woodType, x, y, z)
	return eid
}

// broadcastBoatMetadata sends entity metadata for the boat's wood type.
// Boat entity metadata: index 11 = Type (VarInt), serializer ID = 1 (VarInt).
func (bm *BoatManager) broadcastBoatMetadata(boat *Boat) {
	// Build metadata: index 11, type 1 (VarInt), value = woodType
	// Then terminator 0xFF
	var metaBuf []byte
	metaBuf = append(metaBuf, 11) // index
	metaBuf = appendVarInt(metaBuf, 1) // serializer: VarInt
	metaBuf = appendVarInt(metaBuf, int(boat.WoodType))
	metaBuf = append(metaBuf, 0xFF) // terminator

	pkt := pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(boat.EID),
		pk.PluginMessageData(metaBuf),
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// MountBoat mounts a player onto a boat. Returns true if successful.
func (bm *BoatManager) MountBoat(player *game.Player, boatEID int32) bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	boat, ok := bm.boats[boatEID]
	if !ok {
		return false
	}

	// Boat already has a rider
	if boat.RiderEID != 0 {
		return false
	}

	// Player already riding something
	if player.RidingEntityEID != 0 {
		return false
	}

	boat.RiderEID = player.EID
	player.RidingEntityEID = boatEID

	// Send SetPassengers to all players
	pkt := pk.Marshal(
		packetid.ClientboundSetPassengers,
		pk.VarInt(boatEID),
		pk.VarInt(1),          // passenger count
		pk.VarInt(player.EID), // passenger EID
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	bm.logf("Player %s mounted boat EID=%d", player.Name, boatEID)
	return true
}

// DismountBoat dismounts a player from the boat they are riding.
func (bm *BoatManager) DismountBoat(player *game.Player) {
	bm.mu.Lock()
	boatEID := player.RidingEntityEID
	if boatEID == 0 {
		bm.mu.Unlock()
		return
	}

	boat, ok := bm.boats[boatEID]
	if ok {
		boat.RiderEID = 0
	}
	player.RidingEntityEID = 0
	bm.mu.Unlock()

	// Send SetPassengers with 0 passengers
	pkt := pk.Marshal(
		packetid.ClientboundSetPassengers,
		pk.VarInt(boatEID),
		pk.VarInt(0), // no passengers
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Teleport player slightly above the boat position
	if ok {
		newX := boat.X
		newY := boat.Y + 0.6
		newZ := boat.Z
		player.SetPosition(newX, newY, newZ)

		// Send position update to the player
		player.WritePacket(pk.Marshal(
			packetid.ClientboundPlayerPosition,
			pk.VarInt(0),       // teleport ID
			pk.Double(newX),
			pk.Double(newY),
			pk.Double(newZ),
			pk.Double(0),       // vel_x
			pk.Double(0),       // vel_y
			pk.Double(0),       // vel_z
			pk.Float(0),        // yaw (relative = 0)
			pk.Float(0),        // pitch (relative = 0)
			pk.Int(0x08|0x10),  // flags: yaw+pitch relative
		))
	}

	bm.logf("Player %s dismounted boat EID=%d", player.Name, boatEID)
}

// DamageBoat applies damage to a boat. Returns true if the boat was destroyed.
func (bm *BoatManager) DamageBoat(attackerEID int32, boatEID int32, damage float32) bool {
	bm.mu.Lock()
	boat, ok := bm.boats[boatEID]
	if !ok {
		bm.mu.Unlock()
		return false
	}

	boat.Health -= damage
	if boat.Health <= 0 {
		// Copy position for item drop
		bx, by, bz := boat.X, boat.Y, boat.Z
		woodType := boat.WoodType

		// Dismount rider if any
		var rider *game.Player
		if boat.RiderEID != 0 {
			rider = bm.Manager.GetByEID(boat.RiderEID)
		}
		delete(bm.boats, boatEID)
		bm.mu.Unlock()

		if rider != nil {
			rider.RidingEntityEID = 0
			// Send empty passengers
			emptyPkt := pk.Marshal(
				packetid.ClientboundSetPassengers,
				pk.VarInt(boatEID),
				pk.VarInt(0),
			)
			bm.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(emptyPkt)
			})
			// Teleport rider out
			rider.SetPosition(bx, by+0.6, bz)
			rider.WritePacket(pk.Marshal(
				packetid.ClientboundPlayerPosition,
				pk.VarInt(0),
				pk.Double(bx),
				pk.Double(by+0.6),
				pk.Double(bz),
				pk.Double(0), pk.Double(0), pk.Double(0),
				pk.Float(0), pk.Float(0),
				pk.Int(0x08|0x10),
			))
		}

		// Remove entity
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(boatEID),
		)
		bm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})

		// Drop boat item
		if bm.ItemEntities != nil {
			boatItemID := boatItemIDForWoodType(woodType)
			if boatItemID > 0 {
				bm.ItemEntities.SpawnItem(bm.Manager, bx, by+0.5, bz, boatItemID, 1, 10)
			}
		}

		bm.logf("Boat EID=%d destroyed", boatEID)
		return true
	}

	bm.mu.Unlock()

	// Broadcast hurt animation
	pkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(boatEID),
		pk.Float(0), // yaw
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	return false
}

// IsBoat returns true if the given EID is a tracked boat entity.
func (bm *BoatManager) IsBoat(eid int32) bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	_, ok := bm.boats[eid]
	return ok
}

// Tick processes all boats (physics, position broadcasts).
func (bm *BoatManager) Tick(tick int64) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, boat := range bm.boats {
		boat.TickAge++

		// Water physics
		bx := int(math.Floor(boat.X))
		by := int(math.Floor(boat.Y))
		bz := int(math.Floor(boat.Z))

		inWater := bm.isWaterAt(bx, by, bz)
		onWater := bm.isWaterAt(bx, by-1, bz) && !inWater

		if inWater {
			// Float upward toward water surface
			boat.VelY = 0.04
		} else if onWater {
			// On water surface: stabilize
			boat.VelY = 0
		} else {
			// Gravity when not in water
			boat.VelY -= 0.04
		}

		// Check if boat is sitting on a solid block
		belowState, err := bm.World.GetBlock(bx, by-1, bz)
		if err == nil && belowState != 0 && !bm.isWaterAt(bx, by-1, bz) {
			if boat.VelY < 0 {
				boat.VelY = 0
			}
		}

		// Apply velocity
		boat.X += boat.VelX
		boat.Y += boat.VelY
		boat.Z += boat.VelZ

		// Friction
		boat.VelX *= 0.9
		boat.VelZ *= 0.9

		// If has rider, update boat position based on rider
		if boat.RiderEID != 0 {
			rider := bm.Manager.GetByEID(boat.RiderEID)
			if rider != nil {
				yaw, _ := rider.Rotation()
				boat.Yaw = yaw
			}
		}

		// Broadcast position periodically (every 3 ticks)
		if boat.TickAge%3 == 0 {
			bm.broadcastBoatPosition(boat)
		}
	}
}

// HandleMoveVehicle processes ServerboundMoveVehicle for boat movement.
func (bm *BoatManager) HandleMoveVehicle(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundMoveVehicle {
		return false
	}

	if player.RidingEntityEID == 0 {
		return true // consume but ignore
	}

	var x, y, z pk.Double
	var yaw, pitch pk.Float
	if err := p.Scan(&x, &y, &z, &yaw, &pitch); err != nil {
		return true
	}

	bm.mu.Lock()
	boat, ok := bm.boats[player.RidingEntityEID]
	if ok {
		boat.X = float64(x)
		boat.Y = float64(y)
		boat.Z = float64(z)
		boat.Yaw = float32(yaw)
	}
	bm.mu.Unlock()

	// Update player position to match boat
	player.SetPosition(float64(x), float64(y), float64(z))

	// Broadcast to other players
	if ok {
		bm.mu.Lock()
		bm.broadcastBoatPosition(boat)
		bm.mu.Unlock()
	}

	return true
}

// HandlePaddleBoat processes ServerboundPaddleBoat packets (paddle animations).
func (bm *BoatManager) HandlePaddleBoat(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundPaddleBoat {
		return false
	}
	// Paddle state is cosmetic; just consume the packet
	return true
}

// broadcastBoatPosition sends a teleport update for a boat to all players.
// Must be called with bm.mu held.
func (bm *BoatManager) broadcastBoatPosition(boat *Boat) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(boat.EID),
		pk.Double(boat.X),
		pk.Double(boat.Y),
		pk.Double(boat.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(boat.Yaw), pk.Float(0), // yaw, pitch
		pk.Int(0), // relative flags (all absolute)
		pk.Boolean(false), // on ground
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SendExistingBoats sends all current boats to a newly joined player.
func (bm *BoatManager) SendExistingBoats(player *game.Player) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, boat := range bm.boats {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(boat.EID),
			pk.UUID(entityUUID),
			pk.VarInt(boatEntityType),
			pk.Double(boat.X),
			pk.Double(boat.Y),
			pk.Double(boat.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(degToAngle(boat.Yaw)),
			pk.Angle(0),
			pk.VarInt(0),
		))

		// Send type metadata
		var metaBuf []byte
		metaBuf = append(metaBuf, 11)
		metaBuf = appendVarInt(metaBuf, 1)
		metaBuf = appendVarInt(metaBuf, int(boat.WoodType))
		metaBuf = append(metaBuf, 0xFF)
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityData,
			pk.VarInt(boat.EID),
			pk.PluginMessageData(metaBuf),
		))

		// Send passenger info if boat has a rider
		if boat.RiderEID != 0 {
			player.WritePacket(pk.Marshal(
				packetid.ClientboundSetPassengers,
				pk.VarInt(boat.EID),
				pk.VarInt(1),
				pk.VarInt(boat.RiderEID),
			))
		}
	}
}

// isWaterAt checks if the block at (x, y, z) is water.
func (bm *BoatManager) isWaterAt(x, y, z int) bool {
	state, err := bm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) < len(block.StateList) && block.StateList[state] != nil {
		_, ok := block.StateList[state].(block.Water)
		return ok
	}
	return false
}

// isBoatItem returns true if the item name corresponds to a boat item,
// and returns the wood type.
func isBoatItem(name string) (int32, bool) {
	switch name {
	case "oak_boat":
		return BoatWoodOak, true
	case "spruce_boat":
		return BoatWoodSpruce, true
	case "birch_boat":
		return BoatWoodBirch, true
	case "jungle_boat":
		return BoatWoodJungle, true
	case "acacia_boat":
		return BoatWoodAcacia, true
	case "cherry_boat":
		return BoatWoodCherry, true
	case "dark_oak_boat":
		return BoatWoodDarkOak, true
	case "mangrove_boat":
		return BoatWoodMangrove, true
	}
	// Also match chest boats (same wood type, different entity but use same boat entity for now)
	if strings.HasSuffix(name, "_chest_boat") {
		base := strings.TrimSuffix(name, "_chest_boat")
		switch base {
		case "oak":
			return BoatWoodOak, true
		case "spruce":
			return BoatWoodSpruce, true
		case "birch":
			return BoatWoodBirch, true
		case "jungle":
			return BoatWoodJungle, true
		case "acacia":
			return BoatWoodAcacia, true
		case "cherry":
			return BoatWoodCherry, true
		case "dark_oak":
			return BoatWoodDarkOak, true
		case "mangrove":
			return BoatWoodMangrove, true
		}
	}
	return 0, false
}

// boatItemIDForWoodType returns the item ID for a boat of the given wood type.
func boatItemIDForWoodType(woodType int32) int32 {
	var name string
	switch woodType {
	case BoatWoodOak:
		name = "oak_boat"
	case BoatWoodSpruce:
		name = "spruce_boat"
	case BoatWoodBirch:
		name = "birch_boat"
	case BoatWoodJungle:
		name = "jungle_boat"
	case BoatWoodAcacia:
		name = "acacia_boat"
	case BoatWoodCherry:
		name = "cherry_boat"
	case BoatWoodDarkOak:
		name = "dark_oak_boat"
	case BoatWoodMangrove:
		name = "mangrove_boat"
	default:
		name = "oak_boat"
	}
	return itemIDByName(name)
}

// appendVarInt appends a VarInt to a byte slice and returns the new slice.
func appendVarInt(buf []byte, v int) []byte {
	val := uint32(v)
	for val >= 0x80 {
		buf = append(buf, byte(val&0x7F)|0x80)
		val >>= 7
	}
	buf = append(buf, byte(val))
	return buf
}

func (bm *BoatManager) logf(format string, args ...any) {
	if bm.Logger != nil {
		bm.Logger.Printf(format, args...)
	}
}
