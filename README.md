# Go-MC (Fork)

![Version](https://img.shields.io/badge/Minecraft-26.1-blue.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/Tnze/go-mc.svg)](https://pkg.go.dev/github.com/Tnze/go-mc)

> This is a fork of [Tnze/go-mc](https://github.com/Tnze/go-mc), a collection of Go libraries for building Minecraft clients and servers.

## Project Goals

### Primary: Fully functional Minecraft 26.1 server in Go

Build a complete, production-ready Minecraft 26.1 server written entirely in Go with Mojang authentication. No Java on the server side if we can avoid it.

### Secondary: NeoForge mod compatibility

Support modded [NeoForge](https://neoforged.net/) 26.1 Java clients by implementing server-side mod logic natively in Go. We are **not** running Java -- we port mod functionality from Java to Go as standalone modules that can be enabled or disabled at compile time.

Reference NeoForge version: [26.1.0.0-alpha.1](https://maven.neoforged.net/releases/net/neoforged/neoforge/26.1.0.0-alpha.1%2Bsnapshot-1/neoforge-26.1.0.0-alpha.1%2Bsnapshot-1-changelog.txt)

### Mod Porting Roadmap

1. **[Chat Heads](https://github.com/dzwdz/chat_heads)** -- First target. A small, self-contained mod to validate the porting approach. Implemented as a standalone Go module with compile-time enable/disable.
2. **[Create](https://github.com/Creators-of-Create/Create)** -- Ultimate goal. Full server-side implementation of the Create mod's mechanics in Go.

### Architecture: Distributed on Kubernetes

The server is designed from the ground up to run in a distributed fashion on Kubernetes, with horizontal scaling across nodes for world regions, player sessions, and game systems.

## Inspiration

- [Tnze/go-mc](https://github.com/Tnze/go-mc) -- The upstream project this fork is based on
- [minekube/gate](https://github.com/minekube/gate) -- Protocol support and proxy architecture reference

## Status

- [x] Minecraft network protocol
- [x] Server framework
- [x] Dual role RCON protocol (Server & Client)
- [x] Chat Message (JSON and legacy `§` format)
- [x] NBT (reflection-based)
- [x] SNBT
- [x] Regions & Chunks & Blocks
- [x] Mojang authentication
- [ ] Minecraft 26.1 protocol support
- [ ] NeoForge handshake / mod negotiation
- [ ] Chat Heads mod (Go port)
- [ ] Create mod (Go port)
- [ ] Kubernetes-native distributed architecture

> API stability is not guaranteed while we work toward 26.1 support.

Require Go version: 1.22

## Getting Started

```sh
go get github.com/Tnze/go-mc@26.1
```

### Run Examples

- Ping a server: `go run github.com/Tnze/go-mc/examples/mcping localhost`
- Join a local server: `go run github.com/Tnze/go-mc/examples/daze`
