package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The walls a character's quests let it through and the items a creature's
// aggro looks for, told to the World each second (entity/questwalls.go: the
// World's lock may not wait on the engine's, so the engine says what it
// knows). Only what changed is told.

// questState is what the engine last told the World of a character, and the
// walls of an item whose line it was told as it came up against them (told
// once until it goes wallForgetReach from the wall).
type questState struct {
	sent string
	told map[int]bool
}

// wallTellReach is how near a wall of an item's middle a character it stops
// stands to be told its line (a walk stops 0.6 short of the box's face, half
// of 5 times its size from the middle: 5.6 at size 2); wallForgetReach how far
// it goes for the line to be told again.
const (
	wallTellReach   float32 = 8
	wallForgetReach float32 = 25
)

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
	itemWalls := false
	for _, wl := range walls {
		itemWalls = itemWalls || wl.Item > 0
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
			if wl.QuestID == 0 {
				continue // a wall of an item alone (below)
			}
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
		// a wall of an item: passed by whoever carries or wears it
		for _, wl := range walls {
			if wl.Item > 0 && holds[wl.Item] && !passes[entity.EntityID(wl.NPCID)] {
				passes[entity.EntityID(wl.NPCID)] = true
				keys = append(keys, fmt.Sprintf("w%d", wl.NPCID))
			}
		}
		sort.Strings(keys)
		sent := "+" + strings.Join(keys, ",")
		if sent != c.qstate.sent {
			c.qstate.sent = sent
			q.gw.SetQuestState(id, passes, holds)
			log.Printf("Quests: %s passes %d walls and holds %d watched items (%s)", p.Name, len(passes), len(holds), sent)
		}
		if itemWalls {
			q.tellWallsLocked(c, sess, p)
		}
	}
}

// tellWallsLocked tells a character the line of each wall of an item it has
// come up against and does not pass, once, until it goes wallForgetReach off.
func (q *QuestEngine) tellWallsLocked(c *questChar, sess *session.Session, p *entity.Player) {
	stops := q.gw.ItemWallsStopping(p, wallForgetReach)
	near := map[int]bool{}
	for _, s := range stops {
		near[s.NPCID] = true
		if s.Dist > wallTellReach || c.qstate.told[s.NPCID] {
			continue
		}
		if c.qstate.told == nil {
			c.qstate.told = map[int]bool{}
		}
		c.qstate.told[s.NPCID] = true
		if s.Text != "" {
			SendErrorMessage(sess, s.Text)
			log.Printf("Quests: %s stopped at wall %d: %q", p.Name, s.NPCID, s.Text)
		}
	}
	for id := range c.qstate.told {
		if !near[id] {
			delete(c.qstate.told, id)
		}
	}
}
