package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questdeath.go — a quest's playerdeath event (database.EventPlayerDeath): the
// player dies as the quest's price (entity.World.QuestDeath: no debt, no
// tunar, never a Hardcore fall), and the events written after it in the same
// list wait for the respawn at its bind point.
//
// Heirloom petition 3488 (Nathan, 2026-11-28; the owner approved it as the
// player asked): "once you turn in this envaric ascension quest, it actually
// kills you as part of the quest. once you pop up at the spirit master ...
// you will see that you have received the port via a pop up".

// priceAfterRespawn is how long after the respawn's MemoryDump the waiting
// events run: the client fades in and answers the dump first (respawn.go).
var priceAfterRespawn = 3 * time.Second

// priceRestLife is how long the waiting events are kept for a respawn that
// does not come (a logout while dead): a death's own wait and then some.
const priceRestLife = 10 * time.Minute

// priceRest is the rest of an event list after a playerdeath, kept for the respawn.
type priceRest struct {
	quest, step, spawnStep int
	evs                    []database.QuestEvent
	at                     time.Time
}

// priceDeathLocked is a playerdeath event: the player killed as the quest's
// price, the events after it (rest) kept for its respawn. It reports false
// when the player could not be killed (dead already, a GM's test mode), and
// the rest then run at once.
func (q *QuestEngine) priceDeathLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, step, spawnStep int, rest []database.QuestEvent) bool {
	if q.gw == nil || c == nil || !q.gw.QuestDeath(sess, p) {
		log.Printf("Quests: %q step %d: %s not killed for its price; the rest at once", qu.Name, step, p.Name)
		return false
	}
	if len(rest) > 0 {
		if q.afterDeath == nil {
			q.afterDeath = map[uint32]priceRest{}
		}
		q.afterDeath[c.serverID] = priceRest{quest: qu.ID, step: step, spawnStep: spawnStep,
			evs: append([]database.QuestEvent(nil), rest...), at: time.Now()}
	}
	log.Printf("Quests: %q step %d: %s dies for its price, %d events after the respawn", qu.Name, step, p.Name, len(rest))
	return true
}

// AfterRespawn runs, priceAfterRespawn later, the events a quest's price kept
// for the player's respawn (respawn.go: the bind point's, or a rez's), if any.
func (q *QuestEngine) AfterRespawn(sess *session.Session, p *entity.Player) {
	if q == nil || sess == nil || p == nil {
		return
	}
	q.mu.Lock()
	r, ok := q.afterDeath[p.CharServerID]
	delete(q.afterDeath, p.CharServerID)
	q.mu.Unlock()
	if !ok || time.Since(r.at) > priceRestLife {
		return
	}
	time.AfterFunc(priceAfterRespawn, func() { q.runPriceRest(sess, p, r) })
}

// runPriceRest runs a price's kept events for a player still in the world.
func (q *QuestEngine) runPriceRest(sess *session.Session, p *entity.Player, r priceRest) {
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[r.quest]
	if qu == nil || !sess.CharacterInWorld() || q.gw.SessionPlayer(sess) != p {
		return
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	q.runEventsLocked(sess, p, c, qu, r.step, r.spawnStep, r.evs)
}
