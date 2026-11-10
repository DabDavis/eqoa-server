package entity

import (
	"fmt"
	"testing"
	"time"
)

// The owner's of 2026-11-10, watching Innoruuk fight Mr. Moogie: "innoruuk is casting veil of death on moogie
// and the buffs, should be on innoruuk", "innoruuk keeps casting veil of death also no reason since it is a
// longer cooldown"; and "largesse of plague should have a proc spellfx"; and the GM Jibrish's petition 1458, Mr.
// Moogie's line at Innoruuk's death (selfbuff.go).

const (
	fxLargesseBuff = 0x0A0B0C01 // the test's buff visuals
	fxVeilBuff     = 0x0A0B0C02
	fxDiseaseHit   = 0x0CACDEE1 // SPELLFX.CSF's disease proc hit
)

// hatedKit is Innoruuk's proc and veil of buff spells (innoruukSpells: Largesse of Plague's 5 minutes,
// Veil of Death +'s 30), the proc's look the disease hit's, as tools/fix_innoruuk_visuals_20261110.py
// writes them; plainProc the control's, a proc of its own figures (no buff).
var (
	hatedKit  = fmt.Sprintf(`[{"kind":"proc","spell_id":7301,"fx":%d},{"kind":"veil","spell_id":7302,"tap_pct":1,"resist":"AR"}]`, fxDiseaseHit)
	plainProc = `[{"kind":"proc","proc_chance":10,"proc_damage":50}]`
)

// innoruukSpells writes hatedKit's two buffs: 7301 a proc buff of 650 lasting 5 minutes (as the EQOAEmu
// import keeps Largesse of Plague), 7302 a line of 30 minutes (as Veil of Death + is kept), each with its
// own visual.
func innoruukSpells(r *templateRig) {
	r.t.Helper()
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire,
			spellname, spelldesc) VALUES (7301, 50, 0, 0, 16880, 0, 0, 0, 5, 1, 0, 0, 'Test Largesse', '')`,
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (7301, '', %d, 'test', '{"v":1,"scope":"target",`+
			`"target_type":"Friendly","harmful":false,"duration_s":300,"effects":[{"id":"0001","type":"Offensive Proc","kind":"proc","procs":[`+
			`{"id":"0002","type":"Dmg/Heal","kind":"hp","amount":-650,"resist":"dr","on":"offense","scope":"target","target_type":"Enemy"}]}]}')`, fxLargesseBuff),
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire,
			spellname, spelldesc) VALUES (7302, 12, 0, 0, 16880, 0, 0, 0, 5, 0, 0, 0, 'Test Veil +', '')`,
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (7302, 'Self, Increase AC by 1, 30min', %d, 'test')`, fxVeilBuff),
	} {
		if _, err := r.db.Exec(q); err != nil {
			r.t.Fatalf("%s: %v", q, err)
		}
	}
}

// innoruukRig is the template rig with hatedKit on the brute, plainProc on the guard for the control, and
// a player fighting each.
func innoruukRig(t *testing.T) (r *templateRig, inno, ctl *NPC, p, q *Player, ps, qs []func() [][]byte) {
	t.Helper()
	r = newTemplateRig(t)
	innoruukSpells(r)
	r.godTemplate("hated", ", class, melee_pct", ", 0, 10", hatedKit, "gnoll brute")
	r.godTemplate("plain proc", ", class, melee_pct", ", 0, 10", plainProc, "gnoll guard")
	inno, ctl = r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
	pool(inno, 500_000)
	pool(ctl, 100_000)
	p, sp := r.player("Tank", inno.X+1, inno.Z)
	q, sq := r.player("Other", ctl.X+1, ctl.Z)
	for _, pl := range []*Player{p, q} {
		pl.HP, pl.MaxHP = 10_000_000, 10_000_000
	}
	r.fight(inno, p.ServerID)
	r.fight(ctl, q.ServerID)
	return r, inno, ctl, p, q, []func() [][]byte{func() [][]byte { return drain(sp) }}, []func() [][]byte{func() [][]byte { return drain(sq) }}
}

// selfCasts is how many 0x00B1s of npc's at itself msgs hold, by visual.
func selfCasts(r *templateRig, npc *NPC, msgs [][]byte) map[uint32]int {
	out := map[uint32]int{}
	for _, c := range castsIn(msgs) {
		if c.caster == r.objectID(npc.ServerID) && c.target == c.caster {
			out[c.fx]++
		}
	}
	return out
}

// His buffs are his: as his fight begins he casts Largesse of Plague and Veil of Death + on himself, their own
// visuals on him, once; Largesse again at its 5 minutes, the Veil not before its 30. His procs and veil send
// no buff's visual at what he fights: a Largesse proc shows the disease hit's at it, a veil's tap nothing. The
// control, a creature whose proc is of its own figures (no buff), casts nothing on itself.
func TestACreaturesBuffsAreCastOnItself(t *testing.T) {
	r, inno, ctl, p, _, ps, qs := innoruukRig(t)
	r.godTick(0)
	first := ps[0]()
	if got := selfCasts(r, inno, first); got[fxLargesseBuff] != 1 || got[fxVeilBuff] != 1 {
		t.Fatalf("as his fight begins: his casts on himself %v, want Largesse's and the Veil's once each", got)
	}
	if got := selfCasts(r, ctl, qs[0]()); len(got) != 0 {
		t.Errorf("the control cast %v on itself", got)
	}
	for d := 100 * time.Millisecond; d < 5*time.Minute; d += 10 * time.Second {
		r.godTick(d)
		if got := selfCasts(r, inno, ps[0]()); len(got) != 0 {
			t.Fatalf("at %v, within both buffs' times: cast %v on himself again", d, got)
		}
	}
	r.godTick(5 * time.Minute)
	if got := selfCasts(r, inno, ps[0]()); got[fxLargesseBuff] != 1 || got[fxVeilBuff] != 0 {
		t.Errorf("at 5 minutes: %v, want Largesse again and not the Veil", got)
	}
	r.godTick(30 * time.Minute)
	if got := selfCasts(r, inno, ps[0]()); got[fxVeilBuff] != 1 {
		t.Errorf("at 30 minutes: %v, want the Veil again", got)
	}

	// a proc and a tap on the player: the disease hit's look at him, no buff's visual, the tap silent
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 }
	ps[0]()
	hp := p.HP
	r.w.mu.Lock()
	r.w.abilityProcsOnPlayerLocked(r.w.findSessionByEntityIDLocked(p.ServerID), p, inno, r.at(31*time.Minute))
	r.w.mu.Unlock()
	if hp-p.HP != 650+5000 {
		t.Errorf("the proc and the tap took %d, want 650 and 5,000", hp-p.HP)
	}
	var fxs []uint32
	for _, c := range castsIn(ps[0]()) {
		fxs = append(fxs, c.fx)
	}
	if len(fxs) != 1 || fxs[0] != fxDiseaseHit {
		t.Errorf("the visuals at him: %x, want the disease hit's alone (the tap silent)", fxs)
	}
}

// The owner's own case, the scripted fight: Innoruuk set on Mr. Moogie (the brute on the guard, fighting
// back by its template) casts his buffs on himself as the fight begins, once in its first minute, and
// never at the guard; his Largesse procs on the guard show the disease hit's look there, his veil's taps
// (5,000 at every hit) nothing. The control, the guard's proc of its own figures, casts nothing on itself.
func TestACreaturesBuffsAreItsOwnInAScriptedFight(t *testing.T) {
	r := newTemplateRig(t)
	innoruukSpells(r)
	defer func(f func(int) int) { templateIntn = f }(templateIntn)
	templateIntn = func(int) int { return 0 } // every proc lands
	r.godTemplate("hated", ", class, melee_pct", ", 0, 10", hatedKit, "gnoll brute")
	r.godTemplate("plain proc", ", class, melee_pct", ", 0, 10", plainProc, "gnoll guard")
	brute, guard := r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
	r.place(brute, guard.X+3, guard.Z)
	pool(brute, 500_000)
	pool(guard, 100_000_000)
	_, watcher := r.player("Watcher", guard.X+20, guard.Z)
	if _, ok := r.w.SetNPCOn(brute.ServerID, "gnoll guard", 30, true); !ok {
		t.Fatal("no sic of the brute on the guard")
	}
	r.run(0, time.Minute)
	if brute.CurrentEnemy != guard.ServerID || guard.CurrentEnemy != brute.ServerID {
		t.Fatalf("the fight: brute on %d (want %d), guard on %d (want %d)", brute.CurrentEnemy, guard.ServerID, guard.CurrentEnemy, brute.ServerID)
	}
	b, g := r.objectID(brute.ServerID), r.objectID(guard.ServerID)
	self, at := map[uint32]int{}, map[uint32]int{}
	for _, c := range castsIn(drain(watcher)) {
		switch {
		case c.caster == b && c.target == b:
			self[c.fx]++
		case c.caster == b && c.target == g:
			at[c.fx]++
		case c.caster == g && c.target == g && (c.fx == fxLargesseBuff || c.fx == fxVeilBuff):
			t.Errorf("the control cast %08x on itself", c.fx)
		}
	}
	if self[fxLargesseBuff] != 1 || self[fxVeilBuff] != 1 {
		t.Errorf("his casts on himself in the fight's first minute: %x, want Largesse's and the Veil's once each", self)
	}
	if at[fxLargesseBuff] != 0 || at[fxVeilBuff] != 0 || at[fxDiseaseHit] == 0 {
		t.Errorf("his looks at the guard: %x, want the disease hit's (his procs) and no buff's", at)
	}
	if lost := guard.MaxHP - guard.HP; lost < 10*(650+5000) {
		t.Errorf("the guard lost %d in a minute, want his procs' 650 and his veil's 5,000 at each of his hits", lost)
	}
}

// Petition 1458: Mr. Moogie's line in /say when his hit kills Innoruuk, heard within a say's reach; the
// control, his kill of another creature, says nothing.
func TestAKillSaysItsLine(t *testing.T) {
	r := newTemplateRig(t)
	r.godTemplate("dunker", ", class", ", 0", `[{"kind":"killsay","foes":["Gnoll Brute"],"say":"Dab got dunked again"}]`, "gnoll guard")
	guard, brute, pup := r.npc("Gnoll Guard"), r.npc("Gnoll Brute"), r.npc("Gnoll Pup")
	_, sess := r.player("Listener", guard.X+20, guard.Z)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	pup.HP = 1
	r.w.npcHitNPCLocked(guard, pup, r.at(0))
	if !pup.Dead || countLines(linesOf(drain(sess)), "Dab got dunked again") != 0 {
		t.Fatalf("the control, the pup killed (dead %v): the line said", pup.Dead)
	}
	brute.HP = 1
	r.w.castFoeHitLocked(guard, brute, 10, "Kick", r.at(time.Second))
	if lines := linesOf(drain(sess)); !brute.Dead || !hasLine(lines, "Gnoll Guard: Dab got dunked again") {
		t.Errorf("the brute killed (dead %v): heard %q", brute.Dead, lines)
	}
}
