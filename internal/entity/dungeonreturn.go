package entity

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/session"
)

// dungeonreturn.go — the Scroll of Returning (Heirloom petition 3199, Wolfmoon: "Heroic
// Dungeon scrolls: scrolls that return you to town, and back to your exact position in
// the dungeon before consumed on use"; handlers/dungeonreturn.go is the item's Use).
//
// Read inside a heroic dungeon's instance (one the player may stand in), the scroll
// keeps the player's exact place (the instance, x, y, z and facing) and sends it home
// as Return Home does (its bind point, else the login spawn); the scroll is kept.
// Read again outside the dungeons within DungeonReturnWindow, while that instance is
// still open (the same instance: its id and the time it opened) and the player may
// still stand in it (its owner, its group, or added) and is not locked out of it
// (dungeonlockout.go: an instance opened before the lockout began lets it back in), it
// sends the player back to the kept place and the scroll is spent: two reads, out and
// back. A read in the dungeon again keeps the new place instead. A way back gone (the
// window passed, the instance closed, the player no longer of its party) is forgotten
// and the scroll kept, for another trip; one barred for now (a lockout) is kept.
//
// The places live in memory, as the instances do; a restart that keeps the sessions
// keeps them (keepdungeon.go), a cold start closes every instance and so every way back.

// DungeonReturnWindow is how long after the read out a read back takes the player
// back: the owner's "within N minutes" at 30, the instances' own CloseAfter.
const DungeonReturnWindow = 30 * time.Minute

// dungeonReturn is a character's way back: the instance (its id and when it opened, so
// a later instance under a reused id is never taken for it) and the place in it.
type dungeonReturn struct {
	inst    uint32
	opened  time.Time
	world   uint8
	x, y, z float32
	facing  float32
	at      time.Time // the read out
}

// DungeonReturnResult is what a read of the scroll did.
type DungeonReturnResult int

const (
	// ReturnRefused: nothing done (the line says why); the scroll kept
	ReturnRefused DungeonReturnResult = iota
	// ReturnOut: the place kept, the player sent home; the scroll kept
	ReturnOut
	// ReturnBack: the player sent back to its place; the scroll spent
	ReturnBack
)

// The scroll's lines (ours).
const (
	returnLineOut     = "The Scroll of Returning marks your place in %s and carries you home. Read it again within %d minutes to return."
	returnLineBack    = "The Scroll of Returning carries you back into %s and crumbles to dust."
	returnLineNone    = "The Scroll of Returning stirs only within a heroic dungeon, or to take you back to the place it marked there."
	returnLineFaded   = "The way back has faded: %s. The scroll keeps its power for another journey."
	returnLineNotHere = "You may not read the Scroll of Returning in that instance."
	returnLineNow     = "You cannot read the Scroll of Returning now."
)

// DungeonReturnRead is a read of the scroll by the session's player at now: inside an
// instance it keeps the place and sends the player home; outside, its way back taken.
// It says what it did and a line for the player.
func (w *World) DungeonReturnRead(sess *session.Session, now time.Time) (DungeonReturnResult, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	switch {
	case p == nil || p.IsDead() || p.respawnPending():
		return ReturnRefused, returnLineNow
	case !w.dungeons.on:
		return ReturnRefused, returnLineNone
	}
	mark := w.dungeons.returns[p.CharServerID]
	var res DungeonReturnResult
	var line string
	if w.inDungeonsLocked(p) {
		res, line = w.dungeonReturnOutLocked(sess, p, now, &mark, nil)
	} else {
		res, line = w.dungeonReturnBackLocked(sess, p, now, &mark, nil)
	}
	if mark.at.IsZero() {
		delete(w.dungeons.returns, p.CharServerID)
	} else {
		if w.dungeons.returns == nil {
			w.dungeons.returns = make(map[uint32]dungeonReturn)
		}
		w.dungeons.returns[p.CharServerID] = mark
	}
	return res, line
}

// dungeonReturnOutLocked keeps p's place in its instance and sends it home.
func (w *World) dungeonReturnOutLocked(sess *session.Session, p *Player, now time.Time, mark *dungeonReturn,
	home *GateSpot) (DungeonReturnResult, string) {
	in := w.dungeons.insts[p.Instance]
	if in == nil || !w.dungeonAllowsLocked(in, p) || w.dungeons.defs[in.def].Hidden() {
		// a hidden dungeon is entered at its own NPC alone (DungeonDef.Via): no way back kept to it
		return ReturnRefused, returnLineNotHere
	}
	*mark = dungeonReturn{inst: in.id, opened: in.opened, world: p.World, x: p.X, y: p.Y, z: p.Z, facing: p.Facing, at: now}
	w.dungeons.movedAt[p.ServerID] = now
	name := w.dungeons.defs[in.def].Name
	travel := GateTravel{Sess: sess, P: p, Home: p.ServerID, Spell: "the Scroll of Returning"}
	if home != nil {
		travel.Home, travel.To = 0, *home
	}
	w.gates.due = append(w.gates.due, travel)
	log.Printf("Dungeons: %s reads the Scroll of Returning in %s (instance %d) at (%.1f, %.1f, %.1f): home", p.Name,
		name, in.id, p.X, p.Y, p.Z)
	return ReturnOut, fmt.Sprintf(returnLineOut, name, int(DungeonReturnWindow/time.Minute))
}

// dungeonReturnBackLocked sends p back to its kept place, when its way back holds.
func (w *World) dungeonReturnBackLocked(sess *session.Session, p *Player, now time.Time, mark *dungeonReturn,
	spend func(GateSpot) bool) (DungeonReturnResult, string) {
	r := *mark
	if r.at.IsZero() {
		return ReturnRefused, returnLineNone
	}
	in := w.dungeons.insts[r.inst]
	faded := ""
	switch {
	case now.Sub(r.at) > DungeonReturnWindow:
		faded = fmt.Sprintf("more than %d minutes have passed", int(DungeonReturnWindow/time.Minute))
	case in == nil || !in.opened.Equal(r.opened):
		faded = "that instance is closed"
	case !w.dungeonAllowsLocked(in, p):
		faded = fmt.Sprintf("you are no longer of %s's party", in.ownerName)
	}
	if faded != "" {
		*mark = dungeonReturn{}
		log.Printf("Dungeons: %s's Scroll of Returning finds no way back (%s)", p.Name, faded)
		return ReturnRefused, fmt.Sprintf(returnLineFaded, faded)
	}
	if why, locked := w.dungeonLockedOutLocked(p.CharServerID, in.def, in, now); locked {
		return ReturnRefused, why
	}
	d := &w.dungeons.defs[in.def]
	to := GateSpot{World: d.World, X: r.x, Y: r.y, Z: r.z, Place: d.Name}
	if spend != nil && !spend(to) {
		return ReturnRefused, returnLineNow
	}
	*mark = dungeonReturn{}
	w.dungeons.stays[p.CharServerID] = dungeonStay{From: GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z,
		Place: "where you were"}, FromFacing: p.Facing}
	p.Instance = in.id
	in.lastIn = now
	in.markStood(p.CharServerID)
	w.dungeons.movedAt[p.ServerID] = now
	delete(w.dungeons.badAt, p.ServerID)
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to,
		Spell: "the Scroll of Returning", Facing: r.facing, HasFacing: true})
	log.Printf("Dungeons: %s reads the Scroll of Returning: back into %s (instance %d, %s's) at (%.1f, %.1f, %.1f)",
		p.Name, d.Name, in.id, in.ownerName, r.x, r.y, r.z)
	return ReturnBack, fmt.Sprintf(returnLineBack, d.Name)
}

// DungeonReturnOf is a character's way back, for tests and the log: its instance and
// place, false for none.
func (w *World) DungeonReturnOf(char uint32) (inst uint32, x, y, z float32, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	r, ok := w.dungeons.returns[char]
	return r.inst, r.x, r.y, r.z, ok
}

// KeptDungeonReturn is a way back a keep restart carries.
type KeptDungeonReturn struct {
	Char   uint32     `json:"char"`
	Inst   uint32     `json:"inst"`
	Opened time.Time  `json:"opened"`
	Place  [4]float32 `json:"place"` // x, y, z, facing
	At     time.Time  `json:"at"`
}

// dungeonReturnsPruneLocked forgets the ways back past their window.
func (w *World) dungeonReturnsPruneLocked(now time.Time) {
	for c, r := range w.dungeons.returns {
		if now.Sub(r.at) > DungeonReturnWindow {
			delete(w.dungeons.returns, c)
		}
	}
}

// keptReturnsLocked is every way back still in its window at now, by character.
func (w *World) keptReturnsLocked(now time.Time) []KeptDungeonReturn {
	var out []KeptDungeonReturn
	for c, r := range w.dungeons.returns {
		if now.Sub(r.at) > DungeonReturnWindow || w.dungeons.insts[r.inst] == nil {
			continue
		}
		out = append(out, KeptDungeonReturn{Char: c, Inst: r.inst, Opened: r.opened, Place: [4]float32{r.x, r.y, r.z, r.facing}, At: r.at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Char < out[j].Char })
	return out
}

// restoreReturnsLocked puts kept ways back in again (their instances are checked at the read).
func (w *World) restoreReturnsLocked(rs []KeptDungeonReturn) {
	for _, r := range rs {
		if w.dungeons.returns == nil {
			w.dungeons.returns = map[uint32]dungeonReturn{}
		}
		w.dungeons.returns[r.Char] = dungeonReturn{inst: r.Inst, opened: r.Opened, x: r.Place[0], y: r.Place[1],
			z: r.Place[2], facing: r.Place[3], at: r.At}
	}
}
