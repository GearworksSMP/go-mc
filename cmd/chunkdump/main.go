// Command chunkdump connects to a Minecraft server and dumps the play-phase
// spawn packet sequence plus the first chunk packet, for protocol analysis.
package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
)

// packetName returns a human-readable name for clientbound game packet IDs.
func packetName(id int32) string {
	names := map[int32]string{
		int32(packetid.BundleDelimiter):                   "BundleDelimiter",
		int32(packetid.ClientboundChunkBatchFinished):      "ChunkBatchFinished",
		int32(packetid.ClientboundChunkBatchStart):         "ChunkBatchStart",
		int32(packetid.ClientboundGameEvent):               "GameEvent",
		int32(packetid.ClientboundKeepAlive):               "KeepAlive",
		int32(packetid.ClientboundLevelChunkWithLight):     "LevelChunkWithLight",
		int32(packetid.ClientboundLogin):                   "Login",
		int32(packetid.ClientboundPlayerPosition):          "PlayerPosition",
		int32(packetid.ClientboundPlayerRotation):          "PlayerRotation",
		int32(packetid.ClientboundSetChunkCacheCenter):     "SetChunkCacheCenter",
		int32(packetid.ClientboundSetDefaultSpawnPosition): "SetDefaultSpawnPosition",
		int32(packetid.ClientboundSetChunkCacheRadius):     "SetChunkCacheRadius",
		int32(packetid.ClientboundPlayerAbilities):         "PlayerAbilities",
		int32(packetid.ClientboundPlayerInfoUpdate):        "PlayerInfoUpdate",
		int32(packetid.ClientboundCommands):                "Commands",
		int32(packetid.ClientboundRecipeBookAdd):           "RecipeBookAdd",
		int32(packetid.ClientboundSetExperience):           "SetExperience",
		int32(packetid.ClientboundSetHealth):               "SetHealth",
		int32(packetid.ClientboundContainerSetContent):     "ContainerSetContent",
		int32(packetid.ClientboundSetEntityData):           "SetEntityData",
		int32(packetid.ClientboundUpdateAdvancements):      "UpdateAdvancements",
		int32(packetid.ClientboundChangeDifficulty):        "ChangeDifficulty",
		int32(packetid.ClientboundServerData):              "ServerData",
	}
	if name, ok := names[id]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(0x%02X)", id)
}

func main() {
	addr := "localhost:25565"
	outPrefix := "chunk_ours"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	if len(os.Args) > 2 {
		outPrefix = os.Args[2]
	}

	// Determine port for handshake
	port := uint16(25565)
	if parts := strings.SplitN(addr, ":", 2); len(parts) == 2 {
		var p int
		fmt.Sscanf(parts[1], "%d", &p)
		port = uint16(p)
	}

	log.Printf("Connecting to %s (output prefix: %s)...", addr, outPrefix)
	conn, err := net.DialMC(addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Handshake → login
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.VarInt(1073742112),
		pk.String("localhost"),
		pk.UnsignedShort(port),
		pk.VarInt(2),
	))
	if err != nil {
		log.Fatalf("handshake: %v", err)
	}

	// Login Start
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.String("DumpBot"),
		pk.UUID{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89},
	))
	if err != nil {
		log.Fatalf("login start: %v", err)
	}

	phase := "login"
	var p pk.Packet
	chunkCount := 0
	for i := 0; i < 5000; i++ {
		err = conn.ReadPacket(&p)
		if err != nil {
			log.Fatalf("read packet %d (%s): %v", i, phase, err)
		}

		switch phase {
		case "login":
			fmt.Printf("[login] #%d: ID=0x%02X len=%d\n", i, p.ID, len(p.Data))
			if p.ID == 0x03 { // SetCompression
				var threshold pk.VarInt
				p.Scan(&threshold)
				conn.SetThreshold(int(threshold))
			}
			if p.ID == 0x02 { // LoginSuccess
				conn.WritePacket(pk.Marshal(0x03)) // LoginAcknowledged
				phase = "config"
			}

		case "config":
			fmt.Printf("[config] #%d: ID=0x%02X len=%d\n", i, p.ID, len(p.Data))
			if p.ID == int32(packetid.ClientboundConfigSelectKnownPacks) {
				log.Printf("SelectKnownPacks (0x%02X) → responding", p.ID)
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigSelectKnownPacks),
					pk.VarInt(1),
					pk.String("minecraft"),
					pk.String("core"),
					pk.String("26.1-snapshot-2"),
				))
			}
			if p.ID == int32(packetid.ClientboundConfigFinishConfiguration) {
				log.Printf("FinishConfiguration → play phase")
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigFinishConfiguration)))
				phase = "play"
			}

		case "play":
			if p.ID == int32(packetid.ClientboundLevelChunkWithLight) {
				var x, z pk.Int
				p.Scan(&x, &z)
				chunkCount++

				// Save first chunk in full
				if chunkCount == 1 {
					log.Printf("FIRST CHUNK (%d,%d): %d bytes total", x, z, len(p.Data))
					data := p.Data[8:]
					dumpLen := 300
					if dumpLen > len(data) {
						dumpLen = len(data)
					}
					fmt.Printf("Chunk (%d,%d) payload (after coords), first %d bytes:\n%s\n",
						x, z, dumpLen, hex.Dump(data[:dumpLen]))
					os.WriteFile(outPrefix+".bin", p.Data, 0644)
					log.Printf("Saved full chunk to %s.bin", outPrefix)
				} else {
					fmt.Printf("[play] #%d: CHUNK (%d,%d) %d bytes\n", i, x, z, len(p.Data))
				}

				// After receiving a few chunks, also save the chunk batch finished
				if chunkCount >= 5 {
					log.Printf("Captured %d chunks, exiting", chunkCount)
					return
				}
				continue
			}

			name := packetName(p.ID)
			fmt.Printf("[play] #%d: ID=0x%02X (%s) len=%d\n", i, p.ID, name, len(p.Data))

			// Hex dump important packets
			switch packetid.ClientboundPacketID(p.ID) {
			case packetid.ClientboundLogin:
				dumpLen := min(len(p.Data), 200)
				fmt.Printf("  Login packet hex:\n%s\n", hex.Dump(p.Data[:dumpLen]))
			case packetid.ClientboundSetDefaultSpawnPosition:
				fmt.Printf("  SpawnPos hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundGameEvent:
				fmt.Printf("  GameEvent hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundSetChunkCacheCenter:
				fmt.Printf("  ChunkCacheCenter hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundPlayerPosition:
				fmt.Printf("  PlayerPosition hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundPlayerAbilities:
				fmt.Printf("  PlayerAbilities hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundChunkBatchStart:
				fmt.Printf("  ChunkBatchStart hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundChunkBatchFinished:
				fmt.Printf("  ChunkBatchFinished hex:\n%s\n", hex.Dump(p.Data))
			case packetid.ClientboundSetChunkCacheRadius:
				fmt.Printf("  ChunkCacheRadius hex:\n%s\n", hex.Dump(p.Data))
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
