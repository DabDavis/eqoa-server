package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// Ammunition (petition 2390, 2026-11-18: "arrows not getting
// used. have stack of 9999 and it just stays there"). CLIENT's HasAmmoFor
// (0x00BDC8F8, decomp viclient.cpp), asked by TargetInRange before every
// ranged swing: attack type 7 (a bow) wants an item of kind 1 in the pack that
// is never worn, 9 (a crossbow) one of kind 2, 10 (a thrown weapon or a wand)
// nothing; otherwise word 724 or 725 in red and no swing. The kind is the
// item record's field after its wear slot (VIItem +0x18, itempattern.unk3):
// "arrow" 1805 and "Arrow of Love" are 1, "bolt" 1324 is 2 (some bows carry 1
// or 2 there too, but a bow is worn, so CLIENT never counts one). CLIENT
// spends nothing itself: the server takes one off the stack for each shot
// and says so (0x0039, as a thrown potion's).
const (
	AmmoArrows = 1
	AmmoBolts  = 2
)

// AmmoKind is the ammunition a ranged weapon of that attack type uses, 0 none.
func AmmoKind(attackType int) int {
	switch attackType {
	case 7:
		return AmmoArrows
	case 9:
		return AmmoBolts
	}
	return 0
}

// IsAmmo is whether an item record (its unk3 and its wear slot) is
// ammunition of that kind, as HasAmmoFor counts it: the kind and never worn.
func IsAmmo(unk3, equipSlot, kind int) bool {
	return kind != 0 && unk3 == kind && equipSlot == -1
}

// AmmoSpentFunc takes one piece of ammunition of the kind off the character's
// pack (the handlers' hook), called after a shot, off the World's lock.
type AmmoSpentFunc func(sess *session.Session, charServerID uint32, kind int)

// OnAmmoSpent sets the hook that takes a shot's ammunition off its stack.
func (w *World) OnAmmoSpent(fn AmmoSpentFunc) {
	w.mu.Lock()
	w.onAmmoSpent = fn
	w.mu.Unlock()
}

// shotAmmoLocked is whether p's ranged shot has its ammunition (a swing that
// needs none: true). Without it the shot is not made, as CLIENT makes none:
// CLIENT has said word 724 or 725 itself. Must be called with the lock held.
func (w *World) shotAmmoLocked(p *Player) bool {
	if p.Swing.Ammo == 0 || p.Swing.AmmoLeft > 0 {
		return true
	}
	log.Printf("Combat: %s has no ammunition (kind %d) for the shot: refused", p.Name, p.Swing.Ammo)
	return false
}

// spendAmmoLocked takes one piece of p's ammunition for a shot made: the
// swing's count now, the pack's through the hook. Must be called with the
// lock held.
func (w *World) spendAmmoLocked(sess *session.Session, p *Player) {
	if p.Swing.Ammo == 0 {
		return
	}
	p.Swing.AmmoLeft--
	if fn := w.onAmmoSpent; fn != nil && sess != nil {
		go fn(sess, p.CharServerID, p.Swing.Ammo)
	}
}
