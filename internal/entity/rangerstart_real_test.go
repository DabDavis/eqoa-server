package entity

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Over a real database after tools/ranger_start_20261127.py --apply
// (EQOA_RANGER=1 and EQOA_REAL_DB naming a copy): a new ranger of every race
// starts with nothing there attacking it on sight (StartThreats, the server's
// own start check), the glade's people show none of them a hostile or wary
// face, and they welcome a ranger of another race than the humans (npcdialog
// town Surefall Glade, class 1 not_race 0), never a human ranger.
func TestRangerStartRealDB(t *testing.T) {
	if os.Getenv("EQOA_RANGER") == "" || os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_RANGER=1 and EQOA_REAL_DB naming a database after tools/ranger_start_20261127.py --apply")
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
	var rangers []database.StartSpot
	for _, s := range spots {
		if s.Class == 1 {
			rangers = append(rangers, s)
		}
	}
	if len(rangers) == 0 {
		t.Fatal("no ranger start rows")
	}
	g := rangers[0]
	for _, s := range rangers {
		if s.Race == 0 && s.HumType == 2 {
			g = s
		}
	}
	rangers = append(rangers, database.StartSpot{Race: raceFroglok, Class: 1, HumType: 1, World: g.World, X: g.X, Y: g.Y, Z: g.Z})
	for _, th := range w.StartThreats(rangers) {
		t.Errorf("a new ranger is attacked at its start: %+v", th)
	}
	lines, err := db.NPCDialog()
	if err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	var speakers []*NPC
	for _, npc := range w.NPCs {
		if npc.World == 0 && distXZ(npc.SpawnX, npc.SpawnZ, g.X, g.Z) <= 80 && w.npcFactionLocked(npc) != "" {
			speakers = append(speakers, npc)
		}
	}
	w.mu.Unlock()
	if len(speakers) == 0 {
		t.Fatal("no one of a faction in the glade")
	}
	for _, s := range rangers {
		if s.Race == 1 || s.World != g.World || s.X != g.X || s.Z != g.Z {
			continue // the elves' own start
		}
		p := &Player{ServerID: 0x7FFFFFF0, Name: "Ix", Level: 1, Race: s.Race, Class: 1, HumType: s.HumType}
		welcomed := 0
		for _, npc := range speakers {
			w.mu.Lock()
			mood := w.dialogMoodLocked(p, npc)
			w.mu.Unlock()
			if mood == MoodHostile || mood == MoodWary {
				t.Errorf("race %d ranger: %s is %s", s.Race, npc.Name, mood)
			}
			for i := 0; i < 40; i++ {
				if _, line, id, ok := w.TownLine(p, npc, lines, 0); ok {
					for _, l := range lines {
						if l.ID == id && l.ForClass {
							welcomed++
							if i == 0 && welcomed == 1 {
								t.Logf("race %d ranger hears %s: %q", s.Race, npc.Name, line)
							}
						}
					}
				}
			}
		}
		human := s.Race == 0
		if human && welcomed > 0 || !human && welcomed == 0 {
			t.Errorf("race %d city %d ranger: welcomed %d times in %d talks", s.Race, s.HumType, welcomed, 40*len(speakers))
		}
	}
}
