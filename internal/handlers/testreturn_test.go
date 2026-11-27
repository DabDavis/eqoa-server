package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

func TestTestScrollOfReturningItemUse(t *testing.T) {
	r := newRockRig(t)
	if _, err := r.db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade,
		rent, unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim,
		lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, ?, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 32767, 2, 0, 0, 0, ?, '', 0, 0)`,
		database.TestReturnScrollID, scrollIcon, testReturnName); err != nil {
		t.Fatal(err)
	}
	id := database.TestReturnScrollID
	quantity := func() int {
		t.Helper()
		rows, err := r.inv.GetCharacterItems(r.sid)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.Pattern.PatternID == id {
				return row.StackLeft
			}
		}
		return 0
	}
	r.h.summonItem(r.sess, r.sid, testReturnName, 1, "Test summon")
	if quantity() != 0 {
		t.Fatal("a spell summoned the GM-issued scroll")
	}
	itemID, err := r.inv.AddCharItem(r.sid, id, 2, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	r.gw.SetBind(r.p.ServerID, entity.Bind{Set: true, World: 0, X: 9000, Y: 50, Z: 9000, By: "Spiritmaster"})
	const x, y, z, facing = 9617.25, 54.12, 5705.45, 1.25
	r.gw.PlaceFacing(r.p.ServerID, x, y, z, 1, facing)

	first := r.use(r.key(id), 0)
	if !strings.Contains(chatTexts(first), "marks your place") || quantity() != 2 {
		t.Fatalf("mark response %q, %d scrolls", chatTexts(first), quantity())
	}
	r.gw.Tick(time.Now())
	r.sent()
	if r.p.World != 0 || r.p.X != 9000 || r.p.Y != 50 || r.p.Z != 9000 {
		t.Fatalf("bind landing world %d at %.2f, %.2f, %.2f", r.p.World, r.p.X, r.p.Y, r.p.Z)
	}
	if sent := r.use(r.key(id), 1); !strings.Contains(textOf(sent, protocol.OpcodeErrorMessage), "again in") || quantity() != 2 {
		t.Fatalf("quick slot during cooldown: %q, %d scrolls", textOf(sent, protocol.OpcodeErrorMessage), quantity())
	}
	key := r.key(id)
	row, err := r.inv.GetCharItem(r.sid, itemID)
	if err != nil {
		t.Fatal(err)
	}
	r.h.useTestReturnScroll(r.sess, r.p, int(key), &row, time.Now().Add(5*time.Second))
	if sent := r.sent(); !strings.Contains(chatTexts(sent), "back to the marked spot") || quantity() != 1 {
		t.Fatalf("return response %q, %d scrolls", chatTexts(sent), quantity())
	}
	r.gw.Tick(time.Now())
	if r.p.World != 1 || r.p.X != x || r.p.Y != y || r.p.Z != z || r.p.Facing != facing {
		t.Fatalf("return landing world %d at %.2f, %.2f, %.2f facing %.2f", r.p.World, r.p.X, r.p.Y, r.p.Z, r.p.Facing)
	}
}
