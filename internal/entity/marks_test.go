package entity

import (
	"testing"
	"time"
)

// marksHeard records the kill marks' hook.
func marksHeard(w *World) *[]KillMarks {
	for _, p := range w.Players {
		if p.CharServerID == 0 {
			p.CharServerID = uint32(p.ServerID) // a character's, as a login makes them
		}
	}
	var got []KillMarks
	w.killMarks = func(k KillMarks) { got = append(got, k) }
	return &got
}

// A creature's kill hands each earning sharer to the marks' hook with its weight (an even-con
// solo kill 1); the controls: a level 60 not reliving, a GM and a green kill are not handed on.
func TestAKillHandsEarnersToTheMarks(t *testing.T) {
	k := newKillWorld(t, 20)
	got := marksHeard(k.w)
	npc := &NPC{ServerID: 5101, Name: "orc pawn", Level: 20, X: 1000, Z: 1000, SpawnX: 1010, SpawnZ: 990, World: 0}
	k.w.awardKillXPLocked(k.sess[0], k.players[0], npc)
	if len(*got) != 1 || (*got)[0].Weight != 1 || (*got)[0].SpawnX != 1010 {
		t.Fatalf("an even-con solo kill: %+v, want one at weight 1 with its spawn", *got)
	}
	// a red kill weighs more, to 1.5
	*got = nil
	k.w.awardKillXPLocked(k.sess[0], k.players[0], &NPC{ServerID: 5102, Name: "orc boss", Level: 30, X: 1000, Z: 1000})
	if len(*got) != 1 || (*got)[0].Weight != KillMarksWeightMost {
		t.Fatalf("a red kill: %+v, want weight %v", *got, KillMarksWeightMost)
	}
	// a green kill gives no experience: no roll
	*got = nil
	k.w.awardKillXPLocked(k.sess[0], k.players[0], &NPC{ServerID: 5103, Name: "rat", Level: 5, X: 1000, Z: 1000})
	if len(*got) != 0 {
		t.Fatalf("a green kill rolled: %+v", *got)
	}
	// a level 60 earns nothing; reliving it does
	sixty := newKillWorld(t, 60)
	g60 := marksHeard(sixty.w)
	sixty.w.awardKillXPLocked(sixty.sess[0], sixty.players[0], &NPC{ServerID: 5104, Name: "drake", Level: 60, X: 1000, Z: 1000})
	if len(*g60) != 0 {
		t.Fatalf("a level 60 rolled: %+v", *g60)
	}
	gm := newKillWorld(t, 20)
	gm.players[0].GM = true
	ggm := marksHeard(gm.w)
	gm.w.awardKillXPLocked(gm.sess[0], gm.players[0], npc)
	if len(*ggm) != 0 {
		t.Fatalf("a GM rolled: %+v", *ggm)
	}
}

// The group bonus counts the other earners of the group near: four earners count three; a level 60
// in the group (not reliving) adds nothing.
func TestMarksGroupCountsEarnersNear(t *testing.T) {
	k := newKillWorld(t, 20, 20, 20, 20)
	if n := k.w.marksGroupLocked(k.players[0]); n != 3 {
		t.Fatalf("four earners: %d others, want 3", n)
	}
	mixed := newKillWorld(t, 20, 60)
	if n := mixed.w.marksGroupLocked(mixed.players[0]); n != 0 {
		t.Fatalf("with a level 60: %d, want 0", n)
	}
	mixed.players[1].Relive = &ReliveState{Level: 60}
	mixed.players[1].Level = 30
	if n := mixed.w.marksGroupLocked(mixed.players[0]); n != 1 {
		t.Fatalf("with a reliving 60: %d, want 1", n)
	}
}

// No player's death ever reaches the marks (the owner, 2026-11-22: marks only from creatures and
// quests): a flagged player killed in PvP hands nothing on; the control, a creature's kill by the
// same player, does.
func TestNoMarksFromAPvPKill(t *testing.T) {
	d := newPvP(t)
	d.a.Level, d.b.Level = 20, 20
	d.a.CharServerID, d.b.CharServerID = 51, 52
	got := marksHeard(d.w)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	d.w.mu.Lock()
	d.b.HP = 3
	d.w.duelHurtLocked(d.w.pvpFightLocked(d.a, d.b), d.a, d.a.ServerID, d.b, 5000, d.at(time.Second), numberMelee)
	dead := d.b.IsDead()
	d.w.mu.Unlock()
	if !dead {
		t.Fatal("not dead")
	}
	if len(*got) != 0 {
		t.Fatalf("a PvP kill handed on marks: %+v", *got)
	}
	d.w.mu.Lock()
	d.w.awardKillXPLocked(d.sa, d.a, &NPC{ServerID: 5201, Name: "orc pawn", Level: 20, X: d.a.X, Z: d.a.Z})
	d.w.mu.Unlock()
	if len(*got) != 1 || (*got)[0].ServerID != d.a.CharServerID {
		t.Fatalf("the control, a creature's kill: %+v", *got)
	}
}

// A Relive plays at its level and saves its true one: the progress written is the 60's, the Relive's
// row its own; its experience fills the Relive level's bar and never its masteries; ending it puts
// the 60 back.
func TestReliveSavesTheTrueLevel(t *testing.T) {
	k := newKillWorld(t, 60)
	p, s := k.players[0], k.sess[0]
	p.CharServerID = 77
	p.XPInLevel = 0
	var rows []ReliveSave
	k.w.OnReliveSave(func(r ReliveSave) { rows = append(rows, r) })
	if why := k.w.StartRelive(s, 20, 0, 0, time.Unix(1000, 0)); why != "" {
		t.Fatalf("refused: %s", why)
	}
	if p.Level != 20 || !p.Reliving() || len(rows) != 1 || !rows[0].Active || rows[0].Level != 20 {
		t.Fatalf("level %d, rows %+v", p.Level, rows)
	}
	level, _, _ := p.SavedProgress()
	if level != 60 {
		t.Fatalf("a save writes level %d, want 60", level)
	}
	if ps := progressOf(p); ps.Level != 60 || !ps.Relive || ps.ReliveRow.Level != 20 {
		t.Fatalf("progress %+v", ps)
	}
	mastery := p.Mastery
	k.w.mu.Lock()
	k.w.awardXP(s, p, CharXPDict[20]+5)
	k.w.mu.Unlock()
	if p.Level != 21 || p.XPInLevel != 5 || p.Mastery != mastery {
		t.Fatalf("after a bar's worth: level %d, %d in it, mastery %+v (was %+v)", p.Level, p.XPInLevel, p.Mastery, mastery)
	}
	// the control: a 60 not reliving puts it into its masteries
	if why := k.w.StartRelive(s, 30, 0, 0, time.Now()); why != ReliveAlready {
		t.Fatalf("a second Relive: %q", why)
	}
	r, ok := k.w.EndRelive(s)
	if !ok || r.Active || r.Level != 21 || r.XPInLevel != 5 || p.Level != 60 || p.Reliving() {
		t.Fatalf("ended: %+v, level %d", r, p.Level)
	}
}

// A reliving character may not cast a spell above its Relive level, and spends nothing for it; the
// control casts it at 60.
func TestReliveRefusesSpellsAboveItsLevel(t *testing.T) {
	w := NewWorld()
	p := &Player{Name: "R", Level: 20, Relive: &ReliveState{Level: 60}}
	if why := w.reliveCastRefusalLocked(p, &CastSpell{SpellID: 1, Level: 40}); why != ReliveCastLine {
		t.Fatalf("a level 40 spell at Relive 20: %q", why)
	}
	if why := w.reliveCastRefusalLocked(p, &CastSpell{SpellID: 1, Level: 20}); why != "" {
		t.Fatalf("a level 20 spell: %q", why)
	}
	if why := w.reliveCastRefusalLocked(&Player{Level: 60}, &CastSpell{SpellID: 1, Level: 40}); why != "" {
		t.Fatalf("not reliving: %q", why)
	}
}
