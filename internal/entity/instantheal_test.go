package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The GM's petition (2026-11-20): "Lay hands should fire off its heal instantly
// when used". A heal of no cast time lands at its press, not castLand on; the
// control, the same heal with a 2 s cast, lands castLand after its cast time,
// and an instant hit (a taunt aside) keeps castLand.
func TestAHealOfNoCastTimeLandsAtOnce(t *testing.T) {
	layHands := func(cast time.Duration) CastSpell {
		return CastSpell{SpellID: 700046, DisplayIndex: 3, Scope: 1, Name: "Lay Hands", CastTime: cast,
			Recast: 900 * time.Second, Range: 30, Def: ParseSpellText("Target, Increase Hitpoints by 1 ; Increase 50% based on CHA")}
	}
	c := newCaster(t)
	c.p.HP = 100
	if !c.w.StartCast(c.sess, layHands(0), c.me, c.at(0)) {
		t.Fatal("not begun")
	}
	c.sent()
	c.tick(time.Millisecond)
	if msgs := c.sent(); c.p.HP <= 100 || len(msgs) == 0 || opOfMsg(msgs[0]) != protocol.OpcodeDamageIndication {
		t.Fatalf("1 ms after the press: HP %d, %d messages; want it healed and told", c.p.HP, len(msgs))
	}
	if !instantHeal(&CastSpell{Def: ParseSpellText("Self, Increase Hitpoints by 45 ; Increase 10% based on WIS")}) {
		t.Error("a monk's Mend: not an instant heal")
	}
	if instantHeal(&CastSpell{Def: ParseSpellText("Target, Decrease Hitpoints by 26")}) {
		t.Error("an instant hit read as an instant heal")
	}

	c = newCaster(t)
	c.p.HP = 100
	c.w.StartCast(c.sess, layHands(2*time.Second), c.me, c.at(0))
	c.sent()
	c.tick(2*time.Second + castLand - time.Millisecond)
	if c.p.HP != 100 {
		t.Fatalf("the control, a 2 s cast: healed before %v", 2*time.Second+castLand)
	}
	c.tick(2*time.Second + castLand)
	if c.p.HP <= 100 {
		t.Errorf("the control: not healed at %v", 2*time.Second+castLand)
	}
}
