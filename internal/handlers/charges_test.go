package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// msgsOf is the messages of one opcode out of a use's.
func msgsOf(all [][]byte, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	for _, m := range all {
		if len(m) >= 2 && protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == op {
			out = append(out, m)
		}
	}
	return out
}

// Petition 1088: Reconstructive has 4 usable charges. A new one holds them all
// (itemeffect.charges); each use does what it does, takes a charge (remaincharge)
// and tells the client with 0x003E {list index, 1} (CLIENT's Msg003E counts its copy
// down), never 0x003C; the fourth takes the record, as Msg003E's count reaching 0
// does on the client. The controls: an item with no charges takes one off its stack
// with 0x003C (TestDrinkPotionTakesOne), and an item that stacks holds none.
func TestAChargedItemIsSpentAChargeAtATime(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	d := f.potion("Reconstructive", -1, 1, 1, "Self, Increase Hitpoints by 5", "", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET charges = 4 WHERE patternid = ?`, d); err != nil {
		t.Fatal(err)
	}
	if n := f.inv.ItemCharges(d); n != 4 {
		t.Fatalf("ItemCharges %d, want 4", n)
	}
	if _, err := f.inv.AddCharItem(f.sid, d, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	left := func() (records, charges int) {
		t.Helper()
		if err := f.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(remaincharge), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`,
			f.sid, d).Scan(&records, &charges); err != nil {
			t.Fatal(err)
		}
		return
	}
	if r, c := left(); r != 1 || c != 4 {
		t.Fatalf("new: %d records, %d charges", r, c)
	}
	want := []byte{0x3E, 0x00, 0x00, 0x01}
	for use, wantLeft := range []int{3, 2, 1} {
		f.player.HP = 100
		msgs := f.useAs(0, 0xFFFFFFFF, 1)
		if got := msgsOf(msgs, protocol.OpcodeInteractItem); len(got) != 1 || !bytes.Equal(got[0], want) {
			t.Fatalf("use %d: 0x003E % X, want % X", use+1, got, want)
		}
		if got := msgsOf(msgs, protocol.OpcodeRemoveInvItem); len(got) != 0 {
			t.Errorf("use %d: a 0x003C went out too: % X", use+1, got)
		}
		if f.player.HP != 105 {
			t.Errorf("use %d: %d HP, want its heal's 105", use+1, f.player.HP)
		}
		if r, c := left(); r != 1 || c != wantLeft {
			t.Errorf("use %d: %d records, %d charges; want 1, %d", use+1, r, c, wantLeft)
		}
	}
	msgs := f.useAs(0, 0xFFFFFFFF, 1)
	if got := msgsOf(msgs, protocol.OpcodeInteractItem); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the last use: 0x003E % X, want % X", got, want)
	}
	if r, _ := left(); r != 0 {
		t.Errorf("after the last charge: %d records, want none", r)
	}
	if l := sessionInvList(f.sess); l != nil {
		if _, ok := l.at(0); ok {
			t.Error("the list still holds it")
		}
	}

	// a stack holds no charges, whatever its row says
	s := f.potion("Stack Draught", -1, 3, 1, "Self, Increase Hitpoints by 5", "", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET charges = 4 WHERE patternid = ?`, s); err != nil {
		t.Fatal(err)
	}
	if n := f.inv.ItemCharges(s); n != 0 {
		t.Errorf("a stack's ItemCharges %d, want 0", n)
	}
}

// A charged item given by a sale or a summons goes out holding its charges, on the
// wire too (giveItem's record); one with charges left, moved, keeps those.
func TestAGivenItemHoldsItsCharges(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	d := f.potion("Reconstructive", -1, 1, 1, "Self, Increase Hitpoints by 5", "", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET charges = 4 WHERE patternid = ?`, d); err != nil {
		t.Fatal(err)
	}
	p, ok, err := f.inv.ItemPatternByName("Reconstructive")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	f.loadList()
	h := &NPCWindowHandler{inv: f.inv}
	out, _, err := h.giveItem(f.player, sessionInvList(f.sess), database.CharInventoryEntry{StackLeft: 1, Pattern: p}, 1, 0)
	if err != nil || out.Charges != 4 {
		t.Fatalf("a new one: %+v charges, %v", out.Charges, err)
	}
	out, _, err = h.giveItem(f.player, sessionInvList(f.sess), database.CharInventoryEntry{StackLeft: 1, Charges: 2, Pattern: p}, 1, 0)
	if err != nil || out.Charges != 2 {
		t.Fatalf("one with 2 left: %d charges, %v", out.Charges, err)
	}
}

// Petition 1104: the Rolling Dice roll 0 to 99 for everyone in say range, in orange,
// the roller too and nobody past it, at most once in diceEvery a player; another
// player's own dice go on regardless.
func TestTheRollingDiceRollForTheRoom(t *testing.T) {
	old := rollDice
	rollDice = func(n int) int { return n - 1 }
	defer func() { rollDice = old }()
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	near, pn := c.join(2, "Near", 25050, 16000, 0)
	far, _ := c.join(3, "Far", 25500, 16000, 0)
	t0 := time.Unix(1_800_000_000, 0)
	c.h.RollTheDice(alice, pa, t0)
	for _, s := range []*session.Session{alice, near} {
		got := heard(t, s)
		if len(got) != 1 || got[0].text != "Alice rolls 99 (0 to 99)." || got[0].rgb != diceRGB {
			t.Errorf("in range heard %+v", got)
		}
		s.Queue().GetPending()
	}
	if got := heard(t, far); len(got) != 0 {
		t.Errorf("out of range heard %+v", got)
	}
	c.h.RollTheDice(alice, pa, t0.Add(diceEvery-time.Millisecond))
	if got := heard(t, near); len(got) != 0 {
		t.Errorf("a second roll inside the wait: %+v", got)
	}
	c.h.RollTheDice(near, pn, t0.Add(time.Millisecond))
	if got := heard(t, alice); len(got) != 1 || got[0].text != "Near rolls 99 (0 to 99)." {
		t.Errorf("another's dice: %+v", got)
	}
	alice.Queue().GetPending()
	c.h.RollTheDice(alice, pa, t0.Add(diceEvery))
	if got := heard(t, alice); len(got) != 1 {
		t.Errorf("after the wait: %+v", got)
	}
}

// The dice's Use (either way the client sends it, any target) rolls and keeps them;
// the record's name and being worn nowhere are what make it dice.
func TestUsingTheRollingDice(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	rolls := 0
	f.h.dice = func(*session.Session, *entity.Player, time.Time) { rolls++ }
	dice := f.item(RollingDiceName, false, 32767, 1, 99)
	f.own(dice, 1)
	f.loadList()
	f.useAs(0, 0xFFFFFFFF, 0)
	f.useAs(0, 1234, 1)
	if rolls != 2 || f.stackOf(RollingDiceName) != 1 {
		t.Errorf("%d rolls, %d dice left; want 2, 1", rolls, f.stackOf(RollingDiceName))
	}
	if !IsRollingDice(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " rolling dice"}) ||
		IsRollingDice(&database.ItemPatternRecord{EquipSlot: 18, ItemName: RollingDiceName}) ||
		IsRollingDice(&database.ItemPatternRecord{EquipSlot: -1, ItemName: "dice"}) {
		t.Error("IsRollingDice")
	}
}
