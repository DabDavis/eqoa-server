package entity

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// Every spell's workbook effects as the import's tool writes them read
// whole: EQOA_WORKBOOK_JSONL names a file of one {"name", "json"} object a
// line (spelleffect.effects as the tool would write it, made from the
// operator's own workbook), skipped without one. None may fail to read; what
// the engine has no rule for is counted, by its words, for the log.
func TestRealWorkbookEffectsRead(t *testing.T) {
	path := os.Getenv("EQOA_WORKBOOK_JSONL")
	if path == "" {
		t.Skip("EQOA_WORKBOOK_JSONL not set")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	spells, empty := 0, 0
	kinds := map[EffectKind]int{}
	why := map[string]int{}
	for sc.Scan() {
		var row struct{ Name, JSON string }
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		spells++
		d, err := ParseSpellJSON(row.Name, "", row.JSON)
		if err != nil {
			if strings.Contains(err.Error(), "no effects") {
				empty++
				continue
			}
			t.Errorf("%s: %v", row.Name, err)
			continue
		}
		for _, e := range d.Effects {
			kinds[e.Kind]++
		}
		for _, u := range d.Unsupported {
			// the words without their figures
			w := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' || r == '-' || r == '.' {
					return -1
				}
				return r
			}, u)
			why[w]++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for w, n := range why {
		lines = append(lines, strings.TrimSpace(w)+": "+itoa(n))
	}
	sort.Strings(lines)
	t.Logf("%d spells, %d with no effects; effects by kind %v", spells, empty, kinds)
	t.Logf("unsupported:\n  %s", strings.Join(lines, "\n  "))
}
