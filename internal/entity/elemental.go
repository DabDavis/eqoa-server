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
// research package of 2026-11-14 (a GM
// collaborator's through the owner; its reference is internal/elemental),
// the model "elemental-2026-11-14":
//
//	a player takes  D0 x (350 - R) / 350, R its effective resist, 0..315
//	a creature takes D0 x (100 - R) / 100, R its template's, 0..100; 100 none
//
// combat.resist_model "legacy" keeps the model before it, for a rollback:
// resist.go's RollResist (a GM's dice at the cap, a template's resist_fail
// anywhere) and enchanter.go's creatureHitAtResist (at least 1 at 100).
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
//     land whole). In both a creature template's resist_fail (Lord Nagafen's
//     4, petition 1170) is its own rule: that percent of its hits on a player
//     land whole (fixed_full_chance at it). resistance_scaled (work board
//     2027, a GM's decision of 2026-11-22) is the adapter's own dice: a
//     resist holds R/350 of the time (taking its magnitude_only share,
//     R/350), a template's resist_fail f making it 1 - s(1-f) that it fails;
//     on a failure with combat.resist_partial a second roll under s makes it
//     a partial (a share drawn evenly from 0 to s), else it lands whole. A
//     creature never rolls: 100 is always nothing.
//   - A player's resist below 0 (a debuff past its gear) adds R/350 to the
//     hit (ResistLowered), in every elemental mode, with no roll.
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
	// Partial is resistance_scaled's second roll on a failed resist
	// (combat.resist_partial): a partial at the resist's own chance
	Partial bool
}

// String is the model as the start's log line says it.
func (m ResistModel) String() string {
	switch {
	case m.Version == ResistModelLegacy:
		return m.Version
	case m.Mode == elemental.ModeFixedFullChance:
		return fmt.Sprintf("%s %s %.3g", m.Version, m.Mode, m.FullChance)
	case m.Partial:
		return fmt.Sprintf("%s %s partial", m.Version, m.Mode)
	}
	return fmt.Sprintf("%s %s", m.Version, m.Mode)
}

// ParseResistModel reads combat.resist_model, resist_mode,
// resist_full_chance and resist_partial: "" for each is the elemental model,
// magnitude_only; resist_partial only with resistance_scaled.
func ParseResistModel(version, mode string, full float64, partial bool) (ResistModel, error) {
	m := ResistModel{Version: version, Mode: elemental.Mode(mode), FullChance: full, Partial: partial}
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
	if partial && (m.Version != ResistModelElemental || m.Mode != elemental.ModeResistanceScaled) {
		return ResistModel{}, fmt.Errorf("combat.resist_partial: only with resist_mode %q (the elemental model)", elemental.ModeResistanceScaled)
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
	Taken int     // what lands, rounded
	Real  float64 // before the rounding, after the fight's share
	// Share is the share of d0 the resist took off: 0 on a whole hit, at most
	// 1; below 0 a lowered resist's added share (ResistLowered)
	Share float64
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
		share = max(share, 0)
	} else {
		share, how = playerElementalShare(m, in) // below 0 for a lowered resist
	}
	share = min(share, 1)
	real := float64(max(in.d0, 0)) * (1 - share) * fight
	taken := roundHalfUp(real)
	if in.d0 > 0 && taken < 1 && (m.Version != ResistModelLegacy || !in.noFloor) {
		taken = 1
	}
	return elemResult{Taken: taken, Real: real, Share: share, How: how}
}

// playerElementalShare is the elemental model's share of a player's resist
// on one component: the package's reference (internal/elemental) on the
// clamped resist, the creature's resist_fail as a whole-hit chance of its own;
// resistance_scaled's dice the adapter's (scaledResistShare); a resist below 0
// a negative share, no roll.
func playerElementalShare(m *ResistModel, in playerElem) (float64, ResistOutcome) {
	if in.resist < 0 && in.d0 > 0 {
		return float64(in.resist) / playerResistDivisor, ResistLowered
	}
	r := min(max(in.resist, 0), playerResistMax)
	if r == 0 || in.d0 <= 0 {
		return 0, ResistHeld
	}
	if m.Mode == elemental.ModeResistanceScaled {
		share, how := scaledResistShare(m, in, r)
		if in.atCap > 0 {
			share *= in.atCap / ResistCapShare
		}
		return share, how
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

// scaledResistShare is resistance_scaled's roll on a player's resist r (1 to
// 315; work board 2027): s = r/350, the magnitude_only reduction's share. It
// holds 1 - failChance of the time, failChance = 1 - s(1 - f), f the
// template's resist_fail percent (0 without one; at most 1). A failure with
// Partial on draws again: under s it is a partial, s x an even draw;
// otherwise, and always with Partial off, the hit lands whole. At R 315 with
// Partial: 90% of hits take 10%, 9% a partial, 1% whole (a mean of 14.95%).
func scaledResistShare(m *ResistModel, in playerElem, r int) (float64, ResistOutcome) {
	res, err := elemental.Calculate(elemental.Input{Target: elemental.Player, D0: float64(in.d0), Resistance: float64(r),
		Mode: elemental.ModeMagnitudeOnly})
	if err != nil {
		log.Printf("Resist: %v (D0 %d, R %d): taken whole", err, in.d0, r)
		return 0, ResistFailed
	}
	s := 1 - res.ReducedDamage/float64(in.d0)
	f := 0.0
	if in.tmpl != nil && in.tmpl.ResistFail > 0 {
		f = min(in.tmpl.ResistFail/100, 1)
	}
	if resistRand() >= 1-s*(1-f) {
		return s, ResistHeld
	}
	if m.Partial && resistRand() < s {
		return s * resistRand(), ResistPartial
	}
	return 0, ResistFailed
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
