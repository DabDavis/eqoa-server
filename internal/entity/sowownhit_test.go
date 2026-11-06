package entity

import (
	"testing"
	"time"
)

// Petition 1109: "Spirit of the wolf is dropped when hit, it should only drop
// when player hits mob". Spirit of the Wolf as the live record 901140 has it:
// a run speed and a Remove Ability that reads as one, each marked to end at a
// cast and at contact.
const sowRecordJSON = `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":1800,"stack":1,"effects":[
	{"id":"0575","type":"Increase Speed","kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true},
	{"id":"0576","type":"Remove Ability","kind":"cure","removes":"ID?","dispel_on_cast":true,"dispel_on_contact":true},
	{"id":"0576","type":"Remove Ability","kind":"speed","amount":130,"dispel_on_cast":true,"dispel_on_contact":true}]}`

// Through the game's own paths: a creature's melee hit leaves it on, nothing
// told; the press alone leaves it on; the player's first swing at the creature
// ends it, the run speed told. On the code before, the hit ended it and the
// swing did not.
func TestARunSpeedEndsAtItsBearersSwingNotAtAHitTaken(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	c.land(t, wbSpell(9941, 33, 0, "Test Wolf Spirit", sowRecordJSON), 0xFFFFFFFF, 0)
	if effectNamed(c.p.Effects, 9941) == nil || c.p.SpeedNow() <= run {
		t.Fatalf("landed: on %v, speed %v (run %v)", effectNamed(c.p.Effects, 9941) != nil, c.p.SpeedNow(), run)
	}

	hp := c.p.HP
	c.w.mu.Lock()
	c.w.npcAttackPlayer(c.npc, c.p.ServerID, c.at(10*time.Second))
	c.w.mu.Unlock()
	if c.p.HP >= hp {
		t.Fatalf("the creature's hit took nothing (%d of %d HP): the test proves nothing", c.p.HP, hp)
	}
	if effectNamed(c.p.Effects, 9941) == nil || c.p.SpeedNow() <= run || sentSpeed(c.sent()) {
		t.Fatalf("after a hit taken: on %v, speed %v (run %v); want it on, fast, nothing told", effectNamed(c.p.Effects, 9941) != nil, c.p.SpeedNow(), run)
	}

	c.p.AutoAttack = true // the button held
	swing := Swing{Kind: 0, Delay: 3 * time.Second, Reach: 4.5}
	if !c.w.EngageAttack(c.sess, c.npcO, swing, c.at(20*time.Second)) {
		t.Fatal("not engaged")
	}
	if effectNamed(c.p.Effects, 9941) == nil || c.p.SpeedNow() <= run {
		t.Fatalf("after the press: on %v, speed %v (run %v); want it on until the swing", effectNamed(c.p.Effects, 9941) != nil, c.p.SpeedNow(), run)
	}
	attack := func(d time.Duration) {
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		c.w.attackSystem(c.at(d))
	}
	c.sent()
	attack(20*time.Second + 300*time.Millisecond) // the round begun, its first hit not due
	attack(21 * time.Second)                      // and landed
	if v, ok := lastSpeedSent(c.sent()); effectNamed(c.p.Effects, 9941) != nil || c.p.SpeedNow() != run || !ok || v != run {
		t.Errorf("after its swing: on %v, speed %v, told %v %v; want it gone, the run's %v told", effectNamed(c.p.Effects, 9941) != nil,
			c.p.SpeedNow(), v, ok, run)
	}
}

// A thrown potion is its bearer's own hit too. The control: a throw with no run
// speed on ends nothing else (the stat beside it, not marked).
func TestARunSpeedEndsAtItsBearersThrow(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	c.land(t, wbSpell(9941, 33, 0, "Test Wolf Spirit", sowRecordJSON), 0xFFFFFFFF, 0)
	c.land(t, wbSpell(9942, 30, 0, "Test Might", `{"v":1,"scope":"self","target_type":"Self","duration_s":1800,"effects":[
		{"kind":"stat","stat":"str","amount":10}]}`), 0xFFFFFFFF, 5*time.Second)
	c.sent()
	if r := c.use(t, thrown(), c.npcO, 10*time.Second); r != "" {
		t.Fatalf("the throw refused: %q", r)
	}
	if effectNamed(c.p.Effects, 9941) != nil || c.p.SpeedNow() != run {
		t.Errorf("after a throw: on %v, speed %v (run %v); want it gone", effectNamed(c.p.Effects, 9941) != nil, c.p.SpeedNow(), run)
	}
	if effectNamed(c.p.Effects, 9942) == nil || c.p.Mods.Str != 10 {
		t.Errorf("the stat after a throw: on %v, STR %+d; want it on", effectNamed(c.p.Effects, 9942) != nil, c.p.Mods.Str)
	}
}

// Only the run speed changed its end, and only one so marked. What else a hit
// ends beside a speed still ends at the hit, the speed staying until its
// bearer's own hit ends the rest whole; a lone speed marked to end at contact
// alone (no cast) keeps its parts to stay through a hit, as one with both does;
// a speed the workbook does not mark (Selo's Accelerando) ends at neither.
func TestOnlyAMarkedRunSpeedChangedItsEnd(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	c.land(t, wbSpell(9943, 30, 0, "Test Brisk Veil", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"invis","dispel_on_contact":true},{"kind":"stat","stat":"agi","amount":10,"dispel_on_contact":true},
		{"kind":"speed","dispel_on_contact":true}]}`), 0xFFFFFFFF, 0)
	if !c.p.Invisible() || c.p.Mods.Agi != 10 {
		t.Fatalf("landed: invisible %v, AGI %+d", c.p.Invisible(), c.p.Mods.Agi)
	}
	c.contactPlayer()
	if effectNamed(c.p.Effects, 9943) == nil || c.p.SpeedNow() <= run || c.p.Invisible() || c.p.Mods.Agi != 0 {
		t.Fatalf("after a hit taken: on %v, speed %v (run %v), invisible %v, AGI %+d; want on, fast, seen, none",
			effectNamed(c.p.Effects, 9943) != nil, c.p.SpeedNow(), run, c.p.Invisible(), c.p.Mods.Agi)
	}
	c.ownHit()
	if effectNamed(c.p.Effects, 9943) != nil || c.p.SpeedNow() != run {
		t.Errorf("after its own hit: on %v, speed %v (run %v); want it ended whole", effectNamed(c.p.Effects, 9943) != nil, c.p.SpeedNow(), run)
	}

	d := bodyRig(t)
	d.land(t, wbSpell(9944, 30, 0, "Test Contact Stride", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[
		{"kind":"speed","dispel_on_contact":true}]}`), 0xFFFFFFFF, 0)
	d.contactPlayer()
	if effectNamed(d.p.Effects, 9944) == nil || d.p.SpeedNow() <= run {
		t.Errorf("marked at contact alone, after a hit taken: on %v, speed %v (run %v); want on, fast", effectNamed(d.p.Effects, 9944) != nil, d.p.SpeedNow(), run)
	}
	d.ownHit()
	if effectNamed(d.p.Effects, 9944) != nil || d.p.SpeedNow() != run {
		t.Errorf("marked at contact alone, after its own hit: on %v, speed %v (run %v); want it gone", effectNamed(d.p.Effects, 9944) != nil, d.p.SpeedNow(), run)
	}

	f := bodyRig(t)
	f.land(t, wbSpell(9945, 30, 0, "Test Plain Stride", `{"v":1,"scope":"self","target_type":"Self","duration_s":600,"effects":[{"kind":"speed"}]}`), 0xFFFFFFFF, 0)
	f.contactPlayer()
	f.ownHit()
	if effectNamed(f.p.Effects, 9945) == nil || f.p.SpeedNow() <= run {
		t.Errorf("not marked: on %v, speed %v (run %v); want on, fast", effectNamed(f.p.Effects, 9945) != nil, f.p.SpeedNow(), run)
	}
}

// A creature keeps its old end: its own run speed marked to end at contact ends
// at a hit on it (contactNPCLocked), as it did.
func TestACreaturesMarkedSpeedStillEndsAtAHit(t *testing.T) {
	npc := &NPC{ServerID: 7001, Name: "Test Hare", HP: 100, MaxHP: 100}
	e := &ActiveEffect{SpellID: 9946, Name: "Test Hare Stride", Speed: true, Rules: EffectRules{DispelOnContact: true},
		parts: []*effectPart{{Speed: true, Contact: true, Cast: true}}}
	npc.Effects = []*ActiveEffect{e}
	contactNPCLocked(npc)
	if len(npc.Effects) != 0 {
		t.Errorf("a creature's marked speed after a hit: %d effects, want it gone", len(npc.Effects))
	}
}

// Petition 1111: "SoW when cast on self does not seem to work? my dual box
// character runs so far ahead as opposed to keeping pace". A recast with Spirit
// of the Wolf still on: the old one's end told the client the plain run speed
// and the new one's landing compared the speed from before it, so the client
// ran plain while the server held the fast one. The last speed told is what the
// client runs at: the recast's must be the fast one, a talk at a board or the
// fang's use with one on the same. The control: the first cast tells the fast.
func TestARecastKeepsTheClientsRunSpeed(t *testing.T) {
	c := bodyRig(t)
	run := c.p.SpeedNow()
	sow := wbSpell(9941, 33, 0, "Test Wolf Spirit", sowRecordJSON)
	if !c.w.StartCast(c.sess, sow, 0xFFFFFFFF, c.at(0)) {
		t.Fatalf("the cast did not begin: %x", c.sent())
	}
	c.tick(sow.CastTime + castLand)
	fast := c.p.SpeedNow()
	if v, ok := lastSpeedSent(c.sent()); !ok || v != fast || fast <= run {
		t.Fatalf("the first cast told %v %v; want the fast %v (run %v)", v, ok, fast, run)
	}
	at := 10 * time.Second
	if !c.w.StartCast(c.sess, sow, 0xFFFFFFFF, c.at(at)) {
		t.Fatalf("the recast did not begin: %x", c.sent())
	}
	c.tick(at + sow.CastTime + castLand)
	if effectNamed(c.p.Effects, 9941) == nil || c.p.SpeedNow() != fast {
		t.Fatalf("after the recast: on %v, speed %v; want it on at %v", effectNamed(c.p.Effects, 9941) != nil, c.p.SpeedNow(), fast)
	}
	if v, ok := lastSpeedSent(c.sent()); ok && v != fast {
		t.Errorf("the recast left the client at %v; want the fast %v", v, fast)
	}

	d := bodyRig(t)
	if given, renewed := d.w.GiveWolf(d.sess, d.at(0), "the fang's"); !given || renewed {
		t.Fatalf("the fang: given %v renewed %v", given, renewed)
	}
	d.sent()
	if given, renewed := d.w.GiveWolf(d.sess, d.at(time.Minute), "the fang's"); !given || !renewed {
		t.Fatalf("the fang again: given %v renewed %v", given, renewed)
	}
	if v, ok := lastSpeedSent(d.sent()); ok && v != d.p.SpeedNow() {
		t.Errorf("the fang's renewal left the client at %v; want %v", v, d.p.SpeedNow())
	}
}
