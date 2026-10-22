package gmweb

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Every table, as SQLite describes it (sqlite_master and PRAGMA table_info),
// with what the catalog (catalog.go) knows about it laid over: so a table an
// importer added yesterday is editable today, and the ones the game reads come
// with their meaning.

// Column is one column of a table.
type Column struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Affinity string  `json:"affinity"`
	NotNull  bool    `json:"notnull,omitempty"`
	Default  *string `json:"default,omitempty"`
	PK       int     `json:"pk,omitempty"`

	Label    string `json:"label,omitempty"`
	Help     string `json:"help,omitempty"`
	Enum     string `json:"enum,omitempty"`
	Flags    string `json:"flags,omitempty"`
	Ref      string `json:"ref,omitempty"`
	Format   string `json:"format,omitempty"`
	Section  string `json:"section,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
	ReadOnly bool   `json:"readonly,omitempty"`
}

// Table is one table.
type Table struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Help    string   `json:"help,omitempty"`
	Columns []Column `json:"columns"`
	Key     []string `json:"key"`
	RowID   bool     `json:"rowid,omitempty"`
	Title   string   `json:"title,omitempty"`
	Show    []string `json:"show,omitempty"`
	Search  []string `json:"search,omitempty"`
	Sort    string   `json:"sort,omitempty"`
	Write   string   `json:"write"`
	Live    string   `json:"live,omitempty"`
	Count   int64    `json:"count"`
	Hidden  bool     `json:"-"`

	cols map[string]*Column
}

// Write levels: who may change a table's rows through the generic editor.
const (
	writeGM    = "gm"
	writeAdmin = "admin"
	writeNone  = "none"
)

// col is a column by name, or nil.
func (t *Table) col(name string) *Column {
	return t.cols[name]
}

// keyed is whether the table has a key of its own (else rows go by rowid).
func (t *Table) keyed() bool { return !t.RowID }

type schemaCache struct {
	mu      sync.Mutex
	tables  map[string]*Table
	loaded  time.Time
	counted time.Time
}

func (c *schemaCache) invalidate() {
	c.mu.Lock()
	c.tables = nil
	c.mu.Unlock()
}

// quoteIdent quotes a name SQLite holds. Only names read from the schema
// itself are ever quoted: nothing a client sends becomes an identifier
// without being matched against them first.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// affinity is SQLite's type affinity for a declared type (the five rules of
// "Datatypes In SQLite", section 3.1).
func affinity(decl string) string {
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

// tables is every table, read from the database once and kept until a
// statement could have changed the schema; with counts at most 10 s old.
func (s *Server) tables() (map[string]*Table, error) {
	c := s.schema
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tables == nil {
		ts, err := s.readSchema()
		if err != nil {
			return nil, err
		}
		c.tables, c.loaded, c.counted = ts, time.Now(), time.Time{}
	}
	if time.Since(c.counted) > 10*time.Second {
		for _, t := range c.tables {
			var n int64
			if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM ` + quoteIdent(t.Name)).Scan(&n); err == nil {
				t.Count = n
			}
		}
		c.counted = time.Now()
	}
	return c.tables, nil
}

// table is one table by name, if the database has it (and it is not hidden).
func (s *Server) table(name string) (*Table, error) {
	ts, err := s.tables()
	if err != nil {
		return nil, err
	}
	t, ok := ts[name]
	if !ok || t.Hidden {
		return nil, fmt.Errorf("no table %q (list_tables names them)", name)
	}
	return t, nil
}

// tableList is the visible tables, by group and label.
func (s *Server) tableList() ([]*Table, error) {
	ts, err := s.tables()
	if err != nil {
		return nil, err
	}
	var out []*Table
	for _, t := range ts {
		if !t.Hidden {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		gi, gj := groupOrder(out[i].Group), groupOrder(out[j].Group)
		if gi != gj {
			return gi < gj
		}
		return out[i].Label < out[j].Label
	})
	return out, nil
}

func (s *Server) readSchema() (map[string]*Table, error) {
	rows, err := s.d.DB.Query(`SELECT name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	type entry struct{ name, sql string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.name, &e.sql); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := map[string]*Table{}
	for _, e := range entries {
		t := &Table{Name: e.name, cols: map[string]*Column{}}
		cr, err := s.d.DB.Query(`PRAGMA table_info(` + quoteIdent(e.name) + `)`)
		if err != nil {
			return nil, err
		}
		var pks []Column
		for cr.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt *string
			if err := cr.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				cr.Close()
				return nil, err
			}
			col := Column{Name: name, Type: typ, Affinity: affinity(typ), NotNull: notnull != 0, Default: dflt, PK: pk}
			t.Columns = append(t.Columns, col)
			if pk > 0 {
				pks = append(pks, col)
			}
		}
		cr.Close()
		sort.Slice(pks, func(i, j int) bool { return pks[i].PK < pks[j].PK })
		for _, c := range pks {
			t.Key = append(t.Key, c.Name)
		}
		withoutRowID := strings.Contains(strings.ToUpper(e.sql), "WITHOUT ROWID")
		if len(t.Key) == 0 {
			if withoutRowID {
				continue // a table with neither a key nor a rowid cannot name a row
			}
			t.RowID = true
			t.Key = []string{"rowid"}
		}
		for i := range t.Columns {
			t.cols[t.Columns[i].Name] = &t.Columns[i]
		}
		applyCatalog(t)
		out[e.name] = t
	}
	return out, nil
}
