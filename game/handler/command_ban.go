package handler

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// BanEntry represents a single ban record.
type BanEntry struct {
	PlayerName string `json:"player_name"`
	UUID       string `json:"uuid"`
	Reason     string `json:"reason"`
	BannedAt   string `json:"banned_at"`
	BannedBy   string `json:"banned_by"`
}

// BanManager manages player bans with file persistence.
type BanManager struct {
	mu       sync.RWMutex
	filePath string
	bans     []BanEntry
}

// LoadBans loads bans from a JSON file. Creates an empty file if it does not exist.
func LoadBans(path string) (*BanManager, error) {
	bm := &BanManager{filePath: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty ban file
			bm.bans = []BanEntry{}
			return bm, bm.SaveBans()
		}
		return nil, err
	}

	if len(data) == 0 {
		bm.bans = []BanEntry{}
		return bm, nil
	}

	if err := json.Unmarshal(data, &bm.bans); err != nil {
		return nil, err
	}
	return bm, nil
}

// SaveBans persists the ban list to the JSON file.
func (bm *BanManager) SaveBans() error {
	bm.mu.RLock()
	data, err := json.MarshalIndent(bm.bans, "", "  ")
	bm.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(bm.filePath, data, 0644)
}

// Ban adds a player to the ban list.
func (bm *BanManager) Ban(name, playerUUID, reason, bannedBy string) {
	bm.mu.Lock()
	// Remove existing entry for this player first
	filtered := bm.bans[:0]
	for _, b := range bm.bans {
		if !strings.EqualFold(b.PlayerName, name) {
			filtered = append(filtered, b)
		}
	}
	bm.bans = append(filtered, BanEntry{
		PlayerName: name,
		UUID:       playerUUID,
		Reason:     reason,
		BannedAt:   time.Now().UTC().Format(time.RFC3339),
		BannedBy:   bannedBy,
	})
	bm.mu.Unlock()
	_ = bm.SaveBans()
}

// Pardon removes a player from the ban list.
func (bm *BanManager) Pardon(name string) {
	bm.mu.Lock()
	filtered := bm.bans[:0]
	for _, b := range bm.bans {
		if !strings.EqualFold(b.PlayerName, name) {
			filtered = append(filtered, b)
		}
	}
	bm.bans = filtered
	bm.mu.Unlock()
	_ = bm.SaveBans()
}

// IsBanned checks if a player is banned. Returns the ban status and reason.
func (bm *BanManager) IsBanned(name string) (bool, string) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	for _, b := range bm.bans {
		if strings.EqualFold(b.PlayerName, name) {
			return true, b.Reason
		}
	}
	return false, ""
}

// IsBannedUUID checks if a player UUID is banned.
func (bm *BanManager) IsBannedUUID(playerUUID string) (bool, string) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	for _, b := range bm.bans {
		if b.UUID == playerUUID {
			return true, b.Reason
		}
	}
	return false, ""
}

// ListBans returns all current ban entries.
func (bm *BanManager) ListBans() []BanEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	result := make([]BanEntry, len(bm.bans))
	copy(result, bm.bans)
	return result
}
