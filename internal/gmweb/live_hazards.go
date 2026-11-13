package gmweb

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// liveHazards reads the hazards again (entity/hazards.go, table hazards; the change is committed:
// the database is free).
func (s *Server) liveHazards() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadHazards(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: within half a second, whoever stands in it"
}

// checkHazardRow holds a hazards row to what the game can hurt anyone by (database.CheckHazard). A
// column the row leaves out takes the table's default.
func checkHazardRow(row map[string]any) error {
	var h database.Hazard
	world := 0
	ints := map[string]*int{"world": &world, "damage": &h.Damage, "every_ms": &h.EveryMS}
	for col, dst := range ints {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || math.Abs(f) > 1e7 {
			return badRequest("%s is a whole number", col)
		}
		*dst = int(f)
	}
	nums := map[string]*float32{"x1": &h.X1, "y1": &h.Y1, "z1": &h.Z1, "x2": &h.X2, "y2": &h.Y2, "z2": &h.Z2,
		"radius": &h.Radius, "below": &h.Below, "above": &h.Above}
	for col, dst := range nums {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
		*dst = float32(f)
	}
	if v, ok := row["words"]; ok && v != nil {
		h.Words = strings.TrimSpace(fmt.Sprint(v))
	}
	if err := database.CheckHazard(h, world); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

// liveNoRoam reads the no-roam areas again (entity/noroam.go, table noroam).
func (s *Server) liveNoRoam() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadNoRoam(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: the next leg each wanderer chooses"
}

// checkNoRoamRow holds a noroam row to what the world can use (database.CheckNoRoam).
func checkNoRoamRow(row map[string]any) error {
	var a database.NoRoam
	world := 0
	if v, ok := row["world"]; ok && v != nil {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) {
			return badRequest("world is a whole number")
		}
		world = int(f)
	}
	for col, dst := range map[string]*float32{"x": &a.X, "z": &a.Z, "radius": &a.Radius} {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
		*dst = float32(f)
	}
	if err := database.CheckNoRoam(a, world); err != nil {
		return badRequest("%v", err)
	}
	return nil
}
