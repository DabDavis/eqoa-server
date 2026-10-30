package entity

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The necromancer's zombie, mummy and spectre as Sandstorm's (petrotation.go),
// in the pet rig: its owner a necromancer of 51, the crab given hit points
// to last and only the pets' system run, so it never fights back and the
// pet's hit points move by its taps alone. Each kind is run on the dice's
// lowest throws and on their highest, which pins the ranges: a tap every 2
// swings (all the long one) or every 5, a spectre's instant after every
// swing or every sixth, 0.5 s after it or 0.7.

// The swings' visuals as Sandstorm sent them.
const (
	fxAttack2 uint32 = 0xDA9BEA12
	fxAttack3 uint32 = 0xDA9BEA13
)

const crabObject = 0x0133E8A0

// necroSpell is a necromancer's summoning with this server's own line for it
// (spelleffect): Putrid Flesh's says a zombie, Howling Death's names none.
func necroSpell(id, level int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 30, Level: level, Name: name,
		CastTime: 8 * time.Second, PowerCost: 100, Def: ParseSpellText(line)}
}

func decayingFlesh() CastSpell {
	return necroSpell(918, 25, "Decaying Flesh", "This spell summons pet")
}

func putridFlesh() CastSpell {
	return necroSpell(939, 35, "Putrid Flesh", "Summons a Zombie pet")
}

func howlingDeath() CastSpell {
	return necroSpell(956, 44, "Howling Death", "Summon Pet")
}

// rotationTaps are the necromancer's taps the pets cast (lifetap_test.go's).
var rotationTaps = []PetTap{
	{Level: 16, Name: "Siphon Life", Hit: SpellEffect{Kind: EffectHP, Amount: -29, AmountMax: -29, Drain: DrainCaster}, FX: 0x0A0A0A0A},
	{Level: 34, Name: "Spirit Tap", Hit: SpellEffect{Kind: EffectHP, Amount: -156, AmountMax: -156, Drain: DrainCaster}, FX: 0x0B0B0B0B},
	{Level: 55, Name: "Life's Bane", Hit: SpellEffect{Kind: EffectHP, Amount: -275, AmountMax: -275, Drain: DrainCaster}, FX: 0x0C0C0C0C},
}

var (
	diceLow  = func(int) int { return 0 }
	diceHigh = func(n int) int { return n - 1 }
)

// petCast is one 0x00B1 the pet sent: when (ms of the rig's clock), its
// place among that tick's messages, its target, cast time and visual.
type petCast struct {
	at, order      int
	target, ms, fx uint32
}

func (c petCast) String() string {
	return fmt.Sprintf("%d:%d %#x->%#x %dms", c.at, c.order, c.fx, c.target, c.ms)
}

type petFight struct {
	r      *petRig
	pet    *NPC
	petID  uint32
	casts  []petCast
	told   int // 0x00DB naming the crab
	startH int // the pet's hit points when sent in
}

// necroFight summons spell as a player of class and level 51, puts its pet
// beside the crab at half its hit points and sends it in, then runs the
// pets' system every 100 ms for d on the dice given.
func necroFight(t *testing.T, class int, spell CastSpell, d time.Duration, dice func(int) int) *petFight {
	t.Helper()
	old := petTapIntn
	petTapIntn = dice
	t.Cleanup(func() { petTapIntn = old })
	r := newPetRig(t)
	r.p.Class, r.p.Level = class, 51
	r.w.petTaps = rotationTaps
	r.enemy.HP, r.enemy.MaxHP, r.enemy.Level = 1_000_000, 1_000_000, 1
	pet, _ := r.summon(t, spell, 0)
	ox, oz := pet.X, pet.Z
	pet.X, pet.Y, pet.Z = r.enemy.X-2, r.enemy.Y, r.enemy.Z
	r.w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
	pet.HP = pet.MaxHP / 2
	f := &petFight{r: r, pet: pet, petID: r.w.objectIDForEntityLocked(r.sess, pet.ServerID), startH: pet.HP}
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(9600*time.Millisecond))
	r.sent()
	for ms := 9700; ms <= 9700+int(d/time.Millisecond); ms += 100 {
		r.w.mu.Lock()
		r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
		r.w.mu.Unlock()
		for i, m := range r.sent() {
			switch opOfMsg(m) {
			case protocol.OpcodeCastingCombat:
				if binary.LittleEndian.Uint32(m[2:]) == f.petID {
					f.casts = append(f.casts, petCast{at: ms, order: i, target: binary.LittleEndian.Uint32(m[6:]),
						ms: binary.LittleEndian.Uint32(m[11:]), fx: binary.LittleEndian.Uint32(m[15:])})
				}
			case protocol.OpcodeDamageIndication:
				for _, di := range damageIndications([][]byte{m}) {
					if di[2] == crabObject {
						f.told++
					}
				}
			}
		}
	}
	return f
}

// swings are the pet's swings with that visual; the rest are its spells.
func (f *petFight) split(swingFX uint32) (swings, spells []petCast) {
	for _, c := range f.casts {
		if c.fx == swingFX {
			swings = append(swings, c)
		} else {
			spells = append(spells, c)
		}
	}
	return swings, spells
}

// checkSwings: every swing its visual and cast time, each the round after
// the last, at least n of them.
func checkSwings(t *testing.T, swings []petCast, ms uint32, round, n int) {
	t.Helper()
	if len(swings) < n {
		t.Fatalf("%d swings, want %d or more: %v", len(swings), n, swings)
	}
	for i, s := range swings {
		if s.ms != ms || s.target != crabObject {
			t.Errorf("swing %d: %v, want %d ms at the crab", i, s, ms)
		}
		if i > 0 && s.at-swings[i-1].at != round {
			t.Errorf("swing %d %d ms after the last, want %d", i, s.at-swings[i-1].at, round)
		}
	}
}

// spellAt is the spell sent in swing i's tick, before it, or nil.
func spellAt(swings, spells []petCast, i int) *petCast {
	for j := range spells {
		if spells[j].at == swings[i].at {
			if spells[j].order > swings[i].order {
				return nil
			}
			return &spells[j]
		}
	}
	return nil
}

// tapsAt checks the taps fall on exactly those swings (each in its tick,
// before it) with the visual and cast time given.
func tapsAt(t *testing.T, swings, spells []petCast, at []int, fx, ms uint32) {
	t.Helper()
	want := map[int]bool{}
	for _, i := range at {
		if i < len(swings) {
			want[i] = true
		}
	}
	for i := range swings {
		c := spellAt(swings, spells, i)
		isTap := c != nil && (c.fx == fxPetTap || c.fx == fxPetTapLong)
		if isTap != want[i] {
			t.Errorf("swing %d (%d ms): spell %v, want a tap %v", i, swings[i].at, c, want[i])
			continue
		}
		if isTap && (c.fx != fx || c.ms != ms || c.target != crabObject) {
			t.Errorf("swing %d's tap %v, want %#x %d ms at the crab", i, c, fx, ms)
		}
	}
}

func every(from, step, n int) []int {
	var out []int
	for i := from; i < n; i += step {
		out = append(out, i)
	}
	return out
}

// The pets kinds come from the necromancer's summoning by name, whatever
// case or spacing; any other summoning, and any other class's, is ours.
func TestPetKindsByTheSummoning(t *testing.T) {
	for _, c := range []struct {
		class int
		name  string
		want  petKind
	}{
		{classNecromancer, "Wilting Flesh", petKindZombie},
		{classNecromancer, "Decaying Flesh", petKindZombie},
		{classNecromancer, "Rotting Flesh", petKindZombie},
		{classNecromancer, "Putrid Flesh", petKindMummy},
		{classNecromancer, "Preserved Flesh", petKindMummy},
		{classNecromancer, "Howling Death", petKindSpectre},
		{classNecromancer, "  ANCIENT death ", petKindSpectre},
		// not recorded: ours
		{classNecromancer, "Shambling Bones", petKindOurs},
		{classNecromancer, "Swarming Bones", petKindOurs},
		{classNecromancer, "Swarming Death", petKindOurs},
		{classNecromancer, "Fire Elemental", petKindOurs},
		{10, "Decaying Flesh", petKindOurs},
		{3, "Howling Death", petKindOurs},
	} {
		if got := petKindOf(c.class, c.name); got != c.want {
			t.Errorf("class %d's %q: %v, want %v", c.class, c.name, got, c.want)
		}
	}
}

// The zombie: Attack2 every 4.0 s; the tap on its first swing, then on every
// second swing (the dice's lowest: the long tap, 2000 ms, each time) or
// every fifth (their highest: Drain Spirit's, cast 0), each in its swing's
// tick and before it; no 0x00DB for any hit of its own while the crab bleeds;
// each tap's figure back on the pet. Then pets' regeneration still heals it,
// and a dismissal is answered with 0x007F.
func TestTheZombieFightsAsSandstorms(t *testing.T) {
	for _, c := range []struct {
		name string
		dice func(int) int
		step int
		fx   uint32
		ms   uint32
	}{{"lowest", diceLow, 2, fxPetTapLong, petTapLongMs}, {"highest", diceHigh, 5, fxPetTap, 0}} {
		t.Run(c.name, func(t *testing.T) {
			f := necroFight(t, classNecromancer, decayingFlesh(), 120*time.Second, c.dice)
			if !f.pet.SwingFixed || f.pet.SwingKind != 4 || f.r.w.pets[f.r.p.ServerID].kind != petKindZombie {
				t.Fatalf("the pet's swing %v/%d, kind %v", f.pet.SwingFixed, f.pet.SwingKind, f.r.w.pets[f.r.p.ServerID].kind)
			}
			swings, spells := f.split(fxAttack2)
			checkSwings(t, swings, 4000, 4000, 29)
			tapsAt(t, swings, spells, every(0, c.step, len(swings)), c.fx, c.ms)
			if len(spells) != len(every(0, c.step, len(swings))) {
				t.Errorf("spells %v: taps alone, want %d", spells, len(every(0, c.step, len(swings))))
			}
			if f.told != 0 || f.r.enemy.HP >= f.r.enemy.MaxHP {
				t.Errorf("%d hits told; crab %d of %d: want none told, and the crab hurt", f.told, f.r.enemy.HP, f.r.enemy.MaxHP)
			}
			heal := -f.r.w.petTapForLocked(f.pet.Level).Hit.Amount
			if want := min(f.startH+len(spells)*heal, f.pet.MaxHP); f.pet.HP != want {
				t.Errorf("the pet at %d HP after %d taps of %d from %d, want %d", f.pet.HP, len(spells), heal, f.startH, want)
			}

			// the balance's pet regeneration, left as it was (npcregen.go)
			f.pet.HP = f.pet.MaxHP / 2
			f.r.w.mu.Lock()
			f.r.w.TickCount = 0
			f.r.w.npcRegenSystem(f.r.at(130 * time.Second))
			f.r.w.mu.Unlock()
			if want := f.pet.MaxHP/2 + max(1, f.pet.MaxHP/50); f.pet.HP != want {
				t.Errorf("regenerated to %d, want %d", f.pet.HP, want)
			}

			f.r.w.PetCommand(f.r.sess, petCmdDismiss, 0, f.r.at(131*time.Second))
			var cleared bool
			for _, m := range f.r.sent() {
				cleared = cleared || opOfMsg(m) == protocol.OpcodeClearPet
			}
			if !cleared || f.r.w.petOfLocked(f.r.p) != nil {
				t.Error("the dismissal: no 0x007F, or the pet stays")
			}
		})
	}
}

// The mummy: Attack2 every 4.0 s; on its first swing the stamina drain
// (0x723913E6, 1000 ms) at its target, before the swing, and no tap; then a
// tap 2 or 5 swings on, and so on; nothing told.
func TestTheMummyFightsAsSandstorms(t *testing.T) {
	for _, c := range []struct {
		name string
		dice func(int) int
		step int
		fx   uint32
		ms   uint32
	}{{"lowest", diceLow, 2, fxPetTapLong, petTapLongMs}, {"highest", diceHigh, 5, fxPetTap, 0}} {
		t.Run(c.name, func(t *testing.T) {
			f := necroFight(t, classNecromancer, putridFlesh(), 120*time.Second, c.dice)
			swings, spells := f.split(fxAttack2)
			checkSwings(t, swings, 4000, 4000, 29)
			if o := spellAt(swings, spells, 0); o == nil || o.fx != fxPetStaDrain || o.ms != petOpenerMs || o.target != crabObject {
				t.Fatalf("the opener %v, want the stamina drain at the crab, 1000 ms, before the first swing", o)
			}
			var taps []petCast
			for _, s := range spells[1:] {
				taps = append(taps, s)
			}
			tapsAt(t, swings, taps, every(c.step, c.step, len(swings)), c.fx, c.ms)
			if len(taps) != len(every(c.step, c.step, len(swings))) {
				t.Errorf("spells after the opener %v, want taps alone", taps)
			}
			if f.told != 0 {
				t.Errorf("%d hits told", f.told)
			}
		})
	}
}

// The spectre: Attack3 (5000 ms) every 5.2 s; Cursed Hand's visual at its
// target on its first swing and at itself on its second, nothing on the
// third, the stamina drain on the fourth, each before its swing; then taps
// 2 or 5 swings on. Its instant (0xFD069D13, cast 0, at its target) 0.5 s
// after every swing from the first 20 s into the fight on the lowest dice,
// 0.7 s after every sixth on the highest, never in a swing's tick; nothing
// told.
func TestTheSpectreFightsAsSandstorms(t *testing.T) {
	for _, c := range []struct {
		name     string
		dice     func(int) int
		step     int
		fx, ms   uint32
		specials []int // the swings an instant follows
		lag      int
	}{
		{"lowest", diceLow, 2, fxPetTapLong, petTapLongMs, every(4, 1, 30), 500},
		{"highest", diceHigh, 5, fxPetTap, 0, every(9, 6, 30), 700},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := necroFight(t, classNecromancer, howlingDeath(), 120*time.Second, c.dice)
			if !f.pet.SwingFixed || f.pet.SwingKind != 3 || f.pet.SwingRound != 5200*time.Millisecond {
				t.Fatalf("the pet's swing %v/%d/%v", f.pet.SwingFixed, f.pet.SwingKind, f.pet.SwingRound)
			}
			swings, rest := f.split(fxAttack3)
			checkSwings(t, swings, 5000, 5200, 22)
			var spells, specials []petCast
			for _, s := range rest {
				if s.fx == fxPetSpecial {
					specials = append(specials, s)
				} else {
					spells = append(spells, s)
				}
			}
			for i, want := range []struct {
				fx, target uint32
			}{{fxPetSpectre, crabObject}, {fxPetSpectre, f.petID}, {0, 0}, {fxPetStaDrain, crabObject}} {
				o := spellAt(swings, spells, i)
				if want.fx == 0 {
					if o != nil {
						t.Errorf("swing %d carries %v, want nothing", i, o)
					}
					continue
				}
				if o == nil || o.fx != want.fx || o.target != want.target || o.ms != petOpenerMs {
					t.Errorf("swing %d's opener %v, want %#x at %#x, 1000 ms", i, o, want.fx, want.target)
				}
			}
			tapsAt(t, swings, spells[3:], every(3+c.step, c.step, len(swings)), c.fx, c.ms)

			var want []int
			for _, i := range c.specials {
				if i < len(swings) && swings[i].at+c.lag <= 9700+120000 {
					want = append(want, swings[i].at+c.lag)
				}
			}
			var got []int
			for _, s := range specials {
				got = append(got, s.at)
				if s.ms != 0 || s.target != crabObject {
					t.Errorf("an instant %v, want cast 0 at the crab", s)
				}
				for _, sw := range swings {
					if sw.at == s.at {
						t.Errorf("an instant in a swing's tick: %v", s)
					}
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("the instants at %v, want %v (swings at %v)", got, want, swings)
			}
			if swings[4].at-swings[0].at < int(petSpecialFrom/time.Millisecond) || swings[3].at-swings[0].at >= int(petSpecialFrom/time.Millisecond) {
				t.Errorf("the fifth swing %d ms in: the first past 20 s should be the fifth", swings[4].at-swings[0].at)
			}
			if f.told != 0 {
				t.Errorf("%d hits told", f.told)
			}
		})
	}
}

// The controls: pets not recorded keep ours. A necromancer's skeleton
// (Shambling Bones, too young to tap) and its Swarming Death (a level 60
// pet, of its owner's 51: ours taps every 30 s with the tap's own visual,
// Spirit Tap's) and a magician's Fire
// Elemental send none of the rotation's visuals, swing as their level and
// class make them, and tell their owner each hit.
func TestUnrecordedPetsKeepOurs(t *testing.T) {
	rotation := map[uint32]bool{fxPetTap: true, fxPetTapLong: true, fxPetStaDrain: true, fxPetSpectre: true, fxPetSpecial: true}
	for _, c := range []struct {
		name  string
		class int
		spell CastSpell
		taps  int // ours: the tap's own visual every 30 s
	}{
		{"Shambling Bones", classNecromancer, necroSpell(700011, 1, "Shambling Bones", "Summons a Skeleton to aid you"), 0},
		{"Swarming Death", classNecromancer, necroSpell(700823, 60, "Swarming Death", "Summon Pet"), 3},
		{"Fire Elemental", 10, fireElemental(), 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := necroFight(t, c.class, c.spell, 65*time.Second, diceLow)
			if ps := f.r.w.pets[f.r.p.ServerID]; ps.kind != petKindOurs || f.pet.SwingRound != 0 {
				t.Fatalf("kind %v, round %v: want ours", ps.kind, f.pet.SwingRound)
			}
			var taps int
			for _, s := range f.casts {
				if rotation[s.fx] {
					t.Errorf("a rotation's visual: %v", s)
				}
				for _, tp := range rotationTaps {
					if s.fx == tp.FX {
						taps++
					}
				}
			}
			if taps != c.taps || f.told == 0 {
				t.Errorf("%d of ours taps, %d hits told; want %d taps and hits told", taps, f.told, c.taps)
			}
		})
	}
}

// A spectre's instant comes when due, within a second of it; one later than
// that (its pet off chasing, out of reach, when it fell due) is dropped, not
// sent late. The control: one 0.3 s late is sent.
func TestASpectresLateInstantIsDropped(t *testing.T) {
	r := newPetRig(t)
	r.p.Class = classNecromancer
	pet, _ := r.summon(t, howlingDeath(), 0)
	ps := r.w.pets[r.p.ServerID]
	for _, c := range []struct {
		late time.Duration
		sent bool
	}{{300 * time.Millisecond, true}, {1500 * time.Millisecond, false}} {
		now := r.at(60 * time.Second)
		ps.specialAt = now.Add(-c.late)
		r.w.mu.Lock()
		r.w.petSpecialDueLocked(ps, r.enemy, now)
		r.w.mu.Unlock()
		var sent bool
		for _, m := range r.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat && binary.LittleEndian.Uint32(m[15:]) == fxPetSpecial &&
				binary.LittleEndian.Uint32(m[2:]) == r.w.objectIDForEntityLocked(r.sess, pet.ServerID) {
				sent = true
			}
		}
		if sent != c.sent || !ps.specialAt.IsZero() {
			t.Errorf("%v late: sent %v, want %v; still lined up %v", c.late, sent, c.sent, !ps.specialAt.IsZero())
		}
	}
}

// A recorded kind's melee keeps the damage a second our round gave the pet:
// on the same fixed dice, with no taps, over two minutes the zombie of 25,
// the mummy of 35 and the spectre of 44 take off what the same pet fighting
// as ours (the same summoning under a name not recorded: the same body,
// level and round) takes off, within 5%. Each hit is scaled by our round's
// hits a second over Sandstorm's: x1 for the zombie (our round at 25 is
// Sandstorm's), x4/3 for the mummy (four hits in 6 s against two in 4) and
// x1.73 for the spectre (two in 5.2 s).
func TestARecordedPetKeepsItsDamageASecond(t *testing.T) {
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	dealt := func(t *testing.T, spell CastSpell) (int, float64, petKind) {
		t.Helper()
		r := newPetRig(t)
		r.p.Class, r.p.Level = classNecromancer, 51
		r.w.petTaps = nil
		r.enemy.HP, r.enemy.MaxHP, r.enemy.Level = 1_000_000, 1_000_000, 1
		pet, _ := r.summon(t, spell, 0)
		ox, oz := pet.X, pet.Z
		pet.X, pet.Y, pet.Z = r.enemy.X-2, r.enemy.Y, r.enemy.Z
		r.w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
		r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(9600*time.Millisecond))
		for ms := 9700; ms <= 9700+120000; ms += 100 {
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
			r.sent()
		}
		ps := r.w.pets[r.p.ServerID]
		return r.enemy.MaxHP - r.enemy.HP, ps.hitScale, ps.kind
	}
	for _, c := range []struct {
		name  string
		spell CastSpell
		scale float64
	}{
		{"zombie", decayingFlesh(), 1},
		{"mummy", putridFlesh(), (4.0 / 6) / (2.0 / 4)},
		{"spectre", howlingDeath(), (4.0 / 6) / (2 / 5.2)},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, scale, kind := dealt(t, c.spell)
			ours := c.spell
			ours.Name += " Unrecorded"
			want, oursScale, oursKind := dealt(t, ours)
			if kind == petKindOurs || oursKind != petKindOurs || oursScale != 1 {
				t.Fatalf("kinds %v and %v (scale %v): want a recorded kind against ours", kind, oursKind, oursScale)
			}
			if math.Abs(scale-c.scale) > 1e-6 {
				t.Errorf("hits x%.4f, want x%.4f", scale, c.scale)
			}
			if want == 0 || math.Abs(float64(got-want)) > 0.05*float64(want) {
				t.Errorf("took off %d in two minutes, ours %d: want within 5%%", got, want)
			}
		})
	}
}
