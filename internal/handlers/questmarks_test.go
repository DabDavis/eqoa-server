package handlers

import (
	"encoding/binary"
	"math"
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
	f.tw.talk(kellina)
	f.msgs()
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
