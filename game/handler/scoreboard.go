package handler

import (
	"fmt"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

const scoreboardObjective = "health"

// SendScoreboard sends the scoreboard objective and all current player scores to a newly joined player.
func SendScoreboard(manager *game.PlayerManager, player *game.Player) {
	// Create objective: action=0, name="health", displayName="♥", renderType=1 (hearts)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetObjective,
		pk.String(scoreboardObjective),
		pk.Byte(0), // action: create
		chat.Text("♥"),
		pk.VarInt(1),      // render type: hearts
		pk.Boolean(false), // no number format
	))

	// Display objective: slot=2 (below_name)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSetDisplayObjective,
		pk.VarInt(2), // slot: below_name
		pk.String(scoreboardObjective),
	))

	// Send all online players' health scores
	manager.ForEach(func(p *game.Player) {
		sendScorePacket(player, p.Name, int32(p.Health))
	})
}

// UpdateHealthScore broadcasts an updated health score for a player to all online players.
func UpdateHealthScore(manager *game.PlayerManager, player *game.Player) {
	manager.ForEach(func(p *game.Player) {
		sendScorePacket(p, player.Name, int32(player.Health))
	})
}

// RemovePlayerScore broadcasts a score removal for a leaving player to all online players.
func RemovePlayerScore(manager *game.PlayerManager, playerName string) {
	pkt := pk.Marshal(
		packetid.ClientboundResetScore,
		pk.String(playerName),
		pk.Boolean(true), // has objective
		pk.String(scoreboardObjective),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// sendScorePacket sends a SetScore packet to a single recipient.
func sendScorePacket(recipient *game.Player, entityName string, value int32) {
	recipient.WritePacket(pk.Marshal(
		packetid.ClientboundSetScore,
		pk.String(entityName),
		pk.String(scoreboardObjective),
		pk.VarInt(value),
		pk.Boolean(false), // no display name
		pk.Boolean(false), // no number format
	))
}

// displaySlotID maps slot names to their protocol IDs.
var displaySlotID = map[string]int32{
	"list":      0,
	"sidebar":   1,
	"belowName": 2,
	"belowname": 2,
	"below_name": 2,
}

// objective represents a scoreboard objective.
type objective struct {
	name        string
	displayName string
}

// ScoreboardManager manages custom scoreboard objectives and scores.
type ScoreboardManager struct {
	mu         sync.RWMutex
	manager    *game.PlayerManager
	objectives map[string]*objective             // name -> objective
	scores     map[string]map[string]int32       // objective -> (entity -> score)
	display    map[string]string                 // slot -> objective name
}

// NewScoreboardManager creates a new ScoreboardManager.
func NewScoreboardManager(manager *game.PlayerManager) *ScoreboardManager {
	return &ScoreboardManager{
		manager:    manager,
		objectives: make(map[string]*objective),
		scores:     make(map[string]map[string]int32),
		display:    make(map[string]string),
	}
}

// AddObjective creates a new scoreboard objective.
func (sm *ScoreboardManager) AddObjective(name, displayName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.objectives[name]; exists {
		return fmt.Errorf("objective '%s' already exists", name)
	}

	sm.objectives[name] = &objective{name: name, displayName: displayName}
	sm.scores[name] = make(map[string]int32)

	// Broadcast objective creation
	pkt := pk.Marshal(
		packetid.ClientboundSetObjective,
		pk.String(name),
		pk.Byte(0), // action: create
		chat.Text(displayName),
		pk.VarInt(0),      // render type: integer
		pk.Boolean(false), // no number format
	)
	sm.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	return nil
}

// RemoveObjective removes a scoreboard objective.
func (sm *ScoreboardManager) RemoveObjective(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.objectives[name]; !exists {
		return fmt.Errorf("objective '%s' does not exist", name)
	}

	delete(sm.objectives, name)
	delete(sm.scores, name)

	// Remove from display slots
	for slot, obj := range sm.display {
		if obj == name {
			delete(sm.display, slot)
		}
	}

	// Broadcast objective removal
	pkt := pk.Marshal(
		packetid.ClientboundSetObjective,
		pk.String(name),
		pk.Byte(1), // action: remove
	)
	sm.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	return nil
}

// SetDisplay sets the display slot for an objective.
func (sm *ScoreboardManager) SetDisplay(slot, objectiveName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	slotID, ok := displaySlotID[slot]
	if !ok {
		return fmt.Errorf("unknown display slot: %s (use list, sidebar, or belowName)", slot)
	}

	if _, exists := sm.objectives[objectiveName]; !exists {
		return fmt.Errorf("objective '%s' does not exist", objectiveName)
	}

	sm.display[slot] = objectiveName

	pkt := pk.Marshal(
		packetid.ClientboundSetDisplayObjective,
		pk.VarInt(slotID),
		pk.String(objectiveName),
	)
	sm.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
	return nil
}

// SetScore sets a score for an entity on an objective.
func (sm *ScoreboardManager) SetScore(entityName, objectiveName string, value int32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	scores, ok := sm.scores[objectiveName]
	if !ok {
		return
	}
	scores[entityName] = value

	pkt := pk.Marshal(
		packetid.ClientboundSetScore,
		pk.String(entityName),
		pk.String(objectiveName),
		pk.VarInt(value),
		pk.Boolean(false), // no display name
		pk.Boolean(false), // no number format
	)
	sm.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// AddScore adds to a score for an entity on an objective.
func (sm *ScoreboardManager) AddScore(entityName, objectiveName string, delta int32) {
	sm.mu.Lock()

	scores, ok := sm.scores[objectiveName]
	if !ok {
		sm.mu.Unlock()
		return
	}
	scores[entityName] += delta
	newVal := scores[entityName]
	sm.mu.Unlock()

	pkt := pk.Marshal(
		packetid.ClientboundSetScore,
		pk.String(entityName),
		pk.String(objectiveName),
		pk.VarInt(newVal),
		pk.Boolean(false), // no display name
		pk.Boolean(false), // no number format
	)
	sm.manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// ResetScores resets scores for an entity. If objectiveName is empty, resets all objectives.
func (sm *ScoreboardManager) ResetScores(entityName, objectiveName string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if objectiveName != "" {
		if scores, ok := sm.scores[objectiveName]; ok {
			delete(scores, entityName)
		}
		pkt := pk.Marshal(
			packetid.ClientboundResetScore,
			pk.String(entityName),
			pk.Boolean(true),
			pk.String(objectiveName),
		)
		sm.manager.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	} else {
		for objName, scores := range sm.scores {
			delete(scores, entityName)
			pkt := pk.Marshal(
				packetid.ClientboundResetScore,
				pk.String(entityName),
				pk.Boolean(true),
				pk.String(objName),
			)
			sm.manager.ForEach(func(p *game.Player) {
				p.WritePacket(pkt)
			})
		}
	}
}

// SendAllObjectives sends all custom objectives and scores to a player (for join sync).
func (sm *ScoreboardManager) SendAllObjectives(player *game.Player) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, obj := range sm.objectives {
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSetObjective,
			pk.String(obj.name),
			pk.Byte(0), // create
			chat.Text(obj.displayName),
			pk.VarInt(0),      // integer render
			pk.Boolean(false), // no number format
		))
	}

	for slot, objName := range sm.display {
		slotID, ok := displaySlotID[slot]
		if !ok {
			continue
		}
		player.WritePacket(pk.Marshal(
			packetid.ClientboundSetDisplayObjective,
			pk.VarInt(slotID),
			pk.String(objName),
		))
	}

	for objName, scores := range sm.scores {
		for entity, value := range scores {
			player.WritePacket(pk.Marshal(
				packetid.ClientboundSetScore,
				pk.String(entity),
				pk.String(objName),
				pk.VarInt(value),
				pk.Boolean(false),
				pk.Boolean(false),
			))
		}
	}
}
