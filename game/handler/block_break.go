package handler

import (
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// handlePlayerAction handles ServerboundPlayerAction.
// In creative mode, action=0 causes instant break.
// In survival mode, action=0 starts digging, action=1 cancels, action=2 finishes.
// Actions 3/4 are Q-key drops (all game modes).
func (h *BlockHandler) handlePlayerAction(player *game.Player, p pk.Packet) {
	var action pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&action, &pos, &face, &sequence); err != nil {
		return
	}

	// Q-key drops and offhand swap (all game modes)
	switch action {
	case 3: // drop item
		h.dropFromHotbar(player, false)
		return
	case 4: // drop item stack (ctrl+Q)
		h.dropFromHotbar(player, true)
		return
	case 6: // swap item with offhand
		main := int(player.HeldSlot) + 36
		player.Inventory[main], player.Inventory[45] = player.Inventory[45], player.Inventory[main]
		SendSlotUpdate(player, main)
		SendSlotUpdate(player, 45)
		BroadcastEquipment(h.Manager, player)
		return
	}

	// Note block: left-click plays the note (all game modes).
	if action == 0 && h.NoteBlockMgr != nil {
		h.NoteBlockMgr.PlayNote(pos.X, pos.Y, pos.Z)
	}

	if player.GameMode == 1 { // creative — instant break
		if action == 0 {
			h.breakBlock(player, pos.X, pos.Y, pos.Z, int32(sequence))
		}
		return
	}

	// Spectator cannot break blocks
	if player.GameMode == 3 {
		return
	}

	// Survival / adventure mode
	switch action {
	case 0: // started_digging
		CancelEating(player)
		player.Blocking = false
		player.DrawingBow = false
		player.Digging = true
		player.DigX, player.DigY, player.DigZ = pos.X, pos.Y, pos.Z
		player.DigStartTime = time.Now()
		// Broadcast break progress stage 0
		h.broadcastBlockDestruction(player.EID, pos.X, pos.Y, pos.Z, 0)
	case 1: // cancelled_digging
		player.Digging = false
		// Reset break progress (stage 10+)
		h.broadcastBlockDestruction(player.EID, player.DigX, player.DigY, player.DigZ, 10)
	case 2: // finished_digging
		if player.Digging && player.DigX == pos.X && player.DigY == pos.Y && player.DigZ == pos.Z {
			if h.validateBreakTime(player, pos.X, pos.Y, pos.Z) {
				h.breakBlock(player, pos.X, pos.Y, pos.Z, int32(sequence))
			} else {
				// Reject: send ack only, don't break
				h.sendAck(player, int32(sequence))
			}
		}
		player.Digging = false
	}
}

// dropFromHotbar drops items from the player's currently held hotbar slot.
// If dropAll is true, drops the entire stack; otherwise drops one item.
func (h *BlockHandler) dropFromHotbar(player *game.Player, dropAll bool) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.ID == 0 || invItem.Count <= 0 {
		return
	}

	dropCount := int32(1)
	if dropAll {
		dropCount = invItem.Count
	}

	itemID := invItem.ID

	if dropAll {
		*invItem = game.ItemStack{}
	} else {
		invItem.Count--
		if invItem.Count <= 0 {
			*invItem = game.ItemStack{}
		}
	}
	SendSlotUpdate(player, slot)

	// Spawn item entity at player position
	if h.ItemEntities != nil {
		px, py, pz := player.Position()
		h.ItemEntities.SpawnItem(h.Manager, px, py+1.3, pz, itemID, dropCount, 40)
	}
}

// breakBlock removes a block (sets to air) and broadcasts the change.
func (h *BlockHandler) breakBlock(player *game.Player, x, y, z int, sequence int32) {
	// Adventure mode: only allow breaking if held item's CanDestroy list matches.
	if player.GameMode == 2 {
		stateID, err := h.World.GetBlock(x, y, z)
		if err != nil {
			h.sendAck(player, sequence)
			return
		}
		blockName := BlockNameFromState(int(stateID))
		if !CanBreakBlock(player, blockName) {
			// Revert client prediction by re-sending the existing block state.
			h.sendBlockUpdate(player, x, y, z, int32(stateID))
			h.sendAck(player, sequence)
			return
		}
	}

	oldState, err := h.World.SetBlock(x, y, z, 0) // 0 = air
	if err != nil {
		h.logf("Error breaking block at (%d,%d,%d): %v", x, y, z, err)
		return
	}

	h.broadcastBlockUpdate(x, y, z, 0)
	h.sendAck(player, sequence)

	// Block break particles + sound
	if oldState > 0 {
		BroadcastLevelEvent(h.Manager, 2001, x, y, z, int32(oldState))
	}

	// Drop item in survival mode
	if player.GameMode == 0 && oldState > 0 {
		h.dropBlockItem(player, int(oldState), x, y, z)
		// Decrement tool durability
		h.decrementToolDurability(player)
		// Award ore XP (suppressed by Silk Touch)
		blockName := BlockNameFromState(int(oldState))
		heldSlot := &player.Inventory[player.HeldSlot+36]
		if xp := GetOreXP(blockName); xp > 0 && !enchant.HasEnchant(heldSlot.Enchantments, enchant.SilkTouch) {
			if h.XPOrbMgr != nil {
				h.XPOrbMgr.SpawnXPOrbs(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, xp)
			} else {
				AddExperience(player, xp)
			}
		}
		// Mining exhaustion
		player.Exhaustion += 0.005
	}

	// If breaking a bed, also remove the other half
	h.breakBedOtherHalf(int(oldState), x, y, z)

	// If breaking a nether_portal or obsidian, remove connected portal blocks
	if oldState > 0 {
		oldBlockName := BlockNameFromState(int(oldState))
		if oldBlockName == "nether_portal" {
			// The portal block itself was broken; flood-fill remove all connected portals
			removeConnectedPortals(h.World, h.Manager, x, y, z)
		} else if oldBlockName == "obsidian" {
			// Frame obsidian was broken; check adjacent blocks for portal blocks
			removeAdjacentPortals(h.World, h.Manager, x, y, z)
		}
	}

	// If breaking a fire block, untrack it from the fire manager
	if h.FireMgr != nil && oldState > 0 {
		oldBlockName := BlockNameFromState(int(oldState))
		if oldBlockName == "fire" || oldBlockName == "soul_fire" {
			h.FireMgr.ExtinguishFire(x, y, z)
		}
	}

	// Check for falling blocks above
	if h.FallingMgr != nil {
		h.FallingMgr.CheckAndSpawnFalling(x, y+1, z)
	}

	// Check if fluid is above the broken block — schedule downward flow
	if h.FluidMgr != nil {
		h.FluidMgr.CheckFlowDown(x, y+1, z)
	}

	// Unregister sapling if broken
	if h.TreeMgr != nil {
		blockName := BlockNameFromState(int(oldState))
		if isSapling(blockName) {
			h.TreeMgr.UnregisterSapling(x, y, z)
		}
	}

	// Unregister crop if broken
	if h.CropMgr != nil {
		blockName := BlockNameFromState(int(oldState))
		if isCropBlock(blockName) {
			switch blockName {
			case "pumpkin_stem", "melon_stem", "attached_pumpkin_stem", "attached_melon_stem":
				h.CropMgr.UnregisterStem(x, y, z)
			case "sugar_cane":
				h.CropMgr.UnregisterSugarCane(x, y, z)
			case "bamboo", "bamboo_sapling":
				h.CropMgr.UnregisterBamboo(x, y, z)
			case "nether_wart":
				h.CropMgr.UnregisterNetherWart(x, y, z)
			case "cocoa":
				h.CropMgr.UnregisterCocoa(x, y, z)
			case "sweet_berry_bush":
				h.CropMgr.UnregisterBerry(x, y, z)
			default:
				h.CropMgr.UnregisterCrop(x, y, z)
			}
		}
	}

	// Notify persistence layer (for chest/furnace deletion)
	if h.OnBlockBreak != nil {
		blockName := BlockNameFromState(int(oldState))
		h.OnBlockBreak(blockName, x, y, z)
	}

	// Unlink double chests if breaking a chest
	if h.Chests != nil {
		blockName := BlockNameFromState(int(oldState))
		if blockName == "chest" {
			h.Chests.UnlinkChest(x, y, z, h.World, h.Manager)
		}
	}

	// Remove sign data if breaking a sign
	if h.SignMgr != nil {
		h.SignMgr.RemoveSign(x, y, z)
	}

	// Remove banner data if breaking a banner
	if h.BannerMgr != nil {
		h.BannerMgr.RemoveBanner(x, y, z)
	}

	// Eject disc if breaking a jukebox
	if h.JukeboxMgr != nil {
		h.JukeboxMgr.OnJukeboxBreak(x, y, z)
	}

	// Drop book if breaking a lectern
	if h.LecternMgr != nil {
		h.LecternMgr.OnLecternBreak(x, y, z)
	}

	// Drop contents and clean up decorated pot data
	if h.DecoratedPotMgr != nil && oldState > 0 {
		if BlockNameFromState(int(oldState)) == "decorated_pot" {
			h.DecoratedPotMgr.BreakPot(x, y, z)
		}
	}

	// Drop campfire cooking items if breaking a campfire
	if h.CampfireMgr != nil && oldState > 0 {
		bn := BlockNameFromState(int(oldState))
		if bn == "campfire" || bn == "soul_campfire" {
			h.CampfireMgr.RemoveItems(x, y, z)
		}
	}

	// Clean up redstone power source if breaking a lever, button, or pressure plate
	if h.RedstoneMgr != nil {
		h.RedstoneMgr.CleanupSource(x, y, z)
		// Notify observers of block change
		h.RedstoneMgr.NotifyBlockChange(x, y, z)
	}

	// Untrack daylight detector if broken
	if h.RedstoneMgr != nil && oldState > 0 {
		oldBlockName := BlockNameFromState(int(oldState))
		if oldBlockName == "daylight_detector" {
			h.RedstoneMgr.UntrackDaylightDetector(x, y, z)
		}
	}

	// Untrack beacon if broken
	if h.BeaconMgr != nil && oldState > 0 {
		oldBlockName := BlockNameFromState(int(oldState))
		if oldBlockName == "beacon" {
			h.BeaconMgr.UntrackBeacon(x, y, z)
		}
	}

	// Notify neighbors for support-dependent blocks, multi-block structures, connections
	if h.BlockUpdateMgr != nil {
		h.BlockUpdateMgr.NotifyNeighbors(x, y, z)
	}

	h.logf("Player %s broke block at (%d, %d, %d)", player.Name, x, y, z)
}

// dropBlockItem spawns a dropped item entity for the broken block, or adds directly
// to inventory if no ItemEntityManager is available.
func (h *BlockHandler) dropBlockItem(player *game.Player, stateID int, x, y, z int) {
	blockName := BlockNameFromState(stateID)
	if blockName == "" {
		return
	}

	// Read held item enchantments
	heldSlot := &player.Inventory[player.HeldSlot+36]
	hasSilkTouch := enchant.HasEnchant(heldSlot.Enchantments, enchant.SilkTouch)
	fortuneLevel := enchant.GetLevel(heldSlot.Enchantments, enchant.Fortune)

	// Silk Touch: drop the block itself instead of the processed item
	if hasSilkTouch {
		if id := itemIDByName(blockName); id > 0 {
			if h.ItemEntities != nil {
				h.ItemEntities.SpawnItem(h.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, id, 1, 10)
			} else {
				slot := player.Inventory.AddItem(id, 1)
				if slot >= 0 {
					SendSlotUpdate(player, slot)
				}
			}
			return
		}
		// If the block has no matching item, fall through to normal logic
	}

	// Crop blocks have special drop logic based on age
	if isCropBlock(blockName) {
		h.dropCropItems(stateID, x, y, z)
		return
	}

	// Melon drops 3-7 melon_slices, Fortune adds up to fortuneLevel*2 (cap 9)
	if blockName == "melon" && h.ItemEntities != nil {
		if id := itemIDByName("melon_slice"); id > 0 {
			count := int32(3 + rand.Intn(5)) // 3-7
			if fortuneLevel > 0 {
				count += int32(rand.Intn(int(fortuneLevel)*2 + 1))
				if count > 9 {
					count = 9
				}
			}
			h.ItemEntities.SpawnItem(h.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, id, count, 10)
		}
		return
	}

	// Check tool requirements
	heldName := ItemNameByID(heldSlot.ID)
	if !CanHarvestBlock(blockName, heldName) {
		return // wrong tool — no drop
	}

	// Get the drop item name (may differ from block name)
	dropName, drops := GetBlockDropItemName(blockName)
	if !drops {
		return // block drops nothing
	}

	itemID := itemIDByName(dropName)
	if itemID <= 0 {
		return
	}

	// Fortune: multiply drop count for ores
	dropCount := int32(1)
	if fortuneLevel > 0 {
		dropCount = fortuneDropCount(blockName, fortuneLevel)
	}

	if h.ItemEntities != nil {
		h.ItemEntities.SpawnItem(h.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, itemID, dropCount, 10)
	} else {
		slot := player.Inventory.AddItem(itemID, dropCount)
		if slot < 0 {
			return
		}
		SendSlotUpdate(player, slot)
	}
}

// fortuneDropCount returns the number of items to drop with the given Fortune level.
func fortuneDropCount(blockName string, fortuneLevel int32) int32 {
	switch blockName {
	case "diamond_ore", "deepslate_diamond_ore",
		"emerald_ore", "deepslate_emerald_ore",
		"coal_ore", "deepslate_coal_ore",
		"nether_quartz_ore":
		// 1 + random(0, fortuneLevel)
		return 1 + int32(rand.Intn(int(fortuneLevel)+1))

	case "lapis_ore", "deepslate_lapis_ore":
		// Base 4-9, multiply by (1 + random(0, fortuneLevel))
		base := int32(4 + rand.Intn(6))
		multiplier := int32(1 + rand.Intn(int(fortuneLevel)+1))
		return base * multiplier

	case "redstone_ore", "deepslate_redstone_ore":
		// Base 4-5, add random(0, fortuneLevel) extra
		base := int32(4 + rand.Intn(2))
		return base + int32(rand.Intn(int(fortuneLevel)+1))

	case "copper_ore", "deepslate_copper_ore":
		// 2-5 raw copper, with fortune: multiply by (1 + random(0, fortuneLevel))
		base := int32(2 + rand.Intn(4))
		multiplier := int32(1 + rand.Intn(int(fortuneLevel)+1))
		return base * multiplier

	default:
		return 1
	}
}

// validateBreakTime checks whether the player has spent enough time mining a block.
// Returns true if the break is allowed. Unknown blocks always pass.
func (h *BlockHandler) validateBreakTime(player *game.Player, x, y, z int) bool {
	stateID, err := h.World.GetBlock(x, y, z)
	if err != nil {
		return true // can't look up — allow
	}
	blockName := BlockNameFromState(int(stateID))
	if blockName == "" {
		return true
	}
	heldSlot := &player.Inventory[player.HeldSlot+36]
	heldName := ItemNameByID(heldSlot.ID)
	expected := CalculateBreakTime(blockName, heldName)
	if expected <= 0 {
		return true // instant break or unknown
	}
	if expected < 0 {
		return false // unbreakable
	}
	// Efficiency enchantment: multiply speed by (1 + level^2)
	if effLvl := enchant.GetLevel(heldSlot.Enchantments, enchant.Efficiency); effLvl > 0 {
		expected /= float64(1 + effLvl*effLvl)
	}
	// Haste: 20% faster per level
	if player.Effects != nil {
		if eff, ok := player.Effects[EffectHaste]; ok {
			expected /= 1.0 + 0.2*float64(eff.Level+1)
		}
		// Mining Fatigue: 3^level slower
		if eff, ok := player.Effects[EffectMiningFatigue]; ok {
			mult := 1.0
			for i := int32(0); i <= eff.Level; i++ {
				mult *= 3
			}
			expected *= mult
		}
	}
	elapsed := time.Since(player.DigStartTime).Seconds()
	// 20% tolerance for network latency
	return elapsed >= expected*0.8
}

// decrementToolDurability reduces the held tool's durability by 1 after breaking a block.
// Unbreaking enchantment: skip durability loss with probability level/(level+1).
// If durability reaches 0, the tool breaks (slot is cleared).
func (h *BlockHandler) decrementToolDurability(player *game.Player) {
	slot := int(player.HeldSlot) + 36
	invItem := &player.Inventory[slot]
	if invItem.MaxDurability <= 0 {
		return // not a tool
	}
	if unbreakLvl := enchant.GetLevel(invItem.Enchantments, enchant.Unbreaking); unbreakLvl > 0 {
		if rand.Int31n(unbreakLvl+1) > 0 {
			return // unbreaking saved this durability point
		}
	}
	invItem.Durability--
	if invItem.Durability <= 0 {
		*invItem = game.ItemStack{} // tool breaks
	}
	SendSlotUpdate(player, slot)
}

// breakBedOtherHalf removes the other half of a bed when one half is broken.
func (h *BlockHandler) breakBedOtherHalf(stateID int, x, y, z int) {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	bed, ok := block.StateList[stateID].(block.RedBed)
	if !ok {
		return
	}

	ox, oz := bedHeadOffset(bed.Facing)
	if bed.Part == block.BedPartHead {
		// This was the head, remove the foot (opposite direction)
		ox, oz = -ox, -oz
	}

	otherX, otherZ := x+ox, z+oz
	otherState, err := h.World.GetBlock(otherX, y, otherZ)
	if err != nil {
		return
	}
	if int(otherState) < len(block.StateList) && block.StateList[otherState] != nil {
		if _, isBed := block.StateList[otherState].(block.RedBed); isBed {
			h.World.SetBlock(otherX, y, otherZ, 0)
			h.broadcastBlockUpdate(otherX, y, otherZ, 0)
		}
	}
}

// dropCropItems spawns crop-specific drops based on the crop's age.
func (h *BlockHandler) dropCropItems(stateID int, x, y, z int) {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	if h.ItemEntities == nil {
		return
	}

	fx, fy, fz := float64(x)+0.5, float64(y)+0.5, float64(z)+0.5

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		age := int(b.Age)
		if age >= 7 {
			if id := itemIDByName("wheat"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
			}
			if id := itemIDByName("wheat_seeds"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, int32(1+rand.Intn(3)), 10)
			}
		} else {
			if id := itemIDByName("wheat_seeds"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
			}
		}
	case block.Carrots:
		age := int(b.Age)
		if id := itemIDByName("carrot"); id > 0 {
			count := int32(1)
			if age >= 7 {
				count = int32(1 + rand.Intn(4))
			}
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, count, 10)
		}
	case block.Potatoes:
		age := int(b.Age)
		if id := itemIDByName("potato"); id > 0 {
			count := int32(1)
			if age >= 7 {
				count = int32(1 + rand.Intn(4))
			}
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, count, 10)
		}
	case block.Beetroots:
		age := int(b.Age)
		if age >= 3 {
			if id := itemIDByName("beetroot"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
			}
			if id := itemIDByName("beetroot_seeds"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, int32(1+rand.Intn(3)), 10)
			}
		} else {
			if id := itemIDByName("beetroot_seeds"); id > 0 {
				h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
			}
		}
	case block.PumpkinStem, block.AttachedPumpkinStem:
		// Stems drop 0-3 seeds
		if id := itemIDByName("pumpkin_seeds"); id > 0 {
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, int32(rand.Intn(4)), 10)
		}
	case block.MelonStem, block.AttachedMelonStem:
		// Stems drop 0-3 seeds
		if id := itemIDByName("melon_seeds"); id > 0 {
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, int32(rand.Intn(4)), 10)
		}
	case block.SugarCane:
		if id := itemIDByName("sugar_cane"); id > 0 {
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
		}
	case block.NetherWart:
		age := int(b.Age)
		if id := itemIDByName("nether_wart"); id > 0 {
			count := int32(1)
			if age >= 3 {
				count = int32(2 + rand.Intn(3)) // 2-4
			}
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, count, 10)
		}
	case block.Cocoa:
		age := int(b.Age)
		if id := itemIDByName("cocoa_beans"); id > 0 {
			count := int32(1)
			if age >= 2 {
				count = 3
			}
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, count, 10)
		}
	case block.SweetBerryBush:
		age := int(b.Age)
		if id := itemIDByName("sweet_berries"); id > 0 {
			var count int32
			switch {
			case age >= 3:
				count = int32(2 + rand.Intn(2)) // 2-3
			case age == 2:
				count = int32(1 + rand.Intn(2)) // 1-2
			default:
				count = 1
			}
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, count, 10)
		}
	case block.Bamboo:
		if id := itemIDByName("bamboo"); id > 0 {
			h.ItemEntities.SpawnItem(h.Manager, fx, fy, fz, id, 1, 10)
		}
	}
}
