package entity

import (
	"encoding/binary"
	"math"
	"net"
	"slices"
	"testing"
	"time"
)

// Illusions (F3 20261025-173245, jib's). The bodies are charactermodel's
// rows (race, sex, model, as the wire carries a model id) and this server's
// own Clay Golems'.

const (
	gnomeMale   uint32 = 2845600533
	trollMale   uint32 = 1282385202
	trollFemale uint32 = 1306732289
	elfMale     uint32 = 3637866488
	elfFemale   uint32 = 3783858679
	clayGolem   uint32 = 3966371332
)

func raceBodies() map[[2]int]uint32 {
	return map[[2]int]uint32{
		{3, 0}: gnomeMale, {5, 0}: trollMale, {5, 1}: trollFemale, {1, 0}: elfMale, {1, 1}: elfFemale,
	}
}

// illusionRig is newCaster's druid made a gnome enchanter with pink hair of
// style 3, among the races' bodies and three Clay Golems.
func illusionRig(t *testing.T) *caster {
	t.Helper()
	c := newCaster(t)
	c.p.Class, c.p.Race, c.p.Sex, c.p.ModelID, c.p.Size = 12, 3, 0, int64(gnomeMale), 1
	c.p.HairColor, c.p.HairLength, c.p.HairStyle, c.p.FaceOption = 9, 2, 3, 1
	c.w.SetRaceModels(raceBodies())
	for i := 0; i < 3; i++ {
		c.w.NPCs[EntityID(5000+i)] = &NPC{ServerID: EntityID(5000 + i), Name: "Clay Golem", Level: 40, ModelID: int64(clayGolem), Size: 1.25}
	}
	return c
}

func illusionSpell(id int, name, line, desc string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 5, Scope: 1, Name: name, Icon: 0x12345678,
		CastTime: 5 * time.Second, Recast: time.Second, PowerCost: 120, Range: 30, Def: ParseSpellNamed(name, line, desc)}
}

func troll() CastSpell {
	return illusionSpell(565, "Illusion: Troll", "Self, Troll form, Faction Change, 10min", "Illusion only")
}

// channelBody is the model and scale a player's entity channel shows.
func channelBody(p *Player) (uint32, float32) {
	b := SerializePlayerUpdate(1, p)
	return binary.LittleEndian.Uint32(b[28:32]), math.Float32frombits(binary.LittleEndian.Uint32(b[36:40]))
}

// land casts spell at target and lands it.
func (c *caster) land(t *testing.T, spell CastSpell, target uint32, at time.Duration) {
	t.Helper()
	if !c.w.StartCast(c.sess, spell, target, c.at(at)) {
		t.Fatalf("%q did not begin: %x", spell.Name, c.sent())
	}
	c.tick(at + spell.CastTime + castLand)
	c.sent()
}

// What each name says: Illusion and the forms are the caster's, Glamour the
// caster's or a group member's, Remove Illusion and True Form end one; the
// lines' "X form" clauses are no longer unread, "Faction Change" still is.
// The controls: the same lines read without their names give no illusion,
// and a beast's power "without changing your form" is none.
func TestIllusionsAreReadOffTheirNames(t *testing.T) {
	for _, c := range []struct {
		name, line, desc string
		kind             FormKind
		form             string
		dur              time.Duration
	}{
		{"Illusion: Troll", "Self, Troll form, Faction Change, 10min", "Illusion only", FormSelf, "troll", 10 * time.Minute},
		{"Illusion: Dark Elf", "Self, Delf form, Faction Change, 10min", "Illusion only", FormSelf, "dark elf", 10 * time.Minute},
		{"Illusion: Clay Golem", "Self, Golem Form, 10min", "Illusion only", FormSelf, "clay golem", 10 * time.Minute},
		{"Glamour: Elf", "Elf form, Faction Change, Increase DEX by 10, DR by 10, 10min", "CM: Illusion and faction change.", FormGroup, "elf", 10 * time.Minute},
		{"Illusion: Rat", "", "", FormSelf, "rat", illusionDuration},
		{"Mouse Form", "", "Assume the form of a mouse.", FormSelf, "mouse", illusionDuration},
		{"Feral Lion Form", "", "CM: Illusion", FormSelf, "feral lion", illusionDuration},
		{"Feral Lion Form", "", "This spell grants the power of the beast without changing your form.", FormNone, "", 0},
		{"Remove Illusion", "", "This ability removes the current illusion on you.", FormEnd, "", 0},
		{"True Form", "Reverts caster to natural form", "Returns Caster to Natural Self.", FormEnd, "", 0},
	} {
		d := ParseSpellNamed(c.name, c.line, c.desc)
		if d.FormKind != c.kind || d.Form != c.form || d.Duration != c.dur || d.Harmful {
			t.Errorf("%q: kind %d form %q for %v harmful %v, want %d %q for %v", c.name, d.FormKind, d.Form, d.Duration, d.Harmful, c.kind, c.form, c.dur)
		}
		if c.kind == FormNone {
			continue
		}
		for _, u := range d.Unread {
			if u != "faction change" {
				t.Errorf("%q: %q still unread", c.name, u)
			}
		}
		if bare := ParseSpell(c.line, c.desc); bare.FormKind != FormNone || slices.ContainsFunc(bare.Effects, func(e SpellEffect) bool {
			return e.Kind == EffectIllusion || e.Kind == EffectEndIllusion
		}) {
			t.Errorf("the control: %q's line read alone gives an illusion", c.name)
		}
	}
	glamour := ParseSpellNamed("Glamour: Elf", "Elf form, Faction Change, Increase DEX by 10, DR by 10, 10min", "")
	var m StatMods
	for _, e := range glamour.Effects {
		if e.Kind == EffectStat {
			m.add(e.Stat, e.Amount)
		}
	}
	if m.Dex != 10 || m.DR != 10 {
		t.Errorf("Glamour: Elf's stats %+v, want DEX and DR +10", m)
	}
}

// Illusion: Troll with a creature targeted lands on its caster, as a troll's
// body of the caster's sex, the gnome's own hair and face numbers on it, at
// the caster's own scale; Remove Illusion gives the gnome back, and so does
// the end of its ten minutes. The control: the record's scope (a target
// spell) refuses it at a creature, as before ("Target is not friendly.").
func TestIllusionShowsTheCasterAsTheRace(t *testing.T) {
	c := illusionRig(t)
	before := SerializePlayerUpdate(1, c.p)
	c.land(t, troll(), c.npcO, 0)
	if m, s := channelBody(c.p); m != trollMale || s != 1 {
		t.Fatalf("the troll's body: model %d scale %v, want %d at 1", m, s, trollMale)
	}
	after := SerializePlayerUpdate(1, c.p)
	if !slices.Equal(before[105:109], after[105:109]) || before[144] != after[144] || before[110] != after[110] {
		t.Errorf("hair/face %v -> %v, race %d -> %d, sex %d -> %d: the gnome's own must stay", before[105:109], after[105:109], before[144], after[144], before[110], after[110])
	}
	if c.npc.Effects != nil {
		t.Error("the illusion landed on the creature")
	}
	if len(c.p.Buffs) != 1 || c.p.Buffs[0].Name != "Illusion: Troll" {
		t.Errorf("the effects channel: %+v", c.p.Buffs)
	}

	end := illusionSpell(1793, "Remove Illusion", "", "This ability removes the current illusion on you.")
	end.Scope = 0
	c.land(t, end, 0xFFFFFFFF, 20*time.Second)
	if m, _ := channelBody(c.p); m != gnomeMale || len(c.p.Buffs) != 0 {
		t.Fatalf("after Remove Illusion: model %d, buffs %+v; want the gnome's %d and none", m, c.p.Buffs, gnomeMale)
	}

	c.land(t, troll(), 0xFFFFFFFF, 40*time.Second)
	c.tick(40*time.Second + 5*time.Second + castLand + 10*time.Minute)
	if m, _ := channelBody(c.p); m != gnomeMale {
		t.Fatalf("after its ten minutes: model %d, want the gnome's", m)
	}

	ctl := illusionRig(t)
	plain := troll()
	plain.Def = ParseSpell("Self, Troll form, Faction Change, 10min", "Illusion only")
	if ctl.w.StartCast(ctl.sess, plain, ctl.npcO, ctl.at(0)) {
		t.Fatal("the control: read without its name, the illusion went at the creature")
	}
}

// A new illusion takes the old one's place, a creature's body comes at the
// creature's own size, and both are kept over a logout (buffsave.go).
func TestIllusionsReplaceEachOtherAndAreKept(t *testing.T) {
	c := illusionRig(t)
	c.land(t, troll(), 0xFFFFFFFF, 0)
	golem := illusionSpell(575, "Illusion: Clay Golem", "Self, Golem Form, 10min", "Illusion only")
	c.land(t, golem, 0xFFFFFFFF, 20*time.Second)
	if m, s := channelBody(c.p); m != clayGolem || s != 1.25 {
		t.Fatalf("the golem: model %d scale %v, want %d at its 1.25", m, s, clayGolem)
	}
	n := 0
	for _, e := range c.p.Effects {
		if e.Form != 0 {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d illusions on, want the newest only", n)
	}

	rows := savedEffects(c.p, c.at(30*time.Second))
	d := illusionRig(t)
	if d.w.RestoreEffects(d.sess, rows, d.at(0)) != 1 {
		t.Fatalf("restored: %+v", d.p.Effects)
	}
	if m, s := channelBody(d.p); m != clayGolem || s != 1.25 {
		t.Fatalf("after a logout: model %d scale %v, want the golem's", m, s)
	}
}

// Glamour lands on a member of the caster's group, as that member's own
// sex's body with the line's stats; at no one it is the caster's own; at
// anyone outside the group it is refused in red.
func TestGlamourIsTheCastersOrTheGroups(t *testing.T) {
	c := illusionRig(t)
	sess2 := c.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 12), Port: 7000}, 2, 2, 2, 3, true)
	friend := &Player{ServerID: 1002, Name: "Friend", Race: 0, Sex: 1, ModelID: 223572789, Size: 1, HP: 100, MaxHP: 100, X: 104, Y: 10, Z: 100}
	c.w.Players[friend.ServerID] = friend
	sess2.SetECSEntity(friend)
	sess2.SetCharacterInWorld(true)
	sess2.SetPlayerObjectID(0x22220001)
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1] = friend.ServerID
	vs.ChannelObjectID[1] = 0x01312E99
	elf := illusionSpell(503, "Glamour: Elf", "Elf form, Faction Change, Increase DEX by 10, DR by 10, 10min", "CM: Illusion and faction change.")

	// outside the group: refused
	if c.w.StartCast(c.sess, elf, 0x01312E99, c.at(0)) {
		t.Fatal("a glamour at someone outside the group began")
	}
	if msgs := c.sent(); len(msgs) != 1 || chatText(msgs[0]) != castNotInGroup {
		t.Fatalf("the refusal: %x", msgs)
	}

	c.p.GroupID, friend.GroupID = 7, 7
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, friend.ServerID}}}
	c.land(t, elf, 0x01312E99, 10*time.Second)
	if m, _ := channelBody(friend); m != elfFemale || friend.Mods.Dex != 10 {
		t.Fatalf("the friend: model %d, DEX +%d; want the elf woman's %d and +10", m, friend.Mods.Dex, elfFemale)
	}
	if m, _ := channelBody(c.p); m != gnomeMale {
		t.Errorf("the caster became model %d", m)
	}
	c.land(t, elf, 0xFFFFFFFF, 30*time.Second)
	if m, _ := channelBody(c.p); m != elfMale {
		t.Errorf("at no one: the caster is model %d, want the elf man's", m)
	}
}

// The user's ruling of 2026-10-29 (the test lanes' report: True Form ended no
// druid or shaman form): Remove Illusion and True Form end every illusion,
// glamour and form that changes how its bearer looks, and the stats each gave
// with it: a form with no body here, a glamour, a mask, an illusion whose
// body this server lacks. A buff that is no form stays. The marks are kept
// over a logout.
func TestTrueFormEndsEveryLook(t *testing.T) {
	c := illusionRig(t)
	// (a new illusion replaces one with a body, so the glamour comes after the
	// illusion that has none)
	looks := []CastSpell{
		illusionSpell(9301, "Wolf Form", "Self, Worf form, Increase AGI by 9, MaxHP by 90, 30min", ""),
		illusionSpell(9304, "Illusion: Stone Golem", "Self, Form, 10min", "Illusion only"), // no Stone Golem in the rig
		illusionSpell(9302, "Glamour: Elf", "Elf form, Faction Change, Increase DEX by 10, DR by 10, 10min", "CM: Illusion and faction change."),
		illusionSpell(9303, "Grim Mask", "Self, Troll Mask, 5min", ""),
	}
	plain := illusionSpell(9305, "Stoneskin", "Self, Increase AC by 15, 30min", "")
	for i, sp := range append(looks, plain) {
		c.land(t, sp, c.me, time.Duration(i)*10*time.Second)
	}
	if len(c.p.Effects) != 5 || c.p.Mods.Agi != 9 || c.p.Mods.Dex != 10 || c.p.Mods.AC != 15 {
		t.Fatalf("before: %d effects, mods %+v", len(c.p.Effects), c.p.Mods)
	}
	rows, _ := c.w.SavedEffects(c.p.ServerID, c.at(50*time.Second))
	saved := c.p.Effects
	c.p.Effects, c.p.Buffs = nil, nil
	if n := c.w.RestoreEffects(c.sess, rows, c.at(50*time.Second)); n != 5 {
		t.Fatalf("restored %d", n)
	}
	for i, e := range c.p.Effects {
		if e.Appearance != saved[i].Appearance || e.Appearance != (e.SpellID != plain.SpellID) {
			t.Errorf("%q kept over a logout: appearance %v", e.Name, e.Appearance)
		}
	}
	c.land(t, illusionSpell(454, "True Form", "Reverts caster to natural form", "Returns Caster to Natural Self."), c.me, 50*time.Second)
	if len(c.p.Effects) != 1 || c.p.Effects[0].SpellID != plain.SpellID {
		t.Fatalf("after True Form: %+v", c.p.Effects)
	}
	if m := c.p.Mods; m.Agi != 0 || m.MaxHP != 0 || m.Dex != 0 || m.DR != 0 || m.AC != 15 {
		t.Errorf("after True Form the forms' stats remain: %+v", m)
	}
	if model, _ := channelBody(c.p); model != gnomeMale {
		t.Errorf("body %#x, want the gnome's own", model)
	}
}

// What is a look and what is not: the forms', Lich's, a mask's and a
// transformation's words are read; a form that "does not change your form",
// one whose description says so, and a spell that removes one are no looks.
func TestAppearanceIsReadOffTheLine(t *testing.T) {
	for _, c := range []struct {
		name, line, desc string
		look             bool
	}{
		{"Bear Form", "Self, Form, Increase STR by 7, 30min", "", true},
		{"Wolf Form", "Group, Wolf Form, Increase STR by 3, SoW, 15min", "", true},
		{"Lich", "Self, Sprite Change, Increase Power per tic by 10, 30min", "", true},
		{"Grim Mask", "Self, Troll Mask, 5min", "", true},
		{"Otter Shape", "SELF Transform into Otter Form, 15min", "", true},
		{"Illusion: Troll", "Self, Troll form, Faction Change, 10min", "Illusion only", true},
		{"Feral Lion Form", "", "This spell grants the power of the beast without changing your form.", false},
		{"Disguise", "does not change your form, it alters your faction", "", false},
		{"Revert Bat", "Self, Removes Bat form", "", false},
		{"True Form", "Reverts caster to natural form", "Returns Caster to Natural Self.", false},
		{"Stoneskin", "Self, Increase AC by 15, 30min", "", false},
	} {
		d := ParseSpellNamed(c.name, c.line, c.desc)
		if d.Appearance != c.look {
			t.Errorf("%q: appearance %v, want %v", c.name, d.Appearance, c.look)
		}
		if c.look {
			for _, u := range d.Unread {
				if u != "faction change" {
					t.Errorf("%q: %q still unread", c.name, u)
				}
			}
		}
	}
}
