package database

import (
	"fmt"
	"sort"
)

// npctint: a tint on the bodies of the NPCs of a name, for a client that draws
// one (entity/npctint.go, protocol.OpcodeNPCTints; the server owner,
// 2026-11-10: Yhafiran, "the black and green oriental dragon", wears the disc's
// only green oriental dragon, CHAR.ESF 0xB5C8A651, which the Plane of
// Disease's twins wear too, and no black and green body exists, so "vanilla
// draws Yhafiran tinted toward black and green"). A PS2 is never told: it draws
// the body in its own colours.
//
// color: 0xRRGGBBAA, red in the high byte (as chardyes holds a colour): each of
// the body's colours' red, green and blue times the tint's (255ths: 0xFF keeps
// a channel, 0x00 makes it black), as much of it as the alpha says (0xFF the
// whole, 0x80 about half, 0 none: a row kept with nothing drawn). The key is
// the name as npcclass keys it (NPCClassKey: any case, trimmed, no trailing
// '$'), so a GM's #spawn copy and every spawn of the name share it. source: gm,
// or seed for tools/seed_npc_tint.py's rows (its --undo takes out exactly what
// it wrote).
const npcTintSchema = `
	CREATE TABLE IF NOT EXISTS npctint (
		name   TEXT PRIMARY KEY,
		color  INTEGER NOT NULL DEFAULT 4294967295,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCTint makes table npctint (initSchema).
func (db *SQLiteDB) ensureNPCTint() error {
	if _, err := db.Exec(npcTintSchema); err != nil {
		return fmt.Errorf("npctint: %w", err)
	}
	return nil
}

// NPCTint is a row of npctint, read: the name's key and its colour.
type NPCTint struct {
	Name  string // NPCClassKey of the row's name
	Color uint32 // 0xRRGGBBAA
}

// NPCTintChanges reports whether a tint changes anything drawn: an alpha of 0
// or a white tint leaves the body as it is.
func NPCTintChanges(color uint32) bool {
	return color&0xFF != 0 && color>>8 != 0xFFFFFF
}

// tint32 is an npctint colour as 32 bits, whether it was stored unsigned (the
// GM panel's) or as a signed 32-bit value; anything wider is none.
func tint32(c int64) (uint32, bool) {
	if c < -(1<<31) || c > 0xFFFFFFFF {
		return 0, false
	}
	return uint32(c), true
}

// ValidNPCTint reports whether an npctint colour is a 32-bit 0xRRGGBBAA (the
// GM panel's check of a row).
func ValidNPCTint(c int64) bool {
	_, ok := tint32(c)
	return ok
}

// NPCTints is every row of npctint that tints something, by NPCClassKey of the
// name (a blank one, and one whose colour changes nothing, left out), in the
// keys' order. Two rows of one key: the later name in the table's order wins.
func (db *SQLiteDB) NPCTints() ([]NPCTint, error) {
	rows, err := db.Query(`SELECT name, color FROM npctint ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := make(map[string]uint32)
	for rows.Next() {
		var name string
		var color int64
		if err := rows.Scan(&name, &color); err != nil {
			return nil, err
		}
		k := NPCClassKey(name)
		c, ok := tint32(color)
		if k == "" || !ok {
			continue
		}
		if NPCTintChanges(c) {
			by[k] = c
		} else {
			delete(by, k)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NPCTint, 0, len(by))
	for k, c := range by {
		out = append(out, NPCTint{Name: k, Color: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
