package main

import (
	"fmt"
	"testing"

	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
)

func TestStoneID(t *testing.T) {
	stoneID, ok := block.ToStateID[block.Stone{}]
	t.Logf("Stone StateID: %d, found: %v", stoneID, ok)

	// Create a chunk with stone at Y=99
	chunk := level.EmptyChunk(24)
	sec := &chunk.Sections[10]
	sec.SetBlock(3*256+0*16+0, stoneID) // one stone block at (0,3,0) in section 10

	t.Logf("Section 10 block count: %d", sec.BlockCount)
	t.Logf("Section 10 block at index 3*256+0*16+0: %d", sec.GetBlock(3*256+0*16+0))

	data, err := chunk.Data()
	if err != nil {
		t.Fatalf("chunk.Data() error: %v", err)
	}
	t.Logf("Chunk data size: %d bytes", len(data))

	// Verify non-empty section has different data than empty
	emptyChunk := level.EmptyChunk(24)
	emptyData, _ := emptyChunk.Data()
	t.Logf("Empty chunk data size: %d bytes", len(emptyData))

	if len(data) == len(emptyData) {
		t.Log("WARNING: chunk with stone has same size as empty chunk!")
	} else {
		t.Logf("Data size difference: %d bytes (stone chunk is larger)", len(data)-len(emptyData))
	}

	// Print all blocks from the stone section
	fmt.Printf("All non-air blocks in section 10:\n")
	count := 0
	for i := 0; i < 16*16*16; i++ {
		b := sec.GetBlock(i)
		if b != 0 {
			count++
		}
	}
	fmt.Printf("  Non-air block count: %d\n", count)
}

