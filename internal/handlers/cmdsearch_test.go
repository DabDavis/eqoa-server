package handlers

import (
	"strings"
	"testing"
)

// Petition 1865: "/tan ?" finds a GM's #tankprofile line, "/raid ?" the raid's commands for anyone, and a word no
// command begins with says so; a player is not shown the GM's.
func TestCommandSearch(t *testing.T) {
	has := func(lines []string, sub string) bool {
		for _, l := range lines {
			if strings.Contains(l, sub) {
				return true
			}
		}
		return false
	}
	if got := commandsBeginning("/tan", true); !has(got, "#tankprofile1") {
		t.Fatalf("a GM's /tan ?: %q", got)
	}
	if got := commandsBeginning("#TAN", false); has(got, "#tankprofile1") || !has(got, "none") {
		t.Fatalf("a player's #TAN ?: %q", got)
	}
	if got := commandsBeginning("/raid", false); !has(got, "/raidinvite") || !has(got, "/raidleave") {
		t.Fatalf("/raid ?: %q", got)
	}
	if got := commandsBeginning("/zzz", true); len(got) != 1 || !has(got, "none") {
		t.Fatalf("/zzz ?: %q", got)
	}
	for _, m := range []string{"/tan ?", "#tan?", " /tan  ? "} {
		if cmdSearchAsk.FindStringSubmatch(m) == nil {
			t.Errorf("%q not read as a search", m)
		}
	}
	for _, m := range []string{"/tan", "hello ?", "/tell bob ?", "?"} {
		if cmdSearchAsk.FindStringSubmatch(m) != nil {
			t.Errorf("%q read as a search", m)
		}
	}
}
