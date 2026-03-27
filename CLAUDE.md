# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Fork of [Tnze/go-mc](https://github.com/Tnze/go-mc) targeting Minecraft 26.1. Pure Go server with Mojang auth, NeoForge client compatibility (mods ported from Java to Go), and Kubernetes-native distributed architecture. No Java on the server side.

## Build & Test Commands

```bash
go build ./...           # Build everything
go test ./...            # Run all tests
go test ./nbt/...        # Run tests for a specific package
go test -run TestName ./chat/  # Run a single test
go generate ./...        # Regenerate data files (entities, items, blocks, sounds, languages)
```

CI tests against Go 1.22 (minimum) and latest. CI also builds examples: `go build -o ./bin/tools ./examples/...`

## Architecture

### Packet Serialization (`net/packet/`)

The entire protocol is built on two interfaces that mirror `io.WriterTo`/`io.ReaderFrom`:

- `FieldEncoder` — `WriteTo(w io.Writer) (int64, error)`
- `FieldDecoder` — `ReadFrom(r io.Reader) (int64, error)`
- `Field` — implements both

Every protocol type (`VarInt`, `String`, `Double`, `Boolean`, `Position`, `NBT`, etc.) implements `Field`. Packets are built and parsed with:

```go
p := pk.Marshal(packetID, pk.String(name), pk.VarInt(protocol))  // build
err := p.Scan(&name, &protocol)                                   // parse
```

Helper types: `pk.Array` (VarInt-prefixed slices), `pk.Option` (Boolean-prefixed optionals), `pk.Opt` (custom-condition optionals).

### Network Layer (`net/`)

`net.Conn` wraps a TCP socket and handles framing, zlib compression (`SetThreshold`), and AES/CFB8 encryption (`SetCipher`). `ReadPacket`/`WritePacket` operate on `pk.Packet{ID, Data}`.

### Bot Framework (`bot/`)

Event-driven client framework. Modules register packet handlers on `Client.Events` with priority ordering (higher = first):

```go
c.Events.AddListener(bot.PacketHandler{ID: packetid.ClientboundLogin, Priority: 64, F: handler})
```

Sub-packages are optional modules: `bot/basic` (keepalive, settings, player/world info), `bot/msg` (chat), `bot/world` (chunks), `bot/playerlist`. Modules depend on each other explicitly (e.g., `msg` depends on `basic` and `playerlist`).

### Server Framework (`server/`)

Three-phase connection lifecycle driven by interfaces:

1. **ListPingHandler** — responds to server list pings
2. **LoginHandler** — authentication (Mojang or offline), encryption, compression
3. **GamePlay** — `AcceptPlayer(name, id, pubKey, properties, protocol, conn)` — connection stays open until this returns

`server/auth` provides Mojang authentication with RSA key exchange. A configuration stage between login and gameplay handles registry sync, resource packs, and feature negotiation.

### NBT (`nbt/`)

Reflection-based encoder/decoder modeled after `encoding/json`. Struct tags: `nbt:"name,omitempty"` and `nbtkey:"name"` (for keys containing commas). Special types: `nbt.RawMessage`, `nbt.StringifiedMessage` (SNBT), `dynbt.Value` (dynamic manipulation).

### Data Generation (`data/`)

`go:generate` scripts download from PrismarineJS minecraft-data and Minecraft wiki to produce Go source for entity types, items, blocks, sound IDs, packet IDs, and language strings. Update `version` constants in `gen_*.go` files then run `go generate ./...`.

Block states are stored as an embedded gzip-compressed NBT file (`level/block/block_states.nbt`) decompressed at init time into `ToStateID`/`StateList` maps.

### Chat (`chat/`)

`chat.Message` implements both `pk.Field` (network serialization) and `json.Marshaler`/`json.Unmarshaler`. Supports JSON format, legacy `§` color codes, translate messages, click/hover events. `.String()` produces ANSI-colored output; `.ClearString()` produces plain text.

### Registry (`registry/`)

Generic `Registry[E]` type mapping string keys to integer IDs with tag support. Used for blocks, items, entities, dimensions, etc.

## Minecraft 26.1 Protocol Changes

The upstream go-mc code targets older Minecraft versions. Several wire formats changed in 26.1 that require careful attention.

### PalettedContainer (chunk section serialization)

26.1 **removed the `VarInt(dataLongsCount)` prefix** before the data long array in `PalettedContainer`. The data array length is now inferred from `bitsPerEntry`:

```
Old format: Byte(bitsPerEntry) + palette + VarInt(dataLongsCount) + longs
New format: Byte(bitsPerEntry) + palette + longs  (count = ceil(entries * bits / 64))
```

For single-value palettes (bitsPerEntry=0), there is no data array at all — just `Byte(0) + VarInt(value)`. An empty section is 6 bytes, not 8. The `level/bitstorage.go` WriteTo/ReadFrom were updated to use the new format (commit 040a9e0).

### Chunk packet (LevelChunkWithLight)

- **Heightmaps**: Changed from NBT compound to `ByteBufCodecs.map(Heightmap.Types, LONG_ARRAY)`. Format: `VarInt(count) + count × [VarInt(typeOrdinal) + VarInt(longCount) + longs]`. Required types: WORLD_SURFACE(1), MOTION_BLOCKING(4), MOTION_BLOCKING_NO_LEAVES(5). Sending empty heightmaps causes blocks to be invisible.
- **Light data**: Removed the Trust Edges boolean. Now just 4 BitSets + 2 Lists.
- **ChunkBatchFinished**: Added `VarInt batchSize` field.

### Spawn sequence

The vanilla 26.1 server sends packets in this order (verified by packet capture):

1. Login (JoinGame) — includes `VarInt seaLevel` field added in 1.21.2
2. PlayerAbilities (flags=0x0D for creative)
3. **PlayerPosition** — must come BEFORE chunks
4. SetDefaultSpawnPosition — uses RespawnData format: `Identifier(dimension) + Position + Float(yaw) + Float(pitch)`
5. GameEvent(13, 0) — start waiting for chunks
6. SetChunkCacheCenter
7. ChunkBatchStart → Chunks → ChunkBatchFinished(batchSize)

### Protocol constants

- Version: `0x40000120` (1073742112), name: `26.1-snapshot-2`
- Packet IDs are in `data/packetid/packetid.go` (generated via iota)

## NeoForge 26.1 Compatibility Notes

NeoForge 26.1 is in **beta** (`26.1.0.1-beta`) as of March 2026 and is likely the next stable modding version. Key points affecting this project:

- **Obfuscation removed.** Minecraft 26.1 ships with official Mojang parameter names. This makes protocol analysis and Java mod source reading significantly easier.
- **Java 25 required** by NeoForge/vanilla. Our server doesn't run Java, but the vanilla JAR in `.cache/` needs Java 25 for packet capture comparisons.
- **ItemStack/FluidStack now require loaded registries** to instantiate. This reflects a deeper change in how item data is structured — data components are part of the wire protocol. This may affect item serialization if vanilla updates the wire format.
- **New NeoForge versioning:** `<mc.major>.<mc.minor>.<mc.patch>.<neoforge.build>-<suffix>` (e.g., `26.1.0.10-beta`).
- NeoForge API is still unstable (beta) — breaking changes are expected.

### Distributed Architecture (`cmd/proxy/`, `cmd/region/`, `cluster/`)

The server supports two deployment modes:

1. **Standalone** (`cmd/server261/`) — single-instance server with all gameplay systems
2. **Clustered** (`cmd/proxy/` + `cmd/region/`) — proxy routes clients to region servers, Redis for state sync, range-based entity ID allocation

Docker Compose supports both modes: `docker compose up` (standalone) or `docker compose --profile cluster up` (distributed).

## Debugging Tools

- `cmd/server261/` — minimal working 26.1 server with headless integration tests (`TestChunkBlockContent` verifies chunk format)
- `cmd/chunkdump/` — connects to a server and dumps the play-phase packet sequence with hex output
- `cmd/chunkcompare/` — byte-level comparison of two chunk binary files
- `.cache/server-26.1-ss2.jar` — vanilla 26.1 server JAR for packet capture comparison

## Common Pitfalls

- **`bytes.Buffer.Bytes()` returns shared memory.** If you call `buf.Bytes()`, then `buf.Reset()` and write new data, the original slice is overwritten. Always copy with `append([]byte(nil), buf.Bytes()...)` if you need to keep the data.
- **`level/chunk.go` was updated for 26.1** (commit 040a9e0). The `Chunk.WriteTo()` now uses the new heightmap and PalettedContainer formats. Verify against vanilla captures if changing chunk encoding.

## Reference Data

`reference/` contains Minecraft Wiki XML dumps organized by category (blocks, items, mobs, gameplay, enchantments, biomes, etc.). Use these as reference when implementing game mechanics. Focus exclusively on **Java Edition** behavior — ignore Bedrock Edition differences. Re-export with `reference/export_wiki.py` if needed.

## External Documentation

`external-docs/` contains downloaded wiki pages for the Create mod, useful as reference when porting Create mod functionality to Go.

## Module Path

`github.com/Tnze/go-mc` — Go 1.22 minimum.
