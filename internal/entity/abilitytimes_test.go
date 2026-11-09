package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// What a creature's ability carries past one cast (abilitytimes.go; the GM Jibrish's petition 1394, Mr.
// Moogie's "1 hour cool down complete heal at 50% health, and if he uses it he says "Thats an hour cool
// down, you'll pay for that". After Lh he also gets a proc buff on melee attacks, 25% chance to proc
// additional melee damage that does 1500").

const (
	fxLayHands  = 0x0A0A0B01
	layHandsID  = 7100
	layHandsSay = "Thats an hour cool down, you'll pay for that"
	// a heal of his whole below half, on the fight's clock an hour and across fights an hour, his line,
	// and a proc for the rest of the fight
	layHandsAbilities = `[{"kind":"heal","spell_id":7100,"heal_pct":100,"below":50,"every":3600,"cooldown":3600,` +
		`"say":"Thats an hour cool down, you'll pay for that","proc_chance":25,"proc_damage":1500,"proc_name":"Royal Fury"}]`
	// the control: the same heal on its fight's clock alone (as the lava centipedes' "once an hour")
	fightClockAbilities = `[{"kind":"heal","spell_id":7100,"heal_pct":100,"below":50,"every":3600}]`
)

// layHandsRig is the template rig with the paladin's Lay Hands for the look, Mr. Moogie's heal on the
// gnoll guard and the control's on the gnoll brute.
func layHandsRig(t *testing.T) *templateRig {
	t.Helper()
	r := newTemplateRig(t)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (7100, 15, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Lay Hands', '')`,
		`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (7100, 'Target, Increase Hitpoints by 1', 168430337, 'test')`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for name, abilities := range map[string]string{"lion": layHandsAbilities, "centipede": fightClockAbilities} {
		if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities) VALUES (?, 0, ?)`, name, abilities); err != nil {
			t.Fatal(err)
		}
	}
	r.assign("gnoll brute", "centipede")
	if got := r.assign("gnoll guard", "lion"); got != 2 {
		t.Fatalf("%d creatures fight by the templates; problems %q", got, r.w.ClassTemplateProblems())
	}
	for _, p := range r.w.ClassTemplateProblems() {
		if strings.Contains(p, `"lion"`) || strings.Contains(p, `"centipede"`) {
			t.Fatalf("a problem of the test's templates: %s", p)
		}
	}
	return r
}

// refight sets n on foe again after the world has ticked, as a hit would, its sets told.
func (r *templateRig) refight(n *NPC, foe EntityID) {
	r.fight(n, foe)
	r.w.mu.Lock()
	r.w.wakeNPCLocked(n)
	r.w.mu.Unlock()
}

// endFight takes n's fight away (its foes gone) and runs its turn, which ends its fight by its template.
func (r *templateRig) endFight(n *NPC, d time.Duration) {
	r.w.mu.Lock()
	n.Threats, n.CurrentEnemy = nil, 0
	r.w.wakeNPCLocked(n)
	r.w.mu.Unlock()
	r.tick(d)
}

func layHandsCasts(r *templateRig, n *NPC, msgs [][]byte) int {
	c := 0
	for _, b := range castsIn(msgs) {
		if b.caster == r.objectID(n.ServerID) && b.fx == fxLayHands {
			c++
		}
	}
	return c
}

func countLines(lines []string, part string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, part) {
			n++
		}
	}
	return n
}

// Below half his hit points he lays hands on himself, whole, once: its cast with the spell's visual, his
// line in /say to everyone within a say's reach (not past it), and from then a proc for the rest of the
// fight (none before it). The heal waits out its hour across the fight's end: a new fight within it gets
// no heal and no proc, and after the hour he heals again. The control, the same heal on its fight's clock
// alone, heals again in its very next fight.
func TestALayHandsOnceAnHourAcrossFights(t *testing.T) {
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	r := layHandsRig(t)
	guard, brute := r.npc("Gnoll Guard"), r.npc("Gnoll Brute")
	hero, sess := r.player("Hero", guard.X+1, guard.Z)
	near, nsess := r.player("Near", guard.X+150, guard.Z)
	far, fsess := r.player("Far", guard.X+250, guard.Z)
	ctl, _ := r.player("Control", brute.X+1, brute.Z)
	for _, p := range []*Player{hero, near, far, ctl} {
		p.HP, p.MaxHP = 10_000_000, 10_000_000
	}
	r.fight(guard, hero.ServerID)
	r.fight(brute, ctl.ServerID)
	templateIntn = func(int) int { return 0 } // every proc there is goes off

	// whole, and a while below half's line: no heal, no proc
	for d := time.Duration(0); d <= 8*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	lines := linesOf(drain(sess))
	if n := countLines(lines, "Royal Fury"); n != 0 || guard.fightProcNow() != nil {
		t.Fatalf("a proc before the heal: %d lines, %+v", n, guard.fightProcNow())
	}
	guard.HP = guard.MaxHP / 2 // half is not below half
	r.tick(9 * time.Second)
	if got := layHandsCasts(r, guard, drain(sess)); got != 0 {
		t.Fatalf("a heal at half: %d casts", got)
	}

	// below half: the cast, then its landing
	guard.HP = guard.MaxHP*49/100 - 1
	brute.HP = brute.MaxHP*49/100 - 1
	r.tick(10 * time.Second)
	if got := layHandsCasts(r, guard, drain(sess)); got != 1 {
		t.Fatalf("its casts below half: %d, want one with Lay Hands' visual", got)
	}
	r.tick(10*time.Second + castLand)
	if guard.HP != guard.MaxHP || brute.HP != brute.MaxHP {
		t.Fatalf("after the landing: the guard %d/%d, the control %d/%d: want both whole", guard.HP, guard.MaxHP, brute.HP, brute.MaxHP)
	}
	say := "Gnoll Guard: " + layHandsSay
	if got := countLines(linesOf(drain(sess)), say); got != 1 {
		t.Errorf("the one it fights heard %d of %q", got, say)
	}
	if got := countLines(linesOf(drain(nsess)), say); got != 1 {
		t.Errorf("one 150 off heard %d", got)
	}
	if got := countLines(linesOf(drain(fsess)), say); got != 0 {
		t.Errorf("one 250 off heard %d", got)
	}
	if fp := guard.fightProcNow(); fp == nil || fp.chance != 25 || fp.damage != 1500 || fp.name != "Royal Fury" {
		t.Fatalf("its proc: %+v", fp)
	}
	if brute.fightProcNow() != nil {
		t.Error("the control has a proc it was never given")
	}

	// its melee from now on: each hit that lands procs (the dice at 0), 1,500 more
	before := hero.HP
	for d := 11 * time.Second; d <= 20*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	lines = linesOf(drain(sess))
	procs := countLines(lines, "Gnoll Guard's Royal Fury hits you for 1500.")
	if procs == 0 || before-hero.HP < 1500*procs {
		t.Fatalf("%d procs, %d taken: %q", procs, before-hero.HP, lines)
	}
	templateIntn = func(int) int { return 99 } // and none at the dice's top
	drain(sess)
	for d := 20 * time.Second; d <= 30*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if got := countLines(linesOf(drain(sess)), "Royal Fury"); got != 0 {
		t.Errorf("%d procs at the dice's top (25%% is below 25)", got)
	}

	// #stats says it all
	r.w.mu.RLock()
	stats := r.w.templateLineLocked(guard, r.at(30*time.Second))
	r.w.mu.RUnlock()
	for _, want := range []string{
		"heal Lay Hands (0s cast) of 100% of its hit points below 50% every 1h0m0s, once in 1h0m0s across fights (again in 59m40s)",
		`saying "Thats an hour cool down, you'll pay for that", then Royal Fury for the fight: 25% of its melee hits 1500 more`,
		"Royal Fury on for this fight: 25% of its melee hits 1500 more",
	} {
		if !strings.Contains(stats, want) {
			t.Errorf("#stats lacks %q:\n%s", want, stats)
		}
	}

	// the fight ends: the proc goes, the hour runs on
	r.endFight(guard, 31*time.Second)
	r.endFight(brute, 31*time.Second)
	if guard.fightProcNow() != nil {
		t.Error("the proc outlived its fight")
	}
	r.refight(guard, hero.ServerID)
	r.refight(brute, ctl.ServerID)
	templateIntn = func(int) int { return 0 }
	r.tick(20 * time.Minute)
	guard.HP = guard.MaxHP / 4
	brute.HP = brute.MaxHP / 4
	drain(sess)
	drain(nsess)
	for d := 20 * time.Minute; d <= 20*time.Minute+10*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	msgs := drain(sess)
	if got := layHandsCasts(r, guard, msgs); got != 0 || guard.HP > guard.MaxHP/4 {
		t.Errorf("a second heal inside the hour, in a new fight: %d casts, HP %d/%d", got, guard.HP, guard.MaxHP)
	}
	if got := countLines(linesOf(msgs), "Royal Fury"); got != 0 || guard.fightProcNow() != nil {
		t.Errorf("a new fight's proc with no heal: %d lines", got)
	}
	if got := countLines(linesOf(drain(nsess)), say); got != 0 {
		t.Errorf("his line again with no heal: %d", got)
	}
	if brute.HP != brute.MaxHP {
		t.Errorf("the control on its fight's clock: %d/%d, want healed again in its new fight", brute.HP, brute.MaxHP)
	}

	// the hour over: he heals again, says it again, and procs again
	d := 10*time.Second + time.Hour + time.Second
	r.tick(d)
	r.tick(d + castLand)
	if guard.HP != guard.MaxHP || guard.fightProcNow() == nil {
		t.Errorf("after the hour: HP %d/%d, proc %+v", guard.HP, guard.MaxHP, guard.fightProcNow())
	}
	if got := countLines(linesOf(drain(nsess)), say); got != 1 {
		t.Errorf("his line after the hour: heard %d", got)
	}
}

// A restart that keeps the sessions carries a cooldown still running, and only one: a creature on its
// spawn at full health with nothing else to keep is written for it, through the file's JSON, and back in
// the new world the heal still waits out its hour; one run out is not written.
func TestALayHandsCooldownOutlivesARestart(t *testing.T) {
	old := layHandsRig(t)
	guard := old.npc("Gnoll Guard")
	key := abilityKey(&old.w.templates.byName["lion"].abilities[0].TemplateAbility)
	ready := time.Now().Add(42 * time.Minute).Truncate(time.Second)
	guard.abilityReady = map[string]time.Time{key: ready}
	kept := keptOf(keepFile(t, old.w.KeepNPCs()), guard.ServerID)
	if len(kept) != 1 || !kept[0].Cooldowns[key].Equal(ready) || kept[0].Place != nil || kept[0].MaxHP != 0 {
		t.Fatalf("kept: %+v", kept)
	}

	r := layHandsRig(t)
	if got := r.w.RestoreNPCs(kept); got.Back != 1 || got.Cooldown != 1 {
		t.Fatalf("restored: %v", got)
	}
	g := r.npc("Gnoll Guard")
	if !g.abilityReady[key].Equal(ready) {
		t.Fatalf("its cooldown after the restart: %v, want %v", g.abilityReady, ready)
	}
	hero, sess := r.player("Hero", g.X+1, g.Z)
	hero.HP, hero.MaxHP = 10_000_000, 10_000_000
	r.fight(g, hero.ServerID)
	g.HP = g.MaxHP / 4
	now := ready.Add(-time.Minute) // still inside it
	r.w.mu.Lock()
	r.w.aiSystem(now)
	r.w.templateSystem(now.Add(castLand))
	r.w.mu.Unlock()
	if got := layHandsCasts(r, g, drain(sess)); got != 0 || g.HP != g.MaxHP/4 {
		t.Errorf("a heal inside the kept hour: %d casts, HP %d/%d", got, g.HP, g.MaxHP)
	}
	r.w.mu.Lock()
	r.w.aiSystem(ready.Add(time.Second))
	r.w.templateSystem(ready.Add(time.Second + castLand))
	r.w.mu.Unlock()
	if g.HP != g.MaxHP {
		t.Errorf("past the kept hour: HP %d/%d, want whole", g.HP, g.MaxHP)
	}

	// one run out is not written: the creature has nothing else to keep
	guard.abilityReady[key] = time.Now().Add(-time.Minute)
	if got := keptOf(old.w.KeepNPCs(), guard.ServerID); len(got) != 0 {
		t.Errorf("a cooldown run out kept: %+v", got)
	}
}

// The fight's proc lands on a pet as on a player, whole; a creature's heal of its share that pins no spell
// goes out under its name with the heal's stand-in.
func TestAFightsProcOnAPetAndAShareHealOfNoSpell(t *testing.T) {
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	r := layHandsRig(t)
	guard := r.npc("Gnoll Guard")
	owner, _ := r.player("Owner", guard.X+20, guard.Z)
	pet := &NPC{ServerID: 3000, Name: "skeleton", Level: 20, HP: 100000, MaxHP: 100000, X: guard.X + 1.5, Y: guard.Y, Z: guard.Z, Owner: owner.ServerID}
	r.w.NPCs[pet.ServerID] = pet
	r.w.SpatialIndex.Insert(pet.ServerID, pet.X, pet.Z)
	templateIntn = func(int) int { return 0 }
	r.w.mu.Lock()
	guard.fight().on = true
	guard.fight().proc = &fightProc{chance: 25, damage: 1500, name: "Royal Fury"}
	died := r.w.creatureProcOnPetLocked(guard, pet, r.at(0))
	r.w.mu.Unlock()
	if died || pet.HP != 100000-1500 {
		t.Errorf("the pet: died %v, HP %d, want 1,500 off", died, pet.HP)
	}
	s := shareHealSpell(database.TemplateAbility{Kind: database.AbilityHeal, HealPct: 30})
	if s.Name != database.DefaultShareHealName || s.visual() != fxHeal || s.CastTime != 0 {
		t.Errorf("a share heal of no spell: %q, visual %#x, %v", s.Name, s.visual(), s.CastTime)
	}
	if s := shareHealSpell(database.TemplateAbility{Kind: database.AbilityHeal, HealPct: 30, Name: "Mend"}); s.Name != "Mend" {
		t.Errorf("named: %q", s.Name)
	}
}
