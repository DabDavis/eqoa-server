package handlers

// questtrials.go — a trial's clock, its forbidden ground and its price (Live
// petition 5663, Xtra: "The were hunter trials are not working like this guide
// from archive"; the owner, 2026-11-26, of what the data alone could not do:
// "fix these"). The Were-Hunter Trials as eqoa.forumotion.com t235 (Ronson,
// 2010) has them:
//
//   - Trainer Iruko "will present you with the Concentration Totem and your TEN
//     MINUTES will start"; "If you HAPPEN to answer incorrectly or take longer
//     than 10 minutes; you will LOSE 75 faction with the WereHunters of Tunaria
//     and will have to re-farm those 75 faction points back in order to take the
//     test over";
//   - Master Landaar: "After he tells you to begin, the clock for a 15-minute
//     deadline begins ticking"; "If you fail this portion of the trial ... He
//     will tell you to redo the 1st Trial (no faction lost unless you answer
//     wrong)";
//   - the captured were-beast: "DO NOT ... Go directly to the were-beast as you
//     WILL scare it off and you will have to repeat the 1st Trial over again";
//   - Nivaan D'Tarin: "if you do not have your sealed letter then you will lose
//     -250 faction with the werehunters and then have to farm it".
//
// Each is a setback: the quest put back to an earlier step, standings changed
// and, with regain, that step not done again until the standing lost is earned
// back. Written as the quests' data, so any quest may use them
// (database.QuestEvent's clock, ward and back; database.StepOption's back,
// factions and regain):
//
//   - a clock event (a step's start or done): the time from then to reach step
//     until; short of it when the time is up, the setback. It runs whether the
//     player is in the world or not (a guide's minutes are the clock's on the
//     wall) and over a restart (table charquesttrials); one offline when it ran
//     out has the setback at its next login;
//   - a ward event (a step's start): a place the player must not come into while
//     on the step;
//   - a back event (a give or show step's, on lacking): the step's NPC talked to
//     without what the step needs (its notyet, else its reminder, said first);
//   - a choice's option with back: picked, the setback instead of the step.
//
// Putting the quest back undoes the steps passed back over: what their give
// steps took is handed back and what they gave (their done events' gives, their
// reward items) taken back, when held, so a step is done again as it was the
// first time (Irlynn Poe's ring: the plain one handed back, the upgraded one
// taken, and she upgrades it again on the way). The steps' spawns go (Iruko's
// three beasts), and the clock with them. A standing owed is asked for at the
// step put back to, by its NPC, until it is met again.

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// What a trial says on its own (the data's words come first when it has any).
const (
	questClockLeft   = "%s: %d minutes remain."
	questClockLeft1  = "%s: one minute remains."
	questClockUp     = "%s: your time is up."
	questOwesLine    = "You have lost the trust of the %s. Earn it back, then come to me again."
	questTakenBack   = "%s is taken back."
	questClockWarnAt = 5 // minutes left said, then one
)

// setback is a trial's setback (the file's doc).
type setback struct {
	back     int
	factions []database.FactionDelta
	regain   bool
	speaker  string
	says     string
}

// trialsLocked is the character's trial states, read once.
func (q *QuestEngine) trialsLocked(c *questChar) map[int]*database.QuestTrial {
	if c.trials == nil {
		c.trials = map[int]*database.QuestTrial{}
		if q.store != nil {
			if all, err := q.store.QuestTrials(c.serverID); err != nil {
				log.Printf("Quests: reading character %d's trials: %v", c.serverID, err)
			} else {
				c.trials = all
			}
		}
	}
	return c.trials
}

// trialLocked is the character's trial state in a quest, made when make is set.
func (q *QuestEngine) trialLocked(c *questChar, quest int, make bool) *database.QuestTrial {
	all := q.trialsLocked(c)
	t := all[quest]
	if t == nil && make {
		t = &database.QuestTrial{ServerID: c.serverID, QuestID: quest}
		all[quest] = t
	}
	return t
}

// saveTrialLocked writes a trial state (its row gone when it holds nothing).
func (q *QuestEngine) saveTrialLocked(c *questChar, t *database.QuestTrial) {
	if t.Empty() {
		delete(c.trials, t.QuestID)
	}
	if q.store == nil {
		return
	}
	if err := q.store.SaveQuestTrial(t); err != nil {
		log.Printf("Quests: character %d's trial in quest %d: %v", c.serverID, t.QuestID, err)
	}
}

// startClockLocked is a clock event: the time from now to reach e.Until.
func (q *QuestEngine) startClockLocked(c *questChar, qu *database.Quest, e database.QuestEvent, now time.Time) {
	t := q.trialLocked(c, qu.ID, true)
	t.Deadline = now.Add(time.Duration(e.Minutes * float64(time.Minute)))
	t.Until, t.Back, t.Factions, t.Regain, t.Says = e.Until, e.Back, e.Factions, e.Regain, e.Text
	delete(c.clockTold, qu.ID)
	q.saveTrialLocked(c, t)
	log.Printf("Quests: character %d's %q: a clock of %g minutes to step %d", c.serverID, qu.Name, e.Minutes, e.Until)
}

// stopClockLocked takes a quest's clock away (its standing owed stays).
func (q *QuestEngine) stopClockLocked(c *questChar, quest int) {
	t := q.trialLocked(c, quest, false)
	if t == nil || t.Deadline.IsZero() {
		return
	}
	t.Deadline, t.Until, t.Back, t.Factions, t.Regain, t.Says = time.Time{}, 0, 0, "", false, ""
	delete(c.clockTold, quest)
	q.saveTrialLocked(c, t)
}

// checkTrials runs the clocks and the wards of the characters in the world
// (once a second): a clock whose quest reached its step, or left the trial, is
// stopped; one whose time is up is the setback; one running says what is left
// at five minutes and at one. A player on a step that wards a place and
// standing in it has the setback.
func (q *QuestEngine) checkTrials(now time.Time) {
	if q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		sess := q.sessionOf(id)
		if sess == nil {
			continue
		}
		p := q.playerOf(sess)
		if p == nil {
			continue
		}
		for quest, t := range q.trialsLocked(c) {
			if t.Deadline.IsZero() {
				continue
			}
			qu, cq := q.cat.byID[quest], c.quests[quest]
			if qu == nil || cq == nil || !cq.Active() || cq.Step >= t.Until || cq.Step < t.Back {
				q.stopClockLocked(c, quest) // done in time, or gone from the trial
				continue
			}
			left := t.Deadline.Sub(now)
			if left <= 0 {
				log.Printf("Quests: %s's clock in %q ran out at step %d", p.Name, qu.Name, cq.Step)
				says := t.Says
				if strings.TrimSpace(says) == "" {
					says = fmt.Sprintf(questClockUp, qu.Name)
				}
				q.setbackLocked(sess, p, c, qu, cq, setback{back: t.Back, factions: database.ParseFactionDeltas(t.Factions),
					regain: t.Regain, says: says}, "its time ran out")
				continue
			}
			q.clockWarnLocked(sess, c, qu, left)
		}
		q.checkWardsLocked(sess, p, c)
	}
}

// clockWarnLocked says the minutes left as they pass five and then one.
func (q *QuestEngine) clockWarnLocked(sess *session.Session, c *questChar, qu *database.Quest, left time.Duration) {
	mins := int((left + time.Minute - 1) / time.Minute) // started minutes left
	at := 0
	switch {
	case mins <= 1:
		at = 1
	case mins <= questClockWarnAt:
		at = questClockWarnAt
	default:
		return
	}
	if c.clockTold == nil {
		c.clockTold = map[int]int{}
	}
	if told, ok := c.clockTold[qu.ID]; ok && told <= at {
		return
	}
	c.clockTold[qu.ID] = at
	if at == 1 {
		SendClientMessage(sess, fmt.Sprintf(questClockLeft1, qu.Name))
	} else {
		SendClientMessage(sess, fmt.Sprintf(questClockLeft, qu.Name, at))
	}
}

// checkWardsLocked is the setback of a player standing in a place its step
// wards.
func (q *QuestEngine) checkWardsLocked(sess *session.Session, p *entity.Player, c *questChar) {
	if q.gw == nil {
		return
	}
	world, x, _, z, alive := q.gw.PlayerPlace(p)
	if !alive {
		return
	}
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil {
			continue
		}
		for _, e := range database.EventsOn(st.Events, database.EventStart) {
			if e.Do != database.EventWard || int(world) != e.World || e.Radius <= 0 {
				continue
			}
			if dx, dz := x-e.X, z-e.Z; dx*dx+dz*dz > e.Radius*e.Radius {
				continue
			}
			log.Printf("Quests: %s came into step %d's ward of %q", p.Name, st.Step, qu.Name)
			q.setbackLocked(sess, p, c, qu, cq, setback{back: e.Back, factions: database.ParseFactionDeltas(e.Factions),
				regain: e.Regain, speaker: e.NPC, says: e.Text}, "it came into a ward")
			break
		}
	}
}

// backEventLocked is a back event (a give or show step's, on lacking).
func (q *QuestEngine) backEventLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, e database.QuestEvent) {
	cq := c.quests[qu.ID]
	if cq == nil || !cq.Active() {
		return
	}
	q.setbackLocked(sess, p, c, qu, cq, setback{back: e.Back, factions: database.ParseFactionDeltas(e.Factions),
		regain: e.Regain, speaker: e.NPC, says: e.Text}, "it lacked what its step needs")
}

// optionSetbackLocked is a choice's option with back picked: its words, then
// the setback.
func (q *QuestEngine) optionSetbackLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep, opt *database.StepOption) {
	q.tellLocked(c, sess, st.Target, sayLines(opt.Says)...)
	q.setbackLocked(sess, p, c, qu, cq, setback{back: opt.Back, factions: opt.Factions, regain: opt.Regain},
		fmt.Sprintf("it answered %q at step %d", opt.Answer, st.Step))
}

// optionFactionsLocked changes the standings an option names (one without back).
func (q *QuestEngine) optionFactionsLocked(sess *session.Session, p *entity.Player, opt *database.StepOption) {
	if len(opt.Factions) == 0 || q.gw == nil || p == nil {
		return
	}
	hits := make([]entity.FactionHit, len(opt.Factions))
	for i, f := range opt.Factions {
		hits[i] = entity.FactionHit{Faction: f.Faction, Delta: f.Delta}
	}
	q.gw.ApplyFactionHits(sess, p, hits)
}

// setbackLocked puts the quest back to sb.back (the file's doc): its words, the
// standings changed (and owed, with regain), the steps passed back over undone,
// the spawns and the clock gone, the journal's line the step's. A back not
// before the step to do leaves the quest where it is. Must be called with q.mu
// held.
func (q *QuestEngine) setbackLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, sb setback, why string) {
	at := cq.Step
	if s := strings.TrimSpace(sb.says); s != "" {
		q.tellLocked(c, sess, sb.speaker, sayLines(s)...)
	}
	// the standings: what is lost now is owed before the step is done again
	var owes []database.FactionReq
	if len(sb.factions) > 0 && q.gw != nil && p != nil {
		look := q.lookAt(sess)
		hits := make([]entity.FactionHit, 0, len(sb.factions))
		for _, f := range sb.factions {
			if sb.regain && f.Delta < 0 {
				if v, ok := look.standing(f.Faction); ok {
					owes = append(owes, database.FactionReq{Faction: f.Faction, Min: v})
				}
			}
			hits = append(hits, entity.FactionHit{Faction: f.Faction, Delta: f.Delta})
		}
		q.gw.ApplyFactionHits(sess, p, hits)
	}
	q.stopClockLocked(c, qu.ID)
	if len(owes) > 0 {
		t := q.trialLocked(c, qu.ID, true)
		t.Owes, t.OweStep = database.FormatFactionReqs(owes), sb.back
		q.saveTrialLocked(c, t)
	}
	q.clearSpawnsLocked(c.serverID, qu.ID, 0)
	if sb.back < 1 || sb.back > at || qu.Step(sb.back) == nil {
		log.Printf("Quests: %s's %q: a setback (%s) to step %d, at step %d: the quest stays", playerName(p), qu.Name, why, sb.back, at)
		return
	}
	// the steps passed back over undone: what they gave taken back (when held),
	// what they took handed back
	var handBack database.ClassReward
	var takeBack []database.ItemCount
	class := 0
	if p != nil {
		class = p.Class
	}
	for _, st := range stepsInOrder(qu) {
		if st.Step < sb.back || st.Step >= at {
			continue
		}
		if st.Kind == database.StepGive {
			handBack.Items = append(handBack.Items, st.Items...)
			handBack.Tunar += st.Tunar
		}
		for _, e := range database.EventsOn(st.Events, database.EventDone) {
			if e.Do == database.EventGive {
				takeBack = append(takeBack, database.ParseItemCounts(e.Items)...)
			}
		}
		takeBack = append(takeBack, st.RewardFor(class).Items...)
	}
	var lines []string
	for _, it := range takeBack {
		one := &database.QuestStep{Kind: database.StepGive, Items: []database.ItemCount{it}}
		if p == nil || q.lackingLocked(p, one) != "" {
			continue // not held: nothing to take
		}
		if given, ok := q.takeLocked(sess, p, one); ok && given != nil {
			for _, n := range given.names {
				lines = append(lines, fmt.Sprintf(questTakenBack, n))
			}
		}
	}
	idx := c.index(qu.ID)
	old := q.lineLocked(cq)
	cq.Step, cq.Count = sb.back, 0
	if text := q.lineLocked(cq); text != old || idx < 0 {
		q.dropLineLocked(sess, c, idx)
		cq.Slot = c.nextSlot()
		q.addLineLocked(sess, c, text)
	}
	q.saveLocked(cq)
	lines = append(lines, q.rewardLocked(sess, p, handBack, economy.QuestRefund)...)
	q.tellLocked(c, sess, "", lines...)
	log.Printf("Quests: %s's %q put back from step %d to step %d: %s (owes %v; taken back %d, handed back %d items)",
		playerName(p), qu.Name, at, sb.back, why, owes, len(takeBack), len(handBack.Items))
}

// owesRefusalLocked is what the NPC of the step a setback put the quest back to
// says while the standing it took is not earned back ("" when the step may be
// done: none owed, or owed and met, the debt then forgotten).
func (q *QuestEngine) owesRefusalLocked(sess *session.Session, c *questChar, qu *database.Quest, st *database.QuestStep) string {
	t := q.trialLocked(c, qu.ID, false)
	if t == nil || t.Owes == "" || st.Step != t.OweStep {
		return ""
	}
	look := q.lookAt(sess)
	for _, r := range database.ParseFactionReqs(t.Owes) {
		if v, ok := look.standing(r.Faction); ok && !r.Met(v) {
			return fmt.Sprintf(questOwesLine, r.Faction)
		}
	}
	t.Owes, t.OweStep = "", 0
	q.saveTrialLocked(c, t)
	return ""
}
