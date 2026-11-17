package database

import (
	"encoding/json"
	"fmt"
	"strings"
)

// npctours and npcwalkon: an NPC's tour of a place for anyone aboard its vehicle,
// and the NPCs a body walks onto or through (the owner, 2026-11-17: "lets have
// santa claus give a scenic tour of freeport for anyone on the sled, he announces
// it in chat. he will initiate once a player talks to him and chooses to start",
// "the sled with the pegasus and santa will altogether fly together", "let players
// run onto the sled without jumping"). Not retail.
//
// A tour (entity/npctour.go) is a host (Santa Claus) who offers it when talked
// to, a vehicle (Santa's Sleigh) the riders stand on, and the members that fly
// with it as one body (the team, the host himself), each kept where it stands
// from the vehicle as at home and turned with it. Its route is a list of
// waypoints in its world, flown at its speed; a point may hold a line the host
// says to the riders there, a pause, and a view: where a client that cannot ride
// (a PS2) is set down to watch that stretch. The riders stand on the vehicle's
// deck (deck: its places in the vehicle's frame, x right, y up, z ahead, as a
// light string's), and are set down at drop when it lands, as is anyone who
// leaves it on the way.
//
// npcwalkon: an NPC (by name) whose body a player's walk does not stop at:
// "deck" walked into and stood on (a vehicle's deck, radius across and height
// over its feet, which the movement check takes as a floor), or "pass" walked
// through (a team's horses). A client that walks (vanilla) is told the names
// with the decorations (OpcodeDecorLights, layout 2); a PS2 is not, and still
// stops at them.
const npcToursSchema = `
	CREATE TABLE IF NOT EXISTS npctours (
		name      TEXT PRIMARY KEY COLLATE NOCASE,
		host      TEXT NOT NULL DEFAULT '',
		vehicle   TEXT NOT NULL DEFAULT '',
		members   TEXT NOT NULL DEFAULT '[]',
		world     INTEGER NOT NULL DEFAULT 0,
		route     TEXT NOT NULL DEFAULT '[]',
		deck      TEXT NOT NULL DEFAULT '[]',
		drop_at   TEXT NOT NULL DEFAULT '[]',
		speed     REAL NOT NULL DEFAULT 10,
		countdown REAL NOT NULL DEFAULT 30,
		cooldown  REAL NOT NULL DEFAULT 120,
		board     REAL NOT NULL DEFAULT 6,
		lines     TEXT NOT NULL DEFAULT '{}',
		season    TEXT NOT NULL DEFAULT '',
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS npcwalkon (
		name   TEXT PRIMARY KEY COLLATE NOCASE,
		mode   TEXT NOT NULL DEFAULT 'deck',
		radius REAL NOT NULL DEFAULT 3,
		height REAL NOT NULL DEFAULT 1,
		season TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCTours makes tables npctours and npcwalkon (initSchema).
func (db *SQLiteDB) ensureNPCTours() error {
	if _, err := db.Exec(npcToursSchema); err != nil {
		return fmt.Errorf("npctours: %w", err)
	}
	return nil
}

// TourPoint is a waypoint of a tour's route: a place, the host's line there (""
// none), how long the vehicle hangs there (seconds), and where a rider whose
// client cannot ride is set down to watch (View nil: not moved).
type TourPoint struct {
	X     float32     `json:"x"`
	Y     float32     `json:"y"`
	Z     float32     `json:"z"`
	Say   string      `json:"say,omitempty"`
	Pause float32     `json:"pause,omitempty"`
	View  *[3]float32 `json:"view,omitempty"`
}

// TourLines is what a tour's host and others say, each overriding the code's
// default where set (entity/npctour.go's tourDefaults).
type TourLines struct {
	Offer     string `json:"offer,omitempty"`     // the talk's question
	Start     string `json:"start,omitempty"`     // the answer that starts it (or boards)
	NotNow    string `json:"notnow,omitempty"`    // the answer that does not
	Announce  string `json:"announce,omitempty"`  // shouted when one starts (%d: the countdown's seconds)
	Countdown string `json:"countdown,omitempty"` // shouted near the end of it (%d: the seconds left)
	Aboard    string `json:"aboard,omitempty"`    // to one who climbs aboard by the talk
	Depart    string `json:"depart,omitempty"`    // to the riders as it leaves
	Land      string `json:"land,omitempty"`      // to the riders as it lands
	Empty     string `json:"empty,omitempty"`     // shouted when it leaves with no one aboard
	Away      string `json:"away,omitempty"`      // the host's answer while out (%d: seconds to the next)
	Cooldown  string `json:"cooldown,omitempty"`  // the host's answer between tours (%d: seconds to the next)
	Off       string `json:"off,omitempty"`       // to a rider set down before the end
	Stand     string `json:"stand,omitempty"`     // the NPC who answers for the host while it is out
	StandLine string `json:"standline,omitempty"` // and what it says (%d: seconds to the next)
}

// NPCTour is a row of npctours.
type NPCTour struct {
	Name      string
	Host      string
	Vehicle   string
	Members   []string
	World     uint8
	Route     []TourPoint
	Deck      [][3]float32
	Drop      []float32 // x, y, z[, facing (radians)]
	Speed     float32
	Countdown float32
	Cooldown  float32
	Board     float32
	Lines     TourLines
	Season    string
}

// Check says what makes a tour unflyable ("" nothing).
func (t *NPCTour) Check() string {
	switch {
	case strings.TrimSpace(t.Host) == "" || strings.TrimSpace(t.Vehicle) == "":
		return "no host or vehicle"
	case len(t.Route) < 2:
		return "a route needs two points at least"
	case len(t.Deck) == 0:
		return "no place on the deck"
	case len(t.Drop) < 3:
		return "no drop point"
	case t.Speed <= 0 || t.Speed > 20:
		return "speed outside (0, 20]: the entity channel's velocity tops out at 20.8"
	}
	return ""
}

// NPCTours is every row of npctours, each with what makes it unflyable ("" none).
func (db *SQLiteDB) NPCTours() ([]NPCTour, []string, error) {
	rows, err := db.Query(`SELECT name, host, vehicle, members, world, route, deck, drop_at, speed, countdown, cooldown, board, lines, season FROM npctours ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []NPCTour
	var problems []string
	for rows.Next() {
		var t NPCTour
		var members, route, deck, drop, lines string
		var world int
		if err := rows.Scan(&t.Name, &t.Host, &t.Vehicle, &members, &world, &route, &deck, &drop, &t.Speed, &t.Countdown, &t.Cooldown, &t.Board, &lines, &t.Season); err != nil {
			return nil, nil, err
		}
		t.World = uint8(world)
		bad := ""
		for _, f := range []struct {
			col string
			s   string
			v   any
		}{{"members", members, &t.Members}, {"route", route, &t.Route}, {"deck", deck, &t.Deck}, {"drop_at", drop, &t.Drop}, {"lines", lines, &t.Lines}} {
			if strings.TrimSpace(f.s) == "" {
				continue
			}
			if err := json.Unmarshal([]byte(f.s), f.v); err != nil && bad == "" {
				bad = fmt.Sprintf("%s: %v", f.col, err)
			}
		}
		if bad == "" {
			bad = t.Check()
		}
		if bad != "" {
			problems = append(problems, fmt.Sprintf("tour %q: %s", t.Name, bad))
			continue
		}
		out = append(out, t)
	}
	return out, problems, rows.Err()
}

// The walk-ons' modes (npcwalkon.mode).
const (
	WalkOnDeck = "deck" // walked into and stood on
	WalkOnPass = "pass" // walked through
)

// NPCWalkOn is a row of npcwalkon, its name keyed (NPCClassKey).
type NPCWalkOn struct {
	Name   string
	Mode   string
	Radius float32
	Height float32
	Season string
}

// NPCWalkOns is every row of npcwalkon of a mode it knows.
func (db *SQLiteDB) NPCWalkOns() ([]NPCWalkOn, error) {
	rows, err := db.Query(`SELECT name, mode, radius, height, season FROM npcwalkon ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCWalkOn
	for rows.Next() {
		var w NPCWalkOn
		if err := rows.Scan(&w.Name, &w.Mode, &w.Radius, &w.Height, &w.Season); err != nil {
			return nil, err
		}
		w.Name, w.Mode = NPCClassKey(w.Name), strings.ToLower(strings.TrimSpace(w.Mode))
		if w.Name != "" && (w.Mode == WalkOnDeck || w.Mode == WalkOnPass) {
			out = append(out, w)
		}
	}
	return out, rows.Err()
}
