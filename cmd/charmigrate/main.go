// charmigrate moves a character in from another server by a plan a GM writes:
// its level, its mastery points, its class's spells, item records put right or
// made for what it held that this server lacks, and its items, into the bank
// or the pack. The plan is a JSON file kept outside this repository (it names
// a player's character); see planFile below for its fields.
//
// It writes the server's database beside the running server, so the character
// must be OUT of the world: the logout save would write its level back over
// the new one, and the game keeps its inventory in memory while it plays.
// -wait <gameserver.log> waits for that: it reads the log until the session of
// the character's latest login has been cleaned up ("Cleaning up terminated
// session ... objectID=N", N the playerObjectID its "SelectedChar" lines gave),
// and only then applies.
//
//	charmigrate -db eqoa.db -plan plan.json             a dry run: what it would do
//	charmigrate -db eqoa.db -plan plan.json -apply      do it, in one transaction
//	charmigrate -db eqoa.db -plan plan.json -apply -wait gameserver.log -backup eqoa.db.bak-<date>-pre-<name>
//	charmigrate -db eqoa.db -runs                       the runs it has made
//	charmigrate -db eqoa.db -undo <run>                 take a run back
//
// Every change is a row of table charmigrate_log (the run, the character, what
// kind, the table and row, the row before and after as JSON), and -undo takes
// a run back in reverse order: a row changed since by anything else is left as
// it is and said (-force takes it back all the same).
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// planFile is a migration: the character by name (it must exist here, made by
// its player, so its race, sex and look are theirs), and what it gets.
type planFile struct {
	Character string `json:"character"`
	Note      string `json:"note"`
	// Level sets its level and the experience that reaches it (0: as it is).
	// Training points follow at its next login (entity.TopUpTraining).
	Level int `json:"level"`
	// MasteryPoints are unspent class mastery points added to what it has,
	// never past entity.MasteryCap spent and unspent together.
	MasteryPoints int `json:"mastery_points"`
	// ClassSpells puts every spell its class gets by level in its book, as
	// #template does (database.GrantClassSpells' rule).
	ClassSpells bool `json:"class_spells"`
	// FullVitals drops its saved hit points and power, so it comes in whole
	// at its next login (charvitals: no row, whole).
	FullVitals bool `json:"full_vitals"`
	// Records are item records put right: columns of itempattern set.
	Records []recordFix `json:"records"`
	// NewRecords are item records made, each a copy of Like with Set over it,
	// for what the character held that this server has no record of.
	NewRecords []newRecord `json:"new_records"`
	// Items are given: a record by Pattern, or a new record by its key, Count
	// rows of one each (default 1).
	Items []planItem `json:"items"`
	// To is where the items go: "bank" (the default) or "pack".
	To string `json:"to"`
	// Moves are rows the character holds already, moved to the bank or the
	// pack (unequipped, at the end of its list): an earlier run's items
	// taken out of the bank, say.
	Moves []planMove `json:"moves"`
}

type planMove struct {
	ItemID int    `json:"itemid"`
	To     string `json:"to"`
}

type recordFix struct {
	Pattern int            `json:"pattern"`
	Set     map[string]any `json:"set"`
	Why     string         `json:"why"`
}

type newRecord struct {
	Ref  string         `json:"ref"`
	Like int            `json:"like"`
	Set  map[string]any `json:"set"`
	Why  string         `json:"why"`
}

type planItem struct {
	Pattern int    `json:"pattern"`
	New     string `json:"new"`
	Count   int    `json:"count"`
}

// packCap is how many records CLIENT's pack holds, worn ones among them
// (handlers/itemlist.go invListCap). bankWarn is past what this tool will put
// in a bank without -force (CLIENT's bank list is not measured here).
const (
	packCap  = 40
	bankWarn = 60
)

const logSchema = `CREATE TABLE IF NOT EXISTS charmigrate_log (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	run      TEXT NOT NULL,
	serverid INTEGER NOT NULL,
	charname TEXT NOT NULL,
	kind     TEXT NOT NULL,
	tbl      TEXT NOT NULL,
	row_id   INTEGER NOT NULL,
	before   TEXT NOT NULL DEFAULT '',
	after    TEXT NOT NULL DEFAULT '',
	note     TEXT NOT NULL DEFAULT '',
	at       TEXT NOT NULL,
	undone   TEXT NOT NULL DEFAULT ''
)`

func main() {
	dbPath := flag.String("db", "eqoa.db", "the server's database")
	planPath := flag.String("plan", "", "the migration plan (JSON)")
	apply := flag.Bool("apply", false, "do it (else a dry run)")
	wait := flag.String("wait", "", "the server's log: apply only once the character's session there has been cleaned up")
	waitFor := flag.Duration("wait-for", 24*time.Hour, "how long -wait waits")
	undo := flag.String("undo", "", "take this run back")
	runs := flag.Bool("runs", false, "list the runs")
	force := flag.Bool("force", false, "past the checks: a full bank, rows changed since a run")
	backup := flag.String("backup", "", "with -apply: copy the database here first (VACUUM INTO), after any -wait")
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.LUTC)

	db, err := open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	switch {
	case *runs:
		if err := listRuns(db); err != nil {
			log.Fatal(err)
		}
		return
	case *undo != "":
		if err := undoRun(db, *undo, *force); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *planPath == "" {
		log.Fatal("give -plan (or -runs, -undo)")
	}
	p, err := readPlan(*planPath)
	if err != nil {
		log.Fatal(err)
	}
	if *wait != "" {
		if !*apply {
			log.Fatal("-wait goes with -apply")
		}
		if err := waitOffline(*wait, p.Character, *waitFor); err != nil {
			log.Fatal(err)
		}
	}
	if *apply && *backup != "" {
		if _, err := os.Stat(*backup); err == nil {
			log.Fatalf("%s is there already", *backup)
		}
		if _, err := db.Exec(`VACUUM INTO ?`, *backup); err != nil {
			log.Fatalf("the backup: %v", err)
		}
		log.Printf("database copied to %s", *backup)
	}
	if err := migrate(db, p, *apply, *force, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func open(path string) (*sql.DB, error) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return nil, fmt.Errorf("no database at %s", path)
	}
	db, err := sql.Open("sqlite", path+"?"+database.BusyTimeoutDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func readPlan(path string) (planFile, error) {
	var p planFile
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("the plan: %w", err)
	}
	if strings.TrimSpace(p.Character) == "" {
		return p, errors.New("the plan names no character")
	}
	if p.Level < 0 || p.Level > 60 {
		return p, fmt.Errorf("level %d: 1-60, or 0 to leave it", p.Level)
	}
	if p.MasteryPoints < 0 || p.MasteryPoints > entity.MasteryCap {
		return p, fmt.Errorf("mastery_points %d: 0-%d", p.MasteryPoints, entity.MasteryCap)
	}
	switch p.To {
	case "", "bank", "pack":
	default:
		return p, fmt.Errorf("to %q: bank or pack", p.To)
	}
	for _, m := range p.Moves {
		if m.ItemID <= 0 || (m.To != "bank" && m.To != "pack") {
			return p, fmt.Errorf("a move names an itemid and \"bank\" or \"pack\" (%d %q)", m.ItemID, m.To)
		}
	}
	refs := map[string]bool{}
	for _, n := range p.NewRecords {
		if n.Ref == "" || refs[n.Ref] {
			return p, fmt.Errorf("a new record needs a ref of its own (%q)", n.Ref)
		}
		refs[n.Ref] = true
	}
	for _, it := range p.Items {
		if (it.Pattern == 0) == (it.New == "") {
			return p, errors.New("an item names a pattern or a new record's ref, one of them")
		}
		if it.New != "" && !refs[it.New] {
			return p, fmt.Errorf("an item names new record %q, which the plan does not make", it.New)
		}
		if it.Count < 0 || it.Count > 20 {
			return p, fmt.Errorf("count %d: 1-20", it.Count)
		}
	}
	return p, nil
}

// The log's lines a login and its end leave (handlers/character.go,
// cmd/gameserver): the character selected, then the player object it gets;
// the session's clean-up names that object.
var (
	reSelected = regexp.MustCompile(`SelectedChar: selected '([^']+)'`)
	reObject   = regexp.MustCompile(`SelectedChar: generated playerObjectID=(\d+)`)
	reCleanup  = regexp.MustCompile(`Cleaning up terminated session \S+ \(instance=\S+ objectID=(\d+)\)`)
)

// offlineInLog reads a log and says whether the character's latest login has
// ended: the session holding the object its latest selection got has been
// cleaned up. ok is false when the log holds no login of it (it may have come
// in before the log began).
func offlineInLog(path, name string) (offline, ok bool, object string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	pending := 0 // lines left to find the selection's object in
	for sc.Scan() {
		line := sc.Text()
		if m := reSelected.FindStringSubmatch(line); m != nil {
			if strings.EqualFold(m[1], name) {
				pending, offline, ok, object = 8, false, false, ""
			}
			continue
		}
		if pending > 0 {
			pending--
			if m := reObject.FindStringSubmatch(line); m != nil {
				object, ok, pending = m[1], true, 0
			}
			continue
		}
		if object != "" && !offline {
			if m := reCleanup.FindStringSubmatch(line); m != nil && m[1] == object {
				offline = true
			}
		}
	}
	return offline, ok, object, sc.Err()
}

// waitOffline waits until the log says the character's session is over, then
// a few seconds more for the save behind the clean-up.
func waitOffline(path, name string, limit time.Duration) error {
	until := time.Now().Add(limit)
	said := false
	for {
		off, ok, object, err := offlineInLog(path, name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s holds no login of %s: is it the running server's log? (apply without -wait once you know it is out)", path, name)
		}
		if off {
			log.Printf("%s's session (object %s) has been cleaned up", name, object)
			time.Sleep(5 * time.Second)
			if off2, ok2, object2, err := offlineInLog(path, name); err == nil && ok2 && off2 && object2 == object {
				return nil
			}
			log.Printf("%s came back in meanwhile: waiting again", name)
			continue
		}
		if !said {
			log.Printf("%s is in the world (object %s): waiting for its session to end", name, object)
			said = true
		}
		if time.Now().After(until) {
			return fmt.Errorf("%s still in the world after %v", name, limit)
		}
		time.Sleep(5 * time.Second)
	}
}

type character struct {
	serverID           uint32
	name               string
	class, race, level int
	totalXP            int
}

type migration struct {
	tx     *sql.Tx
	run    string
	ch     character
	apply  bool
	out    io.Writer
	stamp  string
	logged int
}

func (m *migration) say(format string, a ...any) { fmt.Fprintf(m.out, format+"\n", a...) }

// note logs one change (applied only).
func (m *migration) note(kind, tbl string, rowID int64, before, after any, note string) error {
	m.logged++
	if !m.apply {
		return nil
	}
	enc := func(v any) string {
		if v == nil {
			return ""
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	_, err := m.tx.Exec(`INSERT INTO charmigrate_log (run, serverid, charname, kind, tbl, row_id, before, after, note, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, m.run, m.ch.serverID, m.ch.name, kind, tbl, rowID, enc(before), enc(after), note, m.stamp)
	return err
}

func migrate(db *sql.DB, p planFile, apply, force bool, out io.Writer) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if apply {
		if _, err := tx.Exec(logSchema); err != nil {
			return fmt.Errorf("the log: %w", err)
		}
	}
	var ch character
	err = tx.QueryRow(`SELECT serverid, charName, tclass, race, level, totalXP FROM characters WHERE charName = ? COLLATE NOCASE`,
		p.Character).Scan(&ch.serverID, &ch.name, &ch.class, &ch.race, &ch.level, &ch.totalXP)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no character named %q (its player makes it first)", p.Character)
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	m := &migration{tx: tx, ch: ch, apply: apply, out: out, stamp: now.Format(time.RFC3339),
		run: now.Format("20060102T150405Z") + "-" + strings.ToLower(ch.name)}
	if apply {
		// a run's id is its own: another in the same second gets -2, -3...
		base := m.run
		for n := 2; ; n++ {
			var used int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM charmigrate_log WHERE run = ?`, m.run).Scan(&used); err != nil {
				return err
			}
			if used == 0 {
				break
			}
			m.run = fmt.Sprintf("%s-%d", base, n)
		}
	}
	if apply {
		m.say("Run %s: %s (serverid %d, class %d, race %d, level %d)", m.run, ch.name, ch.serverID, ch.class, ch.race, ch.level)
	} else {
		m.say("Dry run: %s (serverid %d, class %d, race %d, level %d); nothing written", ch.name, ch.serverID, ch.class, ch.race, ch.level)
	}
	if p.Note != "" {
		m.say("Note: %s", p.Note)
	}

	if err := m.fixRecords(p.Records); err != nil {
		return err
	}
	made, err := m.newRecords(p.NewRecords)
	if err != nil {
		return err
	}
	if err := m.setLevel(p.Level); err != nil {
		return err
	}
	if err := m.addMastery(p.MasteryPoints); err != nil {
		return err
	}
	if p.ClassSpells {
		if err := m.classSpells(); err != nil {
			return err
		}
	}
	if p.FullVitals {
		if err := m.dropVitals(); err != nil {
			return err
		}
	}
	if err := m.giveItems(p.Items, made, p.To, force); err != nil {
		return err
	}
	if err := m.moveItems(p.Moves); err != nil {
		return err
	}
	if !apply {
		m.say("Dry run done: %d changes; -apply to make them", m.logged)
		return nil
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.say("Done: %d changes, logged as run %s (-undo %s takes them back)", m.logged, m.run, m.run)
	return nil
}

// patternColumns is itempattern's columns, in order.
func patternColumns(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info('itempattern') ORDER BY cid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// columnOf is the column of itempattern named c (any case), or "".
func columnOf(cols []string, c string) string {
	for _, k := range cols {
		if strings.EqualFold(k, c) {
			return k
		}
	}
	return ""
}

// patternRow is a record's columns by name.
func patternRow(tx *sql.Tx, cols []string, pattern int) (map[string]any, error) {
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	err := tx.QueryRow(`SELECT * FROM itempattern WHERE patternid = ?`, pattern).Scan(ptrs...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no item record %d", pattern)
	}
	if err != nil {
		return nil, err
	}
	row := make(map[string]any, len(cols))
	for i, c := range cols {
		if b, ok := vals[i].([]byte); ok {
			row[c] = string(b)
		} else {
			row[c] = vals[i]
		}
	}
	return row, nil
}

// value is a plan's JSON value as SQLite takes it: whole numbers as integers.
func value(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	return v
}

func (m *migration) fixRecords(fixes []recordFix) error {
	if len(fixes) == 0 {
		return nil
	}
	cols, err := patternColumns(m.tx)
	if err != nil {
		return err
	}
	for _, f := range fixes {
		row, err := patternRow(m.tx, cols, f.Pattern)
		if err != nil {
			return err
		}
		before, after := map[string]any{}, map[string]any{}
		var sets []string
		var args []any
		names := make([]string, 0, len(f.Set))
		for c := range f.Set {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			col := columnOf(cols, c)
			if col == "" || col == "patternid" {
				return fmt.Errorf("record %d: no column %q to set", f.Pattern, c)
			}
			before[col], after[col] = row[col], value(f.Set[c])
			sets = append(sets, col+" = ?")
			args = append(args, value(f.Set[c]))
		}
		m.say("Record %d %q: %s -> %s (%s)", f.Pattern, row["itemname"], js(before), js(after), f.Why)
		if m.apply {
			if _, err := m.tx.Exec(`UPDATE itempattern SET `+strings.Join(sets, ", ")+` WHERE patternid = ?`,
				append(args, f.Pattern)...); err != nil {
				return err
			}
		}
		if err := m.note("record", "itempattern", int64(f.Pattern), before, after, f.Why); err != nil {
			return err
		}
	}
	return nil
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// newRecords makes the plan's new records: the patternid each ref got (in a
// dry run, a stand-in below 0).
func (m *migration) newRecords(news []newRecord) (map[string]int, error) {
	made := map[string]int{}
	if len(news) == 0 {
		return made, nil
	}
	cols, err := patternColumns(m.tx)
	if err != nil {
		return nil, err
	}
	for i, n := range news {
		row, err := patternRow(m.tx, cols, n.Like)
		if err != nil {
			return nil, err
		}
		from := row["itemname"]
		for c, v := range n.Set {
			col := columnOf(cols, c)
			if col == "" || col == "patternid" {
				return nil, fmt.Errorf("new record %q: no column %q to set", n.Ref, c)
			}
			row[col] = value(v)
		}
		var use []string
		var args []any
		for _, c := range cols {
			if c == "patternid" {
				continue
			}
			use = append(use, c)
			args = append(args, row[c])
		}
		id := -(i + 1)
		if m.apply {
			res, err := m.tx.Exec(`INSERT INTO itempattern (`+strings.Join(use, ", ")+`) VALUES (`+
				strings.TrimSuffix(strings.Repeat("?, ", len(use)), ", ")+`)`, args...)
			if err != nil {
				return nil, fmt.Errorf("new record %q: %w", n.Ref, err)
			}
			n64, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			id = int(n64)
			row["patternid"] = n64
		}
		made[n.Ref] = id
		m.say("New record %d %q, a copy of %d %q with %s (%s)", id, row["itemname"], n.Like, from, js(n.Set), n.Why)
		if err := m.note("newrecord", "itempattern", int64(id), nil, row, n.Why); err != nil {
			return nil, err
		}
	}
	return made, nil
}

func (m *migration) setLevel(level int) error {
	if level == 0 || level == m.ch.level {
		return nil
	}
	xp := entity.TotalXP(level, 0)
	before := map[string]any{"level": m.ch.level, "totalXP": m.ch.totalXP}
	after := map[string]any{"level": level, "totalXP": xp}
	m.say("Level %d -> %d (experience %d -> %d); training points made up at its next login", m.ch.level, level, m.ch.totalXP, xp)
	if m.apply {
		if _, err := m.tx.Exec(`UPDATE characters SET level = ?, totalXP = ? WHERE serverid = ?`, level, xp, m.ch.serverID); err != nil {
			return err
		}
	}
	return m.note("level", "characters", int64(m.ch.serverID), before, after, "")
}

type masteryRow struct {
	Points, Spent, Percent, XP int
}

func (m *migration) masteryNow() (masteryRow, bool, error) {
	var r masteryRow
	err := m.tx.QueryRow(`SELECT points, spent, percent, xp FROM charmastery WHERE serverid = ?`, m.ch.serverID).
		Scan(&r.Points, &r.Spent, &r.Percent, &r.XP)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

func (m *migration) addMastery(points int) error {
	if points == 0 {
		return nil
	}
	now, had, err := m.masteryNow()
	if err != nil {
		return err
	}
	next := now
	next.Points = min(now.Points+points, entity.MasteryCap-now.Spent)
	if next.Points < now.Points {
		next.Points = now.Points
	}
	m.say("Mastery points %d unspent (%d spent) -> %d unspent", now.Points, now.Spent, next.Points)
	if next.Points-now.Points < points {
		m.say("  (held to %d spent and unspent together)", entity.MasteryCap)
	}
	if m.apply {
		if _, err := m.tx.Exec(`INSERT INTO charmastery (serverid, points, spent, percent, xp) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (serverid) DO UPDATE SET points = excluded.points, spent = excluded.spent,
			percent = excluded.percent, xp = excluded.xp`, m.ch.serverID, next.Points, next.Spent, next.Percent, next.XP); err != nil {
			return err
		}
	}
	var before any
	if had {
		before = now
	}
	return m.note("mastery", "charmastery", int64(m.ch.serverID), before, next, "")
}

// classSpells is database.GrantClassSpells inside the run's transaction: every
// spell of the class's list it lacks, but a were's and those a level does not
// give, in level then name order after what its book holds.
func (m *migration) classSpells() error {
	rows, err := m.tx.Query(`SELECT c.spellid, p.spellname, COALESCE(p.spelldesc, ''), COALESCE(c.source, '') FROM classspells c
		JOIN spellpattern p ON p.spellid = c.spellid
		WHERE c.class = ? AND c.spellid NOT IN (SELECT spellid FROM spells WHERE serverid = ?)
		ORDER BY p.abilitylvl, p.spellname, c.spellid`, m.ch.class, m.ch.serverID)
	if err != nil {
		return err
	}
	type sp struct {
		id   int
		name string
	}
	var give []sp
	seen := map[int]bool{}
	for rows.Next() {
		var id int
		var name, desc, source string
		if err := rows.Scan(&id, &name, &desc, &source); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] && !database.IsWereAbility(name, desc) && database.Granted(source) {
			seen[id] = true
			give = append(give, sp{id, name})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var next int
	if err := m.tx.QueryRow(`SELECT COALESCE(MAX(addedorder) + 1, 0) FROM spells WHERE serverid = ? AND addedorder < 252`,
		m.ch.serverID).Scan(&next); err != nil {
		return err
	}
	if room := 252 - next; len(give) > room {
		m.say("Spells: %d of the class's, the book has room for %d", len(give), max(room, 0))
		give = give[:max(room, 0)]
	}
	names := make([]string, len(give))
	for i, s := range give {
		names[i] = s.name
	}
	m.say("Spells: %d of the class's into the book: %s", len(give), strings.Join(names, ", "))
	for i, s := range give {
		var rowID int64 = -1
		if m.apply {
			res, err := m.tx.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
				VALUES (?, ?, ?, 0, -1, 0, -1)`, m.ch.serverID, s.id, next+i)
			if err != nil {
				return err
			}
			if rowID, err = res.LastInsertId(); err != nil {
				return err
			}
		}
		if err := m.note("spell", "spells", rowID, nil, map[string]any{"spellid": s.id, "addedorder": next + i}, s.name); err != nil {
			return err
		}
	}
	return nil
}

func (m *migration) dropVitals() error {
	var hp, power int
	var saved string
	err := m.tx.QueryRow(`SELECT hp, power, saved FROM charvitals WHERE serverid = ?`, m.ch.serverID).Scan(&hp, &power, &saved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	m.say("Saved hit points %d and power %d dropped: whole at its next login", hp, power)
	if m.apply {
		if _, err := m.tx.Exec(`DELETE FROM charvitals WHERE serverid = ?`, m.ch.serverID); err != nil {
			return err
		}
	}
	return m.note("vitals", "charvitals", int64(m.ch.serverID), map[string]any{"hp": hp, "power": power, "saved": saved}, nil, "")
}

func (m *migration) giveItems(items []planItem, made map[string]int, to string, force bool) error {
	if len(items) == 0 {
		return nil
	}
	loc, where := 1, "bank"
	if to == "pack" {
		loc, where = 0, "pack"
	}
	var have int
	if err := m.tx.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = ?`, m.ch.serverID, loc).Scan(&have); err != nil {
		return err
	}
	rows := 0
	for _, it := range items {
		rows += max(it.Count, 1)
	}
	switch {
	case loc == 0 && have+rows > packCap:
		return fmt.Errorf("the pack holds %d records and %d more would pass its %d: give to the bank", have, rows, packCap)
	case loc == 1 && have+rows > bankWarn && !force:
		return fmt.Errorf("the bank holds %d records and %d more would pass %d (-force to go on)", have, rows, bankWarn)
	}
	m.say("Items: %d into the %s (holding %d)", rows, where, have)
	for _, it := range items {
		pattern := it.Pattern
		if it.New != "" {
			pattern = made[it.New]
		}
		var name string
		var maxHP int
		if pattern > 0 {
			if err := m.tx.QueryRow(`SELECT itemname, maxhp FROM itempattern WHERE patternid = ?`, pattern).Scan(&name, &maxHP); err != nil {
				return fmt.Errorf("item record %d: %w", pattern, err)
			}
		} else {
			name = "(new record " + it.New + ")"
		}
		for n := 0; n < max(it.Count, 1); n++ {
			var itemID int64 = -1
			if m.apply {
				res, err := m.tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
					VALUES (?, 1, ?, 0, ?, -1, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))`,
					m.ch.serverID, maxHP, pattern, loc, m.ch.serverID, loc)
				if err != nil {
					return fmt.Errorf("giving %q: %w", name, err)
				}
				if itemID, err = res.LastInsertId(); err != nil {
					return err
				}
			}
			m.say("  %s (record %d)", name, pattern)
			if err := m.note("item", "charinventory", itemID, nil, map[string]any{"patternid": pattern, "location": loc}, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveItems moves rows the character holds to the bank or the pack,
// unequipped, at the end of that list; the pack held to packCap.
func (m *migration) moveItems(moves []planMove) error {
	if len(moves) == 0 {
		return nil
	}
	var pack int
	if err := m.tx.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0`, m.ch.serverID).Scan(&pack); err != nil {
		return err
	}
	for _, mv := range moves {
		loc := 1
		if mv.To == "pack" {
			loc = 0
		}
		var name string
		var from, equip, list int
		err := m.tx.QueryRow(`SELECT p.itemname, c.location, c.equiploc, c.listnumber FROM charinventory c
			JOIN itempattern p ON p.patternid = c.patternid WHERE c.itemid = ? AND c.serverid = ?`, mv.ItemID, m.ch.serverID).
			Scan(&name, &from, &equip, &list)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("the character holds no item %d", mv.ItemID)
		}
		if err != nil {
			return err
		}
		if from == loc {
			m.say("  %s (item %d) is in the %s already", name, mv.ItemID, mv.To)
			continue
		}
		if loc == 0 {
			if pack >= packCap {
				return fmt.Errorf("the pack holds %d records: no room for %s", pack, name)
			}
			pack++
		}
		next := 0
		if err := m.tx.QueryRow(`SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?`,
			m.ch.serverID, loc).Scan(&next); err != nil {
			return err
		}
		m.say("  %s (item %d) to the %s", name, mv.ItemID, mv.To)
		if m.apply {
			if _, err := m.tx.Exec(`UPDATE charinventory SET location = ?, equiploc = -1, listnumber = ? WHERE itemid = ? AND serverid = ?`,
				loc, next, mv.ItemID, m.ch.serverID); err != nil {
				return err
			}
		}
		before := map[string]any{"location": from, "equiploc": equip, "listnumber": list}
		after := map[string]any{"location": loc, "equiploc": -1, "listnumber": next}
		if err := m.note("move", "charinventory", int64(mv.ItemID), before, after, name); err != nil {
			return err
		}
	}
	return nil
}

func listRuns(db *sql.DB) error {
	rows, err := db.Query(`SELECT run, charname, COUNT(*), MIN(at), MAX(undone) FROM charmigrate_log GROUP BY run ORDER BY MIN(id)`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			fmt.Println("No runs.")
			return nil
		}
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var run, name, at, undone string
		var n int
		if err := rows.Scan(&run, &name, &n, &at, &undone); err != nil {
			return err
		}
		state := ""
		if undone != "" {
			state = " (undone " + undone + ")"
		}
		fmt.Printf("%s  %s  %d changes  %s%s\n", run, name, n, at, state)
	}
	return rows.Err()
}

type logRow struct {
	id                    int64
	serverID              uint32
	kind, tbl, before, af string
	rowID                 int64
}

func undoRun(db *sql.DB, run string, force bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, serverid, kind, tbl, row_id, before, after FROM charmigrate_log
		WHERE run = ? AND undone = '' ORDER BY id DESC`, run)
	if err != nil {
		return err
	}
	var list []logRow
	for rows.Next() {
		var r logRow
		if err := rows.Scan(&r.id, &r.serverID, &r.kind, &r.tbl, &r.rowID, &r.before, &r.af); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	if len(list) == 0 {
		return fmt.Errorf("no change of run %q to take back", run)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	left := 0
	for _, r := range list {
		ok, err := undoOne(tx, r, force)
		if err != nil {
			return fmt.Errorf("change %d (%s %d): %w", r.id, r.kind, r.rowID, err)
		}
		if !ok {
			left++
			continue
		}
		if _, err := tx.Exec(`UPDATE charmigrate_log SET undone = ? WHERE id = ?`, stamp, r.id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("Run %s: %d changes taken back, %d left as they are\n", run, len(list)-left, left)
	return nil
}

// undoOne takes one change back: false (and said) when its row changed since
// and force is off.
func undoOne(tx *sql.Tx, r logRow, force bool) (bool, error) {
	var before, after map[string]any
	if r.before != "" {
		if err := json.Unmarshal([]byte(r.before), &before); err != nil {
			return false, err
		}
	}
	if r.af != "" {
		if err := json.Unmarshal([]byte(r.af), &after); err != nil {
			return false, err
		}
	}
	changed := func(what string) bool {
		if force {
			return false
		}
		fmt.Printf("  left: %s %d, changed since the run (%s)\n", r.kind, r.rowID, what)
		return true
	}
	switch r.kind {
	case "item":
		_, err := tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, r.rowID, r.serverID)
		return err == nil, err
	case "move":
		var loc int
		err := tx.QueryRow(`SELECT location FROM charinventory WHERE itemid = ? AND serverid = ?`, r.rowID, r.serverID).Scan(&loc)
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("  left: move %d, the item is gone\n", r.rowID)
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if float64(loc) != after["location"] && changed(fmt.Sprintf("location %d", loc)) {
			return false, nil
		}
		_, err = tx.Exec(`UPDATE charinventory SET location = ?, equiploc = ?, listnumber = ? WHERE itemid = ? AND serverid = ?`,
			before["location"], before["equiploc"], before["listnumber"], r.rowID, r.serverID)
		return err == nil, err
	case "spell":
		_, err := tx.Exec(`DELETE FROM spells WHERE id = ? AND serverid = ?`, r.rowID, r.serverID)
		return err == nil, err
	case "vitals":
		_, err := tx.Exec(`INSERT OR IGNORE INTO charvitals (serverid, hp, power, saved) VALUES (?, ?, ?, ?)`,
			r.serverID, before["hp"], before["power"], before["saved"])
		return err == nil, err
	case "mastery":
		var points int
		if err := tx.QueryRow(`SELECT points FROM charmastery WHERE serverid = ?`, r.serverID).Scan(&points); err != nil {
			return false, err
		}
		if float64(points) != after["Points"] && changed(fmt.Sprintf("points %d", points)) {
			return false, nil
		}
		if before == nil {
			_, err := tx.Exec(`DELETE FROM charmastery WHERE serverid = ?`, r.serverID)
			return err == nil, err
		}
		_, err := tx.Exec(`UPDATE charmastery SET points = ?, spent = ?, percent = ?, xp = ? WHERE serverid = ?`,
			before["Points"], before["Spent"], before["Percent"], before["XP"], r.serverID)
		return err == nil, err
	case "level":
		var level int
		if err := tx.QueryRow(`SELECT level FROM characters WHERE serverid = ?`, r.serverID).Scan(&level); err != nil {
			return false, err
		}
		if float64(level) != after["level"] && changed(fmt.Sprintf("level %d", level)) {
			return false, nil
		}
		_, err := tx.Exec(`UPDATE characters SET level = ?, totalXP = ? WHERE serverid = ?`, before["level"], before["totalXP"], r.serverID)
		return err == nil, err
	case "record":
		cols, err := patternColumns(tx)
		if err != nil {
			return false, err
		}
		row, err := patternRow(tx, cols, int(r.rowID))
		if err != nil {
			return false, err
		}
		var sets []string
		var args []any
		for c, v := range after {
			if fmt.Sprint(row[c]) != fmt.Sprint(v) && changed(fmt.Sprintf("%s %v", c, row[c])) {
				return false, nil
			}
			sets = append(sets, c+" = ?")
			args = append(args, before[c])
		}
		if len(sets) == 0 {
			return true, nil
		}
		_, err = tx.Exec(`UPDATE itempattern SET `+strings.Join(sets, ", ")+` WHERE patternid = ?`, append(args, r.rowID)...)
		return err == nil, err
	case "newrecord":
		var held int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE patternid = ?`, r.rowID).Scan(&held); err != nil {
			return false, err
		}
		if held > 0 && changed(fmt.Sprintf("%d items of it held", held)) {
			return false, nil
		}
		_, err := tx.Exec(`DELETE FROM itempattern WHERE patternid = ?`, r.rowID)
		return err == nil, err
	}
	return false, fmt.Errorf("a change of kind %q", r.kind)
}
