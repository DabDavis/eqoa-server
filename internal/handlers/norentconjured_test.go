package handlers

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// conjuredTool runs tools/norent_conjured_20261110.py on the database at path.
func conjuredTool(t *testing.T, path string, args ...string) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 for tools/norent_conjured_20261110.py")
	}
	cmd := exec.Command(py, append([]string{"-E", "-s", "../../tools/norent_conjured_20261110.py", "--db", path}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("norent_conjured_20261110.py %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// The owner's decision of 2026-11-10 on jib's PR #1, end to end: tools/norent_conjured_20261110.py gives the twenty
// conjured records a norentvanish row each (by name among the records of rent 0, at their old ids), and a login more
// than five minutes after the logout takes a held copy; a second --apply writes nothing, and --undo takes out exactly
// its own rows. The controls: the Unit of Blood stays, the game's own record of a conjured one's name (rent 1) stays,
// a row there before (another record's) is left by the undo, and a login within the five minutes keeps the copy.
func TestTheConjuredRecordsVanishFiveMinutesAway(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conjured.db")
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	record := func(id, rent int, name string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (?, 0, 0, 1, 0, -1, 0, 0, ?, 0, 0, 0, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`, id, rent, name); err != nil {
			t.Fatal(err)
		}
	}
	names := []string{"Phase Shield", "Lava Stone", "Aura Stone", "Phase Mallet", "Ice Shard", "Phase Ring", "Phase Gladius",
		"Fiery Pebble", "Phase Crown", "Burning Blade", "Frost Orb", "Scoria Stone", "Shadow Knife", "Mana Stone",
		"Unfiltered Water", "Weak Ale", "Citrus Mixture", "Ginger Root Ale", "Bitter Draft", "Fine Red Wine"}
	for i, n := range names {
		record(705298+i, 0, n)
	}
	record(709302, 0, "Unit of Blood")
	record(8456, 1, "Lava Stone") // the game's own, of a conjured one's name
	record(715011, 0, "Made-up Raid Bow")
	if _, err := db.Exec(`INSERT INTO norentvanish (patternid, note) VALUES (715011, 'there before')`); err != nil {
		t.Fatal(err)
	}
	give := func(itemID, serverID, pattern int) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, ?, 1, 100, 0, ?, -1, 0, ?)`, itemID, serverID, pattern, itemID); err != nil {
			t.Fatal(err)
		}
	}
	held := func(serverID, pattern int) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, serverID, pattern).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	vanishing := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM norentvanish`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if out := conjuredTool(t, path); !strings.Contains(out, "report only: 20 norentvanish rows") || vanishing() != 1 {
		t.Fatalf("the report (%d rows):\n%s", vanishing(), out)
	}
	if out := conjuredTool(t, path, "--apply"); !strings.Contains(out, "applied: 20 norentvanish rows") ||
		!strings.Contains(out, "Unit of Blood 709302: stays") {
		t.Fatalf("the apply said:\n%s", out)
	}
	if vanishing() != 21 {
		t.Fatalf("%d norentvanish rows after the apply, want the 20 and the one there before", vanishing())
	}
	if out := conjuredTool(t, path, "--apply"); !strings.Contains(out, "nothing to do") || vanishing() != 21 {
		t.Fatalf("a second apply (%d rows):\n%s", vanishing(), out)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'norentvanish' AND action = 'insert'`).Scan(&audits); err != nil || audits != 20 {
		t.Fatalf("%d gm_audit lines (%v), want 20", audits, err)
	}

	// a login six minutes after the logout takes the conjured copy and leaves the controls
	give(1, 31, 705298) // Phase Shield
	give(2, 31, 709302) // Unit of Blood
	give(3, 31, 8456)   // the game's own Lava Stone
	give(4, 32, 705312) // Unfiltered Water, its holder back within the five minutes
	now := time.Unix(1_800_000_000, 0)
	defer func(f func() time.Time) { noRentNow = f }(noRentNow)
	noRentNow = func() time.Time { return now }
	defer func() {
		vanishedMu.Lock()
		delete(vanished, 31)
		delete(vanished, 32)
		vanishedMu.Unlock()
	}()
	if err := db.SaveLogout(31, now.Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveLogout(32, now.Add(-3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), nil, nil, database.NewSpellbookStore(db))
	h.vanishNoRent(31)
	h.vanishNoRent(32)
	if held(31, 705298) {
		t.Error("the Phase Shield stayed through a login six minutes after the logout")
	}
	if !held(31, 709302) || !held(31, 8456) {
		t.Errorf("the controls went: Unit of Blood %v, the game's Lava Stone %v", held(31, 709302), held(31, 8456))
	}
	if !held(32, 705312) {
		t.Error("the control: a login three minutes after the logout took the drink")
	}
	db.Close()

	if out := conjuredTool(t, path, "--undo"); !strings.Contains(out, "took out 20 rows, 0 left") {
		t.Fatalf("the undo said:\n%s", out)
	}
	db, err = database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var left int
	var note string
	if err := db.QueryRow(`SELECT COUNT(*), MAX(note) FROM norentvanish`).Scan(&left, &note); err != nil || left != 1 || note != "there before" {
		t.Errorf("after the undo: %d rows (%q, %v), want the one there before alone", left, note, err)
	}
}
