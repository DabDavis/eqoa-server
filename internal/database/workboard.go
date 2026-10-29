package database

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// The work board: who is working on what on this server, so that two of its
// workers do not change the same thing at once. They are the GMs, on the GM
// panel's Work board page or through their AI agents over its MCP and OpenAPI
// (work_board, post_work, update_work_overview), and dab's sessions on the
// server's code and data, by ssh on the box (cmd/workboard). The board is an
// overview everyone edits (markdown: what it is for, conventions, what not to
// touch, who is around) and entries: a title, the area, the tables or files it
// touches, a status, an owner, details and notes in markdown, a result once
// done, and when and by whom it was made and last changed.
//
// Every change goes through PostWork or SetWorkOverview: in one transaction
// with its line in gm_audit, keyed as the panel keys a row ({"id": n}), so the
// panel's audit log lists it and revert_change undoes it as any row change.
// Each row's version goes up at every change, and a trigger raises it for a
// statement that did not (the panel's table editor, a revert), so a writer
// who sends the version it read is refused when anyone wrote since.
const workBoardSchema = `
	CREATE TABLE IF NOT EXISTS workboard (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT NOT NULL,
		area       TEXT NOT NULL DEFAULT 'other',
		touches    TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL DEFAULT 'planned',
		owner      TEXT NOT NULL DEFAULT '',
		details    TEXT NOT NULL DEFAULT '',
		notes      TEXT NOT NULL DEFAULT '',
		result     TEXT NOT NULL DEFAULT '',
		created    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		created_by TEXT NOT NULL DEFAULT '',
		updated    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		updated_by TEXT NOT NULL DEFAULT '',
		done_at    TEXT NOT NULL DEFAULT '',
		version    INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_workboard_status ON workboard(status);
	CREATE TABLE IF NOT EXISTS workboard_overview (
		id         INTEGER PRIMARY KEY CHECK (id = 1),
		text       TEXT NOT NULL DEFAULT '',
		version    INTEGER NOT NULL DEFAULT 1,
		updated    TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT ''
	);
	CREATE TRIGGER IF NOT EXISTS workboard_version AFTER UPDATE ON workboard
		WHEN NEW.version <= OLD.version
		BEGIN UPDATE workboard SET version = OLD.version + 1 WHERE id = NEW.id; END;
	CREATE TRIGGER IF NOT EXISTS workboard_overview_version AFTER UPDATE ON workboard_overview
		WHEN NEW.version <= OLD.version
		BEGIN UPDATE workboard_overview SET version = OLD.version + 1 WHERE id = NEW.id; END;`

// DefaultWorkOverview is the overview a new board starts with: what the board
// is for, the loop its workers keep, the statuses and the tools.
//
//go:embed workboard_overview.md
var DefaultWorkOverview string

// The statuses of an entry.
const (
	WorkInProgress = "in_progress" // being changed now: what it touches is taken
	WorkBlocked    = "blocked"     // started and waiting on something: still taken
	WorkPlanned    = "planned"     // to be done, nothing changed yet
	WorkDone       = "done"        // finished, its result saying what came of it
)

// WorkStatuses is every status, in the board's order.
var WorkStatuses = []string{WorkInProgress, WorkBlocked, WorkPlanned, WorkDone}

// WorkAreas is every area, in the order the page offers them: the server's
// code, its tables and content, quests, the client, the GM tools, other.
var WorkAreas = []string{"code", "data", "quests", "client", "gm_tools", "other"}

// What else a person or an agent may call a status or an area.
var (
	workStatusAliases = map[string]string{
		"inprogress": WorkInProgress, "wip": WorkInProgress, "active": WorkInProgress, "doing": WorkInProgress,
		"started": WorkInProgress, "working": WorkInProgress, "ongoing": WorkInProgress,
		"todo": WorkPlanned, "to_do": WorkPlanned, "plan": WorkPlanned, "planning": WorkPlanned, "open": WorkPlanned,
		"stuck": WorkBlocked, "waiting": WorkBlocked, "on_hold": WorkBlocked, "paused": WorkBlocked,
		"closed": WorkDone, "finished": WorkDone, "complete": WorkDone, "completed": WorkDone, "dropped": WorkDone,
	}
	workAreaAliases = map[string]string{
		"server": "code", "server_code": "code", "go": "code",
		"content": "data", "data/content": "data", "data_content": "data", "data_and_content": "data",
		"database": "data", "tables": "data", "quest": "quests",
		"gm": "gm_tools", "gmtools": "gm_tools", "gm_tool": "gm_tools", "panel": "gm_tools", "gm_panel": "gm_tools",
		"tools": "gm_tools",
	}
)

// The bounds of an entry and of the overview, in characters.
const (
	WorkMaxTitle    = 200
	WorkMaxOwner    = 64
	WorkMaxTouches  = 2000 // the list as text
	WorkMaxDetails  = 20000
	WorkMaxNote     = 4000 // one note
	WorkMaxNotes    = 60000
	WorkMaxResult   = 300
	WorkMaxOverview = 50000
)

// WorkDoneDays is how long a done entry stays on the board before it is
// folded away with the older ones.
const WorkDoneDays = 14

func workKey(s string) string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// NormalizeWorkStatus is a status as the board keeps it ("In progress" and
// "wip" are in_progress), false for no status.
func NormalizeWorkStatus(s string) (string, bool) {
	k := workKey(s)
	if containsString(WorkStatuses, k) {
		return k, true
	}
	v, ok := workStatusAliases[k]
	return v, ok
}

// NormalizeWorkArea is an area as the board keeps it ("GM tools" is
// gm_tools), false for no area.
func NormalizeWorkArea(s string) (string, bool) {
	k := workKey(s)
	if containsString(WorkAreas, k) {
		return k, true
	}
	v, ok := workAreaAliases[k]
	return v, ok
}

// ParseWorkTouches reads what an entry touches: items separated by commas,
// semicolons or line breaks, each trimmed and kept once (in any case).
func ParseWorkTouches(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' }) {
		f = strings.TrimSpace(f)
		k := touchKey(f)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// FormatWorkTouches writes a touches list as ParseWorkTouches reads it.
func FormatWorkTouches(items []string) string {
	return strings.Join(ParseWorkTouches(strings.Join(items, "\n")), ", ")
}

// touchKey is an item as two are compared: in lower case, without quotes or
// backticks round it, a leading "./" or a trailing "/".
func touchKey(s string) string {
	k := strings.ToLower(strings.Trim(strings.TrimSpace(s), "`'\""))
	k = strings.TrimPrefix(k, "./")
	return strings.TrimRight(k, "/")
}

// TouchesOverlap is whether two items name the same thing: the same table or
// file, or one inside the other (internal/gmweb holds internal/gmweb/ops.go;
// npcs holds npcs.modelid).
func TouchesOverlap(a, b string) bool {
	ka, kb := touchKey(a), touchKey(b)
	if ka == "" || kb == "" {
		return false
	}
	if ka == kb {
		return true
	}
	inside := func(long, short string) bool {
		return strings.HasPrefix(long, short+"/") || strings.HasPrefix(long, short+".")
	}
	return inside(ka, kb) || inside(kb, ka)
}

// WorkEntry is one entry of the board.
type WorkEntry struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Area      string   `json:"area"`
	Touches   []string `json:"touches"`
	Status    string   `json:"status"`
	Owner     string   `json:"owner"`
	Details   string   `json:"details"`
	Notes     string   `json:"notes"`
	Result    string   `json:"result"`
	Created   string   `json:"created"`
	CreatedBy string   `json:"created_by"`
	Updated   string   `json:"updated"`
	UpdatedBy string   `json:"updated_by"`
	DoneAt    string   `json:"done_at,omitempty"`
	Version   int64    `json:"version"`
}

// WorkOverview is the board's overview.
type WorkOverview struct {
	Text      string `json:"text"`
	Version   int64  `json:"version"`
	Updated   string `json:"updated"`
	UpdatedBy string `json:"updated_by"`
}

// WorkConflict refuses a change sent with a version older than the row's: it
// carries the row as it is now (a WorkEntry or the WorkOverview).
type WorkConflict struct {
	What    string // "entry 12", "the overview"
	Given   int64
	Version int64
	By, At  string
	Current any
}

func (e *WorkConflict) Error() string {
	last := ""
	if e.By != "" || e.At != "" {
		last = fmt.Sprintf(" (last changed by %s at %s)", e.By, e.At)
	}
	return fmt.Sprintf("%s changed since version %d was read: it is at version %d now%s. "+
		"Read it again, make your change to what it says now and send that with version %d",
		e.What, e.Given, e.Version, last, e.Version)
}

// ensureWorkBoard makes the board's tables and seeds its overview
// (initSchema, OpenWorkBoard). The audit log's table is made too: every
// change to the board is written there, whether or not the panel runs.
func (db *SQLiteDB) ensureWorkBoard() error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return fmt.Errorf("gm_audit: %w", err)
	}
	if _, err := db.Exec(workBoardSchema); err != nil {
		return fmt.Errorf("workboard: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO workboard_overview (id, text, version, updated, updated_by)
		VALUES (1, ?, 1, ?, 'the server')`, workText(DefaultWorkOverview), workStamp(time.Now())); err != nil {
		return fmt.Errorf("workboard_overview: %w", err)
	}
	return nil
}

// OpenWorkBoard opens an existing database for the board alone: the audit
// log's table and the board's are made when they are missing and the
// overview seeded, and nothing else of the game's schema is touched, so a
// tool on the box may write the board beside a server of another build.
func OpenWorkBoard(path string) (*SQLiteDB, error) {
	if st, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no database at %s (give -db, or run where config.yaml is): %w", path, err)
	} else if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a database", path)
	}
	db, err := sql.Open("sqlite", path+"?"+BusyTimeoutDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &SQLiteDB{DB: db, path: path}
	if err := s.ensureWorkBoard(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func workStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// workBy is who made a change, as the board shows it: the account's name, and
// the agent or token it came through ("jibrish (oauth:ChatGPT)").
func workBy(w GMEdit) string {
	name := strings.TrimSpace(w.Username)
	if name == "" {
		name = fmt.Sprintf("account %d", w.AccountID)
	}
	switch w.Via {
	case "", "web", "game", "cli":
		return name
	}
	return name + " (" + w.Via + ")"
}

// oneLine is text as one line: line breaks and tabs as spaces, trimmed.
func oneLine(s string) string {
	return strings.TrimSpace(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s))
}

// workText is markdown as the board keeps it: line breaks as \n, no blank
// lines before it, nothing trailing.
func workText(s string) string {
	lines := strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), " \t\n")
}

// workNote is a note as the notes keep it: a list item with the time and who
// wrote it, its further lines inside the item.
func workNote(at time.Time, by, text string) string {
	lines := strings.Split(workText(text), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "- %s, %s: %s", at.UTC().Format("2006-01-02 15:04 UTC"), by, strings.TrimSpace(lines[0]))
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("\n  " + l)
	}
	return b.String()
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// WorkEntryProblem is what keeps an entry from being kept as it is: a title
// missing or too long, a status or an area the board does not have, a done
// entry with no result, text past its bound. nil when there is none.
func WorkEntryProblem(e WorkEntry) error {
	switch {
	case e.Title == "":
		return errors.New("an entry needs a title (one line saying what is being done)")
	case runes(e.Title) > WorkMaxTitle:
		return fmt.Errorf("the title is %d characters: %d at most", runes(e.Title), WorkMaxTitle)
	case strings.ContainsAny(e.Title, "\r\n"):
		return errors.New("the title is one line")
	case !containsString(WorkStatuses, e.Status):
		return fmt.Errorf("status %q is not one of %s", e.Status, strings.Join(WorkStatuses, ", "))
	case !containsString(WorkAreas, e.Area):
		return fmt.Errorf("area %q is not one of %s", e.Area, strings.Join(WorkAreas, ", "))
	case runes(e.Owner) > WorkMaxOwner:
		return fmt.Errorf("the owner is %d characters: %d at most", runes(e.Owner), WorkMaxOwner)
	case runes(FormatWorkTouches(e.Touches)) > WorkMaxTouches:
		return fmt.Errorf("what it touches is %d characters: %d at most (name the tables and files, or their folder)",
			runes(FormatWorkTouches(e.Touches)), WorkMaxTouches)
	case runes(e.Details) > WorkMaxDetails:
		return fmt.Errorf("the details are %d characters: %d at most", runes(e.Details), WorkMaxDetails)
	case runes(e.Notes) > WorkMaxNotes:
		return fmt.Errorf("the notes are %d characters: %d at most (close it with a result and post a new entry)",
			runes(e.Notes), WorkMaxNotes)
	case runes(e.Result) > WorkMaxResult:
		return fmt.Errorf("the result is %d characters: %d at most, one line (the details and notes hold the rest)",
			runes(e.Result), WorkMaxResult)
	case strings.ContainsAny(e.Result, "\r\n"):
		return errors.New("the result is one line")
	case e.Status == WorkDone && strings.TrimSpace(e.Result) == "":
		return errors.New("a done entry says what came of it: give result, one line (\"18 guards moved\", \"dropped: not needed\")")
	}
	return nil
}

const workCols = `id, title, area, touches, status, owner, details, notes, result, created, created_by, updated, updated_by, done_at, version`

func scanWorkEntry(sc interface{ Scan(...any) error }) (WorkEntry, error) {
	var e WorkEntry
	var touches string
	err := sc.Scan(&e.ID, &e.Title, &e.Area, &touches, &e.Status, &e.Owner, &e.Details, &e.Notes, &e.Result,
		&e.Created, &e.CreatedBy, &e.Updated, &e.UpdatedBy, &e.DoneAt, &e.Version)
	e.Touches = ParseWorkTouches(touches)
	if e.Touches == nil {
		e.Touches = []string{}
	}
	return e, err
}

// WorkEntryByID is one entry, false when there is none.
func (db *SQLiteDB) WorkEntryByID(id int64) (WorkEntry, bool, error) {
	e, err := scanWorkEntry(db.QueryRow(`SELECT `+workCols+` FROM workboard WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkEntry{}, false, nil
	}
	if err != nil {
		return WorkEntry{}, false, fmt.Errorf("workboard: %w", err)
	}
	return e, true, nil
}

// WorkPost is a post to the board: a new entry (ID 0) or a change to one. A
// nil field is left as it is; a new entry takes its default (area other,
// status planned, the poster as its owner).
type WorkPost struct {
	ID      int64
	Title   *string
	Area    *string
	Touches *string // comma or line separated
	Status  *string
	Owner   *string
	Details *string
	// Note is added to the entry's notes: a line with the time and who wrote it.
	Note   string
	Result *string
	// Version is the entry's version as it was read; 0 is not checked.
	Version int64
}

// PostWork makes or changes an entry, in one transaction with its gm_audit
// line as who's row change (revertible in the panel). It returns the entry
// as it is now and the line's id (0 when nothing changed).
func (db *SQLiteDB) PostWork(p WorkPost, who GMEdit) (WorkEntry, int64, error) {
	by := workBy(who)
	now := time.Now()
	stamp := workStamp(now)
	tx, err := db.Begin()
	if err != nil {
		return WorkEntry{}, 0, err
	}
	defer tx.Rollback()

	isNew := p.ID == 0
	var cur WorkEntry
	if isNew {
		if p.Version != 0 {
			return WorkEntry{}, 0, errors.New("a version goes with a change to an entry: give its id too")
		}
		cur = WorkEntry{Area: "other", Status: WorkPlanned, Owner: oneLine(who.Username), Touches: []string{}}
	} else {
		cur, err = scanWorkEntry(tx.QueryRow(`SELECT `+workCols+` FROM workboard WHERE id = ?`, p.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return WorkEntry{}, 0, fmt.Errorf("no work entry %d (work_board lists them)", p.ID)
		}
		if err != nil {
			return WorkEntry{}, 0, err
		}
		if p.Version != 0 && p.Version != cur.Version {
			return WorkEntry{}, 0, &WorkConflict{What: fmt.Sprintf("entry %d", cur.ID), Given: p.Version,
				Version: cur.Version, By: cur.UpdatedBy, At: cur.Updated, Current: cur}
		}
	}

	next := cur
	if p.Title != nil {
		next.Title = oneLine(*p.Title)
	}
	if p.Area != nil {
		a, ok := NormalizeWorkArea(*p.Area)
		if !ok {
			return WorkEntry{}, 0, fmt.Errorf("area %q is not one of %s", *p.Area, strings.Join(WorkAreas, ", "))
		}
		next.Area = a
	}
	if p.Touches != nil {
		next.Touches = ParseWorkTouches(*p.Touches)
		if next.Touches == nil {
			next.Touches = []string{}
		}
	}
	if p.Status != nil {
		st, ok := NormalizeWorkStatus(*p.Status)
		if !ok {
			return WorkEntry{}, 0, fmt.Errorf("status %q is not one of %s", *p.Status, strings.Join(WorkStatuses, ", "))
		}
		next.Status = st
	}
	if p.Owner != nil {
		next.Owner = oneLine(*p.Owner)
	}
	if p.Details != nil {
		next.Details = workText(*p.Details)
	}
	if p.Result != nil {
		next.Result = oneLine(*p.Result)
	}
	note := workText(p.Note)
	noted := note != ""
	if noted {
		if runes(note) > WorkMaxNote {
			return WorkEntry{}, 0, fmt.Errorf("the note is %d characters: %d at most", runes(note), WorkMaxNote)
		}
		line := workNote(now, by, note)
		if next.Notes == "" {
			next.Notes = line
		} else {
			next.Notes += "\n" + line
		}
	}
	switch {
	case next.Status != WorkDone:
		next.DoneAt = ""
	case cur.Status != WorkDone || isNew:
		next.DoneAt = stamp
	}
	if err := WorkEntryProblem(next); err != nil {
		return WorkEntry{}, 0, err
	}
	if !isNew && sameWork(cur, next) {
		return cur, 0, nil
	}

	touches := FormatWorkTouches(next.Touches)
	var id int64
	var before map[string]any
	action := "insert"
	if isNew {
		res, err := tx.Exec(`INSERT INTO workboard (title, area, touches, status, owner, details, notes, result,
			created, created_by, updated, updated_by, done_at, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			next.Title, next.Area, touches, next.Status, next.Owner, next.Details, next.Notes, next.Result,
			stamp, by, stamp, by, next.DoneAt)
		if err != nil {
			return WorkEntry{}, 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return WorkEntry{}, 0, err
		}
	} else {
		id, action = cur.ID, "update"
		if before, err = fullRow(tx, `SELECT * FROM workboard WHERE id = ?`, id); err != nil {
			return WorkEntry{}, 0, err
		}
		res, err := tx.Exec(`UPDATE workboard SET title = ?, area = ?, touches = ?, status = ?, owner = ?, details = ?,
			notes = ?, result = ?, done_at = ?, updated = ?, updated_by = ?, version = version + 1
			WHERE id = ? AND version = ?`,
			next.Title, next.Area, touches, next.Status, next.Owner, next.Details, next.Notes, next.Result, next.DoneAt,
			stamp, by, id, cur.Version)
		if err != nil {
			return WorkEntry{}, 0, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return WorkEntry{}, 0, fmt.Errorf("entry %d changed while it was being written: read it again", id)
		}
	}
	after, err := fullRow(tx, `SELECT * FROM workboard WHERE id = ?`, id)
	if err != nil {
		return WorkEntry{}, 0, err
	}
	audit, err := auditRowChange(tx, who, action, "workboard", map[string]any{"id": id}, before, after,
		workDetail(isNew, id, cur, next, noted))
	if err != nil {
		return WorkEntry{}, 0, err
	}
	out, err := scanWorkEntry(tx.QueryRow(`SELECT `+workCols+` FROM workboard WHERE id = ?`, id))
	if err != nil {
		return WorkEntry{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return WorkEntry{}, 0, err
	}
	return out, audit, nil
}

// sameWork is whether two versions of an entry say the same.
func sameWork(a, b WorkEntry) bool {
	return a.Title == b.Title && a.Area == b.Area && FormatWorkTouches(a.Touches) == FormatWorkTouches(b.Touches) &&
		a.Status == b.Status && a.Owner == b.Owner && a.Details == b.Details && a.Notes == b.Notes &&
		a.Result == b.Result && a.DoneAt == b.DoneAt
}

// workDetail is the audit line's words for a post.
func workDetail(isNew bool, id int64, cur, next WorkEntry, noted bool) string {
	if isNew {
		return fmt.Sprintf("work board: entry %d posted (%s): %s", id, next.Status, next.Title)
	}
	var what []string
	if cur.Status != next.Status {
		what = append(what, cur.Status+" -> "+next.Status)
	}
	for _, f := range []struct {
		name string
		a, b string
	}{
		{"title", cur.Title, next.Title}, {"area", cur.Area, next.Area},
		{"touches", FormatWorkTouches(cur.Touches), FormatWorkTouches(next.Touches)},
		{"owner", cur.Owner, next.Owner}, {"details", cur.Details, next.Details}, {"result", cur.Result, next.Result},
	} {
		if f.a != f.b {
			what = append(what, f.name)
		}
	}
	if noted {
		what = append(what, "a note")
	}
	return fmt.Sprintf("work board: entry %d (%s): %s", id, next.Title, strings.Join(what, ", "))
}

// WorkBoardOverview is the overview as it is now (version 0 and no text when
// there is none).
func (db *SQLiteDB) WorkBoardOverview() (WorkOverview, error) {
	return scanWorkOverview(db.QueryRow(`SELECT text, version, updated, updated_by FROM workboard_overview WHERE id = 1`))
}

func scanWorkOverview(r *sql.Row) (WorkOverview, error) {
	var o WorkOverview
	err := r.Scan(&o.Text, &o.Version, &o.Updated, &o.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkOverview{}, nil
	}
	if err != nil {
		return WorkOverview{}, fmt.Errorf("workboard_overview: %w", err)
	}
	return o, nil
}

// SetWorkOverview replaces the overview's text, sent with the version it was
// read at: an older one is refused with the overview as it is now (a
// *WorkConflict). The change is who's row change in gm_audit, revertible. It
// returns the overview and the line's id (0 when the text was the same).
func (db *SQLiteDB) SetWorkOverview(text string, version int64, who GMEdit) (WorkOverview, int64, error) {
	text = workText(text)
	if runes(text) > WorkMaxOverview {
		return WorkOverview{}, 0, fmt.Errorf("the overview is %d characters: %d at most (the entries hold the work itself)",
			runes(text), WorkMaxOverview)
	}
	by := workBy(who)
	stamp := workStamp(time.Now())
	tx, err := db.Begin()
	if err != nil {
		return WorkOverview{}, 0, err
	}
	defer tx.Rollback()
	cur, err := scanWorkOverview(tx.QueryRow(`SELECT text, version, updated, updated_by FROM workboard_overview WHERE id = 1`))
	if err != nil {
		return WorkOverview{}, 0, err
	}
	if version != cur.Version {
		return WorkOverview{}, 0, &WorkConflict{What: "the overview", Given: version, Version: cur.Version,
			By: cur.UpdatedBy, At: cur.Updated, Current: cur}
	}
	if cur.Version != 0 && text == cur.Text {
		return cur, 0, nil
	}
	const sel = `SELECT * FROM workboard_overview WHERE id = 1`
	var before map[string]any
	action := "insert"
	if cur.Version == 0 {
		if _, err := tx.Exec(`INSERT INTO workboard_overview (id, text, version, updated, updated_by) VALUES (1, ?, 1, ?, ?)`,
			text, stamp, by); err != nil {
			return WorkOverview{}, 0, err
		}
	} else {
		action = "update"
		if before, err = fullRow(tx, sel); err != nil {
			return WorkOverview{}, 0, err
		}
		res, err := tx.Exec(`UPDATE workboard_overview SET text = ?, updated = ?, updated_by = ?, version = version + 1
			WHERE id = 1 AND version = ?`, text, stamp, by, cur.Version)
		if err != nil {
			return WorkOverview{}, 0, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return WorkOverview{}, 0, errors.New("the overview changed while it was being written: read it again")
		}
	}
	after, err := fullRow(tx, sel)
	if err != nil {
		return WorkOverview{}, 0, err
	}
	audit, err := auditRowChange(tx, who, action, "workboard_overview", map[string]any{"id": int64(1)}, before, after,
		fmt.Sprintf("work board: the overview edited (%d characters)", runes(text)))
	if err != nil {
		return WorkOverview{}, 0, err
	}
	out, err := scanWorkOverview(tx.QueryRow(`SELECT text, version, updated, updated_by FROM workboard_overview WHERE id = 1`))
	if err != nil {
		return WorkOverview{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return WorkOverview{}, 0, err
	}
	return out, audit, nil
}

// WorkQuery narrows the board.
type WorkQuery struct {
	// Statuses: none is every status (a done entry: one done within
	// WorkDoneDays, unless OldDone).
	Statuses []string
	Area     string
	Owner    string   // in the owner, any case
	Touches  []string // entries touching any of these (TouchesOverlap)
	Search   string   // in the title, owner, touches, details, notes and result, any case
	OldDone  bool     // done entries older than WorkDoneDays too
	Limit    int      // entries at most: default 200, at most 1000
	Offset   int
}

// WorkBoard is the board as a query reads it.
type WorkBoard struct {
	Overview WorkOverview `json:"overview"`
	Entries  []WorkEntry  `json:"entries"`
	// Matching is how many entries the query matches (Entries is a page).
	Matching int `json:"matching"`
	// Counts is every entry by status, the done ones within WorkDoneDays.
	Counts map[string]int `json:"counts"`
	// OlderDone is how many entries were done before DoneSince.
	OlderDone int    `json:"olderDone"`
	DoneSince string `json:"doneSince"`
}

// workDoneAt is when an entry was done (its last change for a row the table
// editor made done).
func workDoneAt(e WorkEntry) string {
	if e.DoneAt != "" {
		return e.DoneAt
	}
	return e.Updated
}

// allWork is every entry.
func (db *SQLiteDB) allWork() ([]WorkEntry, error) {
	rows, err := db.Query(`SELECT ` + workCols + ` FROM workboard`)
	if err != nil {
		return nil, fmt.Errorf("workboard: %w", err)
	}
	defer rows.Close()
	var out []WorkEntry
	for rows.Next() {
		e, err := scanWorkEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("workboard: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// WorkBoard is the overview and the entries a query takes: in progress,
// blocked, planned, then done (the last WorkDoneDays unless OldDone), each
// the latest changed first.
func (db *SQLiteDB) WorkBoard(q WorkQuery) (WorkBoard, error) {
	ov, err := db.WorkBoardOverview()
	if err != nil {
		return WorkBoard{}, err
	}
	all, err := db.allWork()
	if err != nil {
		return WorkBoard{}, err
	}
	since := workStamp(time.Now().AddDate(0, 0, -WorkDoneDays))
	board := WorkBoard{Overview: ov, Counts: map[string]int{}, DoneSince: since, Entries: []WorkEntry{}}
	for _, st := range WorkStatuses {
		board.Counts[st] = 0
	}
	var want map[string]bool
	for _, s := range q.Statuses {
		st, ok := NormalizeWorkStatus(s)
		if !ok {
			return WorkBoard{}, fmt.Errorf("status %q is not one of %s", s, strings.Join(WorkStatuses, ", "))
		}
		if want == nil {
			want = map[string]bool{}
		}
		want[st] = true
	}
	area := ""
	if strings.TrimSpace(q.Area) != "" {
		a, ok := NormalizeWorkArea(q.Area)
		if !ok {
			return WorkBoard{}, fmt.Errorf("area %q is not one of %s", q.Area, strings.Join(WorkAreas, ", "))
		}
		area = a
	}
	owner := strings.ToLower(strings.TrimSpace(q.Owner))
	search := strings.ToLower(strings.TrimSpace(q.Search))
	var match []WorkEntry
	for _, e := range all {
		old := e.Status == WorkDone && workDoneAt(e) < since
		if old {
			board.OlderDone++
		} else {
			board.Counts[e.Status]++
		}
		if want != nil && !want[e.Status] {
			continue
		}
		if old && !q.OldDone {
			continue
		}
		if area != "" && e.Area != area {
			continue
		}
		if owner != "" && !strings.Contains(strings.ToLower(e.Owner), owner) {
			continue
		}
		if len(q.Touches) > 0 && len(sharedTouches(e.Touches, q.Touches)) == 0 {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(strings.Join([]string{e.Title, e.Owner,
			FormatWorkTouches(e.Touches), e.Details, e.Notes, e.Result}, "\n")), search) {
			continue
		}
		match = append(match, e)
	}
	order := map[string]int{}
	for i, st := range WorkStatuses {
		order[st] = i
	}
	sort.SliceStable(match, func(i, j int) bool {
		a, b := match[i], match[j]
		if order[a.Status] != order[b.Status] {
			return order[a.Status] < order[b.Status]
		}
		ta, tb := a.Updated, b.Updated
		if a.Status == WorkDone {
			ta, tb = workDoneAt(a), workDoneAt(b)
		}
		if ta != tb {
			return ta > tb
		}
		return a.ID > b.ID
	})
	board.Matching = len(match)
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	limit = min(limit, 1000)
	off := min(max(q.Offset, 0), len(match))
	board.Entries = append(board.Entries, match[off:min(off+limit, len(match))]...)
	return board, nil
}

// sharedTouches is which of mine name what one of theirs does.
func sharedTouches(mine, theirs []string) []string {
	var out []string
	for _, a := range mine {
		for _, b := range theirs {
			if TouchesOverlap(a, b) {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// WorkOverlap is another open entry touching something an entry touches.
type WorkOverlap struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Owner   string   `json:"owner"`
	Updated string   `json:"updated"`
	Touches []string `json:"touches"` // what both name, as the other entry names it
}

// WorkOverlaps is every entry but id that is not done and touches any of
// touches: in progress first, then blocked, then planned.
func (db *SQLiteDB) WorkOverlaps(id int64, touches []string) ([]WorkOverlap, error) {
	if len(touches) == 0 {
		return nil, nil
	}
	all, err := db.allWork()
	if err != nil {
		return nil, err
	}
	order := map[string]int{}
	for i, st := range WorkStatuses {
		order[st] = i
	}
	var out []WorkOverlap
	for _, e := range all {
		if e.ID == id || e.Status == WorkDone {
			continue
		}
		if shared := sharedTouches(e.Touches, touches); len(shared) > 0 {
			out = append(out, WorkOverlap{ID: e.ID, Title: e.Title, Status: e.Status, Owner: e.Owner,
				Updated: e.Updated, Touches: shared})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Status] != order[out[j].Status] {
			return order[out[i].Status] < order[out[j].Status]
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Taken is whether the other entry holds what it touches: in progress or
// blocked (a planned entry has changed nothing yet).
func (o WorkOverlap) Taken() bool { return o.Status == WorkInProgress || o.Status == WorkBlocked }

// WorkChange is one line of an entry's or the overview's history, read from
// gm_audit.
type WorkChange struct {
	Audit      int64    `json:"audit"`
	At         string   `json:"at"`
	By         string   `json:"by"`
	Action     string   `json:"action"`
	Changed    []string `json:"changed,omitempty"`
	Detail     string   `json:"detail,omitempty"`
	RevertedBy int64    `json:"revertedBy,omitempty"`
}

// WorkHistory is the changes to one row of the board (table workboard or
// workboard_overview), oldest first, the last limit of them.
func (db *SQLiteDB) WorkHistory(table string, id int64, limit int) ([]WorkChange, error) {
	if table != "workboard" && table != "workboard_overview" {
		return nil, fmt.Errorf("%s is not the work board's", table)
	}
	if limit <= 0 {
		limit = 50
	}
	k1, _ := json.Marshal(map[string]any{"id": id})
	k2, _ := json.Marshal(map[string]any{"id": fmt.Sprint(id)})
	rows, err := db.Query(`SELECT id, at, username, via, action, COALESCE(before, ''), COALESCE(after, ''), detail,
		COALESCE(reverted_by, 0) FROM gm_audit WHERE tbl = ? AND rowkey IN (?, ?) ORDER BY id DESC LIMIT ?`,
		table, string(k1), string(k2), limit)
	if err != nil {
		return nil, fmt.Errorf("gm_audit: %w", err)
	}
	defer rows.Close()
	var out []WorkChange
	for rows.Next() {
		var c WorkChange
		var user, via, before, after string
		if err := rows.Scan(&c.Audit, &c.At, &user, &via, &c.Action, &before, &after, &c.Detail, &c.RevertedBy); err != nil {
			return nil, fmt.Errorf("gm_audit: %w", err)
		}
		c.By = workBy(GMEdit{Username: user, Via: via})
		if c.Action == "update" {
			var b, a map[string]any
			json.Unmarshal([]byte(before), &b)
			json.Unmarshal([]byte(after), &a)
			for k, v := range a {
				switch k {
				case "updated", "updated_by", "version":
					continue
				}
				if fmt.Sprint(v) != fmt.Sprint(b[k]) {
					c.Changed = append(c.Changed, k)
				}
			}
			sort.Strings(c.Changed)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
