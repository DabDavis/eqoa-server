package gmweb

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
)

// livePlaceTexts reads the place texts again (entity/placetexts.go, table
// placetexts; the change is committed: the database is free).
func (s *Server) livePlaceTexts() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if _, err := s.d.World.LoadPlaceTexts(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	return "at once: within half a second, whoever walks in"
}

// rowNum is a row's number in col, def when the row leaves it out; ok false
// when it is not a number.
func rowNum(row map[string]any, col string, def float64) (float64, bool) {
	v, set := row[col]
	if !set || v == nil {
		return def, true
	}
	f, ok := asFloat(v)
	return f, ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

// checkPlaceTextRow holds a placetexts row to what the game can show
// (database.PlaceTextProblem).
func checkPlaceTextRow(row map[string]any) error {
	var p database.PlaceText
	nums := map[string]*float32{"x1": &p.X1, "x2": &p.X2, "z1": &p.Z1, "z2": &p.Z2, "y1": &p.Y1, "y2": &p.Y2}
	defs := map[string]float64{"y1": -100000, "y2": 100000}
	for col, dst := range nums {
		f, ok := rowNum(row, col, defs[col])
		if !ok {
			return badRequest("%s is a number", col)
		}
		*dst = float32(f)
	}
	world, ok := rowNum(row, "world", 0)
	if !ok || world != math.Trunc(world) {
		return badRequest("world is a whole number")
	}
	p.Text, _ = row["text"].(string)
	p.Answer = "Done"
	if a, set := row["answer"].(string); set {
		p.Answer = a
	}
	if why := database.PlaceTextProblem(int(world), p); why != "" {
		return badRequest("%s", why)
	}
	return nil
}

// checkLightSphereRow holds a lightspheres row to what a client can show and
// travel to: a line of at most 127 characters and an answer of 63, neither
// with '|' or '%'; worlds and facing in their ranges.
func checkLightSphereRow(row map[string]any) error {
	line, _ := row["line"].(string)
	answer, set := row["answer"].(string)
	if !set {
		answer = "Touch the light."
	}
	switch {
	case strings.TrimSpace(line) == "":
		return badRequest("line is the sphere's words")
	case strings.ContainsAny(line+answer, "|%"):
		return badRequest("line and answer hold no '|' or '%%'")
	case utf8.RuneCountInString(line) > 127:
		return badRequest("line is at most 127 characters (CLIENT's question box)")
	case utf8.RuneCountInString(answer) > 63 || strings.TrimSpace(answer) == "":
		return badRequest("answer is 1 to 63 characters")
	}
	for col, top := range map[string]float64{"world": 255, "to_world": 255, "to_facing": 255} {
		f, ok := rowNum(row, col, 0)
		if !ok || f != math.Trunc(f) || f < 0 || f > top {
			return badRequest("%s is a whole number 0-%d", col, int(top))
		}
	}
	for _, col := range []string{"x", "z", "to_x", "to_y", "to_z"} {
		if f, ok := rowNum(row, col, 0); !ok || math.Abs(f) > 200000 {
			return badRequest("%s is a number within 200000 of the middle", col)
		}
	}
	return nil
}
