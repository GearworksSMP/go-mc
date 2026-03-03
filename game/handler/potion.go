package handler

import (
	"log"
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Sound IDs for potions.
const (
	SoundPotionDrink       int32 = 342 // entity.generic.drink
	SoundSplashPotionBreak int32 = 997 // entity.splash_potion.break
	SoundSplashPotionThrow int32 = 998 // entity.splash_potion.throw
)

// Splash potion entity type ID (26.1-snapshot-2 registry).
const splashPotionEntityType int32 = 105

// PotionEffect describes the effect a potion applies.
type PotionEffect struct {
	EffectID int32
	Level    int32 // 0-indexed
	Duration int32 // ticks
}

// potionTypeEffects maps potion type strings to effects.
// Used when throwing splash potions or drinking potions with a type qualifier.
var potionTypeEffects = map[string]PotionEffect{
	"healing":         {EffectInstantHealth, 0, 1},
	"strong_healing":  {EffectInstantHealth, 1, 1},
	"harming":         {EffectInstantDamage, 0, 1},
	"strong_harming":  {EffectInstantDamage, 1, 1},
	"regeneration":    {EffectRegeneration, 0, 900},
	"long_regeneration": {EffectRegeneration, 0, 1800},
	"strong_regeneration": {EffectRegeneration, 1, 440},
	"strength":        {EffectStrength, 0, 3600},
	"long_strength":   {EffectStrength, 0, 9600},
	"strong_strength": {EffectStrength, 1, 1800},
	"speed":           {EffectSpeed, 0, 3600},
	"long_speed":      {EffectSpeed, 0, 9600},
	"strong_speed":    {EffectSpeed, 1, 1800},
	"poison":          {EffectPoison, 0, 900},
	"long_poison":     {EffectPoison, 0, 1800},
	"strong_poison":   {EffectPoison, 1, 440},
	"fire_resistance": {EffectFireResistance, 0, 3600},
	"long_fire_resistance": {EffectFireResistance, 0, 9600},
	"night_vision":    {EffectNightVision, 0, 3600},
	"long_night_vision": {EffectNightVision, 0, 9600},
	"water_breathing": {EffectWaterBreathing, 0, 3600},
	"long_water_breathing": {EffectWaterBreathing, 0, 9600},
	"invisibility":    {EffectInvisibility, 0, 3600},
	"long_invisibility": {EffectInvisibility, 0, 9600},
	"weakness":        {EffectWeakness, 0, 1800},
	"long_weakness":   {EffectWeakness, 0, 4800},
	"slow_falling":    {EffectSlowFalling, 0, 1800},
	"long_slow_falling": {EffectSlowFalling, 0, 4800},
	"resistance":      {EffectResistance, 0, 3600},
	"absorption":      {EffectAbsorption, 0, 2400},
}

// SplashPotion represents a flying splash potion projectile.
type SplashPotion struct {
	EID                  int32
	ThrowerEID           int32
	X, Y, Z              float64
	VelX, VelY, VelZ     float64
	PotionType           string
	LifeTick             int64
}

// PotionManager manages potion drinking and splash potion projectiles.
type PotionManager struct {
	Manager   *game.PlayerManager
	EffectMgr *EffectManager
	Survival  *SurvivalHandler
	Logger    *log.Logger
	mu        sync.Mutex
	potions   map[int32]*SplashPotion
}

// NewPotionManager creates a new PotionManager.
func NewPotionManager(manager *game.PlayerManager, effectMgr *EffectManager, survival *SurvivalHandler, logger *log.Logger) *PotionManager {
	return &PotionManager{
		Manager:   manager,
		EffectMgr: effectMgr,
		Survival:  survival,
		Logger:    logger,
		potions:   make(map[int32]*SplashPotion),
	}
}

// HandleDrinkPotion processes a player drinking a potion.
// itemName is the item name (e.g. "potion", "milk_bucket").
// Returns true if the item was handled as a potion/milk.
func (pm *PotionManager) HandleDrinkPotion(player *game.Player, itemName string) bool {
	// Milk bucket: clear all effects
	if itemName == "milk_bucket" {
		pm.EffectMgr.ClearAllEffects(player)
		// Replace milk bucket with empty bucket in the held slot
		slot := int(player.HeldSlot) + 36
		bucketID := itemIDByName("bucket")
		if bucketID > 0 {
			player.Inventory[slot] = game.ItemStack{ID: bucketID, Count: 1}
		} else {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
		// Play drink sound
		px, py, pz := player.Position()
		BroadcastSound(pm.Manager, SoundPotionDrink, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
		pm.logf("Player %s drank milk (cleared all effects)", player.Name)
		return true
	}

	if itemName != "potion" {
		return false
	}

	// For MVP, since we don't have potion NBT parsing, apply a basic healing effect.
	// In a full implementation, the potion type would be read from the item's
	// potion_contents component.
	pe := PotionEffect{EffectInstantHealth, 0, 1}
	pm.EffectMgr.ApplyEffect(player, pe.EffectID, pe.Level, pe.Duration, false)

	// Replace potion with glass bottle
	slot := int(player.HeldSlot) + 36
	bottleID := itemIDByName("glass_bottle")
	if bottleID > 0 {
		player.Inventory[slot] = game.ItemStack{ID: bottleID, Count: 1}
	} else {
		player.Inventory[slot] = game.ItemStack{}
	}
	SendSlotUpdate(player, slot)

	// Play drink sound
	px, py, pz := player.Position()
	BroadcastSound(pm.Manager, SoundPotionDrink, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	pm.logf("Player %s drank a potion", player.Name)
	return true
}

// ThrowSplashPotion creates a splash potion projectile in the player's look direction.
func (pm *PotionManager) ThrowSplashPotion(player *game.Player, potionType string) {
	px, py, pz := player.Position()
	yaw, pitch := player.Rotation()

	// Calculate throw direction from yaw/pitch
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	speed := 0.5
	velX := dirX * speed
	velY := dirY*speed + 0.2 // slight upward arc
	velZ := dirZ * speed

	eid := pm.Manager.NextEntityID()
	potion := &SplashPotion{
		EID:        eid,
		ThrowerEID: player.EID,
		X:          px,
		Y:          py + 1.5, // throw from eye height
		Z:          pz,
		VelX:       velX,
		VelY:       velY,
		VelZ:       velZ,
		PotionType: potionType,
	}

	pm.mu.Lock()
	pm.potions[eid] = potion
	pm.mu.Unlock()

	// Broadcast spawn entity
	entityUUID := uuid.New()
	data := player.EID + 1 // data = thrower entity ID + 1
	spawnPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(splashPotionEntityType),
		pk.Double(potion.X),
		pk.Double(potion.Y),
		pk.Double(potion.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(data),
	)
	pm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(spawnPkt)
	})

	// Consume the splash potion from inventory
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	invItem.Count--
	if invItem.Count <= 0 {
		*invItem = game.ItemStack{}
	}
	SendSlotUpdate(player, slot)

	// Play throw sound
	BroadcastSound(pm.Manager, SoundSplashPotionThrow, SoundCategoryNeutral, px, py, pz, 1.0, 1.0)

	pm.logf("Player %s threw a splash potion (%s)", player.Name, potionType)
}

// Tick processes splash potion projectile physics and impacts.
func (pm *PotionManager) Tick(tick int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var toRemove []int32

	for eid, potion := range pm.potions {
		potion.LifeTick++

		// Max flight time: 3 seconds (60 ticks)
		if potion.LifeTick > 60 {
			pm.splashImpact(potion)
			toRemove = append(toRemove, eid)
			continue
		}

		// Move
		potion.X += potion.VelX
		potion.Y += potion.VelY
		potion.Z += potion.VelZ
		potion.VelY -= 0.04 // gravity (lighter than arrows)

		// Check if hit ground (simplified: check if block at position is solid)
		bx := int(math.Floor(potion.X))
		by := int(math.Floor(potion.Y))
		bz := int(math.Floor(potion.Z))
		_ = bx
		_ = by
		_ = bz

		// Ground collision: if Y velocity went from positive to negative and below start
		if potion.VelY < -0.2 && potion.Y < float64(by)+0.5 && potion.LifeTick > 2 {
			pm.splashImpact(potion)
			toRemove = append(toRemove, eid)
			continue
		}

		// Broadcast position update
		pkt := pk.Marshal(
			packetid.ClientboundTeleportEntity,
			pk.VarInt(potion.EID),
			pk.Double(potion.X),
			pk.Double(potion.Y),
			pk.Double(potion.Z),
			pk.Double(potion.VelX), pk.Double(potion.VelY), pk.Double(potion.VelZ),
			pk.Float(0), pk.Float(0),
			pk.Int(0), // relative flags (all absolute)
			pk.Boolean(false),
		)
		pm.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}

	// Remove impacted potions
	for _, eid := range toRemove {
		pm.removePotion(eid)
	}
}

// splashImpact applies splash potion effects to nearby players.
func (pm *PotionManager) splashImpact(potion *SplashPotion) {
	pe, ok := potionTypeEffects[potion.PotionType]
	if !ok {
		// Default to healing for unknown types
		pe = PotionEffect{EffectInstantHealth, 0, 1}
	}

	// Play break sound and particles at impact location
	BroadcastSound(pm.Manager, SoundSplashPotionBreak, SoundCategoryNeutral,
		potion.X, potion.Y, potion.Z, 1.0, 1.0)

	// Apply effects to all players within 4 blocks
	pm.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dx := px - potion.X
		dy := (py + 0.9) - potion.Y
		dz := pz - potion.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist > 4.0 {
			return
		}

		// Scale effect by distance: full at 0, 25% at 4 blocks
		distFraction := 1.0 - dist/4.0
		if distFraction < 0.25 {
			distFraction = 0.25
		}

		// For instant effects, scale the potency
		if pe.EffectID == EffectInstantHealth || pe.EffectID == EffectInstantDamage {
			pm.EffectMgr.ApplyEffect(p, pe.EffectID, pe.Level, 1, false)
		} else {
			// Scale duration by distance
			scaledDuration := int32(float64(pe.Duration) * distFraction)
			if scaledDuration < 20 {
				scaledDuration = 20 // minimum 1 second
			}
			pm.EffectMgr.ApplyEffect(p, pe.EffectID, pe.Level, scaledDuration, false)
		}
	})
}

// removePotion removes a splash potion entity and broadcasts removal.
func (pm *PotionManager) removePotion(eid int32) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	pm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
	delete(pm.potions, eid)
}

// SendExistingPotions sends all current splash potions to a newly joined player.
func (pm *PotionManager) SendExistingPotions(player *game.Player) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, potion := range pm.potions {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(potion.EID),
			pk.UUID(entityUUID),
			pk.VarInt(splashPotionEntityType),
			pk.Double(potion.X),
			pk.Double(potion.Y),
			pk.Double(potion.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(potion.ThrowerEID+1),
		))
	}
}

func (pm *PotionManager) logf(format string, args ...any) {
	if pm.Logger != nil {
		pm.Logger.Printf(format, args...)
	}
}
