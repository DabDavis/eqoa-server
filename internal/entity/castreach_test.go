package entity

import (
	"math"
	"testing"
	"time"
)

// jib's quest walk of 2026-10-30 saw Rapid Strike (reach 4.5) refused "Out of
// range." at a Training Dummy #spawn had put 3 ahead of the warrior. The test
// is CLIENT's TargetInRange, centre to centre in three dimensions, on the
// server's places of both: a copy #spawn makes (SpawnNPCCopy 3 ahead, at the
// player's own height) is in reach, so a refusal there means one of the two
// moved between the spawn and the cast (the warrior walking on, its height
// put right by the client after a #goto), not the test. The controls: the
// player 2 back, or 3.4 above it at the same 3 across, is out of reach, and
// 3.3 above is not. (A large or huge body only adds to the reach: reach.go.)
func TestASpawnedTargetThreeAheadIsInReach(t *testing.T) {
	c := newCaster(t)
	tpl := &NPC{ServerID: 2003, Name: "Training Dummy Lv20", Level: 20, HP: 3000, MaxHP: 3000, X: 400, Y: 10, Z: 400}
	c.w.NPCs[tpl.ServerID] = tpl
	c.p.Facing = 1.0
	// as #spawn places it (handlers/devcmds.go spawnCommand)
	x := c.p.X + 3*float32(math.Sin(float64(c.p.Facing)))
	z := c.p.Z + 3*float32(math.Cos(float64(c.p.Facing)))
	id, _, ok := c.w.SpawnNPCCopy("Training Dummy Lv20", x, c.p.Y, z, c.p.Facing+math.Pi, c.p.World)
	if !ok {
		t.Fatal("no copy made")
	}
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = id, 0x01312E99
	rapid := CastSpell{SpellID: 1525, DisplayIndex: 7, Scope: 1, Name: "Rapid Strike", Recast: 30 * time.Second,
		PowerCost: 98, Range: 4.5, Def: ParseSpellText("Target, Decrease Hitpoints by 40 ; Increase 10% based on STR")}
	try := func() string {
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		ids, why := c.w.castTargetsLocked(c.sess, c.p, &rapid, 0x01312E99)
		if why == "" && (len(ids) != 1 || ids[0] != id) {
			t.Fatalf("landed on %v, want the copy %d", ids, id)
		}
		return why
	}
	if why := try(); why != "" {
		t.Fatalf("the copy 3 ahead: %q, want in reach", why)
	}
	// the controls
	x0, y0, z0 := c.p.X, c.p.Y, c.p.Z
	c.p.X -= 2 * float32(math.Sin(float64(c.p.Facing)))
	c.p.Z -= 2 * float32(math.Cos(float64(c.p.Facing)))
	if why := try(); why != castOutOfRange {
		t.Fatalf("2 back (5 across): %q, want %q", why, castOutOfRange)
	}
	c.p.X, c.p.Z = x0, z0
	c.p.Y = y0 + 3.4
	if why := try(); why != castOutOfRange {
		t.Fatalf("3 across and 3.4 up: %q, want %q", why, castOutOfRange)
	}
	c.p.Y = y0 + 3.3
	if why := try(); why != "" {
		t.Fatalf("3 across and 3.3 up (4.46 off): %q, want in reach", why)
	}
}
