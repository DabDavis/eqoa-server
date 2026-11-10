package udp

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/handlers"
	"github.com/eqoa/server/internal/session"
)

// newItemLinksProcess wires the handlers as a process does
// (RegisterDefaultHandlers, here with no world, so no engine starts), which
// gives it its own empty store of item links.
func newItemLinksProcess(t *testing.T) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/links.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	handlers.RegisterDefaultHandlers(handlers.NewRegistry(), database.NewSQLiteCharacterStore(db), database.NewSQLiteAccountStore(db),
		session.NewManager(), "127.0.0.1", nil, database.NewInventoryStore(db), database.NewSpellbookStore(db))
}

// The links in players' chat go across a keep restart: written down with the
// sessions, through the file, and put back by the next image before a
// datagram is read, so each still opens; a file without them leaves the next
// image's links empty (the control).
func TestTheItemLinksGoAcrossTheExec(t *testing.T) {
	now := time.Now()
	str := 5
	links := []handlers.KeptItemLink{
		{ID: 41, Owner: 198, Item: 7, Who: "Alice", Name: "Rusty Sword", At: now.Add(-time.Minute),
			Record: database.CharInventoryEntry{ItemID: 7, StackLeft: 1, RemainHP: 40, EquipLoc: -1, Location: -1,
				Pattern: database.ItemPatternRecord{PatternID: 9, ItemName: "Rusty Sword", MaxHP: 50, Str: &str}}},
		{ID: 42, Owner: 198, Item: 8, Who: "Alice", Name: "Gem One", At: now,
			Record: database.CharInventoryEntry{ItemID: 8, StackLeft: 1, EquipLoc: -1, Location: -1,
				Pattern: database.ItemPatternRecord{PatternID: 10, ItemName: "Gem One"}}},
	}
	newItemLinksProcess(t)
	if n := handlers.RestoreItemLinks(links); n != 2 { // the links this process made
		t.Fatalf("%d links to begin with", n)
	}
	l := NewListener(config.DefaultConfig(), session.NewManager(), nil, entity.NewWorld())
	snap := l.Snapshot(now)
	if !reflect.DeepEqual(snap.ItemLinks, handlers.KeepItemLinks()) || len(snap.ItemLinks) != 2 {
		t.Fatalf("written down: %+v", snap.ItemLinks)
	}
	file := filepath.Join(t.TempDir(), "keep-sessions.json")
	if err := snap.Write(file); err != nil {
		t.Fatal(err)
	}
	back, err := ReadSnapshot(file, now)
	if err != nil {
		t.Fatal(err)
	}

	newItemLinksProcess(t) // the next image
	if got := handlers.KeepItemLinks(); len(got) != 0 {
		t.Fatalf("a new image's links: %+v", got)
	}
	l2 := NewListener(config.DefaultConfig(), session.NewManager(), nil, entity.NewWorld())
	l2.ResumeKept(back, now)
	got := handlers.KeepItemLinks()
	if len(got) != 2 || got[0].ID != 41 || got[1].ID != 42 || *got[0].Record.Pattern.Str != 5 || got[1].Name != "Gem One" {
		t.Fatalf("after the restart: %+v", got)
	}

	newItemLinksProcess(t) // and from a file without them
	back.ItemLinks = nil
	NewListener(config.DefaultConfig(), session.NewManager(), nil, entity.NewWorld()).ResumeKept(back, now)
	if got := handlers.KeepItemLinks(); len(got) != 0 {
		t.Fatalf("links out of nothing: %+v", got)
	}
}
