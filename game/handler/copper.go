package handler

import (
	"strings"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// CopperManager handles copper block waxing with honeycomb and scraping with axes.
type CopperManager struct {
	Manager *game.PlayerManager
	World   game.World
}

// copperToWaxed maps un-waxed copper block names to their waxed variants.
var copperToWaxed = map[string]string{
	"copper_block":               "waxed_copper_block",
	"exposed_copper":             "waxed_exposed_copper",
	"weathered_copper":           "waxed_weathered_copper",
	"oxidized_copper":            "waxed_oxidized_copper",
	"cut_copper":                 "waxed_cut_copper",
	"exposed_cut_copper":         "waxed_exposed_cut_copper",
	"weathered_cut_copper":       "waxed_weathered_cut_copper",
	"oxidized_cut_copper":        "waxed_oxidized_cut_copper",
	"chiseled_copper":            "waxed_chiseled_copper",
	"exposed_chiseled_copper":    "waxed_exposed_chiseled_copper",
	"weathered_chiseled_copper":  "waxed_weathered_chiseled_copper",
	"oxidized_chiseled_copper":   "waxed_oxidized_chiseled_copper",
	"cut_copper_stairs":          "waxed_cut_copper_stairs",
	"exposed_cut_copper_stairs":  "waxed_exposed_cut_copper_stairs",
	"weathered_cut_copper_stairs": "waxed_weathered_cut_copper_stairs",
	"oxidized_cut_copper_stairs": "waxed_oxidized_cut_copper_stairs",
	"cut_copper_slab":            "waxed_cut_copper_slab",
	"exposed_cut_copper_slab":    "waxed_exposed_cut_copper_slab",
	"weathered_cut_copper_slab":  "waxed_weathered_cut_copper_slab",
	"oxidized_cut_copper_slab":   "waxed_oxidized_cut_copper_slab",
	"copper_door":                "waxed_copper_door",
	"exposed_copper_door":        "waxed_exposed_copper_door",
	"weathered_copper_door":      "waxed_weathered_copper_door",
	"oxidized_copper_door":       "waxed_oxidized_copper_door",
	"copper_trapdoor":            "waxed_copper_trapdoor",
	"exposed_copper_trapdoor":    "waxed_exposed_copper_trapdoor",
	"weathered_copper_trapdoor":  "waxed_weathered_copper_trapdoor",
	"oxidized_copper_trapdoor":   "waxed_oxidized_copper_trapdoor",
	"copper_grate":               "waxed_copper_grate",
	"exposed_copper_grate":       "waxed_exposed_copper_grate",
	"weathered_copper_grate":     "waxed_weathered_copper_grate",
	"oxidized_copper_grate":      "waxed_oxidized_copper_grate",
	"copper_bulb":                "waxed_copper_bulb",
	"exposed_copper_bulb":        "waxed_exposed_copper_bulb",
	"weathered_copper_bulb":      "waxed_weathered_copper_bulb",
	"oxidized_copper_bulb":       "waxed_oxidized_copper_bulb",
}

// waxedToCopper is the reverse of copperToWaxed.
var waxedToCopper map[string]string

// copperScrapeBack maps an oxidized copper block to its previous oxidation stage.
var copperScrapeBack = map[string]string{
	"exposed_copper":             "copper_block",
	"weathered_copper":           "exposed_copper",
	"oxidized_copper":            "weathered_copper",
	"exposed_cut_copper":         "cut_copper",
	"weathered_cut_copper":       "exposed_cut_copper",
	"oxidized_cut_copper":        "weathered_cut_copper",
	"exposed_chiseled_copper":    "chiseled_copper",
	"weathered_chiseled_copper":  "exposed_chiseled_copper",
	"oxidized_chiseled_copper":   "weathered_chiseled_copper",
	"exposed_cut_copper_stairs":  "cut_copper_stairs",
	"weathered_cut_copper_stairs": "exposed_cut_copper_stairs",
	"oxidized_cut_copper_stairs": "weathered_cut_copper_stairs",
	"exposed_cut_copper_slab":    "cut_copper_slab",
	"weathered_cut_copper_slab":  "exposed_cut_copper_slab",
	"oxidized_cut_copper_slab":   "weathered_cut_copper_slab",
	"exposed_copper_door":        "copper_door",
	"weathered_copper_door":      "exposed_copper_door",
	"oxidized_copper_door":       "weathered_copper_door",
	"exposed_copper_trapdoor":    "copper_trapdoor",
	"weathered_copper_trapdoor":  "exposed_copper_trapdoor",
	"oxidized_copper_trapdoor":   "weathered_copper_trapdoor",
	"exposed_copper_grate":       "copper_grate",
	"weathered_copper_grate":     "exposed_copper_grate",
	"oxidized_copper_grate":      "weathered_copper_grate",
	"exposed_copper_bulb":        "copper_bulb",
	"weathered_copper_bulb":      "exposed_copper_bulb",
	"oxidized_copper_bulb":       "weathered_copper_bulb",
}

// copperDefaultStateID caches the default state ID for each copper block name.
// Built at init time from block.ToStateID and block.FromID.
var copperDefaultStateID map[string]int

func init() {
	// Build reverse waxed→unwaxed map.
	waxedToCopper = make(map[string]string, len(copperToWaxed))
	for k, v := range copperToWaxed {
		waxedToCopper[v] = k
	}

	// Build default state ID cache for all copper block names referenced in the maps.
	names := make(map[string]struct{})
	for k, v := range copperToWaxed {
		names[k] = struct{}{}
		names[v] = struct{}{}
	}
	for k, v := range copperScrapeBack {
		names[k] = struct{}{}
		names[v] = struct{}{}
	}

	copperDefaultStateID = make(map[string]int, len(names))
	for name := range names {
		fullName := "minecraft:" + name
		b, ok := block.FromID[fullName]
		if !ok {
			continue
		}
		if sid, ok := block.ToStateID[b]; ok {
			copperDefaultStateID[name] = int(sid)
		}
	}
}

// copperTransformStateID computes the target state ID when transforming one copper
// block type to another, preserving block properties (facing, half, type, etc.).
// It uses the offset from the source block's default state to the current state,
// and applies it to the target block's default state.
func copperTransformStateID(currentStateID int, srcName, dstName string) (int, bool) {
	srcDefault, ok1 := copperDefaultStateID[srcName]
	dstDefault, ok2 := copperDefaultStateID[dstName]
	if !ok1 || !ok2 {
		return 0, false
	}
	offset := currentStateID - srcDefault
	targetID := dstDefault + offset
	// Validate the target state ID exists and belongs to the right block.
	if targetID < 0 || targetID >= len(block.StateList) || block.StateList[targetID] == nil {
		return 0, false
	}
	return targetID, true
}

// WaxCopper waxes a copper block at the given position using honeycomb.
// Returns true if the block was waxed.
func (m *CopperManager) WaxCopper(player *game.Player, x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))
	waxedName, ok := copperToWaxed[name]
	if !ok {
		return false
	}
	newStateID, ok := copperTransformStateID(int(state), name, waxedName)
	if !ok {
		return false
	}
	m.World.SetBlock(x, y, z, block.StateID(newStateID))
	broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newStateID))
	return true
}

// ScrapeCopper scrapes wax or one oxidation stage from a copper block using an axe.
// Returns true if the block was modified.
func (m *CopperManager) ScrapeCopper(player *game.Player, x, y, z int) bool {
	state, err := m.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	name := BlockNameFromState(int(state))

	// First try de-waxing.
	if unwaxedName, ok := waxedToCopper[name]; ok {
		newStateID, ok := copperTransformStateID(int(state), name, unwaxedName)
		if !ok {
			return false
		}
		m.World.SetBlock(x, y, z, block.StateID(newStateID))
		broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newStateID))
		return true
	}

	// Then try scraping one oxidation level.
	if prevName, ok := copperScrapeBack[name]; ok {
		newStateID, ok := copperTransformStateID(int(state), name, prevName)
		if !ok {
			return false
		}
		m.World.SetBlock(x, y, z, block.StateID(newStateID))
		broadcastBlockUpdateDirect(m.Manager, x, y, z, int32(newStateID))
		return true
	}

	return false
}

// IsAxeItem returns true if the item name is any type of axe.
func IsAxeItem(name string) bool {
	return strings.HasSuffix(name, "_axe")
}
