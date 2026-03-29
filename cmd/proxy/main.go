// Command proxy is a TCP proxy that sits between Minecraft clients and region servers.
// It handles Mojang authentication, routes players to the correct backend, and manages
// player transfers when they cross region boundaries.
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/cluster"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/registry"
	"github.com/Tnze/go-mc/server"
	"github.com/Tnze/go-mc/server/vanilla"

	"github.com/redis/go-redis/v9"
)

func main() {
	logger := log.New(os.Stdout, "[proxy] ", log.LstdFlags|log.Lmsgprefix)

	cfg, err := cluster.LoadConfigFromEnv()
	if err != nil {
		logger.Fatalf("Failed to load cluster config: %v", err)
	}
	logger.Printf("Loaded cluster config: %d servers, default=%s", len(cfg.Servers), cfg.DefaultServer)

	// Connect to Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Fatalf("Invalid REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatalf("Redis ping failed: %v", err)
	}
	logger.Println("Connected to Redis")

	regs := buildProxyRegistries()

	healthChecker := NewHealthChecker(logger)
	go healthChecker.Run(ctx, cfg)

	gameplay := &ProxyGamePlay{
		Config:  cfg,
		Redis:   rdb,
		Logger:  logger,
		Health:  healthChecker,
	}

	srv := server.Server{
		Logger: logger,
		ListPingHandler: &proxyPingHandler{},
		LoginHandler: &server.MojangLoginHandler{
			OnlineMode: true,
			Threshold:  256,
		},
		ConfigHandler: &server.Configurations{
			Registries: regs,
			KnownPacks: []server.KnownPack{
				{Namespace: "minecraft", ID: "core", Version: server.ProtocolName},
			},
			KnownPackEntries: vanilla.RegistryKeys(),
			Tags:             vanilla.ConfigTags(),
			Logger:           logger,
		},
		GamePlay: gameplay,
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":25565"
	}

	logger.Printf("Proxy listening on %s", listenAddr)
	if err := srv.Listen(listenAddr); err != nil {
		logger.Fatalf("Server error: %v", err)
	}
}

// proxyPingHandler implements server.ListPingHandler for the proxy.
type proxyPingHandler struct{}

func (p *proxyPingHandler) Name() string               { return server.ProtocolName }
func (p *proxyPingHandler) Protocol(int32) int          { return server.ProtocolVersion }
func (p *proxyPingHandler) MaxPlayer() int              { return 100 }
func (p *proxyPingHandler) OnlinePlayer() int           { return 0 }
func (p *proxyPingHandler) PlayerSamples() []server.PlayerSample { return nil }
func (p *proxyPingHandler) Description() *chat.Message {
	msg := chat.Text("go-mc cluster proxy")
	return &msg
}
func (p *proxyPingHandler) FavIcon() string { return "" }

// buildProxyRegistries creates the minimal registry set needed for the config phase.
// These must match what the backend servers use.
func buildProxyRegistries() registry.Registries {
	regs := registry.NewNetworkCodec()

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
	regs.DamageType.Put("minecraft:fall", registry.DamageType{
		MessageID:  "fall",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.0,
	})
	regs.DamageType.Put("minecraft:player_attack", registry.DamageType{
		MessageID:  "player",
		Scaling:    "when_caused_by_living_non_player",
		Exhaustion: 0.1,
	})
	regs.DamageType.Put("minecraft:out_of_world", registry.DamageType{
		MessageID:  "outOfWorld",
		Scaling:    "never",
		Exhaustion: 0.0,
	})

	return regs
}

func mustNBTRaw(v any) nbt.RawMessage {
	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)
	if err := enc.Encode(v, ""); err != nil {
		panic(fmt.Sprintf("nbt.Encode: %v", err))
	}
	data := buf.Bytes()
	return nbt.RawMessage{
		Type: data[0],
		Data: append([]byte(nil), data[1:]...),
	}
}
