package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A mercenary and a familiar never enter capture the flag's arena (ctfaway.go): they wait outside while their owner
// fights, nothing calls them in, they never count on a team or touch a banner, and they come back when their owner
// is out; a dismissal or a logout ends the wait, a keep restart carries it; a group across the teams stays the group
// it was. Each test's control is the same thing outside the arena, or a player without them.

// ctfAwayRig is a ctfRig whose ticks run the mercs' and the familiars' systems too, keeping the merc events.
type ctfAwayRig struct {
	*ctfRig
	notes []MercNote
}

func newCTFAway(t *testing.T, n int) *ctfAwayRig {
	t.Helper()
	r := &ctfAwayRig{ctfRig: newCTF(t, n)}
	r.w.OnMercNote(func(m MercNote) { r.notes = append(r.notes, m) })
	return r
}

// all runs the mercs', the familiars' and capture the flag's systems at x, as the tick does, then hands out the
// merc events.
func (r *ctfAwayRig) all(x time.Duration) {
	r.w.mu.Lock()
	r.w.TickCount++
	now := r.at(x)
	r.w.mercSystem(now)
	r.w.familiarSystem(now)
	r.w.ctfSystem(now)
	r.w.mu.Unlock()
	r.w.runMercNotes()
}

// ctfMercKit is a warrior merc of p's level in a plate set and a sword.
func ctfMercKit(p *Player, name string) MercKit {
	k := MercKit{Class: 0, Race: 0, Level: p.Level, Name: name, Base: SimBaseStats(0, 0, p.Level),
		Gear: Gear{WeaponCond: 1, AC: 240, Sta: 30, Str: 20, MaxHP: 160, MaxPower: 40, WeaponDamage: 180, WeaponSlot: 14, WeaponType: 1}}
	k.Look = MercLook{Robe: -1, Primary: 0x47D9E5E0, PrimaryType: 1}
	k.Worn[14], k.AttackType = true, 1
	return k
}

// hire hires player i a merc, which must come.
func (r *ctfAwayRig) hire(t *testing.T, i int, name string) *mercState {
	t.Helper()
	if _, why := r.w.HireMerc(r.ss[i], ctfMercKit(r.ps[i], name), MercAssist, r.at(0)); why != "" {
		t.Fatalf("%s's hire was refused: %s", r.ps[i].Name, why)
	}
	r.yesToMerc(i)
	return r.w.mercs[r.ps[i].ServerID]
}

// yesToMerc answers yes to player i's merc's box, if one is up (a merc fights
// and follows only in its owner's group: mercgroup.go).
func (r *ctfAwayRig) yesToMerc(i int) {
	p := r.ps[i]
	r.w.mu.Lock()
	ask, up := r.w.invites[p.ServerID][p.ServerID]
	r.w.mu.Unlock()
	if up && ask.merc {
		r.w.GroupAccept(r.ss[i], r.ss[i].PlayerObjectID())
	}
}

// callHealbot is Healbot's item landing for player i at x.
func (r *ctfAwayRig) callHealbot(i int, x time.Duration) *familiarState {
	d := healbot()
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.summonFamiliarLocked(r.ps[i], r.ss[i], &d, r.at(x))
	return r.w.familiars[r.ps[i].ServerID]
}

// followersOf is p's merc and familiar in the world, nil each for none.
func (r *ctfAwayRig) followersOf(p *Player) (merc, familiar *NPC) {
	if ms := r.w.mercs[p.ServerID]; ms != nil && ms.owner == p && r.w.NPCs[ms.npc.ServerID] == ms.npc {
		merc = ms.npc
	}
	if fs := r.w.familiars[p.ServerID]; fs != nil && fs.owner == p && r.w.NPCs[fs.npc.ServerID] == fs.npc {
		familiar = fs.npc
	}
	return merc, familiar
}

// theirsIn is the names of every NPC in the world that is p's merc or familiar, whatever list holds it.
func (r *ctfAwayRig) theirsIn(p *Player, world uint8) []string {
	var out []string
	for _, n := range r.w.NPCs {
		if n.World == world && (n.merc != nil && n.merc.owner == p || n.familiar != nil && n.familiar.owner == p) {
			out = append(out, n.Name)
		}
	}
	return out
}

// backNotes is the MercNoteBack events for p.
func (r *ctfAwayRig) backNotes(p *Player) []MercNote {
	var out []MercNote
	for _, n := range r.notes {
		if n.Kind == MercNoteBack && n.Owner == p {
			out = append(out, n)
		}
	}
	return out
}

// ctfLinesOf is every line s was sent since the last look.
func ctfLinesOf(s *session.Session) []string {
	var msgs [][]byte
	for _, m := range s.Queue().GetPending() {
		msgs = append(msgs, m.Data())
	}
	return linesOf(msgs)
}

// endAndSendBack runs the match to its time limit and the way back, everyone landed; it returns the time of the
// landing.
func (r *ctfAwayRig) endAndSendBack(t *testing.T) time.Duration {
	t.Helper()
	m := r.w.ctf.m
	r.all(r.live() + r.w.ctf.set.TimeLimit + time.Second)
	if m.phase != ctfOver {
		t.Fatalf("phase %d at the time limit", m.phase)
	}
	back := m.overAt.Sub(r.t0) + r.w.ctf.set.EndHold
	r.all(back)
	if r.w.ctf.m != nil {
		t.Fatal("the match did not let go")
	}
	r.arrive()
	return back
}

// Sent in, its owner leaves its merc and its familiar outside (out of the world, told so), the merc out of its
// team and the arena; they come along into no part of the match, and once their owner is back beside the herald the
// familiar comes back with its share of hit points and the handlers are asked to bring the merc (MercNoteBack), whose
// hire stands it beside its owner. The controls: before the match both follow their owner; the other player, sent
// in with neither, has neither after and no event.
func TestCTFMercAndFamiliarWaitOutside(t *testing.T) {
	r := newCTFAway(t, 2)
	a, b := r.ps[0], r.ps[1]
	r.hire(t, 0, "Gorvak")
	fs := r.callHealbot(0, 0)
	fs.npc.HP = fs.npc.MaxHP / 2
	r.all(100 * time.Millisecond)
	if m, f := r.followersOf(a); m == nil || f == nil {
		t.Fatalf("before the match: merc %v familiar %v", m != nil, f != nil)
	}
	ctfLinesOf(r.ss[0])

	r.startMatch(t)
	if m, f := r.followersOf(a); m != nil || f != nil {
		t.Fatalf("in the match: merc %v familiar %v", m != nil, f != nil)
	}
	lines := ctfLinesOf(r.ss[0])
	if !hasLine(lines, fmt.Sprintf(ctfLineMercAway, "Gorvak")) || !hasLine(lines, fmt.Sprintf(ctfLineFamiliarAway, "Healbot")) {
		t.Fatalf("its owner was told %q", lines)
	}
	if mm := r.w.ctf.m.teams[a.ctf.team].members; len(mm) != 1 || mm[0] != a.ServerID {
		t.Fatalf("its owner's team: %v", mm)
	}
	live := r.live()
	for i := 1; i <= 5; i++ {
		r.all(live + time.Duration(i)*time.Second)
	}
	if got := r.theirsIn(a, r.w.ctf.set.World); len(got) != 0 || len(r.notes) != 0 {
		t.Fatalf("in the arena: %v, events %v", got, r.notes)
	}

	back := r.endAndSendBack(t)
	r.all(back + 100*time.Millisecond)
	_, f := r.followersOf(a)
	if f == nil || f.World != 0 || f.HP != f.MaxHP/2 || distXZ(f.X, f.Z, a.X, a.Z) > petLost {
		t.Fatalf("the familiar after: %+v", f)
	}
	if got := r.backNotes(a); len(got) != 1 || got[0].Name != "Gorvak" || got[0].Sess != r.ss[0] {
		t.Fatalf("the merc's events: %+v", r.notes)
	}
	if !hasLine(ctfLinesOf(r.ss[0]), fmt.Sprintf(ctfLineFamiliarBack, "Healbot")) {
		t.Error("its owner was not told the familiar is back")
	}
	// the handlers' answer to the event: the hire again, from its row
	if _, why := r.w.HireMerc(r.ss[0], ctfMercKit(a, "Gorvak"), MercAssist, r.at(back+200*time.Millisecond)); why != "" {
		t.Fatalf("the merc's return was refused: %s", why)
	}
	if m, _ := r.followersOf(a); m == nil || m.World != 0 || distXZ(m.X, m.Z, a.X, a.Z) > petLost {
		t.Fatalf("the merc after: %+v", m)
	}
	r.all(back + 300*time.Millisecond)
	if len(r.backNotes(a)) != 1 {
		t.Error("the merc was asked back twice")
	}
	if m, f := r.followersOf(b); m != nil || f != nil || len(r.backNotes(b)) != 0 {
		t.Fatalf("the other player: merc %v familiar %v events %d", m != nil, f != nil, len(r.backNotes(b)))
	}
}

// Any other way into the arena's world leaves them outside as well (a GM's move, in no match: the stray rule leaves
// a GM where he stands), sent out at their next step, and back once he stands elsewhere. The control: a move to
// another world of the game brings them along, as ever.
func TestCTFFollowersWaitOutsideTheArenasWorld(t *testing.T) {
	r := newCTFAway(t, 1)
	a := r.ps[0]
	a.GM = true
	r.hire(t, 0, "Gorvak")
	r.callHealbot(0, 0)
	a.World, a.X, a.Z = 3, 4000, 4000
	r.all(time.Second)
	if m, f := r.followersOf(a); m == nil || f == nil || m.World != 3 || f.World != 3 {
		t.Fatalf("not along into world 3: merc %+v familiar %+v", m, f)
	}
	a.World, a.X, a.Z = r.w.ctf.set.World, 500, 500
	r.all(2 * time.Second)
	if got := r.theirsIn(a, r.w.ctf.set.World); len(got) != 0 {
		t.Fatalf("in the arena's world: %v", got)
	}
	if m, f := r.followersOf(a); m != nil || f != nil {
		t.Fatalf("out in the arena's world: merc %v familiar %v", m != nil, f != nil)
	}
	r.all(3 * time.Second)
	if len(r.backNotes(a)) != 0 {
		t.Fatal("asked back while its owner stood in the arena's world")
	}
	a.World, a.X, a.Z = 0, 5000, 5000
	r.all(4 * time.Second)
	if _, f := r.followersOf(a); f == nil || len(r.backNotes(a)) != 1 {
		t.Fatalf("not back: familiar %v, events %d", f != nil, len(r.backNotes(a)))
	}
}

// In a match nothing calls them in: a hire (/merc, a liaison) is refused with the line and kept for after, and the
// familiar's item lands nothing but the line; both come once their owner is out, the familiar whole (a new call).
// The controls: the same hire and item outside the arena bring them at once.
func TestCTFNothingCallsThemIntoTheArena(t *testing.T) {
	r := newCTFAway(t, 2)
	a := r.ps[0]
	r.startMatch(t)
	live := r.live()
	if _, why := r.w.HireMerc(r.ss[0], ctfMercKit(a, "Gorvak"), MercAssist, r.at(live)); why != fmt.Sprintf(ctfLineMercAway, "Gorvak") {
		t.Fatalf("a hire in the arena: %q", why)
	}
	if fs := r.callHealbot(0, live); fs != nil {
		t.Fatal("the familiar's item called it into the arena")
	}
	if !hasLine(ctfLinesOf(r.ss[0]), fmt.Sprintf(ctfLineFamiliarAway, "Healbot")) {
		t.Error("the item's use told nothing")
	}
	if got := r.theirsIn(a, r.w.ctf.set.World); len(got) != 0 {
		t.Fatalf("in the arena: %v", got)
	}
	back := r.endAndSendBack(t)
	r.all(back + 100*time.Millisecond)
	if _, f := r.followersOf(a); f == nil || f.HP != f.MaxHP || len(r.backNotes(a)) != 1 {
		t.Fatalf("after: familiar %+v, events %d", f, len(r.backNotes(a)))
	}
	// the controls, out of the arena: at once
	r2 := newCTFAway(t, 1)
	if _, why := r2.w.HireMerc(r2.ss[0], ctfMercKit(r2.ps[0], "Gorvak"), MercAssist, r2.at(0)); why != "" {
		t.Fatalf("a hire outside the arena: %q", why)
	}
	if fs := r2.callHealbot(0, 0); fs == nil {
		t.Fatal("the item outside the arena called nothing")
	}
}

// A dismissal while they wait ends them (the hire's row is the handlers'), and nothing comes after; a logout lets
// the wait go (a login brings the merc back from its row). The controls: a second dismissal finds nothing, and a
// player whose wait was neither dismissed nor let go is asked back after.
func TestCTFWaitingDismissedOrLoggedOut(t *testing.T) {
	r := newCTFAway(t, 3)
	a, b, c := r.ps[0], r.ps[1], r.ps[2]
	r.hire(t, 0, "Gorvak")
	r.callHealbot(0, 0)
	r.hire(t, 1, "Brisa")
	r.hire(t, 2, "Torrin")
	r.startMatch(t)
	if name, ok := r.w.DismissMerc(r.ss[0], r.at(r.live())); !ok || name != "Gorvak" {
		t.Fatalf("dismissing the waiting merc: %q %v", name, ok)
	}
	if name, ok := r.w.DismissFamiliar(r.ss[0]); !ok || name != "Healbot" {
		t.Fatalf("dismissing the waiting familiar: %q %v", name, ok)
	}
	if _, ok := r.w.DismissMerc(r.ss[0], r.at(r.live())); ok {
		t.Error("a second dismissal found a merc")
	}
	if _, ok := r.w.DismissFamiliar(r.ss[0]); ok {
		t.Error("a second dismissal found a familiar")
	}
	r.w.mu.Lock()
	r.w.ctfLeavingLocked(b)
	delete(r.w.Players, b.ServerID)
	r.w.mu.Unlock()
	if r.w.ctf.away[b.ServerID] != nil {
		t.Error("a logout kept the wait")
	}
	back := r.endAndSendBack(t)
	r.all(back + 100*time.Millisecond)
	if m, f := r.followersOf(a); m != nil || f != nil || len(r.backNotes(a)) != 0 {
		t.Fatalf("dismissed, yet: merc %v familiar %v events %d", m != nil, f != nil, len(r.backNotes(a)))
	}
	if len(r.backNotes(b)) != 0 {
		t.Error("a player logged out was asked back")
	}
	if got := r.backNotes(c); len(got) != 1 || got[0].Name != "Torrin" {
		t.Errorf("the control's events: %+v", r.notes)
	}
}

// A keep restart carries what waits as if it were out (KeepPlayer), and the new process stands it beside its owner
// out of the arena, with the familiar's share of hit points; an owner standing in the arena's world gets it back to
// waiting instead. The control: a player with neither carries neither.
func TestCTFAwayThroughAKeepRestart(t *testing.T) {
	r := newCTFAway(t, 2)
	r.hire(t, 0, "Gorvak")
	fs := r.callHealbot(0, 0)
	fs.npc.HP = fs.npc.MaxHP / 4
	r.startMatch(t)
	kp, ok := r.w.KeepPlayer(r.ss[0], r.at(r.live()))
	if !ok || kp.Merc == nil || kp.Merc.Name != "Gorvak" || !kp.Merc.Away || kp.Familiar == nil || kp.Familiar.Item != healbot().Item || kp.Familiar.HPPct != 0.25 {
		t.Fatalf("kept: merc %+v familiar %+v", kp.Merc, kp.Familiar)
	}
	if kb, _ := r.w.KeepPlayer(r.ss[1], r.at(r.live())); kb.Merc != nil || kb.Familiar != nil {
		t.Fatalf("the control kept: %+v %+v", kb.Merc, kb.Familiar)
	}

	// the new process, its owner beside the herald (the arena login rule)
	r2 := newCTFAway(t, 1)
	if !r2.w.RestoreMerc(r2.ss[0], ctfMercKit(r2.ps[0], "Gorvak"), *kp.Merc, r2.at(0)) {
		t.Fatal("the merc did not come back")
	}
	if !r2.w.RestoreFamiliar(r2.ss[0], healbot(), kp.Familiar.HPPct, r2.at(0)) {
		t.Fatal("the familiar did not come back")
	}
	m, f := r2.followersOf(r2.ps[0])
	if m == nil || f == nil || m.World != 0 || distXZ(m.X, m.Z, r2.ps[0].X, r2.ps[0].Z) > petLost || f.HP != f.MaxHP/4 {
		t.Fatalf("back: merc %+v familiar %+v", m, f)
	}
	// its owner asked into a group with it, as at its return from the arena (a merc carried out of a group is not:
	// TestMercKeepRestart)
	r2.ss[0].Queue().GetPending()
	for i := 1; i <= 5; i++ {
		r2.all(time.Duration(i) * 100 * time.Millisecond)
	}
	asked := 0
	for _, msg := range r2.ss[0].Queue().GetPending() {
		if opOfMsg(msg.Data()) == protocol.OpcodeGroupInviteReceived {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("its owner was asked into a group %d times, want once", asked)
	}

	// an owner in the arena's world: back to waiting
	r3 := newCTFAway(t, 1)
	p3 := r3.ps[0]
	p3.World, p3.X, p3.Z = r3.w.ctf.set.World, 500, 500
	if r3.w.RestoreMerc(r3.ss[0], ctfMercKit(p3, "Gorvak"), *kp.Merc, r3.at(0)) ||
		r3.w.RestoreFamiliar(r3.ss[0], healbot(), kp.Familiar.HPPct, r3.at(0)) {
		t.Fatal("restored into the arena's world")
	}
	if aw := r3.w.ctf.away[p3.ServerID]; aw == nil || aw.merc != "Gorvak" || aw.familiar == nil || aw.famHP != 0.25 {
		t.Fatalf("not waiting: %+v", aw)
	}
	if got := r3.theirsIn(p3, r3.w.ctf.set.World); len(got) != 0 {
		t.Fatalf("in the arena's world: %v", got)
	}
}

// A merc is never a match's: one put in the arena all the same (spawnMercLocked, which the hire's refusal guards)
// standing on the other team's stand takes nothing, its owner's team is its owner alone, and its next step sends it
// out. The control: its owner on that stand takes the banner.
func TestCTFMercNeverTakesABanner(t *testing.T) {
	r := newCTFAway(t, 2)
	a := r.ps[0]
	r.startMatch(t)
	m, lay := r.w.ctf.m, r.w.ctf.lay
	other := lay.Teams[1-a.ctf.team].Stand
	r.w.mu.Lock()
	ms := r.w.spawnMercLocked(a, ctfMercKit(a, "Gorvak"), MercAssist, r.at(r.live()))
	ms.npc.X, ms.npc.Z = other.X, other.Z
	r.w.ctfTouchAllLocked(m, r.at(r.live()+time.Second))
	r.w.mu.Unlock()
	if f := m.flags[1-a.ctf.team]; f.state != ctfFlagHome {
		t.Fatalf("the banner with the merc on its stand: state %d carrier %d", f.state, f.carrier)
	}
	for t2 := range m.teams {
		for _, id := range m.teams[t2].members {
			if r.w.Players[id] == nil {
				t.Fatalf("team %d holds %d, no player", t2, id)
			}
		}
	}
	r.all(r.live() + 2*time.Second)
	if got := r.theirsIn(a, r.w.ctf.set.World); len(got) != 0 {
		t.Fatalf("its next step left it in the arena: %v", got)
	}
	if aw := r.w.ctf.away[a.ServerID]; aw == nil || aw.merc != "Gorvak" {
		t.Fatalf("not waiting: %+v", aw)
	}
	r.step(a, other.X, other.Z, r.live()+3*time.Second)
	if f := m.flags[1-a.ctf.team]; f.state != ctfFlagCarried || f.carrier != a.ServerID {
		t.Fatalf("its owner on the stand: state %d carrier %d", f.state, f.carrier)
	}
}

// A raid flag's group with a merc in it signs up: the teams are the match's own (as even as they come by levels,
// whatever the group), the group keeps its four players (the merc out of it, waiting), still a raid, and the teams
// fight across it while teammates do not; after the match the merc is asked back. The control: before the match
// none of the group is another's foe.
func TestCTFGroupsAcrossTheTeamsStayTheirGroup(t *testing.T) {
	r := newCTFAway(t, 4)
	a, b, c, d := r.ps[0], r.ps[1], r.ps[2], r.ps[3]
	r.w.mu.Lock()
	g := r.w.formGroupLocked(a)
	for _, p := range []*Player{b, c, d} {
		g.Members = append(g.Members, p.ServerID)
		p.GroupID = g.ID
	}
	g.flagged = true
	r.w.mu.Unlock()
	ms := r.hire(t, 0, "Gorvak")
	if len(g.Members) != 5 || !groupHas(g, ms.npc.ServerID) {
		t.Fatalf("the merc not in the raid: %v", g.Members)
	}
	for _, p := range r.ps {
		for _, q := range r.ps {
			if p != q && p.pvpFoe(q) {
				t.Fatalf("before the match %s and %s are foes", p.Name, q.Name)
			}
		}
	}
	ember, dusk := r.startMatch(t)
	// levels 30 to 33: 33 and 30 against 32 and 31 (ctfSplit), the group split across
	if len(ember) != 2 || len(dusk) != 2 || !(d.ctf.team == a.ctf.team && c.ctf.team == b.ctf.team && a.ctf.team != b.ctf.team) {
		t.Fatalf("the teams: %v against %v", r.w.ctfNamesLocked(r.w.ctf.m.teams[0].members), r.w.ctfNamesLocked(r.w.ctf.m.teams[1].members))
	}
	if len(g.Members) != 4 || groupHas(g, ms.npc.ServerID) || !g.flagged || r.w.groups[g.ID] != g {
		t.Fatalf("the group in the match: %v flagged %v", g.Members, g.flagged)
	}
	for _, p := range r.ps {
		if p.GroupID != g.ID {
			t.Fatalf("%s left the group", p.Name)
		}
	}
	if !a.pvpFoe(b) || !d.pvpFoe(c) || a.pvpFoe(d) || b.pvpFoe(c) {
		t.Fatal("foes not by team alone")
	}
	back := r.endAndSendBack(t)
	r.all(back + 100*time.Millisecond)
	if len(r.backNotes(a)) != 1 {
		t.Fatalf("the merc's events: %+v", r.notes)
	}
	if _, why := r.w.HireMerc(r.ss[0], ctfMercKit(a, "Gorvak"), MercAssist, r.at(back+200*time.Millisecond)); why != "" {
		t.Fatalf("back: %s", why)
	}
	if ms2 := r.w.mercs[a.ServerID]; ms2 == nil || !groupHas(g, ms2.npc.ServerID) {
		t.Fatalf("back, not in the raid: %v", g.Members)
	}
}
