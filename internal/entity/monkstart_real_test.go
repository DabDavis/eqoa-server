package entity

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Over a real database after tools/monk_start.py --apply (EQOA_MONK=1 and
// EQOA_REAL_DB naming a copy): a new monk of every race starts at the
// monastery with nothing there attacking it on sight (StartThreats, the
// server's own start check), the Qeynos factions' NPCs show it no red face,
// and the monastery's people welcome a monk of another race than the humans
// (Qeynos's lines, npcdialog class 4 not_race 0), never a human monk.
func TestMonkStartRealDB(t *testing.T) {
	if os.Getenv("EQOA_MONK") == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_MONK=1 and EQOA_REAL_DB naming a database after tools/monk_start.py --apply")
	}
	w := realWorld(t)
	db, err := database.NewSQLite(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := LoadRaceDefs(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	spots, err := db.StartSpots()
	if err != nil {
		t.Fatal(err)
	}
	var monks []database.StartSpot
	for _, s := range spots {
		if s.Class == 4 {
			monks = append(monks, s)
		}
	}
	monks = append(monks, database.StartSpot{Race: raceFroglok, Class: 4, HumType: 1, World: monks[0].World,
		X: monks[0].X, Y: monks[0].Y, Z: monks[0].Z})
	for _, th := range w.StartThreats(monks) {
		t.Errorf("a new monk is attacked at its start: %+v", th)
	}
	lines, err := db.NPCDialog()
	if err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	var speakers []*NPC
	for _, npc := range w.NPCs {
		if npc.World == 0 && distXZ(npc.SpawnX, npc.SpawnZ, 5593.7, 17626.6) <= 80 && w.npcFactionLocked(npc) != "" {
			speakers = append(speakers, npc)
		}
	}
	w.mu.Unlock()
	if len(speakers) == 0 {
		t.Fatal("no one of a faction at the monastery")
	}
	for _, s := range monks {
		p := &Player{ServerID: 0x7FFFFFF0, Name: "Ix", Level: 1, Race: s.Race, Class: 4, HumType: s.HumType}
		welcomed := 0
		for _, npc := range speakers {
			w.mu.Lock()
			mood := w.dialogMoodLocked(p, npc)
			w.mu.Unlock()
			if mood == MoodHostile || mood == MoodWary {
				t.Errorf("race %d monk: %s is %s", s.Race, npc.Name, mood)
			}
			for i := 0; i < 40; i++ {
				if _, line, id, ok := w.TownLine(p, npc, lines, 0); ok {
					for _, l := range lines {
						if l.ID == id && l.ForClass {
							welcomed++
							if i == 0 && welcomed == 1 {
								t.Logf("race %d monk hears %s: %q", s.Race, npc.Name, line)
							}
						}
					}
				}
			}
		}
		human := s.Race == 0
		if human && welcomed > 0 || !human && welcomed == 0 {
			t.Errorf("race %d city %d monk: welcomed %d times in %d talks", s.Race, s.HumType, welcomed, 40*len(speakers))
		}
	}
}
