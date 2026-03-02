package handler

import (
	"math"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// FallingBlock represents a block that is falling (sand, gravel).
type FallingBlock struct {
	EID     int32
	StateID level.BlocksState
	X, Y, Z float64
	VelY    float64 // downward velocity (negative = falling)
}

// FallingBlockManager handles gravity for sand/gravel blocks.
type FallingBlockManager struct {
	World   game.World
	Manager *game.PlayerManager
	mu      sync.Mutex
	blocks  map[int32]*FallingBlock
}

// NewFallingBlockManager creates a new FallingBlockManager.
func NewFallingBlockManager(world game.World, manager *game.PlayerManager) *FallingBlockManager {
	return &FallingBlockManager{
		World:   world,
		Manager: manager,
		blocks:  make(map[int32]*FallingBlock),
	}
}

// isFallingBlock returns true if the block name is a gravity-affected block.
func isFallingBlock(blockName string) bool {
	switch blockName {
	case "sand", "gravel", "red_sand":
		return true
	}
	return false
}

// CheckAndSpawnFalling checks if the block at (x,y,z) should fall, and spawns a falling entity.
func (f *FallingBlockManager) CheckAndSpawnFalling(x, y, z int) {
	state, err := f.World.GetBlock(x, y, z)
	if err != nil || state == 0 {
		return
	}
	blockName := BlockNameFromState(int(state))
	if !isFallingBlock(blockName) {
		return
	}

	// Check block below is air
	below, err := f.World.GetBlock(x, y-1, z)
	if err != nil || below != 0 {
		return
	}

	// Remove the block from the world
	f.World.SetBlock(x, y, z, 0)
	f.broadcastBlockUpdate(x, y, z, 0)

	// Spawn falling entity
	f.mu.Lock()
	eid := f.Manager.NextEntityID()
	fb := &FallingBlock{
		EID:     eid,
		StateID: state,
		X:       float64(x) + 0.5,
		Y:       float64(y),
		Z:       float64(z) + 0.5,
		VelY:    0,
	}
	f.blocks[eid] = fb
	f.mu.Unlock()

	// Entity type 28 = falling_block, data = block state ID
	id := uuid.New()
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(id),
		pk.VarInt(28), // falling_block
		pk.Double(fb.X),
		pk.Double(fb.Y),
		pk.Double(fb.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),
		pk.Angle(0),
		pk.Angle(0),
		pk.VarInt(int32(state)), // data = block state
	)
	f.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// Tick processes all falling blocks. Called every tick.
func (f *FallingBlockManager) Tick(tick int64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var toRemove []int32
	var checkAbove [][3]int

	for eid, fb := range f.blocks {
		// Apply gravity
		fb.VelY -= 0.04
		fb.Y += fb.VelY

		// Check if landed
		landY := int(math.Floor(fb.Y))
		landX := int(math.Floor(fb.X - 0.5))
		landZ := int(math.Floor(fb.Z - 0.5))

		groundState, err := f.World.GetBlock(landX, landY, landZ)
		if err != nil || (groundState != 0 && fb.VelY < 0) {
			// Land: place block on top of ground
			placeY := landY
			if groundState != 0 {
				placeY = landY + 1
			}
			f.World.SetBlock(landX, placeY, landZ, fb.StateID)
			f.broadcastBlockUpdate(landX, placeY, landZ, int32(fb.StateID))

			// Remove entity
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(eid),
			)
			f.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(removePkt)
			})

			toRemove = append(toRemove, eid)
			// Check above landing position for more falling blocks
			checkAbove = append(checkAbove, [3]int{landX, placeY + 1, landZ})
			continue
		}

		// Broadcast position update
		pkt := pk.Marshal(
			packetid.ClientboundTeleportEntity,
			pk.VarInt(eid),
			pk.Double(fb.X),
			pk.Double(fb.Y),
			pk.Double(fb.Z),
			pk.Double(0), pk.Double(fb.VelY*8000), pk.Double(0),
			pk.Float(0),
			pk.Float(0),
			pk.Boolean(false),
		)
		f.Manager.ForEach(func(p *game.Player) {
			p.WritePacket(pkt)
		})
	}

	for _, eid := range toRemove {
		delete(f.blocks, eid)
	}

	// Check for chain reactions (must be done outside the lock since CheckAndSpawnFalling takes the lock)
	if len(checkAbove) > 0 {
		f.mu.Unlock()
		for _, pos := range checkAbove {
			f.CheckAndSpawnFalling(pos[0], pos[1], pos[2])
		}
		f.mu.Lock()
	}
}

func (f *FallingBlockManager) broadcastBlockUpdate(x, y, z int, stateID int32) {
	pkt := pk.Marshal(
		packetid.ClientboundBlockUpdate,
		pk.Position{X: x, Y: y, Z: z},
		pk.VarInt(stateID),
	)
	f.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}

// isSapling returns true if the block name is a sapling type.
func isSapling(blockName string) bool {
	switch blockName {
	case "oak_sapling", "spruce_sapling", "birch_sapling",
		"jungle_sapling", "acacia_sapling", "dark_oak_sapling",
		"cherry_sapling":
		return true
	}
	return false
}

// isSolidBlock returns true if a block state represents a solid block that mobs can't walk through.
func isSolidBlock(stateID level.BlocksState) bool {
	if stateID == 0 {
		return false
	}
	name := BlockNameFromState(int(stateID))
	switch name {
	case "air", "water", "lava", "short_grass", "tall_grass",
		"dandelion", "poppy", "torch", "wall_torch", "snow",
		"oak_sapling", "spruce_sapling", "birch_sapling",
		"jungle_sapling", "acacia_sapling", "dark_oak_sapling",
		"cherry_sapling", "dead_bush", "fern", "large_fern",
		"red_mushroom", "brown_mushroom", "":
		return false
	}
	// Check if it's a fluid with level > 0
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		switch block.StateList[stateID].(type) {
		case block.Water, block.Lava:
			return false
		}
	}
	return true
}
