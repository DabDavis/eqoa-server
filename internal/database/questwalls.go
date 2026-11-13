package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The Cazic-Thule trials' tables (2026-11-11; work board 430's code asks 5
// and 6, 656's asks 1 and 2): entity/questwalls.go, entity/aggro.go,
// handlers/itemhide.go.
//
// questwalls: an NPC (an invisible blocker, CHAR.ESF 0x6F412342, a box CLIENT
// stops a walk at: claude-notes/invisible-blockers-20261029.md) that a
// character stops seeing, so walks through, once it is past a step of a quest:
// its quest finished, or under way at a step after step. "Each completed trial
// lets the questor pass a new invisible wall" (the t196 walkthrough). By npcs
// row. source: gm, or the tool that wrote it.
//
// A wall may open for an item instead (the Last House's ward, the owner
// 2026-11-13: "where dust is standing is where something should block players
// until they possess the suit"): item, an itempattern a character carries in
// its pack or wears, passes it (questid 0: no quest does); inner, a JSON list
// of boxes [x0, y0, z0, x1, y1, z1] in the wall's world: a character standing
// in one walks through, so none is shut in on the wall's far side (none: no
// such side); text, the line a character stopped at it is told once (none:
// nothing said). A GM passes a wall of an item.
const questWallsSchema = `
	CREATE TABLE IF NOT EXISTS questwalls (
		npcid   INTEGER PRIMARY KEY,
		questid INTEGER NOT NULL,
		step    INTEGER NOT NULL DEFAULT 0,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm',
		item    INTEGER NOT NULL DEFAULT 0,
		inner   TEXT NOT NULL DEFAULT '',
		text    TEXT NOT NULL DEFAULT ''
	);`

// questWallItemColumns are the columns an item's wall added (2026-11-13), for a
// table made before them.
var questWallItemColumns = []struct{ name, ddl string }{
	{"item", `ALTER TABLE questwalls ADD COLUMN item INTEGER NOT NULL DEFAULT 0`},
	{"inner", `ALTER TABLE questwalls ADD COLUMN inner TEXT NOT NULL DEFAULT ''`},
	{"text", `ALTER TABLE questwalls ADD COLUMN text TEXT NOT NULL DEFAULT ''`},
}

// itemhide: an item whose Use near an NPC of a name takes that NPC out of the
// world for a while (handlers/itemhide.go): Cazic's Scent at the Estate's door
// "to remove the Guardian blocking the doorway" (t196), once the user is past
// step of quest as a wall's passer is (questid 0: anyone). text is said to the
// user, refuse to one not past it (none: ours).
const itemHideSchema = `
	CREATE TABLE IF NOT EXISTS itemhide (
		patternid INTEGER PRIMARY KEY,
		npc       TEXT NOT NULL,
		radius    REAL NOT NULL DEFAULT 20,
		minutes   REAL NOT NULL DEFAULT 10,
		questid   INTEGER NOT NULL DEFAULT 0,
		step      INTEGER NOT NULL DEFAULT 0,
		text      TEXT NOT NULL DEFAULT '',
		refuse    TEXT NOT NULL DEFAULT '',
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureQuestWalls makes tables questwalls and itemhide, and npcaggro's
// holding column (initSchema, after ensureNPCAggro).
func (db *SQLiteDB) ensureQuestWalls() error {
	if _, err := db.Exec(questWallsSchema); err != nil {
		return fmt.Errorf("questwalls: %w", err)
	}
	for _, c := range questWallItemColumns {
		ok, err := hasColumn(db.DB, "questwalls", c.name)
		if err != nil {
			return fmt.Errorf("questwalls: %w", err)
		}
		if !ok {
			if _, err := db.Exec(c.ddl); err != nil {
				return fmt.Errorf("questwalls %s: %w", c.name, err)
			}
		}
	}
	if _, err := db.Exec(itemHideSchema); err != nil {
		return fmt.Errorf("itemhide: %w", err)
	}
	ok, err := hasColumn(db.DB, "npcaggro", "holding")
	if err != nil {
		return fmt.Errorf("npcaggro: %w", err)
	}
	if !ok {
		if _, err := db.Exec(`ALTER TABLE npcaggro ADD COLUMN holding INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("npcaggro holding: %w", err)
		}
	}
	return nil
}

// QuestWall is one row of questwalls.
type QuestWall struct {
	NPCID, QuestID, Step int
	// Item passes it to whoever carries or wears it (0: none); Inner are the
	// boxes of its far side, where a character walks through it whatever it
	// holds; Text is told once to one it stops.
	Item  int
	Inner []WallBox
	Text  string
}

// WallBox is a box of a wall's far side: X0 <= x <= X1, Y0 <= y <= Y1, Z0 <=
// z <= Z1.
type WallBox struct{ X0, Y0, Z0, X1, Y1, Z1 float32 }

// Holds is whether the box holds the place.
func (b WallBox) Holds(x, y, z float32) bool {
	return x >= b.X0 && x <= b.X1 && y >= b.Y0 && y <= b.Y1 && z >= b.Z0 && z <= b.Z1
}

// ParseWallBoxes reads a questwalls row's inner: a JSON list of [x0, y0, z0,
// x1, y1, z1] boxes, each corner pair in any order ("" none).
func ParseWallBoxes(s string) ([]WallBox, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var raw [][]float32
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("inner %q: %w", s, err)
	}
	out := make([]WallBox, 0, len(raw))
	for i, r := range raw {
		if len(r) != 6 {
			return nil, fmt.Errorf("inner box %d: %d numbers, want 6 (x0, y0, z0, x1, y1, z1)", i+1, len(r))
		}
		out = append(out, WallBox{min(r[0], r[3]), min(r[1], r[4]), min(r[2], r[5]), max(r[0], r[3]), max(r[1], r[4]), max(r[2], r[5])})
	}
	return out, nil
}

// QuestWalls is every row of questwalls. A row whose inner does not read is
// an error: a wall shut in on its far side is worse than none.
func (db *SQLiteDB) QuestWalls() ([]QuestWall, error) {
	rows, err := db.Query(`SELECT npcid, questid, step, item, inner, text FROM questwalls ORDER BY npcid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuestWall
	for rows.Next() {
		var w QuestWall
		var inner string
		if err := rows.Scan(&w.NPCID, &w.QuestID, &w.Step, &w.Item, &inner, &w.Text); err != nil {
			return nil, err
		}
		if w.Inner, err = ParseWallBoxes(inner); err != nil {
			return nil, fmt.Errorf("questwalls row %d: %w", w.NPCID, err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ItemHide is one row of itemhide.
type ItemHide struct {
	PatternID int
	NPC       string
	Radius    float64
	Minutes   float64
	QuestID   int
	Step      int
	Text      string
	Refuse    string
}

// ItemHideOf is the item's row of itemhide, false when it has none.
func (s *InventoryStore) ItemHideOf(patternID int) (ItemHide, bool, error) {
	var h ItemHide
	err := s.db.QueryRow(`SELECT patternid, npc, radius, minutes, questid, step, text, refuse FROM itemhide WHERE patternid = ?`, patternID).
		Scan(&h.PatternID, &h.NPC, &h.Radius, &h.Minutes, &h.QuestID, &h.Step, &h.Text, &h.Refuse)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ItemHide{}, false, nil
		}
		return ItemHide{}, false, fmt.Errorf("itemhide: %w", err)
	}
	return h, true, nil
}
