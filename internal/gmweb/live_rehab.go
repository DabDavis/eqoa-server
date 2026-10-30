package gmweb

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// The rehabilitators in the panel (handlers/rehab.go): their prices and the
// free resets every character has (table rehabsettings, a row a setting) and
// each character's counts (charresets, a row a kind), so a GM sees and sets
// them as any row is changed, audited and revertible. The game reads both at
// each talk to a rehabilitator, so a change needs nothing done to the running
// world.

// rehabSettingHelp is rehabsettings.setting's help: every setting, its
// default and what it is.
func rehabSettingHelp() string {
	parts := make([]string, len(database.RehabSettingRows))
	for i, r := range database.RehabSettingRows {
		parts[i] = fmt.Sprintf("%s (default %d): %s", r.Name, r.Default, r.Note)
	}
	return "one of: " + strings.Join(parts, "; ")
}

// wholeAtLeast0 is a column's value as a whole number from 0 to most, or why
// it is not one; a column the row leaves out is fine.
func wholeAtLeast0(row map[string]any, col string, most float64) error {
	v, present := row[col]
	if !present || v == nil {
		return nil
	}
	n, ok := asFloat(v)
	if !ok || n != math.Trunc(n) || n < 0 || n > most {
		return badRequest("%s is a whole number from 0 to %.0f", col, most)
	}
	return nil
}

// checkCharResetRow holds a charresets row to a character, a kind the game
// counts (mastery, training, path, as spelled) and counts of 0 or more.
func checkCharResetRow(row map[string]any) error {
	id, ok := asFloat(row["serverid"])
	if !ok || id != math.Trunc(id) || id <= 0 {
		return badRequest("serverid is the character's (characters.serverid)")
	}
	kind, _ := row["kind"].(string)
	known := false
	for _, k := range database.ResetKinds {
		known = known || k == kind
	}
	if !known {
		return badRequest("kind %q is none the game counts: one of %s", kind, strings.Join(database.ResetKinds, ", "))
	}
	for _, col := range []string{"used", "paid", "bonus"} {
		if err := wholeAtLeast0(row, col, database.MaxRehabValue); err != nil {
			return err
		}
	}
	return nil
}

// checkRehabSettingRow holds a rehabsettings row to a setting the game reads,
// spelled as it is, and a value a purse can pay.
func checkRehabSettingRow(row map[string]any) error {
	name, _ := row["setting"].(string)
	known := false
	for _, n := range database.RehabSettingNames() {
		if n == name {
			known = true
		} else if strings.EqualFold(strings.TrimSpace(name), n) {
			return badRequest("setting %q is spelled %q", name, n)
		}
	}
	if !known {
		return badRequest("setting %q is none the game reads: one of %s", name, strings.Join(database.RehabSettingNames(), ", "))
	}
	if v, present := row["value"]; present && v == nil {
		return badRequest("value is a number")
	}
	return wholeAtLeast0(row, "value", database.MaxRehabValue)
}
