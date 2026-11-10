package entity

// jump.go — a player's jump (the user, 2026-11-09: "lets add jump to the
// game"). EQOA has none: CLIENT's walk (decomp viwalkcontrol.cpp) gives a body
// on land no way up, and its Move sticks a body to any floor within its
// step-down tolerance (0.6) under its feet. The jump is vanilla's (walker.rs,
// the client's own, as a GM's flight is): from a floor, a start speed upward,
// then CLIENT's own fall (Gravity -19.6, the air's damping 0.8 a second) and
// its own landing, the feet at most JumpTop over the floor it left, about 0.3 s
// up and 0.6 s in the air on flat ground. world.jump turns it on (SetJumps);
// off, the default, nothing is offered and nothing changes.
//
// The server offers it (protocol.OpcodeJump, handlers/jump.go) only to a client
// of its own, one that answered its bug report offer: a PS2 answers none, so it
// is never sent the jump's. A client that answers 1 jumps (Player.Jumps), and
// the movement check (movecheck.go) takes a jump's arc from that player:
//
//   - a jump begins at a report in the air, rising, after a report standing on
//     a floor (the last place taken held); vanilla jumps only once a report
//     standing has gone out since it last landed, so every jump of its starts
//     from one. Its floor is the higher of the floor under that report and the
//     floor under its first report in the air (a run up a slope leaves a floor
//     higher than the place it last reported);
//   - its reports rise as a jump rises (where the air takes no rise of more
//     than mcAirRise), to JumpTop + mcJumpTol over its floor and no further;
//   - once a report of it has come down from its highest, a rise is a second
//     jump in the air: "rose again in the air";
//   - it falls as any fall does, the fall rule asking it down from its floor
//     once its time is up (the jump's top is within the rule's own mcFallTol);
//   - it ends at a report standing or swimming.
//
// Everything else is judged as before: a rise with no floor under its start, a
// rise past the jump's top, hovering, flying, speed, walls. A player held in
// place (no speed: a GM's Freeze, a duel's hold, a root) does not jump; nor does
// its client.

import (
	"fmt"

	"github.com/eqoa/server/internal/session"
)

// JumpTop is the highest a jump takes a body's feet over the floor it left:
// vanilla's (walker.rs JUMP_TOP, half the walk's body of 1.8, m_parm30), at the
// start speed that reaches it with CLIENT's own integrator. The two are the same
// number; a client that jumps higher is judged as jumping too high.
const JumpTop float32 = 0.9

const (
	// mcJumpTol is how far over JumpTop a report of a jump may be: the 0x40's
	// rounding and a console frame's step. JumpTop plus it is the fall rule's
	// own tolerance (mcFallTol), so neither rule asks less than the other.
	mcJumpTol float32 = 0.1
	// mcJumpDip is how far under its highest a jump is come down: a rise of
	// more than this after it is a second jump in the air.
	mcJumpDip float32 = 0.1
)

// jumpState is a jump the judge follows (moveState.jump); its zero value is
// none.
type jumpState struct {
	on    bool
	floor float32 // the height of the floor it left
	peak  float32 // its highest report
	down  bool    // a report of it has come down from its highest by mcJumpDip
}

// SetJumps is world.jump: whether players may jump at all.
func (w *World) SetJumps(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.jumpsAllowed = on
}

// JumpsAllowed reports world.jump.
func (w *World) JumpsAllowed() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.jumpsAllowed
}

// SetJumpClient makes the client's answer to the jump offer its player's: on,
// the movement check takes a jump's arc from it; off, as from anyone. It
// reports false when the session has no player in the world, or when it says it
// jumps on a server without jumps (which never offered them: nothing is kept).
func (w *World) SetJumpClient(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || (on && !w.jumpsAllowed) {
		return false
	}
	p.Jumps = on
	if !on {
		p.mv.jump = jumpState{}
	}
	return true
}

// JumpClient is whether sess's character's client jumps; ok false with no
// character.
func (w *World) JumpClient(sess *session.Session) (on, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false, false
	}
	return p.Jumps, true
}

// judgeJumpLocked is a report of p's in the air (dry, not standing, on a ground
// the server knows) as a jump's: the jump after it, none when p's client does
// not jump or the report is no jump's, and what is wrong with it ("" nothing).
// dy is its rise since the last place taken. It changes nothing: the jump is
// the judgement's, kept by take. Must be called with the lock held.
func (w *World) judgeJumpLocked(p *Player, st *moveState, u *ClientUpdate, dy float32) (jumpState, string) {
	if !p.Jumps || !w.jumpsAllowed {
		return jumpState{}, ""
	}
	j := st.jump
	if !j.on {
		// a jump leaves a floor going up: the last place taken stood on one,
		// and the player may move (one held in place does not jump)
		if dy <= 0 || !st.judged || st.air || st.wet || p.SpeedNow() <= 0 {
			return jumpState{}, ""
		}
		j = jumpState{on: true, floor: st.y}
		if mg, ok := w.ground.(MoveGround); ok {
			if f, ok := mg.Support(p.World, st.x, st.y, st.z, mcFloorAbove, mcFloorDepth); ok {
				j.floor = f
			}
			// up a slope it left a floor higher than its last report's
			if f, ok := mg.Support(p.World, u.X, u.Y, u.Z, 0, mcFloorDepth); ok && f > j.floor {
				j.floor = f
			}
		}
		j.peak = j.floor
	}
	if over := u.Y - j.floor; over > JumpTop+mcJumpTol {
		return j, fmt.Sprintf("jumped %.2f over the floor it left, %.2f at most", over, JumpTop)
	}
	if j.down && dy > mcJumpDip {
		return j, fmt.Sprintf("rose %.2f again in the air, its jump come down", dy)
	}
	j.peak = max(j.peak, u.Y)
	if u.Y < j.peak-mcJumpDip {
		j.down = true
	}
	return j, ""
}
