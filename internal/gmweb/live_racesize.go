package gmweb

import (
	"fmt"
	"math"

	"github.com/eqoa/server/internal/database"
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

// checkRaceSizeRow holds a racesize row to a race of the network enum and a
// size the server sends (database.ValidRaceSize): a size of 0 would make the
// race's players vanish, 40 fill a street.
func checkRaceSizeRow(row map[string]any) error {
	if v, ok := row["race"]; ok {
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || f < 0 || f > 10 {
			return badRequest("race is a race's number: 0 Human ... 3 Gnome ... 9 Ogre, 10 Froglok")
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
