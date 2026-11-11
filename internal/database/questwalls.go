package database

import (
	"database/sql"
	"errors"
	"fmt"
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
const questWallsSchema = `
	CREATE TABLE IF NOT EXISTS questwalls (
		npcid   INTEGER PRIMARY KEY,
		questid INTEGER NOT NULL,
		step    INTEGER NOT NULL DEFAULT 0,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

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
}

// QuestWalls is every row of questwalls.
func (db *SQLiteDB) QuestWalls() ([]QuestWall, error) {
	rows, err := db.Query(`SELECT npcid, questid, step FROM questwalls ORDER BY npcid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuestWall
	for rows.Next() {
		var w QuestWall
		if err := rows.Scan(&w.NPCID, &w.QuestID, &w.Step); err != nil {
			return nil, err
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
