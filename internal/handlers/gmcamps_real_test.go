package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Petition 1293's camp on a real server (EQOA_REAL_DB; a copy is opened): a GM
// marks four spots on Xrag's rock and makes a camp of children and pariahs of
// Xrag there. Each spawn is a copy of a row of its kind from the rock itself
// (the nearest of its name: the rock's own levels, 60 to 64), stands, and
// pops both kinds in its returns; the camps read again over every NPC of the
// server in well under a second; /camp undo leaves the database's npcs as
// they were. Skipped without a database, or with no Xrag's children in it.
func TestRealACampOnXragsRock(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "camps.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var children int
	db.QueryRow(`SELECT COUNT(*) FROM npcs WHERE lower(npc_name) = 'child of xrag' AND world = 0`).Scan(&children)
	if children == 0 {
		t.Skip("no child of Xrag in world 0")
	}
	var before int
	db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&before)

	c := newChatWorld(t)
	c.h.SetAccessLevels(levels{1: 1})
	c.h.SetCampStore(db)
	c.gw.LoadNPCs(database.NewNPCStore(db))
	if _, err := c.gw.LoadMoveRules(db); err != nil {
		t.Fatal(err)
	}
	c.gw.SetAggro(false)
	gs, gp := c.join(900001, "Gm", 15424, 27039, 0)
	gs.SetAccountID(1)
	spots := [][4]float32{{15424.2, 15.625, 27039.4, 0.5}, {15446.4, 7.4375, 27031.4, 2}, {15430.9, 7.4375, 27052.5, 4}, {15430.5, 15.2812, 27016.7, 6}}
	markAt(t, c, gs, gp, spots...)
	start := time.Now()
	c.say(gs, "/camp child of Xrag, pariah of Xrag")
	took := time.Since(start)
	l := gmReplies(t, gs)
	if !hasLine(l, "4 static spawns, each popping child of Xrag or pariah of Xrag at random every time it comes back") {
		t.Fatalf("/camp: %q", l)
	}
	t.Logf("/camp over %d NPCs took %v: %s", before, took, strings.Join(l, " | "))
	rows, err := db.CampSpawns()
	if err != nil || len(rows) != 4 {
		t.Fatalf("campspawns: %+v %v", rows, err)
	}
	store := database.NewNPCStore(db)
	var ids []entity.EntityID
	for i, r := range rows {
		rec, err := store.GetNPC(r.NPCID)
		if err != nil {
			t.Fatal(err)
		}
		k := database.NPCClassKey(rec.NPCName)
		if k != "child of xrag" && k != "pariah of xrag" || rec.NPCLevel < 60 || rec.NPCLevel > 64 || rec.World != 0 {
			t.Fatalf("spawn %d: %s level %d in world %d", i, rec.NPCName, rec.NPCLevel, rec.World)
		}
		if st, ok := c.gw.NPCStateOf(entity.EntityID(r.NPCID)); !ok || !strings.Contains(st.Movement, "camp's static spawn") {
			t.Fatalf("spawn %d in the world: %+v %v", i, st, ok)
		}
		ids = append(ids, entity.EntityID(r.NPCID))
	}
	start = time.Now()
	if n, err := c.gw.LoadCampSpawns(db); err != nil || n != 4 {
		t.Fatalf("read again: %d %v", n, err)
	}
	if read := time.Since(start); read > time.Second {
		t.Errorf("reading the camps again over every NPC took %v", read)
	} else {
		t.Logf("the camps read again in %v", read)
	}

	c.gw.SetCorpseTimes(time.Millisecond, time.Millisecond)
	seen := map[string]bool{}
	for i := 0; i < 60 && len(seen) < 2; i++ {
		c.gw.KillNPCNow(ids[0])
		c.gw.Tick(time.Now().Add(time.Second))
		st, _ := c.gw.NPCStateOf(ids[0])
		if st.Dead || st.X != spots[0][0] || st.Z != spots[0][2] || st.Level < 60 || st.Level > 64 {
			t.Fatalf("back: %+v", st)
		}
		seen[database.NPCClassKey(st.Name)] = true
	}
	if !seen["child of xrag"] || !seen["pariah of xrag"] {
		t.Fatalf("it popped only %v", seen)
	}

	c.say(gs, "/camp undo")
	if l := gmReplies(t, gs); !hasLine(l, "taken away: its 4 spawns are gone") {
		t.Fatalf("/camp undo: %q", l)
	}
	var after int
	db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&after)
	if after != before {
		t.Fatalf("npcs: %d rows before, %d after the undo", before, after)
	}
}
