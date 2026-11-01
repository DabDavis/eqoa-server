package entity

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// spelljson.go — what a spell does, read from EQOAEmu's spell workbook
// (2026-11-01, the user's "workbook wins, re-fit": where the workbook has a
// value it is what the spell does).
//
// A tool of the import fills spelleffect.effects with a spell's effects as
// JSON, version 1 (the import's shared brief: one object a spell, its scope,
// target type, harm, duration, stack type and level, the lowest level that
// may receive it, its area's radius, and one entry an effect with the
// workbook's EffectID, TYPE and a kind). When it is not empty it is the
// spell's SpellDef (SpellDefFrom); the line stays for people to read and is
// read only when effects is empty, so every spell the workbook does not
// reach is as it was.
//
// The kinds map onto the line reader's EffectKinds (spelltext.go) and its
// fields; what the workbook says that the lines never did is read here too:
//
//   - resist: a harmful effect may be resisted by its target, by the
//     resist the workbook names (workbook.go);
//   - level_pct: the caster's level times level_pct/100 more (spellAmount);
//   - stack and level: the stack type the best-of-a-line rule goes by
//     (buffline.go);
//   - hp2: the second number, for a target-based area spell its area's (the
//     target takes it too: the workbook's legend, "AoE hits target and also
//     the radius from the target, including target a second time"), else a
//     second hit on the target;
//   - hp_tick and power_tick tick twice at the landing (the legend: "HoT and
//     PoT tick twice on first effect and every 6seconds after"), then as the
//     server's ticks do;
//   - dispel_on_cast, dispel_on_contact, cure, mitigation, invulnerable,
//     lull, breath, hide and the lowest level that may receive it
//     (workbook.go);
//   - what the engine has no rule for (melee resist debuffs, off and def
//     mods, the hp factor, pickpocket, faction changes, calling a player) is
//     kept in SpellDef.Unsupported and logged, never acted on.
//
// A malformed object is logged once and the line read instead.

// SpellDefFrom is a spell's SpellDef: its workbook effects (effects, the
// spelleffect column, JSON) when it has them, else its line as
// ParseSpellNamed reads it. Every place that builds what a spell does goes
// through here.
func SpellDefFrom(name, line, desc, effects string) SpellDef {
	if strings.TrimSpace(effects) != "" {
		d, err := ParseSpellJSON(name, desc, effects)
		if err == nil {
			return d
		}
		if _, seen := badSpellJSON.LoadOrStore(name, true); !seen {
			log.Printf("Spell: %q's effects (JSON) are not read, its line is: %v", name, err)
		}
	}
	return ParseSpellNamed(name, line, desc)
}

// badSpellJSON is the spells whose effects were logged as unread, once each.
var badSpellJSON sync.Map

// spellJSON is a spell's effects object, version 1 (the file's doc).
type spellJSON struct {
	V          int          `json:"v"`
	Scope      string       `json:"scope"`
	TargetType string       `json:"target_type"`
	Harmful    *bool        `json:"harmful"`
	DurationS  float64      `json:"duration_s"`
	Stack      int          `json:"stack"`
	Level      int          `json:"level"`
	MinLevel   int          `json:"min_level"`
	Radius     float64      `json:"radius"`
	ReqEquip   string       `json:"req_equipment"`
	Effects    []effectJSON `json:"effects"`
}

// effectJSON is one effect of it; a proc's own effects ("procs") are the
// same, each with "on" offense or defense.
type effectJSON struct {
	ID              string        `json:"id"`
	Type            string        `json:"type"`
	Kind            string        `json:"kind"`
	Stat            string        `json:"stat"`
	Amount          float64       `json:"amount"`
	Scale           string        `json:"scale"`
	ScalePct        float64       `json:"scale_pct"`
	LevelPct        float64       `json:"level_pct"`
	Resist          string        `json:"resist"`
	Hate            *float64      `json:"hate"`
	DurationS       float64       `json:"duration_s"`
	Scope           string        `json:"scope"`
	DispelOnCast    bool          `json:"dispel_on_cast"`
	DispelOnContact bool          `json:"dispel_on_contact"`
	Pet             *petJSON      `json:"pet"`
	Item            *itemJSON     `json:"item"`
	Consume         *itemJSON     `json:"consume"`
	Sprite          string        `json:"sprite"`
	Location        string        `json:"location"`
	Removes         string        `json:"removes"`
	RezPct          float64       `json:"rez_pct"`
	Mitigation      *mitigateJSON `json:"mitigation"`
	Procs           []effectJSON  `json:"procs"`
	On              string        `json:"on"`
}

type petJSON struct {
	SheetPetID string `json:"sheet_pet_id"`
	Name       string `json:"name"`
}

type itemJSON struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type mitigateJSON struct {
	Pct float64 `json:"pct"`
	Max float64 `json:"max"`
}

// SpellItem is an item a spell makes or takes, by name.
type SpellItem struct {
	Name  string
	Count int
}

// jsonStats are the stats a workbook effect names, as the brief spells them.
var jsonStats = map[string]Stat{
	"str": StatStr, "sta": StatSta, "agi": StatAgi, "dex": StatDex, "wis": StatWis, "int": StatInt, "cha": StatCha,
	"ac": StatAC, "maxhp": StatMaxHP, "maxpower": StatMaxPower, "hot": StatHoT, "pot": StatPoT,
	"fr": StatFR, "cr": StatCR, "lr": StatLR, "pr": StatPR, "dr": StatDR, "ar": StatAR,
	"pierce": StatPierce, "blunt": StatBlunt, "slash": StatSlash, "ranged": StatRanged,
}

// jsonStat is a stat by its name in the workbook's words, any case;
// StatNone and false for none.
func jsonStat(s string) (Stat, bool) {
	st, ok := jsonStats[strings.ToLower(strings.TrimSpace(s))]
	return st, ok
}

// invisibleSprite is the invisible body (either byte order): an illusion of
// it is invisibility, as the workbook's invisibilities "alter appearance" to
// it.
func invisibleSprite(v uint32) bool { return v == 0x435EF663 || v == 0x63F65E43 }

// ParseSpellJSON reads a spell's workbook effects (the file's doc). name and
// desc are its record's, which say what an illusion is.
func ParseSpellJSON(name, desc, text string) (SpellDef, error) {
	var s spellJSON
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		return SpellDef{}, err
	}
	if s.V != 1 {
		return SpellDef{}, fmt.Errorf("version %d, not 1", s.V)
	}
	if len(s.Effects) == 0 {
		return SpellDef{}, fmt.Errorf("no effects")
	}
	d := SpellDef{Workbook: true, Stack: s.Stack, StackLevel: s.Level, MinLevel: s.MinLevel}
	scope := strings.ToLower(strings.TrimSpace(s.Scope))
	tt := strings.ToLower(s.TargetType)
	switch scope {
	case "self", "player_aoe":
		d.Target = TargetSelf
	case "group":
		d.Target = TargetGroup
	case "pet":
		d.Target = TargetPet
	default: // target, target_aoe, owner
		d.Target = TargetSingle
	}
	if strings.Contains(tt, "undead") {
		d.UndeadOnly = true
		if d.Target == TargetSingle {
			d.Target = TargetUndead
		}
	}
	for _, kind := range []string{"animal", "summoned"} {
		if strings.Contains(tt, kind) {
			d.unsupported("target type %q (any creature takes it)", s.TargetType)
		}
	}
	if s.ReqEquip != "" && !strings.EqualFold(s.ReqEquip, "any") {
		d.unsupported("required equipment %q", s.ReqEquip)
	}
	d.Duration = seconds(s.DurationS)
	var area []SpellEffect
	targetOnly := map[int]bool{} // a target-based area spell's effects for its target alone
	for _, e := range s.Effects {
		if d.Duration <= 0 && e.DurationS > 0 {
			d.Duration = seconds(e.DurationS)
		}
		n := len(d.Effects)
		d.readEffectJSON(name, desc, e, scope == "target_aoe", &area)
		switch es := strings.ToLower(strings.TrimSpace(e.Scope)); {
		case es == "self" && scope != "self" && scope != "player_aoe":
			// an effect of the spell's on its caster (a price, a tap's own
			// share, a taunt's AC): lifetap.go lands it, once a cast
			for i := n; i < len(d.Effects); i++ {
				if k := d.Effects[i].Kind; k == EffectHP || k == EffectPower || k == EffectStat {
					d.Effects[i].Self = true
				}
			}
		case es == "target" && scope == "target_aoe":
			for i := n; i < len(d.Effects); i++ {
				targetOnly[i] = true
			}
		case es != "" && es != scope && es != "self" && es != "target":
			d.unsupported("an effect for %q in a spell for %q (it lands as the spell's)", es, scope)
		}
	}
	switch {
	case s.Harmful != nil:
		d.Harmful = *s.Harmful
	default:
		d.Harmful = strings.Contains(tt, "enemy")
	}
	if d.Target == TargetUndead {
		d.Harmful = true
	}
	// a pet, an illusion and a charm are the whole spell (spelltext.go's
	// summonLine, illusion.go, charm.go)
	if d.Summons() {
		d.Target, d.Harmful = TargetSelf, false
	}
	if d.FormKind != FormNone {
		d.Harmful = false
	}
	r := float32(s.Radius)
	switch {
	case scope == "target_aoe":
		a := SpellArea{Centre: AreaOnTarget, Radius: r, Why: "the workbook's target-based area"}
		if len(area) > 0 {
			// the area's own number, and what the spell does beside its hits
			// and its hate (a debuff, a tick)
			for i, e := range d.Effects {
				if e.Kind != EffectHP && e.Kind != EffectHate && !e.Self && !targetOnly[i] {
					area = append(area, e)
				}
			}
			a.Effects = area
		}
		d.Area = &a
	case scope == "player_aoe":
		d.Area = &SpellArea{Centre: AreaOnCaster, Radius: r, Why: "the workbook's player-based area"}
	case scope == "group" && r > 0:
		d.Area = &SpellArea{Centre: AreaOnCaster, Radius: r, Why: fmt.Sprintf("the workbook's group radius %g", s.Radius)}
	default:
		d.Area = &SpellArea{} // the workbook gives none: no line words make one
	}
	if d.Charms() || d.Summons() || d.FormKind != FormNone {
		d.Area = &SpellArea{}
	}
	d.lineDefaults() // spellnotes.go: a stat buff with no time lasts 30 minutes
	if len(d.Unsupported) > 0 {
		if _, seen := unsupportedLogged.LoadOrStore(name, true); !seen {
			log.Printf("Spell: %q's workbook effects the engine has no rule for: %s", name, strings.Join(d.Unsupported, "; "))
		}
	}
	return d, nil
}

// unsupportedLogged is the spells whose unsupported effects were logged.
var unsupportedLogged sync.Map

// unsupported keeps what the engine does not act on, for the log.
func (d *SpellDef) unsupported(format string, args ...any) {
	d.Unsupported = append(d.Unsupported, fmt.Sprintf(format, args...))
}

// seconds is a workbook duration as a time.
func seconds(s float64) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// round is a workbook number as an int.
func round(v float64) int { return int(math.Round(v)) }

// effectOf is a workbook effect's figures as a SpellEffect of kind k: its
// amount, its scale, its level scaling and its resist.
func effectOf(k EffectKind, e effectJSON) SpellEffect {
	n := round(e.Amount)
	out := SpellEffect{Kind: k, Amount: n, AmountMax: n, ScalePct: round(e.ScalePct), LevelPct: round(e.LevelPct), ID: e.ID}
	if out.ScalePct != 0 {
		out.Scale, _ = jsonStat(e.Scale)
		if out.Scale > StatCha {
			out.Scale, out.ScalePct = StatNone, 0 // a scale goes by STR to CHA alone
		}
	}
	if st, ok := jsonStat(e.Resist); ok {
		out.Resist = st
	}
	return out
}

// readEffectJSON adds one workbook effect to d; area collects a
// target-based area's own numbers (hp2) when aoe.
func (d *SpellDef) readEffectJSON(name, desc string, e effectJSON, aoe bool, area *[]SpellEffect) {
	kind := strings.ToLower(strings.TrimSpace(e.Kind))
	if e.DispelOnCast {
		d.DispelOnCast = true
	}
	if e.DispelOnContact {
		d.DispelOnContact = true
	}
	// a hate beside another kind's figures (a nuke's "Hate")
	if e.Hate != nil && *e.Hate != 0 && kind != "hate" {
		h := effectOf(EffectHate, e)
		h.Amount, h.AmountMax, h.Resist = round(*e.Hate), round(*e.Hate), StatNone
		d.Effects = append(d.Effects, h)
	}
	add := func(se SpellEffect) { d.Effects = append(d.Effects, se) }
	switch kind {
	case "hp", "backstab":
		if e.Amount != 0 {
			add(effectOf(EffectHP, e))
		}
		if kind == "backstab" {
			d.Ignored = append(d.Ignored, "backstab: from anywhere (no facing is tested)")
		}
	case "hp2":
		if e.Amount == 0 {
			return
		}
		hit := effectOf(EffectHP, e)
		add(hit) // the target a second time (the file's doc)
		if aoe {
			*area = append(*area, hit)
		}
	case "hp_tick":
		if e.Amount != 0 {
			add(effectOf(EffectHPTick, e))
		}
	case "power":
		if e.Amount != 0 {
			add(effectOf(EffectPower, e))
		}
	case "power_tick":
		if e.Amount != 0 {
			add(effectOf(EffectPowerTick, e))
		}
	case "stat", "stat_set":
		st, ok := jsonStat(e.Stat)
		switch {
		case strings.EqualFold(strings.TrimSpace(e.Stat), "speed"):
			d.readSpeedJSON(e)
		case !ok:
			d.unsupported("%s of %q by %g", kind, e.Stat, e.Amount)
		case st >= StatPierce && st <= StatRanged:
			d.unsupported("%s resist debuff by %g", e.Stat, e.Amount)
		case kind == "stat_set" && st == StatMaxPower:
			n := max(round(e.Amount), 0)
			add(SpellEffect{Kind: EffectPowerCap, Amount: n, AmountMax: n, ID: e.ID})
		case kind == "stat_set" && (st == StatMaxHP || st > StatCha):
			d.unsupported("%s set to %g", e.Stat, e.Amount)
		case kind == "stat_set":
			se := effectOf(EffectStat, e)
			se.Stat, se.Set = st, true
			add(se)
		case e.Amount != 0:
			se := effectOf(EffectStat, e)
			se.Stat = st
			add(se)
		}
	case "speed":
		d.readSpeedJSON(e)
	case "hate":
		h := effectOf(EffectHate, e)
		if e.Hate != nil {
			h.Amount, h.AmountMax = round(*e.Hate), round(*e.Hate)
		}
		if h.Amount != 0 {
			add(h)
		}
	case "summon":
		add(SpellEffect{Kind: EffectSummon, ID: e.ID})
		if e.Pet != nil {
			d.Pet = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(e.Pet.Name), "Summon ")))
			d.SheetPetID = e.Pet.SheetPetID
		}
	case "illusion":
		v, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e.Sprite)), "0x"), 16, 32)
		if invisibleSprite(uint32(v)) {
			add(SpellEffect{Kind: EffectInvis, ID: e.ID})
			return
		}
		d.readIllusion(name, desc)
		if d.FormKind == FormEnd {
			return // "Remove Illusion", "True Form": the bearer's own body back
		}
		if d.FormKind == FormNone {
			// a body by its sprite alone, on whom the spell lands
			d.FormKind = FormSelf
			if !strings.EqualFold(strings.TrimSpace(e.Scope), "self") && d.Target != TargetSelf {
				d.FormKind = FormGroup
			}
			d.Target = TargetSelf
			d.Effects = append(d.Effects, SpellEffect{Kind: EffectIllusion, ID: e.ID})
		}
		d.FormModel = uint32(v)
		if d.Form == "" {
			d.Form = strings.ToLower(strings.TrimSpace(e.Sprite))
		}
		if d.Duration <= 0 {
			d.Duration = illusionDuration
		}
	case "create_item":
		if e.Item != nil && strings.TrimSpace(e.Item.Name) != "" {
			d.Items = append(d.Items, SpellItem{Name: strings.TrimSpace(e.Item.Name), Count: max(e.Item.Count, 1)})
		} else {
			d.unsupported("an item made with no name")
		}
	case "drain", "drain_group":
		to := DrainCaster
		if kind == "drain_group" {
			to = DrainGroup
		}
		var se SpellEffect
		switch st, ok := jsonStat(e.Stat); {
		case e.Stat == "" || strings.EqualFold(e.Stat, "hp"):
			se = effectOf(EffectHP, e)
		case strings.EqualFold(e.Stat, "power") || strings.EqualFold(e.Stat, "pow"):
			se = effectOf(EffectPower, e)
		case ok && st <= StatAR:
			se = effectOf(EffectStat, e)
			se.Stat = st
		default:
			d.unsupported("a drain of %q", e.Stat)
			return
		}
		if se.Amount > 0 {
			se.Amount, se.AmountMax = -se.Amount, -se.AmountMax // a drain takes
		}
		if se.Amount != 0 {
			se.Drain = to
			add(se)
		}
	case "proc", "def_proc":
		d.readProcsJSON(kind, e)
	case "gate":
		add(SpellEffect{Kind: EffectGate, ID: e.ID})
	case "teleport":
		d.Location = strings.TrimSpace(e.Location)
		// read as a teleport (gates.go): a spellgates row by its name is
		// where it lands; with none it is refused, its place logged
		d.Unread = append(d.Unread, "teleport to "+d.Location)
	case "cure":
		add(SpellEffect{Kind: EffectCure, ID: e.ID})
		d.Removes = append(d.Removes, strings.TrimSpace(e.Removes))
	case "charm":
		add(effectOf(EffectCharm, e))
	case "invis":
		add(SpellEffect{Kind: EffectInvis, ID: e.ID})
	case "hide":
		add(SpellEffect{Kind: EffectHide, ID: e.ID})
	case "resurrect":
		n := round(math.Abs(e.RezPct))
		if n == 0 {
			n = round(math.Abs(e.Amount))
		}
		add(SpellEffect{Kind: EffectResurrect, Amount: n, AmountMax: n, ID: e.ID})
	case "invulnerable", "mitigation":
		pct, most := 0, 0
		if e.Mitigation != nil {
			pct, most = round(e.Mitigation.Pct), round(math.Abs(e.Mitigation.Max))
		}
		if pct <= 0 && kind == "invulnerable" {
			pct = 100
		}
		if most >= mitigateNoLimit {
			most = 0
		}
		if pct > 0 {
			add(SpellEffect{Kind: EffectMitigate, Amount: min(pct, 100), AmountMax: most, ID: e.ID})
		}
	case "lull":
		se := effectOf(EffectLull, e)
		se.Amount, se.AmountMax = 0, 0
		add(se)
	case "breath":
		add(SpellEffect{Kind: EffectBreath, ID: e.ID})
	case "faction":
		d.unsupported("a faction change")
	case "pickpocket":
		d.unsupported("pickpocket")
	case "melee_debuff", "off_mod", "def_mod", "hp_factor":
		d.unsupported("%s %s by %g", kind, e.Stat, e.Amount)
	default:
		d.unsupported("kind %q (%s)", e.Kind, e.Type)
	}
	if e.Consume != nil && strings.TrimSpace(e.Consume.Name) != "" {
		d.Reagent, d.ReagentCount = strings.TrimSpace(e.Consume.Name), max(e.Consume.Count, 1)
	}
}

// mitigateNoLimit is a mitigation's most that is no limit: no hit adds up to
// it in an effect's time (the workbook writes such a most for a mitigation
// meant to last the whole effect).
const mitigateNoLimit = 100000

// readSpeedJSON is a speed effect: faster (the movement buff's, cast.go's
// speedBuff: the workbook writes it as a percent of the run) or slower by a
// percent (a snare on a creature, on a player a slow of its own beside the
// buff that carries it: workbook.go).
func (d *SpellDef) readSpeedJSON(e effectJSON) {
	n := round(e.Amount)
	switch {
	case n > 0 || strings.EqualFold(strings.TrimSpace(e.Kind), "speed") && e.Amount == 0:
		// a speed with no figure is the movement buff too
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectSpeed, Amount: n, AmountMax: n, ID: e.ID})
	case n < 0:
		se := effectOf(EffectSnare, e)
		se.Amount, se.AmountMax = max(n, -100), max(n, -100)
		d.Effects = append(d.Effects, se)
	}
}

// readProcsJSON is a proc buff: each of its procs' effects that hits is the
// line reader's proc (an offensive proc's, proc.go) or defensive proc
// (dshield.go: the workbook's defensive procs answer every hit, its legend's
// "Defensive procs have 100% proc rate on hit"), a heal a heal of its bearer
// at the proc (workbook.go); the rest is kept as unsupported.
func (d *SpellDef) readProcsJSON(kind string, e effectJSON) {
	if len(e.Procs) == 0 {
		d.unsupported("a %s with no proc effects", kind)
		return
	}
	for _, p := range e.Procs {
		on := strings.ToLower(strings.TrimSpace(p.On))
		def := on == "defense" || on == "defence" || (on == "" && kind == "def_proc")
		pk := strings.ToLower(strings.TrimSpace(p.Kind))
		n := round(p.Amount)
		switch {
		case (pk == "hp" || pk == "hp2") && n < 0:
			se := effectOf(EffectProc, p)
			se.Amount, se.AmountMax = -n, -n
			se.ScaleBearer = se.Scale != StatNone // the proc effect's own modifier: its bearer's (the legend)
			if def {
				se.Kind, se.Chance = EffectDefProc, 100
				// a defensive proc's damage is worked out at its landing
				// (Shield.with), its scale its caster's
				se.ScaleBearer = false
			}
			d.Effects = append(d.Effects, se)
		case pk == "hp" && n > 0:
			se := effectOf(EffectProcHeal, p)
			se.Defense = def
			if def {
				se.Chance = 100
			}
			d.Effects = append(d.Effects, se)
		default:
			d.unsupported("a %s proc of kind %q (%s)", on, p.Kind, p.Type)
		}
	}
}
