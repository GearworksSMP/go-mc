package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// NoteBlockManager handles note block tuning, playback, and ambient biome sounds.
type NoteBlockManager struct {
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex
	// ambientTimers tracks the next tick each player should hear an ambient sound.
	// Key is player EID.
	ambientTimers map[int32]int64
}

// NewNoteBlockManager creates a new NoteBlockManager.
func NewNoteBlockManager(manager *game.PlayerManager, world game.World, logger *log.Logger) *NoteBlockManager {
	return &NoteBlockManager{
		Manager:       manager,
		World:         world,
		Logger:        logger,
		ambientTimers: make(map[int32]int64),
	}
}

// PlayNote plays the note block at the given position without changing its pitch.
func (nm *NoteBlockManager) PlayNote(x, y, z int) {
	stateID, err := nm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	nb, ok := block.StateList[stateID].(block.NoteBlock)
	if !ok {
		return
	}
	// Check that the block above is air (note blocks require air above to play).
	aboveID, err := nm.World.GetBlock(x, y+1, z)
	if err == nil {
		aboveName := BlockNameFromState(int(aboveID))
		if aboveName != "air" && aboveName != "cave_air" && aboveName != "void_air" {
			return
		}
	}
	nm.playNoteSound(x, y, z, nb)
}

// TuneNote increments the note block pitch (mod 25), updates the block state,
// and plays the new note.
func (nm *NoteBlockManager) TuneNote(x, y, z int) {
	stateID, err := nm.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}
	nb, ok := block.StateList[stateID].(block.NoteBlock)
	if !ok {
		return
	}

	// Cycle note 0 -> 1 -> ... -> 24 -> 0
	newNote := (int(nb.Note) + 1) % 25
	nb.Note = block.Integer(newNote)
	newID, ok := block.ToStateID[nb]
	if !ok {
		return
	}
	nm.World.SetBlock(x, y, z, newID)
	broadcastBlockUpdateDirect(nm.Manager, x, y, z, int32(newID))

	nm.PlayNote(x, y, z)
}

// OnRedstone plays the note block when it receives redstone power.
func (nm *NoteBlockManager) OnRedstone(x, y, z int) {
	nm.PlayNote(x, y, z)
}

// playNoteSound broadcasts the note block sound and block event particle.
func (nm *NoteBlockManager) playNoteSound(x, y, z int, nb block.NoteBlock) {
	note := int(nb.Note)
	pitch := float32(math.Pow(2.0, float64(note-12)/12.0))

	soundID := noteBlockInstrumentSoundID(nb.Instrument)
	BroadcastSound(nm.Manager, soundID, SoundCategoryBlock,
		float64(x)+0.5, float64(y)+1.0, float64(z)+0.5, 3.0, pitch)

	// Send block event (action 0 = play note) for note particle.
	blockStateID, _ := nm.World.GetBlock(x, y, z)
	noteEventPkt := pk.Marshal(
		packetid.ClientboundBlockEvent,
		pk.Position{X: x, Y: y, Z: z},
		pk.UnsignedByte(0),          // action: play note
		pk.UnsignedByte(byte(note)), // note value (determines particle color)
		pk.VarInt(blockStateID),
	)
	nm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(noteEventPkt)
	})
}

// noteBlockInstrumentSoundID maps a NoteBlockInstrument to its sound ID.
func noteBlockInstrumentSoundID(inst block.NoteBlockInstrument) int32 {
	switch inst {
	case block.NoteBlockInstrumentHarp:
		return SoundNoteBlockHarp
	case block.NoteBlockInstrumentBasedrum:
		return SoundNoteBlockBasedrum
	case block.NoteBlockInstrumentSnare:
		return SoundNoteBlockSnare
	case block.NoteBlockInstrumentHat:
		return SoundNoteBlockHat
	case block.NoteBlockInstrumentBass:
		return SoundNoteBlockBass
	case block.NoteBlockInstrumentFlute:
		return SoundNoteBlockFlute
	case block.NoteBlockInstrumentBell:
		return SoundNoteBlockBell
	case block.NoteBlockInstrumentGuitar:
		return SoundNoteBlockGuitar
	case block.NoteBlockInstrumentChime:
		return SoundNoteBlockChime
	case block.NoteBlockInstrumentXylophone:
		return SoundNoteBlockXylophone
	case block.NoteBlockInstrumentIronXylophone:
		return SoundNoteBlockIronXylophone
	case block.NoteBlockInstrumentCowBell:
		return SoundNoteBlockCowBell
	case block.NoteBlockInstrumentDidgeridoo:
		return SoundNoteBlockDidgeridoo
	case block.NoteBlockInstrumentBit:
		return SoundNoteBlockBit
	case block.NoteBlockInstrumentBanjo:
		return SoundNoteBlockBanjo
	case block.NoteBlockInstrumentPling:
		return SoundNoteBlockPling
	default:
		return SoundNoteBlockHarp // fallback for mob head instruments
	}
}

// ---------------------------------------------------------------------------
// Ambient Biome Sounds
// ---------------------------------------------------------------------------

// TickAmbientSounds plays ambient biome sounds for players periodically.
// Call this from the main tick loop.
func (nm *NoteBlockManager) TickAmbientSounds(tick int64) {
	nm.Manager.ForEach(func(p *game.Player) {
		if p.Dead {
			return
		}

		nm.mu.Lock()
		nextTick, exists := nm.ambientTimers[p.EID]
		if !exists {
			// Initialize with a random delay of 200-400 ticks.
			nm.ambientTimers[p.EID] = tick + 200 + int64(rand.Intn(201))
			nm.mu.Unlock()
			return
		}
		if tick < nextTick {
			nm.mu.Unlock()
			return
		}
		// Schedule the next ambient sound 200-400 ticks from now.
		nm.ambientTimers[p.EID] = tick + 200 + int64(rand.Intn(201))
		nm.mu.Unlock()

		px, py, pz := p.Position()

		// Cave ambient: play when player is below Y=50.
		if py < 50 {
			sendSoundToPlayer(p, SoundAmbientCave, SoundCategoryAmbient,
				px, py, pz, 1.0, 1.0)
			return
		}

		// Underwater ambient: play when player head is submerged.
		// Check the block at the player's head position.
		headY := int(py) + 1
		headStateID, err := nm.World.GetBlock(int(px), headY, int(pz))
		if err == nil {
			headName := BlockNameFromState(int(headStateID))
			if headName == "water" {
				sendSoundToPlayer(p, SoundAmbientUnderwaterLoop, SoundCategoryAmbient,
					px, py, pz, 1.0, 1.0)
				return
			}
		}
	})
}

// CleanupPlayer removes ambient timer state for a disconnected player.
func (nm *NoteBlockManager) CleanupPlayer(eid int32) {
	nm.mu.Lock()
	delete(nm.ambientTimers, eid)
	nm.mu.Unlock()
}

// sendSoundToPlayer sends a ClientboundSound packet to a single player.
func sendSoundToPlayer(p *game.Player, soundID, category int32, x, y, z float64, volume, pitch float32) {
	pkt := pk.Marshal(
		packetid.ClientboundSound,
		pk.VarInt(soundID+1),  // sound ID (+1 because 0 = custom)
		pk.VarInt(category),   // category
		pk.Int(int32(x*8)),    // fixed-point x
		pk.Int(int32(y*8)),    // fixed-point y
		pk.Int(int32(z*8)),    // fixed-point z
		pk.Float(volume),      // volume
		pk.Float(pitch),       // pitch
		pk.Long(rand.Int63()), // seed
	)
	p.WritePacket(pkt)
}
