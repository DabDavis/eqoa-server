package database

import (
	"path/filepath"
	"testing"
	"time"
)

func rehabTemplateCount(t *testing.T, db *SQLiteDB, serverID uint32) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rehabtemplates WHERE serverid = ?`, serverID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A character's two templates: each slot written and read back with its
// training points and ranks in their order, a slot saved again replaced, the
// other slot and another character's untouched; a slot past 2 refused; a
// deleted character's templates gone (the control: another's stay). The
// table's names read as no secret, so a snapshot keeps it.
func TestRehabTemplatesKept(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "tpl.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteCharacterStore(db)
	a, b := &CharacterRecord{CharName: "Anja", AccountID: 1}, &CharacterRecord{CharName: "Frejo", AccountID: 1}
	if err := store.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(b); err != nil {
		t.Fatal(err)
	}
	if got, err := db.RehabTemplates(a.ServerID); err != nil || len(got) != 0 {
		t.Fatalf("at first: %v %v", got, err)
	}
	at := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	one := RehabTemplate{Slot: 1, Level: 50, TP: [7]int{50, 20, 0, 0, 0, 0, -3},
		CMs: []OwnedCM{{ID: 20, Cost: 200}, {ID: 1, Cost: 10}, {ID: 2, Cost: 15}}}
	two := RehabTemplate{Slot: 2, Level: 51, TP: [7]int{0, 30}, CMs: []OwnedCM{{ID: 50, Cost: 100}}}
	for _, tp := range []RehabTemplate{one, two} {
		if err := db.SaveRehabTemplate(a.ServerID, tp, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveRehabTemplate(b.ServerID, two, at); err != nil {
		t.Fatal(err)
	}
	got, err := db.RehabTemplates(a.ServerID)
	if err != nil || len(got) != 2 {
		t.Fatalf("read back: %v %v", got, err)
	}
	g := got[1]
	if g.Level != 50 || g.TP != [7]int{50, 20} || FormatTemplateCMs(g.CMs) != "20:200,1:10,2:15" || g.Saved != "2026-11-09T03:00:00Z" ||
		g.TPTotal() != 70 || g.CMTotal() != 225 || g.Empty() {
		t.Errorf("slot 1: %+v", g)
	}
	// saved again: replaced; slot 2 and the other character's untouched
	one.CMs, one.TP = []OwnedCM{{ID: 1, Cost: 10}}, [7]int{5}
	if err := db.SaveRehabTemplate(a.ServerID, one, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ = db.RehabTemplates(a.ServerID)
	if FormatTemplateCMs(got[1].CMs) != "1:10" || got[1].TP != [7]int{5} || got[1].Saved != "2026-11-09T04:00:00Z" {
		t.Errorf("slot 1 saved again: %+v", got[1])
	}
	if FormatTemplateCMs(got[2].CMs) != "50:100" || got[2].TP != [7]int{0, 30} {
		t.Errorf("the control: slot 2 changed: %+v", got[2])
	}
	if other, _ := db.RehabTemplates(b.ServerID); len(other) != 1 || other[2].Level != 51 {
		t.Errorf("the control: another's: %+v", other)
	}
	for _, slot := range []int{0, 3} {
		if err := db.SaveRehabTemplate(a.ServerID, RehabTemplate{Slot: slot}, at); err == nil {
			t.Errorf("slot %d kept", slot)
		}
	}
	// a slot a GM wrote out of range is not read
	db.Exec(`INSERT INTO rehabtemplates (serverid, slot) VALUES (?, 7)`, a.ServerID)
	if got, _ := db.RehabTemplates(a.ServerID); len(got) != 2 {
		t.Errorf("a slot 7 read: %v", got)
	}
	if err := store.Delete(a.ServerID); err != nil {
		t.Fatal(err)
	}
	if n := rehabTemplateCount(t, db, a.ServerID); n != 0 {
		t.Errorf("a deleted character's templates: %d rows", n)
	}
	if n := rehabTemplateCount(t, db, b.ServerID); n != 1 {
		t.Errorf("the control: another's templates: %d rows", n)
	}
	for _, name := range []string{"rehabtemplates", "slot", "level", "cms", "saved", "tpstrength", "tpcharisma"} {
		if SecretLikeName(name) {
			t.Errorf("%q reads like a secret: every snapshot would be refused", name)
		}
	}
	if (RehabTemplate{}).Empty() != true || (RehabTemplate{TP: [7]int{0, 0, 1}}).Empty() {
		t.Errorf("Empty")
	}
}

// A cms column reads in its order: what is not a positive id skipped, an id
// twice once (its first), a cost left out 0, a negative one 0.
func TestParseTemplateCMs(t *testing.T) {
	got := FormatTemplateCMs(ParseTemplateCMs(" 6634:10, x:5,, 2:15, 6634:99, 3, -4:2, 0:1, 9:-7 "))
	if got != "6634:10,2:15,3:0,9:0" {
		t.Errorf("parsed: %q", got)
	}
	if len(ParseTemplateCMs("")) != 0 {
		t.Errorf("an empty column has ranks")
	}
}

// A template used is kept in one transaction each: the tp columns moved by the
// difference (a point a trainer took meanwhile stays), the unspent and total
// points the game's; the ranks no longer held gone, those held kept with their
// first purchase's time, the new ones and the points written.
func TestRespendTrainingAndSetOwnedCMs(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "respend.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteCharacterStore(db)
	c := &CharacterRecord{CharName: "Talker", AccountID: 1}
	if err := store.Create(c); err != nil {
		t.Fatal(err)
	}
	// the row: STR 30, STA 10 (and 2 more on DEX a trainer took after the
	// game read it)
	db.Exec(`UPDATE characters SET tpstrength = 30, tpstamina = 10, tpdexterity = 2, unusedTP = 58, totalTP = 100 WHERE serverid = ?`, c.ServerID)
	if err := store.RespendTraining(c.ServerID, [7]int{30, 10}, [7]int{5, 40}, 53, 100); err != nil {
		t.Fatal(err)
	}
	rec, _ := store.GetByServerID(c.ServerID)
	if rec.TPStrength != 5 || rec.TPStamina != 40 || rec.TPDexterity != 2 || rec.UnusedTP != 53 || rec.TotalTP != 100 {
		t.Errorf("the row: STR %d STA %d DEX %d unspent %d of %d", rec.TPStrength, rec.TPStamina, rec.TPDexterity, rec.UnusedTP, rec.TotalTP)
	}
	if err := store.RespendTraining(99999, [7]int{}, [7]int{}, 0, 0); err == nil {
		t.Errorf("a character that is none respent")
	}

	db.Exec(`INSERT INTO charcms (serverid, cmid, cost, bought) VALUES (?, 1, 10, 111), (?, 2, 15, 222), (?, 21, 200, 333)`,
		c.ServerID, c.ServerID, c.ServerID)
	if err := store.SetOwnedCMs(c.ServerID, []int{21}, []OwnedCM{{ID: 1, Cost: 12}, {ID: 2, Cost: 15}, {ID: 20, Cost: 200}},
		MasteryRow{Points: 205, Spent: 227, Percent: 40, XP: 9}); err != nil {
		t.Fatal(err)
	}
	owned, _ := store.LoadOwnedCMs(c.ServerID)
	if FormatTemplateCMs(owned) != "1:12,2:15,20:200" {
		t.Errorf("charcms: %q", FormatTemplateCMs(owned))
	}
	var bought int64
	db.QueryRow(`SELECT bought FROM charcms WHERE serverid = ? AND cmid = 2`, c.ServerID).Scan(&bought)
	if bought != 222 {
		t.Errorf("a rank held before and after: bought %d, want its first purchase's 222", bought)
	}
	db.QueryRow(`SELECT bought FROM charcms WHERE serverid = ? AND cmid = 20`, c.ServerID).Scan(&bought)
	if bought < 1_700_000_000 {
		t.Errorf("a new rank's purchase time: %d", bought)
	}
	if m, _ := store.LoadMastery(c.ServerID); m != (MasteryRow{Points: 205, Spent: 227, Percent: 40, XP: 9}) {
		t.Errorf("charmastery: %+v", m)
	}
	order, _ := store.OwnedCMsInOrder(c.ServerID)
	if FormatTemplateCMs(order) != "1:12,2:15,20:200" {
		t.Errorf("in the order bought: %q", FormatTemplateCMs(order))
	}
	db.Exec(`UPDATE charcms SET bought = 50 WHERE serverid = ? AND cmid = 20`, c.ServerID)
	if order, _ := store.OwnedCMsInOrder(c.ServerID); FormatTemplateCMs(order) != "20:200,1:12,2:15" {
		t.Errorf("in the order bought: %q", FormatTemplateCMs(order))
	}
}
