package gmweb

import (
	"math"

	"github.com/eqoa/server/internal/database"
)

// liveOrbs reads the teleport orbs again (entity/orbs.go, table orbs; the
// change is committed: the database is free).
func (s *Server) liveOrbs() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadOrbs(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next step into the orb uses it"
}

// checkOrbRow holds an orbs row to what the game can send anyone by
// (database.CheckOrb): worlds of the client's table, places of finite
// numbers, a radius of 0 to database.OrbRadiusMax, a destination outside its
// own orb. A column the row leaves out takes the table's default (0).
func checkOrbRow(row map[string]any) error {
	var o database.Orb
	world, toWorld := 0, 0
	for _, col := range []string{"world", "to_world"} {
		v, ok := row[col]
		if !ok {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) {
			return badRequest("%s is a whole number, 0 to 29", col)
		}
		if col == "world" {
			world = int(f)
		} else {
			toWorld = int(f)
		}
	}
	nums := map[string]*float32{"x": &o.X, "y": &o.Y, "z": &o.Z, "radius": &o.Radius,
		"to_x": &o.ToX, "to_y": &o.ToY, "to_z": &o.ToZ, "to_facing": &o.ToFacing}
	for col, dst := range nums {
		v, ok := row[col]
		if !ok {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
		*dst = float32(f)
	}
	if err := database.CheckOrb(o, world, toWorld); err != nil {
		return badRequest("%v", err)
	}
	return nil
}
