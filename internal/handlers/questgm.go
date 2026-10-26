package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// What the GM panel (internal/gmweb, ops_quests.go) does with the quests: a
// character's quests read and set with its journal kept in step, the quests
// read again after an edit, a character's read again after its rows changed.

// QuestState is a character's quest as the panel shows it.
type QuestState struct {
	QuestID int    `json:"questid"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Step    int    `json:"step"`            // the step to do, 0 when finished
	Kind    string `json:"kind,omitempty"`  // that step's kind
	Steps   int    `json:"steps"`           // how many steps the quest has
	Count   int    `json:"count,omitempty"` // creatures killed so far on a kill step, items taken on a loot step
	Need    int    `json:"need,omitempty"`  // how many the kill or loot step wants
	Done    int    `json:"done"`            // times finished
	Journal string `json:"journal,omitempty"`
	Line    int    `json:"line"` // its place in the journal, -1 for none
	Updated string `json:"updated,omitempty"`
}

// CharacterQuests is a character's quests, under way first in journal order,
// then finished ones by quest.
func (q *QuestEngine) CharacterQuests(serverID uint32) []QuestState {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.charLocked(serverID)
	var out []QuestState
	seen := map[int]bool{}
	for i, cq := range c.active() {
		out = append(out, q.stateLocked(cq, i))
		seen[cq.QuestID] = true
	}
	var rest []int
	for id := range c.quests {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sortInts(rest)
	for _, id := range rest {
		out = append(out, q.stateLocked(c.quests[id], -1))
	}
	return out
}

func (q *QuestEngine) stateLocked(cq *database.CharQuest, line int) QuestState {
	s := QuestState{QuestID: cq.QuestID, Step: cq.Step, Count: cq.Count, Done: cq.Done, Line: line, Updated: cq.Updated}
	if qu := q.cat.byID[cq.QuestID]; qu != nil {
		s.Name, s.Enabled, s.Steps = qu.Name, qu.Enabled, len(qu.Steps)
		if st := qu.Step(cq.Step); st != nil {
			s.Kind = st.Kind
			if st.Kind == database.StepKill {
				s.Need = max(st.Count, 1)
			}
			if _, need, ok := st.LootItem(); ok && st.Kind == database.StepLoot {
				s.Need = need
			}
		}
	} else {
		s.Name = fmt.Sprintf("(quest %d is gone)", cq.QuestID)
	}
	if cq.Active() {
		s.Journal = q.lineLocked(cq)
	}
	return s
}

func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// The GM's changes to a character's quest.
const (
	QuestGive     = "give"     // under way at a step (its first when none is named)
	QuestSetStep  = "step"     // the step to do set
	QuestComplete = "complete" // finished, no rewards
	QuestGiveUp   = "giveup"   // ended unfinished, as the player's own delete
	QuestReset    = "reset"    // forgotten altogether: it may be taken again
)

// QuestActions is every change, in the panel's order.
var QuestActions = []string{QuestGive, QuestSetStep, QuestComplete, QuestGiveUp, QuestReset}

// SetCharacterQuest makes a GM's change to a character's quest. A character
// in the world sees its journal change at once (and "You have received a
// quest!" for a quest given); no reward is given. It says what happened.
func (q *QuestEngine) SetCharacterQuest(serverID uint32, questID int, action string, step int) (string, error) {
	if q == nil {
		return "", errors.New("the quests are not running")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[questID]
	c := q.charLocked(serverID)
	cq := c.quests[questID]
	var sess *session.Session
	if q.sessionOf != nil {
		sess = q.sessionOf(serverID)
	}
	// the player in the world, for the quest's events (questevents.go)
	var p *entity.Player
	if sess != nil && q.playerOf != nil {
		p = q.playerOf(sess)
	}
	where := "at the character's next login"
	if sess != nil {
		where = "in their journal now"
	}
	switch action {
	case QuestGive:
		if qu == nil {
			return "", fmt.Errorf("no quest %d", questID)
		}
		if why := q.startLocked(sess, p, c, qu, step, true); why != "" {
			return "", errors.New(why)
		}
		return fmt.Sprintf("%q is under way at step %d, %s", qu.Name, c.quests[questID].Step, where), nil
	case QuestSetStep:
		if qu == nil {
			return "", fmt.Errorf("no quest %d", questID)
		}
		st := qu.Step(step)
		if st == nil {
			return "", fmt.Errorf("%q has no step %d", qu.Name, step)
		}
		if cq == nil || !cq.Active() {
			if why := q.startLocked(sess, p, c, qu, step, true); why != "" {
				return "", errors.New(why)
			}
			return fmt.Sprintf("%q given at step %d, %s", qu.Name, step, where), nil
		}
		idx := c.index(questID)
		old := q.lineLocked(cq)
		q.clearSpawnsLocked(serverID, questID, cq.Step)
		cq.Step, cq.Count = st.Step, 0
		if text := q.lineLocked(cq); text != old {
			q.dropLineLocked(sess, c, idx)
			cq.Slot = c.nextSlot()
			q.addLineLocked(sess, c, text)
		}
		q.saveLocked(cq)
		q.stepStartedLocked(sess, p, c, qu, cq, st)
		return fmt.Sprintf("%q is at step %d, %s", qu.Name, step, where), nil
	case QuestComplete:
		if cq == nil {
			cq = &database.CharQuest{ServerID: serverID, QuestID: questID}
			c.quests[questID] = cq
		}
		if cq.Active() {
			q.dropLineLocked(sess, c, c.index(questID))
		}
		q.clearSpawnsLocked(serverID, questID, 0)
		cq.Step, cq.Count = 0, 0
		cq.Done++
		q.saveLocked(cq)
		return fmt.Sprintf("quest %d is finished (%d time(s)), no reward given", questID, cq.Done), nil
	case QuestGiveUp:
		if cq == nil || !cq.Active() {
			return "", fmt.Errorf("quest %d is not under way", questID)
		}
		idx := c.index(questID)
		q.giveUpLocked(cq)
		q.dropLineLocked(sess, c, idx)
		return fmt.Sprintf("quest %d given up, %s", questID, where), nil
	case QuestReset:
		if cq == nil {
			return fmt.Sprintf("the character had nothing of quest %d", questID), nil
		}
		if cq.Active() {
			q.dropLineLocked(sess, c, c.index(questID))
		}
		q.clearSpawnsLocked(serverID, questID, 0)
		if err := q.store.DeleteCharQuest(serverID, questID); err != nil {
			return "", err
		}
		delete(c.quests, questID)
		return fmt.Sprintf("quest %d forgotten: it may be taken again", questID), nil
	}
	return "", fmt.Errorf("no change %q (%s)", action, strings.Join(QuestActions, ", "))
}

// ReloadCharacter reads a character's quests again (its rows were changed
// outside the engine: a panel row edit, a revert) and, in the world, redraws
// its journal. It says when the character sees it.
func (q *QuestEngine) ReloadCharacter(serverID uint32) string {
	if q == nil {
		return "at the next start"
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	old := q.chars[serverID]
	delete(q.chars, serverID)
	c := q.charLocked(serverID)
	if old != nil {
		c.token, c.shown = old.token, old.shown
	}
	var sess *session.Session
	if q.sessionOf != nil {
		sess = q.sessionOf(serverID)
	}
	if sess == nil || old == nil {
		return "at the character's next login"
	}
	if q.redrawLocked(sess, c) {
		return "at once: the journal redrawn where they stand"
	}
	return "at once"
}

// ReloadQuests reads the quests again and redraws the journals in the world
// whose lines changed with them. It says so.
func (q *QuestEngine) ReloadQuests() string {
	if q == nil {
		return "at the next start"
	}
	if err := q.Reload(); err != nil {
		return "not applied: " + err.Error()
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	redrawn := 0
	if q.sessionOf != nil {
		for id, c := range q.chars {
			if len(c.shown) == 0 && len(c.active()) == 0 {
				continue
			}
			if sess := q.sessionOf(id); sess != nil && q.redrawLocked(sess, c) {
				redrawn++
			}
		}
	}
	if redrawn > 0 {
		return fmt.Sprintf("at once: the next talk, kill or step uses it, and %d journal(s) were redrawn", redrawn)
	}
	return "at once: the next talk, kill or step uses it"
}

// GiveToSession gives a quest to the character a session plays (a GM trying
// their own): as SetCharacterQuest's give.
func (q *QuestEngine) GiveToSession(sess *session.Session, questID int) (string, error) {
	if q == nil || q.playerOf == nil {
		return "", errors.New("the quests are not running")
	}
	p := q.playerOf(sess)
	if p == nil {
		return "", errors.New("that session has no character in the world")
	}
	msg, err := q.SetCharacterQuest(p.CharServerID, questID, QuestGive, 0)
	if err == nil {
		log.Printf("Quests: %s tries quest %d", p.Name, questID)
	}
	return msg, err
}

// QuestSummary is a quest as the panel lists it.
type QuestSummary struct {
	ID       int    `json:"questid"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Giver    string `json:"giver"`
	MinLevel int    `json:"minlevel"`
	MaxLevel int    `json:"maxlevel"`
	Steps    int    `json:"steps"`
	Source   string `json:"source,omitempty"`
	UnderWay int    `json:"underway"` // characters with it under way
	Finished int    `json:"finished"` // characters who finished it
}
