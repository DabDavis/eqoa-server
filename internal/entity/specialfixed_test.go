package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// newFixedCritRig is critRig's brute on a template whose crit is a flat 850 (special_fixed, special_damage
// 0) ready every 10 s (work board #1632: "crits just do that amount", not a figure per level).
func newFixedCritRig(t *testing.T, level int) *critRig {
	t.Helper()
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, special, special_damage, special_fixed, special_every, special_first, special_jitter)
		VALUES ('flat', 0, 'Critical Hit', 0, 850, 10, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll brute", "flat")
	b := r.npc("Gnoll Brute")
	b.Level, b.HP, b.MaxHP = level, 100000, 100000
	b.OffMod, b.WeaponDamage = NPCOffMod(level), NPCWeaponDamage(level)
	return &critRig{templateRig: r, brute: b}
}

// A template with special_fixed 850 and special_damage 0 crits by rounds, and each crit lands for 850 at
// level 55 and at level 30 alike; #stats tells the 850.
func TestSpecialFixedLandsItsFigureAtAnyLevel(t *testing.T) {
	for _, level := range []int{55, 30} {
		r := newFixedCritRig(t, level)
		if r.brute.tmpl == nil || r.brute.tmpl.SpecialFixed != 850 {
			t.Fatalf("level %d: the brute's template %+v, want special_fixed 850", level, r.brute.tmpl)
		}
		if !r.brute.critRounds() {
			t.Fatalf("level %d: critRounds false for a special_fixed template", level)
		}
		p, sess := r.player("Hero", 2001.5, 100)
		p.AC = 1763
		r.fight(r.brute, p.ServerID)
		src := int64(r.objectID(r.brute.ServerID))
		var crits []int64
		for d := time.Duration(0); d < 13*time.Second; d += 100 * time.Millisecond {
			p.HP = p.MaxHP
			r.tick(d)
			msgs := drain(sess)
			crit := false
			for _, c := range castsIn(msgs) {
				if c.caster == uint32(src) && c.fx == templateSpecialFX {
					crit = true
				}
			}
			if !crit {
				continue
			}
			for _, f := range damageIndications(msgs) {
				if f[0] == src {
					crits = append(crits, -f[1])
				}
			}
		}
		if len(crits) != 2 || crits[0] != 850 || crits[1] != 850 {
			t.Errorf("level %d: crits for %v, want [850 850] (at 0.5 s and 12.5 s)", level, crits)
		}
		r.w.mu.RLock()
		stats := r.w.templateLineLocked(r.brute, r.at(13*time.Second))
		r.w.mu.RUnlock()
		if !strings.Contains(stats, "Critical Hit for 850 in place of a round") {
			t.Errorf("level %d: #stats %q does not tell the crit's 850", level, stats)
		}
	}
}

// specialFigure is special_fixed when set, else the level's special_damage, at least 1.
func TestSpecialFigure(t *testing.T) {
	fixed := &ClassTemplate{ClassTemplate: database.ClassTemplate{SpecialDamage: 7.4, SpecialFixed: 850}}
	byLevel := &ClassTemplate{ClassTemplate: database.ClassTemplate{SpecialDamage: 7.4}}
	none := &ClassTemplate{}
	for _, c := range []struct {
		t     *ClassTemplate
		level int
		want  int
		has   bool
	}{{fixed, 55, 850, true}, {fixed, 1, 850, true}, {byLevel, 40, 296, true}, {byLevel, 0, 1, true}, {none, 40, 1, false}} {
		if got := specialFigure(c.t, c.level); got != c.want {
			t.Errorf("specialFigure(%+v, %d) = %d, want %d", c.t.ClassTemplate, c.level, got, c.want)
		}
		if got := hasSpecial(c.t); got != c.has {
			t.Errorf("hasSpecial(%+v) = %v, want %v", c.t.ClassTemplate, got, c.has)
		}
	}
}

// A timed special with neither figure is refused; with special_fixed alone it stands; special_fixed is
// bounded as melee_fixed is.
func TestSpecialFixedValidate(t *testing.T) {
	base := database.ClassTemplate{Name: "x", Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100, Special: "Critical Hit", SpecialEvery: 10}
	if err := database.CheckClassTemplate(base); err == nil {
		t.Error("special_every 10 with special_damage and special_fixed 0 was taken")
	}
	ok := base
	ok.SpecialFixed = 850
	if err := database.CheckClassTemplate(ok); err != nil {
		t.Errorf("special_fixed 850 alone refused: %v", err)
	}
	chance := database.ClassTemplate{Name: "x", Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100, Special: "Kick", SpecialChance: 20, SpecialFixed: 400}
	if err := database.CheckClassTemplate(chance); err != nil {
		t.Errorf("a chance special of special_fixed 400 refused: %v", err)
	}
	for _, bad := range []int{-1, database.MaxTemplateShield + 1} {
		b := ok
		b.SpecialFixed = bad
		if err := database.CheckClassTemplate(b); err == nil {
			t.Errorf("special_fixed %d was taken", bad)
		}
	}
}
