package handlers

import (
	"strconv"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// /aura (gmaura.go): a GM's aura on its target creature's name (npctint) and
// on an item's worn piece (itemaura), each kept, audited and in the world at
// once; bad input and a player's /aura change nothing (the controls).
func TestGMAuraCommand(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	targetAs(gp, campWolfNear, 100101, gs, ps)

	c.say(ps, "/aura both FF2020FF")
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM npctint`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a player's /aura: %d rows, %v", n, err)
	}
	c.say(gs, "/aura sparkly FF2020FF")
	if l := gmReplies(t, gs); !hasLine(l, "Usage") {
		t.Fatalf("a bad kind: %q", l)
	}

	c.say(gs, "/aura both FF2020FF 88EC092D 12")
	if l := gmReplies(t, gs); !hasLine(l, "Every wolf") {
		t.Fatalf("/aura: %q", l)
	}
	tints := c.gw.NPCTints()
	want := database.NPCTint{Name: "wolf", Color: 0xFFFFFFFF, Glow: 0xFF2020FF, AuraKind: database.AuraBoth, AuraSource: 0x88EC092D, AuraNodes: 12}
	if len(tints) != 1 || tints[0] != want {
		t.Fatalf("the world's tints %+v, want %+v", tints, want)
	}
	if auditLines(t, db, "npctint", "insert") != 1 {
		t.Fatal("not audited")
	}
	// particles alone, no glow: kept (they need none)
	c.say(gs, "/aura particles 0")
	if tints := c.gw.NPCTints(); len(tints) != 1 || tints[0].AuraKind != database.AuraParticles || tints[0].Glow != 0 {
		t.Fatalf("particles alone: %+v", tints)
	}
	c.say(gs, "/aura off")
	if tints := c.gw.NPCTints(); len(tints) != 0 || auditLines(t, db, "npctint", "update") != 2 {
		t.Fatalf("/aura off: %+v", tints)
	}

	// an item's worn piece
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 7, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Glowing Bracer', '', 4, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	c.say(gs, "/aura item 999999 shell 3060FFFF")
	if l := gmReplies(t, gs); !hasLine(l, "no item 999999") || c.gw.ItemAuraCount() != 0 {
		t.Fatalf("no such item: %q", l)
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" shell 3060FFFF 0 4")
	if l := gmReplies(t, gs); !hasLine(l, "on its worn piece at once") || c.gw.ItemAuraCount() != 1 {
		t.Fatalf("/aura item: %q", l)
	}
	got, err := db.ItemAuras()
	if a := got[int(pid)]; err != nil || a != (database.ItemAura{PatternID: int(pid), Color: 0x3060FFFF, Kind: database.AuraShell, Nodes: 4}) {
		t.Fatalf("the row: %+v, %v", a, err)
	}
	// the trim in black: a dark aura, kept though its colour lights nothing
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" trim 000000FF")
	if l := gmReplies(t, gs); !hasLine(l, "aura trim in 000000FF (dark)") || c.gw.ItemAuraCount() != 1 {
		t.Fatalf("/aura item trim black: %q", l)
	}
	if got, _ := db.ItemAuras(); got[int(pid)] != (database.ItemAura{PatternID: int(pid), Color: 0x000000FF, Kind: database.AuraTrim}) {
		t.Fatalf("the dark trim row: %+v", got[int(pid)])
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" 7 FFC040FF")
	if l := gmReplies(t, gs); !hasLine(l, "Usage") {
		t.Fatalf("kind 7: %q", l)
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" 6 FFC040FF")
	if got, _ := db.ItemAuras(); got[int(pid)].Kind != database.AuraTrimShell {
		t.Fatalf("kind 6 by number: %+v", got[int(pid)])
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" off")
	if c.gw.ItemAuraCount() != 0 || auditLines(t, db, "itemaura", "insert") != 1 || auditLines(t, db, "itemaura", "delete") != 1 ||
		auditLines(t, db, "itemaura", "update") != 2 {
		t.Fatal("/aura item off: not deleted, or not audited")
	}
}
