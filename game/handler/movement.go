// Package handler provides play-phase packet handlers for the game server.
package handler

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// MovementHandler processes player movement packets and triggers chunk load/unload.
type MovementHandler struct {
	World           game.World
	Manager         *game.PlayerManager
	Logger          *log.Logger
	Encoder         *ChunkSender
	SurvivalHandler *SurvivalHandler
}

// HandlePacket processes a single packet for the given player.
// Returns true if the packet was handled.
func (h *MovementHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundMovePlayerPos:
		var x, y, z pk.Double
		var flags pk.VarInt
		if err := p.Scan(&x, &y, &z, &flags); err != nil {
			return true
		}
		onGround := (int32(flags) & 0x01) != 0
		player.OnGround = onGround
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
		h.broadcastPos(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z))
		return true

	case packetid.ServerboundMovePlayerPosRot:
		var x, y, z pk.Double
		var yaw, pitch pk.Float
		var flags pk.VarInt
		if err := p.Scan(&x, &y, &z, &yaw, &pitch, &flags); err != nil {
			return true
		}
		onGround := (int32(flags) & 0x01) != 0
		player.OnGround = onGround
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
		h.broadcastPosRot(player, oldX, oldY, oldZ, float64(x), float64(y), float64(z), float32(yaw), float32(pitch))
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

// onChunkChange is called when a player crosses a chunk boundary.
func (h *MovementHandler) onChunkChange(player *game.Player, oldChunk, newChunk game.ChunkPos) {
	// Update chunk cache center
	if err := player.WritePacket(pk.Marshal(
		packetid.ClientboundSetChunkCacheCenter,
		pk.VarInt(newChunk.X),
		pk.VarInt(newChunk.Z),
	)); err != nil {
		return
	}

	if h.Encoder != nil {
		h.Encoder.UpdateChunks(player)
	}
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

	// Load new chunks
	newChunks := 0
	for pos := range needed {
		if !player.LoadedChunks[pos] {
			if err := cs.sendChunk(player, pos); err != nil {
				continue
			}
			newChunks++
		}
	}
}

// broadcastPos broadcasts a position-only movement to all other players.
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
	h.Manager.ForEach(func(p *game.Player) {
		if p.UUID != player.UUID {
			p.WritePacket(pkt)
		}
	})
}

// broadcastPosRot broadcasts position+rotation movement to all other players.
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
	h.Manager.ForEach(func(p *game.Player) {
		if p.UUID != player.UUID {
			p.WritePacket(movPkt)
			p.WritePacket(headPkt)
		}
	})
}

// broadcastRot broadcasts rotation-only movement to all other players.
func (h *MovementHandler) broadcastRot(player *game.Player, yaw, pitch float32) {
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
	h.Manager.ForEach(func(p *game.Player) {
		if p.UUID != player.UUID {
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
	}
}

// trackFall tracks vertical movement for fall damage.
// Called on every position update with old Y, new Y, and the onGround flag.
func (h *MovementHandler) trackFall(player *game.Player, oldY, newY float64, onGround bool) {
	if player.Dead || player.GameMode == 1 { // no fall damage in creative
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
			damage := float32(fallDist - 3)
			if h.SurvivalHandler != nil {
				h.SurvivalHandler.ApplyDamage(h.Manager, player, damage, h.SurvivalHandler.FallDamageTypeID)
			}
		}
	}
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
