package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A GM's marks, camps and paths in the game (gmcamps.go): /mark keeps where
// the GM stands, by account, through a restart; /camp makes a static spawn at
// each mark, in the database and the world at once, popping its kinds; /path
// walks the GM's target through them; a GM's petition carries them. Every
// change has its gm_audit line. A player's are an unknown command and change
// nothing (each test's control).

// The NPC rows of a camp world, all in world 0 near the GM at (25000, 15000).
const (
	campWolfNear  = 101 // the wolf nearest the GM: the one a camp of wolves copies
	campWolfFar   = 102 // a wolf far off, of another level
	campPariah    = 103 // a pariah of Xrag
	campPatroller = 104 // an NPC a path is given to
	campDummy     = 105 // a training dummy: no camp of it
)

const campCreature, campPariahBody = 0x1234, 0x5678

// campWorld is gmWorld (the GM Gm, account 1, at (25000, 50, 15000); the
// player Player, account 2, beside it) on a database at path holding the rows
// above, loaded into the world, with aggro off and the camp tools on it.
func campWorld(t *testing.T) (*chatWorld, *database.SQLiteDB, string, *session.Session, *entity.Player, *session.Session) {
	t.Helper()
	c, gs, gp, ps, _ := gmWorld(t)
	path := filepath.Join(t.TempDir(), "camps.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, r := range []struct {
		id    int
		name  string
		x, z  float64
		model int64
		level int
	}{
		{campWolfNear, "wolf", 25080, 15080, campCreature, 12},
		{campWolfFar, "wolf", 40000, 40000, campCreature, 40},
		{campPariah, "pariah of Xrag", 25200, 15200, campPariahBody, 63},
		{campPatroller, "a patroller", 25100, 15000, campCreature, 30},
		{campDummy, "Training Dummy Lv50", 25300, 15300, campCreature, 50},
	} {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, 0, ?, 50, ?, 0, ?, 1.0, ?, 0, '0', 7)`, r.id, r.name, r.x, r.z, r.model, r.level); err != nil {
			t.Fatal(err)
		}
	}
	c.gw.LoadNPCs(database.NewNPCStore(db))
	c.gw.SetAggro(false)
	c.h.SetCampStore(db)
	return c, db, path, gs, gp, ps
}

// stand moves p to (x, y, z) facing f, as its client's reports do.
func stand(c *chatWorld, p *entity.Player, x, y, z, f float32) {
	c.gw.UpdatePosition(p.ServerID, x, y, z, f)
}

// auditLines is how many gm_audit lines a table's action has.
func auditLines(t *testing.T, db *database.SQLiteDB, table, action string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = ? AND action = ?`, table, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// /mark keeps where the GM stands (world, x, y, z, facing) with its note, as
// its account's next mark, said as "Mark n: (x, y, z) in world w"; /marks
// lists them, /marks undo drops the last, /marks clear all. They are the
// account's, in the database: another character of it, and the server
// started again on the same file, have them. Each is a row change in
// gm_audit. The control: a player's /mark and #mark keep nothing; with no
// camp report open they are told to open one (campfix.go: a player's /mark
// is a camp report's mark, board entry 513).
func TestMarksAreAGMsAndKept(t *testing.T) {
	c, db, path, gs, gp, ps := campWorld(t)
	stand(c, gp, 15023.3, 16.8, 27282.1, 1.5)
	gp.World = 0
	c.say(gs, "/mark the rock")
	if l := gmReplies(t, gs); !hasLine(l, "Mark 1: (15023.3, 16.8, 27282.1) in world 0 - the rock") {
		t.Fatalf("the first mark: %q", l)
	}
	stand(c, gp, 15040, 17, 27290, -0.5)
	c.say(gs, "#mark")
	stand(c, gp, 15060, 18, 27300, 3)
	c.say(gs, "/mark")
	if l := gmReplies(t, gs); !hasLine(l, "Mark 2: (15040.0, 17.0, 27290.0) in world 0") || !hasLine(l, "Mark 3: (15060.0, 18.0, 27300.0) in world 0") {
		t.Fatalf("marks 2 and 3: %q", l)
	}
	c.say(gs, "/marks")
	if l := gmReplies(t, gs); !hasLine(l, "Your marks (3)") || !hasLine(l, "Mark 1: (15023.3, 16.8, 27282.1) in world 0 - the rock, facing 1.50") ||
		!hasLine(l, "Mark 2: (15040.0, 17.0, 27290.0) in world 0, facing -0.50") {
		t.Fatalf("/marks: %q", l)
	}

	// the control: a player's
	ps.Queue().GetPending()
	c.say(ps, "/mark mine")
	c.say(ps, "#mark mine")
	if l := gmReplies(t, ps); len(l) != 2 || l[0] != campfixLineOpenFirst || l[1] != campfixLineOpenFirst {
		t.Fatalf("a player's /mark: %q", l)
	}
	if m, _ := db.GMMarks(2); len(m) != 0 {
		t.Fatalf("a player's /mark kept %+v", m)
	}

	// undo drops the last
	c.say(gs, "/marks undo")
	if l := gmReplies(t, gs); !hasLine(l, "Mark 3 dropped: (15060.0, 18.0, 27300.0) in world 0. 2 left.") {
		t.Fatalf("/marks undo: %q", l)
	}
	if n := auditLines(t, db, "gmmarks", "insert"); n != 3 {
		t.Fatalf("%d gmmarks inserts audited", n)
	}
	if n := auditLines(t, db, "gmmarks", "delete"); n != 1 {
		t.Fatalf("%d gmmarks deletes audited", n)
	}

	// a restart: the database opened again, a new handler, another character of the account
	db.Close()
	db2, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	c2 := newChatWorld(t)
	c2.h.SetAccessLevels(levels{1: 1})
	c2.h.SetCampStore(db2)
	alt, _ := c2.join(9, "Alt", 100, 100, 0)
	alt.SetAccountID(1)
	c2.say(alt, "/marks")
	if l := gmReplies(t, alt); !hasLine(l, "Your marks (2)") || !hasLine(l, "Mark 1: (15023.3, 16.8, 27282.1) in world 0 - the rock") || hasLine(l, "Mark 3") {
		t.Fatalf("after a restart: %q", l)
	}
	m, err := db2.GMMarks(1)
	if err != nil || len(m) != 2 || m[0].CharName != "Gm" || math.Abs(m[0].Facing-1.5) > 1e-6 || m[1].World != 0 {
		t.Fatalf("the rows: %+v %v", m, err)
	}
	c2.say(alt, "/marks clear")
	if l := gmReplies(t, alt); !hasLine(l, "2 marks cleared.") {
		t.Fatalf("/marks clear: %q", l)
	}
	if m, _ := db2.GMMarks(1); len(m) != 0 {
		t.Fatalf("after /marks clear: %+v", m)
	}
}

// markAt stands the GM at each place in turn and marks it.
func markAt(t *testing.T, c *chatWorld, gs *session.Session, gp *entity.Player, places ...[4]float32) {
	t.Helper()
	for _, p := range places {
		stand(c, gp, p[0], p[1], p[2], p[3])
		c.say(gs, "/mark")
	}
	gmReplies(t, gs)
}

// /camp makes a static spawn at each mark, at once in the database and the
// world: a row of npcs each, a copy of the row of one of the names nearest the
// first mark (the wolf near, not the one far off: its level and body), on the
// mark facing as the GM did; a row of campspawns each naming both kinds, the
// camp's row, the marks cleared, each row in gm_audit. Each stands though a
// wolf wanders (the control: the wolf it copies does), and pops a wolf or a
// pariah each time it comes back. /camp undo takes it all away, in the world
// too, and gives the marks back. The controls: a player's /camp makes
// nothing; a name of no NPC, a training dummy and marks in two worlds are
// refused, and nothing is made.
func TestCampMakesStaticSpawnsThatPopTheirKinds(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	spots := [][4]float32{{25000, 50, 15000, 0}, {25010, 50.5, 15005, math.Pi / 2}, {25020, 51, 15010, -math.Pi / 2}}
	markAt(t, c, gs, gp, spots...)

	// the refusals first: nothing made
	c.say(ps, "/camp wolf")
	c.say(gs, "/camp nobody at all")
	c.say(gs, "/camp wolf, Training Dummy Lv50")
	if l := gmReplies(t, gs); !hasLine(l, `No NPC is called "nobody at all": nothing made.`) || !hasLine(l, "is a training dummy") {
		t.Fatalf("the refusals: %q", l)
	}
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/camp"`) {
		t.Fatalf("a player's /camp: %q", l)
	}
	if rows, _ := db.CampSpawns(); len(rows) != 0 {
		t.Fatalf("a refused /camp made %+v", rows)
	}

	c.say(gs, "/camp wolf, pariah of Xrag")
	l := gmReplies(t, gs)
	if !hasLine(l, "Camp 1 made in world 0: 3 static spawns, each popping wolf or pariah of Xrag at random every time it comes back.") ||
		!hasLine(l, "Your marks are cleared. /camp undo takes the camp away.") {
		t.Fatalf("/camp: %q", l)
	}
	rows, err := db.CampSpawns()
	if err != nil || len(rows) != 3 {
		t.Fatalf("campspawns: %+v %v", rows, err)
	}
	store := database.NewNPCStore(db)
	var ids []entity.EntityID
	for i, r := range rows {
		if r.Camp != 1 || strings.Join(r.Kinds, "|") != "wolf|pariah of Xrag" {
			t.Fatalf("spawn %d: %+v", i, r)
		}
		rec, err := store.GetNPC(r.NPCID)
		if err != nil {
			t.Fatal(err)
		}
		if rec.XCoord != spots[i][0] || rec.YCoord != spots[i][1] || rec.ZCoord != spots[i][2] || rec.Facing != facingByte(float64(spots[i][3])) || rec.World != 0 || rec.Zone != 7 {
			t.Fatalf("spawn %d's row: %+v", i, rec)
		}
		switch rec.NPCName {
		case "wolf":
			if rec.NPCLevel != 12 || rec.ModelID != campCreature {
				t.Fatalf("a wolf not the near one's copy: %+v", rec)
			}
		case "pariah of Xrag":
			if rec.NPCLevel != 63 || rec.ModelID != campPariahBody {
				t.Fatalf("a pariah not the pariah's copy: %+v", rec)
			}
		default:
			t.Fatalf("spawn %d is %q", i, rec.NPCName)
		}
		if !hasLine(l, fmt.Sprintf("NPC %d: %s (level %d) at mark %d (", r.NPCID, rec.NPCName, rec.NPCLevel, i+1)) {
			t.Fatalf("spawn %d not said: %q", i, l)
		}
		st, ok := c.gw.NPCStateOf(entity.EntityID(r.NPCID))
		if !ok || st.Dead || st.X != spots[i][0] || st.Z != spots[i][2] || !strings.Contains(st.Movement, "camp's static spawn") {
			t.Fatalf("spawn %d in the world: %+v %v", i, st, ok)
		}
		ids = append(ids, entity.EntityID(r.NPCID))
	}
	if facingByte(math.Pi/2) != 64 || facingByte(-math.Pi/2) != 192 || facingByte(0) != 0 {
		t.Fatal("facingByte")
	}
	if m, _ := db.GMMarks(1); len(m) != 0 {
		t.Fatalf("the marks were not cleared: %+v", m)
	}
	for table, want := range map[string]int{"npcs": 3, "campspawns": 3, "gmcamps": 1} {
		if n := auditLines(t, db, table, "insert"); n != want {
			t.Fatalf("%d %s inserts audited, want %d", n, table, want)
		}
	}
	if n := auditLines(t, db, "gmmarks", "delete"); n != 3 {
		t.Fatalf("%d marks' deletes audited", n)
	}

	// they stand; the wolf they copy wanders (the control)
	wolf, _ := c.gw.NPCStateOf(campWolfNear)
	now, wandered := time.Now(), false
	for i := 0; i < 1800; i++ {
		now = now.Add(100 * time.Millisecond)
		c.gw.Tick(now)
		for j, id := range ids {
			if st, _ := c.gw.NPCStateOf(id); st.X != spots[j][0] || st.Z != spots[j][2] {
				t.Fatalf("spawn %d left its mark: at (%v, %v)", j, st.X, st.Z)
			}
		}
		if st, _ := c.gw.NPCStateOf(campWolfNear); st.X != wolf.X || st.Z != wolf.Z {
			wandered = true
		}
	}
	if !wandered {
		t.Fatal("the control: the wolf the camp copies never wandered in three minutes")
	}

	// each time one comes back it pops a wolf or a pariah, on its mark
	c.gw.SetCorpseTimes(time.Millisecond, time.Millisecond)
	seen := map[string]bool{}
	for i := 0; i < 60 && len(seen) < 2; i++ {
		if !c.gw.KillNPCNow(ids[0]) {
			t.Fatal("not killed")
		}
		c.gw.Tick(time.Now().Add(time.Second))
		st, _ := c.gw.NPCStateOf(ids[0])
		if st.Dead || st.X != spots[0][0] || st.Z != spots[0][2] {
			t.Fatalf("back: %+v", st)
		}
		if st.Name == "pariah of Xrag" && st.Level != 63 || st.Name == "wolf" && st.Level != 12 {
			t.Fatalf("back as %s of level %d", st.Name, st.Level)
		}
		seen[st.Name] = true
	}
	if !seen["wolf"] || !seen["pariah of Xrag"] {
		t.Fatalf("in its returns it popped only %v", seen)
	}

	// /camp undo: all of it gone, in the world too, the marks back
	c.say(gs, "/camp undo")
	if l := gmReplies(t, gs); !hasLine(l, "Camp 1 (wolf, pariah of Xrag) taken away: its 3 spawns are gone") ||
		!hasLine(l, "Its 3 marks are yours again") {
		t.Fatalf("/camp undo: %q", l)
	}
	for _, id := range ids {
		if _, ok := c.gw.NPCStateOf(id); ok {
			t.Fatalf("NPC %d is still in the world", id)
		}
		if _, err := store.GetNPC(int(id)); err == nil {
			t.Fatalf("NPC %d's row is still there", id)
		}
	}
	if rows, _ := db.CampSpawns(); len(rows) != 0 {
		t.Fatalf("campspawns after the undo: %+v", rows)
	}
	m, _ := db.GMMarks(1)
	if len(m) != 3 || m[1].X != float64(spots[1][0]) || m[2].Z != float64(spots[2][2]) {
		t.Fatalf("the marks back: %+v", m)
	}
	if n := auditLines(t, db, "npcs", "delete"); n != 3 {
		t.Fatalf("%d npcs deletes audited", n)
	}
	c.say(gs, "/camp undo")
	if l := gmReplies(t, gs); !hasLine(l, "You have made no camp that still stands.") {
		t.Fatalf("a second undo: %q", l)
	}

	// marks in two worlds are refused
	gp.World = 1
	c.say(gs, "/mark")
	gp.World = 0
	gmReplies(t, gs)
	c.say(gs, "/camp wolf")
	if l := gmReplies(t, gs); !hasLine(l, "Mark 4 is in world 1, mark 1 in world 0: a camp is in one world") {
		t.Fatalf("two worlds: %q", l)
	}
}

// /path gives the GM's target (an NPC of a row) a path through the marks, at
// once in npcpaths and the world, as the GM panel's map does: the mode and the
// pace given, the marks cleared, the row in gm_audit; /path clear takes it
// away. The controls: a player's /path gives nothing; with no target, or a
// mark in another world than the NPC's, nothing is given and the marks stay.
func TestPathWalksTheTargetThroughTheMarks(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	n, _ := c.gw.NPCStateOf(campPatroller)
	for _, s := range []*session.Session{gs, ps} {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ID, 100107
	}
	markAt(t, c, gs, gp, [4]float32{25100, 50.04, 15000, 0}, [4]float32{25140, 50, 15000, 0}, [4]float32{25140, 50, 15040, 0})

	gp.TargetObjectID = 0xFFFFFFFF
	c.say(gs, "/path")
	if l := gmReplies(t, gs); !hasLine(l, "/path: target the NPC first") {
		t.Fatalf("no target: %q", l)
	}
	c.say(ps, "/path")
	if l := gmReplies(t, ps); !hasLine(l, `Unrecognized command "/path"`) {
		t.Fatalf("a player's /path: %q", l)
	}
	if _, ok := c.gw.NPCPathOf(campPatroller); ok {
		t.Fatal("a path given")
	}

	gp.TargetObjectID = 100107
	c.say(gs, "/path pingpong run")
	if l := gmReplies(t, gs); !hasLine(l, "a patroller (NPC 104) walks a path of 3 points now: back and forth, at a run") {
		t.Fatalf("/path: %q", l)
	}
	p, ok := c.gw.NPCPathOf(campPatroller)
	if !ok || p.Mode != entity.PathPingPong || !p.Run || len(p.Points) != 3 || p.Points[0].Y != 50 || p.Points[2].Z != 15040 {
		t.Fatalf("the path in the world: %+v %v", p, ok)
	}
	recs, _, err := database.NewNPCPathStore(db).All()
	if err != nil || len(recs) != 1 || recs[0].NPCID != campPatroller || recs[0].Mode != "pingpong" || recs[0].Speed != "run" ||
		len(recs[0].Points) != 3 || recs[0].Points[1].X != 25140 {
		t.Fatalf("npcpaths: %+v %v", recs, err)
	}
	if m, _ := db.GMMarks(1); len(m) != 0 {
		t.Fatalf("the marks were not cleared: %+v", m)
	}
	if n := auditLines(t, db, "npcpaths", "insert"); n != 1 {
		t.Fatalf("%d npcpaths inserts audited", n)
	}

	// a mark in another world than the NPC's: refused, the marks kept
	gp.World = 1
	c.say(gs, "/mark")
	gp.World = 0
	gmReplies(t, gs)
	c.say(gs, "/path once")
	if l := gmReplies(t, gs); !hasLine(l, "Mark 1 is in world 1, a patroller's row in world 0") {
		t.Fatalf("another world: %q", l)
	}
	if m, _ := db.GMMarks(1); len(m) != 1 {
		t.Fatalf("the refused path took the marks: %+v", m)
	}
	if p, _ := c.gw.NPCPathOf(campPatroller); p.Mode != entity.PathPingPong {
		t.Fatalf("the refused path changed it: %+v", p)
	}

	c.say(gs, "/path clear")
	if l := gmReplies(t, gs); !hasLine(l, "a patroller (NPC 104) has no path now") {
		t.Fatalf("/path clear: %q", l)
	}
	if _, ok := c.gw.NPCPathOf(campPatroller); ok {
		t.Fatal("still walking its path")
	}
	if recs, _, _ := database.NewNPCPathStore(db).All(); len(recs) != 0 {
		t.Fatalf("npcpaths after clear: %+v", recs)
	}
	if n := auditLines(t, db, "npcpaths", "delete"); n != 1 {
		t.Fatalf("%d npcpaths deletes audited", n)
	}
}

// A GM's petition (/bug here; Support's form and F3 file the same way) carries
// the GM's marks in its server view, each with its place, facing and note, as
// /marks numbers them. The control: a player's petition carries none.
func TestAGMsPetitionCarriesItsMarks(t *testing.T) {
	c, h, db, as, bs := bugWorld(t)
	h.limit.off = true
	c.h.SetCampStore(db)
	h.SetMarks(db)
	bob := c.gw.SessionPlayer(bs)
	stand(c, bob, 15023.3, 16.8, 27282.1, 1.25)
	c.say(bs, "/mark rock, east side")
	stand(c, bob, 15030, 17, 27290, 0)
	c.say(bs, "/mark")
	chat(t, c, bs, "/bug the camp at the rock: spawns at my marks")
	chat(t, c, as, "/bug something else")
	v := views(t, db)
	if len(v) != 2 {
		t.Fatalf("%d petitions", len(v))
	}
	gm, player := v[1001], v[1002]
	if gm.Character.Name != "Bob" || len(gm.Marks) != 2 {
		t.Fatalf("the GM's view: %+v", gm)
	}
	m := gm.Marks[0]
	if m.N != 1 || m.World != 0 || math.Abs(m.X-15023.3) > 1e-3 || math.Abs(m.Y-16.8) > 1e-3 || math.Abs(m.Z-27282.1) > 1e-3 ||
		math.Abs(m.Facing-1.25) > 1e-6 || m.Note != "rock, east side" || m.Char != "Bob" || m.At == "" {
		t.Fatalf("mark 1 in the view: %+v", m)
	}
	if gm.Marks[1].N != 2 || gm.Marks[1].X != 15030 {
		t.Fatalf("mark 2 in the view: %+v", gm.Marks[1])
	}
	if player.Character.Name != "Ann" || len(player.Marks) != 0 {
		t.Fatalf("the player's view: %+v", player)
	}
	var raw string
	db.QueryRow(`SELECT server_view FROM petitions WHERE id = 1001`).Scan(&raw)
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &keys); err != nil || keys["marks"] == nil {
		t.Fatalf("no marks key in the JSON: %v %.300s", err, raw)
	}
}
