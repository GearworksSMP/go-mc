package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// Sound ID for fire ambient / fire extinguish.
const (
	SoundFireExtinguish int32 = 418 // block.fire.extinguish
)

// FireManager manages fire blocks in the world.
type FireManager struct {
	Manager    *game.PlayerManager
	World      game.World
	Survival   *SurvivalHandler
	Rules      *GameRules
	WeatherMgr *WeatherManager
	Logger     *log.Logger
	mu         sync.Mutex
	fires      map[[3]int]int64 // position -> tick when placed
}

// NewFireManager creates a new FireManager.
func NewFireManager(manager *game.PlayerManager, world game.World, survival *SurvivalHandler, logger *log.Logger) *FireManager {
	return &FireManager{
		Manager:  manager,
		World:    world,
		Survival: survival,
		Logger:   logger,
		fires:    make(map[[3]int]int64),
	}
}

// PlaceFire places a fire block at the given position.
func (fm *FireManager) PlaceFire(x, y, z int, tick int64) {
	// Only place fire on top of a solid block (check block below)
	belowState, err := fm.World.GetBlock(x, y-1, z)
	if err != nil || belowState == 0 {
		return // no solid block below
	}

	// Don't place fire where there's already a block
	currentState, err := fm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if currentState != 0 {
		// Check if it's already fire
		currentName := BlockNameFromState(int(currentState))
		if currentName == "fire" || currentName == "soul_fire" {
			return // already fire
		}
		return // occupied by another block
	}

	// Place fire block (age=0, no connections)
	fireBlock := block.Fire{
		Age:   block.Integer(0),
		East:  block.Boolean(false),
		North: block.Boolean(false),
		South: block.Boolean(false),
		Up:    block.Boolean(false),
		West:  block.Boolean(false),
	}
	stateID, ok := block.ToStateID[fireBlock]
	if !ok {
		fm.logf("Fire: could not find fire block state")
		return
	}

	fm.World.SetBlock(x, y, z, stateID)
	broadcastBlockUpdateDirect(fm.Manager, x, y, z, int32(stateID))

	fm.mu.Lock()
	fm.fires[[3]int{x, y, z}] = tick
	fm.mu.Unlock()

	fm.logf("Fire placed at (%d, %d, %d)", x, y, z)
}

// ExtinguishFire removes a fire block at the given position.
func (fm *FireManager) ExtinguishFire(x, y, z int) {
	fm.mu.Lock()
	_, exists := fm.fires[[3]int{x, y, z}]
	if exists {
		delete(fm.fires, [3]int{x, y, z})
	}
	fm.mu.Unlock()

	// Also set block to air (in case fire was not tracked)
	state, err := fm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	blockName := BlockNameFromState(int(state))
	if blockName == "fire" || blockName == "soul_fire" {
		fm.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(fm.Manager, x, y, z, 0)
		BroadcastSound(fm.Manager, SoundFireExtinguish, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)
	}
}

// Tick processes fire spread, burn-out, and fire damage.
func (fm *FireManager) Tick(tick int64) {
	// Fire damage: every tick, check for players standing in fire
	if tick%20 == 0 { // damage once per second
		fm.mu.Lock()
		firePositions := make([][3]int, 0, len(fm.fires))
		for pos := range fm.fires {
			firePositions = append(firePositions, pos)
		}
		fm.mu.Unlock()

		for _, pos := range firePositions {
			fm.Manager.ForEach(func(p *game.Player) {
				if p.Dead || p.IsInvulnerable() {
					return
				}
				px, py, pz := p.Position()
				dx := px - (float64(pos[0]) + 0.5)
				dy := py - float64(pos[1])
				dz := pz - (float64(pos[2]) + 0.5)
				// Player hitbox is roughly 0.6 wide, 1.8 tall
				if math.Abs(dx) < 0.8 && dy >= -0.1 && dy < 1.8 && math.Abs(dz) < 0.8 {
					p.LastDamageMessage = p.Name + " burned to death"
					if fm.Survival != nil {
						fm.Survival.ApplyDamage(fm.Manager, p, 1, fm.Survival.AttackDamageTypeID)
					}
				}
			})
		}
	}

	// Rain extinguishes exposed fires every 20 ticks
	if tick%20 == 0 && fm.WeatherMgr != nil && fm.WeatherMgr.State() >= WeatherRain {
		fm.mu.Lock()
		var rainExtinguish [][3]int
		for pos := range fm.fires {
			if fm.isExposedToSky(pos[0], pos[1], pos[2]) {
				rainExtinguish = append(rainExtinguish, pos)
			}
		}
		for _, pos := range rainExtinguish {
			delete(fm.fires, pos)
			state, err := fm.World.GetBlock(pos[0], pos[1], pos[2])
			if err == nil {
				name := BlockNameFromState(int(state))
				if name == "fire" || name == "soul_fire" {
					fm.World.SetBlock(pos[0], pos[1], pos[2], 0)
					broadcastBlockUpdateDirect(fm.Manager, pos[0], pos[1], pos[2], 0)
					BroadcastSound(fm.Manager, SoundFireExtinguish, SoundCategoryBlock,
						float64(pos[0])+0.5, float64(pos[1])+0.5, float64(pos[2])+0.5, 1.0, 1.0)
				}
			}
		}
		fm.mu.Unlock()
	}

	// Fire spread and burn-out: every 30 ticks (1.5 seconds)
	// Skip fire spread if doFireTick is false
	if tick%30 != 0 || (fm.Rules != nil && !fm.Rules.GetDoFireTick()) {
		return
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	var toExtinguish [][3]int
	var toSpread [][3]int

	for pos, placedTick := range fm.fires {
		age := tick - placedTick

		// Check if the block below is netherrack (eternal fire)
		belowState, err := fm.World.GetBlock(pos[0], pos[1]-1, pos[2])
		isEternal := false
		if err == nil {
			belowName := BlockNameFromState(int(belowState))
			if belowName == "netherrack" || belowName == "magma_block" {
				isEternal = true
			}
		}

		// Fire burns out after 200-400 ticks unless on netherrack
		if !isEternal && age > int64(200+rand.Intn(200)) {
			toExtinguish = append(toExtinguish, pos)
			continue
		}

		// Verify fire block still exists
		currentState, err := fm.World.GetBlock(pos[0], pos[1], pos[2])
		if err != nil {
			toExtinguish = append(toExtinguish, pos)
			continue
		}
		currentName := BlockNameFromState(int(currentState))
		if currentName != "fire" && currentName != "soul_fire" {
			// Fire was removed externally (e.g. by water)
			toExtinguish = append(toExtinguish, pos)
			continue
		}

		// If the block below was removed, extinguish
		if belowState == 0 && !isEternal {
			toExtinguish = append(toExtinguish, pos)
			continue
		}

		// Check adjacent blocks for flammable materials, attempt to spread
		adjOffsets := [][3]int{
			{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1},
			{0, 1, 0}, {0, -1, 0},
		}
		for _, off := range adjOffsets {
			adjX := pos[0] + off[0]
			adjY := pos[1] + off[1]
			adjZ := pos[2] + off[2]

			adjState, err := fm.World.GetBlock(adjX, adjY, adjZ)
			if err != nil || adjState == 0 {
				continue
			}

			adjName := BlockNameFromState(int(adjState))
			if !isFlammable(adjName) {
				continue
			}

			// 20% chance to spread fire on top of the adjacent block
			if rand.Float64() < 0.20 {
				// Place fire on top of the adjacent flammable block
				fireX := adjX
				fireY := adjY + 1
				fireZ := adjZ
				// Check that the space above is air
				aboveState, err := fm.World.GetBlock(fireX, fireY, fireZ)
				if err == nil && aboveState == 0 {
					toSpread = append(toSpread, [3]int{fireX, fireY, fireZ})
				}
			}
		}
	}

	// Extinguish fires
	for _, pos := range toExtinguish {
		delete(fm.fires, pos)
		// Set to air if still fire
		state, err := fm.World.GetBlock(pos[0], pos[1], pos[2])
		if err == nil {
			name := BlockNameFromState(int(state))
			if name == "fire" || name == "soul_fire" {
				fm.World.SetBlock(pos[0], pos[1], pos[2], 0)
				broadcastBlockUpdateDirect(fm.Manager, pos[0], pos[1], pos[2], 0)
			}
		}
	}

	// Spread fires
	for _, pos := range toSpread {
		// Avoid duplicate fires
		if _, exists := fm.fires[pos]; exists {
			continue
		}
		fireBlock := block.Fire{
			Age:   block.Integer(0),
			East:  block.Boolean(false),
			North: block.Boolean(false),
			South: block.Boolean(false),
			Up:    block.Boolean(false),
			West:  block.Boolean(false),
		}
		stateID, ok := block.ToStateID[fireBlock]
		if !ok {
			continue
		}
		fm.World.SetBlock(pos[0], pos[1], pos[2], stateID)
		broadcastBlockUpdateDirect(fm.Manager, pos[0], pos[1], pos[2], int32(stateID))
		fm.fires[pos] = tick
	}
}

// isExposedToSky checks if a position has no solid block above it up to y=320.
func (fm *FireManager) isExposedToSky(x, y, z int) bool {
	for checkY := y + 1; checkY <= 320; checkY++ {
		state, err := fm.World.GetBlock(x, checkY, z)
		if err != nil {
			return true // can't read above = assume sky
		}
		if state != 0 {
			return false
		}
	}
	return true
}

func (fm *FireManager) logf(format string, args ...any) {
	if fm.Logger != nil {
		fm.Logger.Printf(format, args...)
	}
}
