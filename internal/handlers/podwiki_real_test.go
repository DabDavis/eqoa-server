package handlers

import (
	"io"
	"log"
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

// A real server's Plane of Disease flag quest as the wiki has it (EQOA_REAL_DB
// names its database, a copy of which is walked; skipped before
// tools/pod_wiki_20261113.py): both quests at level 51 under the wiki's names;
// Xeptip gives no suit; the quester walks in without one, and the Oozing
// Portal does not let it through until it has one; six stitched zombie skins
// and a sinew cord, handed to Fatima Ubel (two swatches, then the suit), make
// it, and the portal then sends it on. The ward where Dust stood stops a
// player without the suit on the lower halls' side alone: not one with it,
// not a GM, not one on its far side. With EQOA_GROUND_DIR (a worlds/ground
// folder) every creature the tool placed stands on a floor and the ward spans
// the hall.
func TestRealPoDFlagQuestAsTheWiki(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var n int
	w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'pod_wiki_1113_log'`).Scan(&n)
	if n == 0 {
		t.Skip("tools/pod_wiki_20261113.py is not applied here")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	id := func(sql string, args ...any) int {
		t.Helper()
		var v int
		if err := w.db.QueryRow(sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return v
	}
	flag1 := id(`SELECT questid FROM quest_import_quests WHERE key = 'Portal/plane-of-disease/flag-1'`)
	flag2 := id(`SELECT questid FROM quest_import_quests WHERE key = 'Portal/plane-of-disease/flag-2'`)
	swatchQ := id(`SELECT questid FROM quest_import_quests WHERE key = 'Portal/plane-of-disease/suit-swatch'`)
	sewQ := id(`SELECT questid FROM quest_import_quests WHERE key = 'Portal/plane-of-disease/suit-sew'`)
	skin := id(`SELECT patternid FROM itempattern WHERE itemname = 'Stitched Zombie Skin'`)
	cord := id(`SELECT patternid FROM itempattern WHERE itemname = 'Sinew Cord'`)
	q1, q2 := w.quest(flag1), w.quest(flag2)
	if q1 == nil || q2 == nil || q1.MinLevel != 51 || q2.MinLevel != 51 {
		t.Fatalf("the quests' levels: %+v %+v, want 51", q1, q2)
	}
	if st := q1.Step(1); st.Target != "Necrologist Xeptip" || len(st.Events) != 0 {
		t.Errorf("Xeptip's step %+v: no give", st)
	}
	if q1.Step(3).Target != "Deathknight Bileheart" || q1.Step(4).Target != "Deathknight Halik" {
		t.Errorf("the death knights: %q, %q", q1.Step(3).Target, q1.Step(4).Target)
	}
	for step, name := range map[int]string{2: "Fatima Ubel", 6: "Rebo Chilins", 11: "Medelonia Virel", 14: "Oozing Portal"} {
		if got := q2.Step(step).Target; got != name {
			t.Errorf("The Last House's step %d is at %q, want %q", step, got, name)
		}
	}
	if st := q2.Step(1); st.Kind != database.StepTalk || len(st.Items) != 0 {
		t.Errorf("the mausoleum's step asks %v (%s): the wiki's first entry needs no suit", st.Items, st.Kind)
	}
	for _, nm := range []string{"Fatima Ubel", "Rebo Chilins", "Medelonia Virel", "Oozing Portal", "Deathknight Bileheart", "Deathknight Halik"} {
		if id(`SELECT count(*) FROM npcs WHERE npc_name = ?`, nm) != 1 {
			t.Errorf("no single NPC %q", nm)
		}
	}

	// the walk: the hearts, then the Last House to the Oozing Portal
	if why := w.walkTo(flag1); why != "" {
		t.Fatalf("The Death Knights' Hearts: %s", why)
	}
	if held := id(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, w.p.CharServerID, zombieFleshSuit); held != 0 {
		t.Fatalf("Xeptip gave %d suits", held)
	}
	w.tidy()
	w.p.Level = 51
	if _, err := w.qe.SetCharacterQuest(w.p.CharServerID, flag2, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	w.settle()
	for guard := 0; guard < 40; guard++ {
		step, _ := w.progress(flag2)
		if step == 14 {
			break
		}
		if why := w.doStep(q2, q2.Step(step)); why != "" {
			t.Fatalf("The Last House step %d: %s", step, why)
		}
		w.settle()
	}
	if step, _ := w.progress(flag2); step != 14 {
		t.Fatalf("The Last House at step %d, want the Oozing Portal's 14", step)
	}
	w.qe.Talk(w.sess, w.p, w.npc("Oozing Portal", 0))
	w.settle()
	if step, done := w.progress(flag2); step != 14 || done != 0 {
		t.Fatalf("the Oozing Portal let a player without the suit through (step %d, done %d)", step, done)
	}

	// the suit, sewn by Fatima: six skins and a cord
	give := func(pid, count int) {
		t.Helper()
		pat, err := w.qe.items.GetItemPattern(pid)
		if err != nil || pat == nil {
			t.Fatalf("item %d: %v", pid, err)
		}
		if got, err := GiveItemNow(w.sess, w.gw, w.inv, *pat, count); err != nil || got != count {
			t.Fatalf("item %d: %d given (%v)", pid, got, err)
		}
	}
	give(skin, 6)
	give(cord, 1)
	for _, qid := range []int{swatchQ, swatchQ, sewQ} {
		if why := w.walkOne(w.quest(qid)); why != "" {
			t.Fatalf("hand-in %d: %s", qid, why)
		}
	}
	if held := id(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, w.p.CharServerID, zombieFleshSuit); held != 1 {
		t.Fatalf("after the hand-ins %d suits held, want 1", held)
	}
	if left := id(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid IN (?, ?)`, w.p.CharServerID, skin, cord); left != 0 {
		t.Errorf("%d skins or cords left after the suit was sewn", left)
	}
	w.qe.Talk(w.sess, w.p, w.npc("Oozing Portal", 0))
	w.settle()
	if _, done := w.progress(flag2); done != 1 {
		t.Fatalf("with the suit the Oozing Portal did not finish The Last House: %s", w.lastWords())
	}

	// the ward
	walls, err := w.db.QuestWalls()
	if err != nil {
		t.Fatal(err)
	}
	var ward database.QuestWall
	for _, wl := range walls {
		if wl.Item == zombieFleshSuit {
			ward = wl
		}
	}
	if ward.NPCID == 0 || ward.Text == "" || len(ward.Inner) == 0 {
		t.Fatalf("no ward of the suit: %+v", walls)
	}
	var wx, wy, wz, size float32
	if err := w.db.QueryRow(`SELECT x_coord, y_coord, z_coord, size FROM npcs WHERE id = ? AND world = 5 AND modelid = ?`,
		ward.NPCID, int32(entity.BlockerBody)).Scan(&wx, &wy, &wz, &size); err != nil {
		t.Fatalf("the ward's row %d: %v", ward.NPCID, err)
	}
	gw := entity.NewWorld()
	gw.NPCs[entity.EntityID(ward.NPCID)] = &entity.NPC{ServerID: entity.EntityID(ward.NPCID), Name: "invisible wall", World: 5,
		X: wx, Y: wy, Z: wz, NPCType: 0x80, ModelID: int64(entity.BlockerBody)}
	gw.SetQuestWalls(walls)
	p := &entity.Player{ServerID: 1, CharServerID: 2, Name: "Dust", World: 5}
	at := func(x, y, z float32, gm bool) bool {
		p.X, p.Y, p.Z, p.GM = x, y, z, gm
		return len(gw.ItemWallsStopping(p, 30)) == 1
	}
	if !at(4634, -11.9, 3604, false) {
		t.Error("the ward does not stop a player without the suit in the lower hall east of it")
	}
	if at(4634, -11.9, 3604, true) {
		t.Error("the ward stops a GM")
	}
	if at(4614, -12, 3610, false) || at(4605, -1.3, 3616, false) {
		t.Error("the ward shuts a player without the suit in on its far side (the west arm, the ramp's head)")
	}
	gw.SetQuestState(p.CharServerID, map[entity.EntityID]bool{entity.EntityID(ward.NPCID): true}, map[int]bool{zombieFleshSuit: true})
	if at(4634, -11.9, 3604, false) {
		t.Error("the ward stops the suit's holder")
	}

	dir := os.Getenv("EQOA_GROUND_DIR")
	if dir == "" {
		t.Log("EQOA_GROUND_DIR not set: the placements' floors not checked")
		return
	}
	g, err := ground.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	rows, err := w.db.Query(`SELECT npc_name, x_coord, y_coord, z_coord FROM npcs WHERE world = 5 AND
		lower(npc_name) IN ('last house surgeon', 'master surgeon', 'last house zombie', 'surgeon''s zombie')
		OR id IN (SELECT rid FROM pod_wiki_1113_log WHERE tbl = 'npcs' AND op = 'added')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	placed := 0
	for rows.Next() {
		var nm string
		var x, y, z float32
		rows.Scan(&nm, &x, &y, &z)
		if nm == "invisible wall" {
			continue
		}
		placed++
		if f, ok := g.Floor(5, x, y+1, z); !ok || f < y-0.5 || f > y+0.5 {
			t.Errorf("%s at (%.1f, %.2f, %.1f): the floor there is %.2f (%v)", nm, x, y, z, f, ok)
		}
	}
	if placed != 9 {
		t.Errorf("%d placed creatures, want 9", placed)
	}
	// the hall's walls within the box's half-width either side of its middle
	half := 2.5 * size
	floor, _ := g.Floor(5, wx, wy, wz)
	if !g.Wall(5, wx, floor+3, wz, wx, floor+3, wz-half) || !g.Wall(5, wx, floor+3, wz, wx, floor+3, wz+half) {
		t.Errorf("the ward (%.1f, %.1f) half-width %.1f does not reach both of the hall's walls", wx, wz, half)
	}
}
