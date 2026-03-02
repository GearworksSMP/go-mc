package handler

import (
	"math/rand"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// CropManager handles crop growth simulation.
type CropManager struct {
	World   game.World
	Manager *game.PlayerManager

	mu          sync.Mutex
	crops       map[[3]int]struct{} // positions of registered crops (wheat, carrots, potatoes, beetroots)
	stems       map[[3]int]struct{} // pumpkin/melon stems
	sugarCanes  map[[3]int]struct{} // sugar cane positions
	bamboos     map[[3]int]struct{} // bamboo positions
	netherWarts map[[3]int]struct{} // nether wart positions
	cocoas      map[[3]int]struct{} // cocoa bean positions
	berries     map[[3]int]struct{} // sweet berry positions
}

// NewCropManager creates a new CropManager.
func NewCropManager(world game.World, manager *game.PlayerManager) *CropManager {
	return &CropManager{
		World:       world,
		Manager:     manager,
		crops:       make(map[[3]int]struct{}),
		stems:       make(map[[3]int]struct{}),
		sugarCanes:  make(map[[3]int]struct{}),
		bamboos:     make(map[[3]int]struct{}),
		netherWarts: make(map[[3]int]struct{}),
		cocoas:      make(map[[3]int]struct{}),
		berries:     make(map[[3]int]struct{}),
	}
}

// RegisterCrop registers a crop for growth ticking.
func (c *CropManager) RegisterCrop(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.crops[[3]int{x, y, z}] = struct{}{}
}

// UnregisterCrop removes a crop from growth ticking.
func (c *CropManager) UnregisterCrop(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.crops, [3]int{x, y, z})
}

// RegisterStem registers a pumpkin/melon stem for growth ticking.
func (c *CropManager) RegisterStem(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stems[[3]int{x, y, z}] = struct{}{}
}

// UnregisterStem removes a stem from growth ticking.
func (c *CropManager) UnregisterStem(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.stems, [3]int{x, y, z})
}

// RegisterSugarCane registers a sugar cane for growth ticking.
func (c *CropManager) RegisterSugarCane(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sugarCanes[[3]int{x, y, z}] = struct{}{}
}

// UnregisterSugarCane removes a sugar cane from growth ticking.
func (c *CropManager) UnregisterSugarCane(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sugarCanes, [3]int{x, y, z})
}

// RegisterBamboo registers a bamboo for growth ticking.
func (c *CropManager) RegisterBamboo(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bamboos[[3]int{x, y, z}] = struct{}{}
}

// UnregisterBamboo removes a bamboo from growth ticking.
func (c *CropManager) UnregisterBamboo(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bamboos, [3]int{x, y, z})
}

// RegisterNetherWart registers a nether wart for growth ticking.
func (c *CropManager) RegisterNetherWart(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.netherWarts[[3]int{x, y, z}] = struct{}{}
}

// UnregisterNetherWart removes a nether wart from growth ticking.
func (c *CropManager) UnregisterNetherWart(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.netherWarts, [3]int{x, y, z})
}

// RegisterCocoa registers a cocoa bean for growth ticking.
func (c *CropManager) RegisterCocoa(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cocoas[[3]int{x, y, z}] = struct{}{}
}

// UnregisterCocoa removes a cocoa bean from growth ticking.
func (c *CropManager) UnregisterCocoa(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cocoas, [3]int{x, y, z})
}

// RegisterBerry registers a sweet berry bush for growth ticking.
func (c *CropManager) RegisterBerry(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.berries[[3]int{x, y, z}] = struct{}{}
}

// UnregisterBerry removes a sweet berry bush from growth ticking.
func (c *CropManager) UnregisterBerry(x, y, z int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.berries, [3]int{x, y, z})
}

// Tick processes crop growth. Called every server tick.
// Growth checks happen every 100 ticks (~5 seconds).
func (c *CropManager) Tick(tick int64) {
	if tick%100 != 0 {
		return
	}

	c.mu.Lock()
	cropPositions := make([][3]int, 0, len(c.crops))
	for pos := range c.crops {
		cropPositions = append(cropPositions, pos)
	}
	stemPositions := make([][3]int, 0, len(c.stems))
	for pos := range c.stems {
		stemPositions = append(stemPositions, pos)
	}
	sugarCanePositions := make([][3]int, 0, len(c.sugarCanes))
	for pos := range c.sugarCanes {
		sugarCanePositions = append(sugarCanePositions, pos)
	}
	bambooPositions := make([][3]int, 0, len(c.bamboos))
	for pos := range c.bamboos {
		bambooPositions = append(bambooPositions, pos)
	}
	netherWartPositions := make([][3]int, 0, len(c.netherWarts))
	for pos := range c.netherWarts {
		netherWartPositions = append(netherWartPositions, pos)
	}
	cocoaPositions := make([][3]int, 0, len(c.cocoas))
	for pos := range c.cocoas {
		cocoaPositions = append(cocoaPositions, pos)
	}
	berryPositions := make([][3]int, 0, len(c.berries))
	for pos := range c.berries {
		berryPositions = append(berryPositions, pos)
	}
	c.mu.Unlock()

	for _, pos := range cropPositions {
		c.tickCrop(pos[0], pos[1], pos[2])
	}
	for _, pos := range stemPositions {
		c.tickStem(pos[0], pos[1], pos[2])
	}
	for _, pos := range sugarCanePositions {
		c.tickSugarCane(pos[0], pos[1], pos[2])
	}
	for _, pos := range bambooPositions {
		c.tickBamboo(pos[0], pos[1], pos[2])
	}
	for _, pos := range netherWartPositions {
		c.tickNetherWart(pos[0], pos[1], pos[2])
	}
	for _, pos := range cocoaPositions {
		c.tickCocoa(pos[0], pos[1], pos[2])
	}
	for _, pos := range berryPositions {
		c.tickSweetBerry(pos[0], pos[1], pos[2])
	}
}

func (c *CropManager) tickCrop(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterCrop(x, y, z)
		return
	}

	// Verify block below is farmland
	belowState, err := c.World.GetBlock(x, y-1, z)
	if err != nil {
		return
	}
	belowName := BlockNameFromState(int(belowState))
	if belowName != "farmland" {
		// Farmland destroyed — break crop, drop as item
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterCrop(x, y, z)
		return
	}

	// Growth chance: 1/25 base, doubled if hydrated farmland
	chance := 25
	if c.isHydrated(x, y-1, z) {
		chance = 12
	}
	if rand.Intn(chance) != 0 {
		return
	}

	c.growCropState(x, y, z)
}

// tickStem handles pumpkin/melon stem growth and fruit spawning.
func (c *CropManager) tickStem(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterStem(x, y, z)
		return
	}

	// Verify block below is farmland
	belowState, err := c.World.GetBlock(x, y-1, z)
	if err != nil {
		return
	}
	belowName := BlockNameFromState(int(belowState))
	if belowName != "farmland" {
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterStem(x, y, z)
		return
	}

	// Growth chance: 1/25 base, doubled if hydrated
	chance := 25
	if c.isHydrated(x, y-1, z) {
		chance = 12
	}
	if rand.Intn(chance) != 0 {
		return
	}

	switch b := block.StateList[stateID].(type) {
	case block.PumpkinStem:
		age := int(b.Age)
		if age < 7 {
			newBlock := block.PumpkinStem{Age: block.Integer(age + 1)}
			if newStateID, ok := block.ToStateID[newBlock]; ok {
				c.World.SetBlock(x, y, z, newStateID)
				broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
			}
		} else {
			// Try to spawn pumpkin on random adjacent block
			c.trySpawnFruit(x, y, z, block.Pumpkin{})
		}
	case block.MelonStem:
		age := int(b.Age)
		if age < 7 {
			newBlock := block.MelonStem{Age: block.Integer(age + 1)}
			if newStateID, ok := block.ToStateID[newBlock]; ok {
				c.World.SetBlock(x, y, z, newStateID)
				broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
			}
		} else {
			// Try to spawn melon on random adjacent block
			c.trySpawnFruit(x, y, z, block.Melon{})
		}
	default:
		c.UnregisterStem(x, y, z)
	}
}

// trySpawnFruit attempts to place a pumpkin or melon on a random adjacent block.
func (c *CropManager) trySpawnFruit(x, y, z int, fruit block.Block) {
	// Cardinal directions: N, S, E, W
	offsets := [4][2]int{{0, -1}, {0, 1}, {1, 0}, {-1, 0}}
	// Shuffle direction order
	order := rand.Perm(4)

	for _, i := range order {
		dx, dz := offsets[i][0], offsets[i][1]
		fx, fz := x+dx, z+dz

		// Check that the target block is air
		targetState, err := c.World.GetBlock(fx, y, fz)
		if err != nil || targetState != 0 {
			continue
		}

		// Check that the block below the target is dirt or grass_block
		belowState, err := c.World.GetBlock(fx, y-1, fz)
		if err != nil {
			continue
		}
		belowName := BlockNameFromState(int(belowState))
		if belowName != "dirt" && belowName != "grass_block" && belowName != "farmland" {
			continue
		}

		// Place the fruit
		fruitStateID, ok := block.ToStateID[fruit]
		if !ok {
			return
		}
		c.World.SetBlock(fx, y, fz, fruitStateID)
		broadcastBlockUpdateDirect(c.Manager, fx, y, fz, int32(fruitStateID))
		return
	}
}

// tickSugarCane handles sugar cane growth.
func (c *CropManager) tickSugarCane(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterSugarCane(x, y, z)
		return
	}

	// Verify current block is sugar cane
	if BlockNameFromState(int(stateID)) != "sugar_cane" {
		c.UnregisterSugarCane(x, y, z)
		return
	}

	// Growth chance: 1/15
	if rand.Intn(15) != 0 {
		return
	}

	// Find the base of the sugar cane column
	baseY := y
	for {
		belowState, err := c.World.GetBlock(x, baseY-1, z)
		if err != nil {
			break
		}
		if BlockNameFromState(int(belowState)) != "sugar_cane" {
			break
		}
		baseY--
	}

	// Check that base is on a valid block
	foundationState, err := c.World.GetBlock(x, baseY-1, z)
	if err != nil {
		return
	}
	foundationName := BlockNameFromState(int(foundationState))
	if foundationName != "dirt" && foundationName != "grass_block" && foundationName != "sand" {
		return
	}

	// Check for water within 1 block of the foundation (same Y level)
	hasWater := false
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			ws, err := c.World.GetBlock(x+dx, baseY-1, z+dz)
			if err != nil {
				continue
			}
			if int(ws) < len(block.StateList) && block.StateList[ws] != nil {
				if _, ok := block.StateList[ws].(block.Water); ok {
					hasWater = true
					break
				}
			}
		}
		if hasWater {
			break
		}
	}
	if !hasWater {
		return
	}

	// Count current height
	height := 0
	for cy := baseY; ; cy++ {
		s, err := c.World.GetBlock(x, cy, z)
		if err != nil {
			break
		}
		if BlockNameFromState(int(s)) != "sugar_cane" {
			break
		}
		height++
	}

	// Grow up to 3 blocks tall
	if height >= 3 {
		return
	}

	topY := baseY + height
	// Check air above
	aboveState, err := c.World.GetBlock(x, topY, z)
	if err != nil || aboveState != 0 {
		return
	}

	caneBlock := block.SugarCane{Age: 0}
	caneStateID, ok := block.ToStateID[caneBlock]
	if !ok {
		return
	}
	c.World.SetBlock(x, topY, z, caneStateID)
	broadcastBlockUpdateDirect(c.Manager, x, topY, z, int32(caneStateID))
	// Register the new block for ticking too
	c.RegisterSugarCane(x, topY, z)
}

// tickBamboo handles bamboo growth.
func (c *CropManager) tickBamboo(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterBamboo(x, y, z)
		return
	}

	blockName := BlockNameFromState(int(stateID))
	if blockName != "bamboo" && blockName != "bamboo_sapling" {
		c.UnregisterBamboo(x, y, z)
		return
	}

	// Growth chance: 1/10
	if rand.Intn(10) != 0 {
		return
	}

	// Find the base of the bamboo column
	baseY := y
	for {
		belowState, err := c.World.GetBlock(x, baseY-1, z)
		if err != nil {
			break
		}
		bn := BlockNameFromState(int(belowState))
		if bn != "bamboo" && bn != "bamboo_sapling" {
			break
		}
		baseY--
	}

	// Check foundation is valid
	foundationState, err := c.World.GetBlock(x, baseY-1, z)
	if err != nil {
		return
	}
	foundationName := BlockNameFromState(int(foundationState))
	if foundationName != "grass_block" && foundationName != "dirt" && foundationName != "sand" {
		return
	}

	// Count current height
	height := 0
	for cy := baseY; ; cy++ {
		s, err := c.World.GetBlock(x, cy, z)
		if err != nil {
			break
		}
		bn := BlockNameFromState(int(s))
		if bn != "bamboo" && bn != "bamboo_sapling" {
			break
		}
		height++
	}

	// Grow up to 16 blocks tall
	if height >= 16 {
		return
	}

	topY := baseY + height
	// Check air above
	aboveState, err := c.World.GetBlock(x, topY, z)
	if err != nil || aboveState != 0 {
		return
	}

	bambooBlock := block.Bamboo{Age: 0, Leaves: block.BambooLeavesNone, Stage: 0}
	if height > 3 {
		bambooBlock.Leaves = block.BambooLeavesLarge
	} else if height > 1 {
		bambooBlock.Leaves = block.BambooLeavesSmall
	}
	bambooStateID, ok := block.ToStateID[bambooBlock]
	if !ok {
		return
	}
	c.World.SetBlock(x, topY, z, bambooStateID)
	broadcastBlockUpdateDirect(c.Manager, x, topY, z, int32(bambooStateID))
	c.RegisterBamboo(x, topY, z)
}

// tickNetherWart handles nether wart growth.
func (c *CropManager) tickNetherWart(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterNetherWart(x, y, z)
		return
	}

	b, ok := block.StateList[stateID].(block.NetherWart)
	if !ok {
		c.UnregisterNetherWart(x, y, z)
		return
	}

	// Verify block below is soul_sand
	belowState, err := c.World.GetBlock(x, y-1, z)
	if err != nil {
		return
	}
	belowName := BlockNameFromState(int(belowState))
	if belowName != "soul_sand" {
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterNetherWart(x, y, z)
		return
	}

	// Growth chance: 1/15 (no hydration bonus)
	if rand.Intn(15) != 0 {
		return
	}

	age := int(b.Age)
	if age >= 3 {
		return // already mature
	}

	newBlock := block.NetherWart{Age: block.Integer(age + 1)}
	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}
	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
}

// tickCocoa handles cocoa bean growth.
func (c *CropManager) tickCocoa(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterCocoa(x, y, z)
		return
	}

	b, ok := block.StateList[stateID].(block.Cocoa)
	if !ok {
		c.UnregisterCocoa(x, y, z)
		return
	}

	// Verify block behind cocoa is jungle log (based on facing direction)
	dx, dz := facingToOffset(b.Facing)
	logState, err := c.World.GetBlock(x+dx, y, z+dz)
	if err != nil {
		return
	}
	logName := BlockNameFromState(int(logState))
	if logName != "jungle_log" {
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterCocoa(x, y, z)
		return
	}

	// Growth chance: 1/20
	if rand.Intn(20) != 0 {
		return
	}

	age := int(b.Age)
	if age >= 2 {
		return // already mature
	}

	newBlock := block.Cocoa{Age: block.Integer(age + 1), Facing: b.Facing}
	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}
	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
}

// tickSweetBerry handles sweet berry bush growth.
func (c *CropManager) tickSweetBerry(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		c.UnregisterBerry(x, y, z)
		return
	}

	b, ok := block.StateList[stateID].(block.SweetBerryBush)
	if !ok {
		c.UnregisterBerry(x, y, z)
		return
	}

	// Verify block below is dirt or grass_block
	belowState, err := c.World.GetBlock(x, y-1, z)
	if err != nil {
		return
	}
	belowName := BlockNameFromState(int(belowState))
	if belowName != "dirt" && belowName != "grass_block" {
		c.World.SetBlock(x, y, z, 0)
		broadcastBlockUpdateDirect(c.Manager, x, y, z, 0)
		c.UnregisterBerry(x, y, z)
		return
	}

	// Growth chance: 1/20
	if rand.Intn(20) != 0 {
		return
	}

	age := int(b.Age)
	if age >= 3 {
		return // already mature
	}

	newBlock := block.SweetBerryBush{Age: block.Integer(age + 1)}
	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}
	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
}

// facingToOffset returns the dx, dz offset for the block behind a cocoa bean.
// Cocoa faces away from the log, so the log is in the opposite direction.
func facingToOffset(facing block.Direction) (int, int) {
	switch facing {
	case block.North:
		return 0, 1 // log is south
	case block.South:
		return 0, -1 // log is north
	case block.East:
		return -1, 0 // log is west
	case block.West:
		return 1, 0 // log is east
	}
	return 0, 0
}

// growCropState attempts to advance a crop's age by 1.
func (c *CropManager) growCropState(x, y, z int) {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return
	}

	var newBlock block.Block
	atMax := false

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Wheat{Age: block.Integer(age + 1)}
		}
	case block.Carrots:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Carrots{Age: block.Integer(age + 1)}
		}
	case block.Potatoes:
		age := int(b.Age)
		if age >= 7 {
			atMax = true
		} else {
			newBlock = block.Potatoes{Age: block.Integer(age + 1)}
		}
	case block.Beetroots:
		age := int(b.Age)
		if age >= 3 {
			atMax = true
		} else {
			newBlock = block.Beetroots{Age: block.Integer(age + 1)}
		}
	default:
		c.UnregisterCrop(x, y, z)
		return
	}

	if atMax {
		return // already mature
	}

	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return
	}

	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
}

// BoneMealCrop advances a crop by 2-5 growth stages (random, capped at max).
// Returns true if the crop was advanced.
func (c *CropManager) BoneMealCrop(x, y, z int) bool {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return false
	}

	var newBlock block.Block
	var changed bool

	advance := 2 + rand.Intn(4) // 2-5 stages

	switch b := block.StateList[stateID].(type) {
	case block.Wheat:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Wheat{Age: block.Integer(newAge)}
		changed = true
	case block.Carrots:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Carrots{Age: block.Integer(newAge)}
		changed = true
	case block.Potatoes:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.Potatoes{Age: block.Integer(newAge)}
		changed = true
	case block.Beetroots:
		age := int(b.Age)
		if age >= 3 {
			return false
		}
		newAge := age + advance
		if newAge > 3 {
			newAge = 3
		}
		newBlock = block.Beetroots{Age: block.Integer(newAge)}
		changed = true
	case block.PumpkinStem:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.PumpkinStem{Age: block.Integer(newAge)}
		changed = true
	case block.MelonStem:
		age := int(b.Age)
		if age >= 7 {
			return false
		}
		newAge := age + advance
		if newAge > 7 {
			newAge = 7
		}
		newBlock = block.MelonStem{Age: block.Integer(newAge)}
		changed = true
	case block.NetherWart:
		age := int(b.Age)
		if age >= 3 {
			return false
		}
		newAge := age + advance
		if newAge > 3 {
			newAge = 3
		}
		newBlock = block.NetherWart{Age: block.Integer(newAge)}
		changed = true
	case block.Cocoa:
		age := int(b.Age)
		if age >= 2 {
			return false
		}
		newAge := age + advance
		if newAge > 2 {
			newAge = 2
		}
		newBlock = block.Cocoa{Age: block.Integer(newAge), Facing: b.Facing}
		changed = true
	case block.SweetBerryBush:
		age := int(b.Age)
		if age >= 3 {
			return false
		}
		newAge := age + advance
		if newAge > 3 {
			newAge = 3
		}
		newBlock = block.SweetBerryBush{Age: block.Integer(newAge)}
		changed = true
	}

	if !changed {
		return false
	}

	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return false
	}

	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))
	return true
}

// HarvestSweetBerries harvests berries from a mature bush, resetting it to age 1.
// Returns the number of berries dropped.
func (c *CropManager) HarvestSweetBerries(x, y, z int) int {
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return 0
	}
	if int(stateID) >= len(block.StateList) || block.StateList[stateID] == nil {
		return 0
	}

	b, ok := block.StateList[stateID].(block.SweetBerryBush)
	if !ok {
		return 0
	}

	age := int(b.Age)
	if age < 2 {
		return 0
	}

	// Calculate drop count
	var count int
	if age == 2 {
		count = 1 + rand.Intn(2) // 1-2
	} else {
		count = 2 + rand.Intn(2) // 2-3
	}

	// Reset bush to age 1
	newBlock := block.SweetBerryBush{Age: block.Integer(1)}
	newStateID, ok := block.ToStateID[newBlock]
	if !ok {
		return 0
	}
	c.World.SetBlock(x, y, z, newStateID)
	broadcastBlockUpdateDirect(c.Manager, x, y, z, int32(newStateID))

	return count
}

// isHydrated checks if farmland at (x, y, z) is hydrated (water within 4 blocks).
func (c *CropManager) isHydrated(x, y, z int) bool {
	// Check farmland moisture property
	stateID, err := c.World.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	if int(stateID) < len(block.StateList) && block.StateList[stateID] != nil {
		if fl, ok := block.StateList[stateID].(block.Farmland); ok {
			if int(fl.Moisture) > 0 {
				return true
			}
		}
	}

	// Check for nearby water (4 blocks horizontally, same Y or Y+1)
	for dx := -4; dx <= 4; dx++ {
		for dz := -4; dz <= 4; dz++ {
			for dy := 0; dy <= 1; dy++ {
				waterState, err := c.World.GetBlock(x+dx, y+dy, z+dz)
				if err != nil {
					continue
				}
				if int(waterState) < len(block.StateList) && block.StateList[waterState] != nil {
					if _, ok := block.StateList[waterState].(block.Water); ok {
						return true
					}
				}
			}
		}
	}
	return false
}
