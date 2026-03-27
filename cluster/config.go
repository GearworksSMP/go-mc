package cluster

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// ClusterConfig defines the topology of a region-based distributed server cluster.
type ClusterConfig struct {
	Servers       map[string]ServerConfig `json:"servers"`
	DefaultServer string                  `json:"default_server"`
	// Regions maps "dim:rx:rz" → serverID.
	Regions map[string]string `json:"regions"`
}

// ServerConfig describes a single backend region server.
type ServerConfig struct {
	MCAddr   string `json:"mc_addr"`
	GRPCAddr string `json:"grpc_addr,omitempty"`
}

// LoadConfig reads a ClusterConfig from a JSON file.
func LoadConfig(path string) (*ClusterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cluster: read config %s: %w", path, err)
	}
	var cfg ClusterConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cluster: parse config: %w", err)
	}
	return &cfg, nil
}

// LoadConfigFromEnv reads config from CLUSTER_CONFIG (inline JSON) or CLUSTER_CONFIG_FILE (path).
func LoadConfigFromEnv() (*ClusterConfig, error) {
	if raw := os.Getenv("CLUSTER_CONFIG"); raw != "" {
		var cfg ClusterConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, fmt.Errorf("cluster: parse CLUSTER_CONFIG: %w", err)
		}
		return &cfg, nil
	}
	if path := os.Getenv("CLUSTER_CONFIG_FILE"); path != "" {
		return LoadConfig(path)
	}
	return nil, fmt.Errorf("cluster: neither CLUSTER_CONFIG nor CLUSTER_CONFIG_FILE set")
}

// RegionKey builds the map key for a region.
func RegionKey(dimension string, rx, rz int) string {
	return fmt.Sprintf("%s:%d:%d", dimension, rx, rz)
}

// ServerForRegion returns the server ID that owns the given region, or the default server.
func (c *ClusterConfig) ServerForRegion(dimension string, rx, rz int) string {
	key := RegionKey(dimension, rx, rz)
	if id, ok := c.Regions[key]; ok {
		return id
	}
	return c.DefaultServer
}

// ServerForBlock converts block coordinates to a region and looks up the owning server.
func (c *ClusterConfig) ServerForBlock(dimension string, x, z int) string {
	rx := blockToRegion(x)
	rz := blockToRegion(z)
	return c.ServerForRegion(dimension, rx, rz)
}

// ServerForChunk converts chunk coordinates to a region and looks up the owning server.
func (c *ClusterConfig) ServerForChunk(dimension string, cx, cz int) string {
	rx := cx >> 5
	rz := cz >> 5
	return c.ServerForRegion(dimension, rx, rz)
}

// RegionsForServer returns all region keys assigned to a given server ID.
func (c *ClusterConfig) RegionsForServer(serverID string) []string {
	var keys []string
	for k, v := range c.Regions {
		if v == serverID {
			keys = append(keys, k)
		}
	}
	return keys
}

func blockToRegion(coord int) int {
	return int(math.Floor(float64(coord) / 512.0))
}
