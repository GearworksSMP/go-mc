package handler

import (
	"bytes"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Villager entity type ID (26.1-snapshot-2 registry).
const MobTypeVillager int32 = 139

// Villager sound IDs (from data/soundid/soundid.go).
const (
	SoundVillagerAmbient int32 = 1070
	SoundVillagerDeath   int32 = 1072
	SoundVillagerHurt    int32 = 1073
	SoundVillagerNo      int32 = 1074
	SoundVillagerTrade   int32 = 1075
	SoundVillagerYes     int32 = 1076
)

// Merchant window ID.
const MerchantWindowID = 20

// villagerProfessionID maps a profession string to its protocol ID.
func villagerProfessionID(name string) int32 {
	switch name {
	case "armorer":
		return 1
	case "butcher":
		return 2
	case "cartographer":
		return 3
	case "cleric":
		return 4
	case "farmer":
		return 5
	case "fisherman":
		return 6
	case "fletcher":
		return 7
	case "leatherworker":
		return 8
	case "librarian":
		return 9
	case "mason":
		return 10
	case "nitwit":
		return 11
	case "shepherd":
		return 12
	case "toolsmith":
		return 13
	case "weaponsmith":
		return 14
	default:
		return 0 // none
	}
}

// VillagerData holds the profession and trade list for a villager mob.
type VillagerData struct {
	Profession      string
	Trades          []Trade
	RestocksToday   int   // number of restocks performed today (max 2 per day)
	LastRestockTick int64 // tick when the last restock occurred
	Gossip          []GossipEntry // gossip entries about players
	WorkstationPos  [3]int        // claimed workstation block position (zero value = none)
	LastSleepTick   int64         // tick when villager last slept
}

// SetProfession changes the villager's profession and regenerates trades.
func (vd *VillagerData) SetProfession(profession string) {
	vd.Profession = profession
	vd.Trades = tradesForProfession(profession)
}

// Trade represents a single villager trade offer.
type Trade struct {
	InputItem1      game.ItemStack // required first input
	InputItem2      game.ItemStack // optional second input (ID=0 if none)
	OutputItem      game.ItemStack // output
	Uses            int32
	MaxUses         int32
	XP              int32
	PriceMultiplier float32
}

// merchantSession tracks an active villager trading session.
type merchantSession struct {
	VillagerEID    int32
	SelectedTrade  int // currently selected trade index, or -1
}

// VillagerManager handles villager spawning, trading UI, and trade execution.
type VillagerManager struct {
	MobMgr  *MobManager
	Manager *game.PlayerManager
	mu      sync.Mutex
	// Track which player has which villager's merchant window open.
	// Key: player UUID string.
	openMerchant map[string]*merchantSession
}

// NewVillagerManager creates a new VillagerManager.
func NewVillagerManager(mobMgr *MobManager, manager *game.PlayerManager) *VillagerManager {
	return &VillagerManager{
		MobMgr:       mobMgr,
		Manager:      manager,
		openMerchant: make(map[string]*merchantSession),
	}
}

// professions lists the available villager professions.
var professions = []string{
	"farmer", "librarian", "armorer", "butcher", "cleric",
	"cartographer", "fisherman", "fletcher", "leatherworker",
	"mason", "shepherd", "toolsmith", "weaponsmith",
}

// RandomProfession picks a random villager profession.
func RandomProfession() string {
	return professions[rand.Intn(len(professions))]
}

// NewVillagerData creates VillagerData with profession-appropriate trades.
func NewVillagerData(profession string) *VillagerData {
	vd := &VillagerData{
		Profession: profession,
		Trades:     tradesForProfession(profession),
	}
	return vd
}

// tradesForProfession returns the simplified trade list for the given profession.
func tradesForProfession(profession string) []Trade {
	switch profession {
	case "farmer":
		return []Trade{
			makeTrade("wheat", 20, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("carrot", 22, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 1, "", 0, "bread", 6, 12, 1, 0.05),
		}
	case "librarian":
		return []Trade{
			makeTrade("paper", 24, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 9, "", 0, "bookshelf", 1, 12, 1, 0.05),
			makeTrade("emerald", 5, "book", 1, "enchanted_book", 1, 12, 1, 0.05),
		}
	case "armorer":
		return []Trade{
			makeTrade("iron_ingot", 4, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 7, "", 0, "iron_chestplate", 1, 12, 1, 0.05),
			makeTrade("emerald", 3, "", 0, "iron_helmet", 1, 12, 1, 0.05),
		}
	case "butcher":
		return []Trade{
			makeTrade("beef", 10, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("chicken", 14, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 1, "", 0, "cooked_beef", 5, 12, 1, 0.05),
		}
	case "cleric":
		return []Trade{
			makeTrade("rotten_flesh", 32, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("gold_ingot", 3, "", 0, "emerald", 1, 12, 2, 0.05),
			makeTrade("emerald", 5, "", 0, "ender_pearl", 1, 12, 1, 0.05),
		}
	case "cartographer":
		return []Trade{
			makeTrade("paper", 24, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 7, "", 0, "glass_pane", 16, 12, 1, 0.05),
			makeTrade("emerald", 13, "compass", 1, "filled_map", 1, 12, 1, 0.05),
		}
	case "fisherman":
		return []Trade{
			makeTrade("string", 20, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("cod", 15, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 3, "", 0, "cooked_cod", 6, 12, 1, 0.05),
		}
	case "fletcher":
		return []Trade{
			makeTrade("stick", 32, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 1, "", 0, "arrow", 16, 12, 1, 0.05),
			makeTrade("emerald", 2, "", 0, "bow", 1, 12, 1, 0.05),
		}
	case "leatherworker":
		return []Trade{
			makeTrade("leather", 6, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("rabbit_hide", 10, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 7, "", 0, "leather_leggings", 1, 12, 1, 0.05),
		}
	case "mason":
		return []Trade{
			makeTrade("clay_ball", 10, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 1, "", 0, "brick", 10, 16, 1, 0.05),
			makeTrade("emerald", 1, "", 0, "stone", 16, 16, 1, 0.05),
		}
	case "shepherd":
		return []Trade{
			makeTrade("white_wool", 18, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 2, "", 0, "shears", 1, 12, 1, 0.05),
			makeTrade("emerald", 1, "", 0, "white_bed", 1, 12, 1, 0.05),
		}
	case "toolsmith":
		return []Trade{
			makeTrade("iron_ingot", 4, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 1, "", 0, "stone_axe", 1, 12, 1, 0.05),
			makeTrade("emerald", 3, "", 0, "iron_pickaxe", 1, 12, 1, 0.05),
		}
	case "weaponsmith":
		return []Trade{
			makeTrade("iron_ingot", 4, "", 0, "emerald", 1, 16, 2, 0.05),
			makeTrade("emerald", 2, "", 0, "iron_sword", 1, 12, 1, 0.05),
			makeTrade("diamond", 3, "emerald", 8, "diamond_sword", 1, 3, 5, 0.2),
		}
	}
	return nil
}

// makeTrade is a helper to build a Trade from item names and counts.
func makeTrade(input1Name string, input1Count int32, input2Name string, input2Count int32, outputName string, outputCount int32, maxUses int32, xp int32, priceMult float32) Trade {
	t := Trade{
		InputItem1:      game.ItemStack{ID: itemIDByName(input1Name), Count: input1Count},
		OutputItem:      NewItemStack(itemIDByName(outputName), outputCount),
		MaxUses:         maxUses,
		XP:              xp,
		PriceMultiplier: priceMult,
	}
	if input2Name != "" && input2Count > 0 {
		t.InputItem2 = game.ItemStack{ID: itemIDByName(input2Name), Count: input2Count}
	}
	return t
}

// OpenMerchantUI opens the villager trading UI for a player interacting with a villager.
func (vm *VillagerManager) OpenMerchantUI(player *game.Player, villagerEID int32) {
	vm.MobMgr.mu.Lock()
	mob, ok := vm.MobMgr.Mobs[villagerEID]
	if !ok || mob.Health <= 0 || mob.TypeID != MobTypeVillager || mob.VillagerData == nil {
		vm.MobMgr.mu.Unlock()
		return
	}
	vd := mob.VillagerData
	vm.MobMgr.mu.Unlock()

	// Track this trade session
	vm.mu.Lock()
	vm.openMerchant[player.UUID.String()] = &merchantSession{
		VillagerEID:   villagerEID,
		SelectedTrade: 0, // default to first trade
	}
	vm.mu.Unlock()

	player.OpenWindowID = MerchantWindowID

	// Profession display name (capitalize first letter)
	profName := vd.Profession
	if len(profName) > 0 {
		profName = string(profName[0]-32) + profName[1:]
	}

	// Open merchant screen: menu type 18 = merchant
	title := chat.Text(profName)
	player.WritePacket(pk.Marshal(
		packetid.ClientboundOpenScreen,
		pk.VarInt(MerchantWindowID),
		pk.VarInt(18), // menu type: merchant
		title,
	))

	vm.sendMerchantOffers(player, vd)

	// Play trade sound
	BroadcastSound(vm.Manager, SoundVillagerTrade, SoundCategoryNeutral,
		mob.X, mob.Y, mob.Z, 1.0, 1.0)
}

// sendMerchantOffers sends the ClientboundMerchantOffers packet.
// Format: VarInt(windowID) + VarInt(tradeCount) + [trades...] +
//
//	VarInt(villagerLevel) + VarInt(villagerXP) +
//	Boolean(isRegularVillager) + Boolean(canRestock)
//
// Each trade: Slot(inputItem1) + Slot(outputItem) + Slot(inputItem2) +
//
//	Boolean(disabled) + Int(uses) + Int(maxUses) +
//	Int(xp) + Int(specialPrice) + Float(priceMultiplier) + Int(demand)
func (vm *VillagerManager) sendMerchantOffers(player *game.Player, vd *VillagerData) {
	var buf bytes.Buffer

	rep := CalculateReputation(vd.Gossip, player.UUID)

	// Window ID
	pk.VarInt(MerchantWindowID).WriteTo(&buf)

	// Trade count
	pk.VarInt(len(vd.Trades)).WriteTo(&buf)

	// Each trade
	for _, t := range vd.Trades {
		// Input item 1
		slot1 := t.InputItem1.ToSlot()
		slot1.WriteTo(&buf)

		// Output item
		outSlot := t.OutputItem.ToSlot()
		outSlot.WriteTo(&buf)

		// Input item 2
		slot2 := t.InputItem2.ToSlot()
		slot2.WriteTo(&buf)

		// Disabled (trade exhausted)
		disabled := t.Uses >= t.MaxUses
		pk.Boolean(disabled).WriteTo(&buf)

		// Uses
		pk.Int(t.Uses).WriteTo(&buf)

		// Max uses
		pk.Int(t.MaxUses).WriteTo(&buf)

		// XP
		pk.Int(t.XP).WriteTo(&buf)

		// Special price adjustment (gossip reputation: +/-30% cap)
		specialPrice := int32(0)
		if rep != 0 && t.InputItem1.Count > 0 {
			basePrice := float64(t.InputItem1.Count)
			adjust := -float64(rep) * 0.003 * basePrice
			maxAdj := 0.30 * basePrice
			if adjust > maxAdj {
				adjust = maxAdj
			} else if adjust < -maxAdj {
				adjust = -maxAdj
			}
			specialPrice = int32(adjust)
		}
		pk.Int(specialPrice).WriteTo(&buf)

		// Price multiplier
		pk.Float(t.PriceMultiplier).WriteTo(&buf)

		// Demand
		pk.Int(0).WriteTo(&buf)
	}

	// Villager level (1 = novice)
	pk.VarInt(1).WriteTo(&buf)

	// Villager XP
	pk.VarInt(0).WriteTo(&buf)

	// Is regular villager
	pk.Boolean(true).WriteTo(&buf)

	// Can restock
	pk.Boolean(true).WriteTo(&buf)

	player.WritePacket(pk.Packet{
		ID:   int32(packetid.ClientboundMerchantOffers),
		Data: buf.Bytes(),
	})
}

// SendMerchantWindowContent sends the merchant window slots.
// Merchant window layout: 3 trade slots (0=input1, 1=input2, 2=result) + 27 main inv + 9 hotbar = 39 slots.
func SendMerchantWindowContent(player *game.Player) {
	stateID := player.NextStateID()

	slots := make(game.Slot261Array, 39)
	// Merchant slots 0-2 are empty (server-managed trade slots)
	slots[0] = game.Slot261{} // input 1
	slots[1] = game.Slot261{} // input 2
	slots[2] = game.Slot261{} // result

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
		pk.UnsignedByte(MerchantWindowID),
		pk.VarInt(stateID),
		slots,
		cursor,
	))
}

// MerchantSlot returns a pointer to the item stack for a merchant window slot.
// For merchant trade slots (0-2), returns nil since they are virtual.
func MerchantSlot(player *game.Player, windowSlot int) *game.ItemStack {
	switch {
	case windowSlot >= 0 && windowSlot <= 2:
		return nil // trade slots are virtual, handled by trade execution
	case windowSlot >= 3 && windowSlot <= 29:
		return &player.Inventory[windowSlot-3+9] // main inv: 9..35
	case windowSlot >= 30 && windowSlot <= 38:
		return &player.Inventory[windowSlot-30+36] // hotbar: 36..44
	}
	return nil
}

// HandleSelectTrade processes ServerboundSelectTrade.
// The client sends this when the player clicks on a trade in the merchant UI.
func (vm *VillagerManager) HandleSelectTrade(player *game.Player, p pk.Packet) bool {
	if packetid.ServerboundPacketID(p.ID) != packetid.ServerboundSelectTrade {
		return false
	}

	var tradeIndex pk.VarInt
	if err := p.Scan(&tradeIndex); err != nil {
		return true
	}

	// Store selected trade index on the player for later execution via container click
	vm.mu.Lock()
	session, ok := vm.openMerchant[player.UUID.String()]
	if ok {
		session.SelectedTrade = int(tradeIndex)
	}
	vm.mu.Unlock()
	if !ok {
		return true
	}

	vm.MobMgr.mu.Lock()
	mob, ok := vm.MobMgr.Mobs[session.VillagerEID]
	if !ok || mob.VillagerData == nil {
		vm.MobMgr.mu.Unlock()
		return true
	}
	vd := mob.VillagerData

	idx := int(tradeIndex)
	if idx < 0 || idx >= len(vd.Trades) {
		vm.MobMgr.mu.Unlock()
		return true
	}

	trade := &vd.Trades[idx]
	vm.MobMgr.mu.Unlock()

	// Show the result in the merchant result slot
	resultSlot := trade.OutputItem.ToSlot()
	player.WritePacket(pk.Marshal(
		packetid.ClientboundContainerSetSlot,
		pk.UnsignedByte(MerchantWindowID),
		pk.VarInt(0),
		pk.Short(2), // slot 2 = result
		resultSlot,
	))

	return true
}

// ExecuteTrade executes a villager trade when the player takes the result.
// Called from the inventory handler when a click happens on slot 2 (result) of the merchant window.
func (vm *VillagerManager) ExecuteTrade(player *game.Player, tradeIndex int) {
	vm.mu.Lock()
	session, ok := vm.openMerchant[player.UUID.String()]
	vm.mu.Unlock()
	if !ok {
		return
	}
	villagerEID := session.VillagerEID

	vm.MobMgr.mu.Lock()
	mob, ok := vm.MobMgr.Mobs[villagerEID]
	if !ok || mob.VillagerData == nil {
		vm.MobMgr.mu.Unlock()
		return
	}
	vd := mob.VillagerData

	if tradeIndex < 0 || tradeIndex >= len(vd.Trades) {
		vm.MobMgr.mu.Unlock()
		return
	}
	trade := &vd.Trades[tradeIndex]

	// Check trade not exhausted
	if trade.Uses >= trade.MaxUses {
		vm.MobMgr.mu.Unlock()
		BroadcastSound(vm.Manager, SoundVillagerNo, SoundCategoryNeutral, mob.X, mob.Y, mob.Z, 1.0, 1.0)
		return
	}
	mobX, mobY, mobZ := mob.X, mob.Y, mob.Z
	vm.MobMgr.mu.Unlock()

	// Validate the player has the required input items
	if !vm.playerHasItems(player, trade.InputItem1) {
		BroadcastSound(vm.Manager, SoundVillagerNo, SoundCategoryNeutral, mobX, mobY, mobZ, 1.0, 1.0)
		return
	}
	if trade.InputItem2.ID > 0 && trade.InputItem2.Count > 0 {
		if !vm.playerHasItems(player, trade.InputItem2) {
			BroadcastSound(vm.Manager, SoundVillagerNo, SoundCategoryNeutral, mobX, mobY, mobZ, 1.0, 1.0)
			return
		}
	}

	// Consume input items
	vm.consumeItems(player, trade.InputItem1)
	if trade.InputItem2.ID > 0 && trade.InputItem2.Count > 0 {
		vm.consumeItems(player, trade.InputItem2)
	}

	// Give output item
	outputID := trade.OutputItem.ID
	outputCount := trade.OutputItem.Count
	slot := player.Inventory.AddItem(outputID, outputCount)
	if slot >= 0 {
		SendSlotUpdate(player, slot)
	}

	vm.MobMgr.mu.Lock()
	if mob, ok := vm.MobMgr.Mobs[villagerEID]; ok && mob.VillagerData != nil {
		if tradeIndex < len(mob.VillagerData.Trades) {
			mob.VillagerData.Trades[tradeIndex].Uses++
		}
		mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipTrading, player.UUID, 2, vm.MobMgr.currentTick)
		mob.VillagerData.Gossip = AddGossip(mob.VillagerData.Gossip, GossipMinorPositive, player.UUID, 1, vm.MobMgr.currentTick)
	}
	vm.MobMgr.mu.Unlock()

	// Award XP to player
	AddExperience(player, trade.XP)

	// Play yes sound
	BroadcastSound(vm.Manager, SoundVillagerYes, SoundCategoryNeutral, mobX, mobY, mobZ, 1.0, 1.0)

	// Resend offers with updated uses
	vm.MobMgr.mu.Lock()
	if mob, ok := vm.MobMgr.Mobs[villagerEID]; ok && mob.VillagerData != nil {
		vdUpdated := mob.VillagerData
		vm.MobMgr.mu.Unlock()
		vm.sendMerchantOffers(player, vdUpdated)
	} else {
		vm.MobMgr.mu.Unlock()
	}

	// Sync full inventory
	SendFullInventory(player)
}

// playerHasItems checks if the player has at least the given item stack in their inventory (slots 9-44).
func (vm *VillagerManager) playerHasItems(player *game.Player, required game.ItemStack) bool {
	remaining := required.Count
	for i := 9; i <= 44; i++ {
		if player.Inventory[i].ID == required.ID && player.Inventory[i].Count > 0 {
			remaining -= player.Inventory[i].Count
			if remaining <= 0 {
				return true
			}
		}
	}
	return remaining <= 0
}

// consumeItems removes the given amount of an item from the player's inventory (slots 9-44).
func (vm *VillagerManager) consumeItems(player *game.Player, required game.ItemStack) {
	remaining := required.Count
	for i := 9; i <= 44; i++ {
		if remaining <= 0 {
			break
		}
		inv := &player.Inventory[i]
		if inv.ID == required.ID && inv.Count > 0 {
			if inv.Count <= remaining {
				remaining -= inv.Count
				*inv = game.ItemStack{}
			} else {
				inv.Count -= remaining
				remaining = 0
			}
		}
	}
}

// OnMerchantClose is called when the player closes a merchant window.
func (vm *VillagerManager) OnMerchantClose(player *game.Player) {
	vm.mu.Lock()
	delete(vm.openMerchant, player.UUID.String())
	vm.mu.Unlock()
}

// IsVillager returns true if the given entity ID belongs to a villager mob.
func (vm *VillagerManager) IsVillager(eid int32) bool {
	vm.MobMgr.mu.Lock()
	defer vm.MobMgr.mu.Unlock()
	mob, ok := vm.MobMgr.Mobs[eid]
	return ok && mob.TypeID == MobTypeVillager
}

// GetOpenTradeIndex returns the currently selected trade index for the player, or -1.
// The index is set by HandleSelectTrade when the client selects a trade in the UI.
func (vm *VillagerManager) GetOpenTradeIndex(player *game.Player) int {
	vm.mu.Lock()
	session, ok := vm.openMerchant[player.UUID.String()]
	vm.mu.Unlock()
	if !ok {
		return -1
	}
	return session.SelectedTrade
}

// TickRestock refreshes villager trades during the day, matching vanilla behavior.
// Villagers restock up to 2 times per day, with at least 2400 ticks between restocks.
// At midnight (time crosses 18000), the daily restock counter resets.
func (vm *VillagerManager) TickRestock(tick int64, timeMgr *TimeManager) {
	dayTime := timeMgr.GetDayTime()
	isDaytime := dayTime < 13000

	vm.MobMgr.mu.Lock()
	defer vm.MobMgr.mu.Unlock()

	for _, mob := range vm.MobMgr.Mobs {
		if mob.TypeID != MobTypeVillager || mob.VillagerData == nil || mob.Health <= 0 {
			continue
		}
		vd := mob.VillagerData

		// Reset daily counter at midnight (dayTime wraps past 18000).
		// We detect this by checking if dayTime is in the range [18000, 18020)
		// since this tick runs once per server tick.
		if dayTime >= 18000 && dayTime < 18020 && vd.RestocksToday > 0 {
			vd.RestocksToday = 0
		}

		// Only restock during daytime
		if !isDaytime {
			continue
		}

		// Max 2 restocks per day
		if vd.RestocksToday >= 2 {
			continue
		}

		// At least 2400 ticks since last restock
		if tick-vd.LastRestockTick < 2400 {
			continue
		}

		// Check if any trade actually needs restocking
		needsRestock := false
		for i := range vd.Trades {
			if vd.Trades[i].Uses > 0 {
				needsRestock = true
				break
			}
		}
		if !needsRestock {
			continue
		}

		// Restock: reset all trade uses
		for i := range vd.Trades {
			vd.Trades[i].Uses = 0
		}
		vd.RestocksToday++
		vd.LastRestockTick = tick
	}
}
