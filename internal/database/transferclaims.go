package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// What a player says a character had done on the other server besides its
// gear (the user, 2026-11-05: "add path, were or vamp, flags completed,
// quests, etc. to the form"): the quests of this server it names, picked off
// TransferCatalog's lists for its class, and the rest in its own words. It is
// the player's word: a GM's plan (charmove.Plan.Quests, Vampire) is what
// applies.

// TransferClaims is a transfer's claims column.
type TransferClaims struct {
	// Picks are the options taken at a quest's choices (a path, the epic),
	// each saying its quest was done.
	Picks []ClaimPick `json:"picks,omitempty"`
	// Quests are other quests done: a were line's or the vampire's, a flag.
	Quests []int `json:"quests,omitempty"`
	// Line is the were line it says it was, or "vampire" ("" neither).
	Line string `json:"line,omitempty"`
	// Other is the quests it names in its own words.
	Other string `json:"other,omitempty"`
}

// ClaimPick is one option taken: of a quest's step's choice, from 0.
type ClaimPick struct {
	Quest  int `json:"quest"`
	Step   int `json:"step"`
	Option int `json:"option"`
}

// The limits of the claims.
const (
	TransferMaxClaims      = 8000 // the column, bytes
	TransferMaxOther       = 2000 // the player's own words
	TransferMaxClaimQuests = 200
	TransferMaxClaimPicks  = 24
)

// TransferLines are the lines a character may say it was, and their labels.
// The werehunter's is a line too (the user, 2026-11-06: "the werehunter doesnt
// show up for the transfer form"; Larrybird's Heroic Were-Hunter, which needs
// its brand, was missed for want of it): its quests are not a were line's
// parts (p1, L40) but their own steps (reCatWerehunter), and the class's
// trials and ability quest are only its archetype's (questForClass).
var TransferLines = []struct{ Line, Label string }{
	{"wolfwere", "Wolfwere"}, {"bearwere", "Bearwere"}, {"ratwere", "Ratwere"},
	{"lionwere", "Lionwere"}, {"gatorwere", "Gatorwere"}, {"werehunter", "Werehunter"},
	{"vampire", "Vampire"},
}

// werehunterOrder is the werehunter line's quests in the order they are done
// (each needs the one before it: Silver and Fur, the Werehunter's Brand, the
// Siloths hindered, the Siloth Shipment, Lord Phinnius hindered, the ability
// quest, the Token, the Tails, the Ring, Elder Tchekof, the Trials); a step
// it does not know goes last.
func werehunterOrder(step string) int {
	switch {
	case step == "p1":
		return 1
	case step == "brand":
		return 2
	case step == "hinder":
		return 3
	case step == "p2":
		return 4
	case step == "trials-p1":
		return 5
	case strings.HasPrefix(step, "ability-"):
		return 6
	case step == "trials-insignia":
		return 7
	case step == "trials-set":
		return 8
	case step == "trials-ring":
		return 9
	case step == "tchekof":
		return 10
	case strings.HasPrefix(step, "trials-"):
		return 11
	}
	return 100
}

func transferLine(line string) bool {
	for _, l := range TransferLines {
		if l.Line == line {
			return true
		}
	}
	return line == ""
}

// Empty is whether it claims nothing.
func (c TransferClaims) Empty() bool {
	return len(c.Picks) == 0 && len(c.Quests) == 0 && c.Line == "" && c.Other == ""
}

// ParseTransferClaims reads a claims column ("" is none), its shape checked:
// a known line, positive quest ids, a pick's step from 1 and option from 0 to
// MaxStepOptions-1, each quest and each step's pick once, within the limits.
func ParseTransferClaims(s string) (TransferClaims, error) {
	var c TransferClaims
	if strings.TrimSpace(s) == "" {
		return c, nil
	}
	if len(s) > TransferMaxClaims {
		return c, fmt.Errorf("the quests are %d bytes at most", TransferMaxClaims)
	}
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return TransferClaims{}, errors.New("the quests could not be read")
	}
	return c.normal()
}

// normal is c checked and tidied (sorted, each once, the words as kept).
func (c TransferClaims) normal() (TransferClaims, error) {
	c.Line = strings.TrimSpace(c.Line)
	if !transferLine(c.Line) {
		return TransferClaims{}, fmt.Errorf("no line %q", c.Line)
	}
	c.Other = transferText(c.Other)
	if utf8.RuneCountInString(c.Other) > TransferMaxOther {
		return TransferClaims{}, fmt.Errorf("the other quests are %d characters at most", TransferMaxOther)
	}
	if len(c.Quests) > TransferMaxClaimQuests || len(c.Picks) > TransferMaxClaimPicks {
		return TransferClaims{}, fmt.Errorf("at most %d quests and %d picks", TransferMaxClaimQuests, TransferMaxClaimPicks)
	}
	seen := map[int]bool{}
	var quests []int
	for _, q := range c.Quests {
		if q <= 0 {
			return TransferClaims{}, fmt.Errorf("no quest %d", q)
		}
		if !seen[q] {
			seen[q] = true
			quests = append(quests, q)
		}
	}
	sort.Ints(quests)
	c.Quests = quests
	picked := map[[2]int]bool{}
	var picks []ClaimPick
	for _, p := range c.Picks {
		if p.Quest <= 0 || p.Step < 1 || p.Option < 0 || p.Option >= MaxStepOptions {
			return TransferClaims{}, fmt.Errorf("no option %d of quest %d's step %d", p.Option, p.Quest, p.Step)
		}
		if k := [2]int{p.Quest, p.Step}; !picked[k] {
			picked[k] = true
			picks = append(picks, p)
		}
	}
	sort.Slice(picks, func(i, j int) bool {
		if picks[i].Quest != picks[j].Quest {
			return picks[i].Quest < picks[j].Quest
		}
		return picks[i].Step < picks[j].Step
	})
	c.Picks = picks
	return c, nil
}

// FormatTransferClaims is c as the claims column keeps it ("" for none).
func FormatTransferClaims(c TransferClaims) string {
	if c.Empty() {
		return ""
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// TransferCatalog is what the transfer page offers a character of a class to
// say it had done: its path's choices (levels 20, 30, 40, 49) and its epic's
// (60), the were lines and the vampire's, the flags (the Portal quests); only
// quests in the game that the class may take.
type TransferCatalog struct {
	Class   int             `json:"class"`
	Choices []CatalogChoice `json:"choices"`
	Lines   []CatalogLine   `json:"lines"`
	Flags   []CatalogGroup  `json:"flags"`
}

// CatalogChoice is one step's choice of a path's or the epic's quest. A
// quest asking two (the 49 path's ability and weapon) is two.
type CatalogChoice struct {
	Kind    string          `json:"kind"`  // "path" or "epic"
	Level   int             `json:"level"` // 20, 30, 40, 49; 60 the epic
	Quest   int             `json:"quest"`
	Name    string          `json:"name"`
	Step    int             `json:"step"`
	Ask     string          `json:"ask"`
	Options []CatalogOption `json:"options"`
	// Also is the class's other quests asking the same (another city's).
	Also []int `json:"also,omitempty"`
	// Chains is, for Quest and each of Also, the quests it needs done first
	// of its own guide (a path's or the epic's earlier quests), which its
	// claim says were done too.
	Chains map[int][]int `json:"chains,omitempty"`
}

// CatalogOption is an option of a choice, by its place among the step's
// options (charquests' picks count every option; one that does not do the
// step is not offered).
type CatalogOption struct {
	Option int    `json:"option"`
	Answer string `json:"answer"`
}

// CatalogLine is a were line's or the vampire's quests, in their order.
type CatalogLine struct {
	Line   string         `json:"line"`
	Label  string         `json:"label"`
	Quests []CatalogQuest `json:"quests"`
}

// CatalogGroup is a place's flags.
type CatalogGroup struct {
	Place  string         `json:"place"`
	Label  string         `json:"label"`
	Quests []CatalogQuest `json:"quests"`
}

// CatalogQuest is one quest offered.
type CatalogQuest struct {
	Quest int    `json:"quest"`
	Name  string `json:"name"`
}

// TransferClassSlugs are the classes (by id) as the quest import's keys
// spell them.
var TransferClassSlugs = []string{"warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid",
	"shaman", "cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"}

var (
	reCatL20  = regexp.MustCompile(`^1-20/[a-z0-9-]+/([a-z]+)/L20$`)
	reCatPath = regexp.MustCompile(`^30,40,49/[a-z0-9-]+/L(30|40|49)-([a-z]+)$`)
	reCatEpic = regexp.MustCompile(`^60/[a-z0-9-]+/reward-([a-z]+)$`)
	reCatWere = regexp.MustCompile(`^Weres/([a-z]+)/(p\d+|L\d+)(?:-[a-z]+)?$`)
	// the werehunter's steps: p1, brand, ..., trials-tank, ability-healer
	reCatWerehunter = regexp.MustCompile(`^Weres/werehunter/([a-z0-9]+(?:-[a-z0-9]+)?)$`)
	reCatFlags      = regexp.MustCompile(`^Portal/([a-z0-9-]+)/[a-z0-9-]+$`)
)

// QuestImportKey is a quest's import key off its source ("EQOA-Data
// 1-20/freeport/warrior/L20 (tools/import_quests.py)"), or "".
func QuestImportKey(source string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(source), "EQOA-Data ")
	if !ok || !strings.HasSuffix(rest, "(tools/import_quests.py)") {
		return ""
	}
	key, _, _ := strings.Cut(rest, " ")
	return key
}

// questForClass is whether a class may take q (no classes: any).
func questForClass(q *Quest, class int) bool {
	if len(q.Classes) == 0 {
		return true
	}
	for _, c := range q.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// placeLabel is a place's slug as words: "plane-of-sky" "Plane of Sky".
func placeLabel(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		if i > 0 && (w == "of" || w == "the") {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// partOrder sorts a line's parts: p1, p2, p3, then L40, L50, L60.
func partOrder(part string) int {
	n, _ := strconv.Atoi(part[1:])
	if part[0] == 'p' {
		return n
	}
	return 1000 + n
}

// TransferCatalog is the catalog for a class, off the quests as they are.
func (db *SQLiteDB) TransferCatalog(class int) (TransferCatalog, error) {
	cat := TransferCatalog{Class: class, Choices: []CatalogChoice{}, Lines: []CatalogLine{}, Flags: []CatalogGroup{}}
	if class < 0 || class >= len(TransferClassSlugs) {
		return cat, fmt.Errorf("no class %d", class)
	}
	slug := TransferClassSlugs[class]
	quests, err := NewQuestStore(db).LoadQuests()
	if err != nil {
		return cat, err
	}
	byID := map[int]*Quest{}
	keys := map[int]string{}
	for i := range quests {
		byID[quests[i].ID] = &quests[i]
		keys[quests[i].ID] = QuestImportKey(quests[i].Source)
	}
	// chain is the quests q needs done first of its own guide (the key's
	// folder), each once, in id order.
	chain := func(q *Quest) []int {
		folder := keys[q.ID]
		if i := strings.LastIndex(folder, "/"); i >= 0 {
			folder = folder[:i+1]
		}
		seen := map[int]bool{q.ID: true}
		var out []int
		todo := []int{q.ID}
		for len(todo) > 0 {
			cur := byID[todo[0]]
			todo = todo[1:]
			if cur == nil {
				continue
			}
			for _, group := range cur.Required() {
				for _, id := range group {
					if seen[id] || byID[id] == nil || !strings.HasPrefix(keys[id], folder) {
						continue
					}
					seen[id] = true
					out = append(out, id)
					todo = append(todo, id)
				}
			}
		}
		sort.Ints(out)
		return out
	}
	type part struct {
		order int
		q     CatalogQuest
	}
	lines := map[string][]part{}
	flags := map[string][]CatalogQuest{}
	same := map[string]int{} // a choice asked again (another city's) -> its index
	for i := range quests {
		q := &quests[i]
		if !q.Enabled || !questForClass(q, class) {
			continue
		}
		key := keys[q.ID]
		kind, level := "", 0
		if m := reCatL20.FindStringSubmatch(key); m != nil && m[1] == slug {
			kind, level = "path", 20
		} else if m := reCatPath.FindStringSubmatch(key); m != nil && m[2] == slug {
			kind, level = "path", map[string]int{"30": 30, "40": 40, "49": 49}[m[1]]
		} else if m := reCatEpic.FindStringSubmatch(key); m != nil && m[1] == slug {
			kind, level = "epic", 60
		} else if m := reCatWerehunter.FindStringSubmatch(key); m != nil {
			lines["werehunter"] = append(lines["werehunter"], part{werehunterOrder(m[1]), CatalogQuest{q.ID, q.Name}})
			continue
		} else if m := reCatWere.FindStringSubmatch(key); m != nil && transferLine(m[1]) {
			lines[m[1]] = append(lines[m[1]], part{partOrder(m[2]), CatalogQuest{q.ID, q.Name}})
			continue
		} else if m := reCatFlags.FindStringSubmatch(key); m != nil {
			flags[m[1]] = append(flags[m[1]], CatalogQuest{q.ID, q.Name})
			continue
		} else {
			continue
		}
		for _, st := range q.Steps {
			if len(st.Choices.Options) == 0 {
				continue
			}
			var opts []CatalogOption
			for n, o := range st.Choices.Options {
				if !o.Stay {
					opts = append(opts, CatalogOption{n, o.Answer})
				}
			}
			if len(opts) == 0 {
				continue
			}
			sig := fmt.Sprint(kind, level, st.Choices.Ask, opts)
			if at, ok := same[sig]; ok {
				cat.Choices[at].Also = append(cat.Choices[at].Also, q.ID)
				if c := chain(q); len(c) > 0 {
					cat.Choices[at].Chains[q.ID] = c
				}
				continue
			}
			same[sig] = len(cat.Choices)
			chains := map[int][]int{}
			if c := chain(q); len(c) > 0 {
				chains[q.ID] = c
			}
			cat.Choices = append(cat.Choices, CatalogChoice{Kind: kind, Level: level, Quest: q.ID, Name: q.Name, Step: st.Step,
				Ask: st.Choices.Ask, Options: opts, Chains: chains})
		}
	}
	sort.SliceStable(cat.Choices, func(i, j int) bool {
		a, b := cat.Choices[i], cat.Choices[j]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		if a.Quest != b.Quest {
			return a.Quest < b.Quest
		}
		return a.Step < b.Step
	})
	for _, l := range TransferLines {
		ps := lines[l.Line]
		if len(ps) == 0 {
			continue
		}
		sort.SliceStable(ps, func(i, j int) bool {
			if ps[i].order != ps[j].order {
				return ps[i].order < ps[j].order
			}
			return ps[i].q.Quest < ps[j].q.Quest
		})
		line := CatalogLine{Line: l.Line, Label: l.Label}
		for _, p := range ps {
			line.Quests = append(line.Quests, p.q)
		}
		cat.Lines = append(cat.Lines, line)
	}
	var places []string
	for p := range flags {
		places = append(places, p)
	}
	sort.Slice(places, func(i, j int) bool { return placeLabel(places[i]) < placeLabel(places[j]) })
	for _, p := range places {
		qs := flags[p]
		sort.Slice(qs, func(i, j int) bool { return qs[i].Quest < qs[j].Quest })
		cat.Flags = append(cat.Flags, CatalogGroup{Place: p, Label: placeLabel(p), Quests: qs})
	}
	return cat, nil
}

// ApproximatedItems is the names of the records a transfer's plan makes for
// items this server lacks (charmove's new_records), in its order: what the
// player is asked to report with their real figures. Read off the plan's
// JSON, so this package needs none of charmove.
func ApproximatedItems(plan string) []string {
	var p struct {
		NewRecords []struct {
			Set map[string]any `json:"set"`
		} `json:"new_records"`
	}
	if json.Unmarshal([]byte(plan), &p) != nil {
		return nil
	}
	var out []string
	for _, n := range p.NewRecords {
		if name, ok := n.Set["itemname"].(string); ok && strings.TrimSpace(name) != "" {
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}
