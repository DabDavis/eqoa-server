package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A merc's name is the world's (petition 2409: a player's "Arthas" and
// Photon's, and vanilla's merc list keys its mercs by name). A new hire passes
// over every other character's hires and every character; at its owner's
// login a hire named as another's EARLIER hire, or as a character, is named
// again, and the earlier hire keeps its name.
func TestMercNamesAreTheWorlds(t *testing.T) {
	r, m, db := newMercFixture(t)
	early := time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	// another character's hire took "Arthas" first; a character is "Belion"
	if err := db.SetMerc(database.MercRow{ServerID: 9001, Slot: 1, Name: "Arthas", Class: 10, Race: 1, Stance: 2, Hired: early}); err != nil {
		t.Fatal(err)
	}
	insertCharacter(t, db, 9002, "Belion")
	held, err := db.MercNamesElsewhere(r.pa.CharServerID)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := held["arthas"]; !ok || !got.Equal(early) {
		t.Fatalf("arthas held: %v %v", got, ok)
	}
	if _, ok := held["belion"]; !ok {
		t.Fatal("a character's name is not held")
	}
	// the player's own: "Arthas" later than the other's, "Belion" a character's, "Calbon" free
	for slot, name := range map[int]string{1: "Arthas", 2: "Belion", 3: "Calbon"} {
		if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Slot: slot, Name: name, Class: 9, Race: 0, Stance: 2, Hired: late}); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := db.Mercs(r.pa.CharServerID)
	rows = m.distinctNames(r.pa, rows)
	got := map[int]string{}
	for _, row := range rows {
		got[row.Slot] = row.Name
	}
	if strings.EqualFold(got[1], "Arthas") || strings.EqualFold(got[2], "Belion") || got[3] != "Calbon" {
		t.Fatalf("after the login: %v", got)
	}
	if other, _ := db.Mercs(9001); len(other) != 1 || other[0].Name != "Arthas" {
		t.Fatalf("the earlier hire lost its name: %+v", other)
	}
	// the earlier hire's owner logging in keeps it: the later hire is renamed already, and was later
	// a new hire passes over the world's names
	taken := m.namesTaken(r.pa, rows)
	for _, n := range []string{"arthas", "belion"} {
		found := false
		for _, t := range taken {
			found = found || strings.EqualFold(t, n)
		}
		if !found {
			t.Errorf("%s is not taken for a new hire", n)
		}
	}
}

// With every two-part name taken, mercName finds a three-part one.
func TestMercNameThirdPart(t *testing.T) {
	var all []string
	for _, p := range namePrefixes {
		for _, s := range nameSuffixes {
			all = append(all, p+s)
		}
	}
	at := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	n := mercName(44, 9, 3, at, all...)
	for _, a := range all {
		if strings.EqualFold(a, n) {
			t.Fatalf("%q is taken", n)
		}
	}
	if len(n) < 5 {
		t.Fatalf("name %q", n)
	}
}

// insertCharacter adds a characters row of that serverid and name, every
// other column NOT NULL without a default given a zero.
func insertCharacter(t *testing.T, db *database.SQLiteDB, serverID int, name string) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(characters)`)
	if err != nil {
		t.Fatal(err)
	}
	cols, vals := []string{"serverid", "charName"}, []any{serverID, name}
	for rows.Next() {
		var cid, notnull, pk int
		var col, typ string
		var dflt any
		if err := rows.Scan(&cid, &col, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if notnull == 1 && dflt == nil && col != "serverid" && col != "charName" {
			cols = append(cols, col)
			if strings.Contains(strings.ToUpper(typ), "TEXT") {
				vals = append(vals, "")
			} else {
				vals = append(vals, 0)
			}
		}
	}
	rows.Close()
	q := "INSERT INTO characters (" + strings.Join(cols, ", ") + ") VALUES (?" + strings.Repeat(", ?", len(cols)-1) + ")"
	if _, err := db.Exec(q, vals...); err != nil {
		t.Fatal(err)
	}
}
