package gmweb

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
)

// checkCityIntroRow holds a cityintro row (handlers/cityintro.go) to what a
// client can show: each box at most 217 characters (CLIENT's ShowDialog), no
// '|' (a run of CLIENT's glyph codes kills a PS2's TextBox) or '%', a circle
// with a radius, race and class -1 (any) or a game id.
func checkCityIntroRow(row map[string]any) error {
	for _, col := range []string{"boxes", "yes_last", "no_last"} {
		text, _ := row[col].(string)
		if strings.ContainsAny(text, "|%") {
			return badRequest("%s holds no '|' or '%%'", col)
		}
		for _, l := range database.IntroLines(text) {
			if utf8.RuneCountInString(l) > 217 {
				return badRequest("%s: a box is at most 217 characters (%q)", col, l)
			}
		}
	}
	if v, ok := row["radius"]; ok && v != nil {
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || f <= 0 || f > 5000 {
			return badRequest("radius is above 0 and at most 5000")
		}
	}
	for _, col := range []string{"race", "class"} {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f < -1 || f > 255 {
			return badRequest("%s is -1 (any) or a game id", col)
		}
	}
	return nil
}
