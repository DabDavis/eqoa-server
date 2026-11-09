package charmove

import (
	"io"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 1314: a transfer's gear comes in as its records' imported copies
// (70% of their stats), a new record's too, each record copied once over
// every transfer; the same plan as a GM's grant (not Imported) gives the
// records themselves. A dry run makes no copy.
func TestATransfersGearComesInAsItsImportedCopies(t *testing.T) {
	db, _ := migrateDB(t)
	p := testPlan()
	p.Imported = true
	p.Level, p.MasteryPoints, p.ClassSpells, p.FullVitals, p.Records = 0, 0, false, false, nil
	if _, err := Migrate(db, p, false, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var copies int
	if err := db.QueryRow(`SELECT count(*) FROM itempattern WHERE patternid >= 990001`).Scan(&copies); err != nil || copies != 0 {
		t.Fatalf("a dry run made %d copies (%v)", copies, err)
	}
	if _, err := Migrate(db, p, true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT i.patternid, p.itemname, p.AC, p.weapondamage, COALESCE(m.base, 0) FROM charinventory i
		JOIN itempattern p ON p.patternid = i.patternid LEFT JOIN itemimported m ON m.patternid = i.patternid
		WHERE i.serverid = 126 AND i.location = 1 ORDER BY i.itemid`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		pattern     int
		name        string
		ac, dmg, of int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.pattern, &r.name, &r.ac, &r.dmg, &r.of); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	rows.Close()
	if len(got) != 4 {
		t.Fatalf("given %+v, want the helm, two swords and the no-dachi", got)
	}
	if got[0].of != 6207 || got[0].ac != 91 || got[0].name != "Otherworldly Helm" {
		t.Errorf("the helm: %+v, want the copy of 6207 at AC 91", got[0])
	}
	if got[1].of != 1297 || got[2].pattern != got[1].pattern || got[1].dmg != 7 {
		t.Errorf("the swords: %+v %+v, want one copy of 1297 at damage 7 for both", got[1], got[2])
	}
	if got[3].of == 0 || got[3].name != "No-Dachi of Toxicity" || got[3].dmg != 552 {
		t.Errorf("the new record: %+v, want its copy at damage 552", got[3])
	}

	// the same items as a GM's grant: the records themselves
	grant := Plan{Character: "wanderer", Items: []Item{{Pattern: 6207}}}
	if _, err := Migrate(db, grant, true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var last int
	if err := db.QueryRow(`SELECT patternid FROM charinventory WHERE serverid = 126 ORDER BY itemid DESC LIMIT 1`).Scan(&last); err != nil || last != 6207 {
		t.Errorf("a grant's helm is record %d (%v), want 6207 itself", last, err)
	}
	m, err := database.ImportedMap(db)
	if err != nil || len(m) != 3 {
		t.Errorf("copies %v (%v), want the helm's, the sword's and the no-dachi's", m, err)
	}
}
