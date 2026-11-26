package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// "Unlock my masteries" (petition 1391) on a real server's data (EQOA_REAL_DB
// names its database, a copy of which is opened): for every class he opens
// the seven lines and marks their 28 trials (the stat-boost quests,
// Misc/stat-boost-quests, 101195-101222: four a stat), every one in the game, a
// trial a character finished left; and in a level 60 character's mastery
// window each line's first rank is refused for its trial before, and no rank
// of the seven is refused for a trial after (their levels and the ranks
// before them still asked, the points aside). Skipped without a database.
func TestRealArchivistUnlocksTheStatMasteries(t *testing.T) {
	db := realRehabDB(t)
	qe, err := NewQuestEngine(db, nil, nil, database.NewInventoryStore(db), nil)
	if err != nil {
		t.Fatal(err)
	}
	arch, err := loadSpellArchive(database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	inGame := qe.InGame()
	var want []int
	for id := 101195; id <= 101222; id++ {
		want = append(want, id)
	}
	var names []string
	for _, id := range want {
		name, in := inGame[id]
		if !in {
			t.Errorf("trial %d is not in the game", id)
		}
		names = append(names, fmt.Sprintf("%d %s", id, name))
	}
	t.Logf("the trials: %s", strings.Join(names, "; "))
	for class := 0; class <= database.MaxClassID; class++ {
		lines, mark := unlockQuests(arch, inGame, map[int]bool{}, class, 0)
		if fmt.Sprint(lines) != fmt.Sprint(archivistUnlockLines) || fmt.Sprint(mark) != fmt.Sprint(want) {
			t.Errorf("%s: lines %v, trials %v; want %v and %v", database.TransferClassSlugs[class], lines, mark, archivistUnlockLines, want)
		}
		if _, mark := unlockQuests(arch, inGame, map[int]bool{101195: true, 101215: true}, class, 0); fmt.Sprint(mark) != fmt.Sprint(append(append([]int{}, want[1:20]...), want[21:]...)) {
			t.Errorf("%s: with two trials finished, trials %v", database.TransferClassSlugs[class], mark)
		}
	}

	rows, err := db.CMCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := newArchivistFixture(t, db)
	f.w.SetCMCatalog(rows)
	f.p.Level, f.p.Class = 60, 0
	f.p.Mastery.Points, f.p.Mastery.Spent = 1500, 1500
	before := entity.CMKnows{Book: map[int]bool{}, Quests: map[int]bool{}, QuestNames: inGame}
	after := entity.CMKnows{Book: map[int]bool{}, Quests: map[int]bool{}, QuestNames: inGame}
	for _, id := range want {
		after.Quests[id] = true
	}
	ranks := 0
	for _, r := range rows {
		if !r.Enabled || unlockLine(r.Name) == "" {
			continue
		}
		ranks++
		n, ok := f.w.CMNodeOf(f.sess, r.ID, before)
		if !ok {
			t.Errorf("%s (%d) is not offered to a warrior", r.Name, r.ID)
			continue
		}
		if r.Parent == 0 && !strings.Contains(n.Text, "You must finish") {
			t.Errorf("%s before: %q, want refused for its trial", r.Name, n.Text)
		}
		if n, _ = f.w.CMNodeOf(f.sess, r.ID, after); strings.Contains(n.Text, "You must finish") {
			t.Errorf("%s after: %q, still refused for a trial", r.Name, n.Text)
		}
	}
	if ranks != 28 {
		t.Errorf("%d ranks of the seven lines, want 28", ranks)
	}
}
