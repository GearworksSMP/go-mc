package game

import "testing"

func TestBlockToChunk(t *testing.T) {
	tests := []struct {
		x, z       int
		wantCX, CZ int
	}{
		{0, 0, 0, 0},
		{15, 15, 0, 0},
		{16, 16, 1, 1},
		{-1, -1, -1, -1},
		{-16, -16, -1, -1},
		{-17, -17, -2, -2},
		{31, 31, 1, 1},
		{32, 32, 2, 2},
	}

	for _, tt := range tests {
		pos := BlockToChunk(tt.x, tt.z)
		if pos.X != tt.wantCX || pos.Z != tt.CZ {
			t.Errorf("BlockToChunk(%d, %d) = (%d, %d), want (%d, %d)",
				tt.x, tt.z, pos.X, pos.Z, tt.wantCX, tt.CZ)
		}
	}
}

func TestChunkToRegion(t *testing.T) {
	tests := []struct {
		cx, cz     int
		wantRX, RZ int
	}{
		{0, 0, 0, 0},
		{31, 31, 0, 0},
		{32, 32, 1, 1},
		{-1, -1, -1, -1},
		{-32, -32, -1, -1},
		{-33, -33, -2, -2},
	}

	for _, tt := range tests {
		pos := ChunkToRegion(ChunkPos{X: tt.cx, Z: tt.cz})
		if pos.X != tt.wantRX || pos.Z != tt.RZ {
			t.Errorf("ChunkToRegion(%d, %d) = (%d, %d), want (%d, %d)",
				tt.cx, tt.cz, pos.X, pos.Z, tt.wantRX, tt.RZ)
		}
	}
}
