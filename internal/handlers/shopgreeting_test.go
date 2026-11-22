package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// useRow is an npcdialog row of an NPC's own name and a use.
func useRow(id int, name, use, line string) database.NPCDialogLine {
	return database.NPCDialogLine{ID: id, Name: name, Line: line, Weight: 1, Use: use,
		Race: -1, SpeakerRace: -1, Class: -1, NotRace: -1}
}

// A merchant with a shop row of its own (npcdialog.use shop) greets with it
// atop its window; one without greets "Have a look at my wares."; a talk row
// or an ambient row of its name is never its greeting; a greeting past 127
// characters is clipped (CLIENT's ReadString keeps a longer one EMPTY). The
// control: with no reader at all, the old constant.
func TestAMerchantGreetsWithItsOwnShopLine(t *testing.T) {
	f := newBackFixture(t)
	f.catalog(f.sword, 1, 0)
	f.loadLists()
	open := func(rows ...database.NPCDialogLine) string {
		t.Helper()
		f.h.SetDialogLines(func() ([]database.NPCDialogLine, error) { return rows, nil })
		return f.open().greeting
	}

	// no world: the constant
	if got := f.open().greeting; got != merchantGreeting {
		t.Fatalf("with no world: %q", got)
	}
	f.h.gameWorld = entity.NewWorld()

	if got := open(); got != merchantGreeting {
		t.Errorf("no rows: %q, want %q", got, merchantGreeting)
	}
	if got := open(useRow(1, "Merchant", database.DialogUseShop, "I have a few odds and ends to sell.")); got != "I have a few odds and ends to sell." {
		t.Errorf("its shop row: %q", got)
	}
	if got := open(useRow(1, "Merchant", database.DialogUseTalk, "Hello there."),
		useRow(2, "Merchant", database.DialogUseAmbient, "Fresh wares!")); got != merchantGreeting {
		t.Errorf("a talk and an ambient row only: %q, want %q", got, merchantGreeting)
	}
	if got := open(useRow(1, "Another merchant", database.DialogUseShop, "Not mine.")); got != merchantGreeting {
		t.Errorf("another merchant's shop row: %q", got)
	}
	long := strings.Repeat("x", 200)
	if got := open(useRow(1, "Merchant", database.DialogUseShop, long)); len(got) != greetingMax {
		t.Errorf("a 200-character greeting: %d characters, want %d", len(got), greetingMax)
	}
}
