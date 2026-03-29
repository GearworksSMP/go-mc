package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// TNT entity type ID (26.1-snapshot-2 registry).
const tntEntityType int32 = 132

// Sound ID for entity.tnt.primed (from data/soundid/soundid.go: 1022).
const SoundTNTPrimed int32 = 1022

// PrimedTNT represents a primed (ignited) TNT entity in the world.
type PrimedTNT struct {
	EID            int32
	X, Y, Z        float64
	VelX, VelY, VelZ float64
	FuseTicks      int64 // counts down from 80 (4 seconds)
}

// TNTManager manages primed TNT entities.
type TNTManager struct {
	Manager      *game.PlayerManager
	World        game.World
	Survival     *SurvivalHandler
	ItemEntities *ItemEntityManager
	Logger       *log.Logger
	mu           sync.Mutex
	tnts         map[int32]*PrimedTNT
}

// NewTNTManager creates a new TNTManager.
func NewTNTManager(manager *game.PlayerManager, world game.World, survival *SurvivalHandler, itemEntities *ItemEntityManager, logger *log.Logger) *TNTManager {
	return &TNTManager{
		Manager:      manager,
		World:        world,
		Survival:     survival,
		ItemEntities: itemEntities,
		Logger:       logger,
		tnts:         make(map[int32]*PrimedTNT),
	}
}

// Ignite removes a TNT block at the given position and spawns a primed TNT entity.
func (tm *TNTManager) Ignite(x, y, z int) {
	// Remove TNT block from world (set to air)
	oldState, err := tm.World.SetBlock(x, y, z, 0)
	if err != nil {
		tm.logf("TNT: error removing block at (%d,%d,%d): %v", x, y, z, err)
		return
	}

	// Broadcast block change to air
	broadcastBlockUpdateDirect(tm.Manager, x, y, z, 0)

	// Block break particles if there was a block
	if oldState > 0 {
		BroadcastLevelEvent(tm.Manager, 2001, x, y, z, int32(oldState))
	}

	// Spawn PrimedTNT entity at block center
	spawnX := float64(x) + 0.5
	spawnY := float64(y)
	spawnZ := float64(z) + 0.5

	eid := tm.Manager.NextEntityID()
	tnt := &PrimedTNT{
		EID:       eid,
		X:         spawnX,
		Y:         spawnY,
		Z:         spawnZ,
		VelY:      0.2, // initial upward velocity
		FuseTicks: 80,  // 4 seconds at 20 TPS
	}

	tm.mu.Lock()
	tm.tnts[eid] = tnt
	tm.mu.Unlock()

	// Broadcast AddEntity for primed TNT
	entityUUID := uuid.New()
	addPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(tntEntityType),
		pk.Double(spawnX),
		pk.Double(spawnY),
		pk.Double(spawnZ),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	tm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(addPkt)
	})

	// Send metadata with fuse timer (index 8, serializer 1 = VarInt, value = 80)
	fuseMetadata := buildTNTFuseMetadata(80)
	tm.Manager.ForEach(func(p *game.Player) {
		SendEntityMetadata(p, eid, fuseMetadata)
	})

	// Play TNT prime sound
	BroadcastSound(tm.Manager, SoundTNTPrimed, SoundCategoryBlock, spawnX, spawnY, spawnZ, 1.0, 1.0)

	tm.logf("TNT ignited at (%d, %d, %d) EID=%d", x, y, z, eid)
}

// IgniteFromExplosion ignites TNT at the given block position with horizontal velocity
// away from the explosion center, simulating blast propulsion.
func (tm *TNTManager) IgniteFromExplosion(x, y, z int, expCenterX, expCenterZ float64) {
	// Calculate outward horizontal velocity from explosion center
	dx := float64(x) + 0.5 - expCenterX
	dz := float64(z) + 0.5 - expCenterZ
	dist := math.Sqrt(dx*dx + dz*dz)

	var velX, velZ float64
	if dist > 0.01 {
		velX = dx / dist * 0.5
		velZ = dz / dist * 0.5
	}

	// Remove TNT block from world
	oldState, err := tm.World.SetBlock(x, y, z, 0)
	if err != nil {
		return
	}
	broadcastBlockUpdateDirect(tm.Manager, x, y, z, 0)
	if oldState > 0 {
		BroadcastLevelEvent(tm.Manager, 2001, x, y, z, int32(oldState))
	}

	spawnX := float64(x) + 0.5
	spawnY := float64(y)
	spawnZ := float64(z) + 0.5

	eid := tm.Manager.NextEntityID()
	tnt := &PrimedTNT{
		EID:       eid,
		X:         spawnX,
		Y:         spawnY,
		Z:         spawnZ,
		VelX:      velX,
		VelY:      0.2 + rand.Float64()*0.2, // slight random upward
		VelZ:      velZ,
		FuseTicks: 10 + rand.Int63n(20), // short random fuse (0.5-1.5s)
	}

	tm.mu.Lock()
	tm.tnts[eid] = tnt
	tm.mu.Unlock()

	entityUUID := uuid.New()
	addPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(tntEntityType),
		pk.Double(spawnX),
		pk.Double(spawnY),
		pk.Double(spawnZ),
		pk.UnsignedByte(0),
		pk.Angle(0), pk.Angle(0), pk.Angle(0),
		pk.VarInt(0),
	)
	tm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(addPkt)
	})

	fuseMetadata := buildTNTFuseMetadata(int32(tnt.FuseTicks))
	tm.Manager.ForEach(func(p *game.Player) {
		SendEntityMetadata(p, eid, fuseMetadata)
	})

	BroadcastSound(tm.Manager, SoundTNTPrimed, SoundCategoryBlock, spawnX, spawnY, spawnZ, 1.0, 1.0)
}

// Tick processes all primed TNT entities: gravity, fuse countdown, explosion.
func (tm *TNTManager) Tick(tick int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var toExplode []*PrimedTNT
	var toRemove []int32

	for eid, tnt := range tm.tnts {
		// Apply gravity and horizontal movement
		tnt.VelY -= 0.04
		tnt.X += tnt.VelX
		tnt.Y += tnt.VelY
		tnt.Z += tnt.VelZ
		tnt.VelX *= 0.98 // horizontal friction
		tnt.VelZ *= 0.98

		// Simple ground collision: don't fall below the block below
		groundY := math.Floor(tnt.Y)
		bx := int(math.Floor(tnt.X))
		by := int(groundY) - 1
		bz := int(math.Floor(tnt.Z))
		if by >= -64 {
			state, err := tm.World.GetBlock(bx, by, bz)
			if err == nil && state != 0 {
				// There's a solid block below, stop falling
				if tnt.Y < groundY+1 {
					tnt.Y = groundY + 1
					tnt.VelY = 0
				}
			}
		}

		// Decrement fuse
		tnt.FuseTicks--

		if tnt.FuseTicks <= 0 {
			toExplode = append(toExplode, tnt)
			toRemove = append(toRemove, eid)
			continue
		}

		// Broadcast position update
		tm.broadcastTNTMove(tnt)
	}

	// Process explosions (must happen while still holding the lock so chain reactions work)
	for _, tnt := range toExplode {
		tm.explode(tnt)
	}

	// Remove exploded TNTs
	for _, eid := range toRemove {
		tm.removeTNT(eid)
	}
}

// explode handles the explosion of a primed TNT.
func (tm *TNTManager) explode(tnt *PrimedTNT) {
	cx, cy, cz := tnt.X, tnt.Y+0.5, tnt.Z // explosion center (slightly above entity pos)
	const blockRadius = 4
	const entityRadius = 7.0

	// Play explosion sound
	BroadcastSound(tm.Manager, SoundExplode, SoundCategoryBlock, cx, cy, cz, 4.0, 1.0)

	// Send explosion particles via LevelEvent
	// Event 2003 = end dragon death (creates particles), but we'll use individual block breaks
	// Instead, just break blocks and let the block-break particles handle visual effects.

	// Block destruction: sphere of radius 4
	for dx := -blockRadius; dx <= blockRadius; dx++ {
		for dy := -blockRadius; dy <= blockRadius; dy++ {
			for dz := -blockRadius; dz <= blockRadius; dz++ {
				dist := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
				if dist > float64(blockRadius) {
					continue
				}

				bx := int(math.Floor(cx)) + dx
				by := int(math.Floor(cy)) + dy
				bz := int(math.Floor(cz)) + dz

				state, err := tm.World.GetBlock(bx, by, bz)
				if err != nil || state == 0 {
					continue // air or error
				}

				blockName := BlockNameFromState(int(state))
				if blockName == "" {
					continue
				}

				// Skip blast-resistant blocks
				if isBlastResistant(blockName) {
					continue
				}

				// Destruction chance decreases with distance
				destructionChance := 1.0 - (dist / float64(blockRadius+1))
				if rand.Float64() > destructionChance {
					continue
				}

				// Check for chain reaction: TNT blocks in range get ignited with blast velocity
				if blockName == "tnt" {
					go func(bx, by, bz int, ecx, ecz float64) {
						tm.IgniteFromExplosion(bx, by, bz, ecx, ecz)
					}(bx, by, bz, cx, cz)
					continue
				}

				// Set block to air
				tm.World.SetBlock(bx, by, bz, 0)
				broadcastBlockUpdateDirect(tm.Manager, bx, by, bz, 0)

				// Block break particles
				BroadcastLevelEvent(tm.Manager, 2001, bx, by, bz, int32(state))

				// 33% chance to spawn item drop for the destroyed block
				if rand.Float64() < 0.33 && tm.ItemEntities != nil {
					dropName, drops := GetBlockDropItemName(blockName)
					if drops && dropName != "" {
						dropID := itemIDByName(dropName)
						if dropID > 0 {
							tm.ItemEntities.SpawnItem(tm.Manager,
								float64(bx)+0.5, float64(by)+0.5, float64(bz)+0.5,
								dropID, 1, 20)
						}
					}
				}
			}
		}
	}

	// Entity damage: players within 7-block radius
	tm.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}

		px, py, pz := p.Position()
		dx := px - cx
		dy := (py + 0.9) - cy // center of player
		dz := pz - cz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist > entityRadius {
			return
		}

		// Damage = max(1, (1 - distance/7) * 40) -- up to 40 damage at center
		damage := float32(math.Max(1, (1-dist/entityRadius)*40))

		// Apply blast protection enchantment
		blastProtTotal := int32(0)
		for _, slot := range []int{5, 6, 7, 8} {
			blastProtTotal += enchant.GetLevel(p.Inventory[slot].Enchantments, enchant.BlastProtection)
		}
		if blastProtTotal > 0 {
			reduction := float32(blastProtTotal) * 0.08
			if reduction > 0.8 {
				reduction = 0.8
			}
			damage *= (1 - reduction)
		}

		p.LastDamageMessage = p.Name + " was blown up"
		if tm.Survival != nil {
			tm.Survival.ApplyDamage(tm.Manager, p, damage, tm.Survival.AttackDamageTypeID)
		}

		// Knockback: push away from explosion center
		if dist > 0.01 {
			scale := 6000.0 * (1 - dist/entityRadius) / dist
			velX := int16(dx * scale)
			velY := int16(3000 * (1 - dist/entityRadius))
			velZ := int16(dz * scale)

			p.WritePacket(pk.Marshal(
				packetid.ClientboundSetEntityMotion,
				pk.VarInt(p.EID),
				pk.Short(velX),
				pk.Short(velY),
				pk.Short(velZ),
			))
		}
	})
}

// broadcastTNTMove sends a teleport update for a primed TNT entity.
func (tm *TNTManager) broadcastTNTMove(tnt *PrimedTNT) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(tnt.EID),
		pk.Double(tnt.X),
		pk.Double(tnt.Y),
		pk.Double(tnt.Z),
		pk.Double(tnt.VelX), pk.Double(tnt.VelY), pk.Double(tnt.VelZ), // velocity
		pk.Float(0), pk.Float(0), // yaw, pitch
		pk.Int(0), // relative flags (all absolute)
		pk.Boolean(false), // on ground
	)
	tm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// removeTNT removes a primed TNT entity and broadcasts removal.
func (tm *TNTManager) removeTNT(eid int32) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	tm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
	delete(tm.tnts, eid)
}

// SendExistingTNTs sends all currently primed TNT entities to a newly joined player.
func (tm *TNTManager) SendExistingTNTs(player *game.Player) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, tnt := range tm.tnts {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(tnt.EID),
			pk.UUID(entityUUID),
			pk.VarInt(tntEntityType),
			pk.Double(tnt.X),
			pk.Double(tnt.Y),
			pk.Double(tnt.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(0),
		))
		fuseMetadata := buildTNTFuseMetadata(int32(tnt.FuseTicks))
		SendEntityMetadata(player, tnt.EID, fuseMetadata)
	}
}

// buildTNTFuseMetadata creates entity metadata bytes with the fuse timer at index 8.
// For PrimedTNT, index 8 is the fuse ticks (VarInt serializer = 1).
func buildTNTFuseMetadata(fuseTicks int32) []byte {
	var w MetadataWriter
	// Index 8, serializer 1 = VarInt
	w.writeIndex(8, 1)
	writeVarIntBuf(&w.buf, fuseTicks)
	return w.Bytes()
}

// isBlastResistant returns true for blocks that cannot be destroyed by explosions.
func isBlastResistant(blockName string) bool {
	switch blockName {
	case "bedrock", "obsidian", "crying_obsidian", "barrier",
		"end_portal_frame", "end_portal", "command_block",
		"chain_command_block", "repeating_command_block",
		"structure_block", "jigsaw", "reinforced_deepslate",
		"nether_portal":
		return true
	}
	return false
}

// isFlammable returns true for blocks that can be burned by fire.
func isFlammable(blockName string) bool {
	switch blockName {
	case "oak_planks", "spruce_planks", "birch_planks", "jungle_planks",
		"acacia_planks", "dark_oak_planks", "cherry_planks", "mangrove_planks",
		"bamboo_planks",
		"oak_log", "spruce_log", "birch_log", "jungle_log",
		"acacia_log", "dark_oak_log", "cherry_log", "mangrove_log",
		"oak_leaves", "spruce_leaves", "birch_leaves", "jungle_leaves",
		"acacia_leaves", "dark_oak_leaves", "cherry_leaves", "mangrove_leaves",
		"bookshelf",
		"oak_fence", "spruce_fence", "birch_fence", "jungle_fence",
		"acacia_fence", "dark_oak_fence", "cherry_fence", "mangrove_fence",
		"bamboo_fence",
		"oak_fence_gate", "spruce_fence_gate", "birch_fence_gate", "jungle_fence_gate",
		"acacia_fence_gate", "dark_oak_fence_gate", "cherry_fence_gate", "mangrove_fence_gate",
		"bamboo_fence_gate",
		"oak_stairs", "spruce_stairs", "birch_stairs", "jungle_stairs",
		"acacia_stairs", "dark_oak_stairs", "cherry_stairs", "mangrove_stairs",
		"bamboo_stairs",
		"white_wool", "orange_wool", "magenta_wool", "light_blue_wool",
		"yellow_wool", "lime_wool", "pink_wool", "gray_wool",
		"light_gray_wool", "cyan_wool", "purple_wool", "blue_wool",
		"brown_wool", "green_wool", "red_wool", "black_wool",
		"white_carpet", "orange_carpet", "magenta_carpet", "light_blue_carpet",
		"yellow_carpet", "lime_carpet", "pink_carpet", "gray_carpet",
		"light_gray_carpet", "cyan_carpet", "purple_carpet", "blue_carpet",
		"brown_carpet", "green_carpet", "red_carpet", "black_carpet",
		"hay_block", "bamboo", "tnt":
		return true
	}
	return false
}

func (tm *TNTManager) logf(format string, args ...any) {
	if tm.Logger != nil {
		tm.Logger.Printf(format, args...)
	}
}
