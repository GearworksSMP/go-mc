package handler

import (
	"log"
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Minecart entity type IDs (26.1-snapshot-2 registry).
const (
	minecartEntityType      int32 = 85
	chestMinecartEntityType int32 = 25
	furnaceMinecartEntityType int32 = 56
	hopperMinecartEntityType int32 = 65
)

// Minecart variant constants.
const (
	MinecartNormal  = 0
	MinecartChest   = 1
	MinecartHopper  = 2
	MinecartFurnace = 3
)

// Chest minecart window ID.
const ChestMinecartWindowID = 22

// Sound IDs for minecarts (from data/soundid/soundid.go).
const (
	SoundMinecartRiding int32 = 592 // entity.minecart.riding
)

// Minecart direction constants.
const (
	DirNorth = 0 // -Z
	DirEast  = 1 // +X
	DirSouth = 2 // +Z
	DirWest  = 3 // -X
)

// Minecart represents a minecart entity in the world.
type Minecart struct {
	EID       int32
	X, Y, Z   float64
	Yaw       float32
	Speed     float64 // blocks per tick (positive = forward in Direction)
	Direction int     // 0=north(-Z), 1=east(+X), 2=south(+Z), 3=west(-X)
	RiderEID  int32   // 0 = empty
	Health    float32
	TickAge   int64
	OnRail    bool
	Variant   int                 // MinecartNormal, MinecartChest, MinecartHopper, MinecartFurnace
	Items     [27]game.ItemStack  // chest minecart inventory
	FuelTicks int64               // furnace minecart fuel remaining
}

// MinecartManager manages minecart entities in the world.
type MinecartManager struct {
	Manager      *game.PlayerManager
	World        game.World
	RedstoneMgr  *RedstoneManager
	ItemEntities *ItemEntityManager
	Logger       *log.Logger
	mu           sync.Mutex
	carts        map[int32]*Minecart
}

// NewMinecartManager creates a new MinecartManager.
func NewMinecartManager(mgr *game.PlayerManager, world game.World, redstoneMgr *RedstoneManager, itemEntities *ItemEntityManager, logger *log.Logger) *MinecartManager {
	return &MinecartManager{
		Manager:      mgr,
		World:        world,
		RedstoneMgr:  redstoneMgr,
		ItemEntities: itemEntities,
		Logger:       logger,
		carts:        make(map[int32]*Minecart),
	}
}

// minecartEntityTypeForVariant returns the entity type ID for a minecart variant.
func minecartEntityTypeForVariant(variant int) int32 {
	switch variant {
	case MinecartChest:
		return chestMinecartEntityType
	case MinecartFurnace:
		return furnaceMinecartEntityType
	case MinecartHopper:
		return hopperMinecartEntityType
	default:
		return minecartEntityType
	}
}

// minecartItemForVariant returns the item name dropped when a minecart variant is destroyed.
func minecartItemForVariant(variant int) string {
	switch variant {
	case MinecartChest:
		return "chest_minecart"
	case MinecartFurnace:
		return "furnace_minecart"
	case MinecartHopper:
		return "hopper_minecart"
	default:
		return "minecart"
	}
}

// SpawnMinecart creates a new minecart entity at the given position and broadcasts it.
// The minecart is centered on the block (x+0.5, y, z+0.5).
// Returns the entity ID.
func (mm *MinecartManager) SpawnMinecart(x, y, z float64) int32 {
	return mm.SpawnMinecartVariant(x, y, z, MinecartNormal)
}

// SpawnMinecartVariant creates a new minecart of the given variant.
func (mm *MinecartManager) SpawnMinecartVariant(x, y, z float64, variant int) int32 {
	eid := mm.Manager.NextEntityID()
	cart := &Minecart{
		EID:     eid,
		X:       math.Floor(x) + 0.5,
		Y:       y,
		Z:       math.Floor(z) + 0.5,
		Health:  40,
		Variant: variant,
	}

	mm.mu.Lock()
	mm.carts[eid] = cart
	mm.mu.Unlock()

	// Broadcast spawn to all players
	entityUUID := uuid.New()
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(minecartEntityTypeForVariant(variant)),
		pk.Double(cart.X),
		pk.Double(cart.Y),
		pk.Double(cart.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	mm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	mm.logf("Spawned %s EID=%d at (%.1f, %.1f, %.1f)", minecartItemForVariant(variant), eid, cart.X, cart.Y, cart.Z)
	return eid
}

// MountMinecart mounts a player onto a minecart. Returns true if successful.
func (mm *MinecartManager) MountMinecart(player *game.Player, cartEID int32) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	cart, ok := mm.carts[cartEID]
	if !ok {
		return false
	}

	// Variant minecarts can't be ridden — interact instead
	if cart.Variant != MinecartNormal {
		return false
	}

	// Cart already has a rider
	if cart.RiderEID != 0 {
		return false
	}

	// Player already riding something
	if player.RidingEntityEID != 0 {
		return false
	}

	cart.RiderEID = player.EID
	player.RidingEntityEID = cartEID

	// Send SetPassengers to all players
	pkt := pk.Marshal(
		packetid.ClientboundSetPassengers,
		pk.VarInt(cartEID),
		pk.VarInt(1),          // passenger count
		pk.VarInt(player.EID), // passenger EID
	)
	mm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	mm.logf("Player %s mounted minecart EID=%d", player.Name, cartEID)
	return true
}

// DismountMinecart dismounts a player from the minecart they are riding.
func (mm *MinecartManager) DismountMinecart(player *game.Player) {
	mm.mu.Lock()
	cartEID := player.RidingEntityEID
	if cartEID == 0 {
		mm.mu.Unlock()
		return
	}

	cart, ok := mm.carts[cartEID]
	if ok {
		cart.RiderEID = 0
	}
	player.RidingEntityEID = 0
	mm.mu.Unlock()

	// Send SetPassengers with 0 passengers
	pkt := pk.Marshal(
		packetid.ClientboundSetPassengers,
		pk.VarInt(cartEID),
		pk.VarInt(0), // no passengers
	)
	mm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Teleport player above the minecart
	if ok {
		newX := cart.X
		newY := cart.Y + 0.7
		newZ := cart.Z
		player.SetPosition(newX, newY, newZ)
		player.TeleportPending = true

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

	mm.logf("Player %s dismounted minecart EID=%d", player.Name, cartEID)
}

// DamageMinecart applies damage to a minecart. Returns true if the minecart was destroyed.
func (mm *MinecartManager) DamageMinecart(attackerEID, cartEID int32, damage float32) bool {
	mm.mu.Lock()
	cart, ok := mm.carts[cartEID]
	if !ok {
		mm.mu.Unlock()
		return false
	}

	cart.Health -= damage
	if cart.Health <= 0 {
		cx, cy, cz := cart.X, cart.Y, cart.Z

		// Dismount rider if any
		var rider *game.Player
		if cart.RiderEID != 0 {
			rider = mm.Manager.GetByEID(cart.RiderEID)
		}
		delete(mm.carts, cartEID)
		mm.mu.Unlock()

		if rider != nil {
			rider.RidingEntityEID = 0
			emptyPkt := pk.Marshal(
				packetid.ClientboundSetPassengers,
				pk.VarInt(cartEID),
				pk.VarInt(0),
			)
			mm.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(emptyPkt)
			})
			rider.SetPosition(cx, cy+0.7, cz)
			rider.TeleportPending = true
			rider.WritePacket(pk.Marshal(
				packetid.ClientboundPlayerPosition,
				pk.VarInt(0),
				pk.Double(cx),
				pk.Double(cy+0.7),
				pk.Double(cz),
				pk.Double(0), pk.Double(0), pk.Double(0),
				pk.Float(0), pk.Float(0),
				pk.Int(0x08|0x10),
			))
		}

		// Remove entity from all clients
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(cartEID),
		)
		mm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})

		// Drop minecart item
		if mm.ItemEntities != nil {
			dropItemName := minecartItemForVariant(cart.Variant)
			dropItemID := itemIDByName(dropItemName)
			if dropItemID > 0 {
				mm.ItemEntities.SpawnItem(mm.Manager, cx, cy+0.5, cz, dropItemID, 1, 10)
			}
			// Drop chest minecart contents
			if cart.Variant == MinecartChest {
				for _, item := range cart.Items {
					if item.ID > 0 && item.Count > 0 {
						mm.ItemEntities.SpawnItem(mm.Manager, cx, cy+0.5, cz, item.ID, item.Count, 10)
					}
				}
			}
		}

		mm.logf("Minecart EID=%d destroyed", cartEID)
		return true
	}

	mm.mu.Unlock()

	// Broadcast hurt animation
	pkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(cartEID),
		pk.Float(0), // yaw
	)
	mm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})

	return false
}

// IsMinecart returns true if the given EID is a tracked minecart entity.
func (mm *MinecartManager) IsMinecart(eid int32) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	_, ok := mm.carts[eid]
	return ok
}

// PushMinecart gives a minecart a push in the given direction with the given speed.
func (mm *MinecartManager) PushMinecart(cartEID int32, direction int, speed float64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	cart, ok := mm.carts[cartEID]
	if !ok {
		return
	}
	if cart.Speed < speed {
		cart.Speed = speed
		cart.Direction = direction
	}
}

// Tick processes all minecarts (rail following, physics, position broadcasts).
func (mm *MinecartManager) Tick(tick int64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	for _, cart := range mm.carts {
		cart.TickAge++

		// Get block at minecart's feet position
		bx := int(math.Floor(cart.X))
		by := int(math.Floor(cart.Y))
		bz := int(math.Floor(cart.Z))

		railShape, railType := mm.getRailInfo(bx, by, bz)
		if railShape < 0 {
			// Check one block below (in case minecart is slightly above)
			railShape, railType = mm.getRailInfo(bx, by-1, bz)
			if railShape >= 0 {
				by = by - 1
			}
		}

		if railShape >= 0 {
			// On a rail
			cart.OnRail = true

			// Snap to rail center
			cart.X = float64(bx) + 0.5
			cart.Z = float64(bz) + 0.5

			// Handle rail shape
			mm.processRail(cart, railShape, railType, bx, by, bz)

			// Powered rail interactions
			if railType == railTypePowered {
				powered := false
				if mm.RedstoneMgr != nil {
					powered = mm.RedstoneMgr.IsPowered(bx, by, bz)
				}
				// Also check if the powered rail block state says "powered"
				if !powered {
					powered = mm.isPoweredRailActive(bx, by, bz)
				}

				if powered {
					// Boost: ensure minimum speed
					if cart.Speed < 0.4 {
						cart.Speed = 0.4
					}
				} else {
					// Brake: heavy friction
					cart.Speed *= 0.5
					if cart.Speed < 0.001 {
						cart.Speed = 0
					}
				}
			}

			// Apply movement
			mm.applyMovement(cart)

			// Normal friction
			cart.Speed *= 0.98
			if cart.Speed < 0.001 {
				cart.Speed = 0
			}

		} else {
			// Not on a rail: apply gravity and heavy friction
			cart.OnRail = false
			cart.Y -= 0.04 // gravity
			cart.Speed *= 0.5
			if cart.Speed < 0.001 {
				cart.Speed = 0
			}

			// Apply any remaining movement
			if cart.Speed > 0 {
				mm.applyMovement(cart)
			}

			// Check for solid block below to prevent falling through
			belowState, err := mm.World.GetBlock(bx, by-1, bz)
			if err == nil && belowState != 0 && !mm.isAirBlock(bx, by-1, bz) {
				if cart.Y < float64(by) {
					cart.Y = float64(by)
				}
			}
		}

		// Variant-specific tick behavior
		switch cart.Variant {
		case MinecartFurnace:
			mm.tickFurnaceMinecart(cart)
		case MinecartHopper:
			mm.tickHopperMinecart(cart)
		}

		// Sync rider position
		if cart.RiderEID != 0 {
			rider := mm.Manager.GetByEID(cart.RiderEID)
			if rider != nil {
				rider.SetPosition(cart.X, cart.Y+0.2, cart.Z)
			}
		}

		// Update yaw based on direction
		cart.Yaw = directionToYaw(cart.Direction)

		// Broadcast position periodically (every 3 ticks if moving, every 20 if still)
		if cart.Speed > 0.001 {
			if cart.TickAge%3 == 0 {
				mm.broadcastCartPosition(cart)
			}
		} else if cart.TickAge%20 == 0 {
			mm.broadcastCartPosition(cart)
		}
	}
}

// processRail handles direction changes based on rail shape.
func (mm *MinecartManager) processRail(cart *Minecart, shape int, railType int, bx, by, bz int) {
	switch block.RailShape(shape) {
	case block.RailShapeNorthSouth:
		// Constrain to N/S movement
		if cart.Direction == DirEast || cart.Direction == DirWest {
			cart.Direction = DirNorth
		}
	case block.RailShapeEastWest:
		// Constrain to E/W movement
		if cart.Direction == DirNorth || cart.Direction == DirSouth {
			cart.Direction = DirEast
		}
	case block.RailShapeAscendingEast:
		if cart.Direction == DirEast {
			cart.Y = float64(by) + 1.0
		} else if cart.Direction == DirWest {
			// Going down from east
		}
		if cart.Direction == DirNorth || cart.Direction == DirSouth {
			cart.Direction = DirEast
		}
	case block.RailShapeAscendingWest:
		if cart.Direction == DirWest {
			cart.Y = float64(by) + 1.0
		}
		if cart.Direction == DirNorth || cart.Direction == DirSouth {
			cart.Direction = DirWest
		}
	case block.RailShapeAscendingNorth:
		if cart.Direction == DirNorth {
			cart.Y = float64(by) + 1.0
		}
		if cart.Direction == DirEast || cart.Direction == DirWest {
			cart.Direction = DirNorth
		}
	case block.RailShapeAscendingSouth:
		if cart.Direction == DirSouth {
			cart.Y = float64(by) + 1.0
		}
		if cart.Direction == DirEast || cart.Direction == DirWest {
			cart.Direction = DirSouth
		}
	case block.RailShapeSouthEast:
		// Curve: coming from south → turn east, coming from east → turn south
		if cart.Direction == DirNorth {
			cart.Direction = DirEast
		} else if cart.Direction == DirWest {
			cart.Direction = DirSouth
		}
	case block.RailShapeSouthWest:
		if cart.Direction == DirNorth {
			cart.Direction = DirWest
		} else if cart.Direction == DirEast {
			cart.Direction = DirSouth
		}
	case block.RailShapeNorthWest:
		if cart.Direction == DirSouth {
			cart.Direction = DirWest
		} else if cart.Direction == DirEast {
			cart.Direction = DirNorth
		}
	case block.RailShapeNorthEast:
		if cart.Direction == DirSouth {
			cart.Direction = DirEast
		} else if cart.Direction == DirWest {
			cart.Direction = DirNorth
		}
	}
}

// applyMovement moves the minecart in its current direction.
func (mm *MinecartManager) applyMovement(cart *Minecart) {
	switch cart.Direction {
	case DirNorth:
		cart.Z -= cart.Speed
	case DirEast:
		cart.X += cart.Speed
	case DirSouth:
		cart.Z += cart.Speed
	case DirWest:
		cart.X -= cart.Speed
	}
}

// Rail type constants.
const (
	railTypeNormal   = 0
	railTypePowered  = 1
	railTypeDetector = 2
	railTypeActivator = 3
)

// getRailInfo returns the rail shape and rail type at (x, y, z), or (-1, -1) if not a rail.
func (mm *MinecartManager) getRailInfo(x, y, z int) (shape int, railType int) {
	stateID, err := mm.World.GetBlock(x, y, z)
	if err != nil {
		return -1, -1
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return -1, -1
	}

	switch rail := block.StateList[stateID].(type) {
	case block.Rail:
		return int(rail.Shape), railTypeNormal
	case block.PoweredRail:
		return int(rail.Shape), railTypePowered
	case block.DetectorRail:
		return int(rail.Shape), railTypeDetector
	case block.ActivatorRail:
		return int(rail.Shape), railTypeActivator
	}
	return -1, -1
}

// isPoweredRailActive checks if the powered rail block at (x,y,z) has its Powered property set.
func (mm *MinecartManager) isPoweredRailActive(x, y, z int) bool {
	stateID, err := mm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	if rail, ok := block.StateList[stateID].(block.PoweredRail); ok {
		return bool(rail.Powered)
	}
	return false
}

// isAirBlock returns true if the block at (x,y,z) is air.
func (mm *MinecartManager) isAirBlock(x, y, z int) bool {
	stateID, err := mm.World.GetBlock(x, y, z)
	if err != nil {
		return true
	}
	return stateID == 0
}

// broadcastCartPosition sends a teleport update for a minecart to all players.
// Must be called with mm.mu held.
func (mm *MinecartManager) broadcastCartPosition(cart *Minecart) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(cart.EID),
		pk.Double(cart.X),
		pk.Double(cart.Y),
		pk.Double(cart.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(cart.Yaw), pk.Float(0), // yaw, pitch
		pk.Int(0), // relative flags (all absolute)
		pk.Boolean(false), // on ground
	)
	mm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SendExistingMinecarts sends all current minecarts to a newly joined player.
func (mm *MinecartManager) SendExistingMinecarts(player *game.Player) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	for _, cart := range mm.carts {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(cart.EID),
			pk.UUID(entityUUID),
			pk.VarInt(minecartEntityTypeForVariant(cart.Variant)),
			pk.Double(cart.X),
			pk.Double(cart.Y),
			pk.Double(cart.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(degToAngle(cart.Yaw)),
			pk.Angle(0),
			pk.VarInt(0),
		))

		// Send passenger info if minecart has a rider
		if cart.RiderEID != 0 {
			player.WritePacket(pk.Marshal(
				packetid.ClientboundSetPassengers,
				pk.VarInt(cart.EID),
				pk.VarInt(1),
				pk.VarInt(cart.RiderEID),
			))
		}
	}
}

// HandleMoveVehicle processes ServerboundMoveVehicle for minecart movement.
// Returns true if the packet was consumed (player is riding a minecart).
func (mm *MinecartManager) HandleMoveVehicle(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundMoveVehicle {
		return false
	}

	if player.RidingEntityEID == 0 {
		return false
	}

	// Check if the player is riding a minecart specifically
	mm.mu.Lock()
	_, isCart := mm.carts[player.RidingEntityEID]
	mm.mu.Unlock()

	if !isCart {
		return false
	}

	// Parse the packet but don't let the client control the minecart position directly.
	// Minecarts are server-authoritative (follow rails).
	var x, y, z pk.Double
	var yaw, pitch pk.Float
	if err := p.Scan(&x, &y, &z, &yaw, &pitch); err != nil {
		return true
	}

	// For minecarts, the server controls position (rail physics), so we just consume the packet.
	// But we update the yaw to match what the player wants for the cart direction.
	mm.mu.Lock()
	if cart, ok := mm.carts[player.RidingEntityEID]; ok {
		// Player looking direction can influence initial push direction
		cart.Yaw = float32(yaw)
	}
	mm.mu.Unlock()

	return true
}

// directionToYaw converts a direction constant to a yaw angle in degrees.
func directionToYaw(dir int) float32 {
	switch dir {
	case DirNorth:
		return 180
	case DirEast:
		return 270
	case DirSouth:
		return 0
	case DirWest:
		return 90
	}
	return 0
}

// yawToMinecartDirection converts a player yaw angle to the nearest cardinal direction.
func yawToMinecartDirection(yaw float32) int {
	// Normalize yaw to 0-360
	y := math.Mod(float64(yaw), 360)
	if y < 0 {
		y += 360
	}
	// Yaw: 0=south, 90=west, 180=north, 270=east
	switch {
	case y >= 315 || y < 45:
		return DirSouth
	case y >= 45 && y < 135:
		return DirWest
	case y >= 135 && y < 225:
		return DirNorth
	default:
		return DirEast
	}
}

// isMinecartItem returns true if the item name is a placeable minecart.
func isMinecartItem(name string) bool {
	switch name {
	case "minecart", "chest_minecart", "hopper_minecart", "furnace_minecart":
		return true
	}
	return false
}

// minecartVariantForItem returns the variant for a minecart item name.
func minecartVariantForItem(name string) int {
	switch name {
	case "chest_minecart":
		return MinecartChest
	case "hopper_minecart":
		return MinecartHopper
	case "furnace_minecart":
		return MinecartFurnace
	default:
		return MinecartNormal
	}
}

// isRailBlock returns true if the block name is a rail type.
func isRailBlock(name string) bool {
	switch name {
	case "rail", "powered_rail", "detector_rail", "activator_rail":
		return true
	}
	return false
}

// isRailItem returns true if the item name is a rail type.
func isRailItem(name string) bool {
	return isRailBlock(name)
}

// PlaceRail places a rail block at (x, y, z) with auto-curving based on adjacent rails.
// Returns the state ID if placed, or 0 if cannot place.
func (mm *MinecartManager) PlaceRail(x, y, z int, railName string) (level.BlocksState, bool) {
	shape := mm.determineRailShape(x, y, z, railName)

	var blk block.Block
	switch railName {
	case "rail":
		blk = block.Rail{Shape: shape, Waterlogged: false}
	case "powered_rail":
		s := limitRailShape(shape)
		blk = block.PoweredRail{Powered: false, Shape: s, Waterlogged: false}
	case "detector_rail":
		s := limitRailShape(shape)
		blk = block.DetectorRail{Powered: false, Shape: s, Waterlogged: false}
	case "activator_rail":
		s := limitRailShape(shape)
		blk = block.ActivatorRail{Powered: false, Shape: s, Waterlogged: false}
	default:
		return 0, false
	}

	stateID, ok := block.ToStateID[blk]
	if !ok {
		return 0, false
	}
	return stateID, true
}

// limitRailShape constrains a shape to straight-only (no curves) for powered/detector/activator rails.
func limitRailShape(shape block.RailShape) block.RailShape {
	switch shape {
	case block.RailShapeNorthSouth, block.RailShapeEastWest,
		block.RailShapeAscendingEast, block.RailShapeAscendingWest,
		block.RailShapeAscendingNorth, block.RailShapeAscendingSouth:
		return shape
	default:
		// Curves not allowed for non-normal rails, default to north_south
		return block.RailShapeNorthSouth
	}
}

// determineRailShape checks adjacent rails and determines the best shape for a new rail.
func (mm *MinecartManager) determineRailShape(x, y, z int, railName string) block.RailShape {
	// Check four cardinal neighbors for rails
	hasNorth := mm.hasRailAt(x, y, z-1) || mm.hasRailAt(x, y+1, z-1) || mm.hasRailAt(x, y-1, z-1)
	hasSouth := mm.hasRailAt(x, y, z+1) || mm.hasRailAt(x, y+1, z+1) || mm.hasRailAt(x, y-1, z+1)
	hasEast := mm.hasRailAt(x+1, y, z) || mm.hasRailAt(x+1, y+1, z) || mm.hasRailAt(x+1, y-1, z)
	hasWest := mm.hasRailAt(x-1, y, z) || mm.hasRailAt(x-1, y+1, z) || mm.hasRailAt(x-1, y-1, z)

	// Check for ascending rails (rail exists one block up in a direction)
	if hasEast && mm.hasRailAt(x+1, y+1, z) {
		return block.RailShapeAscendingEast
	}
	if hasWest && mm.hasRailAt(x-1, y+1, z) {
		return block.RailShapeAscendingWest
	}
	if hasNorth && mm.hasRailAt(x, y+1, z-1) {
		return block.RailShapeAscendingNorth
	}
	if hasSouth && mm.hasRailAt(x, y+1, z+1) {
		return block.RailShapeAscendingSouth
	}

	// Curves (only for normal rails)
	if railName == "rail" {
		if hasSouth && hasEast {
			return block.RailShapeSouthEast
		}
		if hasSouth && hasWest {
			return block.RailShapeSouthWest
		}
		if hasNorth && hasWest {
			return block.RailShapeNorthWest
		}
		if hasNorth && hasEast {
			return block.RailShapeNorthEast
		}
	}

	// Straight rails
	if hasEast || hasWest {
		return block.RailShapeEastWest
	}

	// Default: north-south
	return block.RailShapeNorthSouth
}

// hasRailAt returns true if there is a rail block at (x, y, z).
func (mm *MinecartManager) hasRailAt(x, y, z int) bool {
	stateID, err := mm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	switch block.StateList[stateID].(type) {
	case block.Rail, block.PoweredRail, block.DetectorRail, block.ActivatorRail:
		return true
	}
	return false
}

// PlayerPushMinecart checks if a player is close enough to push a minecart,
// and gives it a push in the player's movement direction.
func (mm *MinecartManager) PlayerPushMinecart(player *game.Player, oldX, oldZ float64) {
	px, py, pz := player.Position()
	dx := px - oldX
	dz := pz - oldZ
	if dx == 0 && dz == 0 {
		return // not moving
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	for _, cart := range mm.carts {
		if cart.RiderEID != 0 {
			continue // skip occupied carts
		}

		// Distance check: within 1.0 block
		dist := math.Sqrt((px-cart.X)*(px-cart.X) + (py-cart.Y)*(py-cart.Y) + (pz-cart.Z)*(pz-cart.Z))
		if dist > 1.2 {
			continue
		}

		// Determine push direction from player movement
		dir := yawToMinecartDirection(player.Yaw)

		// Give a small push
		if cart.Speed < 0.1 {
			cart.Speed = 0.1
			cart.Direction = dir
		}
	}
}

// InteractMinecart handles right-click on a variant minecart. Returns true if handled.
func (mm *MinecartManager) InteractMinecart(player *game.Player, cartEID int32) bool {
	mm.mu.Lock()
	cart, ok := mm.carts[cartEID]
	if !ok {
		mm.mu.Unlock()
		return false
	}
	variant := cart.Variant
	mm.mu.Unlock()

	switch variant {
	case MinecartChest:
		mm.openChestMinecart(player, cartEID)
		return true
	case MinecartFurnace:
		mm.fuelFurnaceMinecart(player, cartEID)
		return true
	}
	return false
}

// openChestMinecart opens the chest minecart inventory for the player.
func (mm *MinecartManager) openChestMinecart(player *game.Player, cartEID int32) {
	player.OpenWindowID = ChestMinecartWindowID
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(ChestMinecartWindowID),
		pk.VarInt(2), // menu type: generic_9x3
		pk.String(`{"text":"Minecart with Chest"}`),
	))

	mm.mu.Lock()
	cart, ok := mm.carts[cartEID]
	if !ok {
		mm.mu.Unlock()
		return
	}

	// Build container slots (27 chest + 27 main inv + 9 hotbar = 63)
	slots := make(game.Slot261Array, 63)
	for i := 0; i < 27; i++ {
		slots[i] = cart.Items[i].ToSlot()
	}
	mm.mu.Unlock()

	// Fill player inventory
	for i := 9; i <= 35; i++ {
		slots[27+(i-9)] = player.Inventory[i].ToSlot()
	}
	for i := 36; i <= 44; i++ {
		slots[54+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	stateID := player.NextStateID()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(ChestMinecartWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// fuelFurnaceMinecart adds fuel if the player is holding coal/charcoal.
func (mm *MinecartManager) fuelFurnaceMinecart(player *game.Player, cartEID int32) {
	slot := player.HeldSlot
	held := &player.Inventory[36+slot]
	if held.ID == 0 || held.Count == 0 {
		return
	}
	itemName := ItemNameByID(held.ID)
	if itemName != "coal" && itemName != "charcoal" {
		return
	}

	mm.mu.Lock()
	cart, ok := mm.carts[cartEID]
	if !ok {
		mm.mu.Unlock()
		return
	}
	cart.FuelTicks += 200 // 10 seconds of fuel per item
	mm.mu.Unlock()

	// Consume fuel item
	held.Count--
	if held.Count <= 0 {
		*held = game.ItemStack{}
	}
	SendSlotUpdate(player, 36+int(slot))

	// Play fuel sound
	BroadcastSound(mm.Manager, SoundMinecartRiding, SoundCategoryNeutral,
		cart.X, cart.Y, cart.Z, 0.5, 1.0)
}

// tickFurnaceMinecart handles furnace minecart self-propulsion.
func (mm *MinecartManager) tickFurnaceMinecart(cart *Minecart) {
	if cart.FuelTicks <= 0 {
		return
	}
	cart.FuelTicks--

	// Self-propel at 0.4 blocks/tick while fueled
	if cart.Speed < 0.4 {
		cart.Speed = 0.4
	}
}

// tickHopperMinecart handles hopper minecart item pickup.
func (mm *MinecartManager) tickHopperMinecart(cart *Minecart) {
	if mm.ItemEntities == nil {
		return
	}

	// Only pick up every 8 ticks
	if cart.TickAge%8 != 0 {
		return
	}

	// Find first empty or stackable slot
	items := mm.ItemEntities.FindItemsNear(cart.X, cart.Y+0.5, cart.Z, 1.0)
	for _, ie := range items {
		// Try to add to existing stack first
		added := false
		for i := 0; i < 5; i++ { // hopper minecart has 5 slots
			if cart.Items[i].ID == ie.ItemID && cart.Items[i].Count < 64 {
				cart.Items[i].Count++
				mm.ItemEntities.RemoveItem(ie.EID)
				added = true
				break
			}
		}
		if added {
			continue
		}
		// Try empty slot
		for i := 0; i < 5; i++ {
			if cart.Items[i].ID == 0 || cart.Items[i].Count == 0 {
				cart.Items[i] = game.ItemStack{ID: ie.ItemID, Count: 1}
				mm.ItemEntities.RemoveItem(ie.EID)
				break
			}
		}
	}
}

func (mm *MinecartManager) logf(format string, args ...any) {
	if mm.Logger != nil {
		mm.Logger.Printf(format, args...)
	}
}
