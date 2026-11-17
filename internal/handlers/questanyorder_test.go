package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The Holiday Elf's "Gifts for Freeport" as the holiday quests are written:
// two visits (Tailor Bariston, Bowyer Koll) done in any order, then back to
// the elf; repeatable daily.
func anyOrderFixture(t *testing.T) (*questFixture, uint32, uint32, uint32) {
	t.Helper()
	f := newQuestFixture(t)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable, offer, accept, decline, started)
		VALUES (102901, 'Gifts for Freeport', 1, 'Holiday Elf', 1, 60, 2, 'Will you deliver gifts?', 'I will.', 'Not now.', 'Off you go.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, answer, says, anyorder) VALUES
		(102901, 1, 'talk', 'Tailor Bariston', 'Bring a Christmas gift to Tailor Bariston in Freeport.', 'A gift for you.', 'Bariston: For me?', 1),
		(102901, 2, 'talk', 'Bowyer Koll', 'Bring a Christmas gift to Bowyer Koll in Freeport.', 'A gift for you.', 'Koll: A gift!', 1),
		(102901, 3, 'talk', 'Holiday Elf', 'Return to the Holiday Elf by Freeport''s Christmas tree.', '', 'Elf: Every gift delivered!', 0)`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	elf := f.tw.add(11, &entity.NPC{ServerID: 811, Name: "Holiday Elf", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	tailor := f.tw.add(12, &entity.NPC{ServerID: 812, Name: "Tailor Bariston", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	bowyer := f.tw.add(13, &entity.NPC{ServerID: 813, Name: "Bowyer Koll", SpawnX: 25404.5, SpawnY: 54.1, SpawnZ: 15725})
	return f, elf, tailor, bowyer
}

// visit talks to npc and answers its step's question.
func (f *questFixture) visit(npc uint32) []string {
	f.t.Helper()
	f.tw.talk(npc)
	m := f.msgs()
	if strings.Contains(strings.Join(m, "\n"), "ask#") {
		f.tw.pick(boxID(m), 0)
		m = append(m, f.msgs()...)
	}
	return m
}

func TestAnyOrderStepsCompleteInEitherOrder(t *testing.T) {
	for _, order := range []string{"tailor first", "bowyer first"} {
		t.Run(order, func(t *testing.T) {
			f, elf, tailor, bowyer := anyOrderFixture(t)
			first, second := tailor, bowyer
			firstName, secondName := "Tailor Bariston", "Bowyer Koll"
			if order == "bowyer first" {
				first, second = bowyer, tailor
				firstName, secondName = secondName, firstName
			}
			f.tw.talk(elf)
			f.tw.pick(boxID(f.msgs()), 0)
			if _, lines := f.journal(); len(lines) != 1 || lines[0] != "Gifts for Freeport: Tailor Bariston, Bowyer Koll" {
				t.Fatalf("taken: the journal says who to see: %q", lines)
			}
			// the elf has nothing for the player until both are seen
			f.tw.talk(elf)
			if m := f.msgs(); strings.Contains(strings.Join(m, "\n"), "Every gift delivered") {
				t.Fatalf("the elf took the return before the visits: %q", m)
			}
			m := f.visit(first)
			if !strings.Contains(strings.Join(m, "\n"), ": For me?") && !strings.Contains(strings.Join(m, "\n"), ": A gift!") {
				t.Fatalf("the first visit's words: %q", m)
			}
			_, lines := f.journal()
			if len(lines) != 1 || !strings.Contains(lines[0], secondName) || strings.Contains(lines[0], firstName) {
				t.Fatalf("after %s: the journal is %q, want %s left", firstName, lines, secondName)
			}
			// the one seen again: nothing more
			f.tw.talk(first)
			if m := f.msgs(); strings.Contains(strings.Join(m, "\n"), "ask#") {
				t.Fatalf("%s asked again: %q", firstName, m)
			}
			f.visit(second)
			if s, _ := f.progress(102901); s != 3 {
				t.Fatalf("both seen: step %d, want 3 (back to the elf)", s)
			}
			if _, lines := f.journal(); len(lines) != 1 || !strings.HasPrefix(lines[0], "Return to the Holiday Elf") {
				t.Fatalf("both seen: the journal is %q", lines)
			}
			m = f.visit(elf)
			if !strings.Contains(strings.Join(m, "\n"), "Every gift delivered") {
				t.Fatalf("the return: %q", m)
			}
			if s, d := f.progress(102901); s != 0 || d != 1 {
				t.Fatalf("finished: step %d done %d, want 0 1", s, d)
			}
		})
	}
}

// A player part way through before the steps were any order (on step 2,
// Bariston seen) keeps that: Koll is all that is left.
func TestAnyOrderKeepsARowFromBefore(t *testing.T) {
	f, elf, tailor, bowyer := anyOrderFixture(t)
	f.tw.talk(elf)
	f.tw.pick(boxID(f.msgs()), 0)
	f.msgs()
	f.qe.Stop()
	f.exec(`UPDATE charquests SET step = 2, count = 0 WHERE serverid = ? AND questid = 102901`, f.sid)
	qe, err := NewQuestEngine(f.db, f.tw.w, f.tw.mgr, f.inv, database.NewSpellbookStore(f.db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	f.qe = qe
	if _, lines := f.journal(); len(lines) != 1 || !strings.Contains(lines[0], "Bowyer Koll") || strings.Contains(lines[0], "Bariston") {
		t.Fatalf("an old row on step 2: the journal is %q", lines)
	}
	f.tw.talk(tailor)
	if m := f.msgs(); strings.Contains(strings.Join(m, "\n"), "ask#") {
		t.Fatalf("Bariston, seen before, asked again: %q", m)
	}
	f.visit(bowyer)
	if s, _ := f.progress(102901); s != 3 {
		t.Fatalf("Koll seen: step %d, want 3", s)
	}
}

// Its group: the run of anyorder steps at NPCs; a lone one, or a step not at
// an NPC, is none.
func TestAnyOrderGroup(t *testing.T) {
	q := &database.Quest{Steps: []database.QuestStep{
		{Step: 1, Kind: "talk", AnyOrder: true}, {Step: 2, Kind: "give", AnyOrder: true}, {Step: 3, Kind: "talk"},
		{Step: 4, Kind: "talk", AnyOrder: true}, {Step: 5, Kind: "kill", AnyOrder: true},
	}}
	if g := q.AnyOrderGroup(2); len(g) != 2 || g[0].Step != 1 || g[1].Step != 2 {
		t.Errorf("step 2's group: %v", g)
	}
	if q.AnyOrderGroup(3) != nil || q.AnyOrderGroup(4) != nil || q.AnyOrderGroup(5) != nil {
		t.Error("no group for a plain step, a lone one, or a kill")
	}
}
