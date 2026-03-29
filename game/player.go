package game

import (
	"io"
	"sync"
	"time"

	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
	"github.com/google/uuid"
)

// LodestoneTarget stores the position a lodestone compass points at.
type LodestoneTarget struct {
	Dimension string // e.g. "minecraft:overworld"
	X, Y, Z   int
}

// Slot261 is a 26.1 wire-format inventory slot.
type Slot261 struct {
	Count         int32
	ItemID        int32
	Durability    int32            // remaining durability (0 = no durability info)
	MaxDurability int32            // max durability (0 = not a tool)
	Lodestone     *LodestoneTarget // non-nil if this is a lodestone compass
}

// WriteTo implements pk.FieldEncoder for the 26.1 slot format:
// VarInt(count); if count > 0: VarInt(itemID) + VarInt(addedCount) + components + VarInt(0)
func (s Slot261) WriteTo(w io.Writer) (n int64, err error) {
	nn, err := pk.VarInt(s.Count).WriteTo(w)
	n += nn
	if err != nil || s.Count <= 0 {
		return
	}
	nn, err = pk.VarInt(s.ItemID).WriteTo(w)
	n += nn
	if err != nil {
		return
	}

	// Count how many components to add
	damageUsed := s.MaxDurability - s.Durability
	hasDamage := s.MaxDurability > 0 && damageUsed > 0
	hasLodestone := s.Lodestone != nil

	var compCount int32
	if hasDamage {
		compCount++
	}
	if hasLodestone {
		compCount++
	}

	nn, err = pk.VarInt(compCount).WriteTo(w)
	n += nn
	if err != nil {
		return
	}

	if hasDamage {
		nn, err = pk.VarInt(3).WriteTo(w) // minecraft:damage = index 3
		n += nn
		if err != nil {
			return
		}
		nn, err = pk.VarInt(damageUsed).WriteTo(w)
		n += nn
		if err != nil {
			return
		}
	}

	if hasLodestone {
		nn, err = pk.VarInt(44).WriteTo(w) // minecraft:lodestone_tracker = index 44
		n += nn
		if err != nil {
			return
		}
		// HasGlobalPosition = true
		nn, err = pk.Boolean(true).WriteTo(w)
		n += nn
		if err != nil {
			return
		}
		// Dimension identifier
		nn, err = pk.Identifier(s.Lodestone.Dimension).WriteTo(w)
		n += nn
		if err != nil {
			return
		}
		// Position
		nn, err = pk.Position{X: s.Lodestone.X, Y: s.Lodestone.Y, Z: s.Lodestone.Z}.WriteTo(w)
		n += nn
		if err != nil {
			return
		}
		// Tracked = true
		nn, err = pk.Boolean(true).WriteTo(w)
		n += nn
		if err != nil {
			return
		}
	}

	nn, err = pk.VarInt(0).WriteTo(w) // removed components
	n += nn
	return
}

// ReadFrom implements pk.FieldDecoder for the 26.1 slot format.
func (s *Slot261) ReadFrom(r io.Reader) (n int64, err error) {
	var count pk.VarInt
	nn, err := count.ReadFrom(r)
	n += nn
	if err != nil || count <= 0 {
		s.Count = 0
		s.ItemID = 0
		return
	}
	s.Count = int32(count)
	var itemID, addComp, remComp pk.VarInt
	nn, err = itemID.ReadFrom(r)
	n += nn
	if err != nil {
		return
	}
	s.ItemID = int32(itemID)
	nn, err = addComp.ReadFrom(r)
	n += nn
	if err != nil {
		return
	}
	// Skip added component data (each is VarInt typeID + type-specific data)
	for i := 0; i < int(addComp); i++ {
		var typeID pk.VarInt
		nn, err = typeID.ReadFrom(r)
		n += nn
		if err != nil {
			return
		}
		switch typeID {
		case 3: // minecraft:damage — VarInt value
			var val pk.VarInt
			nn, err = val.ReadFrom(r)
			n += nn
			if err != nil {
				return
			}
		case 44: // minecraft:lodestone_tracker
			var hasPos pk.Boolean
			nn, err = hasPos.ReadFrom(r)
			n += nn
			if err != nil {
				return
			}
			if hasPos {
				var dim pk.Identifier
				var pos pk.Position
				nn, err = dim.ReadFrom(r)
				n += nn
				if err != nil {
					return
				}
				nn, err = pos.ReadFrom(r)
				n += nn
				if err != nil {
					return
				}
				s.Lodestone = &LodestoneTarget{
					Dimension: string(dim),
					X:         pos.X,
					Y:         pos.Y,
					Z:         pos.Z,
				}
			}
			var tracked pk.Boolean
			nn, err = tracked.ReadFrom(r)
			n += nn
			if err != nil {
				return
			}
		}
	}
	nn, err = remComp.ReadFrom(r)
	n += nn
	if err != nil {
		return
	}
	// Skip removed component IDs
	for i := 0; i < int(remComp); i++ {
		var typeID pk.VarInt
		nn, err = typeID.ReadFrom(r)
		n += nn
		if err != nil {
			return
		}
	}
	return
}

// Slot261Array is a VarInt-length-prefixed array of Slot261 values.
type Slot261Array []Slot261

// WriteTo writes VarInt(len) followed by each slot.
func (a Slot261Array) WriteTo(w io.Writer) (n int64, err error) {
	nn, err := pk.VarInt(len(a)).WriteTo(w)
	n += nn
	if err != nil {
		return
	}
	for _, s := range a {
		nn, err = s.WriteTo(w)
		n += nn
		if err != nil {
			return
		}
	}
	return
}

// ItemStack represents a single item slot.
type ItemStack struct {
	ID            int32            // 0 = empty
	Count         int32
	Durability    int32            // remaining durability (tools only)
	MaxDurability int32            // max durability (tools only; 0 = not a tool)
	Enchantments  map[string]int32 `json:"enchantments,omitempty"` // server-side only
	DisplayName   string           `json:"display_name,omitempty"` // custom name (from anvil rename)
	PotionType    string           `json:"potion_type,omitempty"`  // e.g. "healing", "strength"
	Lodestone     *LodestoneTarget `json:"lodestone,omitempty"`    // lodestone compass target
	CanDestroy    []string         `json:"can_destroy,omitempty"`  // adventure mode: blocks this item can break
	CanPlaceOn    []string         `json:"can_place_on,omitempty"` // adventure mode: blocks this item can be placed on
}

// ToSlot converts an ItemStack to a Slot261 for wire encoding.
func (s ItemStack) ToSlot() Slot261 {
	if s.ID <= 0 || s.Count <= 0 {
		return Slot261{}
	}
	return Slot261{
		Count:         s.Count,
		ItemID:        s.ID,
		Durability:    s.Durability,
		MaxDurability: s.MaxDurability,
		Lodestone:     s.Lodestone,
	}
}

// Inventory represents a player's survival inventory (46 slots).
// Slots 0-8 = crafting, 9-35 = main inventory, 36-44 = hotbar, 45 = offhand.
type Inventory [46]ItemStack

// AddItem tries to add an item to the inventory (main + hotbar slots 9-44).
// Returns the slot index that was modified, or -1 if the inventory is full.
func (inv *Inventory) AddItem(id, count int32) int {
	// First try stacking with existing (skip tools — they are unstackable)
	for i := 9; i <= 44; i++ {
		if inv[i].ID == id && inv[i].Count > 0 && inv[i].Count < 64 && inv[i].MaxDurability == 0 {
			inv[i].Count += count
			return i
		}
	}
	// Then find first empty slot
	for i := 9; i <= 44; i++ {
		if inv[i].ID == 0 || inv[i].Count <= 0 {
			inv[i] = ItemStack{ID: id, Count: count}
			return i
		}
	}
	return -1
}

// EnchantOfferData stores a single enchantment offer.
type EnchantOfferData struct {
	RequiredLevel int32
	EnchantID     string
	EnchantLevel  int32
}

// EnchantSessionData stores an active enchanting session.
type EnchantSessionData struct {
	TablePos [3]int
	WindowID int
	Offers   [3]EnchantOfferData
}

// AnvilSessionData stores an active anvil UI session.
type AnvilSessionData struct {
	WindowID   int
	RenameText string
	Input      ItemStack // slot 0 (left)
	Material   ItemStack // slot 1 (right)
	Output     ItemStack // slot 2 (result)
	RepairCost int32     // XP level cost
}

// SessionEvents receives lightweight event notifications for a player session.
// Implementations must be safe to call from the player's packet-loop goroutine.
// All methods are no-ops when the receiver is nil.
type SessionEvents interface {
	OnDeath(deathMessage string)
	OnRespawn()
	OnDimensionChange(from, to string)
	OnCommand(command string)
}

// Player represents a connected player with their state.
type Player struct {
	Name           string
	UUID           uuid.UUID
	EID            int32 // entity ID
	Properties     []user.Property
	ProfileKey     *user.PublicKey // Mojang profile public key (online mode)
	ChatSessionID  uuid.UUID      // random session ID for chat signing

	mu   sync.Mutex
	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround    bool
	WasOnGround bool

	// LoadedChunks tracks which chunks have been sent to this player.
	LoadedChunks map[ChunkPos]bool

	// VisiblePlayers tracks which other players are within tracking range.
	VisiblePlayers map[uuid.UUID]bool

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

	// Status effects (potions)
	Effects    map[int32]*ActiveEffect // effect ID → active effect
	Absorption float32                // extra absorption hearts from Absorption effect

	// Fall damage tracking
	FallStartY float64

	// Fire ticks remaining (0 = not on fire). Decrements each tick, deals 1 damage every 20 ticks.
	FireTicks int32

	// Drowning — air supply remaining. Max 300 (15s). When <= 0, take 2 drowning damage per second.
	AirTicks int32

	// Entity state
	Sneaking  bool
	Sprinting bool
	Swimming  bool
	InWater   bool // true when player's feet are in water
	OnLadder  bool // true when player is on a ladder/vine (cancels fall damage)
	SkinParts uint8

	// Block breaking state (survival)
	Digging          bool
	DigX, DigY, DigZ int
	DigStartTime     time.Time

	// Survival inventory
	Inventory  Inventory
	EnderItems [27]ItemStack // per-player ender chest storage
	CursorItem ItemStack     // item held on cursor during inventory interaction
	StateID    int32         // container state ID, incremented per server update

	// Eating animation state
	EatingStart time.Time // zero = not eating

	// Experience
	Experience      float32 // 0.0-1.0 bar progress
	ExperienceLevel int32   // current level
	ExperienceTotal int32   // total XP collected

	// Combat cooldown
	LastAttackTime    time.Time
	LastDamageMessage string // death message override (e.g. "X was slain by Y")

	// Shield blocking state
	Blocking            bool
	ShieldCooldownUntil time.Time

	// Bow drawing state
	DrawingBow   bool
	BowDrawStart time.Time

	// Crossbow state
	LoadingCrossbow  bool
	CrossbowLoadStart time.Time
	CrossbowLoaded   bool

	// Elytra gliding state
	Gliding bool

	// Riding state (e.g. boat)
	RidingEntityEID int32

	// Fishing state
	FishingBobberEID int32 // 0 = no bobber out

	// Spawn point (bed)
	SpawnX, SpawnY, SpawnZ float64
	HasSpawnPoint          bool

	// Bad Omen level (0 = none, 1-7 = active)
	BadOmen int32

	// Unlocked recipes (persisted across sessions)
	UnlockedRecipes []string

	// Sleep state
	Sleeping      bool
	LastSleepTick int64

	// Drag state (mode 5)
	DragSlots []int16 // slots collected during a drag operation

	// Container window state
	OpenWindowID        int           // 0=none, 1=crafting table, 2=chest, 3=furnace, 7=brewing stand
	CraftingGrid        [9]ItemStack  // temporary 3x3 crafting grid
	OpenChestPos        [3]int        // world position of open chest
	OpenFurnacePos      [3]int        // world position of open furnace
	OpenBrewingStandPos [3]int        // world position of open brewing stand
	OpenHopperPos       [3]int        // world position of open hopper
	OpenDispenserPos    [3]int        // world position of open dispenser/dropper
	OpenBeaconPos       [3]int        // world position of open beacon
	EnchantSession *EnchantSessionData // active enchanting table session
	EnchantSeed    int32               // deterministic seed for enchant offers; rerolled after each enchant
	AnvilSession   *AnvilSessionData   // active anvil UI session

	// Sign editing state
	EditingSignPos *[3]int // position of sign being edited, nil if not editing

	// Dimension tracking
	Dimension      string // "minecraft:overworld", "minecraft:the_nether", or "minecraft:the_end"
	PortalCooldown int64  // ticks remaining before can use portal again
	PortalTicks    int64  // ticks spent standing in portal (teleport at 80)

	// Ender Dragon boss bar
	DragonBossBarID uuid.UUID // UUID of the active boss bar, zero if none

	// Chat message index (per-player counter for ClientboundPlayerChat, 1.21.5+)
	ChatIndex int32

	// Movement validation (anti-cheat)
	TeleportPending    bool    // skip validation after server-initiated teleport
	ViolationCount     int     // consecutive movement violations
	LastValidX         float64 // last accepted position
	LastValidY         float64
	LastValidZ         float64
	AirTicks_AC        int64   // ticks spent airborne (anti-cheat tracker, separate from AirTicks)

	// Movement enchantment state (updated periodically by enchant_effects.go)
	DepthStriderLevel int32 // current Depth Strider level (0 = inactive)
	SoulSpeedActive   bool  // true when Soul Speed is boosting movement
	SwiftSneakLevel   int32 // current Swift Sneak level (0 = inactive)
	EnchantMoveTick   int64 // tick counter for throttling enchantment checks

	// Step/splash sound tracking
	WalkDistAccum float64 // accumulated walk distance for step sounds
	LastStepTick  int64   // tick of last step sound (throttle)
	WasInWater    bool    // previous tick water state for splash detection
	SwimSoundTick int64   // tick of last swim sound

	// SessionEvents receives tracing events (nil when tracing is disabled).
	SessionEvents SessionEvents
}

// NewPlayer creates a new Player with the given connection info.
func NewPlayer(name string, id uuid.UUID, eid int32, conn *net.Conn) *Player {
	return &Player{
		Name:           name,
		UUID:           id,
		EID:            eid,
		Conn:           conn,
		LoadedChunks:   make(map[ChunkPos]bool),
		VisiblePlayers: make(map[uuid.UUID]bool),
		ViewDistance:    10,
		CreativeItem:   make(map[int16]int32),
		Health:         20,
		Food:           20,
		Saturation:     5,
		FallStartY:     -999,
		AirTicks:       300,
		SkinParts:      0x7F,
		Dimension:      "minecraft:overworld",
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

// IsInvulnerable returns true if the player's game mode prevents damage (creative or spectator).
func (p *Player) IsInvulnerable() bool {
	return p.GameMode == 1 || p.GameMode == 3
}

// HeldItemID returns the item ID of the currently held item (thread-safe).
// Returns 0 if no item is held.
func (p *Player) HeldItemID() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.GameMode == 1 {
		// Creative mode: use CreativeItem map
		return p.CreativeItem[p.HeldSlot+36]
	}
	// Survival mode: read from inventory
	slot := p.Inventory[p.HeldSlot+36]
	if slot.Count <= 0 {
		return 0
	}
	return slot.ID
}

// InventorySlots returns all 46 inventory slots as a Slot261Array for wire encoding.
func (p *Player) InventorySlots() Slot261Array {
	slots := make(Slot261Array, 46)
	for i, s := range p.Inventory {
		slots[i] = s.ToSlot()
	}
	return slots
}

// NextStateID increments and returns the container state ID.
func (p *Player) NextStateID() int32 {
	p.StateID++
	return p.StateID
}

// NextChatIndex returns the current chat index and increments it.
// Used for the globalIndex field in ClientboundPlayerChat (1.21.5+).
func (p *Player) NextChatIndex() int32 {
	idx := p.ChatIndex
	p.ChatIndex++
	return idx
}
