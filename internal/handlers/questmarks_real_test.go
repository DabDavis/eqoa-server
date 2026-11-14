package handlers

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questmarks_real_test.go — the quest helper's marks against where each step really is (the
// owner, 2026-11-14: "also lets update quest helper to be more accurate").
//
// TestRealQuestMarks reads a real server's database (EQOA_REAL_DB; a copy), loads its NPCs as
// the game server does, and for every step of every enabled quest puts a player where the step
// before it was done (the giver's spawn for the first) and asks the marks code where the step's
// marks go (stepSpotsLocked, and the way mark wayToLocked adds for a step in another world). Each
// is held to the step's real targets: a talk, show or give step's NPCs the engine matches
// (stepNPC: name, a pinned spawn) standing in the world, else the originals its spawns come again
// by; a kill or loot step's creatures (its Target and Also); a reach step's place. Per step:
//
//   - off: a mark more than markOffBy from every real target of its world (a spawn-only original
//     where one stands, a namesake in another world, a pinned row gone);
//   - stray: a hunt's mark on a spawn with fewer of the step's creatures within huntCampLink than
//     min(huntCampFew, the world's biggest camp of them) while a bigger camp stands in its world;
//   - no way: the step's targets are all in another world than the player's and no mark in the
//     player's world says how to get there;
//   - none: no mark though the step has a real target;
//   - no target: the step has nothing real to mark (the data's, listed).
//
// The same is measured for the marks as they were before 2026-11-14 (oldStepSpots: the nearest
// two spawns of any of the step's names, spawn-only originals and strays alike, no way mark), so
// the report says before and after. EQOA_QUESTMARKS_OUT names a file for every finding. It fails
// on an off mark, a stray or a step with no mark; a missing way is listed (no travel NPC leads
// out of Lavastorm or the Plane of Sky).
func TestRealQuestMarks(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	w := newQuestWalker(t, src)
	defer w.db.Close()
	gw := w.gw
	npcStore := database.NewNPCStore(w.db)
	gw.LoadNPCs(npcStore)
	if _, _, err := gw.LoadSpawnOnly(w.db); err != nil {
		t.Fatal(err)
	}
	q := w.qe
	q.mu.Lock()
	defer q.mu.Unlock()
	spots := q.markSpotsLocked(time.Now())

	type real struct {
		world   uint8
		x, z    float32
		orig    bool // a spawn-only original
		spawned bool
	}
	byFold := map[string][]*entity.NPC{}
	for _, n := range gw.NPCs {
		byFold[foldName(n.Name)] = append(byFold[foldName(n.Name)], n)
	}
	all := map[string]bool{}
	for k := range byFold {
		all[k] = true
	}
	orig := map[entity.EntityID]bool{}
	for _, ss := range gw.NPCSpotsNamed(all) {
		for _, s := range ss {
			orig[s.ID] = s.SpawnOnly
		}
	}
	realOf := func(st *database.QuestStep) []real {
		var out []real
		if st.Kind == database.StepReach {
			if st.X != 0 || st.Z != 0 {
				out = append(out, real{world: uint8(st.World), x: st.X, z: st.Z})
			}
			return out
		}
		if !database.AtNPC(st.Kind) && !database.Hunts(st.Kind) {
			return nil
		}
		names := []string{st.Target}
		if database.Hunts(st.Kind) {
			names = append(names, st.Also...)
		}
		var standing, origs []real
		for _, nm := range names {
			for _, n := range byFold[foldName(nm)] {
				if n.Temporary || n.Owner != 0 || !stepNPC(st, n.Name, n.ServerID) {
					continue
				}
				r := real{world: n.World, x: n.SpawnX, z: n.SpawnZ, orig: orig[n.ServerID]}
				if r.orig {
					origs = append(origs, r)
				} else {
					standing = append(standing, r)
				}
			}
		}
		if database.Hunts(st.Kind) {
			return append(standing, origs...) // an original's spot brings the step's spawns again
		}
		if len(standing) > 0 {
			return standing
		}
		return origs
	}
	nearestReal := func(rs []real, wld uint8, x, z float32) (real, float32, bool) {
		best, bd, ok := real{}, float32(math.MaxFloat32), false
		for _, r := range rs {
			if r.world != wld {
				continue
			}
			if d := float32(math.Hypot(float64(r.x-x), float64(r.z-z))); d < bd {
				best, bd, ok = r, d, true
			}
		}
		return best, bd, ok
	}
	// the old marks: the nearest two spawns of the step's names (of its first, only a pinned one)
	oldStepSpots := func(st *database.QuestStep, world uint8, x, z float32) []entity.NPCSpot {
		var out []entity.NPCSpot
		if st.Kind == database.StepReach {
			if (st.X != 0 || st.Z != 0) && st.World >= 0 && st.World < 256 {
				out = append(out, entity.NPCSpot{World: uint8(st.World), X: st.X, Z: st.Z})
			}
			return out
		}
		var names []string
		switch {
		case database.AtNPC(st.Kind):
			names = []string{st.Target}
		case database.Hunts(st.Kind):
			names = st.Targets()
		default:
			return nil
		}
		for i, name := range names {
			for _, s := range spots[foldName(name)] {
				if i > 0 || st.NPCID == 0 || int(s.ID) == st.NPCID {
					out = append(out, s)
				}
			}
		}
		nearestFirst(out, world, x, z)
		if len(out) > maxMarkSpots {
			out = out[:maxMarkSpots]
		}
		return out
	}
	type tally struct{ steps, off, stray, noWay, none, noTarget int }
	var before, after tally
	var lines []string
	quests := append([]*database.Quest(nil), q.cat.all...)
	sort.Slice(quests, func(i, j int) bool { return quests[i].ID < quests[j].ID })
	judge := func(tl *tally, label string, qu *database.Quest, st *database.QuestStep, rs []real, marks []entity.NPCSpot, way bool,
		world uint8, px, pz float32) {
		tl.steps++
		where := fmt.Sprintf("%s quest %d %q step %d (%s %q)", label, qu.ID, qu.Name, st.Step, st.Kind, st.Target)
		if len(rs) == 0 {
			tl.noTarget++
			lines = append(lines, where+": no target")
			return
		}
		if len(marks) == 0 {
			tl.none++
			lines = append(lines, where+": no mark")
			return
		}
		inWorld := false
		for _, r := range rs {
			if r.world == world {
				inWorld = true
			}
		}
		if !inWorld && !way {
			tl.noWay++
			lines = append(lines, fmt.Sprintf("%s: in world %d, the player in %d at (%.0f, %.0f): no way mark", where, rs[0].world, world, px, pz))
		}
		var off []string
		stray := ""
		for _, m := range marks {
			_, d, ok := nearestReal(rs, m.World, m.X, m.Z)
			if !ok || d > markOffBy {
				off = append(off, fmt.Sprintf("w%d (%.0f, %.0f) %.0f off", m.World, m.X, m.Z, d))
				continue
			}
			if database.Hunts(st.Kind) && st.NPCID == 0 {
				// the camp the mark is in (the step's creatures linked within
				// huntCampLink of one another) against the biggest of its world
				var mine []entity.NPCSpot
				for i, r := range rs {
					if r.world == m.World {
						mine = append(mine, entity.NPCSpot{ID: entity.EntityID(i + 1), World: r.world, X: r.x, Z: r.z})
					}
				}
				here, big := 0, 0
				for _, c := range campsOf(mine) {
					big = max(big, c.n)
				}
				for _, c := range campsOfWith(mine, m) {
					here = c
				}
				if here < min(huntCampFew, big) {
					stray = fmt.Sprintf("w%d (%.0f, %.0f): %d of them there, %d at the biggest camp", m.World, m.X, m.Z, here, big)
				}
			}
		}
		if len(off) > 0 {
			tl.off++
			lines = append(lines, where+": off: "+strings.Join(off, "; "))
		}
		if stray != "" {
			tl.stray++
			lines = append(lines, where+": stray: "+stray)
		}
	}
	for _, qu := range quests {
		if !qu.Enabled || len(qu.Steps) == 0 {
			continue
		}
		// the player starts at the giver
		var world uint8
		var px, pz float32
		placed := false
		for _, n := range byFold[foldName(qu.Giver)] {
			if !n.Temporary && n.Owner == 0 && !orig[n.ServerID] {
				world, px, pz, placed = n.World, n.SpawnX, n.SpawnZ, true
				break
			}
		}
		steps := append([]database.QuestStep(nil), qu.Steps...)
		sort.Slice(steps, func(i, j int) bool { return steps[i].Step < steps[j].Step })
		for i := range steps {
			st := &steps[i]
			rs := realOf(st)
			if !placed && len(rs) > 0 {
				world, px, pz, placed = rs[0].world, rs[0].x, rs[0].z, true
			}
			if database.AtNPC(st.Kind) || database.Hunts(st.Kind) || st.Kind == database.StepReach {
				judge(&before, "before", qu, st, rs, oldStepSpots(st, world, px, pz), false, world, px, pz)
				_, marks := q.stepSpotsLocked(spots, st, 0, world, px, pz)
				way := false
				if len(marks) > 0 && marks[0].World != world {
					_, way = q.wayToLocked(spots, world, marks[0].World, px, pz)
				}
				judge(&after, "after", qu, st, rs, marks, way, world, px, pz)
			}
			// the player goes where the step is done: its nearest real target
			if r, _, ok := nearestReal(rs, world, px, pz); ok {
				px, pz = r.x, r.z
			} else if len(rs) > 0 {
				world, px, pz = rs[0].world, rs[0].x, rs[0].z
			}
		}
	}
	sum := func(l string, tl tally) string {
		return fmt.Sprintf("%s: %d steps: %d with a mark more than %.0f off every real target, %d on a stray where a camp stands, %d in another world with no way mark, %d with no mark, %d with no target",
			l, tl.steps, tl.off, markOffBy, tl.stray, tl.noWay, tl.none, tl.noTarget)
	}
	t.Log(sum("before", before))
	t.Log(sum("after", after))
	if out := os.Getenv("EQOA_QUESTMARKS_OUT"); out != "" {
		body := sum("before", before) + "\n" + sum("after", after) + "\n\n" + strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a step in another world with no way mark is one no travel NPC leads to from where the player
	// stands (Lavastorm, the Plane of Sky: their ways out are a spell or a bind): listed, not failed
	if after.off > 0 || after.stray > 0 || after.none > 0 {
		t.Errorf("after: %d off, %d stray, %d with no mark (EQOA_QUESTMARKS_OUT lists them)", after.off, after.stray, after.none)
	}
}

// markOffBy is how far a mark may be from the step's nearest real target.
const markOffBy float32 = 50

// campsOfWith is the size of the camp of spots (campsOf's linking) that holds
// a spot within 1 of m.
func campsOfWith(spots []entity.NPCSpot, m entity.NPCSpot) []int {
	var out []int
	for _, c := range campMembers(spots) {
		for _, s := range c {
			if math.Hypot(float64(s.X-m.X), float64(s.Z-m.Z)) < 1 {
				out = append(out, len(c))
				break
			}
		}
	}
	return out
}

// campMembers is spots linked as campsOf links them, each camp's members.
func campMembers(spots []entity.NPCSpot) [][]entity.NPCSpot {
	parent := make([]int, len(spots))
	for i := range parent {
		parent[i] = i
	}
	root := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	for i := range spots {
		for j := 0; j < i; j++ {
			if spots[i].World == spots[j].World && math.Hypot(float64(spots[i].X-spots[j].X), float64(spots[i].Z-spots[j].Z)) <= huntCampLink {
				parent[root(i)] = root(j)
			}
		}
	}
	by := map[int][]entity.NPCSpot{}
	for i := range spots {
		by[root(i)] = append(by[root(i)], spots[i])
	}
	var out [][]entity.NPCSpot
	for _, m := range by {
		out = append(out, m)
	}
	return out
}
