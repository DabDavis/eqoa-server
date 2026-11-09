package handlers

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// archivistpaths.go — the Spell Archivist's paths (the GM Jibrish's petition
// 1325, 2026-11-09: "The spell archivist should also give the path spells as
// an option. Offer one path at a time to the class and if they pick the other
// swap it. Essentially they only get 1 path at a time unless they do the
// quests manually. If an enchanter goes self power it should remove the power
// for others line, and if they switch back it removes self power and gives
// power for others.").
//
// A class's path spells are what its path reward quests give at levels 30,
// 40 and 49 (30,40,49/<group>/L<30|40|49>-<class>, rehabpaths.go's rePathKey;
// a class's reward is in one group or two, the same spells in each): a talk
// step's choice of options, an ability (and a weapon, which the archivist
// leaves to the quests), and at the cleric's 49 a step's own spell every pick
// gets (First Aid); and, since petition 1361, the level 20 class quest's
// choice (1-20/<city>/<class>/L20, rehabpaths.go's reL20Key): a scroll of
// each path's first spell (Spacious Mind, Power Boon), whose spell is read off
// it as the scribe reads one, of every city whose quest asks the choice (one
// that kept a single path asks none). Nothing in the data says which options
// are one path: the lines are archivistPathLines', an ability carried up the
// levels (the enchanter's Spacious, Expansive, Limitless, Boundless and
// Infinite Mind, its self power; Power Boon, Power Jet, Power Surge, Power
// Stream and Power Flux, power for others), and
// TestRealArchivistPathsAreTheQuests holds them to the quests. A path is a line: its spells of every level its holder has reached
// (from the first any path gives, 20 on the live data, the levels capped at
// 60), with the steps' own spells of those levels, which every path has. A
// spell a merchant sells the class as a scroll is no path's: the basic grant
// gives it (the magician's Incinerating Aura, a 49 option too).
//
// "Grant path spells" says each path's spells to the player's level and
// which the book holds, then asks which. The one picked is written in:
//
//   - its spells the book lacks go in off the bar, each 0x00B3 with the flag
//     0 (no "You have gained a new ability." a spell), as the basic grant's
//     do; with nothing of another path in the book, at once;
//   - another path's spells in the book come out: the change is said and
//     asked Yes or No first, then done in one transaction (SwapSpells), the
//     client's book told 0x00B8 a row (takeRows, as the clear tells it) and
//     the World what it holds of them (entity/bookchange.go: their lockouts
//     kept, so a change and back makes no recast ready, and the player's own
//     effects of them ended). Refused while a cast is on its way, which would
//     land after the change and put back what it ended, and when the book has
//     no room, before anything moves;
//   - a spell a quest of the character's gave never comes out (spellsKeptBut:
//     every step it passed, the option it picked there, every option of a
//     step whose pick was not kept): the petition's "unless they do the
//     quests manually", a quest's spell kept beside the archivist's one path.
//     Nor do the steps' own spells.
//
// Below the first level a path gives a spell at (pathsFrom: the level 20
// picks' on the live data, 30 before petition 1361) he offers nothing. The
// boxes are the quests' (0x0046, 0x0034), which a PS2 reads: no new message.
// The words are ours.

// archivistPathLines are each class's paths by class id, each a line of
// abilities by name across levels 30, 40 and 49, as the path reward quests'
// options name them, then the level 20 class quest's pick of the line (petition
// 1361); a line's first is its name ("the path of Expansive Mind"), kept from
// petition 1325's. The GM's table (the petitions' reports): an option's spell
// named in no line is no path's, and the real data's test says so.
var archivistPathLines = map[int][][]string{
	0: { // warrior: a self hit point buff; a taunt with armour
		{"Roar", "Howl", "Battlecry", "Warcry", "Bellow"},
		{"Pillar of Strength", "Pillar of Force", "Pillar of Heroics", "Pillar of Deeds", "Pillar of Might"},
	},
	1: { // ranger: wisdom and power over time; a weapon proc
		{"Raven Eye", "Hawk Eye", "Falcon Eye", "Eagle Eye", "Sparrow Eye"},
		{"Windblade", "Shockblade", "Stormblade", "Natureblade", "Strikeblade"},
	},
	2: { // paladin: a taunt with armour; armour for another; a melee shield
		{"Gleaming Bastion", "Hallowed Bastion", "Sacred Bastion", "Blessed Bastion", "Shining Bastion"},
		{"Secure", "Bolster", "Reinforce", "Bulwark", "Fortify"},
		{"Marble Barrier", "Granite Barrier", "Iron Barrier", "Steel Barrier", "Clay Barrier"},
	},
	3: { // shadowknight: a life drain; a taunt with armour
		{"Bloodraven", "Bloodleech", "Bloodghoul", "Bloodwraith", "Bloodwasp"},
		{"Umbral Tower", "Grim Tower", "Cursed Tower", "Doom Tower", "Shadow Tower"},
	},
	4: { // monk: a defensive stance; an offensive strike
		{"Adamant Stance", "Iron Stance", "Diamond Stance", "Steel Stance", "Stone Stance"},
		{"Deft Strike", "Balanced Strike", "Masterful Strike", "Facile Strike", "Stasis Strike"},
	},
	5: { // bard: the group's power; the group's stats
		{"Power Twist", "Power Jig", "Power Groove", "Power Vibe", "Power Dance"},
		{"Flowing Combat", "Fluid Combat", "Decisive Combat", "Elegant Combat", "Sweeping Combat"},
	},
	6: { // rogue: hate lost; a direct strike
		{"Evasion", "Evade", "Elude", "Escape", "Avoidance"},
		{"Lesser Wound", "Wound", "Grievous Wound", "Mortal Wound", "Minor Wound"},
	},
	7: { // druid: fire damage over time; the group's heal
		{"Emberspark", "Inflame", "Emblaze", "Calefaction", "Enkindle"},
		{"Crawling Health", "Verdant Health", "Blooming Health", "Flowering Health", "Creeping Health"},
	},
	8: { // shaman: a debuff; a pet
		{"Sickening Wind", "Wilting Wind", "Withering Wind", "Decaying Wind"},
		{"Spiritual Guard", "Spiritual Hunter", "Spiritual Warrior", "Spiritual Guide"},
	},
	9: { // cleric: resists; a heal; the elements' resists (First Aid, the 49 step's own, every path's)
		{"Poison Ward", "Flame Ward", "Disease Ward"},
		{"Bind Wounds", "Quick Stitching", "Field Dress"},
		{"Elemental Guard", "Elemental Barrier", "Elemental Bulwark", "Elemental Ward"},
	},
	10: { // magician: the pet's mark; a conjured missile (the 49's Incinerating Aura is a merchant's scroll)
		{"Burning Mark", "Earthen Mark", "Elemental Mark", "Frozen Mark"},
		{"Ice Shard", "Fiery Pebble", "Frost Orb", "Lava Stone"},
	},
	11: { // necromancer: the group's hit points; the group's power
		{"Blood Squall", "Blood Storm", "Blood Tempest", "Blood Vortex", "Blood Gale"},
		{"Power Squall", "Power Storm", "Power Tempest", "Power Vortex", "Power Gale"},
	},
	12: { // enchanter: self power, the Mind path; power for others, the Boon path
		{"Expansive Mind", "Limitless Mind", "Boundless Mind", "Infinite Mind", "Spacious Mind"},
		// its 49s the level 49 reward's options since petition 1366 (Fujiko: "Didnt
		// receive 49 Boon spell (pwr to target)"): the import gave the level only
		// the Mind path's two, EQOA-Data's list calling these a mastery's (CM),
		// where EQOAEmu's workbook has them scrolls from a quest as Boundless and
		// Infinite Mind are, and EQOA-Data's own 60 epic names the Boon path
		{"Power Jet", "Power Surge", "Power Stream", "Power Flux", "Power Boon"},
	},
	13: { // wizard: a nuke that sheds hate; self power
		{"Static Clash", "Freezing Clash", "Burning Clash", "Shocking Clash", "Blazing Clash"},
		{"Mental Surge", "Mental Boost", "Heightened Mind", "Towering Mind", "Mental Focus"},
	},
	14: { // alchemist: a self buff; a conjured cocktail
		{"Teacher's Knack", "Professor's Knack", "Dean's Knack", "Inventor's Knack", "Student's Knack"},
		{"Moradhim Cocktail", "Neriak Cocktail", "Freeport Cocktail", "Qeynos Cocktail", "Klik'Anon Cocktail"},
	},
}

// What he says of the paths (ours). A question shows 127 characters, an
// answer 63 and a box 217, his name included (the boxes are cut where they
// end, between sentences).
const (
	archivistPathsTooLow  = "The path spells begin at level %d. Come back to me then."
	archivistNoPaths      = "I keep no path of your calling."
	archivistPathIs       = "The path of %s: %s."
	archivistPathIsLater  = "The path of %s begins at level %d."
	archivistPathOne      = "I write one path at a time: choosing another takes this one's spells out of your book, but never what your quests gave you."
	archivistPathHeld     = "Your book holds the path of %s. " + archivistPathOne
	archivistPathsHeld    = "Your book holds spells of the paths of %s. I write one path at a time: choosing one takes the others' spells out, but never what your quests gave you."
	archivistPathAsk      = SpellArchivistName + ": 'Which path shall I write into your book?'"
	archivistPathAnswer   = "The path of %s"
	archivistPathNeither  = "Neither, thank you"
	archivistPathHasAll   = "Your book holds the path of %s to your level already."
	archivistPathGiven    = "There: %s written into your book. The path of %s is yours."
	archivistPathSwapIs   = "For the path of %s, %s."
	archivistPathStays    = "%s %s in your book: your quests gave %s."
	archivistPathAskSwap  = SpellArchivistName + ": 'Shall I make the change?'"
	archivistPathYes      = "Yes, change it"
	archivistPathNo       = "No"
	archivistPathDone     = "It is done: the path of %s is yours now."
	archivistPathMoved    = "Out of your book: %s."
	archivistPathMovedIn  = "Out of your book: %s. Into it: %s."
	archivistPathEnded    = "The old path's effects on you have ended: %s."
	archivistPathCasting  = "You are casting. Let your spell land, then ask me again."
	archivistPathNoRoom   = "Your book has no room for %s."
	archivistPathsChanged = "The paths have changed in my books since I spoke. Ask me again."
)

// pathSpell is a spell a path gives: the record its quest names, and the
// quest's level.
type pathSpell struct {
	id    int
	name  string
	level int
}

// classPath is one of a class's paths as the quests give it now: its
// line's spells, by level, then the line's order.
type classPath struct {
	name   string
	spells []pathSpell
}

// upTo is the path's spells of the levels to level.
func (pa classPath) upTo(level int) []pathSpell {
	var out []pathSpell
	for _, s := range pa.spells {
		if s.level <= level {
			out = append(out, s)
		}
	}
	return out
}

// pathReward is what a path quest gives at its level: a 30/40/49 reward
// quest's steps' own spells and the class's (every pick's) and its options'
// spells; a level 20 class quest's options' spells and items (its scrolls).
type pathReward struct {
	level        int
	own, options []int
	optionItems  []int
}

// reRewardKey is a path reward quest's import key (rehabpaths.go's
// rePathKey): its level and its class's slug.
var reRewardKey = regexp.MustCompile(`^30,40,49/group-[^/]+/L(30|40|49)-([a-z]+)$`)

// pathRewards is every enabled path quest of the class (its slug
// database.TransferClassSlugs'): its 30/40/49 reward quests and its level 20
// class quests that ask the choice, in id order.
func (q *QuestEngine) pathRewards(class int) []pathReward {
	if q == nil || class < 0 || class >= len(database.TransferClassSlugs) {
		return nil
	}
	slug := database.TransferClassSlugs[class]
	q.mu.Lock()
	defer q.mu.Unlock()
	level := func(qu *database.Quest) int {
		key := importKey(qu.Source)
		if m := reRewardKey.FindStringSubmatch(key); m != nil && m[2] == slug {
			n, _ := strconv.Atoi(m[1])
			return n
		}
		if m := reL20Key.FindStringSubmatch(key); m != nil && m[1] == slug {
			return 20
		}
		return 0
	}
	var qs []*database.Quest
	for _, qu := range q.cat.all {
		if qu.Enabled && level(qu) > 0 {
			qs = append(qs, qu)
		}
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	var out []pathReward
	for _, qu := range qs {
		r := pathReward{level: level(qu)}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			if r.level != 20 {
				r.own = append(r.own, st.RewardFor(class).Spells...)
			}
			for _, o := range st.Choices.Options {
				if o.Stay {
					continue
				}
				r.options = append(r.options, o.Reward.Spells...)
				if r.level == 20 {
					for _, it := range o.Reward.Items {
						r.optionItems = append(r.optionItems, it.PatternID)
					}
				}
			}
		}
		if r.level == 20 && len(r.options)+len(r.optionItems) == 0 {
			continue // a city's quest that kept a single path: no choice
		}
		out = append(out, r)
	}
	return out
}

// classPaths is the class's paths as the running quests give them (the
// notes at the top) and the spells every path of it has (the steps' own): a
// spell a merchant sells the class (soldTo) is neither, nor is an option's
// spell of no line, which is logged. A path the quests give nothing of is
// left out.
func classPaths(qe *QuestEngine, arch *spellArchive, class int) ([]classPath, []pathSpell) {
	lines := archivistPathLines[class]
	lineOf := map[string]int{}
	for i, line := range lines {
		for _, name := range line {
			if _, ok := lineOf[exactKey(name)]; !ok {
				lineOf[exactKey(name)] = i
			}
		}
	}
	got := make([][]pathSpell, len(lines))
	var common []pathSpell
	seen := map[int]bool{}
	has := func(id int) bool { _, ok := arch.sources[id][class]; return ok }
	for _, r := range qe.pathRewards(class) {
		for _, id := range r.own {
			if s, ok := arch.names[id]; ok && !seen[id] && !arch.soldTo(class, s.Name) {
				seen[id] = true
				common = append(common, pathSpell{id: id, name: s.Name, level: r.level})
			}
		}
		for _, id := range arch.optionSpells(r, has) {
			s, ok := arch.names[id]
			if !ok || seen[id] || arch.soldTo(class, s.Name) {
				continue
			}
			seen[id] = true
			i, ok := lineOf[exactKey(s.Name)]
			if !ok {
				log.Printf("Archivist: class %d's level %d path option %q (%d) is in no line of archivistPathLines", class, r.level, s.Name, id)
				continue
			}
			got[i] = append(got[i], pathSpell{id: id, name: s.Name, level: r.level})
		}
	}
	var paths []classPath
	for i, spells := range got {
		if len(spells) == 0 {
			continue
		}
		rank := func(s pathSpell) int {
			for k, name := range lines[i] {
				if exactKey(name) == exactKey(s.name) {
					return k
				}
			}
			return len(lines[i])
		}
		sort.SliceStable(spells, func(a, b int) bool {
			if spells[a].level != spells[b].level {
				return spells[a].level < spells[b].level
			}
			return rank(spells[a]) < rank(spells[b])
		})
		name := spells[0].name // the line's first named, else its first given
		for _, s := range spells {
			if rank(s) == 0 {
				name = s.name
			}
		}
		paths = append(paths, classPath{name: name, spells: spells})
	}
	sort.SliceStable(common, func(a, b int) bool { return common[a].level < common[b].level })
	return paths, common
}

// optionSpells is a path quest's options' spells: their own, then their
// scrolls' (a level 20 pick's), each read as the scribe reads it for the class
// (has: the class's classspells).
func (a *spellArchive) optionSpells(r pathReward, has func(int) bool) []int {
	out := append([]int(nil), r.options...)
	for _, it := range r.optionItems {
		if sc, ok := a.scrollRecs[it]; ok {
			if s, ok := a.spells.scrollSpell(sc.Name, sc.LevelReq, has); ok {
				out = append(out, s.SpellID)
			}
		}
	}
	return out
}

// soldTo is whether a merchant sells a character of the class a scroll of the
// spell's name (spellKey, as the rehabilitators' soldScrolls reads it): a
// basic spell, the basic grant's.
func (a *spellArchive) soldTo(class int, name string) bool {
	return a.sold[spellKey(name)]&(1<<uint(class)) != 0
}

// pathPlan is a path asked for, as the character stands: the path's and the
// common spells to the level the book lacks (in, at display indices from
// order on once out are gone), the book's rows of the class's other paths
// coming out (out), and the other paths' spells the book keeps because a
// quest gave them (kept).
type pathPlan struct {
	path  int
	name  string
	in    []pathSpell
	out   []database.CharSpellEntry
	kept  []string
	order int
	stays []database.CharSpellEntry // the book once out are gone
}

// outIDs is the spells coming out, each once, in the book's order.
func (u *pathPlan) outIDs() []int {
	var ids []int
	seen := map[int]bool{}
	for _, e := range u.out {
		if !seen[e.Pattern.SpellID] {
			seen[e.Pattern.SpellID] = true
			ids = append(ids, e.Pattern.SpellID)
		}
	}
	return ids
}

// outNames is their names, each once.
func (u *pathPlan) outNames() []string {
	var names []string
	seen := map[int]bool{}
	for _, e := range u.out {
		if !seen[e.Pattern.SpellID] {
			seen[e.Pattern.SpellID] = true
			names = append(names, e.Pattern.SpellName)
		}
	}
	return names
}

// inIDs is the spells going in, in order; inNames their names.
func (u *pathPlan) inIDs() []int {
	ids := make([]int, len(u.in))
	for i, s := range u.in {
		ids[i] = s.id
	}
	return ids
}

func (u *pathPlan) inNames() []string {
	names := make([]string, len(u.in))
	for i, s := range u.in {
		names[i] = s.name
	}
	return names
}

// pathOfRow is which path a book row's spell is of, by its record, else by
// its exact name (lacking's rule): its index, -1 for the common spells, false
// for none of the class's.
func pathOfRow(paths []classPath, common []pathSpell, e database.CharSpellEntry) (int, bool) {
	key := exactKey(e.Pattern.SpellName)
	for _, byID := range []bool{true, false} {
		for i, pa := range paths {
			for _, s := range pa.spells {
				if (byID && s.id == e.Pattern.SpellID) || (!byID && exactKey(s.name) == key) {
					return i, true
				}
			}
		}
		for _, s := range common {
			if (byID && s.id == e.Pattern.SpellID) || (!byID && exactKey(s.name) == key) {
				return -1, true
			}
		}
	}
	return 0, false
}

// planPath is path k asked for at the level (capped at 60) with this book, the
// spells the character's quests gave being kept: what goes in and comes out
// (the notes at the top).
func planPath(book []database.CharSpellEntry, paths []classPath, common []pathSpell, k, level int, kept map[int]bool) *pathPlan {
	level = min(level, archivistLevelCap)
	u := &pathPlan{path: k, name: paths[k].name}
	keptSeen := map[int]bool{}
	for _, e := range book {
		i, ok := pathOfRow(paths, common, e)
		switch {
		case !ok || i == k || i < 0:
			u.stays = append(u.stays, e)
		case kept[e.Pattern.SpellID]:
			u.stays = append(u.stays, e)
			if !keptSeen[e.Pattern.SpellID] {
				keptSeen[e.Pattern.SpellID] = true
				u.kept = append(u.kept, e.Pattern.SpellName)
			}
		default:
			u.out = append(u.out, e)
		}
	}
	want := paths[k].upTo(level)
	for _, s := range common {
		if s.level <= level {
			want = append(want, s)
		}
	}
	sort.SliceStable(want, func(a, b int) bool { return want[a].level < want[b].level })
	names := make([]database.SpellName, len(want))
	for i, s := range want {
		names[i] = database.SpellName{SpellID: s.id, Name: s.name, Level: s.level}
	}
	for _, s := range lacking(u.stays, names) {
		u.in = append(u.in, pathSpell{id: s.SpellID, name: s.Name, level: s.Level})
	}
	u.order = nextDisplayIndex(u.stays)
	return u
}

// pathsFrom is the first level any of the paths gives a spell at (20, the
// level 20 picks', on the live data).
func pathsFrom(paths []classPath) int {
	from := 0
	for _, pa := range paths {
		for _, s := range pa.spells {
			if from == 0 || s.level < from {
				from = s.level
			}
		}
	}
	return from
}

// sameIDs is whether two lists hold the same ids.
func sameIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]int(nil), a...), append([]int(nil), b...)
	sort.Ints(x)
	sort.Ints(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// world is the World the window handler serves, nil without one.
func (a *Archivist) world() *entity.World {
	if a.windows == nil {
		return nil
	}
	return a.windows.gameWorld
}

// paths is the player's class's paths as the quests and the archive have
// them now, said when there is something wrong (false then).
func (a *Archivist) paths(sess *session.Session, p *entity.Player) (*spellArchive, []classPath, []pathSpell, bool) {
	arch, err := a.archived()
	if err != nil {
		log.Printf("Archivist: %s's paths, the archive: %v", p.Name, err)
		a.say(sess, archivistTrouble)
		return nil, nil, nil, false
	}
	qe := a.quests()
	if qe == nil {
		log.Printf("Archivist: %s's paths: no quests running", p.Name)
		a.say(sess, archivistTrouble)
		return nil, nil, nil, false
	}
	paths, common := classPaths(qe, arch, p.Class)
	if len(paths) == 0 {
		a.say(sess, archivistNoPaths)
		return nil, nil, nil, false
	}
	return arch, paths, common, true
}

// askPaths says each path's spells to the player's level and what the book
// holds, and asks which path to write (the notes at the top); below the first
// level a path gives a spell at (pathsFrom) he offers nothing.
func (a *Archivist) askPaths(sess *session.Session, id uint32) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	a.book.Lock()
	defer a.book.Unlock()
	_, paths, common, ok := a.paths(sess, p)
	if !ok {
		return
	}
	if from := pathsFrom(paths); p.Level < from {
		a.say(sess, fmt.Sprintf(archivistPathsTooLow, from))
		return
	}
	book, err := a.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		log.Printf("Archivist: %s's paths, the book: %v", p.Name, err)
		a.say(sess, archivistTrouble)
		return
	}
	level := min(p.Level, archivistLevelCap)
	var says []string
	for _, pa := range paths {
		spells := pa.upTo(level)
		for _, s := range common {
			if s.level <= level {
				spells = append(spells, s)
			}
		}
		if len(spells) == 0 {
			says = append(says, fmt.Sprintf(archivistPathIsLater, pa.name, pa.spells[0].level))
			continue
		}
		names := make([]string, len(spells))
		for i, s := range spells {
			names[i] = s.name
		}
		says = append(says, fmt.Sprintf(archivistPathIs, pa.name, joinAnd(names)))
	}
	a.say(sess, strings.Join(says, " "))
	var held []string
	seen := map[int]bool{}
	for _, e := range book {
		if i, ok := pathOfRow(paths, common, e); ok && i >= 0 && !seen[i] {
			seen[i] = true
			held = append(held, paths[i].name)
		}
	}
	switch len(held) {
	case 0:
	case 1:
		a.say(sess, fmt.Sprintf(archivistPathHeld, held[0]))
	default:
		a.say(sess, fmt.Sprintf(archivistPathsHeld, joinAnd(held)))
	}
	answers := make([]string, 0, len(paths)+1)
	offered := make([]int, 0, len(paths))
	var names []string
	for i, pa := range paths {
		if len(answers) == maxDialogOptions-1 {
			break // CLIENT's box holds six answers, Neither one of them
		}
		answers = append(answers, clipRunes(fmt.Sprintf(archivistPathAnswer, pa.name), questAnswerChars))
		offered, names = append(offered, i), append(names, pa.name)
	}
	answers = append(answers, archivistPathNeither)
	a.askWith(sess, p, archivistAsk{stage: archivistAskPath, npc: id, paths: offered, names: names}, archivistPathAsk, answers)
	log.Printf("Archivist: %s (class %d, level %d) asked which path: %d offered, the book holding %v", p.Name, p.Class, p.Level,
		len(paths), held)
}

// pickPath is a path picked (its index and name as offered): written at once
// when nothing of another path comes out of the book, else said and asked
// first (the notes at the top).
func (a *Archivist) pickPath(sess *session.Session, id uint32, k int, name string) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	a.book.Lock()
	defer a.book.Unlock()
	u, ok := a.plan(sess, p, k, name)
	if !ok {
		return
	}
	if len(u.out) == 0 {
		a.writePath(sess, p, u)
		return
	}
	a.askSwapLocked(sess, p, id, u)
}

// askSwapLocked says what a change takes out and puts in, and asks Yes or
// No. Called with book held.
func (a *Archivist) askSwapLocked(sess *session.Session, p *entity.Player, id uint32, u *pathPlan) {
	out := u.outNames()
	goes := joinAnd(out) + " " + plural(len(out), "leaves", "leave") + " your book"
	if len(u.in) > 0 {
		goes += ", and " + joinAnd(u.inNames()) + " " + plural(len(u.in), "enters", "enter") + " it"
	}
	lines := []string{fmt.Sprintf(archivistPathSwapIs, u.name, goes)}
	if len(u.kept) > 0 {
		lines = append(lines, fmt.Sprintf(archivistPathStays, joinAnd(u.kept), plural(len(u.kept), "stays", "stay"),
			plural(len(u.kept), "it", "them")))
	}
	a.say(sess, strings.Join(lines, " "))
	a.askWith(sess, p, archivistAsk{stage: archivistAskSwap, npc: id, path: u.path, pathName: u.name, out: u.outIDs(), in: u.inIDs()},
		archivistPathAskSwap, []string{archivistPathYes, archivistPathNo})
}

// swapPath is a change agreed: planned again, asked again when it moved
// since (a level gained, a quest done, the book changed), refused while a cast
// is on its way, then done. ask is the question answered.
func (a *Archivist) swapPath(sess *session.Session, ask archivistAsk) {
	p, _, ok := a.here(sess, ask.npc)
	if !ok {
		return
	}
	a.book.Lock()
	defer a.book.Unlock()
	u, ok := a.plan(sess, p, ask.path, ask.pathName)
	if !ok {
		return
	}
	switch {
	case len(u.out) == 0:
		a.writePath(sess, p, u) // nothing comes out now: nothing to ask
		return
	case !sameIDs(u.outIDs(), ask.out) || !sameIDs(u.inIDs(), ask.in):
		a.askSwapLocked(sess, p, ask.npc, u)
		return
	}
	if w := a.world(); w != nil && w.Casting(sess) {
		a.say(sess, archivistPathCasting)
		return
	}
	a.writePath(sess, p, u)
}

// plan is path k, named so when it was offered, planned for the player as it
// stands, said when it cannot be (false then): the paths changed since (a GM's
// edit of the quests), the book holds it all, or it has no room. Called with
// book held.
func (a *Archivist) plan(sess *session.Session, p *entity.Player, k int, name string) (*pathPlan, bool) {
	_, paths, common, ok := a.paths(sess, p)
	if !ok {
		return nil, false
	}
	if from := pathsFrom(paths); p.Level < from {
		a.say(sess, fmt.Sprintf(archivistPathsTooLow, from)) // a level lost since the question
		return nil, false
	}
	if k < 0 || k >= len(paths) || paths[k].name != name {
		a.say(sess, archivistPathsChanged)
		return nil, false
	}
	book, err := a.spells.GetCharacterSpells(p.CharServerID)
	if err != nil {
		log.Printf("Archivist: %s's path, the book: %v", p.Name, err)
		a.say(sess, archivistTrouble)
		return nil, false
	}
	kept := a.quests().spellsKeptBut(p.CharServerID, p.Class, 0, 0)
	u := planPath(book, paths, common, k, p.Level, kept)
	switch {
	case len(u.out) == 0 && len(u.in) == 0:
		a.say(sess, fmt.Sprintf(archivistPathHasAll, u.name))
		return nil, false
	case u.order+len(u.in) > 252: // the book's display indices end at 252
		a.say(sess, fmt.Sprintf(archivistPathNoRoom, joinAnd(u.inNames())))
		return nil, false
	}
	return u, true
}

// writePath writes a plan: the book's rows out and in, in one transaction,
// the client told (0x00B8 a row gone, then 0x00B3 a spell come), the World
// told (bookchange.go), and what was done said. Called with book held.
func (a *Archivist) writePath(sess *session.Session, p *entity.Player, u *pathPlan) {
	out, in := u.outIDs(), u.inIDs()
	rows, err := a.spells.SwapSpells(p.CharServerID, out, in, u.order)
	if err != nil {
		log.Printf("Archivist: %s's path of %q: %v (nothing changed)", p.Name, u.name, err)
		a.say(sess, archivistTrouble)
		return
	}
	stayAt := map[int]bool{}
	for _, e := range u.stays {
		stayAt[e.AddedOrder] = true
	}
	later := takeRows(sess, u.out, stayAt)
	came := map[int]int{}
	if len(in) > 0 {
		book, err := a.spells.GetCharacterSpells(p.CharServerID)
		if err != nil {
			log.Printf("Archivist: %s's book after the path: %v (the spells are written, shown at the next login)", p.Name, err)
		}
		added := map[int]bool{}
		for _, id := range in {
			added[id] = true
		}
		for i := range book {
			if e := &book[i]; added[e.Pattern.SpellID] && e.AddedOrder >= u.order && e.AddedOrder < u.order+len(in) {
				SendAbilityAdd(sess, 0, false, e)
				came[e.Pattern.SpellID] = e.AddedOrder
			}
		}
	}
	var ended []string
	shown := 0
	if w := a.world(); w != nil {
		ended, shown = w.BookChanged(sess, out, came, a.now())
	}
	log.Printf("Archivist: %s (class %d, level %d) took the path of %q: out %v (%d rows, %d gone at the next login), in %v at %d, "+
		"kept by quests %v, effects ended %v, lockouts shown %d", p.Name, p.Class, p.Level, u.name, u.outNames(), rows, len(later),
		u.inNames(), u.order, u.kept, ended, shown)
	if len(out) == 0 {
		a.say(sess, fmt.Sprintf(archivistPathGiven, joinAnd(u.inNames()), u.name))
	} else {
		moved := fmt.Sprintf(archivistPathMoved, strings.Join(u.outNames(), ", "))
		if len(in) > 0 {
			moved = fmt.Sprintf(archivistPathMovedIn, strings.Join(u.outNames(), ", "), strings.Join(u.inNames(), ", "))
		}
		a.say(sess, fmt.Sprintf(archivistPathDone, u.name)+" "+moved)
	}
	if len(ended) > 0 {
		a.say(sess, fmt.Sprintf(archivistPathEnded, joinAnd(ended)))
	}
	if len(later) > 0 {
		SendColoredChat(sess, fmt.Sprintf(archivistLater, strings.Join(later, ", ")), 255, 255, 0)
	}
}

// takeRows tells the client the book's rows gone: 0x00B8 by each row's
// display index, one a row (two rows of one index are two abilities, and the
// second finds nothing when they were one spell), but not a row at an index a
// row kept holds too (Msg00B8 erases the first ability of an index, which
// could be the kept one) nor one at 252 or more (no index of its own): those
// leave the client's book at the next login, and their names are returned,
// each once.
func takeRows(sess *session.Session, gone []database.CharSpellEntry, keptAt map[int]bool) []string {
	var later []string
	told := map[int]bool{}
	for _, e := range gone {
		if e.AddedOrder < 252 && !keptAt[e.AddedOrder] {
			SendAbilityRemove(sess, e.AddedOrder)
		} else if !told[e.Pattern.SpellID] {
			told[e.Pattern.SpellID] = true
			later = append(later, e.Pattern.SpellName)
		}
	}
	return later
}
