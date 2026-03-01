// Command testbot is a minimal raw-protocol bot for testing multi-player visibility
// and the command system. It connects to a 26.1-snapshot-2 server, completes
// login+config, then periodically sends chat messages and commands.
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Tnze/go-mc/chat/sign"
	"github.com/Tnze/go-mc/data/packetid"
	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/server"
)

func main() {
	addr := "127.0.0.1:25565"
	name := "TestBot"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	if len(os.Args) > 2 {
		name = os.Args[2]
	}

	log.Printf("Connecting to %s as %s...", addr, name)

	conn, err := mcnet.DialMC(addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Handshake
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.VarInt(server.ProtocolVersion),
		pk.String("127.0.0.1"),
		pk.UnsignedShort(25565),
		pk.VarInt(2), // login
	))
	if err != nil {
		log.Fatalf("handshake: %v", err)
	}

	// Login Start
	err = conn.WritePacket(pk.Marshal(0x00,
		pk.String(name),
		pk.UUID{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
			0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00},
	))
	if err != nil {
		log.Fatalf("login start: %v", err)
	}

	phase := "login"
	var p pk.Packet
	var mu sync.Mutex // protects conn writes

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		conn.Close()
		os.Exit(0)
	}()

	// sendChat sends a chat message (ServerboundChat packet)
	sendChat := func(msg string) {
		mu.Lock()
		defer mu.Unlock()
		history := sign.HistoryUpdate{
			Acknowledged: pk.NewFixedBitSet(20),
		}
		err := conn.WritePacket(pk.Marshal(
			int32(packetid.ServerboundChat),
			pk.String(msg),
			pk.Long(time.Now().UnixMilli()),
			pk.Long(0),          // salt
			pk.Boolean(false),   // no signature
			history,
		))
		if err != nil {
			log.Printf("Failed to send chat: %v", err)
		}
	}

	// sendCommand sends a command (ServerboundChatCommand packet)
	sendCommand := func(cmd string) {
		mu.Lock()
		defer mu.Unlock()
		err := conn.WritePacket(pk.Marshal(
			int32(packetid.ServerboundChatCommand),
			pk.String(cmd),
		))
		if err != nil {
			log.Printf("Failed to send command: %v", err)
		}
	}

	// Start repeating chat/command loop once in play phase
	startChatLoop := func() {
		messages := []struct {
			chat string // if non-empty, send as chat
			cmd  string // if non-empty, send as command
		}{
			{"Hello! I'm a bot testing the command system.", ""},
			{"", "help"},
			{"", "gamemode creative"},
			{"Flying around in creative mode!", ""},
			{"", "tp 100 200 100"},
			{"", "say Announcement from TestBot!"},
			{"", "gamemode survival"},
			{"Back to survival mode.", ""},
			{"", "gm sp"},
			{"Now in spectator!", ""},
			{"", "gamemode survival"},
			{"", "kill"},
			{"Ouch! That hurt.", ""},
		}

		go func() {
			round := 0
			for {
				round++
				log.Printf(">>> Starting message round %d", round)
				for _, m := range messages {
					time.Sleep(8 * time.Second)
					if m.chat != "" {
						log.Printf(">>> Sending chat: %s", m.chat)
						sendChat(m.chat)
					}
					if m.cmd != "" {
						log.Printf(">>> Sending command: /%s", m.cmd)
						sendCommand(m.cmd)
					}
				}
				log.Printf(">>> Round %d complete, waiting before next round...", round)
				time.Sleep(15 * time.Second)
			}
		}()
	}

	chatLoopStarted := false

	for {
		err = conn.ReadPacket(&p)
		if err != nil {
			log.Fatalf("read packet (%s): %v", phase, err)
		}

		switch phase {
		case "login":
			if p.ID == 0x03 { // SetCompression
				var threshold pk.VarInt
				p.Scan(&threshold)
				conn.SetThreshold(int(threshold))
				log.Printf("Compression threshold: %d", threshold)
			}
			if p.ID == 0x02 { // LoginSuccess
				conn.WritePacket(pk.Marshal(0x03)) // LoginAcknowledged
				phase = "config"
				log.Println("Login success, entering config phase")
			}

		case "config":
			switch packetid.ClientboundPacketID(p.ID) {
			case packetid.ClientboundConfigSelectKnownPacks:
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigSelectKnownPacks),
					pk.VarInt(1),
					pk.String("minecraft"),
					pk.String("core"),
					pk.String(server.ProtocolName),
				))
				log.Println("Responded to SelectKnownPacks")

			case packetid.ClientboundConfigFinishConfiguration:
				conn.WritePacket(pk.Marshal(int32(packetid.ServerboundConfigFinishConfiguration)))
				phase = "play"
				log.Println("Config complete, entering play phase")
			}

		case "play":
			switch packetid.ClientboundPacketID(p.ID) {
			case packetid.ClientboundLogin:
				log.Println("Received JoinGame")

			case packetid.ClientboundPlayerPosition:
				var teleportID pk.VarInt
				p.Scan(&teleportID)
				mu.Lock()
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundAcceptTeleportation),
					teleportID,
				))
				mu.Unlock()
				log.Printf("Accepted teleport %d", teleportID)

			case packetid.ClientboundKeepAlive:
				var keepAliveID pk.Long
				p.Scan(&keepAliveID)
				mu.Lock()
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundKeepAlive),
					keepAliveID,
				))
				mu.Unlock()

			case packetid.ClientboundChunkBatchFinished:
				mu.Lock()
				conn.WritePacket(pk.Marshal(
					int32(packetid.ServerboundChunkBatchReceived),
					pk.Float(20.0),
				))
				mu.Unlock()
				log.Println("Acknowledged chunk batch")
				// Start chat loop after first chunk batch
				if !chatLoopStarted {
					chatLoopStarted = true
					startChatLoop()
				}

			case packetid.ClientboundSystemChat:
				// Log system messages (command feedback)
				var msg pk.String
				p.Scan(&msg)
				log.Printf("<<< System: %s", msg)

			case packetid.ClientboundDisguisedChat:
				// Log chat messages
				var msg pk.String
				p.Scan(&msg)
				log.Printf("<<< Chat: %s", msg)

			case packetid.ClientboundPlayerCombatKill:
				log.Println("<<< Player died!")

			case packetid.ClientboundSetHealth:
				var health pk.Float
				var food pk.VarInt
				p.Scan(&health, &food)
				log.Printf("<<< Health: %.1f, Food: %d", health, food)

			case packetid.ClientboundGameEvent:
				var event pk.UnsignedByte
				var value pk.Float
				p.Scan(&event, &value)
				log.Printf("<<< GameEvent: event=%d value=%.1f", event, value)

			case packetid.ClientboundPlayerAbilities:
				var flags pk.Byte
				p.Scan(&flags)
				log.Printf("<<< PlayerAbilities: flags=0x%02X", flags)

			case packetid.ClientboundPlayerInfoUpdate:
				log.Println("Received PlayerInfoUpdate")

			case packetid.ClientboundAddEntity:
				var eid pk.VarInt
				var uuid pk.UUID
				var entityType pk.VarInt
				p.Scan(&eid, &uuid, &entityType)
				log.Printf("Received AddEntity: eid=%d type=%d", eid, entityType)

			case packetid.ClientboundRemoveEntities:
				log.Println("Received RemoveEntities")

			case packetid.ClientboundPlayerInfoRemove:
				log.Println("Received PlayerInfoRemove")

			case packetid.ClientboundMoveEntityPos,
				packetid.ClientboundMoveEntityPosRot,
				packetid.ClientboundMoveEntityRot:
				fmt.Print(".")

			default:
				// Silently discard
			}
		}
	}
}
