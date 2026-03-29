package handler

import (
	"bytes"
	"encoding/json"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

// BannerPattern represents a single pattern layer on a banner.
type BannerPattern struct {
	Pattern string // e.g. "minecraft:stripe_bottom"
	Color   string // e.g. "red"
}

// BannerData holds the state of a placed banner.
type BannerData struct {
	X, Y, Z   int
	BaseColor int32 // dye color index 0-15
	Patterns  []BannerPattern
}

// BannerManager tracks placed banners and their pattern data.
type BannerManager struct {
	Manager *game.PlayerManager
	World   game.World
	mu      sync.Mutex
	banners map[[3]int]*BannerData
}

// NewBannerManager creates a new BannerManager.
func NewBannerManager(mgr *game.PlayerManager, world game.World) *BannerManager {
	return &BannerManager{
		Manager: mgr,
		World:   world,
		banners: make(map[[3]int]*BannerData),
	}
}

// PlaceBanner registers a banner with pattern data at the given position.
func (bm *BannerManager) PlaceBanner(x, y, z int, baseColor int32, patterns []BannerPattern) {
	bm.mu.Lock()
	bm.banners[[3]int{x, y, z}] = &BannerData{
		X: x, Y: y, Z: z,
		BaseColor: baseColor,
		Patterns:  patterns,
	}
	bm.mu.Unlock()

	// Broadcast block entity data to all players
	bm.broadcastBannerBlockEntity(x, y, z)
}

// RemoveBanner removes banner data when the block is broken.
func (bm *BannerManager) RemoveBanner(x, y, z int) {
	bm.mu.Lock()
	delete(bm.banners, [3]int{x, y, z})
	bm.mu.Unlock()
}

// SendBannerToPlayer sends banner block entity data for a specific banner.
func (bm *BannerManager) SendBannerToPlayer(player *game.Player, x, y, z int) {
	bm.mu.Lock()
	banner, exists := bm.banners[[3]int{x, y, z}]
	bm.mu.Unlock()
	if !exists {
		return
	}

	nbtData := buildBannerNBT(banner)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockEntityData,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(20), // block entity type 20 = banner
		pk.PluginMessageData(nbtData),
	))
}

// SendAllBannersInChunk sends banner data for all banners in a chunk to a player.
func (bm *BannerManager) SendAllBannersInChunk(player *game.Player, chunkX, chunkZ int) {
	bm.mu.Lock()
	var toSend []*BannerData
	for pos, banner := range bm.banners {
		cx := pos[0] >> 4
		cz := pos[2] >> 4
		if cx == chunkX && cz == chunkZ {
			toSend = append(toSend, banner)
		}
	}
	bm.mu.Unlock()

	for _, banner := range toSend {
		nbtData := buildBannerNBT(banner)
		player.WritePacket(pk.Marshal(
			packetid.ClientboundBlockEntityData,
			pk.Position{X: banner.X, Y: banner.Y, Z: banner.Z},
			pk.VarInt(20),
			pk.PluginMessageData(nbtData),
		))
	}
}

// persistedBannerPattern is the JSON format for a banner pattern.
type persistedBannerPattern struct {
	Pattern string `json:"pattern"`
	Color   string `json:"color"`
}

// SaveAll serializes all banner states for persistence.
func (bm *BannerManager) SaveAll(dim string) []store.BlockEntityData {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	var result []store.BlockEntityData
	for pos, banner := range bm.banners {
		patterns := make([]persistedBannerPattern, len(banner.Patterns))
		for i, p := range banner.Patterns {
			patterns[i] = persistedBannerPattern{Pattern: p.Pattern, Color: p.Color}
		}
		data, err := json.Marshal(map[string]interface{}{
			"base_color": banner.BaseColor,
			"patterns":   patterns,
		})
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "banner", Data: data,
		})
	}
	return result
}

// LoadAll restores banner states from persisted data.
func (bm *BannerManager) LoadAll(entities []store.BlockEntityData) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for _, e := range entities {
		if e.Type != "banner" {
			continue
		}
		var raw struct {
			BaseColor int32                    `json:"base_color"`
			Patterns  []persistedBannerPattern `json:"patterns"`
		}
		if err := json.Unmarshal(e.Data, &raw); err != nil {
			continue
		}
		patterns := make([]BannerPattern, len(raw.Patterns))
		for i, p := range raw.Patterns {
			patterns[i] = BannerPattern{Pattern: p.Pattern, Color: p.Color}
		}
		bm.banners[[3]int{e.X, e.Y, e.Z}] = &BannerData{
			X: e.X, Y: e.Y, Z: e.Z,
			BaseColor: raw.BaseColor,
			Patterns:  patterns,
		}
	}
}

// broadcastBannerBlockEntity sends banner block entity data to all players.
func (bm *BannerManager) broadcastBannerBlockEntity(x, y, z int) {
	bm.mu.Lock()
	banner, exists := bm.banners[[3]int{x, y, z}]
	bm.mu.Unlock()
	if !exists {
		return
	}

	nbtData := buildBannerNBT(banner)
	pkt := pk.Marshal(
		packetid.ClientboundBlockEntityData,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(20),
		pk.PluginMessageData(nbtData),
	)
	bm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// bannerNBTData is the NBT structure for banner block entity data.
type bannerNBTData struct {
	Patterns []bannerPatternNBT `nbt:"patterns"`
}

type bannerPatternNBT struct {
	Pattern string `nbt:"pattern"`
	Color   string `nbt:"color"`
}

// buildBannerNBT encodes banner pattern data as NBT.
func buildBannerNBT(banner *BannerData) []byte {
	if len(banner.Patterns) == 0 {
		// Empty compound for no patterns
		var buf bytes.Buffer
		nbt.NewEncoder(&buf).Encode(struct{}{}, "")
		return buf.Bytes()
	}

	data := bannerNBTData{
		Patterns: make([]bannerPatternNBT, len(banner.Patterns)),
	}
	for i, p := range banner.Patterns {
		data.Patterns[i] = bannerPatternNBT{
			Pattern: p.Pattern,
			Color:   p.Color,
		}
	}

	var buf bytes.Buffer
	nbt.NewEncoder(&buf).Encode(data, "")
	return buf.Bytes()
}

// bannerColorFromBlockName extracts the base color index from a banner block name.
func bannerColorFromBlockName(name string) int32 {
	switch name {
	case "white_banner", "white_wall_banner":
		return 0
	case "orange_banner", "orange_wall_banner":
		return 1
	case "magenta_banner", "magenta_wall_banner":
		return 2
	case "light_blue_banner", "light_blue_wall_banner":
		return 3
	case "yellow_banner", "yellow_wall_banner":
		return 4
	case "lime_banner", "lime_wall_banner":
		return 5
	case "pink_banner", "pink_wall_banner":
		return 6
	case "gray_banner", "gray_wall_banner":
		return 7
	case "light_gray_banner", "light_gray_wall_banner":
		return 8
	case "cyan_banner", "cyan_wall_banner":
		return 9
	case "purple_banner", "purple_wall_banner":
		return 10
	case "blue_banner", "blue_wall_banner":
		return 11
	case "brown_banner", "brown_wall_banner":
		return 12
	case "green_banner", "green_wall_banner":
		return 13
	case "red_banner", "red_wall_banner":
		return 14
	case "black_banner", "black_wall_banner":
		return 15
	default:
		return 0
	}
}
