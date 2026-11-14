package handlers

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// /copy is /clone (the owner, 2026-11-14: "./copy should work for jib also on
// live server"): a GM of Jibrish's level (acctlevel 2, as on the Live Server)
// copies its target with /copy, #copy and a PS2's "/t copy"; a sub GM may as it
// may /clone; a player's /copy is an unknown command and its "/t copy" a tell.
func TestCopyIsClone(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	c.h.SetAccessLevels(levels{1: 2, 2: 0})
	pp := c.gw.SessionPlayer(ps)
	targetAs(gp, campWolfNear, 100101, gs, ps)
	pp.TargetObjectID = 100101
	rows := func() int { return count(t, db, `SELECT COUNT(*) FROM npcs`) }

	c.say(ps, "/copy")
	c.say(ps, "/t copy")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/copy"`) {
		t.Fatalf("a player's /copy: %q", l)
	}
	if n := rows(); n != 5 {
		t.Fatalf("%d rows after a player's /copy", n)
	}
	for i, cmd := range []string{"/copy", "#copy", "/t copy", "/COPY roamer"} {
		c.say(gs, cmd)
		l := gmReplies(t, gs)
		if !hasLine(l, "Cloned wolf as row") || rows() != 6+i {
			t.Fatalf("a GM's %s: %q (%d rows)", cmd, l, rows())
		}
	}
	var nid int64
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if r, ok := wanderRadius(t, db, nid); !ok || r != database.RoamerRadius {
		t.Fatalf("/copy roamer's wander: %v %v", r, ok)
	}
	var detail string
	db.QueryRow(`SELECT detail FROM gm_audit WHERE tbl = 'npcs' AND action = 'insert' ORDER BY id DESC LIMIT 1`).Scan(&detail)
	if !strings.HasPrefix(detail, "/clone in the game: wolf") {
		t.Fatalf("/copy's audit line: %q", detail)
	}

	// a sub GM's
	c.h.SetAccessLevels(subs{levels{1: 2, 2: 0}, map[int]bool{2: true}})
	c.say(ps, "/copy")
	if l := gmReplies(t, ps); !hasLine(l, "Cloned wolf as row") {
		t.Fatalf("a sub GM's /copy: %q", l)
	}
	if !npcRowTellOK("copy", "") || !npcRowTellOK("copy", "roamer 120") || npcRowTellOK("copy", "these crocs") {
		t.Fatal("/t copy's words")
	}
}

func npcRowTellOK(name, body string) bool {
	_, _, ok := npcRowTell(name, body)
	return ok
}

// mirrorWorlds is the Live Server (live: its world, database and GM, account
// 1 "jibrish", and a player, account 2) and the Heirloom Test Server (hl: its
// world and database, whose wolf near is level 99 where Live's is 12), Live's
// NPC edits by jibrish mirrored to Heirloom's database.
type mirrorWorlds struct {
	live, hl     *chatWorld
	gs, ps       *session.Session
	gp, pp       *entity.Player
	liveDB, hlDB *database.SQLiteDB
	hlPath       string
}

func newMirrorWorlds(t *testing.T) mirrorWorlds {
	t.Helper()
	live, liveDB, _, gs, gp, ps, pp := campWorldWithPlayer(t)
	hl, hlDB, hlPath, _, _, _ := campWorld(t)
	for _, q := range []struct {
		db *database.SQLiteDB
		q  string
	}{
		{liveDB, `INSERT INTO accountinfo (accountid, username) VALUES (1, 'jibrish'), (2, 'someone')`},
		{hlDB, `INSERT INTO accountinfo (accountid, username) VALUES (1, 'jibrish'), (2, 'someone')`},
		{hlDB, fmt.Sprintf(`UPDATE npcs SET npc_level = 99 WHERE id = %d`, campWolfNear)},
		{hlDB, fmt.Sprintf(`DELETE FROM npcs WHERE id = %d`, campPariah)},
		// Heirloom's own rows past Live's: its new ids are its own
		{hlDB, fmt.Sprintf(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (500, 'filler', 0, 30000, 50, 30000, 0, %d, 1.0, 5, 0, '0', 7)`, campCreature)},
		{hlDB, fmt.Sprintf(`INSERT INTO npcrespawn (npc_row, seconds) VALUES (%d, 77)`, campWolfNear)},
		{liveDB, fmt.Sprintf(`INSERT INTO npcrespawn (npc_row, seconds) VALUES (%d, 100)`, campWolfNear)},
	} {
		if _, err := q.db.Exec(q.q); err != nil {
			t.Fatal(err)
		}
	}
	hl.gw.LoadNPCs(database.NewNPCStore(hlDB))
	SetNPCMirror([]string{" JibRish "}, hlPath)
	t.Cleanup(func() { SetNPCMirror(nil, "") })
	return mirrorWorlds{live: live, hl: hl, gs: gs, gp: gp, ps: ps, pp: pp, liveDB: liveDB, hlDB: hlDB, hlPath: hlPath}
}

// mirrorOf is the Heirloom row Live's row id is mirrored as, or 0.
func (m mirrorWorlds) mirrorOf(t *testing.T, id int64) int64 {
	t.Helper()
	to, err := database.NPCMirrorOf(m.hlDB, id)
	if err != nil {
		t.Fatal(err)
	}
	return to
}

// The owner, 2026-11-14: "his placements and copies should show on heirloom
// also". jibrish's /clone on Live makes a row on Heirloom too, a copy of
// Heirloom's own row of the source (its level 99, its respawn) on the GM's spot,
// audited there "mirrored from Live: ...", in Heirloom's running world at its
// next inbox read; his /roamer, /move, /size and /delete of that row change its
// copy, each in Heirloom's world too. A source Heirloom does not have
// (pariah): Live's row's values. The controls: another GM's /clone is not
// mirrored; jibrish's /move of a row never mirrored (one both shards have) is
// not mirrored; nothing of Live's changes for the mirror.
func TestNPCEditsMirrorToHeirloom(t *testing.T) {
	m := newMirrorWorlds(t)
	c := m.live
	gs, gp, ps, pp := m.gs, m.gp, m.ps, m.pp
	hlRows := func() int { return count(t, m.hlDB, `SELECT COUNT(*) FROM npcs`) }

	// /clone (as /copy) of the wolf near
	stand(c, gp, 25500, 51, 15500, math.Pi/2)
	targetAs(gp, campWolfNear, 100101, gs)
	c.say(gs, "/copy")
	l := gmReplies(t, gs)
	var nid int64
	m.liveDB.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	hid := m.mirrorOf(t, nid)
	if hid <= 500 || !hasLine(l, fmt.Sprintf("Cloned wolf as row %d", nid)) || !hasLine(l, fmt.Sprintf("Mirrored to Heirloom as row %d.", hid)) {
		t.Fatalf("/copy: %q (Heirloom row %d)", l, hid)
	}
	hrow := npcRowMap(t, m.hlDB, hid)
	if hrow["npc_level"].(int64) != 99 || hrow["x_coord"].(float64) != 25500 || hrow["z_coord"].(float64) != 15500 ||
		hrow["facing"].(int64) != 64 || hrow["npc_name"] != "wolf" {
		t.Fatalf("Heirloom's copy: %v", hrow)
	}
	if lrow := npcRowMap(t, m.liveDB, nid); lrow["npc_level"].(int64) != 12 {
		t.Fatalf("Live's row changed: %v", lrow)
	}
	if n := count(t, m.hlDB, `SELECT COUNT(*) FROM npcrespawn WHERE npc_row = ? AND seconds = 100`, hid); n != 1 {
		t.Fatal("the respawn is not Live's row's")
	}
	if n := count(t, m.hlDB, `SELECT COUNT(*) FROM gm_audit WHERE tbl = 'npcs' AND action = 'insert' AND username = 'jibrish'
		AND detail LIKE 'mirrored from Live: /clone in the game: wolf%'`); n != 1 {
		t.Fatalf("%d Heirloom audit lines", n)
	}
	if _, ok := m.hl.gw.NPCStateOf(entity.EntityID(hid)); ok {
		t.Fatal("in Heirloom's world before its inbox is read")
	}
	if n := m.hl.h.applyNPCMirrorInbox(); n != 1 {
		t.Fatalf("Heirloom's inbox: %d lines", n)
	}
	if st, ok := m.hl.gw.NPCStateOf(entity.EntityID(hid)); !ok || st.X != 25500 || st.Z != 15500 || st.RespawnSec != 100 {
		t.Fatalf("Heirloom's world: %+v %v", st, ok)
	}

	// /roamer, /move, /size of it
	targetAs(gp, entity.EntityID(nid), 100200, gs)
	c.say(gs, "/roamer")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Its Heirloom copy (row %d) changed too.", hid)) {
		t.Fatalf("/roamer: %q", l)
	}
	if r, ok := wanderRadius(t, m.hlDB, hid); !ok || r != database.RoamerRadius {
		t.Fatalf("Heirloom's wander: %v %v", r, ok)
	}
	stand(c, gp, 25600, 52, 15700, 0)
	c.say(gs, "/move")
	if l := gmReplies(t, gs); !hasLine(l, "Moved wolf") || !hasLine(l, "Its Heirloom copy") {
		t.Fatalf("/move: %q", l)
	}
	c.say(gs, "/size 2")
	if l := gmReplies(t, gs); !hasLine(l, "keeps size") || !hasLine(l, "Its Heirloom copy") {
		t.Fatalf("/size: %q", l)
	}
	hrow = npcRowMap(t, m.hlDB, hid)
	if hrow["x_coord"].(float64) != 25600 || hrow["z_coord"].(float64) != 15700 || math.Abs(hrow["size"].(float64)-2) > 0.01 ||
		hrow["npc_level"].(int64) != 99 {
		t.Fatalf("Heirloom's copy moved and sized: %v", hrow)
	}
	m.hl.h.applyNPCMirrorInbox()
	if st, ok := m.hl.gw.NPCStateOf(entity.EntityID(hid)); !ok || st.X != 25600 || st.Z != 15700 {
		t.Fatalf("Heirloom's world after the move: %+v %v", st, ok)
	}

	// another GM's /clone: not mirrored
	c.h.SetAccessLevels(levels{1: 2, 2: 1})
	stand(c, pp, 25010, 50, 15010, 0)
	targetAs(pp, campWolfNear, 100101, ps)
	before := hlRows()
	c.say(ps, "/clone")
	if l := gmReplies(t, ps); !hasLine(l, "Cloned wolf") || hasLine(l, "Heirloom") || hlRows() != before {
		t.Fatalf("another GM's /clone: %q (%d Heirloom rows, %d before)", l, hlRows(), before)
	}
	// a row both shards have: jibrish's /move is not mirrored
	hfar := npcRowMap(t, m.hlDB, campWolfFar)
	targetAs(gp, campWolfFar, 100300, gs)
	c.say(gs, "/move")
	if l := gmReplies(t, gs); !hasLine(l, "Moved wolf") || hasLine(l, "Heirloom") {
		t.Fatalf("/move of an unmirrored row: %q", l)
	}
	if fmt.Sprint(npcRowMap(t, m.hlDB, campWolfFar)) != fmt.Sprint(hfar) {
		t.Fatal("Heirloom's own row moved")
	}

	// a source Heirloom has not: Live's values
	targetAs(gp, campPariah, 100400, gs)
	c.say(gs, "/clone")
	gmReplies(t, gs)
	var pid int64
	m.liveDB.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&pid)
	hp := m.mirrorOf(t, pid)
	if hp == 0 || npcRowMap(t, m.hlDB, hp)["npc_level"].(int64) != 63 {
		t.Fatalf("the pariah's copy: %d %v", hp, npcRowMap(t, m.hlDB, hp))
	}

	// /delete the wolf: its copy goes, from Heirloom's world too
	targetAs(gp, entity.EntityID(nid), 100200, gs)
	c.say(gs, "/delete")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Its Heirloom copy (row %d) deleted too.", hid)) {
		t.Fatalf("/delete: %q", l)
	}
	if npcRowMap(t, m.hlDB, hid) != nil || count(t, m.hlDB, `SELECT COUNT(*) FROM npcwander WHERE npcid = ?`, hid) != 0 ||
		m.mirrorOf(t, nid) != 0 {
		t.Fatal("Heirloom's copy or its wander or mapping left")
	}
	m.hl.h.applyNPCMirrorInbox()
	if _, ok := m.hl.gw.NPCStateOf(entity.EntityID(hid)); ok {
		t.Fatal("the deleted copy in Heirloom's world")
	}
}

// Heirloom's database unavailable (a wrong path, the file gone): jibrish's
// /place on Live is made all the same, and he is told it is not mirrored.
func TestNPCMirrorFailureNeverFailsTheCommand(t *testing.T) {
	m := newMirrorWorlds(t)
	SetNPCMirror([]string{"jibrish"}, filepath.Join(t.TempDir(), "no-such.db"))
	gs, gp := m.gs, m.gp
	stand(m.live, gp, 25500, 51, 15500, 0)
	before := count(t, m.liveDB, `SELECT COUNT(*) FROM npcs`)
	m.live.say(gs, "/place wolf")
	l := gmReplies(t, gs)
	if !hasLine(l, "Placed wolf as row") || !hasLine(l, "Not mirrored to Heirloom:") {
		t.Fatalf("/place: %q", l)
	}
	if n := count(t, m.liveDB, `SELECT COUNT(*) FROM npcs`); n != before+1 {
		t.Fatalf("%d Live rows, %d before", n, before)
	}
}
