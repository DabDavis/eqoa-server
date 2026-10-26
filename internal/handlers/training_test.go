package handlers

import "testing"

// The trainer's own rules (VIWndTrainer::Adjust, VIClient::TrainStats):
// points into a stat whose shown value items hold at the full cap are taken
// (into its base, as a tester's level-60 Stamina took 115, F3 20261025-214427);
// past the base cap (m_p44: 400 from level 46, 350 below), more than are
// left, nothing, or a stat lowered are not.
func TestTrainRefusal(t *testing.T) {
	base := [7]int{60, 195, 70, 70, 55, 85, 50}
	cases := []struct {
		level  int
		adds   [7]int64
		unused int
		paid   bool
	}{
		{60, [7]int64{0, 3, 0, 0, 0, 0, 0}, 347, true},   // a capped Stamina's base still takes them
		{60, [7]int64{0, 205, 0, 0, 0, 0, 0}, 347, true}, // to the base cap exactly (195 + 205 = 400)
		{60, [7]int64{0, 206, 0, 0, 0, 0, 0}, 347, false},
		{45, [7]int64{0, 156, 0, 0, 0, 0, 0}, 347, false}, // 350 below level 46
		{45, [7]int64{0, 155, 0, 0, 0, 0, 0}, 347, true},
		{60, [7]int64{10, 0, 0, 0, 0, 0, 0}, 9, false}, // more than are left
		{60, [7]int64{}, 9, false},                     // nothing
		{60, [7]int64{5, -2, 0, 0, 0, 0, 0}, 9, false}, // a stat lowered
		{60, [7]int64{1, 1, 1, 1, 1, 1, 1}, 7, true},
	}
	for i, c := range cases {
		why := trainRefusal(c.level, base, c.adds, c.unused)
		if (why == "") != c.paid {
			t.Errorf("case %d (L%d %v, %d left): refusal %q, paid wanted %v", i, c.level, c.adds, c.unused, why, c.paid)
		}
	}
}
