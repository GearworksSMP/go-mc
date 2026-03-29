package enchant

// GetLevel returns the level of an enchantment on an item (0 if absent).
// Safe to call with a nil map.
func GetLevel(enchantments map[string]int32, name string) int32 {
	if enchantments == nil {
		return 0
	}
	return enchantments[name]
}

// HasEnchant returns true if the item has the named enchantment at level >= 1.
// Safe to call with a nil map.
func HasEnchant(enchantments map[string]int32, name string) bool {
	return GetLevel(enchantments, name) > 0
}

// MaxLevel returns the maximum allowed level for an enchantment.
// Returns 0 for unknown enchantments.
func MaxLevel(name string) int32 {
	if e, ok := Registry[name]; ok {
		return e.MaxLevel
	}
	return 0
}

// IsCompatible returns whether two enchantments can coexist on one item.
// Unknown enchantments are considered compatible.
func IsCompatible(a, b string) bool {
	if ea, ok := Registry[a]; ok {
		for _, inc := range ea.Incompatible {
			if inc == b {
				return false
			}
		}
	}
	return true
}

// EnsureMap returns the map, creating it if nil.
func EnsureMap(enchantments map[string]int32) map[string]int32 {
	if enchantments == nil {
		return make(map[string]int32)
	}
	return enchantments
}

// ApplyToItem adds an enchantment to an item, respecting max levels.
// Returns the (possibly new) enchantments map.
func ApplyToItem(enchantments map[string]int32, name string, level int32) map[string]int32 {
	enchantments = EnsureMap(enchantments)
	max := MaxLevel(name)
	if max > 0 && level > max {
		level = max
	}
	enchantments[name] = level
	return enchantments
}

// MergeEnchantments merges source into target (anvil-style).
// Same enchantment at same level → upgrade by 1 (if under max).
// Same enchantment at different level → keep higher.
// New enchantment → add if compatible with all existing.
// Returns the (possibly new) target map.
func MergeEnchantments(target, source map[string]int32) map[string]int32 {
	target = EnsureMap(target)
	for name, srcLevel := range source {
		// Check compatibility with existing enchantments
		compatible := true
		for existingName := range target {
			if !IsCompatible(name, existingName) {
				compatible = false
				break
			}
		}
		if !compatible {
			continue
		}

		existing, ok := target[name]
		if !ok {
			target[name] = srcLevel
		} else if srcLevel == existing {
			// Same level: upgrade by 1 if under max
			max := MaxLevel(name)
			if max == 0 || existing+1 <= max {
				target[name] = existing + 1
			}
		} else if srcLevel > existing {
			target[name] = srcLevel
		}
	}
	return target
}
