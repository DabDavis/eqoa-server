package gmweb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The audit log's facets: what each line is about, worked out once and kept
// beside it, so the log can be filtered by what a GM means (an NPC's name, an
// item added, a camp report's changes, a tool's run) and counted, fast, on
// 60,000 lines and more. gm_audit is written by the panel, the game's GM
// commands, the data tools (tools/*.py write it straight) and the database
// package; none of them needs to know of this table: every read of the log
// first indexes the lines past the last one indexed (indexAudit), so a line
// is in the facets from the next read on, whoever wrote it.
//
// A facet line is derived wholly from its gm_audit line and the rows it names
// as they stood when it was indexed; it can be thrown away and made again.
// The sanitised snapshot clears it with gm_audit (database/snapshot.go).

const facetSchema = `
CREATE TABLE IF NOT EXISTS gm_audit_facets (
	id       INTEGER PRIMARY KEY,
	at       TEXT NOT NULL,
	who      TEXT NOT NULL,
	source   TEXT NOT NULL,
	agent    TEXT NOT NULL DEFAULT '',
	tool     TEXT NOT NULL DEFAULT '',
	action   TEXT NOT NULL,
	kind     TEXT NOT NULL,
	isrevert INTEGER NOT NULL DEFAULT 0,
	tbl      TEXT NOT NULL DEFAULT '',
	grp      TEXT NOT NULL DEFAULT '',
	rkey     TEXT NOT NULL DEFAULT '',
	rid      INTEGER,
	ids      TEXT NOT NULL DEFAULT '',
	thing    TEXT NOT NULL DEFAULT '',
	names    TEXT NOT NULL DEFAULT '',
	world    INTEGER,
	refs     TEXT NOT NULL DEFAULT '',
	ver      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_tbl ON gm_audit_facets(tbl, kind);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_grp ON gm_audit_facets(grp, kind);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_rkey ON gm_audit_facets(tbl, rkey);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_who ON gm_audit_facets(who);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_at ON gm_audit_facets(at);
CREATE INDEX IF NOT EXISTS idx_gm_audit_facets_rid ON gm_audit_facets(rid);
CREATE INDEX IF NOT EXISTS idx_gm_audit_reverted ON gm_audit(reverted_by) WHERE reverted_by IS NOT NULL;
`

// facetVersion is the derivation's version: a line indexed by an older one is
// indexed again (indexAudit), so a better rule reaches the old lines.
const facetVersion = 2

// facetBatch is how many lines one transaction indexes.
const facetBatch = 500

// facet is one line's facets.
type facet struct {
	ID       int64
	At       string
	Who      string
	Source   string
	Agent    string
	Tool     string
	Action   string
	Kind     string
	IsRevert bool
	Table    string
	Group    string
	RKey     string
	RID      sql.NullInt64
	IDs      string
	Thing    string
	Names    string
	World    sql.NullInt64
	Refs     string
}

// The kinds of change, as a GM says them.
const (
	kindAdded   = "added"
	kindChanged = "changed"
	kindDeleted = "deleted"
	kindOther   = "other"
)

func kindOf(action string) string {
	switch action {
	case "insert", "create":
		return kindAdded
	case "update":
		return kindChanged
	case "delete":
		return kindDeleted
	}
	return kindOther
}

// The sources: who or what wrote the line.
const (
	sourcePanel = "panel" // a person on the page
	sourceAgent = "agent" // an AI agent signed in as a GM (OAuth or a token)
	sourceGame  = "game"  // a GM command in the game, or a game feature that logs
	sourceTool  = "tool"  // a data tool or a shell on the server
)

func sourceOf(via string) (source, agent string) {
	switch {
	case via == "web" || via == "snapshot link":
		return sourcePanel, ""
	case strings.HasPrefix(via, "oauth:"):
		return sourceAgent, strings.TrimPrefix(via, "oauth:")
	case strings.HasPrefix(via, "token:"):
		return sourceAgent, strings.TrimPrefix(via, "token:")
	case via == "game":
		return sourceGame, ""
	case via == "tool" || via == "shell" || via == "cli" || via == "sql":
		return sourceTool, ""
	}
	return via, ""
}

// The table groups, for a GM's "NPCs" or "loot": a table is in one.
var (
	groupExact = map[string]string{
		"npcitem": "merchants", "heroes_stock": "merchants",
		"npcloot": "loot", "npcfaction": "factions",
		"classtemplates": "npcs", "signtext": "npcs", "petstats": "npcs", "pettemplate": "npcs",
		"cmcatalog": "spells", "classspells": "spells",
		"heirlooms": "items", "norentvanish": "items", "foodeffect": "items", "startitems": "items",
		"defaultcharacter": "characters", "spells": "characters", "quickslots": "characters",
		"auction_notices": "characters", "guilds": "characters", "importdamper_log": "characters",
		"chartransfers": "transfers", "petitions": "petitions",
		"accountinfo": "accounts", "gm_testers": "accounts", "gm_storytellers": "accounts", "gm_totp": "accounts",
		"gmtriggers": "triggers", "orbs": "world", "hazards": "world", "noroam": "world", "mapmarks": "world", "racedefs": "world",
		"racesize": "world", "racearmorbody": "world", "guide_set_log": "world",
		"gmcamps": "camps", "campspawns": "camps", "gmmarks": "camps", "campfix_reports": "camps",
	}
	groupPrefix = []struct{ prefix, group string }{
		{"workboard", "work"}, {"npc", "npcs"}, {"item", "items"}, {"spell", "spells"},
		{"loot", "loot"}, {"quest", "quests"}, {"char", "characters"}, {"faction", "factions"},
		{"balance", "balance"}, {"gm_", "panel"},
	}
)

// auditGroups are the groups in the page's order, with what each says.
var auditGroups = []EnumValue{
	{Label: "npcs", Help: "NPCs and their kinds: npcs, respawns, wanderers, paths, aggro, movement, templates, dialog, signs, pets"},
	{Label: "items", Help: "item records: itempattern, item effects, ascended, hidden, heirlooms, starting items"},
	{Label: "spells", Help: "spell records: spellpattern, effects, areas, gates, class spells, masteries"},
	{Label: "loot", Help: "loot tables, pools, rates, creature loot"},
	{Label: "merchants", Help: "what merchants sell (npcitem)"},
	{Label: "quests", Help: "quests, steps, targets, unlocks"},
	{Label: "characters", Help: "characters and their rows: inventory, spell books, quests, masteries, dyes"},
	{Label: "factions", Help: "factions, faction hits, starts, NPC factions"},
	{Label: "triggers", Help: "GM triggers"},
	{Label: "camps", Help: "GM camps, marks, camp spawns, camp reports"},
	{Label: "petitions", Help: "petitions"},
	{Label: "transfers", Help: "character transfers"},
	{Label: "accounts", Help: "accounts and panel roles"},
	{Label: "balance", Help: "the balance tables"},
	{Label: "world", Help: "races, orbs, map marks"},
	{Label: "work", Help: "the work board"},
	{Label: "panel", Help: "the panel's own tables"},
	{Label: "other", Help: "any other table"},
	{Label: "system", Help: "no table: sign-ins, tokens, SQL, backups, settings, live actions, restarts"},
}

func groupOf(table string) string {
	if table == "" {
		return "system"
	}
	if g, ok := groupExact[table]; ok {
		return g
	}
	for _, p := range groupPrefix {
		if strings.HasPrefix(table, p.prefix) {
			return p.group
		}
	}
	return "other"
}

var (
	// a script a tool line names: tools/x.py, ~/charmigrate/y.py, run.sh
	toolScriptRE = regexp.MustCompile(`([A-Za-z0-9_.-]+\.(?:py|sh|go))\b`)
	// a GM command in the game: "/clone in the game", "#reward in the game"
	gameCommandRE = regexp.MustCompile(`^([/#][A-Za-z0-9_]+) in the game`)
	// petitions named in a line's words: "petition 1446", "petitions 1166, 1167",
	// "tickets 1196, 1197", "petitions 1327-1337"
	petitionRE = regexp.MustCompile(`(?i)\b(?:petitions?|tickets?)\s+#?(\d+(?:\s*(?:,|and|&|-|/)\s*#?\d+)*)`)
	campRE     = regexp.MustCompile(`(?i)\bcamp (?:fix )?reports?\s+#?(\d+)`)
	boardRE    = regexp.MustCompile(`(?i)\b(?:work board: entry|board entry|board)\s+#?(\d+(?:\s*[,/]\s*\d+)*)`)
	numberRE   = regexp.MustCompile(`\d+`)
)

// toolOf is the tool a line came through: a script's name for a tool, the
// command for a GM's in the game, else what the line was written through.
func toolOf(source, via, table, detail string) string {
	switch source {
	case sourceTool:
		if m := toolScriptRE.FindStringSubmatch(detail); m != nil {
			return m[1]
		}
		if via == "cli" && strings.HasPrefix(table, "workboard") {
			return "workboard"
		}
		return via
	case sourceGame:
		if m := gameCommandRE.FindStringSubmatch(detail); m != nil {
			return m[1]
		}
	}
	return ""
}

// refsOf is what a line's words name, as " p1446 c122 b1226 ": petitions
// (p), camp reports (c), work board entries (b); with the line's own row
// when it is one of those.
func refsOf(table string, key map[string]any, detail string) string {
	seen := map[string]bool{}
	var out []string
	add := func(tag, n string) {
		if n = strings.TrimLeft(n, "0"); n == "" {
			return
		}
		if r := tag + n; !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, m := range petitionRE.FindAllStringSubmatch(detail, -1) {
		for _, n := range numberRE.FindAllString(m[1], -1) {
			add("p", n)
		}
	}
	for _, m := range campRE.FindAllStringSubmatch(detail, -1) {
		add("c", m[1])
	}
	for _, m := range boardRE.FindAllStringSubmatch(detail, -1) {
		for _, n := range numberRE.FindAllString(m[1], -1) {
			add("b", n)
		}
	}
	if id, ok := key["id"]; ok && len(key) == 1 {
		n := fmt.Sprint(id)
		switch table {
		case "petitions":
			add("p", n)
		case "campfix_reports":
			add("c", n)
		case "workboard":
			add("b", n)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return " " + strings.Join(out, " ") + " "
}

// The columns that name a row when its table's title does not.
var nameColumns = []string{"name", "npc_name", "itemname", "spellname", "charName", "charname", "username",
	"title", "tablename", "spell", "boss", "category", "from_name"}

// Tables whose rows a row usually belongs to: named after the thing itself.
var ownerTables = map[string]bool{"characters": true, "npcs": true, "accountinfo": true}

// facetCtx is one indexing run's lookups.
type facetCtx struct {
	s      *Server
	tables map[string]*Table
	q      dbtx
	names  map[string]refName
}

type refName struct {
	name  string
	world sql.NullInt64
}

func asInt(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x), true
		}
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n, true
		}
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func clipName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 80 {
		return string(r[:79]) + "…"
	}
	return s
}

// ownName is what a row calls itself: its table's title column when that is
// words (not a reference, not a number), else a column named like a name.
func ownName(t *Table, row map[string]any) string {
	if row == nil {
		return ""
	}
	try := func(col string) string {
		if t != nil {
			if c := t.col(col); c != nil && c.Ref != "" {
				return ""
			}
		}
		if v, ok := row[col].(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				if _, num := asInt(v); !num {
					return clipName(v)
				}
			}
		}
		return ""
	}
	if t != nil && t.Title != "" {
		if n := try(t.Title); n != "" {
			return n
		}
	}
	for _, c := range nameColumns {
		if n := try(c); n != "" {
			return n
		}
	}
	return ""
}

// lookup is the name (and an NPC's world) of row val of table ref: as it is
// now, else as the log last had it (a deleted row's line keeps it).
func (fc *facetCtx) lookup(ref string, val any) refName {
	id := ref + "\x00" + fmt.Sprint(val)
	if r, ok := fc.names[id]; ok {
		return r
	}
	var out refName
	defer func() { fc.names[id] = out }()
	rt := fc.tables[ref]
	keyCol := ""
	if rt != nil && len(rt.Key) == 1 {
		keyCol = rt.Key[0]
	} else if rt != nil && rt.RowID {
		keyCol = "rowid"
	}
	if keyCol == "" {
		return out
	}
	rows, err := fc.q.Query(`SELECT * FROM `+quoteIdent(ref)+` WHERE `+quoteIdent(keyCol)+` = ? LIMIT 1`, val)
	if err == nil {
		list, _ := scanRowMaps(rows)
		if len(list) == 1 {
			out.name = ownName(rt, list[0])
			if w, ok := asInt(list[0]["world"]); ok && ref == "npcs" {
				out.world = sql.NullInt64{Int64: w, Valid: true}
			}
			return out
		}
	}
	// gone: the log's last word of it
	compact, _ := json.Marshal(map[string]any{keyCol: val})
	spaced := strings.Replace(string(compact), `":`, `": `, 1)
	var before, after sql.NullString
	if fc.q.QueryRow(`SELECT before, after FROM gm_audit WHERE tbl = ? AND rowkey IN (?, ?) ORDER BY id DESC LIMIT 1`,
		ref, string(compact), spaced).Scan(&before, &after) == nil {
		var row map[string]any
		if after.Valid {
			json.Unmarshal([]byte(after.String), &row)
		}
		if row == nil && before.Valid {
			json.Unmarshal([]byte(before.String), &row)
		}
		out.name = ownName(rt, row)
		if w, ok := asInt(row["world"]); ok && ref == "npcs" {
			out.world = sql.NullInt64{Int64: w, Valid: true}
		}
	}
	return out
}

// scanRowMaps reads rows as name -> value (fromSQL's forms).
func scanRowMaps(rs *sql.Rows) ([]map[string]any, error) {
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
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
		m := map[string]any{}
		for i, c := range cols {
			m[c] = fromSQL(vals[i])
		}
		out = append(out, m)
	}
	return out, rs.Err()
}

// rawAudit is a gm_audit line as the indexer reads it.
type rawAudit struct {
	id                                          int64
	at, username, via, action, tbl, key, detail string
	before, after                               sql.NullString
}

// facetOf works a line's facets out.
func (fc *facetCtx) facetOf(r rawAudit) facet {
	f := facet{ID: r.id, At: r.at, Who: strings.ToLower(r.username), Action: r.action, Kind: kindOf(r.action),
		Table: r.tbl, Group: groupOf(r.tbl)}
	f.Source, f.Agent = sourceOf(r.via)
	sqlLine := r.action == "sql"
	if !sqlLine {
		f.Tool = toolOf(f.Source, r.via, r.tbl, r.detail)
	}
	d := strings.ToLower(r.detail)
	f.IsRevert = r.action == "revert" || strings.HasPrefix(d, "reverts entry") || strings.HasPrefix(d, "restored ")
	var key, before, after map[string]any
	if r.key != "" {
		json.Unmarshal([]byte(r.key), &key)
	}
	if r.before.Valid && !sqlLine {
		json.Unmarshal([]byte(r.before.String), &before)
	}
	if r.after.Valid && !sqlLine {
		json.Unmarshal([]byte(r.after.String), &after)
	}
	if key != nil {
		f.RKey = keyText(key)
	}
	if !sqlLine {
		f.Refs = refsOf(r.tbl, key, r.detail)
	}
	if r.tbl == "" || sqlLine {
		return f
	}
	t := fc.tables[r.tbl]
	row := after
	if row == nil {
		row = before
	}
	// a tool may log only the columns it changed: the key's own are the row's too
	full := map[string]any{}
	for k, v := range key {
		full[k] = v
	}
	for k, v := range row {
		full[k] = v
	}

	// the ids: the key's, then what the row refers to
	var ids []string
	addID := func(v any) {
		if n, ok := asInt(v); ok {
			s := strconv.FormatInt(n, 10)
			for _, x := range ids {
				if x == s {
					return
				}
			}
			ids = append(ids, s)
		}
	}
	keyCols := make([]string, 0, len(key))
	for k := range key {
		keyCols = append(keyCols, k)
	}
	if t != nil && len(t.Key) > 0 && !t.RowID {
		keyCols = append([]string(nil), t.Key...)
	} else {
		sort.Strings(keyCols)
	}
	for _, k := range keyCols {
		v := full[k]
		if n, ok := asInt(v); ok && !f.RID.Valid {
			f.RID = sql.NullInt64{Int64: n, Valid: true}
		}
		addID(v)
	}

	// the names: a key's reference first (a respawn's NPC, an effect's
	// spell), else the row's own; then the things it refers to, its owner
	// (a character, an NPC) last
	var names, owners []string
	addName := func(list *[]string, n string) {
		if n == "" {
			return
		}
		for _, x := range names {
			if strings.EqualFold(x, n) {
				return
			}
		}
		for _, x := range owners {
			if strings.EqualFold(x, n) {
				return
			}
		}
		*list = append(*list, n)
	}
	refCol := func(col string) {
		ref := refOf(fc.tables, t, r.tbl, col)
		if ref == "" {
			return
		}
		v := full[col]
		if v == nil {
			return
		}
		if n, ok := asInt(v); ok && n == 0 {
			return
		}
		addID(v)
		rn := fc.lookup(ref, v)
		if rn.world.Valid && !f.World.Valid {
			f.World = rn.world
		}
		if ownerTables[ref] {
			addName(&owners, rn.name)
		} else {
			addName(&names, rn.name)
		}
	}
	keyHasRef := false
	for _, k := range keyCols {
		if refOf(fc.tables, t, r.tbl, k) != "" {
			keyHasRef = true
			refCol(k)
		}
	}
	if !keyHasRef || (len(names) == 0 && len(owners) == 0) {
		own := ownName(t, row)
		if own == "" && len(key) == 1 {
			// a row logged by its changed columns alone: its name as it is now
			for _, v := range key {
				rn := fc.lookup(r.tbl, v)
				own = rn.name
				if rn.world.Valid && !f.World.Valid {
					f.World = rn.world
				}
			}
		}
		addName(&names, own)
	}
	var cols []string
	if t != nil {
		for _, c := range t.Columns {
			cols = append(cols, c.Name)
		}
	} else {
		for k := range full {
			cols = append(cols, k)
		}
		sort.Strings(cols)
	}
	for _, c := range cols {
		if !contains(keyCols, c) {
			refCol(c)
		}
	}
	all := append(append([]string(nil), names...), owners...)
	// the thing as a line shows it: two names at most (an item and whose)
	shown := all
	if len(shown) > 2 {
		shown = shown[:2]
	}
	f.Thing = strings.Join(shown, " · ")
	if len(all) > 3 {
		all = all[:3]
	}
	// what the name filter searches: every name the line had, before and after
	lc := []string{}
	for _, n := range all {
		lc = append(lc, strings.ToLower(n))
	}
	if before != nil && after != nil {
		if old := ownName(t, before); old != "" && !strings.EqualFold(old, ownName(t, after)) {
			lc = append(lc, strings.ToLower(old))
		}
	}
	f.Names = strings.Join(lc, "\n")
	if f.Names != "" {
		f.Names = "\n" + f.Names + "\n"
	}
	if w, ok := asInt(full["world"]); ok && (t == nil || t.col("world") != nil) {
		f.World = sql.NullInt64{Int64: w, Valid: true}
	}
	if len(ids) > 0 {
		f.IDs = " " + strings.Join(ids, " ") + " "
	}
	return f
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// indexAudit indexes the lines not yet indexed (and those an older rule
// indexed), a batch a transaction. It is cheap when nothing is new: two
// lookups of a largest id.
func (s *Server) indexAudit() error {
	s.facetMu.Lock()
	defer s.facetMu.Unlock()
	return s.indexAuditLocked()
}

func (s *Server) indexAuditLocked() error {
	var last, top sql.NullInt64
	if err := s.d.DB.QueryRow(`SELECT MAX(id) FROM gm_audit_facets`).Scan(&last); err != nil {
		return err
	}
	if err := s.d.DB.QueryRow(`SELECT MAX(id) FROM gm_audit`).Scan(&top); err != nil {
		return err
	}
	if last.Int64 > top.Int64 {
		// the log was emptied (a snapshot's copy) and begun again
		if _, err := s.d.DB.Exec(`DELETE FROM gm_audit_facets`); err != nil {
			return err
		}
		last.Int64 = 0
	}
	var stale int64
	if !s.facetVerChecked { // once a server: a line is indexed by this rule from then on
		s.d.DB.QueryRow(`SELECT COUNT(*) FROM gm_audit_facets WHERE ver < ?`, facetVersion).Scan(&stale)
	}
	if stale > 0 {
		if _, err := s.d.DB.Exec(`DELETE FROM gm_audit_facets WHERE ver < ?`, facetVersion); err != nil {
			return err
		}
		last.Int64 = 0
	}
	if last.Int64 >= top.Int64 && stale == 0 {
		s.facetVerChecked = true
		return nil
	}
	tables, err := s.tables()
	if err != nil {
		return err
	}
	start := time.Now()
	n := 0
	fc := &facetCtx{s: s, tables: tables, q: s.d.DB, names: map[string]refName{}}
	for {
		rs, err := s.d.DB.Query(`SELECT id, at, username, via, action, tbl, rowkey, before, after, detail FROM gm_audit
			WHERE id > ? AND id NOT IN (SELECT id FROM gm_audit_facets WHERE id > ?) ORDER BY id LIMIT ?`, last.Int64, last.Int64, facetBatch)
		if err != nil {
			return err
		}
		var batch []rawAudit
		for rs.Next() {
			var r rawAudit
			if err := rs.Scan(&r.id, &r.at, &r.username, &r.via, &r.action, &r.tbl, &r.key, &r.before, &r.after, &r.detail); err != nil {
				rs.Close()
				return err
			}
			batch = append(batch, r)
		}
		rs.Close()
		if err := rs.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		facets := make([]facet, len(batch))
		for i, r := range batch {
			facets[i] = fc.facetOf(r)
		}
		if err := s.writeFacets(facets); err != nil {
			return err
		}
		last.Int64 = batch[len(batch)-1].id
		n += len(batch)
		if len(batch) == facetBatch {
			time.Sleep(5 * time.Millisecond) // the game's turn at the one connection
		}
	}
	s.facetVerChecked = true
	if n > 1000 {
		log.Printf("GM panel: indexed %d audit lines for the log's filters in %v", n, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func (s *Server) writeFacets(list []facet) error {
	tx, err := s.d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT OR REPLACE INTO gm_audit_facets (id, at, who, source, agent, tool, action, kind, isrevert,
		tbl, grp, rkey, rid, ids, thing, names, world, refs, ver) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, f := range list {
		rev := 0
		if f.IsRevert {
			rev = 1
		}
		if _, err := st.Exec(f.ID, f.At, f.Who, f.Source, f.Agent, f.Tool, f.Action, f.Kind, rev, f.Table, f.Group, f.RKey,
			f.RID, f.IDs, f.Thing, f.Names, f.World, f.Refs, facetVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// reindexAudit works one line's facets out again, after its words changed
// (a revert's "reverts entry N", a ban's reason).
func (s *Server) reindexAudit(id int64) {
	if !s.facetMu.TryLock() {
		return // a run is indexing: it reaches this line, the newest, as it stands
	}
	defer s.facetMu.Unlock()
	// the lines before it first: the index goes on from its largest id
	if err := s.indexAuditLocked(); err != nil {
		log.Printf("GM panel: indexing the audit log: %v", err)
		return
	}
	var r rawAudit
	err := s.d.DB.QueryRow(`SELECT id, at, username, via, action, tbl, rowkey, before, after, detail FROM gm_audit WHERE id = ?`, id).
		Scan(&r.id, &r.at, &r.username, &r.via, &r.action, &r.tbl, &r.key, &r.before, &r.after, &r.detail)
	if err != nil {
		return
	}
	tables, err := s.tables()
	if err != nil {
		return
	}
	fc := &facetCtx{s: s, tables: tables, q: s.d.DB, names: map[string]refName{}}
	if err := s.writeFacets([]facet{fc.facetOf(r)}); err != nil {
		log.Printf("GM panel: could not index audit line %d: %v", id, err)
	}
}

// indexAuditIfFree indexes the new lines unless a run is at it already (the
// first after a start takes a while on a big log): busy says the facets are
// still being made, and a reader answers from those made so far.
func (s *Server) indexAuditIfFree() (busy bool, err error) {
	if !s.facetMu.TryLock() {
		return true, nil
	}
	defer s.facetMu.Unlock()
	return false, s.indexAuditLocked()
}

// indexAuditInBackground indexes what a server start finds, so the first
// reader does not wait for it.
func (s *Server) indexAuditInBackground() {
	go func() {
		if err := s.indexAudit(); err != nil {
			log.Printf("GM panel: indexing the audit log: %v", err)
		}
	}()
}

// impliedRefs are the columns named for what they refer to, in tables the
// catalog does not describe (most a tool made).
var impliedRefs = map[string]string{
	"npcid": "npcs", "npc_row": "npcs", "patternid": "itempattern", "spellid": "spellpattern",
	"serverid": "characters", "charid": "characters", "questid": "quests", "cmid": "cmcatalog",
}

// refOf is the table a column refers to: the catalog's word, else its name's.
func refOf(tables map[string]*Table, t *Table, table, col string) string {
	if t != nil {
		if c := t.col(col); c != nil && c.Ref != "" {
			return c.Ref
		}
	}
	ref := impliedRefs[col]
	if ref == "" || ref == table || tables[ref] == nil {
		return ""
	}
	return ref
}
