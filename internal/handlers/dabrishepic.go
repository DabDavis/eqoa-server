package handlers

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// dabrishepic.go — Dabrish exchanges a path epic for the other path's (the
// owner, 2026-11-13: "allow dabrish to exchange epic weapons for the other
// path for players"). A class's path epics are the two items its level 49
// path reward quest offers at its weapon pick (30,40,49/<group>/L49-<class>,
// archivistpaths.go's reRewardKey: the warrior's Steelwind and Foereaver, the
// alchemist's Metamorphic Robes and Odan's Bonded Gloves), read off the
// running quests as their bases (an option's Ascended record counts for its
// base, database/ascended.go).
//
// A level 60 epic is exchanged for another of its pick (the owner, 2026-11-16:
// "yes extend dabrish for the level 60 epics", Heirloom petition 2203: a
// shadowknight's Epic Greataxe "not linked to a path"). A class's level 60
// epic reward quest (60/<group>/reward-<class>, reEpic60Key) asks at a step
// which of its weapons, robes and tunics the character takes (the
// shadowknight's Sword of the Contender, Epic Lance and Epic Greataxe; the
// bard's two blades, axe and four tunics): each such step's options' items,
// as their bases, are a set, and one held is exchanged for any other of every
// set it is in (a class's good and evil quests' picks differ: the wizard's
// share the staff and focus, the magician's share nothing), which he asks
// when there are several. Every rule of the path epics holds; only the
// rehabilitators' kept pick is the level 49 quest's alone (a path change
// takes no level 60 epic back), so none is moved. A level 60 epic in no set
// of two (the paladin's sword, every pick's own) is still refused as no
// path's. An option giving two or more different records (the bard's two
// blades, classEpicGroups) is one set, offered and given as one: every record
// of it carried in the pack is taken together, refused when one is missing,
// worn or banked, and every record of it given together (Heirloom petition
// 2550).
//
// "Exchange my epic weapon", his first box's sixth answer, reads the
// character's rows:
//
//   - a path epic of its class in the pack, unworn, is offered: which he gives
//     for it is the other path's in the same version, the regular record for a
//     regular one, the Ascended record for an Ascended one (refused when the
//     other has none), an imported copy for an imported one (the transfer's
//     damper, database/imported.go: the other's copy, made when it has none).
//     With several, he asks which first;
//   - refused, each with its own line: one worn (taken off first, as a
//     rehabilitator's path change refuses, rehab.go), one banked, the other
//     path's epic owned already in any version and anywhere (no duplicates), a
//     level 60 epic in no pick (no counterpart), an Heirloom of a path epic or
//     of a level 60 pick's (the Heirloom Outfitter sells his for nothing), and
//     none at all. A worn level 60 epic's line names what it would get.
//
// He says what is held and what he gives, then asks Yes or No. On Yes it is
// planned again from the rows (refused if anything moved) and done in one
// transaction (InventoryStore.ExchangeCharItem): the row deleted, the new one
// at the end of the pack with the old one's share of its durability and its
// charges; the client told 0x003C for the old record and 0x003B for the new,
// the session's copy of its list kept in step. A level 49 reward quest's kept
// weapon pick is moved with it, so a rehabilitator's path change later takes
// back what the character holds. Free: the rehabilitators' resets are free
// since 2026-11-06 (rehab.go; before, 2 free path respecs then a Rose of
// Rehabilitation, sold for 10,000,000, each), and this is no more than half of
// one. Logged; no gm_audit line, as no quest reward, sale or rehabilitator's
// change has one. The words are ours.

// The exchange's answer in his first box, and its boxes' marks in dabrishAsk.rng.
const (
	dabrishEpicLabel   = "Exchange my epic weapon"
	dabrishEpicWhich   = 100 // which of several epics
	dabrishEpicConfirm = 101 // Yes or No
	dabrishEpicFor     = 102 // which of a level 60 epic's others
)

// His words.
const (
	dabrishEpicNoPaths  = "I know of no path epics for your class."
	dabrishEpicNone     = "You hold no level 49 path epic of your class for me to exchange."
	dabrishEpicNoPair   = "The %s belongs to no path: I have nothing of another path to give for it."
	dabrishEpicHeirloom = "Your %s is an heirloom: the Heirloom Outfitter gives either path's for nothing."
	dabrishEpicWorn     = "Take the %s off first, then ask me again."
	dabrishEpicBanked   = "Bring the %s from your bank first, then ask me again."
	dabrishEpicOwned    = "You own the %s already. I will not give you a second."
	dabrishEpicNoMatch  = "There is no %s version of the %s for me to give for your %s."
	dabrishEpicShow     = "You hold the %s. For it I give you the %s, the other path's epic, free of charge."
	dabrishEpicAsk      = "Dabrish: 'Exchange the %s for the %s?'"
	dabrishEpicAskWhich = "Dabrish: 'Which epic shall I exchange?'"
	dabrishEpicYes      = "Yes, exchange it."
	dabrishEpicNo       = "No."
	dabrishEpicNotNow   = "As you like. Your %s stays yours."
	dabrishEpicDone     = "There: the %s is yours, and the %s mine."
	dabrishEpicMoved    = "You no longer hold the %s as you did. Ask me again."

	// a level 60 epic's
	dabrishEpicHeirloom60 = "Your %s is an heirloom, and I exchange no heirloom: the Heirloom Outfitter gives his for nothing."
	dabrishEpicWorn60     = "Take the %s off and carry it in your pack first, then ask me again: for it I can give you %s."
	dabrishEpicShow60     = "You hold the %s. For it I give you the %s, another of your calling's level 60 epics, free of charge."
	dabrishEpicShowFor    = "You hold the %s. For it I give you %s, whichever you choose, free of charge."
	dabrishEpicAskFor     = "Dabrish: 'Which shall I give for your %s?'"

	// a pair's (classEpicCopies), one of it worn
	dabrishEpicWornPair = "Take both of your %s off and carry them in your pack first, then ask me again: for the pair I can give you %s."

	// a set's (classEpicGroups): its records' names joined, "A and the B"
	dabrishEpicShowSetFrom = "You hold the %s. For them I give you the %s, free of charge."
	dabrishEpicShowSetTo   = "You hold the %s. For it I give you the %s, free of charge."
	dabrishEpicShowForSet  = "You hold the %s. For them I give you %s, whichever you choose, free of charge."
	dabrishEpicWornSet     = "Take the %s off and carry them in your pack first, then ask me again: for them I can give you %s."
	dabrishEpicApart       = "The %s are one pick, taken only together: carry each in your pack, then ask me again."
	dabrishEpicDoneSet     = "There: the %s are yours, and the %s mine."
)

// reEpic60Key is a level 60 epic reward quest's import key: its class's slug.
var reEpic60Key = regexp.MustCompile(`^60/[^/]+/reward-([a-z]+)$`)

// epicSwap is one exchange planned: the pack's row, what it is and what is
// given for it.
type epicSwap struct {
	itemID     int
	from       int    // the row's record
	fromName   string // its name
	fromBase   int    // the path epic it is a version of
	toBase     int    // the other path's
	to         int    // the record given; 0 with importedOf
	importedOf int    // toBase, whose imported copy is given (made when none)
	toName     string
	sixty      bool  // a level 60 epic's, not a path's
	more       []int // the pair's other rows given up with itemID (classEpicCopies)
	give       int   // copies of the record given (0: one)
	// a set's (classEpicGroups): its other rows given up with itemID, its
	// other records given with to, and whether the rows given up are a set
	// (fromName and fromBase the set's) or the records given (toName and
	// toBase the set's)
	with       []database.ExchangeRow
	also       []database.ExchangeGive
	set, toSet bool
	// a set's names as its boxes' questions and answers name it, its words in
	// common once ("Ascended Blade of Lyrical Harmony and Refrain"); "": the
	// name
	fromShort, toShort string
}

// fromBox is the rows given up as a box names them.
func (s epicSwap) fromBox() string { return cmp.Or(s.fromShort, s.fromName) }

// toBox is what is given as a box names it.
func (s epicSwap) toBox() string { return cmp.Or(s.toShort, s.toName) }

// dabrishEpics is what the exchange reads and writes. Nil: no exchange.
type dabrishEpics struct {
	inv    *database.InventoryStore
	quests func() *QuestEngine
	now    func() time.Time
}

// newDabrishEpics is the exchange over the inventory and the running quests.
func newDabrishEpics(inv *database.InventoryStore) *dabrishEpics {
	if inv == nil {
		return nil
	}
	return &dabrishEpics{inv: inv, quests: Quests, now: time.Now}
}

// classEpicItems is what the class's enabled epic quests offer: the level 49
// path reward quests' options' items (pair), the level 60 epic rewards'
// options' and steps' items (sixty), each record once, in order; and the
// level 60 epic rewards' picks (picks): each step's options' items, a step
// with options of two or more items.
func (q *QuestEngine) classEpicItems(class int) (pair, sixty []int, picks [][]int) {
	if q == nil || class < 0 || class >= len(database.TransferClassSlugs) {
		return nil, nil, nil
	}
	slug := database.TransferClassSlugs[class]
	q.mu.Lock()
	defer q.mu.Unlock()
	qs := slices.Clone(q.cat.all)
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	add := func(to []int, items []database.ItemCount) []int {
		for _, it := range items {
			if !slices.Contains(to, it.PatternID) {
				to = append(to, it.PatternID)
			}
		}
		return to
	}
	for _, qu := range qs {
		if !qu.Enabled {
			continue
		}
		key := importKey(qu.Source)
		if m := reRewardKey.FindStringSubmatch(key); m != nil && m[1] == "49" && m[2] == slug {
			for i := range qu.Steps {
				for _, o := range qu.Steps[i].Choices.Options {
					if !o.Stay {
						pair = add(pair, o.Reward.Items)
					}
				}
			}
		} else if m := reEpic60Key.FindStringSubmatch(key); m != nil && m[1] == slug {
			for i := range qu.Steps {
				sixty = add(sixty, qu.Steps[i].RewardFor(class).Items)
				var pick []int
				for _, o := range qu.Steps[i].Choices.Options {
					sixty = add(sixty, o.Reward.Items)
					pick = add(pick, o.Reward.Items)
				}
				if len(pick) >= 2 {
					picks = append(picks, pick)
				}
			}
		}
	}
	return pair, sixty, picks
}

// classEpicCopies is how many of each record the class's level 60 epic
// reward picks give at once, where an option gives more than one (the rogue's
// "The Ascended Rogue's Epic Daggers", 980148*2): a pair is exchanged as a
// pair, both taken and both given (Heirloom petition 2236: "I
// exchanged my 60 epic weps for tunic but it only took the unequipped wep. It
// should take both").
func (q *QuestEngine) classEpicCopies(class int) map[int]int {
	if q == nil || class < 0 || class >= len(database.TransferClassSlugs) {
		return nil
	}
	slug := database.TransferClassSlugs[class]
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[int]int{}
	for _, qu := range q.cat.all {
		if !qu.Enabled {
			continue
		}
		if m := reEpic60Key.FindStringSubmatch(importKey(qu.Source)); m == nil || m[1] != slug {
			continue
		}
		for i := range qu.Steps {
			for _, o := range qu.Steps[i].Choices.Options {
				for _, it := range o.Reward.Items {
					if it.Count > 1 && it.Count > out[it.PatternID] {
						out[it.PatternID] = it.Count
					}
				}
			}
		}
	}
	return out
}

// classEpicGroups is the class's level 60 epic reward picks' options giving
// two or more different records, each option's records once, in order: a
// set, exchanged as one, every record of it taken and given together (Heirloom
// petition 2550 (Deathcore, a bard: "exchanging epic weapons with Dabrish only
// gives one of the lyrical weapons back"): "The Bard's Epic Reward" gives the
// Ascended Blades of Lyrical Harmony and Refrain, 980083 and 980084, as one
// option).
func (q *QuestEngine) classEpicGroups(class int) [][]int {
	if q == nil || class < 0 || class >= len(database.TransferClassSlugs) {
		return nil
	}
	slug := database.TransferClassSlugs[class]
	q.mu.Lock()
	defer q.mu.Unlock()
	qs := slices.Clone(q.cat.all)
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	var out [][]int
	for _, qu := range qs {
		if !qu.Enabled {
			continue
		}
		if m := reEpic60Key.FindStringSubmatch(importKey(qu.Source)); m == nil || m[1] != slug {
			continue
		}
		for i := range qu.Steps {
			for _, o := range qu.Steps[i].Choices.Options {
				var g []int
				for _, it := range o.Reward.Items {
					if !slices.Contains(g, it.PatternID) {
						g = append(g, it.PatternID)
					}
				}
				if len(g) >= 2 {
					out = append(out, g)
				}
			}
		}
	}
	return out
}

// bothNames is a set's names as one, for "the %s": "A and the B", "A, the B
// and the C".
func bothNames(names []string) string {
	out := ""
	for i, n := range names {
		switch {
		case i == 0:
		case i == len(names)-1:
			out += " and the "
		default:
			out += ", the "
		}
		out += n
	}
	return out
}

// shortNames is a set's names as one with their first words in common said
// once: "Ascended Blade of Lyrical Harmony and Refrain" (a box's answer has
// room for 63 characters); bothNames' when they have none in common.
func shortNames(names []string) string {
	if len(names) < 2 {
		return bothNames(names)
	}
	words := make([][]string, len(names))
	for i, n := range names {
		words[i] = strings.Fields(n)
	}
	k := 0
	for ; ; k++ {
		same := true
		for _, w := range words {
			if k >= len(w)-1 || w[k] != words[0][k] {
				same = false
				break
			}
		}
		if !same {
			break
		}
	}
	if k == 0 {
		return bothNames(names)
	}
	out := strings.Join(words[0][:k], " ")
	for i, w := range words {
		switch {
		case i == 0:
			out += " "
		case i == len(words)-1:
			out += " and "
		default:
			out += ", "
		}
		out += strings.Join(w[k:], " ")
	}
	return out
}

// epicSets is the level 60 picks as their bases, each base once a set, a
// path epic in none, each set of two or more.
func epicSets(picks [][]int, v epicVersions, pair []int) [][]int {
	var out [][]int
	for _, pick := range picks {
		var set []int
		for _, id := range pick {
			if b, _ := v.base(id); !slices.Contains(pair, b) && !slices.Contains(set, b) {
				set = append(set, b)
			}
		}
		if len(set) >= 2 {
			out = append(out, set)
		}
	}
	return out
}

// epicOthers is what a level 60 epic's base may be exchanged for: every
// other base of the sets it is in, each once, in order.
func epicOthers(sets [][]int, b int) []int {
	var out []int
	for _, set := range sets {
		if !slices.Contains(set, b) {
			continue
		}
		for _, o := range set {
			if o != b && !slices.Contains(out, o) {
				out = append(out, o)
			}
		}
	}
	return out
}

// theNames is names as a line: "the A", "the A or the B", "the A, the B or
// the C".
func theNames(names []string) string {
	out := ""
	for i, n := range names {
		switch {
		case i == 0:
		case i == len(names)-1:
			out += " or "
		default:
			out += ", "
		}
		out += "the " + n
	}
	return out
}

// epicVersions is the records' versions: an Ascended record's and an
// imported copy's base.
type epicVersions struct {
	asc, imp map[int]int
}

// base is a record's base and its version: "Ascended", "imported" or "".
func (v epicVersions) base(id int) (int, string) {
	if b, ok := v.asc[id]; ok {
		return b, "Ascended"
	}
	if b, ok := v.imp[id]; ok {
		return b, "imported"
	}
	return id, ""
}

// of is every record that is base in some version: itself, its Ascended
// records, its imported copies.
func (v epicVersions) of(base int) []int {
	out := []int{base}
	for _, m := range []map[int]int{v.asc, v.imp} {
		for c, b := range m {
			if b == base {
				out = append(out, c)
			}
		}
	}
	sort.Ints(out[1:])
	return out
}

// copyOf is base's record in the version, and whether it has one.
func (v epicVersions) copyOf(base int, version string) (int, bool) {
	var m map[int]int
	switch version {
	case "":
		return base, true
	case "Ascended":
		m = v.asc
	default:
		m = v.imp
	}
	best := 0
	for c, b := range m {
		if b == base && (best == 0 || c < best) {
			best = c
		}
	}
	return best, best != 0
}

// name is a record's name, or its number.
func (e *dabrishEpics) name(id int) string {
	if pat, ok, err := e.inv.ItemPatternByID(id); err == nil && ok {
		return pat.ItemName
	}
	return fmt.Sprintf("item %d", id)
}

// plan is what the character may exchange: each path epic of its class in its
// pack, unworn, with what it would get, and each level 60 epic of a pick so,
// once for each other of its picks it would get; or, with none, why not.
func (e *dabrishEpics) plan(p *entity.Player) ([]epicSwap, string, error) {
	pairIDs, sixtyIDs, picks := e.quests().classEpicItems(p.Class)
	asc, imp, err := e.inv.ItemVersions()
	if err != nil {
		return nil, "", err
	}
	v := epicVersions{asc: asc, imp: imp}
	var pair []int
	for _, id := range pairIDs {
		if b, _ := v.base(id); !slices.Contains(pair, b) {
			pair = append(pair, b)
		}
	}
	if len(pair) != 2 {
		pair = nil
	}
	// the sets of records given as one option (classEpicGroups), as their
	// bases, each a set's first in the picks for the whole set
	setOf := map[int]int{}     // a set's base: its first's
	members := map[int][]int{} // a set's first's base: the set's, in order
	for _, g := range e.quests().classEpicGroups(p.Class) {
		var bs []int
		for _, id := range g {
			if b, _ := v.base(id); !slices.Contains(bs, b) && !slices.Contains(pair, b) {
				bs = append(bs, b)
			}
		}
		if len(bs) < 2 || slices.ContainsFunc(bs, func(b int) bool { _, ok := setOf[b]; return ok }) {
			continue
		}
		for _, b := range bs {
			setOf[b] = bs[0]
		}
		members[bs[0]] = bs
	}
	for i, pick := range picks {
		picks[i] = slices.DeleteFunc(slices.Clone(pick), func(id int) bool {
			b, _ := v.base(id)
			first, ok := setOf[b]
			return ok && first != b
		})
	}
	sets := epicSets(picks, v, pair)
	if pair == nil && len(sets) == 0 {
		return nil, dabrishEpicNoPaths, nil
	}
	sixty := map[int]bool{}
	for _, id := range sixtyIDs {
		b, _ := v.base(id)
		sixty[b] = !slices.Contains(pair, b)
	}
	copies := map[int]int{} // a base's copies given at once, a pair's 2
	for id, n := range e.quests().classEpicCopies(p.Class) {
		b, _ := v.base(id)
		copies[b] = max(copies[b], n)
	}
	heirlooms, err := e.inv.Heirlooms()
	if err != nil {
		return nil, "", err
	}
	rows, err := e.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, "", err
	}
	// a pair's rows carried or worn, by record, the pack's first: the pair
	// exchanged as one, from its first row
	mates := map[int][]database.CharInventoryEntry{}
	for _, row := range rows {
		b, _ := v.base(row.Pattern.PatternID)
		if _, inSet := setOf[b]; !inSet && copies[b] > 1 && row.Location != database.LocBank {
			mates[row.Pattern.PatternID] = append(mates[row.Pattern.PatternID], row)
		}
	}
	for id, m := range mates {
		sort.SliceStable(m, func(i, j int) bool { return m[i].EquipLoc < 0 && m[j].EquipLoc >= 0 })
		b, _ := v.base(id)
		mates[id] = m[:min(len(m), copies[b])]
	}
	// rank is a row's place among its record's for a set: the pack's unworn
	// first, then worn, then banked
	rank := func(r database.CharInventoryEntry) int {
		switch {
		case r.Location == database.LocBank:
			return 2
		case r.EquipLoc >= 0:
			return 1
		}
		return 0
	}
	var swaps []epicSwap
	var refusals, worn, banked, noPair, heirloom []string
	setDone := map[int]bool{}
	for _, row := range rows {
		b, version := v.base(row.Pattern.PatternID)
		// a set's member: the set planned once, from its first member's
		// best row held, its others' best rows given up with it
		fromName, fromShort := row.Pattern.ItemName, ""
		var setRows []database.CharInventoryEntry
		var apart bool
		var bankedName string
		first, inSet := setOf[b]
		if inSet {
			if setDone[first] {
				continue
			}
			setDone[first] = true
			var names []string
			held := false
			for _, m := range members[first] {
				var best database.CharInventoryEntry
				found := false
				for _, r := range rows {
					if rb, _ := v.base(r.Pattern.PatternID); rb == m && (!found || rank(r) < rank(best)) {
						best, found = r, true
					}
				}
				switch {
				case !found:
					apart = true
					id, ok := v.copyOf(m, version)
					if !ok {
						id = m
					}
					names = append(names, e.name(id))
					continue
				case !held:
					row, held = best, true
					_, version = v.base(best.Pattern.PatternID)
				default:
					setRows = append(setRows, best)
				}
				if best.Location == database.LocBank && bankedName == "" {
					bankedName = best.Pattern.ItemName
				}
				names = append(names, best.Pattern.ItemName)
			}
			b = first
			fromName = bothNames(names)
			fromShort = shortNames(names)
		}
		gear := row.Pattern.EquipSlot >= 1 && row.Pattern.MaxStack < 2
		var others []int
		if i := slices.Index(pair, b); i >= 0 {
			others = []int{pair[1-i]}
		} else {
			others = epicOthers(sets, b)
		}
		is60 := !slices.Contains(pair, b)
		pairRows := mates[row.Pattern.PatternID]
		wornAny := row.EquipLoc >= 0
		for _, m := range setRows {
			wornAny = wornAny || m.EquipLoc >= 0
		}
		if len(pairRows) > 0 {
			if row.Location == database.LocBank || pairRows[0].ItemID != row.ItemID {
				continue // the pair's, from its first row
			}
			for _, m := range pairRows {
				wornAny = wornAny || m.EquipLoc >= 0
			}
		}
		switch {
		case len(others) == 0 && gear && sixty[b]:
			noPair = append(noPair, fromName)
			continue
		case len(others) == 0:
			if def, ok := heirlooms[row.Pattern.PatternID]; ok && def.Epic != 0 {
				eb, _ := v.base(def.Epic)
				if slices.Contains(pair, eb) {
					heirloom = append(heirloom, fmt.Sprintf(dabrishEpicHeirloom, row.Pattern.ItemName))
				} else if len(epicOthers(sets, eb)) > 0 {
					heirloom = append(heirloom, fmt.Sprintf(dabrishEpicHeirloom60, row.Pattern.ItemName))
				}
			}
			continue
		case !gear:
			continue
		case row.Location == database.LocBank:
			banked = append(banked, row.Pattern.ItemName)
			continue
		case bankedName != "":
			banked = append(banked, bankedName)
			continue
		case apart:
			refusals = append(refusals, fmt.Sprintf(dabrishEpicApart, fromName))
			continue
		case wornAny && !is60:
			worn = append(worn, fmt.Sprintf(dabrishEpicWorn, row.Pattern.ItemName))
			continue
		}
		var got []epicSwap
		var no []string
	others:
		for _, other := range others {
			tos := members[other] // a set given: every record of it
			if len(tos) == 0 {
				tos = []int{other}
			}
			for _, t := range tos {
				if owned, err := e.inv.OwnsAnyOf(p.CharServerID, v.of(t)); err != nil {
					return nil, "", err
				} else if owned {
					no = append(no, fmt.Sprintf(dabrishEpicOwned, e.name(t)))
					continue others
				}
			}
			s := epicSwap{itemID: row.ItemID, from: row.Pattern.PatternID, fromName: fromName, fromBase: b,
				toBase: other, sixty: is60, give: copies[other], set: inSet, toSet: len(tos) > 1, fromShort: fromShort}
			for _, m := range pairRows[min(1, len(pairRows)):] {
				s.more = append(s.more, m.ItemID)
			}
			for _, m := range setRows {
				s.with = append(s.with, database.ExchangeRow{ItemID: m.ItemID, From: m.Pattern.PatternID})
			}
			var names []string
			for i, t := range tos {
				var g database.ExchangeGive
				to, ok := v.copyOf(t, version)
				switch {
				case ok:
					g.To = to
					names = append(names, e.name(to))
				case version == "imported":
					g.ImportedOf = t // the damper's copy, made at the exchange
					names = append(names, e.name(t))
				default:
					no = append(no, fmt.Sprintf(dabrishEpicNoMatch, version, e.name(t), fromName))
					continue others
				}
				if i == 0 {
					s.to, s.importedOf = g.To, g.ImportedOf
				} else {
					s.also = append(s.also, g)
				}
			}
			s.toName = bothNames(names)
			if s.toSet {
				s.toShort = shortNames(names)
			}
			got = append(got, s)
		}
		switch {
		case len(got) == 0:
			refusals = append(refusals, no...)
		case wornAny: // a level 60 epic worn: what it would get said
			names := make([]string, len(got))
			for i, s := range got {
				names[i] = s.toName
			}
			if inSet {
				worn = append(worn, fmt.Sprintf(dabrishEpicWornSet, fromName, theNames(names)))
			} else if len(pairRows) > 1 {
				worn = append(worn, fmt.Sprintf(dabrishEpicWornPair, row.Pattern.ItemName, theNames(names)))
			} else {
				worn = append(worn, fmt.Sprintf(dabrishEpicWorn60, row.Pattern.ItemName, theNames(names)))
			}
		default:
			swaps = append(swaps, got...)
		}
	}
	if len(swaps) > 0 {
		return swaps, "", nil
	}
	switch {
	case len(refusals) > 0:
		return nil, refusals[0], nil
	case len(worn) > 0:
		return nil, worn[0], nil
	case len(banked) > 0:
		return nil, fmt.Sprintf(dabrishEpicBanked, banked[0]), nil
	case len(noPair) > 0:
		return nil, fmt.Sprintf(dabrishEpicNoPair, noPair[0]), nil
	case len(heirloom) > 0:
		return nil, heirloom[0], nil
	}
	return nil, dabrishEpicNone, nil
}

// bySwapRow is swaps by their rows, each row's in order, the rows in order.
func bySwapRow(swaps []epicSwap) [][]epicSwap {
	var out [][]epicSwap
	for _, s := range swaps {
		if n := len(out); n > 0 && out[n-1][0].itemID == s.itemID {
			out[n-1] = append(out[n-1], s)
			continue
		}
		out = append(out, []epicSwap{s})
	}
	return out
}

// dabrishEpicStart is his first box's exchange: what may be exchanged, asked
// which when several, else shown and asked Yes or No.
func (h *InteractHandler) dabrishEpicStart(sess *session.Session, p *entity.Player, npc entity.EntityID) {
	swaps, no, err := h.dabrishEpic.plan(p)
	switch {
	case err != nil:
		log.Printf("Dabrish: %s's epics: %v", p.Name, err)
		dabrishSay(sess, dabrishFailed)
		return
	case no != "":
		dabrishSay(sess, no)
		return
	}
	rows := bySwapRow(swaps)
	if len(rows) == 1 {
		h.dabrishEpicOffer(sess, p, npc, rows[0])
		return
	}
	rows = rows[:min(len(rows), dialogRoom(sess))]
	answers := make([]string, len(rows))
	swaps = nil
	for i, r := range rows {
		answers[i] = clipRunes(r[0].fromBox(), questAnswerChars)
		swaps = append(swaps, r...)
	}
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, dabrishEpicWhich, npc)
	a := h.dabrish.asked[p.CharServerID]
	a.swaps = swaps
	h.dabrish.asked[p.CharServerID] = a
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, dabrishEpicAskWhich, answers)
}

// dabrishEpicOffer is a row's exchanges: one asked Yes or No, several (a
// level 60 epic's others) shown and asked which.
func (h *InteractHandler) dabrishEpicOffer(sess *session.Session, p *entity.Player, npc entity.EntityID, row []epicSwap) {
	if len(row) == 1 {
		h.dabrishEpicAsk(sess, p, npc, row[0])
		return
	}
	row = row[:min(len(row), dialogRoom(sess))]
	names := make([]string, len(row))
	answers := make([]string, len(row))
	for i, s := range row {
		names[i] = s.toName
		answers[i] = clipRunes(s.toBox(), questAnswerChars)
	}
	showFor := dabrishEpicShowFor
	if row[0].set {
		showFor = dabrishEpicShowForSet
	}
	dabrishSay(sess, fmt.Sprintf(showFor, row[0].fromName, theNames(names)))
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, dabrishEpicFor, npc)
	a := h.dabrish.asked[p.CharServerID]
	a.swaps = row
	h.dabrish.asked[p.CharServerID] = a
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, clipRunes(fmt.Sprintf(dabrishEpicAskFor, row[0].fromBox()), questAskChars), answers)
}

// dabrishEpicAsk shows an exchange and asks Yes or No.
func (h *InteractHandler) dabrishEpicAsk(sess *session.Session, p *entity.Player, npc entity.EntityID, s epicSwap) {
	show := dabrishEpicShow
	switch {
	case s.set:
		show = dabrishEpicShowSetFrom
	case s.toSet:
		show = dabrishEpicShowSetTo
	case s.sixty:
		show = dabrishEpicShow60
	}
	dabrishSay(sess, fmt.Sprintf(show, s.fromName, s.toName))
	h.boardMu.Lock()
	id := h.dabrishAskLocked(p, dabrishEpicConfirm, npc)
	a := h.dabrish.asked[p.CharServerID]
	a.swaps = []epicSwap{s}
	h.dabrish.asked[p.CharServerID] = a
	h.boardMu.Unlock()
	SendQuestPopupMulti(sess, id, clipRunes(fmt.Sprintf(dabrishEpicAsk, s.fromBox(), s.toBox()), questAskChars),
		[]string{dabrishEpicYes, dabrishEpicNo})
}

// answerDabrishEpic is an answer to one of the exchange's boxes.
func (h *InteractHandler) answerDabrishEpic(sess *session.Session, p *entity.Player, asked dabrishAsk, n int) {
	switch asked.rng {
	case dabrishEpicWhich:
		if rows := bySwapRow(asked.swaps); n < len(rows) {
			h.dabrishEpicOffer(sess, p, asked.npc, rows[n])
		}
	case dabrishEpicFor:
		if n < len(asked.swaps) {
			h.dabrishEpicAsk(sess, p, asked.npc, asked.swaps[n])
		}
	case dabrishEpicConfirm:
		if len(asked.swaps) != 1 {
			return
		}
		if n != 0 {
			dabrishSay(sess, fmt.Sprintf(dabrishEpicNotNow, asked.swaps[0].fromName))
			return
		}
		h.dabrishEpicDo(sess, p, asked.swaps[0])
	}
}

// dabrishEpicDo carries out an exchange agreed to: planned again (refused
// when the row or what it gets moved), done in one transaction, the client
// told, the level 49 quest's kept pick moved with it.
func (h *InteractHandler) dabrishEpicDo(sess *session.Session, p *entity.Player, want epicSwap) {
	e := h.dabrishEpic
	swaps, no, err := e.plan(p)
	if err != nil {
		log.Printf("Dabrish: %s's epics: %v", p.Name, err)
		dabrishSay(sess, dabrishFailed)
		return
	}
	i := slices.IndexFunc(swaps, func(s epicSwap) bool {
		return s.itemID == want.itemID && s.from == want.from && s.toBase == want.toBase && s.to == want.to
	})
	if i < 0 {
		if no == "" {
			no = fmt.Sprintf(dabrishEpicMoved, want.fromName)
		}
		dabrishSay(sess, no)
		return
	}
	s := swaps[i]
	done, err := e.inv.ExchangeCharItem(p.CharServerID, database.ItemExchange{ItemID: s.itemID, More: s.more, From: s.from,
		To: s.to, Count: s.give, ImportedOf: s.importedOf, With: s.with, Also: s.also, Source: "Dabrish's epic exchange"}, e.now())
	if errors.Is(err, database.ErrExchangeGone) {
		dabrishSay(sess, fmt.Sprintf(dabrishEpicMoved, s.fromName))
		return
	}
	if err != nil {
		log.Printf("Dabrish: exchanging %s's %q (row %d): %v", p.Name, s.fromName, s.itemID, err)
		dabrishSay(sess, dabrishFailed)
		return
	}
	pat, ok, err := e.inv.ItemPatternByID(done.To)
	if err != nil || !ok {
		log.Printf("Dabrish: %s's new record %d: %v", p.Name, done.To, err)
	}
	if l := sessionInvList(sess); l != nil {
		gone := append([]int{s.itemID}, s.more...)
		for _, r := range s.with {
			gone = append(gone, r.ItemID)
		}
		for _, id := range gone {
			if at := l.indexOfItem(id); at >= 0 && l.took(at, 1) {
				queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(at, 0, 1))
			}
		}
		if ok {
			for _, id := range append([]int{done.ItemID}, done.More...) {
				rec := wireRecord(database.CharInventoryEntry{ItemID: id, StackLeft: 1, RemainHP: done.RemainHP,
					Charges: done.Charges, Pattern: pat})
				if key, took := l.add(id, done.To, 1, pat.MaxStack); took > 0 {
					queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key, readsCapes(sess)))
				}
			}
		}
		for _, g := range done.Also { // a set's other records
			pat, ok, err := e.inv.ItemPatternByID(g.To)
			if err != nil || !ok {
				log.Printf("Dabrish: %s's new record %d: %v", p.Name, g.To, err)
				continue
			}
			rec := wireRecord(database.CharInventoryEntry{ItemID: g.ItemID, StackLeft: 1, RemainHP: g.RemainHP,
				Charges: g.Charges, Pattern: pat})
			if key, took := l.add(g.ItemID, g.To, 1, pat.MaxStack); took > 0 {
				queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&rec, key, readsCapes(sess)))
			}
		}
	}
	moved := 0
	if !s.sixty { // a level 60 epic's quest keeps no pick a path change takes back
		moved = e.quests().moveEpicPick(p.CharServerID, s.fromBase, s.toBase, e.versionsBase())
	}
	log.Printf("Dabrish: %s exchanged %q (record %d, row %d, with %v) for %q (record %d, row %d, with %v), condition %d, charges %d, picks moved %d, free",
		p.Name, s.fromName, s.from, s.itemID, s.with, s.toName, done.To, done.ItemID, done.Also, done.RemainHP, done.Charges, moved)
	doneLine := dabrishEpicDone
	if s.toSet {
		doneLine = dabrishEpicDoneSet
	}
	dabrishSay(sess, fmt.Sprintf(doneLine, s.toName, s.fromName))
}

// versionsBase is a record's base, read now (moveEpicPick's).
func (e *dabrishEpics) versionsBase() func(int) int {
	asc, imp, err := e.inv.ItemVersions()
	if err != nil {
		log.Printf("Dabrish: the records' versions: %v", err)
	}
	v := epicVersions{asc: asc, imp: imp}
	return func(id int) int { b, _ := v.base(id); return b }
}

// moveEpicPick moves the character's kept level 49 weapon picks from the
// option giving from to the one giving to with the same spells (the cleric's
// "Elemental Bulwark and the Ascended Mace of Repentence"), as a
// rehabilitator's change keeps one (setPick). It returns how many it moved.
func (q *QuestEngine) moveEpicPick(serverID uint32, from, to int, base func(int) int) int {
	if q == nil {
		return 0
	}
	gives := func(o database.StepOption, b int) bool {
		for _, it := range o.Reward.Items {
			if base(it.PatternID) == b {
				return true
			}
		}
		return false
	}
	moved := 0
	for _, pq := range q.pathQuests(serverID) {
		if pq.level != 49 {
			continue
		}
		picks := database.ParsePicks(pq.row.Picks)
		for i := range pq.quest.Steps {
			st := &pq.quest.Steps[i]
			opts := st.Choices.Options
			k, ok := picks[st.Step]
			if !ok || k >= len(opts) || !gives(opts[k], from) {
				continue
			}
			for j, o := range opts {
				if j != k && gives(o, to) && slices.Equal(o.Reward.Spells, opts[k].Reward.Spells) {
					if q.setPick(serverID, pq.quest.ID, st.Step, j) {
						moved++
					}
					break
				}
			}
		}
	}
	return moved
}
