package handlers

import (
	"os"
	"strings"
	"testing"
)

// TestRealSkyPassSteps: on a real server's database (EQOA_REAL_DB), every Plane of Sky passage does a step that is
// a talk at its own essence, and each chain's passages do its essence steps in order, one each, none left over. The
// chain of desolation's passages had kept the step numbers it had before its guide steps were added (4, 9, 12, 14,
// where the essence steps are 7, 12, 15, 17): a quester on the third isle stood at step 12 with the harpies not
// counting (Heirloom petition 2675).
func TestRealSkyPassSteps(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	done := map[int][]int{} // quest -> the steps its passages do, in the passages' order
	for _, p := range skyPasses {
		for _, qs := range p.Steps {
			q := w.quest(qs[0])
			if q == nil {
				t.Errorf("%s (%s): no quest %d", p.NPC, p.Key, qs[0])
				continue
			}
			at := p.NPC
			if p.StepsAs != "" {
				at = p.StepsAs // the portal does the essence's step
			}
			st := q.Step(qs[1])
			if st == nil || !strings.EqualFold(st.Target, at) {
				t.Errorf("%s (%s, to %s): quest %d step %d is %+v, not a step at %s", p.NPC, p.Key, p.To, qs[0], qs[1], st, at)
				continue
			}
			done[qs[0]] = append(done[qs[0]], qs[1])
		}
	}
	for qid, steps := range done {
		for i := 1; i < len(steps); i++ {
			if steps[i] < steps[i-1] { // two passages may do one step (a portal and its essence)
				t.Errorf("quest %d: the passages do steps %v, not in the chain's order", qid, steps)
			}
		}
	}
	// the chain of desolation's every essence step is a passage's
	want := []int{7, 12, 15, 17}
	if got := done[101308]; len(got) != len(want) {
		t.Errorf("the chain of desolation's passages do steps %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("the chain of desolation's passages do steps %v, want %v", got, want)
				break
			}
		}
	}
}
