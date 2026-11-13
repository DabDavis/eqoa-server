package entity

import (
	"strings"
	"testing"
	"time"
)

// The GM Jibrish's petition 1892: Mr. Moogie's ward at 15%: "a 500 a hit heal DS that heals him when hit, and ... a
// 5000 damage a hit additional damage shield that damages everyone with pure damage when hes hit". Over the
// template rig: above its share (20%) a hit is answered by nothing (the control); below it (14%) the ward comes
// up with its line, every hit that lands heals it 500 more than its hit heal (150) and is answered, at its next
// tick, with 5,000 on whoever landed it, through any resist: the player (a spell's hit, a melee hit, two hits two
// answers), the NPC that hit it. A killing hit is answered by nothing. Its fight's end takes the ward down.
func TestAWardBelowItsShare(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n / 2 }
	r.godTemplate("warded king", ", class", ", 0",
		`[{"kind":"hitheal","heal":150},{"kind":"ward","below":15,"heal":500,"damage":5000,"name":"Royal Ward","say":"Feel my wrath!"}]`,
		"gnoll guard")
	guard, brute := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
	pool(guard, 100_000)
	pool(brute, 1_000_000)
	p, sess := r.player("Hitter", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 100_000, 100_000
	p.FireR, p.ColdR, p.LightningR, p.ArcaneR, p.PoisonR, p.DiseaseR = 500, 500, 500, 500, 500, 500
	r.fight(guard, p.ServerID)
	phase := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.templatePhasesLocked(guard, r.at(d))
		r.w.mu.Unlock()
	}
	spell := func(n int) {
		r.w.mu.Lock()
		r.w.spellHitLocked(p, sess, guard, n, numberSpell)
		r.w.mu.Unlock()
	}

	// at 20%: no ward; a hit heals its hit heal alone, answered by nothing
	guard.HP = 20_000
	phase(0)
	spell(1000)
	phase(100 * time.Millisecond)
	if guard.HP != 20_000-1000+150 || p.HP != 100_000 {
		t.Fatalf("at 20%%: guard HP %d (want %d), player HP %d (want untouched)", guard.HP, 20_000-1000+150, p.HP)
	}
	drain(sess)

	// at 14%: the ward comes up, its line heard
	guard.HP = 14_000
	phase(200 * time.Millisecond)
	if guard.tfight == nil || guard.tfight.warded == nil {
		t.Fatal("at 14%: no ward")
	}
	said := false
	for _, l := range linesOf(drain(sess)) {
		said = said || strings.Contains(l, "Feel my wrath!")
	}
	if !said {
		t.Error("its line was not heard")
	}
	// a spell's hit: healed 650 in all, answered with 5,000 at its next tick, no resist taken off
	spell(1000)
	if guard.HP != 14_000-1000+650 {
		t.Errorf("a hit under the ward: guard HP %d, want %d (its 150 and the ward's 500)", guard.HP, 14_000-1000+650)
	}
	if p.HP != 100_000 {
		t.Errorf("the answer landed inside the hit: player HP %d", p.HP)
	}
	phase(300 * time.Millisecond)
	if p.HP != 95_000 {
		t.Errorf("the ward's answer: player HP %d, want 95000 (pure 5,000, its resists of 500 taking nothing)", p.HP)
	}
	answered := false
	for _, l := range linesOf(drain(sess)) {
		answered = answered || strings.Contains(l, "Gnoll Guard's Royal Ward hits you for 5000.")
	}
	if !answered {
		t.Error("the answer's line was not said")
	}
	// two melee hits: two answers
	r.w.mu.Lock()
	r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
	r.w.playerHitLocked(sess, p, guard, r.objectID(guard.ServerID))
	r.w.mu.Unlock()
	phase(400 * time.Millisecond)
	if p.HP != 85_000 {
		t.Errorf("two melee hits: player HP %d, want 85000", p.HP)
	}
	// an NPC's hit: the NPC answered
	r.w.mu.Lock()
	r.w.npcHitNPCLocked(brute, guard, r.at(500*time.Millisecond))
	r.w.mu.Unlock()
	phase(600 * time.Millisecond)
	if brute.HP != 1_000_000-5000 {
		t.Errorf("an NPC's hit: its HP %d, want %d", brute.HP, 1_000_000-5000)
	}
	// a killing hit: no answer
	guard.HP = 10
	spell(1000)
	phase(700 * time.Millisecond)
	if !guard.Dead || p.HP != 85_000 {
		t.Errorf("a killing hit: guard dead %v, player HP %d; want dead, no answer", guard.Dead, p.HP)
	}
	if w := wardWords(guard); !strings.Contains(w, "Royal Ward below 15%") {
		t.Errorf("#stats: %q", w)
	}
}
