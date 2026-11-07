package handlers

import (
	"bytes"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/database"
)

// loreLineSent reports a client message naming the Lore refusal among the
// session's pending messages, in either string encoding.
func (f *scribeFixture) loreLineSent(name string) bool {
	text := "You can hold only one " + name
	wide := make([]byte, 0, 2*len(text))
	for _, u := range utf16.Encode([]rune(text)) {
		wide = append(wide, byte(u), byte(u>>8))
	}
	for _, m := range f.sess.Queue().GetPending() {
		if d := m.Data(); bytes.Contains(d, wide) || bytes.Contains(d, []byte(text)) {
			return true
		}
	}
	return false
}

// A Lore item (itempattern.lore; the user, 2026-11-07, Dasher: "reconstruction
// should be Lore, so only one possessible at a time"): a summon of three gives
// one and says why, a second summon gives none; the control, the same kind of
// item not Lore, comes three records at a time.
func TestALoreItemComesOneAtATime(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	lore := f.potion("Reconstructive", -1, 1, 0, "Self, Increase Hitpoints by 20000, Increase Power by 20000", "", "")
	f.potion("Restorative", -1, 1, 0, "Self, Increase Hitpoints by 20", "", "")
	if _, err := f.db.Exec(`UPDATE itempattern SET lore = 1 WHERE patternid = ?`, lore); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	count := func(name string) int {
		n := 0
		for _, r := range f.rowsOf() {
			if r.Pattern.ItemName == name {
				n += r.StackLeft
			}
		}
		return n
	}

	f.h.summonItem(f.sess, f.sid, "reconstructive", 3, "Reconstructive")
	if n := count("Reconstructive"); n != 1 {
		t.Errorf("a summon of three Lore potions gave %d, want 1", n)
	}
	if !f.loreLineSent("Reconstructive") {
		t.Error("the player was not told why only one came")
	}
	f.h.summonItem(f.sess, f.sid, "reconstructive", 1, "Reconstructive")
	if n := count("Reconstructive"); n != 1 {
		t.Errorf("a second summon while one is held gave more: %d held, want 1", n)
	}
	f.h.summonItem(f.sess, f.sid, "restorative", 3, "Restorative")
	if n := count("Restorative"); n != 3 {
		t.Errorf("the control: %d of a potion that is not Lore, want 3", n)
	}
}

// loreRoom itself: one at most of a Lore item, none while one is held, any
// other item's room unchanged, and no room stays no room.
func TestLoreRoom(t *testing.T) {
	l := &invList{}
	lore := &database.ItemPatternRecord{PatternID: 7, Lore: 1, MaxStack: 1}
	stack := &database.ItemPatternRecord{PatternID: 9, Lore: 1, MaxStack: 20}
	plain := &database.ItemPatternRecord{PatternID: 8, MaxStack: 20}
	if n := l.loreRoom(lore, l.wouldAdd(7, 1, 1)); n != 1 {
		t.Errorf("an empty list takes %d of a Lore item, want 1", n)
	}
	if n := l.loreRoom(stack, l.wouldAdd(9, 5, 20)); n != 1 {
		t.Errorf("a Lore stack of 5 comes as %d, want 1", n)
	}
	l.add(1, 7, 1, 1)
	if n := l.loreRoom(lore, l.wouldAdd(7, 1, 1)); n != 0 {
		t.Errorf("a list holding the Lore item takes %d more, want 0", n)
	}
	if n := l.loreRoom(plain, l.wouldAdd(8, 5, 20)); n != 5 {
		t.Errorf("the control: %d of 5 of an item that is not Lore, want 5", n)
	}
	if n := l.loreRoom(lore, 0); n != 0 {
		t.Errorf("no room became %d", n)
	}
}
