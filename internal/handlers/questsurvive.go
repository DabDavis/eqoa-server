package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A survive step (database.StepSurvive; the Cazic-Thule trials, 2026-11-11,
// work board 430's code ask 6): the Trial of Endurance. "You will recieve a
// pop-up and 6 thoughtbleeders will spawn on you ... apparently the
// thoughtbleeders will continue to spawn for 30 minutes, you must kill them
// all"; "Questor must stay alive for 30 min to get pop up and advance to 2nd
// trial", yet a poster who died at the 29th minute and was rezzed had the
// popup (the t196 walkthrough). So:
//
//   - the step's count is seconds, counted (charquests.count) once a second
//     while the character is in the world, dead or alive, kept over a logout;
//   - its start's spawn events marked keep are kept up while they run: one of
//     their copies whose corpse is gone (looted, or decayed) is made again
//     before the player (a corpse lying counts: "dead bees not looted don't
//     respawn", a poster); a logout takes them away and they come back with
//     the player;
//   - once the time is up nothing more comes, and the step is done when the
//     player is alive (within the step's radius of x, z, when they are set:
//     "wait in the hall for the popup") and none of its kept spawns is alive.
//
// What the minutes left are is said every five minutes.

// surviveSaveEvery is how often (seconds) a survive step's count is written.
const surviveSaveEvery = 15

// checkSurvive counts the survive steps of the characters in the world, keeps
// their spawns up and does those that are over (once a second).
func (q *QuestEngine) checkSurvive() {
	if q.sessionOf == nil || q.playerOf == nil || q.gw == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		for _, cq := range c.active() {
			qu := q.cat.byID[cq.QuestID]
			if qu == nil {
				continue
			}
			st := qu.Step(cq.Step)
			if st == nil || st.Kind != database.StepSurvive {
				continue
			}
			sess := q.sessionOf(id)
			if sess == nil {
				break
			}
			p := q.playerOf(sess)
			if p == nil {
				break
			}
			need := max(st.Count, 1)
			if cq.Count < need {
				cq.Count++
				if cq.Count%surviveSaveEvery == 0 || cq.Count == need {
					q.saveLocked(cq)
				}
				if left := need - cq.Count; left > 0 && left%300 == 0 {
					SendClientMessage(sess, fmt.Sprintf("%s: %d minutes remain.", qu.Name, left/60))
				}
				if cq.Count < need {
					q.keepSpawnsLocked(p, c, qu, st)
					continue
				}
				SendClientMessage(sess, fmt.Sprintf("%s: the time is up.", qu.Name))
			}
			world, x, _, z, alive := q.gw.PlayerPlace(p)
			if !alive || q.keptAliveLocked(id, qu, st) {
				continue
			}
			if st.X != 0 || st.Z != 0 {
				r := st.Radius
				if r <= 0 {
					r = 15
				}
				if int(world) != st.World {
					continue
				}
				if dx, dz := x-st.X, z-st.Z; dx*dx+dz*dz > r*r {
					continue
				}
			}
			if q.waitsForRoomLocked(sess, p, c, qu, cq, st) {
				continue
			}
			q.completeStepLocked(sess, p, c, qu, cq, st, nil, nil)
		}
	}
}

// keptEvents is a survive step's kept spawns: its start's spawn events marked keep.
func keptEvents(st *database.QuestStep) []database.QuestEvent {
	var out []database.QuestEvent
	for _, e := range database.EventsOn(st.Events, database.EventStart) {
		if e.Do == database.EventSpawn && e.Keep && strings.TrimSpace(e.NPC) != "" {
			out = append(out, e)
		}
	}
	return out
}

// keepSpawnsLocked makes again the kept spawns of the step whose copies are
// gone (neither alive nor a corpse lying).
func (q *QuestEngine) keepSpawnsLocked(p *entity.Player, c *questChar, qu *database.Quest, st *database.QuestStep) {
	if _, _, _, _, ok := q.gw.PlayerPlace(p); !ok {
		return // dead: they come again when the player stands
	}
	for _, e := range keptEvents(st) {
		n := max(e.Count, 1)
		stand := 0
		for _, s := range q.spawns[c.serverID] {
			if s.quest != qu.ID || s.step != st.Step || !strings.EqualFold(s.npc, strings.TrimSpace(e.NPC)) {
				continue
			}
			if q.gw.NPCAlive(s.id) {
				stand++
			} else if _, lies := q.gw.CorpseLies(s.id); lies {
				stand++
			}
		}
		if stand < n {
			log.Printf("Quests: %q step %d: %d of %d %q stand for %s; more come", qu.Name, st.Step, stand, n, e.NPC, p.Name)
			q.spawnSomeLocked(p, c, qu, st.Step, e, stand, n, n)
		}
	}
}

// keptAliveLocked is whether one of the step's kept spawns for the character is alive.
func (q *QuestEngine) keptAliveLocked(serverID uint32, qu *database.Quest, st *database.QuestStep) bool {
	kept := keptEvents(st)
	for _, s := range q.spawns[serverID] {
		if s.quest != qu.ID || s.step != st.Step || !q.gw.NPCAlive(s.id) {
			continue
		}
		for _, e := range kept {
			if strings.EqualFold(s.npc, strings.TrimSpace(e.NPC)) {
				return true
			}
		}
	}
	return false
}
