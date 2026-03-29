package handler

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// TPSMetrics provides TPS, MSPT, and lag spike data.
// Satisfied by MetricsServer in cmd/server261.
type TPSMetrics interface {
	GetTPS() float64
	GetMSPT() float64
	GetLagStats() TPSLagStats
}

// TPSLagStats holds lag spike counters returned by TPSMetrics.
type TPSLagStats struct {
	OverrunCount int
	WorstTick    time.Duration
}

// TPSManager monitors tick performance and broadcasts warnings for lag spikes.
type TPSManager struct {
	Manager *game.PlayerManager
	Metrics TPSMetrics
	Logger  *log.Logger
	PermMgr *PermissionManager
}

// Tick checks the latest tick duration for lag spikes, logging warnings
// at >100ms and broadcasting to ops at >500ms.
func (t *TPSManager) Tick(tickDuration time.Duration) {
	if tickDuration > 100*time.Millisecond {
		t.Logger.Printf("Lag spike detected: tick took %dms (2x normal)", tickDuration.Milliseconds())
	}
	if tickDuration > 500*time.Millisecond {
		msg := chat.Message{
			Text:  fmt.Sprintf("[Server] Lag spike: tick took %dms", tickDuration.Milliseconds()),
			Color: "red",
			Bold:  true,
		}
		pkt := pk.Marshal(packetid.ClientboundSystemChat, msg, pk.Boolean(false))
		t.Manager.ForEach(func(p *game.Player) {
			if t.PermMgr != nil && t.PermMgr.OpLevel(p.UUID) >= 2 {
				p.WritePacket(pkt)
			}
		})
	}
}

// HandleTPSCommand sends TPS, MSPT, and memory stats to the requesting player.
func (t *TPSManager) HandleTPSCommand(player *game.Player) {
	tps := t.Metrics.GetTPS()
	mspt := t.Metrics.GetMSPT()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memMB := memStats.Alloc / 1024 / 1024

	var tpsColor string
	switch {
	case tps >= 18:
		tpsColor = "green"
	case tps >= 15:
		tpsColor = "yellow"
	default:
		tpsColor = "red"
	}

	lagStats := t.Metrics.GetLagStats()

	msg := chat.Message{
		Extra: []chat.Message{
			{Text: fmt.Sprintf("TPS: %.1f", tps), Color: tpsColor},
			{Text: fmt.Sprintf(" | MSPT: %.1fms", mspt), Color: "aqua"},
			{Text: fmt.Sprintf(" | Memory: %dMB", memMB), Color: "gray"},
			{Text: fmt.Sprintf(" | Overruns: %d | Worst: %dms",
				lagStats.OverrunCount, lagStats.WorstTick.Milliseconds()), Color: "gray"},
		},
	}

	player.WritePacket(pk.Marshal(packetid.ClientboundSystemChat, msg, pk.Boolean(false)))
}
