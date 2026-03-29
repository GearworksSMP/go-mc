package handler

import (
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// TimeManager tracks world time and broadcasts it to players.
// Full day = 24000 ticks (20 min). Dawn=0, noon=6000, sunset=12000, midnight=18000.
type TimeManager struct {
	mu       sync.Mutex
	WorldAge int64      // total ticks, never resets
	DayTime  int64      // time of day (0-24000, wraps)
	Frozen   bool       // if true, DayTime doesn't advance
	Rules    *GameRules // if set, doDaylightCycle overrides Frozen
}

// Tick advances time and broadcasts to all players every 20 ticks (1 second).
func (t *TimeManager) Tick(tick int64, manager *game.PlayerManager) {
	t.mu.Lock()
	t.WorldAge++
	frozen := t.Frozen
	if t.Rules != nil {
		frozen = !t.Rules.GetDoDaylightCycle()
	}
	if !frozen {
		t.DayTime++
		if t.DayTime >= 24000 {
			t.DayTime -= 24000
		}
	}
	age := t.WorldAge
	dayTime := t.DayTime
	t.mu.Unlock()

	// Broadcast every 20 ticks (1 second)
	if age%20 != 0 {
		return
	}

	pkt := pk.Marshal(
		packetid.ClientboundSetTime,
		pk.Long(age),
		pk.Long(dayTime),
		pk.Boolean(!frozen), // tickDayTime: advance client-side time?
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// GetDayTime returns the current day time (thread-safe).
func (t *TimeManager) GetDayTime() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.DayTime
}

// SetDayTime sets the day time (thread-safe).
func (t *TimeManager) SetDayTime(dayTime int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.DayTime = dayTime % 24000
}

// IsNight returns true when hostile mobs can spawn (DayTime 13000-23000).
func (t *TimeManager) IsNight() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.DayTime >= 13000 && t.DayTime <= 23000
}

// SendTime sends current time to a single player (for initial join).
func (t *TimeManager) SendTime(player *game.Player) {
	t.mu.Lock()
	age := t.WorldAge
	dayTime := t.DayTime
	frozen := t.Frozen
	if t.Rules != nil {
		frozen = !t.Rules.GetDoDaylightCycle()
	}
	t.mu.Unlock()

	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetTime,
		pk.Long(age),
		pk.Long(dayTime),
		pk.Boolean(!frozen), // tickDayTime: advance client-side time?
	))
}
