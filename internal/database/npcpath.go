package database

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// npcpaths: the paths GMs give NPCs (the GM panel's map, entity/path.go). A
// row an NPC, by its npcs row, so a path outlives the creature's deaths and a
// restart; its waypoints are one JSON list, so a whole path is one row change
// in the panel's audit log and one revert puts it back.

// The modes a path is walked in.
const (
	PathModeLoop     = "loop"     // the last point back to the first, round and round
	PathModePingPong = "pingpong" // to the last point and back the same way
	PathModeOnce     = "once"     // to the last point, then it stands there
)

// The paces a path is walked at.
const (
	PathSpeedWalk = "walk"
	PathSpeedRun  = "run"
)

// The limits a path is held to: a point list a GM draws, not a dump.
const (
	MaxPathPoints = 256
	MaxPathPause  = 600 // seconds
	maxPathCoord  = 1e6
)

// PathPoint is one waypoint: where in the path's world, and how long the NPC
// stands there (nil: the path's own pause).
type PathPoint struct {
	X     float64  `json:"x"`
	Y     float64  `json:"y"`
	Z     float64  `json:"z"`
	Pause *float64 `json:"pause,omitempty"`
}

// NPCPathRecord is a row of npcpaths.
type NPCPathRecord struct {
	NPCID  int
	World  int
	Mode   string
	Pause  float64
	Speed  string
	Points []PathPoint
}

// PathMode is a mode as stored, from its name or an alias a GM types ("back
// and forth", "patrol"), or false.
func PathMode(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "loop", "circuit", "round":
		return PathModeLoop, true
	case "pingpong", "ping-pong", "back and forth", "back-and-forth", "backandforth", "patrol":
		return PathModePingPong, true
	case "once", "once then hold", "hold":
		return PathModeOnce, true
	}
	return "", false
}

// The pace a path may give in units a second, beside walk and run: a patrol
// as Sandstorm's walks it (tools/seed_npc_patrols.py: its pikers pace at
// 3.12, under the walk's 5).
const (
	MinPathPace = 0.5
	MaxPathPace = 30
)

// PathSpeed is a pace as stored, or false: walk, run, or units a second
// (MinPathPace..MaxPathPace) as their number.
func PathSpeed(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "walk":
		return PathSpeedWalk, true
	case "run":
		return PathSpeedRun, true
	}
	if v, ok := PathPace(s); ok {
		return strconv.FormatFloat(v, 'f', -1, 64), true
	}
	return "", false
}

// PathPace is a pace given in units a second, false for walk, run or
// anything else.
func PathPace(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || v < MinPathPace || v > MaxPathPace {
		return 0, false
	}
	return v, true
}

// ParsePathPoints reads a points column.
func ParsePathPoints(text string) ([]PathPoint, error) {
	var pts []PathPoint
	if err := json.Unmarshal([]byte(text), &pts); err != nil {
		return nil, fmt.Errorf("points is not a JSON list of {x, y, z[, pause]}: %v", err)
	}
	return pts, nil
}

// PathPointsJSON is the points as the column keeps them.
func PathPointsJSON(pts []PathPoint) string {
	if pts == nil {
		pts = []PathPoint{}
	}
	b, _ := json.Marshal(pts)
	return string(b)
}

// Check is what a path must be to be walked: a known mode and pace, a pause
// and every point's within MaxPathPause, 1 to MaxPathPoints points at
// finite places.
func (r *NPCPathRecord) Check() error {
	if _, ok := PathMode(r.Mode); !ok {
		return fmt.Errorf("mode %q is not loop, pingpong (back and forth) or once", r.Mode)
	}
	if _, ok := PathSpeed(r.Speed); !ok {
		return fmt.Errorf("speed %q is not walk or run", r.Speed)
	}
	if r.World < 0 || r.World > 255 {
		return fmt.Errorf("world %d is not a world", r.World)
	}
	if !(r.Pause >= 0 && r.Pause <= MaxPathPause) {
		return fmt.Errorf("pause %v is not 0 to %d seconds", r.Pause, MaxPathPause)
	}
	if len(r.Points) == 0 {
		return fmt.Errorf("a path needs a point at least")
	}
	if len(r.Points) > MaxPathPoints {
		return fmt.Errorf("%d points: a path holds at most %d", len(r.Points), MaxPathPoints)
	}
	for i, p := range r.Points {
		for _, v := range []float64{p.X, p.Y, p.Z} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > maxPathCoord {
				return fmt.Errorf("point %d is not a place (%v, %v, %v)", i+1, p.X, p.Y, p.Z)
			}
		}
		if p.Pause != nil && !(*p.Pause >= 0 && *p.Pause <= MaxPathPause) {
			return fmt.Errorf("point %d's pause %v is not 0 to %d seconds", i+1, *p.Pause, MaxPathPause)
		}
	}
	return nil
}

// NPCPathStore reads npcpaths.
type NPCPathStore struct {
	db *SQLiteDB
}

// NewNPCPathStore creates a path store.
func NewNPCPathStore(db *SQLiteDB) *NPCPathStore {
	return &NPCPathStore{db: db}
}

// All is every path, by NPC; a row that does not parse comes back with its
// error beside it, the others still read.
func (s *NPCPathStore) All() ([]NPCPathRecord, []error, error) {
	rows, err := s.db.Query(`SELECT npcid, world, mode, pause, speed, points FROM npcpaths ORDER BY npcid`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []NPCPathRecord
	var bad []error
	for rows.Next() {
		var r NPCPathRecord
		var points string
		if err := rows.Scan(&r.NPCID, &r.World, &r.Mode, &r.Pause, &r.Speed, &points); err != nil {
			return nil, nil, err
		}
		pts, err := ParsePathPoints(points)
		if err == nil {
			r.Points = pts
			err = r.Check()
		}
		if err != nil {
			bad = append(bad, fmt.Errorf("NPC %d's path: %w", r.NPCID, err))
			continue
		}
		out = append(out, r)
	}
	return out, bad, rows.Err()
}
