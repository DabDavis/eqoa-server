package database

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Quests: what a GM writes (quests, queststeps: the GM panel's Quests page)
// and how far each character has come in them (charquests). handlers/quests.go
// runs them: a quest's giver offers it when talked to, each step is done by
// talking to an NPC, handing an NPC items or tunar, killing creatures or
// reaching a place, and the journal (CLIENT's eight quest lines, 0x007C /
// 0x007D and the MemoryDump's list) shows the step to do.
//
// The two tables Return Home's schema brought (quests: questID, questNPC,
// questLog, questStep; playerquest) were never filled or read here. Empty,
// they are dropped for these; holding rows (another server's), they are kept
// as quests_returnhome and playerquest_returnhome.
const questSchema = `
CREATE TABLE IF NOT EXISTS quests (
	questid    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL DEFAULT '',
	enabled    INTEGER NOT NULL DEFAULT 0,
	giver      TEXT NOT NULL DEFAULT '',
	minlevel   INTEGER NOT NULL DEFAULT 1,
	maxlevel   INTEGER NOT NULL DEFAULT 60,
	classes    TEXT NOT NULL DEFAULT '',
	races      TEXT NOT NULL DEFAULT '',
	requires   TEXT NOT NULL DEFAULT '',
	repeatable INTEGER NOT NULL DEFAULT 0,
	offer      TEXT NOT NULL DEFAULT '',
	accept     TEXT NOT NULL DEFAULT '',
	decline    TEXT NOT NULL DEFAULT '',
	declined   TEXT NOT NULL DEFAULT '',
	started    TEXT NOT NULL DEFAULT '',
	source     TEXT NOT NULL DEFAULT '',
	notes      TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS queststeps (
	questid     INTEGER NOT NULL,
	step        INTEGER NOT NULL,
	kind        TEXT NOT NULL DEFAULT 'talk',
	target      TEXT NOT NULL DEFAULT '',
	npcid       INTEGER NOT NULL DEFAULT 0,
	items       TEXT NOT NULL DEFAULT '',
	tunar       INTEGER NOT NULL DEFAULT 0,
	count       INTEGER NOT NULL DEFAULT 1,
	world       INTEGER NOT NULL DEFAULT 0,
	x           REAL NOT NULL DEFAULT 0,
	y           REAL NOT NULL DEFAULT 0,
	z           REAL NOT NULL DEFAULT 0,
	radius      REAL NOT NULL DEFAULT 15,
	journal     TEXT NOT NULL DEFAULT '',
	answer      TEXT NOT NULL DEFAULT '',
	says        TEXT NOT NULL DEFAULT '',
	waiting     TEXT NOT NULL DEFAULT '',
	notyet      TEXT NOT NULL DEFAULT '',
	reminder    TEXT NOT NULL DEFAULT '',
	xp          INTEGER NOT NULL DEFAULT 0,
	rewardtunar INTEGER NOT NULL DEFAULT 0,
	rewarditems TEXT NOT NULL DEFAULT '',
	factions    TEXT NOT NULL DEFAULT '',
	spells      TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (questid, step)
);
CREATE TABLE IF NOT EXISTS charquests (
	serverid INTEGER NOT NULL,
	questid  INTEGER NOT NULL,
	step     INTEGER NOT NULL DEFAULT 1,
	count    INTEGER NOT NULL DEFAULT 0,
	done     INTEGER NOT NULL DEFAULT 0,
	slot     INTEGER NOT NULL DEFAULT 0,
	updated  TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (serverid, questid)
);
CREATE INDEX IF NOT EXISTS idx_queststeps_target ON queststeps(target);
CREATE INDEX IF NOT EXISTS idx_charquests_quest ON charquests(questid);
`

// initQuests moves Return Home's quest tables aside and makes the quest
// tables, adding to an older one the columns it lacks.
func (db *SQLiteDB) initQuests() error {
	if err := retireReturnHomeQuests(db.DB); err != nil {
		return err
	}
	if _, err := db.Exec(questSchema); err != nil {
		return err
	}
	for _, c := range []struct{ table, col string }{
		{"quests", "declined"}, {"queststeps", "notyet"}, {"queststeps", "reminder"},
	} {
		has, err := hasColumn(db.DB, c.table, c.col)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	return nil
}

// retireReturnHomeQuests drops Return Home's quests and playerquest when they
// are its shape and empty, or renames them when they hold rows.
func retireReturnHomeQuests(db *sql.DB) error {
	old, err := hasColumn(db, "quests", "questLog")
	if err != nil || !old {
		return err
	}
	rows := func(t string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	pq, _ := hasColumn(db, "playerquest", "questIndex")
	if rows("quests") == 0 && (!pq || rows("playerquest") == 0) {
		if pq {
			if _, err := db.Exec(`DROP TABLE playerquest`); err != nil {
				return err
			}
		}
		_, err := db.Exec(`DROP TABLE quests`)
		log.Printf("Quests: Return Home's empty quest tables replaced")
		return err
	}
	if pq {
		if _, err := db.Exec(`ALTER TABLE playerquest RENAME TO playerquest_returnhome`); err != nil {
			return err
		}
	}
	_, err = db.Exec(`ALTER TABLE quests RENAME TO quests_returnhome`)
	log.Printf("Quests: Return Home's quest tables kept as quests_returnhome and playerquest_returnhome")
	return err
}

// hasColumn is whether table t exists with column c.
func hasColumn(db *sql.DB, t, c string) (bool, error) {
	rs, err := db.Query(`SELECT name FROM pragma_table_info(?)`, t)
	if err != nil {
		return false, err
	}
	defer rs.Close()
	for rs.Next() {
		var name string
		if err := rs.Scan(&name); err != nil {
			return false, err
		}
		if strings.EqualFold(name, c) {
			return true, nil
		}
	}
	return false, rs.Err()
}

// The step kinds.
const (
	StepTalk  = "talk"  // talk to an NPC (target), answering it when the step has an answer
	StepShow  = "show"  // talk to an NPC (target) holding items, which are kept (Sandstorm's first warrior quest: the bracer)
	StepGive  = "give"  // hand an NPC (target) items and tunar
	StepKill  = "kill"  // kill count creatures named target
	StepReach = "reach" // come within radius of a place
)

// StepKinds is every step kind, in the order the panel offers them.
var StepKinds = []string{StepTalk, StepShow, StepGive, StepKill, StepReach}

// AtNPC is whether a step kind is done by talking to its NPC.
func AtNPC(kind string) bool { return kind == StepTalk || kind == StepShow || kind == StepGive }

// Quest is one quest and its steps, in order.
type Quest struct {
	ID         int
	Name       string
	Enabled    bool
	Giver      string // the NPC (by name) who offers it
	MinLevel   int
	MaxLevel   int
	Classes    []int // class ids that may take it; none: any
	Races      []int // race ids; none: any
	Requires   []int // quests finished first
	Repeatable bool
	// Offer is the giver's words offering it, a paragraph (lines between
	// blank lines) a question: its lines a box each, its last line the
	// question the paragraph's answer (Accept's line of that number) answers.
	Offer string
	// Accept is the player's answers taking it, a line a question (Sandstorm's
	// first warrior quest asks two); "" takes it when the giver is talked to.
	Accept  string
	Decline string // the player's answer refusing it, offered with each answer; "" none
	// Declined is the giver's reply to a refusal, said as written (Sandstorm's
	// Captain Norgam's carries no name before it).
	Declined string
	Started  string // the giver's words once it is taken
	Source   string
	Notes    string
	Steps    []QuestStep
}

// Step is the quest's step n (from 1), or nil.
func (q *Quest) Step(n int) *QuestStep {
	for i := range q.Steps {
		if q.Steps[i].Step == n {
			return &q.Steps[i]
		}
	}
	return nil
}

// Next is the step after n, or nil when n is the last.
func (q *Quest) Next(n int) *QuestStep {
	var best *QuestStep
	for i := range q.Steps {
		s := &q.Steps[i]
		if s.Step > n && (best == nil || s.Step < best.Step) {
			best = s
		}
	}
	return best
}

// First is the quest's first step, or nil.
func (q *Quest) First() *QuestStep { return q.Next(0) }

// QuestStep is one step of a quest.
type QuestStep struct {
	Quest  int
	Step   int
	Kind   string
	Target string // talk/give: the NPC's name; kill: the creature's
	NPCID  int    // talk/give/kill: only that spawn (npcs.id), 0 any of the name
	Items  []ItemCount
	Tunar  int
	Count  int
	World  int
	X, Y   float32
	Z      float32
	Radius float32
	// Journal is the journal's line while this is the step to do.
	Journal string
	// Answer is the player's answer that does a talk or give step; "" does
	// it on talking.
	Answer string
	// Says is the NPC's words when the step is done (a line a box).
	Says string
	// Waiting is the NPC's words when talked to on this step: its lines a box
	// each, its last line the question its Answer (the items held) or NotYet
	// (not yet) answers.
	Waiting string
	// NotYet is the player's answer when a show or give step's items are not
	// held (Sandstorm's "No, I am lost."); "" asks nothing then.
	NotYet string
	// Reminder is the NPC's words when the step's items are not held (after
	// NotYet when there is one).
	Reminder string
	// The rewards when the step is done.
	XP          int
	RewardTunar int
	RewardItems []ItemCount
	Factions    []FactionDelta
	Spells      []int
}

// ItemCount is count of an item (itempattern.patternid).
type ItemCount struct {
	PatternID int
	Count     int
}

// FactionDelta is a change to a faction standing.
type FactionDelta struct {
	Faction string
	Delta   int
}

// CharQuest is how far a character has come in a quest.
type CharQuest struct {
	ServerID uint32
	QuestID  int
	Step     int // the step to do; 0 when finished
	Count    int // creatures killed so far on a kill step
	Done     int // times finished
	Slot     int // the journal's order: its line came after every smaller one
	Updated  string
}

// Active is whether the quest is under way (it has a journal line).
func (c *CharQuest) Active() bool { return c.Step > 0 }

// QuestStore reads and writes the quests and the characters' progress.
type QuestStore struct{ db *SQLiteDB }

// NewQuestStore is the store over db.
func NewQuestStore(db *SQLiteDB) *QuestStore { return &QuestStore{db: db} }

// LoadQuests reads every quest, drafts too, with its steps in order.
func (s *QuestStore) LoadQuests() ([]Quest, error) {
	rs, err := s.db.Query(`SELECT questid, name, enabled, giver, minlevel, maxlevel, classes, races, requires,
		repeatable, offer, accept, decline, declined, started, source, notes FROM quests ORDER BY questid`)
	if err != nil {
		return nil, err
	}
	var out []Quest
	byID := map[int]int{}
	for rs.Next() {
		var q Quest
		var enabled, repeatable int
		var classes, races, requires string
		if err := rs.Scan(&q.ID, &q.Name, &enabled, &q.Giver, &q.MinLevel, &q.MaxLevel, &classes, &races, &requires,
			&repeatable, &q.Offer, &q.Accept, &q.Decline, &q.Declined, &q.Started, &q.Source, &q.Notes); err != nil {
			rs.Close()
			return nil, err
		}
		q.Enabled, q.Repeatable = enabled != 0, repeatable != 0
		q.Classes, q.Races, q.Requires = ParseIntList(classes), ParseIntList(races), ParseIntList(requires)
		byID[q.ID] = len(out)
		out = append(out, q)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	srs, err := s.db.Query(`SELECT questid, step, kind, target, npcid, items, tunar, count, world, x, y, z, radius,
		journal, answer, says, waiting, notyet, reminder, xp, rewardtunar, rewarditems, factions, spells
		FROM queststeps ORDER BY questid, step`)
	if err != nil {
		return nil, err
	}
	defer srs.Close()
	for srs.Next() {
		var st QuestStep
		var items, rewardItems, factions, spells string
		if err := srs.Scan(&st.Quest, &st.Step, &st.Kind, &st.Target, &st.NPCID, &items, &st.Tunar, &st.Count, &st.World,
			&st.X, &st.Y, &st.Z, &st.Radius, &st.Journal, &st.Answer, &st.Says, &st.Waiting, &st.NotYet, &st.Reminder,
			&st.XP, &st.RewardTunar,
			&rewardItems, &factions, &spells); err != nil {
			return nil, err
		}
		st.Kind = strings.ToLower(strings.TrimSpace(st.Kind))
		st.Items, st.RewardItems = ParseItemCounts(items), ParseItemCounts(rewardItems)
		st.Factions, st.Spells = ParseFactionDeltas(factions), ParseIntList(spells)
		if i, ok := byID[st.Quest]; ok {
			out[i].Steps = append(out[i].Steps, st)
		}
	}
	return out, srs.Err()
}

// CharQuests is a character's quests, under way and finished.
func (s *QuestStore) CharQuests(serverID uint32) ([]CharQuest, error) {
	rs, err := s.db.Query(`SELECT serverid, questid, step, count, done, slot, updated FROM charquests
		WHERE serverid = ? ORDER BY slot, questid`, serverID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []CharQuest
	for rs.Next() {
		var c CharQuest
		if err := rs.Scan(&c.ServerID, &c.QuestID, &c.Step, &c.Count, &c.Done, &c.Slot, &c.Updated); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rs.Err()
}

// SaveCharQuest writes a character's progress in a quest.
func (s *QuestStore) SaveCharQuest(c *CharQuest) error {
	c.Updated = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid, questid) DO UPDATE SET step = excluded.step, count = excluded.count,
			done = excluded.done, slot = excluded.slot, updated = excluded.updated`,
		c.ServerID, c.QuestID, c.Step, c.Count, c.Done, c.Slot, c.Updated)
	return err
}

// DeleteCharQuest forgets a character's progress in a quest.
func (s *QuestStore) DeleteCharQuest(serverID uint32, questID int) error {
	_, err := s.db.Exec(`DELETE FROM charquests WHERE serverid = ? AND questid = ?`, serverID, questID)
	return err
}

// ParseIntList reads "1, 2, 3" (anything else between the numbers is a
// separator).
func ParseIntList(s string) []int {
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '-' }) {
		if n, err := strconv.Atoi(f); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// FormatIntList writes a list as ParseIntList reads it.
func FormatIntList(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}

// ParseItemCounts reads "4866*2, 8373": an item's patternid, then how many
// after a * (1 without).
func ParseItemCounts(s string) []ItemCount {
	var out []ItemCount
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		id, n := f, "1"
		if i := strings.IndexAny(f, "*x×"); i >= 0 {
			id, n = strings.TrimSpace(f[:i]), strings.TrimSpace(f[i+len(string([]rune(f[i:])[0])):])
		}
		pid, err1 := strconv.Atoi(id)
		cnt, err2 := strconv.Atoi(n)
		if err1 != nil || err2 != nil || cnt < 1 {
			continue
		}
		out = append(out, ItemCount{PatternID: pid, Count: cnt})
	}
	return out
}

// FormatItemCounts writes a list as ParseItemCounts reads it.
func FormatItemCounts(v []ItemCount) string {
	parts := make([]string, len(v))
	for i, c := range v {
		if c.Count == 1 {
			parts[i] = strconv.Itoa(c.PatternID)
		} else {
			parts[i] = fmt.Sprintf("%d*%d", c.PatternID, c.Count)
		}
	}
	return strings.Join(parts, ", ")
}

// ParseFactionDeltas reads "Freeport Militia:+10, Lockjaw:-5" (a faction's
// name as the factions table has it, a colon, the change).
func ParseFactionDeltas(s string) []FactionDelta {
	var out []FactionDelta
	for _, f := range strings.Split(s, ",") {
		i := strings.LastIndex(f, ":")
		if i < 0 {
			continue
		}
		name := strings.TrimSpace(f[:i])
		d, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(f[i+1:]), "+"))
		if name == "" || err != nil || d == 0 {
			continue
		}
		out = append(out, FactionDelta{Faction: name, Delta: d})
	}
	return out
}

// FormatFactionDeltas writes a list as ParseFactionDeltas reads it.
func FormatFactionDeltas(v []FactionDelta) string {
	parts := make([]string, len(v))
	for i, d := range v {
		parts[i] = fmt.Sprintf("%s:%+d", d.Faction, d.Delta)
	}
	return strings.Join(parts, ", ")
}
