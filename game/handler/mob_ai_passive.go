package handler

import (
	"math"
	"math/rand"
	"strings"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/game"
	pk "github.com/Tnze/go-mc/net/packet"
)

// tickBee runs bee AI: pollination, honey production, and defensive stinging.
func (m *MobManager) tickBee(mob *Mob, tick int64) {
	// If the bee is inside a hive, check if it should exit
	if mob.InsideHive {
		if tick >= mob.HiveExitTick {
			mob.InsideHive = false
			if mob.BeeHivePos != nil && m.HiveMgr != nil {
				m.HiveMgr.ReleaseBee(mob.BeeHivePos[0], mob.BeeHivePos[1], mob.BeeHivePos[2])
			}
			// Teleport bee to just above the hive
			if mob.BeeHivePos != nil {
				mob.X = float64(mob.BeeHivePos[0]) + 0.5
				mob.Y = float64(mob.BeeHivePos[1]) + 1.0
				mob.Z = float64(mob.BeeHivePos[2]) + 0.5
			}
			m.broadcastMobMove(mob)
		}
		return // skip all AI while inside hive
	}

	// Bees fly — gentle vertical bobbing
	if mob.Y < mob.FlyTargetY {
		mob.Y += 0.05
	} else if mob.Y > mob.FlyTargetY+0.5 {
		mob.Y -= 0.03
	}
	if tick%80 == 0 {
		mob.FlyTargetY = mob.Y + (rand.Float64()*4 - 2)
	}

	// Decrement angry ticks
	if mob.BeeAngryTicks > 0 {
		mob.BeeAngryTicks--
		if mob.BeeAngryTicks <= 0 {
			mob.Hostile = false
		}
	}

	if mob.Hostile {
		// Angry bee — chase nearest player and sting
		var nearest *game.Player
		nearestDist := 16.0
		m.Manager.ForEach(func(p *game.Player) {
			if p.Dead || p.GameMode != 0 {
				return
			}
			ppx, ppy, ppz := p.Position()
			d := math.Sqrt(sqDist3(ppx-mob.X, ppy-mob.Y, ppz-mob.Z))
			if d < nearestDist {
				nearestDist = d
				nearest = p
			}
		})

		if nearest != nil {
			px, py, pz := nearest.Position()
			dx := px - mob.X
			dy := py + 1 - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist > 0.5 {
				speed := mob.Speed * 2
				mob.X += dx / dist * speed
				mob.Y += dy / dist * speed
				mob.Z += dz / dist * speed
				mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
			}

			if nearestDist <= 1.5 && mob.AttackCooldown <= 0 {
				mob.AttackCooldown = 20
				m.Survival.ApplyDamage(m.Manager, nearest, 1, m.Survival.MobDamageTypeID)
				// Apply poison effect (ID 19, level 0, 200 ticks = 10 seconds)
				if m.Survival.EffectMgr != nil {
					m.Survival.EffectMgr.ApplyEffect(nearest, 19, 0, 200, false)
				}
				// Bee dies after stinging
				mob.Health = 0
				m.killMob(mob, nil)
				return
			}
		}
	} else {
		// Peaceful bee behavior: find flowers, pollinate, return to hive
		m.tickBeePollination(mob, tick)
	}

	m.broadcastMobMove(mob)
}

// tickBeePollination handles the bee's flower-finding, pollination, and hive return cycle.
func (m *MobManager) tickBeePollination(mob *Mob, tick int64) {
	// If pollinated and has a hive, fly toward it
	if mob.BeePollinated && mob.BeeHivePos != nil {
		hx := float64(mob.BeeHivePos[0]) + 0.5
		hy := float64(mob.BeeHivePos[1]) + 0.5
		hz := float64(mob.BeeHivePos[2]) + 0.5
		dx := hx - mob.X
		dy := hy - mob.Y
		dz := hz - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist <= 2 {
			// Arrived at hive — deposit pollen and enter hive
			if m.HiveMgr != nil {
				m.HiveMgr.IncrementHoney(mob.BeeHivePos[0], mob.BeeHivePos[1], mob.BeeHivePos[2])
				if m.HiveMgr.AddBeeToHive(mob.BeeHivePos[0], mob.BeeHivePos[1], mob.BeeHivePos[2]) {
					mob.InsideHive = true
					mob.HiveExitTick = tick + 600 // stay inside ~30 seconds
				}
			}
			mob.BeePollinated = false
			mob.BeeFlowerPos = nil
			mob.BeePollTimer = 0
		} else if dist > 0 {
			speed := mob.Speed * 1.5
			mob.X += dx / dist * speed
			mob.Y += dy / dist * speed
			mob.Z += dz / dist * speed
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
		return
	}

	// If has a flower target and not yet pollinated, fly toward it
	if mob.BeeFlowerPos != nil && !mob.BeePollinated {
		fx := float64(mob.BeeFlowerPos[0]) + 0.5
		fy := float64(mob.BeeFlowerPos[1]) + 0.5
		fz := float64(mob.BeeFlowerPos[2]) + 0.5
		dx := fx - mob.X
		dy := fy - mob.Y
		dz := fz - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist <= 1 {
			// At flower — pollinate over time
			mob.BeePollTimer++
			if mob.BeePollTimer >= 30 {
				mob.BeePollinated = true
				mob.BeeFlowerPos = nil
				mob.BeePollTimer = 0
			}
		} else if dist > 0 {
			speed := mob.Speed * 1.5
			mob.X += dx / dist * speed
			mob.Y += dy / dist * speed
			mob.Z += dz / dist * speed
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		}
		return
	}

	// No flower target — search for one every 60 ticks
	if tick%60 == 0 && mob.BeeFlowerPos == nil {
		m.beeSearchFlower(mob)
	}

	// No hive — try to find one every 100 ticks
	if tick%100 == 0 && mob.BeeHivePos == nil && m.HiveMgr != nil {
		m.beeSearchHive(mob)
	}

	// Wander randomly while idle
	mob.X += (rand.Float64() - 0.5) * 0.15
	mob.Z += (rand.Float64() - 0.5) * 0.15
}

// beeSearchFlower samples random blocks within 16 blocks to find a flower.
func (m *MobManager) beeSearchFlower(mob *Mob) {
	const radius = 16
	bx := int(math.Floor(mob.X))
	by := int(math.Floor(mob.Y))
	bz := int(math.Floor(mob.Z))

	// Sample 40 random positions to avoid scanning the full 33x9x33 cube
	for i := 0; i < 40; i++ {
		wx := bx + rand.Intn(radius*2+1) - radius
		wy := by + rand.Intn(9) - 4
		wz := bz + rand.Intn(radius*2+1) - radius
		state, err := m.World.GetBlock(wx, wy, wz)
		if err != nil {
			continue
		}
		name := BlockNameFromState(int(state))
		if flowerBlocks[name] {
			pos := [3]int{wx, wy, wz}
			mob.BeeFlowerPos = &pos
			return
		}
	}
}

// beeSearchHive scans for nearby hives tracked by the HiveManager.
func (m *MobManager) beeSearchHive(mob *Mob) {
	if m.HiveMgr == nil {
		return
	}
	m.HiveMgr.mu.Lock()
	defer m.HiveMgr.mu.Unlock()

	bestDist := 32.0
	var bestPos *[3]int
	for pos, hd := range m.HiveMgr.hives {
		if hd.BeeCount >= 3 {
			continue // hive is full
		}
		dx := float64(pos[0]) + 0.5 - mob.X
		dy := float64(pos[1]) + 0.5 - mob.Y
		dz := float64(pos[2]) + 0.5 - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist < bestDist {
			bestDist = dist
			p := pos // copy loop variable
			bestPos = &p
		}
	}
	if bestPos != nil {
		mob.BeeHivePos = bestPos
	}
}

// tickFrog runs frog AI: jump-based movement, tongue attack on small slimes/magma cubes.
// Frogs have 3 variants: 0=temperate, 1=warm, 2=cold.
func (m *MobManager) tickFrog(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Initialize jump tick if not set
	if mob.FrogJumpTick == 0 {
		mob.FrogJumpTick = tick + int64(40+rand.Intn(41))
	}

	// Tongue attack cooldown
	if mob.FrogTongueTick > 0 && tick >= mob.FrogTongueTick {
		mob.FrogTongueTick = 0
	}

	// Look for small slimes or magma cubes to eat with tongue
	if mob.FrogTongueTick == 0 {
		m.mu.Lock()
		var target *Mob
		targetDist := 4.0 // tongue range
		for _, other := range m.Mobs {
			if other.EID == mob.EID || other.Health <= 0 {
				continue
			}
			// Only eat small slimes (size 1) and small magma cubes (size 1)
			if (other.TypeID == MobTypeSlime || other.TypeID == MobTypeMagmaCube) && other.SlimeSize <= 1 {
				dx := other.X - mob.X
				dy := other.Y - mob.Y
				dz := other.Z - mob.Z
				d := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if d < targetDist {
					targetDist = d
					target = other
				}
			}
		}
		m.mu.Unlock()

		if target != nil {
			// Face target
			dx := target.X - mob.X
			dz := target.Z - mob.Z
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

			// Instant kill with tongue
			target.Health = 0
			m.killMob(target, nil)

			// Drop froglight variant based on frog variant
			if m.ItemEntities != nil {
				var froglightName string
				switch mob.FrogVariant {
				case 0: // temperate
					froglightName = "ochre_froglight"
				case 1: // warm
					froglightName = "pearlescent_froglight"
				case 2: // cold
					froglightName = "verdant_froglight"
				default:
					froglightName = "ochre_froglight"
				}
				froglightID := itemIDByName(froglightName)
				if froglightID > 0 {
					m.ItemEntities.SpawnItem(m.Manager, target.X, target.Y+0.5, target.Z, froglightID, 1, 10)
				}
			}

			mob.FrogTongueTick = tick + 60 // 3 second cooldown
			m.broadcastMobMove(mob)
			return
		}
	}

	// Jump-based movement: jump every 40-80 ticks
	if tick >= mob.FrogJumpTick {
		mob.FrogJumpTick = tick + int64(40+rand.Intn(41))

		// Pick a random direction and jump
		mob.WanderYaw += (rand.Float32() - 0.5) * 90
		rad := float64(mob.WanderYaw) * math.Pi / 180
		jumpDist := 0.4 + rand.Float64()*0.3
		nx := math.Cos(rad) * jumpDist
		nz := math.Sin(rad) * jumpDist

		destX := mob.X + nx
		destZ := mob.Z + nz
		if !m.isCliff(destX, mob.Y, destZ) {
			m.tryMove(mob, nx, nz)
			mob.VelY = 0.3 // small upward jump
		} else {
			mob.WanderYaw += 180
		}
		mob.Yaw = mob.WanderYaw
	}

	m.broadcastMobMove(mob)
}

// tickAxolotl runs axolotl AI: water movement, attacks hostile aquatic mobs, plays dead when hurt.
func (m *MobManager) tickAxolotl(mob *Mob, tick int64) {
	// Axolotls are aquatic — don't apply normal gravity, swim instead
	// Simple float/swim behavior
	mob.X += (rand.Float64() - 0.5) * 0.1
	mob.Z += (rand.Float64() - 0.5) * 0.1
	mob.Y += (rand.Float64() - 0.5) * 0.05

	// Play dead mechanic: when health drops below 50%, play dead and regenerate
	if mob.AxolotlPlayingDead {
		if tick >= mob.AxolotlPlayDeadEnd {
			mob.AxolotlPlayingDead = false
			mob.Health = float32(math.Min(float64(mob.MaxHealth), float64(mob.Health)+4))
		} else {
			// Slowly regenerate while playing dead
			if tick%20 == 0 && mob.Health < mob.MaxHealth {
				mob.Health += 1
			}
			// Don't move while playing dead
			m.broadcastMobMove(mob)
			return
		}
	}

	if mob.AttackCooldown > 0 {
		mob.AttackCooldown--
	}

	// Look for hostile aquatic mobs to attack (guardians, drowned)
	var targetMob *Mob
	targetDist := 12.0
	m.mu.Lock()
	for _, other := range m.Mobs {
		if other.EID == mob.EID || other.Health <= 0 {
			continue
		}
		if other.TypeID == MobTypeGuardian || other.TypeID == MobTypeElderGuardian || other.TypeID == MobTypeDrowned {
			dx := other.X - mob.X
			dy := other.Y - mob.Y
			dz := other.Z - mob.Z
			d := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if d < targetDist {
				targetDist = d
				targetMob = other
			}
		}
	}
	m.mu.Unlock()

	if targetMob != nil {
		dx := targetMob.X - mob.X
		dy := targetMob.Y - mob.Y
		dz := targetMob.Z - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

		// Swim toward target
		if dist > 1.5 {
			mob.X += dx / dist * 0.15
			mob.Y += dy / dist * 0.1
			mob.Z += dz / dist * 0.15
		} else if mob.AttackCooldown <= 0 {
			// Attack
			targetMob.Health -= mob.Damage
			mob.AttackCooldown = 20
			if targetMob.Health <= 0 {
				targetMob.Health = 0
				m.killMob(targetMob, nil)
			} else {
				// Broadcast hurt animation
				hurtPkt := pk.Marshal(
					packetid.ClientboundHurtAnimation,
					pk.VarInt(targetMob.EID),
					pk.Float(0),
				)
				m.Manager.ForEach(func(p *game.Player) {
					p.WritePacket(hurtPkt)
				})
			}
		}
	} else {
		// Wander by swimming
		if tick%30 == 0 {
			mob.WanderYaw += (rand.Float32() - 0.5) * 60
		}
		rad := float64(mob.WanderYaw) * math.Pi / 180
		mob.X += math.Cos(rad) * mob.Speed * 0.3
		mob.Z += math.Sin(rad) * mob.Speed * 0.3
		mob.Yaw = mob.WanderYaw
	}

	m.broadcastMobMove(mob)
}

// TriggerAxolotlPlayDead should be called when an axolotl takes damage.
// If health drops below 50%, it plays dead for 200 ticks (10 seconds).
func (m *MobManager) TriggerAxolotlPlayDead(mob *Mob, tick int64) {
	if mob.TypeID != MobTypeAxolotl || mob.AxolotlPlayingDead {
		return
	}
	if mob.Health <= mob.MaxHealth/2 {
		mob.AxolotlPlayingDead = true
		mob.AxolotlPlayDeadEnd = tick + 200
	}
}

// tickAllay runs allay AI: collects dropped items matching held item, delivers to note block or player.
func (m *MobManager) tickAllay(mob *Mob, tick int64) {
	// Allays fly — no gravity, gentle floating
	mob.Y += (rand.Float64() - 0.5) * 0.05

	// If no held item, follow nearest player
	if mob.AllayHeldItem == "" {
		var nearest *game.Player
		nearestDist := 16.0
		m.Manager.ForEach(func(p *game.Player) {
			if p.Dead || p.GameMode == 3 { // skip spectators
				return
			}
			px, py, pz := p.Position()
			d := math.Sqrt(sqDist3(px-mob.X, py-mob.Y, pz-mob.Z))
			if d < nearestDist {
				nearestDist = d
				nearest = p
			}
		})

		if nearest != nil {
			px, py, pz := nearest.Position()
			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

			// Follow player at ~3 block distance
			if dist > 3 {
				mob.X += dx / dist * mob.Speed * 2
				mob.Y += dy / dist * mob.Speed * 2
				mob.Z += dz / dist * mob.Speed * 2
			}
		} else {
			// Idle float
			mob.X += (rand.Float64() - 0.5) * 0.08
			mob.Z += (rand.Float64() - 0.5) * 0.08
		}

		m.broadcastMobMove(mob)
		return
	}

	// Has a held item — search for matching dropped items within 32 blocks
	if m.ItemEntities != nil {
		heldID := itemIDByName(mob.AllayHeldItem)
		nearbyItems := m.ItemEntities.FindItemsNear(mob.X, mob.Y, mob.Z, 32.0)

		var targetItem *NearbyItem
		for i := range nearbyItems {
			if nearbyItems[i].ItemID == heldID {
				targetItem = &nearbyItems[i]
				break
			}
		}

		if targetItem != nil {
			// Fly toward the item (we don't have its position directly, so just collect it)
			m.ItemEntities.RemoveItem(targetItem.EID)
		}
	}

	// Deliver: fly toward note block or nearest player
	if mob.AllayDeliverTo != nil {
		// Fly toward note block position
		tx := float64(mob.AllayDeliverTo[0]) + 0.5
		ty := float64(mob.AllayDeliverTo[1]) + 1.0
		tz := float64(mob.AllayDeliverTo[2]) + 0.5
		dx := tx - mob.X
		dy := ty - mob.Y
		dz := tz - mob.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

		if dist > 1.5 {
			mob.X += dx / dist * mob.Speed * 3
			mob.Y += dy / dist * mob.Speed * 3
			mob.Z += dz / dist * mob.Speed * 3
		} else {
			// Drop the collected items here
			if m.ItemEntities != nil {
				heldID := itemIDByName(mob.AllayHeldItem)
				if heldID > 0 {
					m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y, mob.Z, heldID, 1, 10)
				}
			}
		}
	} else {
		// Deliver to nearest player
		var nearest *game.Player
		nearestDist := 32.0
		m.Manager.ForEach(func(p *game.Player) {
			if p.Dead {
				return
			}
			px, py, pz := p.Position()
			d := math.Sqrt(sqDist3(px-mob.X, py-mob.Y, pz-mob.Z))
			if d < nearestDist {
				nearestDist = d
				nearest = p
			}
		})

		if nearest != nil {
			px, py, pz := nearest.Position()
			dx := px - mob.X
			dy := py - mob.Y
			dz := pz - mob.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

			if dist > 2 {
				mob.X += dx / dist * mob.Speed * 3
				mob.Y += dy / dist * mob.Speed * 3
				mob.Z += dz / dist * mob.Speed * 3
			} else if m.ItemEntities != nil {
				// Drop items near player
				heldID := itemIDByName(mob.AllayHeldItem)
				if heldID > 0 {
					m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y, mob.Z, heldID, 1, 10)
				}
			}
		}
	}

	m.broadcastMobMove(mob)
}

// tickSniffer runs sniffer AI: periodically sniffs the ground, digs to produce seeds/pods.
func (m *MobManager) tickSniffer(mob *Mob, tick int64) {
	m.applyGravity(mob)

	// Initialize sniff tick if not set
	if mob.SnifferSniffTick == 0 {
		mob.SnifferSniffTick = tick + int64(200+rand.Intn(201))
	}

	// Handle digging state
	if mob.SnifferDigging {
		if tick >= mob.SnifferDigEnd {
			mob.SnifferDigging = false
			mob.SnifferSniffTick = tick + int64(200+rand.Intn(201))

			// Produce item: 50% torchflower seeds, 50% pitcher pod
			if m.ItemEntities != nil {
				var itemName string
				if rand.Float64() < 0.5 {
					itemName = "torchflower_seeds"
				} else {
					itemName = "pitcher_pod"
				}
				itemID := itemIDByName(itemName)
				if itemID > 0 {
					m.ItemEntities.SpawnItem(m.Manager, mob.X, mob.Y+0.5, mob.Z, itemID, 1, 10)
				}
			}

			m.broadcastMobMove(mob)
			return
		}
		// While digging, don't move — just broadcast occasional head movement
		if tick%10 == 0 {
			mob.Pitch = float32(30 + rand.Intn(20)) // look down while digging
		}
		m.broadcastMobMove(mob)
		return
	}

	// Sniff check
	if tick >= mob.SnifferSniffTick {
		// Start digging at current position
		mob.SnifferDigging = true
		mob.SnifferDigEnd = tick + 60 // dig for 3 seconds (60 ticks)

		// Play head-down animation
		mob.Pitch = 45

		m.broadcastMobMove(mob)
		return
	}

	// Normal passive wandering
	m.tickWander(mob, tick)
}

// tickVillager runs villager daily schedule AI based on world time.
func (m *MobManager) tickVillager(mob *Mob, tick int64) {
	m.applyGravity(mob)

	if mob.VillagerData == nil {
		m.tickWander(mob, tick)
		return
	}
	vd := mob.VillagerData

	// Flee behavior takes priority over schedule (reuse tickPassive flee logic)
	if m.tickFlee(mob) {
		return
	}

	dayTime := m.TimeMgr.GetDayTime()

	// Determine target state from time of day
	var targetState VillagerState
	switch {
	case dayTime < 2000:
		targetState = VillagerWandering
	case dayTime < 9000:
		targetState = VillagerWorking
	case dayTime < 11000:
		targetState = VillagerSocializing
	default:
		targetState = VillagerSleeping
	}

	// Handle state transitions
	if vd.State != targetState {
		oldState := vd.State
		vd.State = targetState

		if oldState == VillagerSleeping {
			m.broadcastMobPose(mob, 0) // standing
		}

		vd.PathAttemptTick = tick
	}

	switch vd.State {
	case VillagerWorking:
		m.villagerMoveToward(mob, vd, tick, vd.WorkstationPos)
	case VillagerSleeping:
		m.tickVillagerSleep(mob, vd, tick)
	default:
		m.tickWander(mob, tick)
	}
}

// tickFlee handles flee behavior for a mob. Returns true if the mob is fleeing.
func (m *MobManager) tickFlee(mob *Mob) bool {
	if mob.FleeTicks <= 0 {
		return false
	}
	mob.FleeTicks--
	dx := mob.FleeX - mob.X
	dz := mob.FleeZ - mob.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist > 1.0 {
		nx := dx / dist * mob.Speed * 1.5
		nz := dz / dist * mob.Speed * 1.5
		m.tryMove(mob, nx, nz)
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
		return true
	}
	mob.FleeTicks = 0
	return false
}

// broadcastMobPose sends a pose metadata update for a mob to nearby players.
func (m *MobManager) broadcastMobPose(mob *Mob, pose int32) {
	var w MetadataWriter
	w.WritePose(6, pose)
	data := w.Bytes()
	m.Manager.ForEachNearby(mob.X, mob.Z, PlayerTrackingRange, func(p *game.Player) {
		SendEntityMetadata(p, mob.EID, data)
	})
}

// villagerMoveToward moves a villager toward a target position, stopping within 2 blocks.
// Falls back to wandering if no target or pathfinding times out after 200 ticks.
func (m *MobManager) villagerMoveToward(mob *Mob, vd *VillagerData, tick int64, target [3]int) {
	if target == ([3]int{}) {
		m.tickWander(mob, tick)
		return
	}

	dx := float64(target[0]) + 0.5 - mob.X
	dz := float64(target[2]) + 0.5 - mob.Z
	distSq := dx*dx + dz*dz

	if distSq <= 4 {
		mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
		m.broadcastMobMove(mob)
		return
	}

	if tick-vd.PathAttemptTick > 200 {
		m.tickWander(mob, tick)
		return
	}

	dist := math.Sqrt(distSq)
	m.tryMove(mob, dx/dist*mob.Speed, dz/dist*mob.Speed)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
	m.broadcastMobMove(mob)
}

// tickVillagerSleep moves the villager to bed and sets sleeping pose.
func (m *MobManager) tickVillagerSleep(mob *Mob, vd *VillagerData, tick int64) {
	bed := vd.BedPos
	if bed == ([3]int{}) {
		m.tickWander(mob, tick)
		return
	}

	// Check bed still exists (only every 20 ticks to avoid per-tick block lookups)
	if tick%20 == 0 {
		name := blockNameAt(m.World, bed[0], bed[1], bed[2])
		if !strings.HasSuffix(name, "_bed") {
			vd.BedPos = [3]int{}
			m.tickWander(mob, tick)
			return
		}
	}

	dx := float64(bed[0]) + 0.5 - mob.X
	dz := float64(bed[2]) + 0.5 - mob.Z
	distSq := dx*dx + dz*dz

	if distSq <= 4 {
		vd.LastSleepTick = tick
		// Only send pose update once when first arriving (not every tick)
		if tick == vd.PathAttemptTick || (tick-vd.PathAttemptTick)%100 == 0 {
			m.broadcastMobPose(mob, 2) // sleeping
		}
		return
	}

	if tick-vd.PathAttemptTick > 200 {
		m.tickWander(mob, tick)
		return
	}

	dist := math.Sqrt(distSq)
	m.tryMove(mob, dx/dist*mob.Speed, dz/dist*mob.Speed)
	mob.Yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)
	m.broadcastMobMove(mob)
}
