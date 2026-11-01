package handlers

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The switch of hands for an art (handswitch.go) and the hands' damage
// (wornGear), on made-up items, spells and names.

// switchFixture is a gearFixture whose player stands in a World beside a
// creature, with a book of arts on its bar and the SpellbookHandler casting
// them through the EquipHandler's switch.
type switchFixture struct {
	*gearFixture
	w    *entity.World
	npc  *entity.NPC
	npcO uint32
	sb   *SpellbookHandler
	bar  map[string]byte // an art's bar slot by name
}

func newSwitchFixture(t *testing.T, class Class) *switchFixture {
	t.Helper()
	f := &switchFixture{gearFixture: newGearFixture(t), bar: map[string]byte{}}
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 11), Port: 7002}, 1, 1, 1, 2, true)
	f.sess, f.w = sess, w
	p := f.player
	p.Class, p.Level, p.HP, p.MaxHP, p.Power, p.MaxPower = int(class), 20, 400, 400, 500, 500
	p.X, p.Y, p.Z = 100, 10, 100
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x00AA0002)
	f.npc = &entity.NPC{ServerID: 2003, Name: "Mire Toad", Level: 18, HP: 5000, MaxHP: 5000, X: 103, Y: 10, Z: 100}
	w.NPCs[f.npc.ServerID] = f.npc
	f.npcO = 0x0131_4022
	vs := entity.GetOrCreateVisibility(sess)
	vs.ChannelEntity[0] = f.npc.ServerID
	vs.ChannelObjectID[0] = f.npcO
	f.h.SetGameWorld(w)
	f.h.charHandler.gameWorld = w
	f.sb = NewSpellbookHandler(w, database.NewSpellbookStore(f.db))
	f.sb.SetInventory(f.inv)
	f.sb.SetHandSwitch(f.h.SwitchHandsFor)
	return f
}

// weapon adds a weapon: its wear slot, attack type, DMG and range (unk5),
// any class's from level 1.
func (f *switchFixture) weapon(name string, wear, attack, dmg, rng int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 209763529, 0, ?, 0, 0, 1, 0, ?, ?, ?, 1, 1, 0, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`,
		wear, attack, dmg, rng, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// art puts an art on the player's bar: a harmful spell at a target of no
// cast time, reach 30, power 10, that names required equipment.
func (f *switchFixture) art(id int, name, req string) {
	f.t.Helper()
	slot := byte(len(f.bar))
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		  VALUES (?, 1, 0, 0, 16880, 0, 10, 1, 2, 1, 6, -1, ?, '')`, []any{id, name}},
		{`INSERT INTO spelleffect (spellid, line, effects, fxdict, source) VALUES (?, '', ?, 0, 'test')`,
			[]any{id, `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"req_equipment":"` + req + `","effects":[{"kind":"hp","amount":-12}]}`}},
		{`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (?, ?, ?, 1, ?, 0, -1)`,
			[]any{f.sid, id, int(slot) + 1, slot}},
	} {
		if _, err := f.db.Exec(q.sql, q.args...); err != nil {
			f.t.Fatal(err)
		}
	}
	f.bar[name] = slot
}

// look puts the worn hands on the player, as a login does.
func (f *switchFixture) look() {
	f.t.Helper()
	f.loadList()
	f.h.charHandler.applyEquipmentFromDB(f.player, f.sid, false)
}

// cast uses the art from the bar at the creature and returns what was sent,
// each message opcode first; the cast's busy time is let pass after it.
func (f *switchFixture) cast(name string) [][]byte {
	f.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUseSkill)
	msg.SetData(build(protocol.OpcodeUseSkill, 16, func(w *buffer.Writer) {
		w.WriteUint32(0)
		w.WriteByte(f.bar[name])
		w.WriteUint32(f.npcO)
	})[2:])
	if err := f.sb.handleUseSkill(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	f.player.Casting, f.player.Lockouts = nil, nil // its busy time and lockout over
	return out
}

// began is whether a cast began: its lockout's 0x00CD, sent only then (a
// refusal's line or a recovery's answer never sends one).
func began(msgs [][]byte) bool {
	for _, op := range opsOf(msgs) {
		if op == protocol.OpcodeCooldownTimer {
			return true
		}
	}
	return false
}

func toldWeapon(msgs [][]byte) bool {
	for _, m := range msgs {
		if bytes.Contains(m, utf16le("You do not have the right weapon for that.")) {
			return true
		}
	}
	return false
}

// A dual-wielding ranger's arrow art (Range) puts its bow on from the pack,
// as the item menu's equip does (0x003F, the client taking the blades off
// itself), and is cast: never told it lacks the weapon. Its blade art then
// puts the blades it held back, each in its hand, and is cast too. The bow
// it held last is the one an arrow art takes up again, though another of
// more DMG is carried.
func TestARangersArtsSwitchBetweenItsBladesAndItsBow(t *testing.T) {
	f := newSwitchFixture(t, ClassRanger)
	a := f.own(f.weapon("Fen Cutter", 14, 1, 200, 0), 14)
	b := f.own(f.weapon("Reed Blade", 14, 1, 180, 0), 15)
	bow := f.own(f.weapon("Ash Shortbow", 16, 7, 220, 25), -1)
	f.art(1801, "Test Arrow", "Range")
	f.art(1802, "Test Edge", "1HS,2HS")
	f.look()

	out := f.cast("Test Arrow")
	if len(out) == 0 || !bytes.Equal(out[0], []byte{0x3F, 0x00, 2, 18}) || !began(out) || toldWeapon(out) {
		t.Fatalf("the arrow art: %x, want the bow (list index 2) into 18, then the cast", out)
	}
	if w := f.worn(); len(w) != 1 || w[18] != bow {
		t.Fatalf("worn %v after the arrow art: want the bow alone", w)
	}
	if f.player.SecondaryType != 7 || f.player.PrimaryType != 0 {
		t.Errorf("the look after: %d/%d, want the bow's 7 in the second hand", f.player.PrimaryType, f.player.SecondaryType)
	}

	out = f.cast("Test Edge")
	want := [][]byte{{0x3F, 0x00, 0, 14}, {0x3F, 0x00, 1, 15}}
	if len(out) < 2 || !bytes.Equal(out[0], want[0]) || !bytes.Equal(out[1], want[1]) || !began(out) || toldWeapon(out) {
		t.Fatalf("the blade art: %x, want the blades back into 14 and 15, then the cast", out)
	}
	if w := f.worn(); len(w) != 2 || w[14] != a || w[15] != b {
		t.Fatalf("worn %v after the blade art: want the blades in their hands", w)
	}

	// a better bow carried: the arrow art takes up the one it held
	f.own(f.weapon("Yew Greatbow", 16, 7, 400, 25), -1)
	f.loadList()
	if out := f.cast("Test Arrow"); !began(out) {
		t.Fatalf("the arrow art again: %x", out)
	}
	if w := f.worn(); w[18] != bow {
		t.Errorf("worn %v: want the bow it held (%d), not the better one", w, bow)
	}
	// nothing to switch: the bow in hand, the arrow art cast as it is
	if out := f.cast("Test Arrow"); !began(out) || opsOf(out)[0] == protocol.OpcodeEquipItem {
		t.Errorf("an arrow art with the bow in hand: %x", out)
	}
}

// With no ranged weapon carried (a thrown potion is none) the arrow art is
// refused as before, the blades kept; out of its reach it is refused for the
// range alone, nothing switched; out of power nothing is switched either,
// and the refusal is the one StartCast always gave (the weapon's, which its
// order puts before the power's).
func TestARangersArrowArtIsRefusedWithNoSwitch(t *testing.T) {
	f := newSwitchFixture(t, ClassRanger)
	a := f.own(f.weapon("Fen Cutter", 14, 1, 200, 0), 14)
	b := f.own(f.weapon("Reed Blade", 14, 1, 180, 0), 15)
	flask := f.weapon("Fire Flask", 16, 10, 0, 20)
	f.own(flask, -1)
	if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, source) VALUES (?, 'Target, Decrease Hitpoints by 30', 'test')`, flask); err != nil {
		t.Fatal(err)
	}
	f.h.LoadPotions()
	f.art(1801, "Test Arrow", "Range")
	f.look()
	blades := func(when string) {
		if w := f.worn(); len(w) != 2 || w[14] != a || w[15] != b {
			t.Errorf("%s: worn %v, want the blades kept", when, w)
		}
	}
	out := f.cast("Test Arrow")
	if began(out) || !toldWeapon(out) || opsOf(out)[0] == protocol.OpcodeEquipItem {
		t.Errorf("no bow: %x, want the weapon refusal and no switch", out)
	}
	blades("no bow")

	bow := f.own(f.weapon("Ash Shortbow", 16, 7, 220, 25), -1)
	f.loadList()
	f.npc.X = f.player.X + 60 // out of the art's reach (30)
	out = f.cast("Test Arrow")
	if began(out) || toldWeapon(out) || len(out) != 1 || !bytes.Contains(out[0], utf16le("Out of range.")) {
		t.Errorf("out of reach: %x, want the range refusal alone", out)
	}
	blades("out of reach")
	f.npc.X = f.player.X + 3
	f.player.Power = 0
	if out := f.cast("Test Arrow"); began(out) || len(out) != 1 || opsOf(out)[0] == protocol.OpcodeEquipItem {
		t.Errorf("out of power: %x, want a refusal alone (the weapon's, before the power's in the cast's order)", out)
	}
	blades("out of power")
	f.player.Power = 500
	if out := f.cast("Test Arrow"); !began(out) || f.worn()[18] != bow {
		t.Errorf("the control, in reach with power: %x, worn %v", out, f.worn())
	}
}

// A knight never gets a weapon in its off hand (offHandWeaponBarred): a
// paladin's blunt art puts its best mace in the primary hand beside its
// shield, the second mace left in the pack. The control: a warrior's takes
// both maces up, the second in its off hand.
func TestASwitchGivesAKnightNoOffHandWeapon(t *testing.T) {
	for _, c := range []struct {
		class Class
		off   bool
	}{{ClassPaladin, false}, {ClassWarrior, true}} {
		f := newSwitchFixture(t, c.class)
		f.own(f.weapon("Cold Iron Sword", 12, 1, 150, 0), 14)
		shield := f.own(f.pattern("Bark Buckler", 13, 0, 0), 17)
		heavy := f.own(f.weapon("Granite Maul", 14, 3, 210, 0), -1)
		light := f.own(f.weapon("Oak Cudgel", 14, 3, 120, 0), -1)
		f.art(1803, "Test Crush", "1HB,2HB")
		f.look()
		out := f.cast("Test Crush")
		if !began(out) || toldWeapon(out) {
			t.Fatalf("class %d: the blunt art %x", c.class, out)
		}
		w := f.worn()
		if w[14] != heavy {
			t.Errorf("class %d: worn %v, want the better mace in the primary hand", c.class, w)
		}
		if !c.off && (w[15] != 0 || w[17] != shield) {
			t.Errorf("a paladin's hands %v: want no off-hand weapon and its shield kept", w)
		}
		if c.off && w[15] != light {
			t.Errorf("the control: a warrior's hands %v, want the second mace in the off hand", w)
		}
	}
}

// The hands' damage (wornGear): a bow's or a thrown weapon's when no weapon
// is in hand, never a hand weapon's place taken; bare hands with no weapon
// in a hand nor in the ranged slot, a shield and an item held being none.
// The control: the slots read before (14 and 16) give a bow 0.
func TestWornGearTakesTheRangedWeaponsDamage(t *testing.T) {
	item := func(slot, wear, attack, dmg int) database.CharInventoryEntry {
		return database.CharInventoryEntry{EquipLoc: slot, Pattern: database.ItemPatternRecord{EquipSlot: wear, AttackType: attack, WeaponDamage: dmg}}
	}
	bow, knife := item(18, 16, 7, 300), item(18, 17, 10, 120)
	sword, shield, wand := item(14, 14, 1, 250), item(17, 13, 0, 0), item(19, 18, 0, 0)
	for _, c := range []struct {
		name  string
		items []database.CharInventoryEntry
		dmg   int
		slot  int
		bare  bool
	}{
		{"a bow", []database.CharInventoryEntry{bow}, 300, 18, false},
		{"a thrown weapon", []database.CharInventoryEntry{knife}, 120, 18, false},
		{"a sword beside a thrown weapon", []database.CharInventoryEntry{knife, sword}, 250, 14, false},
		{"a sword", []database.CharInventoryEntry{sword}, 250, 14, false},
		{"a shield and a wand", []database.CharInventoryEntry{shield, wand}, 0, 0, true},
		{"a second weapon alone", []database.CharInventoryEntry{item(15, 14, 1, 90)}, 0, 0, false},
		{"bait", []database.CharInventoryEntry{item(15, wearBait, 0, 0)}, 0, 0, true},
		{"nothing", nil, 0, 0, true},
	} {
		g := wornGear(c.items)
		if g.WeaponDamage != c.dmg || g.WeaponSlot != c.slot || g.BareHands != c.bare {
			t.Errorf("%s: DMG %d from slot %d, bare %v; want %d from %d, bare %v", c.name, g.WeaponDamage, g.WeaponSlot, g.BareHands, c.dmg, c.slot, c.bare)
		}
	}
	before := func(items []database.CharInventoryEntry) int {
		d := 0
		for _, it := range items {
			if it.EquipLoc == 14 || it.EquipLoc == 16 {
				d = it.Pattern.WeaponDamage
			}
		}
		return d
	}
	if before([]database.CharInventoryEntry{bow}) != 0 {
		t.Error("the control: the slots read before must give a bow 0")
	}
}

// A change of hands in a fight is the fight's next round (entity's
// HandsChanged): a ranger at a creature with its blades takes up its bow from
// the item menu, and its next round is the bow's ranged swing at the bow's
// reach. The control: out of a fight nothing waits.
func TestTheItemMenusEquipChangesAFightsSwing(t *testing.T) {
	f := newSwitchFixture(t, ClassRanger)
	f.own(f.weapon("Fen Cutter", 14, 1, 200, 0), 14)
	f.own(f.weapon("Ash Shortbow", 16, 7, 220, 25), -1)
	f.look()
	stats := func() string { return strings.Join(f.w.StatsReport(f.sess, 0, time.Now()), "\n") }
	f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(0xFFFFFFFF); w.WriteByte(0) })
	if strings.Contains(stats(), "from the next round") {
		t.Fatal("out of a fight a change of hands waits for a round")
	}
	f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(0xFFFFFFFF); w.WriteByte(0) })
	if !f.w.EngageAttack(f.sess, f.npcO, swingOf(f.w, f.inv, f.sess, f.player), time.Now()) {
		t.Fatal("not engaged")
	}
	if s := stats(); !strings.Contains(s, "Swing: Attack2 (kind 4)") {
		t.Fatalf("the blades' swing: %s", s)
	}
	f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(0xFFFFFFFF); w.WriteByte(0) })
	if s := stats(); !strings.Contains(s, "from the next round Range2 (kind 9) every 5.0 s, reach 25.0") {
		t.Errorf("after the bow went on: %s", s)
	}
}

// A monk's punch (None) takes the weapon off its hands, a 0x0040 for the
// slot (the client takes nothing off for it itself), and is cast: its hands
// bare, so its melee rolls its hand to hand (the gear the World is given).
// Its shield stays: a shield is no weapon.
func TestAMonksPunchTakesItsWeaponOff(t *testing.T) {
	f := newSwitchFixture(t, ClassMonk)
	f.own(f.weapon("Oak Cudgel", 12, 3, 120, 0), 14)
	shield := f.own(f.pattern("Bark Buckler", 13, 0, 0), 17)
	f.art(1804, "Test Punch", "None")
	f.look()
	if f.player.Gear.BareHands {
		t.Fatal("bare-handed with a cudgel in hand")
	}
	out := f.cast("Test Punch")
	if len(out) == 0 || !bytes.Equal(out[0], []byte{0x40, 0x00, 14}) || !began(out) || toldWeapon(out) {
		t.Fatalf("the punch: %x, want the cudgel's slot emptied (0x0040 14), then the cast", out)
	}
	if w := f.worn(); len(w) != 1 || w[17] != shield {
		t.Errorf("worn %v after the punch: want the shield alone", w)
	}
	if !f.player.Gear.BareHands {
		t.Error("the World's gear is not bare-handed after the punch")
	}
}
