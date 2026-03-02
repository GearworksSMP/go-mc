package main

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"
	"strings"
)

// Simple byte reader with offset tracking
type reader struct {
	data []byte
	pos  int
	name string
}

func newReader(data []byte, name string) *reader {
	return &reader{data: data, name: name}
}

func (r *reader) remaining() int {
	return len(r.data) - r.pos
}

func (r *reader) peekByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("[%s] EOF at offset %d", r.name, r.pos)
	}
	return r.data[r.pos], nil
}

func (r *reader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("[%s] EOF at offset %d", r.name, r.pos)
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *reader) readBytes(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, fmt.Errorf("[%s] EOF: need %d bytes at offset %d, have %d", r.name, n, r.pos, len(r.data)-r.pos)
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *reader) readInt32() (int32, error) {
	b, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), nil
}

func (r *reader) readInt64() (int64, error) {
	b, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil
}

func (r *reader) readVarInt() (int32, int, error) {
	var result int32
	var numRead int
	for {
		b, err := r.readByte()
		if err != nil {
			return 0, numRead, err
		}
		result |= int32(b&0x7F) << (7 * numRead)
		numRead++
		if b&0x80 == 0 {
			break
		}
		if numRead > 5 {
			return 0, numRead, fmt.Errorf("[%s] VarInt too long at offset %d", r.name, r.pos)
		}
	}
	return result, numRead, nil
}

func (r *reader) readUint16() (uint16, error) {
	b, err := r.readBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

// Heightmap entry: key (VarInt type ID) + packed long array
type heightmapEntry struct {
	key        int32
	longCount  int32
	longs      []int64
	firstVals  []int // first few decoded values (9 bits per entry, 7 per long)
}

type bitSet struct {
	longCount int32
	longs     []int64
}

func (b bitSet) String() string {
	if b.longCount == 0 {
		return "empty"
	}
	setBits := 0
	for _, l := range b.longs {
		setBits += bits.OnesCount64(uint64(l))
	}
	return fmt.Sprintf("%d longs, %d bits set", b.longCount, setBits)
}

func (b bitSet) setBitIndices() []int {
	var result []int
	for i, l := range b.longs {
		ul := uint64(l)
		for ul != 0 {
			bit := bits.TrailingZeros64(ul)
			result = append(result, i*64+bit)
			ul &^= 1 << uint(bit)
		}
	}
	return result
}

type lightArray struct {
	length int32
	data   []byte
}

type section struct {
	blockCount    int16
	stateBits     byte
	statePalette  []int32
	stateDataLen  int32 // number of longs
	stateData     []int64
	biomeBits     byte
	biomePalette  []int32
	biomeDataLen  int32
	biomeData     []int64
}

type chunkData struct {
	chunkX          int32
	chunkZ          int32
	heightmaps      []heightmapEntry
	sectionBytes    int32
	sections        []section
	blockEntCnt     int32
	formatHasVarInt bool // whether the paletted container has VarInt data-length prefix

	skyYMask        bitSet
	blockYMask      bitSet
	emptySkyYMask   bitSet
	emptyBlockYMask bitSet
	skyUpdates      []lightArray
	blockUpdates    []lightArray
}

func readBitSet(r *reader) (bitSet, error) {
	count, _, err := r.readVarInt()
	if err != nil {
		return bitSet{}, fmt.Errorf("reading bitset long count: %w", err)
	}
	longs := make([]int64, count)
	for i := int32(0); i < count; i++ {
		l, err := r.readInt64()
		if err != nil {
			return bitSet{}, fmt.Errorf("reading bitset long %d: %w", i, err)
		}
		longs[i] = l
	}
	return bitSet{longCount: count, longs: longs}, nil
}

// calcDataLongs computes the expected number of longs for a paletted container
// given bitsPerEntry and the total number of entries.
func calcDataLongs(bitsPerEntry int, totalEntries int) int {
	if bitsPerEntry == 0 {
		return 0
	}
	valuesPerLong := 64 / bitsPerEntry
	return (totalEntries + valuesPerLong - 1) / valuesPerLong
}

// readPalettedContainer reads a paletted container.
// If hasVarIntLen is true, it expects a VarInt data-array-length prefix (pre-26.1 format).
// If false, it infers the data array length from bitsPerEntry and totalEntries (26.1+ format).
func readPalettedContainer(r *reader, totalEntries int, hasVarIntLen bool) (bitsPerEntry byte, palette []int32, dataLongCount int32, dataLongs []int64, err error) {
	b, err := r.readByte()
	if err != nil {
		return 0, nil, 0, nil, fmt.Errorf("reading bits per entry: %w", err)
	}
	bitsPerEntry = b

	if bitsPerEntry == 0 {
		// Single-value palette
		val, _, err := r.readVarInt()
		if err != nil {
			return 0, nil, 0, nil, fmt.Errorf("reading single palette value: %w", err)
		}
		palette = []int32{val}
		if hasVarIntLen {
			// Read and skip data array length (should be 0)
			dataLen, _, err := r.readVarInt()
			if err != nil {
				return 0, nil, 0, nil, fmt.Errorf("reading data array length: %w", err)
			}
			dataLongs = make([]int64, dataLen)
			for i := int32(0); i < dataLen; i++ {
				dataLongs[i], err = r.readInt64()
				if err != nil {
					return 0, nil, 0, nil, fmt.Errorf("reading data long %d: %w", i, err)
				}
			}
			return bitsPerEntry, palette, dataLen, dataLongs, nil
		}
		// No data for single-value in new format
		return bitsPerEntry, palette, 0, nil, nil
	}

	// Non-zero bitsPerEntry: check indirect vs direct
	// For block states: bits 1-8 = indirect (palette), 15 = direct
	// For biomes: bits 1-3 = indirect (palette), 6 = direct
	if bitsPerEntry <= 8 {
		// Indirect - has palette
		paletteLen, _, err := r.readVarInt()
		if err != nil {
			return 0, nil, 0, nil, fmt.Errorf("reading palette length: %w", err)
		}
		palette = make([]int32, paletteLen)
		for i := int32(0); i < paletteLen; i++ {
			palette[i], _, err = r.readVarInt()
			if err != nil {
				return 0, nil, 0, nil, fmt.Errorf("reading palette entry %d: %w", i, err)
			}
		}
	}
	// else: direct encoding, no palette

	var dataLen int32
	if hasVarIntLen {
		dataLen, _, err = r.readVarInt()
		if err != nil {
			return 0, nil, 0, nil, fmt.Errorf("reading data array length: %w", err)
		}
	} else {
		dataLen = int32(calcDataLongs(int(bitsPerEntry), totalEntries))
	}

	dataLongs = make([]int64, dataLen)
	for i := int32(0); i < dataLen; i++ {
		dataLongs[i], err = r.readInt64()
		if err != nil {
			return 0, nil, 0, nil, fmt.Errorf("reading data long %d: %w", i, err)
		}
	}
	return bitsPerEntry, palette, dataLen, dataLongs, nil
}

func decodePacked(longs []int64, bitsPerEntry int, maxEntries int) []int {
	if bitsPerEntry == 0 || len(longs) == 0 {
		return nil
	}
	entriesPerLong := 64 / bitsPerEntry
	mask := int64((1 << bitsPerEntry) - 1)
	var result []int
	for _, l := range longs {
		for j := 0; j < entriesPerLong && len(result) < maxEntries; j++ {
			val := int((l >> (j * bitsPerEntry)) & mask)
			result = append(result, val)
		}
	}
	return result
}

// detectFormat tries both formats and returns which one works.
// It parses all sections with each format and checks if the consumed bytes
// match the declared section byte length. When both work (ambiguous case,
// e.g. all single-value palettes), it prefers the format that yields 24
// sections (the standard overworld section count).
func detectFormat(data []byte, sectionStart int, sectionByteLen int, name string) bool {
	type result struct {
		ok       bool
		sections int
	}

	tryFormat := func(hasVarInt bool) result {
		r := newReader(data, name+"_probe")
		r.pos = sectionStart
		sectionEnd := sectionStart + sectionByteLen
		count := 0
		for r.pos < sectionEnd {
			if _, err := r.readUint16(); err != nil {
				return result{false, count}
			}
			if _, _, _, _, err := readPalettedContainer(r, 4096, hasVarInt); err != nil {
				return result{false, count}
			}
			if _, _, _, _, err := readPalettedContainer(r, 64, hasVarInt); err != nil {
				return result{false, count}
			}
			count++
		}
		return result{r.pos == sectionStart+sectionByteLen, count}
	}

	newFmt := tryFormat(false)  // 26.1 format (no VarInt data-length)
	oldFmt := tryFormat(true)   // pre-26.1 format (with VarInt data-length)

	fmt.Printf("  [%s] Format probe: new_format(no-varint)=%v/%d-sections, old_format(varint)=%v/%d-sections\n",
		name, newFmt.ok, newFmt.sections, oldFmt.ok, oldFmt.sections)

	if newFmt.ok && oldFmt.ok {
		// Both work -- prefer the one that gives 24 sections
		if newFmt.sections == 24 && oldFmt.sections != 24 {
			return false
		}
		if oldFmt.sections == 24 && newFmt.sections != 24 {
			return true
		}
		// Both give 24 or neither does -- check for non-trivial sections
		// (sections with actual block data). New format is the 26.1 standard.
		return false
	}
	if oldFmt.ok {
		return true
	}
	return false // default to new format
}

func parseChunk(data []byte, name string) (*chunkData, error) {
	r := newReader(data, name)
	c := &chunkData{}
	var err error

	// 1. Chunk X, Z
	c.chunkX, err = r.readInt32()
	if err != nil {
		return nil, fmt.Errorf("reading chunkX: %w", err)
	}
	c.chunkZ, err = r.readInt32()
	if err != nil {
		return nil, fmt.Errorf("reading chunkZ: %w", err)
	}

	// 3. Heightmaps: VarInt(count) + entries
	hmCount, _, err := r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("reading heightmap count: %w", err)
	}
	c.heightmaps = make([]heightmapEntry, hmCount)
	for i := int32(0); i < hmCount; i++ {
		key, _, err := r.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("reading heightmap key %d: %w", i, err)
		}
		longCount, _, err := r.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("reading heightmap longCount %d: %w", i, err)
		}
		longs := make([]int64, longCount)
		for j := int32(0); j < longCount; j++ {
			longs[j], err = r.readInt64()
			if err != nil {
				return nil, fmt.Errorf("reading heightmap long %d/%d: %w", i, j, err)
			}
		}
		// Decode first few values: 9 bits per entry, 7 per long
		firstVals := decodePacked(longs, 9, 16)
		c.heightmaps[i] = heightmapEntry{
			key:       key,
			longCount: longCount,
			longs:     longs,
			firstVals: firstVals,
		}
	}

	// 4. Section data: VarInt(byteLen) + bytes
	sectionByteLen, _, err := r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("reading section byte length: %w", err)
	}
	c.sectionBytes = sectionByteLen

	sectionStart := r.pos
	sectionEnd := r.pos + int(sectionByteLen)

	// Auto-detect format: try without VarInt data-length prefix first (26.1 format),
	// then with (pre-26.1 / go-mc format)
	hasVarIntLen := detectFormat(data, sectionStart, int(sectionByteLen), name)
	c.formatHasVarInt = hasVarIntLen
	formatStr := "26.1 (no VarInt data-length)"
	if hasVarIntLen {
		formatStr = "pre-26.1 (with VarInt data-length)"
	}
	fmt.Printf("  [%s] Detected section format: %s\n", name, formatStr)

	// Parse individual sections
	for r.pos < sectionEnd {
		secIdx := len(c.sections)
		var s section
		// Block count (Int16, big-endian)
		bc, err := r.readUint16()
		if err != nil {
			break
		}
		s.blockCount = int16(bc)

		// Block states paletted container
		s.stateBits, s.statePalette, s.stateDataLen, s.stateData, err = readPalettedContainer(r, 4096, hasVarIntLen)
		if err != nil {
			return nil, fmt.Errorf("reading block states section %d: %w", secIdx, err)
		}

		// Biomes paletted container
		s.biomeBits, s.biomePalette, s.biomeDataLen, s.biomeData, err = readPalettedContainer(r, 64, hasVarIntLen)
		if err != nil {
			return nil, fmt.Errorf("reading biomes section %d: %w", secIdx, err)
		}

		c.sections = append(c.sections, s)
	}

	// Verify section data consumed correctly
	if r.pos != sectionEnd {
		fmt.Printf("  [%s] WARNING: section data parse ended at offset %d, expected %d (diff=%d)\n",
			name, r.pos, sectionEnd, r.pos-sectionEnd)
		r.pos = sectionEnd // Reset to expected position
	} else {
		consumed := r.pos - sectionStart
		fmt.Printf("  [%s] Section data: declared %d bytes, consumed %d bytes -- OK\n", name, sectionByteLen, consumed)
	}

	// 5. Block entities
	c.blockEntCnt, _, err = r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("reading block entity count: %w", err)
	}
	for i := int32(0); i < c.blockEntCnt; i++ {
		// packed XZ: 1 byte
		_, err := r.readByte()
		if err != nil {
			return nil, fmt.Errorf("reading block entity %d packedXZ: %w", i, err)
		}
		// Y: short (2 bytes)
		_, err = r.readBytes(2)
		if err != nil {
			return nil, fmt.Errorf("reading block entity %d Y: %w", i, err)
		}
		// type: VarInt
		_, _, err = r.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("reading block entity %d type: %w", i, err)
		}
		// NBT: skip a full NBT value
		err = skipNBT(r)
		if err != nil {
			return nil, fmt.Errorf("reading block entity %d NBT: %w", i, err)
		}
	}

	// 6. Light data
	c.skyYMask, err = readBitSet(r)
	if err != nil {
		return nil, fmt.Errorf("reading skyYMask: %w", err)
	}
	c.blockYMask, err = readBitSet(r)
	if err != nil {
		return nil, fmt.Errorf("reading blockYMask: %w", err)
	}
	c.emptySkyYMask, err = readBitSet(r)
	if err != nil {
		return nil, fmt.Errorf("reading emptySkyYMask: %w", err)
	}
	c.emptyBlockYMask, err = readBitSet(r)
	if err != nil {
		return nil, fmt.Errorf("reading emptyBlockYMask: %w", err)
	}

	// Sky light updates
	skyCount, _, err := r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("reading sky update count: %w", err)
	}
	c.skyUpdates = make([]lightArray, skyCount)
	for i := int32(0); i < skyCount; i++ {
		length, _, err := r.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("reading sky update %d length: %w", i, err)
		}
		data, err := r.readBytes(int(length))
		if err != nil {
			return nil, fmt.Errorf("reading sky update %d data: %w", i, err)
		}
		c.skyUpdates[i] = lightArray{length: length, data: data}
	}

	// Block light updates
	blockCount, _, err := r.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("reading block update count: %w", err)
	}
	c.blockUpdates = make([]lightArray, blockCount)
	for i := int32(0); i < blockCount; i++ {
		length, _, err := r.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("reading block update %d length: %w", i, err)
		}
		data, err := r.readBytes(int(length))
		if err != nil {
			return nil, fmt.Errorf("reading block update %d data: %w", i, err)
		}
		c.blockUpdates[i] = lightArray{length: length, data: data}
	}

	fmt.Printf("  [%s] Parse complete. Consumed %d / %d bytes, remaining: %d\n",
		name, r.pos, len(r.data), r.remaining())

	return c, nil
}

// skipNBT skips a complete NBT value starting with the root tag.
// Minecraft network NBT is "nameless" - the root compound starts with just TAG_Compound (0x0A)
// with no name, or TAG_End (0x00) for empty.
func skipNBT(r *reader) error {
	tagType, err := r.readByte()
	if err != nil {
		return err
	}
	if tagType == 0 {
		// TAG_End - empty NBT
		return nil
	}
	// Network NBT: root tag has no name
	return skipNBTPayload(r, tagType)
}

func skipNBTPayload(r *reader, tagType byte) error {
	switch tagType {
	case 1: // TAG_Byte
		_, err := r.readByte()
		return err
	case 2: // TAG_Short
		_, err := r.readBytes(2)
		return err
	case 3: // TAG_Int
		_, err := r.readBytes(4)
		return err
	case 4: // TAG_Long
		_, err := r.readBytes(8)
		return err
	case 5: // TAG_Float
		_, err := r.readBytes(4)
		return err
	case 6: // TAG_Double
		_, err := r.readBytes(8)
		return err
	case 7: // TAG_Byte_Array
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		_, err = r.readBytes(int(length))
		return err
	case 8: // TAG_String
		length, err := r.readUint16()
		if err != nil {
			return err
		}
		_, err = r.readBytes(int(length))
		return err
	case 9: // TAG_List
		elemType, err := r.readByte()
		if err != nil {
			return err
		}
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		for i := int32(0); i < length; i++ {
			if err := skipNBTPayload(r, elemType); err != nil {
				return err
			}
		}
		return nil
	case 10: // TAG_Compound
		for {
			childType, err := r.readByte()
			if err != nil {
				return err
			}
			if childType == 0 { // TAG_End
				return nil
			}
			// Read name (modified UTF-8: uint16 length + bytes)
			nameLen, err := r.readUint16()
			if err != nil {
				return err
			}
			_, err = r.readBytes(int(nameLen))
			if err != nil {
				return err
			}
			if err := skipNBTPayload(r, childType); err != nil {
				return err
			}
		}
	case 11: // TAG_Int_Array
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		_, err = r.readBytes(int(length) * 4)
		return err
	case 12: // TAG_Long_Array
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		_, err = r.readBytes(int(length) * 8)
		return err
	default:
		return fmt.Errorf("unknown NBT tag type %d at offset %d", tagType, r.pos-1)
	}
}

func heightmapTypeName(id int32) string {
	names := map[int32]string{
		0: "WORLD_SURFACE_WG",
		1: "WORLD_SURFACE",
		2: "OCEAN_FLOOR_WG",
		3: "OCEAN_FLOOR",
		4: "MOTION_BLOCKING",
		5: "MOTION_BLOCKING_NO_LEAVES",
	}
	if n, ok := names[id]; ok {
		return n
	}
	return fmt.Sprintf("UNKNOWN(%d)", id)
}

func main() {
	vanillaPath := `C:\Users\marku\GolandProjects\go-mc\chunk_vanilla.bin`
	oursPath := `C:\Users\marku\GolandProjects\go-mc\chunk_ours_fixed.bin`

	vanillaData, err := os.ReadFile(vanillaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading vanilla: %v\n", err)
		os.Exit(1)
	}
	oursData, err := os.ReadFile(oursPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading ours: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== File Sizes ===\n")
	fmt.Printf("  Vanilla: %d bytes\n", len(vanillaData))
	fmt.Printf("  Ours:    %d bytes\n\n", len(oursData))

	fmt.Println("=== Parsing Vanilla ===")
	vanilla, err := parseChunk(vanillaData, "vanilla")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing vanilla: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Parsing Ours ===")
	ours, err := parseChunk(oursData, "ours")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing ours: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("SIDE-BY-SIDE COMPARISON")
	fmt.Println(strings.Repeat("=", 80))

	// Format detection
	fmt.Printf("\n--- Wire Format ---\n")
	fmt.Printf("  %-25s vanilla=%-30s ours=%-30s\n", "Section format",
		fmtName(vanilla.formatHasVarInt), fmtName(ours.formatHasVarInt))

	// Chunk coordinates
	fmt.Printf("\n--- Chunk Coordinates ---\n")
	compare("ChunkX", vanilla.chunkX, ours.chunkX)
	compare("ChunkZ", vanilla.chunkZ, ours.chunkZ)

	// Heightmaps
	fmt.Printf("\n--- Heightmaps ---\n")
	compare("Count", len(vanilla.heightmaps), len(ours.heightmaps))
	maxHM := len(vanilla.heightmaps)
	if len(ours.heightmaps) > maxHM {
		maxHM = len(ours.heightmaps)
	}
	for i := 0; i < maxHM; i++ {
		var vHM, oHM *heightmapEntry
		if i < len(vanilla.heightmaps) {
			vHM = &vanilla.heightmaps[i]
		}
		if i < len(ours.heightmaps) {
			oHM = &ours.heightmaps[i]
		}
		fmt.Printf("  Heightmap[%d]:\n", i)
		if vHM != nil && oHM != nil {
			vName := heightmapTypeName(vHM.key)
			oName := heightmapTypeName(oHM.key)
			fmt.Printf("    Key:       vanilla=%d (%s)  ours=%d (%s)%s\n",
				vHM.key, vName, oHM.key, oName, diffMark(vHM.key, oHM.key))
			fmt.Printf("    LongCount: vanilla=%d  ours=%d%s\n",
				vHM.longCount, oHM.longCount, diffMark(vHM.longCount, oHM.longCount))
			fmt.Printf("    First values (vanilla): %v\n", vHM.firstVals)
			fmt.Printf("    First values (ours):    %v\n", oHM.firstVals)
			// Compare long arrays
			matchCount := 0
			diffCount := 0
			minLen := int(vHM.longCount)
			if int(oHM.longCount) < minLen {
				minLen = int(oHM.longCount)
			}
			for j := 0; j < minLen; j++ {
				if vHM.longs[j] == oHM.longs[j] {
					matchCount++
				} else {
					diffCount++
				}
			}
			fmt.Printf("    Longs compared: %d match, %d differ\n", matchCount, diffCount)
		} else if vHM != nil {
			fmt.Printf("    ONLY in vanilla: key=%d (%s), %d longs\n", vHM.key, heightmapTypeName(vHM.key), vHM.longCount)
		} else if oHM != nil {
			fmt.Printf("    ONLY in ours: key=%d (%s), %d longs\n", oHM.key, heightmapTypeName(oHM.key), oHM.longCount)
		}
	}

	// Section data
	fmt.Printf("\n--- Section Data ---\n")
	compare("Declared byte length", vanilla.sectionBytes, ours.sectionBytes)
	compare("Sections parsed", len(vanilla.sections), len(ours.sections))

	maxSec := len(vanilla.sections)
	if len(ours.sections) > maxSec {
		maxSec = len(ours.sections)
	}
	for i := 0; i < maxSec; i++ {
		yLevel := (i * 16) - 64
		fmt.Printf("\n  Section[%d] (Y %d to %d):\n", i, yLevel, yLevel+15)
		var vS, oS *section
		if i < len(vanilla.sections) {
			vS = &vanilla.sections[i]
		}
		if i < len(ours.sections) {
			oS = &ours.sections[i]
		}

		if vS != nil && oS != nil {
			compare2("    blockCount", vS.blockCount, oS.blockCount)
			compare2("    states bitsPerEntry", vS.stateBits, oS.stateBits)
			compare2("    states palette size", len(vS.statePalette), len(oS.statePalette))
			if len(vS.statePalette) <= 16 && len(oS.statePalette) <= 16 {
				if !intSliceEqual(vS.statePalette, oS.statePalette) {
					fmt.Printf("      vanilla palette: %v\n", vS.statePalette)
					fmt.Printf("      ours palette:    %v\n", oS.statePalette)
				}
			}
			compare2("    states data longs", vS.stateDataLen, oS.stateDataLen)
			if vS.stateDataLen > 0 && oS.stateDataLen > 0 {
				stateMatch, stateDiff := compareLongs(vS.stateData, oS.stateData)
				if stateDiff > 0 {
					fmt.Printf("      state data: %d longs match, %d differ\n", stateMatch, stateDiff)
				}
			}
			compare2("    biome bitsPerEntry", vS.biomeBits, oS.biomeBits)
			compare2("    biome palette size", len(vS.biomePalette), len(oS.biomePalette))
			if !intSliceEqual(vS.biomePalette, oS.biomePalette) {
				fmt.Printf("      vanilla biome palette: %v\n", vS.biomePalette)
				fmt.Printf("      ours biome palette:    %v\n", oS.biomePalette)
			}
			compare2("    biome data longs", vS.biomeDataLen, oS.biomeDataLen)
		} else if vS != nil {
			fmt.Printf("    ONLY in vanilla: blockCount=%d, stateBits=%d, palette=%v, biomeBits=%d\n",
				vS.blockCount, vS.stateBits, vS.statePalette, vS.biomeBits)
		} else if oS != nil {
			fmt.Printf("    ONLY in ours: blockCount=%d, stateBits=%d, palette=%v, biomeBits=%d\n",
				oS.blockCount, oS.stateBits, oS.statePalette, oS.biomeBits)
		}
	}

	// Block entities
	fmt.Printf("\n--- Block Entities ---\n")
	compare("Count", vanilla.blockEntCnt, ours.blockEntCnt)

	// Light data
	fmt.Printf("\n--- Light Data ---\n")
	fmt.Printf("  skyYMask:        vanilla=(%s)  ours=(%s)\n", vanilla.skyYMask, ours.skyYMask)
	fmt.Printf("  blockYMask:      vanilla=(%s)  ours=(%s)\n", vanilla.blockYMask, ours.blockYMask)
	fmt.Printf("  emptySkyYMask:   vanilla=(%s)  ours=(%s)\n", vanilla.emptySkyYMask, ours.emptySkyYMask)
	fmt.Printf("  emptyBlockYMask: vanilla=(%s)  ours=(%s)\n", vanilla.emptyBlockYMask, ours.emptyBlockYMask)

	// Show which light sections have data
	fmt.Printf("\n  skyYMask bits set:        vanilla=%v  ours=%v\n",
		vanilla.skyYMask.setBitIndices(), ours.skyYMask.setBitIndices())
	fmt.Printf("  blockYMask bits set:      vanilla=%v  ours=%v\n",
		vanilla.blockYMask.setBitIndices(), ours.blockYMask.setBitIndices())
	fmt.Printf("  emptySkyYMask bits set:   vanilla=%v  ours=%v\n",
		vanilla.emptySkyYMask.setBitIndices(), ours.emptySkyYMask.setBitIndices())
	fmt.Printf("  emptyBlockYMask bits set: vanilla=%v  ours=%v\n",
		vanilla.emptyBlockYMask.setBitIndices(), ours.emptyBlockYMask.setBitIndices())

	fmt.Printf("\n  Sky light arrays:   vanilla=%d  ours=%d%s\n",
		len(vanilla.skyUpdates), len(ours.skyUpdates),
		diffMark(len(vanilla.skyUpdates), len(ours.skyUpdates)))
	for i := 0; i < len(vanilla.skyUpdates) || i < len(ours.skyUpdates); i++ {
		vLen := int32(-1)
		oLen := int32(-1)
		if i < len(vanilla.skyUpdates) {
			vLen = vanilla.skyUpdates[i].length
		}
		if i < len(ours.skyUpdates) {
			oLen = ours.skyUpdates[i].length
		}
		dm := ""
		if vLen >= 0 && oLen >= 0 {
			match := true
			if vLen == oLen {
				for j := 0; j < int(vLen); j++ {
					if vanilla.skyUpdates[i].data[j] != ours.skyUpdates[i].data[j] {
						match = false
						break
					}
				}
				if !match {
					dm = " <-- DIFF (content)"
				}
			} else {
				dm = " <-- DIFF (size)"
			}
			fmt.Printf("    [%d] vanilla=%d bytes, ours=%d bytes%s\n", i, vLen, oLen, dm)
		} else if vLen >= 0 {
			fmt.Printf("    [%d] vanilla=%d bytes, ours=MISSING\n", i, vLen)
		} else {
			fmt.Printf("    [%d] vanilla=MISSING, ours=%d bytes\n", i, oLen)
		}
	}

	fmt.Printf("\n  Block light arrays: vanilla=%d  ours=%d%s\n",
		len(vanilla.blockUpdates), len(ours.blockUpdates),
		diffMark(len(vanilla.blockUpdates), len(ours.blockUpdates)))
	for i := 0; i < len(vanilla.blockUpdates) || i < len(ours.blockUpdates); i++ {
		vLen := int32(-1)
		oLen := int32(-1)
		if i < len(vanilla.blockUpdates) {
			vLen = vanilla.blockUpdates[i].length
		}
		if i < len(ours.blockUpdates) {
			oLen = ours.blockUpdates[i].length
		}
		if vLen >= 0 && oLen >= 0 {
			dm := ""
			if vLen != oLen {
				dm = " <-- DIFF"
			}
			fmt.Printf("    [%d] vanilla=%d bytes, ours=%d bytes%s\n", i, vLen, oLen, dm)
		} else if vLen >= 0 {
			fmt.Printf("    [%d] vanilla=%d bytes, ours=MISSING\n", i, vLen)
		} else {
			fmt.Printf("    [%d] vanilla=MISSING, ours=%d bytes\n", i, oLen)
		}
	}

	// Summary
	fmt.Print("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Println("SUMMARY OF DIFFERENCES")
	fmt.Println(strings.Repeat("=", 80))

	diffs := 0

	if vanilla.formatHasVarInt != ours.formatHasVarInt {
		fmt.Printf("  ** CRITICAL: Section wire format mismatch! **\n")
		fmt.Printf("     Vanilla uses 26.1 format (no VarInt data-length prefix in PalettedContainer)\n")
		fmt.Printf("     Ours uses pre-26.1 format (has VarInt data-length prefix)\n")
		fmt.Printf("     This means go-mc's PaletteContainer.WriteTo/ReadFrom must be updated\n")
		fmt.Printf("     to remove the VarInt(len(data)) prefix from BitStorage serialization.\n")
		diffs++
	}

	if vanilla.chunkX != ours.chunkX || vanilla.chunkZ != ours.chunkZ {
		fmt.Printf("  - Chunk coordinates differ (expected -- different chunks captured)\n")
		diffs++
	}

	hmDiffs := 0
	for i := 0; i < len(vanilla.heightmaps) && i < len(ours.heightmaps); i++ {
		if vanilla.heightmaps[i].key != ours.heightmaps[i].key {
			hmDiffs++
		}
		allZero := true
		for _, v := range vanilla.heightmaps[i].firstVals {
			if v != 0 {
				allZero = false
				break
			}
		}
		oursAllZero := true
		for _, v := range ours.heightmaps[i].firstVals {
			if v != 0 {
				oursAllZero = false
				break
			}
		}
		if !allZero && oursAllZero {
			hmDiffs++
		}
	}
	if hmDiffs > 0 {
		fmt.Printf("  - Heightmap data differs: vanilla has actual height values, ours has all zeros\n")
		diffs++
	}

	if vanilla.sectionBytes != ours.sectionBytes {
		fmt.Printf("  - Section byte length: vanilla=%d, ours=%d\n", vanilla.sectionBytes, ours.sectionBytes)
		diffs++
	}
	if len(vanilla.sections) != len(ours.sections) {
		fmt.Printf("  - Number of sections: vanilla=%d, ours=%d\n", len(vanilla.sections), len(ours.sections))
		diffs++
	}

	secDiffs := 0
	minSec := len(vanilla.sections)
	if len(ours.sections) < minSec {
		minSec = len(ours.sections)
	}
	for i := 0; i < minSec; i++ {
		vS := vanilla.sections[i]
		oS := ours.sections[i]
		if vS.blockCount != oS.blockCount || vS.stateBits != oS.stateBits ||
			vS.stateDataLen != oS.stateDataLen || vS.biomeBits != oS.biomeBits ||
			vS.biomeDataLen != oS.biomeDataLen ||
			!intSliceEqual(vS.statePalette, oS.statePalette) {
			secDiffs++
		}
	}
	if secDiffs > 0 {
		fmt.Printf("  - %d/%d sections have structural differences\n", secDiffs, minSec)
		diffs++
	}

	if vanilla.blockEntCnt != ours.blockEntCnt {
		fmt.Printf("  - Block entity count: vanilla=%d, ours=%d\n", vanilla.blockEntCnt, ours.blockEntCnt)
		diffs++
	}

	lightDiffs := 0
	if vanilla.skyYMask.String() != ours.skyYMask.String() {
		fmt.Printf("  - skyYMask differs: vanilla has %d sections, ours has %d\n",
			len(vanilla.skyYMask.setBitIndices()), len(ours.skyYMask.setBitIndices()))
		lightDiffs++
	}
	if vanilla.blockYMask.String() != ours.blockYMask.String() {
		fmt.Printf("  - blockYMask differs\n")
		lightDiffs++
	}
	if vanilla.emptySkyYMask.String() != ours.emptySkyYMask.String() {
		fmt.Printf("  - emptySkyYMask differs\n")
		lightDiffs++
	}
	if vanilla.emptyBlockYMask.String() != ours.emptyBlockYMask.String() {
		fmt.Printf("  - emptyBlockYMask differs\n")
		lightDiffs++
	}
	if len(vanilla.skyUpdates) != len(ours.skyUpdates) {
		fmt.Printf("  - Sky light array count: vanilla=%d, ours=%d\n",
			len(vanilla.skyUpdates), len(ours.skyUpdates))
		lightDiffs++
	}
	if len(vanilla.blockUpdates) != len(ours.blockUpdates) {
		fmt.Printf("  - Block light array count: vanilla=%d, ours=%d\n",
			len(vanilla.blockUpdates), len(ours.blockUpdates))
		lightDiffs++
	}
	if lightDiffs > 0 {
		fmt.Printf("  - Total %d light data fields differ\n", lightDiffs)
		diffs++
	}

	if diffs == 0 {
		fmt.Println("  No structural differences found!")
	} else {
		fmt.Printf("\n  Total: %d categories of differences\n", diffs)
	}
}

func fmtName(hasVarInt bool) string {
	if hasVarInt {
		return "pre-26.1 (with VarInt data-len)"
	}
	return "26.1 (no VarInt data-len)"
}

func compare[T comparable](label string, v, o T) {
	mark := ""
	if v != o {
		mark = " <-- DIFF"
	}
	fmt.Printf("  %-25s vanilla=%-12v ours=%-12v%s\n", label, v, o, mark)
}

func compare2[T comparable](label string, v, o T) {
	mark := ""
	if v != o {
		mark = " <-- DIFF"
	}
	fmt.Printf("  %-25s vanilla=%-8v ours=%-8v%s\n", label, v, o, mark)
}

func diffMark[T comparable](a, b T) string {
	if a != b {
		return " <-- DIFF"
	}
	return ""
}

func intSliceEqual(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func compareLongs(a, b []int64) (match, diff int) {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] {
			match++
		} else {
			diff++
		}
	}
	if len(a) > minLen {
		diff += len(a) - minLen
	}
	if len(b) > minLen {
		diff += len(b) - minLen
	}
	return
}
