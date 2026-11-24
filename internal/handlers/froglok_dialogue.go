package handlers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
)

//go:embed froglok_refugee_dialogue.json
var froglokRefugeeScript []byte

type froglokQuestLine struct {
	Quest   int    `json:"quest"`
	Step    int    `json:"step"`
	Speaker string `json:"speaker"`
	Before  string `json:"before"` // prefix of the original spoken line that follows this one
	Line    string `json:"line"`
}

// EnableFroglokRefugeeDialogue is called only when the world opts in. An unset
// gm.shard_id denotes Heirloom, as in the GM panel's ownShard.
func (q *QuestEngine) EnableFroglokRefugeeDialogue(shard string) error {
	if id := strings.ToLower(strings.TrimSpace(shard)); id != "" && id != "heirloom" {
		return fmt.Errorf("not available on shard %q", shard)
	}
	var script []froglokQuestLine
	if err := json.Unmarshal(froglokRefugeeScript, &script); err != nil {
		return err
	}
	lines := make(map[[2]int]froglokQuestLine, len(script))
	for _, line := range script {
		key := [2]int{line.Quest, line.Step}
		_, duplicate := lines[key]
		if duplicate || line.Quest <= 0 || line.Step < 0 || line.Speaker == "" || line.Before == "" ||
			strings.TrimSpace(line.Line) == "" || strings.ContainsAny(line.Line+line.Before, "\r\n") {
			return fmt.Errorf("invalid Froglok line for quest %d step %d", line.Quest, line.Step)
		}
		lines[key] = line
	}
	q.mu.Lock()
	q.froglokLines = lines
	q.mu.Unlock()
	log.Printf("Quests: enabled %d Froglok refugee lines on Heirloom", len(lines))
	return nil
}

// froglokWords personalizes a single conversation without editing the shared
// quest. A changed speaker or missing original phrase leaves the scene as written.
func (q *QuestEngine) froglokWords(p *entity.Player, quest, step int, speaker, words string) string {
	if p == nil || p.Race != int(RaceFroglok) || q.froglokLines == nil {
		return words
	}
	line, ok := q.froglokLines[[2]int{quest, step}]
	if !ok || line.Speaker != speaker {
		return words
	}
	// Match at the beginning of a spoken line, including the first one.
	at := strings.Index("\n"+words, "\n"+line.Before)
	if at < 0 {
		return words
	}
	return words[:at] + line.Line + "\n" + words[at:]
}
