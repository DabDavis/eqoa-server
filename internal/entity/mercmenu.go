package entity

import (
	"slices"

	"github.com/eqoa/server/internal/session"
)

// mercmenu.go — the World's half of a merc's own menu (handlers/mercmenu.go;
// the owner, 2026-11-12: "let players press Square to open a dialog with
// their mercs"): which of its hirer's mercs a talk names, its spells as it
// casts them (each on or off), and its spells turned off.

// MercOwnSlot is the slot of the session's player's merc whose entity id is
// id; ok false when it is not one of that player's mercs (another's, or no
// merc).
func (w *World) MercOwnSlot(sess *session.Session, id EntityID) (slot int, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, ms := range w.mercsOfLocked(w.sessionPlayerLocked(sess)) {
		if ms.npc.ServerID == id {
			return ms.slot, true
		}
	}
	return 0, false
}

// MercSpell is a spell of a merc's book as its menu lists it.
type MercSpell struct {
	ID   int
	Name string
	Use  string // what it casts it for (MercUseName)
	Off  bool   // turned off by its hirer
}

// MercSpells is the spells the session's player's merc of the slot casts
// (mercBook with none turned off: each line's best, nothing far beneath its
// level), the highest level first, each marked off or on; false when no merc
// of the slot is out.
func (w *World) MercSpells(sess *session.Session, slot int) ([]MercSpell, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot)
	if ms == nil {
		return nil, false
	}
	all := &mercState{kit: ms.kit, body: ms.body}
	all.kit.SpellsOff = nil
	book := all.mercBook()
	out := make([]MercSpell, 0, len(book))
	for _, sp := range book {
		out = append(out, MercSpell{ID: sp.s.SpellID, Name: sp.s.Name, Use: MercUseName(sp.s), Off: slices.Contains(ms.kit.SpellsOff, sp.s.SpellID)})
	}
	levels := map[int]int{}
	for _, sp := range book {
		levels[sp.s.SpellID] = sp.s.Level
	}
	slices.SortStableFunc(out, func(a, b MercSpell) int { return levels[b.ID] - levels[a.ID] })
	return out, true
}

// SetMercSpellsOff gives the session's player's merc of the slot the spells
// it keeps out of its book; false when none of the slot is out.
func (w *World) SetMercSpellsOff(sess *session.Session, slot int, off []int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot)
	if ms == nil {
		return false
	}
	ms.kit.SpellsOff = slices.Clone(off)
	ms.book, ms.bookOf = nil, nil // made again without them
	return true
}
