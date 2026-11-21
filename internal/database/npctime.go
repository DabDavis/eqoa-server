package database

import (
	"fmt"
	"strings"
)

// npctime: the NPCs that stand in the world only by day, or only by night (the
// owner, 2026-11-21, from a walkthrough of Southwatch Keep: "mercenaries by
// day, skeletal watchmen by night; the wildlife round it changes with the time
// of day too"). A row names NPCs as npcspawnonly does: by name (NPCClassKey:
// every NPC of the name), or "#n" for the npcs row n alone (entity.RowKey),
// which wins over its name's row. time is "day" or "night"; any other value
// counts as no row. Out of its time an NPC is held out of the world as a
// spawn-only one is (entity/npctime.go has the turn); copies (a GM's, a
// quest's, a trigger's) are never held.
const npcTimeSchema = `
	CREATE TABLE IF NOT EXISTS npctime (
		name   TEXT PRIMARY KEY,
		time   TEXT NOT NULL DEFAULT 'night',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// The two times a row of npctime may name.
const (
	NPCTimeDay   = "day"
	NPCTimeNight = "night"
)

// NPCTimeOf reads a time of npctime: "day" or "night" (any case, trimmed), ok
// false for anything else.
func NPCTimeOf(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case NPCTimeDay:
		return NPCTimeDay, true
	case NPCTimeNight:
		return NPCTimeNight, true
	}
	return "", false
}

// ensureNPCTime makes table npctime (initSchema).
func (db *SQLiteDB) ensureNPCTime() error {
	if _, err := db.Exec(npcTimeSchema); err != nil {
		return fmt.Errorf("npctime: %w", err)
	}
	return nil
}

// NPCTimes is every row of npctime that counts: its key (NPCClassKey of the
// name, which keeps a "#n" row key as it is) to "day" or "night".
func (db *SQLiteDB) NPCTimes() (map[string]string, error) {
	rows, err := db.Query(`SELECT name, time FROM npctime`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var name, t string
		if err := rows.Scan(&name, &t); err != nil {
			return nil, err
		}
		k := NPCClassKey(strings.TrimSpace(name))
		if when, ok := NPCTimeOf(t); ok && k != "" {
			out[k] = when
		}
	}
	return out, rows.Err()
}
