package entity

// npctour.go — an NPC's tour of a place for anyone aboard its vehicle (table
// npctours, database/npctours.go; the owner, 2026-11-17: "lets have santa claus
// give a scenic tour of freeport for anyone on the sled, he announces it in chat.
// he will initiate once a player talks to him and chooses to start", then "the
// sled with the pegasus and santa will altogether fly together").
//
// A tour is idle, counting down, flying or resting:
//
//   - idle: its host (Santa Claus) offers it when talked to (handlers/npctour.go).
//     A player who starts it is aboard, and the host shouts it to everyone near
//     (Lines.Announce) with the countdown's seconds.
//   - counting down: whoever talks to the host and climbs aboard is aboard too;
//     at its end everyone standing within Board of the vehicle (Santa's Sleigh)
//     rides as well, alive, in its world.
//   - flying: the vehicle and its members (the team, the host at the reins) fly
//     the route as one body: each kept where it stands from the vehicle at home
//     (their npcs rows' spawns), turned with the vehicle's heading about it and
//     lifted with the route's climb along it (a team pulling up ahead). The route
//     is a Catmull-Rom curve through the waypoints, flown at Speed with a
//     start and a stop at each point with a pause (and the first and last),
//     2.5 s each way. Each member's velocity is the route's, so every client
//     carries it on between the channel's updates (CLIENT's ExtrapolateMove; an
//     NPC more than 1.5 over a floor is not dropped onto it, ExtrapolateGround).
//     The riders stand on the vehicle's deck (Deck, its frame), the server's
//     place for them whatever their clients report (ApplyClientUpdate takes no
//     place from a rider), run speed 0. A client that rides (OpcodeRide: vanilla,
//     layout 2) keeps its walker on the deck as it draws the vehicle; one that
//     cannot (a PS2) is set down at each waypoint's View as the vehicle sets off
//     for it, to watch that stretch from the ground (the vehicle hangs there for
//     the point's pause), and everyone else still sees it on the deck. The host's
//     line at each point goes to the riders in chat.
//   - at the last point (the vehicle's home) everyone is home as their rows
//     stand, and the riders are set down at Drop beside it; then it rests
//     Cooldown before it is offered again (the host says how long).
//
// A rider leaves on the way by logging out (saved at Drop: SavePlace), by
// another world or a move of the server's (a gate, a GM's #goto: wherever that
// took it), or by dying; one set down before the end is told (Lines.Off).
// Members gone from the world (their season turned off, a GM's edit) end the
// flight: everyone home, the riders at Drop. A keep restart ends a flight the
// same way before it writes anything down (StandNPCs). While the host is out
// the NPC named Lines.Stand (the Holiday Elf) says where he is to anyone who
// talks to it (TourStandLine).

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The tour's phases.
const (
	tourIdle = iota
	tourCountdown
	tourFlying
	tourResting
)

// tourAccel is how long the vehicle takes to reach its speed from a stop, and
// to stop.
const tourAccel = 2.5

// tourSampleStep is the route's arc-length table's step.
const tourSampleStep = 0.25

// tourStep is how far ahead (seconds) a member's velocity is measured: a tick.
const tourStep = 0.1

// Lines a tour says, where its row's lines leave them blank.
var tourDefaults = database.TourLines{
	Offer:     "Ho ho ho! Would you like a ride in my sleigh? I'll show you Freeport from above!",
	Start:     "Take a sleigh ride tour of Freeport",
	NotNow:    "Not now",
	Announce:  "Ho ho ho! All aboard! Santa's sleigh leaves on a tour of Freeport in %d seconds. Climb aboard by the tree!",
	Countdown: "Last call! The sleigh leaves in %d seconds. Climb aboard by the tree!",
	Aboard:    "Ho ho! You're aboard. Stand by the sleigh and hold on tight!",
	Depart:    "Hold on to your hats! Up, Rudolph! Up, Dasher and Dancer, Prancer and Vixen!",
	Land:      "Ho ho ho! Back by the tree, safe and sound. Merry Christmas!",
	Empty:     "No one aboard? Then the team can rest a little longer.",
	Away:      "Hold on tight! We'll be back by the tree in about %d seconds.",
	Cooldown:  "The team needs a breather. The next tour leaves in about %d seconds. Come back and ask me then!",
	Off:       "You've left Santa's sleigh. The tour goes on without you.",
	StandLine: "Santa's out on his sleigh! He'll be back by the tree in about %d seconds.",
}

// line is the row's line of that name, else the default.
func tourLine(row, def string) string {
	if strings.TrimSpace(row) != "" {
		return row
	}
	return def
}

// tourSample is a point of the route's arc-length table: where, its heading
// (the curve's on the ground plane, held through a climb or a descent straight
// up), and its climb (rise over run, held to ±0.8).
type tourSample struct {
	s, x, y, z, h, slope float32
}

// tourDef is a tour as the world flies it: its row, the route's table, each
// waypoint's place along it and the times the vehicle reaches and leaves each.
type tourDef struct {
	database.NPCTour
	key     string
	samples []tourSample
	knotS   []float32
	arrive  []float64 // seconds from the start: the vehicle at point i
	leave   []float64 // and off again
	total   float64
	home    float32 // the vehicle's heading at the start (the team ahead of it at home)
	runs    []tourMotion
}

// tourMotion is a run from one stop to the next: a trapezoid of speed.
type tourMotion struct {
	t0, s0, d, dur float64
	v, a           float64
}

// at is how far along the run it is t seconds in.
func (m tourMotion) at(t float64) float64 {
	t = max(0, min(t, m.dur))
	if m.d <= 0 {
		return 0
	}
	ta := m.v / m.a
	if m.dur <= 2*ta+1e-9 { // too short to reach the speed: a triangle
		half := m.dur / 2
		if t <= half {
			return 0.5 * m.a * t * t
		}
		return m.d - 0.5*m.a*(m.dur-t)*(m.dur-t)
	}
	switch {
	case t < ta:
		return 0.5 * m.a * t * t
	case t < m.dur-ta:
		return 0.5*m.a*ta*ta + m.v*(t-ta)
	}
	return m.d - 0.5*m.a*(m.dur-t)*(m.dur-t)
}

func newTourMotion(t0, s0, d, v float64) tourMotion {
	a := v / tourAccel
	m := tourMotion{t0: t0, s0: s0, d: d, v: v, a: a}
	if d >= v*tourAccel {
		m.dur = d/v + tourAccel
	} else {
		vp := math.Sqrt(a * d)
		m.dur = 2 * vp / a
	}
	return m
}

// buildTourDef lays a tour's route out.
func buildTourDef(t database.NPCTour) *tourDef {
	d := &tourDef{NPCTour: t, key: strings.ToLower(strings.TrimSpace(t.Name))}
	pts := t.Route
	n := len(pts)
	p := func(i int) [3]float64 {
		i = max(0, min(i, n-1))
		return [3]float64{float64(pts[i].X), float64(pts[i].Y), float64(pts[i].Z)}
	}
	var s float64
	var last [3]float64
	first := true
	d.knotS = make([]float32, n)
	for i := 0; i+1 < n; i++ {
		p0, p1, p2, p3 := p(i-1), p(i), p(i+1), p(i+2)
		chord := math.Sqrt(sq(p2[0]-p1[0]) + sq(p2[1]-p1[1]) + sq(p2[2]-p1[2]))
		steps := max(1, int(math.Ceil(chord/tourSampleStep)))
		for k := 0; k <= steps; k++ {
			if k == 0 && !first {
				continue
			}
			u := float64(k) / float64(steps)
			var q [3]float64
			for c := 0; c < 3; c++ {
				q[c] = catmull(p0[c], p1[c], p2[c], p3[c], u)
			}
			// never under the lower of the leg's ends (a curve dips past a
			// point it comes down to: the vehicle's ground at home)
			q[1] = max(q[1], min(p1[1], p2[1]))
			if !first {
				s += math.Sqrt(sq(q[0]-last[0]) + sq(q[1]-last[1]) + sq(q[2]-last[2]))
			}
			d.samples = append(d.samples, tourSample{s: float32(s), x: float32(q[0]), y: float32(q[1]), z: float32(q[2])})
			last, first = q, false
		}
		d.knotS[i+1] = float32(s)
	}
	// headings and climbs from the neighbours, a near-vertical stretch holding
	// the last heading found (and the start the first)
	firstH := float32(-1)
	for i := range d.samples {
		a, b := d.samples[max(0, i-2)], d.samples[min(len(d.samples)-1, i+2)]
		dx, dy, dz := b.x-a.x, b.y-a.y, b.z-a.z
		hz := float32(math.Hypot(float64(dx), float64(dz)))
		if hz > 0.3*float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) && hz > 1e-4 {
			d.samples[i].h = headingTo(dx, dz)
			d.samples[i].slope = max(-0.8, min(0.8, dy/hz))
			if firstH < 0 {
				firstH = d.samples[i].h
			}
		} else {
			d.samples[i].h = -1
		}
	}
	if firstH < 0 {
		firstH = 0
	}
	held := firstH
	for i := range d.samples {
		if d.samples[i].h < 0 {
			d.samples[i].h = held
		} else {
			held = d.samples[i].h
		}
	}
	d.home = d.samples[0].h
	// the timeline: a run from each stop to the next
	stop := func(i int) bool { return i == 0 || i == n-1 || pts[i].Pause > 0 }
	d.arrive, d.leave = make([]float64, n), make([]float64, n)
	t0 := 0.0
	from := 0
	for i := 1; i < n; i++ {
		if !stop(i) {
			continue
		}
		m := newTourMotion(t0, float64(d.knotS[from]), float64(d.knotS[i]-d.knotS[from]), float64(t.Speed))
		d.runs = append(d.runs, m)
		for k := from + 1; k <= i; k++ {
			d.arrive[k] = t0 + m.timeTo(float64(d.knotS[k]-d.knotS[from]))
		}
		t0 += m.dur
		d.leave[i] = t0 + float64(pts[i].Pause)
		if i < n-1 {
			t0 = d.leave[i]
		}
		for k := from + 1; k < i; k++ {
			d.leave[k] = d.arrive[k]
		}
		from = i
	}
	d.total = t0
	return d
}

// timeTo is when the run is d along.
func (m tourMotion) timeTo(d float64) float64 {
	lo, hi := 0.0, m.dur
	for k := 0; k < 50; k++ {
		mid := (lo + hi) / 2
		if m.at(mid) < d {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

func sq(v float64) float64 { return v * v }

// catmull is a uniform Catmull-Rom curve's coordinate between p1 and p2.
func catmull(p0, p1, p2, p3, u float64) float64 {
	u2, u3 := u*u, u*u*u
	return 0.5 * (2*p1 + (-p0+p2)*u + (2*p0-5*p1+4*p2-p3)*u2 + (-p0+3*p1-3*p2+p3)*u3)
}

// sAt is how far along the route the vehicle is t seconds into the flight.
func (d *tourDef) sAt(t float64) float64 {
	for _, m := range d.runs {
		if t < m.t0 {
			return m.s0 // a pause before this run
		}
		if t <= m.t0+m.dur {
			return m.s0 + m.at(t-m.t0)
		}
	}
	return float64(d.knotS[len(d.knotS)-1])
}

// sample is the route at s along it.
func (d *tourDef) sample(s float64) tourSample {
	ss := d.samples
	i := sort.Search(len(ss), func(i int) bool { return float64(ss[i].s) >= s })
	if i <= 0 {
		return ss[0]
	}
	if i >= len(ss) {
		return ss[len(ss)-1]
	}
	a, b := ss[i-1], ss[i]
	f := float32(0)
	if b.s > a.s {
		f = (float32(s) - a.s) / (b.s - a.s)
	}
	h := a.h + angleBetween(a.h, b.h)*f
	return tourSample{s: float32(s), x: a.x + (b.x-a.x)*f, y: a.y + (b.y-a.y)*f, z: a.z + (b.z-a.z)*f, h: h, slope: a.slope + (b.slope-a.slope)*f}
}

// angleBetween is the turn from a to b, within (-pi, pi].
func angleBetween(a, b float32) float32 {
	d := math.Mod(float64(b-a), 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d <= -math.Pi {
		d += 2 * math.Pi
	}
	return float32(d)
}

// pose is the vehicle t seconds into the flight: where, its turn from home
// (eased back to none over the last few seconds, so it lands as it stood), and
// its climb.
func (d *tourDef) pose(t float64) (x, y, z, turn, slope float32) {
	smp := d.sample(d.sAt(t))
	turn = angleBetween(d.home, smp.h)
	if left := d.total - t; left < 4 {
		turn *= float32(max(0, left/4))
	}
	// the climb's lift from none on the ground at both ends
	slope = smp.slope * float32(max(0, min(1, t/3, (d.total-t)/3)))
	return smp.x, smp.y, smp.z, turn, slope
}

// tourMember is an NPC flying with the vehicle: its id, its place from the
// vehicle at home (world axes) and its facing's, its home.
type tourMember struct {
	id        EntityID
	rel       [3]float32
	relFacing float32
	home      [4]float32
	vehicle   bool
}

// tourRider is a player aboard: its place on the deck, whether its client rides
// (OpcodeRide), and where the tour last put it.
type tourRider struct {
	tour   string
	slot   int
	rides  bool
	last   [3]float32
	world  uint8
	drop   [4]float32
	viewed int // the last waypoint whose view it was set down at
}

// tourRun is a tour's state.
type tourRun struct {
	def     *tourDef
	phase   int
	at      time.Time // the phase's start
	starter string
	joined  map[EntityID]bool
	riders  map[EntityID]*tourRider
	members []tourMember
	lastT   float64
	shouted bool
	vehicle EntityID
	host    EntityID
}

// tourState is the world's tours.
type tourState struct {
	defs map[string]*tourDef // by key
	runs map[string]*tourRun
	// walk-ons: each deck NPC (npcwalkon "deck") by id, and the names to send
	decks   map[EntityID]database.NPCWalkOn
	walkOns []database.NPCWalkOn
}

// SetNPCTours puts tours and walk-ons in the world (LoadDecorLights reads them
// with the decorations); a tour flying goes on with the route it had.
func (w *World) SetNPCTours(tours []database.NPCTour, walkOns []database.NPCWalkOn, active map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	defs := map[string]*tourDef{}
	for _, t := range tours {
		if t.Season != "" && !active[database.SeasonKey(t.Season)] {
			continue
		}
		d := buildTourDef(t)
		defs[d.key] = d
	}
	w.tours.defs = defs
	if w.tours.runs == nil {
		w.tours.runs = map[string]*tourRun{}
	}
	for k, r := range w.tours.runs {
		if r.phase == tourIdle || r.phase == tourResting {
			if d := defs[k]; d != nil {
				r.def = d
			} else if r.phase == tourIdle {
				delete(w.tours.runs, k)
			}
		}
	}
	var on []database.NPCWalkOn
	for _, wo := range walkOns {
		if wo.Season == "" || active[database.SeasonKey(wo.Season)] {
			on = append(on, wo)
		}
	}
	w.tours.walkOns = on
	w.refreshDecksLocked()
}

// refreshDecksLocked finds the NPCs of the deck walk-ons' names. Must be called
// with the lock held.
func (w *World) refreshDecksLocked() {
	decks := map[EntityID]database.NPCWalkOn{}
	byName := map[string]database.NPCWalkOn{}
	for _, wo := range w.tours.walkOns {
		if wo.Mode == database.WalkOnDeck {
			byName[wo.Name] = wo
		}
	}
	if len(byName) > 0 {
		for id, npc := range w.NPCs {
			if wo, ok := byName[database.NPCClassKey(npc.Name)]; ok && !npc.Temporary {
				decks[id] = wo
			}
		}
	}
	w.tours.decks = decks
}

// WalkOns is the walk-on rows in force (sent with the decorations).
func (w *World) WalkOns() []database.NPCWalkOn {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]database.NPCWalkOn(nil), w.tours.walkOns...)
}

// onDeckLocked is a body with its feet at (x, y, z) standing on a deck NPC's
// deck: within its radius across and at most its height over its feet (a
// little under them too: the report's rounding). Must be called with the lock
// held.
func (w *World) onDeckLocked(world uint8, x, y, z float32) bool {
	for id, wo := range w.tours.decks {
		n := w.NPCs[id]
		if n == nil || n.World != world || n.Dead {
			continue
		}
		if distXZ(n.X, n.Z, x, z) <= wo.Radius && y >= n.Y-0.3 && y <= n.Y+wo.Height+0.3 {
			return true
		}
	}
	return false
}

// tourOfHostLocked is the tour whose host npc is, and its run (made idle when it
// has none). Must be called with the lock held.
func (w *World) tourOfHostLocked(npc *NPC) (*tourDef, *tourRun) {
	if npc == nil {
		return nil, nil
	}
	key := database.NPCClassKey(npc.Name)
	for k, d := range w.tours.defs {
		if database.NPCClassKey(d.Host) != key || npc.World != d.World {
			continue
		}
		r := w.tours.runs[k]
		if r == nil {
			r = &tourRun{def: d}
			w.tours.runs[k] = r
		}
		return d, r
	}
	// a host out on its flight whose tour was reloaded away
	for _, r := range w.tours.runs {
		if r.host == npc.ServerID && r.phase == tourFlying {
			return r.def, r
		}
	}
	return nil, nil
}

// TourTalk is a talk to a tour's host: what it asks (the box's question and
// answers, for the handlers to send) or says (one line, no box). ok false: npc
// hosts no tour.
type TourTalk struct {
	Tour    string
	Ask     string
	Answers []string // [0] starts (or boards), [1] not now
	Say     string
}

// TalkTour is the talk of p to npc, a tour's host.
func (w *World) TalkTour(p *Player, npc *NPC, now time.Time) (TourTalk, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d, r := w.tourOfHostLocked(npc)
	if d == nil {
		return TourTalk{}, false
	}
	l := d.Lines
	out := TourTalk{Tour: d.Name}
	switch r.phase {
	case tourIdle:
		out.Ask = tourLine(l.Offer, tourDefaults.Offer)
		out.Answers = []string{tourLine(l.Start, tourDefaults.Start), tourLine(l.NotNow, tourDefaults.NotNow)}
	case tourCountdown:
		if r.joined[p.ServerID] {
			out.Say = tourLine(l.Aboard, tourDefaults.Aboard)
			break
		}
		left := r.def.Countdown - float32(now.Sub(r.at).Seconds())
		out.Ask = fmt.Sprintf("We leave in %d seconds! Climb aboard?", max(1, int(math.Ceil(float64(left)))))
		out.Answers = []string{"Climb aboard", tourLine(l.NotNow, tourDefaults.NotNow)}
	case tourFlying:
		out.Say = fmt.Sprintf(tourLine(l.Away, tourDefaults.Away), r.secondsHome(now))
	case tourResting:
		out.Say = fmt.Sprintf(tourLine(l.Cooldown, tourDefaults.Cooldown), r.secondsRested(now))
	}
	return out, true
}

// secondsHome is how long the flight has left, whole seconds, at least 1.
func (r *tourRun) secondsHome(now time.Time) int {
	return max(1, int(math.Ceil(r.def.total-now.Sub(r.at).Seconds())))
}

// secondsRested is how long the rest has left, whole seconds, at least 1.
func (r *tourRun) secondsRested(now time.Time) int {
	return max(1, int(math.Ceil(float64(r.def.Cooldown)-now.Sub(r.at).Seconds())))
}

// AnswerTour is p's answer to its host's box of tour: start it (idle) or climb
// aboard (counting down); anything else, or a box the tour has moved on from,
// gets the host's word for where it is. It returns what the host says ("":
// nothing).
func (w *World) AnswerTour(sess *session.Session, p *Player, tour string, yes bool, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.tours.defs[strings.ToLower(strings.TrimSpace(tour))]
	if d == nil || !yes {
		return ""
	}
	r := w.tours.runs[d.key]
	if r == nil {
		r = &tourRun{def: d}
		w.tours.runs[d.key] = r
	}
	l := d.Lines
	switch r.phase {
	case tourIdle:
		host := w.tourNPCLocked(d, d.Host)
		veh := w.tourNPCLocked(d, d.Vehicle)
		if host == nil || veh == nil {
			return ""
		}
		r.def = d
		r.phase, r.at, r.starter = tourCountdown, now, p.Name
		r.joined = map[EntityID]bool{p.ServerID: true}
		r.riders, r.members, r.shouted, r.lastT = nil, nil, false, 0
		log.Printf("Tour %q: %s starts it; leaves in %.0f s", d.Name, p.Name, d.Countdown)
		w.tourShoutLocked(d, host, fmt.Sprintf(tourLine(l.Announce, tourDefaults.Announce), int(d.Countdown)))
		return tourLine(l.Aboard, tourDefaults.Aboard)
	case tourCountdown:
		r.joined[p.ServerID] = true
		log.Printf("Tour %q: %s climbs aboard", d.Name, p.Name)
		return tourLine(l.Aboard, tourDefaults.Aboard)
	case tourFlying:
		return fmt.Sprintf(tourLine(l.Away, tourDefaults.Away), r.secondsHome(now))
	case tourResting:
		return fmt.Sprintf(tourLine(l.Cooldown, tourDefaults.Cooldown), r.secondsRested(now))
	}
	return ""
}

// TourStandLine is what npc says for a tour's host while the host is out ("",
// false: nothing).
func (w *World) TourStandLine(npc *NPC, now time.Time) (string, bool) {
	if npc == nil {
		return "", false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	key := database.NPCClassKey(npc.Name)
	for _, r := range w.tours.runs {
		if r.phase != tourFlying || r.def == nil || database.NPCClassKey(r.def.Lines.Stand) != key {
			continue
		}
		return fmt.Sprintf(tourLine(r.def.Lines.StandLine, tourDefaults.StandLine), r.secondsHome(now)), true
	}
	return "", false
}

// tourNPCLocked is the NPC of name in the tour's world, alive, a row's (not a
// GM's spawn). Must be called with the lock held.
func (w *World) tourNPCLocked(d *tourDef, name string) *NPC {
	var best *NPC
	for _, npc := range w.NPCs {
		if npc.Temporary || npc.Dead || npc.World != d.World || !strings.EqualFold(npc.Name, name) {
			continue
		}
		if best == nil || npc.ServerID < best.ServerID {
			best = npc
		}
	}
	return best
}

// tourShoutLocked is the host's shout to every player in the tour's world near
// its vehicle's home (1000 across: the town). Must be called with the lock held.
func (w *World) tourShoutLocked(d *tourDef, host *NPC, text string) {
	if w.SessionManager == nil || text == "" {
		return
	}
	line := host.Name + " shouts: " + text
	n := 0
	for _, p := range w.Players {
		if p.World != d.World || distXZ(p.X, p.Z, d.Route[0].X, d.Route[0].Z) > 1000 {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendColoredLocked(sess, line, [3]byte{0x3F, 0xFF, 0x3F})
			n++
		}
	}
	log.Printf("Tour %q: shout %q to %d players", d.Name, text, n)
}

// tourSayLocked is the host's line to the riders. Must be called with the lock
// held.
func (w *World) tourSayLocked(r *tourRun, text string) {
	if w.SessionManager == nil || text == "" {
		return
	}
	line := r.def.Host + ": " + text
	for id := range r.riders {
		if sess := w.findSessionByEntityIDLocked(id); sess != nil {
			w.sendChatToSession(sess, line)
		}
	}
}

// tourSystem moves the tours on. Must be called with the lock held.
func (w *World) tourSystem(now time.Time) {
	for _, r := range w.tours.runs {
		switch r.phase {
		case tourCountdown:
			left := float64(r.def.Countdown) - now.Sub(r.at).Seconds()
			if !r.shouted && left <= 10 && left >= 2 && r.def.Countdown > 15 {
				r.shouted = true
				if host := w.tourNPCLocked(r.def, r.def.Host); host != nil {
					w.tourShoutLocked(r.def, host, fmt.Sprintf(tourLine(r.def.Lines.Countdown, tourDefaults.Countdown), int(math.Ceil(left))))
				}
			}
			if left <= 0 {
				w.tourDepartLocked(r, now)
			}
		case tourFlying:
			t := now.Sub(r.at).Seconds()
			if t >= r.def.total {
				w.tourLandLocked(r, now, "landed")
				continue
			}
			w.tourStepLocked(r, t, now)
		case tourResting:
			if now.Sub(r.at).Seconds() >= float64(r.def.Cooldown) {
				r.phase, r.at = tourIdle, now
				if d := w.tours.defs[r.def.key]; d != nil {
					r.def = d
				}
			}
		}
	}
}

// tourDepartLocked sets a tour off: its members found and placed from the
// vehicle at home, its riders aboard. Must be called with the lock held.
func (w *World) tourDepartLocked(r *tourRun, now time.Time) {
	d := r.def
	veh := w.tourNPCLocked(d, d.Vehicle)
	host := w.tourNPCLocked(d, d.Host)
	if veh == nil || host == nil {
		log.Printf("Tour %q: no %s or %s in world %d: not flown", d.Name, d.Vehicle, d.Host, d.World)
		r.phase, r.at = tourIdle, now
		return
	}
	// the riders: whoever started or climbed aboard, near enough to be fetched,
	// and everyone standing by the vehicle
	var ids []EntityID
	for _, p := range w.Players {
		if p.World != d.World || p.IsDead() || p.tourRide != nil {
			continue
		}
		dist := distXZ(p.X, p.Z, veh.X, veh.Z)
		near := dist <= d.Board && float32(math.Abs(float64(p.Y-veh.Y))) < 6
		if near || (r.joined[p.ServerID] && dist <= 150) {
			ids = append(ids, p.ServerID)
		}
	}
	if len(ids) == 0 {
		w.tourShoutLocked(d, host, tourLine(d.Lines.Empty, tourDefaults.Empty))
		r.phase, r.at = tourIdle, now
		return
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	r.vehicle, r.host = veh.ServerID, host.ServerID
	r.members = []tourMember{{id: veh.ServerID, home: [4]float32{veh.SpawnX, veh.SpawnY, veh.SpawnZ, veh.SpawnFacing}, vehicle: true}}
	seen := map[EntityID]bool{veh.ServerID: true}
	for _, name := range append([]string{d.Host}, d.Members...) {
		m := w.tourNPCLocked(d, name)
		if m == nil || seen[m.ServerID] {
			continue
		}
		seen[m.ServerID] = true
		r.members = append(r.members, tourMember{
			id:        m.ServerID,
			rel:       [3]float32{m.SpawnX - veh.SpawnX, m.SpawnY - veh.SpawnY, m.SpawnZ - veh.SpawnZ},
			relFacing: angleBetween(veh.SpawnFacing, m.SpawnFacing),
			home:      [4]float32{m.SpawnX, m.SpawnY, m.SpawnZ, m.SpawnFacing},
		})
	}
	drop := [4]float32{d.Drop[0], d.Drop[1], d.Drop[2], 0}
	if len(d.Drop) > 3 {
		drop[3] = d.Drop[3]
	}
	r.riders = map[EntityID]*tourRider{}
	for i, id := range ids {
		p := w.Players[id]
		rd := &tourRider{tour: d.key, slot: i, world: d.World, drop: drop, viewed: -1}
		if sess := w.findSessionByEntityIDLocked(id); sess != nil {
			rd.rides = sess.DecorLightsLayout() >= 2
		}
		r.riders[id] = rd
		p.tourRide = rd
	}
	r.phase, r.at, r.lastT = tourFlying, now, 0
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, w.Players[id].Name)
	}
	log.Printf("Tour %q: away with %d riders (%s), %d members, %.0f s", d.Name, len(ids), strings.Join(names, ", "), len(r.members), d.total)
	w.tourStepLocked(r, 0, now)
	for id, rd := range r.riders {
		sess := w.findSessionByEntityIDLocked(id)
		if sess == nil {
			continue
		}
		sendPlayerSpeed(sess, 0)
		if rd.rides {
			sess.Queue().AddWithSequence(rideOnMessage(veh.Name, w.deckSlot(d, rd.slot)))
		}
	}
	w.tourSayLocked(r, tourLine(d.Lines.Depart, tourDefaults.Depart))
}

// deckSlot is a rider's place on the deck: the deck's places in turn, a crowd
// standing a little apart on each.
func (w *World) deckSlot(d *tourDef, slot int) [3]float32 {
	s := d.Deck[slot%len(d.Deck)]
	if k := slot / len(d.Deck); k > 0 {
		s[0] += 0.25 * float32(k%3-1)
		s[2] += 0.3 * float32((k+1)%3-1)
	}
	return s
}

// tourPlace is where a member (rel, relFacing from the vehicle at home), or a
// rider (deck, the vehicle's frame), is t seconds into the flight.
func (d *tourDef) memberAt(t float64, m tourMember) (x, y, z, f float32) {
	vx, vy, vz, turn, slope := d.pose(t)
	c, s := float32(math.Cos(float64(turn))), float32(math.Sin(float64(turn)))
	rx := m.rel[0]*c + m.rel[2]*s
	rz := m.rel[2]*c - m.rel[0]*s
	// lifted along the climb: ahead of the vehicle higher on the way up
	h := d.home + turn
	along := rx*float32(math.Sin(float64(h))) + rz*float32(math.Cos(float64(h)))
	// never under the ground it stood on at home: a team ahead on the way
	// down to the sand stays on it
	return vx + rx, max(vy+m.rel[1]+along*slope, m.home[1]), vz + rz, m.home[3] + turn
}

// riderAt is a rider at deck place q (the vehicle's frame) t seconds in, the
// vehicle's own facing at home vf.
func (d *tourDef) riderAt(t float64, vf float32, q [3]float32) (x, y, z, f float32) {
	vx, vy, vz, turn, slope := d.pose(t)
	h := vf + turn
	sn, cs := float32(math.Sin(float64(h))), float32(math.Cos(float64(h)))
	rx := q[2]*sn + q[0]*cs
	rz := q[2]*cs - q[0]*sn
	th := d.home + turn
	along := rx*float32(math.Sin(float64(th))) + rz*float32(math.Cos(float64(th)))
	return vx + rx, vy + q[1] + along*slope, vz + rz, th
}

// tourStepLocked puts the members and riders where the flight has them t
// seconds in, with the velocity that carries them on, and says the lines and
// sets the PS2s down at the views it has come to. Must be called with the lock
// held.
func (w *World) tourStepLocked(r *tourRun, t float64, now time.Time) {
	d := r.def
	var vf float32
	for _, m := range r.members {
		npc := w.NPCs[m.id]
		if npc == nil || npc.Dead || npc.World != d.World {
			log.Printf("Tour %q: a member (%d) left the world: home", d.Name, m.id)
			w.tourLandLocked(r, now, "a member left the world")
			return
		}
		if m.vehicle {
			vf = m.home[3]
		}
		x, y, z, f := d.memberAt(t, m)
		x2, y2, z2, _ := d.memberAt(t+tourStep, m)
		oldX, oldZ := npc.X, npc.Z
		npc.X, npc.Y, npc.Z, npc.Facing = x, y, z, wrapHeading(f)
		npc.VelX, npc.VelY, npc.VelZ = (x2-x)/tourStep, (y2-y)/tourStep, (z2-z)/tourStep
		npc.Loco = LocoStand
		if !m.vehicle && npc.ServerID != r.host && (npc.VelX != 0 || npc.VelZ != 0) {
			npc.Loco = LocoRun // the team's stride in the air
		}
		if w.SpatialIndex != nil {
			w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, x, z)
		}
	}
	for id, rd := range r.riders {
		p := w.Players[id]
		if p == nil {
			delete(r.riders, id)
			continue
		}
		// moved by something else since (a gate, a GM, a respawn), another world, dead: off
		if t > 0 && (p.World != rd.world || p.IsDead() || distXZ(p.X, p.Z, rd.last[0], rd.last[2]) > 0.5 || math.Abs(float64(p.Y-rd.last[1])) > 0.5) {
			w.tourDropLocked(r, p, rd, false, "left the vehicle")
			continue
		}
		q := w.deckSlot(d, rd.slot)
		x, y, z, f := d.riderAt(t, vf, q)
		x2, y2, z2, _ := d.riderAt(t+tourStep, vf, q)
		oldX, oldZ := p.X, p.Z
		p.X, p.Y, p.Z, p.Facing = x, y, z, wrapHeading(f)
		p.VelX, p.VelY, p.VelZ = (x2-x)/tourStep, (y2-y)/tourStep, (z2-z)/tourStep
		p.WalkFlags, p.HasWalkFlags, p.Animation = 0, true, 0
		rd.last = [3]float32{x, y, z}
		if w.SpatialIndex != nil {
			w.SpatialIndex.Update(id, oldX, oldZ, x, z)
		}
	}
	// the lines at the points reached since the last step, and the views of
	// the legs begun
	for i := range d.Route {
		if d.arrive[i] > r.lastT && d.arrive[i] <= t && i > 0 && i < len(d.Route)-1 {
			if say := d.Route[i].Say; say != "" {
				w.tourSayLocked(r, say)
			}
		}
	}
	for i := 1; i < len(d.Route); i++ {
		v := d.Route[i].View
		if v == nil || !(d.leave[i-1] <= t && (d.leave[i-1] > r.lastT || t == 0)) {
			continue
		}
		face := headingTo(d.Route[i].X-v[0], d.Route[i].Z-v[2])
		for id, rd := range r.riders {
			if rd.rides || rd.viewed >= i {
				continue
			}
			rd.viewed = i
			if sess := w.findSessionByEntityIDLocked(id); sess != nil {
				sendTourTeleport(sess, d.World, v[0], v[1], v[2], face)
			}
		}
	}
	r.lastT = t
}

// wrapHeading is a heading within [0, 2pi).
func wrapHeading(f float32) float32 {
	r := math.Mod(float64(f), 2*math.Pi)
	if r < 0 {
		r += 2 * math.Pi
	}
	return float32(r)
}

// tourLandLocked ends a flight: the members home as their rows stand, each
// rider set down at the drop, the tour resting. Must be called with the lock
// held.
func (w *World) tourLandLocked(r *tourRun, now time.Time, why string) {
	d := r.def
	for _, m := range r.members {
		npc := w.NPCs[m.id]
		if npc == nil {
			continue
		}
		oldX, oldZ := npc.X, npc.Z
		npc.X, npc.Y, npc.Z, npc.Facing = m.home[0], m.home[1], m.home[2], m.home[3]
		npc.stopMoving()
		if w.SpatialIndex != nil {
			w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		}
	}
	if why == "landed" {
		w.tourSayLocked(r, tourLine(d.Lines.Land, tourDefaults.Land))
	}
	for id, rd := range r.riders {
		if p := w.Players[id]; p != nil {
			w.tourDropLocked(r, p, rd, true, why)
		}
	}
	log.Printf("Tour %q: home (%s)", d.Name, why)
	r.riders, r.members, r.joined = nil, nil, nil
	r.phase, r.at = tourResting, now
}

// tourDropLocked takes p off the vehicle: set down at the drop (put true: a
// landing's, or a logout's; a rider moved away by something else is left
// where that put it), its run speed its own again. Must be called with the
// lock held.
func (w *World) tourDropLocked(r *tourRun, p *Player, rd *tourRider, put bool, why string) {
	delete(r.riders, p.ServerID)
	p.tourRide = nil
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if put && p.World == rd.world && !p.IsDead() {
		x, y, z, f := w.dropPlaceLocked(rd)
		oldX, oldZ := p.X, p.Z
		p.X, p.Y, p.Z, p.Facing = x, y, z, f
		p.VelX, p.VelY, p.VelZ = 0, 0, 0
		p.WalkFlags, p.HasWalkFlags = walkFlagsOnFloor, true
		p.noteStood()
		if w.SpatialIndex != nil {
			w.SpatialIndex.Update(p.ServerID, oldX, oldZ, x, z)
		}
		if sess != nil {
			if rd.rides {
				sess.Queue().AddWithSequence(rideOffMessage(x, y, z, f))
			} else {
				sendTourTeleport(sess, p.World, x, y, z, f)
			}
		}
	} else if sess != nil && rd.rides {
		// let go where the other move put it
		sess.Queue().AddWithSequence(rideOffMessage(p.X, p.Y, p.Z, p.Facing))
	}
	if sess != nil {
		sendPlayerSpeed(sess, p.SpeedNow())
		if why != "landed" && why != "logout" {
			w.sendChatToSession(sess, tourLine(r.def.Lines.Off, tourDefaults.Off))
		}
	}
	log.Printf("Tour %q: %s off (%s)", r.def.Name, p.Name, why)
}

// dropPlaceLocked is where a rider is set down: the drop, a crowd a little
// apart round it, on the floor there. Must be called with the lock held.
func (w *World) dropPlaceLocked(rd *tourRider) (x, y, z, f float32) {
	x, y, z, f = rd.drop[0], rd.drop[1], rd.drop[2], rd.drop[3]
	if k := rd.slot; k > 0 {
		a := float64(k) * 2.4
		rad := float32(1.2 + 0.4*float64(k/6))
		x += rad * float32(math.Sin(a))
		z += rad * float32(math.Cos(a))
	}
	if w.ground != nil {
		if fl, ok := w.floorLocked(rd.world, x, y+1, z); ok && math.Abs(float64(fl-y)) < 3 {
			y = fl
		}
	}
	return x, y, z, f
}

// tourLeavingLocked takes a player logging out off its tour: its place the drop
// (SavePlace saved it there already). Must be called with the lock held.
func (w *World) tourLeavingLocked(p *Player) {
	rd := p.tourRide
	if rd == nil {
		return
	}
	if r := w.tours.runs[rd.tour]; r != nil && r.riders[p.ServerID] == rd {
		w.tourDropLocked(r, p, rd, true, "logout")
		return
	}
	p.tourRide = nil
}

// endToursLocked lands every tour flying at once (a keep restart's: before
// anything is written down) and stops every countdown. Must be called with the
// lock held.
func (w *World) endToursLocked(why string) {
	now := time.Now()
	for _, r := range w.tours.runs {
		switch r.phase {
		case tourFlying:
			w.tourLandLocked(r, now, why)
		case tourCountdown:
			r.phase, r.at = tourIdle, now
		}
	}
}

// pinTourLocked keeps a rider's vehicle, its members and the other riders in its
// view, first among its slots. Must be called with the lock held.
func (w *World) pinTourLocked(p *Player, cands []entityDist) []entityDist {
	rd := p.tourRide
	if rd == nil {
		return cands
	}
	r := w.tours.runs[rd.tour]
	if r == nil {
		return cands
	}
	for _, m := range r.members {
		cands = pinLocked(cands, w.NPCs[m.id], p)
	}
	for id := range r.riders {
		if id == p.ServerID || w.Players[id] == nil {
			continue
		}
		found := false
		for i := range cands {
			if cands[i].ID == id {
				cands[i].DistSq, found = -1, true
				break
			}
		}
		if !found {
			cands = append(cands, entityDist{ID: id, DistSq: -1})
		}
	}
	return cands
}

// TourSummary says what tours the world holds, for the log at the start.
func (w *World) TourSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.tours.defs) == 0 {
		return "tours: none (npctours of a season that is on)"
	}
	var parts []string
	for _, d := range w.tours.defs {
		parts = append(parts, fmt.Sprintf("%s (%s, %d points, %.0f s)", d.Name, d.Host, len(d.Route), d.total))
	}
	sort.Strings(parts)
	return "tours: " + strings.Join(parts, "; ")
}

// rideOnMessage is OpcodeRide on: the vehicle's name and the rider's place on it.
func rideOnMessage(vehicle string, at [3]float32) *protocol.Message {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeRide))
	b = append(b, 1, byte(min(len(vehicle), 255)))
	b = append(b, vehicle[:min(len(vehicle), 255)]...)
	for _, v := range at {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRide)
	msg.SetData(b)
	return msg
}

// rideOffMessage is OpcodeRide off: where the client sets its walker down.
func rideOffMessage(x, y, z, f float32) *protocol.Message {
	b := binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeRide))
	b = append(b, 0)
	for _, v := range []float32{x, y, z, f} {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRide)
	msg.SetData(b)
	return msg
}

// sendTourTeleport is the teleport (0x00B2) to a place in the player's own world
// facing f, as a GM's #goto sends it (token 0).
func sendTourTeleport(sess *session.Session, world uint8, x, y, z, f float32) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeTeleport)
	wr := buffer.NewWriter(msg.Span())
	wr.WriteUint16(uint16(protocol.OpcodeTeleport))
	wr.WriteByte(world)
	wr.WriteFloat32(x)
	wr.WriteFloat32(y)
	wr.WriteFloat32(z)
	wr.WriteFloat32(f)
	wr.WriteUint32(0)
	wr.WriteUint32(0)
	wr.WriteUint32(0)
	msg.SetSize(wr.Position())
	sess.Queue().AddWithSequence(msg)
}

// TourHost is the host's name of a tour ("" none).
func (w *World) TourHost(tour string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if d := w.tours.defs[strings.ToLower(strings.TrimSpace(tour))]; d != nil {
		return d.Host
	}
	return ""
}

// FirstTour is the name of a tour in force, the first by name ("" none).
func (w *World) FirstTour() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	first := ""
	for _, d := range w.tours.defs {
		if first == "" || d.Name < first {
			first = d.Name
		}
	}
	return first
}
