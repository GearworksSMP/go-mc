package handler

import (
	"log"
	"math"
	"strings"
	"sync"

	"github.com/Tnze/go-mc/game"
	"github.com/Tnze/go-mc/level/block"
)

// workstationProfession maps workstation block IDs to villager professions.
var workstationProfession = map[string]string{
	"minecraft:blast_furnace":     "armorer",
	"minecraft:smoker":            "butcher",
	"minecraft:brewing_stand":     "cleric",
	"minecraft:cartography_table": "cartographer",
	"minecraft:barrel":            "fisherman",
	"minecraft:fletching_table":   "fletcher",
	"minecraft:composter":         "farmer",
	"minecraft:lectern":           "librarian",
	"minecraft:cauldron":          "leatherworker",
	"minecraft:stonecutter":       "mason",
	"minecraft:loom":              "shepherd",
	"minecraft:smithing_table":    "toolsmith",
	"minecraft:grindstone":        "weaponsmith",
}

// Village represents a detected cluster of beds and workstations with associated villagers.
type Village struct {
	CenterX, CenterY, CenterZ float64
	Beds                       [][3]int
	Workstations               [][3]int
	Villagers                  []int32
	LastGolemSpawnTick         int64
}

// VillageManager detects villages and manages workstation claiming, bed claiming, and iron golem spawning.
type VillageManager struct {
	MobMgr  *MobManager
	Manager *game.PlayerManager
	World   game.World
	Logger  *log.Logger
	mu      sync.Mutex

	villages            []*Village
	claimedWorkstations map[[3]int]int32 // position -> villager EID
	claimedBeds         map[[3]int]int32 // position -> villager EID
}

// NewVillageManager creates a new VillageManager.
func NewVillageManager(mobMgr *MobManager, manager *game.PlayerManager, world game.World, logger *log.Logger) *VillageManager {
	return &VillageManager{
		MobMgr:              mobMgr,
		Manager:             manager,
		World:               world,
		Logger:              logger,
		claimedWorkstations: make(map[[3]int]int32),
		claimedBeds:         make(map[[3]int]int32),
	}
}

// Tick runs village detection and related systems every 200 ticks.
func (vm *VillageManager) Tick(tick int64) {
	if tick%200 != 0 {
		return
	}
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.detectVillages()
	vm.claimWorkstations()
	vm.claimBeds()
	vm.checkIronGolemSpawns(tick)
}

// blockNameAt returns the block ID string at the given position, or "" on error.
func blockNameAt(w game.World, x, y, z int) string {
	state, err := w.GetBlock(x, y, z)
	if err != nil {
		return ""
	}
	if int(state) >= len(block.StateList) || block.StateList[state] == nil {
		return ""
	}
	return block.StateList[state].ID()
}

// findBlocks scans the area for blocks matching the predicate. Vertical range is +/-8.
func (vm *VillageManager) findBlocks(cx, cy, cz, radius int, match func(string) bool) [][3]int {
	var result [][3]int
	const vertRange = 8
	for x := cx - radius; x <= cx+radius; x++ {
		for z := cz - radius; z <= cz+radius; z++ {
			for y := cy - vertRange; y <= cy+vertRange; y++ {
				if name := blockNameAt(vm.World,x, y, z); name != "" && match(name) {
					result = append(result, [3]int{x, y, z})
				}
			}
		}
	}
	return result
}

type villagerInfo struct {
	eid     int32
	x, y, z float64
}

// detectVillages scans for bed clusters near villagers and rebuilds the village list.
func (vm *VillageManager) detectVillages() {
	vm.MobMgr.mu.Lock()
	var villagers []villagerInfo
	for _, mob := range vm.MobMgr.Mobs {
		if mob.TypeID == MobTypeVillager && mob.Health > 0 && mob.VillagerData != nil {
			villagers = append(villagers, villagerInfo{mob.EID, mob.X, mob.Y, mob.Z})
		}
	}
	vm.MobMgr.mu.Unlock()

	if len(villagers) == 0 {
		vm.villages = nil
		return
	}

	// Scan for beds near each villager, dedup by position.
	allBeds := make(map[[3]int]bool)
	isBed := func(name string) bool { return strings.HasSuffix(name, "_bed") }
	for _, v := range villagers {
		cx, cy, cz := int(math.Floor(v.x)), int(math.Floor(v.y)), int(math.Floor(v.z))
		for _, b := range vm.findBlocks(cx, cy, cz, 48, isBed) {
			allBeds[b] = true
		}
	}

	if len(allBeds) == 0 {
		vm.villages = nil
		return
	}

	bedList := make([][3]int, 0, len(allBeds))
	for b := range allBeds {
		bedList = append(bedList, b)
	}

	clusters := clusterPositions(bedList, 48)
	isWorkstation := func(name string) bool { _, ok := workstationProfession[name]; return ok }

	vm.villages = make([]*Village, 0, len(clusters))
	for _, cluster := range clusters {
		village := &Village{Beds: cluster}

		var sx, sy, sz float64
		for _, b := range cluster {
			sx += float64(b[0]) + 0.5
			sy += float64(b[1]) + 0.5
			sz += float64(b[2]) + 0.5
		}
		n := float64(len(cluster))
		village.CenterX = sx / n
		village.CenterY = sy / n
		village.CenterZ = sz / n

		village.Workstations = vm.findBlocks(
			int(math.Floor(village.CenterX)),
			int(math.Floor(village.CenterY)),
			int(math.Floor(village.CenterZ)),
			48, isWorkstation,
		)

		for _, v := range villagers {
			dx := v.x - village.CenterX
			dy := v.y - village.CenterY
			dz := v.z - village.CenterZ
			if dx*dx+dy*dy+dz*dz <= 48*48 {
				village.Villagers = append(village.Villagers, v.eid)
			}
		}

		vm.villages = append(vm.villages, village)
	}
}

// clusterPositions groups positions within maxDist of each other using BFS.
func clusterPositions(positions [][3]int, maxDist int) [][][3]int {
	if len(positions) == 0 {
		return nil
	}
	maxDistSq := maxDist * maxDist
	assigned := make([]int, len(positions))
	nextCluster := 1

	for i := range positions {
		if assigned[i] != 0 {
			continue
		}
		assigned[i] = nextCluster
		queue := []int{i}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for j := range positions {
				if assigned[j] != 0 {
					continue
				}
				dx := positions[cur][0] - positions[j][0]
				dy := positions[cur][1] - positions[j][1]
				dz := positions[cur][2] - positions[j][2]
				if dx*dx+dy*dy+dz*dz <= maxDistSq {
					assigned[j] = nextCluster
					queue = append(queue, j)
				}
			}
		}
		nextCluster++
	}

	clusters := make(map[int][][3]int)
	for i, c := range assigned {
		clusters[c] = append(clusters[c], positions[i])
	}
	result := make([][][3]int, 0, len(clusters))
	for _, cl := range clusters {
		result = append(result, cl)
	}
	return result
}

// claimWorkstations assigns unemployed villagers to nearby unclaimed workstations.
func (vm *VillageManager) claimWorkstations() {
	// Remove stale claims for dead/removed villagers.
	vm.MobMgr.mu.Lock()
	for pos, eid := range vm.claimedWorkstations {
		mob, ok := vm.MobMgr.Mobs[eid]
		if !ok || mob.Health <= 0 {
			delete(vm.claimedWorkstations, pos)
		}
	}
	vm.MobMgr.mu.Unlock()

	for _, village := range vm.villages {
		for _, eid := range village.Villagers {
			vm.MobMgr.mu.Lock()
			mob, ok := vm.MobMgr.Mobs[eid]
			if !ok || mob.Health <= 0 || mob.VillagerData == nil {
				vm.MobMgr.mu.Unlock()
				continue
			}
			if mob.VillagerData.Profession != "" && mob.VillagerData.Profession != "none" {
				vm.MobMgr.mu.Unlock()
				continue
			}
			if mob.VillagerData.WorkstationPos != [3]int{} {
				vm.MobMgr.mu.Unlock()
				continue
			}
			mobX, mobY, mobZ := mob.X, mob.Y, mob.Z
			vm.MobMgr.mu.Unlock()

			bestDist := math.MaxFloat64
			bestPos := [3]int{}
			bestProf := ""
			for _, ws := range village.Workstations {
				if _, claimed := vm.claimedWorkstations[ws]; claimed {
					continue
				}
				dx := float64(ws[0]) + 0.5 - mobX
				dy := float64(ws[1]) + 0.5 - mobY
				dz := float64(ws[2]) + 0.5 - mobZ
				dist := dx*dx + dy*dy + dz*dz
				if dist > 48*48 || dist >= bestDist {
					continue
				}
				name := blockNameAt(vm.World,ws[0], ws[1], ws[2])
				if prof, ok := workstationProfession[name]; ok {
					bestDist = dist
					bestPos = ws
					bestProf = prof
				}
			}

			if bestProf == "" {
				continue
			}

			vm.MobMgr.mu.Lock()
			mob, ok = vm.MobMgr.Mobs[eid]
			if ok && mob.Health > 0 && mob.VillagerData != nil {
				mob.VillagerData.SetProfession(bestProf)
				mob.VillagerData.WorkstationPos = bestPos
				vm.claimedWorkstations[bestPos] = eid

				var w MetadataWriter
				w.WriteVillagerData(18, 0, villagerProfessionID(bestProf), 1)
				data := w.Bytes()
				vm.MobMgr.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
					SendEntityMetadata(p, mob.EID, data)
				})
			}
			vm.MobMgr.mu.Unlock()
		}
	}
}

// claimBeds assigns villagers without beds to nearby unclaimed beds.
func (vm *VillageManager) claimBeds() {
	// Remove stale bed claims for dead/removed villagers.
	vm.MobMgr.mu.Lock()
	for pos, eid := range vm.claimedBeds {
		mob, ok := vm.MobMgr.Mobs[eid]
		if !ok || mob.Health <= 0 {
			delete(vm.claimedBeds, pos)
		}
	}
	vm.MobMgr.mu.Unlock()

	for _, village := range vm.villages {
		for _, eid := range village.Villagers {
			vm.MobMgr.mu.Lock()
			mob, ok := vm.MobMgr.Mobs[eid]
			if !ok || mob.Health <= 0 || mob.VillagerData == nil {
				vm.MobMgr.mu.Unlock()
				continue
			}
			// Skip if already has a bed
			if mob.VillagerData.BedPos != ([3]int{}) {
				vm.MobMgr.mu.Unlock()
				continue
			}
			mobX, mobY, mobZ := mob.X, mob.Y, mob.Z
			vm.MobMgr.mu.Unlock()

			bestDist := math.MaxFloat64
			bestPos := [3]int{}
			for _, bed := range village.Beds {
				if _, claimed := vm.claimedBeds[bed]; claimed {
					continue
				}
				dx := float64(bed[0]) + 0.5 - mobX
				dy := float64(bed[1]) + 0.5 - mobY
				dz := float64(bed[2]) + 0.5 - mobZ
				dist := dx*dx + dy*dy + dz*dz
				if dist > 48*48 || dist >= bestDist {
					continue
				}
				bestDist = dist
				bestPos = bed
			}

			if bestDist == math.MaxFloat64 {
				continue
			}

			vm.MobMgr.mu.Lock()
			mob, ok = vm.MobMgr.Mobs[eid]
			if ok && mob.Health > 0 && mob.VillagerData != nil {
				mob.VillagerData.BedPos = bestPos
				vm.claimedBeds[bestPos] = eid
			}
			vm.MobMgr.mu.Unlock()
		}
	}
}

// checkIronGolemSpawns spawns iron golems in qualifying villages.
// Requires 3+ villagers who slept recently, max 1 golem per 10 villagers, 600-tick cooldown.
func (vm *VillageManager) checkIronGolemSpawns(tick int64) {
	for _, village := range vm.villages {
		if len(village.Villagers) < 3 {
			continue
		}
		if tick-village.LastGolemSpawnTick < 600 {
			continue
		}

		// Single lock pass: count golems and sleeping villagers.
		golemCount := 0
		sleepCount := 0
		vm.MobMgr.mu.Lock()
		for _, mob := range vm.MobMgr.Mobs {
			if mob.TypeID == MobTypeIronGolem && mob.Health > 0 {
				dx := mob.X - village.CenterX
				dz := mob.Z - village.CenterZ
				if dx*dx+dz*dz <= 48*48 {
					golemCount++
				}
			}
		}
		for _, eid := range village.Villagers {
			mob, ok := vm.MobMgr.Mobs[eid]
			if !ok || mob.Health <= 0 || mob.VillagerData == nil {
				continue
			}
			if mob.VillagerData.LastSleepTick > 0 && tick-mob.VillagerData.LastSleepTick < 24000 {
				sleepCount++
			}
		}
		vm.MobMgr.mu.Unlock()

		maxGolems := len(village.Villagers) / 10
		if maxGolems < 1 {
			maxGolems = 1
		}
		if golemCount >= maxGolems || sleepCount < 3 {
			continue
		}

		vm.MobMgr.SpawnMobAt(MobTypeIronGolem, village.CenterX, village.CenterY, village.CenterZ)
		village.LastGolemSpawnTick = tick

		if vm.Logger != nil {
			vm.Logger.Printf("Village spawned iron golem at (%.1f, %.1f, %.1f) with %d villagers",
				village.CenterX, village.CenterY, village.CenterZ, len(village.Villagers))
		}
	}
}
