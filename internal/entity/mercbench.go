package entity

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/eqoa/server/internal/session"
)

// mercbench.go — up to three mercs a player, and the standby (the owner,
// 2026-11-12: "if a player wants to have 3 mercenaries in their group it is
// fine. If they invite a player they will replace a merc, but if the player
// leaves the merc comes back").
//
// A player hires up to MercSlots mercs, each in a slot of its own (1 to 3:
// charmerc.slot, /merc 2, its follow place round its owner: followslot.go).
// Each takes a place in its owner's group as one merc always did
// (mercgroup.go), so a player alone with three is a group of four.
//
// A player takes a place before a merc: a plain group (its leader raised no
// raid flag) is full at GroupWindowSize PLAYERS, not members. A player's yes
// to an invitation into a group of four with mercs in it puts the player in
// and the merc that joined the group LAST out of it, onto standby
// (mercsMakeRoomLocked); the joiner's own mercs come in only to places still
// free, the rest on standby. A merc that finds no place (a hire, a revival,
// a login's coming back into a full group) goes on standby too, never
// refused. On standby a merc is out of the world (its NPC gone, as at a
// logout) but its hire is kept whole (its row: name, class, race, look,
// stance, path, pet; its level is its owner's at every coming).
//
// A place freeing in its owner's group (a player leaves, is booted, logs out,
// goes to another group; a merc dies or is dismissed) brings a standby merc
// back by itself, beside its owner and into the group (mercBenchSystemLocked,
// every 5 ticks, with the group upkeep): the standby of that group's owners
// in the order they stepped out, first out first back, one a free place; so
// the owner who lost one first has one back first. An owner in no group, or
// in a raid flag's group (where a merc takes no place: raidflag.go), has all
// of its standby back. The handlers make it again from its row
// (MercNoteBack's Slot), as at a login.
//
// The standby lives in the World (mercBench) while its owner is in it; a
// logout lets it go (the login brings every hire back, those with no place on
// standby again), and a keep restart carries it (keepmerc.go: KeptMerc's
// Standby).

// mercBenched is a merc on standby: whose, its slot, its name, the order it
// stepped out in, and when its return was last asked of the handlers.
type mercBenched struct {
	owner *Player
	slot  int
	name  string
	seq   uint64
	asked time.Time
}

// mercBenchAskFor is how long a return asked of the handlers holds its place
// before it is asked again.
const mercBenchAskFor = 10 * time.Second

// The standby's lines (ours).
const (
	mercStandbyFullLine = "Your mercenary %s waits on standby: your group has no place for it. It comes back when a place frees."
	mercStandbyForLine  = "Your mercenary %s steps out of the group to make room for %s. It waits on standby and comes back when a place frees."
	mercStandbyOutLine  = "Your mercenary %s steps out of the group: it has no place for it. It waits on standby and comes back when a place frees."
)

// benchedOfLocked is p's mercs on standby, in the order they stepped out.
// Must be called with the lock held.
func (w *World) benchedOfLocked(p *Player) []*mercBenched {
	if p == nil {
		return nil
	}
	var out []*mercBenched
	for _, b := range w.mercBench[p.ServerID] {
		if b.owner == p {
			out = append(out, b)
		}
	}
	return out
}

// benchedAtLocked is p's merc of the slot on standby, nil for none. Must be
// called with the lock held.
func (w *World) benchedAtLocked(p *Player, slot int) *mercBenched {
	for _, b := range w.benchedOfLocked(p) {
		if b.slot == slot {
			return b
		}
	}
	return nil
}

// benchSlotLocked puts p's merc of the slot (name) on standby, p told line
// (with the name) on sess (nil: no one). One there already keeps its turn.
// Must be called with the lock held.
func (w *World) benchSlotLocked(p *Player, sess *session.Session, slot int, name, line string, now time.Time) {
	if b := w.benchedAtLocked(p, slot); b != nil {
		b.name, b.asked = name, time.Time{}
	} else {
		if w.mercBench == nil {
			w.mercBench = map[EntityID][]*mercBenched{}
		}
		w.mercBenchSeq++
		list := w.mercBench[p.ServerID][:0:0]
		for _, o := range w.mercBench[p.ServerID] {
			if o.owner == p {
				list = append(list, o)
			}
		}
		w.mercBench[p.ServerID] = append(list, &mercBenched{owner: p, slot: slot, name: name, seq: w.mercBenchSeq})
	}
	if sess != nil && line != "" {
		w.sendChatToSession(sess, fmt.Sprintf(line, name))
	}
	log.Printf("Merc: %s's %q (slot %d) on standby", p.Name, name, slot)
}

// unbenchLocked takes p's merc of the slot off standby (it came, or its hire
// ended). Must be called with the lock held.
func (w *World) unbenchLocked(p *Player, slot int) {
	list := w.mercBench[p.ServerID]
	for i, b := range list {
		if b.owner == p && b.slot == slot {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(w.mercBench, p.ServerID)
		return
	}
	w.mercBench[p.ServerID] = list
}

// benchLeavingLocked lets p's standby go with it out of the world (the file's
// doc). Must be called with the lock held.
func (w *World) benchLeavingLocked(p *Player) {
	if list := w.mercBench[p.ServerID]; len(list) > 0 && list[0].owner == p {
		delete(w.mercBench, p.ServerID)
	}
}

// benchMercLocked puts a merc out on standby: out of the world and its group,
// its owner told line (with its name, then extra's when the line takes two).
// Must be called with the lock held.
func (w *World) benchMercLocked(ms *mercState, line string, extra string, now time.Time) {
	o, name, slot := ms.owner, ms.npc.Name, ms.slot
	sess := w.sessionOfLocked(o)
	w.mercGoesLocked(ms, sess, now)
	w.benchSlotLocked(o, nil, slot, name, "", now)
	if sess != nil {
		if extra != "" {
			w.sendChatToSession(sess, fmt.Sprintf(line, name, extra))
		} else {
			w.sendChatToSession(sess, fmt.Sprintf(line, name))
		}
	}
}

// groupPlayersLocked is how many of g's members are players. Must be called
// with the lock held.
func (w *World) groupPlayersLocked(g *Group) int {
	n := 0
	for _, id := range g.membersOrNone() {
		if w.Players[id] != nil {
			n++
		}
	}
	return n
}

// groupFullForPlayerLocked reports g with no place for another player by an
// invitation: a plain group's four players (its mercs make way: the file's
// doc), a raid flag's group's four members as ever (its flag asks past them).
// Must be called with the lock held.
func (w *World) groupFullForPlayerLocked(g *Group) bool {
	if g == nil {
		return false
	}
	if g.flagged {
		return len(g.Members) >= GroupWindowSize
	}
	return w.groupPlayersLocked(g) >= GroupWindowSize
}

// mercRoomLocked reports a place for a merc of p's in p's group, old (a merc
// it takes the place of, or nil) giving back its own: no group, a raid flag's
// group, or a plain one of fewer than GroupWindowSize. Must be called with
// the lock held.
func (w *World) mercRoomLocked(p *Player, old *mercState) bool {
	g := w.groups[p.GroupID]
	if p.GroupID == 0 || g == nil || g.flagged {
		return true
	}
	n := len(g.Members)
	if old != nil && groupHas(g, old.npc.ServerID) {
		n--
	}
	return n < GroupWindowSize
}

// mercsMakeRoomLocked puts on standby, while plain g has more than
// GroupWindowSize members, the merc that joined it last (the file's doc),
// each owner told for whom (joiner, nil for no one). Must be called with the
// lock held.
func (w *World) mercsMakeRoomLocked(g *Group, joiner *Player, now time.Time) {
	for g != nil && !g.flagged && len(g.Members) > GroupWindowSize {
		var last *mercState
		for i := len(g.Members) - 1; i >= 0 && last == nil; i-- {
			if n := w.NPCs[g.Members[i]]; n.isMerc() && w.mercLiveLocked(n.merc) {
				last = n.merc
			}
		}
		if last == nil {
			return
		}
		if joiner != nil {
			w.benchMercLocked(last, mercStandbyForLine, joiner.Name, now)
		} else {
			w.benchMercLocked(last, mercStandbyOutLine, "", now)
		}
		if w.groups[g.ID] != g {
			return
		}
	}
}

// mercBenchSystemLocked brings standby mercs back to free places (the file's
// doc): their owners gone let go, a plain group past four made to fit, and
// each free place asked of the handlers for the standby merc that stepped out
// first. Every 5 ticks; must be called with the lock held.
func (w *World) mercBenchSystemLocked(now time.Time) {
	if w.TickCount%5 != 0 {
		return
	}
	for _, g := range w.groups {
		if !g.flagged && len(g.Members) > GroupWindowSize {
			w.mercsMakeRoomLocked(g, nil, now)
		}
	}
	if len(w.mercBench) == 0 {
		return
	}
	byGroup := map[uint32][]*mercBenched{}
	for id, list := range w.mercBench {
		if len(list) == 0 || w.Players[id] != list[0].owner {
			delete(w.mercBench, id)
			continue
		}
		o := list[0].owner
		sess := w.sessionOfLocked(o)
		if sess == nil || !sess.CharacterInWorld() || o.IsDead() || o.respawnPending() || w.ctfKeepsOutLocked(o) {
			continue
		}
		g := w.groups[o.GroupID]
		if o.GroupID == 0 || g == nil || g.flagged {
			for _, b := range list {
				w.askBenchBackLocked(b, sess, now) // a place for each
			}
			continue
		}
		byGroup[g.ID] = append(byGroup[g.ID], list...)
	}
	for gid, list := range byGroup {
		g := w.groups[gid]
		free := GroupWindowSize - len(g.Members)
		var waiting []*mercBenched
		for _, b := range list {
			if !b.asked.IsZero() && now.Sub(b.asked) < mercBenchAskFor {
				free-- // its return on its way holds a place
				continue
			}
			waiting = append(waiting, b)
		}
		sort.Slice(waiting, func(i, j int) bool { return waiting[i].seq < waiting[j].seq })
		for _, b := range waiting {
			if free <= 0 {
				break
			}
			w.askBenchBackLocked(b, w.sessionOfLocked(b.owner), now)
			free--
		}
	}
}

// askBenchBackLocked asks the handlers to bring the standby merc back from its
// row (MercNoteBack), once a mercBenchAskFor. Must be called with the lock
// held.
func (w *World) askBenchBackLocked(b *mercBenched, sess *session.Session, now time.Time) {
	if !b.asked.IsZero() && now.Sub(b.asked) < mercBenchAskFor {
		return
	}
	b.asked = now
	w.noteMercLocked(MercNote{Kind: MercNoteBack, Owner: b.owner, Slot: b.slot, Sess: sess, Name: b.name, Level: b.owner.Level})
	log.Printf("Merc: %s's %q (slot %d) comes back from standby", b.owner.Name, b.name, b.slot)
}

// MercStandby is the session's player's mercs on standby, by slot: their
// names.
func (w *World) MercStandby(sess *session.Session) map[int]string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := map[int]string{}
	for _, b := range w.benchedOfLocked(w.sessionPlayerLocked(sess)) {
		out[b.slot] = b.name
	}
	return out
}

// BenchMerc puts the session's player's merc of the slot (name), not out, on
// standby, told line (a keep restart's carried standby: ""). The handlers'
// coming back of a hire with no place (HireMercAt) needs none of it.
func (w *World) BenchMerc(sess *session.Session, slot int, name string, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil && w.mercAtLocked(p, slot) == nil {
		w.benchSlotLocked(p, nil, slot, name, "", now)
	}
}

// DropStandby takes the session's player's merc of the slot off standby (its
// row gone: nothing to bring back).
func (w *World) DropStandby(sess *session.Session, slot int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.sessionPlayerLocked(sess); p != nil {
		w.unbenchLocked(p, slot)
	}
}
