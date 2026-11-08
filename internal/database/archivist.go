package database

import "strings"

// What the Spell Archivist's grant reads and his grant and clear write
// (handlers/archivist.go; the GM Jibrish's petition 1279, 2026-11-08: "Grant
// basic spells", the spells of a player's class a merchant sells, and a clear
// book kept but for Return Home).

// MerchantScroll is a spell scroll a merchant sells: its record's id, name,
// level and class and race masks, which the scribe checks (handlers/scribe.go).
type MerchantScroll struct {
	PatternID int
	Name      string
	LevelReq  int
	ClassUse  int
	RaceUse   int
}

// MerchantScrolls is every spell scroll a merchant sells, in record order: an
// item record with the scroll art (icon, stored signed or not) worn nowhere, in
// the npcitem list of an NPC with the merchant flag (the low byte of npc_type,
// bit 0x01, which CLIENT opens a shop for). The flag matters: a creature's
// npcitem rows are what its corpse holds (handlers/loot.go), and a scroll a
// corpse drops is no merchant's.
func (s *SpellbookStore) MerchantScrolls(icon int) ([]MerchantScroll, error) {
	signed := int64(int32(icon))
	rows, err := s.db.Query(`SELECT DISTINCT p.patternid, p.itemname, p.levelreq, p.classuse, p.raceuse
		FROM npcitem n
		JOIN itempattern p ON p.patternid = n.patternid
		JOIN (SELECT DISTINCT npc_id FROM npcs WHERE npc_id != 0 AND (npc_type & 1) = 1) m ON m.npc_id = n.npcid
		WHERE p.itemicon IN (?, ?) AND p.equipslot = -1
		ORDER BY p.patternid`, signed, int64(uint32(signed)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MerchantScroll
	for rows.Next() {
		var m MerchantScroll
		if err := rows.Scan(&m.PatternID, &m.Name, &m.LevelReq, &m.ClassUse, &m.RaceUse); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ClassSpellSources is every classspells row's source by spell, then class:
// which classes have a spell, and how each gets it (GotBy, Granted).
func (s *SpellbookStore) ClassSpellSources() (map[int]map[int]string, error) {
	rows, err := s.db.Query(`SELECT class, spellid, COALESCE(source, '') FROM classspells`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]map[int]string{}
	for rows.Next() {
		var class, id int
		var source string
		if err := rows.Scan(&class, &id, &source); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[int]string{}
		}
		out[id][class] = source
	}
	return out, rows.Err()
}

// SpellDescriptions is every spell pattern's description by id: what tells a
// were's ability from a class's beside its name (IsWereAbility).
func (s *SpellbookStore) SpellDescriptions() (map[int]string, error) {
	rows, err := s.db.Query(`SELECT spellid, COALESCE(spelldesc, '') FROM spellpattern`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var desc string
		if err := rows.Scan(&id, &desc); err != nil {
			return nil, err
		}
		out[id] = desc
	}
	return out, rows.Err()
}

// AddSpells puts spells in a character's book at display indices order,
// order+1 and on, in the order given, as AddSpell puts one (off the bar, no
// lockout, -1 at VIAbility+0x14), in one transaction: none or all of those
// below 252 (the book shows no display index past it). It returns how many
// went in.
func (s *SpellbookStore) AddSpells(serverID uint32, ids []int, order int) (int, error) {
	n := 0
	for n < len(ids) && order+n < 252 {
		n++
	}
	if n == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for i, id := range ids[:n] {
		if _, err := tx.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
			VALUES (?, ?, ?, 0, -1, 0, -1)`, serverID, id, order+i); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// RemoveSpells takes spells out of a character's book, every row of each, in
// one transaction. It returns how many rows went.
func (s *SpellbookStore) RemoveSpells(serverID uint32, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	gone := 0
	for _, id := range ids {
		res, err := tx.Exec(`DELETE FROM spells WHERE serverid = ? AND spellid = ?`, serverID, id)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		gone += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return gone, nil
}

// IsReturnHome is whether a spell of the name is Return Home: every record so
// named, any case (1623 as Sandstorm numbers it, 1542, and the wizards' 2300).
func IsReturnHome(name string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(name), " "), "Return Home")
}
