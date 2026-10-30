package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
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
//
// questunlocks is the trips quests open (QuestUnlock): a travel NPC's route
// (handlers/waysender.go) offered only to a player who has finished one of the
// quests that unlock it.
//
// queststeptargets is the other creatures a kill or loot step counts besides
// its own target (QuestStep.Also): a journal's "from the ravens and magpies"
// as two names, the step's target "raven" and a row "magpie" (the user,
// 2026-10-30: the beak a step dropped only off ravens never dropped off the
// magpies its journal names; tools/fix_quest_targets.py).
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
	notes      TEXT NOT NULL DEFAULT '',
	events     TEXT NOT NULL DEFAULT '',
	factionreq TEXT NOT NULL DEFAULT ''
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
	chance      REAL NOT NULL DEFAULT 0,
	events      TEXT NOT NULL DEFAULT '',
	byclass     TEXT NOT NULL DEFAULT '',
	choices     TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (questid, step)
);
CREATE TABLE IF NOT EXISTS questunlocks (
	questid INTEGER NOT NULL,
	npc     TEXT NOT NULL,
	dest    TEXT NOT NULL,
	PRIMARY KEY (questid, npc, dest)
);
CREATE TABLE IF NOT EXISTS queststeptargets (
	questid INTEGER NOT NULL,
	step    INTEGER NOT NULL,
	target  TEXT NOT NULL,
	PRIMARY KEY (questid, step, target)
);
CREATE TABLE IF NOT EXISTS charquests (
	serverid INTEGER NOT NULL,
	questid  INTEGER NOT NULL,
	step     INTEGER NOT NULL DEFAULT 1,
	count    INTEGER NOT NULL DEFAULT 0,
	done     INTEGER NOT NULL DEFAULT 0,
	slot     INTEGER NOT NULL DEFAULT 0,
	updated  TEXT NOT NULL DEFAULT '',
	picks    TEXT NOT NULL DEFAULT '',
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
	for _, c := range []struct{ table, col, decl string }{
		{"quests", "declined", `TEXT NOT NULL DEFAULT ''`}, {"queststeps", "notyet", `TEXT NOT NULL DEFAULT ''`},
		{"queststeps", "reminder", `TEXT NOT NULL DEFAULT ''`},
		{"quests", "events", `TEXT NOT NULL DEFAULT ''`}, {"queststeps", "events", `TEXT NOT NULL DEFAULT ''`},
		{"queststeps", "chance", `REAL NOT NULL DEFAULT 0`},
		{"quests", "factionreq", `TEXT NOT NULL DEFAULT ''`}, {"queststeps", "byclass", `TEXT NOT NULL DEFAULT ''`},
		{"queststeps", "choices", `TEXT NOT NULL DEFAULT ''`},
		{"charquests", "picks", `TEXT NOT NULL DEFAULT ''`},
	} {
		has, err := hasColumn(db.DB, c.table, c.col)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.col + ` ` + c.decl); err != nil {
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
	StepKill  = "kill"  // kill count creatures named target (or one of its others, Also)
	StepLoot  = "loot"  // kill creatures named target (or Also) until count of the item (items) are taken off their corpses during the step
	StepReach = "reach" // come within radius of a place
)

// StepKinds is every step kind, in the order the panel offers them.
var StepKinds = []string{StepTalk, StepShow, StepGive, StepKill, StepLoot, StepReach}

// When a quest's events happen: a quest's when it is taken and when it is
// finished, a step's when it becomes the step to do and when it is done.
const (
	EventAccept = "accept"
	EventFinish = "finish"
	EventStart  = "start"
	EventDone   = "done"
)

// What an event does.
const (
	EventPopup    = "popup"    // a window the player closes: text, its one answer
	EventSay      = "say"      // an NPC's words to the player, a box a line
	EventGive     = "give"     // items into the player's inventory
	EventSpawn    = "spawn"    // a copy of an NPC beside the player, for the step
	EventVisual   = "visual"   // a spell visual played on the player
	EventTeleport = "teleport" // the player moved to a place
	EventVampire  = "vampire"  // the player made a vampire (charvampire, entity/vampire.go): the bats' infection
)

// QuestEventWhens are the times a quest's own events may name, StepEventWhens
// a step's; QuestEventActions every action, in the order the panel offers them.
var (
	QuestEventWhens   = []string{EventAccept, EventFinish}
	StepEventWhens    = []string{EventStart, EventDone}
	QuestEventActions = []string{EventPopup, EventSay, EventGive, EventSpawn, EventVisual, EventTeleport, EventVampire}
)

// QuestEvent is one thing a quest or a step does at one of its times, kept as
// a JSON array in the events column.
type QuestEvent struct {
	On      string  `json:"on"`                // when: accept or finish (a quest's), start or done (a step's)
	Do      string  `json:"do"`                // what: popup, say, give, spawn, visual, teleport, vampire
	Text    string  `json:"text,omitempty"`    // popup, say: the words (%p the player's name)
	Answer  string  `json:"answer,omitempty"`  // popup: its one answer ("OK" when none)
	NPC     string  `json:"npc,omitempty"`     // say: who says it (nobody: no name before it); spawn: whom
	Items   string  `json:"items,omitempty"`   // give: patternid*count, comma separated
	Hostile bool    `json:"hostile,omitempty"` // spawn: it may be attacked, and attacks the player
	FX      string  `json:"fx,omitempty"`      // visual: its DictID (0x1DAEB5B6)
	Seconds float64 `json:"seconds,omitempty"` // visual: its casting part (0 to 30)
	World   int     `json:"world,omitempty"`   // teleport: the world
	X       float32 `json:"x,omitempty"`       // teleport: the place
	Y       float32 `json:"y,omitempty"`
	Z       float32 `json:"z,omitempty"`
}

// ParseQuestEvents reads an events column: a JSON array of QuestEvent, ""
// none. When and what are lowered and trimmed.
func ParseQuestEvents(s string) ([]QuestEvent, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []QuestEvent
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].On = strings.ToLower(strings.TrimSpace(out[i].On))
		out[i].Do = strings.ToLower(strings.TrimSpace(out[i].Do))
	}
	return out, nil
}

// FormatQuestEvents writes events as ParseQuestEvents reads them ("" none).
func FormatQuestEvents(v []QuestEvent) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// EventProblems is what is wrong with an events column for a quest (step
// false) or a step: unreadable JSON, a time or an action there is not, an
// event missing what it needs. It checks nothing in the database.
func EventProblems(s string, step bool) []string {
	evs, err := ParseQuestEvents(s)
	if err != nil {
		return []string{"its events are not a JSON list of events: " + err.Error()}
	}
	whens := QuestEventWhens
	if step {
		whens = StepEventWhens
	}
	var out []string
	add := func(i int, format string, a ...any) {
		out = append(out, fmt.Sprintf("event %d: ", i+1)+fmt.Sprintf(format, a...))
	}
	for i, e := range evs {
		if !containsString(whens, e.On) {
			add(i, "on %q is not one of %s", e.On, strings.Join(whens, ", "))
		}
		switch e.Do {
		case EventPopup, EventSay:
			if strings.TrimSpace(e.Text) == "" {
				add(i, "%s says nothing (text)", e.Do)
			}
		case EventGive:
			if len(ParseItemCounts(e.Items)) == 0 {
				add(i, "give names no items (patternid*count)")
			}
		case EventSpawn:
			if strings.TrimSpace(e.NPC) == "" {
				add(i, "spawn names no NPC")
			}
			if !step {
				add(i, "a spawn is a step's: it goes when the step is done")
			}
		case EventVisual:
			if _, ok := ParseVisualID(e.FX); !ok {
				add(i, "visual's fx %q is not a DictID (0x1DAEB5B6)", e.FX)
			}
		case EventTeleport:
			if e.X == 0 && e.Z == 0 {
				add(i, "teleport's place is 0, 0")
			}
		case EventVampire:
		default:
			add(i, "do %q is not one of %s", e.Do, strings.Join(QuestEventActions, ", "))
		}
	}
	return out
}

// ParseVisualID reads a visual's DictID as handlers.ParseVisual does (0x and
// 1 to 8 hex digits, or exactly eight hex digits); 0 is no visual.
func ParseVisualID(s string) (uint32, bool) {
	t := strings.TrimSpace(s)
	switch {
	case len(t) > 2 && (t[:2] == "0x" || t[:2] == "0X"):
		t = t[2:]
		if len(t) > 8 {
			return 0, false
		}
	case len(t) != 8:
		return 0, false
	}
	v, err := strconv.ParseUint(t, 16, 32)
	if err != nil || v == 0 {
		return 0, false
	}
	return uint32(v), true
}

func containsString(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

// LootChance is a loot step's chance that a kill drops its item: the step's,
// every kill when it gives none (0) or more than 1.
func LootChance(c float64) float64 {
	if c <= 0 || c > 1 {
		return 1
	}
	return c
}

// AtNPC is whether a step kind is done by talking to its NPC.
func AtNPC(kind string) bool { return kind == StepTalk || kind == StepShow || kind == StepGive }

// Hunts is whether a step kind is done by killing creatures (kill, loot):
// the kinds that count several (QuestStep.Also).
func Hunts(kind string) bool { return kind == StepKill || kind == StepLoot }

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
	Requires   []int // every quest it names to be finished first (each alternative too)
	Repeatable bool
	// RequireGroups is what it asks finished first (the requires column,
	// ParseRequires): every group met, a group by any one of its quests,
	// so "a, b|c" is a, and b or c. A quest several classes take whose
	// hand-in each class gets from a quest of its own (a recruiter's token)
	// asks for those as one group.
	RequireGroups [][]int
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
	// Events are what happens when it is taken (accept) and finished (finish).
	Events []QuestEvent
	// FactionReqs are the standings it asks for (the factionreq column): it
	// is offered, and its steps at its giver are done, only while the player
	// meets every one.
	FactionReqs []FactionReq
	// Unlocks are the trips it opens (the questunlocks table).
	Unlocks []QuestUnlock
	Steps   []QuestStep
}

// EventsOn is the events of one time, in order.
func EventsOn(evs []QuestEvent, on string) []QuestEvent {
	var out []QuestEvent
	for _, e := range evs {
		if e.On == on {
			out = append(out, e)
		}
	}
	return out
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

// Required is what the quest asks finished first, by group (RequireGroups;
// a quest made without them, each of its Requires a group of its own).
func (q *Quest) Required() [][]int {
	if q.RequireGroups != nil || len(q.Requires) == 0 {
		return q.RequireGroups
	}
	out := make([][]int, len(q.Requires))
	for i, id := range q.Requires {
		out[i] = []int{id}
	}
	return out
}

// QuestStep is one step of a quest.
type QuestStep struct {
	Quest  int
	Step   int
	Kind   string
	Target string // talk/give: the NPC's name; kill, loot: the creature's
	NPCID  int    // talk/give/kill/loot: only that spawn (npcs.id) of Target, 0 any of the name
	// Also is the other creatures a kill or loot step counts, at any of their
	// spawns (the queststeptargets table): a talk, show, give or reach step's
	// are kept and counted for nothing
	Also []string
	// Items are what a show or give step wants held, and a loot step's item
	// (its first; Count of it, else the item's own count, is to be taken off
	// corpses during the step).
	Items []ItemCount
	Tunar int
	Count int
	// Chance is a loot step's chance (0-1) that a kill drops its item for a
	// player on the step (LootChance: 0 is every kill).
	Chance float64
	// Events are what happens when it becomes the step to do (start) and when
	// it is done (done).
	Events []QuestEvent
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
	// ByClass is what a player of a class (its id) is given besides those
	// (the byclass column; the guides' "Rewards by Class").
	ByClass map[int]ClassReward
	// Choices is the pick a talk, show or give step asks for when it is done
	// (the choices column; a guide's "choose your epic weapon").
	Choices StepChoices
}

// StepChoices is a step's question and the options the player picks among
// when the step is done at its NPC: an option's rewards are given besides
// the step's own, and an option that stays does not do the step.
type StepChoices struct {
	Ask     string
	Options []StepOption
}

// StepOption is one answer of a step's choice.
type StepOption struct {
	Answer string // the player's answer (63 characters show)
	Says   string // the NPC's words when it is picked, a line a box
	Stay   bool   // picked, the step is not done (a wrong answer: talk again)
	Reward ClassReward
}

// MaxStepOptions is how many answers a choice has room for (CLIENT's
// Msg0034 shows six).
const MaxStepOptions = 6

// stepChoicesJSON is StepChoices as the choices column keeps it, an
// option's items and spells written as the step's own columns have them.
type stepChoicesJSON struct {
	Ask     string `json:"ask,omitempty"`
	Options []struct {
		Answer string `json:"answer"`
		Says   string `json:"says,omitempty"`
		Stay   bool   `json:"stay,omitempty"`
		XP     int    `json:"xp,omitempty"`
		Tunar  int    `json:"tunar,omitempty"`
		Items  string `json:"items,omitempty"`
		Spells string `json:"spells,omitempty"`
	} `json:"options"`
}

// ParseStepChoices reads a choices column: {"ask": "<the question>",
// "options": [{"answer", "says", "stay", "xp", "tunar", "items":
// "patternid*count, ...", "spells": "id, ..."}]} ("" none). Options past
// MaxStepOptions and without an answer are left out (ChoiceProblems names
// them).
func ParseStepChoices(s string) (StepChoices, error) {
	if strings.TrimSpace(s) == "" {
		return StepChoices{}, nil
	}
	var raw stepChoicesJSON
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return StepChoices{}, err
	}
	out := StepChoices{Ask: strings.TrimSpace(raw.Ask)}
	for _, o := range raw.Options {
		if strings.TrimSpace(o.Answer) == "" || len(out.Options) == MaxStepOptions {
			continue
		}
		out.Options = append(out.Options, StepOption{Answer: strings.TrimSpace(o.Answer), Says: o.Says, Stay: o.Stay,
			Reward: ClassReward{XP: o.XP, Tunar: o.Tunar, Items: ParseItemCounts(o.Items), Spells: ParseIntList(o.Spells)}})
	}
	return out, nil
}

// ChoiceProblems is what is wrong with a choices column on a step of a kind:
// unreadable JSON, a choice on a step not done at an NPC, no options or more
// than CLIENT shows, an answer missing or longer than 63 characters, items
// or spells not written as the step's own are, negative experience or tunar.
// It checks nothing in the database.
func ChoiceProblems(s, kind string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var raw stepChoicesJSON
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return []string{`its choice is not {"ask", "options": [{"answer", "says", "stay", "xp", "tunar", "items", "spells"}]}: ` + err.Error()}
	}
	var out []string
	if !AtNPC(kind) {
		out = append(out, fmt.Sprintf("a %s step asks no choice (talk, show and give steps do, at their NPC)", kind))
	}
	switch {
	case len(raw.Options) == 0:
		out = append(out, "its choice has no options")
	case len(raw.Options) > MaxStepOptions:
		out = append(out, fmt.Sprintf("its choice has %d options: the client shows %d", len(raw.Options), MaxStepOptions))
	}
	for i, o := range raw.Options {
		where := fmt.Sprintf("option %d", i+1)
		switch n := len([]rune(strings.TrimSpace(o.Answer))); {
		case n == 0:
			out = append(out, where+" has no answer")
		case n > 63:
			out = append(out, where+"'s answer is longer than 63 characters: the client cuts it")
		}
		if o.XP < 0 || o.Tunar < 0 {
			out = append(out, where+": experience and tunar are not below 0")
		}
		if len(nonEmptyParts(o.Items)) != len(ParseItemCounts(o.Items)) {
			out = append(out, fmt.Sprintf("%s: items %q are not patternid*count, comma separated", where, o.Items))
		}
		if len(nonEmptyParts(o.Spells)) != len(ParseIntList(o.Spells)) {
			out = append(out, fmt.Sprintf("%s: spells %q are not spell ids, comma separated", where, o.Spells))
		}
	}
	return out
}

// RewardFor is the step's rewards for a player of a class: its own and then
// its class's (ByClass), each spell once.
func (s *QuestStep) RewardFor(class int) ClassReward {
	out := ClassReward{XP: s.XP, Tunar: s.RewardTunar,
		Items: append([]ItemCount(nil), s.RewardItems...), Spells: append([]int(nil), s.Spells...)}
	if cr, ok := s.ByClass[class]; ok {
		out = out.Plus(cr)
	}
	return out
}

// Plus is r and o together: experience and tunar added, o's items after r's,
// each spell once.
func (r ClassReward) Plus(o ClassReward) ClassReward {
	out := ClassReward{XP: r.XP + o.XP, Tunar: r.Tunar + o.Tunar,
		Items: append(append([]ItemCount(nil), r.Items...), o.Items...), Spells: append([]int(nil), r.Spells...)}
	for _, sp := range o.Spells {
		seen := false
		for _, have := range out.Spells {
			seen = seen || have == sp
		}
		if !seen {
			out.Spells = append(out.Spells, sp)
		}
	}
	return out
}

// ClassReward is a step's rewards for one class: experience, tunar, items
// and spells. RewardFor makes one of a step's own rewards too.
type ClassReward struct {
	XP     int
	Tunar  int
	Items  []ItemCount
	Spells []int
}

// classRewardJSON is a ClassReward as the byclass column keeps it: the items
// and spells in the forms the step's own columns have them (rewarditems,
// spells), so a GM reads one way of writing them.
type classRewardJSON struct {
	XP     int    `json:"xp,omitempty"`
	Tunar  int    `json:"tunar,omitempty"`
	Items  string `json:"items,omitempty"`
	Spells string `json:"spells,omitempty"`
}

// MaxClassID is the last class id (14, the alchemist: root CLAUDE.md's
// verified ids).
const MaxClassID = 14

// ParseClassRewards reads a byclass column: a JSON object by class id, each
// {"xp", "tunar", "items": "patternid*count, ...", "spells": "id, ..."}
// ("" none). A key that is not a class id is left out (ClassRewardProblems
// names it).
func ParseClassRewards(s string) (map[int]ClassReward, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var raw map[string]classRewardJSON
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	out := map[int]ClassReward{}
	for k, v := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || id < 0 || id > MaxClassID {
			continue
		}
		out[id] = ClassReward{XP: v.XP, Tunar: v.Tunar, Items: ParseItemCounts(v.Items), Spells: ParseIntList(v.Spells)}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// FormatClassRewards writes rewards by class as ParseClassRewards reads them
// ("" none), class ids in order.
func FormatClassRewards(m map[int]ClassReward) string {
	if len(m) == 0 {
		return ""
	}
	raw := map[string]classRewardJSON{}
	for id, cr := range m {
		raw[strconv.Itoa(id)] = classRewardJSON{XP: cr.XP, Tunar: cr.Tunar, Items: FormatItemCounts(cr.Items), Spells: FormatIntList(cr.Spells)}
	}
	b, err := json.Marshal(raw) // encoding/json sorts a map's keys
	if err != nil {
		return ""
	}
	return string(b)
}

// ClassRewardProblems is what is wrong with a byclass column: unreadable
// JSON, a key that is no class id (0 Warrior ... 14 Alchemist), negative
// experience or tunar, items or spells not written as the step's own are.
// It checks nothing in the database.
func ClassRewardProblems(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var raw map[string]classRewardJSON
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return []string{`its rewards by class are not a JSON object of class ids ({"10": {"xp": 100, "items": "4866*2", "spells": "12"}}): ` + err.Error()}
	}
	var out []string
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := raw[k]
		id, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || id < 0 || id > MaxClassID {
			out = append(out, fmt.Sprintf("rewards for class %q: no such class (0 Warrior ... 14 Alchemist)", k))
			continue
		}
		if v.XP < 0 || v.Tunar < 0 {
			out = append(out, fmt.Sprintf("rewards for class %d: experience and tunar are not below 0", id))
		}
		if n := len(nonEmptyParts(v.Items)); n != len(ParseItemCounts(v.Items)) {
			out = append(out, fmt.Sprintf("rewards for class %d: items %q are not patternid*count, comma separated", id, v.Items))
		}
		if n := len(nonEmptyParts(v.Spells)); n != len(ParseIntList(v.Spells)) {
			out = append(out, fmt.Sprintf("rewards for class %d: spells %q are not spell ids, comma separated", id, v.Spells))
		}
	}
	return out
}

// nonEmptyParts is a list's comma-separated entries that say something.
func nonEmptyParts(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if strings.TrimSpace(f) != "" {
			out = append(out, f)
		}
	}
	return out
}

// FactionReq is a standing a quest asks for: at least Min with a faction,
// and at most Max when HasMax.
type FactionReq struct {
	Faction string
	Min     int
	Max     int
	HasMax  bool
}

// Met is whether a standing meets it.
func (r FactionReq) Met(standing int) bool {
	return standing >= r.Min && (!r.HasMax || standing <= r.Max)
}

// ParseFactionReqs reads a factionreq column: "Freeport Militia:100,
// Lockjaw:-500..0" (a faction's name as the factions table has it, a colon,
// the lowest standing, and after ".." the highest). An entry that cannot be
// read is left out (FactionReqProblems names it).
func ParseFactionReqs(s string) []FactionReq {
	var out []FactionReq
	for _, f := range strings.Split(s, ",") {
		if r, ok := parseFactionReq(f); ok {
			out = append(out, r)
		}
	}
	return out
}

func parseFactionReq(f string) (FactionReq, bool) {
	i := strings.LastIndex(f, ":")
	if i < 0 {
		return FactionReq{}, false
	}
	name := strings.TrimSpace(f[:i])
	lo, hi, ranged := strings.Cut(strings.TrimSpace(f[i+1:]), "..")
	lowest, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(lo), "+"))
	if name == "" || err != nil {
		return FactionReq{}, false
	}
	r := FactionReq{Faction: name, Min: lowest}
	if ranged {
		highest, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(hi), "+"))
		if err != nil || highest < lowest {
			return FactionReq{}, false
		}
		r.Max, r.HasMax = highest, true
	}
	return r, true
}

// FormatFactionReqs writes requirements as ParseFactionReqs reads them.
func FormatFactionReqs(v []FactionReq) string {
	parts := make([]string, len(v))
	for i, r := range v {
		parts[i] = fmt.Sprintf("%s:%d", r.Faction, r.Min)
		if r.HasMax {
			parts[i] += fmt.Sprintf("..%d", r.Max)
		}
	}
	return strings.Join(parts, ", ")
}

// FactionReqProblems is what is wrong with a factionreq column: an entry that
// is not "Faction Name:min" or "Faction Name:min..max", a range that runs
// backwards. It checks nothing in the database.
func FactionReqProblems(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if strings.TrimSpace(f) == "" {
			continue
		}
		if _, ok := parseFactionReq(f); !ok {
			out = append(out, fmt.Sprintf("the faction requirement %q is not Faction Name:min or Faction Name:min..max (min not above max)", strings.TrimSpace(f)))
		}
	}
	return out
}

// QuestUnlock is a trip a quest opens: the travel NPC NPC (by name, as npcs
// spells it) offers its route to Dest (the route's label, handlers/
// waysender.go) only to a player who has finished this quest or another that
// unlocks the same trip, while one of those quests is in the game.
type QuestUnlock struct {
	Quest int
	NPC   string
	Dest  string
}

// Targets is the names a step's NPC or creatures go by: its Target, then for
// a kill or loot step each of Also not already said (trimmed, any case: as
// names are matched), each trimmed.
func (s *QuestStep) Targets() []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		n = strings.TrimSpace(n)
		k := strings.ToLower(n)
		if n == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, n)
	}
	add(s.Target)
	if Hunts(s.Kind) {
		for _, n := range s.Also {
			add(n)
		}
	}
	return out
}

// TargetWords is a step's creatures said as one: "raven", "raven or magpie",
// "crow, raven or magpie".
func (s *QuestStep) TargetWords() string {
	t := s.Targets()
	switch len(t) {
	case 0:
		return ""
	case 1:
		return t[0]
	}
	return strings.Join(t[:len(t)-1], ", ") + " or " + t[len(t)-1]
}

// LootItem is a loot step's item and how many of it are to be taken: Count,
// else the item's own count; ok false when the step names no item.
func (s *QuestStep) LootItem() (patternID, need int, ok bool) {
	if len(s.Items) == 0 {
		return 0, 0, false
	}
	need = s.Count
	if need < 1 {
		need = max(s.Items[0].Count, 1)
	}
	return s.Items[0].PatternID, need, true
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
	// Picks is the options picked at the quest's choices ("2:1, 3:0": step 2
	// its second option, step 3 its first; FormatPicks), so a rehabilitator
	// undoing a path takes back what was picked (handlers/rehab.go)
	Picks string
}

// Active is whether the quest is under way (it has a journal line).
func (c *CharQuest) Active() bool { return c.Step > 0 }

// QuestStore reads and writes the quests and the characters' progress.
type QuestStore struct{ db *SQLiteDB }

// NewQuestStore is the store over db.
func NewQuestStore(db *SQLiteDB) *QuestStore { return &QuestStore{db: db} }

// MarkVampire is a quest's vampire event on the character (vampire.go).
func (s *QuestStore) MarkVampire(serverID uint32, quest string) (bool, error) {
	return s.db.MarkVampire(serverID, "quest", "infected by "+quest)
}

// LoadQuests reads every quest, drafts too, with its steps in order.
func (s *QuestStore) LoadQuests() ([]Quest, error) {
	rs, err := s.db.Query(`SELECT questid, name, enabled, giver, minlevel, maxlevel, classes, races, requires,
		repeatable, offer, accept, decline, declined, started, source, notes, events, factionreq FROM quests ORDER BY questid`)
	if err != nil {
		return nil, err
	}
	var out []Quest
	byID := map[int]int{}
	for rs.Next() {
		var q Quest
		var enabled, repeatable int
		var classes, races, requires, events, factionReq string
		if err := rs.Scan(&q.ID, &q.Name, &enabled, &q.Giver, &q.MinLevel, &q.MaxLevel, &classes, &races, &requires,
			&repeatable, &q.Offer, &q.Accept, &q.Decline, &q.Declined, &q.Started, &q.Source, &q.Notes, &events, &factionReq); err != nil {
			rs.Close()
			return nil, err
		}
		q.Enabled, q.Repeatable = enabled != 0, repeatable != 0
		q.Classes, q.Races, q.Requires = ParseIntList(classes), ParseIntList(races), ParseIntList(requires)
		q.RequireGroups = ParseRequires(requires)
		if q.Events, err = ParseQuestEvents(events); err != nil {
			log.Printf("Quests: quest %d's events are not read: %v", q.ID, err)
		}
		q.FactionReqs = ParseFactionReqs(factionReq)
		if ps := FactionReqProblems(factionReq); len(ps) > 0 {
			// left out, the quest asks less than written: said, as the panel says it
			log.Printf("Quests: quest %d: %s", q.ID, strings.Join(ps, "; "))
		}
		byID[q.ID] = len(out)
		out = append(out, q)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return nil, err
	}
	srs, err := s.db.Query(`SELECT questid, step, kind, target, npcid, items, tunar, count, world, x, y, z, radius,
		journal, answer, says, waiting, notyet, reminder, xp, rewardtunar, rewarditems, factions, spells, chance, events, byclass,
		choices FROM queststeps ORDER BY questid, step`)
	if err != nil {
		return nil, err
	}
	for srs.Next() {
		var st QuestStep
		var items, rewardItems, factions, spells, events, byClass, choices string
		if err := srs.Scan(&st.Quest, &st.Step, &st.Kind, &st.Target, &st.NPCID, &items, &st.Tunar, &st.Count, &st.World,
			&st.X, &st.Y, &st.Z, &st.Radius, &st.Journal, &st.Answer, &st.Says, &st.Waiting, &st.NotYet, &st.Reminder,
			&st.XP, &st.RewardTunar,
			&rewardItems, &factions, &spells, &st.Chance, &events, &byClass, &choices); err != nil {
			srs.Close()
			return nil, err
		}
		st.Kind = strings.ToLower(strings.TrimSpace(st.Kind))
		st.Items, st.RewardItems = ParseItemCounts(items), ParseItemCounts(rewardItems)
		st.Factions, st.Spells = ParseFactionDeltas(factions), ParseIntList(spells)
		if st.Events, err = ParseQuestEvents(events); err != nil {
			log.Printf("Quests: quest %d step %d's events are not read: %v", st.Quest, st.Step, err)
		}
		if st.ByClass, err = ParseClassRewards(byClass); err != nil {
			log.Printf("Quests: quest %d step %d's rewards by class are not read: %v", st.Quest, st.Step, err)
		}
		if st.Choices, err = ParseStepChoices(choices); err != nil {
			log.Printf("Quests: quest %d step %d's choice is not read: %v", st.Quest, st.Step, err)
		} else if len(st.Choices.Options) > 0 && !AtNPC(st.Kind) {
			log.Printf("Quests: quest %d step %d is a %s step: its choice is never asked", st.Quest, st.Step, st.Kind)
		}
		if i, ok := byID[st.Quest]; ok {
			out[i].Steps = append(out[i].Steps, st)
		}
	}
	srs.Close()
	if err := srs.Err(); err != nil {
		return nil, err
	}
	// each step's other creatures (queststeptargets), in the order written
	trs, err := s.db.Query(`SELECT questid, step, target FROM queststeptargets ORDER BY questid, step, rowid`)
	if err != nil {
		return nil, err
	}
	for trs.Next() {
		var qid, n int
		var t string
		if err := trs.Scan(&qid, &n, &t); err != nil {
			trs.Close()
			return nil, err
		}
		i, ok := byID[qid]
		if !ok || strings.TrimSpace(t) == "" {
			continue
		}
		if st := out[i].Step(n); st != nil {
			st.Also = append(st.Also, strings.TrimSpace(t))
		}
	}
	trs.Close()
	if err := trs.Err(); err != nil {
		return nil, err
	}
	urs, err := s.db.Query(`SELECT questid, npc, dest FROM questunlocks ORDER BY questid, npc, dest`)
	if err != nil {
		return nil, err
	}
	defer urs.Close()
	for urs.Next() {
		var u QuestUnlock
		if err := urs.Scan(&u.Quest, &u.NPC, &u.Dest); err != nil {
			return nil, err
		}
		u.NPC, u.Dest = strings.TrimSpace(u.NPC), strings.TrimSpace(u.Dest)
		if i, ok := byID[u.Quest]; ok && u.NPC != "" && u.Dest != "" {
			out[i].Unlocks = append(out[i].Unlocks, u)
		}
	}
	return out, urs.Err()
}

// CharQuests is a character's quests, under way and finished.
func (s *QuestStore) CharQuests(serverID uint32) ([]CharQuest, error) {
	rs, err := s.db.Query(`SELECT serverid, questid, step, count, done, slot, updated, picks FROM charquests
		WHERE serverid = ? ORDER BY slot, questid`, serverID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []CharQuest
	for rs.Next() {
		var c CharQuest
		if err := rs.Scan(&c.ServerID, &c.QuestID, &c.Step, &c.Count, &c.Done, &c.Slot, &c.Updated, &c.Picks); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rs.Err()
}

// SaveCharQuest writes a character's progress in a quest.
func (s *QuestStore) SaveCharQuest(c *CharQuest) error {
	c.Updated = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(serverid, questid) DO UPDATE SET step = excluded.step, count = excluded.count,
			done = excluded.done, slot = excluded.slot, updated = excluded.updated, picks = excluded.picks`,
		c.ServerID, c.QuestID, c.Step, c.Count, c.Done, c.Slot, c.Updated, c.Picks)
	return err
}

// ParsePicks reads a charquests picks column: the option picked (from 0) by
// step. A part it cannot read is left out.
func ParsePicks(s string) map[int]int {
	out := map[int]int{}
	for _, f := range strings.Split(s, ",") {
		step, opt, ok := strings.Cut(strings.TrimSpace(f), ":")
		if !ok {
			continue
		}
		st, err1 := strconv.Atoi(strings.TrimSpace(step))
		o, err2 := strconv.Atoi(strings.TrimSpace(opt))
		if err1 != nil || err2 != nil || st < 1 || o < 0 {
			continue
		}
		out[st] = o
	}
	return out
}

// FormatPicks writes picks as ParsePicks reads them, by step.
func FormatPicks(picks map[int]int) string {
	steps := make([]int, 0, len(picks))
	for st := range picks {
		steps = append(steps, st)
	}
	sort.Ints(steps)
	parts := make([]string, len(steps))
	for i, st := range steps {
		parts[i] = fmt.Sprintf("%d:%d", st, picks[st])
	}
	return strings.Join(parts, ", ")
}

// WithPick is a picks column with the option picked at step set.
func WithPick(picks string, step, option int) string {
	m := ParsePicks(picks)
	m[step] = option
	return FormatPicks(m)
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

// ParseRequires reads a requires column: "1, 2, 3" (every quest; anything
// but a | between the numbers is a separator, as ParseIntList reads it), and
// "4|5|6" one group met by any of them. Each number outside a group is a
// group of its own.
func ParseRequires(s string) [][]int {
	var out [][]int
	for _, part := range strings.Split(s, ",") {
		if !strings.Contains(part, "|") {
			for _, n := range ParseIntList(part) {
				out = append(out, []int{n})
			}
			continue
		}
		var g []int
		for _, alt := range strings.Split(part, "|") {
			g = append(g, ParseIntList(alt)...)
		}
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// FormatRequires writes groups as ParseRequires reads them: "1, 4|5|6".
func FormatRequires(groups [][]int) string {
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		alts := make([]string, len(g))
		for i, n := range g {
			alts[i] = strconv.Itoa(n)
		}
		if len(alts) > 0 {
			parts = append(parts, strings.Join(alts, "|"))
		}
	}
	return strings.Join(parts, ", ")
}

// UnmetRequire is the first of the groups none of whose quests finished
// says is finished, and whether there is one: what keeps a quest that
// requires them from being taken.
func UnmetRequire(groups [][]int, finished func(id int) bool) ([]int, bool) {
	for _, g := range groups {
		met := false
		for _, id := range g {
			if finished(id) {
				met = true
				break
			}
		}
		if !met {
			return g, true
		}
	}
	return nil, false
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
