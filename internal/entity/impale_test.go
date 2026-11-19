package entity

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 2189 (a L60 gnome ranger of 7,960 HP) and the
// owner, 2026-11-15: "Impale is killing players in 1 hit". Impale (L55, "iod
// impale": melee_pct 928) casts Impaling Gore (760163) every 30 s from 3 s in,
// "Decrease AC by 2500, 1min". Its 928% was fitted where AC 2-3k puts every
// roll under the floor (softHit's window, ~20 a hit); the Gore took a 2.5k
// tank to AC 0, where the roll lands over the floor, up to 7x the window's
// top, each of it times 9.28: on a player 246-1,578 a hit, a quad round up to
// 6,312 before the level's balance (175% at 55), 11,046 after. And an AC
// past what was worn went under 0, which raised every roll further (a wizard
// of 1,000 AC under it, at -1,500: 3,239 a hit before the balance, 5,668
// after).
//
// Now the Gore is AC -1,500 (tools/impale_gore_20261115.py) and a creature's
// AC debuff takes a player to AC 0 at most (templateDebuffLocked): on a player
// a quad round under it is at most 2,080 before the balance (3,640 after),
// against 6,312 (11,046) under the old Gore; without it, 796 (1,393), as
// before.
func TestImpaleUnderItsGoreIsHardNotAOneShot(t *testing.T) {
	impale := &NPC{Level: 55, tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "iod impale", MeleePct: 928}}}
	const balance55 = 1.75 // balancelevel's damage_pct at 55 (190 at 50, 160 at 60), the con even and the ranger 100%
	const quad = 4         // a L55 warrior's Attack4

	zerk := func(gore int) *Player {
		p := &Player{Level: 60, Class: 1, AC: 2520, Agi: 520}
		p.Mods.AC = gore
		return p
	}
	most := func(p *Player) int { _, hi, _, _ := hitsOver(impale, p); return hi }
	mean := func(p *Player) float64 { _, _, m, _ := hitsOver(impale, p); return m }

	plain, oldGore, newGore := zerk(0), zerk(-2500), zerk(-1500)
	t.Logf("Zorbaxes: a hit at most %d / mean %.0f plain, %d / %.0f under the old Gore, %d / %.0f under the new (before the balance's %.0f%%)",
		most(plain), mean(plain), most(oldGore), mean(oldGore), most(newGore), mean(newGore), balance55*100)

	if got := float64(quad*most(newGore)) * balance55; got >= 7960/2 {
		t.Errorf("under the new Gore a quad round reaches %.0f on a ranger of 7,960: want under half", got)
	}
	if most(newGore) <= most(plain) {
		t.Errorf("the new Gore does nothing: %d a hit at most under it, %d without", most(newGore), most(plain))
	}
	if float64(quad*most(oldGore))*balance55 < 7960 {
		t.Errorf("the old Gore's quad round %.0f: the case the petition reported is not reproduced", float64(quad*most(oldGore))*balance55)
	}
}

// A creature's AC debuff lands on a player down to AC 0 and no further: a
// wizard of AC 1,000 under a -2,500 debuff reads 0, and again at its next
// landing (its last share off first); a tank of AC 3,000 takes all of it (500
// left). The control: a player's own negative AC (a vampire's Bat Form, AC
// -963) is not touched: that is TestAVampiresFormSetsItsStat's.
func TestACreaturesACDebuffStopsAtZero(t *testing.T) {
	r := newTemplateRig(t)
	const gore = 7201
	if _, err := r.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
		eqprequire, spellname, spelldesc) VALUES (?, 50, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Impaling Gore', '')`, gore); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Target, Decrease AC by 2500, 1min', 99, 'test')`, gore); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES ('impaler', -1, ?)`,
		fmt.Sprintf(`[{"kind":"debuff","spell_id":%d,"every":30,"first":1}]`, gore)); err != nil {
		t.Fatal(err)
	}
	if got := r.assign("gnoll guard", "impaler"); got != 1 {
		t.Fatalf("%d creatures of the template", got)
	}
	guard := r.npc("Gnoll Guard")
	p, _ := r.player("Hero", guard.X+1, guard.Z)
	p.HP, p.MaxHP, p.AC = 100000, 100000, 1000
	r.fight(guard, p.ServerID)
	r.tick(0)
	r.tick(1 * time.Second)
	r.tick(1*time.Second + castLand)
	if !p.hasEffect(gore) {
		t.Fatal("the Gore did not land")
	}
	if got := p.StatNow(StatAC); got != 0 {
		t.Errorf("a wizard of AC 1,000 under the Gore: AC %d, want 0", got)
	}
	r.tick(31*time.Second + castLand) // its next landing
	if got := p.StatNow(StatAC); got != 0 {
		t.Errorf("at the Gore's next landing: AC %d, want 0", got)
	}

	p.AC = 3000
	r.tick(61*time.Second + castLand)
	if got := p.StatNow(StatAC); got != 500 {
		t.Errorf("a tank of AC 3,000 under the Gore: AC %d, want 500 (all of it)", got)
	}
}

// Impaling Gore's line as the tool writes it reads AC -1,500 for a minute, on
// its target.
func TestImpalingGoreLineReads(t *testing.T) {
	d := SpellDefFrom("Impaling Gore", "Target, Decrease AC by 1500, 1min", "", "")
	var ac int
	for _, e := range d.Effects {
		if e.Kind == EffectStat && e.Stat == StatAC {
			ac += e.Amount
		}
	}
	if ac != -1500 || d.Duration.Minutes() != 1 || !d.Harmful {
		t.Errorf("Impaling Gore reads AC %d for %v, harmful %v: want -1500 for 1m, harmful", ac, d.Duration, d.Harmful)
	}
}
