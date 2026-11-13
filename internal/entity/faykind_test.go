package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
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

// The user's figures and QuestMasters' (a fairy form of 15 DEX, a 380+70%Dex CR
// proc), as the name gives them, and how #stats says them.
func TestFaykindIsTheUsersFigures(t *testing.T) {
	wp, ok := WeaponProcOf(715031, "Staff of Faykind", "Its words say nothing of a proc.")
	if !ok {
		t.Fatal("no proc")
	}
	if wp.Kind != ProcForm || wp.Chance != 0 || wp.ChanceFrom != 20 || wp.ChanceMost != 45 || wp.Duration != 45*time.Second ||
		wp.DurationTo != 60*time.Second || wp.Damage != 380 || wp.DamageTo != 0 ||
		wp.Scale != StatDex || wp.ScalePct != 89 || wp.Resist != StatCR || wp.FormStat != StatDex || wp.FormStatBy != 15 ||
		wp.RoundMost != 800 || wp.FX != fxBuff {
		t.Errorf("%+v", wp)
	}
	if s := wp.String(); !strings.Contains(s, "Pixie Form (+15 DEX) for 45-60 s") ||
		!strings.Contains(s, "380 + 89% of DEX CR at 20% (+1% each 20 DEX, at most 45%) a hit, at most 800 a round") {
		t.Errorf("#stats says %q", s)
	}
	if s := wp.Explain(); !strings.Contains(s, "+15 DEX while it lasts") || !strings.Contains(s, "for 45-60 s") ||
		!strings.Contains(s, "380 + 89% of DEX cold at 20% (+1% each 20 DEX, at most 45%)") {
		t.Errorf("the panel says %q", s)
	}
}

// formProc is what the form's proc does in full on a hit of its bearer as it is now:
// 380 and 70% of its DEX (the form's 15 in it).
func (r *procRig) formProc() int { return 380 + r.friend.StatNow(StatDex)*89/100 }

// formChance is the staff's and its form's chance a hit at the bearer's DEX now (the owner, 2026-11-13: "let dex
// raise the chance"): 20% and one more each 20 DEX, at most 45%.
func (r *procRig) formChance() int { return dexChancePct(20, 45, r.friend.StatNow(StatDex)) }

// 30% a hit, whatever the wielder's INT (the rule by INT stops at 25): 29 of
// 100 lands it, 30 does not. It shows the pixies' body for 30 seconds, its
// name on the effects list, and no figure comes with it.
func TestFaykindGivesTheFormAtThirtyPercent(t *testing.T) {
	r := faykindRig(t)
	body, _ := r.friend.Body()
	dex := r.friend.StatNow(StatDex)
	c := r.formChance()
	if c <= 20 || c > 45 {
		t.Fatalf("the rig's DEX %d gives a chance of %d", dex, c)
	}
	procIntn = dice(c)
	r.hit()
	if now, _ := r.friend.Body(); now != body || len(r.friend.Effects) != 0 {
		t.Fatalf("a roll of %d: body %#x, effects %d", c, now, len(r.friend.Effects))
	}
	figures(r.fsess)
	procIntn = dice(c - 1)
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
	if left := e.Until.Sub(before); left < 45*time.Second-time.Second || left > 60*time.Second+time.Second {
		t.Errorf("it lasts %v, want 45-60 s", left)
	}
	if e.Name != "Pixie Form" || e.Proc != 380 || e.ProcMax != 380 || e.ProcScale != StatDex || e.ProcScalePct != 89 ||
		e.Rules.Resist != StatCR || e.Rules.ProcChanceFrom != 20 || e.Rules.ProcChanceMost != 45 || e.Rules.ProcRound != 800 {
		t.Errorf("%+v", e)
	}
	if now := r.friend.StatNow(StatDex); now != dex+15 {
		t.Errorf("DEX %d in the form, %d before: want 15 more", now, dex)
	}
	if len(r.friend.Buffs) != 1 || r.friend.Buffs[0].Name != "Pixie Form" {
		t.Errorf("effects list %+v", r.friend.Buffs)
	}
}

// A form on lasts 30 s unless the staff procs again, which renews it to 30 s
// from then (the user, 2026-11-08: "staff of faykind pixie form should only
// last 30 secs unless it procs again"), the same effect with its round's tally.
// The control: a roll over the chance leaves its time to run out.
func TestFaykindRenewsOnlyWhenItProcsAgain(t *testing.T) {
	r := faykindRig(t)
	e := r.giveForm(t)
	until := time.Now().Add(5 * time.Second)
	e.Until = until
	procIntn = dice(99) // the form's own proc and the staff's over their chance
	r.hit()
	if len(r.friend.Effects) != 1 || !r.friend.Effects[0].Until.Equal(until) {
		t.Fatalf("no proc: effects %d, its end moved %v", len(r.friend.Effects), r.friend.Effects[0].Until.Sub(until))
	}
	procIntn = dice(99, 0) // the form's own over its chance, the staff's under
	before := time.Now()
	r.hit()
	if len(r.friend.Effects) != 1 || r.friend.Effects[0] != e {
		t.Fatalf("a proc: effects %+v, want the same one", r.friend.Effects)
	}
	if left := e.Until.Sub(before); left < 44*time.Second || left > 61*time.Second {
		t.Errorf("a proc renewed it to %v, want 45-60 s", left)
	}
}

// In the form a hit procs at 30%, not by INT, 380 and 70% of the bearer's DEX, and
// no more than 800 in all in a round: the second proc is cut to what is left, the
// third does not happen, and a new round begins again. The controls: a roll of 30
// does not proc, and a resisted one does its least, 380 (no DEX), which counts
// against the round.
func TestFaykindFormProcsUpToEightHundredARound(t *testing.T) {
	r := faykindRig(t)
	r.giveForm(t)
	full := r.formProc()
	if full <= 420 {
		t.Fatalf("the rig's DEX gives %d: too little to reach the round's limit", full)
	}
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
	c := r.formChance()
	// chance, resist; then the staff's own roll is not made: the form is on
	if f := hit(c, 99); len(f) != 0 {
		t.Errorf("a roll of %d procs %v", c, f)
	}
	// resisted: the least it does, 380, which counts against the round
	if f := hit(c-1, 0); len(f) != 1 || f[0] != -380 {
		t.Errorf("a resisted one procs %v, want its least, 380", f)
	}
	if f := hit(c-1, 99); len(f) != 1 || f[0] != -420 {
		t.Errorf("second proc %v, want the 420 left of 800", f)
	}
	if f := hit(0, 99); len(f) != 0 {
		t.Errorf("third proc %v, want none", f)
	}
	r.friend.RoundStart = r.friend.RoundStart.Add(6 * time.Second)
	if f := hit(0, 99); len(f) != 1 || f[0] != -min(full, 800) {
		t.Errorf("a new round's proc %v, want %d", f, min(full, 800))
	}
}

// Its proc is cold: a creature strong in CR takes less of it (its template's
// resist_pct), and one strong in AR alone all of it.
func TestFaykindFormProcIsCold(t *testing.T) {
	for _, c := range []struct {
		kind string
		cut  bool
	}{{"CR", true}, {"AR", false}} {
		r := faykindRig(t)
		r.giveForm(t)
		r.npc.tmpl = &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "cold", Class: -1, HPPct: 100, TakenPct: 100,
			MeleePct: 100, ResistKind: c.kind, ResistPct: 50}}
		full := min(r.formProc(), 800)
		procIntn = dice(0, 99)
		r.hit()
		f := figures(r.fsess)
		want := -full
		if c.cut {
			want = -full / 2
		}
		if len(f) != 2 || f[1] < want-1 || f[1] > want+1 {
			t.Errorf("%s 50: figures %v, want the melee and %d", c.kind, f, want)
		}
	}
}

// The proc is the form's: with the staff out of the hand it goes on, and with
// the form gone (Remove Illusion) the hit is the melee alone.
func TestFaykindProcGoesWithTheForm(t *testing.T) {
	r := faykindRig(t)
	r.giveForm(t)
	r.friend.Gear.MainProc = nil
	want := min(r.formProc(), 800)
	procIntn = dice(0, 99)
	r.hit()
	if f := figures(r.fsess); len(f) != 2 || f[1] != -want {
		t.Errorf("without the staff: figures %v, want the melee and %d", f, want)
	}
	r.w.mu.Lock()
	r.w.endIllusionsLocked(r.friend, 0)
	r.w.mu.Unlock()
	if body, _ := r.friend.Body(); body == 0x94BA77CD {
		t.Error("the body stayed")
	}
	r.friend.RoundStart = r.friend.RoundStart.Add(6 * time.Second)
	procIntn = dice(0, 99)
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
	if e.Form != 0x94BA77CD || e.Proc != 380 || e.ProcScale != StatDex || e.ProcScalePct != 89 || e.Rules.Resist != StatCR ||
		e.Mods.Dex != 15 || e.Rules.ProcChanceFrom != 20 || e.Rules.ProcChanceMost != 45 || e.Rules.ProcRound != 800 {
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

// Petition 1371 (Uzii): the Steelwind Staff, the Staff of Faykind's figures in the Steelwind's look, procs the
// Faykind's pixie form; the control, another name, does not.
func TestSteelwindStaffProcsAsTheFaykind(t *testing.T) {
	want, ok := WeaponProcOf(715031, "Staff of Faykind", "")
	if !ok {
		t.Fatal("the Staff of Faykind has no proc")
	}
	got, ok := WeaponProcOf(990974, "Steelwind Staff", "A staff of old fay make, light as a reed.")
	if !ok || got.Kind != ProcForm || got.Form != want.Form || got.Damage != want.Damage || got.Chance != want.Chance {
		t.Fatalf("the Steelwind Staff's proc: %+v %v, want the Faykind's %+v", got, ok, want)
	}
	if _, ok := WeaponProcOf(990999, "Steelwind Club", "A club."); ok {
		t.Fatal("a weapon of no proc procced")
	}
}

// The owner, 2026-11-13: "let dex raise the chance" and "form lasts 45-60 secs". 20% at no DEX, one more each 20 DEX,
// at most 45%; a form's time drawn from 45 to 60 s at each proc, every second of it reachable.
func TestFaykindChanceByDexAndTimeRange(t *testing.T) {
	wp, _ := WeaponProcOf(715031, "Staff of Faykind", "")
	for dex, want := range map[int]int{0: 20, 199: 29, 200: 30, 475: 43, 600: 45, 1000: 45} {
		if got := wp.chancePct(dex); got != want {
			t.Errorf("DEX %d: %d%%, want %d%%", dex, got, want)
		}
	}
	defer func(f func(int) int) { procIntn = f }(procIntn)
	seen := map[time.Duration]bool{}
	for roll := 0; roll < 20; roll++ {
		procIntn = dice(roll)
		d := wp.formTime()
		if d < 45*time.Second || d > 60*time.Second {
			t.Fatalf("a roll of %d: %v", roll, d)
		}
		seen[d] = true
	}
	if !seen[45*time.Second] || !seen[60*time.Second] || len(seen) != 16 {
		t.Errorf("times reached: %d of 16 (45 s %v, 60 s %v)", len(seen), seen[45*time.Second], seen[60*time.Second])
	}
	// the control: a form of one time keeps it
	one := WeaponProc{Kind: ProcForm, Duration: 30 * time.Second}
	if d := one.formTime(); d != 30*time.Second {
		t.Errorf("a fixed form's time %v", d)
	}
}
