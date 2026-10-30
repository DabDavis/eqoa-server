package handlers

import (
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// No Rent and lore items (norent.go, potion.go; the owner's ruling of
// 2026-10-29: summoned items are No Rent, gone after more than five minutes
// logged out, and the alchemist's ogre pill is lore). The fixture's druid
// holds them; the items and their numbers are made up.

// noRentItems gives the fixture's character a No Rent drink in its
// inventory, a No Rent ring worn and a No Rent stone in its bank, and an
// ordinary sword beside each.
func (f *scribeFixture) noRentItems() {
	f.t.Helper()
	give := func(name string, rent, location int, worn bool) {
		id := f.item(name, false, 1<<classDruid, 1, 20)
		if _, err := f.db.Exec(`UPDATE itempattern SET rent = ? WHERE patternid = ?`, rent, id); err != nil {
			f.t.Fatal(err)
		}
		key, err := f.inv.AddCharItem(f.sid, id, 3, 0, 0, location)
		if err != nil {
			f.t.Fatal(err)
		}
		if worn {
			if err := f.inv.SetEquipLoc(f.sid, key, 8); err != nil {
				f.t.Fatal(err)
			}
		}
	}
	give("Made-up Summoned Drink", 0, 0, false)
	give("Made-up Summoned Ring", 0, 0, true)
	give("Made-up Summoned Stone", 0, 1, false)
	give("Made-up Sword", 1, 0, false)
	give("Made-up Banked Sword", 1, 1, false)
}

func (f *scribeFixture) heldNames() []string {
	f.t.Helper()
	var names []string
	for _, r := range f.rowsOf() {
		names = append(names, r.Pattern.ItemName)
	}
	slices.Sort(names)
	return names
}

// The login takes the No Rent items, worn, carried and banked, after more
// than NoRentAway out of the world, and leaves the rest; the logout's row is
// spent by the reading. The controls: just under NoRentAway away, and no
// logout recorded at all (the first login after the table was made), keep
// them.
func TestNoRentItemsGoAfterFiveMinutesAway(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	all := []string{"Made-up Banked Sword", "Made-up Summoned Drink", "Made-up Summoned Ring", "Made-up Summoned Stone", "Made-up Sword"}
	for _, c := range []struct {
		name   string
		logout time.Duration // before now; 0: none recorded
		want   []string
	}{
		{"six minutes away", 6 * time.Minute, []string{"Made-up Banked Sword", "Made-up Sword"}},
		{"just under five", NoRentAway - time.Second, all},
		{"no logout recorded", 0, all},
	} {
		f := newScribeFixture(t)
		f.noRentItems()
		store := database.NewSQLiteCharacterStore(f.db)
		if c.logout > 0 {
			if err := store.SaveLogout(f.sid, now.Add(-c.logout)); err != nil {
				t.Fatal(err)
			}
		}
		h := &CharacterHandler{store: store, inventoryStore: f.inv}
		rec, err := store.GetByServerID(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		h.takeNoRent(rec, now)
		if got := f.heldNames(); !slices.Equal(got, c.want) {
			t.Errorf("%s: holds %q, want %q", c.name, got, c.want)
		}
		if _, ok, err := store.TakeLogout(f.sid); ok || err != nil {
			t.Errorf("%s: the logout's row is still there (%v)", c.name, err)
		}
	}
}

// A character selected after six minutes away enters the world without its
// No Rent items: the login takes them before it reads the inventory for the
// world, so its list holds the swords alone.
func TestALoginTakesTheNoRentItems(t *testing.T) {
	f := newScribeFixture(t)
	f.noRentItems()
	store := database.NewSQLiteCharacterStore(f.db)
	if err := store.SaveLogout(f.sid, time.Now().Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w, inventoryStore: f.inv}
	s := sm.CreateSession(testAddr(4), 4, 4, 2, 2, true)
	s.SetAccountID(1)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
	msg.SetData(u32b(f.sid))
	if err := h.HandleSelectedChar(s, msg); err != nil {
		t.Fatal(err)
	}
	if got := f.heldNames(); !slices.Equal(got, []string{"Made-up Banked Sword", "Made-up Sword"}) {
		t.Errorf("after the login the character holds %q", got)
	}
	// the list the MemoryDump described: the inventory's sword alone
	l := sessionInvList(s)
	if l == nil {
		t.Fatal("no inventory list after the login")
	}
	if len(l.recs) != 1 {
		t.Errorf("the world's list holds %d records, want the sword alone: %+v", len(l.recs), l.recs)
	}
}

// A lore item is summoned once: one to a character holding none, though
// the summoning names three, and none, with a line in red, to one that holds
// it (in its bank too). The control: an ordinary potion comes three at a
// time whatever is held.
func TestALoreItemIsSummonedOnce(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	pill := f.potion("Made-up Ogre Pill", -1, 20, 1, "Self, Illusion: Ogre, Increase Maximum HP by 100, 10min", "Made-up Ogre Form", "")
	if _, err := f.db.Exec(`UPDATE itempattern SET lore = 1, rent = 1 WHERE patternid = ?`, pill); err != nil {
		t.Fatal(err)
	}
	f.potion("Made-up Tonic", -1, 20, 1, "Self, Increase Power by 10", "", "")
	f.loadList()

	f.h.summonItem(f.sess, f.sid, "made-up ogre pill", 3, "Made-up Pill Spell")
	if n := f.stackOf("Made-up Ogre Pill"); n != 1 {
		t.Fatalf("the first summoning gave %d, want 1", n)
	}
	f.sess.Queue().GetPending()
	f.h.summonItem(f.sess, f.sid, "made-up ogre pill", 0, "Made-up Pill Spell")
	if n := f.stackOf("Made-up Ogre Pill"); n != 1 {
		t.Errorf("held already, the summoning made it %d", n)
	}
	if !refusedWith(f.sess, loreHeldLine("Made-up Ogre Pill")) {
		t.Error("no red line for the lore item held")
	}

	// one in the bank counts as held
	for _, r := range f.rowsOf() {
		if r.Pattern.PatternID == pill {
			if err := f.inv.DeleteCharItem(f.sid, r.ItemID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.inv.AddCharItem(f.sid, pill, 1, 0, 0, 1); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	f.h.summonItem(f.sess, f.sid, "made-up ogre pill", 0, "Made-up Pill Spell")
	held := 0
	for _, r := range f.rowsOf() {
		if r.Pattern.PatternID == pill {
			held += r.StackLeft
		}
	}
	if held != 1 {
		t.Errorf("with one in the bank the character holds %d", held)
	}

	f.h.summonItem(f.sess, f.sid, "made-up tonic", 0, "Made-up Tonic Spell")
	f.h.summonItem(f.sess, f.sid, "made-up tonic", 0, "Made-up Tonic Spell")
	if n := f.stackOf("Made-up Tonic"); n != 6 {
		t.Errorf("the control: %d tonics, want 6", n)
	}
}

// refusedWith is whether a red line (0x00CE) of text is queued to sess.
func refusedWith(sess *session.Session, text string) bool {
	want := buildErrorMessage(text)
	for _, m := range sess.Queue().GetPending() {
		if slices.Equal(m.Data(), want) {
			return true
		}
	}
	return false
}
