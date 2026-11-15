package database

// formulamode.go — the damage formula mode of a shard, kept in its database
// (internal/formulamode; the owner, 2026-11-14: "give GMs /formula ... so we
// can switch between the two"). One row of table formula_setting: the mode
// ("off", "live" or "shadow"), who set it and when; the config's
// combat.formula_mode only seeds it. The shard's own server writes beside it
// what it has loaded (the formulas' count, whether a shadow log is set), so
// that another shard's GM can see it and be refused a mode the shard cannot
// run. Another shard's server writes the row of THIS database too, with a line
// of formula_mode_inbox, which this shard's server reads within seconds
// (TakeFormulaModeInbox) to apply the mode and tell its GMs; the npc mirror's
// way (npcmirror.go). Every change has its gm_audit line in the database
// whose mode it changed.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

const formulaModeSchema = `
CREATE TABLE IF NOT EXISTS formula_setting (
	id         INTEGER PRIMARY KEY CHECK (id = 1),
	mode       TEXT NOT NULL DEFAULT 'off',
	by_name    TEXT NOT NULL DEFAULT '',
	at         TEXT NOT NULL DEFAULT '',
	loaded     INTEGER NOT NULL DEFAULT 0,
	shadow_log INTEGER NOT NULL DEFAULT 0,
	seen_at    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS formula_mode_inbox (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	mode    TEXT NOT NULL,
	by_name TEXT NOT NULL DEFAULT '',
	shard   TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL
);`

// FormulaSetting is a shard's formula_setting row.
type FormulaSetting struct {
	Mode      string // "off", "live" or "shadow"
	By        string // who set it ("config" for the seed)
	At        string
	Loaded    int  // the formulas its server has loaded
	ShadowLog bool // whether its server has a shadow log
	SeenAt    string
	Exists    bool // false: no row yet (nothing seeded, no server of this build ran on it)
}

func (db *SQLiteDB) formulaSchema() error {
	_, err := db.Exec(formulaModeSchema)
	return err
}

// FormulaSetting reads the row (Exists false when there is none).
func (db *SQLiteDB) FormulaSetting() (FormulaSetting, error) {
	var s FormulaSetting
	if !tableExists(db, "formula_setting") {
		return s, nil
	}
	var shadow int
	err := db.QueryRow(`SELECT mode, by_name, at, loaded, shadow_log, seen_at FROM formula_setting WHERE id = 1`).
		Scan(&s.Mode, &s.By, &s.At, &s.Loaded, &shadow, &s.SeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.ShadowLog, s.Exists = shadow != 0, true
	return s, nil
}

// SeedFormulaMode makes the row with the config's mode when there is none;
// an existing row (a GM's choice) is kept. It returns the row as it is.
func (db *SQLiteDB) SeedFormulaMode(mode string) (FormulaSetting, error) {
	if err := db.formulaSchema(); err != nil {
		return FormulaSetting{}, err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO formula_setting (id, mode, by_name, at) VALUES (1, ?, 'config', ?)`,
		mode, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return FormulaSetting{}, err
	}
	return db.FormulaSetting()
}

// SetFormulaLoaded records what this shard's server has loaded.
func (db *SQLiteDB) SetFormulaLoaded(loaded int, shadowLog bool) error {
	if err := db.formulaSchema(); err != nil {
		return err
	}
	sl := 0
	if shadowLog {
		sl = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO formula_setting (id, mode, by_name, at, loaded, shadow_log, seen_at) VALUES (1, 'off', 'config', ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET loaded = excluded.loaded, shadow_log = excluded.shadow_log, seen_at = excluded.seen_at`,
		now, loaded, sl, now)
	return err
}

// SetFormulaModeAudited sets the mode, with its gm_audit line (detail) in one
// transaction; with fromShard set (another shard's GM wrote it) a line of the
// inbox too, for this database's own server.
func (db *SQLiteDB) SetFormulaModeAudited(mode string, who GMEdit, detail, fromShard string) error {
	if err := db.formulaSchema(); err != nil {
		return err
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if who.Username == "" {
		if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username); err != nil {
			return fmt.Errorf("account %d: %w", who.AccountID, err)
		}
	}
	before, _ := fullRow(tx, `SELECT * FROM formula_setting WHERE id = 1`)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO formula_setting (id, mode, by_name, at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET mode = excluded.mode, by_name = excluded.by_name, at = excluded.at`, mode, who.Username, now); err != nil {
		return err
	}
	after, err := fullRow(tx, `SELECT * FROM formula_setting WHERE id = 1`)
	if err != nil {
		return err
	}
	action := "update"
	if before == nil {
		action = "insert"
	}
	if _, err := auditRowChange(tx, who, action, "formula_setting", map[string]any{"id": 1}, before, after, detail); err != nil {
		return err
	}
	if fromShard != "" {
		if _, err := tx.Exec(`INSERT INTO formula_mode_inbox (mode, by_name, shard, at) VALUES (?, ?, ?, ?)`,
			mode, who.Username, fromShard, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FormulaModeNote is a line of formula_mode_inbox: a mode another shard's GM
// set here.
type FormulaModeNote struct {
	Mode, By, Shard string
}

// TakeFormulaModeInbox reads and deletes the inbox's lines, oldest first.
func (db *SQLiteDB) TakeFormulaModeInbox() ([]FormulaModeNote, error) {
	if !tableExists(db, "formula_mode_inbox") {
		return nil, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, mode, by_name, shard FROM formula_mode_inbox ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []FormulaModeNote
	var last int64
	for rows.Next() {
		var n FormulaModeNote
		if err := rows.Scan(&last, &n.Mode, &n.By, &n.Shard); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM formula_mode_inbox WHERE id <= ?`, last); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// OpenFormulaPeer opens another shard's database to read or set its mode: an
// existing file only (a wrong path must not make an empty database), with a
// short busy timeout and none of this server's schema run on it but the two
// formula tables. It must have an accountinfo table (a shard's). The caller
// closes it.
func OpenFormulaPeer(path string) (*SQLiteDB, error) {
	if path == "" {
		return nil, errors.New("no database named")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=rw&_pragma=busy_timeout(%d)", path, npcMirrorBusyMS))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	peer := &SQLiteDB{DB: db, path: path}
	if !tableExists(peer, "accountinfo") {
		db.Close()
		return nil, fmt.Errorf("%s is no shard's database (no accountinfo)", path)
	}
	return peer, nil
}
