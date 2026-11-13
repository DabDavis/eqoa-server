package handlers

import (
	"fmt"
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

func wanderRadius(t *testing.T, db *database.SQLiteDB, id int64) (float64, bool) {
	t.Helper()
	var r float64
	err := db.QueryRow(`SELECT radius FROM npcwander WHERE npcid = ?`, id).Scan(&r)
	return r, err == nil
}

// /move sets the target's row's spawn point, facing and world to the GM's,
// in the world at once, audited, with a wander if asked (roamer 300, roamer
// <r>, wander <r>, stay); /move undo puts the last back exactly, wander too.
// /clone roamer and /place "<name>" roamer give the copy a roamer's wander.
// The controls: a player's /move is an unknown command, "/t move" a tell;
// a bad radius changes nothing.
func TestGMMoveAndRoamers(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	if _, err := db.Exec(`INSERT INTO npcwander (npcid, radius, note, source) VALUES (?, 25, 'x', 'disperse')`, campWolfNear); err != nil {
		t.Fatal(err)
	}
	if _, err := c.gw.LoadRowWander(db); err != nil {
		t.Fatal(err)
	}
	before := npcRowMap(t, db, campWolfNear)

	// a player's: nothing
	pp := c.gw.SessionPlayer(ps)
	targetAs(gp, campWolfNear, 100101, gs, ps)
	pp.TargetObjectID = 100101
	c.say(ps, "/move")
	c.say(ps, "/t move")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/move"`) {
		t.Fatalf("a player's /move: %q", l)
	}
	if fmt.Sprint(npcRowMap(t, db, campWolfNear)) != fmt.Sprint(before) {
		t.Fatal("a player's /move moved")
	}

	// a bad radius
	stand(c, gp, 25500, 51, 15500, math.Pi/2)
	c.say(gs, "/move roamer 99999")
	if l := gmReplies(t, gs); !hasLine(l, "Usage: /move") {
		t.Fatalf("a bad radius: %q", l)
	}

	// /move roamer
	c.say(gs, "/move roamer")
	l := gmReplies(t, gs)
	if !hasLine(l, fmt.Sprintf("Moved wolf (row %d) here (from 25080, 50, 15080).", campWolfNear)) || !hasLine(l, "A roamer, 300 from its spawn") {
		t.Fatalf("/move roamer: %q", l)
	}
	row := npcRowMap(t, db, campWolfNear)
	if row["x_coord"].(float64) != 25500 || row["z_coord"].(float64) != 15500 || row["y_coord"].(float64) != 51 || row["facing"].(int64) != 64 {
		t.Fatalf("the moved row: %v", row)
	}
	if r, ok := wanderRadius(t, db, campWolfNear); !ok || r != 300 {
		t.Fatalf("the roamer's wander: %v %v", r, ok)
	}
	if st, ok := c.gw.NPCStateOf(campWolfNear); !ok || st.SpawnX != 25500 || st.SpawnZ != 15500 || st.X != 25500 {
		t.Fatalf("the moved NPC in the world: %+v", st)
	}
	if n := auditLines(t, db, "npcs", "update"); n != 1 {
		t.Fatalf("%d npcs updates audited", n)
	}
	if n := auditLines(t, db, "npcwander", "update"); n != 1 {
		t.Fatalf("%d npcwander updates audited", n)
	}

	// /move undo: the row and its wander as they were
	c.say(gs, "/move undo")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Put wolf (row %d) back where it stood, with its wander as it was.", campWolfNear)) {
		t.Fatalf("/move undo: %q", l)
	}
	if fmt.Sprint(npcRowMap(t, db, campWolfNear)) != fmt.Sprint(before) {
		t.Fatalf("put back:\n%v\nwas\n%v", npcRowMap(t, db, campWolfNear), before)
	}
	if r, ok := wanderRadius(t, db, campWolfNear); !ok || r != 25 {
		t.Fatalf("the wander put back: %v %v", r, ok)
	}
	if st, _ := c.gw.NPCStateOf(campWolfNear); st.SpawnX != 25080 {
		t.Fatalf("put back in the world: %+v", st)
	}
	c.say(gs, "/move undo")
	if l := gmReplies(t, gs); !hasLine(l, "You have moved nothing that is not back already.") {
		t.Fatalf("a second undo: %q", l)
	}

	// a PS2's "/t move stay", in another world: its own wander taken away
	gp.World = 1
	c.say(gs, "/t move stay")
	if l := gmReplies(t, gs); !hasLine(l, "Its own wander taken away") {
		t.Fatalf("/t move stay: %q", l)
	}
	if _, ok := wanderRadius(t, db, campWolfNear); ok {
		t.Fatal("stay left the wander")
	}
	if row := npcRowMap(t, db, campWolfNear); row["world"].(int64) != 1 {
		t.Fatalf("the world not moved: %v", row)
	}
	c.say(gs, "/move undo")
	if _, ok := wanderRadius(t, db, campWolfNear); !ok || fmt.Sprint(npcRowMap(t, db, campWolfNear)) != fmt.Sprint(before) {
		t.Fatal("stay's undo")
	}
	gp.World = 0
	gmReplies(t, gs)

	// /move wander 40 then moved again by hand: the undo refuses
	c.say(gs, "/move wander 40")
	if r, _ := wanderRadius(t, db, campWolfNear); r != 40 {
		t.Fatalf("wander 40: %v", r)
	}
	db.Exec(`UPDATE npcs SET x_coord = 1 WHERE id = ?`, campWolfNear)
	gmReplies(t, gs)
	c.say(gs, "/move undo")
	if l := gmReplies(t, gs); !hasLine(l, "is moved or changed since") {
		t.Fatalf("an undo over a later change: %q", l)
	}

	// /clone roamer 150
	targetAs(gp, campPariah, 100200, gs)
	c.say(gs, "/clone roamer 150")
	var nid int64
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Cloned pariah of Xrag as row %d.", nid)) || !hasLine(l, "A roamer, 150 from its spawn") {
		t.Fatalf("/clone roamer 150: %q", l)
	}
	if r, ok := wanderRadius(t, db, nid); !ok || r != 150 {
		t.Fatalf("the clone's wander: %v %v", r, ok)
	}

	// /place of a name only in another world, no quotes, roamer
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (120, 'pusling lacer', 1, 1000, 50, 1000, 0, ?, 1.0, 20, 0, '0', 7)`, campCreature); err != nil {
		t.Fatal(err)
	}
	c.gw.LoadNPCs(database.NewNPCStore(db))
	c.say(gs, "/place pusling lacer roamer")
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Placed pusling lacer as row %d (a copy of row 120).", nid)) || !hasLine(l, "A roamer, 300") {
		t.Fatalf("/place pusling lacer roamer: %q", l)
	}
	if row := npcRowMap(t, db, nid); row["world"].(int64) != 0 || row["x_coord"].(float64) != 25500 {
		t.Fatalf("the placed row: %v", row)
	}
	if r, _ := wanderRadius(t, db, nid); r != 300 {
		t.Fatalf("the placed roamer's wander: %v", r)
	}
	// quoted, with a wander; and a row's name not in the world now
	c.say(gs, `/place "Pusling Lacer" wander 30`)
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if r, _ := wanderRadius(t, db, nid); r != 30 {
		t.Fatalf("quoted /place's wander: %v %q", r, gmReplies(t, gs))
	}
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (130, 'a hidden thing', 2, 1000, 50, 1000, 0, ?, 1.0, 20, 0, '0', 7)`, campCreature); err != nil {
		t.Fatal(err)
	}
	gmReplies(t, gs)
	c.say(gs, "/place a hidden thing")
	if l := gmReplies(t, gs); !hasLine(l, "Placed a hidden thing as row") {
		t.Fatalf("a row's name not in the world: %q", l)
	}
	if _, ok := c.gw.NPCStateOf(entity.EntityID(nid)); !ok {
		t.Fatal("the placed NPC is not in the world")
	}
}

func TestNPCRowTellMove(t *testing.T) {
	for _, c := range []struct {
		name, body string
		ok         bool
	}{
		{"move", "", true}, {"move", "undo", true}, {"move", "roamer", true}, {"move", "roamer 200", true},
		{"Move", "Wander 30", true}, {"move", "stay", true}, {"clone", "roamer", true}, {"clone", "undo", false},
		{"move", "hello there", false}, {"delete", "roamer", false}, {"move", "wander", false},
	} {
		if _, _, ok := npcRowTell(c.name, c.body); ok != c.ok {
			t.Errorf("npcRowTell(%q, %q) = %v", c.name, c.body, ok)
		}
	}
}

func TestRoamerTellWords(t *testing.T) {
	for _, c := range []struct {
		body string
		ok   bool
	}{{"", true}, {"500", true}, {"stay", true}, {"fast", false}} {
		if _, _, ok := npcRowTell("roamer", c.body); ok != c.ok {
			t.Errorf("/t roamer %q: ok %v, want %v", c.body, ok, c.ok)
		}
	}
}

// /roamer (the owner, 2026-11-12): the target roams where it stands, its spawn
// point kept; /roamer <r>; /roamer stay; a player's is nothing.
func TestGMRoamer(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	before := npcRowMap(t, db, campWolfNear)
	pp := c.gw.SessionPlayer(ps)
	targetAs(gp, campWolfNear, 100101, gs, ps)
	pp.TargetObjectID = 100101
	c.say(ps, "/roamer")
	if _, ok := wanderRadius(t, db, campWolfNear); ok {
		t.Fatal("a player's /roamer set a wander")
	}
	c.say(gs, "/roamer")
	if l := gmReplies(t, gs); !hasLine(l, "A roamer, 300 from its spawn") {
		t.Fatalf("/roamer: %q", l)
	}
	if r, ok := wanderRadius(t, db, campWolfNear); !ok || r != 300 {
		t.Fatalf("/roamer's wander: %v %v", r, ok)
	}
	if fmt.Sprint(npcRowMap(t, db, campWolfNear)) != fmt.Sprint(before) {
		t.Fatal("/roamer moved the row")
	}
	c.say(gs, "/roamer 120")
	if r, ok := wanderRadius(t, db, campWolfNear); !ok || r != 120 {
		t.Fatalf("/roamer 120: %v %v", r, ok)
	}
	c.say(gs, "/roamer stay")
	if _, ok := wanderRadius(t, db, campWolfNear); ok {
		t.Fatal("/roamer stay left a wander")
	}
	c.say(gs, "/roamer fast")
	if l := gmReplies(t, gs); !hasLine(l, "roamer:") {
		t.Fatalf("/roamer fast: %q", l)
	}
}

// A GM's /size on a creature is kept in its row, audited, so a restart keeps
// it (the owner, 2026-11-13: "size may not being preserved across restarts").
func TestGMSizeKeptInTheRow(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	targetAs(gp, campWolfNear, 100101, gs, ps)
	before := npcRowMap(t, db, campWolfNear)["size"]
	c.say(gs, "/size 2.5")
	l := gmReplies(t, gs)
	if !hasLine(l, "keeps size") {
		t.Fatalf("/size: %q", l)
	}
	got := npcRowMap(t, db, campWolfNear)["size"].(float64)
	if math.Abs(got-2.5*before.(float64)) > 0.01 {
		t.Fatalf("the row's size %v, want 2.5x %v", got, before)
	}
	if n := auditLines(t, db, "npcs", "update"); n != 1 {
		t.Fatalf("%d npcs updates audited", n)
	}
}
