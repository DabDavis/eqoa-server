package database

import (
	"database/sql"
	"testing"
)

// An itemeffect made before body and target_line (2026-11-01: the body an
// item's use shows, a thrown one's target's own share) gets them at the next
// start, its rows kept and reading none; written, they read back. The
// control: a new database has them from the start. Made-up items.
func TestItemEffectGainsBodyAndTargetLine(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE itemeffect (patternid INTEGER PRIMARY KEY, line TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '', icon INTEGER NOT NULL DEFAULT 0, breaks TEXT NOT NULL DEFAULT '',
		wear INTEGER NOT NULL DEFAULT 0, area REAL NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO itemeffect (patternid, line, name, area) VALUES (90001, 'Target, Decrease Hitpoints by 20', '', 10)`); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"body", "target_line"} {
		if has, _ := hasColumn(raw, "itemeffect", c); has {
			t.Fatalf("the old table already has %s", c)
		}
	}
	raw.Close()

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewInventoryStore(db)
	r, ok, err := store.ItemEffect(90001)
	if err != nil || !ok || r.Line != "Target, Decrease Hitpoints by 20" || r.Area != 10 || r.Body != 0 || r.TargetLine != "" {
		t.Fatalf("the old row after the start: %+v %v (%v)", r, ok, err)
	}
	if _, err := db.Exec(`UPDATE itemeffect SET body = ?, target_line = ? WHERE patternid = 90001`,
		int64(0xF1E2D3C4), "Target, Decrease Hitpoints by 15"); err != nil {
		t.Fatal(err)
	}
	r, _, _ = store.ItemEffect(90001)
	if r.Body != 0xF1E2D3C4 || r.TargetLine != "Target, Decrease Hitpoints by 15" {
		t.Errorf("read back: body %#x, target line %q", r.Body, r.TargetLine)
	}
	all, err := store.ItemEffects()
	if err != nil || len(all) != 1 || all[0].Body != 0xF1E2D3C4 {
		t.Errorf("every row: %+v (%v)", all, err)
	}

	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	for _, c := range []string{"body", "target_line"} {
		if has, err := hasColumn(fresh.DB, "itemeffect", c); err != nil || !has {
			t.Errorf("a new database lacks %s (%v)", c, err)
		}
	}
}
