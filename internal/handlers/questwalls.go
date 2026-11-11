package handlers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/entity"
)

// The walls a character's quests let it through and the items a creature's
// aggro looks for, told to the World each second (entity/questwalls.go: the
// World's lock may not wait on the engine's, so the engine says what it
// knows). Only what changed is told.

// questState is what the engine last told the World of a character.
type questState struct{ sent string }

// syncQuestState tells the World, for each character in the world, the walls
// it passes and the watched items it holds, near a creature that looks for one.
func (q *QuestEngine) syncQuestState() {
	if q.gw == nil || q.sessionOf == nil || q.playerOf == nil {
		return
	}
	walls := q.gw.QuestWalls()
	items := q.gw.WatchedItems()
	if len(walls) == 0 && len(items) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		sess := q.sessionOf(id)
		if sess == nil {
			if c.qstate.sent != "" {
				q.gw.ForgetQuestState(id)
				c.qstate.sent = ""
			}
			continue
		}
		p := q.playerOf(sess)
		if p == nil {
			continue
		}
		passes := map[entity.EntityID]bool{}
		var keys []string
		for _, wl := range walls {
			if cq := c.quests[wl.QuestID]; cq != nil && (cq.Done > 0 || (cq.Active() && cq.Step > wl.Step)) {
				passes[entity.EntityID(wl.NPCID)] = true
				keys = append(keys, fmt.Sprintf("w%d", wl.NPCID))
			}
		}
		var holds map[int]bool
		if len(items) > 0 && q.gw.NearHoldingRule(p) {
			want := map[int]bool{}
			for _, it := range items {
				want[it] = true
			}
			for _, r := range q.shownItems(p) {
				if want[r.Pattern.PatternID] {
					if holds == nil {
						holds = map[int]bool{}
					}
					if !holds[r.Pattern.PatternID] {
						keys = append(keys, fmt.Sprintf("i%d", r.Pattern.PatternID))
					}
					holds[r.Pattern.PatternID] = true
				}
			}
		}
		sort.Strings(keys)
		sent := "+" + strings.Join(keys, ",")
		if sent == c.qstate.sent {
			continue
		}
		c.qstate.sent = sent
		q.gw.SetQuestState(id, passes, holds)
	}
}
