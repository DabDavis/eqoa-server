package handlers

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// cmChain is VIWndClassMastery::Chain (decomp viwndclassmastery.cpp): out of
// the skills in, the first whose parent is no listed skill's key, then one at
// a time the first whose parent is the last one's key; the rest keep their
// order. nil when every skill's parent is listed (the window stops there).
func cmChain(in []cmSkill) (chain, rest []cmSkill) {
	root := -1
	for i, a := range in {
		if !slices.ContainsFunc(in, func(b cmSkill) bool { return a.parent == b.key }) {
			root = i
			break
		}
	}
	if root < 0 {
		return nil, in
	}
	chain = []cmSkill{in[root]}
	rest = slices.Delete(slices.Clone(in), root, root+1)
	for id := in[root].key; ; {
		i := slices.IndexFunc(rest, func(s cmSkill) bool { return s.parent == id })
		if i < 0 {
			return chain, rest
		}
		id = rest[i].key
		chain = append(chain, rest[i])
		rest = slices.Delete(rest, i, i+1)
	}
}

// cmWindow is what CLIENT's Class Mastery window shows of a category
// (VIWndClassMastery::SelectCategory): the owned skills, and the rest known
// first, each sorted into chains; the Owned list shows each chain's LAST
// skill, the Available list each chain's FIRST, its name and cost when known,
// else "???" ("Locked").
func cmWindow(skills []cmSkill, category int) (owned, available []string) {
	var lists [2][]cmSkill
	for _, s := range skills {
		if s.cat == category && (s.owned || s.known) {
			lists[b2i(!s.owned)] = append(lists[b2i(!s.owned)], s)
		}
	}
	for _, s := range skills {
		if s.cat == category && !s.owned && !s.known {
			lists[1] = append(lists[1], s)
		}
	}
	for in := lists[0]; len(in) > 0; {
		c, rest := cmChain(in)
		if c == nil {
			break
		}
		owned, in = append(owned, c[len(c)-1].name), rest
	}
	for in := lists[1]; len(in) > 0; {
		c, rest := cmChain(in)
		if c == nil {
			break
		}
		line := "???"
		if c[0].known {
			line = fmt.Sprintf("%s %d", c[0].name, c[0].cost)
		}
		available, in = append(available, line), rest
	}
	return owned, available
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cmFlagsAfter is what CLIENT keeps of a purchase's (Recv05) or a removal's
// (Recv06) reply: the known flags of every skill listed (ReadFlags) and the
// rank's owned flag; the keys and parents stay the list's, which only the
// next list (the window opened again) brings.
func cmFlagsAfter(t *testing.T, list []cmSkill, reply []byte, owned bool) []cmSkill {
	t.Helper()
	r := &cmReader{b: reply, o: 5}
	id := int(r.u32())
	if owned {
		r.u32()
		r.u32()
	}
	st := r.state()
	if r.err != nil || r.o != len(r.b) {
		t.Fatalf("the reply does not read as CLIENT reads it: %v", r.err)
	}
	out := slices.Clone(list)
	for i := range out {
		if k, ok := st.flags[out[i].id]; ok {
			out[i].known = k
		}
		if out[i].id == id {
			out[i].owned = owned
		}
	}
	return out
}

// masterCatalog is testCatalog's other categories and two warrior master
// classes as tools/seed_masteries.py writes them now (F3 20261026-222613):
// Bulwark, whose abilities Guard and Toughness need only it and whose
// Immortal Guard (free) improves Guard; and Hero, with two abilities.
func masterCatalog() []database.CMRankRow {
	var rows []database.CMRankRow
	for _, r := range testCatalog() {
		if r.Category != 4 {
			rows = append(rows, r)
		}
	}
	mc := func(id, rank, parent, cost int, mastery, name string) database.CMRankRow {
		r := database.CMRankRow{ID: id, Category: 4, Classes: 1 << 0, Races: 2047, Mastery: mastery, Rank: rank,
			Name: name, Parent: parent, Cost: cost, Level: 60, Enabled: true}
		if rank == 1 {
			r.Level, r.Spent = 55, 500
		}
		return r
	}
	return append(rows,
		mc(50, 1, 0, 200, "Bulwark", "Bulwark"),
		mc(51, 2, 50, 75, "Bulwark", "Guard"),
		mc(52, 3, 50, 75, "Bulwark", "Toughness"),
		mc(53, 4, 51, 0, "Bulwark", "Immortal Guard"),
		mc(60, 1, 0, 200, "Hero", "Hero"),
		mc(61, 2, 60, 75, "Hero", "Mantle"),
		mc(62, 3, 60, 75, "Hero", "Blow"))
}

// A master class's abilities need only the class (jib, F3 20261026-222613:
// "Tag *unlocks* the 2"), and CLIENT's window, which sorts a category into
// chains by the parent keys the list gives (Recv08, at its opening) and keeps
// them through a purchase (Recv05 brings flags alone), shows it so: locked,
// each master class one "???"; buyable, the class (each other ability one
// "???" meanwhile); bought, every ability available at once, in the same
// window too, bought in any order, each owned one beside the class in Owned,
// and an upgrade (Immortal Guard) shown in Guard's place once owned. The
// controls: the catalog's own parents sent as they are leave a locked master
// class two lines and hide the class in Owned once an ability is bought. The
// rules hold: one master class, the class not removed before its abilities,
// Guard not before Immortal Guard.
func TestCMWindowReadsAMasterClassesAbilities(t *testing.T) {
	c := newCMTest(t, 0, 0, 54)
	c.w.gw.SetCMCatalog(masterCatalog())
	c.p.Mastery.Points, c.p.Mastery.Spent = 1000, 500
	want := func(what string, list []cmSkill, owned, available []string) {
		t.Helper()
		o, a := cmWindow(list, 4)
		if !slices.Equal(o, owned) || !slices.Equal(a, available) {
			t.Errorf("%s: Owned %q, Available %q; want %q, %q", what, o, a, owned, available)
		}
	}
	buy := func(list []cmSkill, id int) []cmSkill {
		t.Helper()
		rs, red := c.send(cmBuy, id)
		if len(red) != 0 || len(rs) != 1 || rs[0][0] != cmRespBought {
			t.Fatalf("buying %d: % x %v", id, rs, red)
		}
		return cmFlagsAfter(t, list, rs[0], true)
	}

	want("level 54, both locked", c.list(), nil, []string{"???", "???"})
	asSent := func(list []cmSkill) []cmSkill { // the control: the catalog's parents
		out := slices.Clone(list)
		for i := range out {
			if r := c.w.gw.CMRankByID(out[i].id); r != nil {
				out[i].parent = r.Parent
			}
		}
		return out
	}
	if _, a := cmWindow(asSent(c.list()), 4); len(a) != 4 {
		t.Errorf("the control: the catalog's parents as sent show %q, not a stray line an ability", a)
	}

	c.p.Level = 60
	list := c.list()
	want("buyable", list, nil, []string{"Bulwark 200", "Hero 200", "???", "???"})
	if n, ok := c.w.gw.CMNodeOf(c.sess, 52, entity.CMKnows{}); !ok || !slices.Equal(n.Masteries, []string{"Bulwark"}) {
		t.Errorf("Toughness before Bulwark: needs %q, want the class", n.Masteries)
	}
	list = buy(list, 50)
	want("Bulwark bought, the same window", list, []string{"Bulwark"}, []string{"Guard 75", "Toughness 75", "???", "???"})
	want("opened again", c.list(), []string{"Bulwark"}, []string{"Guard 75", "Toughness 75", "???"})
	if _, red := c.send(cmBuy, 60); len(red) != 1 || !strings.Contains(red[0], "only one Master Class") {
		t.Errorf("a second master class: %v", red)
	}

	list = buy(c.list(), 52) // the second ability first
	want("Toughness bought", list, []string{"Bulwark", "Toughness"}, []string{"Guard 75", "???"})
	want("opened again", c.list(), []string{"Bulwark", "Toughness"}, []string{"Guard 75", "???"})
	if o, _ := cmWindow(asSent(c.list()), 4); slices.Contains(o, "Bulwark") {
		t.Errorf("the control: the catalog's parents as sent show Owned %q, the class hidden", o)
	}
	if _, red := c.send(cmBuy, 53); len(red) != 1 || !strings.Contains(red[0], "Guard first") {
		t.Errorf("Immortal Guard before Guard: %v", red)
	}
	list = buy(c.list(), 51)
	want("Guard bought", list, []string{"Bulwark", "Guard", "Toughness"}, []string{"Immortal Guard 0", "???"})
	buy(c.list(), 53)
	want("Immortal Guard bought", c.list(), []string{"Bulwark", "Immortal Guard", "Toughness"}, []string{"???"})

	for id, why := range map[int]string{50: "before Bulwark", 51: "Remove Immortal Guard before Guard"} {
		if _, red := c.send(cmRemove, id); len(red) != 1 || !strings.Contains(red[0], why) {
			t.Errorf("removing %d: %v, want %q", id, red, why)
		}
	}
	if rs, red := c.send(cmRemove, 52); len(red) != 0 || rs[0][0] != cmRespRemoved {
		t.Errorf("removing Toughness, which nothing needs: % x %v", rs, red)
	}
	want("Toughness removed", c.list(), []string{"Bulwark", "Immortal Guard"}, []string{"Toughness 75", "???"})
}
