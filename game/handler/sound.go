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

	// Wolf sounds
	SoundWolfAmbient  int32 = 1130
	SoundWolfBark     int32 = 1131
	SoundWolfDeath    int32 = 1132
	SoundWolfGrowl    int32 = 1133
	SoundWolfHurt     int32 = 1134
	SoundWolfPant     int32 = 1135
	SoundWolfWhine    int32 = 1138

	// Cat sounds
	SoundCatAmbient   int32 = 167
	SoundCatDeath     int32 = 171
	SoundCatHiss      int32 = 173
	SoundCatHurt      int32 = 174
	SoundCatPurr      int32 = 176
	SoundCatPurreow   int32 = 177

	// Horse sounds
	SoundHorseAmbient int32 = 465
	SoundHorseDeath   int32 = 470
	SoundHorseHurt    int32 = 473

	// Parrot sounds
	SoundParrotAmbient int32 = 742
	SoundParrotDeath   int32 = 743
	SoundParrotHurt    int32 = 748

	// Crossbow sounds
	SoundCrossbowHit        int32 = 238
	SoundCrossbowLoadingEnd int32 = 239
	SoundCrossbowShoot      int32 = 245

	// Trident sounds
	SoundTridentHit           int32 = 1024
	SoundTridentHitGround     int32 = 1025
	SoundTridentReturnToOwner int32 = 1026
	SoundTridentRiptide       int32 = 1027
	SoundTridentThrow         int32 = 1030
	SoundTridentThunder       int32 = 1031

	// Elytra sounds
	SoundElytraFlying int32 = 316

	// Firework sounds
	SoundFireworkLaunch int32 = 361
	SoundFireworkShoot  int32 = 362

	// Combat sounds
	SoundSweepAttack int32 = 800 // entity.player.attack.sweep

	// New mob sounds
	SoundBlazeDeath  int32 = 100
	SoundBlazeHurt   int32 = 102
	SoundBlazeShoot  int32 = 104
	SoundGhastDeath  int32 = 432
	SoundGhastHurt   int32 = 433
	SoundGhastShoot  int32 = 435
	SoundIronGolemDeath int32 = 512
	SoundIronGolemHurt  int32 = 514
	SoundGuardianDeath  int32 = 448
	SoundGuardianHurt   int32 = 451
	SoundDrownedDeath   int32 = 294
	SoundDrownedHurt    int32 = 297
	SoundHuskDeath      int32 = 479
	SoundHuskHurt       int32 = 480
	SoundStrayDeath     int32 = 1002
	SoundStrayHurt      int32 = 1003
	SoundCaveSpiderDeath  int32 = 994
	SoundCaveSpiderHurt   int32 = 995
	SoundSilverfishDeath  int32 = 917
	SoundSilverfishHurt   int32 = 918
	SoundEndermiteDeath   int32 = 335
	SoundEndermiteHurt    int32 = 336
	SoundMagmaCubeDeath   int32 = 570
	SoundMagmaCubeHurt    int32 = 571
	SoundPiglinDeath      int32 = 767
	SoundPiglinHurt       int32 = 770
	SoundWitherSkeletonDeath int32 = 1125
	SoundWitherSkeletonHurt  int32 = 1126
	SoundShulkerDeath        int32 = 901
	SoundShulkerHurt         int32 = 903
	SoundPillagerDeath       int32 = 765
	SoundPillagerHurt        int32 = 766
	SoundVindicatorDeath     int32 = 1101
	SoundVindicatorHurt      int32 = 1102
	SoundEvokerDeath         int32 = 351
	SoundEvokerHurt          int32 = 354
	SoundRavagerDeath        int32 = 816
	SoundRavagerHurt         int32 = 821
	SoundBeeDeath            int32 = 79
	SoundBeeHurt             int32 = 82
	SoundFoxDeath            int32 = 414
	SoundFoxHurt             int32 = 417
	SoundRabbitDeath         int32 = 810
	SoundRabbitHurt          int32 = 811
	SoundBatDeath            int32 = 67
	SoundBatHurt             int32 = 69
	SoundSnowGolemDeath      int32 = 982
	SoundSnowGolemHurt       int32 = 984
	SoundHoglinDeath         int32 = 459
	SoundHoglinHurt          int32 = 461
	SoundStriderDeath        int32 = 1007
	SoundStriderHurt         int32 = 1010
	SoundZombifiedPiglinDeath int32 = 1178
	SoundZombifiedPiglinHurt  int32 = 1180
	SoundVexDeath             int32 = 1094
	SoundVexHurt              int32 = 1096

	// Warden sounds
	SoundWardenAmbient        int32 = 1103
	SoundWardenDeath          int32 = 1106
	SoundWardenHurt           int32 = 1110
	SoundWardenHeartbeat      int32 = 1109
	SoundWardenRoar           int32 = 1113
	SoundWardenSonicBoom      int32 = 1116
	SoundWardenSniff          int32 = 1115

	// Note block instrument sounds
	SoundNoteBlockBasedrum     int32 = 686
	SoundNoteBlockBass         int32 = 687
	SoundNoteBlockBell         int32 = 688
	SoundNoteBlockChime        int32 = 689
	SoundNoteBlockFlute        int32 = 690
	SoundNoteBlockGuitar       int32 = 691
	SoundNoteBlockHarp         int32 = 692
	SoundNoteBlockHat          int32 = 693
	SoundNoteBlockPling        int32 = 694
	SoundNoteBlockSnare        int32 = 695
	SoundNoteBlockXylophone    int32 = 696
	SoundNoteBlockIronXylophone int32 = 697
	SoundNoteBlockCowBell      int32 = 698
	SoundNoteBlockDidgeridoo   int32 = 699
	SoundNoteBlockBit          int32 = 700
	SoundNoteBlockBanjo        int32 = 701

	// Ambient sounds
	SoundAmbientCave           int32 = 11  // ambient.cave
	SoundAmbientUnderwaterLoop int32 = 24  // ambient.underwater.loop

	// Sound category for ambient
	SoundCategoryAmbient int32 = 8

	// Chest sounds
	SoundChestOpen  int32 = 195 // block.chest.open
	SoundChestClose int32 = 193 // block.chest.close

	// Experience sounds
	SoundXPPickup int32 = 353 // entity.experience_orb.pickup
	SoundLevelUp  int32 = 804 // entity.player.levelup

	// Eating sounds
	SoundGenericEat int32 = 398 // entity.generic.eat
	SoundPlayerBurp int32 = 797 // entity.player.burp

	// Anvil sounds
	SoundAnvilUse int32 = 45 // block.anvil.use

	// Block material sounds
	SoundGrassBreak int32 = 448  // block.grass.break
	SoundGrassPlace int32 = 451  // block.grass.place
	SoundStoneBreak int32 = 1005 // block.stone.break
	SoundStonePlace int32 = 1010 // block.stone.place
	SoundWoodBreak  int32 = 1147 // block.wood.break
	SoundWoodPlace  int32 = 1152 // block.wood.place

	// Fence gate sounds
	SoundFenceGateOpen  int32 = 355 // block.fence_gate.open
	SoundFenceGateClose int32 = 354 // block.fence_gate.close

	// Barrel sounds
	SoundBarrelOpen  int32 = 63 // block.barrel.open
	SoundBarrelClose int32 = 62 // block.barrel.close

	// Shulker box sounds
	SoundShulkerBoxOpen  int32 = 907 // block.shulker_box.open
	SoundShulkerBoxClose int32 = 906 // block.shulker_box.close

	// Mob ambient sounds
	SoundCowAmbient      int32 = 228  // entity.cow.ambient
	SoundPigAmbient      int32 = 762  // entity.pig.ambient
	SoundSheepAmbient    int32 = 891  // entity.sheep.ambient
	SoundChickenAmbient  int32 = 196  // entity.chicken.ambient
	SoundZombieAmbient   int32 = 1173 // entity.zombie.ambient
	SoundSkeletonAmbient int32 = 923  // entity.skeleton.ambient
	SoundSpiderAmbient   int32 = 993  // entity.spider.ambient

	// Armor equip sounds
	SoundArmorEquipLeather   int32 = 46  // item.armor.equip_leather
	SoundArmorEquipChain     int32 = 47  // item.armor.equip_chain
	SoundArmorEquipIron      int32 = 49  // item.armor.equip_iron
	SoundArmorEquipGold      int32 = 48  // item.armor.equip_gold
	SoundArmorEquipDiamond   int32 = 50  // item.armor.equip_diamond
	SoundArmorEquipNetherite int32 = 51  // item.armor.equip_netherite
	SoundArmorEquipGeneric   int32 = 52  // item.armor.equip_generic

	// Turtle sounds
	SoundTurtleAmbient  int32 = 1045 // entity.turtle.ambient_land
	SoundTurtleDeath    int32 = 1046 // entity.turtle.death
	SoundTurtleHurt     int32 = 1051 // entity.turtle.hurt
	SoundTurtleLayEgg   int32 = 1053 // entity.turtle.lay_egg
	SoundTurtleEggHatch int32 = 1050 // entity.turtle.egg_hatch
	SoundTurtleSwim     int32 = 1056 // entity.turtle.swim

	// Fall sounds
	SoundFallBig   int32 = 397 // entity.generic.big_fall
	SoundFallSmall int32 = 980 // entity.generic.small_fall

	// Water sounds
	SoundGenericSplash int32 = 983 // entity.generic.splash
	SoundGenericSwim   int32 = 981 // entity.generic.swim

	// Step sounds (generic)
	SoundStoneStep int32 = 1008 // block.stone.step
	SoundGrassStep int32 = 450  // block.grass.step
	SoundWoodStep  int32 = 1150 // block.wood.step

	// UI sounds
	SoundUIButtonClick int32 = 1055 // ui.button.click
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
	case MobTypeWolf:
		return SoundWolfHurt
	case MobTypeCat:
		return SoundCatHurt
	case MobTypeHorse:
		return SoundHorseHurt
	case MobTypeParrot:
		return SoundParrotHurt
	case MobTypeBlaze:
		return SoundBlazeHurt
	case MobTypeGhast:
		return SoundGhastHurt
	case MobTypeIronGolem:
		return SoundIronGolemHurt
	case MobTypeSnowGolem:
		return SoundSnowGolemHurt
	case MobTypeGuardian, MobTypeElderGuardian:
		return SoundGuardianHurt
	case MobTypeDrowned:
		return SoundDrownedHurt
	case MobTypeHusk:
		return SoundHuskHurt
	case MobTypeStray:
		return SoundStrayHurt
	case MobTypeCaveSpider:
		return SoundCaveSpiderHurt
	case MobTypeSilverfish:
		return SoundSilverfishHurt
	case MobTypeEndermite:
		return SoundEndermiteHurt
	case MobTypeMagmaCube:
		return SoundMagmaCubeHurt
	case MobTypePiglin, MobTypeZombifiedPiglin:
		return SoundPiglinHurt
	case MobTypeWitherSkeleton:
		return SoundWitherSkeletonHurt
	case MobTypeShulker:
		return SoundShulkerHurt
	case MobTypePillager:
		return SoundPillagerHurt
	case MobTypeVindicator:
		return SoundVindicatorHurt
	case MobTypeEvoker:
		return SoundEvokerHurt
	case MobTypeVex:
		return SoundVexHurt
	case MobTypeRavager:
		return SoundRavagerHurt
	case MobTypeBee:
		return SoundBeeHurt
	case MobTypeFox:
		return SoundFoxHurt
	case MobTypeRabbit:
		return SoundRabbitHurt
	case MobTypeBat:
		return SoundBatHurt
	case MobTypeHoglin:
		return SoundHoglinHurt
	case MobTypeStrider:
		return SoundStriderHurt
	case MobTypeWarden:
		return SoundWardenHurt
	case MobTypeTurtle:
		return SoundTurtleHurt
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
	case MobTypeWolf:
		return SoundWolfDeath
	case MobTypeCat:
		return SoundCatDeath
	case MobTypeHorse:
		return SoundHorseDeath
	case MobTypeParrot:
		return SoundParrotDeath
	case MobTypeBlaze:
		return SoundBlazeDeath
	case MobTypeGhast:
		return SoundGhastDeath
	case MobTypeIronGolem:
		return SoundIronGolemDeath
	case MobTypeSnowGolem:
		return SoundSnowGolemDeath
	case MobTypeGuardian, MobTypeElderGuardian:
		return SoundGuardianDeath
	case MobTypeDrowned:
		return SoundDrownedDeath
	case MobTypeHusk:
		return SoundHuskDeath
	case MobTypeStray:
		return SoundStrayDeath
	case MobTypeCaveSpider:
		return SoundCaveSpiderDeath
	case MobTypeSilverfish:
		return SoundSilverfishDeath
	case MobTypeEndermite:
		return SoundEndermiteDeath
	case MobTypeMagmaCube:
		return SoundMagmaCubeDeath
	case MobTypePiglin, MobTypeZombifiedPiglin:
		return SoundPiglinDeath
	case MobTypeWitherSkeleton:
		return SoundWitherSkeletonDeath
	case MobTypeShulker:
		return SoundShulkerDeath
	case MobTypePillager:
		return SoundPillagerDeath
	case MobTypeVindicator:
		return SoundVindicatorDeath
	case MobTypeEvoker:
		return SoundEvokerDeath
	case MobTypeVex:
		return SoundVexDeath
	case MobTypeRavager:
		return SoundRavagerDeath
	case MobTypeBee:
		return SoundBeeDeath
	case MobTypeFox:
		return SoundFoxDeath
	case MobTypeRabbit:
		return SoundRabbitDeath
	case MobTypeBat:
		return SoundBatDeath
	case MobTypeHoglin:
		return SoundHoglinDeath
	case MobTypeStrider:
		return SoundStriderDeath
	case MobTypeWarden:
		return SoundWardenDeath
	case MobTypeTurtle:
		return SoundTurtleDeath
	}
	return SoundPlayerDeath
}

// MobSoundCategory returns the appropriate sound category for a mob type.
func MobSoundCategory(typeID int32) int32 {
	switch typeID {
	case MobTypeCow, MobTypePig, MobTypeSheep, MobTypeChicken, MobTypeVillager:
		return SoundCategoryNeutral
	case MobTypeWolf, MobTypeCat, MobTypeHorse, MobTypeParrot,
		MobTypeFox, MobTypeRabbit, MobTypeBat, MobTypeBee,
		MobTypeIronGolem, MobTypeSnowGolem, MobTypeStrider,
		MobTypeTurtle:
		return SoundCategoryNeutral
	case MobTypeEnderman:
		// Enderman is neutral until provoked, but uses hostile sound category
		return SoundCategoryHostile
	default:
		return SoundCategoryHostile
	}
}

// MobAmbientSound returns the ambient sound ID for a mob type, or -1 if the mob has none.
func MobAmbientSound(typeID int32) int32 {
	switch typeID {
	case MobTypeCow:
		return SoundCowAmbient
	case MobTypePig:
		return SoundPigAmbient
	case MobTypeSheep:
		return SoundSheepAmbient
	case MobTypeChicken:
		return SoundChickenAmbient
	case MobTypeZombie:
		return SoundZombieAmbient
	case MobTypeSkeleton:
		return SoundSkeletonAmbient
	case MobTypeSpider:
		return SoundSpiderAmbient
	case MobTypeEnderman:
		return SoundEndermanAmbient
	case MobTypeVillager:
		return SoundVillagerAmbient
	case MobTypeWitch:
		return SoundWitchAmbient
	case MobTypePhantom:
		return SoundPhantomAmbient
	case MobTypeWolf:
		return SoundWolfAmbient
	case MobTypeCat:
		return SoundCatAmbient
	case MobTypeHorse:
		return SoundHorseAmbient
	case MobTypeParrot:
		return SoundParrotAmbient
	case MobTypeTurtle:
		return SoundTurtleAmbient
	}
	return -1
}

// ArmorEquipSound returns the equip sound ID for an armor material.
func ArmorEquipSound(material string) int32 {
	switch material {
	case "leather":
		return SoundArmorEquipLeather
	case "chainmail":
		return SoundArmorEquipChain
	case "iron":
		return SoundArmorEquipIron
	case "gold":
		return SoundArmorEquipGold
	case "diamond":
		return SoundArmorEquipDiamond
	case "netherite":
		return SoundArmorEquipNetherite
	default:
		return SoundArmorEquipGeneric
	}
}

// BlockPlaceSound returns the place sound ID for a block based on its material category.
func BlockPlaceSound(blockName string) int32 {
	return blockMaterialSound(blockName, SoundGrassPlace, SoundStonePlace, SoundWoodPlace)
}

// BlockBreakSound returns the break sound ID for a block based on its material category.
func BlockBreakSound(blockName string) int32 {
	return blockMaterialSound(blockName, SoundGrassBreak, SoundStoneBreak, SoundWoodBreak)
}

// BlockStepSound returns the step sound ID for a block based on its material category.
func BlockStepSound(blockName string) int32 {
	return blockMaterialSound(blockName, SoundGrassStep, SoundStoneStep, SoundWoodStep)
}

// blockMaterialSound classifies a block name and returns the appropriate sound.
func blockMaterialSound(blockName string, grass, stone, wood int32) int32 {
	// Wood-based blocks
	for _, w := range []string{"planks", "log", "wood", "fence", "door", "trapdoor", "stairs",
		"slab", "button", "sign", "pressure_plate", "boat", "chest", "barrel",
		"crafting_table", "bookshelf", "ladder", "jukebox", "note_block",
		"composter", "beehive", "bee_nest", "campfire"} {
		if contains(blockName, w) {
			return wood
		}
	}
	for _, w := range []string{"oak", "spruce", "birch", "jungle", "acacia", "cherry",
		"dark_oak", "mangrove", "bamboo", "crimson", "warped"} {
		if len(blockName) >= len(w) && blockName[:len(w)] == w {
			return wood
		}
	}

	// Grass/organic blocks
	for _, g := range []string{"grass", "dirt", "farmland", "podzol", "mycelium",
		"sand", "gravel", "soul_sand", "soul_soil", "clay",
		"moss", "mud", "snow", "powder_snow", "hay", "sponge",
		"leaves", "vine", "lily_pad", "flower", "fern",
		"azalea", "moss_carpet", "sculk"} {
		if contains(blockName, g) {
			return grass
		}
	}

	// Everything else: stone/metal
	return stone
}

// contains checks if s contains substr (simple substring match).
func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
