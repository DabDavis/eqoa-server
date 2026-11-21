package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A level 60 pick's option giving two different records, exchanged as one
// set (Heirloom petition 2550 (Deathcore, a bard: "exchanging epic weapons
// with Dabrish only gives one of the lyrical weapons back"): "The Bard's Epic
// Reward" gives the Ascended Blades of Lyrical Harmony and Refrain as one
// option). The fixture's warrior pick is the bard's shape: the two blades as
// one option, the Greatsword and the Chestplate each another.
const (
	epHarmony, epRefrain       = 718130, 718131
	epAscHarmony, epAscRefrain = 980083, 980084
)

const (
	nameAscHarmony = "Ascended Blade of Lyrical Harmony"
	nameAscRefrain = "Ascended Blade of Lyrical Refrain"
	nameAscBlades  = nameAscHarmony + " and the " + nameAscRefrain
	boxAscBlades   = "Ascended Blade of Lyrical Harmony and Refrain" // a box's
)

func newEpicSetFixture(t *testing.T) *epicFixture {
	t.Helper()
	f := newEpic60Fixture(t)
	f.item(epHarmony, "Blade of Lyrical Harmony", 5, 14, 1)
	f.item(epRefrain, "Blade of Lyrical Refrain", 5, 14, 1)
	f.item(epAscHarmony, nameAscHarmony, 5, 14, 1)
	f.item(epAscRefrain, nameAscRefrain, 5, 14, 1)
	f.exec(`UPDATE itempattern SET maxhp = 100 WHERE patternid IN (?, ?)`, epAscHarmony, epAscRefrain)
	f.exec(`INSERT INTO itemascended (patternid, base) VALUES (?, ?), (?, ?)`, epAscHarmony, epHarmony, epAscRefrain, epRefrain)
	f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(
		fmt.Sprintf("The Ascended Blades of Lyrical Harmony and Refrain=:%d, %d", epAscHarmony, epAscRefrain),
		fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword),
		fmt.Sprintf("The Ascended Scintillating Chestplate=:%d", epAscChest)), qEpicExchange60)
	f.exec(`DELETE FROM quests WHERE questid = ?`, qEpic60Good)
	f.exec(`DELETE FROM queststeps WHERE questid = ?`, qEpic60Good)
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		t.Fatalf("reload: %s", msg)
	}
	return f
}

// The set is one entry of the pick: the blades' bases one, under the first.
func TestDabrishEpicSetIsOne(t *testing.T) {
	f := newEpicSetFixture(t)
	if got := fmt.Sprint(f.qe.classEpicGroups(0)); got != fmt.Sprint([][]int{{epAscHarmony, epAscRefrain}}) {
		t.Errorf("groups %s", got)
	}
	if got := bothNames([]string{"A", "B", "C"}); got != "A, the B and the C" {
		t.Errorf("names %q", got)
	}
	if got := shortNames([]string{nameAscHarmony, nameAscRefrain}); got != boxAscBlades {
		t.Errorf("short %q", got)
	}
	if got := shortNames([]string{"Red Axe", "Blue Axe"}); got != "Red Axe and the Blue Axe" {
		t.Errorf("short, nothing in common %q", got)
	}
	f.own(epGreatsword, 1, -1, 0)
	swaps, no, err := f.tw.h.dabrishEpic.plan(f.tw.p)
	if err != nil || no != "" || len(swaps) != 2 || swaps[0].toBase != epHarmony || !swaps[0].toSet ||
		fmt.Sprint(swaps[0].also) != fmt.Sprintf("[{%d 0}]", epAscRefrain) || swaps[1].to != epAscChest {
		t.Errorf("the greatsword's: %+v %q %v", swaps, no, err)
	}
}

// Both blades carried: offered once (no question which epic), the others of
// the set asked which; the control, No, changes nothing; Yes takes both blade
// rows for one Greatsword, from the rows and the client's list.
func TestDabrishExchangesASetAsOne(t *testing.T) {
	f := newEpicSetFixture(t)
	f.own(epAscHarmony, 1, -1, 0)
	f.own(epAscRefrain, 1, -1, 0)
	f.exec(`UPDATE charinventory SET remainHP = 50 WHERE serverid = ? AND patternid = ?`, f.sid, epAscHarmony)
	f.lists()

	m := f.exchange()
	if !rehabSays(m, "Dabrish: You hold the "+nameAscBlades+".") ||
		!rehabSays(m, "For them I give you the Ascended Warrior's Epic Greatsword or the Ascended Scintillating Chestplate, whichever you choose, free of charge.") {
		t.Fatalf("what he shows: %q", m)
	}
	if q, a := asked(m); q != "Dabrish: 'Which shall I give for your "+boxAscBlades+"?'" ||
		strings.Join(a, "|") != "Ascended Warrior's Epic Greatsword|Ascended Scintillating Chestplate" {
		t.Fatalf("which: %q %q", q, a)
	}
	m = f.pick(0)
	if !rehabSays(m, "Dabrish: You hold the "+nameAscBlades+". For them I give you the Ascended Warrior's Epic Greatsword, free of charge.") {
		t.Fatalf("what he shows of the one picked: %q", m)
	}
	m = f.pick(1)
	if f.held(epAscHarmony) != 1 || f.held(epAscRefrain) != 1 || f.held(epGreatsword) != 0 {
		t.Fatalf("the control, No: %q", m)
	}

	f.exchange()
	f.pick(0)
	m = f.pick(0)
	if f.held(epAscHarmony) != 0 || f.held(epAscRefrain) != 0 || f.held(epGreatsword) != 1 ||
		f.listHeld(epAscHarmony) != 0 || f.listHeld(epAscRefrain) != 0 || f.listHeld(epGreatsword) != 1 {
		t.Fatalf("after Yes: harmony %d refrain %d greatsword %d (%q)", f.held(epAscHarmony), f.held(epAscRefrain),
			f.held(epGreatsword), m)
	}
	if !rehabSays(m, "There: the Ascended Warrior's Epic Greatsword is yours, and the "+nameAscBlades+" mine.") {
		t.Errorf("his line: %q", m)
	}
	if hp := f.remainHP(epGreatsword); hp != 50 {
		t.Errorf("the greatsword's condition %d, want 50 (the first blade's)", hp)
	}
}

// Holding the Greatsword: the set is one other, and the exchange gives both
// blades, each at the Greatsword's share of its durability.
func TestDabrishGivesASetAsOne(t *testing.T) {
	f := newEpicSetFixture(t)
	f.own(epGreatsword, 1, -1, 0)
	f.exec(`UPDATE charinventory SET remainHP = 50 WHERE serverid = ? AND patternid = ?`, f.sid, epGreatsword)
	f.lists()

	m := f.exchange()
	if _, a := asked(m); strings.Join(a, "|") != boxAscBlades+"|Ascended Scintillating Chestplate" {
		t.Fatalf("the greatsword's others: %q", m)
	}
	m = f.pick(0)
	if !rehabSays(m, "Dabrish: You hold the Ascended Warrior's Epic Greatsword. For it I give you the "+nameAscBlades+", free of charge.") {
		t.Fatalf("what he shows: %q", m)
	}
	if q, _ := asked(m); q != "Dabrish: 'Exchange the Ascended Warrior's Epic Greatsword for the "+boxAscBlades+"?'" {
		t.Fatalf("his question: %q", q)
	}
	m = f.pick(0)
	if f.held(epGreatsword) != 0 || f.held(epAscHarmony) != 1 || f.held(epAscRefrain) != 1 ||
		f.listHeld(epGreatsword) != 0 || f.listHeld(epAscHarmony) != 1 || f.listHeld(epAscRefrain) != 1 {
		t.Fatalf("after Yes: greatsword %d harmony %d (list %d) refrain %d (list %d) (%q)", f.held(epGreatsword),
			f.held(epAscHarmony), f.listHeld(epAscHarmony), f.held(epAscRefrain), f.listHeld(epAscRefrain), m)
	}
	if !rehabSays(m, "There: the "+nameAscBlades+" are yours, and the Ascended Warrior's Epic Greatsword mine.") {
		t.Errorf("his line: %q", m)
	}
	if h, r := f.remainHP(epAscHarmony), f.remainHP(epAscRefrain); h != 50 || r != 50 {
		t.Errorf("the blades' condition %d and %d, want 50", h, r)
	}

	// and back: the pair of blades exchanged as one again
	f.exchange()
	f.pick(0)
	f.pick(0)
	if f.held(epGreatsword) != 1 || f.held(epAscHarmony) != 0 || f.held(epAscRefrain) != 0 {
		t.Fatalf("back: greatsword %d harmony %d refrain %d", f.held(epGreatsword), f.held(epAscHarmony), f.held(epAscRefrain))
	}
}

// One blade alone is refused, as is one of the two worn or banked, each with
// its line and nothing changed; the control, both carried, is offered (above).
// One blade owned with the Greatsword: the set is not offered for it (no
// second blade), the Chestplate still is. A blade banked between the offer
// and the Yes: nothing moves.
func TestDabrishRefusesASetApart(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *epicFixture)
		want string
	}{
		{"one blade", func(f *epicFixture) { f.own(epAscRefrain, 1, -1, 0) },
			"The " + nameAscBlades + " are one pick, taken only together: carry each in your pack, then ask me again."},
		{"one blade worn", func(f *epicFixture) {
			f.own(epAscHarmony, 1, 14, 0)
			f.own(epAscRefrain, 1, -1, 0)
		}, "Take the " + nameAscBlades + " off and carry them in your pack first, then ask me again: for them I can give you the Ascended Warrior's Epic Greatsword"}, // the rest in a second box
		{"one blade banked", func(f *epicFixture) {
			f.own(epAscHarmony, 1, -1, 0)
			f.own(epAscRefrain, 1, -1, 1)
		}, "Bring the " + nameAscRefrain + " from your bank first, then ask me again."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEpicSetFixture(t)
			c.set(f)
			var before int
			f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ?`, f.sid).Scan(&before)
			m := f.exchange()
			if !rehabSays(m, "Dabrish: "+c.want) || boxID(m) != 0 {
				t.Fatalf("said %q, want %q and no question", m, c.want)
			}
			var after int
			f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ?`, f.sid).Scan(&after)
			if after != before || hasMsg(m, "item") {
				t.Errorf("something changed: rows %d -> %d, %q", before, after, m)
			}
		})
	}

	t.Run("one blade with the greatsword", func(t *testing.T) {
		f := newEpicSetFixture(t)
		f.own(epGreatsword, 1, -1, 0)
		f.own(epAscHarmony, 1, -1, 0)
		m := f.exchange()
		if q, _ := asked(m); q != "Dabrish: 'Exchange the Ascended Warrior's Epic Greatsword for the Ascended Scintillating Chestplate?'" {
			t.Fatalf("his question: %q", m)
		}
		f.pick(0)
		if f.held(epGreatsword) != 0 || f.held(epAscChest) != 1 || f.held(epAscHarmony) != 1 || f.held(epAscRefrain) != 0 {
			t.Fatalf("after: greatsword %d chest %d harmony %d refrain %d", f.held(epGreatsword), f.held(epAscChest),
				f.held(epAscHarmony), f.held(epAscRefrain))
		}
	})

	t.Run("banked before the yes", func(t *testing.T) {
		f := newEpicSetFixture(t)
		f.own(epAscHarmony, 1, -1, 0)
		f.own(epAscRefrain, 1, -1, 0)
		f.exchange()
		f.pick(0)
		f.exec(`UPDATE charinventory SET location = 1 WHERE serverid = ? AND patternid = ?`, f.sid, epAscRefrain)
		m := f.pick(0)
		if !rehabSays(m, "Bring the "+nameAscRefrain+" from your bank first") || f.held(epAscHarmony) != 1 ||
			f.held(epAscRefrain) != 1 || f.held(epGreatsword) != 0 {
			t.Fatalf("the yes after a blade was banked: %q", m)
		}
	})
}

// The exchange's transaction checks a set's other rows as its first: a row
// not of the record named, or named twice, is gone and nothing changes; the
// control, both right, gives both records.
func TestExchangeCharItemTakesASet(t *testing.T) {
	f := newEpicSetFixture(t)
	f.own(epGreatsword, 1, -1, 0)
	f.own(epAscHarmony, 1, -1, 0)
	f.own(epAscRefrain, 1, -1, 0)
	row := func(pattern int) int {
		var id int
		f.db.QueryRow(`SELECT itemid FROM charinventory WHERE serverid = ? AND patternid = ?`, f.sid, pattern).Scan(&id)
		return id
	}
	h, r := row(epAscHarmony), row(epAscRefrain)
	for _, with := range [][]database.ExchangeRow{{{ItemID: r, From: epAscHarmony}}, {{ItemID: h, From: epAscHarmony}}} {
		_, err := f.inv.ExchangeCharItem(f.sid, database.ItemExchange{ItemID: h, From: epAscHarmony, To: epAscChest,
			With: with, Source: "test"}, time.Now())
		if !errors.Is(err, database.ErrExchangeGone) || f.held(epAscHarmony) != 1 || f.held(epAscRefrain) != 1 || f.held(epAscChest) != 0 {
			t.Fatalf("with %v: %v", with, err)
		}
	}
	done, err := f.inv.ExchangeCharItem(f.sid, database.ItemExchange{ItemID: row(epGreatsword), From: epGreatsword,
		To: epAscHarmony, Also: []database.ExchangeGive{{To: epAscChest}}, Source: "test"}, time.Now())
	if err != nil || done.To != epAscHarmony || len(done.Also) != 1 || done.Also[0].To != epAscChest ||
		f.held(epGreatsword) != 0 || f.held(epAscHarmony) != 2 || f.held(epAscChest) != 1 {
		t.Fatalf("the control: %+v %v", done, err)
	}
}

// His set lines fit their boxes with the bard's names.
func TestDabrishEpicSetWordsFit(t *testing.T) {
	long := "Ascended Shadowknight's Epic Greataxe"
	for _, s := range []string{dabrishEpicShowSetFrom, dabrishEpicShowSetTo} {
		if n := len([]rune(DabrishName + ": " + fmt.Sprintf(s, nameAscBlades, long))); n > questBoxChars {
			t.Errorf("%q is %d characters", s, n)
		}
	}
	if n := len([]rune(DabrishName + ": " + fmt.Sprintf(dabrishEpicApart, nameAscBlades))); n > questBoxChars {
		t.Errorf("the apart line is %d characters", n)
	}
}
