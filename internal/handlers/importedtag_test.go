package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// "(Imported)" goes after an imported copy's name as it is sent, when the
// switch is on and the name with it fits CLIENT's 63 characters; never on a
// record that is no copy, and never with the switch off.
func TestTheImportedTagIsSentBehindItsSwitch(t *testing.T) {
	defer entity.SetImportedItems(nil)
	defer SetImportedTag(false)
	entity.SetImportedItems(map[int]int{990001: 1026, 990002: 1027})
	copyRec := &database.ItemPatternRecord{PatternID: 990001, ItemName: "Steelwind"}
	base := &database.ItemPatternRecord{PatternID: 1026, ItemName: "Steelwind"}
	long := &database.ItemPatternRecord{PatternID: 990002, ItemName: strings.Repeat("x", 53)}

	if got := itemNameShown(copyRec); got != "Steelwind" {
		t.Errorf("the switch off: %q", got)
	}
	SetImportedTag(true)
	if got := itemNameShown(copyRec); got != "Steelwind (Imported)" {
		t.Errorf("the switch on: %q", got)
	}
	if got := itemNameShown(base); got != "Steelwind" {
		t.Errorf("the base with the switch on: %q", got)
	}
	if got := itemNameShown(long); got != long.ItemName {
		t.Errorf("a name the tag would push past 63: %q", got)
	}
}
