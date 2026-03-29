package handler

import (
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Raid state constants.
const (
	raidStateSpawning = iota
	raidStateActive
	raidStateVictory
	raidStateDefeat
)

// Raid represents an active raid triggered by Bad Omen.
type Raid struct {
	ID               int
	CenterX, CenterZ float64
	Wave             int
	MaxWaves         int
	WaveMobs         []int32
	State            int
	StartTick        int64
	PlayerUUID       uuid.UUID
}

// RaidManager manages raid lifecycle and wave spawning.
type RaidManager struct {
	Manager   *game.PlayerManager
	MobMgr    *MobManager
	World     game.World
	EffectMgr *EffectManager
	Logger    *log.Logger
	mu        sync.Mutex
	raids     map[int]*Raid
	nextID    int
}

// NewRaidManager creates a new RaidManager.
func NewRaidManager(manager *game.PlayerManager, mobMgr *MobManager, world game.World, effectMgr *EffectManager, logger *log.Logger) *RaidManager {
	return &RaidManager{
		Manager:   manager,
		MobMgr:    mobMgr,
		World:     world,
		EffectMgr: effectMgr,
		Logger:    logger,
		raids:     make(map[int]*Raid),
	}
}

// Tick checks for raid triggers and advances active raids.
func (rm *RaidManager) Tick(tick int64) {
	if tick%100 != 0 {
		return
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.Manager.ForEach(func(p *game.Player) {
		if p.BadOmen <= 0 || p.Dead {
			return
		}
		px, py, pz := p.Position()
		if rm.hasVillageNearby(px, int(py), pz) {
			rm.startRaid(p, px, pz, tick)
		}
	})

	for id, raid := range rm.raids {
		switch raid.State {
		case raidStateSpawning:
			rm.spawnWave(raid)
			raid.State = raidStateActive
		case raidStateActive:
			alive := 0
			for _, eid := range raid.WaveMobs {
				if rm.MobMgr.IsMobAlive(eid) {
					alive++
				}
			}
			if alive == 0 {
				raid.Wave++
				if raid.Wave > raid.MaxWaves {
					raid.State = raidStateVictory
				} else {
					raid.State = raidStateSpawning
				}
			}
			if tick-raid.StartTick > 12000 {
				raid.State = raidStateDefeat
			}
		case raidStateVictory:
			rm.onVictory(raid)
			delete(rm.raids, id)
		case raidStateDefeat:
			if rm.Logger != nil {
				rm.Logger.Printf("Raid %d defeated (timed out)", raid.ID)
			}
			rm.broadcastMessage("Raid defeated...", "red", raid.CenterX, raid.CenterZ)
			delete(rm.raids, id)
		}
	}
}

// hasVillageNearby scans for bed or bell blocks near the player.
func (rm *RaidManager) hasVillageNearby(px float64, baseY int, pz float64) bool {
	cx, cz := int(px), int(pz)
	for dx := -32; dx <= 32; dx += 4 {
		for dz := -32; dz <= 32; dz += 4 {
			for dy := -10; dy <= 10; dy += 2 {
				state, err := rm.World.GetBlock(cx+dx, baseY+dy, cz+dz)
				if err != nil {
					continue
				}
				name := BlockNameFromState(int(state))
				if strings.Contains(name, "bed") || name == "bell" {
					return true
				}
			}
		}
	}
	return false
}

func (rm *RaidManager) startRaid(p *game.Player, cx, cz float64, tick int64) {
	maxWaves := int(p.BadOmen)
	if maxWaves > 7 {
		maxWaves = 7
	}

	p.BadOmen = 0
	if rm.EffectMgr != nil {
		rm.EffectMgr.RemoveEffect(p, EffectBadOmen)
	}

	rm.nextID++
	raid := &Raid{
		ID:         rm.nextID,
		CenterX:    cx,
		CenterZ:    cz,
		Wave:       1,
		MaxWaves:   maxWaves,
		State:      raidStateSpawning,
		StartTick:  tick,
		PlayerUUID: p.UUID,
	}
	rm.raids[raid.ID] = raid

	if rm.Logger != nil {
		rm.Logger.Printf("Raid %d started at (%.0f, %.0f) with %d waves for %s", raid.ID, cx, cz, maxWaves, p.Name)
	}
	rm.broadcastMessage("Raid starting!", "dark_red", cx, cz)
}

type mobSpec struct {
	typeID int32
	health float32
	damage float32
	speed  float64
}

var (
	specPillager   = mobSpec{MobTypePillager, 24, 4, 0.1}
	specVindicator = mobSpec{MobTypeVindicator, 24, 8, 0.12}
	specRavager    = mobSpec{MobTypeRavager, 100, 12, 0.15}
	specEvoker     = mobSpec{MobTypeEvoker, 24, 6, 0.1}
	specWitch      = mobSpec{MobTypeWitch, 26, 3, 0.1}
)

func repeatSpec(s mobSpec, n int) []mobSpec {
	out := make([]mobSpec, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func waveComposition(wave int) []mobSpec {
	switch wave {
	case 1:
		return repeatSpec(specPillager, 4)
	case 2:
		return append(repeatSpec(specPillager, 3), repeatSpec(specVindicator, 2)...)
	case 3:
		w := append(repeatSpec(specPillager, 4), repeatSpec(specVindicator, 2)...)
		return append(w, specRavager)
	case 4:
		w := append(repeatSpec(specPillager, 4), repeatSpec(specVindicator, 3)...)
		return append(w, specRavager, specWitch)
	default:
		w := append(repeatSpec(specPillager, 5), repeatSpec(specVindicator, 3)...)
		w = append(w, repeatSpec(specRavager, 2)...)
		return append(w, specEvoker, specWitch, specWitch)
	}
}

func (rm *RaidManager) spawnWave(raid *Raid) {
	specs := waveComposition(raid.Wave)
	raid.WaveMobs = make([]int32, 0, len(specs))

	for _, ms := range specs {
		angle := rand.Float64() * 2 * math.Pi
		dist := 48 + rand.Float64()*16
		sx := raid.CenterX + math.Cos(angle)*dist
		sz := raid.CenterZ + math.Sin(angle)*dist
		sy := float64(rm.MobMgr.findSurfaceY(int(sx), int(sz)))
		if sy < float64(rm.MobMgr.MinY) {
			sy = 64
		}

		eid := rm.MobMgr.SpawnMobAt(ms.typeID, sx+0.5, sy, sz+0.5)
		raid.WaveMobs = append(raid.WaveMobs, eid)
	}

	if rm.Logger != nil {
		rm.Logger.Printf("Raid %d wave %d/%d: spawned %d mobs", raid.ID, raid.Wave, raid.MaxWaves, len(specs))
	}
	rm.broadcastMessage("Wave "+strconv.Itoa(raid.Wave)+"!", "gold", raid.CenterX, raid.CenterZ)
}

func (rm *RaidManager) onVictory(raid *Raid) {
	if rm.Logger != nil {
		rm.Logger.Printf("Raid %d victory!", raid.ID)
	}
	rm.broadcastMessage("Raid victory!", "gold", raid.CenterX, raid.CenterZ)

	rm.Manager.ForEachNearby(raid.CenterX, raid.CenterZ, 64, func(p *game.Player) {
		if p.Dead {
			return
		}
		if rm.EffectMgr != nil {
			rm.EffectMgr.ApplyEffect(p, EffectHeroOfTheVillage, 0, 40000, false)
		}
	})
}

func (rm *RaidManager) broadcastMessage(text, color string, cx, cz float64) {
	msg := chat.Message{Text: text, Color: color}
	pkt := pk.Marshal(packetid.ClientboundSystemChat, msg, pk.Boolean(false))
	rm.Manager.ForEachNearby(cx, cz, 96, func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
