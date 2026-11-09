package handlers

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// sentMarks is the last 0x7E04 queued for the session, decoded (nil, false
// when none was).
func sentMarks(t *testing.T, s *session.Session) ([]questMark, bool) {
	t.Helper()
	var body []byte
	found := false
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestMarkersData {
			body, found = d[2:], true
		}
	}
	if !found {
		return nil, false
	}
	return decodeMarks(t, body), true
}

func decodeMarks(t *testing.T, b []byte) []questMark {
	t.Helper()
	text := func() string {
		n := int(b[0])
		s := string(b[1 : 1+n])
		b = b[1+n:]
		return s
	}
	n := int(b[0])
	b = b[1:]
	out := make([]questMark, n)
	for i := range out {
		m := &out[i]
		m.kind, m.world = b[0], b[1]
		m.x = math.Float32frombits(binary.LittleEndian.Uint32(b[2:]))
		m.z = math.Float32frombits(binary.LittleEndian.Uint32(b[6:]))
		b = b[10:]
		m.npc = text()
		nq := int(b[0])
		b = b[1:]
		for k := 0; k < nq; k++ {
			var mq markQuest
			mq.name = text()
			mq.min, mq.max = int(b[0]), int(b[1])
			b = b[2:]
			mq.line = text()
			mq.detail = text()
			m.quests = append(m.quests, mq)
		}
	}
	if len(b) != 0 {
		t.Fatalf("%d bytes past the marks", len(b))
	}
	return out
}

// The marks are what the player may do (the user: "add quest markers for
// givers to the map too", "only if eligible for the class/level"): a "!" at
// the giver of a quest it may take, with the quest's levels, the start of the
// offer and its rewards; none for a quest of another class or levels (the
// controls); once taken, no "!" there but a "?" at the step's NPC with its
// journal line, and what a give step needs.
func TestQuestMarksAreWhatThePlayerMayDo(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	for _, n := range []*entity.NPC{f.tw.w.NPCs[701], f.tw.w.NPCs[702]} {
		n.World = 0
	}
	// the controls: Malsis's quests for a warrior and for level 20
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, classes) VALUES
		(10011, 'For warriors', 1, 'Malsis', 1, 10, '0'), (10012, 'Later', 1, 'Malsis', 20, 30, '10')`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(10011, 1, 'talk', 'Kellina', 'w'), (10012, 1, 'talk', 'Kellina', 'l')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	f.msgs()

	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, ok := sentMarks(t, f.tw.s)
	if !ok || len(marks) != 1 {
		t.Fatalf("marks %+v (sent %v), want Malsis's one", marks, ok)
	}
	m := marks[0]
	if m.kind != markTake || m.npc != "Malsis" || m.x != 25403.5 || m.z != 15725.5 || len(m.quests) != 1 {
		t.Fatalf("the giver's mark %+v", m)
	}
	if q := m.quests[0]; q.name != "The pincers" || q.min != 1 || q.max != 10 || q.line != "You there." ||
		!strings.Contains(q.detail, "430 XP") || !strings.Contains(q.detail, "Smoldering Aura") {
		t.Fatalf("its quest %+v, want the pincers' levels, offer and rewards", q)
	}

	// nothing new: nothing sent
	f.qe.syncMarks(time.Now())
	if _, ok := sentMarks(t, f.tw.s); ok {
		t.Fatal("the same set sent again")
	}

	// taken: Malsis's "!" goes, Kellina's "?" comes
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.qe.syncMarks(time.Now())
	marks, ok = sentMarks(t, f.tw.s)
	if !ok || len(marks) != 1 || marks[0].kind != markStep || marks[0].npc != "Kellina" ||
		marks[0].quests[0].line != "Go speak to Kellina." {
		t.Fatalf("after taking it: %+v (sent %v), want Kellina's step", marks, ok)
	}
	// no drain between: the worker may have sent the set already (the
	// step's save nudges it), and sentMarks takes the last one queued
	f.tw.talk(kellina)
	f.qe.syncMarks(time.Now())
	marks, _ = sentMarks(t, f.tw.s)
	if len(marks) != 1 || marks[0].quests[0].detail != "2 cracked ant pincer" ||
		marks[0].quests[0].line != "Bring Kellina two cracked ant pincers." {
		t.Fatalf("the give step's mark %+v, want what it needs", marks)
	}

	// off: nothing more
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	f.tw.p.Level = 6
	f.qe.syncMarks(time.Now())
	if _, ok := sentMarks(t, f.tw.s); ok {
		t.Fatal("sent after the client stopped them")
	}
}

// A kill step's creatures get a hunting mark at their nearest spawns with the
// kills done; a mark is at most maxMarkSpots of a name's spawns.
func TestQuestMarksHuntAtTheNearestSpawns(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	for i, x := range []float32{25600, 25450, 26000} {
		f.tw.add(3+i, &entity.NPC{ServerID: entity.EntityID(800 + i), Name: "Orc Pawn ", SpawnX: x, SpawnZ: 15725, Level: 2})
	}
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (30, 'Pawns', 1, 'Malsis', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (30, 1, 'kill', 'Orc Pawn', 3, 'Kill three orc pawns.')`)
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.qe.SetCharacterQuest(f.sid, 30, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(t, f.tw.s)
	var hunt []questMark
	for _, m := range marks {
		if m.kind == markHunt {
			hunt = append(hunt, m)
		}
	}
	if len(hunt) != maxMarkSpots || hunt[0].x != 25450 || hunt[1].x != 25600 {
		t.Fatalf("hunting marks %+v, want the two nearest pawns' spawns", hunt)
	}
	if d := hunt[0].quests[0].detail; d != "Orc Pawn: 0 of 3 killed" {
		t.Fatalf("its detail %q", d)
	}
}

// The body is bounded: past maxMarksBytes the farther marks are left out, and
// a long text is clipped at a rune with its mark.
func TestQuestMarksStayBounded(t *testing.T) {
	long := strings.Repeat("é", 200)
	var marks []questMark
	for i := 0; i < 300; i++ {
		marks = append(marks, questMark{kind: markTake, npc: long, quests: []markQuest{{name: long, line: long, detail: long}}})
	}
	body := encodeQuestMarks(marks)
	if len(body) > maxMarksBytes {
		t.Fatalf("%d bytes, past %d", len(body), maxMarksBytes)
	}
	got := decodeMarks(t, body)
	if len(got) == 0 || len(got) >= len(marks) {
		t.Fatalf("%d marks kept of %d", len(got), len(marks))
	}
	if s := got[0].npc; len(s) > maxMarkText || !strings.HasSuffix(s, "...") || !strings.HasPrefix(s, "é") {
		t.Fatalf("a long name %q (%d bytes)", s, len(s))
	}
}

// takeMarksBy is the marks sent now, as "giver: quest" for each quest to take
// on a "!", and "?giver" for each step's mark; the marks are off again after.
func (f *questFixture) takeMarksBy() []string {
	f.t.Helper()
	f.qe.SetMarkers(f.tw.s, f.tw.p, true)
	marks, _ := sentMarks(f.t, f.tw.s)
	f.qe.SetMarkers(f.tw.s, f.tw.p, false)
	var out []string
	for _, m := range marks {
		if m.kind != markTake {
			out = append(out, "?"+m.npc)
			continue
		}
		for _, q := range m.quests {
			out = append(out, m.npc+": "+q.name)
		}
	}
	sort.Strings(out)
	return out
}

// A quest's name is marked once, at its nearest giver (the same quest is
// given in each city: Armorcrafting by twelve smiths), and by none of its
// givers while one of that name is under way or once one is finished; one
// that may be done again is marked again, at its nearest, once finished. The
// controls: another quest of the far giver's is marked there throughout, and
// with only the far giver standing, the far one is marked.
func TestQuestMarksANameOnceAtItsNearestGiver(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Smithy Near", SpawnX: 25500, SpawnZ: 15725})
	f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Smithy Far", SpawnX: 27000, SpawnZ: 15725})
	f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Kellina", SpawnX: 25404, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel, repeatable) VALUES
		(20001, 'Armorcrafting', 1, 'Smithy Far', 1, 60, 0), (20002, 'Armorcrafting', 1, 'Smithy Near', 1, 60, 0),
		(20003, 'Far Errand', 1, 'Smithy Far', 1, 60, 0),
		(20004, 'Chore', 1, 'Smithy Far', 1, 60, 1), (20005, 'Chore', 1, 'Smithy Near', 1, 60, 1)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(20001, 1, 'talk', 'Kellina', 'a'), (20002, 1, 'talk', 'Kellina', 'a'), (20003, 1, 'talk', 'Kellina', 'e'),
		(20004, 1, 'talk', 'Kellina', 'c'), (20005, 1, 'talk', 'Kellina', 'c')`)
	f.reload()
	f.msgs()
	want := func(when string, w ...string) {
		t.Helper()
		if got := f.takeMarksBy(); strings.Join(got, "|") != strings.Join(w, "|") {
			t.Fatalf("%s: marks %q, want %q", when, got, w)
		}
	}
	want("nothing taken", "Smithy Far: Far Errand", "Smithy Near: Armorcrafting", "Smithy Near: Chore")

	if _, err := f.qe.SetCharacterQuest(f.sid, 20002, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	f.msgs()
	want("Armorcrafting under way", "?Kellina", "Smithy Far: Far Errand", "Smithy Near: Chore")
	f.finish(20002)
	want("Armorcrafting finished", "Smithy Far: Far Errand", "Smithy Near: Chore")

	// a repeatable one, finished, is marked again at its nearest
	f.finish(20005)
	want("the chore finished", "Smithy Far: Far Errand", "Smithy Near: Chore")

	// the near smith gone: the far one's
	delete(f.tw.w.NPCs, 701)
	f.reload()
	f.exec(`DELETE FROM charquests`)
	f.qe.mu.Lock()
	delete(f.qe.chars, f.sid)
	f.qe.mu.Unlock()
	want("only the far smith standing", "Smithy Far: Armorcrafting", "Smithy Far: Chore", "Smithy Far: Far Errand")
}

// Only quests near the player's level are marked (world.quest_marks_below):
// unset, a level 60 is shown a quest for 50 and not one for 49 or for 1 (a
// quest of no lowest level counts as one for 1); 15 shows the one for 49, and
// -1 every one. The controls: a level 11 is shown the quests for 1, and a
// level 12 is not.
func TestQuestMarksNearThePlayersLevel(t *testing.T) {
	f := newQuestFixture(t)
	defer SetQuestMarksBelow(0)
	f.tw.p.HP, f.tw.p.Level = 100, 60
	f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Elder", SpawnX: 25500, SpawnZ: 15725})
	f.tw.add(3, &entity.NPC{ServerID: 703, Name: "Kellina", SpawnX: 25404, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES
		(20011, 'For fifty', 1, 'Elder', 50, 60), (20012, 'For forty-nine', 1, 'Elder', 49, 60),
		(20013, 'For one', 1, 'Elder', 1, 60), (20014, 'For anyone', 1, 'Elder', 0, 0)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES
		(20011, 1, 'talk', 'Kellina', 'a'), (20012, 1, 'talk', 'Kellina', 'b'), (20013, 1, 'talk', 'Kellina', 'c'),
		(20014, 1, 'talk', 'Kellina', 'd')`)
	f.reload()
	f.msgs()
	for _, c := range []struct {
		below, level int
		want         string
	}{
		{0, 60, "Elder: For fifty"},
		{10, 60, "Elder: For fifty"},
		{15, 60, "Elder: For fifty|Elder: For forty-nine"},
		{-1, 60, "Elder: For anyone|Elder: For fifty|Elder: For forty-nine|Elder: For one"},
		{0, 11, "Elder: For anyone|Elder: For one"},
		{0, 12, ""},
	} {
		SetQuestMarksBelow(c.below)
		f.tw.p.Level = c.level
		if got := strings.Join(f.takeMarksBy(), "|"); got != c.want {
			t.Errorf("quest_marks_below %d, level %d: marks %q, want %q", c.below, c.level, got, c.want)
		}
	}
}

// A giver where the player has a step to do shows that step's "?" alone, not
// a "!" for its own quest besides; once the step is done, its "!" is back.
// The control: the giver's "!" before the step is taken.
func TestQuestMarksAStepOverAGiversQuest(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25420, SpawnY: 54.1, SpawnZ: 15726})
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (20021, 'Kellina''s own', 1, 'Kellina', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, journal) VALUES (20021, 1, 'talk', 'Malsis', 'k')`)
	f.reload()
	f.msgs()
	if got, want := strings.Join(f.takeMarksBy(), "|"), "Kellina: Kellina's own|Malsis: The pincers"; got != want {
		t.Fatalf("before: marks %q, want %q", got, want)
	}
	// the pincers taken: its step at Kellina, Kellina's "!" gone
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	if got, want := strings.Join(f.takeMarksBy(), "|"), "?Kellina"; got != want {
		t.Fatalf("a step at Kellina: marks %q, want %q", got, want)
	}
}
