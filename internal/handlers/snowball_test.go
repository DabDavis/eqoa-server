package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Snowball (entity/moogieitems.go; petition 2204: "Moogie snowballs showing as stackable trash
// item and cant be equipped to throw at other players"), as tools/snowball_throw_20261116.py makes its record:
// wear 16 (the ranged slot), so CLIENT lists it under Equipment and, its type 2, under the usable items. Each
// list's choice is the same 0x003E: at another player targeted it is thrown (worn or not, from the menu or a
// quick slot), from the menu with no player targeted it is worn and taken off, from a quick slot with none it
// is refused in red and kept.

func snowWide(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), 0)
	}
	return b
}

func snowEquipped(msgs [][]byte, slot byte) bool {
	for _, m := range msgs {
		if len(m) >= 4 && protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == protocol.OpcodeEquipItem && m[3] == slot {
			return true
		}
	}
	return false
}

func snowSaid(msgs [][]byte, text string) bool {
	for _, m := range msgs {
		if bytes.Contains(m, snowWide(text)) {
			return true
		}
	}
	return false
}

func TestSnowballThrownAtAPlayerAndWorn(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	w.SessionManager = session.NewManager() // Bob has no client: told nothing
	bob := &entity.Player{ServerID: 1002, CharServerID: 77, Name: "Bob", Level: 30, HP: 400, MaxHP: 400, X: 5}
	w.Players[bob.ServerID] = bob
	at := 0x7F000000 | uint32(bob.ServerID)
	sb := f.potion("Snowball", 16, 99, 0, "Target, Snowball 0x15C66F59", "", "")
	f.own(sb, 5)
	f.loadList()

	// the usable list's Use at Bob, not worn: thrown, nothing worn
	msgs := f.useAs(0, at, 0)
	if snowEquipped(msgs, 18) || !snowSaid(msgs, "You hit Bob with a snowball!") {
		t.Fatalf("Use at a player sent % X; want a throw, nothing worn", msgs)
	}
	if bob.HP != 399 || f.stackOf("Snowball") != 4 {
		t.Fatalf("after a throw Bob has %d HP and %d are left; want 399 and 4", bob.HP, f.stackOf("Snowball"))
	}

	// the Equipment list's Equip, no one targeted: worn in the ranged slot, nothing thrown
	msgs = f.useAs(0, 0xFFFFFFFF, 0)
	if !snowEquipped(msgs, 18) || snowSaid(msgs, entity.SnowballRefusal) {
		t.Fatalf("Equip sent % X; want 0x003F into the ranged slot (18)", msgs)
	}
	if f.stackOf("Snowball") != 4 {
		t.Fatalf("worn, yet %d left", f.stackOf("Snowball"))
	}

	// worn: a quick slot's Use at Bob throws one
	f.loadList()
	msgs = f.useAs(0, at, 1)
	if !snowSaid(msgs, "You hit Bob with a snowball!") || bob.HP != 398 || f.stackOf("Snowball") != 3 {
		t.Fatalf("a worn one from a quick slot at Bob: sent % X, Bob %d HP, %d left; want a throw, 398, 3", msgs, bob.HP, f.stackOf("Snowball"))
	}

	if f.player.Swing.Potion != sb || f.player.Swing.PotionLeft != 3 {
		t.Errorf("the swing throws %d, %d left; want the snowball, 3 (the worn stack's count kept)", f.player.Swing.Potion, f.player.Swing.PotionLeft)
	}

	// a quick slot's Use at no one: refused in red, kept
	f.loadList()
	msgs = f.useAs(0, 0xFFFFFFFF, 1)
	if !snowSaid(msgs, entity.SnowballRefusal) || f.stackOf("Snowball") != 3 {
		t.Fatalf("a quick slot's at no one: sent % X, %d left; want the refusal, 3", msgs, f.stackOf("Snowball"))
	}

	// the hit leaves 1
	bob.HP = 1
	f.loadList()
	f.useAs(0, at, 0)
	if bob.HP != 1 || f.stackOf("Snowball") != 2 {
		t.Fatalf("at 1 HP: Bob %d, %d left; want 1 and 2", bob.HP, f.stackOf("Snowball"))
	}
}

// usesPotion for a snowball, worn (16) and not (-1), by interaction and mark.
func TestSnowballUsesPotion(t *testing.T) {
	sb := entity.NewPotionEffect(1, "Snowball", "Target, Snowball 0x15C66F59", "", 0, "", 0, 0)
	if sb.Special != entity.PotionSnowball {
		t.Fatalf("Special %q", sb.Special)
	}
	cases := []struct {
		wear        int
		interaction byte
		atPlayer    bool
		want        bool
	}{
		{-1, 0, false, true}, {-1, 1, false, true}, // worn nowhere (the old record): every Use throws
		{16, 0, true, true}, {16, 1, true, true}, // at another player: thrown
		{16, 1, false, true},  // a quick slot's at no player: the throw's refusal
		{16, 0, false, false}, // the menu with no player: worn or taken off
	}
	for _, c := range cases {
		if got := usesPotion(&sb, &database.ItemPatternRecord{EquipSlot: c.wear}, c.interaction, c.atPlayer); got != c.want {
			t.Errorf("wear %d, interaction %d, at a player %v: %v, want %v", c.wear, c.interaction, c.atPlayer, got, c.want)
		}
	}
}
