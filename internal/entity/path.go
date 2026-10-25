package entity

import (
	"fmt"
	"math"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Paths a GM gives creatures (the GM panel's path editor, internal/gmweb;
// table npcpaths): "we also need a pathing editor in the gm panel". A path is
// a list of waypoints in one world, walked
//   - in a loop (the last point back to the first),
//   - back and forth (to the last point and back the same way), or
//   - once (to the last point, where it then stands);
// at Sandstorm's walking pace or the chase's run, standing at each point for
// the path's pause or the point's own. It is walked with the wander's and
// the chase's own step (npcmove.go): on the ground where the creature
// spawned on it (groundnpc.go), else along the waypoints' heights; facing
// along the leg; its velocity and walk or run on its channel.
//
// A creature with a path does not wander (roam.go), and walks it whether or
// not a player is near (paths are a GM's few, and the panel shows them
// moving). A fight takes it off its path (chase.go moves it, and brings it
// back to where the fight began); a pet's owner too (a charm); then it takes
// its path up again at the end of the leg nearest it. A root holds it, a
// snare halves its pace. Dead, it starts its path again when it comes back.
// Its path taken away, it walks back to its spawn point and is as it was.
//
// The path is keyed by the creature's id, its npcs row, so it outlives its
// deaths, an edit of its row (UpsertNPC) and a restart (LoadNPCPaths).

// PathMode is how a path is walked.
type PathMode uint8

const (
	PathLoop PathMode = iota
	PathPingPong
	PathOnce
)

func (m PathMode) String() string {
	switch m {
	case PathPingPong:
		return database.PathModePingPong
	case PathOnce:
		return database.PathModeOnce
	}
	return database.PathModeLoop
}

// PathPoint is a waypoint: a place in the path's world, and how long a
// creature stands there once it arrives (Pause < 0: the path's own).
type PathPoint struct {
	X, Y, Z float32
	Pause   time.Duration
}

// NPCPath is one creature's path.
type NPCPath struct {
	World  uint8
	Mode   PathMode
	Pause  time.Duration
	Run    bool
	Points []PathPoint
}

// PathFromRecord is the path a row of npcpaths describes, held to its
// checks (database.NPCPathRecord.Check).
func PathFromRecord(rec database.NPCPathRecord) (*NPCPath, error) {
	if err := rec.Check(); err != nil {
		return nil, err
	}
	mode, _ := database.PathMode(rec.Mode)
	speed, _ := database.PathSpeed(rec.Speed)
	p := &NPCPath{
		World: uint8(rec.World),
		Pause: secondsDuration(rec.Pause),
		Run:   speed == database.PathSpeedRun,
	}
	switch mode {
	case database.PathModePingPong:
		p.Mode = PathPingPong
	case database.PathModeOnce:
		p.Mode = PathOnce
	}
	for _, pt := range rec.Points {
		q := PathPoint{X: float32(pt.X), Y: float32(pt.Y), Z: float32(pt.Z), Pause: -1}
		if pt.Pause != nil {
			q.Pause = secondsDuration(*pt.Pause)
		}
		p.Points = append(p.Points, q)
	}
	return p, nil
}

func secondsDuration(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// pauseAt is how long a creature stands at point i.
func (p *NPCPath) pauseAt(i int) time.Duration {
	if q := p.Points[i].Pause; q >= 0 {
		return q
	}
	return p.Pause
}

// speed is the pace it is walked at.
func (p *NPCPath) speed() float32 {
	if p.Run {
		return NPCRunSpeed
	}
	return NPCWalkSpeed
}

// after is the point a creature walks to once it reached point i walking
// dir (+1 on, -1 back), and whether it stands there for good instead (the
// end of a path walked once, or a path of one point).
func (p *NPCPath) after(i, dir int) (next, newDir int, hold bool) {
	n := len(p.Points)
	if n <= 1 {
		return 0, 1, true
	}
	switch p.Mode {
	case PathOnce:
		if i >= n-1 {
			return n - 1, 1, true
		}
		return i + 1, 1, false
	case PathPingPong:
		if dir == 0 {
			dir = 1
		}
		j := i + dir
		if j < 0 || j >= n {
			dir = -dir
			j = i + dir
		}
		return j, dir, false
	}
	return (i + 1) % n, 1, false
}

// resume is where a creature off its path takes it up again, at (x, z): the
// point ending the leg nearest it, in the way the path is walked, so one
// that left it half way along a leg walks on to that leg's end rather than
// back to its start. On a path walked back and forth a leg counts both
// ways, the way it was going first.
func (p *NPCPath) resume(x, z float32, dir int) (next, newDir int) {
	n := len(p.Points)
	if n == 1 {
		return 0, 1
	}
	best := float32(math.MaxFloat32)
	next, newDir = 0, 1
	consider := func(a, b, d int) {
		if dist := distToLeg(x, z, p.Points[a], p.Points[b]); dist < best {
			best, next, newDir = dist, b, d
		}
	}
	forward := func() {
		for i := 0; i+1 < n; i++ {
			consider(i, i+1, 1)
		}
	}
	back := func() {
		for i := n - 1; i > 0; i-- {
			consider(i, i-1, -1)
		}
	}
	switch p.Mode {
	case PathLoop:
		for i := 0; i < n; i++ {
			consider(i, (i+1)%n, 1)
		}
	case PathOnce:
		forward()
	case PathPingPong:
		if dir < 0 {
			back()
			forward()
		} else {
			forward()
			back()
		}
	}
	return next, newDir
}

// distToLeg is how far (x, z) is from the leg a→b on the ground plane.
func distToLeg(x, z float32, a, b PathPoint) float32 {
	dx, dz := b.X-a.X, b.Z-a.Z
	l2 := dx*dx + dz*dz
	t := float32(0)
	if l2 > 0 {
		t = ((x-a.X)*dx + (z-a.Z)*dz) / l2
		t = max(0, min(1, t))
	}
	return distXZ(x, z, a.X+t*dx, a.Z+t*dz)
}

// pathState is a creature on its path: the point it walks to and which way,
// a pause, whether it stands at its end, where its leg began (the height off
// the ground), and whether something took it off its path.
type pathState struct {
	started bool
	broken  bool // a fight, a charm or another world: take it up again where nearest
	held    bool // at the end of a path walked once, or of a one-point path
	next    int
	dir     int
	until   time.Time // paused at a point until then
	last    time.Time // its last step
	fromX   float32
	fromY   float32
	fromZ   float32
}

// stepDT is the time since its last step (a stalled tick does not jump it),
// the step's clock set to now.
func (st *pathState) stepDT(now time.Time) float32 {
	dt := float32(0)
	if !st.last.IsZero() {
		dt = float32(now.Sub(st.last).Seconds())
	}
	st.last = now
	return max(0, min(dt, 0.5))
}

// legFrom notes where a leg begins.
func (st *pathState) legFrom(npc *NPC) {
	st.fromX, st.fromY, st.fromZ = npc.X, npc.Y, npc.Z
}

// pathTable is the paths by creature, and the creatures walking back to
// their spawn point after their path was taken away.
type pathTable struct {
	byNPC map[EntityID]*NPCPath
	home  map[EntityID]bool
}

// pathedLocked reports a creature that walks a path, or back from one: it
// does not wander. Must be called with the lock held.
func (w *World) pathedLocked(id EntityID) bool {
	return w.paths.byNPC[id] != nil || w.paths.home[id]
}

// LoadNPCPaths gives the creatures the paths the database holds, at the
// start: the number given.
func (w *World) LoadNPCPaths(recs []database.NPCPathRecord) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, rec := range recs {
		p, err := PathFromRecord(rec)
		if err != nil {
			continue
		}
		w.setPathLocked(EntityID(rec.NPCID), p)
		n++
	}
	return n
}

// SetNPCPath gives a creature its path, at once: it walks to its nearest
// leg (a path walked once, to its first point) and on. A creature of
// another world than the path's is refused; one not in the world takes it
// when it comes.
func (w *World) SetNPCPath(id EntityID, p *NPCPath) error {
	if p == nil || len(p.Points) == 0 {
		return fmt.Errorf("a path needs a point at least")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if npc := w.NPCs[id]; npc != nil {
		if npc.World != p.World && npc.Owner == 0 {
			return fmt.Errorf("%s is in world %d, the path in world %d", npc.Name, npc.World, p.World)
		}
		if npc.Temporary {
			return fmt.Errorf("%s is a GM's spawn: it has no row to keep a path", npc.Name)
		}
	}
	w.setPathLocked(id, p)
	return nil
}

func (w *World) setPathLocked(id EntityID, p *NPCPath) {
	if w.paths.byNPC == nil {
		w.paths.byNPC = map[EntityID]*NPCPath{}
	}
	w.paths.byNPC[id] = p
	delete(w.paths.home, id)
	if npc := w.NPCs[id]; npc != nil {
		npc.path = pathState{}
		npc.roam.walking = false
	}
}

// ClearNPCPath takes a creature's path away: it walks back to its spawn
// point and is as before (it wanders, or stands facing as spawned). It
// reports whether it had one.
func (w *World) ClearNPCPath(id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.paths.byNPC[id] == nil {
		return false
	}
	delete(w.paths.byNPC, id)
	if npc := w.NPCs[id]; npc != nil && !npc.Dead {
		npc.path = pathState{}
		npc.path.legFrom(npc)
		if distXZ(npc.X, npc.Z, npc.SpawnX, npc.SpawnZ) > 0.5 {
			if w.paths.home == nil {
				w.paths.home = map[EntityID]bool{}
			}
			w.paths.home[id] = true
		} else {
			npc.stopMoving()
			npc.Facing = npc.SpawnFacing
		}
	}
	return true
}

// NPCPathOf is a creature's path, a copy.
func (w *World) NPCPathOf(id EntityID) (NPCPath, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.paths.byNPC[id]
	if p == nil {
		return NPCPath{}, false
	}
	c := *p
	c.Points = append([]PathPoint(nil), p.Points...)
	return c, true
}

// PathStatus is a creature on its path as the world has it now (the GM
// panel's live view): where it is, the point it walks to (from 0) and what
// it does.
type PathStatus struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Z      float32 `json:"z"`
	Facing float32 `json:"facing"`
	World  uint8   `json:"world"`
	Next   int     `json:"next"`
	// State: walking, paused, holding (at the end of a path walked once),
	// fighting, rooted, dead, away (its path is in another world), home
	// (walking back to its spawn point, its path taken away), starting
	// (it has not taken its first step), or none (no path).
	State string `json:"state"`
}

// PathStatusOf is a creature's PathStatus, false when it is not in the
// world.
func (w *World) PathStatusOf(id EntityID) (PathStatus, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc := w.NPCs[id]
	if npc == nil {
		return PathStatus{}, false
	}
	s := PathStatus{X: npc.X, Y: npc.Y, Z: npc.Z, Facing: npc.Facing, World: npc.World, Next: npc.path.next}
	p := w.paths.byNPC[id]
	st := &npc.path
	switch {
	case npc.Dead:
		s.State = "dead"
	case w.paths.home[id]:
		s.State = "home"
	case p == nil:
		s.State = "none"
	case npc.CurrentEnemy != 0 || npc.chase.on || npc.chase.returning || npc.DummyTarget != 0 || npc.Owner != 0:
		s.State = "fighting"
	case npc.World != p.World:
		s.State = "away"
	case !st.started:
		s.State = "starting"
	case st.held:
		s.State = "holding"
	case npc.rooted(time.Now()):
		s.State = "rooted"
	case time.Now().Before(st.until):
		s.State = "paused"
	default:
		s.State = "walking"
	}
	return s, true
}

// GroundFloor is the ground's floor at (x, z) for a place at height y in a
// world (groundnpc.go: the highest terrain no more than a step above y),
// false where the world has no ground there.
func (w *World) GroundFloor(world uint8, x, y, z float32) (float32, bool) {
	w.mu.RLock()
	g := w.ground
	w.mu.RUnlock()
	if g == nil {
		return y, false
	}
	return g.Floor(world, x, y, z)
}

// NPCOnGround reports whether a creature walks on the ground (it spawned on
// the terrain, groundnpc.go), so a path's heights are the floor's; false
// for one off it or with no ground known.
func (w *World) NPCOnGround(id EntityID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil || w.ground == nil {
		return false
	}
	if !npc.groundKnown {
		x, y, z := npc.X, npc.Y, npc.Z
		npc.X, npc.Y, npc.Z = npc.SpawnX, npc.SpawnY, npc.SpawnZ
		w.groundNPCLocked(npc)
		npc.X, npc.Y, npc.Z = x, y, z
	}
	return npc.onGround
}

// pathSystem walks every creature with a path, and those walking back from
// one (the file's doc). Every tick; must be called with the lock held.
func (w *World) pathSystem(now time.Time) {
	for id, p := range w.paths.byNPC {
		if npc := w.NPCs[id]; npc != nil {
			w.pathStepLocked(npc, p, now)
		}
	}
	for id := range w.paths.home {
		npc := w.NPCs[id]
		if npc == nil || w.pathHomeStepLocked(npc, now) {
			delete(w.paths.home, id)
		}
	}
}

// offPath reports a creature something else moves now: a fight (its chase
// and its way back), a dummy's fight, a charm.
func (n *NPC) offPath() bool {
	return n.CurrentEnemy != 0 || n.chase.on || n.chase.returning || n.DummyTarget != 0 || n.Owner != 0
}

// pathStepLocked is one tick of a creature on its path. Must be called with
// the lock held.
func (w *World) pathStepLocked(npc *NPC, p *NPCPath, now time.Time) {
	st := &npc.path
	dt := st.stepDT(now)
	switch {
	case npc.Dead:
		// it starts its path again when it comes back
		*st = pathState{}
		return
	case npc.offPath():
		// theirs to move (chase.go, dummy.go, pet.go): standing until its
		// chase has it, as a wanderer does (roam.go)
		if !npc.chase.on && !npc.chase.returning && npc.Owner == 0 && npc.moving() {
			npc.stopMoving()
		}
		st.broken = st.started
		return
	case npc.World != p.World:
		if npc.moving() {
			npc.stopMoving()
		}
		st.broken = st.started
		return
	case npc.rooted(now):
		if npc.moving() {
			npc.stopMoving()
		}
		return
	}
	switch {
	case !st.started:
		st.started = true
		if p.Mode == PathOnce {
			st.next, st.dir = 0, 1
		} else {
			st.next, st.dir = p.resume(npc.X, npc.Z, 1)
		}
		st.legFrom(npc)
	case st.broken:
		st.broken, st.held, st.until = false, false, time.Time{}
		st.next, st.dir = p.resume(npc.X, npc.Z, st.dir)
		st.legFrom(npc)
	}
	if st.next >= len(p.Points) {
		st.next = len(p.Points) - 1
	}
	if st.held || now.Before(st.until) {
		if npc.moving() {
			npc.stopMoving()
		}
		return
	}
	pt := p.Points[st.next]
	speed := p.speed()
	if npc.snared(now) {
		speed /= 2
	}
	arrived := w.stepNPCToward(npc, pt.X, pt.Z, speed, dt)
	w.pathHeightLocked(npc, pt.X, pt.Y, pt.Z)
	if !arrived || dt <= 0 {
		return
	}
	if d := p.pauseAt(st.next); d > 0 {
		st.until = now.Add(d)
	}
	st.legFrom(npc)
	st.next, st.dir, st.held = p.after(st.next, st.dir)
}

// pathHomeStepLocked is one tick of a creature walking back to its spawn
// point after its path was taken away; true once it is home (or dead, and
// so coming back there). Must be called with the lock held.
func (w *World) pathHomeStepLocked(npc *NPC, now time.Time) (done bool) {
	st := &npc.path
	dt := st.stepDT(now)
	switch {
	case npc.Dead:
		return true
	case npc.offPath():
		if !npc.chase.on && !npc.chase.returning && npc.Owner == 0 && npc.moving() {
			npc.stopMoving()
		}
		st.legFrom(npc)
		return false
	case npc.rooted(now):
		if npc.moving() {
			npc.stopMoving()
		}
		return false
	}
	speed := NPCWalkSpeed
	if npc.snared(now) {
		speed /= 2
	}
	arrived := w.stepNPCToward(npc, npc.SpawnX, npc.SpawnZ, speed, dt)
	w.pathHeightLocked(npc, npc.SpawnX, npc.SpawnY, npc.SpawnZ)
	if !arrived || dt <= 0 {
		return false
	}
	// home: as it was spawned
	npc.Facing = npc.SpawnFacing
	npc.path = pathState{}
	npc.roam = roamState{}
	return true
}

// pathHeightLocked is the height of a creature that does not stand on the
// ground (a bridge's, a roof's: groundnpc.go) walking its leg toward (x, y,
// z): the leg's own slope from where it began. One on the ground keeps the
// floor stepNPCToward gave it. Must be called with the lock held.
func (w *World) pathHeightLocked(npc *NPC, x, y, z float32) {
	if w.ground != nil && npc.onGround {
		return
	}
	st := &npc.path
	total := distXZ(st.fromX, st.fromZ, x, z)
	if total <= 0.01 {
		npc.Y = y
		return
	}
	t := 1 - distXZ(npc.X, npc.Z, x, z)/total
	npc.Y = st.fromY + (y-st.fromY)*max(0, min(1, t))
}
