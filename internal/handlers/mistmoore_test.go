package handlers

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The werehunters' Mistmoore encounter as tools/mistmoore_20261121.py writes it (its --dump: MasterVG782's GameFAQs
// walkthrough), played through a quest and the triggers: Vedel's songbook played on the carpet is taken and the
// Maestro of Rancor rises five seconds later; his fight lost (the run ended without his kill) puts the questor back
// to the carpet with the songbook; vedel's song of awakening given to the mistmoore coffin is taken, the coffin goes
// down and Mayong Mistmoore rises; his kill does the step, and the coffin stays down half an hour after the run.
// The tool on a real database is TestRealMistmoore.

type mmDump struct {
	Triggers []struct {
		Slot   int             `json:"slot"`
		Name   string          `json:"name"`
		Scope  string          `json:"scope"`
		Radius float32         `json:"radius"`
		World  int             `json:"world"`
		X      float32         `json:"x"`
		Y      float32         `json:"y"`
		Z      float32         `json:"z"`
		Steps  json.RawMessage `json:"steps"`
	} `json:"triggers"`
	Rows map[string]struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		X     float32 `json:"x"`
		Y     float32 `json:"y"`
		Z     float32 `json:"z"`
		Level int     `json:"level"`
	} `json:"rows"`
	Items map[string]int            `json:"items"`
	Steps map[string]map[string]any `json:"steps"`
}

func mmDumpRead(t *testing.T) mmDump {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3 to dump the set")
	}
	out, err := exec.Command(py, "-I", filepath.Join("..", "..", "tools", "mistmoore_20261121.py"), "--dump").Output()
	if err != nil {
		t.Fatal(err)
	}
	var d mmDump
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMistmooreEncounterAsTheGuide(t *testing.T) {
	f := newQuestFixture(t)
	d := mmDumpRead(t)
	tome, page := f.pattern("tome of Vedel's songs", 1), f.pattern("vedel's song of awakening", 1)
	f.tw.p.HP, f.tw.p.MaxHP, f.tw.p.Level = 5000, 5000, 60

	// the rows: both bosses held out of the world, the coffin talked to
	for _, k := range []string{"maestro", "mistmoore", "coffin"} {
		r := d.Rows[k]
		n := &entity.NPC{ServerID: entity.EntityID(r.ID), Name: r.Name, HP: 1000, MaxHP: 1000, Level: max(r.Level, 1),
			X: r.X, Y: r.Y, Z: r.Z, SpawnX: r.X, SpawnY: r.Y, SpawnZ: r.Z, World: 0, NPCType: 0x80, SpawnOnly: k != "coffin"}
		if k == "coffin" {
			n.NPCType = 0x180
		}
		f.tw.w.NPCs[n.ServerID] = n
	}
	coffin := f.tw.w.NPCs[entity.EntityID(d.Rows["coffin"].ID)]

	e := NewTriggerEngine(f.db, f.tw.w, f.tw.mgr, f.inv)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	e.now = func() time.Time { return now }
	e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(e, nil) })
	at := func(dur time.Duration) {
		now = now.Add(dur)
		e.Tick()
	}
	for _, tr := range d.Triggers {
		f.exec(`INSERT INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
			tr.Slot, tr.Name, tr.Scope, tr.Radius, tr.World, tr.X, tr.Y, tr.Z, string(tr.Steps))
	}
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}

	// the quest: three steps before the tool's, then its 4-8, the items the fixture's
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept) VALUES (101383, 'Mistmoore', 1, 'Djivan Mertshak', 'x', 'y')`)
	for s := 1; s <= 3; s++ {
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (101383, ?, 'talk', 'Enrique Revnold', 'x')`, s)
	}
	fix := func(v any) any {
		if s, ok := v.(string); ok {
			s = strings.ReplaceAll(s, "701545", itoa(tome))
			return strings.ReplaceAll(s, "701546", itoa(page))
		}
		return v
	}
	for s := 4; s <= 8; s++ {
		cols, vals := []string{"questid", "step"}, []any{101383, s}
		for c, v := range d.Steps[itoa(s)] {
			cols, vals = append(cols, c), append(vals, fix(v))
		}
		f.exec(`INSERT INTO queststeps (`+strings.Join(cols, ", ")+`) VALUES (?`+strings.Repeat(", ?", len(cols)-1)+`)`, vals...)
	}
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 101383, QuestGive, 4); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	carpet := func() {
		f.tw.p.World, f.tw.p.X, f.tw.p.Y, f.tw.p.Z = 0, 14481, -60, 18966
	}
	maestros := func() (n int) {
		for _, c := range f.tw.w.NPCs {
			if c.Temporary && !c.Dead && c.Name == "Maestro of Rancor" {
				n++
			}
		}
		return n
	}

	// on the carpet without the songbook: nothing
	carpet()
	f.qe.checkReach()
	if s, _ := f.progress(101383); s != 4 || e.RunningNamed("Maestro of Rancor") {
		t.Fatalf("no songbook: step %d, running %v", s, e.RunningNamed("Maestro of Rancor"))
	}
	// a floor below the carpet: nothing (the step's height)
	f.own(tome, 1)
	f.tw.p.Y = -88
	f.qe.checkReach()
	if s, _ := f.progress(101383); s != 4 {
		t.Fatalf("a floor below: step %d", s)
	}
	// on the carpet with it: taken, the popup, the Maestro five seconds later
	carpet()
	f.qe.checkReach()
	m := strings.Join(f.msgs(), "|")
	if s, _ := f.progress(101383); s != 5 || f.held(tome) != 0 || !e.RunningNamed("Maestro of Rancor") {
		t.Fatalf("on the carpet: step %d, %d songbooks, running %v (%s)", s, f.held(tome), e.RunningNamed("Maestro of Rancor"), m)
	}
	if !strings.Contains(m, "songbook crumbles") {
		t.Errorf("no popup: %s", m)
	}
	at(time.Second)
	if n := maestros(); n != 0 {
		t.Fatalf("the Maestro came at once: %d", n)
	}
	at(5 * time.Second)
	if n := maestros(); n != 1 {
		t.Fatalf("five seconds on: %d Maestros", n)
	}
	for _, c := range f.tw.w.NPCs {
		if c.Temporary && c.Name == "Maestro of Rancor" && (c.CopyOf != entity.EntityID(d.Rows["maestro"].ID) || c.NPCType&0x80 != 0) {
			t.Errorf("the Maestro: a copy of %d, type %#x: want his row, hostile", c.CopyOf, c.NPCType)
		}
	}

	// the fight lost: the run ends with no kill, and the questor goes back to the Writ (step 2) as the guide has it,
	// the songbook not handed back (the steps give it again)
	if runs, _ := e.StopRuns(901); runs != 1 {
		t.Fatalf("%d runs stopped", runs)
	}
	t0 := time.Now().Add(time.Hour)
	f.qe.checkStranded(t0)
	f.qe.checkStranded(t0.Add(strandGrace + time.Second))
	if s, _ := f.progress(101383); s != 2 || f.held(tome) != 0 {
		t.Fatalf("after a lost fight: step %d, %d songbooks; want step 2 (the Writ), none handed back", s, f.held(tome))
	}

	// the song: at the coffin, taken, the coffin down, Mistmoore three seconds later
	if _, err := f.qe.SetCharacterQuest(f.sid, 101383, QuestSetStep, 6); err != nil {
		t.Fatal(err)
	}
	f.own(page, 1)
	f.msgs()
	f.tw.p.X, f.tw.p.Y, f.tw.p.Z = coffin.X+3, coffin.Y, coffin.Z
	f.qe.Talk(f.tw.s, f.tw.p, coffin)
	if ms := f.msgs(); boxID(ms) != 0 {
		f.qe.Answer(f.tw.s, f.tw.p, boxID(ms), 0)
	}
	if s, _ := f.progress(101383); s != 7 || f.held(page) != 0 || !e.RunningNamed("Mayong Mistmoore") {
		t.Fatalf("at the coffin: step %d, %d songs, running %v (%v)", s, f.held(page), e.RunningNamed("Mayong Mistmoore"), f.msgs())
	}
	at(time.Second)
	if !coffin.HiddenUntil.After(now.Add(70 * time.Minute)) {
		t.Fatalf("the coffin is not down: hidden until %v (now %v)", coffin.HiddenUntil, now)
	}
	at(3 * time.Second)
	var mis *entity.NPC
	for _, c := range f.tw.w.NPCs {
		if c.Temporary && !c.Dead && c.Name == "Mayong Mistmoore" {
			mis = c
		}
	}
	if mis == nil || mis.CopyOf != entity.EntityID(d.Rows["mistmoore"].ID) {
		t.Fatalf("no Mistmoore of his row: %+v", mis)
	}
	// a second song while he stands is refused (his lock), nothing taken
	if line := e.BusyNamed("Mayong Mistmoore"); !line {
		t.Fatal("the trigger is free while he stands")
	}

	// the group shares his kill: his trigger sets everyone near on the quest at steps 4-7 to step 8
	shared := 0
	for _, tr := range d.Triggers {
		var steps []database.TriggerStep
		if err := json.Unmarshal(tr.Steps, &steps); err != nil {
			t.Fatal(err)
		}
		for _, st := range steps {
			if st.Do == "questset" && st.Quest == 101383 && st.QuestStep == 8 && st.From == 4 {
				shared++
			}
		}
	}
	if shared != 1 {
		t.Fatalf("Mistmoore's trigger sets the group's quest %d times, want once", shared)
	}

	// his kill: the step done; the run ends and the coffin stays down half an hour from then
	f.killFor("Mayong Mistmoore", mis.ServerID)
	if s, _ := f.progress(101383); s != 8 {
		t.Fatalf("his kill: step %d", s)
	}
	mis.Dead, mis.HP, mis.DeathTime = true, 0, now
	at(time.Second)
	at(time.Second)
	if e.RunningNamed("Mayong Mistmoore") {
		t.Fatal("the run outlived him")
	}
	if !coffin.HiddenUntil.After(now.Add(29*time.Minute)) || coffin.HiddenUntil.After(now.Add(31*time.Minute)) {
		t.Fatalf("the coffin after his death: hidden until %v (now %v), want half an hour", coffin.HiddenUntil, now)
	}
}
