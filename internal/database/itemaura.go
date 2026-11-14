package database

import (
	"database/sql"
	"fmt"
)

// itemaura: an aura on a worn piece of armour or a robe, for a client that
// draws one (entity/pieceaura.go; the owner, 2026-11-13: "not just a template
// for whole models, but even pieces of armor like bracers only if that piece
// has aura. or only a breastplate etc. and they can have their own aura color
// each"; claude-notes/auras-20261113.md). Keyed by the item's record
// (itempattern.patternid), so every copy of the item wears it; a character
// wearing it shows it on that piece alone (chest, bracer, gloves, legs, boots,
// helm or robe) to every client that asked for the pieces' auras (vanilla's
// LIVE_LOOK_AURAS). A PS2 is never told.
//
// color: 0xRRGGBBAA, the aura's colour (the rgb at the alpha's share; alpha 0
// none: particles in the source's own colours). kind: AuraDefault (the shell
// while the colour lights), AuraNone, AuraShell, AuraParticles, AuraBoth,
// AuraTrim, AuraTrimShell; a black or near-black colour (AuraDark) with a
// shell or trim named is a dark aura.
// source: the DictID of the item or model whose particle sprite the particles
// are, 0 the Sword of Virtue's (0x88EC092D). nodes: on how many of the piece's
// nodes, 0 the default. A row that draws nothing (AuraShows) is left out.
const itemAuraSchema = `
	CREATE TABLE IF NOT EXISTS itemaura (
		patternid INTEGER PRIMARY KEY,
		color     INTEGER NOT NULL DEFAULT 0,
		kind      INTEGER NOT NULL DEFAULT 0,
		source    INTEGER NOT NULL DEFAULT 0,
		nodes     INTEGER NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT ''
	);`

// ensureItemAura makes table itemaura (initSchema).
func (db *SQLiteDB) ensureItemAura() error {
	if _, err := db.Exec(itemAuraSchema); err != nil {
		return fmt.Errorf("itemaura: %w", err)
	}
	return nil
}

// ItemAura is a row of itemaura, read.
type ItemAura struct {
	PatternID int
	Color     uint32 // 0xRRGGBBAA, 0 none
	Kind      uint8
	Source    uint32 // a DictID, 0 the Sword of Virtue's
	Nodes     uint8  // 0 the default
}

// Shows reports whether the aura draws anything.
func (a ItemAura) Shows() bool { return AuraShows(a.Color, a.Kind) }

// ItemAuras is every row of itemaura that draws something, by patternid (a
// row out of the client's ranges left out).
func (db *SQLiteDB) ItemAuras() (map[int]ItemAura, error) {
	rows, err := db.Query(`SELECT patternid, color, kind, source, nodes FROM itemaura`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]ItemAura)
	for rows.Next() {
		var id int
		var color, kind, source, nodes int64
		if err := rows.Scan(&id, &color, &kind, &source, &nodes); err != nil {
			return nil, err
		}
		c, cok := tint32(color)
		src, sok := tint32(source)
		if !cok || !sok || !ValidItemAura(kind, nodes) {
			continue
		}
		if a := (ItemAura{PatternID: id, Color: c, Kind: uint8(kind), Source: src, Nodes: uint8(nodes)}); a.Shows() {
			out[id] = a
		}
	}
	return out, rows.Err()
}

// SetItemAuraAudited writes item patternid's aura (a row made when there is
// none; an aura that draws nothing deletes it), with an audit line: a GM's
// /aura item. It reports whether an itempattern of that id exists (none: no
// change).
func (db *SQLiteDB) SetItemAuraAudited(a ItemAura, who GMEdit, detail string) (bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM itempattern WHERE patternid = ?`, a.PatternID).Scan(&n); err != nil || n == 0 {
		return false, err
	}
	fillWho(tx, &who)
	key := map[string]any{"patternid": a.PatternID}
	before, err := fullRow(tx, `SELECT * FROM itemaura WHERE patternid = ?`, a.PatternID)
	if err != nil {
		return true, err
	}
	action := "update"
	switch {
	case !a.Shows():
		if before == nil {
			return true, nil
		}
		action = "delete"
		_, err = tx.Exec(`DELETE FROM itemaura WHERE patternid = ?`, a.PatternID)
	case before == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO itemaura (patternid, color, kind, source, nodes) VALUES (?, ?, ?, ?, ?)`,
			a.PatternID, int64(a.Color), a.Kind, int64(a.Source), a.Nodes)
	default:
		_, err = tx.Exec(`UPDATE itemaura SET color = ?, kind = ?, source = ?, nodes = ? WHERE patternid = ?`,
			int64(a.Color), a.Kind, int64(a.Source), a.Nodes, a.PatternID)
	}
	if err != nil {
		return true, err
	}
	after, err := fullRow(tx, `SELECT * FROM itemaura WHERE patternid = ?`, a.PatternID)
	if err != nil {
		return true, err
	}
	if _, err := auditRowChange(tx, who, action, "itemaura", key, before, after, detail); err != nil {
		return true, err
	}
	return true, tx.Commit()
}

// SetNPCTintAuraAudited sets the glow and the aura of npctint's row for name
// (made, untinted, when there is none: its other columns kept when there is),
// with an audit line: a GM's /aura on a creature. glow is the aura's colour.
func (db *SQLiteDB) SetNPCTintAuraAudited(name string, glow uint32, kind uint8, source uint32, nodes uint8, who GMEdit, detail string) error {
	k := NPCClassKey(name)
	if k == "" {
		return fmt.Errorf("npctint: no name")
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	// the row of that key, whatever case a GM or a tool wrote its name in (of
	// two, the one NPCTints reads: the last by name)
	row, err := npcTintRowName(tx, k)
	if err != nil {
		return err
	}
	action := "update"
	if row == "" {
		row, action = k, "insert"
	}
	before, err := fullRow(tx, `SELECT * FROM npctint WHERE name = ?`, row)
	if err != nil {
		return err
	}
	if action == "insert" {
		_, err = tx.Exec(`INSERT INTO npctint (name, glow, aura_kind, aura_source, aura_nodes, source) VALUES (?, ?, ?, ?, ?, 'gm')`,
			row, int64(glow), kind, int64(source), nodes)
	} else {
		_, err = tx.Exec(`UPDATE npctint SET glow = ?, aura_kind = ?, aura_source = ?, aura_nodes = ? WHERE name = ?`,
			int64(glow), kind, int64(source), nodes, row)
	}
	if err != nil {
		return err
	}
	after, err := fullRow(tx, `SELECT * FROM npctint WHERE name = ?`, row)
	if err != nil {
		return err
	}
	if _, err := auditRowChange(tx, who, action, "npctint", map[string]any{"name": row}, before, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}

// npcTintRowName is the name of npctint's row that keys to k, "" none.
func npcTintRowName(tx *sql.Tx, k string) (string, error) {
	rows, err := tx.Query(`SELECT name FROM npctint ORDER BY name`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	found := ""
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		if NPCClassKey(n) == k {
			found = n
		}
	}
	return found, rows.Err()
}
