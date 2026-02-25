package main

import "github.com/Tnze/go-mc/server" // used by vanillaRegistryKeys

// vanillaRegistryKeys returns the registry entry keys for vanilla 26.1-snapshot-2.
// These are extracted from the server JAR's built-in data pack. When the client
// confirms it knows the "minecraft:core" pack via SelectKnownPacks, we send
// these keys with has_data=false so the client loads its built-in data.
func vanillaRegistryKeys() []server.RegistryKeys {
	return []server.RegistryKeys{
		{ID: "minecraft:dimension_type", Entries: prefixAll("minecraft:", dimensionTypeKeys)},
		{ID: "minecraft:worldgen/biome", Entries: prefixAll("minecraft:", biomeKeys)},
		{ID: "minecraft:chat_type", Entries: prefixAll("minecraft:", chatTypeKeys)},
		{ID: "minecraft:damage_type", Entries: prefixAll("minecraft:", damageTypeKeys)},
		{ID: "minecraft:banner_pattern", Entries: prefixAll("minecraft:", bannerPatternKeys)},
		{ID: "minecraft:cat_variant", Entries: prefixAll("minecraft:", catVariantKeys)},
		{ID: "minecraft:chicken_variant", Entries: prefixAll("minecraft:", chickenVariantKeys)},
		{ID: "minecraft:cow_variant", Entries: prefixAll("minecraft:", cowVariantKeys)},
		{ID: "minecraft:enchantment", Entries: prefixAll("minecraft:", enchantmentKeys)},
		{ID: "minecraft:frog_variant", Entries: prefixAll("minecraft:", frogVariantKeys)},
		{ID: "minecraft:instrument", Entries: prefixAll("minecraft:", instrumentKeys)},
		{ID: "minecraft:jukebox_song", Entries: prefixAll("minecraft:", jukeboxSongKeys)},
		{ID: "minecraft:painting_variant", Entries: prefixAll("minecraft:", paintingVariantKeys)},
		{ID: "minecraft:pig_variant", Entries: prefixAll("minecraft:", pigVariantKeys)},
		{ID: "minecraft:timeline", Entries: prefixAll("minecraft:", timelineKeys)},
		{ID: "minecraft:trim_material", Entries: prefixAll("minecraft:", trimMaterialKeys)},
		{ID: "minecraft:trim_pattern", Entries: prefixAll("minecraft:", trimPatternKeys)},
		{ID: "minecraft:wolf_sound_variant", Entries: prefixAll("minecraft:", wolfSoundVariantKeys)},
		{ID: "minecraft:wolf_variant", Entries: prefixAll("minecraft:", wolfVariantKeys)},
		{ID: "minecraft:zombie_nautilus_variant", Entries: prefixAll("minecraft:", zombieNautilusVariantKeys)},
	}
}

func prefixAll(prefix string, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = prefix + k
	}
	return out
}

var dimensionTypeKeys = []string{
	"overworld", "overworld_caves", "the_end", "the_nether",
}

var biomeKeys = []string{
	"badlands", "bamboo_jungle", "basalt_deltas", "beach", "birch_forest",
	"cherry_grove", "cold_ocean", "crimson_forest", "dark_forest",
	"deep_cold_ocean", "deep_dark", "deep_frozen_ocean", "deep_lukewarm_ocean",
	"deep_ocean", "desert", "dripstone_caves", "end_barrens", "end_highlands",
	"end_midlands", "eroded_badlands", "flower_forest", "forest", "frozen_ocean",
	"frozen_peaks", "frozen_river", "grove", "ice_spikes", "jagged_peaks",
	"jungle", "lukewarm_ocean", "lush_caves", "mangrove_swamp", "meadow",
	"mushroom_fields", "nether_wastes", "ocean", "old_growth_birch_forest",
	"old_growth_pine_taiga", "old_growth_spruce_taiga", "pale_garden", "plains",
	"river", "savanna", "savanna_plateau", "small_end_islands", "snowy_beach",
	"snowy_plains", "snowy_slopes", "snowy_taiga", "soul_sand_valley",
	"sparse_jungle", "stony_peaks", "stony_shore", "sunflower_plains", "swamp",
	"taiga", "the_end", "the_void", "warm_ocean", "warped_forest",
	"windswept_forest", "windswept_gravelly_hills", "windswept_hills",
	"windswept_savanna", "wooded_badlands",
}

var chatTypeKeys = []string{
	"chat", "emote_command", "msg_command_incoming", "msg_command_outgoing",
	"say_command", "team_msg_command_incoming", "team_msg_command_outgoing",
}

var damageTypeKeys = []string{
	"arrow", "bad_respawn_point", "cactus", "campfire", "cramming",
	"dragon_breath", "drown", "dry_out", "ender_pearl", "explosion", "fall",
	"falling_anvil", "falling_block", "falling_stalactite", "fireball",
	"fireworks", "fly_into_wall", "freeze", "generic", "generic_kill",
	"hot_floor", "in_fire", "in_wall", "indirect_magic", "lava",
	"lightning_bolt", "mace_smash", "magic", "mob_attack",
	"mob_attack_no_aggro", "mob_projectile", "on_fire", "out_of_world",
	"outside_border", "player_attack", "player_explosion", "sonic_boom",
	"spear", "spit", "stalagmite", "starve", "sting", "sweet_berry_bush",
	"thorns", "thrown", "trident", "unattributed_fireball", "wind_charge",
	"wither", "wither_skull",
}

var bannerPatternKeys = []string{
	"base", "border", "bricks", "circle", "creeper", "cross", "curly_border",
	"diagonal_left", "diagonal_right", "diagonal_up_left", "diagonal_up_right",
	"flow", "flower", "globe", "gradient", "gradient_up", "guster",
	"half_horizontal", "half_horizontal_bottom", "half_vertical",
	"half_vertical_right", "mojang", "piglin", "rhombus", "skull",
	"small_stripes", "square_bottom_left", "square_bottom_right",
	"square_top_left", "square_top_right", "straight_cross", "stripe_bottom",
	"stripe_center", "stripe_downleft", "stripe_downright", "stripe_left",
	"stripe_middle", "stripe_right", "stripe_top", "triangle_bottom",
	"triangle_top", "triangles_bottom", "triangles_top",
}

var catVariantKeys = []string{
	"all_black", "black", "british_shorthair", "calico", "jellie", "persian",
	"ragdoll", "red", "siamese", "tabby", "white",
}

var chickenVariantKeys = []string{"cold", "temperate", "warm"}
var cowVariantKeys = []string{"cold", "temperate", "warm"}

var enchantmentKeys = []string{
	"aqua_affinity", "bane_of_arthropods", "binding_curse", "blast_protection",
	"breach", "channeling", "density", "depth_strider", "efficiency",
	"feather_falling", "fire_aspect", "fire_protection", "flame", "fortune",
	"frost_walker", "impaling", "infinity", "knockback", "looting", "loyalty",
	"luck_of_the_sea", "lunge", "lure", "mending", "multishot", "piercing",
	"power", "projectile_protection", "protection", "punch", "quick_charge",
	"respiration", "riptide", "sharpness", "silk_touch", "smite", "soul_speed",
	"sweeping_edge", "swift_sneak", "thorns", "unbreaking", "vanishing_curse",
	"wind_burst",
}

var frogVariantKeys = []string{"cold", "temperate", "warm"}

var instrumentKeys = []string{
	"admire_goat_horn", "call_goat_horn", "dream_goat_horn", "feel_goat_horn",
	"ponder_goat_horn", "seek_goat_horn", "sing_goat_horn", "yearn_goat_horn",
}

var jukeboxSongKeys = []string{
	"11", "13", "5", "blocks", "cat", "chirp", "creator", "creator_music_box",
	"far", "lava_chicken", "mall", "mellohi", "otherside", "pigstep",
	"precipice", "relic", "stal", "strad", "tears", "wait", "ward",
}

var paintingVariantKeys = []string{
	"alban", "aztec", "aztec2", "backyard", "baroque", "bomb", "bouquet",
	"burning_skull", "bust", "cavebird", "changing", "cotan", "courbet",
	"creebet", "dennis", "donkey_kong", "earth", "endboss", "fern", "fighters",
	"finding", "fire", "graham", "humble", "kebab", "lowmist", "match",
	"meditative", "orb", "owlemons", "passage", "pigscene", "plant", "pointer",
	"pond", "pool", "prairie_ride", "sea", "skeleton", "skull_and_roses",
	"stage", "sunflowers", "sunset", "tides", "unpacked", "void", "wanderer",
	"wasteland", "water", "wind", "wither",
}

var pigVariantKeys = []string{"cold", "temperate", "warm"}

var trimMaterialKeys = []string{
	"amethyst", "copper", "diamond", "emerald", "gold", "iron", "lapis",
	"netherite", "quartz", "redstone", "resin",
}

var trimPatternKeys = []string{
	"bolt", "coast", "dune", "eye", "flow", "host", "raiser", "rib", "sentry",
	"shaper", "silence", "snout", "spire", "tide", "vex", "ward", "wayfinder",
	"wild",
}

var wolfSoundVariantKeys = []string{
	"angry", "big", "classic", "cute", "grumpy", "puglin", "sad",
}

var wolfVariantKeys = []string{
	"ashen", "black", "chestnut", "pale", "rusty", "snowy", "spotted",
	"striped", "woods",
}

var timelineKeys = []string{"day", "early_game", "moon", "villager_schedule"}

var zombieNautilusVariantKeys = []string{"temperate", "warm"}

// vanillaConfigTags is defined in tags_generated.go (generated by cmd/taggen).
