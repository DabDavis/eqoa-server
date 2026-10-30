package handlers

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A real server's database after tools/fix_quest_spawns.py (EQOA_REAL_DB names
// it; a copy is read): once npcspawnonly is read, every NPC of a name in it
// stands out of the world and every other NPC in it; and on each quest whose
// step 5 is Jarek's Creation's loot, a player at the hut on that step with no
// creation gets one made before it (the lost spawn made again, the creation's
// first making being step 4's, TestJareksCreationIsMadeThenFoughtAndLooted's),
// out of the original that stands out of the world, attacking it, and a kill
// of it drops the step's item and taking that does the step. The control: a
// player 40 off the original gets none. Skipped without a database, and on one
// the tool has not been applied to: none is in this repository.
func TestRealQuestSpawnsStandOutOfTheWorld(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "spawns.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'quest_spawn_fix_log'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/fix_quest_spawns.py has not been applied to this database")
	}

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	log.SetOutput(io.Discard) // a line an NPC
	w.LoadNPCs(database.NewNPCStore(db))
	held, _, err := w.LoadSpawnOnly(db)
	log.SetOutput(os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	names, err := db.NPCSpawnOnly()
	if err != nil {
		t.Fatal(err)
	}
	if held == 0 || len(names) == 0 {
		t.Fatalf("npcspawnonly: %d names, %d NPCs out of the world", len(names), held)
	}
	count := 0
	for _, st := range w.NPCStates(-1) {
		if st.SpawnOnly != names[database.NPCClassKey(st.Name)] {
			t.Errorf("%q (%d): out of the world %v, its name in npcspawnonly %v", st.Name, st.ID, st.SpawnOnly, names[database.NPCClassKey(st.Name)])
		}
		if st.SpawnOnly {
			count++
		}
	}
	if count != held {
		t.Errorf("%d NPCs out of the world, LoadSpawnOnly said %d", count, held)
	}
	if !names[database.NPCClassKey("Jarek's Creation")] {
		t.Fatal("Jarek's Creation is not in npcspawnonly")
	}
	ox, oy, oz, _, world, ok := w.NPCByName("Jarek's Creation")
	if !ok {
		t.Fatal("no Jarek's Creation in the world")
	}

	qe, err := NewQuestEngine(db, w, mgr, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}
	var drops []lootDrop
	qe.drop = func(npc entity.EntityID, _ time.Time, char uint32, rec database.CharInventoryEntry) {
		drops = append(drops, lootDrop{npc, char, rec.Pattern.PatternID})
	}
	qe.roll = func() float64 { return 0.99 }
	rec := &database.CharacterRecord{CharName: "Spawntest", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7019}, 19, 1, 1, 2, true)
	p := &entity.Player{ServerID: 900019, CharServerID: rec.ServerID, Name: "Spawntest", Level: 10, HP: 500, MaxHP: 500,
		X: ox + 10, Y: oy, Z: oz, World: world}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	copies := func() []*entity.NPC {
		var out []*entity.NPC
		for _, c := range w.NPCs {
			if c.QuestSpawn && !c.Dead {
				out = append(out, c)
			}
		}
		return out
	}

	checked := 0
	for _, qu := range qe.cat.all {
		st := qu.Step(5)
		if !qu.Enabled || st == nil || st.Kind != database.StepLoot || !strings.EqualFold(st.Target, "Jarek's Creation") {
			continue
		}
		pid, _, ok := st.LootItem()
		if !ok {
			t.Errorf("%d %q step 5 drops no item", qu.ID, qu.Name)
			continue
		}
		checked++
		if _, err := qe.SetCharacterQuest(rec.ServerID, qu.ID, QuestSetStep, 5); err != nil {
			t.Fatalf("%d: %v", qu.ID, err)
		}
		p.X = ox + 40
		qe.respawnLost()
		if n := len(copies()); n != 0 {
			t.Errorf("%d %q: 40 off the original, %d creations came", qu.ID, qu.Name, n)
		}
		p.X = ox + 10
		qe.respawnLost()
		cs := copies()
		if len(cs) != 1 || cs[0].Name != "Jarek's Creation" || cs[0].SpawnOnly || cs[0].CurrentEnemy != p.ServerID {
			t.Errorf("%d %q: %d creations at the hut, want one attacking the player", qu.ID, qu.Name, len(cs))
			continue
		}
		c := cs[0]
		drops = nil
		qe.killed(entity.KillCredit{NPCName: c.Name, NPCID: c.ServerID, Died: time.Now(),
			Players: []entity.KillPlayer{{ServerID: rec.ServerID, Name: p.Name}}})
		if len(drops) != 1 || drops[0] != (lootDrop{c.ServerID, rec.ServerID, pid}) {
			t.Errorf("%d %q: the creation's drops %v, want item %d on it", qu.ID, qu.Name, drops, pid)
		}
		qe.Looted(sess, p, pid, 1)
		if got := qe.CharacterQuests(rec.ServerID); len(got) != 1 || got[0].Step != 6 {
			t.Errorf("%d %q: the item taken, the quest is %+v, want at step 6", qu.ID, qu.Name, got)
		}
		if _, err := qe.SetCharacterQuest(rec.ServerID, qu.ID, QuestReset, 0); err != nil {
			t.Fatal(err)
		}
		w.DespawnNPC(c.ServerID)
	}
	if checked < 5 {
		t.Errorf("%d quests loot Jarek's Creation at step 5, want the five", checked)
	}
}
