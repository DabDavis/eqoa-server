package gmweb

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// liveSpellGates reads where the teleport spells send again
// (entity/gates.go, table spellgates; the change is committed: the database
// is free).
func (s *Server) liveSpellGates() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadSpellGates(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next cast of the spell uses it"
}

// checkSpellGateRow holds a spellgates row to what the game can send anyone
// by (database.CheckSpellGate): a spell's name, a kind of the three, a world
// of the client's table, a place of finite numbers, no negative radius. An
// insert names its spell; a column it leaves out takes the table's default.
func checkSpellGateRow(row map[string]any) error {
	spell, ok := row["spell"]
	if !ok || spell == nil || strings.TrimSpace(fmt.Sprint(spell)) == "" {
		return badRequest("spell is the spell's name")
	}
	g := database.SpellGate{Spell: fmt.Sprint(spell), Kind: database.GateSelf}
	if v, ok := row["kind"]; ok && v != nil {
		g.Kind = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	}
	world := 0
	if v, ok := row["world"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) {
			return badRequest("world is a whole number, 0 to 29")
		}
		world = int(f)
	}
	for _, col := range []string{"x", "y", "z", "radius"} {
		v, ok := row[col]
		if !ok {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
		if col == "radius" {
			g.Radius = float32(f)
		}
	}
	if err := database.CheckSpellGate(g, world); err != nil {
		return badRequest("%v", err)
	}
	return nil
}
