package database

import "fmt"

// npcdyes: dyes on the NPCs of a name, slot by slot, as chardyes dyes a
// character (the owner, 2026-11-16: "make holiday elfs as gnome model, with red
// and green dyed clothing, also add ogre santa clause in a santa suit"). A row
// is a name (NPCClassKey), a dye slot (chardyes' numbers: chest 0 .. helm 5,
// robe 9, the trims 11-16) and a colour 0xRRGGBBAA. A client of the live look
// (vanilla) draws the colour itself, a trim's too; a PS2 is sent the disc tint
// nearest each armour slot's and the robe's (entity/npcdyes.go), and no trim.
//
// season (2026-11-17, the owner: "make the toymaker tinsel, holiday elf weapon
// enchanter wear santa hats and santas robe"): blank always, else a season of
// gmseasons the row holds only while it is on (SeasonHolds): off, the slot is
// as with no row. robe, on a robe row (slot 9) alone: the robe the name wears
// while the row holds (0-3, as npcs.robe_style: Santa's is 0), -1 (the default)
// the NPC's own; every client draws it, a PS2 too.
const npcDyesSchema = `
	CREATE TABLE IF NOT EXISTS npcdyes (
		name   TEXT NOT NULL,
		slot   INTEGER NOT NULL,
		color  INTEGER NOT NULL,
		season TEXT NOT NULL DEFAULT '',
		robe   INTEGER NOT NULL DEFAULT -1,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (name, slot)
	);`

func (db *SQLiteDB) ensureNPCDyes() error {
	if _, err := db.Exec(npcDyesSchema); err != nil {
		return fmt.Errorf("npcdyes: %w", err)
	}
	// a table made before the season and the robe (2026-11-17) takes them
	if err := addTextColumn(db, "npcdyes", "season"); err != nil {
		return err
	}
	has, err := hasColumn(db.DB, "npcdyes", "robe")
	if err != nil {
		return fmt.Errorf("npcdyes: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npcdyes ADD COLUMN robe INTEGER NOT NULL DEFAULT -1`); err != nil {
			return fmt.Errorf("npcdyes: robe: %w", err)
		}
	}
	return nil
}

// addTextColumn gives a table made before it a TEXT column, blank by default.
func addTextColumn(db *SQLiteDB, table, col string) error {
	has, err := hasColumn(db.DB, table, col)
	if err != nil {
		return fmt.Errorf("%s: %w", table, err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("%s: %s: %w", table, col, err)
		}
	}
	return nil
}

// NPCDyeRobeMost is the last robe an npcdyes row may put on (npcs.robe_style's
// 0-3; 255 none).
const NPCDyeRobeMost = 3

// NPCDyes is every row of npcdyes by NPCClassKey of the name, each name's
// colours by dye slot (0: none). A row of a slot out of range or of colour 0,
// or of a season that is off, is left out.
func (db *SQLiteDB) NPCDyes() (map[string][DyeSlots]uint32, error) {
	out, _, err := db.NPCDyeLooks()
	return out, err
}

// NPCDyeLooks is NPCDyes and the robes the robe rows put on (their robe 0 ..
// NPCDyeRobeMost, by NPCClassKey; a robe row of colour 0 puts its robe on all
// the same).
func (db *SQLiteDB) NPCDyeLooks() (map[string][DyeSlots]uint32, map[string]int, error) {
	on, err := db.ActiveSeasons()
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.Query(`SELECT name, slot, color, season, robe FROM npcdyes`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out, robes := map[string][DyeSlots]uint32{}, map[string]int{}
	for rows.Next() {
		var name, season string
		var slot, robe int
		var color int64
		if err := rows.Scan(&name, &slot, &color, &season, &robe); err != nil {
			return nil, nil, err
		}
		k := NPCClassKey(name)
		if k == "" || slot < 0 || slot >= DyeSlots || !SeasonHolds(season, on) {
			continue
		}
		if slot == DyeRobe && robe >= 0 && robe <= NPCDyeRobeMost {
			robes[k] = robe
		}
		if uint32(color) == 0 {
			continue
		}
		d := out[k]
		d[slot] = uint32(color)
		out[k] = d
	}
	return out, robes, rows.Err()
}
