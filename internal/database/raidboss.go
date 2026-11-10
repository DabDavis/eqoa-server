package database

// RaidBoss reports whether the loot's tables make a creature of that name a raid boss,
// for a raid's need before greed (handlers/raidloot.go): its loot categories' profile is
// "boss" (lootcreature, a GM's on the panel's Loot creatures page), or one of its loot
// tables is a "boss ..." one (npcloot: tools/seed_boss_loot.py's). Its class template's
// hit points are the World's (entity.RaidKill.TemplateHPPct).
func (s *InventoryStore) RaidBoss(name string) (bool, error) {
	key := NPCClassKey(name)
	var boss bool
	err := s.db.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM lootcreature WHERE npc_name = ? AND lower(trim(profile)) = 'boss')
		OR EXISTS(SELECT 1 FROM npcloot WHERE npc_name = ? AND lower(tablename) LIKE 'boss %')`, key, key).Scan(&boss)
	return boss, err
}

// RaidBossNames is every name RaidBoss says yes to, each as NPCClassKey keys it: the
// same two rules read whole, for what asks under the World's lock and cannot ask the
// database a kill at a time (entity/heroes.go: a raid boss's kill is worth five with the
// Testing Heroes). TestRaidBossNamesAreRaidBosss holds the two to each other.
func (db *SQLiteDB) RaidBossNames() ([]string, error) {
	rows, err := db.Query(`SELECT npc_name FROM lootcreature WHERE lower(trim(profile)) = 'boss'
		UNION SELECT npc_name FROM npcloot WHERE lower(tablename) LIKE 'boss %'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
