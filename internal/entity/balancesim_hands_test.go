package entity

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The simulator's hands are the server's (balancekit.go SimHands, the
// switch of handlers/handswitch.go): what a kit holds decides its swing, its
// DMG and which of its arts it may cast, and it switches for an art as a
// player's hands are switched. The kits and arts here are made up.

// simArt is a made-up art (no cast time, at a creature) of the damage, that
// names required equipment.
func simArt(id int, name, req string, dmg int) CastSpell {
	def := ParseSpellText("Target, Decrease Hitpoints by " + strconv.Itoa(dmg))
	def.ReqEquip = req
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: 30, Scope: 1, Name: name, Recast: 6 * time.Second,
		PowerCost: 10, Range: 25, Def: def}
}

// simFightLog fights one fight with the log kept: the result and the log.
func simFightLog(pl SimPlayer, foe SimNPC) (SimResult, string) {
	var buf bytes.Buffer
	simLogTo = &buf
	defer func() { simLogTo = nil }()
	r := SimulateFight(pl, foe, SeedBalance(), 1, 10*time.Minute)
	return r, buf.String()
}

// A ranger dual wielding its blades, its bow carried, casts its arrow art
// and its blade art alike: the bow taken up for the one and the blades for
// the other, never refused for its weapon, and with the bow in hand its
// rounds are the bow's (one hit at the bow's reach). The control: the same
// kit with no bow has its every arrow art refused, and fights at the
// blades' reach alone.
func TestASimKitSwitchesItsHandsForItsArts(t *testing.T) {
	blades := SimHands{AttackType: 1, Primary: 1, Secondary: 1, Gear: Gear{WeaponCond: 1, WeaponDamage: 300, WeaponSlot: 14, WeaponType: 1}}
	blades.Worn[14], blades.Worn[15] = true, true
	bow := SimHands{AttackType: 7, Range: 25, Secondary: 7, Gear: Gear{WeaponCond: 1, WeaponDamage: 330, WeaponSlot: 18, WeaponType: 7}}
	bow.Worn[18] = true
	kit := simKit(1, 30)
	kit.Body = kit.Gear
	kit.Body.WeaponDamage = 0
	kit.Spells = []CastSpell{simArt(9201, "an arrow", "Range", 200), simArt(9202, "a blade", "1HS,2HS", 180)}
	kit.Play = SimPlay{Melee: true, Spells: true}
	foe := SimNPC{Level: 31}

	kit.Hands = []SimHands{blades, bow}
	r, logged := simFightLog(kit, foe)
	arrows, edges := strings.Count(logged, `begins "an arrow"`), strings.Count(logged, `begins "a blade"`)
	if arrows == 0 || edges == 0 || strings.Contains(logged, "right weapon") || !strings.Contains(logged, "hands changed") {
		t.Errorf("with its bow: %d arrows, %d blades, refused for its weapon %v, hands changed %v (won %v in %v)",
			arrows, edges, strings.Contains(logged, "right weapon"), strings.Contains(logged, "hands changed"), r.Won, r.Time)
	}
	if !strings.Contains(logged, "reach 25.00") || !strings.Contains(logged, "reach 4.50") {
		t.Error("its rounds not both the blades' (reach 4.5) and the bow's (25)")
	}

	kit.Hands = []SimHands{blades}
	_, logged = simFightLog(kit, foe)
	if strings.Contains(logged, `begins "an arrow"`) || !strings.Contains(logged, "right weapon") || strings.Contains(logged, "reach 25.00") {
		t.Error("the control: with no bow its arrow arts were cast, or it fought at a bow's reach")
	}
}

// A monk's kit is bare-handed: its punches (None) are cast and its melee
// rolls its hand to hand, so it wins faster than with the bare hands that
// rolled a weapon's 0. The control: a monk holding a weapon, as the kits
// had it, has its every punch refused.
func TestASimMonkPunchesBareHanded(t *testing.T) {
	bare := simKit(classMonk, 30)
	bare.Worn, bare.AttackType, bare.SecondaryType = [21]bool{}, 0, 0
	bare.Gear.WeaponDamage, bare.Gear.BareHands = 0, true
	bare.Spells = []CastSpell{simArt(9301, "a punch", "None", 150)}
	bare.Play = SimPlay{Melee: true, Spells: true}
	foe := SimNPC{Level: 30}
	r, logged := simFightLog(bare, foe)
	if !r.Won || !strings.Contains(logged, `begins "a punch"`) || strings.Contains(logged, "right weapon") {
		t.Fatalf("a bare-handed monk: won %v, punched %v", r.Won, strings.Contains(logged, `begins "a punch"`))
	}
	zero := bare
	zero.Gear.BareHands = false // a weapon's 0, as bare hands rolled before
	if was, _ := simFightLog(zero, foe); was.Won && was.Time <= r.Time {
		t.Errorf("hand to hand won in %v, a weapon's 0 in %v", r.Time, was.Time)
	}
	armed := simKit(classMonk, 30)
	armed.Spells = bare.Spells
	armed.Play = bare.Play
	if _, logged := simFightLog(armed, foe); strings.Contains(logged, `begins "a punch"`) || !strings.Contains(logged, "right weapon") {
		t.Error("the control: a monk with a weapon punched")
	}
}

// A ranger whose only art is an arrow art, fighting in melee, is back on its
// blades after each one has landed, as the server puts a player's back
// (handlers/handswitch.go HandsBack): after its first arrow its hits are the
// blades' (reach 4.5) again, a bow's (one hit a round, reach 25) only from
// a round begun before an arrow landed, never more than one an arrow. The
// control: a kit whose fighting hands hold nothing has nothing to put back,
// and every hit after its first arrow is the bow's.
func TestASimRangerIsBackOnItsBladesAfterEachArrowArt(t *testing.T) {
	blades := SimHands{AttackType: 1, Primary: 1, Secondary: 1, Gear: Gear{WeaponCond: 1, WeaponDamage: 300, WeaponSlot: 14, WeaponType: 1}}
	blades.Worn[14], blades.Worn[15] = true, true
	bow := SimHands{AttackType: 7, Range: 25, Secondary: 7, Gear: Gear{WeaponCond: 1, WeaponDamage: 330, WeaponSlot: 18, WeaponType: 7}}
	bow.Worn[18] = true
	kit := simKit(1, 30)
	kit.Body = kit.Gear
	kit.Body.WeaponDamage = 0
	kit.Spells = []CastSpell{simArt(9201, "an arrow", "Range", 200)}
	kit.Play = SimPlay{Melee: true, Spells: true}
	foe := SimNPC{Level: 31}
	// the arrows, and the player's own hits after its first (not the
	// creature's on it), by their reach
	after := func(logged string) (arrows, blade, bowed int) {
		i := strings.Index(logged, `begins "an arrow"`)
		if i < 0 {
			return 0, 0, 0
		}
		for _, l := range strings.Split(logged[i:], "\n") {
			switch {
			case strings.Contains(l, `begins "an arrow"`):
				arrows++
			case !strings.Contains(l, "Combat: Simmer hit"):
			case strings.HasSuffix(l, "reach 4.50"):
				blade++
			case strings.HasSuffix(l, "reach 25.00"):
				bowed++
			}
		}
		return arrows, blade, bowed
	}

	kit.Hands = []SimHands{blades, bow}
	r, logged := simFightLog(kit, foe)
	arrows, blade, bowed := after(logged)
	if arrows < 2 || blade == 0 || bowed > arrows {
		t.Errorf("with its blades: %d arrows, then %d blade hits and %d bow hits (won %v in %v); want blade hits, at most a bow hit an arrow",
			arrows, blade, bowed, r.Won, r.Time)
	}

	kit.Hands = []SimHands{simBareHands(), bow}
	_, logged = simFightLog(kit, foe)
	if arrows, blade, bowed := after(logged); arrows == 0 || blade != 0 || bowed == 0 {
		t.Errorf("the control, bare hands: %d arrows, then %d hits at a melee reach and %d with the bow; want the bow's alone", arrows, blade, bowed)
	}
}

// The simulator gives back a bow an earlier switch took up as the server
// does: a ranger whose arrow art took its bow up at range (the bow kept, out
// of a melee reach) casts its next one in melee with the bow in hand, no
// switch of its own, and is back on its blades once it has landed. The
// control: a bow taken up by a change of hands that is no switch (hold, as
// the item menu's) stays.
func TestASimRangerPulledAtRangeIsBackOnItsBladesAfterAnArrowArtInMelee(t *testing.T) {
	blades := SimHands{AttackType: 1, Primary: 1, Secondary: 1, Gear: Gear{WeaponCond: 1, WeaponDamage: 300, WeaponSlot: 14, WeaponType: 1}}
	blades.Worn[14], blades.Worn[15] = true, true
	bow := SimHands{AttackType: 7, Range: 25, Secondary: 7, Gear: Gear{WeaponCond: 1, WeaponDamage: 330, WeaponSlot: 18, WeaponType: 7}}
	bow.Worn[18] = true
	kit := simKit(1, 30)
	kit.Body = kit.Gear
	kit.Body.WeaponDamage = 0
	arrow := simArt(9201, "an arrow", "Range", 50)
	kit.Spells = []CastSpell{arrow}
	kit.Hands = []SimHands{blades, bow}
	for _, switched := range []bool{true, false} {
		f := newSimFight(kit, SimNPC{Level: 31}, SeedBalance())
		f.now = f.t0
		at := func(x float32) {
			f.w.mu.Lock()
			f.npc.X = f.p.X + x
			f.p.Lockouts, f.p.artReady = nil, time.Time{} // its lockout waited out
			f.w.mu.Unlock()
		}
		shoot := func(when string) {
			s := arrow
			f.handsFor(&s, simFoeID)
			if !f.w.StartCast(f.sess, s, simFoeID, f.now) {
				t.Fatalf("switched %v: %s, the arrow did not begin", switched, when)
			}
			for range int((castLand + simTick) / simTick) {
				f.tick()
			}
		}
		if switched {
			at(20)
			shoot("at range")
			if f.hands != 1 {
				t.Fatalf("at range the bow was not kept: hands %d", f.hands)
			}
		} else {
			f.hold(1) // the bow by a change of hands that is no switch
		}
		at(3)
		shoot("in melee")
		if want := map[bool]int{true: 0, false: 1}[switched]; f.hands != want {
			t.Errorf("switched %v: hands %d after the arrow in melee, want %d", switched, f.hands, want)
		}
	}
}
