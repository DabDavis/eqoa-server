package entity

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The training dummies held to what Sandstorm's did (2026-10-21): the
// round's swing and hit times, the hit with its heal, a hit taken off
// nothing, one player at a time, and the fight over when the player stops.
// What a hit is, is defence.go's (defence_test.go).

// newDummy is the caster's fixture with its creature made a dummy of the
// level and kind given, the player engaged with it.
func newDummy(t *testing.T, name string, level int) *caster {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.Dummy = name, level, DummyKindOf(name)
	c.npc.HP, c.npc.MaxHP = 4000, 4000
	c.p.AttackTarget, c.p.AttackObjectID = c.npc.ServerID, c.npcO
	return c
}

func (c *caster) dummyTick(d time.Duration) [][]byte {
	c.w.mu.Lock()
	c.w.aiSystem(c.at(d))
	c.w.mu.Unlock()
	return c.sent()
}

// 0x00DB as the client reads it: attacker, zigzag amount, defender.
func damageMsg(t *testing.T, from uint32, amount int, to uint32) []byte {
	t.Helper()
	b := []byte{0xDB, 0x00, byte(from), byte(from >> 8), byte(from >> 16), byte(from >> 24)}
	z := uint64(amount) << 1
	if amount < 0 {
		z = uint64(-amount)<<1 - 1
	}
	for z >= 0x80 {
		b = append(b, byte(z)|0x80)
		z >>= 7
	}
	b = append(b, byte(z), byte(to), byte(to>>8), byte(to>>16), byte(to>>24))
	return b
}

func TestTrainingDummyFightsInRounds(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv35", 35)
	c.p.AC = 600 // enough armour for the Lv35's floor, 14, every hit
	c.w.mu.Lock()
	killed := c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	hit := c.sent()
	if killed || c.npc.Dead || c.npc.HP != 4000 || c.p.TotalXP != 0 {
		t.Fatalf("the hit killed %v, dead %v, HP %d, XP %d: a dummy takes nothing", killed, c.npc.Dead, c.npc.HP, c.p.TotalXP)
	}
	if len(hit) != 1 || opOfMsg(hit[0]) != protocol.OpcodeDamageIndication {
		t.Fatalf("the hit sent %x, want its 0x00DB", hit)
	}
	if c.npc.DummyTarget != c.p.ServerID || c.npc.Threats != nil || c.npc.CurrentEnemy != 0 {
		t.Fatalf("target %d, threats %v, enemy %d: the dummy fights the hitter and holds no aggro", c.npc.DummyTarget, c.npc.Threats, c.npc.CurrentEnemy)
	}

	// The round: the swing at once, Attack4 over its 6 s
	start := c.dummyTick(0)
	want := unhex(t, "b100982e3101d0a12414007017000014ea9bda") // 0x00B1 dummy -> player, 0 more, 6000 ms, Attack4
	if len(start) != 1 || !bytes.Equal(start[0], want) {
		t.Fatalf("round start % X, want % X", start, want)
	}
	if got := c.dummyTick(499 * time.Millisecond); len(got) != 0 {
		t.Fatalf("a hit before 0.5 s: %x", got)
	}
	// each hit 14 off and a 300 heal after it, within the most
	first := c.dummyTick(500 * time.Millisecond)
	if len(first) != 2 || !bytes.Equal(first[0], damageMsg(t, c.npcO, -14, c.me)) || !bytes.Equal(first[1], damageMsg(t, c.npcO, 300, c.me)) {
		t.Fatalf("first hit %x", first)
	}
	if c.p.HP != 536 {
		t.Errorf("HP %d after 500 -14 +300, want the most, 536", c.p.HP)
	}
	if got := c.dummyTick(2400 * time.Millisecond); len(got) != 6 {
		t.Fatalf("hits at 1, 1.7 and 2.4 s: %d messages", len(got))
	}
	if got := c.dummyTick(5999 * time.Millisecond); len(got) != 0 {
		t.Fatalf("more than four hits a round: %x", got)
	}
	if got := c.dummyTick(6 * time.Second); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
		t.Fatalf("the second round's swing: %x", got)
	}
	// The player lets go: the dummy fights on, as Sandstorm's did, until
	// the player walks out of its reach.
	c.p.disengage()
	if got := c.dummyTick(8400 * time.Millisecond); len(got) != 8 {
		t.Errorf("the round's four hits after a letting go: %d messages", len(got))
	}
	if got := c.dummyTick(12 * time.Second); len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeCastingCombat {
		t.Errorf("no third round for a player still beside it: %x", got)
	}
	c.p.X += 20
	if got := c.dummyTick(12500 * time.Millisecond); len(got) != 0 || c.npc.DummyTarget != 0 {
		t.Errorf("hits on a player 20 away: %x, target %d", got, c.npc.DummyTarget)
	}
}

// A Regular Dummy hits as a creature does and heals nothing.
func TestRegularDummyNoHeal(t *testing.T) {
	c := newDummy(t, "Regular Dummy Lv60", 60)
	c.w.mu.Lock()
	c.w.dummyEngagedLocked(c.npc, c.p, c.at(0))
	c.w.mu.Unlock()
	c.dummyTick(0)
	got := c.dummyTick(500 * time.Millisecond)
	if len(got) != 1 || opOfMsg(got[0]) != protocol.OpcodeDamageIndication || c.p.HP >= 500 {
		t.Errorf("a Regular Dummy's hit: %x, HP %d", got, c.p.HP)
	}
}

// One player at a time: a second player's hit does not take the dummy, and
// walking out of its reach ends the fight.
func TestDummyFightsOnePlayer(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv20", 20)
	other := &Player{ServerID: 1002, Name: "Other", HP: 100, MaxHP: 100, X: 100, Z: 100}
	c.w.Players[other.ServerID] = other
	c.w.mu.Lock()
	c.w.dummyEngagedLocked(c.npc, c.p, c.at(0))
	c.w.dummyEngagedLocked(c.npc, other, c.at(0))
	c.w.mu.Unlock()
	if c.npc.DummyTarget != c.p.ServerID {
		t.Fatalf("the dummy went to %d", c.npc.DummyTarget)
	}
	c.p.X += 20
	if got := c.dummyTick(0); len(got) != 0 || c.npc.DummyTarget != 0 {
		t.Errorf("fought a player 20 away: %x, target %d", got, c.npc.DummyTarget)
	}
}

// A spell on a dummy is told and takes nothing, and the dummy fights the caster.
func TestDummySpellHit(t *testing.T) {
	c := newDummy(t, "Training Dummy Lv45", 45)
	c.w.mu.Lock()
	killed := c.w.spellHitLocked(c.p, c.sess, c.npc, 5000)
	c.w.mu.Unlock()
	if killed || c.npc.HP != 4000 || c.npc.DummyTarget != c.p.ServerID {
		t.Errorf("killed %v, HP %d, target %d", killed, c.npc.HP, c.npc.DummyTarget)
	}
	if got := c.sent(); len(got) != 1 || !bytes.Equal(got[0], damageMsg(t, c.me, -5000, c.npcO)) {
		t.Errorf("the spell's figure: %x", got)
	}
}
