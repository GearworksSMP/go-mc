package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// tickPassive runs passive mob AI with flee and breeding behavior.
func (m *MobManager) tickPassive(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if m.tickFlee(mob) {
		return
	}

	// Baby growth: count down and convert to adult
	if mob.Baby && mob.BabyAge > 0 {
		mob.BabyAge--
		if mob.BabyAge <= 0 {
			mob.Baby = false
			mob.Speed /= 1.5 // revert baby speed boost
			// Send adult metadata (isBaby = false)
			var w MetadataWriter
			w.WriteBoolean(16, false)
			data := w.Bytes()
			m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
				SendEntityMetadata(p, mob.EID, data)
			})
		}
	}

	// Love mode countdown (babies can't breed)
	if !mob.Baby && mob.LoveTicks > 0 {
		mob.LoveTicks--
		// Check for nearby mob in love mode for breeding
		if mob.LoveTicks > 0 && mob.BreedCooldown <= 0 {
			m.tryBreed(mob)
		}
	}

	// Breed cooldown
	if mob.BreedCooldown > 0 {
		mob.BreedCooldown--
	}

	m.tickWander(mob, tick)
}

// isBreedingFood returns true if the item is food for the given mob type.
func isBreedingFood(mobType int32, itemName string) bool {
	switch mobType {
	case MobTypeCow, MobTypeSheep:
		return itemName == "wheat"
	case MobTypePig:
		return itemName == "carrot" || itemName == "potato" || itemName == "beetroot"
	case MobTypeChicken:
		return itemName == "wheat_seeds" || itemName == "melon_seeds" ||
			itemName == "pumpkin_seeds" || itemName == "beetroot_seeds"
	case MobTypeWolf:
		return itemName == "cooked_beef" || itemName == "cooked_porkchop" ||
			itemName == "cooked_chicken" || itemName == "cooked_mutton"
	case MobTypeCat:
		return itemName == "cod" || itemName == "salmon" ||
			itemName == "raw_cod" || itemName == "raw_salmon"
	case MobTypeHorse:
		return itemName == "golden_carrot" || itemName == "golden_apple"
	case MobTypeParrot:
		return false // parrots don't breed
	case MobTypeTurtle:
		return itemName == "seagrass"
	}
	return false
}

// FeedMob attempts to put a mob into love mode for breeding.
// Returns true if the mob was fed successfully.
func (m *MobManager) FeedMob(targetEID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 || mob.Hostile {
		return false
	}
	if mob.Baby {
		// Feeding a baby accelerates growth by 10% (2400 ticks)
		if mob.BabyAge > 0 {
			mob.BabyAge -= 2400
			if mob.BabyAge < 0 {
				mob.BabyAge = 0
			}
		}
		return true
	}
	if mob.LoveTicks > 0 || mob.BreedCooldown > 0 {
		return false
	}

	mob.LoveTicks = 600 // 30 seconds to find a mate

	// Broadcast heart particles
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(18), // love mode hearts
		))
	})

	return true
}

// tryBreed checks for a nearby mob of the same type in love mode and breeds them.
func (m *MobManager) tryBreed(mob *Mob) {
	for _, other := range m.Mobs {
		if other.EID == mob.EID || other.TypeID != mob.TypeID {
			continue
		}
		if other.Health <= 0 || other.LoveTicks <= 0 || other.BreedCooldown > 0 {
			continue
		}
		dx := other.X - mob.X
		dz := other.Z - mob.Z
		if dx*dx+dz*dz > 64 { // within 8 blocks
			continue
		}

		// Breed! Spawn baby at midpoint
		babyX := (mob.X + other.X) / 2
		babyZ := (mob.Z + other.Z) / 2

		eid := m.Manager.NextEntityID()
		baby := &Mob{
			EID:       eid,
			TypeID:    mob.TypeID,
			X:         babyX,
			Y:         mob.Y,
			Z:         babyZ,
			PrevX:     babyX,
			PrevY:     mob.Y,
			PrevZ:     babyZ,
			Health:    mob.MaxHealth / 2,
			MaxHealth: mob.MaxHealth,
			Speed:     mob.Speed * 1.5, // babies move faster
			WanderYaw: rand.Float32() * 360,
			Hostile:   false,
			Baby:      true,
			BabyAge:   24000, // 20 minutes to grow up
		}
		m.Mobs[eid] = baby
		m.broadcastSpawn(baby)

		// Heart particles at both parent positions
		BroadcastParticle(m.Manager, ParticleHeart, mob.X, mob.Y+1.0, mob.Z, 0.3, 0.3, 0.3, 0.0, 7)
		BroadcastParticle(m.Manager, ParticleHeart, other.X, other.Y+1.0, other.Z, 0.3, 0.3, 0.3, 0.0, 7)

		// Reset parents
		mob.LoveTicks = 0
		mob.BreedCooldown = 6000 // 5 minutes
		other.LoveTicks = 0
		other.BreedCooldown = 6000

		return
	}
}

// sweepDeadMobs removes mobs whose death animation has finished.
func (m *MobManager) sweepDeadMobs(tick int64) {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.DeathTick > 0 && tick-mob.DeathTick >= 20 {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		mob := m.Mobs[eid]
		if mob != nil {
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(mob.EID),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(removePkt)
			})
		}
		m.Spatial.Remove(eid)
		delete(m.Mobs, eid)
	}
}

// grantBadOmen increments a player's Bad Omen level when they kill a patrol captain.
func (m *MobManager) grantBadOmen(killer *game.Player) {
	killer.BadOmen++
	if killer.BadOmen > 7 {
		killer.BadOmen = 7
	}
	if m.EffectMgr != nil {
		m.EffectMgr.ApplyEffect(killer, EffectBadOmen, killer.BadOmen-1, 120000, false)
	}
	if m.Logger != nil {
		m.Logger.Printf("Player %s killed patrol captain, Bad Omen level %d", killer.Name, killer.BadOmen)
	}
}

// killMobWithLooting handles mob death with Looting enchantment bonus drops.
func (m *MobManager) killMobWithLooting(mob *Mob, killer *game.Player, lootingLevel int32) {
	if mob.DeathTick > 0 {
		return // already dying
	}
	mob.Health = 0
	mob.DeathTick = m.currentTick
	if mob.DeathTick == 0 {
		mob.DeathTick = 1
	}

	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Death particles (poof + smoke)
	BroadcastParticle(m.Manager, ParticlePoof, mob.X, mob.Y+0.5, mob.Z, 0.3, 0.5, 0.3, 0.05, 10)
	BroadcastParticle(m.Manager, ParticleSmoke, mob.X, mob.Y+0.5, mob.Z, 0.2, 0.4, 0.2, 0.02, 5)

	// Death animation
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(3),
		))
	})

	// Drop loot with looting bonus
	m.dropMobLootWithLooting(mob, lootingLevel)

	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 1 {
		m.slimeSplit(mob)
	}

	// Spawn XP orbs at mob death location
	if killer != nil {
		if mob.TypeID == MobTypePillager && mob.IsPatrolCaptain {
			m.grantBadOmen(killer)
		}
		m.spawnMobXPOrbs(mob)
	}
}

// killMob handles mob death: animation, drops, XP. Entity removal is deferred
// to sweepDeadMobs (20 ticks later) to allow the death animation to play.
func (m *MobManager) killMob(mob *Mob, killer *game.Player) {
	if mob.DeathTick > 0 {
		return // already dying
	}
	mob.Health = 0
	mob.DeathTick = m.currentTick
	if mob.DeathTick == 0 {
		mob.DeathTick = 1 // ensure nonzero so sweep detects it
	}

	// Death sound
	BroadcastSound(m.Manager, MobDeathSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Death particles (poof + smoke)
	BroadcastParticle(m.Manager, ParticlePoof, mob.X, mob.Y+0.5, mob.Z, 0.3, 0.5, 0.3, 0.05, 10)
	BroadcastParticle(m.Manager, ParticleSmoke, mob.X, mob.Y+0.5, mob.Z, 0.2, 0.4, 0.2, 0.02, 5)

	// Death animation
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(3), // death
		))
	})

	// Drop loot
	m.dropMobLoot(mob)

	// Slime split on death: spawn smaller slimes
	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 1 {
		m.slimeSplit(mob)
	}

	// Advancement check
	if killer != nil && m.AdvMgr != nil {
		m.AdvMgr.CheckMobKill(killer, mob.TypeID)
	}

	// Award XP to killer
	// Spawn XP orbs at mob death location
	if killer != nil {
		if mob.TypeID == MobTypePillager && mob.IsPatrolCaptain {
			m.grantBadOmen(killer)
		}
		m.spawnMobXPOrbs(mob)
	}
}

// spawnMobXPOrbs spawns XP orb entities at a mob's death location.
func (m *MobManager) spawnMobXPOrbs(mob *Mob) {
	if m.XPOrbMgr == nil {
		return
	}
	xp := mobXPAmount(mob)
	if xp > 0 {
		m.XPOrbMgr.SpawnXPOrbs(mob.X, mob.Y+0.5, mob.Z, xp)
	}
}

// dropMobLoot drops item entities for a killed mob.
func (m *MobManager) dropMobLoot(mob *Mob) {
	m.dropMobLootWithLooting(mob, 0)
}

// dropMobLootWithLooting drops loot with Looting enchantment bonus.
// Each Looting level adds 0-1 extra items per drop.
func (m *MobManager) dropMobLootWithLooting(mob *Mob, lootingLevel int32) {
	if m.ItemEntities == nil {
		return
	}

	type drop struct {
		name     string
		minCount int32
		maxCount int32
	}

	var drops []drop
	switch mob.TypeID {
	case MobTypeCow:
		drops = []drop{{"beef", 1, 3}, {"leather", 0, 2}}
	case MobTypePig:
		drops = []drop{{"porkchop", 1, 3}}
	case MobTypeSheep:
		// Drop colored wool based on sheep's wool color
		woolName := woolColorToItem(mob.WoolColor)
		drops = []drop{{woolName, 1, 1}, {"mutton", 1, 2}}
	case MobTypeChicken:
		drops = []drop{{"chicken", 1, 1}, {"feather", 0, 2}}
	case MobTypeZombie:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Rare drops: 2.5% base + 1% per looting level
		rareChance := 0.025 + float64(lootingLevel)*0.01
		if rand.Float64() < rareChance {
			switch rand.Intn(3) {
			case 0:
				drops = append(drops, drop{"iron_ingot", 1, 1})
			case 1:
				drops = append(drops, drop{"carrot", 1, 1})
			case 2:
				drops = append(drops, drop{"potato", 1, 1})
			}
		}
	case MobTypeSkeleton:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
		// 8.5% chance to drop a bow (+ 1% per looting level)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"bow", 1, 1})
		}
	case MobTypeSpider:
		drops = []drop{{"string", 0, 2}}
		// Spider eye only drops when killed by player (always true here)
		if rand.Float64() < 0.33+float64(lootingLevel)*0.015 {
			drops = append(drops, drop{"spider_eye", 1, 1})
		}
	case MobTypeCreeper:
		drops = []drop{{"gunpowder", 0, 2}}
	case MobTypeEnderman:
		drops = []drop{{"ender_pearl", 0, 1}}
	case MobTypeBlaze:
		drops = []drop{{"blaze_rod", 0, 1}}
	case MobTypeBreeze:
		drops = []drop{{"breeze_rod", 1, 2}}
	case MobTypeWitherSkeleton:
		drops = []drop{{"bone", 0, 2}, {"coal", 0, 1}}
		if rand.Float64() < 0.025+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"wither_skeleton_skull", 1, 1})
		}
	case MobTypeGuardian:
		drops = []drop{{"prismarine_shard", 0, 2}}
		if rand.Float64() < 0.4 {
			drops = append(drops, drop{"cod", 0, 1})
		}
	case MobTypeElderGuardian:
		drops = []drop{{"prismarine_shard", 0, 2}, {"wet_sponge", 1, 1}}
		if rand.Float64() < 0.5 {
			drops = append(drops, drop{"cod", 0, 2})
		}
	case MobTypeDrowned:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Rare drop: copper ingot (5% + 2% per looting)
		if rand.Float64() < 0.05+float64(lootingLevel)*0.02 {
			drops = append(drops, drop{"copper_ingot", 1, 1})
		}
	case MobTypeHusk:
		drops = []drop{{"rotten_flesh", 0, 2}}
		// Same rare drops as zombie
		rareChance := 0.025 + float64(lootingLevel)*0.01
		if rand.Float64() < rareChance {
			switch rand.Intn(3) {
			case 0:
				drops = append(drops, drop{"iron_ingot", 1, 1})
			case 1:
				drops = append(drops, drop{"carrot", 1, 1})
			case 2:
				drops = append(drops, drop{"potato", 1, 1})
			}
		}
	case MobTypeStray:
		drops = []drop{{"bone", 0, 2}, {"arrow", 0, 2}}
		// Stray drops tipped arrows of slowness (50% + looting)
		if rand.Float64() < 0.5+float64(lootingLevel)*0.05 {
			drops = append(drops, drop{"tipped_arrow", 1, 1})
		}
	case MobTypeCaveSpider:
		drops = []drop{{"string", 0, 2}}
		if rand.Float64() < 0.33+float64(lootingLevel)*0.015 {
			drops = append(drops, drop{"spider_eye", 1, 1})
		}
	case MobTypeMagmaCube:
		drops = []drop{{"magma_cream", 0, 1}}
	case MobTypeGhast:
		drops = []drop{{"ghast_tear", 0, 1}, {"gunpowder", 0, 2}}
	case MobTypePiglin:
		drops = []drop{{"gold_ingot", 0, 1}}
	case MobTypeZombifiedPiglin:
		drops = []drop{{"rotten_flesh", 0, 1}, {"gold_nugget", 0, 1}}
		// Rare drop: gold ingot (2.5% + looting)
		if rand.Float64() < 0.025+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"gold_ingot", 1, 1})
		}
	case MobTypeHoglin:
		drops = []drop{{"porkchop", 2, 4}, {"leather", 1, 3}}
	case MobTypeStrider:
		drops = []drop{{"string", 2, 5}}
	case MobTypePillager:
		drops = []drop{{"arrow", 0, 2}}
		// Rare: crossbow (8.5% + looting)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"crossbow", 1, 1})
		}
	case MobTypeVindicator:
		drops = []drop{{"emerald", 0, 1}}
		// Rare: iron axe (8.5% + looting)
		if rand.Float64() < 0.085+float64(lootingLevel)*0.01 {
			drops = append(drops, drop{"iron_axe", 1, 1})
		}
	case MobTypeEvoker:
		drops = []drop{{"totem_of_undying", 1, 1}, {"emerald", 0, 1}}
	case MobTypeRavager:
		drops = []drop{{"saddle", 1, 1}}
	case MobTypeShulker:
		// 50% chance + 6.25% per looting level
		if rand.Float64() < 0.5+float64(lootingLevel)*0.0625 {
			drops = []drop{{"shulker_shell", 1, 1}}
		}
	case MobTypeRabbit:
		drops = []drop{{"rabbit", 0, 1}, {"rabbit_hide", 0, 1}}
		// Rare: rabbit's foot (10% + looting)
		if rand.Float64() < 0.1+float64(lootingLevel)*0.03 {
			drops = append(drops, drop{"rabbit_foot", 1, 1})
		}
	case MobTypeIronGolem:
		drops = []drop{{"iron_ingot", 3, 5}, {"poppy", 0, 2}}
	case MobTypeSnowGolem:
		drops = []drop{{"snowball", 0, 15}}
	case MobTypeVillager:
		// Villagers don't drop items in vanilla
	case MobTypeWitch:
		// Witch drops 1-3 of these categories randomly
		witchItems := []drop{
			{"glass_bottle", 0, 2},
			{"redstone", 0, 2},
			{"glowstone_dust", 0, 2},
			{"gunpowder", 0, 2},
			{"sugar", 0, 2},
			{"spider_eye", 0, 2},
			{"stick", 0, 2},
		}
		count := 1 + rand.Intn(3) // drop 1-3 categories
		for i := 0; i < count && i < len(witchItems); i++ {
			idx := rand.Intn(len(witchItems))
			drops = append(drops, witchItems[idx])
		}
	case MobTypeSlime:
		if mob.SlimeSize <= 1 {
			drops = []drop{{"slime_ball", 0, 2}}
		}
	case MobTypePhantom:
		drops = []drop{{"phantom_membrane", 0, 1}}
	case MobTypeCat:
		if rand.Float64() < 0.5 {
			drops = []drop{{"string", 0, 2}}
		}
	case MobTypeHorse:
		drops = []drop{{"leather", 0, 2}}
		if mob.TameData != nil {
			if mob.TameData.ArmorItemID > 0 {
				if armorName := ItemNameByID(mob.TameData.ArmorItemID); armorName != "" {
					drops = append(drops, drop{armorName, 1, 1})
				}
			}
			if mob.TameData.HasSaddle {
				drops = append(drops, drop{"saddle", 1, 1})
			}
		}
	case MobTypeLlama, MobTypeTraderLlama:
		drops = []drop{{"leather", 0, 2}}
		// Drop chest contents
		if mob.TameData != nil && mob.TameData.HasChest {
			for _, item := range mob.TameData.ChestInventory {
				if item.ID > 0 {
					itemName := ItemNameByID(item.ID)
					if itemName != "" {
						drops = append(drops, drop{itemName, item.Count, item.Count})
					}
				}
			}
		}
	case MobTypeParrot:
		drops = []drop{{"feather", 1, 2}}
	case MobTypeBee:
		// Bees drop nothing in vanilla
	case MobTypeWarden:
		drops = []drop{{"sculk_catalyst", 1, 1}}
	case MobTypeFrog:
		// Frogs drop nothing in vanilla (froglights come from tongue attack)
	case MobTypeAxolotl:
		// Axolotls drop nothing in vanilla
	case MobTypeAllay:
		// Allays drop nothing in vanilla (but drop their held item)
	case MobTypeSniffer:
		drops = []drop{{"moss_block", 1, 1}}
	case MobTypeTurtle:
		if !mob.Baby {
			drops = []drop{{"seagrass", 0, 2}}
		}
		// Baby turtles drop scute on growth, not on death
	}

	for _, d := range drops {
		count := d.minCount
		if d.maxCount > d.minCount {
			count += rand.Int31n(d.maxCount - d.minCount + 1)
		}
		// Looting bonus: add 0 to lootingLevel extra items
		if lootingLevel > 0 {
			count += rand.Int31n(lootingLevel + 1)
		}
		if count <= 0 {
			continue
		}
		// Cook meat if mob died while on fire
		itemName := d.name
		if mob.FireTicks > 0 {
			if cooked, ok := cookedMeats[itemName]; ok {
				itemName = cooked
			}
		}
		itemID := itemIDByName(itemName)
		if itemID <= 0 {
			continue
		}
		m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, count, 10)
	}
}

// DamageMob applies damage to a mob from a player attack.
// Returns true if the mob was found and damaged.
func (m *MobManager) DamageMob(attacker *game.Player, targetEID int32, damage float32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		return false
	}

	// Apply horse armor damage reduction
	if mob.TypeID == MobTypeHorse {
		reduction := horseArmorDamageReduction(mob)
		damage -= reduction
		if damage < 0 {
			damage = 0
		}
	}

	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
	}

	// Broadcast hurt animation
	hurtPkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(mob.EID),
		pk.Float(0),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(hurtPkt)
	})

	// Broadcast damage event
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(mob.EID),
		pk.VarInt(m.Survival.MobDamageTypeID),
		pk.VarInt(attacker.EID+1), // cause entity (+1 because 0 = none)
		pk.VarInt(attacker.EID+1), // direct entity
		pk.Boolean(false),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Enderman teleport on hit
	if mob.TypeID == MobTypeEnderman && mob.TeleportCooldown <= 0 && mob.Health > 0 {
		m.endermanTeleport(mob)
		mob.TeleportCooldown = 20
		mob.Target = attacker // aggro on the attacker
	}

	// Zombified piglin swarm aggro
	if mob.TypeID == MobTypeZombifiedPiglin {
		mob.Hostile = true
		mob.Target = attacker
		m.AggroZombifiedPiglins(attacker, mob.X, mob.Y, mob.Z)
	}

	// Passive mobs flee when hit
	if !mob.Hostile {
		px, _, pz := attacker.Position()
		dx := mob.X - px
		dz := mob.Z - pz
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			mob.FleeX = mob.X + dx/dist*16
			mob.FleeZ = mob.Z + dz/dist*16
		} else {
			mob.FleeX = mob.X + (rand.Float64()-0.5)*16
			mob.FleeZ = mob.Z + (rand.Float64()-0.5)*16
		}
		mob.FleeTicks = 60 // 3 seconds
	}

	m.applyVillagerDamageGossip(mob, attacker)

	if mob.Health <= 0 {
		m.applyVillagerKillGossip(mob, attacker)
		m.killMob(mob, attacker)
	}

	return true
}

// DamageMobEx applies damage to a mob with combat enchant effects.
// knockbackLevel: extra knockback (from enchant + sprint).
// fireAspectLevel: sets mob on fire visual (4s per level).
// lootingLevel: extra loot drops on kill.
func (m *MobManager) DamageMobEx(attacker *game.Player, targetEID int32, damage float32, knockbackLevel, fireAspectLevel, lootingLevel int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		return false
	}

	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
	}

	// Broadcast hurt animation
	hurtPkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(mob.EID),
		pk.Float(0),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(hurtPkt)
	})

	// Broadcast damage event
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(mob.EID),
		pk.VarInt(m.Survival.MobDamageTypeID),
		pk.VarInt(attacker.EID+1),
		pk.VarInt(attacker.EID+1),
		pk.Boolean(false),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	// Knockback enchantment
	if knockbackLevel > 0 {
		ax, _, az := attacker.Position()
		dx := mob.X - ax
		dz := mob.Z - az
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			kbDist := float64(knockbackLevel) * 0.5
			mob.X += dx / dist * kbDist
			mob.Z += dz / dist * kbDist
			m.broadcastMobTeleport(mob)
		}
	}

	// Fire Aspect: set mob on fire with ticking damage
	if fireAspectLevel > 0 {
		mob.FireTicks = fireAspectLevel * 80 // 4 seconds per level
		broadcastFireMetadata(m.Manager, mob.EID, true)
	}

	// Enderman teleport on hit
	if mob.TypeID == MobTypeEnderman && mob.TeleportCooldown <= 0 && mob.Health > 0 {
		m.endermanTeleport(mob)
		mob.TeleportCooldown = 20
		mob.Target = attacker
	}

	// Passive mobs flee when hit
	if !mob.Hostile {
		px, _, pz := attacker.Position()
		dx := mob.X - px
		dz := mob.Z - pz
		dist := math.Sqrt(dx*dx + dz*dz)
		if dist > 0.1 {
			mob.FleeX = mob.X + dx/dist*16
			mob.FleeZ = mob.Z + dz/dist*16
		} else {
			mob.FleeX = mob.X + (rand.Float64()-0.5)*16
			mob.FleeZ = mob.Z + (rand.Float64()-0.5)*16
		}
		mob.FleeTicks = 60
	}

	m.applyVillagerDamageGossip(mob, attacker)

	if mob.Health <= 0 {
		m.applyVillagerKillGossip(mob, attacker)
		m.killMobWithLooting(mob, attacker, lootingLevel)
	}

	return true
}

// DamageMobsNearExcept deals sweep damage to mobs within radius of the target mob.
func (m *MobManager) DamageMobsNearExcept(attacker *game.Player, primaryEID int32, damage float32, radius float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	primary, ok := m.Mobs[primaryEID]
	if !ok {
		return
	}

	for _, mob := range m.Mobs {
		if mob.EID == primaryEID || mob.Health <= 0 {
			continue
		}
		dx := mob.X - primary.X
		dy := mob.Y - primary.Y
		dz := mob.Z - primary.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > radius {
			continue
		}

		mob.Health -= damage
		if mob.Health < 0 {
			mob.Health = 0
		}

		hurtPkt := pk.Marshal(
			packetid.ClientboundHurtAnimation,
			pk.VarInt(mob.EID),
			pk.Float(0),
		)
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(hurtPkt)
		})

		if mob.Health <= 0 {
			m.killMob(mob, attacker)
		}
	}
}

// DamageMobByArrow applies arrow damage to a mob. Returns whether the mob died and its type ID.
func (m *MobManager) DamageMobByArrow(shooterEID, targetEID int32, damage float32) (killed bool, typeID int32) {
	m.mu.Lock()
	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		m.mu.Unlock()
		return false, 0
	}
	typeID = mob.TypeID
	mob.Health -= damage
	if mob.Health < 0 {
		mob.Health = 0
	}

	// Broadcast hurt animation
	hurtPkt := pk.Marshal(packetid.ClientboundHurtAnimation, pk.VarInt(mob.EID), pk.Float(0))
	m.Manager.ForEach(func(p *game.Player) { p.WritePacket(hurtPkt) })

	// Play hurt sound
	BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)

	if mob.Health <= 0 {
		m.mu.Unlock()
		m.killMob(mob, nil) // no player killer for mob-on-mob
		return true, typeID
	}
	m.mu.Unlock()
	return false, typeID
}

// sunlightDamage applies damage to zombies and skeletons in sunlight.
// Skipped when raining, since rain protects undead from burning.
func (m *MobManager) sunlightDamage() {
	if m.WeatherMgr != nil && m.WeatherMgr.State() >= WeatherRain {
		return
	}
	for _, mob := range m.Mobs {
		if mob.Health <= 0 {
			continue
		}
		if mob.TypeID != MobTypeZombie && mob.TypeID != MobTypeSkeleton {
			continue
		}
		mob.Health -= 1
		if mob.Health <= 0 {
			mob.Health = 0
			m.killMob(mob, nil)
		} else {
			// Broadcast hurt
			hurtPkt := pk.Marshal(
				packetid.ClientboundHurtAnimation,
				pk.VarInt(mob.EID),
				pk.Float(0),
			)
			m.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(hurtPkt)
			})
			BroadcastSound(m.Manager, MobHurtSound(mob.TypeID), MobSoundCategory(mob.TypeID), mob.X, mob.Y, mob.Z, 1.0, 1.0)
		}
	}
}

// slimeSplit spawns 2-4 smaller slimes when a slime dies.
func (m *MobManager) slimeSplit(mob *Mob) {
	var newSize int32
	switch mob.SlimeSize {
	case 4:
		newSize = 2
	case 2:
		newSize = 1
	default:
		return // small slimes don't split
	}

	count := 2 + rand.Int31n(3) // 2-4 smaller slimes
	for i := int32(0); i < count; i++ {
		var health float32
		var damage float32
		switch newSize {
		case 1:
			health, damage = 1, 0
		case 2:
			health, damage = 4, 2
		}

		babyX := mob.X + (rand.Float64()-0.5)*2
		babyZ := mob.Z + (rand.Float64()-0.5)*2
		eid := m.Manager.NextEntityID()
		baby := &Mob{
			EID:       eid,
			TypeID:    MobTypeSlime,
			X:         babyX,
			Y:         mob.Y,
			Z:         babyZ,
			PrevX:     babyX,
			PrevY:     mob.Y,
			PrevZ:     babyZ,
			Health:    health,
			MaxHealth: health,
			Damage:    damage,
			Speed:     0.1,
			WanderYaw: rand.Float32() * 360,
			Hostile:   true,
			SlimeSize: newSize,
		}
		m.Mobs[eid] = baby
		m.broadcastSpawn(baby)
	}
}

// applyVillagerDamageGossip adds minor negative gossip when a villager is hit.
// Must be called with m.mu already held.
func (m *MobManager) applyVillagerDamageGossip(mob *Mob, attacker *game.Player) {
	if mob.TypeID != MobTypeVillager || mob.VillagerData == nil {
		return
	}
	mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipMinorNegative, attacker.UUID, 5, m.currentTick)
	m.spreadVillagerGossip(mob, GossipMinorNegative, attacker.UUID, 5)
}

// applyVillagerKillGossip adds major negative gossip when a villager is killed.
// Must be called with m.mu already held.
func (m *MobManager) applyVillagerKillGossip(mob *Mob, attacker *game.Player) {
	if mob.TypeID != MobTypeVillager || mob.VillagerData == nil {
		return
	}
	mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipMajorNegative, attacker.UUID, 25, m.currentTick)
	m.spreadVillagerGossip(mob, GossipMajorNegative, attacker.UUID, 25)
}

// spreadVillagerGossip propagates a gossip entry to nearby villagers within 16 blocks.
// Must be called with m.mu already held.
func (m *MobManager) spreadVillagerGossip(source *Mob, gtype string, playerUUID uuid.UUID, value int) {
	for _, other := range m.Mobs {
		if other.EID == source.EID || other.TypeID != MobTypeVillager || other.Health <= 0 || other.VillagerData == nil {
			continue
		}
		dx := other.X - source.X
		dz := other.Z - source.Z
		if dx*dx+dz*dz <= 16*16 {
			other.VillagerData.Gossip = AddGossip(other.VillagerData.Gossip, gtype, playerUUID, value/2, m.currentTick)
		}
	}
}
