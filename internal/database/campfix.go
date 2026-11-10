package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Camp fix reports (handlers/campfix.go; board entry 513, the GM Jibrish's ask
// of 2026-11-09: "a command players can use to report a camp's creatures
// standing in the wrong spot or facing the wrong way, or one too many or too
// few"): a player's marks, sent with /campfix, or a petition's spawn request
// made a report (handlers/campfixpetition.go), kept as a row of
// campfix_reports with a work board entry. Nothing here changes a spawn: a
// report waits until a GM approves some or all of its marks or declines it on
// the GM panel's Camp reports page (gmweb/ops_campfix.go; CLAUDE.md, "Camp fix
// reports: how they are reviewed"), which applies them through its audited
// row edits.
//
// A row: the account and the character that sent it (its name, level and
// class then), the world and the first mark's x and z, the note (the player's,
// or the filer's naming the petition; masked, at most 200 characters), the
// marks, the creatures near them and the flags the server raised (each JSON,
// handlers/campfix.go's), its status (waiting until a GM approves it, applied,
// or declines it, declined; partial is an older review's; the reviewed time
// follows a change of it), the review's lines, the board entry, when it was
// sent.

const campfixSchema = `
	CREATE TABLE IF NOT EXISTS campfix_reports (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid INTEGER NOT NULL DEFAULT 0,
		charname  TEXT NOT NULL DEFAULT '',
		level     INTEGER NOT NULL DEFAULT 0,
		class     INTEGER NOT NULL DEFAULT 0,
		world     INTEGER NOT NULL DEFAULT 0,
		x         REAL NOT NULL DEFAULT 0,
		z         REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		marks     TEXT NOT NULL DEFAULT '[]',
		near      TEXT NOT NULL DEFAULT '[]',
		flags     TEXT NOT NULL DEFAULT '[]',
		status    TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'applied', 'partial', 'declined')),
		review    TEXT NOT NULL DEFAULT '',
		board     INTEGER NOT NULL DEFAULT 0,
		created   TEXT NOT NULL DEFAULT '',
		reviewed  TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_campfix_reports_account ON campfix_reports(accountid, id);
	CREATE INDEX IF NOT EXISTS idx_campfix_reports_status ON campfix_reports(status);
	CREATE TRIGGER IF NOT EXISTS campfix_reports_reviewed AFTER UPDATE OF status ON campfix_reports
		WHEN NEW.status IS NOT OLD.status
		BEGIN UPDATE campfix_reports SET reviewed = CASE WHEN NEW.status = 'waiting' THEN ''
			ELSE strftime('%Y-%m-%dT%H:%M:%SZ', 'now') END WHERE id = NEW.id; END;`

// A report's statuses: waiting for a GM, or what the review made of it:
// applied (some or all of its marks: the review says which), declined (none);
// partial is an older review's word for some.
const (
	CampfixWaiting  = "waiting"
	CampfixApplied  = "applied"
	CampfixPartial  = "partial"
	CampfixDeclined = "declined"
)

// CampfixStatuses are a report's statuses, as a list filters them.
var CampfixStatuses = []string{CampfixWaiting, CampfixApplied, CampfixPartial, CampfixDeclined}

// ensureCampfix makes table campfix_reports (initSchema, OpenCampfix).
func (db *SQLiteDB) ensureCampfix() error {
	if _, err := db.Exec(campfixSchema); err != nil {
		return fmt.Errorf("campfix_reports: %w", err)
	}
	return nil
}

// OpenCampfix opens an existing database to file camp reports beside a
// running server (cmd/campfixfile): as OpenWorkBoard, and campfix_reports
// made when it is missing; nothing else of the game's schema is touched.
func OpenCampfix(path string) (*SQLiteDB, error) {
	db, err := OpenWorkBoard(path)
	if err != nil {
		return nil, err
	}
	if err := db.ensureCampfix(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// CampfixReport is one row of campfix_reports. Marks, Near and Flags are JSON.
type CampfixReport struct {
	ID        int64
	AccountID int
	CharName  string
	Level     int
	Class     int
	World     int
	X, Z      float64
	Note      string
	Marks     string
	Near      string
	Flags     string
	Status    string
	Review    string
	Board     int64
	Created   string
	Reviewed  string
}

const campfixCols = `id, accountid, charname, level, class, world, x, z, note, marks, near, flags, status, review, board, created, reviewed`

func scanCampfix(sc interface{ Scan(...any) error }) (CampfixReport, error) {
	var r CampfixReport
	err := sc.Scan(&r.ID, &r.AccountID, &r.CharName, &r.Level, &r.Class, &r.World, &r.X, &r.Z, &r.Note, &r.Marks,
		&r.Near, &r.Flags, &r.Status, &r.Review, &r.Board, &r.Created, &r.Reviewed)
	return r, err
}

// CampfixReportByID is one report, false when there is none.
func (db *SQLiteDB) CampfixReportByID(id int64) (CampfixReport, bool, error) {
	r, err := scanCampfix(db.QueryRow(`SELECT `+campfixCols+` FROM campfix_reports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CampfixReport{}, false, nil
	}
	if err != nil {
		return CampfixReport{}, false, fmt.Errorf("campfix_reports: %w", err)
	}
	return r, true, nil
}

// CampfixLimits is what the sending's limits read: when the account last sent
// a report (zero for never), how many of its reports wait for a GM, and how
// many wait in all.
type CampfixLimits struct {
	LastSent      time.Time
	Waiting       int
	WaitingAll    int
	LastSentKnown bool
}

// CampfixLimitsOf reads CampfixLimits for an account.
func (db *SQLiteDB) CampfixLimitsOf(accountID int) (CampfixLimits, error) {
	var l CampfixLimits
	var last string
	err := db.QueryRow(`SELECT created FROM campfix_reports WHERE accountid = ? ORDER BY id DESC LIMIT 1`, accountID).Scan(&last)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return l, fmt.Errorf("campfix_reports: %w", err)
	default:
		if t, err := time.Parse(time.RFC3339, last); err == nil {
			l.LastSent, l.LastSentKnown = t, true
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(accountid = ?), 0) FROM campfix_reports WHERE status = ?`,
		accountID, CampfixWaiting).Scan(&l.WaitingAll, &l.Waiting); err != nil {
		return l, fmt.Errorf("campfix_reports: %w", err)
	}
	return l, nil
}

// FileCampfixReport keeps r as a new waiting report sent at now and posts its
// work board entry, made by post from the report's id, as who (PostWork: the
// entry and its gm_audit line); the report names the entry. A post refused
// takes the report back: nothing is kept. It returns the report and the entry
// as kept.
func (db *SQLiteDB) FileCampfixReport(r CampfixReport, now time.Time, post func(id int64) WorkPost, who GMEdit) (CampfixReport, WorkEntry, error) {
	r.Status, r.Review, r.Board, r.Reviewed = CampfixWaiting, "", 0, ""
	r.Created = now.UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO campfix_reports (accountid, charname, level, class, world, x, z, note, marks, near, flags,
		status, review, board, created, reviewed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, ?, '')`,
		r.AccountID, r.CharName, r.Level, r.Class, r.World, r.X, r.Z, r.Note, r.Marks, r.Near, r.Flags, r.Status, r.Created)
	if err != nil {
		return CampfixReport{}, WorkEntry{}, fmt.Errorf("campfix_reports: %w", err)
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return CampfixReport{}, WorkEntry{}, fmt.Errorf("campfix_reports: %w", err)
	}
	entry, _, err := db.PostWork(post(r.ID), who)
	if err != nil {
		if _, derr := db.Exec(`DELETE FROM campfix_reports WHERE id = ?`, r.ID); derr != nil {
			return CampfixReport{}, WorkEntry{}, fmt.Errorf("the board refused it (%v), and the report was not taken back: %w", err, derr)
		}
		return CampfixReport{}, WorkEntry{}, fmt.Errorf("the work board refused it: %w", err)
	}
	if _, err := db.Exec(`UPDATE campfix_reports SET board = ? WHERE id = ?`, entry.ID, r.ID); err != nil {
		return CampfixReport{}, WorkEntry{}, fmt.Errorf("campfix_reports: entry %d not named: %w", entry.ID, err)
	}
	r.Board = entry.ID
	return r, entry, nil
}

// CampfixQuery is a page of reports: of a status (CampfixStatuses, or "all";
// empty: waiting), at most Limit (100 when 0, at most 100), Offset skipped.
// The waiting ones come oldest first (the order a GM decides them in), the
// rest newest first.
type CampfixQuery struct {
	Status string
	Limit  int
	Offset int
}

// CampfixList is a page of reports, how many the query matched, and how many
// reports there are of each status.
type CampfixList struct {
	Reports  []CampfixReport
	Matching int
	Counts   map[string]int
}

// CampfixReports is the page q asks for.
func (db *SQLiteDB) CampfixReports(q CampfixQuery) (CampfixList, error) {
	out := CampfixList{Reports: []CampfixReport{}, Counts: map[string]int{}}
	status := q.Status
	if status == "" {
		status = CampfixWaiting
	}
	where, order := "", "id DESC"
	var args []any
	switch {
	case status == "all":
	case containsString(CampfixStatuses, status):
		where, args = " WHERE status = ?", []any{status}
		if status == CampfixWaiting {
			order = "id"
		}
	default:
		return out, fmt.Errorf("status %q is not one of %s or all", q.Status, strings.Join(CampfixStatuses, ", "))
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM campfix_reports`+where, args...).Scan(&out.Matching); err != nil {
		return out, fmt.Errorf("campfix_reports: %w", err)
	}
	rows, err := db.Query(`SELECT `+campfixCols+` FROM campfix_reports`+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`,
		append(args, limit, max(0, q.Offset))...)
	if err != nil {
		return out, fmt.Errorf("campfix_reports: %w", err)
	}
	for rows.Next() {
		r, err := scanCampfix(rows)
		if err != nil {
			rows.Close()
			return out, fmt.Errorf("campfix_reports: %w", err)
		}
		out.Reports = append(out.Reports, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("campfix_reports: %w", err)
	}
	counts, err := db.Query(`SELECT status, COUNT(*) FROM campfix_reports GROUP BY status`)
	if err != nil {
		return out, fmt.Errorf("campfix_reports: %w", err)
	}
	defer counts.Close()
	for counts.Next() {
		var st string
		var n int
		if err := counts.Scan(&st, &n); err != nil {
			return out, fmt.Errorf("campfix_reports: %w", err)
		}
		out.Counts[st] = n
	}
	return out, counts.Err()
}

// DecideCampfixReport sets a waiting report's status (CampfixApplied or
// CampfixDeclined) and its review, in one transaction with its gm_audit line
// as who's row change (revertible: the panel's revert_change puts it back to
// waiting); the reviewed time follows (the table's trigger). A report decided
// already is refused, so two GMs cannot both decide one. It returns the report
// now and the line's id.
func (db *SQLiteDB) DecideCampfixReport(id int64, status, review string, who GMEdit) (CampfixReport, int64, error) {
	if status != CampfixApplied && status != CampfixDeclined {
		return CampfixReport{}, 0, fmt.Errorf("a report is decided %s or %s, not %q", CampfixApplied, CampfixDeclined, status)
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return CampfixReport{}, 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return CampfixReport{}, 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	cur, err := scanCampfix(tx.QueryRow(`SELECT `+campfixCols+` FROM campfix_reports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CampfixReport{}, 0, fmt.Errorf("no camp report %d", id)
	}
	if err != nil {
		return CampfixReport{}, 0, fmt.Errorf("campfix_reports: %w", err)
	}
	if cur.Status != CampfixWaiting {
		return cur, 0, fmt.Errorf("camp report %d is %s already (%s)", id, cur.Status, cur.Reviewed)
	}
	const sel = `SELECT * FROM campfix_reports WHERE id = ?`
	before, err := fullRow(tx, sel, id)
	if err != nil {
		return CampfixReport{}, 0, err
	}
	res, err := tx.Exec(`UPDATE campfix_reports SET status = ?, review = ? WHERE id = ? AND status = ?`, status, review, id, CampfixWaiting)
	if err != nil {
		return CampfixReport{}, 0, fmt.Errorf("campfix_reports: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return CampfixReport{}, 0, fmt.Errorf("camp report %d was decided meanwhile", id)
	}
	after, err := fullRow(tx, sel, id)
	if err != nil {
		return CampfixReport{}, 0, err
	}
	audit, err := auditRowChange(tx, who, "update", "campfix_reports", map[string]any{"id": id}, before, after,
		fmt.Sprintf("camp report %d %s", id, status))
	if err != nil {
		return CampfixReport{}, 0, err
	}
	out, err := scanCampfix(tx.QueryRow(`SELECT `+campfixCols+` FROM campfix_reports WHERE id = ?`, id))
	if err != nil {
		return CampfixReport{}, 0, fmt.Errorf("campfix_reports: %w", err)
	}
	return out, audit, tx.Commit()
}
