package database

import (
	"os"
	"strings"
	"testing"
)

// The Siliskor quest as tools/siliskor_quest_20261118.py writes it, on a real
// server's database (EQOA_REAL_DB names it; a copy is read: VACUUM INTO): its
// events read, every spawn a group's at a place, Siliskor's and Pomus's
// templates load (his tank's 700 and his players-only poison), and Zaryn Boome
// keeps a rare clock. Skipped where the tool is not applied.
func TestRealSiliskorQuestAsWritten(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	db, err := NewSQLite(src)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'siliskor_1118_log'`).Scan(&n)
	if n == 0 {
		t.Skip("tools/siliskor_quest_20261118.py is not applied to this database")
	}
	qs, err := NewQuestStore(db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	var qu *Quest
	for i := range qs {
		if qs[i].Name == "Siliskor" && qs[i].Giver == "Zaryn Boome" {
			qu = &qs[i]
		}
	}
	if qu == nil {
		t.Fatal("no Siliskor quest")
	}
	spawns := map[int][]string{}
	for _, st := range qu.Steps {
		var evs string
		db.QueryRow(`SELECT events FROM queststeps WHERE questid = ? AND step = ?`, qu.ID, st.Step).Scan(&evs)
		if p := EventProblems(evs, true); len(p) > 0 {
			t.Errorf("step %d's events: %v", st.Step, p)
		}
		for _, e := range st.Events {
			if e.Do != EventSpawn {
				continue
			}
			if !e.Group || e.X == 0 || e.Z == 0 {
				t.Errorf("step %d spawns %q not as a group's at a place: %+v", st.Step, e.NPC, e)
			}
			spawns[st.Step] = append(spawns[st.Step], strings.ToLower(e.NPC))
		}
	}
	if got := strings.Join(spawns[9], ","); got != "siliskor,pomus magemell,dragon worshipper,dragon worshipper" {
		t.Errorf("step 9 raises %q", got)
	}
	if got := strings.Join(spawns[6], ","); got != "mirrored soul" {
		t.Errorf("step 6 raises %q", got)
	}
	if len(spawns[3]) != 1 || !strings.HasPrefix(spawns[3][0], "#") {
		t.Errorf("step 3's watching dragon: %q", spawns[3])
	}
	ts, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if strings.Contains(b, "siliskor") || strings.Contains(b, "pomus") {
			t.Errorf("a template does not load: %s", b)
		}
	}
	sil, ok := ts["siliskor"]
	if !ok || sil.HPPct != 1914 || sil.SpecialEvery != 0 || len(sil.Abilities) != 2 {
		t.Fatalf("siliskor: %+v", sil)
	}
	if a := sil.Abilities[0]; a.Kind != AbilitySetHP || a.SetHP != 700 || a.Radius != 0 {
		t.Errorf("his tank's: %+v", a)
	}
	if a := sil.Abilities[1]; a.Kind != AbilitySetHP || !a.PlayersOnly || a.Radius != 40 || a.Tick != 600 || a.Resist != "PR" {
		t.Errorf("his poison: %+v", a)
	}
	if p, ok := ts["pomus magemell"]; !ok || p.HPPct != 328 {
		t.Errorf("pomus magemell: %+v", p)
	}
	cyc, err := db.NPCRareCycles()
	if err != nil {
		t.Fatal(err)
	}
	var zaryn int
	db.QueryRow(`SELECT id FROM npcs WHERE npc_name = 'Zaryn Boome'`).Scan(&zaryn)
	found := false
	for _, c := range cyc {
		found = found || c.NPCRow == zaryn
	}
	if !found {
		t.Errorf("Zaryn Boome (%d) keeps no rare clock: %+v", zaryn, cyc)
	}
}
