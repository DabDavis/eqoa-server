package entity

import (
	"strings"
	"testing"
	"time"
)

// faykindRig is a rig whose player holds the Staff of Faykind, and a round begun.
func faykindRig(t *testing.T) *procRig {
	t.Helper()
	r := weaponRig(t, "Staff of Faykind", "A staff of the fay.")
	t.Cleanup(func(f func(int) int) func() { return func() { procIntn = f } }(procIntn))
	r.friend.RoundStart = time.Now()
	return r
}

// giveForm lands the staff's form at once.
func (r *procRig) giveForm(t *testing.T) *ActiveEffect {
	t.Helper()
	procIntn = dice(0)
	r.hit()
	figures(r.fsess)
	for _, e := range r.friend.Effects {
		if e.Form != 0 {
			return e
		}
	}
	t.Fatal("no form landed")
	return nil
}

// The user's figures, as the name gives them, and how #stats says them.
func TestFaykindIsTheUsersFigures(t *testing.T) {
	wp, ok := WeaponProcOf(715031, "Staff of Faykind", "Its words say nothing of a proc.")
	if !ok {
		t.Fatal("no proc")
	}
	if wp.Kind != ProcForm || wp.Chance != 30 || wp.Duration != 10*time.Minute || wp.DamageTo != 800 || wp.RoundMost != 800 || wp.FX != fxBuff {
		t.Errorf("%+v", wp)
	}
	if s := wp.String(); !strings.Contains(s, "Pixie Form for 10 min") || !strings.Contains(s, "400-800 at 30% a hit, at most 800 a round") {
		t.Errorf("#stats says %q", s)
	}
}

// 30% a hit, whatever the wielder's INT (the rule by INT stops at 25): 29 of
// 100 lands it, 30 does not. It shows the pixies' body for ten minutes, its
// name on the effects list, and no figure comes with it.
func TestFaykindGivesTheFormAtThirtyPercent(t *testing.T) {
	r := faykindRig(t)
	body, _ := r.friend.Body()
	procIntn = dice(30)
	r.hit()
	if now, _ := r.friend.Body(); now != body || len(r.friend.Effects) != 0 {
		t.Fatalf("a roll of 30: body %#x, effects %d", now, len(r.friend.Effects))
	}
	figures(r.fsess)
	procIntn = dice(29)
	before := time.Now()
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("the form came with figures %v", f)
	}
	if now, _ := r.friend.Body(); now != 0x94BA77CD {
		t.Errorf("body %#x", now)
	}
	if len(r.friend.Effects) != 1 {
		t.Fatalf("effects %d", len(r.friend.Effects))
	}
	e := r.friend.Effects[0]
	if left := e.Until.Sub(before); left < 10*time.Minute-time.Second || left > 10*time.Minute+time.Second {
		t.Errorf("it lasts %v", left)
	}
	if e.Name != "Pixie Form" || e.Proc != 400 || e.ProcMax != 800 || e.Rules.ProcChance != 30 || e.Rules.ProcRound != 800 {
		t.Errorf("%+v", e)
	}
	if len(r.friend.Buffs) != 1 || r.friend.Buffs[0].Name != "Pixie Form" {
		t.Errorf("effects list %+v", r.friend.Buffs)
	}
}

// A form on is not begun again: the staff rolls no dice for it and its time
// runs on.
func TestFaykindDoesNotBeginAgain(t *testing.T) {
	r := faykindRig(t)
	e := r.giveForm(t)
	until := e.Until.Add(-time.Minute)
	e.Until = until
	procIntn = dice(99, 99) // the form's own proc over its chance
	r.hit()
	if len(r.friend.Effects) != 1 || !r.friend.Effects[0].Until.Equal(until) {
		t.Errorf("effects %d, until moved %v", len(r.friend.Effects), r.friend.Effects[0].Until.Sub(until))
	}
}

// In the form a hit procs at 30%, not by INT, from 400 to 800, and no more
// than 800 in all in a round: the second proc is cut to what is left, the third
// does not happen, and a new round begins again. The controls: a roll of 30 does
// not proc, and a resisted one does its least, 400, which counts against the round.
func TestFaykindFormProcsUpToEightHundredARound(t *testing.T) {
	r := faykindRig(t)
	r.giveForm(t)
	hit := func(rolls ...int) []int {
		t.Helper()
		procIntn = dice(rolls...)
		hp := r.npc.HP
		r.hit()
		f := figures(r.fsess)
		lost := 0
		for _, n := range f {
			lost -= n
		}
		if hp-r.npc.HP != lost {
			t.Errorf("figures %v, the creature lost %d", f, hp-r.npc.HP)
		}
		return f[1:] // the melee's first
	}
	// chance, resist, the roll above 400; then the staff's own roll is not made: the form is on
	if f := hit(30, 99, 0); len(f) != 0 {
		t.Errorf("a roll of 30 procs %v", f)
	}
	// resisted: the least it does, 400, which counts against the round
	if f := hit(29, 0); len(f) != 1 || f[0] != -400 {
		t.Errorf("a resisted one procs %v, want its least, 400", f)
	}
	if f := hit(29, 99, 200); len(f) != 1 || f[0] != -400 {
		t.Errorf("second proc %v, want the 400 left of 800", f)
	}
	if f := hit(0, 99, 400); len(f) != 0 {
		t.Errorf("third proc %v, want none", f)
	}
	r.friend.RoundStart = r.friend.RoundStart.Add(6 * time.Second)
	if f := hit(0, 99, 400); len(f) != 1 || f[0] != -800 {
		t.Errorf("a new round's proc %v, want 800", f)
	}
}

// The proc is the form's: with the staff out of the hand it goes on, and with
// the form gone (Remove Illusion) the hit is the melee alone.
func TestFaykindProcGoesWithTheForm(t *testing.T) {
	r := faykindRig(t)
	r.giveForm(t)
	r.friend.Gear.MainProc = nil
	procIntn = dice(0, 99, 0)
	r.hit()
	if f := figures(r.fsess); len(f) != 2 || f[1] != -400 {
		t.Errorf("without the staff: figures %v, want the melee and 400", f)
	}
	r.w.mu.Lock()
	r.w.endIllusionsLocked(r.friend, 0)
	r.w.mu.Unlock()
	if body, _ := r.friend.Body(); body == 0x94BA77CD {
		t.Error("the body stayed")
	}
	r.friend.RoundStart = r.friend.RoundStart.Add(6 * time.Second)
	procIntn = dice(0, 99, 0)
	r.hit()
	if f := figures(r.fsess); len(f) != 1 {
		t.Errorf("no form: figures %v, want the melee alone", f)
	}
}

// Kept over a logout with its chance, its limit and its body.
func TestFaykindFormIsKeptOverALogout(t *testing.T) {
	r := faykindRig(t)
	r.giveForm(t)
	now := time.Now()
	rows := savedEffects(r.friend, now)
	if len(rows) != 1 {
		t.Fatalf("rows %d", len(rows))
	}
	r.friend.Effects, r.friend.Buffs = nil, nil
	if n := r.w.RestoreEffects(r.fsess, rows, now); n != 1 {
		t.Fatalf("restored %d", n)
	}
	e := r.friend.Effects[0]
	if e.Form != 0x94BA77CD || e.Proc != 400 || e.ProcMax != 800 || e.Rules.ProcChance != 30 || e.Rules.ProcRound != 800 {
		t.Errorf("%+v", e)
	}
	if body, _ := r.friend.Body(); body != 0x94BA77CD {
		t.Errorf("body %#x", body)
	}
}

// Every other proc is as it was: by INT, and no limit in a round.
func TestOtherProcsHaveNoRoundLimit(t *testing.T) {
	var p Player
	p.RoundStart = time.Now()
	limited := &ActiveEffect{Rules: EffectRules{ProcRound: 800}}
	free := &ActiveEffect{}
	got := p.procsOfRound([]procRoll{{n: 700, ae: limited}, {n: 5000, ae: free}, {n: 700, ae: limited}, {n: 700, ae: limited}})
	want := []int{700, 5000, 100}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i].n != want[i] {
			t.Errorf("%d: %d, want %d", i, got[i].n, want[i])
		}
	}
}
