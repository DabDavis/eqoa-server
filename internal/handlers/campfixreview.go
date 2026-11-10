package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Camp fix reports under review (the GM panel's Camp reports page,
// gmweb/ops_campfix.go; the owner, 2026-11-10: "lets require GM approval"): a
// report waits until a GM approves some or all of its marks or declines it,
// and nothing is applied before that. This is what the page shows a GM of a
// waiting report: each mark with what applying it does now (CampfixPlan), or
// why it cannot (stale: its row moved, was changed or deleted since the report
// was sent), and the review rules' colour as guidance (the server's
// CLAUDE.md, "Camp fix reports: how they are reviewed"): red a mark the GM
// decides on its own facts, unchecked by default; yellow one to look at;
// green one the rules call safe. It is worked out from the database as it is
// now and, when a world runs, from its ground, its towns and what attacks on
// sight. Nothing here writes: the panel applies a GM's choice through its own
// audited row edits, so each change is revertible and the world follows it.

// The review's figures (the review rules').
const (
	// CampfixFloorReach: an add's or a move's height is the ground's highest
	// floor within this of the mark's (entity.CampfixFloorNear), the mark's
	// own with none that near
	CampfixFloorReach float32 = 2.5
	// campfixStacked: a removal of a spawn within this of another of its name
	// is of a stacked duplicate; no add or move sets one within it of another
	campfixStacked float32 = 3
	// campfixLongRespawn: a name one of whose spawns comes back after more
	// seconds than this is rare
	campfixLongRespawn = 600
	// campfixDropAbove: an item more levels above its creature than this
	// makes it valuable
	campfixDropAbove = 5
	// campfixManyAdds: more adds than this in one report are each red
	campfixManyAdds = 5
)

// The colours of the review rules.
const (
	CampfixRed    = "red"
	CampfixYellow = "yellow"
	CampfixGreen  = "green"
)

// CampfixDeclinedAbuse begins the review line of a report declined as abuse:
// every mark of the account's later reports is red.
const CampfixDeclinedAbuse = "Declined as abuse"

// campfixBiomeTables are tools/seed_camp_loot.py's tables: a creature bound
// to these alone rolls a camp's ordinary loot.
var campfixBiomeTables = map[string]bool{"arctic": true, "desert": true, "forest": true, "marine": true, "mountain": true,
	"plains": true, "swamp": true, "rathe mountains": true, "red desert": true, "wetlands": true, "deep jungle": true,
	"wastelands": true, "generic": true}

// CampfixReview is a report under review: each mark with its plan, its
// colour and its reasons (a waiting report's; a decided one's marks are only
// read back), the creatures it recorded near and its flags read back, what the
// review says of the whole report, the petition it was filed from (0 for a
// player's own /campfix), and how many marks are of each colour.
type CampfixReview struct {
	Marks    []CampfixMarkReview `json:"marks"`
	Near     []CampfixNearRecord `json:"near"`
	Flags    []CampfixFlagRecord `json:"flags"`
	Notes    []string            `json:"notes,omitempty"`
	Petition int64               `json:"petition,omitempty"`
	Counts   map[string]int      `json:"counts"`
}

// CampfixMarkReview is one mark under review: its number (from 1), the mark
// as filed, the report's flags on it, its colour (empty for a stale one) with
// the reasons the rules gave, why it cannot be applied (Stale), whether the
// page checks it to begin with (neither red nor stale), and what applying it
// does now.
type CampfixMarkReview struct {
	N int `json:"n"`
	CampfixMarkRecord
	Flags   []CampfixFlagRecord `json:"flags"`
	Colour  string              `json:"colour,omitempty"`
	Reasons []string            `json:"reasons"`
	// Red, Yellow and Green are every rule that holds, by its colour: the
	// colour is the worst of them, Reasons its list
	Red     []string    `json:"red,omitempty"`
	Yellow  []string    `json:"yellow,omitempty"`
	Green   []string    `json:"green,omitempty"`
	Stale   string      `json:"stale,omitempty"`
	Checked bool        `json:"checked"`
	Plan    CampfixPlan `json:"plan"`
}

// The plan's actions: a row changed (a move, a turn), a row added (an add's
// copy), a row deleted (a removal), or nothing to change.
const (
	CampfixUpdate = "update"
	CampfixInsert = "insert"
	CampfixDelete = "delete"
	CampfixNone   = "none"
)

// CampfixPlan is what applying a mark does now: its action on which npcs row
// (an add's: the row it copies), the place and the facing byte it writes (a
// move's and an add's height on the ground, Ground saying how), the camp spawn
// row an add copies or a removal deletes with it, whether a removal deletes a
// path too, whom the NPC fights now (a change makes it anew, whole, on its
// spawn), what it does in words, and the row's place and facing as read (Was:
// what an update expects to find, else it is refused as changed meanwhile).
type CampfixPlan struct {
	Action   string        `json:"action,omitempty"`
	Row      int           `json:"row,omitempty"`
	Template int           `json:"template,omitempty"`
	X        float64       `json:"x"`
	Y        float64       `json:"y"`
	Z        float64       `json:"z"`
	Facing   int           `json:"facing"`
	Ground   string        `json:"ground,omitempty"`
	Camp     int64         `json:"camp,omitempty"`
	Kinds    []string      `json:"kinds,omitempty"`
	Path     bool          `json:"path,omitempty"`
	Fighting string        `json:"fighting,omitempty"`
	Words    string        `json:"words"`
	Was      *CampfixPlace `json:"was,omitempty"`
}

// CampfixPlace is a row's spawn and facing byte as read, and which of those
// columns hold NULL (read as 0).
type CampfixPlace struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Z      float64  `json:"z"`
	Facing int      `json:"facing"`
	Null   []string `json:"null,omitempty"`
}

// CampfixReviewer reviews reports, reading the other tables the rules read
// once for all of them, and each world's rows of a name once (a report's own
// rows round its marks are its own read).
type CampfixReviewer struct {
	db    *database.SQLiteDB
	gw    *entity.World
	facts *campfixFacts
	names map[uint8]map[string][]campfixRowNPC
	drops map[string]string
}

// NewCampfixReviewer reviews against db and, when gw is not nil, the world
// running on it.
func NewCampfixReviewer(db *database.SQLiteDB, gw *entity.World) *CampfixReviewer {
	return &CampfixReviewer{db: db, gw: gw, names: map[uint8]map[string][]campfixRowNPC{}, drops: map[string]string{}}
}

func (rv *CampfixReviewer) loadFacts() (*campfixFacts, error) {
	if rv.facts == nil {
		f, err := loadCampfixFacts(rv.db)
		if err != nil {
			return nil, err
		}
		rv.facts = f
	}
	return rv.facts, nil
}

// rowsOf is world's rows round pts, the world's rows of a name read before
// kept.
func (rv *CampfixReviewer) rowsOf(world uint8, pts [][3]float32) (*campfixRows, error) {
	f, err := rv.loadFacts()
	if err != nil {
		return nil, err
	}
	if rv.names[world] == nil {
		rv.names[world] = map[string][]campfixRowNPC{}
	}
	return loadCampfixRows(rv.db, world, f.spawnOnly, pts, campfixRowsReach, rv.names[world])
}

var campfixFromPetition = regexp.MustCompile(`^From petition (\d+)\b`)

// CampfixPetitionOf is the petition a report was filed from, read off its
// note (PlanCampfixPetition's), 0 for none.
func CampfixPetitionOf(note string) int64 {
	if m := campfixFromPetition.FindStringSubmatch(note); m != nil {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		return id
	}
	return 0
}

// campfixReviewing is one report's review as it is worked out.
type campfixReviewing struct {
	rv         *CampfixReviewer
	r          database.CampfixReport
	world      uint8
	rows       *campfixRows
	facts      *campfixFacts
	created    time.Time
	marks      []CampfixMarkRecord
	flags      []CampfixFlagRecord
	camp       map[string]int // a name's spawns within campfixSpan of the marks, by campfixKey
	adds       map[string]int // the report's adds of a name
	removes    map[string]int // and its removals
	totalAdds  int
	removed    map[int]bool // the rows it removes
	median     float64
	haveMedian bool
	abuse      []int64
	combos     [][3]int
	markers    []CampMarker
}

// Review is r under review now (CampfixReview).
func (rv *CampfixReviewer) Review(r database.CampfixReport) (CampfixReview, error) {
	out := CampfixReview{Petition: CampfixPetitionOf(r.Note), Counts: map[string]int{}, Near: []CampfixNearRecord{},
		Flags: []CampfixFlagRecord{}, Marks: []CampfixMarkReview{}}
	var marks []CampfixMarkRecord
	if err := json.Unmarshal([]byte(r.Marks), &marks); err != nil {
		out.Notes = append(out.Notes, "Its marks are not readable ("+err.Error()+"): nothing of it can be applied.")
		return out, nil
	}
	if err := json.Unmarshal([]byte(r.Near), &out.Near); err != nil || out.Near == nil {
		out.Near = []CampfixNearRecord{}
	}
	if err := json.Unmarshal([]byte(r.Flags), &out.Flags); err != nil || out.Flags == nil {
		out.Flags = []CampfixFlagRecord{}
	}
	flagsOf := func(n int) []CampfixFlagRecord {
		fl := []CampfixFlagRecord{}
		for _, f := range out.Flags {
			if f.Mark == n {
				fl = append(fl, f)
			}
		}
		return fl
	}
	if r.Status != database.CampfixWaiting {
		for i, m := range marks {
			out.Marks = append(out.Marks, CampfixMarkReview{N: i + 1, CampfixMarkRecord: m, Flags: flagsOf(i + 1), Reasons: []string{}})
		}
		return out, nil
	}
	c := &campfixReviewing{rv: rv, r: r, world: uint8(r.World), marks: marks, flags: out.Flags,
		camp: map[string]int{}, adds: map[string]int{}, removes: map[string]int{}, removed: map[int]bool{}}
	var err error
	if c.facts, err = rv.loadFacts(); err != nil {
		return out, err
	}
	if c.created, err = time.Parse(time.RFC3339, r.Created); err != nil {
		c.created = time.Time{}
		out.Notes = append(out.Notes, "When it was sent is not readable: changes to its rows since are looked for by their place and facing alone, not in the audit log.")
	}
	if rv.gw == nil {
		out.Notes = append(out.Notes, "No world runs here: the ground, the towns and what attacks on sight are not looked at, and each add or move keeps its mark's height.")
	}
	if out.Petition != 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("Filed from petition %d: its marks are where the character stood when it filed the petition.", out.Petition))
	}
	if c.abuse, err = rv.abuseOf(r); err != nil {
		return out, err
	}
	if len(c.abuse) > 0 {
		out.Notes = append(out.Notes, fmt.Sprintf("Account %d had an earlier report declined as abuse (%s): every mark is red.",
			r.AccountID, campfixIDs(c.abuse)))
	}
	pts := make([][3]float32, len(marks))
	for i, m := range marks {
		pts[i] = [3]float32{float32(m.X), float32(m.Y), float32(m.Z)}
	}
	if c.rows, err = rv.rowsOf(c.world, pts); err != nil {
		return out, err
	}
	for _, m := range marks {
		k := campfixKey(m.Name)
		switch m.Kind {
		case campfixAdd:
			c.adds[k]++
			c.totalAdds++
		case campfixRemove:
			c.removes[k]++
			if m.Row != 0 {
				c.removed[m.Row] = true
			}
		}
	}
	for _, n := range c.rows.near(pts, campfixSpan, 0) {
		c.camp[campfixKey(n.Name)]++
	}
	var levels []CampfixNearRecord
	for _, n := range c.rows.near(pts, campfixNearReach, 0) {
		levels = append(levels, CampfixNearRecord{Level: n.Level})
	}
	c.median, c.haveMedian = campfixMedian(levels)
	c.combos = campfixNewcomers(c.facts.spots)
	c.markers = CampMarkers(nil, int(c.world)) // the camps' written places: looking up each anchor in the world takes a second
	for i := range marks {
		mr, err := c.mark(i, flagsOf(i+1))
		if err != nil {
			return out, err
		}
		out.Marks = append(out.Marks, mr)
		if mr.Stale != "" {
			out.Counts["stale"]++
		} else {
			out.Counts[mr.Colour]++
		}
	}
	return out, nil
}

// abuseOf is the account's earlier reports declined as abuse.
func (rv *CampfixReviewer) abuseOf(r database.CampfixReport) ([]int64, error) {
	var ids []int64
	err := campfixEach(rv.db, `SELECT id FROM campfix_reports WHERE accountid = ? AND id < ? AND status = ? AND review LIKE ?
		ORDER BY id`, func(sc func(...any) error) error {
		var id int64
		if err := sc(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	}, r.AccountID, r.ID, database.CampfixDeclined, CampfixDeclinedAbuse+"%")
	if err != nil {
		return nil, fmt.Errorf("campfix_reports: %w", err)
	}
	return ids, nil
}

func campfixIDs(ids []int64) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = fmt.Sprintf("report %d", id)
	}
	return strings.Join(s, ", ")
}

// campfixProperNoun reports a name cased as one creature's own: SOE wrote a
// generic creature's name in lower case but for the proper nouns in it
// ("Deathfist reaver", "Tae Ew crusader") and an individual's as written
// ("Lady Vox"), so it is one when every word of it but of, the, a, an, and is
// capitalised (claude-notes/npc-name-case-20261030.md).
func campfixProperNoun(name string) bool {
	small := map[string]bool{"of": true, "the": true, "a": true, "an": true, "and": true}
	n := 0
	for _, w := range strings.Fields(name) {
		if small[strings.ToLower(w)] {
			continue
		}
		r, _ := utf8.DecodeRuneInString(w)
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.IsUpper(r) {
			return false
		}
		n++
	}
	return n > 0
}

// rare is why the name counts as rare or valuable (the review rules' red a),
// none when it does not: few spawns in the world, a proper noun, a long
// respawn, loot of its own or of worth, a camp's rare mob, spawn-only.
func (c *campfixReviewing) rare(name string, subject campfixRowNPC) ([]string, error) {
	var why []string
	key := campfixKey(name)
	rows, err := c.rows.named(name)
	if err != nil {
		return nil, err
	}
	if n := len(rows); n <= campfixFew {
		why = append(why, fmt.Sprintf("%d %s in %s", n, plural(n, "spawn", "spawns"), worldName(c.world)))
	}
	if campfixProperNoun(name) {
		why = append(why, "its name is a proper noun")
	}
	longest, longRow := 0.0, 0
	for _, r := range rows {
		if s := c.facts.respawn[uint32(r.ID)]; s > longest {
			longest, longRow = s, int(r.ID)
		}
	}
	if longest > campfixLongRespawn {
		why = append(why, fmt.Sprintf("row %d comes back after %s (npcrespawn)", longRow, campfixMinutes(longest)))
	}
	tables := c.facts.loot[database.NPCClassKey(name)]
	for _, t := range tables {
		if !campfixBiomeTables[t] {
			why = append(why, fmt.Sprintf("it rolls the loot table %q", t))
		}
	}
	drops, err := c.rv.dropsOf(tables, subject)
	if err != nil {
		return nil, err
	}
	if drops != "" {
		why = append(why, drops)
	}
	if camp, ok := c.facts.rare[key]; ok {
		why = append(why, fmt.Sprintf("a rare mob of camp %s (its notes)", camp))
	}
	if c.facts.spawnOnly[database.NPCClassKey(name)] {
		why = append(why, "it is spawn only (npcspawnonly)")
	}
	return why, nil
}

func campfixMinutes(sec float64) string {
	if sec >= 3600 {
		return fmt.Sprintf("%.1f hours", sec/3600)
	}
	return fmt.Sprintf("%.0f minutes", sec/60)
}

// dropsOf is what makes a creature's loot worth a GM's look, "" when nothing:
// a boss table it rolls, an item its tables give it at its level more than
// campfixDropAbove levels above it or NO TRADE, the same of its row's own
// corpse loot (npcitem; a merchant's is its stock, not looked at).
func (rv *CampfixReviewer) dropsOf(tables []string, subject campfixRowNPC) (string, error) {
	ck := fmt.Sprintf("%v|%d|%d", tables, subject.Level, subject.ID)
	if s, ok := rv.drops[ck]; ok {
		return s, nil
	}
	found := ""
	for _, t := range tables {
		if strings.HasPrefix(t, "boss") {
			found = fmt.Sprintf("it rolls the boss table %q", t)
			break
		}
	}
	item := func(name string, level, trade int) {
		if found != "" {
			return
		}
		if trade == 1 {
			found = fmt.Sprintf("it drops %s, NO TRADE", name)
		} else {
			found = fmt.Sprintf("it drops %s, level %d (it is level %d)", name, level, subject.Level)
		}
	}
	if found == "" && len(tables) > 0 {
		q := `SELECT COALESCE(p.itemname, ''), COALESCE(p.levelreq, 0), COALESCE(p.trade, 0) FROM loottables t
			JOIN itempattern p ON p.patternid = t.patternid
			WHERE LOWER(t.tablename) IN (?` + strings.Repeat(", ?", len(tables)-1) + `) AND t.weight > 0 AND t.minlevel <= ?
			AND t.maxlevel >= ? AND (COALESCE(p.levelreq, 0) > ? OR COALESCE(p.trade, 0) = 1) ORDER BY p.trade DESC, p.levelreq DESC LIMIT 1`
		var args []any
		for _, t := range tables {
			args = append(args, t)
		}
		args = append(args, subject.Level, subject.Level, subject.Level+campfixDropAbove)
		if err := campfixEach(rv.db, q, func(sc func(...any) error) error {
			var name string
			var level, trade int
			if err := sc(&name, &level, &trade); err != nil {
				return err
			}
			item(name, level, trade)
			return nil
		}, args...); err != nil {
			return "", fmt.Errorf("loottables: %w", err)
		}
	}
	if found == "" && subject.ID != 0 && subject.Type&0x01 == 0 {
		if err := campfixEach(rv.db, `SELECT COALESCE(p.itemname, ''), COALESCE(p.levelreq, 0), COALESCE(p.trade, 0) FROM npcitem i
			JOIN itempattern p ON p.patternid = i.patternid WHERE i.npcid = ? AND (COALESCE(p.levelreq, 0) > ? OR COALESCE(p.trade, 0) = 1)
			ORDER BY p.trade DESC, p.levelreq DESC LIMIT 1`, func(sc func(...any) error) error {
			var name string
			var level, trade int
			if err := sc(&name, &level, &trade); err != nil {
				return err
			}
			item(name, level, trade)
			return nil
		}, int(subject.ID), subject.Level+campfixDropAbove); err != nil {
			return "", fmt.Errorf("npcitem: %w", err)
		}
	}
	rv.drops[ck] = found
	return found, nil
}

// campfixAuditTime reads a gm_audit line's time: the panel's and the game's
// RFC 3339, a tool's own forms besides; the zero time when it is none.
func campfixAuditTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04:05Z07:00"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// campfixChangedSince is the latest audited change of the npcs row id at or
// after since, in words ("" for none): the panel's, a game command's, a
// tool's (its key written with or without spaces, by number or by text).
func campfixChangedSince(db *database.SQLiteDB, id int, since time.Time) (string, error) {
	if since.IsZero() {
		return "", nil
	}
	keys := []any{fmt.Sprintf(`{"id":%d}`, id), fmt.Sprintf(`{"id": %d}`, id), fmt.Sprintf(`{"id":"%d"}`, id), fmt.Sprintf(`{"id": "%d"}`, id)}
	found := ""
	err := campfixEach(db, `SELECT id, at, username, via, action FROM gm_audit WHERE tbl = 'npcs' AND rowkey IN (?, ?, ?, ?)
		ORDER BY id DESC LIMIT 50`, func(sc func(...any) error) error {
		var line int64
		var at, user, via, action string
		if err := sc(&line, &at, &user, &via, &action); err != nil {
			return err
		}
		if found == "" {
			if t := campfixAuditTime(at); !t.IsZero() && !t.Before(since) {
				found = fmt.Sprintf("%s by %s at %s (audit %d)", action, database.WorkByName(database.GMEdit{Username: user, Via: via}), at, line)
			}
		}
		return nil
	}, keys...)
	if err != nil {
		return "", fmt.Errorf("gm_audit: %w", err)
	}
	return found, nil
}

// staleRow is the row a move, a turn or a removal names, and why it is not
// as the report recorded it ("" when it is): gone, in another world, another
// name, out of the world, moved, turned, or changed by anyone since the report
// was sent (the audit log).
func (c *campfixReviewing) staleRow(m CampfixMarkRecord) (campfixRowNPC, string, error) {
	row, ok, err := campfixRowByID(c.rv.db, m.Row)
	switch {
	case err != nil:
		return row, "", err
	case !ok:
		return row, fmt.Sprintf("row %d is gone: deleted since the report was sent", m.Row), nil
	case int(row.World) != c.r.World:
		return row, fmt.Sprintf("row %d is in %s now", m.Row, worldName(row.World)), nil
	case campfixKey(row.Name) != campfixKey(m.Name):
		return row, fmt.Sprintf("row %d is %s now, not %s", m.Row, row.Name, m.Name), nil
	case c.facts.spawnOnly[database.NPCClassKey(row.Name)]:
		return row, fmt.Sprintf("%s stands out of the world now (NPC spawn only)", row.Name), nil
	}
	const near = 0.05 // the report keeps its spawn to 0.01
	if math.Abs(float64(row.X)-m.FromX) > near || math.Abs(float64(row.Y)-m.FromY) > near || math.Abs(float64(row.Z)-m.FromZ) > near {
		return row, fmt.Sprintf("row %d moved since the report was sent: it spawns at (%.2f, %.2f, %.2f), the report found it at (%.2f, %.2f, %.2f)",
			m.Row, row.X, row.Y, row.Z, m.FromX, m.FromY, m.FromZ), nil
	}
	if b := facingByte(float64(row.Facing)); b != m.FromFacing {
		return row, fmt.Sprintf("row %d turned since the report was sent: it faces %d, the report found %d", m.Row, b, m.FromFacing), nil
	}
	changed, err := campfixChangedSince(c.rv.db, m.Row, c.created)
	if err != nil {
		return row, "", err
	}
	if changed != "" {
		return row, fmt.Sprintf("row %d was changed since the report was sent: %s", m.Row, changed), nil
	}
	return row, "", nil
}

// ground is the height an add or a move writes at (x, y, z): the ground's
// highest floor within CampfixFloorReach of the mark's, else the mark's own;
// the words, and whether a floor was found.
func (c *campfixReviewing) ground(x, y, z float32) (float64, string, bool) {
	if c.rv.gw == nil {
		return campfix2(y), "the mark's own height: no world runs to read the ground", false
	}
	f, ok := c.rv.gw.CampfixFloorNear(c.world, x, y, z, CampfixFloorReach)
	if !ok {
		return campfix2(y), fmt.Sprintf("the mark's own height: the ground gives no floor within %.1f of it", CampfixFloorReach), false
	}
	switch d := float64(f - y); {
	case math.Abs(d) < 0.005:
		return campfix2(f), "on the ground's floor, at the mark's height", true
	case d < 0:
		return campfix2(f), fmt.Sprintf("on the ground's floor, %.2f below the mark", -d), true
	default:
		return campfix2(f), fmt.Sprintf("on the ground's floor, %.2f above the mark", d), true
	}
}

// newcomerPlace is what of new players' ground a place is within
// campfixTownReach of, across (the review rules' red b): a town, a start
// spot, a coachman, a spiritmaster, a waysender, a merchant or a quest giver,
// "" none; skip is the creature's own row.
func (c *campfixReviewing) newcomerPlace(x, z float32, skip entity.EntityID) string {
	if c.rv.gw != nil && c.rv.gw.NearTown(c.world, x, z, campfixTownReach) {
		return "a town"
	}
	for _, s := range c.facts.spots {
		if s.World == int(c.world) && campfixXZ(s.X, s.Z, x, z) <= campfixTownReach {
			return fmt.Sprintf("a new %s %s's start spot", strings.ToLower(entity.RaceName(s.Race)), strings.ToLower(entity.ClassName(s.Class)))
		}
	}
	for _, r := range c.rows.list {
		if r.ID == skip || campfixXZ(r.X, r.Z, x, z) > campfixTownReach {
			continue
		}
		what := ""
		switch {
		case entity.IsCoachman(r.Name):
			what = "a coachman"
		case IsSpiritmaster(r.Name):
			what = "a spiritmaster"
		case IsWaysender(r.Name):
			what = "a waysender"
		case r.Type&0x01 != 0:
			what = "a merchant"
		case c.facts.givers[campfixKey(r.Name)]:
			what = "a quest giver"
		}
		if what != "" {
			return fmt.Sprintf("%s (row %d), %s", r.Name, r.ID, what)
		}
	}
	return ""
}

// campOf is the #tele camp a place is in: the nearest camp marker within
// campfixCampName across, "" none.
func (c *campfixReviewing) campOf(x, z float32) string {
	key, best := "", float32(-1)
	for _, m := range c.markers {
		if d := campfixXZ(m.X, m.Z, x, z); d <= campfixCampName && (best < 0 || d < best || d == best && m.Key < key) {
			key, best = m.Key, d
		}
	}
	return key
}

// stackedNear is a row other than skip, and none the report removes, whose
// spawn lies within campfixStacked of (x, y, z): its words, "" none.
func (c *campfixReviewing) stackedNear(x, y, z float32, skip entity.EntityID) string {
	best, bestD := campfixRowNPC{}, float32(-1)
	for _, r := range c.rows.near([][3]float32{{x, y, z}}, campfixStacked, 0) {
		if r.ID == skip || c.removed[int(r.ID)] {
			continue
		}
		if bestD < 0 || r.Dist < bestD {
			best, bestD = r, r.Dist
		}
	}
	if bestD < 0 {
		return ""
	}
	return fmt.Sprintf("%.1f from %s (row %d): no spawn is set within %.0f of another", bestD, best.Name, best.ID, campfixStacked)
}

// recorded reports the report's flag of kind on a mark.
func campfixRecorded(fl []CampfixFlagRecord, kind string) (CampfixFlagRecord, bool) {
	for _, f := range fl {
		if f.Flag == kind {
			return f, true
		}
	}
	return CampfixFlagRecord{}, false
}

// mark is mark i under review (its flags fl).
func (c *campfixReviewing) mark(i int, fl []CampfixFlagRecord) (CampfixMarkReview, error) {
	m := c.marks[i]
	mr := CampfixMarkReview{N: i + 1, CampfixMarkRecord: m, Flags: fl, Reasons: []string{}}
	var red, yellow, green []string
	key := campfixKey(m.Name)
	if len(c.abuse) > 0 {
		red = append(red, fmt.Sprintf("the account had an earlier report declined as abuse (%s)", campfixIDs(c.abuse)))
	}
	named, err := c.rows.named(m.Name)
	if err != nil {
		return mr, err
	}
	worldCount := len(named)
	common := worldCount > campfixFew && !campfixProperNoun(m.Name)
	mx, my, mz := float32(m.X), float32(m.Y), float32(m.Z)
	fighting := func(id entity.EntityID) string {
		if c.rv.gw == nil {
			return ""
		}
		if st, ok := c.rv.gw.NPCStateOf(id); ok {
			return st.Fighting
		}
		return ""
	}
	// what placing the subject at (x, yg, z) asks of the rules (red b, c,
	// water, stacking; yellow ground)
	placing := func(subject campfixRowNPC, yg float64, floored bool, skip entity.EntityID) {
		if c.rv.gw != nil {
			if c.rv.gw.CampfixNewcomerThreat(subject.ID, c.combos) {
				if where := c.newcomerPlace(mx, mz, subject.ID); where != "" {
					red = append(red, fmt.Sprintf("%s attacks a level 1 of some race on sight, and the mark is within %d of %s",
						m.Name, campfixTownReach, where))
				}
			}
			if g := c.rv.gw.CampfixGroundAt(c.world, mx, float32(yg), mz); g.WetKnown && g.Wet && c.rv.gw.CampfixKeepsToLand(subject.ID) {
				red = append(red, fmt.Sprintf("%s keeps to the land, and the mark is in the water", m.Name))
			}
		} else {
			if f, ok := campfixRecorded(fl, "aggro"); ok {
				red = append(red, "the report found: "+f.Text)
			}
			if f, ok := campfixRecorded(fl, "in_water"); ok {
				red = append(red, "the report found: "+f.Text)
			}
		}
		if c.haveMedian && math.Abs(float64(subject.Level)-c.median) > campfixLevelOff {
			red = append(red, fmt.Sprintf("level %d, %s from the camp's median level %s", subject.Level,
				campfixNum(math.Abs(float64(subject.Level)-c.median)), campfixNum(c.median)))
		}
		if s := c.stackedNear(mx, float32(yg), mz, skip); s != "" {
			red = append(red, s)
		}
		if !floored {
			yellow = append(yellow, "the ground gives no floor within 2.5 of the mark: it keeps the mark's height")
		}
		if f, ok := campfixRecorded(fl, "above_ground"); ok {
			yellow = append(yellow, "the report found "+f.Text+": grounded as the plan says")
		}
	}

	switch m.Kind {
	case campfixMove, campfixTurn, campfixRemove:
		if m.Row == 0 {
			mr.Stale = fmt.Sprintf("no spawn of %s was left free near the mark to %s when it was sent", m.Name, m.Kind)
			break
		}
		row, why, err := c.staleRow(m)
		if err != nil {
			return mr, err
		}
		if why != "" {
			mr.Stale = why
			break
		}
		was := &CampfixPlace{X: row.RawX, Y: row.RawY, Z: row.RawZ, Facing: row.FacingByte, Null: row.RawNull}
		switch m.Kind {
		case campfixMove:
			yg, words, floored := c.ground(mx, my, mz)
			d := campfixDist(row.X, row.Y, row.Z, mx, my, mz)
			mr.Plan = CampfixPlan{Action: CampfixUpdate, Row: m.Row, X: m.X, Y: yg, Z: m.Z, Facing: m.FacingByte, Ground: words,
				Fighting: fighting(row.ID), Was: was, Words: fmt.Sprintf("row %d (%s) moves %.1f to (%.2f, %.2f, %.2f), facing %d (it faced %d)",
					m.Row, row.Name, d, m.X, yg, m.Z, m.FacingByte, row.FacingByte)}
			rare, err := c.rare(m.Name, row)
			if err != nil {
				return mr, err
			}
			from, to := c.campOf(row.X, row.Z), c.campOf(mx, mz)
			other := from != to && from != "" && to != ""
			switch {
			case len(rare) > 0 && (d > campfixFarMove || other):
				red = append(red, "a move of a rare or valuable creature into another camp: "+strings.Join(rare, "; "))
			case d > campfixSpan:
				red = append(red, fmt.Sprintf("a move of %.0f, past a camp's %.0f", d, campfixSpan))
			case d >= campfixFarMove:
				yellow = append(yellow, fmt.Sprintf("a move of %.0f (150 to 300)", d))
			case common && !other:
				green = append(green, fmt.Sprintf("a move of %.0f, under 150, of a common creature within its camp", d))
			}
			if other {
				yellow = append(yellow, fmt.Sprintf("from camp %s to camp %s", from, to))
			}
			placing(row, yg, floored, row.ID)
		case campfixTurn:
			mr.Plan = CampfixPlan{Action: CampfixUpdate, Row: m.Row, X: row.RawX, Y: row.RawY, Z: row.RawZ, Facing: m.FacingByte,
				Fighting: fighting(row.ID), Was: was, Words: fmt.Sprintf("row %d (%s) turns from facing %d to %d", m.Row, row.Name,
					row.FacingByte, m.FacingByte)}
			if row.FacingByte == m.FacingByte {
				mr.Plan.Action = CampfixNone
				mr.Plan.Words = fmt.Sprintf("row %d (%s) faces %d already: nothing to change", m.Row, row.Name, m.FacingByte)
			}
			green = append(green, "a change of facing alone")
		case campfixRemove:
			mr.Plan = CampfixPlan{Action: CampfixDelete, Row: m.Row, X: row.RawX, Y: row.RawY, Z: row.RawZ, Facing: row.FacingByte,
				Fighting: fighting(row.ID), Was: was, Words: fmt.Sprintf("row %d (%s) is deleted", m.Row, row.Name)}
			if cs, ok := c.facts.camps[m.Row]; ok {
				mr.Plan.Camp, mr.Plan.Kinds = cs.Camp, cs.Kinds
				mr.Plan.Words += fmt.Sprintf(", and its camp spawn row (camp %d)", cs.Camp)
			}
			if c.facts.paths[m.Row] {
				mr.Plan.Path = true
				mr.Plan.Words += ", and its path"
			}
			if q, ok := c.facts.pins[m.Row]; ok {
				red = append(red, fmt.Sprintf("row %d is the spawn of %s", m.Row, q))
			} else if q, ok := c.facts.quests[key]; ok {
				red = append(red, fmt.Sprintf("%s is %s", m.Name, q))
			}
			if c.removes[key] >= worldCount {
				red = append(red, fmt.Sprintf("it removes the last %s of %s in %s", plural(worldCount, "spawn", "spawns"), m.Name, worldName(c.world)))
			}
			if c.removes[key] >= c.camp[key] {
				yellow = append(yellow, fmt.Sprintf("it leaves no %s in the camp", m.Name))
			}
			for _, r := range c.rows.near([][3]float32{{row.X, row.Y, row.Z}}, campfixStacked, 0) {
				if r.ID != row.ID && !c.removed[int(r.ID)] && campfixKey(r.Name) == key {
					green = append(green, fmt.Sprintf("a stacked duplicate: row %d of its name spawns %.1f from it", r.ID, r.Dist))
					break
				}
			}
		}
	case campfixAdd:
		t, ok, err := c.rows.nearestNamed(m.Name, mx, my, mz)
		if err != nil {
			return mr, err
		}
		if !ok {
			mr.Stale = fmt.Sprintf("no row of %s is left in %s to copy", m.Name, worldName(c.world))
			break
		}
		yg, words, floored := c.ground(mx, my, mz)
		mr.Plan = CampfixPlan{Action: CampfixInsert, Template: int(t.ID), X: m.X, Y: yg, Z: m.Z, Facing: m.FacingByte, Ground: words,
			Words: fmt.Sprintf("a new row, a copy of row %d (%s, level %d: its body, level and gear; its loot, class, template and "+
				"faction go by its name), at (%.2f, %.2f, %.2f), facing %d", t.ID, t.Name, t.Level, m.X, yg, m.Z, m.FacingByte)}
		if cs, ok := c.facts.camps[int(t.ID)]; ok {
			mr.Plan.Camp, mr.Plan.Kinds = cs.Camp, cs.Kinds
			mr.Plan.Words += fmt.Sprintf("; a spawn of camp %d, as its template is, popping %s", cs.Camp, database.CampKindsText(cs.Kinds))
		}
		rare, err := c.rare(m.Name, t)
		if err != nil {
			return mr, err
		}
		if len(rare) > 0 {
			red = append(red, "an add of a rare or valuable creature: "+strings.Join(rare, "; "))
		}
		if c.totalAdds > campfixManyAdds {
			red = append(red, fmt.Sprintf("%d adds in one report, more than %d", c.totalAdds, campfixManyAdds))
		}
		k, n := c.adds[key], c.camp[key]
		switch {
		case n == 0:
			red = append(red, fmt.Sprintf("%d %s where none spawns in the camp", k, plural(k, "add", "adds")))
		case 2*k > n && !(n <= 2 && k == 1):
			red = append(red, fmt.Sprintf("%d %s to the camp's %d: more than half", k, plural(k, "add", "adds"), n))
		case k <= 2 && common:
			green = append(green, "an add of one or two of a common creature already in the camp")
		}
		if t.Dist >= campfixSpan {
			yellow = append(yellow, fmt.Sprintf("its template, row %d, is %.0f from the mark", t.ID, t.Dist))
		}
		placing(t, yg, floored, 0)
	default:
		mr.Stale = fmt.Sprintf("a mark of kind %q is not one a report makes", m.Kind)
	}
	if mr.Stale != "" {
		mr.Reasons = append(mr.Reasons, mr.Stale)
		return mr, nil
	}
	if len(red)+len(yellow)+len(green) == 0 {
		yellow = []string{"no rule calls it green"}
	}
	mr.Red, mr.Yellow, mr.Green = red, yellow, green
	switch {
	case len(red) > 0:
		mr.Colour, mr.Reasons = CampfixRed, red
	case len(yellow) > 0:
		mr.Colour, mr.Reasons = CampfixYellow, yellow
	default:
		mr.Colour, mr.Reasons = CampfixGreen, green
	}
	mr.Checked = mr.Colour != CampfixRed
	return mr, nil
}
