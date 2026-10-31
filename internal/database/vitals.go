package database

import (
	"database/sql"
	"time"
)

// A character's hit points and power over a logout (the user, 2026-10-31:
// "i just logged out with jedd and i fully healed and mobs did not kill
// me"). Until then every login filled both, so a quit mid-fight was a free
// heal. The session's end keeps them (charvitals) and when; the next login
// takes them, with what its HoT and PoT give back over the time away (the
// user: "they essentially rested"), within the maxima its gear and effects
// give it then, and the row goes, so a session that ends with no save (a
// crash) comes back whole rather than with an older session's wounds. No
// row: whole, as before.

// SaveVitals keeps the hit points and power a character left the world with,
// replacing what was kept before.
func (s *SQLiteCharacterStore) SaveVitals(serverID uint32, hp, power int) error {
	_, err := s.db.Exec(`INSERT INTO charvitals (serverid, hp, power, saved) VALUES (?, ?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET hp = excluded.hp, power = excluded.power, saved = excluded.saved`,
		serverID, hp, power, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ClearVitals forgets them: the character comes back whole (one that left
// dead, its respawn's).
func (s *SQLiteCharacterStore) ClearVitals(serverID uint32) error {
	_, err := s.db.Exec(`DELETE FROM charvitals WHERE serverid = ?`, serverID)
	return err
}

// TakeVitals is what SaveVitals kept, taken out, and when it was kept (the
// zero time when the row says nothing readable): ok is false when nothing
// was.
func (s *SQLiteCharacterStore) TakeVitals(serverID uint32) (hp, power int, saved time.Time, ok bool, err error) {
	var at string
	err = s.db.QueryRow(`SELECT hp, power, saved FROM charvitals WHERE serverid = ?`, serverID).Scan(&hp, &power, &at)
	if err == sql.ErrNoRows {
		return 0, 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, 0, time.Time{}, false, err
	}
	if _, err = s.db.Exec(`DELETE FROM charvitals WHERE serverid = ?`, serverID); err != nil {
		return 0, 0, time.Time{}, false, err
	}
	saved, _ = time.Parse(time.RFC3339, at)
	return hp, power, saved, true, nil
}
