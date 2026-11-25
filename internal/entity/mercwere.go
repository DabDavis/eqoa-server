package entity

import (
	"log"
	"time"
)

// mercwere.go — a merc in a were form (petition 3069, 2026-11-25: "it would be cool if we
// could pick were-forms for our mercs like we do their path and spells, it would be cool
// to have a pack of lv 60 wolves"; the owner approved).
//
// Its hirer gives it a form by /merc were (handlers/mercwere.go): one of the were forms
// (IsWereForm: the five lines' own and their Feral masteries') its hirer's own book holds,
// so a merc takes only what its hirer earned on the were line, kept in its row
// (charmerc.were). Its kit carries the form's spell (MercKit.Were), and each of its turns
// keeps it on (mercWereLocked): the spell lands on it as on a player who casts it, its
// effects at the merc's body's level (its stats, its maximum hit points), its body the
// form's (landIllusionLocked on its body: the model and size a player of its race and sex
// is shown in, and only a body the disc has, so a PS2 draws it as it draws a player's
// form), on until it is dropped, as a player's were form is (wereFormLasts). A death ends
// it with its other effects and its next life takes it again; another form given takes the
// old off; none given (or its hirer's book no longer holds it) takes it off. Every client
// sees its body on the entity channel (NPC.Body), and its group's members its effect among
// its buffs (groupeffects.go).

// mercWereLocked keeps the merc's were form on it, and no other (the file's doc). Must be
// called with the lock held.
func (w *World) mercWereLocked(ms *mercState, now time.Time) {
	n := ms.npc
	if n.Dead {
		return
	}
	s := ms.kit.Were
	has, dropped := false, false
	keep := n.Effects[:0]
	for _, e := range n.Effects {
		switch {
		case s != nil && e.SpellID == s.SpellID:
			has = true
		case IsWereForm(e.Name):
			dropped = true
			log.Printf("Merc: %s's %q leaves its were form %q", ms.owner.Name, n.Name, e.Name)
			continue
		}
		keep = append(keep, e)
	}
	for i := len(keep); i < len(n.Effects); i++ {
		n.Effects[i] = nil
	}
	n.Effects = keep
	if dropped {
		syncMercBody(ms)
	}
	if s == nil || has {
		return
	}
	cs := &castState{spell: *s, caster: n.ServerID, targets: []EntityID{n.ServerID}}
	ae := lasting(cs, ms.body, now)
	if ae == nil {
		if ms.wereTried != s.SpellID {
			ms.wereTried = s.SpellID
			log.Printf("Merc: %s's %q: its were form %q lasts no time: not taken", ms.owner.Name, n.Name, s.Name)
		}
		return
	}
	if cs.spell.Def.hasIllusion() || cs.spell.Def.Form != "" || cs.spell.Def.FormModel != 0 {
		w.landIllusionLocked(cs, ms.body, ae) // a player's body of the form, for its race and sex (illusion.go)
	}
	if over := putOnPetLocked(n, &cs.spell, ae); over != nil {
		if ms.wereTried != s.SpellID {
			ms.wereTried = s.SpellID
			log.Printf("Merc: %s's %q: its were form %q does not take over %q", ms.owner.Name, n.Name, s.Name, over.Name)
		}
		return
	}
	ms.wereTried = 0
	syncMercBody(ms)
	if s.FX != 0 {
		w.broadcastNPCCastLocked(n, n.ServerID, 0, s.FX)
	}
	log.Printf("Merc: %s's %q takes its were form %q (model %#x)", ms.owner.Name, n.Name, s.Name, ae.Form)
}
