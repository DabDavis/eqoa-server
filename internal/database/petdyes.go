package database

import (
	"fmt"
	"strconv"
	"strings"
)

// petdyes and petdyeareas: a pet's dyes, two colours on two areas of its body,
// which the Pet Groomer puts on (handlers/petgroomer.go) and a client that
// draws them is told (entity/petdyes.go, protocol.OpcodePetDyes). The owner,
// 2026-11-16, of Zerkziee's drachnid: "make it have 2 different color areas
// that can be changed like armor dye (will be a template for pet color dye)".
// A PS2 is never told: it draws every pet in its body's own colours.
//
// petdyes: a character's dyes on a pet body, kept by the body, so each look of
// a pet keeps its own (a construct dyed red stays red when the drachnid is
// summoned, and the drachnid's own come back with it): serverid, the
// character; look, the body's model DictID (npcs.modelid's 32 bits); area1 and
// area2, 0xRRGGBBAA as chardyes holds a colour (red in the high byte, alpha
// never 0; 0: that area undyed). A row with both 0 is no row.
//
// petdyeareas: a body's two areas, by its palette's materials, in place of the
// client's rule: look, the body; area1 and area2, the areas' names as the
// Groomer asks them ("Torso", "Spider body"); materials, area 2's material
// indices ("0,2,5", 0-63: the palette entries a body's batches name, esf's
// Batch.material), every other material area 1 (none: one area, its whole
// body). A body with no row (2026-11-16, the owner: "the pets need their
// clothes as one dye and the rest another like we two tone equipment") is the
// rule's: area 1 its clothing, area 2 the rest of it, as the client tells them
// apart (entity.PetBodyClothed: a body that wears none is dyed whole, area 2).
const petDyesSchema = `
	CREATE TABLE IF NOT EXISTS petdyes (
		serverid INTEGER NOT NULL,
		look     INTEGER NOT NULL,
		area1    INTEGER NOT NULL DEFAULT 0,
		area2    INTEGER NOT NULL DEFAULT 0,
		note     TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serverid, look)
	);
	CREATE TABLE IF NOT EXISTS petdyeareas (
		look      INTEGER PRIMARY KEY,
		area1     TEXT NOT NULL DEFAULT 'Body',
		area2     TEXT NOT NULL DEFAULT '',
		materials TEXT NOT NULL DEFAULT '',
		note      TEXT NOT NULL DEFAULT ''
	);`

// PetDyeAreas is how many areas a pet body has at most.
const PetDyeAreas = 2

// PetDyeMaterialsMost is one past the highest material index an area names:
// the wire's mask is 64 bits.
const PetDyeMaterialsMost = 64

// ensurePetDyes makes tables petdyes and petdyeareas (initSchema).
func (db *SQLiteDB) ensurePetDyes() error {
	if _, err := db.Exec(petDyesSchema); err != nil {
		return fmt.Errorf("petdyes: %w", err)
	}
	return nil
}

// PetDyeArea is one row of petdyeareas, read.
type PetDyeArea struct {
	Look         uint32
	Area1, Area2 string
	// Mask is area 2's materials, bit n material n
	Mask uint64
	Note string
}

// TwoAreas is whether the body has a second area.
func (a PetDyeArea) TwoAreas() bool { return a.Mask != 0 && strings.TrimSpace(a.Area2) != "" }

// ParsePetDyeMaterials reads petdyeareas.materials: material indices 0-63,
// apart by commas or spaces. A word that is no such index is an error.
func ParsePetDyeMaterials(s string) (uint64, error) {
	var m uint64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 || n >= PetDyeMaterialsMost {
			return 0, fmt.Errorf("material %q: 0 to %d", f, PetDyeMaterialsMost-1)
		}
		m |= 1 << uint(n)
	}
	return m, nil
}

// FormatPetDyeMaterials is a mask as petdyeareas.materials writes it.
func FormatPetDyeMaterials(m uint64) string {
	var out []string
	for i := 0; i < PetDyeMaterialsMost; i++ {
		if m&(1<<uint(i)) != 0 {
			out = append(out, strconv.Itoa(i))
		}
	}
	return strings.Join(out, ",")
}

// PetDyeAreaRows is petdyeareas by body; bad says each row it left out and
// why (a materials list it cannot read).
func (db *SQLiteDB) PetDyeAreaRows() (map[uint32]PetDyeArea, []string, error) {
	if err := db.ensurePetDyes(); err != nil {
		return nil, nil, err
	}
	rows, err := db.Query(`SELECT look, area1, area2, materials, note FROM petdyeareas`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := map[uint32]PetDyeArea{}
	var bad []string
	for rows.Next() {
		var look int64
		var a PetDyeArea
		var mats string
		if err := rows.Scan(&look, &a.Area1, &a.Area2, &mats, &a.Note); err != nil {
			return nil, nil, err
		}
		m, err := ParsePetDyeMaterials(mats)
		if err != nil {
			bad = append(bad, fmt.Sprintf("petdyeareas 0x%08X: %v", uint32(look), err))
			continue
		}
		a.Look, a.Mask = uint32(look), m
		out[a.Look] = a
	}
	return out, bad, rows.Err()
}

// PetDyesOf is a character's pet dyes by body: [area 1, area 2], 0 undyed (a
// colour not ValidDye is none).
func (db *SQLiteDB) PetDyesOf(serverID uint32) (map[uint32][PetDyeAreas]uint32, error) {
	if err := db.ensurePetDyes(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT look, area1, area2 FROM petdyes WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32][PetDyeAreas]uint32{}
	for rows.Next() {
		var look, a1, a2 int64
		if err := rows.Scan(&look, &a1, &a2); err != nil {
			return nil, err
		}
		var d [PetDyeAreas]uint32
		for i, c := range []int64{a1, a2} {
			if u, ok := dye32(c); ok {
				d[i] = u
			}
		}
		if d != ([PetDyeAreas]uint32{}) {
			out[uint32(look)] = d
		}
	}
	return out, rows.Err()
}

// SetPetDye dyes one area (0 or 1) of a character's pet body (color 0 takes
// the dye off), writing the change and its gm_audit line in one transaction
// as SetCharDye does, so the GM panel lists and reverts it. A row left with
// no dye is deleted. It returns the colour the area had and the audit line's
// id (0 when nothing changed).
func (db *SQLiteDB) SetPetDye(serverID, look uint32, area int, color uint32, who GMEdit, detail string) (before uint32, audit int64, err error) {
	if area < 0 || area >= PetDyeAreas {
		return 0, 0, fmt.Errorf("no pet dye area %d", area)
	}
	if color != 0 && !ValidDye(int64(color)) {
		return 0, 0, fmt.Errorf("colour %08X has alpha 0", color)
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, 0, err
	}
	if err := db.ensurePetDyes(); err != nil {
		return 0, 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if who.Username == "" {
		if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username); err != nil {
			who.Username = fmt.Sprintf("account %d", who.AccountID)
		}
	}
	const sel = `SELECT * FROM petdyes WHERE serverid = ? AND look = ?`
	prev, err := fullRow(tx, sel, serverID, int64(look))
	if err != nil {
		return 0, 0, err
	}
	var cur [PetDyeAreas]uint32
	if prev != nil {
		var a1, a2 int64
		if err := tx.QueryRow(`SELECT area1, area2 FROM petdyes WHERE serverid = ? AND look = ?`, serverID, int64(look)).Scan(&a1, &a2); err != nil {
			return 0, 0, err
		}
		for i, c := range []int64{a1, a2} {
			if u, ok := dye32(c); ok {
				cur[i] = u
			}
		}
	}
	before = cur[area]
	if before == color {
		return before, 0, nil
	}
	cur[area] = color
	col := []string{"area1", "area2"}[area]
	action := "update"
	switch {
	case cur == [PetDyeAreas]uint32{}:
		action = "delete"
		_, err = tx.Exec(`DELETE FROM petdyes WHERE serverid = ? AND look = ?`, serverID, int64(look))
	case prev == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO petdyes (serverid, look, area1, area2, note) VALUES (?, ?, ?, ?, ?)`,
			serverID, int64(look), int64(cur[0]), int64(cur[1]), detail)
	default:
		_, err = tx.Exec(`UPDATE petdyes SET `+col+` = ? WHERE serverid = ? AND look = ?`, int64(color), serverID, int64(look))
	}
	if err != nil {
		return 0, 0, err
	}
	after, err := fullRow(tx, sel, serverID, int64(look))
	if err != nil {
		return 0, 0, err
	}
	key := map[string]any{"serverid": int64(serverID), "look": int64(look)}
	if audit, err = auditRowChange(tx, who, action, "petdyes", key, prev, after, detail); err != nil {
		return 0, 0, err
	}
	return before, audit, tx.Commit()
}
