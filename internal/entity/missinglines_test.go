package entity

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The lines tools/fix_missing_effect_lines.py gives the class spells that had
// none (jib's review, F3 pass 19), out of tools/missing_effect_lines.json,
// read as the server reads them at a cast (ParseSpellNamed, with the spell's
// name and its record's description) and held to what each line's words say:
// whom it is cast at, every effect with its amount, how long it lasts, whether
// it is cast at an enemy, the pet it calls, and the words the reader leaves
// unread, which the file names with the reason.

// missingLinesFile is the tool's data, from this package's directory.
const missingLinesFile = "../../tools/missing_effect_lines.json"

type missingLine struct {
	Classes []int  `json:"classes"`
	Name    string `json:"name"`
	Level   int    `json:"level"`
	Line    string `json:"line"`
	Desc    string `json:"desc"`
	Source  string `json:"source"`
	Expect  struct {
		Target  string   `json:"target"`
		Harmful bool     `json:"harmful"`
		Seconds float64  `json:"seconds"`
		Ticks   int      `json:"ticks"`
		Pet     string   `json:"pet"`
		Effects []string `json:"effects"`
	} `json:"expect"`
	Unread []struct {
		Part string `json:"part"`
		Why  string `json:"why"`
	} `json:"unread"`
}

type missingLines struct {
	Lines []missingLine `json:"lines"`
	None  []struct {
		Classes []int  `json:"classes"`
		Name    string `json:"name"`
		Level   int    `json:"level"`
		Why     string `json:"why"`
	} `json:"none"`
}

func readMissingLines(t *testing.T) missingLines {
	t.Helper()
	data, err := os.ReadFile(missingLinesFile)
	if err != nil {
		t.Fatal(err)
	}
	var f missingLines
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("%s: %v", missingLinesFile, err)
	}
	if len(f.Lines) == 0 {
		t.Fatalf("%s holds no lines", missingLinesFile)
	}
	return f
}

// lineStatWord is a stat as the expectations write it.
var lineStatWord = map[Stat]string{
	StatStr: "str", StatSta: "sta", StatAgi: "agi", StatDex: "dex", StatWis: "wis", StatInt: "int",
	StatCha: "cha", StatAC: "ac", StatMaxHP: "maxhp", StatMaxPower: "maxpower",
	StatFR: "fr", StatCR: "cr", StatPR: "pr", StatDR: "dr", StatLR: "lr", StatAR: "ar",
}

// effectWords is one read effect as the expectations write it: "hp -10",
// "hptick -2", "str +15", "power +50", "root", "summon"; a range as
// "hp -60..-50", a scale as " +25% cha", the caster's own as " self".
func effectWords(e SpellEffect) string {
	amount := func(kind string) string {
		s := fmt.Sprintf("%s %+d", kind, e.Amount)
		if e.AmountMax != e.Amount {
			s += fmt.Sprintf("..%+d", e.AmountMax)
		}
		return s
	}
	var s string
	switch e.Kind {
	case EffectHP:
		s = amount("hp")
	case EffectHPTick:
		s = amount("hptick")
	case EffectPower:
		s = amount("power")
	case EffectPowerTick:
		s = amount("powertick")
	case EffectStat:
		s = amount(lineStatWord[e.Stat])
	case EffectHate:
		s = amount("hate")
	case EffectProc:
		s = amount("proc")
	case EffectRoot:
		s = "root"
	case EffectSnare:
		s = "snare"
	case EffectSpeed:
		s = "speed"
	case EffectInvis:
		s = "invis"
	case EffectGate:
		s = "gate"
	case EffectSummon:
		s = "summon"
	case EffectCharm:
		s = "charm"
	case EffectIllusion:
		s = "illusion"
	case EffectEndIllusion:
		s = "end illusion"
	case EffectShield:
		s = amount("shield")
	case EffectDefProc:
		s = amount("defproc")
	case EffectResurrect:
		s = amount("rez")
	case EffectMark:
		s = amount("mark")
	case EffectMarkHeal:
		s = amount("markheal")
	case EffectCure:
		s = "cure"
		for _, k := range []struct {
			bit  CureKind
			word string
		}{{CurePoison, " poison"}, {CureDisease, " disease"}, {CureAny, " any"}} {
			if e.Cure&k.bit != 0 {
				s += k.word
			}
		}
		if e.Amount > 0 {
			s += fmt.Sprintf(" x%d", e.Amount)
		}
	case EffectHPCap:
		s = amount("hpcap")
	case EffectInvuln:
		s = "invulnerable"
	default:
		s = fmt.Sprintf("kind %d", e.Kind)
	}
	if e.Scale != StatNone && e.ScalePct != 0 {
		s += fmt.Sprintf(" +%d%% %s", e.ScalePct, lineStatWord[e.Scale])
	}
	if e.Self {
		s += " self"
	}
	if e.Drain != DrainNone {
		s += " drain"
	}
	return s
}

var lineTargetWord = map[SpellTarget]string{
	TargetSingle: "target", TargetSelf: "self", TargetGroup: "group", TargetPet: "pet", TargetUndead: "target undead",
}

// TestMissingEffectLinesRead reads every line the tool writes and holds it to
// its words.
func TestMissingEffectLinesRead(t *testing.T) {
	f := readMissingLines(t)
	for _, l := range f.Lines {
		name := fmt.Sprintf("%s (L%d, classes %v)", l.Name, l.Level, l.Classes)
		if strings.TrimSpace(l.Line) == "" || strings.TrimSpace(l.Source) == "" || len(l.Classes) == 0 || l.Level <= 0 {
			t.Errorf("%s: a line, its source, its classes and its level are all needed", name)
			continue
		}
		d := ParseSpellNamed(l.Name, l.Line, l.Desc)
		var got []string
		for _, e := range d.Effects {
			got = append(got, effectWords(e))
		}
		want := l.Expect.Effects
		if !reflect.DeepEqual(got, want) && (len(got) > 0 || len(want) > 0) {
			t.Errorf("%s: %q reads as %q, want %q", name, l.Line, got, want)
		}
		if w := lineTargetWord[d.Target]; w != l.Expect.Target {
			t.Errorf("%s: %q is cast at %q, want %q", name, l.Line, w, l.Expect.Target)
		}
		if d.Harmful != l.Expect.Harmful {
			t.Errorf("%s: %q harmful %v, want %v", name, l.Line, d.Harmful, l.Expect.Harmful)
		}
		if want := time.Duration(l.Expect.Seconds * float64(time.Second)); d.Duration != want {
			t.Errorf("%s: %q lasts %v, want %v", name, l.Line, d.Duration, want)
		}
		if d.Ticks != l.Expect.Ticks {
			t.Errorf("%s: %q ticks %d times, want %d", name, l.Line, d.Ticks, l.Expect.Ticks)
		}
		if d.Pet != l.Expect.Pet {
			t.Errorf("%s: %q calls %q, want %q", name, l.Line, d.Pet, l.Expect.Pet)
		}
		var unread []string
		for _, u := range l.Unread {
			if strings.TrimSpace(u.Why) == "" {
				t.Errorf("%s: unread %q says no why", name, u.Part)
			}
			unread = append(unread, u.Part)
		}
		if !reflect.DeepEqual(d.Unread, unread) && (len(d.Unread) > 0 || len(unread) > 0) {
			t.Errorf("%s: %q leaves %q unread, the file says %q", name, l.Line, d.Unread, unread)
		}
	}
}

// TestMissingEffectLinesOnce: a spell is in the file once, with a line or in
// the list of those no source covers, never both.
func TestMissingEffectLinesOnce(t *testing.T) {
	f := readMissingLines(t)
	seen := map[string]string{}
	key := func(classes []int, name string, level int) []string {
		var ks []string
		for _, c := range classes {
			ks = append(ks, fmt.Sprintf("%d/%s/%d", c, strings.ToLower(strings.TrimSpace(name)), level))
		}
		return ks
	}
	for _, l := range f.Lines {
		for _, k := range key(l.Classes, l.Name, l.Level) {
			if seen[k] != "" {
				t.Errorf("%s twice", k)
			}
			seen[k] = "line"
		}
	}
	for _, n := range f.None {
		if strings.TrimSpace(n.Why) == "" {
			t.Errorf("%s: no line and no why", n.Name)
		}
		for _, k := range key(n.Classes, n.Name, n.Level) {
			if seen[k] != "" {
				t.Errorf("%s: in the file with a line and without one", k)
			}
			seen[k] = "none"
		}
	}
}

// TestMissingLinesToolKeepsTheProcWords: the tool holds a line whose record's
// description names a proc, as the reader would take its hit for one
// (reProcDesc); its words must be the reader's.
func TestMissingLinesToolKeepsTheProcWords(t *testing.T) {
	src, err := os.ReadFile("../../tools/fix_missing_effect_lines.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`PROC_DESC = re\.compile\(r"([^"]*)"\)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("tools/fix_missing_effect_lines.py has no PROC_DESC")
	}
	if string(m[1]) != reProcDesc.String() {
		t.Errorf("the tool's PROC_DESC is %q, spelltext.go's reProcDesc %q", m[1], reProcDesc.String())
	}
}
