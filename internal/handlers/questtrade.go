package handlers

import (
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questtrade.go — a trade picked in an NPC's talk tree (work board 747: Ionma Oschba's Benediction tokens,
// her menu as filmed): the answer names a quest whose first step gives items to this NPC, and picking it
// hands them in at once, as the step would at the next talk: its items taken and its choice of rewards
// asked (atNPCLocked), the quest taken first if it is not under way (a repeatable trade is taken again).
// A player without the items is told so (the answer's own words, else the step's reminder), and nothing
// is taken. An NPC whose tree trades takes no quest offer by talk (interact.go, tradeMenu): the tree is
// the way in.

// tradeLackingDefault is what an NPC says to a player without a trade's items when neither the answer nor
// the step says anything (ours).
const tradeLackingDefault = "You do not have what I trade for."

// TradeAt hands in quest questID's first step at the NPC called giver (spawn, its spawn), for a talk
// tree's trade answer. It reports whether the quests answered (false: no such trade here).
func (q *QuestEngine) TradeAt(sess *session.Session, p *entity.Player, giver string, spawn entity.EntityID, questID int, lacking string) bool {
	if q == nil || p == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[questID]
	if qu == nil {
		log.Printf("Quests: a trade at %s names quest %d, which is not read", giver, questID)
		return false
	}
	st := qu.First()
	if st == nil || st.Kind != database.StepGive || !stepNPC(st, giver, spawn) {
		log.Printf("Quests: a trade at %s names %q, whose first step is not a give here", giver, qu.Name)
		return false
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	look := q.lookAt(sess)
	if q.lackingLocked(p, st) != "" {
		q.tellLocked(c, sess, giver, orText(lacking, orText(st.Reminder, tradeLackingDefault)))
		return true
	}
	cq := c.quests[qu.ID]
	if cq == nil || !cq.Active() {
		if why := q.mayTakeLocked(look, p, c, qu); why != "" {
			log.Printf("Quests: %s picked the trade %q at %s, but %s", p.Name, qu.Name, giver, why)
			if line := giverWontDeal(look, qu, st); line != "" {
				q.tellLocked(c, sess, "", line)
			}
			return true
		}
		if why := q.startLocked(sess, p, c, qu, 0, false); why != "" {
			log.Printf("Quests: %s's trade %q not taken: %s", p.Name, qu.Name, why)
			return true
		}
		if cq = c.quests[qu.ID]; cq == nil || !cq.Active() {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(st.Target), strings.TrimSpace(giver)) || stepNPC(st, giver, spawn) {
		q.atNPCLocked(sess, p, c, qu, cq, st, false, -1)
	}
	return true
}
