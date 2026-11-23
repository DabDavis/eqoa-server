package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Heirloom petition 2683: a ratman wizard took the good side's epic through
// its recruiter by mistake, deleted the epic's first quest and went to the
// evil side's giver, who sent him off for ever: the evil first quest excludes
// the good recruiter, which stayed finished. Giving up a path's first quest
// unfinished forgets the choice that led to it (a finished one-talk quest that
// pays nothing and excludes others, required by nothing else he holds), so the
// other side is offered. The controls: a choice that pays experience is kept,
// and so is one another quest the character holds requires.
func TestGivingUpAPathForgetsItsChoice(t *testing.T) {
	f := newQuestFixture(t)
	// the recruiters (choices), each excluding the other side's first quest
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline, excludes)
		VALUES (20001, 'The Recruiter', 1, 'Magi', 'Magi: Go?', 'Yes.', 'No.', '20012'),
		       (20002, 'The Recruiter', 1, 'Recruiter', 'Recruiter: Go?', 'Yes.', 'No.', '20011')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(20001, 1, 'talk', 'Magi', 'Speak to Magi.'), (20002, 1, 'talk', 'Recruiter', 'Speak to the recruiter.')`)
	// the sides' first quests, each excluding the other side's recruiter
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline, requires, excludes)
		VALUES (20011, 'Goo', 1, 'Magi', 'Magi: Goo?', 'Yes.', 'No.', '20001', '20002'),
		       (20012, 'Crystals', 1, 'Jen', 'Jen: Crystals and branches?', 'Yes.', 'No.', '20002|20009', '20001')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(20011, 1, 'talk', 'Kellina', 'Bring Magi goo.'), (20012, 1, 'talk', 'Kellina', 'Bring Jen crystals.')`)
	// the evil side's native recruiter, which pays nothing either and excludes nothing
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, offer, accept, decline) VALUES (20009, 'Native', 1, 'Recruiter', 'Recruiter: Native?', 'Yes.', 'No.')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20009, 1, 'talk', 'Recruiter', 'Speak to the recruiter.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	jen := f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Jen", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	set := func(rows string) {
		t.Helper()
		f.exec(`DELETE FROM charquests WHERE serverid = ?`, f.sid)
		f.exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES `+rows, f.sid, f.sid, f.sid)
		f.qe.ReloadCharacter(f.sid)
		f.msgs()
	}
	drop := func() {
		t.Helper()
		token, lines := f.journal()
		if len(lines) != 1 {
			t.Fatalf("journal %q, want the one quest", lines)
		}
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDeleteQuest)
		m.SetData(build(protocol.OpcodeDeleteQuest, 16, func(w *buffer.Writer) {
			w.WriteUint32(token)
			w.Write7BitEncodedInt64(0)
		})[2:])
		if err := f.qe.HandleDrop(f.tw.s, m); err != nil {
			t.Fatal(err)
		}
		f.msgs()
	}
	offersCrystals := func() bool {
		f.tw.talk(jen)
		for _, m := range f.msgs() {
			if len(m) > 4 && m[:4] == "ask#" && strings.Contains(m, "Crystals and branches?") {
				return true
			}
		}
		return false
	}

	// the good recruiter and the native one finished, the good first quest
	// under way: Jen offers nothing
	set("(?, 20001, 0, 1), (?, 20009, 0, 1), (?, 20011, 1, 0)")
	if offersCrystals() {
		t.Fatal("offered the evil side while holding the good")
	}
	drop()
	if s, d := f.progress(20001); s != -1 || d != -1 {
		t.Errorf("the good recruiter after giving up its path: step %d done %d, want forgotten", s, d)
	}
	if s, _ := f.progress(20009); s == -1 {
		t.Error("the native recruiter (excludes nothing) was forgotten")
	}
	if !offersCrystals() {
		t.Error("the evil side not offered after the good path was given up")
	}

	// the control: a recruiter that pays experience is no mere choice: kept
	f.exec(`UPDATE queststeps SET xp = 100 WHERE questid = 20001`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	set("(?, 20001, 0, 1), (?, 20009, 0, 1), (?, 20011, 1, 0)")
	drop()
	if _, d := f.progress(20001); d != 1 {
		t.Error("a recruiter that pays was forgotten")
	}
	f.exec(`UPDATE queststeps SET xp = 0 WHERE questid = 20001`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}

	// the control: another quest the character finished requires the choice: kept
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (20020, 'Other', 1, 'Magi', '20001')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20020, 1, 'talk', 'Magi', 'x')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.exec(`DELETE FROM charquests WHERE serverid = ?`, f.sid)
	f.exec(`INSERT INTO charquests (serverid, questid, step, done) VALUES (?, 20001, 0, 1), (?, 20020, 0, 1), (?, 20011, 1, 0)`, f.sid, f.sid, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.msgs()
	drop()
	if _, d := f.progress(20001); d != 1 {
		t.Error("a choice another finished quest requires was forgotten")
	}
}
