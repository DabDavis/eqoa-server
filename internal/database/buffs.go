package database

// BuffRow is one of a character's spell effects as it is kept over a logout
// (charbuffs): what is left of it and of its next tick, counted while the
// character is in the world, and what it does (entity/buffsave.go).
type BuffRow struct {
	SpellID   int
	Name      string
	Icon      uint32
	Caster    uint32 // the caster's character (serverid)
	LeftMS    int64
	TickMS    int64 // to the next tick; -1 for none
	HPTick    int
	PowerTick int
	Mods      string // the stat changes, JSON
	Flags     int    // 1 root, 2 snare, 4 speed, 8 invisibility
}

// SaveBuffs replaces a character's kept effects with rows, in their order.
func (s *SQLiteCharacterStore) SaveBuffs(serverID uint32, rows []BuffRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM charbuffs WHERE serverid = ?`, serverID); err != nil {
		tx.Rollback()
		return err
	}
	for i, r := range rows {
		if _, err := tx.Exec(`INSERT INTO charbuffs (serverid, seq, spellid, name, icon, caster, left_ms, tick_ms,
			hp_tick, power_tick, mods, flags) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			serverID, i, r.SpellID, r.Name, int64(r.Icon), int64(r.Caster), r.LeftMS, r.TickMS,
			r.HPTick, r.PowerTick, r.Mods, r.Flags); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LoadBuffs is a character's kept effects, in their order.
func (s *SQLiteCharacterStore) LoadBuffs(serverID uint32) ([]BuffRow, error) {
	rows, err := s.db.Query(`SELECT spellid, name, icon, caster, left_ms, tick_ms, hp_tick, power_tick, mods, flags
		FROM charbuffs WHERE serverid = ? ORDER BY seq`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BuffRow
	for rows.Next() {
		var r BuffRow
		var icon, caster int64
		if err := rows.Scan(&r.SpellID, &r.Name, &icon, &caster, &r.LeftMS, &r.TickMS, &r.HPTick, &r.PowerTick, &r.Mods, &r.Flags); err != nil {
			return nil, err
		}
		r.Icon, r.Caster = uint32(icon), uint32(caster)
		out = append(out, r)
	}
	return out, rows.Err()
}
