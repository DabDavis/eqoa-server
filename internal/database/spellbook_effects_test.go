package database

import (
	"database/sql"
	"testing"
)

// A spelleffect made before effects (2026-11-01, the workbook's effects as
// JSON) gets the column at the next start, its rows kept and reading none;
// one written reads back whole. The control: a new database has it from the
// start, and a spell with no row reads nothing.
func TestSpellEffectGainsEffects(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE spelleffect (spellid INTEGER PRIMARY KEY, line TEXT NOT NULL,
		fxdict INTEGER NOT NULL, source TEXT NOT NULL);
		INSERT INTO spelleffect VALUES (7, 'Target, Decrease Hitpoints by 20', 3, 'eqoa-data')`); err != nil {
		t.Fatal(err)
	}
	if has, _ := hasColumn(raw, "spelleffect", "effects"); has {
		t.Fatal("the old table already has effects")
	}
	raw.Close()

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSpellbookStore(db)
	r, ok, err := store.SpellEffectRow(7)
	if err != nil || !ok || r.Line != "Target, Decrease Hitpoints by 20" || r.Effects != "" || r.FX != 3 {
		t.Fatalf("the old row after the start: %+v %v (%v)", r, ok, err)
	}
	const js = `{"v":1,"scope":"target","harmful":true,"effects":[{"kind":"hp","amount":-20}]}`
	if _, err := db.Exec(`UPDATE spelleffect SET effects = ? WHERE spellid = 7`, js); err != nil {
		t.Fatal(err)
	}
	if r, _, _ := store.SpellEffectRow(7); r.Effects != js {
		t.Errorf("effects read back as %q", r.Effects)
	}
	if line, fx, ok, err := store.SpellEffect(7); err != nil || !ok || line != "Target, Decrease Hitpoints by 20" || fx != 3 {
		t.Errorf("SpellEffect, unchanged: %q %d %v (%v)", line, fx, ok, err)
	}
	if js2, err := store.SpellEffectsJSON(7); err != nil || js2 != js {
		t.Errorf("SpellEffectsJSON: %q (%v)", js2, err)
	}
	if _, ok, err := store.SpellEffectRow(8); ok || err != nil {
		t.Errorf("a spell with no row reads one (%v)", err)
	}

	fresh, err := NewSQLite(t.TempDir() + "/new.db")
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if has, err := hasColumn(fresh.DB, "spelleffect", "effects"); err != nil || !has {
		t.Errorf("the control: a new database's spelleffect lacks effects (%v)", err)
	}
}
