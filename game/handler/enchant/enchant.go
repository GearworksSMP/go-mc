// Package enchant provides a centralized enchantment registry, constants,
// and helpers for the Minecraft 26.1 server.
package enchant

// Enchantment name constants.
const (
	Sharpness            = "sharpness"
	Smite                = "smite"
	BaneOfArthropods     = "bane_of_arthropods"
	Knockback            = "knockback"
	FireAspect           = "fire_aspect"
	Looting              = "looting"
	SweepingEdge         = "sweeping_edge"
	Protection           = "protection"
	FireProtection       = "fire_protection"
	BlastProtection      = "blast_protection"
	ProjectileProtection = "projectile_protection"
	Thorns               = "thorns"
	FeatherFalling       = "feather_falling"
	Efficiency           = "efficiency"
	Unbreaking           = "unbreaking"
	Fortune              = "fortune"
	SilkTouch            = "silk_touch"
	Mending              = "mending"
	Power                = "power"
	Punch                = "punch"
	Flame                = "flame"
	Infinity             = "infinity"
	QuickCharge          = "quick_charge"
	Multishot            = "multishot"
	Riptide              = "riptide"
	Channeling           = "channeling"
	Loyalty              = "loyalty"
	Lure                 = "lure"
	LuckOfTheSea         = "luck_of_the_sea"
	Respiration          = "respiration"
	AquaAffinity         = "aqua_affinity"
	DepthStrider         = "depth_strider"
	FrostWalker          = "frost_walker"
	SoulSpeed            = "soul_speed"
	SwiftSneak           = "swift_sneak"
	Piercing             = "piercing"
	CurseOfBinding   = "binding_curse"
	CurseOfVanishing = "vanishing_curse"
	Sweeping         = "sweeping" // alias for sweeping_edge
	Density          = "density"
	Breach           = "breach"
)

// Enchantment holds metadata about an enchantment type.
type Enchantment struct {
	Name         string
	MaxLevel     int32
	Incompatible []string
}

// Registry maps enchantment names to their metadata.
var Registry map[string]*Enchantment

func init() {
	Registry = map[string]*Enchantment{
		Sharpness:            {Name: Sharpness, MaxLevel: 5, Incompatible: []string{Smite, BaneOfArthropods}},
		Smite:                {Name: Smite, MaxLevel: 5, Incompatible: []string{Sharpness, BaneOfArthropods}},
		BaneOfArthropods:     {Name: BaneOfArthropods, MaxLevel: 5, Incompatible: []string{Sharpness, Smite}},
		Knockback:            {Name: Knockback, MaxLevel: 2},
		FireAspect:           {Name: FireAspect, MaxLevel: 2},
		Looting:              {Name: Looting, MaxLevel: 3},
		SweepingEdge:         {Name: SweepingEdge, MaxLevel: 3},
		Protection:           {Name: Protection, MaxLevel: 4, Incompatible: []string{FireProtection, BlastProtection, ProjectileProtection}},
		FireProtection:       {Name: FireProtection, MaxLevel: 4, Incompatible: []string{Protection, BlastProtection, ProjectileProtection}},
		BlastProtection:      {Name: BlastProtection, MaxLevel: 4, Incompatible: []string{Protection, FireProtection, ProjectileProtection}},
		ProjectileProtection: {Name: ProjectileProtection, MaxLevel: 4, Incompatible: []string{Protection, FireProtection, BlastProtection}},
		Thorns:               {Name: Thorns, MaxLevel: 3},
		FeatherFalling:       {Name: FeatherFalling, MaxLevel: 4},
		Efficiency:           {Name: Efficiency, MaxLevel: 5},
		Unbreaking:           {Name: Unbreaking, MaxLevel: 3},
		Fortune:              {Name: Fortune, MaxLevel: 3, Incompatible: []string{SilkTouch}},
		SilkTouch:            {Name: SilkTouch, MaxLevel: 1, Incompatible: []string{Fortune}},
		Mending:              {Name: Mending, MaxLevel: 1, Incompatible: []string{Infinity}},
		Power:                {Name: Power, MaxLevel: 5},
		Punch:                {Name: Punch, MaxLevel: 2},
		Flame:                {Name: Flame, MaxLevel: 1},
		Infinity:             {Name: Infinity, MaxLevel: 1, Incompatible: []string{Mending}},
		QuickCharge:          {Name: QuickCharge, MaxLevel: 3},
		Multishot:            {Name: Multishot, MaxLevel: 1, Incompatible: []string{Piercing}},
		Piercing:             {Name: Piercing, MaxLevel: 4, Incompatible: []string{Multishot}},
		Riptide:              {Name: Riptide, MaxLevel: 3, Incompatible: []string{Loyalty, Channeling}},
		Channeling:           {Name: Channeling, MaxLevel: 1, Incompatible: []string{Riptide}},
		Loyalty:              {Name: Loyalty, MaxLevel: 3, Incompatible: []string{Riptide}},
		Lure:                 {Name: Lure, MaxLevel: 3},
		LuckOfTheSea:         {Name: LuckOfTheSea, MaxLevel: 3},
		Respiration:          {Name: Respiration, MaxLevel: 3},
		AquaAffinity:         {Name: AquaAffinity, MaxLevel: 1},
		DepthStrider:         {Name: DepthStrider, MaxLevel: 3, Incompatible: []string{FrostWalker}},
		FrostWalker:          {Name: FrostWalker, MaxLevel: 2, Incompatible: []string{DepthStrider}},
		SoulSpeed:            {Name: SoulSpeed, MaxLevel: 3},
		SwiftSneak:           {Name: SwiftSneak, MaxLevel: 3},
		CurseOfBinding:  {Name: CurseOfBinding, MaxLevel: 1},
		CurseOfVanishing: {Name: CurseOfVanishing, MaxLevel: 1},
	}
	// Register the sweeping alias pointing to the same entry.
	Registry[Sweeping] = Registry[SweepingEdge]

	// Mace-exclusive enchantments
	Registry[Density] = &Enchantment{Name: Density, MaxLevel: 5, Incompatible: []string{Breach, Smite, BaneOfArthropods, Sharpness}}
	Registry[Breach] = &Enchantment{Name: Breach, MaxLevel: 4, Incompatible: []string{Density, Smite, BaneOfArthropods, Sharpness}}
}
