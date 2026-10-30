package entity

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// spelltext.go — what a spell does, read out of the text that says so.
//
// A spell's ability record (spellpattern) says how it is cast, never what it
// does. The numbers are in the per-class spell lists of the EQOA-Data
// project (external/EQOA-Data, Information/databases/spells), one line a
// spell: "Target, Increase AC by 150, Max Power by 60, MaxHP by 30, HP by 30,
// 30min". tools/import_spell_effects.py copies each line into spelleffect by
// the spell's name; this reads it.
//
// Sandstorm agrees with those lines where it was measured (a druid of 23,
// 2026-10-21, claude-notes/casting-sandstorm-20261021.md): Rockskin gave AC
// +150, max power +60, max HP +30; Minor Healing's tick was 35 with CHA 60
// ("20 ; Increase 25% based on CHA"), Healing's 140 ("125", the same 25%).
// Not everywhere: Nature's Wisdom gave WIS +12 where its line says 1, and
// Fiery Burst hit for 36 where "26 ; Increase 8% based on DEX" makes 34 at
// DEX 105. What the lines leave out is not guessed at here.

// SpellTarget is whom a spell lands on: its line's first word.
type SpellTarget int

const (
	TargetSingle SpellTarget = iota // "Target": the caster's target
	TargetSelf                      // "Self": the caster, whatever is targeted
	TargetGroup                     // "Group": the caster's group around them
	TargetPet                       // "Pet": the caster's pet (pet.go)
	TargetUndead                    // "Target undead": the target, harmful
)

// EffectKind is one thing a spell does.
type EffectKind int

const (
	EffectHP          EffectKind = iota // hit points now (Amount < 0: damage)
	EffectHPTick                        // hit points each tick of the duration
	EffectPower                         // power now
	EffectPowerTick                     // power each tick
	EffectStat                          // a stat changed for the duration
	EffectRoot                          // can not move, for the duration
	EffectSnare                         // moves slower
	EffectSpeed                         // runs faster (Spirit of the Wolf: "Sow")
	EffectInvis                         // unseen
	EffectGate                          // back to the bind point
	EffectHate                          // the target's hate for the caster
	EffectSummon                        // a pet (pet.go): SpellDef.Pet names it
	EffectProc                          // the bearer's melee hits may do Amount more (proc.go)
	EffectCharm                         // the target becomes the caster's pet (charm.go)
	EffectIllusion                      // the bearer shown in another body (illusion.go): SpellDef.Form
	EffectEndIllusion                   // the bearer's own body back (illusion.go)
	EffectResurrect                     // Amount% of the latest death's debt given back (resurrect.go)
	EffectShield                        // a creature's melee hit on the bearer takes Amount back (dshield.go)
	EffectDefProc                       // a creature's melee hit on the bearer may take Amount back (dshield.go)
	EffectPowerCap                      // the bearer's maximum power at most Amount while it lasts; lasting no time, its power taken down to it (powercap.go)
	EffectForget                        // the target forgets its hate for the caster (vampire.go: Vampiric Gaze)
	EffectWard                          // a creature's melee hit on the bearer cut by Amount%, or to 1 when ToOne, Cap in all (ward.go)
	EffectMark                          // on a creature: each player's or pet's hit on it does Amount to AmountMax more, 0 its default (mark.go)
	EffectMarkHeal                      // on a creature: each player's or pet's hit on it heals the attacker Amount to AmountMax (mark.go)
	EffectCure                          // takes Amount harmful effects of the Cure kind off the bearer, 0 by its level (cure.go)
	EffectHPCap                         // the bearer's maximum hit points at most Amount (1 or more) while it lasts; lasting no time, its hit points taken down to it (spiritform.go)
	EffectInvuln                        // the bearer takes nothing of a creature's hit while it lasts (spiritform.go)
	EffectLull                          // a creature notices a player only within LullRadius while it lasts, up to level Amount (0: the spell's own and LullLevelBand) (lull.go)
	EffectCallHero                      // the caster's group far from it asked to come to its side (callhero.go)
)

// Stat names the stat an EffectStat changes, and the one an amount scales
// with.
type Stat int

const (
	StatNone Stat = iota
	StatStr
	StatSta
	StatAgi
	StatDex
	StatWis
	StatInt
	StatCha
	StatAC
	StatMaxHP
	StatMaxPower
	StatFR
	StatCR
	StatPR
	StatDR
	StatLR
	StatAR
	// StatMelee is what the bearer's every melee hit does more ("Increase
	// Damage per Hit by 500": Frenzy, vampire.go); no stat channel shows it
	StatMelee
)

// SpellEffect is one effect of a spell as its line gives it: Amount, plus
// ScalePct percent of the caster's Scale stat ("; Increase 25% based on
// CHA"), up to AmountMax when the line gives a range ("by 50 to 60").
type SpellEffect struct {
	Kind      EffectKind
	Stat      Stat // EffectStat
	Amount    int  // signed: a decrease is negative
	AmountMax int  // a range's top, else Amount
	Scale     Stat
	ScalePct  int
	// Self is an effect on the caster, whoever the spell lands on: a
	// pact's price ("Decrease Hitpoints by 50 to Self"), "Increase AC by
	// 25 Self", "Increase Caster HP by 1160" (lifetap.go)
	Self bool
	// Drain is where what the target loses goes: a life tap's damage heals
	// its caster (or the caster's group), a drained stat or power is the
	// caster's (lifetap.go)
	Drain DrainTo
	// ScaleBearer: the scale is its bearer's stat, taken each time it is
	// used ("; Increase 20% based on bearer's STR"): a proc's, the stat of
	// the one whose hits proc it, not its caster's at its landing (proc.go)
	ScaleBearer bool
	// Cap is the most the effect does, its scale included ("max 250"); 0
	// for no cap
	Cap int
	// Undead: a shield that answers undead attackers alone (dshield.go)
	Undead bool
	// Set: the stat is set to Amount, not changed by it ("Set STR to 500"):
	// the difference from the bearer's own stat as the spell lands
	// (vampire.go's setStatsLocked)
	Set bool
	// Masks: an end of looks that ends the rogue's masks alone, Remove
	// Mask's (illusion.go)
	Masks bool
	// ToOne: a ward that makes each hit it cuts do 1 ("Reduce Damage Taken
	// to 1"), its Cap the most it takes off in all (ward.go)
	ToOne bool
	// Cure is what a cure takes off: poison, disease, both, or CureAny
	// (cure.go)
	Cure CureKind
}

// SpellDef is a spell's line, read: whom it lands on, what it does, how
// long what lasts lasts, and whether it is cast at an enemy.
type SpellDef struct {
	Target   SpellTarget
	Effects  []SpellEffect
	Duration time.Duration // 0: it all happens when it lands
	Harmful  bool
	// Pet is the creature a summoning line names ("Summons an Iron Golem to
	// your defense": "iron golem"), "" when it names none ("Summons a pet").
	Pet string
	// Unread is the part of the line this does not act on (procs, cures...),
	// for the log.
	Unread []string
	// Ignored is what the line says that is read and left alone on purpose
	// (spellnotes.go): a group spell's radius, a buff's hate, an epic list's
	// power cost, cast and recast.
	Ignored []string
	// CharmMax is the highest level a charm takes when its line or
	// description says ("Charms up to level 54 mobs", "Charm up to level
	// 54"); 0: the spell's own level (charm.go).
	CharmMax int
	// Form is the body an illusion shows its bearer in, as the spell's name
	// gives it ("troll", "clay golem"), and FormKind whom it lands on
	// (illusion.go); "" and FormNone for every other spell.
	Form     string
	FormKind FormKind
	// Appearance: what lasts changes how its bearer looks, with or without a
	// body this server shows (illusion.go readAppearance): an illusion, a
	// glamour, a druid's or shaman's form, a mask, Lich. Remove Illusion and
	// True Form end it.
	Appearance bool
	// UndeadOnly: the spell works on undead alone (undead.go), a "Target
	// undead" line or a charm its name or description says is for undead.
	UndeadOnly bool
	// Ticks is how many times what it gives over time ticks ("3 ticks"),
	// the first at its landing; 0: every six seconds for its duration.
	Ticks int
	// PetLevel is the level of the pet a summoning line makes when it says
	// ("Summons a pet, level 30"), at most the caster's; 0: the spell's own
	// (pet.go).
	PetLevel int
	// Reagent is the item a cast takes ReagentCount of from its caster's
	// inventory as it begins, refused without them ("Reagent Unit of Blood
	// x2": the vampires' abilities, vampire.go); "" for none.
	Reagent      string
	ReagentCount int
	// Afflicts is what kind of harm the spell does, poison or disease or
	// both, by its name's and line's words, and NoCure that its line says no
	// cure takes it off ("cannot be cured"): what a cure looks at (cure.go).
	Afflicts CureKind
	NoCure   bool
}

// Charms reports whether the spell makes its target the caster's pet.
func (d *SpellDef) Charms() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectCharm {
			return true
		}
	}
	return false
}

// reCharmMax is a charm's level cap in its words.
var reCharmMax = regexp.MustCompile(`up to level (\d+)`)

// Summons reports whether the spell makes a pet.
func (d *SpellDef) Summons() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectSummon {
			return true
		}
	}
	return false
}

// A line that summons a pet: a summoning ("Summons", "Summon", the lists'
// "Summuns") of a pet, of a creature "to aid you", "to your aid" or "to your
// defense", or of a familiar (the wizard's: "Summons a familar creature",
// the lists' spelling, read as "familiar"; the test lanes' report of
// 2026-10-29: Lesser Familiar and Lesser Guardian made nothing, Wilting
// Flesh's "to your aid" neither). The alchemists' "SELF Summons Unstable
// Potion" and Phase Ring's "Summons a ring to invis" make items, not pets,
// and say none of those.
var (
	reSummons  = regexp.MustCompile(`\bsumm[ou]ns?\b`)
	rePetLevel = regexp.MustCompile(`\blevel\s+(\d+)\b`)
	reSummonOf = regexp.MustCompile(`\bsumm[ou]ns?\s+(?:a\s+|an\s+|the\s+)?(.+?)\s+(?:to aid you|to your aid|to your defense|pet|creature)\b`)
	reFamiliar = regexp.MustCompile(`\bfamil(?:i)?ar\b`)
)

// summonLine reads a summoning line: whether it makes a pet, and the
// creature it names ("" for a bare "pet").
func summonLine(text string) (bool, string) {
	if !reSummons.MatchString(text) {
		return false, ""
	}
	text = reFamiliar.ReplaceAllString(text, "familiar")
	if !strings.Contains(text, "pet") && !strings.Contains(text, "to aid you") && !strings.Contains(text, "to your aid") &&
		!strings.Contains(text, "to your defense") && !strings.Contains(text, "familiar") {
		return false, ""
	}
	if m := reSummonOf.FindStringSubmatch(text); m != nil {
		if name := strings.TrimSpace(m[1]); name != "" && name != "a" && name != "an" {
			return true, name
		}
	}
	return true, ""
}

var (
	reDuration = regexp.MustCompile(`^(?:for\s+)?(\d+(?:\.\d+)?)\s*(sec|secs|second|seconds|s|min|mins|minute|minutes|hr|hrs|hour|hours)$`)
	reFor      = regexp.MustCompile(`\s+for\s+(\d+(?:\.\d+)?)\s*(sec|secs|second|seconds|s|min|mins|minute|minutes)$`)
	reScale    = regexp.MustCompile(`^(?:increase|modified|mod)?\s*(\d+)\s*%\s*(?:based on|mod by|modified by|by)\s+((?:bearer|wearer)'?s\s+)?([a-z]+)\.?$`)
	reCap      = regexp.MustCompile(`^(?:max|at most)\s+(\d+)$`)
	reTicks    = regexp.MustCompile(`^(\d+)\s+ticks?$`)
	reScaleIn  = regexp.MustCompile(`\s+(?:modified|mod)\s+(\d+)\s*%\s*(?:based on|by)\s+([a-z]+)\w*$`)
	reBy       = regexp.MustCompile(`^(.*?)\s+by\s+(\d+)(?:\s+to\s+(\d+))?(?:\s+x\d+)?$`)
	reVerb     = regexp.MustCompile(`^(increase|decrease|buff|debuff|drain|raise|lower|heal)\s+(.*)$`)
	reTail     = regexp.MustCompile(`\s+(\d+(?:\.\d+)?)\s*(sec|secs|seconds|min|mins|minutes)$`)
	reHate     = regexp.MustCompile(`^\+\s*(\d+)\s*hate$`)
	reReagent  = regexp.MustCompile(`^reagents?\s*:?\s+(.+?)\s+x\s*(\d+)$`)
	reSetStat  = regexp.MustCompile(`^set\s+(.+?)\s+to\s+(\d+)$`)
	reSplit    = regexp.MustCompile(`\s*(?:&|/|\band\b)\s*`)
)

// spellAliases rewrite a line's other ways of saying one thing.
var spellAliases = []struct{ from, to string }{
	{"decrease drain ", "drain "},
	{"hp via backstab by", "hp by"},
	{"damage by ", "decrease hitpoints by "},
	// the lists' typos and a monk's "Target Damage Hp by 16" (spellnotes.go)
	{"damage hp by ", "decrease hp by "},
	{"hitoints", "hitpoints"},
	{", ecrease ", ", decrease "},
	{"based on dext", "based on dex"},
}

// statWords are the stat names a line uses, longest first where one starts
// another.
var statWords = []struct {
	word string
	stat Stat
}{
	{"maximum hp", StatMaxHP}, {"max hp", StatMaxHP}, {"maxhp", StatMaxHP}, {"mhp", StatMaxHP},
	{"maximum power", StatMaxPower}, {"max power", StatMaxPower}, {"maxpower", StatMaxPower},
	{"str", StatStr}, {"sta", StatSta}, {"agi", StatAgi}, {"dex", StatDex},
	{"wis", StatWis}, {"int", StatInt}, {"cha", StatCha}, {"ac", StatAC},
	{"fr", StatFR}, {"cr", StatCR}, {"pr", StatPR}, {"dr", StatDR}, {"lr", StatLR}, {"ar", StatAR},
	{"resist arcane", StatAR}, {"resist fire", StatFR}, {"resist cold", StatCR},
	{"resist poison", StatPR}, {"resist disease", StatDR}, {"resist lightning", StatLR},
	// cold resist as Allakhazam's pages and the wizard's Element Guard word
	// it ("Increase Resist Fire & Ice by 45"; jib's review, F3 pass 19)
	{"resist ice", StatCR},
	// a bearer's every melee hit (Frenzy's "Increase Damage per Hit")
	{"damage per hit", StatMelee},
}

func statWord(s string) (Stat, bool) {
	s = strings.TrimSpace(s)
	for _, w := range statWords {
		if s == w.word {
			return w.stat, true
		}
	}
	return StatNone, false
}

// defaultSpellLines are what a spell does when spelleffect holds no line
// for it (a GM's row there comes first). Arcane Beckon (F3 20261025-180443,
// jib: "supposed to buff charisma"): an enchanter's level 15 self spell,
// "Fills you with an arcane presence, increasing charisma." No list, wiki
// or recording gives its numbers; CHA +15 for 30 minutes is ours.
var defaultSpellLines = map[string]string{
	"arcane beckon": "Self, Increase CHA by 15, 30min",
}

// DefaultSpellLine is the line a spell of this name has when the database
// gives none, "" when there is none.
func DefaultSpellLine(name string) string {
	return defaultSpellLines[strings.ToLower(strings.TrimSpace(name))]
}

// ParseSpellNamed is ParseSpell with what the spell's name says beside its
// line and description: a default line when it has none, and an illusion's
// body and whom it lands on.
func ParseSpellNamed(name, line, desc string) SpellDef {
	if strings.TrimSpace(line) == "" {
		line = DefaultSpellLine(name)
	}
	d := ParseSpell(line, desc)
	d.readIllusion(name, desc)
	d.readAppearance(desc)       // illusion.go
	d.readUndead(name, desc)     // undead.go
	d.readEnchanterProc(name)    // enchanter.go
	d.healWithMaxHP(name)        // spellnotes.go
	d.readShield(name, desc)     // dshield.go
	d.readAffliction(name, line) // cure.go
	return d
}

// ParseSpell reads a spell's line with its ability's description, which
// says what the line cannot: that its hit is a proc (proc.go), done to what
// the bearer hits in melee rather than to the target, and so cast at a
// friend. The enchanter's Chaotic Flux, Mark and Glyph are "Target,
// Decrease Hitpoints by 64, 5min" in EQOA-Data's lists, and "Imbues your
// attacks with chaotic arcane energy, giving them a chance to deal
// additional arcane damage" or "Arcane Offensive Proc" in their records.
func ParseSpell(line, desc string) SpellDef {
	d := ParseSpellText(line)
	if reProcDesc.MatchString(strings.ToLower(desc)) {
		d = d.asProc()
	}
	if d.Charms() && d.CharmMax == 0 {
		if m := reCharmMax.FindStringSubmatch(strings.ToLower(desc)); m != nil {
			d.CharmMax = atoi(m[1])
		}
	}
	return d
}

var reProcDesc = regexp.MustCompile(`\bproc\b|chance to deal additional`)

// asProc makes the line's hit a proc: its damage (low to high) the proc's,
// nothing harmful left, the rest as it was. A line with no hit is left as
// it is.
func (d SpellDef) asProc() SpellDef {
	at := -1
	for i, e := range d.Effects {
		if e.Kind == EffectHP && e.Amount < 0 && !e.Self {
			at = i
			break
		}
	}
	if at < 0 {
		return d
	}
	hit := d.Effects[at]
	effects := append([]SpellEffect(nil), d.Effects...)
	// its scale stays its own (enchanter.go: Stormlord's Rune's DEX)
	effects[at] = SpellEffect{Kind: EffectProc, Amount: -hit.AmountMax, AmountMax: -hit.Amount, Scale: hit.Scale, ScalePct: hit.ScalePct,
		ScaleBearer: hit.ScaleBearer, Cap: hit.Cap}
	d.Effects = effects
	d.Harmful = d.takesPower() // lifetap.go
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick, EffectStat:
			d.Harmful = d.Harmful || (e.Amount < 0 && !e.Self)
		case EffectRoot, EffectSnare, EffectHate, EffectMark, EffectMarkHeal, EffectLull:
			d.Harmful = true
		}
	}
	return d
}

// ParseSpellText reads one spell line (see the file's comment).
func ParseSpellText(line string) SpellDef {
	var d SpellDef
	text := strings.ToLower(strings.TrimSpace(line))
	if text == "" {
		return d
	}
	// A summoning line is the whole spell: a pet beside the caster.
	if ok, name := summonLine(text); ok {
		d.Target = TargetSelf
		d.Effects = []SpellEffect{{Kind: EffectSummon}}
		d.Pet = name
		if m := rePetLevel.FindStringSubmatch(text); m != nil {
			d.PetLevel, _ = strconv.Atoi(m[1])
		}
		return d
	}
	// An area spell's line names the target's share first: "Decrease
	// Hitpoints by 210 aoe Decrease Hitpoints by 90" (area.go reads the rest).
	text = targetsShare(text)
	for _, a := range spellAliases {
		text = strings.ReplaceAll(text, a.from, a.to)
	}
	text = joinStatLists(text) // spellnotes.go
	text = shieldWords(text)   // dshield.go
	text = markWords(text)     // mark.go
	// The verb a clause without one goes on with ("Increase AC by 12,
	// Maximum Power by 5"), and the effects a scale clause belongs to: the
	// last one, or every stat of the list it ends ("PR/DR by 15 ; Increase
	// 15% based on WIS" scales both; until the test lanes' report of
	// 2026-10-29 only DR was).
	verb := ""
	first, last := -1, -1
	for i, clause := range strings.Split(text, ",") {
		for j, part := range strings.Split(clause, ";") {
			part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), "."))
			if part == "" {
				continue
			}
			if i == 0 && j == 0 {
				var rest string
				d.Target, rest = spellTarget(part)
				if rest == "" {
					continue
				}
				part = rest
			}
			if m := reDuration.FindStringSubmatch(part); m != nil {
				d.Duration = spellDuration(m[1], m[2])
				continue
			}
			if m := reScale.FindStringSubmatch(part); m != nil {
				if st, ok := statWord(m[3]); ok && last >= 0 {
					pct, _ := strconv.Atoi(m[1])
					for k := first; k <= last; k++ {
						d.Effects[k].Scale, d.Effects[k].ScalePct, d.Effects[k].ScaleBearer = st, pct, m[2] != ""
					}
					continue
				}
			}
			if m := reCap.FindStringSubmatch(part); m != nil && last >= 0 {
				d.Effects[last].Cap, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reTicks.FindStringSubmatch(part); m != nil {
				d.Ticks, _ = strconv.Atoi(m[1])
				continue
			}
			if m := reReagent.FindStringSubmatch(part); m != nil {
				d.Reagent, d.ReagentCount = strings.TrimSpace(m[1]), atoi(m[2])
				continue
			}
			if spellNote(part) { // spellnotes.go
				d.Ignored = append(d.Ignored, part)
				continue
			}
			n := len(d.Effects)
			verb = d.parsePart(part, verb)
			if len(d.Effects) > n {
				first, last = statList(d.Effects, n), len(d.Effects)-1
			}
		}
	}
	// a resurrection's share of the latest death's debt (resurrect.go)
	d.readResurrect()
	// Cast at an enemy: what takes hit points or a stat, holds or slows,
	// works on hate, marks a creature (mark.go: the owner's ruling of
	// 2026-10-29), or takes power (lifetap.go takesPower: a drain or
	// decrease of the target's power, the owner's ruling of 2026-10-29; not
	// Minor Healing's "Decrease Power by 1" beside its heal), and a lull
	// (lull.go: cast at a creature, though its landing is no attack).
	d.Harmful = d.takesPower()
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick, EffectStat:
			if e.Amount < 0 && !e.Self {
				d.Harmful = true
			}
		case EffectRoot, EffectSnare, EffectHate, EffectCharm, EffectForget, EffectMark, EffectMarkHeal, EffectLull:
			d.Harmful = true
		}
	}
	if d.Target == TargetUndead {
		d.Harmful = true
	}
	// and what is cast at an enemy though nothing here acts on it yet
	for _, u := range d.Unread {
		for _, w := range []string{"charm", "lull", "calm", "mesmer", "fear", "stun", "aggro", "pickpocket", "taunt"} {
			if strings.Contains(u, w) {
				d.Harmful = true
			}
		}
	}
	d.lineDefaults() // spellnotes.go: what a line leaves out
	d.lullDefaults() // lull.go: a lull's time when it names none
	return d
}

// statList is where the effects one part added from n begin when they are a
// list of stats by one number ("AGI/STA by 20", "STR & DEX by 9", "buff DR,
// PR, FR, CR by 100"): n, so a scale after them is all of theirs. Anything
// else is its last effect alone, as before.
func statList(effects []SpellEffect, n int) int {
	last := len(effects) - 1
	for k := n; k < last; k++ {
		e, l := effects[k], effects[last]
		if e.Kind != EffectStat || l.Kind != EffectStat || e.Amount != l.Amount || e.AmountMax != l.AmountMax ||
			e.Drain != l.Drain || e.Self != l.Self {
			return last
		}
	}
	return n
}

// spellTarget reads a line's first clause: the target word, and what
// follows it in the same clause ("Target Root", "Self Buff HP by 50").
func spellTarget(first string) (SpellTarget, string) {
	for _, t := range []struct {
		word   string
		target SpellTarget
	}{
		{"target undead", TargetUndead}, {"target", TargetSingle}, {"self", TargetSelf},
		{"group", TargetGroup}, {"pet", TargetPet},
	} {
		if first == t.word {
			return t.target, ""
		}
		if strings.HasPrefix(first, t.word+" ") {
			return t.target, strings.TrimSpace(first[len(t.word):])
		}
	}
	return TargetSingle, first
}

func spellDuration(n, unit string) time.Duration {
	v, _ := strconv.ParseFloat(n, 64)
	switch {
	case strings.HasPrefix(unit, "s"):
		return time.Duration(v * float64(time.Second))
	case strings.HasPrefix(unit, "m"):
		return time.Duration(v * float64(time.Minute))
	}
	return time.Duration(v * float64(time.Hour))
}

// parsePart reads one clause or the part of one between semicolons, adding
// what it says to d, and returns the verb a following part without one goes
// on with.
func (d *SpellDef) parsePart(part, verb string) string {
	// what goes to the caster: a tap's "give self", a pact's "to Self"
	// (lifetap.go); a clause of those words alone is the last effect's
	part, to, self := splitTransfer(part)
	if part == "" {
		d.transferFrom(len(d.Effects)-1, "", to, self)
		return verb
	}
	// A trailing "for 30 seconds" is the duration, and so is a bare "5min"
	// at the end of a clause the line did not cut off ("Target Invis 5min").
	if m := reFor.FindStringSubmatch(part); m != nil {
		d.Duration = spellDuration(m[1], m[2])
		part = strings.TrimSpace(part[:len(part)-len(m[0])])
	} else if m := reTail.FindStringSubmatch(part); m != nil && !strings.HasSuffix(strings.TrimSpace(part[:len(part)-len(m[0])]), " by") {
		d.Duration = spellDuration(m[1], m[2])
		part = strings.TrimSpace(part[:len(part)-len(m[0])])
	}
	if m := reHate.FindStringSubmatch(part); m != nil {
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectHate, Amount: atoi(m[1]), AmountMax: atoi(m[1])})
		return verb
	}
	if e, ok := shieldPart(part); ok { // dshield.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if e, ok := wardPart(part); ok { // ward.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if e, ok := markPart(part); ok { // mark.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if e, ok := curePart(part); ok { // cure.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if reNoCure.MatchString(part) { // cure.go: a debuff no cure takes off
		d.NoCure = true
		return verb
	}
	if n, ok := powerCapPart(part); ok { // powercap.go
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectPowerCap, Amount: n, AmountMax: n})
		return verb
	}
	// the target's hate for the caster gone (Vampiric Gaze, vampire.go)
	if part == "set aggro to 0" || part == "aggro to 0" {
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectForget})
		return verb
	}
	// a stat set to a figure, not changed by one ("Set STR to 500")
	if m := reSetStat.FindStringSubmatch(part); m != nil {
		if st, ok := statWord(m[1]); ok && st != StatMaxHP && st != StatMaxPower && st != StatMelee {
			n := atoi(m[2])
			d.Effects = append(d.Effects, SpellEffect{Kind: EffectStat, Stat: st, Amount: n, AmountMax: n, Set: true})
			return verb
		}
	}
	if e, ok := spiritformPart(part); ok { // spiritform.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if e, ok := lullPart(part); ok { // lull.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	if d.lullLevelPart(part) { // lull.go: "up to level N" after a lull
		return verb
	}
	if e, ok := callHeroPart(part); ok { // callhero.go
		d.Effects = append(d.Effects, e)
		return verb
	}
	scale, scalePct := StatNone, 0
	if m := reScaleIn.FindStringSubmatch(part); m != nil {
		if st, ok := statWord(m[2]); ok {
			scale, scalePct = st, atoi(m[1])
			part = strings.TrimSpace(part[:len(part)-len(m[0])])
		}
	}
	switch {
	case part == "root" || part == "roots" || strings.HasSuffix(part, " and roots") || strings.HasSuffix(part, " root"):
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectRoot})
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(part, "and roots"), "roots"), "root"))
		if part == "" {
			return verb
		}
	case part == "snare":
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectSnare})
		return verb
	case part == "charm" || strings.HasPrefix(part, "charms ") || strings.HasPrefix(part, "charm "):
		// "Target, Charm ; Increase 25% based on CHA , 4min" (the scale clause
		// after it is the charm's), "Charms up to level 54 mobs, 1 hour"
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectCharm})
		if m := reCharmMax.FindStringSubmatch(part); m != nil {
			d.CharmMax = atoi(m[1])
		}
		return verb
	case part == "sow" || part == "spirit of the wolf":
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectSpeed})
		return verb
	case part == "invis" || part == "invisibility":
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectInvis})
		return verb
	case strings.Contains(part, "bind point") || strings.Contains(part, "bind location"):
		d.Effects = append(d.Effects, SpellEffect{Kind: EffectGate})
		return verb
	}
	// "X by N and Y by M" (or "&"): two of one verb.
	for _, joint := range []string{" and ", " & "} {
		if pieces := strings.Split(part, joint); len(pieces) == 2 && strings.Contains(pieces[0], " by ") && strings.Contains(pieces[1], " by ") {
			n := len(d.Effects)
			v := d.parsePart(pieces[0], verb)
			v = d.parsePart(pieces[1], v)
			d.transferFrom(n, v, to, self)
			return v
		}
	}
	if m := reVerb.FindStringSubmatch(part); m != nil {
		verb, part = m[1], m[2]
	}
	m := reBy.FindStringSubmatch(part)
	if m == nil || verb == "" {
		d.Unread = append(d.Unread, part)
		return verb
	}
	subject, amount, top := strings.TrimSpace(m[1]), atoi(m[2]), atoi(m[3])
	sign := 1
	if verb == "decrease" || verb == "debuff" || verb == "drain" || verb == "lower" {
		sign = -1
	}
	subject = strings.TrimSuffix(subject, " via")
	if top < amount {
		top = amount
	}
	add := func(e SpellEffect) {
		e.Amount, e.AmountMax = sign*amount, sign*top
		if e.Amount > e.AmountMax {
			e.Amount, e.AmountMax = e.AmountMax, e.Amount
		}
		e.Scale, e.ScalePct = scale, scalePct
		e.Drain, e.Self = resolveTransfer(verb, sign, to, self)
		d.Effects = append(d.Effects, e)
	}
	switch subject {
	case "hitpoints", "hit points", "hp", "health":
		add(SpellEffect{Kind: EffectHP})
		return verb
	case "hp per tic", "hitpoints per tic", "hp per tick":
		add(SpellEffect{Kind: EffectHPTick})
		return verb
	case "power":
		add(SpellEffect{Kind: EffectPower})
		return verb
	case "power per tic", "power per tick":
		add(SpellEffect{Kind: EffectPowerTick})
		return verb
	case "hate":
		add(SpellEffect{Kind: EffectHate})
		return verb
	case "movement speed":
		if sign < 0 {
			add(SpellEffect{Kind: EffectSnare})
		} else {
			add(SpellEffect{Kind: EffectSpeed})
		}
		return verb
	}
	// One stat or several: "STR & DEX", "AGI/STA", "FR & CR", "PR/DR", and
	// resists named once for all of them ("Resist Fire & Ice").
	var stats []Stat
	for i, w := range reSplit.Split(subject, -1) {
		st, ok := statWord(w)
		if !ok && i > 0 && strings.HasPrefix(subject, "resist ") {
			st, ok = statWord("resist " + w)
		}
		if !ok {
			d.Unread = append(d.Unread, part)
			return verb
		}
		stats = append(stats, st)
	}
	for _, st := range stats {
		add(SpellEffect{Kind: EffectStat, Stat: st})
	}
	return verb
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
