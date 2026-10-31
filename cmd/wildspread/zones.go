package main

import (
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Zones with creatures of their own (the user, 2026-10-31, on Rathe's west:
// "envar, dead hills have different mobs, undead", "geomancers citadel has
// the archon bosses, betrayer of sarek, grundle, dryads etc", "cyclops are
// in both"; and their answers that day: Geomancer's Citadel left as it is,
// Cyclops Fortress's cyclopes at 40-46, West Feerrott's Tae Ew and ogre and
// dwarf thugs at 18-22). A zone is CLIENT's (ZoneName: the decomp's
// vientity.cpp sZones0 and sZones1, a WORDS.TXT word for each 2,000-unit
// cell); a rule says what its fill is, in place of the wildlife round it:
//
//   - leave: nothing moved in it and nothing added (a zone of bosses and
//     quest creatures);
//   - undead: the undead round it (by name, npcundead or body), as the
//     wildlife's kinds are drawn, at the place's level;
//   - kinds: those kinds alone, at the rule's levels, and camps of them at
//     points of EQOA-Data's map (camps and quest points, through Rathe's
//     fit), placed before the fill.

// zones1 is CLIENT's zone grid of Rathe (sZones1, 0x00CF39C0): row z / 2000
// - 1, column x / 2000 - 1.
var zones1 = [][]int{
	{641, 1177, 1181, 1185},
	{641, 1178, 1182, 1186},
	{641, 1179, 1183, 1187},
	{641, 1180, 1184, 1188},
}

// zoneWord is the WORDS.TXT word naming the zone at (x, z) of world w, 641
// (none) off its grid or for a world without one here.
func zoneWord(w int, x, z float64) int {
	var grid [][]int
	switch w {
	case 0:
		grid = zones0
	case 1:
		grid = zones1
	default:
		return 641
	}
	row, col := int(math.Floor(z/2000))-1, int(math.Floor(x/2000))-1
	if row < 0 || row >= len(grid) || col < 0 || col >= len(grid[row]) {
		return 641
	}
	return grid[row][col]
}

// zoneAt is the zone's name at (x, z) of world w ("" with no WORDS.TXT).
func (p *plan) zoneAt(w int, x, z float64) string {
	word := zoneWord(w, x, z)
	if word >= len(p.words) {
		return ""
	}
	return strings.TrimSpace(p.words[word])
}

type ruleKind struct {
	name   string
	lo, hi int     // the levels it is made at
	weight float64 // its share of the zone's fill
}

// ruleCamp is a camp of a rule's kinds: at a point of EQOA-Data's map (mx,
// my, laid over the world by campFits), count of them round it.
type ruleCamp struct {
	what   string // what the map calls it
	mx, my float64
	kinds  []string // drawn by the rule's weights; all the rule's when empty
	count  int
}

type zoneRule struct {
	world  int
	zone   string // as WORDS.TXT names it
	leave  bool
	undead bool
	kinds  []ruleKind
	camps  []ruleCamp
}

// zoneRules, the user's of 2026-10-31. Geomancer's Pass is left as the
// Citadel is: its creatures are the Sarek story's (Archon Seradesh, the
// Anvil of Sarek, Xalakharx, the spectral warlord), EQOA-Data's quest
// points all.
var zoneRules = []zoneRule{
	{world: 1, zone: "Geomancer's Citadel", leave: true},
	{world: 1, zone: "Geomancer's Pass", leave: true},
	{world: 1, zone: "Dead Hills", undead: true},
	{world: 1, zone: "Envar", undead: true},
	{world: 1, zone: "Cyclops Fortress",
		kinds: []ruleKind{
			{"cyclops", 40, 44, 12}, {"cyclops guard", 43, 45, 5}, {"cyclops warder", 44, 46, 4},
			{"cyclops elder", 46, 46, 2}, {"cyclops elite guard", 45, 46, 2},
		},
		camps: []ruleCamp{{what: "Cyclops Fortress", mx: 5878, my: 9037, count: 10}}},
	{world: 1, zone: "West Feerrott",
		kinds: []ruleKind{
			// the quests' "ogre thugs of West Feerot" are this data's "orge thug"
			{"orge thug", 18, 21, 3}, {"dwarf thug", 19, 21, 2}, {"tae ew commander", 21, 22, 2},
		},
		camps: []ruleCamp{
			{what: "Tae Ew Commander (22)", mx: 5955, my: 11930, kinds: []string{"tae ew commander"}, count: 5},
			{what: "Ogre Thug (20)", mx: 6271, my: 11631, kinds: []string{"orge thug"}, count: 5},
			{what: "Dwarf Thug (20)", mx: 6281, my: 11658, kinds: []string{"dwarf thug"}, count: 4},
		}},
}

func init() {
	// Rathe's fit (tools/gen_camp_teles.py's least squares on the map's
	// points that name one of Rathe's NPCs, 2026-10-31: 23 of 30, median 73
	// off), the map's Rathe sheet drawn in Tunaria's empty south-west.
	campFits[1] = [4]float64{2.12356, -5529.1, 2.13161, -16186.8}
}

// ruleAt is the rule of the zone at (x, z) of world w, nil for none.
func (p *plan) ruleAt(w int, x, z float64) *zoneRule {
	if p.words == nil {
		return nil
	}
	name := p.zoneAt(w, x, z)
	for i := range zoneRules {
		if zoneRules[i].world == w && strings.EqualFold(zoneRules[i].zone, name) {
			return &zoneRules[i]
		}
	}
	return nil
}

// anyKind is a kind by name for a rule: every row of the name a player
// fights, in any world (the body a copy takes; a players' race body too: the
// rule names it, an ogre thug is an ogre), nil with none.
func (p *plan) anyKind(name string) *kind {
	key := database.NPCClassKey(name)
	if k, ok := p.ruled[key]; ok {
		return k
	}
	var rows []*npcRow
	for _, r := range p.in.rows {
		if r.key == key && r.model != 0 && r.typ&0x80 == 0 && !r.spawnOnly {
			rows = append(rows, r)
		}
	}
	var k *kind
	if len(rows) > 0 {
		k = &kind{key: key, spelling: spelling(rows), rows: rows, spots: spotCount(rows)}
	}
	if p.ruled == nil {
		p.ruled = map[string]*kind{}
	}
	p.ruled[key] = k
	return k
}

// pickRuleKind draws one of the rule's kinds (of only, when given) by its
// weight, and a level in its range.
func (p *plan) pickRuleKind(r *zoneRule, only []string, rng *rand.Rand) (*kind, int) {
	var ks []ruleKind
	total := 0.0
	for _, rk := range r.kinds {
		if len(only) > 0 {
			in := false
			for _, o := range only {
				in = in || o == rk.name
			}
			if !in {
				continue
			}
		}
		if p.anyKind(rk.name) == nil {
			continue
		}
		ks = append(ks, rk)
		total += rk.weight
	}
	if len(ks) == 0 {
		return nil, 0
	}
	pick := rng.Float64() * total
	chosen := ks[len(ks)-1]
	for _, rk := range ks {
		pick -= rk.weight
		if pick <= 0 {
			chosen = rk
			break
		}
	}
	return p.anyKind(chosen.name), chosen.lo + rng.Intn(chosen.hi-chosen.lo+1)
}

// undeadBodies are the bodies of the creatures whose names are undead's, at
// least three of them: a creature of another name in one is undead too (a
// grave usher is a skeleton).
func (p *plan) undeadBodies() map[uint32]bool {
	if p.deadBodies != nil {
		return p.deadBodies
	}
	n := map[uint32]int{}
	for _, r := range p.in.rows {
		if r.model != 0 && entity.UndeadName(r.name) {
			n[r.model]++
		}
	}
	p.deadBodies = map[uint32]bool{}
	for m, c := range n {
		if c >= 3 {
			p.deadBodies[m] = true
		}
	}
	return p.deadBodies
}

// undead reports a row the world counts undead: npcundead's word for its
// name, else its name or its body.
func (p *plan) undead(r *npcRow) bool {
	if u, ok := p.in.undead[r.key]; ok {
		return u
	}
	return entity.UndeadName(r.name) || p.undeadBodies()[r.model]
}

// undeadKinds are the kinds a zone of the dead copies in world w: undead,
// more than one of them, mostly on the open ground (a tower's grave usher is
// a quest's, not the land's), out of the water, no players' race.
func (p *plan) undeadKinds(w int) map[string]*kind {
	if k, ok := p.deadKinds[w]; ok {
		return k
	}
	by := map[string][]*npcRow{}
	for _, r := range p.in.rows {
		if r.world == w && creature(r) && !p.in.players[r.model] && p.undead(r) {
			by[r.key] = append(by[r.key], r)
		}
	}
	out := map[string]*kind{}
	for key, rows := range by {
		open, wet := 0, 0
		for _, r := range rows {
			if r.open {
				open++
			}
			if r.wet {
				wet++
			}
		}
		if len(rows) < 2 || open*2 < len(rows) || wet*5 > len(rows) {
			continue
		}
		out[key] = &kind{key: key, spelling: spelling(rows), rows: rows, spots: spotCount(rows)}
	}
	if p.deadKinds == nil {
		p.deadKinds = map[int]map[string]*kind{}
	}
	p.deadKinds[w] = out
	return out
}

// placeCamps places the rules' camps of world w before the fill: each
// camp's count of its kinds on the open land within campSpread of its point,
// campSpacing apart and clear of every creature; what was placed.
func (p *plan) placeCamps(land *landGrid, w int, open func(int) bool, occupied *index, rng *rand.Rand) []fill {
	var out []fill
	for i := range zoneRules {
		r := &zoneRules[i]
		if r.world != w {
			continue
		}
		for _, c := range r.camps {
			cx, cz, ok := campPoint{c.what, c.mx, c.my}.at(w)
			if !ok {
				continue
			}
			// the open samples round the point, nearest first
			type cand struct {
				k int
				d float64
			}
			var cands []cand
			n := int(math.Ceil(campSpread / step))
			c0 := land.cell(cx, cz)
			if c0 < 0 {
				say("camp %q: its point (%.0f, %.0f) is off world %d's land", c.what, cx, cz, w)
				continue
			}
			for dj := -n; dj <= n; dj++ {
				for di := -n; di <= n; di++ {
					i, j := c0%land.nx+di, c0/land.nx+dj
					if i < 0 || j < 0 || i >= land.nx || j >= land.nz {
						continue
					}
					k := land.at(i, j)
					x, z := land.xz(k)
					if d := math.Hypot(x-cx, z-cz); d <= campSpread && open(k) {
						cands = append(cands, cand{k, d})
					}
				}
			}
			rng.Shuffle(len(cands), func(a, b int) { cands[a], cands[b] = cands[b], cands[a] })
			placed := 0
			for _, cd := range cands {
				if placed >= c.count {
					break
				}
				x, z := land.xz(cd.k)
				if occupied.within(x, z, campSpacing) {
					continue
				}
				k, level := p.pickRuleKind(r, c.kinds, rng)
				if k == nil {
					break
				}
				fx, fy, fz := p.settle(land, cd.k, rng)
				out = append(out, fill{kind: k, template: template(k, w, fx, fz), level: level, x: fx, y: fy, z: fz,
					facing: rng.Intn(256), source: "camp", placeLevel: level})
				occupied.add(fx, fz, -1)
				placed++
			}
			say("camp %q in %s: %d of %d placed round (%.0f, %.0f)", c.what, r.zone, placed, c.count, cx, cz)
		}
	}
	return out
}

// A camp's spread round its point and its creatures' spacing.
const (
	campSpread  = 160.0
	campSpacing = 18.0
)

// zoneLevel is the level of the zone at (x, z) of world w by its own
// creatures (the median, the players' race bodies left out), 0 with none: a
// zone of the dead's fill keeps to its own levels, not its neighbours'.
func (p *plan) zoneLevel(w int, x, z float64) int {
	word := zoneWord(w, x, z)
	key := [2]int{w, word}
	if l, ok := p.zoneLevels[key]; ok {
		return l
	}
	var ls []int
	for _, r := range p.in.rows {
		if r.world == w && creature(r) && !p.in.players[r.model] && zoneWord(w, r.x, r.z) == word {
			ls = append(ls, r.level)
		}
	}
	l := 0
	if len(ls) > 0 {
		sort.Ints(ls)
		l = ls[len(ls)/2]
	}
	if p.zoneLevels == nil {
		p.zoneLevels = map[[2]int]int{}
	}
	p.zoneLevels[key] = l
	return l
}
