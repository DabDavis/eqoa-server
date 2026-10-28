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
// Vanilla shows each line's name (not retail: protocol.OpcodeGMTriggers): a
// GM's client is offered the names with the GM channel, and one that answers
// 1 is sent the whole set (OpcodeGMTriggersData) then and whenever a trigger
// changes, starts or ends. A PS2 ignores the offer and shows eight TRIGGERs;
// #trigger alone lists them.

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
	// each with the last set it was sent
	named map[*session.Session]string

	now  func() time.Time
	rand func() float64
	stop chan struct{}
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
}

// triggerSpawn is an NPC a run's spawn step placed.
type triggerSpawn struct {
	id    entity.EntityID
	name  string
	until time.Time
}

// TriggerFirer is who fires a trigger: a GM in the world (its session and
// player), or the GM panel (Name, with the GM's character when it is in the
// world).
type TriggerFirer struct {
	Name   string
	Sess   *session.Session
	Player *entity.Player
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
		named: map[*session.Session]string{},
		now:   time.Now,
		rand:  rand.Float64,
		stop:  make(chan struct{}),
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
	go func() {
		t := time.NewTicker(triggerTick)
		defer t.Stop()
		for {
			select {
			case <-e.stop:
				return
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cat = all
	e.syncNamesLocked()
	return nil
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
	r := &triggerRun{trig: t, steps: steps, by: strings.TrimSpace(by.Name)}
	if by.Player != nil {
		r.gmChar = by.Player.CharServerID
		if r.by == "" {
			r.by = by.Player.Name
		}
	}
	if t.Fixed() {
		r.world, r.x, r.y, r.z = uint8(t.World), t.X, t.Y, t.Z
	} else {
		world, x, y, z, _, ok := e.gw.PlayerSpot(by.Player)
		if by.Player == nil || !ok {
			return []string{t.Title() + " happens where the GM stands, and you are not in the world: give it a place of its own, or fire it in the game"}, ErrTriggerNoPlace
		}
		r.world, r.x, r.y, r.z = world, x, y, z
	}

	e.mu.Lock()
	defer e.mu.Unlock()
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
	for _, r := range append([]*triggerRun(nil), e.runs...) {
		if !r.stopped && r.next < len(r.steps) && !now.Before(r.at) {
			lines := e.advanceLocked(r)
			e.tellGMLocked(r, lines)
		}
	}
	keep := e.runs[:0]
	for _, r := range e.runs {
		alive := r.spawns[:0]
		for _, s := range r.spawns {
			switch {
			case !e.gw.NPCAlive(s.id):
				// dead (its corpse goes as a GM's spawn's does) or gone
			case now.After(s.until):
				e.gw.DespawnNPC(s.id)
				log.Printf("Triggers: %s's %s (%d) stayed its time: taken away", r.trig.Title(), s.name, s.id)
			default:
				alive = append(alive, s)
			}
		}
		r.spawns = alive
		if r.next >= len(r.steps) && len(r.spawns) == 0 {
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
	if r.gmChar == 0 || len(lines) == 0 {
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
			r.at = e.now().Add(time.Duration(s.Seconds * float64(time.Second)))
			if r.next < len(r.steps) {
				lines = append(lines, fmt.Sprintf("waiting %s", seconds(s.Seconds)))
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
	}
	return ""
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
	n, refused := 0, 0
	for _, m := range e.audience(r) {
		if m.dead {
			continue
		}
		if !CanLoadWorld(m.sess, world) {
			refused++
			continue
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

// spawnLocked places the step's copies round its place (its own, or the
// run's): spread apart on a ring, each facing the middle, for their stay.
// Must be called with e.mu held.
func (e *TriggerEngine) spawnLocked(r *triggerRun, s database.TriggerStep) string {
	world, cx, cy, cz := r.world, r.x, r.y, r.z
	if s.World != nil {
		world, cx, cy, cz = uint8(*s.World), s.X, s.Y, s.Z
	}
	n := max(s.Count, 1)
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

// buildTriggerNames is OpcodeGMTriggersData with body.
func buildTriggerNames(body []byte) []byte {
	return build(protocol.OpcodeGMTriggersData, 2+len(body), func(w *buffer.Writer) {
		w.WriteBytes(body)
	})
}

// SetNames makes a session's client have the names (on) or stop; on, they go
// out at once.
func (e *TriggerEngine) SetNames(sess *session.Session, on bool) {
	if e == nil || sess == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !on {
		delete(e.named, sess)
		return
	}
	e.named[sess] = ""
	e.syncNamesLocked()
}

// syncNamesLocked sends the names to each client that asked whose last set
// differs, and forgets the sessions gone. Must be called with e.mu held.
func (e *TriggerEngine) syncNamesLocked() {
	if len(e.named) == 0 {
		return
	}
	body := encodeTriggerNames(e.cat, e.runningLocked())
	for sess, last := range e.named {
		if !sess.CharacterInWorld() || sess.PendingTermination() {
			delete(e.named, sess)
			continue
		}
		if last == string(body) {
			continue
		}
		e.named[sess] = string(body)
		queueReliable(sess, protocol.OpcodeGMTriggersData, buildTriggerNames(body))
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
// triggers' names (a u32, nonzero yes). A GM's alone is taken; nothing is
// answered.
func (h *ChatHandler) HandleGMTriggerNames(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("Triggers: a short names answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if !h.isGM(sess) {
		log.Printf("Triggers: account %d asked for the names, not a GM: dropped", sess.AccountID())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	Triggers().SetNames(sess, on)
	return nil
}
