package gmweb

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The quests as a GM looks over hundreds of them (tools/import_quests.py makes
// 746 out of EQOA-Data's city guides, a draft each where a name is missing):
// every quest with its city, classes and levels, and why a draft is one,
// worked out from the database as it is NOW, not from what the import said,
// so a draft a GM has since mended (the NPC placed, the item made) shows as
// ready to go in. One read of each table, not a query a name. A kill or loot
// step's names are its target and its other creatures (queststeptargets),
// each looked for.
//
// A quest's status: "in the game" (enabled), "ready" (a draft nothing keeps
// out: every name it uses is here, every quest it requires in the game or
// ready itself), or "draft" (with its reasons). An enabled quest with a
// reason is still in the game, the reason a warning (it cannot be finished).
// The later guides' fields count as their names do: a faction a standing is
// asked with, a class's reward items and spells, a rewards-by-class column
// that cannot be read (a class id there is not), a choice's options' items
// and spells and a choice that cannot be asked, a trip it unlocks that no
// route is or whose NPC is not here.

// Quest statuses.
const (
	questInGame = "in the game"
	questReady  = "ready"
	questDraft  = "draft"
)

// questRow is one quest as the list shows it.
type questRow struct {
	ID       int64    `json:"questid"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Status   string   `json:"status"`
	Giver    string   `json:"giver"`
	MinLevel int      `json:"minlevel"`
	MaxLevel int      `json:"maxlevel"`
	Classes  []int    `json:"classes"`
	Races    []int    `json:"races,omitempty"`
	City     string   `json:"city,omitempty"`
	Key      string   `json:"key,omitempty"`
	Source   string   `json:"source,omitempty"`
	Steps    int      `json:"steps"`
	UnderWay int      `json:"underway"`
	Finished int      `json:"finished"`
	Requires []int64  `json:"requires,omitempty"`
	Why      []string `json:"why,omitempty"`

	groups  [][]int64   // what it requires by group (database.ParseRequires): any one of a group, every group
	missing []questMiss // the names behind Why, for the summary
	notes   string
	targets []string // the NPCs its talk, show and give steps send players to, in order (the map's givers)
}

// questMiss is a name a quest uses that this server lacks.
type questMiss struct {
	Kind string `json:"kind"` // NPC, creature, item, spell, faction
	Name string `json:"name"`
}

// questFilter narrows the list (quest_list, quest_set_enabled).
type questFilter struct {
	Search   string `json:"search"`
	Status   string `json:"status"`   // "", "in the game", "draft" (every draft), "ready", "blocked" (a draft not ready)
	City     string `json:"city"`     // the city a quest's import key names ("freeport"); "none" for the rest
	Class    *int   `json:"class"`    // a class id the quest is for (or any class's)
	Level    int    `json:"level"`    // a level its range takes in
	MinLevel int    `json:"minLevel"` // its range meeting [minLevel, maxLevel]
	MaxLevel int    `json:"maxLevel"`
	Giver    string `json:"giver"`   // part of its giver's name
	Missing  string `json:"missing"` // a name it lacks ("Maryan")
}

// reQuestKey is the import's key in a quest's source ("EQOA-Data
// 1-20/freeport/warrior/L3 (tools/import_quests.py)").
var reQuestKey = regexp.MustCompile(`^EQOA-Data (\S+) \(tools/import_quests\.py\)`)

// reNoteName is an entry of the import's notes that names a missing thing:
// "the file's unresolved 'X'" (the content's own list).
var reNoteName = regexp.MustCompile(`the file's unresolved (?:'([^']*)'|"([^"]*)")`)

// questCity is the city of an import key ("1-20/freeport/warrior/L3" is
// freeport's), or of a later guide's ("<EQOA-Data folder>/<guide>/<part>")
// its category as its content file's city names it ("Misc/kratas/p1" misc,
// "30,40,49/group-1-good/p1" 30-40-49, "TankExemplar/warrior/p1"
// tank-exemplar), or "".
func questCity(key string) string {
	parts := strings.Split(key, "/")
	switch {
	case parts[0] == "1-20":
		if len(parts) >= 3 {
			return parts[1]
		}
	case len(parts) >= 2 && parts[0] != "":
		return categorySlug(parts[0])
	}
	return ""
}

// categorySlug is an EQOA-Data folder's name as a slug: lower case, a hyphen
// between words ("HuntAndGather" hunt-and-gather) and for a comma
// ("30,40,49" 30-40-49).
func categorySlug(folder string) string {
	var b strings.Builder
	lower := false
	for _, r := range strings.TrimSpace(folder) {
		switch {
		case r == ',' || r == ' ' || r == '_':
			b.WriteRune('-')
			lower = false
		case unicode.IsUpper(r):
			if lower {
				b.WriteRune('-')
			}
			b.WriteRune(unicode.ToLower(r))
			lower = false
		default:
			b.WriteRune(r)
			lower = unicode.IsLower(r)
		}
	}
	return b.String()
}

// foldQuestName is a name as the import matched it: case, apostrophes and
// spaces aside.
func foldQuestName(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("’", "", "`", "", "'", "").Replace(s)
	var b strings.Builder
	for _, r := range s {
		if r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || r > 127 {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// questNames is what the database holds, read once.
type questNames struct {
	npcs        map[string]npcKind // by lower name
	npcFold     map[string]npcKind // by folded name
	items       map[int64]bool
	itemFold    map[string]bool
	spells      map[int64]bool
	spellFold   map[string]bool
	factions    map[string]bool
	factionFold map[string]bool
}

type npcKind struct {
	attackable bool // a spawn without the NPC bit 0x80
}

func (s *Server) loadQuestNames() (*questNames, error) {
	n := &questNames{npcs: map[string]npcKind{}, npcFold: map[string]npcKind{}, items: map[int64]bool{},
		itemFold: map[string]bool{}, spells: map[int64]bool{}, spellFold: map[string]bool{}, factions: map[string]bool{},
		factionFold: map[string]bool{}}
	scan := func(q string, each func(*sql.Rows) error) error {
		rs, err := s.d.DB.Query(q)
		if err != nil {
			return err
		}
		defer rs.Close()
		for rs.Next() {
			if err := each(rs); err != nil {
				return err
			}
		}
		return rs.Err()
	}
	if err := scan(`SELECT npc_name, COALESCE(npc_type, 0) FROM npcs WHERE npc_name IS NOT NULL`, func(rs *sql.Rows) error {
		var name string
		var t int64
		if err := rs.Scan(&name, &t); err != nil {
			return err
		}
		for i, m := range []map[string]npcKind{n.npcs, n.npcFold} {
			k := strings.ToLower(strings.TrimSpace(name))
			if i == 1 {
				k = foldQuestName(name)
			}
			cur := m[k]
			cur.attackable = cur.attackable || t&0x80 == 0
			m[k] = cur
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := scan(`SELECT patternid, itemname FROM itempattern`, func(rs *sql.Rows) error {
		var id int64
		var name string
		if err := rs.Scan(&id, &name); err != nil {
			return err
		}
		n.items[id], n.itemFold[foldQuestName(name)] = true, true
		return nil
	}); err != nil {
		return nil, err
	}
	if err := scan(`SELECT spellid, spellname FROM spellpattern`, func(rs *sql.Rows) error {
		var id int64
		var name sql.NullString
		if err := rs.Scan(&id, &name); err != nil {
			return err
		}
		n.spells[id] = true
		if name.Valid {
			n.spellFold[foldQuestName(name.String)] = true
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// factions is the server's own table; a database without it has none
	_ = scan(`SELECT name FROM factions`, func(rs *sql.Rows) error {
		var name string
		if err := rs.Scan(&name); err != nil {
			return err
		}
		n.factions[name], n.factionFold[foldQuestName(name)] = true, true
		return nil
	})
	return n, nil
}

// questIndex is every quest with its status and reasons, by id order.
type questIndex struct {
	rows []*questRow
	byID map[int64]*questRow
}

// loadQuestIndex reads every quest and its steps and works out each one's
// status from the database as it is.
func (s *Server) loadQuestIndex() (*questIndex, error) {
	names, err := s.loadQuestNames()
	if err != nil {
		return nil, err
	}
	quests, err := s.everyRow(`SELECT questid, name, enabled, giver, minlevel, maxlevel, classes, races, requires, source, notes,
		factionreq FROM quests ORDER BY questid`)
	if err != nil {
		return nil, err
	}
	// the trips quests unlock (a database without the table has none)
	trips, _ := s.everyRow(`SELECT questid, npc, dest FROM questunlocks ORDER BY questid, npc, dest`)
	steps, err := s.everyRow(`SELECT * FROM queststeps ORDER BY questid, step`)
	if err != nil {
		return nil, err
	}
	// the kill and loot steps' other creatures (a database without the table has none)
	others := map[[2]int64][]string{}
	if rows, err := s.everyRow(`SELECT questid, step, target FROM queststeptargets ORDER BY questid, step, rowid`); err == nil {
		for _, o := range rows {
			k := [2]int64{0, 0}
			if v, ok := asFloat(o["questid"]); ok {
				k[0] = int64(v)
			}
			if v, ok := asFloat(o["step"]); ok {
				k[1] = int64(v)
			}
			if t, _ := o["target"].(string); strings.TrimSpace(t) != "" {
				others[k] = append(others[k], strings.TrimSpace(t))
			}
		}
	}
	progress, err := s.everyRow(`SELECT questid, SUM(CASE WHEN step > 0 THEN 1 ELSE 0 END) AS underway,
		SUM(CASE WHEN done > 0 THEN 1 ELSE 0 END) AS finished FROM charquests GROUP BY questid`)
	if err != nil {
		return nil, err
	}
	text := func(r map[string]any, col string) string { v, _ := r[col].(string); return strings.TrimSpace(v) }
	num := func(r map[string]any, col string) int64 { v, _ := asFloat(r[col]); return int64(v) }
	idx := &questIndex{byID: map[int64]*questRow{}}
	for _, q := range quests {
		r := &questRow{ID: num(q, "questid"), Name: text(q, "name"), Enabled: num(q, "enabled") != 0,
			Giver: text(q, "giver"), MinLevel: int(num(q, "minlevel")), MaxLevel: int(num(q, "maxlevel")),
			Classes: database.ParseIntList(text(q, "classes")), Races: database.ParseIntList(text(q, "races")),
			Source: text(q, "source"), notes: text(q, "notes")}
		for _, id := range database.ParseIntList(text(q, "requires")) {
			r.Requires = append(r.Requires, int64(id))
		}
		for _, g := range database.ParseRequires(text(q, "requires")) {
			ids := make([]int64, len(g))
			for i, id := range g {
				ids[i] = int64(id)
			}
			r.groups = append(r.groups, ids)
		}
		if m := reQuestKey.FindStringSubmatch(r.Source); m != nil {
			r.Key, r.City = m[1], questCity(m[1])
		}
		if r.Classes == nil {
			r.Classes = []int{}
		}
		idx.rows = append(idx.rows, r)
		idx.byID[r.ID] = r
	}
	for _, p := range progress {
		if r := idx.byID[num(p, "questid")]; r != nil {
			r.UnderWay, r.Finished = int(num(p, "underway")), int(num(p, "finished"))
		}
	}
	// the names each quest uses
	for _, r := range idx.rows {
		if r.Giver == "" {
			r.why("it has no giver: no one offers it")
		} else if _, ok := names.npcs[strings.ToLower(r.Giver)]; !ok {
			r.lack("NPC", r.Giver, "no NPC %q (its giver)", r.Giver)
		}
		// a giver with a window (merchant, bank or blacksmith) offers at its
		// window, first (handlers.QuestEngine.AtWindow)
		for _, m := range reNoteName.FindAllStringSubmatch(r.notes, -1) {
			name := m[1] + m[2]
			bare := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "(attackable)"))
			k, isNPC := names.npcFold[foldQuestName(bare)]
			switch {
			case strings.HasSuffix(strings.TrimSpace(name), "(attackable)") && isNPC && !k.attackable:
				r.lack("NPC", bare, "the content's note: %q is to be attacked, and every one carries the NPC bit", bare)
			case strings.HasSuffix(strings.TrimSpace(name), "(attackable)"):
				if !isNPC {
					r.lack("NPC", bare, "the content's note: no %q", bare)
				}
			case !isNPC && !names.itemFold[foldQuestName(bare)] && !names.spellFold[foldQuestName(bare)] &&
				!names.factionFold[foldQuestName(bare)]:
				r.lack("NPC", bare, "the content's note: no %q", bare)
			}
		}
	}
	// the standings asked for, and the trips unlocked
	for _, q := range quests {
		r := idx.byID[num(q, "questid")]
		for _, p := range database.FactionReqProblems(text(q, "factionreq")) {
			r.why("%s", p)
		}
		for _, f := range database.ParseFactionReqs(text(q, "factionreq")) {
			if len(names.factions) > 0 && !names.factions[f.Faction] {
				r.lack("faction", f.Faction, "no faction %q (a standing it asks for)", f.Faction)
			}
		}
	}
	for _, t := range trips {
		r := idx.byID[num(t, "questid")]
		if r == nil {
			continue
		}
		npc, dest := text(t, "npc"), text(t, "dest")
		if _, ok := names.npcs[strings.ToLower(npc)]; !ok {
			r.lack("NPC", npc, "no NPC %q (whose trip to %s it unlocks)", npc, dest)
		}
		if _, ok := handlers.RouteOf(npc, dest); !ok {
			r.why("it unlocks %s's trip to %s, which is no route", npc, dest)
		}
	}
	for _, st := range steps {
		r := idx.byID[num(st, "questid")]
		if r == nil {
			continue
		}
		r.Steps++
		n := num(st, "step")
		kind := strings.ToLower(text(st, "kind"))
		target := text(st, "target")
		switch kind {
		case database.StepTalk, database.StepShow, database.StepGive:
			// an NPC with a window is fine: the step is done when its window
			// is asked for (handlers.QuestEngine.AtWindow)
			if _, ok := names.npcs[strings.ToLower(target)]; !ok {
				r.lack("NPC", target, "no NPC %q (step %d)", target, n)
			}
			if target != "" && !containsString(r.targets, target) {
				r.targets = append(r.targets, target)
			}
		case database.StepKill, database.StepLoot:
			for _, c := range append([]string{target}, others[[2]int64{r.ID, n}]...) {
				if k, ok := names.npcs[strings.ToLower(c)]; !ok {
					r.lack("creature", c, "no creature %q (step %d)", c, n)
				} else if !k.attackable {
					r.lack("creature", c, "every %q carries the NPC bit, so none can be attacked (step %d)", c, n)
				}
			}
		case database.StepReach:
		default:
			r.why("step %d is of no kind the engine has (%q)", n, kind)
		}
		for _, it := range database.ParseItemCounts(text(st, "items")) {
			if !names.items[int64(it.PatternID)] {
				r.lack("item", fmt.Sprint(it.PatternID), "no item %d (step %d)", it.PatternID, n)
			}
		}
		for _, it := range database.ParseItemCounts(text(st, "rewarditems")) {
			if !names.items[int64(it.PatternID)] {
				r.lack("item", fmt.Sprint(it.PatternID), "no item %d (step %d's reward)", it.PatternID, n)
			}
		}
		for _, sp := range database.ParseIntList(text(st, "spells")) {
			if !names.spells[int64(sp)] {
				r.lack("spell", fmt.Sprint(sp), "no spell %d (step %d's reward)", sp, n)
			}
		}
		for _, f := range database.ParseFactionDeltas(text(st, "factions")) {
			if len(names.factions) > 0 && !names.factions[f.Faction] {
				r.lack("faction", f.Faction, "no faction %q (step %d's reward)", f.Faction, n)
			}
		}
		// the rewards by class: readable, real classes, items and spells here
		for _, p := range database.ClassRewardProblems(text(st, "byclass")) {
			r.why("step %d's %s", n, p)
		}
		for _, p := range database.ChoiceProblems(text(st, "choices"), kind) {
			r.why("step %d: %s", n, p)
		}
		if ch, err := database.ParseStepChoices(text(st, "choices")); err == nil {
			for k, o := range ch.Options {
				for _, it := range o.Reward.Items {
					if !names.items[int64(it.PatternID)] {
						r.lack("item", fmt.Sprint(it.PatternID), "no item %d (step %d's option %d)", it.PatternID, n, k+1)
					}
				}
				for _, sp := range o.Reward.Spells {
					if !names.spells[int64(sp)] {
						r.lack("spell", fmt.Sprint(sp), "no spell %d (step %d's option %d)", sp, n, k+1)
					}
				}
			}
		}
		if byClass, err := database.ParseClassRewards(text(st, "byclass")); err == nil {
			for _, class := range sortedKeys(byClass) {
				cr := byClass[class]
				for _, it := range cr.Items {
					if !names.items[int64(it.PatternID)] {
						r.lack("item", fmt.Sprint(it.PatternID), "no item %d (step %d's reward for class %d)", it.PatternID, n, class)
					}
				}
				for _, sp := range cr.Spells {
					if !names.spells[int64(sp)] {
						r.lack("spell", fmt.Sprint(sp), "no spell %d (step %d's reward for class %d)", sp, n, class)
					}
				}
			}
		}
		evs, _ := database.ParseQuestEvents(text(st, "events"))
		for _, e := range evs {
			switch e.Do {
			case database.EventSay, database.EventSpawn:
				if who := strings.TrimSpace(e.NPC); who != "" {
					if _, ok := names.npcs[strings.ToLower(who)]; !ok {
						r.lack("NPC", who, "no NPC %q (step %d's %s)", who, n, e.Do)
					}
				}
			case database.EventGive:
				for _, it := range database.ParseItemCounts(e.Items) {
					if !names.items[int64(it.PatternID)] {
						r.lack("item", fmt.Sprint(it.PatternID), "no item %d (step %d's give)", it.PatternID, n)
					}
				}
			}
		}
	}
	for _, r := range idx.rows {
		if r.Steps == 0 {
			r.why("it has no steps")
		}
	}
	// a draft nothing of its own keeps out is ready when every quest it
	// requires is in the game or ready itself (a chain goes in together)
	blocked := map[int64]bool{}
	for _, r := range idx.rows {
		blocked[r.ID] = len(r.Why) > 0
	}
	// a quest it requires keeps it out when it does not exist or is a draft
	// that is not ready; a group of alternatives only when every one of them does
	out := func(req int64) bool {
		q := idx.byID[req]
		return q == nil || (!q.Enabled && blocked[req])
	}
	groupOut := func(g []int64) bool {
		for _, req := range g {
			if !out(req) {
				return false
			}
		}
		return true
	}
	for changed := true; changed; {
		changed = false
		for _, r := range idx.rows {
			if blocked[r.ID] {
				continue
			}
			for _, g := range r.groups {
				if groupOut(g) {
					blocked[r.ID], changed = true, true
					break
				}
			}
		}
	}
	for _, r := range idx.rows {
		for _, g := range r.groups {
			if !groupOut(g) {
				continue
			}
			for _, req := range g {
				switch q := idx.byID[req]; {
				case q == nil:
					r.why("it requires quest %d, which does not exist", req)
				case !q.Enabled && blocked[req]:
					r.why("it requires quest %d (%s), a draft that is not ready", req, q.Name)
				}
			}
		}
		switch {
		case r.Enabled:
			r.Status = questInGame
		case blocked[r.ID]:
			r.Status = questDraft
		default:
			r.Status = questReady
		}
	}
	return idx, nil
}

func (r *questRow) why(format string, a ...any) {
	r.Why = append(r.Why, fmt.Sprintf(format, a...))
}

func (r *questRow) lack(kind, name, format string, a ...any) {
	r.why(format, a...)
	if name != "" {
		r.missing = append(r.missing, questMiss{Kind: kind, Name: name})
	}
}

// match is the quests the filter takes, in id order.
func (idx *questIndex) match(f questFilter) []*questRow {
	search := strings.ToLower(strings.TrimSpace(f.Search))
	giver := strings.ToLower(strings.TrimSpace(f.Giver))
	missing := foldQuestName(f.Missing)
	var out []*questRow
	for _, r := range idx.rows {
		switch f.Status {
		case "":
		case "draft", "drafts":
			if r.Enabled {
				continue
			}
		case "blocked":
			if r.Status != questDraft {
				continue
			}
		case questReady, questInGame:
			if r.Status != f.Status {
				continue
			}
		case "ingame", "enabled":
			if !r.Enabled {
				continue
			}
		default:
			continue
		}
		if f.City != "" && !(f.City == "none" && r.City == "") && !strings.EqualFold(f.City, r.City) {
			continue
		}
		if f.Class != nil && len(r.Classes) > 0 && !containsIntList(r.Classes, *f.Class) {
			continue
		}
		hi := r.MaxLevel
		if hi <= 0 {
			hi = 1 << 20
		}
		if f.Level > 0 && (f.Level < r.MinLevel || f.Level > hi) {
			continue
		}
		if f.MinLevel > 0 && hi < f.MinLevel {
			continue
		}
		if f.MaxLevel > 0 && r.MinLevel > f.MaxLevel {
			continue
		}
		if giver != "" && !strings.Contains(strings.ToLower(r.Giver), giver) {
			continue
		}
		if missing != "" {
			found := false
			for _, m := range r.missing {
				if foldQuestName(m.Name) == missing {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Name), search) && !strings.Contains(strings.ToLower(r.Giver), search) &&
			!strings.Contains(strings.ToLower(r.notes), search) && !strings.Contains(strings.ToLower(r.Key), search) &&
			fmt.Sprint(r.ID) != search {
			continue
		}
		out = append(out, r)
	}
	return out
}

func containsIntList(v []int, n int) bool {
	for _, x := range v {
		if x == n {
			return true
		}
	}
	return false
}

// questCityCount is a city's quests, as the list's city choice offers them.
type questCityCount struct {
	City   string `json:"city"`
	Quests int    `json:"quests"`
	InGame int    `json:"inGame"`
	Ready  int    `json:"ready"`
	Drafts int    `json:"drafts"`
}

// cities is every city with its counts, the quests of none ("") last.
func (idx *questIndex) cities() []questCityCount {
	by := map[string]*questCityCount{}
	for _, r := range idx.rows {
		c := by[r.City]
		if c == nil {
			c = &questCityCount{City: r.City}
			by[r.City] = c
		}
		c.Quests++
		switch r.Status {
		case questInGame:
			c.InGame++
		case questReady:
			c.Ready++
		default:
			c.Drafts++
		}
	}
	var out []questCityCount
	for _, c := range by {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].City == "") != (out[j].City == "") {
			return out[j].City == ""
		}
		return out[i].City < out[j].City
	})
	return out
}

// questMissCount is a name drafts lack, and how many.
type questMissCount struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Quests int    `json:"quests"`
}

// missing is what the given drafts lack, the most wanted first: a name once
// whatever it was lacked as (a step's creature, the content's note), its
// kinds joined.
func missingOf(rows []*questRow) []questMissCount {
	type entry struct {
		name  string
		kinds []string
		n     int
	}
	by := map[string]*entry{}
	for _, r := range rows {
		if r.Enabled {
			continue
		}
		seen := map[string]bool{}
		for _, m := range r.missing {
			k := foldQuestName(m.Name)
			e := by[k]
			if e == nil {
				e = &entry{name: m.Name}
				by[k] = e
			}
			if !containsString(e.kinds, m.Kind) {
				e.kinds = append(e.kinds, m.Kind)
			}
			if !seen[k] {
				seen[k] = true
				e.n++
			}
		}
	}
	out := make([]questMissCount, 0, len(by))
	for _, e := range by {
		sort.Strings(e.kinds)
		out = append(out, questMissCount{Kind: strings.Join(e.kinds, "/"), Name: e.name, Quests: e.n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Quests != out[j].Quests {
			return out[i].Quests > out[j].Quests
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func containsString(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

// statusCounts is how many of the rows are in the game, ready and drafts.
func statusCounts(rows []*questRow) map[string]int {
	out := map[string]int{questInGame: 0, questReady: 0, questDraft: 0}
	for _, r := range rows {
		out[r.Status]++
	}
	return out
}

// empty is whether the filter takes every quest.
func (f questFilter) empty() bool {
	return f.Search == "" && f.Status == "" && f.City == "" && f.Class == nil && f.Level == 0 && f.MinLevel == 0 &&
		f.MaxLevel == 0 && f.Giver == "" && f.Missing == ""
}

// everyRow is rowsOf without its cap of a thousand rows: the index reads every
// quest and every step (746 quests have 3,604).
func (s *Server) everyRow(query string, args ...any) ([]map[string]any, error) {
	rs, err := s.d.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	out := []map[string]any{}
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
