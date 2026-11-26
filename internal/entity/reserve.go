package entity

// reserve.go — a woken creature kept for the party that woke it (the owner,
// 2026-11-26, Heirloom petition 3257: the Necrologist's death on Rain 4 woke
// Chimareon the Tyrant on Rain 5, and a player already standing there killed
// him before the group that woke him could port in).
//
// A trigger's spawn step may say reserve "waker" (database.TriggerStep): when
// the run was fired by a creature's death (handlers/gmtriggers.go's Killed),
// each copy it places is the killer's party's, its group or raid with it
// (ReserveNPC). While it holds:
//
//   - no one else may hurt it: their attacks and harmful spells are refused with
//     ReserveLine (challengeBars and challengeRefusal, as a Metal Moogie or a
//     siege's King is kept), their pets and mercs held off it too, and nothing
//     they do puts them on its hate list (addThreatLocked);
//   - it notices no one else on sight (threatensLocked).
//
// The party is the characters credited with the waking kill, and every member
// of a group or raid one of them stands in, looked at again each second
// (reserveSystem): one who joins the group afterwards is the party's, and stays
// so. Their pets and mercs are the party's while they are. It lifts:
//
//   - when no one of the party has stood alive in the creature's world for
//     ReserveGrace (all gone from the zone, or all dead: the grace covers a
//     portal's load and a quick rez);
//   - at its end (the step's reserveminutes; none: the creature's whole stay),
//     unless the creature is fighting the party then: it holds until that fight
//     is over, so the end never opens a fight under way to a bystander;
//   - with the creature, dead or gone.
//
// A keep restart carries it (keeptrigger.go's KeptEventNPC.Reserve).

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// ReserveGrace is how long the party may all be gone from the creature's world,
// or all dead, before its reservation lifts.
const ReserveGrace = 60 * time.Second

// reserveEvery is how often the reservations are looked at.
const reserveEvery = time.Second

// ReserveLine is what one barred from a reserved creature is told (%s its name).
const ReserveLine = "%s is bound to another party's challenge."

// WakerParty is who woke a trigger's creature: the characters credited with the
// waking kill (characters.serverid), Name as the log says them.
type WakerParty struct {
	Name  string   `json:"name"`
	Chars []uint32 `json:"chars"`
}

// npcReserve is a creature's reservation (the file's doc).
type npcReserve struct {
	name  string
	chars map[uint32]bool // the party's characters (sticky: a member once is one)
	until time.Time       // its end; zero: the creature's whole stay
	// gone is when the party was first seen all gone or dead (zero: someone stands)
	gone time.Time
	// may is the entity ids that may hurt it now: the party's players, pets and
	// mercs (reserveRefreshLocked)
	may map[EntityID]bool
}

// reserveBars reports id (a player, or a pet or merc) barred from hurting npc,
// another party's reserved creature.
func reserveBars(npc *NPC, id EntityID) bool {
	return npc != nil && npc.reserve != nil && !npc.Dead && !npc.reserve.may[id]
}

// reserveRefusal is the line for one barred from a reserved creature, "" for none.
func reserveRefusal(npc *NPC, id EntityID) string {
	if !reserveBars(npc, id) {
		return ""
	}
	return fmt.Sprintf(ReserveLine, npc.Name)
}

// ReserveNPC keeps the creature id for party (the file's doc) until until (zero:
// its whole stay). False for one gone or dead, or a party of no one.
func (w *World) ReserveNPC(id EntityID, party WakerParty, until time.Time) bool {
	if len(party.Chars) == 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil || npc.Dead {
		return false
	}
	r := &npcReserve{name: party.Name, chars: map[uint32]bool{}, until: until}
	for _, c := range party.Chars {
		if c != 0 {
			r.chars[c] = true
		}
	}
	if len(r.chars) == 0 {
		return false
	}
	w.setReserveLocked(npc, r)
	log.Printf("Reserve: %s (%d) kept for %s (%d characters), %s", npc.Name, npc.ServerID, r.name, len(r.chars), reserveEnd(until))
	return true
}

// setReserveLocked puts r on npc and looks at its party at once. Must be called
// with the lock held.
func (w *World) setReserveLocked(npc *NPC, r *npcReserve) {
	npc.reserve = r
	if w.reserved == nil {
		w.reserved = map[EntityID]*NPC{}
	}
	w.reserved[npc.ServerID] = npc
	w.reserveRefreshLocked(r)
}

// reserveEnd is a reservation's end as a log line says it.
func reserveEnd(until time.Time) string {
	if until.IsZero() {
		return "for its whole stay"
	}
	return "until " + until.UTC().Format("15:04:05")
}

// ReservedFor is the party the creature id is kept for ("" none) and its
// characters, for the GM panel and the tests.
func (w *World) ReservedFor(id EntityID) (string, []uint32) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc := w.NPCs[id]
	if npc == nil || npc.reserve == nil {
		return "", nil
	}
	chars := make([]uint32, 0, len(npc.reserve.chars))
	for c := range npc.reserve.chars {
		chars = append(chars, c)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	return npc.reserve.name, chars
}

// reserveRefreshLocked makes the party what it is now: every member of a group
// or raid one of its characters stands in joins it, and may is its players in
// the world and their pets, side pets and mercs. It returns the party's players
// in the world. Must be called with the lock held.
func (w *World) reserveRefreshLocked(r *npcReserve) []*Player {
	groups := map[uint32]bool{}
	for _, p := range w.Players {
		if r.chars[p.CharServerID] && p.GroupID != 0 {
			groups[p.GroupID] = true
		}
	}
	for gid := range groups {
		if g := w.groups[gid]; g != nil {
			for _, id := range g.Members {
				if m := w.Players[id]; m != nil && m.CharServerID != 0 {
					r.chars[m.CharServerID] = true
				}
			}
		}
	}
	may := map[EntityID]bool{}
	var party []*Player
	for _, p := range w.Players {
		if !r.chars[p.CharServerID] {
			continue
		}
		party = append(party, p)
		may[p.ServerID] = true
		if pet := w.petOfLocked(p); pet != nil {
			may[pet.ServerID] = true
		}
		if sp := w.sidePets[p.ServerID]; sp != nil && sp.npc != nil {
			may[sp.npc.ServerID] = true
		}
		for _, ms := range w.mercsOfLocked(p) {
			may[ms.npc.ServerID] = true
		}
	}
	r.may = may
	return party
}

// reserveSystem looks at the reservations once a second: each party made what it
// is now, and lifted as the file's doc says. Must be called with the lock held.
func (w *World) reserveSystem(now time.Time) {
	if len(w.reserved) == 0 || now.Before(w.reserveNext) {
		return
	}
	w.reserveNext = now.Add(reserveEvery)
	for id, npc := range w.reserved {
		if cur := w.NPCs[id]; cur != npc || npc.Dead || npc.reserve == nil {
			delete(w.reserved, id)
			if cur == npc && npc.reserve != nil {
				npc.reserve = nil // dead: its corpse is no one's to keep
			}
			continue
		}
		if why := w.reserveLapseLocked(npc, now); why != "" {
			log.Printf("Reserve: %s (%d) no longer kept for %s: %s", npc.Name, npc.ServerID, npc.reserve.name, why)
			npc.reserve = nil
			delete(w.reserved, id)
		}
	}
}

// reserveLapseLocked is why npc's reservation lifts now, "" while it holds. Must
// be called with the lock held.
func (w *World) reserveLapseLocked(npc *NPC, now time.Time) string {
	r := npc.reserve
	party := w.reserveRefreshLocked(r)
	here := false
	for _, p := range party {
		if p.World == npc.World && !p.IsDead() {
			here = true
			break
		}
	}
	if here {
		r.gone = time.Time{}
	} else if r.gone.IsZero() {
		r.gone = now
	} else if now.Sub(r.gone) >= ReserveGrace {
		return fmt.Sprintf("its party all gone from its world or dead for %s", ReserveGrace)
	}
	if !r.until.IsZero() && !now.Before(r.until) && !w.reserveFightLocked(npc) {
		return "its time is over"
	}
	return ""
}

// reserveFightLocked reports npc in a fight with its party: one of them on its
// hate list, or its enemy. Must be called with the lock held.
func (w *World) reserveFightLocked(npc *NPC) bool {
	r := npc.reserve
	if r == nil {
		return false
	}
	if r.may[npc.CurrentEnemy] {
		return true
	}
	for id := range npc.Threats {
		if r.may[id] {
			return true
		}
	}
	return false
}

// KeptReserve is a reservation as a restart carries it.
type KeptReserve struct {
	Name  string    `json:"name"`
	Chars []uint32  `json:"chars"`
	Until time.Time `json:"until,omitzero"`
	Gone  time.Time `json:"gone,omitzero"`
}

// keptReserveLocked is npc's reservation as a restart carries it, nil for none.
// Must be called with the lock held.
func keptReserveLocked(npc *NPC) *KeptReserve {
	r := npc.reserve
	if r == nil {
		return nil
	}
	k := &KeptReserve{Name: r.name, Until: r.until, Gone: r.gone}
	for c := range r.chars {
		k.Chars = append(k.Chars, c)
	}
	sort.Slice(k.Chars, func(i, j int) bool { return k.Chars[i] < k.Chars[j] })
	return k
}

// restoreReserveLocked puts a kept reservation back on npc (DelayReserve moves
// its times on by the restart's length). Must be called with the lock held.
func (w *World) restoreReserveLocked(npc *NPC, k *KeptReserve) {
	if k == nil || len(k.Chars) == 0 {
		return
	}
	r := &npcReserve{name: k.Name, chars: map[uint32]bool{}, until: k.Until, gone: k.Gone}
	for _, c := range k.Chars {
		r.chars[c] = true
	}
	w.setReserveLocked(npc, r)
	log.Printf("Reserve: %s (%d) kept for %s again after the restart, %s", npc.Name, npc.ServerID, strings.TrimSpace(r.name), reserveEnd(r.until))
}

// DelayReserve moves the times of the creature id's reservation on by gap (a keep
// restart's length, as a trigger run's times are), so the hold counts against
// neither its end nor its party's absence.
func (w *World) DelayReserve(id EntityID, gap time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil || npc.reserve == nil || gap <= 0 {
		return
	}
	if r := npc.reserve; !r.until.IsZero() {
		r.until = r.until.Add(gap)
	}
	if r := npc.reserve; !r.gone.IsZero() {
		r.gone = r.gone.Add(gap)
	}
}
