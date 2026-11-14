package entity

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/elemental"
)

// withResistModel puts m in force until the returned func runs.
func withResistModel(m ResistModel) func() {
	old := CurrentResistModel()
	if m.Mode == "" {
		m.Mode = elemental.ModeMagnitudeOnly
	}
	SetResistModel(m)
	return func() { SetResistModel(old) }
}

// withDice makes every resist draw d until the returned func runs.
func withDice(d float64) func() {
	old := resistRand
	resistRand = func() float64 { return d }
	return func() { resistRand = old }
}

// The config's lines: unset is the elemental model, magnitude_only; legacy
// for a rollback; anything else refused.
func TestParseResistModel(t *testing.T) {
	m, err := ParseResistModel("", "", 0)
	if err != nil || m.Version != ResistModelElemental || m.Mode != elemental.ModeMagnitudeOnly || m.String() != "elemental-2026-11-14 magnitude_only" {
		t.Errorf("unset: %+v %v", m, err)
	}
	if m, err := ParseResistModel("legacy", "", 0); err != nil || m.String() != "legacy" {
		t.Errorf("legacy: %+v %v", m, err)
	}
	if m, err := ParseResistModel(ResistModelElemental, "fixed_full_chance", 0.1); err != nil || m.String() != "elemental-2026-11-14 fixed_full_chance 0.1" {
		t.Errorf("fixed: %+v %v", m, err)
	}
	for _, bad := range [][2]string{{"elemental", ""}, {"", "sometimes"}} {
		if _, err := ParseResistModel(bad[0], bad[1], 0); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := ParseResistModel("", "fixed_full_chance", 1.5); err == nil {
		t.Error("a chance of 1.5 accepted")
	}
	if got := CurrentResistModel(); got.Version != ResistModelElemental || got.Mode != elemental.ModeMagnitudeOnly {
		t.Errorf("the default in force: %+v", got)
	}
}

// templateHitOn lands a creature's single-element nuke of d0 on c's player
// at its resist r of fire, by the creature's template tm (nil: none), and
// returns what it took and the lines it was told.
func templateHitOn(t *testing.T, d0, r int, tm *ClassTemplate) (int, string) {
	t.Helper()
	c := newCaster(t)
	c.p.HP, c.p.MaxHP = 100000, 100000
	c.p.FireR = r
	npc := &NPC{ServerID: 2003, Name: "a fire imp", Level: 30, HP: 500, MaxHP: 500, X: 101, Y: 10, Z: 100, tmpl: tm}
	c.w.NPCs[npc.ServerID] = npc
	cast := &npcCast{npc: npc, tmpl: tm, spell: &CastSpell{Name: "Flame Bolt", resist: StatFR}, pct: 100}
	before := c.p.HP
	c.w.mu.Lock()
	c.w.templateHitLocked(cast, c.p.ServerID, []SpellEffect{{Kind: EffectHP, Amount: -d0, AmountMax: -d0}}, c.at(time.Second))
	c.w.mu.Unlock()
	return before - c.p.HP, strings.Join(linesOf(drain(c.sess)), "|")
}

// The package's acceptance cases on a player, through a creature's spell
// (templateHitLocked): R315 680 -> 68 and 1000 -> 100, R245 -> 300, R0 the
// whole; past the cap as at it, a negative as none. magnitude_only rolls no
// dice.
func TestElementalAcceptancePlayer(t *testing.T) {
	defer withResistModel(ResistModel{Version: ResistModelElemental})()
	defer func(f func() float64) { resistRand = f }(resistRand)
	resistRand = func() float64 { t.Fatal("magnitude_only threw the dice"); return 0 }
	for _, tt := range []struct{ d0, r, want int }{
		{680, 315, 68}, {1000, 315, 100}, {1000, 245, 300}, {1000, 0, 1000}, {1000, 400, 100}, {1000, -50, 1000},
		{1000, 175, 500}, {3, 175, 2}, {7, 315, 1}, {1, 315, 1},
	} {
		if got, lines := templateHitOn(t, tt.d0, tt.r, nil); got != tt.want {
			t.Errorf("D0 %d at R %d: took %d, want %d (%q)", tt.d0, tt.r, got, tt.want, lines)
		}
	}
	if _, lines := templateHitOn(t, 1000, 315, nil); !strings.Contains(lines, "hits you for 100 (your resist took 90%)") {
		t.Errorf("the line: %q", lines)
	}
}

// The stochastic modes, explicitly selected: a draw under the whole-hit
// chance lands the whole, at it or over the reduction.
func TestElementalStochasticModes(t *testing.T) {
	defer withResistModel(ResistModel{Version: ResistModelElemental, Mode: elemental.ModeFixedFullChance, FullChance: 0.1})()
	func() {
		defer withDice(math.Nextafter(0.1, 0))()
		if got, lines := templateHitOn(t, 1000, 315, nil); got != 1000 || !strings.Contains(lines, "your resist failed") {
			t.Errorf("fixed 0.10, a draw under: %d %q", got, lines)
		}
	}()
	func() {
		defer withDice(0.1)()
		if got, _ := templateHitOn(t, 1000, 315, nil); got != 100 {
			t.Errorf("fixed 0.10, a draw at it: %d", got)
		}
	}()
	SetResistModel(ResistModel{Version: ResistModelElemental, Mode: elemental.ModeResistanceScaled})
	func() {
		defer withDice(0.9)() // whole under (350-100)/350 = 0.714
		if got, _ := templateHitOn(t, 1000, 100, nil); got != 714 {
			t.Errorf("resistance_scaled R100, reduced: %d, want 714", got)
		}
	}()
}

// A template's resist_fail (Lord Nagafen's 4) is a whole-hit chance of its
// own in every mode; the legacy model keeps its partials.
func TestElementalTemplateResistFail(t *testing.T) {
	defer withResistModel(ResistModel{Version: ResistModelElemental})()
	naga := &ClassTemplate{}
	naga.ResistFail = 4
	func() {
		defer withDice(0.039)()
		if got, _ := templateHitOn(t, 1000, 315, naga); got != 1000 {
			t.Errorf("under 4%%: %d", got)
		}
	}()
	func() {
		defer withDice(0.05)() // the legacy model's partial band: held here
		if got, _ := templateHitOn(t, 1000, 315, naga); got != 100 {
			t.Errorf("at 5%%: %d", got)
		}
	}()
	defer withResistModel(ResistModel{Version: ResistModelLegacy})()
	func() {
		dice := []float64{0.05, 0}
		old := resistRand
		defer func() { resistRand = old }()
		resistRand = func() float64 { d := dice[0]; dice = dice[1:]; return d }
		// legacy: 0.05 < 3 x 4%: a partial, the low end 0.6 of the 90% share (54%): 460
		if got, _ := templateHitOn(t, 1000, 315, naga); got != 460 {
			t.Errorf("legacy partial: %d, want 460", got)
		}
	}()
}

// A creature's elemental damage on the 0..100 line (creatureResists): R50
// 500, R99 10, R100 nothing, with the player's stochastic modes on too; no
// template or no resist named, the whole; odd figures half up, at least 1
// below 100. The legacy model: at least 1 at 100.
func TestElementalAcceptanceNPC(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, m := range []ResistModel{
		{Version: ResistModelElemental},
		{Version: ResistModelElemental, Mode: elemental.ModeFixedFullChance, FullChance: 1},
		{Version: ResistModelElemental, Mode: elemental.ModeResistanceScaled},
	} {
		func() {
			defer withResistModel(m)()
			defer withDice(0)()
			for _, tt := range []struct{ d0, r, want int }{
				{1000, 50, 500}, {1000, 99, 10}, {1000, 100, 0}, {7, 100, 0}, {3, 50, 2}, {1, 99, 1}, {1000, 0, 1000},
			} {
				tm := &ClassTemplate{}
				tm.ResistKind, tm.ResistPct = "FR", tt.r
				npc := &NPC{ServerID: 9, Level: 54, tmpl: tm}
				if got := -creatureResists(StatFR, npc, -tt.d0, now); got != tt.want {
					t.Errorf("%s: D0 %d at R %d: %d, want %d", m, tt.d0, tt.r, got, tt.want)
				}
			}
		}()
	}
	// immune is immune through taken_resist ("LR:115") too
	tm := &ClassTemplate{}
	tm.ResistKind, tm.ResistPct, tm.TakenResist = "LR", 100, "LR:115"
	if got := creatureResists(StatLR, &NPC{tmpl: tm}, -1000, now); got != 0 {
		t.Errorf("immune with taken_resist: %d", got)
	}
	// a debuff's points come off first (the 75% line: 105 FR off is 25 points), then the line
	tm = &ClassTemplate{}
	tm.ResistKind, tm.ResistPct = "FR", 100
	npc := &NPC{tmpl: tm, Effects: []*ActiveEffect{{SpellID: 1, Until: now.Add(time.Minute), Mods: StatMods{FR: -105}}}}
	if got := creatureResists(StatFR, npc, -1000, now); got != -250 {
		t.Errorf("R100 less a 105 FR debuff: %d, want -250", got)
	}
	defer withResistModel(ResistModel{Version: ResistModelLegacy})()
	tm.ResistPct = 100
	if got := creatureResists(StatFR, &NPC{tmpl: tm}, -1000, now); got != -1 {
		t.Errorf("legacy at 100: %d, want -1", got)
	}
}

// An immune creature takes nothing of a player's spell or its ticks through
// the cast's own entry (landOnNPCLocked, tickNPCEffectLocked), and the cold
// component of the same caster's other spell lands whole: each element by
// itself.
func TestElementalImmuneCreatureThroughACast(t *testing.T) {
	defer withResistModel(ResistModel{Version: ResistModelElemental, Mode: elemental.ModeFixedFullChance, FullChance: 1})()
	r, _, _ := charmRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = func(int) int { return 99 }
	crab := r.enemy
	crab.HP, crab.MaxHP = 5000000, 5000000
	spell := func(id int, kind, resist string, duration int) CastSpell {
		return wbSpell(id, 20, 1, "Test "+kind, fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":%d,"stack":0,"level":20,"min_level":1,"req_equipment":"Any",
			"effects":[{"id":"0001","type":"Dmg/Heal","kind":"%s","amount":-10000,"resist":"%s"}]}`, duration, kind, resist))
	}
	took := func(s CastSpell, ticks bool) int {
		before := crab.HP
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
		if ticks {
			for _, e := range crab.Effects {
				if e.SpellID == s.SpellID {
					e.NextTick = r.at(2 * time.Second)
					r.w.tickNPCEffectLocked(crab, e, r.at(2*time.Second))
					e.Until = r.at(0)
				}
			}
		}
		return before - crab.HP
	}
	flame, frost, burn := spell(9311, "hp", "fr", 0), spell(9312, "hp", "cr", 0), spell(9313, "hp_tick", "fr", 60)
	frostWhole := took(frost, false)
	tm := &ClassTemplate{}
	tm.ResistKind, tm.ResistPct = "FR", 100
	crab.tmpl = tm
	if got := took(flame, false); got != 0 {
		t.Errorf("a fire nuke on a creature immune to fire: %d", got)
	}
	if got := took(burn, true); got != 0 {
		t.Errorf("a fire tick on it: %d", got)
	}
	if got := took(frost, false); got != frostWhole || got <= 0 {
		t.Errorf("a cold nuke on it: %d, want %d", got, frostWhole)
	}
}

// A duel's elemental spell: the opponent's resist, then the duel's half,
// once: R315 1000 -> 50 (the package's case), R0 1000 -> 500; an odd
// figure rounds half up after the half (R0 3 -> 2). The legacy model rolled
// no resist in a duel.
func TestElementalDuelOnce(t *testing.T) {
	defer withResistModel(ResistModel{Version: ResistModelElemental})()
	nuke := func(id, n int) CastSpell {
		sp := strike()
		sp.SpellID = id
		sp.Def = ParseSpellText(fmt.Sprintf("Target, Decrease Hitpoints by %d", n))
		for i := range sp.Def.Effects {
			if sp.Def.Effects[i].Kind == EffectHP {
				sp.Def.Effects[i].Resist = StatFR
			}
		}
		return sp
	}
	for _, tt := range []struct {
		legacy     bool
		d0, r, out int
	}{{false, 1000, 315, 50}, {false, 1000, 0, 500}, {false, 3, 0, 2}, {false, 5, 175, 1}, {true, 1000, 315, 500}} {
		func() {
			if tt.legacy {
				defer withResistModel(ResistModel{Version: ResistModelLegacy})()
			}
			d := newDuelists(t)
			d.start(t)
			d.a.FireR = tt.r
			d.a.HP, d.a.MaxHP = 5000, 5000
			sp := nuke(651, tt.d0)
			if !d.w.StartCast(d.sb, sp, d.bSeesA, d.at(2*time.Second)) {
				t.Fatalf("refused: %q", duelLines(duelDrain(d.sb)))
			}
			before := d.a.HP
			d.w.mu.Lock()
			d.w.castSystem(d.at(2*time.Second + sp.CastTime + castLand))
			d.w.mu.Unlock()
			if got := before - d.a.HP; got != tt.out {
				t.Errorf("legacy %v, D0 %d at R %d: took %d, want %d", tt.legacy, tt.d0, tt.r, got, tt.out)
			}
		}()
	}
}

// A pact's price on the player's line moves nothing against petition 1532's
// scale: 680 at 315 AR costs 68, 1640 at 117 AR 1092, 50 at 210 20, in
// either model (the legacy one held below the cap).
func TestElementalPactUnchanged(t *testing.T) {
	for _, m := range []ResistModel{{Version: ResistModelElemental}, {Version: ResistModelLegacy}} {
		func() {
			defer withResistModel(m)()
			defer withDice(0.5)() // legacy at the cap: held
			for _, tt := range []struct{ n, ar, want int }{{680, 315, 68}, {1640, 117, 1092}, {50, 210, 20}, {50, 0, 50}} {
				if got := resolvePlayerElemental(playerElem{d0: tt.n, resist: tt.ar, pact: true, noFloor: true}).Taken; got != tt.want {
					t.Errorf("%s: %d at %d AR: %d, want %d", m, tt.n, tt.ar, got, tt.want)
				}
				if got := int(math.Round(float64(tt.n) * (1 - PactResistShare(tt.ar)))); got != tt.want {
					t.Errorf("the old pact scale moved: %d at %d AR: %d", tt.n, tt.ar, got)
				}
			}
		}()
	}
	// through the cast: Gloom Pact's 50 at 315 AR costs 5
	defer withResistModel(ResistModel{Version: ResistModelElemental})()
	c, _, fo := pactRig(t)
	c.p.ArcaneR = 315
	c.landSeen(t, gloomPact(), fo)
	if c.p.HP != 495 {
		t.Errorf("Gloom Pact at 315 AR: caster %d HP, want 495", c.p.HP)
	}
}

// A charm's, a proc's and a shield's chances are no damage: the arcane debuff's
// points off a creature's chance stay on their own 75% line in either model.
func TestElementalChancesUnchanged(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	npc := &NPC{Effects: []*ActiveEffect{{SpellID: 1, Until: now.Add(time.Minute), Mods: StatMods{AR: -25}}}}
	for _, m := range []ResistModel{{Version: ResistModelElemental}, {Version: ResistModelLegacy}} {
		func() {
			defer withResistModel(m)()
			if got := npcResistOffPct(npc, now); got != 6 {
				t.Errorf("%s: 25 AR off a creature's chance: %d points, want 6", m, got)
			}
			if got := ResistShare(315); math.Abs(got-0.9) > 1e-12 {
				t.Errorf("%s: ResistShare(315) %v", m, got)
			}
		}()
	}
}
