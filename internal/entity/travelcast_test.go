package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// An item's cast that sends its caster to a place as it lands (CastSpell.Travel:
// the Rock of Freeport, handlers/rockfreeport.go), against the cast tests' druid
// (newCaster), the travel hook recording. The place, the name and the visual are
// made up.

func pebbleCast(to GateSpot) CastSpell {
	return CastSpell{SpellID: -1999, Name: "Pebble of Elsewhere", CastTime: 2 * time.Second, FX: 0x12345678, Travel: &to}
}

var pebbleSquare = GateSpot{World: 0, X: 500, Y: 20, Z: 600, Place: "a made-up square"}

// travels records what the travel hook is handed.
func (c *caster) travels() *[]GateTravel {
	var sent []GateTravel
	c.w.OnTravel(func(tr GateTravel) { sent = append(sent, tr) })
	return &sent
}

// The start is the 0x00B1 alone, the caster at itself for 2000 ms in the cast's
// visual: no 0x00CD, no lockout, no power. Nothing is handed over before the
// landing (the cast time and castLand, as every cast), then the caster alone to
// the place, by the cast's name, and the cast is over. The control: the same
// cast with no place is a spell's start, 0x00CD first and a lockout, and lands
// sending no one.
func TestATravelCastSendsItsCasterWhenItLands(t *testing.T) {
	c := newCaster(t)
	sent := c.travels()
	power := c.p.Power
	if !c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(0)) {
		t.Fatal("not begun")
	}
	msgs := c.sent()
	wantOps(t, msgs, protocol.OpcodeCastingCombat)
	if w := unhex(t, "b100d0a12414d0a1241400d007000078563412"); !bytes.Equal(msgs[0], w) {
		t.Errorf("0x00B1 % X, want % X (itself, 2000 ms, the visual)", msgs[0], w)
	}
	if len(c.p.Lockouts) != 0 || c.p.Power != power {
		t.Errorf("lockouts %+v, power %d (was %d): want none taken", c.p.Lockouts, c.p.Power, power)
	}
	c.tick(2*time.Second + castLand - time.Millisecond)
	c.w.runTravels()
	if len(*sent) != 0 || len(c.sent()) != 0 {
		t.Fatalf("before its landing: handed %+v", *sent)
	}
	c.tick(2*time.Second + castLand)
	c.w.runTravels()
	if len(*sent) != 1 {
		t.Fatalf("at its landing: handed %d travellers, want the caster", len(*sent))
	}
	tr := (*sent)[0]
	if tr.P != c.p || tr.Sess != c.sess || tr.To != pebbleSquare || tr.Ring != 0 || tr.Home != 0 || tr.Spell != "Pebble of Elsewhere" {
		t.Errorf("travel %+v, want the caster to the place by the cast's name", tr)
	}
	if c.p.Casting != nil || len(c.sent()) != 0 {
		t.Errorf("after its landing: casting %v", c.p.Casting)
	}

	// the control: no place, a spell's start, and a landing that sends no one
	c2 := newCaster(t)
	sent2 := c2.travels()
	plain := pebbleCast(pebbleSquare)
	plain.Travel = nil
	if !c2.w.StartCast(c2.sess, plain, 0xFFFFFFFF, c2.at(0)) {
		t.Fatal("the control: not begun")
	}
	wantOps(t, c2.sent(), protocol.OpcodeCooldownTimer, protocol.OpcodeCastingCombat)
	if len(c2.p.Lockouts) != 1 {
		t.Errorf("the control: lockouts %+v, want one", c2.p.Lockouts)
	}
	c2.tick(2*time.Second + castLand)
	c2.w.runTravels()
	if len(*sent2) != 0 {
		t.Errorf("the control: handed %+v", *sent2)
	}
}

// One at a time: a second use within the cast time is let go as any cast is,
// and so is one after the cast's busy time while the first is still on its way
// (a spell may begin then); one after the landing begins. The control: a spell
// begun while the travel is on its way does not keep it from landing.
func TestATravelCastIsOneAtATime(t *testing.T) {
	c := newCaster(t)
	sent := c.travels()
	if !c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(0)) {
		t.Fatal("not begun")
	}
	c.sent()
	if c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(time.Second)) {
		t.Error("a second begun within the cast time")
	}
	onItsWay := 2*time.Second + castBusy + 100*time.Millisecond
	if c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(onItsWay)) {
		t.Error("a second begun while the first is on its way")
	}
	if len(c.sent()) != 0 {
		t.Error("a use let go was answered")
	}
	// the control: a spell then begins, and the travel lands all the same
	heal := minorHeal()
	if !c.w.StartCast(c.sess, heal, c.me, c.at(onItsWay)) {
		t.Fatal("a spell after the travel's busy time: not begun")
	}
	c.tick(2*time.Second + castLand)
	c.w.runTravels()
	if len(*sent) != 1 {
		t.Fatalf("with a spell begun on its way: handed %d, want the caster", len(*sent))
	}
	c.sent()
	c.tick(onItsWay + 2*time.Second + castBusy)
	if !c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(onItsWay+2*time.Second+castBusy)) {
		t.Error("after the landing and the spell: not begun")
	}
}

// The dead are sent nowhere: a caster dead at the landing is handed to no one.
// The control: alive, the same cast lands.
func TestATravelCastOfTheDeadLandsNowhere(t *testing.T) {
	for _, dead := range []bool{true, false} {
		c := newCaster(t)
		sent := c.travels()
		if !c.w.StartCast(c.sess, pebbleCast(pebbleSquare), 0xFFFFFFFF, c.at(0)) {
			t.Fatal("not begun")
		}
		if dead {
			c.p.HP = 0
		}
		c.tick(2*time.Second + castLand)
		c.w.runTravels()
		if want := map[bool]int{true: 0, false: 1}[dead]; len(*sent) != want {
			t.Errorf("dead %v: handed %d, want %d", dead, len(*sent), want)
		}
	}
}
