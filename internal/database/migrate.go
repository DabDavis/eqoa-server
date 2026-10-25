package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// MigrateTotalXP carries every character's kept experience (totalXP) to
// another level table once: convert gives a row's new total from its level
// and old total (entity.ConvertTotalXP keeps the level and the share of it).
// It is recorded in servermeta under key "xp_table" as version; a database
// already at that version is left as it is. It reports how many rows it
// changed.
func (s *SQLiteCharacterStore) MigrateTotalXP(version string, convert func(level, total int) int) (int, error) {
	var at string
	switch err := s.db.QueryRow(`SELECT value FROM servermeta WHERE key = 'xp_table'`).Scan(&at); {
	case err == nil && at == version:
		return 0, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return 0, err
	}
	type row struct{ id, level, total int }
	rows, err := s.db.Query(`SELECT serverid, level, totalXP FROM characters`)
	if err != nil {
		return 0, err
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.level, &r.total); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, r := range all {
		if n := convert(r.level, r.total); n != r.total {
			if _, err := tx.Exec(`UPDATE characters SET totalXP = ? WHERE serverid = ?`, n, r.id); err != nil {
				tx.Rollback()
				return 0, fmt.Errorf("character %d: %w", r.id, err)
			}
			changed++
		}
	}
	if _, err := tx.Exec(`INSERT INTO servermeta (key, value) VALUES ('xp_table', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, version); err != nil {
		tx.Rollback()
		return 0, err
	}
	return changed, tx.Commit()
}
