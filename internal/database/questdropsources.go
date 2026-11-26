package database

import "strings"

// DropSources is, for each item (patternid) asked, the names of the creatures
// whose loot tables drop it: npcloot's tables joined to loottables, and a
// lootpool row that names a creature. Read for the quest marks (a give step's
// item the player has to farm; petition 3221). A database lacking a table
// gives nothing from it.
func (s *QuestStore) DropSources(items map[int]bool) (map[int][]string, error) {
	out := map[int][]string{}
	seen := map[int]map[string]bool{}
	for _, q := range []string{
		`SELECT t.patternid, n.npc_name FROM loottables t JOIN npcloot n ON n.tablename = t.tablename ORDER BY n.npc_name`,
		`SELECT patternid, npc_name FROM lootpool WHERE npc_name IS NOT NULL AND npc_name <> '' ORDER BY npc_name`,
	} {
		rows, err := s.db.Query(q)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return nil, err
		}
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, err
			}
			if !items[id] {
				continue
			}
			k := strings.ToLower(strings.TrimSpace(name))
			if k == "" {
				continue
			}
			if seen[id] == nil {
				seen[id] = map[string]bool{}
			}
			if !seen[id][k] {
				seen[id][k] = true
				out[id] = append(out[id], strings.TrimSpace(name))
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
