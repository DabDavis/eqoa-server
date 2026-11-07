package gmweb

import (
	"fmt"
	"math"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// liveRaceSizes reads the player races' sizes again (entity/racesize.go,
// table racesize; the change is committed: the database is free): every
// player in the world of the race takes the new size where it stands.
func (s *Server) liveRaceSizes() string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	n, err := s.d.World.LoadRaceSizes(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	return fmt.Sprintf("at once: %s; %d players in the world are drawn at their new size from the next tick, on every client "+
		"(a race's illusion cast after it takes it too)", s.d.World.RaceSizeSummary(), n)
}

// checkRaceSizeRow holds a racesize row to a race of the network enum or of
// racedefs, and a size the server sends (database.ValidRaceSize): a size of 0
// would make the race's players vanish, 40 fill a street.
func checkRaceSizeRow(row map[string]any) error {
	if v, ok := row["race"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f < 0 || (f > 10 && !entity.IsCustomRace(int(f))) {
			return badRequest("race is a race's number: 0 Human ... 3 Gnome ... 9 Ogre, 10 Froglok, or one of the custom races")
		}
	} else {
		return badRequest("race is the race's number (3 is Gnome)")
	}
	if v, ok := row["size"]; ok {
		f, fok := asFloat(v)
		if !fok || !database.ValidRaceSize(f) {
			return badRequest("size is the race's body size, %v to %v (1.0 its own; 0.9 a tenth smaller)",
				database.RaceSizeMin, database.RaceSizeMax)
		}
	}
	return nil
}

// checkRaceDefRow holds a racedefs row to one the game can use
// (entity.CheckRaceDef). It runs on a change's whole row, and on an update of
// some columns on those: what is absent was already checked.
func checkRaceDefRow(row map[string]any) error {
	id, name, wire := entity.FirstCustomRace, "x", 0
	if v, ok := row["id"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) {
			return badRequest("id is the race's number, %d to %d", entity.FirstCustomRace, entity.LastCustomRace)
		}
		id = int(f)
	}
	if v, ok := row["name"]; ok {
		name, _ = v.(string)
	}
	if v, ok := row["wire_race"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) {
			return badRequest("wire_race is one of the game's races, 0 Human ... 9 Ogre")
		}
		wire = int(f)
	}
	if why := entity.CheckRaceDef(id, name, wire); why != "" {
		return badRequest("%s", why)
	}
	// creation's columns (entity/creation.go), each where the change has it; the
	// city against the wire race only when both are there
	mask, city := 0, 0
	for col, dst := range map[string]*int{"class_mask": &mask, "city": &city} {
		if v, ok := row[col]; ok {
			f, fok := asFloat(v)
			if !fok || f != math.Trunc(f) {
				return badRequest("%s is a whole number", col)
			}
			*dst = int(f)
		}
	}
	if _, ok := row["wire_race"]; !ok {
		wire = 0
	}
	if why := entity.CheckRaceDefCreation(wire, mask, city); why != "" {
		return badRequest("%s", why)
	}
	return nil
}

// liveRaceDefs reads the custom races again (entity/race.go; the change is
// committed: the database is free) and tells the clients that show them.
func (s *Server) liveRaceDefs() string {
	if s.d.DB == nil {
		return "no world is running: at the next start"
	}
	if err := entity.LoadRaceDefs(s.d.DB); err != nil {
		return "not applied to the world: " + err.Error()
	}
	if s.d.World != nil {
		s.d.World.RefreshRaceLabels(s.d.DB)
	}
	msg := "at once: " + entity.RaceDefSummary() + "; a character's stats from its next login"
	var lost int
	if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM characters WHERE race > 10`).Scan(&lost); err == nil && lost > 0 {
		chars, err := s.d.DB.CharactersOfRaces(entity.CustomRaceIDs())
		if err == nil && len(chars) < lost {
			msg += fmt.Sprintf("; %d characters are of a race with no row, and go to the clients with no camera or items: move them", lost-len(chars))
		}
	}
	return msg
}

// refreshRaceLabels tells the clients that show custom races after a change to
// a character of one, or one made one (a GM's new name or race).
func (s *Server) refreshRaceLabels(before, after map[string]any) {
	custom := func(row map[string]any) bool {
		f, ok := asFloat(row["race"])
		return ok && entity.IsCustomRace(int(f))
	}
	if s.d.World != nil && s.d.DB != nil && (custom(before) || custom(after)) {
		s.d.World.RefreshRaceLabels(s.d.DB)
	}
}
