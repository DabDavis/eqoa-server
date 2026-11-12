package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// testerProLine is the Tester Pro's itemeffect line as
// tools/tester_pro_20261112.py writes it (petition 1725).
const testerProLine = "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP to 1, Set All Stats to 1, Unlimited Uses, 30min"

// The Tester Pro used from the item menu (0x003E) twice: its effect on its
// user, one, the item kept and nothing said spent (no 0x003C, no charge's
// 0x003E back). The control: a potion of the same line without "Unlimited
// Uses" is drunk and gone.
func TestTheTesterProIsNeverSpent(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	res, err := f.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 209763529, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 7, 1, 0, 0, 'Tester Pro', '', 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, name, icon, source) VALUES (?, ?, 'Tester Pro', 638503769, 'test')`,
		id, testerProLine); err != nil {
		t.Fatal(err)
	}
	f.own(int(id), 1)
	f.loadList()
	f.sess.Queue().GetPending()
	spent := func(out [][]byte) bool {
		for _, m := range out {
			if bytes.HasPrefix(m, opBytes(protocol.OpcodeRemoveInvItem)) || bytes.HasPrefix(m, opBytes(protocol.OpcodeInteractItem)) {
				return true
			}
		}
		return false
	}
	for i := range 2 {
		out := f.use(0)
		if spent(out) {
			t.Errorf("use %d: the client was told it was spent", i+1)
		}
		n := 0
		for _, e := range f.player.Effects {
			if e.SpellID == entity.PotionEffectID(int(id)) {
				n++
			}
		}
		if n != 1 || f.player.MaxHPNow() != 1 || f.player.StatNow(entity.StatStr) != 1 {
			t.Fatalf("use %d: %d effects, max HP %d, STR %d", i+1, n, f.player.MaxHPNow(), f.player.StatNow(entity.StatStr))
		}
		if f.stackOf("Tester Pro") != 1 {
			t.Fatalf("use %d: %d left, want the one", i+1, f.stackOf("Tester Pro"))
		}
	}
	pot := f.potion("Tester Draught", -1, 3, 1, "Self, Run Speed 2x Spirit of the Wolf, Set Maximum HP to 1, Set All Stats to 1, 30min", "", "")
	f.own(pot, 1)
	f.loadList()
	if out := f.use(1); !spent(out) || f.stackOf("Tester Draught") != 0 {
		t.Fatalf("the control: spent %v, %d left; want spent, none", spent(out), f.stackOf("Tester Draught"))
	}
}
