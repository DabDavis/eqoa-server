package gmweb

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/handlers"
)

// The generic editor's operations: any table's rows, read and changed one at
// a time or by filter, every change audited and applied live where the game
// holds the data.

func (s *Server) registerRowOps() {
	queryProps := map[string]any{
		"table":   str("the table (list_tables)"),
		"search":  str("text any of the table's name-like columns contains (and a number also finds the row with that key)"),
		"filters": arr(filterSchema, "conditions, all of which must hold"),
		"sort":    str("the column to order by (default: the table's own)"),
		"desc":    boolean("order from the highest"),
		"limit":   integer(fmt.Sprintf("rows a page, default %d, at most %d", defaultLimit, maxLimit)),
		"offset":  integer("rows to skip, for the next page"),
		"columns": arr(str(""), "only these columns (the key always comes); default all"),
	}
	s.addOp(&Op{
		Name: "query_rows", Title: "Query rows", Scope: scopeRead, Core: true,
		Description: "Read a page of any table's rows, filtered, searched and sorted, with how many match in all. " +
			"Each row carries its key under \"__key\", which get_row, update_row and delete_row take.",
		Input: obj(queryProps, "table"),
		Handler: func(c *opCtx) (any, error) {
			var q Query
			if err := c.decode(&q); err != nil {
				return nil, err
			}
			t, err := c.s.table(q.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			rows, total, err := c.s.queryRows(t, q)
			if err != nil {
				return nil, badRequest("%v", err)
			}
			return map[string]any{"table": t.Name, "rows": rows, "total": total, "offset": max(0, q.Offset),
				"limit": len(rows)}, nil
		},
	})
	s.addOp(&Op{
		Name: "count_rows", Title: "Count rows", Scope: scopeRead,
		Description: "Count a table's rows that match, overall or grouped by up to three columns (\"how many NPCs per " +
			"world\", \"items by wear slot\"). The 200 largest groups come back.",
		Input: obj(map[string]any{
			"table":    str("the table"),
			"search":   str("as query_rows"),
			"filters":  arr(filterSchema, "as query_rows"),
			"group_by": arr(str(""), "up to three columns to count by"),
		}, "table"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Query
				GroupBy []string `json:"group_by"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			return c.s.countRows(t, req.Query, req.GroupBy)
		},
	})
	s.addOp(&Op{
		Name: "get_row", Title: "Get a row", Scope: scopeRead,
		Description: "One row of a table by its key.",
		Input:       obj(map[string]any{"table": str("the table"), "key": keySchema}, "table", "key"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table string
				Key   map[string]any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			row, err := getRow(c.s.d.DB, t, req.Key)
			if err != nil {
				return nil, notFound("%v", err)
			}
			return map[string]any{"table": t.Name, "row": row}, nil
		},
	})
	s.addOp(&Op{
		Name: "insert_row", Title: "Add a row", Scope: scopeWrite, Core: true,
		Description: "Add a row to a table: the columns to set (the rest take their defaults; an id column left out " +
			"is numbered by the database). Returns the row as stored and what it did in the running game (a new NPC " +
			"spawns at once). dry_run checks it and writes nothing. To copy an existing row with changes, use clone_row.",
		Input: obj(map[string]any{
			"table":   str("the table"),
			"values":  freeObject("column -> value; an INTEGER column also takes hex as \"0x1A2B\""),
			"dry_run": boolean("check and show, write nothing"),
		}, "table", "values"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table  string
				Values map[string]any
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			if t.Name == "characters" {
				return nil, badRequest("characters are made in the game or on the account page; create_account makes accounts")
			}
			return c.s.insertRow(c.a, t, req.Values, writeOpts{Dry: req.DryRun})
		},
	})
	s.addOp(&Op{
		Name: "clone_row", Title: "Copy a row", Scope: scopeWrite, Core: true,
		Description: "Copy an existing row into a new one with some columns changed: the way to add an NPC (copy one " +
			"that looks right, give it a name and a place), an item or a spell. The new row gets a new id. dry_run " +
			"shows it and writes nothing.",
		Input: obj(map[string]any{
			"table":   str("the table"),
			"key":     keySchema,
			"changes": freeObject("column -> value to differ from the original"),
			"dry_run": boolean("check and show, write nothing"),
		}, "table", "key"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table   string
				Key     map[string]any
				Changes map[string]any
				DryRun  bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			if t.Name == "accountinfo" || t.Name == "characters" {
				return nil, badRequest("%s rows are not copied", t.Name)
			}
			src, err := getRow(c.s.d.DB, t, req.Key)
			if err != nil {
				return nil, notFound("%v", err)
			}
			vals := map[string]any{}
			for _, col := range t.Columns {
				if col.Secret {
					continue
				}
				// an integer key is numbered anew
				if col.PK > 0 && len(t.Key) == 1 && col.Affinity == "INTEGER" {
					continue
				}
				if v, ok := src[col.Name]; ok {
					vals[col.Name] = v
				}
			}
			for k, v := range req.Changes {
				vals[k] = v
			}
			ch, err := c.s.insertRow(c.a, t, vals, writeOpts{Dry: req.DryRun, Raw: false})
			if err != nil {
				return nil, err
			}
			return map[string]any{"from": req.Key, "change": ch}, nil
		},
	})
	s.addOp(&Op{
		Name: "update_row", Title: "Change a row", Scope: scopeWrite, Core: true,
		Description: "Change some columns of one row. Give expect (the values you read) to refuse the change if the " +
			"row moved since. Returns the row before and after and what it did in the running game (an NPC changes " +
			"at once; an online character's level, tunar and place too). dry_run shows the change and writes nothing.",
		Input: obj(map[string]any{
			"table":   str("the table"),
			"key":     keySchema,
			"values":  freeObject("column -> new value"),
			"expect":  freeObject("column -> the value you read: the change is refused if any differs now"),
			"dry_run": boolean("check and show, write nothing"),
		}, "table", "key", "values"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table  string
				Key    map[string]any
				Values map[string]any
				Expect map[string]any
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			return c.s.updateRow(c.a, t, req.Key, req.Values, req.Expect, writeOpts{Dry: req.DryRun})
		},
	})
	s.addOp(&Op{
		Name: "delete_row", Title: "Delete a row", Scope: scopeWrite, Destructive: true, Core: true,
		Description: "Delete one row (an NPC leaves the world at once). A character goes with its items, spells, " +
			"effects and hotkeys. The audit log keeps the row, and revert_change puts it back. dry_run shows what " +
			"would go.",
		Input: obj(map[string]any{"table": str("the table"), "key": keySchema, "dry_run": boolean("show, delete nothing")},
			"table", "key"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table  string
				Key    map[string]any
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			if t.Name == "characters" {
				id, ok := asFloat(req.Key["serverid"])
				if !ok {
					return nil, badRequest(`a character's key is {"serverid": n}`)
				}
				return c.s.deleteCharacter(c.a, int64(id), req.DryRun)
			}
			return c.s.deleteRow(c.a, t, req.Key, writeOpts{Dry: req.DryRun})
		},
	})
	bulkProps := func(extra map[string]any) map[string]any {
		p := map[string]any{
			"table":   str("the table"),
			"search":  str("as query_rows"),
			"filters": arr(filterSchema, "which rows (required: a bulk change never takes a whole table by default)"),
			"max":     integer("refuse if more rows than this match (default 50, at most 1000)"),
			"dry_run": boolean("show which rows and how, change nothing"),
		}
		for k, v := range extra {
			p[k] = v
		}
		return p
	}
	s.addOp(&Op{
		Name: "update_rows", Title: "Change many rows", Scope: scopeWrite, Destructive: true,
		Description: "Set the same values on every row the filters match, each change audited (and revertible) on " +
			"its own and applied live. Refused when more rows match than max. Run it with dry_run first.",
		Input: obj(bulkProps(map[string]any{"values": freeObject("column -> new value")}), "table", "values"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Query
				Values map[string]any
				Max    int
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.bulk(c.a, req.Query, req.Max, req.DryRun, func(t *Table, key map[string]any, dry bool) (*change, error) {
				return c.s.updateRow(c.a, t, key, req.Values, nil, writeOpts{Dry: dry})
			})
		},
	})
	s.addOp(&Op{
		Name: "delete_rows", Title: "Delete many rows", Scope: scopeWrite, Destructive: true,
		Description: "Delete every row the filters match, each kept in the audit log (and revertible) on its own. " +
			"Refused when more rows match than max. Run it with dry_run first.",
		Input: obj(bulkProps(nil), "table"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Query
				Max    int
				DryRun bool `json:"dry_run"`
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			if req.Table == "characters" || req.Table == "accountinfo" {
				return nil, badRequest("%s are deleted one at a time", req.Table)
			}
			return c.s.bulk(c.a, req.Query, req.Max, req.DryRun, func(t *Table, key map[string]any, dry bool) (*change, error) {
				return c.s.deleteRow(c.a, t, key, writeOpts{Dry: dry})
			})
		},
	})
	s.addOp(&Op{
		Name: "lookup", Title: "Name rows by key", Scope: scopeRead,
		Description: "The names of some rows of a table by their keys (what an id in another table refers to): " +
			"{table: \"itempattern\", keys: [1, 2]} gives each item's name.",
		Input: obj(map[string]any{"table": str("the table"), "keys": arr(map[string]any{}, "single-column key values")},
			"table", "keys"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Table string
				Keys  []any
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			names, err := c.s.lookupNames(t, req.Keys)
			if err != nil {
				return nil, err
			}
			return map[string]any{"table": t.Name, "names": names}, nil
		},
	})
}

// countRows counts a query's rows, overall or grouped.
func (s *Server) countRows(t *Table, q Query, groupBy []string) (any, error) {
	w, args, err := where(t, q)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	if len(groupBy) == 0 {
		var n int64
		if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM `+quoteIdent(t.Name)+w, args...).Scan(&n); err != nil {
			return nil, err
		}
		return map[string]any{"table": t.Name, "count": n}, nil
	}
	if len(groupBy) > 3 {
		return nil, badRequest("group by three columns at most")
	}
	var cols []string
	for _, g := range groupBy {
		c := t.col(g)
		if c == nil || c.Secret {
			return nil, badRequest("%s has no column %q", t.Name, g)
		}
		cols = append(cols, quoteIdent(c.Name))
	}
	list := strings.Join(cols, ", ")
	groups, err := s.rowsOf(`SELECT `+list+`, COUNT(*) AS count FROM `+quoteIdent(t.Name)+w+
		` GROUP BY `+list+` ORDER BY count DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	var total int64
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM `+quoteIdent(t.Name)+w, args...).Scan(&total)
	return map[string]any{"table": t.Name, "groups": groups, "total": total}, nil
}

// bulk runs one change on every row a query matches, after checking how many.
func (s *Server) bulk(a *actor, q Query, limit int, dry bool, each func(*Table, map[string]any, bool) (*change, error)) (any, error) {
	t, err := s.table(q.Table)
	if err != nil {
		return nil, notFound("%v", err)
	}
	if err := mayWrite(a, t); err != nil {
		return nil, forbidden("%v", err)
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 1000)
	keys, total, err := s.matchingKeys(t, q, limit)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	if total > int64(limit) {
		return nil, badRequest("%d rows match, more than max (%d): narrow the filters or raise max", total, limit)
	}
	var changes []*change
	var failures []string
	for _, k := range keys {
		ch, err := each(t, k, dry)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", keyText(k), err))
			continue
		}
		changes = append(changes, ch)
	}
	out := map[string]any{"table": t.Name, "matched": total, "dryRun": dry, "done": len(changes)}
	if dry || len(changes) <= 20 {
		out["changes"] = changes
	} else {
		var lives []string
		seen := map[string]bool{}
		for _, ch := range changes {
			if ch.Live != "" && !seen[ch.Live] && len(lives) < 10 {
				seen[ch.Live] = true
				lives = append(lives, ch.Live)
			}
		}
		out["firstAudit"], out["lastAudit"] = changes[0].Audit, changes[len(changes)-1].Audit
		out["live"] = lives
	}
	if len(failures) > 0 {
		out["failures"] = failures
	}
	return out, nil
}

// lookupNames names rows of a table with a single-column key by their title
// column (the key itself when there is none).
func (s *Server) lookupNames(t *Table, keys []any) (map[string]string, error) {
	out := map[string]string{}
	if len(keys) == 0 {
		return out, nil
	}
	if len(keys) > 1000 {
		return nil, badRequest("at most 1000 keys")
	}
	if len(t.Key) != 1 || t.RowID {
		return nil, badRequest("%s has no single-column key", t.Name)
	}
	title := t.Title
	if title == "" {
		title = t.Key[0]
	}
	kc := t.col(t.Key[0])
	marks := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		if n, ok := k.(json.Number); ok {
			k = string(n)
		}
		v, err := toSQL(kc, k)
		if err != nil {
			return nil, badRequest("%v", err)
		}
		marks = append(marks, "?")
		args = append(args, v)
	}
	rows, err := s.rowsOf(`SELECT `+quoteIdent(kc.Name)+` AS k, `+quoteIdent(title)+` AS t FROM `+quoteIdent(t.Name)+
		` WHERE `+quoteIdent(kc.Name)+` IN (`+strings.Join(marks, ", ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[fmt.Sprint(r["k"])] = fmt.Sprint(r["t"])
	}
	return out, nil
}

// deleteCharacter deletes a character with everything that is its: items,
// spells, effects, standings, hotkeys, weapon sets, quests, coach ledgers and
// guild membership, each row audited (so revert_change can put any back), the
// character last. One in the world is ejected first.
func (s *Server) deleteCharacter(a *actor, id int64, dry bool) (any, error) {
	if !a.may(scopeWrite) {
		return nil, forbidden("this credential is read-only")
	}
	ct, err := s.table("characters")
	if err != nil {
		return nil, err
	}
	charRow, err := getRow(s.d.DB, ct, map[string]any{"serverid": id})
	if err != nil {
		return nil, notFound("%v", err)
	}
	acc, _ := asFloat(charRow["accountid"])
	if owner, err := s.accountByID(int(acc)); err == nil && owner.ID != a.AccountID && owner.Level >= a.Level {
		return nil, forbidden("that character's account is at or above your level")
	}
	deps := []struct{ table, col string }{
		{"charinventory", "serverid"}, {"spells", "serverid"}, {"charbuffs", "serverid"}, {"charfactions", "serverid"}, {"hotkeys", "serverid"},
		{"weaponhotbar", "serverid"}, {"quickslots", "serverid"}, {"gearsets", "serverid"},
		{"charquests", "serverid"}, {"charcoaches", "serverid"}, {"charresets", "serverid"}, {"guild_members", "player_id"},
	}
	plan := map[string]int64{}
	for _, d := range deps {
		if _, err := s.table(d.table); err != nil {
			continue
		}
		var n int64
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM `+quoteIdent(d.table)+` WHERE `+quoteIdent(d.col)+` = ?`, id).Scan(&n)
		if n > 0 {
			plan[d.table] = n
		}
	}
	if dry {
		return map[string]any{"dryRun": true, "character": charRow, "alsoDeleted": plan}, nil
	}
	if sess := s.sessionFor(uint32(id)); sess != nil {
		handlers.Eject(sess)
	}
	removed := map[string]int{}
	for _, d := range deps {
		if plan[d.table] == 0 {
			continue
		}
		t, err := s.table(d.table)
		if err != nil {
			continue
		}
		q := Query{Table: d.table, Filters: []Filter{{Column: d.col, Op: "=", Value: id}}}
		keys, _, err := s.matchingKeys(t, q, 100000)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if _, err := s.deleteRow(a, t, k, writeOpts{}); err != nil {
				return nil, fmt.Errorf("deleting %s %s: %w", d.table, keyText(k), err)
			}
			removed[d.table]++
		}
	}
	ch, err := s.deleteRow(a, ct, map[string]any{"serverid": id}, writeOpts{})
	if err != nil {
		return nil, err
	}
	return map[string]any{"change": ch, "alsoDeleted": removed}, nil
}
