package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/server"
)

// connectToPlayPhase connects to the test server, completes login+config,
// and returns the connection in the play phase along with the first play packet.
// The caller is responsible for closing the connection.
func connectToPlayPhase(t *testing.T, addr string) (*mcnet.Conn, []pk.Packet) {
	t.Helper()

	conn, err := mcnet.DialMC(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	port := 25565
	fmt.Sscanf(addr, "127.0.0.1:%d", &port)

	// Handshake
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.VarInt(server.ProtocolVersion),
		pk.String("127.0.0.1"),
		pk.UnsignedShort(uint16(port)),
		pk.VarInt(2),
	))
	if err != nil {
		conn.Close()
		t.Fatalf("handshake: %v", err)
	}

	// Login Start
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.String("ProtoBot"),
		pk.UUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
	))
	if err != nil {
		conn.Close()
		t.Fatalf("login start: %v", err)
	}

	phase := "login"
	var p pk.Packet
	var playPackets []pk.Packet

	timeout := time.After(10 * time.Second)
	for i := 0; i < 5000; i++ {
		select {
		case <-timeout:
			conn.Close()
			t.Fatalf("timeout after 10s (phase=%s)", phase)
		default:
		}

		err = conn.ReadPacket(&p)
		if err != nil {
			conn.Close()
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
			// Save a copy of the packet data since it may be overwritten
			saved := pk.Packet{
				ID:   p.ID,
				Data: append([]byte(nil), p.Data...),
			}
			playPackets = append(playPackets, saved)

			// Collect until we see ChunkBatchFinished
			if p.ID == int32(packetid.ClientboundChunkBatchFinished) {
				return conn, playPackets
			}
		}
	}

	conn.Close()
	t.Fatal("Did not reach ChunkBatchFinished")
	return nil, nil
}

// TestPlayerPositionPacket verifies the ClientboundPlayerPosition packet format.
func TestPlayerPositionPacket(t *testing.T) {
	addr := startTestServer(t)
	conn, packets := connectToPlayPhase(t, addr)
	defer conn.Close()

	var found bool
	for _, p := range packets {
		if p.ID != int32(packetid.ClientboundPlayerPosition) {
			continue
		}
		found = true
		r := bytes.NewReader(p.Data)

		// VarInt teleport ID
		teleportID := readVarInt261(r)
		if teleportID <= 0 {
			t.Errorf("teleportID = %d, want > 0", teleportID)
		}

		// Double x, y, z
		var x, y, z float64
		binary.Read(r, binary.BigEndian, &x)
		binary.Read(r, binary.BigEndian, &y)
		binary.Read(r, binary.BigEndian, &z)

		// Double vx, vy, vz (velocity)
		var vx, vy, vz float64
		binary.Read(r, binary.BigEndian, &vx)
		binary.Read(r, binary.BigEndian, &vy)
		binary.Read(r, binary.BigEndian, &vz)

		// Float yaw, pitch
		var yaw, pitch float32
		binary.Read(r, binary.BigEndian, &yaw)
		binary.Read(r, binary.BigEndian, &pitch)

		// Int flags
		var flags int32
		binary.Read(r, binary.BigEndian, &flags)

		if r.Len() != 0 {
			t.Errorf("PlayerPosition: %d extra bytes remaining after parsing", r.Len())
		}

		t.Logf("PASS: PlayerPosition — teleportID=%d pos=(%.1f, %.1f, %.1f) vel=(%.1f, %.1f, %.1f) yaw=%.1f pitch=%.1f flags=%d",
			teleportID, x, y, z, vx, vy, vz, yaw, pitch, flags)
		break
	}

	if !found {
		t.Fatal("ClientboundPlayerPosition packet not found in play phase")
	}
}

// TestSpawnSequenceOrder verifies the order of packets received matches vanilla 26.1.
func TestSpawnSequenceOrder(t *testing.T) {
	addr := startTestServer(t)
	conn, packets := connectToPlayPhase(t, addr)
	defer conn.Close()

	if len(packets) == 0 {
		t.Fatal("No packets received in play phase")
	}

	// Build a list of relevant packet IDs in order
	type entry struct {
		name string
		id   int32
	}
	var sequence []entry

	for _, p := range packets {
		switch p.ID {
		case int32(packetid.ClientboundLogin):
			sequence = append(sequence, entry{"Login", p.ID})
		case int32(packetid.ClientboundPlayerAbilities):
			sequence = append(sequence, entry{"PlayerAbilities", p.ID})
		case int32(packetid.ClientboundPlayerPosition):
			sequence = append(sequence, entry{"PlayerPosition", p.ID})
		case int32(packetid.ClientboundSetDefaultSpawnPosition):
			sequence = append(sequence, entry{"SetDefaultSpawnPosition", p.ID})
		case int32(packetid.ClientboundGameEvent):
			if len(p.Data) >= 1 && p.Data[0] == 13 {
				sequence = append(sequence, entry{"GameEvent(13)", p.ID})
			}
		case int32(packetid.ClientboundChunkBatchStart):
			sequence = append(sequence, entry{"ChunkBatchStart", p.ID})
		case int32(packetid.ClientboundChunkBatchFinished):
			sequence = append(sequence, entry{"ChunkBatchFinished", p.ID})
		}
	}

	t.Logf("Spawn sequence (%d relevant packets):", len(sequence))
	for i, e := range sequence {
		t.Logf("  %d. %s (0x%02X)", i+1, e.name, e.id)
	}

	// Verify Login comes first
	if len(sequence) == 0 || sequence[0].name != "Login" {
		t.Errorf("Expected Login to be first packet, got %v", sequence)
	}

	// Helper to find index in sequence
	indexOf := func(name string) int {
		for i, e := range sequence {
			if e.name == name {
				return i
			}
		}
		return -1
	}

	loginIdx := indexOf("Login")
	abilitiesIdx := indexOf("PlayerAbilities")
	positionIdx := indexOf("PlayerPosition")
	spawnPosIdx := indexOf("SetDefaultSpawnPosition")
	gameEventIdx := indexOf("GameEvent(13)")
	batchStartIdx := indexOf("ChunkBatchStart")
	batchFinishedIdx := indexOf("ChunkBatchFinished")

	// All must be present
	for _, check := range []struct {
		name string
		idx  int
	}{
		{"Login", loginIdx},
		{"PlayerAbilities", abilitiesIdx},
		{"PlayerPosition", positionIdx},
		{"SetDefaultSpawnPosition", spawnPosIdx},
		{"GameEvent(13)", gameEventIdx},
		{"ChunkBatchStart", batchStartIdx},
		{"ChunkBatchFinished", batchFinishedIdx},
	} {
		if check.idx < 0 {
			t.Errorf("%s packet not found in spawn sequence", check.name)
		}
	}

	// Verify ordering constraints
	if loginIdx >= 0 && abilitiesIdx >= 0 && loginIdx >= abilitiesIdx {
		t.Errorf("Login (idx=%d) should come before PlayerAbilities (idx=%d)", loginIdx, abilitiesIdx)
	}
	if abilitiesIdx >= 0 && positionIdx >= 0 && abilitiesIdx >= positionIdx {
		t.Errorf("PlayerAbilities (idx=%d) should come before PlayerPosition (idx=%d)", abilitiesIdx, positionIdx)
	}
	if positionIdx >= 0 && batchStartIdx >= 0 && positionIdx >= batchStartIdx {
		t.Errorf("PlayerPosition (idx=%d) should come before ChunkBatchStart (idx=%d)", positionIdx, batchStartIdx)
	}
	if spawnPosIdx >= 0 && batchStartIdx >= 0 && spawnPosIdx >= batchStartIdx {
		t.Errorf("SetDefaultSpawnPosition (idx=%d) should come before ChunkBatchStart (idx=%d)", spawnPosIdx, batchStartIdx)
	}
	if gameEventIdx >= 0 && batchStartIdx >= 0 && gameEventIdx >= batchStartIdx {
		t.Errorf("GameEvent(13) (idx=%d) should come before ChunkBatchStart (idx=%d)", gameEventIdx, batchStartIdx)
	}
	if batchStartIdx >= 0 && batchFinishedIdx >= 0 && batchStartIdx >= batchFinishedIdx {
		t.Errorf("ChunkBatchStart (idx=%d) should come before ChunkBatchFinished (idx=%d)", batchStartIdx, batchFinishedIdx)
	}

	t.Log("PASS: Spawn sequence order matches vanilla 26.1")
}

// TestSetDefaultSpawnPositionFormat verifies the 26.1 RespawnData format.
func TestSetDefaultSpawnPositionFormat(t *testing.T) {
	addr := startTestServer(t)
	conn, packets := connectToPlayPhase(t, addr)
	defer conn.Close()

	var found bool
	for _, p := range packets {
		if p.ID != int32(packetid.ClientboundSetDefaultSpawnPosition) {
			continue
		}
		found = true
		r := bytes.NewReader(p.Data)

		// Identifier (dimension name) — VarInt-prefixed UTF-8 string
		dimLen := readVarInt261(r)
		if dimLen <= 0 || dimLen > 256 {
			t.Fatalf("SetDefaultSpawnPosition: dimension name length = %d, expected 1..256", dimLen)
		}
		dimBytes := make([]byte, dimLen)
		n, err := r.Read(dimBytes)
		if err != nil || n != dimLen {
			t.Fatalf("SetDefaultSpawnPosition: failed to read dimension name: read %d bytes, err=%v", n, err)
		}
		dimName := string(dimBytes)
		if dimName != "minecraft:overworld" {
			t.Errorf("SetDefaultSpawnPosition: dimension = %q, want %q", dimName, "minecraft:overworld")
		}

		// Position — packed 64-bit block position
		var posVal int64
		if err := binary.Read(r, binary.BigEndian, &posVal); err != nil {
			t.Fatalf("SetDefaultSpawnPosition: failed to read Position: %v", err)
		}
		// Decode packed position: x (26 bits) | z (26 bits) | y (12 bits)
		posX := int32(posVal >> 38)
		posZ := int32(posVal << 26 >> 38)
		posY := int32(posVal << 52 >> 52)
		t.Logf("SetDefaultSpawnPosition: position = (%d, %d, %d)", posX, posY, posZ)

		// Float yaw
		var yaw float32
		if err := binary.Read(r, binary.BigEndian, &yaw); err != nil {
			t.Fatalf("SetDefaultSpawnPosition: failed to read yaw: %v", err)
		}

		// Float pitch
		var pitch float32
		if err := binary.Read(r, binary.BigEndian, &pitch); err != nil {
			t.Fatalf("SetDefaultSpawnPosition: failed to read pitch: %v", err)
		}

		// Verify no remaining bytes
		if r.Len() != 0 {
			t.Errorf("SetDefaultSpawnPosition: %d extra bytes remaining", r.Len())
		}

		t.Logf("PASS: SetDefaultSpawnPosition — dim=%s pos=(%d,%d,%d) yaw=%.1f pitch=%.1f",
			dimName, posX, posY, posZ, yaw, pitch)
		break
	}

	if !found {
		t.Fatal("ClientboundSetDefaultSpawnPosition packet not found")
	}
}

// TestChunkBatchFinishedFormat verifies the ChunkBatchFinished packet contains VarInt batchSize.
func TestChunkBatchFinishedFormat(t *testing.T) {
	addr := startTestServer(t)
	conn, packets := connectToPlayPhase(t, addr)
	defer conn.Close()

	var found bool
	for _, p := range packets {
		if p.ID != int32(packetid.ClientboundChunkBatchFinished) {
			continue
		}
		found = true
		r := bytes.NewReader(p.Data)

		batchSize := readVarInt261(r)
		if batchSize <= 0 {
			t.Errorf("ChunkBatchFinished: batchSize = %d, expected > 0", batchSize)
		}

		if r.Len() != 0 {
			t.Errorf("ChunkBatchFinished: %d extra bytes remaining after VarInt batchSize", r.Len())
		}

		t.Logf("PASS: ChunkBatchFinished — batchSize=%d", batchSize)
		break
	}

	if !found {
		t.Fatal("ClientboundChunkBatchFinished packet not found")
	}
}

// TestLoginPacketSeaLevel verifies the ClientboundLogin packet includes VarInt seaLevel.
func TestLoginPacketSeaLevel(t *testing.T) {
	addr := startTestServer(t)
	conn, packets := connectToPlayPhase(t, addr)
	defer conn.Close()

	var found bool
	for _, p := range packets {
		if p.ID != int32(packetid.ClientboundLogin) {
			continue
		}
		found = true
		r := bytes.NewReader(p.Data)

		// Int entityID (4 bytes)
		var entityID int32
		binary.Read(r, binary.BigEndian, &entityID)

		// Boolean isHardcore
		readByte261(r)

		// Array of dimension names: VarInt count, then count x String
		dimCount := readVarInt261(r)
		for i := 0; i < dimCount; i++ {
			strLen := readVarInt261(r)
			skip := make([]byte, strLen)
			r.Read(skip)
		}

		// VarInt maxPlayers, viewDistance, simulationDistance
		readVarInt261(r)
		readVarInt261(r)
		readVarInt261(r)

		// Boolean reducedDebugInfo
		readByte261(r)

		// Boolean enableRespawnScreen
		readByte261(r)

		// Boolean doLimitedCrafting
		readByte261(r)

		// VarInt dimensionType
		readVarInt261(r)

		// Identifier dimensionName
		dimNameLen := readVarInt261(r)
		dimNameBytes := make([]byte, dimNameLen)
		r.Read(dimNameBytes)

		// Long hashedSeed
		var hashedSeed int64
		binary.Read(r, binary.BigEndian, &hashedSeed)

		// UnsignedByte gamemode
		readByte261(r)

		// Byte previousGamemode
		readByte261(r)

		// Boolean isDebug
		readByte261(r)

		// Boolean isFlat
		readByte261(r)

		// Boolean hasDeathLocation
		hasDeathLocation := readByte261(r)
		if hasDeathLocation != 0 {
			// Skip death location data: Identifier + Position
			deathDimLen := readVarInt261(r)
			skip := make([]byte, deathDimLen)
			r.Read(skip)
			var deathPos int64
			binary.Read(r, binary.BigEndian, &deathPos)
		}

		// VarInt portalCooldown
		readVarInt261(r)

		// VarInt seaLevel — the field added in 1.21.2
		seaLevel := readVarInt261(r)
		if seaLevel != 63 {
			t.Errorf("Login: seaLevel = %d, want 63", seaLevel)
		}

		// Boolean enforcesSecureChat
		readByte261(r)

		// Verify no remaining bytes
		if r.Len() != 0 {
			t.Errorf("Login: %d extra bytes remaining after parsing all fields", r.Len())
		}

		t.Logf("PASS: Login packet — entityID=%d seaLevel=%d (remaining=%d)", entityID, seaLevel, r.Len())
		break
	}

	if !found {
		t.Fatal("ClientboundLogin packet not found")
	}
}
