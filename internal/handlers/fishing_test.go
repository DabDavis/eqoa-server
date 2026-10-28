package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Fishing against a fixture: a character wearing a pole (wear 20, 1000 hit
// points) in its hand and three baits (wear 21), a catch table, the World's
// view of the angler stood in for, the dice scripted and the bite's timer
// run by hand. The items' icons are made up.

type fishFixture struct {
	t      *testing.T
	db     *database.SQLiteDB
	inv    *database.InventoryStore
	chars  *database.SQLiteCharacterStore
	h      *FishingHandler
	sess   *session.Session
	player *entity.Player
	sid    uint32
	view   entity.FishingView
	dice   []int
	timer  func()

	pole, bait, carp, boot int // item patterns
}

func newFishFixture(t *testing.T) *fishFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/fish.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fishFixture{t: t, db: db, inv: database.NewInventoryStore(db), chars: database.NewSQLiteCharacterStore(db)}
	rec := &database.CharacterRecord{CharName: "Angler", AccountID: 1}
	if err := f.chars.Create(rec); err != nil {
		t.Fatal(err)
	}
	f.sid = rec.ServerID
	f.player = &entity.Player{ServerID: 1001, CharServerID: f.sid, Name: "Angler", Level: 10}
	f.sess = testSession()
	f.view = entity.FishingView{Name: "Angler", CharServerID: f.sid, Level: 10, Water: entity.FishWaterThere,
		X: 25400, Y: 54, Z: 15816, LandX: 25400, LandZ: 15826}
	f.h = NewFishingHandler(db, f.chars, f.inv)
	f.h.view = func(*session.Session, time.Time) (entity.FishingView, bool) { return f.view, f.view.Name != "" }
	f.h.playerOf = func(*session.Session) *entity.Player { return f.player }
	f.h.now = func() time.Time { return time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC) }
	f.h.rnd = func(n int) int {
		if len(f.dice) == 0 {
			t.Fatalf("the dice ran out (rnd(%d))", n)
		}
		v := f.dice[0]
		f.dice = f.dice[1:]
		if v >= n {
			t.Fatalf("a scripted throw %d for rnd(%d)", v, n)
		}
		return v
	}
	f.h.after = func(_ time.Duration, fn func()) *time.Timer { f.timer = fn; return nil }

	f.pole = f.pattern("Fishing Pole", 20, 1, 1000)
	f.bait = f.pattern("can of worms", 21, 99, 0)
	f.carp = f.pattern("carp", -1, 50, 0)
	f.boot = f.pattern("old boot", -1, 1, 0)
	f.wear(f.pole, 1, 14)
	f.wear(f.bait, 3, 15)
	return f
}

// pattern adds an item: worn at wear (-1 nowhere), stacking to maxStack.
func (f *fishFixture) pattern(name string, wear, maxStack, maxHP int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1511586099, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, ?, ?, 50, 32767, 1023, 2, 0, 0, 0, ?, '', 0, 0)`,
		wear, maxStack, maxHP, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// wear gives the character n of an item, worn at equip slot loc (-1: carried),
// at full condition.
func (f *fishFixture) wear(pattern, n, loc int) {
	f.t.Helper()
	p, err := database.NewItemPatternStore(f.db).GetItemPattern(pattern)
	if err != nil {
		f.t.Fatal(err)
	}
	id, err := f.inv.AddCharItem(f.sid, pattern, n, p.MaxHP, 0, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	if loc >= 0 {
		if err := f.inv.SetEquipLoc(f.sid, id, loc); err != nil {
			f.t.Fatal(err)
		}
	}
}

// catches adds a fishcatch row for every world, anywhere.
func (f *fishFixture) catches(item, count, weight, minSkill int) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO fishcatch (itemid, count, weight, minskill) VALUES (?, ?, ?, ?)`,
		item, count, weight, minSkill); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fishFixture) loadList() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvList(rows))
}

// sent is what the handler queued since the last call.
func (f *fishFixture) sent() [][]byte {
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// reply sends 0x0F01: a sub-op and its words.
func (f *fishFixture) reply(sub byte, words ...uint32) [][]byte {
	f.t.Helper()
	payload := build(OpcodeFishingReply, 16, func(w *buffer.Writer) {
		w.WriteByte(sub)
		for _, v := range words {
			w.WriteUint32(v)
		}
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, OpcodeFishingReply)
	msg.SetData(payload)
	if err := f.h.HandleFishing(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	return f.sent()
}

// start opens a session and checks the window was asked for.
func (f *fishFixture) start() {
	f.t.Helper()
	if !f.h.StartFishing(f.sess) {
		f.t.Fatal("no session")
	}
	msgs := f.sent()
	mustOps(f.t, msgs, OpcodeFishing)
	if !bytes.Equal(msgs[0], []byte{0x02, 0x0F, fishStart}) {
		f.t.Fatalf("Start % X", msgs[0])
	}
}

// bite runs the cast's timer.
func (f *fishFixture) bite() [][]byte {
	f.t.Helper()
	if f.timer == nil {
		f.t.Fatal("no cast in the water")
	}
	fn := f.timer
	f.timer = nil
	fn()
	return f.sent()
}

func (f *fishFixture) rows() []database.CharInventoryEntry {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *fishFixture) held(pattern int) (n, records int) {
	for _, r := range f.rows() {
		if r.Pattern.PatternID == pattern {
			n += r.StackLeft
			records++
		}
	}
	return n, records
}

func (f *fishFixture) skill() int {
	f.t.Helper()
	s, err := f.chars.FishingSkill(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func sub0F02(m []byte) byte {
	if len(m) < 3 || opOf(m) != OpcodeFishing {
		return 0
	}
	return m[2]
}

// A session needs a pole in the hand: said in red, and no window asked
// for, without one (carried is not worn), dead or fighting; with one, Start.
func TestFishingStartNeedsAPoleInTheHand(t *testing.T) {
	f := newFishFixture(t)
	f.loadList()
	f.start()

	g := newFishFixture(t)
	g.db.Exec(`UPDATE charinventory SET equiploc = -1 WHERE patternid = ?`, g.pole)
	if g.h.StartFishing(g.sess) {
		t.Error("a session with the pole carried")
	}
	msgs := g.sent()
	mustOps(t, msgs, protocol.OpcodeColoredChat)
	if !bytes.Contains(msgs[0], utf16le(fishNoPole)) {
		t.Errorf("no pole: % X", msgs[0])
	}
	for _, c := range []struct {
		dead, fighting bool
		line           string
	}{{true, false, fishDead}, {false, true, fishFighting}} {
		h := newFishFixture(t)
		h.view.Dead, h.view.Fighting = c.dead, c.fighting
		if h.h.StartFishing(h.sess) {
			t.Errorf("a session dead %v fighting %v", c.dead, c.fighting)
		}
		msgs := h.sent()
		mustOps(t, msgs, protocol.OpcodeColoredChat)
		if !bytes.Contains(msgs[0], utf16le(c.line)) {
			t.Errorf("dead %v fighting %v: % X", c.dead, c.fighting, msgs[0])
		}
	}
}

// A perfect cast that bites brings up the catch two to five seconds later:
// nothing at the cast itself, then one bait off its stack (0x003C at the
// bait's list index) and the catch list (0x0F02 7: zigzag 1, the counter
// 1, the record keyed 0). Keep files it as CLIENT's Caught does (Caught(0,
// 3): a new record under the list's counter) and the empty list closes (6).
func TestFishingCastCatchKeep(t *testing.T) {
	f := newFishFixture(t)
	f.catches(f.carp, 3, 10, 0)
	f.loadList()
	f.start()
	f.dice = []int{1500} // the wait
	if msgs := f.reply(fishCast, 0); len(msgs) != 0 {
		t.Fatalf("the cast was answered at once: %s", ops(msgs))
	}
	f.dice = []int{
		89, // the bite: under 90 for a perfect cast
		99, // not lost, not snapped
		0,  // the one row
		99, // no second item
		99, // no skill
	}
	msgs := f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, OpcodeFishing)
	if want := mustHex(t, "3c00010002"); !bytes.Equal(msgs[0], want) {
		t.Errorf("the bait's 0x003C % X, want % X", msgs[0], want)
	}
	loot := msgs[1]
	if sub0F02(loot) != fishLoot || loot[3] != 0x02 || binary.LittleEndian.Uint32(loot[4:]) != 1 {
		t.Fatalf("the catch list % X", loot)
	}
	// the record: 3, its condition, charges, not worn, no quick slot, key 0
	r := buffer.NewReader(loot[8:])
	for i, want := range []int64{3, 0, 0, -1, -1} {
		if v, err := r.Read7BitEncodedInt64(); err != nil || v != want {
			t.Fatalf("field %d %d, want %d (%v)", i, v, want, err)
		}
	}
	if key, _ := r.ReadUint32(); key != 0 {
		t.Errorf("key %d", key)
	}
	if n, _ := f.held(f.bait); n != 2 {
		t.Errorf("%d baits left, want 2", n)
	}

	msgs = f.reply(fishKeep, 0, 3)
	mustOps(t, msgs, OpcodeFishing, OpcodeFishing)
	if !bytes.Equal(msgs[0], caughtPayload(0, 3)) || sub0F02(msgs[1]) != fishClose {
		t.Errorf("Keep: % X, % X", msgs[0], msgs[1])
	}
	if n, recs := f.held(f.carp); n != 3 || recs != 1 {
		t.Errorf("carp %d in %d records, want 3 in 1", n, recs)
	}
	l := sessionInvList(f.sess)
	if rec, ok := l.at(2); !ok || rec.key != 2 || rec.patternID != f.carp || rec.qty != 3 {
		t.Errorf("the client's list at 2: %+v %v, want the carp under key 2", rec, ok)
	}
	// the control: a second Keep of the same key files nothing, and with no
	// catch up the client goes back to casting (Ready)
	msgs = f.reply(fishKeep, 0, 3)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishReady {
		t.Errorf("a stale Keep: % X", msgs[0])
	}
	if n, _ := f.held(f.carp); n != 3 {
		t.Errorf("a stale Keep filed: %d carp", n)
	}
}

// What stacks goes onto the stack the angler holds (CLIENT's AddItem), up
// to its limit; what does not fit stays on the line and the list says so.
// Take All files every record that fits, then the list closes; the control
// is Leave All, which files nothing.
func TestFishingTakeAllStacksAndFull(t *testing.T) {
	f := newFishFixture(t)
	f.wear(f.carp, 48, -1) // two short of the 50
	f.catches(f.carp, 5, 10, 0)
	f.catches(f.boot, 1, 10, 0)
	f.loadList()
	f.start()
	f.dice = []int{0}
	f.reply(fishCast, 5)
	f.dice = []int{0, 99, 0, 0, 0, 99} // bite, keep it, the carp, a second, the boot, no skill
	msgs := f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, OpcodeFishing)
	if msgs[1][3] != 0x04 {
		t.Fatalf("two records: % X", msgs[1])
	}
	msgs = f.reply(fishReel)
	// Caught(0, 2): two carp onto the stack, three stay at 0; Caught(1, 1):
	// the boot; then the full line and the list closed
	mustOps(t, msgs, OpcodeFishing, OpcodeFishing, protocol.OpcodeColoredChat, OpcodeFishing)
	if !bytes.Equal(msgs[0], caughtPayload(0, 2)) || !bytes.Equal(msgs[1], caughtPayload(1, 1)) || sub0F02(msgs[3]) != fishClose {
		t.Errorf("Take All: % X / % X / % X", msgs[0], msgs[1], msgs[3])
	}
	if !bytes.Contains(msgs[2], utf16le(fishFull)) {
		t.Errorf("full: % X", msgs[2])
	}
	if n, recs := f.held(f.carp); n != 50 || recs != 1 {
		t.Errorf("carp %d in %d records, want 50 in 1", n, recs)
	}
	if n, _ := f.held(f.boot); n != 1 {
		t.Errorf("%d boots", n)
	}

	g := newFishFixture(t)
	g.catches(g.carp, 2, 10, 0)
	g.loadList()
	g.start()
	g.dice = []int{0}
	g.reply(fishCast, 0)
	g.dice = []int{0, 99, 0, 99, 99}
	g.bite()
	msgs = g.reply(fishPull)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishClose {
		t.Errorf("Leave All % X", msgs[0])
	}
	if n, _ := g.held(g.carp); n != 0 {
		t.Errorf("Leave All filed %d carp", n)
	}
}

// Nothing bites (8): the bait stays. A fish gets away (9) or the line
// snaps (10: 0x0060 first, the pole's condition 900): the bait goes. A poor
// cast's chance is 40 at skill 0, a good one's 70.
func TestFishingMissLostSnap(t *testing.T) {
	f := newFishFixture(t)
	f.catches(f.carp, 1, 10, 0)
	f.loadList()
	f.start()
	f.dice = []int{0}
	f.reply(fishCast, 15)
	f.dice = []int{40} // 40 is not under 40
	msgs := f.bite()
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishMissed {
		t.Errorf("missed % X", msgs[0])
	}
	if n, _ := f.held(f.bait); n != 3 {
		t.Errorf("a miss took bait: %d left", n)
	}

	f.dice = []int{0}
	f.reply(fishCast, 5)
	f.dice = []int{69, 14} // a bite under 70; 14 under the 15 lost
	msgs = f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, OpcodeFishing)
	if sub0F02(msgs[1]) != fishLost {
		t.Errorf("lost % X", msgs[1])
	}

	f.dice = []int{0}
	f.reply(fishCast, 0)
	f.dice = []int{0, 15} // 15: past the lost, under 15 + 5 snapped
	msgs = f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAdjustItemHP, OpcodeFishing)
	if want := adjustItemHPPayload(0, 900); !bytes.Equal(msgs[1], want) {
		t.Errorf("0x0060 % X, want % X", msgs[1], want)
	}
	if sub0F02(msgs[2]) != fishDamaged {
		t.Errorf("damaged % X", msgs[2])
	}
	if n, _ := f.held(f.bait); n != 1 {
		t.Errorf("%d baits left, want 1", n)
	}
	for _, r := range f.rows() {
		if r.Pattern.PatternID == f.pole && r.RemainHP != 900 {
			t.Errorf("the pole's condition %d", r.RemainHP)
		}
	}
}

// No row applies (none at all, or none at this skill) and the server knows
// no water ahead: missed, the bait kept, and a line says which. Water the
// server does not know (no ground files) is no refusal: the control.
func TestFishingNoCatchNoWater(t *testing.T) {
	f := newFishFixture(t)
	f.catches(f.carp, 1, 10, 50) // skill 50 and more
	f.loadList()
	f.start()
	f.dice = []int{0}
	f.reply(fishCast, 0)
	msgs := f.bite()
	mustOps(t, msgs, protocol.OpcodeColoredChat, OpcodeFishing)
	if !bytes.Contains(msgs[0], utf16le(fishNothingYet)) || sub0F02(msgs[1]) != fishMissed {
		t.Errorf("no row: % X / % X", msgs[0], msgs[1])
	}

	f.catches(f.carp, 1, 10, 0)
	f.view.Water = entity.FishWaterNone
	f.dice = []int{0}
	f.reply(fishCast, 0)
	msgs = f.bite()
	mustOps(t, msgs, protocol.OpcodeColoredChat, OpcodeFishing)
	if !bytes.Contains(msgs[0], utf16le(fishNoWater)) || sub0F02(msgs[1]) != fishMissed {
		t.Errorf("no water: % X / % X", msgs[0], msgs[1])
	}
	if n, _ := f.held(f.bait); n != 3 {
		t.Errorf("%d baits left, want 3", n)
	}

	f.view.Water = entity.FishWaterUnknown
	f.dice = []int{0}
	f.reply(fishCast, 0)
	f.dice = []int{0, 99, 0, 99, 99}
	msgs = f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, OpcodeFishing)
	if sub0F02(msgs[1]) != fishLoot {
		t.Errorf("unknown water: % X", msgs[1])
	}
}

// Cancel closes the window (Stop), with a session or without one (a Start
// the HUD could not open); a cast cancelled before its bite is never
// answered. A cast without a session closes the window too.
func TestFishingCancel(t *testing.T) {
	f := newFishFixture(t)
	f.catches(f.carp, 1, 10, 0)
	f.loadList()
	f.start()
	f.dice = []int{0}
	f.reply(fishCast, 0)
	msgs := f.reply(fishCancel)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishStop {
		t.Errorf("Cancel % X", msgs[0])
	}
	if msgs := f.bite(); len(msgs) != 0 {
		t.Errorf("a cancelled cast answered: %s", ops(msgs))
	}
	msgs = f.reply(fishCancel)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishStop {
		t.Errorf("Cancel with no session % X", msgs[0])
	}
	msgs = f.reply(fishCast, 0)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishStop {
		t.Errorf("a cast with no session % X", msgs[0])
	}
	if f.timer != nil {
		t.Error("a cast with no session went in")
	}
}

// A catch teaches a point of skill when the throw is under (100 - skill) /
// 2, kept in characters.fishing and said; the control is a throw at it.
func TestFishingSkill(t *testing.T) {
	f := newFishFixture(t)
	f.catches(f.carp, 1, 10, 0)
	f.loadList()
	f.start()
	f.dice = []int{0}
	f.reply(fishCast, 0)
	f.dice = []int{0, 99, 0, 99, 50} // 50 is not under 50
	f.bite()
	if s := f.skill(); s != 0 {
		t.Errorf("skill %d after a throw of 50", s)
	}
	f.reply(fishPull)
	f.dice = []int{0}
	f.reply(fishCast, 0)
	f.dice = []int{0, 99, 0, 99, 49}
	msgs := f.bite()
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, OpcodeFishing, protocol.OpcodeColoredChat)
	if s := f.skill(); s != 1 {
		t.Errorf("skill %d, want 1", s)
	}
	if !bytes.Contains(msgs[2], utf16le("(1)")) {
		t.Errorf("the skill line % X", msgs[2])
	}
}

// The watch stops an angler who fights (said) or dies; one gone from the
// world is dropped with nothing sent.
func TestFishingWatch(t *testing.T) {
	f := newFishFixture(t)
	f.loadList()
	f.start()
	f.h.watch()
	if msgs := f.sent(); len(msgs) != 0 {
		t.Fatalf("a quiet angler stopped: %s", ops(msgs))
	}
	f.view.Fighting = true
	f.h.watch()
	msgs := f.sent()
	mustOps(t, msgs, protocol.OpcodeColoredChat, OpcodeFishing)
	if sub0F02(msgs[1]) != fishStop {
		t.Errorf("fighting % X", msgs[1])
	}
	f.view.Fighting = false
	f.start()
	f.view.Dead = true
	f.h.watch()
	mustOps(t, f.sent(), OpcodeFishing)
	f.view.Dead = false
	f.start()
	f.view.Name = "" // gone
	f.h.watch()
	mustOps(t, f.sent())
	if len(f.h.active) != 0 {
		t.Error("a gone angler kept")
	}
}

// A row applies in its world (or every world), inside its box, from its
// skill, in its months.
func TestFishCatchApplies(t *testing.T) {
	box := database.FishCatch{ItemID: 1, Count: 1, Weight: 1, World: 0, Box: true,
		MinX: 100, MinZ: 200, MaxX: 300, MaxZ: 400, MinSkill: 20, Months: 1<<11 | 1 | 2}
	for _, c := range []struct {
		world        uint8
		x, z         float32
		skill, month int
		want         bool
	}{
		{0, 100, 200, 20, 1, true},
		{0, 300, 400, 99, 12, true},
		{1, 150, 250, 20, 1, false},  // another world
		{0, 99, 250, 20, 1, false},   // west of the box
		{0, 150, 401, 20, 1, false},  // north of it
		{0, 150, 250, 19, 1, false},  // too little skill
		{0, 150, 250, 20, 3, false},  // March
		{0, 150, 250, 20, 2, true},   // February
		{0, 150, 250, 20, 13, true},  // no month: every month
	} {
		if got := box.Applies(c.world, c.x, c.z, c.skill, c.month); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
	any := database.FishCatch{Count: 1, Weight: 1, World: -1}
	if !any.Applies(7, -5, 9e6, 0, 6) {
		t.Error("a row of every world, anywhere")
	}
	if (database.FishCatch{Count: 1, World: -1}).Applies(0, 0, 0, 0, 1) {
		t.Error("a row of weight 0 applies")
	}
}

// The HUD's quick slot (UseItem(i, 1)) on the worn pole fishes and leaves it
// worn; the item menu's (0) takes it off, the control. One not yet worn is
// put in the hand (0x003F to 14), then the window.
func TestFishingFromTheQuickSlot(t *testing.T) {
	f := newFishFixture(t)
	f.loadList()
	eq := NewEquipHandler(nil, f.inv, nil, nil)
	eq.playerOf = func(*session.Session) *entity.Player { return f.player }
	eq.fishing = f.h
	use := func(key uint32, interaction byte) [][]byte {
		payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
			w.WriteUint32(key)
			w.WriteUint32(0xFFFFFFFF)
			w.WriteByte(interaction)
		})[2:]
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
		msg.SetData(payload)
		if err := eq.HandleInteractItem(f.sess, msg); err != nil {
			t.Fatal(err)
		}
		return f.sent()
	}
	msgs := use(0, 1)
	mustOps(t, msgs, OpcodeFishing)
	if sub0F02(msgs[0]) != fishStart {
		t.Errorf("quick slot % X", msgs[0])
	}
	if loc := f.equipLoc(f.pole); loc != 14 {
		t.Errorf("the pole moved to %d", loc)
	}
	msgs = use(0, 0)
	if len(msgs) == 0 || opOf(msgs[0]) == OpcodeFishing {
		t.Errorf("the item menu fished: %s", ops(msgs))
	}
	if loc := f.equipLoc(f.pole); loc != -1 {
		t.Errorf("the item menu left the pole at %d", loc)
	}
	msgs = use(0, 0) // the item menu's Equip: on, and the line saying how to fish, no window
	if len(msgs) == 0 || opOf(msgs[len(msgs)-1]) != protocol.OpcodeColoredChat || !bytes.Contains(msgs[len(msgs)-1], utf16le(fishHowTo)) {
		t.Errorf("the item menu's Equip: %s", ops(msgs))
	}
	for _, m := range msgs {
		if opOf(m) == OpcodeFishing {
			t.Error("the item menu's Equip fished")
		}
	}
	use(0, 0) // off again
	msgs = use(0, 1)
	if len(msgs) < 2 || opOf(msgs[len(msgs)-1]) != OpcodeFishing || sub0F02(msgs[len(msgs)-1]) != fishStart {
		t.Errorf("an unworn pole from the quick slot: %s", ops(msgs))
	}
	for _, m := range msgs {
		if bytes.Contains(m, utf16le(fishHowTo)) {
			t.Error("the quick slot said how to fish")
		}
	}
	if loc := f.equipLoc(f.pole); loc != 14 {
		t.Errorf("the pole is at %d, want 14", loc)
	}
}

func (f *fishFixture) equipLoc(pattern int) int {
	for _, r := range f.rows() {
		if r.Pattern.PatternID == pattern {
			return r.EquipLoc
		}
	}
	return -2
}

// /fish (any case, with words after it) and #fish start a session for a
// player who is no GM; the control is a line that only begins with the
// letters, said aloud as any line is.
func TestFishingSlashFish(t *testing.T) {
	c := newChatWorld(t)
	sess, p := c.join(1, "Angler", 25400, 15816, 0)
	f := newFishFixture(t)
	f.sess = sess
	f.player = p
	f.h.playerOf = c.gw.SessionPlayer
	f.view.CharServerID = p.CharServerID
	f.db.Exec(`UPDATE charinventory SET serverid = ?`, p.CharServerID)
	c.h.fishing = f.h
	for _, line := range []string{"/fish", "#fish", "/FISH now"} {
		if err := c.h.HandleSayChat(sess, incoming(protocol.OpcodeClientSayChat, wide(line, 0xFFFFFFFF))); err != nil {
			t.Fatal(err)
		}
		msgs := f.sent()
		if len(msgs) != 1 || sub0F02(msgs[0]) != fishStart {
			t.Errorf("%q: %s", line, ops(msgs))
		}
	}
	if err := c.h.HandleSayChat(sess, incoming(protocol.OpcodeClientSayChat, wide("/fishy", 0xFFFFFFFF))); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.sent() {
		if opOf(m) == OpcodeFishing {
			t.Errorf("/fishy fished")
		}
	}
}

// A gear set whose main hand is a fishing pole fishes once it is on (0x003F
// to 14 and 15, then the window); the control is a set of a sword, which
// only equips.
func TestFishingFromAGearSet(t *testing.T) {
	f := newGearFixture(t)
	pole := f.own(f.pattern("Fishing Pole", 20, 0, 0), 14)
	bait := f.own(f.pattern("can of worms", 21, 0, 0), 15)
	sword := f.own(f.pattern("Blade", 12, 1, 0), -1)
	f.loadList()
	fh := NewFishingHandler(f.db, nil, f.inv)
	fh.view = func(*session.Session, time.Time) (entity.FishingView, bool) {
		return entity.FishingView{Name: "Gear", CharServerID: f.sid}, true
	}
	f.h.fishing = fh
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(0)
		writeWideString(w, "Fish", 64)
	})
	for _, c := range []equipChange{{pole, -1}, {bait, -1}, {sword, 14}} {
		if err := f.inv.SetEquipLoc(f.sid, c.itemID, c.equipLoc); err != nil {
			t.Fatal(err)
		}
	}
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(1)
		writeWideString(w, "Fight", 64)
	})
	use := func(set byte) [][]byte {
		return f.send(protocol.OpcodeUseWeaponSet, func(w *buffer.Writer) { w.WriteByte(set) })
	}
	out := use(0)
	if len(out) < 3 || sub0F02(out[len(out)-1]) != fishStart {
		t.Fatalf("the fishing set: %s", ops(out))
	}
	if w := f.worn(); w[14] != pole || w[15] != bait {
		t.Fatalf("worn after the fishing set: %v", w)
	}
	for _, m := range use(1) {
		if opOf(m) == OpcodeFishing {
			t.Errorf("the sword's set fished")
		}
	}
	if w := f.worn(); w[14] != sword {
		t.Errorf("worn after the sword's set: %v", w)
	}
}

// An item's Fish stat goes out as CLIENT's stat 28 (its stat words' "Fish"),
// and only when it has one: the control.
func TestItemFishStat(t *testing.T) {
	five, zero := 5, 0
	if s := itemPatternStats(&database.ItemPatternRecord{Fish: &five}); s[28] != 5 || len(s) != 1 {
		t.Errorf("fish 5: %v", s)
	}
	if s := itemPatternStats(&database.ItemPatternRecord{Fish: &zero}); len(s) != 0 {
		t.Errorf("fish 0: %v", s)
	}
}
