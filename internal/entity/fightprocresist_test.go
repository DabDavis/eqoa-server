package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A proc an ability's landing gives can be cut by the ability's resist (work board #1637): it lands on a
// player as the template's own proc of that resist and figure does; one with no resist lands whole as before.

// An ability with a proc and a resist parses; a resist with no proc and no fixed damage is still refused.
func TestAFightProcsResistParses(t *testing.T) {
	ok := `[{"kind":"heal","heal_pct":100,"below":60,"every":6,"cooldown":600,"name":"Mend",` +
		`"proc_chance":10,"proc_damage":3000,"proc_name":"Searing","resist":"FR"}]`
	list, err := database.ParseTemplateAbilities(ok)
	if err != nil {
		t.Fatalf("a heal with a proc and a resist refused: %v", err)
	}
	if list[0].Resist != "FR" || list[0].ProcChance != 10 {
		t.Errorf("parsed %+v", list[0])
	}
	for _, bad := range []string{
		`[{"kind":"heal","heal_pct":100,"below":60,"every":6,"name":"Mend","resist":"FR"}]`,
		`[{"kind":"buff","every":30,"resist":"CR"}]`,
	} {
		if _, err := database.ParseTemplateAbilities(bad); err == nil {
			t.Errorf("a resist with no proc and no damage taken: %s", bad)
		}
	}
}

// fightProcRig is the template rig with a heal of its whole below half on the gnoll guard ("mender"),
// its landing giving a proc of 3000 in 10% of its hits, cut by FR when withResist; and on the gnoll brute
// ("searer") a template's own proc of 3000 in 10% of its hits cut by FR, the control.
func fightProcRig(t *testing.T, withResist bool) *templateRig {
	t.Helper()
	r := newTemplateRig(t)
	resist := ""
	if withResist {
		resist = `,"resist":"FR"`
	}
	abilities := `[{"kind":"heal","heal_pct":100,"below":50,"every":3600,"name":"Mend",` +
		`"proc_chance":10,"proc_damage":3000,"proc_name":"Searing"` + resist + `}]`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO classtemplates (name, class, abilities) VALUES ('mender', 0, ?)`, []any{abilities}},
		{`INSERT INTO classtemplates (name, class, proc_chance, proc_damage, proc_name, proc_resist) VALUES ('searer', 0, 10, 3000, 'Searing', 'FR')`, nil},
	} {
		if _, err := r.db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	r.assign("gnoll brute", "searer")
	if got := r.assign("gnoll guard", "mender"); got != 2 {
		t.Fatalf("%d creatures fight by the templates; problems %q", got, r.w.ClassTemplateProblems())
	}
	return r
}

// The guard's heal lands and gives it the proc for the fight; at a player's fire resist cap the proc
// lands as the brute's template proc of FR and 3000 does (a tenth), told the same; #stats says the resist.
// Without the resist the same proc lands whole at the cap, as before.
func TestAFightProcCutByItsResist(t *testing.T) {
	for _, withResist := range []bool{true, false} {
		defer func(f func(int) int) { templateIntn = f }(templateIntn)
		defer func(f func() float64) { resistRand = f }(resistRand)
		resistRand = func() float64 { return 0.5 }
		r := fightProcRig(t, withResist)
		guard, brute := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
		hero, sess := r.player("Hero", guard.X+1, guard.Z)
		hero.HP, hero.MaxHP = 10_000_000, 10_000_000
		r.fight(guard, hero.ServerID)
		templateIntn = func(int) int { return 99 } // no proc while it fights to its heal
		r.tick(0)
		guard.HP = guard.MaxHP*49/100 - 1
		r.tick(time.Second)
		r.tick(time.Second + castLand)
		fp := guard.fightProcNow()
		if fp == nil {
			t.Fatalf("resist %v: no proc after the heal landed (HP %d/%d)", withResist, guard.HP, guard.MaxHP)
		}
		if want := map[bool]string{true: "FR", false: ""}[withResist]; fp.resist != want {
			t.Errorf("the fight's proc's resist %q, want %q", fp.resist, want)
		}
		roll := func(n *NPC) (int, string) {
			templateIntn = func(int) int { return 0 }
			drain(sess)
			hero.HP = hero.MaxHP
			r.w.mu.Lock()
			r.w.creatureProcLocked(sess, hero, n, r.at(2*time.Second))
			r.w.mu.Unlock()
			return hero.MaxHP - hero.HP, strings.Join(linesOf(drain(sess)), "|")
		}
		hero.FireR = ResistCap
		got, gotLine := roll(guard)
		if withResist {
			want, wantLine := roll(brute)
			if want != 300 || got != want {
				t.Errorf("at the fire cap: the fight's proc %d, the template's %d; want both 300", got, want)
			}
			if !strings.Contains(gotLine, "Gnoll Guard's Searing hits you for 300 (your resist took 90%).") ||
				!strings.Contains(wantLine, "Gnoll Brute's Searing hits you for 300 (your resist took 90%).") {
				t.Errorf("their lines: %q and %q", gotLine, wantLine)
			}
		} else if got != 3000 {
			t.Errorf("a fight proc of no resist at the fire cap: %d, want 3000 whole", got)
		}
		r.w.mu.RLock()
		stats := r.w.templateLineLocked(guard, r.at(2*time.Second))
		r.w.mu.RUnlock()
		says := strings.Contains(stats, "then Searing for the fight: 10% of its melee hits 3000 more, cut by FR")
		if says != withResist || !strings.Contains(stats, "then Searing for the fight: 10% of its melee hits 3000 more") {
			t.Errorf("resist %v: #stats %q", withResist, stats)
		}
	}
}
