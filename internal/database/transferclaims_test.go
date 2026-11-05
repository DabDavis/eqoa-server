package database

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// catalogQuests adds to db a ranger's quests of every kind the catalog
// offers, the controls beside them: another city's level 20 path (the same
// choice), the 49 path's two choices, the epic and its chain, a were line
// with its archetypes' 40s, the vampire's, two places' flags, a quest out of
// the game and a warrior's path.
func catalogQuests(t *testing.T, db *SQLiteDB) {
	t.Helper()
	src := func(key string) string { return "EQOA-Data " + key + " (tools/import_quests.py)" }
	quests := []struct {
		id               int
		name, key        string
		classes, require string
		enabled          int
	}{
		{9, "Wolf Pelts", "1-20/surefall-glade/ranger/L19", "1", "", 1},
		{10, "Crethley Manor", "1-20/surefall-glade/ranger/L20", "1", "9", 1},
		{20, "Yiprew's Research", "1-20/tethelin/ranger/L20", "1", "", 1},
		{29, "The Group's Errand", "30,40,49/group-3-good/p1", "", "", 1},
		{30, "The Ranger's Reward, Level 49", "30,40,49/group-3-good/L49-ranger", "1", "29", 1},
		{39, "The Vampires of Greyvax", "60/good-melee/ep3-2", "", "", 1},
		{40, "The Ranger's Epic", "60/good-melee/reward-ranger", "1", "39", 1},
		{50, "Fryda's Kin", "Weres/wolfwere/p1", "", "", 1},
		{51, "Wolfwere Tails", "Weres/wolfwere/p2", "", "50", 1},
		{52, "Duskrender's Trial", "Weres/wolfwere/L40-tank", "0, 2, 3", "51", 1},
		{53, "Duskrender's Trial", "Weres/wolfwere/L40-melee", "1, 4, 5, 6", "51", 1},
		{60, "Razeal Has Won", "Weres/vampire/p1", "", "", 1},
		{61, "Creed's Lessons", "Weres/vampire/L40", "", "60", 1},
		{70, "The Manticores' Riddles", "Portal/plane-of-sky/portal-1", "", "", 1},
		{71, "The Death Knights' Hearts", "Portal/plane-of-disease/flag-1", "", "", 1},
		{80, "Not in the game", "Portal/plane-of-sky/portal-2", "", "", 0},
		{90, "A Warrior's Path", "1-20/freeport/warrior/L20", "0", "", 1},
	}
	for _, q := range quests {
		if _, err := db.Exec(`INSERT INTO quests (questid, name, enabled, classes, requires, source) VALUES (?, ?, ?, ?, ?, ?)`,
			q.id, q.name, q.enabled, q.classes, q.require, src(q.key)); err != nil {
			t.Fatal(err)
		}
	}
	two := `{"ask": "Which do you take?", "options": [{"answer": "Sparrow Eye", "items": "707006*1"}, {"answer": "Strikeblade", "items": "707007*1"}]}`
	steps := []struct {
		quest, step int
		choices     string
	}{
		{9, 1, ""}, {10, 12, two}, {20, 12, two}, {29, 1, ""},
		{30, 1, `{"ask": "Which ability do you take?", "options": [{"answer": "Falcon Eye", "spells": "700599"}, {"answer": "Stormblade", "spells": "700651"}]}`},
		{30, 2, `{"ask": "Which weapon do you take?", "options": [{"answer": "Not yet", "stay": true}, {"answer": "The Pure Emerald Longsword", "items": "700913*1"}]}`},
		{39, 1, ""},
		{40, 1, `{"ask": "Which ability do you take?", "options": [{"answer": "Natureblade", "spells": "700623"}]}`},
		{50, 1, ""}, {51, 1, ""}, {52, 1, ""}, {53, 1, ""}, {60, 1, ""}, {61, 1, ""}, {70, 1, ""}, {71, 1, ""}, {80, 1, ""},
		{90, 5, two},
	}
	for _, st := range steps {
		if _, err := db.Exec(`INSERT INTO queststeps (questid, step, kind, target, choices) VALUES (?, ?, 'talk', 'Someone', ?)`,
			st.quest, st.step, st.choices); err != nil {
			t.Fatal(err)
		}
	}
}

// A ranger's catalog: the level 20 path once (Tethelin's as its Also, the
// manor's chain its level 19), the 49 path's two steps (an option that stays
// left out), the epic with its chain, the wolfwere line in its order with the
// melee 40 alone, the vampire's, the flags by place; the controls: the quest
// out of the game, the warrior's path and the tank 40 are not offered.
func TestTransferCatalog(t *testing.T) {
	db := transferDB(t)
	catalogQuests(t, db)
	cat, err := db.TransferCatalog(1)
	if err != nil {
		t.Fatal(err)
	}
	type ch struct {
		kind         string
		level, quest int
		step         int
		opts         []CatalogOption
		also         []int
		chains       map[int][]int
	}
	var got []ch
	for _, c := range cat.Choices {
		got = append(got, ch{c.Kind, c.Level, c.Quest, c.Step, c.Options, c.Also, c.Chains})
	}
	want := []ch{
		{"path", 20, 10, 12, []CatalogOption{{0, "Sparrow Eye"}, {1, "Strikeblade"}}, []int{20}, map[int][]int{10: {9}}},
		{"path", 49, 30, 1, []CatalogOption{{0, "Falcon Eye"}, {1, "Stormblade"}}, nil, map[int][]int{30: {29}}},
		{"path", 49, 30, 2, []CatalogOption{{1, "The Pure Emerald Longsword"}}, nil, map[int][]int{30: {29}}},
		{"epic", 60, 40, 1, []CatalogOption{{0, "Natureblade"}}, nil, map[int][]int{40: {39}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("choices\n got %+v\nwant %+v", got, want)
	}
	lines := map[string][]int{}
	for _, l := range cat.Lines {
		for _, q := range l.Quests {
			lines[l.Line] = append(lines[l.Line], q.Quest)
		}
	}
	if !reflect.DeepEqual(lines, map[string][]int{"wolfwere": {50, 51, 53}, "vampire": {60, 61}}) || cat.Lines[0].Label != "Wolfwere" {
		t.Fatalf("lines %v %+v", lines, cat.Lines)
	}
	if len(cat.Flags) != 2 || cat.Flags[0].Label != "Plane of Disease" || cat.Flags[1].Label != "Plane of Sky" ||
		len(cat.Flags[1].Quests) != 1 || cat.Flags[1].Quests[0].Quest != 70 {
		t.Fatalf("flags %+v", cat.Flags)
	}
	// the warrior gets its own path and the tank 40
	war, err := db.TransferCatalog(0)
	if err != nil || len(war.Choices) != 1 || war.Choices[0].Quest != 90 || war.Lines[0].Quests[2].Quest != 52 {
		t.Fatalf("a warrior's: %+v %v", war, err)
	}
	if _, err := db.TransferCatalog(15); err == nil {
		t.Fatal("a class past the last")
	}
}

// Claims: tidied (each quest and each step's pick once, sorted, the words as
// kept), refused out of shape; filed, a quest this server lacks is left out
// and the rest kept as filed.
func TestTransferClaims(t *testing.T) {
	c, err := ParseTransferClaims(`{"picks": [{"quest": 30, "step": 2, "option": 0}, {"quest": 30, "step": 1, "option": 1},
		{"quest": 30, "step": 1, "option": 0}], "quests": [61, 60, 61], "line": "vampire", "other": "  Lost Ring\r\nThe Key  "}`)
	if err != nil {
		t.Fatal(err)
	}
	want := TransferClaims{Picks: []ClaimPick{{30, 1, 1}, {30, 2, 0}}, Quests: []int{60, 61}, Line: "vampire", Other: "Lost Ring\nThe Key"}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v", c)
	}
	for _, bad := range []string{`{"line": "werecat"}`, `{"quests": [0]}`, `{"picks": [{"quest": 1, "step": 0, "option": 0}]}`,
		`{"picks": [{"quest": 1, "step": 1, "option": 6}]}`, `not json`, `{"other": "` + strings.Repeat("x", TransferMaxOther+1) + `"}`} {
		if _, err := ParseTransferClaims(bad); err == nil {
			t.Errorf("%.40s: taken", bad)
		}
	}
	if c, err := ParseTransferClaims(""); err != nil || !c.Empty() || FormatTransferClaims(c) != "" {
		t.Fatalf("none: %+v %v", c, err)
	}
	db := transferDB(t)
	catalogQuests(t, db)
	tr, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60,
		Claims: `{"picks": [{"quest": 10, "step": 12, "option": 1}, {"quest": 999, "step": 1, "option": 0}], "quests": [70, 998], "line": "wolfwere"}`},
		time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if tr.Claims != `{"picks":[{"quest":10,"step":12,"option":1}],"quests":[70],"line":"wolfwere"}` {
		t.Fatalf("filed %s", tr.Claims)
	}
	if _, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60, Claims: `{"line": "x"}`}, time.Now()); err == nil {
		t.Fatal("a claim out of shape filed")
	}
}

// Ready to tell: an approved transfer is the account's to be told of until
// marked (and an arrival told marks it too); waiting ones and another
// account's are not listed.
func TestTransfersReadyToTell(t *testing.T) {
	db := transferDB(t)
	now := time.Now()
	tr, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60}, now)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := db.TransfersReadyToTell(3); len(l) != 0 {
		t.Fatal("a waiting one listed")
	}
	if _, _, err := db.ApproveTransfer(tr.ID, `{"character": "Hero"}`, "Welcome.", GMEdit{Username: "gm"}, now); err != nil {
		t.Fatal(err)
	}
	if l, _ := db.TransfersReadyToTell(4); len(l) != 0 {
		t.Fatal("listed for another account")
	}
	if l, _ := db.TransfersReadyToTell(3); len(l) != 1 || l[0].ID != tr.ID || l[0].ToldReady {
		t.Fatalf("ready %+v", l)
	}
	if err := db.MarkTransferToldReady(tr.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := db.TransfersReadyToTell(3); len(l) != 0 {
		t.Fatal("told twice")
	}
}

// The names a plan approximates, in its order; none from a plan without.
func TestApproximatedItems(t *testing.T) {
	plan := `{"character": "Hero", "new_records": [{"ref": "new1", "like": 5408, "set": {"itemname": "No-Dachi of Toxicity"}},
		{"ref": "new2", "like": 1, "set": {"itemdesc": "no name"}}, {"ref": "new3", "like": 6089, "set": {"itemname": " Frosted Cape "}}]}`
	if got := ApproximatedItems(plan); !reflect.DeepEqual(got, []string{"No-Dachi of Toxicity", "Frosted Cape"}) {
		t.Fatalf("got %v", got)
	}
	if got := ApproximatedItems(`{"character": "Hero"}`); got != nil {
		t.Fatalf("none: %v", got)
	}
}
