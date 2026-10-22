package gmweb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

// The audit log: every change through the panel or an agent, who made it and
// through what, and for a row change the row before and after, which is what
// revert puts back.

// auditEntry is one line of the log.
type auditEntry struct {
	ID         int64          `json:"id"`
	At         string         `json:"at"`
	AccountID  int            `json:"accountid"`
	Username   string         `json:"username"`
	Via        string         `json:"via"`
	IP         string         `json:"ip"`
	Action     string         `json:"action"`
	Table      string         `json:"table,omitempty"`
	Key        map[string]any `json:"key,omitempty"`
	Before     map[string]any `json:"before,omitempty"`
	After      map[string]any `json:"after,omitempty"`
	Detail     string         `json:"detail,omitempty"`
	RevertedBy int64          `json:"revertedBy,omitempty"`
}

func jsonOrNil(v map[string]any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

// auditTx writes a line inside the change's own transaction, so a change and
// its line are both kept or neither.
func (s *Server) auditTx(db dbtx, a *actor, action, table string, key, before, after map[string]any, detail string) (int64, error) {
	rowkey := ""
	if key != nil {
		rowkey = keyText(key)
	}
	res, err := db.Exec(`INSERT INTO gm_audit (at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now(), a.AccountID, a.Username, a.Via, a.IP, action, table, rowkey, jsonOrNil(before), jsonOrNil(after), detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// audit writes a line for something that is not a row change (a kick, a
// broadcast, a setting), and logs it.
func (s *Server) audit(a *actor, action, table string, key, before, after map[string]any, detail string) int64 {
	id, err := s.auditTx(s.d.DB, a, action, table, key, before, after, detail)
	if err != nil {
		log.Printf("GM audit: could not record %s by %s: %v", action, a.Username, err)
	}
	if detail != "" {
		log.Printf("GM %s (%s): %s", a.Username, a.Via, detail)
	}
	return id
}

// auditFilter narrows the log.
type auditFilter struct {
	Username string `json:"username,omitempty"`
	Table    string `json:"table,omitempty"`
	Action   string `json:"action,omitempty"`
	Key      string `json:"key,omitempty"`
	Search   string `json:"search,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`

	// hideSQL keeps an admin's SQL from someone who is not one: the entries
	// show, their statements do not, and a search never matches them (SQL
	// can carry a password hash).
	hideSQL bool
}

func scanAudit(rs *sql.Rows) ([]auditEntry, error) {
	defer rs.Close()
	var out []auditEntry
	for rs.Next() {
		var e auditEntry
		var rowkey string
		var before, after sql.NullString
		var reverted sql.NullInt64
		if err := rs.Scan(&e.ID, &e.At, &e.AccountID, &e.Username, &e.Via, &e.IP, &e.Action, &e.Table,
			&rowkey, &before, &after, &e.Detail, &reverted); err != nil {
			return nil, err
		}
		if rowkey != "" {
			json.Unmarshal([]byte(rowkey), &e.Key)
		}
		if before.Valid {
			json.Unmarshal([]byte(before.String), &e.Before)
		}
		if after.Valid {
			json.Unmarshal([]byte(after.String), &e.After)
		}
		e.RevertedBy = reverted.Int64
		out = append(out, e)
	}
	return out, rs.Err()
}

const auditColumns = `id, at, accountid, username, via, ip, action, tbl, rowkey, before, after, detail, reverted_by`

// auditList is a page of the log, newest first.
func (s *Server) auditList(f auditFilter) ([]auditEntry, int64, error) {
	var parts []string
	var args []any
	if f.Username != "" {
		parts = append(parts, "LOWER(username) = LOWER(?)")
		args = append(args, f.Username)
	}
	if f.Table != "" {
		parts = append(parts, "tbl = ?")
		args = append(args, f.Table)
	}
	if f.Action != "" {
		parts = append(parts, "action = ?")
		args = append(args, f.Action)
	}
	if f.Key != "" {
		parts = append(parts, "rowkey = ?")
		args = append(args, f.Key)
	}
	if f.Search != "" {
		clause := "(detail LIKE ? ESCAPE '\\' OR before LIKE ? ESCAPE '\\' OR after LIKE ? ESCAPE '\\')"
		if f.hideSQL {
			clause = "(action != 'sql' AND " + clause + ")"
		}
		parts = append(parts, clause)
		p := "%" + likeEscape(f.Search) + "%"
		args = append(args, p, p, p)
	}
	w := ""
	if len(parts) > 0 {
		w = " WHERE " + strings.Join(parts, " AND ")
	}
	var total int64
	if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM gm_audit`+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	rs, err := s.d.DB.Query(`SELECT `+auditColumns+` FROM gm_audit`+w+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, f.Limit, max(0, f.Offset))...)
	if err != nil {
		return nil, 0, err
	}
	list, err := scanAudit(rs)
	if list == nil {
		list = []auditEntry{}
	}
	if f.hideSQL {
		for i := range list {
			if list[i].Action == "sql" {
				list[i].Detail = "an admin's SQL (shown to admins only)"
			}
		}
	}
	return list, total, err
}

// auditGet is one line.
func (s *Server) auditGet(id int64) (*auditEntry, error) {
	rs, err := s.d.DB.Query(`SELECT `+auditColumns+` FROM gm_audit WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	list, err := scanAudit(rs)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no audit entry %d", id)
	}
	return &list[0], nil
}

// revert undoes a row change from the log: an insert's row deleted, a
// delete's row put back, an update's changed columns set back to their
// before. The row must still be as the change left it (force: whatever it is
// now), and the undoing is itself a change, logged and revertible.
func (s *Server) revert(a *actor, id int64, force, dry bool) (*change, error) {
	e, err := s.auditGet(id)
	if err != nil {
		return nil, err
	}
	if e.RevertedBy != 0 {
		return nil, fmt.Errorf("entry %d was reverted already, by entry %d", id, e.RevertedBy)
	}
	if e.Table == "" || e.Key == nil {
		return nil, fmt.Errorf("entry %d (%s) is not a row change; it cannot be reverted", id, e.Action)
	}
	t, err := s.table(e.Table)
	if err != nil {
		return nil, err
	}
	if t.Name == "accountinfo" {
		// an account's level, status and name are guarded by who changes
		// them; a revert must not be a way round that
		if !a.may(scopeAdmin) {
			return nil, forbidden("reverting a change to an account is an admin's")
		}
		if e.Action == "delete" {
			return nil, badRequest("a deleted account is not put back by revert (its password was never logged): make it again with create_account")
		}
	}
	var ch *change
	switch e.Action {
	case "insert":
		if !force {
			if cur, err := getRow(s.d.DB, t, e.Key); err == nil {
				if moved := movedColumns(e.After, cur); len(moved) > 0 {
					return nil, &errConflict{current: cur, cols: moved}
				}
			}
		}
		ch, err = s.deleteRow(a, t, e.Key, writeOpts{Dry: dry, Raw: true})
	case "delete":
		vals := map[string]any{}
		for k, v := range e.Before {
			if c := t.col(k); c != nil && !c.Secret {
				vals[k] = v
			}
		}
		if t.RowID {
			return nil, fmt.Errorf("%s has no key of its own, so a deleted row cannot be put back under its old rowid; insert it again by hand", t.Name)
		}
		ch, err = s.insertRow(a, t, vals, writeOpts{Dry: dry, Raw: true})
	case "update":
		key := e.Key
		if e.After != nil && !t.RowID { // the update may have moved the key
			nk := map[string]any{}
			for _, k := range t.Key {
				nk[k] = e.After[k]
			}
			key = nk
		}
		if _, err2 := getRow(s.d.DB, t, key); err2 != nil {
			return nil, fmt.Errorf("the row is gone: %v", err2)
		}
		vals := map[string]any{}
		for k, v := range e.Before {
			if !sameValue(v, e.After[k]) {
				vals[k] = v
			}
		}
		if len(vals) == 0 {
			return nil, errors.New("that change changed nothing")
		}
		var base map[string]any
		if !force {
			base = map[string]any{}
			for k := range vals {
				base[k] = e.After[k]
			}
		}
		ch, err = s.updateRow(a, t, key, vals, base, writeOpts{Dry: dry, Raw: true})
	default:
		return nil, fmt.Errorf("entry %d (%s) cannot be reverted", id, e.Action)
	}
	if err != nil {
		return nil, err
	}
	if !dry && ch.Audit != 0 {
		s.d.DB.Exec(`UPDATE gm_audit SET reverted_by = ? WHERE id = ?`, ch.Audit, id)
		s.d.DB.Exec(`UPDATE gm_audit SET detail = ? WHERE id = ?`, fmt.Sprintf("reverts entry %d", id), ch.Audit)
	}
	return ch, nil
}

// movedColumns is which of want's columns cur no longer holds.
func movedColumns(want, cur map[string]any) []string {
	var moved []string
	for k, v := range want {
		if k == "__key" {
			continue
		}
		if !sameValue(v, cur[k]) {
			moved = append(moved, k)
		}
	}
	return moved
}
