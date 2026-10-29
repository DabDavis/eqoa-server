package gmweb

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A GM's racesize row is the game's at once (entity/racesize.go): the gnome
// in the world takes the new size where it stands and the human beside it
// does not; a row the game cannot use (a size of 0 or 40, a race past
// Froglok, none named) is refused and the world keeps what it had. The
// control: the table as made (jib's gnomes at 0.9) is not the world's until
// it is read, which the server does at its start and a GM's edit does here.
func TestRaceSizeEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	online := func(id uint32, name string, race int) func() float32 {
		sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000 + int(id)}, id, id, 1, 2, true)
		sess.SetCharacterInWorld(true)
		p := f.world.CreatePlayer(sess, &database.CharacterRecord{ServerID: id, CharName: name, Race: race, X: 25300, Y: 54, Z: 15700})
		return func() float32 { _, s := p.Body(); return s }
	}
	gnome := online(31, "Tinkle", 3)
	human := online(32, "Plainman", 0)
	if gnome() != 1 || human() != 1 {
		t.Fatalf("the control: before the table is read, gnome %v human %v", gnome(), human())
	}

	code, out := c.op("update_row", map[string]any{"table": "racesize", "key": map[string]any{"race": 3},
		"values": map[string]any{"size": 0.85}})
	if code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if gnome() != float32(0.85) || human() != 1 {
		t.Fatalf("after the update: gnome %v human %v", gnome(), human())
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "Gnome 0.85") {
		t.Errorf("live: %v", out["live"])
	}
	for _, bad := range []map[string]any{{"size": 0}, {"size": 40}, {"size": "big"}} {
		if code, out := c.op("update_row", map[string]any{"table": "racesize", "key": map[string]any{"race": 3}, "values": bad}); code != 400 {
			t.Errorf("update %v: %d %v, want refused", bad, code, out)
		}
	}
	for _, bad := range []map[string]any{{"race": 11, "size": 1.1}, {"size": 1.1}} {
		if code, out := c.op("insert_row", map[string]any{"table": "racesize", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if gnome() != float32(0.85) {
		t.Errorf("after the refusals the gnome is %v, want 0.85", gnome())
	}
	// the row deleted: the race's own size again
	if code, out := c.op("delete_row", map[string]any{"table": "racesize", "key": map[string]any{"race": 3}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if gnome() != 1 {
		t.Errorf("after the delete the gnome is %v, want 1.0", gnome())
	}
}
