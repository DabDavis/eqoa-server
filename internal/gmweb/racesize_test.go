package gmweb

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
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
	for _, bad := range []map[string]any{{"race": 13, "size": 1.1}, {"size": 1.1}} {
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

// The panel names a custom race and takes its rows: the character's race reads by
// the row's name (where a number would stand), the enum and the masks hold it, a
// racesize row for it is taken and one for a race with no row is not, a row the
// game could not use is refused, and an edit is the game's at once.
func TestCustomRacesInThePanel(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	t.Cleanup(func() { entity.SetRaceDefs(nil) })
	if got := raceName(11); got != "race 11" {
		t.Fatalf("the control: with no row race 11 reads %q", got)
	}
	code, out := c.op("insert_row", map[string]any{"table": "racedefs", "values": map[string]any{
		"id": 11, "name": "Hill Giant", "short_name": "HGT", "wire_race": 9, "bonus_str": 25, "bonus_sta": 20,
		"bonus_agi": -5, "bonus_dex": -5, "body_male": -5117396, "body_female": -5117396}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "Hill Giant (11, HGT, as the Ogre)") {
		t.Errorf("live: %v", out["live"])
	}
	if entity.WireRace(11) != 9 || entity.RaceName(11) != "Hill Giant" {
		t.Errorf("not the game's at once: wire %d, %q", entity.WireRace(11), entity.RaceName(11))
	}
	if got := raceName(11); got != "Hill Giant" {
		t.Errorf("raceName(11) = %q", got)
	}
	has := func(values []EnumValue) bool {
		for _, e := range values {
			if e.Value == 11 && e.Label == "Hill Giant" {
				return true
			}
		}
		return false
	}
	en := enums()
	if !has(en["race"]) || !has(en["raceany"]) || has(en["wirerace"]) {
		t.Errorf("the enums: race %v, raceany %v, wirerace %v", has(en["race"]), has(en["raceany"]), has(en["wirerace"]))
	}
	if len(en["wirerace"]) != 10 || en["race"][10].Label != "Froglok" {
		t.Errorf("the game's own: %d wire races, race 10 %q", len(en["wirerace"]), en["race"][10].Label)
	}
	var bit bool
	for _, b := range flagSets()["racemask11"] {
		bit = bit || (b.Bit == 1<<11 && b.Label == "Hill Giant")
	}
	if !bit {
		t.Error("cmcatalog.races' flags have no bit for it")
	}

	if code, out := c.op("insert_row", map[string]any{"table": "racesize", "values": map[string]any{"race": 11, "size": 1.1}}); code != 200 {
		t.Errorf("a size for a custom race: %d %v", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "racesize", "values": map[string]any{"race": 12, "size": 1.1}}); code != 400 {
		t.Errorf("a size for a race with no row: %d %v", code, out)
	}
	for _, bad := range []map[string]any{
		{"id": 9, "name": "Second Ogre", "wire_race": 9},
		{"id": entity.LastCustomRace + 1, "name": "Too Far", "wire_race": 9},
		{"id": 12, "name": "", "wire_race": 9},
		{"id": 12, "name": "No Client Has", "wire_race": 10},
		{"id": 12, "name": "Itself", "wire_race": 12},
	} {
		if code, out := c.op("insert_row", map[string]any{"table": "racedefs", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if code, out := c.op("update_row", map[string]any{"table": "racedefs", "key": map[string]any{"id": 11}, "values": map[string]any{"wire_race": 5}}); code != 200 {
		t.Fatalf("an update: %d %v", code, out)
	}
	if entity.WireRace(11) != 5 {
		t.Errorf("the update is not the game's: %d", entity.WireRace(11))
	}

	// a character of it left with no row: the panel says so
	mustExec(t, f.s.d.DB, `INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle, faceoption, sex)
		VALUES ('Bigfellow', 3, 90, 1, 0, 11, 0, 60, 0, 0, 0, 0, 0)`)
	code, out = c.op("delete_row", map[string]any{"table": "racedefs", "key": map[string]any{"id": 11}})
	if code != 200 {
		t.Fatalf("a delete: %d %v", code, out)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "1 characters are of a race with no row") {
		t.Errorf("the delete's live: %v", out["live"])
	}
	if entity.WireRace(11) != 11 || entity.IsCustomRace(11) {
		t.Errorf("the row deleted: wire %d", entity.WireRace(11))
	}
}
