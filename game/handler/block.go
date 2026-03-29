package handler

import (
	"log"
	"math/rand"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler/enchant"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// faceOffsets maps block face index to the XYZ offset for block placement.
var faceOffsets = [6][3]int{
	{0, -1, 0}, // 0: Bottom (-Y)
	{0, 1, 0},  // 1: Top (+Y)
	{0, 0, -1}, // 2: North (-Z)
	{0, 0, 1},  // 3: South (+Z)
	{-1, 0, 0}, // 4: West (-X)
	{1, 0, 0},  // 5: East (+X)
}

// BlockHandler processes block break/place packets and creative inventory.
type BlockHandler struct {
	World        game.World
	Manager      *game.PlayerManager
	Logger       *log.Logger
	ItemEntities *ItemEntityManager
	Chests       *ChestManager
	Furnaces     *FurnaceManager
	TimeMgr      *TimeManager
	FluidMgr     *FluidManager
	FallingMgr   *FallingBlockManager
	TreeMgr      *TreeGrowthManager
	CropMgr      *CropManager
	EnchantMgr   *EnchantManager
	AnvilMgr     *AnvilManager
	BrewingMgr   *BrewingStandManager
	BedMgr       *BedManager
	SignMgr      *SignManager
	BoatMgr      *BoatManager
	MinecartMgr  *MinecartManager
	TNTMgr       *TNTManager
	FireMgr      *FireManager
	RedstoneMgr  *RedstoneManager
	WireMgr      *WireManager
	PistonMgr    *PistonManager
	HopperMgr    *HopperManager
	DispenserMgr *DispenserManager
	DimensionMgr    *DimensionManager                   // optional; when set, uses dimension-aware world
	EndPortalMgr    *EndPortalManager                   // optional; handles end portal frame interaction
	BarrelMgr       *BarrelManager                      // optional; handles barrel interactions
	GrindstoneMgr   *GrindstoneManager                  // optional; handles grindstone interactions
	StonecutterMgr  *StonecutterManager                 // optional; handles stonecutter interactions
	SmokerMgr       *SmokerManager                      // optional; handles smoker interactions
	BlastFurnaceMgr *BlastFurnaceManager                // optional; handles blast furnace interactions
	ShulkerBoxMgr   *ShulkerBoxManager                  // optional; handles shulker box interactions
	SmithingMgr     *SmithingTableManager               // optional; handles smithing table interactions
	XPOrbMgr         *XPOrbManager                       // optional; spawns XP orbs for ore mining
	ComposterMgr     *ComposterManager                   // optional; handles composter interactions
	CauldronMgr      *CauldronManager                    // optional; handles cauldron interactions
	BeaconMgr        *BeaconManager                      // optional; handles beacon effects
	WitherMgr        *WitherManager                      // optional; handles wither summoning
	ArmorStandMgr    *ArmorStandManager                  // optional; handles armor stand placement
	ItemFrameMgr     *ItemFrameManager                   // optional; handles item frame placement
	PaintingMgr      *PaintingManager                    // optional; handles painting placement
	LeashMgr         *LeashManager                       // optional; handles leash tie to fence
	JukeboxMgr       *JukeboxManager                     // optional; handles jukebox interactions
	LecternMgr       *LecternManager                     // optional; handles lectern interactions
	BannerMgr        *BannerManager                      // optional; handles banner patterns
	RespawnAnchorMgr *RespawnAnchorManager               // optional; handles respawn anchor interactions
	CopperMgr        *CopperManager                      // optional; handles copper waxing/scraping
	HiveMgr          *HiveManager                        // optional; handles beehive/bee_nest interactions
	DecoratedPotMgr  *DecoratedPotManager                // optional; handles decorated pot interactions
	CampfireMgr      *CampfireManager                   // optional; handles campfire cooking
	NoteBlockMgr     *NoteBlockManager                  // optional; handles note block tuning/playback
	LoomMgr          *LoomManager                      // optional; handles loom interactions
	CrafterMgr       *CrafterManager                   // optional; handles crafter block interactions
	OnBlockBreak     func(blockName string, x, y, z int) // called when a block is broken
}

// worldForPlayer returns the world for the player's current dimension.
func (h *BlockHandler) worldForPlayer(player *game.Player) game.World {
	if h.DimensionMgr != nil {
		return h.DimensionMgr.WorldForPlayer(player)
	}
	return h.World
}

// HandlePacket processes a single packet for the given player.
// Returns true if the packet was handled.
func (h *BlockHandler) HandlePacket(player *game.Player, p pk.Packet) bool {
	// Resolve the correct world for the player's current dimension.
	// This is safe because HandlePacket is called from a per-player packet loop.
	if h.DimensionMgr != nil {
		h.World = h.DimensionMgr.WorldForPlayer(player)
	}

	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundPlayerAction:
		h.handlePlayerAction(player, p)
		return true

	case packetid.ServerboundUseItemOn:
		h.handleUseItemOn(player, p)
		return true

	case packetid.ServerboundSetCreativeModeSlot:
		if player.GameMode == 1 { // only in creative
			h.handleCreativeSlot(player, p)
		}
		return true

	case packetid.ServerboundSetCarriedItem:
		h.handleSetCarriedItem(player, p)
		return true

	case packetid.ServerboundSignUpdate:
		if h.SignMgr != nil {
			h.SignMgr.HandleSignUpdate(player, p)
		}
		return true
	}

	return false
}

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

// handleUseItemOn handles ServerboundUseItemOn (block placement).
func (h *BlockHandler) handleUseItemOn(player *game.Player, p pk.Packet) {
	var hand pk.VarInt
	var pos pk.Position
	var face pk.VarInt
	var cursorX, cursorY, cursorZ pk.Float
	var insideBlock pk.Boolean
	var sequence pk.VarInt
	if err := p.Scan(&hand, &pos, &face, &cursorX, &cursorY, &cursorZ, &insideBlock, &sequence); err != nil {
		return
	}

	_ = hand

	if int(face) < 0 || int(face) >= len(faceOffsets) {
		return
	}

	// Check if the clicked block is interactive (and player isn't sneaking)
	if !player.Sneaking {
		stateID, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
		if err == nil {
			blockName := BlockNameFromState(int(stateID))
			switch blockName {
			case "crafting_table":
				h.openCraftingTable(player)
				h.sendAck(player, int32(sequence))
				return
			case "chest":
				if h.Chests != nil {
					h.Chests.OpenChest(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "furnace":
				if h.Furnaces != nil {
					h.Furnaces.OpenFurnace(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "ender_chest":
				OpenEnderChest(player)
				h.sendAck(player, int32(sequence))
				return
			case "enchanting_table":
				if h.EnchantMgr != nil {
					h.EnchantMgr.OpenEnchantingTable(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "anvil", "chipped_anvil", "damaged_anvil":
				if h.AnvilMgr != nil {
					h.AnvilMgr.OpenAnvil(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "brewing_stand":
				if h.BrewingMgr != nil {
					h.BrewingMgr.OpenBrewingStand(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "hopper":
				if h.HopperMgr != nil {
					h.HopperMgr.OpenHopper(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "dispenser":
				if h.DispenserMgr != nil {
					h.DispenserMgr.OpenDispenser(player, pos.X, pos.Y, pos.Z, false)
					h.sendAck(player, int32(sequence))
					return
				}
			case "dropper":
				if h.DispenserMgr != nil {
					h.DispenserMgr.OpenDispenser(player, pos.X, pos.Y, pos.Z, true)
					h.sendAck(player, int32(sequence))
					return
				}
			case "crafter":
				if h.CrafterMgr != nil {
					h.CrafterMgr.OpenCrafter(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "barrel":
				if h.BarrelMgr != nil {
					h.BarrelMgr.OpenBarrel(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "grindstone":
				if h.GrindstoneMgr != nil {
					h.GrindstoneMgr.OpenGrindstone(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "stonecutter":
				if h.StonecutterMgr != nil {
					h.StonecutterMgr.OpenStonecutter(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "smoker":
				if h.SmokerMgr != nil {
					h.SmokerMgr.OpenSmoker(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "blast_furnace":
				if h.BlastFurnaceMgr != nil {
					h.BlastFurnaceMgr.OpenBlastFurnace(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "smithing_table":
				if h.SmithingMgr != nil {
					h.SmithingMgr.OpenSmithingTable(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "decorated_pot":
				if h.DecoratedPotMgr != nil {
					h.DecoratedPotMgr.UsePot(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
			case "loom":
				if h.LoomMgr != nil {
					h.LoomMgr.OpenLoom(player)
					h.sendAck(player, int32(sequence))
					return
				}
			case "end_portal_frame":
				if h.EndPortalMgr != nil {
					if h.EndPortalMgr.HandleFrameClick(player, pos.X, pos.Y, pos.Z, int(stateID)) {
						h.sendAck(player, int32(sequence))
						return
					}
				}
			case "respawn_anchor":
				if h.RespawnAnchorMgr != nil {
					if anchor, ok := block.StateList[int(stateID)].(block.RespawnAnchor); ok {
						if h.RespawnAnchorMgr.HandleInteraction(player, pos.X, pos.Y, pos.Z, anchor) {
							h.sendAck(player, int32(sequence))
							return
						}
					}
				}
			default:
				if h.ShulkerBoxMgr != nil && IsShulkerBox(blockName) {
					h.ShulkerBoxMgr.OpenShulkerBox(player, pos.X, pos.Y, pos.Z)
					h.sendAck(player, int32(sequence))
					return
				}
				if h.handleBlockInteraction(player, pos.X, pos.Y, pos.Z, int(stateID)) {
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}
	}

	// Check for leash tie to fence post (player holding leashed mobs + right-clicking fence)
	if h.LeashMgr != nil && h.LeashMgr.PlayerHasLeashes(player.UUID) {
		clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
		if err == nil {
			clickedName := BlockNameFromState(int(clickedState))
			if IsFenceBlock(clickedName) {
				h.LeashMgr.TieToFencePost(player, pos.X, pos.Y, pos.Z)
				h.sendAck(player, int32(sequence))
				return
			}
		}
	}

	// Copper waxing (honeycomb) and scraping (axe) interactions
	if h.CopperMgr != nil {
		heldID := player.HeldItemID()
		if heldID > 0 {
			held := ItemNameByID(heldID)
			if held == "honeycomb" {
				if h.CopperMgr.WaxCopper(player, pos.X, pos.Y, pos.Z) {
					if player.GameMode == 0 {
						slot := int(player.HeldSlot) + 36
						player.Inventory[slot].Count--
						if player.Inventory[slot].Count <= 0 {
							player.Inventory[slot] = game.ItemStack{}
						}
						SendSlotUpdate(player, slot)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			} else if IsAxeItem(held) {
				if h.CopperMgr.ScrapeCopper(player, pos.X, pos.Y, pos.Z) {
					if player.GameMode == 0 {
						h.decrementToolDurability(player)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}
	}

	offset := faceOffsets[face]
	placeX := pos.X + offset[0]
	placeY := pos.Y + offset[1]
	placeZ := pos.Z + offset[2]

	// Adventure mode: only allow placement if held item's CanPlaceOn list matches
	// the clicked (adjacent) block.
	if player.GameMode == 2 {
		adjacentState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
		if err != nil {
			h.sendAck(player, int32(sequence))
			return
		}
		adjacentName := BlockNameFromState(int(adjacentState))
		if !CanPlaceBlock(player, adjacentName) {
			h.sendAck(player, int32(sequence))
			return
		}
	}

	// Check if holding a door or bed item (needs special two-block placement)
	heldItemID := player.HeldItemID()
	if heldItemID > 0 {
		heldName := ItemNameByID(heldItemID)
		if isDoorItem(heldName) {
			h.placeDoor(player, placeX, placeY, placeZ, heldName, int32(sequence))
			return
		}
		if heldName == "red_bed" {
			h.placeBed(player, placeX, placeY, placeZ, int32(sequence))
			return
		}
		if isSignItem(heldName) && h.SignMgr != nil {
			h.placeSign(player, placeX, placeY, placeZ, heldName, int(face), int32(sequence))
			return
		}
		if isHangingSignItem(heldName) && h.SignMgr != nil {
			h.placeHangingSign(player, placeX, placeY, placeZ, heldName, int(face), int32(sequence))
			return
		}
		// Boat placement: place boat on water surface
		if woodType, ok := isBoatItem(heldName); ok && h.BoatMgr != nil {
			h.placeBoat(player, pos.X, pos.Y, pos.Z, int(face), woodType, int32(sequence))
			return
		}
		// Minecart placement: place minecart on a rail
		if isMinecartItem(heldName) && h.MinecartMgr != nil {
			h.placeMinecart(player, pos.X, pos.Y, pos.Z, int(face), int32(sequence), heldName)
			return
		}
		// Armor stand placement
		if heldName == "armor_stand" && h.ArmorStandMgr != nil {
			spawnX := float64(placeX) + 0.5
			spawnY := float64(placeY)
			spawnZ := float64(placeZ) + 0.5
			yaw := player.Yaw + 180 // face toward placer
			h.ArmorStandMgr.PlaceArmorStand(player, spawnX, spawnY, spawnZ, yaw)
			if player.GameMode == 0 {
				slot := int(player.HeldSlot) + 36
				player.Inventory[slot].Count--
				if player.Inventory[slot].Count <= 0 {
					player.Inventory[slot] = game.ItemStack{}
				}
				SendSlotUpdate(player, slot)
			}
			h.sendAck(player, int32(sequence))
			return
		}
		// Painting placement (on wall face)
		if heldName == "painting" && h.PaintingMgr != nil {
			if h.PaintingMgr.PlacePainting(player, pos.X, pos.Y, pos.Z, int32(face)) {
				if player.GameMode == 0 {
					slot := int(player.HeldSlot) + 36
					player.Inventory[slot].Count--
					if player.Inventory[slot].Count <= 0 {
						player.Inventory[slot] = game.ItemStack{}
					}
					SendSlotUpdate(player, slot)
				}
			}
			h.sendAck(player, int32(sequence))
			return
		}
		// Item frame placement (on block face)
		if (heldName == "item_frame" || heldName == "glow_item_frame") && h.ItemFrameMgr != nil {
			h.ItemFrameMgr.PlaceItemFrame(player, pos.X, pos.Y, pos.Z, int32(face), heldName == "glow_item_frame")
			if player.GameMode == 0 {
				slot := int(player.HeldSlot) + 36
				player.Inventory[slot].Count--
				if player.Inventory[slot].Count <= 0 {
					player.Inventory[slot] = game.ItemStack{}
				}
				SendSlotUpdate(player, slot)
			}
			h.sendAck(player, int32(sequence))
			return
		}
		// Rail placement: use MinecartManager for auto-curving
		if isRailItem(heldName) && h.MinecartMgr != nil {
			h.placeRail(player, placeX, placeY, placeZ, heldName, int32(sequence))
			return
		}
		// Bone meal on sapling → instant tree growth
		if heldName == "bone_meal" && h.TreeMgr != nil {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && isSapling(BlockNameFromState(int(clickedState))) {
				if h.TreeMgr.GrowTree(pos.X, pos.Y, pos.Z) {
					// Consume bone meal in survival
					if player.GameMode == 0 {
						slot := int(player.HeldSlot) + 36
						player.Inventory[slot].Count--
						if player.Inventory[slot].Count <= 0 {
							player.Inventory[slot] = game.ItemStack{}
						}
						SendSlotUpdate(player, slot)
					}
					// Bone meal particles
					BroadcastLevelEvent(h.Manager, 1505, pos.X, pos.Y, pos.Z, 0)
				}
				h.sendAck(player, int32(sequence))
				return
			}
		}

		// Bone meal on crops → advance growth
		if heldName == "bone_meal" && h.CropMgr != nil {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil {
				clickedName := BlockNameFromState(int(clickedState))
				if isCropBlock(clickedName) {
					if h.CropMgr.BoneMealCrop(pos.X, pos.Y, pos.Z) {
						if player.GameMode == 0 {
							slot := int(player.HeldSlot) + 36
							player.Inventory[slot].Count--
							if player.Inventory[slot].Count <= 0 {
								player.Inventory[slot] = game.ItemStack{}
							}
							SendSlotUpdate(player, slot)
						}
						BroadcastLevelEvent(h.Manager, 1505, pos.X, pos.Y, pos.Z, 0)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}

		// Flint and steel: TNT ignition, nether portal, or fire placement
		if heldName == "flint_and_steel" {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil {
				clickedName := BlockNameFromState(int(clickedState))

				// Flint and steel on TNT → ignite
				if clickedName == "tnt" && h.TNTMgr != nil {
					h.TNTMgr.Ignite(pos.X, pos.Y, pos.Z)
					if player.GameMode == 0 {
						h.decrementToolDurability(player)
					}
					h.sendAck(player, int32(sequence))
					return
				}

				// Flint and steel on obsidian → light nether portal
				if isObsidianState(int(clickedState)) {
					if portalBlocks, axis, ok := detectPortalFrame(h.World, pos.X, pos.Y, pos.Z); ok {
						portalBlock := block.NetherPortal{Axis: axis}
						if portalStateID, found := block.ToStateID[portalBlock]; found {
							for _, pb := range portalBlocks {
								h.World.SetBlock(pb[0], pb[1], pb[2], portalStateID)
								h.broadcastBlockUpdate(pb[0], pb[1], pb[2], int32(portalStateID))
							}
							if player.GameMode == 0 {
								h.decrementToolDurability(player)
							}
							h.logf("Player %s lit nether portal at (%d, %d, %d) axis=%s", player.Name, pos.X, pos.Y, pos.Z, axis)
						}
						h.sendAck(player, int32(sequence))
						return
					}
				}

				// Flint and steel on any other block → place fire on the clicked face
				if h.FireMgr != nil {
					off := faceOffsets[int(face)]
					fireX := pos.X + off[0]
					fireY := pos.Y + off[1]
					fireZ := pos.Z + off[2]
					h.FireMgr.PlaceFire(fireX, fireY, fireZ, 0)
					if player.GameMode == 0 {
						h.decrementToolDurability(player)
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}

		// Seed planting on farmland
		if isSeedItem(heldName) {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && int(face) == 1 { // top face only
				clickedName := BlockNameFromState(int(clickedState))
				if clickedName == "farmland" {
					if h.plantSeed(player, pos.X, pos.Y+1, pos.Z, heldName, int32(sequence)) {
						return
					}
				}
			}
		}

		// Non-farmland plant placement (sugar cane, bamboo, nether wart, sweet berries)
		if isNonFarmlandPlantItem(heldName) && int(face) == 1 {
			if h.placeNonFarmlandPlant(player, pos.X, pos.Y+1, pos.Z, heldName, int32(sequence)) {
				return
			}
		}

		// Hoe on dirt/grass_block → farmland
		if isHoeItem(heldName) {
			clickedState, err := h.World.GetBlock(pos.X, pos.Y, pos.Z)
			if err == nil && int(face) == 1 { // top face only
				clickedName := BlockNameFromState(int(clickedState))
				if clickedName == "dirt" || clickedName == "grass_block" {
					farmland := block.Farmland{Moisture: block.Integer(0)}
					if farmID, ok := block.ToStateID[farmland]; ok {
						h.World.SetBlock(pos.X, pos.Y, pos.Z, farmID)
						h.broadcastBlockUpdate(pos.X, pos.Y, pos.Z, int32(farmID))
						if player.GameMode == 0 {
							h.decrementToolDurability(player)
						}
					}
					h.sendAck(player, int32(sequence))
					return
				}
			}
		}
	}

	stateID := h.heldBlockState(player)
	if stateID < 0 {
		// Not holding a placeable block — check for bucket usage
		if heldItemID > 0 {
			heldName := ItemNameByID(heldItemID)
			if action, ok := bucketActions[heldName]; ok {
				h.handleBucket(player, placeX, placeY, placeZ, pos.X, pos.Y, pos.Z, action, heldName, int32(sequence))
				return
			}
		}
		h.sendAck(player, int32(sequence))
		return
	}

	// Check if placement would collide with any player
	if h.wouldCollideWithPlayer(placeX, placeY, placeZ) {
		h.broadcastBlockUpdate(placeX, placeY, placeZ, 0) // resync client
		h.sendAck(player, int32(sequence))
		return
	}

	orientedState := orientBlock(player, int(stateID), int(face))
	h.placeBlock(player, placeX, placeY, placeZ, level.BlocksState(orientedState), int32(sequence))
}

// openCraftingTable opens a 3x3 crafting window for the player.
func (h *BlockHandler) openCraftingTable(player *game.Player) {
	player.OpenWindowID = 1
	// Clear crafting grid
	for i := range player.CraftingGrid {
		player.CraftingGrid[i] = game.ItemStack{}
	}

	// Send ClientboundOpenScreen: windowID=1, type=12 (crafting), title
	title := chat.Text("Crafting")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(1),  // window ID
		pk.VarInt(12), // menu type: crafting (3x3)
		title,
	))

	// Send initial window content
	SendCraftingWindowContent(player)
}

// handleCreativeSlot handles ServerboundSetCreativeModeSlot.
func (h *BlockHandler) handleCreativeSlot(player *game.Player, p pk.Packet) {
	var slotNumber pk.VarInt
	var count pk.VarInt
	if err := p.Scan(&slotNumber, &count); err != nil {
		return
	}

	if count == 0 {
		player.SetCreativeSlot(int16(slotNumber), 0)
		return
	}

	// Read item ID (next VarInt after count)
	var itemID pk.VarInt
	if err := p.Scan(&slotNumber, &count, &itemID); err != nil {
		return
	}

	player.SetCreativeSlot(int16(slotNumber), int32(itemID))
}

// handleSetCarriedItem handles ServerboundSetCarriedItem (hotbar slot selection).
func (h *BlockHandler) handleSetCarriedItem(player *game.Player, p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
	CancelEating(player)
	player.Blocking = false
	player.SetHeldSlot(int16(slot))
	BroadcastEquipment(h.Manager, player)
}

// heldBlockState returns the block state ID for the item the player is holding,
// or -1 if the player isn't holding a placeable block.
func (h *BlockHandler) heldBlockState(player *game.Player) int32 {
	itemID := player.HeldItemID()
	if itemID <= 0 {
		return -1
	}

	it, ok := item.ByID[item.ID(itemID)]
	if !ok {
		return -1
	}

	b, ok := block.FromID["minecraft:"+it.Name]
	if !ok {
		return -1
	}

	sid, ok := block.ToStateID[b]
	if !ok {
		return -1
	}

	return int32(sid)
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

// blockToItemID maps a block name (e.g. "minecraft:dirt") to its item ID.
func blockToItemID(blockName string) (int32, bool) {
	// Strip "minecraft:" prefix for matching against item names
	name := blockName
	if len(name) > 10 && name[:10] == "minecraft:" {
		name = name[10:]
	}
	for id, itm := range item.ByID {
		if itm.Name == name {
			return int32(id), true
		}
	}
	return 0, false
}

// orientBlock adjusts a block's state based on player facing and clicked face.
// Returns the oriented block state ID, or the original if no orientation applies.
func orientBlock(player *game.Player, state int, face int) int {
	if state < 0 || state >= len(block.StateList) || block.StateList[state] == nil {
		return state
	}

	yaw, pitch := player.Rotation()
	facing := yawToDirection(yaw)

	b := block.StateList[state]
	var oriented block.Block

	switch v := b.(type) {
	// Stairs: face toward player, half based on click position
	case block.OakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobblestoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.StoneBrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BrickStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SpruceStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.BirchStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.JungleStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.AcaciaStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.DarkOakStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.SandstoneStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v
	case block.CobbledDeepslateStairs:
		v.Facing = facing
		if face == 0 {
			v.Half = block.Top
		} else {
			v.Half = block.Bottom
		}
		oriented = v

	// Logs: axis based on clicked face
	case block.OakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.SpruceLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.BirchLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.JungleLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.AcaciaLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.DarkOakLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.CherryLog:
		v.Axis = faceToAxis(face)
		oriented = v
	case block.MangroveLog:
		v.Axis = faceToAxis(face)
		oriented = v

	// Slabs: type based on click position
	case block.StoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobblestoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.StoneBrickSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SandstoneSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.OakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.SpruceSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.BirchSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.JungleSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.AcaciaSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.DarkOakSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v
	case block.CobbledDeepslateSlab:
		if face == 0 {
			v.Type = block.SlabTypeTop
		} else {
			v.Type = block.SlabTypeBottom
		}
		oriented = v

	// Furnace, chest facing
	case block.Furnace:
		v.Facing = facing
		oriented = v
	case block.Chest:
		v.Facing = facing
		oriented = v

	// Pumpkin/carved pumpkin
	case block.CarvedPumpkin:
		v.Facing = facing
		oriented = v

	// Glazed terracotta
	case block.WhiteGlazedTerracotta:
		v.Facing = facing
		oriented = v

	// Observer
	case block.Observer:
		v.Facing = yawPitchToFacing6(yaw, pitch)
		oriented = v

	default:
		return state
	}

	if newID, ok := block.ToStateID[oriented]; ok {
		return int(newID)
	}
	return state
}

// faceToAxis converts a clicked face index to a log axis.
func faceToAxis(face int) block.Axis {
	switch face {
	case 0, 1: // bottom, top
		return block.Y
	case 2, 3: // north, south
		return block.Z
	case 4, 5: // west, east
		return block.X
	}
	return block.Y
}

// yawPitchToFacing6 converts yaw+pitch to a 6-direction facing (includes up/down).
func yawPitchToFacing6(yaw, pitch float32) block.Direction {
	if pitch > 45 {
		return block.Down
	}
	if pitch < -45 {
		return block.Up
	}
	return yawToDirection(yaw)
}

// placeBlock places a block and broadcasts the change.
func (h *BlockHandler) placeBlock(player *game.Player, x, y, z int, state level.BlocksState, sequence int32) {
	_, err := h.World.SetBlock(x, y, z, state)
	if err != nil {
		h.logf("Error placing block at (%d,%d,%d): %v", x, y, z, err)
		h.sendAck(player, sequence)
		return
	}

	h.broadcastBlockUpdate(x, y, z, int32(state))
	h.sendAck(player, sequence)

	// Play block place sound
	placedBlockName := BlockNameFromState(int(state))
	BroadcastSound(h.Manager, BlockPlaceSound(placedBlockName), SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 0.8)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Register sapling for tree growth
	if h.TreeMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isSapling(placedName) {
			h.TreeMgr.RegisterSapling(x, y, z)
		}
	}

	// Check if placed block should fall
	if h.FallingMgr != nil {
		placedName := BlockNameFromState(int(state))
		if isFallingBlock(placedName) {
			// Check if block below is air
			belowState, err := h.World.GetBlock(x, y-1, z)
			if err == nil && belowState == 0 {
				h.FallingMgr.CheckAndSpawnFalling(x, y, z)
			}
		}
	}

	// Try linking adjacent chests as double chest
	if h.Chests != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "chest" {
			h.Chests.TryLinkDouble(x, y, z, h.World, h.Manager)
		}
	}

	// Notify observers of block change
	if h.RedstoneMgr != nil {
		h.RedstoneMgr.NotifyBlockChange(x, y, z)
	}

	// Track daylight detector placement
	if h.RedstoneMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "daylight_detector" {
			h.RedstoneMgr.TrackDaylightDetector(x, y, z)
		}
	}

	// Track beacon placement
	if h.BeaconMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "beacon" {
			h.BeaconMgr.TrackBeacon(x, y, z)
		}
	}

	// Check wither summoning when placing wither_skeleton_skull
	if h.WitherMgr != nil {
		placedName := BlockNameFromState(int(state))
		if placedName == "wither_skeleton_skull" || placedName == "wither_skeleton_wall_skull" {
			h.WitherMgr.CheckWitherSummon(x, y, z)
		}
	}

	h.logf("Player %s placed block at (%d, %d, %d) state=%d", player.Name, x, y, z, state)
}

// broadcastBlockDestruction sends ClientboundBlockDestruction to all players.
// stage 0-9 = progress, 10+ = reset.
func (h *BlockHandler) broadcastBlockDestruction(entityID int32, x, y, z int, stage int8) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockDestruction,
		pk.VarInt(entityID),
		pk.Position{X: x, Y: y, Z: z},
		pk.Byte(stage),
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// sendBlockUpdate sends a ClientboundBlockUpdate to a single player (e.g. to revert a prediction).
func (h *BlockHandler) sendBlockUpdate(player *game.Player, x, y, z int, stateID int32) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	))
}

// broadcastBlockUpdate sends ClientboundBlockUpdate to all connected players.
func (h *BlockHandler) broadcastBlockUpdate(x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	h.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// sendAck sends ClientboundBlockChangedAck to the player.
func (h *BlockHandler) sendAck(player *game.Player, sequence int32) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundBlockChangedAck,
		pk.VarInt(sequence),
	))
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

// handleBlockInteraction handles right-click interactions with doors, levers, and buttons.
// Returns true if the interaction was handled (prevents item placement).
func (h *BlockHandler) handleBlockInteraction(player *game.Player, x, y, z int, stateID int) bool {
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	b := block.StateList[stateID]

	switch door := b.(type) {
	case block.OakDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.SpruceDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.BirchDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.JungleDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.AcaciaDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.DarkOakDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.CherryDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.MangroveDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.BambooDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.CrimsonDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.WarpedDoor:
		door.Open = !door.Open
		h.toggleDoor(x, y, z, door, door.Half, door.Facing, door.Hinge, bool(door.Open), bool(door.Powered))
		return true
	case block.IronDoor:
		return true // iron doors require redstone, no hand interaction
	case block.Lever:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ToggleLever(player, x, y, z)
		} else {
			door.Powered = !door.Powered
			if newID, ok := block.ToStateID[door]; ok {
				h.World.SetBlock(x, y, z, newID)
				h.broadcastBlockUpdate(x, y, z, int32(newID))
			}
		}
		return true
	case block.StoneButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "stone_button", door.Face, door.Facing, 30)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "stone_button", 30)
		}
		return true
	case block.OakButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "oak_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "oak_button", 20)
		}
		return true
	case block.SpruceButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "spruce_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "spruce_button", 20)
		}
		return true
	case block.BirchButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "birch_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "birch_button", 20)
		}
		return true
	case block.JungleButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "jungle_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "jungle_button", 20)
		}
		return true
	case block.AcaciaButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "acacia_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "acacia_button", 20)
		}
		return true
	case block.CherryButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "cherry_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "cherry_button", 20)
		}
		return true
	case block.DarkOakButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "dark_oak_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "dark_oak_button", 20)
		}
		return true
	case block.MangroveButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "mangrove_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "mangrove_button", 20)
		}
		return true
	case block.BambooButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "bamboo_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "bamboo_button", 20)
		}
		return true
	case block.CrimsonButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "crimson_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "crimson_button", 20)
		}
		return true
	case block.WarpedButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "warped_button", door.Face, door.Facing, 20)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "warped_button", 20)
		}
		return true
	case block.PolishedBlackstoneButton:
		if h.RedstoneMgr != nil {
			h.RedstoneMgr.ActivateButton(player, x, y, z, "polished_blackstone_button", door.Face, door.Facing, 30)
		} else {
			h.pressButton(x, y, z, door.Face, door.Facing, "polished_blackstone_button", 30)
		}
		return true
	case block.RedBed:
		h.interactBed(player, x, y, z)
		return true

	// Trapdoors
	case block.OakTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.SpruceTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BirchTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.JungleTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.AcaciaTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CherryTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.DarkOakTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.MangroveTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BambooTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CrimsonTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.WarpedTrapdoor:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true

	// Fence gates
	case block.OakFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.SpruceFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BirchFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.JungleFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.AcaciaFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CherryFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.DarkOakFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.MangroveFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.BambooFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.CrimsonFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.WarpedFenceGate:
		door.Open = !door.Open
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true

	// Sweet berry bush: harvest on right-click
	case block.SweetBerryBush:
		if h.CropMgr != nil {
			age := int(door.Age)
			if age >= 2 {
				count := h.CropMgr.HarvestSweetBerries(x, y, z)
				if count > 0 && h.ItemEntities != nil {
					if id := itemIDByName("sweet_berries"); id > 0 {
						h.ItemEntities.SpawnItem(h.Manager, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, id, int32(count), 10)
					}
				}
				return true
			}
		}
		return false

	// Redstone: repeater (cycle delay), comparator (toggle mode)
	case block.Repeater:
		if h.WireMgr != nil {
			h.WireMgr.CycleRepeaterDelay(x, y, z)
		}
		return true
	case block.Comparator:
		if h.WireMgr != nil {
			h.WireMgr.ToggleComparatorMode(x, y, z)
		}
		return true
	case block.Jukebox:
		if h.JukeboxMgr != nil {
			return h.JukeboxMgr.InteractJukebox(player, x, y, z)
		}
	case block.Lectern:
		if h.LecternMgr != nil {
			return h.LecternMgr.InteractLectern(player, x, y, z)
		}
	case block.NoteBlock:
		if h.NoteBlockMgr != nil {
			h.NoteBlockMgr.TuneNote(x, y, z)
		} else if h.RedstoneMgr != nil {
			h.RedstoneMgr.CycleNoteBlock(x, y, z)
		}
		return true
	case block.Bell:
		RingBell(h.Manager, x, y, z)
		return true
	case block.Composter:
		if h.ComposterMgr != nil {
			return h.ComposterMgr.UseComposter(player, x, y, z)
		}
	case block.Cauldron:
		if h.CauldronMgr != nil {
			return h.CauldronMgr.UseCauldron(player, x, y, z)
		}
	case block.WaterCauldron:
		if h.CauldronMgr != nil {
			return h.CauldronMgr.UseCauldron(player, x, y, z)
		}
	case block.LavaCauldron:
		if h.CauldronMgr != nil {
			return h.CauldronMgr.UseCauldron(player, x, y, z)
		}
	case block.PowderSnowCauldron:
		if h.CauldronMgr != nil {
			return h.CauldronMgr.UseCauldron(player, x, y, z)
		}
	case block.DaylightDetector:
		// Toggle inverted state on right-click
		door.Inverted = !door.Inverted
		if newID, ok := block.ToStateID[door]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
		return true
	case block.Lodestone:
		return h.interactLodestone(player, x, y, z)
	case block.Beacon:
		if h.BeaconMgr != nil {
			h.BeaconMgr.OpenBeaconUI(player, x, y, z)
			return true
		}
	case block.RespawnAnchor:
		if h.RespawnAnchorMgr != nil {
			return h.RespawnAnchorMgr.HandleInteraction(player, x, y, z, b.(block.RespawnAnchor))
		}
	case block.BeeNest:
		if h.HiveMgr != nil {
			return h.HiveMgr.UseHive(player, x, y, z)
		}
	case block.Beehive:
		if h.HiveMgr != nil {
			return h.HiveMgr.UseHive(player, x, y, z)
		}
	case block.Campfire:
		if h.CampfireMgr != nil {
			return h.CampfireMgr.PlaceItem(player, x, y, z)
		}
	case block.SoulCampfire:
		if h.CampfireMgr != nil {
			return h.CampfireMgr.PlaceItem(player, x, y, z)
		}
	}

	return false
}

// interactLodestone handles right-clicking a lodestone with a compass to create a lodestone compass.
func (h *BlockHandler) interactLodestone(player *game.Player, x, y, z int) bool {
	slot := int(player.HeldSlot) + 36
	held := &player.Inventory[slot]
	if ItemNameByID(held.ID) != "compass" {
		return false
	}

	held.Lodestone = &game.LodestoneTarget{
		Dimension: "minecraft:overworld",
		X:         x,
		Y:         y,
		Z:         z,
	}

	SendSlotUpdate(player, slot)
	BroadcastSound(h.Manager, 574, SoundCategoryBlock, float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	return true
}

// toggleDoor updates both halves of a door.
func (h *BlockHandler) toggleDoor(x, y, z int, clickedBlock block.Block, half block.DoubleBlockHalf, facing block.Direction, hinge block.DoorHingeSide, open, powered bool) {
	// Update clicked half
	if newID, ok := block.ToStateID[clickedBlock]; ok {
		h.World.SetBlock(x, y, z, newID)
		h.broadcastBlockUpdate(x, y, z, int32(newID))
	}

	// Update other half
	otherY := y + 1
	if half == block.DoubleBlockHalfUpper {
		otherY = y - 1
	}
	otherHalf := block.DoubleBlockHalfUpper
	if half == block.DoubleBlockHalfUpper {
		otherHalf = block.DoubleBlockHalfLower
	}

	// Get the other half's current state and update its Open property
	otherState, err := h.World.GetBlock(x, otherY, z)
	if err != nil {
		return
	}
	if int(otherState) >= len(block.StateList) || block.StateList[otherState] == nil {
		return
	}

	// Create matching other half using the block name lookup
	otherBlock := block.StateList[otherState]
	switch d := otherBlock.(type) {
	case block.OakDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.SpruceDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.BirchDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.JungleDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.AcaciaDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.DarkOakDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.CherryDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.MangroveDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.BambooDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.CrimsonDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	case block.WarpedDoor:
		d.Open = block.Boolean(open)
		d.Half = otherHalf
		if newID, ok := block.ToStateID[d]; ok {
			h.World.SetBlock(x, otherY, z, newID)
			h.broadcastBlockUpdate(x, otherY, z, int32(newID))
		}
	}
}

// pressButton sets a button to powered and schedules it to reset.
func (h *BlockHandler) pressButton(x, y, z int, face block.AttachFace, facing block.Direction, buttonType string, _ int) {
	var pressedState block.Block
	switch buttonType {
	case "stone_button":
		pressedState = block.StoneButton{Face: face, Facing: facing, Powered: true}
	case "oak_button":
		pressedState = block.OakButton{Face: face, Facing: facing, Powered: true}
	case "spruce_button":
		pressedState = block.SpruceButton{Face: face, Facing: facing, Powered: true}
	case "birch_button":
		pressedState = block.BirchButton{Face: face, Facing: facing, Powered: true}
	case "jungle_button":
		pressedState = block.JungleButton{Face: face, Facing: facing, Powered: true}
	case "acacia_button":
		pressedState = block.AcaciaButton{Face: face, Facing: facing, Powered: true}
	case "dark_oak_button":
		pressedState = block.DarkOakButton{Face: face, Facing: facing, Powered: true}
	default:
		return
	}

	if newID, ok := block.ToStateID[pressedState]; ok {
		h.World.SetBlock(x, y, z, newID)
		h.broadcastBlockUpdate(x, y, z, int32(newID))
	}

	// Schedule reset (done via tick system — for now, reset after a goroutine delay)
	go func() {
		var resetState block.Block
		switch buttonType {
		case "stone_button":
			resetState = block.StoneButton{Face: face, Facing: facing, Powered: false}
		case "oak_button":
			resetState = block.OakButton{Face: face, Facing: facing, Powered: false}
		case "spruce_button":
			resetState = block.SpruceButton{Face: face, Facing: facing, Powered: false}
		case "birch_button":
			resetState = block.BirchButton{Face: face, Facing: facing, Powered: false}
		case "jungle_button":
			resetState = block.JungleButton{Face: face, Facing: facing, Powered: false}
		case "acacia_button":
			resetState = block.AcaciaButton{Face: face, Facing: facing, Powered: false}
		case "dark_oak_button":
			resetState = block.DarkOakButton{Face: face, Facing: facing, Powered: false}
		}
		// Wait 1.5s for stone, 1s for wood
		delay := 1500 * time.Millisecond
		if buttonType != "stone_button" {
			delay = 1000 * time.Millisecond
		}
		time.Sleep(delay)
		if newID, ok := block.ToStateID[resetState]; ok {
			h.World.SetBlock(x, y, z, newID)
			h.broadcastBlockUpdate(x, y, z, int32(newID))
		}
	}()
}

// placeDoor places both halves of a door at the given position.
func (h *BlockHandler) placeDoor(player *game.Player, x, y, z int, doorName string, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	var lowerBlock, upperBlock block.Block

	switch doorName {
	case "oak_door":
		lowerBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.OakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "spruce_door":
		lowerBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.SpruceDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "birch_door":
		lowerBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BirchDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "jungle_door":
		lowerBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.JungleDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "acacia_door":
		lowerBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.AcaciaDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "dark_oak_door":
		lowerBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.DarkOakDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "cherry_door":
		lowerBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CherryDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "mangrove_door":
		lowerBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.MangroveDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "bamboo_door":
		lowerBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.BambooDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "crimson_door":
		lowerBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.CrimsonDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "warped_door":
		lowerBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.WarpedDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	case "iron_door":
		lowerBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfLower, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
		upperBlock = block.IronDoor{Facing: facing, Half: block.DoubleBlockHalfUpper, Hinge: block.DoorHingeSideLeft, Open: false, Powered: false}
	default:
		return
	}

	lowerID, ok1 := block.ToStateID[lowerBlock]
	upperID, ok2 := block.ToStateID[upperBlock]
	if !ok1 || !ok2 {
		return
	}

	h.World.SetBlock(x, y, z, lowerID)
	h.World.SetBlock(x, y+1, z, upperID)
	h.broadcastBlockUpdate(x, y, z, int32(lowerID))
	h.broadcastBlockUpdate(x, y+1, z, int32(upperID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}
}

// placeSign handles sign placement. Standing signs are placed on top of blocks
// (face 1), wall signs on side faces (face 2-5).
func (h *BlockHandler) placeSign(player *game.Player, x, y, z int, itemName string, face int, sequence int32) {
	// Check for player collision
	if h.wouldCollideWithPlayer(x, y, z) {
		h.broadcastBlockUpdate(x, y, z, 0)
		h.sendAck(player, sequence)
		return
	}

	var signBlock block.Block

	if face >= 2 && face <= 5 {
		// Wall sign: placed on the side of a block
		dir, ok := faceToWallSignDirection(face)
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		wallName := wallSignBlockForItem(itemName)
		if wallName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = wallSignWithFacing(wallName, dir)
	} else {
		// Standing sign: placed on top of a block
		yaw, _ := player.Rotation()
		rotation := yawToSignRotation(yaw)
		standingName := signBlockForItem(itemName)
		if standingName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = standingSignWithRotation(standingName, rotation)
	}

	if signBlock == nil {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := block.ToStateID[signBlock]
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Open sign editor
	h.SignMgr.PlaceSign(player, x, y, z)

	h.logf("Player %s placed sign at (%d, %d, %d)", player.Name, x, y, z)
}

// standingSignWithRotation creates a standing sign block with the given rotation.
func standingSignWithRotation(blockName string, rotation int) block.Block {
	rot := block.Integer(rotation)
	switch blockName {
	case "minecraft:oak_sign":
		return block.OakSign{Rotation: rot}
	case "minecraft:spruce_sign":
		return block.SpruceSign{Rotation: rot}
	case "minecraft:birch_sign":
		return block.BirchSign{Rotation: rot}
	case "minecraft:jungle_sign":
		return block.JungleSign{Rotation: rot}
	case "minecraft:acacia_sign":
		return block.AcaciaSign{Rotation: rot}
	case "minecraft:cherry_sign":
		return block.CherrySign{Rotation: rot}
	case "minecraft:dark_oak_sign":
		return block.DarkOakSign{Rotation: rot}
	case "minecraft:mangrove_sign":
		return block.MangroveSign{Rotation: rot}
	case "minecraft:bamboo_sign":
		return block.BambooSign{Rotation: rot}
	case "minecraft:crimson_sign":
		return block.CrimsonSign{Rotation: rot}
	case "minecraft:warped_sign":
		return block.WarpedSign{Rotation: rot}
	}
	return nil
}

// wallSignWithFacing creates a wall sign block with the given facing direction.
func wallSignWithFacing(blockName string, facing block.Direction) block.Block {
	switch blockName {
	case "minecraft:oak_wall_sign":
		return block.OakWallSign{Facing: facing}
	case "minecraft:spruce_wall_sign":
		return block.SpruceWallSign{Facing: facing}
	case "minecraft:birch_wall_sign":
		return block.BirchWallSign{Facing: facing}
	case "minecraft:jungle_wall_sign":
		return block.JungleWallSign{Facing: facing}
	case "minecraft:acacia_wall_sign":
		return block.AcaciaWallSign{Facing: facing}
	case "minecraft:cherry_wall_sign":
		return block.CherryWallSign{Facing: facing}
	case "minecraft:dark_oak_wall_sign":
		return block.DarkOakWallSign{Facing: facing}
	case "minecraft:mangrove_wall_sign":
		return block.MangroveWallSign{Facing: facing}
	case "minecraft:bamboo_wall_sign":
		return block.BambooWallSign{Facing: facing}
	case "minecraft:crimson_wall_sign":
		return block.CrimsonWallSign{Facing: facing}
	case "minecraft:warped_wall_sign":
		return block.WarpedWallSign{Facing: facing}
	}
	return nil
}

// placeHangingSign handles hanging sign placement. Ceiling hanging signs are placed
// below a block (face 0 = bottom), wall hanging signs on side faces (face 2-5).
func (h *BlockHandler) placeHangingSign(player *game.Player, x, y, z int, itemName string, face int, sequence int32) {
	if h.wouldCollideWithPlayer(x, y, z) {
		h.broadcastBlockUpdate(x, y, z, 0)
		h.sendAck(player, sequence)
		return
	}

	var signBlock block.Block

	if face >= 2 && face <= 5 {
		// Wall hanging sign: attached to the side of a block
		dir, ok := faceToWallSignDirection(face)
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		wallName := wallHangingSignBlockForItem(itemName)
		if wallName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = wallHangingSignWithFacing(wallName, dir)
	} else {
		// Ceiling hanging sign: attached below a block, rotation from player yaw
		yaw, _ := player.Rotation()
		rotation := yawToSignRotation(yaw)
		ceilingName := hangingSignBlockForItem(itemName)
		if ceilingName == "" {
			h.sendAck(player, sequence)
			return
		}
		signBlock = ceilingHangingSignWithRotation(ceilingName, rotation)
	}

	if signBlock == nil {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := block.ToStateID[signBlock]
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}

	// Open sign editor
	h.SignMgr.PlaceSign(player, x, y, z)

	h.logf("Player %s placed hanging sign at (%d, %d, %d)", player.Name, x, y, z)
}

// ceilingHangingSignWithRotation creates a ceiling hanging sign block with the given rotation.
func ceilingHangingSignWithRotation(blockName string, rotation int) block.Block {
	rot := block.Integer(rotation)
	switch blockName {
	case "minecraft:oak_hanging_sign":
		return block.OakHangingSign{Rotation: rot}
	case "minecraft:spruce_hanging_sign":
		return block.SpruceHangingSign{Rotation: rot}
	case "minecraft:birch_hanging_sign":
		return block.BirchHangingSign{Rotation: rot}
	case "minecraft:jungle_hanging_sign":
		return block.JungleHangingSign{Rotation: rot}
	case "minecraft:acacia_hanging_sign":
		return block.AcaciaHangingSign{Rotation: rot}
	case "minecraft:cherry_hanging_sign":
		return block.CherryHangingSign{Rotation: rot}
	case "minecraft:dark_oak_hanging_sign":
		return block.DarkOakHangingSign{Rotation: rot}
	case "minecraft:mangrove_hanging_sign":
		return block.MangroveHangingSign{Rotation: rot}
	case "minecraft:bamboo_hanging_sign":
		return block.BambooHangingSign{Rotation: rot}
	case "minecraft:crimson_hanging_sign":
		return block.CrimsonHangingSign{Rotation: rot}
	case "minecraft:warped_hanging_sign":
		return block.WarpedHangingSign{Rotation: rot}
	}
	return nil
}

// wallHangingSignWithFacing creates a wall hanging sign block with the given facing direction.
func wallHangingSignWithFacing(blockName string, facing block.Direction) block.Block {
	switch blockName {
	case "minecraft:oak_wall_hanging_sign":
		return block.OakWallHangingSign{Facing: facing}
	case "minecraft:spruce_wall_hanging_sign":
		return block.SpruceWallHangingSign{Facing: facing}
	case "minecraft:birch_wall_hanging_sign":
		return block.BirchWallHangingSign{Facing: facing}
	case "minecraft:jungle_wall_hanging_sign":
		return block.JungleWallHangingSign{Facing: facing}
	case "minecraft:acacia_wall_hanging_sign":
		return block.AcaciaWallHangingSign{Facing: facing}
	case "minecraft:cherry_wall_hanging_sign":
		return block.CherryWallHangingSign{Facing: facing}
	case "minecraft:dark_oak_wall_hanging_sign":
		return block.DarkOakWallHangingSign{Facing: facing}
	case "minecraft:mangrove_wall_hanging_sign":
		return block.MangroveWallHangingSign{Facing: facing}
	case "minecraft:bamboo_wall_hanging_sign":
		return block.BambooWallHangingSign{Facing: facing}
	case "minecraft:crimson_wall_hanging_sign":
		return block.CrimsonWallHangingSign{Facing: facing}
	case "minecraft:warped_wall_hanging_sign":
		return block.WarpedWallHangingSign{Facing: facing}
	}
	return nil
}

// yawToDirection converts player yaw to a block.Direction for placement.
func yawToDirection(yaw float32) block.Direction {
	// Normalize yaw to 0-360
	y := float64(yaw)
	for y < 0 {
		y += 360
	}
	for y >= 360 {
		y -= 360
	}
	// Player faces: 0=south, 90=west, 180=north, 270=east
	switch {
	case y >= 315 || y < 45:
		return block.South
	case y >= 45 && y < 135:
		return block.West
	case y >= 135 && y < 225:
		return block.North
	default:
		return block.East
	}
}

// isDoorItem returns true if the item name is a door item.
func isDoorItem(name string) bool {
	switch name {
	case "oak_door", "spruce_door", "birch_door", "jungle_door",
		"acacia_door", "dark_oak_door", "cherry_door", "mangrove_door",
		"bamboo_door", "crimson_door", "warped_door", "iron_door":
		return true
	}
	return false
}

// placeBed places both halves of a red bed at the given position.
func (h *BlockHandler) placeBed(player *game.Player, x, y, z int, sequence int32) {
	yaw, _ := player.Rotation()
	facing := yawToDirection(yaw)

	foot := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartFoot}
	head := block.RedBed{Facing: facing, Occupied: false, Part: block.BedPartHead}

	footID, ok1 := block.ToStateID[foot]
	headID, ok2 := block.ToStateID[head]
	if !ok1 || !ok2 {
		h.sendAck(player, sequence)
		return
	}

	// Head block offset based on facing
	hx, hz := bedHeadOffset(facing)
	headX, headZ := x+hx, z+hz

	// Check that head position is air
	headState, err := h.World.GetBlock(headX, y, headZ)
	if err != nil || headState != 0 {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, footID)
	h.World.SetBlock(headX, y, headZ, headID)
	h.broadcastBlockUpdate(x, y, z, int32(footID))
	h.broadcastBlockUpdate(headX, y, headZ, int32(headID))
	h.sendAck(player, sequence)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		invItem := &player.Inventory[slot]
		if invItem.Count > 0 {
			invItem.Count--
			if invItem.Count <= 0 {
				*invItem = game.ItemStack{}
			}
			SendSlotUpdate(player, slot)
		}
	}
}

// interactBed handles right-clicking a bed: attempt sleeping or set spawn.
func (h *BlockHandler) interactBed(player *game.Player, x, y, z int) {
	if h.BedMgr != nil {
		h.BedMgr.TryStartSleep(player, x, y, z)
		return
	}
	// Fallback when BedMgr is not configured
	if h.TimeMgr != nil && h.TimeMgr.IsNight() {
		player.HasSpawnPoint = true
		player.SpawnX = float64(x) + 0.5
		player.SpawnY = float64(y) + 0.6 // slightly above bed
		player.SpawnZ = float64(z) + 0.5
		msg := chat.Message{Text: "Respawn point set", Color: "green"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
	} else {
		msg := chat.Message{Text: "You can only sleep at night", Color: "red"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
	}
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

// bedHeadOffset returns the (dx, dz) offset from foot to head for a given facing.
func bedHeadOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, -1
	case block.South:
		return 0, 1
	case block.West:
		return -1, 0
	case block.East:
		return 1, 0
	}
	return 0, 1
}

// wouldCollideWithPlayer checks if placing a block at (bx, by, bz) would overlap any player's hitbox.
func (h *BlockHandler) wouldCollideWithPlayer(bx, by, bz int) bool {
	var collides bool
	h.Manager.ForEach(func(p *game.Player) {
		px, py, pz := p.Position()
		// Player hitbox: 0.6 wide, 1.8 tall, centered on X/Z
		if float64(bx+1) > px-0.3 && float64(bx) < px+0.3 &&
			float64(by+1) > py && float64(by) < py+1.8 &&
			float64(bz+1) > pz-0.3 && float64(bz) < pz+0.3 {
			collides = true
		}
	})
	return collides
}

// handleBucket handles placing/picking up water or lava with buckets.
func (h *BlockHandler) handleBucket(player *game.Player, placeX, placeY, placeZ, clickX, clickY, clickZ int, action, heldName string, sequence int32) {
	slot := int(player.HeldSlot) + 36

	switch action {
	case "water":
		// Place water source block
		waterState, ok := block.ToStateID[block.Water{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, waterState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(waterState))
		// Replace held item with empty bucket
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		// Schedule fluid flow
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "water")
		}

	case "lava":
		// Place lava source block
		lavaState, ok := block.ToStateID[block.Lava{Level: block.Integer(0)}]
		if !ok {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(placeX, placeY, placeZ, lavaState)
		h.broadcastBlockUpdate(placeX, placeY, placeZ, int32(lavaState))
		if player.GameMode == 0 {
			bucketID := itemIDByName("bucket")
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourcePlaced(placeX, placeY, placeZ, "lava")
		}

	case "pickup":
		// Pick up source block at clicked position
		stateID, err := h.World.GetBlock(clickX, clickY, clickZ)
		if err != nil {
			h.sendAck(player, sequence)
			return
		}
		blockName := BlockNameFromState(int(stateID))
		var bucketName string
		switch blockName {
		case "water":
			// Only pick up source blocks (level=0)
			if b, ok := block.StateList[stateID].(block.Water); ok && int(b.Level) == 0 {
				bucketName = "water_bucket"
			}
		case "lava":
			if b, ok := block.StateList[stateID].(block.Lava); ok && int(b.Level) == 0 {
				bucketName = "lava_bucket"
			}
		}
		if bucketName == "" {
			h.sendAck(player, sequence)
			return
		}
		h.World.SetBlock(clickX, clickY, clickZ, 0) // remove fluid
		h.broadcastBlockUpdate(clickX, clickY, clickZ, 0)
		if player.GameMode == 0 {
			bucketID := itemIDByName(bucketName)
			player.Inventory[slot] = NewItemStack(bucketID, 1)
			SendSlotUpdate(player, slot)
		}
		if h.FluidMgr != nil {
			h.FluidMgr.OnSourceRemoved(clickX, clickY, clickZ)
		}
	}

	h.sendAck(player, sequence)
}

// plantSeed places a crop block on top of farmland and consumes the seed item.
func (h *BlockHandler) plantSeed(player *game.Player, x, y, z int, seedName string, sequence int32) bool {
	// Check that target position is air
	targetState, err := h.World.GetBlock(x, y, z)
	if err != nil || targetState != 0 {
		h.sendAck(player, sequence)
		return false
	}

	var cropBlock block.Block
	isStem := false
	switch seedName {
	case "wheat_seeds":
		cropBlock = block.Wheat{Age: block.Integer(0)}
	case "carrot":
		cropBlock = block.Carrots{Age: block.Integer(0)}
	case "potato":
		cropBlock = block.Potatoes{Age: block.Integer(0)}
	case "beetroot_seeds":
		cropBlock = block.Beetroots{Age: block.Integer(0)}
	case "pumpkin_seeds":
		cropBlock = block.PumpkinStem{Age: block.Integer(0)}
		isStem = true
	case "melon_seeds":
		cropBlock = block.MelonStem{Age: block.Integer(0)}
		isStem = true
	default:
		return false
	}

	cropStateID, ok := block.ToStateID[cropBlock]
	if !ok {
		return false
	}

	h.World.SetBlock(x, y, z, cropStateID)
	h.broadcastBlockUpdate(x, y, z, int32(cropStateID))
	h.sendAck(player, sequence)

	// Consume seed in survival
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Register crop for growth
	if h.CropMgr != nil {
		if isStem {
			h.CropMgr.RegisterStem(x, y, z)
		} else {
			h.CropMgr.RegisterCrop(x, y, z)
		}
	}

	return true
}

// placeNonFarmlandPlant handles placing sugar cane, bamboo, nether wart, and sweet berries.
func (h *BlockHandler) placeNonFarmlandPlant(player *game.Player, x, y, z int, itemName string, sequence int32) bool {
	// Check that target position is air
	targetState, err := h.World.GetBlock(x, y, z)
	if err != nil || targetState != 0 {
		h.sendAck(player, sequence)
		return false
	}

	// Check block below
	belowState, err := h.World.GetBlock(x, y-1, z)
	if err != nil {
		h.sendAck(player, sequence)
		return false
	}
	belowName := BlockNameFromState(int(belowState))

	var plantBlock block.Block

	switch itemName {
	case "sugar_cane":
		// Must be on dirt, sand, grass_block, or another sugar_cane
		if belowName != "dirt" && belowName != "sand" && belowName != "grass_block" && belowName != "sugar_cane" {
			return false
		}
		// If placing on ground (not on another sugar cane), check for adjacent water
		if belowName != "sugar_cane" {
			if !h.hasAdjacentWater(x, y-1, z) {
				return false
			}
		}
		plantBlock = block.SugarCane{Age: 0}

	case "bamboo":
		// Must be on grass_block, dirt, or sand (or another bamboo)
		if belowName != "grass_block" && belowName != "dirt" && belowName != "sand" && belowName != "bamboo" && belowName != "bamboo_sapling" {
			return false
		}
		plantBlock = block.Bamboo{Age: 0, Leaves: block.BambooLeavesNone, Stage: 0}

	case "nether_wart":
		// Must be on soul_sand
		if belowName != "soul_sand" {
			return false
		}
		plantBlock = block.NetherWart{Age: block.Integer(0)}

	case "sweet_berries":
		// Must be on grass_block or dirt
		if belowName != "grass_block" && belowName != "dirt" {
			return false
		}
		plantBlock = block.SweetBerryBush{Age: block.Integer(0)}

	default:
		return false
	}

	plantStateID, ok := block.ToStateID[plantBlock]
	if !ok {
		return false
	}

	h.World.SetBlock(x, y, z, plantStateID)
	h.broadcastBlockUpdate(x, y, z, int32(plantStateID))
	h.sendAck(player, sequence)

	// Consume item in survival
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	// Register for growth ticking
	if h.CropMgr != nil {
		switch itemName {
		case "sugar_cane":
			h.CropMgr.RegisterSugarCane(x, y, z)
		case "bamboo":
			h.CropMgr.RegisterBamboo(x, y, z)
		case "nether_wart":
			h.CropMgr.RegisterNetherWart(x, y, z)
		case "sweet_berries":
			h.CropMgr.RegisterBerry(x, y, z)
		}
	}

	return true
}

// hasAdjacentWater checks if there is water within 1 block horizontally at the given Y level.
func (h *BlockHandler) hasAdjacentWater(x, y, z int) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			ws, err := h.World.GetBlock(x+dx, y, z+dz)
			if err != nil {
				continue
			}
			if int(ws) < len(block.StateList) && block.StateList[ws] != nil {
				if _, ok := block.StateList[ws].(block.Water); ok {
					return true
				}
			}
		}
	}
	return false
}

// isCropBlock returns true if the block name is a crop (includes all crop types).
func isCropBlock(name string) bool {
	switch name {
	case "wheat", "carrots", "potatoes", "beetroots",
		"pumpkin_stem", "melon_stem", "attached_pumpkin_stem", "attached_melon_stem",
		"sugar_cane", "bamboo", "bamboo_sapling",
		"nether_wart", "cocoa", "sweet_berry_bush":
		return true
	}
	return false
}

// isSeedItem returns true if the item name is a seed that can be planted on farmland.
func isSeedItem(name string) bool {
	switch name {
	case "wheat_seeds", "carrot", "potato", "beetroot_seeds",
		"pumpkin_seeds", "melon_seeds":
		return true
	}
	return false
}

// isNonFarmlandPlantItem returns true if the item is a plant that goes on non-farmland blocks.
func isNonFarmlandPlantItem(name string) bool {
	switch name {
	case "sugar_cane", "bamboo", "nether_wart", "sweet_berries":
		return true
	}
	return false
}

// isHoeItem returns true if the item name is a hoe.
func isHoeItem(name string) bool {
	info := GetToolInfo(name)
	return info != nil && info.Type == ToolHoe
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

// placeBoat handles placing a boat item on water or on the top face of a block.
func (h *BlockHandler) placeBoat(player *game.Player, clickX, clickY, clickZ, face int, woodType int32, sequence int32) {
	// Determine spawn position: place on the clicked block's top surface or on water
	spawnX := float64(clickX) + 0.5
	spawnY := float64(clickY) + 1.0
	spawnZ := float64(clickZ) + 0.5

	// If the clicked block is water, place boat on the water surface
	clickedState, err := h.World.GetBlock(clickX, clickY, clickZ)
	if err == nil {
		if int(clickedState) < len(block.StateList) && block.StateList[clickedState] != nil {
			if _, ok := block.StateList[clickedState].(block.Water); ok {
				// Find water surface (topmost water block)
				surfaceY := clickY
				for sy := clickY + 1; sy < clickY+10; sy++ {
					aboveState, err := h.World.GetBlock(clickX, sy, clickZ)
					if err != nil {
						break
					}
					if int(aboveState) < len(block.StateList) && block.StateList[aboveState] != nil {
						if _, ok := block.StateList[aboveState].(block.Water); ok {
							surfaceY = sy
							continue
						}
					}
					break
				}
				spawnY = float64(surfaceY) + 0.5625 // boat sits slightly above water
			}
		}
	}

	h.BoatMgr.SpawnBoat(spawnX, spawnY, spawnZ, woodType)

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}

// placeMinecart places a minecart entity on the clicked rail block.
func (h *BlockHandler) placeMinecart(player *game.Player, clickX, clickY, clickZ, face int, sequence int32, itemName string) {
	// The minecart should be placed on top of the clicked block
	spawnX := float64(clickX) + 0.5
	spawnY := float64(clickY) + 0.0625 // slightly above the block
	spawnZ := float64(clickZ) + 0.5

	// Check if clicked block is a rail
	clickedState, err := h.World.GetBlock(clickX, clickY, clickZ)
	if err == nil && int(clickedState) < len(block.StateList) && block.StateList[clickedState] != nil {
		switch block.StateList[clickedState].(type) {
		case block.Rail, block.PoweredRail, block.DetectorRail, block.ActivatorRail:
			// Good, place on the rail
		default:
			// If we clicked the top of a block, check the block above for a rail
			if face == 1 {
				aboveState, err := h.World.GetBlock(clickX, clickY+1, clickZ)
				if err == nil && int(aboveState) < len(block.StateList) && block.StateList[aboveState] != nil {
					switch block.StateList[aboveState].(type) {
					case block.Rail, block.PoweredRail, block.DetectorRail, block.ActivatorRail:
						spawnY = float64(clickY+1) + 0.0625
					default:
						h.sendAck(player, sequence)
						return // not on a rail
					}
				} else {
					h.sendAck(player, sequence)
					return
				}
			} else {
				h.sendAck(player, sequence)
				return // not on a rail
			}
		}
	} else {
		h.sendAck(player, sequence)
		return
	}

	h.MinecartMgr.SpawnMinecartVariant(spawnX, spawnY, spawnZ, minecartVariantForItem(itemName))

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}

// placeRail places a rail block with auto-curving using the MinecartManager.
func (h *BlockHandler) placeRail(player *game.Player, x, y, z int, railName string, sequence int32) {
	// Check that the target position is empty (air)
	existing, err := h.World.GetBlock(x, y, z)
	if err == nil && existing != 0 {
		h.sendAck(player, sequence)
		return
	}

	// Check that there is a solid block below (rails need support)
	belowState, err := h.World.GetBlock(x, y-1, z)
	if err != nil || belowState == 0 {
		h.sendAck(player, sequence)
		return
	}

	stateID, ok := h.MinecartMgr.PlaceRail(x, y, z, railName)
	if !ok {
		h.sendAck(player, sequence)
		return
	}

	h.World.SetBlock(x, y, z, stateID)
	h.broadcastBlockUpdate(x, y, z, int32(stateID))

	// Consume item in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	h.sendAck(player, sequence)
}

func (h *BlockHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
