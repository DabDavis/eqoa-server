package entity

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/bits"
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
//   - hate_pct (ours, not the workbook's): the share of a hit's damage that
//     is its hate on the creature, all of it when absent (petition 1372:
//     Vylinnuza's Merciless Blizzard and Inferno, "little to no aggro
//     generated", hate_pct 0);
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
			d.undeadFromLine(line)
			d.charmCapFromWords(line, desc)
			return d
		}
		if _, seen := badSpellJSON.LoadOrStore(name, true); !seen {
			log.Printf("Spell: %q's effects (JSON) are not read, its line is: %v", name, err)
		}
	}
	return ParseSpellNamed(name, line, desc)
}

// charmCapFromWords gives a charm read from the workbook the level cap its
// line or description words ("Charms up to level 58 mobs", Binding Grasp's;
// "Charm up to level 54", the bard's), which no workbook field holds: until
// 2026-11-02 such a charm took up to its own level (Binding Grasp, 60: Lady
// Vox at 57), its words read only by the line's reader (spelltext.go).
func (d *SpellDef) charmCapFromWords(line, desc string) {
	if !d.Charms() || d.CharmMax > 0 {
		return
	}
	for _, words := range []string{line, desc} {
		if m := reCharmMax.FindStringSubmatch(strings.ToLower(words)); m != nil {
			d.CharmMax = atoi(m[1])
			return
		}
	}
}

// undeadFromLine makes a harmful spell read from the workbook for undead
// alone when its line says "Target undead" (EQOA-Data's words, which the line
// reader reads the same way: spelltext.go's target words), the one thing a
// line still gives a spell its effects make: the workbook marks Punish Death
// and Exorcise "Enemy" where EQOA-Data and the live game have them for
// undead (2026-11-01). A friendly spell keeps the workbook's reading (a
// paladin's Wards and Smite Undead, procs its line says "Target undead").
func (d *SpellDef) undeadFromLine(line string) {
	if !d.Harmful || d.undeadOnly() {
		return
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "target undead") {
		d.UndeadOnly = true
	}
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
	SheetType  string       `json:"sheet_type"`
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
	CureMax         int           `json:"cure_max"` // a cure's most effects taken (Restorative's 3), 0 all
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
	// TargetType is a proc's own target type: "Enemy - Undead" fires on the
	// undead alone (the proc's, not its buff's, which goes on a friend)
	TargetType string `json:"target_type"`
	// Absolute is a "Statistic Absolute" row's figure: set, not changed by
	// (the spells lane's key: its speed below 0 a root, its power set)
	Absolute bool `json:"absolute"`
	// DebuffScale is a hit that grows with what its target's resist of a
	// kind is lowered by (SpellEffect.DebuffStat)
	DebuffScale *debuffScaleJSON `json:"debuff_scale"`
	// HatePct is the share of a hit's damage, in percent, that is its hate
	// on the creature it lands on (SpellEffect.HateOff); absent, all of it
	HatePct *float64 `json:"hate_pct"`
	// ProcFX is a proc's own look on what each of its procs hits (ours, not the workbook's:
	// the owner, 2026-11-11, of Largesse of Plague's, 0x830D8A97): a SPELLFX.CSF DictID, a
	// number or a "0x..." string, on the proc's own entry or on its Offensive Proc (for each
	// of its procs); none, the stand-in of its kind (proc.go)
	ProcFX fxJSON `json:"proc_fx"`
}

// fxJSON is a visual's DictID in the effects JSON: a number (2198702743) or a hex string
// ("0x830D8A97").
type fxJSON uint32

func (f *fxJSON) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x"), 16, 32)
		if err != nil {
			return fmt.Errorf("proc_fx %q: %v", s, err)
		}
		*f = fxJSON(v)
		return nil
	}
	var n uint32
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("proc_fx %s: a DictID, a number or \"0x...\"", b)
	}
	*f = fxJSON(n)
	return nil
}

// debuffScaleJSON is an effect's debuff_scale: {"stat": a resist's name, as
// "resist" takes it, "pct": the percent more a point of its debuff adds}.
type debuffScaleJSON struct {
	Stat string  `json:"stat"`
	Pct  float64 `json:"pct"`
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

// spriteBody is the body a workbook SpriteID shows: the CHAR.ESF CSprite of
// that DictID read the other way round. The workbook writes every SpriteID
// byte-reversed against the disc's DictIDs, which npcs.modelid and
// charactermodel hold and the entity channel carries as they are (measured
// 2026-11-01 against the disc's 568 CSprites: 106 of 106 of its spell
// effects' sprites and 6 of 6 of its item effects'). Until then the sprite
// was sent as written, a model no CSprite has, and every illusion the
// workbook gives by its sprite showed none (the brief's contract: "as
// written; the lane resolves byte order").
func spriteBody(v uint32) uint32 { return bits.ReverseBytes32(v) }

// readEnemyBody reads an illusion on a spell for an enemy: the body goes on the
// creature it hits, for what lasts of it, and the spell keeps its target, as a
// thrown potion's does (potion.go readBody). readEffectJSON's illusion is for a
// friend's body and would turn the spell on its caster. It reports whether e was one
// (Transmutive Mix Potion, petition 1181, is the first such record).
func (d *SpellDef) readEnemyBody(e effectJSON, targetType string) bool {
	if !strings.Contains(targetType, "enemy") || !strings.EqualFold(strings.TrimSpace(e.Kind), "illusion") {
		return false
	}
	v, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e.Sprite)), "0x"), 16, 32)
	if v == 0 || invisibleSprite(uint32(v)) {
		return false
	}
	if !d.hasIllusion() {
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectIllusion, ID: e.ID})
	}
	d.FormModel = spriteBody(uint32(v))
	return true
}

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
	d := SpellDef{Workbook: true, Stack: s.Stack, StackLevel: s.Level, MinLevel: s.MinLevel,
		AltersLook: strings.EqualFold(strings.TrimSpace(s.SheetType), "Alter Appearance")}
	scope := strings.ToLower(strings.TrimSpace(s.Scope))
	if scope == "target" && allTargetArea(s.Effects) {
		// a spell for a target whose every effect is for the area round it
		// (Lightning Maelstrom, the only one: its radius 25 and its line's
		// "AOE") is that area: its target both figures, the rest the
		// area's (2026-11-01; until then its target alone took both)
		scope = "target_aoe"
	}
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
			d.TargetKind = kind // workbookmore.go
		}
	}
	if r := strings.TrimSpace(s.ReqEquip); r != "" && !strings.EqualFold(r, "any") {
		d.ReqEquip = r // workbookmore.go
	}
	d.Duration = seconds(s.DurationS)
	var area []SpellEffect
	targetOnly := map[int]bool{} // a target-based area spell's effects for its target alone
	casterArea := map[int]bool{} // a target spell's effects for the area round its caster too
	for _, e := range s.Effects {
		if d.Duration <= 0 && e.DurationS > 0 {
			d.Duration = seconds(e.DurationS)
		}
		if d.readEnemyBody(e, tt) {
			continue
		}
		n := len(d.Effects)
		d.readEffectJSON(name, desc, e, scope == "target_aoe", &area)
		for i := n; i < len(d.Effects); i++ {
			// its own marks (parts.go)
			d.Effects[i].EndsOnContact, d.Effects[i].EndsOnCast = e.DispelOnContact, e.DispelOnCast
		}
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
		case es == "group" && (scope == "target" || scope == "pet") && groupShare(d.Effects[n:]):
			// a share for the caster's group in a spell cast at an enemy
			// (the shadow knight's screams' AC) or at its pet (the
			// alchemist's Destroy Contraption: the group's healing over time
			// and power as its contraption goes; the user, 2026-11-01, "make
			// like scream"): its caster's, its group's when a mastery adds the
			// spell (ShareGroup, lifetap.go)
			for i := n; i < len(d.Effects); i++ {
				d.Effects[i].Self, d.Effects[i].ShareGroup = true, true
			}
			d.ShareRadius = float32(s.Radius)
		case es == "group" && scope == "self" && groupShare(d.Effects[n:]):
			// a self spell's part for the group (the bard's PB AE songs:
			// Chant of Battle, Elemental Rhythms; Celedonicus' Grand
			// Finale's power): the caster's and its group's in the radius,
			// whatever adds the spell (2026-11-01; until then the caster's
			// alone)
			for i := n; i < len(d.Effects); i++ {
				d.Effects[i].Self, d.Effects[i].ShareGroup, d.Effects[i].ShareAll = true, true, true
			}
			d.ShareRadius = float32(s.Radius)
		case es == "player_aoe" && scope == "target":
			// a part for the area round its caster in a spell cast at a
			// target (the warrior's Shouts: the target's hate, and every
			// creature's round the warrior): the target takes it with the
			// rest, the area's others it alone (2026-11-01; until then the
			// target alone took it)
			for i := n; i < len(d.Effects); i++ {
				casterArea[i] = true
			}
		case es != "" && es != scope && es != "self" && es != "target":
			d.unsupported("an effect for %q in a spell for %q (it lands as the spell's)", es, scope)
		}
	}
	// invisibility is one part here (an Alter Appearance to the invisible
	// body is read as invisibility: invisibleSprite), so a spell whose
	// Invisible part a hit or a cast ends loses its invisibility whole, its
	// invisible body with it (Elemental Veil, Spirit Walk: the workbook marks
	// their Invisible and not their invisible body; marked alone, a hit
	// would end nothing seen)
	for _, e := range d.Effects {
		if e.Kind != EffectInvis {
			continue
		}
		for i := range d.Effects {
			if d.Effects[i].Kind == EffectInvis {
				d.Effects[i].EndsOnContact = d.Effects[i].EndsOnContact || e.EndsOnContact
				d.Effects[i].EndsOnCast = d.Effects[i].EndsOnCast || e.EndsOnCast
			}
		}
	}
	// a self spell with a part for its group: its own parts below nothing are
	// its caster's price (Celedonicus' Grand Finale's -5000 power beside the
	// group's +5000, the bard among them: "include bard in the +5000"),
	// landed with the caster's other parts, a price first (lifetap.go)
	if scope == "self" {
		shares := false
		for _, e := range d.Effects {
			shares = shares || e.ShareAll
		}
		for i := range d.Effects {
			if e := &d.Effects[i]; shares && !e.ShareGroup && e.Amount < 0 && (e.Kind == EffectHP || e.Kind == EffectPower || e.Kind == EffectStat) {
				e.Self = true
			}
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
	// a stance (Stance): a self spell raising stats whose cure is the
	// workbook's "ID?" ends the bearer's other stances, not every harm
	if d.Target == TargetSelf && !d.Harmful {
		raises := false
		for _, e := range d.Effects {
			if e.Kind == EffectStat && e.Amount > 0 && !e.Set {
				raises = true
			}
		}
		for i, w := range d.Removes {
			if raises && strings.EqualFold(strings.TrimSpace(w), "id?") {
				d.Removes[i], d.Stance = "stance", true
			}
		}
	}
	// a run speed buff whose cure is the workbook's "ID?" (Spirit of the
	// Wolf, Pack Spirit) ends its bearer's other speed buffs, as the
	// workbook's own "removing lesser speed boosts" do, not every harm (the
	// user, 2026-11-01: "yes make them end other speed buffs")
	if !d.Harmful {
		quick := false
		for _, e := range d.Effects {
			if e.Kind == EffectSpeed && e.Amount >= 0 { // a speed of no figure is the movement buff (readSpeedJSON)
				quick = true
			}
		}
		for i, w := range d.Removes {
			if quick && strings.EqualFold(strings.TrimSpace(w), "id?") {
				d.Removes[i] = "speed"
			}
		}
	}
	// a harmful spell the workbook gives no time whose effect ends at
	// contact lasts until contact (untilContact, workbook.go): Enshrouding
	// darkness, the shadow knight's snare, Duration 0 and DisspellOnContact
	// 1 (the user, 2026-11-01: "until hit"); until then it lasted no time
	if d.Duration <= 0 && d.DispelOnContact && d.Harmful {
		d.Duration = untilContact
	}
	// the undead check, which the workbook's Legend calls the one oddity of a
	// negative HP1 and its Target Type gives only some of the undead spells
	// ("Enemy - Undead": the cleric's Death line, the Charm Undead line;
	// "Enemy": the paladin's and the cleric's Undead lines, Punish Death,
	// Exorcise): a harmful spell named for the undead is for undead alone,
	// as its line's reading has it (undead.go); SpellDefFrom adds a line's
	// own "Target undead"
	d.readUndead(name, desc)
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
	case len(casterArea) > 0:
		var share []SpellEffect
		for i := range d.Effects {
			if casterArea[i] {
				share = append(share, d.Effects[i])
			}
		}
		d.Area = &SpellArea{Centre: AreaOnCaster, Radius: r, Effects: share, Why: "the workbook's area round its caster in a spell for a target"}
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

// allTargetArea reports whether every effect that names a scope names the
// area round the target (and one does).
func allTargetArea(effects []effectJSON) bool {
	n := 0
	for _, e := range effects {
		switch es := strings.ToLower(strings.TrimSpace(e.Scope)); es {
		case "":
		case "target_aoe":
			n++
		default:
			return false
		}
	}
	return n > 0
}

// cureByName is what a cure the workbook leaves at "ID?" (it does not know
// which) takes off, by its spell's name (the user, 2026-11-01: "by name"):
// an antidote poison, a curative disease, a mask's removal a mask
// (cureMatches' words); "" when the name says nothing, and the cure takes
// every harmful effect as before (the warriors' stances, the monks' Purity,
// Spirit of the Wolf).
func cureByName(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "antidote"):
		return "poison"
	case strings.Contains(n, "curative"):
		return "disease"
	case strings.Contains(n, "mask"):
		return "mask"
	case strings.Contains(n, "shield break"):
		// the paladin's Take a Shield Break: its own barriers end, their
		// hold with them (the user, 2026-11-01: "yes end the barriers")
		return "mitigation"
	}
	return ""
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
	if ds := e.DebuffScale; ds != nil && ds.Pct != 0 {
		if st, ok := jsonStat(ds.Stat); ok {
			out.DebuffStat, out.DebuffPct = st, ds.Pct
		}
	}
	if e.HatePct != nil {
		out.HateOff = 100 - max(0, min(100, round(*e.HatePct))) // only a hit reads it (cast.go)
	}
	return out
}

// groupShare is whether a workbook effect's parts for a group, in a spell
// cast at an enemy, are a share for the caster's side: each raises a stat
// (Arcane Bonding's root on the target's group of enemies is not one).
func groupShare(es []SpellEffect) bool {
	for _, e := range es {
		if e.Kind != EffectStat && e.Kind != EffectPower || e.Amount <= 0 {
			return false
		}
	}
	return len(es) > 0
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
			d.Backstab = true // its caster behind its target (backstab.go)
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
		if e.Absolute {
			se := effectOf(EffectPower, e) // the bearer's power set to it (cast.go)
			se.Set = true
			add(se)
		} else if e.Amount != 0 {
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
		case kind == "stat_set" && st >= StatPierce && st <= StatRanged:
			d.unsupported("%s set to %g", e.Stat, e.Amount)
		case kind == "stat_set" && st == StatMaxPower:
			n := max(round(e.Amount), 0)
			add(SpellEffect{Kind: EffectPowerCap, Amount: n, AmountMax: n, ID: e.ID})
		case kind == "stat_set" && (st == StatMaxHP || st > StatCha):
			d.unsupported("%s set to %g", e.Stat, e.Amount)
		case kind == "stat_set":
			se := effectOf(EffectStat, e)
			se.Stat, se.Set = st, true
			add(se)
		case e.Amount != 0 || e.LevelPct != 0 || e.ScalePct != 0:
			// one all of scaling (amount 0) counts too: the Wisp's Ponder,
			// 80% of the caster's level and 3% of its INT (wisp_kit_20261112)
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
		d.FormModel = spriteBody(uint32(v))
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
		case strings.EqualFold(e.Stat, "hot"):
			// hit points over time, each tick's to the caster (Infernal
			// Bond's: the sheet's HoT column of a Drain, its line "Decrease
			// HP per tic by 240, give to self"; cast.go's tick)
			se = effectOf(EffectHPTick, e)
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
		d.readProcsJSON(name, kind, e)
	case "gate":
		add(SpellEffect{Kind: EffectGate, ID: e.ID})
	case "teleport":
		d.Location = strings.TrimSpace(e.Location)
		// read as a teleport (gates.go): a spellgates row by its name is
		// where it lands; with none it is refused, its place logged
		d.Unread = append(d.Unread, "teleport to "+d.Location)
	case "cure":
		add(SpellEffect{Kind: EffectCure, ID: e.ID})
		rm := strings.TrimSpace(e.Removes)
		if strings.EqualFold(rm, "id?") {
			if by := cureByName(name); by != "" {
				rm = by
			}
		}
		d.Removes = append(d.Removes, rm)
		if e.CureMax > 0 {
			d.CureMax = e.CureMax
		}
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
	case "mark_heal":
		// ours (markheal.go): a mark on a creature that heals whoever hits it
		if n := round(e.Amount); n > 0 {
			add(SpellEffect{Kind: EffectMarkHeal, Amount: n, AmountMax: n, ID: e.ID})
		} else {
			d.unsupported("a mark_heal of %v", e.Amount)
		}
	case "faction":
		add(SpellEffect{Kind: EffectDisguise, ID: e.ID}) // workbookmore.go
	case "raiment":
		// ours (wispform.go): a Wisp in its own form shown in its gear
		add(SpellEffect{Kind: EffectRaiment, ID: e.ID})
	case "pickpocket":
		add(SpellEffect{Kind: EffectPickpocket, ID: e.ID})
	case "summon_player":
		add(SpellEffect{Kind: EffectSummonPlayer, ID: e.ID})
	case "off_mod", "def_mod", "hp_factor":
		se := effectOf(EffectStat, e)
		se.Stat = map[string]Stat{"off_mod": StatOffMod, "def_mod": StatDefMod, "hp_factor": StatHPFactor}[kind]
		se.Resist = StatNone
		if se.Amount != 0 {
			add(se)
		}
	case "melee_debuff":
		if st, ok := jsonStat(e.Stat); ok && st >= StatPierce && st <= StatRanged && e.Amount != 0 {
			se := effectOf(EffectStat, e)
			se.Stat = st
			add(se)
		} else {
			d.unsupported("a melee debuff of %q", e.Stat)
		}
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
	case n < 0 && e.Absolute:
		// a speed set below nothing: held in place (the spells lane's "Root"),
		// resisted as a root is (root.go)
		se := effectOf(EffectRoot, e)
		se.Amount, se.AmountMax = 0, 0
		d.Effects = append(d.Effects, se)
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
// (dshield.go), a heal a heal of its bearer at the proc (workbook.go); the
// rest is kept as unsupported.
//
// The workbook files every damage shield as a defensive proc (its legend:
// "Defensive procs have 100% proc rate on hit"): the ranger's coats from
// Thistlecoat on, the druid's thorns, the magician's auras and pet marks, the
// monk's stances, Chilling Aura (57 spells, 37 of them its arch type "Damage
// Shield"; every one's line says "Damage Shield Decrease Hitpoints by N").
// Read as dshield.go's defensive procs they answered one hit in ten
// (defProcChancePct, resisted too); each is a damage shield here (every hit,
// unresisted, the strongest of the bearer's): the ranger's coats first (the
// user, 2026-11-04: "thistlecoat is supposed to be Damage shield"), then the
// rest with the balance fitted again on them ("re-fit the other damage
// shields": tools/refit_damage_shields.py). A defensive proc of another kind
// (Innoruuk's Aura's hate) stays a proc.
func (d *SpellDef) readProcsJSON(name, kind string, e effectJSON) {
	if len(e.Procs) == 0 {
		d.unsupported("a %s with no proc effects", kind)
		return
	}
	for _, p := range e.Procs {
		on := strings.ToLower(strings.TrimSpace(p.On))
		def := on == "defense" || on == "defence" || (on == "" && kind == "def_proc")
		pk := strings.ToLower(strings.TrimSpace(p.Kind))
		n := round(p.Amount)
		// a proc for the undead alone (its own target type, "Enemy -
		// Undead": Smite Undead's, Crusader's Aura's) fires only on an
		// undead creature (undead.go); one for animals or summoned
		// creatures is not read so (none is in the workbook)
		tt := strings.ToLower(p.TargetType)
		undead := strings.Contains(tt, "undead")
		if strings.Contains(tt, "animal") || strings.Contains(tt, "summoned") {
			d.unsupported("a proc for %q alone (it fires on any creature)", p.TargetType)
		}
		switch {
		case (pk == "hp" || pk == "hp2") && n < 0:
			se := effectOf(EffectProc, p)
			se.Amount, se.AmountMax = -n, -n
			se.ScaleBearer = se.Scale != StatNone // the proc effect's own modifier: its bearer's (the legend)
			se.Undead = undead
			se.ProcFX = uint32(p.ProcFX)
			if se.ProcFX == 0 {
				se.ProcFX = uint32(e.ProcFX) // its Offensive Proc's, for each of its procs
			}
			if def {
				se.Kind = EffectShield
				// a defensive proc's or a shield's damage is worked out at
				// its landing (Shield.with), its scale its caster's
				se.ScaleBearer = false
			}
			d.Effects = append(d.Effects, se)
		case pk == "hp" && n > 0:
			se := effectOf(EffectProcHeal, p)
			se.Defense = def
			se.Undead = undead
			if def {
				se.Chance = 100
			}
			d.Effects = append(d.Effects, se)
		default:
			// anything else a proc does (a hate, a stat taken, a tick, power)
			// is a spell of its own the proc casts (workbookmore.go)
			var pd SpellDef
			var none []SpellEffect
			pd.readEffectJSON("", "", p, false, &none)
			if len(pd.Effects) == 0 {
				d.unsupported("a %s proc of kind %q (%s)", on, p.Kind, p.Type)
				continue
			}
			if strings.EqualFold(strings.TrimSpace(p.Scope), "self") {
				for i := range pd.Effects {
					pd.Effects[i].Self = true // on the bearer
				}
			}
			ps := ProcSpell{Defense: def, Effects: pd.Effects, Duration: seconds(p.DurationS), Undead: undead}
			if def {
				ps.Chance = 100 // the legend's: a defensive proc at every hit
			}
			d.ProcSpells = append(d.ProcSpells, ps)
		}
	}
}
