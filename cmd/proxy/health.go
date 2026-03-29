package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Tnze/go-mc/cluster"
)

// serverHealth tracks the health status of a single backend server.
type serverHealth struct {
	healthy   bool
	failCount int
	lastCheck time.Time
}

// HealthChecker periodically TCP-dials backend servers to detect failures.
type HealthChecker struct {
	mu      sync.RWMutex
	servers map[string]*serverHealth
	logger  *log.Logger
}

// NewHealthChecker creates a HealthChecker ready for use.
func NewHealthChecker(logger *log.Logger) *HealthChecker {
	return &HealthChecker{
		servers: make(map[string]*serverHealth),
		logger:  logger,
	}
}

const (
	healthCheckInterval = 5 * time.Second
	healthCheckTimeout  = 2 * time.Second
	unhealthyThreshold  = 3
)

// Run performs health checks every 5 seconds until the context is cancelled.
func (hc *HealthChecker) Run(ctx context.Context, cfg *cluster.ClusterConfig) {
	hc.mu.Lock()
	for id := range cfg.Servers {
		hc.servers[id] = &serverHealth{healthy: true}
	}
	hc.mu.Unlock()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll(cfg)
		}
	}
}

// checkAll probes every backend server once.
func (hc *HealthChecker) checkAll(cfg *cluster.ClusterConfig) {
	for id, sc := range cfg.Servers {
		conn, err := net.DialTimeout("tcp", sc.MCAddr, healthCheckTimeout)
		if conn != nil {
			conn.Close()
		}
		now := time.Now()

		hc.mu.Lock()
		sh := hc.servers[id]
		if sh == nil {
			sh = &serverHealth{healthy: true}
			hc.servers[id] = sh
		}
		sh.lastCheck = now

		if err != nil {
			sh.failCount++
			if sh.failCount >= unhealthyThreshold && sh.healthy {
				sh.healthy = false
				hc.logger.Printf("Backend %s marked unhealthy after %d consecutive failures", id, sh.failCount)
			}
		} else {
			if !sh.healthy {
				hc.logger.Printf("Backend %s recovered", id)
			}
			sh.failCount = 0
			sh.healthy = true
		}
		hc.mu.Unlock()
	}
}

// IsHealthy returns whether the given server is considered healthy.
// Unknown servers are treated as unhealthy.
func (hc *HealthChecker) IsHealthy(serverID string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	if sh, ok := hc.servers[serverID]; ok {
		return sh.healthy
	}
	return false
}
