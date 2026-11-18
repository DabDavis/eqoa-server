package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questpreloot.go — a quest's items drop before its step (the owner, 2026-11-14: "allow
// pre-looting for all quests"; Live petition 5016, Odium: the fire bat wings of Neriak's level 5
// quest could not be had before the step; the owner again, 2026-11-18: "allow pre farming of
// quest items").
//
// A loot step's item drops for a character on that step (questevents.go lootKillLocked). A kill
// of a loot step's creature (stepNPC: its target or its other creatures) also drops the step's
// item, at the step's own chance, for each player the kill counts for who is not on that step
// yet, each on its own view of the corpse (questloot.go), by these rules (aheadRefusalLocked):
//   - the quest under way at an earlier step: any later loot step's item;
//   - the quest not taken (or a repeatable one finished): only while the player may take it now
//     (mayTakeLocked: its level, class, race, the quests it requires, its twins, its factions,
//     its day group), and only a loot step that no step but talks and other loot steps comes
//     before (the giver's own errand, what is asked at once: "kill bats, bring me four wings").
//     A quest's later loot steps, past a hand-in, a kill or a place, wait for the quest: every
//     creature of the world is some quest's later step, and their items would litter the bags
//     of players who never take those quests (a Lore item one of each, never sold back);
//   - never: past the step, a finished quest that does not repeat, a draft, holding as many as
//     the step wants already (its Ascended records counted, as a quest counts them, less what
//     the quest's earlier loot steps keep: keptForEarlier), one of a Lore item, a full stack.
// One kill drops one of an item for a player at most: a second quest's step on the same creature
// and item rolls nothing more, nor does the step the player is on.
//
// Not ahead, where a drop ahead would break the step's design (aheadExcludedLocked):
//   - an item that an earlier loot or give step of the quest, not yet done, wants too (the
//     Spider Queen Aszec's seven Crystal Spider Shards, one off each of seven named spiders in
//     turn; the Split Ruby's halves): the pack cannot say which creature an item came from, so
//     Fengzel's shard taken ahead would do Grosp's step 1 at once and Fengzel's step 3 want
//     another, and the order the quest walks is lost;
//   - a creature the quest brings (a spawn event of any of its steps or of its taking: the
//     Siliskor quest's mirrored soul and its Scepter of Reflection, Jarek's Creation) or one that
//     stands in no world (npcspawnonly: what a trigger raises, Kratas for the harpoon of
//     Telinan): its item is the encounter's, had by fighting the one the step brings; a drop off
//     another player's copy would do the step at its start, its own spawn standing for no one.
//     Except for a player on the step that brought that creature (Harbingers of Khalimet,
//     petition 2098: Armis, spawned by step 3, drops step 4's wand on the same kill);
//   - a step whose doing fires a trigger, a spawn or an attack (its done events): those happen
//     where the step is done, and a step done at its start (the items held) would raise them
//     wherever the player stood, at the giver in town;
//   - a key an event reads in the pack (raidportal.go's Tallon's Key, whose one bearer closes
//     the portal behind the raid: one key a raid, a drop ahead would make a second bearer;
//     skypass.go's isles' keys) and a watched item (a creature's aggro looks for it, a wall or a
//     door opens for it: entity/questwalls.go): holding it does something before the step.
//
// A loot step that becomes the step to do (stepStartedLocked: taking the quest, or the step
// before done) counts the items already in the pack, up to what it wants, and is done at once
// when they are enough (lootStepHeldLocked); a give or show step takes or looks at what the pack
// holds, as ever. Taking items off a corpse while on the step counts as ever (Looted). The
// tracker (questprogress.go) shows the items held ahead of a later loot step of a quest under
// way; a kill of a later step's creature says that its item drops ahead (laterStepKillLocked).

// lootRef is one enabled quest's loot step.
type lootRef struct {
	qu *database.Quest
	st *database.QuestStep
}

// lootStepsFor is every enabled quest's loot step a creature's kill matches
// (stepKill: an AnyCreature step's area too).
func (q *QuestEngine) lootStepsForLocked(k entity.KillCredit) []lootRef {
	var out []lootRef
	for _, qu := range q.cat.all {
		if !qu.Enabled {
			continue
		}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			if st.Kind == database.StepLoot && stepKill(st, k) {
				out = append(out, lootRef{qu, st})
			}
		}
	}
	return out
}

// preLootLocked drops the loot steps' items of the kill for one player it counts for who is not
// on those steps yet (the file's doc); rolled holds the items already rolled for the player on
// this kill (the step it is on).
func (q *QuestEngine) preLootLocked(k entity.KillCredit, kp entity.KillPlayer, c *questChar, rolled map[int]bool) {
	if q.drop == nil || k.NPCID == 0 || kp.Session == nil || q.playerOf == nil {
		return
	}
	p := q.playerOf(kp.Session)
	if p == nil {
		return
	}
	look := q.lookAt(kp.Session)
	for _, ref := range q.lootStepsForLocked(k) {
		qu, st := ref.qu, ref.st
		pid, need, ok := st.LootItem()
		if !ok || rolled[pid] {
			continue
		}
		if q.aheadRefusalLocked(look, p, c, qu, st, k.NPCName) != "" {
			continue
		}
		held := sessionInvList(kp.Session).heldStandingFor(pid)
		if held-keptForEarlier(qu, st, pid) >= need {
			continue
		}
		pat, err := q.items.GetItemPattern(pid)
		if err != nil || pat == nil {
			continue
		}
		if pat.Lore != 0 && held > 0 || q.aheadExcludedLocked(qu, st, pat) != "" {
			continue
		}
		rolled[pid] = true
		if q.roll() >= database.LootChance(st.Chance) {
			continue
		}
		if fullStack(sessionInvList(kp.Session), pat) {
			continue
		}
		drop := q.drop
		if q.dropAhead != nil {
			drop = q.dropAhead
		}
		drop(k.NPCID, k.Died, kp.ServerID, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat})
		log.Printf("Quests: %q dropped %q for %s ahead of step %d of %q", k.NPCName, pat.ItemName, kp.Name, st.Step, qu.Name)
	}
}

// aheadRefusalLocked is why a loot step's item does not drop ahead of its step for the player
// on a kill of the creature named killed, or "" when it does (the file's doc: whose quest, and
// which steps). Not the item's own tests (aheadExcludedLocked, the pack's holding).
func (q *QuestEngine) aheadRefusalLocked(look *factionLook, p *entity.Player, c *questChar, qu *database.Quest, st *database.QuestStep, killed string) string {
	pid, _, ok := st.LootItem()
	if !ok || st.Kind != database.StepLoot {
		return "not a loot step"
	}
	cq := c.quests[qu.ID]
	underWay := cq != nil && cq.Active()
	from := 0 // the step the player stands at: every step from it to st is still to do
	if underWay {
		if cq.Step >= st.Step {
			return "on the step or past it"
		}
		from = cq.Step
	} else {
		if st.AnyInArea() {
			// an area's any creature drops it for its quest's players alone: every kill
			// there is one, and one who has not taken the quest is not hunting for it
			return "an area's step, the quest not under way"
		}
		if why := q.mayTakeLocked(look, p, c, qu); why != "" {
			return why
		}
		for i := range qu.Steps {
			if s := &qu.Steps[i]; s.Step < st.Step && s.Kind != database.StepTalk && s.Kind != database.StepLoot {
				return fmt.Sprintf("not taken, and step %d (%s) comes first", s.Step, s.Kind)
			}
		}
	}
	for i := range qu.Steps {
		s := &qu.Steps[i]
		if s.Step < from || s.Step >= st.Step {
			continue
		}
		switch s.Kind {
		case database.StepLoot:
			if spid, _, ok := s.LootItem(); ok && spid == pid {
				return fmt.Sprintf("step %d, still to do, wants the item too", s.Step)
			}
		case database.StepGive:
			for _, it := range s.Items {
				if it.PatternID == pid {
					return fmt.Sprintf("step %d, still to do, takes the item", s.Step)
				}
			}
		}
	}
	if q.questBringsLocked(qu, st) {
		if !underWay {
			return "its creature is the quest's encounter"
		}
		_, names := stepSpawns(qu, qu.Step(cq.Step))
		mine := false
		for _, n := range names {
			mine = mine || foldName(n) == foldName(killed)
		}
		if !mine {
			return "its creature is the quest's encounter, brought by another step"
		}
	}
	return ""
}

// questBringsLocked is whether a loot step's creature is one its quest brings (a spawn event
// of the quest's taking or of any of its steps names it) or one that stands in no world
// (npcspawnonly: a trigger's, a quest's original).
func (q *QuestEngine) questBringsLocked(qu *database.Quest, st *database.QuestStep) bool {
	for _, t := range st.Targets() {
		if t == database.AnyCreature {
			continue
		}
		if q.gw != nil && q.gw.SpawnOnlyNamed(t) {
			return true
		}
		names := func(evs []database.QuestEvent) bool {
			for _, e := range evs {
				if e.Do == database.EventSpawn && foldName(e.NPC) == foldName(t) {
					return true
				}
			}
			return false
		}
		if names(qu.Events) {
			return true
		}
		for i := range qu.Steps {
			if names(qu.Steps[i].Events) {
				return true
			}
		}
	}
	return false
}

// aheadExcludedLocked is why a loot step's item never drops ahead of its step for anyone, or ""
// (the file's doc): the step's doing raises something where it is done, or the item is a key
// an event reads in the pack, or a watched one.
func (q *QuestEngine) aheadExcludedLocked(qu *database.Quest, st *database.QuestStep, pat *database.ItemPatternRecord) string {
	for _, e := range database.EventsOn(st.Events, database.EventDone) {
		switch e.Do {
		case database.EventTrigger, database.EventSpawn, database.EventAttack:
			return "its doing fires a " + e.Do
		}
	}
	if eventKey(pat.ItemName) {
		return "a key an event reads"
	}
	if q.gw != nil {
		for _, id := range q.gw.WatchedItems() {
			if id == pat.PatternID {
				return "a watched item"
			}
		}
	}
	return ""
}

// eventKey is whether an item, by name, is a key an event reads in the pack: a raid portal's
// (raidportal.go) or an isle's passage's (skypass.go).
func eventKey(name string) bool {
	name = strings.TrimSpace(name)
	for _, rp := range raidPortals {
		if strings.EqualFold(rp.Key, name) {
			return true
		}
	}
	for _, sp := range skyPasses {
		if strings.EqualFold(sp.Key, name) {
			return true
		}
	}
	return false
}

// aheadHeld is the items held ahead of a quest's later loot steps for a player on it: each
// step's item, the pack's holding less what the earlier steps keep (keptForEarlier), up to what
// the step wants; only those with one held at least (the tracker's, questprogress.go).
func aheadHeld(l *invList, qu *database.Quest, cq *database.CharQuest) []progressAhead {
	var out []progressAhead
	for i := range qu.Steps {
		s := &qu.Steps[i]
		if s.Kind != database.StepLoot || s.Step <= cq.Step {
			continue
		}
		pid, need, ok := s.LootItem()
		if !ok {
			continue
		}
		if n := min(l.heldStandingFor(pid)-keptForEarlier(qu, s, pid), need); n > 0 {
			out = append(out, progressAhead{pattern: pid, have: n, need: need})
		}
	}
	return out
}

// progressAhead is one later loot step's items held ahead of it.
type progressAhead struct {
	pattern, have, need int
}

// keptForEarlier is how many of a loot step's item the earlier loot steps of its quest want
// that no give step between them and it hands in: the pack's items of that number are theirs,
// kept for the hand-in that wants them all, not the step's own. The Split Ruby (Oggok, 100535):
// a ruby half off a wandering forager at step 1, one off a fat piranha at step 2, the two given
// to Oligan at step 4; step 2 counted step 1's half as its own when it started, was done at
// once, and the hand-in wanted a second half the quest would never drop again.
func keptForEarlier(qu *database.Quest, st *database.QuestStep, pid int) int {
	n := 0
	for i := range qu.Steps {
		s := &qu.Steps[i]
		if s.Step >= st.Step {
			continue
		}
		switch s.Kind {
		case database.StepLoot:
			if p, need, ok := s.LootItem(); ok && p == pid {
				n += need
			}
		case database.StepGive:
			for _, it := range s.Items {
				if it.PatternID == pid {
					n = max(0, n-max(it.Count, 1))
				}
			}
		}
	}
	return n
}

// lootStepHeldLocked counts, on a loot step become the step to do, the step's items the pack
// holds already (up to what it wants), and does the step when they are enough (the file's doc).
// It reports whether the step was done.
func (q *QuestEngine) lootStepHeldLocked(sess *session.Session, p *entity.Player, c *questChar, qu *database.Quest, cq *database.CharQuest, st *database.QuestStep) bool {
	if st.Kind != database.StepLoot || sess == nil || p == nil || !cq.Active() || cq.Step != st.Step {
		return false
	}
	pid, need, ok := st.LootItem()
	if !ok {
		return false
	}
	held := min(max(0, sessionInvList(sess).heldStandingFor(pid)-keptForEarlier(qu, st, pid)), need)
	if held <= cq.Count {
		return false
	}
	log.Printf("Quests: %s holds %d of step %d of %q's item %d already: counted", p.Name, held, st.Step, qu.Name, pid)
	if held < need {
		cq.Count = held
		q.saveLocked(cq)
		name := fmt.Sprintf("item %d", pid)
		if pat, err := q.items.GetItemPattern(pid); err == nil && pat != nil {
			name = pat.ItemName
		}
		SendClientMessage(sess, fmt.Sprintf("%s: %d of %d.", name, cq.Count, need))
		return false
	}
	q.countLootLocked(sess, p, c, qu, cq, st, held-cq.Count)
	return true
}
