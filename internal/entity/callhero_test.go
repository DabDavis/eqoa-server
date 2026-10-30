package entity

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Call of the Hero (callhero.go; the owner's ruling of 2026-10-29: it
// summoned a player from anywhere "if the player accepted"): whom a call
// asks, and what each answer does. The gate fixture's wizard casts it; the
// spell, people and places are made up.

// beckoning is a made-up call as Call of the Hero's record has it: the
// group's scope, a long cast and an hour's recast.
func beckoning() CastSpell {
	return CastSpell{SpellID: 9801, DisplayIndex: 30, Level: 50, Scope: 2, Name: "Made-up Beckoning", CastTime: 5 * time.Second,
		Recast: time.Hour, PowerCost: 300, Range: 30, Def: ParseSpellNamed("Made-up Beckoning", "Summons a player to where you are", "")}
}

// calls casts the call with the hook recording whom it asks, and reports
// them.
func (f *gateFixture) calls() []HeroCall {
	f.t.Helper()
	var asked []HeroCall
	f.w.OnHeroCall(func(c HeroCall) { asked = append(asked, c) })
	if !f.cast(beckoning()) {
		f.t.Fatalf("the call did not begin")
	}
	f.w.runHeroCalls()
	return asked
}

// answer is a member's 0x0035 to a box, the travels it queues handed over.
func (f *gateFixture) answer(s *session.Session, box uint32, option byte, at time.Duration) string {
	line := f.w.AnswerHeroCall(s, box, option, f.t0.Add(at))
	f.w.runTravels()
	return line
}

// The words are read as a call, of no harm, nothing left unread; and its
// record's scope does not choose its targets: it is cast on its caster.
func TestTheCallIsRead(t *testing.T) {
	for _, line := range []string{"Summons a player to where you are", "Self, Summon Player", "Summons player"} {
		d := ParseSpellNamed("Made-up Beckoning", line, "")
		if len(d.Effects) != 1 || d.Effects[0].Kind != EffectCallHero || d.Harmful || len(d.Unread) != 0 || d.Summons() {
			t.Errorf("%q: %+v", line, d)
		}
	}
	if d := ParseSpellNamed("Made-up Imp", "Summons a pet", ""); !d.Summons() || d.callsHero() {
		t.Errorf("the control, a pet: %+v", d)
	}
}

// A call asks the members far from its caster, in its world or another,
// and no one at its side, dead, loading a world, or whose game cannot load
// the caster's world; the caster is told whom.
func TestACallAsksOnlyTheFarGroup(t *testing.T) {
	f := newGateFixture(t)
	f.member(1002, "Near", 1010, 50, 1000)
	f.member(1003, "Far", 6000, 50, 1000)
	yonder, _ := f.member(1004, "Yonder", 1000, 50, 1000)
	yonder.World = 2
	fallen, _ := f.member(1005, "Fallen", 6000, 50, 1000)
	fallen.HP = 0
	_, ls := f.member(1006, "Loading", 6000, 50, 1000)
	ls.HoldStateChannels(time.Minute)
	_, ds := f.member(1007, "Discless", 6000, 50, 1000)
	f.w.SetLoadableWorlds(func(s *session.Session) func(uint8) bool {
		return func(world uint8) bool { return s != ds }
	})
	asked := f.calls()
	var names []string
	for _, c := range asked {
		names = append(names, c.P.Name)
		if !IsHeroCallBox(c.Box) || c.Caster != "Wizard" || c.Spell != "Made-up Beckoning" || c.Sess == nil {
			t.Errorf("%s asked as %+v", c.P.Name, c)
		}
	}
	if !slices.Equal(names, []string{"Far", "Yonder"}) || asked[0].Box == asked[1].Box {
		t.Errorf("asked %v (%+v), want Far and Yonder, each its own box", names, asked)
	}
	if got := lines(f.sess); !slices.Contains(got, "You call Far, Yonder to your side.") {
		t.Errorf("the caster was told %q", got)
	}

	alone := newGateFixture(t)
	alone.member(1002, "Near", 1010, 50, 1000)
	if asked := alone.calls(); len(asked) != 0 || !slices.Contains(lines(alone.sess), "No one of your group is far enough to call.") {
		t.Errorf("no one far: asked %+v, told %q", asked, lines(alone.sess))
	}
}

// Yes lands the member beside the caster where it stands at the answer, as
// a group gate lands its members; No lands no one, nor does a Yes after it,
// one too late, or one from another session.
func TestACallLandsAMemberOnYesAlone(t *testing.T) {
	f := newGateFixture(t)
	_, fs := f.member(1003, "Far", 6000, 50, 1000)
	_, ys := f.member(1004, "Yonder", 7000, 50, 1000)
	_, ns := f.member(1005, "Late", 8000, 50, 1000)
	asked := f.calls()
	if len(asked) != 3 {
		t.Fatalf("asked %+v", asked)
	}
	box := map[string]uint32{}
	for _, c := range asked {
		box[c.P.Name] = c.Box
	}
	f.caster.X = 1200 // the caster walks on before the answer

	if line := f.answer(ys, box["Far"], 0, 10*time.Second); line != "" || len(f.sent) != 0 {
		t.Errorf("another's box answered: %q, travels %+v", line, f.sent)
	}
	if line := f.answer(fs, box["Far"], 255, 10*time.Second); line != "" || len(f.sent) != 0 {
		t.Errorf("a close: %q, travels %+v", line, f.sent)
	}
	if line := f.answer(fs, box["Far"], 0, 12*time.Second); line != "" || len(f.sent) != 1 {
		t.Fatalf("Yes: %q, travels %+v", line, f.sent)
	}
	tr := f.sent[0]
	if tr.P.Name != "Far" || tr.Sess != fs || tr.To.World != 0 || tr.To.X != 1200 || tr.To.Z != 1000 || tr.Ring != 1 || tr.Spell != "Made-up Beckoning" {
		t.Errorf("Yes lands %+v, want Far beside the wizard at (1200, 1000)", tr)
	}

	if line := f.answer(ys, box["Yonder"], 1, 12*time.Second); line != "" || len(f.sent) != 1 {
		t.Errorf("No: %q, travels %+v", line, f.sent[1:])
	}
	if line := f.answer(ys, box["Yonder"], 0, 13*time.Second); line != "" || len(f.sent) != 1 {
		t.Errorf("Yes after No: %q, travels %+v", line, f.sent[1:])
	}
	if line := f.answer(ns, box["Late"], 0, 5*time.Second+castLand+HeroCallWait+time.Second); line != heroCallPassed || len(f.sent) != 1 {
		t.Errorf("Yes too late: %q, travels %+v", line, f.sent[1:])
	}
}

// A caster gone by the answer calls no one there: the member is told the
// call has ended.
func TestACallEndsWithItsCaster(t *testing.T) {
	f := newGateFixture(t)
	_, fs := f.member(1003, "Far", 6000, 50, 1000)
	asked := f.calls()
	if len(asked) != 1 {
		t.Fatalf("asked %+v", asked)
	}
	f.caster.HP = 0
	if line := f.answer(fs, asked[0].Box, 0, 10*time.Second); line != heroCallEnded || len(f.sent) != 0 {
		t.Errorf("a dead caster's call: %q, travels %+v", line, f.sent)
	}
}

// A call's box is told from a duel's: their answers go to different hands.
func TestACallsBoxIsNoDuels(t *testing.T) {
	call := uint32(HeroCallBoxMark | 7)
	if !IsHeroCallBox(call) || IsDuelBox(call) {
		t.Errorf("a call's box %#x: call %v, duel %v", call, IsHeroCallBox(call), IsDuelBox(call))
	}
	for _, duel := range []uint32{DuelBoxID(7), DuelCancelBox} {
		if IsHeroCallBox(duel) || !IsDuelBox(duel) {
			t.Errorf("a duel's box %#x: call %v, duel %v", duel, IsHeroCallBox(duel), IsDuelBox(duel))
		}
	}
}
