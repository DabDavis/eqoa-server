package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// racesize.go — a player's body size by its race (table racesize; jib's F3
// 20261029-212125-e7ff987a: "Shrink player gnome size by about 10%"). It is
// the state channel's ModelSize (bytes 36-39, Player.Body), so every client
// draws it: the player's own (channel 0) and everyone who sees it. 1.0 is the
// race's own body as CHAR.ESF authors it, which Sandstorm sends for every
// player (PlayerSize); a race with no row is that. A race's illusion takes
// that race's size, so a glamoured human is a gnome player's height. NPCs
// keep their own rows' sizes (NPCSize).

// SetRaceSizes sets each player race's size (table racesize, by race on the
// network enum; one outside database.ValidRaceSize is dropped and the race
// drawn at PlayerSize), and puts it on every player in the world now, whose
// clients draw it from the next tick. It returns how many players changed.
func (w *World) SetRaceSizes(sizes map[int]float64) (changed int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.raceSizes = make(map[int]float32, len(sizes))
	for race, s := range sizes {
		if !database.ValidRaceSize(s) {
			log.Printf("racesize: %s's size %v is not between %v and %v: drawn at %.2f", raceName(race), s,
				database.RaceSizeMin, database.RaceSizeMax, PlayerSize)
			continue
		}
		w.raceSizes[race] = float32(s)
	}
	for _, p := range w.Players {
		if s := w.raceSizeLocked(p.Race); p.Size != s {
			p.Size = s
			changed++
		}
	}
	return changed
}

// LoadRaceSizes reads table racesize into the world (SetRaceSizes): at the
// start and after a GM's edit. Call it without the database held.
func (w *World) LoadRaceSizes(db *database.SQLiteDB) (changed int, err error) {
	sizes, err := db.RaceSizes()
	if err != nil {
		return 0, fmt.Errorf("racesize: %w", err)
	}
	return w.SetRaceSizes(sizes), nil
}

// RaceSize is a player race's body size.
func (w *World) RaceSize(race int) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.raceSizeLocked(race)
}

// raceSizeLocked is a player race's body size. Must be called with the lock
// held.
func (w *World) raceSizeLocked(race int) float32 {
	if s, ok := w.raceSizes[race]; ok {
		return s
	}
	return PlayerSize
}

// RaceSizeSummary is the sizes in a line, for the start and the GM panel:
// each race not at PlayerSize, then the rest.
func (w *World) RaceSizeSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var races []int
	for race, s := range w.raceSizes {
		if s != PlayerSize {
			races = append(races, race)
		}
	}
	sort.Ints(races)
	var parts []string
	for _, race := range races {
		parts = append(parts, fmt.Sprintf("%s %.2f", raceName(race), w.raceSizes[race]))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("every race at %.2f (racesize has no other)", PlayerSize)
	}
	return fmt.Sprintf("%s, every other race at %.2f", strings.Join(parts, ", "), PlayerSize)
}

func raceName(race int) string {
	if race >= 0 && race < len(raceNames) {
		return raceNames[race]
	}
	return fmt.Sprintf("race %d", race)
}
