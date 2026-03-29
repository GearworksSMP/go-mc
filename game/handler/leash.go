package handler

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// EntityTypeLeashKnot is the entity type ID for leash_knot (26.1-snapshot-2 registry, 0-indexed).
const EntityTypeLeashKnot int32 = 68

// Leash sound event IDs.
const (
	SoundLeashKnotBreak int32 = 778 // entity.leash_knot.break (line 784 - 6)
	SoundLeashKnotPlace int32 = 779 // entity.leash_knot.place (line 785 - 6)
)

// LeashKnot represents a leash knot entity on a fence post.
type LeashKnot struct {
	EID      int32
	UUID     uuid.UUID
	X, Y, Z  int // block position of the fence post
}

// LeashManager manages leash/lead mechanics.
type LeashManager struct {
	mu      sync.Mutex
	Manager *game.PlayerManager
	MobMgr  *MobManager
	ItemMgr *ItemEntityManager
	Logger  *log.Logger

	knots        map[int32]*LeashKnot          // keyed by EID
	playerLeash  map[uuid.UUID]map[int32]bool  // player UUID -> set of mob EIDs the player is leashing
	mobLeashKnot map[int32]int32               // mob EID -> leash knot EID (for mobs tied to posts)
}

// NewLeashManager creates a new LeashManager.
func NewLeashManager(manager *game.PlayerManager, mobMgr *MobManager, itemMgr *ItemEntityManager, logger *log.Logger) *LeashManager {
	return &LeashManager{
		Manager:      manager,
		MobMgr:       mobMgr,
		ItemMgr:      itemMgr,
		Logger:       logger,
		knots:        make(map[int32]*LeashKnot),
		playerLeash:  make(map[uuid.UUID]map[int32]bool),
		mobLeashKnot: make(map[int32]int32),
	}
}

// AttachLeash attaches a lead from a player to a mob.
func (lm *LeashManager) AttachLeash(player *game.Player, mobEID int32) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Check mob exists
	if !lm.MobMgr.IsMob(mobEID) {
		return
	}

	// Don't leash if already leashed by this player
	if leashes, ok := lm.playerLeash[player.UUID]; ok {
		if leashes[mobEID] {
			return
		}
	}

	// Add to player's leash set
	if lm.playerLeash[player.UUID] == nil {
		lm.playerLeash[player.UUID] = make(map[int32]bool)
	}
	lm.playerLeash[player.UUID][mobEID] = true

	// Remove from knot leash if it was tied to a post
	delete(lm.mobLeashKnot, mobEID)

	// Send entity link: mob attached to player
	lm.broadcastEntityLink(mobEID, player.EID)

	// Consume lead item in survival
	if player.GameMode == 0 {
		heldSlot := int(player.HeldSlot) + 36
		player.Inventory[heldSlot].Count--
		if player.Inventory[heldSlot].Count <= 0 {
			player.Inventory[heldSlot] = game.ItemStack{}
		}
		SendSlotUpdate(player, heldSlot)
	}

	lm.logf("Player %s leashed mob EID=%d", player.Name, mobEID)
}

// HandleKnotInteract handles right-click on a leash knot.
// Attaches all leashed mobs from this player to the knot.
func (lm *LeashManager) HandleKnotInteract(player *game.Player, knotEID int32) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	knot, ok := lm.knots[knotEID]
	if !ok {
		return
	}

	leashes := lm.playerLeash[player.UUID]
	if len(leashes) == 0 {
		return
	}

	// Transfer all player-held leashes to this knot
	for mobEID := range leashes {
		lm.mobLeashKnot[mobEID] = knotEID
		lm.broadcastEntityLink(mobEID, knot.EID)
	}
	delete(lm.playerLeash, player.UUID)

	lm.logf("Player %s tied %d mobs to leash knot EID=%d", player.Name, len(leashes), knotEID)
}

// TieToFencePost creates a leash knot at a fence post and ties all player-held mobs to it.
// Called when right-clicking a fence while holding leashed mobs.
func (lm *LeashManager) TieToFencePost(player *game.Player, x, y, z int) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	leashes := lm.playerLeash[player.UUID]
	if len(leashes) == 0 {
		return false
	}

	// Check if knot already exists at this position
	var knot *LeashKnot
	for _, k := range lm.knots {
		if k.X == x && k.Y == y && k.Z == z {
			knot = k
			break
		}
	}

	// Create knot if needed
	if knot == nil {
		eid := lm.Manager.NextEntityID()
		knot = &LeashKnot{
			EID:  eid,
			UUID: uuid.New(),
			X:    x,
			Y:    y,
			Z:    z,
		}
		lm.knots[eid] = knot
		lm.broadcastKnotSpawn(knot)
		BroadcastSound(lm.Manager, SoundLeashKnotPlace, SoundCategoryNeutral,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	}

	// Transfer all player-held leashes to this knot
	for mobEID := range leashes {
		lm.mobLeashKnot[mobEID] = knot.EID
		lm.broadcastEntityLink(mobEID, knot.EID)
	}
	delete(lm.playerLeash, player.UUID)

	lm.logf("Player %s tied %d mobs to fence post at (%d,%d,%d)", player.Name, len(leashes), x, y, z)
	return true
}

// IsLeashKnot returns true if the entity ID is a leash knot.
func (lm *LeashManager) IsLeashKnot(eid int32) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	_, ok := lm.knots[eid]
	return ok
}

// BreakKnot destroys a leash knot, releasing all attached mobs and dropping leads.
func (lm *LeashManager) BreakKnot(knotEID int32) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	knot, ok := lm.knots[knotEID]
	if !ok {
		return
	}
	delete(lm.knots, knotEID)

	// Release all mobs tied to this knot and drop leads
	dropCount := int32(0)
	for mobEID, kEID := range lm.mobLeashKnot {
		if kEID == knotEID {
			delete(lm.mobLeashKnot, mobEID)
			lm.broadcastEntityLink(mobEID, -1) // detach
			dropCount++
		}
	}

	fx := float64(knot.X) + 0.5
	fy := float64(knot.Y) + 0.5
	fz := float64(knot.Z) + 0.5

	// Drop lead items
	if lm.ItemMgr != nil && dropCount > 0 {
		leadID := itemIDByName("lead")
		if leadID > 0 {
			lm.ItemMgr.SpawnItem(lm.Manager, fx, fy, fz, leadID, dropCount, 10)
		}
	}

	BroadcastSound(lm.Manager, SoundLeashKnotBreak, SoundCategoryNeutral, fx, fy, fz, 1.0, 1.0)

	// Remove entity
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(knotEID),
	)
	lm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})

	lm.logf("Leash knot broken EID=%d, released %d mobs", knotEID, dropCount)
}

// Tick updates leash mechanics. Called every server tick.
func (lm *LeashManager) Tick(tick int64) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Check player-held leashes: mobs follow player, break if too far
	for playerUUID, leashes := range lm.playerLeash {
		var player *game.Player
		lm.Manager.ForEach(func(p *game.Player) {
			if p.UUID == playerUUID {
				player = p
			}
		})
		if player == nil || player.Dead {
			// Player disconnected or dead — drop all leads
			for mobEID := range leashes {
				lm.broadcastEntityLink(mobEID, -1)
			}
			delete(lm.playerLeash, playerUUID)
			continue
		}

		px, py, pz := player.Position()

		var toRemove []int32
		for mobEID := range leashes {
			lm.MobMgr.mu.Lock()
			mob, ok := lm.MobMgr.Mobs[mobEID]
			if !ok {
				lm.MobMgr.mu.Unlock()
				toRemove = append(toRemove, mobEID)
				continue
			}

			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			// Break leash if too far (>10 blocks)
			if dist > 10 {
				lm.MobMgr.mu.Unlock()
				toRemove = append(toRemove, mobEID)
				lm.broadcastEntityLink(mobEID, -1)
				// Drop lead item
				if lm.ItemMgr != nil {
					leadID := itemIDByName("lead")
					if leadID > 0 {
						lm.ItemMgr.SpawnItem(lm.Manager, mob.X, mob.Y+1, mob.Z, leadID, 1, 10)
					}
				}
				continue
			}

			// Pull mob toward player if > 5 blocks away
			if dist > 5 {
				speed := mob.Speed
				if speed <= 0 {
					speed = 0.15
				}
				mob.X += dx / dist * speed
				mob.Y += dy / dist * speed
				mob.Z += dz / dist * speed
			}
			lm.MobMgr.mu.Unlock()
		}

		for _, eid := range toRemove {
			delete(leashes, eid)
		}
		if len(leashes) == 0 {
			delete(lm.playerLeash, playerUUID)
		}
	}

	// Mobs tied to knots: keep within range of knot
	for mobEID, knotEID := range lm.mobLeashKnot {
		knot, ok := lm.knots[knotEID]
		if !ok {
			delete(lm.mobLeashKnot, mobEID)
			lm.broadcastEntityLink(mobEID, -1)
			continue
		}

		lm.MobMgr.mu.Lock()
		mob, ok := lm.MobMgr.Mobs[mobEID]
		if !ok {
			lm.MobMgr.mu.Unlock()
			delete(lm.mobLeashKnot, mobEID)
			continue
		}

		kx := float64(knot.X) + 0.5
		ky := float64(knot.Y) + 0.5
		kz := float64(knot.Z) + 0.5

		dx := kx - mob.X
		dy := ky - mob.Y
		dz := kz - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		// Pull back if too far from post
		if dist > 5 {
			speed := mob.Speed
			if speed <= 0 {
				speed = 0.15
			}
			mob.X += dx / dist * speed
			mob.Y += dy / dist * speed
			mob.Z += dz / dist * speed
		}
		lm.MobMgr.mu.Unlock()
	}
}

// PlayerHasLeashes returns true if the player is holding any leashed mobs.
func (lm *LeashManager) PlayerHasLeashes(playerUUID uuid.UUID) bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return len(lm.playerLeash[playerUUID]) > 0
}

// OnPlayerDisconnect releases all leashes held by a disconnecting player.
func (lm *LeashManager) OnPlayerDisconnect(playerUUID uuid.UUID) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	leashes := lm.playerLeash[playerUUID]
	for mobEID := range leashes {
		lm.broadcastEntityLink(mobEID, -1)
	}
	delete(lm.playerLeash, playerUUID)
}

// SendExistingKnots sends all leash knots and links to a newly joined player.
func (lm *LeashManager) SendExistingKnots(player *game.Player) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, knot := range lm.knots {
		lm.sendKnotSpawnTo(player, knot)
	}

	// Send entity links for mobs tied to knots
	for mobEID, knotEID := range lm.mobLeashKnot {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSetEntityLink,
			pk.Int(mobEID),
			pk.Int(knotEID),
		))
	}
}

// IsFenceBlock returns true if the block name is a fence (not a fence gate).
func IsFenceBlock(name string) bool {
	if strings.Contains(name, "fence") && !strings.Contains(name, "gate") {
		return true
	}
	return false
}

func (lm *LeashManager) broadcastEntityLink(entityEID int32, holderEID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundSetEntityLink,
		pk.Int(entityEID),
		pk.Int(holderEID),
	)
	lm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (lm *LeashManager) broadcastKnotSpawn(knot *LeashKnot) {
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(knot.EID),
		pk.UUID(knot.UUID),
		pk.VarInt(EntityTypeLeashKnot),
		pk.Double(float64(knot.X)+0.5),
		pk.Double(float64(knot.Y)+0.25),
		pk.Double(float64(knot.Z)+0.5),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.VarInt(0),
	)
	lm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (lm *LeashManager) sendKnotSpawnTo(player *game.Player, knot *LeashKnot) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(knot.EID),
		pk.UUID(knot.UUID),
		pk.VarInt(EntityTypeLeashKnot),
		pk.Double(float64(knot.X)+0.5),
		pk.Double(float64(knot.Y)+0.25),
		pk.Double(float64(knot.Z)+0.5),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.VarInt(0),
	))
}

func (lm *LeashManager) logf(format string, args ...any) {
	if lm.Logger != nil {
		lm.Logger.Printf(format, args...)
	}
}
