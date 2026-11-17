package handlers

// npctour.go — a tour's host's talk (entity/npctour.go, table npctours; the owner,
// 2026-11-17: Santa Claus's sleigh ride over Freeport, "he will initiate once a
// player talks to him and chooses to start"). Idle, the host asks in a box under
// tourBoxMark (start it, or not now); counting down, whether to climb aboard;
// out or resting, he says how long in a line. The tour's stand-in (the Holiday
// Elf) says where he is while he is out, in chat, before its own talk.

import (
	"sync"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// tourBoxMark marks a host's question's id (beside the boards' plates' 0x6F).
const tourBoxMark = 0x6E000000

// tourAsks is the boxes the hosts asked, by the character's serverid.
type tourAsks struct {
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]tourAsk
}

// tourAsk is a box a host asked: its id and the tour.
type tourAsk struct {
	box  uint32
	tour string
}

var tours tourAsks

// tourTalk is a talk to npc: its question or its line when npc hosts a tour,
// else false.
func (h *InteractHandler) tourTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || h.gameWorld == nil {
		return false
	}
	t, ok := h.gameWorld.TalkTour(p, npc, time.Now())
	if !ok {
		return false
	}
	if t.Say != "" || len(t.Answers) == 0 {
		SendClientMessage(sess, npc.Name+": "+t.Say)
		return true
	}
	forgetDialogs(p)
	tours.mu.Lock()
	tours.seq++
	id := tourBoxMark | (tours.seq & 0xFFFFFF)
	if tours.asked == nil {
		tours.asked = map[uint32]tourAsk{}
	}
	tours.asked[p.CharServerID] = tourAsk{box: id, tour: t.Tour}
	tours.mu.Unlock()
	answers := make([]string, 0, len(t.Answers))
	for _, a := range t.Answers {
		answers = append(answers, clipRunes(a, questAnswerChars))
	}
	SendQuestPopupMulti(sess, id, clipRunes(npc.Name+": "+t.Ask, questAskChars), answers)
	return true
}

// answerTour is HandleDialogueOption's question for a host's box: the first
// answer starts the tour or climbs aboard. It reports whether the box was a
// host's (an old one, or a close, does nothing).
func (h *InteractHandler) answerTour(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != tourBoxMark {
		return false
	}
	if option == 255 {
		return true // a close: a PS2 closes one box before the pick in the next
	}
	tours.mu.Lock()
	asked, ok := tours.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(tours.asked, p.CharServerID)
	}
	tours.mu.Unlock()
	if !ok || asked.box != box || h.gameWorld == nil {
		return true
	}
	if line := h.gameWorld.AnswerTour(sess, p, asked.tour, option == 0, time.Now()); line != "" {
		SendClientMessage(sess, h.gameWorld.TourHost(asked.tour)+": "+line)
	}
	return true
}

// tourStandLine is what npc says first while a tour's host is out (the Holiday
// Elf: "Santa's out on his sleigh"), in chat; its own talk goes on after it.
func (h *InteractHandler) tourStandLine(sess *session.Session, npc *entity.NPC) {
	if npc == nil || h.gameWorld == nil {
		return
	}
	if line, ok := h.gameWorld.TourStandLine(npc, time.Now()); ok {
		SendClientMessage(sess, npc.Name+": "+line)
	}
}
