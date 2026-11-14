package entity

import (
	"fmt"
	"log"
	"math"
	"sync/atomic"

	"github.com/eqoa/server/internal/elemental"
)

// elemental.go — the one owner of what a resist takes off elemental (FR, CR,
// LR, AR, PR, DR) spell and effect damage at the hit-point boundary: the
// research package of 2026-11-14 (EQOA-Elemental-Resistance-for-Dab, a GM
// collaborator's through the owner; its reference is internal/elemental),
// the model "elemental-2026-11-14":
//
//	a player takes  D0 x (350 - R) / 350, R its effective resist, 0..315
//	a creature takes D0 x (100 - R) / 100, R its template's, 0..100; 100 none
//
// combat.resist_model "legacy" keeps the model before it, for a rollback:
// resist.go's RollResist (jib's dice at the cap, a template's resist_fail
// anywhere) and enchanter.go's creatureStrongIn (at least 1 at 100).
//
// The adapter's decisions (the package leaves them to the server):
//
//   - R: a player's StatNow of the element (gear, masteries, WIS/7 once in
//     gear.go's wisResist, buffs and debuffs added uncapped), then clamped to
//     0..315; a creature's template percent (resist_kind/resist_pct, or the
//     kind's own "LR:75") less a debuff's points (npcResistKindPct, the 75%
//     line it always had), clamped to 0..100; none named: 0. A pet's or a
//     merc's resists are player figures (its sheet, its body), so it takes
//     the player's line.
//   - Mode: combat.resist_mode, magnitude_only unless set (every hit the
//     reduction); fixed_full_chance (combat.resist_full_chance of the hits
//     land whole) and resistance_scaled (whole at (350-R)/350) are there to
//     compare. A creature template's resist_fail (Lord Nagafen's 4, petition
//     1170) is its own rule in every mode: that percent of its hits on a
//     player land whole (fixed_full_chance at it); the old "twice as often in
//     part" is gone, the package having no partial outcome. A creature never
//     rolls: 100 is always nothing.
//   - Order: the reduction, then the fight's share (a duel's or a PvP
//     fight's DuelDamageShare, once), then one rounding, half up; then the
//     operator's formula and wards as before. A positive figure on a player
//     is never under 1 (a player is never immune); on a creature it is never
//     under 1 below 100, and at 100 it is 0, with no floor after.
//   - A damage over time rolls at each tick (its mode's draw, when it has one).
//   - Each elemental component of a spell is resolved by itself (its own
//     effect's resist: cast.go, duelfight.go).

// The resist models combat.resist_model names.
const (
	ResistModelElemental = "elemental-2026-11-14"
	ResistModelLegacy    = "legacy"
)

const (
	playerResistDivisor = 350.0
	playerResistMax     = 315
	npcResistMax        = 100
)

// ResistModel is the model in force: its version, and the player's mode.
type ResistModel struct {
	Version    string
	Mode       elemental.Mode
	FullChance float64 // fixed_full_chance's share of whole hits
}

// String is the model as the start's log line says it.
func (m ResistModel) String() string {
	switch {
	case m.Version == ResistModelLegacy:
		return m.Version
	case m.Mode == elemental.ModeFixedFullChance:
		return fmt.Sprintf("%s %s %.3g", m.Version, m.Mode, m.FullChance)
	}
	return fmt.Sprintf("%s %s", m.Version, m.Mode)
}

// ParseResistModel reads combat.resist_model, resist_mode and
// resist_full_chance: "" for each is the elemental model, magnitude_only.
func ParseResistModel(version, mode string, full float64) (ResistModel, error) {
	m := ResistModel{Version: version, Mode: elemental.Mode(mode), FullChance: full}
	if m.Version == "" {
		m.Version = ResistModelElemental
	}
	if m.Mode == "" {
		m.Mode = elemental.ModeMagnitudeOnly
	}
	switch m.Version {
	case ResistModelElemental, ResistModelLegacy:
	default:
		return ResistModel{}, fmt.Errorf("combat.resist_model %q: want %q or %q", version, ResistModelElemental, ResistModelLegacy)
	}
	switch m.Mode {
	case elemental.ModeMagnitudeOnly, elemental.ModeResistanceScaled:
	case elemental.ModeFixedFullChance:
		if math.IsNaN(full) || full < 0 || full > 1 {
			return ResistModel{}, fmt.Errorf("combat.resist_full_chance %v: want 0 to 1", full)
		}
	default:
		return ResistModel{}, fmt.Errorf("combat.resist_mode %q: want magnitude_only, fixed_full_chance or resistance_scaled", mode)
	}
	return m, nil
}

var resistModel atomic.Pointer[ResistModel]

func init() {
	resistModel.Store(&ResistModel{Version: ResistModelElemental, Mode: elemental.ModeMagnitudeOnly})
}

// SetResistModel puts a model in force (at the start; tests).
func SetResistModel(m ResistModel) { resistModel.Store(&m) }

// CurrentResistModel is the model in force.
func CurrentResistModel() ResistModel { return *resistModel.Load() }

func legacyResists() bool { return resistModel.Load().Version == ResistModelLegacy }

// elementStat is whether st is one of the six elements' resists.
func elementStat(st Stat) bool { return st >= StatFR && st <= StatAR }

// roundHalfUp is a positive figure rounded, a half up.
func roundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

// playerElem is one elemental component landing on a player (or a pet or a
// merc, on the player's line).
type playerElem struct {
	d0     int            // its figure before any resist
	resist int            // the recipient's resist of its element, as StatNow has it
	tmpl   *ClassTemplate // the creature's that dealt it (its resist_fail), nil for none
	// atCap is the share a resist at the cap takes in place of the line's 90%
	// (a boss's tick: ActiveEffect.npcAtCap), the same proportion below; 0: the line's
	atCap float64
	fight float64 // the fight's share (a duel's), 0 for none
	// pact and noFloor are the legacy model's: a pact's own scale, a call that
	// had no floor of 1
	pact, noFloor bool
}

// elemResult is what a component lands as.
type elemResult struct {
	Taken int           // what lands, rounded
	Real  float64       // before the rounding, after the fight's share
	Share float64       // the share of d0 the resist took off (0 on a whole hit)
	How   ResistOutcome // ResistFailed: it landed whole
}

// resolvePlayerElemental is the model's figure for one elemental component on
// a player.
func resolvePlayerElemental(in playerElem) elemResult {
	fight := in.fight
	if fight <= 0 {
		fight = 1
	}
	var share float64
	var how ResistOutcome
	m := resistModel.Load()
	if m.Version == ResistModelLegacy {
		if in.pact {
			share, how = RollPactResist(in.resist)
		} else {
			share, how = in.tmpl.rollResist(in.resist)
		}
		if in.atCap > 0 {
			share *= in.atCap / ResistCapShare
		}
	} else {
		share, how = playerElementalShare(m, in)
	}
	share = min(max(share, 0), 1)
	real := float64(max(in.d0, 0)) * (1 - share) * fight
	taken := roundHalfUp(real)
	if in.d0 > 0 && taken < 1 && (m.Version != ResistModelLegacy || !in.noFloor) {
		taken = 1
	}
	return elemResult{Taken: taken, Real: real, Share: share, How: how}
}

// playerElementalShare is the elemental model's share of a player's resist
// on one component: the package's reference (internal/elemental) on the
// clamped resist, the creature's resist_fail as a whole-hit chance of its own.
func playerElementalShare(m *ResistModel, in playerElem) (float64, ResistOutcome) {
	r := min(max(in.resist, 0), playerResistMax)
	if r == 0 || in.d0 <= 0 {
		return 0, ResistHeld
	}
	ei := elemental.Input{Target: elemental.Player, D0: float64(in.d0), Resistance: float64(r), Mode: m.Mode,
		FixedFullProbability: m.FullChance}
	if in.tmpl != nil && in.tmpl.ResistFail > 0 {
		ei.Mode, ei.FixedFullProbability = elemental.ModeFixedFullChance, min(in.tmpl.ResistFail/100, 1)
	}
	if ei.Mode != elemental.ModeMagnitudeOnly {
		roll := resistRand()
		ei.Roll = &roll
	}
	res, err := elemental.Calculate(ei)
	if err != nil {
		log.Printf("Resist: %v (D0 %d, R %d): taken whole", err, in.d0, r)
		return 0, ResistFailed
	}
	if res.Outcome == elemental.OutcomeFull {
		return 0, ResistFailed
	}
	share := 1 - res.ReducedDamage/ei.D0
	if in.atCap > 0 {
		share *= in.atCap / ResistCapShare
	}
	return share, ResistHeld
}

// playerResistShareNow is the share a player's resist takes off a figure
// that is no hit-point damage (a mana tap's power) as the model has it:
// resolvePlayerElemental's share.
func playerResistShareNow(resist int, tmpl *ClassTemplate) (float64, ResistOutcome) {
	r := resolvePlayerElemental(playerElem{d0: 1, resist: resist, tmpl: tmpl})
	return r.Share, r.How
}

// npcElementalTaken is the elemental model's figure of a creature's: n (a
// negative hit) on the 0..100 line at r, the creature's effective resist; at
// least -1 below 100, 0 at 100.
func npcElementalTaken(n, r int) int {
	if n >= 0 || r <= 0 {
		return n
	}
	r = min(r, npcResistMax)
	res, err := elemental.Calculate(elemental.Input{Target: elemental.NPC, D0: float64(-n), Resistance: float64(r)})
	if err != nil {
		log.Printf("Resist: %v (D0 %d, R %d): taken whole", err, -n, r)
		return n
	}
	taken := roundHalfUp(res.ReducedDamage)
	if r < npcResistMax {
		taken = max(taken, 1)
	}
	return -taken
}
