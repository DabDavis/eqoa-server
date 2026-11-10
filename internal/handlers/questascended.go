package handlers

import (
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Ascended items in quests (database/ascended.go; petition 1446, the GM
// Jibrish: "For quest items that produce gear that they drop, add an option
// at the end of that normal quest line where they can turn in the ascended
// version instead and get a +10% version, but they still have to do all steps
// of the quest. AKA if they have ring of veeshan or ascended ring of veeshan,
// its the same exact quest but the NPC accepts either of those rings at the
// end and gives you the ascended version if thats what you have"). For every
// quest, not one by one:
//
//   - a show or give step's item is met by the record or by its Ascended
//     record, one for one (a stack of either counts);
//   - the step "comes Ascended" when it wants at least one record that has an
//     Ascended record and the pack's Ascended records alone meet every such
//     record's count: a give step then takes those (the Ascended first), and
//     the step's reward gear (its rewards, its class's, the picked option's)
//     is given as its Ascended records, made by the rule when a reward has none
//     (ItemPatternStore.AscendedOf). Otherwise the regular records go first and
//     an Ascended one only makes up what they lack, and the rewards are the
//     regular ones: a mixed hand-in of one boss's ring and another's Ascended
//     gem gives the regular reward;
//   - a loot step counts an Ascended record taken off a corpse for its base.
//
// What a quest's events hand over (a give event) and its tunar, experience,
// factions and spells are never Ascended. Worn items and the bank are not
// handed in, as before.

// handIn is how a show or give step is met from the unworn pack: each row a
// give step takes and how many of it, what still lacks (lackingLocked's
// words), and whether the step comes Ascended.
type handIn struct {
	takes    []handInTake
	lack     []string
	ascended bool
}

// handInTake is one inventory row a give step takes from, and how many.
type handInTake struct {
	row database.CharInventoryEntry
	n   int
}

// handedIn is what doing a step was met with: the names of what was handed
// in (each said "You have given away ..."), and whether it came Ascended.
type handedIn struct {
	names    []string
	ascended bool
}

// ascendedMapLocked is table itemascended (copy -> base), read at each
// hand-in (a tool's new records count at once), else the World's copy
// (entity.AscendedItems) when the database is not at hand.
func (q *QuestEngine) ascendedMapLocked() map[int]int {
	if q.items == nil {
		return entity.AscendedItems()
	}
	m, err := q.items.AscendedMap()
	if err != nil {
		log.Printf("Quests: the Ascended records: %v", err)
		return entity.AscendedItems()
	}
	return m
}

// planHandInLocked is how the player's unworn pack meets a give step's items
// and tunar, and its pack and what it wears a show step's (shownItems).
func (q *QuestEngine) planHandInLocked(p *entity.Player, st *database.QuestStep) handIn {
	var plan handIn
	held := q.heldItems(p)
	if st.Kind == database.StepShow {
		held = q.shownItems(p)
	}
	asc := q.ascendedMapLocked()
	baseOf := func(id int) (int, bool) {
		b, ok := asc[id]
		return b, ok
	}
	hasAscended := map[int]bool{} // records an Ascended record was made of
	for _, b := range asc {
		hasAscended[b] = true
	}
	need, order := map[int]int{}, []int{}
	for _, it := range st.Items {
		if need[it.PatternID] == 0 {
			order = append(order, it.PatternID)
		}
		need[it.PatternID] += it.Count
	}
	own, standIn := map[int]int{}, map[int]int{} // units of the record itself, of its Ascended records
	for _, r := range held {
		own[r.Pattern.PatternID] += r.StackLeft
		if b, ok := baseOf(r.Pattern.PatternID); ok {
			standIn[b] += r.StackLeft
		}
	}
	some, all := false, true
	for _, id := range order {
		if !hasAscended[id] {
			continue
		}
		some = true
		all = all && standIn[id] >= need[id]
	}
	plan.ascended = some && all
	for _, id := range order {
		if have := own[id] + standIn[id]; have < need[id] {
			plan.lack = append(plan.lack, fmt.Sprintf("%d of item %d", need[id]-have, id))
		}
	}
	if p.Tunar < st.Tunar {
		plan.lack = append(plan.lack, fmt.Sprintf("%d tunar", st.Tunar-p.Tunar))
	}
	if len(plan.lack) > 0 || st.Kind != database.StepGive {
		return plan
	}
	left := make([]int, len(held))
	for i := range held {
		left[i] = held[i].StackLeft
	}
	for _, id := range order {
		n := need[id]
		take := func(match func(int) bool) {
			for i := range held {
				if n == 0 {
					return
				}
				if left[i] == 0 || !match(held[i].Pattern.PatternID) {
					continue
				}
				k := min(n, left[i])
				plan.takes = append(plan.takes, handInTake{row: held[i], n: k})
				left[i] -= k
				n -= k
			}
		}
		isOwn := func(pid int) bool { return pid == id }
		isAscended := func(pid int) bool { b, ok := baseOf(pid); return ok && b == id }
		if plan.ascended {
			take(isAscended)
			take(isOwn)
		} else {
			take(isOwn)
			take(isAscended)
		}
	}
	return plan
}

// invTakes is the plan's takes as the pack's room is worked out with them.
func (h handIn) invTakes() []invTake {
	out := make([]invTake, 0, len(h.takes))
	for _, t := range h.takes {
		out = append(out, invTake{itemID: t.row.ItemID, n: t.n})
	}
	return out
}

// ascendRewardLocked is a step's rewards as an Ascended hand-in gives them:
// each item of gear its Ascended record (made when it has none), the rest as
// they are; a record that cannot be made one is given as it is, and said.
func (q *QuestEngine) ascendRewardLocked(rw database.ClassReward, who string) database.ClassReward {
	if q.items == nil || len(rw.Items) == 0 {
		return rw
	}
	out := rw
	out.Items = make([]database.ItemCount, 0, len(rw.Items))
	made := false
	for _, it := range rw.Items {
		id, fresh, err := q.items.AscendedOf(it.PatternID, "quest reward")
		if err != nil {
			log.Printf("Quests: %s's Ascended reward of item %d not made, the regular one given: %v", who, it.PatternID, err)
			id = it.PatternID
		}
		if fresh {
			log.Printf("Quests: Ascended record %d made of item %d for %s's reward", id, it.PatternID, who)
			made = true
		}
		out.Items = append(out.Items, database.ItemCount{PatternID: id, Count: it.Count})
	}
	if made {
		if m, err := q.items.AscendedMap(); err == nil {
			entity.SetAscendedItems(m) // its proc and hit effect read as an Ascended record's at once
		}
	}
	return out
}

// ascendedFor reports whether record id is an Ascended record of base.
func ascendedFor(id, base int) bool {
	b, ok := entity.AscendedBase(id)
	return ok && b == base
}

// heldStandingFor is how many of an item the list holds counted as a quest
// counts it: its own stacks and its Ascended records' (the quest tracker's,
// questprogress.go).
func (l *invList) heldStandingFor(patternID int) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, r := range l.recs {
		if r.patternID == patternID || ascendedFor(r.patternID, patternID) {
			n += r.qty
		}
	}
	return n
}
