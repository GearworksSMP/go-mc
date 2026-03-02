package handler

import (
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// BedManager handles sleeping mechanics: entering/leaving bed, time skipping, and spawn point setting.
type BedManager struct {
	Manager    *game.PlayerManager
	MobManager *MobManager
	TimeMgr    *TimeManager
	WeatherMgr *WeatherManager
	World      game.World
	Logger     *log.Logger
	AdvMgr     *AdvancementManager
}

// TryStartSleep attempts to put a player to sleep at the given bed position.
// Returns true if the player successfully entered the bed.
func (b *BedManager) TryStartSleep(player *game.Player, x, y, z int) bool {
	// Check if it's night time (players can sleep between 12542 and 23459)
	dayTime := b.TimeMgr.GetDayTime()
	isNight := dayTime >= 12542 && dayTime <= 23459

	// Also allow sleeping during thunderstorms
	isThunder := b.WeatherMgr != nil && b.WeatherMgr.State() == WeatherThunder

	if !isNight && !isThunder {
		msg := chat.Message{Text: "You can only sleep at night or during thunderstorms", Color: "red"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
		return false
	}

	// Check for hostile mobs within 8 blocks of the bed
	if b.hasHostileMobsNear(float64(x)+0.5, float64(y), float64(z)+0.5, 8.0) {
		msg := chat.Message{Text: "You may not rest now, there are monsters nearby", Color: "red"}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSystemChat,
			msg,
			pk.Boolean(false),
		))
		return false
	}

	// Set player as sleeping
	player.Sleeping = true

	// Advancement: sleep in bed
	if b.AdvMgr != nil {
		b.AdvMgr.CheckSleep(player)
	}

	// Set spawn point
	player.HasSpawnPoint = true
	player.SpawnX = float64(x) + 0.5
	player.SpawnY = float64(y) + 0.6
	player.SpawnZ = float64(z) + 0.5

	// Notify the player their spawn point was set
	msg := chat.Message{Text: "Respawn point set", Color: "green"}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false),
	))

	// Broadcast sleeping pose to all players
	BroadcastEntityFlags(b.Manager, player)

	if b.Logger != nil {
		b.Logger.Printf("Player %s started sleeping at (%d, %d, %d)", player.Name, x, y, z)
	}

	// Check if all players are now sleeping
	b.checkAllSleeping()

	return true
}

// checkAllSleeping checks if all online survival-mode players are sleeping,
// and if so, advances time to morning.
func (b *BedManager) checkAllSleeping() {
	allSleeping := true
	survivalCount := 0

	b.Manager.ForEach(func(p *game.Player) {
		if p.GameMode == 0 && !p.Dead { // survival mode, alive
			survivalCount++
			if !p.Sleeping {
				allSleeping = false
			}
		}
	})

	if survivalCount == 0 {
		return
	}

	if allSleeping {
		b.advanceToMorning()
	}
}

// advanceToMorning sets world time to dawn (0) and wakes all sleeping players.
func (b *BedManager) advanceToMorning() {
	if b.Logger != nil {
		b.Logger.Printf("All players sleeping, advancing to morning")
	}

	// Set time to morning (0 = dawn)
	b.TimeMgr.SetDayTime(0)

	// Broadcast new time immediately to all players
	b.Manager.ForEach(func(p *game.Player) {
		b.TimeMgr.SendTime(p)
	})

	// Clear weather if raining/thundering
	if b.WeatherMgr != nil && b.WeatherMgr.State() != WeatherClear {
		b.WeatherMgr.SetWeather(WeatherClear, 0)
	}

	// Wake all sleeping players
	b.Manager.ForEach(func(p *game.Player) {
		if p.Sleeping {
			b.WakePlayer(p)
		}
	})

	// Broadcast chat message
	msg := chat.Message{Text: "The night was skipped", Color: "yellow"}
	pkt := pk.Marshal(packetid.ClientboundSystemChat, msg, pk.Boolean(false))
	b.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// WakePlayer wakes a sleeping player and broadcasts the standing pose.
func (b *BedManager) WakePlayer(player *game.Player) {
	if !player.Sleeping {
		return
	}
	player.Sleeping = false

	// Broadcast standing pose to all players
	BroadcastEntityFlags(b.Manager, player)
}

// WakeAllPlayers wakes all sleeping players (used when conditions change).
func (b *BedManager) WakeAllPlayers() {
	b.Manager.ForEach(func(p *game.Player) {
		if p.Sleeping {
			b.WakePlayer(p)
		}
	})
}

// Tick performs periodic checks for sleeping players.
func (b *BedManager) Tick(tick int64) {
	// Check every 20 ticks (1 second) if sleeping conditions are still valid
	if tick%20 != 0 {
		return
	}

	hasSleepers := false
	b.Manager.ForEach(func(p *game.Player) {
		if p.Sleeping {
			hasSleepers = true
		}
	})

	if !hasSleepers {
		return
	}

	// Re-check if all survival players are sleeping (someone may have joined)
	b.checkAllSleeping()
}

// hasHostileMobsNear returns true if there are hostile mobs within the given distance.
func (b *BedManager) hasHostileMobsNear(x, y, z, radius float64) bool {
	if b.MobManager == nil {
		return false
	}

	b.MobManager.mu.Lock()
	defer b.MobManager.mu.Unlock()

	radiusSq := radius * radius
	for _, mob := range b.MobManager.Mobs {
		if !mob.Hostile || mob.Health <= 0 {
			continue
		}
		dx := mob.X - x
		dy := mob.Y - y
		dz := mob.Z - z
		distSq := dx*dx + dy*dy + dz*dz
		if distSq <= radiusSq {
			return true
		}
	}
	return false
}
