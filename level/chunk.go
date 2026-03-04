package level

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"strconv"

	"github.com/Tnze/go-mc/level/block"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/save"
)

type ChunkPos [2]int32

func (c ChunkPos) WriteTo(w io.Writer) (n int64, err error) {
	n, err = pk.Int(c[0]).WriteTo(w)
	if err != nil {
		return
	}
	n1, err := pk.Int(c[1]).WriteTo(w)
	return n + n1, err
}

func (c *ChunkPos) ReadFrom(r io.Reader) (n int64, err error) {
	var x, z pk.Int
	if n, err = x.ReadFrom(r); err != nil {
		return n, err
	}
	var n1 int64
	if n1, err = z.ReadFrom(r); err != nil {
		return n + n1, err
	}
	*c = ChunkPos{int32(x), int32(z)}
	return n + n1, nil
}

type Chunk struct {
	Sections    []Section
	HeightMaps  HeightMaps
	BlockEntity []BlockEntity
	Status      ChunkStatus
}

func EmptyChunk(secs int) *Chunk {
	sections := make([]Section, secs)
	for i := range sections {
		sections[i] = Section{
			BlockCount: 0,
			States:     NewStatesPaletteContainer(16*16*16, 0),
			Biomes:     NewBiomesPaletteContainer(4*4*4, 0),
		}
	}
	return &Chunk{
		Sections: sections,
		HeightMaps: HeightMaps{
			WorldSurfaceWG:         NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
			WorldSurface:           NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
			OceanFloorWG:           NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
			OceanFloor:             NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
			MotionBlocking:         NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
			MotionBlockingNoLeaves: NewBitStorage(bits.Len(uint(secs)*16+1), 16*16, nil),
		},
		Status: StatusEmpty,
	}
}

// ChunkFromSave convert save.Chunk to level.Chunk.
func ChunkFromSave(c *save.Chunk) (*Chunk, error) {
	secs := len(c.Sections)
	sections := make([]Section, secs)
	for _, v := range c.Sections {
		i := int32(v.Y) - c.YPos
		if i < 0 || i >= int32(secs) {
			return nil, fmt.Errorf("section Y value %d out of bounds", v.Y)
		}
		var err error
		sections[i].States, err = readStatesPalette(v.BlockStates.Palette, v.BlockStates.Data)
		if err != nil {
			return nil, err
		}
		sections[i].BlockCount = countNoneAirBlocks(&sections[i])
		sections[i].Biomes, err = readBiomesPalette(v.Biomes.Palette, v.Biomes.Data)
		if err != nil {
			return nil, err
		}
		sections[i].SkyLight = v.SkyLight
		sections[i].BlockLight = v.BlockLight
	}

	blockEntities := make([]BlockEntity, len(c.BlockEntities))
	for i, v := range c.BlockEntities {
		var tmp struct {
			ID string `nbt:"id"`
			X  int32  `nbt:"x"`
			Y  int32  `nbt:"y"`
			Z  int32  `nbt:"z"`
		}
		if err := v.Unmarshal(&tmp); err != nil {
			return nil, err
		}
		blockEntities[i].Data = v
		if x, z := int(tmp.X-c.XPos<<4), int(tmp.Z-c.ZPos<<4); !blockEntities[i].PackXZ(x, z) {
			return nil, errors.New("Packing a XZ(" + strconv.Itoa(x) + ", " + strconv.Itoa(z) + ") out of bound")
		}
		blockEntities[i].Y = int16(tmp.Y)
		blockEntities[i].Type = block.EntityTypes[tmp.ID]
	}

	bitsForHeight := bits.Len( /* chunk height in blocks */ uint(secs)*16 + 1)
	return &Chunk{
		Sections: sections,
		HeightMaps: HeightMaps{
			WorldSurface:           NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["WORLD_SURFACE_WG"]),
			WorldSurfaceWG:         NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["WORLD_SURFACE"]),
			OceanFloorWG:           NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["OCEAN_FLOOR_WG"]),
			OceanFloor:             NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["OCEAN_FLOOR"]),
			MotionBlocking:         NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["MOTION_BLOCKING"]),
			MotionBlockingNoLeaves: NewBitStorage(bitsForHeight, 16*16, c.Heightmaps["MOTION_BLOCKING_NO_LEAVES"]),
		},
		BlockEntity: blockEntities,
		Status:      ChunkStatus(c.Status),
	}, nil
}

func readStatesPalette(palette []save.BlockState, data []uint64) (paletteData *PaletteContainer[BlocksState], err error) {
	statePalette := make([]BlocksState, len(palette))
	for i, v := range palette {
		b, ok := block.FromID[v.Name]
		if !ok {
			return nil, fmt.Errorf("unknown block id: %v", v.Name)
		}
		if v.Properties.Data != nil {
			if err := v.Properties.Unmarshal(&b); err != nil {
				return nil, fmt.Errorf("unmarshal block properties fail: %v", err)
			}
		}
		s, ok := block.ToStateID[b]
		if !ok {
			return nil, fmt.Errorf("unknown block: %v", b)
		}
		statePalette[i] = s
	}
	paletteData = NewStatesPaletteContainerWithData(16*16*16, data, statePalette)
	return
}

func readBiomesPalette(palette []save.BiomeState, data []uint64) (*PaletteContainer[BiomesState], error) {
	biomesRawPalette := make([]BiomesState, len(palette))
	for i, v := range palette {
		err := biomesRawPalette[i].UnmarshalText([]byte(v))
		if err != nil {
			return nil, err
		}
	}
	return NewBiomesPaletteContainerWithData(4*4*4, data, biomesRawPalette), nil
}

func countNoneAirBlocks(sec *Section) (blockCount int16) {
	for i := 0; i < 16*16*16; i++ {
		b := sec.GetBlock(i)
		if !block.IsAir(b) {
			blockCount++
		}
	}
	return
}

// ChunkToSave convert level.Chunk to save.Chunk
func ChunkToSave(c *Chunk, dst *save.Chunk) (err error) {
	secs := len(c.Sections)
	sections := make([]save.Section, secs)
	for i, v := range c.Sections {
		s := &sections[i]
		states := &s.BlockStates
		biomes := &s.Biomes
		s.Y = int8(int32(i) + dst.YPos)
		states.Palette, states.Data, err = writeStatesPalette(v.States)
		if err != nil {
			return
		}
		biomes.Palette, biomes.Data, err = writeBiomesPalette(v.Biomes)
		if err != nil {
			return
		}
		s.SkyLight = v.SkyLight
		s.BlockLight = v.BlockLight
	}
	dst.Sections = sections
	if dst.Heightmaps == nil {
		dst.Heightmaps = make(map[string][]uint64)
	}
	dst.Heightmaps["WORLD_SURFACE_WG"] = c.HeightMaps.WorldSurfaceWG.Raw()
	dst.Heightmaps["WORLD_SURFACE"] = c.HeightMaps.WorldSurface.Raw()
	dst.Heightmaps["OCEAN_FLOOR_WG"] = c.HeightMaps.OceanFloorWG.Raw()
	dst.Heightmaps["OCEAN_FLOOR"] = c.HeightMaps.OceanFloor.Raw()
	dst.Heightmaps["MOTION_BLOCKING"] = c.HeightMaps.MotionBlocking.Raw()
	dst.Heightmaps["MOTION_BLOCKING_NO_LEAVES"] = c.HeightMaps.MotionBlockingNoLeaves.Raw()
	dst.Status = string(c.Status)
	return
}

func writeStatesPalette(paletteData *PaletteContainer[BlocksState]) (palette []save.BlockState, data []uint64, err error) {
	rawPalette := paletteData.palette.export()
	palette = make([]save.BlockState, len(rawPalette))

	var buffer bytes.Buffer
	for i, v := range rawPalette {
		b := block.StateList[v]
		if b == nil {
			palette[i].Name = "minecraft:air"
			continue
		}
		palette[i].Name = b.ID()

		buffer.Reset()
		err = nbt.NewEncoder(&buffer).Encode(b, "")
		if err != nil {
			return
		}
		_, err = nbt.NewDecoder(&buffer).Decode(&palette[i].Properties)
		if err != nil {
			return
		}
	}

	data = make([]uint64, len(paletteData.data.Raw()))
	copy(data, paletteData.data.Raw())
	return
}

func writeBiomesPalette(paletteData *PaletteContainer[BiomesState]) (palette []save.BiomeState, data []uint64, err error) {
	rawPalette := paletteData.palette.export()
	palette = make([]save.BiomeState, len(rawPalette))

	var biomeID []byte
	for i, v := range rawPalette {
		biomeID, err = v.MarshalText()
		if err != nil {
			return
		}
		palette[i] = save.BiomeState(biomeID)
	}

	data = make([]uint64, len(paletteData.data.Raw()))
	copy(data, paletteData.data.Raw())
	return
}

// Heightmap type ordinals matching Minecraft's Heightmap.Types enum.
const (
	HeightmapWorldSurface           = 1
	HeightmapMotionBlocking         = 4
	HeightmapMotionBlockingNoLeaves = 5
)

func (c *Chunk) WriteTo(w io.Writer) (int64, error) {
	data, err := c.Data()
	if err != nil {
		return 0, err
	}

	numLightSections := len(c.Sections) + 2
	skyLightMask := make(pk.BitSet, (numLightSections+63)/64)
	blockLightMask := make(pk.BitSet, (numLightSections+63)/64)
	var skyLightArrays []pk.ByteArray
	var blockLightArrays []pk.ByteArray

	for i, v := range c.Sections {
		lightIdx := i + 1
		if v.SkyLight != nil {
			skyLightMask.Set(lightIdx, true)
			skyLightArrays = append(skyLightArrays, v.SkyLight)
		}
		if v.BlockLight != nil {
			blockLightMask.Set(lightIdx, true)
			blockLightArrays = append(blockLightArrays, v.BlockLight)
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

	light := lightData{
		SkyLightMask:   skyLightMask,
		BlockLightMask: blockLightMask,
		EmptySkyMask:   emptySkyMask,
		EmptyBlockMask: emptyBlockMask,
		SkyLight:       skyLightArrays,
		BlockLight:     blockLightArrays,
	}

	return pk.Tuple{
		&heightmapEncoder{&c.HeightMaps, len(c.Sections)},
		pk.ByteArray(data),
		pk.Array(c.BlockEntity),
		&light,
	}.WriteTo(w)
}

func (c *Chunk) ReadFrom(r io.Reader) (int64, error) {
	var data pk.ByteArray

	// Read heightmaps in 26.1 format: VarInt(count) + count × [VarInt(type) + VarInt(longCount) + longs]
	hmDec := &heightmapDecoder{HeightMaps: &c.HeightMaps, NumSections: len(c.Sections)}

	numLightSections := len(c.Sections) + 2
	light := lightData{
		SkyLightMask:   make(pk.BitSet, (numLightSections+63)/64),
		BlockLightMask: make(pk.BitSet, (numLightSections+63)/64),
		EmptySkyMask:   make(pk.BitSet, (numLightSections+63)/64),
		EmptyBlockMask: make(pk.BitSet, (numLightSections+63)/64),
		SkyLight:       []pk.ByteArray{},
		BlockLight:     []pk.ByteArray{},
	}

	n, err := pk.Tuple{
		hmDec,
		&data,
		pk.Array(&c.BlockEntity),
		&light,
	}.ReadFrom(r)
	if err != nil {
		return n, err
	}

	err = c.PutData(data)
	return n, err
}

func (c *Chunk) Data() ([]byte, error) {
	var buff bytes.Buffer
	for i := range c.Sections {
		_, err := c.Sections[i].WriteTo(&buff)
		if err != nil {
			return nil, err
		}
	}
	return buff.Bytes(), nil
}

func (c *Chunk) PutData(data []byte) error {
	r := bytes.NewReader(data)
	for i := range c.Sections {
		_, err := c.Sections[i].ReadFrom(r)
		if err != nil {
			return err
		}
	}
	return nil
}

type HeightMaps struct {
	WorldSurfaceWG         *BitStorage // test = NOT_AIR
	WorldSurface           *BitStorage // test = NOT_AIR
	OceanFloorWG           *BitStorage // test = MATERIAL_MOTION_BLOCKING
	OceanFloor             *BitStorage // test = MATERIAL_MOTION_BLOCKING
	MotionBlocking         *BitStorage // test = BlocksMotion or isFluid
	MotionBlockingNoLeaves *BitStorage // test = BlocksMotion or isFluid
}

type BlockEntity struct {
	XZ   int8
	Y    int16
	Type block.EntityType
	Data nbt.RawMessage
}

func (b BlockEntity) UnpackXZ() (X, Z int) {
	return int((uint8(b.XZ) >> 4) & 0xF), int(uint8(b.XZ) & 0xF)
}

func (b *BlockEntity) PackXZ(X, Z int) bool {
	if X > 0xF || Z > 0xF || X < 0 || Z < 0 {
		return false
	}
	b.XZ = int8(X<<4 | Z)
	return true
}

func (b BlockEntity) WriteTo(w io.Writer) (n int64, err error) {
	return pk.Tuple{
		pk.Byte(b.XZ),
		pk.Short(b.Y),
		pk.VarInt(b.Type),
		pk.NBT(b.Data),
	}.WriteTo(w)
}

func (b *BlockEntity) ReadFrom(r io.Reader) (n int64, err error) {
	return pk.Tuple{
		(*pk.Byte)(&b.XZ),
		(*pk.Short)(&b.Y),
		(*pk.VarInt)(&b.Type),
		pk.NBT(&b.Data),
	}.ReadFrom(r)
}

type Section struct {
	BlockCount int16
	States     *PaletteContainer[BlocksState]
	Biomes     *PaletteContainer[BiomesState]
	// Half a byte per light value.
	// Could be nil if not exist
	SkyLight   []byte // len() == 2048
	BlockLight []byte // len() == 2048
}

func (s *Section) GetBlock(i int) BlocksState {
	return s.States.Get(i)
}

func (s *Section) SetBlock(i int, v BlocksState) {
	if !block.IsAir(s.States.Get(i)) {
		s.BlockCount--
	}
	if !block.IsAir(v) {
		s.BlockCount++
	}
	s.States.Set(i, v)
}

func (s *Section) WriteTo(w io.Writer) (int64, error) {
	return pk.Tuple{
		pk.Short(s.BlockCount),
		s.States,
		s.Biomes,
	}.WriteTo(w)
}

func (s *Section) ReadFrom(r io.Reader) (int64, error) {
	return pk.Tuple{
		(*pk.Short)(&s.BlockCount),
		s.States,
		s.Biomes,
	}.ReadFrom(r)
}

type lightData struct {
	SkyLightMask   pk.BitSet
	BlockLightMask pk.BitSet
	EmptySkyMask   pk.BitSet
	EmptyBlockMask pk.BitSet
	SkyLight       []pk.ByteArray
	BlockLight     []pk.ByteArray
}

func (l *lightData) WriteTo(w io.Writer) (int64, error) {
	// 26.1: no Trust Edges boolean — just 4 BitSets + 2 arrays
	return pk.Tuple{
		l.SkyLightMask,
		l.BlockLightMask,
		l.EmptySkyMask,
		l.EmptyBlockMask,
		pk.Array(l.SkyLight),
		pk.Array(l.BlockLight),
	}.WriteTo(w)
}

func (l *lightData) ReadFrom(r io.Reader) (int64, error) {
	return pk.Tuple{
		&l.SkyLightMask,
		&l.BlockLightMask,
		&l.EmptySkyMask,
		&l.EmptyBlockMask,
		pk.Array(&l.SkyLight),
		pk.Array(&l.BlockLight),
	}.ReadFrom(r)
}

// heightmapEncoder writes heightmaps in 26.1 format:
// VarInt(count) + count × [VarInt(typeOrdinal) + VarInt(longCount) + longs]
type heightmapEncoder struct {
	hm          *HeightMaps
	numSections int
}

func (h *heightmapEncoder) WriteTo(w io.Writer) (int64, error) {
	bitsForHeight := bits.Len(uint(h.numSections)*16 + 1)

	type hmEntry struct {
		ordinal int
		storage *BitStorage
	}

	entries := []hmEntry{
		{HeightmapWorldSurface, h.hm.WorldSurface},
		{HeightmapMotionBlocking, h.hm.MotionBlocking},
		{HeightmapMotionBlockingNoLeaves, h.hm.MotionBlockingNoLeaves},
	}

	// Ensure all entries have valid storage
	for i := range entries {
		if entries[i].storage == nil {
			entries[i].storage = NewBitStorage(bitsForHeight, 16*16, nil)
		}
	}

	var n int64
	nn, err := pk.VarInt(len(entries)).WriteTo(w)
	n += nn
	if err != nil {
		return n, err
	}

	for _, e := range entries {
		nn, err = pk.VarInt(e.ordinal).WriteTo(w)
		n += nn
		if err != nil {
			return n, err
		}

		raw := e.storage.Raw()
		nn, err = pk.VarInt(len(raw)).WriteTo(w)
		n += nn
		if err != nil {
			return n, err
		}

		for _, l := range raw {
			err = binary.Write(w, binary.BigEndian, int64(l))
			n += 8
			if err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

func (h *heightmapEncoder) ReadFrom(io.Reader) (int64, error) {
	panic("heightmapEncoder does not support ReadFrom; use heightmapDecoder")
}

// heightmapDecoder reads heightmaps in 26.1 format.
type heightmapDecoder struct {
	HeightMaps  *HeightMaps
	NumSections int
}

func (h *heightmapDecoder) ReadFrom(r io.Reader) (int64, error) {
	var count pk.VarInt
	n, err := count.ReadFrom(r)
	if err != nil {
		return n, err
	}

	bitsForHeight := bits.Len(uint(h.NumSections)*16 + 1)

	for i := 0; i < int(count); i++ {
		var typeOrd pk.VarInt
		nn, err := typeOrd.ReadFrom(r)
		n += nn
		if err != nil {
			return n, err
		}

		var longCount pk.VarInt
		nn, err = longCount.ReadFrom(r)
		n += nn
		if err != nil {
			return n, err
		}

		longs := make([]uint64, int(longCount))
		for j := range longs {
			var v int64
			err = binary.Read(r, binary.BigEndian, &v)
			n += 8
			if err != nil {
				return n, err
			}
			longs[j] = uint64(v)
		}

		bs := NewBitStorage(bitsForHeight, 16*16, longs)
		switch int(typeOrd) {
		case HeightmapWorldSurface:
			h.HeightMaps.WorldSurface = bs
		case HeightmapMotionBlocking:
			h.HeightMaps.MotionBlocking = bs
		case HeightmapMotionBlockingNoLeaves:
			h.HeightMaps.MotionBlockingNoLeaves = bs
		}
	}
	return n, nil
}

func (h *heightmapDecoder) WriteTo(io.Writer) (int64, error) {
	panic("heightmapDecoder does not support WriteTo; use heightmapEncoder")
}
