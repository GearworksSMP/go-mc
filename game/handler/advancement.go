package handler

import (
	"bytes"
	"io"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// AdvancementID is a namespaced advancement identifier.
type AdvancementID string

// Advancement categories.
const (
	// Story (root)
	AdvStory      AdvancementID = "minecraft:story/root"
	AdvMineStone  AdvancementID = "minecraft:story/mine_stone"
	AdvUpgradeTools AdvancementID = "minecraft:story/upgrade_tools"
	AdvSmeltIron  AdvancementID = "minecraft:story/smelt_iron"
	AdvObtainArmor AdvancementID = "minecraft:story/obtain_armor"
	AdvLavaBucket AdvancementID = "minecraft:story/lava_bucket"
	AdvIronTools  AdvancementID = "minecraft:story/iron_tools"
	AdvDeflect    AdvancementID = "minecraft:story/deflect_arrow"
	AdvFormObsidian AdvancementID = "minecraft:story/form_obsidian"
	AdvEnterNether AdvancementID = "minecraft:story/enter_the_nether"

	// Nether
	AdvNether         AdvancementID = "minecraft:nether/root"
	AdvFindFortress   AdvancementID = "minecraft:nether/find_fortress"
	AdvObtainBlaze    AdvancementID = "minecraft:nether/obtain_blaze_rod"
	AdvBrewPotion     AdvancementID = "minecraft:nether/brew_potion"
	AdvFindBastion    AdvancementID = "minecraft:nether/find_bastion"

	// End
	AdvEnd          AdvancementID = "minecraft:end/root"
	AdvEnterEnd     AdvancementID = "minecraft:end/enter_end_gateway"
	AdvKillDragon   AdvancementID = "minecraft:end/kill_dragon"
	AdvDragonEgg    AdvancementID = "minecraft:end/dragon_egg"

	// Adventure
	AdvAdventure    AdvancementID = "minecraft:adventure/root"
	AdvKillMob      AdvancementID = "minecraft:adventure/kill_a_mob"
	AdvTradePiglin  AdvancementID = "minecraft:adventure/trade_at_piglin"
	AdvKillAllMobs  AdvancementID = "minecraft:adventure/kill_all_mobs"
	AdvShootArrow   AdvancementID = "minecraft:adventure/shoot_arrow"
	AdvSleep        AdvancementID = "minecraft:adventure/sleep_in_bed"
	AdvHeroOfVillage AdvancementID = "minecraft:adventure/hero_of_the_village"

	// Husbandry
	AdvHusbandry    AdvancementID = "minecraft:husbandry/root"
	AdvBreedAnimal  AdvancementID = "minecraft:husbandry/breed_an_animal"
	AdvTameAnimal   AdvancementID = "minecraft:husbandry/tame_an_animal"
	AdvFisherfolk   AdvancementID = "minecraft:husbandry/fisherfolk"
	AdvPlantSeed    AdvancementID = "minecraft:husbandry/plant_seed"
)

// AdvancementDef defines an advancement's display and parent.
type AdvancementDef struct {
	ID       AdvancementID
	Parent   AdvancementID // empty for root
	Title    string
	Desc     string
	Icon     string // item name
	Frame    int32  // 0=task, 1=goal, 2=challenge
	BackgroundTexture string // only for root nodes
}

// advancementDefs is the list of all advancements.
var advancementDefs = []AdvancementDef{
	// Story tab
	{AdvStory, "", "Minecraft", "The heart and story of the game", "grass_block", 0, "minecraft:textures/gui/advancements/backgrounds/stone.png"},
	{AdvMineStone, AdvStory, "Stone Age", "Mine stone with your new pickaxe", "wooden_pickaxe", 0, ""},
	{AdvUpgradeTools, AdvMineStone, "Getting an Upgrade", "Construct a better pickaxe", "stone_pickaxe", 0, ""},
	{AdvSmeltIron, AdvUpgradeTools, "Acquire Hardware", "Smelt an iron ingot", "iron_ingot", 0, ""},
	{AdvObtainArmor, AdvSmeltIron, "Suit Up", "Protect yourself with a piece of iron armor", "iron_chestplate", 0, ""},
	{AdvLavaBucket, AdvSmeltIron, "Hot Stuff", "Fill a bucket with lava", "lava_bucket", 0, ""},
	{AdvIronTools, AdvSmeltIron, "Isn't It Iron Pick", "Upgrade your pickaxe", "iron_pickaxe", 0, ""},
	{AdvDeflect, AdvObtainArmor, "Not Today, Thank You", "Deflect a projectile with a shield", "shield", 0, ""},
	{AdvFormObsidian, AdvLavaBucket, "Ice Bucket Challenge", "Form and mine obsidian", "obsidian", 0, ""},
	{AdvEnterNether, AdvFormObsidian, "We Need to Go Deeper", "Build, light and enter a Nether Portal", "flint_and_steel", 0, ""},

	// Nether tab
	{AdvNether, "", "Nether", "Bring summer clothes", "netherrack", 0, "minecraft:textures/gui/advancements/backgrounds/nether.png"},
	{AdvFindFortress, AdvNether, "A Terrible Fortress", "Break into a Nether Fortress", "nether_bricks", 0, ""},
	{AdvObtainBlaze, AdvFindFortress, "Into Fire", "Relieve a Blaze of its rod", "blaze_rod", 0, ""},
	{AdvBrewPotion, AdvObtainBlaze, "Local Brewery", "Brew a potion", "potion", 0, ""},
	{AdvFindBastion, AdvNether, "Those Were the Days", "Enter a Bastion Remnant", "polished_blackstone_bricks", 0, ""},

	// End tab
	{AdvEnd, "", "The End", "Or the beginning?", "end_stone", 0, "minecraft:textures/gui/advancements/backgrounds/end.png"},
	{AdvEnterEnd, AdvEnd, "The End?", "Enter the End Portal", "ender_eye", 0, ""},
	{AdvKillDragon, AdvEnterEnd, "Free the End", "Kill the Ender Dragon", "dragon_head", 2, ""},
	{AdvDragonEgg, AdvKillDragon, "The Next Generation", "Hold the Dragon Egg", "dragon_egg", 1, ""},

	// Adventure tab
	{AdvAdventure, "", "Adventure", "Adventure, exploration, and combat", "map", 0, "minecraft:textures/gui/advancements/backgrounds/adventure.png"},
	{AdvKillMob, AdvAdventure, "Monster Hunter", "Kill any hostile monster", "iron_sword", 0, ""},
	{AdvShootArrow, AdvKillMob, "Take Aim", "Shoot something with an arrow", "bow", 0, ""},
	{AdvSleep, AdvAdventure, "Sweet Dreams", "Sleep in a bed to change your respawn point", "red_bed", 0, ""},

	// Husbandry tab
	{AdvHusbandry, "", "Husbandry", "The world is full of friends and food", "hay_block", 0, "minecraft:textures/gui/advancements/backgrounds/husbandry.png"},
	{AdvBreedAnimal, AdvHusbandry, "The Parrots and the Bats", "Breed two animals together", "wheat", 0, ""},
	{AdvTameAnimal, AdvHusbandry, "Best Friends Forever", "Tame an animal", "lead", 0, ""},
	{AdvFisherfolk, AdvHusbandry, "Fishy Business", "Catch a fish", "fishing_rod", 0, ""},
	{AdvPlantSeed, AdvHusbandry, "A Seedy Place", "Plant a seed and watch it grow", "wheat_seeds", 0, ""},
}

// AdvancementManager tracks player advancements and sends updates.
type AdvancementManager struct {
	mu       sync.Mutex
	Manager  *game.PlayerManager

	// Per-player granted advancements.
	granted map[string]map[AdvancementID]bool // player name → set of granted advancement IDs
}

// NewAdvancementManager creates a new advancement manager.
func NewAdvancementManager(manager *game.PlayerManager) *AdvancementManager {
	return &AdvancementManager{
		Manager: manager,
		granted: make(map[string]map[AdvancementID]bool),
	}
}

// SendAdvancementsOnJoin sends the full advancement tree to a player on login.
func (am *AdvancementManager) SendAdvancementsOnJoin(player *game.Player) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.granted[player.Name] == nil {
		am.granted[player.Name] = make(map[AdvancementID]bool)
	}

	// Always grant root advancements for tab display
	for _, def := range advancementDefs {
		if def.Parent == "" {
			am.granted[player.Name][def.ID] = true
		}
	}

	am.sendFullAdvancementPacket(player)
}

// Grant grants an advancement to a player and sends the update.
func (am *AdvancementManager) Grant(player *game.Player, id AdvancementID) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.granted[player.Name] == nil {
		am.granted[player.Name] = make(map[AdvancementID]bool)
	}

	if am.granted[player.Name][id] {
		return // already granted
	}

	am.granted[player.Name][id] = true
	am.sendAdvancementUpdate(player, id)
}

// ExportGranted returns a slice of advancement IDs that the player has been granted.
func (am *AdvancementManager) ExportGranted(playerName string) []string {
	am.mu.Lock()
	defer am.mu.Unlock()
	g := am.granted[playerName]
	if len(g) == 0 {
		return nil
	}
	out := make([]string, 0, len(g))
	for id := range g {
		out = append(out, string(id))
	}
	return out
}

// ImportGranted marks the given advancements as granted for the player without
// sending toast notifications. This is used to restore persisted state on login.
func (am *AdvancementManager) ImportGranted(playerName string, advancements []string) {
	if len(advancements) == 0 {
		return
	}
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.granted[playerName] == nil {
		am.granted[playerName] = make(map[AdvancementID]bool)
	}
	for _, id := range advancements {
		am.granted[playerName][AdvancementID(id)] = true
	}
}

// HasAdvancement checks if a player has a specific advancement.
func (am *AdvancementManager) HasAdvancement(playerName string, id AdvancementID) bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.granted[playerName][id]
}

// sendFullAdvancementPacket sends the complete advancement tree.
func (am *AdvancementManager) sendFullAdvancementPacket(player *game.Player) {
	var buf bytes.Buffer

	// Reset = true (clear existing, send full tree)
	pk.Boolean(true).WriteTo(&buf)

	// Advancement mapping: VarInt count + entries
	pk.VarInt(len(advancementDefs)).WriteTo(&buf)

	for _, def := range advancementDefs {
		// Key (advancement ID)
		pk.Identifier(def.ID).WriteTo(&buf)

		// Parent (optional)
		if def.Parent != "" {
			pk.Boolean(true).WriteTo(&buf)
			pk.Identifier(def.Parent).WriteTo(&buf)
		} else {
			pk.Boolean(false).WriteTo(&buf)
		}

		// Display (optional — always present for our advancements)
		pk.Boolean(true).WriteTo(&buf)
		writeAdvancementDisplay(&buf, def)

		// Requirements: 1 requirement group with 1 criterion
		criterionName := string(def.ID) + "/done"
		pk.VarInt(1).WriteTo(&buf) // number of requirement groups
		pk.VarInt(1).WriteTo(&buf) // number of criteria in this group
		pk.String(criterionName).WriteTo(&buf)

		// SendsTelemetryEvent (after requirements)
		pk.Boolean(false).WriteTo(&buf)
	}

	// Remove identifiers: empty (must come before progress)
	pk.VarInt(0).WriteTo(&buf)

	// Progress mapping: granted advancements
	grantedList := am.granted[player.Name]
	progressCount := 0
	for range grantedList {
		progressCount++
	}

	pk.VarInt(progressCount).WriteTo(&buf)
	for id := range grantedList {
		pk.Identifier(id).WriteTo(&buf)

		criterionName := string(id) + "/done"
		pk.VarInt(1).WriteTo(&buf) // number of criteria
		pk.String(criterionName).WriteTo(&buf)
		// CriterionProgress: achieved = true, date = current
		pk.Boolean(true).WriteTo(&buf)
		pk.Long(0).WriteTo(&buf) // epoch millis (0 = unspecified)
	}

	// showAdvancements (26.1: trailing boolean)
	pk.Boolean(false).WriteTo(&buf)

	pkt := pk.Packet{
		ID:   int32(packetid.ClientboundUpdateAdvancements),
		Data: buf.Bytes(),
	}
	player.WritePacket(pkt)
}

// sendAdvancementUpdate sends an incremental advancement update (single grant).
func (am *AdvancementManager) sendAdvancementUpdate(player *game.Player, id AdvancementID) {
	// Find the definition
	var def *AdvancementDef
	for i := range advancementDefs {
		if advancementDefs[i].ID == id {
			def = &advancementDefs[i]
			break
		}
	}
	if def == nil {
		return
	}

	var buf bytes.Buffer

	// Reset = false (incremental update)
	pk.Boolean(false).WriteTo(&buf)

	// Advancement mapping: send this advancement definition
	pk.VarInt(1).WriteTo(&buf)
	pk.Identifier(def.ID).WriteTo(&buf)

	// Parent
	if def.Parent != "" {
		pk.Boolean(true).WriteTo(&buf)
		pk.Identifier(def.Parent).WriteTo(&buf)
	} else {
		pk.Boolean(false).WriteTo(&buf)
	}

	// Display
	pk.Boolean(true).WriteTo(&buf)
	writeAdvancementDisplay(&buf, *def)

	// Requirements
	criterionName := string(def.ID) + "/done"
	pk.VarInt(1).WriteTo(&buf)
	pk.VarInt(1).WriteTo(&buf)
	pk.String(criterionName).WriteTo(&buf)

	// SendsTelemetryEvent (after requirements)
	pk.Boolean(false).WriteTo(&buf)

	// Remove identifiers: empty (must come before progress)
	pk.VarInt(0).WriteTo(&buf)

	// Progress: 1 advancement granted
	pk.VarInt(1).WriteTo(&buf)
	pk.Identifier(def.ID).WriteTo(&buf)
	pk.VarInt(1).WriteTo(&buf)
	pk.String(criterionName).WriteTo(&buf)
	pk.Boolean(true).WriteTo(&buf)
	pk.Long(0).WriteTo(&buf)

	// showAdvancements (26.1: trailing boolean)
	pk.Boolean(true).WriteTo(&buf)

	pkt := pk.Packet{
		ID:   int32(packetid.ClientboundUpdateAdvancements),
		Data: buf.Bytes(),
	}
	player.WritePacket(pkt)

	// Show toast notification
	am.showAdvancementToast(player, *def)
}

// showAdvancementToast sends a toast notification for an advancement.
func (am *AdvancementManager) showAdvancementToast(player *game.Player, def AdvancementDef) {
	// Toast is handled client-side from the advancement update
	// We just need to play the advancement sound
	BroadcastSound(am.Manager, SoundAdvancementComplete, SoundCategoryMaster, player.X, player.Y, player.Z, 1.0, 1.0)
}

// writeAdvancementDisplay writes the display data for an advancement.
func writeAdvancementDisplay(w io.Writer, def AdvancementDef) {
	// Title (Chat component — NBT encoded)
	chat.Text(def.Title).WriteTo(w)

	// Description (Chat component — NBT encoded)
	chat.Text(def.Desc).WriteTo(w)

	// Icon: ItemStack (VarInt count + VarInt itemID + DataComponentPatch)
	itemID := itemIDByName(def.Icon)
	pk.VarInt(1).WriteTo(w)  // count (1 item)
	pk.VarInt(itemID).WriteTo(w)
	pk.VarInt(0).WriteTo(w)  // added components count
	pk.VarInt(0).WriteTo(w)  // removed components count

	// Frame type (0=task, 1=goal, 2=challenge)
	pk.VarInt(def.Frame).WriteTo(w)

	// Flags: bit 0 = has background texture, bit 1 = show toast, bit 2 = hidden
	flags := int32(0x2) // show toast
	if def.BackgroundTexture != "" {
		flags |= 0x1 // has background
	}
	pk.Int(flags).WriteTo(w)

	// Background texture (only if bit 0 set)
	if def.BackgroundTexture != "" {
		pk.Identifier(AdvancementID(def.BackgroundTexture)).WriteTo(w)
	}

	// X, Y position (float)
	pk.Float(0).WriteTo(w)
	pk.Float(0).WriteTo(w)
}

// Sound constant for advancement.
const SoundAdvancementComplete = 1211

// CheckItemCraft checks if a crafted item triggers any advancements.
func (am *AdvancementManager) CheckItemCraft(player *game.Player, itemName string) {
	switch itemName {
	case "stone_pickaxe":
		am.Grant(player, AdvUpgradeTools)
	case "iron_pickaxe":
		am.Grant(player, AdvIronTools)
	case "iron_helmet", "iron_chestplate", "iron_leggings", "iron_boots":
		am.Grant(player, AdvObtainArmor)
	}
}

// CheckItemSmelt checks if a smelted item triggers any advancements.
func (am *AdvancementManager) CheckItemSmelt(player *game.Player, itemName string) {
	switch itemName {
	case "iron_ingot":
		am.Grant(player, AdvSmeltIron)
	}
}

// CheckBlockMine checks if mining a block triggers any advancements.
func (am *AdvancementManager) CheckBlockMine(player *game.Player, blockName string) {
	switch blockName {
	case "stone", "cobblestone", "deepslate":
		am.Grant(player, AdvMineStone)
	case "obsidian":
		am.Grant(player, AdvFormObsidian)
	}
}

// CheckMobKill checks if killing a mob triggers any advancements.
func (am *AdvancementManager) CheckMobKill(player *game.Player, mobTypeID int32) {
	am.Grant(player, AdvKillMob)
}

// CheckDimensionChange checks if entering a dimension triggers advancements.
func (am *AdvancementManager) CheckDimensionChange(player *game.Player, dimension string) {
	switch dimension {
	case "minecraft:the_nether":
		am.Grant(player, AdvEnterNether)
		am.Grant(player, AdvNether)
	case "minecraft:the_end":
		am.Grant(player, AdvEnd)
		am.Grant(player, AdvEnterEnd)
	}
}

// CheckSleep checks if sleeping triggers advancements.
func (am *AdvancementManager) CheckSleep(player *game.Player) {
	am.Grant(player, AdvSleep)
}

// CheckBreed checks if breeding triggers advancements.
func (am *AdvancementManager) CheckBreed(player *game.Player) {
	am.Grant(player, AdvBreedAnimal)
}

// CheckTame checks if taming triggers advancements.
func (am *AdvancementManager) CheckTame(player *game.Player) {
	am.Grant(player, AdvTameAnimal)
}

// CheckFish checks if catching a fish triggers advancements.
func (am *AdvancementManager) CheckFish(player *game.Player) {
	am.Grant(player, AdvFisherfolk)
}

// CheckPlantSeed checks if planting a seed triggers advancements.
func (am *AdvancementManager) CheckPlantSeed(player *game.Player) {
	am.Grant(player, AdvPlantSeed)
}

