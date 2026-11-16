package weather

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// seedRules is tools/seed_weather.py's rows.
func seedRules() Rules {
	r := Rules{Profiles: map[int]Climate{}, Zones: map[Zone]Climate{}, Climates: map[Climate]Rule{}}
	for p := 0; p < 22; p++ {
		r.Profiles[p] = Temperate
	}
	r.Profiles[2], r.Profiles[10] = Arctic, Arctic
	r.Profiles[3], r.Profiles[11] = Desert, Desert
	r.Profiles[19] = None
	for c := 0; c <= 3; c++ {
		for row := 0; row <= 2; row++ {
			r.Zones[Zone{3, c, row}] = None
		}
	}
	r.Zones[Zone{5, 2, 3}] = None
	base := Rule{CycleMinutes: 40, EventChance: 1, WeightLight: 60, WeightModerate: 30, WeightHeavy: 10, WhiteoutMin: 30, WhiteoutMax: 60, FadeSeconds: 30}
	t := base
	t.Climate, t.Kind, t.EventMin, t.EventMax = Temperate, Rain, 6, 12
	a := base
	a.Climate, a.Kind, a.EventMin, a.EventMax, a.WhiteoutChance = Arctic, Snow, 6, 12, 0.5
	d := base
	d.Climate, d.Kind, d.EventMin, d.EventMax = Desert, Sand, 3, 6
	r.Climates[Temperate], r.Climates[Arctic], r.Climates[Desert] = t, a, d
	return r
}

// testGrid is a 2x2-zone world: zone (0,0) temperate (profile 0), zone (1,0)
// half snow (2) half sand (3), zone (0,1) lava (19), zone (1,1) temperate.
func testGrid() *Profiles {
	p := &Profiles{Rows: 2, Cols: 2, Subdiv: 4, Cell: 2000, OriginX: 1000, OriginZ: 1000, Data: make([]byte, 64)}
	for j := 0; j < 8; j++ {
		for i := 0; i < 8; i++ {
			v := byte(0)
			switch {
			case i >= 4 && j < 4 && i < 6:
				v = 2
			case i >= 6 && j < 4:
				v = 3
			case i < 4 && j >= 4:
				v = 19
			}
			p.Data[j*8+i] = v
		}
	}
	return p
}

func testEngine() *Engine {
	e := NewEngine()
	e.SetGrids(map[int]*Profiles{0: testGrid()})
	e.SetRules(seedRules())
	return e
}

var t0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func TestRulesSeedCheck(t *testing.T) {
	for c, r := range seedRules().Climates {
		if err := r.Check(); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
	bad := seedRules().Climates[Temperate]
	bad.EventMax = 30 // 30*60+720 > 40*60
	if bad.Check() == nil {
		t.Error("an event too long for its cycle passed")
	}
	bad = seedRules().Climates[Temperate]
	bad.Kind = Snow
	if bad.Check() == nil {
		t.Error("snow for temperate passed")
	}
	bad = seedRules().Climates[Temperate]
	bad.WeightLight, bad.WeightModerate, bad.WeightHeavy = 0, 0, 0
	if bad.Check() == nil {
		t.Error("all-zero weights passed")
	}
	if CheckProfileRow(256, "arctic") == nil || CheckProfileRow(2, "tundra") == nil || CheckProfileRow(2, "arctic") != nil {
		t.Error("profile row checks")
	}
	if CheckZoneRow(30, 0, 0, "none") == nil || CheckZoneRow(3, 0, 0, "none") != nil {
		t.Error("zone row checks")
	}
}

func TestProfilesRoundTrip(t *testing.T) {
	p := testGrid()
	var b bytes.Buffer
	if err := WriteProfiles(&b, p); err != nil {
		t.Fatal(err)
	}
	q, err := ReadProfiles(&b)
	if err != nil {
		t.Fatal(err)
	}
	if q.Rows != 2 || q.Cols != 2 || q.Subdiv != 4 || q.Cell != 2000 || !bytes.Equal(q.Data, p.Data) {
		t.Fatalf("round trip: %+v", q)
	}
	for _, c := range []struct{ sx, sz, want int }{{0, 0, 0}, {4, 0, 2}, {6, 3, 3}, {7, 3, 3}, {0, 4, 19}, {7, 7, 0}} {
		if got, ok := q.At(c.sx, c.sz); !ok || got != c.want {
			t.Errorf("square (%d, %d): %d %v, want %d", c.sx, c.sz, got, ok, c.want)
		}
	}
	if _, ok := q.At(8, 0); ok {
		t.Error("off the grid read")
	}
}

func TestClimates(t *testing.T) {
	e := testEngine()
	for _, c := range []struct {
		world, sx, sz int
		want          Climate
	}{{0, 0, 0, Temperate}, {0, 4, 0, Arctic}, {0, 6, 0, Desert}, {0, 0, 4, None}, {1, 0, 0, None}} {
		if _, _, got := e.Square(c.world, c.sx, c.sz); got != c.want {
			t.Errorf("w%d (%d, %d): %s, want %s", c.world, c.sx, c.sz, got, c.want)
		}
	}
	r := seedRules()
	r.Zones[Zone{0, 0, 0}] = Desert // a GM's word for the zone, above the profiles
	r.Zones[Zone{1, 0, 0}] = Arctic // and where there is no profile file
	e.SetRules(r)
	if _, _, c := e.Square(0, 1, 1); c != Desert {
		t.Errorf("weatherzones over profiles: %s", c)
	}
	if _, _, c := e.Square(1, 2, 2); c != Arctic {
		t.Errorf("weatherzones without a grid: %s", c)
	}
	cl := e.ZoneClimates(Zone{0, 1, 0})
	if cl[Arctic] != 0x3333 || cl[Desert] != 0xCCCC || len(cl) != 2 {
		t.Errorf("zone (1,0) climates: %x", cl)
	}
}

func TestDeterministic(t *testing.T) {
	a, b := testEngine(), testEngine()
	for i := 0; i < 50; i++ {
		now := t0.Add(time.Duration(i) * 17 * time.Minute)
		ta, tb := a.Zone(Zone{0, 1, 0}, now), b.Zone(Zone{0, 1, 0}, now)
		if len(ta) != len(tb) {
			t.Fatalf("lengths differ")
		}
		for j := range ta {
			if ta[j].Revision != tb[j].Revision || len(ta[j].Phases) != len(tb[j].Phases) || !ta[j].Start().Equal(tb[j].Start()) {
				t.Fatalf("timelines differ at %v", now)
			}
		}
	}
}

// TestSchedule simulates 72 h over 540 areas of each climate.
func TestSchedule(t *testing.T) {
	r := seedRules()
	from, to := t0.Unix(), t0.Add(72*time.Hour).Unix()
	for _, c := range []Climate{Temperate, Arctic, Desert} {
		rule := r.Climates[c]
		var wet, events, heavy, whiteouts int64
		minGap := int64(math.MaxInt64)
		for n := 0; n < 540; n++ {
			a := area{world: n % 6, climate: c, bc: n / 6 % 15, br: n / 90}
			var lastEnd int64 = -1
			for k := cycleAt(a, rule, from) - 1; k <= cycleAt(a, rule, to)+1; k++ {
				ev, has := draw(a, rule, k)
				if !has {
					continue
				}
				z := Zone{a.world, 3*a.bc + 1, 3*a.br + 1}
				tl := ev.timeline(z, c, rule)
				s, e := tl.Start().Unix(), tl.End().Unix()
				for i := 1; i < len(tl.Phases); i++ {
					if tl.Phases[i].At.Before(tl.Phases[i-1].At) {
						t.Fatalf("%s: phases out of order: %+v", c, tl.Phases)
					}
				}
				if lastEnd >= 0 {
					if s < lastEnd {
						t.Fatalf("%s: two events of an area overlap", c)
					}
					minGap = min(minGap, s-lastEnd)
				}
				lastEnd = e
				if ev.whiteout {
					whiteouts++
					if c != Arctic || ev.strength != Heavy {
						t.Fatalf("a whiteout in %s at %g", c, ev.strength)
					}
					// inside the event
					for _, p := range tl.Phases {
						if p.Kind == Whiteout && (p.At.Unix() <= s || p.At.Unix() >= e) {
							t.Fatalf("a whiteout phase outside its event")
						}
					}
				}
				if e <= from || s >= to {
					continue
				}
				events++
				if ev.strength == Heavy {
					heavy++
				}
				wet += min(e, to) - max(s, from)
			}
		}
		share := float64(wet) / float64(540*(to-from))
		t.Logf("%s: wet %.1f%%, %d events, heavy %.1f%%, whiteouts %.1f%%, least gap %d min", c, 100*share, events,
			100*float64(heavy)/float64(events), 100*float64(whiteouts)/float64(events), minGap/60)
		lo, hi := 0.20, 0.26
		if c == Desert {
			lo, hi = 0.09, 0.14
		}
		if share < lo || share > hi {
			t.Errorf("%s wet %.3f, want %.2f-%.2f", c, share, lo, hi)
		}
		if minGap < 10*60 {
			t.Errorf("%s: a gap of %d s", c, minGap)
		}
		if c != Arctic && whiteouts > 0 {
			t.Errorf("%s had whiteouts", c)
		}
	}
}

func TestLag(t *testing.T) {
	r := seedRules().Climates[Temperate]
	for n := 0; n < 200; n++ {
		a := area{0, Temperate, n % 7, n / 7}
		ev, _ := draw(a, r, int64(n))
		for dr := 0; dr < 3; dr++ {
			for dc := 0; dc < 3; dc++ {
				l := ev.lag(Zone{0, 3*a.bc + dc, 3*a.br + dr}, a)
				if l < 0 || l > 120 || l%60 != 0 {
					t.Fatalf("lag %d", l)
				}
			}
		}
		if l := ev.lag(Zone{0, 3*a.bc + 1, 3*a.br + 1}, a); l != 60 {
			t.Fatalf("the block centre's lag %d", l)
		}
	}
}

// eventAt finds a time inside an automatic event of a zone's climate.
func eventAt(t *testing.T, e *Engine, z Zone, c Climate) Timeline {
	t.Helper()
	for i := 0; i < 200; i++ {
		now := t0.Add(time.Duration(i) * time.Minute)
		for _, tl := range e.Zone(z, now) {
			if tl.Climate == c && !now.Before(tl.Start()) {
				return tl
			}
		}
	}
	t.Fatalf("no %s event in %s", c, z.ID())
	return Timeline{}
}

func TestRetailBytes(t *testing.T) {
	tl := Timeline{Phases: []Phase{{t0, Clear, 0}, {t0.Add(30 * time.Second), Snow, Light}, {t0.Add(90 * time.Second), Snow, 1},
		{t0.Add(10 * time.Minute), Snow, 1}, {t0.Add(11 * time.Minute), Clear, 0}}}
	var seq []byte
	last := byte(255)
	for s := -60; s < 800; s++ {
		b := tl.Byte(t0.Add(time.Duration(s) * time.Second))
		if b != last {
			seq = append(seq, b)
			last = b
		}
	}
	if !bytes.Equal(seq, []byte{0, 4, 6, 0}) {
		t.Errorf("bytes %v, want 0 4 6 0", seq)
	}
	for _, c := range []struct {
		k    Kind
		s    float64
		want byte
	}{{Rain, 0.005, 0}, {Rain, 0.15, 1}, {Rain, 0.5, 2}, {Rain, 1, 3}, {Snow, 0.15, 4}, {Whiteout, 1, 6}, {Sand, 1, 0}, {Clear, 1, 0}} {
		if got := RetailByte(c.k, c.s); got != c.want {
			t.Errorf("RetailByte(%s, %g) = %d, want %d", c.k, c.s, got, c.want)
		}
	}
}

func TestOverride(t *testing.T) {
	e := testEngine()
	lava := Zone{0, 0, 1}
	if tl := e.Zone(lava, t0); len(tl) != 0 {
		t.Fatalf("lava has weather: %+v", tl)
	}
	if _, ok := e.At(0, 100, 2100, t0); ok {
		t.Fatal("a lava square has a timeline")
	}
	o, err := e.Call(0, 100, 2100, Where{Kind: "here"}, Snow, Heavy, 10, "Dab", 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Zones) != 1 || o.Zones[0] != lava || !o.Until.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("override %+v", o)
	}
	tl, ok := e.At(0, 100, 2100, t0.Add(5*time.Minute))
	if !ok || tl.Mask != 0xFFFF || tl.Override != o.ID || tl.Byte(t0.Add(5*time.Minute)) != 6 {
		t.Fatalf("lava under the override: %+v %v", tl, ok)
	}
	// it fades from what showed (clear) at its start
	if p := tl.Phases[0]; p.Kind != Clear || !p.At.Equal(t0) {
		t.Errorf("first phase %+v", p)
	}
	if _, ok := e.At(0, 100, 2100, t0.Add(11*time.Minute)); ok {
		t.Error("the lava kept its override after its end")
	}
	if len(e.Overrides(t0.Add(time.Minute))) != 1 || len(e.Overrides(t0.Add(11*time.Minute))) != 0 {
		t.Error("Overrides in force")
	}
}

func TestOverrideFadesFromWhatShows(t *testing.T) {
	e := testEngine()
	z := Zone{0, 0, 0}
	ev := eventAt(t, e, z, Temperate)
	mid := ev.Start().Add(ev.End().Sub(ev.Start()) / 2)
	k, s := e.Showing(z, mid)
	if k != Rain || s <= 0 {
		t.Fatalf("showing mid-event %s %g", k, s)
	}
	o, err := e.Call(0, 1000, 1000, Where{Kind: "here"}, Snow, Heavy, 5, "Dab", 0, mid)
	if err != nil {
		t.Fatal(err)
	}
	tl := e.Zone(z, mid)[0]
	if tl.Phases[0].Kind != Rain || math.Abs(tl.Phases[0].Strength-s) > 1e-9 {
		t.Errorf("starts at %+v, want rain %g", tl.Phases[0], s)
	}
	// automatic weather comes back with an event that STARTS after the end + fade
	end := o.Until
	for m := 0; m < 120; m++ {
		now := end.Add(time.Duration(m) * time.Minute)
		for _, tl := range e.Zone(z, now) {
			if tl.Start().Before(end.Add(30 * time.Second)) {
				t.Fatalf("an automatic event starting %v came back after the override's end %v", tl.Start(), end)
			}
		}
	}
}

func TestEndOverride(t *testing.T) {
	e := testEngine()
	o, _ := e.Call(0, 1000, 1000, Where{Kind: "near", Near: 1}, Sand, Heavy, 30, "Dab", 7, t0)
	if len(o.Zones) != 4 { // (0,0) and its in-grid neighbours (1,0), (0,1), (1,1)
		t.Fatalf("near 1: %d zones", len(o.Zones))
	}
	now := t0.Add(5 * time.Minute)
	if n := e.End(0, 1000, 1000, Where{Kind: "here"}, now); n != 1 {
		t.Fatalf("end here: %d", n)
	}
	tl := e.Zone(Zone{0, 0, 0}, now.Add(10*time.Second))
	if len(tl) != 1 || tl[0].Override != o.ID || tl[0].Phases[len(tl[0].Phases)-1].Kind != Clear || !tl[0].End().Equal(now.Add(30*time.Second)) {
		t.Fatalf("ending fades to clear over the fade: %+v", tl)
	}
	if tl := e.Zone(Zone{0, 1, 1}, now); len(tl) != 1 || tl[0].Override != o.ID {
		t.Fatal("the other zones kept it")
	}
	if n := e.EndRun(7, now); n != 3 {
		t.Fatalf("EndRun: %d zones", n)
	}
	if e.EndID(o.ID, now.Add(time.Hour)) {
		t.Error("ended an override that was over")
	}
}

func TestWhere(t *testing.T) {
	e := testEngine()
	if zs := e.Resolve(0, 100, 100, Where{Kind: "here"}); len(zs) != 1 || zs[0] != (Zone{0, 0, 0}) {
		t.Errorf("here: %v", zs)
	}
	if zs := e.Resolve(0, 2100, 2100, Where{Kind: "near", Near: 2}); len(zs) != 16 { // cols and rows -1..3 less the negatives: 4x4
		t.Errorf("near 2 at (1,1): %d zones", len(zs))
	}
	// world: the zones with a climate other than none (lava (0,1) left out), and its own
	zs := e.Resolve(0, 100, 100, Where{Kind: "world"})
	if len(zs) != 3 {
		t.Errorf("world: %v", zs)
	}
	zs = e.Resolve(0, 100, 2100, Where{Kind: "world"})
	if len(zs) != 4 || zs[0] != (Zone{0, 0, 1}) {
		t.Errorf("world from lava: %v", zs)
	}
	if _, err := ParseWhere("near", 6); err == nil {
		t.Error("near 6 passed")
	}
}

func TestEncode(t *testing.T) {
	e := testEngine()
	// a whiteout timeline, the longest: 10 phases
	var tls []Timeline
	for i := 0; i < 3; i++ {
		tls = append(tls, Timeline{Mask: 0x000F << (4 * i), Revision: 9, Seed: 3, WindX: 1, WindZ: -2,
			Phases: make([]Phase, 10)})
	}
	for i := range tls {
		for j := range tls[i].Phases {
			tls[i].Phases[j] = Phase{t0.Add(time.Duration(j) * time.Minute), Snow, 1}
		}
		tls[i].Phases[9].Kind, tls[i].Phases[9].Strength = Clear, 0
	}
	var zs []ZoneData
	for i := 0; i < 5; i++ {
		zs = append(zs, ZoneData{Zone{0, i, 0}, tls})
	}
	b := EncodeData(zs, t0.Add(90*time.Second))
	if len(b) > 1300 {
		t.Fatalf("%d bytes", len(b))
	}
	if b[0] != 4 || b[1] != 0 || binary.LittleEndian.Uint16(b[2:]) != 0 || b[6] != 3 {
		t.Fatalf("head % x", b[:8])
	}
	p := 7
	if binary.LittleEndian.Uint16(b[p:]) != 0x000F || b[p+18] != 10 {
		t.Fatalf("timeline head % x", b[p:p+19])
	}
	at := int32(binary.LittleEndian.Uint32(b[p+19:]))
	if at != -90000 || b[p+23] != byte(Snow) || b[p+24] != 255 {
		t.Fatalf("first phase %d %d %d", at, b[p+23], b[p+24])
	}
	last := p + 19 + 9*6
	if b[last+4] != byte(Clear) {
		t.Fatal("the last phase is not clear")
	}
	// real timelines: every zone's phases end clear and are at most 12
	for i := 0; i < 100; i++ {
		now := t0.Add(time.Duration(i) * 7 * time.Minute)
		for _, z := range []Zone{{0, 0, 0}, {0, 1, 0}, {0, 1, 1}} {
			for _, tl := range e.Zone(z, now) {
				if n := len(tl.Phases); n < 2 || n > MaxPhases || tl.Phases[n-1].Kind != Clear {
					t.Fatalf("%s: %d phases", z.ID(), n)
				}
			}
		}
	}
	if near := ZonesNear(0, 1950, 100); len(near) != 2 || near[1] != (Zone{0, 1, 0}) {
		t.Errorf("zones near the edge: %v", near)
	}
	if near := ZonesNear(0, 1000, 1000); len(near) != 1 {
		t.Errorf("zones near the middle: %v", near)
	}
}
