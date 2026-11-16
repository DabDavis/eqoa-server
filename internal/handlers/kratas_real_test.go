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

// A shard's database after tools/kratas_20261116.py (EQOA_REAL_DB names it; a copy is read), loaded as the game
// server loads it: Kratas (row 123007) stands nowhere (spawn-only); the Kratas quest (101265) set on its last step and
// its player brought to the whale bones finishes it, and the trigger "Kratas Rises" raises one Kratas there, a
// hostile copy of the original at level 57 with his template's hit points (about 200,000); a second player there on
// the same step waits while he lives (the trigger's lock). His loot tables are his own two "boss kratas" ones, which
// make him a raid boss; the five jewelcraft recipes load. Skipped without a database.
func TestRealKratasRises(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "kratas.db")
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

	orig := w.NPCs[123007]
	if orig == nil || orig.Name != "Kratas" || !orig.SpawnOnly {
		t.Fatalf("Kratas's row: %+v, want spawn-only", orig)
	}
	if of, _ := w.NPCTemplateCount("kratas"); of != 1 {
		t.Fatalf("%d NPCs fight by the template kratas, want his row alone", of)
	}
	for _, pr := range w.ClassTemplateProblems() {
		if strings.Contains(strings.ToLower(pr), "kratas") {
			t.Errorf("a template problem: %s", pr)
		}
	}

	type char struct {
		rec  *database.CharacterRecord
		sess *session.Session
		p    *entity.Player
	}
	mk := func(name string, port int, id entity.EntityID) char {
		rec := &database.CharacterRecord{CharName: name, AccountID: 1, Level: 57}
		if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
			t.Fatal(err)
		}
		sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, uint32(port), 1, 1, 2, true)
		p := &entity.Player{ServerID: id, CharServerID: rec.ServerID, Name: name, Level: 57, HP: 8000, MaxHP: 8000,
			X: 13000, Y: 60, Z: 31000, World: 0}
		w.Players[p.ServerID] = p
		w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
		sess.SetECSEntity(p)
		sess.SetCharacterInWorld(true)
		sess.SetInventoryList(newInvList(nil))
		if _, err := qe.SetCharacterQuest(rec.ServerID, 101265, QuestGive, 4); err != nil {
			t.Fatal(err)
		}
		return char{rec, sess, p}
	}
	stepOf := func(c char) (int, int) {
		for _, cq := range qe.CharacterQuests(c.rec.ServerID) {
			if cq.QuestID == 101265 {
				return cq.Step, cq.Done
			}
		}
		return -1, -1
	}
	risen := func() (out []*entity.NPC) {
		for _, n := range w.NPCs {
			if n.Temporary && !n.Dead && strings.EqualFold(n.Name, "Kratas") {
				out = append(out, n)
			}
		}
		return out
	}

	a, b := mk("Kratasone", 7041, 900041), mk("Kratastwo", 7042, 900042)
	if s, _ := stepOf(a); s != 4 {
		t.Fatalf("set on step %d, want 4", s)
	}
	if n := len(risen()); n != 0 {
		t.Fatalf("%d Kratas before the bones", n)
	}
	w.UpdatePosition(a.p.ServerID, orig.SpawnX+10, orig.SpawnY+5, orig.SpawnZ+10, 0)
	qe.checkReach()
	if s, d := stepOf(a); s != 0 || d != 1 {
		t.Fatalf("at the bones: step %d done %d, want the quest done", s, d)
	}
	k := risen()
	if len(k) != 1 {
		t.Fatalf("%d Kratas rose, want 1", len(k))
	}
	kr := k[0]
	if kr.CopyOf != 123007 || kr.SpawnOnly || kr.Level != 57 || kr.MaxHP < 180000 || kr.MaxHP > 220000 {
		t.Fatalf("the risen Kratas: copy of %d, spawn-only %v, level %d, %d hit points", kr.CopyOf, kr.SpawnOnly, kr.Level, kr.MaxHP)
	}
	t.Logf("Kratas risen: level %d, %d hit points", kr.Level, kr.MaxHP)

	w.UpdatePosition(b.p.ServerID, orig.SpawnX-10, orig.SpawnY+5, orig.SpawnZ-10, 0)
	qe.checkReach()
	if s, d := stepOf(b); s != 4 || d != 0 {
		t.Fatalf("the second player while Kratas lives: step %d done %d, want still 4", s, d)
	}
	if n := len(risen()); n != 1 {
		t.Fatalf("%d Kratas with the second player there, want still 1", n)
	}

	rolls, err := inv.NPCLootRolls("Kratas", 57)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rolls {
		for _, c := range r.Choices {
			got[r.Table] = c.Item.Pattern.ItemName
		}
	}
	if got["boss kratas"] != "Shimmering Seaweed" || got["boss kratas tentacle"] != "Kratas Tentacle" || len(got) != 2 {
		t.Fatalf("his loot tables: %v", got)
	}
	if boss, err := inv.RaidBoss("Kratas"); err != nil || !boss {
		t.Fatalf("a raid boss: %v %v", boss, err)
	}
	recipes, err := db.CraftRecipes()
	if err != nil {
		t.Fatal(err)
	}
	for id := 8001; id <= 8005; id++ {
		r, ok := recipes.Recipe(id)
		if !ok || r.Skill != 3 || r.Level != 9 {
			t.Fatalf("recipe %d: %+v", id, r)
		}
	}
}
