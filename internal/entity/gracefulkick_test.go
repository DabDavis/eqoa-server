package entity

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// gracefulKickRank is the rank as tools/seed_graceful_kick.py writes it.
func gracefulKickRank() database.CMRankRow {
	return database.CMRankRow{ID: 900916, Category: 3, Classes: 1 << 5, Races: 2047, Mastery: "Graceful Kick", Rank: 1,
		Name: "Graceful Kick", Level: 16, Effects: "GracefulKick:1", Enabled: true}
}

// swingArt is an art whose record plays Attack1, as Graceful Strike's does
// (spelleffect 700148: 0xDA9BEA11).
func swingArt(id int, name string) CastSpell {
	s := artOf(id, name)
	s.FX = HashResourceID("Attack1")
	return s
}

// castVisualSent is the visual of the one 0x00B1 the cast sent its caster.
func castVisualSent(t *testing.T, c *caster) uint32 {
	t.Helper()
	var fx []uint32
	for _, m := range c.sent() {
		if opOfMsg(m) == protocol.OpcodeCastingCombat {
			fx = append(fx, binary.LittleEndian.Uint32(m[len(m)-4:]))
		}
	}
	if len(fx) != 1 {
		t.Fatalf("%d casts sent (%x), want one", len(fx), fx)
	}
	return fx[0]
}

// The owner, 2026-11-16: "lets add a 0 cost cm for bards graceful strike to
// kick animation". Without the rank Graceful Strike plays its own swing; with
// it, the Kick art's visual (SetAction 43, the kick); the rank taken out, the
// swing again. The control: another art keeps its own under the rank.
func TestGracefulStrikeKicksWithTheRank(t *testing.T) {
	c := newCaster(t)
	c.p.Class, c.p.Level = 5, 20 // a bard
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	c.w.SetCMCatalog([]database.CMRankRow{gracefulKickRank()})
	c.w.RestoreCMs(c.sess, nil)
	c.sent()
	gs := swingArt(700148, "Graceful Strike")
	attack1 := HashResourceID("Attack1")

	step := time.Duration(0)
	cast := func(s CastSpell) uint32 {
		t.Helper()
		step += 20 * time.Second
		c.tick(step - time.Millisecond)
		c.sent()
		if !c.w.StartCast(c.sess, s, c.npcO, c.at(step)) {
			t.Fatalf("%q was not begun", s.Name)
		}
		return castVisualSent(t, c)
	}

	if fx := cast(gs); fx != attack1 {
		t.Errorf("without the rank: visual %08X, want Attack1 %08X", fx, attack1)
	}
	c.w.RestoreCMs(c.sess, []database.OwnedCM{{ID: 900916}})
	if !c.p.GracefulKick {
		t.Fatal("the rank owned, GracefulKick not set")
	}
	if fx := cast(gs); fx != fxKick {
		t.Errorf("with the rank: visual %08X, want Kick %08X", fx, uint32(fxKick))
	}
	if fx := cast(swingArt(700149, "Rapid Strike")); fx != attack1 {
		t.Errorf("another art with the rank: visual %08X, want its own %08X", fx, attack1)
	}
	c.w.RestoreCMs(c.sess, nil)
	if fx := cast(gs); fx != attack1 {
		t.Errorf("the rank taken out: visual %08X, want Attack1 again", fx)
	}
}

// Kick's visual is the disc's "Kick" by its name, the Kick art's own
// (spelleffect 1604 and its kin: 0x0A286E2E).
func TestKickVisualIsKicksName(t *testing.T) {
	if got := HashResourceID("Kick"); got != fxKick {
		t.Errorf("HashResourceID(Kick) = %08X, fxKick %08X", got, uint32(fxKick))
	}
}

// The rank's effects key is read on its own, slot-free; a rank without it
// (or at 0) is no kick.
func TestGracefulKickIsReadOffTheEffects(t *testing.T) {
	r := ParseCMRank(gracefulKickRank())
	if !r.GracefulKick || r.Effects != ([CMSlots]int{}) {
		t.Errorf("the rank: GracefulKick %v, effects %v; want true and none", r.GracefulKick, r.Effects)
	}
	for _, effects := range []string{"", "GracefulKick:0", "NoWereIllusion:1", "STR:5"} {
		row := gracefulKickRank()
		row.Effects = effects
		if ParseCMRank(row).GracefulKick {
			t.Errorf("effects %q read as GracefulKick", effects)
		}
	}
}
