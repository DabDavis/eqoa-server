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
	// CharmMax is the highest level a charm takes when its line or
	// description says ("Charms up to level 54 mobs", "Charm up to level
	// 54"); 0: the spell's own level (charm.go).
	CharmMax int
	// Form is the body an illusion shows its bearer in, as the spell's name
	// gives it ("troll", "clay golem"), and FormKind whom it lands on
	// (illusion.go); "" and FormNone for every other spell.
	Form     string
	FormKind FormKind
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
// "Summuns") of a pet, or of a creature "to aid you" or "to your defense".
// The alchemists' "SELF Summons Unstable Potion" and Phase Ring's "Summons
// a ring to invis" make items, not pets, and say neither.
var (
	reSummons  = regexp.MustCompile(`\bsumm[ou]ns?\b`)
	reSummonOf = regexp.MustCompile(`\bsumm[ou]ns?\s+(?:a\s+|an\s+|the\s+)?(.+?)\s+(?:to aid you|to your defense|pet)\b`)
)

// summonLine reads a summoning line: whether it makes a pet, and the
// creature it names ("" for a bare "pet").
func summonLine(text string) (bool, string) {
	if !reSummons.MatchString(text) {
		return false, ""
	}
	if !strings.Contains(text, "pet") && !strings.Contains(text, "to aid you") && !strings.Contains(text, "to your defense") {
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
	reScale    = regexp.MustCompile(`^(?:increase|modified|mod)?\s*(\d+)\s*%\s*(?:based on|mod by|modified by|by)\s+([a-z]+)\.?$`)
	reScaleIn  = regexp.MustCompile(`\s+(?:modified|mod)\s+(\d+)\s*%\s*(?:based on|by)\s+([a-z]+)\w*$`)
	reBy       = regexp.MustCompile(`^(.*?)\s+by\s+(\d+)(?:\s+to\s+(\d+))?(?:\s+x\d+)?$`)
	reVerb     = regexp.MustCompile(`^(increase|decrease|buff|debuff|drain|raise|lower|heal)\s+(.*)$`)
	reTail     = regexp.MustCompile(`\s+(\d+(?:\.\d+)?)\s*(sec|secs|seconds|min|mins|minutes)$`)
	reHate     = regexp.MustCompile(`^\+\s*(\d+)\s*hate$`)
	reSplit    = regexp.MustCompile(`\s*(?:&|/|\band\b)\s*`)
)

// spellAliases rewrite a line's other ways of saying one thing.
var spellAliases = []struct{ from, to string }{
	{"decrease drain ", "drain "},
	{"hp via backstab by", "hp by"},
	{"damage by ", "decrease hitpoints by "},
}

// statWords are the stat names a line uses, longest first where one starts
// another.
var statWords = []struct {
	word string
	stat Stat
}{
	{"maximum hp", StatMaxHP}, {"max hp", StatMaxHP}, {"maxhp", StatMaxHP},
	{"maximum power", StatMaxPower}, {"max power", StatMaxPower}, {"maxpower", StatMaxPower},
	{"str", StatStr}, {"sta", StatSta}, {"agi", StatAgi}, {"dex", StatDex},
	{"wis", StatWis}, {"int", StatInt}, {"cha", StatCha}, {"ac", StatAC},
	{"fr", StatFR}, {"cr", StatCR}, {"pr", StatPR}, {"dr", StatDR}, {"lr", StatLR}, {"ar", StatAR},
	{"resist arcane", StatAR}, {"resist fire", StatFR}, {"resist cold", StatCR},
	{"resist poison", StatPR}, {"resist disease", StatDR}, {"resist lightning", StatLR},
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
		if e.Kind == EffectHP && e.Amount < 0 {
			at = i
			break
		}
	}
	if at < 0 {
		return d
	}
	hit := d.Effects[at]
	effects := append([]SpellEffect(nil), d.Effects...)
	effects[at] = SpellEffect{Kind: EffectProc, Amount: -hit.AmountMax, AmountMax: -hit.Amount}
	d.Effects = effects
	d.Harmful = false
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick, EffectStat:
			d.Harmful = d.Harmful || e.Amount < 0
		case EffectRoot, EffectSnare, EffectHate:
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
		return d
	}
	// An area spell's line names the target's share first: "Decrease
	// Hitpoints by 210 aoe Decrease Hitpoints by 90".
	if i := strings.Index(text, " aoe "); i >= 0 {
		end := strings.IndexAny(text[i:], ",;")
		if end < 0 {
			text = text[:i]
		} else {
			text = text[:i] + text[i+end:]
		}
	}
	for _, a := range spellAliases {
		text = strings.ReplaceAll(text, a.from, a.to)
	}
	// The verb a clause without one goes on with ("Increase AC by 12,
	// Maximum Power by 5"), and the effect a scale clause belongs to.
	verb := ""
	last := -1
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
				if st, ok := statWord(m[2]); ok && last >= 0 {
					d.Effects[last].Scale = st
					d.Effects[last].ScalePct, _ = strconv.Atoi(m[1])
					continue
				}
			}
			n := len(d.Effects)
			verb = d.parsePart(part, verb)
			if len(d.Effects) > n {
				last = len(d.Effects) - 1
			}
		}
	}
	// Cast at an enemy: what takes hit points or a stat, holds or slows, or
	// works on hate. Power is not counted: "Decrease Power by 1" is in Minor
	// Healing's line.
	for _, e := range d.Effects {
		switch e.Kind {
		case EffectHP, EffectHPTick, EffectStat:
			if e.Amount < 0 {
				d.Harmful = true
			}
		case EffectRoot, EffectSnare, EffectHate, EffectCharm:
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
	return d
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
	// A drain's "and transfer to caster" / "give self": the caster's share
	// is not acted on yet; the target's is.
	for _, tail := range []string{" and transfer to caster", " and give to self", " gives to self", " give self"} {
		part = strings.TrimSuffix(part, tail)
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
			v := d.parsePart(pieces[0], verb)
			return d.parsePart(pieces[1], v)
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
	// One stat or several: "STR & DEX", "AGI/STA", "FR & CR", "PR/DR".
	var stats []Stat
	for _, w := range reSplit.Split(subject, -1) {
		st, ok := statWord(w)
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
