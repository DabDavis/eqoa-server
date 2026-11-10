// Package charmove moves a character in from another server by a plan a GM
// writes: its level, its mastery points, its class's spells, item records put
// right or made for what it held that this server lacks, its items into the
// bank or the pack, and rows it holds moved between them. One transaction, every
// change a row of charmigrate_log (the row before and after as JSON); Undo takes
// a run back in reverse order and leaves a row changed since unless forced.
// cmd/charmigrate runs it from a shell; the server runs a GM's approved transfer
// (database/transfers.go) at the character's next login.
//
// The character must be out of the world while a plan applies: the logout
// save would write its level back over the new one, and the game keeps its
// inventory in memory while it plays.
package charmove

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Plan is a migration: the character by name (it must exist here, made by
// its player, so its race, sex and look are theirs), and what it gets.
type Plan struct {
	Character string `json:"character"`
	// CharacterID names the character by its serverid instead (a transfer's).
	CharacterID uint32 `json:"serverid,omitempty"`
	Note        string `json:"note"`
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
	Records []RecordFix `json:"records"`
	// NewRecords are item records made, each a copy of Like with Set over it,
	// for what the character held that this server has no record of.
	NewRecords []NewRecord `json:"new_records"`
	// Items are given: a record by Pattern, or a new record by its key, Count
	// rows of one each (default 1).
	Items []Item `json:"items"`
	// To is where the items go: "bank" (the default), "pack", or "auto" (the
	// pack while it has room, then the bank).
	To string `json:"to"`
	// Moves are rows the character holds already, moved to the bank or the
	// pack (unequipped, at the end of its list): an earlier run's items
	// taken out of the bank, say.
	Moves []Move `json:"moves"`
	// Quests are quests marked done, a transfer's path, epic, were line and
	// flags (QuestDone, quests.go).
	Quests []QuestDone `json:"quests,omitempty"`
	// Vampire makes the character a vampire (table charvampire, source
	// "transfer"), unless it is one.
	Vampire bool `json:"vampire,omitempty"`
	// Imported gives the items' gear as its imported copies, at ImportedShare of its
	// records' stats (database.ImportedCopyTx; petition 1314): a transfer's,
	// never a GM's grant (handlers/transfers.go decides it at the login).
	Imported bool `json:"imported,omitempty"`
	// PackSlots and BankSlots are how many records the character's pack (worn
	// ones among them) and bank hold as its client keeps them: CLIENT's 40
	// (PackCap, BankCap) when 0, or a larger client's (handlers/itemlists.go).
	// "pack" and "auto" fill the pack to PackSlots and the bank is checked
	// against BankSlots. Not the plan file's: the login that applies a
	// transfer sets them (handlers/transfers.go).
	PackSlots int `json:"-"`
	BankSlots int `json:"-"`
}

type Move struct {
	ItemID int    `json:"itemid"`
	To     string `json:"to"`
}

type RecordFix struct {
	Pattern int            `json:"pattern"`
	Set     map[string]any `json:"set"`
	Why     string         `json:"why"`
}

type NewRecord struct {
	Ref  string         `json:"ref"`
	Like int            `json:"like"`
	Set  map[string]any `json:"set"`
	Why  string         `json:"why"`
}

type Item struct {
	Pattern int    `json:"pattern"`
	New     string `json:"new"`
	Count   int    `json:"count"`
}

// PackCap is how many records CLIENT's pack holds, worn ones among them, and
// BankCap its bank's (VIItemList's 40, handlers/itemlist.go invListCap): what a
// plan fills to and checks against unless it says its client holds more
// (Plan.PackSlots, BankSlots). More than BankCap in a bank needs -force; past
// what the character's client holds, the rest is kept for it at its login
// (handlers/itemlists.go), never sent to a client that cannot hold it.
const (
	PackCap = 40
	BankCap = 40
)

const LogSchema = `CREATE TABLE IF NOT EXISTS charmigrate_log (
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

// ReadPlan reads a plan from a JSON file and checks it.
func ReadPlan(path string) (Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	return ParsePlan(b)
}

// ParsePlan reads a plan from JSON (an unknown field refused) and checks it.
func ParsePlan(b []byte) (Plan, error) {
	var p Plan
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("the plan: %w", err)
	}
	return p, p.Check()
}

// Check says what is wrong with a plan, nil when nothing is.
func (p Plan) Check() error {
	if strings.TrimSpace(p.Character) == "" && p.CharacterID == 0 {
		return errors.New("the plan names no character")
	}
	if p.Level < 0 || p.Level > 60 {
		return fmt.Errorf("level %d: 1-60, or 0 to leave it", p.Level)
	}
	if p.MasteryPoints < 0 || p.MasteryPoints > entity.MasteryCap {
		return fmt.Errorf("mastery_points %d: 0-%d", p.MasteryPoints, entity.MasteryCap)
	}
	switch p.To {
	case "", "bank", "pack", "auto":
	default:
		return fmt.Errorf("to %q: bank, pack or auto", p.To)
	}
	for _, m := range p.Moves {
		if m.ItemID <= 0 || (m.To != "bank" && m.To != "pack") {
			return fmt.Errorf("a move names an itemid and \"bank\" or \"pack\" (%d %q)", m.ItemID, m.To)
		}
	}
	refs := map[string]bool{}
	for _, n := range p.NewRecords {
		if n.Ref == "" || refs[n.Ref] {
			return fmt.Errorf("a new record needs a ref of its own (%q)", n.Ref)
		}
		refs[n.Ref] = true
	}
	for _, it := range p.Items {
		if (it.Pattern == 0) == (it.New == "") {
			return errors.New("an item names a pattern or a new record's ref, one of them")
		}
		if it.New != "" && !refs[it.New] {
			return fmt.Errorf("an item names new record %q, which the plan does not make", it.New)
		}
		if it.Count < 0 || it.Count > 20 {
			return fmt.Errorf("count %d: 1-20", it.Count)
		}
	}
	return checkQuests(p.Quests)
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
	// imported is Plan.Imported
	imported bool
	// packCap, bankCap: Plan.PackSlots and BankSlots, CLIENT's 40 for 0
	packCap, bankCap int
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

// Migrate applies a plan in one transaction (a dry run when apply is false:
// what it would do said to out, nothing written) and gives the run's id, by
// which Undo takes it back.
func Migrate(db *sql.DB, p Plan, apply, force bool, out io.Writer) (string, error) {
	if err := p.Check(); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if apply {
		if _, err := tx.Exec(LogSchema); err != nil {
			return "", fmt.Errorf("the log: %w", err)
		}
	}
	var ch character
	if p.CharacterID != 0 {
		err = tx.QueryRow(`SELECT serverid, charName, tclass, race, level, totalXP FROM characters WHERE serverid = ?`,
			p.CharacterID).Scan(&ch.serverID, &ch.name, &ch.class, &ch.race, &ch.level, &ch.totalXP)
	} else {
		err = tx.QueryRow(`SELECT serverid, charName, tclass, race, level, totalXP FROM characters WHERE charName = ? COLLATE NOCASE`,
			p.Character).Scan(&ch.serverID, &ch.name, &ch.class, &ch.race, &ch.level, &ch.totalXP)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no character %q (its player makes it first)", firstNonEmpty(p.Character, fmt.Sprint(p.CharacterID)))
	}
	if err != nil {
		return "", err
	}
	if p.CharacterID != 0 && p.Character != "" && !strings.EqualFold(p.Character, ch.name) {
		return "", fmt.Errorf("character %d is %q, not %q", ch.serverID, ch.name, p.Character)
	}
	run, err := migrateTx(tx, ch, p, apply, force, out)
	if err != nil || !apply {
		return run, err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return run, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// migrateTx is Migrate's work inside its transaction.
func migrateTx(tx *sql.Tx, ch character, p Plan, apply, force bool, out io.Writer) (string, error) {
	now := time.Now().UTC()
	m := &migration{tx: tx, ch: ch, apply: apply, out: out, stamp: now.Format(time.RFC3339),
		run: now.Format("20060102T150405Z") + "-" + strings.ToLower(ch.name), imported: p.Imported,
		packCap: max(p.PackSlots, PackCap), bankCap: max(p.BankSlots, BankCap)}
	if apply {
		// a run's id is its own: another in the same second gets -2, -3...
		base := m.run
		for n := 2; ; n++ {
			var used int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM charmigrate_log WHERE run = ?`, m.run).Scan(&used); err != nil {
				return "", err
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
		return "", err
	}
	made, err := m.newRecords(p.NewRecords)
	if err != nil {
		return "", err
	}
	if err := m.setLevel(p.Level); err != nil {
		return "", err
	}
	if err := m.addMastery(p.MasteryPoints); err != nil {
		return "", err
	}
	if p.ClassSpells {
		if err := m.classSpells(); err != nil {
			return "", err
		}
	}
	if err := m.questsDone(p.Quests); err != nil {
		return "", err
	}
	if p.Vampire {
		if err := m.makeVampire(); err != nil {
			return "", err
		}
	}
	if p.FullVitals {
		if err := m.dropVitals(); err != nil {
			return "", err
		}
	}
	if err := m.giveItems(p.Items, made, p.To, force); err != nil {
		return "", err
	}
	if err := m.moveItems(p.Moves); err != nil {
		return "", err
	}
	if !apply {
		m.say("Dry run done: %d changes; apply to make them", m.logged)
		return "", nil
	}
	m.say("Done: %d changes, logged as run %s (Undo %s takes them back)", m.logged, m.run, m.run)
	return m.run, nil
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

func (m *migration) fixRecords(fixes []RecordFix) error {
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
func (m *migration) newRecords(news []NewRecord) (map[string]int, error) {
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

func (m *migration) giveItems(items []Item, made map[string]int, to string, force bool) error {
	if len(items) == 0 {
		return nil
	}
	var pack, bank int
	if err := m.tx.QueryRow(`SELECT COALESCE(SUM(location = 0), 0), COALESCE(SUM(location = 1), 0) FROM charinventory WHERE serverid = ?`,
		m.ch.serverID).Scan(&pack, &bank); err != nil {
		return err
	}
	rows := 0
	for _, it := range items {
		rows += max(it.Count, 1)
	}
	// where each row goes: the pack while it has room ("auto"), else as asked
	toPack := 0
	switch to {
	case "pack":
		if pack+rows > m.packCap {
			return fmt.Errorf("the pack holds %d records and %d more would pass its %d: give to the bank or auto", pack, rows, m.packCap)
		}
		toPack = rows
	case "auto":
		toPack = min(rows, max(m.packCap-pack, 0))
	}
	if toBank := rows - toPack; bank+toBank > m.bankCap && !force {
		return fmt.Errorf("the bank holds %d records and %d more would pass its %d (force to go on)", bank, toBank, m.bankCap)
	}
	m.say("Items: %d into the pack (holding %d), %d into the bank (holding %d)", toPack, pack, rows-toPack, bank)
	given := 0
	for _, it := range items {
		pattern := it.Pattern
		if it.New != "" {
			pattern = made[it.New]
		}
		if pattern > 0 && m.imported {
			copyID, made, err := database.ImportedCopyTx(m.tx, pattern, "transfer run "+m.run, time.Now())
			if err != nil {
				return fmt.Errorf("item record %d's imported copy: %w", pattern, err)
			}
			if made {
				m.say("  record %d's imported copy made: %d", pattern, copyID)
			}
			pattern = copyID
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
			loc, where := 1, "bank"
			if given < toPack {
				loc, where = 0, "pack"
			}
			given++
			var itemID int64 = -1
			if m.apply {
				// a non-stacking item of charges (itemeffect.charges) holds them all, as
				// one bought does (database.ItemCharges): 0 would make its first use its last
				res, err := m.tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
					VALUES (?, 1, ?, COALESCE((SELECT e.charges FROM itemeffect e JOIN itempattern p ON p.patternid = e.patternid
						WHERE e.patternid = ? AND p.maxstack < 2), 0), ?, -1, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))`,
					m.ch.serverID, maxHP, pattern, pattern, loc, m.ch.serverID, loc)
				if err != nil {
					return fmt.Errorf("giving %q: %w", name, err)
				}
				if itemID, err = res.LastInsertId(); err != nil {
					return err
				}
			}
			m.say("  %s (record %d), the %s", name, pattern, where)
			if err := m.note("item", "charinventory", itemID, nil, map[string]any{"patternid": pattern, "location": loc}, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// moveItems moves rows the character holds to the bank or the pack,
// unequipped, at the end of that list; the pack held to its client's (packCap).
func (m *migration) moveItems(moves []Move) error {
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
			if pack >= m.packCap {
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

// Runs lists the runs made, to out.
func Runs(db *sql.DB, out io.Writer) error {
	rows, err := db.Query(`SELECT run, charname, COUNT(*), MIN(at), MAX(undone) FROM charmigrate_log GROUP BY run ORDER BY MIN(id)`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			fmt.Fprintln(out, "No runs.")
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
		fmt.Fprintf(out, "%s  %s  %d changes  %s%s\n", run, name, n, at, state)
	}
	return rows.Err()
}

type logRow struct {
	id                    int64
	serverID              uint32
	kind, tbl, before, af string
	rowID                 int64
}

// Undo takes a run back in reverse order: a row changed since the run is left as
// it is and said to out, unless force. It gives how many changes were taken
// back and how many left.
func Undo(db *sql.DB, run string, force bool, out io.Writer) (taken, left int, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, serverid, kind, tbl, row_id, before, after FROM charmigrate_log
		WHERE run = ? AND undone = '' ORDER BY id DESC`, run)
	if err != nil {
		return 0, 0, err
	}
	var list []logRow
	for rows.Next() {
		var r logRow
		if err := rows.Scan(&r.id, &r.serverID, &r.kind, &r.tbl, &r.rowID, &r.before, &r.af); err != nil {
			rows.Close()
			return 0, 0, err
		}
		list = append(list, r)
	}
	rows.Close()
	if len(list) == 0 {
		return 0, 0, fmt.Errorf("no change of run %q to take back", run)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	for _, r := range list {
		ok, err := undoOne(tx, r, force, out)
		if err != nil {
			return 0, 0, fmt.Errorf("change %d (%s %d): %w", r.id, r.kind, r.rowID, err)
		}
		if !ok {
			left++
			continue
		}
		if _, err := tx.Exec(`UPDATE charmigrate_log SET undone = ? WHERE id = ?`, stamp, r.id); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	fmt.Fprintf(out, "Run %s: %d changes taken back, %d left as they are\n", run, len(list)-left, left)
	return len(list) - left, left, nil
}

// undoOne takes one change back: false (and said) when its row changed since
// and force is off.
func undoOne(tx *sql.Tx, r logRow, force bool, out io.Writer) (bool, error) {
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
		fmt.Fprintf(out, "  left: %s %d, changed since the run (%s)\n", r.kind, r.rowID, what)
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
			fmt.Fprintf(out, "  left: move %d, the item is gone\n", r.rowID)
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
	case "quest":
		return undoQuest(tx, r, before, after, changed)
	case "vampire":
		return undoVampire(tx, r)
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
