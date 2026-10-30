package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// rehabFixture is a talk world with a database, the quests running, the
// masteries' catalog, a level 60 warrior in the world beside Rehabilitator
// Anja, and the Rose of Rehabilitation's record.
type rehabFixture struct {
	t     *testing.T
	tw    *talkWorld
	db    *database.SQLiteDB
	store *database.SQLiteCharacterStore
	inv   *database.InventoryStore
	book  *database.SpellbookStore
	qe    *QuestEngine
	cm    *ClassMasteryHandler
	r     *Rehab
	sid   uint32
	anja  uint32 // the object id the session knows her by
	last  []string
}

func newRehabFixture(t *testing.T) *rehabFixture {
	t.Helper()
	tw := newTalkWorld(t, 23431, 19601)
	db, err := database.NewSQLite(t.TempDir() + "/rehab.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 60}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	p := tw.p
	p.CharServerID, p.Class, p.Race, p.Level, p.BaseStr = rec.ServerID, 0, 0, 60, 130
	p.RecalcStats(true)
	f := &rehabFixture{t: t, tw: tw, db: db, store: store, inv: database.NewInventoryStore(db),
		book: database.NewSpellbookStore(db), sid: rec.ServerID}
	f.item(database.RoseOfRehabilitation, database.RoseOfRehabilitationName, 759960290, -1, 1)
	qe, err := NewQuestEngine(db, tw.w, tw.mgr, f.inv, f.book)
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	tw.w.SetCMCatalog(testCatalog())
	f.cm = NewClassMasteryHandler(tw.mgr)
	f.cm.SetGameWorld(tw.w)
	f.cm.SetStore(store)
	f.cm.SetSpellbook(f.book)
	f.r = NewRehab(db, tw.w, f.inv, f.book, f.cm, store)
	f.r.quests = func() *QuestEngine { return qe }
	tw.h.rehab = f.r
	f.lists()
	f.anja = tw.add(1, &entity.NPC{ServerID: 2205, Name: "Rehabilitator Anja", NPCType: 0x80,
		SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5})
	return f
}

func (f *rehabFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatal(err)
	}
}

// item adds an item record under its own id: worn in slot (-1 none).
func (f *rehabFixture) item(id int, name string, icon int64, slot, maxStack int) {
	f.t.Helper()
	f.exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4,
		attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6,
		craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, ?, 0, ?, 0, 1, 1, 0, 0, 0, 0, 1, ?, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
		id, icon, slot, maxStack, name)
}

// spell adds a spell record.
func (f *rehabFixture) spell(id int, name string) {
	f.t.Helper()
	f.exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
		recast, eqprequire, spellname, spelldesc) VALUES (?, 30, 0, 0, 16880, 2, 15, 1234, -5678, 1, 1, -1, ?, '')`, id, name)
}

// know puts spells in the book, in order.
func (f *rehabFixture) know(ids ...int) {
	f.t.Helper()
	for _, id := range ids {
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM spells WHERE serverid = ?`, f.sid).Scan(&n)
		f.exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
			VALUES (?, ?, ?, 0, -1, 0, -1)`, f.sid, id, n)
	}
}

// knows is whether the book holds a spell.
func (f *rehabFixture) knows(id int) bool {
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM spells WHERE serverid = ? AND spellid = ?`, f.sid, id).Scan(&n)
	return n > 0
}

// own gives the character n of an item: worn in a slot (-1 not), in the
// bank (location 1) or not; the client's lists as its MemoryDump left them.
func (f *rehabFixture) own(pattern, n, equipLoc, location int) {
	f.t.Helper()
	f.exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, ?, 0, 0, ?, ?, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))`,
		f.sid, n, pattern, equipLoc, location, f.sid, location)
	f.lists()
}

// lists sets the session's copies of the client's pack and bank lists from
// the rows.
func (f *rehabFixture) lists() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	var pack, bank []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 1 {
			bank = append(bank, r)
		} else {
			pack = append(pack, r)
		}
	}
	f.tw.s.SetInventoryList(newInvList(pack))
	f.tw.s.SetBankList(newInvList(bank))
}

// held is how many of an item the character holds, worn or not, anywhere.
func (f *rehabFixture) held(pattern int) int {
	var n int
	f.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`,
		f.sid, pattern).Scan(&n)
	return n
}

// purse sets the purse, in the world and in the row.
func (f *rehabFixture) purse(n int) {
	f.tw.p.Tunar = n
	f.exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, n, f.sid)
}

// purseNow is the purse in the world and in the row, which must agree.
func (f *rehabFixture) purseNow() int {
	f.t.Helper()
	var row int
	f.db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, f.sid).Scan(&row)
	if row != f.tw.p.Tunar {
		f.t.Errorf("the purse is %d in the world and %d in the row", f.tw.p.Tunar, row)
	}
	return row
}

// counts is the character's charresets row of a kind: used, paid.
func (f *rehabFixture) counts(kind string) (int, int) {
	f.t.Helper()
	all, err := f.db.CharResets(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return all[kind].Used, all[kind].Paid
}

// talk talks to Anja and returns what was sent.
func (f *rehabFixture) talk() []string {
	f.t.Helper()
	f.tw.talk(f.anja)
	f.last = questMsgs(f.tw.s)
	return f.last
}

// resets talks to Anja and answers her menu's reset: the question which.
func (f *rehabFixture) resets() []string {
	f.t.Helper()
	f.talk()
	return f.pick(0)
}

// pick answers the last question with option n and returns what was sent.
func (f *rehabFixture) pick(n byte) []string {
	f.t.Helper()
	id := boxID(f.last)
	if id == 0 {
		f.t.Fatalf("no question to answer in %q", f.last)
	}
	f.tw.pick(id, n)
	f.last = questMsgs(f.tw.s)
	return f.last
}

// asked is the last question in msgs: its words and answers.
func asked(msgs []string) (string, []string) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.HasPrefix(msgs[i], "ask#") {
			_, rest, _ := strings.Cut(msgs[i], ":")
			parts := strings.Split(rest, "|")
			return parts[0], parts[1:]
		}
	}
	return "", nil
}

// rehabSays is whether a box in msgs holds part.
func rehabSays(msgs []string, part string) bool {
	for _, m := range msgs {
		if strings.HasPrefix(m, "box:") && strings.Contains(m, part) {
			return true
		}
	}
	return false
}

// giveMastery gives the warrior Brawn 1 (the test catalog's rank 1, 10
// points), in the world and in its rows.
func (f *rehabFixture) giveMastery() {
	f.t.Helper()
	p := f.tw.p
	p.CMs = map[int]int{1: 10}
	p.Mastery = entity.Mastery{Points: 90, Spent: 10}
	if err := f.store.BuyCM(f.sid, database.OwnedCM{ID: 1, Cost: 10}, database.MasteryRow{Points: 90, Spent: 10}); err != nil {
		f.t.Fatal(err)
	}
}

// spendTraining has the warrior spend points on STR and STA, as the trainer
// does: its base stats raised and its row's tp columns.
func (f *rehabFixture) spendTraining(str, sta int) {
	f.t.Helper()
	bs := entity.LookupBaseStats(f.tw.p.Race, f.tw.p.Class)
	f.tw.p.BaseStr, f.tw.p.BaseSta = bs.Str+str, bs.Sta+sta
	f.tw.p.RecalcStats(true)
	f.exec(`UPDATE characters SET tpstrength = ?, tpstamina = ?, unusedTP = 0, totalTP = ? WHERE serverid = ?`,
		str, sta, str+sta, f.sid)
}

// The menu says what the character has left and offers three answers, a
// page of CLIENT's answer box (VIWndResponse shows three, then More, Back and
// its own Close): a reset (then which, each free while a free one is left and
// at its price after), a path, a rose. The question is a rehabilitator's box
// (0x54), which no other NPC answers; a Close (255) and an old box do nothing.
func TestRehabilitatorsMenu(t *testing.T) {
	f := newRehabFixture(t)
	m := f.talk()
	if !rehabSays(m, "Rehabilitator Anja: Be welcome, Talker.") ||
		!rehabSays(m, "You have 1 free Combat Mastery reset, 1 free stat point reset and 2 free path respecs left, and hold no Rose of Rehabilitation.") {
		t.Fatalf("the greeting: %q", m)
	}
	q, answers := asked(m)
	want := []string{"Reset my masteries or stat points", "Choose a path again (free)", "Buy a Rose of Rehabilitation (10,000,000 tunar)"}
	if q != rehabAskMenu || strings.Join(answers, "|") != strings.Join(want, "|") {
		t.Fatalf("the menu: %q %q", q, answers)
	}
	first := boxID(m)
	if !IsRehabBox(first) || first&0xFF000000 != 0x54000000 {
		t.Errorf("the box id %#x is not a rehabilitator's", first)
	}
	m = f.pick(0)
	q, answers = asked(m)
	want = []string{"My Combat Masteries (free)", "My stat points (free)", "Both (free)"}
	if q != rehabAskResets || strings.Join(answers, "|") != strings.Join(want, "|") {
		t.Fatalf("which reset: %q %q", q, answers)
	}
	for _, l := range append(answers, "Buy a Rose of Rehabilitation (10,000,000 tunar)") {
		if len([]rune(l)) > questAnswerChars {
			t.Errorf("%q is longer than CLIENT shows", l)
		}
	}
	// a Close keeps the box; the old menu's id answers nothing now
	f.tw.pick(boxID(m), 255)
	f.tw.pick(first, 1)
	if m := questMsgs(f.tw.s); len(m) != 0 {
		t.Errorf("a close or an old box did something: %q", m)
	}
	// the control: an NPC of another name is no rehabilitator
	other := f.tw.add(2, &entity.NPC{ServerID: 2206, Name: "Rehab Anja", NPCType: 0x80, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5})
	f.tw.talk(other)
	if m := questMsgs(f.tw.s); boxID(m) != 0 {
		t.Errorf("another NPC opened the menu: %q", m)
	}
}

// A free Combat Mastery reset is used once: the masteries go, their points
// come back, the count is kept; the next is asked 1,000,000 tunar and takes
// exactly that, and with too little it is refused, nothing taken.
func TestRehabMasteryResetFreeOnceThenPriced(t *testing.T) {
	f := newRehabFixture(t)
	f.purse(5000)
	f.giveMastery()
	f.resets()
	m := f.pick(0)
	if q, a := asked(m); q != "Shall I begin? It uses your free mastery reset." || strings.Join(a, "|") != "Yes, begin.|Not now." {
		t.Fatalf("the question: %q %q (%q)", q, a, m)
	}
	m = f.pick(0)
	if len(f.tw.p.CMs) != 0 || f.tw.p.Mastery.Points != 100 {
		t.Fatalf("after the free reset: %v %+v (%q)", f.tw.p.CMs, f.tw.p.Mastery, m)
	}
	if owned, _ := f.store.LoadOwnedCMs(f.sid); len(owned) != 0 {
		t.Errorf("the ranks kept: %v", owned)
	}
	if !rehabSays(m, "It is done. 1 mastery is unlearned, and 10 points are yours to spend again.") || !rehabSays(m, "That was your free mastery reset.") {
		t.Errorf("the lines: %q", m)
	}
	if u, p := f.counts(database.ResetMastery); u != 1 || p != 0 || f.purseNow() != 5000 {
		t.Errorf("used %d paid %d, purse %d: want 1 0 5000", u, p, f.purseNow())
	}

	// the second: priced
	f.giveMastery()
	m = f.resets()
	if _, a := asked(m); a[0] != "My Combat Masteries (1,000,000 tunar)" {
		t.Fatalf("the second's price: %q", a)
	}
	f.purse(999_999)
	m = f.pick(0)
	if !rehabSays(m, "That costs 1,000,000 tunar, and you carry 999,999 tunar.") || boxID(m) != 0 {
		t.Fatalf("too little tunar: %q", m)
	}
	if len(f.tw.p.CMs) != 1 || f.purseNow() != 999_999 {
		t.Fatalf("refused, yet changed: %v, purse %d", f.tw.p.CMs, f.purseNow())
	}
	f.purse(1_250_000)
	f.resets()
	m = f.pick(0)
	if q, _ := asked(m); q != "Shall I begin? It costs 1,000,000 tunar." {
		t.Fatalf("the paid question: %q", q)
	}
	m = f.pick(0)
	if len(f.tw.p.CMs) != 0 || f.purseNow() != 250_000 || !rehabSays(m, "You paid 1,000,000 tunar.") {
		t.Fatalf("the paid reset: %v, purse %d, %q", f.tw.p.CMs, f.purseNow(), m)
	}
	if u, p := f.counts(database.ResetMastery); u != 1 || p != 1 {
		t.Errorf("used %d paid %d: want 1 1", u, p)
	}
	// nothing to reset: said, nothing taken
	f.resets()
	if m := f.pick(0); !rehabSays(m, rehabNoMastery) || boxID(m) != 0 {
		t.Errorf("no masteries: %q", m)
	}
}

// A free stat point reset gives back the points spent, as #rst_both's
// training half: the base stats as born, the row's tp columns 0, the trainer
// told (0x001D); the next is priced by the GMs' setting, read at the talk, and
// takes exactly that.
func TestRehabStatPointsResetFreeOnceThenPriced(t *testing.T) {
	f := newRehabFixture(t)
	f.purse(800_000)
	f.spendTraining(5, 3)
	f.resets()
	m := f.pick(1)
	if q, _ := asked(m); q != "Shall I begin? It uses your free stat point reset." {
		t.Fatalf("the question: %q (%q)", q, m)
	}
	m = f.pick(0)
	rec, _ := f.store.GetByServerID(f.sid)
	bs := entity.LookupBaseStats(0, 0)
	if rec.TPStrength != 0 || rec.TPStamina != 0 || rec.UnusedTP != 8 || f.tw.p.BaseStr != bs.Str || f.tw.p.UnusedTP != 8 {
		t.Fatalf("after: row tp %d/%d unused %d, base STR %d (born %d), unused %d", rec.TPStrength, rec.TPStamina,
			rec.UnusedTP, f.tw.p.BaseStr, bs.Str, f.tw.p.UnusedTP)
	}
	if !rehabSays(m, "8 training points are yours to spend again.") {
		t.Errorf("the lines: %q", m)
	}
	if !hasMsg(m, fmt.Sprintf("op:%04X", uint16(protocol.OpcodeUpdateTrainingPoints))) && !hasMsg(m, "op:001D") {
		t.Errorf("the trainer not told: %q", m)
	}
	if u, p := f.counts(database.ResetTraining); u != 1 || p != 0 {
		t.Errorf("used %d paid %d", u, p)
	}

	// a GM's price: read at the talk, taken exactly
	f.exec(`UPDATE rehabsettings SET value = 750000 WHERE setting = 'price_training'`)
	f.spendTraining(2, 0)
	m = f.resets()
	if _, a := asked(m); a[1] != "My stat points (750,000 tunar)" {
		t.Fatalf("the GM's price: %q", a)
	}
	f.pick(1)
	f.pick(0)
	if f.purseNow() != 50_000 {
		t.Errorf("the purse after 750,000 of 800,000: %d", f.purseNow())
	}
	if u, p := f.counts(database.ResetTraining); u != 1 || p != 1 {
		t.Errorf("used %d paid %d: want 1 1", u, p)
	}
}

// Both together: free while both have a free reset; with one left, the
// other's price; with none, the price of both (a GM's 1,500,000 here, not the
// two prices added); a GM's bonus gives one more free one.
func TestRehabBothTogether(t *testing.T) {
	f := newRehabFixture(t)
	f.purse(4_000_000)
	reset := func(wantLabel, wantQuestion string) {
		t.Helper()
		f.giveMastery()
		f.spendTraining(4, 0)
		m := f.resets()
		if _, a := asked(m); a[2] != wantLabel {
			t.Fatalf("the label: %q, want %q", a[2], wantLabel)
		}
		m = f.pick(2)
		if q, _ := asked(m); q != wantQuestion {
			t.Fatalf("the question: %q, want %q (%q)", q, wantQuestion, m)
		}
		f.pick(0)
		if len(f.tw.p.CMs) != 0 || f.tw.p.UnusedTP != 4 {
			t.Fatalf("not reset: %v, unused %d", f.tw.p.CMs, f.tw.p.UnusedTP)
		}
		f.tw.p.UnusedTP = 0
	}
	reset("Both (free)", "Shall I begin? It uses your free mastery reset and your free stat point reset.")
	if f.purseNow() != 4_000_000 {
		t.Fatalf("free, yet %d", f.purseNow())
	}
	// a GM's price of both, not the two prices added
	f.exec(`UPDATE rehabsettings SET value = 1500000 WHERE setting = 'price_both'`)
	reset("Both (1,500,000 tunar)", "Shall I begin? It costs 1,500,000 tunar.")
	if f.purseNow() != 2_500_000 {
		t.Fatalf("after the price of both: %d", f.purseNow())
	}
	f.exec(`UPDATE charresets SET bonus = 1 WHERE serverid = ? AND kind = 'mastery'`, f.sid)
	reset("Both (1,000,000 tunar)", "Shall I begin? It costs 1,000,000 tunar and your free mastery reset.")
	if f.purseNow() != 1_500_000 {
		t.Fatalf("after the stat points' price: %d", f.purseNow())
	}
	um, pm := f.counts(database.ResetMastery)
	ut, pt := f.counts(database.ResetTraining)
	if um != 2 || pm != 1 || ut != 1 || pt != 2 {
		t.Errorf("mastery used %d paid %d, training used %d paid %d: want 2 1 1 2", um, pm, ut, pt)
	}
}

// The spells, items and quests of a warrior's paths: a level 20 class quest
// whose pick is a scroll, group 5's level 30, 40 and 49 rewards, the 60
// epic's reward and a quest of no path.
const (
	spRoar, spStrength, spHowl, spForce, spBattlecry, spWarcry = 9001, 9002, 9003, 9004, 9005, 9006
	spMend, spBellow, spMight, spRally                         = 9010, 9020, 9021, 9030
	itBellow, itMight                                          = 8568, 8569
	itFalchion, itPoleaxe, itSteelwind, itFoereaver            = 5943, 5554, 1026, 5631
	qL20, qL30, qL40, qL49, qEpic, qOther                      = 100100, 100801, 100802, 100803, 100900, 100500
)

// choice is a choices column: each option an answer and what it gives, as
// "spells:items".
func choice(options ...string) string {
	var parts []string
	for _, o := range options {
		answer, gives, _ := strings.Cut(o, "=")
		spells, items, _ := strings.Cut(gives, ":")
		parts = append(parts, fmt.Sprintf(`{"answer":%q,"says":"It is yours.","spells":%q,"items":%q}`, answer, spells, items))
	}
	return `{"ask":"Which do you take?","options":[` + strings.Join(parts, ",") + `]}`
}

// quest adds a quest by its import key, given by giver, a talk step at it a
// choices column (none: a step with no choice, teaching spells).
func (f *rehabFixture) quest(id int, key, giver string, steps ...string) {
	f.t.Helper()
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, source)
		VALUES (?, ?, 1, ?, 1, 60, '0', ?)`, id, "Quest "+key, giver, "EQOA-Data "+key+" (tools/import_quests.py)")
	for i, st := range steps {
		if strings.HasPrefix(st, "{") {
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, choices) VALUES (?, ?, 'talk', ?, 'Talk.', ?)`,
				id, i+1, giver, st)
		} else {
			f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, spells) VALUES (?, ?, 'talk', ?, 'Talk.', ?)`,
				id, i+1, giver, st)
		}
	}
}

// done marks a quest finished with its picks ("" none kept).
func (f *rehabFixture) done(id int, picks string) {
	f.t.Helper()
	f.exec(`INSERT INTO charquests (serverid, questid, step, count, done, slot, picks) VALUES (?, ?, 0, 0, 1, 0, ?)`, f.sid, id, picks)
}

// row is whether the character has a row of the quest.
func (f *rehabFixture) row(id int) bool {
	var n int
	f.db.QueryRow(`SELECT COUNT(*) FROM charquests WHERE serverid = ? AND questid = ?`, f.sid, id).Scan(&n)
	return n > 0
}

// warriorPaths is the warrior having taken every path: Bellow at 20 (its
// scroll used), Roar at 30, Howl and the Falchion at 40 (picks kept), and at
// 49 (from before picks were kept) Battlecry and the Steelwind, banked; the
// epic's Warcry and a quest of no path's Mend besides.
func (f *rehabFixture) warriorPaths() {
	f.t.Helper()
	for id, name := range map[int]string{spRoar: "Roar", spStrength: "Pillar of Strength", spHowl: "Howl", spForce: "Pillar of Force",
		spBattlecry: "Battlecry", spWarcry: "Warcry", spMend: "Mend", spBellow: "Bellow", spMight: "Pillar of Might", spRally: "Jakai's Rallying Cry"} {
		f.spell(id, name)
	}
	f.item(itBellow, "Bellow", scrollIcon, -1, 1)
	f.item(itMight, "Pillar of Might", scrollIcon, -1, 1)
	f.item(itFalchion, "Calamitous Falchion", 5, 12, 1)
	f.item(itPoleaxe, "Calamitous Poleaxe", 5, 15, 1)
	f.item(itSteelwind, "Steelwind", 5, 14, 1)
	f.item(itFoereaver, "Foereaver", 5, 15, 1)
	f.quest(qL20, "1-20/freeport/warrior/L20", "Captain Norgam", choice(fmt.Sprintf("Bellow=:%d", itBellow), fmt.Sprintf("Pillar of Might=:%d", itMight)))
	f.quest(qL30, "30,40,49/group-5-evil/L30-warrior", "Warlord Honan", choice(fmt.Sprintf("Roar=%d:", spRoar), fmt.Sprintf("Pillar of Strength=%d:", spStrength)))
	f.quest(qL40, "30,40,49/group-5-evil/L40-warrior", "Zentar Reese",
		choice(fmt.Sprintf("Howl and the Calamitous Falchion=%d:%d", spHowl, itFalchion), fmt.Sprintf("Pillar of Force and the Calamitous Poleaxe=%d:%d", spForce, itPoleaxe)))
	f.quest(qL49, "30,40,49/group-5-evil/L49-warrior", "Warlord Honan",
		choice(fmt.Sprintf("Battlecry=%d:", spBattlecry), fmt.Sprintf("Warcry=%d:", spWarcry)),
		choice(fmt.Sprintf("The Steelwind=:%d", itSteelwind), fmt.Sprintf("The Foereaver=:%d", itFoereaver)))
	f.quest(qEpic, "60/evil-tank/reward-warrior", "Ediopolus", choice(fmt.Sprintf("Jakai's Rallying Cry=%d:", spRally), fmt.Sprintf("Warcry=%d:", spWarcry)))
	f.quest(qOther, "23-47/23/p1", "Feria Hurkert", fmt.Sprint(spMend))
	f.done(qL20, "1:0")
	f.done(qL30, "1:0")
	f.done(qL40, "1:0")
	f.done(qL49, "")
	f.done(qEpic, "1:1")
	f.done(qOther, "")
	f.know(spBellow, spRoar, spHowl, spBattlecry, spWarcry, spMend)
	f.own(itFalchion, 1, -1, 0)
	f.own(itSteelwind, 1, -1, 1)
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		f.t.Fatalf("reload: %s", msg)
	}
	f.qe.ReloadCharacter(f.sid)
}

// pathsAsked talks to Anja, answers Choose a path again, and returns the
// picks offered.
func (f *rehabFixture) pathsAsked() []string {
	f.t.Helper()
	f.talk()
	m := f.pick(1)
	q, a := asked(m)
	if q != rehabAskPath {
		f.t.Fatalf("the paths not asked: %q", m)
	}
	return a
}

// picks is the character's kept picks of a quest.
func (f *rehabFixture) picks(quest int) string {
	var picks string
	f.db.QueryRow(`SELECT picks FROM charquests WHERE serverid = ? AND questid = ?`, f.sid, quest).Scan(&picks)
	return picks
}

// rowsAsTheyWere is whether every quest row of warriorPaths is still there,
// finished once, at no step: nothing was done again.
func (f *rehabFixture) rowsAsTheyWere() bool {
	for _, id := range []int{qL20, qL30, qL40, qL49, qEpic, qOther} {
		var step, done int
		if err := f.db.QueryRow(`SELECT step, done FROM charquests WHERE serverid = ? AND questid = ?`, f.sid, id).Scan(&step, &done); err != nil ||
			step != 0 || done != 1 {
			return false
		}
	}
	return true
}

// fillPack fills the pack with records of an item that does not stack until
// it holds n records.
func (f *rehabFixture) fillPack(n int) int {
	f.t.Helper()
	f.item(4999, "cracked shell", 5, -1, 1)
	for {
		var have int
		f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND location = 0`, f.sid).Scan(&have)
		if have >= n {
			return 4999
		}
		f.own(4999, 1, -1, 0)
	}
}

// count is how many messages of an opcode are in msgs.
func countOp(msgs []string, op uint16) int {
	n := 0
	for _, m := range msgs {
		if m == fmt.Sprintf("op:%04X", op) {
			n++
		}
	}
	return n
}

// A pick chosen again on the spot: the level 30 Roar for Pillar of Strength.
// The picks are offered a step each (the 49's two, read off the book and the
// bank as picks from before were not kept; the epic's Warcry is not the 49's),
// then the step's other options; on the yes Roar leaves the book (0x00B8),
// Pillar of Strength enters it (0x00B3), the pick is kept, every quest row stays
// as it was, no experience comes, the 40 and 49 picks stay, and the free respec
// is counted. The control: "Not now." changes nothing.
func TestRehabPathChosenAgainOnTheSpot(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	paths := f.pathsAsked()
	want := []string{"Level 20: Bellow", "Level 30: Roar", "Level 40: Howl and the Calamitous Falchion", "Level 49: Battlecry",
		"Level 49: The Steelwind"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("the paths: %q", paths)
	}
	m := f.pick(1)
	if q, a := asked(m); q != "What will you take instead of Roar?" || strings.Join(a, "|") != "Pillar of Strength" {
		t.Fatalf("the other options: %q %q", q, a)
	}
	m = f.pick(0)
	if !rehabSays(m, "Your level 30 path will be Pillar of Strength, not Roar.") ||
		!rehabSays(m, "Roar leaves your book and Pillar of Strength enters it.") {
		t.Fatalf("the question's words: %q", m)
	}
	if q, a := asked(m); q != "Shall I make the change? This is a free path respec (1 left after)." ||
		strings.Join(a, "|") != "Yes, change it.|Not now." {
		t.Fatalf("the question: %q %q", q, a)
	}
	f.pick(1) // not now
	if !f.knows(spRoar) || f.knows(spStrength) || f.picks(qL30) != "1:0" {
		t.Fatal("the control: Not now. changed something")
	}
	if u, _ := f.counts(database.ResetPath); u != 0 {
		t.Fatalf("Not now. counted a respec: %d", u)
	}

	f.pathsAsked()
	f.pick(1)
	f.pick(0)
	m = f.pick(0)
	if f.knows(spRoar) || !f.knows(spStrength) || f.picks(qL30) != "1:1" {
		t.Fatalf("after: Roar %v, Pillar of Strength %v, picks %q", f.knows(spRoar), f.knows(spStrength), f.picks(qL30))
	}
	if !f.rowsAsTheyWere() || !f.knows(spHowl) || !f.knows(spBattlecry) || f.held(itFalchion) != 1 || f.held(itSteelwind) != 1 {
		t.Errorf("more changed than the 30's pick: rows %v Howl %v Battlecry %v falchion %d steelwind %d", f.rowsAsTheyWere(),
			f.knows(spHowl), f.knows(spBattlecry), f.held(itFalchion), f.held(itSteelwind))
	}
	if countOp(m, uint16(protocol.OpcodeAbilityRemove)) != 1 || countOp(m, 0x00B3) != 1 || hasMsg(m, "xp") ||
		hasMsg(m, "box:"+questFinished) {
		t.Errorf("the client was told: %q", m)
	}
	if !rehabSays(m, "It is done. Your level 30 path is Pillar of Strength now.") ||
		!rehabSays(m, "Out of your book: Roar. Into it: Pillar of Strength.") ||
		!rehabSays(m, "That was a free path respec: 1 left.") {
		t.Errorf("the lines: %q", m)
	}
	if u, p := f.counts(database.ResetPath); u != 1 || p != 0 {
		t.Errorf("path used %d paid %d", u, p)
	}
}

// A pick's items change with it: the 40's Howl and the Calamitous Falchion for
// Pillar of Force and the Calamitous Poleaxe, the falchion taken back and the
// poleaxe given.
func TestRehabPathChangeTradesItsItems(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.pathsAsked()
	m := f.pick(2)
	if _, a := asked(m); strings.Join(a, "|") != "Pillar of Force and the Calamitous Poleaxe" {
		t.Fatalf("the other options: %q", a)
	}
	m = f.pick(0)
	if !rehabSays(m, "Howl leaves your book, Pillar of Force enters it, I take back the Calamitous Falchion and I give you the Calamitous Poleaxe.") {
		t.Fatalf("the question's words: %q", m)
	}
	m = f.pick(0)
	if f.knows(spHowl) || !f.knows(spForce) || f.held(itFalchion) != 0 || f.held(itPoleaxe) != 1 || f.picks(qL40) != "1:1" {
		t.Fatalf("after: Howl %v Force %v falchion %d poleaxe %d picks %q", f.knows(spHowl), f.knows(spForce), f.held(itFalchion),
			f.held(itPoleaxe), f.picks(qL40))
	}
	if !hasMsg(m, "take:0*1") || !hasMsg(m, "item") || !f.rowsAsTheyWere() {
		t.Errorf("the client was told %q; rows as they were %v", m, f.rowsAsTheyWere())
	}
}

// An item both options give stays where it is (neither taken nor given),
// and one the old pick gave that is held nowhere now (sold) is named as no
// longer the character's to take back, the new pick given all the same.
func TestRehabPathChangeItemsSharedAndLost(t *testing.T) {
	t.Run("shared", func(t *testing.T) {
		f := newRehabFixture(t)
		f.warriorPaths()
		f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ?`, choice(fmt.Sprintf("Howl and the Calamitous Falchion=%d:%d", spHowl, itFalchion),
			fmt.Sprintf("Pillar of Force and the Calamitous Falchion=%d:%d", spForce, itFalchion)), qL40)
		f.qe.ReloadQuests()
		f.pathsAsked()
		f.pick(2)
		m := f.pick(0)
		if !rehabSays(m, "Howl leaves your book and Pillar of Force enters it.") {
			t.Fatalf("the question's words: %q", m)
		}
		m = f.pick(0)
		if f.held(itFalchion) != 1 || hasMsg(m, "item") || strings.Contains(strings.Join(m, "\n"), "take:") || !f.knows(spForce) {
			t.Errorf("the falchion %d, Force %v (%q)", f.held(itFalchion), f.knows(spForce), m)
		}
	})
	t.Run("lost", func(t *testing.T) {
		f := newRehabFixture(t)
		f.warriorPaths()
		f.exec(`DELETE FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, itFalchion)
		f.lists()
		f.pathsAsked()
		f.pick(2)
		m := f.pick(0)
		if !rehabSays(m, "Howl leaves your book, Pillar of Force enters it and I give you the Calamitous Poleaxe.") {
			t.Fatalf("the question's words: %q", m)
		}
		m = f.pick(0)
		if f.held(itPoleaxe) != 1 || !rehabSays(m, "Out of your book: Howl. Into it: Pillar of Force. Given: Calamitous Poleaxe. No longer yours to take back: Calamitous Falchion.") {
			t.Errorf("the poleaxe %d (%q)", f.held(itPoleaxe), m)
		}
	})
}

// What else gives a spell keeps it in the book: Warcry, which the epic gave
// as well, stays when the 49's Warcry is changed back for Battlecry; Roar,
// which a merchant sells as a scroll, stays when the 30's Roar is changed for
// Pillar of Strength. The control is TestRehabPathChosenAgainOnTheSpot, where
// no merchant sells Roar and it leaves.
func TestRehabPathChangeKeepsWhatElseGives(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.exec(`UPDATE charquests SET picks = '1:1' WHERE serverid = ? AND questid = ?`, f.sid, qL49) // Warcry
	f.exec(`DELETE FROM spells WHERE serverid = ? AND spellid = ?`, f.sid, spBattlecry)
	f.qe.ReloadCharacter(f.sid)
	if paths := f.pathsAsked(); paths[3] != "Level 49: Warcry" {
		t.Fatalf("the paths: %q", paths)
	}
	f.pick(3)
	m := f.pick(0)
	if !rehabSays(m, "Battlecry enters it.") || rehabSays(m, "leaves your book") {
		t.Fatalf("the question's words: %q", m)
	}
	f.pick(0)
	if !f.knows(spWarcry) || !f.knows(spBattlecry) || f.picks(qL49) != "1:0" {
		t.Errorf("after: Warcry %v Battlecry %v picks %q", f.knows(spWarcry), f.knows(spBattlecry), f.picks(qL49))
	}

	f.item(4100, "Roar", scrollIcon, -1, 1)
	f.exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (3100, 1, 0, 4100, 0)`)
	f.pathsAsked()
	f.pick(1)
	m = f.pick(0)
	if !rehabSays(m, "Pillar of Strength enters it.") || rehabSays(m, "Roar leaves") {
		t.Fatalf("the question's words: %q", m)
	}
	f.pick(0)
	if !f.knows(spRoar) || !f.knows(spStrength) {
		t.Errorf("after: Roar %v Pillar of Strength %v", f.knows(spRoar), f.knows(spStrength))
	}
}

// A book that holds two options' spells nothing else explains had them from a
// class grant (a GM's grant_spell "all", #template): the change keeps them all
// and says so, the pick changed. The controls: TestRehabPathChosenAgainOnTheSpot's
// book, which holds only Roar, and Roar leaves; TestRehabInferredPickChanged's,
// whose 49 Warcry the epic explains, and Battlecry leaves.
func TestRehabPathChangeLeavesAGrantedBook(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.know(spStrength)
	f.pathsAsked()
	f.pick(1)
	m := f.pick(0)
	if !rehabSays(m, "You know every ability this path offers already: none leaves your book.") || rehabSays(m, "leaves your book and") {
		t.Fatalf("the question's words: %q", m)
	}
	m = f.pick(0)
	if !f.knows(spRoar) || !f.knows(spStrength) || f.picks(qL30) != "1:1" || countOp(m, uint16(protocol.OpcodeAbilityRemove)) != 0 {
		t.Errorf("after: Roar %v Pillar of Strength %v picks %q (%q)", f.knows(spRoar), f.knows(spStrength), f.picks(qL30), m)
	}
}

// A book with no room for the new pick's spells refuses before anything
// moves (the book's display indices end at 252). The control is
// TestRehabPathChosenAgainOnTheSpot's book.
func TestRehabPathChangeNeedsRoomInTheBook(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	for id := 20000; id < 20246; id++ {
		f.spell(id, fmt.Sprintf("Filler %d", id))
		f.know(id)
	}
	f.pathsAsked()
	f.pick(1)
	m := f.pick(0)
	if !rehabSays(m, "Your spell book has no room for Pillar of Strength.") || boxID(m) != 0 {
		t.Fatalf("a full book: %q", m)
	}
	if !f.knows(spRoar) || f.knows(spStrength) || f.picks(qL30) != "1:0" {
		t.Error("refused, yet changed")
	}
}

// A level 20 pick is a scroll: Bellow (scribed) for Pillar of Might, Bellow
// out of the book and Pillar of Might's scroll given to scribe.
func TestRehabLevel20PickChangesItsScroll(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.pathsAsked()
	f.pick(0)
	m := f.pick(0)
	if !rehabSays(m, "Bellow leaves your book and I give you the Pillar of Might.") {
		t.Fatalf("the question's words: %q", m)
	}
	f.pick(0)
	if f.knows(spBellow) || f.held(itMight) != 1 || f.picks(qL20) != "1:1" || !f.rowsAsTheyWere() {
		t.Errorf("after: Bellow %v, the Pillar of Might scroll %d, picks %q, rows %v", f.knows(spBellow), f.held(itMight),
			f.picks(qL20), f.rowsAsTheyWere())
	}
}

// A pick from before picks were kept is read off the book: the 49's
// Battlecry (Warcry is the epic's), changed for Warcry, which the book holds
// already through the epic: Battlecry leaves, nothing is taught twice, and the
// pick is kept from then on.
func TestRehabInferredPickChanged(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.pathsAsked()
	m := f.pick(3)
	if q, a := asked(m); q != "What will you take instead of Battlecry?" || strings.Join(a, "|") != "Warcry" {
		t.Fatalf("the other options: %q %q", q, a)
	}
	m = f.pick(0)
	if !rehabSays(m, "Battlecry leaves your book.") || rehabSays(m, "enters it") {
		t.Fatalf("the question's words: %q", m)
	}
	m = f.pick(0)
	if f.knows(spBattlecry) || !f.knows(spWarcry) || f.picks(qL49) != "1:1" || f.picks(qEpic) != "1:1" ||
		countOp(m, 0x00B3) != 0 {
		t.Errorf("after: Battlecry %v Warcry %v picks %q/%q (%q)", f.knows(spBattlecry), f.knows(spWarcry), f.picks(qL49), f.picks(qEpic), m)
	}
}

// A path's item worn is taken off first, and a pack with no room for the new
// item refuses: both before anything moves, the rose kept (it is used only
// when the change is done). The controls: taken off, and with the falchion in
// the pack (its record freed for the poleaxe), the change goes through for
// the rose.
func TestRehabPathChangeRefusedBeforeAnythingMoves(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.exec(`INSERT INTO charresets (serverid, kind, used) VALUES (?, 'path', 2)`, f.sid)
	f.own(database.RoseOfRehabilitation, 1, -1, 1)
	unchanged := func(what string) {
		t.Helper()
		if !f.knows(spHowl) || f.knows(spForce) || f.held(itFalchion) != 1 || f.held(itPoleaxe) != 0 ||
			f.held(database.RoseOfRehabilitation) != 1 || f.picks(qL40) != "1:0" {
			t.Fatalf("%s, yet changed: Howl %v Force %v falchion %d poleaxe %d roses %d picks %q", what, f.knows(spHowl),
				f.knows(spForce), f.held(itFalchion), f.held(itPoleaxe), f.held(database.RoseOfRehabilitation), f.picks(qL40))
		}
		if _, p := f.counts(database.ResetPath); p != 0 {
			t.Fatalf("%s, yet a rose counted", what)
		}
	}

	f.exec(`UPDATE charinventory SET equiploc = 14 WHERE serverid = ? AND patternid = ?`, f.sid, itFalchion)
	f.lists()
	f.pathsAsked()
	f.pick(2)
	m := f.pick(0)
	if !rehabSays(m, "Take off the Calamitous Falchion first: what a path gave goes back with it.") || boxID(m) != 0 {
		t.Fatalf("worn: %q", m)
	}
	unchanged("worn")

	// banked, the falchion frees no record in a full pack
	f.exec(`UPDATE charinventory SET equiploc = -1, location = 1 WHERE serverid = ? AND patternid = ?`, f.sid, itFalchion)
	filler := f.fillPack(40)
	f.pathsAsked()
	f.pick(2)
	m = f.pick(0)
	if !rehabSays(m, "Your pack is too full for the Calamitous Poleaxe. Make room and come back to me.") || boxID(m) != 0 {
		t.Fatalf("a full pack: %q", m)
	}
	unchanged("a full pack")

	// in the pack, taking it back frees the record the poleaxe needs
	f.exec(`DELETE FROM charinventory WHERE itemid = (SELECT MIN(itemid) FROM charinventory WHERE serverid = ? AND patternid = ?)`, f.sid, filler)
	f.exec(`UPDATE charinventory SET location = 0 WHERE serverid = ? AND patternid = ?`, f.sid, itFalchion)
	f.lists()
	f.pathsAsked()
	f.pick(2)
	m = f.pick(0)
	if q, _ := asked(m); q != "Shall I make the change, for a Rose of Rehabilitation?" {
		t.Fatalf("the control's question: %q", m)
	}
	f.pick(0)
	if f.knows(spHowl) || f.held(itFalchion) != 0 || f.held(itPoleaxe) != 1 || f.held(database.RoseOfRehabilitation) != 0 {
		t.Errorf("the control: Howl %v falchion %d poleaxe %d roses %d", f.knows(spHowl), f.held(itFalchion), f.held(itPoleaxe),
			f.held(database.RoseOfRehabilitation))
	}
	if u, p := f.counts(database.ResetPath); u != 2 || p != 1 {
		t.Errorf("path used %d paid %d: want 2 1", u, p)
	}
}

// Past the free path respecs a path needs a rose: offered for sale, refused
// with too little tunar (nothing taken), bought for exactly 10,000,000 and
// given, then the paths asked and one changed for it, the rose gone; the menu
// sells one too, at the GMs' price.
func TestRehabRoseForAPath(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.exec(`INSERT INTO charresets (serverid, kind, used) VALUES (?, 'path', 2)`, f.sid)
	m := f.talk()
	if _, a := asked(m); a[1] != "Choose a path again (a Rose of Rehabilitation)" {
		t.Fatalf("the menu: %q", a)
	}
	f.purse(9_999_999)
	m = f.pick(1)
	if !rehabSays(m, "I sell them for 10,000,000 tunar, and you carry 9,999,999 tunar.") || boxID(m) != 0 {
		t.Fatalf("too little tunar: %q", m)
	}
	if f.purseNow() != 9_999_999 || f.held(database.RoseOfRehabilitation) != 0 {
		t.Fatal("refused, yet changed")
	}
	f.purse(10_000_000)
	f.talk()
	m = f.pick(1)
	if q, a := asked(m); q != "Will you buy one?" || a[0] != "Yes, buy a rose." {
		t.Fatalf("the offer: %q", m)
	}
	m = f.pick(0)
	if f.purseNow() != 0 || f.held(database.RoseOfRehabilitation) != 1 || !rehabSays(m, "You paid 10,000,000 tunar.") {
		t.Fatalf("bought: purse %d, roses %d (%q)", f.purseNow(), f.held(database.RoseOfRehabilitation), m)
	}
	if q, _ := asked(m); q != rehabAskPath {
		t.Fatalf("the paths not asked after the rose: %q", m)
	}
	f.pick(1)
	m = f.pick(0)
	if q, _ := asked(m); q != "Shall I make the change, for a Rose of Rehabilitation?" {
		t.Fatalf("the question: %q", m)
	}
	m = f.pick(0)
	if f.held(database.RoseOfRehabilitation) != 0 || f.knows(spRoar) || !f.knows(spStrength) || !rehabSays(m, "You gave up a Rose of Rehabilitation.") {
		t.Fatalf("changed for the rose: roses %d, Roar %v, Pillar of Strength %v (%q)", f.held(database.RoseOfRehabilitation),
			f.knows(spRoar), f.knows(spStrength), m)
	}
	if u, p := f.counts(database.ResetPath); u != 2 || p != 1 {
		t.Errorf("path used %d paid %d: want 2 1", u, p)
	}

	// the menu's rose, at a GM's price
	f.exec(`UPDATE rehabsettings SET value = 4000000 WHERE setting = 'price_rose'`)
	f.purse(5_000_000)
	m = f.talk()
	if _, a := asked(m); a[2] != "Buy a Rose of Rehabilitation (4,000,000 tunar)" {
		t.Fatalf("the menu's rose: %q", a)
	}
	f.pick(2)
	f.pick(0)
	if f.purseNow() != 1_000_000 || f.held(database.RoseOfRehabilitation) != 1 {
		t.Errorf("bought from the menu: purse %d roses %d", f.purseNow(), f.held(database.RoseOfRehabilitation))
	}
}

// A rose is for paths alone: a mastery reset past the free one asks tunar
// and leaves the rose; the control: the next path change takes it.
func TestRehabRoseIsForPathsOnly(t *testing.T) {
	f := newRehabFixture(t)
	f.warriorPaths()
	f.exec(`INSERT INTO charresets (serverid, kind, used) VALUES (?, 'mastery', 1), (?, 'path', 2)`, f.sid, f.sid)
	f.own(database.RoseOfRehabilitation, 1, -1, 0)
	f.purse(1_000_000)
	f.giveMastery()
	f.resets()
	m := f.pick(0)
	if q, _ := asked(m); q != "Shall I begin? It costs 1,000,000 tunar." {
		t.Fatalf("the mastery question: %q", m)
	}
	f.pick(0)
	if len(f.tw.p.CMs) != 0 || f.purseNow() != 0 || f.held(database.RoseOfRehabilitation) != 1 {
		t.Fatalf("a mastery reset: ranks %v, purse %d, roses %d", f.tw.p.CMs, f.purseNow(), f.held(database.RoseOfRehabilitation))
	}
	f.pathsAsked()
	f.pick(1)
	f.pick(0)
	f.pick(0)
	if f.held(database.RoseOfRehabilitation) != 0 || f.knows(spRoar) {
		t.Errorf("the path: roses %d, Roar %v", f.held(database.RoseOfRehabilitation), f.knows(spRoar))
	}
}

// A pick made through the quests is kept with the quest's row, and the
// rehabilitator changes exactly it: Pillar of Strength, picked at Warlord
// Honan, is the path offered and the spell that goes for Roar.
func TestRehabPathPickedThroughTheQuests(t *testing.T) {
	f := newRehabFixture(t)
	f.spell(spRoar, "Roar")
	f.spell(spStrength, "Pillar of Strength")
	f.quest(qL30, "30,40,49/group-5-evil/L30-warrior", "Warlord Honan", choice(fmt.Sprintf("Roar=%d:", spRoar), fmt.Sprintf("Pillar of Strength=%d:", spStrength)))
	f.qe.ReloadQuests()
	honan := f.tw.add(3, &entity.NPC{ServerID: 3001, Name: "Warlord Honan", NPCType: 0x80, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5})
	f.tw.talk(honan) // given at once: no answer to take it
	questMsgs(f.tw.s)
	f.tw.talk(honan)
	m := questMsgs(f.tw.s)
	if q, a := asked(m); q != "Which do you take?" || len(a) != 2 {
		t.Fatalf("the pick: %q", m)
	}
	f.tw.pick(boxID(m), 1)
	questMsgs(f.tw.s)
	if f.picks(qL30) != "1:1" || !f.knows(spStrength) {
		t.Fatalf("the pick kept: %q, Pillar of Strength known %v", f.picks(qL30), f.knows(spStrength))
	}
	if paths := f.pathsAsked(); strings.Join(paths, "|") != "Level 30: Pillar of Strength" {
		t.Fatalf("the paths: %q", paths)
	}
	f.pick(0)
	f.pick(0)
	f.pick(0)
	if f.knows(spStrength) || !f.knows(spRoar) || f.picks(qL30) != "1:0" || !f.row(qL30) {
		t.Fatalf("after: Pillar of Strength %v, Roar %v, picks %q, row %v", f.knows(spStrength), f.knows(spRoar), f.picks(qL30), f.row(qL30))
	}
}

// Rehabilitator Frejo talks rather than fights: with the NPC bit (0x80, what
// tools/seed_rehabilitation.py gives him) his talk opens the menu and the
// World will not have him attacked; the control: without it he is a
// creature to fight.
func TestRehabFrejoTalksNotFights(t *testing.T) {
	f := newRehabFixture(t)
	frejo := &entity.NPC{ServerID: 101943, Name: "Rehabilitator Frejo", NPCType: 0x80, Level: 31,
		SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5, HP: 500, MaxHP: 500}
	id := f.tw.add(2, frejo)
	f.tw.talk(id)
	if m := questMsgs(f.tw.s); !IsRehabBox(boxID(m)) || !rehabSays(m, "Rehabilitator Frejo: Be welcome") {
		t.Fatalf("Frejo's talk: %q", m)
	}
	if f.tw.w.EngageAttack(f.tw.s, id, entity.Swing{Reach: 50}, time.Now()) {
		t.Error("Frejo with the NPC bit can be attacked")
	}
	frejo.NPCType = 0
	if !f.tw.w.EngageAttack(f.tw.s, id, entity.Swing{Reach: 50}, time.Now()) {
		t.Error("the control: Frejo without the NPC bit cannot be attacked")
	}
}

// A merchant will not buy a Rose of Rehabilitation (its worth is a path, and
// an epic's would be ten million tunar): refused with a line, the rose kept;
// the control: another item sells.
func TestMerchantsDoNotBuyTheRose(t *testing.T) {
	f := newRehabFixture(t)
	f.item(4242, "Shiny Pebble", 5, -1, 1)
	f.exec(`UPDATE itempattern SET itemcost = 10 WHERE patternid = 4242`)
	f.own(database.RoseOfRehabilitation, 1, -1, 0)
	f.own(4242, 1, -1, 0)
	merchant := &entity.NPC{ServerID: 3100, Name: "Merchant Yulia", NPCID: 3100, NPCType: 0x81, SpawnX: 23431.5, SpawnY: 54, SpawnZ: 19601.5}
	mid := f.tw.add(4, merchant)
	win := NewNPCWindowHandler(f.tw.mgr, f.inv)
	win.SetGameWorld(f.tw.w)
	f.tw.p.Tunar = 0
	if win.sell(f.tw.s, mid, 0, 1) {
		t.Fatal("a merchant bought the rose")
	}
	if got := texts(heard(t, f.tw.s)); !hasLine(got, roseNotSold) {
		t.Errorf("the refusal not said: %q", got)
	}
	f.tw.s.Queue().GetPending()
	if f.held(database.RoseOfRehabilitation) != 1 || f.tw.p.Tunar != 0 {
		t.Fatalf("refused, yet the rose %d, the purse %d", f.held(database.RoseOfRehabilitation), f.tw.p.Tunar)
	}
	if !win.sell(f.tw.s, mid, 1, 1) || f.tw.p.Tunar != 10 {
		t.Errorf("the control: the pebble did not sell (purse %d)", f.tw.p.Tunar)
	}
}
