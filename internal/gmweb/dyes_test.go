package gmweb

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A GM's chardyes row is the game's at once (entity/livelook.go): the
// character in the world wears the dye where it stands, the one beside it
// does not; a row the channel cannot carry (alpha 0, a slot past the robe, no
// colour) is refused and the world keeps what it had; the row deleted, the
// slot is undyed again. The control: before any row, both are undyed.
func TestDyeEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	online := func(id uint32, name string) func() [database.DyeSlots]uint32 {
		sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9100 + int(id)}, id, id, 1, 2, true)
		sess.SetCharacterInWorld(true)
		p := f.world.CreatePlayer(sess, &database.CharacterRecord{ServerID: id, CharName: name, X: 25300, Y: 54, Z: 15700})
		return func() [database.DyeSlots]uint32 { return p.Dyes }
	}
	dyed := online(41, "Tinctoria")
	plain := online(42, "Plainjane")
	if dyed() != ([database.DyeSlots]uint32{}) || plain() != ([database.DyeSlots]uint32{}) {
		t.Fatal("the control: dyes before any row")
	}
	code, out := c.op("insert_row", map[string]any{"table": "chardyes",
		"values": map[string]any{"serverid": 41, "slot": database.DyeLegs, "color": 0x400080FF}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if dyed()[database.DyeLegs] != 0x400080FF || plain() != ([database.DyeSlots]uint32{}) {
		t.Fatalf("after the insert: %08x / %08x", dyed(), plain())
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "Tinctoria") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{{"color": 0x40008000}, {"color": "purple"}, {"slot": 10}} {
		if code, out := c.op("update_row", map[string]any{"table": "chardyes",
			"key": map[string]any{"serverid": 41, "slot": database.DyeLegs}, "values": bad}); code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	if code, out := c.op("insert_row", map[string]any{"table": "chardyes", "values": map[string]any{"serverid": 41, "slot": 1}}); code != 400 {
		t.Errorf("insert with no colour: %d %v, want refused", code, out)
	}
	if dyed()[database.DyeLegs] != 0x400080FF {
		t.Errorf("after the refusals: %08x", dyed())
	}
	if code, out := c.op("delete_row", map[string]any{"table": "chardyes", "key": map[string]any{"serverid": 41, "slot": database.DyeLegs}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if dyed() != ([database.DyeSlots]uint32{}) {
		t.Errorf("after the delete: %08x", dyed())
	}
}

// The colours a GM may write: hex with or without alpha, a disc tint, a
// name; alpha 0 and nonsense refused.
func TestParseDyeColor(t *testing.T) {
	for in, want := range map[string]uint32{
		"ff0000": 0xFF0000FF, "#00ff00": 0x00FF00FF, "0x0000FF80": 0x0000FF80, "A50501FF": 0xA50501FF,
		"t0": 0xFFFFFFFF, "t13": 0x96003CFF, "purple": 0x400080FF, "none": 0, "0": 0,
	} {
		if got, err := database.ParseDyeColor(in); err != nil || got != want {
			t.Errorf("%q: %08x %v, want %08x", in, got, err, want)
		}
	}
	for _, bad := range []string{"ff000000", "t15", "zzzzzz", "12345", "chartreuse"} {
		if got, err := database.ParseDyeColor(bad); err == nil {
			t.Errorf("%q: %08x, want refused", bad, got)
		}
	}
}
