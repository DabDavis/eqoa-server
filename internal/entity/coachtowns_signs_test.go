package entity

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// tools/seed_signs.py points its road signs at the coach towns, so it must
// hold coachTowns as they are.
func TestSeedSignsKeepsTheCoachTowns(t *testing.T) {
	src, err := os.ReadFile("../../tools/seed_signs.py")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\nCOACH_TOWNS = \[(.*?)\n\]`).FindSubmatch(src)
	if block == nil {
		t.Fatal("tools/seed_signs.py has no COACH_TOWNS")
	}
	var got []coachTown
	for _, m := range regexp.MustCompile(`\("([^"]+)", (-?\d+), (-?\d+)\)`).FindAllSubmatch(block[1], -1) {
		x, _ := strconv.Atoi(string(m[2]))
		z, _ := strconv.Atoi(string(m[3]))
		got = append(got, coachTown{string(m[1]), float32(x), float32(z)})
	}
	if len(got) != len(coachTowns) {
		t.Fatalf("the tool has %d towns, coachtowns.go %d", len(got), len(coachTowns))
	}
	for i, want := range coachTowns {
		if got[i] != want {
			t.Errorf("town %d: the tool has %v, coachtowns.go %v", i, got[i], want)
		}
	}
}
