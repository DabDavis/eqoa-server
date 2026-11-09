package entity

// movecheck.go — the server's own view of a player's moves (2026-11-01, the
// user: "we are hardening the eqoaclassic server. the client is authoritative
// on several things like Y axis, gravity, etc.", "its really in the state
// channel manipulation").
//
// A client says where it is in its own state channel, 0x40, and until now
// the server took every word: a client that writes its own channel (a
// modified client, a PS2's memory written by a cheat, a script speaking the
// protocol) could stand in the air, walk under the ground and through walls,
// run at any speed and never drown, since the 0x40's walker flags are the
// client's own claim of standing and of being under water. The place itself
// is the one thing a client must send; everything else the server can work
// out from it, as CLIENT's own walk does (decomp viwalkcontrol.cpp), against
// the same collision baked off the disc (internal/ground: the terrain,
// everything placed on it, and the water's BSP):
//
//   - a body stands on a floor: a surface whose front looks up within half a
//     unit under its feet (CalcFit's sweep takes one from 1.2 over the feet
//     to 0.6 under them, and Move sets the feet on it), or swims in a liquid
//     (the env leaf at its middle, 1.2 over the feet, as Move asks);
//   - anything else is in the air, and falls: CLIENT adds Gravity (-19.6)
//     to a body with no floor and no liquid, damped by m_parm44 (0.8) a
//     second, and nothing pushes it up (Walk and Run need a floor or a
//     liquid; VIWalkControl has no jump). A body in the air that does not
//     come down is held up by nothing. The one rise in the air taken is a
//     jump's, from a player whose client jumps on a server with world.jump
//     (jump.go): from a floor, to JumpTop over it, once;
//   - on a floor it climbs no steeper than the walk lets it: the sweep's
//     ball (0.6 round at 1.2 over the feet) stops at anything steeper, and
//     a step up is at most what the ball clears;
//   - it moves no faster than the speed the server gave it (0x00F8: the run
//     speed, its buffs and masteries), and in a liquid no faster than that
//     along its pitch plus SwimUp's 3;
//   - its middle never passes through the front of a face of the collision
//     (VIScene::Collide stops the ball at one): a wall, a house, the
//     terrain's own floor (down through the ground), a cave's roof.
//
// Measured on eqoaclassic's own players before any of it judged anyone
// (cmd/movereplay over the live log of 2026-10-31, 58,422 reports of 16
// sessions, PS2s, native and browser clients): every report its walker said
// stood on a floor was within 0.1 of a floor of the ground but six, all
// single reports on a step down (1.6 at most), which the fall rule takes;
// no climb between two reports was steeper than 1:1 plus 0.13; no way
// between two reports crossed a face; the fastest was the Spirit of the
// Wolf's 11.48 (the reports' own cadence, 101.7 ms, puts a few a step at
// 12-13); every fall went down at the game's gravity; and the walker's
// water flags agreed with the server's water at all but one report, at a
// surface's edge.
//
// What it does with a report it does not believe is the mode's
// (world.movecheck, #movecheck): "off" takes every report, as before;
// "log" (the default) takes every report and logs what it would not have
// taken ("MOVECHECK" lines); "enforce" leaves the player where it last was
// right (the report's place is not taken, so nothing the server decides by
// a player's place, its fights, its reach, who sees it, uses it), and when
// the client keeps it up, or crosses a wall at all, moves the client back
// there with a teleport (0x00B2, which CLIENT's Respawn answers with 0x0049).
// A server move (a login, a teleport, a coach, a gate, a respawn, a GM's
// goto) is the server's own: the reports from before the client heard of it
// are dropped until one comes from where the server put it.

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

// MoveCheckMode is what the server does with a report it does not believe.
type MoveCheckMode int32

const (
	MoveCheckOff     MoveCheckMode = iota // every report taken, unjudged (as before)
	MoveCheckLog                          // every report taken; what would not be is logged
	MoveCheckEnforce                      // a report not believed is not taken; the client is moved back
)

func (m MoveCheckMode) String() string {
	switch m {
	case MoveCheckOff:
		return "off"
	case MoveCheckEnforce:
		return "enforce"
	}
	return "log"
}

// ParseMoveCheckMode reads world.movecheck's word ("" is log).
func ParseMoveCheckMode(s string) (MoveCheckMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off":
		return MoveCheckOff, nil
	case "", "log":
		return MoveCheckLog, nil
	case "enforce", "on":
		return MoveCheckEnforce, nil
	}
	return MoveCheckLog, fmt.Errorf("movecheck %q: off, log or enforce", s)
}

var moveCheckMode atomic.Int32

func init() { moveCheckMode.Store(int32(MoveCheckLog)) }

// SetMoveCheck sets the mode, at once for every player.
func SetMoveCheck(m MoveCheckMode) { moveCheckMode.Store(int32(m)) }

// MoveCheck is the mode now.
func MoveCheck() MoveCheckMode { return MoveCheckMode(moveCheckMode.Load()) }

// MoveGround is what the judge asks of the ground (internal/ground's
// Support, Blocked and Knows). A Ground that is not one (or no ground) leaves
// only the speed rule.
type MoveGround interface {
	Support(world uint8, x, y, z, up, down float32) (float32, bool)
	Blocked(world uint8, x0, y0, z0, x1, y1, z1 float32) (string, bool)
	Knows(world uint8) bool
}

// envKnows is an Env that says which worlds it has (ground.Env's Knows):
// where the water is unknown, a body off every floor may be swimming, and
// the rules of the air are not applied.
type envKnows interface {
	Knows(world uint8) bool
}

// The rules' numbers. The game's are CLIENT's (viwalkcontrol.cpp); the
// margins are over the measured players' (the file's comment).
const (
	mcStandTol    float32 = 0.5  // a floor this far under the feet holds them
	mcFloorAbove  float32 = 0.25 // ...or this far over them (the report's rounding)
	mcFloorDepth  float32 = 2    // how far down a floor is looked for (it stands within mcStandTol)
	mcMiddle      float32 = 1.2  // the body's middle over its feet (m_parm38)
	mcRiseSlope   float32 = 1.0  // a climb on a floor: up at most this much a unit across...
	mcRiseStep    float32 = 1.3  // ...and a step more (the walk lifts a body 1.2; measured: 0.13)
	mcAirRise     float32 = 0.3  // up in the air, at most (nothing pushes a body up)
	mcSwimUp      float32 = 3.0  // SwimUp's speed (m_swimSpeed), over the speed's own
	mcSlack       float32 = 1.15 // the speed allowed over the speed given
	mcBurst       float32 = 3    // units in hand over the speed's seconds
	mcSpeedHeld           = 3 * time.Second
	mcFallGrace           = 0.75   // seconds in the air before the fall is asked for
	mcCadence             = 0.1017 // a falling client reports this far apart at least (m_p07568: 100 VITimer ticks)
	mcLag                 = 2.0    // seconds of reports a stalled link may bring in a bunch
	mcFallTol     float32 = 1.0    // and this much short of it allowed
	mcFallShare           = 0.7    // the share of the game's own fall asked for
	mcArriveAcr   float32 = 4      // a report this near a server move's place is the client there
	mcArriveUp    float32 = 5      // (CLIENT's Respawn puts it there, on a floor 3 up or down,
	mcArriveDown  float32 = 30     // and it may have stepped off since, or begun to fall)
	mcDiverge     float32 = 20     // the client this far ahead of a speed it may not have: moved back
	mcResend              = 10 * time.Second
	mcBadReports          = 3               // reports not believed in a row before the client is moved back
	mcBadFor              = time.Second     // or for this long
	mcSilentAir           = 3 * time.Second // in the air with no report for this long: put down
	mcFixesWindow         = 5 * time.Minute
	mcFixesNoted          = 5 // moves back in the window that are logged as a pattern
	mcLogEvery            = 2 * time.Second
)

// gravity and the air's damping (VIWalkControl::Gravity, m_parm44)
const (
	mcGravity = 19.6
	mcAirK    = 0.2231435513142097 // -ln(0.8)
)

// fallMin is how far a body falls in t seconds from rest, at mcFallShare of
// the game's: the damped fall, -g/k (t - (1 - e^-kt)/k).
func fallMin(t float64) float32 {
	if t <= 0 {
		return 0
	}
	return float32(mcFallShare * mcGravity / mcAirK * (t - (1-math.Exp(-mcAirK*t))/mcAirK))
}

// moveState is a player's moves as the server judged them.
type moveState struct {
	init    bool
	world   uint8
	x, y, z float32   // the place last taken: the player's place after the last report
	at      time.Time // when it was taken
	judged  bool      // the last place was judged with the ground and the water known
	air     bool      // and was in the air
	wet     bool      // and in a liquid
	airAt   time.Time // when it last stood or swam: the fall's start
	airY    float32   // and at what height
	airN    int       // reports taken in the air since
	jump    jumpState // the jump it is in (jump.go), none

	credit  float32 // the speed rule's distance in hand
	fastest float32 // the fastest speed given lately
	fastAt  time.Time

	pending           bool // a server move the client has not reported from yet
	pendWorld         uint8
	pendX, pendY      float32
	pendZ             float32
	pendAt, pendFixAt time.Time
	pendFix           bool // the move is ours: a move back

	bad   int // reports not believed in a row
	badAt time.Time
	fixes []time.Time

	loggedAt time.Time
	held     int // log lines held back since
	why      string

	breathLogged uint64 // the tick the breath's disagreement was last logged
}

// serverMovedLocked: the player stands where the server put it, and the
// client is to report from there.
func (st *moveState) serverMoved(p *Player, now time.Time, ours bool) {
	st.init = true
	st.world, st.x, st.y, st.z, st.at = p.World, p.X, p.Y, p.Z, now
	st.judged, st.air, st.wet = false, false, false
	st.airAt, st.airY = now, p.Y
	st.jump = jumpState{}
	st.credit = 0
	st.pending, st.pendWorld, st.pendX, st.pendY, st.pendZ = true, p.World, p.X, p.Y, p.Z
	st.pendAt, st.pendFixAt, st.pendFix = now, now, ours
	st.bad = 0
}

// arrived reports a report from where a server move put the player.
func (st *moveState) arrived(world uint8, x, y, z float32) bool {
	if world != st.pendWorld {
		return false
	}
	dx, dz := x-st.pendX, z-st.pendZ
	return dx*dx+dz*dz <= mcArriveAcr*mcArriveAcr && y <= st.pendY+mcArriveUp && y >= st.pendY-mcArriveDown
}

// speed is the fastest speed the server gave the player in the last few
// seconds (0x00F8: the run speed with its buffs, masteries and holds): a
// speed taken away reaches the client a moment later.
func (st *moveState) speed(p *Player, now time.Time) float32 {
	s := max(p.SpeedNow(), 0)
	if s >= st.fastest || now.Sub(st.fastAt) > mcSpeedHeld {
		st.fastest, st.fastAt = s, now
	}
	return max(s, st.fastest)
}

// judgement is what the judge made of a report.
type judgement struct {
	why               string // "" believed
	severe            bool   // a fault no client on its own makes even once
	slow              bool   // too fast, and nothing else: the place it may have reached is taken
	sx, sy, sz        float32
	ahead             float32 // how far the report is past it
	held, wet, judged bool    // the place stands, swims, and the server knows
	credit, cost      float32
	jump              jumpState // the jump it is a report of (jump.go), none
}

// judgeMoveLocked judges one report: whether its place is taken, or (too
// fast) the place it may have reached on its way there (at). It is
// ApplyClientUpdate's, under the lock; tookMove follows the update. Must be
// called with the lock held.
func (w *World) judgeMoveLocked(p *Player, u *ClientUpdate, now time.Time) (take bool, at *[3]float32) {
	mode := MoveCheck()
	st := &p.mv
	if mode == MoveCheckOff || p.flyingNow(now) {
		// off, or a GM in the air by its own #fly (fly.go): nothing to judge
		st.init = false
		return true, nil
	}
	if !st.init || st.world != p.World || st.x != p.X || st.y != p.Y || st.z != p.Z {
		// something but this put the player where it is: a login, a
		// teleport, a respawn
		st.serverMoved(p, now, false)
	}
	world := p.World
	if u.HasWorld && u.World != world {
		// a report from another world: from before a world change the
		// client has not made yet, or a client that never made it
		// (listener's reportWorldMismatch says so)
		return mode != MoveCheckEnforce, nil
	}
	if st.pending {
		if w.arrivedLocked(st, world, u) {
			held, wet, judged := w.heldLocked(world, u.X, u.Y, u.Z)
			st.pending, st.bad = false, 0
			st.x, st.y, st.z, st.at = u.X, u.Y, u.Z, now
			st.judged, st.air, st.wet = judged, judged && !held && !wet, wet
			st.airAt, st.airY, st.airN = now, u.Y, 0
			st.jump = jumpState{}
			st.credit = 0 // a move back of ours: nothing in hand
			if !st.pendFix {
				st.credit = st.speed(p, now)*mcSlack + mcBurst // the server's own move: a second's worth
			}
			return true, nil
		}
		// from before the client heard of the move: not a place it is
		if mode == MoveCheckEnforce && now.Sub(st.pendFixAt) >= mcResend {
			st.pendFixAt = now
			w.sendMoveBackLocked(p, st.pendWorld, st.pendX, st.pendY, st.pendZ)
			w.moveLogLocked(p, now, fmt.Sprintf("no report from where it was put (%.1f, %.1f, %.1f) in %.0f s",
				st.pendX, st.pendY, st.pendZ, now.Sub(st.pendAt).Seconds()), u, "sent there again", true)
		}
		return mode != MoveCheckEnforce, nil
	}

	j := w.judgeLocked(p, st, u, now)
	if j.why == "" {
		st.take(u.Y, now, j)
		st.bad = 0
		return true, nil
	}
	st.why = j.why
	if mode == MoveCheckLog {
		w.moveLogLocked(p, now, j.why, u, "taken (log mode)", false)
		st.take(u.Y, now, j)
		st.bad = 0
		return true, nil
	}
	if j.slow {
		// too fast and nothing else: the place it may have reached on its
		// way, so the server's player never outruns its speed; the client,
		// that far ahead and more, is moved back to it
		st.take(j.sy, now, j)
		st.bad = 0
		if j.ahead <= mcDiverge {
			w.moveLogLocked(p, now, j.why, u, "slowed", false)
			return true, &[3]float32{j.sx, j.sy, j.sz}
		}
		w.moveLogLocked(p, now, j.why, u, "slowed, moved back", true)
		st.x, st.y, st.z = j.sx, j.sy, j.sz
		oldX, oldZ := p.X, p.Z
		p.X, p.Y, p.Z = j.sx, j.sy, j.sz
		w.SpatialIndex.Update(p.ServerID, oldX, oldZ, p.X, p.Z)
		w.moveBackLocked(p, now) // the place set here, the client sent to it
		return false, nil
	}
	// enforce: the place is not taken
	st.bad++
	if st.bad == 1 {
		st.badAt = now
	}
	fix := j.severe || st.bad >= mcBadReports || now.Sub(st.badAt) >= mcBadFor
	if fix {
		w.moveLogLocked(p, now, j.why, u, "moved back", true)
		w.moveBackLocked(p, now)
	} else {
		w.moveLogLocked(p, now, j.why, u, "not taken", false)
	}
	return false, nil
}

// arrivedLocked reports a report from where a server move put the player:
// near it, and not on the far side of a wall from it. Must be called with
// the lock held.
func (w *World) arrivedLocked(st *moveState, world uint8, u *ClientUpdate) bool {
	if !st.arrived(world, u.X, u.Y, u.Z) {
		return false
	}
	if mg, ok := w.ground.(MoveGround); ok && mg.Knows(world) {
		if _, hit := mg.Blocked(world, st.pendX, st.pendY+mcMiddle, st.pendZ, u.X, u.Y+mcMiddle, u.Z); hit {
			return false
		}
	}
	return true
}

// judgeLocked is what is wrong with a report's place against the last one
// taken (the judgement's why, "" nothing). It changes nothing but the speed
// remembered. Must be called with the lock held.
func (w *World) judgeLocked(p *Player, st *moveState, u *ClientUpdate, now time.Time) (j judgement) {
	world := p.World
	dt := now.Sub(st.at).Seconds()
	if dt < 0 {
		dt = 0
	}
	dx, dz, dy := u.X-st.x, u.Z-st.z, u.Y-st.y
	dh := float32(math.Hypot(float64(dx), float64(dz)))
	j.held, j.wet, j.judged = w.heldLocked(world, u.X, u.Y, u.Z)

	// the speed: a distance in hand that the speed fills, a few seconds'
	// worth (the reports' timing as they reach the server)
	v := st.speed(p, now) * mcSlack
	gap := float32(min(5, max(2, dt)))
	j.credit = min(v*gap+mcBurst, st.credit+v*float32(dt))
	j.cost = dh
	if j.wet || st.wet {
		up := max(0, dy-mcSwimUp*float32(dt))
		j.cost = float32(math.Hypot(float64(dh), float64(up)))
	}
	if j.cost > 3*j.credit+5+mcDiverge {
		// a jump no client makes on its way anywhere: not a place
		j.why = fmt.Sprintf("jumped %.1f in %.2f s, %.1f allowed at speed %.2f", j.cost, dt, j.credit, v/mcSlack)
		j.severe = true
		return j
	}
	fast := j.cost > j.credit+0.05
	if fast {
		j.why = fmt.Sprintf("too fast: %.1f in %.2f s, %.1f allowed at speed %.2f", j.cost, dt, j.credit, v/mcSlack)
	}
	if j.judged {
		dry := !j.wet && !st.wet
		if dry && !j.held {
			// a jump's, from a client that jumps (jump.go)
			var why string
			if j.jump, why = w.judgeJumpLocked(p, st, u, dy); why != "" {
				j.why = why
				return j
			}
		}
		rising := j.jump.on && !j.jump.down // a jump on its way up
		switch {
		case dy > mcAirRise && dry && !j.held && !rising:
			j.why = fmt.Sprintf("rose %.2f in the air", dy)
			return j
		case dy > dh*mcRiseSlope+mcRiseStep && dry:
			j.why = fmt.Sprintf("climbed %.2f over %.2f across", dy, dh)
			return j
		}
		if !j.held && !j.wet {
			start, from, n := st.airAt, st.airY, st.airN+1
			if !st.air || !st.judged {
				// it stood at the last report (or the ground was not
				// known): it left its floor since, and no more than a
				// moment ago
				start, from, n = st.at, st.y, 1
				if t := now.Add(-300 * time.Millisecond); start.Before(t) {
					start = t
				}
			}
			if j.jump.on {
				from = max(from, j.jump.floor) // a jump falls from the floor it left
			}
			// how long it has been in the air: a falling client reports
			// at its cadence (its state changes every frame), so its
			// reports count the fall's time even when a stalled link
			// brings them in a bunch; their arrival counts it too, less
			// what a stall may hold back (a client that reports seldom
			// while it hangs in the air)
			recv := now.Sub(start).Seconds()
			t := min(recv, max(float64(n)*mcCadence, recv-mcLag))
			need := fallMin(t - mcFallGrace)
			if u.Y > from-need+mcFallTol && !w.nearBlockerLocked(world, u.X, u.Y, u.Z) {
				j.why = fmt.Sprintf("held up in the air: %.1f s off its floor (%d reports), %.1f under where it left it (%.1f asked)",
					t, n, from-u.Y, need)
				return j
			}
		}
		if mg, ok := w.ground.(MoveGround); ok {
			if what, hit := mg.Blocked(world, st.x, st.y+mcMiddle, st.z, u.X, u.Y+mcMiddle, u.Z); hit {
				j.why, j.severe = "through "+what, true
				return j
			}
		}
	}
	if fast {
		// the place it may have reached on its way there, on the floor
		// there if there is one near
		f := max(0, j.credit) / j.cost
		j.slow = true
		j.sx, j.sy, j.sz = st.x+dx*f, st.y+dy*f, st.z+dz*f
		j.ahead = j.cost - max(0, j.credit)
		if mg, ok := w.ground.(MoveGround); ok && j.judged {
			if y, ok := mg.Support(world, j.sx, j.sy, j.sz, 1.5, 3); ok {
				j.sy = y
			}
			j.held, j.wet, _ = w.heldLocked(world, j.sx, j.sy, j.sz)
		}
		j.cost = j.credit
	}
	return j
}

// take moves the judge's state to a place taken (at height y): the speed's
// distance spent, the fall's start. The place itself tookMove takes.
func (st *moveState) take(y float32, now time.Time, j judgement) {
	st.credit = max(0, j.credit-j.cost)
	if !j.judged {
		st.judged, st.air, st.wet = false, false, false
		st.jump = jumpState{}
		return
	}
	switch {
	case j.held || j.wet:
		st.airAt, st.airY, st.airN = now, y, 0
	case !st.air || !st.judged:
		st.airAt, st.airY, st.airN = st.at, st.y, 1
		if t := now.Add(-300 * time.Millisecond); st.airAt.Before(t) {
			st.airAt = t
		}
	default:
		st.airN++
	}
	// the jump it is in: none once it stands or swims (judgeLocked judges no
	// jump there), its fall from the floor it left (jump.go)
	st.jump = j.jump
	if st.jump.on {
		st.airY = max(st.airY, st.jump.floor)
	}
	st.judged, st.air, st.wet = true, !j.held && !j.wet, j.wet
}

// tookMove notes the place ApplyClientUpdate took (or kept): the next
// report is judged from it, and a place moved by anything else is a server
// move. Must be called with the lock held.
func (st *moveState) tookMove(p *Player, now time.Time, taken bool) {
	if !st.init {
		return
	}
	if taken {
		st.x, st.y, st.z, st.at = p.X, p.Y, p.Z, now
	}
	st.world = p.World
}

// inAir reports the player in the air at its last report: the judge's own
// word where it enforces and knows the ground, else the walker's flags.
func (p *Player) inAir() bool {
	if MoveCheck() == MoveCheckEnforce && p.mv.init && p.mv.judged {
		return p.mv.air
	}
	return p.HasWalkFlags && !Standing(p.WalkFlags)
}

// heldLocked is whether a body with its feet at (x, y, z) stands on a floor
// or swims, and whether the server knows (the ground and the water of the
// world). Must be called with the lock held.
func (w *World) heldLocked(world uint8, x, y, z float32) (held, wet, judged bool) {
	mg, ok := w.ground.(MoveGround)
	if !ok || w.env == nil || !mg.Knows(world) {
		return false, false, false
	}
	if ek, ok := w.env.(envKnows); ok && !ek.Knows(world) {
		return false, false, false
	}
	if f, ok := mg.Support(world, x, y, z, mcFloorAbove, mcFloorDepth); ok && y-f <= mcStandTol {
		held = true
	}
	// in a liquid: the leaf at its middle, a little over and under it (the
	// walk's surface test asks 0.1 either side)
	for _, dy := range [...]float32{mcMiddle - 0.2, mcMiddle, mcMiddle + 0.2} {
		if f, ok := w.env.Flags(world, x, y+dy, z); ok && f&0xF != 0 {
			wet = true
			break
		}
	}
	return held, wet, true
}

// underLiquidLocked is CLIENT's IsUnderLiquid worked out from the place, as
// Move sets the flags: in a liquid at its middle and at no liquid's surface
// (over the middle by 0.1 not in it, under it by 0.1 in it); known false
// where the water is not. Must be called with the lock held.
func (w *World) underLiquidLocked(p *Player) (under, known bool) {
	if w.env == nil {
		return false, false
	}
	if ek, ok := w.env.(envKnows); ok && !ek.Knows(p.World) {
		return false, false
	}
	at := func(dy float32) uint32 {
		f, _ := w.env.Flags(p.World, p.X, p.Y+dy, p.Z)
		return f & 0xF
	}
	mid, up, dn := at(mcMiddle), at(mcMiddle+0.1), at(mcMiddle-0.1)
	return mid != 0 && dn&^up == 0, true
}

// nearBlockerLocked is a place over an invisible blocker's box (Sandstorm's
// collision-only SimpleSprite, 5 a side, which a body may stand on and the
// ground does not hold). Must be called with the lock held.
func (w *World) nearBlockerLocked(world uint8, x, y, z float32) bool {
	for _, n := range w.NPCs {
		if n.World != world || uint32(n.ModelID) != BlockerBody {
			continue
		}
		if dx, dz := n.X-x, n.Z-z; dx*dx+dz*dz < 6*6 && y > n.Y-1 && y < n.Y+8 {
			return true
		}
	}
	return false
}

// moveBackLocked moves the client back to the last place taken, or, when
// that was in the air, onto the floor under it, where the fall would have
// put it. Must be called with the lock held.
func (w *World) moveBackLocked(p *Player, now time.Time) {
	st := &p.mv
	if st.judged && st.air {
		if mg, ok := w.ground.(MoveGround); ok {
			if f, ok := mg.Support(st.world, st.x, st.y, st.z, mcFloorAbove, 2000); ok {
				st.y = f
			} else if p.HasStood && p.StoodWorld == st.world {
				st.x, st.y, st.z = p.StoodX, p.StoodY, p.StoodZ
			}
		}
		if p.World == st.world {
			oldX, oldZ := p.X, p.Z
			p.X, p.Y, p.Z = st.x, st.y, st.z
			if oldX != st.x || oldZ != st.z {
				w.SpatialIndex.Update(p.ServerID, oldX, oldZ, st.x, st.z)
			}
		}
		st.air = false
	}
	p.VelX, p.VelY, p.VelZ = 0, 0, 0
	st.bad = 0
	st.jump = jumpState{}
	st.pending, st.pendWorld, st.pendX, st.pendY, st.pendZ = true, st.world, st.x, st.y, st.z
	st.pendAt, st.pendFixAt, st.pendFix = now, now, true
	w.sendMoveBackLocked(p, st.world, st.x, st.y, st.z)
	cut := now.Add(-mcFixesWindow)
	k := 0
	for _, t := range st.fixes {
		if t.After(cut) {
			st.fixes[k] = t
			k++
		}
	}
	st.fixes = append(st.fixes[:k], now)
	if len(st.fixes) == mcFixesNoted {
		log.Printf("MOVECHECK: %s moved back %d times in %.0f minutes (the last: %s)",
			p.Name, len(st.fixes), mcFixesWindow.Minutes(), st.why)
	}
}

// sendMoveBackLocked is the teleport (0x00B2) to a place in the player's
// own world, as a GM's #goto sends it. Must be called with the lock held.
func (w *World) sendMoveBackLocked(p *Player, world uint8, x, y, z float32) {
	if w.SessionManager == nil {
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil {
		return
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTeleport)
	wr := buffer.NewWriter(msg.Span())
	wr.WriteUint16(uint16(protocol.OpcodeTeleport))
	wr.WriteByte(world)
	wr.WriteFloat32(x)
	wr.WriteFloat32(y)
	wr.WriteFloat32(z)
	wr.WriteFloat32(p.Facing)
	wr.WriteUint32(0)
	wr.WriteUint32(0)
	wr.WriteUint32(0) // the token (0x0049 answers it), as #goto's
	msg.SetSize(wr.Position())
	sess.Queue().AddWithSequence(msg)
}

// moveLogLocked logs a report not believed and what became of it, a line
// every few seconds a player at most but for a move back. Must be called
// with the lock held.
func (w *World) moveLogLocked(p *Player, now time.Time, why string, u *ClientUpdate, what string, always bool) {
	st := &p.mv
	if !always && now.Sub(st.loggedAt) < mcLogEvery {
		st.held++
		return
	}
	more := ""
	if st.held > 0 {
		more = fmt.Sprintf(" (+%d more since)", st.held)
	}
	log.Printf("MOVECHECK %s: %s at (%.1f, %.1f, %.1f) world %d from (%.1f, %.1f, %.1f), flags %#x: %s%s",
		what, p.Name, u.X, u.Y, u.Z, p.World, st.x, st.y, st.z, u.WalkFlags, why, more)
	st.loggedAt, st.held = now, 0
}

// silentAirSystem puts down a player whose last report was in the air and
// that has sent none since: a falling client reports at every step (its
// state changes), so one that stops in the air is held up by nothing but
// its silence. Must be called with the lock held.
func (w *World) silentAirSystem(now time.Time) {
	if MoveCheck() != MoveCheckEnforce || w.TickCount%10 != 0 {
		return
	}
	for _, p := range w.Players {
		st := &p.mv
		if !st.init || !st.judged || !st.air || st.pending || p.HP <= 0 || now.Sub(st.at) < mcSilentAir {
			continue
		}
		if st.x != p.X || st.y != p.Y || st.z != p.Z || st.world != p.World {
			continue // moved by the server since: its judge starts again
		}
		log.Printf("MOVECHECK put down: %s silent %.1f s in the air at (%.1f, %.1f, %.1f)",
			p.Name, now.Sub(st.at).Seconds(), p.X, p.Y, p.Z)
		st.why = "silent in the air"
		w.moveBackLocked(p, now)
	}
}

// MoveCheckSummary is a line for each player the check moved back in the
// last few minutes: how many times, and the last fault.
func (w *World) MoveCheckSummary(now time.Time) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []string
	cut := now.Add(-mcFixesWindow)
	for _, p := range w.Players {
		n := 0
		for _, t := range p.mv.fixes {
			if t.After(cut) {
				n++
			}
		}
		if n > 0 {
			out = append(out, fmt.Sprintf("%s: moved back %d times (last: %s)", p.Name, n, p.mv.why))
		}
	}
	sort.Strings(out)
	return out
}
