package handlers

import (
	"encoding/binary"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// talkHeard is what a session was sent, taken: each dialogue box's text, and the last question box's id,
// question and answers.
type talkHeard struct {
	boxes    []string
	box      uint32
	question string
	answers  []string
}

func heardTalk(s *session.Session) talkHeard {
	var h talkHeard
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 6 {
			continue
		}
		switch protocol.GameOpcode(binary.LittleEndian.Uint16(d)) {
		case protocol.OpcodeDialogueBox:
			h.boxes = append(h.boxes, wideText(d[6:]))
		case protocol.OpcodeQuestPopupMulti:
			p := 6
			str := func() string {
				n := int(binary.LittleEndian.Uint32(d[p:]))
				u := make([]uint16, n)
				for i := range u {
					u[i] = binary.LittleEndian.Uint16(d[p+4+2*i:])
				}
				p += 4 + 2*n
				return string(utf16.Decode(u))
			}
			h.box = binary.LittleEndian.Uint32(d[2:6])
			h.question = str()
			n := int(d[p])
			p++
			h.answers = nil
			for range n {
				h.answers = append(h.answers, str())
			}
		}
	}
	return h
}

// alanaxiaTree is a five-node tree as work board 883 filmed it (its first and last lines here).
func alanaxiaTree() map[string]map[int]database.TalkNode {
	return map[string]map[int]database.TalkNode{"alanaxia": {
		1: {NPC: "alanaxia", Node: 1, Says: []string{"With the Dominators retreated, Tunaria will recover from this famine."},
			Choices: []database.TalkChoice{{Text: "Tell me about the famine.", Next: 2}}},
		2: {NPC: "alanaxia", Node: 2, Says: []string{"Well, I'll start from the beginning.", "Josephine ran into the gates.", "We found out she was telling the truth."},
			Choices: []database.TalkChoice{{Text: "Where did the insects come from?", Next: 3}}},
		3: {NPC: "alanaxia", Node: 3, Says: []string{"The chiktar came from their hives.", "Some thought we were doomed to starve!"},
			Choices: []database.TalkChoice{{Text: "What was controlling these bugs?", Next: 4}}},
		4: {NPC: "alanaxia", Node: 4, Says: []string{"Several evil eye dominators.", "Everything seems back to normal."},
			Choices: []database.TalkChoice{{Text: "Are we safe from the famine now?", Next: 5}}},
		5: {NPC: "alanaxia", Node: 5, Says: []string{"Things are looking up.", "Cheers to you!"}},
	}}
}

// Alanaxia's five nodes play in order: each node's lines in boxes with her name, its last asked with the
// filmed answer and Exit, an answer playing the next; the last node ends with no question. Exit closes. The
// control: with no tree she says her npcdialog line, and a talk tree never takes a quest's NPC's talk.
func TestTalkTreePlaysInOrder(t *testing.T) {
	tw := newTalkWorld(t, 12513, 18669)
	ala := tw.add(1, &entity.NPC{ServerID: 801, Name: "Alanaxia", SpawnX: 12513.1, SpawnY: 54, SpawnZ: 18669.3})
	tw.h.SetDialogLines(func() ([]database.NPCDialogLine, error) {
		return []database.NPCDialogLine{{ID: 1, Name: "Alanaxia", Line: "Her npcdialog line.", Race: -1, Weight: 1, SpeakerRace: -1, Class: -1, NotRace: -1}}, nil
	})
	tw.talk(ala)
	if h := heardTalk(tw.s); h.box != 0 {
		t.Fatalf("the control: no tree, yet a question %q", h.question)
	}
	tw.h.SetTalkTrees(func() (map[string]map[int]database.TalkNode, error) { return alanaxiaTree(), nil })
	tw.talk(ala)
	h := heardTalk(tw.s)
	if len(h.boxes) != 0 || h.question != "With the Dominators retreated, Tunaria will recover from this famine." ||
		!slices.Equal(h.answers, []string{"Tell me about the famine.", "Exit"}) || h.box&0xFF000000 != talkBoxMark {
		t.Fatalf("node 1: %+v", h)
	}
	want := []struct {
		boxes   []string
		answers []string
	}{
		{[]string{"Alanaxia: Well, I'll start from the beginning.", "Alanaxia: Josephine ran into the gates."}, []string{"Where did the insects come from?", "Exit"}},
		{[]string{"Alanaxia: The chiktar came from their hives."}, []string{"What was controlling these bugs?", "Exit"}},
		{[]string{"Alanaxia: Several evil eye dominators."}, []string{"Are we safe from the famine now?", "Exit"}},
	}
	for i, w := range want {
		tw.pick(h.box, 0)
		h = heardTalk(tw.s)
		if !slices.Equal(h.boxes, w.boxes) || !slices.Equal(h.answers, w.answers) {
			t.Fatalf("node %d: %+v", i+2, h)
		}
	}
	tw.pick(h.box, 0)
	last := heardTalk(tw.s)
	if !slices.Equal(last.boxes, []string{"Alanaxia: Things are looking up.", "Alanaxia: Cheers to you!"}) || last.box != 0 {
		t.Fatalf("node 5: %+v", last)
	}
	// the old box's answer again does nothing; Exit closes
	tw.pick(h.box, 0)
	if got := opcodes(tw.s); len(got) != 0 {
		t.Errorf("an old box answered: %v", got)
	}
	tw.talk(ala)
	h = heardTalk(tw.s)
	tw.pick(h.box, 1)
	if got := opcodes(tw.s); len(got) != 0 {
		t.Errorf("Exit: %v", got)
	}
}

// A next naming no node closes the conversation; a box with more answers than the room keeps Exit last.
func TestTalkTreeMissingNodeAndRoom(t *testing.T) {
	tw := newTalkWorld(t, 100, 100)
	n := tw.add(1, &entity.NPC{ServerID: 802, Name: "Talker Npc", SpawnX: 100, SpawnY: 54, SpawnZ: 100})
	var many []database.TalkChoice
	for i := range 8 {
		many = append(many, database.TalkChoice{Text: string(rune('a' + i)), Next: 9})
	}
	tw.h.SetTalkTrees(func() (map[string]map[int]database.TalkNode, error) {
		return map[string]map[int]database.TalkNode{"talker npc": {1: {Says: []string{"Pick."}, Choices: many}}}, nil
	})
	tw.talk(n)
	h := heardTalk(tw.s)
	if len(h.answers) != 6 || h.answers[5] != "Exit" {
		t.Fatalf("a PS2's box: %v", h.answers)
	}
	tw.pick(h.box, 0)
	if got := opcodes(tw.s); len(got) != 0 {
		t.Errorf("a missing node: %v", got)
	}
}

// Quest 100792's offer (work board 883): three questions, the second and third with their own refusals and
// the first none; a decline of one line is every question's, as before.
func TestDeclinePerQuestion(t *testing.T) {
	qu := &database.Quest{ID: 100792, Accept: "I'm just checking to see if you need my help.\nSure.\nOk, I'll be back as quick as I can!",
		Decline: "\nNo, I already carry more than I should physically be able to.\nI'm not running down there again."}
	labels := func(k int) []string {
		var out []string
		for _, c := range offerChoices(qu, k, nil) {
			out = append(out, c.label)
		}
		return out
	}
	if got := labels(0); !slices.Equal(got, []string{"I'm just checking to see if you need my help."}) {
		t.Errorf("question 1: %v", got)
	}
	if got := labels(1); !slices.Equal(got, []string{"Sure.", "No, I already carry more than I should physically be able to."}) {
		t.Errorf("question 2: %v", got)
	}
	if got := labels(2); !slices.Equal(got, []string{"Ok, I'll be back as quick as I can!", "I'm not running down there again."}) {
		t.Errorf("question 3: %v", got)
	}
	qu.Decline = "No thanks."
	for k := range 3 {
		if got := labels(k); len(got) != 2 || got[1] != "No thanks." {
			t.Errorf("one decline, question %d: %v", k+1, got)
		}
	}
}
