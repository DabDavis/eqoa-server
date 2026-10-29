package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// sentProgress is the last 0x7E06 queued for the session, decoded (nil,
// false when none was).
func sentProgress(t *testing.T, s *session.Session) ([]questProgress, bool) {
	t.Helper()
	var body []byte
	found := false
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeQuestProgressData {
			body, found = d[2:], true
		}
	}
	if !found {
		return nil, false
	}
	return decodeProgress(t, body), true
}

func decodeProgress(t *testing.T, b []byte) []questProgress {
	t.Helper()
	text := func() string {
		n := int(b[0])
		s := string(b[1 : 1+n])
		b = b[1+n:]
		return s
	}
	n := int(b[0])
	b = b[1:]
	out := make([]questProgress, n)
	for i := range out {
		out[i].name = text()
		ng := int(b[0])
		b = b[1:]
		for k := 0; k < ng; k++ {
			g := progressGoal{text: text()}
			g.have, g.need = int(binary.LittleEndian.Uint16(b)), int(binary.LittleEndian.Uint16(b[2:]))
			b = b[4:]
			out[i].goals = append(out[i].goals, g)
		}
	}
	if len(b) != 0 {
		t.Fatalf("%d bytes past the tracker", len(b))
	}
	return out
}

// show is a tracker as one line, for the comparisons.
func show(qs []questProgress) string {
	var parts []string
	for _, qp := range qs {
		var gs []string
		for _, g := range qp.goals {
			if g.need > 0 {
				gs = append(gs, fmt.Sprintf("%s %d/%d", g.text, g.have, g.need))
			} else {
				gs = append(gs, g.text)
			}
		}
		parts = append(parts, qp.name+": "+strings.Join(gs, ", "))
	}
	return strings.Join(parts, "; ")
}

// The tracker is the journal's quests with what each step asks, counted (the
// user: "if quest says obtain 3 bat eyes, have 0/3"): empty with none under
// way; a talk step's NPC; a give step's items held of those wanted, counted
// in the session's own list, and the NPC to give them to. It goes out again
// only when it changes (the control: nothing changed, nothing sent), and not
// at all once the client stops it.
func TestQuestProgressIsTheJournalCounted(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	malsis := f.tw.add(1, &entity.NPC{ServerID: 701, Name: "Malsis", SpawnX: 25403.5, SpawnY: 54.1, SpawnZ: 15725.5})
	kellina := f.tw.add(2, &entity.NPC{ServerID: 702, Name: "Kellina", SpawnX: 25404, SpawnY: 54.1, SpawnZ: 15726})
	f.msgs()
	f.qe.SetProgress(f.tw.s, f.tw.p, true)
	if got, ok := sentProgress(t, f.tw.s); !ok || len(got) != 0 {
		t.Fatalf("no quest under way: %q (sent %v), want an empty tracker", show(got), ok)
	}
	f.tw.talk(malsis)
	f.tw.pick(boxID(f.msgs()), 0)
	f.qe.syncProgress()
	if got, _ := sentProgress(t, f.tw.s); show(got) != "The pincers: Speak with Kellina" {
		t.Fatalf("taken: %q", show(got))
	}
	// no drain between: the worker may have sent it already (the step's
	// save nudges it), and sentProgress takes the last one queued
	f.tw.talk(kellina)
	f.qe.syncProgress()
	if got, _ := sentProgress(t, f.tw.s); show(got) != "The pincers: cracked ant pincer 0/2, Give to Kellina" {
		t.Fatalf("the give step: %q", show(got))
	}
	f.qe.syncProgress()
	if got, ok := sentProgress(t, f.tw.s); ok {
		t.Fatalf("nothing changed, and %q went out again", show(got))
	}
	// a pincer gained, in the session's list: 1 of 2
	sessionInvList(f.tw.s).add(9001, f.pincer, 1, 20)
	f.qe.syncProgress()
	if got, _ := sentProgress(t, f.tw.s); show(got) != "The pincers: cracked ant pincer 1/2, Give to Kellina" {
		t.Fatalf("a pincer held: %q", show(got))
	}
	// stopped: nothing more
	f.qe.SetProgress(f.tw.s, f.tw.p, false)
	sessionInvList(f.tw.s).add(9002, f.pincer, 1, 20)
	f.qe.syncProgress()
	if got, ok := sentProgress(t, f.tw.s); ok {
		t.Fatalf("sent %q after the client stopped it", show(got))
	}
}

// A kill step's kills and a loot step's drops are counted as they come: the
// worker sends the tracker itself after a kill (saveLocked's nudge, no
// second's tick waited for), and a drop taken counts on its step; the
// control: another item taken counts for nothing and sends nothing.
func TestQuestProgressCountsKillsAndDrops(t *testing.T) {
	f := newQuestFixture(t)
	f.tw.p.HP = 100
	eye := f.pattern("Bat Eye", 20)
	f.exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel, maxlevel) VALUES (30, 'Pawns', 1, 'Malsis', 1, 10), (31, 'Bat eyes', 1, 'Malsis', 1, 10)`)
	f.exec(`INSERT INTO queststeps (questid, step, kind, target, count, journal) VALUES (30, 1, 'kill', 'Orc Pawn', 3, 'Kill three orc pawns.')`)
	f.exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, count, journal) VALUES (31, 1, 'loot', 'Bat', '%d', 3, 'Bring three bat eyes.')`, eye))
	if err := f.qe.Reload(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{30, 31} {
		if _, err := f.qe.SetCharacterQuest(f.sid, id, QuestGive, 0); err != nil {
			t.Fatal(err)
		}
	}
	f.msgs()
	f.qe.SetProgress(f.tw.s, f.tw.p, true)
	if got, _ := sentProgress(t, f.tw.s); show(got) != "Pawns: Orc Pawn 0/3; Bat eyes: Bat Eye 0/3" {
		t.Fatalf("at first: %q", show(got))
	}
	f.qe.heardKill(entity.KillCredit{NPCName: "Orc Pawn", Players: []entity.KillPlayer{{Session: f.tw.s, ServerID: f.sid, Name: "Talker"}}})
	want := "Pawns: Orc Pawn 1/3; Bat eyes: Bat Eye 0/3"
	var last string
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if got, ok := sentProgress(t, f.tw.s); ok {
			if last = show(got); last == want {
				break
			}
		}
	}
	if last != want {
		t.Fatalf("after a kill: %q, want %q sent by the worker", last, want)
	}
	f.qe.Looted(f.tw.s, f.tw.p, eye, 1)
	f.qe.syncProgress()
	if got, _ := sentProgress(t, f.tw.s); show(got) != "Pawns: Orc Pawn 1/3; Bat eyes: Bat Eye 1/3" {
		t.Fatalf("a bat eye taken: %q", show(got))
	}
	f.qe.Looted(f.tw.s, f.tw.p, f.bone, 1)
	f.qe.syncProgress()
	if got, ok := sentProgress(t, f.tw.s); ok {
		t.Fatalf("bone chips taken, and %q went out", show(got))
	}
}

// The body is bounded: past maxProgressBytes the later quests are left out,
// a long text is clipped at a rune with its mark, and a count past a u16 is
// shown as the most it holds.
func TestQuestProgressStaysBounded(t *testing.T) {
	long := strings.Repeat("é", 100)
	var qs []questProgress
	for i := 0; i < 40; i++ {
		qp := questProgress{name: long}
		for k := 0; k < maxProgressGoals; k++ {
			qp.goals = append(qp.goals, progressGoal{text: long, have: 70000, need: 3})
		}
		qs = append(qs, qp)
	}
	body := encodeQuestProgress(qs)
	if len(body) > maxProgressBytes {
		t.Fatalf("%d bytes, past %d", len(body), maxProgressBytes)
	}
	got := decodeProgress(t, body)
	if len(got) == 0 || len(got) >= len(qs) {
		t.Fatalf("%d quests kept of %d", len(got), len(qs))
	}
	if s := got[0].name; len(s) > maxProgressText || !strings.HasSuffix(s, "...") || !strings.HasPrefix(s, "é") {
		t.Fatalf("a long name %q (%d bytes)", s, len(s))
	}
	if g := got[0].goals[0]; g.have != 0xFFFF || g.need != 3 {
		t.Fatalf("the counts %+v", g)
	}
}

// A quest saved nudges the worker (the marks and trackers looked at now, not
// at the next second), once at most and never blocking; the control: nothing
// saved, no nudge.
func TestAQuestSavedNudgesTheWorker(t *testing.T) {
	f := newQuestFixture(t)
	q := &QuestEngine{store: database.NewQuestStore(f.db), wake: make(chan struct{}, 1)}
	woken := func() bool {
		select {
		case <-q.wake:
			return true
		default:
			return false
		}
	}
	if woken() {
		t.Fatal("a nudge with nothing saved")
	}
	q.saveLocked(&database.CharQuest{ServerID: f.sid, QuestID: 10010, Step: 1})
	q.saveLocked(&database.CharQuest{ServerID: f.sid, QuestID: 10010, Step: 2})
	if !woken() {
		t.Fatal("saved, and no nudge")
	}
	if woken() {
		t.Fatal("two saves, two nudges: one is enough")
	}
}
