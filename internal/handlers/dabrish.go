package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// dabrish.go — Dabrish, who sets a player's level and adds to its mastery
// points (the GM Jibrish's petition 1266, 2026-11-08: "When talked to Dabrish
// should let players set their level. They can pick in increments of 5. Level
// 5 to level 60. They can change their level at will. He should also have an
// option for CM's to set them between 0 and 1000 CM's, but it will just add
// cm's on their existing total up to 1000. It wont remove CM's."; petition 1461,
// 2026-11-10: "Add another CM option on Dabrish to take CM's up to 1250, working
// identical to all the other options."). A talk asks which range (levels 5 to 30
// or 35 to 60, mastery points 100 to 500, 600 to 1000 or 1100 to 1250 in all: a
// box holds six answers, maxDialogOptions), then the one. A
// level is set as #setlevel sets it, up or down, as often as asked
// (ChatHandler.applyLevel: the level alone); a mastery total is made up to
// with unspent points, spent and unspent counted together, and never taken
// down (ChatHandler.giveMastery, as #grantcm's). Any NPC named Dabrish is him;
// he stands on the Heirloom Test Server alone. His questions go under box mark
// 0x5C000000 (beside the quests' 0x51 and the others to Mr. Moogie's 0x5B). The
// first box's sixth answer exchanges a path epic for the other path's
// (dabrishepic.go; the owner, 2026-11-13). The words are ours.

// DabrishName is his name: an NPC of it is Dabrish.
const DabrishName = "Dabrish"

// dabrishBoxMark marks his question's id.
const dabrishBoxMark = 0x5C000000

// His questions and answers, and what he says.
const (
	dabrishQuestion = "Dabrish: 'I set levels from 5 to 60, up or down as often as you like, and add mastery points up to 1250 in all.'" // 127 at most (questAskChars)
	dabrishLevelAsk = "Dabrish: 'Which level?'"
	dabrishCMAsk    = "Dabrish: 'How many mastery points in all, spent and unspent? I only ever add.'"
	dabrishLevelSet = "There: you are level %d."
	dabrishCMHave   = "You hold %d mastery points already, spent and unspent. I only ever add, never take away."
	dabrishCMGiven  = "There: %d mastery points more, %d in all, %d of them unspent."
	dabrishFailed   = "Something went wrong with my ledger. Try again."
)

// The first box's answers: each range's first value, its step and its count.
var dabrishRanges = []struct {
	label             string
	mastery           bool
	first, step, many int
}{
	{"A level from 5 to 30", false, 5, 5, 6},
	{"A level from 35 to 60", false, 35, 5, 6},
	{"Mastery points: 100 to 500 in all", true, 100, 100, 5},
	{"Mastery points: 600 to 1000 in all", true, 600, 100, 5},
	{"Mastery points: 1100 to 1250 in all", true, 1100, 50, 4},
}

// dabrishAsks is the boxes he has asked, by the character's serverid.
type dabrishAsks struct {
	seq   uint32
	asked map[uint32]dabrishAsk
}

// dabrishAsk is a question: its box, which range was picked (-1: the first
// box) and which Dabrish asked it.
type dabrishAsk struct {
	box uint32
	rng int
	npc entity.EntityID
	// swaps are the epic exchanges a box of the exchange's offers
	// (dabrishepic.go: rng dabrishEpicWhich or dabrishEpicConfirm)
	swaps []epicSwap
}

// dabrishAskLocked asks p a question under his mark. Called with boardMu held.
func (h *InteractHandler) dabrishAskLocked(p *entity.Player, rng int, npc entity.EntityID) uint32 {
	h.dabrish.seq++
	id := dabrishBoxMark | (h.dabrish.seq & 0xFFFFFF)
	if h.dabrish.asked == nil {
		h.dabrish.asked = map[uint32]dabrishAsk{}
	}
	h.dabrish.asked[p.CharServerID] = dabrishAsk{box: id, rng: rng, npc: npc}
	return id
}

// dabrishSay is his line in a dialogue box.
func dabrishSay(sess *session.Session, line string) {
	for _, part := range splitSay(DabrishName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// dabrishTalk is a talk to npc: his question when npc is Dabrish, else false.
func (h *InteractHandler) dabrishTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !strings.EqualFold(strings.TrimSpace(npc.Name), DabrishName) {
		return false
	}
	if h.dabrishLevel == nil || h.dabrishMastery == nil || h.gameWorld == nil {
		dabrishSay(sess, "My ledger is closed today.")
		return true
	}
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, -1, npc.ServerID)
	h.boardMu.Unlock()
	answers := make([]string, len(dabrishRanges), len(dabrishRanges)+1)
	for i, r := range dabrishRanges {
		answers[i] = r.label
	}
	if h.dabrishEpic != nil {
		answers = append(answers, dabrishEpicLabel) // dabrishepic.go
	}
	SendQuestPopupMulti(sess, id, dabrishQuestion, answers)
	return true
}

// answerDabrish is HandleDialogueOption's question for his box: a range asks
// for its value, a value is done. It reports whether the box was his (an old
// one, or a close, does nothing).
func (h *InteractHandler) answerDabrish(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != dabrishBoxMark {
		return false
	}
	if option == 255 {
		return true // a close
	}
	h.boardMu.Lock()
	asked, ok := h.dabrish.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(h.dabrish.asked, p.CharServerID)
	}
	h.boardMu.Unlock()
	if !ok || asked.box != box || h.dabrishLevel == nil || h.dabrishMastery == nil || h.gameWorld == nil {
		return true
	}
	n := int(option)
	if asked.rng >= dabrishEpicWhich {
		if h.dabrishEpic != nil {
			h.answerDabrishEpic(sess, p, asked, n)
		}
		return true
	}
	if asked.rng < 0 {
		if n == len(dabrishRanges) && h.dabrishEpic != nil {
			h.dabrishEpicStart(sess, p, asked.npc)
			return true
		}
		if n >= len(dabrishRanges) {
			return true
		}
		r := dabrishRanges[n]
		answers := make([]string, r.many)
		for i := range answers {
			v := r.first + i*r.step
			if r.mastery {
				answers[i] = fmt.Sprintf("%d in all", v)
			} else {
				answers[i] = fmt.Sprintf("Level %d", v)
			}
		}
		ask := dabrishLevelAsk
		if r.mastery {
			ask = dabrishCMAsk
		}
		h.boardMu.Lock()
		id := h.dabrishAskLocked(p, n, asked.npc)
		h.boardMu.Unlock()
		SendQuestPopupMulti(sess, id, ask, answers)
		return true
	}
	if asked.rng >= len(dabrishRanges) {
		return true
	}
	r := dabrishRanges[asked.rng]
	if n >= r.many {
		return true
	}
	v := r.first + n*r.step
	if !r.mastery {
		level, err := h.dabrishLevel(sess, p, v)
		if err != nil {
			dabrishSay(sess, dabrishFailed)
			return true
		}
		log.Printf("Dabrish: %s set to level %d", p.Name, level)
		dabrishSay(sess, fmt.Sprintf(dabrishLevelSet, level))
		return true
	}
	m, ok := h.gameWorld.MasteryOf(p.ServerID)
	if !ok {
		return true
	}
	if have := m.Points + m.Spent; have >= v {
		dabrishSay(sess, fmt.Sprintf(dabrishCMHave, have))
		return true
	}
	give, now, ok, err := h.dabrishMastery(sess, v-(m.Points+m.Spent))
	if !ok || err != nil {
		dabrishSay(sess, dabrishFailed)
		return true
	}
	log.Printf("Dabrish: %s given %d mastery points (%d in all, %d unspent)", p.Name, give, now.Points+now.Spent, now.Points)
	dabrishSay(sess, fmt.Sprintf(dabrishCMGiven, give, now.Points+now.Spent, now.Points))
	return true
}
