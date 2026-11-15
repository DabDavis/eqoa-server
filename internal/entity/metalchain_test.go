package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The Metal Moogie chain (work board #688, the GM Jibrish's): V3 to V5 on the metal rig, their originals
// spawn-only beside Metal Moogie's, their templates as the spec has them (V2's, then Arcane Siphon, then the
// gearlings at 25%, then Self Repair at 50%), the Moogie Gearling at size 4.5 on a template of a fixed 25 a
// hit and the root pinned to a stand-in of spell 460.

const (
	chainRootSpell = 460
	chainRootFX    = 3479902439
)

func newChainRig(t *testing.T) *metalRig {
	t.Helper()
	return newMetalRigWith(t, func(exec func(q string, args ...any)) {
		for i, name := range []string{MetalMoogieV2Name, MetalMoogieV3Name, MetalMoogieV4Name, MetalMoogieV5Name} {
			id := 702 + i
			exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, npc_id)
				VALUES (?, ?, 0, 1000, 10, 1030, 128, ?, 9.0, 60, 0, '0', ?)`, id, name, metalBody, id)
			exec(`INSERT INTO npcspawnonly (name, note, source) VALUES (?, 'test', 'test')`, strings.ToLower(name))
		}
		exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, npc_id)
			VALUES (706, 'Moogie Gearling', 0, 1000, 10, 1030, 128, ?, 4.5, 60, 0, '0', 706)`, metalBody)
		exec(`INSERT INTO npcspawnonly (name, note, source) VALUES ('moogie gearling', 'test', 'test')`)
		exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, 8, 0, 0, 16880, 3, 0, 0, 77, 1, 0, 0, 'Root', '')`, chainRootSpell)
		exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Target, Root, 36sec', ?, 'test')`, chainRootSpell, chainRootFX)
		v2 := fmt.Sprintf(`{"kind":"lifetap","spell_id":%d,"every":30,"first":30,"damage_pct":110}`, metalTapSpell)
		v3 := v2 + `,{"kind":"lifetap","name":"Arcane Siphon","fx":2943531157,"damage":1000,"resist":"AR","every":30,"first":15}`
		v4 := v3 + `,{"kind":"summon","below":25,"count":2,"npc":"Moogie Gearling","say":"Gearlings! Assist your master!"}`
		v5 := v4 + `,{"kind":"heal","name":"Self Repair","heal_pct":100,"below":50,"every":2,"cooldown":3600,"say":"Self repair complete."}`
		for name, abil := range map[string]string{"metal moogie v2": v2, "metal moogie v3": v3, "metal moogie v4": v4, "metal moogie v5": v5} {
			exec(`INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, proc_chance, proc_damage, proc_name, proc_resist,
				kick_damage, kick_every, kick_first, abilities, note, source)
				VALUES (?, 0, 382, 100, 578, 25, 385, 'Static Discharge', 'LR', 578, 6, 6, ?, 'test', 'test')`, name, "["+abil+"]")
			exec(`INSERT INTO npctemplate (name, template, note, source) VALUES (?, ?, 'test', 'test')`, name, name)
		}
		exec(`INSERT INTO classtemplates (name, class, hp_pct, taken_pct, melee_pct, melee_fixed, abilities, note, source)
			VALUES ('moogie gearling', 0, 35, 100, 100, 25, ?, 'test', 'test')`,
			fmt.Sprintf(`[{"kind":"root","spell_id":%d,"every":18,"first":3}]`, chainRootSpell))
		exec(`INSERT INTO npctemplate (name, template, note, source) VALUES ('moogie gearling', 'moogie gearling', 'test', 'test')`)
	})
}

// vchallenge is challenge of the version named: his countdown run out, the clock moved on by it.
func (r *metalRig) vchallenge(sess *session.Session, name string, d time.Duration) (string, bool, *NPC) {
	r.t.Helper()
	line, ok := r.w.MetalMoogieVersionChallenge(sess, r.moogie.ServerID, name, r.at(d))
	if ok {
		for s := 0; s <= MetalCountdown; s++ {
			r.w.mu.Lock()
			r.w.metalMoogieSystem(r.at(d + time.Duration(s)*time.Second))
			r.w.mu.Unlock()
		}
		r.t0 = r.t0.Add(MetalCountdown * time.Second)
		drain(sess)
	}
	return r.standing(line, ok)
}

// gearlings is the living adds standing.
func (r *metalRig) gearlings() []*NPC {
	var out []*NPC
	for _, n := range r.w.NPCs {
		if n.Name == MoogieGearlingName && !n.SpawnOnly && !n.Dead {
			out = append(out, n)
		}
	}
	return out
}

// The chain's templates read with no problem, each version's original is seen, and one metal duel stands
// at a time across all five: V3 rises for Anna with V2's line in his name, and while he stands Bert's asks of
// V1, V2, V4 and V5 are refused naming whom he duels, Anna's of V5 too (hers already); V1's name is no
// version's (refused as absent).
func TestMetalChainOneDuelAtATimeAcrossAllFive(t *testing.T) {
	r := newChainRig(t)
	for _, p := range r.w.ClassTemplateProblems() {
		if strings.Contains(p, "metal moogie") || strings.Contains(p, "gearling") {
			t.Fatalf("a chain template's problem: %q", p)
		}
	}
	for _, n := range []string{MetalMoogieV2Name, MetalMoogieV3Name, MetalMoogieV4Name, MetalMoogieV5Name} {
		if !r.w.MetalMoogieVersionHere(n) {
			t.Errorf("%s's original not seen", n)
		}
	}
	if r.w.MetalMoogieVersionHere("Mini Metal Moogie") || IsMetalMoogieName("Mini Metal Moogie") || !IsMetalMoogieName("metal moogie v4") {
		t.Error("the familiar's name read as a duel's, or V4's not")
	}
	a, sa := r.player("Anna", 1000, 1026)
	_, sb := r.player("Bert", 1004, 1026)
	line, ok, c := r.vchallenge(sa, MetalMoogieV3Name, 0)
	if !ok || c == nil || c.Name != MetalMoogieV3Name || line != fmt.Sprintf(MetalLineRisesN, "Anna", MetalMoogieV3Name) {
		t.Fatalf("Anna's V3: %q %v %+v", line, ok, c)
	}
	if c.TemplateName() != "metal moogie v3" || c.challenge == nil || c.challenge.player != a.ServerID || c.CurrentEnemy != a.ServerID {
		t.Errorf("V3 standing: template %q, enemy %d", c.TemplateName(), c.CurrentEnemy)
	}
	for _, n := range []string{MetalMoogieName, MetalMoogieV2Name, MetalMoogieV4Name, MetalMoogieV5Name} {
		var line string
		var ok bool
		if n == MetalMoogieName {
			line, ok = r.w.MetalMoogieChallenge(sb, r.moogie.ServerID, r.at(time.Second))
		} else {
			line, ok = r.w.MetalMoogieVersionChallenge(sb, r.moogie.ServerID, n, r.at(time.Second))
		}
		if ok || line != fmt.Sprintf(MetalLineBusy, "Anna") {
			t.Errorf("Bert's ask of %s while Anna duels V3: %q %v", n, line, ok)
		}
	}
	if line, ok := r.w.MetalMoogieVersionChallenge(sa, r.moogie.ServerID, MetalMoogieV5Name, r.at(time.Second)); ok || line != MetalLineMine {
		t.Errorf("Anna's ask of V5 mid-duel: %q %v", line, ok)
	}
	if line, ok := r.w.MetalMoogieVersionChallenge(sb, r.moogie.ServerID, MetalMoogieName, r.at(time.Second)); ok || line != MetalLineAbsent {
		t.Errorf("V1's name as a version: %q %v", line, ok)
	}
	n := 0
	for _, x := range r.w.NPCs {
		if IsMetalMoogieName(x.Name) && !x.SpawnOnly {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d metal duels stand, want one", n)
	}
	// his win's line names his part
	r.w.mu.Lock()
	c.HP = 0
	r.w.killNPC(c)
	r.w.mu.Unlock()
	drain(sa)
	r.tick(2 * time.Second)
	if got := linesOf(drain(sa)); !hasLine(got, fmt.Sprintf(MetalLineWonPart, MetalMoogieV3Name, MetalTorsoName)) {
		t.Errorf("Anna told at V3's death: %q", got)
	}
}

// V4's gearlings are the duel's: below 25% two rise (his line said), each the challenger's alone (Bert's
// hit refused in the duel's line, his hate none, his client shown the NPC bit; Anna's not), on their own
// template at 5,040 hit points; once a fight (V4 healed and down again calls none more). They die with him,
// and at any other end of the duel (a flight) they go, their root on Anna with them.
func TestMetalChainGearlingsAreTheDuels(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	b, sb := r.player("Bert", 1004, 1026)
	_, _, c := r.vchallenge(sa, MetalMoogieV4Name, 0)
	if c == nil {
		t.Fatal("no V4")
	}
	r.w.mu.Lock()
	c.HP = c.MaxHP * 24 / 100
	r.w.templatePhasesLocked(c, r.at(time.Second))
	r.w.mu.Unlock()
	gs := r.gearlings()
	if len(gs) != 2 {
		t.Fatalf("%d gearlings below 25%%, want 2", len(gs))
	}
	if got := linesOf(drain(sa)); !hasLine(got, MetalMoogieV4Name+": Gearlings! Assist your master!") {
		t.Errorf("Anna heard at the summon: %q", got)
	}
	for _, g := range gs {
		if g.challenge != c.challenge || g.TemplateName() != "moogie gearling" || g.MaxHP != 5040 || g.Size != 4.5 || g.NPCType&0x80 != 0 {
			t.Errorf("a gearling: challenge %v, template %q, HP %d, size %v, type %#x", g.challenge == c.challenge, g.TemplateName(), g.MaxHP, g.Size, g.NPCType)
		}
		if !challengeBars(g, b.ServerID) || challengeBars(g, a.ServerID) {
			t.Errorf("bars: Bert %v (want true), Anna %v", challengeBars(g, b.ServerID), challengeBars(g, a.ServerID))
		}
		if line := challengeRefusal(g, b.ServerID); line != "Moogie Gearling duels Anna alone." {
			t.Errorf("Bert's refusal: %q", line)
		}
		r.w.mu.Lock()
		r.w.addThreatLocked(g, b.ServerID, 500)
		bufB, bufA := make([]byte, 200), make([]byte, 200)
		r.w.challengeViewLocked(sb, g, bufB)
		r.w.challengeViewLocked(sa, g, bufA)
		r.w.mu.Unlock()
		if g.Threats[b.ServerID] != 0 {
			t.Errorf("Bert's hate on a gearling: %d", g.Threats[b.ServerID])
		}
		if bufB[146]&0x80 == 0 || bufA[146]&0x80 != 0 {
			t.Errorf("the NPC bit: Bert's view %#x, Anna's %#x", bufB[146], bufA[146])
		}
		if !strings.Contains(metalWord(g), "a summon of Metal Moogie V4's duel") || !strings.Contains(g.DuelWord(), "Anna") {
			t.Errorf("#stats on a gearling: %q", metalWord(g))
		}
	}
	if !strings.Contains(metalWord(c), "Metal Moogie V4's duel") {
		t.Errorf("#stats on V4: %q", metalWord(c))
	}
	// once a fight: whole again and down again, none more
	r.w.mu.Lock()
	c.HP = c.MaxHP
	r.w.templatePhasesLocked(c, r.at(2*time.Second))
	c.HP = c.MaxHP / 10
	r.w.templatePhasesLocked(c, r.at(3*time.Second))
	r.w.mu.Unlock()
	if n := len(r.gearlings()); n != 2 {
		t.Errorf("%d gearlings after his climb back and fall, want the 2 still", n)
	}
	// they die with him
	r.w.mu.Lock()
	c.HP = 0
	r.w.killNPC(c)
	r.w.mu.Unlock()
	for _, g := range gs {
		if !g.Dead || !g.CorpseOfNothing() {
			t.Errorf("a gearling after V4's death: dead %v, nothing %v", g.Dead, g.CorpseOfNothing())
		}
	}

	// another duel: a flight takes them, and their root
	r2 := newChainRig(t)
	a2, sa2 := r2.player("Anna", 1000, 1026)
	_, _, c2 := r2.vchallenge(sa2, MetalMoogieV4Name, 0)
	r2.w.mu.Lock()
	c2.HP = c2.MaxHP / 5
	r2.w.templatePhasesLocked(c2, r2.at(time.Second))
	g := r2.gearlings()[0]
	g.X, g.Y, g.Z = a2.X, a2.Y, a2.Z+2
	r2.w.templateTurnLocked(g, a2.ServerID, r2.at(time.Second))
	r2.w.templateTurnLocked(g, a2.ServerID, r2.at(4*time.Second))
	r2.w.templateSystem(r2.at(9 * time.Second))
	r2.w.mu.Unlock()
	if a2.SpeedNow() != 0 {
		t.Fatalf("Anna's speed under a gearling's root: %v", a2.SpeedNow())
	}
	a2.Z += 200 // fled
	r2.tick(10 * time.Second)
	if r2.w.metal != nil || len(r2.gearlings()) != 0 {
		t.Errorf("after the flight: duel %v, %d gearlings", r2.w.metal != nil, len(r2.gearlings()))
	}
	if a2.SpeedNow() == 0 {
		t.Error("Anna still rooted after her duel ended")
	}
}

// A gearling's root holds the player it fights for 6 s: cast on its clock (its first 3 s into the fight,
// its spell's 3 s cast), the spell's visual and name, Anna's run speed 0 from its landing and back after
// 6 s; at a pet it waits, due. Its melee is a fixed 25 a hit through any armour: a hit on Anna takes 25
// whatever her AC.
func TestMetalChainGearlingRootAndFixedMelee(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	_, _, c := r.vchallenge(sa, MetalMoogieV5Name, 0)
	r.w.mu.Lock()
	c.HP = c.MaxHP / 5
	r.w.templatePhasesLocked(c, r.at(time.Second))
	r.w.mu.Unlock()
	gs := r.gearlings()
	if len(gs) != 2 {
		t.Fatalf("%d gearlings", len(gs))
	}
	g := gs[0]
	g.X, g.Y, g.Z = a.X, a.Y, a.Z+2
	speed := a.SpeedNow()
	r.w.mu.Lock()
	casting := r.w.templateTurnLocked(g, a.ServerID, r.at(time.Second)) // its clock begins: the first at +3 s
	r.w.mu.Unlock()
	if casting || speed == 0 {
		t.Fatalf("a root cast before its first (%v), or Anna slow already (%v)", casting, speed)
	}
	r.w.mu.Lock()
	casting = r.w.templateTurnLocked(g, a.ServerID, r.at(4*time.Second))
	r.w.mu.Unlock()
	if !casting {
		t.Fatal("no root cast at its first")
	}
	r.w.mu.Lock()
	r.w.templateSystem(r.at(5 * time.Second)) // its 3 s cast not over
	r.w.mu.Unlock()
	if a.SpeedNow() == 0 {
		t.Fatal("rooted before the cast was over")
	}
	r.w.mu.Lock()
	r.w.templateSystem(r.at(9 * time.Second))
	r.w.mu.Unlock()
	if a.SpeedNow() != 0 || !a.hasEffect(chainRootSpell) {
		t.Fatalf("after the landing: speed %v, effect %v", a.SpeedNow(), a.hasEffect(chainRootSpell))
	}
	var until time.Time
	for _, e := range a.Effects {
		if e.SpellID == chainRootSpell {
			until = e.Until
		}
	}
	landed := until.Add(-CreatureRootHold)
	r.w.mu.Lock()
	r.w.effectSystem(until.Add(-100 * time.Millisecond))
	r.w.mu.Unlock()
	if a.SpeedNow() != 0 {
		t.Error("let go before 6 s")
	}
	r.w.mu.Lock()
	r.w.effectSystem(until)
	r.w.mu.Unlock()
	if a.SpeedNow() != speed {
		t.Errorf("after 6 s (landed %v): speed %v, want %v again", landed.Sub(r.t0), a.SpeedNow(), speed)
	}
	// its melee: a fixed 25, through any armour
	for _, ac := range []int{0, 5000} {
		a.AC = ac
		hp := a.HP
		r.w.mu.Lock()
		r.w.npcAttackPlayer(g, a.ServerID, r.at(20*time.Second))
		r.w.mu.Unlock()
		if hp-a.HP != 25 {
			t.Errorf("a gearling's hit on AC %d took %d, want 25", ac, hp-a.HP)
		}
	}
	if g.meleeFixed() != 25 || c.meleeFixed() != 0 {
		t.Errorf("fixed melee: gearling %d, V5 %d", g.meleeFixed(), c.meleeFixed())
	}
}

// V5's script: Self Repair heals him whole below half (his template's, once a duel); after it, the next fall
// below half says the hacks line once, with Anna's name; below a tenth, COMPLETE HEAL and his hit points whole
// at once; 10 s later (not before) MALFUNCTION and a hundredth of his most, and nothing heals him after: his
// template's heal is not wanted, a heal of his share lands nothing, a veil's take nothing.
func TestMetalChainV5Timeline(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	_, sc := r.player("Cora", 1100, 1000) // within MetalSayHeard of him
	_, _, c := r.vchallenge(sa, MetalMoogieV5Name, 0)
	if c == nil || c.TemplateName() != "metal moogie v5" {
		t.Fatalf("no V5: %+v", c)
	}
	drain(sa)
	drain(sc)
	sys := func(d time.Duration) {
		r.w.mu.Lock()
		r.w.metalMoogieSystem(r.at(d))
		r.w.mu.Unlock()
	}
	// below half before any heal: no hacks line
	r.w.mu.Lock()
	c.HP = c.MaxHP * 45 / 100
	r.w.mu.Unlock()
	sys(time.Second)
	if got := linesOf(drain(sa)); len(got) != 0 {
		t.Errorf("before Self Repair: %q", got)
	}
	// Self Repair
	r.w.mu.Lock()
	if !r.w.templateTurnLocked(c, a.ServerID, r.at(time.Second)) {
		r.w.mu.Unlock()
		t.Fatal("no Self Repair below half")
	}
	r.w.templateSystem(r.at(3 * time.Second))
	r.w.mu.Unlock()
	if c.HP != c.MaxHP || !c.selfHealed {
		t.Fatalf("after Self Repair: HP %d/%d, healed %v", c.HP, c.MaxHP, c.selfHealed)
	}
	if got := linesOf(drain(sc)); !hasLine(got, MetalMoogieV5Name+": Self repair complete.") {
		t.Errorf("Cora heard at Self Repair: %q", got)
	}
	hacks := MetalMoogieV5Name + ": " + fmt.Sprintf(MetalV5Hacks, "Anna")
	sys(4 * time.Second)
	if got := linesOf(drain(sa)); hasLine(got, hacks) {
		t.Error("the hacks line above half")
	}
	c.HP = c.MaxHP * 49 / 100
	sys(5 * time.Second)
	c.HP = c.MaxHP * 30 / 100
	sys(6 * time.Second)
	got := linesOf(drain(sa))
	n := 0
	for _, l := range got {
		if l == hacks {
			n++
		}
	}
	if n != 1 || !hasLine(linesOf(drain(sc)), hacks) {
		t.Errorf("the hacks line %d times (want once), Anna heard %q", n, got)
	}
	// below a tenth: complete heal
	c.HP = c.MaxHP * 9 / 100
	sys(10 * time.Second)
	if c.HP != c.MaxHP || !hasLine(linesOf(drain(sa)), MetalMoogieV5Name+": "+MetalV5Complete) {
		t.Fatalf("at 9%%: HP %d/%d", c.HP, c.MaxHP)
	}
	c.HP = c.MaxHP * 8 / 100 // a second fall under a tenth: no second complete heal
	sys(19*time.Second + 900*time.Millisecond)
	if c.HP != c.MaxHP*8/100 || c.noHeal {
		t.Errorf("before the 10 s: HP %d, barred %v", c.HP, c.noHeal)
	}
	sys(20 * time.Second)
	if c.HP != c.MaxHP/100 || !c.noHeal || c.Dead {
		t.Fatalf("at the malfunction: HP %d (want %d), barred %v, dead %v", c.HP, c.MaxHP/100, c.noHeal, c.Dead)
	}
	if !hasLine(linesOf(drain(sa)), MetalMoogieV5Name+": "+MetalV5Malfunction) {
		t.Error("no MALFUNCTION line")
	}
	// nothing heals him now
	hp := c.HP
	r.w.mu.Lock()
	wanted := false
	for i := range c.tmpl.abilities {
		if ab := &c.tmpl.abilities[i]; ab.Kind == "heal" {
			wanted = r.w.templateWantsLocked(c, ab, a.ServerID, r.at(21*time.Second))
		}
	}
	r.w.templateHealLocked(&npcCast{npc: c, healPct: 100, spell: &CastSpell{Name: "Self Repair"}})
	r.w.veilHealLocked(c, nil, 5000, nil)
	r.w.mu.Unlock()
	if wanted || c.HP != hp {
		t.Errorf("after the malfunction: heal wanted %v, HP %d -> %d", wanted, hp, c.HP)
	}
	sys(60 * time.Second)
	if c.HP != hp {
		t.Errorf("a later tick moved his HP: %d -> %d", hp, c.HP)
	}
}

// The duel ending inside the 10 s leaves nothing more to come: Anna flees after COMPLETE HEAL, and at the
// 10 s no MALFUNCTION is said and no V5 stands.
func TestMetalChainV5DuelEndsDuringTheTenSeconds(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1026)
	_, sc := r.player("Cora", 1100, 1000)
	_, _, c := r.vchallenge(sa, MetalMoogieV5Name, 0)
	c.HP = c.MaxHP / 20
	r.tick(time.Second)
	if c.HP != c.MaxHP {
		t.Fatalf("no complete heal: %d/%d", c.HP, c.MaxHP)
	}
	a.Z += 300 // fled
	r.tick(3 * time.Second)
	if r.w.metal != nil {
		t.Fatal("the duel did not end at her flight")
	}
	drain(sa)
	drain(sc)
	r.tick(12 * time.Second)
	r.tick(20 * time.Second)
	for _, l := range append(linesOf(drain(sa)), linesOf(drain(sc))...) {
		if strings.Contains(l, MetalV5Malfunction) {
			t.Errorf("said after the duel ended: %q", l)
		}
	}
	if _, ok := r.w.NPCs[c.ServerID]; ok {
		t.Error("V5 still stands")
	}
}

// The Snowball: at another player alive, in the world, within 30: its 0x00B1 from the thrower with the row's
// visual, 1 off the target, never below 1, each told; its 1 s cooldown. Refused in red (nothing used) at
// oneself, a creature, a dead player, one 31 off and one in another world. No flag, no hate.
func TestMetalChainSnowballTargetsAndFloor(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1000)
	b, sb := r.player("Bert", 1010, 1000)
	d, sd := r.player("Dora", 1031, 1000)
	e, se := r.player("Eve", 1005, 1000)
	e.World = 3
	pe := NewPotionEffect(9901, "Snowball", "Target, Snowball 0x15C6A459", "", 0, "", 0, 0)
	pe.Cooldown = time.Second
	if pe.Special != PotionSnowball || pe.SpecialFX != SnowballFX || !pe.Thrown() {
		t.Fatalf("the snowball's row read as %+v", pe)
	}
	_ = sd
	_ = se
	throw := func(at uint32, dt time.Duration) string {
		t.Helper()
		refusal, err := r.w.UsePotion(sa, pe, at, r.at(dt))
		if err != nil {
			t.Fatal(err)
		}
		return refusal
	}
	for name, id := range map[string]uint32{
		"herself": sa.PlayerObjectID(), "Mr. Moogie": r.see(sa, r.moogie.ServerID), "Dora at 31": r.see(sa, d.ServerID),
		"Eve in another world": r.see(sa, e.ServerID), "no one": 0,
	} {
		if got := throw(id, 0); got != SnowballRefusal {
			t.Errorf("a snowball at %s: %q", name, got)
		}
	}
	ob := r.see(sa, b.ServerID)
	r.see(sb, a.ServerID)
	drain(sa)
	drain(sb)
	hp := b.HP
	if got := throw(ob, 0); got != "" {
		t.Fatalf("at Bert: %q", got)
	}
	if b.HP != hp-1 {
		t.Errorf("Bert's HP %d -> %d, want 1 off", hp, b.HP)
	}
	var fx bool
	for _, m := range drain(sb) {
		if opOfMsg(m) == 0x00B1 && strings.Contains(string(m), string([]byte{0x59, 0xA4, 0xC6, 0x15})) {
			fx = true
		}
	}
	if !fx {
		t.Error("Bert was sent no 0x00B1 with the snowball's visual")
	}
	if got := throw(ob, 500*time.Millisecond); !strings.HasPrefix(got, "You can use Snowball again") {
		t.Errorf("a second throw within 1 s: %q", got)
	}
	b.HP = 1
	drain(sb)
	if got := throw(ob, 2*time.Second); got != "" || b.HP != 1 || b.IsDead() {
		t.Errorf("at Bert on 1 HP: %q, HP %d", got, b.HP)
	}
	if !hasLine(linesOf(drain(sb)), "Anna hits you with a snowball!") || !hasLine(linesOf(drain(sa)), "You hit Bert with a snowball!") {
		t.Error("the lines")
	}
	b.HP = 0

	if got := throw(ob, 4*time.Second); got != SnowballRefusal {
		t.Errorf("at dead Bert: %q", got)
	}
	if a.PvPFlagged() || b.PvPFlagged() {
		t.Error("a snowball flagged someone")
	}
}

// Moogie's Insight: +25 on the Boon's percent (200 -> 225), 125 without one; kept over a death and a strip as
// the Boon (outlivesDeath); a second drink puts it back to a whole hour.
func TestMetalChainInsightStacksWithTheBoon(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1000)
	pe := NewPotionEffect(9902, "Potion of Moogie's Insight", "Self, Moogie's Insight", "", 0, "", 0, 0)
	if pe.Special != PotionInsight || pe.Thrown() {
		t.Fatalf("read as %+v", pe)
	}
	r.w.mu.Lock()
	pct, _ := r.w.xpPctLocked(a)
	r.w.mu.Unlock()
	if pct != 100 {
		t.Fatalf("before: %d", pct)
	}
	if got, err := r.w.UsePotion(sa, pe, 0, r.at(0)); got != "" || err != nil {
		t.Fatalf("drunk: %q %v", got, err)
	}
	if !hasLine(linesOf(drain(sa)), InsightLine) {
		t.Error("no line")
	}
	r.w.mu.Lock()
	pct, tag := r.w.xpPctLocked(a)
	r.w.mu.Unlock()
	if pct != 125 || !strings.Contains(tag, "insight") {
		t.Errorf("without a boon: %d %q, want 125", pct, tag)
	}
	r.w.SetBoon(DefaultBoon)
	r.w.GiveBoon(sa, "Freeport", r.at(0))
	r.w.mu.Lock()
	pct, _ = r.w.xpPctLocked(a)
	r.w.mu.Unlock()
	if pct != 225 {
		t.Errorf("with the Boon's 200: %d, want 225", pct)
	}
	if !outlivesDeath(InsightEffectID) {
		t.Error("not kept as the Boon is")
	}
	if _, err := r.w.UsePotion(sa, pe, 0, r.at(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, e := range a.Effects {
		if e.SpellID == InsightEffectID && !e.Until.Equal(r.at(30*time.Minute+InsightLasts)) {
			t.Errorf("a second drink: until %v, want a whole hour from it", e.Until.Sub(r.t0))
		}
	}
}

// The Deputy's Draught lands its spell on everyone within 30 of the drinker, the drinker too, of any level,
// grouped or not; not one 31 off, a dead one, or one in another world. Each told, the drinker how many.
func TestMetalChainDeputysDraught(t *testing.T) {
	r := newChainRig(t)
	a, sa := r.player("Anna", 1000, 1000)
	b, sb := r.player("Bert", 1020, 1000)
	b.Level = 5
	d, _ := r.player("Dora", 1031, 1000)
	e, _ := r.player("Eve", 1005, 1000)
	e.World = 3
	f, _ := r.player("Finn", 1000, 1010)
	f.HP = 0
	pe := NewPotionEffect(9903, "Deputy's Draught", "Self, Deputy's Draught 901843, Radius 30", "", 0, "", 0, 0)
	if pe.Special != PotionDeputy || pe.SpecialSpell != 901843 || pe.SpecialRadius != 30 {
		t.Fatalf("read as %+v", pe)
	}
	if got, _ := r.w.UsePotion(sa, pe, 0, r.at(0)); got == "" {
		t.Error("drunk with no record of its spell")
	}
	spell := CastSpell{SpellID: 901843, Name: "Halfling Heroism", Scope: 2, Level: 60,
		Def: ParseSpellNamed("Halfling Heroism", "Group, Gives Form, Increase AGI by 70, Increase DEX by 70, 30min", "")}
	spell.Def.MinLevel = 36
	pe.Grant = &spell
	drain(sb)
	if got, err := r.w.UsePotion(sa, pe, 0, r.at(time.Second)); got != "" || err != nil {
		t.Fatalf("drunk: %q %v", got, err)
	}
	for _, p := range []*Player{a, b} {
		if !p.hasEffect(901843) {
			t.Errorf("%s has no Halfling Heroism", p.Name)
		}
	}
	for _, p := range []*Player{d, e, f} {
		if p.hasEffect(901843) {
			t.Errorf("%s caught it", p.Name)
		}
	}
	if !hasLine(linesOf(drain(sb)), "Anna's Deputy's Draught makes a halfling of you!") {
		t.Error("Bert not told")
	}
	if !hasLine(linesOf(drain(sa)), "Your Deputy's Draught makes a halfling of 2 players.") {
		t.Error("Anna not told how many")
	}
}
