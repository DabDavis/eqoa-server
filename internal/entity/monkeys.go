package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"time"

	"github.com/eqoa/server/internal/session"
)

// monkeys.go — the Barrel of Monkeys (the owner, 2026-11-16: "give moose a
// barrel of monkeys unique item. it spawns a monkey that follows him around,
// up to 5. they have no life bar and cannot be targeted. they do not take up
// a pet slot, but are still owned by that player only", then "the gorillas
// make small like monkeys" and "give them random monkey names").
//
// The barrel is an item record (handlers/monkeybarrel.go: by its name, bound
// to its one character by table itemowner; tools/barrel_of_monkeys_20261116.py
// makes it). Each Use brings one more monkey out (UseMonkeyBarrel), up to
// MonkeysMax; a Use with all five out sends them all back into the barrel, so
// the barrel is the one switch, as the Summoning Portal's second use is.
//
// A monkey is a follower as a familiar is (familiar.go), and less: a Temporary
// NPC with the NPC bit (npc flags 0x80), never owned (no pet slot, no 0x007E,
// not on w.pets: a pet, a charm, a familiar, a genie and the mercs go on as
// with no monkey by), on no hate list (isFamiliarIDLocked counts it), in the
// gorilla's body (karba'nai brute, CHAR.ESF 0x95EE6EC6: the disc has no small
// monkey) at MonkeySize, under a monkey's name (monkeyNames, none twice among
// one player's). It is never targeted: its channel's byte 26 is 0 (CLIENT's
// VIEntity +0x18 bit 0, IsSelectable; the target cycle and the pick skip it,
// vanilla's too, as a board's plate: boardplate.go) and byte 142 a pet's 3
// (its name in the pet's colour, which the target cycle and TargetCentre skip
// too), so no target, no life bar and no con; nothing fights it. Its owner
// walks through it (0x1003, its own entry on the non-collide list).
//
// Each keeps a place of its own round its owner (followslot.go's slotMonkey1
// to slotMonkey5, nearer than a pet's: monkeyRadius), walking and running
// with him through every zone he walks, idling when he stands. They go back
// into the barrel (all of them, told once) when he dies, leaves the world,
// goes to another world or is moved far at once (a teleport, a gate, a GM's
// move: farther than petLost), or goes into capture the flag's arena. Not kept
// over a logout or a keep restart: the barrel brings them again.

// MonkeysMax is how many monkeys one player may have out.
const MonkeysMax = 5

// MonkeyBody is a monkey's body: the karba'nai brute's gorilla (CHAR.ESF
// #73), MonkeySize its size, picked by a render beside Moose's hill giant
// (the owner's "the gorillas make small like monkeys", next to him knee to
// waist high): at 1.2 they stood to his thigh, too big (the owner,
// 2026-11-16: "make the monkeys smaller"); at 0.7 about a man's knee, his
// shin; at 0.45 they reached his ankle.
const (
	MonkeyBody uint32  = 0x95EE6EC6
	MonkeySize float32 = 0.7
)

// monkeyRadius is how far off its owner a monkey's place is (followRadius is
// a pet's 3.5); a monkey is small, so its five places sit nearer.
const monkeyRadius float32 = 3.0

// monkeyNoCollideID is the first monkey's entry on its owner's non-collide
// list, past the pet's, the mercs', the familiar's and the side pet's
// (petNoCollideID + 0 to 5); a monkey of slot i is this + i.
const monkeyNoCollideID uint32 = petNoCollideID + 0x10

// monkeyNames are the names a monkey is given, one at random not already
// worn by another of its owner's (each at most the channel's 24 letters).
var monkeyNames = []string{
	"Bubbles", "Coco", "Mojo", "Bongo", "Kiki", "Chuckles", "Banana", "Peanut", "Momo", "Abu",
	"Zippy", "Jojo", "Nanu", "Tiko", "Pippin", "Bobo", "Mango", "Cheeky", "Noodle", "Gizmo",
	"Biscuit", "Pickles", "Taffy", "Zuzu", "Bananas", "Squeaky", "Rascal", "Jinx", "Doodle", "Monty",
	"Ooki", "Fuzzy", "Scamp", "Tamarin", "Waffles", "Kong", "Bonbon", "Sprocket", "Mischief", "Dodger",
}

// monkeyPick picks one of n (tests make it fixed).
var monkeyPick = rand.IntN

// monkeySlots are the five monkeys' follow slots, by their slot index.
var monkeySlots = [MonkeysMax]followSlot{slotMonkey1, slotMonkey2, slotMonkey3, slotMonkey4, slotMonkey5}

// monkeyState is one monkey out of its owner's barrel.
type monkeyState struct {
	owner *Player
	npc   *NPC
	slot  int // 0 to MonkeysMax-1: its follow slot and non-collide entry
	last  time.Time
	// noCollide: its owner was sent its non-collide entry
	noCollide bool
	// nextThrow is its next try at a snowball (monkeythrow.go)
	nextThrow time.Time
}

// isMonkey reports a monkey (monkeys.go).
func (n *NPC) isMonkey() bool { return n != nil && n.monkey != nil }

// MonkeyUse is what a use of the barrel came to.
type MonkeyUse int

const (
	MonkeyNone  MonkeyUse = iota // no player in the world
	MonkeyDead                   // its user is dead: nothing comes
	MonkeyAway                   // its user is in capture the flag's arena: nothing comes
	MonkeyCame                   // one more came out
	MonkeysBack                  // all were out: every one went back in
)

// MonkeyResult is a use of the barrel: what it came to, the monkey that came
// (MonkeyCame) and how many are out after it.
type MonkeyResult struct {
	Use  MonkeyUse
	Name string
	Out  int
}

// UseMonkeyBarrel is a use of the barrel by the session's player at now (the
// file's doc): one more monkey beside it, or, with MonkeysMax out, all of them
// gone back.
func (w *World) UseMonkeyBarrel(sess *session.Session, now time.Time) MonkeyResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return MonkeyResult{}
	}
	out := w.monkeys[p.ServerID]
	switch {
	case p.IsDead() || p.respawnPending():
		return MonkeyResult{Use: MonkeyDead, Out: len(out)}
	case w.ctfKeepsOutLocked(p):
		return MonkeyResult{Use: MonkeyAway, Out: len(out)}
	case len(out) >= MonkeysMax:
		w.monkeysGoLocked(p, sess)
		return MonkeyResult{Use: MonkeysBack}
	}
	ms := w.monkeyComesLocked(p, sess, now)
	return MonkeyResult{Use: MonkeyCame, Name: ms.npc.Name, Out: len(w.monkeys[p.ServerID])}
}

// MonkeysOut is how many monkeys the session's player has out.
func (w *World) MonkeysOut(sess *session.Session) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.monkeys[w.sessionPlayerLocked(sess).idOrZero()])
}

// monkeyComesLocked puts one more monkey beside p, in the first free slot,
// under a name none of p's others wears. Must be called with the lock held
// and fewer than MonkeysMax out.
func (w *World) monkeyComesLocked(p *Player, sess *session.Session, now time.Time) *monkeyState {
	out := w.monkeys[p.ServerID]
	used := make(map[int]bool, len(out))
	worn := make(map[string]bool, len(out))
	for _, o := range out {
		used[o.slot] = true
		worn[o.npc.Name] = true
	}
	slot := 0
	for used[slot] {
		slot++
	}
	id := w.allocFreeIDLocked()
	n := &NPC{ServerID: id, Name: monkeyName(worn), World: p.World, Level: p.Level, ModelID: int64(MonkeyBody),
		Size: MonkeySize, NPCType: 0x80, Faction: FactionFriendly, Temporary: true, RespawnTime: petCorpseTime,
		CorpseDecay: petCorpseTime, MaxHP: 100, HP: 100, Race: "0"}
	n.X, n.Y, n.Z = w.followPlaceLocked(n, p, monkeySlots[slot])
	n.Facing = headingTo(p.X-n.X, p.Z-n.Z)
	n.SpawnX, n.SpawnY, n.SpawnZ, n.SpawnFacing = n.X, n.Y, n.Z, n.Facing
	ms := &monkeyState{owner: p, npc: n, slot: slot, last: now, nextThrow: monkeyNextThrow(now)}
	n.monkey = ms
	w.putNPCLocked(n)
	w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	if w.monkeys == nil {
		w.monkeys = make(map[EntityID][]*monkeyState)
	}
	w.monkeys[p.ServerID] = append(out, ms)
	w.monkeyTellLocked(ms, sess)
	log.Printf("Monkeys: %s's barrel lets out %q (slot %d, %d out)", p.Name, n.Name, slot+1, len(w.monkeys[p.ServerID]))
	return ms
}

// monkeyName is a name of monkeyNames that worn has not, else one numbered.
func monkeyName(worn map[string]bool) string {
	free := make([]string, 0, len(monkeyNames))
	for _, s := range monkeyNames {
		if !worn[s] {
			free = append(free, s)
		}
	}
	if len(free) > 0 {
		return free[monkeyPick(len(free))]
	}
	for i := 2; ; i++ {
		if s := fmt.Sprintf("Monkey %d", i); !worn[s] {
			return s
		}
	}
}

// monkeyTellLocked puts the monkey on its owner's non-collide list once.
func (w *World) monkeyTellLocked(ms *monkeyState, sess *session.Session) {
	if sess == nil || ms.noCollide {
		return
	}
	sendNoCollideAdd(sess, ms.npc.Name, monkeyNoCollideID+uint32(ms.slot))
	ms.noCollide = true
}

// monkeysGoLocked sends every monkey of p's back into the barrel at once, off
// its non-collide list (sess, p's, or nil). It reports how many went. Must
// be called with the lock held.
func (w *World) monkeysGoLocked(p *Player, sess *session.Session) int {
	out := w.monkeys[p.ServerID]
	delete(w.monkeys, p.ServerID)
	for _, ms := range out {
		n := ms.npc
		if sess != nil && ms.noCollide {
			sendNoCollideRemove(sess, monkeyNoCollideID+uint32(ms.slot))
		}
		if w.NPCs[n.ServerID] == n {
			w.removeNPCLocked(n)
		}
		n.Dead = true
	}
	if len(out) > 0 {
		log.Printf("Monkeys: %s's %d go back into the barrel", p.Name, len(out))
	}
	return len(out)
}

// monkeysLeavingLocked is the owner leaving the world: its monkeys go (not
// kept over a logout). Must be called with the lock held.
func (w *World) monkeysLeavingLocked(p *Player) {
	if out := w.monkeys[p.ServerID]; len(out) > 0 && out[0].owner == p {
		w.monkeysGoLocked(p, nil)
	}
}

// monkeysBackLine is what an owner is told when its monkeys go back by
// themselves (a death, a world, a long move, the arena).
const monkeysBackLine = "Your monkeys scamper back into their barrel."

// monkeySystem moves every monkey with its owner; an owner gone, dead, in
// another world, moved far at once or in capture the flag's arena sends them
// all back. Every tick; must be called with the lock held.
func (w *World) monkeySystem(now time.Time) {
	if len(w.monkeys) == 0 {
		return
	}
	for id, out := range w.monkeys {
		if len(out) == 0 {
			delete(w.monkeys, id)
			continue
		}
		owner := out[0].owner
		if w.Players[id] != owner {
			w.monkeysGoLocked(owner, nil) // its owner logged out or in again
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if w.monkeysMustGoLocked(owner, out) {
			w.monkeysGoLocked(owner, sess)
			if sess != nil {
				w.sendChatToSession(sess, monkeysBackLine)
			}
			continue
		}
		for _, ms := range out {
			n := ms.npc
			dt := float32(now.Sub(ms.last).Seconds())
			ms.last = now
			dt = min(max(dt, 0), 0.5)
			w.followLocked(n, owner, monkeySlots[ms.slot], dt, now)
			n.Level = owner.Level
			w.monkeyTellLocked(ms, sess)
		}
		w.monkeyThrowsLocked(owner, out, now) // now and then a snowball (monkeythrow.go)
	}
}

// monkeysMustGoLocked reports owner's monkeys to go back (monkeySystem's
// reasons). Must be called with the lock held.
func (w *World) monkeysMustGoLocked(owner *Player, out []*monkeyState) bool {
	if owner.IsDead() || owner.respawnPending() || w.ctfKeepsOutLocked(owner) {
		return true
	}
	for _, ms := range out {
		n := ms.npc
		if n.Dead || w.NPCs[n.ServerID] != n || n.World != owner.World || distXZ(n.X, n.Z, owner.X, owner.Z) > petLost {
			return true
		}
	}
	return false
}

// monkeyChannel is a monkey's channel made never a target (the file's doc):
// byte 26 not selectable, 142 a pet's name colour, 143 never a fight's.
func monkeyChannel(buf []byte) {
	buf[26] = 0
	buf[142] = 3
	buf[143] = 0
}

// monkeySlotAngle is the five monkeys' places round their owner: behind it,
// then out to each side and forward, 0.3 pi apart (2.7 apart at
// monkeyRadius, past followApart).
var monkeySlotAngle = [MonkeysMax]float32{
	math.Pi, 0.7 * math.Pi, -0.7 * math.Pi, 0.4 * math.Pi, -0.4 * math.Pi,
}
