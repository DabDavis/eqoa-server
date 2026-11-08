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
// marks four spots on Xrag's rock (where the four Xrag rows nearest its first
// child of Xrag stand) and makes a camp of children and pariahs of Xrag there.
// Each spawn is a copy of the row of its kind nearest the first mark (its level
// and body), stands, and pops both kinds in its returns, each with its kind's
// row's figures; the camps read again over every NPC of the server in well
// under a second; /camp undo leaves the database's npcs as they were. Skipped
// without a database, or with no child of Xrag in its world 0.
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
	var ax, az float64
	if err := db.QueryRow(`SELECT x_coord, z_coord FROM npcs WHERE lower(npc_name) = 'child of xrag' AND world = 0 ORDER BY id LIMIT 1`).
		Scan(&ax, &az); err != nil {
		t.Skip("no child of Xrag in world 0")
	}
	q, err := db.Query(`SELECT x_coord, y_coord, z_coord FROM npcs WHERE lower(npc_name) IN ('child of xrag', 'pariah of xrag') AND world = 0
		ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?), id LIMIT 4`, ax, ax, az, az)
	if err != nil {
		t.Fatal(err)
	}
	var spots [][4]float32
	for q.Next() {
		var x, y, z float64
		if err := q.Scan(&x, &y, &z); err != nil {
			t.Fatal(err)
		}
		spots = append(spots, [4]float32{float32(x), float32(y), float32(z), float32(len(spots)) * 1.5})
	}
	q.Close()
	if len(spots) < 4 {
		t.Skipf("only %d Xrag rows in world 0", len(spots))
	}
	// each kind's row nearest the first mark: what its spawns copy
	want := map[string][2]int64{} // key -> level, body
	for _, name := range []string{"child of xrag", "pariah of xrag"} {
		var level, model int64
		if err := db.QueryRow(`SELECT npc_level, modelid FROM npcs WHERE lower(npc_name) = ? AND world = 0
			ORDER BY (x_coord - ?) * (x_coord - ?) + (z_coord - ?) * (z_coord - ?), id LIMIT 1`,
			name, spots[0][0], spots[0][0], spots[0][2], spots[0][2]).Scan(&level, &model); err != nil {
			t.Skipf("no %s in world 0", name)
		}
		want[name] = [2]int64{level, model}
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
	gs, gp := c.join(900001, "Gm", spots[0][0], spots[0][2], 0)
	gs.SetAccountID(1)
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
		w, ok := want[database.NPCClassKey(rec.NPCName)]
		if !ok || int64(rec.NPCLevel) != w[0] || int64(rec.ModelID) != w[1] || rec.World != 0 {
			t.Fatalf("spawn %d: %s level %d body %d in world %d, want a copy of %v", i, rec.NPCName, rec.NPCLevel, rec.ModelID, rec.World, w)
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
		key := database.NPCClassKey(st.Name)
		w, ok := want[key]
		if st.Dead || st.X != spots[0][0] || st.Z != spots[0][2] || !ok || int64(st.Level) != w[0] || st.ModelID != w[1] {
			t.Fatalf("back: %+v", st)
		}
		seen[key] = true
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
