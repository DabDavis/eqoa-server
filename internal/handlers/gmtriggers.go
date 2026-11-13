package handlers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A GM's triggers: scripted events (the user: "lets build it as gm scripted
// events"). The GM's TEST menu has eight lines under Programming > TRIGGER,
// each sending 0x0702 {100, its index}; no record says what SOE's server did
// with one (claude-notes/gm-trigger-0702-20261028.md), so here a line fires
// the trigger of its slot (database/gmtriggers.go: the table, its steps and
// their bounds), as #trigger <n> does and the GM panel's Triggers page.
//
// A firing is a run: the trigger's steps in order at its place (its own, or
// where the GM stands), each that reaches players reaching those its scope
// takes there at that moment. A wait puts the rest off; the engine looks
// once a second (Tick) for a run whose wait is over, and for its spawns past
// their stay, which it takes away. A run ends when its steps are done and
// none of its spawns is alive, or when it is stopped: #trigger stop, the
// panel's Stop, a stop step of another trigger. A trigger fired while it runs
// runs again beside it.
//
// A trigger is also how the guides' raid encounters run (the user, 2026-11-07:
// the Plane of Sky's constructs and twins, Rallos Zek, the Unnamed Champion): a
// quest's trigger event (questevents.go) and a creature's death (table npcdeath)
// fire one by its name, at its own place or where the creature fell, and its steps
// wait on what the raid does: awaittalk for a player at an NPC (Talk, before the
// quests), awaitdead for its spawns, a clock for the whole of it, a place moved
// when the raid is moved, a lock so one raid at a time has the place. The firing
// player is not told the GMs' lines. A restart that keeps the sessions keeps the
// runs (gmtriggerkeep.go: each at its step, its lock held, its clock's time left,
// its spawns alive where they stood at their hit points, its waits watching them);
// a cold start ends them, as it ends a pet, and a quest waiting on an encounter's
// kill is put back for its characters (questencounter.go).
//
// Vanilla shows each line's name (not retail: protocol.OpcodeGMTriggers): a
// GM's client is offered the names with the GM channel, and one that answers
// 1 is sent the whole set (OpcodeGMTriggersData) then and whenever a trigger
// changes, starts or ends. One that answers 2 is sent every trigger in the
// table instead (OpcodeGMTriggerList, the user's "can we add the triggers to
// client menu also in gm menus", 2026-11-09), at the same moments, and its
// menu fires a line by its slot (OpcodeGMTriggerFire), so a slot past the
// eight lines fires from the client too. A PS2 ignores the offer and shows
// eight TRIGGERs; #trigger alone lists them.

// triggerBoxMark marks a trigger's popup (0x0034's u32, echoed by 0x0035), so
// its answer is told from a quest's (questBoxMark) and an NPC's (0).
const triggerBoxMark = 0x52000000

// triggerTick is how often a run's waits and its spawns' stays are looked at.
const triggerTick = time.Second

// triggerEngine is the running engine, for the GM panel and the GM channel.
var triggerEngine atomic.Pointer[TriggerEngine]

// Triggers is the running trigger engine, or nil.
func Triggers() *TriggerEngine { return triggerEngine.Load() }

// TriggerEngine runs the GMs' triggers.
type TriggerEngine struct {
	mu    sync.Mutex
	db    *database.SQLiteDB
	gw    *entity.World
	sm    *session.Manager
	inv   *database.InventoryStore
	items *database.ItemPatternStore

	// cat is the table as last read (Reload, each firing): the names
	cat    []database.GMTrigger
	runs   []*triggerRun
	runSeq int
	boxSeq uint32

	// named are the sessions whose clients asked for the names (0x7E07),
	// each with what it asked for and the last set it was sent
	named map[*session.Session]*triggerNamesAsk

	now  func() time.Time
	rand func() float64
	stop chan struct{}

	// deaths is table npcdeath; kills what the World heard, for the worker
	deaths map[string][]database.NPCDeathTrigger
	kills  chan entity.KillCredit
}

// triggerRun is one firing of a trigger.
type triggerRun struct {
	id      int
	trig    database.GMTrigger
	steps   []database.TriggerStep
	by      string // who fired it, as lines name them
	gmChar  uint32 // the firing GM's character (serverid); 0 none
	world   uint8
	x, y, z float32
	next    int       // the next step
	at      time.Time // when it goes on (a wait's end)
	start   time.Time
	spawns  []triggerSpawn
	stopped bool // ended by a stop: a tick in hand passes it by
	quiet   bool // fired by a quest or a death: no GM to tell its steps

	// lock is its trigger's (database.TriggerLockName); reach its own, once a
	// place step set one (0: the trigger's)
	lock  string
	reach float32
	// await is the step it waits on (awaittalk, awaitdead), until at when it
	// has a time (zero: none); deadline its clock's end (zero: none); died the
	// names of its spawns that died during this wait, and dead where its last did
	await    *database.TriggerStep
	deadline time.Time
	// emptyAfter is the clock's empty (0: none), lastNear when a player was last within
	// its reach
	emptyAfter time.Duration
	lastNear   time.Time
	died       []string
	dead       *triggerPlaceAt
	// resummons are its resummon steps from the step on: each keeps one of its spawns
	// called back while its caller lives (resummonLocked)
	resummons []*triggerResummon
	// protect is its protect step's NPC, guarded to its end (nil: none)
	protect *triggerProtect
}

// triggerProtect is the NPC a run guards: its death fails the run, text said to its
// players in a window (the file's protect step).
type triggerProtect struct {
	id   entity.EntityID
	name string
	text string
}

// triggerResummon is one resummon step a run keeps: the step, when its NPC's last
// spawn was first seen gone (zero: one stands, or none has come yet), and whether its
// caller has been seen living since the step (or since its spawns last died with it).
type triggerResummon struct {
	step       database.TriggerStep
	fell       time.Time
	callerSeen bool
}

// triggerPlaceAt is a place in a world.
type triggerPlaceAt struct {
	world   uint8
	x, y, z float32
}

// triggerSpawn is an NPC a run's spawn step placed.
type triggerSpawn struct {
	id    entity.EntityID
	name  string
	until time.Time
	pet   bool // marked its resummon's caller's pet (resummonLocked)
}

// TriggerFirer is who fires a trigger: a GM in the world (its session and
// player), or the GM panel (Name, with the GM's character when it is in the
// world).
type TriggerFirer struct {
	Name   string
	Sess   *session.Session
	Player *entity.Player
	// Quiet is a firing no GM made: nobody is told its steps' lines
	Quiet bool
	// At is the place of a trigger with none of its own, where no GM stands (a
	// death's)
	At *triggerPlaceAt
}

// TriggerRunInfo is a run as the GM panel lists it.
type TriggerRunInfo struct {
	Run     int     `json:"run"`
	Slot    int     `json:"slot"`
	Name    string  `json:"name"`
	By      string  `json:"by"`
	World   int     `json:"world"`
	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	Z       float32 `json:"z"`
	Started string  `json:"started"`
	Step    int     `json:"step"`  // the next step (1-based); past the last: all done
	Steps   int     `json:"steps"` // how many it has
	WaitFor float64 `json:"waitFor,omitempty"`
	Spawns  int     `json:"spawns"` // alive
}

// NewTriggerEngine makes the engine over db's gmtriggers. Start sets it
// running.
func NewTriggerEngine(db *database.SQLiteDB, gw *entity.World, sm *session.Manager, inv *database.InventoryStore) *TriggerEngine {
	e := &TriggerEngine{
		db:    db,
		gw:    gw,
		sm:    sm,
		inv:   inv,
		named: map[*session.Session]*triggerNamesAsk{},
		now:   time.Now,
		rand:  rand.Float64,
		stop:  make(chan struct{}),
		kills: make(chan entity.KillCredit, 256),
	}
	if db != nil {
		e.items = database.NewItemPatternStore(db)
	}
	if err := e.Reload(); err != nil {
		log.Printf("Triggers: %v", err)
	}
	return e
}

// Start looks at the runs once a second and makes this the running engine.
func (e *TriggerEngine) Start() {
	if e.gw != nil {
		e.gw.OnKill(e.heardKill)
	}
	go func() {
		t := time.NewTicker(triggerTick)
		defer t.Stop()
		for {
			select {
			case <-e.stop:
				return
			case k := <-e.kills:
				e.Killed(k)
			case <-t.C:
				e.Tick()
			}
		}
	}()
	triggerEngine.Store(e)
}

// Stop ends the worker (the runs are left as they are).
func (e *TriggerEngine) Stop() {
	select {
	case <-e.stop:
	default:
		close(e.stop)
	}
	triggerEngine.CompareAndSwap(e, nil)
}

// Reload reads the table again (a GM's change) and sends the names to the
// clients that asked for them.
func (e *TriggerEngine) Reload() error {
	if e == nil || e.db == nil {
		return nil
	}
	all, err := e.db.GMTriggers()
	if err != nil {
		return err
	}
	deaths, err := e.db.NPCDeathTriggers()
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cat = all
	e.deaths = deaths
	e.syncNamesLocked()
	return nil
}

// heardKill takes a kill from the World (its lock held: never wait here).
func (e *TriggerEngine) heardKill(k entity.KillCredit) {
	select {
	case e.kills <- k:
	default:
		log.Printf("Triggers: a kill of %q was not heard (the queue is full)", k.NPCName)
	}
}

// Killed fires what a creature's death fires (table npcdeath) where it fell.
func (e *TriggerEngine) Killed(k entity.KillCredit) {
	e.mu.Lock()
	rows := e.deaths[database.NPCClassKey(k.NPCName)]
	e.mu.Unlock()
	if len(rows) == 0 {
		return
	}
	world, x, y, z, ok := e.gw.NPCSpot(k.NPCID)
	if !ok {
		return
	}
	for _, r := range rows {
		if r.Chance < 100 && e.rand()*100 >= r.Chance {
			continue
		}
		if _, err := e.FireNamed(r.Trigger, TriggerFirer{Name: k.NPCName + "'s death", Quiet: true,
			At: &triggerPlaceAt{world: world, x: x, y: y, z: z}}); err != nil && !errors.Is(err, ErrTriggerBusy) {
			log.Printf("Triggers: %q's death fires %q: %v", k.NPCName, r.Trigger, err)
		}
	}
}

// bySlotNamed is the slot of the trigger called name (any case), the first by
// slot, false for none.
func (e *TriggerEngine) slotNamed(name string) (int, bool) {
	all, err := e.db.GMTriggers()
	if err != nil {
		log.Printf("Triggers: %v", err)
		return 0, false
	}
	name = strings.TrimSpace(name)
	for _, t := range all {
		if strings.EqualFold(strings.TrimSpace(t.Name), name) {
			return t.Slot, true
		}
	}
	return 0, false
}

// FireNamed is Fire for the trigger called name (a quest's event, npcdeath's rows).
func (e *TriggerEngine) FireNamed(name string, by TriggerFirer) ([]string, error) {
	if e == nil || e.db == nil || e.gw == nil {
		return nil, ErrTriggerNone
	}
	slot, ok := e.slotNamed(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTriggerNone, name)
	}
	return e.Fire(slot, by)
}

// BusyNamed reports the trigger called name kept from firing by its lock: a run
// of the same lock lives. What is handed in for a firing asks first
// (questevents.go).
func (e *TriggerEngine) BusyNamed(name string) bool {
	if e == nil || e.db == nil {
		return false
	}
	slot, ok := e.slotNamed(name)
	if !ok {
		return false
	}
	t, ok, err := e.db.GMTriggerBySlot(slot)
	if err != nil || !ok {
		return false
	}
	steps, err := database.ParseTriggerSteps(t.Steps)
	if err != nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lockHeldLocked(database.TriggerLockName(steps))
}

// RunningNamed reports a run of the trigger called name (any case) lives, one
// stopped not counted: a quest waiting on its encounter's kill asks
// (questencounter.go).
func (e *TriggerEngine) RunningNamed(name string) bool {
	if e == nil {
		return false
	}
	name = strings.TrimSpace(name)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.runs {
		if !r.stopped && strings.EqualFold(strings.TrimSpace(r.trig.Name), name) {
			return true
		}
	}
	return false
}

// SpawnsNPC reports the trigger called name (as last read) spawns npc (any case):
// a spawn or resummon step of it. What a quest's kill step waits on
// (questencounter.go).
func (e *TriggerEngine) SpawnsNPC(name, npc string) bool {
	if e == nil {
		return false
	}
	name, npc = strings.TrimSpace(name), strings.TrimSpace(npc)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.cat {
		if !strings.EqualFold(strings.TrimSpace(t.Name), name) {
			continue
		}
		steps, err := database.ParseTriggerSteps(t.Steps)
		if err != nil {
			continue
		}
		for _, s := range steps {
			if (s.Do == database.TriggerSpawn || s.Do == database.TriggerResummon) && strings.EqualFold(strings.TrimSpace(s.NPC), npc) {
				return true
			}
		}
	}
	return false
}

// lockHeldLocked is whether a run of that lock lives ("": no lock, never). Must
// be called with e.mu held.
func (e *TriggerEngine) lockHeldLocked(lock string) bool {
	if lock == "" {
		return false
	}
	for _, r := range e.runs {
		if r.lock == lock {
			return true
		}
	}
	return false
}

// List is every trigger as last read.
func (e *TriggerEngine) List() []database.GMTrigger {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]database.GMTrigger(nil), e.cat...)
}

// Errors Fire answers with, for the GM panel to tell apart.
var (
	ErrTriggerNone    = errors.New("no such trigger")
	ErrTriggerOff     = errors.New("the trigger is off")
	ErrTriggerBroken  = errors.New("the trigger cannot run as written")
	ErrTriggerNoPlace = errors.New("the trigger has no place")
	ErrTriggerBusy    = errors.New("a run of the trigger's lock lives")
)

// Fire runs the trigger in slot as by: its steps until a wait (Tick goes on
// from there). It answers the lines for the GM: what was done, or why
// nothing was (the error then).
func (e *TriggerEngine) Fire(slot int, by TriggerFirer) ([]string, error) {
	if e == nil || e.db == nil || e.gw == nil {
		return []string{"The triggers are not running on this server"}, ErrTriggerNone
	}
	t, ok, err := e.db.GMTriggerBySlot(slot)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []string{fmt.Sprintf("Trigger %d: none set (the GM panel's Triggers page sets one; #trigger lists them)", slot)}, ErrTriggerNone
	}
	if !t.Enabled {
		return []string{t.Title() + " is off (the GM panel's Triggers page turns it on)"}, ErrTriggerOff
	}
	if probs := database.TriggerProblems(t); len(probs) > 0 {
		return []string{fmt.Sprintf("%s cannot run: %s", t.Title(), probs[0])}, ErrTriggerBroken
	}
	steps, _ := database.ParseTriggerSteps(t.Steps)
	r := &triggerRun{trig: t, steps: steps, by: strings.TrimSpace(by.Name), quiet: by.Quiet, lock: database.TriggerLockName(steps)}
	if by.Player != nil {
		r.gmChar = by.Player.CharServerID
		if r.by == "" {
			r.by = by.Player.Name
		}
	}
	if t.Fixed() {
		r.world, r.x, r.y, r.z = uint8(t.World), t.X, t.Y, t.Z
	} else if by.At != nil {
		r.world, r.x, r.y, r.z = by.At.world, by.At.x, by.At.y, by.At.z
	} else {
		world, x, y, z, _, ok := e.gw.PlayerSpot(by.Player)
		if by.Player == nil || !ok {
			return []string{t.Title() + " happens where the GM stands, and you are not in the world: give it a place of its own, or fire it in the game"}, ErrTriggerNoPlace
		}
		r.world, r.x, r.y, r.z = world, x, y, z
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lockHeldLocked(r.lock) {
		return []string{fmt.Sprintf("%s waits: a run of its lock %q lives", t.Title(), r.lock)}, ErrTriggerBusy
	}
	if all, err := e.db.GMTriggers(); err == nil {
		e.cat = all
	}
	e.runSeq++
	r.id = e.runSeq
	r.start = e.now()
	r.at = r.start
	e.runs = append(e.runs, r)
	log.Printf("Triggers: %s fired by %s at (%.1f, %.1f, %.1f) world=%d (run %d)", t.Title(), r.by, r.x, r.y, r.z, r.world, r.id)
	lines := []string{fmt.Sprintf("%s fired at (%.0f, %.0f, %.0f) in %s", t.Title(), r.x, r.y, r.z, worldName(r.world))}
	lines = append(lines, e.advanceLocked(r)...)
	e.syncNamesLocked()
	return lines, nil
}

// StopRuns ends the runs of the trigger in slot (0: every run): their steps
// left undone, their spawns alive taken away. It answers how many runs and
// spawns that was.
func (e *TriggerEngine) StopRuns(slot int) (runs, spawns int) {
	if e == nil {
		return 0, 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	runs, spawns = e.stopLocked(func(r *triggerRun) bool { return slot == 0 || r.trig.Slot == slot })
	e.syncNamesLocked()
	return runs, spawns
}

// stopLocked ends the runs which takes. Must be called with e.mu held.
func (e *TriggerEngine) stopLocked(which func(*triggerRun) bool) (runs, spawns int) {
	keep := e.runs[:0]
	for _, r := range e.runs {
		if !which(r) {
			keep = append(keep, r)
			continue
		}
		runs++
		r.stopped = true
		spawns += e.despawnLocked(r, "")
		log.Printf("Triggers: %s's run %d stopped at step %d of %d", r.trig.Title(), r.id, r.next+1, len(r.steps))
	}
	for i := len(keep); i < len(e.runs); i++ {
		e.runs[i] = nil
	}
	e.runs = keep
	return runs, spawns
}

// Runs is every run, oldest first.
func (e *TriggerEngine) Runs() []TriggerRunInfo {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	out := make([]TriggerRunInfo, 0, len(e.runs))
	for _, r := range e.runs {
		info := TriggerRunInfo{Run: r.id, Slot: r.trig.Slot, Name: r.trig.Name, By: r.by, World: int(r.world),
			X: r.x, Y: r.y, Z: r.z, Started: r.start.UTC().Format(time.RFC3339), Step: r.next + 1, Steps: len(r.steps)}
		if r.next < len(r.steps) && r.at.After(now) {
			info.WaitFor = math.Round(r.at.Sub(now).Seconds()*10) / 10
		}
		for _, s := range r.spawns {
			if e.gw.NPCAlive(s.id) {
				info.Spawns++
			}
		}
		out = append(out, info)
	}
	return out
}

// Running reports which slots have a run.
func (e *TriggerEngine) Running() map[int]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runningLocked()
}

func (e *TriggerEngine) runningLocked() map[int]bool {
	on := map[int]bool{}
	for _, r := range e.runs {
		on[r.trig.Slot] = true
	}
	return on
}

// Tick goes on with the runs whose wait is over, takes away the spawns past
// their stay, ends the runs that are done, and sends the names where they
// changed (once a second).
func (e *TriggerEngine) Tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	// its spawns first: a wait on one's death goes on in the same second
	for _, r := range e.runs {
		alive := r.spawns[:0]
		for _, s := range r.spawns {
			switch {
			case !e.gw.NPCAlive(s.id):
				// dead (its corpse goes as a GM's spawn's does) or gone
				r.died = append(r.died, s.name)
				if world, x, y, z, ok := e.gw.NPCSpot(s.id); ok {
					r.dead = &triggerPlaceAt{world: world, x: x, y: y, z: z}
				}
			case now.After(s.until):
				e.gw.DespawnNPC(s.id)
				log.Printf("Triggers: %s's %s (%d) stayed its time: taken away", r.trig.Title(), s.name, s.id)
			default:
				alive = append(alive, s)
			}
		}
		r.spawns = alive
	}
	for _, r := range append([]*triggerRun(nil), e.runs...) {
		if r.stopped {
			continue
		}
		if r.protect != nil && !e.gw.NPCAlive(r.protect.id) {
			e.protectFailedLocked(r)
			continue
		}
		e.resummonLocked(r, now)
		if !r.deadline.IsZero() && (!now.Before(r.deadline) || e.emptyLocked(r, now)) {
			e.clockEndLocked(r)
		}
		if r.await != nil {
			if !e.awaitOverLocked(r, now) {
				continue
			}
			// over, by its death (a talk) or its seconds: its seconds were the
			// longest it waits, not the least (the user, 2026-11-08: "the
			// triggers dont reset when the NPC dies": a death within an
			// awaitdead's 3600 s held the run, and its lock, the whole hour)
			r.at = time.Time{}
		}
		if r.next < len(r.steps) && !now.Before(r.at) {
			r.await = nil
			lines := e.advanceLocked(r)
			e.tellGMLocked(r, lines)
		}
	}
	keep := e.runs[:0]
	for _, r := range e.runs {
		// done: its steps, its spawns and a last wait's time (a trigger's last
		// wait is its rest, its lock held through it: the throne of
		// Bertoxxulous's 3 days 16 hours, the Bloody Portal's shut time; until
		// 2026-11-12 the run ended as that wait began, which made it nothing)
		if r.next >= len(r.steps) && len(r.spawns) == 0 && (r.await != nil || !now.Before(r.at)) {
			log.Printf("Triggers: %s's run %d ended", r.trig.Title(), r.id)
			continue
		}
		keep = append(keep, r)
	}
	for i := len(keep); i < len(e.runs); i++ {
		e.runs[i] = nil
	}
	e.runs = keep
	e.syncNamesLocked()
}

// tellGMLocked says a run's lines to the GM who fired it, when it is in the
// world.
func (e *TriggerEngine) tellGMLocked(r *triggerRun, lines []string) {
	if r.gmChar == 0 || len(lines) == 0 || r.quiet {
		return
	}
	sess := SessionOfCharacter(e.gw, e.sm, r.gmChar)
	if sess == nil {
		return
	}
	for _, l := range lines {
		SendClientMessage(sess, "[GM] "+r.trig.Title()+": "+l)
	}
}

// advanceLocked runs a run's steps from its next until a wait or the end, and
// answers a line for each. Must be called with e.mu held.
func (e *TriggerEngine) advanceLocked(r *triggerRun) []string {
	var lines []string
	for r.next < len(r.steps) {
		s := r.steps[r.next]
		r.next++
		if s.Do == database.TriggerWait {
			d := s.Seconds
			if s.Upto > s.Seconds { // a random time from seconds to upto
				d += e.rand() * (s.Upto - s.Seconds)
			}
			r.at = e.now().Add(time.Duration(d * float64(time.Second)))
			if r.next < len(r.steps) {
				lines = append(lines, fmt.Sprintf("waiting %s", seconds(math.Round(d))))
			}
			return lines
		}
		if s.Do == database.TriggerAwaitTalk || s.Do == database.TriggerAwaitDead {
			step := s
			r.await, r.died, r.at = &step, nil, time.Time{}
			if s.Seconds > 0 {
				r.at = e.now().Add(time.Duration(s.Seconds * float64(time.Second)))
			}
			return lines
		}
		if l := e.stepLocked(r, s); l != "" {
			lines = append(lines, l)
			log.Printf("Triggers: %s step %d: %s", r.trig.Title(), r.next, l)
		}
	}
	return lines
}

// seconds is a wait as a GM reads it.
func seconds(s float64) string {
	if s >= 60 && math.Mod(s, 60) == 0 {
		return fmt.Sprintf("%.0f min", s/60)
	}
	return strconv.FormatFloat(s, 'f', -1, 64) + " s"
}

// triggerMember is a player a step reaches.
type triggerMember struct {
	sess *session.Session
	p    *entity.Player
	dead bool
}

// audience is the players the run's scope takes now, by name: the firing GM
// (gm), those within its reach of its place in its world (near), everyone in
// its world (world) or in the game (all).
func (e *TriggerEngine) audience(r *triggerRun) []triggerMember {
	if e.sm == nil || e.gw == nil {
		return nil
	}
	var sessions []*session.Session
	e.sm.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() || sess.PendingTermination() {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced {
			return true
		}
		sessions = append(sessions, sess)
		return true
	})
	reach := r.trig.Reach()
	if r.reach > 0 {
		reach = r.reach
	}
	var out []triggerMember
	for _, sess := range sessions {
		p := e.gw.SessionPlayer(sess)
		world, x, _, z, dead, ok := e.gw.PlayerSpot(p)
		if !ok {
			continue
		}
		switch r.trig.Scope {
		case database.TriggerScopeGM:
			if r.gmChar == 0 || p.CharServerID != r.gmChar {
				continue
			}
		case database.TriggerScopeNear:
			if world != r.world || math.Hypot(float64(x-r.x), float64(z-r.z)) > float64(reach) {
				continue
			}
		case database.TriggerScopeWorld:
			if world != r.world {
				continue
			}
		}
		out = append(out, triggerMember{sess: sess, p: p, dead: dead})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].p.Name < out[j].p.Name })
	return out
}

// players is "n players" as a line says it.
func players(n int) string {
	if n == 1 {
		return "1 player"
	}
	return fmt.Sprintf("%d players", n)
}

// stepLocked does one step that is not a wait and answers a line saying what
// it did. Must be called with e.mu held.
func (e *TriggerEngine) stepLocked(r *triggerRun, s database.TriggerStep) string {
	switch s.Do {
	case database.TriggerBroadcast:
		who := e.audience(r)
		for _, m := range who {
			GMLine(m.sess, triggerText(s.Text, m.p.Name), gmBroadcastRGB)
		}
		return "said to " + players(len(who))
	case database.TriggerPopup:
		who := e.audience(r)
		for _, m := range who {
			e.popupLocked(m.sess, triggerText(s.Text, m.p.Name), s.Answer)
		}
		return "a window for " + players(len(who))
	case database.TriggerSay:
		who := e.audience(r)
		for _, m := range who {
			sayBoxes(m.sess, s.NPC, triggerText(s.Text, m.p.Name))
		}
		speaker := strings.TrimSpace(s.NPC)
		if speaker == "" {
			speaker = "words"
		}
		return fmt.Sprintf("%s to %s", speaker, players(len(who)))
	case database.TriggerGive:
		return e.giveLocked(r, s)
	case database.TriggerVisual:
		fx, _ := database.ParseVisualID(s.FX)
		n := 0
		for _, m := range e.audience(r) {
			if !m.dead && e.gw.PlayVisualOn(m.sess, fx, time.Duration(s.Seconds*float64(time.Second))) {
				n++
			}
		}
		return fmt.Sprintf("visual 0x%08X on %s", fx, players(n))
	case database.TriggerTeleport:
		return e.teleportLocked(r, s)
	case database.TriggerSpawn:
		return e.spawnLocked(r, s)
	case database.TriggerDespawn:
		n := e.despawnLocked(r, s.NPC)
		if npc := strings.TrimSpace(s.NPC); npc != "" {
			return fmt.Sprintf("took away %d of its %s", n, npc)
		}
		return fmt.Sprintf("took away %d of its spawns", n)
	case database.TriggerStop:
		runs, spawns := e.stopLocked(func(o *triggerRun) bool {
			return o != r && (s.Slot == 0 || o.trig.Slot == s.Slot)
		})
		if s.Slot == 0 {
			return fmt.Sprintf("stopped %d other runs (%d spawns taken away)", runs, spawns)
		}
		return fmt.Sprintf("stopped %d runs of trigger %d (%d spawns taken away)", runs, s.Slot, spawns)
	case database.TriggerPlace:
		if s.Dead && r.dead != nil {
			r.world, r.x, r.y, r.z = r.dead.world, r.dead.x, r.dead.y, r.dead.z
		} else if s.World != nil {
			r.world, r.x, r.y, r.z = uint8(*s.World), s.X, s.Y, s.Z
		}
		if s.Radius > 0 {
			r.reach = s.Radius
		}
		return fmt.Sprintf("its place now (%.0f, %.0f) in %s", r.x, r.z, worldName(r.world))
	case database.TriggerClock:
		r.deadline = e.now().Add(time.Duration(s.Seconds * float64(time.Second)))
		r.emptyAfter = time.Duration(s.Empty * float64(time.Second))
		r.lastNear = e.now()
		return "its clock set: " + seconds(s.Seconds)
	case database.TriggerEnd:
		r.deadline = time.Time{}
		return ""
	case database.TriggerWalk:
		world := r.world
		if s.World != nil {
			world = uint8(*s.World)
		}
		return fmt.Sprintf("%d of %s set walking", e.gw.WalkNPCsTo(strings.TrimSpace(s.NPC), r.world, r.x, r.z, e.reachOf(r, s),
			world, s.X, s.Y, s.Z), s.NPC)
	case database.TriggerHome:
		return fmt.Sprintf("%d of %s sent home", e.gw.SendNPCsHome(strings.TrimSpace(s.NPC), r.world, r.x, r.z, e.reachOf(r, s)), s.NPC)
	case database.TriggerRouse:
		n := 0
		for _, sp := range r.spawns {
			if strings.EqualFold(sp.name, strings.TrimSpace(s.NPC)) && e.gw.RouseNPC(sp.id) {
				n++
			}
		}
		return fmt.Sprintf("%d of %s roused", n, s.NPC)
	case database.TriggerResummon:
		r.resummons = append(r.resummons, &triggerResummon{step: s})
		return fmt.Sprintf("%s calls %s back %s after it falls", strings.TrimSpace(s.By), strings.TrimSpace(s.NPC), seconds(s.Seconds))
	case database.TriggerSic:
		n, on := 0, ""
		for _, sp := range r.spawns {
			if strings.TrimSpace(s.NPC) != "" && !strings.EqualFold(sp.name, strings.TrimSpace(s.NPC)) {
				continue
			}
			if name, ok := e.gw.SetNPCOn(sp.id, s.Target, e.reachOf(r, s), s.Casts); ok {
				n, on = n+1, name
			}
		}
		who := strings.TrimSpace(s.NPC)
		if who == "" {
			who = "its spawns"
		}
		if n == 0 {
			return fmt.Sprintf("none of %s has a %s within reach", who, strings.TrimSpace(s.Target))
		}
		return fmt.Sprintf("%d of %s set on %s", n, who, on)
	case database.TriggerProtect:
		id, ok := e.gw.NearestNPC(s.NPC, r.world, r.x, r.z, e.reachOf(r, s))
		if !ok {
			return fmt.Sprintf("no %s within reach to protect", strings.TrimSpace(s.NPC))
		}
		r.protect = &triggerProtect{id: id, name: strings.TrimSpace(s.NPC), text: s.Text}
		return fmt.Sprintf("%s (%d) protected to the run's end", strings.TrimSpace(s.NPC), id)
	case database.TriggerQuestStep:
		qe := Quests()
		if qe == nil {
			return "no quests on this server"
		}
		var members []heldMember
		for _, m := range e.audience(r) {
			members = append(members, heldMember{sess: m.sess, p: m.p})
		}
		title, quest, step := r.trig.Title(), s.Quest, s.QuestStep
		// on its own goroutine: the quest engine's lock comes before this one's
		// (a step's events fire triggers), never after
		go func() {
			n := qe.DoHeldStep(members, quest, step)
			log.Printf("Triggers: %s: step %d of quest %d done for %s", title, step, quest, players(n))
		}()
		return fmt.Sprintf("step %d of quest %d for those of %s on it", step, quest, players(len(members)))
	case database.TriggerHide:
		until := time.Time{}
		if s.Minutes > 0 {
			until = e.now().Add(time.Duration(s.Minutes * float64(time.Minute)))
		}
		return fmt.Sprintf("%d of %s out of view for %s", e.gw.HideNPCs(strings.TrimSpace(s.NPC), r.world, r.x, r.z, e.reachOf(r, s), until), s.NPC, seconds(s.Minutes*60))
	}
	return ""
}

// protectFailedLocked is a run's protected NPC dead or gone: its text in a window for
// its players, its spawns taken away, its steps left undone (and with them its
// queststep: no one's items are taken). Must be called with e.mu held.
func (e *TriggerEngine) protectFailedLocked(r *triggerRun) {
	who := e.audience(r)
	if t := strings.TrimSpace(r.protect.text); t != "" {
		for _, m := range who {
			e.popupLocked(m.sess, triggerText(t, m.p.Name), "")
		}
	}
	log.Printf("Triggers: %s's run %d failed: %s (%d) fell (%s told)", r.trig.Title(), r.id, r.protect.name, r.protect.id, players(len(who)))
	e.stopLocked(func(o *triggerRun) bool { return o == r })
}

// reachOf is how far from the run's place a step takes its NPCs: its radius, else
// the run's reach. Must be called with e.mu held.
func (e *TriggerEngine) reachOf(r *triggerRun, s database.TriggerStep) float32 {
	switch {
	case s.Radius > 0:
		return s.Radius
	case r.reach > 0:
		return r.reach
	}
	return r.trig.Reach()
}

// emptyLocked is whether a run's place has stood empty as long as its clock allows
// (a raid wiped, or gone): the clock's end at once, so the place's lock does not
// hold for the rest of its time. Must be called with e.mu held.
func (e *TriggerEngine) emptyLocked(r *triggerRun, now time.Time) bool {
	if r.emptyAfter <= 0 {
		return false
	}
	reach := r.trig.Reach()
	if r.reach > 0 {
		reach = r.reach
	}
	if e.gw.PlayerNear(r.world, r.x, r.z, reach) {
		r.lastNear = now
		return false
	}
	return now.Sub(r.lastNear) >= r.emptyAfter
}

// clockEndLocked is a run's clock run out: its spawns alive taken away, and it goes
// on from its end step (past all of them when it has none). Must be called with
// e.mu held.
func (e *TriggerEngine) clockEndLocked(r *triggerRun) {
	r.deadline, r.await, r.at = time.Time{}, nil, time.Time{}
	e.despawnLocked(r, "")
	next := len(r.steps)
	for i := r.next; i < len(r.steps); i++ {
		if r.steps[i].Do == database.TriggerEnd {
			next = i
			break
		}
	}
	r.next = next
	log.Printf("Triggers: %s's run %d: its clock ran out, on from step %d of %d", r.trig.Title(), r.id, next+1, len(r.steps))
}

// awaitOverLocked is whether what a run waits on is over: its time up, or for
// awaitdead its spawns' deaths (those of its npc; any: one, else all of them gone).
// A talk ends its wait in Talk. Must be called with e.mu held.
func (e *TriggerEngine) awaitOverLocked(r *triggerRun, now time.Time) bool {
	s := r.await
	if s == nil {
		return true
	}
	if !r.at.IsZero() && !now.Before(r.at) {
		return true
	}
	if s.Do != database.TriggerAwaitDead {
		return false
	}
	names := func(n string) bool {
		return strings.TrimSpace(s.NPC) == "" || strings.EqualFold(n, strings.TrimSpace(s.NPC))
	}
	if s.Any {
		for _, n := range r.died {
			if names(n) {
				return true
			}
		}
		return false
	}
	for _, sp := range r.spawns {
		if names(sp.name) {
			return false
		}
	}
	return true
}

// Talk is HandleInteract's question before the quests: a run waiting for a player
// to talk to this NPC goes on, the step's words said to the talker; a class the
// step does not take is told its refusal. It reports whether a run answered.
func (e *TriggerEngine) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if e == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.runs {
		s := r.await
		if r.stopped || s == nil || s.Do != database.TriggerAwaitTalk || !strings.EqualFold(strings.TrimSpace(s.NPC), strings.TrimSpace(npc.Name)) || npc.World != r.world {
			continue
		}
		// one of the name elsewhere in the world (the main isle's Astral Light) is not this one
		if reach := e.reachOf(r, *s); math.Hypot(float64(npc.X-r.x), float64(npc.Z-r.z)) > float64(reach) {
			continue
		}
		if len(s.Classes) > 0 && !containsInt(s.Classes, p.Class) {
			sayBoxes(sess, npc.Name, orText(triggerText(s.Refuse, p.Name), "Not you."))
			return true
		}
		if t := strings.TrimSpace(s.Text); t != "" {
			sayBoxes(sess, npc.Name, triggerText(t, p.Name))
		}
		log.Printf("Triggers: %s talked to %s: %s's run %d goes on", p.Name, npc.Name, r.trig.Title(), r.id)
		r.await, r.at = nil, time.Time{}
		e.tellGMLocked(r, e.advanceLocked(r))
		e.syncNamesLocked()
		return true
	}
	return false
}

// triggerText is a step's words for a player: %p its name.
func triggerText(text, name string) string {
	return strings.ReplaceAll(text, "%p", name)
}

// sayBoxes says each line of text in a box of its own (0x0046), the
// speaker's name before it unless the line names it already, as a quest's
// NPC speaks (QuestEngine.tellLocked).
func sayBoxes(sess *session.Session, speaker, text string) {
	speaker = strings.TrimSpace(speaker)
	for _, l := range sayLines(text) {
		if speaker != "" && !strings.HasPrefix(strings.ToLower(l), strings.ToLower(speaker)+":") {
			l = speaker + ": " + l
		}
		for _, part := range splitSay(l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
}

// popupLocked opens a window the player closes, as a quest's popup event
// does (questevents.go): the text's lines in 0x0046 boxes, its last part the
// question of a 0x0034 with one answer, under triggerBoxMark so the answer
// changes nothing (HandleDialogueOption). Must be called with e.mu held.
func (e *TriggerEngine) popupLocked(sess *session.Session, text, answer string) {
	var parts []string
	for _, l := range sayLines(text) {
		parts = append(parts, splitSay(l, questAskChars)...)
	}
	if len(parts) == 0 {
		return
	}
	for _, p := range parts[:len(parts)-1] {
		SendDialogueBox(sess, 0, p)
	}
	e.boxSeq++
	id := uint32(triggerBoxMark | (e.boxSeq & 0xFFFFFF))
	SendQuestPopupMulti(sess, id, parts[len(parts)-1], []string{clipRunes(orText(answer, questPopupAnswer), questAnswerChars)})
}

// IsTriggerBox reports a 0x0035's id as a trigger popup's.
func IsTriggerBox(id uint32) bool { return id&0xFF000000 == triggerBoxMark }

// giveLocked gives the step's items to each player its scope takes. Must be
// called with e.mu held.
func (e *TriggerEngine) giveLocked(r *triggerRun, s database.TriggerStep) string {
	if e.items == nil || e.inv == nil {
		return "no items given: this server keeps none"
	}
	type pat struct {
		p database.ItemPatternRecord
		n int
	}
	var pats []pat
	for _, it := range database.ParseItemCounts(s.Items) {
		p, err := e.items.GetItemPattern(it.PatternID)
		if err != nil || p == nil {
			log.Printf("Triggers: %s: item %d: %v", r.trig.Title(), it.PatternID, err)
			continue
		}
		pats = append(pats, pat{*p, it.Count})
	}
	if len(pats) == 0 {
		return "no items given: none of them is on this server"
	}
	who := e.audience(r)
	got := 0
	for _, m := range who {
		any := false
		for _, it := range pats {
			n, err := GiveItemNow(m.sess, e.gw, e.inv, it.p, it.n)
			if err != nil {
				log.Printf("Triggers: giving %s %q: %v", m.p.Name, it.p.ItemName, err)
			}
			if n > 0 {
				any = true
				SendClientMessage(m.sess, "You have received "+countOf(n, it.p.ItemName)+".")
			}
			if n < it.n {
				SendClientMessage(m.sess, fmt.Sprintf("Your inventory is full: %s could not be given.", countOf(it.n-n, it.p.ItemName)))
			}
		}
		if any {
			got++
		}
	}
	names := make([]string, len(pats))
	for i, it := range pats {
		names[i] = countOf(it.n, it.p.ItemName)
	}
	return fmt.Sprintf("gave %s to %s", strings.Join(names, ", "), players(got))
}

// teleportLocked moves each living player the scope takes to the step's
// place, as a travel NPC's warp does (QuestEngine.teleportLocked); a world a
// player's client cannot load is refused for that player. Must be called with
// e.mu held.
func (e *TriggerEngine) teleportLocked(r *triggerRun, s database.TriggerStep) string {
	if s.World == nil {
		return ""
	}
	world := byte(*s.World)
	if _, err := ValidateTeleport(s.X, s.Y, s.Z, world); err != nil {
		return "teleport refused: " + err.Error()
	}
	who := e.audience(r)
	if s.Max > 0 {
		// the firer first, then the nearest; each with a pet counts it too
		// ("it only ports 16 ppl and pets count as 1")
		dist := func(m triggerMember) float64 {
			_, x, _, z, _, _ := e.gw.PlayerSpot(m.p)
			if r.gmChar != 0 && m.p.CharServerID == r.gmChar {
				return -1
			}
			return math.Hypot(float64(x-r.x), float64(z-r.z))
		}
		sort.SliceStable(who, func(i, j int) bool { return dist(who[i]) < dist(who[j]) })
	}
	n, refused, room := 0, 0, s.Max
	for _, m := range who {
		if m.dead {
			continue
		}
		if !CanLoadWorld(m.sess, world) {
			refused++
			continue
		}
		if s.Max > 0 {
			take := teleportRoomTaken(e.gw.PetOf(m.p) != 0)
			if take > room {
				if t := strings.TrimSpace(s.Refuse); t != "" {
					SendClientMessage(m.sess, triggerText(t, m.p.Name))
				}
				continue
			}
			room -= take
		}
		facing, counter, changed, ok := e.gw.BeginWarp(m.p.ServerID, world)
		if !ok {
			continue
		}
		sendTeleportWarp(m.sess, world, s.X, s.Y, s.Z, facing, counter)
		beginWorldLoad(m.sess, changed)
		e.gw.UpdatePosition(m.p.ServerID, s.X, s.Y, s.Z, facing)
		n++
	}
	line := fmt.Sprintf("moved %s to (%.0f, %.0f, %.0f) in %s", players(n), s.X, s.Y, s.Z, worldName(world))
	if refused > 0 {
		line += fmt.Sprintf(" (%s whose game cannot load it stayed)", players(refused))
	}
	return line
}

// teleportRoomTaken is how much of a capped teleport one player takes: a pet is one more.
func teleportRoomTaken(hasPet bool) int {
	if hasPet {
		return 2
	}
	return 1
}

// spawnLocked places the step's copies round its place (its own, or the
// run's): spread apart on a ring, each facing the middle, for their stay.
// Must be called with e.mu held.
func (e *TriggerEngine) spawnLocked(r *triggerRun, s database.TriggerStep) string {
	world, cx, cy, cz := r.world, r.x, r.y, r.z
	if s.World != nil {
		world, cx, cy, cz = uint8(*s.World), s.X, s.Y, s.Z
	}
	n := max(s.Count, 1)
	if s.Max > 0 {
		n = min(n, s.Max-e.gw.CountNear(strings.TrimSpace(s.NPC), world, cx, cz, 300))
		if n <= 0 {
			return fmt.Sprintf("%s: %d stand near already", s.NPC, s.Max)
		}
	}
	spread := s.Spread
	if spread <= 0 {
		spread = database.TriggerDefaultSpread
	}
	stay := s.Minutes
	if stay <= 0 {
		stay = database.TriggerDefaultStay
	}
	until := e.now().Add(time.Duration(stay * float64(time.Minute)))
	name := strings.TrimSpace(s.NPC)
	placed, spelled := 0, name
	turn := e.rand() * 2 * math.Pi
	for i := 0; i < n; i++ {
		// evenly round the ring, each a little off its share, between
		// two fifths of the spread and the whole of it from the middle
		a := turn + (float64(i)+0.6*(e.rand()-0.5))*2*math.Pi/float64(n)
		d := float64(spread) * (0.4 + 0.6*e.rand())
		x := cx + float32(d*math.Sin(a))
		z := cz + float32(d*math.Cos(a))
		id, sp, ok := e.gw.SpawnEventNPC(name, world, cx, cy, cz, x, z, s.Hostile)
		if !ok {
			return fmt.Sprintf("no NPC named %q to spawn", name)
		}
		if s.Attackable {
			e.gw.MakeAttackable(id)
		}
		spelled = sp
		r.spawns = append(r.spawns, triggerSpawn{id: id, name: sp, until: until})
		placed++
	}
	how := ""
	if s.Hostile {
		how = " hostile"
	}
	return fmt.Sprintf("spawned %d%s %s within %.0f of (%.0f, %.0f) for %s", placed, how, spelled, spread, cx, cz, seconds(stay*60))
}

// resummonLocked calls back each of the run's resummoned NPCs that has no spawn
// standing (the user, 2026-11-09: "cause tunare to resummon her pets"): while one of
// the run's spawns named by lives, the step's seconds after the NPC's last spawn was
// first seen gone, one more beside the caller (within 3), with the step's attackable,
// hostile and minutes, set on its target with its radius and casts, its text said to
// the run's players. The caller dead or gone, nothing comes (and the wait starts
// again when one does), and the NPC's spawns standing die with it (the owner,
// 2026-11-10: "when the summoned pet's master dies, so does the pet"; entity's
// KillWithMaster: their deaths played, corpses of nothing). Must be called with e.mu
// held.
func (e *TriggerEngine) resummonLocked(r *triggerRun, now time.Time) {
	for _, rs := range r.resummons {
		name, by := strings.TrimSpace(rs.step.NPC), strings.TrimSpace(rs.step.By)
		var caller entity.EntityID
		standing := false
		for i, sp := range r.spawns {
			switch {
			case strings.EqualFold(sp.name, name):
				standing = true
				if !sp.pet { // its caller's pet: no loot, whichever dies first (ticket 2008; entity's MarkMasterPet)
					e.gw.MarkMasterPet(sp.id)
					r.spawns[i].pet = true
				}
			case caller == 0 && strings.EqualFold(sp.name, by) && e.gw.NPCAlive(sp.id):
				caller = sp.id
			}
		}
		if caller != 0 {
			rs.callerSeen = true
		} else if rs.callerSeen {
			// its caller is dead (or gone): its pets die with it
			rs.callerSeen = false
			for _, sp := range r.spawns {
				if strings.EqualFold(sp.name, name) {
					if n, ok := e.gw.KillWithMaster(sp.id); ok {
						log.Printf("Triggers: %s's %s (%d) dies with its %s", r.trig.Title(), n, sp.id, by)
					}
				}
			}
		}
		if standing || caller == 0 {
			rs.fell = time.Time{}
			continue
		}
		if rs.fell.IsZero() {
			rs.fell = now
		}
		if now.Sub(rs.fell) < time.Duration(rs.step.Seconds*float64(time.Second)) {
			continue
		}
		world, x, y, z, ok := e.gw.NPCSpot(caller)
		if !ok {
			continue
		}
		w := int(world)
		line := e.spawnLocked(r, database.TriggerStep{Do: database.TriggerSpawn, NPC: name, Spread: 3, World: &w, X: x, Y: y, Z: z,
			Minutes: rs.step.Minutes, Hostile: rs.step.Hostile, Attackable: rs.step.Attackable})
		if strings.TrimSpace(rs.step.Target) != "" {
			line += "; " + e.stepLocked(r, database.TriggerStep{Do: database.TriggerSic, NPC: name, Target: rs.step.Target,
				Radius: rs.step.Radius, Casts: rs.step.Casts})
		}
		if strings.TrimSpace(rs.step.Text) != "" {
			e.stepLocked(r, database.TriggerStep{Do: database.TriggerBroadcast, Text: rs.step.Text})
		}
		rs.fell = time.Time{}
		log.Printf("Triggers: %s's %s calls %s back: %s", r.trig.Title(), by, name, line)
	}
}

// despawnLocked takes away the run's live spawns (those named npc; none:
// all) and answers how many. Must be called with e.mu held.
func (e *TriggerEngine) despawnLocked(r *triggerRun, npc string) int {
	npc = strings.TrimSpace(npc)
	n := 0
	keep := r.spawns[:0]
	for _, s := range r.spawns {
		if npc != "" && !strings.EqualFold(s.name, npc) {
			keep = append(keep, s)
			continue
		}
		if e.gw.NPCAlive(s.id) {
			e.gw.DespawnNPC(s.id)
			n++
		}
	}
	r.spawns = keep
	return n
}

// --- the names, for vanilla's TEST menu (protocol.OpcodeGMTriggers) ---

// buildTriggerNamesOffer is OpcodeGMTriggers from the server: a u32, 1.
func buildTriggerNamesOffer() []byte {
	return build(protocol.OpcodeGMTriggers, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// encodeTriggerNames is OpcodeGMTriggersData's body: a u8 count, then each
// menu slot's trigger: a u8 index (the slot less one), u8 flags (bit 0 on,
// bit 1 running) and its name (a u8 byte count, UTF-8, MenuName's at most
// 32 bytes). Slots past the menu's eight are left out.
func encodeTriggerNames(cat []database.GMTrigger, running map[int]bool) []byte {
	var body []byte
	n := 0
	for _, t := range cat {
		if t.Slot < 1 || t.Slot > database.TriggerMenuSlots {
			continue
		}
		var flags byte
		if t.Enabled {
			flags |= 1
		}
		if running[t.Slot] {
			flags |= 2
		}
		name := t.MenuName()
		body = append(body, byte(t.Slot-1), flags, byte(len(name)))
		body = append(body, name...)
		n++
	}
	return append([]byte{byte(n)}, body...)
}

// buildTriggerNames is op (OpcodeGMTriggersData, OpcodeGMTriggerList) with
// body.
func buildTriggerNames(op protocol.GameOpcode, body []byte) []byte {
	return build(op, 2+len(body), func(w *buffer.Writer) {
		w.WriteBytes(body)
	})
}

// encodeTriggerList is OpcodeGMTriggerList's body: a u16 count, then every
// trigger in cat's order (by slot): a u32 slot, u8 flags (bit 0 on, bit 1
// running) and its name (a u8 byte count, UTF-8, ListName's at most 32
// bytes, none for a trigger with no name), little-endian. A slot a u32
// cannot carry is left out (no client could fire it).
func encodeTriggerList(cat []database.GMTrigger, running map[int]bool) []byte {
	body := make([]byte, 2, 2+len(cat)*16)
	n := 0
	for _, t := range cat {
		if t.Slot < 1 || uint64(t.Slot) > math.MaxUint32 || n == math.MaxUint16 {
			continue
		}
		var flags byte
		if t.Enabled {
			flags |= 1
		}
		if running[t.Slot] {
			flags |= 2
		}
		name := t.ListName()
		body = binary.LittleEndian.AppendUint32(body, uint32(t.Slot))
		body = append(body, flags, byte(len(name)))
		body = append(body, name...)
		n++
	}
	binary.LittleEndian.PutUint16(body, uint16(n))
	return body
}

// What a client's answer to the names' offer asks for (a u32 after
// protocol.OpcodeGMTriggers).
const (
	TriggerNamesOff  = 0 // nothing more
	TriggerNamesMenu = 1 // the TEST menu's eight lines' names (OpcodeGMTriggersData)
	TriggerNamesAll  = 2 // every trigger, by slot (OpcodeGMTriggerList)
)

// triggerNamesAsk is a session's ask for the names: which (all: the whole
// list), and the body it was sent last ("": none yet).
type triggerNamesAsk struct {
	all  bool
	last string
}

// SetNames makes a session's client have the names (TriggerNamesMenu), every
// trigger (TriggerNamesAll) or stop (TriggerNamesOff); another ask is the
// menu's names. Asked, they go out at once.
func (e *TriggerEngine) SetNames(sess *session.Session, ask uint32) {
	if e == nil || sess == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ask == TriggerNamesOff {
		delete(e.named, sess)
		return
	}
	e.named[sess] = &triggerNamesAsk{all: ask == TriggerNamesAll}
	e.syncNamesLocked()
}

// syncNamesLocked sends each client that asked what it asked for (the
// menu's names or every trigger), where it differs from what it was sent
// last, and forgets the sessions gone. Must be called with e.mu held.
func (e *TriggerEngine) syncNamesLocked() {
	if len(e.named) == 0 {
		return
	}
	running := e.runningLocked()
	var menu, all []byte // each made once, when a session asked for it
	for sess, ask := range e.named {
		if !sess.CharacterInWorld() || sess.PendingTermination() {
			delete(e.named, sess)
			continue
		}
		var op protocol.GameOpcode
		var body []byte
		if ask.all {
			if all == nil {
				all = encodeTriggerList(e.cat, running)
			}
			op, body = protocol.OpcodeGMTriggerList, all
		} else {
			if menu == nil {
				menu = encodeTriggerNames(e.cat, running)
			}
			op, body = protocol.OpcodeGMTriggersData, menu
		}
		if ask.last == string(body) {
			continue
		}
		ask.last = string(body)
		queueReliable(sess, op, buildTriggerNames(op, body))
	}
}

// --- the GM's side: 0x0702, #trigger, the names' answer ---

// fireTrigger fires slot for the GM gm (whose session is sess) and says what
// happened.
func (h *ChatHandler) fireTrigger(sess *session.Session, gm *entity.Player, slot int) {
	e := Triggers()
	if e == nil {
		h.gmReply(sess, fmt.Sprintf("Trigger %d: the triggers are not running on this server", slot))
		return
	}
	lines, _ := e.Fire(slot, TriggerFirer{Name: gm.Name, Sess: sess, Player: gm})
	for _, l := range lines {
		h.gmReply(sess, l)
	}
}

// triggerCommand is #trigger: alone, every trigger and what runs; <n> or a
// trigger's name fires it; stop [n] ends its runs (none: every run).
func (h *ChatHandler) triggerCommand(sess *session.Session, args []string) {
	e := Triggers()
	if e == nil {
		h.gmReply(sess, "The triggers are not running on this server")
		return
	}
	gm := h.gameWorld.SessionPlayer(sess)
	if gm == nil {
		return
	}
	if len(args) == 0 {
		h.listTriggers(sess, e)
		return
	}
	if strings.EqualFold(args[0], "stop") {
		slot := 0
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				h.gmReply(sess, "#trigger stop [n]: a trigger's number, or none for every run")
				return
			}
			slot = n
		}
		runs, spawns := e.StopRuns(slot)
		if slot == 0 {
			h.gmReply(sess, fmt.Sprintf("Stopped %d runs, %d spawns taken away", runs, spawns))
		} else {
			h.gmReply(sess, fmt.Sprintf("Trigger %d: stopped %d runs, %d spawns taken away", slot, runs, spawns))
		}
		log.Printf("GM: %s stopped trigger %d's runs (%d)", gm.Name, slot, runs)
		return
	}
	if n, err := strconv.Atoi(args[0]); err == nil {
		h.fireTrigger(sess, gm, n)
		return
	}
	// a name: the trigger of that name, else the ones it begins
	name := strings.Join(args, " ")
	var exact, begun []database.GMTrigger
	for _, t := range e.List() {
		n := strings.TrimSpace(t.Name)
		switch {
		case n == "":
		case strings.EqualFold(n, name):
			exact = append(exact, t)
		case strings.HasPrefix(strings.ToLower(n), strings.ToLower(name)):
			begun = append(begun, t)
		}
	}
	match := exact
	if len(match) == 0 {
		match = begun
	}
	switch len(match) {
	case 0:
		h.gmReply(sess, fmt.Sprintf("No trigger named %q (#trigger lists them)", name))
	case 1:
		h.fireTrigger(sess, gm, match[0].Slot)
	default:
		h.gmReply(sess, fmt.Sprintf("%q is more than one trigger's name:", name))
		for _, t := range match {
			h.gmReply(sess, fmt.Sprintf("  %d %s", t.Slot, t.Name))
		}
	}
}

// listTriggers is #trigger alone: each trigger, its line in the TEST menu,
// whether it is on, its scope and place, and whether it runs.
func (h *ChatHandler) listTriggers(sess *session.Session, e *TriggerEngine) {
	all := e.List()
	if len(all) == 0 {
		h.gmReply(sess, "No triggers: the GM panel's Triggers page makes them")
		return
	}
	running := e.Running()
	h.gmReply(sess, "Triggers (#trigger <n> fires one; TEST > Programming > TRIGGER, top line 1):")
	for _, t := range all {
		state := "on"
		if !t.Enabled {
			state = "off"
		}
		where := "where you stand"
		if t.Fixed() {
			where = fmt.Sprintf("at (%.0f, %.0f) in %s", t.X, t.Z, worldName(uint8(t.World)))
		}
		scope := t.Scope
		if t.Scope == database.TriggerScopeNear {
			scope = fmt.Sprintf("near (%.0f)", t.Reach())
		}
		line := fmt.Sprintf("  %d %s: %s, %s, %s", t.Slot, orText(t.Name, "(no name)"), state, scope, where)
		if running[t.Slot] {
			line += ", RUNNING"
		}
		h.gmReply(sess, line)
	}
}

// HandleGMTriggerNames is a client's OpcodeGMTriggers: whether it shows the
// triggers' names (a u32: 0 no, 2 every trigger, any other the menu's eight
// lines'). A GM's alone is taken; nothing is answered.
func (h *ChatHandler) HandleGMTriggerNames(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("Triggers: a short names answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if !h.isGM(sess) {
		if !h.isSubGM(sess) { // a sub GM's answer to its offer (SendGMChannel), the names a GM's
			log.Printf("Triggers: account %d asked for the names, not a GM: dropped", sess.AccountID())
		}
		return nil
	}
	Triggers().SetNames(sess, binary.LittleEndian.Uint32(data[:4]))
	return nil
}

// HandleGMTriggerFire is a client's OpcodeGMTriggerFire: a GM's firing of the
// trigger in a slot (a u32) from a line of the whole list (vanilla's TEST
// menu), fired as 0x0702's line is; anyone else's is logged and dropped.
func (h *ChatHandler) HandleGMTriggerFire(sess *session.Session, msg *protocol.Message) error {
	d := msg.Data()
	if len(d) < 4 {
		log.Printf("GM channel: a short trigger firing (%d bytes) from %s", len(d), sess.RemoteAddr())
		return nil
	}
	slot := binary.LittleEndian.Uint32(d[:4])
	if !h.isGM(sess) {
		log.Printf("GM channel: trigger %d fired by account %d, not a GM: dropped", slot, sess.AccountID())
		return nil
	}
	if h.gameWorld == nil {
		return nil
	}
	gm := h.gameWorld.SessionPlayer(sess)
	if gm == nil {
		return nil
	}
	log.Printf("GM channel: %s: TRIGGER slot %d", gm.Name, slot)
	h.fireTrigger(sess, gm, int(slot))
	return nil
}
