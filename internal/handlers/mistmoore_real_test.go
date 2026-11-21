package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// TestRealMistmoore runs tools/mistmoore_20261121.py on a copy of a server database (EQOA_REAL_DB) and holds what
// it wrote to the server's own readers: the four Mistmoore quests' steps 4-8 (the organ's reach taking the tome and
// firing "Maestro of Rancor", the coffin's give of the song firing "Mayong Mistmoore", the kill, the return), both
// triggers with no problem, the Maestro's and Mistmoore's world-0 rows held out of the world (the Salt Mines
// dungeon's world-14 copies untouched), the coffin standing under its new name, both templates read with no problem
// and given to the bosses by name (the dungeon copies keep their own by row), and Rancor's Requiem an area drain.
// A second run writes nothing; --undo leaves every table as it was.
func TestRealMistmoore(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "real.db")
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
	db0, err := database.NewSQLite(path) // the server's own tables first, as a start makes them
	if err != nil {
		t.Fatal(err)
	}
	db0.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	before := tableDigest(t, raw)
	raw.Close()
	t.Logf("apply:\n%s", mmTool(t, path, "--apply", "--no-backup"))
	if again := mmTool(t, path, "--apply", "--no-backup"); !strings.Contains(again, "nothing to do") {
		t.Errorf("a second run: %s", again)
	}

	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	// the quests
	qs, err := database.NewQuestStore(db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := range qs {
		q := &qs[i]
		if q.Name != "Mistmoore" || q.Giver != "Djivan Mertshak" {
			continue
		}
		n++
		if len(q.Steps) != 8 {
			t.Fatalf("quest %d has %d steps", q.ID, len(q.Steps))
		}
		s4, s5, s6, s7, s8 := q.Steps[3], q.Steps[4], q.Steps[5], q.Steps[6], q.Steps[7]
		if s4.Kind != database.StepReach || len(s4.Items) != 1 || s4.Items[0].PatternID != 701545 || s4.Height != 20 {
			t.Errorf("quest %d step 4: %+v", q.ID, s4)
		}
		trig := func(st database.QuestStep, name string) bool {
			for _, e := range st.Events {
				if e.Do == database.EventTrigger && e.Trigger == name && e.Text != "" {
					return true
				}
			}
			return false
		}
		if !trig(s4, "Maestro of Rancor") {
			t.Errorf("quest %d step 4 fires no Maestro: %+v", q.ID, s4.Events)
		}
		if s5.Kind != database.StepLoot || s5.Target != "Maestro of Rancor" || s5.Items[0].PatternID != 701546 {
			t.Errorf("quest %d step 5: %+v", q.ID, s5)
		}
		if s6.Kind != database.StepGive || s6.Target != "mistmoore coffin" || len(s6.Items) != 1 || s6.Items[0].PatternID != 701546 ||
			!trig(s6, "Mayong Mistmoore") {
			t.Errorf("quest %d step 6: %+v", q.ID, s6)
		}
		if s7.Kind != database.StepKill || s7.Target != "Mayong Mistmoore" {
			t.Errorf("quest %d step 7: %+v", q.ID, s7)
		}
		if s8.Kind != database.StepTalk || s8.Target != "Djivan Mertshak" || len(s8.RewardItems) == 0 {
			t.Errorf("quest %d step 8: %+v", q.ID, s8)
		}
	}
	if n != 4 {
		t.Errorf("%d Mistmoore quests, want 4", n)
	}

	// the triggers
	trigs, err := db.GMTriggers()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tr := range trigs {
		switch tr.Name {
		case "Maestro of Rancor", "Mayong Mistmoore":
			found[tr.Name] = true
			if p := database.TriggerProblems(tr); len(p) > 0 {
				t.Errorf("trigger %q: %v", tr.Name, p)
			}
		}
	}
	if len(found) != 2 {
		t.Errorf("triggers: %v", found)
	}

	// the templates as the database reads them
	rows, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if strings.Contains(b, "maestro") || strings.Contains(b, "mistmoore") {
			t.Errorf("template refused: %s", b)
		}
	}
	for _, name := range []string{"maestro of rancor", "mayong mistmoore"} {
		if _, ok := rows[name]; !ok {
			t.Errorf("no template %q", name)
		}
	}
	if m := rows["mayong mistmoore"]; m.HPPct != 1485 || len(m.Abilities) != 3 {
		t.Errorf("Mistmoore's template: hp %d, %d abilities", m.HPPct, len(m.Abilities))
	}

	// the world's reading
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(strings.ToLower(p), "maestro") || strings.Contains(strings.ToLower(p), "mistmoore") {
			t.Errorf("template: %s", p)
		}
	}
	coffins := 0
	for _, npc := range w.NPCs {
		switch {
		case npc.Name == "Maestro of Rancor" || npc.Name == "Mayong Mistmoore":
			if npc.World == 0 {
				if !npc.SpawnOnly {
					t.Errorf("%s (%d) stands in world 0", npc.Name, npc.ServerID)
				}
				want := strings.ToLower(npc.Name)
				if got := npc.TemplateName(); got != want {
					t.Errorf("%s fights by %q, want %q", npc.Name, got, want)
				}
			} else if got := npc.TemplateName(); !strings.HasPrefix(got, "heroic salt mines") {
				t.Errorf("the dungeon's %s (%d) fights by %q", npc.Name, npc.ServerID, got)
			}
		case npc.Name == "mistmoore coffin":
			coffins++
			if npc.SpawnOnly || npc.NPCType&0x80 == 0 {
				t.Errorf("the coffin: spawn-only %v, type %#x", npc.SpawnOnly, npc.NPCType)
			}
		case npc.Name == "coffin of Mayong":
			t.Errorf("a coffin of Mayong still stands (%d)", npc.ServerID)
		}
	}
	if coffins != 1 {
		t.Errorf("%d mistmoore coffins", coffins)
	}
	var eff string
	db.QueryRow(`SELECT e.effects FROM spelleffect e JOIN spellpattern p USING (spellid) WHERE p.spellname = 'Rancor''s Requiem'`).Scan(&eff)
	var fx struct {
		Scope   string `json:"scope"`
		Radius  int    `json:"radius"`
		Effects []struct {
			Kind   string `json:"kind"`
			Amount int    `json:"amount"`
		} `json:"effects"`
	}
	if err := json.Unmarshal([]byte(eff), &fx); err != nil || fx.Scope != "player_aoe" || fx.Radius != 30 || len(fx.Effects) != 1 ||
		fx.Effects[0].Kind != "drain" || fx.Effects[0].Amount != -860 {
		t.Errorf("Rancor's Requiem: %s (%v)", eff, err)
	}
	db.Close()

	t.Logf("undo:\n%s", mmTool(t, path, "--undo", "--no-backup"))
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	after := tableDigest(t, raw)
	for tbl, h := range before {
		if after[tbl] != h && tbl != "gm_audit" && tbl != "sqlite_sequence" {
			t.Errorf("table %s differs after --undo", tbl)
		}
	}
	for tbl := range after {
		if _, ok := before[tbl]; !ok {
			t.Errorf("table %s left after --undo", tbl)
		}
	}
}

// mmTool runs tools/mistmoore_20261121.py on the database.
func mmTool(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-E", "-s", "../../tools/mistmoore_20261121.py", "--db", path}, args...)...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the tool %v: %v\n%s", args, err, b)
	}
	return string(b)
}
