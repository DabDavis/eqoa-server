package handlers

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// The guides' raid encounters on the triggers (gmtriggers.go): a lock, a talk and a
// death waited on, a clock, a place moved, a cap on a teleport, a death that fires one
// (npcdeath), and a quest's event that fires one and hands nothing in while it cannot.

// kill has a run's spawn die, as the world would have it.
func (w *trigWorld) kill(n *entity.NPC) {
	n.Dead = true
	n.HP = 0
}

func TestTriggerLockKeepsOneRun(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Isle one", "near", 50, `[{"do":"lock","lock":"isle"},{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	w.set(21, "Isle two", "near", 50, `[{"do":"lock","lock":"Isle "},{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	if w.e.BusyNamed("isle two") {
		t.Fatal("busy before anything ran")
	}
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	if !w.e.BusyNamed("isle two") {
		t.Fatal("the lock is held: the second must say it is busy")
	}
	if _, err := w.e.FireNamed("Isle Two", TriggerFirer{Name: "x", Player: w.gp}); !errors.Is(err, ErrTriggerBusy) {
		t.Fatalf("fired while its lock is held: %v", err)
	}
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns, want the first run's one", n)
	}
	w.kill(w.spawned()[0])
	w.at(time.Second)
	if w.e.BusyNamed("isle two") {
		t.Fatal("the run ended with its spawn: the lock is free")
	}
	if _, err := w.e.FireNamed("isle two", TriggerFirer{Name: "x", Player: w.gp}); err != nil {
		t.Fatal(err)
	}
}

// The control of the lock: a trigger with none runs beside itself, as ever.
func TestTriggerWithNoLockRunsBesideItself(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Free", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	for i := 0; i < 2; i++ {
		if _, err := w.fire(20); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(w.spawned()); n != 2 {
		t.Fatalf("%d spawns, want 2", n)
	}
}

func TestTriggerAwaitsATalkByClass(t *testing.T) {
	w := newTrigWorld(t)
	w.setAt(20, "Graves", "near", 50, 0, 110, 50, 110, `[{"do":"awaittalk","npc":"Orc Pawn","classes":[0,2,3],"text":"The tank is heard.","refuse":"Only a tank."},
		{"do":"spawn","npc":"Orc Pawn","minutes":10}]`, true)
	if _, err := w.e.Fire(20, TriggerFirer{Name: "quest", Player: w.gp, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("%d spawns before the talk", n)
	}
	w.clear()
	npc := w.gw.NPCs[700]
	// one of the name out of the place's reach is another NPC
	far := *npc
	far.X = 9000
	if w.e.Talk(w.gs, w.gp, &far) {
		t.Fatal("a run answered for an NPC of its name far from its place")
	}
	w.gp.Class = 9
	if !w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("the run did not answer another class")
	}
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("a cleric went on: %d spawns", n)
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "Only a tank.") {
		t.Fatalf("no refusal: %s", got)
	}
	w.gp.Class = 3
	if !w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("the run did not answer its class")
	}
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns after the talk, want 1", n)
	}
	if got := strings.Join(questMsgs(w.gs), "|"); !strings.Contains(got, "The tank is heard.") || strings.Contains(got, "[GM]") {
		t.Fatalf("the talker's words, no GM lines to a quiet run's firer: %s", got)
	}
	// the wait is over: the NPC has nothing more to say to this run
	if w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("a run past its talk answered")
	}
	// another NPC's name, or another world's, is not it
	w.setAt(21, "Other", "near", 50, 0, 110, 50, 110, `[{"do":"awaittalk","npc":"Somebody"}]`, true)
	w.e.Fire(21, TriggerFirer{Name: "quest", Player: w.gp, Quiet: true})
	if w.e.Talk(w.gs, w.gp, npc) {
		t.Fatal("a wait on another NPC answered")
	}
}

func TestTriggerAwaitsDeathsAndMovesToTheLast(t *testing.T) {
	w := newTrigWorld(t)
	w.setAt(20, "Forms", "all", 50, 0, 25000, 50, 15000, `[{"do":"spawn","npc":"Orc Pawn","count":2,"minutes":60},
		{"do":"awaitdead","npc":"Orc Pawn"},{"do":"place","dead":true},{"do":"spawn","npc":"Orc Pawn","spread":1,"minutes":60},
		{"do":"awaitdead","any":true,"seconds":300},{"do":"broadcast","text":"after"}]`, true)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	first := w.spawned()
	if len(first) != 2 {
		t.Fatalf("%d spawns, want 2", len(first))
	}
	w.kill(first[0])
	w.at(time.Second)
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("one of two dead went on: %d alive", n)
	}
	first[1].X, first[1].Z = 25222, 15111
	w.kill(first[1])
	w.at(2 * time.Second)
	next := w.spawned()
	if len(next) != 1 {
		t.Fatalf("%d spawns of the next form, want 1", len(next))
	}
	if math.Hypot(float64(next[0].X-25222), float64(next[0].Z-15111)) > 2 {
		t.Fatalf("the next form stands at (%.0f, %.0f), not where the last fell", next[0].X, next[0].Z)
	}
	w.clear()
	w.at(3 * time.Second)
	if len(yellow(t, w.gs)) != 0 {
		t.Fatal("went on before its death or its time")
	}
	w.at(304 * time.Second) // its time up with the form alive
	if lines := yellow(t, w.gs); len(lines) == 0 {
		t.Fatal("a wait's time up did not go on")
	}
}

func TestTriggerClockEndsAtItsEndStep(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Isle", "near", 50, `[{"do":"clock","seconds":60},{"do":"spawn","npc":"Orc Pawn","minutes":60},
		{"do":"awaittalk","npc":"Somebody"},{"do":"spawn","npc":"Orc Pawn","count":5},{"do":"end"},{"do":"broadcast","text":"Time."}]`)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns, want 1", n)
	}
	w.clear()
	w.at(59 * time.Second)
	if len(yellow(t, w.gs)) != 0 {
		t.Fatal("before its time")
	}
	w.at(61 * time.Second)
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("%d spawns past the clock, want none (and the five skipped)", n)
	}
	if lines := yellow(t, w.gs); len(lines) == 0 || !strings.Contains(strings.Join(lines, "|"), "Time.") {
		t.Fatalf("the end step's line: %v", lines)
	}
	w.at(62 * time.Second)
	if w.e.Running()[20] {
		t.Fatal("the run ended")
	}
}

func TestTriggerTeleportTakesAtMostItsNumber(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Port", "near", 50, `[{"do":"teleport","world":0,"x":24000,"y":50,"z":14000,"max":1,"refuse":"The light takes no more."}]`)
	gx, _, gz := w.gp.GetPosition()
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	x, _, _ := w.gp.GetPosition()
	if x != 24000 {
		t.Fatalf("the firer stands at %v (from %v, %v), want moved first", x, gx, gz)
	}
	other := w.gw.SessionPlayer(w.ps)
	if ox, _, _ := other.GetPosition(); ox == 24000 {
		t.Fatal("the second went past the cap of 1")
	}
	if got := strings.Join(questMsgs(w.ps), "|"); !strings.Contains(got, "The light takes no more.") {
		t.Fatalf("the one left is told: %s", got)
	}
}

func TestACreaturesDeathFiresWhereItFell(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Captain", "near", 50, `[{"do":"lock","lock":"captain"},{"do":"spawn","npc":"Orc Pawn","spread":1,"minutes":10}]`)
	if _, err := w.db.Exec(`INSERT INTO npcdeath (name, trigger, chance) VALUES ('orc pawn', 'captain', 30)`); err != nil {
		t.Fatal(err)
	}
	if err := w.e.Reload(); err != nil {
		t.Fatal(err)
	}
	pawn := w.gw.NPCs[700]
	pawn.X, pawn.Z = 300, 400
	k := entity.KillCredit{NPCName: "orc pawn", NPCID: 700}
	w.e.rand = func() float64 { return 0.5 } // 50 of 100: past a chance of 30
	w.e.Killed(k)
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("%d spawns on a failed roll", n)
	}
	w.e.rand = func() float64 { return 0.1 }
	w.e.Killed(k)
	s := w.spawned()
	if len(s) != 1 || math.Hypot(float64(s[0].X-300), float64(s[0].Z-400)) > 2 {
		t.Fatalf("the death's spawn: %+v", s)
	}
	w.e.Killed(k) // its lock held: not a second
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns: a second came while the first lives", n)
	}
	// another creature's death fires nothing
	w.e.Killed(entity.KillCredit{NPCName: "a bat", NPCID: 700})
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("%d spawns after another's death", n)
	}
}

func TestTriggerWalksAndSendsHome(t *testing.T) {
	w := newTrigWorld(t)
	w.setAt(20, "Fuse", "near", 300, 0, 100, 50, 100, `[{"do":"walk","npc":"Orc Pawn","x":200,"y":50,"z":200,"radius":50},{"do":"wait","seconds":60},
		{"do":"home","npc":"Orc Pawn","radius":500},{"do":"hide","npc":"Orc Pawn","radius":500,"minutes":1}]`, true)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.gw.NPCPathOf(700); !ok {
		t.Fatal("the walk gave it no path")
	}
	w.at(61 * time.Second)
	if _, ok := w.gw.NPCPathOf(700); ok {
		t.Fatal("home left its path on")
	}
	if !w.gw.NPCs[700].HiddenUntil.After(w.now.Add(30 * time.Second)) {
		t.Fatal("hide did not hide it")
	}
}

// lightQuests are two repeatable hand-ins at one light, each firing its trigger, done
// on the answer that takes it (dostep).
func lightQuests(f *questFixture, sun, rain int) *TriggerEngine {
	f.t.Helper()
	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	triggerEngine.Store(e)
	f.t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	f.exec(`INSERT INTO gmtriggers (slot, name, world, x, z, steps) VALUES (20, 'Isle', -1, 0, 0, '[{"do":"lock","lock":"isle"},{"do":"wait","seconds":60}]')`)
	for i, c := range []struct {
		id   int
		name string
		item int
	}{{70, "The sun's shards", sun}, {71, "The rain's shards", rain}} {
		_ = i
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable, offer, accept, decline, declined)
			VALUES (?, ?, 1, 'Light', 1, 'The light hums.', 'Feed it.', 'Not now.', 'It waits.')`, c.id, c.name)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, reminder, says, events)
			VALUES (?, 1, 'give', 'Light', ?, 'Feed the light.', 'It wants shards.', 'It takes them.',
			'[{"on":"start","do":"dostep"},{"on":"done","do":"trigger","trigger":"isle","text":"Others are on the isle."}]')`, c.id, fmt.Sprintf("%d*2", c.item))
	}
	f.qe.Reload()
	return e
}

// A hand-in that fires a trigger is done on the answer; a second, its lock held, takes
// nothing; a refused quest stands aside for the giver's next, and comes round again.
func TestQuestTriggerEventAndItsLock(t *testing.T) {
	f := newQuestFixture(t)
	sun, rain := f.pattern("sun shard", 20), f.pattern("rain shard", 20)
	lightQuests(f, sun, rain)
	f.own(sun, 2)
	f.own(rain, 2)
	light := &entity.NPC{Name: "Light", ServerID: 900}
	talk := func() []string {
		t.Helper()
		if !f.qe.Talk(f.tw.s, f.tw.p, light) {
			t.Fatal("the light said nothing")
		}
		return f.msgs()
	}
	m := talk()
	if !strings.Contains(strings.Join(m, "|"), "Feed it.") {
		t.Fatalf("no offer: %v", m)
	}
	// refused: the next talk offers the rain's, not the sun's again
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 1)
	f.msgs()
	m = talk()
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 0) // the rain's
	m = f.msgs()
	if f.held(rain) != 0 || f.held(sun) != 2 {
		t.Fatalf("the rain's taken at once: rain %d sun %d (%v)", f.held(rain), f.held(sun), m)
	}
	if !strings.Contains(strings.Join(m, "|"), "You have finished a quest!") {
		t.Fatalf("not finished on the answer: %v", m)
	}
	// the lock is held: the sun's hand-in takes nothing and says why
	m = talk()
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 0)
	m = f.msgs()
	if f.held(sun) != 2 {
		t.Fatalf("shards taken for a firing that could not happen: %d left (%v)", f.held(sun), m)
	}
	if !strings.Contains(strings.Join(m, "|"), "Others are on the isle.") {
		t.Fatalf("no word on why: %v", m)
	}
}

// A hand-in done on the answer is offered only to one who holds it: no quest in the journal
// it cannot do, and the giver goes on to what else it has to say.
func TestQuestDoStepOffersOnlyWhatIsHeld(t *testing.T) {
	f := newQuestFixture(t)
	sun := f.pattern("sun shard", 20)
	rain := f.pattern("rain shard", 20)
	lightQuests(f, sun, rain)
	light := &entity.NPC{Name: "Light", ServerID: 900}
	if f.qe.Talk(f.tw.s, f.tw.p, light) {
		t.Fatalf("offered with no shards: %v", f.msgs())
	}
	f.own(rain, 2)
	if !f.qe.Talk(f.tw.s, f.tw.p, light) {
		t.Fatal("no offer with the rain's shards held")
	}
	if m := strings.Join(f.msgs(), "|"); !strings.Contains(m, "Feed it.") {
		t.Fatalf("the offer: %s", m)
	}
}

// A pet takes a place in a capped teleport ("it only ports 16 ppl and pets count as 1").
func TestTeleportCapCountsAPet(t *testing.T) {
	if teleportRoomTaken(false) != 1 || teleportRoomTaken(true) != 2 {
		t.Fatalf("alone %d, with a pet %d", teleportRoomTaken(false), teleportRoomTaken(true))
	}
}

// A spawn may be open to attack and still leave people alone until it is roused (a pet
// let off by its trainer's death). The control: a plain spawn keeps its row's NPC bit.
func TestTriggerAttackableSpawnAndItsRousing(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Pet", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":10},
		{"do":"wait","seconds":30},{"do":"rouse","npc":"Orc Pawn"},{"do":"wait","seconds":30}]`)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	s := w.spawned()
	if len(s) != 1 {
		t.Fatalf("%d spawns, want 1", len(s))
	}
	if s[0].NPCType&0x80 != 0 || s[0].EventAggro {
		t.Fatalf("attackable: NPC type %#x, attacks on sight %v; want no NPC bit, leaving people alone", s[0].NPCType, s[0].EventAggro)
	}
	w.at(31 * time.Second)
	if !s[0].EventAggro || s[0].NPCType&0x80 != 0 {
		t.Fatalf("roused: NPC type %#x, attacks on sight %v", s[0].NPCType, s[0].EventAggro)
	}

	w.set(21, "Plain", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","minutes":10}]`)
	before := map[entity.EntityID]bool{s[0].ServerID: true}
	if _, err := w.fire(21); err != nil {
		t.Fatal(err)
	}
	for _, n := range w.spawned() {
		if !before[n.ServerID] && (n.NPCType&0x80 == 0 || n.EventAggro) {
			t.Fatalf("a plain spawn: NPC type %#x, attacks on sight %v", n.NPCType, n.EventAggro)
		}
	}
}

// A clock with an empty ends the run once no one has stood at its place that long (a
// raid wiped or gone), so its lock does not hold out the clock's whole time. The control:
// with someone there it runs on.
func TestTriggerClockEndsWhenItsPlaceStandsEmpty(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Isle", "near", 50, `[{"do":"lock","lock":"isle"},{"do":"clock","seconds":600,"empty":30},{"do":"spawn","npc":"Orc Pawn","minutes":60},
		{"do":"awaittalk","npc":"Somebody"},{"do":"end"},{"do":"despawn"}]`)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	w.at(40 * time.Second)
	if n := len(w.spawned()); n != 1 || !w.e.Running()[20] {
		t.Fatalf("someone stands there: %d spawns, running %v", n, w.e.Running()[20])
	}
	w.gw.UpdatePosition(w.gp.ServerID, 25900, 50, 15000, 0)
	w.gw.UpdatePosition(w.gw.SessionPlayer(w.ps).ServerID, 25900, 50, 15000, 0)
	w.at(50 * time.Second) // 10 s since the last one left
	if n := len(w.spawned()); n != 1 || !w.e.BusyNamed("isle") {
		t.Fatalf("not yet its empty: %d spawns, busy %v", n, w.e.BusyNamed("isle"))
	}
	w.at(71 * time.Second)
	w.at(72 * time.Second)
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("%d spawns left after its empty", n)
	}
	if w.e.Running()[20] || w.e.BusyNamed("isle") {
		t.Fatal("the run and its lock should be over")
	}
}

// A spawn's max is how many of the name may stand near at once, whatever fires it: what
// multiplies on death stops at it. Dead ones do not count.
func TestTriggerSpawnMaxHoldsTheNumber(t *testing.T) {
	w := newTrigWorld(t)
	w.set(20, "Multiply", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","count":2,"spread":5,"minutes":10,"max":3}]`)
	for i, want := range []int{2, 3, 3} {
		if _, err := w.fire(20); err != nil {
			t.Fatal(err)
		}
		if n := len(w.spawned()); n != want {
			t.Fatalf("firing %d: %d stand, want %d", i, n, want)
		}
	}
	w.kill(w.spawned()[0])
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	if n := len(w.spawned()); n != 3 {
		t.Fatalf("one dead and fired: %d stand, want 3", n)
	}
}

// A repeatable quest done at a shop's window is offered there again in the same session
// (a second set of shards from Wibble): the window's once a session is for what has not
// been done. The control: declined, the next request opens the shop.
func TestAFinishedRepeatableIsOfferedAgainAtTheWindow(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, repeatable, offer, accept, decline, declined)
		VALUES (72, 'A set of shards', 1, 'Wibble', 1, 'Shards?', 'Yes.', 'No.', 'As you like.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, items, journal, reminder, says, events)
		VALUES (72, 1, 'talk', 'Wibble', '', 'Buy them.', '', 'Here.', '[{"on":"start","do":"dostep"}]')`)
	f.qe.Reload()
	shop := &entity.NPC{Name: "Wibble", ServerID: 901, NPCType: 0x81}
	if !f.qe.AtWindow(f.tw.s, f.tw.p, shop) {
		t.Fatal("not offered at the window")
	}
	m := f.msgs()
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 1) // declined
	f.msgs()
	if f.qe.AtWindow(f.tw.s, f.tw.p, shop) {
		t.Fatalf("a refused quest was offered again in the session: %v", f.msgs())
	}
	f.qe.LoggedIn(f.sid) // a new session
	if !f.qe.AtWindow(f.tw.s, f.tw.p, shop) {
		t.Fatal("not offered in a new session")
	}
	m = f.msgs()
	f.qe.Answer(f.tw.s, f.tw.p, boxID(m), 0)
	if m = f.msgs(); !strings.Contains(strings.Join(m, "|"), "You have finished a quest!") {
		t.Fatalf("not finished: %v", m)
	}
	if !f.qe.AtWindow(f.tw.s, f.tw.p, shop) {
		t.Fatal("not offered again once finished")
	}
}

// A sic sets the run's spawns on the NPC named its target nearest each (the user,
// 2026-11-08: "spawn nagafen attacking mr moogie"): the spawn's enemy is the nearer
// of two of that name in reach, a threat on it as a hit leaves. The controls: the
// same spawn with no sic has no enemy, and a sic with none of the name in reach
// sets nothing and says so.
func TestTriggerSicSetsItsSpawnsOnAnNPC(t *testing.T) {
	w := newTrigWorld(t)
	gp := w.gp
	near := &entity.NPC{ServerID: 701, Name: "Mr. Moogie", HP: 1000, MaxHP: 1000, Level: 50, X: gp.X + 6, Y: gp.Y, Z: gp.Z, World: gp.World}
	far := &entity.NPC{ServerID: 702, Name: "Mr. Moogie", HP: 1000, MaxHP: 1000, Level: 50, X: gp.X + 20, Y: gp.Y, Z: gp.Z, World: gp.World}
	w.gw.NPCs[near.ServerID], w.gw.NPCs[far.ServerID] = near, far
	w.set(20, "Nagafen vs Moogie", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":10},
		{"do":"sic","npc":"Orc Pawn","target":"Mr. Moogie","radius":30}]`)
	lines, err := w.fire(20)
	if err != nil {
		t.Fatal(err)
	}
	s := w.spawned()
	if len(s) != 1 {
		t.Fatalf("%d spawns, want 1", len(s))
	}
	if s[0].CurrentEnemy != near.ServerID || s[0].Threats[near.ServerID] <= 0 {
		t.Fatalf("the spawn's enemy %d (threats %v), want the nearer Mr. Moogie %d; lines %q", s[0].CurrentEnemy, s[0].Threats, near.ServerID, lines)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "1 of Orc Pawn set on Mr. Moogie") {
		t.Errorf("the GM's lines %q", lines)
	}

	before := map[entity.EntityID]bool{s[0].ServerID: true}
	w.set(21, "Plain", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":10}]`)
	w.set(22, "Nobody", "near", 50, `[{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":10},{"do":"sic","target":"Nobody Here"}]`)
	for _, slot := range []int{21, 22} {
		lines, err := w.fire(slot)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range w.spawned() {
			if !before[n.ServerID] {
				before[n.ServerID] = true
				if n.CurrentEnemy != 0 {
					t.Errorf("slot %d: a spawn's enemy %d, want none", slot, n.CurrentEnemy)
				}
			}
		}
		if slot == 22 && !strings.Contains(strings.Join(lines, "\n"), "none of its spawns has a Nobody Here within reach") {
			t.Errorf("slot 22's lines %q", lines)
		}
	}
}

// An awaitdead's seconds are the longest it waits: its spawn dead within them,
// the run goes on at the next tick, its despawn runs and the run ends, so the
// trigger fires again (the user, 2026-11-08: "the triggers dont reset when the
// NPC dies"; a death held the run, and its lock, until the seconds were out).
// The control: with its spawn alive, the run waits out the seconds.
func TestAnAwaitDeadGoesOnAtTheDeathNotItsSeconds(t *testing.T) {
	w := newTrigWorld(t)
	steps := `[{"do":"lock","lock":"pawn vs"},{"do":"spawn","npc":"Orc Pawn","attackable":true,"minutes":60},
		{"do":"awaitdead","npc":"Orc Pawn","seconds":3600},{"do":"despawn","npc":"Orc Pawn"}]`
	w.set(20, "Pawn", "near", 50, steps)
	if _, err := w.fire(20); err != nil {
		t.Fatal(err)
	}
	s := w.spawned()
	if len(s) != 1 {
		t.Fatalf("%d spawns, want 1", len(s))
	}
	w.at(5 * time.Second)
	if !w.e.Running()[20] {
		t.Fatal("over before its spawn died")
	}
	if !w.gw.KillNPCNow(s[0].ServerID) {
		t.Fatal("the spawn would not die")
	}
	w.at(10 * time.Second)
	w.at(11 * time.Second)
	if w.e.Running()[20] || w.e.BusyNamed("pawn vs") {
		t.Fatalf("its spawn dead at 5-10 s: running %v, its lock held %v; want both over", w.e.Running()[20], w.e.BusyNamed("pawn vs"))
	}
	if _, err := w.fire(20); err != nil {
		t.Fatalf("fired again: %v", err)
	}

	// the control: a run whose spawn lives waits out its seconds
	w.set(21, "Pawn2", "near", 50, strings.ReplaceAll(steps, "pawn vs", "pawn vs 2"))
	if _, err := w.fire(21); err != nil {
		t.Fatal(err)
	}
	w.at(20 * time.Second)
	w.at(30 * time.Minute)
	if !w.e.Running()[21] {
		t.Fatal("the control ended with its spawn alive, before its seconds")
	}
	w.at(3700 * time.Second)
	w.at(3701 * time.Second)
	if w.e.Running()[21] {
		t.Fatal("the control still runs past its seconds")
	}
}
