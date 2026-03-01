package game

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/bits"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// HeightmapType corresponds to Minecraft's Heightmap.Types enum ordinals.
const (
	HeightmapWorldSurface           = 1
	HeightmapMotionBlocking         = 4
	HeightmapMotionBlockingNoLeaves = 5
)

// WriteChunkPacket261 encodes a level.Chunk into a 26.1 LevelChunkWithLight packet.
// The minY parameter is the minimum Y coordinate (e.g., -64 for overworld).
func WriteChunkPacket261(pos ChunkPos, chunk *level.Chunk, minY int) (pk.Packet, error) {
	sectionData, err := encodeSections261(chunk)
	if err != nil {
		return pk.Packet{}, err
	}

	heightmapBytes := encodeHeightmaps261(chunk, len(chunk.Sections))
	lightFields := encodeLightData261(chunk)

	fields := make([]pk.FieldEncoder, 0, 10)
	fields = append(fields,
		pk.Int(int32(pos.X)), pk.Int(int32(pos.Z)),
		rawBytes(heightmapBytes),
		pk.ByteArray(sectionData),
		pk.VarInt(0), // block entities: empty list
	)
	fields = append(fields, lightFields...)

	return pk.Marshal(packetid.ClientboundLevelChunkWithLight, fields...), nil
}

// encodeSections261 encodes all chunk sections in 26.1 format.
func encodeSections261(chunk *level.Chunk) ([]byte, error) {
	var buf bytes.Buffer
	for i := range chunk.Sections {
		if err := encodeSection261(&buf, &chunk.Sections[i]); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// encodeSection261 encodes a single section in 26.1 format.
func encodeSection261(w *bytes.Buffer, sec *level.Section) error {
	binary.Write(w, binary.BigEndian, sec.BlockCount)
	if err := encodePaletteContainer261(w, sec.States, statesBitsMapping); err != nil {
		return err
	}
	return encodePaletteContainer261(w, sec.Biomes, biomesBitsMapping)
}

// bitsMapping maps raw bits (from PaletteContainer.bits) to actual storage bits.
// This mirrors the paletteCfg.bits() logic which is unexported.
type bitsMapping func(raw int) int

// statesBitsMapping mirrors statesCfg.bits() — maps raw block palette bits to actual.
func statesBitsMapping(raw int) int {
	switch raw {
	case 0:
		return 0
	case 1, 2, 3, 4:
		return 4
	case 5, 6, 7, 8:
		return raw
	default:
		return block.BitsPerBlock
	}
}

// biomesBitsMapping mirrors biomesCfg.bits() — maps raw biome palette bits to actual.
func biomesBitsMapping(raw int) int {
	switch raw {
	case 0:
		return 0
	case 1, 2, 3:
		return raw
	default:
		// biome.BitsPerBiome — we compute from the biome registry size
		// For vanilla this is typically 6, but we can use a safe fallback
		return raw
	}
}

// encodePaletteContainer261 encodes a PaletteContainer in 26.1 format:
// UnsignedByte(actualBitsPerEntry) + palette + data longs (NO VarInt length prefix)
//
// The go-mc PaletteContainer.WriteTo writes a raw bits value that needs config mapping.
// In the old format, this was fine because BitStorage had a VarInt length prefix.
// In 26.1, the data length is inferred from bitsPerEntry, so we must write the actual bits.
func encodePaletteContainer261[T level.State](w *bytes.Buffer, pc *level.PaletteContainer[T], mapping bitsMapping) error {
	// Capture old-format WriteTo output
	var tmp bytes.Buffer
	if _, err := pc.WriteTo(&tmp); err != nil {
		return err
	}
	data := tmp.Bytes()
	if len(data) == 0 {
		return nil
	}

	rawBits := int(data[0])
	actualBits := mapping(rawBits)

	if actualBits == 0 {
		// Single-value palette: UnsignedByte(0) + VarInt(value)
		// Old output: [0x00, VarInt(value), VarInt(0)]
		// New output: [0x00, VarInt(value)]
		w.WriteByte(0)
		rest := data[1:]
		val, _ := readVarIntBytes(rest)
		writeVarIntBuf(w, val)
		return nil
	}

	// Write actual bits (not raw bits)
	w.WriteByte(byte(actualBits))

	rest := data[1:]

	// Read and copy palette: VarInt(len) + len*VarInt(value)
	paletteLen, n := readVarIntBytes(rest)
	pos := n
	for i := 0; i < paletteLen; i++ {
		_, vn := readVarIntBytes(rest[pos:])
		pos += vn
	}
	// Write palette bytes as-is
	w.Write(rest[:pos])

	// Skip VarInt(dataLongCount) and write raw longs directly
	dataCount, dcn := readVarIntBytes(rest[pos:])
	pos += dcn
	longBytes := dataCount * 8
	if pos+longBytes <= len(rest) {
		w.Write(rest[pos : pos+longBytes])
	}

	return nil
}

// encodeHeightmaps261 encodes heightmaps in the 26.1 stream codec format:
// VarInt(count) + count * [VarInt(typeOrdinal) + VarInt(longCount) + longs]
func encodeHeightmaps261(chunk *level.Chunk, numSections int) []byte {
	var buf bytes.Buffer
	bitsForHeight := bits.Len(uint(numSections)*16 + 1)

	type hmEntry struct {
		typeOrdinal int
		storage     *level.BitStorage
	}

	entries := []hmEntry{
		{HeightmapWorldSurface, chunk.HeightMaps.WorldSurface},
		{HeightmapMotionBlocking, chunk.HeightMaps.MotionBlocking},
		{HeightmapMotionBlockingNoLeaves, chunk.HeightMaps.MotionBlockingNoLeaves},
	}

	var valid []hmEntry
	for _, e := range entries {
		if e.storage == nil {
			e.storage = level.NewBitStorage(bitsForHeight, 16*16, nil)
		}
		valid = append(valid, e)
	}

	writeVarIntBuf(&buf, len(valid))
	for _, e := range valid {
		writeVarIntBuf(&buf, e.typeOrdinal)
		raw := e.storage.Raw()
		writeVarIntBuf(&buf, len(raw))
		for _, l := range raw {
			binary.Write(&buf, binary.BigEndian, int64(l))
		}
	}
	return buf.Bytes()
}

// encodeLightData261 encodes light data for 26.1 (no Trust Edges boolean).
func encodeLightData261(chunk *level.Chunk) []pk.FieldEncoder {
	numLightSections := len(chunk.Sections) + 2

	skyLightMask := make(pk.BitSet, (numLightSections+63)/64)
	blockLightMask := make(pk.BitSet, (numLightSections+63)/64)
	var skyLightArrays []pk.ByteArray
	var blockLightArrays []pk.ByteArray

	for i, sec := range chunk.Sections {
		lightIdx := i + 1
		if len(sec.SkyLight) > 0 {
			skyLightMask.Set(lightIdx, true)
			skyLightArrays = append(skyLightArrays, sec.SkyLight)
		}
		if len(sec.BlockLight) > 0 {
			blockLightMask.Set(lightIdx, true)
			blockLightArrays = append(blockLightArrays, sec.BlockLight)
		}
	}

	// If no sky light data, provide full sky light for all sections
	if len(skyLightArrays) == 0 {
		fullLight := make(pk.ByteArray, 2048)
		for i := range fullLight {
			fullLight[i] = 0xFF
		}
		for i := 0; i < numLightSections; i++ {
			skyLightMask.Set(i, true)
			skyLightArrays = append(skyLightArrays, fullLight)
		}
	}

	emptySkyMask := make(pk.BitSet, len(skyLightMask))
	emptyBlockMask := make(pk.BitSet, len(blockLightMask))

	return []pk.FieldEncoder{
		skyLightMask,
		blockLightMask,
		emptySkyMask,
		emptyBlockMask,
		pk.Array(skyLightArrays),
		arrayOrEmpty(blockLightArrays),
	}
}

func arrayOrEmpty(arr []pk.ByteArray) pk.FieldEncoder {
	if len(arr) == 0 {
		return pk.VarInt(0)
	}
	return pk.Array(arr)
}

// rawBytes is a FieldEncoder that writes its contents directly.
type rawBytes []byte

func (r rawBytes) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r)
	return int64(n), err
}

func writeVarIntBuf(buf *bytes.Buffer, v int) {
	val := uint32(v)
	for val >= 0x80 {
		buf.WriteByte(byte(val&0x7F) | 0x80)
		val >>= 7
	}
	buf.WriteByte(byte(val))
}

func readVarIntBytes(data []byte) (int, int) {
	var result int
	for i := 0; i < 5 && i < len(data); i++ {
		b := data[i]
		result |= int(b&0x7F) << (i * 7)
		if b&0x80 == 0 {
			return result, i + 1
		}
	}
	return result, 0
}
