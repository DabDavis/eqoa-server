package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// #setlevel names any character (Jibrish, petition 1265: "/setlevel Nuke 60
// should set the character nuke to 60 if im targeting them, but not add or
// remove spells or change anything beyond level"): by name, in either order;
// with none the GM's target player, else the GM; a character out of the world
// on its row. The player is told, and is sent its level (0x0020) and no
// ability (0x00B3). The control: a player's #setlevel sets nobody's.
func TestSetLevelTakesANameOrTheTarget(t *testing.T) {
	w := newResetWorld(t)
	w.gp.TargetObjectID = 0xFFFFFFFF
	gmReplies(t, w.ps)
	w.say(w.gs, "#setlevel player 30")
	rec, _ := w.store.GetByServerID(2)
	if w.pp.Level != 30 || rec.Level != 30 || rec.TotalXP != entity.TotalXP(30, 0) || w.gp.Level != 60 {
		t.Fatalf("by name: the player %d (its row %d with %d xp), the GM %d", w.pp.Level, rec.Level, rec.TotalXP, w.gp.Level)
	}
	if d := sentOp(w.ps, protocol.OpcodeGrantXP); len(d) < 3 || d[2] != 30*2 { // zigzag
		t.Errorf("0x0020 to the player: %x", d)
	}
	if d := sentOp(w.ps, protocol.OpcodeAbilityAdd); d != nil {
		t.Errorf("an ability was sent: %x", d)
	}
	if l := gmReplies(t, w.ps); !hasLine(l, "A GM set your level to 30.") {
		t.Errorf("the player was told %q", l)
	}
	if l := gmReplies(t, w.gs); !hasLine(l, "Player's level set to 30") {
		t.Errorf("the GM was told %q", l)
	}
	w.say(w.gs, "#setlevel 35 player")
	if w.pp.Level != 35 {
		t.Errorf("the level first: %d", w.pp.Level)
	}

	// the GM's target, a player
	w.group(t, w.gs, w.gp, w.ps, w.pp)
	w.gp.TargetObjectID = w.gw.IDSeenBy(w.gs, w.pp)
	w.say(w.gs, "/setlevel 45")
	if w.pp.Level != 45 || w.gp.Level != 60 {
		t.Fatalf("the target: the player %d, the GM %d", w.pp.Level, w.gp.Level)
	}
	// no target: the GM's own
	w.gp.TargetObjectID = 0xFFFFFFFF
	w.say(w.gs, "#setlevel 20")
	if w.gp.Level != 20 || w.pp.Level != 45 {
		t.Fatalf("no target: the GM %d, the player %d", w.gp.Level, w.pp.Level)
	}
	// out of the world: its row, for its next login
	w.say(w.gs, "/setlevel away 12")
	if away, _ := w.store.GetByServerID(3); away.Level != 12 || away.TotalXP != entity.TotalXP(12, 0) {
		t.Fatalf("Away's row: level %d with %d xp", away.Level, away.TotalXP)
	}
	if l := gmReplies(t, w.gs); !hasLine(l, "Away's level set to 12") {
		t.Errorf("the GM was told %q", l)
	}

	w.say(w.ps, "#setlevel gm 1")
	w.say(w.ps, "/setlevel 1")
	if w.gp.Level != 20 || w.pp.Level != 45 {
		t.Errorf("a player's #setlevel: the GM %d, the player %d", w.gp.Level, w.pp.Level)
	}
}

// #testmode and #tankprofile1-3 (the GM Jibrish's petition 1272) toggle on a
// GM, in either spelling, and say so; a player's are an unknown command and
// change nothing.
func TestTestModeAndTankProfilesToggle(t *testing.T) {
	c, gs, gp, ps, pp := gmWorld(t)
	c.say(gs, "/testmode")
	if !gp.GMTestMode || !hasLine(gmReplies(t, gs), "Test mode on") {
		t.Fatal("a GM's /testmode: not on")
	}
	c.say(gs, "#testmode")
	if gp.GMTestMode {
		t.Fatal("a second #testmode: still on")
	}
	c.say(gs, "/tankprofile2")
	if gp.GMTankProfile != 2 || !hasLine(gmReplies(t, gs), "AGI 400 and AC 2000") {
		t.Fatalf("/tankprofile2: profile %d", gp.GMTankProfile)
	}
	c.say(gs, "/tankprofile3")
	c.say(gs, "/tankprofile3")
	if gp.GMTankProfile != 0 {
		t.Fatalf("/tankprofile3 twice: profile %d", gp.GMTankProfile)
	}
	c.say(ps, "/testmode")
	c.say(ps, "/tankprofile1")
	if pp.GMTestMode || pp.GMTankProfile != 0 {
		t.Errorf("a player's: test mode %v, profile %d", pp.GMTestMode, pp.GMTankProfile)
	}
}
