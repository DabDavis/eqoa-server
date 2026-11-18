package gmweb

import (
	"fmt"
	"math"

	"github.com/eqoa/server/internal/database"
)

// liveFXLand reads table fxland again into the world (entity/fxland.go; the change is committed: the
// database is free): the next cast of a visual lands at its row's hit.
func (s *Server) liveFXLand() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	n, err := s.d.World.LoadFXLand(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	return fmt.Sprintf("at once: %d visuals' hit times; the next cast of each lands at its row's", n)
}

// checkFXLandRow holds an fxland row to a visual's DictID (a 32-bit number, not 0) and a land_ms of 0 to
// database.FXLandMaxMS. It runs on a change's whole row, and on an update of some columns on those.
func checkFXLandRow(row map[string]any) error {
	if v, ok := row["fxdict"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f <= 0 || f > math.MaxUint32 {
			return badRequest("fxdict is a visual's DictID (0x00000001 to 0xFFFFFFFF)")
		}
	}
	if v, ok := row["land_ms"]; ok {
		f, fok := asFloat(v)
		if !fok || !database.ValidFXLandMS(f) {
			return badRequest("land_ms is the milliseconds from the cast time's end to the hit, 0 to %d", database.FXLandMaxMS)
		}
	}
	return nil
}
