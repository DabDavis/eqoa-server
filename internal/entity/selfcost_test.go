package entity

import (
	"testing"
	"time"
)

// A self spell's own price (lifetap.go selfCostLocked; jib's PR #1, the GM
// Jibrish's ruling of 2026-10-29: "You could definitely kill yourself with lich
// form, pacts and so on."). The lines, the workbook objects and their figures
// are made up; only their shapes are the live data's (EQOA-Data's line, the
// import's workbook object).

// selfSpell is a free self line cast in 2 s.
func selfSpell(id int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Scope: 0, Name: name, Icon: 0x12345678,
		CastTime: 2 * time.Second, Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// A self line's lost hit points are its caster's price, in full: a
// cannibalize trades them for power; a price the caster cannot pay kills it,
// and nothing more of the spell lands.
func TestSelfLinePaysItsPrice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Power = 500, 100
	c.land(t, selfSpell(9401, "Little Cannibalize", "Self, Decrease Hitpoints by 30, Increase Power by 8"), c.me, 0)
	if c.p.HP != 470 || c.p.Power != 108 {
		t.Fatalf("HP %d power %d, want 470 and 108", c.p.HP, c.p.Power)
	}
	c.p.HP = 20
	c.land(t, selfSpell(9402, "Big Cannibalize", "Self, Decrease Hitpoints by 30, Increase Power by 8"), c.me, 10*time.Second)
	if !c.p.IsDead() || c.p.Power != 108 {
		t.Fatalf("dead %v, power %d: the price that kills gives nothing", c.p.IsDead(), c.p.Power)
	}
}

// The live Cannibalize ranks are read from the workbook (hp -N and power +M on
// a self spell, no area): the same price. The controls: a self spell with the
// workbook's player-based area (the custom races' Giant's Stomp, Frost Breath)
// costs its caster nothing, its figure being the area's hit; and a pact's
// price ("... to Self") is paid once, not twice.
func TestWorkbookSelfSpellPaysItsPrice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Power = 500, 100
	canni := wbSpell(9411, 20, 0, "Test Cannibalize", `{"v":1,"scope":"self","target_type":"Self","harmful":false,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-200},{"id":"0001","type":"Dmg/Heal","kind":"power","amount":150}]}`)
	c.land(t, canni, c.me, 0)
	if c.p.HP != 300 || c.p.Power != 240 {
		t.Fatalf("a workbook cannibalize: HP %d power %d, want 300 and 240 (its cast's 10, then its 150)", c.p.HP, c.p.Power)
	}

	a := newCaster(t)
	a.p.HP = 500
	stomp := wbSpell(9412, 60, 0, "Test Stomp", `{"v":1,"scope":"player_aoe","target_type":"Enemy","harmful":true,"radius":20,
		"effects":[{"id":"0002","type":"Dmg/Heal","kind":"hp","amount":-60}]}`)
	if stomp.Area.Centre == AreaNone {
		t.Fatalf("the control's area spell read with no area: %+v", stomp.Area)
	}
	a.land(t, stomp, a.me, 0)
	if a.p.HP != 500 || a.p.IsDead() {
		t.Errorf("an area self spell cost its caster: HP %d, want 500", a.p.HP)
	}

	p := newCaster(t)
	p.p.HP, p.p.Power = 500, 100
	p.land(t, selfSpell(9413, "Test Pact", "Self, Decrease Hitpoints by 30 to Self, Increase Power by 8"), p.me, 0)
	if p.p.HP < 470 || p.p.Power != 108 {
		t.Errorf("a pact: HP %d (want 470 or more: its resisted price once) and power %d (want 108)", p.p.HP, p.p.Power)
	}
}

// A self spell's loss each tick (Cursed Flesh's) is paid at the landing and
// every tick while it lasts, never below 1 hit point, as the Lich form's
// price over time is (regen.go). The control: the same line's heal over time
// heals.
func TestSelfLineTickIsItsPrice(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 500, 536
	c.land(t, selfSpell(9421, "Test Flesh", "Self, Gives Form, Decrease HP per tic by 7, Increase STR by 35, 2min"), c.me, 0)
	land := 2*time.Second + castLand
	if c.p.HP != 493 {
		t.Fatalf("at its landing: HP %d, want 493", c.p.HP)
	}
	c.tick(land + 6*time.Second)
	c.tick(land + 12*time.Second)
	if c.p.HP != 479 {
		t.Fatalf("after two more ticks: HP %d, want 479", c.p.HP)
	}
	c.p.HP = 5
	c.tick(land + 18*time.Second)
	c.tick(land + 24*time.Second)
	if c.p.HP != 1 || c.p.IsDead() {
		t.Errorf("a tick's price with 5 left: HP %d dead %v, want 1 and alive", c.p.HP, c.p.IsDead())
	}

	h := newCaster(t)
	h.p.HP, h.p.MaxHP = 400, 536
	h.land(t, selfSpell(9422, "Test Mend", "Self, Increase HP per tic by 7, 2min"), h.me, 0)
	if h.p.HP <= 400 {
		t.Errorf("the control: a self heal over time left HP %d, want more than 400", h.p.HP)
	}
}

// A drunk potion pays its health once, as it is drunk: the landing that
// follows takes no second price.
func TestDrunkPotionPaysOnce(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Power = 500, 100
	pe := NewPotionEffect(59101, "Test Draught", "Self, Decrease Hitpoints by 60, Increase Power by 40", "", 0, "", 0, 0)
	if r := c.use(t, pe, 0, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.p.HP != 440 || c.p.Power != 140 {
		t.Errorf("HP %d power %d, want 440 and 140", c.p.HP, c.p.Power)
	}
}
