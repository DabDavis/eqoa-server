package handlers

import (
	"fmt"
	"strings"
	"testing"
)

// Dabrish's exchange of a level 60 epic (the owner, 2026-11-16: "yes extend
// dabrish for the level 60 epics"; Heirloom petition 2203). The fixture's
// level 60 reward (evil, the warrior's) asks its weapon pick of three, as the
// shadowknight's does: the Greatsword, the Blade and a Chestplate, Ascended as
// on the live data; a second (good) reward's pick shares the Chestplate with
// another chest, so its holder may take any of four.
const (
	ep60Greatsword, ep60Blade = 718023, 718024
	epAscBlade                = 980143
	ep60Chest, epAscChest     = 6512, 980098
	ep60Chest2, epAscChest2   = 6509, 980096
	epHeirloomBlade           = 714990
	qEpic60Good               = 101114
)

func newEpic60Fixture(t *testing.T) *epicFixture {
	t.Helper()
	f := newEpicFixture(t)
	f.item(ep60Greatsword, "Warrior's Epic Greatsword", 5, 15, 1)
	f.item(ep60Blade, "Warrior's Epic Blade", 5, 14, 1)
	f.item(epAscBlade, "Ascended Warrior's Epic Blade", 5, 14, 1)
	f.item(ep60Chest, "Scintillating Chestplate", 5, 5, 1)
	f.item(epAscChest, "Ascended Scintillating Chestplate", 5, 5, 1)
	f.item(ep60Chest2, "Chestplate of the Icy Whirlwind", 5, 5, 1)
	f.item(epAscChest2, "Ascended Chestplate of the Icy Whirlwind", 5, 5, 1)
	f.item(epHeirloomBlade, "Heirloom Blade", 5, 14, 1)
	f.exec(`UPDATE itempattern SET maxhp = 100 WHERE patternid IN (?, ?, ?, ?, ?, ?)`, epGreatsword, ep60Greatsword, epAscBlade, ep60Blade,
		epAscChest, ep60Chest)
	f.exec(`INSERT INTO itemascended (patternid, base) VALUES (?, ?), (?, ?), (?, ?), (?, ?)`, epGreatsword, ep60Greatsword,
		epAscBlade, ep60Blade, epAscChest, ep60Chest, epAscChest2, ep60Chest2)
	f.exec(`INSERT INTO heirlooms (patternid, class, role, epic) VALUES (?, 0, 'main', ?)`, epHeirloomBlade, ep60Blade)
	f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(
		fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword),
		fmt.Sprintf("The Ascended Warrior's Epic Blade=:%d", epAscBlade),
		fmt.Sprintf("The Ascended Scintillating Chestplate=:%d", epAscChest)), qEpicExchange60)
	f.quest(qEpic60Good, "60/good-tank/reward-warrior", "Ediopolus",
		choice(fmt.Sprintf("Warcry=%d:", spWarcry)),
		choice(fmt.Sprintf("The Ascended Chestplate of the Icy Whirlwind=:%d", epAscChest2),
			fmt.Sprintf("The Ascended Scintillating Chestplate=:%d", epAscChest)))
	if msg := f.qe.ReloadQuests(); !strings.HasPrefix(msg, "at once") {
		t.Fatalf("reload: %s", msg)
	}
	return f
}

// The sets as the quests give them: a pick's bases, the path epics in none.
func TestDabrishEpic60Sets(t *testing.T) {
	f := newEpic60Fixture(t)
	pairIDs, _, picks := f.qe.classEpicItems(0)
	asc, imp, err := f.inv.ItemVersions()
	if err != nil {
		t.Fatal(err)
	}
	v := epicVersions{asc: asc, imp: imp}
	var pair []int
	for _, id := range pairIDs {
		b, _ := v.base(id)
		pair = append(pair, b)
	}
	got := fmt.Sprint(epicSets(picks, v, pair))
	if want := fmt.Sprint([][]int{{ep60Chest2, ep60Chest}, {ep60Greatsword, ep60Blade, ep60Chest}}); got != want {
		t.Errorf("sets %s, want %s", got, want)
	}
	if got := fmt.Sprint(epicOthers(epicSets(picks, v, pair), ep60Chest)); got != fmt.Sprint([]int{ep60Chest2, ep60Greatsword, ep60Blade}) {
		t.Errorf("the chestplate's others %s", got)
	}
	if got := theNames([]string{"A", "B", "C"}); got != "the A, the B or the C" {
		t.Errorf("names %q", got)
	}
}

// A level 60 epic of a pick of three: both others shown and asked which, then
// the one picked asked Yes or No and given at the same condition; the level
// 49 quest's kept picks untouched. The control: No changes nothing.
func TestDabrishExchangesALevel60Epic(t *testing.T) {
	f := newEpic60Fixture(t)
	f.own(epGreatsword, 1, -1, 0)
	f.exec(`UPDATE charinventory SET remainHP = 50 WHERE serverid = ? AND patternid = ?`, f.sid, epGreatsword)
	f.lists()
	picks := f.picks(qL49)

	m := f.exchange()
	if !rehabSays(m, "Dabrish: You hold the Ascended Warrior's Epic Greatsword. For it I give you the Ascended Warrior's Epic Blade or the Ascended Scintillating Chestplate, whichever you choose, free of charge.") {
		t.Fatalf("what he shows: %q", m)
	}
	if q, a := asked(m); q != "Dabrish: 'Which shall I give for your Ascended Warrior's Epic Greatsword?'" ||
		strings.Join(a, "|") != "Ascended Warrior's Epic Blade|Ascended Scintillating Chestplate" {
		t.Fatalf("which: %q %q", q, a)
	}
	m = f.pick(0)
	if !rehabSays(m, "Dabrish: You hold the Ascended Warrior's Epic Greatsword. For it I give you the Ascended Warrior's Epic Blade, another of your calling's level 60 epics, free of charge.") {
		t.Fatalf("what he shows of the one picked: %q", m)
	}
	if q, a := asked(m); q != "Dabrish: 'Exchange the Ascended Warrior's Epic Greatsword for the Ascended Warrior's Epic Blade?'" ||
		strings.Join(a, "|") != "Yes, exchange it.|No." {
		t.Fatalf("his question: %q %q", q, a)
	}
	m = f.pick(1)
	if f.held(epGreatsword) != 1 || f.held(epAscBlade) != 0 || !rehabSays(m, "Your Ascended Warrior's Epic Greatsword stays yours.") {
		t.Fatalf("the control, No: %q", m)
	}

	f.exchange()
	f.pick(1) // the chestplate
	m = f.pick(0)
	if f.held(epGreatsword) != 0 || f.held(epAscChest) != 1 || f.listHeld(epGreatsword) != 0 || f.listHeld(epAscChest) != 1 {
		t.Fatalf("after Yes: greatsword %d chestplate %d (%q)", f.held(epGreatsword), f.held(epAscChest), m)
	}
	if !rehabSays(m, "There: the Ascended Scintillating Chestplate is yours, and the Ascended Warrior's Epic Greatsword mine.") {
		t.Errorf("his line: %q", m)
	}
	if p := f.picks(qL49); p != picks {
		t.Errorf("the level 49 picks %q, were %q", p, picks)
	}

	// the chestplate is in both picks: its others are both's, the Greatsword back among them
	m = f.exchange()
	if q, a := asked(m); q != "Dabrish: 'Which shall I give for your Ascended Scintillating Chestplate?'" ||
		strings.Join(a, "|") != "Ascended Chestplate of the Icy Whirlwind|Ascended Warrior's Epic Greatsword|Ascended Warrior's Epic Blade" {
		t.Fatalf("the chestplate's which: %q %q", q, a)
	}
	f.pick(1)
	f.pick(0)
	if f.held(epGreatsword) != 1 || f.held(epAscChest) != 0 {
		t.Fatalf("back: greatsword %d chestplate %d", f.held(epGreatsword), f.held(epAscChest))
	}
	if hp := f.remainHP(epGreatsword); hp != 50 {
		t.Errorf("the greatsword's condition %d, want 50 again", hp)
	}
}

// The version is kept: a regular record gets the regular others; an Ascended
// one only the others with an Ascended record. One other owned (in any
// version, anywhere) leaves the last, asked Yes or No at once.
func TestDabrishLevel60KeepsTheRules(t *testing.T) {
	f := newEpic60Fixture(t)
	f.own(ep60Greatsword, 1, -1, 0)
	if _, a := asked(f.exchange()); strings.Join(a, "|") != "Warrior's Epic Blade|Scintillating Chestplate" {
		t.Fatalf("the regular one's others: %q", a)
	}

	g := newEpic60Fixture(t)
	g.own(epGreatsword, 1, -1, 0)
	g.own(ep60Blade, 1, -1, 1) // banked, regular
	m := g.exchange()
	if q, _ := asked(m); q != "Dabrish: 'Exchange the Ascended Warrior's Epic Greatsword for the Ascended Scintillating Chestplate?'" {
		t.Fatalf("one other owned: %q", m)
	}

	h := newEpic60Fixture(t)
	h.own(epGreatsword, 1, -1, 0)
	h.exec(`DELETE FROM itemascended WHERE patternid = ?`, epAscBlade)
	h.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(
		fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword),
		fmt.Sprintf("The Warrior's Epic Blade=:%d", ep60Blade),
		fmt.Sprintf("The Ascended Scintillating Chestplate=:%d", epAscChest)), qEpicExchange60)
	h.qe.ReloadQuests()
	if q, _ := asked(h.exchange()); q != "Dabrish: 'Exchange the Ascended Warrior's Epic Greatsword for the Ascended Scintillating Chestplate?'" {
		t.Fatalf("no Ascended blade: %q", q)
	}

	// a path epic and a level 60 epic in the pack: which first, each once
	k := newEpic60Fixture(t)
	k.own(itSteelwind, 1, -1, 0)
	k.own(epGreatsword, 1, -1, 0)
	if q, a := asked(k.exchange()); q != dabrishEpicAskWhich || strings.Join(a, "|") != "Steelwind|Ascended Warrior's Epic Greatsword" {
		t.Fatalf("which: %q %q", q, a)
	}
	if _, a := asked(k.pick(1)); strings.Join(a, "|") != "Ascended Warrior's Epic Blade|Ascended Scintillating Chestplate" {
		t.Fatalf("the greatsword's others: %q", a)
	}
	k.pick(0)
	k.pick(0)
	if k.held(epGreatsword) != 0 || k.held(epAscBlade) != 1 || k.held(itSteelwind) != 1 {
		t.Fatalf("after: greatsword %d blade %d steelwind %d", k.held(epGreatsword), k.held(epAscBlade), k.held(itSteelwind))
	}
}

// The refusals of a level 60 epic, each its own line and nothing changed:
// worn (petition 2203's: what it would get named), every other owned, no
// Ascended version of any, an heirloom of a pick's; one in no pick of two is
// still no path's.
func TestDabrishRefusesALevel60Epic(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *epicFixture)
		want string
	}{
		{"worn", func(f *epicFixture) { f.own(epGreatsword, 1, 15, 0) },
			"Take the Ascended Warrior's Epic Greatsword off and carry it in your pack first, then ask me again: for it I can give you the Ascended Warrior's Epic Blade or the Ascended Scintillating Chestplate."},
		{"every other owned", func(f *epicFixture) {
			f.own(epGreatsword, 1, -1, 0)
			f.own(ep60Blade, 1, -1, 1)
			f.own(epAscChest, 1, 5, 0)
		}, "You own the Warrior's Epic Blade already. I will not give you a second."},
		{"no Ascended other", func(f *epicFixture) {
			f.own(epGreatsword, 1, -1, 0)
			f.exec(`DELETE FROM itemascended WHERE patternid IN (?, ?)`, epAscBlade, epAscChest)
			f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(
				fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword),
				fmt.Sprintf("The Warrior's Epic Blade=:%d", ep60Blade),
				fmt.Sprintf("The Scintillating Chestplate=:%d", ep60Chest)), qEpicExchange60)
			f.qe.ReloadQuests()
		}, "There is no Ascended version of the Warrior's Epic Blade for me to give for your Ascended Warrior's Epic Greatsword."},
		{"heirloom", func(f *epicFixture) { f.own(epHeirloomBlade, 1, -1, 0) },
			"Your Heirloom Blade is an heirloom, and I exchange no heirloom: the Heirloom Outfitter gives his for nothing."},
		{"in no pick of two", func(f *epicFixture) {
			f.own(epGreatsword, 1, -1, 0)
			f.exec(`UPDATE queststeps SET choices = ? WHERE questid = ? AND step = 2`, choice(
				fmt.Sprintf("The Ascended Warrior's Epic Greatsword=:%d", epGreatsword)), qEpicExchange60)
			f.qe.ReloadQuests()
		}, "The Ascended Warrior's Epic Greatsword belongs to no path: I have nothing of another path to give for it."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newEpic60Fixture(t)
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

	t.Run("put on before the yes", func(t *testing.T) {
		f := newEpic60Fixture(t)
		f.own(epGreatsword, 1, -1, 0)
		f.exchange()
		f.pick(0)
		f.exec(`UPDATE charinventory SET equiploc = 15 WHERE serverid = ? AND patternid = ?`, f.sid, epGreatsword)
		m := f.pick(0)
		if !rehabSays(m, "Take the Ascended Warrior's Epic Greatsword off") || f.held(epGreatsword) != 1 || f.held(epAscBlade) != 0 {
			t.Fatalf("the yes after it was worn: %q", m)
		}
	})
}

// His level 60 lines fit their boxes with the longest names on the live data.
func TestDabrishEpic60WordsFit(t *testing.T) {
	long := "Ascended Shadowknight's Epic Greataxe" // 37
	if n := len([]rune(fmt.Sprintf(dabrishEpicAskFor, long))); n > questAskChars {
		t.Errorf("the which question is %d characters", n)
	}
	for _, s := range []string{dabrishEpicHeirloom60, dabrishEpicShow60} {
		if n := len([]rune(DabrishName + ": " + fmt.Sprintf(s, long, long))); n > questBoxChars {
			t.Errorf("%q is %d characters", s, n)
		}
	}
}
