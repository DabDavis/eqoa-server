package gmweb

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed guide.md
var guideText string

// The operations that find things out: the guide, the server's state, the
// tables and what their columns mean, search across everything, and fetch of
// one thing whole.

func (s *Server) registerDiscoveryOps() {
	s.addOp(&Op{
		Name: "guide", Title: "Read the GM guide", Scope: scopeRead, Core: true,
		Description: "Read this first. How this EverQuest Online Adventures server's data fits together (accounts, " +
			"characters, NPCs, items, spells, merchants), its conventions (ids, worlds, flags, model DictIDs) and how to " +
			"make common changes safely. Returns markdown.",
		Handler: func(c *opCtx) (any, error) {
			return map[string]any{"guide": guideText}, nil
		},
	})
	s.addOp(&Op{
		Name: "server_status", Title: "Server status", Scope: scopeRead, Core: true,
		Description: "The server now: who you are signed in as and what you may do, players online, NPCs in the " +
			"world, how many accounts, characters, NPC rows, items and spells there are, the build and uptime.",
		Handler: func(c *opCtx) (any, error) { return c.s.status(c.a), nil },
	})
	s.addOp(&Op{
		Name: "list_tables", Title: "List tables", Scope: scopeRead, Core: true,
		Description: "Every table of the server's database, grouped, with its row count, what it is for, what an edit " +
			"to it does in the running game, and who may change it (gm, admin, or none).",
		Handler: func(c *opCtx) (any, error) {
			ts, err := c.s.tableList()
			if err != nil {
				return nil, err
			}
			var out []map[string]any
			for _, t := range ts {
				out = append(out, map[string]any{
					"name": t.Name, "label": t.Label, "group": t.Group, "rows": t.Count,
					"key": t.Key, "help": t.Help, "live": t.Live, "write": t.Write,
				})
			}
			return map[string]any{"tables": out}, nil
		},
	})
	s.addOp(&Op{
		Name: "describe_table", Title: "Describe a table", Scope: scopeRead, Core: true,
		Description: "One table's columns: type, key, what each means, the enumerations (race, class, world...) and " +
			"bit flags (npc_type...) a column takes, and which table a column refers to. Read it before writing to a table.",
		Input: obj(map[string]any{"table": str("the table, as list_tables names it")}, "table"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ Table string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			t, err := c.s.table(req.Table)
			if err != nil {
				return nil, notFound("%v", err)
			}
			return c.s.describe(t), nil
		},
	})
	s.addOp(&Op{
		Name: "search", Title: "Search everything", Scope: scopeRead, Core: true,
		Description: "Find accounts, characters, NPCs, items and spells by name (any part, any case) or by id. Each " +
			"result has an id like \"npc:123\" that fetch takes. NPCs of one name are one result with how many spawn.",
		Input: obj(map[string]any{
			"query": str("a name, part of one, or an id"),
			"kinds": arr(strEnum("", "account", "character", "npc", "item", "spell"), "only these kinds (default all)"),
			"limit": integer("results per kind, default 10, at most 50"),
		}, "query"),
		Handler: func(c *opCtx) (any, error) {
			var req struct {
				Query string
				Kinds []string
				Limit int
			}
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.search(c, req.Query, req.Kinds, req.Limit)
		},
	})
	s.addOp(&Op{
		Name: "fetch", Title: "Fetch one thing whole", Scope: scopeRead, Core: true,
		Description: "One account, character, NPC, item or spell with everything about it, by the id search gives " +
			"(\"character:12\", \"npc:123\", \"item:456\", \"spell:9\", \"account:3\"): a character with its items, " +
			"spells and whether it is online; an NPC with its live state, class and merchant stock; an item with who " +
			"sells and who holds it.",
		Input: obj(map[string]any{"id": str("kind:id, as search returns it")}, "id"),
		Handler: func(c *opCtx) (any, error) {
			var req struct{ ID string }
			if err := c.decode(&req); err != nil {
				return nil, err
			}
			return c.s.fetch(c, req.ID)
		},
	})
}

// status is the server's state for server_status and the dashboard.
func (s *Server) status(a *actor) map[string]any {
	counts := map[string]int64{}
	for name, t := range map[string]string{
		"accounts": "accountinfo", "characters": "characters", "npcRows": "npcs",
		"items": "itempattern", "spells": "spellpattern", "merchantStock": "npcitem", "auditEntries": "gm_audit",
	} {
		var n int64
		if err := s.d.DB.QueryRow(`SELECT COUNT(*) FROM ` + quoteIdent(t)).Scan(&n); err == nil {
			counts[name] = n
		}
	}
	out := map[string]any{
		"you": map[string]any{
			"username": a.Username, "accountid": a.AccountID, "level": a.Level, "admin": a.Admin,
			"scope": a.Scope, "via": a.Via,
		},
		"build":         s.build,
		"started":       stamp(s.started),
		"uptimeSeconds": int64(time.Since(s.started).Seconds()),
		"counts":        counts,
		"worlds":        enums()["world"][:8],
	}
	if s.d.World != nil {
		out["playersOnline"] = s.d.World.PlayerCount()
		out["npcsInWorld"] = s.d.World.NPCCount()
		out["gmSpawns"] = len(s.d.World.TemporaryNPCs())
	}
	if s.d.Sessions != nil {
		out["sessions"] = s.d.Sessions.ActiveCount()
	}
	if s.d.Config != nil {
		if fi, err := os.Stat(s.d.Config.Server.DBPath); err == nil {
			out["databaseBytes"] = fi.Size()
		}
	}
	out["settings"] = s.settingsNow()
	return out
}

// describe is a table's columns with everything the catalog knows.
func (s *Server) describe(t *Table) map[string]any {
	en, fl := enums(), flagSets()
	usedEnums, usedFlags := map[string]any{}, map[string]any{}
	for _, c := range t.Columns {
		if c.Enum != "" {
			usedEnums[c.Enum] = en[c.Enum]
		}
		if c.Flags != "" {
			usedFlags[c.Flags] = fl[c.Flags]
		}
	}
	var cols []Column
	for _, c := range t.Columns {
		if !c.Secret {
			cols = append(cols, c)
		}
	}
	return map[string]any{
		"name": t.Name, "label": t.Label, "group": t.Group, "help": t.Help, "live": t.Live,
		"key": t.Key, "rowid": t.RowID, "title": t.Title, "rows": t.Count, "write": t.Write,
		"columns": cols, "enums": usedEnums, "flags": usedFlags, "show": t.Show, "search": t.Search,
	}
}

// searchResult is one thing search found.
type searchResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Key      any    `json:"key"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`
}

func (s *Server) link(c *opCtx, path string) string {
	if c.r == nil {
		return ""
	}
	return s.publicBase(c.r) + "/#/" + path
}

func (s *Server) search(c *opCtx, q string, kinds []string, limit int) (any, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, badRequest("query is empty")
	}
	if limit <= 0 {
		limit = 10
	}
	limit = min(limit, 50)
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	all := len(want) == 0
	like := "%" + likeEscape(q) + "%"
	id, idErr := strconv.ParseInt(q, 10, 64)
	isID := idErr == nil
	var out []searchResult
	query := func(kind, sqlText string, args []any, row func(scan func(...any) error) (searchResult, error)) error {
		if !all && !want[kind] {
			return nil
		}
		rs, err := s.d.DB.Query(sqlText, args...)
		if err != nil {
			return err
		}
		var found []searchResult
		for rs.Next() {
			r, err := row(rs.Scan)
			if err != nil {
				rs.Close()
				return err
			}
			r.Kind = kind
			found = append(found, r)
		}
		rs.Close()
		out = append(out, found...)
		return rs.Err()
	}
	err := query("account", `SELECT accountid, username, COALESCE(acctlevel, 0), COALESCE(acctstatus, 0) FROM accountinfo
		WHERE username LIKE ? ESCAPE '\' OR accountid = ? ORDER BY accountid LIMIT ?`, []any{like, idOr(isID, id), limit},
		func(scan func(...any) error) (searchResult, error) {
			var aid, lvl, st int64
			var name string
			err := scan(&aid, &name, &lvl, &st)
			sub := "player"
			if lvl >= int64(s.cfg.MinLevel) {
				sub = fmt.Sprintf("GM (level %d)", lvl)
			}
			if st < 0 {
				sub += ", banned"
			}
			return searchResult{ID: fmt.Sprintf("account:%d", aid), Key: aid, Title: name, Subtitle: sub,
				URL: s.link(c, fmt.Sprintf("accounts/%d", aid))}, err
		})
	if err != nil {
		return nil, err
	}
	err = query("character", `SELECT c.serverid, c.charName, c.level, c.tclass, c.race, COALESCE(a.username, '') FROM characters c
		LEFT JOIN accountinfo a ON a.accountid = c.accountid
		WHERE c.charName LIKE ? ESCAPE '\' OR c.serverid = ? ORDER BY c.serverid LIMIT ?`, []any{like, idOr(isID, id), limit},
		func(scan func(...any) error) (searchResult, error) {
			var sid, lvl, cls, race int64
			var name, acct string
			err := scan(&sid, &name, &lvl, &cls, &race, &acct)
			sub := fmt.Sprintf("level %d %s %s, account %s", lvl, raceName(int(race)), className(int(cls)), acct)
			if _, on := s.onlineCharacter(uint32(sid)); on {
				sub += " (online)"
			}
			return searchResult{ID: fmt.Sprintf("character:%d", sid), Key: sid, Title: name, Subtitle: sub,
				URL: s.link(c, fmt.Sprintf("characters/%d", sid))}, err
		})
	if err != nil {
		return nil, err
	}
	err = query("npc", `SELECT MIN(id), npc_name, COUNT(*), MIN(npc_level), MAX(npc_level), MIN(world) FROM npcs
		WHERE npc_name LIKE ? ESCAPE '\' OR id = ? GROUP BY npc_name ORDER BY (npc_name NOT LIKE ? ESCAPE '\'), npc_name LIMIT ?`,
		[]any{like, idOr(isID, id), likeEscape(q) + "%", limit},
		func(scan func(...any) error) (searchResult, error) {
			var nid, n, lo, hi, world int64
			var name string
			err := scan(&nid, &name, &n, &lo, &hi, &world)
			lv := fmt.Sprintf("level %d", lo)
			if hi != lo {
				lv = fmt.Sprintf("levels %d-%d", lo, hi)
			}
			sub := fmt.Sprintf("%s, %s", lv, worldName(int(world)))
			if n > 1 {
				sub = fmt.Sprintf("%d spawns, %s", n, sub)
			}
			return searchResult{ID: fmt.Sprintf("npc:%d", nid), Key: nid, Title: name, Subtitle: sub,
				URL: s.link(c, fmt.Sprintf("npcs/%d", nid))}, err
		})
	if err != nil {
		return nil, err
	}
	err = query("item", `SELECT patternid, itemname, levelreq, equipslot FROM itempattern
		WHERE itemname LIKE ? ESCAPE '\' OR patternid = ? ORDER BY (itemname NOT LIKE ? ESCAPE '\'), itemname LIMIT ?`,
		[]any{like, idOr(isID, id), likeEscape(q) + "%", limit},
		func(scan func(...any) error) (searchResult, error) {
			var pid, lvl, slot int64
			var name string
			err := scan(&pid, &name, &lvl, &slot)
			return searchResult{ID: fmt.Sprintf("item:%d", pid), Key: pid, Title: name,
				Subtitle: fmt.Sprintf("level %d, %s", lvl, enumLabel("wearslot", int(slot))),
				URL:      s.link(c, fmt.Sprintf("items/%d", pid))}, err
		})
	if err != nil {
		return nil, err
	}
	err = query("spell", `SELECT spellid, spellname, abilitylvl FROM spellpattern
		WHERE spellname LIKE ? ESCAPE '\' OR spellid = ? ORDER BY (spellname NOT LIKE ? ESCAPE '\'), spellname LIMIT ?`,
		[]any{like, idOr(isID, id), likeEscape(q) + "%", limit},
		func(scan func(...any) error) (searchResult, error) {
			var sid, lvl int64
			var name string
			err := scan(&sid, &name, &lvl)
			return searchResult{ID: fmt.Sprintf("spell:%d", sid), Key: sid, Title: name,
				Subtitle: fmt.Sprintf("level %d", lvl), URL: s.link(c, fmt.Sprintf("spells/%d", sid))}, err
		})
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []searchResult{}
	}
	return map[string]any{"results": out}, nil
}

// idOr is the id to match when the query is a number, else one no row has.
func idOr(isID bool, id int64) int64 {
	if isID {
		return id
	}
	return -1 << 62
}

func enumLabel(enum string, v int) string {
	for _, e := range enums()[enum] {
		if e.Value == v {
			return e.Label
		}
	}
	return fmt.Sprint(v)
}

// fetch is one thing whole, for fetch and the page's detail views.
func (s *Server) fetch(c *opCtx, ref string) (any, error) {
	kind, idText, ok := strings.Cut(strings.TrimSpace(ref), ":")
	if !ok {
		return nil, badRequest(`id is kind:id, such as "npc:123" (search gives them)`)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(idText), 10, 64)
	if err != nil {
		return nil, badRequest("%q is not a number", idText)
	}
	var rec map[string]any
	var title string
	switch kind {
	case "account":
		rec, err = s.accountOverview(id)
	case "character":
		rec, err = s.characterOverview(id)
	case "npc":
		rec, err = s.npcOverview(id)
	case "item":
		rec, err = s.itemOverview(id)
	case "spell":
		rec, err = s.spellOverview(id)
	default:
		return nil, badRequest("kind is account, character, npc, item or spell")
	}
	if err != nil {
		return nil, err
	}
	title, _ = rec["title"].(string)
	text, _ := json.MarshalIndent(rec, "", "  ")
	return map[string]any{
		"id": ref, "title": title, "text": string(text),
		"url": s.link(c, fmt.Sprintf("%ss/%d", kind, id)), "metadata": map[string]any{"kind": kind},
		"record": rec,
	}, nil
}

// rowOf is one row of a table by a single-column key, or a not-found error.
func (s *Server) rowOf(table string, key string, id any) (map[string]any, error) {
	t, err := s.table(table)
	if err != nil {
		return nil, err
	}
	row, err := getRow(s.d.DB, t, map[string]any{key: id})
	if err != nil {
		return nil, notFound("%v", err)
	}
	delete(row, "__key")
	return row, nil
}

// rowsOf is every row a query gives (at most 1000), keys left out.
func (s *Server) rowsOf(query string, args ...any) ([]map[string]any, error) {
	rs, err := s.d.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	out := []map[string]any{}
	for rs.Next() && len(out) < 1000 {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, cname := range cols {
			m[cname] = fromSQL(vals[i])
		}
		out = append(out, m)
	}
	return out, rs.Err()
}

func (s *Server) accountOverview(id int64) (map[string]any, error) {
	acc, err := s.rowOf("accountinfo", "accountid", id)
	if err != nil {
		return nil, err
	}
	chars, err := s.rowsOf(`SELECT serverid, charName, level, tclass, race, world, x, y, z FROM characters
		WHERE accountid = ? ORDER BY serverid`, id)
	if err != nil {
		return nil, err
	}
	for _, ch := range chars {
		sid, _ := asFloat(ch["serverid"])
		_, on := s.onlineCharacter(uint32(sid))
		ch["online"] = on
		cls, _ := asFloat(ch["tclass"])
		race, _ := asFloat(ch["race"])
		ch["className"], ch["raceName"] = className(int(cls)), raceName(int(race))
	}
	tot, _ := s.totpEnabled(int(id))
	return map[string]any{
		"title": acc["username"], "account": acc, "characters": chars,
		"isGM":          func() bool { l, _ := asFloat(acc["acctlevel"]); return int(l) >= s.cfg.MinLevel }(),
		"banned":        func() bool { st, _ := asFloat(acc["acctstatus"]); return st < 0 }(),
		"authenticator": tot,
	}, nil
}

func (s *Server) characterOverview(id int64) (map[string]any, error) {
	ch, err := s.rowOf("characters", "serverid", id)
	if err != nil {
		return nil, err
	}
	items, err := s.rowsOf(`SELECT c.itemid, c.patternid, p.itemname, c.stackleft, c.remainHP, c.remaincharge,
		c.equiploc, c.location, c.listnumber FROM charinventory c LEFT JOIN itempattern p ON p.patternid = c.patternid
		WHERE c.serverid = ? ORDER BY c.location, c.listnumber, c.itemid`, id)
	if err != nil {
		return nil, err
	}
	spells, err := s.rowsOf(`SELECT s.id, s.spellid, p.spellname, p.abilitylvl, s.addedorder, s.onHotBar, s.whereonBar
		FROM spells s LEFT JOIN spellpattern p ON p.spellid = s.spellid WHERE s.serverid = ? ORDER BY s.addedorder`, id)
	if err != nil {
		return nil, err
	}
	buffs, _ := s.rowsOf(`SELECT seq, spellid, name, left_ms FROM charbuffs WHERE serverid = ? ORDER BY seq`, id)
	acct, _ := s.rowsOf(`SELECT accountid, username, acctlevel, acctstatus FROM accountinfo WHERE accountid = ?`, ch["accountid"])
	out := map[string]any{
		"title": ch["charName"], "character": ch, "items": items, "spells": spells, "effects": buffs,
	}
	if len(acct) == 1 {
		out["account"] = acct[0]
	}
	cls, _ := asFloat(ch["tclass"])
	race, _ := asFloat(ch["race"])
	out["className"], out["raceName"] = className(int(cls)), raceName(int(race))
	w, _ := asFloat(ch["world"])
	out["worldName"] = worldName(int(w))
	if p, on := s.onlineCharacter(uint32(id)); on {
		out["online"] = p
	} else {
		out["online"] = nil
	}
	return out, nil
}

func (s *Server) npcOverview(id int64) (map[string]any, error) {
	npc, err := s.rowOf("npcs", "id", id)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprint(npc["npc_name"])
	out := map[string]any{"title": name, "npc": npc}
	if class, err := s.d.NPCs.NPCClassOf(name); err == nil {
		out["class"], out["className"] = class, className(class)
	}
	var spawns int64
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM npcs WHERE npc_name = ?`, name).Scan(&spawns)
	out["spawnsOfThisName"] = spawns
	w, _ := asFloat(npc["world"])
	out["worldName"] = worldName(int(w))
	typ, _ := asFloat(npc["npc_type"])
	var flags []string
	for _, f := range flagSets()["npctype"] {
		if int(typ)&f.Bit != 0 {
			flags = append(flags, f.Label)
		}
	}
	out["typeFlags"] = flags
	if int(typ)&0x01 != 0 {
		stock, err := s.rowsOf(`SELECT n.itemid, n.patternid, p.itemname, p.itemcost, n.stackleft, n.listnumber FROM npcitem n
			LEFT JOIN itempattern p ON p.patternid = n.patternid WHERE n.npcid = ? ORDER BY n.listnumber, n.itemid`, id)
		if err != nil {
			return nil, err
		}
		out["stock"] = stock
	}
	if s.d.World != nil {
		if st, ok := s.d.World.NPCStateOf(entityID(id)); ok {
			out["live"] = st
		} else {
			out["live"] = nil
		}
	}
	return out, nil
}

func (s *Server) itemOverview(id int64) (map[string]any, error) {
	item, err := s.rowOf("itempattern", "patternid", id)
	if err != nil {
		return nil, err
	}
	sellers, err := s.rowsOf(`SELECT n.npcid, c.npc_name, c.world, n.stackleft FROM npcitem n
		LEFT JOIN npcs c ON c.id = n.npcid WHERE n.patternid = ? ORDER BY n.npcid LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	var held int64
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE patternid = ?`, id).Scan(&held)
	slot, _ := asFloat(item["equipslot"])
	return map[string]any{
		"title": item["itemname"], "item": item, "soldBy": sellers, "heldByCharacters": held,
		"wornIn": enumLabel("wearslot", int(slot)),
	}, nil
}

func (s *Server) spellOverview(id int64) (map[string]any, error) {
	sp, err := s.rowOf("spellpattern", "spellid", id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"title": sp["spellname"], "spell": sp}
	eff, _ := s.rowsOf(`SELECT line, fxdict, source FROM spelleffect WHERE spellid = ?`, id)
	if len(eff) == 1 {
		out["effect"] = eff[0]
	}
	classes, _ := s.rowsOf(`SELECT class FROM classspells WHERE spellid = ? ORDER BY class`, id)
	var names []string
	for _, c := range classes {
		v, _ := asFloat(c["class"])
		names = append(names, className(int(v)))
	}
	sort.Strings(names)
	out["classes"] = names
	var known int64
	s.d.DB.QueryRow(`SELECT COUNT(*) FROM spells WHERE spellid = ?`, id).Scan(&known)
	out["knownByCharacters"] = known
	return out, nil
}
