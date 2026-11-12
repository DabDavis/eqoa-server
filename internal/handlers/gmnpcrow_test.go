package handlers

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// targetAs makes id the entity the sessions' channel 1 holds as objectID, and
// the GM's target that object.
func targetAs(gp *entity.Player, id entity.EntityID, objectID uint32, ss ...*session.Session) {
	for _, s := range ss {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = id, objectID
	}
	gp.TargetObjectID = objectID
}

func npcRowMap(t *testing.T, db *database.SQLiteDB, id int64) map[string]any {
	t.Helper()
	rs, err := db.Query(`SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	if !rs.Next() {
		return nil
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rs.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	for i, c := range cols {
		m[c] = vals[i]
	}
	return m
}

func count(t *testing.T, db *database.SQLiteDB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// /clone copies the GM's target's row where the GM stands, facing as it
// faces, with its respawn time and title, in the database and the world at
// once; /delete deletes a row with its extras, out of the world at once;
// /delete undo puts it back exactly. Every row's change has its gm_audit line.
// The controls: a player's /clone and /delete do nothing (an unknown command;
// "/t delete" a tell); a player, a GM's spawn of no row and a row a quest
// step names are refused, nothing changed.
func TestGMDeleteAndClone(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	if _, err := db.Exec(`INSERT INTO npcrespawn (npc_row, seconds) VALUES (?, 100)`, campWolfNear); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO npctitle (npcid, title) VALUES (?, 'the Grey')`, campWolfNear); err != nil {
		t.Fatal(err)
	}
	if _, err := c.gw.LoadRespawnRules(db); err != nil {
		t.Fatal(err)
	}
	stand(c, gp, 25500, 51, 15500, math.Pi/2)
	targetAs(gp, campWolfNear, 100101, gs, ps)

	// a player's: nothing
	c.say(ps, "/clone")
	c.say(ps, "#delete")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/clone"`) || !hasLine(l, `Unrecognized command "#delete"`) {
		t.Fatalf("a player's: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcs`); n != 5 {
		t.Fatalf("%d rows after a player's", n)
	}

	// /clone
	c.say(gs, "/clone")
	l := gmReplies(t, gs)
	var nid int64
	if err := db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid); err != nil || nid <= campDummy {
		t.Fatalf("no new row: %d %v", nid, err)
	}
	if !hasLine(l, fmt.Sprintf("Cloned wolf as row %d, with its npcrespawn, npctitle.", nid)) {
		t.Fatalf("/clone: %q", l)
	}
	row := npcRowMap(t, db, nid)
	if row["x_coord"].(float64) != 25500 || row["z_coord"].(float64) != 15500 || row["y_coord"].(float64) != 51 ||
		row["facing"].(int64) != 64 || row["npc_level"].(int64) != 12 || row["world"].(int64) != 0 {
		t.Fatalf("the clone's row: %v", row)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcrespawn WHERE npc_row = ? AND seconds = 100`, nid); n != 1 {
		t.Fatal("the respawn not copied")
	}
	st, ok := c.gw.NPCStateOf(entity.EntityID(nid))
	if !ok || st.X != 25500 || st.Z != 15500 || st.Temporary || st.RespawnSec != 100 {
		t.Fatalf("the clone in the world: %+v %v", st, ok)
	}
	if n := auditLines(t, db, "npcs", "insert"); n != 1 {
		t.Fatalf("%d npcs inserts audited", n)
	}

	// /delete the clone, then undo
	before := npcRowMap(t, db, nid)
	targetAs(gp, entity.EntityID(nid), 100200, gs)
	c.say(gs, "#delete")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Deleted wolf (row %d), with its npcrespawn, npctitle.", nid)) {
		t.Fatalf("/delete: %q", l)
	}
	if npcRowMap(t, db, nid) != nil || count(t, db, `SELECT COUNT(*) FROM npcrespawn WHERE npc_row = ?`, nid) != 0 ||
		count(t, db, `SELECT COUNT(*) FROM npctitle WHERE npcid = ?`, nid) != 0 {
		t.Fatal("the row or its extras are left")
	}
	if _, ok := c.gw.NPCStateOf(entity.EntityID(nid)); ok {
		t.Fatal("the deleted NPC is in the world")
	}
	if n := auditLines(t, db, "npcs", "delete"); n != 1 {
		t.Fatalf("%d npcs deletes audited", n)
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Put back wolf (row %d) (deleted by account 1) with npcs, npctitle, npcrespawn.", nid)) {
		t.Fatalf("/delete undo: %q", l)
	}
	if after := npcRowMap(t, db, nid); !reflect.DeepEqual(before, after) {
		t.Fatalf("put back:\n%v\nwas\n%v", after, before)
	}
	if st, ok := c.gw.NPCStateOf(entity.EntityID(nid)); !ok || st.RespawnSec != 100 {
		t.Fatalf("put back in the world: %+v %v", st, ok)
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, "No delete is left to put back.") {
		t.Fatalf("a second undo: %q", l)
	}

	// a PS2's "/t delete": a GM's command, a player's tell
	targetAs(gp, campWolfFar, 100300, gs, ps)
	pp := c.gw.SessionPlayer(ps)
	pp.TargetObjectID = 100300
	c.say(ps, "/t delete")
	if npcRowMap(t, db, campWolfFar) == nil {
		t.Fatal("a player's /t delete deleted")
	}
	c.say(gs, "/t delete")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Deleted wolf (row %d).", campWolfFar)) {
		t.Fatalf("/t delete: %q", l)
	}

	// refusals: a quest step's NPC, a player, a GM's spawn
	if _, err := db.Exec(`INSERT INTO queststeps (questid, step, kind, target, npcid) VALUES (7, 1, 'talk', 'a patroller', ?)`, campPatroller); err != nil {
		t.Fatal(err)
	}
	targetAs(gp, campPatroller, 100400, gs)
	c.say(gs, "/delete")
	if l := gmReplies(t, gs); !hasLine(l, "is not deleted: it is named by quest step 7/1") {
		t.Fatalf("a quest's NPC: %q", l)
	}
	targetAs(gp, pp.ServerID, 100500, gs)
	c.say(gs, "/clone")
	c.say(gs, "/delete")
	if l := gmReplies(t, gs); !hasLine(l, "/clone: Player is a player") || !hasLine(l, "/delete: Player is a player") {
		t.Fatalf("a player: %q", l)
	}
	sid, _, ok := c.gw.SpawnNPCCopy("a patroller", 25000, 50, 15000, 0, 0)
	if !ok {
		t.Fatal("no spawn")
	}
	targetAs(gp, sid, 100600, gs)
	c.say(gs, "/clone")
	if l := gmReplies(t, gs); !hasLine(l, "is a spawn with no row") {
		t.Fatalf("a GM's spawn: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcs`); n != 5 {
		t.Fatalf("%d rows at the end", n)
	}
}

// /place <name> makes a new NPC of the name where the GM stands, facing as it
// faces: a copy of a row of the name, of the most common body and level in the
// GM's world, with its respawn time, in the database and the world at once,
// audited. A sub GM may /place, /clone and /delete as it may /fly (the owner:
// "players with the /fly /speed can use also"), "/t place <name>" too; still
// no other GM command (/goto). The controls: a player's /place is an unknown
// command and "/t place wolf" a tell; a name no NPC has is refused with close
// names; nothing made.
func TestPlaceAndSubGMs(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (110, 'wolf', 0, 41000, 50, 41000, 0, ?, 1.0, 40, 0, '0', 7)`, campCreature); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO npcrespawn (npc_row, seconds) VALUES (?, 300)`, campWolfFar); err != nil {
		t.Fatal(err)
	}
	c.say(ps, `/place "wolf"`)
	c.say(ps, "/t place wolf")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/place"`) {
		t.Fatalf("a player's /place: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcs`); n != 6 {
		t.Fatalf("%d rows after a player's", n)
	}

	stand(c, gp, 25600, 52, 15600, math.Pi)
	c.say(gs, `/place "Wolf"`)
	var nid int64
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Placed wolf as row %d (a copy of row %d), with its npcrespawn.", nid, campWolfFar)) {
		t.Fatalf("/place: %q", l)
	}
	row := npcRowMap(t, db, nid)
	if row["x_coord"].(float64) != 25600 || row["z_coord"].(float64) != 15600 || row["facing"].(int64) != 128 || row["npc_level"].(int64) != 40 {
		t.Fatalf("the placed row: %v", row)
	}
	if st, ok := c.gw.NPCStateOf(entity.EntityID(nid)); !ok || st.X != 25600 || st.RespawnSec != 300 {
		t.Fatalf("the placed NPC in the world: %+v %v", st, ok)
	}
	c.say(gs, "/place grey wolfish beast")
	if l := gmReplies(t, gs); !hasLine(l, `/place: no NPC is called "grey wolfish beast": nothing made.`) {
		t.Fatalf("an unknown name: %q", l)
	}
	c.say(gs, "/place patroller of wolves")
	if l := gmReplies(t, gs); !hasLine(l, "Close: wolf") && !hasLine(l, "a patroller") {
		t.Fatalf("close names: %q", l)
	}

	// the player made a sub GM
	c.h.SetAccessLevels(subs{levels{1: 1, 2: 0}, map[int]bool{2: true}})
	pp := c.gw.SessionPlayer(ps)
	stand(c, pp, 25700, 53, 15700, 0)
	c.say(ps, "/t place pariah of Xrag")
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	if l := gmReplies(t, ps); !hasLine(l, fmt.Sprintf("Placed pariah of Xrag as row %d (a copy of row %d).", nid, campPariah)) {
		t.Fatalf("a sub GM's /t place: %q", l)
	}
	targetAs(pp, entity.EntityID(nid), 100700, ps)
	c.say(ps, "#clone")
	c.say(ps, "/delete")
	c.say(ps, "/goto 1 2 3")
	l := gmReplies(t, ps)
	if !hasLine(l, "Cloned pariah of Xrag as row") || !hasLine(l, fmt.Sprintf("Deleted pariah of Xrag (row %d).", nid)) ||
		!hasLine(l, `Unrecognized command "/goto"`) {
		t.Fatalf("a sub GM's: %q", l)
	}
}

// The owner, 2026-11-12: "/delete undo it will undo last delete and keep going if press again", and "let any gm undo
// anyones delete". Two clones deleted, then /delete undo three times: the last deleted comes back first (told one more
// is left), then the one before (made another GM's: put back all the same, its deleter named), then nothing is left.
func TestGMDeleteUndoKeepsGoing(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	stand(c, gp, 25500, 51, 15500, math.Pi/2)
	var ids []int64
	for i := 0; i < 2; i++ {
		targetAs(gp, campWolfNear, 100101, gs, ps)
		c.say(gs, "/clone")
		gmReplies(t, gs)
		var nid int64
		if err := db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, nid)
	}
	for i, nid := range ids {
		targetAs(gp, entity.EntityID(nid), uint32(100300+i), gs)
		c.say(gs, "/delete")
		gmReplies(t, gs)
		if npcRowMap(t, db, nid) != nil {
			t.Fatalf("row %d not deleted", nid)
		}
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Put back wolf (row %d) (deleted by account 1) with npcs. /delete undo again puts back the one before (1 more).", ids[1])) {
		t.Fatalf("the first undo: %q", l)
	}
	if npcRowMap(t, db, ids[1]) == nil || npcRowMap(t, db, ids[0]) != nil {
		t.Fatal("the first undo did not put back the last deleted alone")
	}
	// the one before was another GM's (the owner: "let any gm undo anyones delete")
	if _, err := db.Exec(`UPDATE gm_audit SET accountid = 99, username = 'forestwood' WHERE action = 'delete' AND reverted_by IS NULL`); err != nil {
		t.Fatal(err)
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Put back wolf (row %d) (deleted by forestwood) with npcs.", ids[0])) {
		t.Fatalf("the second undo: %q", l)
	}
	if npcRowMap(t, db, ids[0]) == nil {
		t.Fatal("the second undo did not put back the one before")
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, "No delete is left to put back.") {
		t.Fatalf("a third undo: %q", l)
	}
}
