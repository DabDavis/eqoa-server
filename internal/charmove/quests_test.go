package charmove

import (
	"database/sql"
	"io"
	"strings"
	"testing"
)

// questDB is migrateDB's database with three quests for the shadow knight
// (class 3) Wanderer: a path whose step 2 asks a choice (its second option
// stays), a were line's whose step gives Werewolf Form and, by class, Dark
// Shroud to a shadow knight alone, and a flag half done already.
func questDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := migrateDB(t)
	for _, q := range []string{
		`INSERT INTO quests (questid, name, enabled, source) VALUES (501, 'The Path', 1, ''), (502, 'Fryda''s Kin', 1, ''), (503, 'The Key', 1, '')`,
		`INSERT INTO queststeps (questid, step, kind, target, spells, byclass, choices) VALUES
			(501, 1, 'talk', 'A', '', '', ''),
			(501, 2, 'talk', 'A', '', '', '{"ask": "Which?", "options": [{"answer": "The Shroud", "spells": "13"}, {"answer": "Not yet", "stay": true}, {"answer": "The Blade", "items": "1297*1"}]}'),
			(502, 1, 'talk', 'B', '14', '{"3": {"spells": "12"}, "9": {"spells": "11"}}', ''),
			(503, 1, 'talk', 'C', '', '', ''), (503, 2, 'talk', 'C', '', '', '')`,
		`INSERT INTO charquests (serverid, questid, step, count, done, slot, updated, picks) VALUES (126, 503, 2, 1, 0, 4, 'then', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db
}

func questRows(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT questid, step, count, done, slot, updated, picks FROM charquests WHERE serverid = 126 ORDER BY questid`,
		`SELECT spellid, addedorder FROM spells WHERE serverid = 126 ORDER BY id`,
		`SELECT serverid, source FROM charvampire`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			for _, v := range vals {
				b.WriteString(cell(v))
				b.WriteByte('|')
			}
			b.WriteByte('\n')
		}
		rows.Close()
		b.WriteString("--\n")
	}
	return b.String()
}

// Quests done and a vampire: the path's row finished with its pick and the
// picked option's spell given, the were quest's spell and the class's own
// (not another class's) given, the half-done flag finished (its slot kept),
// a spell the book holds not given twice, the vampire made; a dry run writes
// nothing; the undo puts every row back exactly; the controls: a pick of an
// option that stays, of a step with no choice, and a quest this server lacks
// refuse the plan.
func TestQuestsAndVampire(t *testing.T) {
	db := questDB(t)
	before := questRows(t, db)
	p := Plan{Character: "wanderer", Vampire: true, Quests: []QuestDone{{Quest: 501, Picks: "2:0"}, {Quest: 502}, {Quest: 503}}}
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if _, err := Migrate(db, p, false, false, &out); err != nil {
		t.Fatal(err)
	}
	if questRows(t, db) != before {
		t.Fatal("a dry run wrote")
	}
	for _, want := range []string{`Quest 501 "The Path" done, picking "The Shroud" (was not taken)`, `Quest 503 "The Key" done (was at step 2)`,
		"Spells of the quests into the book: Mastered Shroud, Werewolf Form, Dark Shroud", "Vampire: made one"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the dry run lacks %q:\n%s", want, out.String())
		}
	}
	run, err := Migrate(db, p, true, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got := questRows(t, db)
	for _, want := range []string{"501|0|0|1|0|", "|2:0|", "502|0|0|1|0|", "503|0|0|1|4|", "13|1|", "14|2|", "12|3|", "126|transfer|"} {
		if !strings.Contains(got, want) {
			t.Errorf("after the run, no %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "11|") != 1 {
		t.Errorf("Suffering, in the book already, given again:\n%s", got)
	}
	if taken, left, err := Undo(db, run, false, io.Discard); err != nil || left != 0 || taken == 0 {
		t.Fatalf("undo: %d taken, %d left, %v", taken, left, err)
	}
	if after := questRows(t, db); after != before {
		t.Fatalf("the undo left\n%s\nwas\n%s", after, before)
	}
	for _, bad := range []Plan{
		{Character: "wanderer", Quests: []QuestDone{{Quest: 501, Picks: "2:1"}}},
		{Character: "wanderer", Quests: []QuestDone{{Quest: 501, Picks: "1:0"}}},
		{Character: "wanderer", Quests: []QuestDone{{Quest: 999}}},
	} {
		if _, err := Migrate(db, bad, false, false, io.Discard); err == nil {
			t.Errorf("%+v: taken", bad.Quests)
		}
	}
	for _, bad := range []Plan{
		{Character: "wanderer", Quests: []QuestDone{{Quest: 501}, {Quest: 501}}},
		{Character: "wanderer", Quests: []QuestDone{{Quest: 501, Picks: "two:one"}}},
	} {
		if err := bad.Check(); err == nil {
			t.Errorf("%+v: checked", bad.Quests)
		}
	}
}

// An undo leaves a quest the player has done more of since the run.
func TestUndoLeavesAQuestChangedSince(t *testing.T) {
	db := questDB(t)
	run, err := Migrate(db, Plan{Character: "wanderer", Quests: []QuestDone{{Quest: 503}}}, true, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE charquests SET done = 2 WHERE serverid = 126 AND questid = 503`); err != nil {
		t.Fatal(err)
	}
	if _, left, err := Undo(db, run, false, io.Discard); err != nil || left != 1 {
		t.Fatalf("left %d, %v", left, err)
	}
}
