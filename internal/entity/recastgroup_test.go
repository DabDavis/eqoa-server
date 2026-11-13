package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The ranger's two instincts as the live database has them (spellpattern and
// spelleffect.effects, 2026-11-13): Wolf's Instinct, the wolf form, and Hunter's
// Instinct, the same stats with no form (petition 1183), each a 3 s cast and a
// 1800 s recast, 30 minutes.
const (
	wolfInstinctJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":2,"level":51,` +
		`"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","effects":[{"id":"05A7","type":"Alter Appearance",` +
		`"kind":"illusion","sprite":"0x7E513FF2"},{"id":"05A8","type":"Dmg/Heal","kind":"hp","amount":1500},{"id":"05A9",` +
		`"type":"Statistic Change","kind":"stat","stat":"str","amount":175},{"id":"05A9","type":"Statistic Change","kind":"stat",` +
		`"stat":"agi","amount":175},{"id":"05A9","type":"Statistic Change","kind":"stat","stat":"dex","amount":175},{"id":"05A9",` +
		`"type":"Statistic Change","kind":"stat","stat":"maxhp","amount":1500}]}`
	hunterInstinctJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1800,"stack":9,"level":51,` +
		`"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"id":"05A8","type":"Dmg/Heal","kind":"hp",` +
		`"amount":1500},{"id":"05A9","type":"Statistic Change","kind":"stat","stat":"str","amount":175},{"id":"05A9",` +
		`"type":"Statistic Change","kind":"stat","stat":"agi","amount":175},{"id":"05A9","type":"Statistic Change","kind":"stat",` +
		`"stat":"dex","amount":175},{"id":"05A9","type":"Statistic Change","kind":"stat","stat":"maxhp","amount":1500}]}`
)

// instincts are the two as the handler hands them to StartCast: the wolf at
// book place 7, the hunter at 9; grouped is whether table spellrecastgroup puts
// them in one group (handlers' recastGroupOf).
func instincts(grouped bool) (wolf, hunter CastSpell) {
	wolf = SpellOfRow(&database.SpellPatternRecord{SpellID: 701068, SpellName: "Wolf's Instinct", AbilityLvl: 51, CastTime: 3,
		Recast: 1800, Scope: 0, SpellDesc: "Self, Gives Form, Increase AGI/STR/DEX by 175, Increase MaxHP by 1500, Increase HP by 1500, 30min"},
		7, "", wolfInstinctJSON, 0)
	hunter = SpellOfRow(&database.SpellPatternRecord{SpellID: 760110, SpellName: "Hunter's Instinct", AbilityLvl: 51, CastTime: 3,
		Recast: 1800, Scope: 0, SpellDesc: "Self, Increase AGI/STR/DEX by 175, Increase MaxHP by 1500, Increase HP by 1500, 30min"},
		9, "", hunterInstinctJSON, 0)
	if grouped {
		mates := []GroupMate{{SpellID: 701068, DisplayIndex: 7}, {SpellID: 760110, DisplayIndex: 9}}
		wolf.RecastGroup, wolf.GroupMates = "ranger instinct", mates
		hunter.RecastGroup, hunter.GroupMates = "ranger instinct", mates
	}
	return wolf, hunter
}

// cooldownsOf is the 0x00CD of msgs, ability place -> milliseconds.
func cooldownsOf(msgs [][]byte) map[byte]int64 {
	out := map[byte]int64{}
	for _, m := range msgs {
		if opOfMsg(m) == protocol.OpcodeCooldownTimer {
			out[m[2]] = int64((&vims{b: m[7:]}).signed())
		}
	}
	return out
}

// Ticket 2011: casting Wolf's Instinct locks Hunter's Instinct out too, for the
// wolf's own lockout (3 s cast + 1800 s recast + 300 ms), each told by its own
// 0x00CD so both buttons grey out; Hunter's Instinct cast inside it is refused
// as a recast is (its line, nothing taken, no effect); at the lockout's end both
// are told free (0x00CD 0 on 7 and 9) and the hunter is cast. Its landing takes
// the wolf's effect (and its form) off: one effect on, the stats once. The
// control: ungrouped (before), the hunter is cast at once and both stack.
func TestRecastGroupSharesOneLockout(t *testing.T) {
	c := newCaster(t)
	c.p.Level, c.p.Class = 60, 1
	wolf, hunter := instincts(true)
	lock := wolf.CastTime + wolf.Recast + castLockoutExtra
	if lock != 1803300*time.Millisecond {
		t.Fatalf("the wolf's lockout %v, want 1803.3 s", lock)
	}

	c.p.Power = c.p.MaxPower
	if !c.w.StartCast(c.sess, wolf, c.me, c.at(0)) {
		t.Fatal("Wolf's Instinct not begun")
	}
	if got := cooldownsOf(c.sent()); len(got) != 2 || got[7] != 1803300 || got[9] != 1803300 {
		t.Fatalf("0x00CD sent %v, want abilities 7 and 9 at 1803300 ms", got)
	}
	c.tick(wolf.CastTime + castLand)
	c.sent()
	if len(c.p.Effects) != 1 || c.p.Effects[0].SpellID != wolf.SpellID {
		t.Fatalf("effects %v, want Wolf's Instinct", c.effectNames())
	}
	str := c.p.StatNow(StatStr)

	// the hunter inside the shared lockout: refused, nothing taken
	power := c.p.Power
	if c.w.StartCast(c.sess, hunter, c.me, c.at(10*time.Minute)) {
		t.Fatal("Hunter's Instinct was cast inside Wolf's Instinct's lockout")
	}
	wantOps(t, c.sent(), protocol.OpcodeColoredChat, protocol.OpcodeCastingCombat)
	if c.p.Power != power || len(c.p.Effects) != 1 {
		t.Fatalf("a refused cast took power %d -> %d or landed: %v", power, c.p.Power, c.effectNames())
	}
	// the wolf itself again: refused too
	if c.w.StartCast(c.sess, wolf, c.me, c.at(20*time.Minute)) {
		t.Fatal("Wolf's Instinct was cast inside its own lockout")
	}
	c.sent()

	// the lockout over: both said free, on 7 and 9
	c.tick(lock + time.Second)
	ends := cooldownsOf(c.sent())
	if len(ends) != 2 || ends[7] != 0 || ends[9] != 0 || len(c.p.Lockouts) != 0 {
		t.Fatalf("ends said %v, %d lockouts left; want 0 on 7 and 9", ends, len(c.p.Lockouts))
	}

	// the wolf's effect is put back on (as if it were still on: a Moogie reset,
	// a GM's) to see the hunter take its place: one line
	c.p.Effects = nil
	c.p.Lockouts = nil
	own, _ := c.p.Body()
	c.castOn(t, wolf, 31*time.Minute)
	if b, _ := c.p.Body(); b == own {
		t.Fatalf("Wolf's Instinct gave no form (body %#x)", b)
	}
	c.p.Lockouts = nil // freed, as a Moogie's reset frees them
	c.castOn(t, hunter, 32*time.Minute)
	if got := c.effectNames(); len(got) != 1 || got[0] != "Hunter's Instinct" {
		t.Fatalf("effects %v, want Hunter's Instinct alone (one line)", got)
	}
	if b, _ := c.p.Body(); b != own {
		t.Fatalf("the wolf form (%#x) stayed on with Hunter's Instinct", b)
	}
	if got := c.p.StatNow(StatStr); got != str {
		t.Fatalf("STR %d under Hunter's Instinct, want %d (the 175 once)", got, str)
	}
	// and the hunter locks the wolf out
	if c.w.StartCast(c.sess, wolf, c.me, c.at(33*time.Minute)) {
		t.Fatal("Wolf's Instinct was cast inside Hunter's Instinct's lockout")
	}
}

// The control: the same two spells in no group are what they were, two
// lockouts and two lines: the hunter is cast right after the wolf, its 0x00CD
// on 9 alone, and both stay on, the 175 STR twice.
func TestUngroupedInstinctsStack(t *testing.T) {
	c := newCaster(t)
	c.p.Level, c.p.Class = 60, 1
	wolf, hunter := instincts(false)
	base := c.p.StatNow(StatStr)
	c.castOn(t, wolf, 0)
	c.sent()
	c.p.Power = c.p.MaxPower
	if !c.w.StartCast(c.sess, hunter, c.me, c.at(10*time.Second)) {
		t.Fatal("ungrouped, Hunter's Instinct was refused")
	}
	if got := cooldownsOf(c.sent()); len(got) != 1 || got[9] != 1803300 {
		t.Fatalf("0x00CD %v, want ability 9 alone", got)
	}
	c.tick(10*time.Second + hunter.CastTime + castLand)
	if len(c.p.Effects) != 2 || c.p.StatNow(StatStr) <= base+175 {
		t.Fatalf("ungrouped effects %v, STR %d from %d: want both on, stacking", c.effectNames(), c.p.StatNow(StatStr), base)
	}
}

// A spell of the group the book lacks is locked too, shown on no ability; a
// mate locked out longer keeps its end.
func TestRecastGroupLocksAMateNotInTheBook(t *testing.T) {
	c := newCaster(t)
	c.p.Level, c.p.Class = 60, 1
	wolf, _ := instincts(true)
	wolf.GroupMates = []GroupMate{{SpellID: 701068, DisplayIndex: 7}, {SpellID: 760110, DisplayIndex: -1}, {SpellID: 5555, DisplayIndex: 4}}
	far := c.at(5 * time.Hour)
	c.p.Lockouts = []castLockout{{spellID: 5555, displayIndex: 4, until: far}}
	c.p.Power = c.p.MaxPower
	if !c.w.StartCast(c.sess, wolf, c.me, c.at(0)) {
		t.Fatal("Wolf's Instinct not begun")
	}
	got := cooldownsOf(c.sent())
	if len(got) != 2 || got[7] != 1803300 || got[4] != int64(5*time.Hour/time.Millisecond) {
		t.Fatalf("0x00CD %v, want 7 at 1803300 and 4 kept at 5 h", got)
	}
	ends := map[int]time.Time{}
	for _, l := range c.p.Lockouts {
		ends[l.spellID] = l.until
	}
	if len(ends) != 3 || !ends[760110].Equal(c.at(1803300*time.Millisecond)) || !ends[5555].Equal(far) {
		t.Fatalf("lockouts %+v", c.p.Lockouts)
	}
}
