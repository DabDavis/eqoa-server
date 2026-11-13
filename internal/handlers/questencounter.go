package handlers

// questencounter.go — a quest is never left stuck when the encounter it waits on
// ends without the kill (the owner, 2026-11-13, after Lonewulf lost two Rallos Zek
// fights to server updates).
//
// The guides' raid quests fire a GM trigger (questevents.go's trigger event) and
// then wait on a kill of a creature that trigger spawns: The Mark of Rallos
// (101441) fires "Rallos Zek" at its step 1 (the Mark pressed to the signpost),
// takes the Mark at step 2 (the portal) and waits at step 3 on "Rallos Zek
// Unbound", the run's last spawn. When the run ends without that kill (its clock,
// the isle empty its ten minutes, a GM's stop, a protect failed, a cold start, a
// restart that could not carry it) nothing could ever do step 3, and step 1, the
// only way to fire the trigger again, was behind the character: it was stuck.
//
// So a quest is linked to its encounter (encounterLink, read at Reload): a step F
// whose done fires trigger T (or the quest's accept, F = 0), and a later kill or
// loot step K whose creature T spawns (a spawn or resummon step of it, as the
// triggers are now). A character whose quest stands after F and at or before K
// while no run of T lives, and no copy of K's creature stands (a living spawn of
// the name, a trigger's, a quest's or a GM's; for a loot step a corpse of one still
// lying too, since the run ends at its creature's death and the looting comes
// after: Jarek's Creation, the Enraged Phantasms), is on a kill that cannot come,
// and once it has been so
// strandGrace (a firing that is a moment from starting its run, a login, the
// sessions coming back from a restart) its quest is put back to step F (F = 0:
// forgotten, to be taken again): its journal line again, and what the steps from
// F on took handed back (each give step's items and each give or show step's
// tunar: The Mark of Rallos comes back), so it can do F and fire the encounter
// again. One that got the kill is past K and is left alone; a run of T living
// (its own, or another raid's) leaves everyone as they are. The players looked at
// are those in the world, once a second: a character offline when its run ended is
// put back at its next login.

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// strandGrace is how long a character stands on an encounter's kill with no run of
// it before its quest is put back.
const strandGrace = 10 * time.Second

// encounterLink is a quest whose step from (0: its accept) fires trigger, and whose
// later kill or loot step kill waits on a creature (targets: its target and its
// others) the trigger may spawn.
type encounterLink struct {
	quest, from, kill int
	trigger           string
	targets           []string
	loot              bool // the kill step is a loot step: its corpses are looted after the kill
}

// encounterKey tells one link from another (a character's strandings are kept by it).
type encounterKey struct {
	quest, from, kill int
	trigger           string
}

func (l encounterLink) key() encounterKey {
	return encounterKey{quest: l.quest, from: l.from, kill: l.kill, trigger: l.trigger}
}

// encounterLinks are the quests' links to the triggers their kills wait on, as the
// quests are written (whether the trigger spawns the creature is asked of the
// triggers as they are, at each look).
func encounterLinks(all []*database.Quest) []encounterLink {
	var out []encounterLink
	for _, qu := range all {
		fires := func(from int, evs []database.QuestEvent) {
			for _, e := range evs {
				if e.Do != database.EventTrigger || strings.TrimSpace(e.Trigger) == "" {
					continue
				}
				for i := range qu.Steps {
					st := &qu.Steps[i]
					if st.Step <= from || st.Kind != database.StepKill && st.Kind != database.StepLoot {
						continue
					}
					targets := append([]string{st.Target}, st.Also...)
					out = append(out, encounterLink{quest: qu.ID, from: from, kill: st.Step, trigger: e.Trigger, targets: targets,
						loot: st.Kind == database.StepLoot})
				}
			}
		}
		fires(0, database.EventsOn(qu.Events, database.EventAccept))
		for i := range qu.Steps {
			fires(qu.Steps[i].Step, database.EventsOn(qu.Steps[i].Events, database.EventDone))
		}
	}
	return out
}

// checkStranded puts back the quests of the characters in the world standing on an
// encounter's kill with no run of it (the file's doc), once a second.
func (q *QuestEngine) checkStranded(now time.Time) {
	e := Triggers()
	if e == nil || q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.cat.encounters) == 0 {
		return
	}
	for id, c := range q.chars {
		for _, l := range q.cat.encounters {
			key := l.key()
			cq := c.quests[l.quest]
			onKill := cq != nil && cq.Active() && cq.Step > l.from && cq.Step <= l.kill
			if !onKill || !e.spawnsAny(l.trigger, l.targets) || e.RunningNamed(l.trigger) ||
				q.gw != nil && q.gw.CopyStands(l.targets, l.loot) {
				delete(c.stranded, key)
				continue
			}
			sess := q.sessionOf(id)
			if sess == nil {
				continue
			}
			p := q.playerOf(sess)
			if p == nil {
				continue
			}
			if c.stranded == nil {
				c.stranded = map[encounterKey]time.Time{}
			}
			first, seen := c.stranded[key]
			if !seen {
				c.stranded[key] = now
				continue
			}
			if now.Sub(first) < strandGrace {
				continue
			}
			delete(c.stranded, key)
			q.putBackLocked(sess, p, c, cq, l)
		}
	}
}

// spawnsAny reports the trigger called name spawns any of npcs.
func (e *TriggerEngine) spawnsAny(name string, npcs []string) bool {
	for _, n := range npcs {
		if strings.TrimSpace(n) != "" && e.SpawnsNPC(name, n) {
			return true
		}
	}
	return false
}

// putBackLocked is a stranded character's quest put back to its encounter's firing
// step (forgotten for one its accept fires), what the steps from there took handed
// back. Must be called with q.mu held.
func (q *QuestEngine) putBackLocked(sess *session.Session, p *entity.Player, c *questChar, cq *database.CharQuest, l encounterLink) {
	qu := q.cat.byID[l.quest]
	if qu == nil {
		return
	}
	at := cq.Step
	// what the steps from the firing one on took: a give step's items, a give or show step's tunar
	var back database.ClassReward
	for i := range qu.Steps {
		st := &qu.Steps[i]
		if st.Step < max(l.from, 1) || st.Step >= at {
			continue
		}
		if st.Kind == database.StepGive {
			back.Items = append(back.Items, st.Items...)
		}
		if st.Kind == database.StepGive || st.Kind == database.StepShow {
			back.Tunar += st.Tunar
		}
	}
	q.clearSpawnsLocked(c.serverID, qu.ID, 0)
	idx := c.index(qu.ID)
	var said string
	if l.from == 0 {
		q.giveUpLocked(cq)
		q.dropLineLocked(sess, c, idx)
		said = fmt.Sprintf("%s: the encounter ended before %s fell. The quest is yours to take again.", qu.Name, l.targets[0])
	} else {
		old := q.lineLocked(cq)
		cq.Step, cq.Count = l.from, 0
		if text := q.lineLocked(cq); text != old || idx < 0 {
			q.dropLineLocked(sess, c, idx)
			cq.Slot = c.nextSlot()
			q.addLineLocked(sess, c, text)
		}
		q.saveLocked(cq)
		said = fmt.Sprintf("%s: the encounter ended before %s fell. Its step is yours to do again.", qu.Name, l.targets[0])
	}
	lines := append([]string{said}, q.rewardLocked(sess, p, back)...)
	q.tellLocked(c, sess, "", lines...)
	log.Printf("Quests: %s's %q put back from step %d to step %d: no run of %q lives and %s did not fall (handed back: %d items, %d tunar)",
		p.Name, qu.Name, at, l.from, l.trigger, l.targets[0], len(back.Items), back.Tunar)
}
