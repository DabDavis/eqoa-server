package database

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

// A sanitised snapshot: a copy of the live database a GM can take to a
// machine of their own (the GM panel's download_snapshot) to run the
// real-data tests (EQOA_REAL_DB) on, with every credential and every
// person's data taken out and the game's content left whole.
//
// It is made from a consistent copy (VACUUM INTO), never from the live file,
// and the copy is changed with its own connection, so the game waits only for
// the copy. Then:
//
//   - every table and column is read against snapshotRules, and the snapshot
//     REFUSES when a table it does not know, or a column it has no rule for,
//     is named like a secret (SecretLikeName): a table added tomorrow cannot
//     leak by default, it stops the snapshot until it is given a rule;
//   - the rules run in one transaction, with secure_delete on and no trigger
//     in place (each put back after), and the result is written to a new file
//     by a second VACUUM INTO, so no page of the copy that held a secret is in
//     the file handed out;
//   - that file is checked: every table cleared is empty, every column
//     blanked blank, and every value the rules took out of a scanned column
//     (a password hash, an address, a salt...) is searched for in every cell
//     of every table: one found anywhere refuses the snapshot, naming where.

// SnapshotVersion names the sanitiser's rules: the panel's audit line and the
// snapshot's own snapshot_info table say which made a file. Raise it with
// every change to what is taken out.
const SnapshotVersion = 1

// SnapshotInfoTable is the table a snapshot carries saying what it is: the
// sanitiser's version, when it was made and by whom, and what was taken out.
const SnapshotInfoTable = "snapshot_info"

// snapshotRule is what the sanitiser does to one table.
type snapshotRule struct {
	// clear deletes every row; the table stays, empty.
	clear bool
	// keep names the only columns kept: every other is blanked, a column
	// added later included. Empty: every column is kept.
	keep []string
	// blank are columns set to NULL ('' or 0 where they may not be NULL).
	blank []string
	// pseudonym are columns whose values are each replaced by a stand-in of
	// the same length, an HMAC under a key the snapshot throws away: the same
	// value gives the same stand-in, so the rows still group by it, but
	// nothing links the stand-in back to the value.
	pseudonym []string
	// allow are columns named like a secret that hold none. (A table named
	// like one needs only a rule: whoever wrote it has looked.)
	allow []string
	// scan are columns whose values (before the rules) are searched for in
	// the finished snapshot: none may survive anywhere in it.
	scan []string
	// also runs after the rest, for what the fields cannot say; it returns
	// what it did, for the report. verify checks it on the finished file.
	also   func(tx *sql.Tx) (string, error)
	verify func(db *sql.DB) error
	// why is what the table holds and why the rule is what it is.
	why string
}

// snapshotRules is what the sanitiser knows. A table not here is kept whole,
// unless it or a column of it is named like a secret, which refuses the
// snapshot until it is given a rule. CLAUDE.md's table of what a snapshot
// leaves out is these rules.
var snapshotRules = map[string]snapshotRule{
	// Credentials and the people behind the accounts.
	"accountinfo": {
		keep: []string{"accountid", "username", "acctstatus", "acctlevel", "acctcreation",
			"result", "subtime", "partime", "subfeatures", "gamefeatures"},
		scan:   []string{"pwhash", "email", "ipaddress"},
		also:   renamePlayerAccounts,
		verify: playersRenamed,
		why: "the password's bcrypt hash, the address and e-mail, the Station account's name, birthday, " +
			"country, zip and sex, and when the person last played are blanked (a column added later too: " +
			"only the game's own columns are kept); a player's login name (acctlevel 0) becomes " +
			"account-<id> (a login name is often a person's e-mail or name, and half a credential); a GM's " +
			"is kept (the work board and the panel name GMs)",
	},
	"gm_tokens": {clear: true, scan: []string{"hash", "last_ip"},
		why: "the GMs' API, OAuth and refresh tokens (hashes) and the address each was last used from"},
	"gm_totp": {clear: true, scan: []string{"secret"},
		why: "the GMs' authenticator secrets"},
	"gm_oauth_clients": {clear: true, scan: []string{"client_id"},
		why: "the OAuth clients registered and the addresses they return to"},
	"gm_audit": {clear: true, scan: []string{"ip"},
		why: "who did what from which address: sign-ins, tokens made, admin SQL (which can carry anything, " +
			"a password hash included) and rows before and after, accounts' among them"},
	"spellfx_meta": {clear: true, scan: []string{"value"},
		why: "the salt the community visuals' senders' addresses are hashed with (with it an IPv4 " +
			"sender's hash walks back to the address); a server makes a new one when it needs one"},
	"mapmark_meta": {clear: true, scan: []string{"value"},
		why: "the map suggestions' senders' salt, for the same reason"},
	"spellfx_suggestions": {pseudonym: []string{"addr_hash"}, scan: []string{"addr_hash"},
		why: "the sender's salted address hash is replaced by a stand-in: one sender's suggestions still " +
			"group, and nothing links them to the live server's hashes"},
	"mapmark_suggestions": {pseudonym: []string{"sender_hash"}, scan: []string{"sender_hash"},
		why: "the same, for the map suggestions' senders"},
	"workboard": {clear: true,
		why: "what people and agents wrote to each other: a note can hold a password, a token or an address"},
	"workboard_overview": {clear: true,
		why: "the same; a server seeds the default overview again when it opens the copy"},
	// SQLite's index statistics: stat4 keeps sample KEYS of the indexes, so
	// a sample of an index on a column taken out above
	"sqlite_stat4": {clear: true, why: "samples of the indexes' keys"},
	"sqlite_stat3": {clear: true, why: "samples of the indexes' keys"},

	// What the snapshot says it is (kept by a snapshot of a snapshot).
	SnapshotInfoTable: {allow: []string{"key"},
		why: "the sanitiser's version, when the snapshot was made and by whom, what it took out"},

	// Named like a secret, holding none: the game's own.
	"hotkeys": {allow: []string{"hotkeyid"},
		why: "a character's quick-chat keys (hotkeyid is the row's id)"},

	// Named like a secret, holding none: the content tools' provenance (their
	// --undo, and what the real-data tests read). A key column is a row's key
	// in the table the tool changed, a meta table's key the name of a fact.
	"archived_spells_fix_log":    {allow: []string{"rowkey"}, why: "tools/fix_archived_spells.py's log"},
	"boss_loot_seed":             {allow: []string{"key"}, why: "tools/seed_boss_loot.py's log"},
	"camp_loot_seed":             {allow: []string{"key"}, why: "tools/seed_camp_loot.py's log"},
	"eqoadata_extra_import_meta": {allow: []string{"key"}, why: "tools/import_eqoadata_mastery_spells.py's facts"},
	"eqoadata_extra_import_rows": {allow: []string{"key", "key2"}, why: "tools/import_eqoadata_mastery_spells.py's log"},
	"eqoadata_spell_import_meta": {allow: []string{"key"}, why: "tools/import_eqoadata_spells.py's facts"},
	"eqoadata_spell_import_rows": {allow: []string{"key", "key2"}, why: "tools/import_eqoadata_spells.py's log"},
	"fishing_seed":               {allow: []string{"key"}, why: "tools/seed_fishing.py's log"},
	"grocer_stock":               {allow: []string{"key"}, why: "tools/stock_grocers.py's log"},
	"merchant_stock":             {allow: []string{"key"}, why: "tools/stock_merchants.py's log"},
	"pod_seed_meta":              {allow: []string{"key"}, why: "tools/seed_pod.py's facts"},
	"pod_seed_rules":             {allow: []string{"keycol"}, why: "tools/seed_pod.py's log (keycol names a key column)"},
	"potion_rules_log":           {allow: []string{"rowkey"}, why: "tools/fix_potion_rules.py's log"},
	"prh_import_keyed":           {why: "tools/import_prh_content.py's log (keyed NPC rows)"},
	"prh_import_meta":            {allow: []string{"key"}, why: "tools/import_prh_content.py's facts"},
	"quest_import_meta":          {allow: []string{"key"}, why: "tools/import_quests.py's facts"},
	"quest_import_quests":        {allow: []string{"key"}, why: "tools/import_quests.py's log (key: a guide's section)"},
	"quest_npc_seed_meta":        {allow: []string{"key"}, why: "tools/seed_quest_npcs.py's facts"},
	"sandstorm_import_meta":      {allow: []string{"key"}, why: "tools/import_sandstorm_harvest.py's facts"},
	"sandstorm_import_rows":      {allow: []string{"key"}, why: "tools/import_sandstorm_harvest.py's log"},
	"spell_fill_log":             {allow: []string{"rowkey"}, why: "the spell fill's log"},
	"spell_fill_meta":            {allow: []string{"key"}, why: "the spell fill's facts"},
	"spell_pages_sweep_log":      {allow: []string{"rowkey"}, why: "tools/sweep_spell_pages.py's log"},
	"spell_records_fix_log":      {allow: []string{"rowkey"}, why: "tools/fix_spell_records.py's log"},
	"tested_spells_fix_log":      {allow: []string{"rowkey"}, why: "tools/fix_tested_spells.py's log"},
	"tradeskill_seed":            {allow: []string{"key"}, why: "tools/seed_tradeskills.py's log"},
	"wildlife_seed_meta":         {allow: []string{"key"}, why: "tools/seed_wildlife.py's facts"},
}

// renamePlayerAccounts gives every player's account (acctlevel 0 or less) the
// login name account-<id>; a GM's is kept.
func renamePlayerAccounts(tx *sql.Tx) (string, error) {
	res, err := tx.Exec(`UPDATE accountinfo SET username = 'account-' || accountid WHERE COALESCE(acctlevel, 0) <= 0`)
	if err != nil {
		return "", fmt.Errorf("renaming the players' accounts: %w", err)
	}
	n, _ := res.RowsAffected()
	return fmt.Sprintf("%d players' login names made account-<id>", n), nil
}

// playersRenamed is whether every player's account is called account-<id>.
func playersRenamed(db *sql.DB) error {
	var n int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM accountinfo WHERE COALESCE(acctlevel, 0) <= 0
		AND username IS NOT 'account-' || accountid`).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("%d players' accounts kept their login names", n)
	}
	return nil
}

// SnapshotStep is what the sanitiser did to one table.
type SnapshotStep struct {
	Table string `json:"table"`
	Did   string `json:"did"`
}

// SnapshotReport is what a snapshot is and what it took out.
type SnapshotReport struct {
	Version int            `json:"version"`
	Made    time.Time      `json:"made"`
	Bytes   int64          `json:"bytes"`
	Tables  int            `json:"tables"`
	Steps   []SnapshotStep `json:"removed"`
	// Scanned is how many values taken out were searched for in the finished
	// file, and found in none of it.
	Scanned int `json:"scanned"`
}

// SnapshotRefused is a snapshot the sanitiser would not make, with every
// reason: nothing was written.
type SnapshotRefused struct{ Reasons []string }

func (e *SnapshotRefused) Error() string {
	return "the snapshot was refused, and nothing was written: " + strings.Join(e.Reasons, "; ")
}

// WriteSnapshot writes a sanitised snapshot of the database to out (which
// must not exist), by is who asked (kept in its snapshot_info). The copy it
// is made from goes beside out and is removed whatever happens; out is left
// only when every check passed.
func (db *SQLiteDB) WriteSnapshot(out, by string) (*SnapshotReport, error) {
	raw := out + ".copy"
	if err := createEmpty(raw); err != nil {
		return nil, err
	}
	defer removeDB(raw)
	// the only moment the live database is touched: a consistent copy, made
	// on the game's own connection (the game waits for it, as for a backup)
	if _, err := db.Exec(`VACUUM INTO ?`, raw); err != nil {
		return nil, fmt.Errorf("copying the database: %w", err)
	}
	return SanitizeSnapshot(raw, out, by)
}

// SanitizeSnapshot makes a sanitised snapshot at out (which must not exist)
// of the database file raw, a COPY, which it changes: never give it a live
// database.
func SanitizeSnapshot(raw, out, by string) (rep *SnapshotReport, err error) {
	if err := createEmpty(out); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			removeDB(out)
		}
	}()
	made := time.Now().UTC().Truncate(time.Second)
	rep = &SnapshotReport{Version: SnapshotVersion, Made: made}

	src, err := openSnapshotDB(raw)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	tables, err := snapshotTables(src)
	if err != nil {
		return nil, err
	}
	if reasons := checkSnapshotNames(tables); len(reasons) > 0 {
		return nil, &SnapshotRefused{reasons}
	}
	secrets, err := collectSnapshotSecrets(src, tables)
	if err != nil {
		return nil, err
	}

	// no trigger may run while the rules do (one could copy a value
	// elsewhere): each is put back, as it was, once they have
	triggers, err := dropTriggers(src)
	if err != nil {
		return nil, err
	}
	steps, err := applySnapshotRules(src, tables, made, by)
	if err != nil {
		return nil, err
	}
	for _, t := range triggers {
		if _, err := src.Exec(t); err != nil {
			return nil, fmt.Errorf("putting a trigger back: %w", err)
		}
	}
	rep.Steps = steps

	// a new file, so no page of the copy that held a secret goes with it
	if _, err := src.Exec(`VACUUM INTO ?`, out); err != nil {
		return nil, fmt.Errorf("writing the snapshot: %w", err)
	}
	src.Close()

	// and the file handed out is read again, as it is
	if err := verifySnapshot(out, secrets); err != nil {
		return nil, err
	}
	fi, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	rep.Bytes = fi.Size()
	rep.Tables = len(tables)
	if !hasTable(tables, SnapshotInfoTable) {
		rep.Tables++
	}
	rep.Scanned = len(secrets.values)
	return rep, nil
}

// createEmpty makes an empty file only its owner reads, for VACUUM INTO to
// fill (it takes an empty file, and leaves the mode alone).
func createEmpty(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// removeDB removes a database file and its journal.
func removeDB(path string) {
	os.Remove(path)
	os.Remove(path + "-journal")
	os.Remove(path + "-wal")
	os.Remove(path + "-shm")
}

// openSnapshotDB opens a copy with a connection of its own: deleted content
// overwritten, the rollback journal in memory (the file is a copy: a crash
// loses nothing), no foreign key acting on a delete.
func openSnapshotDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{`PRAGMA secure_delete = ON`, `PRAGMA journal_mode = MEMORY`,
		`PRAGMA foreign_keys = OFF`} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return db, nil
}

// snapTable is a table as the sanitiser reads it.
type snapTable struct {
	name string
	cols []snapColumn
}

type snapColumn struct {
	name     string
	typ      string
	notNull  bool
	affinity string
}

func (t *snapTable) col(name string) *snapColumn {
	for i := range t.cols {
		if strings.EqualFold(t.cols[i].name, name) {
			return &t.cols[i]
		}
	}
	return nil
}

func hasTable(tables []*snapTable, name string) bool {
	for _, t := range tables {
		if t.name == name {
			return true
		}
	}
	return false
}

// snapshotTables is every table that holds rows: SQLite's own but its
// statistics left out (sqlite_sequence holds only the tables' names and
// their highest ids).
func snapshotTables(db *sql.DB) ([]*snapTable, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		if strings.HasPrefix(n, "sqlite_") && !strings.HasPrefix(n, "sqlite_stat") {
			continue
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []*snapTable
	for _, n := range names {
		t := &snapTable{name: n}
		cr, err := db.Query(`SELECT name, type, "notnull" FROM pragma_table_info(?)`, n)
		if err != nil {
			return nil, err
		}
		for cr.Next() {
			var c snapColumn
			var nn int
			if err := cr.Scan(&c.name, &c.typ, &nn); err != nil {
				cr.Close()
				return nil, err
			}
			c.notNull = nn != 0
			c.affinity = snapAffinity(c.typ)
			t.cols = append(t.cols, c)
		}
		cr.Close()
		if err := cr.Err(); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// snapAffinity is SQLite's type affinity for a declared type.
func snapAffinity(decl string) string {
	t := strings.ToUpper(decl)
	switch {
	case strings.Contains(t, "INT"):
		return "INTEGER"
	case strings.Contains(t, "CHAR"), strings.Contains(t, "CLOB"), strings.Contains(t, "TEXT"):
		return "TEXT"
	case t == "" || strings.Contains(t, "BLOB"):
		return "BLOB"
	case strings.Contains(t, "REAL"), strings.Contains(t, "FLOA"), strings.Contains(t, "DOUB"):
		return "REAL"
	}
	return "NUMERIC"
}

func inFold(list []string, name string) bool {
	for _, s := range list {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}

// checkSnapshotNames is every table and column the rules do not cover that is
// named like a secret: a reason each, for SnapshotRefused.
func checkSnapshotNames(tables []*snapTable) []string {
	var reasons []string
	for _, t := range tables {
		rule, known := snapshotRules[t.name]
		if !known && SecretLikeName(t.name) {
			reasons = append(reasons, fmt.Sprintf("table %s is named like it holds a secret, and the sanitiser has no "+
				"rule for it", t.name))
			continue
		}
		if rule.clear {
			continue // every row goes, whatever its columns
		}
		var bad []string
		for _, c := range t.cols {
			if !SecretLikeName(c.name) {
				continue
			}
			covered := inFold(rule.blank, c.name) || inFold(rule.pseudonym, c.name) || inFold(rule.allow, c.name) ||
				(len(rule.keep) > 0 && !inFold(rule.keep, c.name))
			if !covered {
				bad = append(bad, c.name)
			}
		}
		if len(bad) > 0 {
			what := "a table the sanitiser does not know"
			if known {
				what = "a table whose rule does not name it"
			}
			reasons = append(reasons, fmt.Sprintf("%s.%s is named like it holds a secret, in %s", t.name,
				strings.Join(bad, ", "+t.name+"."), what))
		}
	}
	if len(reasons) > 0 {
		reasons = append(reasons, "give each a rule in internal/database/snapshot.go (snapshotRules: clear, blank, "+
			"pseudonym, or allow with the reason it holds no secret)")
	}
	return reasons
}

// snapSecrets is the values the rules take out of the scanned columns, which
// the finished file must not hold anywhere.
type snapSecrets struct {
	values [][]byte
	from   []string // the table.column each came from
}

// snapMinSecret is the shortest value searched for: an IPv4 address is 7.
const snapMinSecret = 7

// worthScanning is whether a value taken out is one to search for: long
// enough not to be found by chance, and not an address that names nobody
// (the loopback, a private network's).
func worthScanning(v []byte) bool {
	s := strings.TrimSpace(string(v))
	if len(s) < snapMinSecret {
		return false
	}
	host := s
	if h, _, err := net.SplitHostPort(s); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast())
	}
	return true
}

func collectSnapshotSecrets(db *sql.DB, tables []*snapTable) (*snapSecrets, error) {
	out := &snapSecrets{}
	seen := map[string]bool{}
	for _, t := range tables {
		rule := snapshotRules[t.name]
		for _, name := range rule.scan {
			c := t.col(name)
			if c == nil {
				continue // an older database's table without it holds none
			}
			rows, err := db.Query(`SELECT DISTINCT CAST(` + quoteSnap(c.name) + ` AS BLOB) FROM ` + quoteSnap(t.name) +
				` WHERE ` + quoteSnap(c.name) + ` IS NOT NULL`)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var v []byte
				if err := rows.Scan(&v); err != nil {
					rows.Close()
					return nil, err
				}
				if !worthScanning(v) || seen[string(v)] {
					continue
				}
				seen[string(v)] = true
				out.values = append(out.values, bytes.Clone(v))
				out.from = append(out.from, t.name+"."+c.name)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func quoteSnap(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// blankValue is what a blanked column is set to: NULL, or the empty value of
// its kind where it may not be NULL.
func blankValue(c *snapColumn) string {
	if !c.notNull {
		return "NULL"
	}
	switch c.affinity {
	case "TEXT":
		return "''"
	case "BLOB":
		return "x''"
	}
	return "0"
}

// dropTriggers drops every trigger, returning the statements that make them
// again.
func dropTriggers(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master WHERE type = 'trigger' AND sql IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	var names, stmts []string
	for rows.Next() {
		var n, s string
		if err := rows.Scan(&n, &s); err != nil {
			rows.Close()
			return nil, err
		}
		names, stmts = append(names, n), append(stmts, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, n := range names {
		if _, err := db.Exec(`DROP TRIGGER ` + quoteSnap(n)); err != nil {
			return nil, err
		}
	}
	return stmts, nil
}

// applySnapshotRules runs the rules, in one transaction, and writes the
// snapshot's snapshot_info.
func applySnapshotRules(db *sql.DB, tables []*snapTable, made time.Time, by string) ([]SnapshotStep, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var steps []SnapshotStep
	for _, t := range tables {
		rule, ok := snapshotRules[t.name]
		if !ok {
			continue
		}
		var did []string
		if rule.clear {
			res, err := tx.Exec(`DELETE FROM ` + quoteSnap(t.name))
			if err != nil {
				return nil, fmt.Errorf("clearing %s: %w", t.name, err)
			}
			n, _ := res.RowsAffected()
			steps = append(steps, SnapshotStep{t.name, fmt.Sprintf("every row deleted (%d)", n)})
			continue
		}
		var blank []*snapColumn
		for i := range t.cols {
			c := &t.cols[i]
			if inFold(rule.blank, c.name) || (len(rule.keep) > 0 && !inFold(rule.keep, c.name)) {
				blank = append(blank, c)
			}
		}
		if len(blank) > 0 {
			var sets, names []string
			for _, c := range blank {
				sets = append(sets, quoteSnap(c.name)+" = "+blankValue(c))
				names = append(names, c.name)
			}
			res, err := tx.Exec(`UPDATE ` + quoteSnap(t.name) + ` SET ` + strings.Join(sets, ", "))
			if err != nil {
				return nil, fmt.Errorf("blanking %s: %w", t.name, err)
			}
			n, _ := res.RowsAffected()
			did = append(did, fmt.Sprintf("%s blanked in %d rows", strings.Join(names, ", "), n))
		}
		for _, name := range rule.pseudonym {
			c := t.col(name)
			if c == nil {
				continue
			}
			n, err := pseudonymise(tx, t.name, c.name, key)
			if err != nil {
				return nil, err
			}
			did = append(did, fmt.Sprintf("%s replaced by a stand-in in %d rows", c.name, n))
		}
		if rule.also != nil {
			d, err := rule.also(tx)
			if err != nil {
				return nil, err
			}
			did = append(did, d)
		}
		if len(did) > 0 {
			steps = append(steps, SnapshotStep{t.name, strings.Join(did, "; ")})
		}
	}
	// what the snapshot is
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS ` + SnapshotInfoTable + ` (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	removed, _ := json.Marshal(steps)
	for k, v := range map[string]string{
		"sanitiser": fmt.Sprint(SnapshotVersion),
		"made":      made.Format(time.RFC3339),
		"by":        by,
		"removed":   string(removed),
		"note": "A sanitised copy of a live server's database (internal/database/snapshot.go): no credential, no " +
			"address, no person's data. It is the server's content all the same: never commit it, attach it or " +
			"share it.",
	} {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO `+SnapshotInfoTable+` (key, value) VALUES (?, ?)`, k, v); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return steps, nil
}

// pseudonymise replaces each value of a column by its stand-in, and says how
// many rows it changed.
func pseudonymise(tx *sql.Tx, table, col string, key []byte) (int64, error) {
	rows, err := tx.Query(`SELECT DISTINCT ` + quoteSnap(col) + ` FROM ` + quoteSnap(table) + ` WHERE ` +
		quoteSnap(col) + ` IS NOT NULL AND ` + quoteSnap(col) + ` != ''`)
	if err != nil {
		return 0, err
	}
	var vals []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return 0, err
		}
		vals = append(vals, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, v := range vals {
		res, err := tx.Exec(`UPDATE `+quoteSnap(table)+` SET `+quoteSnap(col)+` = ? WHERE `+quoteSnap(col)+` = ?`,
			standIn(key, v), v)
		if err != nil {
			return 0, fmt.Errorf("replacing %s.%s: %w", table, col, err)
		}
		k, _ := res.RowsAffected()
		n += k
	}
	return n, nil
}

// standIn is a value's stand-in: an HMAC-SHA256 under the snapshot's own key,
// in hex, as long as the value (the hashes it replaces are hex).
func standIn(key []byte, v string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(v))
	s := hex.EncodeToString(m.Sum(nil))
	for len(s) < len(v) {
		m.Write([]byte(s))
		s += hex.EncodeToString(m.Sum(nil))
	}
	return s[:max(len(v), 1)]
}

// verifySnapshot reads the finished file as it is: whole, its cleared tables
// empty, its blanked columns blank, and no value taken out anywhere in it.
func verifySnapshot(path string, secrets *snapSecrets) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only = 1`); err != nil {
		return err
	}
	var ok string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&ok); err != nil || ok != "ok" {
		return fmt.Errorf("the snapshot is not a whole database (%s, %v)", ok, err)
	}
	tables, err := snapshotTables(db)
	if err != nil {
		return err
	}
	var reasons []string
	if r := checkSnapshotNames(tables); len(r) > 0 {
		reasons = append(reasons, r...)
	}
	for _, t := range tables {
		rule, ok := snapshotRules[t.name]
		if !ok {
			continue
		}
		if rule.clear {
			var n int64
			if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteSnap(t.name)).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				reasons = append(reasons, fmt.Sprintf("%s kept %d rows", t.name, n))
			}
			continue
		}
		if rule.verify != nil {
			if err := rule.verify(db); err != nil {
				reasons = append(reasons, fmt.Sprintf("%s: %v", t.name, err))
			}
		}
		for i := range t.cols {
			c := &t.cols[i]
			if !(inFold(rule.blank, c.name) || (len(rule.keep) > 0 && !inFold(rule.keep, c.name))) {
				continue
			}
			var n int64
			if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quoteSnap(t.name) + ` WHERE ` + quoteSnap(c.name) +
				` IS NOT ` + blankValue(c)).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				reasons = append(reasons, fmt.Sprintf("%s.%s is not blank in %d rows", t.name, c.name, n))
			}
		}
	}
	if len(reasons) == 0 {
		where, err := findSecrets(db, tables, secrets)
		if err != nil {
			return err
		}
		reasons = append(reasons, where...)
	}
	if len(reasons) > 0 {
		return &SnapshotRefused{reasons}
	}
	return nil
}

// findSecrets searches every cell of every table for the values taken out,
// and names each place one is found (a cell, not a byte of the file: a value
// split between two pages of the file is found all the same).
func findSecrets(db *sql.DB, tables []*snapTable, secrets *snapSecrets) ([]string, error) {
	m := newSecretMatcher(secrets.values)
	if m == nil {
		return nil, nil
	}
	var found []string
	reported := map[string]bool{}
	for _, t := range tables {
		if len(t.cols) == 0 {
			continue
		}
		rows, err := db.Query(`SELECT * FROM ` + quoteSnap(t.name))
		if err != nil {
			return nil, err
		}
		cols, _ := rows.Columns()
		cells := make([]sql.RawBytes, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return nil, err
			}
			for i, cell := range cells {
				if k := m.find(cell); k >= 0 {
					where := fmt.Sprintf("a value taken out of %s is still in %s.%s", secrets.from[k], t.name, cols[i])
					if !reported[where] {
						reported[where] = true
						found = append(found, where)
					}
				}
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	sort.Strings(found)
	return found, nil
}

// secretMatcher finds any of many byte strings in a cell in one pass: a
// table of the strings' first two bytes, then their first four.
type secretMatcher struct {
	first [1 << 16]bool
	by4   map[uint32][]int
	pats  [][]byte
}

func newSecretMatcher(pats [][]byte) *secretMatcher {
	if len(pats) == 0 {
		return nil
	}
	m := &secretMatcher{by4: map[uint32][]int{}, pats: pats}
	for i, p := range pats {
		if len(p) < 4 {
			continue
		}
		m.first[uint16(p[0])|uint16(p[1])<<8] = true
		k := uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
		m.by4[k] = append(m.by4[k], i)
	}
	return m
}

// find is the index of a string found in b, or -1.
func (m *secretMatcher) find(b []byte) int {
	for i := 0; i+snapMinSecret <= len(b); i++ {
		if !m.first[uint16(b[i])|uint16(b[i+1])<<8] {
			continue
		}
		k := uint32(b[i]) | uint32(b[i+1])<<8 | uint32(b[i+2])<<16 | uint32(b[i+3])<<24
		for _, p := range m.by4[k] {
			if bytes.HasPrefix(b[i:], m.pats[p]) {
				return p
			}
		}
	}
	return -1
}

// ErrSnapshotNotSanitised is what ReadSnapshotInfo says of a database no
// sanitiser made.
var ErrSnapshotNotSanitised = errors.New("not a sanitised snapshot")

// ReadSnapshotInfo is a snapshot's snapshot_info, as key -> value.
func ReadSnapshotInfo(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM ` + SnapshotInfoTable)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, ErrSnapshotNotSanitised
		}
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
