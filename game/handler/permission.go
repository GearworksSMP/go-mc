package handler

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/google/uuid"
)

// PermissionManager manages operator status and whitelist.
type PermissionManager struct {
	mu               sync.RWMutex
	ops              map[uuid.UUID]int    // UUID → op level (1-4)
	whitelist        map[uuid.UUID]string // UUID → player name
	whitelistEnabled bool
	opsFile          string
	whitelistFile    string
}

// NewPermissionManager creates a new PermissionManager.
func NewPermissionManager(opsFile, whitelistFile string) *PermissionManager {
	pm := &PermissionManager{
		ops:           make(map[uuid.UUID]int),
		whitelist:     make(map[uuid.UUID]string),
		opsFile:       opsFile,
		whitelistFile: whitelistFile,
	}
	pm.loadOps()
	pm.loadWhitelist()
	return pm
}

// IsOp returns true if the player is an operator.
func (pm *PermissionManager) IsOp(id uuid.UUID) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, ok := pm.ops[id]
	return ok
}

// OpLevel returns the op level (0-4, 0 = not op).
func (pm *PermissionManager) OpLevel(id uuid.UUID) int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.ops[id]
}

// SetOp sets a player's op level. Level 0 removes op status.
func (pm *PermissionManager) SetOp(id uuid.UUID, name string, level int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if level <= 0 {
		delete(pm.ops, id)
	} else {
		pm.ops[id] = level
	}
	pm.saveOps()
}

// IsWhitelisted returns true if the player is whitelisted (or whitelist is disabled).
func (pm *PermissionManager) IsWhitelisted(id uuid.UUID) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if !pm.whitelistEnabled {
		return true
	}
	_, ok := pm.whitelist[id]
	return ok
}

// WhitelistEnabled returns whether the whitelist is active.
func (pm *PermissionManager) WhitelistEnabled() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.whitelistEnabled
}

// SetWhitelistEnabled enables or disables the whitelist.
func (pm *PermissionManager) SetWhitelistEnabled(enabled bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.whitelistEnabled = enabled
	pm.saveWhitelist()
}

// AddWhitelist adds a player to the whitelist.
func (pm *PermissionManager) AddWhitelist(id uuid.UUID, name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.whitelist[id] = name
	pm.saveWhitelist()
}

// RemoveWhitelist removes a player from the whitelist.
func (pm *PermissionManager) RemoveWhitelist(id uuid.UUID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.whitelist, id)
	pm.saveWhitelist()
}

// WhitelistNames returns a list of whitelisted player names.
func (pm *PermissionManager) WhitelistNames() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	names := make([]string, 0, len(pm.whitelist))
	for _, name := range pm.whitelist {
		names = append(names, name)
	}
	return names
}

type opEntry struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Level int    `json:"level"`
}

type whitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type whitelistFile struct {
	Enabled bool             `json:"enabled"`
	Players []whitelistEntry `json:"players"`
}

func (pm *PermissionManager) loadOps() {
	data, err := os.ReadFile(pm.opsFile)
	if err != nil {
		return
	}
	var entries []opEntry
	if json.Unmarshal(data, &entries) != nil {
		return
	}
	for _, e := range entries {
		id, err := uuid.Parse(e.UUID)
		if err != nil {
			continue
		}
		pm.ops[id] = e.Level
	}
}

func (pm *PermissionManager) saveOps() {
	entries := make([]opEntry, 0, len(pm.ops))
	for id, level := range pm.ops {
		entries = append(entries, opEntry{UUID: id.String(), Level: level})
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(pm.opsFile, data, 0644)
}

func (pm *PermissionManager) loadWhitelist() {
	data, err := os.ReadFile(pm.whitelistFile)
	if err != nil {
		return
	}
	var wf whitelistFile
	if json.Unmarshal(data, &wf) != nil {
		return
	}
	pm.whitelistEnabled = wf.Enabled
	for _, e := range wf.Players {
		id, err := uuid.Parse(e.UUID)
		if err != nil {
			continue
		}
		pm.whitelist[id] = e.Name
	}
}

func (pm *PermissionManager) saveWhitelist() {
	wf := whitelistFile{Enabled: pm.whitelistEnabled}
	for id, name := range pm.whitelist {
		wf.Players = append(wf.Players, whitelistEntry{UUID: id.String(), Name: name})
	}
	data, err := json.MarshalIndent(wf, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(pm.whitelistFile, data, 0644)
}
