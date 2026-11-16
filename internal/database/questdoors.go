package database

import (
	"encoding/json"
	"fmt"
	"strings"
)

// questdoors: a door an NPC keeps (petition 2212, Hoofbite, Heirloom
// 2026-11-16: "The Guardian in IOD blocking entrance to cazic thule house is
// able to be bypassed via logging and then running forward while
// relogging"). The NPC's body is what CLIENT stops a walk at, and only
// CLIENT: a client that makes no actor of it yet (a login, the moment its
// view fills) or that stands in its body already (logged out pressed into
// it) walks through. The server keeps the door too (entity/questdoors.go): a
// character that steps from the near side (outer, boxes [x0, y0, z0, x1,
// y1, z1] in the NPC's world, as questwalls' inner) into the far side
// (inner) while the NPC stands at its post and the character does not pass
// it is put back at outside ([x, y, z], on the near side's floor) and told
// text (none: ours). It passes past a step of a quest (questid, step: as a
// questwalls row's passer; 0 none) or holding an item (item, carried or worn;
// 0 none); a GM always does. Keyed by the npcs row of the NPC that keeps it;
// the NPC dead, out of the world or away from its post leaves it open.
const questDoorsSchema = `
	CREATE TABLE IF NOT EXISTS questdoors (
		npcid   INTEGER PRIMARY KEY,
		questid INTEGER NOT NULL DEFAULT 0,
		step    INTEGER NOT NULL DEFAULT 0,
		item    INTEGER NOT NULL DEFAULT 0,
		inner   TEXT NOT NULL,
		outer   TEXT NOT NULL,
		outside TEXT NOT NULL,
		text    TEXT NOT NULL DEFAULT '',
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

// QuestDoor is one row of questdoors.
type QuestDoor struct {
	NPCID, QuestID, Step, Item int
	Inner, Outer               []WallBox
	Outside                    [3]float32
	Text                       string
}

// ParsePoint reads a place: a JSON [x, y, z].
func ParsePoint(s string) ([3]float32, error) {
	var p []float32
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &p); err != nil {
		return [3]float32{}, fmt.Errorf("place %q: %w", s, err)
	}
	if len(p) != 3 {
		return [3]float32{}, fmt.Errorf("place %q: %d numbers, want 3 (x, y, z)", s, len(p))
	}
	return [3]float32{p[0], p[1], p[2]}, nil
}

// QuestDoors is every row of questdoors. A row whose boxes or place do not
// read, or that has no box on a side, is an error.
func (db *SQLiteDB) QuestDoors() ([]QuestDoor, error) {
	rows, err := db.Query(`SELECT npcid, questid, step, item, inner, outer, outside, text FROM questdoors ORDER BY npcid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuestDoor
	for rows.Next() {
		var d QuestDoor
		var inner, outer, outside string
		if err := rows.Scan(&d.NPCID, &d.QuestID, &d.Step, &d.Item, &inner, &outer, &outside, &d.Text); err != nil {
			return nil, err
		}
		if d.Inner, err = ParseWallBoxes(inner); err != nil {
			return nil, fmt.Errorf("questdoors row %d: %w", d.NPCID, err)
		}
		if d.Outer, err = ParseWallBoxes(outer); err != nil {
			return nil, fmt.Errorf("questdoors row %d outer: %w", d.NPCID, err)
		}
		if len(d.Inner) == 0 || len(d.Outer) == 0 {
			return nil, fmt.Errorf("questdoors row %d: a box on each side wanted (inner %d, outer %d)", d.NPCID, len(d.Inner), len(d.Outer))
		}
		if d.Outside, err = ParsePoint(outside); err != nil {
			return nil, fmt.Errorf("questdoors row %d outside: %w", d.NPCID, err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
