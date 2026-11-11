package entity

import (
	"sort"
	"time"
)

// mercpetbuff.go — a merc keeps its own pet buffed and healed (the owner,
// 2026-11-11: "for merc casting pets, also cause them to cast the best buffs
// on the pet. like for 60 nec largesse of plague, soul mastery, the ac
// buff").
//
// A pet's spell is one of scope 3 (VIAbility's "pet": cast.go's spellTargets
// puts it on its caster's pet whatever is targeted). Of those in the merc's
// book (its kit: what its levels give a player of its class, no mastery's or
// epic's), it casts:
//
//   - a pet heal (mercPetHeals: an instant heal, no time: the magician's
//     Infuse to Fusion) when its pet is below mercHurtPct of its hit points,
//     the biggest that is ready;
//   - its pet buffs (mercPetBuffs: what lasts and only helps: stats up, a
//     damage shield, a proc, a heal over time; no run speed, no
//     invisibility, no form, nothing that hurts the pet, such as the
//     alchemist's Destroy Contraption), the best of each line: the highest
//     level, then the longest, then the strongest (the sum of its figures),
//     then the highest id. A pet's spells read from EQOAEmu's workbook carry
//     no stack type, so BuffLine gives them no line and nothing on the pet
//     would ever refuse a second of them; their line here is what they
//     change (petBuffKeys: each stat raised, a shield, a proc), two spells
//     that share any of it one line. A level 60 necromancer's are Soul
//     Mastery (STR, STA, DEX, a heal over time: Reinforce Bone to Death
//     Mastery are its lower ranks), Strengthen Bone (AGI, AC) and Largesse
//     of Plague (a proc); a magician's Agitate, Elemental Mark and the like.
//     Each is cast when it is not on the pet (worn off, or a pet summoned
//     again), each at its own cast time, power and recast, as a player casts
//     it (mercCastLocked), and lands as a player's cast on its pet
//     (landOnPetLocked).
//
// The order of a merc's turn (merccast.go's mercCastTurnLocked): its team's
// heals first, then its pet's summoning, then its pet's heal, then its pet's
// buffs, then the rest (its own buffs out of a fight, a tank's taunt, its
// attacks). A pet's buffs come before the fight, in a fight too: the pet
// fights better for them, and a merc in a fight would otherwise never renew
// one that wore off. Shamans' and enchanters' books here have no spell of
// scope 3 (their buffs reach a pet as a group's: enchanter.go), so only
// necromancers' and magicians' mercs cast any.

// mercPetSpells is the pet's spells of the merc's book as it casts it.
type mercPetSpells struct {
	heals []*CastSpell // the biggest first
	buffs []*CastSpell // the best of each line, the highest level first
}

// isPetHeal is a pet's instant heal: scope 3, every effect a heal of it, no
// time.
func isPetHeal(s *CastSpell) bool {
	d := &s.Def
	if s.Scope != 3 || d.Harmful || d.lasts() || len(d.Effects) == 0 {
		return false
	}
	for _, e := range d.Effects {
		if e.Kind != EffectHP || e.Amount <= 0 || e.Self {
			return false
		}
	}
	return true
}

// petBuffKeys is a pet buff's line keys (the file's doc), none for a spell of
// scope 3 that is no pet buff a merc casts.
func petBuffKeys(s *CastSpell) []string {
	d := &s.Def
	if s.Scope != 3 || d.Harmful || d.Duration <= 0 || d.Summons() || d.Charms() || d.FormKind != FormNone || d.AltersLook ||
		d.Reagent != "" || len(d.Items) > 0 || s.Travel != nil || s.Familiar != nil {
		return nil
	}
	seen := map[string]bool{}
	var keys []string
	for _, e := range d.Effects {
		if e.Self {
			continue
		}
		var k string
		switch e.Kind {
		case EffectStat:
			if e.Amount <= 0 {
				continue
			}
			k = "+" + statLineNames[min(max(int(e.Stat), 0), len(statLineNames)-1)]
		case EffectShield:
			k = "shield"
		case EffectDefProc:
			k = "defproc"
		case EffectProc:
			k = "proc"
		case EffectMitigate:
			k = "mitigate"
		case EffectProcHeal:
			k = "procheal"
		case EffectHPTick:
			if e.Amount <= 0 {
				return nil // it hurts the pet
			}
			k = "hot"
		case EffectHP:
			if e.Amount < 0 {
				return nil // it hurts the pet (Destroy Contraption)
			}
			continue
		case EffectSpeed, EffectInvis, EffectHide, EffectIllusion, EffectSize, EffectSummon, EffectCharm, EffectGate:
			return nil
		default:
			continue
		}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// petBuffStrength is a pet buff's figures summed: the tie-break between two
// of one level and time.
func petBuffStrength(s *CastSpell) int {
	n := 0
	for _, e := range s.Def.Effects {
		n += max(e.Amount, -e.Amount)
	}
	return n
}

// petBuffBetter is whether a is the better of one line (the file's doc).
func petBuffBetter(a, b *CastSpell) bool {
	if a.Level != b.Level {
		return a.Level > b.Level
	}
	if a.Def.Duration != b.Def.Duration {
		return a.Def.Duration > b.Def.Duration
	}
	if sa, sb := petBuffStrength(a), petBuffStrength(b); sa != sb {
		return sa > sb
	}
	return a.SpellID > b.SpellID
}

// mercPetBook is the merc's pet spells (the file's doc), made with its book
// (merccast.go's mercBook: again with each kit).
func (ms *mercState) mercPetBook() mercPetSpells {
	if ms.petBook != nil && ms.petBookOf == ms.body {
		return *ms.petBook
	}
	var out mercPetSpells
	type line struct {
		keys map[string]bool
		best *CastSpell
	}
	var lines []*line
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if isPetHeal(s) {
			out.heals = append(out.heals, s)
			continue
		}
		keys := petBuffKeys(s)
		if len(keys) == 0 {
			continue
		}
		// the lines it shares a key with become one (a rank between two)
		var into *line
		kept := lines[:0]
		for _, l := range lines {
			shares := false
			for _, k := range keys {
				shares = shares || l.keys[k]
			}
			if !shares {
				kept = append(kept, l)
				continue
			}
			if into == nil {
				into = l
				kept = append(kept, l)
				continue
			}
			for k := range l.keys {
				into.keys[k] = true
			}
			if petBuffBetter(l.best, into.best) {
				into.best = l.best
			}
		}
		lines = kept
		if into == nil {
			into = &line{keys: map[string]bool{}, best: s}
			lines = append(lines, into)
		}
		for _, k := range keys {
			into.keys[k] = true
		}
		if petBuffBetter(s, into.best) {
			into.best = s
		}
	}
	for _, l := range lines {
		out.buffs = append(out.buffs, l.best)
	}
	sort.SliceStable(out.buffs, func(i, j int) bool { return petBuffBetter(out.buffs[i], out.buffs[j]) })
	sort.SliceStable(out.heals, func(i, j int) bool { return petBuffStrength(out.heals[i]) > petBuffStrength(out.heals[j]) })
	ms.petBook, ms.petBookOf = &out, ms.body
	return out
}

// mercPetCastOnWayLocked is whether the merc has a cast of s on its way.
// Must be called with the lock held.
func (w *World) mercPetCastOnWayLocked(ms *mercState, s *CastSpell) bool {
	for _, c := range w.mercCasts {
		if c.ms == ms && c.life == ms.life && c.spell.SpellID == s.SpellID {
			return true
		}
	}
	return false
}

// mercPetCareTurnLocked is the merc's pet's heal or buff this turn, if any
// (the file's doc). It reports a cast begun. Must be called with the lock
// held.
func (w *World) mercPetCareTurnLocked(ms *mercState, now time.Time) bool {
	ps := w.mercPetOutLocked(ms)
	if ps == nil {
		return false
	}
	pet, m := ps.npc, ms.npc
	if pet.World != m.World {
		return false
	}
	book := ms.mercPetBook()
	if len(book.heals) == 0 && len(book.buffs) == 0 {
		return false
	}
	ok := func(s *CastSpell) bool {
		return ms.mercReady(s, now) && mercReaches(m, s, pet.X, pet.Y, pet.Z, 0) && !w.mercPetCastOnWayLocked(ms, s)
	}
	healing := false // one heal on its way at a time
	for _, s := range book.heals {
		healing = healing || w.mercPetCastOnWayLocked(ms, s)
	}
	if !healing && pet.MaxHP > 0 && pet.HP*100 < pet.MaxHP*mercHurtPct {
		for _, s := range book.heals {
			if ok(s) {
				w.mercCastLocked(ms, s, pet.ServerID, false, now)
				return true
			}
		}
	}
	for _, s := range book.buffs {
		if !buffOn(pet.Effects, s) && ok(s) {
			w.mercCastLocked(ms, s, pet.ServerID, false, now)
			return true
		}
	}
	return false
}

// mercPetLandLocked lands a merc's spell on its own pet as a player's cast on
// its pet lands (pet.go's landOnPetLocked), its body the caster; nothing
// when the pet it was cast at is gone. It reports whether id was its pet.
// Must be called with the lock held.
func (w *World) mercPetLandLocked(ms *mercState, cs *castState, id EntityID, now time.Time) bool {
	ps := ms.pet
	if ps == nil || ps.npc.ServerID != id {
		return false
	}
	if pet := ps.npc; !pet.Dead && w.NPCs[id] == pet {
		w.landOnPetLocked(cs, ms.body, nil, pet, now)
	}
	return true
}
