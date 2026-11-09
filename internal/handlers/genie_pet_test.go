package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Petition 1351 (Jibrish: "All charm lines and pet lines need to be able to be
// out and not lose the pet while they summon the genie in their inventory ...
// Its only for the vendor stones"), end to end through the genie's handlers: a
// master with a summoned pet rubs the lamp and answers each of the genie's
// questions in turn (its bank and a move in it, its wares and a look at them,
// its forge and its close, rehabilitation, dyeing, nothing, a box closed), the
// world ticking between; the pet stays its master's, in the world, and no
// 0x007F (CLIENT's pet bar away) goes out. The control: a second summoning
// sends one and the first pet goes.
func TestTheGeniesAnswersKeepThePet(t *testing.T) {
	f := newGenieFixture(t)
	f.g.rehab = NewRehab(f.db, f.w, f.inv, database.NewSpellbookStore(f.db), nil, database.NewSQLiteCharacterStore(f.db))
	f.g.dyer = NewDyer(f.w, f.db)
	f.p.Class, f.p.World, f.p.Power, f.p.MaxPower = 10, 0, 2000, 2000
	body := &entity.NPC{ServerID: 9001, Name: "Fire Elemental", Level: 30, HP: 900, MaxHP: 900, ModelID: 0x8A416241,
		Size: 1, NPCType: 0x80, X: 5000, Z: 5000}
	f.w.NPCs[body.ServerID] = body
	elemental := entity.CastSpell{SpellID: 717, Level: 30, Name: "Fire Elemental",
		Def: entity.ParseSpellText("Summons a Fire Elemental to aid you")}
	if got := f.w.SummonByItem(f.sess, elemental, f.now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("no pet: %v", got)
	}
	pet := f.w.PetOf(f.p)
	tick := func() {
		for i := 0; i < 5; i++ {
			f.now = f.now.Add(500 * time.Millisecond)
			f.w.Tick(f.now)
		}
	}
	watch := func(msgs [][]byte, when string) {
		t.Helper()
		tick()
		for _, m := range append(msgs, func() (out [][]byte) {
			for _, q := range f.sess.Queue().GetPending() {
				out = append(out, q.Data())
			}
			return out
		}()...) {
			if op := opOf(m); op == protocol.OpcodeClearPet || op == protocol.OpcodeNoCollideRemove {
				t.Errorf("%s: sent %s", when, ops([][]byte{m}))
			}
		}
		if got := f.w.PetOf(f.p); got != pet || f.w.NPCs[pet] == nil {
			t.Fatalf("%s: the pet is %d, want %d (in the world: %v)", when, got, pet, f.w.NPCs[pet] != nil)
		}
	}
	watch(nil, "the pet summoned")

	box := f.summon()
	watch(f.answer(box, genieBank), "the bank")
	watch(f.send(protocol.OpcodeBankItem, func(w *buffer.Writer) {
		w.WriteUint32(uint32(pet)) // CLIENT's target, the pet itself
		w.WriteByte(0)
		w.WriteUint32(0) // the sword's key
		w.Write7BitEncodedInt64(1)
	}), "a move in the bank")
	f.loadLists()
	watch(f.answer(f.summon(), genieShop), "its wares")
	watch(f.answer(f.summon(), genieSmith), "its forge")
	watch(f.send(protocol.OpcodeCloseBlacksmithMenu, func(w *buffer.Writer) {}), "its forge closed")
	watch(f.answer(f.summon(), genieRehab), "rehabilitation")
	watch(f.answer(f.summon(), genieDye), "dyeing")
	watch(f.answer(f.summon(), genieNothing), "nothing, thank you")
	box = f.summon()
	f.now = f.now.Add(3 * time.Second)
	watch(f.answer(box, 255), "the box closed")
	f.summon()
	f.now = f.now.Add(entity.GenieWait + 5*time.Second)
	watch(nil, "the genie's wait out")
	if f.genieNPC() != nil {
		t.Error("the genie outlived its wait")
	}

	// the control: a second summoning sends 0x007F, and the first pet goes
	f.sess.Queue().GetPending()
	if got := f.w.SummonByItem(f.sess, entity.CastSpell{SpellID: 718, Level: 30, Name: "Fire Elemental Two",
		Def: entity.ParseSpellText("Summons a Fire Elemental to aid you")}, f.now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("the control: no second pet: %v", got)
	}
	cleared := false
	for _, q := range f.sess.Queue().GetPending() {
		cleared = cleared || opOf(q.Data()) == protocol.OpcodeClearPet
	}
	if !cleared || f.w.PetOf(f.p) == pet || f.w.NPCs[pet] != nil {
		t.Errorf("the control: 0x007F sent %v, the first pet still out %v", cleared, f.w.NPCs[pet] != nil)
	}
}
