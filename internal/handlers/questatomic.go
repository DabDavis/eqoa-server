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

// A give step whose doing fires an encounter (a done event's trigger, not a held
// one: the Plane of Sky's shard hand-ins, board entry 1263) is done as ONE outcome:
//
//  1. every trigger it fires is reserved (TriggerEngine.ReserveNamed): checked as a
//     firing is (there, on, as written, placed) and its lock taken, so no GM, death
//     or other quest can fire into it meanwhile; any refusal takes nothing;
//  2. the items, the step's tunar and the quest's row as the step leaves it are
//     written in one transaction (database.InventoryStore.HandIn); a refusal of any
//     row (or the progress) changes nothing, and the reservations are released;
//  3. only then is the client told, and the step done as ever (completeStepSkipLocked),
//     its trigger event starting the reserved run, exactly once.
//
// Lock order: QuestEngine.mu (held throughout), then TriggerEngine.mu (taken and let go
// by each reserve, activate, release), then the database's own; no lock is taken
// twice. A reservation is never kept over a restart: either the commit happened and
// the run was started in the same call, or nothing was done.

// handInTriggers are the triggers a step's doing fires (its done events' trigger,
// not a held one: that fires at the NPC, questhold.go).
func handInTriggers(st *database.QuestStep) []database.QuestEvent {
	var out []database.QuestEvent
	for _, e := range database.EventsOn(st.Events, database.EventDone) {
		if e.Do == database.EventTrigger && !e.Hold && strings.TrimSpace(e.Trigger) != "" {
			out = append(out, e)
		}
	}
	return out
}

// handInCannot is said when a hand-in's encounter cannot be started (none, off, as
// written it cannot run, no place): nothing is taken (ours).
const handInCannot = "That cannot be done just now. Come back later."

// atomicHandInLocked does such a step for the player: reserved, taken and written
// whole, then done; or nothing changes and the player is told why. opt is the
// step's choice picked (no cost), or nil.
func (q *QuestEngine) atomicHandInLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, opt *database.StepOption, pick int) {
	var held []*TriggerReservation
	release := func() {
		for _, r := range held {
			Triggers().Release(r)
		}
		q.dropReservedLocked(c.serverID)
	}
	for _, e := range handInTriggers(st) {
		res, lines, err := Triggers().ReserveNamed(e.Trigger, TriggerFirer{Name: p.Name, Sess: sess, Player: p, Quiet: true})
		if err != nil {
			release()
			log.Printf("Quests: %s's hand-in at %q step %d: its trigger %q refused (%v; %s); nothing taken",
				p.Name, qu.Name, st.Step, e.Trigger, err, strings.Join(lines, "; "))
			if errors.Is(err, ErrTriggerBusy) {
				q.tellLocked(c, sess, st.Target, orText(e.Text, "Others are there already. Come back when they are done."))
			} else {
				q.tellLocked(c, sess, st.Target, handInCannot)
			}
			return
		}
		held = append(held, res)
		q.putReservedLocked(c.serverID, e.Trigger, res)
	}
	skip := 0
	var extra *database.ClassReward
	picks := cq.Picks
	if opt != nil {
		skip, extra = opt.Skip, &opt.Reward
		picks = database.WithPick(cq.Picks, st.Step, pick)
	}
	after := q.progressAfterLocked(c, qu, cq, st, skip)
	after.Picks = picks
	given, err := q.takeTxLocked(sess, p, st, &after)
	if err != nil {
		release()
		q.tellLocked(c, sess, st.Target, sayLines(orText(st.Reminder, questNotYet))...)
		return
	}
	if opt != nil {
		q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
		cq.Picks = picks
		log.Printf("Quests: %s picked %q at %q step %d", p.Name, opt.Answer, qu.Name, st.Step)
	}
	q.completeStepSkipLocked(sess, p, c, qu, cq, st, given, extra, skip)
	if cq.Step != after.Step || cq.Count != after.Count || cq.Done != after.Done {
		log.Printf("Quests: %s's %q after step %d is step %d count %d done %d, written as step %d count %d done %d",
			p.Name, qu.Name, st.Step, cq.Step, cq.Count, cq.Done, after.Step, after.Count, after.Done)
	}
	// a run its events did not start (an event the step lost meanwhile) is let go
	for _, r := range q.reservedRuns[c.serverID] {
		Triggers().Release(r)
	}
	q.dropReservedLocked(c.serverID)
}

// progressAfterLocked is the quest's row as completeStepSkipLocked leaves it for
// this step (its next step, its group's count, finished and counted), worked out
// on a copy, for the hand-in's transaction to write.
func (q *QuestEngine) progressAfterLocked(c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, skip int) database.CharQuest {
	cp := *cq
	idx := c.index(qu.ID)
	next := qu.Next(st.Step)
	for ; skip > 0 && next != nil; skip-- {
		next = qu.Next(next.Step)
	}
	stay, oldLine := false, q.lineLocked(cq)
	if all, last, ok := anyOrderDone(qu, &cp, st); ok {
		next, stay = qu.Next(last.Step), !all
	}
	switch {
	case stay:
		if q.lineLocked(&cp) != oldLine || idx < 0 {
			cp.Slot = c.nextSlot()
		}
	case next == nil:
		cp.Step, cp.Count = 0, 0
		cp.Done++
	default:
		cp.Step, cp.Count = next.Step, 0
		if q.lineLocked(&cp) != oldLine || idx < 0 {
			cp.Slot = c.nextSlot()
		}
	}
	return cp
}

// putReservedLocked keeps a hand-in's reservation for its done event.
func (q *QuestEngine) putReservedLocked(serverID uint32, trigger string, res *TriggerReservation) {
	if q.reservedRuns == nil {
		q.reservedRuns = map[uint32]map[string]*TriggerReservation{}
	}
	if q.reservedRuns[serverID] == nil {
		q.reservedRuns[serverID] = map[string]*TriggerReservation{}
	}
	q.reservedRuns[serverID][foldName(trigger)] = res
}

// takeReservedLocked is the reservation the character's hand-in holds for that
// trigger, taken out (nil for none: the event fires as ever).
func (q *QuestEngine) takeReservedLocked(serverID uint32, trigger string) *TriggerReservation {
	m := q.reservedRuns[serverID]
	if m == nil {
		return nil
	}
	k := foldName(trigger)
	res := m[k]
	delete(m, k)
	return res
}

// dropReservedLocked forgets the character's reservations.
func (q *QuestEngine) dropReservedLocked(serverID uint32) {
	delete(q.reservedRuns, serverID)
}

// String names a reservation in a log line.
func (r *TriggerReservation) String() string {
	if r == nil {
		return "no reservation"
	}
	return fmt.Sprintf("%s (reservation %d)", r.run.trig.Title(), r.id)
}
