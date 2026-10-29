package gmweb

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The generic editor: any row of any table, read, added, changed or removed,
// each change audited (audit.go) and applied to the running game where the
// game holds the data (live.go). Identifiers are only ever names read from the
// schema; values are always bound parameters.

// Filter is one condition on a column.
type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  any    `json:"value,omitempty"`
}

// Query is a page of a table's rows.
type Query struct {
	Table   string   `json:"table"`
	Search  string   `json:"search,omitempty"`
	Filters []Filter `json:"filters,omitempty"`
	Sort    string   `json:"sort,omitempty"`
	Desc    bool     `json:"desc,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	Offset  int      `json:"offset,omitempty"`
	Columns []string `json:"columns,omitempty"`
}

const (
	defaultLimit = 50
	maxLimit     = 500
)

// dbtx is what both the database and a transaction answer: the editor runs
// its reads and writes for one change in one transaction.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// errConflict is a change refused because the row moved since it was read.
type errConflict struct {
	current map[string]any
	cols    []string
}

func (e *errConflict) Error() string {
	return fmt.Sprintf("the row changed since it was read (%s); read it again and redo the change", strings.Join(e.cols, ", "))
}

// toSQL is a JSON value as the column stores it: an INTEGER column takes a
// number, a numeric string or hex ("0x1A2B"), a REAL one a number, a TEXT one
// anything as text.
func toSQL(c *Column, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	if n, ok := v.(json.Number); ok {
		v = string(n)
		if c.Affinity == "TEXT" {
			return string(n), nil
		}
	}
	switch c.Affinity {
	case "INTEGER":
		switch x := v.(type) {
		case bool:
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		case float64:
			if x != math.Trunc(x) {
				return nil, fmt.Errorf("%s takes a whole number, not %v", c.Name, x)
			}
			return int64(x), nil
		case int:
			return int64(x), nil
		case int64:
			return x, nil
		case string:
			return parseInt(c.Name, x)
		}
	case "REAL":
		switch x := v.(type) {
		case bool:
			if x {
				return 1.0, nil
			}
			return 0.0, nil
		case float64:
			return x, nil
		case int:
			return float64(x), nil
		case int64:
			return float64(x), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, fmt.Errorf("%s takes a number, not %q", c.Name, x)
			}
			return f, nil
		}
	case "TEXT":
		switch x := v.(type) {
		case string:
			return x, nil
		case bool:
			if x {
				return "1", nil
			}
			return "0", nil
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64), nil
		case int, int64:
			return fmt.Sprint(x), nil
		}
	case "NUMERIC":
		switch x := v.(type) {
		case string:
			t := strings.TrimSpace(x)
			if i, err := strconv.ParseInt(t, 10, 64); err == nil {
				return i, nil
			}
			if f, err := strconv.ParseFloat(t, 64); err == nil && !strings.ContainsAny(t, "-:") {
				return f, nil
			}
			return x, nil
		case bool:
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		case float64, int, int64:
			return x, nil
		}
	case "BLOB":
		if x, ok := v.(string); ok {
			return x, nil
		}
	}
	return nil, fmt.Errorf("%s (%s) cannot take %T", c.Name, c.Affinity, v)
}

// parseInt reads a whole number as typed: decimal, or hex with 0x (which may
// run to 0xFFFFFFFFFFFFFFFF, stored as its two's complement).
func parseInt(col, s string) (int64, error) {
	t := strings.TrimSpace(strings.ReplaceAll(s, "_", ""))
	neg := strings.HasPrefix(t, "-")
	body := strings.TrimPrefix(t, "-")
	if strings.HasPrefix(body, "0x") || strings.HasPrefix(body, "0X") {
		u, err := strconv.ParseUint(body[2:], 16, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %q is not a hex number", col, s)
		}
		if neg {
			return -int64(u), nil
		}
		return int64(u), nil
	}
	if i, err := strconv.ParseInt(t, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<62 {
		return int64(f), nil
	}
	return 0, fmt.Errorf("%s takes a whole number, not %q", col, s)
}

// fromSQL is a stored value as JSON carries it: an integer past what a
// JavaScript number holds exactly becomes a string, bytes become text (or
// base64 when they are not UTF-8), a time SQLite's own text form.
func fromSQL(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case int64:
		if x > 1<<53 || x < -(1<<53) {
			return strconv.FormatInt(x, 10)
		}
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Sprint(x)
		}
		return x
	case []byte:
		if utf8.Valid(x) {
			return string(x)
		}
		return "base64:" + base64.StdEncoding.EncodeToString(x)
	case time.Time:
		return x.UTC().Format("2006-01-02 15:04:05")
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	}
	return v
}

// sameValue is whether two values (as fromSQL gives them, or as a client sent
// them back) are equal: numbers by value, text by text.
func sameValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	fa, aNum := asFloat(a)
	fb, bNum := asFloat(b)
	if aNum && bNum {
		return fa == fb
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case float64:
		return x, true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// selectList is the columns a read returns (a secret one never), with the
// rowid first for a table keyed by it.
func selectList(t *Table, only []string) (string, []string, error) {
	var names, cols []string
	if t.RowID {
		names = append(names, `rowid AS "__rowid"`)
		cols = append(cols, "__rowid")
	}
	want := map[string]bool{}
	for _, c := range only {
		if t.col(c) == nil {
			return "", nil, fmt.Errorf("%s has no column %q", t.Name, c)
		}
		want[c] = true
	}
	for _, k := range t.Key { // the key is always read: rows are named by it
		want[k] = true
	}
	for _, c := range t.Columns {
		if c.Secret || (len(only) > 0 && !want[c.Name]) {
			continue
		}
		names = append(names, quoteIdent(c.Name))
		cols = append(cols, c.Name)
	}
	return strings.Join(names, ", "), cols, nil
}

// scanRows reads every row of a result as column -> value, with the row's key
// under "__key".
func scanRows(t *Table, rs *sql.Rows, cols []string) ([]map[string]any, error) {
	defer rs.Close()
	var out []map[string]any
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols)+1)
		for i, c := range cols {
			row[c] = fromSQL(vals[i])
		}
		key := map[string]any{}
		if t.RowID {
			key["rowid"] = row["__rowid"]
			delete(row, "__rowid")
		} else {
			for _, k := range t.Key {
				key[k] = row[k]
			}
		}
		row["__key"] = key
		out = append(out, row)
	}
	return out, rs.Err()
}

// keyWhere is the WHERE clause naming one row by its key.
func keyWhere(t *Table, key map[string]any) (string, []any, error) {
	if len(key) == 0 {
		return "", nil, errors.New("a key is required: {\"" + strings.Join(t.Key, "\": …, \"") + "\": …}")
	}
	if t.RowID {
		v, ok := key["rowid"]
		if !ok {
			return "", nil, fmt.Errorf("%s has no key of its own: name the row by {\"rowid\": n}", t.Name)
		}
		id, err := toSQL(&Column{Name: "rowid", Affinity: "INTEGER"}, v)
		if err != nil {
			return "", nil, err
		}
		return "rowid = ?", []any{id}, nil
	}
	var parts []string
	var args []any
	for _, k := range t.Key {
		v, ok := key[k]
		if !ok {
			return "", nil, fmt.Errorf("the key of %s is %s; %q is missing", t.Name, strings.Join(t.Key, " + "), k)
		}
		sv, err := toSQL(t.col(k), v)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, quoteIdent(k)+" = ?")
		args = append(args, sv)
	}
	return strings.Join(parts, " AND "), args, nil
}

// where builds a query's WHERE clause from its search text and filters.
func where(t *Table, q Query) (string, []any, error) {
	var parts []string
	var args []any
	if s := strings.TrimSpace(q.Search); s != "" {
		var or []string
		for _, c := range t.Search {
			or = append(or, quoteIdent(c)+" LIKE ? ESCAPE '\\'")
			args = append(args, "%"+likeEscape(s)+"%")
		}
		// a number also finds the row with that key
		if len(t.Key) == 1 {
			kc := &Column{Name: t.Key[0], Affinity: "INTEGER"}
			if c := t.col(t.Key[0]); c != nil {
				kc = c
			}
			if kc.Affinity == "INTEGER" {
				if n, err := parseInt(kc.Name, s); err == nil {
					name := quoteIdent(t.Key[0])
					if t.RowID {
						name = "rowid"
					}
					or = append(or, name+" = ?")
					args = append(args, n)
				}
			}
		}
		if len(or) == 0 {
			return "", nil, fmt.Errorf("%s has no text to search; use filters", t.Name)
		}
		parts = append(parts, "("+strings.Join(or, " OR ")+")")
	}
	for _, f := range q.Filters {
		var c *Column
		name := ""
		if f.Column == "rowid" && t.RowID {
			c, name = &Column{Name: "rowid", Affinity: "INTEGER"}, "rowid"
		} else if c = t.col(f.Column); c == nil || c.Secret {
			return "", nil, fmt.Errorf("%s has no column %q", t.Name, f.Column)
		} else {
			name = quoteIdent(c.Name)
		}
		op := strings.ToLower(strings.TrimSpace(f.Op))
		switch op {
		case "", "=", "==", "eq":
			if f.Value == nil {
				parts = append(parts, name+" IS NULL")
				continue
			}
			v, err := toSQL(c, f.Value)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, name+" = ?")
			args = append(args, v)
		case "!=", "<>", "ne":
			if f.Value == nil {
				parts = append(parts, name+" IS NOT NULL")
				continue
			}
			v, err := toSQL(c, f.Value)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, "("+name+" != ? OR "+name+" IS NULL)")
			args = append(args, v)
		case "<", "<=", ">", ">=", "lt", "le", "gt", "ge":
			sym := map[string]string{"lt": "<", "le": "<=", "gt": ">", "ge": ">="}[op]
			if sym == "" {
				sym = op
			}
			v, err := toSQL(c, f.Value)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, name+" "+sym+" ?")
			args = append(args, v)
		case "like", "contains":
			parts = append(parts, name+" LIKE ? ESCAPE '\\'")
			args = append(args, "%"+likeEscape(fmt.Sprint(f.Value))+"%")
		case "prefix", "starts":
			parts = append(parts, name+" LIKE ? ESCAPE '\\'")
			args = append(args, likeEscape(fmt.Sprint(f.Value))+"%")
		case "in", "notin":
			list, ok := f.Value.([]any)
			if !ok || len(list) == 0 || len(list) > 1000 {
				return "", nil, fmt.Errorf("%q takes a list of 1 to 1000 values", op)
			}
			marks := make([]string, len(list))
			for i, x := range list {
				v, err := toSQL(c, x)
				if err != nil {
					return "", nil, err
				}
				marks[i] = "?"
				args = append(args, v)
			}
			not := ""
			if op == "notin" {
				not = "NOT "
			}
			parts = append(parts, name+" "+not+"IN ("+strings.Join(marks, ", ")+")")
		case "null":
			parts = append(parts, name+" IS NULL")
		case "notnull":
			parts = append(parts, name+" IS NOT NULL")
		case "bits", "hasbits":
			v, err := toSQL(&Column{Name: c.Name, Affinity: "INTEGER"}, f.Value)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, "("+name+" & ?) = ?")
			args = append(args, v, v)
		default:
			return "", nil, fmt.Errorf("unknown op %q (=, !=, <, <=, >, >=, like, prefix, in, notin, null, notnull, bits)", f.Op)
		}
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// orderBy is a query's ORDER BY: its column, else the table's own order,
// then the key so pages do not shuffle.
func orderBy(t *Table, q Query) (string, error) {
	keyOrder := "rowid"
	if !t.RowID {
		var ks []string
		for _, k := range t.Key {
			ks = append(ks, quoteIdent(k))
		}
		keyOrder = strings.Join(ks, ", ")
	}
	col := q.Sort
	if col == "" {
		col = t.Sort
	}
	if col == "" || col == "rowid" {
		if q.Desc {
			return " ORDER BY " + strings.ReplaceAll(keyOrder, ",", " DESC,") + " DESC", nil
		}
		return " ORDER BY " + keyOrder, nil
	}
	c := t.col(col)
	if c == nil || c.Secret {
		return "", fmt.Errorf("%s has no column %q to sort by", t.Name, col)
	}
	dir := " ASC"
	if q.Desc {
		dir = " DESC"
	}
	return " ORDER BY " + quoteIdent(c.Name) + dir + ", " + keyOrder, nil
}

// queryRows is a page of rows and how many there are in all.
func (s *Server) queryRows(t *Table, q Query) ([]map[string]any, int64, error) {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	if q.Limit > maxLimit {
		q.Limit = maxLimit
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	sel, cols, err := selectList(t, q.Columns)
	if err != nil {
		return nil, 0, err
	}
	w, args, err := where(t, q)
	if err != nil {
		return nil, 0, err
	}
	ob, err := orderBy(t, q)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM `+quoteIdent(t.Name)+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rs, err := s.d.DB.Query(`SELECT `+sel+` FROM `+quoteIdent(t.Name)+w+ob+` LIMIT ? OFFSET ?`,
		append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	rows, err := scanRows(t, rs, cols)
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, total, err
}

// getRow is one row by its key, through db (or a transaction).
func getRow(db dbtx, t *Table, key map[string]any) (map[string]any, error) {
	sel, cols, err := selectList(t, nil)
	if err != nil {
		return nil, err
	}
	w, args, err := keyWhere(t, key)
	if err != nil {
		return nil, err
	}
	rs, err := db.Query(`SELECT `+sel+` FROM `+quoteIdent(t.Name)+` WHERE `+w, args...)
	if err != nil {
		return nil, err
	}
	rows, err := scanRows(t, rs, cols)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no row of %s with key %s", t.Name, keyText(key))
	}
	return rows[0], nil
}

func keyText(key map[string]any) string {
	b, _ := json.Marshal(key)
	return string(b)
}

// mayWrite says whether the actor may change the table's rows.
func mayWrite(a *actor, t *Table) error {
	switch t.Write {
	case writeNone:
		return fmt.Errorf("%s is not changed through the editor", t.Name)
	case writeAdmin:
		if !a.may(scopeAdmin) {
			return fmt.Errorf("%s is an admin's to change", t.Name)
		}
	default:
		if !a.may(scopeWrite) {
			return errors.New("this credential is read-only")
		}
	}
	return nil
}

// writeOpts are how a change is made: Dry computes it and writes nothing;
// Raw (revert's) may write the columns the editor keeps read-only, since it
// only ever puts back what the audit log says was there.
type writeOpts struct {
	Dry bool
	Raw bool
	// NoLive leaves the running game alone: a caller writing many rows of one
	// table applies it once after them (quest_save, quest_set_enabled).
	NoLive bool
}

// writableValues checks and converts the columns a change sets.
func writableValues(t *Table, values map[string]any, inserting, raw bool) (map[string]any, error) {
	if len(values) == 0 {
		return nil, errors.New("no values given")
	}
	out := map[string]any{}
	for name, v := range values {
		c := t.col(name)
		if c == nil {
			return nil, fmt.Errorf("%s has no column %q", t.Name, name)
		}
		if c.Secret {
			return nil, fmt.Errorf("%s.%s is not written through the editor", t.Name, name)
		}
		if c.ReadOnly && !raw && !(inserting && c.PK > 0) {
			return nil, fmt.Errorf("%s.%s is read-only here%s", t.Name, name, readOnlyHint(t.Name, name))
		}
		sv, err := toSQL(c, v)
		if err != nil {
			return nil, err
		}
		if sv == nil && c.NotNull {
			return nil, fmt.Errorf("%s.%s may not be empty", t.Name, name)
		}
		out[name] = sv
	}
	return out, nil
}

func readOnlyHint(table, col string) string {
	switch table + "." + col {
	case "accountinfo.acctlevel":
		return " (set_account_level changes it)"
	case "accountinfo.pwhash":
		return " (set_account_password changes it)"
	}
	return ""
}

// change is one row change as it went (or, dry, would go).
type change struct {
	Table  string         `json:"table"`
	Action string         `json:"action"`
	Key    map[string]any `json:"key"`
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
	Audit  int64          `json:"audit,omitempty"`
	Live   string         `json:"live,omitempty"`
	DryRun bool           `json:"dryRun,omitempty"`
}

// guardAccountRow refuses a change to an account at or above the actor's own
// level, but the actor's own: a GM cannot ban or rename an admin.
func (s *Server) guardAccountRow(a *actor, t *Table, row map[string]any) error {
	if t.Name != "accountinfo" || row == nil {
		return nil
	}
	id, _ := asFloat(row["accountid"])
	lvl, _ := asFloat(row["acctlevel"])
	if int(id) != a.AccountID && int(lvl) >= a.Level {
		return forbidden("account %v is at or above your level; only it or a higher one may change it", row["username"])
	}
	return nil
}

// guardLevel holds a change of an account's acctlevel, however it comes (a
// row edit, set_account_level, a revert), to set_account_level's rules: an
// admin's, never one's own, never an account at or above one's level, never
// above one's level.
func (s *Server) guardLevel(a *actor, before map[string]any, newLevel any) error {
	nl, ok := asFloat(newLevel)
	if !ok {
		return badRequest("acctlevel must be a number")
	}
	id, _ := asFloat(before["accountid"])
	ol, _ := asFloat(before["acctlevel"])
	switch {
	case !a.may(scopeAdmin):
		return forbidden("an account's level is an admin's to change")
	case int(id) == a.AccountID:
		return forbidden("not your own level")
	case int(ol) >= a.Level:
		return forbidden("account %v is at or above your level", before["username"])
	case nl < 0 || int(nl) > a.Level:
		return forbidden("level is 0 to your own (%d)", a.Level)
	}
	return nil
}

// ownerColumn is the column naming the character a row belongs to (an
// item's, a spell's, a hotkey's), or "".
func ownerColumn(t *Table) string {
	for _, c := range t.Columns {
		if c.Ref == "characters" {
			switch strings.ToLower(c.Name) {
			case "serverid", "player_id", "charid":
				return c.Name
			}
		}
	}
	return ""
}

// guardOwner refuses a change to a character, or to a row of one (its items,
// spells, hotkeys...), whose account is at or above the actor's level, but
// the actor's own. db is the change's own transaction when there is one: the
// database has one connection.
func (s *Server) guardOwner(db dbtx, a *actor, t *Table, row map[string]any) error {
	if row == nil {
		return nil
	}
	var accountID int64
	if t.Name == "characters" {
		v, ok := asFloat(row["accountid"])
		if !ok {
			return nil
		}
		accountID = int64(v)
	} else {
		col := ownerColumn(t)
		if col == "" {
			return nil
		}
		v, ok := asFloat(row[col])
		if !ok {
			return nil
		}
		if err := db.QueryRow(`SELECT accountid FROM characters WHERE serverid = ?`, int64(v)).Scan(&accountID); err != nil {
			return nil // no such character: nobody's
		}
	}
	return s.guardAccountOf(db, a, accountID)
}

// guardAccountOf refuses work on what belongs to an account at or above the
// actor's level, but the actor's own.
func (s *Server) guardAccountOf(db dbtx, a *actor, accountID int64) error {
	if int(accountID) == a.AccountID {
		return nil
	}
	var lvl int
	var name string
	if err := db.QueryRow(`SELECT COALESCE(acctlevel, 0), username FROM accountinfo WHERE accountid = ?`, accountID).Scan(&lvl, &name); err != nil {
		return nil
	}
	if lvl >= a.Level {
		return forbidden("that belongs to account %s, at or above your level; only it or a higher one may change it", name)
	}
	return nil
}

// guardCharacter is guardOwner for a character by its serverid.
func (s *Server) guardCharacter(db dbtx, a *actor, serverID int64) error {
	var accountID int64
	if err := db.QueryRow(`SELECT accountid FROM characters WHERE serverid = ?`, serverID).Scan(&accountID); err != nil {
		return nil
	}
	return s.guardAccountOf(db, a, accountID)
}

// insertRow adds a row.
func (s *Server) insertRow(a *actor, t *Table, values map[string]any, o writeOpts) (*change, error) {
	if err := mayWrite(a, t); err != nil {
		return nil, err
	}
	vals, err := writableValues(t, values, true, o.Raw)
	if err != nil {
		return nil, err
	}
	if t.Name == "accountinfo" {
		return nil, errors.New("accounts are made with create_account (it hashes the password)")
	}
	if err := checkRow(t.Name, vals); err != nil {
		return nil, err
	}
	ch := &change{Table: t.Name, Action: "insert", DryRun: o.Dry}
	if o.Dry {
		if err := s.guardOwner(s.d.DB, a, t, vals); err != nil {
			return nil, err
		}
		ch.After = vals
		return ch, nil
	}
	tx, err := s.d.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.guardOwner(tx, a, t, vals); err != nil {
		return nil, err
	}
	var names, marks []string
	var args []any
	for _, c := range t.Columns { // in the table's order, for a stable statement
		if v, ok := vals[c.Name]; ok {
			names = append(names, quoteIdent(c.Name))
			marks = append(marks, "?")
			args = append(args, v)
		}
	}
	res, err := tx.Exec(`INSERT INTO `+quoteIdent(t.Name)+` (`+strings.Join(names, ", ")+`) VALUES (`+
		strings.Join(marks, ", ")+`)`, args...)
	if err != nil {
		return nil, sqlError(err)
	}
	key := map[string]any{}
	switch {
	case t.RowID:
		id, _ := res.LastInsertId()
		key["rowid"] = id
	case len(t.Key) == 1 && t.col(t.Key[0]).Affinity == "INTEGER" && vals[t.Key[0]] == nil:
		id, _ := res.LastInsertId()
		key[t.Key[0]] = id
	default:
		for _, k := range t.Key {
			key[k] = fromSQL(vals[k])
		}
	}
	after, err := getRow(tx, t, key)
	if err != nil {
		return nil, err
	}
	delete(after, "__key")
	ch.Key, ch.After = key, after
	if ch.Audit, err = s.auditTx(tx, a, "insert", t.Name, key, nil, after, ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.schema.bumpCount(t.Name, 1)
	if !o.NoLive {
		ch.Live = s.applyLive(a, t, "insert", nil, after)
	}
	return ch, nil
}

// updateRow changes some columns of one row. base, when given, is the row as
// the client last read it: a column being changed that has moved since is a
// conflict, and nothing is written.
func (s *Server) updateRow(a *actor, t *Table, key, values, base map[string]any, o writeOpts) (*change, error) {
	if err := mayWrite(a, t); err != nil {
		return nil, err
	}
	vals, err := writableValues(t, values, false, o.Raw)
	if err != nil {
		return nil, err
	}
	tx, err := s.d.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	before, err := getRow(tx, t, key)
	if err != nil {
		return nil, err
	}
	delete(before, "__key")
	if err := s.guardAccountRow(a, t, before); err != nil {
		return nil, err
	}
	if err := s.guardOwner(tx, a, t, before); err != nil {
		return nil, err
	}
	if base != nil {
		var moved []string
		for name := range vals {
			if b, ok := base[name]; ok && !sameValue(b, before[name]) {
				moved = append(moved, name)
			}
		}
		if len(moved) > 0 {
			return nil, &errConflict{current: before, cols: moved}
		}
	}
	// only what changes
	set := map[string]any{}
	for name, v := range vals {
		if !sameValue(fromSQL(v), before[name]) {
			set[name] = v
		}
	}
	ch := &change{Table: t.Name, Action: "update", Key: key, Before: before, DryRun: o.Dry}
	after := map[string]any{}
	for k, v := range before {
		after[k] = v
	}
	for k, v := range set {
		after[k] = fromSQL(v)
	}
	ch.After = after
	if len(set) == 0 {
		ch.Action = "unchanged"
		return ch, nil
	}
	if err := checkRow(t.Name, after); err != nil {
		return nil, err
	}
	if v, ok := set["acctlevel"]; ok && t.Name == "accountinfo" {
		if err := s.guardLevel(a, before, fromSQL(v)); err != nil {
			return nil, err
		}
	}
	// and where it lands: a character moved to an admin's account, an item
	// to an admin's character
	if err := s.guardOwner(tx, a, t, after); err != nil {
		return nil, err
	}
	if o.Dry {
		return ch, nil
	}
	w, wargs, err := keyWhere(t, key)
	if err != nil {
		return nil, err
	}
	var sets []string
	var args []any
	for _, c := range t.Columns {
		if v, ok := set[c.Name]; ok {
			sets = append(sets, quoteIdent(c.Name)+" = ?")
			args = append(args, v)
		}
	}
	res, err := tx.Exec(`UPDATE `+quoteIdent(t.Name)+` SET `+strings.Join(sets, ", ")+` WHERE `+w, append(args, wargs...)...)
	if err != nil {
		return nil, sqlError(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("%d rows changed, not 1", n)
	}
	// the key itself may have changed
	newKey := map[string]any{}
	for k, v := range key {
		newKey[k] = v
	}
	if !t.RowID {
		for _, k := range t.Key {
			if v, ok := set[k]; ok {
				newKey[k] = fromSQL(v)
			}
		}
	}
	after, err = getRow(tx, t, newKey)
	if err != nil {
		return nil, err
	}
	delete(after, "__key")
	ch.Key, ch.After = newKey, after
	if ch.Audit, err = s.auditTx(tx, a, "update", t.Name, key, before, after, ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if !o.NoLive {
		ch.Live = s.applyLive(a, t, "update", before, after)
	}
	return ch, nil
}

// deleteRow removes one row.
func (s *Server) deleteRow(a *actor, t *Table, key map[string]any, o writeOpts) (*change, error) {
	if err := mayWrite(a, t); err != nil {
		return nil, err
	}
	if t.Name == "accountinfo" && !a.may(scopeAdmin) {
		return nil, errors.New("deleting an account is an admin's (ban it instead)")
	}
	tx, err := s.d.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	before, err := getRow(tx, t, key)
	if err != nil {
		return nil, err
	}
	delete(before, "__key")
	if err := s.guardAccountRow(a, t, before); err != nil {
		return nil, err
	}
	if err := s.guardOwner(tx, a, t, before); err != nil {
		return nil, err
	}
	ch := &change{Table: t.Name, Action: "delete", Key: key, Before: before, DryRun: o.Dry}
	if o.Dry {
		return ch, nil
	}
	w, args, err := keyWhere(t, key)
	if err != nil {
		return nil, err
	}
	res, err := tx.Exec(`DELETE FROM `+quoteIdent(t.Name)+` WHERE `+w, args...)
	if err != nil {
		return nil, sqlError(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, fmt.Errorf("%d rows deleted, not 1", n)
	}
	if ch.Audit, err = s.auditTx(tx, a, "delete", t.Name, key, before, nil, ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.schema.bumpCount(t.Name, -1)
	if !o.NoLive {
		ch.Live = s.applyLive(a, t, "delete", before, nil)
	}
	return ch, nil
}

// matchingKeys is the keys of the rows a query's filters match, at most max+1
// of them (so a caller can tell "more than max").
func (s *Server) matchingKeys(t *Table, q Query, max int) ([]map[string]any, int64, error) {
	if len(q.Filters) == 0 && strings.TrimSpace(q.Search) == "" {
		return nil, 0, errors.New("a bulk change needs filters (or search text) saying which rows")
	}
	w, args, err := where(t, q)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM `+quoteIdent(t.Name)+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	keyCols := "rowid"
	if !t.RowID {
		var ks []string
		for _, k := range t.Key {
			ks = append(ks, quoteIdent(k))
		}
		keyCols = strings.Join(ks, ", ")
	}
	rs, err := s.d.DB.Query(`SELECT `+keyCols+` FROM `+quoteIdent(t.Name)+w+` ORDER BY `+keyCols+` LIMIT ?`, append(args, max+1)...)
	if err != nil {
		return nil, 0, err
	}
	defer rs.Close()
	var keys []map[string]any
	for rs.Next() {
		vals := make([]any, len(t.Key))
		ptrs := make([]any, len(t.Key))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, 0, err
		}
		k := map[string]any{}
		for i, name := range t.Key {
			k[name] = fromSQL(vals[i])
		}
		keys = append(keys, k)
	}
	return keys, total, rs.Err()
}

// sqlError words a database refusal for a person.
func sqlError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"):
		return fmt.Errorf("that value is taken already (%s)", strings.TrimSpace(msg[strings.Index(msg, "UNIQUE"):]))
	case strings.Contains(msg, "NOT NULL constraint failed"):
		return fmt.Errorf("a required column is missing (%s)", strings.TrimSpace(msg[strings.Index(msg, "NOT NULL"):]))
	}
	return err
}

func (c *schemaCache) bumpCount(table string, d int64) {
	c.mu.Lock()
	if t, ok := c.tables[table]; ok {
		t.Count += d
	}
	c.mu.Unlock()
}
