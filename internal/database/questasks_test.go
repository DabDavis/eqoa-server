package database

import (
	"strings"
	"testing"
)

// An asks column reads as JSON; its problems: bad JSON, an answer too long, a
// reply's line too long, too many answers at once (work entry 1597).
func TestQuestAsks(t *testing.T) {
	good := `[{"answer": "Timeskipped?", "says": "Just that!\nNo one knows why.", "asks": [{"answer": "Why once a day?", "says": "FOMO."}]}]`
	as, err := ParseQuestAsks(good)
	if err != nil || len(as) != 1 || len(as[0].Asks) != 1 || as[0].Asks[0].Says != "FOMO." {
		t.Fatalf("read: %+v %v", as, err)
	}
	if p := QuestAskProblems(good, true); len(p) != 0 {
		t.Errorf("a good column: %v", p)
	}
	if as, err := ParseQuestAsks(""); err != nil || as != nil {
		t.Errorf("empty: %v %v", as, err)
	}
	long := `[{"answer": "` + strings.Repeat("a", 64) + `", "says": "` + strings.Repeat("b", 218) + `"}]`
	if p := QuestAskProblems(long, false); len(p) != 2 {
		t.Errorf("too long: %v", p)
	}
	if p := QuestAskProblems(`[{"answer":`, false); len(p) != 1 {
		t.Errorf("bad JSON: %v", p)
	}
	many := `[{"answer":"a","says":"x"},{"answer":"b","says":"x"},{"answer":"c","says":"x"},{"answer":"d","says":"x","asks":[{"answer":"e","says":"y"},{"answer":"f","says":"y"}]}]`
	if p := QuestAskProblems(many, true); len(p) != 1 || !strings.Contains(p[0], "at once") {
		t.Errorf("too many at once: %v", p)
	}
}
