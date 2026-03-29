package handler

import (
	"strings"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// JukeboxManager handles jukebox disc insertion, playback, and ejection.
type JukeboxManager struct {
	Manager      *game.PlayerManager
	World        game.World
	ItemEntities *ItemEntityManager
	mu           sync.Mutex
	jukeboxes    map[[3]int]int32 // position → disc item ID (0 = empty)
}

// NewJukeboxManager creates a new JukeboxManager.
func NewJukeboxManager(mgr *game.PlayerManager, world game.World, items *ItemEntityManager) *JukeboxManager {
	return &JukeboxManager{
		Manager:      mgr,
		World:        world,
		ItemEntities: items,
		jukeboxes:    make(map[[3]int]int32),
	}
}

// InteractJukebox handles right-click on a jukebox block.
func (jm *JukeboxManager) InteractJukebox(player *game.Player, x, y, z int) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	pos := [3]int{x, y, z}
	discID := jm.jukeboxes[pos]

	if discID != 0 {
		// Eject current disc
		jm.ejectDiscLocked(pos, discID, x, y, z)
		return true
	}

	// Try to insert disc from player's held item
	slot := player.HeldSlot
	held := &player.Inventory[36+slot]
	if held.ID == 0 || held.Count == 0 {
		return false
	}
	itemName := ItemNameByID(held.ID)
	if !isMusicDisc(itemName) {
		return false
	}

	// Insert disc
	jm.jukeboxes[pos] = held.ID

	// Consume item from hand
	held.Count--
	if held.Count <= 0 {
		*held = game.ItemStack{}
	}
	SendSlotUpdate(player, 36+int(slot))

	// Update block state to has_record=true
	jm.setHasRecord(x, y, z, true)

	// Play music disc sound
	soundID := musicDiscSoundForItem(itemName)
	if soundID > 0 {
		BroadcastSound(jm.Manager, soundID, SoundCategoryRecord,
			float64(x)+0.5, float64(y)+1.0, float64(z)+0.5, 4.0, 1.0)
	}

	return true
}

// ejectDiscLocked ejects a disc from a jukebox (caller must hold jm.mu).
func (jm *JukeboxManager) ejectDiscLocked(pos [3]int, discID int32, x, y, z int) {
	delete(jm.jukeboxes, pos)

	// Update block state
	jm.setHasRecord(x, y, z, false)

	// Spawn disc as item entity
	if jm.ItemEntities != nil && discID > 0 {
		jm.ItemEntities.SpawnItem(jm.Manager, float64(x)+0.5, float64(y)+1.0, float64(z)+0.5, discID, 1, 10)
	}
}

// OnJukeboxBreak should be called when a jukebox block is broken to eject any disc.
func (jm *JukeboxManager) OnJukeboxBreak(x, y, z int) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	pos := [3]int{x, y, z}
	discID := jm.jukeboxes[pos]
	if discID != 0 {
		jm.ejectDiscLocked(pos, discID, x, y, z)
	}
}

// setHasRecord updates the jukebox block state.
func (jm *JukeboxManager) setHasRecord(x, y, z int, hasRecord bool) {
	stateID, err := jm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	jb, ok := block.StateList[stateID].(block.Jukebox)
	if !ok {
		return
	}
	jb.Has_record = block.Boolean(hasRecord)
	if newID, ok2 := block.ToStateID[jb]; ok2 {
		jm.World.SetBlock(x, y, z, newID)
		broadcastBlockUpdateDirect(jm.Manager, x, y, z, int32(newID))
	}
}

// isMusicDisc returns true if the item name is a music disc.
func isMusicDisc(name string) bool {
	return strings.HasPrefix(name, "music_disc_")
}

// musicDiscSoundForItem maps a music disc item name to its sound ID.
func musicDiscSoundForItem(name string) int32 {
	switch name {
	case "music_disc_11":
		return 616
	case "music_disc_13":
		return 617
	case "music_disc_blocks":
		return 618
	case "music_disc_cat":
		return 619
	case "music_disc_chirp":
		return 620
	case "music_disc_far":
		return 621
	case "music_disc_mall":
		return 622
	case "music_disc_mellohi":
		return 623
	case "music_disc_pigstep":
		return 624
	case "music_disc_stal":
		return 625
	case "music_disc_strad":
		return 626
	case "music_disc_wait":
		return 627
	case "music_disc_ward":
		return 628
	default:
		return -1
	}
}
