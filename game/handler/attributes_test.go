package handler

import (
	"math"
	"testing"

	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/game"
)

func TestComputeAttributes_Defaults(t *testing.T) {
	p := &game.Player{}
	attrs := computeAttributes(p)

	expected := map[string]float64{
		AttrMaxHealth:           20.0,
		AttrMovementSpeed:       0.1,
		AttrAttackDamage:        1.0,
		AttrAttackSpeed:         4.0,
		AttrArmor:               0.0,
		AttrArmorToughness:      0.0,
		AttrKnockbackResistance: 0.0,
	}

	if len(attrs) != 7 {
		t.Fatalf("expected 7 attributes, got %d", len(attrs))
	}

	for _, a := range attrs {
		exp, ok := expected[a.Key]
		if !ok {
			t.Errorf("unexpected attribute key: %s", a.Key)
			continue
		}
		if a.Base != exp {
			t.Errorf("attribute %s: expected base %.1f, got %.1f", a.Key, exp, a.Base)
		}
		if len(a.Modifiers) != 0 {
			t.Errorf("attribute %s: expected 0 modifiers, got %d", a.Key, len(a.Modifiers))
		}
	}
}

func TestComputeAttributes_WithEffects(t *testing.T) {
	p := &game.Player{
		Effects: map[int32]*game.ActiveEffect{
			EffectSpeed:       {ID: EffectSpeed, Level: 0, Duration: 600},       // Speed I
			EffectStrength:    {ID: EffectStrength, Level: 1, Duration: 600},     // Strength II
			EffectHealthBoost: {ID: EffectHealthBoost, Level: 0, Duration: 600}, // Health Boost I
		},
	}

	attrs := computeAttributes(p)

	for _, a := range attrs {
		switch a.Key {
		case AttrMovementSpeed:
			// Speed I: +0.2 * (0+1) = +0.2 multiply_total
			if len(a.Modifiers) != 1 {
				t.Fatalf("movement_speed: expected 1 modifier, got %d", len(a.Modifiers))
			}
			if a.Modifiers[0].Amount != 0.2 {
				t.Errorf("speed modifier amount: expected 0.2, got %f", a.Modifiers[0].Amount)
			}
			if a.Modifiers[0].Operation != ModifierOpMultiplyTotal {
				t.Errorf("speed modifier operation: expected %d, got %d", ModifierOpMultiplyTotal, a.Modifiers[0].Operation)
			}

		case AttrAttackDamage:
			// Strength II: +3 * (1+1) = +6 add
			if len(a.Modifiers) != 1 {
				t.Fatalf("attack_damage: expected 1 modifier, got %d", len(a.Modifiers))
			}
			if a.Modifiers[0].Amount != 6.0 {
				t.Errorf("strength modifier amount: expected 6.0, got %f", a.Modifiers[0].Amount)
			}
			if a.Modifiers[0].Operation != ModifierOpAdd {
				t.Errorf("strength modifier operation: expected %d, got %d", ModifierOpAdd, a.Modifiers[0].Operation)
			}

		case AttrMaxHealth:
			// Health Boost I: +4 * (0+1) = +4 add
			if len(a.Modifiers) != 1 {
				t.Fatalf("max_health: expected 1 modifier, got %d", len(a.Modifiers))
			}
			if a.Modifiers[0].Amount != 4.0 {
				t.Errorf("health_boost modifier amount: expected 4.0, got %f", a.Modifiers[0].Amount)
			}
		}
	}
}

func TestComputeAttributes_WithArmor(t *testing.T) {
	// Find diamond chestplate and iron helmet IDs by scanning item data
	var diamondChestID, ironHelmetID int32
	for id, it := range item.ByID {
		switch it.Name {
		case "diamond_chestplate":
			diamondChestID = int32(id)
		case "iron_helmet":
			ironHelmetID = int32(id)
		}
	}
	if diamondChestID == 0 || ironHelmetID == 0 {
		t.Skip("item IDs not available in test environment")
	}

	p := &game.Player{}
	p.Inventory[5] = game.ItemStack{ID: ironHelmetID, Count: 1}   // helmet: iron = 2 armor
	p.Inventory[6] = game.ItemStack{ID: diamondChestID, Count: 1} // chestplate: diamond = 8 armor, 2 toughness

	attrs := computeAttributes(p)

	for _, a := range attrs {
		switch a.Key {
		case AttrArmor:
			total := 0.0
			for _, m := range a.Modifiers {
				total += m.Amount
			}
			if math.Abs(total-10.0) > 0.001 {
				t.Errorf("armor total: expected 10.0, got %f", total)
			}

		case AttrArmorToughness:
			total := 0.0
			for _, m := range a.Modifiers {
				total += m.Amount
			}
			if math.Abs(total-2.0) > 0.001 {
				t.Errorf("armor toughness total: expected 2.0, got %f", total)
			}
		}
	}
}

func TestEffectModifiesAttributes(t *testing.T) {
	modifying := []int32{EffectSpeed, EffectSlowness, EffectStrength, EffectWeakness, EffectHaste, EffectMiningFatigue, EffectHealthBoost}
	for _, id := range modifying {
		if !effectModifiesAttributes(id) {
			t.Errorf("effect %d should modify attributes", id)
		}
	}

	nonModifying := []int32{EffectRegeneration, EffectPoison, EffectInvisibility, EffectNightVision, EffectAbsorption}
	for _, id := range nonModifying {
		if effectModifiesAttributes(id) {
			t.Errorf("effect %d should not modify attributes", id)
		}
	}
}
