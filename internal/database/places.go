package database

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// lightspheres: teleporting spheres known by their place, not a name
// (handlers/lightsphere.go; work board 773, Highbourne's library's spheres of
// light, nameless on the live game): an NPC standing within LightSphereNear of
// (x, z) in world says line in CLIENT's question box with the one answer
// answer, which sends the player in front of (to_x, to_y, to_z), facing the
// way to_facing (0-255, as npcs.facing) looks back at it.
//
// placetexts: a box (answer "Done") opened once each time a player walks into
// an area (entity/placetexts.go; work board 773 step 41 and 792 step 47, the
// live game's place texts): the area is x1..x2, z1..z2 and y1..y2 in world.
const placesSchema = `
	CREATE TABLE IF NOT EXISTS lightspheres (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		world     INTEGER NOT NULL DEFAULT 0,
		x         REAL NOT NULL DEFAULT 0,
		z         REAL NOT NULL DEFAULT 0,
		line      TEXT NOT NULL DEFAULT '',
		answer    TEXT NOT NULL DEFAULT 'Touch the light.',
		to_world  INTEGER NOT NULL DEFAULT 0,
		to_x      REAL NOT NULL DEFAULT 0,
		to_y      REAL NOT NULL DEFAULT 0,
		to_z      REAL NOT NULL DEFAULT 0,
		to_facing INTEGER NOT NULL DEFAULT 0,
		source    TEXT NOT NULL DEFAULT 'gm',
		note      TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS placetexts (
		id     INTEGER PRIMARY KEY AUTOINCREMENT,
		world  INTEGER NOT NULL DEFAULT 0,
		x1     REAL NOT NULL DEFAULT 0,
		x2     REAL NOT NULL DEFAULT 0,
		z1     REAL NOT NULL DEFAULT 0,
		z2     REAL NOT NULL DEFAULT 0,
		y1     REAL NOT NULL DEFAULT -100000,
		y2     REAL NOT NULL DEFAULT 100000,
		text   TEXT NOT NULL DEFAULT '',
		answer TEXT NOT NULL DEFAULT 'Done',
		source TEXT NOT NULL DEFAULT 'gm',
		note   TEXT NOT NULL DEFAULT ''
	);`

// LightSphereNear is how near a sphere's place an NPC stands to be it.
const LightSphereNear = 3

// ensurePlaces makes tables lightspheres and placetexts (initSchema).
func (db *SQLiteDB) ensurePlaces() error {
	if _, err := db.Exec(placesSchema); err != nil {
		return fmt.Errorf("lightspheres, placetexts: %w", err)
	}
	return nil
}

// LightSphere is a row of lightspheres.
type LightSphere struct {
	ID       int
	World    int
	X, Z     float32
	Line     string
	Answer   string
	ToWorld  int
	ToX      float32
	ToY      float32
	ToZ      float32
	ToFacing int
}

// LightSpheres reads every row.
func (db *SQLiteDB) LightSpheres() ([]LightSphere, error) {
	rows, err := db.Query(`SELECT id, world, x, z, line, answer, to_world, to_x, to_y, to_z, to_facing FROM lightspheres
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LightSphere
	for rows.Next() {
		var s LightSphere
		if err := rows.Scan(&s.ID, &s.World, &s.X, &s.Z, &s.Line, &s.Answer, &s.ToWorld, &s.ToX, &s.ToY, &s.ToZ,
			&s.ToFacing); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// PlaceText is a row of placetexts.
type PlaceText struct {
	ID             int
	World          uint8
	X1, X2, Z1, Z2 float32
	Y1, Y2         float32
	Text, Answer   string
}

// Holds is whether (x, y, z) is in the area.
func (p PlaceText) Holds(x, y, z float32) bool {
	return x >= p.X1 && x <= p.X2 && z >= p.Z1 && z <= p.Z2 && y >= p.Y1 && y <= p.Y2
}

// PlaceTextProblem is what keeps a row from being shown ("" none): a world
// the game has, an area with its low edges first, words a PS2 can show (at
// most 127 characters, CLIENT's question; no '|' or '%'), an answer of at most
// 63.
func PlaceTextProblem(world int, p PlaceText) string {
	switch {
	case world < 0 || world > 255:
		return "world is 0-255"
	case p.X1 > p.X2 || p.Z1 > p.Z2 || p.Y1 > p.Y2:
		return "each low edge (x1, z1, y1) is at or below its high edge"
	case strings.TrimSpace(p.Text) == "":
		return "text is the words the box shows"
	case strings.ContainsAny(p.Text+p.Answer, "|%"):
		return "text and answer hold no '|' or '%'"
	case utf8.RuneCountInString(p.Text) > 127:
		return "text is at most 127 characters (CLIENT's question box)"
	case utf8.RuneCountInString(p.Answer) > 63:
		return "answer is at most 63 characters"
	}
	for _, f := range []float32{p.X1, p.X2, p.Z1, p.Z2, p.Y1, p.Y2} {
		if math.IsNaN(float64(f)) || math.Abs(float64(f)) > 200000 {
			return "an edge is a number within 200000 of the middle"
		}
	}
	return ""
}

// PlaceTexts reads every row the game can show; bad names each left out.
func (db *SQLiteDB) PlaceTexts() (out []PlaceText, bad []string, err error) {
	rows, err := db.Query(`SELECT id, world, x1, x2, z1, z2, y1, y2, text, answer FROM placetexts ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p PlaceText
		var world int
		if err := rows.Scan(&p.ID, &world, &p.X1, &p.X2, &p.Z1, &p.Z2, &p.Y1, &p.Y2, &p.Text, &p.Answer); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(p.Answer) == "" {
			p.Answer = "Done"
		}
		if why := PlaceTextProblem(world, p); why != "" {
			bad = append(bad, fmt.Sprintf("%d: %s", p.ID, why))
			continue
		}
		p.World = uint8(world)
		out = append(out, p)
	}
	return out, bad, rows.Err()
}
