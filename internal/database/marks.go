package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// marks.go — Heirloom Marks (work board 1927, a GM's spec, the Heirloom Test Server alone behind
// world.heirloom_marks): marks a character below 60, or a 60 reliving, earns for quests and kills,
// spent at an Heirloom Keeper on the Ring of Guises, an Heirloom Rank and Mastery Tomes
// (handlers/marks.go says who earns what). The tables:
//
//   - charmarks: a character's marks (unspent), the fraction carried (frac, 0-0.999), every mark it
//     ever earned (lifetime), its hour of kill marks (kill_hour_start, unix seconds, and how many
//     kill marks it holds, before bonuses), its Ring of Guises rank (0-14) and the guise picked
//     (a spell id), and its Heirloom Rank (0-6);
//   - charmarks_log: each award and spending (source quest, kill, spend or gm), its whole marks,
//     what it was for (ref: a quest id, an NPC row, what was bought) and its bonus factor;
//   - hotzones: the GMs' Hot zones (a camp, by its place and radius, or a quest set, by an import
//     key prefix or quest ids), each with its bonus (default +35%) and a note players are shown;
//     camp names the #tele camp a zone was made on (its anchor's landing), "" none;
//   - heirloomrankac: the AC an Heirloom Rank adds to each worn heirloom armour piece, by class and
//     rank;
//   - charrelive: a 60's Relive, its level, the experience into it and its own debt, since when,
//     and whether it is on now (active; the row stays for "Resume at N" after it ends);
//   - quests.marks (NULL: computed, QuestMarksFor) and charquests.taken (unix seconds the quest was
//     taken; NULL on rows older than the column).
//
// Each shard keeps its own; only Heirloom's server reads them (world.heirloom_marks).

const marksSchema = `
CREATE TABLE IF NOT EXISTS charmarks (
	serverid        INTEGER PRIMARY KEY,
	marks           INTEGER NOT NULL DEFAULT 0,
	frac            REAL NOT NULL DEFAULT 0,
	lifetime        INTEGER NOT NULL DEFAULT 0,
	kill_hour_start INTEGER NOT NULL DEFAULT 0,
	kill_hour_marks INTEGER NOT NULL DEFAULT 0,
	ring_rank       INTEGER NOT NULL DEFAULT 0,
	ring_guise      INTEGER NOT NULL DEFAULT 0,
	heirloom_rank   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS charmarks_log (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	serverid INTEGER NOT NULL,
	at       TEXT NOT NULL,
	source   TEXT NOT NULL,
	amount   INTEGER NOT NULL,
	ref      TEXT NOT NULL DEFAULT '',
	bonus    REAL NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_charmarks_log_char ON charmarks_log(serverid);
CREATE TABLE IF NOT EXISTS charrelive (
	serverid    INTEGER PRIMARY KEY,
	level       INTEGER NOT NULL,
	xp_in_level INTEGER NOT NULL DEFAULT 0,
	debt        INTEGER NOT NULL DEFAULT 0,
	since       INTEGER NOT NULL DEFAULT 0,
	active      INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS hotzones (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	kind       TEXT NOT NULL DEFAULT 'camp',
	world      INTEGER NOT NULL DEFAULT 0,
	x          REAL NOT NULL DEFAULT 0,
	y          REAL NOT NULL DEFAULT 0,
	z          REAL NOT NULL DEFAULT 0,
	radius     REAL NOT NULL DEFAULT 150,
	quests     TEXT NOT NULL DEFAULT '',
	bonus_pct  INTEGER NOT NULL DEFAULT 35,
	note       TEXT NOT NULL DEFAULT '',
	announce   INTEGER NOT NULL DEFAULT 0,
	created_by TEXT NOT NULL DEFAULT '',
	updated    TEXT NOT NULL DEFAULT '',
	camp       TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS heirloomrankac (
	class INTEGER NOT NULL,
	rank  INTEGER NOT NULL,
	ac    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (class, rank)
);`

// ensureMarks makes the tables and the two columns (initSchema, after quests and charquests).
func (db *SQLiteDB) ensureMarks() error {
	if _, err := db.Exec(marksSchema); err != nil {
		return fmt.Errorf("heirloom marks: %w", err)
	}
	for _, c := range []struct{ t, col, def string }{
		{"quests", "marks", "INTEGER"},
		{"charquests", "taken", "INTEGER"},
	} {
		has, err := hasColumn(db.DB, c.t, c.col)
		if err != nil {
			return fmt.Errorf("heirloom marks: %s: %w", c.t, err)
		}
		var exists int
		db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, c.t).Scan(&exists)
		if has || exists == 0 {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE ` + c.t + ` ADD COLUMN ` + c.col + ` ` + c.def); err != nil {
			return fmt.Errorf("heirloom marks: %s.%s: %w", c.t, c.col, err)
		}
	}
	return nil
}

// CharMarks is a character's charmarks row; one with none is the zero row.
type CharMarks struct {
	ServerID      uint32
	Marks         int
	Frac          float64
	Lifetime      int
	KillHourStart int64
	KillHourMarks int
	RingRank      int
	RingGuise     int
	HeirloomRank  int
}

const charMarksCols = `serverid, marks, frac, lifetime, kill_hour_start, kill_hour_marks, ring_rank, ring_guise, heirloom_rank`

type rowScanner interface{ Scan(...any) error }

func scanCharMarks(r rowScanner) (CharMarks, error) {
	var m CharMarks
	err := r.Scan(&m.ServerID, &m.Marks, &m.Frac, &m.Lifetime, &m.KillHourStart, &m.KillHourMarks, &m.RingRank, &m.RingGuise, &m.HeirloomRank)
	return m, err
}

// CharMarksOf is a character's row (the zero row, its serverid set, when it has none).
func (db *SQLiteDB) CharMarksOf(serverID uint32) (CharMarks, error) {
	return charMarksTx(db.DB, serverID)
}

type querier interface {
	QueryRow(string, ...any) *sql.Row
}

func charMarksTx(q querier, serverID uint32) (CharMarks, error) {
	m, err := scanCharMarks(q.QueryRow(`SELECT `+charMarksCols+` FROM charmarks WHERE serverid = ?`, serverID))
	if errors.Is(err, sql.ErrNoRows) {
		return CharMarks{ServerID: serverID}, nil
	}
	return m, err
}

func writeCharMarks(tx *sql.Tx, m CharMarks) error {
	_, err := tx.Exec(`INSERT INTO charmarks (`+charMarksCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET marks = excluded.marks, frac = excluded.frac, lifetime = excluded.lifetime,
			kill_hour_start = excluded.kill_hour_start, kill_hour_marks = excluded.kill_hour_marks,
			ring_rank = excluded.ring_rank, ring_guise = excluded.ring_guise, heirloom_rank = excluded.heirloom_rank`,
		m.ServerID, m.Marks, m.Frac, m.Lifetime, m.KillHourStart, m.KillHourMarks, m.RingRank, m.RingGuise, m.HeirloomRank)
	return err
}

func logMarks(tx *sql.Tx, serverID uint32, at time.Time, source string, amount int, ref string, bonus float64) error {
	_, err := tx.Exec(`INSERT INTO charmarks_log (serverid, at, source, amount, ref, bonus) VALUES (?, ?, ?, ?, ?, ?)`,
		serverID, at.UTC().Format(time.RFC3339), source, amount, ref, bonus)
	return err
}

// The kill marks' hour (work 1927 section 4): KillMarksAnHour kill marks, before bonuses, in
// KillHour of world time from kill_hour_start.
const (
	KillMarksAnHour = 38
	KillHour        = 3600
)

// KillHourNow is m's kill marks this hour at now: the count, 0 once the hour is over.
func (m CharMarks) KillHourNow(now time.Time) int {
	if now.Unix()-m.KillHourStart >= KillHour {
		return 0
	}
	return m.KillHourMarks
}

// MarksAward is one award: base marks (a quest's, or 1 for a kill's won roll), the bonus factor
// (group x hot zone), what it was for.
type MarksAward struct {
	Source string // "quest", "kill" or "gm"
	Base   float64
	Bonus  float64
	Ref    string
}

// AwardMarks adds an award to a character's row and logs it, one transaction: base x bonus plus
// the fraction carried, its whole marks added and the rest carried. A kill's award counts on its
// hour (a new hour begun when the last is over) and is refused (false) past KillMarksAnHour. It
// returns the whole marks given and the row after.
func (db *SQLiteDB) AwardMarks(serverID uint32, a MarksAward, now time.Time) (int, CharMarks, bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, CharMarks{}, false, err
	}
	defer tx.Rollback()
	m, err := charMarksTx(tx, serverID)
	if err != nil {
		return 0, m, false, err
	}
	if a.Source == "kill" {
		if now.Unix()-m.KillHourStart >= KillHour {
			m.KillHourStart, m.KillHourMarks = now.Unix(), 0
		}
		if m.KillHourMarks >= KillMarksAnHour {
			return 0, m, false, nil
		}
		m.KillHourMarks++
	}
	if a.Bonus <= 0 {
		a.Bonus = 1
	}
	total := a.Base*a.Bonus + m.Frac
	n := int(math.Floor(total + 1e-9))
	m.Frac = math.Max(0, math.Min(0.999, total-float64(n)))
	m.Marks += n
	m.Lifetime += n
	if err := writeCharMarks(tx, m); err != nil {
		return 0, m, false, err
	}
	if err := logMarks(tx, serverID, now, a.Source, n, a.Ref, a.Bonus); err != nil {
		return 0, m, false, err
	}
	return n, m, true, tx.Commit()
}

// ErrNotEnoughMarks is a spending a character's marks do not cover.
var ErrNotEnoughMarks = errors.New("not enough marks")

// SpendMarks takes price marks and lets change set what was bought on the row, one transaction
// with its log line (source spend, ref what). A change's error refuses it all; a price above the
// marks held is ErrNotEnoughMarks.
func (db *SQLiteDB) SpendMarks(serverID uint32, price int, what string, change func(*CharMarks) error, now time.Time) (CharMarks, error) {
	tx, err := db.Begin()
	if err != nil {
		return CharMarks{}, err
	}
	defer tx.Rollback()
	m, err := charMarksTx(tx, serverID)
	if err != nil {
		return m, err
	}
	if price > m.Marks {
		return m, ErrNotEnoughMarks
	}
	m.Marks -= price
	if change != nil {
		if err := change(&m); err != nil {
			return m, err
		}
	}
	if err := writeCharMarks(tx, m); err != nil {
		return m, err
	}
	if price != 0 || what != "" {
		if err := logMarks(tx, serverID, now, "spend", -price, what, 1); err != nil {
			return m, err
		}
	}
	return m, tx.Commit()
}

// HotZone is a hotzones row.
type HotZone struct {
	ID        int
	Name      string
	Enabled   bool
	Kind      string // "camp" or "quests"
	World     int
	X, Y, Z   float32
	Radius    float32
	Quests    string
	BonusPct  int
	Note      string
	Announce  bool
	CreatedBy string
	Updated   string
	Camp      string
}

// The kinds of a hot zone.
const (
	HotZoneCamp   = "camp"
	HotZoneQuests = "quests"
)

const hotZoneCols = `id, name, enabled, kind, world, x, y, z, radius, quests, bonus_pct, note, announce, created_by, updated, camp`

// HotZones is every row, by id.
func (db *SQLiteDB) HotZones() ([]HotZone, error) {
	rs, err := db.Query(`SELECT ` + hotZoneCols + ` FROM hotzones ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []HotZone
	for rs.Next() {
		var z HotZone
		var en, an int
		if err := rs.Scan(&z.ID, &z.Name, &en, &z.Kind, &z.World, &z.X, &z.Y, &z.Z, &z.Radius, &z.Quests, &z.BonusPct,
			&z.Note, &an, &z.CreatedBy, &z.Updated, &z.Camp); err != nil {
			return nil, err
		}
		z.Enabled, z.Announce = en != 0, an != 0
		out = append(out, z)
	}
	return out, rs.Err()
}

// FitsQuest is whether a quest zone takes a quest of id and import key: its key starts with one of
// the zone's prefixes, or its id is listed.
func (z HotZone) FitsQuest(id int, key string) bool {
	if z.Kind != HotZoneQuests {
		return false
	}
	for _, part := range strings.Split(z.Quests, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			if n == id {
				return true
			}
			continue
		}
		if key != "" && strings.HasPrefix(strings.ToLower(key), strings.ToLower(part)) {
			return true
		}
	}
	return false
}

// FitsSpawn is whether a camp zone takes a kill of a creature spawned at (x, z) of world.
func (z HotZone) FitsSpawn(world int, x, zz float32) bool {
	if z.Kind != HotZoneCamp || z.World != world {
		return false
	}
	dx, dz := x-z.X, zz-z.Z
	return dx*dx+dz*dz <= z.Radius*z.Radius
}

// hotZoneRow is a zone as a row map, for the audit line.
func hotZoneRow(tx *sql.Tx, id int64) (map[string]any, error) {
	return fullRow(tx, `SELECT * FROM hotzones WHERE id = ?`, id)
}

// whoNamed fills a GM's name from its account when the edit carries none (the audit names a GM).
func whoNamed(tx *sql.Tx, who GMEdit) GMEdit {
	if who.Username == "" {
		tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username)
	}
	return who
}

// SaveHotZone writes a zone (a new one when its ID is 0) with its gm_audit line in the change's
// transaction, as the panel's row edits are, so the panel lists and reverts it. It returns the id.
func (db *SQLiteDB) SaveHotZone(z HotZone, who GMEdit, detail string) (int, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	who = whoNamed(tx, who)
	z.Updated = time.Now().UTC().Format(time.RFC3339)
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	var before map[string]any
	id := int64(z.ID)
	action := "insert"
	if z.ID == 0 {
		res, err := tx.Exec(`INSERT INTO hotzones (name, enabled, kind, world, x, y, z, radius, quests, bonus_pct, note, announce, created_by, updated, camp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			z.Name, b(z.Enabled), z.Kind, z.World, z.X, z.Y, z.Z, z.Radius, z.Quests, z.BonusPct, z.Note, b(z.Announce), z.CreatedBy, z.Updated, z.Camp)
		if err != nil {
			return 0, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return 0, err
		}
	} else {
		action = "update"
		if before, err = hotZoneRow(tx, id); err != nil {
			return 0, err
		}
		if before == nil {
			return 0, fmt.Errorf("no hot zone %d", z.ID)
		}
		if _, err := tx.Exec(`UPDATE hotzones SET name = ?, enabled = ?, kind = ?, world = ?, x = ?, y = ?, z = ?, radius = ?, quests = ?,
			bonus_pct = ?, note = ?, announce = ?, updated = ?, camp = ? WHERE id = ?`,
			z.Name, b(z.Enabled), z.Kind, z.World, z.X, z.Y, z.Z, z.Radius, z.Quests, z.BonusPct, z.Note, b(z.Announce), z.Updated, z.Camp, id); err != nil {
			return 0, err
		}
	}
	after, err := hotZoneRow(tx, id)
	if err != nil {
		return 0, err
	}
	if _, err := auditRowChange(tx, who, action, "hotzones", map[string]any{"id": id}, before, after, detail); err != nil {
		return 0, err
	}
	return int(id), tx.Commit()
}

// DeleteHotZone deletes a zone with its gm_audit line.
func (db *SQLiteDB) DeleteHotZone(id int, who GMEdit, detail string) error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	who = whoNamed(tx, who)
	before, err := hotZoneRow(tx, int64(id))
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("no hot zone %d", id)
	}
	if _, err := tx.Exec(`DELETE FROM hotzones WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := auditRowChange(tx, who, "delete", "hotzones", map[string]any{"id": int64(id)}, before, nil, detail); err != nil {
		return err
	}
	return tx.Commit()
}

// HeirloomRankAC is the table: the AC by class and rank.
func (db *SQLiteDB) HeirloomRankAC() (map[[2]int]int, error) {
	rs, err := db.Query(`SELECT class, rank, ac FROM heirloomrankac`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[[2]int]int{}
	for rs.Next() {
		var c, r, ac int
		if err := rs.Scan(&c, &r, &ac); err != nil {
			return nil, err
		}
		out[[2]int{c, r}] = ac
	}
	return out, rs.Err()
}

// CharRelive is a charrelive row: a 60's Relive.
type CharRelive struct {
	ServerID  uint32
	Level     int
	XPInLevel int
	Debt      int
	Since     int64
	Active    bool
}

// CharReliveOf is a character's row; ok false when it has none.
func (db *SQLiteDB) CharReliveOf(serverID uint32) (CharRelive, bool, error) {
	r := CharRelive{ServerID: serverID}
	var active int
	err := db.QueryRow(`SELECT level, xp_in_level, debt, since, active FROM charrelive WHERE serverid = ?`, serverID).
		Scan(&r.Level, &r.XPInLevel, &r.Debt, &r.Since, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	r.Active = active != 0
	return r, err == nil, err
}

// SaveCharRelive writes a character's row.
func (db *SQLiteDB) SaveCharRelive(r CharRelive) error {
	active := 0
	if r.Active {
		active = 1
	}
	_, err := db.Exec(`INSERT INTO charrelive (serverid, level, xp_in_level, debt, since, active) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid) DO UPDATE SET level = excluded.level, xp_in_level = excluded.xp_in_level, debt = excluded.debt,
			since = excluded.since, active = excluded.active`,
		r.ServerID, r.Level, r.XPInLevel, r.Debt, r.Since, active)
	return err
}

// deleteCharMarks forgets a deleted character's marks and Relive (its log stays, for the record).
func (db *SQLiteDB) deleteCharMarks(serverID uint32) error {
	for _, t := range []string{"charmarks", "charrelive"} {
		if _, err := db.Exec(`DELETE FROM `+t+` WHERE serverid = ?`, serverID); err != nil {
			return err
		}
	}
	return nil
}

// --- quest marks (work 1927 section 3) ---

// questMarksPerMinute is a minute's estimate in marks: 100 an hour.
const questMarksPerMinute = 100.0 / 60.0

// QuestMarksLeast is the least a paying quest gives.
const QuestMarksLeast = 8

// QuestMinutes is a quest's estimate in minutes from its steps: 4 for each talk, show, give or
// reach step; 0.5 x max(count, 1) for each kill step; 0.5 x max(count, 1) / chance for each loot
// step (a chance of 0 or 1 counts as 1); at least 5.
func QuestMinutes(q *Quest) float64 {
	var m float64
	for _, st := range q.Steps {
		switch st.Kind {
		case StepTalk, StepShow, StepGive, StepReach:
			m += 4
		case StepKill:
			m += 0.5 * float64(max(st.Count, 1))
		case StepLoot:
			n := st.Count
			if n <= 0 && len(st.Items) > 0 {
				n = st.Items[0].Count
			}
			c := st.Chance
			if c <= 0 || c >= 1 {
				c = 1
			}
			m += 0.5 * float64(max(n, 1)) / c
		}
	}
	return math.Max(m, 5)
}

// QuestKindFactor is the kind factor of an import key (the part before its first '/'), and
// whether such a quest pays at all (the level 60 epics, "60", never do).
func QuestKindFactor(key string) (float64, bool) {
	kind, _, _ := strings.Cut(strings.TrimSpace(key), "/")
	switch kind {
	case "60":
		return 0, false
	case "30,40,49":
		return 2.5, true
	case "Weres", "Portal", "ZP", "TankExemplar":
		return 2, true
	case "23-47", "Misc", "53-59":
		return 1.5, true
	}
	return 1, true
}

// QuestMarksComputed is what a quest pays by its steps and kind, before the real-time limit and
// the bonuses: estimate x 100/60 x kind factor, rounded, at least QuestMarksLeast; 0 for a
// level 60 epic.
func QuestMarksComputed(q *Quest, key string) int {
	f, ok := QuestKindFactor(key)
	if !ok {
		return 0
	}
	return max(int(math.Round(QuestMinutes(q)*questMarksPerMinute*f)), QuestMarksLeast)
}

// QuestMarksFor is what a quest pays before the real-time limit and the bonuses: its marks column
// when set (Quest.Marks), else QuestMarksComputed; 0 for a level 60 epic either way.
func QuestMarksFor(q *Quest, key string) int {
	if _, ok := QuestKindFactor(key); !ok {
		return 0
	}
	if q.Marks != nil {
		return max(*q.Marks, 0)
	}
	return QuestMarksComputed(q, key)
}

// QuestMarksLimit is the real-time limit: a quest taken since minutes ago pays at most twice
// that, in marks.
func QuestMarksLimit(minutes float64) int {
	return int(math.Floor(2 * math.Max(minutes, 0) * questMarksPerMinute))
}

// QuestImportKeys is every imported quest's key (tools/import_quests.py's log), by quest id; a
// database without the log has none.
func (db *SQLiteDB) QuestImportKeys() (map[int]string, error) {
	out := map[int]string{}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'quest_import_quests'`).Scan(&n); err != nil || n == 0 {
		return out, err
	}
	rs, err := db.Query(`SELECT key, questid FROM quest_import_quests`)
	if err != nil {
		return out, err
	}
	defer rs.Close()
	for rs.Next() {
		var k string
		var id int
		if err := rs.Scan(&k, &id); err != nil {
			return out, err
		}
		out[id] = k
	}
	return out, rs.Err()
}

// QuestTaken is when a character took a quest (charquests.taken), ok false when not known.
func (db *SQLiteDB) QuestTaken(serverID uint32, questID int) (time.Time, bool) {
	var t sql.NullInt64
	if err := db.QueryRow(`SELECT taken FROM charquests WHERE serverid = ? AND questid = ?`, serverID, questID).Scan(&t); err != nil || !t.Valid || t.Int64 <= 0 {
		return time.Time{}, false
	}
	return time.Unix(t.Int64, 0), true
}
