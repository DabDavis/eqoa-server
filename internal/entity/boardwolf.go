package entity

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// boardwolf.go — the towns' boards' Spirit of the Wolf (petition 1042, the GM
// Jibrish: "Add a 1 hour duration buff people can get from the announcements
// boards in towns, the same one that currently gives the XP bonus. This buff
// should be spirit of the wolf, and give the according run speed. It should
// drop if the player takes a hostile action."). A town's announcement board
// (handlers/signs.go) puts it on whoever talks to it, beside the boon: the
// movement buff's run speed (speedBuff, Spirit of the Wolf's own) for an
// hour of play, its icon the spell's; a talk with one on puts it back to its
// whole hour. It ends at the player's own hostile act: auto-attack engaged or
// a swing, a harmful spell cast, a potion thrown (breakBoardWolfLocked); not
// at a hit taken, nor at a friendly spell. Its time runs only in the world
// and it is kept over a logout, as every effect's (buffsave.go).

// BoardWolfEffectID keys it among a player's effects (the boon is -1003).
const BoardWolfEffectID = -1004

// BoardWolfName and BoardWolfIcon are its slot on the effects channel: Spirit
// of the Wolf's name and icon (spellpattern 452's).
const (
	BoardWolfName          = "Spirit of the Wolf"
	BoardWolfIcon   uint32 = 0x9CDB7CC9
	BoardWolfLength        = time.Hour
)

// GiveBoardWolf puts the boards' Spirit of the Wolf on the session's player
// for its whole hour, in place of one on it. It reports whether the player
// was given it (false: no living player) and whether one was on already.
func (w *World) GiveBoardWolf(sess *session.Session, now time.Time) (given, renewed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return false, false
	}
	renewed = p.hasEffect(BoardWolfEffectID)
	w.addEffectLocked(p, &ActiveEffect{SpellID: BoardWolfEffectID, Name: BoardWolfName, Icon: BoardWolfIcon,
		Caster: p.ServerID, Until: now.Add(BoardWolfLength), Speed: true}, now)
	log.Printf("Board: %s has the boards' %s for %v (renewed %v)", p.Name, BoardWolfName, BoardWolfLength, renewed)
	return true, renewed
}

// breakBoardWolfLocked ends the boards' Spirit of the Wolf at the player's
// own hostile act (why says which). Must be called with the lock held.
func (w *World) breakBoardWolfLocked(p *Player, why string) {
	if p != nil && w.removeEffectLocked(p, BoardWolfEffectID) {
		log.Printf("Board: %s's %s ends with %s", p.Name, BoardWolfName, why)
	}
}
