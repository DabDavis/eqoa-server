package handlers

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Dabrish's level 60 exchange on a real server's data (EQOA_REAL_DB, a copy
// opened): every class's sets off its level 60 epic reward quests' picks,
// logged as the GM reads them; each set's records are gear of two or more,
// none a path epic; the shadowknight's is petition 2203's three. Then
// petition 2203 itself where its character is in the data (serverid
// 126, the Ascended Epic Greataxe worn): told to take it off, naming what he
// would get; taken off (in the copy), exchanged for the Epic Lance.
func TestRealDabrishLevel60Sets(t *testing.T) {
	db := realRehabDB(t)
	inv := database.NewInventoryStore(db)
	qe, err := NewQuestEngine(db, nil, nil, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	asc, imp, err := inv.ItemVersions()
	if err != nil {
		t.Fatal(err)
	}
	v := epicVersions{asc: asc, imp: imp}
	e := &dabrishEpics{inv: inv, quests: func() *QuestEngine { return qe }, now: time.Now}
	for class := 0; class <= database.MaxClassID; class++ {
		slug := database.TransferClassSlugs[class]
		pairIDs, _, picks := qe.classEpicItems(class)
		var pair []int
		for _, id := range pairIDs {
			if b, _ := v.base(id); !slices.Contains(pair, b) {
				pair = append(pair, b)
			}
		}
		if len(pair) != 2 {
			t.Errorf("%s: path epics %v", slug, pair)
		}
		sets := epicSets(picks, v, pair)
		var lines []string
		for _, set := range sets {
			var names []string
			for _, b := range set {
				pat, ok, err := inv.ItemPatternByID(b)
				if err != nil || !ok {
					t.Errorf("%s: record %d: %v", slug, b, err)
					continue
				}
				if pat.EquipSlot < 1 || pat.MaxStack >= 2 {
					t.Errorf("%s: %s (%d) is not gear", slug, pat.ItemName, b)
				}
				names = append(names, fmt.Sprintf("%s %d", pat.ItemName, b))
			}
			lines = append(lines, "{"+strings.Join(names, ", ")+"}")
		}
		t.Logf("%s: %s", slug, strings.Join(lines, " "))
		if class != 2 && len(sets) == 0 { // the paladin's reward is a sword, no pick
			t.Errorf("%s: no level 60 set", slug)
		}
	}
	_, _, picks := qe.classEpicItems(3)
	if got := fmt.Sprint(epicSets(picks, v, nil)); got != "[[5379 718025 718026]]" {
		t.Errorf("the shadowknight's sets %s, want the Contender, the Lance and the Greataxe", got)
	}

	var worn int
	if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = 126 AND patternid = 980145 AND equiploc >= 0`).
		Scan(&worn); err != nil || worn != 1 {
		t.Skipf("petition 2203's greataxe is not worn in this data (%d, %v)", worn, err)
	}
	p := &entity.Player{Name: "Wanuven", Class: 3, CharServerID: 126}
	swaps, no, err := e.plan(p)
	if err != nil || len(swaps) != 0 || no != "Take the Ascended Shadowknight's Epic Greataxe off and carry it in your pack first, then ask me again: for it I can give you the Ascended Sword of the Contender or the Ascended Shadowknight's Epic Lance." {
		t.Fatalf("worn: %v %q %v", swaps, no, err)
	}
	t.Logf("Dabrish: %s", no)
	if _, err := db.Exec(`UPDATE charinventory SET equiploc = -1 WHERE serverid = 126 AND patternid = 980145`); err != nil {
		t.Fatal(err)
	}
	swaps, no, err = e.plan(p)
	if err != nil || len(swaps) != 2 || swaps[0].to != 980087 || swaps[1].to != 980144 || !swaps[0].sixty {
		t.Fatalf("taken off: %+v %q %v", swaps, no, err)
	}
	done, err := inv.ExchangeCharItem(126, database.ItemExchange{ItemID: swaps[1].itemID, From: swaps[1].from, To: swaps[1].to,
		Source: "test"}, time.Now())
	if err != nil || done.To != 980144 {
		t.Fatalf("exchange: %+v %v", done, err)
	}
	if owns, _ := inv.OwnsAnyOf(126, []int{980145}); owns {
		t.Error("the greataxe is still his")
	}
	swaps, _, _ = e.plan(p)
	if len(swaps) != 2 || swaps[0].fromName != "Ascended Shadowknight's Epic Lance" || swaps[1].to != 980145 {
		t.Errorf("the lance's others: %+v", swaps)
	}
}
