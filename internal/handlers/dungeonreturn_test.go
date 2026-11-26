package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The Scroll of Returning through the item's Use (0x003E): read in town with no place
// kept it says where it works and stays; read in a heroic dungeon's instance it marks
// the place, says so and stays; read in town again it takes its reader back and is
// spent. The controls: a Hardcore character and an [OG] character are refused and keep
// the scroll; a spell scroll of the same icon is still scribed, not read as this.
func TestTheScrollOfReturningOutAndBack(t *testing.T) {
	r := newRockRig(t)
	res, err := r.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (500, 0, ?, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 2, 0, 0, 0, 'Scroll of Returning', '', 0, 0)`, scrollIcon)
	if err != nil {
		t.Fatal(err)
	}
	id64, _ := res.LastInsertId()
	scroll := int(id64)
	if _, err := r.inv.AddCharItem(r.sid, scroll, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	held := func() int {
		rows, _ := r.inv.GetCharacterItems(r.sid)
		n := 0
		for _, row := range rows {
			if row.Pattern.PatternID == scroll {
				n += row.StackLeft
			}
		}
		return n
	}
	defs := []entity.DungeonDef{{Key: "guk", Name: "Test Guk", Boxes: []entity.DungeonBox{{X0: 900, Z0: 900, X1: 1100, Z1: 1100}},
		Entrance: entity.HomeSpot{X: 1000, Y: 50, Z: 1050}}}
	r.gw.SetDungeons(true, 14, defs, entity.DungeonSettings{})
	t.Cleanup(func() { r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{}) })

	sent := r.use(r.key(scroll), 0)
	if got := textOf(sent, protocol.OpcodeErrorMessage); !strings.Contains(got, "only within a heroic dungeon") || held() != 1 {
		t.Fatalf("in town: %q, %d held", got, held())
	}
	if why, ok := r.gw.DungeonEnter(r.sess, "guk", time.Now()); !ok {
		t.Fatal(why)
	}
	r.gw.PlaceRespawn(r.p.ServerID, 1010, 50, 990, 14)

	// refused: Hardcore, [OG]
	entity.SetHardcoreOn(true)
	r.p.Hardcore = true
	sent = r.use(r.key(scroll), 0)
	r.p.Hardcore = false
	entity.SetHardcoreOn(false)
	if got := textOf(sent, protocol.OpcodeErrorMessage); got != returnHardcoreWords {
		t.Errorf("Hardcore: %q", got)
	}
	r.sess.SetOG(true)
	sent = r.use(r.key(scroll), 0)
	r.sess.SetOG(false)
	if got := textOf(sent, protocol.OpcodeErrorMessage); got != returnQoLWords {
		t.Errorf("[OG]: %q", got)
	}
	if _, _, _, _, ok := r.gw.DungeonReturnOf(r.sid); ok {
		t.Fatal("a refused read kept a place")
	}

	sent = r.use(r.key(scroll), 0)
	if !strings.Contains(chatTexts(sent), "marks your place in Test Guk") || held() != 1 {
		t.Fatalf("out: %q (%s), %d held", chatTexts(sent), ops(sent), held())
	}
	if inst, x, _, z, ok := r.gw.DungeonReturnOf(r.sid); !ok || inst == 0 || x != 1010 || z != 990 {
		t.Fatalf("the place: %d %v %v %v", inst, x, z, ok)
	}
	r.gw.Tick(time.Now()) // the trip home goes
	r.sent()
	r.gw.PlaceRespawn(r.p.ServerID, 9000, 50, 9000, 0)

	sent = r.use(r.key(scroll), 0)
	if !strings.Contains(chatTexts(sent), "back into Test Guk") || held() != 0 {
		t.Fatalf("back: %q (%s), %d held", chatTexts(sent), ops(sent), held())
	}
}

// chatTexts is the text of every coloured chat line among msgs (ASCII), joined.
func chatTexts(msgs [][]byte) string {
	var out []string
	for _, m := range msgs {
		if len(m) >= 2 && opOf(m) == protocol.OpcodeColoredChat {
			out = append(out, strings.ReplaceAll(string(m[2:]), "\x00", "")) // UTF-16LE of ASCII: its low bytes
		}
	}
	return strings.Join(out, "|")
}
