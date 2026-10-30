package handlers

import (
	"math/rand"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// rollsCategories is whether a creature's corpse rolls the loot categories:
// not a pet's, a charmed creature's or a training dummy's (lootNames' rule
// for the tables every creature rolls). A kill that gives no experience rolls
// them all the same (the user: "the no XP rule ignore though for the loot").
func rollsCategories(npc *entity.NPC) bool {
	return npc.Owner == 0 && npc.Charm == nil && npc.Dummy == entity.NotDummy
}

// rollCategories is what a creature's loot categories give its corpse at a
// death (database.CategoryRoll): each category on its own, either one roll
// at its rate (float, in [0, 1)) whose landing takes one item of its pool by
// weight (intn), a stackable one minstack to maxstack at a time (its own
// maxstack at most); or, with counts, a count drawn by those chances and
// that many items each drawn from the pool on its own, the same item's
// gathered into stacks of its maxstack. An empty pool gives nothing. float
// and intn are rand.Float64 and rand.Intn when nil.
func rollCategories(rolls []database.CategoryRoll, float func() float64, intn func(int) int) []database.CharInventoryEntry {
	if float == nil {
		float = rand.Float64
	}
	if intn == nil {
		intn = rand.Intn
	}
	var out []database.CharInventoryEntry
	for _, r := range rolls {
		total := 0
		for _, c := range r.Choices {
			total += max(c.Weight, 0)
		}
		if len(r.Counts) > 0 {
			n := drawCount(r.Counts, float())
			if n == 0 || total == 0 {
				continue
			}
			got := map[int]int{} // choice index -> how many
			var order []int
			for ; n > 0; n-- {
				i := pickChoice(r.Choices, intn(total))
				if got[i] == 0 {
					order = append(order, i)
				}
				got[i]++
			}
			for _, i := range order {
				item := r.Choices[i].Item
				for left, top := got[i], max(item.Pattern.MaxStack, 1); left > 0; left -= top {
					it := item
					it.StackLeft = min(left, top)
					out = append(out, it)
				}
			}
			continue
		}
		if r.Rate <= 0 || float() >= r.Rate || total == 0 {
			continue
		}
		item := r.Choices[pickChoice(r.Choices, intn(total))].Item
		item.StackLeft = 1
		if top := min(max(r.MaxStack, 1), item.Pattern.MaxStack); top > 1 {
			lo := min(max(r.MinStack, 1), top)
			item.StackLeft = lo + intn(top-lo+1)
		}
		out = append(out, item)
	}
	return out
}

// drawCount is the count u (in [0, 1)) falls on: counts[n] is n's chance,
// on any scale.
func drawCount(counts []float64, u float64) int {
	sum := 0.0
	for _, c := range counts {
		sum += max(c, 0)
	}
	if sum <= 0 {
		return 0
	}
	x := u * sum
	for n, c := range counts {
		if x -= max(c, 0); x < 0 {
			return n
		}
	}
	return len(counts) - 1
}

// pickChoice is the choice a weight pick (in [0, total weight)) falls on.
func pickChoice(choices []database.LootChoice, pick int) int {
	for i, c := range choices {
		if pick -= max(c.Weight, 0); pick < 0 {
			return i
		}
	}
	return len(choices) - 1
}
