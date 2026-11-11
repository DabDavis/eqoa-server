package handlers

import (
	"errors"
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A held trigger (petition 1617, Lickinfrogs: "collected items for froglok
// herbalist, turned in and no mobs spawned"; EQOA-Data's Good Tank, Episode 1:
// "Talking to the Herbalist with your ingredients will initiate an event in which
// you must protect the herbalist ... If the herbman dies while you are making
// potions, you will have to wait until someone else triggers the event. Since the
// popups have an Area of Effect, your quest will still be advanced if you have the
// necessary items in your inventory").
//
// A give or show step whose done events hold a trigger event with hold set is not
// done at its NPC: talked to with what the step needs, the NPC fires the trigger
// (its lock held: the event's text, as any trigger event's) and says the event's
// says, and nothing changes hands. The trigger's queststep step does the step
// later, as at its NPC, for every player its scope takes then who is on it and
// holds what it needs (DoHeldStep): the items taken, the step's words, rewards and
// done events (the held trigger not fired again). A run that fails before it (its
// protect step's NPC dead) leaves every player's items and step as they were.

// holdLocked is a held trigger's firing at its step's NPC: the trigger fired by
// the player, the step left to do. Must be called with q.mu held.
func (q *QuestEngine) holdLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, st *database.QuestStep, ev database.QuestEvent) {
	_, err := Triggers().FireNamed(ev.Trigger, TriggerFirer{Name: p.Name, Sess: sess, Player: p, Quiet: true})
	switch {
	case errors.Is(err, ErrTriggerBusy):
		q.tellLocked(c, sess, st.Target, orText(ev.Text, "Others are there already. Come back when they are done."))
		return
	case err != nil:
		log.Printf("Quests: %q step %d holds %q: %v", qu.Name, st.Step, ev.Trigger, err)
		q.tellLocked(c, sess, st.Target, "Not now. Come back a little later.")
		return
	}
	log.Printf("Quests: %s at %q step %d fired %q; the step waits for it", p.Name, qu.Name, st.Step, ev.Trigger)
	q.tellLocked(c, sess, st.Target, sayLines(ev.Says)...)
}

// heldMember is a player a trigger's queststep reaches.
type heldMember struct {
	sess *session.Session
	p    *entity.Player
}

// DoHeldStep does a quest's step for each player who is on it and holds what it
// needs, as at its NPC (a trigger's queststep; the file's doc). It answers how
// many did it. The trigger engine calls it on a goroutine of its own: a step's
// events may fire triggers (q.mu is taken before the engine's lock, never after).
func (q *QuestEngine) DoHeldStep(members []heldMember, questID, step int) int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[questID]
	if qu == nil {
		log.Printf("Quests: a trigger's queststep names quest %d, not in the game", questID)
		return 0
	}
	st := qu.Step(step)
	if st == nil {
		log.Printf("Quests: a trigger's queststep names step %d of %q, which it has not", step, qu.Name)
		return 0
	}
	n := 0
	for _, m := range members {
		if m.p == nil || m.sess == nil || !m.sess.CharacterInWorld() {
			continue
		}
		c := q.namedLocked(m.p.CharServerID, m.p.Name)
		cq := c.quests[questID]
		if cq == nil || !cq.Active() || cq.Step != step {
			continue
		}
		if why := q.heldStepLocked(m.sess, m.p, c, qu, cq, st); why != "" {
			log.Printf("Quests: %s's step %d of %q stays: %s", m.p.Name, step, qu.Name, why)
			continue
		}
		n++
	}
	return n
}

// heldStepLocked does one player's held step, or says why not. Must be called
// with q.mu held.
func (q *QuestEngine) heldStepLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) string {
	if st.Kind == database.StepGive || st.Kind == database.StepShow {
		if lack := q.lackingLocked(p, st); lack != "" {
			return "lacks " + lack
		}
	}
	if short := q.packShortLocked(sess, q.handInTakes(p, st), stepGains(p, qu, st, nil)); short != "" {
		q.tellLocked(c, sess, st.Target, fmt.Sprintf(questPackFull, short))
		return "no room for " + short
	}
	var given *handedIn
	switch st.Kind {
	case database.StepGive:
		var ok bool
		if given, ok = q.takeLocked(sess, p, st); !ok {
			return "its items could not be taken"
		}
	case database.StepShow:
		given = &handedIn{ascended: q.planHandInLocked(p, st).ascended}
		q.payStepTunarLocked(sess, p, st, given)
	}
	q.completeStepLocked(sess, p, c, qu, cq, st, given, nil)
	return ""
}
