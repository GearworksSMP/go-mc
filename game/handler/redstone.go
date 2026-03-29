package handler

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Sound IDs for redstone components (0-indexed, wire sends +1).
const (
	SoundLeverClick              int32 = 555  // block.lever.click
	SoundStoneButtonClickOn      int32 = 1007 // block.stone_button.click_on
	SoundStoneButtonClickOff     int32 = 1006 // block.stone_button.click_off
	SoundWoodenButtonClickOn     int32 = 1149 // block.wooden_button.click_on
	SoundWoodenButtonClickOff    int32 = 1148 // block.wooden_button.click_off
	SoundStonePressurePlateOn    int32 = 1012 // block.stone_pressure_plate.click_on
	SoundStonePressurePlateOff   int32 = 1011 // block.stone_pressure_plate.click_off
	SoundWoodenPressurePlateOn   int32 = 1154 // block.wooden_pressure_plate.click_on
	SoundWoodenPressurePlateOff  int32 = 1153 // block.wooden_pressure_plate.click_off
	SoundMetalPressurePlateOn    int32 = 588  // block.metal_pressure_plate.click_on
	SoundMetalPressurePlateOff   int32 = 587  // block.metal_pressure_plate.click_off
	SoundIronDoorOpen            int32 = 522  // block.iron_door.open
	SoundIronDoorClose           int32 = 521  // block.iron_door.close
	SoundIronTrapdoorOpen        int32 = 530  // block.iron_trapdoor.open
	SoundIronTrapdoorClose       int32 = 529  // block.iron_trapdoor.close
)

// PowerSource tracks a redstone power source in the world.
type PowerSource struct {
	X, Y, Z    int
	Powered    bool
	SourceType string // "lever", "button", "pressure_plate"
	PulseTicks int64  // for buttons: ticks remaining until deactivation
	// Button/plate state preservation for reset
	ButtonType string           // e.g. "stone_button", "oak_button"
	Face       block.AttachFace // button attachment face
	Facing     block.Direction  // button facing direction
	// Pressure plate type for sound selection
	PlateType string // e.g. "stone_pressure_plate", "oak_pressure_plate"
}

// observerPulse tracks a pending observer output pulse.
type observerPulse struct {
	X, Y, Z    int
	TicksLeft  int
}

// RedstoneManager handles basic redstone mechanics: levers, buttons, pressure plates,
// and their effects on iron doors, iron trapdoors, and redstone lamps.
type RedstoneManager struct {
	Manager      *game.PlayerManager
	World        game.World
	Logger       *log.Logger
	WireMgr      *WireManager
	PistonMgr    *PistonManager
	DispenserMgr *DispenserManager
	TNTMgr       *TNTManager
	mu           sync.Mutex
	sources      map[[3]int]*PowerSource
	TimeMgr      *TimeManager
	observerPulses []observerPulse
	daylightDetectors map[[3]int]bool // tracked daylight detector positions
}

// NewRedstoneManager creates a new RedstoneManager.
func NewRedstoneManager(mgr *game.PlayerManager, world game.World, logger *log.Logger) *RedstoneManager {
	return &RedstoneManager{
		Manager:           mgr,
		World:             world,
		Logger:            logger,
		sources:           make(map[[3]int]*PowerSource),
		daylightDetectors: make(map[[3]int]bool),
	}
}

// adjacentOffsets are the 6 cardinal direction offsets.
var adjacentOffsets = [6][3]int{
	{0, 1, 0},  // up
	{0, -1, 0}, // down
	{1, 0, 0},  // east
	{-1, 0, 0}, // west
	{0, 0, 1},  // south
	{0, 0, -1}, // north
}

// ToggleLever toggles a lever at (x, y, z) and updates adjacent powered blocks.
func (r *RedstoneManager) ToggleLever(player *game.Player, x, y, z int) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	lever, ok := block.StateList[stateID].(block.Lever)
	if !ok {
		return
	}

	lever.Powered = !lever.Powered
	newID, ok := block.ToStateID[lever]
	if !ok {
		return
	}

	r.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))

	// Update power source tracking
	r.mu.Lock()
	key := [3]int{x, y, z}
	if bool(lever.Powered) {
		r.sources[key] = &PowerSource{
			X: x, Y: y, Z: z,
			Powered:    true,
			SourceType: "lever",
		}
	} else {
		delete(r.sources, key)
	}
	r.mu.Unlock()

	// Update adjacent powered blocks
	r.updateAdjacentPowered(x, y, z)

	// Play lever click sound
	BroadcastSound(r.Manager, SoundLeverClick, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 0.3, boolPitch(bool(lever.Powered)))
}

// ActivateButton activates a button at (x, y, z).
// buttonType is the block name (e.g. "stone_button"), pulseTicks is how long it stays active.
func (r *RedstoneManager) ActivateButton(player *game.Player, x, y, z int, buttonType string, face block.AttachFace, facing block.Direction, pulseTicks int64) {
	r.mu.Lock()
	key := [3]int{x, y, z}
	if src, exists := r.sources[key]; exists && src.Powered {
		// Already powered, ignore
		r.mu.Unlock()
		return
	}
	r.sources[key] = &PowerSource{
		X: x, Y: y, Z: z,
		Powered:    true,
		SourceType: "button",
		PulseTicks: pulseTicks,
		ButtonType: buttonType,
		Face:       face,
		Facing:     facing,
	}
	r.mu.Unlock()

	// Set button block state to powered
	pressedState := makeButtonState(buttonType, face, facing, true)
	if pressedState == nil {
		return
	}
	if newID, ok := block.ToStateID[pressedState]; ok {
		r.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
	}

	// Update adjacent powered blocks
	r.updateAdjacentPowered(x, y, z)

	// Play click sound
	soundID := SoundStoneButtonClickOn
	if buttonType != "stone_button" && buttonType != "polished_blackstone_button" {
		soundID = SoundWoodenButtonClickOn
	}
	BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 0.3, 0.6)
}

// UpdatePressurePlate updates a pressure plate at (x, y, z) based on entity presence.
func (r *RedstoneManager) UpdatePressurePlate(x, y, z int, entityOn bool) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	blockName := BlockNameFromState(int(stateID))
	if !strings.Contains(blockName, "pressure_plate") {
		return
	}

	r.mu.Lock()
	key := [3]int{x, y, z}
	src := r.sources[key]
	alreadyPowered := src != nil && src.Powered
	r.mu.Unlock()

	if entityOn && alreadyPowered {
		return // already powered, no change
	}
	if !entityOn && !alreadyPowered {
		return // already unpowered, no change
	}

	if entityOn {
		// Power the plate
		newState := makePressurePlateState(blockName, true)
		if newState == nil {
			return
		}
		if newID, ok := block.ToStateID[newState]; ok {
			r.World.SetBlock(x, y, z, newID)
			broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
		}

		r.mu.Lock()
		r.sources[key] = &PowerSource{
			X: x, Y: y, Z: z,
			Powered:    true,
			SourceType: "pressure_plate",
			PlateType:  blockName,
		}
		r.mu.Unlock()

		r.updateAdjacentPowered(x, y, z)

		// Play press sound
		soundID := pressurePlateSoundOn(blockName)
		BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.1, float64(z)+0.5, 0.3, 0.6)
	} else {
		// Unpower the plate
		newState := makePressurePlateState(blockName, false)
		if newState == nil {
			return
		}
		if newID, ok := block.ToStateID[newState]; ok {
			r.World.SetBlock(x, y, z, newID)
			broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
		}

		r.mu.Lock()
		delete(r.sources, key)
		r.mu.Unlock()

		r.updateAdjacentPowered(x, y, z)

		// Play depress sound
		soundID := pressurePlateSoundOff(blockName)
		BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.1, float64(z)+0.5, 0.3, 0.5)
	}
}

// IsPowered returns true if the block at (x, y, z) is receiving redstone power
// from any adjacent source, or if it is itself a powered source.
func (r *RedstoneManager) IsPowered(x, y, z int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if the block itself is a power source
	if src, ok := r.sources[[3]int{x, y, z}]; ok && src.Powered {
		return true
	}

	// Check all 6 adjacent positions
	for _, off := range adjacentOffsets {
		adj := [3]int{x + off[0], y + off[1], z + off[2]}
		if src, ok := r.sources[adj]; ok && src.Powered {
			return true
		}
	}

	// Check if redstone wire provides power
	if r.WireMgr != nil && r.WireMgr.WirePowersBlock(x, y, z) {
		return true
	}

	return false
}

// GetPowerLevel returns the redstone power level at a position (0-15).
// Combines power from sources (15) and wire (0-15).
func (r *RedstoneManager) GetPowerLevel(x, y, z int) int {
	if r.IsPowered(x, y, z) {
		return 15
	}
	if r.WireMgr != nil {
		return r.WireMgr.GetPowerLevel(x, y, z)
	}
	return 0
}

// updateAdjacentPowered checks all 6 neighbors of (x,y,z) and updates
// iron doors, iron trapdoors, and redstone lamps based on power state.
func (r *RedstoneManager) updateAdjacentPowered(x, y, z int) {
	for _, off := range adjacentOffsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		r.updatePoweredBlock(nx, ny, nz)
	}
	// Also check the block at the source position itself (for lamps placed at same position)
	r.updatePoweredBlock(x, y, z)

	// Notify wire manager about power source change
	if r.WireMgr != nil {
		r.WireMgr.UpdateFromSource(x, y, z, r.IsPowered(x, y, z))
	}
}

// updatePoweredBlock checks if a block at (x,y,z) is a redstone-responsive block
// and updates its state based on whether it's receiving power.
func (r *RedstoneManager) updatePoweredBlock(x, y, z int) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	b := block.StateList[stateID]
	powered := r.IsPowered(x, y, z)

	switch door := b.(type) {
	case block.IronDoor:
		if bool(door.Open) != powered {
			r.setIronDoor(x, y, z, door, powered)
		}
	case block.IronTrapdoor:
		if bool(door.Open) != powered {
			door.Open = block.Boolean(powered)
			door.Powered = block.Boolean(powered)
			if newID, ok := block.ToStateID[door]; ok {
				r.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
				// Play iron trapdoor sound
				soundID := SoundIronTrapdoorOpen
				if !powered {
					soundID = SoundIronTrapdoorClose
				}
				BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
					float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
			}
		}
	case block.RedstoneLamp:
		if bool(door.Lit) != powered {
			door.Lit = block.Boolean(powered)
			if newID, ok := block.ToStateID[door]; ok {
				r.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
			}
		}
	case block.Piston:
		if r.PistonMgr != nil {
			if powered && !bool(door.Extended) {
				r.PistonMgr.TryActivate(x, y, z)
			} else if !powered && bool(door.Extended) {
				r.PistonMgr.TryRetract(x, y, z)
			}
		}
	case block.StickyPiston:
		if r.PistonMgr != nil {
			if powered && !bool(door.Extended) {
				r.PistonMgr.TryActivate(x, y, z)
			} else if !powered && bool(door.Extended) {
				r.PistonMgr.TryRetract(x, y, z)
			}
		}
	case block.Dispenser:
		if powered && r.DispenserMgr != nil {
			r.DispenserMgr.Activate(x, y, z)
		}
	case block.Dropper:
		if powered && r.DispenserMgr != nil {
			r.DispenserMgr.Activate(x, y, z)
		}
	case block.NoteBlock:
		if powered {
			r.PlayNoteBlockPowered(x, y, z)
		}
	case block.RedstoneTorch:
		r.UpdateRedstoneTorch(x, y, z)
	case block.RedstoneWallTorch:
		r.UpdateRedstoneTorch(x, y, z)
	case block.Tnt:
		if powered && r.TNTMgr != nil {
			r.World.SetBlock(x, y, z, 0)
			broadcastBlockUpdateDirect(r.Manager, x, y, z, 0)
			r.TNTMgr.Ignite(x, y, z)
		}
	case block.PoweredRail:
		r.updatePoweredRail(x, y, z, door, powered)
	default:
		r.updateFenceGate(x, y, z, b, powered)
	}
}

// setIronDoor updates both halves of an iron door.
func (r *RedstoneManager) setIronDoor(x, y, z int, door block.IronDoor, open bool) {
	// Update clicked half
	door.Open = block.Boolean(open)
	door.Powered = block.Boolean(open)
	if newID, ok := block.ToStateID[door]; ok {
		r.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
	}

	// Determine and update the other half
	otherY := y + 1
	if door.Half == block.DoubleBlockHalfUpper {
		otherY = y - 1
	}
	otherHalf := block.DoubleBlockHalfUpper
	if door.Half == block.DoubleBlockHalfUpper {
		otherHalf = block.DoubleBlockHalfLower
	}

	otherState, err := r.World.GetBlock(x, otherY, z)
	if err != nil {
		return
	}
	if int(otherState) >= len(block.StateList) || block.StateList[otherState] == nil {
		return
	}

	if other, ok := block.StateList[otherState].(block.IronDoor); ok {
		other.Open = block.Boolean(open)
		other.Powered = block.Boolean(open)
		other.Half = otherHalf
		if newID, ok := block.ToStateID[other]; ok {
			r.World.SetBlock(x, otherY, z, newID)
			broadcastBlockUpdateDirect(r.Manager, x, otherY, z, int32(newID))
		}
	}

	// Play iron door sound
	soundID := SoundIronDoorOpen
	if !open {
		soundID = SoundIronDoorClose
	}
	BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
}

// Tick processes time-based redstone events (button pulse expiry, pressure plate checks).
func (r *RedstoneManager) Tick(tick int64) {
	r.mu.Lock()
	// Collect sources that need updates (to avoid holding lock during world ops)
	var buttonsToReset [][3]int
	var platesToCheck [][3]int

	for key, src := range r.sources {
		if src.SourceType == "button" && src.Powered {
			src.PulseTicks--
			if src.PulseTicks <= 0 {
				buttonsToReset = append(buttonsToReset, key)
			}
		}
		if src.SourceType == "pressure_plate" && src.Powered {
			platesToCheck = append(platesToCheck, key)
		}
	}

	// Reset expired buttons
	for _, key := range buttonsToReset {
		src := r.sources[key]
		if src == nil {
			continue
		}
		src.Powered = false
		delete(r.sources, key)
	}
	r.mu.Unlock()

	// Process button resets outside the lock
	for _, key := range buttonsToReset {
		x, y, z := key[0], key[1], key[2]

		// We need to read current state to get face/facing for reset
		stateID, err := r.World.GetBlock(x, y, z)
		if err != nil {
			continue
		}
		if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
			// Reset the button to unpowered by reading current state
			resetBlock := resetButtonState(block.StateList[stateID])
			if resetBlock != nil {
				if newID, ok := block.ToStateID[resetBlock]; ok {
					r.World.SetBlock(x, y, z, newID)
					broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
				}
			}
		}

		r.updateAdjacentPowered(x, y, z)

		// Play click-off sound
		blockName := BlockNameFromState(int(stateID))
		soundID := SoundStoneButtonClickOff
		if blockName != "stone_button" && blockName != "polished_blackstone_button" {
			soundID = SoundWoodenButtonClickOff
		}
		BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 0.3, 0.5)
	}

	// Check pressure plates for player presence
	for _, key := range platesToCheck {
		x, y, z := key[0], key[1], key[2]
		if !r.anyPlayerOnPlate(x, y, z) {
			r.UpdatePressurePlate(x, y, z, false)
		}
	}

	// Tick observer pulses
	r.tickObservers()

	// Tick daylight detectors every 20 ticks (1 second)
	if tick%20 == 0 {
		r.tickDaylightDetectors()
	}
}

// anyPlayerOnPlate returns true if any player is standing on the pressure plate at (x, y, z).
func (r *RedstoneManager) anyPlayerOnPlate(x, y, z int) bool {
	found := false
	r.Manager.ForEach(func(p *game.Player) {
		if found {
			return
		}
		px, py, pz := p.Position()
		// Player feet are at py; the plate is at y.
		// Check if player is on top of the plate (within a small tolerance).
		bx := int(math.Floor(px))
		bz := int(math.Floor(pz))
		if bx == x && bz == z {
			// Player's feet should be at or just above the plate
			feetY := py
			plateTop := float64(y) + 0.0625 // pressure plates are 1/16 block tall
			if feetY >= float64(y) && feetY <= plateTop+0.2 {
				found = true
			}
		}
	})
	return found
}

// CheckPressurePlateAt checks the block at the player's feet position and activates
// a pressure plate if present. Called from movement handling.
func (r *RedstoneManager) CheckPressurePlateAt(px, py, pz float64) {
	// Check block at player's feet (slightly below)
	bx := int(math.Floor(px))
	by := int(math.Floor(py))
	bz := int(math.Floor(pz))

	// Check the block at feet level and one below
	for _, testY := range []int{by, by - 1} {
		stateID, err := r.World.GetBlock(bx, testY, bz)
		if err != nil {
			continue
		}
		blockName := BlockNameFromState(int(stateID))
		if strings.Contains(blockName, "pressure_plate") {
			r.UpdatePressurePlate(bx, testY, bz, true)
			return
		}
	}
}

// CleanupSource removes a power source at the given position (e.g., when the block is broken).
func (r *RedstoneManager) CleanupSource(x, y, z int) {
	r.mu.Lock()
	key := [3]int{x, y, z}
	_, existed := r.sources[key]
	delete(r.sources, key)
	r.mu.Unlock()

	if existed {
		r.updateAdjacentPowered(x, y, z)
	}
}

// --- Helper functions ---

// boolPitch returns a pitch value based on a boolean: higher for on, lower for off.
func boolPitch(on bool) float32 {
	if on {
		return 0.6
	}
	return 0.5
}

// makeButtonState creates a button block state for the given type with the powered flag.
func makeButtonState(buttonType string, face block.AttachFace, facing block.Direction, powered bool) block.Block {
	p := block.Boolean(powered)
	switch buttonType {
	case "stone_button":
		return block.StoneButton{Face: face, Facing: facing, Powered: p}
	case "oak_button":
		return block.OakButton{Face: face, Facing: facing, Powered: p}
	case "spruce_button":
		return block.SpruceButton{Face: face, Facing: facing, Powered: p}
	case "birch_button":
		return block.BirchButton{Face: face, Facing: facing, Powered: p}
	case "jungle_button":
		return block.JungleButton{Face: face, Facing: facing, Powered: p}
	case "acacia_button":
		return block.AcaciaButton{Face: face, Facing: facing, Powered: p}
	case "dark_oak_button":
		return block.DarkOakButton{Face: face, Facing: facing, Powered: p}
	case "cherry_button":
		return block.CherryButton{Face: face, Facing: facing, Powered: p}
	case "mangrove_button":
		return block.MangroveButton{Face: face, Facing: facing, Powered: p}
	case "bamboo_button":
		return block.BambooButton{Face: face, Facing: facing, Powered: p}
	case "crimson_button":
		return block.CrimsonButton{Face: face, Facing: facing, Powered: p}
	case "warped_button":
		return block.WarpedButton{Face: face, Facing: facing, Powered: p}
	case "polished_blackstone_button":
		return block.PolishedBlackstoneButton{Face: face, Facing: facing, Powered: p}
	}
	return nil
}

// resetButtonState takes a powered button block and returns the unpowered variant.
func resetButtonState(b block.Block) block.Block {
	switch btn := b.(type) {
	case block.StoneButton:
		btn.Powered = false
		return btn
	case block.OakButton:
		btn.Powered = false
		return btn
	case block.SpruceButton:
		btn.Powered = false
		return btn
	case block.BirchButton:
		btn.Powered = false
		return btn
	case block.JungleButton:
		btn.Powered = false
		return btn
	case block.AcaciaButton:
		btn.Powered = false
		return btn
	case block.DarkOakButton:
		btn.Powered = false
		return btn
	case block.CherryButton:
		btn.Powered = false
		return btn
	case block.MangroveButton:
		btn.Powered = false
		return btn
	case block.BambooButton:
		btn.Powered = false
		return btn
	case block.CrimsonButton:
		btn.Powered = false
		return btn
	case block.WarpedButton:
		btn.Powered = false
		return btn
	case block.PolishedBlackstoneButton:
		btn.Powered = false
		return btn
	}
	return nil
}

// makePressurePlateState creates a pressure plate block state for the given type.
func makePressurePlateState(plateName string, powered bool) block.Block {
	p := block.Boolean(powered)
	switch plateName {
	case "stone_pressure_plate":
		return block.StonePressurePlate{Powered: p}
	case "oak_pressure_plate":
		return block.OakPressurePlate{Powered: p}
	case "spruce_pressure_plate":
		return block.SprucePressurePlate{Powered: p}
	case "birch_pressure_plate":
		return block.BirchPressurePlate{Powered: p}
	case "jungle_pressure_plate":
		return block.JunglePressurePlate{Powered: p}
	case "acacia_pressure_plate":
		return block.AcaciaPressurePlate{Powered: p}
	case "cherry_pressure_plate":
		return block.CherryPressurePlate{Powered: p}
	case "dark_oak_pressure_plate":
		return block.DarkOakPressurePlate{Powered: p}
	case "mangrove_pressure_plate":
		return block.MangrovePressurePlate{Powered: p}
	case "bamboo_pressure_plate":
		return block.BambooPressurePlate{Powered: p}
	case "crimson_pressure_plate":
		return block.CrimsonPressurePlate{Powered: p}
	case "warped_pressure_plate":
		return block.WarpedPressurePlate{Powered: p}
	case "polished_blackstone_pressure_plate":
		return block.PolishedBlackstonePressurePlate{Powered: p}
	}
	// Weighted pressure plates use power level (0 or 15) instead of boolean
	if plateName == "light_weighted_pressure_plate" {
		power := block.Integer(0)
		if powered {
			power = 15
		}
		return block.LightWeightedPressurePlate{Power: power}
	}
	if plateName == "heavy_weighted_pressure_plate" {
		power := block.Integer(0)
		if powered {
			power = 15
		}
		return block.HeavyWeightedPressurePlate{Power: power}
	}
	return nil
}

// pressurePlateSoundOn returns the click-on sound for a pressure plate type.
func pressurePlateSoundOn(plateName string) int32 {
	if plateName == "stone_pressure_plate" || plateName == "polished_blackstone_pressure_plate" {
		return SoundStonePressurePlateOn
	}
	if plateName == "light_weighted_pressure_plate" || plateName == "heavy_weighted_pressure_plate" {
		return SoundMetalPressurePlateOn
	}
	return SoundWoodenPressurePlateOn
}

// pressurePlateSoundOff returns the click-off sound for a pressure plate type.
func pressurePlateSoundOff(plateName string) int32 {
	if plateName == "stone_pressure_plate" || plateName == "polished_blackstone_pressure_plate" {
		return SoundStonePressurePlateOff
	}
	if plateName == "light_weighted_pressure_plate" || plateName == "heavy_weighted_pressure_plate" {
		return SoundMetalPressurePlateOff
	}
	return SoundWoodenPressurePlateOff
}

// ---------------------------------------------------------------------------
// Note Block
// ---------------------------------------------------------------------------

// CycleNoteBlock cycles the note value of a note block (0-24) and plays the note.
func (r *RedstoneManager) CycleNoteBlock(x, y, z int) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	nb, ok := block.StateList[stateID].(block.NoteBlock)
	if !ok {
		return
	}

	// Cycle note 0→1→...→24→0
	newNote := int(nb.Note) + 1
	if newNote > 24 {
		newNote = 0
	}
	nb.Note = block.Integer(newNote)
	newID, ok := block.ToStateID[nb]
	if !ok {
		return
	}
	r.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))

	r.playNoteBlock(x, y, z, nb)
}

// PlayNoteBlockPowered plays a note block when it receives redstone power.
func (r *RedstoneManager) PlayNoteBlockPowered(x, y, z int) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	nb, ok := block.StateList[stateID].(block.NoteBlock)
	if !ok {
		return
	}
	r.playNoteBlock(x, y, z, nb)
}

// noteBlockSoundID maps a NoteBlockInstrument to the correct sound ID.
func noteBlockSoundID(inst block.NoteBlockInstrument) int32 {
	switch inst {
	case block.NoteBlockInstrumentHarp:
		return 692
	case block.NoteBlockInstrumentBasedrum:
		return 686
	case block.NoteBlockInstrumentSnare:
		return 695
	case block.NoteBlockInstrumentHat:
		return 693
	case block.NoteBlockInstrumentBass:
		return 687
	case block.NoteBlockInstrumentFlute:
		return 690
	case block.NoteBlockInstrumentBell:
		return 688
	case block.NoteBlockInstrumentGuitar:
		return 691
	case block.NoteBlockInstrumentChime:
		return 689
	case block.NoteBlockInstrumentXylophone:
		return 696
	case block.NoteBlockInstrumentIronXylophone:
		return 697
	case block.NoteBlockInstrumentCowBell:
		return 698
	case block.NoteBlockInstrumentDidgeridoo:
		return 699
	case block.NoteBlockInstrumentBit:
		return 700
	case block.NoteBlockInstrumentBanjo:
		return 701
	case block.NoteBlockInstrumentPling:
		return 694
	default:
		return 692 // harp fallback for mob head instruments
	}
}

// playNoteBlock plays the note sound with correct pitch.
// Pitch: 2^((note - 12) / 12), so note 0 = F#3, note 12 = F#4, note 24 = F#5.
func (r *RedstoneManager) playNoteBlock(x, y, z int, nb block.NoteBlock) {
	note := int(nb.Note)
	pitch := float32(math.Pow(2.0, float64(note-12)/12.0))

	soundID := noteBlockSoundID(nb.Instrument)
	BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+1.0, float64(z)+0.5, 3.0, pitch)

	// Send block event (action 0 = play note) for note particle
	blockStateID, _ := r.World.GetBlock(x, y, z)
	noteEventPkt := pk.Marshal(
		packetid.ClientboundBlockEvent,
		pk.Position{X: x, Y: y, Z: z},
		pk.UnsignedByte(0), // action: play note
		pk.UnsignedByte(byte(note)),
		pk.VarInt(blockStateID),
	)
	r.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(noteEventPkt)
	})
}

// ---------------------------------------------------------------------------
// Redstone Torch
// ---------------------------------------------------------------------------

// UpdateRedstoneTorch checks if a redstone torch should turn off/on based on
// the block it's attached to receiving power.
// A redstone torch inverts: it's OFF when the block below (or attached to) is powered.
func (r *RedstoneManager) UpdateRedstoneTorch(x, y, z int) {
	stateID, err := r.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	// Handle both wall and floor redstone torches
	switch torch := block.StateList[stateID].(type) {
	case block.RedstoneTorch:
		// Floor torch: attached to block below
		blockPowered := r.isBlockDirectlyPowered(x, y-1, z)
		if blockPowered == bool(torch.Lit) {
			torch.Lit = block.Boolean(!blockPowered)
			if newID, ok := block.ToStateID[torch]; ok {
				r.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
				r.updateAdjacentPowered(x, y, z)
			}
		}
	case block.RedstoneWallTorch:
		// Wall torch: attached to block behind it
		bx, by, bz := wallTorchAttached(x, y, z, torch.Facing)
		blockPowered := r.isBlockDirectlyPowered(bx, by, bz)
		if blockPowered == bool(torch.Lit) {
			torch.Lit = block.Boolean(!blockPowered)
			if newID, ok := block.ToStateID[torch]; ok {
				r.World.SetBlock(x, y, z, newID)
				broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
				r.updateAdjacentPowered(x, y, z)
			}
		}
	}
}

// wallTorchAttached returns the position of the block a wall torch is attached to.
func wallTorchAttached(x, y, z int, facing block.Direction) (int, int, int) {
	switch facing {
	case block.North:
		return x, y, z + 1
	case block.South:
		return x, y, z - 1
	case block.East:
		return x - 1, y, z
	case block.West:
		return x + 1, y, z
	}
	return x, y, z
}

// isBlockDirectlyPowered checks if a block has a direct power source adjacent
// (not through wire). Used for redstone torch burnout logic.
func (r *RedstoneManager) isBlockDirectlyPowered(x, y, z int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, off := range adjacentOffsets {
		adj := [3]int{x + off[0], y + off[1], z + off[2]}
		if src, ok := r.sources[adj]; ok && src.Powered {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tripwire
// ---------------------------------------------------------------------------

// TripwireState tracks a tripwire hook pair and its string.
type TripwireState struct {
	HookA  [3]int // first hook position
	HookB  [3]int // second hook position (zero if not connected)
	Facing block.Direction
}

// CheckTripwire checks if an entity at (ex, ey, ez) is on a tripwire string,
// and if so, powers the connected hooks.
func (r *RedstoneManager) CheckTripwire(ex, ey, ez float64) {
	// Convert to block coords
	bx, by, bz := int(math.Floor(ex)), int(math.Floor(ey)), int(math.Floor(ez))

	stateID, err := r.World.GetBlock(bx, by, bz)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	tw, ok := block.StateList[stateID].(block.Tripwire)
	if !ok {
		return
	}

	if bool(tw.Powered) {
		return // already triggered
	}

	// Set tripwire to powered
	tw.Powered = true
	if newID, ok := block.ToStateID[tw]; ok {
		r.World.SetBlock(bx, by, bz, newID)
		broadcastBlockUpdateDirect(r.Manager, bx, by, bz, int32(newID))
	}

	// Find and power connected hooks (search along connected directions)
	r.powerTripwireHooksNear(bx, by, bz)
}

// powerTripwireHooksNear searches along the 4 horizontal directions from a powered
// tripwire for hooks, and sets them to powered.
func (r *RedstoneManager) powerTripwireHooksNear(x, y, z int) {
	directions := [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}
	for _, dir := range directions {
		for dist := 1; dist <= 40; dist++ {
			nx, ny, nz := x+dir[0]*dist, y, z+dir[2]*dist
			sid, err := r.World.GetBlock(nx, ny, nz)
			if err != nil {
				break
			}
			if int(sid) >= len(block.StateList) || block.StateList[sid] == nil {
				break
			}
			switch hook := block.StateList[sid].(type) {
			case block.TripwireHook:
				if !bool(hook.Powered) {
					hook.Powered = true
					hook.Attached = true
					if newID, ok := block.ToStateID[hook]; ok {
						r.World.SetBlock(nx, ny, nz, newID)
						broadcastBlockUpdateDirect(r.Manager, nx, ny, nz, int32(newID))
						r.updateAdjacentPowered(nx, ny, nz)
					}
				}
				return // found hook, stop searching
			case block.Tripwire:
				_ = hook
				continue // keep searching along string
			default:
				return // non-tripwire block, stop
			}
		}
	}
}

// updatePoweredRail updates a powered rail's state based on redstone power.
func (r *RedstoneManager) updatePoweredRail(x, y, z int, rail block.PoweredRail, powered bool) {
	if bool(rail.Powered) == powered {
		return
	}
	rail.Powered = block.Boolean(powered)
	if newID, ok := block.ToStateID[rail]; ok {
		r.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
	}
}

// updateFenceGate checks if a block is a fence gate and toggles it.
func (r *RedstoneManager) updateFenceGate(x, y, z int, b block.Block, powered bool) {
	switch gate := b.(type) {
	case block.OakFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.SpruceFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.BirchFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.JungleFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.AcaciaFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.CherryFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.DarkOakFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.MangroveFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.BambooFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.CrimsonFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	case block.WarpedFenceGate:
		if bool(gate.Open) != powered {
			gate.Open = block.Boolean(powered)
			gate.Powered = block.Boolean(powered)
			r.setFenceGate(x, y, z, gate, powered)
		}
	}
}

// setFenceGate sets the block state and plays the appropriate sound.
func (r *RedstoneManager) setFenceGate(x, y, z int, gate block.Block, opened bool) {
	if newID, ok := block.ToStateID[gate]; ok {
		r.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(r.Manager, x, y, z, int32(newID))
		soundID := SoundFenceGateOpen
		if !opened {
			soundID = SoundFenceGateClose
		}
		BroadcastSound(r.Manager, soundID, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	}
}

// ---------------------------------------------------------------------------
// Observer block-change detection
// ---------------------------------------------------------------------------

// oppositeDirection returns the opposite facing direction.
func oppositeDirection(d block.Direction) block.Direction {
	switch d {
	case block.Down:
		return block.Up
	case block.Up:
		return block.Down
	case block.North:
		return block.South
	case block.South:
		return block.North
	case block.West:
		return block.East
	case block.East:
		return block.West
	}
	return d
}

// NotifyBlockChange should be called whenever a block is placed, broken, or changed
// at (x, y, z). It checks all 6 neighbors for observer blocks facing this position
// and schedules a 2-tick redstone pulse from the observer's back face.
func (r *RedstoneManager) NotifyBlockChange(x, y, z int) {
	for _, off := range adjacentOffsets {
		ox, oy, oz := x+off[0], y+off[1], z+off[2]
		stateID, err := r.World.GetBlock(ox, oy, oz)
		if err != nil {
			continue
		}
		if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
			continue
		}
		obs, ok := block.StateList[stateID].(block.Observer)
		if !ok {
			continue
		}
		// Check if the observer's face points toward the changed block
		fdx, fdy, fdz := directionOffset(block.Direction(obs.Facing))
		if ox+fdx == x && oy+fdy == y && oz+fdz == z {
			// Already powered? Skip to avoid retriggering
			if bool(obs.Powered) {
				continue
			}
			// Schedule a 2-tick pulse
			r.mu.Lock()
			r.observerPulses = append(r.observerPulses, observerPulse{
				X: ox, Y: oy, Z: oz,
				TicksLeft: 2,
			})
			r.mu.Unlock()

			// Set observer to powered state
			obs.Powered = true
			if newID, ok2 := block.ToStateID[obs]; ok2 {
				r.World.SetBlock(ox, oy, oz, newID)
				broadcastBlockUpdateDirect(r.Manager, ox, oy, oz, int32(newID))
			}
		}
	}

	// Also notify repeaters/comparators adjacent to the changed block
	if r.WireMgr != nil {
		r.WireMgr.notifyNeighborComponents(x, y, z)
	}
}

// tickObservers processes observer pulse countdowns.
func (r *RedstoneManager) tickObservers() {
	r.mu.Lock()
	var remaining []observerPulse
	var expired []observerPulse
	for i := range r.observerPulses {
		r.observerPulses[i].TicksLeft--
		if r.observerPulses[i].TicksLeft <= 0 {
			expired = append(expired, r.observerPulses[i])
		} else {
			remaining = append(remaining, r.observerPulses[i])
		}
	}
	r.observerPulses = remaining
	r.mu.Unlock()

	for _, p := range expired {
		stateID, err := r.World.GetBlock(p.X, p.Y, p.Z)
		if err != nil {
			continue
		}
		if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
			continue
		}
		obs, ok := block.StateList[stateID].(block.Observer)
		if !ok {
			continue
		}
		// Reset observer to unpowered
		obs.Powered = false
		if newID, ok2 := block.ToStateID[obs]; ok2 {
			r.World.SetBlock(p.X, p.Y, p.Z, newID)
			broadcastBlockUpdateDirect(r.Manager, p.X, p.Y, p.Z, int32(newID))
		}

		// Emit pulse from the back face (opposite of facing)
		bdx, bdy, bdz := directionOffset(oppositeDirection(block.Direction(obs.Facing)))
		bx, by, bz := p.X+bdx, p.Y+bdy, p.Z+bdz
		r.updatePoweredBlock(bx, by, bz)
		if r.WireMgr != nil {
			r.WireMgr.UpdateFromSource(p.X, p.Y, p.Z, false)
		}
	}
}

// ---------------------------------------------------------------------------
// Daylight detector time-based power
// ---------------------------------------------------------------------------

// TrackDaylightDetector registers a daylight detector position for tick updates.
func (r *RedstoneManager) TrackDaylightDetector(x, y, z int) {
	r.mu.Lock()
	r.daylightDetectors[[3]int{x, y, z}] = true
	r.mu.Unlock()
}

// UntrackDaylightDetector removes a daylight detector from tick updates.
func (r *RedstoneManager) UntrackDaylightDetector(x, y, z int) {
	r.mu.Lock()
	delete(r.daylightDetectors, [3]int{x, y, z})
	r.mu.Unlock()
}

// tickDaylightDetectors updates daylight detector power levels based on world time.
func (r *RedstoneManager) tickDaylightDetectors() {
	r.mu.Lock()
	detectors := make([][3]int, 0, len(r.daylightDetectors))
	for pos := range r.daylightDetectors {
		detectors = append(detectors, pos)
	}
	r.mu.Unlock()

	if r.TimeMgr == nil {
		return
	}
	dayTime := r.TimeMgr.GetDayTime()
	// Calculate base power: strongest at noon (6000), zero at night
	// Day: 0-12000 ticks. Night: 12000-24000.
	var basePower int
	if dayTime <= 12000 {
		// Distance from noon
		dist := dayTime - 6000
		if dist < 0 {
			dist = -dist
		}
		basePower = 15 - int(dist/400)
		if basePower < 0 {
			basePower = 0
		}
	}

	for _, pos := range detectors {
		stateID, err := r.World.GetBlock(pos[0], pos[1], pos[2])
		if err != nil {
			r.mu.Lock()
			delete(r.daylightDetectors, pos)
			r.mu.Unlock()
			continue
		}
		if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
			continue
		}
		det, ok := block.StateList[stateID].(block.DaylightDetector)
		if !ok {
			r.mu.Lock()
			delete(r.daylightDetectors, pos)
			r.mu.Unlock()
			continue
		}

		power := basePower
		if bool(det.Inverted) {
			power = 15 - basePower
		}
		if power < 0 {
			power = 0
		}

		if int(det.Power) != power {
			det.Power = block.Integer(power)
			if newID, ok2 := block.ToStateID[det]; ok2 {
				r.World.SetBlock(pos[0], pos[1], pos[2], newID)
				broadcastBlockUpdateDirect(r.Manager, pos[0], pos[1], pos[2], int32(newID))
			}
			// Update adjacent powered blocks
			r.updateAdjacentPowered(pos[0], pos[1], pos[2])
		}
	}
}

// getContainerFillLevel returns the comparator signal strength (0-15) for a container
// block at (x,y,z), or -1 if the block is not a container. Uses the vanilla formula:
// if empty return 0, else floor(1 + (sum of count/maxStack for each slot) / totalSlots * 14).
func (w *WireManager) getContainerFillLevel(x, y, z int) int {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return -1
	}
	name := BlockNameFromState(int(stateID))

	switch name {
	case "chest", "trapped_chest":
		if w.ChestMgr == nil {
			return 0
		}
		cs := w.ChestMgr.Get(x, y, z)
		if cs == nil {
			return 0
		}
		return calcFillLevel(cs.Items[:], 64)

	case "furnace", "smoker", "blast_furnace":
		if w.FurnaceMgr == nil {
			return 0
		}
		fs := w.FurnaceMgr.Get(x, y, z)
		if fs == nil {
			return 0
		}
		slots := []game.ItemStack{fs.Input, fs.Fuel, fs.Output}
		return calcFillLevel(slots, 64)

	case "hopper":
		if w.HopperMgr == nil {
			return 0
		}
		hs := w.HopperMgr.Get(x, y, z)
		if hs == nil {
			return 0
		}
		return calcFillLevel(hs.Slots[:], 64)

	case "barrel":
		if w.BarrelMgr == nil {
			return 0
		}
		bs := w.BarrelMgr.Get(x, y, z)
		if bs == nil {
			return 0
		}
		return calcFillLevel(bs.Items[:], 64)

	case "brewing_stand":
		if w.BrewingMgr == nil {
			return 0
		}
		bs := w.BrewingMgr.Get(x, y, z)
		if bs == nil {
			return 0
		}
		slots := []game.ItemStack{bs.Bottles[0], bs.Bottles[1], bs.Bottles[2], bs.Ingredient, bs.Fuel}
		return calcFillLevel(slots, 64)
	}

	return -1 // not a container block
}

// calcFillLevel computes the vanilla comparator signal strength for an inventory.
// maxStack is the max stack size per slot (typically 64).
func calcFillLevel(slots []game.ItemStack, maxStack int) int {
	if len(slots) == 0 {
		return 0
	}
	totalSlots := len(slots)
	sum := 0.0
	nonEmpty := false
	for _, s := range slots {
		if s.ID > 0 && s.Count > 0 {
			nonEmpty = true
			sum += float64(s.Count) / float64(maxStack)
		}
	}
	if !nonEmpty {
		return 0
	}
	level := int(1 + sum/float64(totalSlots)*14)
	if level > 15 {
		level = 15
	}
	return level
}
