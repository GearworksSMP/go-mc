package handler

import (
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SurvivalHandler manages health, damage, hunger, and related survival mechanics.
type SurvivalHandler struct {
	Logger             *log.Logger
	FallDamageTypeID   int32
	AttackDamageTypeID int32
	MobDamageTypeID    int32 // same as AttackDamageTypeID but tagged for difficulty scaling
	VoidDamageTypeID   int32
	FireDamageTypeID   int32
	DrownDamageTypeID  int32
	ItemEntities       *ItemEntityManager
	Rules              *GameRules
	EffectMgr          *EffectManager // status effects (set after construction)
}

// SendSetHealth sends the health/food/saturation HUD update to a player.
func SendSetHealth(player *game.Player) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetHealth,
		pk.Float(player.Health),
		pk.VarInt(player.Food),
		pk.Float(player.Saturation),
	))
}

// ApplyDamage reduces player health, broadcasts hurt animation, and handles death.
func (s *SurvivalHandler) ApplyDamage(manager *game.PlayerManager, player *game.Player, damage float32, damageTypeID int32) {
	if player.Dead || player.IsInvulnerable() {
		return
	}

	// Difficulty scaling for mob damage
	if damageTypeID == s.MobDamageTypeID && s.Rules != nil {
		switch s.Rules.GetDifficulty() {
		case 0: // Peaceful: no mob damage
			return
		case 1: // Easy: half damage
			damage *= 0.5
		case 3: // Hard: 1.5x damage
			damage *= 1.5
		}
	}

	// Shield blocking: absorbs all damage except void (with 250ms cooldown between blocks)
	if damageTypeID != s.VoidDamageTypeID && player.Blocking && isHoldingShield(player) && time.Now().After(player.ShieldCooldownUntil) {
		reduceShieldDurability(player)
		player.ShieldCooldownUntil = time.Now().Add(250 * time.Millisecond)
		// Play shield block sound
		px, py, pz := player.Position()
		BroadcastSound(manager, SoundShieldBlock, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)
		return
	}

	// Calculate armor protection with toughness (vanilla formula)
	armorPts := float32(0)
	toughness := float32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		name := ItemNameByID(player.Inventory[slot].ID)
		armorPts += float32(GetArmorProtection(name))
		toughness += GetArmorToughness(name)
	}
	if armorPts > 0 {
		a := float64(armorPts) / 5.0
		b := float64(armorPts) - float64(damage)/(2.0+float64(toughness)/4.0)
		reduction := float32(math.Max(a, b)) / 25.0
		if reduction > 0.8 {
			reduction = 0.8
		}
		if reduction < 0 {
			reduction = 0
		}
		damage *= (1 - reduction)
	}

	// Protection enchantment: sum from all armor pieces, 4% reduction per level (cap 80%)
	protectionTotal := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		protectionTotal += enchant.GetLevel(player.Inventory[slot].Enchantments, enchant.Protection)
	}
	if protectionTotal > 0 {
		protReduction := float32(protectionTotal) * 0.04
		if protReduction > 0.8 {
			protReduction = 0.8
		}
		damage *= (1 - protReduction)
	}

	// Fire Protection enchantment: 8% reduction per level (cap 80%)
	fireProtTotal := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		fireProtTotal += enchant.GetLevel(player.Inventory[slot].Enchantments, enchant.FireProtection)
	}
	if fireProtTotal > 0 {
		fpReduction := float32(fireProtTotal) * 0.08
		if fpReduction > 0.8 {
			fpReduction = 0.8
		}
		damage *= (1 - fpReduction)
	}

	// Blast Protection enchantment: 8% reduction per level (cap 80%)
	blastProtTotal := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		blastProtTotal += enchant.GetLevel(player.Inventory[slot].Enchantments, enchant.BlastProtection)
	}
	if blastProtTotal > 0 {
		bpReduction := float32(blastProtTotal) * 0.08
		if bpReduction > 0.8 {
			bpReduction = 0.8
		}
		damage *= (1 - bpReduction)
	}

	// Projectile Protection enchantment: 8% reduction per level (cap 80%)
	projProtTotal := int32(0)
	for _, slot := range []int{5, 6, 7, 8} {
		projProtTotal += enchant.GetLevel(player.Inventory[slot].Enchantments, enchant.ProjectileProtection)
	}
	if projProtTotal > 0 {
		ppReduction := float32(projProtTotal) * 0.08
		if ppReduction > 0.8 {
			ppReduction = 0.8
		}
		damage *= (1 - ppReduction)
	}

	// Resistance effect: 20% reduction per level (cap 100%)
	if s.EffectMgr != nil {
		resReduction := s.EffectMgr.GetResistanceReduction(player)
		if resReduction > 0 {
			damage *= (1 - resReduction)
		}
	}

	// Absorption: absorb damage from extra HP pool first
	if player.Absorption > 0 {
		if damage <= player.Absorption {
			player.Absorption -= damage
			damage = 0
		} else {
			damage -= player.Absorption
			player.Absorption = 0
		}
	}

	player.Health -= damage
	if player.Health < 0 {
		player.Health = 0
	}

	// Add exhaustion from taking damage
	player.Exhaustion += 0.1

	// Send hurt animation to all players
	hurtPkt := pk.Marshal(
		packetid.ClientboundHurtAnimation,
		pk.VarInt(player.EID),
		pk.Float(0), // yaw
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(hurtPkt)
	})

	// Send damage event to all players
	damagePkt := pk.Marshal(
		packetid.ClientboundDamageEvent,
		pk.VarInt(player.EID),
		pk.VarInt(damageTypeID),
		pk.VarInt(0), // no cause entity
		pk.VarInt(0), // no direct entity
		pk.Boolean(false), // no source position
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(damagePkt)
	})

	// Play player hurt sound
	px, py, pz := player.Position()
	BroadcastSound(manager, SoundPlayerHurt, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Update health HUD
	SendSetHealth(player)

	// Update health display above player
	BroadcastHealthTag(manager, player)

	// Update scoreboard health
	UpdateHealthScore(manager, player)

	// Reduce armor durability (unbreaking enchant: skip with probability level/(level+1))
	armorChanged := false
	for _, slot := range []int{5, 6, 7, 8} {
		if player.Inventory[slot].MaxDurability > 0 && player.Inventory[slot].ID > 0 {
			unbreakLvl := enchant.GetLevel(player.Inventory[slot].Enchantments, enchant.Unbreaking)
			if unbreakLvl > 0 && rand.Int31n(unbreakLvl+1) > 0 {
				continue // unbreaking saved this durability point
			}
			player.Inventory[slot].Durability--
			if player.Inventory[slot].Durability <= 0 {
				player.Inventory[slot] = game.ItemStack{} // armor breaks
			}
			SendSlotUpdate(player, slot)
			armorChanged = true
		}
	}
	if armorChanged {
		BroadcastEquipment(manager, player)
	}

	if player.Health <= 0 {
		s.handleDeath(manager, player)
	}
}

// ApplyDamageFrom reduces player health from a named attacker and handles death with a PvP message.
func (s *SurvivalHandler) ApplyDamageFrom(manager *game.PlayerManager, player *game.Player, damage float32, damageTypeID int32, attackerName string) {
	player.LastDamageMessage = player.Name + " was slain by " + attackerName
	s.ApplyDamage(manager, player, damage, damageTypeID)
}

// handleDeath handles player death: sends combat kill and death animation.
func (s *SurvivalHandler) handleDeath(manager *game.PlayerManager, player *game.Player) {
	msg := player.Name + " died"
	if player.LastDamageMessage != "" {
		msg = player.LastDamageMessage
		player.LastDamageMessage = ""
	}
	s.handleDeathWithMessage(manager, player, msg)
}

// dropPlayerInventory drops all inventory items as entities and resets XP.
func (s *SurvivalHandler) dropPlayerInventory(manager *game.PlayerManager, player *game.Player) {
	if s.Rules != nil && s.Rules.GetKeepInventory() {
		return
	}
	px, py, pz := player.Position()
	for i := 5; i <= 45; i++ { // armor(5-8) + main(9-35) + hotbar(36-44) + offhand(45)
		item := &player.Inventory[i]
		if item.ID <= 0 || item.Count <= 0 {
			continue
		}
		// Curse of Vanishing: item is destroyed instead of dropped
		if !enchant.HasEnchant(item.Enchantments, enchant.CurseOfVanishing) {
			if s.ItemEntities != nil {
				s.ItemEntities.SpawnItem(manager, px, py+1, pz, item.ID, item.Count, 40)
			}
		}
		*item = game.ItemStack{}
	}
	// Reset XP
	player.Experience = 0
	player.ExperienceLevel = 0
	player.ExperienceTotal = 0
}

// handleDeathWithMessage handles player death with a custom death message.
func (s *SurvivalHandler) handleDeathWithMessage(manager *game.PlayerManager, player *game.Player, deathMessage string) {
	// Drop inventory before marking dead
	s.dropPlayerInventory(manager, player)

	// Clear all status effects on death
	if s.EffectMgr != nil {
		s.EffectMgr.ClearAllEffects(player)
	}

	player.Dead = true
	player.Health = 0
	if player.SessionEvents != nil {
		player.SessionEvents.OnDeath(deathMessage)
	}

	// Send combat kill to the dying player
	deathMsg := chat.Text(deathMessage)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerCombatKill,
		pk.VarInt(player.EID),
		deathMsg,
	))

	// Broadcast entity event (death animation = event 3) to all players
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(player.EID),
			pk.Byte(3), // death
		))
	})

	// Play player death sound
	px, py, pz := player.Position()
	BroadcastSound(manager, SoundPlayerDeath, SoundCategoryPlayer, px, py, pz, 1.0, 1.0)

	// Broadcast death message to all players
	broadcastMsg := chat.Message{Text: deathMessage, Color: "red"}
	deathChatPkt := pk.Marshal(
		packetid.ClientboundSystemChat,
		broadcastMsg,
		pk.Boolean(false),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(deathChatPkt)
	})

	s.logf("Player %s died: %s", player.Name, deathMessage)
}

// Kill forces a player death regardless of game mode.
func (s *SurvivalHandler) Kill(manager *game.PlayerManager, player *game.Player) {
	player.Health = 0
	s.handleDeath(manager, player)
}

// KillWithMessage forces a player death with a custom death message.
func (s *SurvivalHandler) KillWithMessage(manager *game.PlayerManager, player *game.Player, message string) {
	player.Health = 0
	s.handleDeathWithMessage(manager, player, message)
}

// HungerTick processes hunger mechanics for all players.
// Called every tick from the tick loop.
func (s *SurvivalHandler) HungerTick(manager *game.PlayerManager, tick int64) {
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}

		// Process exhaustion → saturation → food depletion
		changed := false
		for p.Exhaustion >= 4.0 {
			p.Exhaustion -= 4.0
			if p.Saturation > 0 {
				p.Saturation -= 1.0
				if p.Saturation < 0 {
					p.Saturation = 0
				}
				changed = true
			} else if p.Food > 0 {
				p.Food--
				changed = true
			}
		}

		// Natural regeneration (three tiers) — only if naturalRegeneration is enabled
		naturalRegen := s.Rules == nil || s.Rules.GetNaturalRegeneration()
		if naturalRegen && p.Health < 20 && !p.Dead {
			if p.Food >= 20 && p.Saturation > 0 {
				// Tier 1: Full food + saturation → heal every tick, consume saturation
				p.Health += 1
				if p.Health > 20 {
					p.Health = 20
				}
				p.Saturation -= 1.0
				if p.Saturation < 0 {
					p.Saturation = 0
				}
				changed = true
			} else if p.Food >= 18 && p.Saturation > 0 && tick%10 == 0 {
				// Tier 2: High food + saturation → heal every 10 ticks (0.5s)
				p.Health += 1
				if p.Health > 20 {
					p.Health = 20
				}
				p.Exhaustion += 6.0
				changed = true
			} else if p.Food >= 18 && tick%80 == 0 {
				// Tier 3: High food, no saturation → heal every 80 ticks (4s)
				p.Health += 1
				if p.Health > 20 {
					p.Health = 20
				}
				p.Exhaustion += 6.0
				changed = true
			}
		}

		// Starvation: when food = 0, damage 1 HP every 80 ticks
		// Easy: stops at 10 HP, Normal: stops at 1 HP, Hard: can kill
		if p.Food == 0 && tick%80 == 0 && p.Health > 0 {
			diff := int32(2) // default Normal
			if s.Rules != nil {
				diff = s.Rules.GetDifficulty()
			}
			canStarve := true
			switch diff {
			case 0: // Peaceful: no starvation
				canStarve = false
			case 1: // Easy: stop at 10 HP
				canStarve = p.Health > 10
			case 2: // Normal: stop at 1 HP
				canStarve = p.Health > 1
			}
			if canStarve {
				p.LastDamageMessage = p.Name + " starved to death"
				s.ApplyDamage(manager, p, 1.0, s.AttackDamageTypeID)
				changed = true
			}
		}

		if changed {
			SendSetHealth(p)
			BroadcastHealthTag(manager, p)
			UpdateHealthScore(manager, p)
		}
	})
}

// VoidDamageTick damages players below the world. Called every tick.
// minY is the world's minimum Y coordinate (e.g. -64).
func (s *SurvivalHandler) VoidDamageTick(manager *game.PlayerManager, minY int) {
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		_, y, _ := p.Position()
		if y < float64(minY)-64 {
			s.KillWithMessage(manager, p, p.Name+" fell out of the world")
		} else if y < float64(minY) {
			s.ApplyDamage(manager, p, 0.2, s.VoidDamageTypeID)
		}
	})
}

// FireTick processes fire damage for burning players every tick.
func (s *SurvivalHandler) FireTick(manager *game.PlayerManager) {
	manager.ForEach(func(p *game.Player) {
		if p.FireTicks <= 0 || p.Dead || p.IsInvulnerable() {
			return
		}
		// Fire Resistance makes player immune
		if s.EffectMgr != nil && s.EffectMgr.HasEffect(p, EffectFireResistance) {
			p.FireTicks = 0
			broadcastFireMetadata(manager, p.EID, false)
			return
		}
		p.FireTicks--
		if p.FireTicks%20 == 0 { // 1 damage per second
			p.LastDamageMessage = p.Name + " burned to death"
			s.ApplyDamage(manager, p, 1.0, s.FireDamageTypeID)
		}
		if p.FireTicks == 0 {
			broadcastFireMetadata(manager, p.EID, false)
		}
	})
}

// WaterTick handles drowning when the player's head is submerged in water.
// Also extinguishes fire when entering water, tracks InWater/OnLadder state,
// and applies swimming exhaustion.
func (s *SurvivalHandler) WaterTick(manager *game.PlayerManager, world game.World) {
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		bx := int(math.Floor(px))
		feetY := int(math.Floor(py))
		bz := int(math.Floor(pz))
		eyeY := int(math.Floor(py + 1.62))

		// Track feet-in-water state (for fall damage cancellation and swimming exhaustion)
		feetInWater := false
		if feetState, err := world.GetBlock(bx, feetY, bz); err == nil {
			if int(feetState) < len(block.StateList) && block.StateList[feetState] != nil {
				_, feetInWater = block.StateList[feetState].(block.Water)
			}
		}
		p.InWater = feetInWater

		// Track ladder/vine state (for fall damage cancellation)
		onClimbable := false
		if climbState, err := world.GetBlock(bx, feetY, bz); err == nil {
			if int(climbState) < len(block.StateList) && block.StateList[climbState] != nil {
				onClimbable = isClimbableBlock(block.StateList[climbState])
			}
		}
		p.OnLadder = onClimbable

		// Head-in-water check for drowning
		inWater := false
		state, err := world.GetBlock(bx, eyeY, bz)
		if err == nil {
			if int(state) < len(block.StateList) && block.StateList[state] != nil {
				_, inWater = block.StateList[state].(block.Water)
			}
		}

		if inWater {
			// Extinguish fire
			if p.FireTicks > 0 {
				p.FireTicks = 0
				broadcastFireMetadata(manager, p.EID, false)
			}

			// Respiration enchant extends air: each level adds ~15s (300 ticks)
			respirationLevel := int32(0)
			if len(p.Inventory) > 5 {
				respirationLevel = enchant.GetLevel(p.Inventory[5].Enchantments, enchant.Respiration)
			}

			// With Respiration, chance per tick to not consume air = level/(level+1)
			consumeAir := true
			if respirationLevel > 0 && rand.Int31n(respirationLevel+1) > 0 {
				consumeAir = false
			}

			if consumeAir {
				p.AirTicks--
			}

			if p.AirTicks <= -20 {
				p.AirTicks = 0
				p.LastDamageMessage = p.Name + " drowned"
				s.ApplyDamage(manager, p, 2.0, s.DrownDamageTypeID)
			}
		} else {
			// Restore air: +4 per tick, cap at 300
			if p.AirTicks < 300 {
				p.AirTicks += 4
				if p.AirTicks > 300 {
					p.AirTicks = 300
				}
			}
		}
	})
}

// AddSprintExhaustion adds exhaustion for sprint distance.
func AddSprintExhaustion(player *game.Player, dx, dz float64) {
	if !player.Sprinting || player.Dead || player.IsInvulnerable() {
		return
	}
	dist := math.Sqrt(dx*dx + dz*dz)
	player.Exhaustion += float32(dist) * 0.1
}

// AddSwimExhaustion adds exhaustion for swimming distance (0.01 per meter).
func AddSwimExhaustion(player *game.Player, dx, dy, dz float64) {
	if !player.InWater || player.Dead || player.IsInvulnerable() {
		return
	}
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist > 0.01 {
		player.Exhaustion += float32(dist) * 0.01
	}
}

// isClimbableBlock returns true if the block allows climbing (ladder, vine, etc.).
func isClimbableBlock(b interface{}) bool {
	switch b.(type) {
	case block.Ladder, block.Vine,
		block.TwistingVines, block.TwistingVinesPlant,
		block.WeepingVines, block.WeepingVinesPlant,
		block.CaveVines, block.CaveVinesPlant:
		return true
	}
	return false
}

// EnvironmentDamageTick handles suffocation, cactus, and magma block damage.
// Called every tick (damage applied once per second = every 20 ticks).
func (s *SurvivalHandler) EnvironmentDamageTick(manager *game.PlayerManager, world game.World, tick int64) {
	if tick%20 != 0 {
		return
	}
	manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		ix := int(math.Floor(px))
		iy := int(math.Floor(py))
		iz := int(math.Floor(pz))

		// Suffocation: check block at head height (Y + 1.5, inside the head hitbox)
		headY := int(math.Floor(py + 1.5))
		if state, err := world.GetBlock(ix, headY, iz); err == nil {
			if isSolidBlock(level.BlocksState(state)) {
				name := BlockNameFromState(int(state))
				if name != "cactus" {
					p.LastDamageMessage = p.Name + " suffocated in a wall"
					s.ApplyDamage(manager, p, 1.0, s.AttackDamageTypeID)
					return
				}
			}
		}

		// Cactus: check block at feet position and adjacent blocks
		for _, pos := range [][3]int{
			{ix, iy, iz},
			{ix + 1, iy, iz}, {ix - 1, iy, iz},
			{ix, iy, iz + 1}, {ix, iy, iz - 1},
		} {
			if state, err := world.GetBlock(pos[0], pos[1], pos[2]); err == nil {
				if int(state) < len(block.StateList) && block.StateList[state] != nil {
					if block.StateList[state].ID() == "cactus" {
						p.LastDamageMessage = p.Name + " was pricked to death"
						s.ApplyDamage(manager, p, 1.0, s.AttackDamageTypeID)
						return
					}
				}
			}
		}

		// Magma block: check block at feet (Y-1), damage if standing and not sneaking
		feetY := int(math.Floor(py)) - 1
		if !p.Sneaking {
			if state, err := world.GetBlock(ix, feetY, iz); err == nil {
				if int(state) < len(block.StateList) && block.StateList[state] != nil {
					if block.StateList[state].ID() == "magma_block" {
						p.LastDamageMessage = p.Name + " discovered the floor was lava"
						s.ApplyDamage(manager, p, 1.0, s.FireDamageTypeID)
					}
				}
			}
		}
	})
}

func (s *SurvivalHandler) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

// FoodHandler handles eating food via ServerboundUseItem with a 1.6s eating animation.
type FoodHandler struct {
	Logger     *log.Logger
	FishingMgr *FishingManager
	PotionMgr  *PotionManager
	EffectMgr  *EffectManager
}

// HandlePacket processes ServerboundUseItem to start eating.
// Returns true if the packet was handled.
func (h *FoodHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundUseItem {
		return false
	}

	var hand pk.VarInt
	var sequence pk.VarInt
	if err := p.Scan(&hand, &sequence); err != nil {
		return true
	}

	if player.GameMode != 0 || player.Dead {
		return true
	}

	// Check offhand for shield when hand=1
	slot := int(player.HeldSlot) + 36
	if int(hand) == 1 {
		slot = 45
	}
	invItem := &player.Inventory[slot]
	if invItem.ID <= 0 || invItem.Count <= 0 {
		return true
	}

	itemName := ItemNameByID(invItem.ID)

	// Shield use: start blocking
	if itemName == "shield" {
		player.Blocking = true
		return true
	}

	// Fishing rod use: cast or reel in
	if itemName == "fishing_rod" && h.FishingMgr != nil {
		h.FishingMgr.CastRod(player)
		return true
	}

	// Potion and milk bucket handling
	if h.PotionMgr != nil {
		if itemName == "milk_bucket" || itemName == "potion" {
			h.PotionMgr.HandleDrinkPotion(player, itemName)
			return true
		}
		if itemName == "splash_potion" {
			potionType := invItem.PotionType
			if potionType == "" {
				potionType = "healing"
			}
			h.PotionMgr.ThrowSplashPotion(player, potionType)
			return true
		}
		if itemName == "lingering_potion" {
			potionType := invItem.PotionType
			if potionType == "" {
				potionType = "healing"
			}
			h.PotionMgr.ThrowLingeringPotion(player, potionType)
			return true
		}
	}

	food := LookupFood(itemName)
	if food == nil {
		return true // not food
	}

	// Can't eat when food is full (except golden apples)
	if player.Food >= 20 && itemName != "golden_apple" && itemName != "enchanted_golden_apple" {
		return true
	}

	// Start eating animation — actual consumption happens in Tick after 1.6s
	player.EatingStart = time.Now()
	return true
}

// Tick processes eating for all players. Called every tick from the tick loop.
func (h *FoodHandler) Tick(manager *game.PlayerManager) {
	manager.ForEach(func(player *game.Player) {
		if player.EatingStart.IsZero() || player.Dead {
			return
		}

		elapsed := time.Since(player.EatingStart)
		if elapsed < 1600*time.Millisecond {
			// Play eating sound at ~400ms intervals (ticks 8, 16, 24 of the 32-tick eating)
			tickInEat := int(elapsed.Milliseconds() / 50)
			if tickInEat > 0 && tickInEat%8 == 0 {
				px, py, pz := player.Position()
				pitch := float32(0.9) + float32(rand.Float64())*0.2
				BroadcastSound(manager, SoundGenericEat, SoundCategoryPlayer, px, py, pz, 0.5, pitch)
			}
			return // still eating
		}

		// Eating complete — consume food
		player.EatingStart = time.Time{}

		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.ID <= 0 || invItem.Count <= 0 {
			return
		}

		itemName := ItemNameByID(invItem.ID)
		food := LookupFood(itemName)
		if food == nil {
			return
		}

		// Apply nutrition
		player.Food += food.Nutrition
		if player.Food > 20 {
			player.Food = 20
		}

		// Apply saturation (capped at food level)
		player.Saturation += food.Saturation
		if player.Saturation > float32(player.Food) {
			player.Saturation = float32(player.Food)
		}

		// Consume item
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)

		// Update health HUD
		SendSetHealth(player)

		// Play burp sound
		px, py, pz := player.Position()
		BroadcastSound(manager, SoundPlayerBurp, SoundCategoryPlayer, px, py, pz, 0.5, 1.0)

		// Apply food-specific special effects
		if h.EffectMgr != nil {
			applyFoodEffects(h.EffectMgr, manager, player, itemName)
		}

		h.logf("Player %s ate %s (food=%d, sat=%.1f)", player.Name, itemName, player.Food, player.Saturation)
	})
}

// CancelEating cancels any in-progress eating for a player.
func CancelEating(player *game.Player) {
	player.EatingStart = time.Time{}
}

// applyFoodEffects applies special effects for specific food items.
func applyFoodEffects(em *EffectManager, manager *game.PlayerManager, player *game.Player, itemName string) {
	switch itemName {
	case "golden_apple":
		em.ApplyEffect(player, EffectRegeneration, 1, 100, false) // Regen II 5s
		em.ApplyEffect(player, EffectAbsorption, 0, 2400, false)  // Absorption I 2min
	case "enchanted_golden_apple":
		em.ApplyEffect(player, EffectRegeneration, 1, 400, false)   // Regen II 20s
		em.ApplyEffect(player, EffectAbsorption, 3, 2400, false)    // Absorption IV 2min
		em.ApplyEffect(player, EffectResistance, 0, 6000, false)    // Resistance I 5min
		em.ApplyEffect(player, EffectFireResistance, 0, 6000, false) // Fire Resistance I 5min
	case "poisonous_potato":
		if rand.Float64() < 0.6 {
			em.ApplyEffect(player, EffectPoison, 0, 100, false) // Poison I 5s
		}
	case "spider_eye":
		em.ApplyEffect(player, EffectPoison, 0, 100, false) // Poison I 5s
	case "rotten_flesh":
		if rand.Float64() < 0.8 {
			em.ApplyEffect(player, EffectHunger, 0, 600, false) // Hunger I 30s
		}
	case "raw_chicken":
		if rand.Float64() < 0.3 {
			em.ApplyEffect(player, EffectHunger, 0, 600, false) // Hunger I 30s
		}
	case "pufferfish":
		em.ApplyEffect(player, EffectPoison, 1, 1200, false) // Poison II 1min
		em.ApplyEffect(player, EffectHunger, 2, 300, false)  // Hunger III 15s
		em.ApplyEffect(player, EffectNausea, 0, 300, false)  // Nausea I 15s
	case "honey_bottle":
		em.RemoveEffect(player, EffectPoison)
	}
}

func (h *FoodHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}

// isHoldingShield returns true if the player has a shield in mainhand or offhand.
func isHoldingShield(player *game.Player) bool {
	offhand := ItemNameByID(player.Inventory[45].ID)
	if offhand == "shield" {
		return true
	}
	mainhand := ItemNameByID(player.Inventory[player.HeldSlot+36].ID)
	return mainhand == "shield"
}

// reduceShieldDurability reduces shield durability by 1.
func reduceShieldDurability(player *game.Player) {
	// Check offhand first, then mainhand
	for _, slot := range []int{45, int(player.HeldSlot) + 36} {
		if ItemNameByID(player.Inventory[slot].ID) == "shield" {
			if player.Inventory[slot].MaxDurability > 0 {
				player.Inventory[slot].Durability--
				if player.Inventory[slot].Durability <= 0 {
					player.Inventory[slot] = game.ItemStack{}
					player.Blocking = false
				}
				SendSlotUpdate(player, slot)
			}
			return
		}
	}
}
