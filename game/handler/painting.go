package handler

import (
	"log"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// EntityTypePainting is the entity type ID for painting (26.1-snapshot-2 registry, 0-indexed).
const EntityTypePainting int32 = 78

// Painting sound event IDs.
const (
	SoundPaintingBreak int32 = 1012
	SoundPaintingPlace int32 = 1013
)

// paintingSize defines the dimensions of a painting variant.
type paintingSize struct {
	Width  int // in blocks
	Height int // in blocks
}

// paintingVariant maps variant name to registry index and size.
type paintingVariant struct {
	ID   int32
	Size paintingSize
}

// paintingVariants is the full list of painting variants with sizes.
// Registry order must match paintingVariantKeys in server/vanilla/registries.go.
var paintingVariants = []paintingVariant{
	{0, paintingSize{1, 1}},  // alban
	{1, paintingSize{1, 1}},  // aztec
	{2, paintingSize{1, 1}},  // aztec2
	{3, paintingSize{2, 1}},  // backyard
	{4, paintingSize{2, 2}},  // baroque
	{5, paintingSize{1, 1}},  // bomb
	{6, paintingSize{2, 2}},  // bouquet
	{7, paintingSize{4, 4}},  // burning_skull
	{8, paintingSize{2, 2}},  // bust
	{9, paintingSize{1, 2}},  // cavebird
	{10, paintingSize{2, 2}}, // changing
	{11, paintingSize{3, 3}}, // cotan
	{12, paintingSize{2, 1}}, // courbet
	{13, paintingSize{2, 1}}, // creebet
	{14, paintingSize{2, 2}}, // dennis (unused in vanilla but registered)
	{15, paintingSize{4, 3}}, // donkey_kong
	{16, paintingSize{2, 2}}, // earth
	{17, paintingSize{3, 3}}, // endboss
	{18, paintingSize{2, 2}}, // fern
	{19, paintingSize{4, 2}}, // fighters
	{20, paintingSize{2, 2}}, // finding
	{21, paintingSize{2, 2}}, // fire
	{22, paintingSize{1, 2}}, // graham
	{23, paintingSize{2, 2}}, // humble
	{24, paintingSize{1, 1}}, // kebab
	{25, paintingSize{4, 2}}, // lowmist
	{26, paintingSize{2, 2}}, // match
	{27, paintingSize{1, 1}}, // meditative
	{28, paintingSize{2, 2}}, // orb
	{29, paintingSize{3, 3}}, // owlemons
	{30, paintingSize{4, 2}}, // passage
	{31, paintingSize{4, 4}}, // pigscene
	{32, paintingSize{1, 1}}, // plant
	{33, paintingSize{4, 4}}, // pointer
	{34, paintingSize{3, 4}}, // pond
	{35, paintingSize{2, 1}}, // pool
	{36, paintingSize{1, 1}}, // prairie_ride
	{37, paintingSize{2, 1}}, // sea
	{38, paintingSize{4, 3}}, // skeleton
	{39, paintingSize{2, 2}}, // skull_and_roses
	{40, paintingSize{2, 2}}, // stage
	{41, paintingSize{3, 3}}, // sunflowers
	{42, paintingSize{2, 1}}, // sunset
	{43, paintingSize{3, 3}}, // tides
	{44, paintingSize{2, 2}}, // unpacked
	{45, paintingSize{2, 2}}, // void
	{46, paintingSize{1, 2}}, // wanderer
	{47, paintingSize{1, 1}}, // wasteland
	{48, paintingSize{2, 2}}, // water
	{49, paintingSize{2, 2}}, // wind
	{50, paintingSize{2, 2}}, // wither
}

// Painting represents a placed painting entity.
type Painting struct {
	EID       int32
	UUID      uuid.UUID
	X, Y, Z   int   // block position (center of painting)
	Face      int32 // facing direction (same as item frame: 0=south, 1=west, 2=north, 3=east)
	VariantID int32 // index into painting_variant registry
}

// PaintingManager manages painting entities.
type PaintingManager struct {
	mu        sync.Mutex
	Manager   *game.PlayerManager
	World     game.World
	ItemMgr   *ItemEntityManager
	Logger    *log.Logger
	paintings map[int32]*Painting // keyed by EID
}

// NewPaintingManager creates a new PaintingManager.
func NewPaintingManager(manager *game.PlayerManager, world game.World, itemMgr *ItemEntityManager, logger *log.Logger) *PaintingManager {
	return &PaintingManager{
		Manager:   manager,
		World:     world,
		ItemMgr:   itemMgr,
		Logger:    logger,
		paintings: make(map[int32]*Painting),
	}
}

// PlacePainting tries to place a painting on the given block face.
// Returns true if a painting was placed.
func (m *PaintingManager) PlacePainting(player *game.Player, blockX, blockY, blockZ int, face int32) bool {
	// Only allow placement on vertical faces (2=north, 3=south, 4=west, 5=east)
	if face < 2 || face > 5 {
		return false
	}

	// Convert block face to painting facing direction
	// Block face: 2=north(Z-), 3=south(Z+), 4=west(X-), 5=east(X+)
	// Painting data: 0=south, 1=west, 2=north, 3=east
	var paintingFace int32
	switch face {
	case 3: // south
		paintingFace = 0
	case 4: // west
		paintingFace = 1
	case 2: // north
		paintingFace = 2
	case 5: // east
		paintingFace = 3
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the largest painting that fits
	variant := m.findFittingPainting(blockX, blockY, blockZ, paintingFace)
	if variant < 0 {
		return false
	}

	eid := m.Manager.NextEntityID()
	painting := &Painting{
		EID:       eid,
		UUID:      uuid.New(),
		X:         blockX,
		Y:         blockY,
		Z:         blockZ,
		Face:      paintingFace,
		VariantID: variant,
	}
	m.paintings[eid] = painting

	m.broadcastSpawn(painting)
	m.broadcastVariantMetadata(painting)

	BroadcastSound(m.Manager, SoundPaintingPlace, SoundCategoryNeutral,
		float64(blockX)+0.5, float64(blockY)+0.5, float64(blockZ)+0.5, 1.0, 1.0)

	m.logf("Painting placed at (%d, %d, %d) face=%d variant=%d EID=%d",
		blockX, blockY, blockZ, paintingFace, variant, eid)
	return true
}

// findFittingPainting finds a random painting variant that fits the wall space.
// Tries largest sizes first, falls back to smaller.
func (m *PaintingManager) findFittingPainting(x, y, z int, face int32) int32 {
	// Group variants by area (largest first)
	type candidate struct {
		id   int32
		area int
	}
	var candidates []candidate
	for _, v := range paintingVariants {
		if m.paintingFits(x, y, z, face, v.Size) {
			candidates = append(candidates, candidate{v.ID, v.Size.Width * v.Size.Height})
		}
	}

	if len(candidates) == 0 {
		return -1
	}

	// Find max area
	maxArea := 0
	for _, c := range candidates {
		if c.area > maxArea {
			maxArea = c.area
		}
	}

	// Filter to largest paintings only
	var largest []int32
	for _, c := range candidates {
		if c.area == maxArea {
			largest = append(largest, c.id)
		}
	}

	return largest[rand.Intn(len(largest))]
}

// paintingFits checks if a painting of the given size fits at the position on the wall.
func (m *PaintingManager) paintingFits(x, y, z int, face int32, size paintingSize) bool {
	// Check that all blocks behind the painting area are solid (not air)
	// and that the painting area itself is clear (air)
	dx, dz := paintingNormal(face)

	// hDir and vDir define the painting plane
	hx, hz := paintingHorizontal(face)

	for w := 0; w < size.Width; w++ {
		for h := 0; h < size.Height; h++ {
			// Position in the painting plane
			px := x + hx*w
			py := y + h
			pz := z + hz*w

			// Check the space where the painting sits (must be passable)
			state, err := m.World.GetBlock(px, py, pz)
			if err != nil {
				return false
			}
			name := BlockNameFromState(int(state))
			if name != "air" && name != "cave_air" && name != "void_air" {
				return false
			}

			// Check the block behind (must be solid)
			bx := px - dx
			bz := pz - dz
			behindState, err := m.World.GetBlock(bx, py, bz)
			if err != nil {
				return false
			}
			behindName := BlockNameFromState(int(behindState))
			if behindName == "air" || behindName == "cave_air" || behindName == "void_air" ||
				behindName == "water" || behindName == "lava" {
				return false
			}
		}
	}
	return true
}

// paintingNormal returns the normal direction for a painting face.
// The normal points away from the wall (toward the viewer).
func paintingNormal(face int32) (dx, dz int) {
	switch face {
	case 0: // south (+Z)
		return 0, 1
	case 1: // west (-X)
		return -1, 0
	case 2: // north (-Z)
		return 0, -1
	case 3: // east (+X)
		return 1, 0
	}
	return 0, 0
}

// paintingHorizontal returns the horizontal direction for width expansion.
func paintingHorizontal(face int32) (hx, hz int) {
	switch face {
	case 0: // south → expand west (-X)
		return -1, 0
	case 1: // west → expand south (+Z... actually north -Z for left-to-right)
		return 0, -1
	case 2: // north → expand east (+X)
		return 1, 0
	case 3: // east → expand south (+Z)
		return 0, 1
	}
	return 0, 0
}

// IsPainting returns true if the entity ID is a painting.
func (m *PaintingManager) IsPainting(eid int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.paintings[eid]
	return ok
}

// RemovePainting destroys a painting and drops the painting item.
func (m *PaintingManager) RemovePainting(eid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	painting, ok := m.paintings[eid]
	if !ok {
		return
	}
	delete(m.paintings, eid)

	fx := float64(painting.X) + 0.5
	fy := float64(painting.Y) + 0.5
	fz := float64(painting.Z) + 0.5

	// Drop painting item
	if m.ItemMgr != nil {
		paintingItemID := itemIDByName("painting")
		if paintingItemID > 0 {
			m.ItemMgr.SpawnItem(m.Manager, fx, fy, fz, paintingItemID, 1, 10)
		}
	}

	BroadcastSound(m.Manager, SoundPaintingBreak, SoundCategoryNeutral, fx, fy, fz, 1.0, 1.0)

	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})

	m.logf("Painting removed EID=%d", eid)
}

// SendExistingPaintings sends all paintings to a newly joined player.
func (m *PaintingManager) SendExistingPaintings(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, painting := range m.paintings {
		m.sendSpawnTo(player, painting)
		m.sendVariantMetadataTo(player, painting)
	}
}

func (m *PaintingManager) broadcastSpawn(painting *Painting) {
	pkt := m.buildSpawnPacket(painting)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *PaintingManager) sendSpawnTo(player *game.Player, painting *Painting) {
	player.WritePacket(m.buildSpawnPacket(painting))
}

func (m *PaintingManager) buildSpawnPacket(painting *Painting) pk.Packet {
	x := float64(painting.X) + 0.5
	y := float64(painting.Y) + 0.5
	z := float64(painting.Z) + 0.5

	return pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(painting.EID),
		pk.UUID(painting.UUID),
		pk.VarInt(EntityTypePainting),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.VarInt(painting.Face), // data field = facing direction
	)
}

// broadcastVariantMetadata sends painting variant metadata.
// Index 8: Painting variant (serializer type 24 = painting_variant)
func (m *PaintingManager) broadcastVariantMetadata(painting *Painting) {
	meta := m.buildVariantMetadata(painting)
	pkt := pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(painting.EID),
		pk.PluginMessageData(meta),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (m *PaintingManager) sendVariantMetadataTo(player *game.Player, painting *Painting) {
	meta := m.buildVariantMetadata(painting)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetEntityData,
		pk.VarInt(painting.EID),
		pk.PluginMessageData(meta),
	))
}

func (m *PaintingManager) buildVariantMetadata(painting *Painting) []byte {
	var buf []byte
	buf = append(buf, 8)                              // index 8
	buf = appendVarInt(buf, 24)                        // serializer: painting_variant
	buf = appendVarInt(buf, int(painting.VariantID))   // variant ID
	buf = append(buf, 0xFF)                            // terminator
	return buf
}

func (m *PaintingManager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}
