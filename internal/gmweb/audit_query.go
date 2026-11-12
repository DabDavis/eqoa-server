package gmweb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/handlers"
)

// Reading the audit log: its filters, its counts, its pages. Every filter
// but the free text goes by the facets (audit_facets.go), a small table with
// an index for each way a GM asks; the free text searches the lines' own
// words and rows.

// auditFilter narrows the log. Every filter given must hold (AND).
type auditFilter struct {
	// who
	Username string `json:"username,omitempty"` // exactly this account's name (any case)
	Who      string `json:"who,omitempty"`      // in the name, the agent's, the tool's or what it came through
	Source   string `json:"source,omitempty"`   // panel, agent, game, tool
	Tool     string `json:"tool,omitempty"`     // in the tool's or the agent's name, or a game command

	// what
	Table  string `json:"table,omitempty"`  // a table, or several by commas
	Group  string `json:"group,omitempty"`  // npcs, items, spells, loot, ... (auditGroups)
	Action string `json:"action,omitempty"` // added, changed, deleted, reverted, revert, or a raw action
	Name   string `json:"name,omitempty"`   // in the thing's name: an NPC's, an item's, a spell's, a character's
	RowID  *int64 `json:"row_id,omitempty"` // the row's id, or an id it refers to
	Key    string `json:"key,omitempty"`    // the row's key, exactly
	World  *int64 `json:"world,omitempty"`

	// when, why
	Since    string `json:"since,omitempty"`
	Until    string `json:"until,omitempty"`
	Petition int64  `json:"petition,omitempty"`
	Camp     int64  `json:"camp_report,omitempty"`
	Board    int64  `json:"board_entry,omitempty"`

	Search string `json:"search,omitempty"`
	Preset string `json:"preset,omitempty"`
	// Today is the reader's own midnight, for the today preset (else UTC's)
	Today string `json:"today,omitempty"`

	Limit  int  `json:"limit,omitempty"`
	Offset int  `json:"offset,omitempty"`
	Counts bool `json:"counts,omitempty"`

	// hideSQL keeps an admin's SQL from someone who is not one: the entries
	// show, their statements do not, and a search never matches them (SQL
	// can carry a password hash).
	hideSQL bool
	// tables keeps the log to these tables (a tester's); empty: every line.
	tables []string
	// limited: a reader below a GM, who is not counted others' names or tools
	limited bool
	// now is the clock "today" reads (tests set it)
	now time.Time
}

// auditPreset is a filter by name, for the page's quick buttons and an
// agent's shorthand.
type auditPreset struct {
	Name, Label, Help string
	apply             func(f *auditFilter)
}

var auditPresets = []auditPreset{
	{"npcs_deleted", "NPCs deleted", "rows of npcs deleted", func(f *auditFilter) { f.Table, f.Action = "npcs", kindDeleted }},
	{"npcs_added", "NPCs added", "rows of npcs added", func(f *auditFilter) { f.Table, f.Action = "npcs", kindAdded }},
	{"npcs_changed", "NPCs changed", "rows of npcs changed", func(f *auditFilter) { f.Table, f.Action = "npcs", kindChanged }},
	{"items_added", "Items added", "item records (itempattern) added", func(f *auditFilter) { f.Table, f.Action = "itempattern", kindAdded }},
	{"items_changed", "Items changed", "item records (itempattern) changed", func(f *auditFilter) { f.Table, f.Action = "itempattern", kindChanged }},
	{"items_given", "Items given", "items put in characters' inventories (charinventory added)", func(f *auditFilter) { f.Table, f.Action = "charinventory", kindAdded }},
	{"spells_changed", "Spells changed", "spell records changed (the spells group: spellpattern, effects, areas, gates, class spells, masteries)",
		func(f *auditFilter) { f.Group, f.Action = "spells", kindChanged }},
	{"loot_changed", "Loot changed", "loot tables, pools, rates and creature loot added, changed or deleted", func(f *auditFilter) { f.Group = "loot" }},
	{"camp_reports", "Camp reports", "camp reports and the changes made for them", func(f *auditFilter) { f.Camp = -1 }},
	{"petitions", "For petitions", "every change whose words name a petition", func(f *auditFilter) { f.Petition = -1 }},
	{"reverts", "Reverts", "lines that undo another", func(f *auditFilter) { f.Action = "revert" }},
	{"reverted", "Reverted", "changes undone since", func(f *auditFilter) { f.Action = "reverted" }},
	{"today", "Today", "since midnight UTC (or since, when given)", func(f *auditFilter) {
		switch {
		case f.Today != "":
			f.Since = f.Today
		case f.Since == "":
			f.Since = f.now.UTC().Format("2006-01-02")
		}
	}},
}

func presetByName(name string) *auditPreset {
	for i := range auditPresets {
		if auditPresets[i].Name == name {
			return &auditPresets[i]
		}
	}
	return nil
}

// auditTime reads a since/until: a date (YYYY-MM-DD, its midnight UTC), or a
// time in RFC 3339. day says it was a date.
func auditTime(v string) (t time.Time, day bool, err error) {
	v = strings.TrimSpace(v)
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), false, nil
	}
	if t, err := time.Parse("2006-01-02T15:04", v); err == nil {
		return t, false, nil
	}
	return time.Time{}, false, badRequest("%q is not a date (2026-11-12) or a time (2026-11-12T08:00:00Z)", v)
}

// The dimensions a count leaves out of its own filter, so the page can show
// what each choice would give.
const (
	dimNone   = ""
	dimKind   = "kind"
	dimSource = "source"
	dimGroup  = "group"
	dimTable  = "table"
	dimWho    = "who"
	dimTool   = "tool"
	dimWorld  = "world"
)

// where is the filter as SQL over the facets (f) and, for the free text, the
// lines (a); skip leaves one dimension out. needsLine says a needs joining.
func (fl *auditFilter) where(skip string) (w string, args []any, needsLine bool, err error) {
	var parts []string
	add := func(clause string, a ...any) {
		parts = append(parts, clause)
		args = append(args, a...)
	}
	like := func(s string) string { return "%" + likeEscape(strings.ToLower(strings.TrimSpace(s))) + "%" }

	if skip != dimWho {
		if fl.Username != "" {
			add("f.who = ?", strings.ToLower(strings.TrimSpace(fl.Username)))
		}
		if fl.Who != "" {
			add(`(f.who LIKE ? ESCAPE '\' OR LOWER(f.agent) LIKE ? ESCAPE '\' OR LOWER(f.tool) LIKE ? ESCAPE '\' OR f.source = ?)`,
				like(fl.Who), like(fl.Who), like(fl.Who), strings.ToLower(strings.TrimSpace(fl.Who)))
		}
	}
	if fl.Source != "" && skip != dimSource {
		add("f.source = ?", strings.ToLower(strings.TrimSpace(fl.Source)))
	}
	if fl.Tool != "" && skip != dimTool {
		add(`(LOWER(f.tool) LIKE ? ESCAPE '\' OR LOWER(f.agent) LIKE ? ESCAPE '\')`, like(fl.Tool), like(fl.Tool))
	}
	if fl.Table != "" && skip != dimTable {
		var ts []string
		for _, t := range strings.Split(fl.Table, ",") {
			if t = strings.TrimSpace(t); t != "" {
				ts = append(ts, t)
			}
		}
		if len(ts) == 1 {
			add("f.tbl = ?", ts[0])
		} else if len(ts) > 1 {
			ph := strings.TrimSuffix(strings.Repeat("?, ", len(ts)), ", ")
			a := make([]any, len(ts))
			for i, t := range ts {
				a[i] = t
			}
			add("f.tbl IN ("+ph+")", a...)
		}
	}
	if fl.Group != "" && skip != dimGroup {
		add("f.grp = ?", strings.ToLower(strings.TrimSpace(fl.Group)))
	}
	if fl.Action != "" && skip != dimKind {
		switch act := strings.ToLower(strings.TrimSpace(fl.Action)); act {
		case kindAdded, kindChanged, kindDeleted, kindOther:
			add("f.kind = ?", act)
		case "revert", "reverts":
			add("f.isrevert = 1")
		case "reverted":
			add("f.id IN (SELECT id FROM gm_audit WHERE reverted_by IS NOT NULL)")
		default:
			add("f.action = ?", act)
		}
	}
	if n := strings.TrimSpace(fl.Name); n != "" {
		add(`f.names LIKE ? ESCAPE '\'`, like(n))
	}
	if fl.RowID != nil {
		add("(f.rid = ? OR f.ids LIKE ?)", *fl.RowID, fmt.Sprintf("%% %d %%", *fl.RowID))
	}
	if fl.Key != "" {
		add("f.rkey = ?", fl.Key)
	}
	if fl.World != nil && skip != dimWorld {
		add("f.world = ?", *fl.World)
	}
	if fl.Since != "" {
		t, _, err := auditTime(fl.Since)
		if err != nil {
			return "", nil, false, err
		}
		add("f.at >= ?", stamp(t))
	}
	if fl.Until != "" {
		t, day, err := auditTime(fl.Until)
		if err != nil {
			return "", nil, false, err
		}
		if day {
			add("f.at < ?", stamp(t.AddDate(0, 0, 1)))
		} else {
			add("f.at <= ?", stamp(t))
		}
	}
	refFilter := func(tag string, n int64, extra string) {
		switch {
		case n > 0:
			add("f.refs LIKE ?", fmt.Sprintf("%% %s%d %%", tag, n))
		case n < 0: // any
			add("(f.refs LIKE ?"+extra+")", "% "+tag+"%")
		}
	}
	refFilter("p", fl.Petition, "")
	refFilter("c", fl.Camp, " OR f.tbl = 'campfix_reports'")
	refFilter("b", fl.Board, "")
	if len(fl.tables) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?, ", len(fl.tables)), ", ")
		a := make([]any, len(fl.tables))
		for i, t := range fl.tables {
			a[i] = t
		}
		add("f.tbl IN ("+ph+")", a...)
	}
	if s := strings.TrimSpace(fl.Search); s != "" {
		clause := `(a.detail LIKE ? ESCAPE '\' OR a.before LIKE ? ESCAPE '\' OR a.after LIKE ? ESCAPE '\' OR f.thing LIKE ? ESCAPE '\')`
		if fl.hideSQL {
			clause = "(a.action != 'sql' AND " + clause + ")"
		}
		p := "%" + likeEscape(s) + "%"
		add(clause, p, p, p, p)
		needsLine = true
	}
	if len(parts) > 0 {
		w = " WHERE " + strings.Join(parts, " AND ")
	}
	return w, args, needsLine, nil
}

func (fl *auditFilter) from(needsLine bool) string {
	if needsLine {
		return ` FROM gm_audit_facets f JOIN gm_audit a ON a.id = f.id`
	}
	return ` FROM gm_audit_facets f`
}

// applyPreset puts a preset's filters on (over those given for the same
// thing).
func (fl *auditFilter) applyPreset() error {
	if fl.now.IsZero() {
		fl.now = time.Now()
	}
	if fl.Preset == "" {
		return nil
	}
	p := presetByName(fl.Preset)
	if p == nil {
		var names []string
		for _, x := range auditPresets {
			names = append(names, x.Name)
		}
		return badRequest("no preset %q (%s)", fl.Preset, strings.Join(names, ", "))
	}
	p.apply(fl)
	return nil
}

// auditCounts is how many lines each choice of a filter would give, the
// other filters held: by kind, source, group, table, who and tool; and each
// preset's lines alone.
type auditCounts struct {
	Kind    map[string]int64 `json:"kind"`
	Source  map[string]int64 `json:"source,omitempty"`
	Group   map[string]int64 `json:"group"`
	Table   []countRow       `json:"table"`
	Who     []countRow       `json:"who,omitempty"`
	Tool    []countRow       `json:"tool,omitempty"`
	World   []countRow       `json:"world"`
	Presets map[string]int64 `json:"presets"`
}

type countRow struct {
	Value string `json:"value"`
	N     int64  `json:"n"`
	Label string `json:"label,omitempty"`
}

func (s *Server) countBy(fl auditFilter, dim, col string, top int) ([]countRow, error) {
	w, args, line, err := fl.where(dim)
	if err != nil {
		return nil, err
	}
	q := `SELECT ` + col + `, COUNT(*) AS n` + fl.from(line) + w + ` GROUP BY 1 ORDER BY n DESC, 1`
	if top > 0 {
		q += fmt.Sprintf(" LIMIT %d", top)
	}
	rs, err := s.d.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []countRow{}
	for rs.Next() {
		var r countRow
		if err := rs.Scan(&r.Value, &r.N); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

func asMap(rows []countRow) map[string]int64 {
	m := map[string]int64{}
	for _, r := range rows {
		m[r.Value] = r.N
	}
	return m
}

func (s *Server) auditCount(fl auditFilter) (int64, error) {
	w, args, line, err := fl.where(dimNone)
	if err != nil {
		return 0, err
	}
	var n int64
	err = s.d.DB.QueryRow(`SELECT COUNT(*)`+fl.from(line)+w, args...).Scan(&n)
	return n, err
}

func (s *Server) auditCounts(fl auditFilter) (*auditCounts, error) {
	// one pass over the lines the whole filter keeps, tallied by every
	// dimension; then each dimension the filter narrows counted again
	// without its own filter, so the page shows what each other choice gives
	w, args, line, err := fl.where(dimNone)
	if err != nil {
		return nil, err
	}
	rs, err := s.d.DB.Query(`SELECT f.kind, f.source, f.grp, f.tbl, f.who, f.tool, f.agent, COALESCE(f.world, -1)`+fl.from(line)+w, args...)
	if err != nil {
		return nil, err
	}
	tally := map[string]map[string]int64{}
	for _, d := range []string{dimKind, dimSource, dimGroup, dimTable, dimWho, dimTool, dimWorld} {
		tally[d] = map[string]int64{}
	}
	for rs.Next() {
		var kind, source, grp, tbl, who, tool, agent string
		var world int64
		if err := rs.Scan(&kind, &source, &grp, &tbl, &who, &tool, &agent, &world); err != nil {
			rs.Close()
			return nil, err
		}
		tally[dimKind][kind]++
		tally[dimSource][source]++
		tally[dimGroup][grp]++
		tally[dimTable][tbl]++
		tally[dimWho][who]++
		if tool != "" {
			tally[dimTool][tool]++
		} else if agent != "" {
			tally[dimTool][agent]++
		}
		if world >= 0 {
			tally[dimWorld][strconv.FormatInt(world, 10)]++
		}
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	active := map[string]bool{
		dimKind: fl.Action != "", dimSource: fl.Source != "", dimGroup: fl.Group != "", dimTable: fl.Table != "",
		dimWho: fl.Username != "" || fl.Who != "", dimTool: fl.Tool != "", dimWorld: fl.World != nil,
	}
	cols := map[string]string{dimKind: "f.kind", dimSource: "f.source", dimGroup: "f.grp", dimTable: "f.tbl", dimWho: "f.who",
		dimTool: "CASE WHEN f.tool != '' THEN f.tool ELSE f.agent END", dimWorld: "COALESCE(f.world, -1)"}
	for d, on := range active {
		if !on || (fl.limited && (d == dimSource || d == dimWho || d == dimTool)) {
			continue
		}
		rows, err := s.countBy(fl, d, cols[d], 0)
		if err != nil {
			return nil, err
		}
		m := map[string]int64{}
		for _, r := range rows {
			if (d == dimTool && r.Value == "") || (d == dimWorld && r.Value == "-1") {
				continue
			}
			m[r.Value] = r.N
		}
		tally[d] = m
	}
	c := &auditCounts{Kind: tally[dimKind], Group: tally[dimGroup], Table: topOf(tally[dimTable], 15), World: topOf(tally[dimWorld], 0)}
	worlds := handlers.WorldNames()
	for i, r := range c.World {
		if w, err := strconv.Atoi(r.Value); err == nil && w >= 0 && w < len(worlds) {
			c.World[i].Label = worlds[w]
		}
	}
	if !fl.limited {
		c.Source, c.Who, c.Tool = tally[dimSource], topOf(tally[dimWho], 12), topOf(tally[dimTool], 15)
	}
	if c.Presets, err = s.presetCounts(fl); err != nil {
		return nil, err
	}
	return c, nil
}

// topOf is a tally's largest, most first (0: all).
func topOf(m map[string]int64, top int) []countRow {
	out := make([]countRow, 0, len(m))
	for v, n := range m {
		out = append(out, countRow{Value: v, N: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Value < out[j].Value
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// presetCounts is each preset's lines alone, as this reader may read the
// log: counted again once a line has been added since, at most every 20
// seconds (the log grows by a line a few seconds while agents work).
func (s *Server) presetCounts(fl auditFilter) (map[string]int64, error) {
	var top int64
	s.d.DB.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM gm_audit_facets`).Scan(&top)
	key := fmt.Sprintf("%s|%s|%s|%v", strings.Join(fl.tables, ","), fl.Today, fl.now.UTC().Format("2006-01-02"), fl.limited)
	s.presetMu.Lock()
	defer s.presetMu.Unlock()
	if s.presetCache == nil {
		s.presetCache = map[string]*presetCount{}
	}
	if c, ok := s.presetCache[key]; ok && (c.top == top || time.Since(c.at) < 20*time.Second) {
		return c.counts, nil
	}
	m := map[string]int64{}
	for _, p := range auditPresets {
		pf := auditFilter{tables: fl.tables, hideSQL: fl.hideSQL, limited: fl.limited, now: fl.now, Preset: p.Name, Today: fl.Today}
		pf.applyPreset()
		n, err := s.auditCount(pf)
		if err != nil {
			return nil, err
		}
		m[p.Name] = n
	}
	if len(s.presetCache) > 64 {
		s.presetCache = map[string]*presetCount{}
	}
	s.presetCache[key] = &presetCount{top: top, at: time.Now(), counts: m}
	return m, nil
}

// presetCount is the presets' counts as they stood at a line.
type presetCount struct {
	top    int64
	at     time.Time
	counts map[string]int64
}

// auditList is a page of the log, newest first.
func (s *Server) auditList(f auditFilter) ([]auditEntry, int64, error) {
	list, total, _, err := s.auditQuery(f)
	return list, total, err
}

// auditQuery is a page of the log, newest first, its total, and with
// f.Counts what each filter's choices would give.
func (s *Server) auditQuery(f auditFilter) ([]auditEntry, int64, *auditCounts, error) {
	if err := s.indexAudit(); err != nil {
		return nil, 0, nil, fmt.Errorf("indexing the audit log: %w", err)
	}
	if err := f.applyPreset(); err != nil {
		return nil, 0, nil, err
	}
	w, args, line, err := f.where(dimNone)
	if err != nil {
		return nil, 0, nil, err
	}
	var total int64
	if err := s.d.DB.QueryRow(`SELECT COUNT(*)`+f.from(line)+w, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	// the page's ids from the facets, then the lines
	rs, err := s.d.DB.Query(`SELECT f.id`+f.from(line)+w+` ORDER BY f.id DESC LIMIT ? OFFSET ?`,
		append(args, f.Limit, max(0, f.Offset))...)
	if err != nil {
		return nil, 0, nil, err
	}
	var ids []any
	for rs.Next() {
		var id int64
		if err := rs.Scan(&id); err != nil {
			rs.Close()
			return nil, 0, nil, err
		}
		ids = append(ids, id)
	}
	rs.Close()
	list := []auditEntry{}
	if len(ids) > 0 {
		ph := strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")
		cols := strings.ReplaceAll(auditColumns, ", ", ", a.")
		// the schema before the rows: the database has one connection, which
		// the open rows hold
		tables, _ := s.tables()
		rs, err := s.d.DB.Query(`SELECT a.`+cols+`, f.thing, f.kind, f.source, f.agent, f.tool, f.grp, f.world, f.refs, f.isrevert
			FROM gm_audit a JOIN gm_audit_facets f ON f.id = a.id WHERE a.id IN (`+ph+`) ORDER BY a.id DESC`, ids...)
		if err != nil {
			return nil, 0, nil, err
		}
		if list, err = scanAuditFaceted(rs, tables); err != nil {
			return nil, 0, nil, err
		}
	}
	if f.hideSQL {
		for i := range list {
			if list[i].Action == "sql" {
				list[i].Detail = "an admin's SQL (shown to admins only)"
			}
		}
	}
	var counts *auditCounts
	if f.Counts {
		if counts, err = s.auditCounts(f); err != nil {
			return nil, 0, nil, err
		}
	}
	return list, total, counts, nil
}

// scanAuditFaceted reads lines with their facets, and works out each changed
// row's changed fields.
func scanAuditFaceted(rs *sql.Rows, tables map[string]*Table) ([]auditEntry, error) {
	defer rs.Close()
	worlds := handlers.WorldNames()
	out := []auditEntry{}
	for rs.Next() {
		var e auditEntry
		var rowkey, refs string
		var before, after sql.NullString
		var reverted, world sql.NullInt64
		var isrev int
		if err := rs.Scan(&e.ID, &e.At, &e.AccountID, &e.Username, &e.Via, &e.IP, &e.Action, &e.Table,
			&rowkey, &before, &after, &e.Detail, &reverted,
			&e.Thing, &e.Kind, &e.Source, &e.Agent, &e.Tool, &e.Group, &world, &refs, &isrev); err != nil {
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
		e.IsRevert = isrev == 1
		if world.Valid {
			w := world.Int64
			e.World = &w
			if w >= 0 && int(w) < len(worlds) {
				e.WorldName = worlds[w]
			}
		}
		for _, r := range strings.Fields(refs) {
			n, err := strconv.ParseInt(r[1:], 10, 64)
			if err != nil {
				continue
			}
			switch r[0] {
			case 'p':
				e.Petitions = append(e.Petitions, n)
			case 'c':
				e.Camps = append(e.Camps, n)
			case 'b':
				e.Boards = append(e.Boards, n)
			}
		}
		if e.Action == "update" && e.Before != nil && e.After != nil {
			e.Changes = changedFields(tables[e.Table], e.Before, e.After)
		}
		out = append(out, e)
	}
	return out, rs.Err()
}

// changedFields is the columns a change changed, in the table's order.
func changedFields(t *Table, before, after map[string]any) []fieldChange {
	var keys []string
	seen := map[string]bool{}
	if t != nil {
		for _, c := range t.Columns {
			keys = append(keys, c.Name)
			seen[c.Name] = true
		}
	}
	var rest []string
	for k := range before {
		if !seen[k] {
			rest = append(rest, k)
			seen[k] = true
		}
	}
	for k := range after {
		if !seen[k] {
			rest = append(rest, k)
			seen[k] = true
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	var out []fieldChange
	for _, k := range keys {
		if k == "__key" {
			continue
		}
		b, inB := before[k]
		a, inA := after[k]
		if !inB && !inA {
			continue
		}
		if sameValue(b, a) {
			continue
		}
		fc := fieldChange{Field: k, Before: b, After: a}
		if t != nil {
			if c := t.col(k); c != nil {
				fc.Label = c.Label
			}
		}
		out = append(out, fc)
	}
	return out
}

func groupNames() []string {
	out := make([]string, len(auditGroups))
	for i, g := range auditGroups {
		out[i] = g.Label
	}
	return out
}

func presetNames() []string {
	out := make([]string, len(auditPresets))
	for i, p := range auditPresets {
		out[i] = p.Name
	}
	return out
}
