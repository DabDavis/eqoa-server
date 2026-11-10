package entity

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// The raid gods' fights of 2026-11-10 (the GM Jibrish's petitions 1437-1443 and the owner's asks for Tunare
// and Innoruuk), over the template rig's creatures (newTemplateRig): a scripted fight fought back by the
// whole template, preferred foes, a fixed tap and a heal at every hit, a heal at a share, a summon's adds
// at a share of their creature, an enrage, an attack from out of reach, a creature's damage over time,
// procs and veil on a creature, and pets dying with their masters.

// godTemplate writes a classtemplates row (its columns and values as given, the abilities a JSON list),
// and gives it to the NPCs of each name.
func (r *templateRig) godTemplate(name, cols, vals, abilities string, npcs ...string) {
	r.t.Helper()
	q := fmt.Sprintf(`INSERT INTO classtemplates (name, abilities%s) VALUES (?, ?%s)`, cols, vals)
	if _, err := r.db.Exec(q, name, abilities); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	for _, n := range npcs {
		r.assign(n, name)
	}
	for _, p := range r.w.ClassTemplateProblems() {
		if strings.Contains(p, fmt.Sprintf("%q", name)) {
			r.t.Fatalf("a problem of the test's template: %s", p)
		}
	}
}

// place moves n (and its spawn) to (x, z).
func (r *templateRig) place(n *NPC, x, z float32) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.SpatialIndex.Update(n.ServerID, n.X, n.Z, x, z)
	n.X, n.Z, n.SpawnX, n.SpawnZ = x, z, x, z
}

// godTick runs a whole tick's fighting at d: the fights, the templates' landings, the adds and the
// effects.
func (r *templateRig) godTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(d)
	r.w.TickCount++
	r.w.aiSystem(now)
	r.w.templateSystem(now)
	r.w.addsSystem(now)
	r.w.effectSystem(now)
}

// run ticks from a to b every 100 ms.
func (r *templateRig) run(a, b time.Duration) {
	for d := a; d <= b; d += 100 * time.Millisecond {
		r.godTick(d)
	}
}

// pool gives n a pool of hit points, whole.
func pool(n *NPC, hp int) {
	n.HP, n.MaxHP = hp, hp
}

// The GM Jibrish's petition 1437: Mr. Moogie's Lay Hands never fired against the gods set on him, for the
// NPC a trigger's sic set on another fought it back by its melee alone. Now the NPC set on (the guard,
// "Moogie") fights the one set on it with casts (the brute, "Tunare") by its whole template: its kick lands
// on it, and below half it lays hands on itself once, whole, a fight's proc of 350 on every hit after (100%),
// its cooldown of 15 minutes kept across fights. The control: a creature of the same template set upon
// without casts (the priest, by the pyro) answers with its melee alone: no kick, no heal.
func TestAScriptedFightIsFoughtBackByTheWholeTemplate(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 }
	r.godTemplate("king", ", class, melee_pct, kick_damage, kick_every, kick_first", ", 0, 10, 1000, 2, 0",
		`[{"kind":"heal","heal_pct":100,"below":50,"every":900,"cooldown":900,"name":"Lay Hands","proc_chance":100,"proc_damage":350,"proc_name":"Royal Fury"}]`,
		"gnoll guard", "gnoll priest")
	guard, brute, priest, pyro := r.npc("Gnoll Guard"), r.npc("Gnoll Brute"), r.npc("Gnoll Priest"), r.npc("Gnoll Pyro")
	r.place(brute, guard.X+3, guard.Z)
	r.place(pyro, priest.X+3, priest.Z)
	for _, n := range []*NPC{guard, brute, priest, pyro} {
		pool(n, 1_000_000)
	}
	if _, ok := r.w.SetNPCOn(brute.ServerID, "gnoll guard", 30, true); !ok {
		t.Fatal("no sic of the brute on the guard")
	}
	if _, ok := r.w.SetNPCOn(pyro.ServerID, "gnoll priest", 30, false); !ok {
		t.Fatal("no sic of the pyro on the priest")
	}
	r.run(0, 10*time.Second)
	if lost := brute.MaxHP - brute.HP; lost < 4000 {
		t.Errorf("the guard's answer to the brute set on it with casts: %d lost, want its kicks of 1000 every 2 s", lost)
	}
	if lost := pyro.MaxHP - pyro.HP; lost >= 1000 || lost == 0 {
		t.Errorf("the control, set on without casts: %d lost, want its melee alone (under a kick)", lost)
	}
	if guard.CurrentEnemy != brute.ServerID || priest.CurrentEnemy != pyro.ServerID {
		t.Fatalf("the fights: guard on %d (want %d), priest on %d (want %d)", guard.CurrentEnemy, brute.ServerID, priest.CurrentEnemy, pyro.ServerID)
	}

	// below half: Lay Hands, whole, and the fight's proc from then; not the control's
	guard.HP, priest.HP = guard.MaxHP*40/100, priest.MaxHP*40/100
	r.run(10*time.Second+100*time.Millisecond, 13*time.Second)
	if guard.HP < guard.MaxHP*95/100 {
		t.Fatalf("below half in the scripted fight: HP %d of %d, want laid hands on, whole", guard.HP, guard.MaxHP)
	}
	if priest.HP > priest.MaxHP*45/100 || priest.fightProcNow() != nil {
		t.Errorf("the control healed itself (HP %d of %d) or took a proc", priest.HP, priest.MaxHP)
	}
	fp := guard.fightProcNow()
	if fp == nil || fp.chance != 100 || fp.damage != 350 || fp.name != "Royal Fury" {
		t.Fatalf("the proc after Lay Hands: %+v, want Royal Fury, 100%% for 350", fp)
	}
	before := brute.HP
	r.w.mu.Lock()
	r.w.creatureProcOnNPCLocked(guard, brute, 10, r.at(13*time.Second))
	r.w.mu.Unlock()
	if got := before - brute.HP; got != 350 {
		t.Errorf("a hit of the guard's on the brute procs %d, want Royal Fury's 350", got)
	}

	// a fight's end and a new one within the 15 minutes: no Lay Hands; after them, again
	r.endFight(guard, 14*time.Second)
	r.endFight(brute, 14*time.Second)
	r.w.SetNPCOn(brute.ServerID, "gnoll guard", 30, true)
	guard.HP = guard.MaxHP * 40 / 100
	r.run(60*time.Second, 70*time.Second)
	if guard.HP > guard.MaxHP*45/100 {
		t.Errorf("a second fight within its 15 minutes: healed to %d of %d", guard.HP, guard.MaxHP)
	}
	r.run(14*time.Second+900*time.Second, 14*time.Second+905*time.Second)
	if guard.HP < guard.MaxHP*95/100 {
		t.Errorf("past its 15 minutes: HP %d of %d, want laid hands on again", guard.HP, guard.MaxHP)
	}
}

// The GM Jibrish's petition 1438: a creature fights its preferred foes first whenever one is on its hate
// list, whatever the rest's hate, and a taunt does not turn it from one; with none of them on the list
// (the control) it fights its highest threat, as ever.
func TestAPreferredFoeIsFoughtFirst(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("devoted", ", class", ", 0", `[{"kind":"prefer","foes":["Gnoll Brute"]}]`, "gnoll guard")
	guard, brute, mage := r.npc("Gnoll Guard"), r.npc("Gnoll Brute"), r.npc("Gnoll Mage")
	r.place(brute, guard.X+3, guard.Z)
	p, _ := r.player("Tank", guard.X+1, guard.Z)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.addThreatLocked(guard, p.ServerID, 1000)
	if guard.CurrentEnemy != p.ServerID {
		t.Fatalf("the control, no preferred foe on its list: fighting %d, want the tank %d", guard.CurrentEnemy, p.ServerID)
	}
	r.w.addThreatLocked(guard, brute.ServerID, 1)
	if guard.CurrentEnemy != brute.ServerID {
		t.Fatalf("its preferred foe on its list (hate 1 against 1000): fighting %d, want the brute %d", guard.CurrentEnemy, brute.ServerID)
	}
	r.w.addThreatLocked(guard, p.ServerID, 50_000)
	if guard.CurrentEnemy != brute.ServerID {
		t.Errorf("more of the tank's hate turned it to %d", guard.CurrentEnemy)
	}
	if got := r.w.tauntedLocked(guard, mage.ServerID); got != brute.ServerID {
		t.Errorf("a taunt left it on %d, want its preferred foe", got)
	}
	r.w.dropAggroLocked(brute.ServerID)
	if guard.CurrentEnemy != p.ServerID {
		t.Errorf("its preferred foe off the list: fighting %d, want the tank %d", guard.CurrentEnemy, p.ServerID)
	}
	if got := r.w.tauntedLocked(guard, mage.ServerID); got != mage.ServerID {
		t.Errorf("the control: a taunt with no preferred foe left it on %d, want the taunter", got)
	}
}

// The GM Jibrish's petition 1439: a lifetap of a fixed 4,000 every 30 s, its look of its own name and
// visual, all of it on the player it fights and all of it back on the creature; the next not before its
// 30 s.
func TestAFixedLifetap(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("siphoner", ", class, melee_pct", ", 0, 10",
		`[{"kind":"lifetap","name":"Royal Siphon","fx":2943530132,"damage":4000,"every":30,"cooldown":30}]`, "gnoll guard")
	guard := r.npc("Gnoll Guard")
	pool(guard, 100_000)
	guard.HP = 80_000
	p, sess := r.player("Tank", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 50_000, 50_000
	r.fight(guard, p.ServerID)
	r.godTick(0)
	casts := castsIn(drain(sess))
	if len(casts) == 0 || casts[len(casts)-1].fx != 2943530132 {
		t.Fatalf("its cast: %+v, want Royal Siphon's own visual", casts)
	}
	ph, gh := p.HP, guard.HP
	r.w.mu.Lock()
	r.w.templateSystem(r.at(castLand))
	r.w.mu.Unlock()
	if ph-p.HP != 4000 || guard.HP-gh != 4000 {
		t.Errorf("its landing: the tank lost %d, the creature gained %d; want 4000 both", ph-p.HP, guard.HP-gh)
	}
	if lines := linesOf(drain(sess)); !hasLine(lines, "Gnoll Guard's Royal Siphon hits you for 4000.") {
		t.Errorf("the tank told %q", lines)
	}
	taps := 0
	for d := castLand + 100*time.Millisecond; d <= 35*time.Second; d += 100 * time.Millisecond {
		r.godTick(d)
		for _, c := range castsIn(drain(sess)) {
			if c.fx == 2943530132 {
				taps++
				if d < 30*time.Second {
					t.Fatalf("a second tap at %v, before its 30 s", d)
				}
			}
		}
	}
	if taps != 1 {
		t.Errorf("%d taps from 1.5 s to 35 s, want the one at 30 s", taps)
	}
}

// The GM Jibrish's petition 1439: a heal of 150 at every hit that lands on the creature, the hit itself
// landing whole: a player's spell hit, its damage over time's tick, its melee, an NPC's melee. Not a hit
// that kills it. The control: a creature of no such template (the pup, the necro's level) takes the same
// hits whole, no heal.
func TestAHealAtEveryHit(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n / 2 }
	r.godTemplate("healing hide", ", class", ", 0", `[{"kind":"hitheal","heal":150}]`, "gnoll necro")
	necro, pup, brute := r.npc("Gnoll Necro"), r.npc("Gnoll Pup"), r.npc("Gnoll Brute")
	p, sess := r.player("Hitter", necro.X+1, necro.Z)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	now := r.at(0)
	for name, hit := range map[string]func(n *NPC){
		"a spell's hit": func(n *NPC) { r.w.spellHitLocked(p, sess, n, 500, numberSpell) },
		"a damage over time's tick": func(n *NPC) {
			ae := &ActiveEffect{SpellID: 4242, Name: "Test Rot", Caster: p.ServerID, Until: now.Add(time.Minute), NextTick: now, HPTick: -300}
			n.Effects = []*ActiveEffect{ae}
			r.w.tickNPCEffectLocked(n, ae, now)
			n.Effects = nil
		},
		"a player's melee": func(n *NPC) { r.w.playerHitLocked(sess, p, n, r.objectID(n.ServerID)) },
		"an NPC's melee":   func(n *NPC) { r.w.npcHitNPCLocked(brute, n, now) },
	} {
		pool(necro, 100_000)
		pool(pup, 100_000)
		necro.HP, pup.HP = 90_000, 90_000
		hit(necro)
		hit(pup)
		lostN, lostP := 90_000-necro.HP, 90_000-pup.HP
		if lostP <= 0 || lostN != lostP-150 {
			t.Errorf("%s: the template's creature lost %d, the control %d; want the control's less 150", name, lostN, lostP)
		}
	}
	necro.HP = 100
	r.w.spellHitLocked(p, sess, necro, 500, numberSpell)
	if !necro.Dead || necro.HP != 0 {
		t.Errorf("a killing hit: dead %v, HP %d; want dead, no heal", necro.Dead, necro.HP)
	}
}

// The owner's Tunare: a full heal below 20%, every and cooldown 30 minutes, first in her list, so it wins
// over her 50% heal when both are due; above 20% (the control) her 50% heal alone, and within its 30 minutes
// no second full heal however low she is.
func TestAFullHealAtTwentyPercent(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("goddess", ", class, melee_pct", ", 9, 10",
		`[{"kind":"heal","heal_pct":100,"below":20,"every":1800,"cooldown":1800,"name":"Tunare's Renewal","fx":3266773242},`+
			`{"kind":"heal","spell":"Healing","every":20,"below":50,"damage_pct":1000}]`, "gnoll guard")
	guard := r.npc("Gnoll Guard")
	pool(guard, 100_000)
	p, sess := r.player("Tank", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 1_000_000, 1_000_000
	r.fight(guard, p.ServerID)
	renewals := func(msgs [][]byte) int {
		n := 0
		for _, c := range castsIn(msgs) {
			if c.fx == 3266773242 {
				n++
			}
		}
		return n
	}
	guard.HP = 30_000
	r.run(0, 4*time.Second) // Healing's 2 s cast and its landing
	if n := renewals(drain(sess)); n != 0 || guard.HP != 30_000+1250 {
		t.Fatalf("at 30%%: %d renewals, HP %d; want her 50%% heal alone (Healing at 1000%%: +1250)", n, guard.HP)
	}
	guard.HP = 21_000
	r.run(4100*time.Millisecond, 6*time.Second)
	if n := renewals(drain(sess)); n != 0 {
		t.Fatalf("at 21%%: %d renewals, want none", n)
	}
	guard.HP = 19_000
	r.run(30*time.Second, 33*time.Second)
	if n := renewals(drain(sess)); n != 1 || guard.HP != guard.MaxHP {
		t.Fatalf("at 19%%: %d renewals, HP %d of %d; want one, whole", n, guard.HP, guard.MaxHP)
	}
	guard.HP = 10_000
	r.run(60*time.Second, 70*time.Second)
	if n := renewals(drain(sess)); n != 0 || guard.HP >= guard.MaxHP {
		t.Errorf("within its 30 minutes: %d renewals, HP %d of %d; want none", n, guard.HP, guard.MaxHP)
	}
}

// The GM Jibrish's petition 1440: at 25% the creature calls three adds, once a fight: none above 25%,
// three below it, not six however long it stays there; each its creature's template at half: half its most
// hit points, its kick at half, its special (a level's) at half of its creature's level's, its melee on the
// same target half of its creature's; the adds call none of their own. They join its fight on its foes. Its
// death takes them with it, laid down dead, corpses of nothing; a fight that ends any other way takes them
// from the world, and the next fight may call them again.
func TestASummonCallsThreeAddsAtHalf(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n / 2 }
	r.godTemplate("summoner", ", class, hp_pct, melee_pct, kick_damage, kick_every, special, special_damage, special_every",
		", 0, 200, 400, 4000, 5, 'Crit', 100, 10",
		`[{"kind":"summon","below":25,"count":3,"npc":"Gnoll Pup","share":50},{"kind":"hitheal","heal":150}]`, "gnoll guard")
	guard := r.npc("Gnoll Guard")
	p, _ := r.player("Tank", guard.X+1, guard.Z)
	p.HP, p.MaxHP = 10_000_000, 10_000_000
	adds := func() []*NPC {
		var out []*NPC
		for _, n := range r.w.NPCs {
			if n.add != nil && n.add.by == guard && !n.Dead {
				out = append(out, n)
			}
		}
		return out
	}
	r.fight(guard, p.ServerID)
	guard.HP = guard.MaxHP*26/100 + 1
	r.run(0, time.Second)
	if n := len(adds()); n != 0 {
		t.Fatalf("at 26%%: %d adds, want none", n)
	}
	guard.HP = guard.MaxHP * 24 / 100
	r.run(1100*time.Millisecond, 3*time.Second)
	got := adds()
	if len(got) != 3 {
		t.Fatalf("below 25%%: %d adds, want 3", len(got))
	}
	a := got[0]
	if want := int(math.Round(float64(guard.MaxHP) / 2)); a.MaxHP != want || a.tmpl != guard.tmpl || a.Name != "Gnoll Pup" || a.Level != 20 {
		t.Errorf("an add: %q level %d, %d hit points (want %d), its creature's template %v", a.Name, a.Level, a.MaxHP, want, a.tmpl == guard.tmpl)
	}
	if a.CurrentEnemy != p.ServerID {
		t.Errorf("an add fights %d, want its creature's foe %d", a.CurrentEnemy, p.ServerID)
	}
	if k := a.shared(guard.tmpl.KickDamage); k != 2000 {
		t.Errorf("an add's kick: %d, want 2000", k)
	}
	if got, want := a.shared(int(math.Round(float64(a.figureLevel())*guard.tmpl.SpecialDamage))), 30*100/2; got != want {
		t.Errorf("an add's crit: %d, want half its creature's level 30 one, %d", got, want)
	}
	r.w.mu.Lock()
	hp := p.HP
	r.w.npcAttackPlayer(guard, p.ServerID, r.at(3*time.Second))
	his := hp - p.HP
	hp = p.HP
	r.w.npcAttackPlayer(a, p.ServerID, r.at(3*time.Second))
	theirs := hp - p.HP
	r.w.mu.Unlock()
	if want := int(math.Round(float64(his) / 2)); his < 10 || theirs < want-1 || theirs > want+1 {
		t.Errorf("melee on the tank: the creature's %d, an add's %d; want half (%d)", his, theirs, want)
	}
	// the adds call none of their own, below 25% too; nor does the creature again in this fight
	for _, n := range got {
		n.HP = n.MaxHP / 10
	}
	r.run(3100*time.Millisecond, 6*time.Second)
	if n := len(adds()); n != 3 {
		t.Fatalf("after more ticks below 25%%: %d adds, want the 3", n)
	}
	for _, n := range r.w.NPCs {
		if n.add != nil && n.add.by != guard {
			t.Fatalf("an add of an add: %q", n.Name)
		}
	}
	// its death takes them, laid down dead with nothing on them
	r.w.mu.Lock()
	guard.HP = 0
	r.w.killNPC(guard)
	r.w.mu.Unlock()
	for _, n := range got {
		if !n.Dead || n.HP != 0 || !n.withMaster || !n.CorpseOfNothing() || r.w.NPCs[n.ServerID] != n {
			t.Errorf("an add at its creature's death: dead %v, HP %d, with its master %v, nothing on it %v, lying %v",
				n.Dead, n.HP, n.withMaster, n.CorpseOfNothing(), r.w.NPCs[n.ServerID] == n)
		}
	}
	r.godTick(9 * time.Second) // its fight put away, as the tick after a death does

	// another fight: called again, and its end (no foes left) takes them out of the world, no corpse
	r.w.mu.Lock()
	r.w.respawnNPCLocked(guard)
	r.w.mu.Unlock()
	r.fight(guard, p.ServerID)
	guard.HP = guard.MaxHP / 5
	r.run(10*time.Second, 11*time.Second)
	again := adds()
	if len(again) != 3 {
		t.Fatalf("a new fight below 25%%: %d adds, want 3", len(again))
	}
	r.endFight(guard, 12*time.Second)
	r.godTick(12*time.Second + 100*time.Millisecond)
	for _, n := range again {
		if r.w.NPCs[n.ServerID] != nil {
			t.Errorf("an add still in the world after its creature's fight ended: dead %v", n.Dead)
		}
	}
}

// The GM Jibrish's petition 1443: below 10% the creature enrages, once a fight: three times its size, every
// 3 s the players of its hate list beyond its reach brought beside it (never one in reach), and half of each
// melee hit on the one it fights on the second of its list, from the round after it enraged, the second in
// its reach. Above 10% (the control) none of it; a fight's end gives it its own size back.
func TestAnEnrageGrowsPullsAndRampages(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { meleeIntn = f }(meleeIntn)
	meleeIntn = func(n int) int { return n / 2 }
	r.godTemplate("rager", ", class, melee_pct", ", 0, 300",
		`[{"kind":"enrage","below":10,"grow":200,"every":3,"rampage":50,"say":"Enough!"}]`, "gnoll guard")
	guard := r.npc("Gnoll Guard")
	pool(guard, 100_000)
	size := guard.Size
	main, _ := r.player("Main", guard.X+1, guard.Z)
	second, ssess := r.player("Second", guard.X-1, guard.Z)
	far, fsess := r.player("Far", guard.X+50, guard.Z)
	for _, p := range []*Player{main, second, far} {
		p.HP, p.MaxHP = 10_000_000, 10_000_000
	}
	guard.Threats = map[EntityID]int64{main.ServerID: 1000, second.ServerID: 500, far.ServerID: 100}
	guard.CurrentEnemy = main.ServerID
	guard.HP = guard.MaxHP * 11 / 100
	r.run(0, 4*time.Second)
	if guard.Size != size || len(r.w.gates.due) != 0 || guard.tfight == nil || guard.tfight.enraged != nil {
		t.Fatalf("at 11%%: size %v (own %v), %d pulls; want none of it", guard.Size, size, len(r.w.gates.due))
	}
	guard.HP = guard.MaxHP * 9 / 100
	r.godTick(4100 * time.Millisecond)
	if guard.Size != 3*size {
		t.Errorf("enraged: size %v, want three times its %v", guard.Size, size)
	}
	if due := r.w.gates.due; len(due) != 1 || due[0].P != far || due[0].To.X != guard.X || due[0].To.Z != guard.Z {
		t.Fatalf("its first pull: %+v, want the far one alone, beside it", due)
	}
	if !hasLine(linesOf(drain(fsess)), "Gnoll Guard pulls you to it!") {
		t.Error("the far one was not told it was pulled")
	}
	r.w.gates.due = nil
	r.run(4200*time.Millisecond, 6900*time.Millisecond)
	if len(r.w.gates.due) != 0 {
		t.Errorf("a pull before its 3 s: %+v", r.w.gates.due)
	}
	r.godTick(7100 * time.Millisecond)
	if due := r.w.gates.due; len(due) != 1 || due[0].P != far {
		t.Errorf("its next pull at 3 s: %+v", due)
	}
	r.w.gates.due = nil

	// the rampage: half of a hit on the one it fights lands on the second, in reach
	r.w.mu.Lock()
	enragedAt := guard.tfight.enraged.at
	guard.RoundStart = enragedAt.Add(-time.Second) // a round begun before it enraged: none
	hm, hs := main.HP, second.HP
	r.w.npcAttackPlayer(guard, main.ServerID, enragedAt.Add(time.Second))
	if hs != second.HP {
		t.Errorf("a round begun before the enrage rampaged %d", hs-second.HP)
	}
	guard.RoundStart = enragedAt.Add(time.Second)
	hm, hs = main.HP, second.HP
	r.w.npcAttackPlayer(guard, main.ServerID, enragedAt.Add(2*time.Second))
	toMain, toSecond := hm-main.HP, hs-second.HP
	if want := int(math.Round(float64(toMain) / 2)); toMain <= 0 || toSecond != want {
		t.Errorf("the rampage: %d on the main target, %d on the second; want half (%d)", toMain, toSecond, want)
	}
	drain(ssess)
	// the second out of its reach: no rampage on anyone
	second.X = guard.X + 40
	hs, hf := second.HP, far.HP
	r.w.npcAttackPlayer(guard, main.ServerID, enragedAt.Add(3*time.Second))
	if second.HP != hs || far.HP != hf {
		t.Errorf("the second out of reach: it took %d, the third %d; want none", hs-second.HP, hf-far.HP)
	}
	r.w.mu.Unlock()
	r.endFight(guard, 20*time.Second)
	if guard.Size != size {
		t.Errorf("after its fight: size %v, want its own %v", guard.Size, size)
	}
}

// The GM Jibrish's petition 1442: an attack of a fixed 2,000 every 10 s, cast only while what it fights
// stands beyond its melee reach (rooted here) and within its range: three in 25 s; never in its reach (the
// control), nor past its range.
func TestAnAttackFromOutOfReach(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("cannon", ", class, melee_pct", ", 0, 10",
		`[{"kind":"nuke","name":"Gear Barrage","fx":3643621442,"damage":2000,"every":10,"out_of_reach":true,"range":100}]`, "gnoll guard")
	guard := r.npc("Gnoll Guard")
	pool(guard, 100_000)
	root := func() {
		guard.Effects = []*ActiveEffect{{SpellID: 77, Name: "Test Root", Root: true, Until: r.at(time.Hour)}}
	}
	for i, c := range []struct {
		name      string
		off       float32
		want, hit int
	}{{"rooted, 20 off", 20, 3, 2000}, {"in reach (the control)", 2, 0, 0}, {"past its range", 150, 0, 0}} {
		base := time.Duration(i) * 100 * time.Second
		r.endFight(guard, base)
		guard.tfight = nil
		root()
		p, sess := r.player("Kiter "+c.name, guard.X+c.off, guard.Z)
		p.HP, p.MaxHP = 1_000_000, 1_000_000
		r.refight(guard, p.ServerID)
		drain(sess)
		n := 0
		var landed []int
		for d := base; d <= base+25*time.Second; d += 100 * time.Millisecond {
			r.w.mu.Lock()
			r.w.aiSystem(r.at(d))
			r.w.mu.Unlock()
			for _, b := range castsIn(drain(sess)) {
				if b.fx == 3643621442 {
					n++
				}
			}
			before := p.HP
			r.w.mu.Lock()
			r.w.templateSystem(r.at(d))
			r.w.mu.Unlock()
			if lost := before - p.HP; lost > 0 && c.off > 4 {
				landed = append(landed, lost)
			}
		}
		if n != c.want {
			t.Errorf("%s: %d casts in 25 s, want %d", c.name, n, c.want)
		}
		for _, l := range landed {
			if l != c.hit {
				t.Errorf("%s: a hit of %d, want %d", c.name, l, c.hit)
			}
		}
		if c.want > 0 && len(landed) != c.want {
			t.Errorf("%s: %d hits landed, want %d", c.name, len(landed), c.want)
		}
	}
}

// The owner's Innoruuk on a creature (his fights are with Mr. Moogie): his damage over time on what he
// fights, kept up; his procs on his hits: his template's (250, cut by an arcane resist of the creature's
// template), a proc spell's of his own (650 disease, a pet's chance of his level), and his veil, 1% of his
// most hit points at every hit, cut by the arcane resist and his heal the same. On a player his veil is cut
// by the player's arcane resist.
func TestACreaturesKitOnACreature(t *testing.T) {
	r := newTemplateRig(t)
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 }
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { return 0.5 }
	const dot, plague = 7201, 7202
	for _, q := range []string{
		fmt.Sprintf(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (%d, 50, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Test Swarm', '')`, dot),
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (%d, 'Target, Decrease HP per tic by 300, 24sec', 11, 'test')`, dot),
		fmt.Sprintf(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (%d, 50, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Test Largesse', '')`, plague),
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (%d, '', 12, 'test', '{"v":1,"scope":"target",`+
			`"target_type":"Friendly","harmful":false,"duration_s":3600,"effects":[{"id":"0001","type":"Offensive Proc","kind":"proc","procs":[`+
			`{"id":"0002","type":"Dmg/Heal","kind":"hp","amount":-650,"resist":"dr","on":"offense","scope":"target","target_type":"Enemy"}]}]}')`, plague),
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r.godTemplate("hated", ", class, melee_pct, proc_chance, proc_damage, proc_name, proc_resist", ", 0, 10, 25, 250, 'Swarming Sting', 'AR'",
		fmt.Sprintf(`[{"kind":"dot","spell_id":%d,"every":24,"first":1,"cast":0.5},{"kind":"proc","spell_id":%d},`+
			`{"kind":"veil","tap_pct":1,"resist":"AR","name":"Veil of Death"}]`, dot, plague), "gnoll brute")
	r.godTemplate("arcane hide", ", class, melee_pct, resist_kind, resist_pct", ", 0, 10, 'AR', 50", `[]`, "gnoll guard")
	inno, moog := r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
	r.place(inno, moog.X+3, moog.Z)
	pool(inno, 500_002)
	pool(moog, 10_000_000)
	if pr := inno.tmpl.procs; len(pr) != 1 || pr[0].least != 650 || pr[0].resist != StatDR || pr[0].chanceFor(inno) != 3+3*34/20 {
		t.Fatalf("the proc spell read: %+v, want 650 disease at a pet's chance of level 34 (%d%%)", pr, 3+3*34/20)
	}
	if _, ok := r.w.SetNPCOn(inno.ServerID, "gnoll guard", 30, true); !ok {
		t.Fatal("no sic")
	}

	// his procs and veil on a hit of his: the sting 250 halved by the arcane hide, the plague 650 whole, the
	// veil's 5,000 (1% of 500,002) halved and his heal the same
	inno.HP = 400_000
	hm := moog.HP
	r.w.mu.Lock()
	r.w.creatureProcOnNPCLocked(inno, moog, 10, r.at(0))
	r.w.mu.Unlock()
	if got, want := hm-moog.HP, 125+650+2500; got != want {
		t.Errorf("his procs on the creature: %d, want the sting's 125, the plague's 650 and the veil's 2,500 (%d)", got, want)
	}
	if got := inno.HP - 400_000; got != 2500 {
		t.Errorf("his veil healed him %d, want the 2,500 it took", got)
	}

	// his damage over time, kept up on the creature
	r.run(0, 5*time.Second)
	var ae *ActiveEffect
	for _, e := range moog.Effects {
		if e.SpellID == dot && e.npcCaster == inno.ServerID {
			ae = e
		}
	}
	if ae == nil || ae.HPTick != -300 {
		t.Fatalf("his damage over time on the creature: %+v", ae)
	}
	r.run(5100*time.Millisecond, 30*time.Second)
	n := 0
	for _, e := range moog.Effects {
		if e.SpellID == dot && e.npcCaster == inno.ServerID {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d of his damage over time on the creature at 30 s, want one kept up", n)
	}

	// on a player: the veil cut by the player's arcane resist, his heal what landed
	p, sess := r.player("Tank", inno.X-1, inno.Z)
	p.HP, p.MaxHP = 1_000_000, 1_000_000
	inno.HP = 400_000
	r.w.mu.Lock()
	hp := p.HP
	r.w.abilityProcsOnPlayerLocked(sess, p, inno, r.at(31*time.Second))
	r.w.mu.Unlock()
	if got := hp - p.HP; got != 650+5000 {
		t.Errorf("on a player of no resists: %d, want the plague's 650 and the veil's 5,000", got)
	}
	if lines := linesOf(drain(sess)); !hasLine(lines, "Gnoll Brute's Veil of Death takes 5000 from you (your resist took 0%).") {
		t.Errorf("the player told %q", lines)
	}
	if got := inno.HP - 400_000; got != 5000 {
		t.Errorf("his veil on the player healed him %d, want 5,000", got)
	}
}

// The owner's "when the summoned pet's master dies, so does the pet": a creature's summoned adds die with it
// (TestASummonCallsThreeAddsAtHalf), and KillWithMaster lays a trigger's spawn down dead, a corpse of
// nothing that lies its death's few seconds; the control, another creature beside it, lives.
func TestKillWithMasterLaysItDown(t *testing.T) {
	r := newTemplateRig(t)
	pup, necro := r.npc("Gnoll Pup"), r.npc("Gnoll Necro")
	if name, ok := r.w.KillWithMaster(pup.ServerID); !ok || name != "Gnoll Pup" {
		t.Fatalf("KillWithMaster: %q %v", name, ok)
	}
	if !pup.Dead || pup.HP != 0 || !pup.CorpseOfNothing() || necro.Dead {
		t.Errorf("laid down: dead %v, HP %d, nothing on it %v; the control dead %v", pup.Dead, pup.HP, pup.CorpseOfNothing(), necro.Dead)
	}
	if !pup.corpseLies(pup.DeathTime.Add(petDismissedLies-time.Second), r.w.corpseDecay) || pup.corpseLies(pup.DeathTime.Add(petDismissedLies), r.w.corpseDecay) {
		t.Errorf("its body lies %v, want petDismissedLies %v", pup.decayOf(r.w.corpseDecay), petDismissedLies)
	}
	if _, ok := r.w.KillWithMaster(pup.ServerID); ok {
		t.Error("a dead one laid down again")
	}
	r.w.mu.Lock()
	r.w.respawnNPCLocked(pup)
	r.w.mu.Unlock()
	if pup.CorpseOfNothing() {
		t.Error("its next life is still one of nothing")
	}
}
