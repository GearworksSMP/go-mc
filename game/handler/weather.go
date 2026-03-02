package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// WeatherState represents the current weather condition.
type WeatherState int

const (
	WeatherClear   WeatherState = 0
	WeatherRain    WeatherState = 1
	WeatherThunder WeatherState = 2
)

// WeatherManager handles weather state transitions and broadcasts.
type WeatherManager struct {
	Manager *game.PlayerManager
	mu      sync.Mutex
	state   WeatherState
	ticks   int64 // countdown until next state change
}

// NewWeatherManager creates a new WeatherManager starting with clear weather.
func NewWeatherManager(manager *game.PlayerManager) *WeatherManager {
	return &WeatherManager{
		Manager: manager,
		state:   WeatherClear,
		ticks:   int64(6000 + rand.Intn(12000)), // 5-15 minutes of clear
	}
}

// Tick advances the weather state machine by one tick.
func (w *WeatherManager) Tick(tick int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.ticks--
	if w.ticks > 0 {
		return
	}

	switch w.state {
	case WeatherClear:
		// Transition to rain
		w.state = WeatherRain
		w.ticks = int64(6000 + rand.Intn(6000)) // 5-10 minutes of rain
		w.broadcastWeatherStart()
	case WeatherRain:
		// 30% chance -> thunder, 70% -> clear
		if rand.Float64() < 0.3 {
			w.state = WeatherThunder
			w.ticks = int64(3000 + rand.Intn(6000)) // 2.5-7.5 minutes of thunder
			w.broadcastThunderStart()
		} else {
			w.state = WeatherClear
			w.ticks = int64(6000 + rand.Intn(12000))
			w.broadcastWeatherStop()
		}
	case WeatherThunder:
		// Back to clear
		w.state = WeatherClear
		w.ticks = int64(6000 + rand.Intn(12000))
		w.broadcastThunderStop()
		w.broadcastWeatherStop()
	}
}

// SetWeather forces a weather state.
func (w *WeatherManager) SetWeather(state WeatherState, duration int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	oldState := w.state
	w.state = state
	if duration > 0 {
		w.ticks = duration
	} else {
		w.ticks = int64(6000 + rand.Intn(6000))
	}

	// Send appropriate packets based on transition
	if oldState != WeatherClear && state == WeatherClear {
		if oldState == WeatherThunder {
			w.broadcastThunderStop()
		}
		w.broadcastWeatherStop()
	}
	if state == WeatherRain && oldState == WeatherClear {
		w.broadcastWeatherStart()
	}
	if state == WeatherThunder {
		if oldState == WeatherClear {
			w.broadcastWeatherStart()
		}
		w.broadcastThunderStart()
	}
}

// State returns the current weather state.
func (w *WeatherManager) State() WeatherState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// SendWeather sends the current weather state to a specific player (for join).
func (w *WeatherManager) SendWeather(player *game.Player) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.state == WeatherRain || w.state == WeatherThunder {
		// Begin raining
		player.WritePacket(pk.Marshal(
			packetid.ClientboundGameEvent,
			pk.UnsignedByte(2),
			pk.Float(0),
		))
		// Rain level
		player.WritePacket(pk.Marshal(
			packetid.ClientboundGameEvent,
			pk.UnsignedByte(7),
			pk.Float(1.0),
		))
	}
	if w.state == WeatherThunder {
		// Thunder level
		player.WritePacket(pk.Marshal(
			packetid.ClientboundGameEvent,
			pk.UnsignedByte(8),
			pk.Float(1.0),
		))
	}
}

func (w *WeatherManager) broadcastWeatherStart() {
	// Begin raining (event 2)
	pkt := pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(2),
		pk.Float(0),
	)
	w.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	// Rain level (event 7)
	pkt2 := pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(7),
		pk.Float(1.0),
	)
	w.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt2)
	})
}

func (w *WeatherManager) broadcastWeatherStop() {
	// End raining (event 1)
	pkt := pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(1),
		pk.Float(0),
	)
	w.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (w *WeatherManager) broadcastThunderStart() {
	// Thunder level (event 8)
	pkt := pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(8),
		pk.Float(1.0),
	)
	w.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

func (w *WeatherManager) broadcastThunderStop() {
	// Thunder level = 0 (event 8)
	pkt := pk.Marshal(
		packetid.ClientboundGameEvent,
		pk.UnsignedByte(8),
		pk.Float(0),
	)
	w.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
