package entity

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// ctfRig is a world with capture the flag on: a herald in world 0, n players signed up from beside it, a test arena
// in world 11 (two stands 800 apart, two power-ups, a square field), every number its default but one a side to
// start.
type ctfRig struct {
	w      *World
	ps     []*Player
	ss     []*session.Session
	t0     time.Time
	herald *NPC
}

func ctfTestLayout() CTFLayout {
	return CTFLayout{
		Name: "the Test Canyon",
		Teams: [2]CTFTeamLayout{
			{Name: "the Ember Band", Short: "Ember", Flag: "Ember Banner", Stand: CTFSpot{X: 100, Z: 500},
				Camp: CTFSpot{X: 100, Z: 560}, Body: 0xF97FD9FC, Size: 0.6},
			{Name: "the Dusk Band", Short: "Dusk", Flag: "Dusk Banner", Stand: CTFSpot{X: 900, Z: 500},
				Camp: CTFSpot{X: 900, Z: 560}, Body: 0x4446A60B, Size: 0.6},
		},
		PowerUps: []CTFPowerUpLayout{
			{Kind: CTFSpeed, Name: "Wind Shrine", At: CTFSpot{X: 500, Z: 300}},
			{Kind: CTFHeal, Name: "Mending Spring", At: CTFSpot{X: 500, Z: 700}},
		},
		Field:     [][2]float32{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}},
		SpeedBody: 0x8143DC70, HealBody: 0xC5F3788D,
	}
}

func ctfTestSettings() CTFSettings {
	set := DefaultCTFSettings()
	set.World = 11
	set.MinPerTeam = 1
	return set
}

func newCTF(t *testing.T, n int) *ctfRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetCTF(true, ctfTestSettings(), ctfTestLayout())
	r := &ctfRig{w: w, t0: time.Unix(1_800_000_000, 0)}
	r.herald = &NPC{ServerID: 900001, Name: CTFHeraldName, X: 5000, Y: 50, Z: 5000, World: 0, NPCType: 0x80, Level: 60, HP: 1, MaxHP: 1}
	w.mu.Lock()
	w.putNPCLocked(r.herald)
	w.SpatialIndex.Insert(r.herald.ServerID, r.herald.X, r.herald.Z)
	w.mu.Unlock()
	for i := 0; i < n; i++ {
		p := &Player{ServerID: EntityID(600000001 + i), Name: fmt.Sprintf("Fighter%d", i), Level: 30 + i, HP: 1000, MaxHP: 1000,
			Power: 500, MaxPower: 500, Speed: 8.833333, X: 5000 + float32(i), Y: 50, Z: 5010, World: 0, CharServerID: uint32(700 + i)}
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 1, 0, byte(i+1)), Port: 7100 + i}, uint32(100+i), uint32(100+i), 1, 2, true)
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		s.SetPlayerObjectID(uint32(0x15000000 + i))
		s.SetServerWorlds([]uint8{11}) // its game built the arena and said so (0x7E31's answer)
		r.ps = append(r.ps, p)
		r.ss = append(r.ss, s)
	}
	return r
}

func (r *ctfRig) at(x time.Duration) time.Time { return r.t0.Add(x) }

// sessOf is p's session.
func (r *ctfRig) sessOf(p *Player) *session.Session {
	for i, q := range r.ps {
		if q == p {
			return r.ss[i]
		}
	}
	return nil
}

// tick runs capture the flag's tick at x.
func (r *ctfRig) tick(x time.Duration) {
	r.w.mu.Lock()
	r.w.ctfSystem(r.at(x))
	r.w.mu.Unlock()
}

// arrive lands every queued travel as the travel hook would: the player put at its spot (ring places ignored).
func (r *ctfRig) arrive() []GateTravel {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	due := r.w.gates.due
	r.w.gates.due = nil
	for _, t := range due {
		t.P.World, t.P.X, t.P.Y, t.P.Z = t.To.World, t.To.X, t.To.Y, t.To.Z
	}
	return due
}

// step moves p to (x, z) at time x as a report of its own place does (the field and the touches).
func (r *ctfRig) step(p *Player, x, z float32, at time.Duration) {
	r.w.mu.Lock()
	p.X, p.Z = x, z
	r.w.ctfStepLocked(p, r.at(at))
	r.w.mu.Unlock()
}

// join signs every player up and starts a match: the countdown, its end, everyone landed in the arena, the
// prep's end. It returns the teams' members by index.
func (r *ctfRig) startMatch(t *testing.T) (ember, dusk []*Player) {
	t.Helper()
	for _, s := range r.ss {
		if got := r.w.CTFJoin(s, r.at(0)); len(got) != 1 || !strings.HasPrefix(got[0], "You are signed up") {
			t.Fatalf("join said %q", got)
		}
	}
	r.tick(time.Second) // the countdown
	if r.w.ctf.startAt.IsZero() {
		t.Fatal("no countdown with enough signed up")
	}
	r.tick(time.Second + r.w.ctf.set.Countdown) // the start
	m := r.w.ctf.m
	if m == nil || m.phase != ctfPrep {
		t.Fatal("no match after the countdown")
	}
	r.arrive()
	r.tick(time.Second + r.w.ctf.set.Countdown + r.w.ctf.set.Prep + time.Second) // live
	if m.phase != ctfPlay {
		t.Fatalf("phase %d after the prep", m.phase)
	}
	for _, p := range r.ps {
		switch p.ctf.team {
		case 0:
			ember = append(ember, p)
		default:
			dusk = append(dusk, p)
		}
	}
	return ember, dusk
}

// live is the time the banners went live in startMatch's clock.
func (r *ctfRig) live() time.Duration {
	return time.Second + r.w.ctf.set.Countdown + r.w.ctf.set.Prep + time.Second
}

// Teams are as even as they come: by count first, then the lower sum of levels takes the next highest.
func TestCTFSplitBalances(t *testing.T) {
	ps := []*Player{{Name: "a", Level: 60}, {Name: "b", Level: 10}, {Name: "c", Level: 50}, {Name: "d", Level: 40}, {Name: "e", Level: 30}}
	teams := ctfSplit(ps)
	if len(teams[0]) != 3 || len(teams[1]) != 2 {
		t.Fatalf("teams of %d and %d", len(teams[0]), len(teams[1]))
	}
	sum := func(ps []*Player) (n int) {
		for _, p := range ps {
			n += p.Level
		}
		return
	}
	// 60 to Ember, 50 to Dusk, 40 to Dusk (lower sum), 30 to Ember, 10 to Ember (fewer: 2 against 2, then sums 90/90)
	if sum(teams[0]) != 100 || sum(teams[1]) != 90 {
		t.Fatalf("sums %d and %d", sum(teams[0]), sum(teams[1]))
	}
}

// Signing up: a client that cannot load the arena (a PS2) is told so and kept out; a signed-up player is told it
// is already; leaving takes it off and stops a countdown that no longer has enough.
func TestCTFSignUpRules(t *testing.T) {
	r := newCTF(t, 3)
	r.w.SetLoadableWorlds(func(s *session.Session) func(uint8) bool {
		return func(world uint8) bool { return s != r.ss[2] || world < 6 }
	})
	if got := r.w.CTFJoin(r.ss[2], r.at(0)); len(got) != 1 || got[0] != ctfLineNoLoad {
		t.Fatalf("a PS2 was told %q", got)
	}
	if got := r.w.CTFJoin(r.ss[0], r.at(0)); len(got) != 1 || !strings.HasPrefix(got[0], "You are signed up") {
		t.Fatalf("joined: %q", got)
	}
	if got := r.w.CTFJoin(r.ss[0], r.at(0)); len(got) != 1 || !strings.HasPrefix(got[0], "You are already signed up") {
		t.Fatalf("again: %q", got)
	}
	r.ps[1].HP, r.ps[1].DiedAt = 0, r.at(0)
	if got := r.w.CTFJoin(r.ss[1], r.at(0)); len(got) != 1 || got[0] != ctfLineDead {
		t.Fatalf("the dead were told %q", got)
	}
	r.ps[1].HP, r.ps[1].DiedAt = 1000, time.Time{}
	r.w.CTFJoin(r.ss[1], r.at(0))
	r.tick(time.Second)
	if r.w.ctf.startAt.IsZero() {
		t.Fatal("no countdown with one a side signed up")
	}
	if got := r.w.CTFLeave(r.ss[1], r.at(2*time.Second)); got != ctfLineLeft {
		t.Fatalf("leave said %q", got)
	}
	if !r.w.ctf.startAt.IsZero() {
		t.Fatal("the countdown ran on with one signed up")
	}
	if got := r.w.CTFLeave(r.ss[1], r.at(3*time.Second)); got != ctfLineNotSigned {
		t.Fatalf("a second leave said %q", got)
	}
}

// Signing up needs the arena held, not only its slot loadable: a game whose table takes every slot (a PS2 on the
// July S2, our hard-disk loader's) that never said it holds the arena is told it cannot load it, and is not sent in
// once signed up some other way. The control: the same game holding the arena signs up and goes in.
func TestCTFSignUpNeedsTheArenaHeld(t *testing.T) {
	r := newCTF(t, 2)
	r.w.SetLoadableWorlds(func(*session.Session) func(uint8) bool { return func(world uint8) bool { return world < 30 } })
	r.ss[0].SetServerWorlds(nil)
	if got := r.w.CTFJoin(r.ss[0], r.at(0)); len(got) != 1 || got[0] != ctfLineNoLoad {
		t.Fatalf("a game holding nothing was told %q", got)
	}
	if got := r.w.CTFJoin(r.ss[1], r.at(0)); len(got) != 1 || !strings.HasPrefix(got[0], "You are signed up") {
		t.Fatalf("the control was told %q", got)
	}
	r.w.mu.Lock()
	r.w.ctf.queue = append(r.w.ctf.queue, ctfSignup{id: r.ps[0].ServerID, who: r.ps[0], at: r.at(0)})
	ready := r.w.ctfReadyLocked()
	r.w.mu.Unlock()
	if len(ready) != 1 || ready[0] != r.ps[1] {
		t.Fatalf("ready to go in: %v", r.w.ctfNamesLocked([]EntityID{r.ps[0].ServerID, r.ps[1].ServerID}))
	}
	r.ss[0].SetServerWorlds([]uint8{10, 11})
	r.w.mu.Lock()
	ready = r.w.ctfReadyLocked()
	r.w.mu.Unlock()
	if len(ready) != 2 {
		t.Fatalf("holding the arena, %d ready", len(ready))
	}
}

// The start: one a side, each sent to its own camp, foes only once the banners are live, and both banners on their
// stands with the power-ups out.
func TestCTFStartSendsTeamsToCamps(t *testing.T) {
	r := newCTF(t, 2)
	for _, s := range r.ss {
		r.w.CTFJoin(s, r.at(0))
	}
	r.tick(time.Second)
	r.tick(time.Second + r.w.ctf.set.Countdown)
	due := r.arrive()
	if len(due) != 2 {
		t.Fatalf("%d travels queued", len(due))
	}
	a, b := r.ps[0], r.ps[1]
	if a.ctf.team == b.ctf.team {
		t.Fatal("both on one team")
	}
	for _, p := range r.ps {
		camp := r.w.ctf.lay.Teams[p.ctf.team].Camp
		if p.World != 11 || p.X != camp.X || p.Z != camp.Z {
			t.Fatalf("%s landed at world %d (%.0f, %.0f), its camp is (%.0f, %.0f)", p.Name, p.World, p.X, p.Z, camp.X, camp.Z)
		}
	}
	if a.pvpFoe(b) {
		t.Fatal("foes in the prep")
	}
	m := r.w.ctf.m
	for i, f := range m.flags {
		npc := r.w.NPCs[f.npc]
		stand := r.w.ctf.lay.Teams[i].Stand
		if f.state != ctfFlagHome || npc == nil || npc.X != stand.X || npc.Z != stand.Z || uint32(npc.ModelID) != r.w.ctf.lay.Teams[i].Body {
			t.Fatalf("banner %d: state %d, NPC %+v", i, f.state, npc)
		}
	}
	if len(m.powerUps) != 2 || r.w.NPCs[m.powerUps[0].npc] == nil {
		t.Fatal("the power-ups are not out")
	}
	r.tick(r.live())
	if !a.pvpFoe(b) || !b.pvpFoe(a) {
		t.Fatal("not foes once live")
	}
}

// Foes in the arena are by team alone: a teammate never, flagged or grouped or not; an outsider never; the PvP flag
// changes nothing there.
func TestCTFFoesByTeam(t *testing.T) {
	r := newCTF(t, 4)
	ember, dusk := r.startMatch(t)
	if len(ember) != 2 || len(dusk) != 2 {
		t.Fatalf("teams of %d and %d", len(ember), len(dusk))
	}
	if ember[0].pvpFoe(ember[1]) || dusk[0].pvpFoe(dusk[1]) {
		t.Fatal("teammates are foes")
	}
	ember[0].GroupID, dusk[0].GroupID = 7, 7
	if !ember[0].pvpFoe(dusk[0]) {
		t.Fatal("a group across the teams kept them from fighting")
	}
	outsider := &Player{ServerID: 600009999, Name: "Visitor", World: 11, HP: 10, MaxHP: 10}
	outsider.pvp.on, ember[0].pvp.on = true, true
	if outsider.pvpFoe(ember[0]) || ember[0].pvpFoe(outsider) {
		t.Fatal("a flagged visitor is a match's foe")
	}
	if got := pvpRefusal(ember[0], ember[1]); got != fmt.Sprintf(ctfLineSameTeam, ember[1].Name) {
		t.Fatalf("the refusal at a teammate: %q", got)
	}
}

// A banner taken from its stand goes with its carrier; brought to the carrier's own stand while its own banner
// stands there, it scores, and goes back to its stand.
func TestCTFTakeAndCapture(t *testing.T) {
	r := newCTF(t, 2)
	ember, dusk := r.startMatch(t)
	a, b := ember[0], dusk[0]
	live := r.live()
	duskStand := r.w.ctf.lay.Teams[1].Stand
	r.step(a, duskStand.X+2, duskStand.Z, live+time.Second)
	m := r.w.ctf.m
	f := m.flags[1]
	if f.state != ctfFlagCarried || f.carrier != a.ServerID || !a.ctf.carries {
		t.Fatalf("not taken: state %d carrier %d", f.state, f.carrier)
	}
	if !strings.HasPrefix(a.ShownName(), CTFCarrierTag) {
		t.Fatalf("the carrier's name %q", a.ShownName())
	}
	// it trails its carrier
	r.step(a, 300, 500, live+20*time.Second)
	r.tick(live + 20*time.Second)
	npc := r.w.NPCs[f.npc]
	if npc == nil || distXZ(npc.X, npc.Z, a.X, a.Z) > 2*ctfTrail {
		t.Fatalf("the banner is not with its carrier: %+v", npc)
	}
	// a Dusk player touching its own stand meanwhile does nothing
	r.step(b, duskStand.X, duskStand.Z, live+21*time.Second)
	if f.state != ctfFlagCarried {
		t.Fatal("the stand took back a carried banner")
	}
	emberStand := r.w.ctf.lay.Teams[0].Stand
	r.step(a, emberStand.X, emberStand.Z+1, live+40*time.Second)
	if m.teams[0].score != 1 || f.state != ctfFlagHome || a.ctf.carries || a.ctf.caps != 1 {
		t.Fatalf("no capture: score %d, state %d, carries %v", m.teams[0].score, f.state, a.ctf.carries)
	}
	if strings.HasPrefix(a.ShownName(), CTFCarrierTag) {
		t.Fatal("the tag stayed")
	}
	npc = r.w.NPCs[f.npc]
	if npc == nil || npc.X != duskStand.X || npc.Z != duskStand.Z {
		t.Fatalf("the banner is not back on its stand: %+v", npc)
	}
}

// A capture counts only while the scorer's own banner is on its stand: with it carried off, the carrier at its stand
// is told so and nothing scores; once its banner is home again, it scores.
func TestCTFCaptureNeedsOwnBannerHome(t *testing.T) {
	r := newCTF(t, 2)
	ember, dusk := r.startMatch(t)
	a, b := ember[0], dusk[0]
	live := r.live()
	lay := r.w.ctf.lay
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	r.step(b, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, live+time.Second)
	m := r.w.ctf.m
	if m.flags[0].carrier != b.ServerID || m.flags[1].carrier != a.ServerID {
		t.Fatal("both banners not taken")
	}
	duelDrain(r.sessOf(a))
	r.step(a, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, live+30*time.Second)
	if m.teams[0].score != 0 {
		t.Fatal("scored with its own banner away")
	}
	said := false
	for _, l := range duelLines(duelDrain(r.sessOf(a))) {
		said = said || l == ctfLineOwnAway
	}
	if !said {
		t.Fatal("not told its own banner must be home")
	}
	// b falls: Ember's banner drops; a's teammate (a itself here) returns it, then a scores
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, b, r.at(live+31*time.Second), 0)
	r.w.mu.Unlock()
	if m.flags[0].state != ctfFlagDropped || b.ctf.carries {
		t.Fatalf("the banner did not drop with its carrier: %d", m.flags[0].state)
	}
	r.step(a, m.flags[0].x, m.flags[0].z, live+32*time.Second)
	if m.flags[0].state != ctfFlagHome || a.ctf.returns != 1 {
		t.Fatalf("not returned by its own team's touch: %d", m.flags[0].state)
	}
	r.step(a, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, live+33*time.Second)
	if m.teams[0].score != 1 {
		t.Fatal("no capture once its banner was home")
	}
}

// A carrier's death drops the banner where it fell; an enemy takes it up again from there, and one left alone goes
// home by itself after FlagReturn.
func TestCTFDropTakeUpAndReturnAlone(t *testing.T) {
	r := newCTF(t, 4)
	_, dusk := r.startMatch(t)
	if len(dusk) != 2 {
		t.Fatalf("Dusk has %d", len(dusk))
	}
	b1, b2 := dusk[0], dusk[1]
	live := r.live()
	lay := r.w.ctf.lay
	m := r.w.ctf.m
	f := m.flags[0] // Ember's, which Dusk takes
	r.step(b1, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, live+time.Second)
	r.step(b1, 600, 520, live+10*time.Second)
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, b1, r.at(live+11*time.Second), 0)
	r.w.mu.Unlock()
	if f.state != ctfFlagDropped || distXZ(f.x, f.z, 600, 520) > 2*ctfTrail || r.w.NPCs[f.npc] == nil {
		t.Fatalf("dropped: state %d at (%.1f, %.1f)", f.state, f.x, f.z)
	}
	// the other Dusk player takes it up where it lies, and falls with it too
	r.step(b2, f.x, f.z, live+12*time.Second)
	if f.state != ctfFlagCarried || f.carrier != b2.ServerID {
		t.Fatal("an enemy did not take it up")
	}
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, b2, r.at(live+13*time.Second), 0)
	r.w.mu.Unlock()
	if f.state != ctfFlagDropped {
		t.Fatal("not dropped the second time")
	}
	r.tick(live + 13*time.Second + r.w.ctf.set.FlagReturn - time.Second)
	if f.state != ctfFlagDropped {
		t.Fatal("went home early")
	}
	r.tick(live + 13*time.Second + r.w.ctf.set.FlagReturn)
	stand := lay.Teams[0].Stand
	if npc := r.w.NPCs[f.npc]; f.state != ctfFlagHome || npc == nil || npc.X != stand.X || npc.Z != stand.Z {
		t.Fatalf("a banner left alone did not go home: %d", f.state)
	}
}

// The dead rise together: the waves come every WaveEvery from the banners going live, and a death waits for the
// first wave at least WaveMinDead after it, through the tick as the server runs it (the respawn system after
// capture the flag's); the respawn lands at its team's camp; a death costs nothing.
func TestCTFWaves(t *testing.T) {
	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	m := r.w.ctf.m
	set := r.w.ctf.set
	// die a second before the second wave: that one is too soon, the third takes it
	second := m.opens.Add(2 * set.WaveEvery)
	died := second.Add(-time.Second)
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, a, died, 0)
	r.w.mu.Unlock()
	rise := func(at time.Time) bool {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.ctfSystem(at)
		for _, d := range r.w.playerRespawnSystem(at) {
			if d.p == a {
				return true
			}
		}
		return false
	}
	if rise(second) {
		t.Fatal("rose with the wave a second after its death")
	}
	if rise(second.Add(set.WaveEvery - time.Millisecond)) {
		t.Fatal("rose before the next wave")
	}
	if !rise(second.Add(set.WaveEvery)) {
		t.Fatal("not risen with the next wave")
	}
	if a.Debt != 0 {
		t.Fatalf("a debt of %d", a.Debt)
	}
	spot, _, ok := r.w.CTFRespawnSpot(a.ServerID)
	camp := r.w.ctf.lay.Teams[a.ctf.team].Camp
	if !ok || spot.World != 11 || spot.X != camp.X || spot.Z != camp.Z {
		t.Fatalf("rises at %+v, its camp is %+v", spot, camp)
	}
}

// ScoreLimit captures win; then EndHold later everyone goes back beside the herald, a dead one at its rise.
func TestCTFScoreLimitWinsAndSendsBack(t *testing.T) {
	r := newCTF(t, 2)
	ember, dusk := r.startMatch(t)
	a, b := ember[0], dusk[0]
	live := r.live()
	lay := r.w.ctf.lay
	m := r.w.ctf.m
	for i := 0; i < r.w.ctf.set.ScoreLimit; i++ {
		x := live + time.Duration(i+1)*time.Minute
		r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, x)
		r.step(a, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, x+30*time.Second)
	}
	if m.phase != ctfOver || m.winner != 0 || m.teams[0].score != r.w.ctf.set.ScoreLimit {
		t.Fatalf("phase %d winner %d score %d", m.phase, m.winner, m.teams[0].score)
	}
	if a.pvpFoe(b) {
		t.Fatal("foes after the end")
	}
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, b, m.overAt, 0) // fallen at the end: rises beside the herald
	r.w.mu.Unlock()
	end := m.overAt.Sub(r.t0)
	r.tick(end + r.w.ctf.set.EndHold)
	if r.w.ctf.m != nil {
		t.Fatal("the match did not let go")
	}
	due := r.arrive()
	if len(due) != 1 || due[0].P != a || due[0].To.World != 0 || distXZ(due[0].To.X, due[0].To.Z, r.herald.X, r.herald.Z) > ctfReturnOut+0.1 {
		t.Fatalf("the way back: %+v", due)
	}
	if a.ctf.match != 0 || !b.ctf.home {
		t.Fatalf("after: a's match %d, b goes home %v", a.ctf.match, b.ctf.home)
	}
	spot, _, ok := r.w.CTFRespawnSpot(b.ServerID)
	if !ok || spot.World != 0 {
		t.Fatalf("the dead one rises at %+v", spot)
	}
}

// At the time limit the higher score wins, and a tie is a draw.
func TestCTFTimeLimit(t *testing.T) {
	for _, c := range []struct {
		ember, dusk, winner int
	}{{1, 0, 0}, {0, 2, 1}, {1, 1, -1}, {0, 0, -1}} {
		r := newCTF(t, 2)
		r.startMatch(t)
		m := r.w.ctf.m
		m.teams[0].score, m.teams[1].score = c.ember, c.dusk
		r.tick(r.live() + r.w.ctf.set.TimeLimit + time.Second)
		if m.phase != ctfOver || m.winner != c.winner {
			t.Fatalf("%d-%d: phase %d winner %d, want %d", c.ember, c.dusk, m.phase, m.winner, c.winner)
		}
	}
}

// A team left empty for ctfForfeitAfter loses; a carrier logging out drops its banner where it was.
func TestCTFLogoutDropsAndForfeits(t *testing.T) {
	r := newCTF(t, 2)
	ember, dusk := r.startMatch(t)
	a, b := ember[0], dusk[0]
	live := r.live()
	lay := r.w.ctf.lay
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	r.step(a, 500, 500, live+5*time.Second)
	r.tick(live + 5*time.Second)
	r.w.mu.Lock()
	r.w.ctfLeavingLocked(a)
	r.w.mu.Unlock()
	m := r.w.ctf.m
	if m.flags[1].state != ctfFlagDropped || len(m.teams[0].members) != 0 {
		t.Fatalf("after the logout: banner %d, Ember %d", m.flags[1].state, len(m.teams[0].members))
	}
	r.tick(live + 5*time.Second + ctfForfeitAfter - time.Second)
	if m.phase == ctfOver {
		t.Fatal("forfeit before its time")
	}
	r.tick(live + 5*time.Second + ctfForfeitAfter)
	if m.phase != ctfOver || m.winner != 1 {
		t.Fatalf("no forfeit: phase %d winner %d", m.phase, m.winner)
	}
	_ = b
}

// A carrier held too long takes more from every hit: WoundStep from WoundAfter, as much again each minute, at most
// WoundMax.
func TestCTFWounds(t *testing.T) {
	set := DefaultCTFSettings()
	for _, c := range []struct {
		held time.Duration
		pct  int
	}{{0, 0}, {set.WoundAfter - time.Second, 0}, {set.WoundAfter, 25}, {set.WoundAfter + time.Minute, 50}, {set.WoundAfter + 30*time.Minute, 200}} {
		if got := ctfWoundPct(set, c.held); got != c.pct {
			t.Fatalf("held %v: %d%%, want %d%%", c.held, got, c.pct)
		}
	}
	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	live := r.live()
	lay := r.w.ctf.lay
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	r.tick(live + time.Second + set.WoundAfter + time.Minute)
	r.w.mu.Lock()
	got := r.w.ctfHurtLocked(a, 100)
	r.w.mu.Unlock()
	if got != 150 {
		t.Fatalf("a hit of 100 on a carrier held %v lands %d", set.WoundAfter+time.Minute, got)
	}
}

// The field's edge sends a straying player back to its camp, dropping a banner it carried.
func TestCTFFieldEdge(t *testing.T) {
	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	live := r.live()
	lay := r.w.ctf.lay
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	r.step(a, 1200, 500, live+2*time.Second)
	if a.ctf.carries || r.w.ctf.m.flags[1].state != ctfFlagDropped {
		t.Fatal("the banner left the field")
	}
	due := r.arrive()
	if len(due) != 1 || due[0].To.X != lay.Teams[0].Camp.X || due[0].To.Z != lay.Teams[0].Camp.Z {
		t.Fatalf("not sent to its camp: %+v", due)
	}
}

// A round field (a hollow's rim: tools/worlds/ring.json's field_circle): a player of the match past its radius goes back
// to its camp and drops the banner it carried, one inside it anywhere (the rectangle's corners too) does not.
func TestCTFRoundFieldEdge(t *testing.T) {
	r := newCTF(t, 2)
	r.w.mu.Lock()
	r.w.ctf.lay.Field = nil
	r.w.ctf.lay.Ring = CTFRing{X: 500, Z: 500, Radius: 450}
	r.w.mu.Unlock()
	ember, _ := r.startMatch(t)
	a := ember[0]
	live := r.live()
	lay := r.w.ctf.lay
	if !lay.InField(500, 60) || lay.InField(60, 60) || !lay.InField(940, 500) || lay.InField(960, 500) {
		t.Fatal("the ring's edge is not its radius")
	}
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	if !a.ctf.carries {
		t.Fatal("the banner was not taken")
	}
	r.step(a, 500, 940, live+2*time.Second) // inside, near the edge
	if !a.ctf.carries || len(r.arrive()) != 0 {
		t.Fatal("sent back from inside the ring")
	}
	r.step(a, 60, 60, live+3*time.Second) // inside the old box, outside the ring
	if a.ctf.carries || r.w.ctf.m.flags[1].state != ctfFlagDropped {
		t.Fatal("the banner left the field")
	}
	due := r.arrive()
	if len(due) != 1 || due[0].To.X != lay.Teams[0].Camp.X || due[0].To.Z != lay.Teams[0].Camp.Z {
		t.Fatalf("not sent to its camp: %+v", due)
	}
}

// The power-ups: the speed one puts on Spirit of the Wolf's run speed, the heal fills hit points and power; each
// gone until PowerUpEvery has passed.
func TestCTFPowerUps(t *testing.T) {
	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	live := r.live()
	m := r.w.ctf.m
	r.step(a, 500, 300, live+time.Second)
	if !a.hasEffect(CTFSpeedEffectID) || a.SpeedNow() <= a.Speed {
		t.Fatalf("no speed: %v", a.SpeedNow())
	}
	if m.powerUps[0].npc != 0 {
		t.Fatal("the shrine stayed")
	}
	a.HP, a.Power = 10, 10
	r.step(a, 500, 700, live+2*time.Second)
	if a.HP != a.MaxHPNow() || a.Power != a.MaxPowerNow() {
		t.Fatalf("not mended: %d/%d", a.HP, a.Power)
	}
	r.tick(live + time.Second + r.w.ctf.set.PowerUpEvery)
	if m.powerUps[0].npc == 0 {
		t.Fatal("the shrine did not come back")
	}
}

// The view: to a player of the match, the other team green and swingable, its own blue and not, the other team's
// carrier of its banner gold (byte 142 0); a dead player of the match lies down for everyone.
func TestCTFView(t *testing.T) {
	r := newCTF(t, 3)
	ember, dusk := r.startMatch(t)
	live := r.live()
	lay := r.w.ctf.lay
	var viewer, ally *Player
	var vs *session.Session
	for i, p := range r.ps {
		if p == ember[0] {
			viewer, vs = p, r.ss[i]
		}
	}
	for _, p := range ember {
		if p != viewer {
			ally = p
		}
	}
	foe := dusk[0]
	view := func(p *Player) []byte {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		buf, _ := r.w.serializeEntityDiscLocked(vs, p.ServerID, 0x1234)
		return buf
	}
	if b := view(foe); b[142] != pvpFoeByte || b[157]&1 == 0 {
		t.Fatalf("a foe: byte 142 %d, 157 %d", b[142], b[157])
	}
	if ally != nil {
		if b := view(ally); b[142] != 2 || b[157]&1 != 0 {
			t.Fatalf("an ally: byte 142 %d, 157 %d", b[142], b[157])
		}
	}
	r.step(foe, lay.Teams[0].Stand.X, lay.Teams[0].Stand.Z, live+time.Second)
	if b := view(foe); b[142] != 0 || !strings.HasPrefix(string(b[116:140]), CTFCarrierTag) {
		t.Fatalf("the carrier of its banner: byte 142 %d, name %q", b[142], b[116:140])
	}
	r.w.mu.Lock()
	r.w.playerDownLocked(nil, foe, r.at(live+2*time.Second), 0)
	r.w.mu.Unlock()
	if b := view(foe); b[58] != pvpDyingState {
		t.Fatalf("the fallen stands: byte 58 %d", b[58])
	}
}

// A player in the arena's world with no match (a restart's, a GM's move of another) is sent beside the herald
// after ctfStrayGrace; a GM himself stays.
func TestCTFStrays(t *testing.T) {
	r := newCTF(t, 2)
	a, gm := r.ps[0], r.ps[1]
	a.World, gm.World, gm.GM = 11, 11, true
	r.tick(time.Second)
	r.tick(time.Second + ctfStrayGrace - time.Millisecond)
	if len(r.w.gates.due) != 0 {
		t.Fatal("sent before its grace")
	}
	r.tick(2*time.Second + ctfStrayGrace)
	due := r.arrive()
	if len(due) != 1 || due[0].P != a || due[0].To.World != 0 {
		t.Fatalf("strays sent: %+v", due)
	}
}

// A signed-up player is sent into a match that is on, to the team with fewer.
func TestCTFBackfill(t *testing.T) {
	r := newCTF(t, 3)
	for _, s := range r.ss[:2] {
		r.w.CTFJoin(s, r.at(0))
	}
	r.tick(time.Second)
	r.tick(time.Second + r.w.ctf.set.Countdown)
	r.arrive()
	r.tick(r.live())
	m := r.w.ctf.m
	if m == nil || m.phase != ctfPlay {
		t.Fatal("no match on")
	}
	late := r.ps[2]
	if got := r.w.CTFJoin(r.ss[2], r.at(r.live()+time.Second)); len(got) != 1 || !strings.Contains(got[0], ctfLineWaitMatch) {
		t.Fatalf("a late sign-up was told %q", got)
	}
	r.tick(r.live() + 2*time.Second)
	if !late.inMatch(m) || len(m.teams[late.ctf.team].members) != 2 {
		t.Fatalf("not sent in: in %v", late.inMatch(m))
	}
}

// A report the move check holds as from before a server's move (its pending: a client's last report from where it
// fell, sent before it heard of its rise) touches nothing, nor anything in the moment after the server put it
// somewhere.
func TestCTFStaleReportTouchesNothing(t *testing.T) {
	r := newCTF(t, 2)
	ember, _ := r.startMatch(t)
	a := ember[0]
	live := r.live()
	lay := r.w.ctf.lay
	a.mv.pending = true
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+time.Second)
	r.tick(live + time.Second)
	if a.ctf.carries {
		t.Fatal("a stale report took the banner")
	}
	a.mv.pending = false
	a.ctf.quiet = r.at(live + 3*time.Second)
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+2*time.Second)
	if a.ctf.carries {
		t.Fatal("took the banner in the quiet after a server's move")
	}
	r.step(a, lay.Teams[1].Stand.X, lay.Teams[1].Stand.Z, live+3*time.Second)
	if !a.ctf.carries {
		t.Fatal("not taken once its reports were its own again")
	}
}
