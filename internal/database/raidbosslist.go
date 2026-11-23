package database

import "fmt"

// raidbosslist: the raid bosses a GM's #raidboss shows (handlers/raidbosscmd.go), by
// NPC name. The owner, 2026-11-22: "improve the /raidmobs (bosses?) command to only show
// actual raid bosses like uzra and poxathixius, lord nagafen etc, bertoxulous throne
// etc. Right now it shows tons of random raid mobs, not bosses": the raid loot's rule
// (RaidBoss, entity's RaidBossHPPct) takes every creature of a raid's hit points, so
// The Hunt's wildlife, the Khal infantry, the Zaoran captains and the Isle of Dread's
// golems were listed beside the bosses. This table names only the bosses, and only
// #raidboss reads it: the raid loot's need before greed and the Testing Heroes' credit
// keep their own rule.
//
// A row's name is matched as npcclass keys one (NPCClassKey: lower case, trimmed, no
// trailing '$'), so every NPC of the name is listed, a dungeon's copy included. A new
// database's table holds raidBossListSeed (source seed); a GM adds or takes out rows on
// the panel's Raid boss list, and a database that has the table keeps what is in it.
const raidBossListSchema = `
	CREATE TABLE IF NOT EXISTS raidbosslist (
		npc_name TEXT PRIMARY KEY,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// raidBossListSeed is the list a new raidbosslist holds: the bosses a raid comes for,
// the end of a raid zone, a world raid's target, an epic-sode's raid boss and the gods,
// as the raid research named them (work boards 426, 430, 1130, 1593, 1641; petition
// 1240). Left out: the creatures of a raid's hit points that are not its boss (The
// Hunt's wildlife, the Khal infantry, the Zaoran captains and sentries, the Isle of
// Dread's golems and nameds, the Plane of Sky's constructs and escorts, the death mists,
// Tunare's treant), the Plane of Fire's minibosses and nameds, the epic quests' mobs and
// the group nameds with a boss loot table.
var raidBossListSeed = []struct{ Name, Note string }{
	// the Plane of Disease
	{"Bertoxxulous", "Plane of Disease"},
	{"Poxathixius", "Plane of Disease"},
	{"Uzrathixius", "Plane of Disease"},
	{"Queen Qy'tha", "Plane of Disease"},
	{"Noroxus", "Plane of Disease"},
	// the Plane of Sky
	{"Quintessence of Veeshan", "Plane of Sky"},
	{"Veeshan's Talon", "Plane of Sky"},
	{"Veeshan's Bulwark", "Plane of Sky"},
	// the Zaoran Plateau
	{"Tallon Zek", "Zaoran Plateau"},
	{"Vallon Zek", "Zaoran Plateau"},
	{"Rallos Zek", "Zaoran Plateau"},
	{"Rallos Zek the Warlord", "Zaoran Plateau"},
	{"Rallos Zek Unbound", "Zaoran Plateau"},
	{"Razeal Moloch", "Zaoran Plateau"},
	// the world's raid bosses
	{"Lord Nagafen", "Lavastorm"},
	{"Lady Vox", "Permafrost"},
	{"CazicThule", "Isle of Dread"},
	{"Megaladon", "Isle of Dread"},
	{"Yhafiran", "a world raid boss"},
	{"Avatar of Fear", "Cazic Thule"},
	{"Overlord Xrag", "a world raid boss"},
	{"Siliskor", "a world raid boss"},
	{"Skahyir", "a world raid boss"},
	{"Toxxulia", "a world raid boss"},
	{"Quintashi", "a world raid boss"},
	{"Dracoliche", "a world raid boss"},
	{"Kratas", "a world raid boss"},
	{"Sarek the Betrayer", "Geo Citadel"},
	{"Mayong Mistmoore", "a world raid boss"},
	{"Dynmath the Tyrant", "a world raid boss"},
	{"Earth Primeval", "a world raid boss"},
	{"Fire Primeval", "a world raid boss"},
	{"Khal Viceroy", "Khal"},
	// the epic-sodes' raid bosses
	{"Aktana the Flamekissed", "epic-sode"},
	{"An Ancient Corpse", "epic-sode"},
	{"Avatar of Love", "epic-sode"},
	{"Avatar of Hate", "epic-sode"},
	{"Synder of the Indigo", "epic-sode"},
	{"Vylinnuza of the Tide", "epic-sode"},
	{"Cyrcalis", "epic-sode"},
	{"Megametamilipizon", "epic-sode"},
	{"Ahriaana the Ultimate", "epic-sode"},
	// the gods
	{"Innoruuk", "a god"},
	{"Tunare", "a god"},
	{"Mithaniel Marr", "a god"},
	{"Erollisi Marr", "a god"},
	// the server's own
	{"Mr. Moogie", "the server's own"},
	{"The Grinch", "a season's event"},
}

// ensureRaidBossList makes table raidbosslist (initSchema), with its seed when the table
// is new.
func (db *SQLiteDB) ensureRaidBossList() error {
	had := tableExists(db, "raidbosslist")
	if _, err := db.Exec(raidBossListSchema); err != nil {
		return fmt.Errorf("raidbosslist: %w", err)
	}
	if had {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("raidbosslist: %w", err)
	}
	defer tx.Rollback()
	for _, s := range raidBossListSeed {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO raidbosslist (npc_name, note, source) VALUES (?, ?, 'seed')`,
			NPCClassKey(s.Name), s.Note); err != nil {
			return fmt.Errorf("raidbosslist: the seed: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("raidbosslist: %w", err)
	}
	return nil
}

// RaidBossList is every name of raidbosslist, each as NPCClassKey keys it (a row in
// capitals or with a trailing '$' as its key), in no order.
func (db *SQLiteDB) RaidBossList() ([]string, error) {
	rows, err := db.Query(`SELECT npc_name FROM raidbosslist`)
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
		if k := NPCClassKey(n); k != "" {
			out = append(out, k)
		}
	}
	return out, rows.Err()
}
