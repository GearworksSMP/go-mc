package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/gen"
	"github.com/Tnze/go-mc/game/handler"
	"github.com/Tnze/go-mc/game/mem"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/server"
)

// findFreePort finds an available TCP port for testing.
func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// startTestServer starts the server261 on a random port and returns the address.
func startTestServer(t *testing.T) string {
	t.Helper()
	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	regs := buildRegistries()
	sfGen := gen.DefaultSuperflat()
	world := mem.NewWorld(sfGen, sfGen.Sections, sfGen.MinY)
	players := game.NewPlayerManager()

	srv := server.Server{
		ListPingHandler: &pingHandler{players: players},
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
		GamePlay: &gamePlay{
			logger:         nil,
			world:          world,
			players:        players,
			sections:       sfGen.Sections,
			minY:           sfGen.MinY,
			spawnY:         sfGen.SpawnY(),
			isFlat:         true,
			timeMgr:        &handler.TimeManager{},
			commandGraph:   handler.BuildCommandGraph(),
			itemEntities:   &handler.ItemEntityManager{Items: map[int32]*handler.DroppedItem{}},
			mobMgr:         &handler.MobManager{},
			arrowMgr:       &handler.ArrowManager{},
			fishingMgr:     &handler.FishingManager{},
			boatMgr:        &handler.BoatManager{},
			minecartMgr:    &handler.MinecartManager{},
			tntMgr:         &handler.TNTManager{},
			potionMgr:      &handler.PotionManager{},
			armorStandMgr:  &handler.ArmorStandManager{},
			itemFrameMgr:   &handler.ItemFrameManager{},
			paintingMgr:    &handler.PaintingManager{},
			leashMgr:       &handler.LeashManager{},
			advancementMgr: handler.NewAdvancementManager(players),
		},
	}

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Listen(addr)
	}()

	// Wait for server to be ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return addr
		}
		time.Sleep(50 * time.Millisecond)
	}

	select {
	case err := <-errCh:
		t.Fatalf("Server failed to start: %v", err)
	default:
	}
	t.Fatal("Server did not become ready in time")
	return ""
}

func TestPingAndList(t *testing.T) {
	addr := startTestServer(t)

	data, delay, err := bot.PingAndListTimeout(addr, 5*time.Second)
	if err != nil {
		t.Fatalf("PingAndList failed: %v", err)
	}

	t.Logf("Ping delay: %v", delay)
	t.Logf("Response: %s", data)

	var resp struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
		Players struct {
			Max    int `json:"max"`
			Online int `json:"online"`
		} `json:"players"`
		Description struct {
			Text string `json:"text"`
		} `json:"description"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if resp.Version.Name != server.ProtocolName {
		t.Errorf("Version name = %q, want %q", resp.Version.Name, server.ProtocolName)
	}
	if resp.Version.Protocol != server.ProtocolVersion {
		t.Errorf("Protocol = %d, want %d", resp.Version.Protocol, server.ProtocolVersion)
	}
	if resp.Players.Max != 20 {
		t.Errorf("MaxPlayers = %d, want 20", resp.Players.Max)
	}
	t.Logf("PASS: Server list ping returns correct version %s (protocol %d)",
		resp.Version.Name, resp.Version.Protocol)
}

// TestChunkBlockContent connects to the server, receives chunks, and verifies
// section data in 26.1 format. The server now uses a superflat world with
// bedrock(1) + dirt(2) + grass_block(1) starting at Y=-64 (section 0).
func TestChunkBlockContent(t *testing.T) {
	addr := startTestServer(t)

	conn, err := mcnet.DialMC(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	port := 25565
	fmt.Sscanf(addr, "127.0.0.1:%d", &port)

	// Handshake
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.VarInt(1073742112),
		pk.String("127.0.0.1"),
		pk.UnsignedShort(uint16(port)),
		pk.VarInt(2),
	))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	// Login Start
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.String("TestBot"),
		pk.UUID{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0},
	))
	if err != nil {
		t.Fatalf("login start: %v", err)
	}

	phase := "login"
	var p pk.Packet

	type chunkResult struct {
		x, z       int32
		dataLen    int
		sections   []sectionInfo
		heightmaps int
	}
	var chunks []chunkResult

	timeout := time.After(10 * time.Second)
	for i := 0; i < 5000; i++ {
		select {
		case <-timeout:
			t.Fatalf("timeout after 10s (phase=%s, received %d chunks)", phase, len(chunks))
		default:
		}

		err = conn.ReadPacket(&p)
		if err != nil {
			t.Fatalf("read packet %d (%s): %v", i, phase, err)
		}

		switch phase {
		case "login":
			if p.ID == 0x03 {
				var threshold pk.VarInt
				p.Scan(&threshold)
				conn.SetThreshold(int(threshold))
			}
			if p.ID == 0x02 {
				conn.WritePacket(pk.Marshal(0x03))
				phase = "config"
			}

		case "config":
			if p.ID == int32(packetid.ClientboundConfigSelectKnownPacks) {
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigSelectKnownPacks),
					pk.VarInt(1),
					pk.String("minecraft"),
					pk.String("core"),
					pk.String("26.1-snapshot-2"),
				))
			}
			if p.ID == int32(packetid.ClientboundConfigFinishConfiguration) {
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigFinishConfiguration)))
				phase = "play"
			}

		case "play":
			if p.ID == int32(packetid.ClientboundLevelChunkWithLight) {
				r := bytes.NewReader(p.Data)
				var x, z int32
				binary.Read(r, binary.BigEndian, &x)
				binary.Read(r, binary.BigEndian, &z)

				hmCount := readVarInt261(r)
				for j := 0; j < hmCount; j++ {
					readVarInt261(r)
					arrLen := readVarInt261(r)
					for k := 0; k < arrLen; k++ {
						var l int64
						binary.Read(r, binary.BigEndian, &l)
					}
				}

				sectionDataLen := readVarInt261(r)
				sectionData := make([]byte, sectionDataLen)
				r.Read(sectionData)

				sr := bytes.NewReader(sectionData)
				var sections []sectionInfo
				for secIdx := 0; secIdx < 24; secIdx++ {
					sec := parseSection261(sr)
					sec.index = secIdx
					sections = append(sections, sec)
				}

				remaining := sr.Len()
				if remaining != 0 {
					t.Errorf("Chunk (%d,%d): %d bytes remaining after parsing 24 sections!", x, z, remaining)
				}

				chunks = append(chunks, chunkResult{
					x: x, z: z,
					dataLen:    sectionDataLen,
					sections:   sections,
					heightmaps: hmCount,
				})

				if len(chunks) >= 49 {
					goto done
				}
			}
		}
	}
done:

	t.Logf("Received %d chunks", len(chunks))
	if len(chunks) < 49 {
		t.Fatalf("Expected at least 49 chunks, got %d", len(chunks))
	}

	// Verify all chunks are superflat: section 0 has blocks, sections 1-23 empty
	for _, c := range chunks {
		if c.heightmaps != 3 {
			t.Errorf("Chunk (%d,%d): expected 3 heightmap entries, got %d", c.x, c.z, c.heightmaps)
		}

		// Section 0: bedrock(1) + dirt(2) + grass_block(1) = 4 layers = 1024 blocks
		sec0 := c.sections[0]
		if sec0.blockCount != 1024 {
			t.Errorf("Chunk (%d,%d) section 0: blockCount=%d, want 1024", c.x, c.z, sec0.blockCount)
		}
		// Section 0 should have a multi-value palette (bitsPerEntry=4)
		if sec0.statesBitsPerEntry != 4 {
			t.Errorf("Chunk (%d,%d) section 0: statesBitsPerEntry=%d, want 4", c.x, c.z, sec0.statesBitsPerEntry)
		}

		// Sections 1-23 should all be empty (blockCount=0)
		for idx := 1; idx < 24; idx++ {
			s := c.sections[idx]
			if s.blockCount != 0 {
				t.Errorf("Chunk (%d,%d) section %d: blockCount=%d, want 0", c.x, c.z, idx, s.blockCount)
			}
		}
	}
	t.Logf("PASS: %d superflat chunks verified — section 0 has 1024 blocks, sections 1-23 empty", len(chunks))
}

type sectionInfo struct {
	index              int
	blockCount         int16
	statesBitsPerEntry int
	statesSingleValue  int
	biomesBitsPerEntry int
	biomesSingleValue  int
}

func parseSection261(r *bytes.Reader) sectionInfo {
	var s sectionInfo
	binary.Read(r, binary.BigEndian, &s.blockCount)

	// 26.1 PaletteContainer: NO VarInt data count prefix
	s.statesBitsPerEntry = int(readByte261(r))
	if s.statesBitsPerEntry == 0 {
		s.statesSingleValue = readVarInt261(r)
	} else {
		paletteLen := readVarInt261(r)
		for i := 0; i < paletteLen; i++ {
			readVarInt261(r)
		}
		dataLongs := (4096*s.statesBitsPerEntry + 63) / 64
		for i := 0; i < dataLongs; i++ {
			var l int64
			binary.Read(r, binary.BigEndian, &l)
		}
	}

	s.biomesBitsPerEntry = int(readByte261(r))
	if s.biomesBitsPerEntry == 0 {
		s.biomesSingleValue = readVarInt261(r)
	} else {
		paletteLen := readVarInt261(r)
		for i := 0; i < paletteLen; i++ {
			readVarInt261(r)
		}
		dataLongs := (64*s.biomesBitsPerEntry + 63) / 64
		for i := 0; i < dataLongs; i++ {
			var l int64
			binary.Read(r, binary.BigEndian, &l)
		}
	}

	return s
}

func readVarInt261(r *bytes.Reader) int {
	var result int
	for shift := 0; shift < 35; shift += 7 {
		b, err := r.ReadByte()
		if err != nil {
			return 0
		}
		result |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			return result
		}
	}
	return result
}

func readByte261(r *bytes.Reader) byte {
	b, _ := r.ReadByte()
	return b
}

// TestHeightmapPacking verifies the heightmap packing algorithm.
func TestHeightmapPacking(t *testing.T) {
	// Test with value 4 (vanilla flat world)
	longs := packHeightmapLongs(4)
	if len(longs) != 37 {
		t.Fatalf("expected 37 longs, got %d", len(longs))
	}

	// Compare with vanilla heightmap data
	vanillaLong := int64(0x0100804020100804)
	if longs[0] != vanillaLong {
		t.Errorf("value=4: long[0] = %016x, want vanilla %016x", uint64(longs[0]), uint64(vanillaLong))
	}
	t.Logf("Heightmap value=4: long[0]=%016x (matches vanilla)", uint64(longs[0]))

	// Test with value 176 (stone at Y=111)
	longs176 := packHeightmapLongs(176)
	val0 := int(longs176[0] & 0x1FF)
	if val0 != 176 {
		t.Errorf("value=176: unpacked[0]=%d, want 176", val0)
	}

	// Test with value 0 (empty chunk)
	longs0 := packHeightmapLongs(0)
	for i, l := range longs0 {
		if l != 0 {
			t.Errorf("value=0: long[%d] = %016x, want 0", i, uint64(l))
		}
	}
	t.Log("PASS: All heightmap packing tests passed")
}

func TestLoginAndConfig(t *testing.T) {
	addr := startTestServer(t)

	client := bot.NewClient()
	client.Auth = bot.Auth{Name: "TestBot"}

	err := client.JoinServer(addr)
	if err != nil {
		t.Fatalf("JoinServer (login+config) failed: %v", err)
	}
	defer client.Close()

	t.Log("PASS: Login + Configuration phase succeeded")
}

// TestNeoForgeConfigPhase simulates a NeoForge client connecting during the
// configuration phase. NeoForge clients send extra CustomPayload packets
// (minecraft:register, c:version, c:register) before responding to
// SelectKnownPacks, and may send more before FinishConfiguration. The server
// must silently discard these and complete the config phase normally.
func TestNeoForgeConfigPhase(t *testing.T) {
	addr := startTestServer(t)

	conn, err := mcnet.DialMC(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	port := 25565
	fmt.Sscanf(addr, "127.0.0.1:%d", &port)

	// Handshake
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.VarInt(server.ProtocolVersion),
		pk.String("127.0.0.1"),
		pk.UnsignedShort(uint16(port)),
		pk.VarInt(2), // login
	))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	// Login Start
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.String("NeoForgeBot"),
		pk.UUID{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99},
	))
	if err != nil {
		t.Fatalf("login start: %v", err)
	}

	phase := "login"
	var p pk.Packet

	timeout := time.After(10 * time.Second)
	for i := 0; i < 5000; i++ {
		select {
		case <-timeout:
			t.Fatalf("timeout after 10s (phase=%s)", phase)
		default:
		}

		err = conn.ReadPacket(&p)
		if err != nil {
			t.Fatalf("read packet %d (%s): %v", i, phase, err)
		}

		switch phase {
		case "login":
			if p.ID == 0x03 { // SetCompression
				var threshold pk.VarInt
				p.Scan(&threshold)
				conn.SetThreshold(int(threshold))
			}
			if p.ID == 0x02 { // LoginSuccess
				// Send LoginAcknowledged to transition to config
				conn.WritePacket(pk.Marshal(0x03))
				phase = "config"
			}

		case "config":
			if p.ID == int32(packetid.ClientboundConfigSelectKnownPacks) {
				// NeoForge client sends extra CustomPayload packets BEFORE
				// responding to SelectKnownPacks.

				// 1. minecraft:register — channel list (null-separated)
				channels := []byte("minecraft:brand\x00c:version\x00c:register")
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundConfigCustomPayload),
					pk.Identifier("minecraft:register"),
					pk.PluginMessageData(channels),
				))

				// 2. c:version — convention API version (VarInt 1)
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundConfigCustomPayload),
					pk.Identifier("c:version"),
					pk.PluginMessageData([]byte{0x01}), // VarInt(1)
				))

				// 3. c:register — convention channel registration
				cChannels := []byte("c:version\x00c:register")
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundConfigCustomPayload),
					pk.Identifier("c:register"),
					pk.PluginMessageData(cChannels),
				))

				// Now respond to SelectKnownPacks normally
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigSelectKnownPacks),
					pk.VarInt(1),
					pk.String("minecraft"),
					pk.String("core"),
					pk.String("26.1-snapshot-2"),
				))
			}
			if p.ID == int32(packetid.ClientboundConfigFinishConfiguration) {
				// NeoForge may send another CustomPayload before ack
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundConfigCustomPayload),
					pk.Identifier("neoforge:register"),
					pk.PluginMessageData([]byte{}),
				))

				// Now send FinishConfiguration ack
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigFinishConfiguration)))
				phase = "play"
			}

		case "play":
			// We entered play phase — verify we get JoinGame
			if p.ID == int32(packetid.ClientboundLogin) {
				t.Log("PASS: NeoForge config phase completed — received JoinGame in play phase")
				return
			}
		}
	}
	t.Fatal("Did not receive JoinGame packet in play phase")
}

// TestRawHandshake tests the lowest-level protocol exchange manually.
func TestRawHandshake(t *testing.T) {
	addr := startTestServer(t)

	conn, err := mcnet.DialMC(addr)
	if err != nil {
		t.Fatalf("DialMC failed: %v", err)
	}
	defer conn.Close()

	err = conn.WritePacket(pk.Marshal(
		0x00,
		pk.VarInt(server.ProtocolVersion),
		pk.String("127.0.0.1"),
		pk.UnsignedShort(25565),
		pk.VarInt(1),
	))
	if err != nil {
		t.Fatalf("Handshake write failed: %v", err)
	}

	err = conn.WritePacket(pk.Marshal(0x00))
	if err != nil {
		t.Fatalf("Status request write failed: %v", err)
	}

	var p pk.Packet
	err = conn.ReadPacket(&p)
	if err != nil {
		t.Fatalf("Status response read failed: %v", err)
	}

	var jsonStr pk.String
	if err := p.Scan(&jsonStr); err != nil {
		t.Fatalf("Failed to scan status response: %v", err)
	}

	t.Logf("Raw handshake response: %s", jsonStr)
	t.Log("PASS: Raw handshake + status exchange works")
}
