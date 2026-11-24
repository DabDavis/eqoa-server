package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A token shop paid in a faction's standing (tokenshop.go, a row's token
// "faction:<name>"; Live petition 5575: the Storyteller's merchants): the toy
// rig's shop with its rows' token changed to the Storyteller's faction.

const storyFaction = "The Storyteller"

func newStoryRig(t *testing.T, standing int) *toyRig {
	t.Helper()
	r := newToyRig(t)
	if _, err := r.db.Exec(`UPDATE tokenshop SET token = ?`, factionTokenPrefix+storyFaction); err != nil {
		t.Fatal(err)
	}
	r.gw.SetFactionDefaults([]database.FactionDefault{{Name: "Citizens of Freeport", Standing: 1}, {Name: storyFaction, Standing: 0}})
	r.ih.tokenShop.chars = database.NewSQLiteCharacterStore(r.db)
	r.pa.Factions = map[string]int{storyFaction: standing}
	return r
}

func (r *toyRig) story() int { return r.pa.Factions[storyFaction] }

func (r *toyRig) storyKept() int {
	r.t.Helper()
	var kept int
	if err := r.db.QueryRow(`SELECT standing FROM charfactions WHERE serverid = ? AND faction = ?`, r.pa.CharServerID, storyFaction).Scan(&kept); err != nil {
		r.t.Fatalf("charfactions: %v", err)
	}
	return kept
}

// chooseStory is choose without the ware shown (its price is "N faction").
func (r *toyRig) chooseStory(box uint32, option byte) ([]string, uint32) {
	r.t.Helper()
	r.pick(r.anna, box, option)
	var got []string
	for _, m := range r.msgs(r.anna) {
		if strings.HasPrefix(m, "chat:"+toyShop+": ") {
			continue
		}
		got = append(got, m)
	}
	return got, heroesBoxOf(got)
}

func TestAFactionShopSellsForStanding(t *testing.T) {
	r := newStoryRig(t, 60)
	got, box := r.visit()
	r.want("Anna", got,
		fmt.Sprintf("box:%s: Welcome, Anna! I trade for standing with The Storyteller: you have 60 to spend, and a purchase takes it off.", toyShop),
		fmt.Sprintf("ask#%d:%s: 'You have 60 to spend with The Storyteller. What will you have?'|%s", box, toyShop, toyList))
	got, buy := r.chooseStory(box, 1) // the Barrel of Monkeys, 36
	r.want("Anna", got, fmt.Sprintf("box:%s: The record's own words for Barrel of Monkeys.", toyShop),
		fmt.Sprintf("ask#%d:%s: 'The Barrel of Monkeys, for 36 faction?'|Yes, I will take it|No, not now", buy, toyShop))
	got, _ = r.chooseStory(buy, 0)
	if !hasMsg(got, "chat:24 faction with The Storyteller (-36).") ||
		!hasMsg(got, fmt.Sprintf("box:%s: The Barrel of Monkeys is yours! You have 24 to spend left.", toyShop)) {
		t.Errorf("the purchase said %q", got)
	}
	if r.count(r.ids[1]) != 1 || r.story() != 24 || r.storyKept() != 24 {
		t.Fatalf("after: %d held, standing %d, kept %d", r.count(r.ids[1]), r.story(), r.storyKept())
	}
	var tok string
	if err := r.db.QueryRow(`SELECT token FROM tokenshop_purchases`).Scan(&tok); err != nil || tok != "faction:The Storyteller" {
		t.Errorf("the log's token %q (%v)", tok, err)
	}
	// 24 left: the 36 is refused, nothing taken
	_, box = r.visit()
	got, next := r.chooseStory(box, 1)
	r.want("Anna", got, fmt.Sprintf("box:%s: The Barrel of Monkeys costs 36 faction, and you have 24 to spend: 12 more to earn.", toyShop))
	if next != 0 || r.story() != 24 {
		t.Errorf("short: box %#x, standing %d", next, r.story())
	}
}

// Below the floor (0) nothing is spent: a negative standing has 0 to spend, and
// a standing lost between the question and the Yes is counted again.
func TestAFactionShopNeverGoesBelowTheFloor(t *testing.T) {
	r := newStoryRig(t, -500)
	got, _ := r.visit()
	if !hasMsg(got, fmt.Sprintf("box:%s: Welcome, Anna! I trade for standing with The Storyteller: you have 0 to spend, and a purchase takes it off.", toyShop)) {
		t.Errorf("the greeting: %q", got)
	}
	r.pa.Factions[storyFaction] = 20
	_, box := r.visit()
	_, buy := r.chooseStory(box, 0) // Santa's Elf, 18
	if buy == 0 {
		t.Fatal("no Yes asked")
	}
	r.pa.Factions[storyFaction] = 10
	got, _ = r.chooseStory(buy, 0)
	r.want("Anna", got, fmt.Sprintf("box:%s: Santa's Elf costs 18 faction, and you have 10 to spend: 8 more to earn.", toyShop))
	if r.count(r.ids[0]) != 0 || r.story() != 10 {
		t.Errorf("%d held, standing %d", r.count(r.ids[0]), r.story())
	}
}

// A give that fails gives the standing back; a faction the table lacks closes
// the shop; an item token's shop is as it was (the toy tests).
func TestAFactionShopGivesBackAndClosesWithoutItsFaction(t *testing.T) {
	r := newStoryRig(t, 100)
	r.ih.tokenShop.give = func(*session.Session, database.ItemPatternRecord) (int, error) {
		return 0, errors.New("the database is away")
	}
	_, box := r.visit()
	_, buy := r.chooseStory(box, 0)
	got, _ := r.chooseStory(buy, 0)
	if !hasMsg(got, "chat:82 faction with The Storyteller (-18).") || !hasMsg(got, "chat:100 faction with The Storyteller (18).") ||
		!hasMsg(got, fmt.Sprintf("box:%s: I could not hand it over: you keep your standing. Try again.", toyShop)) {
		t.Errorf("the refund: %q", got)
	}
	if r.story() != 100 || r.storyKept() != 100 {
		t.Errorf("standing %d, kept %d", r.story(), r.storyKept())
	}
	r.gw.SetFactionDefaults([]database.FactionDefault{{Name: "Citizens of Freeport", Standing: 1}})
	got, box = r.visit()
	r.want("Anna", got, fmt.Sprintf("box:%s: My shop is closed today.", toyShop))
	if box != 0 {
		t.Errorf("a box %#x", box)
	}
}

func TestFactionOfToken(t *testing.T) {
	for tok, want := range map[string]string{"faction:The Storyteller": "The Storyteller", " Faction: Testing Heroes ": "Testing Heroes",
		"": "", "Holly Token": "", "faction:": ""} {
		if got := factionOfToken(tok); got != want {
			t.Errorf("%q: %q, want %q", tok, got, want)
		}
	}
}
