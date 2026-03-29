package handler

import (
	"log"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// MobTypeArmorStand is the entity type ID for armor_stand (26.1-snapshot-2 registry, 0-indexed).
const MobTypeArmorStand int32 = 3

// ArmorStand represents a placed armor stand entity.
type ArmorStand struct {
	EID       int32
	UUID      uuid.UUID
	X, Y, Z   float64
	Yaw       float32
	Equipment [6]game.ItemStack // 0=mainhand, 1=offhand, 2=boots, 3=leggings, 4=chestplate, 5=helmet
}

// ArmorStandManager manages armor stand entities.
type ArmorStandManager struct {
	mu      sync.Mutex
	Manager *game.PlayerManager
	ItemMgr *ItemEntityManager
	Logger  *log.Logger
	stands  map[int32]*ArmorStand // keyed by EID
}

// NewArmorStandManager creates a new ArmorStandManager.
func NewArmorStandManager(manager *game.PlayerManager, itemMgr *ItemEntityManager, logger *log.Logger) *ArmorStandManager {
	return &ArmorStandManager{
		Manager: manager,
		ItemMgr: itemMgr,
		Logger:  logger,
		stands:  make(map[int32]*ArmorStand),
	}
}

// PlaceArmorStand spawns an armor stand at the given position.
func (m *ArmorStandManager) PlaceArmorStand(player *game.Player, x, y, z float64, yaw float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	eid := m.Manager.NextEntityID()
	stand := &ArmorStand{
		EID:  eid,
		UUID: uuid.New(),
		X:    x,
		Y:    y,
		Z:    z,
		Yaw:  yaw,
	}
	m.stands[eid] = stand

	m.broadcastSpawn(stand)
	m.logf("Armor stand placed at (%.1f, %.1f, %.1f) EID=%d", x, y, z, eid)
}

// IsArmorStand returns true if the entity ID is an armor stand.
func (m *ArmorStandManager) IsArmorStand(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.stands[eid]
	return ok
}

// HandleInteract handles right-click on an armor stand.
// Swaps the player's held item with the appropriate equipment slot.
func (m *ArmorStandManager) HandleInteract(player *game.Player, eid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stand, ok := m.stands[eid]
	if !ok {
		return
	}

	heldSlot := int(player.HeldSlot) + 36
	heldItem := player.Inventory[heldSlot]
	heldName := ItemNameByID(heldItem.ID)

	// Determine which equipment slot based on the held item type
	equipSlot := equipmentSlotForItem(heldName)

	// If holding nothing, try to remove the topmost piece of equipment
	if heldItem.ID <= 0 || heldItem.Count <= 0 {
		for i := 5; i >= 0; i-- { // helmet first, then down
			if stand.Equipment[i].ID > 0 {
				player.Inventory[heldSlot] = stand.Equipment[i]
				stand.Equipment[i] = game.ItemStack{}
				SendSlotUpdate(player, heldSlot)
				m.broadcastEquipment(stand)
				return
			}
		}
		return
	}

	// Swap held item with the target equipment slot
	old := stand.Equipment[equipSlot]
	stand.Equipment[equipSlot] = heldItem
	player.Inventory[heldSlot] = old
	SendSlotUpdate(player, heldSlot)
	m.broadcastEquipment(stand)
}

// RemoveArmorStand destroys an armor stand, dropping its equipment.
func (m *ArmorStandManager) RemoveArmorStand(eid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stand, ok := m.stands[eid]
	if !ok {
		return
	}
	delete(m.stands, eid)

	// Drop all equipment
	if m.ItemMgr != nil {
		for _, item := range stand.Equipment {
			if item.ID > 0 && item.Count > 0 {
				m.ItemMgr.SpawnItem(m.Manager, stand.X, stand.Y+1, stand.Z, item.ID, item.Count, 10)
			}
		}
		// Drop the armor stand item itself
		armorStandItemID := itemIDByName("armor_stand")
		if armorStandItemID > 0 {
			m.ItemMgr.SpawnItem(m.Manager, stand.X, stand.Y+1, stand.Z, armorStandItemID, 1, 10)
		}
	}

	// Remove entity
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})

	m.logf("Armor stand removed EID=%d", eid)
}

// SendExistingStands sends all armor stands to a newly joined player.
func (m *ArmorStandManager) SendExistingStands(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, stand := range m.stands {
		m.sendSpawnTo(player, stand)
		m.sendEquipmentTo(player, stand)
	}
}

func (m *ArmorStandManager) broadcastSpawn(stand *ArmorStand) {
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(stand.EID),
		pk.UUID(stand.UUID),
		pk.VarInt(MobTypeArmorStand),
		pk.Double(stand.X),
		pk.Double(stand.Y),
		pk.Double(stand.Z),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(degToAngle(stand.Yaw)),
		pk.Angle(degToAngle(stand.Yaw)),
		pk.VarInt(0),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *ArmorStandManager) sendSpawnTo(player *game.Player, stand *ArmorStand) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(stand.EID),
		pk.UUID(stand.UUID),
		pk.VarInt(MobTypeArmorStand),
		pk.Double(stand.X),
		pk.Double(stand.Y),
		pk.Double(stand.Z),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(degToAngle(stand.Yaw)),
		pk.Angle(degToAngle(stand.Yaw)),
		pk.VarInt(0),
	))
}

func (m *ArmorStandManager) broadcastEquipment(stand *ArmorStand) {
	buf := encodeEquipmentSlots(stand.Equipment[:])
	pkt := pk.Marshal(
		packetid.ClientboundSetEquipment,
		pk.VarInt(stand.EID),
		pk.PluginMessageData(buf),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *ArmorStandManager) sendEquipmentTo(player *game.Player, stand *ArmorStand) {
	hasEquip := false
	for _, item := range stand.Equipment {
		if item.ID > 0 {
			hasEquip = true
			break
		}
	}
	if !hasEquip {
		return
	}
	buf := encodeEquipmentSlots(stand.Equipment[:])
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetEquipment,
		pk.VarInt(stand.EID),
		pk.PluginMessageData(buf),
	))
}

// encodeEquipmentSlots encodes 6 equipment slots for ClientboundSetEquipment.
// Wire slots: 0=mainhand, 1=offhand, 2=boots, 3=leggings, 4=chestplate, 5=helmet.
func encodeEquipmentSlots(items []game.ItemStack) []byte {
	var buf []byte
	for i := 0; i < 6; i++ {
		slotByte := byte(i)
		if i < 5 {
			slotByte |= 0x80 // set MSB for all except last
		}
		buf = append(buf, slotByte)
		s := items[i].ToSlot()
		buf = append(buf, encodeSlot261(s)...)
	}
	return buf
}

// equipmentSlotForItem returns the equipment slot index for an item name.
// 0=mainhand, 1=offhand, 2=boots, 3=leggings, 4=chestplate, 5=helmet
func equipmentSlotForItem(name string) int {
	switch {
	case isHelmet(name):
		return 5
	case isChestplate(name):
		return 4
	case isLeggings(name):
		return 3
	case isBoots(name):
		return 2
	default:
		return 0 // mainhand for anything else
	}
}

func isHelmet(name string) bool {
	switch name {
	case "leather_helmet", "chainmail_helmet", "iron_helmet",
		"golden_helmet", "diamond_helmet", "netherite_helmet", "turtle_helmet":
		return true
	}
	// Skulls and heads can also be worn
	switch name {
	case "player_head", "zombie_head", "skeleton_skull",
		"wither_skeleton_skull", "creeper_head", "dragon_head", "piglin_head",
		"carved_pumpkin":
		return true
	}
	return false
}

func isChestplate(name string) bool {
	switch name {
	case "leather_chestplate", "chainmail_chestplate", "iron_chestplate",
		"golden_chestplate", "diamond_chestplate", "netherite_chestplate", "elytra":
		return true
	}
	return false
}

func isLeggings(name string) bool {
	switch name {
	case "leather_leggings", "chainmail_leggings", "iron_leggings",
		"golden_leggings", "diamond_leggings", "netherite_leggings":
		return true
	}
	return false
}

func isBoots(name string) bool {
	switch name {
	case "leather_boots", "chainmail_boots", "iron_boots",
		"golden_boots", "diamond_boots", "netherite_boots":
		return true
	}
	return false
}

func (m *ArmorStandManager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}
