package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The vampires' abilities (vampire.go), as tools/seed_vampires.py makes them.

// vampireAbility is one of the seed's records: its name, level, cast and
// recast, scope, description and line.
type vampireAbility struct {
	id, level, cast, recast, scope int
	name, desc, line               string
}

// seedAbilities reads the abilities out of tools/seed_vampires.py, so the
// lines the server reads are the ones the seed writes; nil without the file.
func seedAbilities(t *testing.T) []vampireAbility {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "tools", "seed_vampires.py"))
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`(?s)\((\d+), "([^"]+)", (\d+), (\d+), (\d+), (\d+), \w+, \w+,\s*"([^"]*)",\s*"([^"]*)"\)`)
	var out []vampireAbility
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
		out = append(out, vampireAbility{id: n(1), name: m[2], level: n(3), cast: n(4), recast: n(5), scope: n(6), desc: m[7], line: m[8]})
	}
	return out
}

// vampireSpell is an ability as the cast handler builds it (handlers'
// castSpell: ParseSpellNamed), at range 30.
func vampireSpell(a vampireAbility) CastSpell {
	return CastSpell{SpellID: a.id, DisplayIndex: 7, Level: a.level, Scope: a.scope, Name: a.name, Icon: 0x795FDEB6,
		CastTime: time.Duration(a.cast) * time.Second, Recast: time.Duration(a.recast) * time.Second, Range: 30,
		Def: ParseSpellNamed(a.name, a.line, a.desc)}
}

// spellSummary is what a line reads as, in words: each effect, the
// illusion, the reagent, the time, harmful or not, and what is left unread.
func spellSummary(d SpellDef) string {
	var parts []string
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP:
			s := fmt.Sprintf("hp %d..%d", e.Amount, e.AmountMax)
			if e.Self {
				s += " self"
			}
			if e.Drain != DrainNone {
				s += " drain"
			}
			parts = append(parts, s)
		case EffectStat:
			if e.Set {
				parts = append(parts, fmt.Sprintf("%s=%d", statLineNames[e.Stat], e.Amount))
			} else {
				parts = append(parts, fmt.Sprintf("%s%+d", statLineNames[e.Stat], e.Amount))
			}
		case EffectPowerCap:
			parts = append(parts, fmt.Sprintf("power cap %d", e.Amount))
		case EffectForget:
			parts = append(parts, "forget")
		case EffectIllusion:
			parts = append(parts, "form "+d.Form)
		default:
			parts = append(parts, fmt.Sprintf("kind %d", e.Kind))
		}
	}
	if d.Reagent != "" {
		parts = append(parts, fmt.Sprintf("reagent %s x%d", d.Reagent, d.ReagentCount))
	}
	if d.Duration > 0 {
		parts = append(parts, d.Duration.String())
	}
	if d.Harmful {
		parts = append(parts, "harmful")
	}
	for _, u := range d.Unread {
		parts = append(parts, "unread "+u)
	}
	return strings.Join(parts, ", ")
}

// Each of the seed's nine lines reads as the ability it is: Feed drains and
// summons its blood, Sanctuary teleports, the forms take their bodies, a
// stat set is a set, the gaze forgets, each costs its blood. The controls:
// "Increase STR by 500" is a raise, not a set, and "Set Maximum Power to 1"
// stays a ceiling on power.
func TestTheSeedsVampireAbilitiesRead(t *testing.T) {
	abilities := seedAbilities(t)
	if abilities == nil {
		t.Skip("no tools/seed_vampires.py beside this package")
	}
	want := map[string]string{
		"Feed":            "hp -60..-40 drain, harmful, unread summon unit of blood x1",
		"Sanctuary":       "reagent unit of blood x2, unread teleport to the sanctuary",
		"Vampiric Strike": "hp -1500..-1200, hp 400..400 self, reagent unit of blood x7, harmful",
		"Vampire Form":    "form vampire, reagent unit of blood x2, 10m0s",
		"Bat Form":        "STR=500, AC-963, power cap 1, form bat, reagent unit of blood x8, 30s",
		"Vitality":        "hp 1000..1000, AC+300, reagent unit of blood x5, 1m0s",
		"Frenzy":          "damage+500, AC-300, reagent unit of blood x5, 1m0s, harmful",
		"Mist Form":       "AC+2000, STR=0, power cap 1, form mist, reagent unit of blood x8, 30s",
		"Vampiric Gaze":   "forget, reagent unit of blood x10, harmful",
	}
	if len(abilities) != len(want) {
		t.Fatalf("the seed has %d abilities, want %d", len(abilities), len(want))
	}
	for _, a := range abilities {
		if got := spellSummary(ParseSpellNamed(a.name, a.line, a.desc)); got != want[a.name] {
			t.Errorf("%s (%d): %q\n  reads %q\n  want  %q", a.name, a.id, a.line, got, want[a.name])
		}
	}
	if teleports, _ := gateLine(ptr(ParseSpellText(abilities[1].line))); abilities[1].name != "Sanctuary" || !teleports {
		t.Errorf("Sanctuary's line is no teleport to the gates")
	}
	for line, w := range map[string]string{
		"Self, Increase STR by 500, 30sec":    "STR+500, 30s",
		"Self, Set Maximum Power to 1, 30sec": "power cap 1, 30s",
	} {
		if got := spellSummary(ParseSpellText(line)); got != w {
			t.Errorf("the control %q reads %q, want %q", line, got, w)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// vampireAbilityNamed is the seed's ability of the name, or the test fails
// (skipped without the seed).
func vampireAbilityNamed(t *testing.T, name string) vampireAbility {
	t.Helper()
	abilities := seedAbilities(t)
	if abilities == nil {
		t.Skip("no tools/seed_vampires.py beside this package")
	}
	for _, a := range abilities {
		if a.name == name {
			return a
		}
	}
	t.Fatalf("no %q in the seed", name)
	return vampireAbility{}
}

// A stat set is the stat, not a raise: Bat Form's STR 500 held to the cap
// (400 at level 50), Mist Form's STR 0 whatever the character has, and what
// another effect adds still counting (a +12 song on Mist Form: 12). Both
// wear off to the character's own. The controls: the same figure raised,
// "Increase STR by 100", is 160 on a STR of 60 where "Set STR to 100" is
// 100.
func TestAVampiresFormSetsItsStat(t *testing.T) {
	bat := vampireSpell(vampireAbilityNamed(t, "Bat Form"))
	c := newCaster(t)
	c.p.Level, c.p.Str = 50, 260
	c.land(t, bat, c.me, 0)
	if got := c.p.StatNow(StatStr); got != 400 {
		t.Errorf("Bat Form at level 50: STR %d, want 400 (500 held to the cap)", got)
	}
	if got := c.p.StatNow(StatAC); got != -963 {
		t.Errorf("Bat Form: AC %d, want -963", got)
	}
	if c.p.MaxPowerNow() != 1 || c.p.Power > 1 {
		t.Errorf("Bat Form: power %d of %d, want 1 at most", c.p.Power, c.p.MaxPowerNow())
	}
	c.tick(bat.CastTime + castLand + 31*time.Second)
	if got := c.p.StatNow(StatStr); got != 260 {
		t.Errorf("Bat Form worn off: STR %d, want the character's 260", got)
	}

	mist := vampireSpell(vampireAbilityNamed(t, "Mist Form"))
	m := newCaster(t)
	m.p.Level, m.p.Str = 50, 450 // above the cap: its own is what is set from
	m.land(t, mist, m.me, 0)
	if got := m.p.StatNow(StatStr); got != 0 {
		t.Errorf("Mist Form: STR %d, want 0", got)
	}
	if got := m.p.StatNow(StatAC); got != 2000 {
		t.Errorf("Mist Form: AC %d, want 2000", got)
	}
	song := CastSpell{SpellID: 905, DisplayIndex: 2, Scope: 0, Name: "Strength Song", CastTime: time.Second, Recast: time.Second,
		Range: 30, Def: ParseSpellText("Self, Increase STR by 12, 10min")}
	m.land(t, song, m.me, 10*time.Second)
	if got := m.p.StatNow(StatStr); got != 12 {
		t.Errorf("Mist Form under a +12 song: STR %d, want 12", got)
	}

	for _, tc := range []struct {
		line string
		want int
	}{{"Self, Set STR to 100, 1min", 100}, {"Self, Increase STR by 100, 1min", 160}} {
		k := newCaster(t)
		sp := CastSpell{SpellID: 906, DisplayIndex: 2, Scope: 0, Name: "Test", CastTime: time.Second, Recast: time.Second,
			Range: 30, Def: ParseSpellText(tc.line)}
		k.land(t, sp, k.me, 0)
		if got := k.p.StatNow(StatStr); got != tc.want {
			t.Errorf("%q on STR 60: %d, want %d", tc.line, got, tc.want)
		}
	}
}

// Frenzy adds its 500 to every melee hit its bearer lands, and no more once
// it wears off; the control is the same hit without it.
func TestFrenzyAddsToEveryMeleeHit(t *testing.T) {
	frenzy := vampireSpell(vampireAbilityNamed(t, "Frenzy"))
	was := meleeIntn
	meleeIntn = func(n int) int { return 0 } // the bottom of every roll
	defer func() { meleeIntn = was }()
	c := newCaster(t)
	c.p.Level = 60
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 60, 100000, 100000
	hit := func() int {
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		before := c.npc.HP
		c.w.playerHitLocked(c.sess, c.p, c.npc, c.npcO)
		return before - c.npc.HP
	}
	plain := hit()
	c.land(t, frenzy, c.me, 0)
	if got := hit(); got != plain+500 {
		t.Errorf("a hit under Frenzy: %d, want %d (the control's %d and 500)", got, plain+500, plain)
	}
	c.tick(frenzy.CastTime + castLand + 61*time.Second)
	if got := hit(); got != plain {
		t.Errorf("a hit after Frenzy wore off: %d, want %d", got, plain)
	}
}

// The gaze takes the creature's hate for its caster: off the threat list,
// the creature turned on the next it hates, and no hate of 1 left for the
// spell (as any spell that holds or slows leaves). The control: a hit on it
// adds to the caster's.
func TestVampiricGazeForgetsItsCaster(t *testing.T) {
	gaze := vampireSpell(vampireAbilityNamed(t, "Vampiric Gaze"))
	c := newCaster(t)
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 60, 5000, 5000
	const other EntityID = 4242
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 900, other: 300}
	c.npc.CurrentEnemy = c.p.ServerID
	c.land(t, gaze, c.npcO, 0)
	if _, hated := c.npc.Threats[c.p.ServerID]; hated || c.npc.CurrentEnemy != other || c.npc.Threats[other] != 300 {
		t.Errorf("after the gaze: threats %v, enemy %d; want the caster gone and the other (300) its enemy", c.npc.Threats, c.npc.CurrentEnemy)
	}
	alone := newCaster(t)
	alone.npc.HP, alone.npc.MaxHP = 5000, 5000
	alone.npc.Threats = map[EntityID]int64{alone.p.ServerID: 50}
	alone.npc.CurrentEnemy = alone.p.ServerID
	alone.land(t, gaze, alone.npcO, 0)
	if len(alone.npc.Threats) != 0 || alone.npc.CurrentEnemy != 0 {
		t.Errorf("its one enemy gazed: threats %v, enemy %d; want none", alone.npc.Threats, alone.npc.CurrentEnemy)
	}
	k := newCaster(t)
	k.npc.HP, k.npc.MaxHP = 5000, 5000
	k.npc.Threats = map[EntityID]int64{k.p.ServerID: 50}
	k.land(t, strike(), k.npcO, 0)
	if k.npc.Threats[k.p.ServerID] <= 50 {
		t.Errorf("the control: a hit left the caster's hate at %d", k.npc.Threats[k.p.ServerID])
	}
}

// Feed takes 40 to 60 from its target, gives the vampire what it took, and
// summons its Unit of Blood; the control: a plain hit of the same figure
// summons nothing.
func TestFeedDrainsAndSummonsBlood(t *testing.T) {
	feed := vampireSpell(vampireAbilityNamed(t, "Feed"))
	nuke := feed
	nuke.Def = ParseSpellText("Target, Decrease Hitpoints by 40 to 60")
	for _, tc := range []struct {
		spell CastSpell
		blood bool
	}{{feed, true}, {nuke, false}} {
		c := newCaster(t)
		c.p.HP = 300
		c.npc.Level, c.npc.HP, c.npc.MaxHP = 23, 1000, 1000
		got := make(chan string, 4)
		c.w.OnSummonItem(func(_ *session.Session, char uint32, item string, count int, spell string) {
			got <- fmt.Sprintf("%d %s x%d by %s", char, item, count, spell)
		})
		c.land(t, tc.spell, c.npcO, 0)
		took := 1000 - c.npc.HP
		if took < 40 || took > 60 {
			t.Errorf("%q took %d, want 40 to 60", tc.spell.Def.Unread, took)
		}
		if heal := c.p.HP - 300; tc.blood && heal != took {
			t.Errorf("Feed healed %d of the %d it took", heal, took)
		}
		select {
		case s := <-got:
			if !tc.blood || s != "0 unit of blood x1 by Feed" {
				t.Errorf("summoned %q (want blood: %v)", s, tc.blood)
			}
		case <-time.After(300 * time.Millisecond):
			if tc.blood {
				t.Error("Feed summoned no blood")
			}
		}
	}
}

// A vampire drinks no potion that makes it invisible or quickens it (the
// guide: "Prevents invis/sow pots"); a healing one it does. The control: a
// mortal drinks them all.
func TestAVampireDrinksNoInvisibilityOrSpeed(t *testing.T) {
	c := newCaster(t)
	potions := map[string]bool{"Self, Invis, 4min": true, "Self, Sow, 30min": true, "Self, Increase HP by 100": false}
	for line, refused := range potions {
		pe := PotionEffect{ItemName: "Test Potion", Def: ParseSpellText(line)}
		c.p.Vampire = false
		if got := c.w.VampirePotionRefusal(c.sess, pe); got != "" {
			t.Errorf("a mortal refused %q: %q", line, got)
		}
		c.p.Vampire = true
		if got := c.w.VampirePotionRefusal(c.sess, pe); (got != "") != refused {
			t.Errorf("a vampire, %q: %q, want refused %v", line, got, refused)
		}
	}
}

// Vampire Form shows a fledgling vampire's body, Bat Form a bat's; Mist Form
// has no body here and lands as its stats. The control: with no fledgling
// vampire on the server Vampire Form changes nothing.
func TestTheVampireFormsBodies(t *testing.T) {
	const fledgling, bat uint32 = 3457908867, 2163553232
	forms := func(c *caster) {
		c.w.NPCs[6001] = &NPC{ServerID: 6001, Name: "fledgling vampire", Level: 35, ModelID: int64(fledgling), Size: 1}
		for i := 0; i < 2; i++ {
			c.w.NPCs[EntityID(6002+i)] = &NPC{ServerID: EntityID(6002 + i), Name: "bat", Level: 5, ModelID: int64(bat), Size: 0.8}
		}
	}
	c := illusionRig(t)
	forms(c)
	c.land(t, vampireSpell(vampireAbilityNamed(t, "Vampire Form")), c.me, 0)
	if m, _ := c.p.Body(); m != fledgling {
		t.Errorf("Vampire Form shows %#x, want the fledgling vampire's %#x", m, fledgling)
	}
	c.p.Lockouts = nil
	c.land(t, vampireSpell(vampireAbilityNamed(t, "Bat Form")), c.me, 10*time.Second)
	if m, size := c.p.Body(); m != bat || size != 0.8 {
		t.Errorf("Bat Form shows %#x at %.2f, want the bat's %#x at 0.80", m, size, bat)
	}
	m := illusionRig(t)
	forms(m)
	m.land(t, vampireSpell(vampireAbilityNamed(t, "Mist Form")), m.me, 0)
	if body, _ := m.p.Body(); body != gnomeMale || m.p.StatNow(StatAC) != 2000 {
		t.Errorf("Mist Form: body %#x, AC %d; want the gnome's own and 2000", body, m.p.StatNow(StatAC))
	}
	k := illusionRig(t)
	k.land(t, vampireSpell(vampireAbilityNamed(t, "Vampire Form")), k.me, 0)
	if body, _ := k.p.Body(); body != gnomeMale {
		t.Errorf("the control: no fledgling vampire, and Vampire Form shows %#x", body)
	}
}

// sanctuaryRock is a floor at 273.62 under rock 48 above it within reach of
// the Sanctuary's landing, and open ground at outside past it.
type sanctuaryRock struct{ reach, outside float32 }

func (g sanctuaryRock) Floor(_ uint8, x, y, z float32) (float32, bool) {
	if distXZ(x, z, 4322.19, 6179.78) < g.reach {
		switch {
		case y+2 >= 322:
			return 322, true // the rock's top
		case y+2 >= 273.62:
			return 273.62, true
		}
		return 0, false
	}
	return g.outside, y+2 >= g.outside
}

// Sanctuary's gate lands where Sandstorm's spell does, at its place under
// the rock: a gate's search looks for open sky round its place within 12 of
// its floor and, finding none (open ground 20 below the chamber's floor, as
// round the rock where the Sanctuary's NPCs stood), lands on the place
// itself. The control: open ground
// level with the floor 25 off, and the search takes the traveller out from
// under the rock (the rule a cave's gate keeps), so the Sanctuary's rests on
// there being no such ground near (the real bake's: TestRealSanctuary).
func TestTheSanctuaryGateLandsUnderTheRock(t *testing.T) {
	row := database.SpellGate{Spell: "sanctuary", Kind: database.GateSelf, Place: "The Sanctuary", World: 5, X: 4322.19, Y: 273.62, Z: 6179.78}
	land := func(g Ground) (float32, float32, float32) {
		w := NewWorld()
		w.ground = g
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.gateLandingLocked(row)
	}
	if x, y, z := land(sanctuaryRock{reach: 25, outside: 253.5}); x != row.X || y != row.Y || z != row.Z {
		t.Errorf("under the rock: landed at (%.2f, %.2f, %.2f), want the place itself", x, y, z)
	}
	if x, _, z := land(sanctuaryRock{reach: 25, outside: 273.62}); distXZ(x, z, row.X, row.Z) < 25 {
		t.Errorf("the control: open ground level with the floor 25 off, and the gate landed %.1f off, under the rock", distXZ(x, z, row.X, row.Z))
	}
}

// Sanctuary, as the seed makes it (its line and its spellgates row), sends
// its caster alone to the landing, its group left; the control: with no row
// it is refused as it begins, as any teleport with no place.
func TestSanctuarySendsItsCasterToTheLanding(t *testing.T) {
	spell := vampireSpell(vampireAbilityNamed(t, "Sanctuary"))
	f := newGateFixture(t)
	f.member(1002, "Friend", 1003, 50, 1000)
	f.gate(database.SpellGate{Spell: "Sanctuary", Kind: database.GateSelf, Place: "The Sanctuary", World: 5, X: 4322.19, Y: 273.62, Z: 6179.78})
	if !f.cast(spell) {
		t.Fatalf("Sanctuary did not begin: %q", redLines(f.sess))
	}
	if got := f.travellers(); len(got) != 1 || got[0] != "Wizard" {
		t.Fatalf("sent %q, want the caster alone", got)
	}
	if to := f.sent[0].To; to.World != 5 || to.X != 4322.19 || to.Y != 273.62 || to.Z != 6179.78 || to.Place != "The Sanctuary" {
		t.Errorf("sent to %+v, want the Sanctuary's landing", to)
	}
	k := newGateFixture(t)
	if k.cast(spell) || len(k.sent) != 0 {
		t.Errorf("the control: no row, and Sanctuary began (sent %q)", k.travellers())
	}
}
