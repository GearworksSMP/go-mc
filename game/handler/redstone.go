package handler

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
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

// RedstoneManager handles basic redstone mechanics: levers, buttons, pressure plates,
// and their effects on iron doors, iron trapdoors, and redstone lamps.
type RedstoneManager struct {
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex
	sources map[[3]int]*PowerSource
}

// NewRedstoneManager creates a new RedstoneManager.
func NewRedstoneManager(mgr *game.PlayerManager, world game.World, logger *log.Logger) *RedstoneManager {
	return &RedstoneManager{
		Manager: mgr,
		World:   world,
		Logger:  logger,
		sources: make(map[[3]int]*PowerSource),
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
	return false
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
