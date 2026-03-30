package handler

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
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
	BlockUpdateMgr   *BlockUpdateManager               // optional; handles block update propagation
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

func (h *BlockHandler) logf(format string, args ...any) {
	if h.Logger != nil {
		h.Logger.Printf(format, args...)
	}
}
