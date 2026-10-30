package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A quest's items must fit the pack (the user, 2026-10-30: "giver waits for
// room"; until then GiveItemNow gave what fitted and the rest was said lost,
// an epic's weapon and rose too). Before a step is done, everything doing it
// puts in the pack is tried on the session's copy of CLIENT's list
// (invList.roomFor: 40 records, worn items among them, a stackable onto the
// first record of its item), after what a give step takes from it: the
// step's rewards and its class's, the picked option's, the items its done
// events hand over, then the next step's start events' (the quest's finish
// events' after its last). When they do not all fit, nothing happens: no
// item is taken, no experience, tunar, faction, spell or item given, the step
// stays to do, and a line says so; the player makes room and talks again.
// Taking a quest whose accept events (or first step's start events) hand
// over items waits the same way. A kill, loot or reach step's last kill, loot
// or arrival waits too: its count is kept at what the step wants and the
// step is done once a look round (each second) finds the room.

// What is said when the pack is too full (ours).
const (
	questPackFull     = "Your pack is too full for %s. Make room and come back to me."
	questPackFullAway = "Your pack is too full for %s. Make room to finish %q."
	// how often a step waiting for room says so again
	questPackRetell = 2 * time.Minute
)

// eventGives is the items give events hand over.
func eventGives(evs []database.QuestEvent) []database.ItemCount {
	var out []database.ItemCount
	for _, e := range evs {
		if e.Do == database.EventGive {
			out = append(out, database.ParseItemCounts(e.Items)...)
		}
	}
	return out
}

// stepGains is everything doing a step puts in the player's pack: its
// rewards for the player's class and the picked option's (extra, nil none),
// its done events' items, then the next step's start events' or, after the
// last step, the quest's finish events'.
func stepGains(p *entity.Player, qu *database.Quest, st *database.QuestStep, extra *database.ClassReward) []database.ItemCount {
	class := 0
	if p != nil {
		class = p.Class
	}
	rw := st.RewardFor(class)
	if extra != nil {
		rw = rw.Plus(*extra)
	}
	out := append([]database.ItemCount(nil), rw.Items...)
	out = append(out, eventGives(database.EventsOn(st.Events, database.EventDone))...)
	if next := qu.Next(st.Step); next != nil {
		out = append(out, eventGives(database.EventsOn(next.Events, database.EventStart))...)
	} else {
		out = append(out, eventGives(database.EventsOn(qu.Events, database.EventFinish))...)
	}
	return out
}

// startGains is what taking a quest at step st puts in the pack: its accept
// events' items and st's start events'.
func startGains(qu *database.Quest, st *database.QuestStep) []database.ItemCount {
	out := eventGives(database.EventsOn(qu.Events, database.EventAccept))
	return append(out, eventGives(database.EventsOn(st.Events, database.EventStart))...)
}

// handInTakes is what a give step takes from the pack, record by record, as
// takeLocked takes it: the unworn inventory rows in their order.
func (q *QuestEngine) handInTakes(p *entity.Player, st *database.QuestStep) []invTake {
	if st.Kind != database.StepGive || p == nil {
		return nil
	}
	held := q.heldItems(p)
	var out []invTake
	for _, it := range st.Items {
		left := it.Count
		for i := range held {
			r := &held[i]
			if left == 0 || r.Pattern.PatternID != it.PatternID || r.StackLeft == 0 {
				continue
			}
			k := min(left, r.StackLeft)
			out = append(out, invTake{itemID: r.ItemID, n: k})
			r.StackLeft -= k
			left -= k
		}
	}
	return out
}

// packShortLocked is the gains that would not fit the session's pack once
// takes are taken, said as a list ("the Calamitous Falchion and 2 bone
// chips"); "" when everything fits, or there is no pack to fit (no session or
// no list yet: GiveItemNow gives nothing there either).
func (q *QuestEngine) packShortLocked(sess *session.Session, takes []invTake, gains []database.ItemCount) string {
	l := sessionInvList(sess)
	if l == nil || len(gains) == 0 {
		return ""
	}
	return packShort(l, takes, gains, func(id int) (database.ItemPatternRecord, bool) {
		pat, err := q.items.GetItemPattern(id)
		if err != nil || pat == nil {
			return database.ItemPatternRecord{}, false
		}
		return *pat, true
	})
}

// packShort is what of gains would not fit the list once takes are taken,
// said as a list, the items' records read with pattern ("" all fit); an item
// with no record is left out, as a reward gives none of it.
func packShort(l *invList, takes []invTake, gains []database.ItemCount, pattern func(int) (database.ItemPatternRecord, bool)) string {
	names := map[int]string{}
	var gives []invGive
	for _, g := range gains {
		pat, ok := pattern(g.PatternID)
		if !ok || g.Count < 1 {
			continue
		}
		names[pat.PatternID] = pat.ItemName
		gives = append(gives, invGive{patternID: pat.PatternID, n: g.Count, maxStack: pat.MaxStack})
	}
	short := l.roomFor(takes, gives)
	if len(short) == 0 {
		return ""
	}
	parts := make([]string, 0, len(short))
	for _, s := range short {
		if s.n == 1 {
			parts = append(parts, "the "+names[s.patternID])
		} else {
			parts = append(parts, countOf(s.n, names[s.patternID]))
		}
	}
	return joinAnd(parts)
}

// waitsForRoomLocked is a kill, loot or reach step whose last kill, loot or
// arrival came with no room for what doing it gives: its count held at what
// it wants, the player told (again at most every questPackRetell), and true;
// false when the step may be done now.
func (q *QuestEngine) waitsForRoomLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) bool {
	short := q.packShortLocked(sess, nil, stepGains(p, qu, st, nil))
	if short == "" {
		return false
	}
	need := max(st.Count, 1)
	if st.Kind == database.StepLoot {
		if _, n, ok := st.LootItem(); ok {
			need = n
		}
	}
	if st.Kind != database.StepReach && cq.Count > need {
		cq.Count = need
	}
	q.saveLocked(cq)
	key := [2]int{qu.ID, st.Step}
	now := time.Now()
	if c.packTold == nil {
		c.packTold = map[[2]int]time.Time{}
	}
	if told, ok := c.packTold[key]; !ok || now.Sub(told) >= questPackRetell {
		c.packTold[key] = now
		q.tellLocked(c, sess, "", fmt.Sprintf(questPackFullAway, short, qu.Name))
		who := fmt.Sprintf("character %d", c.serverID)
		if p != nil {
			who = p.Name
		}
		log.Printf("Quests: %s's step %d of %q waits for room for %s", who, st.Step, qu.Name, short)
	}
	return true
}

// checkWaiting does the kill and loot steps whose count is reached and that
// waited for room, once the room is there (a reach step is done by
// checkReach while its player stands in its place).
func (q *QuestEngine) checkWaiting() {
	if q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		var sess *session.Session
		for _, cq := range c.active() {
			qu := q.cat.byID[cq.QuestID]
			if qu == nil {
				continue
			}
			st := qu.Step(cq.Step)
			if st == nil || (st.Kind != database.StepKill && st.Kind != database.StepLoot) {
				continue
			}
			need := max(st.Count, 1)
			if st.Kind == database.StepLoot {
				_, n, ok := st.LootItem()
				if !ok {
					continue
				}
				need = n
			}
			if cq.Count < need {
				continue
			}
			if sess == nil {
				if sess = q.sessionOf(id); sess == nil {
					break
				}
			}
			p := q.playerOf(sess)
			if p == nil || q.waitsForRoomLocked(sess, p, c, qu, cq, st) {
				continue
			}
			delete(c.packTold, [2]int{qu.ID, st.Step})
			q.completeStepLocked(sess, p, c, qu, cq, st, nil, nil)
		}
	}
}
