package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// #phcheck (phcheck.go): a GM's and a sub GM's, as /phcheck, #phcheck and a
// PS2's "/t phcheck"; a player's an unknown command (or a tell). By name or
// target, a named's placeholders (row, where, state, odds) and whether it is
// up; a placeholder's named, its odds and its state; alone, the nameds near.
// Every line fits a PS2's text box. Read-only: no row, binding, audit line or
// creature changes. Over campWorld: the near wolf (101) bound to "wolf 90,
// pariah of Xrag 10", the far wolf (102) npcrare's "pariah of Xrag" at 20%,
// the pariahs' own row (103) up.

func TestPhCheckIsAGMsAndASubGMsAndReadOnly(t *testing.T) {
	c, db, _, gs, gp, ps, pp := campWorldWithPlayer(t)
	c.h.SetAccessLevels(subs{levels{1: 1, 2: 0, 3: 0}, map[int]bool{3: true}})
	ts, tp := c.join(3, "Tester", 25020, 15020, 0)
	ts.SetAccountID(3)
	for _, q := range []string{
		`INSERT INTO campspawns (npcid, camp, kinds, weights) VALUES (101, 0, 'wolf, pariah of Xrag', '90, 10')`,
		`INSERT INTO npcrare (npc_row, rare, chance) VALUES (102, 'pariah of xrag', 20)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.gw.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	if n, err := c.gw.LoadRareSpawns(db); err != nil || n != 1 {
		t.Fatalf("rares: %d %v", n, err)
	}
	snapshot := func() string {
		var b strings.Builder
		for _, q := range []string{
			`SELECT group_concat(id || npc_name || x_coord || z_coord, ';') FROM npcs`,
			`SELECT group_concat(npcid || kinds || weights, ';') FROM campspawns`,
			`SELECT group_concat(npc_row || rare || chance, ';') FROM npcrare`,
			`SELECT count(*) FROM gm_audit`,
		} {
			var s any
			db.QueryRow(q).Scan(&s)
			b.WriteString(toText(s) + "#")
		}
		for _, id := range []entity.EntityID{campWolfNear, campWolfFar, campPariah} {
			n, _ := c.gw.NPCStateOf(id)
			b.WriteString(n.Name + "#")
		}
		return b.String()
	}
	before := snapshot()
	gmReplies(t, gs)
	gmReplies(t, ps)
	gmReplies(t, ts)
	fits := func(who string, lines []string) {
		t.Helper()
		for _, l := range lines {
			if len(l) >= 200 || strings.Contains(l, "|") {
				t.Fatalf("%s: a line a PS2 cannot show (%d): %q", who, len(l), l)
			}
		}
	}

	// the player: an unknown command in each spelling, "/t phcheck" a tell
	for _, line := range []string{"/phcheck", "#phcheck", "/phcheck pariah of Xrag", "#phcheck wolf"} {
		c.say(ps, line)
		if l := gmReplies(t, ps); !hasLine(l, "Unrecognized command") || hasLine(l, "placeholder") {
			t.Fatalf("a player's %s: %q", line, l)
		}
	}
	c.say(ps, "/t phcheck pariah of Xrag")
	if l := gmReplies(t, ps); hasLine(l, "placeholder") {
		t.Fatalf("a player's /t phcheck: %q", l)
	}
	_ = pp

	// the named by name: up (its own row, on no placeholder), one at a time
	// (npcrare), its two spots nearest first with their odds
	c.say(gs, "#phcheck PARIAH of xrag")
	l := gmReplies(t, gs)
	fits("named", l)
	if !hasLine(l, "pariah of Xrag: 2 placeholders, UP (row 103, 283 away (25200, 15200), not on a PH). Its npcrare PHs roll only while none is up.") ||
		!hasLine(l, "  101 wolf, 113 away (25080, 15080): up, 10%") ||
		!hasLine(l, "  102 wolf, 29155 away (40000, 40000): up, 20%") {
		t.Fatalf("#phcheck pariah: %q", l)
	}
	if strings.Index(strings.Join(l, "\n"), "  101 wolf") > strings.Index(strings.Join(l, "\n"), "  102 wolf") {
		t.Fatalf("not nearest first: %q", l)
	}

	// dead: the named not up, the near wolf's corpse and return
	if !c.gw.KillNPCNow(campPariah) || !c.gw.KillNPCNow(campWolfNear) {
		t.Fatal("not killed")
	}
	c.say(gs, "/phcheck pariah of xrag")
	l = gmReplies(t, gs)
	if !hasLine(l, "pariah of Xrag: 2 placeholders, not up.") || !hasLine(l, "  101 wolf, 113 away (25080, 15080): corpse lies, back in ~") {
		t.Fatalf("after the deaths: %q", l)
	}

	// the target a placeholder: its named, its odds, its state
	targetAs(gp, campWolfNear, 100107, gs)
	c.say(gs, "#phcheck")
	l = gmReplies(t, gs)
	fits("placeholder", l)
	if !hasLine(l, "wolf (row 101) is a placeholder, 113 away (25080, 15080): corpse lies, back in ~") ||
		!hasLine(l, "  Pops: wolf 90%, pariah of Xrag 10%") ||
		!hasLine(l, "  pariah of Xrag 10% a respawn; not up (2 PHs).") {
		t.Fatalf("target placeholder: %q", l)
	}
	// the far wolf: npcrare's rule
	targetAs(gp, campWolfFar, 100108, gs)
	c.say(gs, "#phcheck")
	if l = gmReplies(t, gs); !hasLine(l, "wolf (row 102) is a placeholder") || !hasLine(l, "  Pops: rare pariah of Xrag 20%") {
		t.Fatalf("target npcrare placeholder: %q", l)
	}
	// the target a named: its placeholders
	targetAs(gp, campPariah, 100109, gs)
	c.say(gs, "#phcheck")
	if l = gmReplies(t, gs); !hasLine(l, "pariah of Xrag: 2 placeholders") {
		t.Fatalf("target named: %q", l)
	}
	// neither
	targetAs(gp, campPatroller, 100110, gs)
	c.say(gs, "#phcheck")
	if l = gmReplies(t, gs); !hasLine(l, "a patroller (row 104) is neither a named with a placeholder nor a placeholder.") {
		t.Fatalf("target neither: %q", l)
	}
	// a placeholder by name
	gp.TargetObjectID = 0xFFFFFFFF
	c.say(gs, "#phcheck wolf")
	if l = gmReplies(t, gs); !hasLine(l, "wolf is a placeholder on 2 spots, for 1 named:") ||
		!hasLine(l, "  pariah of Xrag 10%-20% a respawn on 2 of its 2 PHs; not up. /phcheck pariah of Xrag lists them.") {
		t.Fatalf("placeholder by name: %q", l)
	}
	c.say(gs, "#phcheck nobody at all")
	if l = gmReplies(t, gs); !hasLine(l, "nobody at all: no named with a placeholder and no placeholder is called that.") {
		t.Fatalf("unknown: %q", l)
	}
	// alone: the nameds near (the near wolf's spot is within the radius)
	c.say(gs, "#phcheck")
	l = gmReplies(t, gs)
	fits("near", l)
	if !hasLine(l, "Nameds with placeholders within 400 (1):") ||
		!hasLine(l, "  pariah of Xrag: not up; 1 PH near (2 in all), nearest row 101 wolf 113 away, 10%.") {
		t.Fatalf("alone: %q", l)
	}

	// the sub GM: /phcheck, and a PS2's /t phcheck
	tp.TargetObjectID = 0xFFFFFFFF
	c.say(ts, "/phcheck pariah of xrag")
	if l = gmReplies(t, ts); !hasLine(l, "pariah of Xrag: 2 placeholders, not up.") {
		t.Fatalf("the sub GM's /phcheck: %q", l)
	}
	c.say(ts, "/t phcheck wolf")
	if l = gmReplies(t, ts); !hasLine(l, "wolf is a placeholder on 2 spots") {
		t.Fatalf("the sub GM's /t phcheck: %q", l)
	}
	// /help: the sub GM's has it, the GM's #help, the player's not
	c.say(ts, "/help")
	if l = gmReplies(t, ts); !hasLine(l, "/phcheck [name]") {
		t.Fatalf("the sub GM's /help: %q", l)
	}
	c.say(gs, "#help")
	if l = gmReplies(t, gs); !hasLine(l, "#phcheck [name]") {
		t.Fatalf("the GM's #help: %q", l)
	}
	c.say(ps, "/help")
	if l = gmReplies(t, ps); hasLine(l, "phcheck") {
		t.Fatalf("the player's /help: %q", l)
	}

	// read-only: rows, bindings, rules, audit and the creatures' kinds as they
	// were (the two killed by the test itself, still dead)
	if after := snapshot(); after != before {
		t.Fatalf("#phcheck changed something:\n%s\n%s", before, after)
	}
	for _, id := range []entity.EntityID{campPariah, campWolfNear} {
		if n, _ := c.gw.NPCStateOf(id); !n.Dead {
			t.Fatalf("NPC %d came back", id)
		}
	}
}

// toText is a scanned value as text.
func toText(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}
