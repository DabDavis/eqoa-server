package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// charinfection: a character's animal-were infection, the one flag of it (work
// 1463, a GM's of 2026-11-14: "that debuff must be the infection
// flag"). A row a character: its line (wolfwere, bearwere, ratwere, lionwere,
// gatorwere), when it was caught and when it ends, in unix seconds of real time
// (seven days from its catching: an absolute end, so a logout, a zoning, a death
// or a restart does not stop its clock), the were that gave it, the roll's key
// (a grant retried is the same grant: its key already there is no second one),
// and how it ended (ended_at 0 while it has not: expired, cured, antidote,
// turned, the time it ended). It is active while ended_at is 0 and expires_at is
// after now; a new infection replaces an ended one's row. The line's hidden fever
// marker (charquests, quests 101422-101426: the third quests' prerequisite) is
// kept in step in the same transaction: there while the row is active, gone when
// it ends.
//
// charinfection_migration is the one-time migration of the fever markers held
// before the table (tools/were_infection_migrate_20261115.py, and the server's
// own for a marker it finds with no row: handlers/wereinfect.go), a row a marker:
// what the marker was (its done, step, count, slot, updated, picks: the undo puts
// it back), the infection made of it and how (active from its timestamp, a
// migration's seven days from now, expired, turned: the marker taken, a second
// marker of one character), when and by what. A marker once in it is never
// migrated again: a repeated start or rollout cannot renew it.
const charInfectionSchema = `
	CREATE TABLE IF NOT EXISTS charinfection (
		serverid   INTEGER PRIMARY KEY,
		line       TEXT NOT NULL,
		applied_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		source     TEXT NOT NULL DEFAULT '',
		grant_id   TEXT NOT NULL DEFAULT '',
		ended_at   INTEGER NOT NULL DEFAULT 0,
		ended_how  TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_charinfection_open ON charinfection(ended_at, expires_at);
	CREATE TABLE IF NOT EXISTS charinfection_migration (
		serverid    INTEGER NOT NULL,
		questid     INTEGER NOT NULL,
		line        TEXT NOT NULL,
		marker_done INTEGER NOT NULL,
		marker_step INTEGER NOT NULL DEFAULT 0,
		marker_count INTEGER NOT NULL DEFAULT 0,
		marker_slot INTEGER NOT NULL DEFAULT 0,
		marker_updated TEXT NOT NULL DEFAULT '',
		marker_picks TEXT NOT NULL DEFAULT '',
		applied_at  INTEGER NOT NULL DEFAULT 0,
		expires_at  INTEGER NOT NULL DEFAULT 0,
		outcome     TEXT NOT NULL,
		migrated_at TEXT NOT NULL,
		by          TEXT NOT NULL,
		PRIMARY KEY (serverid, questid)
	);`

// ensureCharInfection makes the infection's tables (initSchema).
func (db *SQLiteDB) ensureCharInfection() error {
	if _, err := db.Exec(charInfectionSchema); err != nil {
		return fmt.Errorf("charinfection: %w", err)
	}
	return nil
}

// Infection is a character's charinfection row.
type Infection struct {
	ServerID  uint32
	Line      string
	AppliedAt time.Time
	ExpiresAt time.Time
	Source    string
	Key       string
	EndedAt   time.Time // zero while it has not ended
	EndedHow  string
}

// Active reports the infection in force at now.
func (i *Infection) Active(now time.Time) bool {
	return i != nil && i.EndedAt.IsZero() && now.Before(i.ExpiresAt)
}

// The ways an infection ends.
const (
	InfectionExpired  = "expired"
	InfectionCured    = "cured"
	InfectionAntidote = "antidote"
	InfectionTurned   = "turned"
)

func unixOrZero(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

// CharInfection is a character's infection row, nil for none.
func (s *QuestStore) CharInfection(serverID uint32) (*Infection, error) {
	var i Infection
	var applied, expires, ended int64
	err := s.db.QueryRow(`SELECT serverid, line, applied_at, expires_at, source, grant_id, ended_at, ended_how
		FROM charinfection WHERE serverid = ?`, serverID).Scan(&i.ServerID, &i.Line, &applied, &expires, &i.Source, &i.Key, &ended, &i.EndedHow)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	i.AppliedAt, i.ExpiresAt, i.EndedAt = time.Unix(applied, 0), time.Unix(expires, 0), unixOrZero(ended)
	return &i, nil
}

// markerUpsert finishes a fever marker (a zero-step draft's row: done 1) in tx,
// as SaveCharQuest writes a quest finished.
func markerUpsert(tx *sql.Tx, serverID uint32, marker int, at time.Time) error {
	_, err := tx.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks)
		VALUES (?, ?, 0, 0, 1, 0, ?, '')
		ON CONFLICT(serverid, questid) DO UPDATE SET step = 0, count = 0, done = MAX(done, 1), updated = excluded.updated`,
		serverID, marker, at.UTC().Format(time.RFC3339))
	return err
}

// ApplyInfection gives the character an infection and its line's marker in one
// transaction: a row with the same key already there is the same grant (false,
// nil: nothing written), an active row of any key refuses it (an error). An
// ended row is replaced.
func (s *QuestStore) ApplyInfection(inf Infection, marker int) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var key string
	var ended, expires int64
	err = tx.QueryRow(`SELECT grant_id, ended_at, expires_at FROM charinfection WHERE serverid = ?`, inf.ServerID).Scan(&key, &ended, &expires)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, err
	case inf.Key != "" && key == inf.Key:
		return false, nil // this grant, kept before
	case ended == 0 && inf.AppliedAt.Unix() < expires:
		return false, fmt.Errorf("character %d already carries an infection", inf.ServerID)
	}
	if _, err := tx.Exec(`INSERT INTO charinfection (serverid, line, applied_at, expires_at, source, grant_id, ended_at, ended_how)
		VALUES (?, ?, ?, ?, ?, ?, 0, '')
		ON CONFLICT(serverid) DO UPDATE SET line = excluded.line, applied_at = excluded.applied_at, expires_at = excluded.expires_at,
			source = excluded.source, grant_id = excluded.grant_id, ended_at = 0, ended_how = ''`,
		inf.ServerID, inf.Line, inf.AppliedAt.Unix(), inf.ExpiresAt.Unix(), inf.Source, inf.Key); err != nil {
		return false, err
	}
	if marker != 0 {
		if err := markerUpsert(tx, inf.ServerID, marker, inf.AppliedAt); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// EndInfection ends the character's open infection (how: InfectionExpired and
// the rest, at its time) and takes its markers off in one transaction; false
// with none open. An expiry ends only a row whose end is at or before at, and
// with none such takes nothing.
func (s *QuestStore) EndInfection(serverID uint32, how string, at time.Time, markers []int) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	q := `UPDATE charinfection SET ended_at = ?, ended_how = ? WHERE serverid = ? AND ended_at = 0`
	args := []any{at.Unix(), how, serverID}
	if how == InfectionExpired {
		q += ` AND expires_at <= ?` // its end come: never a newer row's
		args = append(args, at.Unix())
	}
	res, err := tx.Exec(q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 && how == InfectionExpired {
		return false, nil
	}
	for _, m := range markers {
		if _, err := tx.Exec(`DELETE FROM charquests WHERE serverid = ? AND questid = ?`, serverID, m); err != nil {
			return false, err
		}
	}
	return n > 0, tx.Commit()
}

// SetFeverMarker finishes the character's fever marker, the active infection's
// derived form, when it is missing.
func (s *QuestStore) SetFeverMarker(serverID uint32, marker int, at time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := markerUpsert(tx, serverID, marker, at); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFeverMarkers takes the character's fever markers off (no infection
// active stands behind them).
func (s *QuestStore) DeleteFeverMarkers(serverID uint32, markers []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range markers {
		if _, err := tx.Exec(`DELETE FROM charquests WHERE serverid = ? AND questid = ?`, serverID, m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DueInfections is the open infections whose end is at or before now.
func (s *QuestStore) DueInfections(now time.Time) ([]Infection, error) {
	rs, err := s.db.Query(`SELECT serverid, line, applied_at, expires_at FROM charinfection WHERE ended_at = 0 AND expires_at <= ?`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Infection
	for rs.Next() {
		var i Infection
		var applied, expires int64
		if err := rs.Scan(&i.ServerID, &i.Line, &applied, &expires); err != nil {
			return nil, err
		}
		i.AppliedAt, i.ExpiresAt = time.Unix(applied, 0), time.Unix(expires, 0)
		out = append(out, i)
	}
	return out, rs.Err()
}

// InfectionMigrated reports a migration row for the character's marker.
func (s *QuestStore) InfectionMigrated(serverID uint32, marker int) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM charinfection_migration WHERE serverid = ? AND questid = ?`, serverID, marker).Scan(&n)
	return n > 0, err
}

// MigrateFeverMarker is the server's migration of one marker it found with no
// infection row and no migration row (the tool's rule, tools/
// were_infection_migrate_20261115.py): an infection from the marker's own
// timestamp when it is a time at or before now, else seven days from now; one
// whose seven days are over already ends expired, its marker taken; a character
// turned (turned) has its marker taken and no infection. One transaction, the
// migration row its record. It reports the outcome.
func (s *QuestStore) MigrateFeverMarker(cq CharQuest, line string, turned bool, now time.Time, dur time.Duration) (string, *Infection, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM charinfection_migration WHERE serverid = ? AND questid = ?`, cq.ServerID, cq.QuestID).Scan(&n); err != nil {
		return "", nil, err
	}
	if n > 0 {
		return "migrated before", nil, nil
	}
	applied, outcome := now, "migration default"
	if t, err := time.Parse(time.RFC3339, cq.Updated); err == nil && !t.After(now) {
		applied, outcome = t, "from its timestamp"
	}
	inf := &Infection{ServerID: cq.ServerID, Line: line, AppliedAt: applied, ExpiresAt: applied.Add(dur), Source: "migration",
		Key: fmt.Sprintf("migration/%d/%d", cq.ServerID, cq.QuestID)}
	var ended int64
	how := ""
	switch {
	case turned:
		outcome, ended, how = "turned: marker taken", now.Unix(), InfectionTurned
	case !now.Before(inf.ExpiresAt):
		outcome, ended, how = "expired: marker taken", inf.ExpiresAt.Unix(), InfectionExpired
	}
	if _, err := tx.Exec(`INSERT INTO charinfection (serverid, line, applied_at, expires_at, source, grant_id, ended_at, ended_how)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid) DO NOTHING`, inf.ServerID, line, inf.AppliedAt.Unix(), inf.ExpiresAt.Unix(), inf.Source, inf.Key, ended, how); err != nil {
		return "", nil, err
	}
	if ended != 0 {
		if _, err := tx.Exec(`DELETE FROM charquests WHERE serverid = ? AND questid = ?`, cq.ServerID, cq.QuestID); err != nil {
			return "", nil, err
		}
		inf.EndedAt, inf.EndedHow = time.Unix(ended, 0), how
	}
	if _, err := tx.Exec(`INSERT INTO charinfection_migration (serverid, questid, line, marker_done, marker_step, marker_count, marker_slot,
		marker_updated, marker_picks, applied_at, expires_at, outcome, migrated_at, by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'server')`,
		cq.ServerID, cq.QuestID, line, cq.Done, cq.Step, cq.Count, cq.Slot, cq.Updated, cq.Picks, inf.AppliedAt.Unix(), inf.ExpiresAt.Unix(),
		outcome, now.UTC().Format(time.RFC3339)); err != nil {
		return "", nil, err
	}
	return outcome, inf, tx.Commit()
}
