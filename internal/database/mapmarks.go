package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

// Community map marks (the user, 2026-10-26: "Players can submit waypoints to be
// added to the in-game map. Submissions would require official approval, similar
// to our spellfx"; ONE list on this server for every vanilla player on any game
// server, internal/mapmarks's two open routes). mapmark_suggestions holds what
// players send from the in-game map with no sign-in, mapmarks what a GM accepted
// (or placed): the list every map shows. A mark's place is the map's own: world
// is the game's world index (gWorldInfo's, 0 Tunaria .. 5 Secrets; Sandstorm's S2
// adds 6 and 7), x and z the world's units. name and note are what the player
// typed, kept as given and never acted on; sender_hash is the sender's address
// salted and hashed (mapmark_meta's salt, its own), never the address. A
// suggestion's status is waiting, accepted (mark_id names its mark) or dismissed.
const mapMarksSchema = `
	CREATE TABLE IF NOT EXISTS mapmarks (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		world         INTEGER NOT NULL,
		x             REAL NOT NULL,
		z             REAL NOT NULL,
		name          TEXT NOT NULL,
		note          TEXT NOT NULL DEFAULT '',
		kind          TEXT NOT NULL DEFAULT 'landmark',
		source        TEXT NOT NULL DEFAULT 'gm',
		suggestion_id INTEGER NOT NULL DEFAULT 0,
		created       TEXT NOT NULL DEFAULT '',
		updated       TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_mapmarks_world ON mapmarks(world);
	CREATE TABLE IF NOT EXISTS mapmark_suggestions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		world       INTEGER NOT NULL,
		x           REAL NOT NULL,
		z           REAL NOT NULL,
		name        TEXT NOT NULL,
		note        TEXT NOT NULL DEFAULT '',
		client      TEXT NOT NULL DEFAULT '',
		sender_hash TEXT NOT NULL,
		created     TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'waiting',
		decided_by  TEXT NOT NULL DEFAULT '',
		decided_at  TEXT NOT NULL DEFAULT '',
		mark_id     INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_mapmark_suggestions_status ON mapmark_suggestions(status, world);
	CREATE INDEX IF NOT EXISTS idx_mapmark_suggestions_sender ON mapmark_suggestions(sender_hash, status);
	CREATE TABLE IF NOT EXISTS mapmark_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`

// Map mark suggestion statuses.
const (
	MapMarkWaiting   = "waiting"
	MapMarkAccepted  = "accepted"
	MapMarkDismissed = "dismissed"
)

// MapMarkKinds are what a GM may call a mark (players never send one): the
// first is the default.
var MapMarkKinds = []string{"landmark", "camp", "quest", "merchant", "travel", "danger", "other"}

// The limits a mark's fields keep, the public route's and the GM's alike.
const (
	MapMarkMaxWorld = 29
	MapMarkMaxCoord = 200000
	MapMarkMinName  = 2
	MapMarkMaxName  = 40
	MapMarkMaxNote  = 200
)

// MapMarkKind says whether k is one of MapMarkKinds.
func MapMarkKind(k string) bool {
	for _, v := range MapMarkKinds {
		if v == k {
			return true
		}
	}
	return false
}

// CheckMapMarkPlace is the place's rule: a world of the table and finite
// coordinates within MapMarkMaxCoord.
func CheckMapMarkPlace(world int, x, z float64) error {
	if world < 0 || world > MapMarkMaxWorld {
		return fmt.Errorf("the world is 0 to %d", MapMarkMaxWorld)
	}
	for _, v := range []float64{x, z} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > MapMarkMaxCoord {
			return fmt.Errorf("x and z are within %d of the middle", MapMarkMaxCoord)
		}
	}
	return nil
}

// ensureMapMarks makes the community map marks' tables (initSchema).
func (db *SQLiteDB) ensureMapMarks() error {
	if _, err := db.Exec(mapMarksSchema); err != nil {
		return fmt.Errorf("mapmarks: %w", err)
	}
	return nil
}

// MapMarkSalt is the salt the senders' addresses are hashed with: made once, at
// random, and kept in mapmark_meta (hidden from every page), so a hash means
// nothing off this database.
func (db *SQLiteDB) MapMarkSalt() (string, error) {
	var salt string
	err := db.QueryRow(`SELECT value FROM mapmark_meta WHERE key = 'salt'`).Scan(&salt)
	if err == nil && salt != "" {
		return salt, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	salt = hex.EncodeToString(b)
	// another start may have made one meanwhile: the first kept wins
	if _, err := db.Exec(`INSERT OR IGNORE INTO mapmark_meta (key, value) VALUES ('salt', ?)`, salt); err != nil {
		return "", err
	}
	if err := db.QueryRow(`SELECT value FROM mapmark_meta WHERE key = 'salt'`).Scan(&salt); err != nil {
		return "", err
	}
	return salt, nil
}

// MapMark is one mark of the list.
type MapMark struct {
	ID           int64
	World        int
	X, Z         float64
	Name, Note   string
	Kind, Source string
	SuggestionID int64
	Created      string
	Updated      string
}

// MapMarks is every mark, of one world when world is 0 or more, by world then id.
func (db *SQLiteDB) MapMarks(world int) ([]MapMark, error) {
	q := `SELECT id, world, x, z, name, note, kind, source, suggestion_id, created, updated FROM mapmarks`
	var args []any
	if world >= 0 {
		q += ` WHERE world = ?`
		args = append(args, world)
	}
	q += ` ORDER BY world, id`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MapMark
	for rows.Next() {
		var m MapMark
		if err := rows.Scan(&m.ID, &m.World, &m.X, &m.Z, &m.Name, &m.Note, &m.Kind, &m.Source, &m.SuggestionID, &m.Created, &m.Updated); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MapMarkSuggestion is one suggestion as it is kept.
type MapMarkSuggestion struct {
	ID         int64
	World      int
	X, Z       float64
	Name, Note string
	Client     string
	SenderHash string
	Created    string
	Status     string
	DecidedBy  string
	DecidedAt  string
	MarkID     int64
}

// The refusals AddMapMarkSuggestion answers with.
var (
	ErrMapMarkSenderFull = errors.New("the sender has as many suggestions waiting as it may")
	ErrMapMarkBoxFull    = errors.New("the suggestion box is full")
)

// AddMapMarkSuggestion keeps a suggestion: in place of the same sender's waiting
// one of the same world within near units (replaced: the same id, sent anew),
// else as a new one unless the sender has perSender waiting already
// (ErrMapMarkSenderFull) or boxMax wait in all (ErrMapMarkBoxFull). It answers
// the suggestion's id and whether it replaced one.
func (db *SQLiteDB) AddMapMarkSuggestion(s MapMarkSuggestion, now time.Time, near float64, perSender, boxMax int) (int64, bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, x, z FROM mapmark_suggestions WHERE sender_hash = ? AND status = ? AND world = ?`,
		s.SenderHash, MapMarkWaiting, s.World)
	if err != nil {
		return 0, false, err
	}
	var nearest int64
	best := math.Inf(1)
	for rows.Next() {
		var id int64
		var x, z float64
		if err := rows.Scan(&id, &x, &z); err != nil {
			rows.Close()
			return 0, false, err
		}
		if d := math.Hypot(x-s.X, z-s.Z); d <= near && d < best {
			nearest, best = id, d
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	at := stampNow(now)
	if nearest != 0 {
		if _, err := tx.Exec(`UPDATE mapmark_suggestions SET x = ?, z = ?, name = ?, note = ?, client = ?, created = ? WHERE id = ?`,
			s.X, s.Z, s.Name, s.Note, s.Client, at, nearest); err != nil {
			return 0, false, err
		}
		return nearest, true, tx.Commit()
	}
	var mine, all int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM mapmark_suggestions WHERE sender_hash = ? AND status = ?`, s.SenderHash, MapMarkWaiting).Scan(&mine); err != nil {
		return 0, false, err
	}
	if mine >= perSender {
		return 0, false, ErrMapMarkSenderFull
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM mapmark_suggestions WHERE status = ?`, MapMarkWaiting).Scan(&all); err != nil {
		return 0, false, err
	}
	if all >= boxMax {
		return 0, false, ErrMapMarkBoxFull
	}
	res, err := tx.Exec(`INSERT INTO mapmark_suggestions (world, x, z, name, note, client, sender_hash, created, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, s.World, s.X, s.Z, s.Name, s.Note, s.Client, s.SenderHash, at, MapMarkWaiting)
	if err != nil {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return id, false, tx.Commit()
}

// MapMarkSuggestionCount is how many suggestions are waiting: the box's size.
func (db *SQLiteDB) MapMarkSuggestionCount() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM mapmark_suggestions WHERE status = ?`, MapMarkWaiting).Scan(&n)
	return n, err
}

// MapMarkSuggestions is every suggestion of the given status ("" or "all": every
// one), of world when it is 0 or more, newest first.
func (db *SQLiteDB) MapMarkSuggestions(status string, world int) ([]MapMarkSuggestion, error) {
	q := `SELECT id, world, x, z, name, note, client, sender_hash, created, status, decided_by, decided_at, mark_id FROM mapmark_suggestions`
	var where []string
	var args []any
	if status != "" && status != "all" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if world >= 0 {
		where = append(where, "world = ?")
		args = append(args, world)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created DESC, id DESC"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MapMarkSuggestion
	for rows.Next() {
		var s MapMarkSuggestion
		if err := rows.Scan(&s.ID, &s.World, &s.X, &s.Z, &s.Name, &s.Note, &s.Client, &s.SenderHash, &s.Created, &s.Status, &s.DecidedBy, &s.DecidedAt, &s.MarkID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// MapMarkSuggestionByID is one suggestion, or sql.ErrNoRows.
func (db *SQLiteDB) MapMarkSuggestionByID(id int64) (MapMarkSuggestion, error) {
	var s MapMarkSuggestion
	err := db.QueryRow(`SELECT id, world, x, z, name, note, client, sender_hash, created, status, decided_by, decided_at, mark_id
		FROM mapmark_suggestions WHERE id = ?`, id).Scan(&s.ID, &s.World, &s.X, &s.Z, &s.Name, &s.Note, &s.Client, &s.SenderHash,
		&s.Created, &s.Status, &s.DecidedBy, &s.DecidedAt, &s.MarkID)
	return s, err
}

// DecideMapMarkSuggestion settles a waiting suggestion: status (accepted, with
// the mark it became, or dismissed), who and when. It answers whether it was
// waiting.
func (db *SQLiteDB) DecideMapMarkSuggestion(id int64, status, by string, markID int64, now time.Time) (bool, error) {
	res, err := db.Exec(`UPDATE mapmark_suggestions SET status = ?, decided_by = ?, decided_at = ?, mark_id = ? WHERE id = ? AND status = ?`,
		status, by, stampNow(now), markID, id, MapMarkWaiting)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// DeleteMapMarkSuggestionsFrom deletes the waiting suggestions of the senders
// whose hash begins with prefix (at least 8 characters: a spammer's, as the panel
// shows the hash), and answers how many.
func (db *SQLiteDB) DeleteMapMarkSuggestionsFrom(prefix string) (int, error) {
	if len(prefix) < 8 || strings.ContainsAny(prefix, "%_") {
		return 0, fmt.Errorf("a sender hash prefix of at least 8 characters")
	}
	res, err := db.Exec(`DELETE FROM mapmark_suggestions WHERE sender_hash LIKE ? AND status = ?`, prefix+"%", MapMarkWaiting)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// mapMarksGen counts InvalidateMapMarks: the public list (internal/mapmarks) is
// built again before its next answer once it has moved.
var mapMarksGen atomic.Int64

// InvalidateMapMarks says the marks changed (an accept, a GM's row edit or
// delete, a revert): the public list is built again at its next request.
func InvalidateMapMarks() { mapMarksGen.Add(1) }

// MapMarksGeneration is how many times the marks changed since the start.
func MapMarksGeneration() int64 { return mapMarksGen.Load() }
