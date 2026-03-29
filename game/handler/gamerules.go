package handler

import (
	"fmt"
	"sync"
)

// GameRules holds all server game rules with thread-safe access.
type GameRules struct {
	mu sync.RWMutex

	difficulty          int32 // 0=peaceful, 1=easy, 2=normal, 3=hard
	keepInventory       bool
	doDaylightCycle     bool
	doMobSpawning       bool
	doFireTick          bool
	mobGriefing         bool
	pvp                 bool
	naturalRegeneration bool
	doWeatherCycle      bool
}

// NewGameRules creates a GameRules with vanilla defaults.
func NewGameRules() *GameRules {
	return &GameRules{
		difficulty:          2, // Normal
		keepInventory:       false,
		doDaylightCycle:     true,
		doMobSpawning:       true,
		doFireTick:          true,
		mobGriefing:         true,
		pvp:                 true,
		naturalRegeneration: true,
		doWeatherCycle:      true,
	}
}

// Getters

func (g *GameRules) GetDifficulty() int32 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.difficulty
}

func (g *GameRules) SetDifficulty(d int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.difficulty = d
}

func (g *GameRules) GetKeepInventory() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.keepInventory
}

func (g *GameRules) GetDoDaylightCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.doDaylightCycle
}

func (g *GameRules) GetDoMobSpawning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.doMobSpawning
}

func (g *GameRules) GetDoFireTick() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.doFireTick
}

func (g *GameRules) GetMobGriefing() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.mobGriefing
}

func (g *GameRules) GetPvP() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.pvp
}

func (g *GameRules) GetNaturalRegeneration() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.naturalRegeneration
}

func (g *GameRules) GetDoWeatherCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.doWeatherCycle
}

// SetRule sets a game rule by name. Returns false if the rule is unknown.
func (g *GameRules) SetRule(name string, value bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch name {
	case "keepinventory":
		g.keepInventory = value
	case "dodaylightcycle":
		g.doDaylightCycle = value
	case "domobspawning":
		g.doMobSpawning = value
	case "dofiretick":
		g.doFireTick = value
	case "mobgriefing":
		g.mobGriefing = value
	case "pvp":
		g.pvp = value
	case "naturalregeneration":
		g.naturalRegeneration = value
	case "doweathercycle":
		g.doWeatherCycle = value
	default:
		return false
	}
	return true
}

// GetRule returns the value of a game rule by name. Returns ("", false) if unknown.
func (g *GameRules) GetRule(name string) (bool, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	switch name {
	case "keepinventory":
		return g.keepInventory, true
	case "dodaylightcycle":
		return g.doDaylightCycle, true
	case "domobspawning":
		return g.doMobSpawning, true
	case "dofiretick":
		return g.doFireTick, true
	case "mobgriefing":
		return g.mobGriefing, true
	case "pvp":
		return g.pvp, true
	case "naturalregeneration":
		return g.naturalRegeneration, true
	case "doweathercycle":
		return g.doWeatherCycle, true
	default:
		return false, false
	}
}

// RuleNames returns all available game rule names.
var RuleNames = []string{
	"keepInventory", "doDaylightCycle", "doMobSpawning", "doFireTick",
	"mobGriefing", "pvp", "naturalRegeneration", "doWeatherCycle",
}

// FormatAll returns a formatted list of all game rules and their values.
func (g *GameRules) FormatAll() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return fmt.Sprintf(
		"keepInventory=%v, doDaylightCycle=%v, doMobSpawning=%v, doFireTick=%v, "+
			"mobGriefing=%v, pvp=%v, naturalRegeneration=%v, doWeatherCycle=%v",
		g.keepInventory, g.doDaylightCycle, g.doMobSpawning, g.doFireTick,
		g.mobGriefing, g.pvp, g.naturalRegeneration, g.doWeatherCycle,
	)
}
