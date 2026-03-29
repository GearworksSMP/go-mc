package handler

import (
	"strings"

	"github.com/Tnze/go-mc/game"
)

// CanBreakBlock reports whether the player may break the named block.
// Returns true for all non-adventure game modes.
func CanBreakBlock(p *game.Player, blockName string) bool {
	if p.GameMode != 2 {
		return true
	}
	held := heldItem(p)
	if held == nil || held.ID <= 0 {
		return false
	}
	return matchesBlockList(held.CanDestroy, blockName)
}

// CanPlaceBlock reports whether the player may place against the named block.
// Returns true for all non-adventure game modes.
func CanPlaceBlock(p *game.Player, adjacentBlockName string) bool {
	if p.GameMode != 2 {
		return true
	}
	held := heldItem(p)
	if held == nil || held.ID <= 0 {
		return false
	}
	return matchesBlockList(held.CanPlaceOn, adjacentBlockName)
}

func heldItem(p *game.Player) *game.ItemStack {
	slot := int(p.HeldSlot) + 36
	if slot < 0 || slot >= len(p.Inventory) {
		return nil
	}
	return &p.Inventory[slot]
}

// matchesBlockList checks whether blockName appears in the given list.
// Each entry may be a bare name ("stone") or namespace-qualified
// ("minecraft:stone"); both forms are normalised before comparison.
func matchesBlockList(list []string, blockName string) bool {
	normalised := stripNamespace(blockName)
	for _, entry := range list {
		if stripNamespace(entry) == normalised {
			return true
		}
	}
	return false
}

// stripNamespace removes the "minecraft:" prefix if present.
func stripNamespace(name string) string {
	if strings.HasPrefix(name, "minecraft:") {
		return name[len("minecraft:"):]
	}
	return name
}
