package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

// Vibration event types. The value doubles as the redstone signal strength (1-15).
const (
	VibrationStep           int32 = 1
	VibrationFlap           int32 = 2
	VibrationSwim           int32 = 3
	VibrationElytraGlide    int32 = 4
	VibrationHitGround      int32 = 5
	VibrationTeleport       int32 = 6
	VibrationSplash         int32 = 7
	VibrationBlockPlace     int32 = 8
	VibrationBlockDestroy   int32 = 9
	VibrationFluidPlace     int32 = 10
	VibrationFluidPickup    int32 = 11
	VibrationProjectileLand int32 = 12
	VibrationEntityDamage   int32 = 13
	VibrationEquip          int32 = 14
	VibrationEntityDie      int32 = 15
)

// blockPos is a packed int64 for block positions used as map keys.
type blockPos int64

func packBlockPos(x, y, z int) blockPos {
	return blockPos(int64(x)&0x3FFFFFF | (int64(z)&0x3FFFFFF)<<26 | (int64(y)&0xFFF)<<52)
}

func unpackBlockPos(p blockPos) (x, y, z int) {
	v := int64(p)
	x = int(int32(v<<38) >> 38)
	z = int(int32((v>>26)<<38) >> 38)
	y = int(int16(v>>52) << 4 >> 4)
	return
}

type sculkSensorState struct {
	lastActivation int64
	cooldownEnd    int64
}

type sculkShriekerState struct {
	playerWarnings map[int32]int32
}

type sculkCatalystState struct{}

// VibrationEvent represents a vibration emitted at a world position.
type VibrationEvent struct {
	X, Y, Z   int
	EventType int32
	SourceEID int32 // -1 if no entity source
	XP        int   // XP dropped (only relevant for EntityDie events)
}

// SculkManager tracks sculk block positions and handles vibration mechanics.
type SculkManager struct {
	Manager   *game.PlayerManager
	World     game.World
	MobMgr   *MobManager
	EffectMgr *EffectManager

	mu        sync.Mutex
	sensors   map[blockPos]*sculkSensorState
	shriekers map[blockPos]*sculkShriekerState
	catalysts map[blockPos]*sculkCatalystState

	pendingVibrations []VibrationEvent

	// Cached block state IDs (computed once at init).
	sculkID         level.BlocksState
	sculkVeinID     level.BlocksState
	stoneID         level.BlocksState
	deepslateID     level.BlocksState
	dirtID          level.BlocksState
	calibSensorBase level.BlocksState
	sensorBase      level.BlocksState
	bloomCatalystID level.BlocksState
}

func NewSculkManager(manager *game.PlayerManager, world game.World) *SculkManager {
	sculkID, _ := block.ToStateID[block.Sculk{}]
	sculkVeinID, _ := block.ToStateID[block.SculkVein{Down: true}]
	stoneID, _ := block.ToStateID[block.Stone{}]
	deepslateID, _ := block.ToStateID[block.Deepslate{Axis: block.Y}]
	dirtID, _ := block.ToStateID[block.Dirt{}]
	calibBase, _ := block.ToStateID[block.CalibratedSculkSensor{}]
	sensorBase, _ := block.ToStateID[block.SculkSensor{}]
	bloomID, _ := block.ToStateID[block.SculkCatalyst{Bloom: true}]
	return &SculkManager{
		Manager:         manager,
		World:           world,
		sensors:         make(map[blockPos]*sculkSensorState),
		shriekers:       make(map[blockPos]*sculkShriekerState),
		catalysts:       make(map[blockPos]*sculkCatalystState),
		sculkID:         sculkID,
		sculkVeinID:     sculkVeinID,
		stoneID:         stoneID,
		deepslateID:     deepslateID,
		dirtID:          dirtID,
		calibSensorBase: calibBase,
		sensorBase:      sensorBase,
		bloomCatalystID: bloomID,
	}
}

// RegisterSensor registers a sculk sensor at the given position.
func (s *SculkManager) RegisterSensor(x, y, z int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := packBlockPos(x, y, z)
	if _, ok := s.sensors[pos]; !ok {
		s.sensors[pos] = &sculkSensorState{}
	}
}

// RegisterShrieker registers a sculk shrieker at the given position.
func (s *SculkManager) RegisterShrieker(x, y, z int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := packBlockPos(x, y, z)
	if _, ok := s.shriekers[pos]; !ok {
		s.shriekers[pos] = &sculkShriekerState{
			playerWarnings: make(map[int32]int32),
		}
	}
}

// RegisterCatalyst registers a sculk catalyst at the given position.
func (s *SculkManager) RegisterCatalyst(x, y, z int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := packBlockPos(x, y, z)
	if _, ok := s.catalysts[pos]; !ok {
		s.catalysts[pos] = &sculkCatalystState{}
	}
}

// UnregisterBlock removes any sculk block tracking at the given position.
func (s *SculkManager) UnregisterBlock(x, y, z int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := packBlockPos(x, y, z)
	delete(s.sensors, pos)
	delete(s.shriekers, pos)
	delete(s.catalysts, pos)
}

// EmitVibration queues a vibration event for processing on the next tick.
func (s *SculkManager) EmitVibration(x, y, z int, eventType int32, sourceEID int32, xp int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingVibrations = append(s.pendingVibrations, VibrationEvent{
		X: x, Y: y, Z: z,
		EventType: eventType,
		SourceEID: sourceEID,
		XP:        xp,
	})
}

// Tick processes all pending vibration events and updates sculk block states.
func (s *SculkManager) Tick(tick int64) {
	s.mu.Lock()
	events := s.pendingVibrations
	s.pendingVibrations = nil
	s.mu.Unlock()

	for _, ev := range events {
		s.processSensors(ev, tick)
		s.processShriekers(ev)
		s.processCatalysts(ev)
		s.notifyWardens(ev)
	}

	s.tickSensorCooldowns(tick)
}

func (s *SculkManager) processSensors(ev VibrationEvent, tick int64) {
	strength := ev.EventType
	if strength < 1 || strength > 15 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for pos, state := range s.sensors {
		if tick < state.cooldownEnd {
			continue
		}

		sx, sy, sz := unpackBlockPos(pos)
		if distSq3(float64(ev.X-sx), float64(ev.Y-sy), float64(ev.Z-sz)) > 64.0 { // 8^2
			continue
		}

		blk, err := s.World.GetBlock(sx, sy, sz)
		if err != nil {
			continue
		}
		if s.isCalibratedSensor(blk) && !s.calibratedSensorAccepts(blk, strength) {
			continue
		}

		state.lastActivation = tick
		state.cooldownEnd = tick + 40
		s.updateSensorBlockState(sx, sy, sz, strength, block.SculkSensorPhaseActive)
	}
}

// distSq3 returns the squared distance for 3D deltas.
func distSq3(dx, dy, dz float64) float64 {
	return dx*dx + dy*dy + dz*dz
}

func (s *SculkManager) isCalibratedSensor(stateID level.BlocksState) bool {
	// CalibratedSculkSensor: facing(4) * power(16) * phase(3) * waterlogged(2) = 384 states
	return stateID >= s.calibSensorBase && stateID < s.calibSensorBase+384
}

// calibratedSensorAccepts returns true if the sensor's current power matches the vibration strength.
// In vanilla the input signal comes from a comparator; we approximate using the stored power.
func (s *SculkManager) calibratedSensorAccepts(stateID level.BlocksState, vibStrength int32) bool {
	offset := int32(stateID - s.calibSensorBase)
	// State ordering: facing(4) * [power(16) * [phase(3) * waterlogged(2)]]
	power := (offset / 6) % 16
	return power == 0 || vibStrength == power
}

func (s *SculkManager) updateSensorBlockState(x, y, z int, power int32, phase block.SculkSensorPhase) {
	curState, err := s.World.GetBlock(x, y, z)
	if err != nil {
		return
	}

	var newState level.BlocksState
	if s.isCalibratedSensor(curState) {
		offset := int32(curState - s.calibSensorBase)
		facing := offset / (16 * 3 * 2)
		waterlogged := offset % 2
		newState = s.calibSensorBase + level.BlocksState(facing*(16*3*2)+power*(3*2)+int32(phase)*2+waterlogged)
	} else {
		offset := int32(curState - s.sensorBase)
		waterlogged := offset % 2
		newState = s.sensorBase + level.BlocksState(power*(3*2)+int32(phase)*2+waterlogged)
	}

	s.World.SetBlock(x, y, z, newState)
	broadcastBlockUpdateDirect(s.Manager, x, y, z, int32(newState))
}

func (s *SculkManager) tickSensorCooldowns(tick int64) {
	s.mu.Lock()

	type cooldownUpdate struct {
		x, y, z int
		phase   block.SculkSensorPhase
		reset   bool // reset lastActivation to 0
	}
	var updates []cooldownUpdate

	for pos, state := range s.sensors {
		if state.lastActivation == 0 {
			continue
		}
		d := tick - state.lastActivation
		if d == 30 {
			sx, sy, sz := unpackBlockPos(pos)
			updates = append(updates, cooldownUpdate{sx, sy, sz, block.SculkSensorPhaseCooldown, false})
		} else if d >= 40 {
			sx, sy, sz := unpackBlockPos(pos)
			updates = append(updates, cooldownUpdate{sx, sy, sz, block.SculkSensorPhaseInactive, true})
			state.lastActivation = 0
		}
	}

	s.mu.Unlock()

	for _, u := range updates {
		s.updateSensorBlockState(u.x, u.y, u.z, 0, u.phase)
	}
}

func (s *SculkManager) processShriekers(ev VibrationEvent) {
	if ev.SourceEID < 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for pos, state := range s.shriekers {
		sx, sy, sz := unpackBlockPos(pos)
		if distSq3(float64(ev.X-sx), float64(ev.Y-sy), float64(ev.Z-sz)) > 64.0 {
			continue
		}

		state.playerWarnings[ev.SourceEID]++
		warnings := state.playerWarnings[ev.SourceEID]

		if s.EffectMgr != nil {
			s.Manager.ForEach(func(p *game.Player) {
				if p.Dead || p.GameMode != 0 {
					return
				}
				px, py, pz := p.Position()
				if distSq3(px-float64(sx), py-float64(sy), pz-float64(sz)) <= 1600.0 { // 40^2
					s.EffectMgr.ApplyEffect(p, EffectDarkness, 0, 260, true)
				}
			})
		}

		BroadcastSound(s.Manager, SoundSculkShriekerShriek, SoundCategoryBlock,
			float64(sx)+0.5, float64(sy)+0.5, float64(sz)+0.5, 1.0, 1.0)

		if warnings >= 4 && s.MobMgr != nil {
			state.playerWarnings[ev.SourceEID] = 0

			spawnX := float64(sx) + float64(rand.Intn(11)-5) + 0.5
			spawnY := float64(sy)
			spawnZ := float64(sz) + float64(rand.Intn(11)-5) + 0.5

			s.MobMgr.SpawnMobAt(MobTypeWarden, spawnX, spawnY, spawnZ)
			BroadcastSound(s.Manager, SoundWardenEmerge, SoundCategoryHostile,
				spawnX, spawnY, spawnZ, 1.0, 1.0)
		}
	}
}

func (s *SculkManager) processCatalysts(ev VibrationEvent) {
	if ev.EventType != VibrationEntityDie || ev.XP <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for pos := range s.catalysts {
		cx, cy, cz := unpackBlockPos(pos)
		if distSq3(float64(ev.X-cx), float64(ev.Y-cy), float64(ev.Z-cz)) > 64.0 {
			continue
		}

		radius := ev.XP / 2
		if radius < 2 {
			radius = 2
		}
		if radius > 8 {
			radius = 8
		}

		s.spreadSculk(ev.X, ev.Y, ev.Z, radius)

		s.World.SetBlock(cx, cy, cz, s.bloomCatalystID)
		broadcastBlockUpdateDirect(s.Manager, cx, cy, cz, int32(s.bloomCatalystID))
		break // only one catalyst per death event
	}
}

func (s *SculkManager) spreadSculk(cx, cy, cz, radius int) {
	radiusSq := float64(radius * radius)

	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				if float64(dx*dx+dy*dy+dz*dz) > radiusSq {
					continue
				}

				x, y, z := cx+dx, cy+dy, cz+dz
				blk, err := s.World.GetBlock(x, y, z)
				if err != nil {
					continue
				}

				if blk != s.stoneID && blk != s.deepslateID && blk != s.dirtID {
					continue
				}

				// 70% sculk block, 30% sculk vein if adjacent to air.
				if rand.Intn(10) < 7 {
					s.World.SetBlock(x, y, z, s.sculkID)
					broadcastBlockUpdateDirect(s.Manager, x, y, z, int32(s.sculkID))
				} else {
					newBlock := s.sculkID
					for _, off := range [6][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
						adj, err := s.World.GetBlock(x+off[0], y+off[1], z+off[2])
						if err == nil && adj == 0 {
							newBlock = s.sculkVeinID
							break
						}
					}
					s.World.SetBlock(x, y, z, newBlock)
					broadcastBlockUpdateDirect(s.Manager, x, y, z, int32(newBlock))
				}
			}
		}
	}
}

// notifyWardens feeds vibration events to nearby wardens for anger accumulation.
func (s *SculkManager) notifyWardens(ev VibrationEvent) {
	if s.MobMgr == nil || ev.SourceEID < 0 {
		return
	}

	s.MobMgr.mu.Lock()
	defer s.MobMgr.mu.Unlock()

	angerAdd := ev.EventType * 5
	for _, mob := range s.MobMgr.Mobs {
		if mob.TypeID != MobTypeWarden {
			continue
		}
		if distSq3(float64(ev.X)-mob.X, float64(ev.Y)-mob.Y, float64(ev.Z)-mob.Z) > 256.0 { // 16^2
			continue
		}

		if mob.WardenPlayerAnger == nil {
			mob.WardenPlayerAnger = make(map[int32]int32)
			mob.WardenPrevPos = make(map[int32][3]float64)
		}
		mob.WardenPlayerAnger[ev.SourceEID] += angerAdd
	}
}

const (
	SoundSculkShriekerShriek int32 = 901  // block.sculk_shrieker.shriek
	SoundWardenEmerge        int32 = 1112 // entity.warden.emerge
)
