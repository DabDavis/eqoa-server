package database

import "fmt"

// charaura: an aura on ONE character's worn piece or body, not on the item
// (the owner, 2026-11-13: "lets make it where the gm command only affects that
// players armor pieces only that is targeted. lets allow players to receive a
// wholebody just applies it to body and underclothes, armor will cover it up,
// but can be /aura separately"; claude-notes/auras-20261113.md). /aura on a
// player (or yourself, or a mercenary) writes it; itemaura (an item that
// always glows) stays for the panel. Where both name a piece, charaura wins
// (entity/pieceaura.go).
//
// charid is the character's server_id; merc 0 the character itself, 1.. its
// mercenary of that hire slot (keyed by its owner's character); slot
// CharAuraChest ... CharAuraRobe as the channel's pieces, CharAuraBody the
// body and underclothes under them, CharAuraCape the worn cape, CharAuraMainHand
// and CharAuraOffHand the item held in each hand (2026-11-14: kind
// AuraParticles or AuraEpic alone, ValidCharAuraKind). The columns as itemaura's.
const charAuraSchema = `
	CREATE TABLE IF NOT EXISTS charaura (
		charid INTEGER NOT NULL,
		merc   INTEGER NOT NULL DEFAULT 0,
		slot   INTEGER NOT NULL,
		color  INTEGER NOT NULL DEFAULT 0,
		kind   INTEGER NOT NULL DEFAULT 0,
		source INTEGER NOT NULL DEFAULT 0,
		nodes  INTEGER NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (charid, merc, slot)
	);`

// CharAuraSlots is how many slots a character's aura has: the seven pieces
// (entity.AuraChest ... AuraRobe), the body, the cape (entity.AuraCape,
// 2026-11-14), and the held items, the main hand's and the off hand's
// (entity.AuraMainHand, AuraOffHand, 2026-11-14: the owner's "give zaeires
// equipped weapons the necro's tainted aura from the epic sickle"; the off
// hand's is on a shield held there too).
const (
	CharAuraBody     = 7
	CharAuraCape     = 8
	CharAuraMainHand = 9
	CharAuraOffHand  = 10
	CharAuraSlots    = 11
)

// ensureCharAura makes table charaura (initSchema).
func (db *SQLiteDB) ensureCharAura() error {
	if _, err := db.Exec(charAuraSchema); err != nil {
		return fmt.Errorf("charaura: %w", err)
	}
	return nil
}

// CharAuraKey is whose a charaura row is: a character (Merc 0) or its
// mercenary of a hire slot.
type CharAuraKey struct {
	CharID uint32
	Merc   int
}

// CharAuras is every row of charaura that draws something, by whose, each
// slot's (an aura that draws nothing, or out of the client's ranges, none).
func (db *SQLiteDB) CharAuras() (map[CharAuraKey][CharAuraSlots]ItemAura, error) {
	rows, err := db.Query(`SELECT charid, merc, slot, color, kind, source, nodes FROM charaura`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[CharAuraKey][CharAuraSlots]ItemAura)
	for rows.Next() {
		var charid, merc, slot, color, kind, source, nodes int64
		if err := rows.Scan(&charid, &merc, &slot, &color, &kind, &source, &nodes); err != nil {
			return nil, err
		}
		c, cok := tint32(color)
		src, sok := tint32(source)
		if !cok || !sok || !ValidCharAuraKind(slot, kind) || !ValidAuraNodes(nodes) || slot < 0 || slot >= CharAuraSlots || charid <= 0 || merc < 0 {
			continue
		}
		a := ItemAura{Color: c, Kind: uint8(kind), Source: src, Nodes: uint8(nodes)}
		if !a.Shows() {
			continue
		}
		k := CharAuraKey{CharID: uint32(charid), Merc: int(merc)}
		s := out[k]
		s[slot] = a
		out[k] = s
	}
	return out, rows.Err()
}

// SetCharAuraAudited writes whose slot's aura (a row made when there is
// none; an aura that draws nothing deletes it), with an audit line: a GM's
// /aura on a character or a mercenary.
func (db *SQLiteDB) SetCharAuraAudited(k CharAuraKey, slot int, a ItemAura, who GMEdit, detail string) error {
	if slot < 0 || slot >= CharAuraSlots || k.CharID == 0 {
		return fmt.Errorf("charaura: no slot %d of character %d", slot, k.CharID)
	}
	if a.Shows() && !ValidCharAuraKind(int64(slot), int64(a.Kind)) {
		return fmt.Errorf("charaura: slot %d takes no aura of kind %d (a held item's: particles or epic; epic only a held item's)", slot, a.Kind)
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	if err := db.ensureCharAura(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	key := map[string]any{"charid": int64(k.CharID), "merc": k.Merc, "slot": slot}
	const sel = `SELECT * FROM charaura WHERE charid = ? AND merc = ? AND slot = ?`
	before, err := fullRow(tx, sel, k.CharID, k.Merc, slot)
	if err != nil {
		return err
	}
	action := "update"
	switch {
	case !a.Shows():
		if before == nil {
			return nil
		}
		action = "delete"
		_, err = tx.Exec(`DELETE FROM charaura WHERE charid = ? AND merc = ? AND slot = ?`, k.CharID, k.Merc, slot)
	case before == nil:
		action = "insert"
		_, err = tx.Exec(`INSERT INTO charaura (charid, merc, slot, color, kind, source, nodes) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			k.CharID, k.Merc, slot, int64(a.Color), a.Kind, int64(a.Source), a.Nodes)
	default:
		_, err = tx.Exec(`UPDATE charaura SET color = ?, kind = ?, source = ?, nodes = ? WHERE charid = ? AND merc = ? AND slot = ?`,
			int64(a.Color), a.Kind, int64(a.Source), a.Nodes, k.CharID, k.Merc, slot)
	}
	if err != nil {
		return err
	}
	after, err := fullRow(tx, sel, k.CharID, k.Merc, slot)
	if err != nil {
		return err
	}
	if _, err := auditRowChange(tx, who, action, "charaura", key, before, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}
