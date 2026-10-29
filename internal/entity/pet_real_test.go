package entity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Every summoning spell a class is given on a real server (EQOA_REAL_DB names
// its database; a copy is read) finds a body for that class among the
// server's NPCs, as petTemplateLocked looks (its name, its line's creature,
// the class's stand-in): the test lanes' report of 2026-10-29 found six of
// the wizard's familiars and guardians and the necromancer's Wilting Flesh
// making nothing. Skipped without a database: none is in this repository.
func TestEveryClassPetFindsABody(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realWorld(t)
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pets.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT c.class, p.spellname, e.line FROM classspells c
		JOIN spellpattern p ON p.spellid = c.spellid JOIN spelleffect e ON e.spellid = c.spellid ORDER BY c.class, p.abilitylvl`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var class int
		var name, line string
		if err := rows.Scan(&class, &name, &line); err != nil {
			t.Fatal(err)
		}
		d := ParseSpellText(line)
		if !d.Summons() {
			continue
		}
		n++
		w.mu.Lock()
		body := w.petTemplateLocked(name, d.Pet, class)
		w.mu.Unlock()
		if body == nil {
			t.Errorf("class %d's %q (%q): no body", class, name, line)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d class summoning spells, each with a body", n)
}
