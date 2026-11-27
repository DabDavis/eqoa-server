package database

import (
	"path/filepath"
	"testing"
)

// A quest's place (Quest.OfferAt; handlers/questplaceoffer.go, petitions
// 3322/3325): the columns made on an open, read with a radius above 0, none
// with 0; Within asks the world, the radius round x, z and, with a height,
// the y.
func TestAQuestsPlaceIsReadAndAsked(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, c := range []string{"offerworld", "offerx", "offery", "offerz", "offerradius", "offerheight"} {
		if has, err := hasColumn(db.DB, "quests", c); err != nil || !has {
			t.Fatalf("quests.%s missing (%v)", c, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO quests (questid, name, enabled, offerworld, offerx, offery, offerz, offerradius, offerheight)
		VALUES (1, 'At the altar', 1, 1, 5408.1, 44.5, 9159.4, 6, 3), (2, 'At a giver', 1, 1, 5408.1, 44.5, 9159.4, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	qs, err := NewQuestStore(db).LoadQuests()
	if err != nil || len(qs) != 2 {
		t.Fatalf("read %d quests: %v", len(qs), err)
	}
	pl := qs[0].OfferAt
	if pl == nil || pl.World != 1 || pl.Radius != 6 || pl.Height != 3 || pl.X != 5408.1 || pl.Z != 9159.4 {
		t.Fatalf("quest 1's place: %+v", pl)
	}
	if qs[1].OfferAt != nil {
		t.Errorf("radius 0 read as a place: %+v", qs[1].OfferAt)
	}
	for _, c := range []struct {
		world   int
		x, y, z float32
		want    bool
	}{
		{1, 5408.1, 44.5, 9159.4, true},
		{1, 5412, 44.5, 9162, true},      // 4.9 off
		{1, 5414.2, 44.5, 9159.4, false}, // 6.1 off
		{0, 5408.1, 44.5, 9159.4, false}, // another world
		{1, 5408.1, 48, 9159.4, false},   // 3.5 above
		{1, 5408.1, 42, 9159.4, true},    // 2.5 below
	} {
		if got := pl.Within(c.world, c.x, c.y, c.z); got != c.want {
			t.Errorf("Within(%d, %v, %v, %v) = %v, want %v", c.world, c.x, c.y, c.z, got, c.want)
		}
	}
	var none *QuestPlace
	if none.Within(1, 5408.1, 44.5, 9159.4) {
		t.Error("no place is within")
	}
}
