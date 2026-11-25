package database

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ideaboard.go — the marks behind the Idea Contributors board (handlers/ideaboard.go; petition 3016, Cucumber,
// approved by the owner 2026-11-25: "For the bulletins in FP that show bug report top scores and totals, create
// another for Feature Request top contributors and which features they suggested that ultimately were adopted. If 2
// people suggested the same feature, each gets it listed under them.").
//
// petitionadopted: a row a feature a petition asked for that the game took up, by its name as the board lists it
// ("Auto Loot"), when and by whom (a GM's name, or a tool's). One petition may name several features; two petitions
// may name the same one (each player gets it listed). A row is ours, set by tools/adopt_feature.py or the GM panel's
// table (Adopted features), never by a player; a petition's id is this shard's own (each shard keeps its own rows,
// as its own petitions).
const petitionAdoptedSchema = `
	CREATE TABLE IF NOT EXISTS petitionadopted (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		petitionid INTEGER NOT NULL,
		feature    TEXT NOT NULL,
		adopted_at TEXT NOT NULL DEFAULT '',
		adopted_by TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT ''
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_petitionadopted ON petitionadopted(petitionid, feature COLLATE NOCASE);`

// ensurePetitionAdopted makes table petitionadopted (initSchema).
func (db *SQLiteDB) ensurePetitionAdopted() error {
	if _, err := db.Exec(petitionAdoptedSchema); err != nil {
		return fmt.Errorf("petitionadopted: %w", err)
	}
	return nil
}

// AdoptFeature marks petition id as asking for feature, adopted at at by by; a mark already there (the same
// feature in any case) is left as it is and reported false.
func (db *SQLiteDB) AdoptFeature(id int64, feature, by string, at time.Time) (bool, error) {
	feature = strings.TrimSpace(feature)
	if feature == "" {
		return false, errors.New("petitionadopted: a feature needs a name")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM petitions WHERE id = ?`, id).Scan(&n); err != nil {
		return false, err
	}
	if n == 0 {
		return false, fmt.Errorf("petitionadopted: no petition %d", id)
	}
	res, err := db.Exec(`INSERT OR IGNORE INTO petitionadopted (petitionid, feature, adopted_at, adopted_by) VALUES (?, ?, ?, ?)`,
		id, feature, at.UTC().Format(time.RFC3339), strings.TrimSpace(by))
	if err != nil {
		return false, err
	}
	k, _ := res.RowsAffected()
	return k > 0, nil
}

// IdeaTally is one account's adopted features.
type IdeaTally struct {
	AccountID int
	Username  string // accountinfo.username, "" when the account is gone
	// Player is the character the board names: the one that filed most of the account's Bug Report petitions (the
	// latest on a tie), as the Bug Finder Leaderboard names it, else the one that filed an adopted petition.
	Player string
	// Features are the features its petitions asked for that were adopted, each once (in any case), the first
	// adopted first.
	Features []string
}

// IdeaTallies is every account's adopted features on q, an account with none left out. A database with no
// petitionadopted table yet (the other shard before its deploy) has none.
func IdeaTallies(q Queryer, category int64) ([]IdeaTally, error) {
	var n int
	if err := q.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'petitionadopted'`).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	rows, err := q.Query(`SELECT p.accountid, COALESCE(a.username, ''),
			COALESCE(NULLIF((SELECT p2.charname FROM petitions p2 WHERE p2.accountid = p.accountid AND p2.category = ?
				AND p2.charname != '' GROUP BY p2.charname ORDER BY COUNT(*) DESC, MAX(p2.filed) DESC LIMIT 1), ''),
				MAX(p.charname), ''),
			MIN(TRIM(f.feature)), MIN(f.adopted_at), MIN(f.id)
		FROM petitionadopted f JOIN petitions p ON p.id = f.petitionid
		LEFT JOIN accountinfo a ON a.accountid = p.accountid
		WHERE p.accountid > 0 AND TRIM(f.feature) != ''
		GROUP BY p.accountid, LOWER(TRIM(f.feature))
		ORDER BY p.accountid, MIN(f.adopted_at), MIN(f.id)`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IdeaTally
	for rows.Next() {
		var t IdeaTally
		var feature, at string
		var first int64
		if err := rows.Scan(&t.AccountID, &t.Username, &t.Player, &feature, &at, &first); err != nil {
			return nil, err
		}
		if k := len(out) - 1; k >= 0 && out[k].AccountID == t.AccountID {
			out[k].Features = append(out[k].Features, feature)
			continue
		}
		t.Features = []string{feature}
		out = append(out, t)
	}
	return out, rows.Err()
}
