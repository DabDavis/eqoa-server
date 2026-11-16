package weather

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// The schedule (jibrish's numbers, #1549; no record of the live game's
// exists). A weather AREA is the squares of one climate in one 3x3 block of
// zones: (world, climate, floor(col/3), floor(row/3)), one automatic timeline.
// Its cycles are cycle_minutes long, offset by a hash of the area; cycle k's
// draws come from a generator seeded by hash(20261024, area, k), in this
// order: an event (event_chance), its length D (whole seconds in
// [event_min, event_max] minutes), its start S in [5 min, cycle - D - 5 min -
// 120 s], its strength by the weights, the wind's angle, and for heavy snow a
// whiteout (whiteout_chance, its hold in [whiteout_min, whiteout_max] s) when
// D > 8*fade and D > 2*hold + 4*fade. A zone meets the front late by its place
// along the wind: 60 s * clamp(round(1 + dot(zone - block centre, wind)), 0, 2).
//
// Nothing is saved: the same clock gives the same weather on any server.

const scheduleSeed = 20261024

// Phase is a point of a timeline: at At the weather is Kind at Strength (0-1);
// between two phases it eases from one to the other.
type Phase struct {
	At       time.Time
	Kind     Kind
	Strength float64
}

// Timeline is one weather's course over some of a zone's squares.
type Timeline struct {
	Climate      Climate // the climate's automatic weather; "" a GM's
	Mask         uint16  // the zone's squares it covers (bit (sz%4)*4 + (sx%4)); 0xFFFF a GM's
	Revision     uint32
	Seed         uint32
	WindX, WindZ float32
	Phases       []Phase
	Override     int    // a GM's weather's id; 0 automatic
	By           string // who called a GM's
}

// Start is its first phase's time.
func (t *Timeline) Start() time.Time { return t.Phases[0].At }

// End is its last phase's time (clear).
func (t *Timeline) End() time.Time { return t.Phases[len(t.Phases)-1].At }

// Weights is how strong each kind is at now: between phases a and b,
// t = smoothstep((now - a)/(b - a)), a's kind takes a.s*(1-t), b's b.s*t;
// clear's weight is dropped; outside the first..last phase nothing (as the
// vanilla client samples 0x7E94).
func (t *Timeline) Weights(now time.Time) (w [kinds]float64) {
	n := len(t.Phases)
	if n == 0 || now.Before(t.Phases[0].At) || now.After(t.Phases[n-1].At) {
		return
	}
	for i := 0; i+1 < n; i++ {
		a, b := t.Phases[i], t.Phases[i+1]
		if now.Before(a.At) || now.After(b.At) {
			continue
		}
		f := 1.0
		if span := b.At.Sub(a.At); span > 0 {
			f = float64(now.Sub(a.At)) / float64(span)
		}
		f = f * f * (3 - 2*f)
		w[a.Kind] += a.Strength * (1 - f)
		w[b.Kind] += b.Strength * f
		break
	}
	if n == 1 {
		w[t.Phases[0].Kind] = t.Phases[0].Strength
	}
	w[Clear] = 0
	return
}

// strongest is the kind of most weight and the weight; clear and 0 for none.
func strongest(w [kinds]float64) (Kind, float64) {
	k, s := Clear, 0.0
	for i := Rain; i < kinds; i++ {
		if w[i] > s {
			k, s = i, w[i]
		}
	}
	return k, s
}

// Showing is the kind that shows most at now, and how strongly.
func (t *Timeline) Showing(now time.Time) (Kind, float64) { return strongest(t.Weights(now)) }

// Target is the phase being faded to or held at now (the first after now,
// once the first has passed); false before the first and from the last on.
func (t *Timeline) Target(now time.Time) (Phase, bool) {
	if len(t.Phases) == 0 || now.Before(t.Phases[0].At) {
		return Phase{}, false
	}
	for _, p := range t.Phases[1:] {
		if p.At.After(now) {
			return p, true
		}
	}
	return Phase{}, false
}

// Byte is the retail weather byte at now: its target phase's (RetailByte),
// 0 before the first phase and from the last on.
func (t *Timeline) Byte(now time.Time) byte {
	if p, ok := t.Target(now); ok {
		return RetailByte(p.Kind, p.Strength)
	}
	return 0
}

// Override is a GM's weather as lines show it.
type Override struct {
	ID       int
	World    int
	Zones    []Zone
	Kind     Kind
	Strength float64
	From     time.Time
	Until    time.Time // its last zone's end (an ended one's fade's end)
	By       string
	Run      int // the trigger run that called it; 0 none
	Ended    bool
}

type override struct {
	id       int
	world    int
	zones    []Zone
	until    map[Zone]time.Time // each zone's end (clear)
	endedIn  map[Zone]bool      // the zones it was ended in (fading out)
	phases   map[Zone][]Phase
	kind     Kind
	strength float64
	from     time.Time
	fade     time.Duration
	by       string
	run      int
	gen      int
	ended    bool
	wx, wz   float32
}

func (o *override) last() time.Time {
	var t time.Time
	for _, u := range o.until {
		if u.After(t) {
			t = u
		}
	}
	return t
}

func (o *override) info() Override {
	return Override{ID: o.id, World: o.world, Zones: append([]Zone(nil), o.zones...), Kind: o.kind, Strength: o.strength,
		From: o.from, Until: o.last(), By: o.by, Run: o.run, Ended: o.ended}
}

// MaxOverrides is how many GM weathers are kept at once (a restart drops them).
const MaxOverrides = 256

// Engine is the weather: the grids, the GMs' rules and their overrides.
type Engine struct {
	mu        sync.Mutex
	grids     map[int]*Profiles
	rules     Rules
	overrides []*override
	seq       int
}

// NewEngine is an engine with no grids and no rules: no automatic weather.
func NewEngine() *Engine {
	return &Engine{grids: map[int]*Profiles{}, rules: Rules{Profiles: map[int]Climate{}, Zones: map[Zone]Climate{}, Climates: map[Climate]Rule{}}}
}

// SetGrids sets the worlds' profile grids (read once at the start).
func (e *Engine) SetGrids(g map[int]*Profiles) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.grids = map[int]*Profiles{}
	for w, p := range g {
		e.grids[w] = p
	}
}

// SetRules sets the three tables' word.
func (e *Engine) SetRules(r Rules) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.Profiles == nil {
		r.Profiles = map[int]Climate{}
	}
	if r.Zones == nil {
		r.Zones = map[Zone]Climate{}
	}
	if r.Climates == nil {
		r.Climates = map[Climate]Rule{}
	}
	e.rules = r
}

// Counts is how many grids and rows it holds.
func (e *Engine) Counts() (grids, profiles, zones, climates int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.grids), len(e.rules.Profiles), len(e.rules.Zones), len(e.rules.Climates)
}

// Worlds is the worlds with a grid, in order.
func (e *Engine) Worlds() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]int, 0, len(e.grids))
	for w := range e.grids {
		out = append(out, w)
	}
	sort.Ints(out)
	return out
}

// Square is a square's profile (false: no grid there) and climate: the
// weatherzones row of its zone if there is one, else weatherprofiles[its
// profile], else none.
func (e *Engine) Square(world, sx, sz int) (profile int, has bool, c Climate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.squareLocked(world, sx, sz)
}

func (e *Engine) squareLocked(world, sx, sz int) (int, bool, Climate) {
	prof, has := e.grids[world].At(sx, sz)
	if c, ok := e.rules.Zones[ZoneOfSquare(world, sx, sz)]; ok {
		return prof, has, c
	}
	if !has {
		return prof, false, None
	}
	if c, ok := e.rules.Profiles[prof]; ok {
		return prof, true, c
	}
	return prof, true, None
}

// zoneClimatesLocked is a zone's climates other than none, each with its
// squares' bits.
func (e *Engine) zoneClimatesLocked(z Zone) map[Climate]uint16 {
	out := map[Climate]uint16{}
	for j := 0; j < perZone; j++ {
		for i := 0; i < perZone; i++ {
			sx, sz := z.Col*perZone+i, z.Row*perZone+j
			if _, _, c := e.squareLocked(z.World, sx, sz); c != None {
				out[c] |= SquareBit(sx, sz)
			}
		}
	}
	return out
}

// ZoneClimates is a zone's climates other than none, each with its squares' bits.
func (e *Engine) ZoneClimates(z Zone) map[Climate]uint16 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.zoneClimatesLocked(z)
}

// hash64 is FNV-1a over the parts' words.
func hash64(parts ...any) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		fmt.Fprint(h, p)
		h.Write([]byte{0x1f})
	}
	return h.Sum64()
}

// rng is splitmix64: the same seed, the same draws, on any machine.
type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func (r *rng) float() float64 { return float64(r.next()>>11) / (1 << 53) }

// between is a whole number in [lo, hi].
func (r *rng) between(lo, hi int64) int64 {
	if hi <= lo {
		r.next()
		return lo
	}
	return lo + int64(r.next()%uint64(hi-lo+1))
}

// area is a weather area: one climate's squares in one 3x3 block of zones.
type area struct {
	world   int
	climate Climate
	bc, br  int
}

func areaOf(z Zone, c Climate) area { return area{z.World, c, idiv(z.Col, 3), idiv(z.Row, 3)} }

func (a area) key() string { return fmt.Sprintf("w%d/%s/b%d,%d", a.world, a.climate, a.bc, a.br) }

func floorDiv64(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// event is one cycle's draw for an area.
type event struct {
	k        int64
	start    int64 // unix seconds: the cycle's start + S, before a zone's lag
	dur      int64 // D, seconds
	fade     int64
	strength float64
	wx, wz   float64
	whiteout bool
	hold     int64
	seed     uint32
}

// offset is the area's cycles' offset in seconds.
func (a area) offset(cycle int64) int64 { return int64(hash64(a.key()) % uint64(cycle)) }

// cycleAt is the cycle of an area at unix second t.
func cycleAt(a area, r Rule, t int64) int64 {
	cycle := int64(r.CycleMinutes) * 60
	return floorDiv64(t-a.offset(cycle), cycle)
}

// draw is cycle k's event of an area; false for none.
func draw(a area, r Rule, k int64) (event, bool) {
	cycle := int64(r.CycleMinutes) * 60
	g := rng{hash64(scheduleSeed, a.key(), k)}
	has := g.float() < r.EventChance
	ev := event{k: k}
	ev.dur = g.between(int64(r.EventMin)*60, int64(r.EventMax)*60)
	s := g.between(300, cycle-ev.dur-300-120)
	ev.start = a.offset(cycle) + k*cycle + s
	total := r.WeightLight + r.WeightModerate + r.WeightHeavy
	u := int(g.between(0, int64(max(total, 1))-1))
	switch {
	case u < r.WeightLight:
		ev.strength = Light
	case u < r.WeightLight+r.WeightModerate:
		ev.strength = Moderate
	default:
		ev.strength = Heavy
	}
	angle := g.float() * 2 * math.Pi
	speed := 2 + 6*ev.strength
	ev.wx, ev.wz = speed*math.Cos(angle), speed*math.Sin(angle)
	ev.fade = min(int64(r.FadeSeconds), ev.dur/4)
	wu := g.float()
	ev.hold = g.between(int64(r.WhiteoutMin), int64(r.WhiteoutMax))
	ev.whiteout = r.Kind == Snow && ev.strength == Heavy && wu < r.WhiteoutChance &&
		ev.dur > 8*ev.fade && ev.dur > 2*ev.hold+4*ev.fade
	ev.seed = uint32(hash64(scheduleSeed, a.key(), k, "seed"))
	return ev, has
}

// lag is how late a zone meets an event's front: 60 s *
// clamp(round(1 + dot(zone - block centre, wind unit)), 0, 2).
func (ev event) lag(z Zone, a area) int64 {
	dc, dr := float64(z.Col-(3*a.bc+1)), float64(z.Row-(3*a.br+1))
	sp := math.Hypot(ev.wx, ev.wz)
	if sp == 0 {
		return 60
	}
	d := math.Round(1 + (dc*ev.wx+dr*ev.wz)/sp)
	return 60 * int64(max(0, min(2, d)))
}

// timeline is the event's course in a zone.
func (ev event) timeline(z Zone, c Climate, r Rule) Timeline {
	a := areaOf(z, c)
	s := time.Unix(ev.start+ev.lag(z, a), 0)
	d := time.Duration(ev.dur) * time.Second
	f := time.Duration(ev.fade) * time.Second
	e := s.Add(d)
	k := r.Kind
	ph := []Phase{{s, Clear, 0}, {s.Add(f), k, Light}, {s.Add(3 * f), k, ev.strength}}
	if ev.whiteout {
		mid := s.Add(d / 2)
		hold := time.Duration(ev.hold) * time.Second
		ph = append(ph, Phase{mid.Add(-f), Snow, 1}, Phase{mid, Whiteout, 1},
			Phase{mid.Add(hold), Whiteout, 1}, Phase{mid.Add(hold + f), Snow, 1})
	}
	ph = append(ph, Phase{e.Add(-f), k, ev.strength}, Phase{e, Clear, 0})
	return Timeline{Climate: c, Revision: uint32(hash64(z.ID(), c, ev.k, 0)), Seed: ev.seed,
		WindX: float32(ev.wx), WindZ: float32(ev.wz), Phases: ph}
}

// autoLocked is a zone's climate's automatic timeline at now: its event under
// way or the next, the first that starts at or after barrier when one is
// given (a GM's weather's end + its fade: one under way then is skipped).
func (e *Engine) autoLocked(z Zone, c Climate, now, barrier time.Time) (Timeline, bool) {
	r, ok := e.rules.Climates[c]
	if !ok || c == None {
		return Timeline{}, false
	}
	a := areaOf(z, c)
	from := now
	if barrier.After(from) {
		from = barrier
	}
	k := cycleAt(a, r, from.Unix())
	for i := int64(-1); i < 8; i++ { // a lagged event never leaves its cycle; a dry run of cycles (chance < 1) is looked past
		ev, has := draw(a, r, k+i)
		if !has {
			continue
		}
		tl := ev.timeline(z, c, r)
		if !tl.End().After(now) || !barrier.IsZero() && tl.Start().Before(barrier) {
			continue
		}
		return tl, true
	}
	return Timeline{}, false
}

// overrideForLocked is the newest GM weather a zone is under at now.
func (e *Engine) overrideForLocked(z Zone, now time.Time) *override {
	var best *override
	for _, o := range e.overrides {
		if u, in := o.until[z]; in && now.Before(u) && !now.Before(o.from) && (best == nil || o.id > best.id) {
			best = o
		}
	}
	return best
}

// barrierLocked is when a zone's automatic weather may begin again after the
// GM weathers it was under: the latest end + fade of those that are over.
func (e *Engine) barrierLocked(z Zone, now time.Time) time.Time {
	var b time.Time
	for _, o := range e.overrides {
		if u, in := o.until[z]; in && !now.Before(u) {
			if t := u.Add(o.fade); t.After(b) {
				b = t
			}
		}
	}
	return b
}

func (o *override) timeline(z Zone) Timeline {
	return Timeline{Mask: 0xFFFF, Revision: uint32(hash64(z.ID(), "", 0, o.id, o.gen)), Seed: uint32(hash64("override", o.id)),
		WindX: o.wx, WindZ: o.wz, Phases: o.phases[z], Override: o.id, By: o.by}
}

// zoneLocked is a zone's timelines at now: a GM's over the whole zone, or each
// of its climates' automatic one with the squares it covers.
func (e *Engine) zoneLocked(z Zone, now time.Time) []Timeline {
	if o := e.overrideForLocked(z, now); o != nil {
		return []Timeline{o.timeline(z)}
	}
	cl := e.zoneClimatesLocked(z)
	order := make([]Climate, 0, len(cl))
	for c := range cl {
		order = append(order, c)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	barrier := e.barrierLocked(z, now)
	var out []Timeline
	for _, c := range order {
		if tl, ok := e.autoLocked(z, c, now, barrier); ok {
			tl.Mask = cl[c]
			out = append(out, tl)
		}
	}
	return out
}

// Zone is a zone's timelines at now (0x7E94's).
func (e *Engine) Zone(z Zone, now time.Time) []Timeline {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.zoneLocked(z, now)
}

// squareLocked is the timeline over a square at now: its zone's GM weather,
// else its climate's automatic one; false for none (a none square, no rule).
func (e *Engine) squareTimelineLocked(world, sx, sz int, now time.Time) (Timeline, bool) {
	z := ZoneOfSquare(world, sx, sz)
	if o := e.overrideForLocked(z, now); o != nil {
		return o.timeline(z), true
	}
	_, _, c := e.squareLocked(world, sx, sz)
	if c == None {
		return Timeline{}, false
	}
	return e.autoLocked(z, c, now, e.barrierLocked(z, now))
}

// At is the timeline over the square of a place at now; false for none.
func (e *Engine) At(world int, x, z float32, now time.Time) (Timeline, bool) {
	sx, sz := SquareAt(x, z)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.squareTimelineLocked(world, sx, sz, now)
}

// showingLocked is the weather showing at a zone's centre at now: the four
// squares round it, their weights averaged, the strongest.
func (e *Engine) showingLocked(z Zone, now time.Time) (Kind, float64) {
	var w [kinds]float64
	for _, d := range [4][2]int{{1, 1}, {2, 1}, {1, 2}, {2, 2}} {
		if tl, ok := e.squareTimelineLocked(z.World, z.Col*perZone+d[0], z.Row*perZone+d[1], now); ok {
			sw := tl.Weights(now)
			for k := range w {
				w[k] += sw[k] / 4
			}
		}
	}
	return strongest(w)
}

// Showing is the weather showing at a zone's centre at now.
func (e *Engine) Showing(z Zone, now time.Time) (Kind, float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.showingLocked(z, now)
}

// Where says which zones round a place a GM's weather takes.
type Where struct {
	Kind string // "here" (the place's zone), "near" (within Near cols and rows), "world" (every zone of its world with weather of its own, and its own)
	Near int
}

// ParseWhere reads here, near N or world; "" is here.
func ParseWhere(kind string, near int) (Where, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "here":
		return Where{Kind: "here"}, nil
	case "near":
		if near < 1 || near > 5 {
			return Where{}, fmt.Errorf("near %d: 1 to 5 zones", near)
		}
		return Where{Kind: "near", Near: near}, nil
	case "world":
		return Where{Kind: "world"}, nil
	}
	return Where{}, fmt.Errorf("where %q: here, near or world", kind)
}

func (w Where) String() string {
	if w.Kind == "near" {
		return fmt.Sprintf("near %d", w.Near)
	}
	return w.Kind
}

// worldZonesLocked is every zone of a world with a square of a climate other
// than none (its grid's and its weatherzones rows').
func (e *Engine) worldZonesLocked(world int) []Zone {
	seen := map[Zone]bool{}
	e.grids[world].Squares(func(sx, sz, _ int) {
		z := ZoneOfSquare(world, sx, sz)
		if seen[z] {
			return
		}
		if _, _, c := e.squareLocked(world, sx, sz); c != None {
			seen[z] = true
		}
	})
	for z, c := range e.rules.Zones {
		if z.World == world && c != None {
			seen[z] = true
		}
	}
	out := make([]Zone, 0, len(seen))
	for z := range seen {
		out = append(out, z)
	}
	sortZones(out)
	return out
}

func sortZones(zs []Zone) {
	sort.Slice(zs, func(i, j int) bool {
		a, b := zs[i], zs[j]
		if a.World != b.World {
			return a.World < b.World
		}
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		return a.Col < b.Col
	})
}

// resolveLocked is the zones where takes round a place, its own first.
func (e *Engine) resolveLocked(world int, x, z float32, w Where) []Zone {
	own := ZoneAt(world, x, z)
	out := []Zone{own}
	switch w.Kind {
	case "near":
		for r := own.Row - w.Near; r <= own.Row+w.Near; r++ {
			for c := own.Col - w.Near; c <= own.Col+w.Near; c++ {
				if (c != own.Col || r != own.Row) && c >= 0 && r >= 0 {
					out = append(out, Zone{world, c, r})
				}
			}
		}
	case "world":
		for _, zz := range e.worldZonesLocked(world) {
			if zz != own {
				out = append(out, zz)
			}
		}
	}
	return out
}

// Resolve is the zones where takes round a place, its own first.
func (e *Engine) Resolve(world int, x, z float32, w Where) []Zone {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.resolveLocked(world, x, z, w)
}

// fadeOf is the fade a GM's weather of a kind takes (its climate's rule; 30 s).
func (e *Engine) fadeOf(k Kind) time.Duration {
	if r, ok := e.rules.Climates[ClimateOfKind(k)]; ok {
		return time.Duration(r.FadeSeconds) * time.Second
	}
	return 30 * time.Second
}

// keepLocked is how long an ended GM weather is kept: the longest cycle and a
// fade, so the automatic weather it held off cannot come back mid-event.
func (e *Engine) keepLocked() time.Duration {
	most := 40
	for _, r := range e.rules.Climates {
		most = max(most, r.CycleMinutes)
	}
	return time.Duration(most)*time.Minute + time.Minute
}

func (e *Engine) pruneLocked(now time.Time) {
	keep := e.keepLocked()
	out := e.overrides[:0]
	for _, o := range e.overrides {
		if now.Before(o.last().Add(keep)) {
			out = append(out, o)
		}
	}
	for i := len(out); i < len(e.overrides); i++ {
		e.overrides[i] = nil
	}
	e.overrides = out
}

// Call is a GM's weather: kind at strength (0-1; clear holds the sky clear)
// for minutes (2-120) on the zones where takes round a place, from now. Each
// zone eases from what shows there now to it over the kind's fade, holds it,
// and eases to clear by its end. by is who called it, run the trigger run
// (0 none).
func (e *Engine) Call(world int, x, z float32, w Where, k Kind, strength float64, minutes int, by string, run int, now time.Time) (Override, error) {
	if k >= kinds {
		return Override{}, fmt.Errorf("no weather kind %d", k)
	}
	if !(strength >= 0 && strength <= 1) {
		return Override{}, fmt.Errorf("strength %g: 0 to 1", strength)
	}
	if minutes < 2 || minutes > 120 {
		return Override{}, fmt.Errorf("%d minutes: 2 to 120", minutes)
	}
	if world < 0 || world > 255 {
		return Override{}, fmt.Errorf("world %d", world)
	}
	if k == Clear {
		strength = 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pruneLocked(now)
	if len(e.overrides) >= MaxOverrides {
		// the oldest that is over goes first; all in force: refused
		drop := -1
		for i, o := range e.overrides {
			if !now.Before(o.last()) {
				drop = i
				break
			}
		}
		if drop < 0 {
			return Override{}, fmt.Errorf("%d GM weathers are in force: end one first", len(e.overrides))
		}
		e.overrides = append(e.overrides[:drop], e.overrides[drop+1:]...)
	}
	zones := e.resolveLocked(world, x, z, w)
	e.seq++
	fade := e.fadeOf(k)
	until := now.Add(time.Duration(minutes) * time.Minute)
	angle := float64(hash64("wind", e.seq)%3600) / 3600 * 2 * math.Pi
	speed := 2 + 6*strength
	o := &override{id: e.seq, world: world, zones: zones, until: map[Zone]time.Time{}, endedIn: map[Zone]bool{}, phases: map[Zone][]Phase{},
		kind: k, strength: strength, from: now, fade: fade, by: by, run: run,
		wx: float32(speed * math.Cos(angle)), wz: float32(speed * math.Sin(angle))}
	for _, zz := range zones {
		sk, ss := e.showingLocked(zz, now)
		o.phases[zz] = []Phase{{now, sk, ss}, {now.Add(fade), k, strength}, {until.Add(-fade), k, strength}, {until, Clear, 0}}
		o.until[zz] = until
	}
	e.overrides = append(e.overrides, o)
	return o.info(), nil
}

// endZoneLocked ends o in a zone at now: from what shows to clear over its
// fade. False when it was over there already.
func (e *Engine) endZoneLocked(o *override, z Zone, now time.Time) bool {
	u, in := o.until[z]
	if !in || !now.Before(u) || o.endedIn[z] {
		return false
	}
	tl := o.timeline(z)
	sk, ss := tl.Showing(now)
	o.phases[z] = []Phase{{now, sk, ss}, {now.Add(o.fade), Clear, 0}}
	o.until[z] = now.Add(o.fade)
	o.gen++
	o.ended = true
	o.endedIn[z] = true
	return true
}

// End ends the GM weathers on the zones where takes round a place: each eases
// to clear over its fade, then the automatic weather comes back with its
// first event that starts after that. The zones ended.
func (e *Engine) End(world int, x, z float32, w Where, now time.Time) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, zz := range e.resolveLocked(world, x, z, w) {
		ended := false
		for _, o := range e.overrides {
			if e.endZoneLocked(o, zz, now) {
				ended = true
			}
		}
		if ended {
			n++
		}
	}
	return n
}

// EndID ends one GM weather wherever it is in force. False when there is none
// of that id in force.
func (e *Engine) EndID(id int, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	any := false
	for _, o := range e.overrides {
		if o.id != id {
			continue
		}
		for _, zz := range o.zones {
			if e.endZoneLocked(o, zz, now) {
				any = true
			}
		}
	}
	return any
}

// EndRun ends the GM weathers a trigger run called. The zones ended.
func (e *Engine) EndRun(run int, now time.Time) int {
	if run == 0 {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, o := range e.overrides {
		if o.run != run {
			continue
		}
		for _, zz := range o.zones {
			if e.endZoneLocked(o, zz, now) {
				n++
			}
		}
	}
	return n
}

// Overrides is every GM weather in force at now (in some zone), oldest first.
func (e *Engine) Overrides(now time.Time) []Override {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Override
	for _, o := range e.overrides {
		if now.Before(o.last()) {
			out = append(out, o.info())
		}
	}
	return out
}

// Next is the next automatic event of a square that starts after now: its
// timeline; false for none (a none square, no rule).
func (e *Engine) Next(world, sx, sz int, now time.Time) (Timeline, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _, c := e.squareLocked(world, sx, sz)
	if c == None {
		return Timeline{}, false
	}
	z := ZoneOfSquare(world, sx, sz)
	b := e.barrierLocked(z, now)
	if b.Before(now) {
		b = now
	}
	return e.autoLocked(z, c, now, b)
}

// ZoneNow is a zone with weather now, as the panel shows it.
type ZoneNow struct {
	Zone     Zone
	Climates map[Climate]uint16
	Kind     Kind
	Strength float64
	Override int
	By       string
	Until    time.Time
}

// WeatherNow is every zone of a world with weather showing at now: its
// timelines' strongest.
func (e *Engine) WeatherNow(world int, now time.Time) []ZoneNow {
	e.mu.Lock()
	defer e.mu.Unlock()
	set := map[Zone]bool{}
	for _, z := range e.worldZonesLocked(world) {
		set[z] = true
	}
	for _, o := range e.overrides {
		if o.world == world {
			for _, z := range o.zones {
				set[z] = true
			}
		}
	}
	zs := make([]Zone, 0, len(set))
	for z := range set {
		zs = append(zs, z)
	}
	sortZones(zs)
	var out []ZoneNow
	for _, z := range zs {
		var best ZoneNow
		for _, tl := range e.zoneLocked(z, now) {
			if k, s := tl.Showing(now); s > best.Strength {
				best = ZoneNow{Kind: k, Strength: s, Override: tl.Override, By: tl.By, Until: tl.End()}
			}
		}
		if best.Strength > 0 {
			best.Zone, best.Climates = z, e.zoneClimatesLocked(z)
			out = append(out, best)
		}
	}
	return out
}

// Rules is a copy of the tables' word it holds.
func (e *Engine) Rules() Rules {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := Rules{Profiles: map[int]Climate{}, Zones: map[Zone]Climate{}, Climates: map[Climate]Rule{}}
	for k, v := range e.rules.Profiles {
		r.Profiles[k] = v
	}
	for k, v := range e.rules.Zones {
		r.Zones[k] = v
	}
	for k, v := range e.rules.Climates {
		r.Climates[k] = v
	}
	return r
}
