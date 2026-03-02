package handler

import (
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// FluidManager handles water and lava flow simulation.
type FluidManager struct {
	World   game.World
	Manager *game.PlayerManager

	mu      sync.Mutex
	pending map[[3]int]pendingFlow // position → flow info
}

type pendingFlow struct {
	fluid string // "water" or "lava"
	tick  int64  // tick when to process
}

// NewFluidManager creates a new FluidManager.
func NewFluidManager(world game.World, manager *game.PlayerManager) *FluidManager {
	return &FluidManager{
		World:   world,
		Manager: manager,
		pending: make(map[[3]int]pendingFlow),
	}
}

// OnSourcePlaced schedules flow from a newly placed source block.
func (f *FluidManager) OnSourcePlaced(x, y, z int, fluid string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delay := int64(5) // water: 5 ticks
	if fluid == "lava" {
		delay = 30
	}

	// Schedule downward flow first (higher priority = shorter delay)
	downPos := [3]int{x, y - 1, z}
	f.pending[downPos] = pendingFlow{fluid: fluid, tick: delay / 2}

	// Schedule adjacent horizontal blocks
	for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		pos := [3]int{x + offset[0], y + offset[1], z + offset[2]}
		f.pending[pos] = pendingFlow{fluid: fluid, tick: delay}
	}
}

// CheckFlowDown checks if there's fluid at (x, y, z) and schedules downward flow.
// Called when a block is broken and there might be fluid above.
func (f *FluidManager) CheckFlowDown(x, y, z int) {
	stateID, err := f.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var fluid string
	switch block.StateList[stateID].(type) {
	case block.Water:
		fluid = "water"
	case block.Lava:
		fluid = "lava"
	default:
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	delay := int64(2)
	if fluid == "lava" {
		delay = 15
	}
	downPos := [3]int{x, y - 1, z}
	f.pending[downPos] = pendingFlow{fluid: fluid, tick: delay}
}

// OnSourceRemoved removes a source and cleans up connected flowing blocks.
func (f *FluidManager) OnSourceRemoved(x, y, z int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Remove all flowing blocks connected to this source (BFS)
	visited := make(map[[3]int]bool)
	queue := [][3]int{{x, y, z}}
	visited[[3]int{x, y, z}] = true

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		// Check horizontal and downward neighbors
		for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}} {
			pos := [3]int{cur[0] + offset[0], cur[1] + offset[1], cur[2] + offset[2]}
			if visited[pos] {
				continue
			}

			stateID, err := f.World.GetBlock(pos[0], pos[1], pos[2])
			if err != nil {
				continue
			}

			// Check if this is a flowing fluid block (level > 0)
			if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
				switch b := block.StateList[stateID].(type) {
				case block.Water:
					if int(b.Level) > 0 {
						f.World.SetBlock(pos[0], pos[1], pos[2], 0)
						broadcastBlockUpdateDirect(f.Manager, pos[0], pos[1], pos[2], 0)
						visited[pos] = true
						queue = append(queue, pos)
					}
				case block.Lava:
					if int(b.Level) > 0 {
						f.World.SetBlock(pos[0], pos[1], pos[2], 0)
						broadcastBlockUpdateDirect(f.Manager, pos[0], pos[1], pos[2], 0)
						visited[pos] = true
						queue = append(queue, pos)
					}
				}
			}

			// Remove from pending
			delete(f.pending, pos)
		}
	}
}

// Tick processes pending fluid flow. Called every server tick.
func (f *FluidManager) Tick(tick int64) {
	f.mu.Lock()

	var toProcess []pendingFlowEntry
	for pos, pf := range f.pending {
		pf.tick--
		if pf.tick <= 0 {
			toProcess = append(toProcess, pendingFlowEntry{pos: pos, fluid: pf.fluid})
		} else {
			f.pending[pos] = pf
		}
	}

	// Remove processed from pending
	for _, entry := range toProcess {
		delete(f.pending, entry.pos)
	}

	f.mu.Unlock()

	// Process each pending position
	for _, entry := range toProcess {
		f.processFlow(entry.pos, entry.fluid)
	}
}

type pendingFlowEntry struct {
	pos   [3]int
	fluid string
}

// isFluidPassable returns true if a block at the given state can be replaced by fluid.
func isFluidPassable(stateID int) bool {
	if stateID == 0 {
		return true // air
	}
	if stateID < 0 || stateID >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}
	// Check for replaceable blocks (tall grass, etc.)
	name := block.StateList[stateID].ID()
	switch name {
	case "minecraft:grass", "minecraft:tall_grass", "minecraft:fern", "minecraft:dead_bush":
		return true
	}
	return false
}

func (f *FluidManager) processFlow(pos [3]int, sourceFluid string) {
	// Check if the target position is passable
	stateID, err := f.World.GetBlock(pos[0], pos[1], pos[2])
	if err != nil {
		return
	}

	// Check for fluid interaction (water meets lava or vice versa)
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		switch b := block.StateList[stateID].(type) {
		case block.Water:
			if sourceFluid == "lava" {
				f.fluidInteraction(pos[0], pos[1], pos[2], "lava_into_water", int(b.Level))
				return
			}
			return // already water
		case block.Lava:
			if sourceFluid == "water" {
				f.fluidInteraction(pos[0], pos[1], pos[2], "water_into_lava", int(b.Level))
				return
			}
			return // already lava
		}
	}

	if !isFluidPassable(int(stateID)) {
		return // solid block, can't flow here
	}

	// Check if fluid is flowing from above (downward flow)
	aboveState, err := f.World.GetBlock(pos[0], pos[1]+1, pos[2])
	if err == nil && int(aboveState) < len(block.StateList) && block.StateList[aboveState] != nil {
		switch block.StateList[aboveState].(type) {
		case block.Water:
			if sourceFluid == "water" {
				// Downward flow: place level 1 falling water (full column visually)
				flowBlock := block.Water{Level: block.Integer(1)}
				flowStateID, ok := block.ToStateID[flowBlock]
				if !ok {
					return
				}
				f.World.SetBlock(pos[0], pos[1], pos[2], flowStateID)
				broadcastBlockUpdateDirect(f.Manager, pos[0], pos[1], pos[2], int32(flowStateID))
				f.scheduleFlowFrom(pos[0], pos[1], pos[2], "water")
				return
			}
		case block.Lava:
			if sourceFluid == "lava" {
				flowBlock := block.Lava{Level: block.Integer(1)}
				flowStateID, ok := block.ToStateID[flowBlock]
				if !ok {
					return
				}
				f.World.SetBlock(pos[0], pos[1], pos[2], flowStateID)
				broadcastBlockUpdateDirect(f.Manager, pos[0], pos[1], pos[2], int32(flowStateID))
				f.scheduleFlowFrom(pos[0], pos[1], pos[2], "lava")
				return
			}
		}
	}

	// Find the lowest-level adjacent fluid source (horizontal neighbors only)
	bestLevel := -1
	bestFluid := ""
	for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		adjX, adjY, adjZ := pos[0]+offset[0], pos[1]+offset[1], pos[2]+offset[2]
		adjState, err := f.World.GetBlock(adjX, adjY, adjZ)
		if err != nil {
			continue
		}
		if int(adjState) >= len(block.StateList) || block.StateList[adjState] == nil {
			continue
		}

		switch b := block.StateList[adjState].(type) {
		case block.Water:
			level := int(b.Level)
			if level < 7 && (bestLevel == -1 || level < bestLevel) {
				bestLevel = level
				bestFluid = "water"
			}
		case block.Lava:
			level := int(b.Level)
			if level < 7 && (bestLevel == -1 || level < bestLevel) {
				bestLevel = level
				bestFluid = "lava"
			}
		}
	}

	if bestLevel < 0 || bestLevel >= 7 {
		return // no adjacent fluid or max level reached
	}

	newLevel := bestLevel + 1
	if newLevel > 7 {
		return
	}

	// Place flowing fluid block
	var flowBlock block.Block
	switch bestFluid {
	case "water":
		flowBlock = block.Water{Level: block.Integer(newLevel)}
	case "lava":
		flowBlock = block.Lava{Level: block.Integer(newLevel)}
	default:
		return
	}

	flowStateID, ok := block.ToStateID[flowBlock]
	if !ok {
		return
	}

	f.World.SetBlock(pos[0], pos[1], pos[2], flowStateID)
	broadcastBlockUpdateDirect(f.Manager, pos[0], pos[1], pos[2], int32(flowStateID))

	// Schedule further flow (horizontal + downward)
	if newLevel < 7 {
		f.scheduleFlowFrom(pos[0], pos[1], pos[2], bestFluid)
	} else {
		// Even at max horizontal level, try to flow downward
		f.scheduleDownward(pos[0], pos[1], pos[2], bestFluid)
	}
}

// scheduleFlowFrom schedules horizontal and downward flow from a fluid block.
func (f *FluidManager) scheduleFlowFrom(x, y, z int, fluid string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delay := int64(5)
	if fluid == "lava" {
		delay = 30
	}

	// Schedule downward first (faster)
	downPos := [3]int{x, y - 1, z}
	if _, exists := f.pending[downPos]; !exists {
		f.pending[downPos] = pendingFlow{fluid: fluid, tick: delay / 2}
	}

	// Schedule horizontal
	for _, offset := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		nextPos := [3]int{x + offset[0], y + offset[1], z + offset[2]}
		if _, exists := f.pending[nextPos]; !exists {
			f.pending[nextPos] = pendingFlow{fluid: fluid, tick: delay}
		}
	}
}

// scheduleDownward schedules only downward flow from a fluid block.
func (f *FluidManager) scheduleDownward(x, y, z int, fluid string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delay := int64(2)
	if fluid == "lava" {
		delay = 15
	}
	downPos := [3]int{x, y - 1, z}
	if _, exists := f.pending[downPos]; !exists {
		f.pending[downPos] = pendingFlow{fluid: fluid, tick: delay}
	}
}

// fluidInteraction handles water-lava and lava-water interactions.
func (f *FluidManager) fluidInteraction(x, y, z int, interactionType string, existingLevel int) {
	var resultBlock block.Block

	switch interactionType {
	case "water_into_lava":
		if existingLevel == 0 {
			// Water flows into lava source → obsidian
			resultBlock = block.Obsidian{}
		} else {
			// Water flows into flowing lava → cobblestone
			resultBlock = block.Cobblestone{}
		}
	case "lava_into_water":
		// Lava flows into water → stone
		resultBlock = block.Stone{}
	default:
		return
	}

	resultStateID, ok := block.ToStateID[resultBlock]
	if !ok {
		return
	}

	f.World.SetBlock(x, y, z, resultStateID)
	broadcastBlockUpdateDirect(f.Manager, x, y, z, int32(resultStateID))

	// Play extinguish sound
	BroadcastSound(f.Manager, SoundLavaExtinguish, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 0.5, 2.6)
}

// SoundLavaExtinguish is the sound ID for lava extinguishing.
const SoundLavaExtinguish int32 = 167 // block.lava.extinguish

// broadcastBlockUpdateDirect sends ClientboundBlockUpdate to all players.
func broadcastBlockUpdateDirect(manager *game.PlayerManager, x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
