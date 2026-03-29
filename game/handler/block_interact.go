package handler

import (
	"log"
	"math"
	"math/rand"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Sound IDs for block interactions.
const (
	SoundBellUse       int32 = 117 // block.bell.use
	SoundBellResonate  int32 = 118 // block.bell.resonate
	SoundComposterFill        int32 = 210 // block.composter.fill
	SoundComposterFillSuccess int32 = 211 // block.composter.fill_success
	SoundComposterReady       int32 = 212 // block.composter.ready
	SoundComposterEmpty       int32 = 209 // block.composter.empty

	SoundBucketEmpty          int32 = 149 // item.bucket.empty
	SoundBucketEmptyLava      int32 = 152 // item.bucket.empty_lava
	SoundBucketEmptyPowderSnow int32 = 153 // item.bucket.empty_powder_snow
	SoundBucketFill           int32 = 154 // item.bucket.fill
	SoundBucketFillLava       int32 = 157 // item.bucket.fill_lava
	SoundBucketFillPowderSnow int32 = 158 // item.bucket.fill_powder_snow
	SoundExtinguishFire       int32 = 400 // entity.generic.extinguish_fire
)

// ---------------------------------------------------------------------------
// Bell
// ---------------------------------------------------------------------------

// RingBell plays the bell sound and animation.
func RingBell(manager *game.PlayerManager, x, y, z int) {
	BroadcastSound(manager, SoundBellUse, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 2.0, 1.0)
	BroadcastSound(manager, SoundBellResonate, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 2.0, 1.0)
}

// ---------------------------------------------------------------------------
// Composter
// ---------------------------------------------------------------------------

// ComposterManager tracks composter fill levels.
type ComposterManager struct {
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex
}

// NewComposterManager creates a new ComposterManager.
func NewComposterManager(manager *game.PlayerManager, world game.World, logger *log.Logger) *ComposterManager {
	return &ComposterManager{
		Manager: manager,
		World:   world,
		Logger:  logger,
	}
}

// compostChance returns the composting success chance for an item name.
// Returns 0 if the item is not compostable.
func compostChance(itemName string) int {
	switch itemName {
	// 30% chance
	case "beetroot_seeds", "dried_kelp", "short_grass", "hanging_roots",
		"kelp", "melon_seeds", "pumpkin_seeds", "seagrass",
		"sweet_berries", "glow_berries", "wheat_seeds",
		"moss_carpet", "pink_petals", "small_dripleaf",
		"torchflower_seeds", "pitcher_pod":
		return 30
	// 50% chance
	case "cactus", "dried_kelp_block", "flowering_azalea_leaves",
		"glow_lichen", "melon_slice", "nether_sprouts",
		"sugar_cane", "tall_grass", "twisting_vines",
		"vine", "weeping_vines":
		return 50
	// 65% chance
	case "apple", "azalea", "beetroot", "big_dripleaf",
		"carrot", "cocoa_beans", "fern", "large_fern",
		"lily_pad", "melon", "moss_block", "brown_mushroom",
		"red_mushroom", "mushroom_stem", "nether_wart",
		"potato", "pumpkin", "carved_pumpkin",
		"sea_pickle", "shroomlight", "spore_blossom",
		"wheat", "flowering_azalea":
		return 65
	// 85% chance
	case "baked_potato", "bread", "cookie",
		"hay_block", "brown_mushroom_block", "red_mushroom_block",
		"nether_wart_block", "warped_wart_block":
		return 85
	// 100% chance
	case "cake", "pumpkin_pie":
		return 100
	}
	// Flowers, saplings, and leaves at 65%
	if strings.HasSuffix(itemName, "_sapling") || strings.HasSuffix(itemName, "_leaves") {
		return 65
	}
	// All flowers (tulips, roses, etc.)
	for _, flower := range []string{
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet",
		"red_tulip", "orange_tulip", "white_tulip", "pink_tulip",
		"oxeye_daisy", "cornflower", "lily_of_the_valley", "sunflower",
		"lilac", "rose_bush", "peony", "wither_rose", "torchflower",
		"pitcher_plant",
	} {
		if itemName == flower {
			return 65
		}
	}
	// Seeds at 30%
	if strings.HasSuffix(itemName, "_seeds") {
		return 30
	}
	return 0
}

// UseComposter handles right-clicking a composter.
// Returns true if the interaction was handled.
func (cm *ComposterManager) UseComposter(player *game.Player, x, y, z int) bool {
	stateID, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	comp, ok := block.StateList[stateID].(block.Composter)
	if !ok {
		return false
	}

	level := int(comp.Level)

	// Level 8 = ready to harvest → give bone_meal
	if level == 8 {
		comp.Level = 0
		if newID, ok2 := block.ToStateID[comp]; ok2 {
			cm.World.SetBlock(x, y, z, newID)
			broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
		}
		BroadcastSound(cm.Manager, SoundComposterEmpty, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
		// Give bone_meal to player
		boneMealID := itemIDByName("bone_meal")
		if boneMealID > 0 {
			slot := player.Inventory.AddItem(boneMealID, 1)
			if slot >= 0 {
				SendSlotUpdate(player, slot)
			}
		}
		return true
	}

	// Level 0-6: try to add compostable item
	if level >= 7 {
		return false // full but not ready yet, wait
	}

	// Check held item
	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]
	if held.ID <= 0 || held.Count <= 0 {
		return false
	}

	itemName := ItemNameByID(held.ID)
	chance := compostChance(itemName)
	if chance == 0 {
		return false
	}

	// Consume one item
	held.Count--
	if held.Count <= 0 {
		*held = game.ItemStack{}
	}
	SendSlotUpdate(player, heldSlot)

	// Random chance to increase level
	if rand.Intn(100) < chance {
		comp.Level = block.Integer(level + 1)
		if int(comp.Level) >= 7 {
			comp.Level = 8 // jump to ready state
			BroadcastSound(cm.Manager, SoundComposterReady, SoundCategoryBlock,
				float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
		} else {
			BroadcastSound(cm.Manager, SoundComposterFillSuccess, SoundCategoryBlock,
				float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
		}
	} else {
		BroadcastSound(cm.Manager, SoundComposterFill, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	}

	if newID, ok2 := block.ToStateID[comp]; ok2 {
		cm.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
	}
	return true
}

// ---------------------------------------------------------------------------
// Cauldron
// ---------------------------------------------------------------------------

// CauldronManager handles cauldron interactions.
type CauldronManager struct {
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex
}

// NewCauldronManager creates a new CauldronManager.
func NewCauldronManager(manager *game.PlayerManager, world game.World, logger *log.Logger) *CauldronManager {
	return &CauldronManager{
		Manager: manager,
		World:   world,
		Logger:  logger,
	}
}

// UseCauldron handles right-clicking a cauldron, water_cauldron, lava_cauldron, or powder_snow_cauldron.
// Returns true if the interaction was handled.
func (cm *CauldronManager) UseCauldron(player *game.Player, x, y, z int) bool {
	stateID, err := cm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	cx, cy, cz := float64(x)+0.5, float64(y)+0.5, float64(z)+0.5
	heldSlot := int(player.HeldSlot) + 36
	held := &player.Inventory[heldSlot]

	// Fire extinguishing: player on fire + right-click water cauldron (no item required)
	if b, ok := block.StateList[stateID].(block.WaterCauldron); ok && player.FireTicks > 0 {
		level := int(b.Level)
		if level >= 1 {
			player.FireTicks = 0
			cm.decrementWaterLevel(x, y, z, level)
			BroadcastSound(cm.Manager, SoundExtinguishFire, SoundCategoryPlayer, cx, cy, cz, 1.0, 1.0)
			return true
		}
	}

	if held.ID <= 0 || held.Count <= 0 {
		return false
	}
	heldName := ItemNameByID(held.ID)

	switch b := block.StateList[stateID].(type) {
	case block.Cauldron:
		switch heldName {
		case "water_bucket":
			cm.fillCauldron(player, x, y, z, heldSlot, block.WaterCauldron{Level: 3})
			BroadcastSound(cm.Manager, SoundBucketEmpty, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		case "lava_bucket":
			cm.fillCauldron(player, x, y, z, heldSlot, block.LavaCauldron{})
			BroadcastSound(cm.Manager, SoundBucketEmptyLava, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		case "powder_snow_bucket":
			cm.fillCauldron(player, x, y, z, heldSlot, block.PowderSnowCauldron{Level: 3})
			BroadcastSound(cm.Manager, SoundBucketEmptyPowderSnow, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		}

	case block.WaterCauldron:
		level := int(b.Level)
		switch heldName {
		case "bucket":
			if level == 3 {
				cm.drainCauldron(player, x, y, z, heldSlot, "water_bucket")
				BroadcastSound(cm.Manager, SoundBucketFill, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
				return true
			}
		case "water_bucket":
			b.Level = 3
			if newID, ok := block.ToStateID[b]; ok {
				cm.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
			}
			*held = game.ItemStack{ID: itemIDByName("bucket"), Count: 1}
			SendSlotUpdate(player, heldSlot)
			BroadcastSound(cm.Manager, SoundBucketEmpty, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		case "glass_bottle":
			if level >= 1 {
				cm.decrementWaterLevel(x, y, z, level)
				*held = game.ItemStack{ID: itemIDByName("potion"), Count: 1}
				SendSlotUpdate(player, heldSlot)
				return true
			}
		case "leather_helmet", "leather_chestplate", "leather_leggings", "leather_boots":
			// Leather armor dyeing: wash dyed leather armor in water cauldron
			if level >= 1 {
				cm.decrementWaterLevel(x, y, z, level)
				BroadcastSound(cm.Manager, SoundBucketEmpty, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
				return true
			}
		}

	case block.LavaCauldron:
		if heldName == "bucket" {
			cm.drainCauldron(player, x, y, z, heldSlot, "lava_bucket")
			BroadcastSound(cm.Manager, SoundBucketFillLava, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		}

	case block.PowderSnowCauldron:
		level := int(b.Level)
		switch heldName {
		case "bucket":
			if level == 3 {
				cm.drainCauldron(player, x, y, z, heldSlot, "powder_snow_bucket")
				BroadcastSound(cm.Manager, SoundBucketFillPowderSnow, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
				return true
			}
		case "powder_snow_bucket":
			b.Level = 3
			if newID, ok := block.ToStateID[b]; ok {
				cm.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
			}
			*held = game.ItemStack{ID: itemIDByName("bucket"), Count: 1}
			SendSlotUpdate(player, heldSlot)
			BroadcastSound(cm.Manager, SoundBucketEmptyPowderSnow, SoundCategoryBlock, cx, cy, cz, 1.0, 1.0)
			return true
		}
	}

	return false
}

// fillCauldron sets a cauldron block and replaces the held bucket with an empty one.
func (cm *CauldronManager) fillCauldron(player *game.Player, x, y, z, heldSlot int, newBlock block.Block) bool {
	newID, ok := block.ToStateID[newBlock]
	if !ok {
		return false
	}
	cm.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
	player.Inventory[heldSlot] = game.ItemStack{ID: itemIDByName("bucket"), Count: 1}
	SendSlotUpdate(player, heldSlot)
	return true
}

// drainCauldron empties a cauldron and gives the player a filled bucket.
func (cm *CauldronManager) drainCauldron(player *game.Player, x, y, z, heldSlot int, filledBucket string) bool {
	newID, ok := block.ToStateID[block.Cauldron{}]
	if !ok {
		return false
	}
	cm.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
	player.Inventory[heldSlot] = game.ItemStack{ID: itemIDByName(filledBucket), Count: 1}
	SendSlotUpdate(player, heldSlot)
	return true
}

// decrementWaterLevel reduces a water cauldron's level by 1, emptying it if it reaches 0.
func (cm *CauldronManager) decrementWaterLevel(x, y, z, currentLevel int) {
	newLevel := currentLevel - 1
	if newLevel <= 0 {
		if newID, ok := block.ToStateID[block.Cauldron{}]; ok {
			cm.World.SetBlock(x, y, z, newID)
			broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
		}
	} else {
		if newID, ok := block.ToStateID[block.WaterCauldron{Level: block.Integer(newLevel)}]; ok {
			cm.World.SetBlock(x, y, z, newID)
			broadcastBlockUpdateDirect(cm.Manager, x, y, z, int32(newID))
		}
	}
}

// ---------------------------------------------------------------------------
// Beacon
// ---------------------------------------------------------------------------

// BeaconEffectConfig stores the player-selected effects for a beacon.
type BeaconEffectConfig struct {
	PrimaryEffect   int32 // effect ID (0 = none)
	SecondaryEffect int32 // effect ID (0 = none, only valid at tier 4)
}

// BeaconWindowID is the container window ID for the beacon UI.
const BeaconWindowID = 23

// BeaconManager handles beacon pyramid detection and effect application.
type BeaconManager struct {
	Manager   *game.PlayerManager
	World     game.World
	EffectMgr *EffectManager
	Logger    *log.Logger
	mu        sync.Mutex
	beacons   map[[3]int]int              // position → pyramid tier (0 = invalid)
	effects   map[[3]int]*BeaconEffectConfig // position → configured effects
}

// NewBeaconManager creates a new BeaconManager.
func NewBeaconManager(manager *game.PlayerManager, world game.World, effectMgr *EffectManager, logger *log.Logger) *BeaconManager {
	return &BeaconManager{
		Manager:   manager,
		World:     world,
		EffectMgr: effectMgr,
		Logger:    logger,
		beacons:   make(map[[3]int]int),
		effects:   make(map[[3]int]*BeaconEffectConfig),
	}
}

// TrackBeacon registers a beacon position for periodic updates.
func (bm *BeaconManager) TrackBeacon(x, y, z int) {
	bm.mu.Lock()
	bm.beacons[[3]int{x, y, z}] = 0
	bm.mu.Unlock()
}

// UntrackBeacon removes a beacon from tracking.
func (bm *BeaconManager) UntrackBeacon(x, y, z int) {
	bm.mu.Lock()
	delete(bm.beacons, [3]int{x, y, z})
	bm.mu.Unlock()
}

// isBeaconBaseBlock returns true if the block name is a valid beacon pyramid material.
func isBeaconBaseBlock(name string) bool {
	switch name {
	case "iron_block", "gold_block", "diamond_block", "emerald_block", "netherite_block":
		return true
	}
	return false
}

// checkPyramid returns the pyramid tier (0-4) for a beacon at (bx, by, bz).
func (bm *BeaconManager) checkPyramid(bx, by, bz int) int {
	tier := 0
	for level := 1; level <= 4; level++ {
		checkY := by - level
		size := level // number of blocks from center edge
		valid := true
		for dx := -size; dx <= size; dx++ {
			for dz := -size; dz <= size; dz++ {
				stateID, err := bm.World.GetBlock(bx+dx, checkY, bz+dz)
				if err != nil {
					valid = false
					break
				}
				name := BlockNameFromState(int(stateID))
				if !isBeaconBaseBlock(name) {
					valid = false
					break
				}
			}
			if !valid {
				break
			}
		}
		if !valid {
			break
		}
		tier = level
	}
	return tier
}

// beaconRange returns the effect range for a given pyramid tier.
func beaconRange(tier int) float64 {
	switch tier {
	case 1:
		return 20
	case 2:
		return 30
	case 3:
		return 40
	case 4:
		return 50
	}
	return 0
}

// beaconEffectID returns the primary effect for a beacon tier.
// Tier 1: Speed (1), Tier 2: Haste (3), Tier 3: Resistance (11), Tier 4: + Regeneration (10)
func beaconEffectID(tier int) int32 {
	switch tier {
	case 1:
		return 1 // Speed
	case 2:
		return 3 // Haste
	case 3:
		return 11 // Resistance
	case 4:
		return 1 // Speed (with Regeneration as secondary)
	}
	return 0
}

// Tick processes beacon effects periodically. Call every 80 ticks (4 seconds).
func (bm *BeaconManager) Tick(tick int64) {
	if tick%80 != 0 {
		return
	}
	if bm.EffectMgr == nil {
		return
	}

	bm.mu.Lock()
	beacons := make(map[[3]int]int, len(bm.beacons))
	for pos := range bm.beacons {
		beacons[pos] = 0
	}
	bm.mu.Unlock()

	// Update tiers and apply effects
	for pos := range beacons {
		// Verify beacon block still exists
		stateID, err := bm.World.GetBlock(pos[0], pos[1], pos[2])
		if err != nil {
			bm.mu.Lock()
			delete(bm.beacons, pos)
			bm.mu.Unlock()
			continue
		}
		name := BlockNameFromState(int(stateID))
		if name != "beacon" {
			bm.mu.Lock()
			delete(bm.beacons, pos)
			bm.mu.Unlock()
			continue
		}

		tier := bm.checkPyramid(pos[0], pos[1], pos[2])
		bm.mu.Lock()
		bm.beacons[pos] = tier
		bm.mu.Unlock()

		if tier == 0 {
			continue
		}

		// Apply effects to nearby players
		effectRange := beaconRange(tier)

		bm.mu.Lock()
		cfg := bm.effects[pos]
		bm.mu.Unlock()

		// Use configured effects if set, otherwise fall back to tier default
		primaryEffect := beaconEffectID(tier)
		var secondaryEffect int32
		var primaryLevel int32
		if cfg != nil && cfg.PrimaryEffect != 0 && isValidBeaconEffect(cfg.PrimaryEffect, tier) {
			primaryEffect = cfg.PrimaryEffect
			if cfg.SecondaryEffect != 0 && tier >= 4 {
				if cfg.SecondaryEffect == cfg.PrimaryEffect {
					primaryLevel = 1 // amplify to level II
				} else {
					secondaryEffect = cfg.SecondaryEffect
				}
			}
		} else if tier >= 4 {
			secondaryEffect = 10 // default: Regeneration
		}

		if primaryEffect == 0 {
			continue
		}

		bm.Manager.ForEach(func(p *game.Player) {
			if p.Dead {
				return
			}
			px, py, pz := p.Position()
			dx := px - float64(pos[0])
			dy := py - float64(pos[1])
			dz := pz - float64(pos[2])
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist <= effectRange {
				bm.EffectMgr.ApplyEffect(p, primaryEffect, primaryLevel, 180, true)
				if secondaryEffect != 0 {
					bm.EffectMgr.ApplyEffect(p, secondaryEffect, 0, 180, true)
				}
			}
		})
	}
}

// OpenBeaconUI opens the beacon configuration screen for a player.
func (bm *BeaconManager) OpenBeaconUI(player *game.Player, x, y, z int) {
	tier := bm.checkPyramid(x, y, z)

	player.OpenWindowID = BeaconWindowID
	player.OpenBeaconPos = [3]int{x, y, z}

	// Send open screen (menu type 8 = beacon)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(BeaconWindowID),
		pk.VarInt(8),
		chat.Text("Beacon"),
	))

	// Send beacon properties
	bm.mu.Lock()
	cfg := bm.effects[[3]int{x, y, z}]
	bm.mu.Unlock()

	var primary, secondary int32
	if cfg != nil {
		primary = cfg.PrimaryEffect
		secondary = cfg.SecondaryEffect
	}

	// Property 0: power level (tier)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.UnsignedByte(BeaconWindowID),
		pk.Short(0),
		pk.Short(int16(tier)),
	))
	// Property 1: primary effect
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.UnsignedByte(BeaconWindowID),
		pk.Short(1),
		pk.Short(int16(primary)),
	))
	// Property 2: secondary effect
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.UnsignedByte(BeaconWindowID),
		pk.Short(2),
		pk.Short(int16(secondary)),
	))
}

// HandleBeaconUpdate processes the ServerboundSetBeacon packet.
func (bm *BeaconManager) HandleBeaconUpdate(player *game.Player, p pk.Packet) {
	if player.OpenWindowID != BeaconWindowID {
		return
	}

	var optPrimary, optSecondary pk.Option[pk.VarInt, *pk.VarInt]
	if err := p.Scan(&optPrimary, &optSecondary); err != nil {
		return
	}

	pos := player.OpenBeaconPos
	tier := bm.checkPyramid(pos[0], pos[1], pos[2])
	if tier == 0 {
		return
	}

	var primary, secondary int32
	if optPrimary.Has {
		primary = int32(optPrimary.Val)
	}
	if optSecondary.Has {
		secondary = int32(optSecondary.Val)
	}

	// Validate effects against tier
	if primary != 0 && !isValidBeaconEffect(primary, tier) {
		return
	}
	if secondary != 0 && (tier < 4 || !isValidBeaconSecondary(secondary)) {
		return
	}

	// Consume payment item (iron/gold/diamond/emerald/netherite ingot)
	slot := int(player.HeldSlot) + 36
	held := &player.Inventory[slot]
	if !isBeaconPayment(ItemNameByID(held.ID)) {
		// Check all hotbar slots for a valid payment
		found := false
		for i := 36; i <= 44; i++ {
			if isBeaconPayment(ItemNameByID(player.Inventory[i].ID)) && player.Inventory[i].Count > 0 {
				slot = i
				held = &player.Inventory[i]
				found = true
				break
			}
		}
		if !found {
			return
		}
	}
	held.Count--
	if held.Count <= 0 {
		*held = game.ItemStack{}
	}
	SendSlotUpdate(player, slot)

	// Store effect config
	bm.mu.Lock()
	bm.effects[pos] = &BeaconEffectConfig{
		PrimaryEffect:   primary,
		SecondaryEffect: secondary,
	}
	bm.mu.Unlock()

	// Play beacon activation sound
	BroadcastSound(bm.Manager, 69, SoundCategoryBlock,
		float64(pos[0])+0.5, float64(pos[1])+0.5, float64(pos[2])+0.5, 1.0, 1.0)
}

// isValidBeaconEffect checks if an effect is valid for the given pyramid tier.
func isValidBeaconEffect(effectID int32, tier int) bool {
	switch effectID {
	case 1, 3: // Speed, Haste — tier 1+
		return tier >= 1
	case 11, 8: // Resistance, Jump Boost — tier 2+
		return tier >= 2
	case 5: // Strength — tier 3+
		return tier >= 3
	}
	return false
}

// isValidBeaconSecondary checks if an effect is valid as a secondary beacon effect (tier 4).
func isValidBeaconSecondary(effectID int32) bool {
	switch effectID {
	case 10: // Regeneration
		return true
	case 1, 3, 11, 8, 5: // Same as primary (amplify to II)
		return true
	}
	return false
}

// isBeaconPayment returns true if the item name is a valid beacon payment.
func isBeaconPayment(name string) bool {
	switch name {
	case "iron_ingot", "gold_ingot", "diamond", "emerald", "netherite_ingot":
		return true
	}
	return false
}
