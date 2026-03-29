package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/store"
	"github.com/Tnze/go-mc/level/block"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SignData stores the text content for a placed sign.
type SignData struct {
	Lines    [4]string
	X, Y, Z int
}

// SignManager manages sign block entities (text storage, editor packets).
type SignManager struct {
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex
	signs   map[[3]int]*SignData
}

// NewSignManager creates a new SignManager.
func NewSignManager(mgr *game.PlayerManager, world game.World, logger *log.Logger) *SignManager {
	return &SignManager{
		Manager: mgr,
		World:   world,
		Logger:  logger,
		signs:   make(map[[3]int]*SignData),
	}
}

// PlaceSign creates an empty sign at the given position and opens the sign editor
// for the player. Called after the sign block has been placed in the world.
func (sm *SignManager) PlaceSign(player *game.Player, x, y, z int) {
	sm.mu.Lock()
	pos := [3]int{x, y, z}
	sm.signs[pos] = &SignData{X: x, Y: y, Z: z}
	sm.mu.Unlock()

	// Send ClientboundOpenSignEditor to the placing player
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenSignEditor,
		pk.Position{X: x, Y: y, Z: z},
		pk.Boolean(true), // isFrontText
	))
	player.EditingSignPos = &[3]int{x, y, z}
}

// HandleSignUpdate processes ServerboundSignUpdate from a player.
func (sm *SignManager) HandleSignUpdate(player *game.Player, p pk.Packet) {
	var pos pk.Position
	var isFrontText pk.Boolean
	var line0, line1, line2, line3 pk.String
	if err := p.Scan(&pos, &isFrontText, &line0, &line1, &line2, &line3); err != nil {
		sm.logf("Error parsing ServerboundSignUpdate from %s: %v", player.Name, err)
		return
	}

	key := [3]int{pos.X, pos.Y, pos.Z}
	sm.mu.Lock()
	sign, exists := sm.signs[key]
	if !exists {
		sign = &SignData{X: pos.X, Y: pos.Y, Z: pos.Z}
		sm.signs[key] = sign
	}
	sign.Lines[0] = string(line0)
	sign.Lines[1] = string(line1)
	sign.Lines[2] = string(line2)
	sign.Lines[3] = string(line3)
	sm.mu.Unlock()

	player.EditingSignPos = nil

	// Broadcast sign block entity data to all players
	sm.broadcastSignBlockEntity(pos.X, pos.Y, pos.Z, sign)

	sm.logf("Player %s updated sign at (%d, %d, %d): %q %q %q %q",
		player.Name, pos.X, pos.Y, pos.Z,
		sign.Lines[0], sign.Lines[1], sign.Lines[2], sign.Lines[3])
}

// SendSignData sends sign block entity data for a specific position to a player,
// if a sign exists there. Used when streaming chunks to late-joining players.
func (sm *SignManager) SendSignData(player *game.Player, x, y, z int) {
	sm.mu.Lock()
	sign, exists := sm.signs[[3]int{x, y, z}]
	sm.mu.Unlock()
	if !exists {
		return
	}

	nbtData := buildSignNBT(sign)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockEntityData,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(7), // block entity type 7 = sign
		pk.PluginMessageData(nbtData),
	))
}

// SendAllSignsInChunk sends sign block entity data for all signs in a given chunk
// to a specific player. Called after a chunk is sent.
func (sm *SignManager) SendAllSignsInChunk(player *game.Player, chunkX, chunkZ int) {
	sm.mu.Lock()
	var toSend []*SignData
	for pos, sign := range sm.signs {
		cx := pos[0] >> 4
		cz := pos[2] >> 4
		if cx == chunkX && cz == chunkZ {
			toSend = append(toSend, sign)
		}
	}
	sm.mu.Unlock()

	for _, sign := range toSend {
		nbtData := buildSignNBT(sign)
		player.WritePacket(pk.Marshal(
			packetid.ClientboundBlockEntityData,
			pk.Position{X: sign.X, Y: sign.Y, Z: sign.Z},
			pk.VarInt(7),
			pk.PluginMessageData(nbtData),
		))
	}
}

// SaveAll serializes all sign states for persistence.
func (sm *SignManager) SaveAll(dim string) []store.BlockEntityData {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	var result []store.BlockEntityData
	for pos, sign := range sm.signs {
		data, err := json.Marshal(map[string]interface{}{
			"lines": sign.Lines,
		})
		if err != nil {
			continue
		}
		result = append(result, store.BlockEntityData{
			Dimension: dim, X: pos[0], Y: pos[1], Z: pos[2],
			Type: "sign", Data: data,
		})
	}
	return result
}

// LoadAll restores sign states from persisted data.
func (sm *SignManager) LoadAll(entities []store.BlockEntityData) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, e := range entities {
		if e.Type != "sign" {
			continue
		}
		var raw struct {
			Lines [4]string `json:"lines"`
		}
		if err := json.Unmarshal(e.Data, &raw); err != nil {
			continue
		}
		sm.signs[[3]int{e.X, e.Y, e.Z}] = &SignData{
			Lines: raw.Lines,
			X:     e.X, Y: e.Y, Z: e.Z,
		}
	}
}

// RemoveSign removes sign data when a sign block is broken.
func (sm *SignManager) RemoveSign(x, y, z int) {
	sm.mu.Lock()
	delete(sm.signs, [3]int{x, y, z})
	sm.mu.Unlock()
}

// broadcastSignBlockEntity sends ClientboundBlockEntityData for a sign to all players.
func (sm *SignManager) broadcastSignBlockEntity(x, y, z int, sign *SignData) {
	nbtData := buildSignNBT(sign)
	pkt := pk.Marshal(
		packetid.ClientboundBlockEntityData,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(7), // block entity type 7 = sign
		pk.PluginMessageData(nbtData),
	)
	sm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// buildSignNBT encodes sign text data as NBT in network format.
// The format matches vanilla 26.1 sign block entity data.
func buildSignNBT(sign *SignData) []byte {
	// Build the sign NBT structure matching vanilla format.
	// Each line is a JSON chat component string.
	frontMessages := make([]string, 4)
	backMessages := make([]string, 4)
	for i := 0; i < 4; i++ {
		if sign.Lines[i] != "" {
			frontMessages[i] = `{"text":"` + escapeJSON(sign.Lines[i]) + `"}`
		} else {
			frontMessages[i] = `{"text":""}`
		}
		backMessages[i] = `{"text":""}`
	}

	signNBT := map[string]interface{}{
		"front_text": map[string]interface{}{
			"messages":         frontMessages,
			"color":            "black",
			"has_glowing_text": byte(0),
		},
		"back_text": map[string]interface{}{
			"messages":         backMessages,
			"color":            "black",
			"has_glowing_text": byte(0),
		},
		"is_waxed": byte(0),
	}

	var buf bytes.Buffer
	enc := nbt.NewEncoder(&buf)
	enc.NetworkFormat(true)
	if err := enc.Encode(signNBT, ""); err != nil {
		return nil
	}
	return buf.Bytes()
}

// escapeJSON escapes special characters for embedding in a JSON string value.
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// isSignItem returns true if the item name refers to a placeable sign item
// (standing or wall signs, not hanging signs).
func isSignItem(name string) bool {
	if strings.Contains(name, "hanging_sign") {
		return false
	}
	return strings.HasSuffix(name, "_sign")
}

// isHangingSignItem returns true if the item name refers to a hanging sign item.
func isHangingSignItem(name string) bool {
	return strings.HasSuffix(name, "_hanging_sign")
}

// hangingSignBlockForItem returns the ceiling hanging sign block name for an item, or "".
func hangingSignBlockForItem(itemName string) string {
	switch itemName {
	case "oak_hanging_sign":
		return "minecraft:oak_hanging_sign"
	case "spruce_hanging_sign":
		return "minecraft:spruce_hanging_sign"
	case "birch_hanging_sign":
		return "minecraft:birch_hanging_sign"
	case "jungle_hanging_sign":
		return "minecraft:jungle_hanging_sign"
	case "acacia_hanging_sign":
		return "minecraft:acacia_hanging_sign"
	case "cherry_hanging_sign":
		return "minecraft:cherry_hanging_sign"
	case "dark_oak_hanging_sign":
		return "minecraft:dark_oak_hanging_sign"
	case "mangrove_hanging_sign":
		return "minecraft:mangrove_hanging_sign"
	case "bamboo_hanging_sign":
		return "minecraft:bamboo_hanging_sign"
	case "crimson_hanging_sign":
		return "minecraft:crimson_hanging_sign"
	case "warped_hanging_sign":
		return "minecraft:warped_hanging_sign"
	}
	return ""
}

// wallHangingSignBlockForItem returns the wall hanging sign block name for an item, or "".
func wallHangingSignBlockForItem(itemName string) string {
	switch itemName {
	case "oak_hanging_sign":
		return "minecraft:oak_wall_hanging_sign"
	case "spruce_hanging_sign":
		return "minecraft:spruce_wall_hanging_sign"
	case "birch_hanging_sign":
		return "minecraft:birch_wall_hanging_sign"
	case "jungle_hanging_sign":
		return "minecraft:jungle_wall_hanging_sign"
	case "acacia_hanging_sign":
		return "minecraft:acacia_wall_hanging_sign"
	case "cherry_hanging_sign":
		return "minecraft:cherry_wall_hanging_sign"
	case "dark_oak_hanging_sign":
		return "minecraft:dark_oak_wall_hanging_sign"
	case "mangrove_hanging_sign":
		return "minecraft:mangrove_wall_hanging_sign"
	case "bamboo_hanging_sign":
		return "minecraft:bamboo_wall_hanging_sign"
	case "crimson_hanging_sign":
		return "minecraft:crimson_wall_hanging_sign"
	case "warped_hanging_sign":
		return "minecraft:warped_wall_hanging_sign"
	}
	return ""
}

// signBlockForItem returns the block name (with minecraft: prefix) for a standing
// sign item name, or "" if not recognized.
func signBlockForItem(itemName string) string {
	switch itemName {
	case "oak_sign":
		return "minecraft:oak_sign"
	case "spruce_sign":
		return "minecraft:spruce_sign"
	case "birch_sign":
		return "minecraft:birch_sign"
	case "jungle_sign":
		return "minecraft:jungle_sign"
	case "acacia_sign":
		return "minecraft:acacia_sign"
	case "cherry_sign":
		return "minecraft:cherry_sign"
	case "dark_oak_sign":
		return "minecraft:dark_oak_sign"
	case "mangrove_sign":
		return "minecraft:mangrove_sign"
	case "bamboo_sign":
		return "minecraft:bamboo_sign"
	case "crimson_sign":
		return "minecraft:crimson_sign"
	case "warped_sign":
		return "minecraft:warped_sign"
	}
	return ""
}

// wallSignBlockForItem returns the wall sign block name for a sign item, or "".
func wallSignBlockForItem(itemName string) string {
	switch itemName {
	case "oak_sign":
		return "minecraft:oak_wall_sign"
	case "spruce_sign":
		return "minecraft:spruce_wall_sign"
	case "birch_sign":
		return "minecraft:birch_wall_sign"
	case "jungle_sign":
		return "minecraft:jungle_wall_sign"
	case "acacia_sign":
		return "minecraft:acacia_wall_sign"
	case "cherry_sign":
		return "minecraft:cherry_wall_sign"
	case "dark_oak_sign":
		return "minecraft:dark_oak_wall_sign"
	case "mangrove_sign":
		return "minecraft:mangrove_wall_sign"
	case "bamboo_sign":
		return "minecraft:bamboo_wall_sign"
	case "crimson_sign":
		return "minecraft:crimson_wall_sign"
	case "warped_sign":
		return "minecraft:warped_wall_sign"
	}
	return ""
}

// yawToSignRotation converts player yaw to a sign rotation value (0-15).
// rotation = floor((yaw + 180) * 16 / 360) % 16
func yawToSignRotation(yaw float32) int {
	y := float64(yaw)
	r := math.Floor((y+180.0)*16.0/360.0)
	rot := int(r) % 16
	if rot < 0 {
		rot += 16
	}
	return rot
}

// faceToWallSignDirection maps a clicked block face to the wall sign facing direction.
// Face 2=North, 3=South, 4=West, 5=East. Returns the direction and whether it's valid.
func faceToWallSignDirection(face int) (block.Direction, bool) {
	switch face {
	case 2:
		return block.North, true
	case 3:
		return block.South, true
	case 4:
		return block.West, true
	case 5:
		return block.East, true
	}
	return 0, false
}

func (sm *SignManager) logf(format string, args ...any) {
	if sm.Logger != nil {
		sm.Logger.Printf(format, args...)
	}
}
