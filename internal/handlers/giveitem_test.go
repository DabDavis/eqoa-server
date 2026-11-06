package handlers

import (
	"strconv"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// #giveitem <item id> <player> [count] (petition 1046): a GM gives an online
// player an item, in either spelling, the player told; a player's own is an
// unknown command; a player not online, a bad id or count, said and nothing
// given.
func TestGiveItemCommand(t *testing.T) {
	c, gs, _, ps, pp := gmWorld(t)
	db, err := database.NewSQLite(t.TempDir() + "/give.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv := database.NewInventoryStore(db)
	c.h.SetGMStores(inv, nil)
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (5, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Test Fish', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	fish, _ := res.LastInsertId()
	gs.SetInventoryList(newInvList(nil))
	ps.SetInventoryList(newInvList(nil))
	held := func() int {
		items, err := inv.GetCharacterItems(pp.CharServerID)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range items {
			n += e.StackLeft
		}
		return n
	}
	id := strconv.Itoa(int(fish))
	c.say(gs, "#giveitem "+id+" player")
	if held() != 1 {
		t.Fatalf("#giveitem: %d held", held())
	}
	if got := texts(heard(t, ps)); !hasLine(got, "A GM gave you Test Fish.") {
		t.Fatalf("the player was not told: %q", got)
	}
	c.say(gs, "/giveitem "+id+" Player 3")
	if held() != 4 {
		t.Fatalf("/giveitem with a count: %d held", held())
	}
	gs.Queue().GetPending()
	for _, bad := range []string{"#giveitem " + id + " Nobody", "#giveitem fish Player", "#giveitem " + id + " Player 0", "#giveitem " + id} {
		c.say(gs, bad)
	}
	if held() != 4 {
		t.Fatalf("a refused #giveitem gave: %d held", held())
	}
	if got := gmReplies(t, gs); len(got) < 4 {
		t.Fatalf("the refusals said: %q", got)
	}
	ps.Queue().GetPending()
	c.say(ps, "#giveitem "+id+" Player")
	if held() != 4 {
		t.Fatal("a player's own #giveitem gave")
	}
}
