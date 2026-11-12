package database

import "fmt"

// questmarks: the quests whose NPCs show a "!" and a "?" over their heads
// while the server's quests.markers is "flagged" (the default; the GM
// Jibrish's petition 1813, 2026-11-12: "Add the ability for us to add yellow !
// and ? over NPCs in game ... Once done place these for the first quest
// everyone has for every race and class"; handlers/questheadmarks.go). A row
// a quest: its giver shows "!" while a player may take it, and each NPC of
// one of its steps "?" while that player can do the step there. "all" marks
// every quest and "off" none, whatever the table holds. A GM adds or takes
// rows in the panel (Quest marks); tools/questmarks_20261112.py flags each
// race and class's first quest.
const questMarksSchema = `
CREATE TABLE IF NOT EXISTS questmarks (
	questid INTEGER PRIMARY KEY,
	note    TEXT NOT NULL DEFAULT ''
);`

// ensureQuestMarks makes table questmarks (initSchema), empty.
func (db *SQLiteDB) ensureQuestMarks() error {
	if _, err := db.Exec(questMarksSchema); err != nil {
		return fmt.Errorf("questmarks: %w", err)
	}
	return nil
}

// MarkedQuests is table questmarks: the quests flagged for marks over their
// NPCs, by id. A database without the table (an older one, a test's) flags
// none.
func (s *QuestStore) MarkedQuests() (map[int]bool, error) {
	var had int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'questmarks'`).Scan(&had); err != nil {
		return nil, fmt.Errorf("questmarks: %w", err)
	}
	out := map[int]bool{}
	if had == 0 {
		return out, nil
	}
	rs, err := s.db.Query(`SELECT questid FROM questmarks`)
	if err != nil {
		return nil, fmt.Errorf("questmarks: %w", err)
	}
	defer rs.Close()
	for rs.Next() {
		var id int
		if err := rs.Scan(&id); err != nil {
			return nil, fmt.Errorf("questmarks: %w", err)
		}
		out[id] = true
	}
	return out, rs.Err()
}
