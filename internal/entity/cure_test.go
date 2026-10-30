package entity

import (
	"testing"
	"time"
)

// Cures (cure.go): their words, an effect's kind, and what a cure takes off a
// player, a pet and a potion's drinker. The spells' names, levels and figures
// are made up; the words are the lists'.

// cureSpell is a made-up cure of that level and line, cast at a target.
func cureSpell(id int, name string, level, scope int, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 7, Level: level, Scope: scope, Name: name, Icon: 0x6A6B6C6D,
		CastTime: time.Second, Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// afflicted is a harmful effect on its bearer (a loss of 5 each tick) of a
// spell of that level and kind, lasting an hour from t.
func afflicted(id int, name string, level int, kind CureKind, t time.Time) *ActiveEffect {
	return &ActiveEffect{SpellID: id, Name: name, Until: t.Add(time.Hour), NextTick: t.Add(time.Hour), HPTick: -5,
		Level: level, Afflicts: kind}
}

// effectIDs are the spells of a bearer's effects, in order.
func effectIDs(effects []*ActiveEffect) []int {
	var ids []int
	for _, ae := range effects {
		ids = append(ids, ae.SpellID)
	}
	return ids
}

func sameIDs(a []int, b ...int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every way the lines say a cure is read as one, what it takes off and its
// count (0: by its level), nothing harmful, nothing unread; the controls are
// no cures.
func TestTheCureWordsAreRead(t *testing.T) {
	for _, c := range []struct {
		line   string
		target SpellTarget
		kind   CureKind
		count  int
	}{
		{"Target, Removes Minor Infection", TargetSingle, CureDisease, 0},
		{"This spell removes Minor Poison", TargetSingle, CurePoison, 0},
		{"Target, Removes Lesser Poison", TargetSingle, CurePoison, 0},
		{"Target, removes Greater Infection", TargetSingle, CureDisease, 0},
		{"Target, remove Major Poison", TargetSingle, CurePoison, 0},
		{"Target, removes poison", TargetSingle, CurePoison, 0},
		{"Removes poison and disease", TargetSingle, CurePoison | CureDisease, 0},
		{"Self, Removes poison and disease", TargetSelf, CurePoison | CureDisease, 0},
		{"Self, Cures Posions", TargetSelf, CurePoison, 0},
		{"Dispel Effect x3", TargetSingle, CureAny, 3},
		{"Self, Dispel Effect", TargetSelf, CureAny, 0},
		// a GM's
		{"Target, Cure Disease x2", TargetSingle, CureDisease, 2},
	} {
		d := ParseSpellNamed("Made-up Cure", c.line, "")
		if len(d.Effects) != 1 || d.Effects[0].Kind != EffectCure || d.Effects[0].Cure != c.kind || d.Effects[0].Amount != c.count ||
			d.Harmful || d.Target != c.target || len(d.Unread) != 0 {
			t.Errorf("%q: %+v", c.line, d)
		}
	}
	// a heal's cure beside its heal
	d := ParseSpellNamed("Made-up Mending", "Self, Increase HP per tic by 25, Dispel Effect, Thorn Ward, 24sec", "")
	if len(d.Effects) != 2 || d.Effects[0].Kind != EffectHPTick || d.Effects[1].Kind != EffectCure || d.Effects[1].Cure != CureAny ||
		d.Harmful || len(d.Unread) != 1 || d.Unread[0] != "thorn ward" {
		t.Errorf("a heal with a cure: %+v", d)
	}
	for _, line := range []string{
		"Self, Dispel Mask Effects; Grim, Fey", "Self, Removes Lich form", "Target, Increase Resist Poison by 25, 30min",
		"Target, Decrease HP per tic by 4, 24s",
	} {
		for _, e := range ParseSpellNamed("Made-up Spell", line, "").Effects {
			if e.Kind == EffectCure {
				t.Errorf("%q reads as a cure", line)
			}
		}
	}
}

// An effect's kind is its spell's name's and line's words; a resist named in
// the line counts for nothing; a line saying no cure takes it off is read.
func TestAnEffectsKind(t *testing.T) {
	for _, c := range []struct {
		name, line string
		want       CureKind
	}{
		{"Venom Spit", "Target, Decrease HP per tic by 9, 24s", CurePoison},
		{"Envenomed Thorn", "Target, Decrease HP per tic by 9, 24s", CurePoison},
		{"Bog Fever", "Target, Decrease HP per tic by 9, 24s", CureDisease},
		{"Plaguewind", "Target, Decrease HP per tic by 9, 24s", CureDisease},
		{"Toxic Rot", "Target, Decrease HP per tic by 9, 24s", CurePoison | CureDisease},
		{"Thorn Lash", "Target, Decrease HP per tic by 9 and Debuff Resist Poison by 5, 24s", 0},
		{"Thorn Lash", "Target, Decrease HP per tic by 9, 24s", 0},
	} {
		if d := ParseSpellNamed(c.name, c.line, ""); d.Afflicts != c.want {
			t.Errorf("%s %q: kind %d, want %d", c.name, c.line, d.Afflicts, c.want)
		}
	}
	d := ParseSpellNamed("Hex of Ages", "Target, Decrease HP per tic by 9, cannot be cured, 24s", "")
	if !d.NoCure || !d.Harmful || len(d.Unread) != 0 {
		t.Errorf("a debuff no cure takes: %+v", d)
	}
	if d := ParseSpellNamed("Venom Spit", "Target, Decrease HP per tic by 9, 24s", ""); d.NoCure {
		t.Error("a plain poison read as uncurable")
	}
}

// A cure lands on a player and takes off harmful effects of its kind whose
// spell's level is at most its level + CureLevelMargin, one at a time below
// CureTopLevel, the oldest first; never a help, another kind, or a debuff no
// cure takes. Its effect's level and kind are kept over a logout.
func TestACureTakesItsKindAtOrBelowItsLevel(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	help := &ActiveEffect{SpellID: 9800, Name: "Venom Ward", Until: now.Add(time.Hour), Mods: StatMods{PR: 20}, Level: 10, Afflicts: CurePoison}
	c.p.Effects = []*ActiveEffect{
		help,
		afflicted(9801, "Spit", 20, CurePoison, now),
		afflicted(9802, "Fever", 10, CureDisease, now),
		afflicted(9803, "Sting", 29, CurePoison, now),
		afflicted(9804, "Gash", 5, 0, now),
		afflicted(9805, "Bite", 30, CurePoison, now),
	}
	antidote := cureSpell(9810, "Made-up Antidote", 24, 1, "Target, Removes Poison")
	c.land(t, antidote, c.me, 0)
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9800, 9802, 9803, 9804, 9805) {
		t.Fatalf("the first cure (L24): %v, want the L20 poison off", ids)
	}
	c.land(t, antidote, c.me, 10*time.Second)
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9800, 9802, 9804, 9805) {
		t.Fatalf("the second: %v, want the L29 poison off (24 + %d)", ids, CureLevelMargin)
	}
	c.land(t, antidote, c.me, 20*time.Second)
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9800, 9802, 9804, 9805) {
		t.Fatalf("the third: %v, want the L30 poison left", ids)
	}

	// a top rank takes two; xN takes N
	c.p.Effects = []*ActiveEffect{afflicted(9821, "a", 40, CurePoison, now), afflicted(9822, "b", 49, CurePoison, now),
		afflicted(9823, "c", 45, CurePoison, now)}
	c.w.mu.Lock()
	c.w.curePlayerLocked(c.p, cureOf(ParseSpellText("Target, remove Major Poison").Effects[0], CureTopLevel), "Made-up Major Antidote")
	c.w.mu.Unlock()
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9823) {
		t.Errorf("a level %d cure: %v, want two off", CureTopLevel, ids)
	}
	c.p.Effects = []*ActiveEffect{afflicted(9831, "a", 1, CurePoison, now), afflicted(9832, "b", 2, CurePoison, now),
		afflicted(9833, "c", 3, CurePoison, now), afflicted(9834, "d", 4, CurePoison, now)}
	c.w.mu.Lock()
	c.w.curePlayerLocked(c.p, cureOf(ParseSpellText("Target, Removes Poison x3").Effects[0], 10), "x3")
	c.w.mu.Unlock()
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9834) {
		t.Errorf("x3: %v, want three off", ids)
	}

	// Dispel Effect takes any harmful effect, of a kind or none, within its
	// level; never one no cure takes
	c.p.Effects = []*ActiveEffect{help, afflicted(9841, "Gash", 5, 0, now), afflicted(9842, "Fever", 20, CureDisease, now)}
	c.p.Effects[1].NoCure = true
	dispel := cureSpell(9811, "Made-up Mending", 30, 0, "Self, Increase HP per tic by 25, Dispel Effect, 24sec")
	c.land(t, dispel, 0xFFFFFFFF, 30*time.Second)
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9800, 9841, 9811) {
		t.Errorf("Dispel Effect: %v, want the disease off, the uncurable and the help left, the heal on", ids)
	}

	// kept over a logout: its spell's level and kind
	c.p.Effects = []*ActiveEffect{afflicted(9851, "Spit", 20, CurePoison|CureDisease, now)}
	c.p.Effects[0].NoCure = true
	rows := savedEffects(c.p, now)
	c.p.Effects = nil
	c.w.RestoreEffects(c.sess, rows, now)
	if len(c.p.Effects) != 1 || c.p.Effects[0].Level != 20 || c.p.Effects[0].Afflicts != CurePoison|CureDisease || !c.p.Effects[0].NoCure {
		t.Errorf("after a logout: %+v", c.p.Effects)
	}

	// what lands keeps its spell's level and kind
	c2 := newCaster(t)
	ward := cureSpell(9860, "Venom Ward", 12, 1, "Target, Increase Resist Poison by 20, 30min")
	c2.land(t, ward, c2.me, 0)
	if len(c2.p.Effects) != 1 || c2.p.Effects[0].Level != 12 || c2.p.Effects[0].Afflicts != CurePoison {
		t.Errorf("landed: %+v", c2.p.Effects)
	}
}

// A cure cast at a pet takes off the pet's, by the same rules.
func TestACureOnAPet(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	now := r.at(20 * time.Second)
	pet.Effects = []*ActiveEffect{afflicted(9871, "Spit", 10, CurePoison, now), afflicted(9872, "Fever", 10, CureDisease, now)}
	spell := cureSpell(9870, "Made-up Pet Antidote", 24, 3, "Pet, Removes Poison")
	if !r.w.StartCast(r.sess, spell, 0xFFFFFFFF, now) {
		t.Fatalf("the cure did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(now.Add(spell.CastTime + castLand))
	r.w.mu.Unlock()
	if ids := effectIDs(pet.Effects); !sameIDs(ids, 9872) {
		t.Errorf("on the pet: %v, want the poison off", ids)
	}
}

// A potion's cure ("Dispel Effect xN") still takes N harmful effects off its
// drinker, of any level (an itemeffect row names none), once.
func TestPotionsStillCure(t *testing.T) {
	c := newCaster(t)
	now := c.at(0)
	pe := NewPotionEffect(50040, "Made-up Restorative", "Self, Dispel Effect x3", "", 0, "", 0, 0)
	if pe.Cures != 3 || pe.CureKind != CureAny || pe.Thrown() {
		t.Fatalf("the restorative read as %+v", pe)
	}
	help := &ActiveEffect{SpellID: 9900, Name: "Made-up Skin", Until: now.Add(time.Hour), Mods: StatMods{AC: 20}}
	c.p.Effects = []*ActiveEffect{help, afflicted(9901, "a", 60, 0, now), afflicted(9902, "b", 60, CurePoison, now),
		afflicted(9903, "c", 60, CureDisease, now), afflicted(9904, "d", 60, 0, now)}
	if refusal := c.use(t, pe, 0, time.Second); refusal != "" {
		t.Fatalf("refused: %s", refusal)
	}
	if ids := effectIDs(c.p.Effects); !sameIDs(ids, 9900, 9904) {
		t.Errorf("after the drink: %v, want three off and the help kept", ids)
	}
}
