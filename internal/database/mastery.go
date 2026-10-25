package database

import "database/sql"

// MasteryRow is a character's class mastery points (charmastery): unspent
// and spent, the share of its experience going to them (0-100) and the
// experience gathered toward the next point. A character with no row has
// none, and none of its experience goes to them.
type MasteryRow struct {
	Points, Spent, Percent, XP int
}

// LoadMastery is a character's row of charmastery, or a zero one.
func (s *SQLiteCharacterStore) LoadMastery(serverID uint32) (MasteryRow, error) {
	var m MasteryRow
	err := s.db.QueryRow(`SELECT points, spent, percent, xp FROM charmastery WHERE serverid = ?`, serverID).
		Scan(&m.Points, &m.Spent, &m.Percent, &m.XP)
	if err == sql.ErrNoRows {
		return MasteryRow{}, nil
	}
	return m, err
}

// SaveMastery keeps a character's mastery points.
func (s *SQLiteCharacterStore) SaveMastery(serverID uint32, m MasteryRow) error {
	_, err := s.db.Exec(`INSERT INTO charmastery (serverid, points, spent, percent, xp) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET points = excluded.points, spent = excluded.spent,
		percent = excluded.percent, xp = excluded.xp`, serverID, m.Points, m.Spent, m.Percent, m.XP)
	return err
}
