package game

import (
	"sync"

	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
	"github.com/google/uuid"
)

// Player represents a connected player with their state.
type Player struct {
	Name       string
	UUID       uuid.UUID
	EID        int32 // entity ID
	Properties []user.Property

	mu   sync.Mutex
	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround   bool

	// LoadedChunks tracks which chunks have been sent to this player.
	LoadedChunks map[ChunkPos]bool

	Conn *net.Conn

	// ViewDistance is the player's view distance in chunks.
	ViewDistance int

	// HeldSlot is the player's currently selected hotbar slot (0-8).
	HeldSlot int16
	// CreativeItem maps slot number → item ID for creative mode inventory.
	CreativeItem map[int16]int32

	// Survival mode fields
	Health     float32
	Food       int32
	Saturation float32
	Exhaustion float32
	GameMode   int32
	Dead       bool

	// Fall damage tracking
	FallStartY float64

	// Entity state
	Sneaking  bool
	Sprinting bool
	SkinParts uint8

	// Block breaking state (survival)
	Digging      bool
	DigX, DigY, DigZ int
}

// NewPlayer creates a new Player with the given connection info.
func NewPlayer(name string, id uuid.UUID, eid int32, conn *net.Conn) *Player {
	return &Player{
		Name:         name,
		UUID:         id,
		EID:          eid,
		Conn:         conn,
		LoadedChunks: make(map[ChunkPos]bool),
		ViewDistance:  10,
		CreativeItem: make(map[int16]int32),
		Health:       20,
		Food:         20,
		Saturation:   5,
		FallStartY:   -999,
		SkinParts:    0x7F,
	}
}

// Position returns the player's current position (thread-safe).
func (p *Player) Position() (x, y, z float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.X, p.Y, p.Z
}

// SetPosition updates the player's position (thread-safe).
func (p *Player) SetPosition(x, y, z float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.X = x
	p.Y = y
	p.Z = z
}

// Rotation returns the player's current rotation (thread-safe).
func (p *Player) Rotation() (yaw, pitch float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Yaw, p.Pitch
}

// SetRotation updates the player's rotation (thread-safe).
func (p *Player) SetRotation(yaw, pitch float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Yaw = yaw
	p.Pitch = pitch
}

// ChunkPos returns the chunk the player is currently in.
func (p *Player) ChunkPos() ChunkPos {
	p.mu.Lock()
	defer p.mu.Unlock()
	return BlockToChunk(int(p.X), int(p.Z))
}

// WritePacket sends a packet to this player's connection.
func (p *Player) WritePacket(pkt pk.Packet) error {
	return p.Conn.WritePacket(pkt)
}

// SetHeldSlot sets the player's selected hotbar slot (thread-safe).
func (p *Player) SetHeldSlot(slot int16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.HeldSlot = slot
}

// SetCreativeSlot stores an item ID for a creative inventory slot (thread-safe).
// Pass itemID <= 0 to clear the slot.
func (p *Player) SetCreativeSlot(slot int16, itemID int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if itemID <= 0 {
		delete(p.CreativeItem, slot)
	} else {
		p.CreativeItem[slot] = itemID
	}
}

// HeldItemID returns the item ID of the currently held item (thread-safe).
// Returns 0 if no item is held.
func (p *Player) HeldItemID() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Hotbar slots are inventory slots 36-44
	return p.CreativeItem[p.HeldSlot+36]
}
