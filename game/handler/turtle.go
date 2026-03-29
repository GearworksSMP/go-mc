package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// TurtleManager handles turtle mob AI and turtle egg hatching.
type TurtleManager struct {
	MobMgr  *MobManager
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger

	mu   sync.Mutex
	eggs map[[3]int]int64
}

// NewTurtleManager creates a new TurtleManager.
func NewTurtleManager(mobMgr *MobManager, manager *game.PlayerManager, world game.World, logger *log.Logger) *TurtleManager {
	return &TurtleManager{
		MobMgr:  mobMgr,
		Manager: manager,
		World:   world,
		Logger:  logger,
		eggs:    make(map[[3]int]int64),
	}
}

// tickTurtle runs turtle AI for a single mob.
func (tm *TurtleManager) tickTurtle(mob *Mob, tick int64) {
	tm.MobMgr.applyGravity(mob)

	if mob.Baby && mob.BabyAge > 0 {
		mob.BabyAge--
		if mob.BabyAge <= 0 {
			mob.Baby = false
			mob.Speed /= 1.5
			var w MetadataWriter
			w.WriteBoolean(16, false)
			data := w.Bytes()
			tm.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
				SendEntityMetadata(p, mob.EID, data)
			})
			if tm.MobMgr.ItemEntities != nil {
				itemID := itemIDByName("turtle_scute")
				if itemID > 0 {
					tm.MobMgr.ItemEntities.SpawnItem(tm.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, 1, 10)
				}
			}
		}
	}

	if tm.isWaterAt(int(math.Floor(mob.X)), int(math.Floor(mob.Y)), int(math.Floor(mob.Z))) {
		tm.tickTurtleSwim(mob, tick)
		return
	}

	tm.tickTurtleLand(mob, tick)
}

// tickTurtleSwim handles turtle swimming behavior.
func (tm *TurtleManager) tickTurtleSwim(mob *Mob, tick int64) {
	if tick-mob.WanderTick > int64(60+rand.Intn(60)) {
		mob.WanderTick = tick
		mob.WanderYaw += (rand.Float32() - 0.5) * 90
	}
	rad := float64(mob.WanderYaw) * math.Pi / 180
	nx := math.Cos(rad) * mob.Speed * 0.5
	nz := math.Sin(rad) * mob.Speed * 0.5

	mob.X += nx
	mob.Z += nz
	mob.Yaw = mob.WanderYaw

	surfaceY := tm.findWaterSurface(int(math.Floor(mob.X)), int(math.Floor(mob.Y)), int(math.Floor(mob.Z)))
	if mob.Y < float64(surfaceY)-0.5 {
		mob.Y += 0.04
	} else if mob.Y > float64(surfaceY)+0.5 {
		mob.Y -= 0.02
	}

	tm.MobMgr.broadcastMobMove(mob)
}

// tickTurtleLand handles turtle behavior on land.
func (tm *TurtleManager) tickTurtleLand(mob *Mob, tick int64) {
	bx := int(math.Floor(mob.X))
	by := int(math.Floor(mob.Y))
	bz := int(math.Floor(mob.Z))
	onSand := tm.isSandAt(bx, by-1, bz)

	if onSand && !mob.Baby && mob.TurtleEggCooldown <= 0 && rand.Intn(1200) == 0 {
		tm.layEggs(mob, bx, by, bz, tick)
		mob.TurtleEggCooldown = 4800 // cooldown before next lay
	}

	if mob.TurtleEggCooldown > 0 {
		mob.TurtleEggCooldown--
	}

	if tick%100 == 0 {
		waterDir := tm.findNearbyWater(bx, by, bz, 16)
		if waterDir != nil {
			dx := float64(waterDir[0]) - mob.X
			dz := float64(waterDir[2]) - mob.Z
			mob.WanderYaw = float32(math.Atan2(dz, dx) * 180 / math.Pi)
		}
	}

	tm.MobMgr.tickWander(mob, tick)
}

// layEggs places a turtle_egg block at the turtle's feet.
func (tm *TurtleManager) layEggs(mob *Mob, bx, by, bz int, tick int64) {
	eggs := 1 + rand.Intn(4) // 1-4 eggs
	eggBlock := block.TurtleEgg{Eggs: block.Integer(eggs), Hatch: 0}
	stateID, ok := block.ToStateID[eggBlock]
	if !ok {
		return
	}
	_, err := tm.World.SetBlock(bx, by, bz, stateID)
	if err != nil {
		return
	}
	tm.mu.Lock()
	tm.eggs[[3]int{bx, by, bz}] = tick
	tm.mu.Unlock()

	broadcastBlockUpdateDirect(tm.Manager, bx, by, bz, int32(stateID))
	BroadcastSound(tm.Manager, SoundTurtleLayEgg, SoundCategoryNeutral, mob.X, mob.Y, mob.Z, 1.0, 1.0)

	if tm.Logger != nil {
		tm.Logger.Printf("Turtle laid %d eggs at [%d,%d,%d]", eggs, bx, by, bz)
	}
}

// TickTurtleEggs checks all placed turtle eggs and hatches them after the timer expires.
func (tm *TurtleManager) TickTurtleEggs(tick int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for pos, placedTick := range tm.eggs {
		elapsed := tick - placedTick
		if elapsed < 4800 {
			if elapsed == 1600 || elapsed == 3200 {
				tm.advanceHatchStage(pos)
			}
			continue
		}
		tm.hatchEggs(pos, tick)
		delete(tm.eggs, pos)
	}
}

// advanceHatchStage increments the hatch property of a turtle egg block.
func (tm *TurtleManager) advanceHatchStage(pos [3]int) {
	state, err := tm.World.GetBlock(pos[0], pos[1], pos[2])
	if err != nil {
		return
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return
	}
	egg, ok := block.StateList[state].(block.TurtleEgg)
	if !ok {
		return
	}
	if egg.Hatch < 2 {
		egg.Hatch++
		newState, ok := block.ToStateID[egg]
		if !ok {
			return
		}
		tm.World.SetBlock(pos[0], pos[1], pos[2], newState)
		broadcastBlockUpdateDirect(tm.Manager, pos[0], pos[1], pos[2], int32(newState))
	}
}

// hatchEggs removes the egg block and spawns baby turtles.
func (tm *TurtleManager) hatchEggs(pos [3]int, tick int64) {
	state, err := tm.World.GetBlock(pos[0], pos[1], pos[2])
	if err != nil {
		return
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return
	}
	egg, ok := block.StateList[state].(block.TurtleEgg)
	if !ok {
		return
	}
	numEggs := int(egg.Eggs)
	if numEggs < 1 {
		numEggs = 1
	}

	tm.World.SetBlock(pos[0], pos[1], pos[2], 0)
	broadcastBlockUpdateDirect(tm.Manager, pos[0], pos[1], pos[2], 0)

	// Spawn baby turtles
	for i := 0; i < numEggs; i++ {
		x := float64(pos[0]) + 0.5 + (rand.Float64()-0.5)*0.5
		y := float64(pos[1])
		z := float64(pos[2]) + 0.5 + (rand.Float64()-0.5)*0.5
		eid := tm.MobMgr.SpawnMobAt(MobTypeTurtle, x, y, z)
		// Make it a baby
		tm.MobMgr.mu.Lock()
		if mob, ok := tm.MobMgr.Mobs[eid]; ok {
			mob.Baby = true
			mob.BabyAge = 24000 // 20 minutes to grow
			mob.Speed *= 1.5    // baby speed boost
		}
		tm.MobMgr.mu.Unlock()

		// Send baby metadata
		var w MetadataWriter
		w.WriteBoolean(16, true)
		data := w.Bytes()
		tm.Manager.ForEachNearby(x, z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, eid, data)
		})
	}

	BroadcastSound(tm.Manager, SoundTurtleEggHatch, SoundCategoryNeutral,
		float64(pos[0])+0.5, float64(pos[1])+0.5, float64(pos[2])+0.5, 1.0, 1.0)

	if tm.Logger != nil {
		tm.Logger.Printf("Hatched %d baby turtles at [%d,%d,%d]", numEggs, pos[0], pos[1], pos[2])
	}
}

// RegisterEgg tracks a turtle egg placed by other means (e.g., player placing).
func (tm *TurtleManager) RegisterEgg(x, y, z int, tick int64) {
	tm.mu.Lock()
	tm.eggs[[3]int{x, y, z}] = tick
	tm.mu.Unlock()
}

// UnregisterEgg removes a tracked turtle egg (e.g., when broken).
func (tm *TurtleManager) UnregisterEgg(x, y, z int) {
	tm.mu.Lock()
	delete(tm.eggs, [3]int{x, y, z})
	tm.mu.Unlock()
}

// isWaterAt checks if the block at the given position is water.
func (tm *TurtleManager) isWaterAt(x, y, z int) bool {
	state, err := tm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) < len(block.StateList) && block.StateList[state] != nil {
		_, ok := block.StateList[state].(block.Water)
		return ok
	}
	return false
}

// isSandAt checks if the block at the given position is sand.
func (tm *TurtleManager) isSandAt(x, y, z int) bool {
	state, err := tm.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(state) < len(block.StateList) && block.StateList[state] != nil {
		_, ok := block.StateList[state].(block.Sand)
		return ok
	}
	return false
}

// findWaterSurface returns the Y coordinate of the water surface above the given position.
func (tm *TurtleManager) findWaterSurface(x, y, z int) int {
	for cy := y; cy < y+16; cy++ {
		if !tm.isWaterAt(x, cy, z) {
			return cy
		}
	}
	return y + 1
}

// findNearbyWater searches for a water block within range and returns its position, or nil.
func (tm *TurtleManager) findNearbyWater(cx, cy, cz, radius int) *[3]int {
	// Sample random positions within radius
	for i := 0; i < 10; i++ {
		dx := rand.Intn(radius*2+1) - radius
		dz := rand.Intn(radius*2+1) - radius
		x, z := cx+dx, cz+dz
		for dy := -2; dy <= 2; dy++ {
			if tm.isWaterAt(x, cy+dy, z) {
				pos := [3]int{x, cy + dy, z}
				return &pos
			}
		}
	}
	return nil
}
