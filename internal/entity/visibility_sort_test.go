package entity

import (
	"math/rand"
	"sort"
	"testing"
)

// The view's sort (visibilitySystem) is slices.SortFunc with the pins found
// first; it was sort.Slice with each pin found at each comparison. Both are
// the standard library's pattern-defeating quicksort, made from one template,
// so the same comparisons give the same order, ties included: a view of
// many entities at equal distances keeps its slots as it did.
func TestTheViewSortOrdersAsSortSliceDid(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	for round := 0; round < 3000; round++ {
		n := rnd.Intn(600)
		in := make([]entityDist, n)
		for i := range in {
			// few distinct distances: ties everywhere; a pin now and then
			in[i] = entityDist{ID: EntityID(i + 1), DistSq: float32(rnd.Intn(1 + n/8)), pin: rnd.Intn(40) == 0}
		}
		if round%3 == 0 { // runs already in order, and reversed: the patterns the sort breaks
			sort.Slice(in, func(i, j int) bool { return in[i].DistSq < in[j].DistSq })
			if round%2 == 0 {
				for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
					in[i], in[j] = in[j], in[i]
				}
			}
		}
		old := append([]entityDist(nil), in...)
		sort.Slice(old, func(i, j int) bool {
			if pi, pj := old[i].pin, old[j].pin; pi != pj {
				return pi
			}
			return old[i].DistSq < old[j].DistSq
		})
		got := append([]entityDist(nil), in...)
		sortView(got)
		for i := range old {
			if old[i] != got[i] {
				t.Fatalf("round %d (%d entities): at %d %+v, sort.Slice had %+v", round, n, i, got[i], old[i])
			}
		}
	}
}
