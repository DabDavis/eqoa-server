package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// An animated combat art hits when its graphic ends (fxland.go, work board #1643). Synthetic visuals and
// figures: a 0 s art and a 2 s spell whose visual has a 2000 ms row, beside the control of no row.

const fxSpin = 0x0A0B0C0D // a made-up animated visual

// fxRig is the caster rig with the arts' cooldown at artMS on every level and a 2000 ms row for fxSpin.
func fxRig(t *testing.T, artMS float64) *caster {
	t.Helper()
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 100000, 100000
	b := SeedBalance()
	for i := range b.levels {
		b.levels[i].ArtCooldownMS = artMS
	}
	c.w.balance = b
	c.w.SetFXLand(map[uint32]float64{fxSpin: 2000})
	return c
}

func spinArt() CastSpell {
	s := artOf(801, "Spinning Blades")
	s.FX = fxSpin
	return s
}

// hitAt is the first of the times at which a tick lands a hit on the rig's creature (from 0 by 100 ms to
// limit), -1 for none.
func hitAt(c *caster, limit time.Duration) time.Duration {
	for d := time.Duration(0); d <= limit; d += 100 * time.Millisecond {
		c.tick(d)
		if c.npc.HP < c.npc.MaxHP {
			return d
		}
	}
	return -1
}

// A 0 s art whose visual has a 2000 ms row lands at 2.0 s, not 0.5 s; another art and a spell with a cast
// time sent at 1.0 s are not answered (nothing sent); an art sent at 2.0 s starts.
func TestAnAnimatedArtHitsWhenItsGraphicEnds(t *testing.T) {
	c := fxRig(t, 700)
	if !c.w.StartCast(c.sess, spinArt(), c.npcO, c.at(0)) {
		t.Fatal("the art was not begun")
	}
	c.sent()
	if got := hitAt(c, 900*time.Millisecond); got != -1 {
		t.Fatalf("it landed at %v, before its graphic's end", got)
	}
	if c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(time.Second)) {
		t.Error("an art sent at 1.0 s began")
	}
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(time.Second)) {
		t.Error("a spell with a cast time sent at 1.0 s began")
	}
	if m := c.sent(); len(m) != 0 {
		t.Errorf("the casts at 1.0 s were answered: %d messages", len(m))
	}
	for d := time.Second; d < 2*time.Second; d += 100 * time.Millisecond {
		c.tick(d)
	}
	if c.npc.HP != c.npc.MaxHP {
		t.Fatalf("it landed before 2.0 s: HP %d", c.npc.HP)
	}
	c.tick(2 * time.Second)
	if c.npc.HP == c.npc.MaxHP {
		t.Fatal("it did not land at 2.0 s")
	}
	if !c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(2*time.Second)) {
		t.Error("an art sent at 2.0 s, the hit landed, was held")
	}
}

// With the level's arts' cooldown 3000 ms, the next art waits for 3.0 s (the larger of the two).
func TestAnAnimatedArtUnderALongerArtCooldown(t *testing.T) {
	c := fxRig(t, 3000)
	if !c.w.StartCast(c.sess, spinArt(), c.npcO, c.at(0)) {
		t.Fatal("the art was not begun")
	}
	if got := hitAt(c, 2500*time.Millisecond); got != 2*time.Second {
		t.Errorf("it landed at %v, want 2s", got)
	}
	if c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(2900*time.Millisecond)) {
		t.Error("an art at 2.9 s began, inside the arts' 3 s cooldown")
	}
	if !c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(3*time.Second)) {
		t.Error("an art at 3.0 s was held")
	}
}

// A 2 s cast whose visual has a 2000 ms row lands at 4.0 s.
func TestAnAnimatedCastLandsAfterItsCastTime(t *testing.T) {
	c := fxRig(t, 700)
	s := strike()
	s.FX = fxSpin
	if !c.w.StartCast(c.sess, s, c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	if got := hitAt(c, 5*time.Second); got != 4*time.Second {
		t.Errorf("it landed at %v, want 4s", got)
	}
}

// The control: a spell whose visual has no row lands at its cast time + 0.5 s, the next art waits for the
// arts' cooldown, a spell that is no art for the cast time + 0.4 s, as before.
func TestAVisualOfNoRowLandsAsEver(t *testing.T) {
	c := fxRig(t, 700)
	if !c.w.StartCast(c.sess, artOf(801, "Slash"), c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	if got := hitAt(c, time.Second); got != castLand {
		t.Errorf("it landed at %v, want %v", got, castLand)
	}
	if c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(600*time.Millisecond)) {
		t.Error("an art at 0.6 s began, inside the arts' 0.7 s")
	}
	if !c.w.StartCast(c.sess, artOf(802, "Kick"), c.npcO, c.at(700*time.Millisecond)) {
		t.Error("an art at 0.7 s was held")
	}

	c = fxRig(t, 700)
	if !c.w.StartCast(c.sess, artOf(801, "Slash"), c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	if c.w.StartCast(c.sess, strike(), c.npcO, c.at(300*time.Millisecond)) {
		t.Error("a spell at 0.3 s began, inside the art's busy 0.4 s")
	}
	if !c.w.StartCast(c.sess, strike(), c.npcO, c.at(castBusy)) {
		t.Error("a spell at 0.4 s was held")
	}
}

// A change of the table's row (a GM's edit: LoadFXLand) is the next cast's: the row read again at 1000 ms
// lands the art at 1.0 s.
func TestAnFXLandEditIsTheNextCasts(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/fx.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := fxRig(t, 700)
	if _, err := db.Exec(`INSERT INTO fxland (fxdict, land_ms) VALUES (?, 1000)`, int64(fxSpin)); err != nil {
		t.Fatal(err)
	}
	n, err := c.w.LoadFXLand(db)
	if err != nil || n != len(database.FXLandSeeds)+1 {
		t.Fatalf("read %d rows (%v), want the seeds and the edit", n, err)
	}
	if d, ok := c.w.FXLandHit(0xEA636DCD); !ok || d != 2*time.Second {
		t.Errorf("a seed row: %v %v, want 2s", d, ok)
	}
	if !c.w.StartCast(c.sess, spinArt(), c.npcO, c.at(0)) {
		t.Fatal("not begun")
	}
	if got := hitAt(c, 3*time.Second); got != time.Second {
		t.Errorf("after the edit it landed at %v, want 1s", got)
	}
}

// The fight simulator times a cast by the same rows: the art's span is its hit and castBusy.
func TestTheSimTimesAnAnimatedArt(t *testing.T) {
	f := newSimFight(SimPlayer{Class: 1, Level: 20, FXLand: map[uint32]time.Duration{fxSpin: 2 * time.Second}}, SimNPC{Level: 20}, SeedBalance())
	art, plain := spinArt(), artOf(802, "Kick")
	if got := f.castSpan(&art); got != 2*time.Second+castBusy {
		t.Errorf("the animated art's span %v", got)
	}
	if got := f.castSpan(&plain); got != castLand+castBusy {
		t.Errorf("a plain art's span %v", got)
	}
}
