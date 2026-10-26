package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A "Sparring Dummy" row is a sparring dummy that swings Attack4 at any
// level (F3 20261025-174503: "it just quads you"); the control: a Regular
// Dummy of level 10 swings its level's Attack1.
func TestASparringDummyQuadsAtAnyLevel(t *testing.T) {
	spar := NPCFromRecord(&database.NPCRecord{ID: 1, NPCName: "Sparring Dummy", NPCLevel: 10, Size: 0.25}, 0, nil)
	if spar.Dummy != SparringDummy || SwingEffectName(npcSwingKind(spar)) != "Attack4" {
		t.Errorf("a Sparring Dummy Lv10: kind %d, swing %s, want a sparring dummy's Attack4", spar.Dummy, SwingEffectName(npcSwingKind(spar)))
	}
	reg := NPCFromRecord(&database.NPCRecord{ID: 2, NPCName: "Regular Dummy Lv10", NPCLevel: 10}, 0, nil)
	if SwingEffectName(npcSwingKind(reg)) != "Attack1" {
		t.Errorf("the control: a Regular Dummy Lv10 swings %s, want its level's Attack1", SwingEffectName(npcSwingKind(reg)))
	}
}

// A strike's damage follows the dummy's level and the share the player's
// resist took (resist.go): a level 60's 600 at the middle of its spread, a
// resist of 210 taking half, the cap's 75% a quarter left, never below 1.
func TestASparStrikeFollowsLevelAndResist(t *testing.T) {
	for _, tt := range []struct {
		level, resist int
		roll          float64
		want          int
	}{
		{60, 0, 0.5, 600},
		{60, 210, 0.5, 300},
		{10, 0, 0.5, 100},
		{60, 0, 0, 480},
		{60, 0, 0.999999, 720},
		{60, 314, 0.5, 151},
	} {
		if got := sparStrikeDamage(tt.level, ResistShare(tt.resist), tt.roll); got != tt.want {
			t.Errorf("level %d on resist %d (roll %v): %d, want %d", tt.level, tt.resist, tt.roll, got, tt.want)
		}
	}
	if got := sparStrikeDamage(10, 1, 0.5); got != 1 {
		t.Errorf("a strike resisted whole: %d, want 1", got)
	}
}

// A sparring dummy set to level 40 and cold fights in Attack4 rounds, and
// on every second round strikes with cold 3 s in, said in the chat; it
// never kills, but stops at one hit point.
func TestASparringFight(t *testing.T) {
	c := newDummy(t, "Sparring Dummy", 10)
	c.npc.SwingFixed, c.npc.SwingKind = true, sparSwing
	c.p.HP, c.p.MaxHP = 100000, 100000
	if _, _, ok := c.w.SetSparring(c.npc.ServerID, c.p.ServerID, 3, 1); !ok || c.npc.Level != 40 {
		t.Fatalf("set to level index 3, cold: ok %v, level %d", ok, c.npc.Level)
	}
	c.w.mu.Lock()
	c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	c.sent()

	var hits, strikes, swings int
	var line string
	for d := time.Duration(0); d <= 12500*time.Millisecond; d += 100 * time.Millisecond {
		for _, m := range c.dummyTick(d) {
			switch opOfMsg(m) {
			case protocol.OpcodeDamageIndication:
				hits++
			case protocol.OpcodeCastingCombat:
				swings++
			case protocol.OpcodeClientMessage:
				line = string(m)
				strikes++
			}
		}
	}
	// rounds start at 0, 6 and 12 s: 4 + 4 + 1 (the third's first at 12.5)
	// melee hits, the strike 3 s into the second round, 9 s
	if strikes != 1 || hits != 4+4+1+1 {
		t.Errorf("%d hits and %d strikes in 12.5 s, want 9 hits and 1 strike (10 0x00DB)", hits, strikes)
	}
	if swings != 3+1 {
		t.Errorf("%d 0x00B1, want 3 rounds' swings and the strike's", swings)
	}
	if !strings.Contains(line, string(utf16le("Cold"))) {
		t.Errorf("the strike's line %x does not name cold", line)
	}

	// at 5 hit points the next hit leaves 1 and ends the fight
	c.p.HP = 5
	for d := 12500 * time.Millisecond; d <= 14*time.Second && c.npc.DummyTarget != 0; d += 100 * time.Millisecond {
		c.dummyTick(d)
	}
	if c.p.HP != 1 || c.p.IsDead() || c.npc.DummyTarget != 0 {
		t.Errorf("at 5 HP: HP %d, dead %v, fighting %d; want 1, alive, the fight over", c.p.HP, c.p.IsDead(), c.npc.DummyTarget)
	}
}

// Another player's pick is refused while the dummy spars with someone.
func TestASparringDummyKeepsItsFight(t *testing.T) {
	c := newDummy(t, "Sparring Dummy", 10)
	c.w.mu.Lock()
	c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
	c.w.mu.Unlock()
	if _, busy, ok := c.w.SetSparring(c.npc.ServerID, 9999, 5, 0); ok || busy != c.p.Name {
		t.Errorf("another's pick mid-fight: ok %v, busy %q; want refused, busy with %s", ok, busy, c.p.Name)
	}
	if _, _, ok := c.w.SetSparring(c.npc.ServerID, c.p.ServerID, 5, 0); !ok || c.npc.Level != 60 {
		t.Errorf("the control: its own player's pick: ok %v, level %d", ok, c.npc.Level)
	}
}
