package handlers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The epic exchange's records: the warrior's level 49 path epics (regular,
// Ascended, an imported Steelwind), a level 60 epic, an Heirloom Steelwind.
const (
	epAscSteelwind, epAscFoereaver = 980042, 980093
	epImpSteelwind                 = 990494
	epGreatsword                   = 980142
	epHeirloomSteelwind            = 714302
	qEpicExchange60                = 101127
)

// epicFixture is the rehabilitators' warrior (rehab_test.go) with Dabrish
// beside it and the quests of its epics: group 5's level 49 reward, its
// weapon pick Ascended as on the live data (kept: the Ascended Steelwind),
// and the level 60 epic reward offering a greatsword.
type epicFixture struct {
	*rehabFixture
	dab uint32
}

func newEpicFixture(t *testing.T) *epicFixture {
	t.Helper()
	f := &epicFixture{rehabFixture: newRehabFixture(t)}
	f.spell(spBattlecry, "Battlecry")
	f.spell(spWarcry, "Warcry")
	f.item(itSteelwind, "Steelwind", 5, 14, 1)
	f.item(itFoereaver, "Foereaver", 5, 15, 1)
	f.item(epAscSteelwind, "Ascended Steelwind", 5, 14, 1)
	f.item(epAscFoereaver, "Ascended Foereaver", 5, 15, 1)
	f.item(epImpSteelwind, "Steelwind", 5, 14, 1)
	f.item(epGreatsword, "Ascended Warrior's Epic Greatsword", 5, 15, 1)
	f.item(epHeirloomSteelwind, "Heirloom Steelwind", 5, 14, 1)
	f.exec(`UPDATE itempattern SET maxhp = 80 WHERE patternid IN (?, ?, ?, ?, ?)`, itSteelwind, itFoereaver, epAscSteelwind,
		epAscFoereaver, epImpSteelwind)
	f.exec(`CREATE TABLE IF NOT EXISTS itemascended (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE,
		made TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')`)
	f.exec(`INSERT INTO itemascended (patternid, base) VALUES (?, ?), (?, ?)`, epAscSteelwind, itSteelwind, epAscFoereaver, itFoereaver)
	f.exec(`CREATE TABLE IF NOT EXISTS itemimported (patternid INTEGER PRIMARY KEY, base INTEGER NOT NULL UNIQUE,
		made TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')`)
	f.exec(`INSERT INTO itemimported (patternid, base) VALUES (?, ?)`, epImpSteelwind, itSteelwind)
	f.exec(`INSERT INTO heirlooms (patternid, class, role, epic) VALUES (?, 0, 'main', ?)`, epHeirloomSteelwind, itSteelwind)
	f.quest(qL49, "30,40,49/group-5-evil/L49-warrior", "Warlord Honan",
		choice(fmt.Sprintf("Battlecry=%d:", spBattlecry), fmt.Sprintf("Warcry=%d:", spWarcry)),
		choice(fmt.Sprintf("The Ascended Steelwind=:%d", epAscSteelwind), fmt.Sprintf("The Ascended Foereaver=:%d", epAscFoereaver)))
	f.quest(qEpicExchange60, "60/evil-tank/reward-warrior", "Ediopolus",
		choice(fmt.Sprintf("Warcry=%d:", spWarcry)),
		choice(fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword)))
	f.done(qL49, "1:0,2:0")
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		t.Fatalf("reload: %s", msg)
	}
	f.qe.ReloadCharacter(f.sid)
	f.tw.h.dabrishLevel = func(*session.Session, *entity.Player, int) (int, error) { return 0, nil }
	f.tw.h.dabrishMastery = func(*session.Session, int) (int, entity.Mastery, bool, error) { return 0, entity.Mastery{}, true, nil }
	f.tw.h.dabrishEpic = &dabrishEpics{inv: f.inv, quests: func() *QuestEngine { return f.qe }, now: time.Now}
	f.dab = f.tw.add(2, &entity.NPC{ServerID: 2206, Name: DabrishName, NPCType: 0x80, SpawnX: 23432, SpawnY: 54, SpawnZ: 19602})
	return f
}

// exchange talks to Dabrish and answers "Exchange my epic weapon": what he
// says then.
func (f *epicFixture) exchange() []string {
	f.t.Helper()
	f.tw.talk(f.dab)
	f.last = questMsgs(f.tw.s)
	if _, a := asked(f.last); len(a) != 6 || a[5] != dabrishEpicLabel {
		f.t.Fatalf("his first box: %q", f.last)
	}
	return f.pick(5)
}

// listHeld is how many of a record the session's copy of the client's pack
// list holds.
func (f *epicFixture) listHeld(pattern int) int { return sessionInvList(f.tw.s).held(pattern) }

// remainHP is the condition of the character's row of a record.
func (f *epicFixture) remainHP(pattern int) int {
	var n int
	f.db.QueryRow(`SELECT remainHP FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, pattern).Scan(&n)
	return n
}

// The warrior's path epics both ways: the Steelwind (in the pack, at half its
// condition) shown against the Foereaver and asked Yes or No; on Yes the
// Steelwind is gone from the rows and the client's list (0x003C) and the
// Foereaver given (0x003B) at the same condition, the kept weapon pick moved
// to it; then the Foereaver back for the Steelwind. The control: No changes
// nothing.
func TestDabrishExchangesAPathEpicBothWays(t *testing.T) {
	f := newEpicFixture(t)
	f.own(itSteelwind, 1, -1, 0)
	f.exec(`UPDATE charinventory SET remainHP = 40 WHERE serverid = ? AND patternid = ?`, f.sid, itSteelwind)
	f.lists()

	m := f.exchange()
	if !rehabSays(m, "Dabrish: You hold the Steelwind. For it I give you the Foereaver, the other path's epic, free of charge.") {
		t.Fatalf("what he shows: %q", m)
	}
	if q, a := asked(m); q != "Dabrish: 'Exchange the Steelwind for the Foereaver?'" || strings.Join(a, "|") != "Yes, exchange it.|No." {
		t.Fatalf("his question: %q %q", q, a)
	}
	m = f.pick(1)
	if f.held(itSteelwind) != 1 || f.held(itFoereaver) != 0 || !rehabSays(m, "Your Steelwind stays yours.") {
		t.Fatalf("the control, No: steelwind %d foereaver %d, %q", f.held(itSteelwind), f.held(itFoereaver), m)
	}

	f.exchange()
	m = f.pick(0)
	if f.held(itSteelwind) != 0 || f.held(itFoereaver) != 1 {
		t.Fatalf("after Yes: steelwind %d foereaver %d (%q)", f.held(itSteelwind), f.held(itFoereaver), m)
	}
	if !hasMsg(m, "take:0*1") || !hasMsg(m, "item") || f.listHeld(itSteelwind) != 0 || f.listHeld(itFoereaver) != 1 {
		t.Fatalf("the client: %q, its list steelwind %d foereaver %d", m, f.listHeld(itSteelwind), f.listHeld(itFoereaver))
	}
	if !rehabSays(m, "There: the Foereaver is yours, and the Steelwind mine.") {
		t.Errorf("his line: %q", m)
	}
	if hp := f.remainHP(itFoereaver); hp != 40 {
		t.Errorf("the Foereaver's condition %d, want the Steelwind's 40", hp)
	}
	if p := f.picks(qL49); p != "1:0, 2:1" {
		t.Errorf("the kept picks %q, want the weapon's moved: 1:0, 2:1", p)
	}

	f.exchange()
	m = f.pick(0)
	if f.held(itSteelwind) != 1 || f.held(itFoereaver) != 0 || f.listHeld(itSteelwind) != 1 || f.listHeld(itFoereaver) != 0 {
		t.Fatalf("back: steelwind %d foereaver %d, list %d %d (%q)", f.held(itSteelwind), f.held(itFoereaver),
			f.listHeld(itSteelwind), f.listHeld(itFoereaver), m)
	}
	if p := f.picks(qL49); p != "1:0, 2:0" {
		t.Errorf("the kept picks back %q", p)
	}
}

// An Ascended epic gets the other's Ascended record; an imported copy gets
// the other's imported copy, made when it has none (the transfer's damper);
// with several in the pack he asks which first.
func TestDabrishExchangesKeepTheVersion(t *testing.T) {
	f := newEpicFixture(t)
	f.own(itSteelwind, 1, -1, 0)
	f.own(epAscSteelwind, 1, -1, 0)
	m := f.exchange()
	if q, a := asked(m); q != dabrishEpicAskWhich || strings.Join(a, "|") != "Steelwind|Ascended Steelwind" {
		t.Fatalf("which: %q %q", q, a)
	}
	m = f.pick(1)
	if q, _ := asked(m); q != "Dabrish: 'Exchange the Ascended Steelwind for the Ascended Foereaver?'" {
		t.Fatalf("the Ascended one's question: %q", m)
	}
	f.pick(0)
	if f.held(epAscSteelwind) != 0 || f.held(epAscFoereaver) != 1 || f.held(itSteelwind) != 1 || f.held(itFoereaver) != 0 {
		t.Fatalf("the Ascended exchange: asc steelwind %d asc foereaver %d steelwind %d foereaver %d",
			f.held(epAscSteelwind), f.held(epAscFoereaver), f.held(itSteelwind), f.held(itFoereaver))
	}

	g := newEpicFixture(t)
	g.own(epImpSteelwind, 1, -1, 0)
	m = g.exchange()
	if q, _ := asked(m); q != "Dabrish: 'Exchange the Steelwind for the Foereaver?'" {
		t.Fatalf("the imported one's question: %q", m)
	}
	g.pick(0)
	var copyID int
	g.db.QueryRow(`SELECT patternid FROM itemimported WHERE base = ?`, itFoereaver).Scan(&copyID)
	if copyID == 0 || g.held(copyID) != 1 || g.held(itFoereaver) != 0 || g.held(epImpSteelwind) != 0 || g.listHeld(copyID) != 1 {
		t.Fatalf("the imported exchange: copy %d held %d, foereaver %d, imported steelwind %d", copyID, g.held(copyID),
			g.held(itFoereaver), g.held(epImpSteelwind))
	}
}

// The refusals, each its own line and nothing changed: no epic at all, one of
// no path (the level 60 epic's), an heirloom, one worn, one banked, the other
// path's owned already (banked), an Ascended one whose other has no Ascended
// record; and an exchange agreed after the epic was put on.
func TestDabrishRefusesWhatHeCannotExchange(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *epicFixture)
		want string
	}{
		{"no epic", func(f *epicFixture) {}, dabrishEpicNone},
		{"no counterpart", func(f *epicFixture) { f.own(epGreatsword, 1, -1, 0) },
			"The Ascended Warrior's Epic Greatsword belongs to no path: I have nothing of another path to give for it."},
		{"heirloom", func(f *epicFixture) { f.own(epHeirloomSteelwind, 1, -1, 0) },
			"Your Heirloom Steelwind is an heirloom: the Heirloom Outfitter gives either path's for nothing."},
		{"worn", func(f *epicFixture) { f.own(itSteelwind, 1, 14, 0) }, "Take the Steelwind off first, then ask me again."},
		{"banked", func(f *epicFixture) { f.own(itSteelwind, 1, -1, 1) }, "Bring the Steelwind from your bank first, then ask me again."},
		{"owned already", func(f *epicFixture) { f.own(itSteelwind, 1, -1, 0); f.own(epAscFoereaver, 1, -1, 1) },
			"You own the Foereaver already. I will not give you a second."},
		{"no Ascended counterpart", func(f *epicFixture) {
			// the quest's pick the regular records, the Foereaver with no Ascended record
			f.own(epAscSteelwind, 1, -1, 0)
			f.exec(`DELETE FROM itemascended WHERE patternid = ?`, epAscFoereaver)
			f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(fmt.Sprintf("The Steelwind=:%d", itSteelwind),
				fmt.Sprintf("The Foereaver=:%d", itFoereaver)), qL49)
			f.qe.ReloadQuests()
		}, "There is no Ascended version of the Foereaver for me to give for your Ascended Steelwind."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEpicFixture(t)
			c.set(f)
			var before int
			f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ?`, f.sid).Scan(&before)
			m := f.exchange()
			if !rehabSays(m, "Dabrish: "+c.want) || boxID(m) != 0 {
				t.Fatalf("said %q, want %q and no question", m, c.want)
			}
			var after int
			f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ?`, f.sid).Scan(&after)
			if after != before || f.held(itFoereaver) != 0 || hasMsg(m, "item") {
				t.Errorf("something changed: rows %d -> %d, foereaver %d, %q", before, after, f.held(itFoereaver), m)
			}
		})
	}

	t.Run("put on before the yes", func(t *testing.T) {
		f := newEpicFixture(t)
		f.own(itSteelwind, 1, -1, 0)
		f.exchange()
		f.exec(`UPDATE charinventory SET equiploc = 14 WHERE serverid = ? AND patternid = ?`, f.sid, itSteelwind)
		m := f.pick(0)
		if !rehabSays(m, "Take the Steelwind off first") || f.held(itSteelwind) != 1 || f.held(itFoereaver) != 0 || hasMsg(m, "item") {
			t.Fatalf("the yes after it was worn: %q, steelwind %d foereaver %d", m, f.held(itSteelwind), f.held(itFoereaver))
		}
	})
}

// His sixth answer fits a box, and the first box holds six.
func TestDabrishEpicWordsFit(t *testing.T) {
	if n := len([]rune(dabrishEpicLabel)); n > questAnswerChars {
		t.Errorf("%q is %d characters", dabrishEpicLabel, n)
	}
	if len(dabrishRanges)+1 > maxDialogOptions {
		t.Errorf("%d answers, a box holds %d", len(dabrishRanges)+1, maxDialogOptions)
	}
}
