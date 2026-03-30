package handler

import (
	"bytes"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Attribute modifier operations (vanilla protocol).
const (
	ModifierOpAdd          byte = 0 // add to base
	ModifierOpMultiplyBase byte = 1 // multiply base
	ModifierOpMultiplyTotal byte = 2 // multiply total
)

// attributeKey is a resource-location string for an attribute.
type attributeKey = string

// Base attribute keys (resource locations).
const (
	AttrMaxHealth          attributeKey = "minecraft:max_health"
	AttrMovementSpeed      attributeKey = "minecraft:movement_speed"
	AttrAttackDamage       attributeKey = "minecraft:attack_damage"
	AttrAttackSpeed        attributeKey = "minecraft:attack_speed"
	AttrArmor              attributeKey = "minecraft:armor"
	AttrArmorToughness     attributeKey = "minecraft:armor_toughness"
	AttrKnockbackResistance attributeKey = "minecraft:knockback_resistance"
)

// baseAttributes maps attribute keys to their default base values.
var baseAttributes = map[attributeKey]float64{
	AttrMaxHealth:           20.0,
	AttrMovementSpeed:       0.1,
	AttrAttackDamage:        1.0,
	AttrAttackSpeed:         4.0,
	AttrArmor:               0.0,
	AttrArmorToughness:      0.0,
	AttrKnockbackResistance: 0.0,
}

// armorSlots maps inventory slot indices to modifier ID suffixes.
var armorSlots = [...]struct {
	slot int
	name string
}{
	{5, "helmet"},
	{6, "chestplate"},
	{7, "leggings"},
	{8, "boots"},
}

// attributeModifier is a single modifier applied to an attribute.
type attributeModifier struct {
	ID        string  // resource location, e.g. "minecraft:effect.speed"
	Amount    float64
	Operation byte // 0=add, 1=multiply_base, 2=multiply_total
}

// attributeSnapshot holds the base value and modifiers for one attribute.
type attributeSnapshot struct {
	Key       attributeKey
	Base      float64
	Modifiers []attributeModifier
}

// computeAttributes gathers all attribute values and modifiers for a player
// based on their current equipment, effects, and state.
func computeAttributes(player *game.Player) []attributeSnapshot {
	attrs := make([]attributeSnapshot, 0, 7)

	// --- Max Health ---
	maxHP := attributeSnapshot{Key: AttrMaxHealth, Base: baseAttributes[AttrMaxHealth]}
	if player.Effects != nil {
		if eff, ok := player.Effects[EffectHealthBoost]; ok {
			maxHP.Modifiers = append(maxHP.Modifiers, attributeModifier{
				ID:        "minecraft:effect.health_boost",
				Amount:    4.0 * float64(eff.Level+1),
				Operation: ModifierOpAdd,
			})
		}
	}
	attrs = append(attrs, maxHP)

	// --- Movement Speed ---
	speed := attributeSnapshot{Key: AttrMovementSpeed, Base: baseAttributes[AttrMovementSpeed]}
	if player.Effects != nil {
		if eff, ok := player.Effects[EffectSpeed]; ok {
			speed.Modifiers = append(speed.Modifiers, attributeModifier{
				ID:        "minecraft:effect.speed",
				Amount:    0.2 * float64(eff.Level+1),
				Operation: ModifierOpMultiplyTotal,
			})
		}
		if eff, ok := player.Effects[EffectSlowness]; ok {
			speed.Modifiers = append(speed.Modifiers, attributeModifier{
				ID:        "minecraft:effect.slowness",
				Amount:    -0.15 * float64(eff.Level+1),
				Operation: ModifierOpMultiplyTotal,
			})
		}
	}
	attrs = append(attrs, speed)

	// --- Attack Damage ---
	atkDmg := attributeSnapshot{Key: AttrAttackDamage, Base: baseAttributes[AttrAttackDamage]}
	if player.Effects != nil {
		if eff, ok := player.Effects[EffectStrength]; ok {
			atkDmg.Modifiers = append(atkDmg.Modifiers, attributeModifier{
				ID:        "minecraft:effect.strength",
				Amount:    3.0 * float64(eff.Level+1),
				Operation: ModifierOpAdd,
			})
		}
		if eff, ok := player.Effects[EffectWeakness]; ok {
			atkDmg.Modifiers = append(atkDmg.Modifiers, attributeModifier{
				ID:        "minecraft:effect.weakness",
				Amount:    -4.0 * float64(eff.Level+1),
				Operation: ModifierOpAdd,
			})
		}
	}
	attrs = append(attrs, atkDmg)

	// --- Attack Speed ---
	atkSpd := attributeSnapshot{Key: AttrAttackSpeed, Base: baseAttributes[AttrAttackSpeed]}
	if player.Effects != nil {
		if eff, ok := player.Effects[EffectHaste]; ok {
			atkSpd.Modifiers = append(atkSpd.Modifiers, attributeModifier{
				ID:        "minecraft:effect.haste",
				Amount:    0.1 * float64(eff.Level+1),
				Operation: ModifierOpMultiplyTotal,
			})
		}
		if eff, ok := player.Effects[EffectMiningFatigue]; ok {
			atkSpd.Modifiers = append(atkSpd.Modifiers, attributeModifier{
				ID:        "minecraft:effect.mining_fatigue",
				Amount:    -0.1 * float64(eff.Level+1),
				Operation: ModifierOpMultiplyTotal,
			})
		}
	}
	attrs = append(attrs, atkSpd)

	// Armor, Armor Toughness, Knockback Resistance from equipment
	armorMods := make([]attributeModifier, 0, 4)
	toughnessMods := make([]attributeModifier, 0, 4)
	kbResistMods := make([]attributeModifier, 0, 4)

	for _, as := range armorSlots {
		itm := player.Inventory[as.slot]
		if itm.ID <= 0 || itm.Count <= 0 {
			continue
		}
		itemName := ItemNameByID(itm.ID)
		if itemName == "" {
			continue
		}

		if ap := float64(GetArmorProtection(itemName)); ap > 0 {
			armorMods = append(armorMods, attributeModifier{
				ID:        "minecraft:armor." + as.name,
				Amount:    ap,
				Operation: ModifierOpAdd,
			})
		}

		if at := float64(GetArmorToughness(itemName)); at > 0 {
			toughnessMods = append(toughnessMods, attributeModifier{
				ID:        "minecraft:armor_toughness." + as.name,
				Amount:    at,
				Operation: ModifierOpAdd,
			})
		}

		if kb := GetKnockbackResistance(itemName); kb > 0 {
			kbResistMods = append(kbResistMods, attributeModifier{
				ID:        "minecraft:knockback_resistance." + as.name,
				Amount:    kb,
				Operation: ModifierOpAdd,
			})
		}
	}

	attrs = append(attrs, attributeSnapshot{
		Key:       AttrArmor,
		Base:      baseAttributes[AttrArmor],
		Modifiers: armorMods,
	})
	attrs = append(attrs, attributeSnapshot{
		Key:       AttrArmorToughness,
		Base:      baseAttributes[AttrArmorToughness],
		Modifiers: toughnessMods,
	})
	attrs = append(attrs, attributeSnapshot{
		Key:       AttrKnockbackResistance,
		Base:      baseAttributes[AttrKnockbackResistance],
		Modifiers: kbResistMods,
	})

	return attrs
}

// SendAttributes sends a ClientboundUpdateAttributes packet to the player
// with their current computed attributes.
func SendAttributes(player *game.Player) {
	attrs := computeAttributes(player)
	writeAttributesPacket(player, player.EID, attrs)
}

// BroadcastAttributes sends the player's attributes to all nearby players
// and to the player themselves.
func BroadcastAttributes(manager *game.PlayerManager, player *game.Player) {
	attrs := computeAttributes(player)

	// Send to the player themselves
	writeAttributesPacket(player, player.EID, attrs)

	// Broadcast to nearby players
	px, _, pz := player.Position()
	manager.ForEachNearby(px, pz, PlayerTrackingRange, func(p *game.Player) {
		if p.UUID != player.UUID {
			writeAttributesPacket(p, player.EID, attrs)
		}
	})
}

// writeAttributesPacket encodes and sends the ClientboundUpdateAttributes packet.
func writeAttributesPacket(target *game.Player, entityID int32, attrs []attributeSnapshot) {
	var buf bytes.Buffer
	pk.VarInt(entityID).WriteTo(&buf)
	pk.VarInt(len(attrs)).WriteTo(&buf)

	for _, attr := range attrs {
		pk.Identifier(attr.Key).WriteTo(&buf)
		pk.Double(attr.Base).WriteTo(&buf)
		pk.VarInt(len(attr.Modifiers)).WriteTo(&buf)
		for _, mod := range attr.Modifiers {
			pk.Identifier(mod.ID).WriteTo(&buf)
			pk.Double(mod.Amount).WriteTo(&buf)
			pk.Byte(mod.Operation).WriteTo(&buf)
		}
	}

	target.WritePacket(pk.Packet{
		ID:   int32(packetid.ClientboundUpdateAttributes),
		Data: buf.Bytes(),
	})
}
