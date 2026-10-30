package entity

import (
	"testing"
	"time"
)

// Wards (ward.go; the owner's ruling of 2026-10-29 on the epics' notes): the
// words, a creature's hit cut by a percent or to 1, a cap that ends one, the
// strongest of several, and what is never cut. The spells and figures here
// are made up.

// wardSpell is a made-up ward cast at the caster's target.
func wardSpell(id int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Scope: 1, Name: name, Icon: 0x2B2C2D2E,
		CastTime: 2 * time.Second, Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// wardEffect is a ward on its bearer until the time given.
func wardEffect(id int, name string, wd Ward, until time.Time) *ActiveEffect {
	return &ActiveEffect{SpellID: id, Name: name, Until: until, Ward: &wd}
}

// The ward's words are read as one, whom it lands on and how long, never
// harmful; and the controls are not.
func TestTheWardWordsAreRead(t *testing.T) {
	for _, c := range []struct {
		line   string
		target SpellTarget
		dur    time.Duration
		pct    int
		one    bool
		cap    int
	}{
		{"Self, Reduce Damage Taken by 50%, 30sec", TargetSelf, 30 * time.Second, 50, false, 0},
		{"Target, Reduce Damage Taken to 1, max 2500, 2min", TargetSingle, 2 * time.Minute, 0, true, 2500},
		{"Group, Reduce Damage Taken by 40% max 900, 45sec", TargetGroup, 45 * time.Second, 40, false, 900},
		{"Pet, Reduce Damage Taken by 25 %, 1min", TargetPet, time.Minute, 25, false, 0},
		{"Self, Reduce Damage Taken by 100%, 10sec", TargetSelf, 10 * time.Second, 100, false, 0},
		// no time: a buff's (spellnotes.go)
		{"Self, Reduce Damage Taken to 1, max 1200", TargetSelf, buffDuration, 0, true, 1200},
	} {
		d := ParseSpellNamed("Made-up Ward", c.line, "")
		if len(d.Effects) != 1 || len(d.Unread) != 0 || d.Harmful || d.Target != c.target || d.Duration != c.dur {
			t.Errorf("%q: %+v", c.line, d)
			continue
		}
		if e := d.Effects[0]; e.Kind != EffectWard || e.Amount != c.pct || e.ToOne != c.one || e.Cap != c.cap {
			t.Errorf("%q: the ward %+v", c.line, e)
		}
	}
	for _, line := range []string{
		"Self, Reduce Damage Taken by 0%, 30sec",
		"Self, Reduce Damage Taken by 101%, 30sec",
		"Self, Reduce Damage Taken to 2, 30sec",
		"Self, Reduce Damage Taken, 30sec",
	} {
		d := ParseSpellNamed("Made-up Ward", line, "")
		for _, e := range d.Effects {
			if e.Kind == EffectWard {
				t.Errorf("%q read as a ward: %+v", line, d)
			}
		}
		if len(d.Unread) != 1 {
			t.Errorf("%q: unread %q", line, d.Unread)
		}
	}
}

// A ward cast on the druid cuts the crab's hit (17, its floor) by its
// percent, the cut rounded down: 8 off, 9 taken, and 9 is the figure the
// druid is shown. The control: no ward, the whole 17.
func TestAWardCutsACreaturesHit(t *testing.T) {
	c, _ := casterAndCrab(t)
	c.land(t, wardSpell(9501, "Made-up Bulwark", "Target, Reduce Damage Taken by 50%, 1min"), c.me, 0)
	if len(c.p.Effects) != 1 || c.p.Effects[0].Ward == nil || *c.p.Effects[0].Ward != (Ward{Pct: 50}) ||
		len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Made-up Bulwark" {
		t.Fatalf("on the druid: %+v, buffs %+v", c.p.Effects, c.p.Buffs)
	}
	for i := range 2 {
		hp := c.p.HP
		c.w.mu.Lock()
		c.w.npcAttackPlayer(c.npc, c.p.ServerID, c.at(10*time.Second+time.Duration(i)*time.Second))
		c.w.mu.Unlock()
		want := [][]byte{damageMsg(t, c.npcO, -9, c.me)}
		if got := c.sent(); c.p.HP != hp-9 || !sameMsgs(got, want) {
			t.Errorf("hit %d: HP %d (was %d), sent % X, want % X", i, c.p.HP, hp, got, want)
		}
	}
	if c.p.Effects[0].Ward.Taken != 16 {
		t.Errorf("the ward took %d, want 16", c.p.Effects[0].Ward.Taken)
	}

	n, crab := casterAndCrab(t)
	hp := n.p.HP
	n.w.mu.Lock()
	n.w.npcAttackPlayer(crab, n.p.ServerID, n.at(time.Second))
	n.w.mu.Unlock()
	if got := n.sent(); n.p.HP != hp-17 || !sameMsgs(got, [][]byte{damageMsg(t, n.npcO, -17, n.me)}) {
		t.Errorf("no ward: HP %d (was %d), sent % X", n.p.HP, hp, got)
	}
}

// "to 1": every hit does 1, whatever it rolled.
func TestAWardToOneMakesEachHitOne(t *testing.T) {
	c, crab := casterAndCrab(t)
	c.land(t, wardSpell(9502, "Made-up Rune", "Target, Reduce Damage Taken to 1, 1min"), c.me, 0)
	hp := c.p.HP
	for i := range 3 {
		c.w.mu.Lock()
		c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(10*time.Second+time.Duration(i)*time.Second))
		c.w.mu.Unlock()
		if got := c.sent(); !sameMsgs(got, [][]byte{damageMsg(t, c.npcO, -1, c.me)}) {
			t.Errorf("hit %d sent % X, want a 1", i, got)
		}
	}
	if c.p.HP != hp-3 {
		t.Errorf("three hits: HP %d, want %d", c.p.HP, hp-3)
	}
}

// A ward's max ends it: to 1 with a cap of 40 takes 16 and 16 off the crab's
// 17s, then the 8 it has left (9 taken), and is gone, its icon too; the next
// hit is whole. A spent ward on an effect that also roots leaves the root.
func TestAWardEndsAtItsMax(t *testing.T) {
	c, crab := casterAndCrab(t)
	c.land(t, wardSpell(9503, "Made-up Rune", "Target, Reduce Damage Taken to 1, max 40, 1min"), c.me, 0)
	var got []int
	for i := range 4 {
		hp := c.p.HP
		c.w.mu.Lock()
		c.w.npcAttackPlayer(crab, c.p.ServerID, c.at(10*time.Second+time.Duration(i)*time.Second))
		c.w.mu.Unlock()
		c.sent()
		got = append(got, hp-c.p.HP)
	}
	if len(got) != 4 || got[0] != 1 || got[1] != 1 || got[2] != 9 || got[3] != 17 {
		t.Errorf("four hits took %v, want [1 1 9 17]", got)
	}
	if len(c.p.Effects) != 0 || len(c.p.Buffs) != 0 {
		t.Errorf("a spent ward still on: %+v, buffs %+v", c.p.Effects, c.p.Buffs)
	}

	r, crab2 := casterAndCrab(t)
	held := wardEffect(9504, "Made-up Stance", Ward{Pct: 50, Cap: 5}, r.at(time.Hour))
	held.Root = true
	r.p.Effects = []*ActiveEffect{held}
	r.w.mu.Lock()
	taken := r.w.playerWardLocked(r.p, crab2, 17, r.at(time.Second))
	r.w.mu.Unlock()
	if taken != 12 || len(r.p.Effects) != 1 || r.p.Effects[0].Ward != nil || !r.p.Effects[0].Root {
		t.Errorf("a ward beside a root: took %d, effects %+v", taken, r.p.Effects)
	}
}

// Only the strongest ward counts (ours, as the strongest damage shield):
// the higher percent; on a tie the more left to take, no cap the most; a
// ward to 1 over 99% and under 100%; one past its time not at all.
func TestTheStrongestWardCounts(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	far := now.Add(time.Minute)
	for _, c := range []struct {
		why   string
		wards []*ActiveEffect
		hit   int
		taken int
		which string
	}{
		{"the higher percent", []*ActiveEffect{
			wardEffect(1, "a third", Ward{Pct: 35}, far), wardEffect(2, "two thirds", Ward{Pct: 67}, far)}, 30, 10, "two thirds"},
		{"no cap over a cap", []*ActiveEffect{
			wardEffect(1, "capped", Ward{Pct: 50, Cap: 100}, far), wardEffect(2, "uncapped", Ward{Pct: 50}, far)}, 30, 15, "uncapped"},
		{"the more left", []*ActiveEffect{
			wardEffect(1, "nearly spent", Ward{Pct: 50, Cap: 100, Taken: 90}, far), wardEffect(2, "fresh", Ward{Pct: 50, Cap: 50}, far)}, 30, 15, "fresh"},
		{"to 1 over 99%", []*ActiveEffect{
			wardEffect(1, "ninety-nine", Ward{Pct: 99}, far), wardEffect(2, "to one", Ward{One: true}, far)}, 300, 1, "to one"},
		{"100% over to 1", []*ActiveEffect{
			wardEffect(1, "to one", Ward{One: true}, far), wardEffect(2, "all", Ward{Pct: 100}, far)}, 300, 0, "all"},
		{"one past its time", []*ActiveEffect{
			wardEffect(1, "over", Ward{One: true}, now), wardEffect(2, "a quarter", Ward{Pct: 25}, far)}, 40, 30, "a quarter"},
	} {
		before := make([]int, len(c.wards))
		for i, e := range c.wards {
			before[i] = e.Ward.Taken
		}
		taken, ae, cut, spent := wardHit(c.wards, c.hit, now)
		if ae == nil || ae.Name != c.which || taken != c.taken || cut != c.hit-c.taken || spent {
			t.Errorf("%s: took %d by %+v (cut %d, spent %v), want %d by %q", c.why, taken, ae, cut, spent, c.taken, c.which)
			continue
		}
		for i, e := range c.wards {
			if e != ae && e.Ward.Taken != before[i] {
				t.Errorf("%s: %q took %d too", c.why, e.Name, e.Ward.Taken-before[i])
			}
		}
	}
	if taken, ae, _, _ := wardHit(nil, 30, now); taken != 30 || ae != nil {
		t.Errorf("no ward: took %d by %+v", taken, ae)
	}
}

// A ward cuts no drowning and no self line's price: under water with a ward
// to 1 on, a tenth of the health goes; a self line's 40 is paid whole.
func TestAWardLeavesDrowningAndAPriceAlone(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 500, 500
	c.p.Effects = []*ActiveEffect{wardEffect(9505, "Made-up Rune", Ward{One: true}, time.Now().Add(time.Hour))}
	c.p.Breath, c.p.WalkFlags, c.p.HasWalkFlags = 0, 0x0004, true
	c.w.mu.Lock()
	for range 10 {
		c.w.TickCount = 6
		c.w.breathSystem(time.Now())
	}
	c.w.mu.Unlock()
	if c.p.HP != 450 {
		t.Errorf("drowning with a ward on: HP %d, want 450", c.p.HP)
	}

	s := newCaster(t)
	s.p.HP = 500
	s.p.Effects = []*ActiveEffect{wardEffect(9506, "Made-up Rune", Ward{One: true}, s.at(time.Hour))}
	s.land(t, selfSpell(9507, "Made-up Fervor", "Self, Decrease Hitpoints by 40, Increase STR by 5, 1min"), s.me, 0)
	if s.p.HP != 460 {
		t.Errorf("a self line's price with a ward on: HP %d, want 460", s.p.HP)
	}
}

// A pet's ward cuts a creature's hits on it, as a player's: to 1, one off the
// pet a hit.
func TestAPetsWardCutsTheCreaturesHits(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	pet.X, pet.Z = r.enemy.X-2, r.enemy.Z
	ward := CastSpell{SpellID: 9508, DisplayIndex: 21, Level: 20, Scope: 3, Name: "Made-up Pet Rune", CastTime: 2 * time.Second, PowerCost: 50, Range: 30,
		Def: ParseSpellNamed("Made-up Pet Rune", "Pet, Reduce Damage Taken to 1, 1min", "")}
	if !r.w.StartCast(r.sess, ward, 0xFFFFFFFF, r.at(20*time.Second)) {
		t.Fatalf("the ward did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(22*time.Second + castLand))
	r.w.mu.Unlock()
	if len(pet.Effects) != 1 || pet.Effects[0].Ward == nil || !pet.Effects[0].Ward.One {
		t.Fatalf("on the pet: %+v", pet.Effects)
	}
	r.w.mu.Lock()
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second)) // the swing
	hp := pet.HP
	r.w.npcFightPetLocked(r.enemy, pet, r.at(30*time.Second+500*time.Millisecond))
	r.w.mu.Unlock()
	if pet.HP != hp-1 {
		t.Errorf("the crab's hit on the warded pet: HP %d (was %d), want 1 off", pet.HP, hp)
	}
}

// A training dummy's hit is a creature's: cut by the ward (14 to 7, the
// figure shown 7), then the dummy's heal as ever.
func TestADummysHitIsWarded(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv35", 35)
	c.p.AC = 600 // the Lv35's floor, 14, every hit
	c.p.HP = 100
	c.p.Effects = []*ActiveEffect{wardEffect(9509, "Made-up Bulwark", Ward{Pct: 50}, c.at(time.Hour))}
	c.w.mu.Lock()
	c.w.dummyHitLocked(c.sess, c.npc, c.p, c.at(time.Second))
	c.w.mu.Unlock()
	want := [][]byte{damageMsg(t, c.npcO, -7, c.me), damageMsg(t, c.npcO, dummyHeal, c.me)}
	if got := c.sent(); !sameMsgs(got, want) || c.p.HP != min(100-7+dummyHeal, c.p.MaxHPNow()) {
		t.Errorf("a dummy's hit on a ward: HP %d, sent % X, want % X", c.p.HP, got, want)
	}
}

// A ward is kept over a logout with what it has taken.
func TestAWardIsKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	c.p.Effects = []*ActiveEffect{wardEffect(9510, "Made-up Rune", Ward{One: true, Cap: 300, Taken: 120}, c.at(time.Minute))}
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(0))
	if !ok || len(rows) != 1 {
		t.Fatalf("kept %d (%v)", len(rows), ok)
	}
	d := newCaster(t)
	if n := d.w.RestoreEffects(d.sess, rows, d.at(time.Hour)); n != 1 {
		t.Fatalf("restored %d", n)
	}
	if w := d.p.Effects[0].Ward; w == nil || *w != (Ward{One: true, Cap: 300, Taken: 120}) {
		t.Errorf("the ward came back as %+v", w)
	}
}
