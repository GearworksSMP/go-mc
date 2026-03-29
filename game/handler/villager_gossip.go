package handler

import "github.com/google/uuid"

// GossipEntry represents a single gossip entry held by a villager.
type GossipEntry struct {
	Type   string    // gossip type
	Target uuid.UUID // player UUID this gossip is about
	Value  int       // gossip strength
	Tick   int64     // tick when this gossip was last updated
}

// Gossip type constants.
const (
	GossipMajorPositive = "major_positive"
	GossipMinorPositive = "minor_positive"
	GossipMinorNegative = "minor_negative"
	GossipMajorNegative = "major_negative"
	GossipTrading       = "trading"
)

// gossipConfig holds the max value and daily decay rate for each gossip type.
type gossipConfig struct {
	MaxValue int
	Decay    int // amount to subtract per day (24000 ticks)
}

var gossipConfigs = map[string]gossipConfig{
	GossipMajorPositive: {MaxValue: 20, Decay: 1},
	GossipMinorPositive: {MaxValue: 25, Decay: 1},
	GossipMinorNegative: {MaxValue: 25, Decay: 1},
	GossipMajorNegative: {MaxValue: 25, Decay: 10},
	GossipTrading:       {MaxValue: 25, Decay: 2},
}

// gossipWeight returns the reputation weight for a gossip type.
func gossipWeight(gtype string) int {
	switch gtype {
	case GossipMajorPositive:
		return 5
	case GossipMinorPositive:
		return 1
	case GossipTrading:
		return 1
	case GossipMinorNegative:
		return -1
	case GossipMajorNegative:
		return -5
	default:
		return 0
	}
}

// CalculateReputation computes the weighted reputation sum for a player from gossip entries.
func CalculateReputation(gossips []GossipEntry, playerUUID uuid.UUID) int {
	rep := 0
	for _, g := range gossips {
		if g.Target == playerUUID && g.Value > 0 {
			rep += g.Value * gossipWeight(g.Type)
		}
	}
	return rep
}

// DecayGossip reduces gossip values based on elapsed time since each entry was last updated.
// Entries whose value drops to zero or below are removed.
func DecayGossip(gossips []GossipEntry, currentTick int64) []GossipEntry {
	const ticksPerDay = 24000
	result := gossips[:0] // reuse backing array
	for _, g := range gossips {
		elapsed := currentTick - g.Tick
		if elapsed <= 0 {
			result = append(result, g)
			continue
		}
		cfg, ok := gossipConfigs[g.Type]
		if !ok {
			continue
		}
		days := int(elapsed / ticksPerDay)
		if days <= 0 {
			result = append(result, g)
			continue
		}
		g.Value -= cfg.Decay * days
		g.Tick = currentTick
		if g.Value > 0 {
			result = append(result, g)
		}
	}
	return result
}

// AddGossip adds or increments a gossip entry for the given type and player.
// The value is clamped to the type's maximum. If an existing entry for the same
// type and target exists, its value is increased.
func AddGossip(gossips []GossipEntry, gtype string, playerUUID uuid.UUID, value int, tick int64) []GossipEntry {
	cfg, ok := gossipConfigs[gtype]
	if !ok {
		return gossips
	}

	for i := range gossips {
		if gossips[i].Type == gtype && gossips[i].Target == playerUUID {
			gossips[i].Value += value
			if gossips[i].Value > cfg.MaxValue {
				gossips[i].Value = cfg.MaxValue
			}
			gossips[i].Tick = tick
			return gossips
		}
	}

	v := value
	if v > cfg.MaxValue {
		v = cfg.MaxValue
	}
	return append(gossips, GossipEntry{
		Type:   gtype,
		Target: playerUUID,
		Value:  v,
		Tick:   tick,
	})
}
