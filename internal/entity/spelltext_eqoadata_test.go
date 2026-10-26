package entity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// eqoaDataSpells is every ability line of EQOA-Data's per-class lists
// (Information/databases/spells/<class>.json): the lines spelleffect holds
// and tools/import_eqoadata_spells.py writes. EQOA_DATA names the project's
// checkout; without one (or a sibling external/EQOA-Data) the tests skip.
func eqoaDataSpells(t *testing.T) []eqoaDataSpell {
	t.Helper()
	dir := os.Getenv("EQOA_DATA")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "external", "EQOA-Data")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "Information", "databases", "spells", "*.json"))
	if len(files) == 0 {
		t.Skip("EQOA_DATA names a checkout of EQOA-Data")
	}
	var out []eqoaDataSpell
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var list struct {
			Title string `json:"title"`
			List  []struct {
				Level     int `json:"level"`
				Abilities []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Source      string `json:"source"`
				} `json:"abilities"`
			} `json:"list"`
		}
		if err := json.Unmarshal(data, &list); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, lv := range list.List {
			for _, a := range lv.Abilities {
				out = append(out, eqoaDataSpell{Class: list.Title, Level: lv.Level, Name: a.Name, Line: a.Description, Source: a.Source})
			}
		}
	}
	return out
}

type eqoaDataSpell struct {
	Class, Name, Line, Source string
	Level                     int
}

// TestEQOADataLinesRead counts what the parser leaves unread across every
// EQOA-Data line, and lists the commonest leftovers (go test -v). It fails
// when fewer lines are read whole than eqoaDataReadFloor: 993 of 1,693 were
// before spellnotes.go (F3 pass 17), 1,036 after.
func TestEQOADataLinesRead(t *testing.T) {
	spells := eqoaDataSpells(t)
	read, partly, none := 0, 0, 0
	left := map[string]int{}
	for _, s := range spells {
		d := ParseSpell(s.Line, "")
		switch {
		case len(d.Unread) == 0 && (len(d.Effects) > 0 || d.Duration > 0):
			read++
		case len(d.Effects) > 0:
			partly++
		default:
			none++
		}
		for _, u := range d.Unread {
			left[u]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var top []kv
	for k, v := range left {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].v > top[j].v || top[i].v == top[j].v && top[i].k < top[j].k })
	var b strings.Builder
	for i, e := range top {
		if i == 60 {
			break
		}
		b.WriteString("\n  ")
		b.WriteString(strings.Repeat(" ", 3-len(itoaPad(e.v))) + itoaPad(e.v) + "  " + e.k)
	}
	t.Logf("%d lines: %d read whole, %d in part, %d not at all; commonest unread parts:%s", len(spells), read, partly, none, b.String())
	if read < eqoaDataReadFloor {
		t.Errorf("%d lines read whole, fewer than %d", read, eqoaDataReadFloor)
	}
}

const eqoaDataReadFloor = 1036

func itoaPad(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}
