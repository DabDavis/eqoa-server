package handlers

import (
	"bytes"
	"maps"
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

// An art's required equipment refused, never switched (entity's
// equipRefusal; the owner, 2026-11-15), and the hands' damage (wornGear), on
// made-up items, spells and names.

// switchFixture is a gearFixture whose player stands in a World beside a
// creature, with a book of arts on its bar and the SpellbookHandler casting
// them.
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

// toldWeapon is whether msgs say the weapon refusal (any wording of it).
func toldWeapon(msgs [][]byte) bool {
	for _, m := range msgs {
		if bytes.Contains(m, utf16le("You need ")) && bytes.Contains(m, utf16le(" to use ")) {
			return true
		}
	}
	return false
}

// toldLine is whether msgs say line.
func toldLine(msgs [][]byte, line string) bool {
	for _, m := range msgs {
		if bytes.Contains(m, utf16le(line)) {
			return true
		}
	}
	return false
}

// switched is whether msgs equip or unequip anything (0x003F, 0x0040).
func switched(msgs [][]byte) bool {
	for _, op := range opsOf(msgs) {
		if op == protocol.OpcodeEquipItem || op == protocol.OpcodeUnequipItem {
			return true
		}
	}
	return false
}

// Petition 5161's case, made up: a warrior holding a two-handed sword uses a
// blunt art with a club in its pack. The art is refused with its line naming
// what it takes, in red; nothing is equipped or taken off, no power is spent,
// no lockout set, and the sword is still in hand. Taking the club up by hand
// (the item menu), the same art goes.
func TestAnArtWithTheWrongWeaponIsRefusedNotSwitched(t *testing.T) {
	f := newSwitchFixture(t, ClassWarrior)
	dachi := f.own(f.weapon("Test No Dachi", 15, 2, 300, 0), 16)
	club := f.own(f.weapon("Test Club", 14, 3, 120, 0), -1)
	f.art(1810, "Test Crush", "1HB,2HB")
	f.look()
	power := f.player.Power
	f.sess.Queue().GetPending()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUseSkill)
	msg.SetData(build(protocol.OpcodeUseSkill, 16, func(w *buffer.Writer) {
		w.WriteUint32(0)
		w.WriteByte(f.bar["Test Crush"])
		w.WriteUint32(f.npcO)
	})[2:])
	if err := f.sb.handleUseSkill(f.sess, msg); err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	if began(out) || switched(out) || !toldLine(out, "You need a blunt weapon equipped to use Test Crush.") {
		t.Fatalf("the wrong weapon: %x, want the refusal alone", out)
	}
	if f.player.Power != power || f.player.Casting != nil || len(f.player.Lockouts) != 0 {
		t.Errorf("refused: power %d (was %d), casting %v, lockouts %v: want none spent, none set", f.player.Power, power, f.player.Casting, f.player.Lockouts)
	}
	if w := f.worn(); len(w) != 1 || w[16] != dachi {
		t.Errorf("worn %v after the refusal: want the No Dachi alone, in hand", w)
	}
	if f.player.PrimaryType != 2 {
		t.Errorf("the look after: %d, want the two-hander's 2", f.player.PrimaryType)
	}

	// the club taken up by hand from the item menu: the art goes
	f.loadList()
	f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) { w.WriteUint32(1); w.WriteUint32(0xFFFFFFFF); w.WriteByte(0) })
	if w := f.worn(); len(w) != 1 || w[14] != club {
		t.Fatalf("worn %v: the item menu did not put the club on", w)
	}
	if out := f.cast("Test Crush"); !began(out) || toldWeapon(out) || switched(out) {
		t.Errorf("with the club in hand: %x, want the cast and nothing switched", out)
	}
}

// What hands fit what an art names, on a cast: Any any weapon (or none), a
// two-hander a code of its grip and kind and not the other grip's, an off
// hand's weapon as the main hand's, a shield in the off hand for Shield,
// None bare hands; each miss refused with its words, the hands left as they
// were, no power spent.
func TestAnArtsWeaponCases(t *testing.T) {
	type hand struct {
		name           string
		wear, atk, loc int
	}
	for _, c := range []struct {
		name  string
		class Class
		hands []hand
		req   string
		goes  bool
		line  string
	}{
		{"Any, a sword", ClassWarrior, []hand{{"Test Sword", 14, 1, 14}}, "Any", true, ""},
		{"Any, a bow", ClassRanger, []hand{{"Test Bow", 16, 7, 18}}, "Any", true, ""},
		{"Any, bare hands", ClassMonk, nil, "Any", true, ""},
		{"a two-hander for 1HS,2HS", ClassWarrior, []hand{{"Test Greatsword", 15, 2, 16}}, "1HS,2HS", true, ""},
		{"a two-hander for 1HS alone", ClassWarrior, []hand{{"Test Greatsword", 15, 2, 16}}, "1HS", false,
			"You need a one-handed slashing weapon equipped to use Test Art."},
		{"a one-hander for 2HB alone", ClassWarrior, []hand{{"Test Mace", 14, 3, 14}}, "2HB", false,
			"You need a two-handed blunt weapon equipped to use Test Art."},
		{"an off-hand mace for blunt", ClassWarrior, []hand{{"Test Sword", 14, 1, 14}, {"Test Mace", 14, 3, 15}}, "1HB,2HB", true, ""},
		{"an off-hand dagger for 1HP", ClassRogue, []hand{{"Test Sword", 14, 1, 14}, {"Test Dirk", 14, 5, 15}}, "1HP", true, ""},
		{"dual swords for piercing", ClassRogue, []hand{{"Test Sword", 14, 1, 14}, {"Test Blade", 14, 1, 15}}, "1HP", false,
			"You need a one-handed piercing weapon equipped to use Test Art."},
		{"a shield", ClassPaladin, []hand{{"Test Sword", 14, 1, 14}, {"Test Buckler", 13, 0, 17}}, "Shield", true, ""},
		{"no shield", ClassPaladin, []hand{{"Test Sword", 14, 1, 14}}, "1HB,2HB,Shield", false,
			"You need a blunt weapon or a shield equipped to use Test Art."},
		{"a melee art with a bow", ClassRanger, []hand{{"Test Bow", 16, 7, 18}}, "1HB,1HS,1HP, 2HB,2HS,2HP", false,
			"You need a melee weapon equipped to use Test Art."},
		{"an arrow art with blades", ClassRanger, []hand{{"Test Sword", 14, 1, 14}}, "Range", false,
			"You need a ranged weapon equipped to use Test Art."},
		{"a punch with a weapon", ClassMonk, []hand{{"Test Mace", 14, 3, 14}}, "None", false,
			"You need empty hands to use Test Art."},
		{"a punch bare", ClassMonk, nil, "None", true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newSwitchFixture(t, c.class)
			// what would fit, carried in the pack: never taken up
			f.own(f.weapon("Test Spare Club", 14, 3, 500, 0), -1)
			f.own(f.weapon("Test Spare Bow", 16, 7, 500, 25), -1)
			f.own(f.weapon("Test Spare Dirk", 14, 5, 500, 0), -1)
			for _, h := range c.hands {
				if h.atk == 0 { // a shield: drawn by its model, which says one is held
					id := f.pattern(h.name, h.wear, 0, 0)
					if _, err := f.db.Exec(`UPDATE itempattern SET model = 4242 WHERE patternid = ?`, id); err != nil {
						t.Fatal(err)
					}
					f.own(id, h.loc)
				} else {
					f.own(f.weapon(h.name, h.wear, h.atk, 100, 25), h.loc)
				}
			}
			f.art(1811, "Test Art", c.req)
			f.look()
			before, power := f.worn(), f.player.Power
			out := f.cast("Test Art")
			if c.goes {
				if !began(out) || toldWeapon(out) || switched(out) {
					t.Fatalf("%x: want the cast, nothing switched", out)
				}
				return
			}
			if began(out) || switched(out) || !toldLine(out, c.line) {
				t.Fatalf("%x: want %q alone", out, c.line)
			}
			if f.player.Power != power {
				t.Errorf("power %d, was %d", f.player.Power, power)
			}
			if !maps.Equal(f.worn(), before) {
				t.Errorf("worn %v, was %v", f.worn(), before)
			}
		})
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
