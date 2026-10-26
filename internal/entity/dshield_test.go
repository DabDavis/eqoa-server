package entity

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Damage shields and defensive procs (dshield.go; jib's review, F3 pass
// 19): the lines as EQOA-Data and spelleffect word them, a shield answering
// a creature's hit as Sandstorm's Shroud of Hate answered the training
// dummies', and what is ours (the defaults, the chance, the strongest one).

// castMsg is 0x00B1 as the client reads it: caster, target, no others, the
// cast time in ms, the visual.
func castMsg(from, to, ms, fx uint32) []byte {
	b := []byte{0xB1, 0x00}
	b = binary.LittleEndian.AppendUint32(b, from)
	b = binary.LittleEndian.AppendUint32(b, to)
	b = append(b, 0)
	b = binary.LittleEndian.AppendUint32(b, ms)
	return binary.LittleEndian.AppendUint32(b, fx)
}

// shieldEffect is an effect on its bearer until the time given: a damage
// shield of n, a defensive proc of proc (0 for none).
func shieldEffect(name string, n, proc int, until time.Time) *ActiveEffect {
	return &ActiveEffect{SpellID: 9000 + n + proc, Name: name, Until: until,
		Shield: &Shield{Min: n, Max: n, ProcMin: proc, ProcMax: proc}}
}

// sameMsgs reports whether two lists of messages are the same, in order.
func sameMsgs(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Every way the lines say a shield is read as one, whom it lands on and how
// long kept, nothing harmful; and the GM's ways and a defensive proc's.
func TestTheShieldWordsAreRead(t *testing.T) {
	const coatDesc = "Covers the caster in enchanted thorns, increasing armor class and poisoning enemies that strike you."
	for _, c := range []struct {
		name, line, desc string
		target           SpellTarget
		dur              time.Duration
		kind             EffectKind
		lo, hi           int
		stats, unread    int
		undead           bool
	}{
		{"Divine Protection", "Self, Damage Shield Decrease Hitpoints by 2, 5min",
			"A protective aura injures all undead who come in contact with it.", TargetSelf, 5 * time.Minute, EffectShield, 2, 2, 0, 0, true},
		{"Divine Protection", "Self, Damage Shield Decrease Hitpoints by 2, 5min", "", TargetSelf, 5 * time.Minute, EffectShield, 2, 2, 0, 0, false},
		{"Thistlecoat", "Self, Damage Shield Decrease Hitpoints by 4, Increase AC by 10, 7.5min", coatDesc, TargetSelf, 450 * time.Second, EffectShield, 4, 4, 1, 0, false},
		{"Barbcoat", "Self, Damage SHield Decrease Hitpoints by 8, increase AC by 30, 7.5min", "", TargetSelf, 450 * time.Second, EffectShield, 8, 8, 1, 0, false},
		{"Bramblecoat", "Self, damage shield Decrease HP by 12, Increase AC by 60, 7.5min", "", TargetSelf, 450 * time.Second, EffectShield, 12, 12, 1, 0, false},
		{"Spikecoat", "Self, Decrease Hitpoints by 22, Increase AC by 110, 7.5min", "", TargetSelf, 450 * time.Second, EffectShield, 22, 22, 1, 0, false},
		{"Bladecoat", "Target, Decrease Hitpoints by 30, Increase AC by 140, 7.5min", "", TargetSingle, 450 * time.Second, EffectShield, 30, 30, 1, 0, false},
		{"Chilling Aura", "Self, Damage shield, Decrease Mob Hitpoints by 2, 5min",
			"Summons an aura that surrounds your weapon, inflicting unholy damage.", TargetSelf, 5 * time.Minute, EffectShield, 2, 2, 0, 0, false},
		{"O'Keils Radiation", "Self Debuff Hp by 6 Damage Shield 5min", "", TargetSelf, 5 * time.Minute, EffectShield, 6, 6, 0, 0, false},
		{"O'Keils Embers", "Self, DS Decrease Hitpoints by 10, 5min", "", TargetSelf, 5 * time.Minute, EffectShield, 10, 10, 0, 0, false},
		{"Spirit Shield", "Target, approx. 27 Damage to Mob when it hits, 10min", "", TargetSingle, 10 * time.Minute, EffectShield, 27, 27, 0, 0, false},
		{"Barbshield", "Target, Damageshield, 8.3min", "Arcane Damage Shield", TargetSingle, 498 * time.Second, EffectShield, 0, 0, 0, 0, false},
		{"Smoldering Aura", "Target, DamageShield, 5min", "Fire Damage Shield", TargetSingle, 5 * time.Minute, EffectShield, 0, 0, 0, 0, false},
		{"Smoldering Cloak", "Group, DamageShield, 5min", "", TargetGroup, 5 * time.Minute, EffectShield, 0, 0, 0, 0, false},
		{"Briarshield", "Target, Damage shield, 8.3min", "", TargetSingle, 498 * time.Second, EffectShield, 0, 0, 0, 0, false},
		{"Searing Aura", "Target, Damage Shield, 8min, FR", "", TargetSingle, 8 * time.Minute, EffectShield, 0, 0, 0, 0, false},
		{"Frozen Mark", "Pet, Increase INT by 25, Add Damage Shield, 10min", "", TargetPet, 10 * time.Minute, EffectShield, 0, 0, 1, 0, false},
		{"Burning Mark", "Pet, Increase INT by 35, Add Offensive Proc:, Add DS, 10min", "", TargetPet, 10 * time.Minute, EffectShield, 0, 0, 1, 1, false},
		{"Earthen Mark", "Pet, increase INT by 45, Add Offensive Proc, Add DS Proc, 10min - Enhances Pet", "", TargetPet, 10 * time.Minute, EffectShield, 0, 0, 1, 1, false},
		// a GM's
		{"A Shield", "Target, Damage Shield of 30, 8min", "", TargetSingle, 8 * time.Minute, EffectShield, 30, 30, 0, 0, false},
		{"A Shield", "Target, Damage Shield 25 to 35, 8min", "", TargetSingle, 8 * time.Minute, EffectShield, 25, 35, 0, 0, false},
		{"A Mirror", "Self, Reflect 12 damage, 5min", "", TargetSelf, 5 * time.Minute, EffectShield, 12, 12, 0, 0, false},
		{"A Ward", "Self, Add Defensive Proc, Decrease Hitpoints by 50, 5min", "", TargetSelf, 5 * time.Minute, EffectDefProc, 50, 50, 0, 0, false},
		{"A Ward", "Self, Add Defensive Proc 40 to 60, 5min", "", TargetSelf, 5 * time.Minute, EffectDefProc, 40, 60, 0, 0, false},
		// no line of their own (shieldLines)
		{"Shroud of Hate", "", "Defensive Proc", TargetSelf, 5 * time.Minute, EffectShield, 0, 0, 0, 0, false},
		{"O'Keil's Radiation", "", "Fire damage Shield", TargetSelf, 5 * time.Minute, EffectShield, 6, 6, 0, 0, false},
	} {
		d := ParseSpellNamed(c.name, c.line, c.desc)
		var shields, stats int
		var sh SpellEffect
		for _, e := range d.Effects {
			switch e.Kind {
			case EffectShield, EffectDefProc:
				shields++
				sh = e
			case EffectStat:
				stats++
			default:
				t.Errorf("%s %q: an effect of kind %d", c.name, c.line, e.Kind)
			}
		}
		if shields != 1 || sh.Kind != c.kind || sh.Amount != c.lo || sh.AmountMax != c.hi || sh.Undead != c.undead || stats != c.stats ||
			d.Harmful || d.Target != c.target || d.Duration != c.dur || len(d.Unread) != c.unread {
			t.Errorf("%s %q: %+v", c.name, c.line, d)
		}
	}

	// The controls: the same words read otherwise
	for _, c := range []struct{ name, line, why string }{
		{"Spikeshot", "Self, Decrease Hitpoints by 22, Increase AC by 110, 7.5min", "not a ranger's coat: its hit stays a hit"},
		{"Static Coat", "Self, Decrease Hitpoints by 22, Increase AC by 110, 7.5min", "a two-word coat is the druid's, no hit of it a shield"},
		{"Aura Stone", "Summons Aura Stone, 3 charges, damage shield", "an item"},
		{"Shield of 1000 Needles", "Level 60 Epic third spell option. Power cost: 460, Cast: 2, Recast: 3. 60 epic 3rd choice: up to 125 damage shield and debuff's -25 AR.", "an epic's prose"},
		{"Mark of Pain", "Target, damages target when it hits, 42sec", "a curse on the creature"},
		{"Storm Caller's Curse", "Target, Inflicts Damage on mob when it hits, 60sec", "a curse on the creature"},
		{"A Ward", "Self, Add Defensive Proc: Shroud Effect, 5min", "a proc naming a spell and no damage"},
		{"Fiery Burst", "Target, Decrease Hitpoints by 26 ; Increase 8% based on DEX", "a nuke"},
	} {
		if d := ParseSpellNamed(c.name, c.line, ""); d.hasShield() {
			t.Errorf("%s (%s) read as a shield: %+v", c.name, c.why, d)
		}
	}
	if d := ParseSpellNamed("Spikeshot", "Self, Decrease Hitpoints by 22, Increase AC by 110, 7.5min", ""); !d.Harmful {
		t.Errorf("the control's hit is no longer harmful: %+v", d)
	}
}

// casterAndCrab is the caster fixture with its creature a level 34 crab
// fighting the druid, and so much armour on the druid that every hit is the
// crab's floor (17).
func casterAndCrab(t *testing.T) (*caster, *NPC) {
	c := newCaster(t)
	c.p.AC = 100000
	crab := c.npc
	crab.Name, crab.Level, crab.HP, crab.MaxHP = "Desert Crab", 34, 2000, 2000
	crab.CurrentEnemy = c.p.ServerID
	crab.Threats = map[EntityID]int64{c.p.ServerID: 10}
	return c, crab
}

// barbshield is the druid's level 16 Barbshield as this server keeps it
// (spellpattern 300357, its EQOA-Data line in spelleffect).
func barbshield() CastSpell {
	return CastSpell{SpellID: 300357, DisplayIndex: 3, Level: 16, Scope: 1, Name: "Barbshield", Icon: 0x2A2B2C2D,
		CastTime: 2 * time.Second, Recast: time.Second, PowerCost: 30, Range: 30,
		Def: ParseSpellNamed("Barbshield", "Target, Damageshield, 8.3min", "Arcane Damage Shield")}
}

// The druid's Barbshield on itself: on its effects channel, and the crab's
// hit on it answered as Sandstorm's Shroud of Hate answered a dummy's, a
// 0x00B1 of the druid's (cast 0) and its 0x00DB, the crab's hate for the
// druid up by as much; a shield's kill is the druid's, its XP too.
func TestAShieldHurtsTheAttacker(t *testing.T) {
	c, crab := casterAndCrab(t)
	if !c.w.StartCast(c.sess, barbshield(), c.me, c.at(0)) {
		t.Fatalf("Barbshield did not begin: %x", c.sent())
	}
	c.tick(2*time.Second + castLand)
	// its line names no damage: half its level, 8 (ours)
	if len(c.p.Effects) != 1 || c.p.Effects[0].Shield == nil || *c.p.Effects[0].Shield != (Shield{Min: 8, Max: 8}) ||
		len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Barbshield" {
		t.Fatalf("on the druid: %+v, buffs %+v", c.p.Effects, c.p.Buffs)
	}
	c.sent()

	start := 10 * time.Second
	ai := func(d time.Duration) [][]byte {
		c.w.mu.Lock()
		c.w.aiSystem(c.at(start + d))
		c.w.mu.Unlock()
		return c.sent()
	}
	ai(0) // the crab's swing
	hp := c.p.HP
	got := ai(500 * time.Millisecond)
	want := [][]byte{damageMsg(t, c.npcO, -17, c.me), castMsg(c.me, c.npcO, 0, fxShieldHit), damageMsg(t, c.me, -8, c.npcO)}
	if !sameMsgs(got, want) {
		t.Fatalf("the crab's first hit sent\n% X\nwant\n% X", got, want)
	}
	if crab.HP != 1992 || c.p.HP != hp-17 || crab.Threats[c.p.ServerID] != 18 {
		t.Errorf("crab %d HP, the druid %d (was %d), hate %d", crab.HP, c.p.HP, hp, crab.Threats[c.p.ServerID])
	}

	// the shield kills: the druid's kill, its XP, the crab a corpse
	crab.HP = 5
	xp := c.p.TotalXP
	ai(1500 * time.Millisecond)
	if !crab.Dead || crab.HP != 0 || c.p.TotalXP <= xp || crab.CurrentEnemy != 0 {
		t.Errorf("after the shield's kill: dead %v, HP %d, XP %d (was %d), enemy %d", crab.Dead, crab.HP, c.p.TotalXP, xp, crab.CurrentEnemy)
	}

	// the control: with no shield on, a hit is the crab's alone
	c2, crab2 := casterAndCrab(t)
	c2.w.mu.Lock()
	c2.w.npcAttackPlayer(crab2, c2.p.ServerID, c2.at(time.Second))
	c2.w.mu.Unlock()
	if got := c2.sent(); len(got) != 1 || crab2.HP != 2000 {
		t.Errorf("no shield: sent %x, crab %d HP", got, crab2.HP)
	}
}

// A shield answers nothing once its time is over, whether still on the list
// or taken off by the effects' sweep at its end.
func TestNoShieldPastItsTime(t *testing.T) {
	c, crab := casterAndCrab(t)
	now := c.at(time.Minute)
	c.p.Effects = []*ActiveEffect{shieldEffect("Barbshield", 8, 0, now)} // over at now
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, now)
	c.w.mu.Unlock()
	if got := c.sent(); len(got) != 0 || crab.HP != 2000 {
		t.Errorf("a shield over: sent %x, crab %d HP", got, crab.HP)
	}
	// the control: a millisecond before its end it answers
	c.w.mu.Lock()
	c.w.playerShieldsLocked(c.sess, c.p, crab, now.Add(-time.Millisecond))
	c.w.mu.Unlock()
	if crab.HP != 1992 {
		t.Errorf("a millisecond before its end: crab %d HP", crab.HP)
	}

	// cast, worn off at its 8.3 minutes, the next hit not answered
	c2, crab2 := casterAndCrab(t)
	c2.w.StartCast(c2.sess, barbshield(), c2.me, c2.at(0))
	landed := 2*time.Second + castLand
	c2.tick(landed)
	c2.tick(landed + 498*time.Second)
	c2.sent()
	if len(c2.p.Effects) != 0 {
		t.Fatalf("Barbshield still on after 8.3 minutes: %+v", c2.p.Effects)
	}
	c2.w.mu.Lock()
	c2.w.npcAttackPlayer(crab2, c2.p.ServerID, c2.at(landed+499*time.Second))
	c2.w.mu.Unlock()
	if got := c2.sent(); len(got) != 1 || crab2.HP != 2000 {
		t.Errorf("after its time: sent %x, crab %d HP", got, crab2.HP)
	}
}

// Of several shields only the strongest answers (ours); a defensive proc on
// beside it rolls its own.
func TestTheStrongestShieldAnswers(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	far := now.Add(time.Minute)
	effects := []*ActiveEffect{shieldEffect("Barbcoat", 8, 0, far), shieldEffect("Thornshield", 22, 0, far), shieldEffect("Bramblecoat", 12, 0, far)}
	notUndead := func() bool { return false }
	hits, _ := shieldAnswers(effects, 44, 44, 0, notUndead, now, dice(0))
	if len(hits) != 1 || hits[0] != (shieldHit{n: 22, name: "Thornshield"}) {
		t.Errorf("three shields: %+v, want Thornshield's 22 alone", hits)
	}
	// a range rolls within it
	effects[1].Shield = &Shield{Min: 20, Max: 24}
	if hits, _ := shieldAnswers(effects, 44, 44, 0, notUndead, now, dice(3)); len(hits) != 1 || hits[0].n != 23 {
		t.Errorf("a range of 20-24, roll 3: %+v", hits)
	}
	// and a defensive proc beside it, under its chance and unresisted
	effects = append(effects, shieldEffect("A Ward", 0, 50, far))
	hits, _ = shieldAnswers(effects, 44, 44, 0, notUndead, now, dice(3, 0, 99))
	if len(hits) != 2 || hits[0].n != 23 || hits[1] != (shieldHit{n: 50, name: "A Ward", proc: true}) {
		t.Errorf("a shield and a defensive proc: %+v", hits)
	}
}

// A defensive proc fires on defProcChancePct of the hits (ours), unless the
// creature resists it as it would an offensive proc; when it fires, a
// 0x00B1 of the bearer's and its damage, as a shield's.
func TestADefensiveProcsChance(t *testing.T) {
	defer func(f func(int) int) { procIntn = f }(procIntn)
	now := time.Unix(1_800_000_000, 0)
	effects := []*ActiveEffect{shieldEffect("A Ward", 0, 50, now.Add(time.Minute))}
	notUndead := func() bool { return false }
	for _, c := range []struct {
		why   string
		rolls []int
		want  int
	}{
		{"under the chance, not resisted", []int{0, 99}, 50},
		{"the chance's last", []int{defProcChancePct - 1, 99}, 50},
		{"at the chance", []int{defProcChancePct, 99}, 0},
		{"over it", []int{99, 99}, 0},
		{"resisted (5% at its level)", []int{0, 4}, 0},
		{"the resist's first miss", []int{0, 5}, 50},
	} {
		hits, resisted := shieldAnswers(effects, 20, 20, 0, notUndead, now, dice(c.rolls...))
		got := 0
		if len(hits) == 1 {
			got = hits[0].n
		}
		if got != c.want || len(hits) > 1 || (c.why == "resisted (5% at its level)") != (resisted == 1) {
			t.Errorf("%s: %+v, %d resisted", c.why, hits, resisted)
		}
	}

	// on a player: the crab's hit answered by the proc
	cc, crab := casterAndCrab(t)
	cc.p.Effects = []*ActiveEffect{shieldEffect("A Ward", 0, 50, cc.at(time.Hour))}
	procIntn = dice(0, 99)
	cc.w.mu.Lock()
	cc.w.npcAttackPlayer(crab, cc.p.ServerID, cc.at(time.Second))
	cc.w.mu.Unlock()
	want := [][]byte{damageMsg(t, cc.npcO, -17, cc.me), castMsg(cc.me, cc.npcO, 0, fxShieldHit), damageMsg(t, cc.me, -50, cc.npcO)}
	if got := cc.sent(); !sameMsgs(got, want) || crab.HP != 1950 {
		t.Errorf("the proc on a hit: % X, crab %d HP", got, crab.HP)
	}
	procIntn = dice(99, 99)
	cc.w.mu.Lock()
	cc.w.npcAttackPlayer(crab, cc.p.ServerID, cc.at(2*time.Second))
	cc.w.mu.Unlock()
	if got := cc.sent(); len(got) != 1 || crab.HP != 1950 {
		t.Errorf("a hit over the chance: %x, crab %d HP", got, crab.HP)
	}
}

// The ranger's Spikecoat said a hit of 22 on its caster: landed on a ranger
// of 1 HP it takes none, and a crab's hit the ranger survives takes it down
// by the hit alone, the 22 the crab's. A hit that kills the bearer is not
// answered: its effects end with it.
func TestAShieldNeverKillsItsBearer(t *testing.T) {
	c, crab := casterAndCrab(t)
	c.p.Class, c.p.Level, c.p.HP = 1, 44, 1
	spike := CastSpell{SpellID: 700511, DisplayIndex: 4, Level: 44, Scope: 0, Name: "Spikecoat", Icon: 0x3A3B3C3D,
		CastTime: 3 * time.Second, Recast: 4 * time.Second, PowerCost: 7,
		Def: ParseSpellNamed("Spikecoat", "Self, Decrease Hitpoints by 22, Increase AC by 110, 7.5min", "")}
	if !c.w.StartCast(c.sess, spike, 0xFFFFFFFF, c.at(0)) {
		t.Fatalf("Spikecoat did not begin: %x", c.sent())
	}
	c.tick(3*time.Second + castLand)
	if c.p.IsDead() || c.p.HP != 1 || len(c.p.Effects) != 1 || c.p.Effects[0].Shield == nil || c.p.Effects[0].Shield.Max != 22 ||
		c.p.Effects[0].Mods.AC != 110 {
		t.Fatalf("Spikecoat on a ranger of 1 HP: HP %d, effects %+v", c.p.HP, c.p.Effects)
	}
	c.sent()

	// a hit it survives, at 1 HP after it
	c.p.HP = 18
	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(10*time.Second))
	c.w.mu.Unlock()
	if c.p.IsDead() || c.p.HP != 1 || crab.HP != 2000-22 {
		t.Errorf("a 17 on 18 HP: the ranger %d HP (dead %v), the crab %d", c.p.HP, c.p.IsDead(), crab.HP)
	}
	// a hit that kills it: not answered
	hp := crab.HP
	c.w.mu.Lock()
	c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(11*time.Second))
	c.w.mu.Unlock()
	if !c.p.IsDead() || crab.HP != hp || len(c.p.Effects) != 0 {
		t.Errorf("the killing hit: dead %v, crab %d (was %d), effects %+v", c.p.IsDead(), crab.HP, hp, c.p.Effects)
	}
}

// A pet's: the magician's Frozen Mark on its Fire Elemental, a level 20
// line naming no damage (10, ours), answers the crab's hits on the pet: the
// 0x00B1 the pet's, the figure its owner's as the pet's hits are, the
// crab's hate for the pet.
func TestAPetsShieldAnswersForItsOwner(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	mark := CastSpell{SpellID: 664, DisplayIndex: 20, Level: 20, Scope: 3, Name: "Frozen Mark", CastTime: 2 * time.Second, PowerCost: 50, Range: 30,
		Def: ParseSpellNamed("Frozen Mark", "Pet, Increase INT by 25, Add Damage Shield, 10min", "")}
	if !r.w.StartCast(r.sess, mark, 0xFFFFFFFF, r.at(20*time.Second)) {
		t.Fatalf("Frozen Mark did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(22*time.Second + castLand))
	r.w.mu.Unlock()
	if len(pet.Effects) != 1 || pet.Effects[0].Shield == nil || *pet.Effects[0].Shield != (Shield{Min: 10, Max: 10}) {
		t.Fatalf("on the pet: %+v", pet.Effects)
	}
	r.sent()

	r.w.mu.Lock()
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second)) // the swing
	r.sent()
	hp, crab := pet.HP, r.enemy.HP
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second+500*time.Millisecond))
	r.w.mu.Unlock()
	want := [][]byte{castMsg(petID, 0x0133E8A0, 0, fxShieldHit), damageMsg(t, r.me, -10, 0x0133E8A0)}
	if got := r.sent(); pet.HP >= hp || pet.Dead || r.enemy.HP != crab-10 || r.enemy.Threats[pet.ServerID] < 10 || !sameMsgs(got, want) {
		t.Errorf("the pet %d HP (was %d), the crab %d (was %d), its hate for the pet %d; sent\n% X\nwant\n% X",
			pet.HP, hp, r.enemy.HP, crab, r.enemy.Threats[pet.ServerID], got, want)
	}

	// the control: the mark off, the crab's next hit is its own alone
	pet.Effects = nil
	crab = r.enemy.HP
	r.w.mu.Lock()
	r.w.npcFightPetLocked(r.enemy, pet, r.at(31*time.Second+500*time.Millisecond))
	r.w.mu.Unlock()
	if r.enemy.HP != crab {
		t.Errorf("no mark: the crab lost %d", crab-r.enemy.HP)
	}
}

// A training dummy's hits are answered as a creature's: the figure told
// between the hit and the dummy's heal, taken off nothing (Sandstorm's
// dummies under Shroud of Hate's). A sparring dummy's melee is answered; its
// element strike, a spell, is not.
func TestADummysHitsAreAnswered(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv35", 35)
	c.p.AC = 600 // the Lv35's floor, 14, every hit
	c.w.mu.Lock()
	c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	c.p.Effects = []*ActiveEffect{shieldEffect("Chilling Aura", 2, 0, c.at(time.Hour))}
	c.sent()
	c.dummyTick(0) // its swing
	got := c.dummyTick(500 * time.Millisecond)
	want := [][]byte{damageMsg(t, c.npcO, -14, c.me), castMsg(c.me, c.npcO, 0, fxShieldHit), damageMsg(t, c.me, -2, c.npcO), damageMsg(t, c.npcO, 300, c.me)}
	if !sameMsgs(got, want) || c.npc.HP != 4000 || c.npc.Dead || c.npc.DummyTarget != c.p.ServerID {
		t.Errorf("the dummy's hit sent\n% X\nwant\n% X\nits HP %d, dead %v", got, want, c.npc.HP, c.npc.Dead)
	}

	s := newDummy(t, "Sparring Dummy", 30)
	s.p.AC, s.p.HP, s.p.MaxHP = 600, 5000, 5000
	s.p.Effects = []*ActiveEffect{shieldEffect("Chilling Aura", 2, 0, s.at(time.Hour))}
	s.w.mu.Lock()
	s.npc.DummyTarget = s.p.ServerID
	s.w.dummyHitLocked(s.sess, s.npc, s.p, s.at(time.Second))
	melee := s.sent()
	s.w.sparHitLocked(s.sess, s.npc, s.p, 120, "Sparring Dummy strikes you with Fire for 120 (your resist took 0%).", s.at(2*time.Second))
	strike := s.sent()
	s.w.mu.Unlock()
	if len(melee) != 3 || !bytes.Equal(melee[1], castMsg(s.me, s.npcO, 0, fxShieldHit)) || !bytes.Equal(melee[2], damageMsg(t, s.me, -2, s.npcO)) {
		t.Errorf("the sparring dummy's melee sent % X", melee)
	}
	for _, m := range strike {
		if opOfMsg(m) == 0x00B1 {
			t.Errorf("its strike was answered: % X", strike)
		}
	}
}

// Divine Protection's record says it hurts undead: it answers the rotting
// flesh, not a crab.
func TestAShieldForUndeadAlone(t *testing.T) {
	c := newCaster(t)
	c.p.AC = 100000
	c.p.Effects = []*ActiveEffect{{SpellID: 233, Name: "Divine Protection", Until: c.at(time.Hour), Shield: &Shield{Min: 2, Max: 2, Undead: true}}}
	flesh := c.npc // "Rotting Flesh": undead by its name (undead.go)
	flesh.HP, flesh.MaxHP = 300, 300
	c.w.mu.Lock()
	c.w.npcAttackPlayer(flesh, c.p.ServerID, c.at(time.Second))
	c.w.mu.Unlock()
	if flesh.HP != 298 {
		t.Errorf("the rotting flesh took %d, want 2", 300-flesh.HP)
	}
	flesh.Name = "Desert Crab"
	c.w.mu.Lock()
	c.w.npcAttackPlayer(flesh, c.p.ServerID, c.at(2*time.Second))
	c.w.mu.Unlock()
	if flesh.HP != 298 {
		t.Errorf("a crab took %d", 298-flesh.HP)
	}
}

// A shield's damage when its line names none, at the landing: half its
// level, at least 2 (ours); Shroud of Hate its bearer's level plus 2 to 3
// (Sandstorm's 52-53 on a level 50); a number in the line comes first.
func TestAShieldsDamageWhenItsLineNamesNone(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	land := func(name, line string, spellLevel, casterLevel int) Shield {
		cs := &castState{spell: CastSpell{SpellID: 1, Name: name, Level: spellLevel, Def: ParseSpellNamed(name, line, "")}}
		ae := lasting(cs, &Player{Level: casterLevel}, now)
		if ae == nil || ae.Shield == nil {
			t.Fatalf("%s: nothing lasts", name)
		}
		return *ae.Shield
	}
	for _, c := range []struct {
		name, line   string
		spell, level int
		want         Shield
	}{
		{"Thistleshield", "Target, Damageshield, 8.3min", 8, 30, Shield{Min: 4, Max: 4}},
		{"Smoldering Aura", "Target, DamageShield, 5min", 3, 30, Shield{Min: 2, Max: 2}},
		{"Searing Aura", "Target, Damage Shield, 8min, FR", 60, 60, Shield{Min: 30, Max: 30}},
		{"Shroud of Hate", "", 6, 50, Shield{Min: 52, Max: 53}},
		{"Shroud of Hate", "", 6, 6, Shield{Min: 8, Max: 9}},
		{"Shroud of Hate", "Self, Damage Shield of 40, 5min", 6, 50, Shield{Min: 40, Max: 40}},
		{"Spirit Shield", "Target, approx. 27 Damage to Mob when it hits, 10min", 60, 60, Shield{Min: 27, Max: 27}},
		{"A Shield", "Target, Damage Shield of 10 ; Increase 10% based on WIS, 8min", 30, 30, Shield{Min: 10, Max: 10}},
	} {
		if got := land(c.name, c.line, c.spell, c.level); got != c.want {
			t.Errorf("%s (spell level %d, caster %d): %+v, want %+v", c.name, c.spell, c.level, got, c.want)
		}
	}
	// a scale is its caster's at the landing: WIS 87's 10% is 8 more
	cs := &castState{spell: CastSpell{SpellID: 1, Name: "A Shield", Level: 30,
		Def: ParseSpellNamed("A Shield", "Target, Damage Shield of 10 ; Increase 10% based on WIS, 8min", "")}}
	if ae := lasting(cs, &Player{Level: 30, Wis: 87}, now); ae.Shield.Min != 18 || ae.Shield.Max != 18 {
		t.Errorf("10 and 10%% of WIS 87: %+v", ae.Shield)
	}
}

// Kept over a logout: a shield and a defensive proc ride in the mods
// column; a row kept before shields reads as none.
func TestAShieldIsKeptOverALogout(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	p := &Player{Effects: []*ActiveEffect{
		{SpellID: 700515, Name: "Thornshield", Until: now.Add(time.Minute), Shield: &Shield{Min: 22, Max: 22}},
		{SpellID: 233, Name: "Divine Protection", Until: now.Add(time.Minute), Shield: &Shield{Min: 2, Max: 2, Undead: true}},
		{SpellID: 9001, Name: "A Ward", Until: now.Add(time.Minute), Mods: StatMods{AC: 5}, Shield: &Shield{ProcMin: 40, ProcMax: 60}},
	}}
	rows := savedEffects(p, now)
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7000}, 1, 1, 1, 2, true)
	q := &Player{ServerID: 5}
	w.Players[5] = q
	s.SetECSEntity(q)
	if n := w.RestoreEffects(s, rows, now); n != 3 {
		t.Fatalf("restored %d", n)
	}
	for i, e := range p.Effects {
		if q.Effects[i].Shield == nil || *q.Effects[i].Shield != *e.Shield {
			t.Errorf("%s came back as %+v", e.Name, q.Effects[i].Shield)
		}
	}
	if q.Effects[2].Mods.AC != 5 {
		t.Errorf("A Ward's AC came back as %d", q.Effects[2].Mods.AC)
	}
	old := rows[0]
	old.Mods, old.SpellID = `{"Str":5}`, 99
	if w.RestoreEffects(s, append(rows[:0:0], old), now); q.Effects[3].Shield != nil || q.Effects[3].Mods.Str != 5 {
		t.Errorf("an old row: %+v", q.Effects[3])
	}
}

// Every shield of EQOA-Data's lists is read as one, never cast at an
// enemy, the numbered ones with their numbers (the counts in dshield.go's
// comment).
func TestEQOADataShieldsRead(t *testing.T) {
	// by level and name: the cleric's level 51 Divine Protection is
	// "Invulnerable for 12 seconds"
	numbers := map[string]int{"1 Divine Protection": 2, "1 Thistlecoat": 4, "12 Barbcoat": 8, "24 Bramblecoat": 12, "44 Spikecoat": 22,
		"55 Bladecoat": 30, "1 Chilling Aura": 2, "1 O'Keils Radiation": 6, "8 O'Keils Embers": 10, "60 Spirit Shield": 27}
	n, seen := 0, map[string]bool{}
	for _, s := range eqoaDataSpells(t) {
		d := ParseSpellNamed(s.Name, s.Line, "")
		var sh *SpellEffect
		for i := range d.Effects {
			if d.Effects[i].Kind == EffectShield {
				sh = &d.Effects[i]
			}
		}
		key := itoaPad(s.Level) + " " + s.Name
		if want, ok := numbers[key]; ok {
			seen[key] = true
			if sh == nil || sh.Amount != want {
				t.Errorf("%s %q: %+v, want a shield of %d", s.Name, s.Line, d.Effects, want)
			}
		}
		if sh == nil {
			continue
		}
		n++
		if d.Harmful {
			t.Errorf("%s %q is cast at an enemy", s.Name, s.Line)
		}
	}
	if n < 41 || len(seen) != len(numbers) {
		t.Errorf("%d shields read (41 in c78767f), %d of the numbered ones found", n, len(seen))
	}
}
