package handlers

import (
	"bytes"
	"testing"
	"time"
	"unicode/utf16"
)

// wideBytes is s as a client message carries it (UTF-16LE).
func wideBytes(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

// An item with a cooldown (itemeffect.cooldown; the user, 2026-11-07:
// Reconstructive, 15 minutes on the Heirloom server): drunk once, a second use
// within it is refused in red and the stack kept, its time kept in the
// database for the next login; the control, the same potion with no
// cooldown, is drunk twice.
func TestAPotionsCooldownHoldsItsNextUse(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	d := f.potion("Reconstructive", -1, 20, 0, "Self, Increase Hitpoints by 20, Increase Power by 20", "", "")
	free := f.potion("Restorative Draught", -1, 20, 0, "Self, Increase Hitpoints by 20", "", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET cooldown = 900 WHERE patternid = ?`, d); err != nil {
		t.Fatal(err)
	}
	f.own(d, 3)
	f.own(free, 3)
	f.loadList()

	start := time.Now()
	f.useAs(0, 0xFFFFFFFF, 0)
	if n := f.stackOf("Reconstructive"); n != 2 {
		t.Fatalf("the first use: %d left, want 2", n)
	}
	msgs := f.useAs(0, 0xFFFFFFFF, 0)
	refused := false
	for _, m := range msgs {
		if bytes.Contains(m, wideBytes("You can use Reconstructive again in 1")) || bytes.Contains(m, wideBytes("You can use Reconstructive again in 9:")) {
			refused = true
		}
	}
	if !refused {
		t.Error("a second use within 15 minutes was not refused with the time left")
	}
	if n := f.stackOf("Reconstructive"); n != 2 {
		t.Errorf("a refused use took one: %d left, want 2", n)
	}
	ready, err := f.inv.PotionCooldowns(int(f.sid), start)
	if err != nil {
		t.Fatal(err)
	}
	if until, ok := ready[d]; !ok || until.Sub(start) < 14*time.Minute || until.Sub(start) > 16*time.Minute {
		t.Errorf("kept for the next login: %v (%v from now), want about 15 minutes", ready, until.Sub(start))
	}
	if left, _ := f.inv.PotionCooldowns(int(f.sid), start.Add(16*time.Minute)); len(left) != 0 {
		t.Errorf("past its time it is still read: %v", left)
	}

	// the control: no cooldown, drunk twice
	f.useAs(1, 0xFFFFFFFF, 0)
	f.useAs(1, 0xFFFFFFFF, 0)
	if n := f.stackOf("Restorative Draught"); n != 1 {
		t.Errorf("the control: %d left, want 1", n)
	}
}

// A login with a cooldown still running (SetPotionCooldowns from the
// database's row) is refused until it ends.
func TestAPotionsCooldownComesBackAtTheLogin(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	d := f.potion("Reconstructive", -1, 20, 0, "Self, Increase Hitpoints by 20", "", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET cooldown = 900 WHERE patternid = ?`, d); err != nil {
		t.Fatal(err)
	}
	f.own(d, 2)
	f.loadList()
	w.SetPotionCooldowns(f.sess, map[int]time.Time{d: time.Now().Add(10 * time.Minute)})
	f.useAs(0, 0xFFFFFFFF, 0)
	if n := f.stackOf("Reconstructive"); n != 2 {
		t.Errorf("used within the cooldown its login brought back: %d left, want 2", n)
	}
	w.SetPotionCooldowns(f.sess, map[int]time.Time{d: time.Now().Add(-time.Second)})
	f.useAs(0, 0xFFFFFFFF, 0)
	if n := f.stackOf("Reconstructive"); n != 1 {
		t.Errorf("past its cooldown: %d left, want 1", n)
	}
}
