package handlers

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/ground"
)

// questcheck_real_test.go — every step of the low quests can be done in the running
// game (the owner, 2026-11-14: "make sure all the quest triggers work for the 1-20
// quests").
//
// TestRealQuestTriggers checks, on a real server's database (EQOA_REAL_DB names it; a
// copy is read), every enabled quest of level EQOA_QUESTCHECK_LEVEL (20) and under, with
// the server's own engine and world as the game server loads them (the NPCs, their
// factions and aggro rules, the spawn-only originals, the GM triggers, the quests'
// catalog): what each step asks of the world is there.
//
//   - giver, talk, show and give steps: an NPC the engine matches (stepNPC: its name
//     trimmed, any case, a pinned spawn by its row) stands in the world (not only as a
//     spawn-only original, unless a spawn of the quest or a trigger it fires makes one),
//     is no sign post (signs answer before the quests), does not attack on sight every
//     race and class the quest is for (the World's own target face, at the quest's
//     level, with their starting standings), and stands on ground a player reaches (a
//     flood over the ground and its walls from it gets clear of it; EQOA_GROUND_DIR); a
//     name the engine does not match but one differing by a ` or ' or one letter is
//     named (the R'Nox / R`Noxt kind);
//   - kill and loot steps: a creature of the step's names spawns (or a spawn event or a
//     trigger of the quest makes one), not far above the quest's level, reachable;
//   - a loot step's and a hand-in's items: each record exists, a Lore one is never
//     asked for more than once, and each can be had: the step's own drop, an earlier
//     step's or a required quest's reward or give, another quest's drop or reward its
//     class and race may have, a creature's or a merchant's npcitem, a loot table bound
//     to a creature that spawns, a category pool, a recipe, a starting item, a trigger's
//     give;
//   - reach steps: a place with ground under it, reachable;
//   - events: each reads (EventProblems), its spawn's NPC has a row, its trigger a row
//     that is enabled and reads, its items records;
//   - requires: every group can be met by a quest that exists, is enabled and is for
//     the class and race (no quest reached only through a disabled one);
//   - masks: a city class quest (EQOA-Data 1-20/<city>/<class>) is for that class, and
//     for every race whose start for the class is in that city;
//   - rewards: items and spells exist, a reward scroll scribes a spell, a by-class
//     reward covers each class the quest is for.
//
// Then every quest is walked through the engine (questWalker, questwalk_real_test.go)
// as a player does it, the items a hand-in wants given from the source found above
// (bought, looted), and must be finished.
//
// A finding is "broken" (the step cannot be done; the test fails) or "warn" (it can,
// but something is off). EQOA_QUESTCHECK_OUT names a file for the full report.
func TestRealQuestTriggers(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	top := 20
	if v := os.Getenv("EQOA_QUESTCHECK_LEVEL"); v != "" {
		top, _ = strconv.Atoi(v)
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	c := newQuestCheck(t, src, top)
	defer c.w.db.Close()
	c.static()
	c.walk()
	c.report()
}

// qcFinding is one thing found wrong with a quest.
type qcFinding struct {
	broken      bool
	kind        string
	quest, step int
	text        string
}

// questCheck is the check of one database.
type questCheck struct {
	t   *testing.T
	w   *questWalker
	top int
	g   *ground.Ground
	env *ground.Env

	quests  []*database.Quest       // the quests checked, by id
	byFold  map[string][]*entity.NPC // every NPC by its folded name
	loose   map[string][]string      // folded names by their loose key (no ` ' - space)
	signs   map[int]database.SignText
	trigs   map[string]database.GMTrigger
	sources map[int][]qcSource // item -> where it can be had
	reach   map[entity.EntityID]string
	combos  [][3]int // class, race, humtype a character may be
	items   *database.ItemPatternStore
	spells  map[int]string
	names   []database.SpellName

	findings []qcFinding
	steps    int
	used     []string // each hand-in item's source, for the report
}

// qcSource is one way to have an item.
type qcSource struct {
	kind  string // reward, give, drop (a quest's loot step), npcitem, loottable, pool, recipe, start, trigger
	quest int    // reward, give, drop: the quest
	step  int    // the step (0: the quest's own events)
	what  string
}

func newQuestCheck(t *testing.T, src string, top int) *questCheck {
	w := newQuestWalker(t, src)
	c := &questCheck{t: t, w: w, top: top, byFold: map[string][]*entity.NPC{}, loose: map[string][]string{},
		trigs: map[string]database.GMTrigger{}, sources: map[int][]qcSource{}, reach: map[entity.EntityID]string{},
		items: database.NewItemPatternStore(w.db), spells: map[int]string{}}
	db, gw := w.db, w.gw
	// the world as cmd/gameserver loads it, the parts a quest asks of
	npcStore := database.NewNPCStore(db)
	if models, err := npcStore.PlayerModels(); err == nil {
		gw.SetPlayerModels(models)
	}
	gw.LoadNPCs(npcStore)
	if err := gw.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gw.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.LoadQuestWalls(db); err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("EQOA_GROUND_DIR"); dir != "" {
		g, err := ground.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		c.g = g
		t.Cleanup(g.Close)
		if e, err := ground.OpenEnv(dir); err == nil {
			c.env = e
		}
	} else {
		t.Log("EQOA_GROUND_DIR not set: no NPC's or place's ground is checked")
	}
	for _, n := range gw.NPCs {
		k := foldName(n.Name)
		if len(c.byFold[k]) == 0 {
			c.loose[looseName(n.Name)] = append(c.loose[looseName(n.Name)], k)
		}
		c.byFold[k] = append(c.byFold[k], n)
	}
	var err error
	if c.signs, err = db.SignTexts(); err != nil {
		t.Fatal(err)
	}
	all, err := db.GMTriggers()
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range all {
		c.trigs[foldName(tr.Name)] = tr
	}
	rows, err := db.Query(`SELECT DISTINCT tclass, race, humType FROM defaultcharacter ORDER BY tclass, race, humType`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var cl, r, h int
		rows.Scan(&cl, &r, &h)
		c.combos = append(c.combos, [3]int{cl, r, h})
	}
	rows.Close()
	rows, err = db.Query(`SELECT spellid, spellname FROM spellpattern`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		var n string
		rows.Scan(&id, &n)
		c.spells[id] = n
	}
	rows.Close()
	if c.names, err = database.NewSpellbookStore(db).SpellNames(); err != nil {
		t.Fatal(err)
	}
	w.qe.mu.Lock()
	for _, qu := range w.qe.cat.all {
		if qu.Enabled && qu.MinLevel <= top {
			c.quests = append(c.quests, qu)
		}
	}
	w.qe.mu.Unlock()
	sort.Slice(c.quests, func(i, j int) bool { return c.quests[i].ID < c.quests[j].ID })
	c.readSources()
	return c
}

// looseName is a name with case, spaces, apostrophes, backticks and hyphens taken out:
// two names the engine tells apart that a player cannot.
func looseName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '`', '\'', '-', '_', '.', '’':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// oneEdit reports a and b one letter apart (a letter changed, added or taken out).
func oneEdit(a, b string) bool {
	if a == b || len(a)-len(b) > 1 || len(b)-len(a) > 1 {
		return false
	}
	if len(a) < len(b) {
		a, b = b, a
	}
	i := 0
	for i < len(b) && a[i] == b[i] {
		i++
	}
	if len(a) == len(b) {
		return a[i+1:] == b[i+1:]
	}
	return a[i+1:] == b[i:]
}

func (c *questCheck) add(broken bool, kind string, qu *database.Quest, step int, format string, a ...any) {
	c.findings = append(c.findings, qcFinding{broken: broken, kind: kind, quest: qu.ID, step: step, text: fmt.Sprintf(format, a...)})
}

// standing is an NPC in the world for the players (not a spawn-only original, not a
// copy or a pet).
func standing(n *entity.NPC) bool {
	return !n.SpawnOnly && !n.Temporary && n.Owner == 0
}

// readSources reads every place an item can be had.
func (c *questCheck) readSources() {
	db := c.w.db
	addSrc := func(id int, s qcSource) { c.sources[id] = append(c.sources[id], s) }
	// the quests' rewards, gives and drops (every enabled quest's, of any level)
	c.w.qe.mu.Lock()
	for _, qu := range c.w.qe.cat.all {
		if !qu.Enabled {
			continue
		}
		for _, e := range qu.Events {
			if e.Do == database.EventGive {
				for _, it := range database.ParseItemCounts(e.Items) {
					addSrc(it.PatternID, qcSource{kind: "give", quest: qu.ID, what: "the quest's " + e.On})
				}
			}
		}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			for _, it := range st.RewardItems {
				addSrc(it.PatternID, qcSource{kind: "reward", quest: qu.ID, step: st.Step})
			}
			for _, rw := range st.ByClass {
				for _, it := range rw.Items {
					addSrc(it.PatternID, qcSource{kind: "reward", quest: qu.ID, step: st.Step, what: "by class"})
				}
			}
			for _, o := range st.Choices.Options {
				for _, it := range o.Reward.Items {
					addSrc(it.PatternID, qcSource{kind: "reward", quest: qu.ID, step: st.Step, what: "a choice"})
				}
			}
			for _, e := range st.Events {
				if e.Do == database.EventGive {
					for _, it := range database.ParseItemCounts(e.Items) {
						addSrc(it.PatternID, qcSource{kind: "give", quest: qu.ID, step: st.Step, what: e.On})
					}
				}
			}
			if st.Kind == database.StepLoot {
				if pid, _, ok := st.LootItem(); ok {
					addSrc(pid, qcSource{kind: "drop", quest: qu.ID, step: st.Step})
				}
			}
		}
	}
	c.w.qe.mu.Unlock()
	// a creature's or a merchant's own items (npcitem), of an NPC that stands
	rows, err := db.Query(`SELECT npcid, patternid FROM npcitem`)
	if err != nil {
		c.t.Fatal(err)
	}
	for rows.Next() {
		var npc, pid int
		rows.Scan(&npc, &pid)
		if n := c.w.gw.NPCs[entity.EntityID(npc)]; n != nil && standing(n) {
			addSrc(pid, qcSource{kind: "npcitem", what: fmt.Sprintf("%s (%d)", n.Name, npc)})
		}
	}
	rows.Close()
	// a loot table bound to a creature of a name that stands (or every creature's)
	rows, err = db.Query(`SELECT b.npc_name, t.patternid, t.minlevel, t.maxlevel FROM npcloot b JOIN loottables t ON t.tablename = b.tablename
		WHERE b.chance > 0 AND t.weight > 0`)
	if err != nil {
		c.t.Fatal(err)
	}
	for rows.Next() {
		var name string
		var pid, lo, hi int
		rows.Scan(&name, &pid, &lo, &hi)
		if name == lootEveryCreature {
			addSrc(pid, qcSource{kind: "loottable", what: "every creature"})
			continue
		}
		for _, n := range c.byFold[foldName(name)] {
			if standing(n) && n.Level >= lo && n.Level <= hi {
				addSrc(pid, qcSource{kind: "loottable", what: n.Name})
				break
			}
		}
	}
	rows.Close()
	for _, q := range []struct{ kind, sql string }{
		{"pool", `SELECT patternid, npc_name FROM lootpool WHERE weight > 0`},
		{"recipe", `SELECT product, name FROM craftrecipes`},
		{"start", `SELECT patternid, race || '/' || class FROM startitems`},
	} {
		rows, err = db.Query(q.sql)
		if err != nil {
			c.t.Logf("%s: %v", q.kind, err)
			continue
		}
		for rows.Next() {
			var pid int
			var what string
			rows.Scan(&pid, &what)
			if q.kind == "pool" && strings.TrimSpace(what) != "" {
				// a creature's own pool: of a name that stands
				up := false
				for _, n := range c.byFold[foldName(what)] {
					up = up || standing(n)
				}
				if !up {
					continue
				}
			}
			addSrc(pid, qcSource{kind: q.kind, what: what})
		}
		rows.Close()
	}
	// what the triggers give (a trigger's give step)
	for _, tr := range c.trigs {
		steps, err := database.ParseTriggerSteps(tr.Steps)
		if err != nil || !tr.Enabled {
			continue
		}
		for _, s := range steps {
			if s.Do == database.TriggerGive {
				for _, it := range database.ParseItemCounts(s.Items) {
					addSrc(it.PatternID, qcSource{kind: "trigger", what: tr.Name})
				}
			}
		}
	}
}

// combosFor is the classes, races and human kinds a character taking qu may be.
func (c *questCheck) combosFor(qu *database.Quest) [][3]int {
	var out [][3]int
	for _, cr := range c.combos {
		if (len(qu.Classes) == 0 || containsInt(qu.Classes, cr[0])) && (len(qu.Races) == 0 || questRaceTakes(qu.Races, cr[1])) {
			out = append(out, cr)
		}
	}
	return out
}

// spawnedBy is the NPC names a quest's events and the triggers it fires make, and the
// triggers it fires.
func (c *questCheck) spawnedBy(qu *database.Quest) (names map[string]string, fired []string) {
	names = map[string]string{}
	fire := func(evs []database.QuestEvent, where string) {
		for _, e := range evs {
			switch e.Do {
			case database.EventSpawn:
				names[foldName(e.NPC)] = where + "'s spawn"
			case database.EventTrigger:
				fired = append(fired, e.Trigger)
				if tr, ok := c.trigs[foldName(e.Trigger)]; ok {
					steps, _ := database.ParseTriggerSteps(tr.Steps)
					for _, s := range steps {
						if s.Do == database.TriggerSpawn || s.Do == database.TriggerResummon {
							names[foldName(s.NPC)] = "trigger " + tr.Name
						}
					}
				}
			}
		}
	}
	fire(qu.Events, "the quest")
	for i := range qu.Steps {
		fire(qu.Steps[i].Events, fmt.Sprintf("step %d", qu.Steps[i].Step))
	}
	return names, fired
}

// anyTriggerSpawns is a trigger (any, enabled) that spawns the name.
func (c *questCheck) anyTriggerSpawns(name string) string {
	for _, tr := range c.trigs {
		if !tr.Enabled {
			continue
		}
		steps, _ := database.ParseTriggerSteps(tr.Steps)
		for _, s := range steps {
			if (s.Do == database.TriggerSpawn || s.Do == database.TriggerResummon) && foldName(s.NPC) == foldName(name) {
				return tr.Name
			}
		}
	}
	return ""
}

// near is the names one ` ' or letter off name the world has (not name itself).
func (c *questCheck) near(name string) []string {
	k, l := foldName(name), looseName(name)
	var out []string
	for _, f := range c.loose[l] {
		if f != k {
			out = append(out, f)
		}
	}
	if len(l) >= 5 {
		for ll, fs := range c.loose {
			if oneEdit(l, ll) {
				out = append(out, fs...)
			}
		}
	}
	sort.Strings(out)
	return out
}

// stepNPCs is the NPCs of the world a step (or the giver: st nil) is done at.
func (c *questCheck) stepNPCs(st *database.QuestStep, name string) []*entity.NPC {
	var out []*entity.NPC
	names := []string{name}
	if st != nil && database.Hunts(st.Kind) {
		names = append(names, st.Also...)
	}
	for _, nm := range names {
		for _, n := range c.byFold[foldName(nm)] {
			if st == nil || stepNPC(st, n.Name, n.ServerID) {
				out = append(out, n)
			}
		}
	}
	return out
}

// reachable is "" for an NPC a player can walk to, else why not: a floor under it (or
// water round it) where it stands, the movement check's (broken: none); and a flood of
// the floor from it getting clear of it (pocket: a warning, as a counter, a room's
// doorway or a tree's ramps keep the flood in where a player walks). "" too where there
// is no ground to ask.
func (c *questCheck) reachable(n *entity.NPC) (why string, broken bool) {
	if c.g == nil || !c.g.Knows(n.World) {
		return "", false
	}
	if why, ok := c.reach[n.ServerID]; ok {
		return why, strings.HasPrefix(why, "no floor")
	}
	why = c.floorUnder(n.World, n.SpawnX, n.SpawnY, n.SpawnZ)
	if why == "" {
		why = c.floodFrom(n.World, n.SpawnX, n.SpawnY, n.SpawnZ)
	}
	c.reach[n.ServerID] = why
	return why, strings.HasPrefix(why, "no floor")
}

// floorUnder is "" where a body at (x, y, z) stands on a floor (an upward face no more
// than 3 under its feet or 2 over them: a GM's place a little off its floor) or is in
// water, else why not.
func (c *questCheck) floorUnder(world uint8, x, y, z float32) string {
	for _, sf := range c.g.Surfaces(world, x, z, y-3, y+2) {
		if sf.Up {
			return ""
		}
	}
	if !c.groundKnown(world, x, y, z, 20) {
		return "" // a hole in the baked ground (Snowfist's blue cave): nothing to say
	}
	if c.env != nil {
		if f, ok := c.env.Flags(world, x, y+1.2, z); ok && f&ground.EnvWater != 0 {
			return ""
		}
	}
	below, above := "none", "none"
	for _, sf := range c.g.Surfaces(world, x, z, y-400, y+400) {
		if !sf.Up {
			continue
		}
		if sf.Y < y && (below == "none" || sf.Y > mustF(below)) {
			below = fmt.Sprintf("%.1f", sf.Y)
		}
		if sf.Y > y && (above == "none" || sf.Y < mustF(above)) {
			above = fmt.Sprintf("%.1f", sf.Y)
		}
	}
	return fmt.Sprintf("no floor under (%.0f, %.1f, %.0f): the nearest below %s, above %s", x, y, z, below, above)
}

// groundKnown is whether the baked ground has any floor within r of (x, z) and 30 of
// y: where it has none (a cave the bake left out) a place cannot be judged.
func (c *questCheck) groundKnown(world uint8, x, y, z, r float32) bool {
	for dx := -r; dx <= r; dx += r / 4 {
		for dz := -r; dz <= r; dz += r / 4 {
			for _, sf := range c.g.Surfaces(world, x+dx, z+dz, y-30, y+30) {
				if sf.Up {
					return true
				}
			}
		}
	}
	return false
}

func mustF(s string) float32 {
	v, _ := strconv.ParseFloat(s, 32)
	return float32(v)
}

// qcFloodReach is how far a flood from an NPC must get for it to stand in the open
// world, qcFloodCells how many cells of floor are a hall or a town's streets (open
// enough); qcFloodStep a cell's side.
const (
	qcFloodReach = 60
	qcFloodCells = 1500
	qcFloodStep  = 2
)

// floodFrom floods the ground out from a place, as the petitions 2073 and 2098 fixes
// did, by the moves the server's movement check takes from a player (entity/
// movecheck.go): a cell is reached from its neighbour when a player standing on the
// neighbour's floor may walk to it (a climb no steeper than 1:1 and a step, a drop of
// any height to 40) and the way at the body's middle crosses no face (Blocked). ""
// when the flood gets qcFloodReach clear of the place or over qcFloodCells cells,
// else why not.
func (c *questCheck) floodFrom(world uint8, x, y, z float32) string {
	const climb = qcFloodStep*1.0 + 1.3 // mcRiseSlope a unit across and mcRiseStep
	const middle = 1.2                  // mcMiddle
	floorNear := func(fx, fz, at, up, down float32) (float32, bool) {
		best, found := float32(0), false
		for _, sf := range c.g.Surfaces(world, fx, fz, at-down, at+up) {
			if sf.Up && (!found || abs32(sf.Y-at) < abs32(best-at)) {
				best, found = sf.Y, true
			}
		}
		return best, found
	}
	h, ok := floorNear(x, z, y, 3, 12)
	if !ok {
		if !c.g.Knows(world) {
			return ""
		}
		return c.floorUnder(world, x, y, z)
	}
	type cell struct{ i, j int }
	ys := map[cell]float32{{0, 0}: h}
	q := []cell{{0, 0}}
	for len(q) > 0 {
		cl := q[0]
		q = q[1:]
		cy := ys[cl]
		cx, cz := x+float32(cl.i)*qcFloodStep, z+float32(cl.j)*qcFloodStep
		for _, d := range []cell{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nb := cell{cl.i + d.i, cl.j + d.j}
			if _, seen := ys[nb]; seen {
				continue
			}
			nx, nz := x+float32(nb.i)*qcFloodStep, z+float32(nb.j)*qcFloodStep
			// the player stands at nb and walks to cl: up to cl at most a climb,
			// down to it any drop to 40
			ny, ok := floorNear(nx, nz, cy, 40, climb)
			if !ok {
				continue
			}
			if _, hit := c.g.Blocked(world, nx, ny+middle, nz, cx, cy+middle, cz); hit {
				continue
			}
			if math.Hypot(float64(nx-x), float64(nz-z)) >= qcFloodReach || len(ys) >= qcFloodCells {
				return ""
			}
			ys[nb] = ny
			q = append(q, nb)
		}
	}
	return fmt.Sprintf("pocket: the flood from (%.0f, %.0f, %.0f) found %d cells of floor and no way out", x, h, z, len(ys))
}

// threatensAll is whether every character the quest is for is attacked on sight by n
// at the quest's level (the World's target face), with the standings the quest asks.
func (c *questCheck) threatensAll(qu *database.Quest, n *entity.NPC) (bool, int, int) {
	combos := c.combosFor(qu)
	hit := 0
	for _, cr := range combos {
		p := &entity.Player{ServerID: 1, CharServerID: 1, Class: cr[0], Race: cr[1], HumType: cr[2], Level: max(qu.MinLevel, 1), World: n.World}
		for _, fr := range qu.FactionReqs {
			if p.Factions == nil {
				p.Factions = map[string]int{}
			}
			p.Factions[fr.Faction] = fr.Min
		}
		if c.w.gw.TargetFace(p, n) == entity.FaceHostile {
			hit++
		}
	}
	return hit > 0 && hit == len(combos), hit, len(combos)
}

// checkAt checks the NPC a giver or a talk, show or give step is done at.
func (c *questCheck) checkAt(qu *database.Quest, st *database.QuestStep, name string, made map[string]string) {
	step := 0
	if st != nil {
		step = st.Step
	}
	if strings.TrimSpace(name) == "" {
		c.add(true, "npc", qu, step, "no NPC named")
		return
	}
	npcs := c.stepNPCs(st, name)
	var up []*entity.NPC
	for _, n := range npcs {
		if standing(n) {
			up = append(up, n)
		}
	}
	if len(up) == 0 {
		by := made[foldName(name)]
		if by == "" {
			by = c.anyTriggerSpawns(name)
			if by != "" {
				by = "trigger " + by + " (not fired by this quest)"
			}
		}
		switch {
		case by != "" && len(c.byFold[foldName(name)]) > 0:
			c.add(false, "npc", qu, step, "%q stands only as %s makes it", name, by)
		case len(npcs) > 0:
			c.add(true, "npc", qu, step, "%q stands only out of the world (a spawn-only original) and nothing of the quest makes one", name)
		case st != nil && st.NPCID != 0 && len(c.byFold[foldName(name)]) > 0:
			c.add(true, "npc", qu, step, "%q pinned to row %d, which is not one of the name", name, st.NPCID)
		default:
			c.add(true, "npc", qu, step, "no NPC %q (the world has %s)", name, orNone(c.near(name)))
		}
		return
	}
	// a twin a player may take for it: standing within 300 of one of the name
	var twins []string
	for _, f := range c.near(name) {
	twin:
		for _, t := range c.byFold[f] {
			for _, n := range up {
				if standing(t) && t.World == n.World && math.Hypot(float64(t.SpawnX-n.SpawnX), float64(t.SpawnZ-n.SpawnZ)) < 300 {
					twins = append(twins, f)
					break twin
				}
			}
		}
	}
	if len(twins) > 0 {
		c.add(false, "twin", qu, step, "%q stands; so do %s, a ` ' or a letter off, within 300", name, strings.Join(quoteAll(twins), ", "))
	}
	var sign, hostile, shut, floorless int
	for _, n := range up {
		if IsSign(n, c.signs) {
			sign++
		}
		if all, _, _ := c.threatensAll(qu, n); all {
			hostile++
		}
		if why, broken := c.reachable(n); broken {
			floorless++
		} else if why != "" {
			shut++
		}
	}
	if sign == len(up) {
		c.add(true, "npc", qu, step, "%q is a sign post: it answers with its words, never a quest", name)
	}
	if hostile == len(up) {
		_, hit, of := c.threatensAll(qu, up[0])
		c.add(true, "hostile", qu, step, "%q attacks on sight %d of the %d classes and races the quest is for, at level %d", name, hit, of, max(qu.MinLevel, 1))
	} else if hostile > 0 {
		c.add(false, "hostile", qu, step, "%d of %d %q attack the quest's characters on sight", hostile, len(up), name)
	}
	if floorless == len(up) {
		why, _ := c.reachable(up[0])
		c.add(true, "reach", qu, step, "%q: %s", name, why)
	} else if shut+floorless == len(up) {
		why, _ := c.reachable(up[0])
		c.add(false, "pocket", qu, step, "%q: %s", name, why)
	}
	if st != nil {
		g := c.giverWorld(qu)
		other := true
		for _, n := range up {
			if g < 0 || int(n.World) == g {
				other = false
			}
		}
		if other {
			c.add(false, "world", qu, step, "%q stands in world %d, the giver in world %d", name, up[0].World, g)
		}
	}
}

func orNone(v []string) string {
	if len(v) == 0 {
		return "no name like it"
	}
	return strings.Join(quoteAll(v), ", ")
}

func quoteAll(v []string) []string {
	out := make([]string, len(v))
	for i, s := range v {
		out[i] = strconv.Quote(s)
	}
	return out
}

// giverWorld is the world the quest's giver stands in (-1: none).
func (c *questCheck) giverWorld(qu *database.Quest) int {
	for _, n := range c.byFold[foldName(qu.Giver)] {
		if standing(n) {
			return int(n.World)
		}
	}
	return -1
}

// checkHunt checks a kill or loot step's creatures.
func (c *questCheck) checkHunt(qu *database.Quest, st *database.QuestStep, made map[string]string) {
	npcs := c.stepNPCs(st, st.Target)
	var up []*entity.NPC
	for _, n := range npcs {
		if standing(n) {
			up = append(up, n)
		}
	}
	names := append([]string{st.Target}, st.Also...)
	if len(up) == 0 {
		for _, nm := range names {
			if by := made[foldName(nm)]; by != "" {
				if len(c.byFold[foldName(nm)]) == 0 {
					c.add(true, "spawn", qu, st.Step, "%s makes %q, which has no row to copy", by, nm)
				}
				return
			}
		}
		for _, nm := range names {
			if by := c.anyTriggerSpawns(nm); by != "" {
				c.add(false, "creature", qu, st.Step, "%q comes only by trigger %s, which this quest does not fire", nm, by)
				return
			}
		}
		if len(npcs) > 0 {
			c.add(true, "creature", qu, st.Step, "%q stands only out of the world and nothing of the quest makes one", st.Target)
		} else {
			c.add(true, "creature", qu, st.Step, "no creature %s (the world has %s)", strings.Join(quoteAll(names), " or "), orNone(c.near(st.Target)))
		}
		return
	}
	lo := up[0].Level
	shut, floorless := 0, 0
	for _, n := range up {
		lo = min(lo, n.Level)
		if why, broken := c.reachable(n); broken {
			floorless++
		} else if why != "" {
			shut++
		}
	}
	if top := max(qu.MinLevel, 1) + 10; lo > top {
		c.add(false, "level", qu, st.Step, "the lowest of %d %q is level %d, the quest %d", len(up), st.Target, lo, qu.MinLevel)
	}
	if floorless == len(up) {
		why, _ := c.reachable(up[0])
		c.add(true, "reach", qu, st.Step, "every %q stands off its floor: %s", st.Target, why)
	} else if shut+floorless == len(up) {
		why, _ := c.reachable(up[0])
		c.add(false, "pocket", qu, st.Step, "every %q: %s", st.Target, why)
	}
	if st.Kind == database.StepKill && st.Count > 1 && len(up) == 1 {
		c.add(false, "spawns", qu, st.Step, "%d kills of %q wanted, one spawn", st.Count, st.Target)
	}
}

// itemSource is how the character on step st of qu can have item pid, "" none.
func (c *questCheck) itemSource(qu *database.Quest, st *database.QuestStep, pid int) string {
	before := c.requiredClosure(qu)
	var weak string
	for _, s := range c.sources[pid] {
		switch s.kind {
		case "reward", "give", "drop":
			if s.quest == qu.ID {
				if s.step < st.Step || s.kind == "drop" && s.step == st.Step {
					return fmt.Sprintf("%s at step %d", s.kind, s.step)
				}
				continue
			}
			if before[s.quest] {
				return fmt.Sprintf("%s of quest %d, required", s.kind, s.quest)
			}
			o := c.w.quest(s.quest)
			if o != nil && c.fitsAny(o, qu) && weak == "" {
				weak = fmt.Sprintf("%s of quest %d step %d", s.kind, s.quest, s.step)
			}
		default:
			return s.kind + " " + s.what
		}
	}
	return weak
}

// fitsAny is whether a character qu is for may take o too.
func (c *questCheck) fitsAny(o, qu *database.Quest) bool {
	for _, cr := range c.combosFor(qu) {
		if (len(o.Classes) == 0 || containsInt(o.Classes, cr[0])) && (len(o.Races) == 0 || questRaceTakes(o.Races, cr[1])) {
			return true
		}
	}
	return false
}

// requiredClosure is every quest qu asks finished, and theirs.
func (c *questCheck) requiredClosure(qu *database.Quest) map[int]bool {
	out := map[int]bool{}
	var walk func(q *database.Quest)
	walk = func(q *database.Quest) {
		for _, g := range q.Required() {
			for _, id := range g {
				if !out[id] {
					out[id] = true
					if o := c.w.quest(id); o != nil {
						walk(o)
					}
				}
			}
		}
	}
	walk(qu)
	return out
}

// checkItems checks a loot step's or a hand-in's items.
func (c *questCheck) checkItems(qu *database.Quest, st *database.QuestStep) {
	for _, it := range st.Items {
		need := max(it.Count, 1)
		if st.Kind == database.StepLoot {
			_, need, _ = st.LootItem()
		}
		p, err := c.items.GetItemPattern(it.PatternID)
		if err != nil || p == nil {
			c.add(true, "item", qu, st.Step, "item %d is no record", it.PatternID)
			continue
		}
		if p.Lore != 0 && need > 1 {
			c.add(true, "lore", qu, st.Step, "%d of %q (%d) wanted, a Lore item: one can be held", need, p.ItemName, p.PatternID)
		}
		if st.Kind == database.StepLoot {
			continue
		}
		src := c.itemSource(qu, st, it.PatternID)
		c.used = append(c.used, fmt.Sprintf("%d step %d item %d %q: %s", qu.ID, st.Step, it.PatternID, p.ItemName, src))
		if src == "" {
			c.add(true, "source", qu, st.Step, "%q (%d) can be had nowhere: no drop, reward, merchant, table or start", p.ItemName, p.PatternID)
		}
	}
	if st.Kind == database.StepLoot && len(st.Items) == 0 {
		c.add(true, "item", qu, st.Step, "a loot step that names no item")
	}
	if (st.Kind == database.StepGive || st.Kind == database.StepShow) && len(st.Items) == 0 && st.Tunar == 0 {
		c.add(false, "item", qu, st.Step, "a %s step that wants nothing", st.Kind)
	}
}

// checkReach checks a reach step's place.
func (c *questCheck) checkPlace(qu *database.Quest, st *database.QuestStep) {
	if st.X == 0 && st.Z == 0 {
		c.add(true, "place", qu, st.Step, "a reach step with no place")
		return
	}
	if st.Radius <= 0 {
		c.add(true, "place", qu, st.Step, "a reach step with no radius")
	}
	if c.g == nil || !c.g.Knows(uint8(st.World)) {
		return
	}
	// ground (or water) at the place or within its radius
	if !c.groundKnown(uint8(st.World), st.X, st.Y, st.Z, st.Radius) {
		return // a hole in the baked ground: not judged
	}
	for _, d := range [][2]float32{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		x, z := st.X+d[0]*st.Radius*0.6, st.Z+d[1]*st.Radius*0.6
		for _, sf := range c.g.Surfaces(uint8(st.World), x, z, st.Y-st.Radius, st.Y+st.Radius) {
			if sf.Up {
				if c.floodFrom(uint8(st.World), x, sf.Y, z) != "" {
					c.add(false, "pocket", qu, st.Step, "the place (%.0f, %.0f, %.0f): %s", st.X, st.Y, st.Z, c.floodFrom(uint8(st.World), x, sf.Y, z))
				}
				return
			}
		}
		if c.floorUnder(uint8(st.World), x, st.Y, z) == "" {
			return
		}
	}
	c.add(true, "reach", qu, st.Step, "the place (%.0f, %.0f, %.0f) in world %d, radius %.0f: %s", st.X, st.Y, st.Z, st.World, st.Radius,
		c.floorUnder(uint8(st.World), st.X, st.Y, st.Z))
}

// checkEvents checks a quest's or a step's events.
func (c *questCheck) checkEvents(qu *database.Quest, step int, evs []database.QuestEvent, raw string) {
	for _, pr := range database.EventProblems(raw, step > 0) {
		c.add(true, "event", qu, step, "%s", pr)
	}
	for _, e := range evs {
		switch e.Do {
		case database.EventSpawn:
			if len(c.byFold[foldName(e.NPC)]) == 0 {
				c.add(true, "event", qu, step, "spawn of %q: no NPC row of that name to copy", e.NPC)
			}
		case database.EventTrigger:
			tr, ok := c.trigs[foldName(e.Trigger)]
			switch {
			case !ok:
				c.add(true, "event", qu, step, "trigger %q: no such trigger", e.Trigger)
			case !tr.Enabled:
				c.add(true, "event", qu, step, "trigger %q is off", e.Trigger)
			default:
				for _, pr := range database.TriggerProblems(tr) {
					c.add(true, "event", qu, step, "trigger %q: %s", e.Trigger, pr)
				}
			}
		case database.EventGive:
			for _, it := range database.ParseItemCounts(e.Items) {
				if p, err := c.items.GetItemPattern(it.PatternID); err != nil || p == nil {
					c.add(true, "event", qu, step, "give of item %d: no record", it.PatternID)
				}
			}
		case database.EventAttack, database.EventSay:
			if e.NPC != "" && len(c.byFold[foldName(e.NPC)]) == 0 {
				c.add(false, "event", qu, step, "%s by %q: no NPC of that name", e.Do, e.NPC)
			}
		}
	}
}

// checkRewards checks a step's rewards.
func (c *questCheck) checkRewards(qu *database.Quest, st *database.QuestStep) {
	var items []database.ItemCount
	items = append(items, st.RewardItems...)
	spells := append([]int(nil), st.Spells...)
	for _, rw := range st.ByClass {
		items = append(items, rw.Items...)
		spells = append(spells, rw.Spells...)
	}
	for _, o := range st.Choices.Options {
		items = append(items, o.Reward.Items...)
		spells = append(spells, o.Reward.Spells...)
	}
	for _, it := range items {
		p, err := c.items.GetItemPattern(it.PatternID)
		if err != nil || p == nil {
			c.add(true, "reward", qu, st.Step, "reward item %d: no record", it.PatternID)
			continue
		}
		// a spell's scroll ("This allows you to scribe ..."), not a quest's parchment
		// with the scroll's art (Pardon, Star Charts: handed in, never scribed)
		if isScroll(p) && strings.Contains(strings.ToLower(p.ItemDesc), "scribe") {
			if _, ok := scrollSpell(c.names, p.ItemName, p.LevelReq); !ok {
				c.add(true, "reward", qu, st.Step, "reward scroll %q (%d) scribes no spell", p.ItemName, p.PatternID)
			}
		}
	}
	for _, id := range spells {
		if _, ok := c.spells[id]; !ok {
			c.add(true, "reward", qu, st.Step, "reward spell %d: no record", id)
		}
	}
	if len(st.ByClass) > 0 {
		for _, cr := range c.combosFor(qu) {
			if _, ok := st.ByClass[cr[0]]; !ok {
				c.add(false, "byclass", qu, st.Step, "a by-class reward without class %d's", cr[0])
				break
			}
		}
	}
}

// checkRequires checks the quests it asks finished first.
func (c *questCheck) checkRequires(qu *database.Quest) {
	for _, g := range qu.Required() {
		for _, id := range g {
			if o := c.w.quest(id); o == nil {
				c.add(len(g) == 1, "requires", qu, 0, "asks quest %d finished, which does not exist", id)
			} else if !o.Enabled {
				c.add(len(g) == 1, "requires", qu, 0, "asks quest %d %q finished, which is a draft", id, o.Name)
			}
		}
	}
	var none []string
	combos := c.combosFor(qu)
	if len(combos) == 0 {
		c.add(true, "mask", qu, 0, "no class and race a character may be can take it (classes %v, races %v)", qu.Classes, qu.Races)
		return
	}
	for _, cr := range combos {
		if !c.w.fits(qu, cr[0], cr[1], map[int]bool{}) && cr[1] != 10 {
			none = append(none, fmt.Sprintf("%d/%d", cr[0], cr[1]))
		}
	}
	if len(none) == len(combos) {
		c.add(true, "requires", qu, 0, "no class and race it is for can finish the quests it asks first (%v)", qu.RequireGroups)
	} else if len(none) > 0 {
		c.add(false, "requires", qu, 0, "class/race %s cannot finish the quests it asks first", strings.Join(none, " "))
	}
}

// checkMask checks a city class quest's class and races against where each race of its
// class starts: the city of the quests nearest that start.
func (c *questCheck) checkMask(qu *database.Quest, city, class string, cities map[string][3]float64, starts map[[2]int][3]float64) {
	cl := classByName(class)
	if cl < 0 {
		return
	}
	if len(qu.Classes) != 1 || qu.Classes[0] != cl {
		c.add(false, "mask", qu, 0, "a %s %s quest for classes %v", city, class, qu.Classes)
	}
	var want []int
	for k, s := range starts {
		if k[0] != cl {
			continue
		}
		best, bd := "", math.MaxFloat64
		for name, cc := range cities {
			if int(cc[2]) != int(s[2]) {
				continue
			}
			if d := math.Hypot(cc[0]-s[0], cc[1]-s[1]); d < bd {
				best, bd = name, d
			}
		}
		if best == city && bd < 2500 {
			want = append(want, k[1])
		}
	}
	sort.Ints(want)
	for _, r := range want {
		if len(qu.Races) > 0 && !questRaceTakes(qu.Races, r) {
			c.add(false, "mask", qu, 0, "race %d starts in %s as a %s, the quest is for races %v", r, city, class, qu.Races)
		}
	}
}

var qcClassNames = []string{"warrior", "ranger", "paladin", "shadowknight", "monk", "bard", "rogue", "druid", "shaman",
	"cleric", "magician", "necromancer", "enchanter", "wizard", "alchemist"}

func classByName(s string) int {
	s = strings.ReplaceAll(strings.ToLower(s), " ", "")
	s = strings.ReplaceAll(s, "-", "")
	for i, n := range qcClassNames {
		if n == s {
			return i
		}
	}
	if s == "shadowknights" || s == "sk" {
		return 3
	}
	return -1
}

// static checks every quest's data against the world.
func (c *questCheck) static() {
	raw := map[[2]int]string{}
	rows, err := c.w.db.Query(`SELECT questid, 0, events FROM quests UNION ALL SELECT questid, step, events FROM queststeps`)
	if err != nil {
		c.t.Fatal(err)
	}
	for rows.Next() {
		var q, s int
		var e string
		rows.Scan(&q, &s, &e)
		raw[[2]int{q, s}] = e
	}
	rows.Close()
	// the city class quests' cities: where their givers stand
	cities := map[string][3]float64{}
	nCity := map[string]int{}
	for _, qu := range c.quests {
		city, _, ok := cityClass(qu.Source)
		if !ok {
			continue
		}
		for _, n := range c.byFold[foldName(qu.Giver)] {
			if standing(n) {
				cc := cities[city]
				k := float64(nCity[city])
				cities[city] = [3]float64{(cc[0]*k + float64(n.SpawnX)) / (k + 1), (cc[1]*k + float64(n.SpawnZ)) / (k + 1), float64(n.World)}
				nCity[city]++
				break
			}
		}
	}
	starts := map[[2]int][3]float64{}
	if spots, err := c.w.db.StartSpots(); err == nil {
		for _, s := range spots {
			starts[[2]int{s.Class, s.Race}] = [3]float64{float64(s.X), float64(s.Z), float64(s.World)}
		}
	}
	masks := 0
	defer func() {
		c.t.Logf("%d city class quests' classes and races checked against %d cities' starts (%d starts)", masks, len(cities), len(starts))
	}()
	for _, qu := range c.quests {
		made, _ := c.spawnedBy(qu)
		if strings.TrimSpace(qu.Giver) == "" {
			c.add(false, "giver", qu, 0, "no giver: offered by nothing but an event or a GM")
		} else {
			c.checkAt(qu, nil, qu.Giver, made)
		}
		if len(qu.Steps) == 0 {
			c.add(true, "steps", qu, 0, "no steps")
		}
		c.checkEvents(qu, 0, qu.Events, raw[[2]int{qu.ID, 0}])
		c.checkRequires(qu)
		if city, class, ok := cityClass(qu.Source); ok && len(cities) > 0 {
			c.checkMask(qu, city, class, cities, starts)
			masks++
		}
		for i := range qu.Steps {
			st := &qu.Steps[i]
			c.steps++
			switch st.Kind {
			case database.StepTalk, database.StepShow, database.StepGive:
				c.checkAt(qu, st, st.Target, made)
			case database.StepKill, database.StepLoot:
				c.checkHunt(qu, st, made)
			case database.StepReach:
				c.checkPlace(qu, st)
			default:
				c.add(true, "steps", qu, st.Step, "a step of kind %q the engine does not do", st.Kind)
			}
			if st.Kind != database.StepTalk && st.Kind != database.StepKill && st.Kind != database.StepReach {
				c.checkItems(qu, st)
			}
			c.checkEvents(qu, st.Step, st.Events, raw[[2]int{qu.ID, st.Step}])
			c.checkRewards(qu, st)
		}
	}
}

// cityClass reads a city class quest's import key (EQOA-Data 1-20/<city>/<class>/L<n>).
func cityClass(source string) (city, class string, ok bool) {
	k := database.QuestImportKey(source)
	p := strings.Split(k, "/")
	if len(p) != 4 || p[0] != "1-20" || !strings.HasPrefix(p[3], "L") {
		return "", "", false
	}
	return p[1], p[2], true
}

// walk walks every quest checked through the engine, its hand-ins' items had from the
// sources found.
func (c *questCheck) walk() {
	w := c.w
	w.obtain = func(qu *database.Quest, st *database.QuestStep) string {
		for _, it := range st.Items {
			if c.itemSource(qu, st, it.PatternID) == "" {
				continue // the static check's finding: the walk does not make it up
			}
			pat, err := w.qe.items.GetItemPattern(it.PatternID)
			if err != nil || pat == nil {
				continue
			}
			have := 0
			for _, h := range w.qe.heldItems(w.p) {
				if h.Pattern.PatternID == it.PatternID {
					have += max(h.StackLeft, 1)
				}
			}
			if n := max(it.Count, 1) - have; n > 0 {
				if _, err := GiveItemNow(w.sess, w.gw, w.inv, *pat, n); err != nil {
					return err.Error()
				}
			}
		}
		return ""
	}
	failed := 0
	for _, qu := range c.quests {
		if why := w.walkTo(qu.ID); why != "" {
			failed++
			// a quest failed walking another it requires is that one's finding
			if !strings.Contains(why, fmt.Sprintf("walking %d ", qu.ID)) || strings.Contains(why, "not to be taken: under way") {
				continue
			}
			c.add(true, "walk", qu, 0, "%s", why)
		}
	}
	c.t.Logf("walked %d quests to (%d quests and %d steps walked, %d characters), %d not finished", len(c.quests), w.walked, w.steps,
		len(w.byCombo), failed)
}

// report says what was found, by kind, and fails on what cannot be done.
func (c *questCheck) report() {
	sort.SliceStable(c.findings, func(i, j int) bool {
		a, b := c.findings[i], c.findings[j]
		if a.broken != b.broken {
			return a.broken
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.quest != b.quest {
			return a.quest < b.quest
		}
		return a.step < b.step
	})
	type tally struct{ broken, warn int }
	kinds := map[string]*tally{}
	quests := map[int]bool{}
	for _, f := range c.findings {
		k := kinds[f.kind]
		if k == nil {
			k = &tally{}
			kinds[f.kind] = k
		}
		if f.broken {
			k.broken++
			quests[f.quest] = true
		} else {
			k.warn++
		}
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("%d quests of level %d and under, %d steps checked; %d quests with a step that cannot be done",
		len(c.quests), c.top, c.steps, len(quests)))
	var names []string
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		lines = append(lines, fmt.Sprintf("  %-9s broken %4d, warn %4d", k, kinds[k].broken, kinds[k].warn))
	}
	for _, f := range c.findings {
		sev := "warn  "
		if f.broken {
			sev = "BROKEN"
		}
		qu := c.w.quest(f.quest)
		lines = append(lines, fmt.Sprintf("%s %-8s %d %q step %d: %s", sev, f.kind, f.quest, qu.Name, f.step, f.text))
	}
	lines = append(lines, "", "Each hand-in's items, where they are had:")
	lines = append(lines, c.used...)
	if out := os.Getenv("EQOA_QUESTCHECK_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			c.t.Error(err)
		}
	}
	for i, l := range lines {
		if i <= len(names) {
			c.t.Log(l)
		}
	}
	for _, f := range c.findings {
		if f.broken {
			c.t.Errorf("%d step %d %s: %s", f.quest, f.step, f.kind, f.text)
		}
	}
}
