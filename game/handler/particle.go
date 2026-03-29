package handler

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Particle type IDs (0-indexed from data/registryid/particletype.go).
const (
	ParticleCrit            int32 = 5
	ParticleDamageIndicator int32 = 6
	ParticleEnchantedHit    int32 = 17
	ParticleEffect          int32 = 15
	ParticleHeart           int32 = 48
	ParticlePoof            int32 = 59
	ParticleSmoke           int32 = 62
	ParticleSweepAttack     int32 = 67
	ParticleSplash          int32 = 70
	ParticleSonicBoom       int32 = 63
	ParticleSculkCharge     int32 = 61
	ParticleBlock           int32 = 2  // block crack/break particles
)

// BroadcastParticle sends a ClientboundLevelParticles packet to all players.
func BroadcastParticle(manager *game.PlayerManager, particleID int32,
	x, y, z float64, offsetX, offsetY, offsetZ, speed float32, count int32) {

	pkt := pk.Marshal(
		packetid.ClientboundLevelParticles,
		pk.Boolean(false),        // overrideLimiter
		pk.Boolean(false),        // alwaysShow
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.Float(offsetX),
		pk.Float(offsetY),
		pk.Float(offsetZ),
		pk.Float(speed),
		pk.Int(count),
		pk.VarInt(particleID),
	)
	manager.ForEach(func(p *game.Player) {
		p.WritePacket(pkt)
	})
}
