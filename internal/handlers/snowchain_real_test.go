package handlers

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Plane of Sky's Snow Chain as the owner's source has it (t212), on a real server's database with
// tools/snow_chain_20261118.py applied (EQOA_REAL_DB names it; a copy is read). Skipped where the tool is not applied.
//
// TestRealSnowChainAsWritten walks the chain to its end as a player does (the Night chain before it), the herb taken
// off a creature and handed to Guenzi with the stew's three, then the Storyteller's tale; the two templates and the
// two triggers read without a problem.
//
// TestRealSnowChainEvents plays the chain's two events on the database's own triggers with two characters: Ryini's
// walk does step 2 for the second one near as it starts; her death fails it, hides her row an hour from then and puts
// both back to "speak with Ryini"; Enraged Aeyinar's death sets the second one, on the herb's step, past her kill
// beside the questor; her fifteen minutes run out with the failure's words and the questor back to the walk.

func snowWalker(t *testing.T) (*questWalker, int) {
	t.Helper()
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	t.Cleanup(func() { w.db.Close() })
	var n, qid int
	w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'snow_chain_1118_log'`).Scan(&n)
	if n == 0 {
		t.Skip("tools/snow_chain_20261118.py is not applied to this database")
	}
	w.db.QueryRow(`SELECT questid FROM quests WHERE name = 'The Chain of Snow' AND giver = 'Mark of the Majestic'`).Scan(&qid)
	return w, qid
}

func TestRealSnowChainAsWritten(t *testing.T) {
	w, qid := snowWalker(t)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	qu := w.quest(qid)
	if qu == nil || len(qu.Steps) != 19 {
		t.Fatalf("quest %d: %v", qid, qu)
	}
	herb := qu.Step(13)
	if herb.Kind != database.StepLoot || len(herb.Also) != 7 {
		t.Fatalf("step 13: %s %q also %v", herb.Kind, herb.Target, herb.Also)
	}
	if give := qu.Step(15); give.Kind != database.StepGive || len(give.Items) != 4 {
		t.Fatalf("step 15: %s items %v", give.Kind, give.Items)
	}
	// the Night chain before it (its keys are bought at Liztin's, which the walk does not do): finished, as a GM's
	w.become(w.combo[0][0], w.combo[0][1])
	for _, g := range qu.Required() {
		for _, id := range g {
			if _, err := w.qe.SetCharacterQuest(w.p.CharServerID, id, QuestComplete, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	if why := w.walkOne(qu); why != "" {
		t.Fatalf("the chain: %s", why)
	}
	if _, done := w.progress(qid); done != 1 {
		t.Fatalf("the chain finished %d times", done)
	}
	var story int
	w.db.QueryRow(`SELECT questid FROM quests WHERE name = 'Tales of the Snow'`).Scan(&story)
	if w.p.Factions == nil {
		w.p.Factions = map[string]int{}
	}
	const before = 100
	w.p.Factions["Denizens of the Plane of Sky"] = before
	if why := w.walkOne(w.quest(story)); why != "" {
		t.Fatalf("the Storyteller: %s", why)
	}
	if got := w.p.Factions["Denizens of the Plane of Sky"]; got != before+250 {
		t.Errorf("the Storyteller's faction: %d, want %d + 250", got, before)
	}

	// the templates and triggers as the server reads them
	tpl, bad, err := w.db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if strings.Contains(strings.ToLower(b), "aeyinar") {
			t.Errorf("template: %s", b)
		}
	}
	for _, n := range []string{"enraged aeyinar", "aeyinar"} {
		if _, ok := tpl[n]; !ok {
			t.Errorf("no template %q read", n)
		}
	}
	for _, name := range []string{"Snow 1: Ryini's Walk", "Snow 4: Enraged Aeyinar"} {
		tr := snowTrigger(t, w.db, name)
		if p := database.TriggerProblems(tr); len(p) > 0 {
			t.Errorf("%s: %v", name, p)
		}
	}
}

// snowTrigger is the database's trigger of that name.
func snowTrigger(t *testing.T, db *database.SQLiteDB, name string) database.GMTrigger {
	t.Helper()
	var tr database.GMTrigger
	var on int
	if err := db.QueryRow(`SELECT slot, name, enabled, scope, radius, world, x, y, z, steps FROM gmtriggers WHERE name = ?`, name).
		Scan(&tr.Slot, &tr.Name, &on, &tr.Scope, &tr.Radius, &tr.World, &tr.X, &tr.Y, &tr.Z, &tr.Steps); err != nil {
		t.Fatalf("trigger %q: %v", name, err)
	}
	tr.Enabled = on == 1
	return tr
}

// snowRow puts the database's npcs row in the world as itself (its id its ServerID).
func snowRow(t *testing.T, w *questWalker, id int, spawnOnly bool) *entity.NPC {
	t.Helper()
	n := &entity.NPC{ServerID: entity.EntityID(id), HP: 1000, MaxHP: 1000, NPCType: 0x80, SpawnOnly: spawnOnly}
	var lv int
	if err := w.db.QueryRow(`SELECT npc_name, x_coord, y_coord, z_coord, world, npc_level FROM npcs WHERE id = ?`, id).
		Scan(&n.Name, &n.X, &n.Y, &n.Z, &n.World, &lv); err != nil {
		t.Fatalf("npcs %d: %v", id, err)
	}
	n.Level = lv
	n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
	w.gw.NPCs[n.ServerID] = n
	return n
}

func TestRealSnowChainEvents(t *testing.T) {
	w, qid := snowWalker(t)
	if os.Getenv("SNOW_LOG") == "" {
		log.SetOutput(io.Discard)
		defer log.SetOutput(os.Stderr)
	}
	now := time.Now().Truncate(time.Second)
	e := NewTriggerEngine(w.db, w.gw, w.mgr, w.inv)
	e.now = func() time.Time { return now }
	e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	questEngine.Store(w.qe) // a trigger's queststep and questset reach the quests through it, as the server's do
	t.Cleanup(func() { questEngine.CompareAndSwap(w.qe, nil) })
	walk, enraged := snowTrigger(t, w.db, "Snow 1: Ryini's Walk"), snowTrigger(t, w.db, "Snow 4: Enraged Aeyinar")

	// the rows the events copy, in the world as the server has them
	var ryiniRow, mephitRow, aeyRow, gargRow int
	w.db.QueryRow(`SELECT id FROM npcs WHERE npc_name = 'Ryini' AND world = 4`).Scan(&ryiniRow)
	w.db.QueryRow(`SELECT id FROM npcs WHERE npc_name = 'Wind Mephit' AND world = 4`).Scan(&mephitRow)
	w.db.QueryRow(`SELECT id FROM npcs WHERE npc_name = 'Enraged Aeyinar' AND world = 4`).Scan(&aeyRow)
	w.db.QueryRow(`SELECT min(id) FROM npcs WHERE npc_name = 'icebound gargoyle' AND world = 4`).Scan(&gargRow)
	for _, k := range []int{ryiniRow, mephitRow, aeyRow} {
		if !strings.Contains(walk.Steps+enraged.Steps, "#"+itoa(k)) {
			t.Fatalf("row %d is not one the triggers copy (%s | %s)", k, walk.Steps, enraged.Steps)
		}
	}
	ryini := snowRow(t, w, ryiniRow, false)
	snowRow(t, w, mephitRow, true)
	aey := snowRow(t, w, aeyRow, true)
	snowRow(t, w, gargRow, false)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}

	// two characters: A the questor, B beside it
	type char struct {
		sess *session.Session
		p    *entity.Player
	}
	var ch [2]char
	for i, cr := range [][2]int{w.combo[0], w.combo[len(w.combo)-1]} {
		w.become(cr[0], cr[1])
		w.p.Level = 60
		ch[i] = char{w.sess, w.p}
	}
	at := func(i int, n *entity.NPC) {
		ch[i].p.World, ch[i].p.X, ch[i].p.Y, ch[i].p.Z = n.World, n.X+2, n.Y, n.Z+2
	}
	set := func(i, step int) {
		t.Helper()
		w.qe.mu.Lock()
		cq := w.qe.charLocked(ch[i].p.CharServerID).quests[qid]
		w.qe.mu.Unlock()
		action := QuestGive
		if cq != nil && cq.Active() {
			action = QuestSetStep
		}
		if _, err := w.qe.SetCharacterQuest(ch[i].p.CharServerID, qid, action, step); err != nil {
			t.Fatal(err)
		}
	}
	stepOf := func(i int) int {
		w.qe.mu.Lock()
		defer w.qe.mu.Unlock()
		if cq := w.qe.charLocked(ch[i].p.CharServerID).quests[qid]; cq != nil {
			return cq.Step
		}
		return -1
	}
	waitStep := func(i, want int) int {
		for k := 0; k < 400 && stepOf(i) != want; k++ {
			time.Sleep(5 * time.Millisecond)
		}
		return stepOf(i)
	}
	copyOf := func(name string) *entity.NPC {
		for _, n := range w.gw.NPCs {
			if n.Temporary && !n.Dead && strings.EqualFold(n.Name, name) {
				return n
			}
		}
		return nil
	}

	// Ryini's walk: A talks, B near on step 2 is taken on with it
	at(0, ryini)
	at(1, ryini)
	set(0, 2)
	set(1, 2)
	w.qe.Talk(ch[0].sess, ch[0].p, ryini)
	if !e.RunningNamed(walk.Name) {
		t.Fatalf("the walk did not start (%v)", questMsgs(ch[0].sess))
	}
	if a, b := waitStep(0, 3), waitStep(1, 3); a != 3 || b != 3 {
		t.Fatalf("as the walk starts: A on %d, B on %d, want 3 and 3", a, b)
	}
	walker := copyOf("Ryini")
	if walker == nil || !ryini.HiddenUntil.After(now) {
		t.Fatalf("her copy %v, her row hidden until %v", walker, ryini.HiddenUntil)
	}
	// she falls at 12 minutes
	now = now.Add(12 * time.Minute)
	e.Tick()
	walker.Dead, walker.HP = true, 0
	now = now.Add(time.Second)
	e.Tick()
	if e.RunningNamed(walk.Name) {
		t.Fatal("the walk lives after her death")
	}
	if want := now.Add(time.Hour); !ryini.HiddenUntil.Equal(want) {
		t.Fatalf("her row hidden until %v, want %v (an hour from her death)", ryini.HiddenUntil, want)
	}
	if !strings.Contains(strings.Join(questMsgs(ch[1].sess), "|"), "Ryini has fallen") {
		t.Fatal("B was not told she fell")
	}
	t1 := time.Now().Add(time.Hour)
	w.qe.checkStranded(t1)
	w.qe.checkStranded(t1.Add(strandGrace + time.Second))
	if a, b := stepOf(0), stepOf(1); a != 2 || b != 2 {
		t.Fatalf("after the failed walk: A on %d, B on %d, want both back on 2", a, b)
	}

	// Enraged Aeyinar: A on her kill, B on the herb's step; her death sets B past it beside A
	at(0, aey)
	at(1, aey)
	set(0, 17)
	set(1, 13)
	if _, err := e.FireNamed(enraged.Name, TriggerFirer{Name: ch[0].p.Name, Sess: ch[0].sess, Player: ch[0].p, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	boss := copyOf("Enraged Aeyinar")
	if boss == nil {
		t.Fatal("no Enraged Aeyinar")
	}
	now = now.Add(10 * time.Minute)
	e.Tick()
	boss.Dead, boss.HP = true, 0
	w.qe.killed(entity.KillCredit{NPCName: boss.Name, NPCSpawn: boss.ServerID, NPCID: boss.ServerID, Died: time.Now(),
		Players: []entity.KillPlayer{{Session: ch[0].sess, ServerID: ch[0].p.CharServerID, Name: ch[0].p.Name}}})
	now = now.Add(time.Second)
	e.Tick()
	if a, b := waitStep(0, 18), waitStep(1, 18); a != 18 || b != 18 {
		t.Fatalf("after her death: A on %d, B on %d, want both on 18 (the way back)", a, b)
	}

	// her fifteen minutes: A on the kill again, she outlives them
	set(0, 17)
	for k := 0; k < 3 && e.RunningNamed(enraged.Name); k++ {
		now = now.Add(time.Second)
		e.Tick()
	}
	if _, err := e.FireNamed(enraged.Name, TriggerFirer{Name: ch[0].p.Name, Sess: ch[0].sess, Player: ch[0].p, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	questMsgs(ch[0].sess)
	now = now.Add(14 * time.Minute)
	e.Tick()
	if !e.RunningNamed(enraged.Name) || copyOf("Enraged Aeyinar") == nil {
		t.Fatal("she is gone before her fifteen minutes")
	}
	now = now.Add(time.Minute + time.Second)
	e.Tick()
	if e.RunningNamed(enraged.Name) || copyOf("Enraged Aeyinar") != nil {
		t.Fatal("she outlived her fifteen minutes")
	}
	if !strings.Contains(strings.Join(questMsgs(ch[0].sess), "|"), "fury is spent") {
		t.Fatal("A was not told her time ran out")
	}
	t2 := t1.Add(time.Hour)
	w.qe.checkStranded(t2)
	w.qe.checkStranded(t2.Add(strandGrace + time.Second))
	if a := stepOf(0); a != 16 {
		t.Fatalf("after her time ran out: A on %d, want 16 (the walk toward her rest)", a)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
