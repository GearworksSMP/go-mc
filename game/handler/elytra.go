package handler

import (
	"log"
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	pk "github.com/Tnze/go-mc/net/packet"
)

// elytraMaxDurability is the max durability of elytra (vanilla = 432).
const elytraMaxDurability int32 = 432

// poseFallFlying is the FALL_FLYING pose value for entity metadata.
const poseFallFlying int32 = 1

// ElytraManager handles elytra gliding activation, deactivation, and durability.
type ElytraManager struct {
	Manager  *game.PlayerManager
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// handleElytraActivation is called from AnimationHandler when action 8 (START_FALL_FLYING) is received.
func (em *ElytraManager) handleElytraActivation(player *game.Player) {
	if player.Dead {
		return
	}

	// Check if player has elytra equipped in chestplate slot (slot 6)
	chestItem := &player.Inventory[6]
	if chestItem.ID <= 0 || chestItem.Count <= 0 {
		return
	}
	itemName := ItemNameByID(chestItem.ID)
	if itemName != "elytra" {
		return
	}

	// Check if elytra is broken (durability 0 or less)
	if chestItem.MaxDurability > 0 && chestItem.Durability <= 0 {
		return
	}

	// Player must be falling (not on ground) to activate elytra
	if player.OnGround {
		return
	}

	// Activate gliding
	player.Gliding = true

	// Broadcast pose change to FALL_FLYING
	em.broadcastGlidePose(player, true)

	em.logf("Player %s activated elytra", player.Name)
}

// HandleUseItem processes ServerboundUseItem for firework rocket boost while gliding.
// Returns true if the packet was handled (firework+elytra related).
func (em *ElytraManager) HandleUseItem(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundUseItem {
		return false
	}

	if !player.Gliding {
		return false
	}

	var hand pk.VarInt
	var sequence pk.VarInt
	if err := p.Scan(&hand, &sequence); err != nil {
		return false
	}

	if player.Dead {
		return false
	}

	// Check held item is a firework rocket
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return false
	}
	itemName := ItemNameByID(invItem.ID)
	if itemName != "firework_rocket" {
		return false
	}

	// Consume one firework in survival
	if player.GameMode == 0 {
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Apply velocity boost in the look direction
	yaw, pitch := player.Rotation()
	yawRad := float64(yaw) * math.Pi / 180.0
	pitchRad := float64(pitch) * math.Pi / 180.0
	dirX := -math.Sin(yawRad) * math.Cos(pitchRad)
	dirY := -math.Sin(pitchRad)
	dirZ := math.Cos(yawRad) * math.Cos(pitchRad)

	boostSpeed := 12000.0
	velX := int16(dirX * boostSpeed)
	velY := int16(dirY * boostSpeed)
	velZ := int16(dirZ * boostSpeed)

	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityMotion,
		pk.VarInt(player.EID),
		pk.Short(velX),
		pk.Short(velY),
		pk.Short(velZ),
	))

	// Play firework launch sound
	px, py, pz := player.Position()
	BroadcastSound(em.Manager, SoundFireworkLaunch, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Send acknowledge
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))

	em.logf("Player %s used firework rocket boost", player.Name)
	return true
}

// Tick handles per-tick elytra updates: velocity tracking, durability loss, and landing detection.
func (em *ElytraManager) Tick(tick int64) {
	em.Manager.ForEach(func(player *game.Player) {
		if !player.Gliding {
			player.ElytraTracking = false
			return
		}

		// Track velocity from position deltas
		px, py, pz := player.Position()
		if player.ElytraTracking {
			player.ElytraVelX = px - player.ElytraLastX
			player.ElytraVelY = py - player.ElytraLastY
			player.ElytraVelZ = pz - player.ElytraLastZ
		}
		player.ElytraLastX = px
		player.ElytraLastY = py
		player.ElytraLastZ = pz
		player.ElytraTracking = true

		// Deactivate gliding if player is on ground or dead
		if player.OnGround || player.Dead {
			player.Gliding = false
			em.broadcastGlidePose(player, false)

			if !player.Dead && player.ElytraTracking {
				em.applyLanding(player)
			}

			player.ElytraTracking = false
			// Reset normal fall tracking so trackFall() doesn't double-apply damage
			player.FallStartY = -999
			return
		}

		// Consume durability: 1 point per second (every 20 ticks)
		if tick%20 == 0 && player.GameMode == 0 {
			chestItem := &player.Inventory[6]
			if chestItem.ID > 0 && chestItem.MaxDurability > 0 {
				// Unbreaking enchant check
				unbreakLvl := enchant.GetLevel(chestItem.Enchantments, enchant.Unbreaking)
				shouldReduce := true
				if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
					shouldReduce = false
				}
				if shouldReduce {
					chestItem.Durability--
					if chestItem.Durability <= 1 {
						// Elytra doesn't break completely, it stops working at durability 1
						chestItem.Durability = 1
						player.Gliding = false
						em.broadcastGlidePose(player, false)
						player.ElytraTracking = false
						em.logf("Player %s elytra broke (durability depleted)", player.Name)
					}
					SendSlotUpdate(player, 6)
				}
			}
		}
	})
}

// applyLanding handles elytra landing: fall damage, particles, and sounds.
func (em *ElytraManager) applyLanding(player *game.Player) {
	vx := player.ElytraVelX
	vy := player.ElytraVelY
	vz := player.ElytraVelZ

	// Total speed in blocks/tick
	speed := math.Sqrt(vx*vx + vy*vy + vz*vz)

	px, py, pz := player.Position()

	// Landing particles: broadcast block crack particles if speed is significant
	if speed > 0.5 {
		// Particle count proportional to landing speed (5-20 range)
		count := int32(speed * 10)
		if count < 5 {
			count = 5
		}
		if count > 20 {
			count = 20
		}
		BroadcastParticle(em.Manager, ParticleBlock, px, py, pz, 0.3, 0.1, 0.3, 0.0, count)
	}

	// No damage in creative/spectator, water, invulnerable, or slow falling
	if player.GameMode == 1 || player.GameMode == 3 {
		return
	}
	if player.InWater || player.IsInvulnerable() {
		return
	}
	if player.Effects != nil {
		if _, ok := player.Effects[EffectSlowFalling]; ok {
			return
		}
	}

	// Vanilla elytra damage: floor(speed_per_tick * 10 - 3), minimum 0
	damage := math.Floor(speed*10 - 3)
	if damage <= 0 {
		return
	}

	// Protection enchantment: check all armor slots (5=helmet, 6=chest, 7=legs, 8=boots)
	// Each level of Protection reduces damage by 4% (up to 80% total across all pieces)
	totalProtLevel := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		armorItem := &player.Inventory[slot]
		if lvl := enchant.GetLevel(armorItem.Enchantments, enchant.Protection); lvl > 0 {
			totalProtLevel += lvl
		}
	}
	if totalProtLevel > 0 {
		// Cap at 20 levels (80% reduction)
		if totalProtLevel > 20 {
			totalProtLevel = 20
		}
		reduction := float64(totalProtLevel) * 0.04
		damage *= 1 - reduction
	}

	// Feather Falling on boots: each level reduces by 12%
	bootsItem := &player.Inventory[8]
	if ffLvl := enchant.GetLevel(bootsItem.Enchantments, enchant.FeatherFalling); ffLvl > 0 {
		reduction := float64(ffLvl) * 0.12
		if reduction > 1.0 {
			reduction = 1.0
		}
		damage *= 1 - reduction
	}

	if damage <= 0 {
		return
	}

	// Play fall sound based on damage severity
	if damage >= 4 {
		BroadcastSound(em.Manager, SoundFallBig, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
	} else {
		BroadcastSound(em.Manager, SoundFallSmall, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
	}

	// Apply damage
	player.LastDamageMessage = player.Name + " experienced kinetic energy"
	if em.Survival != nil {
		em.Survival.ApplyDamage(em.Manager, player, float32(damage), em.Survival.FallDamageTypeID)
	}
	em.logf("Player %s elytra landing damage: %.1f (speed=%.2f b/t)", player.Name, damage, speed)
}

// broadcastGlidePose sends a pose metadata update to all players.
func (em *ElytraManager) broadcastGlidePose(player *game.Player, gliding bool) {
	var w MetadataWriter
	w.WriteByte(0, EntityFlags(player))
	if gliding {
		w.WritePose(6, poseFallFlying)
	} else {
		w.WritePose(6, playerPose(player))
	}
	data := w.Bytes()

	em.Manager.ForEach(func(p *game.Player) {
		if p.UUID != player.UUID {
			SendEntityMetadata(p, player.EID, data)
		}
	})
}

// StopGliding forces a player to stop gliding (called when player takes damage).
func (em *ElytraManager) StopGliding(player *game.Player) {
	if !player.Gliding {
		return
	}
	player.Gliding = false
	em.broadcastGlidePose(player, false)
	em.logf("Player %s stopped gliding (took damage)", player.Name)
}

func (em *ElytraManager) logf(format string, args ...any) {
	if em.Logger != nil {
		em.Logger.Printf(format, args...)
	}
}
