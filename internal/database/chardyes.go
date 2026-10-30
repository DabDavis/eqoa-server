package database

import (
	"fmt"
	"strconv"
	"strings"
)

// chardyes: a character's dyes, a colour for one of its armour slots, held
// items or its robe, which a client of SOE's live game's look draws
// (entity/livelook.go, protocol.OpcodeLiveLook): the live game's entity
// channel and character list carry an RGBA colour per slot where the disc's
// carry a tint index. A slot with no row is undyed and is sent the colour of
// the tint the disc's look carries, so it looks exactly as it does to a PS2.
//
// slot: 0 chest, 1 bracer, 2 gloves, 3 legs, 4 boots, 5 helm (the channel's
// armour order), 6 primary, 7 secondary, 8 shield (its three held items), 9
// robe. color: 0xRRGGBBAA, red in the high byte, as itempattern.color holds
// Return Home's item colours and the live client reads the bytes (red
// first); alpha is never 0 (a slot drawn in it would vanish). Set by #dye in
// the game and the GM panel's "Character dyes"; source gm.
const charDyesSchema = `
	CREATE TABLE IF NOT EXISTS chardyes (
		serverid INTEGER NOT NULL,
		slot     INTEGER NOT NULL,
		color    INTEGER NOT NULL,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (serverid, slot)
	);`

// Dye slots, as chardyes numbers them.
const (
	DyeChest = iota
	DyeBracer
	DyeGloves
	DyeLegs
	DyeBoots
	DyeHelm
	DyePrimary
	DyeSecondary
	DyeShield
	DyeRobe
	DyeSlots
)

// DyeSlotNames are the slots' words (#dye, the panel's help).
var DyeSlotNames = [DyeSlots]string{"chest", "bracer", "gloves", "legs", "boots", "helm", "primary", "secondary", "shield", "robe"}

// dyeSlotWords are other words for a slot.
var dyeSlotWords = map[string]int{
	"bracers": DyeBracer, "forearm": DyeBracer, "arms": DyeBracer, "glove": DyeGloves, "hands": DyeGloves,
	"leg": DyeLegs, "feet": DyeBoots, "boot": DyeBoots, "head": DyeHelm, "helmet": DyeHelm,
	"main": DyePrimary, "mainhand": DyePrimary, "weapon": DyePrimary, "off": DyeSecondary, "offhand": DyeSecondary,
}

// DyeSlotByName is a slot by its word (or its number), and whether there is one.
func DyeSlotByName(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for i, n := range DyeSlotNames {
		if s == n {
			return i, true
		}
	}
	if i, ok := dyeSlotWords[s]; ok {
		return i, true
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n < DyeSlots {
		return n, true
	}
	return 0, false
}

// DiscTints is VICSpriteCust::Tints (SUPPORT 0x004E0C08, decomp
// vicspritecust.cpp), RGBA: the colour a disc tint index draws a slot in.
// The live game sent an old tint as its colour (Lostologist's legs faaf32ff,
// tint 6).
var DiscTints = [15][4]byte{
	{255, 255, 255, 255}, {90, 90, 90, 255}, {0, 180, 255, 255}, {150, 100, 50, 255}, {0, 0, 140, 255},
	{0, 200, 50, 255}, {250, 175, 50, 255}, {80, 100, 0, 255}, {240, 60, 60, 255}, {165, 245, 255, 255},
	{160, 170, 180, 255}, {220, 190, 150, 255}, {0, 200, 180, 255}, {150, 0, 60, 255}, {240, 240, 0, 255},
}

// RGBA is a colour as chardyes holds it, 0xRRGGBBAA.
func RGBA(c [4]byte) uint32 {
	return uint32(c[0])<<24 | uint32(c[1])<<16 | uint32(c[2])<<8 | uint32(c[3])
}

// dyeNames are colours a GM may name.
var dyeNames = map[string]uint32{
	"white": 0xFFFFFFFF, "black": 0x101010FF, "grey": 0x808080FF, "gray": 0x808080FF, "red": 0xD10000FF,
	"green": 0x008000FF, "blue": 0x0000C0FF, "yellow": 0xF0F000FF, "orange": 0xFF8000FF, "purple": 0x400080FF,
	"brown": 0x966432FF, "pink": 0xFF5BADFF, "teal": 0x00C8B4FF, "gold": 0xFAAF32FF,
}

// ParseDyeColor reads a colour as a GM writes one: RRGGBB (opaque) or
// RRGGBBAA in hex, with or without a leading # or 0x; tN, the disc tint N's
// colour (0-14); or a name (red, green, blue, ...). "none", "off", "clear"
// and 0 are no dye (0). Alpha 0 is refused: the slot would vanish.
func ParseDyeColor(s string) (uint32, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "none", "off", "clear", "0", "":
		return 0, nil
	}
	if c, ok := dyeNames[s]; ok {
		return c, nil
	}
	if len(s) >= 2 && s[0] == 't' {
		if n, err := strconv.Atoi(s[1:]); err == nil {
			if n < 0 || n >= len(DiscTints) {
				return 0, fmt.Errorf("tint %d: the disc has 0 to %d", n, len(DiscTints)-1)
			}
			return RGBA(DiscTints[n]), nil
		}
	}
	h := strings.TrimPrefix(strings.TrimPrefix(s, "#"), "0x")
	if len(h) != 6 && len(h) != 8 {
		return 0, fmt.Errorf("%q is not a colour: RRGGBB or RRGGBBAA in hex (ff0000 red), tN (a disc tint), or a name", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a colour: RRGGBB or RRGGBBAA in hex", s)
	}
	c := uint32(v)
	if len(h) == 6 {
		c = c<<8 | 0xFF
	}
	if c&0xFF == 0 {
		return 0, fmt.Errorf("%q has alpha 0: the slot would not be drawn", s)
	}
	return c, nil
}

// dye32 is a chardyes colour's 32 bits, and whether it is one the server
// sends: 0xRRGGBBAA, alpha not 0. The table may hold it signed or unsigned
// (the panel stores a hex32 column as it is typed; the game takes the low 32
// bits of either).
func dye32(c int64) (uint32, bool) {
	if c < -(1<<31) || c > 0xFFFFFFFF {
		return 0, false
	}
	u := uint32(c)
	return u, u&0xFF != 0
}

// ValidDye reports whether a chardyes colour is one the server sends: a
// 32-bit 0xRRGGBBAA whose alpha is not 0.
func ValidDye(c int64) bool {
	_, ok := dye32(c)
	return ok
}

// FormatDye is a colour as #dye says it: RRGGBBAA, or "none".
func FormatDye(c uint32) string {
	if c == 0 {
		return "none"
	}
	return fmt.Sprintf("%08X", c)
}

// ensureCharDyes makes table chardyes (initSchema).
func (db *SQLiteDB) ensureCharDyes() error {
	if _, err := db.Exec(charDyesSchema); err != nil {
		return fmt.Errorf("chardyes: %w", err)
	}
	return nil
}

// CharDyes is a character's dyes by slot, 0 where it has none (a row out of
// range or not ValidDye is none).
func (db *SQLiteDB) CharDyes(serverID uint32) ([DyeSlots]uint32, error) {
	var out [DyeSlots]uint32
	rows, err := db.Query(`SELECT slot, color FROM chardyes WHERE serverid = ?`, serverID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var slot int
		var c int64
		if err := rows.Scan(&slot, &c); err != nil {
			return out, err
		}
		if u, ok := dye32(c); ok && slot >= 0 && slot < DyeSlots {
			out[slot] = u
		}
	}
	return out, rows.Err()
}

// SetCharDye dyes one slot of a character (color 0 takes the dye off),
// writing the change and its gm_audit line in one transaction as the GM
// panel writes a row edit, so the panel lists and reverts it. It returns the
// colour the slot had and the audit line's id (0 when nothing changed).
func (db *SQLiteDB) SetCharDye(serverID uint32, slot int, color uint32, who GMEdit, detail string) (before uint32, audit int64, err error) {
	if slot < 0 || slot >= DyeSlots {
		return 0, 0, fmt.Errorf("no dye slot %d", slot)
	}
	if color != 0 && !ValidDye(int64(color)) {
		return 0, 0, fmt.Errorf("colour %08X has alpha 0", color)
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, 0, err
	}
	if err := db.ensureCharDyes(); err != nil {
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
	const sel = `SELECT * FROM chardyes WHERE serverid = ? AND slot = ?`
	prev, err := fullRow(tx, sel, serverID, slot)
	if err != nil {
		return 0, 0, err
	}
	if prev != nil {
		var old int64
		if err := tx.QueryRow(`SELECT color FROM chardyes WHERE serverid = ? AND slot = ?`, serverID, slot).Scan(&old); err != nil {
			return 0, 0, err
		}
		if u, ok := dye32(old); ok {
			before = u
		}
	}
	if before == color && (prev != nil) == (color != 0) {
		return before, 0, nil
	}
	action := "update"
	switch {
	case color == 0:
		action = "delete"
		_, err = tx.Exec(`DELETE FROM chardyes WHERE serverid = ? AND slot = ?`, serverID, slot)
	case prev == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO chardyes (serverid, slot, color, note, source) VALUES (?, ?, ?, ?, 'gm')`,
			serverID, slot, int64(color), "dyed by GM "+who.Username+" in the game")
	default:
		_, err = tx.Exec(`UPDATE chardyes SET color = ? WHERE serverid = ? AND slot = ?`, int64(color), serverID, slot)
	}
	if err != nil {
		return 0, 0, err
	}
	after, err := fullRow(tx, sel, serverID, slot)
	if err != nil {
		return 0, 0, err
	}
	key := map[string]any{"serverid": int64(serverID), "slot": int64(slot)}
	if audit, err = auditRowChange(tx, who, action, "chardyes", key, prev, after, detail); err != nil {
		return 0, 0, err
	}
	return before, audit, tx.Commit()
}
