package handlers

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A real server's quests (EQOA_REAL_DB names its database, a copy of which is
// read) after tools/fix_quest_targets.py: every creature the tool gave a step
// is one of its creatures to the engine, and on each loot step a kill of every
// one of them drops the step's item for a character on it (the draw fixed to
// land), where a kill of a creature the step does not name drops nothing (the
// control). Neriak's "New Gear" (the user, 2026-10-30: "dust was farming beaks
// ... and it never drops") is checked by name when the database has it: a
// magpie drops the beak, a crow does not. Skipped without a database, and on
// one the tool has not been applied to: none is in this repository.
func TestRealQuestTargetsDropOffEveryCreature(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "targets.db")
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
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'quest_targets_fix_log'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/fix_quest_targets.py has not been applied to this database")
	}
	type added struct {
		quest, step int
		name        string
	}
	var adds []added
	rows, err := db.Query(`SELECT questid, step, new FROM quest_targets_fix_log WHERE action = 'added' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a added
		if err := rows.Scan(&a.quest, &a.step, &a.name); err != nil {
			t.Fatal(err)
		}
		adds = append(adds, a)
	}
	rows.Close()
	if len(adds) == 0 {
		t.Fatal("the tool's log holds no creature added")
	}

	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	qe, err := NewQuestEngine(db, w, mgr, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}
	type drop struct {
		npc     entity.EntityID
		char    uint32
		pattern int
	}
	var drops []drop
	qe.drop = func(npc entity.EntityID, _ time.Time, char uint32, rec database.CharInventoryEntry) {
		drops = append(drops, drop{npc, char, rec.Pattern.PatternID})
	}
	qe.roll = func() float64 { return 0 }
	const char uint32 = 900000001
	kill := func(name string, npc entity.EntityID) {
		qe.killed(entity.KillCredit{NPCName: name, NPCSpawn: npc, NPCID: npc, Died: time.Now(),
			Players: []entity.KillPlayer{{ServerID: char, Name: "Targetest"}}})
	}
	checked := map[[2]int]bool{}
	for i, a := range adds {
		qu := qe.cat.byID[a.quest]
		if qu == nil {
			t.Errorf("quest %d is not read", a.quest)
			continue
		}
		st := qu.Step(a.step)
		if st == nil || !containsFold(st.Targets(), a.name) {
			t.Errorf("quest %d step %d: %q is not among its creatures %v", a.quest, a.step, a.name, st.Targets())
			continue
		}
		if st.Kind != database.StepLoot || checked[[2]int{a.quest, a.step}] {
			continue
		}
		checked[[2]int{a.quest, a.step}] = true
		pid, _, ok := st.LootItem()
		if !ok {
			continue
		}
		for _, name := range st.Targets() {
			if _, err := qe.SetCharacterQuest(char, a.quest, QuestReset, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := qe.SetCharacterQuest(char, a.quest, QuestSetStep, a.step); err != nil {
				t.Fatalf("quest %d step %d: %v", a.quest, a.step, err)
			}
			drops = nil
			kill("a creature no step names", entity.EntityID(10000+i))
			if len(drops) != 0 {
				t.Errorf("quest %d step %d: a creature it does not name dropped %v", a.quest, a.step, drops)
			}
			kill(name+" ", entity.EntityID(20000+i))
			if len(drops) != 1 || drops[0].pattern != pid || drops[0].char != char {
				t.Errorf("quest %d step %d: a kill of %q dropped %v, want its item %d", a.quest, a.step, name, drops, pid)
			}
		}
		if _, err := qe.SetCharacterQuest(char, a.quest, QuestReset, 0); err != nil {
			t.Fatal(err)
		}
	}
	// Neriak's shadow knight's beak, by name when this database has the quest as the content wrote it
	if qu := qe.cat.byID[100480]; qu != nil && qu.Name == "New Gear" {
		st := qu.Step(2)
		if st == nil || !containsFold(st.Targets(), "magpie") {
			t.Fatalf("New Gear's step 2 counts %v, want the magpies its journal names", st.Targets())
		}
		if _, err := qe.SetCharacterQuest(char, 100480, QuestSetStep, 2); err != nil {
			t.Fatal(err)
		}
		drops = nil
		kill("Crow ", 30001)
		if len(drops) != 0 {
			t.Errorf("a crow dropped %v: the guide's beak is the ravens' and magpies'", drops)
		}
		kill("Magpie ", 30002)
		if len(drops) != 1 {
			t.Errorf("a magpie dropped %v, want the beak", drops)
		}
	}
	steps := map[[2]int]bool{}
	for _, a := range adds {
		steps[[2]int{a.quest, a.step}] = true
	}
	t.Logf("%d creatures added over %d steps, %d loot steps' drops checked", len(adds), len(steps), len(checked))
}

func containsFold(v []string, s string) bool {
	for _, x := range v {
		if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(s)) {
			return true
		}
	}
	return false
}
