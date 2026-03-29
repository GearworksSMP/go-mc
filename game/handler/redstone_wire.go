package handler

import (
	"math"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// WireManager handles redstone wire power propagation and repeater/comparator logic.
type WireManager struct {
	Manager *game.PlayerManager
	World   game.World
	mu      sync.Mutex
	// wirePower tracks power level at each wire position (0-15)
	wirePower map[[3]int]int
	// scheduledRepeaters tracks repeaters waiting to toggle
	scheduledRepeaters []RepeaterSchedule
}

// NewWireManager creates a new wire propagation manager.
func NewWireManager(mgr *game.PlayerManager, world game.World) *WireManager {
	return &WireManager{
		Manager:   mgr,
		World:     world,
		wirePower: make(map[[3]int]int),
	}
}

// GetPowerLevel returns the redstone power level at a position (0-15).
func (w *WireManager) GetPowerLevel(x, y, z int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wirePower[[3]int{x, y, z}]
}

// PlaceWire places redstone wire at the given position and propagates power.
func (w *WireManager) PlaceWire(x, y, z int) {
	w.mu.Lock()
	w.wirePower[[3]int{x, y, z}] = 0
	w.mu.Unlock()

	w.propagateFrom(x, y, z)
	w.updateWireConnections(x, y, z)
}

// RemoveWire removes redstone wire and re-propagates.
func (w *WireManager) RemoveWire(x, y, z int) {
	w.mu.Lock()
	delete(w.wirePower, [3]int{x, y, z})
	w.mu.Unlock()

	// Re-propagate neighbors
	for _, off := range cardinalOffsets {
		w.propagateFrom(x+off[0], y+off[1], z+off[2])
	}
}

// cardinalOffsets are the 4 horizontal cardinal directions.
var cardinalOffsets = [4][3]int{
	{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1},
}

// UpdateFromSource recalculates wire power from a power source change at (sx, sy, sz).
func (w *WireManager) UpdateFromSource(sx, sy, sz int, powered bool) {
	// Check all adjacent positions for redstone wire
	for _, off := range adjacentOffsets {
		nx, ny, nz := sx+off[0], sy+off[1], sz+off[2]
		w.propagateFrom(nx, ny, nz)
	}
	// Also notify repeaters/comparators adjacent to the source
	w.notifyNeighborComponents(sx, sy, sz)
}

// propagateFrom runs a BFS power propagation starting from position (x, y, z).
func (w *WireManager) propagateFrom(x, y, z int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if !isRedstoneWire(int(stateID)) {
		return
	}

	// BFS to propagate power levels
	type node struct {
		x, y, z int
		power   int
	}

	// Find the power source level for this wire
	sourcePower := w.findSourcePower(x, y, z)

	queue := []node{{x, y, z, sourcePower}}
	visited := make(map[[3]int]bool)

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		key := [3]int{cur.x, cur.y, cur.z}
		if visited[key] {
			continue
		}
		visited[key] = true

		w.mu.Lock()
		oldPower := w.wirePower[key]
		if cur.power > oldPower || (cur.power == 0 && oldPower > 0) {
			w.wirePower[key] = cur.power
		} else if cur.power <= oldPower && oldPower > 0 {
			// Keep old higher power unless we're clearing
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()

		// Update wire block state
		w.setWirePower(cur.x, cur.y, cur.z, cur.power)

		// Propagate to adjacent wires (power decreases by 1)
		if cur.power > 0 {
			for _, off := range cardinalOffsets {
				nx, ny, nz := cur.x+off[0], cur.y+off[1], cur.z+off[2]
				nState, err := w.World.GetBlock(nx, ny, nz)
				if err != nil {
					continue
				}
				if isRedstoneWire(int(nState)) {
					queue = append(queue, node{nx, ny, nz, cur.power - 1})
				}
				// Wire going up a block
				upState, err := w.World.GetBlock(nx, ny+1, nz)
				if err == nil && isRedstoneWire(int(upState)) {
					queue = append(queue, node{nx, ny + 1, nz, cur.power - 1})
				}
				// Wire going down a block
				downState, err := w.World.GetBlock(nx, ny-1, nz)
				if err == nil && isRedstoneWire(int(downState)) {
					queue = append(queue, node{nx, ny - 1, nz, cur.power - 1})
				}
			}
		}
	}
}

// findSourcePower determines the power level at a wire position from adjacent power sources.
func (w *WireManager) findSourcePower(x, y, z int) int {
	maxPower := 0
	for _, off := range adjacentOffsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		state, err := w.World.GetBlock(nx, ny, nz)
		if err != nil {
			continue
		}
		name := BlockNameFromState(int(state))
		// Power sources give 15
		if isPowerSource(name, int(state)) {
			maxPower = 15
			break
		}
		// Repeaters output 15 when powered
		if isRepeaterPowered(int(state)) {
			if maxPower < 15 {
				maxPower = 15
			}
		}
	}
	return maxPower
}

// isPowerSource checks if a block provides redstone power.
func isPowerSource(name string, state int) bool {
	switch name {
	case "lever":
		if state >= 0 && state < len(block.StateList) && block.StateList[state] != nil {
			if l, ok := block.StateList[state].(block.Lever); ok {
				return bool(l.Powered)
			}
		}
	case "redstone_torch":
		if state >= 0 && state < len(block.StateList) && block.StateList[state] != nil {
			if rt, ok := block.StateList[state].(block.RedstoneTorch); ok {
				return bool(rt.Lit)
			}
		}
		return true // fallback
	case "redstone_block":
		return true
	}
	return false
}

// isRedstoneWire checks if a state ID is redstone wire.
func isRedstoneWire(stateID int) bool {
	if stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	_, ok := block.StateList[stateID].(block.RedstoneWire)
	return ok
}

// isRepeaterPowered checks if a state is a powered repeater.
func isRepeaterPowered(stateID int) bool {
	if stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	if r, ok := block.StateList[stateID].(block.Repeater); ok {
		return bool(r.Powered)
	}
	return false
}

// setWirePower updates the wire block state to reflect its power level.
func (w *WireManager) setWirePower(x, y, z, power int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	wire, ok := block.StateList[stateID].(block.RedstoneWire)
	if !ok {
		return
	}
	wire.Power = block.Integer(power)
	newID, ok := block.ToStateID[wire]
	if !ok {
		return
	}
	if int(newID) != int(stateID) {
		w.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(w.Manager, x, y, z, int32(newID))
		// Notify adjacent repeaters/comparators of wire power change
		w.notifyNeighborComponents(x, y, z)
	}
}

// updateWireConnections updates the visual connection state of redstone wire.
func (w *WireManager) updateWireConnections(x, y, z int) {
	// Redstone wire connections are purely visual in our simplified implementation;
	// the block state already tracks power level which is the functional part.
}

// RepeaterSchedule tracks a repeater's scheduled activation/deactivation.
type RepeaterSchedule struct {
	X, Y, Z   int
	TicksLeft int // game ticks remaining before toggle
	Target    bool // target powered state
}

// CycleRepeaterDelay right-clicks a repeater to cycle its delay (1→2→3→4→1).
func (w *WireManager) CycleRepeaterDelay(x, y, z int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	rep, ok := block.StateList[stateID].(block.Repeater)
	if !ok {
		return
	}

	// Cycle delay 1→2→3→4→1
	newDelay := int(rep.Delay) + 1
	if newDelay > 4 {
		newDelay = 1
	}
	rep.Delay = block.Integer(newDelay)
	newID, ok := block.ToStateID[rep]
	if !ok {
		return
	}
	w.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(w.Manager, x, y, z, int32(newID))
}

// ToggleComparatorMode right-clicks a comparator to toggle compare/subtract mode.
func (w *WireManager) ToggleComparatorMode(x, y, z int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	comp, ok := block.StateList[stateID].(block.Comparator)
	if !ok {
		return
	}

	// Toggle mode: compare ↔ subtract
	if comp.Mode == block.ComparatorModeCompare {
		comp.Mode = block.ComparatorModeSubtract
	} else {
		comp.Mode = block.ComparatorModeCompare
	}
	newID, ok := block.ToStateID[comp]
	if !ok {
		return
	}
	w.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(w.Manager, x, y, z, int32(newID))
}

// Tick processes scheduled repeater activations.
func (w *WireManager) Tick(tick int64) {
	w.mu.Lock()
	var remaining []RepeaterSchedule
	var toProcess []RepeaterSchedule
	for i := range w.scheduledRepeaters {
		w.scheduledRepeaters[i].TicksLeft--
		if w.scheduledRepeaters[i].TicksLeft <= 0 {
			toProcess = append(toProcess, w.scheduledRepeaters[i])
		} else {
			remaining = append(remaining, w.scheduledRepeaters[i])
		}
	}
	w.scheduledRepeaters = remaining
	w.mu.Unlock()

	for _, sched := range toProcess {
		w.applyRepeaterOutput(sched.X, sched.Y, sched.Z, sched.Target)
	}
}

// ScheduleRepeater queues a repeater to toggle after its delay.
func (w *WireManager) ScheduleRepeater(x, y, z int, delay int, target bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Cancel existing schedule for same position
	for i := len(w.scheduledRepeaters) - 1; i >= 0; i-- {
		s := w.scheduledRepeaters[i]
		if s.X == x && s.Y == y && s.Z == z {
			w.scheduledRepeaters = append(w.scheduledRepeaters[:i], w.scheduledRepeaters[i+1:]...)
		}
	}

	// Each delay tick = 2 game ticks (1 redstone tick)
	w.scheduledRepeaters = append(w.scheduledRepeaters, RepeaterSchedule{
		X: x, Y: y, Z: z,
		TicksLeft: delay * 2,
		Target:    target,
	})
}

// applyRepeaterOutput sets the repeater's powered state and propagates.
func (w *WireManager) applyRepeaterOutput(x, y, z int, powered bool) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	rep, ok := block.StateList[stateID].(block.Repeater)
	if !ok {
		return
	}
	if bool(rep.Powered) == powered {
		return
	}

	rep.Powered = block.Boolean(powered)
	newID, ok := block.ToStateID[rep]
	if !ok {
		return
	}
	w.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(w.Manager, x, y, z, int32(newID))

	// Propagate to wire/blocks in front of repeater
	fx, fz := facingOffset(rep.Facing)
	w.propagateFrom(x+fx, y, z+fz)
	// Notify repeaters/comparators in front of this repeater
	w.notifyNeighborComponents(x+fx, y, z+fz)
	// Also notify adjacent components at this position
	w.notifyNeighborComponents(x, y, z)
}

// UpdateRepeater checks if a repeater should schedule a toggle based on input power.
func (w *WireManager) UpdateRepeater(x, y, z int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	rep, ok := block.StateList[stateID].(block.Repeater)
	if !ok {
		return
	}

	// Check input (behind the repeater based on facing)
	bx, bz := behindOffset(rep.Facing)
	inputPowered := w.isPositionPowered(x+bx, y, z+bz)

	if inputPowered && !bool(rep.Powered) {
		w.ScheduleRepeater(x, y, z, int(rep.Delay), true)
	} else if !inputPowered && bool(rep.Powered) {
		w.ScheduleRepeater(x, y, z, int(rep.Delay), false)
	}
}

// UpdateComparator checks a comparator and updates its output power.
func (w *WireManager) UpdateComparator(x, y, z int) {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	comp, ok := block.StateList[stateID].(block.Comparator)
	if !ok {
		return
	}

	// Get rear input power
	bx, bz := behindOffset(comp.Facing)
	rearPower := w.getPowerAt(x+bx, y, z+bz)

	// Get side input power (max of both sides)
	lx, lz := leftOffset(comp.Facing)
	rx, rz := rightOffset(comp.Facing)
	sidePower := w.getPowerAt(x+lx, y, z+lz)
	if rp := w.getPowerAt(x+rx, y, z+rz); rp > sidePower {
		sidePower = rp
	}

	// Calculate output
	var output int
	if comp.Mode == block.ComparatorModeCompare {
		if rearPower >= sidePower {
			output = rearPower
		} else {
			output = 0
		}
	} else { // subtract
		output = rearPower - sidePower
		if output < 0 {
			output = 0
		}
	}

	shouldPower := output > 0
	if bool(comp.Powered) != shouldPower {
		comp.Powered = block.Boolean(shouldPower)
		newID, ok := block.ToStateID[comp]
		if !ok {
			return
		}
		w.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(w.Manager, x, y, z, int32(newID))

		// Propagate output to front
		fx, fz := facingOffset(comp.Facing)
		w.propagateFrom(x+fx, y, z+fz)
		// Notify adjacent components
		w.notifyNeighborComponents(x+fx, y, z+fz)
		w.notifyNeighborComponents(x, y, z)
	}
}

// isPositionPowered checks if a position has any redstone power.
func (w *WireManager) isPositionPowered(x, y, z int) bool {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	name := BlockNameFromState(int(stateID))
	if isPowerSource(name, int(stateID)) {
		return true
	}
	if isRepeaterPowered(int(stateID)) {
		return true
	}
	if _, ok := block.StateList[stateID].(block.Comparator); ok {
		comp := block.StateList[stateID].(block.Comparator)
		return bool(comp.Powered)
	}
	// Check wire power
	w.mu.Lock()
	p := w.wirePower[[3]int{x, y, z}]
	w.mu.Unlock()
	return p > 0
}

// getPowerAt returns the power level at a position.
func (w *WireManager) getPowerAt(x, y, z int) int {
	stateID, err := w.World.GetBlock(x, y, z)
	if err != nil {
		return 0
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return 0
	}
	name := BlockNameFromState(int(stateID))
	if isPowerSource(name, int(stateID)) {
		return 15
	}
	if isRepeaterPowered(int(stateID)) {
		return 15
	}
	// Wire power
	w.mu.Lock()
	p := w.wirePower[[3]int{x, y, z}]
	w.mu.Unlock()
	return p
}

// facingOffset returns the X,Z offset for the direction a component faces (output side).
func facingOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, -1
	case block.South:
		return 0, 1
	case block.East:
		return 1, 0
	case block.West:
		return -1, 0
	}
	return 0, 0
}

// behindOffset returns the X,Z offset for the input side (behind) of a component.
func behindOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, 1
	case block.South:
		return 0, -1
	case block.East:
		return -1, 0
	case block.West:
		return 1, 0
	}
	return 0, 0
}

// leftOffset returns the X,Z offset for the left side of a component.
func leftOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return -1, 0
	case block.South:
		return 1, 0
	case block.East:
		return 0, -1
	case block.West:
		return 0, 1
	}
	return 0, 0
}

// rightOffset returns the X,Z offset for the right side of a component.
func rightOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 1, 0
	case block.South:
		return -1, 0
	case block.East:
		return 0, 1
	case block.West:
		return 0, -1
	}
	return 0, 0
}

// notifyNeighborComponents checks all 6 neighbors of (x,y,z) for repeaters
// and comparators, and triggers their update logic.
func (w *WireManager) notifyNeighborComponents(x, y, z int) {
	for _, off := range adjacentOffsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		stateID, err := w.World.GetBlock(nx, ny, nz)
		if err != nil || int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
			continue
		}
		switch block.StateList[stateID].(type) {
		case block.Repeater:
			w.UpdateRepeater(nx, ny, nz)
		case block.Comparator:
			w.UpdateComparator(nx, ny, nz)
		}
	}
}

// WirePowersBlock returns true if redstone wire at any adjacent position
// has power > 0, providing weak power to the block at (x,y,z).
func (w *WireManager) WirePowersBlock(x, y, z int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, off := range adjacentOffsets {
		key := [3]int{x + off[0], y + off[1], z + off[2]}
		if p, ok := w.wirePower[key]; ok && p > 0 {
			return true
		}
	}
	return false
}

// round helpers (unused, but available)
func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func absF(x float64) float64 {
	return math.Abs(x)
}
