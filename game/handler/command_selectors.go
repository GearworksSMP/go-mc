package handler

import (
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/Tnze/go-mc/game"
)

// resolveTargets parses a target selector or player name and returns matching players.
// Supported selectors: @a (all), @p (nearest), @s (self), @r (random).
// Arguments in brackets: @a[limit=3,distance=10]
func resolveTargets(executor *game.Player, selector string, manager *game.PlayerManager) []*game.Player {
	if !strings.HasPrefix(selector, "@") {
		// Plain player name lookup
		p := manager.GetByName(selector)
		if p == nil {
			return nil
		}
		return []*game.Player{p}
	}

	// Parse selector type and arguments
	selectorType, args := parseSelectorArgs(selector)

	limit := -1
	maxDist := -1.0
	if v, ok := args["limit"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v, ok := args["distance"]; ok {
		if d, err := strconv.ParseFloat(v, 64); err == nil && d > 0 {
			maxDist = d
		}
	}

	switch selectorType {
	case "s":
		return []*game.Player{executor}

	case "a":
		return collectPlayers(executor, manager, limit, maxDist)

	case "p":
		return collectNearest(executor, manager, maxDist)

	case "r":
		return collectRandom(executor, manager, limit, maxDist)

	default:
		return nil
	}
}

// parseSelectorArgs splits "@X[key=val,...]" into the selector letter and argument map.
func parseSelectorArgs(selector string) (string, map[string]string) {
	args := make(map[string]string)
	if len(selector) < 2 {
		return "", args
	}

	sType := string(selector[1])
	rest := selector[2:]

	if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
		inner := rest[1 : len(rest)-1]
		for _, pair := range strings.Split(inner, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				args[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	return sType, args
}

// collectPlayers gathers all players, optionally filtering by distance, with an optional limit.
func collectPlayers(executor *game.Player, manager *game.PlayerManager, limit int, maxDist float64) []*game.Player {
	var result []*game.Player
	ex, _, ez := executor.Position()

	manager.ForEach(func(p *game.Player) {
		if maxDist > 0 {
			px, _, pz := p.Position()
			if game.Distance2D(ex, ez, px, pz) > maxDist {
				return
			}
		}
		result = append(result, p)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

// collectNearest returns the single nearest player to the executor, optionally within maxDist.
func collectNearest(executor *game.Player, manager *game.PlayerManager, maxDist float64) []*game.Player {
	ex, _, ez := executor.Position()
	var nearest *game.Player
	bestDist := math.MaxFloat64

	manager.ForEach(func(p *game.Player) {
		px, _, pz := p.Position()
		dist := game.Distance2D(ex, ez, px, pz)
		if maxDist > 0 && dist > maxDist {
			return
		}
		if dist < bestDist {
			bestDist = dist
			nearest = p
		}
	})

	if nearest == nil {
		return nil
	}
	return []*game.Player{nearest}
}

// collectRandom returns up to limit random players, optionally filtered by distance.
func collectRandom(executor *game.Player, manager *game.PlayerManager, limit int, maxDist float64) []*game.Player {
	candidates := collectPlayers(executor, manager, -1, maxDist)
	if len(candidates) == 0 {
		return nil
	}

	if limit <= 0 {
		limit = 1
	}
	if limit >= len(candidates) {
		return candidates
	}

	// Fisher-Yates shuffle and take first limit
	for i := len(candidates) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	return candidates[:limit]
}
