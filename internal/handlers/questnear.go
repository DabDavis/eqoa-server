package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
)

// questnear.go — a step's events while the player on it stands within a
// place (database.EventNear: {"on":"near", ...}). Heirloom petition 3332
// (Lonewulf, 2026-11-27): "Tallon Zek" (101327, repeatable) wakes its raid at
// step 4, a reach step at the fortress whose done fires the GM triggers
// "Tallon's Fortress" (its lock, a two-hour clock, the guardians, the
// sentry, the golems, Tallon himself) and "Fortress Post". A player already
// past step 4 when a run ended (on step 5, the Zek sentry, or step 6, Tallon
// Zek) could never wake the fortress again: nothing fired the trigger for it,
// and the encounter's put-back (questencounter.go) does not hold, the sentry
// being a creature of the world, not the run's. A GM fired trigger 130 by hand
// at 02:30 UTC that night. The owner, 2026-11-27: "yes build the fortress
// restart fix".
//
//   - A near event is a step's, with a place: its own world, x, z and radius,
//     or the step's (its radius and height) when its x and z are 0
//     (database.NearPlace). The near events of the step the player is on whose
//     place is one run together, in order (two triggers at the fortress, as
//     step 4 fires them), with runEventsLocked as any time's events.
//   - Once a second (checkNear, beside checkReach), for each character in the
//     world and alive, standing within one: once each time it comes in. It
//     steps out and in again to have them again; a login starts afresh (a
//     restart too). Coming in is into the place, not the step: a quest moved on
//     to a step of the same near place while the player stands in it (step 5's
//     sentry killed in the fortress, step 6 waking nothing more) has not come
//     in again.
//   - Not while a trigger among them is kept from firing by its lock (a run of
//     it in progress: triggerBusyLocked's rule, as step 4's reach waits): the
//     trigger event's text is said ("The fortress is awake already: another
//     raid is inside."), once that coming in, and the events run the second the
//     lock is free while the player is still there.
//
// Nothing else of the step is done here: a kill step stays a kill step.

// nearKey is a character's coming into a quest's near place.
type nearKey struct {
	quest int
	place database.QuestPlace
}

// A coming in's state: told the lock is held (its events to run when it is
// free), or its events run.
const (
	nearWaiting = 1
	nearDone    = 2
)

// nearGroup is the near events of a step at one place, in order.
type nearGroup struct {
	place database.QuestPlace
	evs   []database.QuestEvent
}

// nearGroupsOf is a step's near events by place, in the order of each place's
// first event; an event with no place (neither its own nor its step's) is in
// none.
func nearGroupsOf(st *database.QuestStep) []nearGroup {
	var out []nearGroup
	for _, e := range database.EventsOn(st.Events, database.EventNear) {
		pl, ok := database.NearPlace(st, e)
		if !ok {
			continue
		}
		i := 0
		for i < len(out) && out[i].place != pl {
			i++
		}
		if i == len(out) {
			out = append(out, nearGroup{place: pl})
		}
		out[i].evs = append(out[i].evs, e)
	}
	return out
}

// nearQuestsOf is the quests with a near event on a step, by id.
func nearQuestsOf(all []*database.Quest) map[int]bool {
	out := map[int]bool{}
	for _, qu := range all {
		for i := range qu.Steps {
			if len(database.EventsOn(qu.Steps[i].Events, database.EventNear)) > 0 {
				out[qu.ID] = true
				break
			}
		}
	}
	return out
}

// nearBusy is what is said when a trigger among a near place's events is kept
// from firing by its lock (its event's text), "" when they may run.
func nearBusy(evs []database.QuestEvent) string {
	for _, e := range evs {
		if e.Do == database.EventTrigger && Triggers().BusyNamed(e.Trigger) {
			return orText(e.Text, "Others are there already. Come back when they are done.")
		}
	}
	return ""
}

// checkNear runs the near events of the characters standing in their steps'
// near places, once each coming in. Once a second, off the engine's own
// goroutine.
func (q *QuestEngine) checkNear() {
	if q.sessionOf == nil || q.playerOf == nil || q.gw == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.cat.nears) == 0 {
		for _, c := range q.chars {
			c.nearAt = nil
		}
		return
	}
	for id, c := range q.chars {
		sess := q.sessionOf(id)
		if sess == nil {
			c.nearAt = nil // out of the world: its next coming in is one
			continue
		}
		p := q.playerOf(sess)
		if p == nil {
			continue
		}
		world, x, y, z, ok := q.gw.PlayerPlace(p)
		if !ok {
			continue // dead or loading: where it stood stands until it is seen again
		}
		var here map[nearKey]bool
		for _, cq := range c.active() {
			if !q.cat.nears[cq.QuestID] {
				continue
			}
			qu := q.cat.byID[cq.QuestID]
			if qu == nil {
				continue
			}
			st := qu.Step(cq.Step)
			if st == nil {
				continue
			}
			for _, g := range nearGroupsOf(st) {
				if !g.place.Within(int(world), x, y, z) {
					continue
				}
				k := nearKey{quest: qu.ID, place: g.place}
				if here == nil {
					here = map[nearKey]bool{}
				}
				here[k] = true
				state := c.nearAt[k]
				if state == nearDone {
					continue
				}
				if c.nearAt == nil {
					c.nearAt = map[nearKey]int{}
				}
				if line := nearBusy(g.evs); line != "" {
					if state == 0 {
						c.nearAt[k] = nearWaiting
						log.Printf("Quests: %s near step %d of %q (world %d, %.1f, %.1f): a trigger's lock is held", p.Name, st.Step, qu.Name, world, x, z)
						q.tellLocked(c, sess, "", line)
					}
					continue
				}
				c.nearAt[k] = nearDone
				log.Printf("Quests: %s near step %d of %q (world %d, %.1f, %.1f): its events run", p.Name, st.Step, qu.Name, world, x, z)
				q.runEventsLocked(sess, p, c, qu, st.Step, st.Step, g.evs)
			}
		}
		for k := range c.nearAt {
			if !here[k] {
				delete(c.nearAt, k) // stepped out (or the step's place is no more): the next coming in is one
			}
		}
	}
}
