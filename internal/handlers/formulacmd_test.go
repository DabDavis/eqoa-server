package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/formulamode"
)

// /formula: any GM sees each shard's mode; a switch is an admin's (the
// control: a GM of acctlevel 1 is refused and nothing changes); jibs is
// refused with no formula loaded, then switches at once with one; a player
// is answered as an unknown command.
func TestFormulaCommand(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	c.h.SetAccessLevels(levels{1: 2, 2: 0, 3: 1})
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "heirloom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Exec(`INSERT OR IGNORE INTO accountinfo (accountid, username) VALUES (1, 'dab'), (3, 'othergm')`)
	ctl, err := formulamode.New(db, c.gw, "heirloom", map[string]string{"live": ""}, entity.FormulaOff)
	if err != nil {
		t.Fatal(err)
	}
	SetFormulaControl(ctl, 2)
	t.Cleanup(func() { SetFormulaControl(nil, 0) })

	c.say(gs, "/formula")
	saysFly(t, gs, "heirloom (here): default (built-in damage); 0 formula(s) loaded")
	saysFly(t, gs, "live: default")
	gs.Queue().GetPending()

	c.say(gs, "/formula jibs heirloom")
	saysFly(t, gs, "no formula loaded on Heirloom")
	if c.gw.DamageFormulaMode() != entity.FormulaOff {
		t.Fatal("switched with no formula")
	}
	gs.Queue().GetPending()

	p := filepath.Join(t.TempDir(), "damage.formula")
	os.WriteFile(p, []byte("formulas:\n  npc_melee: base + 1\n"), 0o600) // made up
	c.gw.SetDamageFormulaFile(p)
	c.gw.SetDamageFormulaMode(entity.FormulaOff)

	os3, _ := c.join(3, "OtherGm", 25020, 15020, 0)
	os3.SetAccountID(3)
	c.say(os3, "#formula jibs heirloom")
	saysFly(t, os3, "a switch is an admin's")
	if c.gw.DamageFormulaMode() != entity.FormulaOff {
		t.Fatal("a GM below admin switched it")
	}

	c.say(gs, "#formula jibs heirloom")
	saysFly(t, gs, "Damage on Heirloom is now jibs")
	if c.gw.DamageFormulaMode() != entity.FormulaLive {
		t.Fatalf("mode %v, want live", c.gw.DamageFormulaMode())
	}
	gs.Queue().GetPending()
	c.say(gs, "/formula default live")
	saysFly(t, gs, "not configured")
	gs.Queue().GetPending()
	c.say(gs, "/formula default")
	if c.gw.DamageFormulaMode() != entity.FormulaOff {
		t.Errorf("/formula default (this shard): %v", c.gw.DamageFormulaMode())
	}

	c.say(ps, "/formula default heirloom")
	saysFly(t, ps, "Unrecognized command")
}
