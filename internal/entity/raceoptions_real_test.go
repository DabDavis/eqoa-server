package entity

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// On a copy of a server's database (EQOA_REAL_DB): every race of racedefs and
// the froglok has options, its rows read from its own male body, and every body
// step but one is another body than the race's own (the satyr's female faun is
// the one step of her row). The control: the same rows keyed by a body no row of
// racedefs wears give no race any. EQOA_RACEOPTIONS_OUT=<file> writes the list a
// client is sent (OpcodeRaceOptionsData's body) there, for vanilla's eqoa dress.
func TestRealRaceOptions(t *testing.T) {
	path := os.Getenv("EQOA_REAL_DB")
	if path == "" {
		t.Skip("EQOA_REAL_DB names a copy of a server's database")
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() { SetRaceDefs(nil) })
	if err := LoadRaceDefs(db); err != nil {
		t.Fatal(err)
	}
	defs := RaceDefs()
	if len(defs) == 0 {
		t.Skip("this database has no custom race")
	}
	for _, d := range defs {
		rows := RaceOptions(d.ID)
		if len(rows) == 0 {
			t.Errorf("%s (%d, body %#x): no options", d.Name, d.ID, uint32(d.Body[0]))
			continue
		}
		t.Logf("%s (%d): %d rows", d.Name, d.ID, len(rows))
	}
	if len(RaceOptions(raceFroglok)) == 0 {
		t.Error("the froglok: no options")
	}
	if out := os.Getenv("EQOA_RACEOPTIONS_OUT"); out != "" {
		body := EncodeRaceOptions()
		if err := os.WriteFile(out, body, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d races, %d bytes", out, body[1], len(body))
	}
	// the control: no body of racedefs, no options
	SetRaceDefs([]database.RaceDefRow{{ID: 11, Name: "Nobody", WireRace: 9, BodyMale: 0x12345678}})
	if len(RaceOptions(11)) != 0 {
		t.Error("the control: a race whose body has no rows has options")
	}
}
