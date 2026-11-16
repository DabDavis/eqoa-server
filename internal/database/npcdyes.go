package database

import "fmt"

// npcdyes: dyes on the NPCs of a name, slot by slot, as chardyes dyes a
// character (the owner, 2026-11-16: "make holiday elfs as gnome model, with red
// and green dyed clothing, also add ogre santa clause in a santa suit"). A row
// is a name (NPCClassKey), a dye slot (chardyes' numbers: chest 0 .. helm 5,
// robe 9, the trims 11-16) and a colour 0xRRGGBBAA. A client of the live look
// (vanilla) draws the colour itself, a trim's too; a PS2 is sent the disc tint
// nearest each armour slot's and the robe's (entity/npcdyes.go), and no trim.
const npcDyesSchema = `
	CREATE TABLE IF NOT EXISTS npcdyes (
		name   TEXT NOT NULL,
		slot   INTEGER NOT NULL,
		color  INTEGER NOT NULL,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (name, slot)
	);`

func (db *SQLiteDB) ensureNPCDyes() error {
	if _, err := db.Exec(npcDyesSchema); err != nil {
		return fmt.Errorf("npcdyes: %w", err)
	}
	return nil
}

// NPCDyes is every row of npcdyes by NPCClassKey of the name, each name's
// colours by dye slot (0: none). A row of a slot out of range or of colour 0 is
// left out.
func (db *SQLiteDB) NPCDyes() (map[string][DyeSlots]uint32, error) {
	rows, err := db.Query(`SELECT name, slot, color FROM npcdyes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][DyeSlots]uint32{}
	for rows.Next() {
		var name string
		var slot int
		var color int64
		if err := rows.Scan(&name, &slot, &color); err != nil {
			return nil, err
		}
		k := NPCClassKey(name)
		if k == "" || slot < 0 || slot >= DyeSlots || uint32(color) == 0 {
			continue
		}
		d := out[k]
		d[slot] = uint32(color)
		out[k] = d
	}
	return out, rows.Err()
}
