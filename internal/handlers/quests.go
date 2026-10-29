package handlers

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The quests a GM writes (database/quests.go; the GM panel's Quests page), run.
//
//   - Talking to an NPC (0x0004, HandleInteract) asks the quests first: a quest
//     whose step to do is this NPC's (talk to it, or hand it items and tunar)
//     is done, or the NPC asks for the step's answer, or says what it still
//     waits for; else an enabled quest this NPC gives that the player may take
//     is offered (its giver's words, a box a line, the last one asking with the
//     answers that take it or refuse it, 0x0034) or, with no answer to take
//     it, given at once. Only then the NPC's own business (a bind, a route, its
//     words).
//   - An answer comes back as 0x0035 with the box's id (questBoxMark), which no
//     other dialog uses.
//   - A kill a player is credited with (entity/questhooks.go), shared with its
//     group near the kill, counts on a kill step; a place is reached when a
//     player stands within a reach step's radius (looked at once a second).
//   - The journal is CLIENT's eight quest lines: the MemoryDump writes them with
//     the journal's token (VIClient+0x2BB0C), 0x007C adds one at the end, 0x007D
//     takes the one at an index, both setting the token, which the player's own
//     delete (0x007D) sends back. A quest's line is its step's journal words; a
//     step done takes the line and adds the next step's at the end, as Return
//     Home's quests did; the quest finished takes it away.
//   - A step done: the NPC's words, "You have finished a quest!" after the last
//     step, a line for each thing handed in and each item given, then the
//     rewards (experience as a kill's, tunar, items as a sale's 0x003B, faction
//     standings in EverQuest's words, spells as a scribe's 0x00B3). Those lines
//     are boxes after the NPC's, as Return Home's scripts wrote them.
//
// A quest that is not enabled is offered to no one; one a character has under
// way goes on (a GM tries a draft by giving it to their own character).
//
// Three things the later guides need (tools/quests/FORMAT.md), all ours in
// their words:
//   - a quest's faction requirements (database.FactionReq): it is offered,
//     marked on the map, and its steps at its giver are done, only while the
//     player's standing with each faction (World.Standing) is in range; at a
//     giver it will not deal with, a line says why;
//   - a step's rewards by class (database.ClassReward), given with its own;
//   - a quest's unlocks (database.QuestUnlock): a travel NPC's trip that only
//     a player who has finished it is offered (waysender.go, TripBarred).

// questJournalLines is CLIENT's room for quest lines (m_questTitle[8]).
const questJournalLines = 8

// Room CLIENT gives what a quest says: a 0x0046 shows 217 characters
// (ShowDialog expands it into 218 units, VIClientText::Expand keeping the last
// for the NUL: a 218th character is dropped, F3 2026-10-29, Captain Scilond's
// offer losing its full stop), a 0x0034's question 127 and an answer 63, a
// journal line 127.
const (
	questBoxChars    = 217
	questAskChars    = 127
	questAnswerChars = 63
	questLineChars   = 127
)

// questBoxMark marks a quest dialog's id (0x0034's u32, echoed by 0x0035);
// every other dialog the server opens has 0.
const questBoxMark = 0x51000000

// What the server says around a quest.
const (
	questReceived = "You have received a quest!"
	questFinished = "You have finished a quest!"
	questFull     = "Your quest journal is full."
	questNotYet   = "Come back when you have what I asked for."
	questAskOffer = "Will you help?"
	questAskStep  = "Yes?"
	// a step's choice asked with no question of its own (ours)
	questAskChoice = "Which do you choose?"
)

// questEngine is the running engine, for the GM panel and the MemoryDump.
var questEngine atomic.Pointer[QuestEngine]

// Quests is the running quest engine, or nil.
func Quests() *QuestEngine { return questEngine.Load() }

// QuestEngine runs the quests.
type QuestEngine struct {
	mu     sync.Mutex
	store  *database.QuestStore
	inv    *database.InventoryStore
	items  *database.ItemPatternStore
	spells *database.SpellbookStore
	gw     *entity.World
	sm     *session.Manager

	cat    questCatalog
	chars  map[uint32]*questChar
	boxSeq uint32

	kills chan entity.KillCredit
	stop  chan struct{}
	// wake is nudge's: the marks and trackers looked at now
	wake chan struct{}

	// drop puts a loot step's item on one character's view of a corpse (the
	// windows' AddQuestDrop, questloot.go); nil drops nothing
	drop func(npc entity.EntityID, died time.Time, serverID uint32, rec database.CharInventoryEntry)
	// roll is a loot step's draw in [0, 1) (tests fix it)
	roll func() float64
	// spawns are the NPCs steps' spawn events placed, by character
	// (questevents.go)
	spawns map[uint32][]questSpawn

	// playerOf and sessionOf find a session's player and a character's
	// in-world session (tests stand in for them)
	playerOf  func(*session.Session) *entity.Player
	sessionOf func(serverID uint32) *session.Session

	// the quest marks' (questmarks.go): each quest's words, made at Reload,
	// and where the quests' NPCs stand, read from the World now and then;
	// names is the steps' items' names by patternid, for the trackers
	// (questprogress.go), made at Reload too
	texts   map[int]*questText
	names   map[int]string
	spots   map[string][]entity.NPCSpot
	spotsAt time.Time
}

// questCatalog is the quests as last read.
type questCatalog struct {
	byID   map[int]*database.Quest
	givers map[string][]*database.Quest // by the giver's folded name, enabled ones, by id
	all    []*database.Quest
	// gates are the trips enabled quests unlock, by tripKey: the quests that
	// unlock each, by id (a draft's unlock bars no one: no one can finish it);
	// unlockers every quest that unlocks each, drafts too (one finished
	// opens it)
	gates     map[string][]*database.Quest
	unlockers map[string][]*database.Quest
	// inGame is the enabled quests' names, by id (what a mastery rank may
	// need finished: InGame); never written once made
	inGame map[int]string
}

// questChar is one character's quests.
type questChar struct {
	serverID uint32
	quests   map[int]*database.CharQuest
	token    uint32
	box      *questBox
	// shown is the journal as the client has it: the MemoryDump's lines, each
	// 0x007C and 0x007D since (redrawLocked)
	shown []string
	// name is its player's, for %p in what is said (Sandstorm's Nothard puts
	// it in an answer: "My name is <the player's name>.")
	name string
	// marksOn is whether its client draws the quest marks, marksSent the
	// last set it was sent (questmarks.go)
	marksOn   bool
	marksSent string
	// progressOn is whether its client draws the quest tracker,
	// progressSent the last one it was sent (questprogress.go)
	progressOn   bool
	progressSent string
	// windowOffered is the quests a shop NPC's window has offered this
	// session (AtWindow): offered once, the next request opens the window
	windowOffered map[int]bool
}

// say is a text with %p the player's name (left as it is while the name is
// not known: CLIENT's own expansion fills a box's).
func (c *questChar) say(s string) string {
	if c == nil || c.name == "" {
		return s
	}
	return strings.ReplaceAll(s, "%p", c.name)
}

// namedLocked is a character's quests, its player's name noted.
func (q *QuestEngine) namedLocked(serverID uint32, name string) *questChar {
	c := q.charLocked(serverID)
	if name != "" {
		c.name = name
	}
	return c
}

// questBox is the quest dialog a character has open.
type questBox struct {
	id      uint32
	choices []questChoice
}

// questChoice is one answer of a quest dialog.
type questChoice struct {
	kind   int
	quest  int
	step   int
	option int // a step's choice: the option picked
	label  string
}

const (
	choiceAccept = iota + 1
	choiceDecline
	choiceStep
	choiceNotYet  // a show or give step's answer while its items lack: the reminder
	choiceNothing // a dead end (Sandstorm's "Farewell")
	choicePick    // an option of a step's choice (database.StepChoices)
)

// NewQuestEngine reads the quests out of db. Start sets it running.
func NewQuestEngine(db *database.SQLiteDB, gw *entity.World, sm *session.Manager, inv *database.InventoryStore, spells *database.SpellbookStore) (*QuestEngine, error) {
	q := &QuestEngine{
		store:  database.NewQuestStore(db),
		inv:    inv,
		items:  database.NewItemPatternStore(db),
		spells: spells,
		gw:     gw,
		sm:     sm,
		chars:  map[uint32]*questChar{},
		kills:  make(chan entity.KillCredit, 256),
		wake:   make(chan struct{}, 1),
		stop:   make(chan struct{}),
		roll:   rand.Float64,
	}
	if gw != nil {
		q.playerOf = gw.SessionPlayer
		q.sessionOf = func(id uint32) *session.Session { return SessionOfCharacter(gw, sm, id) }
	}
	if err := q.Reload(); err != nil {
		return nil, err
	}
	return q, nil
}

// Start hears the World's kills, looks at reach steps once a second, and makes
// this the running engine; the start log says what it read.
func (q *QuestEngine) Start() {
	if q.gw != nil {
		q.gw.OnKill(q.heardKill)
	}
	go q.run()
	questEngine.Store(q)
	log.Print(q.Summary())
}

// Summary is the quests as read, in one line: how many, how many in the game,
// and how many use the later guides' fields.
func (q *QuestEngine) Summary() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	inGame, reqs, repeat, byClass, choices := 0, 0, 0, 0, 0
	for _, qu := range q.cat.all {
		if qu.Enabled {
			inGame++
		}
		if len(qu.FactionReqs) > 0 {
			reqs++
		}
		if qu.Repeatable {
			repeat++
		}
		for _, st := range qu.Steps {
			if len(st.ByClass) > 0 {
				byClass++
			}
			if len(st.Choices.Options) > 0 {
				choices++
			}
		}
	}
	return fmt.Sprintf("Quests: %d read, %d in the game; %d ask a standing, %d repeatable, %d trips unlocked (%d by quests in the game); "+
		"%d steps reward by class, %d ask a choice", len(q.cat.all), inGame, reqs, repeat, len(q.cat.unlockers), len(q.cat.gates), byClass, choices)
}

// Stop ends the worker.
func (q *QuestEngine) Stop() {
	select {
	case <-q.stop:
	default:
		close(q.stop)
	}
	questEngine.CompareAndSwap(q, nil)
}

// heardKill takes a kill from the World (its lock held: never wait here).
func (q *QuestEngine) heardKill(k entity.KillCredit) {
	select {
	case q.kills <- k:
	default:
		log.Printf("Quests: a kill of %q was not counted (the queue is full)", k.NPCName)
	}
}

func (q *QuestEngine) run() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-q.stop:
			return
		case k := <-q.kills:
			q.killed(k)
		case <-q.wake:
			q.syncMarks(time.Now())
			q.syncProgress()
		case <-tick.C:
			q.checkReach()
			q.expireSpawns()
			q.syncMarks(time.Now())
			q.syncProgress()
		}
	}
}

// Reload reads the quests again (a GM's change).
func (q *QuestEngine) Reload() error {
	all, err := q.store.LoadQuests()
	if err != nil {
		return err
	}
	cat := questCatalog{byID: map[int]*database.Quest{}, givers: map[string][]*database.Quest{}, gates: map[string][]*database.Quest{},
		unlockers: map[string][]*database.Quest{}, inGame: map[int]string{}}
	for i := range all {
		qu := &all[i]
		cat.byID[qu.ID] = qu
		cat.all = append(cat.all, qu)
		if qu.Enabled {
			cat.inGame[qu.ID] = qu.Name
		}
		if qu.Enabled && strings.TrimSpace(qu.Giver) != "" {
			k := foldName(qu.Giver)
			cat.givers[k] = append(cat.givers[k], qu)
		}
		for _, u := range qu.Unlocks {
			k := tripKey(u.NPC, u.Dest)
			cat.unlockers[k] = append(cat.unlockers[k], qu)
			if qu.Enabled {
				cat.gates[k] = append(cat.gates[k], qu)
			}
		}
	}
	texts, names := q.questTexts(cat.all)
	q.mu.Lock()
	q.cat = cat
	q.texts, q.names = texts, names
	q.spots = nil // the marks read the NPCs' places again
	q.mu.Unlock()
	q.nudge() // a GM's edit: the marks and trackers again
	return nil
}

// foldName is a name as names are matched: trimmed, any case.
func foldName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// charLocked is a character's quests, read on first use.
func (q *QuestEngine) charLocked(serverID uint32) *questChar {
	if c := q.chars[serverID]; c != nil {
		return c
	}
	c := &questChar{serverID: serverID, quests: map[int]*database.CharQuest{}, token: 1}
	rows, err := q.store.CharQuests(serverID)
	if err != nil {
		log.Printf("Quests: reading character %d's: %v", serverID, err)
	}
	for i := range rows {
		cq := rows[i]
		c.quests[cq.QuestID] = &cq
	}
	q.chars[serverID] = c
	return c
}

// active is the character's quests under way, in journal order.
func (c *questChar) active() []*database.CharQuest {
	var out []*database.CharQuest
	for _, cq := range c.quests {
		if cq.Active() {
			out = append(out, cq)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].QuestID < out[j].QuestID
	})
	return out
}

// index is a quest's place in the journal, or -1.
func (c *questChar) index(questID int) int {
	for i, cq := range c.active() {
		if cq.QuestID == questID {
			return i
		}
	}
	return -1
}

// nextSlot is the journal order a line added now takes.
func (c *questChar) nextSlot() int {
	n := 0
	for _, cq := range c.quests {
		if cq.Slot > n {
			n = cq.Slot
		}
	}
	return n + 1
}

// lineLocked is a quest's journal line: its step's words, else its name.
func (q *QuestEngine) lineLocked(cq *database.CharQuest) string {
	qu := q.cat.byID[cq.QuestID]
	if qu == nil {
		return fmt.Sprintf("Quest %d", cq.QuestID)
	}
	if st := qu.Step(cq.Step); st != nil && strings.TrimSpace(st.Journal) != "" {
		return clipRunes(strings.TrimSpace(st.Journal), questLineChars)
	}
	if strings.TrimSpace(qu.Name) != "" {
		return clipRunes(strings.TrimSpace(qu.Name), questLineChars)
	}
	return fmt.Sprintf("Quest %d", qu.ID)
}

// Journal is the MemoryDump's quest section for a character: the journal's
// token and its lines (at most eight), in order. A dump closes any quest box.
func (q *QuestEngine) Journal(serverID uint32) (uint32, []string) {
	if q == nil {
		return 0, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.charLocked(serverID)
	c.box = nil
	lines := q.linesLocked(c)
	c.shown = append([]string(nil), lines...)
	return c.token, lines
}

// InGame is the names of the quests in the game, by id: a mastery rank needs
// only those of its quests finished (cmcatalog.require_quests; a draft bars
// no one). The map is shared: never write it.
func (q *QuestEngine) InGame() map[int]string {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.cat.inGame
}

// FinishedBy is the quests a character has finished at least once, by id
// (what a mastery rank may need: cmcatalog.require_quests).
func (q *QuestEngine) FinishedBy(serverID uint32) map[int]bool {
	if q == nil {
		return map[int]bool{}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.charLocked(serverID).finished()
}

// finished is the quests the character has finished at least once, by id.
func (c *questChar) finished() map[int]bool {
	out := map[int]bool{}
	for id, cq := range c.quests {
		if cq.Done > 0 {
			out[id] = true
		}
	}
	return out
}

// linesLocked is the character's journal lines (at most eight), in order.
func (q *QuestEngine) linesLocked(c *questChar) []string {
	var lines []string
	for _, cq := range c.active() {
		if len(lines) == questJournalLines {
			break
		}
		lines = append(lines, c.say(q.lineLocked(cq)))
	}
	return lines
}

// questJournal is the MemoryDump's quest section for a player, empty with no
// engine running.
func questJournal(p *entity.Player) (uint32, []string) {
	if q := Quests(); q != nil && p != nil {
		q.mu.Lock()
		q.namedLocked(p.CharServerID, p.Name)
		q.mu.Unlock()
		return q.Journal(p.CharServerID)
	}
	return 0, nil
}

// dropLineLocked takes the journal's line at idx (0x007D).
func (q *QuestEngine) dropLineLocked(sess *session.Session, c *questChar, idx int) {
	if idx < 0 {
		return
	}
	c.token++
	if idx < len(c.shown) {
		c.shown = append(c.shown[:idx:idx], c.shown[idx+1:]...)
	}
	if sess != nil && idx < questJournalLines {
		SendQuestRemove(sess, c.token, int32(idx))
	}
}

// addLineLocked adds a line at the journal's end (0x007C).
func (q *QuestEngine) addLineLocked(sess *session.Session, c *questChar, text string) {
	text = c.say(text)
	c.token++
	if len(c.shown) < questJournalLines {
		c.shown = append(c.shown, text)
	}
	if sess != nil {
		SendQuestAdd(sess, c.token, text)
	}
}

// redrawLocked makes the client's journal the character's: every line taken
// and the lines added again in order, when they differ from what it shows.
func (q *QuestEngine) redrawLocked(sess *session.Session, c *questChar) bool {
	want := q.linesLocked(c)
	if sameLines(want, c.shown) {
		return false
	}
	for n := len(c.shown); n > 0; n-- {
		q.dropLineLocked(sess, c, 0)
	}
	for _, l := range want {
		q.addLineLocked(sess, c, l)
	}
	return true
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// saveLocked writes a character's progress in a quest.
func (q *QuestEngine) saveLocked(cq *database.CharQuest) {
	if err := q.store.SaveCharQuest(cq); err != nil {
		log.Printf("Quests: saving character %d's quest %d: %v", cq.ServerID, cq.QuestID, err)
	}
	q.nudge()
}

// nudge has the worker look at the marks and the trackers at once (a quest
// taken, counted, advanced, finished or given up), not at its next second.
func (q *QuestEngine) nudge() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// --- what a player does ---

// Talk is HandleInteract's first question: a quest step done at this NPC, or
// a quest it offers. It reports whether the quests answered.
func (q *QuestEngine) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	wontDeal := ""
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil || !database.AtNPC(st.Kind) || !stepNPC(st, npc.Name, npc.ServerID) {
			continue
		}
		// a giver the player's standing keeps from its steps: another
		// quest's step here first, else the line that says why
		if line := giverWontDeal(look, qu, st); line != "" {
			if wontDeal == "" {
				wontDeal = line
			}
			continue
		}
		q.atNPCLocked(sess, p, c, qu, cq, st, false, -1)
		return true
	}
	if wontDeal != "" {
		q.tellLocked(c, sess, "", wontDeal)
		log.Printf("Quests: %s: %s", p.Name, wontDeal)
		return true
	}
	for _, qu := range q.cat.givers[foldName(npc.Name)] {
		if q.mayTakeLocked(look, p, c, qu) == "" {
			q.offerLocked(sess, p, c, qu)
			return true
		}
	}
	return false
}

// stepNPC is whether a talk, give or kill step names this NPC.
func stepNPC(st *database.QuestStep, name string, spawn entity.EntityID) bool {
	if !strings.EqualFold(strings.TrimSpace(st.Target), strings.TrimSpace(name)) {
		return false
	}
	return st.NPCID == 0 || entity.EntityID(st.NPCID) == spawn
}

// factionLook is a player's standings as the World has them (World.Standing,
// which takes the World's lock), each faction asked once in one look at the
// quests: a talk, an answer, a set of marks.
type factionLook struct {
	gw   *entity.World
	sess *session.Session
	got  map[string]int
	none bool // the session has no player in the World: no standing is known
}

// lookAt is a look at the session's player's standings.
func (q *QuestEngine) lookAt(sess *session.Session) *factionLook {
	return &factionLook{gw: q.gw, sess: sess}
}

// standing is the player's standing with a faction, false when it is not
// known (no World, no player in it).
func (l *factionLook) standing(faction string) (int, bool) {
	if l == nil || l.gw == nil || l.sess == nil || l.none {
		return 0, false
	}
	if v, ok := l.got[faction]; ok {
		return v, true
	}
	v, ok := l.gw.Standing(l.sess, faction)
	if !ok {
		l.none = true
		return 0, false
	}
	if l.got == nil {
		l.got = map[string]int{}
	}
	l.got[faction] = v
	return v, true
}

// unmetFaction is the first of the quest's faction requirements the player
// does not meet, its standing, and whether there is one (a standing not
// known meets none).
func unmetFaction(look *factionLook, qu *database.Quest) (database.FactionReq, int, bool) {
	for _, r := range qu.FactionReqs {
		v, ok := look.standing(r.Faction)
		if !ok || !r.Met(v) {
			return r, v, true
		}
	}
	return database.FactionReq{}, 0, false
}

// giverWontDeal is the line said when a step at the quest's giver is not
// done for the player's standing with a faction (no recording has one: ours),
// or "" when the step may be done: it is not at the giver, or every
// requirement is met.
func giverWontDeal(look *factionLook, qu *database.Quest, st *database.QuestStep) string {
	if len(qu.FactionReqs) == 0 || !database.AtNPC(st.Kind) || foldName(st.Target) != foldName(qu.Giver) {
		return ""
	}
	r, v, unmet := unmetFaction(look, qu)
	if !unmet {
		return ""
	}
	how := "too low"
	if r.HasMax && v > r.Max {
		how = "too high"
	}
	return fmt.Sprintf("%s will not deal with you: your standing with %s is %s.", strings.TrimSpace(qu.Giver), r.Faction, how)
}

// mayTakeLocked is "" when the player may take the quest, else why not; its
// faction requirements are asked of look (the player's session's).
func (q *QuestEngine) mayTakeLocked(look *factionLook, p *entity.Player, c *questChar, qu *database.Quest) string {
	switch {
	case !qu.Enabled:
		return "it is a draft"
	case qu.First() == nil:
		return "it has no steps"
	case qu.MinLevel > 0 && p.Level < qu.MinLevel:
		return fmt.Sprintf("it starts at level %d", qu.MinLevel)
	case qu.MaxLevel > 0 && p.Level > qu.MaxLevel:
		return fmt.Sprintf("it ends at level %d", qu.MaxLevel)
	case len(qu.Classes) > 0 && !containsInt(qu.Classes, p.Class):
		return "not for that class"
	case len(qu.Races) > 0 && !containsInt(qu.Races, p.Race):
		return "not for that race"
	}
	for _, id := range qu.Requires {
		if cq := c.quests[id]; cq == nil || cq.Done == 0 {
			return fmt.Sprintf("quest %d is to be finished first", id)
		}
	}
	if cq := c.quests[qu.ID]; cq != nil {
		if cq.Active() {
			return "under way"
		}
		if cq.Done > 0 && !qu.Repeatable {
			return "finished"
		}
	}
	// last: the only test that asks the World
	if r, v, unmet := unmetFaction(look, qu); unmet {
		return fmt.Sprintf("its standing with %s is %d, not %s", r.Faction, v, database.FormatFactionReqs([]database.FactionReq{r}))
	}
	return ""
}

func containsInt(v []int, n int) bool {
	for _, x := range v {
		if x == n {
			return true
		}
	}
	return false
}

// offerLocked offers a quest: its giver's first question, or the quest at
// once when nothing need be answered (its words said first).
func (q *QuestEngine) offerLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest) {
	if len(sayLines(qu.Accept)) == 0 {
		q.tellLocked(c, sess, qu.Giver, sayLines(qu.Offer)...)
		q.startLocked(sess, p, c, qu, 0, false)
		return
	}
	q.askStageLocked(sess, c, qu, 0)
	log.Printf("Quests: %s offered %q", p.Name, qu.Name)
}

// askStageLocked asks the offer's question k, as Sandstorm's Commander Nothard
// asks two: the offer's paragraph k (its lines a box each, its last line the
// question; any paragraphs past the answers said before the last question),
// answered by Accept's line k, or Decline.
func (q *QuestEngine) askStageLocked(sess *session.Session, c *questChar, qu *database.Quest, k int) {
	answers := sayLines(qu.Accept)
	if k >= len(answers) {
		return
	}
	paras := paragraphs(qu.Offer)
	words := ""
	switch {
	case k == len(answers)-1 && k < len(paras):
		words = strings.Join(paras[k:], "\n")
	case k < len(paras):
		words = paras[k]
	}
	choices := []questChoice{{kind: choiceAccept, quest: qu.ID, step: k, label: answers[k]}}
	if d := strings.TrimSpace(qu.Decline); d != "" {
		choices = append(choices, questChoice{kind: choiceDecline, quest: qu.ID, label: d})
	}
	q.askLocked(sess, c, qu.Giver, words, questAskOffer, choices)
}

// paragraphs is a text's paragraphs: its lines between blank lines.
func paragraphs(text string) []string {
	var out []string
	var cur []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if strings.TrimSpace(l) == "" {
			if len(cur) > 0 {
				out = append(out, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, strings.TrimSpace(l))
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, "\n"))
	}
	return out
}

// startLocked puts a quest in the journal at step (0: its first). gm marks a
// GM's: the journal's room is checked, the player told the same. It returns
// why it could not, or "".
func (q *QuestEngine) startLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, step int, gm bool) string {
	st := qu.First()
	if step > 0 {
		st = qu.Step(step)
	}
	if st == nil {
		return "it has no such step"
	}
	if cq := c.quests[qu.ID]; cq != nil && cq.Active() {
		return "it is under way already"
	}
	if len(c.active()) >= questJournalLines {
		if sess != nil && !gm {
			q.tellLocked(c, sess, "", questFull)
		}
		return "the journal is full (eight quests under way)"
	}
	cq := c.quests[qu.ID]
	if cq == nil {
		cq = &database.CharQuest{ServerID: c.serverID, QuestID: qu.ID}
		c.quests[qu.ID] = cq
	}
	cq.Step, cq.Count, cq.Slot = st.Step, 0, c.nextSlot()
	q.saveLocked(cq)
	// Sandstorm's order: the journal's line, "You have received a quest!",
	// then the giver's words
	q.addLineLocked(sess, c, q.lineLocked(cq))
	q.tellLocked(c, sess, "", questReceived)
	q.tellLocked(c, sess, qu.Giver, sayLines(qu.Started)...)
	who := fmt.Sprintf("character %d", c.serverID)
	if p != nil {
		who = p.Name
	}
	log.Printf("Quests: %s took %q (step %d)", who, qu.Name, st.Step)
	// its events (questevents.go): the quest's on taking it, then its step's
	q.questEventsLocked(sess, p, c, qu, database.EventAccept)
	q.stepStartedLocked(sess, p, c, qu, cq, st)
	return ""
}

// atNPCLocked is a talk, show or give step's NPC talked to (answered: its
// answer picked; pick: the option of its choice picked, -1 none). Show and
// give wait for what is to be held; give takes it. A step with a choice asks
// it last, and is done with the option's rewards besides its own (an option
// that stays does not do it).
func (q *QuestEngine) atNPCLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, answered bool, pick int) {
	// the standing may have moved since the question was asked
	if line := giverWontDeal(q.lookAt(sess), qu, st); line != "" {
		q.tellLocked(c, sess, "", line)
		return
	}
	if st.Kind == database.StepGive || st.Kind == database.StepShow {
		if lack := q.lackingLocked(p, st); lack != "" {
			log.Printf("Quests: %s at %q step %d still lacks %s", p.Name, qu.Name, st.Step, lack)
			// Sandstorm's Norgam asks his question with only the not-yet
			// answer while the belts lack; the completing one comes once
			// they are held
			if notyet := sayLines(st.NotYet); len(notyet) > 0 && !answered && pick < 0 {
				var choices []questChoice
				for _, l := range notyet {
					choices = append(choices, questChoice{kind: choiceNotYet, quest: qu.ID, step: st.Step, label: l})
				}
				q.askLocked(sess, c, st.Target, st.Waiting, questAskStep, choices)
				return
			}
			q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, orText(st.Waiting, questNotYet)))...)
			return
		}
	}
	// the answer's first line does the step; any more are dead ends
	// (Sandstorm's Zole: "Commander Nothard sent me", "Farewell")
	answers := sayLines(st.Answer)
	if len(answers) > 0 && !answered && pick < 0 {
		choices := []questChoice{{kind: choiceStep, quest: qu.ID, step: st.Step, label: answers[0]}}
		for _, l := range answers[1:] {
			choices = append(choices, questChoice{kind: choiceNothing, label: l})
		}
		q.askLocked(sess, c, st.Target, st.Waiting, questAskStep, choices)
		return
	}
	var opt *database.StepOption
	if opts := st.Choices.Options; len(opts) > 0 {
		if pick < 0 || pick >= len(opts) {
			// the words before it unless an answer's question said them
			words := orText(st.Choices.Ask, questAskChoice)
			if len(answers) == 0 && strings.TrimSpace(st.Waiting) != "" {
				words = st.Waiting + "\n" + words
			}
			choices := make([]questChoice, len(opts))
			for i, o := range opts {
				choices[i] = questChoice{kind: choicePick, quest: qu.ID, step: st.Step, option: i, label: o.Answer}
			}
			q.askLocked(sess, c, st.Target, words, questAskChoice, choices)
			return
		}
		opt = &opts[pick]
		if opt.Stay {
			q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
			log.Printf("Quests: %s picked %q at %q step %d: the step stays", p.Name, opt.Answer, qu.Name, st.Step)
			return
		}
	}
	var given []string
	if st.Kind == database.StepGive {
		var ok bool
		if given, ok = q.takeLocked(sess, p, st); !ok {
			q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, questNotYet))...)
			return
		}
	}
	var extra *database.ClassReward
	if opt != nil {
		q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
		extra = &opt.Reward
		log.Printf("Quests: %s picked %q at %q step %d", p.Name, opt.Answer, qu.Name, st.Step)
	}
	q.completeStepLocked(sess, p, c, qu, cq, st, given, extra)
}

// Answer is HandleDialogueOption's first question: an answer to a quest's box.
// It reports whether the box was a quest's.
func (q *QuestEngine) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if q == nil || p == nil || box&0xFF000000 != questBoxMark {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	if c.box == nil || c.box.id != box {
		return true // an old quest box: nothing to do
	}
	if option == 255 {
		// a close is not the last word: the PS2 sends it just before the
		// real pick when two boxes were open (interact.go)
		return true
	}
	b := c.box
	c.box = nil
	if int(option) >= len(b.choices) {
		return true
	}
	ch := b.choices[option]
	if ch.kind == choiceNothing {
		return true // a dead end (Sandstorm's "Farewell"): nothing changes
	}
	qu := q.cat.byID[ch.quest]
	if qu == nil {
		return true
	}
	switch ch.kind {
	case choiceAccept:
		if why := q.mayTakeLocked(q.lookAt(sess), p, c, qu); why != "" {
			log.Printf("Quests: %s answered to take %q, but %s", p.Name, qu.Name, why)
			return true
		}
		if ch.step+1 < len(sayLines(qu.Accept)) {
			q.askStageLocked(sess, c, qu, ch.step+1)
			return true
		}
		q.startLocked(sess, p, c, qu, 0, false)
	case choiceDecline:
		// the giver's reply, said as written (Sandstorm's Norgam's has no
		// name before it)
		q.tellLocked(c, sess, "", sayLines(qu.Declined)...)
		log.Printf("Quests: %s refused %q", p.Name, qu.Name)
	case choiceNotYet:
		if st := qu.Step(ch.step); st != nil {
			q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, questNotYet))...)
		}
	case choiceStep, choicePick:
		cq := c.quests[qu.ID]
		st := qu.Step(ch.step)
		if cq == nil || cq.Step != ch.step || st == nil {
			return true
		}
		pick := -1
		if ch.kind == choicePick {
			pick = ch.option
		}
		q.atNPCLocked(sess, p, c, qu, cq, st, true, pick)
	}
	return true
}

// HandleDrop is the client's 0x007D (Send007D): the journal's token and the
// index of the quest the player deleted. The quest is given up (a finished
// count is kept) and its line taken, as 0x007D does on the client; a token not
// the journal's last (a line crossed it on the way) is ignored.
func (q *QuestEngine) HandleDrop(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	token, e1 := r.ReadUint32()
	idx, e2 := r.Read7BitEncodedInt64()
	if e1 != nil || e2 != nil || q.playerOf == nil {
		return nil
	}
	p := q.playerOf(sess)
	if p == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	act := c.active()
	if token != c.token {
		log.Printf("Quests: %s deleted journal line %d under token %d, the journal's is %d: ignored", p.Name, idx, token, c.token)
		return nil
	}
	if idx < 0 || int(idx) >= len(act) {
		return nil
	}
	cq := act[idx]
	q.giveUpLocked(cq)
	q.dropLineLocked(sess, c, int(idx))
	log.Printf("Quests: %s gave up quest %d", p.Name, cq.QuestID)
	return nil
}

// giveUpLocked ends a quest under way unfinished: forgotten when it was never
// finished, else left finished as many times as it was.
func (q *QuestEngine) giveUpLocked(cq *database.CharQuest) {
	q.clearSpawnsLocked(cq.ServerID, cq.QuestID, 0)
	cq.Step, cq.Count = 0, 0
	if cq.Done == 0 {
		if err := q.store.DeleteCharQuest(cq.ServerID, cq.QuestID); err != nil {
			log.Printf("Quests: forgetting character %d's quest %d: %v", cq.ServerID, cq.QuestID, err)
		}
		if c := q.chars[cq.ServerID]; c != nil {
			delete(c.quests, cq.QuestID)
		}
		return
	}
	q.saveLocked(cq)
}

// killed counts a kill on the kill steps of every player it was credited to.
func (q *QuestEngine) killed(k entity.KillCredit) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, kp := range k.Players {
		c := q.namedLocked(kp.ServerID, kp.Name)
		for _, cq := range c.active() {
			qu := q.cat.byID[cq.QuestID]
			if qu == nil {
				continue
			}
			st := qu.Step(cq.Step)
			if st != nil && st.Kind == database.StepLoot && stepNPC(st, k.NPCName, k.NPCSpawn) {
				q.lootKillLocked(k, kp, qu, cq, st) // questevents.go
				continue
			}
			if st == nil || st.Kind != database.StepKill || !stepNPC(st, k.NPCName, k.NPCSpawn) {
				continue
			}
			cq.Count++
			need := max(st.Count, 1)
			var p *entity.Player
			if kp.Session != nil && q.playerOf != nil {
				p = q.playerOf(kp.Session)
			}
			if cq.Count < need || p == nil {
				q.saveLocked(cq)
				if kp.Session != nil {
					SendClientMessage(kp.Session, fmt.Sprintf("%s: %d of %d.", k.NPCName, min(cq.Count, need), need))
				}
				continue
			}
			q.completeStepLocked(kp.Session, p, c, qu, cq, st, nil, nil)
		}
	}
}

// checkReach does the reach steps of the players standing in their places.
func (q *QuestEngine) checkReach() {
	if q.sessionOf == nil || q.playerOf == nil || q.gw == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		for _, cq := range c.active() {
			qu := q.cat.byID[cq.QuestID]
			if qu == nil {
				continue
			}
			st := qu.Step(cq.Step)
			if st == nil || st.Kind != database.StepReach {
				continue
			}
			sess := q.sessionOf(id)
			if sess == nil {
				break
			}
			p := q.playerOf(sess)
			world, x, _, z, ok := q.gw.PlayerPlace(p)
			if !ok || int(world) != st.World {
				continue
			}
			r := st.Radius
			if r <= 0 {
				r = 15
			}
			if dx, dz := x-st.X, z-st.Z; dx*dx+dz*dz <= r*r {
				q.completeStepLocked(sess, p, c, qu, cq, st, nil, nil)
			}
		}
	}
}

// completeStepLocked does a step in Sandstorm's order (its first warrior
// quest, recorded): the NPC's words, the journal's line taken (and the next
// step's added), "You have finished a quest!" after the last step, a line for
// what was handed in and each item given, then the experience and the
// faction lines. extra is the rewards of the option the player picked (nil
// none), given besides the step's.
func (q *QuestEngine) completeStepLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, given []string, extra *database.ClassReward) {
	idx := c.index(qu.ID)
	next := qu.Next(st.Step)
	speaker := ""
	if database.AtNPC(st.Kind) {
		speaker = st.Target
	}
	q.tellLocked(c, sess, speaker, sayLines(st.Says)...)
	who := fmt.Sprintf("character %d", c.serverID)
	if p != nil {
		who = p.Name
	}
	var lines []string
	if next == nil {
		cq.Step, cq.Count = 0, 0
		cq.Done++
		q.dropLineLocked(sess, c, idx)
		lines = append(lines, questFinished)
		log.Printf("Quests: %s finished %q", who, qu.Name)
	} else {
		old := q.lineLocked(cq)
		cq.Step, cq.Count = next.Step, 0
		if text := q.lineLocked(cq); text != old || idx < 0 {
			q.dropLineLocked(sess, c, idx)
			cq.Slot = c.nextSlot()
			q.addLineLocked(sess, c, text)
			// each step is its own journal line, both announced (Sandstorm's
			// Zole and Ronks, recorded)
			lines = append(lines, questFinished, questReceived)
		}
		log.Printf("Quests: %s did step %d of %q; step %d next", who, st.Step, qu.Name, next.Step)
	}
	q.saveLocked(cq)
	for _, g := range given {
		lines = append(lines, "You have given away "+g+".")
	}
	// the step's rewards and its rewards for the player's class
	var rw database.ClassReward
	if p != nil {
		rw = st.RewardFor(p.Class)
	}
	if extra != nil {
		rw = rw.Plus(*extra)
	}
	lines = append(lines, q.rewardLocked(sess, p, rw)...)
	if next == nil && sess != nil {
		// a mastery rank the quest opens (cmcatalog.require_quests: each
		// stat-boost trial its line's next rank), in the guide's words:
		// "Congratulations, Bulging Muscles 1 (5cm cost, 10str) is now
		// available for purchase."
		k := entity.CMKnows{Quests: c.finished(), QuestNames: q.cat.inGame}
		for _, name := range q.gw.CMOpenedBy(sess, qu.ID, k) {
			lines = append(lines, name+" is now available for purchase.")
		}
	}
	q.tellLocked(c, sess, "", lines...)
	if rw.XP > 0 && sess != nil && p != nil {
		q.gw.AwardXP(sess, p, rw.XP)
	}
	if len(st.Factions) > 0 && p != nil {
		hits := make([]entity.FactionHit, len(st.Factions))
		for i, f := range st.Factions {
			hits[i] = entity.FactionHit{Faction: f.Faction, Delta: f.Delta}
		}
		q.gw.ApplyFactionHits(sess, p, hits)
	}
	// the events (questevents.go): the step's spawns gone, then its own on its
	// doing (a spawn among them the next step's, to live until that one is
	// done: moradhim's rogue L10 mine goblins), then the quest's on its finish
	// or the next step's on its start
	q.clearSpawnsLocked(c.serverID, qu.ID, st.Step)
	spawnStep := st.Step
	if next != nil {
		spawnStep = next.Step
	}
	q.stepEventsLocked(sess, p, c, qu, st, database.EventDone, spawnStep)
	if next == nil {
		q.questEventsLocked(sess, p, c, qu, database.EventFinish)
		q.clearSpawnsLocked(c.serverID, qu.ID, 0)
		return
	}
	q.stepStartedLocked(sess, p, c, qu, cq, next)
}

// rewardLocked gives a step's tunar, items and spells for the player (its
// RewardFor the player's class; experience and factions are the World's,
// after the lines) and returns the lines that say so.
func (q *QuestEngine) rewardLocked(sess *session.Session, p *entity.Player, rw database.ClassReward) []string {
	if sess == nil || p == nil {
		return nil
	}
	var lines []string
	if rw.Tunar > 0 && q.inv != nil {
		p.Tunar += rw.Tunar
		if err := q.inv.SetPurse(p.CharServerID, p.Tunar); err != nil {
			log.Printf("Quests: %s's tunar: %v", p.Name, err)
		}
		queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(p.Tunar))
		lines = append(lines, fmt.Sprintf("You have received %d tunar.", rw.Tunar))
	}
	for _, it := range rw.Items {
		pat, err := q.items.GetItemPattern(it.PatternID)
		if err != nil || pat == nil {
			log.Printf("Quests: reward item %d: %v", it.PatternID, err)
			continue
		}
		n, err := GiveItemNow(sess, q.gw, q.inv, *pat, it.Count)
		if err != nil {
			log.Printf("Quests: giving %s %q: %v", p.Name, pat.ItemName, err)
		}
		if n > 0 {
			lines = append(lines, "You have received "+countOf(n, pat.ItemName)+".")
		}
		if n < it.Count {
			lines = append(lines, fmt.Sprintf("Your inventory is full: %s could not be given.", countOf(it.Count-n, pat.ItemName)))
		}
	}
	for _, id := range rw.Spells {
		if name, ok := q.teachLocked(sess, p, id); ok {
			lines = append(lines, "You have learned "+name+".")
		}
	}
	return lines
}

// teachLocked puts a spell in the player's book as a scribe does, unless it is
// there; it reports the spell's name and whether it was added.
func (q *QuestEngine) teachLocked(sess *session.Session, p *entity.Player, spellID int) (string, bool) {
	if q.spells == nil {
		return "", false
	}
	book, err := q.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		return "", false
	}
	for _, e := range book {
		if e.Pattern.SpellID == spellID {
			return "", false
		}
	}
	if err := q.spells.AddSpell(p.CharServerID, spellID, nextDisplayIndex(book)); err != nil {
		log.Printf("Quests: teaching %s spell %d: %v", p.Name, spellID, err)
		return "", false
	}
	book, err = q.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		return "", false
	}
	for i := range book {
		if book[i].Pattern.SpellID == spellID {
			SendAbilityAdd(sess, 0, true, &book[i])
			return book[i].Pattern.SpellName, true
		}
	}
	return "", false
}

// heldItems is the player's unworn inventory records, in the client's list
// order (bank and worn items are not handed in).
func (q *QuestEngine) heldItems(p *entity.Player) []database.CharInventoryEntry {
	if q.inv == nil {
		return nil
	}
	rows, err := q.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Quests: %s's inventory: %v", p.Name, err)
		return nil
	}
	var out []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 0 && r.EquipLoc < 0 && r.StackLeft > 0 {
			out = append(out, r)
		}
	}
	return out
}

// lackingLocked is what the player still lacks for a give step, or "".
func (q *QuestEngine) lackingLocked(p *entity.Player, st *database.QuestStep) string {
	have := map[int]int{}
	for _, r := range q.heldItems(p) {
		have[r.Pattern.PatternID] += r.StackLeft
	}
	var lack []string
	for _, it := range st.Items {
		if have[it.PatternID] < it.Count {
			lack = append(lack, fmt.Sprintf("%d of item %d", it.Count-have[it.PatternID], it.PatternID))
		}
	}
	if p.Tunar < st.Tunar {
		lack = append(lack, fmt.Sprintf("%d tunar", st.Tunar-p.Tunar))
	}
	return strings.Join(lack, ", ")
}

// takeLocked takes a give step's items and tunar from the player, in the
// database, the session's list and the client's (0x003C, 0x0052), and returns
// what was handed in, by name; false (and nothing taken) when something lacks.
func (q *QuestEngine) takeLocked(sess *session.Session, p *entity.Player, st *database.QuestStep) ([]string, bool) {
	if q.lackingLocked(p, st) != "" || q.inv == nil {
		return nil, false
	}
	held := q.heldItems(p)
	l := sessionInvList(sess)
	var given []string
	for _, it := range st.Items {
		left, name := it.Count, fmt.Sprintf("item %d", it.PatternID)
		for i := range held {
			r := &held[i]
			if left == 0 || r.Pattern.PatternID != it.PatternID || r.StackLeft == 0 {
				continue
			}
			name = r.Pattern.ItemName
			k := min(left, r.StackLeft)
			var err error
			if k >= r.StackLeft {
				err = q.inv.DeleteCharItem(p.CharServerID, r.ItemID)
			} else {
				err = q.inv.SetItemStack(p.CharServerID, r.ItemID, r.StackLeft-k)
			}
			if err != nil {
				log.Printf("Quests: taking %q from %s: %v", name, p.Name, err)
				continue
			}
			r.StackLeft -= k
			left -= k
			if l != nil {
				if idx := l.indexOfItem(r.ItemID); idx >= 0 && l.took(idx, k) && sess != nil {
					queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, k))
				}
			}
		}
		given = append(given, countOf(it.Count, name))
	}
	if st.Tunar > 0 {
		p.Tunar -= st.Tunar
		if err := q.inv.SetPurse(p.CharServerID, p.Tunar); err != nil {
			log.Printf("Quests: %s's tunar: %v", p.Name, err)
		}
		if sess != nil {
			queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(p.Tunar))
		}
		given = append(given, fmt.Sprintf("%d tunar", st.Tunar))
	}
	return given, true
}

// indexOfItem is the list index of the record of that inventory row, or -1.
func (l *invList) indexOfItem(itemID int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.recs {
		if l.recs[i].itemID == itemID {
			return i
		}
	}
	return -1
}

// countOf is "a thing" said with its count: the name alone for one, else "2
// names".
func countOf(n int, name string) string {
	if n == 1 {
		return name
	}
	return fmt.Sprintf("%d %s", n, name)
}

// --- the boxes ---

// tellLocked says each line in a box of its own (0x0046), the client showing
// them one after another; an NPC's (speaker not "") begin with its name, as
// Sandstorm's do ("Commander Nothard: ..."), unless the line names it already.
func (q *QuestEngine) tellLocked(c *questChar, sess *session.Session, speaker string, lines ...string) {
	if sess == nil {
		return
	}
	speaker = strings.TrimSpace(speaker)
	for _, l := range lines {
		l = c.say(l)
		if speaker != "" && !strings.HasPrefix(strings.ToLower(l), strings.ToLower(speaker)+":") {
			l = speaker + ": " + l
		}
		for _, part := range splitSay(l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
}

// askLocked says words, the speaker's (all lines but the last, which asks;
// def when there is no line short enough to ask; the question carries no
// name, as Sandstorm's do not) and opens the answers' box (0x0034).
func (q *QuestEngine) askLocked(sess *session.Session, c *questChar, speaker, words, def string, choices []questChoice) {
	lines := sayLines(words)
	ask := def
	if n := len(lines); n > 0 && utf8.RuneCountInString(lines[n-1]) <= questAskChars {
		ask, lines = lines[n-1], lines[:n-1]
	}
	q.tellLocked(c, sess, speaker, lines...)
	ask = c.say(ask)
	q.boxSeq++
	id := uint32(questBoxMark | (q.boxSeq & 0xFFFFFF))
	labels := make([]string, len(choices))
	for i, ch := range choices {
		labels[i] = clipRunes(c.say(strings.TrimSpace(ch.label)), questAnswerChars)
	}
	c.box = &questBox{id: id, choices: choices}
	if sess != nil {
		SendQuestPopupMulti(sess, id, ask, labels)
	}
}

// sayLines is a text's lines, empty ones left out.
func sayLines(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// orText is text, or def when it says nothing.
func orText(text, def string) string {
	if strings.TrimSpace(text) == "" {
		return def
	}
	return text
}

// splitSay cuts a line into parts of at most n characters, after a
// sentence's end where one falls in the part, else after a space.
func splitSay(s string, n int) []string {
	r := []rune(strings.TrimSpace(s))
	var out []string
	for len(r) > n {
		cut := -1
		for i := n - 1; i > n/3; i-- {
			if (r[i] == '.' || r[i] == '!' || r[i] == '?') && i+1 < len(r) && r[i+1] == ' ' {
				cut = i + 1
				break
			}
		}
		if cut < 0 {
			for i := n - 1; i > 0; i-- {
				if r[i] == ' ' {
					cut = i
					break
				}
			}
		}
		if cut <= 0 {
			cut = n
		}
		out = append(out, strings.TrimSpace(string(r[:cut])))
		r = []rune(strings.TrimSpace(string(r[cut:])))
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}

// clipRunes is s cut to n characters.
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
