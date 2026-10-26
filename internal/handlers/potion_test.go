package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Potions against the scribe's fixture (a level 23 druid) in a real World:
// an item with an itemeffect row is used, not worn, and one leaves the stack
// by 0x003C; a spell's summoned items arrive as a sale's do (0x003B). The
// items' icons and the rows' numbers are made up.

// potionWorld puts the fixture's player in a World the handler uses.
func (f *scribeFixture) potionWorld() *entity.World {
	f.t.Helper()
	w := entity.NewWorld()
	f.player.HP, f.player.MaxHP, f.player.Power, f.player.MaxPower = 500, 600, 100, 500
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.SetGameWorld(w)
	return w
}

// potion adds an item worn at wear (-1 nowhere) stacking to maxStack, and its
// itemeffect row.
func (f *scribeFixture) potion(name string, wear, maxStack, trade int, line, effectName, breaks string) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 209763529, 0, ?, 0, ?, 1, 0, 10, 20, 20, 1, ?, 0, 50, 32767, 1023, 2, 0, 0, 0, ?, '', 0, 0)`,
		wear, trade, maxStack, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, name, icon, breaks) VALUES (?, ?, ?, 2585, ?)`,
		id, line, effectName, breaks); err != nil {
		f.t.Fatal(err)
	}
	return int(id)
}

// useAs sends UseItem(key, interaction) at the target, as the item menu (0)
// or a quick slot (1) does.
func (f *scribeFixture) useAs(key, target uint32, interaction byte) [][]byte {
	f.t.Helper()
	payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(target)
		w.WriteByte(interaction)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(payload)
	r := NewRegistry()
	f.h.RegisterHandlers(r)
	h, _ := r.Get(protocol.OpcodeInteractItem)
	if err := h(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// A draught drunk from the item menu: its health and power, its effect by the
// row's name, and one of three taken off by 0x003C (index 0, sound 0, one).
func TestDrinkPotionTakesOne(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	d := f.potion("Watery Draught", -1, 3, 1, "Self, Decrease Hitpoints by 20, Increase Power per tic by 25, 18sec", "", "")
	f.own(d, 3)
	f.loadList()
	msgs := f.useAs(0, 0xFFFFFFFF, 0)
	var removed []byte
	for _, m := range msgs {
		if protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == protocol.OpcodeRemoveInvItem {
			removed = m
		}
	}
	if want := mustHex(t, "3c00000002"); !bytes.Equal(removed, want) {
		t.Errorf("0x003C % X, want % X", removed, want)
	}
	if f.player.HP != 480 || f.player.Power != 125 {
		t.Errorf("drunk: %d HP, %d power; want 480, 125", f.player.HP, f.player.Power)
	}
	if len(f.player.Buffs) != 1 || f.player.Buffs[0].Name != "Watery Draught" {
		t.Errorf("effects channel %+v", f.player.Buffs)
	}
	if n := f.stackOf("Watery Draught"); n != 2 {
		t.Errorf("%d left, want 2", n)
	}
}

// A thrown potion is a ranged weapon (F3 20261026-014336): a quick slot's Use
// (as the item menu's) wears it in the ranged slot (0x003F, equip slot 18)
// and throws nothing; only one worn nowhere (a GM's) is thrown by Use, and
// with no target refused in red and kept.
func TestThrownPotionFromAQuickSlot(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	p := f.potion("Unstable Potion", 16, 999, 0, "Target, Decrease Hitpoints by 54", "", "")
	f.own(p, 5)
	f.loadList()
	msgs := f.useAs(0, 0xFFFFFFFF, 1)
	equipped := false
	for _, m := range msgs {
		if bytes.Contains(m, []byte("N\x00o\x00 \x00t\x00a\x00r\x00g\x00e\x00t\x00")) {
			t.Error("a worn thrown potion was thrown from the quick slot (\"No target.\")")
		}
		if protocol.GameOpcode(uint16(m[0])|uint16(m[1])<<8) == protocol.OpcodeEquipItem && len(m) >= 4 && m[3] == 18 {
			equipped = true
		}
	}
	if !equipped {
		t.Errorf("sent % X; want 0x003F into the ranged slot (18)", msgs)
	}
	if n := f.stackOf("Unstable Potion"); n != 5 {
		t.Errorf("worn, yet %d left", n)
	}

	// control: a thrown potion worn nowhere is thrown by Use
	g := newScribeFixture(t)
	g.potionWorld()
	gm := g.potion("Odd Potion", -1, 999, 0, "Target, Decrease Hitpoints by 54", "", "")
	g.own(gm, 5)
	g.loadList()
	msgs = g.useAs(0, 0xFFFFFFFF, 1)
	if len(msgs) != 1 || !bytes.Contains(msgs[0], []byte("N\x00o\x00 \x00t\x00a\x00r\x00g\x00e\x00t\x00")) {
		t.Errorf("sent % X; want the red \"No target.\"", msgs)
	}
	if n := g.stackOf("Odd Potion"); n != 5 {
		t.Errorf("refused, yet %d left", n)
	}

	th := entity.NewPotionEffect(1, "Unstable Potion", "Target, Decrease Hitpoints by 54", "", 0, "", 0, 0)
	dr := entity.NewPotionEffect(2, "Draught", "Self, Increase Power per tic by 25", "", 0, "", 0, 0)
	for _, in := range []byte{0, 1} {
		if usesPotion(&th, &database.ItemPatternRecord{EquipSlot: 16}, in) {
			t.Errorf("a worn thrown potion is thrown by Use (interaction %d), not worn", in)
		}
		if !usesPotion(&th, &database.ItemPatternRecord{EquipSlot: -1}, in) ||
			!usesPotion(&dr, &database.ItemPatternRecord{EquipSlot: -1}, in) {
			t.Errorf("a draught, or a thrown potion worn nowhere, is not used (interaction %d)", in)
		}
	}
}

// A spell's summoned potions: three of one that stacks to three when its
// words say no number, as many as they say otherwise, the spell's own name
// when no item has the words' name, nothing for no item at all.
func TestSummonedPotionsArrive(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	quick := f.potion("Faulty Alacrity Potion", -1, 3, 1, "Self, Sow, 30min", "Crude Quickening", "attack")
	f.potion("Unstable Potion", 17, 999, 0, "Target, Decrease Hitpoints by 54", "", "")
	// the spell's scroll shares the potion's name, and comes first by id
	f.item("Neriak Cocktail", true, 1<<14, 40, 1)
	f.potion("Neriak Cocktail", 17, 999, 0, "Target, Decrease Hitpoints by 626", "", "")
	f.loadList()

	f.h.summonItem(f.sess, f.sid, "faulty alacrity potion", 0, "Crude Quickening")
	f.h.summonItem(f.sess, f.sid, "unstable potion", 0, "Unstable Mixture") // no number: three
	f.h.summonItem(f.sess, f.sid, "unstable potion", 3, "Unstable Mixtures")
	f.h.summonItem(f.sess, f.sid, "grobb cocktail", 0, "Neriak Cocktail")
	f.h.summonItem(f.sess, f.sid, "ring to invis", 0, "Phase Ring")
	f.item("Lava Stone", true, 1<<10, 10, 1) // the spell's scroll, the only item of its name
	f.h.summonItem(f.sess, f.sid, "item", 0, "Lava Stone")
	adds := 0
	for _, m := range f.sess.Queue().GetPending() {
		if d := m.Data(); protocol.GameOpcode(uint16(d[0])|uint16(d[1])<<8) == protocol.OpcodeAddInvItem {
			adds++
		}
	}
	if adds != 4 {
		t.Errorf("%d items arrived (0x003B), want 4 (never a spell's scroll)", adds)
	}
	for name, want := range map[string]int{"Faulty Alacrity Potion": 3, "Unstable Potion": 6, "Neriak Cocktail": 3} {
		if n := f.stackOf(name); n != want {
			t.Errorf("%d %s, want %d", n, name, want)
		}
	}
	for _, r := range f.rowsOf() {
		if r.Pattern.ItemName == "Neriak Cocktail" && isScroll(&r.Pattern) {
			t.Error("Neriak Cocktail summoned its spell's scroll, not the potion")
		}
	}
	if summonCount(&database.ItemPatternRecord{PatternID: quick, MaxStack: 3}, true) != 3 ||
		summonCount(&database.ItemPatternRecord{MaxStack: 999}, true) != 3 ||
		summonCount(&database.ItemPatternRecord{MaxStack: 3}, false) != 3 ||
		summonCount(&database.ItemPatternRecord{MaxStack: 999}, false) != 1 ||
		summonCount(&database.ItemPatternRecord{MaxStack: 1}, false) != 1 {
		t.Error("summonCount")
	}
}

// A NO TRADE item (itempattern.trade 1, CLIENT's m_noTrade: a crude potion) is
// never offered in a trade; the control, a tradeable one, is.
func TestNoTradeItemIsNotOffered(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	crude := f.potion("Faulty Alacrity Potion", -1, 3, 1, "Self, Sow, 30min", "Crude Quickening", "attack")
	fine := f.potion("Self Alacrity Potion", -1, 3, 0, "Self, Sow, 30min", "Quickening", "attack")
	f.own(crude, 3)
	f.own(fine, 3)
	f.loadList()
	other := &entity.Player{ServerID: 1002, Name: "Other"}
	h := NewTradeHandler(nil, &NPCWindowHandler{inv: f.inv})
	h.SetGameWorld(w)
	me := &tradeSide{sess: f.sess, player: f.player, asker: true}
	you := &tradeSide{sess: testSession(), player: other}
	tr := &trade{a: me, b: you, open: true}
	h.offerLocked(tr, me, you, 0, 1)
	if len(me.offers) != 0 {
		t.Errorf("a no-trade potion was offered: %+v", me.offers)
	}
	h.offerLocked(tr, me, you, 1, 1)
	if len(me.offers) != 1 {
		t.Errorf("the control: a tradeable potion was not offered (%+v)", me.offers)
	}
}

// A thrown potion a GM made after the start is thrown in an auto-attack all
// the same: its row is read when the World does not know it; an item with no
// row is no potion.
func TestThrownPotionKnownFromItsRow(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	id := f.potion("Volatile Potion", 17, 999, 0, "Target, Decrease Hitpoints by 100", "", "")
	if w.ThrownPotion(id) {
		t.Fatal("the control: the World knew the new potion already")
	}
	p := &database.ItemPatternRecord{PatternID: id, ItemName: "Volatile Potion", AttackType: 10, EquipSlot: 17}
	if !thrownPotion(w, f.inv, p) || !w.ThrownPotion(id) {
		t.Error("a potion with a row is not thrown, or not kept")
	}
	knife := &database.ItemPatternRecord{PatternID: f.item("rusty throwing knife", false, 32767, 1, 99), AttackType: 10, EquipSlot: 16}
	if thrownPotion(w, f.inv, knife) {
		t.Error("a throwing knife with no row was taken for a potion")
	}
}
