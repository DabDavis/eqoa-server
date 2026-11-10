package handlers

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A petition's spawn request filed as camp reports, and their review, on a
// real server's data (EQOA_REAL_DB; a copy is opened): at a spawn of the
// commonest creature of Tunaria, an add of three and a move of the
// petition's target. The adds copy rows of the name, the move names the
// target, none is stale, and the review of both over every NPC of the server
// takes well under a few seconds. Skipped without a database.
func TestRealCampfixPetitionAndReview(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "campfix.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	var n, id int
	var x, y, z float64
	if err := db.QueryRow(`SELECT npc_name, COUNT(*) FROM npcs WHERE world = 0 AND npc_name != '' GROUP BY lower(trim(npc_name))
		ORDER BY COUNT(*) DESC LIMIT 1`).Scan(&name, &n); err != nil {
		t.Skip("no NPC in Tunaria")
	}
	if err := db.QueryRow(`SELECT id, x_coord, y_coord, z_coord FROM npcs WHERE world = 0 AND lower(trim(npc_name)) = lower(trim(?))
		ORDER BY id LIMIT 1`, name).Scan(&id, &x, &y, &z); err != nil {
		t.Fatal(err)
	}
	gw := entity.NewWorld()
	gw.LoadNPCs(database.NewNPCStore(db))
	view := `{"filed":"2026-11-09T10:00:00Z","via":"chat","character":{"name":"Tester","serverid":1,"level":20,"class":"Warrior",` +
		`"world":0,"x":` + ftoa(x+20) + `,"y":` + ftoa(y) + `,"z":` + ftoa(z+20) + `,"facing":1.0},` +
		`"target":{"kind":"npc","name":"x","npcid":` + ftoa(float64(id)) + `}}`
	p, err := db.FilePetition(database.Petition{ServerID: 1, CharName: "Tester", AccountID: 1, Category: 1, Subcategory: 10,
		Text: "more of them here", ServerView: view}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	who := database.GMEdit{Username: "dab's sessions", Via: "cli", IP: "local"}
	var reports []database.CampfixReport
	for _, req := range []CampfixPetition{{Petition: p.ID, Kind: "add", Name: name, Count: 3}, {Petition: p.ID, Kind: "move", Name: name}} {
		plan, err := PlanCampfixPetition(db, req, time.Now())
		if err != nil {
			t.Fatalf("%s %s: %v", req.Kind, name, err)
		}
		r, _, err := plan.File(db, who, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		reports = append(reports, r)
	}
	start := time.Now()
	rv := NewCampfixReviewer(db, gw)
	for _, r := range reports {
		got, err := rv.Review(r)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range got.Marks {
			t.Logf("report %d mark %d: %s %s, %s: %v; plan %s", r.ID, m.N, m.Kind, m.Name, m.Colour, m.Reasons, m.Plan.Words)
			if m.Stale != "" {
				t.Errorf("mark %d stale: %s", m.N, m.Stale)
			}
			switch m.Kind {
			case "add":
				if m.Plan.Action != CampfixInsert || m.Plan.Template == 0 {
					t.Errorf("an add's plan: %+v", m.Plan)
				}
			case "move":
				if m.Plan.Action != CampfixUpdate || m.Plan.Row != id {
					t.Errorf("the move's plan: %+v, want row %d", m.Plan, id)
				}
			}
		}
	}
	took := time.Since(start)
	t.Logf("%s (%d spawns in Tunaria): two reports reviewed in %v", name, n, took)
	if took > 5*time.Second {
		t.Errorf("the review took %v", took)
	}
}

func ftoa(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
