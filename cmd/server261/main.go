// Command server261 is a minimal Minecraft 26.1-snapshot-2 server.
//
// It handles server list pings, login, configuration (registry sync),
// and spawns the player in a void world. No world logic is implemented —
// the player floats in the void and stays connected via keepalive.
//
// Usage:
//
//	go run ./cmd/server261
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
)

func main() {
	logger := log.New(os.Stdout, "[Server] ", log.LstdFlags)

	// Build minimal registries for 26.1-snapshot-2
	regs := buildRegistries()

	srv := server.Server{
		Logger:          logger,
		ListPingHandler: &pingHandler{},
		LoginHandler: &server.MojangLoginHandler{
			OnlineMode: false,
			Threshold:  256,
		},
		ConfigHandler: &server.Configurations{
			Registries: regs,
			KnownPacks: []server.KnownPack{
				{Namespace: "minecraft", ID: "core", Version: server.ProtocolName},
			},
			KnownPackEntries: vanillaRegistryKeys(),
			Tags:             vanillaConfigTags(),
		},
		GamePlay:      &gamePlay{logger: logger},
	}

	addr := ":25565"
	logger.Printf("Starting server on %s (protocol %s / %d)", addr, server.ProtocolName, server.ProtocolVersion)
	if err := srv.Listen(addr); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListPingHandler
// ---------------------------------------------------------------------------

type pingHandler struct{}

func (p *pingHandler) Name() string                    { return server.ProtocolName }
func (p *pingHandler) Protocol(int32) int              { return server.ProtocolVersion }
func (p *pingHandler) MaxPlayer() int                  { return 20 }
func (p *pingHandler) OnlinePlayer() int               { return 0 }
func (p *pingHandler) PlayerSamples() []server.PlayerSample { return nil }
func (p *pingHandler) Description() *chat.Message {
	msg := chat.Text("A Go-MC 26.1-snapshot-2 Server")
	return &msg
}
func (p *pingHandler) FavIcon() string { return "" }

// ---------------------------------------------------------------------------
// GamePlay
// ---------------------------------------------------------------------------

type gamePlay struct {
	logger *log.Logger
}

func (g *gamePlay) logf(format string, args ...any) {
	if g.logger != nil {
		g.logger.Printf(format, args...)
	}
}

func (g *gamePlay) AcceptPlayer(name string, id uuid.UUID, profilePubKey *user.PublicKey, properties []user.Property, protocol int32, conn *net.Conn) {
	g.logf("Player %s (%s) joined [protocol=%d]", name, id, protocol)
	defer g.logf("Player %s (%s) left", name, id)

	if err := g.sendJoinGame(conn); err != nil {
		g.logf("Error sending JoinGame to %s: %v", name, err)
		return
	}

	// PlayerAbilities — required for creative mode flight/instant break
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerAbilities,
		pk.Byte(0x0D),    // flags: invulnerable(0x01) | allowFlying(0x04) | creativeMode(0x08)
		pk.Float(0.05),   // fly speed
		pk.Float(0.1),    // field of view modifier
	)); err != nil {
		g.logf("Error sending PlayerAbilities to %s: %v", name, err)
		return
	}

	// PlayerPosition — must come BEFORE chunks (vanilla sends it early)
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundPlayerPosition,
		pk.VarInt(1),     // teleport ID
		pk.Double(0.5),   // x (center of block)
		pk.Double(112),   // y (above stone platform at Y=96-111)
		pk.Double(0.5),   // z
		pk.Double(0),     // vel_x
		pk.Double(0),     // vel_y
		pk.Double(0),     // vel_z
		pk.Float(0),      // yaw
		pk.Float(0),      // pitch
		pk.Int(0),        // flags: all absolute
	)); err != nil {
		g.logf("Error sending PlayerPosition to %s: %v", name, err)
		return
	}

	if err := g.sendSpawnSequence(conn); err != nil {
		g.logf("Error sending spawn sequence to %s: %v", name, err)
		return
	}

	// Keepalive + packet drain loop
	g.keepAliveLoop(conn, name)
}

func (g *gamePlay) sendJoinGame(conn *net.Conn) error {
	// Build the JoinGame (Login) packet.
	// Field order per wiki.vg for 1.20.2+ (extended in 1.21.2 with sea_level):
	//   Int           entity_id
	//   Boolean       is_hardcore
	//   Array[Ident]  dimension_names  (VarInt-prefixed)
	//   VarInt        max_players
	//   VarInt        view_distance
	//   VarInt        simulation_distance
	//   Boolean       reduced_debug_info
	//   Boolean       enable_respawn_screen
	//   Boolean       do_limited_crafting
	//   VarInt        dimension_type     (registry index)
	//   Identifier    dimension_name
	//   Long          hashed_seed
	//   UnsignedByte  gamemode
	//   Byte          previous_gamemode  (-1 = none)
	//   Boolean       is_debug
	//   Boolean       is_flat
	//   Optional<death_location>:
	//     Boolean     has_death_location
	//     (if true: Identifier + Position)
	//   VarInt        portal_cooldown
	//   VarInt        sea_level           (1.21.2+)
	//   Boolean       enforces_secure_chat

	dimensionNames := []pk.Identifier{"minecraft:overworld"}

	return conn.WritePacket(pk.Marshal(
		packetid.ClientboundLogin,
		pk.Int(1),                  // entity ID
		pk.Boolean(false),          // not hardcore
		pk.Array(dimensionNames),   // dimension names
		pk.VarInt(20),              // max players
		pk.VarInt(10),              // view distance
		pk.VarInt(10),              // simulation distance
		pk.Boolean(false),          // reduced debug info
		pk.Boolean(true),           // enable respawn screen
		pk.Boolean(false),          // do limited crafting
		pk.VarInt(0),               // dimension type = index 0 in dimension_type registry
		pk.Identifier("minecraft:overworld"), // dimension name
		pk.Long(0),                 // hashed seed
		pk.UnsignedByte(1),         // gamemode: creative
		pk.Byte(-1),                // previous gamemode: none
		pk.Boolean(false),          // is debug
		pk.Boolean(true),           // is flat (superflat for void)
		pk.Boolean(false),          // has death location
		pk.VarInt(0),               // portal cooldown
		pk.VarInt(63),              // sea level
		pk.Boolean(false),          // enforces secure chat
	))
}

func (g *gamePlay) sendSpawnSequence(conn *net.Conn) error {
	// 1. Set default spawn position (26.1: RespawnData = GlobalPos + yaw + pitch)
	err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundSetDefaultSpawnPosition,
		pk.Identifier("minecraft:overworld"), // dimension
		pk.Position{X: 0, Y: 112, Z: 0},     // block pos (above stone)
		pk.Float(0), // yaw
		pk.Float(0), // pitch
	))
	if err != nil {
		return fmt.Errorf("spawn position: %w", err)
	}

	// 2. Game event: start waiting for level chunks (event 13, value 0)
	err = conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(13), // event: start waiting for chunks
		pk.Float(0),         // value
	))
	if err != nil {
		return fmt.Errorf("game event: %w", err)
	}

	// 3. Set chunk cache center
	err = conn.WritePacket(pk.Marshal(
		packetid.ClientboundSetChunkCacheCenter,
		pk.VarInt(0), // chunk X
		pk.VarInt(0), // chunk Z
	))
	if err != nil {
		return fmt.Errorf("chunk cache center: %w", err)
	}

	// 4. Send chunk batch start
	err = conn.WritePacket(pk.Marshal(packetid.ClientboundChunkBatchStart))
	if err != nil {
		return fmt.Errorf("chunk batch start: %w", err)
	}

	// 5. Send spawn chunks with a stone platform
	// Overworld: 384 blocks height (-64 to 320), so 384/16 = 24 sections
	//
	// 26.1 chunk format:
	//   ChunkData: heightmaps (stream codec map) + ByteArray(sections) + List(blockEntities)
	//   LightData: 4 BitSets + 2 Lists (NO Trust Edges boolean)
	//
	// Section format: Short(blockCount) + PaletteContainer(states) + PaletteContainer(biomes)
	// 26.1 CHANGE: PaletteContainer no longer writes VarInt(dataLongsCount) prefix!
	// The data array length is inferred from bitsPerEntry.
	// Single-value palette (bitsPerEntry=0): UnsignedByte(0) + VarInt(value) [no data array at all]

	// Empty section (all air): 6 bytes
	emptySection := []byte{
		0x00, 0x00, // Short(0) blockCount
		0x00,       // UnsignedByte(0) bits per entry for states (single value)
		0x00,       // VarInt(0) = air state ID
		0x00,       // UnsignedByte(0) bits per entry for biomes (single value)
		0x00,       // VarInt(0) = biome ID 0
	}

	// All-stone section: 6 bytes
	stoneSection := []byte{
		0x10, 0x00, // Short(4096) blockCount = all non-air
		0x00,       // UnsignedByte(0) bits per entry for states (single value)
		0x01,       // VarInt(1) = stone state ID
		0x00,       // UnsignedByte(0) bits per entry for biomes (single value)
		0x00,       // VarInt(0) = biome ID 0
	}

	// Build section data: 24 sections, section 10 is stone (Y=96..111), rest air
	var buf bytes.Buffer
	for i := 0; i < 24; i++ {
		if i == 10 {
			buf.Write(stoneSection)
		} else {
			buf.Write(emptySection)
		}
	}
	stoneSectionData := append([]byte(nil), buf.Bytes()...) // copy! Buffer.Bytes() shares memory

	// Build empty chunk section data (all 24 sections air)
	buf.Reset()
	for i := 0; i < 24; i++ {
		buf.Write(emptySection)
	}
	emptySectionData := append([]byte(nil), buf.Bytes()...)

	// Build heightmap data for stone and empty chunks.
	// Heightmap keys: WORLD_SURFACE=1, MOTION_BLOCKING=4, MOTION_BLOCKING_NO_LEAVES=5
	// For stone chunk: top block at Y=111, heightmap value = (111+1) - (-64) = 176
	// For empty chunk: value = 0
	stoneHeightmaps := buildHeightmapBytes(176)
	emptyHeightmaps := buildHeightmapBytes(0)

	// Light data: 24 chunk sections → 26 light sections (one below, one above)
	numLightSections := 26
	skyLightMask := make(pk.BitSet, 1)
	for i := 0; i < numLightSections; i++ {
		skyLightMask.Set(i, true)
	}
	fullSkyLight := make(pk.ByteArray, 2048)
	for i := range fullSkyLight {
		fullSkyLight[i] = 0xFF
	}
	skyLightArrays := make([]pk.ByteArray, numLightSections)
	for i := range skyLightArrays {
		skyLightArrays[i] = fullSkyLight
	}
	emptyBitSet := pk.BitSet{}
	chunkCount := 0

	for cx := -3; cx <= 3; cx++ {
		for cz := -3; cz <= 3; cz++ {
			sectionData := emptySectionData
			heightmaps := emptyHeightmaps
			if cx >= -1 && cx <= 0 && cz >= -1 && cz <= 0 {
				sectionData = stoneSectionData
				heightmaps = stoneHeightmaps
			}
			err = conn.WritePacket(pk.Marshal(
				packetid.ClientboundLevelChunkWithLight,
				pk.Int(int32(cx)), pk.Int(int32(cz)),
				// -- ChunkData --
				rawBytes(heightmaps),      // heightmaps: 3 entries
				pk.ByteArray(sectionData), // section data
				pk.VarInt(0),              // block entities: empty list
				// -- LightData --
				skyLightMask,             // skyYMask
				emptyBitSet,              // blockYMask
				emptyBitSet,              // emptySkyYMask
				emptyBitSet,              // emptyBlockYMask
				pk.Array(skyLightArrays), // skyUpdates
				pk.VarInt(0),             // blockUpdates: empty list
			))
			if err != nil {
				return fmt.Errorf("chunk data (%d,%d): %w", cx, cz, err)
			}
			chunkCount++
		}
	}
	g.logf("Sent %d chunks (%d bytes stone, %d bytes empty)", chunkCount, len(stoneSectionData), len(emptySectionData))

	// 6. Chunk batch finished
	err = conn.WritePacket(pk.Marshal(packetid.ClientboundChunkBatchFinished, pk.VarInt(int32(chunkCount))))
	if err != nil {
		return fmt.Errorf("chunk batch finished: %w", err)
	}

	return nil
}

func (g *gamePlay) keepAliveLoop(conn *net.Conn, name string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Goroutine to send keepalive pings; stops when done is closed.
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				keepAliveID := rand.Int63()
				err := conn.WritePacket(pk.Marshal(
					packetid.ClientboundKeepAlive,
					pk.Long(keepAliveID),
				))
				if err != nil {
					return
				}
			}
		}
	}()

	// Read and discard all incoming packets (handles keepalive responses implicitly)
	var p pk.Packet
	for {
		if err := conn.ReadPacket(&p); err != nil {
			g.logf("Player %s disconnected: %v", name, err)
			return
		}
		// Silently discard all packets — movement, chat, etc.
	}
}

// ---------------------------------------------------------------------------
// Registry data
// ---------------------------------------------------------------------------

func buildRegistries() registry.Registries {
	regs := registry.NewNetworkCodec()

	// minecraft:dimension_type — minimum required for JoinGame
	regs.DimensionType.Put("minecraft:overworld", registry.Dimension{
		HasSkylight:        true,
		HasCeiling:         false,
		Ultrawarm:          false,
		Natural:            true,
		CoordinateScale:    1.0,
		BedWorks:           true,
		RespawnAnchorWorks: 0,
		MinY:               -64,
		Height:             384,
		LogicalHeight:      384,
		InfiniteBurn:       "#minecraft:infiniburn_overworld",
		Effects:            "minecraft:overworld",
		AmbientLight:       0.0,
		PiglinSafe:         0,
		HasRaids:           1,
		MonsterSpawnLightLevel: mustNBTRaw(map[string]any{
			"type": "minecraft:uniform",
			"value": map[string]any{
				"min_inclusive": int32(0),
				"max_inclusive": int32(7),
			},
		}),
		MonsterSpawnBlockLightLimit: 0,
	})

	// minecraft:worldgen/biome — at least one biome needed
	regs.WorldGenBiome.Put("minecraft:plains", mustNBTRaw(map[string]any{
		"has_precipitation": byte(1),
		"temperature":      float32(0.8),
		"downfall":         float32(0.4),
		"effects": map[string]any{
			"sky_color":       int32(7907327),
			"water_fog_color": int32(329011),
			"fog_color":       int32(12638463),
			"water_color":     int32(4159204),
		},
	}))

	// minecraft:chat_type — required for chat functionality
	regs.ChatType.Put("minecraft:chat", registry.ChatType{
		Chat: chat.Decoration{
			TranslationKey: "chat.type.text",
			Parameters:     []string{"sender", "content"},
		},
		Narration: chat.Decoration{
			TranslationKey: "chat.type.text.narrate",
			Parameters:     []string{"sender", "content"},
		},
	})

	// minecraft:damage_type — at least one required
	regs.DamageType.Put("minecraft:generic", registry.DamageType{
		MessageID:  "generic",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:generic_kill", registry.DamageType{
		MessageID:  "genericKill",
		Scaling:    "never",
		Exhaustion: 0.0,
	})

	return regs
}

// rawBytes is a FieldEncoder that writes its contents directly (no length prefix).
type rawBytes []byte

func (r rawBytes) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r)
	return int64(n), err
}

// buildHeightmapBytes builds the raw bytes for a heightmap map with 3 entries
// (WORLD_SURFACE=1, MOTION_BLOCKING=4, MOTION_BLOCKING_NO_LEAVES=5),
// all set to the same uniform value for all 256 columns.
func buildHeightmapBytes(value int32) []byte {
	longs := packHeightmapLongs(value)

	var buf bytes.Buffer
	// VarInt(3) — 3 heightmap entries
	writeVarInt(&buf, 3)
	// Entry 1: WORLD_SURFACE (key=1)
	writeVarInt(&buf, 1)
	writeLongArray(&buf, longs)
	// Entry 2: MOTION_BLOCKING (key=4)
	writeVarInt(&buf, 4)
	writeLongArray(&buf, longs)
	// Entry 3: MOTION_BLOCKING_NO_LEAVES (key=5)
	writeVarInt(&buf, 5)
	writeLongArray(&buf, longs)

	return buf.Bytes()
}

// packHeightmapLongs packs 256 entries of the same value into a compact long array
// using 9 bits per entry (for overworld height 384).
func packHeightmapLongs(value int32) []int64 {
	const bitsPerEntry = 9
	const valsPerLong = 64 / bitsPerEntry // 7
	numLongs := (256 + valsPerLong - 1) / valsPerLong // 37

	longs := make([]int64, numLongs)
	for i := 0; i < 256; i++ {
		longIdx := i / valsPerLong
		bitOffset := uint(i%valsPerLong) * bitsPerEntry
		longs[longIdx] |= int64(value) << bitOffset
	}
	return longs
}

func writeVarInt(buf *bytes.Buffer, v int32) {
	val := uint32(v)
	for val >= 0x80 {
		buf.WriteByte(byte(val&0x7F) | 0x80)
		val >>= 7
	}
	buf.WriteByte(byte(val))
}

func writeLongArray(buf *bytes.Buffer, longs []int64) {
	writeVarInt(buf, int32(len(longs)))
	for _, l := range longs {
		// Big-endian int64
		buf.WriteByte(byte(l >> 56))
		buf.WriteByte(byte(l >> 48))
		buf.WriteByte(byte(l >> 40))
		buf.WriteByte(byte(l >> 32))
		buf.WriteByte(byte(l >> 24))
		buf.WriteByte(byte(l >> 16))
		buf.WriteByte(byte(l >> 8))
		buf.WriteByte(byte(l))
	}
}

// mustNBTRaw encodes v as an nbt.RawMessage suitable for embedding
// as a field value. Uses network format to get [tag_type][payload],
// then stores them separately in RawMessage.
func mustNBTRaw(v any) nbt.RawMessage {
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)
	if err := enc.Encode(v, ""); err != nil {
		panic(fmt.Sprintf("nbt.Encode: %v", err))
	}
	data := buf.Bytes()
	// Network format output: [tag_type_byte] [payload...]
	// RawMessage stores them separately.
	return nbt.RawMessage{
		Type: data[0],
		Data: append([]byte(nil), data[1:]...), // copy payload
	}
}
