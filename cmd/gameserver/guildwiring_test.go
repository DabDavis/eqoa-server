package main

import (
	"os"
	"strings"
	"testing"
)

// The server reads the kept guilds and hands the World its store at the start
// (database/guilds.go): the guild code shipped once without this wiring, so a
// restart showed no guild and kept no change. A source check, as the start is
// not a function a test can run.
func TestMainKeepsTheGuilds(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	load, store, resume := strings.Index(s, "gameWorld.LoadGuilds("), strings.Index(s, "gameWorld.SetGuildStore(db)"), strings.Index(s, "ResumeKept(")
	if load < 0 || store < 0 {
		t.Fatalf("main.go does not load the guilds (%d) or set their store (%d)", load, store)
	}
	if resume >= 0 && (load > resume || store > resume) {
		t.Fatal("the guilds are read after the kept sessions resume: their players would not rejoin them")
	}
}
