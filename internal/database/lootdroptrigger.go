package database

import (
	"fmt"
	"strings"
)

// lootdroptrigger.go — an item a corpse rolls fires a GM trigger where the corpse lies (the owner,
// 2026-11-18, Ronson's t222 "Vallon Zek": "As soon as a Zek Scout drops a Portal Rune, the portal will
// spawn by the campfire"; tools/vallon_zek_20261118.py writes the rune's row). A row names an item record
// (patternid: the shards' ids differ, so a tool fills it by name) and a trigger, by the trigger's name
// (gmtriggers.name, as npcdeath names one). The loot is rolled at a corpse's first opening
// (handlers/loot.go rollCorpse), so the trigger fires then, once a corpse, at the corpse's place, quietly
// (no GM lines), whoever opened it; a trigger whose lock a run holds does not fire again (the portal
// standing already). Read at each roll: a GM's change counts at once.
const lootDropTriggerSchema = `
	CREATE TABLE IF NOT EXISTS lootdroptrigger (
		patternid INTEGER PRIMARY KEY,
		trigger   TEXT NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureLootDropTriggers makes table lootdroptrigger (initSchema).
func (db *SQLiteDB) ensureLootDropTriggers() error {
	if _, err := db.Exec(lootDropTriggerSchema); err != nil {
		return fmt.Errorf("lootdroptrigger: %w", err)
	}
	return nil
}

// LootDropTriggers is the triggers the item records fire when a corpse rolls one, in the order the
// records are given, each trigger once; none for none (or no table).
func (s *InventoryStore) LootDropTriggers(patternIDs []int) ([]string, error) {
	if len(patternIDs) == 0 {
		return nil, nil
	}
	seen := map[int]bool{}
	var ids []any
	for _, id := range patternIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	q := `SELECT patternid, trigger FROM lootdroptrigger WHERE patternid IN (?` + strings.Repeat(`, ?`, len(ids)-1) + `)`
	rows, err := s.db.Query(q, ids...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, fmt.Errorf("lootdroptrigger: %w", err)
	}
	defer rows.Close()
	by := map[int]string{}
	for rows.Next() {
		var id int
		var trig string
		if err := rows.Scan(&id, &trig); err != nil {
			return nil, fmt.Errorf("lootdroptrigger: %w", err)
		}
		if trig = strings.TrimSpace(trig); trig != "" {
			by[id] = trig
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	fired := map[string]bool{}
	for _, id := range patternIDs {
		t, ok := by[id]
		if !ok || fired[strings.ToLower(t)] {
			continue
		}
		fired[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out, nil
}
