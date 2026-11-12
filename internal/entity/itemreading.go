package entity

import (
	"fmt"
	"strings"
)

// itemreading.go — what an item record's combat columns mean, said by the code
// that reads them, for the GM panel's item editor (internal/gmweb) and its AI
// agents. The owner, 2026-11-11: "for attack type in gm menu for weapons 1hb is
// 3 etc. lets put that also like Pierce(4) and any other such QoL for the item
// editor. also for proc number and proc damage for items." The panel builds
// its labels from these, so a label cannot say what this code does not do.

// AttackTypeWord is one of CLIENT's attack types (VIItem m_attackType,
// itempattern.attacktype) with the word its item window shows for it: the
// table gAttackWords (CLIENT 0x00CDF6A0, read out of a live game's EE memory,
// memory-dumps/inworld-eqoa.live.eememory: {WORDS.TXT id, type} pairs) and the
// disc's WORDS.TXT line of that id (VIInfoText::AddAttackType, "ATT: 1HB").
// Name is ours, the word written out.
type AttackTypeWord struct {
	Value int
	Word  string // CLIENT's, as the item window shows it
	Name  string
}

// AttackTypeWords are CLIENT's eleven, 0 to 10. A value past them has no word:
// the item window shows "ATT: <number>".
var AttackTypeWords = []AttackTypeWord{
	{0, "NONE", "None (not a weapon)"},
	{1, "1HS", "1H Slash"},
	{2, "2HS", "2H Slash"},
	{3, "1HB", "1H Blunt"},
	{4, "2HB", "2H Blunt"},
	{5, "1HP", "1H Pierce"},
	{6, "2HP", "2H Pierce"},
	{7, "BOW", "Bow"},
	{8, "1H CROSSBOW", "1H Crossbow"},
	{9, "2H CROSSBOW", "2H Crossbow"},
	{10, "THROWN", "Thrown"},
}

// AttackTypeEmptyHand is CLIENT's attack type of a hand that holds nothing
// (VICSprite's default and DetachItem's, 11): no item word.
const AttackTypeEmptyHand = int(attackTypeEmptyHand)

// MeleeKindOf is the melee resist a hit of a weapon of that attack type is
// taken by (StatSlash, StatPierce or StatBlunt): meleeKindOf.
func MeleeKindOf(attackType int) Stat {
	if attackType < 0 || attackType > 255 {
		return StatBlunt
	}
	return meleeKindOf(byte(attackType))
}

// AttackTypeReading is what the server does with an item of that attack type in
// a hand, in plain words: how far it reaches (SwingFor: CLIENT's
// TargetInRange), the melee kind its hits are, and whether CLIENT's item window
// calls it ranged (VIInfoText::IsRanged: 7 to 10, whose RNG it shows).
func AttackTypeReading(attackType int) string {
	s := SwingFor(0, 1, [21]bool{}, true, attackType, -1)
	var b strings.Builder
	switch {
	case s.Never:
		b.WriteString("never swings (no reach)")
	case s.Reach < 0:
		b.WriteString("reaches its Range (unk5)")
	default:
		fmt.Fprintf(&b, "reaches %.1f", s.Reach)
	}
	if !s.Never {
		fmt.Fprintf(&b, "; its hits are %s", strings.ToLower(StatName(MeleeKindOf(attackType))))
	}
	if attackType >= 7 && attackType <= 10 {
		b.WriteString("; ranged (the item window shows its RNG)")
	}
	return b.String()
}

// StatName is a stat's short name as the panel says it.
func StatName(st Stat) string {
	switch st {
	case StatFR:
		return "Fire"
	case StatCR:
		return "Cold"
	case StatPR:
		return "Poison"
	case StatDR:
		return "Disease"
	case StatLR:
		return "Lightning"
	case StatAR:
		return "Arcane"
	case StatPierce:
		return "Pierce"
	case StatBlunt:
		return "Blunt"
	case StatSlash:
		return "Slash"
	case StatStr:
		return "STR"
	case StatSta:
		return "STA"
	case StatAgi:
		return "AGI"
	case StatDex:
		return "DEX"
	case StatWis:
		return "WIS"
	case StatInt:
		return "INT"
	case StatCha:
		return "CHA"
	}
	return fmt.Sprintf("stat %d", int(st))
}

// WeaponProcChanceRule is the chance a weapon proc of no chance of its own
// fires, in words (weaponProcChancePct).
func WeaponProcChanceRule() string {
	return fmt.Sprintf("%d%% a landed hit, 1 more for each %d DEX of the wielder, %d%% at most",
		weaponProcBasePct, weaponProcDexPer, weaponProcMostPct)
}

// Explain is a weapon proc in plain words: what it does, how often, what the
// creature resists it by.
func (wp WeaponProc) Explain() string {
	figure := fmt.Sprint(wp.Damage)
	if wp.DamageTo > wp.Damage {
		figure = fmt.Sprintf("%d-%d", wp.Damage, wp.DamageTo)
	}
	var what string
	switch wp.Kind {
	case ProcHit:
		what = fmt.Sprintf("hits for %s %s damage", figure, strings.ToLower(StatName(wp.Resist)))
	case ProcTap:
		what = fmt.Sprintf("taps %s (%s): the damage landed comes back to the wielder as health", figure, strings.ToLower(StatName(wp.Resist)))
	case ProcDoT:
		what = fmt.Sprintf("lays %s %s damage every 6 s for %.0f s", figure, strings.ToLower(StatName(wp.Resist)), procDoTFor.Seconds())
	case ProcCalm:
		what = "halves the creature's hate for the wielder"
	case ProcRestore:
		what = fmt.Sprintf("gives %s health and %d power to the wielder and its group within %d", figure, wp.Power, procRestoreReach)
	case ProcForm:
		if wp.Scale != StatNone && wp.ScalePct != 0 {
			figure += fmt.Sprintf(" + %d%% of %s", wp.ScalePct, procStatWord(wp.Scale))
		}
		if wp.Resist != StatNone {
			figure += " " + strings.ToLower(StatName(wp.Resist))
		}
		form := wp.Form
		if wp.FormStat != StatNone && wp.FormStatBy != 0 {
			form += fmt.Sprintf(" (%+d %s while it lasts)", wp.FormStatBy, procStatWord(wp.FormStat))
		}
		what = fmt.Sprintf("turns the wielder into %s for %.0f s (renewed by another proc), which procs %s at %d%% a hit, at most %d a round",
			form, wp.Duration.Seconds(), figure, wp.Chance, wp.RoundMost)
	case ProcRoot:
		what = fmt.Sprintf("roots the creature for %.0f s (a root breaks as any root does)", procRootFor.Seconds())
	}
	if wp.Kind != ProcForm {
		if wp.Chance > 0 {
			what += fmt.Sprintf(", %d%% a landed hit (its own chance, any DEX)", wp.Chance)
		} else {
			what += ", " + WeaponProcChanceRule()
		}
	}
	if wp.Only != "" {
		what += ", on " + wp.Only + "s alone"
	}
	switch wp.Kind {
	case ProcHit, ProcTap, ProcDoT, ProcCalm, ProcRoot:
		what += fmt.Sprintf("; a creature resists it all or nothing, %d%% at the wielder's level and more above (a resisted hit does its least)", resistAtPar)
	}
	return what
}

// Explain is a worn item's answer to each melee hit its wearer takes, in plain
// words (forWearer).
func (s WornShield) Explain() string {
	var parts []string
	if s.Damage > 0 {
		d := fmt.Sprint(s.Damage)
		if s.DamageTo > s.Damage {
			d = fmt.Sprintf("%d-%d", s.Damage, s.DamageTo)
		}
		if s.Line && s.DamageMax > s.Damage {
			d = fmt.Sprintf("%d (at no %s) to %d (at the wearer's %s cap)", s.Damage, StatName(s.Scale), s.DamageMax, StatName(s.Scale))
		}
		on := ""
		if s.Undead {
			on = " on undead alone"
		}
		parts = append(parts, fmt.Sprintf("%s damage back to the creature%s, never resisted", d, on))
	}
	if s.Heal > 0 {
		h := fmt.Sprint(s.Heal)
		switch {
		case s.Line && s.HealMax > s.Heal:
			h = fmt.Sprintf("%d (at no %s) to %d (at the cap)", s.Heal, StatName(s.Scale), s.HealMax)
		case s.HealPct > 0:
			h = fmt.Sprintf("%d + %d%% of %s", s.Heal, s.HealPct, StatName(s.Scale))
			if s.HealMax > 0 {
				h += fmt.Sprintf(", %d at most", s.HealMax)
			}
		}
		parts = append(parts, h+" health back to the wearer")
	}
	if s.Power > 0 {
		parts = append(parts, fmt.Sprintf("%d power back to the wearer", s.Power))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	out := "each melee hit the wearer takes: " + strings.Join(parts, "; ")
	if s.Hate {
		out += "; the heal is hate on the creature that hit"
	}
	return out
}
