package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// "Unlock my masteries" (petition 1391, archivistunlock.go) against a
// database, the quests running and the mastery window's handler over the same
// books and quests (the server's wiring). A human warrior of 30 beside him,
// with points to spend: Bulging Muscles 1 and 2 (levels 20 and 30) and
// Superior Intellect 1, each behind its trial (cmcatalog.require_quests), and
// Unrivaled Grace 1 behind its own, a line he does not open; the trials made
// up as the import keys them, Bulging Muscles' second requiring the first.

const (
	uqStr1, uqStr2, uqInt1, uqDex1 = 101195, 101196, 101215, 101207
	ucStr1, ucStr2, ucInt1, ucDex1 = 11, 12, 111, 91
)

func newUnlockRig(t *testing.T) (*archivistFixture, *ClassMasteryHandler, *QuestEngine) {
	t.Helper()
	f := newArchivistFixture(t, nil)
	f.p.Class, f.p.Level = 0, 30
	f.p.Mastery.Points = 100
	f.shop(SpellArchivistName, 0x81, archivistShop)
	trial := func(id int, key, name string, minlevel int, requires string) {
		t.Helper()
		if _, err := f.db.Exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes, requires, source)
			VALUES (?, ?, 1, 'Goran the Strong', ?, 60, '', ?, ?)`, id, name, minlevel, requires, "EQOA-Data "+key+" (tools/import_quests.py)"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', 'Goran the Strong', 'Talk to Goran.')`,
			id); err != nil {
			t.Fatal(err)
		}
	}
	trial(uqStr1, "Misc/stat-boost-quests/str1", "Bulging Muscles: The First Trial", 20, "")
	trial(uqStr2, "Misc/stat-boost-quests/str2", "Bulging Muscles: The Second Trial", 30, fmt.Sprint(uqStr1))
	trial(uqInt1, "Misc/stat-boost-quests/int1", "Superior Intellect: Three Riddles", 20, "")
	trial(uqDex1, "Misc/stat-boost-quests/dex1", "Unrivaled Grace: The First Step", 20, "")
	rank := func(id int, mastery, name string, parent, level, quest int) {
		t.Helper()
		if _, err := f.db.Exec(`INSERT INTO cmcatalog (id, category, classes, races, mastery, rank, name, parent, cost, level,
			require_quests, effects, enabled, source) VALUES (?, 0, 32767, 2047, ?, 1, ?, ?, 5, ?, ?, 'STR:10', 1, 'test')`,
			id, mastery, name, parent, level, fmt.Sprint(quest)); err != nil {
			t.Fatal(err)
		}
	}
	rank(ucStr1, "Buldging Muscles", "Bulging Muscles 1", 0, 20, uqStr1)
	rank(ucStr2, "Buldging Muscles", "Bulging Muscles 2", ucStr1, 30, uqStr2)
	rank(ucInt1, "Superior Intellect", "Superior Intellect 1", 0, 20, uqInt1)
	rank(ucDex1, "Unrivaled Grace", "Unrivaled Grace 1", 0, 20, uqDex1)
	qe, err := NewQuestEngine(f.db, f.w, f.mgr, f.inv, f.spells)
	if err != nil {
		t.Fatal(err)
	}
	qe.Start() // the mastery window reads the running engine's (Quests)
	t.Cleanup(qe.Stop)
	f.a.quests = func() *QuestEngine { return qe }
	rows, err := f.db.CMCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.w.SetCMCatalog(rows)
	h := NewClassMasteryHandler(f.mgr)
	h.SetGameWorld(f.w)
	h.SetStore(database.NewSQLiteCharacterStore(f.db))
	h.SetSpellbook(f.spells)
	return f, h, qe
}

// Before he is asked, each rank is refused for its trial (the control: a
// character who has not asked cannot buy them). Asked, he marks the trials of
// the lines he opens finished, no other, gives no rank and spends no point,
// and says so; then each rank of them is bought in the window as any, its
// level and the rank before it still needed, while Unrivaled Grace 1 is still
// refused for its trial. Asked again, he says they are open already. Another
// character's trials are its own.
func TestTheArchivistUnlocksTheStatMasteries(t *testing.T) {
	f, h, qe := newUnlockRig(t)
	for id, quest := range map[int]string{ucStr1: "Bulging Muscles: The First Trial", ucInt1: "Superior Intellect: Three Riddles"} {
		red := f.buyRank(h, id)
		if len(red) != 1 || !strings.Contains(red[0], "You must finish "+quest) {
			t.Errorf("mastery %d before asking: %q, want refused for %s", id, red, quest)
		}
	}

	got := f.pick(f.menu(), archivistUnlock)
	f.wantMsgs("unlock", got, "box:Spell Archivist: "+fmt.Sprintf(archivistUnlocked, "Bulging Muscles and Superior Intellect"))
	done := qe.FinishedBy(f.p.CharServerID)
	for _, q := range []int{uqStr1, uqStr2, uqInt1} {
		if !done[q] {
			t.Errorf("trial %d not finished after asking", q)
		}
	}
	if done[uqDex1] {
		t.Errorf("Unrivaled Grace's trial finished: he opens only %v", archivistUnlockLines)
	}
	var owned int
	if err := f.db.QueryRow(`SELECT count(*) FROM charcms WHERE serverid = ?`, f.p.CharServerID).Scan(&owned); err != nil || owned != 0 {
		t.Errorf("asking him: %d ranks owned (%v), want none", owned, err)
	}
	if f.p.Mastery.Points != 100 || len(f.p.CMs) != 0 {
		t.Errorf("asking him: points %d, ranks %v; want 100 and none", f.p.Mastery.Points, f.p.CMs)
	}
	if other := qe.FinishedBy(f.p.CharServerID + 1); len(other) != 0 {
		t.Errorf("another character's finished quests: %v", other)
	}

	if red := f.buyRank(h, ucStr2); len(red) != 1 || !strings.Contains(red[0], "Bulging Muscles 1 first") {
		t.Errorf("Bulging Muscles 2 before 1: %q, want refused for the rank before it", red)
	}
	for _, id := range []int{ucStr1, ucStr2, ucInt1} {
		if red := f.buyRank(h, id); len(red) != 0 {
			t.Errorf("mastery %d after asking: %q", id, red)
		}
	}
	if red := f.buyRank(h, ucDex1); len(red) != 1 || !strings.Contains(red[0], "You must finish Unrivaled Grace: The First Step") {
		t.Errorf("Unrivaled Grace 1 after asking: %q, want still refused for its trial", red)
	}
	if f.p.Mastery.Points != 85 {
		t.Errorf("after buying three ranks at 5: %d points, want 85", f.p.Mastery.Points)
	}

	f.wantMsgs("asked again", f.pick(f.menu(), archivistUnlock),
		"box:Spell Archivist: "+fmt.Sprintf(archivistUnlockedAll, "Bulging Muscles and Superior Intellect"))
}

// The line a rank is of, by its name less its number; the data's "Buldging"
// mastery column is not read.
func TestUnlockLine(t *testing.T) {
	for name, want := range map[string]string{"Bulging Muscles 1": "Bulging Muscles", "bulging muscles 4": "Bulging Muscles",
		"Intestinal Fortitude 2": "Intestinal Fortitude", "Superior Intellect 3": "Superior Intellect", "Superior Intellect": "Superior Intellect",
		"Brawn 2": "", "Unrivaled Grace 1": "", "Sages Wit 1": "", "Bulging Muscles Of Doom 1": ""} {
		if got := unlockLine(name); got != want {
			t.Errorf("unlockLine(%q) = %q, want %q", name, got, want)
		}
	}
}
