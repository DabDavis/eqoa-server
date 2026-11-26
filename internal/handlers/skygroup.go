package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Plane of Sky's chains done as a group (Live petition 5510, the owner,
// 2026-11-23: "when someone in your group completes a Chain event in PoS it
// should be a group-wide flag, and you should be able to talk to the
// Manifestations if you did all the chain but didn't loot all keys; as long as
// 1 person in your group is on the quest it should group complete and accept
// the different quests"). A group of four rode the rain chain's passages
// together, but only the one who talked to Sterie Glitterlight went on in
// "Sterie Glitterlight": the other three stayed at its first step for the
// rest of the chain, never took "The Necrologist of Rain", and had nothing to
// say to the Manifestation of Rain at its end.
//
// For the chains skypass.go's passages carry (skyChainQuests), each step one
// player does (a passage's, a kill's, a loot's, a talk or a give at an NPC,
// the Manifestation's or Mark's at the end) is done too for each member of its
// group (a raid's too) alive in its world within teleReach of it (the reach
// the passages move a group by) who is on that quest at that step: no item is
// taken from the member, the doer's hand-in counts for it, and the step's
// rewards and events are the member's own, but for the world's one-a-group
// events (a trigger, a spawn, an attack, a hide: the doer's alone). A Sky
// chain quest one player takes is taken too by each such member who may take
// it and has it not under way. A member one step behind a passage's step (the
// step the group did before it, out of reach) has that step done with the
// passage's, and a player one step behind the step a portal's way back waits
// for (skyBack) goes on with a groupmate by it who is past it. Other quests
// are untouched: their kills are shared with the group as they always were,
// and each player does its own talks and hand-ins.

// skyChainQuests are the chains: rain (101301-101303), the sun and day
// (101304-101305), night (101306), snow (101307), desolation (101308).
var skyChainQuests = map[int]bool{101301: true, 101302: true, 101303: true, 101304: true, 101305: true,
	101306: true, 101307: true, 101308: true}

// skyMate is a groupmate by the doer: its session, player and quest record.
type skyMate struct {
	sess *session.Session
	p    *entity.Player
	c    *questChar
}

// skyMatesLocked is p's groupmates alive in its world within teleReach of
// it, in the world now. Must be called with q.mu held.
func (q *QuestEngine) skyMatesLocked(p *entity.Player) []skyMate {
	if q.gw == nil || p == nil || q.sessionOf == nil {
		return nil
	}
	members := q.gw.GroupMembers
	if q.mates != nil {
		members = q.mates
	}
	w, x, _, z, dead, ok := q.gw.PlayerSpot(p)
	if !ok || dead {
		return nil
	}
	var out []skyMate
	for _, m := range members(p) {
		if m == nil || m == p || m.CharServerID == p.CharServerID {
			continue
		}
		mw, mx, _, mz, mdead, ok := q.gw.PlayerSpot(m)
		if !ok || mdead || mw != w || (mx-x)*(mx-x)+(mz-z)*(mz-z) > teleReach*teleReach {
			continue
		}
		ms := q.sessionOf(m.CharServerID)
		if ms == nil {
			continue
		}
		out = append(out, skyMate{sess: ms, p: m, c: q.namedLocked(m.CharServerID, m.Name)})
	}
	return out
}

// skyGroupStepLocked does st for each groupmate by p on the quest at it (a
// Sky chain's step p is doing), before p's own is done (p's own events may
// carry it away). Nothing for another quest, nor within another's doing.
// Must be called with q.mu held.
func (q *QuestEngine) skyGroupStepLocked(p *entity.Player, qu *database.Quest, st *database.QuestStep) {
	if q.skySyncing || p == nil || !skyChainQuests[qu.ID] {
		return
	}
	mates := q.skyMatesLocked(p)
	if len(mates) == 0 {
		return
	}
	q.skySyncing = true
	defer func() { q.skySyncing = false }()
	for _, m := range mates {
		cq := m.c.quests[qu.ID]
		if cq == nil || !cq.Active() || cq.Step != st.Step {
			continue
		}
		if st.Kind == database.StepKill && q.skyKill[m.c.serverID] {
			continue // the kill counts for it on its own (killed)
		}
		log.Printf("Quests: %s's step %d of %q is done for %s too (Sky chain, group)", p.Name, st.Step, qu.Name, m.p.Name)
		q.completeStepLocked(m.sess, m.p, m.c, qu, cq, st, nil, nil)
	}
}

// skyGroupTakeLocked gives a Sky chain quest p took to each groupmate by it
// who may take it and has it not under way. Must be called with q.mu held.
func (q *QuestEngine) skyGroupTakeLocked(p *entity.Player, qu *database.Quest, step int) {
	if q.skySyncing || p == nil || !skyChainQuests[qu.ID] {
		return
	}
	mates := q.skyMatesLocked(p)
	if len(mates) == 0 {
		return
	}
	q.skySyncing = true
	defer func() { q.skySyncing = false }()
	for _, m := range mates {
		if cq := m.c.quests[qu.ID]; cq != nil && cq.Active() {
			continue
		}
		if why := q.mayTakeLocked(q.lookAt(m.sess), m.p, m.c, qu); why != "" {
			log.Printf("Quests: %s takes %q; %s does not: %s", p.Name, qu.Name, m.p.Name, why)
			continue
		}
		if why := q.startLocked(m.sess, m.p, m.c, qu, step, false); why != "" {
			log.Printf("Quests: %s takes %q; %s could not: %s", p.Name, qu.Name, m.p.Name, why)
			continue
		}
		log.Printf("Quests: %s takes %q with %s (Sky chain, group)", m.p.Name, qu.Name, p.Name)
	}
}

// skyWorldEvent is whether an event is the world's, one for the group (the
// doer's alone when a step is done for its groupmates).
func skyWorldEvent(do string) bool {
	switch do {
	case database.EventTrigger, database.EventSpawn, database.EventAttack, database.EventHide, database.EventDie:
		return true
	}
	return false
}

// stepBefore is the quest's step before step, nil for none.
func stepBefore(qu *database.Quest, step int) *database.QuestStep {
	var out *database.QuestStep
	for i := range qu.Steps {
		s := &qu.Steps[i]
		if s.Step < step && (out == nil || s.Step > out.Step) {
			out = s
		}
	}
	return out
}

// CatchUpPassSteps does, for a member carried by a passage, the step before
// each of the passage's steps (Sky chains only) when it is at that one: the
// group did it before the passage, the member out of reach then. PassSteps
// then does the passage's own. It reports how many it did.
func (q *QuestEngine) CatchUpPassSteps(sess *session.Session, p *entity.Player, those [][2]int) int {
	if q == nil || p == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	n := 0
	for _, s := range those {
		qu := q.cat.byID[s[0]]
		if qu == nil || !skyChainQuests[qu.ID] {
			continue
		}
		cq := c.quests[qu.ID]
		prev := stepBefore(qu, s[1])
		if cq == nil || !cq.Active() || prev == nil || cq.Step != prev.Step {
			continue
		}
		log.Printf("Quests: %s goes on with its group: step %d of %q done with the passage's", p.Name, prev.Step, qu.Name)
		q.skySyncing = true
		q.completeStepLocked(sess, p, c, qu, cq, prev, nil, nil)
		q.skySyncing = false
		n++
	}
	return n
}

// MateAheadCatchUp is a portal's way back's question (skyBack): a player of
// the quest one step behind before (at the step before it) with a groupmate by
// it on the quest at before or past it, or finished with it, has that step
// done and goes on with the group. It reports whether it did.
func (q *QuestEngine) MateAheadCatchUp(sess *session.Session, p *entity.Player, quest, before int) bool {
	if q == nil || p == nil || !skyChainQuests[quest] {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qu := q.cat.byID[quest]
	if qu == nil {
		return false
	}
	c := q.namedLocked(p.CharServerID, p.Name)
	cq := c.quests[quest]
	prev := stepBefore(qu, before)
	if cq == nil || !cq.Active() || prev == nil || cq.Step != prev.Step {
		return false
	}
	for _, m := range q.skyMatesLocked(p) {
		mq := m.c.quests[quest]
		if mq == nil || !(mq.Active() && mq.Step >= before || !mq.Active() && mq.Done > 0) {
			continue
		}
		log.Printf("Quests: %s goes on with %s: step %d of %q done with the group's", p.Name, m.p.Name, prev.Step, qu.Name)
		q.skySyncing = true
		q.completeStepLocked(sess, p, c, qu, cq, prev, nil, nil)
		q.skySyncing = false
		return true
	}
	return false
}
