package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// npctalk.go — NPC talk trees (work board 883): live NPCs held branching conversations outside any quest
// (an NPC's lines, a question, an answer leading on to more), where npcdialog says one line. A tree is an
// NPC's rows, keyed by its name as npcclass keys it (NPCClassKey) and a node number; a talk that nothing
// before it answers (no quest step or offer, shop, route, bind or sign post) plays node 1 in place of an
// npcdialog line (handlers/npctalk.go):
//   - says: the NPC's lines, one a dialogue box (the speaker's name before each), separated by \n; with
//     choices, the last line is the question's own words;
//   - choices: a JSON list of {"text": the answer, "next": the node it plays (0 closes)}; Exit is added
//     last by the server. An empty list ends the conversation after says.
//
// Read at most npcTalkFor old, as npcdialog is: a row a tool writes is played within it, a GM's edit at once
// (InvalidateNPCTalk).
const npcTalkSchema = `
CREATE TABLE IF NOT EXISTS npctalk (
	npc     TEXT NOT NULL,
	node    INTEGER NOT NULL,
	says    TEXT NOT NULL DEFAULT '',
	choices TEXT NOT NULL DEFAULT '[]',
	source  TEXT NOT NULL DEFAULT 'gm',
	note    TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (npc, node)
);`

func (db *SQLiteDB) ensureNPCTalk() error {
	if _, err := db.Exec(npcTalkSchema); err != nil {
		return fmt.Errorf("npctalk: %w", err)
	}
	return nil
}

// TalkChoice is one answer of a talk node's question.
type TalkChoice struct {
	Text string `json:"text"`
	Next int    `json:"next"` // the node it plays; 0 closes the conversation
}

// TalkNode is one row of npctalk.
type TalkNode struct {
	NPC     string // the name's key
	Node    int
	Says    []string // its lines, empty ones left out
	Choices []TalkChoice
}

// The bounds of a talk node's words: what CLIENT's question box shows (an answer 63 characters, the question
// 127), and the answers a box holds beside Exit on a PS2 (six in all).
const (
	TalkAnswerChars   = 63
	TalkQuestionChars = 127
	TalkMostChoices   = 5
)

// ParseTalkChoices reads a choices column: "" or "[]" is none.
func ParseTalkChoices(s string) ([]TalkChoice, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []TalkChoice
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("choices is not a JSON list of {\"text\", \"next\"}: %v", err)
	}
	return out, nil
}

// TalkNodeProblems is what is wrong with a node's says and choices: choices that do not read, an answer
// empty or over TalkAnswerChars, a next below 0, more than TalkMostChoices answers, a question over
// TalkQuestionChars, a | anywhere (a run of them kills a PS2's TextBox).
func TalkNodeProblems(says, choices string) []string {
	var out []string
	cs, err := ParseTalkChoices(choices)
	if err != nil {
		return []string{err.Error()}
	}
	if len(cs) > TalkMostChoices {
		out = append(out, fmt.Sprintf("%d answers: %d at most beside Exit", len(cs), TalkMostChoices))
	}
	for i, c := range cs {
		switch n := utf8.RuneCountInString(strings.TrimSpace(c.Text)); {
		case n == 0:
			out = append(out, fmt.Sprintf("answer %d has no text", i+1))
		case n > TalkAnswerChars:
			out = append(out, fmt.Sprintf("answer %d is %d characters, %d at most", i+1, n, TalkAnswerChars))
		}
		if c.Next < 0 {
			out = append(out, fmt.Sprintf("answer %d's next is below 0 (0 closes)", i+1))
		}
		if strings.Contains(c.Text, "|") {
			out = append(out, fmt.Sprintf("answer %d holds a | (a run of them kills a PS2)", i+1))
		}
	}
	lines := talkLines(says)
	if len(cs) > 0 && len(lines) > 0 {
		if n := utf8.RuneCountInString(lines[len(lines)-1]); n > TalkQuestionChars {
			out = append(out, fmt.Sprintf("the question (says' last line) is %d characters, %d at most", n, TalkQuestionChars))
		}
	}
	if strings.Contains(says, "|") {
		out = append(out, "says holds a | (a run of them kills a PS2)")
	}
	return out
}

func talkLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// npcTalkFor is how long a read of npctalk is kept.
const npcTalkFor = 30 * time.Second

var npcTalkCache struct {
	sync.Mutex
	trees map[string]map[int]TalkNode
	at    time.Time
	db    *SQLiteDB
}

// InvalidateNPCTalk makes the next NPCTalk read the table again.
func InvalidateNPCTalk() {
	npcTalkCache.Lock()
	npcTalkCache.at = time.Time{}
	npcTalkCache.Unlock()
}

// NPCTalk is every talk tree, by the NPC name's key then node, read at most every npcTalkFor. A row whose
// choices do not read is left out (the panel refuses such a row). Never call it
// holding the World's lock or another query's rows: it may query (the database has one connection).
func (db *SQLiteDB) NPCTalk() (map[string]map[int]TalkNode, error) {
	npcTalkCache.Lock()
	defer npcTalkCache.Unlock()
	if npcTalkCache.db == db && time.Since(npcTalkCache.at) < npcTalkFor {
		return npcTalkCache.trees, nil
	}
	rows, err := db.Query(`SELECT npc, node, says, choices FROM npctalk ORDER BY npc, node`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[int]TalkNode{}
	for rows.Next() {
		var n TalkNode
		var says, choices string
		if err := rows.Scan(&n.NPC, &n.Node, &says, &choices); err != nil {
			return nil, err
		}
		cs, err := ParseTalkChoices(choices)
		if err != nil {
			continue
		}
		n.NPC, n.Says, n.Choices = NPCClassKey(n.NPC), talkLines(says), cs
		if out[n.NPC] == nil {
			out[n.NPC] = map[int]TalkNode{}
		}
		out[n.NPC][n.Node] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	npcTalkCache.trees, npcTalkCache.at, npcTalkCache.db = out, time.Now(), db
	return out, nil
}
