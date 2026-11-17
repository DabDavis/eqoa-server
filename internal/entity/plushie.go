package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// plushie.go — the Pegasus Plushie (the owner, 2026-11-17: "add pegasus
// plushie too"): a toy of Toymaker Tinsel's (tools/christmas_toys_20261117.py)
// whose Use brings out a tiny white winged horse that follows its owner, and
// whose next Use sends it home: one at a time, the item the one switch.
//
// The plushie is a Barrel of Monkeys' monkey in all but its body and number
// (monkeys.go): a Temporary NPC, never owned (no pet slot, no 0x007E), on no
// hate list, never a target (its channel made a monkey's, monkeyChannel: no
// life bar, no con), walked through by its owner (its own non-collide entry),
// keeping a place of its own round its owner (slotPlushie, nearer than the
// monkeys'), with every walk and run. It goes home when its owner dies, leaves
// the world, changes world, is moved far at once or goes into capture the
// flag's arena (monkeysMustGoLocked's reasons), and is not kept over a logout
// or a keep restart. Its body is the white winged horse (CHAR.ESF 0x2F8721C0,
// the sleigh's Starlight) at PlushieSize, a plush pet's size by a render; its
// name a cute one of plushieNames.

// PlushieBody and PlushieSize are the plushie's body and size: the winged
// horse Starlight pulls Santa's sleigh in (0.65 there), at 0.25 its withers
// about a man's knee (render: ~/.cache/toys1117/png/pegasus_*.png).
const (
	PlushieBody uint32  = 0x2F8721C0
	PlushieSize float32 = 0.25
)

// plushieRadius is how far off its owner the plushie keeps (a monkey's 3.0).
const plushieRadius float32 = 2.4

// plushieNoCollideID is the plushie's entry on its owner's non-collide list:
// after the five monkeys' (monkeyNoCollideID + 0 to 4).
const plushieNoCollideID = monkeyNoCollideID + MonkeysMax

// plushieNames are the names a plushie is given, one at random (each within
// the channel's 24 letters).
var plushieNames = []string{
	"Puff", "Starbit", "Cloudwing", "Moonbeam", "Sugarplum", "Snowdrop", "Twinkletoes", "Marshmallow", "Pip",
	"Comet", "Dewdrop", "Buttons", "Gumdrop", "Nimbus", "Flurry", "Sparkle", "Pudding", "Wisp", "Jingle", "Cocoa",
}

// PlushieUse is what a use of the plushie came to.
type PlushieUse int

const (
	PlushieNone PlushieUse = iota // no player in the world
	PlushieDead                   // its user is dead: nothing comes
	PlushieAway                   // its user is in capture the flag's arena: nothing comes
	PlushieCame                   // it came out
	PlushieHome                   // it was out: it went home
)

// PlushieResult is a use of the plushie: what it came to and the plushie's name.
type PlushieResult struct {
	Use  PlushieUse
	Name string
}

// UsePlushie is a use of the plushie by the session's player at now: the
// winged horse out beside it, or, out already, home again.
func (w *World) UsePlushie(sess *session.Session, now time.Time) PlushieResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return PlushieResult{}
	}
	if ps := w.plushies[p.ServerID]; ps != nil {
		name := ps.npc.Name
		w.plushieGoLocked(p, sess)
		return PlushieResult{Use: PlushieHome, Name: name}
	}
	switch {
	case p.IsDead() || p.respawnPending():
		return PlushieResult{Use: PlushieDead}
	case w.ctfKeepsOutLocked(p):
		return PlushieResult{Use: PlushieAway}
	}
	ps := w.plushieComesLocked(p, sess, now)
	return PlushieResult{Use: PlushieCame, Name: ps.npc.Name}
}

// PlushieOut is the session's player's plushie's name, "" for none out.
func (w *World) PlushieOut(sess *session.Session) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if ps := w.plushies[w.sessionPlayerLocked(sess).idOrZero()]; ps != nil {
		return ps.npc.Name
	}
	return ""
}

// plushieComesLocked puts the plushie beside p. Must be called with the lock
// held and none out.
func (w *World) plushieComesLocked(p *Player, sess *session.Session, now time.Time) *monkeyState {
	id := w.allocFreeIDLocked()
	n := &NPC{ServerID: id, Name: plushieNames[monkeyPick(len(plushieNames))], World: p.World, Level: p.Level,
		ModelID: int64(PlushieBody), Size: PlushieSize, NPCType: 0x80, Faction: FactionFriendly, Temporary: true,
		RespawnTime: petCorpseTime, CorpseDecay: petCorpseTime, MaxHP: 100, HP: 100, Race: "0"}
	n.X, n.Y, n.Z = w.followPlaceLocked(n, p, slotPlushie)
	n.Facing = headingTo(p.X-n.X, p.Z-n.Z)
	n.SpawnX, n.SpawnY, n.SpawnZ, n.SpawnFacing = n.X, n.Y, n.Z, n.Facing
	ps := &monkeyState{owner: p, npc: n, slot: MonkeysMax, last: now}
	n.monkey = ps
	w.putNPCLocked(n)
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	if w.plushies == nil {
		w.plushies = make(map[EntityID]*monkeyState)
	}
	w.plushies[p.ServerID] = ps
	w.plushieTellLocked(ps, sess)
	log.Printf("Plushie: %s's Pegasus Plushie comes out as %q", p.Name, n.Name)
	return ps
}

// plushieTellLocked puts the plushie on its owner's non-collide list once.
func (w *World) plushieTellLocked(ps *monkeyState, sess *session.Session) {
	if sess == nil || ps.noCollide {
		return
	}
	sendNoCollideAdd(sess, ps.npc.Name, plushieNoCollideID)
	ps.noCollide = true
}

// plushieGoLocked sends p's plushie home, off its non-collide list (sess, p's,
// or nil). Must be called with the lock held.
func (w *World) plushieGoLocked(p *Player, sess *session.Session) {
	ps := w.plushies[p.ServerID]
	if ps == nil {
		return
	}
	delete(w.plushies, p.ServerID)
	n := ps.npc
	if sess != nil && ps.noCollide {
		sendNoCollideRemove(sess, plushieNoCollideID)
	}
	if w.NPCs[n.ServerID] == n {
		w.removeNPCLocked(n)
	}
	n.Dead = true
	log.Printf("Plushie: %s's %q goes home", p.Name, n.Name)
}

// plushieLeavingLocked is the owner leaving the world: its plushie goes (not
// kept over a logout). Must be called with the lock held.
func (w *World) plushieLeavingLocked(p *Player) {
	if ps := w.plushies[p.ServerID]; ps != nil && ps.owner == p {
		w.plushieGoLocked(p, nil)
	}
}

// plushieHomeLine is what an owner is told when its plushie goes home by
// itself (a death, a world, a long move, the arena).
const plushieHomeLine = "%s flutters back into its plushie."

// plushieSystem moves every plushie with its owner, or sends it home for the
// monkeys' reasons (monkeysMustGoLocked). Every tick; must be called with the
// lock held.
func (w *World) plushieSystem(now time.Time) {
	for id, ps := range w.plushies {
		owner := ps.owner
		if w.Players[id] != owner {
			w.plushieGoLocked(owner, nil) // its owner logged out or in again
			delete(w.plushies, id)
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if w.monkeysMustGoLocked(owner, []*monkeyState{ps}) {
			name := ps.npc.Name
			w.plushieGoLocked(owner, sess)
			if sess != nil {
				w.sendChatToSession(sess, fmt.Sprintf(plushieHomeLine, name))
			}
			continue
		}
		dt := float32(now.Sub(ps.last).Seconds())
		ps.last = now
		dt = min(max(dt, 0), 0.5)
		w.followLocked(ps.npc, owner, slotPlushie, dt, now)
		ps.npc.Level = owner.Level
		w.plushieTellLocked(ps, sess)
	}
}
