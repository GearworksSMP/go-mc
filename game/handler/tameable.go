package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Tameable mob type IDs.
const (
	MobTypeWolf         int32 = 148
	MobTypeCat          int32 = 21
	MobTypeHorse        int32 = 66
	MobTypeParrot       int32 = 98
	MobTypeLlama        int32 = 76
	MobTypeTraderLlama  int32 = 130
)

// TameableMobData holds taming and ownership state for a mob.
type TameableMobData struct {
	OwnerUUID  uuid.UUID
	Tamed      bool
	Sitting    bool
	CollarColor int32 // wolf collar color (0-15 dye colors, default 14=red)

	// Cat fields
	CatVariant int32 // 0-10 for cat skin variants

	// Horse fields
	HorseVariant int32 // combined color+marking
	Temper       int32 // 0-100, tames at 100
	HasSaddle    bool
	HorseSpeed   float64
	HorseJump    float64
	ArmorItemID  int32 // horse armor item ID (0 = none)

	// Parrot fields
	ParrotVariant int32 // 0-4 for parrot colors

	// Llama fields
	HasChest       bool            // llama has attached chest
	CarpetColor    int32           // llama carpet dye color (-1 = none)
	LlamaStrength  int32           // 1-5, determines chest capacity (strength*3 slots)
	ChestInventory []game.ItemStack // llama chest contents
}

// isTameableType returns true if the mob type can be tamed.
func isTameableType(typeID int32) bool {
	return typeID == MobTypeWolf || typeID == MobTypeCat ||
		typeID == MobTypeHorse || typeID == MobTypeParrot ||
		typeID == MobTypeLlama || typeID == MobTypeTraderLlama
}

// isLlamaType returns true if the mob type is a llama or trader llama.
func isLlamaType(typeID int32) bool {
	return typeID == MobTypeLlama || typeID == MobTypeTraderLlama
}

// isTamingItem returns true if the item can be used to tame the given mob type.
func isTamingItem(typeID int32, itemName string) bool {
	switch typeID {
	case MobTypeWolf:
		return itemName == "bone"
	case MobTypeCat:
		return itemName == "cod" || itemName == "salmon" ||
			itemName == "raw_cod" || itemName == "raw_salmon"
	case MobTypeParrot:
		return itemName == "wheat_seeds" || itemName == "melon_seeds" ||
			itemName == "pumpkin_seeds" || itemName == "beetroot_seeds"
	}
	return false
}

// isHorseTemperItem returns the temper boost for feeding a horse, or 0 if not food.
func isHorseTemperItem(itemName string) int32 {
	switch itemName {
	case "sugar":
		return 3
	case "wheat":
		return 3
	case "apple":
		return 3
	case "golden_carrot":
		return 5
	case "golden_apple", "enchanted_golden_apple":
		return 10
	}
	return 0
}

// TryTame attempts to tame a mob when a player right-clicks with the correct item.
// Returns true if the interaction was handled (even if taming failed).
func (m *MobManager) TryTame(player *game.Player, targetEID int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	mob, ok := m.Mobs[targetEID]
	if !ok || mob.Health <= 0 {
		return false
	}

	if !isTameableType(mob.TypeID) {
		return false
	}

	// Already tamed — handle sit toggle or horse/llama mounting
	if mob.TameData != nil && mob.TameData.Tamed {
		if mob.TameData.OwnerUUID == player.UUID {
			if mob.TypeID == MobTypeHorse {
				return m.handleHorseInteract(player, mob)
			}
			if isLlamaType(mob.TypeID) {
				return m.handleLlamaInteract(player, mob)
			}
			// Toggle sitting for wolf/cat/parrot
			mob.TameData.Sitting = !mob.TameData.Sitting
			m.broadcastTameableMetadata(mob)
			return true
		}
		return false // not owner
	}

	heldSlot := int(player.HeldSlot) + 36
	heldItem := &player.Inventory[heldSlot]
	if heldItem.ID <= 0 {
		// Horse/llama: mount attempt even without items
		if mob.TypeID == MobTypeHorse {
			return m.handleHorseMountAttempt(player, mob)
		}
		if isLlamaType(mob.TypeID) {
			return m.handleLlamaMountAttempt(player, mob)
		}
		return false
	}

	heldName := ItemNameByID(heldItem.ID)

	// Llama: feed hay bale to increase temper
	if isLlamaType(mob.TypeID) {
		if heldName == "hay_block" {
			if mob.TameData == nil {
				mob.TameData = &TameableMobData{
					CarpetColor:   -1,
					LlamaStrength: 1 + rand.Int31n(5),
				}
			}
			mob.TameData.Temper += 10
			if player.GameMode == 0 {
				consumeOneItem(player, heldSlot)
			}
			if mob.TameData.Temper >= 100 {
				mob.TameData.Tamed = true
				mob.TameData.OwnerUUID = player.UUID
				m.broadcastTameEvent(mob, true)
			}
			return true
		}
		return m.handleLlamaMountAttempt(player, mob)
	}

	// Horse: feed to increase temper
	if mob.TypeID == MobTypeHorse {
		boost := isHorseTemperItem(heldName)
		if boost > 0 {
			if mob.TameData == nil {
				mob.TameData = &TameableMobData{
					HorseSpeed: 0.1 + rand.Float64()*0.05,
					HorseJump:  0.4 + rand.Float64()*0.3,
				}
			}
			mob.TameData.Temper += boost
			// Consume item
			if player.GameMode == 0 {
				heldItem.Count--
				if heldItem.Count <= 0 {
					*heldItem = game.ItemStack{}
				}
				SendSlotUpdate(player, heldSlot)
			}
			// Check if tamed
			if mob.TameData.Temper >= 100 {
				mob.TameData.Tamed = true
				mob.TameData.OwnerUUID = player.UUID
				mob.TameData.HorseVariant = rand.Int31n(7)*256 + rand.Int31n(5)
				m.broadcastTameEvent(mob, true)
			}
			return true
		}
		return m.handleHorseMountAttempt(player, mob)
	}

	// Wolf/Cat/Parrot: taming with specific items
	if !isTamingItem(mob.TypeID, heldName) {
		return false
	}

	// Consume item
	if player.GameMode == 0 {
		heldItem.Count--
		if heldItem.Count <= 0 {
			*heldItem = game.ItemStack{}
		}
		SendSlotUpdate(player, heldSlot)
	}

	// 33% chance to tame
	if rand.Float64() < 0.33 {
		if mob.TameData == nil {
			mob.TameData = &TameableMobData{}
		}
		mob.TameData.Tamed = true
		mob.TameData.OwnerUUID = player.UUID
		mob.TameData.CollarColor = 14 // red

		switch mob.TypeID {
		case MobTypeCat:
			mob.TameData.CatVariant = rand.Int31n(11)
			mob.Health = 10
			mob.MaxHealth = 10
		case MobTypeWolf:
			mob.Health = 20
			mob.MaxHealth = 20
		case MobTypeParrot:
			mob.TameData.ParrotVariant = rand.Int31n(5)
		}

		m.broadcastTameEvent(mob, true)
	} else {
		m.broadcastTameEvent(mob, false)
	}
	return true
}

// handleHorseMountAttempt handles mounting an untamed horse (increases temper).
func (m *MobManager) handleHorseMountAttempt(player *game.Player, mob *Mob) bool {
	if mob.TameData == nil {
		mob.TameData = &TameableMobData{
			HorseSpeed: 0.1 + rand.Float64()*0.05,
			HorseJump:  0.4 + rand.Float64()*0.3,
		}
	}

	// Each mount attempt adds 5 temper
	mob.TameData.Temper += 5
	if mob.TameData.Temper >= 100 {
		mob.TameData.Tamed = true
		mob.TameData.OwnerUUID = player.UUID
		mob.TameData.HorseVariant = rand.Int31n(7)*256 + rand.Int31n(5)
		m.broadcastTameEvent(mob, true)
	} else {
		// Buck the player off (just show angry particles)
		m.broadcastTameEvent(mob, false)
	}
	return true
}

// isHorseArmor returns true if the item name is a horse armor type.
func isHorseArmor(name string) bool {
	return name == "iron_horse_armor" || name == "golden_horse_armor" ||
		name == "diamond_horse_armor" || name == "leather_horse_armor"
}

// horseArmorProtection returns the damage reduction for a horse armor item name.
func horseArmorProtection(name string) float32 {
	switch name {
	case "leather_horse_armor":
		return 3
	case "iron_horse_armor":
		return 5
	case "golden_horse_armor":
		return 7
	case "diamond_horse_armor":
		return 11
	}
	return 0
}

// handleHorseInteract handles right-clicking a tamed horse (mount if saddled, add saddle/armor).
func (m *MobManager) handleHorseInteract(player *game.Player, mob *Mob) bool {
	heldSlot := int(player.HeldSlot) + 36
	heldItem := &player.Inventory[heldSlot]
	if heldItem.ID > 0 {
		heldName := ItemNameByID(heldItem.ID)
		if heldName == "saddle" && !mob.TameData.HasSaddle {
			mob.TameData.HasSaddle = true
			if player.GameMode == 0 {
				heldItem.Count--
				if heldItem.Count <= 0 {
					*heldItem = game.ItemStack{}
				}
				SendSlotUpdate(player, heldSlot)
			}
			return true
		}
		if isHorseArmor(heldName) && mob.TameData.ArmorItemID == 0 {
			mob.TameData.ArmorItemID = heldItem.ID
			if player.GameMode == 0 {
				consumeOneItem(player, heldSlot)
			}
			m.broadcastHorseEquipment(mob)
			return true
		}
	}
	// Mount if saddled
	if mob.TameData.HasSaddle {
		player.RidingEntityEID = mob.EID
		m.broadcastMount(player, mob)
	}
	return true
}

// broadcastHorseEquipment sends horse body armor as equipment slot 6 (body).
func (m *MobManager) broadcastHorseEquipment(mob *Mob) {
	item := game.ItemStack{}
	if mob.TameData != nil && mob.TameData.ArmorItemID > 0 {
		item = game.ItemStack{ID: mob.TameData.ArmorItemID, Count: 1}
	}
	// Equipment slot 6 is the body slot for horses
	var buf []byte
	slotByte := byte(6) // body slot, last entry so no MSB
	buf = append(buf, slotByte)
	s := item.ToSlot()
	buf = append(buf, encodeSlot261(s)...)

	pkt := pk.Marshal(
		packetid.ClientboundSetEquipment,
		pk.VarInt(mob.EID),
		pk.PluginMessageData(buf),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// handleLlamaInteract handles right-clicking a tamed llama (chest, carpet, ride).
func (m *MobManager) handleLlamaInteract(player *game.Player, mob *Mob) bool {
	heldSlot := int(player.HeldSlot) + 36
	heldItem := &player.Inventory[heldSlot]

	// TODO: open llama chest UI when shift+right-clicking
	if player.Sneaking && mob.TameData.HasChest {
		return true
	}

	if heldItem.ID > 0 {
		heldName := ItemNameByID(heldItem.ID)

		if heldName == "chest" && !mob.TameData.HasChest {
			mob.TameData.HasChest = true
			capacity := int(mob.TameData.LlamaStrength * 3)
			if capacity < 3 {
				capacity = 3
			}
			if capacity > 15 {
				capacity = 15
			}
			mob.TameData.ChestInventory = make([]game.ItemStack, capacity)
			if player.GameMode == 0 {
				consumeOneItem(player, heldSlot)
			}
			m.broadcastLlamaMetadata(mob)
			return true
		}

		carpetColor := carpetNameToColor(heldName)
		if carpetColor >= 0 {
			mob.TameData.CarpetColor = carpetColor
			if player.GameMode == 0 {
				consumeOneItem(player, heldSlot)
			}
			m.broadcastLlamaMetadata(mob)
			return true
		}
	}

	// Llamas can be mounted but not steered by the player
	player.RidingEntityEID = mob.EID
	m.broadcastMount(player, mob)
	return true
}

// handleLlamaMountAttempt handles mounting an untamed llama.
func (m *MobManager) handleLlamaMountAttempt(player *game.Player, mob *Mob) bool {
	if mob.TameData == nil {
		mob.TameData = &TameableMobData{
			CarpetColor:   -1,
			LlamaStrength: 1 + rand.Int31n(5),
		}
	}
	mob.TameData.Temper += 5
	if mob.TameData.Temper >= 100 {
		mob.TameData.Tamed = true
		mob.TameData.OwnerUUID = player.UUID
		m.broadcastTameEvent(mob, true)
	} else {
		m.broadcastTameEvent(mob, false)
	}
	return true
}

// carpetNameToColor returns the dye color index for a carpet item name, or -1 if not a carpet.
func carpetNameToColor(name string) int32 {
	switch name {
	case "white_carpet":
		return 0
	case "orange_carpet":
		return 1
	case "magenta_carpet":
		return 2
	case "light_blue_carpet":
		return 3
	case "yellow_carpet":
		return 4
	case "lime_carpet":
		return 5
	case "pink_carpet":
		return 6
	case "gray_carpet":
		return 7
	case "light_gray_carpet":
		return 8
	case "cyan_carpet":
		return 9
	case "purple_carpet":
		return 10
	case "blue_carpet":
		return 11
	case "brown_carpet":
		return 12
	case "green_carpet":
		return 13
	case "red_carpet":
		return 14
	case "black_carpet":
		return 15
	}
	return -1
}

// broadcastLlamaMetadata sends llama-specific metadata (chest and carpet).
func (m *MobManager) broadcastLlamaMetadata(mob *Mob) {
	if mob.TameData == nil {
		return
	}
	var w MetadataWriter
	// Llama entity data: index 19 = has chest (Boolean), index 20 = strength (Int)
	w.WriteBoolean(19, mob.TameData.HasChest)
	// Carpet color: index 21 (Int, -1 for none)
	w.writeIndex(21, metaSerializerInt)
	writeVarIntBuf(&w.buf, mob.TameData.CarpetColor)
	data := w.Bytes()
	m.Manager.ForEach(func(p *game.Player) {
		SendEntityMetadata(p, mob.EID, data)
	})
}

// tickLlama runs llama AI: follow owner or wander.
func (m *MobManager) tickLlama(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TameData == nil || !mob.TameData.Tamed {
		m.tickWander(mob, tick)
		return
	}

	var owner *game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if p.UUID == mob.TameData.OwnerUUID {
			owner = p
		}
	})

	if owner == nil || owner.Dead {
		m.tickWander(mob, tick)
		return
	}

	ox, _, oz := owner.Position()
	dx := ox - mob.X
	dz := oz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 40 {
		mob.X = ox + (rand.Float64()-0.5)*2
		mob.Z = oz + (rand.Float64()-0.5)*2
		mob.Y = float64(m.findSurfaceY(int(mob.X), int(mob.Z)))
		m.broadcastMobTeleport(mob)
		return
	}

	if dist > 4.0 {
		nx := dx / dist * mob.Speed * 1.2
		nz := dz / dist * mob.Speed * 1.2
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
	} else {
		m.tickWander(mob, tick)
	}
}

// horseArmorDamageReduction returns the damage to subtract for a horse with armor.
func horseArmorDamageReduction(mob *Mob) float32 {
	if mob.TameData == nil || mob.TameData.ArmorItemID == 0 {
		return 0
	}
	name := ItemNameByID(mob.TameData.ArmorItemID)
	return horseArmorProtection(name)
}

// broadcastTameEvent broadcasts tame success/fail particles.
func (m *MobManager) broadcastTameEvent(mob *Mob, success bool) {
	event := int8(6) // smoke (fail)
	if success {
		event = 7 // hearts (success)
	}
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundEntityEvent,
			pk.Int(mob.EID),
			pk.Byte(event),
		))
	})
}

// broadcastMount sends mount packet for a player on a mob.
func (m *MobManager) broadcastMount(player *game.Player, mob *Mob) {
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pk.Marshal(
			packetid.ClientboundSetPassengers,
			pk.VarInt(mob.EID),
			pk.VarInt(1),
			pk.VarInt(player.EID),
		))
	})
}

// broadcastTameableMetadata sends tameable mob metadata (sitting flag).
func (m *MobManager) broadcastTameableMetadata(mob *Mob) {
	if mob.TameData == nil {
		return
	}
	var w MetadataWriter
	// Index 17 for tameable entity flags (bit 0 = sitting, bit 2 = tamed)
	var flags int8
	if mob.TameData.Sitting {
		flags |= 0x01
	}
	if mob.TameData.Tamed {
		flags |= 0x04
	}
	w.WriteByte(17, flags)
	data := w.Bytes()
	m.Manager.ForEach(func(p *game.Player) {
		SendEntityMetadata(p, mob.EID, data)
	})
}

// tickWolf runs wolf AI: follow owner, attack owner's targets.
func (m *MobManager) tickWolf(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TameData == nil || !mob.TameData.Tamed {
		m.tickWander(mob, tick)
		return
	}

	if mob.TameData.Sitting {
		return // sitting wolves don't move
	}

	// Find owner
	var owner *game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if p.UUID == mob.TameData.OwnerUUID {
			owner = p
		}
	})

	if owner == nil || owner.Dead {
		m.tickWander(mob, tick)
		return
	}

	// Follow owner if too far
	ox, _, oz := owner.Position()
	dx := ox - mob.X
	dz := oz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	// Teleport to owner if very far
	if dist > 40 {
		mob.X = ox + (rand.Float64()-0.5)*2
		mob.Z = oz + (rand.Float64()-0.5)*2
		mob.Y = float64(m.findSurfaceY(int(mob.X), int(mob.Z)))
		m.broadcastMobTeleport(mob)
		return
	}

	// If owner has a target (mob that hit them), attack it
	if mob.Target != nil {
		// Check if target mob still exists and alive
		targetMob, ok := m.Mobs[mob.Target.EID]
		if ok && targetMob != nil {
			// This was a mob target from handleOwnerDamaged
			// Move toward mob target and attack
			mdx := targetMob.X - mob.X
			mdz := targetMob.Z - mob.Z
			mdist := math.Sqrt(mdx*mdx + mdz*mdz)
			if mdist > 1.5 {
				nx := mdx / mdist * mob.Speed * 1.5
				nz := mdz / mdist * mob.Speed * 1.5
				m.tryMove(mob, nx, nz)
				mob.Yaw = float32(math.Atan2(-mdx, mdz) * 180 / math.Pi)
			} else if mob.AttackCooldown <= 0 {
				mob.AttackCooldown = 20
				targetMob.Health -= 4
				if targetMob.Health <= 0 {
					targetMob.Health = 0
					m.killMob(targetMob, nil)
				} else {
					BroadcastSound(m.Manager, MobHurtSound(targetMob.TypeID), MobSoundCategory(targetMob.TypeID),
						targetMob.X, targetMob.Y, targetMob.Z, 1.0, 1.0)
				}
			}
			m.broadcastMobMove(mob)
			return
		}
		mob.Target = nil
	}

	// Follow owner if > 4 blocks away
	if dist > 4.0 {
		nx := dx / dist * mob.Speed * 1.2
		nz := dz / dist * mob.Speed * 1.2
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
	} else {
		m.tickWander(mob, tick)
	}
}

// tickCat runs cat AI: follow owner, scare creepers.
func (m *MobManager) tickCat(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TameData == nil || !mob.TameData.Tamed {
		m.tickWander(mob, tick)
		return
	}

	if mob.TameData.Sitting {
		return
	}

	var owner *game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if p.UUID == mob.TameData.OwnerUUID {
			owner = p
		}
	})

	if owner == nil || owner.Dead {
		m.tickWander(mob, tick)
		return
	}

	ox, _, oz := owner.Position()
	dx := ox - mob.X
	dz := oz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 40 {
		mob.X = ox + (rand.Float64()-0.5)*2
		mob.Z = oz + (rand.Float64()-0.5)*2
		mob.Y = float64(m.findSurfaceY(int(mob.X), int(mob.Z)))
		m.broadcastMobTeleport(mob)
		return
	}

	// Scare away creepers and phantoms within 16 blocks
	for _, other := range m.Mobs {
		if other.Health <= 0 {
			continue
		}
		if other.TypeID != MobTypeCreeper && other.TypeID != MobTypePhantom {
			continue
		}
		cdx := other.X - mob.X
		cdz := other.Z - mob.Z
		if cdx*cdx+cdz*cdz < 256 { // 16 block radius
			// Make creeper/phantom flee
			fdist := math.Sqrt(cdx*cdx + cdz*cdz)
			if fdist > 0.5 {
				other.FleeX = other.X + cdx/fdist*16
				other.FleeZ = other.Z + cdz/fdist*16
				other.FleeTicks = 40
			}
		}
	}

	if dist > 4.0 {
		nx := dx / dist * mob.Speed * 1.2
		nz := dz / dist * mob.Speed * 1.2
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
	} else {
		m.tickWander(mob, tick)
	}
}

// tickHorse runs horse AI: follow rider input or wander.
func (m *MobManager) tickHorse(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TameData != nil && mob.TameData.Tamed && mob.TameData.HasSaddle {
		// Check if any player is riding this horse
		var rider *game.Player
		m.Manager.ForEach(func(p *game.Player) {
			if p.RidingEntityEID == mob.EID {
				rider = p
			}
		})
		if rider != nil {
			// Horse moves with the rider's input (handled via movement packets)
			// For now, horse position follows rider
			px, py, pz := rider.Position()
			mob.X = px
			mob.Y = py
			mob.Z = pz
			m.broadcastMobMove(mob)
			return
		}
	}

	m.tickWander(mob, tick)
}

// tickParrot runs parrot AI: follow owner, perch on shoulder.
func (m *MobManager) tickParrot(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.TameData == nil || !mob.TameData.Tamed {
		m.tickWander(mob, tick)
		return
	}

	if mob.TameData.Sitting {
		return
	}

	var owner *game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if p.UUID == mob.TameData.OwnerUUID {
			owner = p
		}
	})

	if owner == nil || owner.Dead {
		m.tickWander(mob, tick)
		return
	}

	ox, _, oz := owner.Position()
	dx := ox - mob.X
	dz := oz - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist > 40 {
		mob.X = ox + (rand.Float64()-0.5)*2
		mob.Z = oz + (rand.Float64()-0.5)*2
		mob.Y = float64(m.findSurfaceY(int(mob.X), int(mob.Z)))
		m.broadcastMobTeleport(mob)
		return
	}

	if dist > 3.0 {
		nx := dx / dist * mob.Speed * 1.5
		nz := dz / dist * mob.Speed * 1.5
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
	} else {
		m.tickWander(mob, tick)
	}
}

// WolfRetaliateForOwner is called when a player takes damage from a mob.
// Any tamed wolf owned by the player will target the attacker mob.
func (m *MobManager) WolfRetaliateForOwner(ownerUUID uuid.UUID, attackerEID int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the attacker mob to use as target
	attackerMob, ok := m.Mobs[attackerEID]
	if !ok || attackerMob.Health <= 0 {
		return
	}

	for _, mob := range m.Mobs {
		if mob.TypeID != MobTypeWolf || mob.Health <= 0 {
			continue
		}
		if mob.TameData == nil || !mob.TameData.Tamed || mob.TameData.Sitting {
			continue
		}
		if mob.TameData.OwnerUUID != ownerUUID {
			continue
		}
		// Set wolf target to attacker - use a "virtual player" with the mob EID
		// We'll check mob-vs-mob in tickWolf
		mob.Target = &game.Player{EID: attackerEID}
	}
}
