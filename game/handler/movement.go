// Package handler provides play-phase packet handlers for the game server.
package handler

import (
	"log"
	"math"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// MovementHandler processes player movement packets and triggers chunk load/unload.
type MovementHandler struct {
	World           game.World
	Manager         *game.PlayerManager
	Logger          *log.Logger
	Encoder         *ChunkSender
	SurvivalHandler *SurvivalHandler
	CropMgr         *CropManager
	BedMgr          *BedManager
	BoatMgr         *BoatManager
	MinecartMgr     *MinecartManager
	RedstoneMgr     *RedstoneManager
	DimensionMgr    *DimensionManager // optional; when set, uses dimension-aware world/encoder

	// OnRegionCrossing is called when a player moves into a different region (32x32 chunks).
	// Used by the cluster region server to trigger player transfers.
	OnRegionCrossing func(player *game.Player, newRegion game.RegionPos)
}

// HandlePacket processes a single packet for the given player.
// Returns true if the packet was handled.
func (h *MovementHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	// Wake sleeping players on any movement input
	if player.Sleeping && h.BedMgr != nil {
		pid := packetid.ServerboundPacketID(p.ID)
		if pid == packetid.ServerboundMovePlayerPos ||
			pid == packetid.ServerboundMovePlayerPosRot ||
			pid == packetid.ServerboundMovePlayerRot {
			h.BedMgr.WakePlayer(player)
		}
	}

	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundMovePlayerPos:
		var x, y, z pk.Double
		var flags pk.VarInt
		if err := p.Scan(&x, &y, &z, &flags); err != nil {
			return true
		}
		onGround := (int32(flags) & 0x01) != 0

		// Spectators: update position and broadcast, skip all survival mechanics
		if IsSpectator(player) {
			oldX, oldY, oldZ := player.Position()
			oldChunk := player.ChunkPos()
			player.SetPosition(float64(x), float64(y), float64(z))
			newChunk := player.ChunkPos()
			if oldChunk != newChunk {
				h.onChunkChange(player, oldChunk, newChunk)
			}
			h.broadcastPos(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z))
			return true
		}

		wasOnGround := player.OnGround
		player.OnGround = onGround
		if !h.validateMovement(player, float64(x), float64(y), float64(z)) {
			return true
		}
		oldX, oldY, oldZ := player.Position()
		oldChunk := player.ChunkPos()
		player.SetPosition(float64(x), float64(y), float64(z))
		newChunk := player.ChunkPos()
		if oldChunk != newChunk {
			h.onChunkChange(player, oldChunk, newChunk)
		}
		h.handleSneakFlag(player, int32(flags))
		h.trackFall(player, oldY, float64(y), onGround)
		AddSprintExhaustion(player, float64(x)-oldX, float64(z)-oldZ)
		AddSwimExhaustion(player, float64(x)-oldX, float64(y)-oldY, float64(z)-oldZ)
		// Jump exhaustion
		if wasOnGround && !onGround && float64(y) > oldY+0.1 {
			if player.Sprinting {
				player.Exhaustion += 0.2 // sprint jump
			} else {
				player.Exhaustion += 0.05 // normal jump
			}
		}
		player.WasOnGround = wasOnGround
		h.broadcastPos(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z))
		// Check pressure plates at new position
		if h.RedstoneMgr != nil && onGround {
			h.RedstoneMgr.CheckPressurePlateAt(float64(x), float64(y), float64(z))
		}
		// Sweet berry bush damage
		h.checkSweetBerryBush(player, float64(x), float64(y), float64(z))
		// Movement enchantment effects (every ~4 ticks)
		player.EnchantMoveTick++
		if player.EnchantMoveTick%4 == 0 {
			h.checkMovementEnchantments(player, float64(x), float64(y), float64(z))
		}
		return true

	case packetid.ServerboundMovePlayerPosRot:
		var x, y, z pk.Double
		var yaw, pitch pk.Float
		var flags pk.VarInt
		if err := p.Scan(&x, &y, &z, &yaw, &pitch, &flags); err != nil {
			return true
		}
		onGround := (int32(flags) & 0x01) != 0

		// Spectators: update position/rotation and broadcast, skip survival mechanics
		if IsSpectator(player) {
			oldX, oldY, oldZ := player.Position()
			oldChunk := player.ChunkPos()
			player.SetPosition(float64(x), float64(y), float64(z))
			player.SetRotation(float32(yaw), float32(pitch))
			newChunk := player.ChunkPos()
			if oldChunk != newChunk {
				h.onChunkChange(player, oldChunk, newChunk)
			}
			h.broadcastPosRot(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z), float32(yaw), float32(pitch))
			return true
		}

		wasOnGround := player.OnGround
		player.OnGround = onGround
		if !h.validateMovement(player, float64(x), float64(y), float64(z)) {
			return true
		}
		oldX, oldY, oldZ := player.Position()
		oldChunk := player.ChunkPos()
		player.SetPosition(float64(x), float64(y), float64(z))
		player.SetRotation(float32(yaw), float32(pitch))
		newChunk := player.ChunkPos()
		if oldChunk != newChunk {
			h.onChunkChange(player, oldChunk, newChunk)
		}
		h.handleSneakFlag(player, int32(flags))
		h.trackFall(player, oldY, float64(y), onGround)
		AddSprintExhaustion(player, float64(x)-oldX, float64(z)-oldZ)
		AddSwimExhaustion(player, float64(x)-oldX, float64(y)-oldY, float64(z)-oldZ)
		// Jump exhaustion
		if wasOnGround && !onGround && float64(y) > oldY+0.1 {
			if player.Sprinting {
				player.Exhaustion += 0.2 // sprint jump
			} else {
				player.Exhaustion += 0.05 // normal jump
			}
		}
		player.WasOnGround = wasOnGround
		h.broadcastPosRot(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z), float32(yaw), float32(pitch))
		// Check pressure plates at new position
		if h.RedstoneMgr != nil && onGround {
			h.RedstoneMgr.CheckPressurePlateAt(float64(x), float64(y), float64(z))
		}
		// Sweet berry bush damage
		h.checkSweetBerryBush(player, float64(x), float64(y), float64(z))
		// Movement enchantment effects (every ~4 ticks)
		player.EnchantMoveTick++
		if player.EnchantMoveTick%4 == 0 {
			h.checkMovementEnchantments(player, float64(x), float64(y), float64(z))
		}
		return true

	case packetid.ServerboundMovePlayerRot:
		var yaw, pitch pk.Float
		var flags pk.VarInt
		if err := p.Scan(&yaw, &pitch, &flags); err != nil {
			return true
		}
		player.SetRotation(float32(yaw), float32(pitch))
		h.handleSneakFlag(player, int32(flags))
		h.broadcastRot(player, float32(yaw), float32(pitch))
		return true

	case packetid.ServerboundMovePlayerStatusOnly:
		// On-ground status only — no position/rotation update
		return true
	}

	return false
}

// worldForPlayer returns the world for the player's current dimension.
func (h *MovementHandler) worldForPlayer(player *game.Player) game.World {
	if h.DimensionMgr != nil {
		return h.DimensionMgr.WorldForPlayer(player)
	}
	return h.World
}

// encoderForPlayer returns the chunk encoder for the player's current dimension.
func (h *MovementHandler) encoderForPlayer(player *game.Player) *ChunkSender {
	if h.DimensionMgr != nil {
		return h.DimensionMgr.EncoderForPlayer(player)
	}
	return h.Encoder
}

// onChunkChange is called when a player crosses a chunk boundary.
func (h *MovementHandler) onChunkChange(player *game.Player, oldChunk, newChunk game.ChunkPos) {
	// Check for region boundary crossing
	if h.OnRegionCrossing != nil {
		oldRegion := game.ChunkToRegion(oldChunk)
		newRegion := game.ChunkToRegion(newChunk)
		if oldRegion != newRegion {
			h.OnRegionCrossing(player, newRegion)
		}
	}

	// Update chunk cache center
	if err := player.WritePacket(pk.Marshal(
		packetid.ClientboundSetChunkCacheCenter,
		pk.VarInt(newChunk.X),
		pk.VarInt(newChunk.Z),
	)); err != nil {
		return
	}

	encoder := h.encoderForPlayer(player)
	if encoder != nil {
		encoder.UpdateChunks(player)
	}

	// Update player entity visibility (spawn/despawn based on distance)
	UpdatePlayerVisibility(h.Manager, player)
}

// ChunkSender handles sending and unloading chunks for players based on view distance.
type ChunkSender struct {
	World game.World
	MinY  int
}

// SendInitialChunks sends the spawn chunks to a newly joined player.
func (cs *ChunkSender) SendInitialChunks(player *game.Player) error {
	center := player.ChunkPos()
	vd := player.ViewDistance

	// Set chunk cache center
	if err := player.WritePacket(pk.Marshal(
		packetid.ClientboundSetChunkCacheCenter,
		pk.VarInt(center.X),
		pk.VarInt(center.Z),
	)); err != nil {
		return err
	}

	// Chunk batch start
	if err := player.WritePacket(pk.Marshal(packetid.ClientboundChunkBatchStart)); err != nil {
		return err
	}

	count := 0
	for dx := -vd; dx <= vd; dx++ {
		for dz := -vd; dz <= vd; dz++ {
			pos := game.ChunkPos{X: center.X + dx, Z: center.Z + dz}
			if err := cs.sendChunk(player, pos); err != nil {
				return err
			}
			count++
		}
	}

	// Chunk batch finished
	return player.WritePacket(pk.Marshal(
		packetid.ClientboundChunkBatchFinished,
		pk.VarInt(int32(count)),
	))
}

// UpdateChunks loads new chunks and unloads old chunks after a player moves.
func (cs *ChunkSender) UpdateChunks(player *game.Player) {
	center := player.ChunkPos()
	vd := player.ViewDistance

	// Determine which chunks should be loaded
	needed := make(map[game.ChunkPos]bool)
	for dx := -vd; dx <= vd; dx++ {
		for dz := -vd; dz <= vd; dz++ {
			needed[game.ChunkPos{X: center.X + dx, Z: center.Z + dz}] = true
		}
	}

	// Unload chunks that are no longer needed
	for pos := range player.LoadedChunks {
		if !needed[pos] {
			delete(player.LoadedChunks, pos)
			// Send ForgetLevelChunk
			player.WritePacket(pk.Marshal(
				packetid.ClientboundForgetLevelChunk,
				pk.Int(int32(pos.Z)), pk.Int(int32(pos.X)), // Z first, then X
			))
		}
	}

	// Load new chunks with batch framing
	var newChunkPositions []game.ChunkPos
	for pos := range needed {
		if !player.LoadedChunks[pos] {
			newChunkPositions = append(newChunkPositions, pos)
		}
	}
	if len(newChunkPositions) > 0 {
		player.WritePacket(pk.Marshal(packetid.ClientboundChunkBatchStart))
		count := 0
		for _, pos := range newChunkPositions {
			if err := cs.sendChunk(player, pos); err != nil {
				continue
			}
			count++
		}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundChunkBatchFinished,
			pk.VarInt(int32(count)),
		))
	}
}

// broadcastPos broadcasts a position-only movement to nearby players.
// Spectator positions are only sent to other spectators.
func (h *MovementHandler) broadcastPos(player *game.Player, oldX, oldY, oldZ, newX, newY, newZ float64) {
	dx := pk.Short((newX - oldX) * 4096)
	dy := pk.Short((newY - oldY) * 4096)
	dz := pk.Short((newZ - oldZ) * 4096)

	pkt := pk.Marshal(
		packetid.ClientboundMoveEntityPos,
		pk.VarInt(player.EID),
		dx, dy, dz,
		pk.Boolean(player.OnGround),
	)
	isSpec := IsSpectator(player)
	h.Manager.ForEachNearby(newX, newZ, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			// Don't send spectator movement to non-spectators
			if isSpec && !IsSpectator(p) {
				return
			}
			p.WritePacket(pkt)
		}
	})
}

// broadcastPosRot broadcasts position+rotation movement to nearby players.
// Spectator movements are only sent to other spectators.
func (h *MovementHandler) broadcastPosRot(player *game.Player, oldX, oldY, oldZ, newX, newY, newZ float64, yaw, pitch float32) {
	dx := pk.Short((newX - oldX) * 4096)
	dy := pk.Short((newY - oldY) * 4096)
	dz := pk.Short((newZ - oldZ) * 4096)
	aYaw := pk.Angle(degToAngle(yaw))
	aPitch := pk.Angle(degToAngle(pitch))

	movPkt := pk.Marshal(
		packetid.ClientboundMoveEntityPosRot,
		pk.VarInt(player.EID),
		dx, dy, dz,
		aYaw, aPitch,
		pk.Boolean(player.OnGround),
	)
	headPkt := pk.Marshal(
		packetid.ClientboundRotateHead,
		pk.VarInt(player.EID),
		aYaw,
	)
	isSpec := IsSpectator(player)
	h.Manager.ForEachNearby(newX, newZ, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			if isSpec && !IsSpectator(p) {
				return
			}
			p.WritePacket(movPkt)
			p.WritePacket(headPkt)
		}
	})
}

// broadcastRot broadcasts rotation-only movement to nearby players.
// Spectator rotations are only sent to other spectators.
func (h *MovementHandler) broadcastRot(player *game.Player, yaw, pitch float32) {
	px, _, pz := player.Position()
	aYaw := pk.Angle(degToAngle(yaw))
	aPitch := pk.Angle(degToAngle(pitch))

	rotPkt := pk.Marshal(
		packetid.ClientboundMoveEntityRot,
		pk.VarInt(player.EID),
		aYaw, aPitch,
		pk.Boolean(player.OnGround),
	)
	headPkt := pk.Marshal(
		packetid.ClientboundRotateHead,
		pk.VarInt(player.EID),
		aYaw,
	)
	isSpec := IsSpectator(player)
	h.Manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			if isSpec && !IsSpectator(p) {
				return
			}
			p.WritePacket(rotPkt)
			p.WritePacket(headPkt)
		}
	})
}

// handleSneakFlag extracts the sneaking bit from movement flags and broadcasts if changed.
func (h *MovementHandler) handleSneakFlag(player *game.Player, flags int32) {
	sneaking := (flags & 0x02) != 0
	if sneaking != player.Sneaking {
		player.Sneaking = sneaking
		BroadcastEntityFlags(h.Manager, player)

		// Dismount vehicle when player starts sneaking
		if sneaking && player.RidingEntityEID != 0 {
			if h.MinecartMgr != nil && h.MinecartMgr.IsMinecart(player.RidingEntityEID) {
				h.MinecartMgr.DismountMinecart(player)
			} else if h.BoatMgr != nil {
				h.BoatMgr.DismountBoat(player)
			}
		}
	}
}

// trackFall tracks vertical movement for fall damage.
// Called on every position update with old Y, new Y, and the onGround flag.
func (h *MovementHandler) trackFall(player *game.Player, oldY, newY float64, onGround bool) {
	if player.Dead || player.IsInvulnerable() {
		return
	}

	// Cancel fall tracking when entering water or on a climbable block
	if player.InWater || player.OnLadder {
		player.FallStartY = -999
		return
	}

	if newY < oldY {
		// Falling — track the start of the fall
		if player.FallStartY < -900 {
			player.FallStartY = oldY
		}
	}

	// Apply fall damage when the player lands (onGround flag or Y stopped decreasing)
	if player.FallStartY > -900 && (onGround || newY >= oldY) {
		fallDist := player.FallStartY - newY
		player.FallStartY = -999
		if fallDist > 3 {
			// Slow Falling negates all fall damage
			if player.Effects != nil {
				if _, ok := player.Effects[EffectSlowFalling]; ok {
					return
				}
			}

			damage := float32(fallDist - 3)

			// Feather Falling enchantment: check boots (slot 8) for reduction.
			// Each level reduces fall damage by 12% (multiply by 1 - 0.12*level).
			bootsItem := &player.Inventory[8]
			if ffLvl := enchant.GetLevel(bootsItem.Enchantments, enchant.FeatherFalling); ffLvl > 0 {
				reduction := float64(ffLvl) * 0.12
				if reduction > 1.0 {
					reduction = 1.0
				}
				damage *= float32(1 - reduction)
			}

			if damage > 0 && h.SurvivalHandler != nil {
				player.LastDamageMessage = player.Name + " fell from a high place"
				h.SurvivalHandler.ApplyDamage(h.Manager, player, damage, h.SurvivalHandler.FallDamageTypeID)
			}
		}

		// Farmland trampling: any significant fall converts farmland to dirt
		if fallDist > 0.5 {
			bx := int(math.Floor(player.X))
			by := int(math.Floor(player.Y)) - 1
			bz := int(math.Floor(player.Z))
			w := h.worldForPlayer(player)
			if state, err := w.GetBlock(bx, by, bz); err == nil {
				blockName := BlockNameFromState(int(state))
				if blockName == "farmland" {
					dirtID, ok := block.ToStateID[block.Dirt{}]
					if ok {
						w.SetBlock(bx, by, bz, dirtID)
						broadcastBlockUpdateDirect(h.Manager, bx, by, bz, int32(dirtID))
					}
					// Break crop on top if any
					if h.CropMgr != nil {
						aboveState, err := w.GetBlock(bx, by+1, bz)
						if err == nil && isCropBlock(BlockNameFromState(int(aboveState))) {
							w.SetBlock(bx, by+1, bz, 0)
							broadcastBlockUpdateDirect(h.Manager, bx, by+1, bz, 0)
							h.CropMgr.UnregisterCrop(bx, by+1, bz)
						}
					}
				}
			}
		}
	}
}

// checkSweetBerryBush applies damage when a player walks through a mature sweet berry bush.
// In vanilla, berry bushes at age 2-3 deal 1 damage and slow movement.
// We apply 1 damage per position change while inside the bush.
func (h *MovementHandler) checkSweetBerryBush(player *game.Player, x, y, z float64) {
	if player.Dead || player.GameMode == 1 || player.GameMode == 3 { // creative/spectator immune
		return
	}

	w := h.worldForPlayer(player)
	bx := int(math.Floor(x))
	by := int(math.Floor(y))
	bz := int(math.Floor(z))

	// Check the block at the player's feet position
	for dy := 0; dy <= 1; dy++ {
		state, err := w.GetBlock(bx, by+dy, bz)
		if err != nil {
			continue
		}
		if int(state) < len(block.StateList) && block.StateList[state] != nil {
			if bush, ok := block.StateList[state].(block.SweetBerryBush); ok {
				if int(bush.Age) >= 2 && h.SurvivalHandler != nil {
					player.LastDamageMessage = player.Name + " was poked to death by a sweet berry bush"
					h.SurvivalHandler.ApplyDamage(h.Manager, player, 1.0, h.SurvivalHandler.FallDamageTypeID)
					return
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Movement Validation (Anti-Cheat)
// ---------------------------------------------------------------------------

const (
	maxWalkSpeed    = 0.32 // blocks/tick (4.317 bps * 1.5 tolerance / 20)
	maxSprintSpeed  = 0.42 // blocks/tick (5.612 bps * 1.5 tolerance / 20)
	maxVerticalUp   = 0.5  // blocks/tick (jump + tolerance)
	maxVerticalDown = 4.0  // blocks/tick (terminal velocity + tolerance)
	maxViolations   = 5    // before rubberband
	kickViolations  = 20   // before kick
	maxFlyTicks     = 80   // airborne ticks before flagging as flying
)

// validateMovement checks if a position update is physically plausible.
// Returns true if the movement should be accepted.
func (h *MovementHandler) validateMovement(player *game.Player, newX, newY, newZ float64) bool {
	// Skip validation after server-initiated teleport
	if player.TeleportPending {
		player.TeleportPending = false
		player.ViolationCount = 0
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		player.AirTicks_AC = 0
		return true
	}

	// Skip if creative or spectator — free flight allowed
	if player.GameMode == 1 || player.GameMode == 3 {
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		return true
	}

	// Skip if riding an entity
	if player.RidingEntityEID != 0 {
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		return true
	}

	// Skip if gliding with elytra
	if player.Gliding {
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		return true
	}

	oldX, oldY, oldZ := player.LastValidX, player.LastValidY, player.LastValidZ

	// First movement: no previous position to validate against
	if oldX == 0 && oldY == 0 && oldZ == 0 {
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		return true
	}

	// Horizontal speed check
	dx := newX - oldX
	dz := newZ - oldZ
	horizDist := math.Sqrt(dx*dx + dz*dz)

	maxHoriz := maxWalkSpeed
	if player.Sprinting {
		maxHoriz = maxSprintSpeed
	}

	// Speed effect multiplier
	if player.Effects != nil {
		if speed, ok := player.Effects[1]; ok && speed != nil { // 1 = Speed effect ID
			maxHoriz *= 1.0 + 0.2*float64(speed.Level)
		}
	}

	// Vertical speed check
	dy := newY - oldY
	violation := false

	if horizDist > maxHoriz {
		violation = true
	}
	if dy > maxVerticalUp {
		violation = true
	}
	if dy < -maxVerticalDown {
		violation = true
	}

	// Flight detection: track airborne ticks
	if !player.OnGround {
		player.AirTicks_AC++
		if player.AirTicks_AC > maxFlyTicks && dy >= 0 && !player.InWater && !player.OnLadder {
			violation = true
		}
	} else {
		player.AirTicks_AC = 0
	}

	if violation {
		player.ViolationCount++
		if player.ViolationCount >= kickViolations {
			h.kickPlayer(player, "Moved too quickly")
			return false
		}
		if player.ViolationCount >= maxViolations {
			h.rubberBandPlayer(player)
			return false
		}
		// Allow small violations without rubberbanding (tolerance)
		player.LastValidX = newX
		player.LastValidY = newY
		player.LastValidZ = newZ
		return true
	}

	player.ViolationCount = 0
	player.LastValidX = newX
	player.LastValidY = newY
	player.LastValidZ = newZ
	return true
}

// rubberBandPlayer sends the player back to their last valid position.
func (h *MovementHandler) rubberBandPlayer(player *game.Player) {
	x, y, z := player.LastValidX, player.LastValidY, player.LastValidZ
	player.SetPosition(x, y, z)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(100), // teleport ID
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.Double(0), // vel_x
		pk.Double(0), // vel_y
		pk.Double(0), // vel_z
		pk.Float(0),  // yaw (relative)
		pk.Float(0),  // pitch (relative)
		pk.Int(0x08|0x10), // flags: yaw+pitch relative, position absolute
	))
	player.TeleportPending = true
}

// kickPlayer disconnects a player for movement violations.
func (h *MovementHandler) kickPlayer(player *game.Player, reason string) {
	if h.Logger != nil {
		h.Logger.Printf("Kicking %s: %s (violations=%d)", player.Name, reason, player.ViolationCount)
	}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundDisconnect,
		chat.Message{Text: reason},
	))
}

// sendChunk loads a chunk from the world and sends it to the player.
func (cs *ChunkSender) sendChunk(player *game.Player, pos game.ChunkPos) error {
	chunk, err := cs.World.LoadChunk(pos)
	if err != nil {
		return err
	}

	pkt, err := game.WriteChunkPacket261(pos, chunk, cs.MinY)
	if err != nil {
		return err
	}

	if err := player.WritePacket(pkt); err != nil {
		return err
	}
	player.LoadedChunks[pos] = true
	return nil
}
