package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
	"unicode/utf8"
)

// GMAuditSchema is the GM panel's audit log (internal/gmweb): every change a
// GM makes through the panel, an AI agent or a command in the game, by whom
// and through what, and for a row change the row before and after, which is
// what the panel's revert puts back. The panel creates it with its other
// tables; a GM command in the game that changes a row creates it too, so the
// change is logged whether or not the panel runs.
const GMAuditSchema = `
CREATE TABLE IF NOT EXISTS gm_audit (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	at          TEXT NOT NULL,
	accountid   INTEGER NOT NULL,
	username    TEXT NOT NULL,
	via         TEXT NOT NULL,
	ip          TEXT NOT NULL,
	action      TEXT NOT NULL,
	tbl         TEXT NOT NULL DEFAULT '',
	rowkey      TEXT NOT NULL DEFAULT '',
	before      TEXT,
	after       TEXT,
	detail      TEXT NOT NULL DEFAULT '',
	reverted_by INTEGER
);
CREATE INDEX IF NOT EXISTS idx_gm_audit_tbl ON gm_audit(tbl, rowkey);
CREATE INDEX IF NOT EXISTS idx_gm_audit_acct ON gm_audit(accountid);
`

// GMEdit is who changes a row from the game, as the audit log names a GM:
// the account, its name, through what ("game": a command in the chat line),
// and from where.
type GMEdit struct {
	AccountID int
	Username  string
	Via       string
	IP        string
}

// auditRowChange writes a row change's line inside the change's own
// transaction, as the panel's auditTx does: the key and the rows as JSON, the
// time in RFC 3339 (UTC). It returns the line's id.
func auditRowChange(tx *sql.Tx, who GMEdit, action, table string, key, before, after map[string]any, detail string) (int64, error) {
	k, err := json.Marshal(key)
	if err != nil {
		return 0, err
	}
	rowJSON := func(m map[string]any) any {
		if m == nil {
			return nil
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil
		}
		return string(b)
	}
	res, err := tx.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), who.AccountID, who.Username, who.Via, who.IP,
		action, table, string(k), rowJSON(before), rowJSON(after), detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// fullRow is every column of the one row `query` selects (SELECT *), by
// name, its values as the panel reads them (gmweb's fromSQL: an integer
// beyond 2^53 as text, bytes as text when they are UTF-8), so a line written
// here reads, compares and reverts as one the panel wrote. nil and no error
// when there is no row.
func fullRow(tx *sql.Tx, query string, args ...any) (map[string]any, error) {
	rs, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	if !rs.Next() {
		return nil, rs.Err()
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rs.Scan(ptrs...); err != nil {
		return nil, err
	}
	row := make(map[string]any, len(cols))
	for i, c := range cols {
		row[c] = auditValue(vals[i])
	}
	return row, rs.Err()
}

// auditValue is a column's value as the panel keeps it in a row.
func auditValue(v any) any {
	switch x := v.(type) {
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
		return fmt.Sprintf("%x", x)
	}
	return v
}
