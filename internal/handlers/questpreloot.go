package handlers

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// questpreloot.go — a quest's items drop before its step (the owner, 2026-11-14: "allow
// pre-looting for all quests"; Live petition 5016, Odium: the fire bat wings of Neriak's level 5
// quest could not be had before the step).
//
// Until then a loot step's item dropped only for a character whose journal was on that step
// (questevents.go lootKillLocked). Now a kill of a loot step's creature (stepNPC: its target or
// its other creatures) also drops the step's item, at the step's own chance, for each player the
// kill counts for who is not on that step yet: one who has not taken the quest, or is on an
// earlier step of it. Not for one:
//   - past the step (it did it), or who has finished the quest and it is not repeatable;
//   - whose class or race the quest is not for, or above the quest's top level (it never could);
//   - who holds as many of the item as the step wants already (its Ascended records counted, as
//     a quest counts them), or one of a Lore item;
//   - for a draft quest.
// One kill drops one of an item for a player at most: a second quest's step on the same creature
// and item rolls nothing more, nor does the step the player is on.
//
// A loot step that becomes the step to do (stepStartedLocked: taking the quest, or the step
// before done) counts the items already in the pack, up to what it wants, and is done at once
// when they are enough (lootStepHeldLocked). Taking items off a corpse while on the step counts
// as ever (Looted).

// lootRef is one enabled quest's loot step.
type lootRef struct {
	qu *database.Quest
	st *database.QuestStep
}

// lootStepsFor is every enabled quest's loot step a creature's kill matches.
func (q *QuestEngine) lootStepsForLocked(name string, spawn entity.EntityID) []lootRef {
	var out []lootRef
	for _, qu := range q.cat.all {
		if !qu.Enabled {
			continue
		}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			if st.Kind == database.StepLoot && stepNPC(st, name, spawn) {
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
	for _, ref := range q.lootStepsForLocked(k.NPCName, k.NPCSpawn) {
		qu, st := ref.qu, ref.st
		pid, need, ok := st.LootItem()
		if !ok || rolled[pid] {
			continue
		}
		if cq := c.quests[qu.ID]; cq != nil {
			if cq.Active() && cq.Step >= st.Step {
				continue // on the step (lootKillLocked's) or past it
			}
			if !cq.Active() && cq.Done > 0 && !qu.Repeatable {
				continue
			}
		}
		if len(qu.Classes) > 0 && !containsInt(qu.Classes, p.Class) || len(qu.Races) > 0 && !questRaceTakes(qu.Races, p.Race) ||
			qu.MaxLevel > 0 && p.Level > qu.MaxLevel {
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
		if pat.Lore != 0 && held > 0 {
			continue
		}
		rolled[pid] = true
		if q.roll() >= database.LootChance(st.Chance) {
			continue
		}
		if fullStack(sessionInvList(kp.Session), pat) {
			continue
		}
		q.drop(k.NPCID, k.Died, kp.ServerID, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat})
		log.Printf("Quests: %q dropped %q for %s ahead of step %d of %q", k.NPCName, pat.ItemName, kp.Name, st.Step, qu.Name)
	}
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
