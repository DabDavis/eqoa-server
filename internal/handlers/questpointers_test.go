package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// pointerFixture is the quest fixture with Four Sashes' shape (petition 1692):
// 20001 ends at Ozmo, whose last words name Tilvyis's place; 20002, which
// requires it, is Tilvyis's. The engine's worker is stopped, so the pointers
// are synced when the test says (syncPointers).
func pointerFixture(t *testing.T) (*questFixture, uint32, uint32) {
	f := newQuestFixture(t)
	f.qe.Stop()
	questEngine.Store(f.qe) // the running engine still, its worker stopped
	t.Cleanup(func() { questEngine.CompareAndSwap(f.qe, nil) })
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (20001, 'Four Sashes', 1, 'Ozmo da Basher', 1)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal, says) VALUES (20001, 1, 'talk', 'Ozmo da Basher',
		'Speak to Ozmo.', 'Tilvyis, in the south of Neriak west of the coachman, want you now.')`)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, requires)
		VALUES (20002, 'The Breastplate and the Leggings', 1, 'Tilvyis S` + "`" + `Lynx', 1, '20001')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20002, 1, 'talk', 'Tilvyis S` + "`" + `Lynx', 'Hear Tilvyis out.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	ozmo := f.tw.add(1, &entity.NPC{ServerID: 801, Name: "Ozmo da Basher", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	tilvyis := f.tw.add(2, &entity.NPC{ServerID: 802, Name: "Tilvyis S`Lynx", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	return f, ozmo, tilvyis
}

const tilvyisLine = "The Breastplate and the Leggings: find Tilvyis S`Lynx in the south of Neriak west of the coachman"

// Hige's case end to end: the quest finished at Ozmo, its line goes and the
// pointer to Tilvyis comes in its place; a login's dump carries it; taking
// the quest at Tilvyis turns it into the quest's own line; finished, nothing.
func TestAFinishedQuestPointsToTheNextGiver(t *testing.T) {
	f, ozmo, tilvyis := pointerFixture(t)
	f.tw.talk(ozmo) // no question: taken at once
	f.want(f.msgs(), "add:Speak to Ozmo.")
	f.tw.talk(ozmo) // the step: finished
	f.want(f.msgs(), "drop:0", "box:"+questFinished)
	f.qe.syncPointers()
	f.want(f.msgs(), "add:"+tilvyisLine)
	f.qe.syncPointers()
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("an unchanged journal was sent again: %q", m)
	}

	// a login: the dump has it
	if _, lines := f.qe.JournalFor(f.tw.s, f.tw.p); len(lines) != 1 || lines[0] != tilvyisLine {
		t.Errorf("the dump's journal: %q", lines)
	}

	// Tilvyis offers it; taken, the pointer's line is the quest's own
	f.tw.talk(tilvyis)
	m := f.msgs()
	if id := boxID(m); id != 0 {
		f.tw.pick(id, 0)
		m = append(m, f.msgs()...)
	}
	f.want(m, "drop:0", "add:Hear Tilvyis out.")
	f.qe.syncPointers()
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("the pointer came back after the quest was taken: %q", m)
	}
	if _, lines := f.qe.JournalFor(f.tw.s, f.tw.p); len(lines) != 1 || lines[0] != "Hear Tilvyis out." {
		t.Errorf("the journal with the quest taken: %q", lines)
	}
}

// The pointer's rules: none for a quest the player may not take yet (level),
// none with the flag off, none when the finished quest ended at the next
// one's giver; a quest under way keeps its place before it, and the
// player's delete of it is kept for the session.
func TestNextGiverPointerRules(t *testing.T) {
	f, _, _ := pointerFixture(t)
	f.exec(`INSERT INTO charquests (serverid, questid, step, done, updated) VALUES (?, 20001, 0, 1, '2026-11-11')`, f.sid)
	f.qe.ReloadCharacter(f.sid)
	f.msgs()

	f.exec(`UPDATE quests SET minlevel = 60 WHERE questid = 20002`)
	f.qe.Reload()
	if _, lines := f.qe.JournalFor(f.tw.s, f.tw.p); len(lines) != 0 {
		t.Errorf("a level 5 is pointed to a level 60 quest: %q", lines)
	}
	f.exec(`UPDATE quests SET minlevel = 1 WHERE questid = 20002`)
	f.qe.Reload()

	SetNextGiverPointer(false)
	_, lines := f.qe.JournalFor(f.tw.s, f.tw.p)
	SetNextGiverPointer(true)
	if len(lines) != 0 {
		t.Errorf("the flag off still points: %q", lines)
	}

	// the control: on, it points
	token, lines := f.qe.JournalFor(f.tw.s, f.tw.p)
	if len(lines) != 1 || lines[0] != tilvyisLine {
		t.Fatalf("the journal: %q", lines)
	}

	// a quest taken comes before it
	malsis := f.tw.add(3, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.want(f.msgs(), "drop:0", "add:Go speak to Kellina.")
	f.qe.syncPointers()
	f.want(f.msgs(), "add:"+tilvyisLine)
	token, lines = f.qe.JournalFor(f.tw.s, f.tw.p)
	if len(lines) != 2 || lines[0] != "Go speak to Kellina." || lines[1] != tilvyisLine {
		t.Fatalf("the journal: %q", lines)
	}

	// the player deletes the pointer: gone for the session, the quest kept
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDeleteQuest)
	msg.SetData(build(protocol.OpcodeDeleteQuest, 16, func(w *buffer.Writer) {
		w.WriteUint32(token)
		w.Write7BitEncodedInt64(1)
	})[2:])
	if err := f.qe.HandleDrop(f.tw.s, msg); err != nil {
		t.Fatal(err)
	}
	f.want(f.msgs(), "drop:1")
	if s, _ := f.progress(10010); s != 1 {
		t.Errorf("deleting the pointer gave up the quest (step %d)", s)
	}
	f.qe.syncPointers()
	if m := f.msgs(); len(m) != 0 {
		t.Errorf("a deleted pointer came back: %q", m)
	}
	f.qe.LoggedIn(f.sid)
	if _, lines := f.qe.JournalFor(f.tw.s, f.tw.p); len(lines) != 2 {
		t.Errorf("after a login the pointer is back: %q", lines)
	}

	// a quest that ended at the next one's giver points nowhere
	f.exec(`UPDATE quests SET giver = 'Ozmo da Basher' WHERE questid = 20002`)
	f.qe.Reload()
	if _, lines := f.qe.JournalFor(f.tw.s, f.tw.p); len(lines) != 1 {
		t.Errorf("pointed back to the NPC just finished with: %q", lines)
	}
}

// More pointers than room: the quests under way keep theirs, the pointers
// take at most pointerCap lines of the rest, the last saying how many more.
func TestPointersKeepToTheRoomLeft(t *testing.T) {
	f, _, _ := pointerFixture(t)
	f.exec(`INSERT INTO charquests (serverid, questid, step, done, updated) VALUES (?, 20001, 0, 1, '2026-11-11')`, f.sid)
	for i := 0; i < 5; i++ {
		id := 20010 + i
		f.exec(`INSERT INTO quests (questid, name, enabled, giver, requires) VALUES (?, ?, 1, ?, '20001')`, id, "Next "+string(rune('A'+i)), "Giver "+string(rune('A'+i)))
		f.exec(`INSERT INTO queststeps (questid, step, kind, target) VALUES (?, 1, 'talk', ?)`, id, "Giver "+string(rune('A'+i)))
	}
	f.qe.Reload()
	f.qe.ReloadCharacter(f.sid)
	_, lines := f.qe.JournalFor(f.tw.s, f.tw.p)
	if len(lines) != pointerCap || !strings.HasSuffix(lines[pointerCap-1], "(+3 more)") {
		t.Fatalf("six pointers: %q", lines)
	}
	// six quests under way leave room for two
	for i := 0; i < 6; i++ {
		id := 20100 + i
		f.exec(`INSERT INTO quests (questid, name, enabled, giver) VALUES (?, 'Busy', 1, 'Nobody')`, id)
		f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (?, 1, 'talk', 'Nobody', ?)`, id, "Busy "+string(rune('0'+i)))
		f.exec(`INSERT INTO charquests (serverid, questid, step, slot) VALUES (?, ?, 1, ?)`, f.sid, id, i+1)
	}
	f.qe.Reload()
	f.qe.ReloadCharacter(f.sid)
	_, lines = f.qe.JournalFor(f.tw.s, f.tw.p)
	if len(lines) != questJournalLines || lines[5] != "Busy 5" || !strings.HasSuffix(lines[7], "(+4 more)") {
		t.Fatalf("six quests and pointers: %q", lines)
	}
}

// Where the words put the giver.
func TestPlaceInSays(t *testing.T) {
	for _, c := range []struct{ says, giver, want string }{
		{"Tilvyis, in the south of Neriak west of the coachman, want you now.", "Tilvyis S`Lynx", "in the south of Neriak west of the coachman"},
		{"Neriak will be pleased. Go to the monastery of Shon-To, north-east of Hatespike, and speak with Ediopolus by the lake.", "Ediopolus",
			"by the lake at the monastery of Shon-To"},
		{"Well done. Next, see Sergeant Grem.", "Sergeant Grem", ""},
		{"Your trainer in Forkwatch has your reward.", "Crikston Gundlefop", ""},
	} {
		if got := placeInSays(c.says, c.giver); got != c.want {
			t.Errorf("placeInSays(%q, %q) = %q, want %q", c.says, c.giver, got, c.want)
		}
	}
}
