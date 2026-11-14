package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// AccountMirror keeps this database's accountinfo a copy of another shard's
// (the user, 2026-11-07: "lets add another shard"; one login works on both).
// One process owns the accounts, with the login port and the account pages; a
// shard beside it has auth.shared_db naming that process's database, and
// everything here goes on reading its own table as ever: a sign-in, a GM
// command's level, the panel's joins, characters.accountid. Only a Pull writes
// the copy, keyed by accountid, every column but lastlogin (each shard's own).
//
// Why a copy rather than that file at each read: the shard goes on working
// with the other process down or its database locked, nothing on this side ever
// holds a lock on it for longer than one SELECT, and no query had to learn about
// two databases. Why not one store of accounts beside both: accountinfo is
// joined and edited by the panel, audited and reverted, in one database.
//
// The other file is opened read-only and closed again at each Pull: no lock or
// page of it is kept, and a file put back under its name (a swap's rollback, a
// restored backup) is simply the file read next. The database is in WAL mode
// since 2026-11-13 (sqlite.go), so a read here and the owner's commits do not
// wait for each other; each read is a whole-table SELECT of a small table.
type AccountMirror struct {
	db   *SQLiteDB
	path string

	mu   sync.Mutex
	held bool
	// the file's size and time at the last Pull that read it whole, and when
	// that was: a file that has not changed is not opened (mirrorMaxQuiet)
	lastSize int64
	lastTime time.Time
	lastPull time.Time
	now      func() time.Time
}

const (
	// mirrorDefaultEvery is how often a Pull runs when auth.shared_refresh is unset.
	mirrorDefaultEvery = 10 * time.Second
	// mirrorMaxQuiet is the longest a Pull trusts an unchanged file's size and
	// time, in case a change fell within the clock's step.
	mirrorMaxQuiet = time.Minute
	// mirrorMostGone is the most accounts one Pull may take away, as a tenth of
	// the copy, but never fewer than this: a file put back by mistake, or a
	// path naming another database, must not empty the accounts.
	mirrorMostGone = 5
)

// mirrorSkip are the columns a Pull leaves alone: lastlogin is each shard's.
var mirrorSkip = map[string]bool{"lastlogin": true}

// NewAccountMirror mirrors the accounts of the database at path into db.
func NewAccountMirror(db *SQLiteDB, path string) *AccountMirror {
	return &AccountMirror{db: db, path: path, now: time.Now}
}

// Hold stops every Pull until Hold(false): nothing of this process writes the
// database while a restart holds it (a swap may put the file back from a copy).
// A nil mirror holds nothing.
func (m *AccountMirror) Hold(on bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = on
	m.lastTime = time.Time{}
}

// Run pulls every so often until the returned function is called.
func (m *AccountMirror) Run(every time.Duration) (stop func()) {
	if every <= 0 {
		every = mirrorDefaultEvery
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		failing := false
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			ch, gone, _, err := m.Pull()
			switch {
			case err != nil && !failing:
				failing = true
				log.Printf("Accounts: the shared accounts cannot be read (%v): the copy here stands", err)
			case err == nil && failing:
				failing = false
				log.Printf("Accounts: the shared accounts can be read again")
			}
			if ch > 0 || gone > 0 {
				log.Printf("Accounts: %d changed and %d gone in the shared accounts, copied", ch, gone)
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// open is the other database, read-only.
func (m *AccountMirror) open() (*sql.DB, error) {
	if _, err := os.Stat(m.path); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Opaque: (&url.URL{Path: m.path}).EscapedPath()}
	dsn := u.String() + "?mode=ro&_pragma=busy_timeout(2000)&_pragma=query_only(1)"
	src, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	src.SetMaxOpenConns(1)
	return src, nil
}

// accountCols is what a Pull copies: accountinfo's columns both databases have,
// the key first, those a Pull leaves alone left out; and how each is read. A date
// comes as its text: the driver would make a time of a DATETIME column, and write
// it back in another spelling.
type accountCols struct {
	names, reads []string
}

// columns is accountCols for these two databases.
func (m *AccountMirror) columns(src *sql.DB) (accountCols, error) {
	var out accountCols
	of := func(q interface {
		Query(string, ...any) (*sql.Rows, error)
	}) (map[string]string, []string, error) {
		rows, err := q.Query(`SELECT name, type FROM pragma_table_info('accountinfo')`)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		set, list := map[string]string{}, []string(nil)
		for rows.Next() {
			var n, t string
			if err := rows.Scan(&n, &t); err != nil {
				return nil, nil, err
			}
			set[n] = t
			list = append(list, n)
		}
		return set, list, rows.Err()
	}
	there, _, err := of(src)
	if err != nil {
		return out, err
	}
	if _, ok := there["accountid"]; !ok {
		return out, errors.New("it has no accountinfo table")
	}
	types, here, err := of(m.db.DB)
	if err != nil {
		return out, err
	}
	add := func(c string) {
		out.names = append(out.names, c)
		if t := strings.ToUpper(types[c]); strings.Contains(t, "DATE") || strings.Contains(t, "TIME") {
			out.reads = append(out.reads, "CAST("+c+" AS TEXT)")
		} else {
			out.reads = append(out.reads, c)
		}
	}
	add("accountid")
	for _, c := range here {
		if _, both := there[c]; c != "accountid" && both && !mirrorSkip[c] {
			add(c)
		}
	}
	return out, nil
}

// accountRow is a row's columns, and how it compares.
type accountRow []any

func (r accountRow) same(o accountRow) bool {
	return fmt.Sprintf("%#v", []any(r)) == fmt.Sprintf("%#v", []any(o))
}

func (r accountRow) id() int64 {
	id, _ := r[0].(int64)
	return id
}

func readAccountRows(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, cols accountCols, where string, args ...any) ([]accountRow, error) {
	rows, err := q.Query(`SELECT `+strings.Join(cols.reads, ", ")+` FROM accountinfo `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []accountRow
	for rows.Next() {
		r := make(accountRow, len(cols.reads))
		ptr := make([]any, len(cols.reads))
		for i := range r {
			ptr[i] = &r[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			return nil, err
		}
		for i, v := range r {
			if b, ok := v.([]byte); ok { // the driver's buffer is its own until the next row
				r[i] = append([]byte(nil), b...)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// writeAccount puts a row in the copy: REPLACE for a new row, with lastlogin
// kept for one already there (REPLACE would clear it).
func writeAccount(tx *sql.Tx, cols accountCols, r accountRow) error {
	var kept any
	_ = tx.QueryRow(`SELECT CAST(lastlogin AS TEXT) FROM accountinfo WHERE accountid = ?`, r.id()).Scan(&kept)
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(cols.names)), ", ")
	if _, err := tx.Exec(`INSERT OR REPLACE INTO accountinfo (`+strings.Join(cols.names, ", ")+`) VALUES (`+marks+`)`, r...); err != nil {
		return err
	}
	if kept != nil {
		if _, err := tx.Exec(`UPDATE accountinfo SET lastlogin = ? WHERE accountid = ?`, kept, r.id()); err != nil {
			return err
		}
	}
	return nil
}

// Pull brings the whole copy up to date: changed the rows written, gone those
// taken away, total the accounts the other database holds. A file whose size
// and time have not moved is not opened (for mirrorMaxQuiet), so two empty shards
// do nothing here.
func (m *AccountMirror) Pull() (changed, gone, total int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held {
		return 0, 0, 0, nil
	}
	fi, err := os.Stat(m.path)
	if err != nil {
		return 0, 0, 0, err
	}
	now := m.now()
	if fi.Size() == m.lastSize && fi.ModTime().Equal(m.lastTime) && now.Sub(m.lastPull) < mirrorMaxQuiet {
		return 0, 0, 0, nil
	}
	src, err := m.open()
	if err != nil {
		return 0, 0, 0, err
	}
	defer src.Close()
	cols, err := m.columns(src)
	if err != nil {
		return 0, 0, 0, err
	}
	there, err := readAccountRows(src, cols, `ORDER BY accountid`)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(there) == 0 {
		return 0, 0, 0, errors.New("it holds no accounts")
	}
	here, err := readAccountRows(m.db.DB, cols, ``)
	if err != nil {
		return 0, 0, 0, err
	}
	byID := make(map[int64]accountRow, len(here))
	for _, r := range here {
		byID[r.id()] = r
	}
	tx, err := m.db.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()
	// those gone first: a name another account takes over must be free
	seen := make(map[int64]bool, len(there))
	for _, r := range there {
		seen[r.id()] = true
	}
	var goneIDs []int64
	for id := range byID {
		if !seen[id] {
			goneIDs = append(goneIDs, id)
		}
	}
	if len(goneIDs) > max(mirrorMostGone, len(here)/10) {
		log.Printf("Accounts: %d of %d accounts are not in %s: left as they are (a file put back? remove them by hand if not)", len(goneIDs), len(here), m.path)
		goneIDs = nil
	}
	for _, id := range goneIDs {
		if _, err := tx.Exec(`DELETE FROM accountinfo WHERE accountid = ?`, id); err != nil {
			return 0, 0, 0, err
		}
		gone++
	}
	for _, r := range there {
		if old, ok := byID[r.id()]; ok && r.same(old) {
			continue
		}
		if err := writeAccount(tx, cols, r); err != nil {
			return 0, 0, 0, err
		}
		changed++
	}
	roles, err := mirrorRoles(tx, src)
	if err != nil {
		return 0, 0, 0, err
	}
	changed += roles
	if changed > 0 || gone > 0 {
		if err := tx.Commit(); err != nil {
			return 0, 0, 0, err
		}
	}
	m.lastSize, m.lastTime, m.lastPull = fi.Size(), fi.ModTime(), now
	return changed, gone, len(there), nil
}

// PullOne brings one username's account up to date, for a sign-in: one made
// since the last Pull is here for it, one taken away is gone from here, and a
// password or a ban is the one in force now.
func (m *AccountMirror) PullOne(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held {
		return nil
	}
	src, err := m.open()
	if err != nil {
		return err
	}
	defer src.Close()
	cols, err := m.columns(src)
	if err != nil {
		return err
	}
	const byName = `WHERE LOWER(username) = LOWER(?)`
	there, err := readAccountRows(src, cols, byName, username)
	if err != nil {
		return err
	}
	here, err := readAccountRows(m.db.DB, cols, byName, username)
	if err != nil {
		return err
	}
	if len(there) == 0 {
		// no such account there. A whole table's absence is Pull's to judge;
		// one name is not.
		if len(here) == 0 {
			return nil
		}
		_, err := m.db.Exec(`DELETE FROM accountinfo WHERE LOWER(username) = LOWER(?)`, username)
		return err
	}
	if len(here) == 1 && len(there) == 1 && there[0].same(here[0]) {
		return nil
	}
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range there { // two accounts may differ by case alone
		if err := writeAccount(tx, cols, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// mirrorRoleTables are the panel's role tables, keyed by accountid, that go
// with the accounts (gmweb/shard.go: "Testers and storytellers go with the
// accounts: one is made on the owner's panel"): a tester, a sub GM (a tester's
// row marked subgm: handlers/devcmds.go's subGMCommands) and a storyteller of
// the main shard are one here too. Until 2026-11-14 a Pull copied accountinfo
// alone, so on the Live Server nobody was a tester, a sub GM or a storyteller,
// and its panel refused to make one ("change them on that server's GM panel").
var mirrorRoleTables = []string{"gm_testers", "gm_storytellers"}

// mirrorRoles makes each of mirrorRoleTables here, inside tx, the other
// database's (src) rows of it, in the columns both have; a table either side
// lacks is passed over (the panel makes them at its start). It returns how many
// rows it wrote or took away.
func mirrorRoles(tx *sql.Tx, src *sql.DB) (int, error) {
	n := 0
	for _, t := range mirrorRoleTables {
		if !tableExists(src, t) || !tableExists(tx, t) {
			continue
		}
		mine, err := tableColumns(tx, t)
		if err != nil {
			return 0, err
		}
		rows, err := src.Query(`SELECT name FROM pragma_table_info(?)`, t)
		if err != nil {
			return 0, err
		}
		theirs := map[string]bool{}
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				rows.Close()
				return 0, err
			}
			theirs[c] = true
		}
		rows.Close()
		var cols []string
		for _, c := range mine {
			if theirs[c] {
				cols = append(cols, quoteCol(c))
			}
		}
		if len(cols) == 0 || !theirs["accountid"] {
			continue
		}
		read := func(q interface {
			Query(string, ...any) (*sql.Rows, error)
		}) (map[string][]any, error) {
			rs, err := q.Query(`SELECT ` + strings.Join(cols, ", ") + ` FROM ` + t)
			if err != nil {
				return nil, err
			}
			defer rs.Close()
			out := map[string][]any{}
			for rs.Next() {
				r := make([]any, len(cols))
				ptr := make([]any, len(cols))
				for i := range r {
					ptr[i] = &r[i]
				}
				if err := rs.Scan(ptr...); err != nil {
					return nil, err
				}
				for i, v := range r {
					if b, ok := v.([]byte); ok {
						r[i] = append([]byte(nil), b...)
					}
				}
				out[fmt.Sprintf("%#v", r)] = r
			}
			return out, rs.Err()
		}
		want, err := read(src)
		if err != nil {
			return 0, err
		}
		have, err := read(tx)
		if err != nil {
			return 0, err
		}
		same := len(want) == len(have)
		for k := range want {
			if _, ok := have[k]; !ok {
				same = false
			}
		}
		if same {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			return 0, err
		}
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
		for _, r := range want {
			if _, err := tx.Exec(`INSERT INTO `+t+` (`+strings.Join(cols, ", ")+`) VALUES (`+marks+`)`, r...); err != nil {
				return 0, fmt.Errorf("%s: %w", t, err)
			}
		}
		n += len(want) + len(have)
		log.Printf("Accounts: %s copied from the shared accounts' database (%d rows, %d before)", t, len(want), len(have))
	}
	return n, nil
}
