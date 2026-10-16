package entity

// clientupdate.go — what the server takes out of a client's own state
// channel, 0x40, beyond its position.
//
// The 41 bytes are CLIENT's func_00BC2F30 [A] (the Go client's
// internal/game/selfstate.go, byte for byte against 3,348 PS2 buffers): the
// walk camera's committed VIWalkPhysics, each vector quantised by
// WriteQuantized over a fixed box, then the flags, the locomotion state, the
// auto-attack flag and the target. Two things matter here:
//
//   - The velocity is the walker's own, in units a second. A client that
//     stands still sends NO 0x40 at all (connection_send_state never sends a
//     state equal to the last one), so a velocity worked out from the
//     difference between two updates never comes back to zero: the last one
//     before a stop would stay on the player for as long as it stood there,
//     and every other client moves an entity by its velocity each frame.
//   - Byte 36 is m_autoAttack (+0x71E4): the attack button, held or toggled.

// The velocity box, data_00CD7A70 as func_00CC6558 builds it: two bytes an
// axis over (-15.3, -84.5, -15.3)..(15.3, 3, 15.3).
var clientVelLo, clientVelHi = [3]float32{-15.3, -84.5, -15.3}, [3]float32{15.3, 3, 15.3}

// Byte offsets in the 0x40.
const (
	clientUpdateVelocity   = 10 // 3 × u16 big-endian
	clientUpdateAutoAttack = 36
)

// ClientUpdateVelocity is the walker's velocity out of a reconstructed 0x40,
// in units a second: each axis the centre of its WriteQuantized bucket.
func ClientUpdateVelocity(b []byte) (v [3]float32, ok bool) {
	if len(b) < clientUpdateVelocity+6 {
		return v, false
	}
	for i := 0; i < 3; i++ {
		q := uint32(b[clientUpdateVelocity+2*i])<<8 | uint32(b[clientUpdateVelocity+2*i+1])
		rng := clientVelHi[i] - clientVelLo[i]
		v[i] = clientVelLo[i] + (float32(q)+0.5)*rng/65536
	}
	return v, true
}

// ClientUpdateAutoAttack is byte 36 of a 0x40: whether the attack button is
// down (CLIENT's ToggleAutoAttack sets it on the press; MaybeStopAutoAttack
// clears it on the release unless the auto-attack option makes it a toggle).
func ClientUpdateAutoAttack(b []byte) (on, ok bool) {
	if len(b) <= clientUpdateAutoAttack {
		return false, false
	}
	return b[clientUpdateAutoAttack] != 0, true
}

// ClientUpdate is what ApplyClientUpdate takes out of one 0x40.
type ClientUpdate struct {
	X, Y, Z, Facing float32
	Velocity        [3]float32 // units a second
	HasVelocity     bool
	Animation       byte // byte 35, the locomotion state
	AutoAttack      bool // byte 36
	HasAutoAttack   bool
}

// ApplyClientUpdate puts one 0x40 on the player, under one lock so the state
// sync never sees the position without its velocity.
func (w *World) ApplyClientUpdate(id EntityID, u ClientUpdate) {
	w.mu.Lock()
	defer w.mu.Unlock()

	p, ok := w.Players[id]
	if !ok {
		return
	}
	oldX, oldZ := p.X, p.Z
	p.X, p.Y, p.Z = u.X, u.Y, u.Z
	if u.Facing != 0 {
		p.Facing = u.Facing
	}
	w.SpatialIndex.Update(id, oldX, oldZ, u.X, u.Z)
	if u.HasVelocity {
		p.VelX, p.VelY, p.VelZ = u.Velocity[0], u.Velocity[1], u.Velocity[2]
	}
	p.Animation = u.Animation
	if u.HasAutoAttack {
		p.AutoAttack = u.AutoAttack
	}
}
