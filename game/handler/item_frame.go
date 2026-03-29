package handler

import (
	"log"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Entity type IDs for item frames (26.1-snapshot-2 registry, 0-indexed).
const (
	EntityTypeItemFrame     int32 = 60
	EntityTypeGlowItemFrame int32 = 47
)

// ItemFrame represents a placed item frame entity.
type ItemFrame struct {
	EID      int32
	UUID     uuid.UUID
	X, Y, Z  int    // block position the frame is attached to
	Face     int32  // facing direction (0=down, 1=up, 2=north, 3=south, 4=west, 5=east)
	Item     game.ItemStack
	Rotation int32 // 0-7
	Glowing  bool  // glow_item_frame
}

// ItemFrameManager manages item frame entities.
type ItemFrameManager struct {
	mu      sync.Mutex
	Manager *game.PlayerManager
	ItemMgr *ItemEntityManager
	Logger  *log.Logger
	frames  map[int32]*ItemFrame // keyed by EID
}

// NewItemFrameManager creates a new ItemFrameManager.
func NewItemFrameManager(manager *game.PlayerManager, itemMgr *ItemEntityManager, logger *log.Logger) *ItemFrameManager {
	return &ItemFrameManager{
		Manager: manager,
		ItemMgr: itemMgr,
		Logger:  logger,
		frames:  make(map[int32]*ItemFrame),
	}
}

// PlaceItemFrame spawns an item frame on the given block face.
func (m *ItemFrameManager) PlaceItemFrame(player *game.Player, blockX, blockY, blockZ int, face int32, glowing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	eid := m.Manager.NextEntityID()
	frame := &ItemFrame{
		EID:     eid,
		UUID:    uuid.New(),
		X:       blockX,
		Y:       blockY,
		Z:       blockZ,
		Face:    face,
		Glowing: glowing,
	}
	m.frames[eid] = frame

	m.broadcastSpawn(frame)
	m.logf("Item frame placed at (%d, %d, %d) face=%d EID=%d glowing=%v", blockX, blockY, blockZ, face, eid, glowing)
}

// IsItemFrame returns true if the entity ID is an item frame.
func (m *ItemFrameManager) IsItemFrame(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.frames[eid]
	return ok
}

// HandleInteract handles right-click on an item frame.
// If empty, inserts the held item. If has an item, rotates it.
func (m *ItemFrameManager) HandleInteract(player *game.Player, eid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	frame, ok := m.frames[eid]
	if !ok {
		return
	}

	if frame.Item.ID <= 0 || frame.Item.Count <= 0 {
		// Frame is empty — insert held item
		heldSlot := int(player.HeldSlot) + 36
		heldItem := player.Inventory[heldSlot]
		if heldItem.ID <= 0 || heldItem.Count <= 0 {
			return
		}

		// Place one item into the frame
		frame.Item = game.ItemStack{
			ID:           heldItem.ID,
			Count:        1,
			Enchantments: heldItem.Enchantments,
			DisplayName:  heldItem.DisplayName,
		}
		frame.Rotation = 0

		// Consume one item in survival
		if player.GameMode == 0 {
			player.Inventory[heldSlot].Count--
			if player.Inventory[heldSlot].Count <= 0 {
				player.Inventory[heldSlot] = game.ItemStack{}
			}
			SendSlotUpdate(player, heldSlot)
		}

		m.broadcastItemMetadata(frame)
	} else {
		// Rotate the item
		frame.Rotation = (frame.Rotation + 1) % 8
		m.broadcastItemMetadata(frame)
	}
}

// HandleAttack handles left-click (attack) on an item frame.
// Drops the contained item first; if empty, breaks the frame.
func (m *ItemFrameManager) HandleAttack(player *game.Player, eid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	frame, ok := m.frames[eid]
	if !ok {
		return
	}

	fx := float64(frame.X) + 0.5
	fy := float64(frame.Y) + 0.5
	fz := float64(frame.Z) + 0.5

	if frame.Item.ID > 0 && frame.Item.Count > 0 {
		// Pop the item out
		if m.ItemMgr != nil {
			m.ItemMgr.SpawnItem(m.Manager, fx, fy, fz, frame.Item.ID, frame.Item.Count, 10)
		}
		frame.Item = game.ItemStack{}
		frame.Rotation = 0
		m.broadcastItemMetadata(frame)
		return
	}

	// Frame is empty — break the frame itself
	delete(m.frames, eid)

	// Drop the frame item
	if m.ItemMgr != nil {
		itemName := "item_frame"
		if frame.Glowing {
			itemName = "glow_item_frame"
		}
		frameItemID := itemIDByName(itemName)
		if frameItemID > 0 {
			m.ItemMgr.SpawnItem(m.Manager, fx, fy, fz, frameItemID, 1, 10)
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

	m.logf("Item frame removed EID=%d", eid)
}

// SendExistingFrames sends all item frames to a newly joined player.
func (m *ItemFrameManager) SendExistingFrames(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, frame := range m.frames {
		m.sendSpawnTo(player, frame)
		if frame.Item.ID > 0 {
			m.sendItemMetadataTo(player, frame)
		}
	}
}

func (m *ItemFrameManager) broadcastSpawn(frame *ItemFrame) {
	entityType := EntityTypeItemFrame
	if frame.Glowing {
		entityType = EntityTypeGlowItemFrame
	}

	pkt := m.buildSpawnPacket(frame, entityType)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *ItemFrameManager) sendSpawnTo(player *game.Player, frame *ItemFrame) {
	entityType := EntityTypeItemFrame
	if frame.Glowing {
		entityType = EntityTypeGlowItemFrame
	}
	player.WritePacket(m.buildSpawnPacket(frame, entityType))
}

func (m *ItemFrameManager) buildSpawnPacket(frame *ItemFrame, entityType int32) pk.Packet {
	// Item frame position: center of the block face
	x := float64(frame.X) + 0.5
	y := float64(frame.Y) + 0.5
	z := float64(frame.Z) + 0.5

	return pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(frame.EID),
		pk.UUID(frame.UUID),
		pk.VarInt(entityType),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0),
		pk.Angle(0), // pitch
		pk.Angle(0), // yaw
		pk.Angle(0), // head yaw
		pk.VarInt(frame.Face), // data field encodes the facing direction
	)
}

// broadcastItemMetadata sends entity metadata for the item frame's displayed item and rotation.
func (m *ItemFrameManager) broadcastItemMetadata(frame *ItemFrame) {
	meta := m.buildItemMetadata(frame)
	pkt := pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(frame.EID),
		pk.PluginMessageData(meta),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *ItemFrameManager) sendItemMetadataTo(player *game.Player, frame *ItemFrame) {
	meta := m.buildItemMetadata(frame)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(frame.EID),
		pk.PluginMessageData(meta),
	))
}

// buildItemMetadata builds the entity metadata bytes for item frame.
// Index 8: ItemStack (serializer type 7)
// Index 9: VarInt rotation (serializer type 1)
func (m *ItemFrameManager) buildItemMetadata(frame *ItemFrame) []byte {
	var buf []byte

	// Index 8: displayed item (serializer type 7 = ItemStack)
	buf = append(buf, 8) // index
	buf = appendVarInt(buf, 7) // serializer: ItemStack
	slot := frame.Item.ToSlot()
	buf = append(buf, encodeSlot261(slot)...)

	// Index 9: rotation (serializer type 1 = VarInt)
	buf = append(buf, 9) // index
	buf = appendVarInt(buf, 1) // serializer: VarInt
	buf = appendVarInt(buf, int(frame.Rotation))

	// Terminator
	buf = append(buf, 0xFF)

	return buf
}

func (m *ItemFrameManager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}
