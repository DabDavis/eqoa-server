package database

import (
	"errors"
	"testing"
	"time"
)

func marksDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/marks.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// tenMinutes is a quest of 10 minutes' estimate: two talks (8) and four kills (2).
func tenMinutes(minLevel int) *Quest {
	return &Quest{ID: 1, MinLevel: minLevel, Steps: []QuestStep{{Kind: StepTalk}, {Kind: StepKill, Count: 4}, {Kind: StepTalk}}}
}

// A quest's marks are its length's, the same at any level (a level 10's 10-minute quest and a level
// 40's of the same kind pay alike), times its kind's factor; a level 60 epic never pays; at least 8;
// the marks column wins. The control: a longer quest pays more.
func TestQuestMarksAreTheQuestsLength(t *testing.T) {
	if m := QuestMinutes(tenMinutes(10)); m != 10 {
		t.Fatalf("minutes %v, want 10", m)
	}
	low, high := QuestMarksFor(tenMinutes(10), "1-20/qeynos/warrior/L10"), QuestMarksFor(tenMinutes(40), "1-20/qeynos/warrior/L40")
	if low != 17 || low != high {
		t.Fatalf("level 10: %d, level 40: %d, want 17 each (10 x 100/60)", low, high)
	}
	if got := QuestMarksFor(tenMinutes(40), "30,40,49/x"); got != 42 {
		t.Errorf("a 30,40,49 quest: %d, want 42 (x2.5)", got)
	}
	if got := QuestMarksFor(tenMinutes(20), "ZP/x"); got != 33 {
		t.Errorf("a ZP quest: %d, want 33 (x2)", got)
	}
	if got := QuestMarksFor(tenMinutes(55), "53-59/x"); got != 25 {
		t.Errorf("a 53-59 quest: %d, want 25 (x1.5)", got)
	}
	if got := QuestMarksFor(tenMinutes(60), "60/epic"); got != 0 {
		t.Errorf("a level 60 epic: %d, want 0", got)
	}
	if got := QuestMarksFor(&Quest{Steps: []QuestStep{{Kind: StepKill}}}, ""); got != QuestMarksLeast {
		t.Errorf("a one-kill quest: %d, want the least, %d (its 5 minutes: %d)", got, QuestMarksLeast, 8)
	}
	// a loot step's chance: 4 of an item at 25%: 8 minutes
	loot := &Quest{Steps: []QuestStep{{Kind: StepLoot, Count: 4, Chance: 0.25}}}
	if m := QuestMinutes(loot); m != 8 {
		t.Errorf("loot at 25%%: %v minutes, want 8", m)
	}
	set := 300
	q := tenMinutes(10)
	q.Marks = &set
	if got := QuestMarksFor(q, ""); got != 300 {
		t.Errorf("the marks column: %d", got)
	}
	long := &Quest{Steps: append(tenMinutes(10).Steps, QuestStep{Kind: StepGive})}
	if QuestMarksFor(long, "") <= low {
		t.Error("the control: a longer quest pays no more")
	}
	if QuestMarksLimit(3) != 10 {
		t.Errorf("3 minutes since taken: at most %d, want 10", QuestMarksLimit(3))
	}
}

// Kill marks stop at 38 an hour and start again once the hour is over; a bonus's fraction is carried
// (1.3 four times: 5, with 0.2 left); a spending past the marks held is refused.
func TestMarksHourAndFraction(t *testing.T) {
	db := marksDB(t)
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < KillMarksAnHour; i++ {
		if _, _, ok, err := db.AwardMarks(7, MarksAward{Source: "kill", Base: 1, Bonus: 1}, now.Add(time.Duration(i)*time.Second)); err != nil || !ok {
			t.Fatalf("kill %d: %v %v", i, ok, err)
		}
	}
	if _, _, ok, _ := db.AwardMarks(7, MarksAward{Source: "kill", Base: 1, Bonus: 1}, now.Add(time.Minute)); ok {
		t.Fatal("the 39th kill mark in the hour was given")
	}
	// a quest's is not a kill's: no cap
	if _, _, ok, _ := db.AwardMarks(7, MarksAward{Source: "quest", Base: 10, Bonus: 1}, now.Add(time.Minute)); !ok {
		t.Fatal("a quest's award refused by the kill cap")
	}
	if _, m, ok, _ := db.AwardMarks(7, MarksAward{Source: "kill", Base: 1, Bonus: 1}, now.Add(KillHour*time.Second)); !ok || m.KillHourMarks != 1 {
		t.Fatalf("a new hour: %v, %d", ok, m.KillHourMarks)
	}
	total := 0
	for i := 0; i < 4; i++ {
		n, _, _, err := db.AwardMarks(8, MarksAward{Source: "quest", Base: 1, Bonus: 1.3}, now)
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	m, _ := db.CharMarksOf(8)
	if total != 5 || m.Marks != 5 || m.Frac < 0.19 || m.Frac > 0.21 {
		t.Fatalf("four 1.3s: %d marks (%d), %.3f carried", total, m.Marks, m.Frac)
	}
	if _, err := db.SpendMarks(8, 6, "too much", nil, now); !errors.Is(err, ErrNotEnoughMarks) {
		t.Fatalf("spending 6 of 5: %v", err)
	}
	if after, err := db.SpendMarks(8, 5, "the lot", func(c *CharMarks) error { c.RingRank = 1; return nil }, now); err != nil || after.Marks != 0 || after.RingRank != 1 {
		t.Fatalf("spending 5: %+v %v", after, err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM charmarks_log WHERE serverid = 8`).Scan(&n)
	if n != 5 {
		t.Errorf("character 8's log: %d lines, want 5", n)
	}
}

// A camp zone fits a creature spawned inside its radius in its world; a quest zone its key's prefix or
// an id it lists. Saving and deleting a zone is audited.
func TestHotZonesFitAndAudit(t *testing.T) {
	camp := HotZone{Kind: HotZoneCamp, World: 0, X: 100, Z: 100, Radius: 50}
	if !camp.FitsSpawn(0, 130, 130) || camp.FitsSpawn(0, 200, 100) || camp.FitsSpawn(1, 100, 100) {
		t.Fatal("a camp zone's fit")
	}
	q := HotZone{Kind: HotZoneQuests, Quests: "homelands/erudite, 101500"}
	if !q.FitsQuest(9, "homelands/erudite/L1") || !q.FitsQuest(101500, "") || q.FitsQuest(9, "homelands/dwarf/L1") {
		t.Fatal("a quest zone's fit")
	}
	db := marksDB(t)
	id, err := db.SaveHotZone(HotZone{Name: "Hexbones", Enabled: true, Kind: HotZoneCamp, Radius: 150, BonusPct: 10}, GMEdit{AccountID: 1, Username: "gm", Via: "game"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveHotZone(HotZone{ID: id, Name: "Hexbones", Enabled: false, Kind: HotZoneCamp, Radius: 150, BonusPct: 10}, GMEdit{AccountID: 1, Username: "gm"}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteHotZone(id, GMEdit{AccountID: 1, Username: "gm"}, "test"); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'hotzones'`).Scan(&n)
	if n != 3 {
		t.Fatalf("%d audit lines, want 3", n)
	}
}

// Every table of the marks has a sanitiser rule.
func TestMarksTablesHaveSnapshotRules(t *testing.T) {
	for _, tbl := range []string{"charmarks", "charmarks_log", "charrelive", "hotzones", "heirloomrankac"} {
		if _, ok := snapshotRules[tbl]; !ok {
			t.Errorf("no snapshot rule for %s", tbl)
		}
	}
}
