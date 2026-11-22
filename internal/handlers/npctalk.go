package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// npctalk.go — NPC talk trees (work board 883: live NPCs held branching conversations outside any quest,
// filmed at Forkwatch). An NPC whose name has an npctalk node 1 (database/npctalk.go) plays it where its
// talk would have said an npcdialog line: nothing before it answers (a quest step or offer, a shop, a
// route, a bind, a sign post). A node's lines go in dialogue boxes, the speaker's name before each, and
// with answers its last line is a question (0x0034, under talkBoxMark) of those answers and Exit; an
// answer plays the node it names, 0 or Exit closes. A conversation is held in memory only: a keep restart
// drops an open one (its box's answer then does nothing, as an old box's), and the next talk starts again.

// talkBoxMark marks a talk tree's questions' ids.
const talkBoxMark = 0x69000000

// talkExit is the answer the server adds last to every talk question.
const talkExit = "Exit"

// talkAsks is the talk boxes asked, by the character's serverid, under boardMu.
type talkAsks struct {
	seq   uint32
	asked map[uint32]talkAsk
}

// talkAsk is a talk box asked: its id, whose tree (the name's key and the speaker's name as shown) and the
// node each answer plays, in the box's order (Exit's 0 last).
type talkAsk struct {
	box     uint32
	tree    string
	speaker string
	spawn   entity.EntityID
	next    []int
}

// SetTalkTrees gives the handler npctalk's reader (database.NPCTalk).
func (h *InteractHandler) SetTalkTrees(f func() (map[string]map[int]database.TalkNode, error)) {
	h.talkTrees = f
}

// talkTreeOf is the npctalk tree of the NPC called name, nil when it has no node 1.
func (h *InteractHandler) talkTreeOf(name string) map[int]database.TalkNode {
	if h.talkTrees == nil {
		return nil
	}
	trees, err := h.talkTrees()
	if err != nil {
		log.Printf("Interact: npctalk: %v", err)
		return nil
	}
	tree := trees[database.NPCClassKey(name)]
	if _, ok := tree[1]; !ok {
		return nil
	}
	return tree
}

// talkTree is a talk to npc: its tree's node 1 when it has one, else false.
func (h *InteractHandler) talkTree(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead {
		return false
	}
	tree := h.talkTreeOf(npc.Name)
	if tree == nil {
		return false
	}
	h.playTalkNode(sess, p, database.NPCClassKey(npc.Name), npc.Name, npc.ServerID, tree, 1)
	return true
}

// playTalkNode says node n of a tree to p: its lines a box each, and with answers its last line asked with
// them and Exit. A node not in the tree closes the conversation.
func (h *InteractHandler) playTalkNode(sess *session.Session, p *entity.Player, key, speaker string, spawn entity.EntityID, tree map[int]database.TalkNode, n int) {
	node, ok := tree[n]
	if !ok {
		log.Printf("Interact: npctalk %q has no node %d: the conversation closes", key, n)
		return
	}
	forgetDialogs(p)
	lines := node.Says
	question := ""
	if len(node.Choices) > 0 && len(lines) > 0 {
		question, lines = lines[len(lines)-1], lines[:len(lines)-1]
	}
	for _, l := range lines {
		for _, part := range splitSay(speaker+": "+l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
	if len(node.Choices) == 0 {
		return
	}
	room := dialogRoom(sess) - 1 // Exit's place
	var labels []string
	var next []int
	for _, c := range node.Choices {
		if len(labels) >= room {
			log.Printf("Interact: npctalk %q node %d has more answers than a box holds: the rest left out", key, n)
			break
		}
		labels = append(labels, clipRunes(strings.TrimSpace(c.Text), questAnswerChars))
		next = append(next, c.Next)
	}
	labels, next = append(labels, talkExit), append(next, 0)
	h.boardMu.Lock()
	h.talks.seq++
	id := talkBoxMark | (h.talks.seq & 0xFFFFFF)
	if h.talks.asked == nil {
		h.talks.asked = map[uint32]talkAsk{}
	}
	h.talks.asked[p.CharServerID] = talkAsk{box: id, tree: key, speaker: speaker, spawn: spawn, next: next}
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, clipRunes(question, questAskChars), labels)
}

// answerTalk is HandleDialogueOption's question for a talk box: the answer plays its node (the tree read
// again: a GM's edit counts), Exit or 0 closes. It reports whether the box was a talk's (an old one does
// nothing).
func (h *InteractHandler) answerTalk(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != talkBoxMark {
		return false
	}
	h.boardMu.Lock()
	asked, ok := h.talks.asked[p.CharServerID]
	if ok && asked.box == box {
		delete(h.talks.asked, p.CharServerID)
	}
	h.boardMu.Unlock()
	if !ok || asked.box != box || int(option) >= len(asked.next) || asked.next[option] <= 0 {
		return true
	}
	tree := h.talkTreeOf(asked.speaker)
	if tree == nil {
		return true
	}
	h.playTalkNode(sess, p, asked.tree, asked.speaker, asked.spawn, tree, asked.next[option])
	return true
}
