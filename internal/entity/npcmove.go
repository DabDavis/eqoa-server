package entity

import "math"

// npcmove.go — an NPC moving on its own: a step toward a place each tick,
// with the velocity and the locomotion state its channel carries (bytes
// 40-42 and 58), which is what a client moves and animates it by between
// updates. VICreatureEntity::Sync hands both to the creature's walk control
// (SetUpdatePhysics), and ExtrapolateMove carries it along the velocity and
// drops it onto the ground it finds within 1.5 above or below
// (ExtrapolateGround), so the server's height need only be near the
// ground's: this server has no terrain.

// The locomotion states (VIWalkControl::UpdateLocomotion): standing, walking
// forward, running. Running is faster than a tenth of the way from the walk
// speed to the run speed (3 to 8.83: 3.58).
const (
	locoIdle     byte    = 0
	locoWalk     byte    = 1
	locoRun      byte    = 3
	runThreshold float32 = 3.58
)

// stepNPCToward moves the NPC toward (x, z) at speed units a second for dt
// seconds, facing the way it goes, and reports whether it got there; there,
// or with no speed, it stands. The spatial index follows it. Must be called
// with the lock held.
func (w *World) stepNPCToward(npc *NPC, x, z, speed, dt float32) (arrived bool) {
	dx, dz := x-npc.X, z-npc.Z
	dist := float32(math.Hypot(float64(dx), float64(dz)))
	oldX, oldZ := npc.X, npc.Z
	if dist > 0.05 {
		npc.Facing = headingTo(dx, dz)
	}
	if step := speed * dt; speed <= 0 || dist <= step || dist <= 0.05 {
		npc.X, npc.Z = x, z
		npc.stand()
		arrived = true
	} else {
		npc.X += dx / dist * step
		npc.Z += dz / dist * step
		npc.VelX, npc.VelY, npc.VelZ = dx/dist*speed, 0, dz/dist*speed
		npc.Loco = locoWalk
		if speed > runThreshold {
			npc.Loco = locoRun
		}
	}
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
	return arrived
}

// stand stops the NPC where it is.
func (n *NPC) stand() {
	n.VelX, n.VelY, n.VelZ = 0, 0, 0
	n.Loco = locoIdle
}

// headingTo is the heading of a direction on the ground, as VIVect3::HPR
// reads one (atan2(x, z): 0 along +z), in [0, 2pi) as the channel's facing
// byte takes it.
func headingTo(dx, dz float32) float32 {
	h := math.Atan2(float64(dx), float64(dz))
	if h < 0 {
		h += 2 * math.Pi
	}
	return float32(h)
}
