package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// AnvilSession tracks an active anvil UI interaction for a player.
type AnvilSession struct {
	WindowID   int
	RenameText string
}

// AnvilManager handles anvil UI interactions.
type AnvilManager struct {
	World game.World
}

// NewAnvilManager creates a new AnvilManager.
func NewAnvilManager(world game.World) *AnvilManager {
	return &AnvilManager{World: world}
}

// OpenAnvil opens the anvil UI for a player.
func (am *AnvilManager) OpenAnvil(player *game.Player, x, y, z int) {
	windowID := 11 // distinct window ID for anvil
	player.OpenWindowID = windowID

	player.AnvilSession = &game.AnvilSessionData{
		WindowID: windowID,
	}

	title := chat.Text("Repair & Name")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(windowID),
		pk.VarInt(8), // menu type: anvil
		title,
	))

	SendAnvilWindowContent(player)
}

// SendAnvilWindowContent sends the full anvil window content to the player.
// Anvil window slot layout:
//
//	0     = input (left)
//	1     = material (right)
//	2     = output (result)
//	3-29  = player main inventory (Inventory[9..35])
//	30-38 = player hotbar (Inventory[36..44])
//
// Total: 39 slots
func SendAnvilWindowContent(player *game.Player) {
	if player.AnvilSession == nil {
		return
	}
	stateID := player.NextStateID()
	windowID := player.AnvilSession.WindowID

	slots := make(game.Slot261Array, 39)
	// Anvil slots 0-2 are tracked on AnvilSessionData
	slots[0] = player.AnvilSession.Input.ToSlot()
	slots[1] = player.AnvilSession.Material.ToSlot()
	slots[2] = player.AnvilSession.Output.ToSlot()
	// Main inventory: window slots 3-29 = player.Inventory[9..35]
	for i := 9; i <= 35; i++ {
		slots[3+(i-9)] = player.Inventory[i].ToSlot()
	}
	// Hotbar: window slots 30-38 = player.Inventory[36..44]
	for i := 36; i <= 44; i++ {
		slots[30+(i-36)] = player.Inventory[i].ToSlot()
	}

	cursor := player.CursorItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetContent,
		pk.UnsignedByte(byte(windowID)),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// AnvilSlot returns a pointer to the item stack for an anvil window slot.
func AnvilSlot(player *game.Player, windowSlot int) *game.ItemStack {
	if player.AnvilSession == nil {
		return nil
	}
	switch {
	case windowSlot == 0:
		return &player.AnvilSession.Input
	case windowSlot == 1:
		return &player.AnvilSession.Material
	case windowSlot == 2:
		return &player.AnvilSession.Output
	case windowSlot >= 3 && windowSlot <= 29:
		return &player.Inventory[windowSlot-3+9]
	case windowSlot >= 30 && windowSlot <= 38:
		return &player.Inventory[windowSlot-30+36]
	}
	return nil
}

// ComputeAnvilResult computes the output item and XP cost for the anvil.
// The result is placed in the anvil session output slot and the cost is sent
// to the client via container data.
func ComputeAnvilResult(player *game.Player) {
	if player.AnvilSession == nil {
		return
	}

	input := player.AnvilSession.Input
	material := player.AnvilSession.Material
	renameText := player.AnvilSession.RenameText
	windowID := player.AnvilSession.WindowID

	// Clear previous output
	player.AnvilSession.Output = game.ItemStack{}
	player.AnvilSession.RepairCost = 0

	if input.ID <= 0 || input.Count <= 0 {
		sendAnvilCost(player, windowID, 0)
		return
	}

	cost := int32(0)
	result := input // copy the input item as a starting point

	if material.ID > 0 && material.Count > 0 {
		inputName := ItemNameByID(input.ID)
		materialName := ItemNameByID(material.ID)

		if input.ID == material.ID && input.MaxDurability > 0 {
			// Repair: same item type with durability → combine durability
			repaired := input.Durability + material.Durability
			if repaired > input.MaxDurability {
				repaired = input.MaxDurability
			}
			// Bonus 12% of max durability
			bonus := input.MaxDurability * 12 / 100
			repaired += bonus
			if repaired > input.MaxDurability {
				repaired = input.MaxDurability
			}
			result.Durability = repaired

			// Merge enchantments from material into result
			if material.Enchantments != nil {
				if result.Enchantments == nil {
					result.Enchantments = make(map[string]int32)
				}
				for k, v := range material.Enchantments {
					existing, ok := result.Enchantments[k]
					if ok {
						if v > existing {
							result.Enchantments[k] = v
						} else if v == existing && v+1 <= maxEnchantLevel(k) {
							result.Enchantments[k] = v + 1
						}
					} else {
						result.Enchantments[k] = v
					}
				}
			}
			cost = 2
		} else if isRepairMaterial(inputName, materialName) {
			// Unit repair: repair with raw material (e.g. iron ingot for iron tools)
			repairAmount := input.MaxDurability / 4
			if repairAmount <= 0 {
				repairAmount = 1
			}
			repaired := input.Durability + repairAmount
			if repaired > input.MaxDurability {
				repaired = input.MaxDurability
			}
			result.Durability = repaired
			cost = 1
		} else if materialName == "enchanted_book" && material.Enchantments != nil {
			// Enchanted book: apply enchantments to the item
			if result.Enchantments == nil {
				result.Enchantments = make(map[string]int32)
			}
			for k, v := range material.Enchantments {
				existing, ok := result.Enchantments[k]
				if ok {
					if v > existing {
						result.Enchantments[k] = v
					} else if v == existing && v+1 <= maxEnchantLevel(k) {
						result.Enchantments[k] = v + 1
					}
				} else {
					result.Enchantments[k] = v
				}
			}
			cost = 3
		}
	}

	// Rename: if rename text differs from the original item's name, add rename cost
	if renameText != "" {
		cost += 1
	}

	// If there's no operation at all (no material, no rename), nothing to output
	if cost == 0 {
		sendAnvilCost(player, windowID, 0)
		return
	}

	player.AnvilSession.Output = result
	player.AnvilSession.RepairCost = cost

	sendAnvilCost(player, windowID, cost)
}

// sendAnvilCost sends the repair cost (property 0) to the client.
func sendAnvilCost(player *game.Player, windowID int, cost int32) {
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetData,
		pk.UnsignedByte(byte(windowID)),
		pk.Short(0), // property 0 = repair cost
		pk.Short(int16(cost)),
	))
}

// sendAnvilOutputSlot sends just the output slot (slot 2) to the client.
func sendAnvilOutputSlot(player *game.Player) {
	if player.AnvilSession == nil {
		return
	}
	s := player.AnvilSession.Output.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetSlot,
		pk.UnsignedByte(byte(player.AnvilSession.WindowID)),
		pk.VarInt(0),
		pk.Short(2), // slot 2 = output
		s,
	))
}

// HandleRenameItem handles ServerboundRenameItem from the client.
func HandleRenameItem(player *game.Player, p pk.Packet) {
	var name pk.String
	if err := p.Scan(&name); err != nil {
		return
	}

	if player.AnvilSession == nil {
		return
	}

	player.AnvilSession.RenameText = string(name)

	// Recompute result with the new name
	ComputeAnvilResult(player)
	sendAnvilOutputSlot(player)
}

// maxEnchantLevel returns the maximum level for an enchantment.
func maxEnchantLevel(enchantID string) int32 {
	maxLevels := map[string]int32{
		"sharpness":   5,
		"protection":  4,
		"efficiency":  5,
		"unbreaking":  3,
		"knockback":   2,
		"fire_aspect": 2,
		"looting":     3,
		"fortune":     3,
		"power":       5,
		"smite":       5,
		"silk_touch":  1,
		"mending":     1,
		"sweeping":    3,
		"thorns":      3,
	}
	if v, ok := maxLevels[enchantID]; ok {
		return v
	}
	return 5 // default
}

// isRepairMaterial checks if the material can repair the given item.
// e.g., iron_ingot repairs iron tools/armor.
var repairMaterials = map[string]string{
	"wooden":  "oak_planks",
	"stone":   "cobblestone",
	"iron":    "iron_ingot",
	"golden":  "gold_ingot",
	"diamond": "diamond",
	"netherite": "netherite_ingot",
	"leather": "leather",
	"chainmail": "iron_ingot",
}

func isRepairMaterial(itemName, materialName string) bool {
	// Check tool tiers
	for prefix, mat := range repairMaterials {
		if len(itemName) > len(prefix)+1 && itemName[:len(prefix)] == prefix && materialName == mat {
			return true
		}
	}
	return false
}
