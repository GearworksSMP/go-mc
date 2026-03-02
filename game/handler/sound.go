package handler

import (
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Sound categories.
const (
	SoundCategoryMaster  int32 = 0
	SoundCategoryMusic   int32 = 1
	SoundCategoryRecord  int32 = 2
	SoundCategoryWeather int32 = 3
	SoundCategoryBlock   int32 = 4
	SoundCategoryHostile int32 = 5
	SoundCategoryNeutral int32 = 6
	SoundCategoryPlayer  int32 = 7
)

// Sound IDs (from data/soundid/soundid.go, zero-indexed).
const (
	SoundPlayerHurt  int32 = 799
	SoundPlayerDeath int32 = 798

	SoundExplode      int32 = 399
	SoundCreeperPrime int32 = 235
	SoundArrowShoot   int32 = 61
	SoundSkeletonShoot int32 = 932

	SoundShieldBlock   int32 = 905
	SoundThornsHit     int32 = 1021

	SoundZombieDeath   int32 = 1172
	SoundZombieHurt    int32 = 1177
	SoundSkeletonDeath int32 = 922
	SoundSkeletonHurt  int32 = 931
	SoundCreeperDeath  int32 = 233
	SoundCreeperHurt   int32 = 234
	SoundSpiderDeath   int32 = 994
	SoundSpiderHurt    int32 = 995
	SoundCowDeath      int32 = 229
	SoundCowHurt       int32 = 230
	SoundPigDeath      int32 = 763
	SoundPigHurt       int32 = 764
	SoundSheepDeath    int32 = 892
	SoundSheepHurt     int32 = 893
	SoundChickenDeath  int32 = 197
	SoundChickenHurt   int32 = 199

	// Enderman sounds
	SoundEndermanAmbient  int32 = 329
	SoundEndermanDeath    int32 = 330
	SoundEndermanHurt     int32 = 331
	SoundEndermanScream   int32 = 332
	SoundEndermanStare    int32 = 333
	SoundEndermanTeleport int32 = 334

	// Witch sounds
	SoundWitchAmbient   int32 = 1118
	SoundWitchCelebrate int32 = 1119
	SoundWitchDeath     int32 = 1120
	SoundWitchDrink     int32 = 1121
	SoundWitchHurt      int32 = 1122
	SoundWitchThrow     int32 = 1123

	// Slime sounds
	SoundSlimeAttack     int32 = 934
	SoundSlimeDeath      int32 = 935
	SoundSlimeHurt       int32 = 936
	SoundSlimeJump       int32 = 937
	SoundSlimeSquish     int32 = 938
	SoundSlimeDeathSmall int32 = 976
	SoundSlimeHurtSmall  int32 = 977
	SoundSlimeJumpSmall  int32 = 978
	SoundSlimeSquishSmall int32 = 979

	// Phantom sounds
	SoundPhantomAmbient int32 = 756
	SoundPhantomBite    int32 = 757
	SoundPhantomDeath   int32 = 758
	SoundPhantomFlap    int32 = 759
	SoundPhantomHurt    int32 = 760
	SoundPhantomSwoop   int32 = 761
)

// BroadcastSound sends a ClientboundSound packet to all players.
// soundID is the registry ID (0-indexed); the wire format sends soundID+1.
// category is a SoundCategory* constant.
// x, y, z are world coordinates.
func BroadcastSound(manager *game.PlayerManager, soundID, category int32, x, y, z float64, volume, pitch float32) {
	pkt := pk.Marshal(
		packetid.ClientboundSound,
		pk.VarInt(soundID+1),         // sound ID (+1 because 0 = custom)
		pk.VarInt(category),          // category
		pk.Int(int32(x*8)),           // fixed-point x
		pk.Int(int32(y*8)),           // fixed-point y
		pk.Int(int32(z*8)),           // fixed-point z
		pk.Float(volume),             // volume
		pk.Float(pitch),              // pitch
		pk.Long(rand.Int63()),        // seed
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// BroadcastLevelEvent sends a ClientboundLevelEvent packet to all players.
// Event 2001 = block break particles+sound, data = block state ID.
func BroadcastLevelEvent(manager *game.PlayerManager, event int32, x, y, z int, data int32) {
	pkt := pk.Marshal(
		packetid.ClientboundLevelEvent,
		pk.Int(event),
		pk.Position{X: x, Y: y, Z: z},
		pk.Int(data),
		pk.Boolean(false), // global event = false
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// MobHurtSound returns the hurt sound ID for a given mob type.
func MobHurtSound(typeID int32) int32 {
	switch typeID {
	case MobTypeZombie:
		return SoundZombieHurt
	case MobTypeSkeleton:
		return SoundSkeletonHurt
	case MobTypeCreeper:
		return SoundCreeperHurt
	case MobTypeSpider:
		return SoundSpiderHurt
	case MobTypeCow:
		return SoundCowHurt
	case MobTypePig:
		return SoundPigHurt
	case MobTypeSheep:
		return SoundSheepHurt
	case MobTypeChicken:
		return SoundChickenHurt
	case MobTypeVillager:
		return SoundVillagerHurt
	case MobTypeEnderman:
		return SoundEndermanHurt
	case MobTypeWitch:
		return SoundWitchHurt
	case MobTypeSlime:
		return SoundSlimeHurt
	case MobTypePhantom:
		return SoundPhantomHurt
	}
	return SoundPlayerHurt
}

// MobDeathSound returns the death sound ID for a given mob type.
func MobDeathSound(typeID int32) int32 {
	switch typeID {
	case MobTypeZombie:
		return SoundZombieDeath
	case MobTypeSkeleton:
		return SoundSkeletonDeath
	case MobTypeCreeper:
		return SoundCreeperDeath
	case MobTypeSpider:
		return SoundSpiderDeath
	case MobTypeCow:
		return SoundCowDeath
	case MobTypePig:
		return SoundPigDeath
	case MobTypeSheep:
		return SoundSheepDeath
	case MobTypeChicken:
		return SoundChickenDeath
	case MobTypeVillager:
		return SoundVillagerDeath
	case MobTypeEnderman:
		return SoundEndermanDeath
	case MobTypeWitch:
		return SoundWitchDeath
	case MobTypeSlime:
		return SoundSlimeDeath
	case MobTypePhantom:
		return SoundPhantomDeath
	}
	return SoundPlayerDeath
}

// MobSoundCategory returns the appropriate sound category for a mob type.
func MobSoundCategory(typeID int32) int32 {
	switch typeID {
	case MobTypeCow, MobTypePig, MobTypeSheep, MobTypeChicken, MobTypeVillager:
		return SoundCategoryNeutral
	case MobTypeEnderman:
		// Enderman is neutral until provoked, but uses hostile sound category
		return SoundCategoryHostile
	default:
		return SoundCategoryHostile
	}
}
