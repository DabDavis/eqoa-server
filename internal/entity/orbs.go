package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// orbs.go — the teleport orbs (table orbs, database/orbs.go; the user,
// 2026-11-09: "the erudite starts that are in towers the orb wont teleport them
// yet to the main highbourne"): a place a player steps into to be sent to
// another, with no window, no word and nothing to click, as the live game's
// pads did.
//
// Highbourne's three guild towers are sealed: an Erudite magician, enchanter
// or wizard starts on a floor of its tower 55 to 115 over the city, and no
// walk leads down (a flood fill of the ground files from each start reaches
// that tower's floors alone, and its pad). Each tower's top floor holds a pad
// (the disc's SimpleSprite C28D156A, an octagon 3.7 across and 0.61 high,
// under a column of light, its guild's banners behind it), and the hall on the
// first floor of the senators' building holds three more, each with one
// tower's banner: tools/seed_orbs.py and tools/orbs/highbourne.json have
// them, paired by those banners. Nothing on the server sent anyone, so the
// three classes started where they could never leave.
//
// The rules, each the server's own:
//
//   - a player is in an orb when the feet a report gives are within its
//     radius across and from OrbBelow under its height to OrbAbove over it
//     (database.OrbHolds): on the pad, not the floor above or below it;
//   - an orb sends a player when a report the server takes (movecheck.go)
//     brings it in: one that stood in it already is not sent again, so a
//     player stands on the pad it arrived beside, or was put on by a login or
//     a GM, until it steps off and back on. A server's move since the last
//     report (a login, a teleport, a coach, an orb's own trip) is told by the
//     place having changed between reports, as the move check tells one;
//   - it sends by the travel the gates and the Summoning Portal use
//     (gates.go's queue, handlers' travelPlayer): the 0x00B2 there with the
//     row's facing, the world loaded when it changes, a pet along (pet.go);
//   - the destination is a spot beside the pair, never in it, and for
//     OrbRest after a trip no orb sends that player: the reports a client
//     sent before it heard of the move (from inside the orb it stepped into,
//     taken as they come when the move check logs and does not enforce) send
//     no one back;
//   - the dead, a GM flying (#fly) and a session loading a world are not
//     sent; anyone else is, in a fight or not, whatever its race or class.
//
// The move check takes the trip as any server move: reports from before the
// client heard of it are not its place until one comes from where it was put
// (movecheck.go's pending), so a client honest about where it is never reads
// as a cheat for it. A PS2 is sent as any client is: the orb is the server's.

// OrbRest is how long after an orb sends a player no orb sends it again.
const OrbRest = 3 * time.Second

// orbState is the orbs, by the world they stand in (table orbs).
type orbState struct {
	byWorld map[uint8][]database.Orb
	n       int
}

// orbWatch is a player's standing with the orbs (Player.orb): the orb its last
// place was in (0 none), that place, whether it has one yet, and until when
// its orbs rest.
type orbWatch struct {
	in      int64
	known   bool
	world   uint8
	x, y, z float32
	rest    time.Time
}

// SetOrbs sets the orbs (table orbs).
func (w *World) SetOrbs(orbs []database.Orb) {
	by := make(map[uint8][]database.Orb)
	for _, o := range orbs {
		by[o.World] = append(by[o.World], o)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.orbs = orbState{byWorld: by, n: len(orbs)}
}

// LoadOrbs reads table orbs into the world: at the start and after a GM's
// edit. Call it without the database held. A row the game cannot use is
// logged and left out. It reports how many it read.
func (w *World) LoadOrbs(db *database.SQLiteDB) (int, error) {
	orbs, bad, err := db.Orbs()
	if err != nil {
		return 0, fmt.Errorf("orbs: %w", err)
	}
	for _, b := range bad {
		log.Printf("WARNING: orbs %s: left out", b)
	}
	w.SetOrbs(orbs)
	return len(orbs), nil
}

// OrbsSummary says what the orbs are, for the log at the start.
func (w *World) OrbsSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var worlds []string
	for wn, os := range w.orbs.byWorld {
		worlds = append(worlds, fmt.Sprintf("world %d: %d", wn, len(os)))
	}
	if len(worlds) == 0 {
		return "orbs: none in table orbs"
	}
	return fmt.Sprintf("orbs: %d in table orbs (%s)", w.orbs.n, strings.Join(worlds, ", "))
}

// orbAtLocked is the orb whose place holds feet at (x, y, z) in world, the
// nearest across when two do; nil for none. Must be called with the lock
// held.
func (w *World) orbAtLocked(world uint8, x, y, z float32) *database.Orb {
	var best *database.Orb
	var bestD float32
	for i := range w.orbs.byWorld[world] {
		o := &w.orbs.byWorld[world][i]
		if !database.OrbHolds(*o, x, y, z) {
			continue
		}
		dx, dz := x-o.X, z-o.Z
		if d := dx*dx + dz*dz; best == nil || d < bestD {
			best, bestD = o, d
		}
	}
	return best
}

// orbID is an orb's id, 0 for none.
func orbID(o *database.Orb) int64 {
	if o == nil {
		return 0
	}
	return o.ID
}

// orbStepLocked is the orbs' look at one report (ApplyClientUpdate's, after
// the move check), at where the server holds the player after it (a place the
// check did not take leaves it where it stood, which is no step in): before is
// where it stood as the report came. A report that brings a living player
// into an orb, its orbs not resting, queues its trip (the travel hook,
// runTravels). Must be called with the lock held.
func (w *World) orbStepLocked(p *Player, before [3]float32, now time.Time) {
	st := &p.orb
	if !st.known || st.world != p.World || st.x != before[0] || st.y != before[1] || st.z != before[2] {
		// moved by the server since the last report: an orb it was put in
		// is one it stands in already
		st.in = orbID(w.orbAtLocked(p.World, before[0], before[1], before[2]))
	}
	defer func() { st.known, st.world, st.x, st.y, st.z = true, p.World, p.X, p.Y, p.Z }()
	o := w.orbAtLocked(p.World, p.X, p.Y, p.Z)
	was := st.in
	st.in = orbID(o)
	if o == nil || o.ID == was || now.Before(st.rest) || p.IsDead() || p.respawnPending() || p.flyingNow(now) {
		return
	}
	if w.SessionManager == nil {
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil || sess.StateChannelsHeld() {
		return // gone, or loading a world
	}
	st.rest = now.Add(OrbRest)
	to := GateSpot{World: o.ToWorld, X: o.ToX, Y: o.ToY, Z: o.ToZ, Place: o.Name}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Spell: o.Name, Facing: o.ToFacing, HasFacing: true})
	log.Printf("Orb: %s steps into %d %q at (%.1f, %.1f, %.1f) world=%d: sent to (%.1f, %.1f, %.1f) world=%d facing %.2f",
		p.Name, o.ID, o.Name, p.X, p.Y, p.Z, p.World, o.ToX, o.ToY, o.ToZ, o.ToWorld, o.ToFacing)
}
