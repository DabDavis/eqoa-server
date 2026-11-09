package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// tplCatalog is testCatalog and the Endurance line, so a warrior can spend
// 500 points: Brawn 1 and 2 (10, 15; STR 100 and 125), Battle Cry 1 (20; it
// needs Taunt, which it takes for Battle Cry), Endurance 1-3 (100, 100, 55)
// and Destroyer 1 (200, a Master Class).
func tplCatalog() []database.CMRankRow {
	all, races := 32767, 2047
	return append(testCatalog(),
		database.CMRankRow{ID: 50, Category: 0, Classes: all, Races: races, Mastery: "Endurance", Rank: 1, Name: "Endurance 1",
			Cost: 100, Level: 10, Effects: "STA:1", Enabled: true},
		database.CMRankRow{ID: 51, Category: 0, Classes: all, Races: races, Mastery: "Endurance", Rank: 2, Name: "Endurance 2",
			Parent: 50, Cost: 100, Level: 20, Effects: "STA:2", Enabled: true},
		database.CMRankRow{ID: 52, Category: 0, Classes: all, Races: races, Mastery: "Endurance", Rank: 3, Name: "Endurance 3",
			Parent: 51, Cost: 55, Level: 30, Effects: "STA:3", Enabled: true},
	)
}

// tplBuys is the order the warrior buys its 500 points' ranks in.
var tplBuys = []int{1, 2, 10, 50, 51, 52, 20}

// tplWant is those ranks as a template keeps them, in that order.
const tplWant = "1:10,2:15,10:20,50:100,51:100,52:55,20:200"

// tplFixture is a rehabilitator's fixture with tplCatalog, a level 60 human
// warrior as born (no training point spent: 100 unspent of 100; 700 mastery
// points, none spent; Taunt in the book) and a trainer.
type tplFixture struct {
	*rehabFixture
	train *TrainingHandler
	floor [7]int
}

func newTplFixture(t *testing.T) *tplFixture {
	t.Helper()
	f := newRehabFixture(t)
	f.tw.w.SetCMCatalog(tplCatalog())
	f.spell(500, "Taunt")
	f.spell(501, "Battle Cry")
	f.know(500)
	bs := entity.LookupBaseStats(0, 0)
	p := f.tw.p
	p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha = bs.Str, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha
	p.UnusedTP, p.TotalTP = 100, 100
	p.CMs = map[int]int{}
	p.Mastery = entity.Mastery{Points: 700}
	p.RecalcStats(true)
	f.exec(`UPDATE characters SET unusedTP = 100, totalTP = 100 WHERE serverid = ?`, f.sid)
	if err := f.store.SaveMastery(f.sid, database.MasteryRow{Points: 700}); err != nil {
		t.Fatal(err)
	}
	th := NewTrainingHandler(f.tw.mgr, f.store)
	th.SetGameWorld(f.tw.w)
	return &tplFixture{rehabFixture: f, train: th, floor: [7]int{bs.Str, bs.Sta, bs.Agi, bs.Dex, bs.Wis, bs.Int, bs.Cha}}
}

// trainStats spends training points as CLIENT's trainer does (0x001D: what
// each stat takes).
func (f *tplFixture) trainStats(adds [7]int) {
	f.t.Helper()
	buf := make([]byte, 64)
	w := buffer.NewWriter(buf)
	for _, v := range adds {
		w.Write7BitEncodedInt64(int64(v))
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateTrainingPoints)
	msg.SetData(buf[:w.Position()])
	before := f.tw.p.UnusedTP
	if err := f.train.HandleUpdateTrainingPoints(f.tw.s, msg); err != nil {
		f.t.Fatal(err)
	}
	if f.tw.p.UnusedTP == before {
		f.t.Fatalf("the trainer took nothing of %v (%d unspent)", adds, before)
	}
	f.tw.s.Queue().GetPending()
}

// buy buys ranks in the window (0x1401 sub-op 2), each a second after the
// last (charcms.bought), as a player buys them over time.
func (f *tplFixture) buy(ids ...int) {
	f.t.Helper()
	for _, id := range ids {
		req := make([]byte, 9)
		req[0] = cmBuy
		binary.LittleEndian.PutUint32(req[5:], uint32(id))
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassMastery)
		msg.SetData(req)
		if err := f.cm.HandleClassMastery(f.tw.s, msg); err != nil {
			f.t.Fatal(err)
		}
		if _, ok := f.tw.p.CMs[id]; !ok {
			f.t.Fatalf("mastery %d not bought: %q", id, questMsgs(f.tw.s))
		}
		var n int
		f.db.QueryRow(`SELECT COUNT(*) FROM charcms WHERE serverid = ?`, f.sid).Scan(&n)
		f.exec(`UPDATE charcms SET bought = ? WHERE serverid = ? AND cmid = ?`, 1000+n, f.sid, id)
	}
	f.tw.s.Queue().GetPending()
}

// spendAll is the warrior spending 70 training points (STR 50, STA 20) and
// 500 of its 700 mastery points on tplBuys.
func (f *tplFixture) spendAll() {
	f.t.Helper()
	f.trainStats([7]int{50, 20})
	f.buy(tplBuys...)
	if m := f.tw.p.Mastery; m.Points != 200 || m.Spent != 500 || f.tw.p.UnusedTP != 30 {
		f.t.Fatalf("spent: %+v, %d training points unspent", m, f.tw.p.UnusedTP)
	}
}

// templatesMenu talks to Anja and answers her menu's templates.
func (f *tplFixture) templatesMenu() []string {
	f.t.Helper()
	f.talk()
	return f.pick(2)
}

// saveTo saves in slot, confirmed: the last messages.
func (f *tplFixture) saveTo(slot int) []string {
	f.t.Helper()
	f.templatesMenu()
	f.pick(0)
	m := f.pick(byte(slot - 1))
	if q, _ := asked(m); q != rehabAskRemember {
		f.t.Fatalf("saving in slot %d asks %q (%q)", slot, q, m)
	}
	return f.pick(0)
}

// askUse asks to use slot: the question's messages, not answered.
func (f *tplFixture) askUse(slot int) []string {
	f.t.Helper()
	f.templatesMenu()
	f.pick(1)
	return f.pick(byte(slot - 1))
}

// resetBoth resets the masteries and the stat points at Anja.
func (f *tplFixture) resetBoth() {
	f.t.Helper()
	f.resets()
	f.pick(2)
	f.pick(0)
	if len(f.tw.p.CMs) != 0 || f.tw.p.Mastery.Spent != 0 {
		f.t.Fatalf("not reset: %v %+v", f.tw.p.CMs, f.tw.p.Mastery)
	}
}

// gain gives mastery points and training points, in the world and the rows.
func (f *tplFixture) gain(mastery, training int) {
	f.t.Helper()
	p := f.tw.p
	p.Mastery.Points += mastery
	p.UnusedTP += training
	p.TotalTP += training
	if err := f.store.SaveMastery(f.sid, MasteryRow(p.Mastery)); err != nil {
		f.t.Fatal(err)
	}
	f.exec(`UPDATE characters SET unusedTP = ?, totalTP = ? WHERE serverid = ?`, p.UnusedTP, p.TotalTP, f.sid)
}

// pickWithCM answers the last question with option n: what was sent, and the
// 0x1402 payloads among it (after the opcode).
func (f *tplFixture) pickWithCM(n byte) ([]string, [][]byte) {
	f.t.Helper()
	id := boxID(f.last)
	if id == 0 {
		f.t.Fatalf("no question to answer in %q", f.last)
	}
	f.tw.pick(id, n)
	var cm [][]byte
	for _, m := range f.tw.s.Queue().PeekPending() {
		if d := m.Data(); protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeClassMasteryServer {
			cm = append(cm, d[2:])
		}
	}
	f.last = questMsgs(f.tw.s)
	return f.last, cm
}

// cmReply is a 0x1402 as CLIENT's Recv1402 reads it: the sub-op, the rank,
// the points (sub-ops 5 and 3) and the state (5 and 6).
type cmReply struct {
	sub, id, points, spent int
	state                  cmStateRead
}

// readReplies reads each payload to its last byte, as CLIENT does.
func readReplies(t *testing.T, raw [][]byte) []cmReply {
	t.Helper()
	var out []cmReply
	for _, b := range raw {
		r := &cmReader{b: b}
		c := cmReply{sub: r.u8()}
		switch c.sub {
		case cmRespBought:
			r.u32()
			c.id, c.points, c.spent = int(r.u32()), int(r.u32()), int(r.u32())
			c.state = r.state()
		case cmRespRemoved:
			r.u32()
			c.id = int(r.u32())
			c.state = r.state()
		case cmRespPoints:
			c.points, c.spent = int(r.u32()), int(r.u32())
		default:
			t.Errorf("a 0x1402 sub-op %d after a template", c.sub)
		}
		if r.err != nil || r.o != len(r.b) {
			t.Errorf("a sub-op %d does not read as CLIENT reads it: %v, %d of %d bytes", c.sub, r.err, r.o, len(r.b))
		}
		out = append(out, c)
	}
	return out
}

// slotRow is a character's template row: its tp columns, its cms and level.
func (f *rehabFixture) slotRow(slot int) (tp [7]int, cms string, level int, ok bool) {
	err := f.db.QueryRow(`SELECT tpstrength, tpstamina, tpagility, tpdexterity, tpwisdom, tpintel, tpcharisma, cms, level
		FROM rehabtemplates WHERE serverid = ? AND slot = ?`, f.sid, slot).
		Scan(&tp[0], &tp[1], &tp[2], &tp[3], &tp[4], &tp[5], &tp[6], &cms, &level)
	return tp, cms, level, err == nil
}

// ownedRows is the character's charcms, "id:cost" by id.
func (f *rehabFixture) ownedRows() string {
	f.t.Helper()
	owned, err := f.store.LoadOwnedCMs(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return database.FormatTemplateCMs(owned)
}

// ownedNow is the player's ranks, "id:cost" by id.
func ownedNow(p *entity.Player) string {
	var ids []int
	for id := range p.CMs {
		ids = append(ids, id)
	}
	var out []database.OwnedCM
	for _, id := range sortedInts(ids) {
		out = append(out, database.OwnedCM{ID: id, Cost: p.CMs[id]})
	}
	return database.FormatTemplateCMs(out)
}

func sortedInts(v []int) []int {
	out := append([]int(nil), v...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// trainingRow is the row's tp columns and unspent and total points.
func (f *rehabFixture) trainingRow() ([7]int, int, int) {
	f.t.Helper()
	rec, err := f.store.GetByServerID(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return [7]int{rec.TPStrength, rec.TPStamina, rec.TPAgility, rec.TPDexterity, rec.TPWisdom, rec.TPIntel, rec.TPCharisma},
		rec.UnusedTP, rec.TotalTP
}

// masteryRow is the row's mastery points.
func (f *rehabFixture) masteryRow() database.MasteryRow {
	f.t.Helper()
	m, err := f.store.LoadMastery(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

// petition 1322's example, whole: a warrior spends 70 training points and 500
// of its 700 mastery points, saves slot 1 (the row keeps each stat's points and
// the ranks with their costs in the order bought), resets everything, gains 5
// mastery points and 6 training points, and uses slot 1: the same 500 points on
// the same ranks, the same 70 training points on the same stats, and the 205
// mastery and 36 training points left unspent; the world, the rows, the book
// and the client's window agree. The controls: slot 2 stays empty, and the
// reset before the use left all 705 and 106 unspent.
func TestRehabTemplateSpendsOnlyWhatItSpent(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	m := f.saveTo(1)
	if !rehabSays(m, "Rehabilitator Anja: It is done. Slot 1 holds 70 training points (STR 50, STA 20) and 7 masteries for 500 points (Master Class: Destroyer) now.") {
		t.Fatalf("saved: %q", m)
	}
	tp, cms, level, ok := f.slotRow(1)
	if !ok || tp != [7]int{50, 20} || cms != tplWant || level != 60 {
		t.Fatalf("slot 1's row: %v %v %q level %d", ok, tp, cms, level)
	}
	if _, _, _, ok := f.slotRow(2); ok {
		t.Errorf("the control: slot 2 has a row")
	}

	f.resetBoth()
	f.gain(5, 6)
	p := f.tw.p
	if p.Mastery.Points != 705 || p.UnusedTP != 106 || len(p.CMs) != 0 {
		t.Fatalf("the control: after the reset and the gain, %+v, %d training points unspent, %v", p.Mastery, p.UnusedTP, p.CMs)
	}

	m = f.askUse(1)
	if q, a := asked(m); q != rehabAskBegin || strings.Join(a, "|") != "Yes, begin.|Not now." {
		t.Fatalf("the question: %q %q (%q)", q, a, m)
	}
	for _, want := range []string{
		"I will spend you as slot 1 keeps you: 70 training points (STR 50, STA 20) and 7 masteries for 500 points (Master Class: Destroyer).",
		"205 of your 705 mastery points and 36 of your 106 training points stay yours to spend.",
	} {
		if !rehabSays(m, want) {
			t.Errorf("the question does not say %q: %q", want, m)
		}
	}
	if len(p.CMs) != 0 || p.Mastery.Points != 705 {
		t.Fatalf("asking spent: %v %+v", p.CMs, p.Mastery)
	}

	m, raw := f.pickWithCM(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,20:200,50:100,51:100,52:55" {
		t.Errorf("the ranks after: %q", got)
	}
	if p.Mastery.Points != 205 || p.Mastery.Spent != 500 {
		t.Errorf("the points after: %+v, want 205 unspent and 500 spent", p.Mastery)
	}
	if got := f.ownedRows(); got != "1:10,2:15,10:20,20:200,50:100,51:100,52:55" {
		t.Errorf("charcms after: %q", got)
	}
	if mr := f.masteryRow(); mr.Points != 205 || mr.Spent != 500 {
		t.Errorf("charmastery after: %+v", mr)
	}
	rowTP, unused, total := f.trainingRow()
	if rowTP != [7]int{50, 20} || unused != 36 || total != 106 || p.UnusedTP != 36 {
		t.Errorf("the training after: row %v %d of %d, world %d unspent", rowTP, unused, total, p.UnusedTP)
	}
	if p.BaseStr != f.floor[0]+50 || p.BaseSta != f.floor[1]+20 || p.BaseAgi != f.floor[2] {
		t.Errorf("the base after: STR %d STA %d AGI %d (born %v)", p.BaseStr, p.BaseSta, p.BaseAgi, f.floor)
	}
	if !f.knows(501) || f.knows(500) {
		t.Errorf("the book after: Battle Cry %v, Taunt %v (Battle Cry 1 takes Taunt for Battle Cry)", f.knows(501), f.knows(500))
	}
	if !rehabSays(m, "It is done. You have spent 70 training points (STR 50, STA 20) and 7 masteries for 500 points (Master Class: Destroyer).") ||
		!rehabSays(m, "205 of your 705 mastery points and 36 of your 106 training points are yours to spend.") {
		t.Errorf("the lines after: %q", m)
	}
	if !hasMsg(m, "op:001D") || !hasMsg(m, "op:00B3") || !hasMsg(m, "op:00B8") {
		t.Errorf("the trainer or the book not told: %q", m)
	}
	replies := readReplies(t, raw)
	var bought []int
	for i, c := range replies {
		switch {
		case c.sub == cmRespBought:
			bought = append(bought, c.id)
			if c.points != 205 || c.spent != 500 {
				t.Errorf("Recv05 %d's points: %d %d", c.id, c.points, c.spent)
			}
		case c.sub == cmRespPoints && i == len(replies)-1:
			if c.points != 205 || c.spent != 500 {
				t.Errorf("Recv03's points: %d %d", c.points, c.spent)
			}
		default:
			t.Errorf("reply %d: sub-op %d", i, c.sub)
		}
	}
	if fmt.Sprint(bought) != fmt.Sprint(tplBuys) {
		t.Errorf("the window told of %v bought, want %v", bought, tplBuys)
	}
	if len(replies) > 0 {
		st := replies[0].state
		if st.stats[0] != 23 || st.stats[1] != 6 || st.bonuses[0] != 85 {
			t.Errorf("the window's state: STR %d STA %d STR cap %d (Brawn 1+2 and Destroyer 1: 23, 15+20+50; Endurance: 6)",
				st.stats[0], st.stats[1], st.bonuses[0])
		}
	}
	if u, _ := f.counts(database.ResetTemplate); u != 1 {
		t.Errorf("the use counted %d times", u)
	}
}

// Fewer mastery points than the template spent (a rank removed in the window
// cost its points: 450 now): its ranks bought in the order bought until the
// points give out, Destroyer 1 (last bought, 200) left out and said; nothing
// past the template bought, no point below 0. Bought in another order the
// same points would have taken Destroyer and left Endurance 3: the order is
// the template's.
func TestRehabTemplateWithFewerMasteryPoints(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	p := f.tw.p
	p.Mastery = entity.Mastery{Points: 450}
	if err := f.store.SaveMastery(f.sid, database.MasteryRow{Points: 450}); err != nil {
		t.Fatal(err)
	}
	m := f.askUse(1)
	if !rehabSays(m, "I cannot buy 1 of its masteries: Destroyer 1 (too few mastery points are left for it).") ||
		!rehabSays(m, "150 of your 450 mastery points") {
		t.Fatalf("the question: %q", m)
	}
	f.pick(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,50:100,51:100,52:55" {
		t.Errorf("the ranks: %q", got)
	}
	if p.Mastery.Points != 150 || p.Mastery.Spent != 300 {
		t.Errorf("the points: %+v", p.Mastery)
	}
	if !rehabSays(f.last, "Not bought: Destroyer 1 (too few mastery points are left for it).") {
		t.Errorf("the lines: %q", f.last)
	}
	if f.tw.p.UnusedTP != 30 {
		t.Errorf("the training: %d unspent, want 30", f.tw.p.UnusedTP)
	}
}

// Fewer training points than the template spent (35 of its 70): each stat
// takes its share (STR 25, STA 10) and nothing is left unspent; the masteries
// see the base the template's training makes, so Brawn 2 (STR 125) is left out
// with the purchase's own words, the rest bought. The control is the first
// test: with all 70, Brawn 2 is bought.
func TestRehabTemplateWithFewerTrainingPoints(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	p := f.tw.p
	p.UnusedTP, p.TotalTP = 35, 35
	f.exec(`UPDATE characters SET unusedTP = 35, totalTP = 35 WHERE serverid = ?`, f.sid)
	m := f.askUse(1)
	if !rehabSays(m, "You hold 35 training points of the 70 it spends: each stat takes its share.") ||
		!rehabSays(m, "Brawn 2 (your STR must be at least 125 to buy Brawn 2)") {
		t.Fatalf("the question: %q", m)
	}
	f.pick(0)
	rowTP, unused, _ := f.trainingRow()
	if rowTP != [7]int{25, 10} || unused != 0 || p.UnusedTP != 0 || p.BaseStr != f.floor[0]+25 {
		t.Errorf("the training: row %v %d unspent, world %d unspent, STR %d", rowTP, unused, p.UnusedTP, p.BaseStr)
	}
	if _, ok := p.CMs[2]; ok || len(p.CMs) != 6 {
		t.Errorf("the ranks: %q", ownedNow(p))
	}
	if p.Mastery.Points != 700-485 || p.Mastery.Spent != 485 {
		t.Errorf("the points: %+v", p.Mastery)
	}
}

// A rank the character no longer meets is left out, and said: after the save
// a GM raised Endurance 2 to level 61, hid Battle Cry 1 (enabled 0) and took
// Destroyer 1 out of the catalog, so Endurance 2 (its level), Endurance 3 (its
// parent), Battle Cry 1 (by name) and Destroyer 1 (which has no name any more)
// are left out, each in its line, and the rest bought. The control: the
// catalog as it was, and the same template buys them all.
func TestRehabTemplateRankNoLongerMet(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	var cat []database.CMRankRow
	for _, r := range tplCatalog() {
		switch r.ID {
		case 20:
			continue
		case 10:
			r.Enabled = false
		case 51:
			r.Level = 61
		}
		cat = append(cat, r)
	}
	f.tw.w.SetCMCatalog(cat)
	m := f.askUse(1)
	for _, want := range []string{"I cannot buy 4 of its masteries:", "Battle Cry 1 (that mastery is no longer offered)",
		"Endurance 2 (you must be level 61 to buy Endurance 2)", "Endurance 3 (you must have Endurance 2 first)",
		"a mastery no longer offered"} {
		if !rehabSays(m, want) {
			t.Errorf("the question does not say %q: %q", want, m)
		}
	}
	f.pick(0)
	p := f.tw.p
	if got := ownedNow(p); got != "1:10,2:15,50:100" {
		t.Errorf("the ranks: %q", got)
	}
	if p.Mastery.Points != 700-125 || p.Mastery.Spent != 125 {
		t.Errorf("the points: %+v", p.Mastery)
	}
	if !rehabSays(f.last, "Not bought:") {
		t.Errorf("the lines: %q", f.last)
	}

	// the control
	f.tw.w.SetCMCatalog(tplCatalog())
	f.askUse(1)
	f.pick(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,20:200,50:100,51:100,52:55" || p.Mastery.Points != 200 {
		t.Errorf("the control: %q %+v", got, p.Mastery)
	}
	if rehabSays(f.last, "Not bought:") {
		t.Errorf("the control: %q", f.last)
	}
}

// A slot saved again holds the new spending, after its question says what it
// holds now and what replaces it; the other slot is untouched. The slots'
// answers say what each holds.
func TestRehabTemplateSlotOverwrite(t *testing.T) {
	f := newTplFixture(t)
	f.trainStats([7]int{30})
	f.buy(1)
	f.saveTo(1)
	f.resetBoth()
	f.trainStats([7]int{0, 20})
	f.buy(50)
	f.saveTo(2)
	f.resetBoth()
	f.trainStats([7]int{50})
	f.buy(1, 2)

	f.templatesMenu()
	if !rehabSays(f.last, "Slot 1 holds 30 training points (STR 30) and 1 mastery for 10 points, kept at level 60 on ") ||
		!rehabSays(f.last, "Slot 2 holds 20 training points (STA 20) and 1 mastery for 100 points, kept at level 60 on ") {
		t.Fatalf("the templates' menu: %q", f.last)
	}
	m := f.pick(0)
	if q, a := asked(m); q != rehabAskSaveSlot || strings.Join(a, "|") !=
		"Slot 1: 1 mastery (10 points), 30 training points|Slot 2: 1 mastery (100 points), 20 training points" {
		t.Fatalf("the slots: %q %q", q, a)
	}
	m = f.pick(0)
	if !rehabSays(m, "You have spent 50 training points (STR 50) and 2 masteries for 25 points.") ||
		!rehabSays(m, "Slot 1 holds 30 training points (STR 30) and 1 mastery for 10 points now. It will hold this instead.") {
		t.Fatalf("the question: %q", m)
	}
	f.pick(0)
	if tp, cms, _, _ := f.slotRow(1); tp != [7]int{50} || cms != "1:10,2:15" {
		t.Errorf("slot 1: %v %q", tp, cms)
	}
	if tp, cms, _, _ := f.slotRow(2); tp != [7]int{0, 20} || cms != "50:100" {
		t.Errorf("the control: slot 2 changed: %v %q", tp, cms)
	}
	// Not now keeps the slot as it is
	f.resetBoth()
	f.trainStats([7]int{10})
	f.templatesMenu()
	f.pick(0)
	f.pick(1)
	if m := f.pick(1); !rehabSays(m, rehabAsYouWish) {
		t.Errorf("Not now: %q", m)
	}
	if tp, cms, _, _ := f.slotRow(2); tp != [7]int{0, 20} || cms != "50:100" {
		t.Errorf("Not now saved slot 2: %v %q", tp, cms)
	}
}

// Using a template is free and counted: the GMs' old prices set and no tunar,
// the purse untouched, a "template" reset counted. A second use when the
// character is spent as the slot keeps it already changes nothing, says so and
// is not counted. Resets stay free: the counts of the reset before are a
// mastery and a training reset, nothing paid.
func TestRehabTemplateUseIsFreeAndCounted(t *testing.T) {
	f := newTplFixture(t)
	f.pricedAsBefore()
	f.purse(0)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	f.askUse(1)
	m := f.pick(0)
	f.saysNothingOfAPrice(m)
	if f.purseNow() != 0 || len(f.tw.p.CMs) != 7 {
		t.Fatalf("the use: purse %d, ranks %v", f.purseNow(), f.tw.p.CMs)
	}
	if u, p := f.counts(database.ResetTemplate); u != 1 || p != 0 {
		t.Errorf("counted: used %d paid %d", u, p)
	}
	if u, p := f.counts(database.ResetMastery); u != 1 || p != 0 {
		t.Errorf("the reset: used %d paid %d", u, p)
	}
	m = f.askUse(1)
	if !rehabSays(m, "You are spent as slot 1 keeps you already: there is nothing to change.") || boxID(m) != 0 {
		t.Errorf("a second use: %q", m)
	}
	if u, _ := f.counts(database.ResetTemplate); u != 1 {
		t.Errorf("nothing changed, yet counted: %d", u)
	}
}

// A question is done only on its terms: a mastery point earned between the
// question and the yes asks again ("Something has changed since I asked.")
// with what is true now, and does nothing; the second yes uses the template.
// Saving the same: a rank bought between asks again.
func TestRehabTemplateAskedAgainWhenMoved(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()

	// experience toward the next mastery point is not a change of terms
	f.askUse(1)
	f.tw.p.Mastery.XP += 77
	if m := f.pick(0); rehabSays(m, rehabTplMoved) || len(f.tw.p.CMs) != 7 {
		t.Fatalf("mastery experience asked again: %q", m)
	}
	f.resetBoth()
	f.askUse(1)
	f.gain(1, 0)
	m := f.pick(0)
	if !rehabSays(m, rehabTplMoved) || !rehabSays(m, "201 of your 701 mastery points") {
		t.Fatalf("asked again: %q", m)
	}
	if len(f.tw.p.CMs) != 0 {
		t.Fatalf("the first yes spent: %v", f.tw.p.CMs)
	}
	f.pick(0)
	if len(f.tw.p.CMs) != 7 || f.tw.p.Mastery.Points != 201 || f.tw.p.Mastery.XP != 77 {
		t.Errorf("the second yes: %v %+v", f.tw.p.CMs, f.tw.p.Mastery)
	}

	// saving: a rank bought between
	f.resetBoth()
	f.trainStats([7]int{50})
	f.buy(1)
	f.templatesMenu()
	f.pick(0)
	f.pick(1)
	f.buy(2)
	m = f.pick(0)
	if !rehabSays(m, rehabTplMoved) || !rehabSays(m, "2 masteries for 25 points") {
		t.Fatalf("saving asked again: %q", m)
	}
	if _, cms, _, ok := f.slotRow(2); ok {
		t.Fatalf("the first yes saved: %q", cms)
	}
	f.pick(0)
	if _, cms, _, _ := f.slotRow(2); cms != "1:10,2:15" {
		t.Errorf("saved: %q", cms)
	}
}

// The client is told the change and no more: holding the template's ranks but
// Defender 1 for Destroyer 1, using it sends one removal (Recv06, Defender 1),
// one purchase (Recv05, Destroyer 1) and the points, and touches no ability
// of a rank held before and after (Battle Cry stays at its place in the book).
// The control: from nothing held, all seven are purchases.
func TestRehabTemplateTellsTheChangeAlone(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	f.trainStats([7]int{50, 20})
	f.buy(1, 2, 10, 50, 51, 52, 21)
	var order int
	f.db.QueryRow(`SELECT addedorder FROM spells WHERE serverid = ? AND spellid = 501`, f.sid).Scan(&order)
	f.askUse(1)
	if !rehabSays(f.last, "I will give back the 70 training points and 7 masteries (500 points) you have spent.") ||
		!rehabSays(f.last, "Then I will spend you as slot 1 keeps you: 70 training points (STR 50, STA 20) and 7 masteries for 500 points (Master Class: Destroyer).") {
		t.Errorf("the question: %q", f.last)
	}
	m, raw := f.pickWithCM(0)
	replies := readReplies(t, raw)
	var got []string
	for _, c := range replies {
		got = append(got, fmt.Sprintf("%d:%d", c.sub, c.id))
	}
	if strings.Join(got, " ") != "6:21 5:20 3:0" {
		t.Errorf("the window told %q, want a removal of 21, a purchase of 20 and the points", got)
	}
	if hasMsg(m, "op:00B3") || hasMsg(m, "op:00B8") {
		t.Errorf("the book changed for ranks held before and after: %q", m)
	}
	var now int
	f.db.QueryRow(`SELECT addedorder FROM spells WHERE serverid = ? AND spellid = 501`, f.sid).Scan(&now)
	if now != order {
		t.Errorf("Battle Cry moved in the book: %d -> %d", order, now)
	}
	if got := ownedNow(f.tw.p); got != "1:10,2:15,10:20,20:200,50:100,51:100,52:55" {
		t.Errorf("the ranks: %q", got)
	}
	// the control
	f.resetBoth()
	f.askUse(1)
	_, raw = f.pickWithCM(0)
	n := 0
	for _, c := range readReplies(t, raw) {
		if c.sub == cmRespBought {
			n++
		}
	}
	if n != 7 {
		t.Errorf("the control: %d purchases told", n)
	}
}

// With nothing spent there is nothing to save, and with nothing saved nothing
// to use: each said, nothing written. The control: a training point spent is
// something to save.
func TestRehabTemplateNothingToSaveOrUse(t *testing.T) {
	f := newTplFixture(t)
	f.templatesMenu()
	if !rehabSays(f.last, "Slot 1 holds nothing yet.") || !rehabSays(f.last, "Slot 2 holds nothing yet.") {
		t.Fatalf("the menu: %q", f.last)
	}
	f.pick(0)
	if m := f.pick(0); !rehabSays(m, rehabNothingToKeep) || boxID(m) != 0 {
		t.Errorf("nothing to save: %q", m)
	}
	f.templatesMenu()
	if m := f.pick(1); !rehabSays(m, rehabNoTemplates) || boxID(m) != 0 {
		t.Errorf("nothing to use: %q", m)
	}
	if _, _, _, ok := f.slotRow(1); ok {
		t.Errorf("a row written")
	}
	f.trainStats([7]int{1})
	f.saveTo(1)
	if tp, cms, _, ok := f.slotRow(1); !ok || tp != [7]int{1} || cms != "" {
		t.Errorf("the control: %v %v %q", ok, tp, cms)
	}
	// an empty slot asked for
	f.templatesMenu()
	f.pick(1)
	if m := f.pick(1); !rehabSays(m, "Slot 2 holds nothing yet: save a template in it first.") {
		t.Errorf("an empty slot: %q", m)
	}
}

// What the templates say fits CLIENT's boxes uncut: an answer 63 characters, a
// question 127, and a slot's answer at its longest (99 masteries for 1,500
// points and 482 training points).
func TestRehabTemplateWordsFitTheBoxes(t *testing.T) {
	for _, q := range []string{rehabAskTemplates, rehabAskSaveSlot, rehabAskUseSlot, rehabAskRemember, rehabAskMenu} {
		if utf8.RuneCountInString(q) > questAskChars {
			t.Errorf("%q is past a question's %d", q, questAskChars)
		}
	}
	tpl := database.RehabTemplate{TP: [7]int{100, 100, 100, 100, 82}}
	for i := 0; i < 99; i++ {
		tpl.CMs = append(tpl.CMs, database.OwnedCM{ID: i + 1, Cost: 15})
	}
	for _, a := range []string{rehabTemplatesAnswer, rehabTplSaveLabel, rehabTplUseLabel, rehabRememberYes,
		slotLabel(2, tpl, true), slotLabel(1, database.RehabTemplate{}, false)} {
		if utf8.RuneCountInString(a) > questAnswerChars {
			t.Errorf("%q is past an answer's %d", a, questAnswerChars)
		}
	}
	if got := slotLabel(2, tpl, true); got != "Slot 2: 99 masteries (1485 points), 482 training points" {
		t.Errorf("the longest label: %q", got)
	}
}

// wholeBoxes fails for a box past CLIENT's 217 characters or one that ends
// mid-sentence (a line say had to cut where no sentence ended).
func wholeBoxes(t *testing.T, msgs []string) {
	t.Helper()
	for _, m := range msgs {
		text, ok := strings.CutPrefix(m, "box:")
		if !ok {
			continue
		}
		if n := utf8.RuneCountInString(text); n > questBoxChars {
			t.Errorf("a box of %d characters: %q", n, text)
		}
		if !strings.HasSuffix(text, ".") && !strings.HasSuffix(text, "?") && !strings.HasSuffix(text, "!") {
			t.Errorf("a box cut mid-sentence: %q", text)
		}
	}
}

// At its longest, what a use says still falls in whole boxes: every stat
// trained, and thirty ranks of long names the character no longer meets
// (level 61 each), at Rehabilitator Anja and at a speaker of a longer name.
func TestRehabTemplateLongestLinesFitWhole(t *testing.T) {
	f := newTplFixture(t)
	var cat []database.CMRankRow
	tpl := database.RehabTemplate{Slot: 1, Level: 60, TP: [7]int{14, 14, 14, 14, 14, 14, 14}}
	for i := 0; i < 30; i++ {
		id := 1000 + i
		cat = append(cat, database.CMRankRow{ID: id, Category: 0, Classes: 32767, Races: 2047, Mastery: "Glacier",
			Rank: i + 1, Name: fmt.Sprintf("Mastery of the Unending Northern Glacier, Rank %d", i+1), Cost: 40, Level: 61, Enabled: true})
		tpl.CMs = append(tpl.CMs, database.OwnedCM{ID: id, Cost: 40})
	}
	f.tw.w.SetCMCatalog(cat)
	if err := f.db.SaveRehabTemplate(f.sid, tpl, f.r.now()); err != nil {
		t.Fatal(err)
	}
	all := f.templatesMenu()
	m := f.pick(1)
	all = append(all, m...)
	m = f.pick(0)
	all = append(all, m...)
	if !rehabSays(m, "I cannot buy 30 of its masteries:") || !rehabSays(m, "and 24 more.") {
		t.Fatalf("the question: %q", m)
	}
	m = f.pick(0)
	all = append(all, m...)
	if !rehabSays(m, "Not bought:") || f.tw.p.UnusedTP != 2 {
		t.Fatalf("the use: %q, %d unspent", m, f.tw.p.UnusedTP)
	}
	wholeBoxes(t, all)

	// spent as it keeps already: said so, nothing asked
	f.askUse(1)
	if !rehabSays(f.last, "You are spent as slot 1 keeps you already: there is nothing to change.") || boxID(f.last) != 0 {
		t.Errorf("a second use: %q", f.last)
	}
	// after a reset, at a speaker of a longer name (a box holds less after it):
	// whole boxes still
	f.resetBoth()
	f.r.Offer(f.tw.s, f.tw.p, "Rehabilitator Anja of the Tea Garden of Muniel")
	f.last = questMsgs(f.tw.s)
	f.pick(2)
	f.pick(1)
	m = f.pick(0)
	wholeBoxes(t, m)
	if !rehabSays(m, "I cannot buy 30 of its masteries:") {
		t.Errorf("at the longer name: %q", m)
	}
}

// withCost is tplCatalog with ranks' costs changed (a GM's edit).
func withCost(costs map[int]int) []database.CMRankRow {
	cat := tplCatalog()
	for i := range cat {
		if c, ok := costs[cat[i].ID]; ok {
			cat[i].Cost = c
		}
	}
	return cat
}

// Never more than the template spent, though the character holds 705: a GM
// raised Endurance 3 from 55 to 80 after the save, so it is bought after the
// rest and left out, in its own words, and Destroyer 1 (the same price) is
// bought; 445 spent, 260 left. Endurance 1 raised to 150 instead: bought last,
// with Endurance 2 after it, and Endurance 3 (its own price unchanged) is what
// would pass the 500. The control: Endurance 3 cheaper (30) is bought, for less.
func TestRehabTemplateNeverSpendsMoreThanItSpent(t *testing.T) {
	f := newTplFixture(t)
	f.spendAll()
	f.saveTo(1)
	f.resetBoth()
	f.gain(5, 0)
	p := f.tw.p

	f.tw.w.SetCMCatalog(withCost(map[int]int{52: 80}))
	m := f.askUse(1)
	if !rehabSays(m, "I cannot buy 1 of its masteries: Endurance 3 (it costs 80 now, not 55: past the 500 points the template spent).") {
		t.Fatalf("the question: %q", m)
	}
	f.pick(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,20:200,50:100,51:100" || p.Mastery.Spent != 445 || p.Mastery.Points != 260 {
		t.Errorf("Endurance 3 dearer: %q %+v", got, p.Mastery)
	}

	f.resetBoth()
	f.tw.w.SetCMCatalog(withCost(map[int]int{50: 150}))
	m = f.askUse(1)
	if !rehabSays(m, "Endurance 3 (that would spend more than the 500 points the template spent)") {
		t.Fatalf("Endurance 1 dearer, the question: %q", m)
	}
	f.pick(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,20:200,50:150,51:100" || p.Mastery.Spent != 495 {
		t.Errorf("Endurance 1 dearer: %q %+v", got, p.Mastery)
	}

	// the control
	f.resetBoth()
	f.tw.w.SetCMCatalog(withCost(map[int]int{52: 30}))
	m = f.askUse(1)
	if rehabSays(m, "I cannot buy") {
		t.Errorf("the control: %q", m)
	}
	f.pick(0)
	if got := ownedNow(p); got != "1:10,2:15,10:20,20:200,50:100,51:100,52:30" || p.Mastery.Spent != 475 || p.Mastery.Points != 230 {
		t.Errorf("the control: %q %+v", got, p.Mastery)
	}
}
