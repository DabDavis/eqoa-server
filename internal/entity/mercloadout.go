package entity

import (
	"slices"

	"github.com/eqoa/server/internal/session"
)

// mercloadout.go — a merc's ten spell slots (the owner, 2026-11-15: "also
// merc spell menu should be like players also with spell book and 10
// spells"; handlers/mercwin.go is vanilla's window over it).
//
// A merc's kit carries the ten its hirer put on its slots (MercKit.Loadout,
// charmerc.spell_slots), by spell id, 0 an empty slot. With a loadout set the
// merc casts only what is on it: its fight book (mercBook: nukes, damage over
// time, taunts, heals, buffs), its pet's summoning (mercPetSpellWhere), its
// pet's heals and buffs (mercPetBook), its charm (mercharm.go) and an
// alchemist's thrown potions (mercthrow.go) each leave out a spell not
// slotted, in the order and by the worth they always had. With none set
// (nil) nothing is left out: its whole book, as before the window, and the
// window shows the ten the AI picks first (MercDefaultLoadout) until its
// hirer changes one. The menu's on and off (mercmenu.go, charmerc.spells_off)
// still holds beside it.

// slotted is whether the merc casts the spell by its slots: any while none
// are set.
func (ms *mercState) slotted(id int) bool {
	return ms.kit.Loadout == nil || slices.Contains(ms.kit.Loadout, id)
}

// MercBookSpell is a spell of a merc's book as its spell window lists it:
// every spell it may cast, of every kind.
type MercBookSpell struct {
	Spell CastSpell
	Use   string // what it casts it for (MercUseName; "pet", "pet buff", "charm", "potion" for the rest)
	Off   bool   // turned off in its menu (charmerc.spells_off)
}

// mercWindowBook is the merc's book as the window lists it: its fight book's
// (mercBook) in its worth's order, then its summonings, its pet's heals and
// buffs, its charms and its thrown potions, each spell once, as it casts
// them with nothing turned off nor left off its slots.
func (ms *mercState) mercWindowBook() []MercBookSpell {
	all := &mercState{kit: ms.kit, body: ms.body}
	all.kit.SpellsOff, all.kit.Loadout = nil, nil
	seen := map[int]bool{}
	var out []MercBookSpell
	add := func(s *CastSpell, use string) {
		if s == nil || seen[s.SpellID] {
			return
		}
		seen[s.SpellID] = true
		out = append(out, MercBookSpell{Spell: *s, Use: use, Off: slices.Contains(ms.kit.SpellsOff, s.SpellID)})
	}
	for _, sp := range all.mercBook() {
		add(sp.s, MercUseName(sp.s))
	}
	for i := range all.kit.Spells {
		s := &all.kit.Spells[i]
		if s.Def.Summons() && !s.Def.Charms() && s.Familiar == nil && s.Travel == nil {
			add(s, "pet")
		}
	}
	pb := all.mercPetBook()
	for _, s := range pb.heals {
		add(s, "pet heal")
	}
	for _, s := range pb.buffs {
		add(s, "pet buff")
	}
	for i := range all.kit.Spells {
		s := &all.kit.Spells[i]
		switch {
		case s.Def.Charms():
			add(s, "charm")
		case all.mercThrowsPotions() && mercPotionSpell(s):
			add(s, "potion")
		}
	}
	return out
}

// mercDefaultLoadout is the ten the merc's AI picks first, its loadout while
// none is set: the first of each use of its fight book (each turn takes the
// first of its kind it may cast: mercBook's order), the pet it summons
// (mercPetSpell), its pet's best heal and buffs, a charm, its potions, then the
// rest of its fight book by worth, up to ten, each once. Spells turned off in
// its menu are not picked, as the AI never casts them.
func (ms *mercState) mercDefaultLoadout() []int {
	all := &mercState{kit: ms.kit, body: ms.body}
	all.kit.Loadout = nil
	var out []int
	add := func(s *CastSpell) {
		if s != nil && len(out) < MercLoadoutSlots && !slices.Contains(out, s.SpellID) && !slices.Contains(all.kit.SpellsOff, s.SpellID) {
			out = append(out, s.SpellID)
		}
	}
	book := all.mercBook()
	used := map[mercUse]bool{}
	for _, sp := range book {
		if !used[sp.use] {
			used[sp.use] = true
			add(sp.s)
		}
	}
	add(all.mercPetSpell())
	pb := all.mercPetBook()
	if len(pb.heals) > 0 {
		add(pb.heals[0])
	}
	for _, s := range pb.buffs {
		add(s)
	}
	for i := range all.kit.Spells {
		s := &all.kit.Spells[i]
		if s.Def.Charms() || all.mercThrowsPotions() && mercPotionSpell(s) {
			add(s)
		}
	}
	for _, sp := range book {
		add(sp.s)
	}
	full := make([]int, MercLoadoutSlots)
	copy(full, out)
	return full
}

// MercLoadoutSlots is a merc's spell slots: a player's ten.
const MercLoadoutSlots = 10

// MercDefaultLoadout is mercDefaultLoadout for a merc of the kit (its body
// made from it): the handlers' and the tests'.
func MercDefaultLoadout(kit MercKit) []int {
	ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
	return ms.mercDefaultLoadout()
}

// MercWindowBook is mercWindowBook for a merc of the kit.
func MercWindowBook(kit MercKit) []MercBookSpell {
	ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
	return ms.mercWindowBook()
}

// MercLoadout is the session's player's merc of the slot out: its window's
// book (mercWindowBook), the ten on its slots (its own, or its default while
// none is set: set false) and ok false when none of the slot is out.
func (w *World) MercLoadout(sess *session.Session, slot int) (book []MercBookSpell, slots []int, set, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot)
	if ms == nil {
		return nil, nil, false, false
	}
	book = ms.mercWindowBook()
	if ms.kit.Loadout != nil {
		slots = make([]int, MercLoadoutSlots)
		copy(slots, ms.kit.Loadout)
		return book, slots, true, true
	}
	return book, ms.mercDefaultLoadout(), false, true
}

// SetMercLoadout gives the session's player's merc of the slot its ten (nil:
// none set, its whole book); false when none of the slot is out.
func (w *World) SetMercLoadout(sess *session.Session, slot int, loadout []int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot)
	if ms == nil {
		return false
	}
	ms.kit.Loadout = slices.Clone(loadout)
	ms.book, ms.bookOf = nil, nil // made again by its slots
	ms.petBook, ms.petBookOf = nil, nil
	return true
}

// MercSheet is a merc's figures as its window shows them: a player's
// summary's.
type MercSheet struct {
	Name                       string
	Class, Race, Level         int
	HP, MaxHP, Power, MaxPower int
	AC                         int
	Stats                      [7]int // STR STA AGI DEX WIS INT CHA
	Out                        bool   // in the world: its figures now (its buffs among them)
}

// MercSheetOf is the merc of the slot's figures: its body in the world now
// when it is out, else a body made from kit (its row's).
func (w *World) MercSheetOf(sess *session.Session, slot int, kit MercKit) MercSheet {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if ms := w.mercAtLocked(w.sessionPlayerLocked(sess), slot); ms != nil {
		sh := sheetOf(ms.body, ms.kit)
		n := ms.npc
		sh.Name, sh.Level, sh.HP, sh.MaxHP, sh.Power, sh.MaxPower, sh.Out = n.Name, n.Level, n.HP, n.MaxHP, n.Power, n.MaxPower, true
		return sh
	}
	b := newMercBody(1, &kit)
	sh := sheetOf(b, kit)
	sh.HP, sh.Power = sh.MaxHP, sh.MaxPower
	return sh
}

// MercSheetFor is MercSheetOf's body made from the kit (a merc not out).
func MercSheetFor(kit MercKit) MercSheet {
	b := newMercBody(1, &kit)
	sh := sheetOf(b, kit)
	sh.HP, sh.Power = sh.MaxHP, sh.MaxPower
	return sh
}

func sheetOf(b *Player, k MercKit) MercSheet {
	sh := MercSheet{Name: k.Name, Class: k.Class, Race: k.Race, Level: max(k.Level, 1), MaxHP: b.MaxHPNow(), MaxPower: b.MaxPowerNow(), AC: b.StatNow(StatAC)}
	for i, st := range []Stat{StatStr, StatSta, StatAgi, StatDex, StatWis, StatInt, StatCha} {
		sh.Stats[i] = b.StatNow(st)
	}
	return sh
}
