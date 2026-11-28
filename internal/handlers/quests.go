package handlers

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
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
//   - A step done: the NPC's words, "You finished a quest!" after the last
//     step, a line for each thing handed in and each item given, then the
//     rewards (experience as a kill's, tunar, items as a sale's 0x003B, faction
//     standings in the reference server's chat lines, spells as a scribe's 0x00B3). Those lines
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
	questReceived = "You received a quest!" // the live game's (board 967; "You have ..." the reference server's)
	questFinished = "You finished a quest!"
	questFull     = "Your quest journal is full."
	questNotYet   = "Come back when you have what I asked for."
	questAskOffer = "Will you help?"
	questAskStep  = "Yes?"
	// a step's choice asked with no question of its own (ours)
	questAskChoice = "Which do you choose?"
	// questChoiceCost: a choice that costs more tunar than the player holds
	questChoiceCost = "You do not have the %d tunar."
)

// questEngine is the running engine, for the GM panel and the MemoryDump.
var questEngine atomic.Pointer[QuestEngine]

// Quests is the running quest engine, or nil.
func Quests() *QuestEngine { return questEngine.Load() }

// QuestEngine runs the quests.
type QuestEngine struct {
	mu sync.Mutex
	// pass holds one player's pack and the Ascended table for a marks pass
	// (questpass.go), nil outside one
	pass *questPass
	// syncAsc is the Ascended records a sync of the marks reads once for
	// every character's pass (questpass.go)
	syncAsc *syncAscended
	db      *database.SQLiteDB
	store   *database.QuestStore
	inv     *database.InventoryStore
	items   *database.ItemPatternStore
	spells  *database.SpellbookStore
	gw      *entity.World
	sm      *session.Manager

	cat          questCatalog
	chars        map[uint32]*questChar
	froglokLines map[[2]int]froglokQuestLine // [quest ID, step]; step zero is the offer
	// prefarm is /prefarm's verdict on the kill being counted (prefarm.go), nil
	// outside one
	prefarm *prefarmKill
	boxSeq  uint32
	// owed are the steps whose spawns a keep restart took from a character,
	// [quest, step] by character, made again by respawnLost wherever it stands
	// (keep.go's QuestSpawns)
	owed map[uint32]map[[2]int]bool
	// afterDeath are the events a quest's price waits to run until its player's
	// respawn, by character (questdeath.go)
	afterDeath map[uint32]priceRest

	kills chan entity.KillCredit
	// queued is the kills in kills not yet taken to be worked, by NPC and death
	// (queuedMu: taken under the World's lock, so never q.mu), and workMu is held
	// across a kill's taking and its working (workKill): a corpse opened while its
	// kill waits in the queue has the kill worked first, its loot steps' drops on
	// it before its window opens (WorkKill; Heirloom petition 2980)
	queuedMu sync.Mutex
	queued   map[killKey]entity.KillCredit
	workMu   sync.Mutex
	// infections are the weres' bites that infect (wereinfect.go), cures
	// the cures the campfire's advisors offered, by character
	infections chan entity.InfectionHit
	cures      map[uint32]advisorBox
	// later is the infections to grant again (their keeping failed, or the
	// queue was full), wereq the catalog's were quests (wereinfect.go)
	later laterInfections
	wereq wereQuests
	stop  chan struct{}
	// wake is nudge's: the marks and trackers looked at now
	wake chan struct{}

	// drop puts a loot step's item on a corpse for whoever may loot it (the windows'
	// AddCorpseQuestDrop, questloot.go; questpreloot.go): ahead, no player of the kill on
	// the step, it keeps the corpse only a while; nil drops nothing
	drop func(npc entity.EntityID, died time.Time, rec database.CharInventoryEntry, ahead bool)
	// dropFor puts a loot step's item on one character's own view of a corpse (the windows'
	// AddQuestDrop): a player of the kill on the step's copy (questpreloot.go); nil, drop's
	dropFor func(npc entity.EntityID, died time.Time, serverID uint32, rec database.CharInventoryEntry)
	// pending and placed bracket a kill's time in the queue (the windows'
	// QuestDropsPending and QuestDropsPlaced, questloot.go): its corpse is not
	// taken away emptied before its loot steps' drops are on it (Heirloom
	// petition 2833: a quest ring placed two seconds after its corpse was emptied
	// and gone). pending runs under the World's lock; nil, neither
	pending, placed func(npc entity.EntityID, died time.Time)
	// questItems is every item a quest step names (questItemsOf), for the loot's sharing
	// (QuestItem): read with no lock, so a corpse's roll never waits on the quests
	questItems atomic.Pointer[map[int]bool]
	// roll is a loot step's draw in [0, 1) (tests fix it)
	roll func() float64
	// spawns are the NPCs steps' spawn events placed, by character
	// (questevents.go)
	spawns map[uint32][]questSpawn
	// notYetLines is table questnotyet (database.QuestNotYet): what a giver
	// says to a player too low for its next quest (quests_notyet.go); nil
	// says questNotStrong
	notYetLines func() ([]database.QuestNotYetLine, error)

	// binds keeps the bind points a teleport event's bind sets
	// (questevents.go); nil keeps none (tests)
	binds bindStore
	// places finds a teleport event's mainland spot (questevents.go)
	places interface {
		MainlandPlace(c *database.CharacterRecord) bool
	}

	// takeOff takes a worn item off before a give step takes it (the
	// EquipHandler's TakeOff; nil: takeOffLocked's own)
	takeOff func(*session.Session, *entity.Player, *database.CharInventoryEntry) bool

	// playerOf and sessionOf find a session's player and a character's
	// in-world session (tests stand in for them)
	playerOf  func(*session.Session) *entity.Player
	sessionOf func(serverID uint32) *session.Session
	// mates is a player's group's members (a raid's too): World.GroupMembers
	// unless a test's own (a group spawn's one set for a group, questevents.go)
	mates func(p *entity.Player) []*entity.Player

	// the quest marks' (questmarks.go): each quest's words, made at Reload,
	// and where the quests' NPCs stand, read from the World now and then;
	// names is the steps' items' names by patternid, for the trackers
	// (questprogress.go), made at Reload too
	texts map[int]*questText
	// dropSrc is the creature names whose loot tables drop each item a give step
	// names (questmarksdrops.go), read with the quests
	dropSrc map[int][]string
	names   map[int]string
	// saysPlaces is the places quests' last words give their next givers
	// (questpointers.go), as worked out, forgotten at Reload
	saysPlaces map[string]string
	spots      map[string][]entity.NPCSpot
	spotsAt    time.Time
	// camps are the hunt steps' creatures' camps and hops the travel NPCs'
	// trips (questmarkspots.go), made from spots and forgotten with them
	camps map[string][]huntCamp
	hops  []wayHop
	// coachSigned reads a character's signed ledgers (charcoaches), signed
	// keeps them a while for the way marks (questcoachway.go); nil: every
	// coach stop counts
	coachSigned func(serverID uint32) (map[string]bool, error)
	signed      map[uint32]signedLedgers

	// marked is table questmarks, the quests flagged for the marks over
	// their NPCs' heads (questheadmarks.go), read at Reload; gen counts the
	// quests' changes (nudge), which those marks are worked out again on
	marked map[int]bool
	gen    atomic.Uint64

	// reservedRuns are the trigger runs a hand-in reserved before it took
	// anything, by character and the trigger's folded name, for its done
	// event to start (questatomic.go); handInFault fails a hand-in's
	// transaction at a write (tests)
	reservedRuns map[uint32]map[string]*TriggerReservation
	handInFault  func(i int) error

	// the guarded definition edits (questdefgate.go): defMu orders every
	// publication of the catalog (Reload, a guarded edit), held the quests
	// it holds back from play, pubGen counts publications
	defMu  sync.Mutex
	held   map[int]bool
	pubGen atomic.Uint64
	// unpublished are the quests of an edit committed but not yet published
	// (held until a reload succeeds)
	unpublished map[int]bool
	// loadHook runs as a publication begins (tests: a slow load)
	loadHook func()

	// noTakeCache and noPackCache make every marks pass ask every quest and
	// read the pack, keeping nothing from the pass before (questtakecache.go;
	// tests: the reference a kept pass is held to)
	noTakeCache, noPackCache bool
	// cacheIgnore switches invalidations off (tests: the controls)
	cacheIgnore uint8
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
	// successors is the enabled quests with a giver that require each quest,
	// by its id (questpointers.go)
	successors map[int][]*database.Quest
	// homeClassLevels is, by racedefs race, the levels of the city class
	// quests its homeland's own quests stand for (their classquest events):
	// questpointers.go points none of those races to a city's quest of them
	homeClassLevels map[int]map[int]bool
	// encounters are the quests whose kills wait on a trigger's spawns
	// (questencounter.go)
	encounters []encounterLink
	// folded is each quest's name folded (foldName), by id, and giverOrder
	// the givers' keys sorted: what the marks ask of every quest for every
	// player each second (questmarks.go), made once
	folded     map[int]string
	giverOrder []string
	// places is the enabled quests offered at a place (database.Quest.OfferAt;
	// questplaceoffer.go), in id order
	places []*database.Quest
	// nears is the quests with a near event on a step, by id (questnear.go)
	nears map[int]bool
}

// foldedName is the quest's name folded, as foldName.
func (cat *questCatalog) foldedName(qu *database.Quest) string {
	if k, ok := cat.folded[qu.ID]; ok && cat.byID[qu.ID] == qu {
		return k
	}
	return foldName(qu.Name)
}

// questChar is one character's quests.
type questChar struct {
	westGate westGateSample
	serverID uint32
	// infection is its were infection's row (nil none), read once when
	// infectionRead (wereinfect.go)
	infection     *database.Infection
	infectionRead bool
	// qstate is what the World was last told of its walls and items
	// (questwalls.go)
	qstate questState
	quests map[int]*database.CharQuest
	token  uint32
	box    *questBox
	// shown is the journal as the client has it: the MemoryDump's lines, each
	// 0x007C and 0x007D since (redrawLocked)
	shown []string
	// name is its player's, for %p in what is said (the reference server's Nothard puts
	// it in an answer: "My name is <the player's name>.")
	name string
	// marksOn is whether its client draws the quest marks, marksSent the
	// last set it was sent (questmarks.go)
	marksOn   bool
	marksSent string
	// takes is the quests kept to ask mayTakeLocked of, pack its pack's
	// last read, kept from one marks pass to the next (questtakecache.go)
	takes *takeCands
	pack  *packCache
	// progressOn is whether its client draws the quest tracker,
	// progressSent the last one it was sent (questprogress.go)
	progressOn   bool
	progressSent string
	// windowOffered is the quests a shop NPC's window has offered this
	// session (AtWindow): offered once, the next request opens the window;
	// forgotten at a login (LoggedIn) and when the quest is given up
	windowOffered map[int]bool
	// windowNotYet is the quests a shop NPC's window has said "not yet" for
	// this session (quests_notyet.go), as windowOffered
	windowNotYet map[int]bool
	// laterSaid is when a kill of a later step's creature of each quest was
	// last said to count later (laterStepKillLocked): once a laterSayGap
	laterSaid map[int]time.Time
	// notYetLast is the questnotyet line said to it last (its id), not said
	// twice running when its city has another
	notYetLast int
	// packTold is when a kill, loot or reach step waiting for room in the
	// pack last said so, by quest and step (questroom.go)
	packTold map[[2]int]time.Time
	// declined is the quests it said no to since its login, not offered
	// again until its giver has none other to offer (offerAtLocked)
	declined map[int]bool
	// bulking is whether a "Hand in all" is handing in its sets
	// (questbulk.go): none of its hand-ins asks it again
	bulking bool
	// pointerIDs is the quests of the pointer lines at the end of shown, in
	// order (questpointers.go); dismissed the pointers the player deleted
	// (kept in charquestpointers over logins)
	pointerIDs []int
	dismissed  map[int]bool
	// headKey is what the marks over the NPCs' heads were last worked out
	// from, headAt when, headSent the set given the World
	// (questheadmarks.go)
	headKey  string
	headAt   time.Time
	headSent string
	// stranded is when it was first seen on an encounter's kill with no run of
	// the encounter, by link (questencounter.go)
	stranded map[encounterKey]time.Time
	// busyTold is when a reach step whose doing fires a trigger of a lock held
	// last said so, by quest (checkReach): once a laterSayGap
	busyTold map[int]time.Time
	// trials is its trial states by quest, read once (questtrials.go: a
	// clock, a standing owed); clockTold the minutes left a clock last said
	trials    map[int]*database.QuestTrial
	clockTold map[int]int
	// atPlace is the quests whose place (questplaceoffer.go) it stands at and
	// was offered at since it came in; placeBox the box of its last such
	// offer, placeQuest that offer's quest
	atPlace    map[int]bool
	placeBox   uint32
	placeQuest int
	// nearAt is the near places of its steps (questnear.go) it stands in
	// since it came in: told their lock held, or their events run
	nearAt map[nearKey]int
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
	// an ask of the offer (choiceAsk): its path ("0", "0.1") and the paths
	// picked before it in this conversation
	ask    string
	picked []string
}

const (
	choiceAccept = iota + 1
	choiceDecline
	choiceStep
	choiceNotYet    // a show or give step's answer while its items lack: the reminder
	choiceNothing   // a dead end (the reference server's "Farewell")
	choicePick      // an option of a step's choice (database.StepChoices)
	choiceAsk       // a question asked at the offer (database.QuestAsk)
	choiceBoard     // a quest picked off a giver's board (questboard.go)
	choiceBoardMore // the board's next page (option: the page)
	choiceBulk      // "Hand in all": a repeatable turn-in's other sets (option: how many; questbulk.go)
)

// NewQuestEngine reads the quests out of db. Start sets it running.
func NewQuestEngine(db *database.SQLiteDB, gw *entity.World, sm *session.Manager, inv *database.InventoryStore, spells *database.SpellbookStore) (*QuestEngine, error) {
	q := &QuestEngine{
		db:         db,
		store:      database.NewQuestStore(db),
		inv:        inv,
		items:      database.NewItemPatternStore(db),
		spells:     spells,
		gw:         gw,
		sm:         sm,
		chars:      map[uint32]*questChar{},
		kills:      make(chan entity.KillCredit, 256),
		queued:     map[killKey]entity.KillCredit{},
		infections: make(chan entity.InfectionHit, 64),
		cures:      map[uint32]advisorBox{},
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
		roll:       rand.Float64,
	}
	if gw != nil {
		q.playerOf = gw.SessionPlayer
		q.sessionOf = func(id uint32) *session.Session { return SessionOfCharacter(gw, sm, id) }
	}
	if db != nil {
		q.notYetLines = db.QuestNotYet
		store := database.NewSQLiteCharacterStore(db)
		q.binds, q.places = store, store
		q.coachSigned = store.CoachTowns
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
		q.gw.OnInfection(q.heardInfection) // wereinfect.go
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
	key := killKey{k.NPCID, k.Died.UnixNano()}
	q.queuedMu.Lock()
	if q.queued == nil {
		q.queued = map[killKey]entity.KillCredit{}
	}
	q.queued[key] = k // before the queue: run may take it at once
	q.queuedMu.Unlock()
	select {
	case q.kills <- k:
		if q.pending != nil {
			q.pending(k.NPCID, k.Died) // its corpse waits for its drops (placed, in workKill)
		}
	default:
		q.queuedMu.Lock()
		delete(q.queued, key)
		q.queuedMu.Unlock()
		log.Printf("Quests: a kill of %q was not counted (the queue is full)", k.NPCName)
	}
}

// killKey is a kill in the queue: its NPC and its death.
type killKey struct {
	npc  entity.EntityID
	died int64 // UnixNano
}

// workKill works the queued kill of NPC npc's death died, once, whoever comes
// first, the queue (run) or a corpse's opening (WorkKill): its kill steps
// counted, its loot steps' items on its corpse (killed), a raid's flag
// (killFlags), and the corpse told (placed). With no quest lock held. It
// reports whether it worked it (false: worked already, or never queued).
func (q *QuestEngine) workKill(npc entity.EntityID, died time.Time) bool {
	q.workMu.Lock()
	defer q.workMu.Unlock()
	key := killKey{npc, died.UnixNano()}
	q.queuedMu.Lock()
	k, ok := q.queued[key]
	delete(q.queued, key)
	q.queuedMu.Unlock()
	if !ok {
		return false
	}
	q.killed(k)
	q.killFlags(k) // raidportal.go: a raid's flag at a kill
	if q.placed != nil {
		q.placed(k.NPCID, k.Died) // with no quest lock held: the loot's, then the World's
	}
	return true
}

// WorkKill is the loot windows' hook as a corpse opens whose kill waits in the
// queue (questloot.go's QuestDropsPending): the kill worked now, ahead of the
// queue, so its loot steps' drops lie on the corpse before its window opens,
// never a moment after. Heirloom petition 2980 (2026-11-25, Matonxwiz at the
// fungals of Guk): with the queue behind, the Flavored Mushroom came onto a
// corpse one to ten seconds after its window opened, and the window was sent
// again then, opening on a player who had looted and walked on. Called with no
// lock held but the opener's item lists' (the handler's: the quests take none).
// When the queue is working the kill already, it waits for that to end.
func (q *QuestEngine) WorkKill(npc entity.EntityID, died time.Time) {
	q.workKill(npc, died)
}

func (q *QuestEngine) run() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-q.stop:
			return
		case k := <-q.kills:
			q.workKill(k.NPCID, k.Died) // unless a corpse's opening worked it already (WorkKill)
		case h := <-q.infections:
			q.infect(h) // wereinfect.go
		case <-q.wake:
			q.syncMarks(time.Now())
			q.syncProgress()
			q.syncPointers()            // questpointers.go
			q.syncHeadMarks(time.Now()) // questheadmarks.go
		case <-tick.C:
			q.checkReach()
			q.checkPlaceOffers() // questplaceoffer.go: a quest offered at a place
			q.checkNear()        // questnear.go: a step's events at a place (petition 3332)
			q.checkSurvive()     // questsurvive.go: the Trial of Endurance's kind of step
			q.checkWaiting()     // questroom.go: steps that waited for room
			q.expireSpawns()
			q.respawnLost()             // questevents.go: a spawn-only creature's lost spawns
			q.checkStranded(time.Now()) // questencounter.go: a kill whose encounter ended
			q.checkTrials(time.Now())   // questtrials.go: a trial's clocks and wards
			q.syncMarks(time.Now())
			q.syncProgress()
			q.syncPointers()
			q.syncQuestState() // questwalls.go: the walls passed, the items held
			q.syncHeadMarks(time.Now())
			q.retryInfections()           // wereinfect.go: grants whose keeping failed
			q.sweepInfections(time.Now()) // wereinfect.go: infections at their end
		}
	}
}

// Reload reads the quests again (a GM's change). Publications are one at a
// time (defMu, questdefgate.go): a load begun before another's can never
// publish over it.
func (q *QuestEngine) Reload() error {
	q.defMu.Lock()
	defer q.defMu.Unlock()
	return q.publishLocked()
}

// publishLocked is Reload with defMu held: the quests read and the catalog
// swapped in, its generation counted; quests an earlier edit left
// unpublished are let go.
func (q *QuestEngine) publishLocked() error {
	if q.loadHook != nil {
		q.loadHook()
	}
	all, err := q.store.LoadQuests()
	if err != nil {
		return err
	}
	cat := questCatalog{byID: map[int]*database.Quest{}, givers: map[string][]*database.Quest{}, gates: map[string][]*database.Quest{},
		unlockers: map[string][]*database.Quest{}, inGame: map[int]string{}, folded: map[int]string{}}
	turkey := map[int]bool{}
	for i := range all {
		qu := &all[i]
		if qu.Source == thanksgivingSource {
			turkey[qu.ID] = true // stepKill's own rules for the event's quests (thanksgiving.go)
		}
		cat.byID[qu.ID] = qu
		cat.folded[qu.ID] = foldName(qu.Name)
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
	thanksgivingQuests.Store(&turkey)
	for g := range cat.givers {
		cat.giverOrder = append(cat.giverOrder, g)
	}
	sort.Strings(cat.giverOrder)
	cat.successors = successorsOf(cat.all)
	cat.homeClassLevels = homeClassLevelsOf(cat.all)
	cat.encounters = encounterLinks(cat.all)
	cat.places = placeQuestsOf(cat.all) // questplaceoffer.go
	cat.nears = nearQuestsOf(cat.all)   // questnear.go
	texts, names := q.questTexts(cat.all)
	marked, err := q.store.MarkedQuests()
	if err != nil {
		return err
	}
	items := questItemsOf(cat.all)
	q.questItems.Store(&items)
	q.mu.Lock()
	q.cat = cat
	q.wereq = wereQuestsOf(cat)                            // wereinfect.go
	q.marked = markedWithFollowUps(marked, cat.successors) // questheadmarks.go
	q.texts, q.names = texts, names
	q.dropSrc = dropSources(q.store, cat.all)
	q.saysPlaces = nil
	q.spots, q.camps, q.hops = nil, nil, nil // the marks read the NPCs' places again
	for id := range q.unpublished {
		delete(q.held, id)
	}
	q.unpublished = nil
	q.pubGen.Add(1)
	q.mu.Unlock()
	q.nudge() // a GM's edit: the marks and trackers again
	return nil
}

// questItemsOf is every item a quest's loot, give or show step names: its quest items
// (a reward alone is not one).
func questItemsOf(all []*database.Quest) map[int]bool {
	out := map[int]bool{}
	for _, qu := range all {
		for i := range qu.Steps {
			st := &qu.Steps[i]
			switch st.Kind {
			case database.StepLoot, database.StepGive, database.StepShow:
				for _, it := range st.Items {
					out[it.PatternID] = true
				}
			}
		}
	}
	return out
}

// QuestItem is whether an item is one a quest step names (questItemsOf). It takes no
// lock: a corpse's roll (loot.go) asks it with the loot's lock held.
func (q *QuestEngine) QuestItem(pattern int) bool {
	if q == nil {
		return false
	}
	m := q.questItems.Load()
	return m != nil && (*m)[pattern]
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
	// the pointers it deleted, kept over logins (questpointers.go)
	if dropped, err := q.store.DroppedPointers(serverID); err != nil {
		log.Printf("Quests: reading character %d's deleted pointers: %v", serverID, err)
	} else if len(dropped) > 0 {
		c.dismissed = dropped
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
	// steps done in any order: who is left (questanyorder.go)
	if line := anyOrderLine(qu, cq); line != "" {
		return clipRunes(line, questLineChars)
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
// engine running: its quests' lines, then its pointers' (questpointers.go).
func questJournal(sess *session.Session, p *entity.Player) (uint32, []string) {
	if q := Quests(); q != nil && p != nil {
		return q.JournalFor(sess, p)
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
		// a pointer's line (questpointers.go) goes with its quest's id
		if first := len(c.shown) - len(c.pointerIDs); len(c.pointerIDs) > 0 && idx >= first {
			c.pointerIDs = append(c.pointerIDs[:idx-first:idx-first], c.pointerIDs[idx-first+1:]...)
		}
		c.shown = append(c.shown[:idx:idx], c.shown[idx+1:]...)
	}
	if sess != nil && idx < questJournalLines {
		SendQuestRemove(sess, c.token, int32(idx))
	}
}

// addLineLocked adds a quest's line after the journal's other quests' (0x007C):
// the pointers' lines (questpointers.go) taken first, syncPointers adding
// them back after it.
func (q *QuestEngine) addLineLocked(sess *session.Session, c *questChar, text string) {
	if len(c.pointerIDs) > 0 {
		q.stripPointersLocked(sess, c)
		q.nudge()
	}
	q.appendLineLocked(sess, c, text)
}

// appendLineLocked adds a line at the journal's end (0x007C).
func (q *QuestEngine) appendLineLocked(sess *session.Session, c *questChar, text string) {
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
//
// The pointers' lines shown (questpointers.go) are kept after the quests', as
// many as there is room for; syncPointers works them out again.
func (q *QuestEngine) redrawLocked(sess *session.Session, c *questChar) bool {
	want := q.linesLocked(c)
	if len(c.pointerIDs) > len(c.shown) {
		c.pointerIDs = nil
	}
	ptrs := append([]string(nil), c.shown[len(c.shown)-len(c.pointerIDs):]...)
	ids := c.pointerIDs
	if n := min(len(ptrs), questJournalLines-len(want)); n < len(ptrs) {
		ptrs, ids = ptrs[:max(n, 0)], ids[:max(n, 0)]
	}
	want = append(want, ptrs...)
	ids = append([]int(nil), ids...)
	if sameLines(want, c.shown) {
		return false
	}
	for n := len(c.shown); n > 0; n-- {
		q.dropLineLocked(sess, c, 0)
	}
	for _, l := range want {
		q.appendLineLocked(sess, c, l)
	}
	c.pointerIDs = ids
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
	q.gen.Add(1)
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// --- what a player does ---

// Talk is HandleInteract's first question: a quest step done at this NPC, or
// a quest it offers. It reports whether the quests answered.
func (q *QuestEngine) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc != nil && npc.Temporary && npc.Row() == 197601 && npc.Name == "Turkey King" {
		return true
	}
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	// what the player can do here now comes first: a step it holds the items
	// for, whichever quest's it is, then a hand-in it holds the items for
	// (several quests at one giver: the Plane of Sky's armourers, a quest a
	// piece, are asked for whichever piece the player has the parts of)
	done := q.doneCount(c)
	if q.stepAtLocked(sess, p, c, look, npc.Name, npc.ServerID, true) {
		q.offerAfterFinishLocked(sess, p, c, npc.Name, done)
		return true
	}
	if qu := q.readyHandInLocked(look, p, c, npc.Name); qu != nil {
		q.offerLocked(sess, p, c, qu)
		return true
	}
	// a refusal of a hand-in the player holds but may not do yet is said only
	// when the giver has nothing else for it: a step of a quest under way or a
	// quest to offer comes first (petition 2395: a player's 20 Honor Guard's
	// blades, 101674's hand-in at the Akessan Agent, kept the Agent refusing
	// and never offering 101665, the quest before them in his line)
	refusal := q.handInRefusalLocked(look, p, c, npc.Name)
	// a step the giver will not do for the player's standing is said only
	// when it has nothing else either (Heirloom petitions 3519/3532)
	stepped, wontDeal := q.stepAtLineLocked(sess, p, c, look, npc.Name, npc.ServerID, false)
	if stepped {
		q.offerAfterFinishLocked(sess, p, c, npc.Name, done)
		return true
	}
	if list := q.boardLocked(look, p, c, npc.Name); list != nil {
		q.askBoardLocked(sess, c, npc.Name, list, 0) // questboard.go
		return true
	}
	if qu := q.offerAtLocked(look, p, c, npc.Name); qu != nil {
		q.offerLocked(sess, p, c, qu)
		return true
	}
	if refusal != "" {
		q.tellLocked(c, sess, npc.Name, refusal)
		return true
	}
	if wontDeal != "" {
		q.tellLocked(c, sess, "", wontDeal)
		log.Printf("Quests: %s: %s", p.Name, wontDeal)
		return true
	}
	if line := q.untrustedLocked(look, p, c, npc.Name); line != "" {
		q.tellLocked(c, sess, npc.Name, sayLines(line)...)
		return true
	}
	// a step pinned to another spawn of this name (the Cazic-Thule trials' three
	// Kithgrins: petition 2530, a player at the second's step talked to the
	// third, who had nothing to say): it says the step's journal line
	if line := q.otherSpawnLineLocked(c, npc.Name, npc.ServerID); line != "" {
		q.tellLocked(c, sess, npc.Name, otherSpawnSays+line)
		return true
	}
	return false
}

// otherSpawnSays begins what an NPC says to a player whose step is at another
// spawn of its name.
const otherSpawnSays = "Not I. "

// otherSpawnLineLocked is the journal line of the player's open step at
// another spawn of the NPC's name (a talk, show or give step pinned to one
// spawn, NPCID, of target name, not this one), "" for none.
func (q *QuestEngine) otherSpawnLineLocked(c *questChar, name string, spawn entity.EntityID) string {
	name = strings.TrimSpace(name)
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		for _, st := range openSteps(qu, cq) {
			if !database.AtNPC(st.Kind) || st.NPCID == 0 || entity.EntityID(st.NPCID) == spawn ||
				!strings.EqualFold(strings.TrimSpace(st.Target), name) {
				continue
			}
			if j := strings.TrimSpace(st.Journal); j != "" {
				return j
			}
			return "Another " + name + " waits for you."
		}
	}
	return ""
}

// ReadyTalk is Talk's first half alone: a quest step at this NPC the player
// can do now (a give or show step's items held), or a hand-in of its quests
// the player holds the items for. HandleInteract asks it before a host's own
// business that would otherwise take every talk (a tour's sleigh ride:
// petition 2368, Santa Claus never took the Grinch's Face). It reports
// whether the quests answered.
func (q *QuestEngine) ReadyTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	if q.stepAtLocked(sess, p, c, look, npc.Name, npc.ServerID, true) {
		return true
	}
	if qu := q.readyHandInLocked(look, p, c, npc.Name); qu != nil {
		q.offerLocked(sess, p, c, qu)
		return true
	}
	return false
}

// TalkStep is Talk without the offers: a quest step done at the NPC called
// name (spawn, its spawn), as a spiritmaster's bind does its step at once
// (answerBind). It reports whether a step answered.
func (q *QuestEngine) TalkStep(sess *session.Session, p *entity.Player, name string, spawn entity.EntityID) bool {
	if q == nil || p == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	return q.stepAtLocked(sess, p, c, look, name, spawn, true) || q.stepAtLocked(sess, p, c, look, name, spawn, false)
}

// stepAtLocked does the first of the player's quest steps at the NPC called
// name (spawn, its spawn), or says why its giver will not deal with the
// player; it reports whether either was said. With ready it does only a step
// that can be done now (a show or give step's items and tunar held) and says
// nothing else: so a step waiting on its items does not stand in the way of
// another quest's at the same NPC (until 2026-11-07 the first in the journal
// answered, whatever the player held).
func (q *QuestEngine) stepAtLocked(sess *session.Session, p *entity.Player, c *questChar, look *factionLook, name string, spawn entity.EntityID, ready bool) bool {
	done, wontDeal := q.stepAtLineLocked(sess, p, c, look, name, spawn, ready)
	if done {
		return true
	}
	if wontDeal != "" && !ready {
		q.tellLocked(c, sess, "", wontDeal)
		log.Printf("Quests: %s: %s", p.Name, wontDeal)
		return true
	}
	return false
}

// stepAtLineLocked is stepAtLocked that does a step but leaves the giver's
// refusal unsaid: done whether a step answered, else wontDeal the line of the
// first quest under way whose giver the player's standing keeps from its step
// ("" none). Talk says it only when the giver has nothing else for the player
// (Heirloom petitions 3519/3532: a repeatable taken again and left at a cap,
// Hinder the Siloths, kept Djivan Mertshak saying "too high" and never
// offering his other quests).
func (q *QuestEngine) stepAtLineLocked(sess *session.Session, p *entity.Player, c *questChar, look *factionLook, name string, spawn entity.EntityID, ready bool) (done bool, wontDeal string) {
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil || q.heldLocked(qu.ID) { // held for an edit: questdefgate.go
			continue
		}
		// its step, or any of its group's not yet done (questanyorder.go)
		for _, st := range openSteps(qu, cq) {
			if !database.AtNPC(st.Kind) || !stepNPC(st, name, spawn) {
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
			if ready && (st.Kind == database.StepGive || st.Kind == database.StepShow) && q.lackingLocked(p, st) != "" {
				continue
			}
			q.atNPCLocked(sess, p, c, qu, cq, st, false, -1)
			return true, ""
		}
	}
	return false, wontDeal
}

// nextOfferLocked is the quest a giver offers the player now, or nil: the
// first of its quests (by id) the player may take and has never finished;
// else, of the repeatable ones it has finished, the one finished fewest times
// (then the first). So a giver whose quests are all repeatable (Chef Lukkif's
// Hunt and Gather) takes the player through them in order and round again,
// where offering the first takeable one offered its first quest for ever.
// skip leaves quests out (a shop's offered this session); nil none.
func (q *QuestEngine) nextOfferLocked(look *factionLook, p *entity.Player, c *questChar, giver string, skip map[int]bool) *database.Quest {
	var again *database.Quest
	for _, qu := range q.cat.givers[foldName(giver)] {
		if skip[qu.ID] || q.mayTakeLocked(look, p, c, qu) != "" {
			continue
		}
		n := c.timesDone(qu.ID)
		if n == 0 {
			return qu
		}
		if again == nil || n < c.timesDone(again.ID) {
			again = qu
		}
	}
	return again
}

// readyHandInLocked is the first quest of this giver the player may take and
// has not just declined whose first step hands the giver items the player
// holds now, or nil: offered before anything else the giver would say, so a
// giver of several such quests (a quest a piece of armour, each repeatable)
// is asked for the one the player has the parts of, in any order, again for a
// replacement. Items only: a key bought for tunar is an ordinary offer.
func (q *QuestEngine) readyHandInLocked(look *factionLook, p *entity.Player, c *questChar, giver string) *database.Quest {
	for _, qu := range q.cat.givers[foldName(giver)] {
		st := qu.First()
		if c.declined[qu.ID] || st == nil || st.Kind != database.StepGive || len(st.Items) == 0 || foldName(st.Target) != foldName(giver) {
			continue
		}
		if q.mayTakeLocked(look, p, c, qu) == "" && q.lackingLocked(p, st) == "" {
			return qu
		}
	}
	return nil
}

// handInRefusalLocked is what a giver says to a player who holds a hand-in's
// items but has not stood with the giver's side (the owner, 2026-11-11: Zaoran
// Plateau's armour only for a player aligned with its lieutenant's side): a
// hand-in that wants a standing (FactionReqs) is refused for a quest to finish
// first or for that standing, in its giver's voice (ours: no recording has
// one), or "" when there is none such.
func (q *QuestEngine) handInRefusalLocked(look *factionLook, p *entity.Player, c *questChar, giver string) string {
	for _, qu := range q.cat.givers[foldName(giver)] {
		st := qu.First()
		if len(qu.FactionReqs) == 0 || st == nil || st.Kind != database.StepGive || len(st.Items) == 0 || foldName(st.Target) != foldName(giver) {
			continue
		}
		if q.lackingLocked(p, st) != "" {
			continue
		}
		why := q.mayTakeLocked(look, p, c, qu)
		if strings.HasPrefix(why, "quest ") || strings.HasPrefix(why, "one of quests ") || strings.HasPrefix(why, "its standing ") {
			return fmt.Sprintf(handInNotYours, qu.FactionReqs[0].Faction)
		}
	}
	return ""
}

// handInNotYours is that refusal: the side the giver works for.
const handInNotYours = "I work only for those who have stood with %s, and you have not. Come back when you have."

// untrustedLocked is what a giver with nothing else to say tells a player its
// standing alone keeps from one of its quests (the quest's faction requirement
// the only thing amiss: mayTakeLocked tests it last), "" for none: the line
// naming the faction (questUntrusted, ours), then the quest's own words on
// earning it (its untrusted column). Heirloom petition 2742 (2026-11-23):
// Apprentice Buerma had "nothing to say" to a player below "The Ancient Stone
// Golem"'s Black Swan Inn requirement, four talks running, and nothing said why.
func (q *QuestEngine) untrustedLocked(look *factionLook, p *entity.Player, c *questChar, giver string) string {
	for _, qu := range q.cat.givers[foldName(giver)] {
		if len(qu.FactionReqs) == 0 || !strings.HasPrefix(q.mayTakeLocked(look, p, c, qu), "its standing ") {
			continue
		}
		r, v, _ := unmetFaction(look, qu)
		line := fmt.Sprintf(questUntrusted, r.Faction)
		if r.HasMax && v > r.Max {
			line = fmt.Sprintf(questTooTrusted, r.Faction)
		}
		log.Printf("Quests: %s's standing with %s (%d) keeps %q from %s", p.Name, r.Faction, v, qu.Name, giver)
		if qu.Untrusted != "" {
			line += "\n" + qu.Untrusted
		}
		return line
	}
	return ""
}

// What a giver says to one its quest's faction requirement keeps from it: a
// standing too low, or too high (ours).
const (
	questUntrusted  = "I do not trust you yet. Earn the trust of the %s first."
	questTooTrusted = "You stand too close to the %s for what I have in mind."
)

// offerAtLocked is the quest a giver talked to offers the player, or nil: the
// next it has (nextOfferLocked) that the player has not just said no to, so a
// no moves on to its next quest; with none other left, round again.
func (q *QuestEngine) offerAtLocked(look *factionLook, p *entity.Player, c *questChar, giver string) *database.Quest {
	qu := q.nextOfferLocked(look, p, c, giver, c.declined)
	if qu == nil && len(c.declined) > 0 {
		for _, g := range q.cat.givers[foldName(giver)] {
			delete(c.declined, g.ID)
		}
		qu = q.nextOfferLocked(look, p, c, giver, nil)
	}
	return qu
}

// stepNPC is whether a step names this NPC: by its target or one of its others
// (Also), a kill or loot step's creatures and a talk, show or give step's NPCs
// alike (either seller of a step's item). A step pinned to one spawn (NPCID)
// takes its target at that spawn only; its others count at any of theirs.
// stepKill reports a kill a kill or loot step takes: its creature by name
// (stepNPC), or any creature that died within its area (AnyCreature). A step
// with a place of its own (queststeps x, z) takes its others (Also) only
// within huntPlaceReach of it, as its marks show them: a family's name stands
// all over the world (Heirloom petition 2718: the tomb rats' "large rat" of
// the River Saren tower's quests dropped their sigils off Halas's rats).
func stepKill(st *database.QuestStep, k entity.KillCredit) bool {
	if (st.Quest == 109911 || st.Quest == 109912) && isThanksgivingQuest(st.Quest) {
		return k.World == 0 && k.NPCSpawn != 0 && stepNPC(st, k.NPCName, k.NPCSpawn) && math.Hypot(float64(k.X-st.X), float64(k.Z-st.Z)) <= float64(st.Radius)
	}
	if st.Quest == turkeyQuest && st.Kind == database.StepKill && isThanksgivingQuest(st.Quest) {
		return k.World == 0 && k.NPCSpawn == 0 && k.NPCName == "Turkey King" && k.OriginalRow == 197601
	}
	if st.TakesKillAt(int(k.World), k.X, k.Z) {
		return true
	}
	if !stepNPC(st, k.NPCName, k.NPCSpawn) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(st.Target), strings.TrimSpace(k.NPCName)) || st.AnyInArea() || (st.X == 0 && st.Z == 0) {
		return true
	}
	dx, dz := float64(k.X-st.X), float64(k.Z-st.Z)
	return int(k.World) == st.World && math.Hypot(dx, dz) <= huntPlaceReach
}

func stepNPC(st *database.QuestStep, name string, spawn entity.EntityID) bool {
	name = strings.TrimSpace(name)
	if strings.EqualFold(strings.TrimSpace(st.Target), name) {
		return st.NPCID == 0 || entity.EntityID(st.NPCID) == spawn
	}
	if !database.Hunts(st.Kind) && !database.AtNPC(st.Kind) {
		return false
	}
	for _, t := range st.Also {
		if strings.EqualFold(strings.TrimSpace(t), name) {
			return true
		}
	}
	return false
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
	return q.mayTakeAtLocked(look, p, c, qu, p.Level)
}

// mayTakeAtLocked is mayTakeLocked for the player at a level of its own: at
// the quest's lowest it says whether the level is all the player lacks (the
// giver's "not yet", quests_notyet.go).
func (q *QuestEngine) mayTakeAtLocked(look *factionLook, p *entity.Player, c *questChar, qu *database.Quest, level int) string {
	switch {
	case q.heldLocked(qu.ID):
		return "it is being changed (questdefgate.go)"
	case !qu.Enabled:
		return "it is a draft"
	case qu.First() == nil:
		return "it has no steps"
	case qu.MinLevel > 0 && level < qu.MinLevel:
		return fmt.Sprintf("it starts at level %d", qu.MinLevel)
	case qu.MaxLevel > 0 && level > qu.MaxLevel:
		return fmt.Sprintf("it ends at level %d", qu.MaxLevel)
	case len(qu.Classes) > 0 && !containsInt(qu.Classes, p.Class):
		return "not for that class"
	case len(qu.Races) > 0 && !questRaceTakes(qu.Races, p.Race):
		return "not for that race"
	}
	if qu.Source == "thanksgiving-2026" {
		if !q.thanksgivingOpen() {
			return "Thanksgiving has ended"
		}
		if qu.ID != turkeyQuest {
			main := c.quests[turkeyQuest]
			if main == nil || main.Step != 1 {
				return "the King must be awaiting your marks"
			}
			if !q.turkeyPatrolOpen(p.CharServerID) {
				return "the muster has paid its share"
			}
		}
	}
	// a homeland's quest stands finished when the city class quest it stands
	// for is (standsFinishedLocked): for its own offer and for the next one's
	finished := func(id int) bool {
		if line, ok := q.wereQuestsLocked().markerLine[id]; ok {
			return q.feverLineLocked(c, time.Now()) == line // a fever marker is the infection in force (wereinfect.go)
		}
		return c.hasFinished(id) || q.standsFinishedLocked(p, c, q.cat.byID[id])
	}
	if g, unmet := database.UnmetRequire(qu.Required(), finished); unmet {
		if len(g) == 1 {
			return fmt.Sprintf("quest %d is to be finished first", g[0])
		}
		return fmt.Sprintf("one of quests %s is to be finished first", formatAlternatives(g))
	}
	// a twin's doing (database.Quest.Excludes, petition 2199): finished, or under way
	for _, id := range qu.Excludes {
		if id == qu.ID {
			continue
		}
		if finished(id) {
			return fmt.Sprintf("quest %d, which excludes it, is finished", id)
		}
		if cq := c.quests[id]; cq != nil && cq.Active() {
			return fmt.Sprintf("quest %d, which excludes it, is under way", id)
		}
	}
	// a day group (database.Quest.DayGroup, work entry 1597): its quests one a
	// day, one under way at a time
	if qu.DayGroup != "" {
		today := time.Now().UTC().Format("2006-01-02")
		for id, g := range q.cat.byID {
			if id == qu.ID || !strings.EqualFold(g.DayGroup, qu.DayGroup) {
				continue
			}
			if cq := c.quests[id]; cq != nil {
				if cq.Active() {
					return fmt.Sprintf("quest %d of its day group is under way", id)
				}
				if cq.Done > 0 && strings.HasPrefix(cq.Updated, today) {
					return fmt.Sprintf("quest %d of its day group was finished today: again tomorrow", id)
				}
			}
		}
	}
	if cq := c.quests[qu.ID]; cq != nil {
		if cq.Active() {
			return "under way"
		}
		if cq.Done > 0 && !qu.Repeatable {
			return "finished"
		}
		// a daily quest (repeatable 2) once a day: finished today, again tomorrow
		if cq.Done > 0 && qu.Daily && strings.HasPrefix(cq.Updated, time.Now().UTC().Format("2006-01-02")) {
			return "finished today: again tomorrow"
		}
	}
	if !qu.Repeatable && q.standsFinishedLocked(p, c, qu) {
		return "finished: the city class quest it stands for is"
	}
	// a hand-in done on the answer (a dostep on its first step) is offered only to
	// one who holds it: else the answer would leave the player a quest it cannot do
	if st := qu.First(); (st.Kind == database.StepGive || st.Kind == database.StepShow) && database.DoesStep(st) {
		if lack := q.lackingLocked(p, st); lack != "" {
			return "it lacks " + lack
		}
	}
	// last: the only test that asks the World
	if r, v, unmet := unmetFaction(look, qu); unmet {
		return fmt.Sprintf("its standing with %s is %d, not %s", r.Faction, v, database.FormatFactionReqs([]database.FactionReq{r}))
	}
	return ""
}

// hasFinished is whether the character has finished the quest at least once.
func (c *questChar) hasFinished(id int) bool { return c.timesDone(id) > 0 }

// questRaceTakes reports a quest's races (not empty) taking a character of
// race. A race of the game's and the froglok (0 to 10) is asked of the
// character's wire race: a race of the server's own does the quests of the
// race it passes as (an Aviak Halas's, as a barbarian). A race of racedefs
// (entity.FirstCustomRace and up) is asked of the character's own race: a
// homeland's quests name its people alone, so a barbarian who rides the coach
// into Aviak Village is offered none of the Roost's (the owner, 2026-11-10:
// "the coach will connect to freeport and the other homeland areas").
func questRaceTakes(races []int, race int) bool {
	wire := entity.WireRace(race)
	for _, r := range races {
		if r >= entity.FirstCustomRace {
			if r == race {
				return true
			}
		} else if r == wire {
			return true
		}
	}
	return false
}

// timesDone is how many times the character has finished the quest.
func (c *questChar) timesDone(id int) int {
	if cq := c.quests[id]; cq != nil {
		return cq.Done
	}
	return 0
}

// formatAlternatives is a group of quest ids as a line says it: "1, 2 or 3".
func formatAlternatives(g []int) string {
	parts := make([]string, len(g))
	for i, id := range g {
		parts[i] = fmt.Sprint(id)
	}
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
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
		q.tellLocked(c, sess, questVoice(qu), sayLines(q.froglokWords(p, qu.ID, 0, qu.Giver, qu.Offer))...)
		q.startLocked(sess, p, c, qu, 0, false)
		return
	}
	q.askStageLocked(sess, p, c, qu, 0)
	log.Printf("Quests: %s offered %q", p.Name, qu.Name)
}

// askStageLocked asks the offer's question k, as the reference server's Commander Nothard
// asks two: the offer's paragraph k (its lines a box each, its last line the
// question; any paragraphs past the answers said before the last question),
// answered by Accept's line k, or Decline.
func (q *QuestEngine) askStageLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, k int) {
	answers := sayLines(qu.Accept)
	if k >= len(answers) {
		return
	}
	paras := paragraphs(q.froglokWords(p, qu.ID, 0, qu.Giver, qu.Offer))
	words := ""
	switch {
	case k == len(answers)-1 && k < len(paras):
		words = strings.Join(paras[k:], "\n")
	case k < len(paras):
		words = paras[k]
	}
	q.askLocked(sess, c, questVoice(qu), words, questAskOffer, offerChoices(qu, k, nil))
}

// offerChoices is the answers of an offer's stage k: the accept, the decline
// and, at the last stage, the asks open after those picked (openAsks), six
// answers at most.
func offerChoices(qu *database.Quest, k int, picked []string) []questChoice {
	answers := sayLines(qu.Accept)
	choices := []questChoice{{kind: choiceAccept, quest: qu.ID, step: k, label: answers[k]}}
	if d := declineAt(qu, k); d != "" {
		choices = append(choices, questChoice{kind: choiceDecline, quest: qu.ID, label: d})
	}
	if k != len(answers)-1 {
		return choices
	}
	for _, o := range openAsks(qu.Asks, picked) {
		if len(choices) >= database.AskMostAtOnce {
			break
		}
		choices = append(choices, questChoice{kind: choiceAsk, quest: qu.ID, step: k, label: o.ask.Answer, ask: o.path, picked: picked})
	}
	return choices
}

// declineAt is the refusal offered beside the offer's question k (work board 883: the live givers refused
// per question): decline read as accept is, a line a question, an empty line that question's none; a
// decline of one line (no \n) is the refusal of every question, as before.
func declineAt(qu *database.Quest, k int) string {
	raw := strings.ReplaceAll(qu.Decline, "\r", "")
	if !strings.Contains(raw, "\n") {
		return strings.TrimSpace(raw)
	}
	lines := strings.Split(raw, "\n")
	if k < 0 || k >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[k])
}

// openAsk is an ask that may be asked now and its path.
type openAsk struct {
	path string
	ask  database.QuestAsk
}

// openAsks is the asks open once those of picked are asked: each top-level
// ask not picked, then the follow-ups of each picked one not picked, in list
// order (work entry 1597).
func openAsks(asks []database.QuestAsk, picked []string) []openAsk {
	was := map[string]bool{}
	for _, p := range picked {
		was[p] = true
	}
	var out []openAsk
	var walk func(as []database.QuestAsk, prefix string, open bool)
	walk = func(as []database.QuestAsk, prefix string, open bool) {
		for i, a := range as {
			path := prefix + strconv.Itoa(i)
			if open && !was[path] {
				out = append(out, openAsk{path: path, ask: a})
			}
			if was[path] {
				walk(a.Asks, path+".", true)
			}
		}
	}
	walk(asks, "", true)
	return out
}

// askAt is the ask at path, or nil.
func askAt(asks []database.QuestAsk, path string) *database.QuestAsk {
	var at *database.QuestAsk
	for _, part := range strings.Split(path, ".") {
		i, err := strconv.Atoi(part)
		if err != nil || i < 0 || i >= len(asks) {
			return nil
		}
		at = &asks[i]
		asks = at.Asks
	}
	return at
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
	// what taking it hands over must fit the pack (questroom.go); a GM's
	// give is the GM's to judge
	if !gm {
		if short := q.packShortLocked(sess, nil, startGains(qu, st)); short != "" {
			q.tellLocked(c, sess, questVoice(qu), fmt.Sprintf(questPackFull, short))
			return "the pack is too full for what it hands over"
		}
	}
	cq := c.quests[qu.ID]
	if cq == nil {
		cq = &database.CharQuest{ServerID: c.serverID, QuestID: qu.ID}
		c.quests[qu.ID] = cq
	}
	// a run of its own: a finished run's picks are not this one's
	cq.Step, cq.Count, cq.Slot, cq.Picks = st.Step, 0, c.nextSlot(), ""
	q.saveLocked(cq)
	if cq.Done == 0 {
		q.store.MarkQuestTaken(c.serverID, qu.ID, time.Now()) // the Heirloom Marks' real-time limit (marks.go)
	}
	// the live game's order (board 967: two tapes, 2009 and 2012): the
	// journal's line, the giver's words, then "You received a quest!"
	q.addLineLocked(sess, c, q.lineLocked(cq))
	q.tellLocked(c, sess, questVoice(qu), sayLines(qu.Started)...)
	q.tellLocked(c, sess, "", questReceived)
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
	if line := q.triggerBusyLocked(st); line != "" {
		q.tellLocked(c, sess, st.Target, line)
		return
	}
	// a standing a trial's setback took, not yet earned back (questtrials.go)
	if line := q.owesRefusalLocked(sess, c, qu, st); line != "" {
		q.tellLocked(c, sess, st.Target, line)
		log.Printf("Quests: %s at %q step %d still owes the standing a setback took", p.Name, qu.Name, st.Step)
		return
	}
	if st.Kind == database.StepGive || st.Kind == database.StepShow {
		if lack := q.lackingLocked(p, st); lack != "" {
			log.Printf("Quests: %s at %q step %d still lacks %s", p.Name, qu.Name, st.Step, lack)
			// met without it, a trial's price (questtrials.go: Nivaan D'Tarin
			// without the Delegate's sealed letter): its words, then the events
			if evs := database.EventsOn(st.Events, database.EventLacking); len(evs) > 0 {
				q.tellLocked(c, sess, st.Target, sayLines(orText(st.NotYet, orText(st.Reminder, orText(st.Waiting, questNotYet))))...)
				q.runEventsLocked(sess, p, c, qu, st.Step, st.Step, evs)
				return
			}
			// the reference server's Norgam asks his question with only the not-yet
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
	// (the reference server's Zole: "Commander Nothard sent me", "Farewell")
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
		// a trial's wrong answer: the setback, not the step (questtrials.go)
		if opt.Back > 0 {
			q.optionSetbackLocked(sess, p, c, qu, cq, st, opt)
			return
		}
		if opt.Stay {
			q.optionFactionsLocked(sess, p, opt) // a price that leaves the step to do
			q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
			log.Printf("Quests: %s picked %q at %q step %d: the step stays", p.Name, opt.Answer, qu.Name, st.Step)
			return
		}
		// another path than the one the Spell Archivist wrote: the step stays to do (questpathpick.go)
		if line, refused := q.pathPickRefusedLocked(p, opt); refused {
			q.tellLocked(c, sess, st.Target, fmt.Sprintf(questPathHeld, line))
			log.Printf("Quests: %s picked %q at %q step %d: the book holds the archivist's path of %q", p.Name, opt.Answer, qu.Name, st.Step, line)
			return
		}
		// a bribe the player cannot pay: the step stays to do
		if opt.Cost > 0 && p.Tunar < opt.Cost {
			q.tellLocked(c, sess, st.Target, fmt.Sprintf(questChoiceCost, opt.Cost))
			log.Printf("Quests: %s picked %q at %q step %d without the %d tunar", p.Name, opt.Answer, qu.Name, st.Step, opt.Cost)
			return
		}
		q.optionFactionsLocked(sess, p, opt) // the standings an option changes (questtrials.go)
	}
	// a held trigger (database.QuestEvent's Hold): the trigger fired, the step
	// left to its queststep (questhold.go)
	if ev := database.HoldTrigger(st); ev != nil && opt == nil {
		q.holdLocked(sess, p, c, qu, st, *ev)
		return
	}
	// what doing the step puts in the pack must fit it, after what it takes,
	// else nothing changes hands and the step stays to do (questroom.go)
	var extraRw *database.ClassReward
	if opt != nil {
		extraRw = &opt.Reward
	}
	if short := q.packShortLocked(sess, q.handInTakes(p, st), stepGains(p, qu, st, extraRw)); short != "" {
		q.tellLocked(c, sess, st.Target, fmt.Sprintf(questPackFull, short))
		log.Printf("Quests: %s's step %d of %q waits for room for %s", p.Name, st.Step, qu.Name, short)
		return
	}
	if qu.Source == "thanksgiving-2026" && (st.Kind == database.StepTalk || st.Kind == database.StepGive) {
		q.turkeyReport(sess, p, c, qu, cq, st)
		return
	}
	var given *handedIn
	switch st.Kind {
	case database.StepGive:
		if (opt == nil || opt.Cost == 0) && len(handInTriggers(st)) > 0 {
			// its doing fires an encounter: the run reserved, the items and
			// the progress one transaction, then the run started (questatomic.go)
			q.atomicHandInLocked(sess, p, c, qu, cq, st, opt, pick)
			return
		}
		var ok bool
		if given, ok = q.takeLocked(sess, p, st); !ok {
			q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, questNotYet))...)
			return
		}
	case database.StepShow:
		// what is shown is kept; shown Ascended, the rewards are (questascended.go);
		// its tunar, a fare, is paid (the lack check above held it)
		given = &handedIn{ascended: q.planHandInLocked(p, st).ascended}
		q.payStepTunarLocked(sess, p, st, given)
	}
	var extra *database.ClassReward
	skip := 0
	if opt != nil {
		if opt.Cost > 0 && q.inv != nil {
			p.Tunar -= opt.Cost
			economy.Spent(economy.QuestPayment, p.CharServerID, p.Name, opt.Cost)
			if err := q.inv.SetPurse(p.CharServerID, p.Tunar); err != nil {
				log.Printf("Quests: %s's tunar: %v", p.Name, err)
			}
			if sess != nil {
				queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(p.Tunar))
			}
			if given == nil {
				given = &handedIn{}
			}
			given.names = append(given.names, fmt.Sprintf("%d tunar", opt.Cost))
		}
		skip = opt.Skip
		q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
		extra = &opt.Reward
		// kept in the quest's row (completeStepLocked saves it): what a
		// rehabilitator undoing a path takes back (rehab.go)
		cq.Picks = database.WithPick(cq.Picks, st.Step, pick)
		log.Printf("Quests: %s picked %q at %q step %d", p.Name, opt.Answer, qu.Name, st.Step)
	}
	q.completeStepSkipLocked(sess, p, c, qu, cq, st, given, extra, skip)
	// a repeatable turn-in finished with more sets held: "Hand in all" (questbulk.go)
	q.offerBulkLocked(sess, p, c, qu, cq, st)
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
		return true // a dead end (the reference server's "Farewell"): nothing changes
	}
	qu := q.cat.byID[ch.quest]
	if qu == nil {
		return true
	}
	if q.heldLocked(qu.ID) {
		// an old box of a quest being changed: nothing of it is done (questdefgate.go)
		q.tellLocked(c, sess, "", questHeldLine)
		return true
	}
	switch ch.kind {
	case choiceBoard:
		if why := q.mayTakeLocked(q.lookAt(sess), p, c, qu); why != "" {
			log.Printf("Quests: %s picked %q off its giver's board, but %s", p.Name, qu.Name, why)
			return true
		}
		q.offerLocked(sess, p, c, qu)
	case choiceBoardMore:
		if list := q.boardLocked(q.lookAt(sess), p, c, qu.Giver); list != nil {
			q.askBoardLocked(sess, c, qu.Giver, list, ch.option)
		}
	case choiceAccept:
		if why := q.mayTakeLocked(q.lookAt(sess), p, c, qu); why != "" {
			log.Printf("Quests: %s answered to take %q, but %s", p.Name, qu.Name, why)
			return true
		}
		if ch.step+1 < len(sayLines(qu.Accept)) {
			q.askStageLocked(sess, p, c, qu, ch.step+1)
			return true
		}
		c.declined = nil
		q.startLocked(sess, p, c, qu, 0, false)
	case choiceDecline:
		// the giver's reply, said as written (the reference server's Norgam's has no
		// name before it)
		q.tellLocked(c, sess, "", sayLines(qu.Declined)...)
		if c.declined == nil {
			c.declined = map[int]bool{}
		}
		c.declined[qu.ID] = true
		q.antidoteLocked(c, qu) // a were line's third quest: its antidote (wereinfect.go)
		log.Printf("Quests: %s refused %q", p.Name, qu.Name)
	case choiceAsk:
		// the giver's reply, then the offer's question again: the ask
		// picked drops out, its follow-ups come in; nothing is kept
		a := askAt(qu.Asks, ch.ask)
		if a == nil || q.mayTakeLocked(q.lookAt(sess), p, c, qu) != "" {
			return true
		}
		picked := append(append([]string{}, ch.picked...), ch.ask)
		paras := paragraphs(qu.Offer)
		question := ""
		if n := len(paras); n > 0 {
			if ls := sayLines(paras[n-1]); len(ls) > 0 {
				question = ls[len(ls)-1]
			}
		}
		q.askLocked(sess, c, questVoice(qu), strings.TrimSpace(a.Says+"\n"+question), questAskOffer, offerChoices(qu, ch.step, picked))
	case choiceBulk:
		q.bulkHandInLocked(sess, p, c, qu, ch.option)
	case choiceNotYet:
		if st := qu.Step(ch.step); st != nil {
			q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, questNotYet))...)
		}
	case choiceStep, choicePick:
		cq := c.quests[qu.ID]
		st := qu.Step(ch.step)
		if cq == nil || !isOpen(qu, cq, ch.step) || st == nil {
			return true
		}
		pick := -1
		if ch.kind == choicePick {
			pick = ch.option
		}
		done := q.doneCount(c)
		q.atNPCLocked(sess, p, c, qu, cq, st, true, pick)
		q.offerAfterFinishLocked(sess, p, c, st.Target, done)
	}
	return true
}

// doneCount is how many times the character has finished its quests that are
// not repeatable, all told: a finish in a talk moves it. A repeatable one (a
// trade picked in a talk tree, a hand-in made again) leads to no next quest.
func (q *QuestEngine) doneCount(c *questChar) int {
	n := 0
	for id, cq := range c.quests {
		if qu := q.cat.byID[id]; qu != nil && !qu.Repeatable {
			n += cq.Done
		}
	}
	return n
}

// offerAfterFinishLocked is a talk's (or a step's answer's) last word when it
// finished a quest that is not repeatable at the NPC called giver (doneBefore: doneCount before it)
// and left no question open: the giver's board, else the quest it offers the
// player now (its next in a line, the finished one its requirement; never one
// finished before, a repeatable's next round), asked at once. Heirloom petition 2735 (2026-11-23: "you have to talk to a quest NPC
// after you should already have the flag ... It just did it again on
// Talknar"): the hand-in that finished "The Broken Pact" at Talknar said its
// words, and "The Ambassadors", his next, came only at a second talk, as
// every quest line's next did; the head mark over him already showed it.
func (q *QuestEngine) offerAfterFinishLocked(sess *session.Session, p *entity.Player, c *questChar, giver string, doneBefore int) {
	if sess == nil || p == nil || c.box != nil || q.doneCount(c) <= doneBefore || strings.TrimSpace(giver) == "" {
		return
	}
	look := q.lookAt(sess)
	if list := q.boardLocked(look, p, c, giver); list != nil {
		q.askBoardLocked(sess, c, giver, list, 0)
		return
	}
	// a new quest only: a repeatable one again waits for the next talk
	if qu := q.offerAtLocked(look, p, c, giver); qu != nil && c.timesDone(qu.ID) == 0 {
		log.Printf("Quests: %s finished a quest at %s, who offers %q at once", p.Name, giver, qu.Name)
		q.offerLocked(sess, p, c, qu)
	}
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
		// a pointer's line (questpointers.go): not shown again this session
		if idx >= 0 && q.dropPointerLocked(sess, c, int(idx)) {
			log.Printf("Quests: %s deleted a pointer's line", p.Name)
		}
		return nil
	}
	cq := act[idx]
	never := cq.Done == 0
	q.giveUpLocked(cq)
	q.dropLineLocked(sess, c, int(idx))
	log.Printf("Quests: %s gave up quest %d", p.Name, cq.QuestID)
	if never {
		for _, id := range q.forgetChoicesLocked(c, cq.QuestID) {
			log.Printf("Quests: %s gave up quest %d, the choice of path it was taken by, with it", p.Name, id)
		}
	}
	return nil
}

// forgetChoicesLocked undoes the choice of path a quest given up unfinished
// was taken by (Heirloom petition 2683: a ratman wizard took the good side's
// epic by mistake, deleted it and was refused the evil side's for ever). A
// choice is a finished quest the given-up one requires that excludes others
// (a recruiter: tools/dwarf_epic_choice_20261122.py), which nothing else the
// character holds or has finished requires: it is forgotten, as if never
// taken, so the other path's quests it excluded are offered again, and it can
// be taken again. It returns the quests forgotten.
func (q *QuestEngine) forgetChoicesLocked(c *questChar, given int) []int {
	qu := q.cat.byID[given]
	if qu == nil {
		return nil
	}
	var out []int
	for _, g := range qu.Required() {
		for _, id := range g {
			choice := q.cat.byID[id]
			cq := c.quests[id]
			if choice == nil || len(choice.Excludes) == 0 || !isChoiceQuest(choice) || cq == nil || cq.Done == 0 || cq.Active() {
				continue
			}
			if q.requiredByOtherLocked(c, id, given) {
				continue
			}
			if err := q.store.DeleteCharQuest(cq.ServerID, id); err != nil {
				log.Printf("Quests: forgetting character %d's quest %d: %v", cq.ServerID, id, err)
				continue
			}
			delete(c.quests, id)
			c.forgetOffer(id)
			out = append(out, id)
		}
	}
	return out
}

// isChoiceQuest is a quest that is only a choice: one talk step that pays
// nothing (no experience, tunar, spells, standings or class rewards; an item
// such as a recruiter's token, which the path's first quest takes, may be
// given), so taking it again gains nothing.
func isChoiceQuest(qu *database.Quest) bool {
	if len(qu.Steps) != 1 {
		return false
	}
	st := qu.Steps[0]
	return st.Kind == database.StepTalk && st.XP == 0 && st.RewardTunar == 0 && len(st.Spells) == 0 &&
		len(st.Factions) == 0 && len(st.ByClass) == 0
}

// requiredByOtherLocked is whether a quest other than except, held or
// finished by the character, requires id.
func (q *QuestEngine) requiredByOtherLocked(c *questChar, id, except int) bool {
	for qid := range c.quests {
		if qid == except || qid == id {
			continue
		}
		other := q.cat.byID[qid]
		if other == nil {
			continue
		}
		for _, g := range other.Required() {
			if slices.Contains(g, id) {
				return true
			}
		}
	}
	return false
}

// giveUpLocked ends a quest under way unfinished: forgotten when it was never
// finished, else left finished as many times as it was.
func (q *QuestEngine) giveUpLocked(cq *database.CharQuest) {
	q.clearSpawnsLocked(cq.ServerID, cq.QuestID, 0)
	if c := q.chars[cq.ServerID]; c != nil {
		c.forgetOffer(cq.QuestID)
	}
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

// killed counts a kill on the kill steps of every player it was credited to, and puts the
// loot steps' items on its corpse, each once, for whoever loots it (questpreloot.go).
func (q *QuestEngine) killed(k entity.KillCredit) {
	q.mu.Lock()
	defer q.mu.Unlock()
	chars := make([]*questChar, len(k.Players))
	for i, kp := range k.Players {
		chars[i] = q.namedLocked(kp.ServerID, kp.Name)
	}
	q.prefarm = &prefarmKill{k: k} // /prefarm (prefarm.go): read at its first need
	defer func() { q.prefarm = nil }()
	dropped := q.corpseDropsLocked(k, chars) // before the steps move on: who is on which, at the kill
	defer q.placeDropsLocked(k, dropped)     // after: but those a full stack counted (lootKillLocked)
	for i, kp := range k.Players {
		c := chars[i]
		for _, cq := range c.active() {
			qu := q.cat.byID[cq.QuestID]
			if qu == nil || q.heldLocked(qu.ID) { // held for an edit: questdefgate.go
				continue
			}
			st := qu.Step(cq.Step)
			if st != nil && st.Kind == database.StepLoot && stepKill(st, k) {
				q.lootKillLocked(k, kp, c, qu, cq, st, dropped) // questevents.go
				continue
			}
			if st == nil || st.Kind != database.StepKill || !stepKill(st, k) {
				q.laterStepKillLocked(c, kp, qu, st, k)
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
				q.nextWaveLocked(p, c, qu, st, k.NPCID, need-cq.Count) // questevents.go
				continue
			}
			if q.waitsForRoomLocked(kp.Session, p, c, qu, cq, st) {
				continue
			}
			q.completeStepLocked(kp.Session, p, c, qu, cq, st, nil, nil)
		}
	}
}

// laterStepKillLocked tells a player whose kill is a later step's of a quest
// (a kill or loot step after the one to do) that it counts for nothing yet, and
// what to do first (petition 1874, 2026-11-13: "Quil'Eyne's Journal"
// asks Randarith's blood, then Raldarith's, then Quil'Eyne's journal; a player
// took the first blood, went on to Quil'Eyne with Raldarith's still to get, and
// Quil'Eyne dropped nothing and the journal said nothing). A kill only ever
// counted on the step to do; it now says so. Nothing is said for a kill no
// later step names, nor on a step that is not a kill or a loot step's (a talk,
// a give: the player knows whom to see).
func (q *QuestEngine) laterStepKillLocked(c *questChar, kp entity.KillPlayer, qu *database.Quest, st *database.QuestStep, k entity.KillCredit) {
	if kp.Session == nil || st == nil {
		return
	}
	now := time.Now()
	if t, ok := c.laterSaid[qu.ID]; ok && now.Sub(t) < laterSayGap {
		return // said a moment ago: a camp of them says it once
	}
	for i := range qu.Steps {
		later := &qu.Steps[i]
		if later.Step <= st.Step || !database.Hunts(later.Kind) || !stepKill(later, k) {
			continue
		}
		first := strings.TrimSpace(st.Journal)
		if first == "" {
			first = fmt.Sprintf("step %d", st.Step)
		}
		if c.laterSaid == nil {
			c.laterSaid = map[int]time.Time{}
		}
		c.laterSaid[qu.ID] = now
		if item, ok := q.dropsAheadNameLocked(kp, c, qu, later, k.NPCName); ok {
			// its item drops ahead of the step (questpreloot.go): kept for it, counted there
			SendClientMessage(kp.Session, fmt.Sprintf("%s: %s may drop %s ahead; it counts when you reach that step. First: %s", qu.Name, k.NPCName, item, first))
			log.Printf("Quests: %s killed %q for step %d of %q, at step %d: told its item drops ahead", kp.Name, k.NPCName, later.Step, qu.Name, st.Step)
			return
		}
		SendClientMessage(kp.Session, fmt.Sprintf("%s: %s counts later in the quest. First: %s", qu.Name, k.NPCName, first))
		log.Printf("Quests: %s killed %q for step %d of %q, at step %d: told what comes first", kp.Name, k.NPCName, later.Step, qu.Name, st.Step)
		return
	}
}

// dropsAheadNameLocked is a later loot step's item's name, and whether its creature's kill
// drops it with the player not on the step yet (questpreloot.go: to anyone, or ahead of the
// step for this player).
func (q *QuestEngine) dropsAheadNameLocked(kp entity.KillPlayer, c *questChar, qu *database.Quest, later *database.QuestStep, killed string) (string, bool) {
	if later.Kind != database.StepLoot || kp.Session == nil || q.items == nil || q.aheadOffLocked() {
		return "", false // /prefarm off: nothing drops ahead on this kill (prefarm.go)
	}
	pid, _, _ := later.LootItem()
	pat, err := q.items.GetItemPattern(pid)
	if err != nil || pat == nil || q.aheadExcludedLocked(qu, later, pat) != "" {
		return "", false
	}
	if q.openDropLocked(qu, later, pat, killed) || qu.Enabled && q.aheadRefusalLocked(c, qu, later, killed) == "" {
		return pat.ItemName, true
	}
	return "", false
}

// laterSayGap is how long after saying a quest's later kill counts later it is
// not said again for that quest.
const laterSayGap = time.Minute

// checkReach does the reach steps of the players standing in their places.
// A step whose doing fires a trigger of a lock held (triggerBusyLocked: the
// Kratas quest's last step at the whale bones raises Kratas, one at a time)
// is not done while it is held, as a talk step's is not: the player is told,
// once a laterSayGap, and the step is done when the lock is free and the
// player still there. Before, it was done and the trigger did not fire: the
// quest finished with no Kratas.
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
			world, x, y, z, ok := q.gw.PlayerPlace(p)
			if qu.ID == turkeyQuest && qu.Source == "thanksgiving-2026" && st.Kind == database.StepReach {
				if !ok || !c.westGate.crosses(world, x, y, z, time.Now()) || !q.thanksgivingOpen() {
					continue
				}
			} else if !ok || int(world) != st.World || !withinHeight(st, y) {
				continue
			}
			r := st.Radius
			if r <= 0 {
				r = 15
			}
			if dx, dz := x-st.X, z-st.Z; !(qu.ID == turkeyQuest && qu.Source == "thanksgiving-2026") && dx*dx+dz*dz > r*r {
				continue
			}
			if line := q.triggerBusyLocked(st); line != "" && !(qu.Source == "thanksgiving-2026" && Triggers().RunningNamed("The King's Last Thanksgiving")) {
				q.reachBusyLocked(sess, c, qu, line, time.Now())
				continue
			}
			if reachTakes(st) {
				if lack := q.lackingLocked(p, st); lack != "" {
					q.reachBusyLocked(sess, c, qu, orText(st.Reminder, orText(st.Waiting, questReachLacks)), time.Now())
					continue
				}
			}
			if q.waitsForRoomLocked(sess, p, c, qu, cq, st) {
				continue
			}
			if reachTakes(st) {
				give := *st
				give.Kind = database.StepGive
				if _, ok := q.takeLocked(sess, p, &give); !ok {
					continue
				}
				log.Printf("Quests: %s set down step %d's items of %q", p.Name, st.Step, qu.Name)
			}
			q.completeStepLocked(sess, p, c, qu, cq, st, nil, nil)
		}
	}
}

// reachTakes is whether a reach step wants items or tunar held: it is done
// only with them in the pack, and they are taken at its doing (set down, as
// the primal stew of "The Scepter of the Ritualists" is in Permafrost to
// raise the enraged Lady Vox, tools/enraged_vox_20261116.py). The player
// there without them is told its reminder (or waiting line), once a
// laterSayGap, and the step stays.
func reachTakes(st *database.QuestStep) bool {
	return st.Kind == database.StepReach && (len(st.Items) > 0 || st.Tunar > 0)
}

// questReachLacks is what a player at a reach step that wants items is told
// without them, when the step has no reminder or waiting line of its own.
const questReachLacks = "You lack what is needed here."

// reachBusyLocked tells a player at a reach step whose trigger's lock is held
// what the step's event says (triggerBusyLocked), once a laterSayGap a quest.
func (q *QuestEngine) reachBusyLocked(sess *session.Session, c *questChar, qu *database.Quest, line string, now time.Time) {
	if t, ok := c.busyTold[qu.ID]; ok && now.Sub(t) < laterSayGap {
		return
	}
	if c.busyTold == nil {
		c.busyTold = map[int]time.Time{}
	}
	c.busyTold[qu.ID] = now
	q.tellLocked(c, sess, "", line)
}

// withinHeight is whether a player at height y is on a reach step's floor:
// within its Height of its Y, any height when it has none.
func withinHeight(st *database.QuestStep, y float32) bool {
	if st.Height <= 0 {
		return true
	}
	d := y - st.Y
	return d <= st.Height && -d <= st.Height
}

// completeStepLocked does a step in the reference server's order (its first warrior
// quest, recorded): the NPC's words, the journal's line taken (and the next
// step's added), "You finished a quest!" after the last step, a line for
// what was handed in and each item given, then the experience and the
// faction lines. extra is the rewards of the option the player picked (nil
// none), given besides the step's.
func (q *QuestEngine) completeStepLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, given *handedIn, extra *database.ClassReward) {
	q.completeStepSkipLocked(sess, p, c, qu, cq, st, given, extra, 0)
}

// completeStepSkipLocked is completeStepLocked passing over skip steps after
// st (a choice's Skip: the bribe that passes over the fight), none of their
// events fired; past the last, the quest is finished.
func (q *QuestEngine) completeStepSkipLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, given *handedIn, extra *database.ClassReward, skip int) {
	delete(c.packTold, [2]int{qu.ID, st.Step})
	idx := c.index(qu.ID)
	next := qu.Next(st.Step)
	for ; skip > 0 && next != nil; skip-- {
		next = qu.Next(next.Step)
	}
	// a step of a group done in any order (questanyorder.go): the quest stays
	// on the group until all of it is done, then goes on past its last step
	stay, oldLine := false, q.lineLocked(cq)
	if all, last, ok := anyOrderDone(qu, cq, st); ok {
		next, stay = qu.Next(last.Step), !all
	}
	speaker := ""
	if database.AtNPC(st.Kind) {
		speaker = st.Target
	}
	q.tellLocked(c, sess, speaker, sayLines(q.froglokWords(p, qu.ID, st.Step, speaker, st.Says))...)
	who := fmt.Sprintf("character %d", c.serverID)
	if p != nil {
		who = p.Name
	}
	var lines []string
	switch {
	case stay:
		// anyOrderDone set the row; the journal's line says who is left
		if text := q.lineLocked(cq); text != oldLine || idx < 0 {
			q.dropLineLocked(sess, c, idx)
			cq.Slot = c.nextSlot()
			q.addLineLocked(sess, c, text)
		}
		log.Printf("Quests: %s did step %d of %q; %d of its group left", who, st.Step, qu.Name, len(openSteps(qu, cq)))
	case next == nil:
		cq.Step, cq.Count = 0, 0
		cq.Done++
		q.turnedLocked(c, qu) // a were line's third quest: its infection ends (wereinfect.go)
		if qu.Repeatable {
			c.forgetOffer(qu.ID) // a shop's window offers it again
		}
		q.dropLineLocked(sess, c, idx)
		lines = append(lines, questFinished)
		log.Printf("Quests: %s finished %q", who, qu.Name)
	default:
		old := q.lineLocked(cq)
		cq.Step, cq.Count = next.Step, 0
		if text := q.lineLocked(cq); text != old || idx < 0 {
			q.dropLineLocked(sess, c, idx)
			cq.Slot = c.nextSlot()
			q.addLineLocked(sess, c, text)
			// each step is its own journal line, both announced (the reference server's
			// Zole and Ronks, recorded)
			lines = append(lines, questFinished, questReceived)
		}
		log.Printf("Quests: %s did step %d of %q; step %d next", who, st.Step, qu.Name, next.Step)
	}
	q.saveLocked(cq)
	if given != nil {
		for _, g := range given.names {
			lines = append(lines, "You have given away "+g+".")
		}
	}
	// the step's rewards and its rewards for the player's class
	var rw database.ClassReward
	if p != nil {
		rw = st.RewardFor(p.Class)
	}
	if qu.ID == turkeyQuest && qu.Source == "thanksgiving-2026" && next == nil && given != nil {
		rw.Items = nil
		q.showTurkeyClub(sess, p)
		lines = append(lines, "You have received Turkey Smasher.")
	}
	if extra != nil {
		rw = rw.Plus(*extra)
	}
	if given != nil && given.ascended {
		// met with Ascended items: its reward gear Ascended (questascended.go)
		rw = q.ascendRewardLocked(rw, who)
		log.Printf("Quests: %s did step %d of %q with Ascended items", who, st.Step, qu.Name)
	}
	rw.XP = questXPShare(qu, rw.XP) // world.quest_xp_pct (questxp.go)
	lines = append(lines, q.rewardLocked(sess, p, rw, economy.QuestReward)...)
	if next == nil && !stay && sess != nil {
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
	// the level the quest was finished at, before its experience: its marks
	// are judged by it, so a reward that levels the player past the quest's
	// minlevel + 5 does not take them away (marks.go; Heirloom 2026-11-23: a
	// level 15 finishing a level 10 quest whose reward took it to 19 was paid
	// nothing)
	finishedAt := 0
	if p != nil {
		finishedAt = p.Level
	}
	if rw.XP > 0 && sess != nil && p != nil {
		q.gw.AwardXP(sess, p, rw.XP)
	}
	if next == nil && !stay && sess != nil && p != nil {
		// its Heirloom Marks, a first finish's (marks.go)
		heirloomMarks.Load().QuestDone(sess, p, qu, cq.Done == 1, finishedAt)
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
	if next != nil && !stay {
		spawnStep = next.Step
	}
	q.stepEventsLocked(sess, p, c, qu, st, database.EventDone, spawnStep)
	if stay {
		return
	}
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
func (q *QuestEngine) rewardLocked(sess *session.Session, p *entity.Player, rw database.ClassReward, src economy.Source) []string {
	if sess == nil || p == nil {
		return nil
	}
	var lines []string
	if rw.Tunar > 0 && q.inv != nil {
		p.Tunar += rw.Tunar
		economy.Made(src, p.CharServerID, p.Name, rw.Tunar)
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
	defer lockBook(p.CharServerID)() // booklock.go: one change to a book at a time
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

// heldItems is what the player has for a quest: its pack's records, the
// ones it wears as well as the ones it carries (never the bank), the carried
// ones first and each part in the client's list order. Every quest check that
// asks what a player has reads it: a show step (which keeps its items), a give
// step (which takes the carried copies first, and a worn one only when no
// carried one is left: takeLocked takes it off first), a give event's
// unless_held, the walls' items, a waysender's (Holds), the Plane of Sky's
// keys (HeldNamed, TakeNamed: the carried copy first). Until
// 2026-11-18 a give step counted only what was carried, and a player who put
// on the robe a quest asked for was told it lacked until they took it off (the
// owner: "for the level 1 quest where players get item from merchant if
// player equips it the quest giver doesnt acknowledge until they unequipped";
// Live, a player's "New Gear" four times on 2026-11-14 before the plain
// robe came off). A show step had counted worn ones since 2026-11-10.
func (q *QuestEngine) heldItems(p *entity.Player) []database.CharInventoryEntry {
	if q.inv == nil {
		return nil
	}
	ps := q.passOf(p)
	if ps != nil && ps.heldOK {
		return ps.held
	}
	rows, err := q.packRowsLocked(p)
	if err != nil {
		log.Printf("Quests: %s's inventory: %v", p.Name, err)
		return nil
	}
	if ps != nil && ps.kept != nil && ps.kept.heldOK { // made of the same kept read (questtakecache.go)
		ps.held, ps.heldOK = ps.kept.held, true
		return ps.held
	}
	var out, worn []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location != 0 || r.StackLeft <= 0 {
			continue
		}
		if r.EquipLoc >= 0 {
			worn = append(worn, r)
		} else {
			out = append(out, r)
		}
	}
	out = append(out, worn...)
	if ps != nil { // the same rows the rest of the pass (questpass.go)
		ps.held, ps.heldOK = out, true
		if ps.kept != nil {
			ps.kept.held, ps.kept.heldOK = out, true
		}
	}
	return out
}

// lackingLocked is what the player still lacks for a give step, or "": an
// item's Ascended record held counts for it (questascended.go).
func (q *QuestEngine) lackingLocked(p *entity.Player, st *database.QuestStep) string {
	ps := q.passOf(p)
	if ps != nil {
		if s, ok := ps.lack[st]; ok {
			return s // asked before in the pass (questpass.go)
		}
	}
	s := strings.Join(q.planHandInLocked(p, st).lack, ", ")
	if ps != nil {
		if ps.lack == nil {
			ps.lack = map[*database.QuestStep]string{}
		}
		ps.lack[st] = s
	}
	return s
}

// takeLocked takes a give step's items and tunar from the player, in the
// database, the session's list and the client's (0x003C, 0x0052), and returns
// what was handed in, by name, and whether it came Ascended (an item's
// Ascended record stands for it: questascended.go); false (and nothing
// taken) when something lacks or the database refused any of it: every row
// and the purse are written in one transaction (takeTxLocked).
func (q *QuestEngine) takeLocked(sess *session.Session, p *entity.Player, st *database.QuestStep) (*handedIn, bool) {
	given, err := q.takeTxLocked(sess, p, st, nil)
	return given, err == nil
}

// takeTxLocked is takeLocked writing progress too: cq, when given, is the
// quest's row as doing the step leaves it, written in the same transaction as
// the items and the purse (database.InventoryStore.HandIn), so a hand-in and
// its progress are kept together or not at all (board entry 1263). Nothing is
// told the client, and nothing changes in memory, until that commit.
func (q *QuestEngine) takeTxLocked(sess *session.Session, p *entity.Player, st *database.QuestStep, cq *database.CharQuest) (*handedIn, error) {
	plan := q.planHandInLocked(p, st)
	if len(plan.lack) > 0 || q.inv == nil {
		return nil, errors.New("lacks " + strings.Join(plan.lack, ", "))
	}
	// a worn copy (taken only when no carried one is left: heldItems' order)
	// comes off first, as Destroy takes one off: its slot emptied in the
	// database, UnequipItem to the client, the look and the hands again (a
	// refusal after leaves it carried, never lost)
	for _, t := range plan.takes {
		if t.row.EquipLoc >= 0 && !q.takeOffLocked(sess, p, t.row) {
			return nil, errors.New("a worn item could not be taken off")
		}
	}
	takes := make([]database.HandInTake, len(plan.takes))
	for i, t := range plan.takes {
		takes[i] = database.HandInTake{ItemID: t.row.ItemID, Left: t.row.StackLeft, N: t.n}
	}
	purse := -1
	if st.Tunar > 0 {
		purse = p.Tunar - st.Tunar
	}
	if err := q.inv.HandIn(p.CharServerID, takes, purse, cq, q.handInFault); err != nil {
		log.Printf("Quests: %s's hand-in at step %d refused whole: %v", p.Name, st.Step, err)
		return nil, err
	}
	l := sessionInvList(sess)
	given := &handedIn{ascended: plan.ascended}
	taken := map[int]int{} // by record, in the order first taken
	var order []int
	for _, t := range plan.takes {
		r := t.row
		if l != nil {
			if idx := l.indexOfItem(r.ItemID); idx >= 0 && l.took(idx, t.n) && sess != nil {
				queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, t.n))
			}
		}
		if taken[r.Pattern.PatternID] == 0 {
			order = append(order, r.Pattern.PatternID)
		}
		taken[r.Pattern.PatternID] += t.n
	}
	names := map[int]string{}
	for _, t := range plan.takes {
		names[t.row.Pattern.PatternID] = t.row.Pattern.ItemName
	}
	for _, id := range order {
		given.names = append(given.names, countOf(taken[id], names[id]))
	}
	if st.Tunar > 0 {
		// written with the rows: the purse in memory and the client's
		p.Tunar = purse
		economy.Spent(economy.QuestPayment, p.CharServerID, p.Name, st.Tunar)
		if sess != nil {
			queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(p.Tunar))
		}
		given.names = append(given.names, fmt.Sprintf("%d tunar", st.Tunar))
	}
	return given, nil
}

// takeOffLocked takes a worn row off before a give step takes it: the
// EquipHandler's TakeOff (takeoff.go: the slot written -1, UnequipItem, the
// worn gear and look again) when the engine has it and the player a session,
// else the slot written -1 and, with a session, UnequipItem. False (nothing
// taken) when the database refused.
func (q *QuestEngine) takeOffLocked(sess *session.Session, p *entity.Player, r database.CharInventoryEntry) bool {
	slot := r.EquipLoc
	if q.takeOff != nil && sess != nil {
		if !q.takeOff(sess, p, &r) {
			return false
		}
	} else {
		if err := q.inv.SetEquipLoc(p.CharServerID, r.ItemID, -1); err != nil {
			log.Printf("Quests: taking %s's item %d off slot %d: %v", p.Name, r.ItemID, slot, err)
			return false
		}
		if sess != nil {
			queueUnequip(sess, slot)
		}
	}
	log.Printf("Quests: %s's item %d (record %d) taken off slot %d to be handed in", p.Name, r.ItemID, r.Pattern.PatternID, slot)
	return true
}

// payStepTunarLocked takes a give or show step's tunar from the player (a
// show step's fare: the portal guardian's 5,000 for the first passage to
// Zaoran Plateau, questmasters1.com's "With the five Ruins and 5,000 tunar
// you can talk to portal NPC and be ported"), in the database and the
// client's purse, and names it among what was handed in. The caller has
// checked the player holds it (planHandInLocked).
func (q *QuestEngine) payStepTunarLocked(sess *session.Session, p *entity.Player, st *database.QuestStep, given *handedIn) {
	if st.Tunar <= 0 || q.inv == nil {
		return
	}
	p.Tunar -= st.Tunar
	economy.Spent(economy.QuestPayment, p.CharServerID, p.Name, st.Tunar)
	if err := q.inv.SetPurse(p.CharServerID, p.Tunar); err != nil {
		log.Printf("Quests: %s's tunar: %v", p.Name, err)
	}
	if sess != nil {
		queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(p.Tunar))
	}
	given.names = append(given.names, fmt.Sprintf("%d tunar", st.Tunar))
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
// the reference server's do ("Commander Nothard: ..."), unless the line names it already.
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
// name, as the reference server's do not) and opens the answers' box (0x0034).
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
