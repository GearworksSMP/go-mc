// Command taggen generates Go source for vanilla 26.1-snapshot-2 config-phase
// UpdateTags data. It reads the generated reports (registries.json) and tag
// files from the extracted server JAR data pack.
//
// Usage:
//
//	go run ./cmd/taggen
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// registryDump is the format of registries.json
type registryDump map[string]registryInfo

type registryInfo struct {
	ProtocolID int                     `json:"protocol_id"`
	Entries    map[string]entryInfo    `json:"entries"`
}

type entryInfo struct {
	ProtocolID int `json:"protocol_id"`
}

// tagFile is the format of a tag JSON file
type tagFile struct {
	Values []json.RawMessage `json:"values"`
}

// configTagRegistries are the registries whose tags are sent during the
// configuration phase. This excludes worldgen and villager_trade (server-side only).
// dynamicEntries: if non-nil, provides the entry order for dynamic registries
// (those not in registries.json). Entries are "minecraft:" prefixed names.
var configTagRegistries = []struct {
	dir            string   // directory name under tags/
	registryID     string   // full registry identifier
	dynamicEntries []string // nil = use registries.json; non-nil = use these as ordered entries
}{
	{"block", "minecraft:block", nil},
	{"item", "minecraft:item", nil},
	{"fluid", "minecraft:fluid", nil},
	{"entity_type", "minecraft:entity_type", nil},
	{"game_event", "minecraft:game_event", nil},
	{"enchantment", "minecraft:enchantment", prefixAll("minecraft:", enchantmentKeys)},
	{"banner_pattern", "minecraft:banner_pattern", prefixAll("minecraft:", bannerPatternKeys)},
	{"damage_type", "minecraft:damage_type", prefixAll("minecraft:", damageTypeKeys)},
	{"instrument", "minecraft:instrument", prefixAll("minecraft:", instrumentKeys)},
	{"painting_variant", "minecraft:painting_variant", prefixAll("minecraft:", paintingVariantKeys)},
	{"point_of_interest_type", "minecraft:point_of_interest_type", nil},
	{"potion", "minecraft:potion", nil},
	{"timeline", "minecraft:timeline", prefixAll("minecraft:", timelineKeys)},
	{"dialog", "minecraft:dialog", nil},
}

func prefixAll(prefix string, keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = prefix + k
	}
	return out
}

// Dynamic registry entry keys (same order as sent in RegistryData KnownPackEntries)
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

var instrumentKeys = []string{
	"admire_goat_horn", "call_goat_horn", "dream_goat_horn", "feel_goat_horn",
	"ponder_goat_horn", "seek_goat_horn", "sing_goat_horn", "yearn_goat_horn",
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

var timelineKeys = []string{"day", "early_game", "moon", "villager_schedule"}

func main() {
	cacheDir := ".cache"
	reportsFile := filepath.Join(cacheDir, "generated", "reports", "registries.json")
	tagsDir := filepath.Join(cacheDir, "extracted", "data", "minecraft", "tags")

	// 1. Load registries.json
	registries, err := loadRegistries(reportsFile)
	if err != nil {
		log.Fatalf("Failed to load registries: %v", err)
	}

	// 2. For each config tag registry, load and resolve tags
	var allTags []registryTagOutput

	for _, reg := range configTagRegistries {
		regDir := filepath.Join(tagsDir, reg.dir)
		if _, err := os.Stat(regDir); os.IsNotExist(err) {
			continue
		}

		// Build name→protocolID map
		nameToID := make(map[string]int)
		if reg.dynamicEntries != nil {
			// Dynamic registry: protocol IDs = order in our entry list
			for i, name := range reg.dynamicEntries {
				nameToID[name] = i
			}
		} else {
			// Static registry: use registries.json
			regInfo, ok := registries[reg.registryID]
			if !ok {
				log.Printf("WARNING: registry %s not found in registries.json, skipping", reg.registryID)
				continue
			}
			for name, info := range regInfo.Entries {
				nameToID[name] = info.ProtocolID
			}
		}

		// Load all tag files for this registry
		allTagFiles, err := loadAllTags(regDir, "")
		if err != nil {
			log.Fatalf("Failed to load tags for %s: %v", reg.registryID, err)
		}

		// Resolve all tag references recursively
		resolved := resolveAllTags(allTagFiles)

		// Convert to protocol IDs
		var tags []tagOutput
		for tagName, entries := range resolved {
			fullTagName := "minecraft:" + tagName
			var ids []int
			for _, entry := range entries {
				id, ok := nameToID[entry]
				if !ok {
					log.Printf("WARNING: entry %s not found in registry %s", entry, reg.registryID)
					continue
				}
				ids = append(ids, id)
			}
			sort.Ints(ids)
			tags = append(tags, tagOutput{Name: fullTagName, IDs: ids})
		}
		sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })

		if len(tags) > 0 {
			allTags = append(allTags, registryTagOutput{
				RegistryID: reg.registryID,
				Tags:       tags,
			})
		}
	}

	// 3. Generate Go source
	generateGoSource(allTags)
}

type registryTagOutput struct {
	RegistryID string
	Tags       []tagOutput
}

type tagOutput struct {
	Name string
	IDs  []int
}

func loadRegistries(path string) (registryDump, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var dump registryDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, err
	}
	return dump, nil
}

// loadAllTags recursively loads all tag JSON files from a directory.
// prefix is the path prefix for nested tags (e.g., "enchantable/" for item/enchantable/).
func loadAllTags(dir string, prefix string) (map[string][]string, error) {
	tags := make(map[string][]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories
			subTags, err := loadAllTags(filepath.Join(dir, entry.Name()), prefix+entry.Name()+"/")
			if err != nil {
				return nil, err
			}
			for k, v := range subTags {
				tags[k] = v
			}
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		tagName := prefix + strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var tf tagFile
		if err := json.Unmarshal(data, &tf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", tagName, err)
		}

		var values []string
		for _, raw := range tf.Values {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				// Could be an object like {"id": "...", "required": false}
				var obj struct {
					ID       string `json:"id"`
					Required bool   `json:"required"`
				}
				if err2 := json.Unmarshal(raw, &obj); err2 != nil {
					log.Printf("WARNING: skipping value in tag %s: %s", tagName, string(raw))
					continue
				}
				s = obj.ID
			}
			values = append(values, s)
		}
		tags[tagName] = values
	}

	return tags, nil
}

// resolveAllTags resolves all #tag references to concrete entry names.
func resolveAllTags(tags map[string][]string) map[string][]string {
	resolved := make(map[string][]string)
	resolving := make(map[string]bool) // cycle detection

	var resolve func(tagName string) []string
	resolve = func(tagName string) []string {
		if result, ok := resolved[tagName]; ok {
			return result
		}
		if resolving[tagName] {
			log.Printf("WARNING: circular tag reference for %s", tagName)
			return nil
		}
		resolving[tagName] = true

		values, ok := tags[tagName]
		if !ok {
			log.Printf("WARNING: tag %s not found", tagName)
			return nil
		}

		seen := make(map[string]bool)
		var result []string
		for _, v := range values {
			if strings.HasPrefix(v, "#minecraft:") {
				// Tag reference - resolve recursively
				refName := strings.TrimPrefix(v, "#minecraft:")
				for _, entry := range resolve(refName) {
					if !seen[entry] {
						seen[entry] = true
						result = append(result, entry)
					}
				}
			} else {
				// Direct entry
				if !seen[v] {
					seen[v] = true
					result = append(result, v)
				}
			}
		}

		resolved[tagName] = result
		delete(resolving, tagName)
		return result
	}

	for tagName := range tags {
		resolve(tagName)
	}
	return resolved
}

func generateGoSource(allTags []registryTagOutput) {
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].RegistryID < allTags[j].RegistryID })

	fmt.Println("// Code generated by cmd/taggen; DO NOT EDIT.")
	fmt.Println("")
	fmt.Println("package main")
	fmt.Println("")
	fmt.Println("import \"github.com/Tnze/go-mc/server\"")
	fmt.Println("")
	fmt.Println("// vanillaConfigTags returns all tags for the configuration-phase")
	fmt.Println("// UpdateTags packet for vanilla 26.1-snapshot-2.")
	fmt.Println("func vanillaConfigTags() []server.RegistryTagData {")
	fmt.Println("\treturn []server.RegistryTagData{")

	for _, reg := range allTags {
		fmt.Printf("\t\t{\n")
		fmt.Printf("\t\t\tRegistryID: %q,\n", reg.RegistryID)
		fmt.Printf("\t\t\tTags: []server.TagData{\n")
		for _, tag := range reg.Tags {
			fmt.Printf("\t\t\t\t{Name: %q, Entries: []int32{", tag.Name)
			for i, id := range tag.IDs {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Print(id)
			}
			fmt.Println("}},")
		}
		fmt.Printf("\t\t\t},\n")
		fmt.Printf("\t\t},\n")
	}

	fmt.Println("\t}")
	fmt.Println("}")
}
