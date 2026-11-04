package entity

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The NPCs each per-tick NPC system has work for, kept as sets, so that a
// tick visits those alone and not every NPC in the world. At 60,793 NPCs
// three full scans a tick were the whole of an idle tick: ~12 ms of every
// 100 ms on a desktop core and ~25 ms on the live box's, under the world
// lock (claude-notes/server-tick-profile-20261103.md). The sets:
//
//   - ai: an NPC aiSystem has a fight, or a fight's end, to see to: an
//     enemy, a dummy's player, a round, a chase, a template's fight or a
//     guard's mark (aiQuiet is the opposite);
//   - corpse: the dead (corpseSystem);
//   - regen: an NPC npcRegenSystem has something to do for every six
//     seconds: a pet, one in a fight, one counting its calm ticks after
//     one, one short of its hit points (regenQuiet);
//   - guard: every NPC whose name has a social rule that comes to help, the
//     only NPCs townGuardLocked can find a town's guard (guardWatchSystem).
//
// An NPC joins a set where its state changes (wakeNPCLocked beside each such
// change; putNPCLocked for one put in the world) and leaves it when a visit
// finds nothing more to do, the same nothing the full scan's next visits
// would have found until the next such change. A set is keyed by the NPC's
// id, so one replaced under its id, or written over in place (the GM panel
// remaking it), stays one member, and the map's own NPC is the one visited.
//
// The NPC map stays the truth. When its size changes other than through
// putNPCLocked and dropNPCLocked (a test filling it), every set is filled
// again from it. With npcSetsCheck on, each system's turn first scans the
// whole map for an NPC with work that is in no set: a change of state made
// without its hook. This package's tests panic on one (main_test.go), so the
// tests that make such a change fail; EQOA_CHECK_NPC_SETS=log has a server
// log each one and put it in its set (the scan costs what the full scans
// did), and any other value panics.
type npcSets struct {
	ai, corpse, regen, guard npcSet

	n        int    // len(w.NPCs) when the sets last knew it
	guardGen uint32 // the social rules' generation the guard set was made for
}

// npcSetsCheck turns the full scan's check of the sets on, npcSetsCheckLog
// to log and mend rather than panic (the type's doc).
var (
	npcSetsCheck    = os.Getenv("EQOA_CHECK_NPC_SETS") != ""
	npcSetsCheckLog = os.Getenv("EQOA_CHECK_NPC_SETS") == "log"
)

// npcSet is one set: its NPCs by id, and the most it has held since its map
// was made.
type npcSet struct {
	m    map[EntityID]*NPC
	peak int
}

func (s *npcSet) add(npc *NPC) {
	if s.m == nil {
		s.m = make(map[EntityID]*NPC)
	}
	s.m[npc.ServerID] = npc
	s.peak = max(s.peak, len(s.m))
}

// tidy makes the set's map again once it holds an eighth of the most it held:
// a Go map keeps its room as its entries go, and a range walks all of it (the
// start's 60,793 NPCs are each looked at once). Call it after a system's loop.
func (s *npcSet) tidy() {
	if s.peak < 512 || len(s.m) > s.peak/8 {
		return
	}
	var m map[EntityID]*NPC
	if len(s.m) > 0 {
		m = make(map[EntityID]*NPC, len(s.m))
		for id, npc := range s.m {
			m[id] = npc
		}
	}
	s.m, s.peak = m, len(m)
}

// wakeNPCLocked puts npc in the sets a change of its state may give work:
// the fights' and the regeneration's (a visit with nothing to do lets it go
// again), and the corpses' when it is dead. Call it where an NPC's enemy,
// fight, death, owner or hit points change. Must be called with the lock
// held.
func (w *World) wakeNPCLocked(npc *NPC) {
	w.sets.ai.add(npc)
	w.sets.regen.add(npc)
	if npc.Dead {
		w.sets.corpse.add(npc)
	}
}

// putNPCLocked puts npc in the world under its id, in place of any NPC there:
// the map and the sets (the spatial index is the caller's). The map stays
// known to the sets only if it was (one changed by hand before is still
// filled again from at the next turn). Must be called with the lock held.
func (w *World) putNPCLocked(npc *NPC) {
	known := len(w.NPCs) == w.sets.n
	w.NPCs[npc.ServerID] = npc
	w.npcChangedLocked(npc)
	if known {
		w.sets.n = len(w.NPCs)
	}
}

// npcChangedLocked is npc new in the world, or made again in place (the GM
// panel's remake): every set looks at it. Must be called with the lock held.
func (w *World) npcChangedLocked(npc *NPC) {
	w.wakeNPCLocked(npc)
	if w.guardRuleLocked(npc) {
		w.sets.guard.add(npc)
	}
}

// dropNPCLocked takes npc out of the world's map and every set, as
// putNPCLocked keeps the map known. Must be called with the lock held.
func (w *World) dropNPCLocked(npc *NPC) {
	id := npc.ServerID
	known := len(w.NPCs) == w.sets.n
	delete(w.NPCs, id)
	delete(w.sets.ai.m, id)
	delete(w.sets.corpse.m, id)
	delete(w.sets.regen.m, id)
	delete(w.sets.guard.m, id)
	if known {
		w.sets.n = len(w.NPCs)
	}
}

// setMember is the NPC a set's entry under id stands for now: the map's own
// (which a remake may have replaced), or nil when the id left the world and
// the entry with it. Must be called with the lock held.
func (w *World) setMember(set map[EntityID]*NPC, id EntityID, npc *NPC) *NPC {
	cur := w.NPCs[id]
	switch {
	case cur == nil:
		delete(set, id)
	case cur != npc:
		set[id] = cur
	}
	return cur
}

// guardRuleLocked reports an NPC whose name has a social rule that comes to
// help: townGuardLocked finds a town's guard among those alone. Must be
// called with the lock held.
func (w *World) guardRuleLocked(npc *NPC) bool {
	if w.npcSocial.gen == 0 {
		return false
	}
	r, ok := w.npcSocial.ruleFor(npc.Name)
	return ok && r.Social
}

// aiQuiet reports an NPC aiSystem has nothing to do for: a pet (petSystem
// fights for it), a dummy with no player, or one with no enemy (or dead)
// whose last fight is put away: no round, no template fight, and, alive, no
// guard's mark and no chase.
func aiQuiet(npc *NPC) bool {
	switch {
	case npc.Owner != 0:
		return true
	case npc.Dummy != NotDummy:
		return npc.DummyTarget == 0
	case npc.CurrentEnemy != 0 && !npc.Dead:
		return false
	}
	return npc.RoundStart.IsZero() && npc.tfight == nil && (npc.Dead || !npc.guardHit && !npc.chase.on)
}

// regenQuiet reports an NPC npcRegenSystem has nothing to do for: a corpse,
// a dummy, or a creature out of a fight, past its calm ticks and whole.
func regenQuiet(npc *NPC) bool {
	if npc.Dead || npc.Dummy != NotDummy {
		return true
	}
	return npc.Owner == 0 && npc.CurrentEnemy == 0 && npc.calmTicks > npcCalmTicks && npc.HP >= npc.MaxHP
}

// corpseQuiet reports an NPC corpseSystem has nothing to do for: a living one.
func corpseQuiet(npc *NPC) bool { return !npc.Dead }

// syncNPCSetsLocked fills every set again from the map when the map's size
// changed other than through putNPCLocked and dropNPCLocked (the type's
// doc). Must be called with the lock held.
func (w *World) syncNPCSetsLocked() {
	if len(w.NPCs) == w.sets.n {
		return
	}
	was, found := w.sets.n, 0
	for _, npc := range w.NPCs {
		if !aiQuiet(npc) {
			if w.sets.ai.m[npc.ServerID] == nil {
				found++
			}
			w.sets.ai.add(npc)
		}
		if !regenQuiet(npc) {
			w.sets.regen.add(npc)
		}
		if npc.Dead {
			w.sets.corpse.add(npc)
		}
	}
	w.refillGuardsLocked()
	w.sets.n = len(w.NPCs)
	if was != 0 && found > 0 {
		// in a server, every NPC comes and goes through putNPCLocked and
		// dropNPCLocked; this is one that did not
		log.Printf("NPC sets: the NPC map went from %d to %d outside putNPCLocked/dropNPCLocked; %d NPCs with a fight found and added", was, len(w.NPCs), found)
	}
}

// refillGuardsLocked makes the guard set again: every NPC with a helping
// social rule, for the rules' generation now. Must be called with the lock
// held.
func (w *World) refillGuardsLocked() {
	w.sets.guard = npcSet{}
	for _, npc := range w.NPCs {
		if w.guardRuleLocked(npc) {
			w.sets.guard.add(npc)
		}
	}
	w.sets.guardGen = w.npcSocial.gen
}

// checkNPCSetLocked is npcSetsCheck's scan (the type's doc): an NPC of the
// map in no entry of members that quiet says has work panics, or with
// npcSetsCheckLog is logged and put in members. Must be called with the lock
// held.
func (w *World) checkNPCSetLocked(set string, members *npcSet, quiet func(*NPC) bool) {
	for id, npc := range w.NPCs {
		if _, in := members.m[id]; in || quiet(npc) {
			continue
		}
		msg := fmt.Sprintf("NPC sets: %q (id %d) has work for the %s set and is not in it: "+
			"enemy %d dead %v owner %d dummy %d/%d round %v tfight %v guardHit %v chase %v calm %d HP %d/%d "+
			"(an NPC's state changed without wakeNPCLocked; a test that sets it by hand calls it too)%s",
			npc.Name, id, set, npc.CurrentEnemy, npc.Dead, npc.Owner, npc.Dummy, npc.DummyTarget,
			!npc.RoundStart.IsZero(), npc.tfight != nil, npc.guardHit, npc.chase.on, npc.calmTicks, npc.HP, npc.MaxHP,
			testCaller())
		if !npcSetsCheckLog {
			panic(msg)
		}
		log.Print(msg)
		members.add(npc)
	}
}

// testCaller is " in <file:line>" of the first _test.go frame calling it, or
// "" (a server).
func testCaller() string {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.File, "_test.go") {
			return fmt.Sprintf(" in %s:%d", filepath.Base(f.File), f.Line)
		}
		if !more {
			return ""
		}
	}
}

// checkGuardSetLocked is npcSetsCheck's scan of the guard set. Must be
// called with the lock held.
func (w *World) checkGuardSetLocked() {
	w.checkNPCSetLocked("guard", &w.sets.guard, func(npc *NPC) bool { return !w.guardRuleLocked(npc) })
}
