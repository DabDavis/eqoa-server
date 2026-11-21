package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questpreloot.go — a quest's loot step items on a corpse.
//
// A kill of a loot step's creature (stepKill: its target, its other creatures, an area's any
// creature) puts the step's item on the corpse ONCE, for whoever may loot it (questloot.go's
// shared drops: the kill group, then anyone once the corpse unlocks), whether or not anyone
// of the kill is on the quest (the owner, 2026-11-21: "players dont need to be on a quest to
// obtain a quest drop ... no instanced quest items for each character ... remove lore from
// quest stacks like madmen blood"). It is one item, taken by one looter like any other loot:
// a group of four on the step sees one, not one each. The looter's own loot step counts it
// as it is taken (Looted); a pack's items count when their loot step becomes the step to do
// (lootStepHeldLocked); a give or show step takes or looks at what the pack holds, as ever.
//
// Before, each player of the kill on the step had a drop of its own on its own view of the
// corpse (the owner, 2026-11-14: "allow pre-looting for all quests"; 2026-11-18: "do not
// require quest at all, just drops"), and a player not on the step one only while it held
// fewer than the step wanted, no Lore one held and no full stack.
//
// One roll per distinct item per corpse (corpseDropsLocked): several quests' steps naming the
// same creature and item (the five quests of Ilenar Crelwin that each want a desert
// madman's blood) roll it once, at the highest of their chances (database.LootChance: a
// chance of 0 is every kill). A step that drops to anyone (openDropLocked) counts; a step
// that does not counts only when a player of the kill is on it, or would have had it ahead of
// it (aheadRefusalLocked, aheadExcludedLocked: the rules below, kept from before).
//
// Not to anyone, where a drop to anyone would break the step's design (those still drop for a
// player of the kill on the step, and ahead of it for one the old rules let):
//   - an area's any-creature step (target '*': every kill in the area is one, the owner
//     2026-11-17: Lord Valkanith's emerald "in this city only"): its quest under way;
//   - an item that an earlier loot or give step of the quest, not yet done, wants too (the
//     Spider Queen Aszec's seven Crystal Spider Shards, one off each of seven named spiders in
//     turn; the Split Ruby's halves): the pack cannot say which creature an item came from, so
//     Fengzel's shard had ahead would do Grosp's step 1 at once and Fengzel's step 3 want
//     another, and the order the quest walks is lost;
//   - a creature the quest brings (a spawn event of any of its steps or of its taking: the
//     Siliskor quest's mirrored soul and its Scepter of Reflection, Jarek's Creation) or one that
//     stands in no world (npcspawnonly: what a trigger raises, Kratas for the harpoon of
//     Telinan): its item is the encounter's, had by fighting the one the step brings. Except for
//     a player on the step that brought that creature (Harbingers of Khalimet, petition 2098:
//     Armis, spawned by step 3, drops step 4's wand on the same kill);
//   - a step whose doing fires a trigger, a spawn or an attack (its done events): those happen
//     where the step is done, and a step done at its start (the items held) would raise them
//     wherever the player stood, at the giver in town;
//   - a key an event reads in the pack (raidportal.go's Tallon's Key, whose one bearer closes
//     the portal behind the raid: one key a raid; skypass.go's isles' keys) and a watched item
//     (a creature's aggro looks for it, a wall or a door opens for it: entity/questwalls.go).
//   - a disabled quest's step: for a player on it alone.
//
// An item dropped with no player of the kill on its step keeps the corpse only aheadKeep
// (questloot.go: Live petition 5118, dead wildlife holding Ant Chitin for no one); one a player
// of the kill is on the step for keeps it as any loot does. A kill no player is credited with
// drops nothing. The tracker (questprogress.go) shows the items held ahead of a later loot
// step of a quest under way; a kill of a later step's creature says that its item drops
// (laterStepKillLocked).

// lootRef is one quest's loot step.
type lootRef struct {
	qu *database.Quest
	st *database.QuestStep
}

// lootStepsForLocked is every quest's loot step a creature's kill matches (stepKill: an
// AnyCreature step's area too), a disabled quest's included (its players on the step).
func (q *QuestEngine) lootStepsForLocked(k entity.KillCredit) []lootRef {
	var out []lootRef
	for _, qu := range q.cat.all {
		for i := range qu.Steps {
			st := &qu.Steps[i]
			if st.Kind == database.StepLoot && stepKill(st, k) {
				out = append(out, lootRef{qu, st})
			}
		}
	}
	return out
}

// noQuests is a character with no quest at all: the one a drop to anyone is judged for.
var noQuests = &questChar{}

// openDropLocked is whether a loot step's item drops on a kill of the creature named killed
// for anyone, no quest asked (the file's doc).
func (q *QuestEngine) openDropLocked(qu *database.Quest, st *database.QuestStep, pat *database.ItemPatternRecord, killed string) bool {
	return qu.Enabled && q.aheadRefusalLocked(noQuests, qu, st, killed) == "" && q.aheadExcludedLocked(qu, st, pat) == ""
}

// onLootStep is whether a character is on a loot step and wants more of its item.
func onLootStep(c *questChar, qu *database.Quest, st *database.QuestStep) bool {
	cq := c.quests[qu.ID]
	_, need, ok := st.LootItem()
	return ok && cq != nil && cq.Active() && cq.Step == st.Step && cq.Count < need
}

// questDrop is one item a kill's corpse rolls: the highest chance of the steps that drop it,
// and whether a player of the kill is on one of them.
type questDrop struct {
	pat    *database.ItemPatternRecord
	chance float64
	onStep bool
	why    string // a step that drops it, for the log
	seq    int    // its place among the kill's items: the steps' order
	// counted is a player on the step with a full stack of it counted it at the kill
	// (lootKillLocked): it goes on no corpse
	counted bool
}

// corpseDropsLocked rolls the loot steps' items of a kill, each item once (the file's doc),
// chars being the kill's players' quests (by k.Players' order): the items that dropped, by
// pattern id, for placeDropsLocked to put on the corpse once the kill's steps are counted.
func (q *QuestEngine) corpseDropsLocked(k entity.KillCredit, chars []*questChar) map[int]*questDrop {
	if q.drop == nil || k.NPCID == 0 || len(k.Players) == 0 {
		return nil
	}
	var order []int
	byItem := map[int]*questDrop{}
	pats := map[int]*database.ItemPatternRecord{}
	for _, ref := range q.lootStepsForLocked(k) {
		qu, st := ref.qu, ref.st
		pid, _, ok := st.LootItem()
		if !ok {
			continue
		}
		pat, seen := pats[pid]
		if !seen {
			if p, err := q.items.GetItemPattern(pid); err == nil {
				pat = p
			} else {
				log.Printf("Quests: %q step %d's item %d: %v", qu.Name, st.Step, pid, err)
			}
			pats[pid] = pat
		}
		if pat == nil {
			continue
		}
		drops, onStep := q.openDropLocked(qu, st, pat, k.NPCName), false
		for _, c := range chars {
			switch {
			case onLootStep(c, qu, st):
				drops, onStep = true, true
			case !drops && qu.Enabled && q.aheadRefusalLocked(c, qu, st, k.NPCName) == "" && q.aheadExcludedLocked(qu, st, pat) == "":
				drops = true
			}
		}
		if !drops {
			continue
		}
		d := byItem[pid]
		if d == nil {
			d = &questDrop{pat: pat, seq: len(order)}
			byItem[pid] = d
			order = append(order, pid)
		}
		if ch := database.LootChance(st.Chance); ch > d.chance { // in (0, 1]: the first sets it
			d.chance, d.why = ch, fmt.Sprintf("step %d of %q", st.Step, qu.Name)
		}
		d.onStep = d.onStep || onStep
	}
	for _, pid := range order {
		if q.roll() >= byItem[pid].chance {
			delete(byItem, pid)
		}
	}
	return byItem
}

// placeDropsLocked puts a kill's rolled items (corpseDropsLocked) on its corpse, each once,
// but one a full stack counted at the kill (lootKillLocked).
func (q *QuestEngine) placeDropsLocked(k entity.KillCredit, dropped map[int]*questDrop) {
	ids := make([]int, 0, len(dropped))
	for pid := range dropped {
		ids = append(ids, pid)
	}
	sort.Slice(ids, func(i, j int) bool { return dropped[ids[i]].seq < dropped[ids[j]].seq })
	for _, pid := range ids {
		d := dropped[pid]
		if d.counted {
			continue
		}
		q.drop(k.NPCID, k.Died, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *d.pat}, !d.onStep)
		log.Printf("Quests: %q dropped %q on its corpse (%s; a player on the step: %v)", k.NPCName, d.pat.ItemName, d.why, d.onStep)
	}
}

// aheadRefusalLocked is why a loot step's item does not drop ahead of its step for the player
// on a kill of the creature named killed, or "" when it does (the file's doc: whose quest, and
// which steps). Not the item's own tests (aheadExcludedLocked, the pack's holding).
func (q *QuestEngine) aheadRefusalLocked(c *questChar, qu *database.Quest, st *database.QuestStep, killed string) string {
	pid, _, ok := st.LootItem()
	if !ok || st.Kind != database.StepLoot {
		return "not a loot step"
	}
	cq := c.quests[qu.ID]
	underWay := cq != nil && cq.Active()
	from := 0 // the step the player stands at: every step from it to st is still to do
	if underWay {
		if cq.Step == st.Step || cq.Step > st.Step && !qu.Repeatable {
			return "on the step or past it"
		}
		if cq.Step > st.Step {
			// a repeatable quest past its loot step farms ahead for its next round (the
			// owner, 2026-11-18, petition 2383): its later steps take what they need
			return ""
		}
		from = cq.Step
	} else if cq != nil && cq.Done > 0 && !qu.Repeatable {
		return "finished, and it does not repeat"
	} else if st.AnyInArea() {
		// an area's any creature (questanyarea_test.go, the owner 2026-11-17: Lord Valkanith's
		// emerald "in this city only") drops for its quest's players alone: every kill there is
		// one, and one who has not taken the quest is not hunting for it
		return "an area's step, the quest not under way"
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
		if sp.Key != "" && strings.EqualFold(sp.Key, name) {
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
