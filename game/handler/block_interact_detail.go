package handler

import (
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

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
