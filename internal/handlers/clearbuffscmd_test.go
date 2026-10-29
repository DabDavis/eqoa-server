package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// #clearbuffs (F3 20261029-011146) is a GM's, and the GM's own buffs alone:
// a player's is answered as an unknown command and its buffs stay; the GM's
// /clearbuffs ends the GM's and says which, the other player's staying (the
// control); a second clear, as #nukebuffs, finds none.
func TestClearBuffsIsAGMsOwn(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	rows := []database.BuffRow{
		{SpellID: 101, Name: "Rockskin", Icon: 0x1111, LeftMS: 1_800_000, TickMS: -1, Mods: `{"AC":150}`},
		{SpellID: 102, Name: "Spirit of the Wolf", Icon: 0x2222, LeftMS: 1_800_000, TickMS: -1, Flags: 4},
	}
	now := time.Now()
	if c.gw.RestoreEffects(gs, rows, now) != 2 || c.gw.RestoreEffects(ps, rows, now) != 2 {
		t.Fatal("the buffs did not go on")
	}

	c.say(ps, "#clearbuffs")
	if got := heard(t, ps); len(got) != 1 || got[0].op != protocol.OpcodeErrorMessage || got[0].text != `Unrecognized command "#clearbuffs".` {
		t.Fatalf("a player's #clearbuffs: %+v", got)
	}
	if len(pp.Effects) != 2 || pp.Mods.AC != 150 {
		t.Fatalf("a player's #clearbuffs took its buffs: %d, AC %+d", len(pp.Effects), pp.Mods.AC)
	}

	c.say(gs, "/clearbuffs")
	if got := texts(heard(t, gs)); !slices.Contains(got, "[GM] Buffs cleared (2): Rockskin, Spirit of the Wolf") {
		t.Fatalf("the GM's /clearbuffs: %q", got)
	}
	if len(gp.Effects) != 0 || len(gp.Buffs) != 0 || gp.Mods.AC != 0 || gp.MoveRate() != 20 {
		t.Errorf("the GM keeps %d effects, %d slots, AC %+d, move rate %d", len(gp.Effects), len(gp.Buffs), gp.Mods.AC, gp.MoveRate())
	}
	if len(pp.Effects) != 2 || pp.Mods.AC != 150 {
		t.Errorf("the control: the player lost its buffs to the GM's clear: %d, AC %+d", len(pp.Effects), pp.Mods.AC)
	}

	gs.Queue().GetPending()
	c.say(gs, "#nukebuffs")
	if got := texts(heard(t, gs)); len(got) != 1 || got[0] != "[GM] No buffs on you" {
		t.Errorf("a second clear: %q", got)
	}
}
