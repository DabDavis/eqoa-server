package handlers

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A shard's database after tools/enraged_vox_20261116.py (EQOA_REAL_DB names it; a copy is read), loaded as the game
// server loads it: the Enraged Lady Vox (row 122824) stands nowhere (spawn-only), Lady Vox (103173) as she did; a
// player on the new step 14 of "The Scepter of the Ritualists" (101028) at her lair without the primal stew is told and
// stays; with it, the stew is taken, the step done (15, the loot step, next) and the trigger "Permafrost: Enraged Lady
// Vox" raises one hostile copy of her row there; a second player there with the stew waits, the stew kept, while
// she lives (the trigger's lock). Skipped without a database.
func TestRealEnragedVoxRises(t *testing.T) {

	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "vox.db")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	log.SetOutput(io.Discard)
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	e := NewTriggerEngine(db, w, mgr, inv)
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	qe, err := NewQuestEngine(db, w, mgr, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	log.SetOutput(os.Stderr)

	orig := w.NPCs[122824]
	if orig == nil || orig.Name != "Enraged Lady Vox" || !orig.SpawnOnly {
		t.Fatalf("her row: %+v, want spawn-only", orig)
	}
	if lv := w.NPCs[103173]; lv == nil || lv.Name != "Lady Vox" || lv.SpawnOnly {
		t.Fatalf("Lady Vox's row: %+v, want standing", lv)
	}

	type char struct {
		rec  *database.CharacterRecord
		sess *session.Session
		p    *entity.Player
	}
	mk := func(name string, port int, id entity.EntityID) char {
		rec := &database.CharacterRecord{CharName: name, AccountID: 1, Level: 60}
		if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
			t.Fatal(err)
		}
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, uint32(port), 1, 1, 2, true)
		p := &entity.Player{ServerID: id, CharServerID: rec.ServerID, Name: name, Level: 60, HP: 8000, MaxHP: 8000,
			X: 4000, Y: 60, Z: 4000, World: 0}
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		sess.SetInventoryList(newInvList(nil))
		if _, err := qe.SetCharacterQuest(rec.ServerID, 101028, QuestGive, 14); err != nil {
			t.Fatal(err)
		}
		return char{rec, sess, p}
	}
	stew := func(c char) {
		if _, err := inv.AddCharItem(c.rec.ServerID, 701090, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		rows, err := inv.GetCharacterItems(c.rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		c.sess.SetInventoryList(newInvList(rows))
	}
	held := func(c char) int {
		var n int
		db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = 701090`,
			c.rec.ServerID).Scan(&n)
		return n
	}
	stepOf := func(c char) int {
		for _, cq := range qe.CharacterQuests(c.rec.ServerID) {
			if cq.QuestID == 101028 {
				return cq.Step
			}
		}
		return -1
	}
	risen := func() (out []*entity.NPC) {
		for _, n := range w.NPCs {
			if n.Temporary && !n.Dead && strings.EqualFold(n.Name, "Enraged Lady Vox") {
				out = append(out, n)
			}
		}
		return out
	}

	a, b := mk("Voxone", 7051, 900051), mk("Voxtwo", 7052, 900052)
	w.UpdatePosition(a.p.ServerID, orig.SpawnX+5, orig.SpawnY, orig.SpawnZ+5, 0)
	qe.checkReach()
	if s := stepOf(a); s != 14 || len(risen()) != 0 {
		t.Fatalf("at her lair with no stew: step %d, %d risen; want 14, none", s, len(risen()))
	}
	stew(a)
	qe.checkReach()
	if s := stepOf(a); s != 15 {
		t.Fatalf("with the stew: step %d, want 15", s)
	}
	if n := held(a); n != 0 {
		t.Errorf("the stew still held (%d), want set down", n)
	}
	v := risen()
	if len(v) != 1 {
		t.Fatalf("%d risen, want 1", len(v))
	}
	if v[0].CopyOf != 122824 || v[0].SpawnOnly {
		t.Fatalf("the risen Vox: copy of %d, spawn-only %v", v[0].CopyOf, v[0].SpawnOnly)
	}
	t.Logf("the enraged Lady Vox risen: level %d, %d hit points", v[0].Level, v[0].MaxHP)
	if st := qe.cat.byID[101028].Step(15); st == nil || st.Kind != database.StepLoot || !strings.EqualFold(st.Target, "Enraged Lady Vox") {
		t.Fatalf("step 15: %+v, want the loot step on her", st)
	}

	stew(b)
	w.UpdatePosition(b.p.ServerID, orig.SpawnX-5, orig.SpawnY, orig.SpawnZ-5, 0)
	qe.checkReach()
	if s := stepOf(b); s != 14 || held(b) != 1 || len(risen()) != 1 {
		t.Fatalf("the second player while she lives: step %d, stew %d, %d risen; want 14, 1, 1", s, held(b), len(risen()))
	}
}
