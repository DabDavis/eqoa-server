package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// cdLockout reads a 0x00CD's lockout (its zigzag ms after the index and stamp).
func cdLockout(t *testing.T, m []byte) time.Duration {
	t.Helper()
	if opOfMsg(m) != protocol.OpcodeCooldownTimer || len(m) < 8 {
		t.Fatalf("not a 0x00CD: % X", m)
	}
	var v uint64
	for i, b := range m[7:] {
		v |= uint64(b&0x7F) << (7 * i)
		if b < 0x80 {
			break
		}
	}
	return time.Duration(int64(v>>1)^-int64(v&1)) * time.Millisecond
}

// Petition 2197 (Hoofbite: "Agony rogue spell no longer darkens spell icon to
// indicate it being cast. should darken"): the rogue's Agony has no cast time
// and no recast, so from petition 2184's castLockoutExtra of 0 its 0x00CD told
// a lockout of 0 and CLIENT greyed nothing. A cast's lockout is now at least
// castBusy (400 ms), an art's at least the arts' cooldown; a strike's is its
// cast and recast as before.
func TestZeroRecastArtLockoutDarkens(t *testing.T) {
	agony := CastSpell{SpellID: 901288, DisplayIndex: 7, Level: 51, Scope: 1, Name: "Agony", PowerCost: 10, Range: 10,
		Def: ParseSpellText("Target, Decrease Hitpoints by 1 ; Increase 100% based on DEX")}
	for _, tc := range []struct {
		name    string
		balance bool
		want    time.Duration
	}{{"no balance", false, castBusy}, {"the seed balance", true, 1500 * time.Millisecond}} {
		c := newCaster(t)
		c.npc.HP, c.npc.MaxHP = 100000, 100000
		if tc.balance {
			c.w.balance = SeedBalance()
		}
		if !c.w.StartCast(c.sess, agony, c.npcO, c.at(0)) {
			t.Fatalf("%s: Agony not begun", tc.name)
		}
		msgs := c.sent()
		if len(msgs) == 0 {
			t.Fatalf("%s: nothing sent", tc.name)
		}
		if got := cdLockout(t, msgs[0]); got != tc.want {
			t.Errorf("%s: Agony's lockout %v, want %v", tc.name, got, tc.want)
		}
		c.tick(tc.want - time.Millisecond)
		for _, m := range c.sent() {
			if opOfMsg(m) == protocol.OpcodeCooldownTimer {
				t.Errorf("%s: its lockout ended early", tc.name)
			}
		}
		c.tick(tc.want)
		ended := false
		for _, m := range c.sent() {
			ended = ended || opOfMsg(m) == protocol.OpcodeCooldownTimer && cdLockout(t, m) == 0
		}
		if !ended {
			t.Errorf("%s: no lockout's end at %v", tc.name, tc.want)
		}
	}
	// a strike's lockout: its cast and recast, as before
	c := newCaster(t)
	c.w.balance = SeedBalance()
	c.w.StartCast(c.sess, strike(), c.npcO, c.at(0))
	if got := cdLockout(t, c.sent()[0]); got != 5*time.Second {
		t.Errorf("a strike's lockout %v, want 5 s", got)
	}
}
