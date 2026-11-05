package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The log decides offline by the latest login's object: in the world after
// its selection, out once that object's session is cleaned up, in again after
// a new login; a log without a login of it is not an answer.
func TestOfflineInLog(t *testing.T) {
	dir := t.TempDir()
	write := func(lines ...string) string {
		p := filepath.Join(dir, "gameserver.log")
		os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
		return p
	}
	login := []string{
		"2026/11/05 02:04:47 SelectedChar: received 20 bytes from 1.2.3.4:5",
		"2026/11/05 02:04:47 SelectedChar: serverID=126 from 1.2.3.4:5",
		"2026/11/05 02:04:47 SelectedChar: selected 'Wanderer' (level 4 race=5 class=3)",
		"2026/11/05 02:04:47 SelectedChar: generated playerObjectID=500000008 (0x1DCD6508)",
	}
	other := "2026/11/05 02:10:00 Cleaning up terminated session 1.2.3.4:6 (instance=0x1, objectID=500000001)"
	mine := "2026/11/05 02:20:00 Cleaning up terminated session 1.2.3.4:5 (instance=0x28ED7DFA, objectID=500000008)"
	cases := []struct {
		lines       []string
		off, ok     bool
		description string
	}{
		{nil, false, false, "no login"},
		{append(append([]string{}, login...), other), false, true, "another session ended"},
		{append(append([]string{}, login...), other, mine), true, true, "its session ended"},
		{append(append(append([]string{}, login...), mine), strings.ReplaceAll(strings.Join(login, "\n"), "500000008", "500000009")), false, true, "back in"},
	}
	for _, c := range cases {
		lines := c.lines
		if lines == nil {
			lines = []string{other}
		}
		off, ok, _, err := offlineInLog(write(lines...), "wanderer")
		if err != nil || off != c.off || ok != c.ok {
			t.Errorf("%s: offline %v ok %v err %v", c.description, off, ok, err)
		}
	}
}
