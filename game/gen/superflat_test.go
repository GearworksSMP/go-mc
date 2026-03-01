package gen

import (
	"testing"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

func TestDefaultSuperflat(t *testing.T) {
	gen := DefaultSuperflat()

	if gen.Sections != 24 {
		t.Errorf("Sections = %d, want 24", gen.Sections)
	}
	if gen.MinY != -64 {
		t.Errorf("MinY = %d, want -64", gen.MinY)
	}
	if len(gen.Layers) != 3 {
		t.Errorf("Layers = %d, want 3", len(gen.Layers))
	}

	// Check total height: bedrock(1) + dirt(2) + grass_block(1) = 4
	topY := gen.TopBlockY()
	if topY != -61 {
		t.Errorf("TopBlockY = %d, want -61", topY)
	}

	spawnY := gen.SpawnY()
	if spawnY != -60 {
		t.Errorf("SpawnY = %f, want -60", spawnY)
	}
}

func TestSuperflatGenerate(t *testing.T) {
	gen := DefaultSuperflat()
	chunk := gen.Generate(game.ChunkPos{X: 0, Z: 0})

	if len(chunk.Sections) != 24 {
		t.Fatalf("Sections = %d, want 24", len(chunk.Sections))
	}

	bedrockID, _ := block.ToStateID[block.Bedrock{}]
	dirtID, _ := block.ToStateID[block.Dirt{}]
	grassID, _ := block.ToStateID[block.GrassBlock{Snowy: false}]

	// Check specific blocks
	// Section 0, Y=-64 (localY=0): bedrock
	sec0 := &chunk.Sections[0]
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			idx := 0*256 + z*16 + x // localY=0
			got := sec0.GetBlock(idx)
			if got != bedrockID {
				t.Errorf("Block at (%d, -64, %d) = %d, want bedrock (%d)", x, z, got, bedrockID)
			}
		}
	}

	// Y=-63 (localY=1): dirt
	for x := 0; x < 16; x++ {
		got := sec0.GetBlock(1*256 + 0*16 + x) // localY=1, z=0
		if got != dirtID {
			t.Errorf("Block at (%d, -63, 0) = %d, want dirt (%d)", x, got, dirtID)
		}
	}

	// Y=-62 (localY=2): dirt
	got := sec0.GetBlock(2*256 + 0*16 + 0)
	if got != dirtID {
		t.Errorf("Block at (0, -62, 0) = %d, want dirt (%d)", got, dirtID)
	}

	// Y=-61 (localY=3): grass_block
	got = sec0.GetBlock(3*256 + 0*16 + 0)
	if got != grassID {
		t.Errorf("Block at (0, -61, 0) = %d, want grass_block (%d)", got, grassID)
	}

	// Y=-60 (localY=4): air
	got = sec0.GetBlock(4*256 + 0*16 + 0)
	if got != 0 {
		t.Errorf("Block at (0, -60, 0) = %d, want air (0)", got)
	}

	// Section 0 block count: 4 layers * 256 = 1024
	if sec0.BlockCount != 1024 {
		t.Errorf("Section 0 BlockCount = %d, want 1024", sec0.BlockCount)
	}

	// All other sections should be empty
	for i := 1; i < 24; i++ {
		if chunk.Sections[i].BlockCount != 0 {
			t.Errorf("Section %d BlockCount = %d, want 0", i, chunk.Sections[i].BlockCount)
		}
	}

	// Heightmaps should be set
	if chunk.HeightMaps.WorldSurface == nil {
		t.Error("WorldSurface heightmap is nil")
	}
	if chunk.HeightMaps.MotionBlocking == nil {
		t.Error("MotionBlocking heightmap is nil")
	}

	// Heightmap value at (0,0): topY - minY + 1 = -61 - (-64) + 1 = 4
	hmVal := chunk.HeightMaps.WorldSurface.Get(0)
	if hmVal != 4 {
		t.Errorf("WorldSurface heightmap at (0,0) = %d, want 4", hmVal)
	}
}

func TestVoidGenerator(t *testing.T) {
	vgen := &VoidGenerator{Sections: 24}
	chunk := vgen.Generate(game.ChunkPos{X: 3, Z: 7})

	if len(chunk.Sections) != 24 {
		t.Fatalf("Sections = %d, want 24", len(chunk.Sections))
	}

	for i, sec := range chunk.Sections {
		if sec.BlockCount != 0 {
			t.Errorf("Void section %d BlockCount = %d, want 0", i, sec.BlockCount)
		}
	}
}
