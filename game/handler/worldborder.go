package handler

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// WorldBorderManager tracks and broadcasts the world border state.
type WorldBorderManager struct {
	mu              sync.RWMutex
	CenterX         float64
	CenterZ         float64
	Size            float64 // current diameter
	TargetSize      float64
	LerpSpeed       float64 // blocks per tick (diameter change per tick)
	DamagePerBlock  float32
	DamageBuffer    float32
	WarningDistance  int32
	WarningTime     int32
	Manager         *game.PlayerManager
	SurvivalHandler *SurvivalHandler
	Logger          *log.Logger

	// portalTeleportBoundary is the max size used in InitializeBorder.
	portalTeleportBoundary float64
}

// NewWorldBorderManager creates a WorldBorderManager with vanilla defaults.
func NewWorldBorderManager(mgr *game.PlayerManager) *WorldBorderManager {
	return &WorldBorderManager{
		CenterX:                0,
		CenterZ:                0,
		Size:                   60000000,
		TargetSize:             60000000,
		DamagePerBlock:         0.2,
		DamageBuffer:           5.0,
		WarningDistance:         5,
		WarningTime:            15,
		Manager:                mgr,
		portalTeleportBoundary: 29999984,
	}
}

// Tick advances the border lerp and applies damage to players outside.
func (wb *WorldBorderManager) Tick(tick int64) {
	wb.mu.Lock()

	// Lerp toward target size
	if wb.Size != wb.TargetSize {
		if wb.LerpSpeed <= 0 {
			wb.Size = wb.TargetSize
		} else {
			if wb.Size < wb.TargetSize {
				wb.Size += wb.LerpSpeed
				if wb.Size >= wb.TargetSize {
					wb.Size = wb.TargetSize
				}
			} else {
				wb.Size -= wb.LerpSpeed
				if wb.Size <= wb.TargetSize {
					wb.Size = wb.TargetSize
				}
			}
		}
	}

	cx := wb.CenterX
	cz := wb.CenterZ
	halfSize := wb.Size / 2
	dmgPerBlock := wb.DamagePerBlock
	dmgBuffer := wb.DamageBuffer
	warnDist := float64(wb.WarningDistance)
	wb.mu.Unlock()

	// Check players
	wb.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}

		px, _, pz := p.Position()

		// Distance outside border (positive = outside)
		distOutside := distanceOutsideBorder(px, pz, cx, cz, halfSize)

		// Apply damage if outside border
		if distOutside > 0 {
			dmg := float32(math.Max(0, distOutside-float64(dmgBuffer))) * dmgPerBlock
			if dmg > 0 && wb.SurvivalHandler != nil {
				wb.SurvivalHandler.ApplyDamage(wb.Manager, p, dmg, wb.SurvivalHandler.AttackDamageTypeID)
			}
		}

		// Send warning tint if near the edge
		if warnDist > 0 {
			distInside := -distOutside // positive when inside
			if distInside >= 0 && distInside < warnDist {
				// Player is inside but near the edge — client handles the tint
				// based on the warning distance we already sent.
			}
		}
	})
}

// distanceOutsideBorder returns how far outside the border a point is.
// Returns 0 if inside, positive if outside.
func distanceOutsideBorder(px, pz, cx, cz, halfSize float64) float64 {
	dx := math.Abs(px-cx) - halfSize
	dz := math.Abs(pz-cz) - halfSize
	if dx <= 0 && dz <= 0 {
		return 0
	}
	return math.Max(dx, dz)
}

// SendBorderState sends the full border initialization packet to a player.
func (wb *WorldBorderManager) SendBorderState(p *game.Player) {
	wb.mu.RLock()
	defer wb.mu.RUnlock()

	// Calculate lerp time remaining in milliseconds
	var lerpTimeMs int64
	if wb.Size != wb.TargetSize && wb.LerpSpeed > 0 {
		remaining := math.Abs(wb.TargetSize - wb.Size)
		ticksLeft := remaining / wb.LerpSpeed
		lerpTimeMs = int64(ticksLeft * 50) // 50ms per tick
	}

	// ClientboundInitializeBorder:
	//   Double x, Double z, Double oldSize, Double newSize,
	//   VarLong lerpTime, VarInt newAbsoluteMaxSize,
	//   VarInt warningBlocks, VarInt warningTime
	p.WritePacket(pk.Marshal(
		packetid.ClientboundInitializeBorder,
		pk.Double(wb.CenterX),
		pk.Double(wb.CenterZ),
		pk.Double(wb.Size),
		pk.Double(wb.TargetSize),
		pk.VarLong(lerpTimeMs),
		pk.VarInt(int32(wb.portalTeleportBoundary)),
		pk.VarInt(wb.WarningDistance),
		pk.VarInt(wb.WarningTime),
	))
}

// SetCenter updates the border center and broadcasts to all players.
func (wb *WorldBorderManager) SetCenter(x, z float64) {
	wb.mu.Lock()
	wb.CenterX = x
	wb.CenterZ = z
	wb.mu.Unlock()

	pkt := pk.Marshal(
		packetid.ClientboundSetBorderCenter,
		pk.Double(x),
		pk.Double(z),
	)
	wb.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SetSize instantly resizes the border and broadcasts to all players.
func (wb *WorldBorderManager) SetSize(size float64) {
	wb.mu.Lock()
	wb.Size = size
	wb.TargetSize = size
	wb.LerpSpeed = 0
	wb.mu.Unlock()

	pkt := pk.Marshal(
		packetid.ClientboundSetBorderSize,
		pk.Double(size),
	)
	wb.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SetSizeLerp starts a gradual resize over timeMs milliseconds.
func (wb *WorldBorderManager) SetSizeLerp(targetSize float64, timeMs int64) {
	wb.mu.Lock()
	wb.TargetSize = targetSize
	if timeMs <= 0 {
		wb.Size = targetSize
		wb.LerpSpeed = 0
	} else {
		ticks := float64(timeMs) / 50.0
		wb.LerpSpeed = math.Abs(targetSize-wb.Size) / ticks
	}
	currentSize := wb.Size
	wb.mu.Unlock()

	// ClientboundSetBorderLerpSize: Double oldSize, Double newSize, VarLong lerpTime
	pkt := pk.Marshal(
		packetid.ClientboundSetBorderLerpSize,
		pk.Double(currentSize),
		pk.Double(targetSize),
		pk.VarLong(timeMs),
	)
	wb.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SetWarningDistance updates and broadcasts the warning distance.
func (wb *WorldBorderManager) SetWarningDistance(dist int32) {
	wb.mu.Lock()
	wb.WarningDistance = dist
	wb.mu.Unlock()

	pkt := pk.Marshal(
		packetid.ClientboundSetBorderWarningDistance,
		pk.VarInt(dist),
	)
	wb.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// SetWarningTime updates and broadcasts the warning time (seconds).
func (wb *WorldBorderManager) SetWarningTime(t int32) {
	wb.mu.Lock()
	wb.WarningTime = t
	wb.mu.Unlock()

	pkt := pk.Marshal(
		packetid.ClientboundSetBorderWarningDelay,
		pk.VarInt(t),
	)
	wb.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// GetSize returns the current border diameter.
func (wb *WorldBorderManager) GetSize() float64 {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	return wb.Size
}

// GetCenter returns the current border center coordinates.
func (wb *WorldBorderManager) GetCenter() (float64, float64) {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	return wb.CenterX, wb.CenterZ
}

func (wb *WorldBorderManager) logf(format string, args ...any) {
	if wb.Logger != nil {
		wb.Logger.Printf(format, args...)
	}
}

// cmdWorldBorder handles the /worldborder command for CommandExecutor.
func (c *CommandExecutor) cmdWorldBorder(player *game.Player, args []string) {
	if c.WorldBorderMgr == nil {
		c.sendSystemMsg(player, "World border is not available", "red")
		return
	}
	if len(args) < 1 {
		c.sendSystemMsg(player, "Usage: /worldborder <center|set|add|get|damage|warning> ...", "red")
		return
	}

	wb := c.WorldBorderMgr
	sub := strings.ToLower(args[0])
	switch sub {
	case "center":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /worldborder center <x> <z>", "red")
			return
		}
		x, err1 := strconv.ParseFloat(args[1], 64)
		z, err2 := strconv.ParseFloat(args[2], 64)
		if err1 != nil || err2 != nil {
			c.sendSystemMsg(player, "Invalid coordinates", "red")
			return
		}
		wb.SetCenter(x, z)
		c.sendSystemMsg(player, fmt.Sprintf("Set world border center to %.1f, %.1f", x, z), "")

	case "set":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /worldborder set <size> [time_seconds]", "red")
			return
		}
		size, err := strconv.ParseFloat(args[1], 64)
		if err != nil || size <= 0 {
			c.sendSystemMsg(player, "Invalid size", "red")
			return
		}
		if len(args) >= 3 {
			timeSec, err := strconv.ParseFloat(args[2], 64)
			if err == nil && timeSec > 0 {
				wb.SetSizeLerp(size, int64(timeSec*1000))
				c.sendSystemMsg(player, fmt.Sprintf("Setting world border to %.1f blocks wide over %.0f seconds", size, timeSec), "")
				return
			}
		}
		wb.SetSize(size)
		c.sendSystemMsg(player, fmt.Sprintf("Set world border to %.1f blocks wide", size), "")

	case "add":
		if len(args) < 2 {
			c.sendSystemMsg(player, "Usage: /worldborder add <size> [time_seconds]", "red")
			return
		}
		delta, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			c.sendSystemMsg(player, "Invalid size", "red")
			return
		}
		newSize := wb.GetSize() + delta
		if newSize < 1 {
			newSize = 1
		}
		if len(args) >= 3 {
			timeSec, err := strconv.ParseFloat(args[2], 64)
			if err == nil && timeSec > 0 {
				wb.SetSizeLerp(newSize, int64(timeSec*1000))
				c.sendSystemMsg(player, fmt.Sprintf("Growing world border to %.1f blocks wide over %.0f seconds", newSize, timeSec), "")
				return
			}
		}
		wb.SetSize(newSize)
		c.sendSystemMsg(player, fmt.Sprintf("Set world border to %.1f blocks wide", newSize), "")

	case "get":
		size := wb.GetSize()
		cx, cz := wb.GetCenter()
		c.sendSystemMsg(player, fmt.Sprintf("World border is %.1f blocks wide, centered at %.1f, %.1f", size, cx, cz), "")

	case "damage":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /worldborder damage <amount|buffer> <value>", "red")
			return
		}
		val, err := strconv.ParseFloat(args[2], 32)
		if err != nil {
			c.sendSystemMsg(player, "Invalid value", "red")
			return
		}
		switch strings.ToLower(args[1]) {
		case "amount":
			wb.mu.Lock()
			wb.DamagePerBlock = float32(val)
			wb.mu.Unlock()
			c.sendSystemMsg(player, fmt.Sprintf("Set border damage to %.2f per block", val), "")
		case "buffer":
			wb.mu.Lock()
			wb.DamageBuffer = float32(val)
			wb.mu.Unlock()
			c.sendSystemMsg(player, fmt.Sprintf("Set border damage buffer to %.1f blocks", val), "")
		default:
			c.sendSystemMsg(player, "Usage: /worldborder damage <amount|buffer> <value>", "red")
		}

	case "warning":
		if len(args) < 3 {
			c.sendSystemMsg(player, "Usage: /worldborder warning <distance|time> <value>", "red")
			return
		}
		val, err := strconv.Atoi(args[2])
		if err != nil {
			c.sendSystemMsg(player, "Invalid value", "red")
			return
		}
		switch strings.ToLower(args[1]) {
		case "distance":
			wb.SetWarningDistance(int32(val))
			c.sendSystemMsg(player, fmt.Sprintf("Set border warning distance to %d blocks", val), "")
		case "time":
			wb.SetWarningTime(int32(val))
			c.sendSystemMsg(player, fmt.Sprintf("Set border warning time to %d seconds", val), "")
		default:
			c.sendSystemMsg(player, "Usage: /worldborder warning <distance|time> <value>", "red")
		}

	default:
		c.sendSystemMsg(player, "Usage: /worldborder <center|set|add|get|damage|warning> ...", "red")
	}
}
