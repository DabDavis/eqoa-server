package entity

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// Taps and pacts as the lists word them (EQOA-Data's necromancer, shadow
// knight and paladin lines, verbatim): what each effect does and to whom.
func TestTapAndPactWords(t *testing.T) {
	type eff struct {
		kind   EffectKind
		stat   Stat
		amount int
		self   bool
		drain  DrainTo
		scale  Stat
	}
	cases := []struct {
		line    string
		harmful bool
		dur     time.Duration
		want    []eff
	}{
		{"Target, Decrease Drain HP by 10", true, 0, []eff{{EffectHP, 0, -10, false, DrainCaster, 0}}},
		{"Target, Drain HP by 22 and transfer to caster", true, 0, []eff{{EffectHP, 0, -22, false, DrainCaster, 0}}},
		{"Target, Decrease HP by 76 and transfer to caster", true, 0, []eff{{EffectHP, 0, -76, false, DrainCaster, 0}}},
		{"Target, Drain HP by 500 give self; Increase 20% based on INT", true, 0, []eff{{EffectHP, 0, -500, false, DrainCaster, StatInt}}},
		{"Target, Decrease Hitpoints by 58 ; Increase 20% based on STR, Drain HP by 9 give self; Increase 2% based on INT", true, 0,
			[]eff{{EffectHP, 0, -58, false, DrainNone, StatStr}, {EffectHP, 0, -9, false, DrainCaster, StatInt}}},
		// the pacts: a heal on the target, a price on the caster, cast at a friend
		{"Target, Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self", false, 0,
			[]eff{{EffectHP, 0, 25, false, DrainNone, 0}, {EffectHP, 0, -50, true, DrainNone, 0}}},
		{"Target, Increase Hitpoints by 45, Decrease Hitpoints by 90 Self Only", false, 0,
			[]eff{{EffectHP, 0, 45, false, DrainNone, 0}, {EffectHP, 0, -90, true, DrainNone, 0}}},
		{"Target, Increase Hitpoints by 90, Decrease 270 Hitpoints to Self", false, 0,
			[]eff{{EffectHP, 0, 90, false, DrainNone, 0}, {EffectHP, 0, -270, true, DrainNone, 0}}},
		{"Target, Increase Hitpoints by 180, Decrease Hitpoints 360 to Self", false, 0,
			[]eff{{EffectHP, 0, 180, false, DrainNone, 0}, {EffectHP, 0, -360, true, DrainNone, 0}}},
		{"Target, Increase Caster HP by 1160, Decrease HP by 12000", true, 0,
			[]eff{{EffectHP, 0, 1160, true, DrainNone, 0}, {EffectHP, 0, -12000, false, DrainNone, 0}}},
		{"Decrease HP by 1570 to Self ; Decrease HP by 2354 to target; Increase 15% based on DEX", true, 0,
			[]eff{{EffectHP, 0, -1570, true, DrainNone, 0}, {EffectHP, 0, -2354, false, DrainNone, StatDex}}},
		// what goes on the caster
		{"Target, Decrease AC by 25, Increase AC by 25 Self", true, 0,
			[]eff{{EffectStat, StatAC, -25, false, DrainNone, 0}, {EffectStat, StatAC, 25, true, DrainNone, 0}}},
		// no time given: a line raising stats lasts buffDuration (spellnotes.go), the price too
		{"Target, Increase AC by 300, Decrease AC by 150 to Self", false, buffDuration,
			[]eff{{EffectStat, StatAC, 300, false, DrainNone, 0}, {EffectStat, StatAC, -150, true, DrainNone, 0}}},
		{"Target, Increase AC by 400, Decrease AC to self by 200", false, buffDuration,
			[]eff{{EffectStat, StatAC, 400, false, DrainNone, 0}, {EffectStat, StatAC, -200, true, DrainNone, 0}}},
		{"Target, Increase Hate by 750, Increase AC by 500 for 12 seconds Self Only", true, 12 * time.Second,
			[]eff{{EffectHate, 0, 750, false, DrainNone, 0}, {EffectStat, StatAC, 500, true, DrainNone, 0}}},
		{"Target, Increase Hate by 750, Increase Self AC by 500, 12sec", true, 12 * time.Second,
			[]eff{{EffectHate, 0, 750, false, DrainNone, 0}, {EffectStat, StatAC, 500, true, DrainNone, 0}}},
		{"Target, Increase Hate by 700 ; Increase 10% based on STR, Increase HP to self by 16 ; Increase 5% based on CHA", true, 0,
			[]eff{{EffectHate, 0, 700, false, DrainNone, StatStr}, {EffectHP, 0, 16, true, DrainNone, StatCha}}},
		// power and stats taken
		{"Target, Decrease Hitpoints by 234 ; Increase 20% based on STR, Drain Power by 39 to self", true, 0,
			[]eff{{EffectHP, 0, -234, false, DrainNone, StatStr}, {EffectPower, 0, -39, false, DrainCaster, 0}}},
		{"Target, Decrease HP by 656 ; Increase 20% based on STR, Drain Power give self by 109", true, 0,
			[]eff{{EffectHP, 0, -656, false, DrainNone, StatStr}, {EffectPower, 0, -109, false, DrainCaster, 0}}},
		{"Target, Decrease STA by 15 and gives to self", true, 0, []eff{{EffectStat, StatSta, -15, false, DrainCaster, 0}}},
		{"Target, Decrease STA by 30, give to self", true, 0, []eff{{EffectStat, StatSta, -30, false, DrainCaster, 0}}},
		{"Target, Decrease STA/AGI by 25 give self", true, 0,
			[]eff{{EffectStat, StatSta, -25, false, DrainCaster, 0}, {EffectStat, StatAgi, -25, false, DrainCaster, 0}}},
		{"Target, Drain AC by 175, give Self", true, 0, []eff{{EffectStat, StatAC, -175, false, DrainCaster, 0}}},
		// the group's
		{"Target, Decrease Drain Group HP by 125", true, 0, []eff{{EffectHP, 0, -125, false, DrainGroup, 0}}},
		{"Target, Decrease Drain Group HP by 250, and gives to group", true, 0, []eff{{EffectHP, 0, -250, false, DrainGroup, 0}}},
		{"Group, Increase Group HP by 500", false, 0, []eff{{EffectHP, 0, 500, false, DrainNone, 0}}},
		// the controls: a plain nuke and a plain heal are neither
		{"Target, Decrease Hitpoints by 52 ; Increase 5% based on DEX", true, 0, []eff{{EffectHP, 0, -52, false, DrainNone, StatDex}}},
		{"Target, Increase Hitpoints by 250 ; Increase 25% based on CHA", false, 0, []eff{{EffectHP, 0, 250, false, DrainNone, StatCha}}},
	}
	for _, c := range cases {
		d := ParseSpellText(c.line)
		var got []eff
		for _, e := range d.Effects {
			got = append(got, eff{e.Kind, e.Stat, e.Amount, e.Self, e.Drain, e.Scale})
		}
		if len(got) != len(c.want) || d.Harmful != c.harmful || d.Duration != c.dur || len(d.Unread) != 0 {
			t.Errorf("%q\n got %+v harmful %v %v unread %q\nwant %+v harmful %v %v", c.line, got, d.Harmful, d.Duration, d.Unread, c.want, c.harmful, c.dur)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q effect %d\n got %+v\nwant %+v", c.line, i, got[i], c.want[i])
			}
		}
	}
	// a pact plays a heal's visual, a tap a strike's
	if _, kind := StandInVisual(ParseSpellText("Target, Increase Caster HP by 1160, Decrease HP by 12000")); kind != "damage" {
		t.Errorf("Dessicate plays a %s's visual", kind)
	}
	if _, kind := StandInVisual(ParseSpellText("Target, Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self")); kind != "heal" {
		t.Errorf("a pact plays a %s's visual", kind)
	}
}

// damageIndications are the 0x00DB in msgs: source, amount, target.
func damageIndications(msgs [][]byte) [][3]int64 {
	var out [][3]int64
	for _, m := range msgs {
		if opOfMsg(m) != protocol.OpcodeDamageIndication {
			continue
		}
		src := binary.LittleEndian.Uint32(m[2:])
		u, n := binary.Uvarint(m[6:])
		v := int64(u>>1) ^ -int64(u&1)
		dst := binary.LittleEndian.Uint32(m[6+n:])
		out = append(out, [3]int64{int64(src), v, int64(dst)})
	}
	return out
}

func spiritTap() CastSpell {
	return CastSpell{SpellID: 937, DisplayIndex: 5, Level: 34, Scope: 1, Name: "Spirit Tap", Icon: 0x7966F926,
		CastTime: 2 * time.Second, Recast: 3 * time.Second, PowerCost: 177, Range: 30,
		Def: ParseSpellText("Target, Decrease Drain HP by 156")}
}

func (c *caster) landSeen(t *testing.T, spell CastSpell, target uint32) [][]byte {
	t.Helper()
	if !c.w.StartCast(c.sess, spell, target, c.at(0)) {
		t.Fatalf("%q did not begin: %x", spell.Name, c.sent())
	}
	c.sent()
	c.tick(spell.CastTime + castLand)
	return c.sent()
}

// A life tap's damage heals its caster by its figure: the creature's 0x00DB
// and the caster's own heal; one that kills heals as much. The control: a
// plain nuke of the same figure heals nothing.
func TestALifeTapHealsItsCaster(t *testing.T) {
	c := newCaster(t)
	c.p.HP = 300
	c.npc.HP, c.npc.MaxHP, c.npc.Level = 1000, 1000, 30
	figures := damageIndications(c.landSeen(t, spiritTap(), c.npcO))
	if c.npc.HP != 1000-156 || c.p.HP != 300+156 {
		t.Fatalf("the tap: creature %d HP, caster %d HP; want %d and %d", c.npc.HP, c.p.HP, 1000-156, 300+156)
	}
	want := [][3]int64{{int64(c.me), -156, int64(c.npcO)}, {int64(c.me), 156, int64(c.me)}}
	if len(figures) != 2 || figures[0] != want[0] || figures[1] != want[1] {
		t.Errorf("the figures %v, want %v", figures, want)
	}

	k := newCaster(t)
	k.p.HP = 100
	k.npc.HP, k.npc.MaxHP = 30, 30
	k.landSeen(t, spiritTap(), k.npcO)
	if !k.npc.Dead || k.p.HP != 256 {
		t.Errorf("a tap that kills: creature dead %v, caster %d HP, want 256", k.npc.Dead, k.p.HP)
	}

	n := newCaster(t)
	n.p.HP = 300
	n.npc.HP, n.npc.MaxHP = 1000, 1000
	nuke := spiritTap()
	nuke.Def = ParseSpellText("Target, Decrease Hitpoints by 156")
	n.landSeen(t, nuke, n.npcO)
	if n.npc.HP != 844 || n.p.HP != 300 {
		t.Errorf("the control nuke: creature %d, caster %d HP", n.npc.HP, n.p.HP)
	}
}

// A shadow knight's strike with a tap in it: both hits on the creature, the
// tap's share (with its INT) back to the caster; a group's tap heals each of
// the group in range, and not one out of it.
func TestTapsInAStrikeAndForAGroup(t *testing.T) {
	c := newCaster(t)
	c.p.HP, c.p.Int = 100, 100
	c.npc.HP, c.npc.MaxHP = 1000, 1000
	strike := spiritTap()
	strike.Def = ParseSpellText("Target, Decrease Hitpoints by 58, Drain HP by 9 give self; Increase 2% based on INT")
	c.landSeen(t, strike, c.npcO)
	if c.npc.HP != 1000-58-11 || c.p.HP != 100+11 {
		t.Fatalf("the strike: creature %d HP, caster %d; want %d and %d", c.npc.HP, c.p.HP, 1000-58-11, 111)
	}

	g := newCaster(t)
	g.p.HP = 100
	g.npc.HP, g.npc.MaxHP = 1000, 1000
	near := &Player{ServerID: 1002, Name: "Near", HP: 50, MaxHP: 500, X: 110, Y: 10, Z: 100}
	far := &Player{ServerID: 1003, Name: "Far", HP: 50, MaxHP: 500, X: 900, Y: 10, Z: 100}
	g.w.Players[near.ServerID], g.w.Players[far.ServerID] = near, far
	g.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{g.p.ServerID, near.ServerID, far.ServerID}}}
	g.p.GroupID, near.GroupID, far.GroupID = 7, 7, 7
	gale := spiritTap()
	gale.Name, gale.Def = "Blood Gale", ParseSpellText("Target, Decrease Drain Group HP by 125")
	g.landSeen(t, gale, g.npcO)
	if g.p.HP != 225 || near.HP != 175 || far.HP != 50 {
		t.Errorf("the group's tap: caster %d, near %d, far %d; want 225, 175, 50", g.p.HP, near.HP, far.HP)
	}
}

// pactRig is the caster with a friend beside it, seen.
func pactRig(t *testing.T) (*caster, *Player, uint32) {
	c := newCaster(t)
	c.p.Class = classNecromancer
	friend := &Player{ServerID: 1004, Name: "Friend", HP: 100, MaxHP: 500, X: 104, Y: 10, Z: 100}
	c.w.Players[friend.ServerID] = friend
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1] = friend.ServerID
	vs.ChannelObjectID[1] = 0x0F00F001
	return c, friend, 0x0F00F001
}

func gloomPact() CastSpell {
	return CastSpell{SpellID: 882, DisplayIndex: 6, Level: 4, Scope: 1, Name: "Gloom Pact", Icon: 0x7966F926,
		CastTime: 2 * time.Second, Recast: 19 * time.Second, PowerCost: 20, Range: 30,
		Def: ParseSpellText("Target, Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self")}
}

// A pact heals a friend and costs its caster twice that, less its arcane
// resist's share (resist.go): at none the whole 50, at 210 half; the line
// says so.
func TestAPactCostsItsCaster(t *testing.T) {
	c, friend, fo := pactRig(t)
	msgs := c.landSeen(t, gloomPact(), fo)
	if friend.HP != 125 || c.p.HP != 450 {
		t.Fatalf("the pact: friend %d HP, caster %d; want 125 and 450", friend.HP, c.p.HP)
	}
	var said bool
	for _, m := range msgs {
		if strings.Contains(chatText(m), "Gloom Pact costs you 50 hit points (your resist took 0%)") {
			said = true
		}
	}
	figs := damageIndications(msgs)
	if !said || len(figs) != 2 || figs[0] != [3]int64{int64(c.me), -50, int64(c.me)} || figs[1] != [3]int64{int64(c.me), 25, int64(fo)} {
		t.Errorf("said %v, figures %v", said, figs)
	}

	r, friend2, fo2 := pactRig(t)
	r.p.ArcaneR = 210
	r.landSeen(t, gloomPact(), fo2)
	if friend2.HP != 125 || r.p.HP != 475 {
		t.Errorf("at arcane resist 210: friend %d, caster %d; want 125 and 475", friend2.HP, r.p.HP)
	}
}

// At the resist cap a pact's price is a quarter (13 of 50), but the dice may
// fail the resist: wholly (the whole 50, "your resist failed") or in part (a
// share of 45% at the bottom of its range: 28 of 50, "took only 45%").
func TestAPactAtTheResistCap(t *testing.T) {
	defer func(f func() float64) { resistRand = f }(resistRand)
	for _, tt := range []struct {
		name  string
		dice  []float64
		hp    int
		words string
	}{
		{"held", []float64{0.5}, 487, "your resist took 75%"},
		{"failed", []float64{0.01}, 450, "your resist failed"},
		{"failed in part", []float64{0.05, 0}, 472, "your resist took only 45%"},
	} {
		dice := append([]float64(nil), tt.dice...)
		resistRand = func() float64 { d := dice[0]; dice = dice[1:]; return d }
		c, _, fo := pactRig(t)
		c.p.ArcaneR = 400
		msgs := c.landSeen(t, gloomPact(), fo)
		var said bool
		for _, m := range msgs {
			if strings.Contains(chatText(m), "Gloom Pact costs you") && strings.Contains(chatText(m), tt.words) {
				said = true
			}
		}
		if c.p.HP != tt.hp || !said || len(dice) != 0 {
			t.Errorf("%s: caster %d HP (want %d), said %q %v, dice left %d", tt.name, c.p.HP, tt.hp, tt.words, said, len(dice))
		}
	}
}

// On oneself a pact costs more than it heals unresisted: the price first,
// then the heal; and a price the caster cannot pay kills it, though the
// friend is healed.
func TestAPactOnOneselfAndAPactThatKills(t *testing.T) {
	c, _, _ := pactRig(t)
	c.p.HP = c.p.MaxHP
	c.landSeen(t, gloomPact(), c.me)
	if c.p.HP != c.p.MaxHP-25 {
		t.Errorf("a pact on oneself at full health: %d HP, want %d", c.p.HP, c.p.MaxHP-25)
	}

	k, friend, fo := pactRig(t)
	k.p.HP = 30
	k.landSeen(t, gloomPact(), fo)
	if !k.p.respawnPending() || k.p.HP != 0 || friend.HP != 125 {
		t.Errorf("a pact past the caster's health: dead %v (%d HP), friend %d", k.p.respawnPending(), k.p.HP, friend.HP)
	}
}

// A stat taken is the caster's for the line's duration, one effect of the
// spell's name for both stats of one line, gone at its end.
func TestAStatTakenIsTheCasters(t *testing.T) {
	c := newCaster(t)
	c.npc.HP, c.npc.MaxHP = 1000, 1000
	tap := spiritTap()
	tap.SpellID, tap.Name = 1449, "Shriek of Hate"
	tap.Def = ParseSpellText("Target, Decrease STA/AGI by 25 give self, 60sec")
	str, agi := c.p.StatNow(StatSta), c.p.StatNow(StatAgi)
	c.landSeen(t, tap, c.npcO)
	var mine []*ActiveEffect
	for _, e := range c.p.Effects {
		if e.SpellID == -1449 {
			mine = append(mine, e)
		}
	}
	if len(mine) != 1 || c.p.StatNow(StatSta) != str+25 || c.p.StatNow(StatAgi) != agi+25 || mine[0].Name != "Shriek of Hate" {
		t.Fatalf("the caster's share: %d effects, STA %d -> %d, AGI %d -> %d", len(mine), str, c.p.StatNow(StatSta), agi, c.p.StatNow(StatAgi))
	}
	c.tick(2*time.Second + castLand + 61*time.Second)
	if c.p.StatNow(StatSta) != str || len(c.p.Effects) != 0 {
		t.Errorf("after the minute: STA %d, %d effects", c.p.StatNow(StatSta), len(c.p.Effects))
	}
}

// A necromancer's pet of level 20 and up casts the highest tap at or below
// its level whenever it is up, every 30 s while it fights in reach: the
// figure on the crab, as much back on the pet. The controls: a magician's
// pet, a necromancer's pet below 20, and a charmed creature never tap.
func TestANecromancersPetTaps(t *testing.T) {
	taps := []PetTap{
		{Level: 16, Name: "Siphon Life", Hit: SpellEffect{Kind: EffectHP, Amount: -29, AmountMax: -29, Drain: DrainCaster}, FX: 0x0A0A0A0A},
		{Level: 34, Name: "Spirit Tap", Hit: SpellEffect{Kind: EffectHP, Amount: -156, AmountMax: -156, Drain: DrainCaster}, FX: 0x0B0B0B0B},
		{Level: 55, Name: "Life's Bane", Hit: SpellEffect{Kind: EffectHP, Amount: -275, AmountMax: -275, Drain: DrainCaster}, FX: 0x0C0C0C0C},
	}
	tapFX := map[uint32]bool{0x0A0A0A0A: true, 0x0B0B0B0B: true, 0x0C0C0C0C: true}
	fight := func(t *testing.T, class, spellLevel int, charm bool) (casts []uint32, heals int) {
		r := newPetRig(t)
		r.p.Class = class
		r.w.petTaps = taps
		r.enemy.HP, r.enemy.MaxHP, r.enemy.Level = 100000, 100000, 1
		spell := fireElemental()
		spell.Level = spellLevel
		pet, _ := r.summon(t, spell, 0)
		if charm {
			pet.Charm = &charmHold{}
		}
		pet.HP = pet.MaxHP / 2
		r.w.PetCommand(r.sess, petCmdAttack, 0x0133E8A0, r.at(9600*time.Millisecond))
		r.sent()
		petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
		for ms := 9700; ms <= 9700+65000; ms += 100 {
			r.tick(time.Duration(ms) * time.Millisecond)
			msgs := r.sent()
			for _, m := range msgs {
				if opOfMsg(m) == protocol.OpcodeCastingCombat && binary.LittleEndian.Uint32(m[2:]) == petID {
					if fx := binary.LittleEndian.Uint32(m[15:]); tapFX[fx] {
						casts = append(casts, fx)
					}
				}
			}
			for _, f := range damageIndications(msgs) {
				if f[1] > 0 && f[2] == int64(petID) {
					heals++
				}
			}
		}
		return casts, heals
	}
	casts, heals := fight(t, classNecromancer, 44, false)
	if len(casts) != 3 || casts[0] != 0x0B0B0B0B || heals != 3 {
		t.Fatalf("a level 44 necromancer's pet over 65 s: taps %x and %d heals, want three of Spirit Tap's", casts, heals)
	}
	if casts, _ := fight(t, classNecromancer, 20, false); len(casts) != 3 || casts[0] != 0x0A0A0A0A {
		t.Errorf("a level 20 pet: taps %x, want Siphon Life's, three", casts)
	}
	for _, c := range []struct {
		name         string
		class, level int
		charm        bool
	}{{"a magician's pet", 10, 44, false}, {"a level 16 pet", classNecromancer, 16, false}, {"a charmed creature", classNecromancer, 44, true}} {
		if casts, _ := fight(t, c.class, c.level, c.charm); len(casts) != 0 {
			t.Errorf("%s tapped: %x", c.name, casts)
		}
	}
}

// petTapLines is a few necromancer's spells as ClassSpellLines gives them.
func petTapLines() []database.ClassSpellLine {
	return []database.ClassSpellLine{
		{SpellID: 875, Level: 1, Scope: 1, Name: "Disease Cloud", Line: "Target, Decrease HP per tic by 4, 24s"},
		{SpellID: 880, Level: 3, Scope: 1, Name: "Life Tap", Line: "Target, Decrease Drain HP by 10", FX: 0x11111111},
		{SpellID: 882, Level: 4, Scope: 1, Name: "Gloom Pact", Line: "Target, Increase Hitpoints by 25, Decrease Hitpoints by 50 to Self"},
		{SpellID: 890, Level: 8, Scope: 1, Name: "Life Draw", Line: "Target, Drain HP by 22 and transfer to caster"},
		{SpellID: 891, Level: 8, Scope: 1, Name: "Life Draw", Line: "Target, Drain HP by 22 and transfer to caster"},
		{SpellID: 905, Level: 20, Scope: 1, Name: "Blood Gale", Line: "Target, Decrease Drain Group HP by 125"},
		{SpellID: 914, Level: 24, Scope: 0, Name: "Decaying Flesh", Line: "This spell summons pet"},
		{SpellID: 937, Level: 34, Scope: 1, Name: "Spirit Tap", Line: "Target, Decrease Drain HP by 156"},
		{SpellID: 943, Level: 39, Scope: 1, Name: "Shock of Spores", Line: "Target, Decrease Hitpoints by 386 ; Increase 15% based on DEX"},
	}
}

// petTapsOf reads the taps out of a class's spells: single-target lines
// taking hit points for their caster, by level; not a nuke, a pact, a
// group's tap or a summoning.
func TestPetTapsOfTheNecromancersSpells(t *testing.T) {
	lines := petTapLines()
	got := petTapsOf(lines)
	var names []string
	for _, tp := range got {
		names = append(names, tp.Name)
	}
	if strings.Join(names, ",") != "Life Tap,Life Draw,Spirit Tap" {
		t.Errorf("the taps: %v", names)
	}
	if got[0].FX != 0x11111111 || got[1].FX != fxDamage {
		t.Errorf("visuals %#x %#x: a tap's own, else a strike's stand-in", got[0].FX, got[1].FX)
	}
}

// A life tap heals by what landed: on a creature five levels up that takes
// half of what its caster deals (balance.go's con), the tap heals half. The
// control: with no balance it heals the whole figure.
func TestALifeTapHealsWhatLanded(t *testing.T) {
	for _, c := range []struct {
		b    *Balance
		heal int
	}{{testBalance(), 78}, {nil, 156}} {
		k := newCaster(t)
		k.w.balance = c.b
		k.p.Level, k.p.HP = 25, 300
		k.npc.HP, k.npc.MaxHP, k.npc.Level = 1000, 1000, 30
		k.landSeen(t, spiritTap(), k.npcO)
		if k.p.HP != 300+c.heal || k.npc.HP != 1000-c.heal {
			t.Errorf("balance %v: caster %d HP, creature %d; want %d and %d", c.b != nil, k.p.HP, k.npc.HP, 300+c.heal, 1000-c.heal)
		}
	}
}

// A power drain is a hostile act (the owner's ruling of 2026-10-29): a line
// that drains or lowers its target's power is harmful, the take routed as a
// tap's; a line that only raises power, a heal's "Decrease Power by 1" and a
// Self line's lost power stay helps. The spells' figures are made up.
func TestAPowerDrainIsHarmful(t *testing.T) {
	for _, c := range []struct {
		line    string
		harmful bool
		amount  int
		drain   DrainTo
		dur     time.Duration
	}{
		{"Target, Drain Power by 1000 to group", true, -1000, DrainGroup, 0},
		{"Target, Drain Power by 400 and give to group", true, -400, DrainGroup, 0},
		{"Target, Decrease Power by 200 and transfer to caster", true, -200, DrainCaster, 0},
		{"Target, Drain Power by 75", true, -75, DrainCaster, 0},
		{"Target, Decrease Power by 150", true, -150, DrainNone, 0},
		{"Target, Drain Power by 250 to self, 30sec", true, -250, DrainCaster, 30 * time.Second},
		// the controls
		{"Target, Increase Power by 300", false, 300, DrainNone, 0},
		{"Group, Increase Group Power by 300", false, 300, DrainNone, 0},
		{"Self, Decrease Power by 100, Increase AC by 50, 1min", false, -100, DrainNone, time.Minute},
	} {
		d := ParseSpellText(c.line)
		var pw *SpellEffect
		for i := range d.Effects {
			if d.Effects[i].Kind == EffectPower {
				pw = &d.Effects[i]
			}
		}
		if pw == nil || d.Harmful != c.harmful || pw.Amount != c.amount || pw.Drain != c.drain || d.Duration != c.dur || len(d.Unread) != 0 {
			t.Errorf("%q: harmful %v, power %+v, %v, unread %q; want harmful %v, %d to %d", c.line, d.Harmful, pw, d.Duration, d.Unread,
				c.harmful, c.amount, c.drain)
		}
	}
	if d := minorHeal().Def; d.Harmful {
		t.Errorf("a heal's Decrease Power by 1 made it harmful: %+v", d)
	}
}

// powerSap is a made-up power drain cast at a target.
func powerSap(line string) CastSpell {
	return CastSpell{SpellID: 9601, DisplayIndex: 7, Level: 30, Scope: 1, Name: "Made-up Sap", Icon: 0x3C3D3E3F,
		CastTime: 2 * time.Second, Recast: 3 * time.Second, PowerCost: 20, Range: 30, Def: ParseSpellText(line)}
}

// A power drain is cast at a creature and lands: the caster given the whole
// figure at once though the creature has no power, the creature's hate for
// the caster that figure and the 1 any landing gives (root.go), so its fight
// begins with no hit. It is refused at a friend, and a raise of power is
// still refused at a creature.
func TestAPowerDrainLandsAtACreature(t *testing.T) {
	c := newCaster(t)
	c.p.Power = 100
	c.landSeen(t, powerSap("Target, Decrease Power by 200 and transfer to caster"), c.npcO)
	if c.p.Power != 100-20+200 || c.npc.HP != 30 {
		t.Errorf("the drain: caster power %d, want %d; creature %d HP", c.p.Power, 100-20+200, c.npc.HP)
	}
	if c.npc.Threats[c.p.ServerID] != 200+1 || c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("the creature's hate %d, enemy %d: want 201 and the caster", c.npc.Threats[c.p.ServerID], c.npc.CurrentEnemy)
	}

	f, _, fo := pactRig(t)
	if f.w.StartCast(f.sess, powerSap("Target, Drain Power by 200 to self"), fo, f.at(0)) || !hasLine(linesOf(f.sent()), castFriendly) {
		t.Error("a power drain at a friend was not refused as friendly")
	}
	r := newCaster(t)
	if r.w.StartCast(r.sess, powerSap("Target, Increase Power by 300"), r.npcO, r.at(0)) || !hasLine(linesOf(r.sent()), castNotFriendly) {
		t.Error("a raise of power at a creature was not refused as not friendly")
	}
}

// "to group": the caster and each of its group in range given the figure at
// once, one out of range not.
func TestAGroupPowerDrain(t *testing.T) {
	g := newCaster(t)
	g.p.Power = 100
	near := &Player{ServerID: 1002, Name: "Near", HP: 50, MaxHP: 500, Power: 50, MaxPower: 500, X: 110, Y: 10, Z: 100}
	far := &Player{ServerID: 1003, Name: "Far", HP: 50, MaxHP: 500, Power: 50, MaxPower: 500, X: 900, Y: 10, Z: 100}
	g.w.Players[near.ServerID], g.w.Players[far.ServerID] = near, far
	g.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{g.p.ServerID, near.ServerID, far.ServerID}}}
	g.p.GroupID, near.GroupID, far.GroupID = 7, 7, 7
	g.landSeen(t, powerSap("Target, Drain Power by 300 to group"), g.npcO)
	if g.p.Power != 100-20+300 || near.Power != 350 || far.Power != 50 {
		t.Errorf("the group's drain: caster %d, near %d, far %d; want %d, 350, 50", g.p.Power, near.Power, far.Power, 100-20+300)
	}
}

// A drain line with a duration gives its figure once, at the landing: no
// power over time on the caster after it.
func TestAPowerDrainWithATimeIsInstant(t *testing.T) {
	c := newCaster(t)
	c.p.Power = 100
	c.landSeen(t, powerSap("Target, Drain Power by 250 to self, 30sec"), c.npcO)
	if c.p.Power != 100-20+250 {
		t.Fatalf("at the landing: power %d, want %d", c.p.Power, 100-20+250)
	}
	for s := 4; s <= 40; s++ {
		c.tick(time.Duration(s) * time.Second)
	}
	if c.p.Power != 100-20+250 {
		t.Errorf("over its 30 s: power %d, want %d still", c.p.Power, 100-20+250)
	}
	for _, e := range c.p.Effects {
		if e.PowerTick != 0 {
			t.Errorf("a power over time on the caster: %+v", e)
		}
	}
}
