package entity

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A template's damage shield answers each melee hit on its creature: the figure
// off the attacker, cut by the resist it names (petition 1154: 500, FR). A bow's
// shot is not answered, and a creature with no shield answers nothing (the
// controls). The attacker can die of it, which ends its round.
func TestACreaturesDamageShieldAnswersAMeleeHit(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, hp_pct, shield_damage, shield_resist) VALUES ('flames', 0, 100, 500, 'FR')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll guard", "flames")
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }
	guard := r.npc("Gnoll Guard")
	guard.HP, guard.MaxHP = 1_000_000, 1_000_000
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	p.HP = 2000
	hit := func(n *NPC) bool {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return r.w.playerHitLocked(sess, p, n, r.objectID(n.ServerID))
	}
	lines := func() string { return strings.Join(linesOf(drain(sess)), "|") }

	lines()
	if hit(guard) || p.HP != 1500 {
		t.Fatalf("a melee hit: HP %d, want 1500", p.HP)
	}
	if got := lines(); !strings.Contains(got, "Gnoll Guard's damage shield hits you for 500 (your resist took 0%).") {
		t.Errorf("no line said it: %q", got)
	}
	p.FireR = ResistCap
	if hit(guard) || p.HP != 1375 {
		t.Errorf("with fire resist at the cap: HP %d, want 1375 (a quarter of 500)", p.HP)
	}
	p.FireR = 0

	p.Swing.Kind = swingRange1
	if before := p.HP; hit(guard) || p.HP != before {
		t.Errorf("a bow's shot was answered: HP %d, was %d", p.HP, before)
	}
	p.Swing.Kind = 0

	plain := r.npc("Gnoll Brute")
	plain.HP, plain.MaxHP = 1_000_000, 1_000_000
	if before := p.HP; hit(plain) || p.HP != before {
		t.Errorf("a creature with no shield answered: HP %d, was %d", p.HP, before)
	}

	p.HP = 100
	if !hit(guard) || !p.IsDead() {
		t.Errorf("a hit it dies of: dead %v, HP %d, the round not ended", p.IsDead(), p.HP)
	}
}

// A template's melee proc rolls each melee hit that lands, cut by its resist and told by name (board entry
// 230, petition 1161). The controls: a roll over its chance does nothing, and a creature with no proc rolls none.
func TestACreaturesMeleeProc(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, proc_chance, proc_damage, proc_name, proc_resist) VALUES ('dragon', 0, 30, 3000, 'Dragonfire', 'FR')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll guard", "dragon")
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	guard, plain := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
	p, sess := r.player("Hero", guard.X+1, guard.Z)
	roll := func(n *NPC, die int) (int, bool) {
		templateIntn = func(int) int { return die }
		drain(sess)
		before := p.HP
		r.w.mu.Lock()
		dead := r.w.creatureProcLocked(sess, p, n, r.at(0))
		r.w.mu.Unlock()
		return before - p.HP, dead
	}
	p.HP = 100000
	if got, _ := roll(guard, 30); got != 0 {
		t.Errorf("a roll of 30 on a 30%% chance: %d taken", got)
	}
	if got, _ := roll(guard, 29); got != 3000 {
		t.Errorf("a proc with no resist: %d, want 3000", got)
	}
	p.FireR = ResistCap
	if got, _ := roll(guard, 0); got != 750 {
		t.Errorf("a proc at the resist's cap: %d, want a quarter, 750", got)
	}
	if got := strings.Join(linesOf(drain(sess)), "|"); !strings.Contains(got, "Gnoll Guard's Dragonfire hits you for 750 (your resist took 75%).") {
		t.Errorf("its line: %q", got)
	}
	if got, _ := roll(plain, 0); got != 0 {
		t.Errorf("a creature with no proc: %d taken", got)
	}
	p.HP, p.FireR = 100, 0
	if _, dead := roll(guard, 0); !dead || !p.IsDead() {
		t.Errorf("a proc it dies of: reported %v, dead %v", dead, p.IsDead())
	}

	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 20, HP: 3500, MaxHP: 3500, X: guard.X + 1, Y: guard.Y, Z: guard.Z, Owner: p.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	templateIntn = func(int) int { return 0 }
	r.w.mu.Lock()
	first, second := r.w.creatureProcOnPetLocked(guard, pet, r.at(0)), r.w.creatureProcOnPetLocked(guard, pet, r.at(0))
	r.w.mu.Unlock()
	if first || !second || !pet.Dead {
		t.Errorf("on a pet of 3500: first %v, second %v, dead %v", first, second, pet.Dead)
	}
}

// A pet's melee hit is answered too, and it can die of it.
func TestACreaturesDamageShieldAnswersAPet(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, shield_damage, shield_resist) VALUES ('flames', 0, 500, 'FR')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll guard", "flames")
	guard := r.npc("Gnoll Guard")
	owner, _ := r.player("Owner", guard.X+5, guard.Z)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 20, HP: 700, MaxHP: 1000, X: guard.X + 1, Y: guard.Y, Z: guard.Z, Owner: owner.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	ps := &petState{npc: pet, owner: owner}
	answer := func() bool {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return r.w.creatureShieldOnPetLocked(ps, guard, r.at(0))
	}
	if answer() || pet.HP != 200 {
		t.Fatalf("the pet's hit: HP %d, want 200", pet.HP)
	}
	if !answer() || !pet.Dead {
		t.Errorf("the hit it dies of: dead %v, HP %d", pet.Dead, pet.HP)
	}
}

// A raid boss's template can reach a million hit points (petitions 1144, 1153);
// out of bounds, and an unknown resist, are refused.
func TestABossTemplatesBounds(t *testing.T) {
	ok := database.ClassTemplate{Name: "boss", Class: 0, HPPct: 6615, TakenPct: 100, MeleePct: 100, ShieldDamage: 500, ShieldResist: "FR"}
	if err := database.CheckClassTemplate(ok); err != nil {
		t.Errorf("a boss of 6,615%%: %v", err)
	}
	for name, bad := range map[string]func(*database.ClassTemplate){
		"hit points past the most": func(c *database.ClassTemplate) { c.HPPct = database.MaxTemplateHPPct + 1 },
		"hit points too few":       func(c *database.ClassTemplate) { c.HPPct = 9 },
		"a negative shield":        func(c *database.ClassTemplate) { c.ShieldDamage = -1 },
		"an unknown resist":        func(c *database.ClassTemplate) { c.ShieldResist = "XR" },
	} {
		c := ok
		bad(&c)
		if database.CheckClassTemplate(c) == nil {
			t.Errorf("%s was taken", name)
		}
	}
}
