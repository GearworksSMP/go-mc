# Go-MC (Fork)

![Version](https://img.shields.io/badge/Minecraft-26.1-blue.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/Tnze/go-mc.svg)](https://pkg.go.dev/github.com/Tnze/go-mc)

> This is a fork of [Tnze/go-mc](https://github.com/Tnze/go-mc), a collection of Go libraries for building Minecraft clients and servers.

## Project Goals

### Primary: Fully functional Minecraft 26.1 server in Go

Build a complete, production-ready Minecraft 26.1 server written entirely in Go with Mojang authentication. No Java on the server side if we can avoid it.

### Secondary: NeoForge mod compatibility

Support modded [NeoForge](https://neoforged.net/) 26.1 Java clients by implementing server-side mod logic natively in Go. We are **not** running Java -- we port mod functionality from Java to Go as standalone modules that can be enabled or disabled at compile time.

Reference NeoForge version: [26.1.0.1-beta](https://neoforged.net/news/26.1release/) (likely the next stable modding version). NeoForge 26.1 removed obfuscation entirely — Mojang ships official parameter names, making protocol analysis and mod porting easier.

### Mod Porting Roadmap

1. **[Chat Heads](https://github.com/dzwdz/chat_heads)** -- First target. A small, self-contained mod to validate the porting approach. Implemented as a standalone Go module with compile-time enable/disable.
2. **[Create](https://github.com/Creators-of-Create/Create)** -- Ultimate goal. Full server-side implementation of the Create mod's mechanics in Go.

### Architecture: Distributed on Kubernetes

The server is designed from the ground up to run in a distributed fashion on Kubernetes, with horizontal scaling across nodes for world regions, player sessions, and game systems.

## Inspiration

- [Tnze/go-mc](https://github.com/Tnze/go-mc) -- The upstream project this fork is based on
- [minekube/gate](https://github.com/minekube/gate) -- Protocol support and proxy architecture reference

## Status

### Core Libraries
- [x] Minecraft network protocol
- [x] Server framework
- [x] Dual role RCON protocol (Server & Client)
- [x] Chat Message (JSON and legacy `§` format)
- [x] NBT (reflection-based)
- [x] SNBT
- [x] Regions & Chunks & Blocks
- [x] Mojang authentication

### Minecraft 26.1 Protocol
- [x] 26.1 protocol support (packet IDs, wire formats, login sequence)
- [x] PalettedContainer / chunk encoding (26.1 format)
- [x] Heightmap encoding (ByteBufCodecs format)
- [x] Mojang authentication with encryption
- [x] Configuration phase (registry sync, feature flags)

### Gameplay Features

**World**
- [x] Day/night cycle and world time
- [x] Weather (rain/thunder state broadcast)
  - [ ] Lightning strikes, weather-driven mob spawning
- [ ] Terrain generation (partial)
  - [x] Simplex noise heightmap, ore placement, basic trees
  - [x] Nether and End dimension generation (basic)
  - [ ] Caves, aquifers, deep dark
  - [ ] Biome-accurate generation (vanilla parity)
  - [ ] Most structures (ancient cities, trail ruins, etc.)
- [ ] Structures (partial)
  - [x] Villages, mineshafts (basic placement)
  - [ ] Proper loot tables, structure-specific mobs

**Blocks & Items**
- [x] Block breaking and placing
- [x] Creative inventory
- [x] Crafting (3x3 grid, crafting table)
- [ ] Furnace / smelting (partial)
  - [x] Smelting recipes, burn time tracking, output
  - [x] Blast furnace, smoker (delegated from furnace)
  - [ ] Proper fuel types, XP from smelting
- [ ] Brewing (partial)
  - [x] Brewing stand tick system
  - [ ] Full ingredient validation, all potion recipes
- [ ] Enchanting (partial)
  - [x] Random offer generation, cost display
  - [ ] Proper level weighting, multi-enchant combinations
- [ ] Anvil (partial)
  - [x] Renaming, repair cost, window management
  - [ ] Full enchantment merging logic
- [ ] Containers (partial)
  - [x] Single/double chests, barrels (basic)
  - [x] Hoppers, dispensers (basic item transfer)
  - [ ] Proper priority ordering, full container interactions
- [x] Signs (editing, text sync)
- [ ] Crops (partial)
  - [x] Growth stages, bonemeal
  - [ ] Proper growth rates, environmental factors
- [ ] Fluids (partial)
  - [x] Simplified flow mechanics
  - [ ] Source vs flowing blocks, realistic hydrology
- [ ] Grindstone, stonecutter, smithing table (basic UI only)

**Combat & Survival**
- [ ] Melee combat (partial)
  - [x] Player/mob damage, basic knockback
  - [x] Sharpness, Smite enchantments
  - [ ] Critical hits, sweeping edge, attack cooldown
- [ ] Ranged combat (partial)
  - [x] Bow draw state, arrow/projectile spawning
  - [x] Crossbow loading
  - [ ] Projectile physics (gravity, collision, trajectory)
  - [ ] Trident mechanics
- [ ] Armor & shields (partial)
  - [x] Damage reduction from armor
  - [x] Basic shield blocking
  - [ ] Full protection enchantments, durability
- [x] Fall damage
- [x] Void damage
- [ ] Food & hunger (partial)
  - [x] Hunger/saturation tracking, food nutrition table (70+ items)
  - [ ] Exhaustion from actions, natural regeneration accuracy
- [ ] Potion effects (partial)
  - [x] Speed, Slowness, Strength, Weakness (with duration)
  - [ ] Most other effects (Resistance, Invisibility, Regeneration, Fire Resistance, etc.)
- [ ] TNT & explosions (partial)
  - [x] Ignition, chain reactions, basic damage
  - [ ] Proper explosion physics, block breaking, damage falloff
- [ ] Fire (partial)
  - [x] Basic fire spreading, player damage
  - [ ] Proper spread rates, random tick integration

**Mobs**
- [ ] Mob spawning (partial)
  - [x] Hostile and passive spawning, despawning
  - [x] Mob spawner block interaction
  - [ ] Proper spawn conditions, rates, light level checks
- [ ] Mob AI (partial)
  - [x] A* pathfinding, target tracking, basic attacks
  - [ ] Behavior trees, state machines, environmental awareness
  - [ ] Most mob-specific behaviors (Creeper explosions, Skeleton aiming, etc.)
- [ ] Tameable mobs (partial)
  - [x] Basic taming, sit state
  - [ ] Ownership persistence, collar colors
- [ ] Villagers (partial)
  - [x] Merchant UI, basic trading
  - [ ] Profession trades, reputation, discounts
- [x] Mob persistence (save/load to database)
- [ ] Ender Dragon (spawning and basic health, missing proper fight sequence)

**Movement & Physics**
- [x] Position/rotation sync between players
- [x] View distance culling (chunks sent only in range)
- [ ] Elytra (partial — gliding state only, no physics)
- [ ] Boats & minecarts (partial — mounting/dismounting, no physics)
- [ ] Portals (partial — dimension switching, no portal creation/linking)
- [ ] Falling blocks (partial — entity spawning, no collision/landing)
- [ ] Proper climbing, swimming, velocity handling

**Other Systems**
- [x] Chat messaging and command routing
- [x] Keepalive
- [x] Player animations (arm swing, sprint)
- [x] Particle effects (broadcasting)
- [x] Sound effects (broadcasting with categories)
- [x] Scoreboard (health display)
- [x] Beds and spawn points
- [ ] Commands (partial)
  - [x] `/tp`, `/gamemode`, `/weather`, `/time`, `/give`
  - [ ] Most vanilla commands, permission system
- [ ] Advancements (partial)
  - [x] Grant system, toast notifications (~25 advancements)
  - [ ] 500+ vanilla advancements, automatic triggers
- [ ] Redstone (partial)
  - [x] Levers, buttons, pressure plates
  - [x] Iron doors/trapdoors, redstone lamps
  - [x] Redstone dust power propagation (basic)
  - [ ] Observers, comparators, repeaters
  - [ ] Complex circuits, proper 3D signal propagation
- [ ] Pistons (partial)
  - [x] Basic push/pull
  - [ ] Sticky piston retraction, block-specific rules

### Infrastructure
- [x] PostgreSQL persistence (chunks, players, mobs)
- [x] OpenTelemetry tracing

### Distributed Architecture
- [x] Proxy server (`cmd/proxy/`) — routes clients to region servers
- [x] Region servers (`cmd/region/`) — per-region game instances
- [x] Redis-backed cluster state and session tracking
- [x] Docker Compose profiles (standalone + cluster)
- [ ] Kubernetes deployment manifests

### Mod Compatibility
- [ ] NeoForge handshake / mod negotiation
- [ ] Chat Heads mod (Go port)
- [ ] Create mod (Go port)

> API stability is not guaranteed.

Require Go version: 1.22

## Getting Started

```sh
go get github.com/Tnze/go-mc@26.1
```

### Run the Server

**Standalone (single instance):**

```sh
# Start PostgreSQL (optional, for persistence)
docker compose up -d postgres

# Run the server
go run ./cmd/server261/
```

Environment variables: `WORLD_TYPE` (`flat` or terrain), `WORLD_SEED`, `DATABASE_URL` (PostgreSQL).

**Distributed (proxy + region servers):**

```sh
docker compose --profile cluster up
```

### Run Examples

- Ping a server: `go run github.com/Tnze/go-mc/examples/mcping localhost`
- Join a local server: `go run github.com/Tnze/go-mc/examples/daze`
