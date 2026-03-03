package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Fishing bobber entity type ID (26.1-snapshot-2 registry).
const fishingBobberEntityType int32 = 156

// Fishing sound IDs (from data/soundid — 0-indexed).
const (
	SoundFishingRetrieve int32 = 368
	SoundFishingSplash   int32 = 369
	SoundFishingThrow    int32 = 370
)

// Fishing rod max durability (vanilla value).
const fishingRodMaxDurability int32 = 64

// FishingBobber represents an active fishing bobber entity.
type FishingBobber struct {
	EID                  int32
	OwnerEID             int32
	X, Y, Z              float64
	VelX, VelY, VelZ     float64
	InWater              bool
	WaitTicks            int64 // ticks until bite
	BiteTicks            int64 // ticks of bite window remaining
	Biting               bool
	LifeTick             int64
	LureLevel            int32 // Lure enchantment level on the rod
	LuckOfTheSeaLevel    int32 // Luck of the Sea enchantment level
}

// FishingManager manages all active fishing bobbers.
type FishingManager struct {
	Manager      *game.PlayerManager
	ItemEntities *ItemEntityManager
	World        game.World
	Logger       *log.Logger
	mu           sync.Mutex
	bobbers      map[int32]*FishingBobber // EID -> bobber
}

// NewFishingManager creates a new FishingManager.
func NewFishingManager(manager *game.PlayerManager, itemEntities *ItemEntityManager, world game.World, logger *log.Logger) *FishingManager {
	return &FishingManager{
		Manager:      manager,
		ItemEntities: itemEntities,
		World:        world,
		Logger:       logger,
		bobbers:      make(map[int32]*FishingBobber),
	}
}

// CastRod handles a player using a fishing rod.
// If the player already has a bobber out, it reels in instead.
func (fm *FishingManager) CastRod(player *game.Player) {
	if player.FishingBobberEID != 0 {
		fm.ReelIn(player)
		return
	}

	// Get rod enchantments from the held item
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	var lureLevel, luckLevel int32
	if invItem.Enchantments != nil {
		lureLevel = invItem.Enchantments["lure"]
		luckLevel = invItem.Enchantments["luck_of_the_sea"]
	}

	// Calculate cast direction from player yaw/pitch
	px, py, pz := player.Position()
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0

	// Eye position (1.62 above feet)
	eyeY := py + 1.62

	// Direction vector
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	// Initial velocity: direction * 1.5 with slight upward arc
	speed := 1.5
	velX := dirX * speed
	velY := dirY*speed + 0.2 // slight upward arc
	velZ := dirZ * speed

	eid := fm.Manager.NextEntityID()
	bobber := &FishingBobber{
		EID:               eid,
		OwnerEID:          player.EID,
		X:                 px,
		Y:                 eyeY,
		Z:                 pz,
		VelX:              velX,
		VelY:              velY,
		VelZ:              velZ,
		LureLevel:         lureLevel,
		LuckOfTheSeaLevel: luckLevel,
	}

	fm.mu.Lock()
	fm.bobbers[eid] = bobber
	fm.mu.Unlock()

	player.FishingBobberEID = eid

	// Broadcast entity spawn to all players
	entityUUID := uuid.New()
	data := player.EID + 1 // data field = owner entity ID + 1
	spawnPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(fishingBobberEntityType),
		pk.Double(bobber.X),
		pk.Double(bobber.Y),
		pk.Double(bobber.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity (movement handled by teleport)
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(data),
	)
	fm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(spawnPkt)
	})

	// Play cast sound
	BroadcastSound(fm.Manager, SoundFishingThrow, SoundCategoryPlayer, px, eyeY, pz, 1.0, 1.0)

	fm.logf("Player %s cast fishing rod (bobber EID=%d)", player.Name, eid)
}

// ReelIn handles reeling in a fishing bobber.
func (fm *FishingManager) ReelIn(player *game.Player) {
	eid := player.FishingBobberEID
	if eid == 0 {
		return
	}

	fm.mu.Lock()
	bobber, ok := fm.bobbers[eid]
	if ok {
		delete(fm.bobbers, eid)
	}
	fm.mu.Unlock()

	// Always clear the player's bobber reference
	player.FishingBobberEID = 0

	if !ok {
		return
	}

	px, py, pz := player.Position()

	// If the bobber was biting, award loot
	if bobber.Biting {
		lootItemID, lootCount := fm.rollLoot(bobber.LuckOfTheSeaLevel)
		if lootItemID > 0 {
			// Spawn item entity near the player (slightly above to fly toward them)
			fm.ItemEntities.SpawnItem(fm.Manager, px, py+1, pz, lootItemID, lootCount, 0)

			// Award 1-6 XP
			xp := int32(1 + rand.Intn(6))
			AddExperience(player, xp)

			// Reduce rod durability
			fm.reduceRodDurability(player)

			lootName := ItemNameByID(lootItemID)
			fm.logf("Player %s caught %s (xp=%d)", player.Name, lootName, xp)
		}
	}

	// Play retrieve sound
	BroadcastSound(fm.Manager, SoundFishingRetrieve, SoundCategoryNeutral, bobber.X, bobber.Y, bobber.Z, 1.0, 1.0)

	// Remove bobber entity
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	fm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
}

// rollLoot determines what the player catches.
// Returns (itemID, count).
func (fm *FishingManager) rollLoot(luckOfTheSea int32) (int32, int32) {
	// Base chances: 85% fish, 10% junk, 5% treasure
	// Luck of the Sea: +1% treasure per level, -1% junk per level
	treasureChance := 5 + int(luckOfTheSea)
	junkChance := 10 - int(luckOfTheSea)
	if junkChance < 1 {
		junkChance = 1
	}
	fishChance := 100 - treasureChance - junkChance

	roll := rand.Intn(100)

	if roll < fishChance {
		return fm.rollFish()
	} else if roll < fishChance+junkChance {
		return fm.rollJunk()
	}
	return fm.rollTreasure()
}

// rollFish returns a random fish item.
// Cod 60%, Salmon 25%, Tropical Fish 13%, Pufferfish 2%.
func (fm *FishingManager) rollFish() (int32, int32) {
	roll := rand.Intn(100)
	switch {
	case roll < 60:
		return itemIDByName("cod"), 1
	case roll < 85:
		return itemIDByName("salmon"), 1
	case roll < 98:
		return itemIDByName("tropical_fish"), 1
	default:
		return itemIDByName("pufferfish"), 1
	}
}

// rollJunk returns a random junk item.
func (fm *FishingManager) rollJunk() (int32, int32) {
	junkItems := []string{
		"leather_boots",
		"stick",
		"string",
		"bowl",
		"fishing_rod",
	}
	choice := junkItems[rand.Intn(len(junkItems))]
	return itemIDByName(choice), 1
}

// rollTreasure returns a random treasure item.
func (fm *FishingManager) rollTreasure() (int32, int32) {
	treasureItems := []string{
		"bow",
		"enchanted_book",
		"name_tag",
		"saddle",
		"lily_pad",
	}
	choice := treasureItems[rand.Intn(len(treasureItems))]
	return itemIDByName(choice), 1
}

// reduceRodDurability reduces the fishing rod's durability by 1.
func (fm *FishingManager) reduceRodDurability(player *game.Player) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return
	}
	name := ItemNameByID(invItem.ID)
	if name != "fishing_rod" {
		return
	}

	// Check unbreaking enchantment: skip with probability level/(level+1)
	unbreakLvl := int32(0)
	if invItem.Enchantments != nil {
		unbreakLvl = invItem.Enchantments["unbreaking"]
	}
	if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
		return // unbreaking saved this durability point
	}

	invItem.Durability--
	if invItem.Durability <= 0 {
		*invItem = game.ItemStack{} // rod breaks
	}
	SendSlotUpdate(player, slot)
}

// isWaterBlock checks if the block at the given world coordinates is water.
func (fm *FishingManager) isWaterBlock(x, y, z int) bool {
	state, err := fm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	return name == "water"
}

// Tick processes all fishing bobbers (movement, water detection, bite mechanics).
func (fm *FishingManager) Tick(tick int64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	var toRemove []int32

	for eid, bobber := range fm.bobbers {
		bobber.LifeTick++

		// Auto-remove after 60 seconds (1200 ticks)
		if bobber.LifeTick > 1200 {
			toRemove = append(toRemove, eid)
			fm.clearPlayerBobber(bobber.OwnerEID)
			continue
		}

		// Check if the owning player is still online
		owner := fm.Manager.GetByEID(bobber.OwnerEID)
		if owner == nil {
			toRemove = append(toRemove, eid)
			continue
		}

		bx := int(math.Floor(bobber.X))
		by := int(math.Floor(bobber.Y))
		bz := int(math.Floor(bobber.Z))

		if !bobber.InWater {
			// Flying through the air: apply gravity and movement
			bobber.VelY -= 0.03
			bobber.X += bobber.VelX
			bobber.Y += bobber.VelY
			bobber.Z += bobber.VelZ

			// Air drag
			bobber.VelX *= 0.92
			bobber.VelY *= 0.92
			bobber.VelZ *= 0.92

			// Recalculate position block
			bx = int(math.Floor(bobber.X))
			by = int(math.Floor(bobber.Y))
			bz = int(math.Floor(bobber.Z))

			// Check if entered water
			if fm.isWaterBlock(bx, by, bz) {
				bobber.InWater = true
				bobber.VelX = 0
				bobber.VelY *= 0.5
				bobber.VelZ = 0

				// Set wait time: 100 + rand(400) = 5-25 seconds
				waitTime := int64(100 + rand.Intn(400))

				// Lure enchantment: reduce wait time by 100 ticks per level
				waitTime -= int64(bobber.LureLevel) * 100
				if waitTime < 20 {
					waitTime = 20 // minimum 1 second
				}
				bobber.WaitTicks = waitTime
			} else {
				// Check for solid block collision (stuck)
				state, err := fm.World.GetBlock(bx, by, bz)
				if err == nil && state != 0 {
					blockName := BlockNameFromState(int(state))
					// Don't count air or plants as collision
					if blockName != "" && blockName != "air" && blockName != "cave_air" &&
						blockName != "short_grass" && blockName != "tall_grass" {
						// Bobber hit a solid block, remove it
						toRemove = append(toRemove, eid)
						fm.clearPlayerBobber(bobber.OwnerEID)
						continue
					}
				}
			}
		} else {
			// Bobber is in water
			if bobber.Biting {
				// Bite window active
				bobber.BiteTicks--
				if bobber.BiteTicks <= 0 {
					// Bite window expired; reset waiting
					bobber.Biting = false
					waitTime := int64(100 + rand.Intn(400))
					waitTime -= int64(bobber.LureLevel) * 100
					if waitTime < 20 {
						waitTime = 20
					}
					bobber.WaitTicks = waitTime
				}

				// During bite: bobber dips down slightly
				// Find water surface and dip 0.2 blocks below
				surfaceY := fm.findWaterSurface(bx, by, bz)
				bobber.Y = surfaceY - 0.2
				bobber.VelX = 0
				bobber.VelY = 0
				bobber.VelZ = 0
			} else {
				// Waiting for a bite
				bobber.WaitTicks--

				// Float at water surface
				surfaceY := fm.findWaterSurface(bx, by, bz)
				bobber.Y = surfaceY
				bobber.VelX = 0
				bobber.VelY = 0
				bobber.VelZ = 0

				if bobber.WaitTicks <= 0 {
					// Start bite
					bobber.Biting = true
					bobber.BiteTicks = 20 // 1 second bite window

					// Play splash sound
					BroadcastSound(fm.Manager, SoundFishingSplash, SoundCategoryNeutral,
						bobber.X, bobber.Y, bobber.Z, 1.0, 1.0)
				}
			}
		}

		// Broadcast position update
		fm.broadcastBobberMove(bobber)
	}

	// Remove expired/invalid bobbers
	for _, eid := range toRemove {
		fm.removeBobber(eid)
	}
}

// findWaterSurface returns the Y coordinate of the water surface at (bx, bz).
func (fm *FishingManager) findWaterSurface(bx, by, bz int) float64 {
	// Scan upward from current position to find the top of the water
	y := by
	for {
		if !fm.isWaterBlock(bx, y+1, bz) {
			// y+1 is not water, so y is the top water block
			return float64(y) + 0.85 // float just below the top of the block
		}
		y++
		if y > by+10 {
			break // safety limit
		}
	}
	return float64(by) + 0.85
}

// broadcastBobberMove sends a teleport update for a bobber to all players.
func (fm *FishingManager) broadcastBobberMove(bobber *FishingBobber) {
	pkt := pk.Marshal(
		packetid.ClientboundTeleportEntity,
		pk.VarInt(bobber.EID),
		pk.Double(bobber.X),
		pk.Double(bobber.Y),
		pk.Double(bobber.Z),
		pk.Double(bobber.VelX), pk.Double(bobber.VelY), pk.Double(bobber.VelZ),
		pk.Float(0), pk.Float(0),
		pk.Int(0), // relative flags (all absolute)
		pk.Boolean(false),
	)
	fm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// removeBobber removes a bobber entity and broadcasts removal.
func (fm *FishingManager) removeBobber(eid int32) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	fm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
	delete(fm.bobbers, eid)
}

// clearPlayerBobber clears the FishingBobberEID for the player with the given EID.
func (fm *FishingManager) clearPlayerBobber(ownerEID int32) {
	owner := fm.Manager.GetByEID(ownerEID)
	if owner != nil {
		owner.FishingBobberEID = 0
	}
}

// SendExistingBobbers sends all current fishing bobbers to a newly joined player.
func (fm *FishingManager) SendExistingBobbers(player *game.Player) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for _, bobber := range fm.bobbers {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(bobber.EID),
			pk.UUID(entityUUID),
			pk.VarInt(fishingBobberEntityType),
			pk.Double(bobber.X),
			pk.Double(bobber.Y),
			pk.Double(bobber.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(bobber.OwnerEID+1),
		))
	}
}

// CleanupPlayer removes any bobbers belonging to a disconnecting player.
func (fm *FishingManager) CleanupPlayer(player *game.Player) {
	if player.FishingBobberEID == 0 {
		return
	}

	eid := player.FishingBobberEID
	player.FishingBobberEID = 0

	fm.mu.Lock()
	_, ok := fm.bobbers[eid]
	if ok {
		delete(fm.bobbers, eid)
	}
	fm.mu.Unlock()

	if ok {
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(eid),
		)
		fm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(removePkt)
		})
	}
}

func (fm *FishingManager) logf(format string, args ...any) {
	if fm.Logger != nil {
		fm.Logger.Printf(format, args...)
	}
}
