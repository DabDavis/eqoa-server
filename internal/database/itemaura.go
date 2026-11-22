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
		note      TEXT NOT NULL DEFAULT '',
		color2    INTEGER NOT NULL DEFAULT 0,
		kind2     INTEGER NOT NULL DEFAULT 0,
		source2   INTEGER NOT NULL DEFAULT 0,
		nodes2    INTEGER NOT NULL DEFAULT 0
	);`

// itemAuraSecondCols are a held item's second aura's columns (the owner,
// 2026-11-16: "also give icing death the sword of virtue aura blue and also
// the freezing so we can have 2 auras now"): color2, kind2, source2, nodes2,
// as color, kind, source, nodes are the first's. All 0 (the default) none. A
// second aura is a held item's alone (HandAuraKind: the particles, an epic's
// look, an energy skin), sent after the hands' first auras to a client that
// asked for it (the live look's bit 7, entity.SecondAuraChannel); an older
// server never reads the columns, an older client is never sent them.
var itemAuraSecondCols = []string{"color2", "kind2", "source2", "nodes2"}

// ensureItemAura makes table itemaura (initSchema), and gives a table made
// before the second auras (2026-11-16) their columns.
func (db *SQLiteDB) ensureItemAura() error {
	if _, err := db.Exec(itemAuraSchema); err != nil {
		return fmt.Errorf("itemaura: %w", err)
	}
	for _, col := range itemAuraSecondCols {
		has, err := hasColumn(db.DB, "itemaura", col)
		if err != nil {
			return fmt.Errorf("itemaura: %w", err)
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE itemaura ADD COLUMN ` + col + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
				return fmt.Errorf("itemaura: %s: %w", col, err)
			}
		}
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // one connection: closed before the next query
	return out, db.legacyAuras(out)
}

// ValidItemAuraSecond holds an itemaura row's second aura (kind2, nodes2): a
// held item's kind (HandAuraKind) with its along (0-AuraAlongMost), or none
// (kind 0 with nothing else: the columns' default; or AuraNone).
func ValidItemAuraSecond(kind, nodes int64) bool {
	if kind == AuraDefault || kind == AuraNone {
		return nodes == 0
	}
	return HandAuraKind(kind) && nodes >= 0 && nodes <= AuraAlongMost
}

// SecondShows reports whether a second aura draws anything: one of a held
// item's kinds (a default or none draws nothing there).
func (a ItemAura) SecondShows() bool { return HandAuraKind(int64(a.Kind)) && a.Shows() }

// ItemSecondAuras is every itemaura row's second aura that draws something
// (SecondShows), by patternid: a held item's second (entity.pieceAuras). A row
// out of the client's ranges left out.
func (db *SQLiteDB) ItemSecondAuras() (map[int]ItemAura, error) {
	rows, err := db.Query(`SELECT patternid, color2, kind2, source2, nodes2 FROM itemaura WHERE kind2 != 0`)
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
		if !cok || !sok || !ValidItemAuraSecond(kind, nodes) {
			continue
		}
		if a := (ItemAura{PatternID: id, Color: c, Kind: uint8(kind), Source: src, Nodes: uint8(nodes)}); a.SecondShows() {
			out[id] = a
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // one connection: closed before the next query
	return out, db.legacyAuras(out)
}

// SetItemAuraSecondAudited writes item a.PatternID's second aura (an aura that
// draws nothing clears the columns; the row made, with no first aura, when
// there is none; a row left with neither deleted), with an audit line: a GM's
// /aura item <id> second. It reports whether an itempattern of that id exists
// (none: no change). The aura must be a held item's (ValidItemAuraSecond).
func (db *SQLiteDB) SetItemAuraSecondAudited(a ItemAura, who GMEdit, detail string) (bool, error) {
	if a.Kind != AuraDefault && a.Kind != AuraNone && !ValidItemAuraSecond(int64(a.Kind), int64(a.Nodes)) {
		return false, fmt.Errorf("a second aura is a held item's: particles, epic or energy")
	}
	if !a.SecondShows() {
		a = ItemAura{PatternID: a.PatternID}
	}
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
	case before == nil && !a.SecondShows():
		return true, nil
	case before == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO itemaura (patternid, kind, color2, kind2, source2, nodes2) VALUES (?, ?, ?, ?, ?, ?)`,
			a.PatternID, AuraNone, int64(a.Color), a.Kind, int64(a.Source), a.Nodes)
	default:
		_, err = tx.Exec(`UPDATE itemaura SET color2 = ?, kind2 = ?, source2 = ?, nodes2 = ? WHERE patternid = ?`,
			int64(a.Color), a.Kind, int64(a.Source), a.Nodes, a.PatternID)
		if err == nil && !a.SecondShows() {
			// neither aura left: the row goes, as a first aura cleared does
			var c, k int64
			if err = tx.QueryRow(`SELECT color, kind FROM itemaura WHERE patternid = ?`, a.PatternID).Scan(&c, &k); err == nil {
				if cc, ok := tint32(c); ok && !AuraShows(cc, uint8(k)) {
					action = "delete"
					_, err = tx.Exec(`DELETE FROM itemaura WHERE patternid = ?`, a.PatternID)
				}
			}
		}
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

// itemAuraHasSecond reports whether item id's itemaura row (in tx) carries a
// second aura that draws (the first cleared keeps the row for it).
func itemAuraHasSecond(tx *sql.Tx, id int) (bool, error) {
	var c, k, s, n int64
	err := tx.QueryRow(`SELECT color2, kind2, source2, nodes2 FROM itemaura WHERE patternid = ?`, id).Scan(&c, &k, &s, &n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	cc, _ := tint32(c)
	ss, _ := tint32(s)
	return ItemAura{Color: cc, Kind: uint8(k), Source: ss, Nodes: uint8(n)}.SecondShows(), nil
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
	second := false
	if before != nil {
		if second, err = itemAuraHasSecond(tx, a.PatternID); err != nil {
			return true, err
		}
	}
	switch {
	case !a.Shows() && second:
		// the first aura cleared, the second kept: the row stays, its first none
		_, err = tx.Exec(`UPDATE itemaura SET color = 0, kind = ?, source = 0, nodes = 0 WHERE patternid = ?`, AuraNone, a.PatternID)
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

// legacyAuras gives each Legacy record (legacy.go) without a row of its own its
// record's aura in m.
func (db *SQLiteDB) legacyAuras(m map[int]ItemAura) error {
	legacy, err := LegacyMap(db.DB)
	if err != nil {
		return err
	}
	for l, r := range legacy {
		if a, ok := m[r]; ok {
			if _, own := m[l]; !own {
				a.PatternID = l
				m[l] = a
			}
		}
	}
	return nil
}
