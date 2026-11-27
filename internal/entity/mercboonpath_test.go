package entity

import (
	"testing"
	"time"
)

// powerBoon is the enchanter's Power Boon as the workbook holds its Boon
// path's line: power now on a friendly target.
func powerBoon() CastSpell {
	return wbSpell(700471, 30, 1, "Power Boon", `{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"power","amount":750,"scale":"cha","scale_pct":20}]}`)
}

// Heirloom petition 2551 (Kang: "boon path Enchanter Merc does not have any
// of the boon spells. Probly why it sucks at giving power"): the Boon path's
// power now on a friend is a merc's power gift, worth its power; the Mind
// path's self power is what it was: an instant gain on itself none, its
// lasting most power a buff.
func TestMercReadsTheBoonPath(t *testing.T) {
	boon := powerBoon()
	if u := mercUseOf(&boon); u != usePower {
		t.Errorf("Power Boon reads as use %v, want a power gift", u)
	}
	if n := MercUseName(&boon); n != "give power" {
		t.Errorf("Power Boon's use in words %q", n)
	}
	if v := mercWorth(&boon, usePower, &Player{Level: 60}); v < 750 {
		t.Errorf("Power Boon worth %v, want its 750 and more", v)
	}
	selfGain := wbSpell(9702, 30, 0, "Test Self Power", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"power","amount":500}]}`)
	if u := mercUseOf(&selfGain); u != useSelfPower { // petition 3431: its own power, under mercSelfPowerBelowPct
		t.Errorf("a self power gain reads as use %v, want its own power", u)
	}
	mind := wbSpell(9703, 30, 0, "Test Expansive Mind", `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":1200,"stack":2,"level":30,
		"min_level":1,"req_equipment":"Any","sheet_type":"Statistic Change","effects":[{"type":"Statistic Change","kind":"stat","stat":"maxpower","amount":260}]}`)
	if u := mercUseOf(&mind); u != useBuff {
		t.Errorf("the Mind path's most power reads as use %v, want a buff", u)
	}
}

// An enchanter merc of the Boon path (petition 2551) casts Power Boon at the
// one of its team lowest in its share of power under mercPowerGiftPct: its
// owner, then a mate merc lower still; none when all are above it (the
// control), nor at a body with no power, nor at itself.
func TestMercGivesPowerToTheLowest(t *testing.T) {
	r := newMercRig(t)
	boon := powerBoon()
	k := r.kit(classEnchanter, 0, 0, boon)
	k2 := r.kit(classEnchanter, 0, 0)
	k2.Name = "Ormon"
	a := r.hireSlot(1, k)
	b := r.hireSlot(2, k2)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	o := r.p
	setPower := func(pctOwner, pctB, pctA int) {
		o.Power = o.MaxPowerNow() * pctOwner / 100
		b.npc.Power = b.npc.MaxPower * pctB / 100
		a.npc.Power = a.npc.MaxPower * pctA / 100
		clear(a.ready)
		r.w.mercCasts = nil
	}
	if o.MaxPowerNow() <= 0 || b.npc.MaxPower <= 0 || a.npc.MaxPower <= boon.PowerCost {
		t.Fatalf("the rig's power: owner %d, mate %d, caster %d", o.MaxPowerNow(), b.npc.MaxPower, a.npc.MaxPower)
	}
	castAt := func(what string) EntityID {
		t.Helper()
		if !r.w.mercPowerTurnLocked(a, r.at(0)) {
			return 0
		}
		c := r.w.mercCasts[len(r.w.mercCasts)-1]
		if c.spell.SpellID != boon.SpellID || c.group {
			t.Fatalf("%s: cast %q (group %v)", what, c.spell.Name, c.group)
		}
		return c.target
	}
	// the control: everyone above the threshold
	setPower(80, 90, 100)
	if id := castAt("all above"); id != 0 {
		t.Errorf("all above %d%%: a gift at %d", mercPowerGiftPct, id)
	}
	// the owner low: the gift at it
	setPower(20, 90, 100)
	if id := castAt("owner low"); id != o.ServerID {
		t.Errorf("the owner at 20%%: the gift at %d, want the owner %d", id, o.ServerID)
	}
	// both low, the mate lower: at the mate
	setPower(40, 10, 100)
	if id := castAt("mate lower"); id != b.npc.ServerID {
		t.Errorf("owner 40%%, mate 10%%: the gift at %d, want the mate %d", id, b.npc.ServerID)
	}
	// itself low alone: never itself
	setPower(90, 90, 30)
	if id := castAt("itself low"); id != 0 {
		t.Errorf("itself low alone: a gift at %d", id)
	}
	// a body with no power is never given any
	setPower(90, 0, 100)
	top := b.npc.MaxPower
	b.npc.MaxPower = 0
	if id := castAt("no power"); id != 0 {
		t.Errorf("a mate with no power: a gift at %d", id)
	}
	b.npc.MaxPower = top
	// its gift on its way to the owner: not a second
	setPower(20, 90, 100)
	if id := castAt("first"); id != o.ServerID {
		t.Fatalf("the first gift at %d", id)
	}
	clear(a.ready)
	if id := castAt("second"); id != 0 {
		t.Errorf("a second gift at %d while the first is on its way", id)
	}
}

// The gift lands (petition 2551): an enchanter merc's Power Boon cast on its
// own turn raises its owner's power by its figure.
func TestMercPowerBoonLands(t *testing.T) {
	r := newMercRig(t)
	boon := powerBoon()
	r.hire(r.kit(classEnchanter, 0, 0, boon), MercAssist)
	r.run(0, time.Second)
	r.w.mu.Lock()
	r.p.Power = r.p.MaxPowerNow() / 10
	before := r.p.Power
	r.w.mu.Unlock()
	r.run(1100*time.Millisecond, 6*time.Second)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if got := r.p.Power - before; got < min(750, r.p.MaxPowerNow()-before) {
		t.Errorf("the owner's power rose %d (%d of %d), want Power Boon's 750", got, r.p.Power, r.p.MaxPowerNow())
	}
}
