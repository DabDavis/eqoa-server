package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The owner, 2026-11-13: GMs refused /place filth squid in the Plane of
// Disease (the name is in npcspawnonly: quest 101289's step 4 copies its
// original). /place of a spawn-only name now makes a row that stands (its
// npcstands row, audited), told so; the name's other rows stay out of the
// world and a quest's spawn of the name is still a copy of the spawn-only
// original. The row stands over a restart; /clone of it copies the exemption;
// /delete takes it with the row and /delete undo puts it back. The control:
// /place of an ordinary name writes no npcstands row.
func TestPlaceSpawnOnlyNameStands(t *testing.T) {
	c, db, _, gs, gp, _ := campWorld(t)
	for _, id := range []int{120, 121} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, 'filth squid', 0, 25050, 50, 15050, 0, ?, 1.0, 53, 0, '0', 7)`, id, campCreature); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO npcspawnonly (name, note, source) VALUES ('filth squid', 'quest 101289 step 4', 'questspawn')`); err != nil {
		t.Fatal(err)
	}
	load := func() {
		t.Helper()
		c.gw.LoadNPCs(database.NewNPCStore(db))
		if _, _, err := c.gw.LoadSpawnOnly(db); err != nil {
			t.Fatal(err)
		}
	}
	load()
	spawnOnly := func(id int64) bool {
		t.Helper()
		st, ok := c.gw.NPCStateOf(entity.EntityID(id))
		if !ok {
			t.Fatalf("row %d not in the world", id)
		}
		return st.SpawnOnly
	}
	if !spawnOnly(120) || !spawnOnly(121) {
		t.Fatal("the spawn-only rows stand in the world before any /place")
	}

	stand(c, gp, 25600, 52, 15600, 0)
	c.say(gs, "/place filth squid")
	var nid int64
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&nid)
	l := gmReplies(t, gs)
	if !hasLine(l, fmt.Sprintf("Placed filth squid as row %d (a copy of row 120), with its npcstands.", nid)) ||
		!hasLine(l, "It is a standing copy of a quest's or trigger's spawn") {
		t.Fatalf("/place of a spawn-only name: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcstands WHERE npc_row = ?`, nid); n != 1 {
		t.Fatalf("%d npcstands rows for the placed row", n)
	}
	if n := auditLines(t, db, "npcstands", "insert"); n != 1 {
		t.Fatalf("%d npcstands inserts audited", n)
	}
	st, ok := c.gw.NPCStateOf(entity.EntityID(nid))
	if !ok || st.SpawnOnly || !st.Stands || st.X != 25600 {
		t.Fatalf("the placed row in the world: %+v %v", st, ok)
	}
	if !spawnOnly(120) || !spawnOnly(121) {
		t.Fatal("the name's other rows came into the world")
	}

	// a quest's spawn of the name: a copy of the spawn-only original, in the world
	cid, _, ok := c.gw.SpawnNPCCopy("filth squid", 25700, 50, 15700, 0, 0)
	if !ok {
		t.Fatal("no quest spawn")
	}
	if cs, ok := c.gw.NPCStateOf(cid); !ok || cs.SpawnOnly || !cs.Temporary {
		t.Fatalf("the quest's spawn: %+v %v", cs, ok)
	}
	if got := c.gw.NPCs[cid].CopyOf; got != 120 {
		t.Fatalf("the quest's spawn is a copy of %d, not the original 120", got)
	}
	if !c.gw.SpawnOnlyNamed("filth squid") {
		t.Fatal("the name is no longer spawn-only")
	}

	// a restart: the row stands still
	load()
	if spawnOnly(nid) || !spawnOnly(120) {
		t.Fatal("after a restart the exemption is not applied")
	}

	// /clone of the standing row: the copy stands too
	targetAs(gp, entity.EntityID(nid), 100300, gs)
	c.say(gs, "/clone")
	var cl int64
	db.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&cl)
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Cloned filth squid as row %d, with its npcstands.", cl)) ||
		!hasLine(l, "standing copy") {
		t.Fatalf("/clone: %q", l)
	}
	if spawnOnly(cl) {
		t.Fatal("the clone stands out of the world")
	}

	// /delete takes the exemption with the row; undo puts both back
	targetAs(gp, entity.EntityID(nid), 100300, gs)
	c.say(gs, "/delete")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Deleted filth squid (row %d), with its npcstands.", nid)) {
		t.Fatalf("/delete: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcstands WHERE npc_row = ?`, nid); n != 0 {
		t.Fatal("the exemption outlived its row")
	}
	if _, ok := c.gw.NPCStateOf(entity.EntityID(nid)); ok {
		t.Fatal("the deleted row is in the world")
	}
	c.say(gs, "/delete undo")
	if l := gmReplies(t, gs); !hasLine(l, fmt.Sprintf("Put back filth squid (row %d)", nid)) || !hasLine(l, "npcstands") {
		t.Fatalf("/delete undo: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcstands WHERE npc_row = ?`, nid); n != 1 {
		t.Fatal("undo did not put the exemption back")
	}
	if spawnOnly(nid) || !spawnOnly(121) {
		t.Fatal("after undo the row is not standing (or the others are)")
	}

	// the control: an ordinary name's /place writes none
	before := count(t, db, `SELECT COUNT(*) FROM npcstands`)
	c.say(gs, "/place wolf")
	if l := gmReplies(t, gs); hasLine(l, "standing copy") {
		t.Fatalf("an ordinary /place said it stands: %q", l)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM npcstands`); n != before {
		t.Fatal("an ordinary /place wrote an npcstands row")
	}
}
