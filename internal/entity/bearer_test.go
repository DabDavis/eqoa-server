package entity

import (
	"testing"
	"time"
)

// The line words a spell's numbers can need beyond a scale and a duration:
// a proc scaled by its bearer's stat, a cap, a count of ticks, a pet's level.
// The spells and numbers here are made up.

// glowRune is a proc of 40 plus 30% of its bearer's STR, at most 65.
func glowRune(line string) CastSpell {
	return CastSpell{SpellID: 9201, DisplayIndex: 9, Level: 20, Scope: 1, Name: "Glow Rune", Icon: 0x0A0B0C0D,
		CastTime: time.Second, Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpell(line, "Arcane Offensive Proc")}
}

const glowLine = "Target, Decrease Hitpoints by 40 ; Increase 30% based on bearer's STR ; max 65, 5min"

// The words read: "bearer's" (or "wearer's") marks the scale as the
// bearer's, "max N" caps the clause before it, "N ticks" counts the ticks,
// a summoning's "level N" is its pet's. The controls: the same lines without
// the words.
func TestTheBearerCapTickAndPetLevelWords(t *testing.T) {
	d := ParseSpell(glowLine, "Arcane Offensive Proc")
	want := SpellEffect{Kind: EffectProc, Amount: 40, AmountMax: 40, Scale: StatStr, ScalePct: 30, ScaleBearer: true, Cap: 65}
	if d.Harmful || d.Duration != 5*time.Minute || len(d.Effects) != 1 || d.Effects[0] != want || len(d.Unread) != 0 {
		t.Errorf("%q: %+v", glowLine, d)
	}
	if d := ParseSpell("Target, Decrease Hitpoints by 40 ; Increase 30% based on wearer's STR, 5min", "Arcane Offensive Proc"); len(d.Effects) != 1 || !d.Effects[0].ScaleBearer {
		t.Errorf("wearer's: %+v", d)
	}
	d = ParseSpell("Target, Decrease Hitpoints by 40 ; Increase 30% based on STR, 5min", "Arcane Offensive Proc")
	if len(d.Effects) != 1 || d.Effects[0].ScaleBearer || d.Effects[0].Cap != 0 || d.Effects[0].Scale != StatStr {
		t.Errorf("the control, no bearer and no cap: %+v", d)
	}

	d = ParseSpellText("Target, Increase Power per tic by 13 ; Increase 40% based on CHA, 3 ticks, 1min")
	if d.Ticks != 3 || d.Duration != time.Minute || len(d.Effects) != 1 || d.Effects[0].Kind != EffectPowerTick || len(d.Unread) != 0 {
		t.Errorf("3 ticks: %+v", d)
	}
	if d := ParseSpellText("Target, Increase Power per tic by 13, 1min"); d.Ticks != 0 {
		t.Errorf("the control, no count: %+v", d)
	}

	d = ParseSpellText("Summons a pet, level 21")
	if !d.Summons() || d.PetLevel != 21 || d.Pet != "" {
		t.Errorf("a pet of level 21: %+v", d)
	}
	if d := ParseSpellText("Summons a Clay Golem to aid you"); d.PetLevel != 0 || d.Pet != "clay golem" {
		t.Errorf("the control, no level: %+v", d)
	}
}

// A cap holds a scaled amount either way; the control: none.
func TestACapHoldsAnAmount(t *testing.T) {
	c := newCaster(t)
	c.p.Wis = 200
	for _, k := range []struct {
		line string
		want int
	}{
		{"Target, Increase HP by 30 ; Increase 50% based on WIS ; max 80", 80},
		{"Target, Decrease Hitpoints by 30 ; Increase 50% based on WIS ; max 80", -80},
		{"Target, Increase HP by 30 ; Increase 50% based on WIS", 130},
		{"Target, Increase HP by 30 ; max 80", 30},
	} {
		d := ParseSpellText(k.line)
		if len(d.Effects) != 1 {
			t.Fatalf("%q: %+v", k.line, d)
		}
		if n := spellAmount(d.Effects[0], c.p); n != k.want {
			t.Errorf("%q: %d, want %d", k.line, n, k.want)
		}
	}
}

// A proc scaled by its bearer's stat is the friend's, at each proc, held to
// its cap: on a friend of STR 50, 40 + 15; at STR 100, 40 + 30 held to 65.
// The control: the same proc scaled by its caster's STR (60) carries 40 + 18
// from its landing, whatever the friend's.
func TestABearersProcIsTheBearersStat(t *testing.T) {
	r := newProcRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	if !r.w.StartCast(r.sess, glowRune(glowLine), r.fid, r.at(0)) {
		t.Fatalf("the rune on a friend did not begin: %x", r.sent())
	}
	r.tick(time.Second + castLand)
	if len(r.friend.Effects) != 1 {
		t.Fatalf("on the friend: %+v", r.friend.Effects)
	}
	ae := r.friend.Effects[0]
	if ae.Proc != 40 || ae.ProcMax != 40 || ae.ProcScale != StatStr || ae.ProcScalePct != 30 || ae.ProcCap != 65 {
		t.Fatalf("the rune: %+v", ae)
	}
	proc := func(str int, at time.Duration) int {
		r.friend.Str = str
		hp := r.npc.HP
		procIntn = dice(0, 99)
		r.w.mu.Lock()
		r.w.playerProcsLocked(r.fsess, r.friend, r.npc, r.at(at))
		r.w.mu.Unlock()
		return hp - r.npc.HP
	}
	r.npc.HP, r.npc.MaxHP = 5000, 5000
	if n := proc(50, 10*time.Second); n != 55 {
		t.Errorf("STR 50: the proc took %d, want 55", n)
	}
	if n := proc(100, 11*time.Second); n != 65 {
		t.Errorf("STR 100: the proc took %d, want 65 (its cap)", n)
	}

	// the control: the caster's
	r.friend.Effects = nil
	r.land(t, glowRune("Target, Decrease Hitpoints by 40 ; Increase 30% based on STR, 5min"), r.fid, 20*time.Second)
	if len(r.friend.Effects) != 1 || r.friend.Effects[0].Proc != 58 || r.friend.Effects[0].ProcScale != StatNone {
		t.Fatalf("the caster's rune: %+v", r.friend.Effects)
	}
	if n := proc(100, 30*time.Second); n != 58 {
		t.Errorf("the caster's rune on STR 100: the proc took %d, want 58", n)
	}
}

// A pet bearing such a proc procs with its stand-in stat (3 a level), held
// to the cap all the same.
func TestAPetsBearersProc(t *testing.T) {
	effects := []*ActiveEffect{{Proc: 40, ProcMax: 40, ProcScale: StatStr, ProcScalePct: 30, ProcCap: 65, Until: time.Unix(2_000_000_000, 0)}}
	now := time.Unix(1_800_000_000, 0)
	if hits, _ := procRolls(effects, flatStat(50), 30, 30, 0, now, dice(0, 99)); len(hits) != 1 || hits[0] != 55 {
		t.Errorf("a bearer of 50: %v", hits)
	}
	if hits, _ := procRolls(effects, flatStat(300), 30, 30, 0, now, dice(0, 99)); len(hits) != 1 || hits[0] != 65 {
		t.Errorf("a bearer of 300: %v, want the cap", hits)
	}
	effects[0].ProcScale, effects[0].ProcCap = StatNone, 0
	if hits, _ := procRolls(effects, flatStat(300), 30, 30, 0, now, dice(0, 99)); len(hits) != 1 || hits[0] != 40 {
		t.Errorf("the control, no scale: %v", hits)
	}
}

// A counted power over time ticks exactly its count, the first at its
// landing: 3 of 13 + 24 (CHA 60) in a minute's duration; the control, no
// count, ticks every six seconds for the minute.
func TestACountedTickTicksItsCount(t *testing.T) {
	ticks := func(line string) (int, int) {
		c := newCaster(t)
		c.p.Power, c.p.MaxPower = 100, 9000
		spell := CastSpell{SpellID: 9202, DisplayIndex: 5, Level: 20, Scope: 1, Name: "Trickle", CastTime: 2 * time.Second,
			Recast: 3 * time.Second, Range: 30, Def: ParseSpellText(line)}
		c.land(t, spell, c.me, 0)
		n, last := 0, c.p.Power
		gained := c.p.Power - 100
		if gained > 0 {
			n = 1
		}
		for s := 4; s <= 90; s++ {
			c.tick(time.Duration(s) * time.Second)
			if c.p.Power != last {
				n++
				last = c.p.Power
			}
		}
		return n, c.p.Power - 100
	}
	if n, got := ticks("Target, Increase Power per tic by 13 ; Increase 40% based on CHA, 3 ticks, 1min"); n != 3 || got != 3*37 {
		t.Errorf("3 ticks: %d ticks, %d power; want 3 and %d", n, got, 3*37)
	}
	if n, _ := ticks("Target, Increase Power per tic by 13 ; Increase 40% based on CHA, 1min"); n < 9 {
		t.Errorf("the control: %d ticks; want one every six seconds for the minute", n)
	}
}

// A counted heal over time ticks its count, not its duration's; the
// control: four in its 24 seconds.
func TestACountedHealOverTime(t *testing.T) {
	heals := func(line string) int {
		c := newCaster(t)
		c.p.HP, c.p.MaxHP = 100, 9000
		spell := CastSpell{SpellID: 9203, DisplayIndex: 0, Level: 20, Scope: 1, Name: "Mend", CastTime: 2 * time.Second,
			Recast: time.Second, Range: 30, Def: ParseSpellText(line)}
		c.land(t, spell, c.me, 0)
		for s := 4; s <= 60; s++ {
			c.tick(time.Duration(s) * time.Second)
		}
		return (c.p.HP - 100) / 20
	}
	if n := heals("Target, Increase HP per tic by 20, 2 ticks, 24sec"); n != 2 {
		t.Errorf("2 ticks: %d", n)
	}
	if n := heals("Target, Increase HP per tic by 20, 24sec"); n != 4 {
		t.Errorf("the control: %d ticks, want 4", n)
	}
}

// A summoning spell's pet is named after it, less "Summon ", and a line's
// level is its pet's, at most its caster's: an enchanter's "Summon Stone
// Servant" of line level 21 is a Stone Servant of 21 that swings once; Grand
// Animation of line level 21 is of 21 and hits twice. The controls: a caster
// of 18 makes one of 18, and a pet spell of no level word is of the spell's.
func TestASummonedPetsNameAndLevel(t *testing.T) {
	for in, want := range map[string]string{"Summon Stone Servant": "Stone Servant", "summon djinn": "djinn",
		"Gymon's Animation": "Gymon's Animation", "Summon": "Summon", " Summon Mirror Wisp ": "Mirror Wisp"} {
		if got := petSpellName(in); got != want {
			t.Errorf("petSpellName(%q) = %q, want %q", in, got, want)
		}
	}
	summon := func(level int, name, line string) *NPC {
		r := newPetRig(t)
		r.p.Class, r.p.Level = classEnchanter, level
		pet, _ := r.summon(t, CastSpell{SpellID: 9204, DisplayIndex: 4, Level: 30, Scope: 0, Name: name,
			CastTime: 5 * time.Second, PowerCost: 100, Def: ParseSpellText(line)}, 0)
		return pet
	}
	if pet := summon(30, "Summon Stone Servant", "Summons a pet, level 21"); pet.Name != "Stone Servant" || pet.Level != 21 ||
		!pet.SwingFixed || pet.SwingKind != 0 || pet.ModelID != 0xDC3B344E {
		t.Errorf("the Stone Servant: %q level %d, swing %v/%d, body %#x", pet.Name, pet.Level, pet.SwingFixed, pet.SwingKind, pet.ModelID)
	}
	if pet := summon(30, "Grand Animation", "Summons a pet, level 21"); pet.Name != "Grand Animation" || pet.Level != 21 || pet.SwingKind != 4 {
		t.Errorf("Grand Animation: %q level %d, swing %d", pet.Name, pet.Level, pet.SwingKind)
	}
	if pet := summon(18, "Summon Stone Servant", "Summons a pet, level 21"); pet.Level != 18 {
		t.Errorf("a caster of 18 (control): level %d", pet.Level)
	}
	if pet := summon(30, "Mirror Animation", "Summons a pet"); pet.Name != "Mirror Animation" || pet.Level != 30 || pet.SwingKind != 0 {
		t.Errorf("Mirror Animation (control): %q level %d, swing %d", pet.Name, pet.Level, pet.SwingKind)
	}
}

// A bearer's proc and its cap come back after a logout as they went.
func TestABearersProcIsKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	c.p.Effects = []*ActiveEffect{{SpellID: 9201, Name: "Glow Rune", Until: now.Add(5 * time.Minute), Proc: 40, ProcMax: 40,
		ProcScale: StatStr, ProcScalePct: 30, ProcCap: 65}}
	rows := savedEffects(c.p, now)
	if len(rows) != 1 {
		t.Fatalf("saved %+v", rows)
	}
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, now); n != 1 || len(c.p.Effects) != 1 {
		t.Fatalf("restored %+v", c.p.Effects)
	}
	if ae := c.p.Effects[0]; ae.Proc != 40 || ae.ProcScale != StatStr || ae.ProcScalePct != 30 || ae.ProcCap != 65 {
		t.Errorf("restored %+v", ae)
	}
}
