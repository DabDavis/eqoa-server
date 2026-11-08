package entity

import (
	"container/heap"
	"log"
	"math"
	"time"
)

// chaseroute.go — a fight's way round what stands between a creature and its
// enemy (the user, 2026-11-08: "the xrags when they are aggroed will run through
// the ledge instead of using the ramp pathing"). A chase ran straight at its
// enemy and stood on whatever floor was under it at each step (chaseLocked,
// groundNPCLocked): off a ledge in one step, or up into a cliff's face and
// through the rock to the floor above.
//
// Now a creature on the ground (spawnOnGroundLocked; not one swimming) looks
// ahead of each step (routeLook) along a leg as a chase runs one (movement.go's
// legRoute: a floor every routeSample, no steeper up or down than a wander
// leg's slope, legDrop in legSample, and nothing placed across it at its knee:
// a house's wall, a fence, a trunk; the terrain's own steep faces are its
// slope's), and while that way is clear runs straight, as before; a straight
// step that would still drop it past legDrop (a gap the look's points stepped
// over, or the water below a ledge) is not taken, and the way is searched.
// Where the way is not clear, it searches the ground for one (planRouteLocked:
// A* over a grid of routeCell, its way weighed routeWeight against what is
// left, each step one such leg, checked when the search takes it, within
// routeSpan of it, routeNodes points at most) to its enemy on the enemy's
// floor, and runs that way at its run, through each of its points
// (followRouteLocked: never across a corner), its points pulled straight where
// a leg between them is clear, until its end (straight from there), or until
// its enemy stands routeKeep off where it was found to (looked at again). A
// step on it that would drop past legDrop or rise past routeClimb (a gap no
// point of the search fell in) keeps the height it had. One whose enemy stands
// over or under it (another floor within its reach across the ground) searches
// at once: it cannot fight from there.
//
// A search that finds nothing leaves the chase straight, as it was, for
// routeAgain, twice that after another, to eight times: an enemy on a roof, an
// island, a bridge or a dock (floors the ground leaves out) is still run at.
// The searches of one tick share routeTickNodes points; one put off or cut
// short by that holds the creature where it stands for the tick, turned to its
// enemy, rather than over the edge, and searches at the next. The way home
// after a fight is found the same way (homeSystem). On the real ground, the
// Elephant Graveyard's Xrags chasing between floors (190 chases) found a way
// in 183, reached their enemy in all of them and never fell or climbed (65 went
// over a ledge straight), a search 2.8 ms on average and 12 ms at most; 281
// ordinary chases across Tunaria searched 6 times (chaseroute_real_test.go).

// The route search's figures: how far ahead of a running creature its way is
// looked at, and how far apart its floor is asked along a leg; the search's
// grid, and its way's weight against what is left (more than 1: fewer points
// tried for a way a little longer); how far from the creature it may go; a
// height past which two stand on different floors; how near its enemy on that
// floor a point ends the search, and from how near a clear leg to it does; how
// far its enemy may go from a route's end before the route is dropped; the
// floors one cell may hold apart; the most a step on a route rises; the points
// one search and one tick's searches may try; how far ahead a point is pulled
// to; and how long a search that found nothing first leaves the chase
// straight.
const (
	routeLook      float32 = 6
	routeSample    float32 = 1
	routeCell      float32 = 3
	routeWeight    float32 = 1.5
	routeSpan      float32 = 90
	routeLevel     float32 = 4
	routeFinish    float32 = 12
	routeKeep      float32 = 6
	routeFloorStep float32 = 3
	routeClimb     float32 = 2.5
	routeNodes             = 2000
	routeTickNodes         = 2500
	routePull              = 16
	routeAgain             = 2 * time.Second
)

// chaseRoutes turns the routes on; a test's control turns them off.
var chaseRoutes = true

// routePoint is a point of a route, on its floor.
type routePoint struct{ x, y, z float32 }

// chaseRoute is a creature's way round, while it has one (chaseState.route).
type chaseRoute struct {
	pts                 []routePoint // what is left of it, the next point first
	goalX, goalY, goalZ float32      // where it was found to
	failedUntil         time.Time    // a search that found nothing: straight until then
	fails               int          // searches in a row that found nothing (each waits twice the last, to routeAgain×8)
	edge                bool         // a straight step stopped at an edge: search, whatever the look ahead says
}

// routesFor reports a creature whose chase may go round: one on the ground, not
// swimming, in a world with its ground. Must be called with the lock held.
func (w *World) routesFor(npc *NPC) bool {
	return chaseRoutes && w.ground != nil && !npc.swimming && w.spawnOnGroundLocked(npc)
}

// routeLegLocked is the leg from (fx, fy, fz) to (x, z) as the creature would
// run it: its floor at (x, z), and whether it is one (legRoute). Must be
// called with the lock held.
func (w *World) routeLegLocked(npc *NPC, fx, fy, fz, x, z float32) (float32, bool) {
	w.routeHeights = w.routeHeights[:0]
	if !w.legFromLocked(npc, fx, fy, fz, x, z, &w.routeHeights, legRoute) || len(w.routeHeights) == 0 {
		return fy, false
	}
	return w.routeHeights[len(w.routeHeights)-1], true
}

// routeLeft is how many points this tick's searches have left.
func (w *World) routeLeft() int {
	if w.routeTick != w.TickCount {
		w.routeTick, w.routeSpent = w.TickCount, 0
	}
	return routeTickNodes - w.routeSpent
}

// routeWayLocked is how a creature on its way to (tx, ty, tz), d off it across
// the ground and moving step this tick, goes: routing, along its route round
// what stands between (followRouteLocked); hold, that it stands this tick (the
// straight way is not one it can run, and its search is put off to the next);
// neither, the straight way, as before (clear, or nothing found). stacked is
// its enemy over or under it: no straight way is looked at. Must be called
// with the lock held.
func (w *World) routeWayLocked(npc *NPC, r *chaseRoute, tx, ty, tz, d, step float32, stacked bool, now time.Time) (routing, hold bool) {
	if !w.routesFor(npc) {
		r.pts = nil
		return false, false
	}
	if len(r.pts) > 0 {
		if distXZ(r.goalX, r.goalZ, tx, tz) <= routeKeep && absf(r.goalY-ty) <= routeLevel {
			return true, false
		}
		r.pts = nil // its enemy left the route's end: looked at again
	}
	if !stacked && !r.edge {
		if d <= 0 {
			return false, false
		}
		ahead := min(d, max(routeLook, 3*step))
		if _, ok := w.routeLegLocked(npc, npc.X, npc.Y, npc.Z, npc.X+(tx-npc.X)/d*ahead, npc.Z+(tz-npc.Z)/d*ahead); ok {
			return false, false
		}
	}
	r.edge = false
	if now.Before(r.failedUntil) {
		return false, false // found nothing a moment ago: straight, as it was
	}
	limit := min(routeNodes, w.routeLeft())
	if limit <= 0 {
		return false, true // the tick's searches are spent: the next tick's
	}
	began := time.Now()
	pts, tried, ok := w.planRouteLocked(npc, tx, ty, tz, limit)
	took := time.Since(began)
	w.routeSpent += tried
	if !ok && tried >= limit && limit < routeNodes {
		return false, true // cut short by the tick's share, not a way it lacks: the next tick's
	}
	if !ok || len(pts) == 0 {
		r.failedUntil = now.Add(routeAgain << min(r.fails, 3))
		r.fails++
		log.Printf("Chase: %s finds no way to (%.0f, %.0f, %.0f) from (%.0f, %.0f, %.0f) (%d points tried, %v): straight", npc.Name,
			tx, ty, tz, npc.X, npc.Y, npc.Z, tried, took.Round(10*time.Microsecond))
		return false, false
	}
	r.pts, r.goalX, r.goalY, r.goalZ, r.fails = pts, tx, ty, tz, 0
	log.Printf("Chase: %s goes round to (%.0f, %.0f, %.0f) from (%.0f, %.0f, %.0f): %d points (%d tried, %v)", npc.Name,
		tx, ty, tz, npc.X, npc.Y, npc.Z, len(pts), tried, took.Round(10*time.Microsecond))
	return true, false
}

// followRouteLocked moves npc dt seconds at speed along its route: through its
// points as it comes to them, never across a corner (each leg is one the search
// found clear, from the very place the creature stood), on the floor as it
// goes, its facing and velocity its leg's. Must be called with the lock held.
func (w *World) followRouteLocked(npc *NPC, r *chaseRoute, speed, dt float32) {
	left := speed * dt
	oldX, oldZ := npc.X, npc.Z
	var ux, uz float32
	for left > 0 && len(r.pts) > 0 {
		p := r.pts[0]
		dx, dz := p.x-npc.X, p.z-npc.Z
		dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		if dist > 0 {
			ux, uz = dx/dist, dz/dist
		}
		y0, swim0 := npc.Y, npc.swimming
		if dist <= left {
			npc.X, npc.Z = p.x, p.z
			left -= dist
			r.pts = r.pts[1:]
		} else {
			npc.X += ux * left
			npc.Z += uz * left
			left = 0
		}
		w.groundNPCLocked(npc)
		if npc.Y < y0-legDrop || npc.Y > y0+routeClimb {
			npc.Y, npc.swimming = y0, swim0 // a gap between the search's points: across it at its height
		}
	}
	if ux != 0 || uz != 0 {
		npc.Facing = headingTo(ux, uz)
	}
	npc.VelX, npc.VelY, npc.VelZ = ux*speed, 0, uz*speed
	npc.Loco = LocoWalk
	if speed >= NPCRunFrom {
		npc.Loco = LocoRun
	}
	if w.SpatialIndex != nil {
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
	}
}

// routeNode is a point of a search: its place, its floor once its leg from its
// parent was found clear (checked), and its way.
type routeNode struct {
	x, y, z float32
	g       float32 // the way to it
	parent  int32
	ix, iz  int32
	checked bool
	done    bool
}

// routeKey is a cell and the floor in it.
type routeKey struct{ ix, iz, floor int32 }

// routeItem is a point waiting, by its way plus what is left at best.
type routeItem struct {
	f float32
	i int32
}

type routeQueue []routeItem

func (q routeQueue) Len() int           { return len(q) }
func (q routeQueue) Less(a, b int) bool { return q[a].f < q[b].f }
func (q routeQueue) Swap(a, b int)      { q[a], q[b] = q[b], q[a] }
func (q *routeQueue) Push(x any)        { *q = append(*q, x.(routeItem)) }
func (q *routeQueue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// routeDirs are a cell's eight neighbours and their lengths in cells.
var routeDirs = [8]struct {
	dx, dz int32
	n      float32
}{{1, 0, 1}, {-1, 0, 1}, {0, 1, 1}, {0, -1, 1}, {1, 1, math.Sqrt2}, {1, -1, math.Sqrt2}, {-1, 1, math.Sqrt2}, {-1, -1, math.Sqrt2}}

// planRouteLocked searches the ground for the creature's way to (tx, ty, tz)
// on that floor, limit points at most: the route's points after the creature's
// own place, how many points the search took (each one leg checked), and
// whether it found one. A point's leg from its parent is checked when the
// point is taken, not when it is first seen (most points seen are never
// taken: a check is the search's cost). Must be called with the lock held.
func (w *World) planRouteLocked(npc *NPC, tx, ty, tz float32, limit int) ([]routePoint, int, bool) {
	sy := npc.Y
	if f, ok := w.ground.Floor(npc.World, npc.X, npc.Y, npc.Z); ok {
		sy = f
	}
	floorOf := func(y float32) int32 { return int32(math.Floor(float64(y / routeFloorStep))) }
	nodes := []routeNode{{x: npc.X, y: sy, z: npc.Z, parent: -1, checked: true}}
	taken := map[routeKey]bool{}
	q := &routeQueue{{f: distXZ(npc.X, npc.Z, tx, tz), i: 0}}
	near := max(npcReach(npc)-0.5, 1.5)
	span := int32(routeSpan / routeCell)
	tried := 0
	for q.Len() > 0 && tried < limit {
		i := heap.Pop(q).(routeItem).i
		n := &nodes[i]
		if n.done {
			continue
		}
		n.done = true
		if !n.checked {
			if taken[routeKey{n.ix, n.iz, floorOf(n.y)}] {
				continue // reached on its parent's floor already, a shorter way
			}
			tried++
			p := nodes[n.parent]
			y, ok := w.routeLegLocked(npc, p.x, p.y, p.z, n.x, n.z)
			if !ok {
				continue
			}
			n.y, n.checked = y, true
		}
		k := routeKey{n.ix, n.iz, floorOf(n.y)}
		if taken[k] {
			continue // this cell's floor was reached a shorter way
		}
		taken[k] = true
		// the end: near its enemy on its floor, or a clear leg to it from near
		if absf(n.y-ty) <= routeLevel {
			dxz := distXZ(n.x, n.z, tx, tz)
			if dxz <= near {
				return w.pullRouteLocked(npc, nodes, i), tried, true
			}
			if dxz <= routeFinish {
				tried++
				if y, ok := w.routeLegLocked(npc, n.x, n.y, n.z, tx, tz); ok && absf(y-ty) <= routeLevel {
					return w.pullRouteLocked(npc, nodes, i), tried, true
				}
			}
		}
		// each neighbour from here, whatever another point offered it: a leg
		// that is clear from one may not be from another
		x0, z0, fl := nodes[0].x, nodes[0].z, floorOf(n.y)
		for _, d := range routeDirs {
			ix, iz := n.ix+d.dx, n.iz+d.dz
			if ix > span || ix < -span || iz > span || iz < -span || taken[routeKey{ix, iz, fl}] {
				continue
			}
			g := n.g + d.n*routeCell
			x, z := x0+float32(ix)*routeCell, z0+float32(iz)*routeCell
			nodes = append(nodes, routeNode{x: x, y: n.y, z: z, g: g, parent: i, ix: ix, iz: iz})
			heap.Push(q, routeItem{f: g + routeWeight*distXZ(x, z, tx, tz), i: int32(len(nodes) - 1)})
			n = &nodes[i] // nodes may have moved
		}
	}
	return nil, tried, false
}

// pullRouteLocked is the way to nodes[end], its points after the start, each
// pulled as far ahead as a clear leg from the last reaches (routePull cells at
// most, halving): fewer turns than the grid's. Must be called with the lock
// held.
func (w *World) pullRouteLocked(npc *NPC, nodes []routeNode, end int32) []routePoint {
	var path []routePoint
	for i := end; i >= 0; i = nodes[i].parent {
		path = append(path, routePoint{nodes[i].x, nodes[i].y, nodes[i].z})
	}
	for a, b := 0, len(path)-1; a < b; a, b = a+1, b-1 {
		path[a], path[b] = path[b], path[a]
	}
	var out []routePoint
	for at := 0; at < len(path)-1; {
		next := at + 1
		for jump := routePull; jump >= 2; jump /= 2 {
			j := min(at+jump, len(path)-1)
			if j <= at+1 {
				continue
			}
			if y, ok := w.routeLegLocked(npc, path[at].x, path[at].y, path[at].z, path[j].x, path[j].z); ok && absf(y-path[j].y) <= 1 {
				next = j
				break
			}
		}
		out = append(out, path[next])
		at = next
	}
	return out
}

// absf is |v|.
func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
