package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// ctfaway.go — a mercenary and a familiar never enter the arena (the CTF deploy's rule of 2026-11-10, under the
// owner's "go ahead with CTF using your proposed defaults"): a match is its players' alone, so a merc never counts on
// a team, never takes or carries a banner, and neither a merc's heals nor Healbot's reach a match. Class pets are the
// PvP fight's own business (pvp.go's pvpPetLocked) and go in as they always did.
//
// When its owner is sent into a match (ctfEnterLocked), or stands in the arena's world any other way (a GM's move,
// a keep restart), its merc and its familiar go out of the world as at a logout (mercGoesLocked: out of its owner's
// group too; familiarGoesLocked) and wait (w.ctf.away), its owner told. Nothing calls one in while its owner is in
// a match or the arena's world: a hire or /merc is refused with the same line (HireMerc), a familiar's item lands
// nothing (summonFamiliarLocked), a keep restart puts what it carried back to waiting (RestoreMerc,
// RestoreFamiliar), and a merc or familiar found there all the same is sent out at its next step (mercSystem,
// familiarSystem). Once its owner is out of every match and of the arena's world, in the world, alive and done
// loading (ctfAwayBackLocked, every tick), its familiar comes back as it was, with its share of hit points, and its
// merc is made again from its hire's row beside it (MercNoteBack: handlers/merc.go, as at a login: whole, its stance
// kept, asked into a group again). A dismissal while it waits ends the hire (DismissMerc, DismissFamiliar); a logout
// lets the wait go (a login brings the merc back from its row, and a familiar is not kept over a logout); a keep
// restart carries what waits as if it were out (KeepPlayer: beside its owner afterwards). The words are ours.

// ctfAway is what a player left outside the arena.
type ctfAway struct {
	who *Player
	// merc is its merc's name, "" none: the handlers make it again from its hire's row
	merc string
	// familiar is its familiar as it was, nil none, and famHP its share of hit points
	familiar *FamiliarDef
	famHP    float64
}

// The lines a merc's or a familiar's owner is told (ours).
const (
	ctfLineMercAway     = "Your mercenary %s waits outside the arena: it rejoins you when you leave it."
	ctfLineFamiliarAway = "%s waits outside the arena: it rejoins you when you leave it."
	ctfLineFamiliarBack = "%s is back at your side."
)

// ctfKeepsOutLocked reports p where no merc or familiar goes: in a match (sent in or on its way) or in the arena's
// world. With capture the flag off, nowhere. Must be called with the lock held.
func (w *World) ctfKeepsOutLocked(p *Player) bool {
	if p == nil {
		return false
	}
	for _, g := range arenaGames { // either game's (read only: no swap)
		if st := w.ctfGameLocked(g); st.on && (p.World == st.set.World || p.inMatch(st.m)) {
			return true
		}
	}
	return false
}

// ctfAwayGameLocked is the game whose list of what waits outside has p, ok false none. Must be called with the lock
// held (read will do).
func (w *World) ctfAwayGameLocked(p *Player) (ArenaGame, bool) {
	for _, g := range arenaGames {
		if a := w.ctfGameLocked(g).away[p.ServerID]; a != nil && a.who == p {
			return g, true
		}
	}
	return GameCTF, false
}

// ctfAwayOfLocked is p's record of what waits outside, nil for none (a record of another login under its id is
// none: ctfAwayBackLocked lets it go). Must be called with the lock held (read will do).
func (w *World) ctfAwayOfLocked(p *Player) *ctfAway {
	if g, ok := w.ctfAwayGameLocked(p); ok {
		return w.ctfGameLocked(g).away[p.ServerID]
	}
	return nil
}

// ctfAwayMadeLocked is p's record of what waits outside, made if it has none. Must be called with the lock held.
func (w *World) ctfAwayMadeLocked(p *Player) *ctfAway {
	if a := w.ctf.away[p.ServerID]; a != nil && a.who == p {
		return a
	}
	if w.ctf.away == nil {
		w.ctf.away = map[EntityID]*ctfAway{}
	}
	a := &ctfAway{who: p}
	w.ctf.away[p.ServerID] = a
	return a
}

// ctfMercWaitsLocked keeps p's merc name waiting outside the arena and tells p so (sess p's, or nil). Must be
// called with the lock held.
func (w *World) ctfMercWaitsLocked(p *Player, sess *session.Session, name string) {
	defer w.ctfForLocked(p)()
	w.ctfAwayMadeLocked(p).merc = name
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(ctfLineMercAway, name))
	}
	log.Printf("%s: %s's mercenary %q waits outside the arena", w.ctfTag(), p.Name, name)
}

// ctfFamiliarWaitsLocked keeps p's familiar d (its share of hit points hp) waiting outside the arena and tells p so
// (sess p's, or nil). Must be called with the lock held.
func (w *World) ctfFamiliarWaitsLocked(p *Player, sess *session.Session, d FamiliarDef, hp float64) {
	defer w.ctfForLocked(p)()
	a := w.ctfAwayMadeLocked(p)
	a.familiar, a.famHP = &d, hp
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(ctfLineFamiliarAway, d.Name))
	}
	log.Printf("%s: %s's familiar %q waits outside the arena", w.ctfTag(), p.Name, d.Name)
}

// ctfSendAwayLocked takes p's merc and familiar out of the world, as at a logout, to wait outside the arena (the
// file's doc). Must be called with the lock held.
func (w *World) ctfSendAwayLocked(p *Player, now time.Time) {
	defer w.ctfForLocked(p)()
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if ms := w.mercOfLocked(p); ms != nil {
		name := ms.npc.Name
		w.mercGoesLocked(ms, sess, now)
		w.ctfMercWaitsLocked(p, sess, name)
	}
	if fs := w.familiars[p.ServerID]; fs != nil && fs.owner == p && !fs.npc.Dead {
		d, hp := fs.def, share(fs.npc.HP, fs.npc.MaxHP)
		w.familiarGoesLocked(fs, sess)
		w.ctfFamiliarWaitsLocked(p, sess, d, hp)
	}
}

// ctfAwayBackLocked brings back what waits outside the arena for each player out of it: in no match and in another
// world, in the world, alive, done loading and with no travel queued. A player gone (a logout) lets its wait go.
// Every tick (ctfSystem); must be called with the lock held.
func (w *World) ctfAwayBackLocked(now time.Time) {
	for id, a := range w.ctf.away {
		p := w.Players[id]
		if p == nil || p != a.who {
			delete(w.ctf.away, id)
			continue
		}
		if w.ctfKeepsOutLocked(p) || p.IsDead() || p.respawnPending() || w.ctfTravelQueuedLocked(p) {
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if sess == nil || !sess.CharacterInWorld() || sess.StateChannelsHeld() {
			continue
		}
		delete(w.ctf.away, id)
		if a.familiar != nil {
			w.summonFamiliarLocked(p, sess, a.familiar, now)
			if fs := w.familiars[id]; fs != nil && fs.owner == p {
				fs.npc.HP = keptShare(a.famHP, fs.npc.MaxHP)
				w.sendChatToSession(sess, fmt.Sprintf(ctfLineFamiliarBack, a.familiar.Name))
			}
		}
		if a.merc != "" {
			w.noteMercLocked(MercNote{Kind: MercNoteBack, Owner: p, Sess: sess, Name: a.merc, Level: p.Level})
		}
		log.Printf("%s: %s is out of the arena: its mercenary %q and familiar %v come back", w.ctfTag(), p.Name, a.merc, a.familiar != nil)
	}
}

// ctfTravelQueuedLocked reports a travel of p's queued (gates.go's due list). Must be called with the lock held.
func (w *World) ctfTravelQueuedLocked(p *Player) bool {
	for _, t := range w.gates.due {
		if t.P == p {
			return true
		}
	}
	return false
}

// ctfDismissAwayLocked ends p's waiting merc (merc true) or familiar, reporting its name, false for none. Must be
// called with the lock held.
func (w *World) ctfDismissAwayLocked(p *Player, merc bool) (string, bool) {
	g, ok := w.ctfAwayGameLocked(p)
	if !ok {
		return "", false
	}
	defer w.ctfUseLocked(g)()
	a := w.ctf.away[p.ServerID]
	var name string
	switch {
	case merc && a.merc != "":
		name, a.merc = a.merc, ""
	case !merc && a.familiar != nil:
		name, a.familiar = a.familiar.Name, nil
	default:
		return "", false
	}
	if a.merc == "" && a.familiar == nil {
		delete(w.ctf.away, p.ServerID)
	}
	return name, true
}

// ctfKeptAwayLocked writes what waits outside the arena for p into what a keep restart carries, as if it were out
// (keepmerc.go): the new process puts the merc and the familiar beside their owner, or back to waiting while their
// owner stands in the arena's world. Must be called with the lock held.
func (w *World) ctfKeptAwayLocked(p *Player, k *KeptPlayer) {
	g, ok := w.ctfAwayGameLocked(p)
	if !ok {
		return
	}
	defer w.ctfUseLocked(g)()
	a := w.ctf.away[p.ServerID]
	if a.merc != "" && k.Merc == nil {
		// no place of its own: RestoreMerc stands it beside its owner (its world is not its owner's)
		k.Merc = &KeptMerc{Name: a.merc, HPPct: 1, PowerPct: 1, World: w.ctf.set.World, Away: true}
	}
	if a.familiar != nil && k.Familiar == nil {
		k.Familiar = &KeptFamiliar{Item: a.familiar.Item, HPPct: a.famHP}
	}
}
