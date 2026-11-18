package handlers

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// pickerOpened is a picker's open (OpcodeDyePicker kind 1) as read back.
type pickerOpened struct {
	token   uint32
	target  uint8
	alpha   bool
	current uint32
	title   string
	slots   []uint8
	pet     dyePetPreview
	aura    dyeAuraPreview
	palette []uint32
}

// readPickerOpen reads encodeDyePickerOpen's message (opcode first); false
// when it is not one or is short.
func readPickerOpen(d []byte) (pickerOpened, bool) {
	var o pickerOpened
	le := binary.LittleEndian
	if len(d) < 13 || le.Uint16(d) != uint16(protocol.OpcodeDyePicker) || d[2] != DyePickerOpen {
		return o, false
	}
	o.token, o.target, o.alpha, o.current = le.Uint32(d[3:]), d[7], d[8]&1 != 0, le.Uint32(d[9:])
	r := d[13:]
	take := func(n int) []byte {
		if len(r) < n {
			r = nil
			return make([]byte, n)
		}
		b := r[:n]
		r = r[n:]
		return b
	}
	o.title = string(take(int(take(1)[0])))
	o.slots = append([]uint8(nil), take(int(take(1)[0]))...)
	switch o.target {
	case DyePickPet:
		o.pet.id, o.pet.look = le.Uint32(take(4)), le.Uint32(take(4))
		o.pet.dye = [2]uint32{le.Uint32(take(4)), le.Uint32(take(4))}
		o.pet.mask = le.Uint64(take(8))
		o.pet.rule = take(1)[0] == entity.PetDyesByRule
	case DyePickAura:
		o.aura.kind, o.aura.source, o.aura.nodes = take(1)[0], le.Uint32(take(4)), take(1)[0]
	}
	n := int(take(1)[0])
	for i := 0; i < n; i++ {
		o.palette = append(o.palette, le.Uint32(take(4)))
	}
	return o, r != nil && len(r) == 0
}

// pickerMsgs is the picker's messages queued to s (left queued: questMsgs
// drains them after).
func pickerMsgs(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 3 && binary.LittleEndian.Uint16(d) == uint16(protocol.OpcodeDyePicker) {
			out = append(out, d)
		}
	}
	return out
}

// answerPicker is the client's OpcodeDyePickerAnswer of kind with its body.
func answerPicker(t *testing.T, h *DyePickerHandler, s *session.Session, kind byte, body ...uint32) {
	t.Helper()
	data := []byte{kind}
	for _, v := range body {
		data = binary.LittleEndian.AppendUint32(data, v)
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDyePickerAnswer)
	msg.SetData(data)
	if err := h.HandleDyePicker(s, msg); err != nil {
		t.Fatal(err)
	}
}

// The offer is a u8 kind 0 and the server's version; the client's answer of
// kind 0 sets the session's (held to the server's), 0 none.
func TestTheDyePickerOffer(t *testing.T) {
	tw := newTalkWorld(t, 100, 100)
	SendDyePickerOffer(tw.s)
	got := pickerMsgs(tw.s)
	if len(got) != 1 || string(got[0]) != string([]byte{0x95, 0x7E, DyePickerOffer, session.DyePickerVersion}) {
		t.Fatalf("the offer: % X", got)
	}
	h := NewDyePickerHandler(tw.w)
	if tw.s.DyePicker() != 0 {
		t.Fatal("a client that never answered speaks the picker")
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDyePickerAnswer)
	msg.SetData([]byte{DyePickerOffer, 7})
	h.HandleDyePicker(tw.s, msg)
	if tw.s.DyePicker() != session.DyePickerVersion {
		t.Fatalf("answered 7: version %d", tw.s.DyePicker())
	}
	msg.SetData([]byte{DyePickerOffer, 0})
	h.HandleDyePicker(tw.s, msg)
	if tw.s.DyePicker() != 0 {
		t.Fatal("answered 0: still on")
	}
}

// The Equipment Dyer's picker: a client that never answered the offer (a PS2,
// an old vanilla) is asked the families alone, as it always was, and sent no
// picker; one that did is asked no families at all: the piece chosen opens its
// picker (the slots, the colour now, the palette) and the colour it answers dyes the
// slots as a palette colour does, alpha FF. The controls: an answered picker
// answered again, a cancelled one, one answered from too far or too late, and
// a token never opened dye nothing.
func TestTheDyerPicker(t *testing.T) {
	tw := newTalkWorld(t, 100, 100)
	db, err := database.NewSQLite(t.TempDir() + "/dyepick.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	d := NewDyer(tw.w, db)
	dyer := &entity.NPC{ServerID: 3101, Name: EquipmentDyerName, NPCType: 0x80, X: 105, Y: 54, Z: 100}
	h := NewDyePickerHandler(tw.w)
	questMsgs(tw.s)
	clock := time.Unix(1_800_000_000, 0)
	picks.now = func() time.Time { return clock }
	t.Cleanup(func() { picks.now = time.Now })

	pick := func(msgs []string, n byte) []string {
		t.Helper()
		id := boxID(msgs)
		if id == 0 || !d.Answer(tw.s, tw.p, id, n) {
			t.Fatalf("no dyer's box in %q", msgs)
		}
		return questMsgs(tw.s)
	}
	talk := func() []string {
		t.Helper()
		if !d.Talk(tw.s, tw.p, dyer) {
			t.Fatal("no dyer")
		}
		return questMsgs(tw.s)
	}
	chest := func() uint32 {
		t.Helper()
		c, err := db.CharDyes(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		return c[database.DyeChest]
	}
	// to the families of the chest (One armor piece, Chest)
	families := func() []string { return pick(pick(talk(), 1), 0) }

	// an old client: the palette alone, and its seventh answer nothing
	got := families()
	if last := got[len(got)-1]; strings.Contains(last, DyePickLabel) || !strings.HasSuffix(last, "|Purples") {
		t.Fatalf("an old client's families: %q", got)
	}
	pick(got, byte(len(database.DyeFamilies)))
	if len(pickerMsgs(tw.s)) != 0 {
		t.Fatal("an old client was sent a picker")
	}

	tw.s.SetDyePicker(1)
	tw.s.SetDialogRoom(12)
	if _, _, err := db.SetCharDye(rec.ServerID, database.DyeChest, 0xD10000FF, database.GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	// a box of twelve: the first question holds the pieces themselves; a
	// piece chosen opens the picker at once, no palette asked (the owner,
	// 2026-11-18)
	// to the piece: its box answered, the picker's messages read before the
	// box's drain them
	toPicker := func(msgs []string, n byte) pickerOpened {
		t.Helper()
		id := boxID(msgs)
		if id == 0 || !d.Answer(tw.s, tw.p, id, n) {
			t.Fatalf("no dyer's box in %q", msgs)
		}
		opens := pickerMsgs(tw.s)
		said := strings.Join(questMsgs(tw.s), "\n")
		if strings.Contains(said, "Purples") || strings.Contains(said, DyePickLabel) {
			t.Fatalf("a picker's client was asked the palette: %q", said)
		}
		if len(opens) != 1 {
			t.Fatalf("picker messages: %d (said %q)", len(opens), said)
		}
		o, ok := readPickerOpen(opens[0])
		if !ok {
			t.Fatalf("not an open: % X", opens[0])
		}
		return o
	}
	families = func() []string { return talk() }
	open := func(got []string) pickerOpened { return toPicker(got, 1) }
	got = families()
	o := open(got)
	if o.target != DyePickChar || o.alpha || o.current != 0xD10000FF || len(o.slots) != 1 || o.slots[0] != database.DyeChest ||
		o.title != "Dye chest" || len(o.palette) != len(dyePalette()) || o.palette[0] != 0xFFFFFFFF {
		t.Fatalf("the open: %+v", o)
	}

	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x3FA7D600)
	if c := chest(); c != 0x3FA7D6FF {
		t.Fatalf("the chest picked: %08X", c)
	}
	if got := questMsgs(tw.s); !hasMsg(got, "box:"+EquipmentDyerName+": There: #3FA7D6.") {
		t.Errorf("said: %q", got)
	}
	if _, _, on, ok := tw.w.DyeTarget(tw.s); !ok || on[database.DyeChest] != 0x3FA7D6FF {
		t.Errorf("on the character: %08X", on[database.DyeChest])
	}
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	if c := chest(); c != 0x3FA7D6FF {
		t.Fatalf("an answered picker answered again: %08X", c)
	}

	// cancelled: nothing, and its colour after nothing
	o = open(families())
	answerPicker(t, h, tw.s, DyePickerCancel, o.token)
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	if c := chest(); c != 0x3FA7D6FF {
		t.Fatalf("a cancelled picker dyed: %08X", c)
	}

	// a palette colour is said by its name, black is black
	o = open(families())
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x101010FF)
	if got := questMsgs(tw.s); chest() != 0x101010FF || !hasMsg(got, "box:"+EquipmentDyerName+": There: black.") {
		t.Fatalf("black: %08X %q", chest(), got)
	}

	// too far from the dyer: refused, nothing dyed
	o = open(families())
	tw.p.X += dyePickReach + 10
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	if got := questMsgs(tw.s); chest() != 0x101010FF || !hasMsg(got, "box:"+EquipmentDyerName+": Come back to me to be dyed.") {
		t.Fatalf("too far: %08X %q", chest(), got)
	}
	tw.p.X -= dyePickReach + 10

	// in another world: refused
	o = open(families())
	tw.p.World = 3
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	tw.p.World = 0
	if chest() != 0x101010FF {
		t.Fatalf("another world dyed: %08X", chest())
	}

	// too late: refused
	o = open(families())
	clock = clock.Add(dyePickFor + time.Second)
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	if got := questMsgs(tw.s); chest() != 0x101010FF || !hasMsg(got, "box:"+EquipmentDyerName+": My dyes have dried. Ask me again.") {
		t.Fatalf("too late: %08X %q", chest(), got)
	}

	// a token never opened, and talking again closes the one open
	o = open(families())
	answerPicker(t, h, tw.s, DyePickerPicked, o.token+1, 0x00FF00FF)
	talk()
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x00FF00FF)
	if chest() != 0x101010FF {
		t.Fatalf("a token not open dyed: %08X", chest())
	}

	// several slots: All armor, every one of them, each in the colour
	o = toPicker(talk(), 0)
	if len(o.slots) != 7 {
		t.Fatalf("all armor's slots: %v", o.slots)
	}
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0xFFFFFFFF)
	c, _ := db.CharDyes(rec.ServerID)
	for _, s := range o.slots {
		if c[s] != 0xFFFFFFFF {
			t.Errorf("all armor white: %s %08X", database.DyeSlotNames[s], c[s])
		}
	}

	// No dye (version 2; the owner, 2026-11-18): the chest's dye off, the
	// rest of the armour white as it was
	o = open(families())
	answerPicker(t, h, tw.s, DyePickerNoDye, o.token)
	c, _ = db.CharDyes(rec.ServerID)
	if got := questMsgs(tw.s); c[database.DyeChest] != 0 || !hasMsg(got, "box:"+EquipmentDyerName+": The dye is off.") {
		t.Fatalf("No dye: the chest %08X, said %q", c[database.DyeChest], got)
	}
	for _, s := range o.slots {
		if s != database.DyeChest && c[s] != 0xFFFFFFFF {
			t.Errorf("No dye on the chest took %s's: %08X", database.DyeSlotNames[s], c[s])
		}
	}
	if _, _, on, ok := tw.w.DyeTarget(tw.s); !ok || on[database.DyeChest] != 0 {
		t.Errorf("on the character: %08X", on[database.DyeChest])
	}
}

// The Pet Groomer's picker: a picker's client is asked no families, the
// piece chosen opens it; the open names the pet as OpcodePetDyes would (its id in the client's view, its
// body, its dyes, its rule) and offers the alpha, which the colour keeps.
func TestTheGroomerPicker(t *testing.T) {
	f := newGenieFixture(t)
	f.p.Class, f.p.World, f.p.Power, f.p.MaxPower = 10, 0, 2000, 2000
	const look = 0x8A416241
	body := &entity.NPC{ServerID: 9001, Name: "Fire Elemental", Level: 30, HP: 900, MaxHP: 900, ModelID: look,
		Size: 1, NPCType: 0x80, X: 5000, Z: 5000}
	f.w.NPCs[body.ServerID] = body
	g := NewGroomer(f.w, f.db)
	groomer := &entity.NPC{ServerID: 3102, Name: "Pet Groomer", NPCType: 0x80, X: f.p.X, Y: f.p.Y, Z: f.p.Z}
	f.sess.SetDyePicker(1)
	f.sess.SetDialogRoom(12)
	h := NewDyePickerHandler(f.w)
	elemental := entity.CastSpell{SpellID: 717, Level: 30, Name: "Fire Elemental", Def: entity.ParseSpellText("Summons a Fire Elemental to aid you")}
	if got := f.w.SummonByItem(f.sess, elemental, f.now, time.Time{}); got != entity.ItemSummoned {
		t.Fatalf("no pet: %v", got)
	}
	questMsgs(f.sess)
	if !g.Talk(f.sess, f.p, groomer) {
		t.Fatal("no groomer")
	}
	got := questMsgs(f.sess)
	if !g.Answer(f.sess, f.p, boxID(got), 0) { // its whole body
		t.Fatal("not the groomer's")
	}
	// the picker at once, no palette asked (the owner, 2026-11-18)
	msgs := pickerMsgs(f.sess)
	if got := questMsgs(f.sess); strings.Contains(strings.Join(got, "\n"), "Purples") {
		t.Fatalf("a picker's client was asked the palette: %q", got)
	}
	questMsgs(f.sess)
	if len(msgs) != 1 {
		t.Fatalf("picker messages: %d", len(msgs))
	}
	o, ok := readPickerOpen(msgs[0])
	if !ok || o.target != DyePickPet || !o.alpha || len(o.slots) != 1 || o.slots[0] != 1 || o.pet.look != look || !o.pet.rule {
		t.Fatalf("the open: %v %+v", ok, o)
	}
	answerPicker(t, h, f.sess, DyePickerPicked, o.token, 0x3FA7D680)
	d, _ := f.db.PetDyesOf(f.p.CharServerID)
	if d[look] != [2]uint32{0, 0x3FA7D680} {
		t.Fatalf("the pet's dyes: %08X", d[look])
	}
	if pet, ok := f.w.PetLookOf(f.sess); !ok || pet.Dye != (entity.PetDye{0, 0x3FA7D680}) {
		t.Errorf("on the pet: %+v", pet)
	}

	// No dye on its whole body (its second area): that dye off, the other
	// area's kept
	if _, _, err := f.db.SetPetDye(f.p.CharServerID, look, 0, 0x101010FF, database.GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	g.Talk(f.sess, f.p, groomer)
	got = questMsgs(f.sess)
	g.Answer(f.sess, f.p, boxID(got), 0)
	msgs = pickerMsgs(f.sess)
	said := questMsgs(f.sess)
	o, ok = readPickerOpen(msgs[len(msgs)-1])
	if !ok || len(o.slots) != 1 || o.slots[0] != 1 {
		t.Fatalf("the body's open: %v %+v", ok, o)
	}
	answerPicker(t, h, f.sess, DyePickerNoDye, o.token)
	d, _ = f.db.PetDyesOf(f.p.CharServerID)
	if got := questMsgs(f.sess); d[look] != [2]uint32{0x101010FF, 0} || !hasMsg(got, "box:Pet Groomer: The dye is off.") {
		t.Fatalf("No dye on the body: %08X, said %q (before %q)", d[look], got, said)
	}
}

// The Weapon Enchanter's colours are its own or "Pick any colour..." alone for
// a client with the picker; the open previews the aura on the hand's place, and the
// colour picked is enchanted as a palette's (the price paid).
func TestTheEnchanterPicker(t *testing.T) {
	t.Cleanup(func() { database.SetInstanceAuraCache(nil); SetEnchanterPrice(-1, "", 0) })
	tw := newTalkWorld(t, 100, 100)
	db, err := database.NewSQLite(t.TempDir() + "/enchantpick.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	ps := database.NewItemPatternStore(db)
	if err := ps.UpsertItemPattern(&database.ItemPatternRecord{PatternID: 900200, ItemName: "keen sword", EquipSlot: 14, MaxHP: 100,
		MaxStack: 1, ClassUse: 32767, RaceUse: 32767, Rent: 1}); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	sword, _ := inv.AddCharItem(rec.ServerID, 900200, 1, 100, 0, 0)
	inv.SetEquipLoc(rec.ServerID, sword, 14)
	rows, _ := inv.GetCharacterItems(rec.ServerID)
	tw.s.SetInventoryList(newInvList(rows))
	tw.p.SetWornPiece(entity.AuraMainHand, 900200)
	tw.p.WornItems[entity.AuraMainHand] = sword
	SetEnchanterPrice(0, "", 0)
	e := NewEnchanter(tw.w, db)
	enchanter := &entity.NPC{ServerID: 3201, Name: WeaponEnchanterName, NPCType: 0x80, X: 100, Y: 54, Z: 100}
	h := NewDyePickerHandler(tw.w)
	tw.s.SetDyePicker(1)
	tw.s.SetDialogRoom(12)
	questMsgs(tw.s)
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		if !e.Answer(tw.s, tw.p, boxID(msgs), n) {
			t.Fatalf("not the enchanter's: %q", msgs)
		}
		return questMsgs(tw.s)
	}
	e.Talk(tw.s, tw.p, enchanter)
	got := pick(pick(questMsgs(tw.s), 1), 0) // the second aura; Virtuous
	// its own colours or the picker, no palette (the owner, 2026-11-18)
	if last := got[len(got)-1]; !strings.HasSuffix(last, "|Its own colours|"+DyePickLabel) {
		t.Fatalf("the colours: %q", got)
	}
	e.Answer(tw.s, tw.p, boxID(got), 1)
	msgs := pickerMsgs(tw.s)
	questMsgs(tw.s)
	if len(msgs) != 1 {
		t.Fatalf("picker messages: %d", len(msgs))
	}
	o, ok := readPickerOpen(msgs[0])
	if !ok || o.target != DyePickAura || o.alpha || len(o.slots) != 1 || o.slots[0] != auraPlaceMainHand2 ||
		o.aura != (dyeAuraPreview{kind: database.AuraEpic, source: 0x88EC092D}) || o.title != "Colour of Virtuous" {
		t.Fatalf("the open: %v %+v", ok, o)
	}
	// an aura's picker has no "No dye": refused, and its picker answered
	answerPicker(t, h, tw.s, DyePickerNoDye, o.token)
	if r, _, _ := db.InstanceAuraOf(sword); r.Second.Source != 0 {
		t.Fatalf("No dye on an aura did something: %+v", r)
	}
	e.Talk(tw.s, tw.p, enchanter)
	got = pick(pick(questMsgs(tw.s), 1), 0)
	e.Answer(tw.s, tw.p, boxID(got), 1)
	msgs = pickerMsgs(tw.s)
	questMsgs(tw.s)
	if len(msgs) == 0 {
		t.Fatal("the picker not opened again")
	}
	o, _ = readPickerOpen(msgs[len(msgs)-1])
	answerPicker(t, h, tw.s, DyePickerPicked, o.token, 0x3FA7D6FF)
	r, _, _ := db.InstanceAuraOf(sword)
	if r.Second.Color != 0x3FA7D6FF || r.Second.Source != 0x88EC092D || r.Tag != "Virtuous" {
		t.Fatalf("the aura: %+v", r)
	}
}
