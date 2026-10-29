package database

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A database made before the faction requirements, the rewards by class and
// the unlocks (its quest tables without factionreq and byclass, no
// questunlocks) opens: the columns are added, its quests read as they were,
// and the new ones are read once written. The control: the same rows without
// the new columns written read with none.
func TestAnOldQuestDatabaseGainsTheNewColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE quests (questid INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 0,
			giver TEXT NOT NULL DEFAULT '', minlevel INTEGER NOT NULL DEFAULT 1, maxlevel INTEGER NOT NULL DEFAULT 60,
			classes TEXT NOT NULL DEFAULT '', races TEXT NOT NULL DEFAULT '', requires TEXT NOT NULL DEFAULT '',
			repeatable INTEGER NOT NULL DEFAULT 0, offer TEXT NOT NULL DEFAULT '', accept TEXT NOT NULL DEFAULT '',
			decline TEXT NOT NULL DEFAULT '', declined TEXT NOT NULL DEFAULT '', started TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '', events TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'talk',
			target TEXT NOT NULL DEFAULT '', npcid INTEGER NOT NULL DEFAULT 0, items TEXT NOT NULL DEFAULT '',
			tunar INTEGER NOT NULL DEFAULT 0, count INTEGER NOT NULL DEFAULT 1, world INTEGER NOT NULL DEFAULT 0,
			x REAL NOT NULL DEFAULT 0, y REAL NOT NULL DEFAULT 0, z REAL NOT NULL DEFAULT 0, radius REAL NOT NULL DEFAULT 15,
			journal TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '', says TEXT NOT NULL DEFAULT '',
			waiting TEXT NOT NULL DEFAULT '', notyet TEXT NOT NULL DEFAULT '', reminder TEXT NOT NULL DEFAULT '',
			xp INTEGER NOT NULL DEFAULT 0, rewardtunar INTEGER NOT NULL DEFAULT 0, rewarditems TEXT NOT NULL DEFAULT '',
			factions TEXT NOT NULL DEFAULT '', spells TEXT NOT NULL DEFAULT '', chance REAL NOT NULL DEFAULT 0,
			events TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step))`,
		`INSERT INTO quests (questid, name, enabled, giver, repeatable) VALUES (5, 'Old one', 1, 'Malsis', 1)`,
		`INSERT INTO queststeps (questid, step, kind, target, xp, rewarditems) VALUES (5, 1, 'talk', 'Malsis', 100, '7*2')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, c := range [][2]string{{"quests", "factionreq"}, {"queststeps", "byclass"}, {"questunlocks", "dest"}} {
		if has, err := hasColumn(db.DB, c[0], c[1]); err != nil || !has {
			t.Errorf("%s.%s missing after the open (%v)", c[0], c[1], err)
		}
	}
	store := NewQuestStore(db)
	qs, err := store.LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || !qs[0].Repeatable || len(qs[0].FactionReqs) != 0 || len(qs[0].Unlocks) != 0 ||
		len(qs[0].Steps) != 1 || qs[0].Steps[0].ByClass != nil {
		t.Fatalf("the old quest as read: %+v", qs)
	}

	for _, q := range []string{
		`UPDATE quests SET factionreq = 'Freeport Militia:100, Lockjaw:-500..0' WHERE questid = 5`,
		`UPDATE queststeps SET byclass = '{"10": {"xp": 50, "items": "9*3", "spells": "12, 13"}, "0": {"tunar": 20}}' WHERE questid = 5`,
		`INSERT INTO questunlocks VALUES (5, 'Waysender Abodai', 'Neriak')`,
		`INSERT INTO questunlocks VALUES (99, 'Waysender Abodai', 'Neriak')`, // a quest that is not here
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	qs, err = store.LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	q := qs[0]
	want := []FactionReq{{Faction: "Freeport Militia", Min: 100}, {Faction: "Lockjaw", Min: -500, Max: 0, HasMax: true}}
	if !reflect.DeepEqual(q.FactionReqs, want) {
		t.Errorf("faction requirements %+v, want %+v", q.FactionReqs, want)
	}
	if len(q.Unlocks) != 1 || q.Unlocks[0] != (QuestUnlock{Quest: 5, NPC: "Waysender Abodai", Dest: "Neriak"}) {
		t.Errorf("unlocks %+v", q.Unlocks)
	}
	mag := q.Steps[0].RewardFor(10)
	if mag.XP != 150 || !reflect.DeepEqual(mag.Items, []ItemCount{{7, 2}, {9, 3}}) || !reflect.DeepEqual(mag.Spells, []int{12, 13}) {
		t.Errorf("a magician's rewards %+v", mag)
	}
	if war := q.Steps[0].RewardFor(0); war.XP != 100 || war.Tunar != 20 || len(war.Spells) != 0 {
		t.Errorf("a warrior's rewards %+v", war)
	}
	if mnk := q.Steps[0].RewardFor(4); mnk.XP != 100 || mnk.Tunar != 0 || !reflect.DeepEqual(mnk.Items, []ItemCount{{7, 2}}) {
		t.Errorf("the control, a monk (no class rewards): %+v", mnk)
	}
}

// A faction requirement reads as written and back; a range is both ends,
// inclusive; what cannot be read is a problem and asks nothing.
func TestFactionRequirements(t *testing.T) {
	rs := ParseFactionReqs(" Freeport Militia : +100 , Lockjaw:-500..-100,  ")
	if got := FormatFactionReqs(rs); got != "Freeport Militia:100, Lockjaw:-500..-100" {
		t.Errorf("written back: %q", got)
	}
	r := rs[1]
	for standing, met := range map[int]bool{-501: false, -500: true, -100: true, -99: false} {
		if r.Met(standing) != met {
			t.Errorf("Lockjaw at %d met %v", standing, !met)
		}
	}
	if !rs[0].Met(2000) || rs[0].Met(99) {
		t.Error("no highest: any standing from 100 up")
	}
	bad := "Freeport Militia, Lockjaw:abc, Coldain:10..5, Good:5"
	if got := ParseFactionReqs(bad); len(got) != 1 || got[0].Faction != "Good" {
		t.Errorf("only the readable one is kept: %+v", got)
	}
	if ps := FactionReqProblems(bad); len(ps) != 3 {
		t.Errorf("three problems, got %q", ps)
	}
	if ps := FactionReqProblems("A:1, B:-5..5"); len(ps) != 0 {
		t.Errorf("the control has problems: %q", ps)
	}
}

// Rewards by class read and write as the byclass column keeps them; a key
// that is no class, items or spells written another way, and broken JSON
// are problems.
func TestRewardsByClass(t *testing.T) {
	m, err := ParseClassRewards(`{"13": {"xp": 10, "tunar": 5, "items": "100*2, 101", "spells": "7"}, "15": {"xp": 1}, "x": {}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || !reflect.DeepEqual(m[13], ClassReward{XP: 10, Tunar: 5, Items: []ItemCount{{100, 2}, {101, 1}}, Spells: []int{7}}) {
		t.Fatalf("read: %+v", m)
	}
	if got := FormatClassRewards(m); got != `{"13":{"xp":10,"tunar":5,"items":"100*2, 101","spells":"7"}}` {
		t.Errorf("written: %s", got)
	}
	ps := ClassRewardProblems(`{"13": {"items": "a sword"}, "15": {}, "2": {"xp": -1, "spells": "7, fire"}}`)
	if len(ps) != 4 || !strings.Contains(strings.Join(ps, "\n"), `class "15": no such class`) {
		t.Errorf("problems: %q", ps)
	}
	if ps := ClassRewardProblems(`[1, 2]`); len(ps) != 1 {
		t.Errorf("a list is no object: %q", ps)
	}
	if ps := ClassRewardProblems(`{"0": {"xp": 5, "items": "3", "spells": ""}}`); len(ps) != 0 {
		t.Errorf("the control has problems: %q", ps)
	}
	if m, err := ParseClassRewards(""); m != nil || err != nil {
		t.Errorf("empty: %v %v", m, err)
	}
}
