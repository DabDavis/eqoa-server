package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A buff and a heal over time kept over a logout: what was left of each,
// and of the heal's next tick, comes back at the next login however long
// after, its time running again from there (Sandstorm's druid kept 30-minute
// buffs over 50 minutes offline), with the buff's stats and its place on the
// effects channel.
func TestEffectsKeptOverALogout(t *testing.T) {
	c := newCaster(t)
	stone := CastSpell{SpellID: 900, DisplayIndex: 10, Scope: 1, Name: "Stoneskin", Icon: 0x727C44F8,
		CastTime: 4 * time.Second, Recast: 3 * time.Second, PowerCost: 120, Range: 30,
		Def: ParseSpellText("Target, Increase AC by 150, Max Power by 60, MaxHP by 30, HP by 30, 30min")}
	c.w.StartCast(c.sess, stone, c.me, c.at(0))
	c.tick(5400 * time.Millisecond) // landed at 5.4 s
	c.w.StartCast(c.sess, minorHeal(), c.me, c.at(8*time.Second))
	c.tick(11400 * time.Millisecond) // landed at 11.4 s: a tick now, the next at 17.4
	c.sent()

	// the logout, 13 s in
	rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(13*time.Second))
	if !ok || len(rows) != 2 {
		t.Fatalf("kept %d (%v)", len(rows), ok)
	}
	if r := rows[0]; r.SpellID != 900 || r.LeftMS != 30*60*1000-7600 || r.TickMS != -1 || r.Flags != 0 {
		t.Errorf("the buff kept as %+v", r)
	}
	if r := rows[1]; r.SpellID != 457 || r.LeftMS != 22400 || r.TickMS != 4400 || r.HPTick != 35 {
		t.Errorf("the heal kept as %+v", r)
	}

	// the next login, three hours on, another session, a fresh Player
	d := newCaster(t)
	d.p.HP = 400
	t0 := d.at(3 * time.Hour)
	if n := d.w.RestoreEffects(d.sess, rows, t0); n != 2 {
		t.Fatalf("restored %d", n)
	}
	if d.p.StatNow(StatAC) != 150 || d.p.MaxHPNow() != 566 || d.p.MaxPowerNow() != 1085 || !d.p.StatsDirty {
		t.Errorf("AC %d, max HP %d, max power %d", d.p.StatNow(StatAC), d.p.MaxHPNow(), d.p.MaxPowerNow())
	}
	want := []BuffSlot{{Icon: 0x727C44F8, Name: "Stoneskin"}, {Icon: 0x828563AF, Name: "Minor Mending"}}
	if len(d.p.Buffs) != 2 || d.p.Buffs[0] != want[0] || d.p.Buffs[1] != want[1] {
		t.Errorf("effects channel %+v", d.p.Buffs)
	}
	d.w.mu.Lock()
	d.w.effectSystem(t0.Add(4399 * time.Millisecond))
	d.w.mu.Unlock()
	if d.p.HP != 400 {
		t.Error("the heal ticked before what was left of its tick")
	}
	d.w.mu.Lock()
	d.w.effectSystem(t0.Add(4400 * time.Millisecond))
	d.w.mu.Unlock()
	msgs := d.sent()
	if d.p.HP != 435 || len(msgs) != 1 || !bytes.Equal(msgs[0], unhex(t, "db00d0a1241446d0a12414")) {
		t.Errorf("the tick after the login: HP %d, sent %x", d.p.HP, msgs)
	}
	// the heal wears off 22.4 s after the login, the buff 30 min less 7.6 s
	d.w.mu.Lock()
	d.w.effectSystem(t0.Add(22400 * time.Millisecond))
	d.w.mu.Unlock()
	if len(d.p.Effects) != 1 || d.p.Effects[0].SpellID != 900 {
		t.Errorf("after 22.4 s: %+v", d.p.Effects)
	}
	d.w.mu.Lock()
	d.w.effectSystem(t0.Add(30*time.Minute - 7600*time.Millisecond))
	d.w.mu.Unlock()
	if len(d.p.Effects) != 0 || d.p.StatNow(StatAC) != 0 {
		t.Errorf("the buff outlived its time: %+v", d.p.Effects)
	}
}

// The control: nothing comes back that was not kept, and what is spent is
// not kept or restored.
func TestEffectsKeptNothingSpent(t *testing.T) {
	c := newCaster(t)
	if rows, ok := c.w.SavedEffects(c.p.ServerID, c.at(0)); !ok || len(rows) != 0 {
		t.Errorf("a player with no effects kept %v", rows)
	}
	if n := c.w.RestoreEffects(c.sess, []database.BuffRow{{SpellID: 1, Name: "Gone", LeftMS: 0, TickMS: -1}}, c.at(0)); n != 0 || len(c.p.Buffs) != 0 {
		t.Errorf("a spent effect came back (%d)", n)
	}
	if _, ok := c.w.SavedEffects(9999, c.at(0)); ok {
		t.Error("an absent player's effects")
	}
}

// A heal over time whose caster is gone still ticks, the figure the
// target's own.
func TestHealTickWithoutItsCaster(t *testing.T) {
	c := newCaster(t)
	c.p.HP = 400
	c.p.Effects = []*ActiveEffect{{SpellID: 7, Name: "Mend", Caster: 4242, Until: c.at(time.Minute), NextTick: c.at(time.Second), HPTick: 20}}
	c.w.mu.Lock()
	c.w.effectSystem(c.at(time.Second))
	c.w.mu.Unlock()
	msgs := c.sent()
	if c.p.HP != 420 || len(msgs) != 1 || opOfMsg(msgs[0]) != protocol.OpcodeDamageIndication || !bytes.Equal(msgs[0], unhex(t, "db00d0a1241428d0a12414")) {
		t.Errorf("HP %d, sent %x", c.p.HP, msgs)
	}
}
