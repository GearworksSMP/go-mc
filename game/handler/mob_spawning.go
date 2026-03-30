package handler

import (
	"math"
	"math/rand"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// SpawnMobAt spawns a mob of the given type at the specified position and returns its EID.
func (m *MobManager) SpawnMobAt(typeID int32, x, y, z float64) int32 {
	health, damage, speed, hostile := mobDefaults(typeID)
	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    typeID,
		X:         x,
		Y:         y,
		Z:         z,
		PrevX:     x,
		PrevY:     y,
		PrevZ:     z,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     speed,
		WanderYaw: rand.Float32() * 360,
		Hostile:   hostile,
	}
	if typeID == MobTypeSheep {
		mob.WoolColor = 0 // default white for summoned sheep
	}
	if isLlamaType(typeID) {
		mob.TameData = &TameableMobData{
			CarpetColor:   -1,
			LlamaStrength: 1 + rand.Int31n(5),
		}
	}
	m.mu.Lock()
	m.Mobs[eid] = mob
	m.Spatial.Insert(eid, x, z)
	m.mu.Unlock()
	m.broadcastSpawn(mob)
	return eid
}

// countMobsNear counts mobs within radiusSq (squared distance) of (x, z).
// If hostile is true, counts only hostile mobs; if false, counts only passive mobs.
func (m *MobManager) countMobsNear(x, z, radiusSq float64, hostile bool) int {
	count := 0
	for _, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		if mob.Hostile != hostile {
			continue
		}
		dx := mob.X - x
		dz := mob.Z - z
		if dx*dx+dz*dz <= radiusSq {
			count++
		}
	}
	return count
}

// trySpawn attempts to spawn hostile mobs near each survival player.
// Uses per-player mob caps and dimension-aware mob selection:
// - Overworld: night-only, standard hostile mobs
// - Nether: always spawns, nether-specific mobs
// - End: always spawns, endermen only
func (m *MobManager) trySpawn() {
	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && p.GameMode == 0 {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	for _, player := range players {
		px, _, pz := player.Position()

		// Per-player cap check: count hostile mobs within 128 blocks
		nearbyHostile := m.countMobsNear(px, pz, 128*128, true)
		if nearbyHostile >= hostileCapPerPlayer {
			continue
		}

		spawnX, spawnY, spawnZ, ok := m.pickSpawnPos(player)
		if !ok {
			continue
		}

		// Dimension-aware spawning
		switch player.Dimension {
		case "minecraft:the_nether":
			m.spawnNetherHostileAt(spawnX, float64(spawnY), spawnZ)
		case "minecraft:the_end":
			m.spawnEndHostileAt(spawnX, float64(spawnY), spawnZ)
		default:
			if !m.TimeMgr.IsNight() {
				continue
			}
			m.spawnOverworldHostileAt(spawnX, float64(spawnY), spawnZ)
		}
	}
}

// pickSpawnPos picks a random spawn position 24-48 blocks from the player and
// returns the coordinates. ok is false if no valid surface was found.
func (m *MobManager) pickSpawnPos(player *game.Player) (x float64, y int, z float64, ok bool) {
	px, _, pz := player.Position()
	angle := rand.Float64() * 2 * math.Pi
	dist := 24 + rand.Float64()*24
	x = px + math.Cos(angle)*dist
	z = pz + math.Sin(angle)*dist
	y = m.findSurfaceY(int(x), int(z))
	ok = y >= m.MinY
	return
}

// spawnOverworldHostileAt spawns a hostile mob at the given position using overworld mob table.
func (m *MobManager) spawnOverworldHostileAt(x, y, z float64) {
	var typeID int32
	var health, damage float32
	var speed float64
	roll := rand.Float64()
	switch {
	case roll < 0.40:
		typeID, health, damage, speed = MobTypeZombie, 20, 3, 0.115
	case roll < 0.60:
		typeID, health, damage, speed = MobTypeSkeleton, 20, 2, 0.1
	case roll < 0.75:
		typeID, health, damage, speed = MobTypeSpider, 16, 2, 0.15
	case roll < 0.85:
		typeID, health, damage, speed = MobTypeCreeper, 20, 0, 0.1
	case roll < 0.90:
		typeID, health, damage, speed = MobTypeEnderman, 40, 7, 0.15
	case roll < 0.95:
		typeID, health, damage, speed = MobTypeWitch, 26, 3, 0.1
	default:
		typeID, health, damage, speed = MobTypeSpider, 16, 2, 0.15
	}

	m.spawnHostileMob(x+0.5, y, z+0.5, typeID, health, damage, speed)
}

// spawnNetherHostileAt spawns a nether-specific hostile mob at the given position.
func (m *MobManager) spawnNetherHostileAt(x, y, z float64) {
	var typeID int32
	var health, damage float32
	var speed float64
	roll := rand.Float64()
	switch {
	case roll < 0.30:
		typeID, health, damage, speed = MobTypeZombifiedPiglin, 20, 5, 0.1
	case roll < 0.50:
		typeID, health, damage, speed = MobTypePiglin, 16, 5, 0.1
	case roll < 0.65:
		typeID, health, damage, speed = MobTypeMagmaCube, 16, 3, 0.1
	case roll < 0.80:
		typeID, health, damage, speed = MobTypeGhast, 10, 6, 0.05
	case roll < 0.90:
		typeID, health, damage, speed = MobTypeWitherSkeleton, 20, 8, 0.1
	default:
		typeID, health, damage, speed = MobTypeBlaze, 20, 6, 0.1
	}

	m.spawnHostileMob(x+0.5, y, z+0.5, typeID, health, damage, speed)
}

// spawnEndHostileAt spawns an enderman at the given position in the End.
func (m *MobManager) spawnEndHostileAt(x, y, z float64) {
	m.spawnHostileMob(x+0.5, y, z+0.5, MobTypeEnderman, 40, 7, 0.15)
}

// spawnHostileMob creates a hostile mob at the given position and broadcasts it.
func (m *MobManager) spawnHostileMob(x, y, z float64, typeID int32, health, damage float32, speed float64) {
	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    typeID,
		X:         x,
		Y:         y,
		Z:         z,
		PrevX:     x,
		PrevY:     y,
		PrevZ:     z,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     speed,
		WanderYaw: rand.Float32() * 360,
		Hostile:   true,
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
}

// trySpawnPassive attempts to spawn passive mobs on grass during daytime.
// Since passive mobs no longer despawn, spawning is conservative:
// only spawns if fewer than passiveCapPerPlayer passive mobs are near the target player.
func (m *MobManager) trySpawnPassive() {
	if m.TimeMgr.IsNight() {
		return
	}

	// Only spawn passive mobs in the overworld
	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && (p.Dimension == "" || p.Dimension == "minecraft:overworld") {
			players = append(players, p)
		}
	})

	for _, player := range players {
		px, _, pz := player.Position()

		// Per-player cap: only spawn if fewer than passiveCapPerPlayer passive mobs nearby
		nearbyPassive := m.countMobsNear(px, pz, 128*128, false)
		if nearbyPassive >= passiveCapPerPlayer {
			continue
		}

		// Pick random position 8-32 blocks from player
		angle := rand.Float64() * 2 * math.Pi
		dist := 8 + rand.Float64()*24
		spawnX := px + math.Cos(angle)*dist
		spawnZ := pz + math.Sin(angle)*dist

		// Find surface Y
		spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
		if spawnY < m.MinY {
			continue
		}

		// Check that the block below is grass_block
		belowState, err := m.World.GetBlock(int(spawnX), spawnY-1, int(spawnZ))
		if err != nil {
			continue
		}
		blockName := BlockNameFromState(int(belowState))
		if blockName != "grass_block" {
			continue
		}

		m.spawnPassiveMobAt(spawnX+0.5, float64(spawnY), spawnZ+0.5)
	}
}

// spawnPassiveMobAt creates a random passive mob at the given position.
func (m *MobManager) spawnPassiveMobAt(x, y, z float64) {
	var typeID int32
	var health float32
	var vdata *VillagerData
	var tdata *TameableMobData
	roll := rand.Float64()
	switch {
	case roll < 0.20:
		typeID, health = MobTypeCow, 10
	case roll < 0.40:
		typeID, health = MobTypePig, 10
	case roll < 0.54:
		typeID, health = MobTypeSheep, 8
	case roll < 0.68:
		typeID, health = MobTypeChicken, 4
	case roll < 0.76:
		typeID, health = MobTypeVillager, 20
		vdata = NewVillagerData(RandomProfession())
	case roll < 0.84:
		typeID, health = MobTypeWolf, 8
	case roll < 0.90:
		typeID, health = MobTypeCat, 10
	case roll < 0.96:
		typeID, health = MobTypeHorse, 15 + float32(rand.Intn(16))
		tdata = &TameableMobData{
			HorseSpeed:   0.1 + rand.Float64()*0.05,
			HorseJump:    0.4 + rand.Float64()*0.3,
			HorseVariant: rand.Int31n(7)*256 + rand.Int31n(5),
		}
	case roll < 0.98:
		typeID, health = MobTypeLlama, 22
		tdata = &TameableMobData{
			CarpetColor:   -1,
			LlamaStrength: 1 + rand.Int31n(5),
		}
	default:
		typeID, health = MobTypeParrot, 6
	}

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:          eid,
		TypeID:       typeID,
		X:            x,
		Y:            y,
		Z:            z,
		PrevX:        x,
		PrevY:        y,
		PrevZ:        z,
		Health:       health,
		MaxHealth:    health,
		Damage:       0,
		Speed:        0.1,
		WanderYaw:    rand.Float32() * 360,
		Hostile:      false,
		VillagerData: vdata,
		TameData:     tdata,
	}
	// Assign random wool color for sheep (vanilla distribution)
	if typeID == MobTypeSheep {
		r := rand.Float64()
		switch {
		case r < 0.8184: // white (81.84%)
			mob.WoolColor = 0
		case r < 0.8184+0.05: // orange
			mob.WoolColor = 1
		case r < 0.8184+0.10: // magenta
			mob.WoolColor = 2
		case r < 0.8184+0.15: // light blue
			mob.WoolColor = 3
		case r < 0.9684: // black (3%)
			mob.WoolColor = 15
		case r < 0.9884: // gray (2%)
			mob.WoolColor = 7
		case r < 0.9984: // brown (1%)
			mob.WoolColor = 12
		default: // pink (0.16%)
			mob.WoolColor = 6
		}
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
}

// findSurfaceY returns the Y coordinate of the surface at (x, z).
func (m *MobManager) findSurfaceY(x, z int) int {
	// Scan from top down
	maxY := m.MinY + 24*16 - 1
	for y := maxY; y >= m.MinY; y-- {
		state, err := m.World.GetBlock(x, y, z)
		if err != nil {
			continue
		}
		if state != 0 { // not air
			// Check block above is air
			above, err := m.World.GetBlock(x, y+1, z)
			if err == nil && above == 0 {
				above2, err2 := m.World.GetBlock(x, y+2, z)
				if err2 == nil && above2 == 0 {
					return y + 1
				}
			}
		}
	}
	return m.MinY - 1
}

// despawnFarMobs removes hostile mobs using vanilla-like tiered despawn rules.
// Passive mobs never naturally despawn. Named and tamed mobs never despawn.
func (m *MobManager) despawnFarMobs() {
	var toRemove []int32
	for eid, mob := range m.Mobs {
		if mob.Health <= 0 || mob.DeathTick > 0 {
			continue
		}
		// Never despawn tamed mobs
		if mob.TameData != nil && mob.TameData.Tamed {
			continue
		}
		// Never despawn named mobs (nametag)
		if mob.CustomName != "" {
			continue
		}
		// Passive mobs never naturally despawn (vanilla behavior)
		if !mob.Hostile {
			continue
		}

		// Find squared distance to nearest player
		minDistSq := math.MaxFloat64
		m.Manager.ForEach(func(p *game.Player) {
			px, _, pz := p.Position()
			dx := px - mob.X
			dz := pz - mob.Z
			distSq := dx*dx + dz*dz
			if distSq < minDistSq {
				minDistSq = distSq
			}
		})

		// Tiered despawn:
		// > 128 blocks: instant despawn
		// 32-128 blocks: ~22% chance per check (approximates vanilla 1/800 per tick over 200 ticks)
		// < 32 blocks: never despawn
		if minDistSq > 128*128 {
			toRemove = append(toRemove, eid)
		} else if minDistSq > 32*32 && rand.Float64() < 0.22 {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		m.removeMobEntity(m.Mobs[eid])
		m.Spatial.Remove(eid)
		delete(m.Mobs, eid)
	}
}

// removeMobEntity sends RemoveEntities for a mob.
func (m *MobManager) removeMobEntity(mob *Mob) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(mob.EID),
	)
	m.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
}

// broadcastSpawn sends AddEntity for a mob to all players.
func (m *MobManager) broadcastSpawn(mob *Mob) {
	id := uuid.New()
	pkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(mob.EID),
		pk.UUID(id),
		pk.VarInt(mob.TypeID),
		pk.Double(mob.X),
		pk.Double(mob.Y),
		pk.Double(mob.Z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(degToAngle(mob.Pitch)),
		pk.Angle(degToAngle(mob.Yaw)),
		pk.Angle(degToAngle(mob.Yaw)), // head yaw
		pk.VarInt(0),                  // data
	)
	m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
		p.WritePacket(pkt)
	})

	// Send slime size metadata (index 16 = VarInt size)
	if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
		var w MetadataWriter
		w.writeIndex(16, metaSerializerInt)
		writeVarIntBuf(&w.buf, mob.SlimeSize)
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}

	// Send baby metadata (AgeableMob index 16, Boolean)
	if mob.Baby && !isSlimeType(mob.TypeID) {
		var w MetadataWriter
		w.WriteBoolean(16, true) // isBaby = true
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}

	// Send tameable metadata
	if mob.TameData != nil && mob.TameData.Tamed {
		m.broadcastTameableMetadata(mob)
		// Send horse armor equipment
		if mob.TypeID == MobTypeHorse && mob.TameData.ArmorItemID > 0 {
			m.broadcastHorseEquipment(mob)
		}
		// Send llama metadata (chest, carpet)
		if isLlamaType(mob.TypeID) {
			m.broadcastLlamaMetadata(mob)
		}
	}

	// Send custom name metadata
	if mob.CustomName != "" {
		m.broadcastCustomName(mob)
	}

	// Send villager data metadata (profession, type, level)
	if mob.TypeID == MobTypeVillager && mob.VillagerData != nil {
		var w MetadataWriter
		w.WriteVillagerData(18, 0, villagerProfessionID(mob.VillagerData.Profession), 1)
		data := w.Bytes()
		m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
			SendEntityMetadata(p, mob.EID, data)
		})
	}
}

// trySpawnSlime attempts to spawn slimes (1% chance per 100 ticks).
func (m *MobManager) trySpawnSlime() {
	if rand.Float64() > 0.01 {
		return
	}

	var players []*game.Player
	m.Manager.ForEach(func(p *game.Player) {
		if !p.Dead && p.GameMode == 0 {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	player := players[rand.Intn(len(players))]
	px, _, pz := player.Position()

	// Per-player cap check
	if m.countMobsNear(px, pz, 128*128, true) >= hostileCapPerPlayer {
		return
	}

	angle := rand.Float64() * 2 * math.Pi
	dist := 24 + rand.Float64()*24
	spawnX := px + math.Cos(angle)*dist
	spawnZ := pz + math.Sin(angle)*dist

	spawnY := m.findSurfaceY(int(spawnX), int(spawnZ))
	if spawnY < m.MinY {
		return
	}

	// Slimes spawn below Y=40 (simulating swamp/deep biome)
	if spawnY > 40 {
		return
	}

	// Pick random size
	var slimeSize int32
	var health, damage float32
	sizeRoll := rand.Float64()
	switch {
	case sizeRoll < 0.5:
		slimeSize, health, damage = 1, 1, 0
	case sizeRoll < 0.8:
		slimeSize, health, damage = 2, 4, 2
	default:
		slimeSize, health, damage = 4, 16, 4
	}

	eid := m.Manager.NextEntityID()
	mob := &Mob{
		EID:       eid,
		TypeID:    MobTypeSlime,
		X:         spawnX + 0.5,
		Y:         float64(spawnY),
		Z:         spawnZ + 0.5,
		PrevX:     spawnX + 0.5,
		PrevY:     float64(spawnY),
		PrevZ:     spawnZ + 0.5,
		Health:    health,
		MaxHealth: health,
		Damage:    damage,
		Speed:     0.1,
		WanderYaw: rand.Float32() * 360,
		Hostile:   true,
		SlimeSize: slimeSize,
	}
	m.Mobs[eid] = mob
	m.broadcastSpawn(mob)
}

// trySpawnPhantom spawns phantoms for players who haven't slept for 3+ in-game days.
func (m *MobManager) trySpawnPhantom(tick int64) {
	if !m.TimeMgr.IsNight() {
		return
	}

	m.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.GameMode != 0 {
			return
		}

		// Check if player hasn't slept for 3+ in-game days (72000 ticks)
		if tick-p.LastSleepTick < 72000 {
			return
		}
		// 5% chance per check
		if rand.Float64() > 0.05 {
			return
		}

		px, py, pz := p.Position()

		// Per-player cap check
		if m.countMobsNear(px, pz, 128*128, true) >= hostileCapPerPlayer {
			return
		}
		spawnY := py + 20 + rand.Float64()*20
		spawnAngle := rand.Float64() * 2 * math.Pi
		spawnDist := 5 + rand.Float64()*10
		spawnX := px + math.Cos(spawnAngle)*spawnDist
		spawnZ := pz + math.Sin(spawnAngle)*spawnDist

		eid := m.Manager.NextEntityID()
		mob := &Mob{
			EID:         eid,
			TypeID:      MobTypePhantom,
			X:           spawnX,
			Y:           spawnY,
			Z:           spawnZ,
			PrevX:       spawnX,
			PrevY:       spawnY,
			PrevZ:       spawnZ,
			Health:      20,
			MaxHealth:   20,
			Damage:      6,
			Speed:       0.3,
			WanderYaw:   rand.Float32() * 360,
			Hostile:     true,
			SwoopPhase:  0,
			SwoopTick:   tick,
			FlyTargetY:  rand.Float64()*5 + 5,
			CircleAngle: rand.Float64() * 2 * math.Pi,
		}
		m.Mobs[eid] = mob
		m.broadcastSpawn(mob)
		BroadcastSound(m.Manager, SoundPhantomAmbient, SoundCategoryHostile, spawnX, spawnY, spawnZ, 1.0, 1.0)
	})
}

// SendExistingMobs sends all current mobs to a newly joined player.
func (m *MobManager) SendExistingMobs(player *game.Player) {
	m.mu.Lock()
	defer m.mu.Unlock()

	px, _, pz := player.Position()
	r2 := PlayerTrackingRange * PlayerTrackingRange

	for _, mob := range m.Mobs {
		if mob.Health <= 0 || mob.InsideHive {
			continue
		}
		// Only send mobs within tracking range
		dx := mob.X - px
		dz := mob.Z - pz
		if dx*dx+dz*dz > r2 {
			continue
		}

		id := uuid.New()
		player.WritePacket(pk.Marshal(
			packetid.ClientboundAddEntity,
			pk.VarInt(mob.EID),
			pk.UUID(id),
			pk.VarInt(mob.TypeID),
			pk.Double(mob.X),
			pk.Double(mob.Y),
			pk.Double(mob.Z),
			pk.UnsignedByte(0),
			pk.Angle(degToAngle(mob.Pitch)),
			pk.Angle(degToAngle(mob.Yaw)),
			pk.Angle(degToAngle(mob.Yaw)),
			pk.VarInt(0),
		))

		// Send head rotation
		player.WritePacket(pk.Marshal(
			packetid.ClientboundRotateHead,
			pk.VarInt(mob.EID),
			pk.Angle(degToAngle(mob.Yaw)),
		))

		// Send slime size metadata
		if mob.TypeID == MobTypeSlime && mob.SlimeSize > 0 {
			var w MetadataWriter
			w.writeIndex(16, metaSerializerInt)
			writeVarIntBuf(&w.buf, mob.SlimeSize)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}

		// Send baby metadata
		if mob.Baby && !isSlimeType(mob.TypeID) {
			var w MetadataWriter
			w.WriteBoolean(16, true)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}

		// Send villager data metadata
		if mob.TypeID == MobTypeVillager && mob.VillagerData != nil {
			var w MetadataWriter
			w.WriteVillagerData(18, 0, villagerProfessionID(mob.VillagerData.Profession), 1)
			SendEntityMetadata(player, mob.EID, w.Bytes())
		}
	}
}
