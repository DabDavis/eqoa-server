package entity

import "math"

// An NPC moving, as CLIENT moves one (VICreatureEntity: decomp
// vicreatureentity.cpp, viaiextrapolate.cpp). A creature that does not run
// its own walk control (byte 141, m_p255, 0: every NPC) is carried along its
// velocity (bytes 40-42) between updates and dropped onto the ground within
// 1.5 above or below the height it was sent (ExtrapolateMove,
// ExtrapolateGround), and its sprite plays byte 58 as a VICSpriteAnimID
// (UpdateCreatures' SetLocomotion): 0 standing, 1 walking, 3 running, 14
// dead. Sandstorm's wandering creatures walk at 5.0 units a second with 1
// and stand with 0, the velocity zero
// (pcaps/ps2_sandstorm_1min10secs.pcap: bears, a lioness, a lavacrawler).

// The animations byte 58 plays (anim-slot-semantics.md, the CPlayList
// slots shared by the humanoid sets).
const (
	LocoStand byte = 0
	LocoWalk  byte = 1
	LocoRun   byte = 3
)

// NPCWalkSpeed is a creature's walking pace, Sandstorm's (5.0 to 5.2 units a
// second on every wandering creature recorded). NPCRunFrom is the speed from
// which a move plays the run instead.
const (
	NPCWalkSpeed float32 = 5.0
	NPCRunFrom   float32 = 7.0
)

// stepNPCToward moves npc dt seconds at speed (units a second) toward (x, z)
// on the ground plane, its height kept: its velocity, facing and animation
// those of the move (a walk, or the run from NPCRunFrom), the spatial index
// updated. Within this step's reach it stops there and reports true. Must
// be called with the lock held.
func (w *World) stepNPCToward(npc *NPC, x, z, speed, dt float32) (arrived bool) {
	dx, dz := x-npc.X, z-npc.Z
	dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	oldX, oldZ := npc.X, npc.Z
	if speed <= 0 || dt <= 0 {
		npc.stopMoving()
		return dist == 0
	}
	if step := speed * dt; dist <= step {
		npc.X, npc.Z = x, z
		if dist > 0 {
			npc.Facing = headingTo(dx, dz)
		}
		npc.stopMoving()
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		return true
	} else {
		ux, uz := dx/dist, dz/dist
		npc.X += ux * step
		npc.Z += uz * step
		npc.VelX, npc.VelY, npc.VelZ = ux*speed, 0, uz*speed
	}
	npc.Facing = headingTo(dx, dz)
	npc.Loco = LocoWalk
	if speed >= NPCRunFrom {
		npc.Loco = LocoRun
	}
	w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	return false
}

// stopMoving leaves it standing: no velocity, the standing animation.
func (n *NPC) stopMoving() {
	n.VelX, n.VelY, n.VelZ = 0, 0, 0
	n.Loco = LocoStand
}

// moving reports an NPC with a velocity.
func (n *NPC) moving() bool {
	return n.VelX != 0 || n.VelY != 0 || n.VelZ != 0
}

// headingTo is the heading of a move by (dx, dz) as CLIENT reads one
// (VIVect3::HPR: atan2(x, z), the direction (sin h, cos h)), within
// [0, 2pi): the state channel's facing byte is h × 128/pi as a signed char.
func headingTo(dx, dz float32) float32 {
	h := math.Atan2(float64(dx), float64(dz))
	if h < 0 {
		h += 2 * math.Pi
	}
	return float32(h)
}
