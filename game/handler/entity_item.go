package handler

import (
	"bytes"
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// DroppedItem is an item entity on the ground.
type DroppedItem struct {
	EID         int32
	ItemID      int32
	Count       int32
	X, Y, Z     float64
	SpawnTick   int64
	PickupDelay int64 // ticks before pickup allowed
}

// ItemEntityManager tracks all dropped item entities in the world.
type ItemEntityManager struct {
	Manager *game.PlayerManager
	Items   map[int32]*DroppedItem // EID → item
	mu      sync.Mutex
	tick    int64
}

// NewItemEntityManager creates a new ItemEntityManager.
func NewItemEntityManager(manager *game.PlayerManager) *ItemEntityManager {
	return &ItemEntityManager{
		Manager: manager,
		Items:   make(map[int32]*DroppedItem),
	}
}

// SpawnItem creates a dropped item entity at the given position.
// pickupDelay is in ticks (20 ticks = 1 second).
func (m *ItemEntityManager) SpawnItem(manager *game.PlayerManager, x, y, z float64, itemID, count int32, pickupDelay int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	eid := manager.NextEntityID()
	item := &DroppedItem{
		EID:         eid,
		ItemID:      itemID,
		Count:       count,
		X:           x,
		Y:           y,
		Z:           z,
		SpawnTick:   m.tick,
		PickupDelay: pickupDelay,
	}
	m.Items[eid] = item

	// Send AddEntity to all players
	entityUUID := uuid.New()
	addPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(55),          // entity type = Item
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0),     // LpVec3 zero velocity
		pk.Angle(0),            // xRot
		pk.Angle(0),            // yRot
		pk.Angle(0),            // yHeadRot
		pk.VarInt(0),           // data
	)

	// Build metadata with item stack (index 8, serializer 7 = ItemStack)
	metaData := buildItemMetadata(itemID, count)

	manager.ForEach(func(p *game.Player) {
		p.WritePacket(addPkt)
		SendEntityMetadata(p, eid, metaData)
	})
}

// SendExistingItems sends all current dropped items to a newly joined player.
func (m *ItemEntityManager) SendExistingItems(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.Items {
		entityUUID := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(item.EID),
			pk.UUID(entityUUID),
			pk.VarInt(55),
			pk.Double(item.X),
			pk.Double(item.Y),
			pk.Double(item.Z),
			pk.UnsignedByte(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.Angle(0),
			pk.VarInt(0),
		))
		metaData := buildItemMetadata(item.ItemID, item.Count)
		SendEntityMetadata(player, item.EID, metaData)
	}
}

// Tick processes item entity logic: pickup and despawn.
func (m *ItemEntityManager) Tick(tick int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tick = tick

	var toRemove []int32

	for eid, item := range m.Items {
		// Despawn after 5 minutes (6000 ticks)
		if tick-item.SpawnTick > 6000 {
			toRemove = append(toRemove, eid)
			m.broadcastRemoveEntity(eid)
			continue
		}

		// Skip if pickup delay hasn't elapsed
		if tick-item.SpawnTick < item.PickupDelay {
			continue
		}

		// Check if any player is close enough to pick up
		var collector *game.Player
		m.Manager.ForEach(func(p *game.Player) {
			if collector != nil || p.Dead || p.GameMode == 3 {
				return
			}
			px, py, pz := p.Position()
			dx := px - item.X
			dy := (py + 0.9) - item.Y // player eye height approximation for center
			dz := pz - item.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist <= 1.5 {
				collector = p
			}
		})

		if collector == nil {
			continue
		}

		// Add to inventory
		slot := collector.Inventory.AddItem(item.ItemID, item.Count)
		if slot < 0 {
			continue // inventory full
		}
		SendSlotUpdate(collector, slot)

		// Send pickup animation to all players
		pickupPkt := pk.Marshal(
			packetid.ClientboundTakeItemEntity,
			pk.VarInt(eid),
			pk.VarInt(collector.EID),
			pk.VarInt(item.Count),
		)
		removePkt := pk.Marshal(
			packetid.ClientboundRemoveEntities,
			pk.VarInt(1),
			pk.VarInt(eid),
		)
		m.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pickupPkt)
			p.WritePacket(removePkt)
		})

		toRemove = append(toRemove, eid)
	}

	for _, eid := range toRemove {
		delete(m.Items, eid)
	}
}

func (m *ItemEntityManager) broadcastRemoveEntity(eid int32) {
	pkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// buildItemMetadata creates entity metadata bytes with the item stack at index 8.
// Serializer 7 = ItemStack (Slot).
func buildItemMetadata(itemID, count int32) []byte {
	var buf bytes.Buffer
	// Index 8, serializer 7 (ItemStack/Slot)
	buf.WriteByte(8)
	writeVarIntBuf(&buf, 7) // serializer: Slot

	// Write Slot261 inline: VarInt(count), VarInt(itemID), VarInt(0 added), VarInt(0 removed)
	writeVarIntBuf(&buf, count)
	writeVarIntBuf(&buf, itemID)
	writeVarIntBuf(&buf, 0) // no added components
	writeVarIntBuf(&buf, 0) // no removed components

	// Terminator
	buf.WriteByte(0xFF)
	return buf.Bytes()
}
