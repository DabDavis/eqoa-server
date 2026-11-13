package handlers

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// headMarksSent is the last OpcodeQuestHeadMarks queued for s, by object id
// ("!" or "?"), and whether one was; the queue is cleared.
func headMarksSent(t *testing.T, s *session.Session) (map[uint32]string, bool) {
	t.Helper()
	var body []byte
	found := false
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestHeadMarks {
			body, found = d[2:], true
		}
	}
	s.Queue().Clear()
	if !found {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint16(body))
	if len(body) != 2+5*n {
		t.Fatalf("a set of %d marks in %d bytes", n, len(body))
	}
	out := map[uint32]string{}
	for i := 0; i < n; i++ {
		e := body[2+5*i:]
		out[binary.LittleEndian.Uint32(e)] = map[byte]string{1: "!", 2: "?"}[e[4]]
	}
	return out, true
}

// headLook is the engine's marks worked out now and the World's look at the
// session: what its client was sent.
func (f *questFixture) headLook(t *testing.T) (map[uint32]string, bool) {
	t.Helper()
	f.qe.syncHeadMarks(time.Now())
	f.tw.w.SendQuestHeadMarks(f.tw.s)
	return headMarksSent(t, f.tw.s)
}

func headAnswer(t *testing.T, w *entity.World, s *session.Session, on byte) {
	t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeQuestHeadMarksAnswer)
	msg.SetData([]byte{on})
	if err := NewQuestHeadMarksHandler(w).HandleAnswer(s, msg); err != nil {
		t.Fatal(err)
	}
}

func setHeadMode(t *testing.T, mode string) {
	t.Helper()
	if err := SetQuestHeadMarks(mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetQuestHeadMarks("") })
}

func (f *questFixture) headSame(t *testing.T, what string, got map[uint32]string, ok bool, want map[uint32]string) {
	t.Helper()
	if !ok {
		t.Fatalf("%s: nothing sent, want %v", what, want)
	}
	if len(got) != len(want) {
		t.Fatalf("%s: %v, want %v", what, got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %v, want %v", what, got, want)
		}
	}
}

// Petition 1813's marks through a quest (the fixture's The pincers: Malsis
// gives it, a talk to Kellina, then two cracked ant pincers given her), the
// quest flagged in table questmarks: "!" over Malsis while it may be taken;
// taken, "?" over Kellina for the talk; at the give step none until the
// pincers are held, then "?" again; finished, none. The controls: an
// unflagged quest marks nothing (quests.markers flagged, the default), and
// another class's quest shows no "!".
func TestQuestHeadMarksFollowTheQuest(t *testing.T) {
	setHeadMode(t, "")
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Warden", SpawnX: 25405, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes) VALUES (10011, 'For warriors', 1, 'Warden', 1, 10, '0')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10011, 1, 'talk', 'Kellina', 'w')`)
	f.exec(`INSERT INTO questmarks (questid, note) VALUES (10011, 'a warrior''s')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	headAnswer(t, f.tw.w, f.tw.s, 1)
	got, ok := f.headLook(t)
	f.headSame(t, "the controls: The pincers unflagged, the warrior's quest not the magician's", got, ok, map[uint32]string{})

	f.exec(`INSERT INTO questmarks (questid, note) VALUES (10010, 'test')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	got, ok = f.headLook(t)
	f.headSame(t, "flagged", got, ok, map[uint32]string{malsis: "!"})
	if _, ok := f.headLook(t); ok {
		t.Fatal("nothing changed, sent again")
	}

	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	got, ok = f.headLook(t)
	f.headSame(t, "taken", got, ok, map[uint32]string{kellina: "?"})

	f.tw.talk(kellina)
	f.msgs()
	got, ok = f.headLook(t)
	f.headSame(t, "the give step, no pincers", got, ok, map[uint32]string{})

	f.own(f.pincer, 2)
	got, ok = f.headLook(t)
	f.headSame(t, "the pincers held", got, ok, map[uint32]string{kellina: "?"})

	f.tw.talk(kellina)
	if b := boxID(f.msgs()); b != 0 {
		f.tw.pick(b, 0)
	}
	if f.held(f.pincer) != 0 {
		t.Fatalf("the pincers were not handed in (%d held)", f.held(f.pincer))
	}
	got, ok = f.headLook(t)
	f.headSame(t, "finished", got, ok, map[uint32]string{})

	// stopped: nothing more
	headAnswer(t, f.tw.w, f.tw.s, 0)
	f.exec(`DELETE FROM charquests`)
	f.tw.p.Level = 6
	if _, ok := f.headLook(t); ok {
		t.Fatal("sent after the client stopped them")
	}
}

// quests.markers all marks every quest (the control: flagged, the same
// unflagged quest shows nothing), a "!" only near the player's level; off
// makes no offer and takes no answer.
func TestQuestHeadMarksModes(t *testing.T) {
	setHeadMode(t, "all")
	f := newQuestFixture(t)
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.msgs()
	headAnswer(t, f.tw.w, f.tw.s, 1)
	got, ok := f.headLook(t)
	f.headSame(t, "all", got, ok, map[uint32]string{malsis: "!"})

	SetQuestHeadMarks("flagged")
	got, ok = f.headLook(t)
	f.headSame(t, "flagged, none flagged", got, ok, map[uint32]string{})

	SetQuestHeadMarks("off")
	f.tw.s.Queue().Clear()
	SendQuestHeadMarksOffer(f.tw.s)
	for _, m := range f.tw.s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestHeadMarksOffer {
			t.Fatal("an offer made with quests.markers off")
		}
	}
	SetQuestHeadMarks("flagged")
	SendQuestHeadMarksOffer(f.tw.s)
	var offer []byte
	for _, m := range f.tw.s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestHeadMarksOffer {
			offer = d
		}
	}
	if string(offer) != "\x61\x7e\x01" {
		t.Fatalf("the offer %x, want 617e01", offer)
	}
	if err := SetQuestHeadMarks("sometimes"); err == nil {
		t.Fatal("quests.markers took a word it does not know")
	}
}

// The GM Jibrish's petition 1875: "After turning in a quest, if the quest NPC has a follow up it is not showing a new
// yellow ! for the newbie quests". The fixture's The pincers flagged (as each race's and class's first quest is), its
// follow-up (Malsis's, requiring it) and that one's own (Kellina's) flagged by no one: before the turn-in the
// follow-up shows nothing (it may not be taken yet); at the turn-in that finishes The pincers, "!" over Malsis at
// once; taken and finished, "!" over Kellina for the next. The control: an unflagged quest's follow-up shows nothing.
func TestQuestHeadMarksShowAFollowUp(t *testing.T) {
	setHeadMode(t, "")
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Warden", SpawnX: 25405, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, requires) VALUES
		(10012, 'The next pincers', 1, 'Malsis', 1, 10, '10010'),
		(10013, 'The last pincers', 1, 'Kellina', 1, 10, '10012'),
		(10020, 'Unflagged', 1, 'Warden', 1, 10, ''),
		(10021, 'After unflagged', 1, 'Warden', 1, 10, '10020')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (10012, 1, 'talk', 'Malsis', 'n'),
		(10013, 1, 'talk', 'Warden', 'l'), (10020, 1, 'talk', 'Malsis', 'u'), (10021, 1, 'talk', 'Malsis', 'a')`)
	f.exec(`INSERT INTO questmarks (questid, note) VALUES (10010, 'a first quest')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	headAnswer(t, f.tw.w, f.tw.s, 1)
	got, ok := f.headLook(t)
	f.headSame(t, "before: The pincers alone (no follow-up, nothing unflagged)", got, ok, map[uint32]string{malsis: "!"})

	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.tw.talk(kellina)
	f.msgs()
	f.own(f.pincer, 2)
	f.headLook(t)
	f.tw.talk(kellina)
	if b := boxID(f.msgs()); b != 0 {
		f.tw.pick(b, 0)
	}
	if f.held(f.pincer) != 0 {
		t.Fatalf("the pincers were not handed in (%d held)", f.held(f.pincer))
	}
	got, ok = f.headLook(t)
	f.headSame(t, "turned in: the follow-up's ! at once", got, ok, map[uint32]string{malsis: "!"})

	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.tw.talk(malsis)
	if b := boxID(f.msgs()); b != 0 {
		f.tw.pick(b, 0)
	}
	got, ok = f.headLook(t)
	f.headSame(t, "the follow-up done: the next one's !", got, ok, map[uint32]string{kellina: "!"})
}
