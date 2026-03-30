package handler

import (
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Lightning bolt entity type ID (26.1 registry).
const lightningBoltEntityType int32 = 60

// Sound IDs for lightning.
const (
	SoundLightningBolt    int32 = 556 // entity.lightning_bolt.thunder
	SoundThunderAmbient   int32 = 557 // entity.lightning_bolt.impact
)

// LightningBolt represents an active lightning bolt entity.
type LightningBolt struct {
	EID      int32
	X, Y, Z  float64
	LifeTick int32
}

// LightningManager handles lightning bolt spawning during thunderstorms.
type LightningManager struct {
	Manager    *game.PlayerManager
	WeatherMgr *WeatherManager
	MobMgr     *MobManager
	Survival   *SurvivalHandler
	FireMgr    *FireManager
	Logger     *log.Logger
	mu         sync.Mutex
	bolts      map[int32]*LightningBolt
}

// NewLightningManager creates a new LightningManager.
func NewLightningManager(manager *game.PlayerManager, weatherMgr *WeatherManager, mobMgr *MobManager, survival *SurvivalHandler, logger *log.Logger) *LightningManager {
	return &LightningManager{
		Manager:    manager,
		WeatherMgr: weatherMgr,
		MobMgr:     mobMgr,
		Survival:   survival,
		Logger:     logger,
		bolts:      make(map[int32]*LightningBolt),
	}
}

// Tick processes lightning bolt lifecycle and random strikes during thunderstorms.
func (lm *LightningManager) Tick(tick int64) {
	// Random strikes during thunder
	if lm.WeatherMgr.State() == WeatherThunder && rand.Intn(200) == 0 {
		lm.randomStrike()
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	var toRemove []int32
	for eid, bolt := range lm.bolts {
		bolt.LifeTick++
		if bolt.LifeTick > 10 {
			toRemove = append(toRemove, eid)
		}
	}
	for _, eid := range toRemove {
		lm.removeBolt(eid)
	}
}

// randomStrike spawns a lightning bolt near a random online player.
func (lm *LightningManager) randomStrike() {
	var players []*game.Player
	lm.Manager.ForEach(func(p *game.Player) {
		if !p.Dead {
			players = append(players, p)
		}
	})
	if len(players) == 0 {
		return
	}

	target := players[rand.Intn(len(players))]
	px, _, pz := target.Position()

	// Random offset within 128 blocks
	x := px + float64(rand.Intn(256)-128)
	z := pz + float64(rand.Intn(256)-128)
	y := 80.0
	if lm.MobMgr != nil {
		surfaceY := lm.MobMgr.findSurfaceY(int(x), int(z))
		if surfaceY >= lm.MobMgr.MinY {
			y = float64(surfaceY)
		}
	}

	lm.SpawnLightningBolt(x, y, z)
}

// SpawnLightningBolt creates a lightning bolt entity at the given position.
// Deals 5 damage to players within 3 blocks and converts applicable mobs.
func (lm *LightningManager) SpawnLightningBolt(x, y, z float64) {
	eid := lm.Manager.NextEntityID()
	bolt := &LightningBolt{
		EID: eid,
		X:   x,
		Y:   y,
		Z:   z,
	}

	lm.mu.Lock()
	lm.bolts[eid] = bolt
	lm.mu.Unlock()

	// Broadcast spawn entity
	entityUUID := uuid.New()
	spawnPkt := pk.Marshal(
		packetid.ClientboundAddEntity,
		pk.VarInt(eid),
		pk.UUID(entityUUID),
		pk.VarInt(lightningBoltEntityType),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.UnsignedByte(0), // LpVec3 zero velocity
		pk.Angle(0),        // pitch
		pk.Angle(0),        // yaw
		pk.Angle(0),        // head yaw
		pk.VarInt(0),       // data
	)
	lm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(spawnPkt)
	})

	// Play thunder sounds
	BroadcastSound(lm.Manager, SoundLightningBolt, SoundCategoryWeather, x, y, z, 10.0, 1.0)
	BroadcastSound(lm.Manager, SoundThunderAmbient, SoundCategoryWeather, x, y, z, 10.0, 1.0)

	// Damage nearby players (within 3 blocks)
	lm.Manager.ForEach(func(p *game.Player) {
		if p.Dead || p.IsInvulnerable() {
			return
		}
		px, py, pz := p.Position()
		dist := math.Sqrt((px-x)*(px-x) + (py-y)*(py-y) + (pz-z)*(pz-z))
		if dist <= 3.0 {
			p.LastDamageMessage = p.Name + " was struck by lightning"
			p.FireTicks = 100 // set on fire for 5 seconds
			if lm.Survival != nil {
				lm.Survival.ApplyDamage(lm.Manager, p, 5.0, 0)
			}
		}
	})

	// Convert nearby mobs
	if lm.MobMgr != nil {
		lm.convertNearbyMobs(x, y, z)
	}

	// Set fire at impact point and 1-2 random adjacent blocks
	if lm.FireMgr != nil {
		ix, iy, iz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
		lm.FireMgr.PlaceFire(ix, iy, iz, 0)
		adjOffsets := [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}
		count := 1 + rand.Intn(2) // 1-2 adjacent fires
		perm := rand.Perm(len(adjOffsets))
		for i := 0; i < count && i < len(perm); i++ {
			off := adjOffsets[perm[i]]
			lm.FireMgr.PlaceFire(ix+off[0], iy+off[1], iz+off[2], 0)
		}
	}

	if lm.Logger != nil {
		lm.Logger.Printf("Lightning bolt struck at (%.0f, %.0f, %.0f)", x, y, z)
	}
}

// convertNearbyMobs converts pigs to zombified piglins and villagers to witches
// within 3 blocks of the lightning strike.
func (lm *LightningManager) convertNearbyMobs(x, y, z float64) {
	lm.MobMgr.mu.Lock()
	defer lm.MobMgr.mu.Unlock()

	var conversions []struct {
		oldEID    int32
		newTypeID int32
		x, y, z   float64
	}

	for _, mob := range lm.MobMgr.Mobs {
		if mob.Health <= 0 {
			continue
		}
		dx := mob.X - x
		dy := mob.Y - y
		dz := mob.Z - z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > 3.0 {
			continue
		}

		switch mob.TypeID {
		case MobTypePig:
			conversions = append(conversions, struct {
				oldEID    int32
				newTypeID int32
				x, y, z   float64
			}{mob.EID, MobTypeZombifiedPiglin, mob.X, mob.Y, mob.Z})
		case MobTypeVillager:
			conversions = append(conversions, struct {
				oldEID    int32
				newTypeID int32
				x, y, z   float64
			}{mob.EID, MobTypeWitch, mob.X, mob.Y, mob.Z})
		}
	}

	for _, conv := range conversions {
		// Remove old mob
		if old, ok := lm.MobMgr.Mobs[conv.oldEID]; ok {
			old.Health = 0
			removePkt := pk.Marshal(
				packetid.ClientboundRemoveEntities,
				pk.VarInt(1),
				pk.VarInt(conv.oldEID),
			)
			lm.MobMgr.Manager.ForEach(func(p *game.Player) {
				p.WritePacket(removePkt)
			})
			delete(lm.MobMgr.Mobs, conv.oldEID)
		}

		// Spawn new mob at the same position
		newMob := &Mob{
			EID:       lm.MobMgr.Manager.NextEntityID(),
			TypeID:    conv.newTypeID,
			X:         conv.x,
			Y:         conv.y,
			Z:         conv.z,
			PrevX:     conv.x,
			PrevY:     conv.y,
			PrevZ:     conv.z,
			Hostile:   conv.newTypeID != MobTypeVillager,
		}
		switch conv.newTypeID {
		case MobTypeZombifiedPiglin:
			newMob.Health = 20
			newMob.MaxHealth = 20
			newMob.Damage = 5
			newMob.Speed = 0.1
		case MobTypeWitch:
			newMob.Health = 26
			newMob.MaxHealth = 26
			newMob.Damage = 3
			newMob.Speed = 0.1
		}
		lm.MobMgr.Mobs[newMob.EID] = newMob
		lm.MobMgr.broadcastSpawn(newMob)

		if lm.Logger != nil {
			lm.Logger.Printf("Lightning converted mob %d to type %d", conv.oldEID, conv.newTypeID)
		}
	}
}

// removeBolt removes a lightning bolt entity and broadcasts its removal.
func (lm *LightningManager) removeBolt(eid int32) {
	removePkt := pk.Marshal(
		packetid.ClientboundRemoveEntities,
		pk.VarInt(1),
		pk.VarInt(eid),
	)
	lm.Manager.ForEach(func(p *game.Player) {
		p.WritePacket(removePkt)
	})
	delete(lm.bolts, eid)
}
