package handler

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// SmithingWindowID is the window ID used for smithing table windows.
const SmithingWindowID = 18

// SmithingTableManager handles smithing table interactions (netherite upgrades).
type SmithingTableManager struct {
	World game.World
}

// NewSmithingTableManager creates a new SmithingTableManager.
func NewSmithingTableManager(world game.World) *SmithingTableManager {
	return &SmithingTableManager{World: world}
}

// OpenSmithingTable opens the smithing table UI for a player.
// Menu type 21 = smithing (new style, 1.20+).
func (stm *SmithingTableManager) OpenSmithingTable(player *game.Player, x, y, z int) {
	player.OpenWindowID = SmithingWindowID

	title := chat.Text("Upgrade Gear")
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(SmithingWindowID),
		pk.VarInt(21), // menu type: smithing (new)
		title,
	))
}

// HandleSmithingClick handles clicking the smithing table result slot.
// In 1.20+, the smithing table has 4 slots:
//
//	0: Template (netherite upgrade template)
//	1: Base item (diamond gear)
//	2: Addition (netherite ingot)
//	3: Result
func (stm *SmithingTableManager) HandleSmithingClick(player *game.Player, slotID int) {
	if player.OpenWindowID != SmithingWindowID || slotID != 3 {
		return
	}

	// For simplicity, upgrade the held diamond item to netherite directly.
	heldSlot := int(player.HeldSlot) + 36
	item := &player.Inventory[heldSlot]
	if item.ID <= 0 {
		return
	}

	inputName := ItemNameByID(item.ID)
	outputName := netheriteUpgrade(inputName)
	if outputName == "" {
		return
	}

	// Check player has netherite ingot somewhere in inventory
	netheriteID := itemIDByName("netherite_ingot")
	if netheriteID <= 0 {
		return
	}
	netheriteSlot := -1
	for i := 9; i <= 44; i++ {
		if player.Inventory[i].ID == netheriteID && player.Inventory[i].Count > 0 {
			netheriteSlot = i
			break
		}
	}
	if netheriteSlot < 0 {
		return
	}

	outputID := itemIDByName(outputName)
	if outputID <= 0 {
		return
	}

	// Consume netherite ingot
	player.Inventory[netheriteSlot].Count--
	if player.Inventory[netheriteSlot].Count <= 0 {
		player.Inventory[netheriteSlot] = game.ItemStack{}
	}
	SendSlotUpdate(player, netheriteSlot)

	// Upgrade the item (preserve enchantments and durability)
	item.ID = outputID
	if item.MaxDurability > 0 {
		// Netherite items have higher durability
		item.MaxDurability = netheriteDurability(outputName)
	}
	SendSlotUpdate(player, heldSlot)

	msg := chat.Message{Text: "Item upgraded!", Color: "gold"}
	player.WritePacket(pk.Marshal(
		packetid.ClientboundSystemChat,
		msg,
		pk.Boolean(false),
	))
}

// netheriteUpgrade maps diamond items to their netherite equivalents.
func netheriteUpgrade(inputName string) string {
	switch inputName {
	case "diamond_sword":
		return "netherite_sword"
	case "diamond_pickaxe":
		return "netherite_pickaxe"
	case "diamond_axe":
		return "netherite_axe"
	case "diamond_shovel":
		return "netherite_shovel"
	case "diamond_hoe":
		return "netherite_hoe"
	case "diamond_helmet":
		return "netherite_helmet"
	case "diamond_chestplate":
		return "netherite_chestplate"
	case "diamond_leggings":
		return "netherite_leggings"
	case "diamond_boots":
		return "netherite_boots"
	default:
		return ""
	}
}

// netheriteDurability returns the max durability for a netherite item.
func netheriteDurability(name string) int32 {
	switch name {
	case "netherite_sword":
		return 2031
	case "netherite_pickaxe":
		return 2031
	case "netherite_axe":
		return 2031
	case "netherite_shovel":
		return 2031
	case "netherite_hoe":
		return 2031
	case "netherite_helmet":
		return 407
	case "netherite_chestplate":
		return 592
	case "netherite_leggings":
		return 555
	case "netherite_boots":
		return 481
	default:
		return 2031
	}
}
