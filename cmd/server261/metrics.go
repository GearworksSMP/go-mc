package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/game/handler"
)

// MetricsServer serves Prometheus-compatible metrics and health/readiness
// endpoints for Kubernetes probes. It uses only the standard library.
type MetricsServer struct {
	Players *game.PlayerManager
	MobMgr  *handler.MobManager

	ready atomic.Bool

	// tpsMu protects tickDurations and tpsIdx.
	tpsMu         sync.Mutex
	tickDurations [20]time.Duration
	tpsIdx        int
}

// SetReady marks the server as ready. The /ready endpoint will return 200
// once this has been called.
func (m *MetricsServer) SetReady() { m.ready.Store(true) }

// RecordTick records the duration of a single game tick for TPS calculation.
func (m *MetricsServer) RecordTick(d time.Duration) {
	m.tpsMu.Lock()
	m.tickDurations[m.tpsIdx%20] = d
	m.tpsIdx++
	m.tpsMu.Unlock()
}

// GetTPS returns the current ticks-per-second based on the last 20 tick
// durations. A perfectly on-time server returns 20.0.
func (m *MetricsServer) GetTPS() float64 {
	n, total := m.tickStats()
	if n == 0 {
		return 20.0
	}
	avgNs := total.Nanoseconds() / int64(n)
	if avgNs <= 0 {
		return 20.0
	}
	tps := float64(time.Second) / float64(avgNs)
	if tps > 20.0 {
		tps = 20.0
	}
	return tps
}

// tickStats returns the number of recorded ticks and their total duration.
func (m *MetricsServer) tickStats() (int, time.Duration) {
	m.tpsMu.Lock()
	defer m.tpsMu.Unlock()
	n := m.tpsIdx
	if n > 20 {
		n = 20
	}
	var total time.Duration
	for i := range n {
		total += m.tickDurations[i]
	}
	return n, total
}

// StartMetricsServer launches the HTTP server in a background goroutine.
// addr is typically ":8080".
func (m *MetricsServer) StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/ready", m.handleReady)
	mux.HandleFunc("/metrics", m.handleMetrics)

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() { _ = srv.ListenAndServe() }()
}

// MetricsAddr returns the metrics listen address from the environment,
// defaulting to ":8080".
func MetricsAddr() string {
	if p := os.Getenv("METRICS_PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

func (m *MetricsServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *MetricsServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !m.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (m *MetricsServer) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	playersOnline := m.Players.Count()
	tps := m.GetTPS()
	entities := m.MobMgr.MobCount()

	var chunksLoaded int
	m.Players.ForEach(func(p *game.Player) {
		chunksLoaded += len(p.LoadedChunks)
	})

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP mc_players_online Current number of online players\n")
	fmt.Fprintf(w, "# TYPE mc_players_online gauge\n")
	fmt.Fprintf(w, "mc_players_online %d\n", playersOnline)
	fmt.Fprintf(w, "# HELP mc_tps Server ticks per second\n")
	fmt.Fprintf(w, "# TYPE mc_tps gauge\n")
	fmt.Fprintf(w, "mc_tps %.2f\n", tps)
	fmt.Fprintf(w, "# HELP mc_chunks_loaded Total loaded chunks\n")
	fmt.Fprintf(w, "# TYPE mc_chunks_loaded gauge\n")
	fmt.Fprintf(w, "mc_chunks_loaded %d\n", chunksLoaded)
	fmt.Fprintf(w, "# HELP mc_entities_active Total active mob entities\n")
	fmt.Fprintf(w, "# TYPE mc_entities_active gauge\n")
	fmt.Fprintf(w, "mc_entities_active %d\n", entities)
}
