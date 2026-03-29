package handler

import (
	"log"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// Sound IDs for respawn anchor interactions.
const (
	SoundRespawnAnchorCharge   int32 = 862
	SoundRespawnAnchorDeplete  int32 = 863
	SoundRespawnAnchorSetSpawn int32 = 864
)

// RespawnAnchorManager handles respawn anchor block interactions:
// charging with glowstone, setting Nether spawn, and exploding outside the Nether.
type RespawnAnchorManager struct {
	Manager  *game.PlayerManager
	World    game.World
	TNTMgr   *TNTManager
	Survival *SurvivalHandler
	Logger   *log.Logger
}

// HandleInteraction processes a right-click on a respawn anchor block.
// Returns true if the interaction was handled.
func (rm *RespawnAnchorManager) HandleInteraction(player *game.Player, x, y, z int, anchor block.RespawnAnchor) bool {
	charges := int(anchor.Charges)

	// Check if the player is holding glowstone
	heldItemID := player.HeldItemID()
	holdingGlowstone := false
	if heldItemID > 0 {
		heldName := ItemNameByID(heldItemID)
		if heldName == "glowstone" {
			holdingGlowstone = true
		}
	}

	// If holding glowstone and charges < 4, charge the anchor
	if holdingGlowstone && charges < 4 {
		rm.charge(player, x, y, z, anchor)
		return true
	}

	// If not in the Nether, explode
	if player.Dimension != "minecraft:the_nether" {
		rm.explode(player, x, y, z)
		return true
	}

	// In the Nether with charges > 0 and not holding glowstone: set spawn and consume a charge
	if charges > 0 && !holdingGlowstone {
		rm.setSpawn(player, x, y, z, anchor)
		return true
	}

	return false
}

// charge adds one charge to the respawn anchor and consumes a glowstone from the player's hand.
func (rm *RespawnAnchorManager) charge(player *game.Player, x, y, z int, anchor block.RespawnAnchor) {
	anchor.Charges++
	newID, ok := block.ToStateID[anchor]
	if !ok {
		return
	}

	rm.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(rm.Manager, x, y, z, int32(newID))

	// Consume glowstone in survival mode
	if player.GameMode == 0 {
		slot := int(player.HeldSlot) + 36
		player.Inventory[slot].Count--
		if player.Inventory[slot].Count <= 0 {
			player.Inventory[slot] = game.ItemStack{}
		}
		SendSlotUpdate(player, slot)
	}

	BroadcastSound(rm.Manager, SoundRespawnAnchorCharge, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)

	rm.logf("Respawn anchor at (%d,%d,%d) charged to %d by %s", x, y, z, int(anchor.Charges), player.Name)
}

// setSpawn sets the player's spawn point at this respawn anchor and consumes one charge.
func (rm *RespawnAnchorManager) setSpawn(player *game.Player, x, y, z int, anchor block.RespawnAnchor) {
	anchor.Charges--
	newID, ok := block.ToStateID[anchor]
	if !ok {
		return
	}

	rm.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(rm.Manager, x, y, z, int32(newID))

	// Set the player's spawn point to the anchor location
	player.SpawnX = float64(x) + 0.5
	player.SpawnY = float64(y) + 1.0
	player.SpawnZ = float64(z) + 0.5

	BroadcastSound(rm.Manager, SoundRespawnAnchorSetSpawn, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 1.0, 1.0)

	rm.logf("Player %s set Nether spawn at (%d,%d,%d), charges remaining: %d", player.Name, x, y, z, int(anchor.Charges))
}

// explode detonates the respawn anchor when used outside the Nether.
// This reuses the TNT explosion logic for block destruction and player damage.
func (rm *RespawnAnchorManager) explode(player *game.Player, x, y, z int) {
	// Remove the respawn anchor block (set to air)
	oldState, err := rm.World.SetBlock(x, y, z, 0)
	if err != nil {
		return
	}
	broadcastBlockUpdateDirect(rm.Manager, x, y, z, 0)

	if oldState > 0 {
		BroadcastLevelEvent(rm.Manager, 2001, x, y, z, int32(oldState))
	}

	// Reuse TNT explosion if TNTManager is available
	if rm.TNTMgr != nil {
		// Create a temporary PrimedTNT at the anchor position and call explode
		tnt := &PrimedTNT{
			X: float64(x) + 0.5,
			Y: float64(y),
			Z: float64(z) + 0.5,
		}
		rm.TNTMgr.mu.Lock()
		rm.TNTMgr.explode(tnt)
		rm.TNTMgr.mu.Unlock()
	} else {
		// Fallback: just play explosion sound
		BroadcastSound(rm.Manager, SoundExplode, SoundCategoryBlock,
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5, 4.0, 1.0)
	}

	rm.logf("Respawn anchor exploded at (%d,%d,%d) — used outside Nether by %s", x, y, z, player.Name)
}

func (rm *RespawnAnchorManager) logf(format string, args ...any) {
	if rm.Logger != nil {
		rm.Logger.Printf(format, args...)
	}
}
