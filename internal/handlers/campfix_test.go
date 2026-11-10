package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The players' camp reports (campfix.go; board entry 513, petition 1423):
// /campfix opens a report and sends it, /mark, /markadd and /markremove make
// its marks, everyone's in every spelling; the marks are a session's until
// sent; sending files a row of campfix_reports and a work board entry, and
// changes no spawn.

// A camp in world 0 round (25000, 50, 15000), where the player stands.
const (
	cfBanditE    = 401 // a bandit 40 east, facing 64
	cfBanditN    = 402 // a bandit 60 north, facing 192
	cfCaptain    = 403 // a bandit captain, the only one
	cfScout      = 404 // a bandit scout, the only one
	cfFarBandit  = 405 // a bandit 7,000 off in the same world
	cfLord       = 406 // Lord Brutus: one of a kind, level 50
	cfWolf       = 407 // a wolf, in world 1
	cfFarBandit2 = 408 // another bandit far off
	cfTownFolk   = 450 // 450-461: twelve townsfolk 1,000 east, a town (towns.go)
	cfRats       = 470 // 470-476: rat one ... rat seven, round (27000, 17000)
	cfGoblins    = 500 // 500-569: seventy goblin raiders round (20000, 20000)
	cfOgres      = 600 // 600-669: seventy ogres of long names round (21500, 21500), 670 a plain ogre
)

const cfCreature, cfHuman = 0x1234, 0x7777

// cfLevels is accountinfo's acctlevel by account, no account a sub GM nor a
// tester (the player's account has no flag of any kind).
type cfLevels map[int]int

func (l cfLevels) AccessLevel(id int) (int, error) { return l[id], nil }
func (l cfLevels) SubGM(int) (bool, error)         { return false, nil }

// cfWorld is the camp's world: the GM Gm (account 1, acctlevel 1) beside the
// camp and the player Player (account 2, acctlevel 0, no tester's row) in it,
// the camp tools on a database of its own, a logout's hook as the registry
// gives it, and the test's own clock.
type cfWorld struct {
	*chatWorld
	db     *database.SQLiteDB
	gs, ps *session.Session
	gp, pp *entity.Player
	now    time.Time
	slot   int
}

func campfixWorld(t *testing.T) *cfWorld {
	t.Helper()
	c := newChatWorld(t)
	c.h.SetAccessLevels(cfLevels{1: 1, 2: 0})
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "campfix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	type row struct {
		id        int
		name      string
		world     int
		x, y, z   float64
		facing    int
		model     int64
		level, ty int
	}
	rows := []row{
		{cfBanditE, "bandit", 0, 25040, 50, 15000, 64, cfCreature, 20, 0},
		{cfBanditN, "bandit", 0, 25000, 50, 15060, 192, cfCreature, 20, 0},
		{cfCaptain, "bandit captain", 0, 25070, 50, 15070, 0, cfCreature, 22, 0},
		{cfScout, "bandit scout", 0, 24900, 50, 14900, 32, cfCreature, 19, 0},
		{cfFarBandit, "bandit", 0, 30000, 50, 20000, 0, cfCreature, 20, 0},
		{cfFarBandit2, "bandit", 0, 31000, 50, 21000, 0, cfCreature, 20, 0},
		{cfLord, "Lord Brutus", 0, 25050, 50, 14950, 100, cfCreature, 50, 0},
		{cfWolf, "wolf", 1, 25000, 50, 15000, 0, cfCreature, 10, 0},
	}
	for i := 0; i < 12; i++ {
		rows = append(rows, row{cfTownFolk + i, "townsperson", 0, 26000 + 20*float64(i), 50, 15000, 0, cfHuman, 10, 0x80})
	}
	for i, n := range []string{"one", "two", "three", "four", "five", "six", "seven"} {
		rows = append(rows, row{cfRats + i, "rat " + n, 0, 27000 + 10*float64(i), 50, 17000, 0, cfCreature, 3, 0})
	}
	for i := 0; i < 70; i++ {
		rows = append(rows, row{cfGoblins + i, "goblin raider", 0, 20000 + 8*float64(i%10), 50, 20000 + 8*float64(i/10), 0, cfCreature, 30, 0})
	}
	for i := 0; i < 70; i++ {
		long := fmt.Sprintf("ogre %03d %s", i, strings.Repeat("of the very long name the import gave the Feerrott's ogres ", 5))
		rows = append(rows, row{cfOgres + i, long, 0, 21500 + 8*float64(i%10), 50, 21500 + 8*float64(i/10), 0, cfCreature, 35, 0})
	}
	rows = append(rows, row{cfOgres + 70, "ogre", 0, 21560, 50, 21440, 0, cfCreature, 35, 0})
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1.0, ?, ?, '0', 7)`, r.id, r.name, r.world, r.x, r.y, r.z, r.facing, r.model, r.level, r.ty); err != nil {
			t.Fatal(err)
		}
	}
	c.gw.SetPlayerModels(map[uint32]bool{cfHuman: true})
	c.gw.LoadNPCs(database.NewNPCStore(db))
	c.h.SetCampStore(db)
	c.gw.OnPlayerLeaving(c.h.campfixLeft) // as the registry gives it
	gs, gp := c.join(1, "Gm", 24990, 15000, 0)
	gs.SetAccountID(1)
	ps, pp := c.join(2, "Player", 25000, 15000, 0)
	ps.SetAccountID(2)
	w := &cfWorld{chatWorld: c, db: db, gs: gs, ps: ps, gp: gp, pp: pp, now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	c.h.campfix.now = func() time.Time { return w.now }
	return w
}

// run sends text as s's chat line: what s was told, and what of it came as a
// refusal (red, 0x? error lines), each forgotten.
func (w *cfWorld) run(s *session.Session, text string) (said, refused []string) {
	w.t.Helper()
	w.chatWorld.say(s, text)
	for _, l := range heard(w.t, s) {
		if l.op == protocol.OpcodeErrorMessage {
			refused = append(refused, l.text)
		} else {
			said = append(said, l.text)
		}
	}
	s.Queue().GetPending()
	return said, refused
}

// ok is run that wants no refusal, and the lines.
func (w *cfWorld) ok(s *session.Session, text string) []string {
	w.t.Helper()
	said, refused := w.run(s, text)
	if len(refused) > 0 {
		w.t.Fatalf("%q refused: %q (and %q)", text, refused, said)
	}
	return said
}

// no is run that wants exactly one refusal, want.
func (w *cfWorld) no(s *session.Session, text, want string) {
	w.t.Helper()
	said, refused := w.run(s, text)
	if len(refused) != 1 || refused[0] != want {
		w.t.Fatalf("%q: refused %q (said %q), want %q", text, refused, said, want)
	}
}

// at moves p to (x, 50, z), its facing kept.
func (w *cfWorld) at(p *entity.Player, x, z float32) {
	w.gw.UpdatePosition(p.ServerID, x, 50, z, p.Facing)
}

// face gives p the heading of a 0x40 whose byte 22 is b, where it stands.
func (w *cfWorld) face(p *entity.Player, b byte) {
	buf := make([]byte, 41)
	buf[22] = b
	f, ok := entity.ClientUpdateFacing(buf)
	if !ok {
		w.t.Fatal("no heading")
	}
	w.gw.ApplyClientUpdate(p.ServerID, entity.ClientUpdate{X: p.X, Y: p.Y, Z: p.Z, Facing: f, HasFacing: true})
}

// target makes the NPC of row id p's target, on one of s's entity channels.
func (w *cfWorld) target(s *session.Session, p *entity.Player, id entity.EntityID) {
	vs := entity.GetOrCreateVisibility(s)
	obj := uint32(100000) + uint32(id)
	for i := range vs.ChannelEntity {
		if vs.ChannelEntity[i] == id {
			p.TargetObjectID = vs.ChannelObjectID[i]
			return
		}
	}
	vs.ChannelEntity[w.slot], vs.ChannelObjectID[w.slot] = id, obj
	w.slot++
	p.TargetObjectID = obj
}

// report is campfix_reports' row id.
func (w *cfWorld) report(id int64) database.CampfixReport {
	w.t.Helper()
	r, ok, err := w.db.CampfixReportByID(id)
	if err != nil || !ok {
		w.t.Fatalf("report %d: %v %v", id, ok, err)
	}
	return r
}

func (w *cfWorld) marks(r database.CampfixReport) []CampfixMarkRecord {
	w.t.Helper()
	var out []CampfixMarkRecord
	if err := json.Unmarshal([]byte(r.Marks), &out); err != nil {
		w.t.Fatalf("marks %q: %v", r.Marks, err)
	}
	return out
}

func (w *cfWorld) flags(r database.CampfixReport) []CampfixFlagRecord {
	w.t.Helper()
	var out []CampfixFlagRecord
	if err := json.Unmarshal([]byte(r.Flags), &out); err != nil {
		w.t.Fatalf("flags %q: %v", r.Flags, err)
	}
	return out
}

func (w *cfWorld) near(r database.CampfixReport) []CampfixNearRecord {
	w.t.Helper()
	var out []CampfixNearRecord
	if err := json.Unmarshal([]byte(r.Near), &out); err != nil {
		w.t.Fatalf("near %q: %v", r.Near, err)
	}
	return out
}

var cfSent = regexp.MustCompile(`^Camp report (\d+) sent: `)

// sent is the report a sending's lines name.
func (w *cfWorld) sent(said []string) database.CampfixReport {
	w.t.Helper()
	for _, l := range said {
		if m := cfSent.FindStringSubmatch(l); m != nil {
			id, _ := strconv.ParseInt(m[1], 10, 64)
			return w.report(id)
		}
	}
	w.t.Fatalf("no report sent: %q", said)
	return database.CampfixReport{}
}

func hasFlag(fl []CampfixFlagRecord, mark int, flag string) bool {
	for _, f := range fl {
		if f.Mark == mark && f.Flag == flag {
			return true
		}
	}
	return false
}

// Petition 1423, jibrish: "Verify everyone on the server can use the
// /campfix set of commands. No gm or tester flag needed." A player of
// acctlevel 0, no sub GM and no tester, drives every command end to end in
// every spelling (/name, #name, a PS2's "/t name"): open, mark, add, remove,
// list, undo, note, send, and cancel; nothing it types is refused, each
// spelling's report is filed with its board entry, and none of it goes out as
// a GM's or a tell.
func TestCampfixIsEveryonesInEverySpelling(t *testing.T) {
	w := campfixWorld(t)
	if w.h.isGM(w.ps) || w.h.isSubGM(w.ps) || w.h.granted(w.ps, "#mark") {
		t.Fatal("the player holds a flag")
	}
	w.target(w.ps, w.pp, cfLord)
	var ids []int64
	for i, sp := range []string{"/", "#", "/t "} {
		cmd := func(s string) string { return sp + s }
		if got := w.ok(w.ps, cmd("campfix")); strings.Join(got, "\n") != strings.Join(campfixOpenLines, "\n") {
			t.Fatalf("%s opening: %q", sp, got)
		}
		w.at(w.pp, 25010, 15000)
		if got := w.ok(w.ps, cmd("mark bandit")); len(got) != 1 || got[0] != "Mark 1 of 15: move bandit (spawn 401, 30 away) here, facing your way." {
			t.Fatalf("%s mark: %q", sp, got)
		}
		w.at(w.pp, 25020, 15010)
		if got := w.ok(w.ps, cmd("markadd bandit captain")); len(got) != 1 || got[0] != "Mark 2 of 15: add bandit captain here, facing your way." {
			t.Fatalf("%s markadd: %q", sp, got)
		}
		w.at(w.pp, 25030, 15020)
		if got := w.ok(w.ps, cmd("markremove")); len(got) != 1 || got[0] != "Mark 3 of 15: remove Lord Brutus (spawn 406, 73 from you)." {
			t.Fatalf("%s markremove: %q", sp, got)
		}
		if got := w.ok(w.ps, cmd("campfix list")); len(got) < 4 || got[0] != "Your camp report: 3 of 15 marks." || !strings.HasPrefix(got[3], "  3: remove Lord Brutus (spawn 406)") {
			t.Fatalf("%s list: %q", sp, got)
		}
		if got := w.ok(w.ps, cmd("campfix undo")); len(got) != 1 || got[0] != "Mark 3 dropped: remove Lord Brutus (spawn 406). 2 left." {
			t.Fatalf("%s undo: %q", sp, got)
		}
		w.ok(w.ps, cmd("markremove"))
		if got := w.ok(w.ps, cmd("campfix note the bandits face the wall")); len(got) != 1 || got[0] != "Note kept: the bandits face the wall" {
			t.Fatalf("%s note: %q", sp, got)
		}
		got := w.ok(w.ps, cmd("campfix"))
		r := w.sent(got)
		if want := fmt.Sprintf("Camp report %d sent: 1 move, 1 add, 1 remove, %d creatures nearby recorded. A GM checks every report before anything changes. Thank you!",
			r.ID, len(w.near(r))); len(got) != 1 || got[0] != want {
			t.Fatalf("%s send: %q, want %q", sp, got, want)
		}
		ids = append(ids, r.ID)
		if r.AccountID != 2 || r.CharName != "Player" || r.Status != database.CampfixWaiting || r.Note != "the bandits face the wall" || r.Board == 0 {
			t.Fatalf("%s row: %+v", sp, r)
		}
		e, ok, err := w.db.WorkEntryByID(r.Board)
		if err != nil || !ok || e.CreatedBy != "account 2 (campfix)" || e.Owner != CampfixBoardOwner || e.Status != database.WorkPlanned {
			t.Fatalf("%s board entry: %+v %v %v", sp, e, ok, err)
		}
		// cancel, in the same spelling
		w.ok(w.ps, cmd("campfix"))
		if got := w.ok(w.ps, cmd("campfix cancel")); len(got) != 1 || got[0] != "Camp report discarded: nothing was sent." {
			t.Fatalf("%s cancel: %q", sp, got)
		}
		if got := w.ok(w.ps, "/campfix"); len(got) != len(campfixOpenLines) {
			t.Fatalf("%s: no report open after the cancel: %q", sp, got)
		}
		w.ok(w.ps, "/campfix cancel")
		w.now = w.now.Add(11 * time.Minute) // past the one-every-10-minutes
		if i == 2 && len(ids) != 3 {
			t.Fatalf("reports %v", ids)
		}
	}
	// not one was a GM's mark
	if m, _ := w.db.GMMarks(2); len(m) != 0 {
		t.Fatalf("the player's marks went to gmmarks: %+v", m)
	}
}

// Routing (chat.go): with no report open "/t mark bob" is a tell, a GM's /mark
// keeps a gmmarks row as ever, and anyone else's /mark, #markadd and
// /markremove (and a bare "/t markremove") are told to open a report; while a
// report is open the GM's /mark and #mark are its marks (no gmmarks row) and
// "/t mark ..." is never a tell; once the report is sent or cancelled the
// GM's /mark is its own again. #marks stays the GM's throughout.
func TestCampfixRouting(t *testing.T) {
	w := campfixWorld(t)
	ms, _ := w.join(3, "Mark", 25020, 15000, 0)
	w.ok(w.ps, "/t mark bob")
	if l := texts(heard(t, ms)); !hasLine(l, "Player tells you, 'bob'") {
		t.Fatalf("Mark heard %q", l)
	}
	ms.Queue().GetPending()
	for _, line := range []string{"/mark bandit", "#mark", "#markadd bandit", "/markremove", "/t markremove", "/t mark"} {
		w.no(w.ps, line, "Open a camp report with /campfix first.")
	}
	if got := w.ok(w.gs, "/mark the rock"); len(got) != 1 || got[0] != "[GM] Mark 1: (24990.0, 50.0, 15000.0) in world 0 - the rock" {
		t.Fatalf("the GM's /mark: %q", got)
	}
	if m, _ := w.db.GMMarks(1); len(m) != 1 {
		t.Fatalf("gmmarks %+v", m)
	}

	w.ok(w.gs, "/campfix")
	w.at(w.gp, 25035, 15000)
	if got := w.ok(w.gs, "/mark bandit"); len(got) != 1 || got[0] != "Mark 1 of 15: move bandit (spawn 401, 5 away) here, facing your way." {
		t.Fatalf("the GM's /mark in a report: %q", got)
	}
	w.at(w.gp, 25000, 15040)
	if got := w.ok(w.gs, "#mark bandit"); len(got) != 1 || got[0] != "Mark 2 of 15: move bandit (spawn 402, 20 away) here, facing your way." {
		t.Fatalf("the GM's #mark in a report: %q", got)
	}
	if got := w.ok(w.gs, "#marks"); len(got) < 1 || got[0] != "[GM] Your marks (1), the first made first:" {
		t.Fatalf("#marks in a report: %q", got)
	}
	if m, _ := w.db.GMMarks(1); len(m) != 1 {
		t.Fatalf("a report's marks went to gmmarks: %+v", m)
	}

	w.ok(w.ps, "/campfix")
	w.no(w.ps, "/t mark bob", "No creature named bob here: target it to see its name.")
	if l := heard(t, ms); len(l) != 0 {
		t.Fatalf("a report's /t mark was a tell: %+v", l)
	}

	w.ok(w.gs, "/campfix cancel")
	w.ok(w.gs, "/mark")
	if m, _ := w.db.GMMarks(1); len(m) != 2 {
		t.Fatalf("the GM's /mark after its report: %+v", m)
	}
}

// The rules of a report's marks: at most 15; one camp (within 300 of the first
// mark, in its world); none within 2 of another; none dead, flying or loading
// a world. A name's marks never outnumber its spawns near: two moves of a name
// with two spawns near take both and a third is refused, whatever order the
// claims come in. The target picks the spawn, and the open moves take the
// least distance between them.
func TestCampfixMarkRules(t *testing.T) {
	w := campfixWorld(t)
	w.ok(w.ps, "/campfix")
	for i := 0; i < CampfixMostMarks; i++ {
		w.at(w.pp, 24960+3*float32(i), 15100)
		if got := w.ok(w.ps, "/markadd bandit"); got[0] != fmt.Sprintf("Mark %d of 15: add bandit here, facing your way.", i+1) {
			t.Fatalf("add %d: %q", i+1, got)
		}
	}
	w.at(w.pp, 25100, 15100)
	w.no(w.ps, "/markadd bandit", "15 marks is the most: /campfix sends them.")
	w.ok(w.ps, "/campfix cancel")

	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25000, 15000)
	w.ok(w.ps, "/markadd bandit")
	w.at(w.pp, 25400, 15000)
	w.no(w.ps, "/markadd bandit", "A camp report covers one camp: you are 400 from your first mark.")
	w.at(w.pp, 25001, 15000)
	w.no(w.ps, "/markadd bandit", "Mark 1 is within 2 of here: a mark stands 2 from every other.")
	w.at(w.pp, 25010, 15000)
	w.pp.World = 1
	w.no(w.ps, "/markadd wolf", "A camp report covers one camp: your first mark is in another world.")
	w.pp.World = 0
	hp := w.pp.HP
	w.pp.HP = 0
	w.no(w.ps, "/markadd bandit", "You cannot mark while you are dead.")
	w.pp.HP = hp
	w.pp.Flying = true
	w.no(w.ps, "/markadd bandit", "You cannot mark while you fly: land where the creature should stand.")
	w.pp.Flying = false
	w.ps.HoldStateChannels(time.Minute)
	w.no(w.ps, "/markadd bandit", "Wait for the world to load before you mark.")
	w.ps.ReleaseStateChannels()
	w.ok(w.ps, "/markadd bandit")
	w.ok(w.ps, "/campfix cancel")

	// claims: two bandits near, two moves take both, a third is refused
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25030, 15000)
	if got := w.ok(w.ps, "/mark bandit"); got[0] != "Mark 1 of 15: move bandit (spawn 401, 10 away) here, facing your way." {
		t.Fatalf("the first move: %q", got)
	}
	w.at(w.pp, 25000, 15050)
	if got := w.ok(w.ps, "/mark bandit"); got[0] != "Mark 2 of 15: move bandit (spawn 402, 10 away) here, facing your way." {
		t.Fatalf("the second move: %q", got)
	}
	w.at(w.pp, 25020, 15030)
	w.no(w.ps, "/mark bandit", "Every bandit near here already has a mark: /markadd for another.")
	w.target(w.ps, w.pp, cfBanditE) // a claim of one an open move needs
	w.no(w.ps, "/mark", "Every bandit near here already has a mark: /markadd for another.")
	w.no(w.ps, "/markremove", "Every bandit near here already has a mark: /campfix undo drops the last.")
	w.ok(w.ps, "/campfix cancel")

	// the target picks
	w.ok(w.ps, "/campfix")
	w.target(w.ps, w.pp, cfBanditN)
	w.at(w.pp, 25010, 15000)
	if got := w.ok(w.ps, "/mark"); got[0] != "Mark 1 of 15: move bandit (spawn 402, 61 away) here, facing your way." {
		t.Fatalf("the target's move: %q", got)
	}
	w.at(w.pp, 25020, 15020)
	w.no(w.ps, "/mark bandit", "bandit (spawn 402) already has a mark: mark 1.")
	w.ok(w.ps, "/campfix cancel")

	// the least distance: the first open move is nearest 401 alone, the
	// second nearer it still; sent, the first takes 402
	w.pp.TargetObjectID = 0xFFFFFFFF
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25020, 15000)
	if got := w.ok(w.ps, "/mark bandit"); got[0] != "Mark 1 of 15: move bandit (spawn 401, 20 away) here, facing your way." {
		t.Fatalf("alone: %q", got)
	}
	w.at(w.pp, 25035, 15000)
	if got := w.ok(w.ps, "/mark bandit"); got[0] != "Mark 2 of 15: move bandit (spawn 401, 5 away) here, facing your way." {
		t.Fatalf("nearer: %q", got)
	}
	if got := w.ok(w.ps, "/campfix list"); !strings.HasPrefix(got[1], "  1: move bandit (spawn 402, picked when sent)") ||
		!strings.HasPrefix(got[2], "  2: move bandit (spawn 401, picked when sent)") {
		t.Fatalf("the list: %q", got)
	}
	m := w.marks(w.sent(w.ok(w.ps, "/campfix")))
	if len(m) != 2 || m[0].Row != cfBanditN || m[1].Row != cfBanditE {
		t.Fatalf("sent: %+v", m)
	}

	// /markremove by the target, then by where the player stands; refused on a
	// row already marked, and with neither
	w.now = w.now.Add(11 * time.Minute)
	w.ok(w.ps, "/campfix")
	w.target(w.ps, w.pp, cfBanditN)
	w.at(w.pp, 25000, 15010)
	if got := w.ok(w.ps, "/markremove"); got[0] != "Mark 1 of 15: remove bandit (spawn 402, 50 from you)." {
		t.Fatalf("by the target: %q", got)
	}
	w.at(w.pp, 25010, 15010)
	w.no(w.ps, "/markremove", "bandit (spawn 402) already has a mark: mark 1.")
	w.pp.TargetObjectID = 0xFFFFFFFF
	w.at(w.pp, 25035, 15005)
	if got := w.ok(w.ps, "/markremove"); got[0] != "Mark 2 of 15: remove bandit (spawn 401, 7 from you)." {
		t.Fatalf("by the spawn stood on: %q", got)
	}
	w.at(w.pp, 25200, 15200)
	w.no(w.ps, "/markremove", "Stand on the creature's spawn, or target it.")
	w.at(w.pp, 25020, 15020)
	w.target(w.ps, w.pp, cfWolf) // 28 off, but in world 1: no target of this world's
	w.no(w.ps, "/markremove", "Stand on the creature's spawn, or target it.")
}

// The facing. After a 0x40 with heading byte b a mark keeps facing_byte
// (b+128) mod 256, for a move, a turn, an add and a remove, and a move's and a
// turn's from_facing is its row's byte; a /mark within 2 of its spawn is a
// turn. For every b of the 256: the player's own byte 15 is (b+128) mod 256,
// the mark's byte is too, and a creature applied from it (its row's facing
// set to that byte, then loaded) sends that same byte 15.
func TestCampfixFacing(t *testing.T) {
	w := campfixWorld(t)
	byteOf := regexp.MustCompile(`facing (\d+)$`)
	w.at(w.pp, 25010, 15020)
	for b := 0; b < 256; b++ {
		want := (b + 128) % 256
		w.face(w.pp, byte(b))
		if got := entity.SerializePlayerUpdate(1, w.pp)[15]; int(got) != want {
			t.Fatalf("heading byte %d: the player's byte 15 is %d, want %d", b, got, want)
		}
		w.ok(w.ps, "/campfix")
		w.ok(w.ps, "/markadd bandit")
		l := w.ok(w.ps, "/campfix list")
		m := byteOf.FindStringSubmatch(l[1])
		if m == nil || m[1] != strconv.Itoa(want) {
			t.Fatalf("heading byte %d: the mark %q, want facing %d", b, l[1], want)
		}
		rec := database.NPCRecord{ID: 9000, NPCName: "bandit", XCoord: 25010, YCoord: 50, ZCoord: 15020, Facing: want, NPCLevel: 20, ModelID: cfCreature, Size: 1}
		if got := entity.SerializeNPCUpdate(1, entity.NPCFromRecord(&rec, 0, nil))[15]; int(got) != want {
			t.Fatalf("heading byte %d: a creature of row byte %d sends %d", b, want, got)
		}
		w.ok(w.ps, "/campfix cancel")
	}
	// one through the database and the live edit, as a review applies it
	if _, err := w.db.Exec(`UPDATE npcs SET facing = 255 WHERE id = ?`, cfCaptain); err != nil {
		t.Fatal(err)
	}
	rec, err := database.NewNPCStore(w.db).GetNPC(cfCaptain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.gw.UpsertNPC(rec, 0); err != nil {
		t.Fatal(err)
	}
	if got := entity.SerializeNPCUpdate(1, w.gw.NPCs[cfCaptain])[15]; got != 255 {
		t.Fatalf("row byte 255 loaded sends %d", got)
	}

	// the four kinds in one report, sent
	w.ok(w.ps, "/campfix")
	w.face(w.pp, 10)
	w.at(w.pp, 25020, 15000)
	w.ok(w.ps, "/mark bandit") // an open move: 401, once 402 is the turn's
	w.face(w.pp, 77)
	w.target(w.ps, w.pp, cfBanditN)
	w.at(w.pp, 25000, 15059)
	if got := w.ok(w.ps, "/mark"); got[0] != "Mark 2 of 15: turn bandit (spawn 402) to face your way." {
		t.Fatalf("within 2 of its spawn: %q", got)
	}
	w.face(w.pp, 200)
	w.at(w.pp, 25060, 15030)
	w.ok(w.ps, "/markadd bandit captain")
	w.face(w.pp, 255)
	w.target(w.ps, w.pp, cfLord)
	w.at(w.pp, 25040, 14960)
	w.ok(w.ps, "/markremove")
	if got := w.ok(w.ps, "/campfix list"); got[2] != "  2: turn bandit (spawn 402) at (25000.0, 50.0, 15059.0) to face 205; it faced 192" {
		t.Fatalf("the turn listed: %q", got)
	}
	m := w.marks(w.sent(w.ok(w.ps, "/campfix")))
	want := []struct {
		kind       string
		b, row     int
		fromFacing int
	}{{campfixMove, 10, cfBanditE, 64}, {campfixTurn, 77, cfBanditN, 192}, {campfixAdd, 200, 0, 255}, {campfixRemove, 255, cfLord, 100}}
	if len(m) != len(want) {
		t.Fatalf("marks %+v", m)
	}
	for i, x := range want {
		if m[i].Kind != x.kind || m[i].FacingByte != (x.b+128)%256 || m[i].Row != x.row || m[i].FromFacing != x.fromFacing {
			t.Errorf("mark %d: %+v, want %s byte %d row %d from %d", i+1, m[i], x.kind, (x.b+128)%256, x.row, x.fromFacing)
		}
	}
	if m[2].Template != cfCaptain {
		t.Errorf("the add's template %d, want the captain's row", m[2].Template)
	}
}

// Names: matched without case, the whole text, else without a leading "a ",
// "an " or "the ", else the one name near holding it ("Taken as ..."); several
// holding it are listed (up to five) and refused; none in the player's world
// is said so (the wolf of world 1 included).
func TestCampfixNames(t *testing.T) {
	w := campfixWorld(t)
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	if got := w.ok(w.ps, "/mark a bandit"); got[0] != "Mark 1 of 15: move bandit (spawn 401, 30 away) here, facing your way." {
		t.Fatalf("a bandit: %q", got)
	}
	w.at(w.pp, 25010, 15010)
	if got := w.ok(w.ps, "/markadd The Bandit Captain"); len(got) != 1 || got[0] != "Mark 2 of 15: add bandit captain here, facing your way." {
		t.Fatalf("The Bandit Captain: %q", got)
	}
	w.at(w.pp, 25010, 15020)
	if got := w.ok(w.ps, "/markadd capt"); len(got) != 2 || got[0] != "Taken as bandit captain." || got[1] != "Mark 3 of 15: add bandit captain here, facing your way." {
		t.Fatalf("capt: %q", got)
	}
	w.at(w.pp, 25040, 15040)
	w.no(w.ps, "/markadd band", `"band" could be bandit, bandit captain or bandit scout: type more of the name.`)
	w.no(w.ps, "/markadd zebra", "No creature named zebra here: target it to see its name.")
	w.no(w.ps, "/markadd wolf", "No creature named wolf here: target it to see its name.")
	w.no(w.ps, "/mark", "Target the creature, or name it: /mark <name>.")
	w.no(w.ps, "/markadd", "Name the creature to add: /markadd <name>.")
	w.no(w.ps, "/mark townsperson", "No townsperson near here to move: /markadd adds one.")
	if got := w.ok(w.ps, "/markadd townsperson"); got[0] != "Mark 4 of 15: add townsperson here, facing your way. (none near here: a GM will check)" {
		t.Fatalf("an add of none near: %q", got)
	}
	w.ok(w.ps, "/campfix cancel")

	w.at(w.pp, 27030, 17010)
	w.ok(w.ps, "/campfix")
	w.no(w.ps, "/markadd rat", `"rat" could be rat five, rat four, rat one, rat seven, rat six or 2 more: type more of the name.`)
	if got := w.ok(w.ps, "/markadd sev"); got[0] != "Taken as rat seven." {
		t.Fatalf("sev: %q", got)
	}
}

// Sending: the table's row and the board entry (owner the GMs, status
// planned, area data, touches npcs, a title of at most 200, details under
// 20,000 with 15 marks and 60 creatures near, in the plan's order), the row
// read-only to the world: nothing is changed by it. Too many long names near
// cut the near table, and the details say so.
func TestCampfixSendsARowAndABoardEntry(t *testing.T) {
	w := campfixWorld(t)
	before := w.gw.NPCs[cfGoblins].X
	w.ok(w.ps, "/campfix")
	for i := 0; i < CampfixMostMarks; i++ {
		w.at(w.pp, 20000+3*float32(i), 20030)
		w.face(w.pp, byte(16*i))
		w.ok(w.ps, "/markadd goblin raider")
	}
	w.ok(w.ps, "/campfix note goblins stand in a square, the guide puts them by the fire")
	got := w.ok(w.ps, "/campfix")
	r := w.sent(got)
	if want := fmt.Sprintf("Camp report %d sent: 15 adds, 60 creatures nearby recorded. A GM checks every report before anything changes. Thank you!", r.ID); got[0] != want {
		t.Fatalf("sent: %q", got)
	}
	if r.World != 0 || r.X != 20000 || r.Z != 20030 || r.Level != 1 || r.Class != 0 || r.Created != "2026-11-10T12:00:00Z" {
		t.Fatalf("the row: %+v", r)
	}
	if n := w.near(r); len(n) != 60 || n[0].Name != "goblin raider" || !n[0].SameName || n[0].Alive == nil || !*n[0].Alive || n[0].Movement == "" {
		t.Fatalf("near: %d %+v", len(n), n[:1])
	}
	if m := w.marks(r); len(m) != 15 || m[3].FacingByte != (48+128)%256 || m[0].Template == 0 || m[0].At != "2026-11-10T12:00:00Z" {
		t.Fatalf("marks: %+v", m[:4])
	}
	e, ok, err := w.db.WorkEntryByID(r.Board)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if e.Owner != "GMs (Camp reports page)" || e.Status != database.WorkPlanned || e.Area != "data" || strings.Join(e.Touches, ",") != "npcs" {
		t.Fatalf("the entry: %+v", e)
	}
	if !strings.HasPrefix(e.Title, fmt.Sprintf("Camp fix report %d: Player (L1 warrior), Tunaria ", r.ID)) || !strings.HasSuffix(e.Title, ": 15 add") ||
		utf8.RuneCountInString(e.Title) > 200 {
		t.Fatalf("the title: %q", e.Title)
	}
	d := e.Details
	if n := utf8.RuneCountInString(d); n >= 20000 {
		t.Fatalf("details of %d characters", n)
	}
	order := []string{"A GM approves or declines it on the GM panel's Camp reports page", "nothing is applied before that, and no session applies one itself.",
		"**Who:** Player, a level 1 Warrior (character 2, account 2)", "**Where:** Tunaria", "**When:** 2026-11-10 12:00:00 UTC",
		"**Build:** ", "**shard:** Marr's Fist", "> goblins stand in a square, the guide puts them by the fire",
		"### Marks", "| 15 | add | goblin raider |", "### Flags", "### Nearby", "60 NPCs of rows"}
	at := 0
	for _, part := range order {
		i := strings.Index(d[at:], part)
		if i < 0 {
			t.Fatalf("details lack %q after character %d:\n%s", part, at, d)
		}
		at += i
	}
	if strings.Count(d, "| goblin raider | 30 |") != 60 || strings.Contains(d, "The table is cut") {
		t.Fatalf("not every one of the 60 near is in the details")
	}
	var audit int
	if err := w.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'workboard' AND via = 'campfix' AND accountid = 2`).Scan(&audit); err != nil || audit != 1 {
		t.Fatalf("the board entry's audit lines: %d %v", audit, err)
	}
	if w.gw.NPCs[cfGoblins].X != before {
		t.Fatal("a spawn moved")
	}

	// too much to say: seventy ogres of long names near cut the table
	w.now = w.now.Add(11 * time.Minute)
	w.ok(w.ps, "/campfix")
	for i := 0; i < CampfixMostMarks; i++ {
		w.at(w.pp, 21500+4*float32(i), 21530)
		w.ok(w.ps, "/markadd ogre")
	}
	r = w.sent(w.ok(w.ps, "/campfix"))
	e, _, _ = w.db.WorkEntryByID(r.Board)
	if n := utf8.RuneCountInString(e.Details); n >= 20000 || !strings.Contains(e.Details, "(The table is cut: ") || len(w.near(r)) != 60 {
		t.Fatalf("details of %d characters, cut %v, %d near kept", n, strings.Contains(e.Details, "The table is cut"), len(w.near(r)))
	}
}

// The limits, each refusal keeping the report open: nothing to send; one
// report an account every 10 minutes; 3 of an account's waiting; 100 waiting
// in all (the GM's own report, while 100 wait).
func TestCampfixSendingLimits(t *testing.T) {
	w := campfixWorld(t)
	t0 := w.now
	w.ok(w.ps, "/campfix")
	w.no(w.ps, "/campfix", "Nothing to send: /campfix cancel closes it.")
	mark := func(x float32) {
		w.at(w.pp, x, 15000)
		w.ok(w.ps, "/markadd bandit")
	}
	mark(25010)
	w.sent(w.ok(w.ps, "/campfix"))

	w.now = t0.Add(4 * time.Minute)
	w.ok(w.ps, "/campfix")
	mark(25012)
	w.no(w.ps, "/campfix", "One camp report every 10 minutes: 6 minutes to go.")
	if got := w.ok(w.ps, "/campfix list"); got[0] != "Your camp report: 1 of 15 marks." {
		t.Fatalf("kept open: %q", got)
	}
	w.now = t0.Add(10*time.Minute - 30*time.Second)
	w.no(w.ps, "/campfix", "One camp report every 10 minutes: 1 minute to go.")
	w.now = t0.Add(10 * time.Minute)
	w.sent(w.ok(w.ps, "/campfix"))

	w.now = t0.Add(20 * time.Minute)
	w.ok(w.ps, "/campfix")
	mark(25014)
	third := w.sent(w.ok(w.ps, "/campfix"))
	w.now = t0.Add(30 * time.Minute)
	w.ok(w.ps, "/campfix")
	mark(25016)
	w.no(w.ps, "/campfix", "You have 3 camp reports waiting for a GM.")
	if _, err := w.db.Exec(`UPDATE campfix_reports SET status = 'applied', review = 'moved' WHERE id = ?`, third.ID); err != nil {
		t.Fatal(err)
	}
	if r := w.report(third.ID); r.Reviewed == "" {
		t.Fatal("a review's status left reviewed empty")
	}
	w.sent(w.ok(w.ps, "/campfix"))

	for i := 0; i < 97; i++ {
		if _, err := w.db.Exec(`INSERT INTO campfix_reports (accountid, charname, created) VALUES (99, 'Other', ?)`, t0.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	w.ok(w.gs, "/campfix")
	w.at(w.gp, 25005, 15030)
	w.ok(w.gs, "/markadd bandit")
	w.no(w.gs, "/campfix", "The GMs have 100 camp reports to read: try again later.")
	if _, err := w.db.Exec(`UPDATE campfix_reports SET status = 'declined' WHERE accountid = 99 AND id = (SELECT MIN(id) FROM campfix_reports WHERE accountid = 99)`); err != nil {
		t.Fatal(err)
	}
	w.sent(w.ok(w.gs, "/campfix"))
	if _, err := w.db.Exec(`UPDATE campfix_reports SET status = 'later' WHERE id = 1`); err == nil {
		t.Fatal("a status of no review's taken")
	}
}

// A logout discards a report (the leaving hook, and a new character of the
// session either way), and so do 30 minutes with no command, which the next
// command says; a death keeps it.
func TestCampfixDiscards(t *testing.T) {
	w := campfixWorld(t)
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	w.ok(w.ps, "/markadd bandit")
	w.gw.RemovePlayer(w.pp.ServerID)
	if len(w.h.campfix.open) != 0 {
		t.Fatal("the report outlived its logout")
	}
	w.pp = w.gw.CreatePlayer(w.ps, &database.CharacterRecord{ServerID: 2, CharName: "Player", X: 25010, Y: 50, Z: 15000})
	w.no(w.ps, "/campfix list", "You have no camp report open: /campfix opens one.")
	w.no(w.ps, "/markadd bandit", "Open a camp report with /campfix first.")

	// another character on the session, with no hook between
	w.ok(w.ps, "/campfix")
	w.ok(w.ps, "/markadd bandit")
	w.pp = w.gw.CreatePlayer(w.ps, &database.CharacterRecord{ServerID: 2, CharName: "Player", X: 25010, Y: 50, Z: 15000})
	w.no(w.ps, "/campfix list", "You have no camp report open: /campfix opens one.")

	// 29 idle minutes keep it, 31 more lose it
	w.ok(w.ps, "/campfix")
	w.ok(w.ps, "/markadd bandit")
	w.now = w.now.Add(29 * time.Minute)
	if got := w.ok(w.ps, "/campfix list"); got[0] != "Your camp report: 1 of 15 marks." {
		t.Fatalf("after 29 minutes: %q", got)
	}
	w.now = w.now.Add(31 * time.Minute)
	w.no(w.ps, "/mark bandit", "Your camp report was discarded: 30 minutes passed with no command. /campfix opens a new one.")
	w.no(w.ps, "/campfix list", "You have no camp report open: /campfix opens one.")

	// a death keeps it
	w.ok(w.ps, "/campfix")
	w.ok(w.ps, "/markadd bandit")
	hp := w.pp.HP
	w.pp.HP = 0
	if got := w.ok(w.ps, "/campfix list"); got[0] != "Your camp report: 1 of 15 marks." {
		t.Fatalf("dead: %q", got)
	}
	w.pp.HP = hp
}

// The note: control characters dropped, at most 200 characters, profanity
// masked, a second replacing the first; the row keeps it so.
func TestCampfixNote(t *testing.T) {
	w := campfixWorld(t)
	w.no(w.ps, "/campfix note hello", "You have no camp report open: /campfix opens one.")
	w.ok(w.ps, "/campfix")
	w.no(w.ps, "/campfix note", "Usage: /campfix note <text> - a note for the GMs, 200 characters at most.")
	if got := w.ok(w.ps, "/campfix note the shit\x01 bandits\tface the wall"); got[0] != "Note kept: the **** banditsface the wall" {
		t.Fatalf("masked: %q", got)
	}
	long := strings.Repeat("abcd ", 50)
	if got := w.ok(w.ps, "/campfix note "+long); got[0] != "Note kept: "+strings.TrimSpace(long[:200])+" (cut to 200 characters)" {
		t.Fatalf("long: %q", got)
	}
	w.ok(w.ps, "/campfix note the shit bandits face the wall")
	w.at(w.pp, 25010, 15000)
	w.ok(w.ps, "/markadd bandit")
	if r := w.sent(w.ok(w.ps, "/campfix")); r.Note != "the **** bandits face the wall" {
		t.Fatalf("the row's note %q", r.Note)
	}
}

// world.campfix false: every command answers that the reports are off; a
// GM's own /mark is as ever, and "/t mark bob" still a tell.
func TestCampfixOff(t *testing.T) {
	w := campfixWorld(t)
	SetCampfix(false)
	t.Cleanup(func() { SetCampfix(true) })
	for _, line := range []string{"/campfix", "#campfix list", "/t campfix", "/mark bandit", "#markadd bandit", "/t markremove"} {
		w.no(w.ps, line, "Camp reports are off on this server.")
	}
	w.no(w.ps, "/t mark bob", "mark is not connected.")
	if got := w.ok(w.gs, "/mark"); len(got) != 1 || !strings.HasPrefix(got[0], "[GM] Mark 1: ") {
		t.Fatalf("the GM's /mark: %q", got)
	}
}

// The flags (the code reviews nothing: it flags): a name of 3 spawns or fewer
// in the world, a name with none within 300 of the marks, a level more than 8
// off the median near, a move of more than 150; a creature that attacks a
// level 1 on sight placed within 150 of a town or a start spot; a mark more
// than 3 above the ground's floor, and a land creature's in the water.
func TestCampfixFlags(t *testing.T) {
	w := campfixWorld(t)
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25010, 15000)
	w.ok(w.ps, "/mark Lord Brutus")    // 1: few, and level 50 among bandits
	w.at(w.pp, 25080, 14990)           // 201 from the scout's spawn
	w.ok(w.ps, "/mark bandit scout")   // 2: few, a move of 201
	w.at(w.pp, 25000, 15030)           //
	w.ok(w.ps, "/markadd townsperson") // 3: none near
	w.at(w.pp, 25020, 15030)           //
	w.ok(w.ps, "/markadd bandit")      // 4: nothing
	fl := w.flags(w.sent(w.ok(w.ps, "/campfix")))
	for _, want := range []struct {
		mark int
		flag string
	}{{1, "few"}, {1, "level"}, {2, "few"}, {2, "far_move"}, {3, "none_near"}} {
		if !hasFlag(fl, want.mark, want.flag) {
			t.Errorf("no %s on mark %d: %+v", want.flag, want.mark, fl)
		}
	}
	for _, f := range fl {
		if f.Mark == 4 || f.Flag == "aggro" || f.Flag == "above_ground" || f.Flag == "in_water" {
			t.Errorf("a flag not raised: %+v", f)
		}
	}

	// a bandit (it attacks a level 1 on sight) added by the town, and by a
	// start spot; the town's ground is 40 round its people, the flag's 150 past it
	if _, err := w.db.Exec(`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		VALUES (0, 0, 1, 1, 0, 0, 0, 0, 24000, 50, 15000, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	for i, at := range []struct {
		x     float32
		where string
	}{{25820, "a town"}, {24100, "a new human warrior's start spot"}, {25700, ""}} {
		w.now = w.now.Add(11 * time.Minute)
		w.ok(w.ps, "/campfix")
		w.at(w.pp, at.x, 15000)
		w.ok(w.ps, "/markadd bandit")
		r := w.sent(w.ok(w.ps, "/campfix"))
		w.db.Exec(`UPDATE campfix_reports SET status = 'applied' WHERE id = ?`, r.ID)
		fl := w.flags(r)
		switch {
		case at.where == "" && hasFlag(fl, 1, "aggro"):
			t.Errorf("%d: flagged at %v: %+v", i, at.x, fl)
		case at.where != "" && !hasFlag(fl, 1, "aggro"):
			t.Errorf("%d: no aggro flag at %v: %+v", i, at.x, fl)
		case at.where != "" && !strings.HasSuffix(fl[len(fl)-1].Text, "within 150 of "+at.where):
			t.Errorf("%d: %+v", i, fl)
		}
	}

	// the ground under the marks at 40 and water east of 25050
	w.gw.SetGround(cfGround{})
	w.gw.SetEnv(cfWater{})
	w.now = w.now.Add(11 * time.Minute)
	w.ok(w.ps, "/campfix")
	w.at(w.pp, 25030, 15040)
	w.ok(w.ps, "/markadd bandit") // 1: 10 above the floor
	w.at(w.pp, 25060, 15040)
	w.ok(w.ps, "/markadd bandit") // 2: and in the water
	fl = w.flags(w.sent(w.ok(w.ps, "/campfix")))
	if !hasFlag(fl, 1, "above_ground") || hasFlag(fl, 1, "in_water") || !hasFlag(fl, 2, "in_water") {
		t.Fatalf("the ground's flags: %+v", fl)
	}
}

// cfGround's floor is 40 everywhere; cfWater's water lies east of x 25050.
type cfGround struct{}

func (cfGround) Floor(world uint8, x, y, z float32) (float32, bool) { return 40, true }

type cfWater struct{}

func (cfWater) Flags(world uint8, x, y, z float32) (uint32, bool) {
	if x > 25050 {
		return 1, true
	}
	return 0, true
}
