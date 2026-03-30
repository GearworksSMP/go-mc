package handler

import (
	"math/rand"
)

// Mob type IDs (entity type for AddEntity packet, 26.1-snapshot-2 registry).
const (
	MobTypeZombie   int32 = 150
	MobTypeSkeleton int32 = 115
	MobTypeCreeper  int32 = 32
	MobTypeSpider   int32 = 124

	MobTypeEnderman int32 = 41
	MobTypeWitch    int32 = 144
	MobTypeSlime    int32 = 117
	MobTypePhantom  int32 = 99

	MobTypeCow     int32 = 30
	MobTypePig     int32 = 100
	MobTypeSheep   int32 = 111
	MobTypeChicken int32 = 26

	MobTypeBlaze            int32 = 14
	MobTypeGhast            int32 = 57
	MobTypeIronGolem        int32 = 70
	MobTypeSnowGolem        int32 = 121
	MobTypeGuardian         int32 = 63
	MobTypeElderGuardian    int32 = 40
	MobTypeDrowned          int32 = 38
	MobTypeHusk             int32 = 67
	MobTypeStray            int32 = 128
	MobTypeCaveSpider       int32 = 22
	MobTypeSilverfish       int32 = 114
	MobTypeEndermite        int32 = 42
	MobTypeMagmaCube        int32 = 80
	MobTypePiglin           int32 = 101
	MobTypeZombifiedPiglin  int32 = 154
	MobTypeHoglin           int32 = 64
	MobTypeStrider          int32 = 129
	MobTypeWitherSkeleton   int32 = 146
	MobTypeShulker          int32 = 112
	MobTypePillager         int32 = 103
	MobTypeVindicator       int32 = 140
	MobTypeEvoker           int32 = 46
	MobTypeVex              int32 = 138
	MobTypeRavager          int32 = 109
	MobTypeBee              int32 = 11
	MobTypeFox              int32 = 54
	MobTypeRabbit           int32 = 108
	MobTypeBat              int32 = 10
	MobTypeWarden           int32 = 132
	MobTypeFrog             int32 = 55
	MobTypeAxolotl          int32 = 7
	MobTypeAllay            int32 = 2
	MobTypeSniffer          int32 = 119
	MobTypeTurtle           int32 = 131
	MobTypeBreeze           int32 = 16
)

// mobNameToType maps entity names to type IDs for /summon.
var mobNameToType = map[string]int32{
	"zombie": MobTypeZombie, "skeleton": MobTypeSkeleton, "creeper": MobTypeCreeper,
	"spider": MobTypeSpider, "enderman": MobTypeEnderman, "witch": MobTypeWitch,
	"slime": MobTypeSlime, "phantom": MobTypePhantom, "cow": MobTypeCow,
	"pig": MobTypePig, "sheep": MobTypeSheep, "chicken": MobTypeChicken,
	"blaze": MobTypeBlaze, "ghast": MobTypeGhast, "iron_golem": MobTypeIronGolem,
	"snow_golem": MobTypeSnowGolem, "guardian": MobTypeGuardian,
	"elder_guardian": MobTypeElderGuardian, "drowned": MobTypeDrowned,
	"husk": MobTypeHusk, "stray": MobTypeStray, "cave_spider": MobTypeCaveSpider,
	"silverfish": MobTypeSilverfish, "endermite": MobTypeEndermite,
	"magma_cube": MobTypeMagmaCube, "piglin": MobTypePiglin,
	"zombified_piglin": MobTypeZombifiedPiglin, "hoglin": MobTypeHoglin,
	"strider": MobTypeStrider, "wither_skeleton": MobTypeWitherSkeleton,
	"shulker": MobTypeShulker, "pillager": MobTypePillager,
	"vindicator": MobTypeVindicator, "evoker": MobTypeEvoker,
	"vex": MobTypeVex, "ravager": MobTypeRavager,
	"bee": MobTypeBee, "fox": MobTypeFox, "rabbit": MobTypeRabbit, "bat": MobTypeBat,
	"warden": MobTypeWarden, "frog": MobTypeFrog, "axolotl": MobTypeAxolotl,
	"allay": MobTypeAllay, "sniffer": MobTypeSniffer,
	"turtle": MobTypeTurtle,
	"llama": MobTypeLlama, "trader_llama": MobTypeTraderLlama,
	"breeze": MobTypeBreeze,
}

// MobTypeByName returns the entity type ID for a mob name, or -1 if unknown.
func MobTypeByName(name string) int32 {
	if id, ok := mobNameToType[name]; ok {
		return id
	}
	return -1
}

// mobDefaults returns default health, damage, speed, and hostile flag for a mob type.
func mobDefaults(typeID int32) (health, damage float32, speed float64, hostile bool) {
	switch typeID {
	case MobTypeZombie:
		return 20, 3, 0.115, true
	case MobTypeSkeleton:
		return 20, 2, 0.1, true
	case MobTypeCreeper:
		return 20, 0, 0.1, true
	case MobTypeSpider:
		return 16, 2, 0.15, true
	case MobTypeEnderman:
		return 40, 7, 0.15, true
	case MobTypeWitch:
		return 26, 3, 0.1, true
	case MobTypeSlime:
		return 16, 3, 0.1, true
	case MobTypeCow, MobTypePig, MobTypeSheep:
		return 10, 0, 0.1, false
	case MobTypeChicken:
		return 4, 0, 0.1, false
	case MobTypeIronGolem:
		return 100, 15, 0.1, false
	case MobTypeBlaze:
		return 20, 6, 0.1, true
	case MobTypeGhast:
		return 10, 6, 0.05, true
	case MobTypeWitherSkeleton:
		return 20, 8, 0.1, true
	case MobTypeCaveSpider:
		return 12, 2, 0.15, true
	case MobTypeWarden:
		return 500, 30, 0.3, true
	case MobTypeFrog:
		return 10, 0, 0.1, false
	case MobTypeAxolotl:
		return 14, 2, 0.1, false
	case MobTypeAllay:
		return 20, 0, 0.08, false
	case MobTypeSniffer:
		return 14, 0, 0.09, false
	case MobTypeTurtle:
		return 30, 0, 0.1, false
	case MobTypeLlama, MobTypeTraderLlama:
		return 22, 1, 0.1, false
	case MobTypeBreeze:
		return 30, 6, 0.6, true
	default:
		return 20, 3, 0.1, true
	}
}

// isSlimeType returns true if the mob type uses index 16 for slime size (not baby flag).
func isSlimeType(typeID int32) bool {
	return typeID == MobTypeSlime || typeID == MobTypeMagmaCube
}

const (
	// Per-player mob caps (vanilla-inspired).
	hostileCapPerPlayer = 70
	passiveCapPerPlayer = 10
)

// mobExtraData holds type-specific mob fields for JSON persistence.
type mobExtraData struct {
	SlimeSize  int32   `json:"slime_size,omitempty"`
	Hostile    bool    `json:"hostile,omitempty"`
	Damage     float32 `json:"damage,omitempty"`
	Speed      float64 `json:"speed,omitempty"`
	CustomName string  `json:"custom_name,omitempty"`
}

// woolColorToItem returns the wool item name for a given dye color ID.
func woolColorToItem(color int32) string {
	switch color {
	case 0:
		return "white_wool"
	case 1:
		return "orange_wool"
	case 2:
		return "magenta_wool"
	case 3:
		return "light_blue_wool"
	case 4:
		return "yellow_wool"
	case 5:
		return "lime_wool"
	case 6:
		return "pink_wool"
	case 7:
		return "gray_wool"
	case 8:
		return "light_gray_wool"
	case 9:
		return "cyan_wool"
	case 10:
		return "purple_wool"
	case 11:
		return "blue_wool"
	case 12:
		return "brown_wool"
	case 13:
		return "green_wool"
	case 14:
		return "red_wool"
	case 15:
		return "black_wool"
	default:
		return "white_wool"
	}
}

// cookedMeats maps raw meat items to their cooked variants (for fire kills).
var cookedMeats = map[string]string{
	"beef":     "cooked_beef",
	"porkchop": "cooked_porkchop",
	"mutton":   "cooked_mutton",
	"chicken":  "cooked_chicken",
	"rabbit":   "cooked_rabbit",
	"cod":      "cooked_cod",
	"salmon":   "cooked_salmon",
}

// mobXPAmount returns the vanilla XP value for a mob type.
func mobXPAmount(mob *Mob) int32 {
	switch mob.TypeID {
	case MobTypeBlaze, MobTypeBreeze:
		return 10
	case MobTypeSlime, MobTypeMagmaCube:
		if mob.SlimeSize >= 4 {
			return 4
		}
		if mob.SlimeSize >= 2 {
			return 2
		}
		return 1
	case MobTypeCow, MobTypePig, MobTypeSheep, MobTypeChicken:
		return int32(1 + rand.Intn(3))
	case MobTypeBat:
		return 0
	default:
		if mob.Hostile {
			return 5
		}
		return int32(1 + rand.Intn(3))
	}
}
