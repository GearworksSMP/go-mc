package handler

import (
	"io"
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// MapCursor represents a cursor/icon displayed on a map.
type MapCursor struct {
	Type        int32  // cursor type: 0=white arrow, 8=monument, 9=mansion, etc.
	X           int8   // X position on map (-128 to 127)
	Z           int8   // Z position on map (-128 to 127)
	Rotation    int8   // rotation 0-15 (yaw / 360 * 16)
	DisplayName string // optional display name (empty for none)
}

// WriteTo encodes a MapCursor to the Minecraft map icon wire format.
func (mc MapCursor) WriteTo(w io.Writer) (n int64, err error) {
	return pk.Tuple{
		pk.VarInt(mc.Type),
		pk.Byte(mc.X),
		pk.Byte(mc.Z),
		pk.Byte(mc.Rotation),
		pk.Boolean(mc.DisplayName != ""),
	}.WriteTo(w)
}

// MapData holds the state of a single map.
type MapData struct {
	ID      int32
	CenterX int
	CenterZ int
	Scale   byte // 0 = 1:1 (128 blocks), 1 = 1:2 (256), etc up to 4
	Locked  bool
	Pixels  [128][128]byte // map color IDs (row-major: Pixels[z][x])
	Cursors []MapCursor    // map cursors/icons (player positions, structure markers)
	Dirty   bool
}

// MapManager tracks all active maps in the world.
type MapManager struct {
	Manager   *game.PlayerManager
	World     game.World
	mu        sync.Mutex
	nextMapID int32
	maps      map[int32]*MapData
}

// NewMapManager creates a new MapManager.
func NewMapManager(mgr *game.PlayerManager, world game.World) *MapManager {
	return &MapManager{
		Manager:   mgr,
		World:     world,
		nextMapID: 1,
		maps:      make(map[int32]*MapData),
	}
}

// CreateMap allocates a new map centered on (cx, cz) and renders it from world data.
// Returns the map ID.
func (mm *MapManager) CreateMap(cx, cz int) int32 {
	mm.mu.Lock()
	id := mm.nextMapID
	mm.nextMapID++

	md := &MapData{
		ID:      id,
		CenterX: cx,
		CenterZ: cz,
		Scale:   0,
		Dirty:   true,
	}
	mm.maps[id] = md
	mm.mu.Unlock()

	// Render initial pixel data from world blocks
	mm.renderMap(md)

	return id
}

// GetMap returns a map by ID, or nil if not found.
func (mm *MapManager) GetMap(id int32) *MapData {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	return mm.maps[id]
}

// SendMapData sends the full map data to a specific player.
func (mm *MapManager) SendMapData(player *game.Player, mapID int32) {
	mm.mu.Lock()
	md, ok := mm.maps[mapID]
	if !ok {
		mm.mu.Unlock()
		return
	}

	// Copy data under lock
	var pixels [128][128]byte
	copy(pixels[:], md.Pixels[:])
	scale := md.Scale
	locked := md.Locked
	cursors := make([]MapCursor, len(md.Cursors))
	copy(cursors, md.Cursors)
	mm.mu.Unlock()

	// Build color data (row-major, 128x128)
	colors := make([]byte, 128*128)
	for z := 0; z < 128; z++ {
		for x := 0; x < 128; x++ {
			colors[z*128+x] = pixels[z][x]
		}
	}

	// Build cursor fields
	cursorFields := make([]pk.FieldEncoder, 0, len(cursors)+1)
	cursorFields = append(cursorFields, pk.VarInt(len(cursors)))
	for _, c := range cursors {
		cursorFields = append(cursorFields, c)
	}

	fields := make([]pk.FieldEncoder, 0, 9+len(cursors))
	fields = append(fields,
		pk.VarInt(mapID),
		pk.Byte(int8(scale)),
		pk.Boolean(locked),
	)
	fields = append(fields, cursorFields...)
	fields = append(fields,
		pk.UnsignedByte(128), // columns (0 = no update, >0 = update)
		pk.UnsignedByte(128), // rows
		pk.UnsignedByte(0),   // x offset
		pk.UnsignedByte(0),   // z offset
		pk.ByteArray(colors), // color data (VarInt-prefixed)
	)

	player.WritePacket(pk.Marshal(packetid.ClientboundMapItemData, fields...))
}

// sendMapCursorsOnly sends only cursor updates (no pixel data) to a player.
func (mm *MapManager) sendMapCursorsOnly(player *game.Player, mapID int32) {
	mm.mu.Lock()
	md, ok := mm.maps[mapID]
	if !ok {
		mm.mu.Unlock()
		return
	}
	scale := md.Scale
	locked := md.Locked
	cursors := make([]MapCursor, len(md.Cursors))
	copy(cursors, md.Cursors)
	mm.mu.Unlock()

	cursorFields := make([]pk.FieldEncoder, 0, len(cursors)+1)
	cursorFields = append(cursorFields, pk.VarInt(len(cursors)))
	for _, c := range cursors {
		cursorFields = append(cursorFields, c)
	}

	fields := make([]pk.FieldEncoder, 0, 4+len(cursors))
	fields = append(fields,
		pk.VarInt(mapID),
		pk.Byte(int8(scale)),
		pk.Boolean(locked),
	)
	fields = append(fields, cursorFields...)
	fields = append(fields, pk.UnsignedByte(0)) // columns=0 means no pixel update

	player.WritePacket(pk.Marshal(packetid.ClientboundMapItemData, fields...))
}

// Tick updates maps for players holding them (called each server tick).
// tickCount should be the current server tick number.
func (mm *MapManager) Tick(tickCount int64) {
	// Update player cursors every 20 ticks (1 second)
	if tickCount%20 != 0 {
		return
	}

	mm.mu.Lock()
	mapIDs := make([]int32, 0, len(mm.maps))
	for id := range mm.maps {
		mapIDs = append(mapIDs, id)
	}
	mm.mu.Unlock()

	for _, mapID := range mapIDs {
		mm.updatePlayerCursors(mapID)
	}
}

// updatePlayerCursors recalculates player arrow cursors for a specific map
// and re-sends cursor data to all players viewing it.
func (mm *MapManager) updatePlayerCursors(mapID int32) {
	mm.mu.Lock()
	md, ok := mm.maps[mapID]
	if !ok || md.Locked {
		mm.mu.Unlock()
		return
	}
	centerX := md.CenterX
	centerZ := md.CenterZ
	scale := md.Scale
	// Keep any non-player cursors (structure markers, type >= 8)
	var structureCursors []MapCursor
	for _, c := range md.Cursors {
		if c.Type >= 8 {
			structureCursors = append(structureCursors, c)
		}
	}
	mm.mu.Unlock()

	blocksPerPixel := 1 << scale
	mapRadius := 64 * blocksPerPixel // half the map width in world blocks

	var newCursors []MapCursor
	newCursors = append(newCursors, structureCursors...)

	// Add a white arrow cursor for each player within the map's coverage
	mm.Manager.ForEach(func(p *game.Player) {
		if p.Dead {
			return
		}
		px, _, pz := p.Position()
		// Check if player is within map bounds
		dx := int(px) - centerX
		dz := int(pz) - centerZ
		if dx < -mapRadius || dx > mapRadius || dz < -mapRadius || dz > mapRadius {
			return
		}

		// Convert world position to map coordinates (-128 to 127)
		mapX := int8(clampInt(dx*128/mapRadius, -128, 127))
		mapZ := int8(clampInt(dz*128/mapRadius, -128, 127))

		// Convert yaw to rotation (0-15)
		yaw := p.Yaw
		rotation := int8(math.Floor(float64(yaw)/360.0*16.0+0.5)) & 0x0F

		newCursors = append(newCursors, MapCursor{
			Type:     0, // white arrow (player)
			X:        mapX,
			Z:        mapZ,
			Rotation: rotation,
		})
	})

	// Update cursors and send to viewers
	mm.mu.Lock()
	md2, ok := mm.maps[mapID]
	if !ok {
		mm.mu.Unlock()
		return
	}
	md2.Cursors = newCursors
	mm.mu.Unlock()

	// Send cursor-only update to all players holding this map
	mm.Manager.ForEach(func(p *game.Player) {
		if p.Dead {
			return
		}
		if isPlayerHoldingMap(p) {
			mm.sendMapCursorsOnly(p, mapID)
		}
	})
}

// AddStructureCursor adds a structure marker cursor to a map.
// cursorType: 8 = ocean monument, 9 = woodland mansion.
func (mm *MapManager) AddStructureCursor(mapID int32, cursorType int32, worldX, worldZ int) {
	mm.mu.Lock()
	md, ok := mm.maps[mapID]
	if !ok {
		mm.mu.Unlock()
		return
	}

	blocksPerPixel := 1 << md.Scale
	mapRadius := 64 * blocksPerPixel
	dx := worldX - md.CenterX
	dz := worldZ - md.CenterZ

	mapX := int8(clampInt(dx*128/mapRadius, -128, 127))
	mapZ := int8(clampInt(dz*128/mapRadius, -128, 127))

	md.Cursors = append(md.Cursors, MapCursor{
		Type: cursorType,
		X:    mapX,
		Z:    mapZ,
	})
	mm.mu.Unlock()
}

// isPlayerHoldingMap checks if a player is holding a filled_map item.
func isPlayerHoldingMap(p *game.Player) bool {
	heldSlot := int(p.HeldSlot) + 36
	if heldSlot < 0 || heldSlot >= len(p.Inventory) {
		return false
	}
	held := p.Inventory[heldSlot]
	if held.ID <= 0 || held.Count <= 0 {
		return false
	}
	return ItemNameByID(held.ID) == "filled_map"
}

// clampInt clamps v to the range [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// renderMap generates pixel data from the world blocks.
func (mm *MapManager) renderMap(md *MapData) {
	blocksPerPixel := 1 << md.Scale // scale 0 = 1 block/pixel, scale 1 = 2, etc
	startX := md.CenterX - 64*blocksPerPixel
	startZ := md.CenterZ - 64*blocksPerPixel

	for pz := 0; pz < 128; pz++ {
		for px := 0; px < 128; px++ {
			worldX := startX + px*blocksPerPixel
			worldZ := startZ + pz*blocksPerPixel

			// Find the topmost non-air block
			colorID := mm.getTopBlockColor(worldX, worldZ)
			md.Pixels[pz][px] = colorID
		}
	}
}

// getTopBlockColor scans from y=319 down to find the top block and returns its map color.
func (mm *MapManager) getTopBlockColor(x, z int) byte {
	for y := 319; y >= -64; y-- {
		state, err := mm.World.GetBlock(x, y, z)
		if err != nil || state == 0 {
			continue
		}
		if int(state) >= len(block.StateList) || block.StateList[state] == nil {
			continue
		}
		name := BlockNameFromState(int(state))
		if name == "air" || name == "cave_air" || name == "void_air" {
			continue
		}
		color := blockToMapColor(name)
		if color > 0 {
			// Apply shade based on height (simplified)
			// Base color * 4 + shade (0=dark, 1=normal, 2=light, 3=darkest)
			return color*4 + 1 // normal shade
		}
	}
	return 0 // transparent
}

// blockToMapColor maps block names to Minecraft map base color IDs.
// Base color * 4 gives the actual color index. There are 64 base colors (0-63).
// See: https://minecraft.wiki/w/Map_item_format#Color_table
func blockToMapColor(name string) byte {
	switch name {
	// Grass (base 1)
	case "grass_block", "slime_block":
		return 1
	// Sand/gravel (base 2)
	case "sand", "sandstone", "sandstone_stairs", "sandstone_slab", "sandstone_wall",
		"birch_planks", "birch_log", "birch_wood", "birch_slab", "birch_stairs",
		"birch_fence", "birch_fence_gate", "birch_door", "birch_trapdoor",
		"end_stone", "end_stone_bricks", "glowstone", "bone_block":
		return 2
	// Wool/carpet (base 3) - white
	case "cobweb", "mushroom_stem", "white_bed":
		return 3
	// Fire/lava (base 4)
	case "lava", "tnt", "fire", "redstone_block":
		return 4
	// Ice (base 5)
	case "ice", "packed_ice", "blue_ice", "frosted_ice":
		return 5
	// Metal/iron (base 6)
	case "iron_block", "iron_door", "iron_trapdoor", "iron_bars",
		"brewing_stand", "heavy_weighted_pressure_plate":
		return 6
	// Plant (base 7)
	case "short_grass", "tall_grass", "fern", "large_fern",
		"oak_leaves", "birch_leaves", "spruce_leaves", "jungle_leaves",
		"acacia_leaves", "dark_oak_leaves", "cherry_leaves",
		"azalea_leaves", "flowering_azalea_leaves",
		"sugar_cane", "lily_pad", "vine", "cactus":
		return 7
	// Snow (base 8)
	case "snow", "snow_block", "white_wool", "white_carpet",
		"white_concrete", "white_concrete_powder",
		"white_stained_glass", "white_stained_glass_pane",
		"white_glazed_terracotta", "white_shulker_box",
		"white_banner", "white_wall_banner":
		return 8
	// Clay (base 9)
	case "clay", "infested_stone":
		return 9
	// Dirt (base 10)
	case "dirt", "coarse_dirt", "rooted_dirt", "farmland",
		"jungle_planks", "jungle_log", "jungle_wood",
		"jungle_slab", "jungle_stairs", "jungle_fence":
		return 10
	// Stone (base 11)
	case "stone", "cobblestone", "gravel", "smooth_stone",
		"stone_bricks", "mossy_stone_bricks", "cracked_stone_bricks",
		"stone_slab", "stone_stairs", "cobblestone_slab", "cobblestone_stairs",
		"stone_brick_slab", "stone_brick_stairs", "stone_brick_wall",
		"cobblestone_wall", "andesite", "polished_andesite",
		"bedrock", "cauldron":
		return 11
	// Water (base 12)
	case "water":
		return 12
	// Wood/oak (base 13)
	case "oak_planks", "oak_log", "oak_wood", "oak_slab", "oak_stairs",
		"oak_fence", "oak_fence_gate", "oak_door", "oak_trapdoor",
		"crafting_table", "bookshelf", "chest", "barrel",
		"note_block", "jukebox", "composter":
		return 13
	// Quartz (base 14)
	case "quartz_block", "quartz_slab", "quartz_stairs",
		"smooth_quartz", "quartz_bricks", "quartz_pillar",
		"diorite", "polished_diorite", "sea_lantern":
		return 14
	// Orange wool (base 15)
	case "orange_wool", "orange_carpet", "orange_concrete", "orange_concrete_powder",
		"acacia_planks", "acacia_log", "acacia_wood",
		"acacia_slab", "acacia_stairs", "acacia_fence",
		"pumpkin", "carved_pumpkin", "jack_o_lantern",
		"terracotta", "red_sand", "red_sandstone":
		return 15
	// Magenta (base 16)
	case "magenta_wool", "magenta_carpet", "magenta_concrete", "magenta_concrete_powder",
		"purpur_block", "purpur_slab", "purpur_stairs", "purpur_pillar":
		return 16
	// Light blue (base 17)
	case "light_blue_wool", "light_blue_carpet", "light_blue_concrete", "light_blue_concrete_powder":
		return 17
	// Yellow (base 18)
	case "yellow_wool", "yellow_carpet", "yellow_concrete", "yellow_concrete_powder",
		"sponge", "wet_sponge", "hay_block":
		return 18
	// Lime (base 19)
	case "lime_wool", "lime_carpet", "lime_concrete", "lime_concrete_powder":
		return 19
	// Pink (base 20)
	case "pink_wool", "pink_carpet", "pink_concrete", "pink_concrete_powder":
		return 20
	// Gray (base 21)
	case "gray_wool", "gray_carpet", "gray_concrete", "gray_concrete_powder":
		return 21
	// Light gray (base 22)
	case "light_gray_wool", "light_gray_carpet", "light_gray_concrete", "light_gray_concrete_powder":
		return 22
	// Cyan (base 23)
	case "cyan_wool", "cyan_carpet", "cyan_concrete", "cyan_concrete_powder",
		"prismarine", "prismarine_slab", "prismarine_stairs", "prismarine_wall":
		return 23
	// Purple (base 24)
	case "purple_wool", "purple_carpet", "purple_concrete", "purple_concrete_powder",
		"shulker_box", "mycelium", "chorus_plant", "chorus_flower",
		"amethyst_block", "budding_amethyst":
		return 24
	// Blue (base 25)
	case "blue_wool", "blue_carpet", "blue_concrete", "blue_concrete_powder",
		"lapis_block":
		return 25
	// Brown (base 26)
	case "brown_wool", "brown_carpet", "brown_concrete", "brown_concrete_powder",
		"dark_oak_planks", "dark_oak_log", "dark_oak_wood",
		"dark_oak_slab", "dark_oak_stairs", "dark_oak_fence",
		"soul_sand", "soul_soil", "command_block":
		return 26
	// Green (base 27)
	case "green_wool", "green_carpet", "green_concrete", "green_concrete_powder",
		"moss_block", "moss_carpet", "dried_kelp_block":
		return 27
	// Red (base 28)
	case "red_wool", "red_carpet", "red_concrete", "red_concrete_powder",
		"red_mushroom_block", "bricks", "brick_slab", "brick_stairs", "brick_wall",
		"nether_wart_block":
		return 28
	// Black (base 29)
	case "black_wool", "black_carpet", "black_concrete", "black_concrete_powder",
		"obsidian", "crying_obsidian", "coal_block",
		"dragon_egg", "basalt", "polished_basalt", "smooth_basalt",
		"respawn_anchor":
		return 29
	// Gold (base 30)
	case "gold_block", "light_weighted_pressure_plate", "raw_gold_block":
		return 30
	// Diamond (base 31)
	case "diamond_block", "prismarine_bricks", "prismarine_brick_slab",
		"prismarine_brick_stairs", "dark_prismarine", "dark_prismarine_slab",
		"dark_prismarine_stairs", "beacon":
		return 31
	// Lapis (base 32)
	case "lapis_block_ore": // note: actual lapis_block is base 25 (blue)
		return 32
	// Emerald (base 33)
	case "emerald_block":
		return 33
	// Spruce (base 34)
	case "spruce_planks", "spruce_log", "spruce_wood",
		"spruce_slab", "spruce_stairs", "spruce_fence",
		"spruce_fence_gate", "spruce_door", "spruce_trapdoor",
		"podzol":
		return 34
	// Nether (base 35)
	case "netherrack", "nether_bricks", "nether_brick_slab",
		"nether_brick_stairs", "nether_brick_wall", "nether_brick_fence",
		"nether_quartz_ore", "nether_gold_ore",
		"magma_block", "crimson_nylium":
		return 35
	// Terracotta white (base 36)
	case "white_terracotta":
		return 36
	// Terracotta orange (base 37)
	case "orange_terracotta":
		return 37
	// Terracotta magenta (base 38)
	case "magenta_terracotta":
		return 38
	// Terracotta light blue (base 39)
	case "light_blue_terracotta":
		return 39
	// Terracotta yellow (base 40)
	case "yellow_terracotta":
		return 40
	// Terracotta lime (base 41)
	case "lime_terracotta":
		return 41
	// Terracotta pink (base 42)
	case "pink_terracotta":
		return 42
	// Terracotta gray (base 43)
	case "gray_terracotta":
		return 43
	// Terracotta light gray (base 44)
	case "light_gray_terracotta":
		return 44
	// Terracotta cyan (base 45)
	case "cyan_terracotta":
		return 45
	// Terracotta purple (base 46)
	case "purple_terracotta":
		return 46
	// Terracotta blue (base 47)
	case "blue_terracotta":
		return 47
	// Terracotta brown (base 48)
	case "brown_terracotta":
		return 48
	// Terracotta green (base 49)
	case "green_terracotta":
		return 49
	// Terracotta red (base 50)
	case "red_terracotta":
		return 50
	// Terracotta black (base 51)
	case "black_terracotta":
		return 51
	// Crimson nylium/stem (base 52)
	case "crimson_planks", "crimson_slab", "crimson_stairs",
		"crimson_fence", "crimson_fence_gate", "crimson_stem",
		"crimson_hyphae", "stripped_crimson_stem", "stripped_crimson_hyphae":
		return 52
	// Warped (base 53)
	case "warped_planks", "warped_slab", "warped_stairs",
		"warped_fence", "warped_fence_gate", "warped_stem",
		"warped_hyphae", "stripped_warped_stem", "stripped_warped_hyphae",
		"warped_nylium", "warped_wart_block":
		return 53
	// Deepslate (base 54)
	case "deepslate", "cobbled_deepslate", "polished_deepslate",
		"deepslate_bricks", "deepslate_tiles", "cracked_deepslate_bricks",
		"cracked_deepslate_tiles", "chiseled_deepslate",
		"cobbled_deepslate_slab", "cobbled_deepslate_stairs",
		"cobbled_deepslate_wall", "deepslate_brick_slab",
		"deepslate_brick_stairs", "deepslate_brick_wall",
		"deepslate_tile_slab", "deepslate_tile_stairs", "deepslate_tile_wall",
		"polished_deepslate_slab", "polished_deepslate_stairs",
		"polished_deepslate_wall", "reinforced_deepslate":
		return 54
	// Raw iron (base 55)
	case "raw_iron_block":
		return 55
	// Glow lichen (base 56)
	case "glow_lichen", "verdant_froglight", "ochre_froglight",
		"pearlescent_froglight", "sculk_sensor":
		return 56
	default:
		// Use a height-based scan to at least show something
		if isLogBlock(name) {
			return 13 // wood
		}
		if isLeavesBlock(name) {
			return 7 // plant
		}
		if isOreBlock(name) {
			return 11 // stone
		}
		return 0 // transparent/unknown
	}
}

func isLogBlock(name string) bool {
	l := len(name)
	return l > 4 && (name[l-4:] == "_log" || (l > 5 && name[l-5:] == "_wood"))
}

func isLeavesBlock(name string) bool {
	l := len(name)
	return l > 7 && name[l-7:] == "_leaves"
}

func isOreBlock(name string) bool {
	l := len(name)
	return l > 4 && name[l-4:] == "_ore"
}

// optimizedTopBlockScan scans downward to find the top block more efficiently.
// For generated terrain, we start at y=127 (typical surface) and scan down, then up if needed.
func (mm *MapManager) optimizedTopBlockScan(x, z int) byte {
	// Try common surface range first
	for y := 127; y >= 60; y-- {
		state, err := mm.World.GetBlock(x, y, z)
		if err != nil || state == 0 {
			continue
		}
		if int(state) >= len(block.StateList) || block.StateList[state] == nil {
			continue
		}
		name := BlockNameFromState(int(state))
		if name == "air" || name == "cave_air" || name == "void_air" {
			continue
		}
		color := blockToMapColor(name)
		if color > 0 {
			return color*4 + 1
		}
		// Known block but no color mapping — skip down
		return 0
	}
	return 0
}

