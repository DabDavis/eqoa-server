package handlers

import (
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The GMs' triggers (gmtriggers.go) against the chat world: the GM (account
// 1) and a player 14 away in Tunaria (gmWorld), one 1000 away there and one
// in Odus, an Orc Pawn to copy, and an engine over a database of its own
// whose clock and dice are the test's.

type trigWorld struct {
	*chatWorld
	t      *testing.T
	e      *TriggerEngine
	db     *database.SQLiteDB
	now    time.Time
	gs, ps *session.Session // the GM, the player beside it
	fs, os *session.Session // 1000 away, in Odus
	gp     *entity.Player
}

func newTrigWorld(t *testing.T) *trigWorld {
	t.Helper()
	c, gs, gp, ps, _ := gmWorld(t)
	fs, _ := c.join(3, "Faraway", 26000, 15000, 0)
	fs.SetAccountID(3)
	os, _ := c.join(4, "Odusian", 25000, 15000, 2)
	os.SetAccountID(4)
	c.gw.NPCs[700] = &entity.NPC{ServerID: 700, Name: "Orc Pawn", HP: 100, MaxHP: 100, Level: 5, NPCType: 0x80,
		X: 100, Y: 50, Z: 100, World: 0}
	db, err := database.NewSQLite(t.TempDir() + "/triggers.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := &trigWorld{chatWorld: c, t: t, db: db, gs: gs, ps: ps, fs: fs, os: os, gp: gp,
		now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
	w.e = NewTriggerEngine(db, c.gw, c.sm, nil)
	w.e.now = func() time.Time { return w.now }
	w.e.rand = func() float64 { return 0.5 }
	triggerEngine.Store(w.e)
	t.Cleanup(func() { triggerEngine.CompareAndSwap(w.e, nil) })
	return w
}

// set writes a trigger at the GM's place (world -1) and reads the table
// again.
func (w *trigWorld) set(slot int, name, scope string, radius float32, steps string) {
	w.t.Helper()
	w.setAt(slot, name, scope, radius, -1, 0, 0, 0, steps, true)
}

func (w *trigWorld) setAt(slot int, name, scope string, radius float32, world int, x, y, z float32, steps string, on bool) {
	w.t.Helper()
	enabled := 0
	if on {
		enabled = 1
	}
	if _, err := w.db.Exec(`INSERT OR REPLACE INTO gmtriggers (slot, name, enabled, scope, radius, world, x, y, z, steps)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, slot, name, enabled, scope, radius, world, x, y, z, steps); err != nil {
		w.t.Fatal(err)
	}
	if err := w.e.Reload(); err != nil {
		w.t.Fatal(err)
	}
}

// fire fires slot as the GM in the world, the lines answered.
func (w *trigWorld) fire(slot int) ([]string, error) {
	return w.e.Fire(slot, TriggerFirer{Name: w.gp.Name, Sess: w.gs, Player: w.gp})
}

// at moves the clock to d after the start and ticks.
func (w *trigWorld) at(d time.Duration) {
	w.now = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC).Add(d)
	w.e.Tick()
}

// clear forgets what every session was sent.
func (w *trigWorld) clear() {
	for _, s := range []*session.Session{w.gs, w.ps, w.fs, w.os} {
		s.Queue().GetPending()
	}
}

// yellow is the GM-yellow lines s was sent.
func yellow(t *testing.T, s *session.Session) []string {
	t.Helper()
	var out []string
	for _, l := range heard(t, s) {
		if l.op == protocol.OpcodeColoredChat && l.rgb == gmBroadcastRGB {
			out = append(out, l.text)
		}
	}
	return out
}

// spawned is the trigger copies in the world: temporary, alive.
func (w *trigWorld) spawned() []*entity.NPC {
	var out []*entity.NPC
	for _, n := range w.gw.NPCs {
		if n.Temporary && !n.Dead {
			out = append(out, n)
		}
	}
	return out
}

// A near trigger's broadcast reaches the players within its reach of the GM,
// in the GMs' yellow, %p each one's own name. The controls: the player 1000
// away and the one in Odus hear nothing. world takes the far one too, not
// Odus's; gm the GM alone; all everyone.
func TestTriggerScopes(t *testing.T) {
	w := newTrigWorld(t)
	steps := `[{"do":"broadcast","text":"The ground shakes, %p!"}]`
	for _, c := range []struct {
		scope string
		hear  [4]bool // the GM, beside, far, Odus
	}{
		{"near", [4]bool{true, true, false, false}},
		{"world", [4]bool{true, true, true, false}},
		{"gm", [4]bool{true, false, false, false}},
		{"all", [4]bool{true, true, true, true}},
	} {
		w.set(1, "Quake", c.scope, 0, steps)
		w.clear()
		lines, err := w.fire(1)
		if err != nil {
			t.Fatalf("%s: %v %q", c.scope, err, lines)
		}
		for i, s := range []*session.Session{w.gs, w.ps, w.fs, w.os} {
			got := yellow(t, s)
			if heardIt := len(got) == 1; heardIt != c.hear[i] {
				t.Errorf("%s: session %d heard %q, want %v", c.scope, i, got, c.hear[i])
			}
		}
		if got := yellow(t, w.gs); len(got) == 1 && got[0] != "The ground shakes, Gm!" {
			t.Errorf("%s: %%p: %q", c.scope, got[0])
		}
	}
	// the reach is the row's: 5 takes the GM alone (the player is 14 away)
	w.set(1, "Quake", "near", 5, steps)
	w.clear()
	if _, err := w.fire(1); err != nil {
		t.Fatal(err)
	}
	if len(yellow(t, w.gs)) != 1 || len(yellow(t, w.ps)) != 0 {
		t.Fatalf("radius 5: the GM %q, the player %q", yellow(t, w.gs), yellow(t, w.ps))
	}
}

// A spawn step places its count of copies round the GM within its spread,
// each a temporary NPC; a hostile one may be attacked (no NPC bit) and
// attacks on sight (EventAggro). A wait puts the despawn off until a Tick
// passes it; the control: the Tick before the wait's end leaves them.
func TestTriggerSpawnWaitDespawn(t *testing.T) {
	w := newTrigWorld(t)
	w.set(2, "Orcs", "near", 0, `[{"do":"spawn","npc":"orc pawn","count":3,"spread":10,"hostile":true},
		{"do":"wait","seconds":30},{"do":"despawn"}]`)
	lines, err := w.fire(2)
	if err != nil {
		t.Fatal(err)
	}
	if !hasLine(lines, "spawned 3 hostile Orc Pawn") || !hasLine(lines, "waiting 30 s") {
		t.Fatalf("lines: %q", lines)
	}
	got := w.spawned()
	if len(got) != 3 {
		t.Fatalf("%d spawned", len(got))
	}
	for _, n := range got {
		d := math.Hypot(float64(n.X-25000), float64(n.Z-15000))
		if d < 4 || d > 10 || n.World != 0 {
			t.Errorf("%s at %.1f from the GM in world %d", n.Name, d, n.World)
		}
		if n.NPCType&0x80 != 0 || !n.EventAggro {
			t.Errorf("hostile: npc type %#x, event aggro %v", n.NPCType, n.EventAggro)
		}
	}
	if tpl := w.gw.NPCs[700]; tpl.EventAggro || tpl.NPCType&0x80 == 0 {
		t.Fatal("the template took the spawn's hostility")
	}
	w.at(29 * time.Second)
	if n := len(w.spawned()); n != 3 {
		t.Fatalf("before the wait's end: %d", n)
	}
	if !w.e.Running()[2] {
		t.Fatal("not running while it waits")
	}
	w.clear()
	w.at(30 * time.Second)
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("after the despawn: %d", n)
	}
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 2 (Orcs): took away 3 of its spawns") {
		t.Fatalf("the GM told: %q", got)
	}
	if w.e.Running()[2] {
		t.Fatal("still running with its steps done and no spawn")
	}
	// not hostile: the template's flags as they are
	w.set(3, "Pawn", "near", 0, `[{"do":"spawn","npc":"Orc Pawn"}]`)
	if _, err := w.fire(3); err != nil {
		t.Fatal(err)
	}
	if got := w.spawned(); len(got) != 1 || got[0].NPCType&0x80 == 0 || got[0].EventAggro {
		t.Fatalf("a spawn not hostile: %+v", got)
	}
}

// A spawn stays its minutes: taken away by the Tick past them (the control:
// the Tick before), and the run ends with them.
func TestTriggerSpawnStays(t *testing.T) {
	w := newTrigWorld(t)
	w.set(1, "Brief", "gm", 0, `[{"do":"spawn","npc":"Orc Pawn","count":2,"minutes":1}]`)
	if _, err := w.fire(1); err != nil {
		t.Fatal(err)
	}
	w.at(59 * time.Second)
	if n := len(w.spawned()); n != 2 || !w.e.Running()[1] {
		t.Fatalf("within its minute: %d spawned, running %v", n, w.e.Running()[1])
	}
	w.at(61 * time.Second)
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("past its minute: %d", n)
	}
	w.at(62 * time.Second)
	if w.e.Running()[1] || len(w.e.Runs()) != 0 {
		t.Fatalf("the run did not end: %+v", w.e.Runs())
	}
	// one killed goes as a GM's spawn's corpse goes, and its run ends
	if _, err := w.fire(1); err != nil {
		t.Fatal(err)
	}
	for _, n := range w.spawned() {
		w.gw.KillNPCNow(n.ServerID)
	}
	w.at(63 * time.Second)
	if len(w.e.Runs()) != 0 {
		t.Fatalf("with its spawns dead: %+v", w.e.Runs())
	}
}

// stop ends runs: StopRuns (#trigger stop, the panel) and another trigger's
// stop step take their spawns away and leave their steps undone. The
// control: a run of another trigger goes on.
func TestTriggerStop(t *testing.T) {
	w := newTrigWorld(t)
	long := `[{"do":"spawn","npc":"Orc Pawn"},{"do":"wait","seconds":600},{"do":"broadcast","text":"late"}]`
	w.set(1, "One", "gm", 0, long)
	w.set(2, "Two", "gm", 0, long)
	w.set(3, "Stop one", "gm", 0, `[{"do":"stop","slot":1}]`)
	w.set(4, "Stop all", "gm", 0, `[{"do":"stop"}]`)
	for _, s := range []int{1, 1, 2} {
		if _, err := w.fire(s); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(w.spawned()); n != 3 {
		t.Fatalf("%d spawned", n)
	}
	lines, err := w.fire(3)
	if err != nil || !hasLine(lines, "stopped 2 runs of trigger 1 (2 spawns taken away)") {
		t.Fatalf("stop step: %q %v", lines, err)
	}
	if runs := w.e.Runs(); len(runs) != 2 || runs[0].Slot != 2 {
		t.Fatalf("after stopping trigger 1: %+v", runs)
	}
	if n := len(w.spawned()); n != 1 {
		t.Fatalf("trigger 2's spawn: %d", n)
	}
	w.clear()
	w.at(601 * time.Second)
	if got := yellow(t, w.gs); len(got) != 1 {
		t.Fatalf("the run left goes on: %q", got)
	}
	if _, err := w.fire(1); err != nil {
		t.Fatal(err)
	}
	if lines, _ := w.fire(4); !hasLine(lines, "stopped 2 other runs") {
		t.Fatalf("stop every other: %q", lines)
	}
	w.at(602 * time.Second) // the stop's own run, done, ends
	if runs, spawns := w.e.StopRuns(0); runs != 0 || spawns != 0 {
		t.Fatalf("StopRuns with nothing running: %d %d", runs, spawns)
	}
	if n := len(w.spawned()); n != 0 {
		t.Fatalf("spawns left: %d", n)
	}
}

// What keeps a trigger from firing says so and does nothing: no trigger in
// the slot, one off, steps that cannot run, a trigger at the GM's place
// fired with no GM in the world (the panel's). The control: a place of its
// own fires from the panel.
func TestTriggerRefusals(t *testing.T) {
	w := newTrigWorld(t)
	w.setAt(1, "Off", "near", 0, -1, 0, 0, 0, `[{"do":"broadcast","text":"x"}]`, false)
	w.set(2, "Broken", "near", 0, `[{"do":"dance"}]`)
	w.set(3, "Here", "near", 0, `[{"do":"broadcast","text":"x"}]`)
	w.setAt(4, "There", "all", 0, 0, 25000, 50, 15000, `[{"do":"broadcast","text":"from the panel"}]`, true)
	for _, c := range []struct {
		slot int
		by   TriggerFirer
		want error
		line string
	}{
		{9, TriggerFirer{Player: w.gp}, ErrTriggerNone, "Trigger 9: none set"},
		{1, TriggerFirer{Player: w.gp}, ErrTriggerOff, "Trigger 1 (Off) is off"},
		{2, TriggerFirer{Player: w.gp}, ErrTriggerBroken, `do "dance" is not one of`},
		{3, TriggerFirer{Name: "panel"}, ErrTriggerNoPlace, "where the GM stands"},
	} {
		w.clear()
		lines, err := w.e.Fire(c.slot, c.by)
		if !errors.Is(err, c.want) || !hasLine(lines, c.line) {
			t.Errorf("slot %d: %v %q, want %v %q", c.slot, err, lines, c.want, c.line)
		}
		if got := yellow(t, w.ps); len(got) != 0 {
			t.Errorf("slot %d did something: %q", c.slot, got)
		}
	}
	if len(w.e.Runs()) != 0 {
		t.Fatalf("runs: %+v", w.e.Runs())
	}
	lines, err := w.e.Fire(4, TriggerFirer{Name: "Panel GM"})
	if err != nil || len(yellow(t, w.os)) != 1 {
		t.Fatalf("a place of its own from the panel: %v %q", err, lines)
	}
}

// The TEST menu's TRIGGER line x fires slot x+1 and the GM is told what it
// did; a player's 0x0702 fires nothing (the control).
func TestTriggerFromTheMenu(t *testing.T) {
	w := newTrigWorld(t)
	w.set(3, "Third line", "near", 0, `[{"do":"broadcast","text":"line three"}]`)
	trig := func(s *session.Session, h string) {
		if err := w.h.HandleGMTrigger(s, incoming(protocol.OpcodeGMTrigger, unhex(t, h))); err != nil {
			t.Fatal(err)
		}
	}
	trig(w.ps, "6402")
	if got := yellow(t, w.ps); len(got) != 0 {
		t.Fatalf("a player's TRIGGER: %q", got)
	}
	trig(w.gs, "6402")
	if got := yellow(t, w.ps); len(got) != 1 || got[0] != "line three" {
		t.Fatalf("the GM's line 2: %q", got)
	}
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 3 (Third line) fired") || !hasLine(got, "said to 2 players") {
		t.Fatalf("the GM told: %q", got)
	}
	trig(w.gs, "6407")
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 8: none set") {
		t.Fatalf("an empty line: %q", got)
	}
}

// names is the bodies of the 0x7E08s queued for s.
func names(s *session.Session) []string {
	var out []string
	for _, b := range sentAll(s, protocol.OpcodeGMTriggersData) {
		out = append(out, hex.EncodeToString(b))
	}
	s.Queue().GetPending()
	return out
}

// The names for vanilla's TEST menu: a GM's 1 is answered with the whole set
// at once, again when a trigger changes, starts or ends, never the same set
// twice; 0 stops them. The control: a player's ask is dropped.
func TestTriggerNames(t *testing.T) {
	w := newTrigWorld(t)
	w.set(2, "Orcs", "gm", 0, `[{"do":"spawn","npc":"Orc Pawn","minutes":1}]`)
	w.setAt(9, "Not in the menu", "gm", 0, -1, 0, 0, 0, `[{"do":"broadcast","text":"x"}]`, true)
	ask := func(s *session.Session, on string) {
		if err := w.h.HandleGMTriggerNames(s, incoming(protocol.OpcodeGMTriggers, unhex(t, on))); err != nil {
			t.Fatal(err)
		}
	}
	w.clear()
	ask(w.ps, "01000000")
	if got := names(w.ps); len(got) != 0 {
		t.Fatalf("a player's ask: %q", got)
	}
	ask(w.gs, "01000000")
	orcs := "01" + "01" + "01" + "04" + hex.EncodeToString([]byte("Orcs"))
	if got := names(w.gs); len(got) != 1 || got[0] != orcs {
		t.Fatalf("the GM's names: %q, want %s", got, orcs)
	}
	w.e.Tick()
	if got := names(w.gs); len(got) != 0 {
		t.Fatalf("the same set again: %q", got)
	}
	if _, err := w.fire(2); err != nil {
		t.Fatal(err)
	}
	running := "01" + "01" + "03" + "04" + hex.EncodeToString([]byte("Orcs"))
	if got := names(w.gs); len(got) != 1 || got[0] != running {
		t.Fatalf("running: %q, want %s", got, running)
	}
	w.at(2 * time.Minute)
	w.at(2*time.Minute + time.Second)
	if got := names(w.gs); len(got) != 1 || got[0] != orcs {
		t.Fatalf("ended: %q", got)
	}
	w.setAt(1, "First", "gm", 0, -1, 0, 0, 0, `[{"do":"broadcast","text":"x"}]`, false)
	both := "02" + "00" + "00" + "05" + hex.EncodeToString([]byte("First")) + "01" + "01" + "04" + hex.EncodeToString([]byte("Orcs"))
	if got := names(w.gs); len(got) != 1 || got[0] != both {
		t.Fatalf("a trigger added: %q, want %s", got, both)
	}
	ask(w.gs, "00000000")
	w.set(3, "Third", "gm", 0, `[{"do":"broadcast","text":"x"}]`)
	if got := names(w.gs); len(got) != 0 {
		t.Fatalf("after 0: %q", got)
	}
}

// encodeTriggerNames: a name cut at 32 bytes on a character's edge, a
// trigger with no name named by its slot, slots past the menu's eight left
// out.
func TestEncodeTriggerNames(t *testing.T) {
	long := strings.Repeat("é", 20) // 40 bytes
	body := encodeTriggerNames([]database.GMTrigger{
		{Slot: 1, Name: long, Enabled: true},
		{Slot: 8},
		{Slot: 9, Name: "Nine", Enabled: true},
	}, map[int]bool{8: true})
	cut := strings.Repeat("é", 16)
	want := "02" + "00" + "01" + "20" + hex.EncodeToString([]byte(cut)) +
		"07" + "02" + "09" + hex.EncodeToString([]byte("Trigger 8"))
	if got := hex.EncodeToString(body); got != want {
		t.Fatalf("%s\nwant %s", got, want)
	}
}

// #trigger: alone the list with each line's state, a number fires, a name's
// start fires, an exact name over its longer namesakes, stop ends. The
// control: a name no trigger has.
func TestTriggerCommand(t *testing.T) {
	w := newTrigWorld(t)
	w.set(1, "Orc raid", "gm", 0, `[{"do":"spawn","npc":"Orc Pawn"},{"do":"wait","seconds":60},{"do":"despawn"}]`)
	w.set(2, "Orc", "gm", 0, `[{"do":"broadcast","text":"just orc"}]`)
	w.setAt(5, "Gate", "near", 30, 2, 1000, 20, 2000, `[{"do":"broadcast","text":"x"}]`, false)
	w.clear()
	w.say(w.gs, "#trigger")
	got := gmReplies(t, w.gs)
	for _, want := range []string{"1 Orc raid: on, gm, where you stand", "2 Orc: on, gm", "5 Gate: off, near (30), at (1000, 2000) in Odus"} {
		if !hasLine(got, want) {
			t.Errorf("the list lacks %q: %q", want, got)
		}
	}
	w.say(w.gs, "#trigger 1")
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 1 (Orc raid) fired") {
		t.Fatalf("#trigger 1: %q", got)
	}
	w.say(w.gs, "/trigger orc")
	if got := yellow(t, w.gs); len(got) != 1 || got[0] != "just orc" {
		t.Fatalf("an exact name before its namesake: %q", got)
	}
	w.say(w.gs, "#trigger orc r")
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 1 (Orc raid) fired") {
		t.Fatalf("a name's start: %q", got)
	}
	w.say(w.gs, "#trigger")
	if got := gmReplies(t, w.gs); !hasLine(got, "1 Orc raid: on, gm, where you stand, RUNNING") {
		t.Fatalf("running: %q", got)
	}
	w.say(w.gs, "#trigger stop 1")
	if got := gmReplies(t, w.gs); !hasLine(got, "Trigger 1: stopped 2 runs, 2 spawns taken away") {
		t.Fatalf("#trigger stop 1: %q", got)
	}
	w.say(w.gs, "#trigger nothing like it")
	if got := gmReplies(t, w.gs); !hasLine(got, `No trigger named "nothing like it"`) {
		t.Fatalf("the control: %q", got)
	}
}

// A popup's 0x0034 carries the triggers' mark, so its answer is told from a
// quest's and an NPC's; the control: a quest box's id is not one.
func TestTriggerPopup(t *testing.T) {
	w := newTrigWorld(t)
	w.set(1, "Notice", "gm", 0, `[{"do":"popup","text":"The gates are closing.","answer":"Run"}]`)
	w.clear()
	if _, err := w.fire(1); err != nil {
		t.Fatal(err)
	}
	box := sentAll(w.gs, protocol.OpcodeQuestPopupMulti)
	if len(box) != 1 {
		t.Fatalf("%d boxes", len(box))
	}
	id := uint32(box[0][0]) | uint32(box[0][1])<<8 | uint32(box[0][2])<<16 | uint32(box[0][3])<<24
	if !IsTriggerBox(id) || IsTriggerBox(questBoxMark|1) || IsTriggerBox(700) {
		t.Fatalf("id %#x", id)
	}
}
