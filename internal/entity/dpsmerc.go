package entity

import "sort"

// dpsmerc.go — a merc's own line on the damage meter (Heirloom petition 3134,
// Cucumber: "Mercenary DPS, HPS seem to fall under a broad 'Pet' category on
// the DPS meter. Recommend splitting them into the mercenary names. Also PPS
// seems to not work from mercenary actions on the meter"; the owner: build
// both).
//
// A merc's damage, healing and power given count, as they did, on its owner's
// line as its helper's share (layouts 1 and 2 are sent byte for byte as
// before), and on a line of its own in the fight too (dpsFight.mercs: its name
// and class, its owner). Layout 3 sends each merc's line apart, flagged 4 (8
// when it is the receiver's own merc) and followed by its owner's name, and its
// owner's line without it: the fight's wholes are the same. A summoned or a
// charmed pet stays its owner's pet share.
//
// Which merc made a figure: the ones counted with no figure name their maker
// (a merc's power given, dpsPowerLocked's giver; a merc's heal on a player,
// healLocked's caster, its body); a figure told its owner as its merc's
// (numberByMerc) is the merc's whose action tells it (dpsByLocked round its
// hits, its beneficial casts' landing, its heals over time's ticks), else its
// owner's only merc out; with none known it stays the owner's merc share alone.
//
// The power the petition missed: a necromancer merc's Power path (Power Gale
// to Power Vortex, cast at its foe as a hit since petition 2302) gave nobody
// any power, its "Drain Group" power effect unread by mercHarmLocked, so its
// owner's meter had no power of it at all. It now gives its team the power a
// player's cast does (mercDrainPowerLocked), counted as the merc's.
//
// A merc's own pet (mercpet.go) is its merc's (Heirloom petition 3252,
// Cucumber: "My Power Scholar NEC Merc did not report anything in the DPS
// meters"): its hits, its damage over time's ticks, its procs and abilities,
// its heals and the power it gives count on its merc's line as that line's pet
// share (dpsByPetLocked round its told figures, dpsPetAddLocked for the ones
// counted with no figure), and so on its owner's line as its merc share, not
// as the owner's own pet's. A necromancer merc's Swarming Death made seven
// times its merc's own damage in the fight the petition named, all of it on
// its hirer's line as a pet of his. And a merc that dies and comes back in a
// fight (another NPC, another id) goes on on its one line, by its owner, its
// slot and its name: it was split in two lines of the same name.

// dpsMercLine is a merc's own line of a fight: its line (pet and healPet its
// own pet's share), its owner's id and name, and its hire's slot.
type dpsMercLine struct {
	dpsLine
	owner     EntityID
	ownerName string
	slot      int
}

// add counts n of what on the line.
func (l *dpsLine) add(what dpsWhat, n int) {
	switch what {
	case dpsHeal:
		l.heal += int64(n)
	case dpsPower:
		l.power += int64(n)
	default:
		l.damage += int64(n)
	}
}

// addPet counts n of what on the line as its pet's share too (a merc's own
// pet's: dpsmerc.go's doc).
func (l *dpsLine) addPet(what dpsWhat, n int) {
	switch what {
	case dpsHeal:
		l.healPet += int64(n)
	case dpsDamage:
		l.pet += int64(n)
	}
}

// dpsBySetLocked makes merc (a merc's id; any other id is never one) the maker
// of the figures told until the func it returns is run. Must be called with
// the lock held.
func (w *World) dpsBySetLocked(merc EntityID) func() {
	was := w.dpsBy
	w.dpsBy = merc
	return func() { w.dpsBy = was }
}

// dpsByLocked runs tell, figures merc (a merc's id) made. Must be called with
// the lock held.
func (w *World) dpsByLocked(merc EntityID, tell func()) {
	defer w.dpsBySetLocked(merc)()
	tell()
}

// dpsMercOfPet is the id of the merc whose own pet ps is (mercpet.go), 0 for
// a player's pet.
func dpsMercOfPet(ps *petState) EntityID {
	if ps != nil && ps.merc != nil && ps.merc.npc != nil {
		return ps.merc.npc.ServerID
	}
	return 0
}

// dpsByPetLocked runs tell, figures ps (a pet) made: a merc's own pet's count
// on that merc's line as its pet's share (dpsFigureMercLocked); a player's
// pet's are its owner's pet's as before. Must be called with the lock held.
func (w *World) dpsByPetLocked(ps *petState, tell func()) {
	was := w.dpsByPet
	w.dpsByPet = dpsMercOfPet(ps)
	defer func() { w.dpsByPet = was }()
	tell()
}

// dpsPetAddLocked counts n of what of ps's (a pet's) making, with no figure
// told: its owner's pet's share, and when ps is a merc's own pet that merc's
// too, on its line as its pet's share. Must be called with the lock held.
func (w *World) dpsPetAddLocked(ps *petState, what dpsWhat, n int) {
	if ps == nil {
		return
	}
	merc := w.dpsMercOfLocked(ps.owner, dpsMercOfPet(ps))
	w.dpsAddByLocked(ps.owner, what, n, numberByPet, merc, dpsMeterNow())
}

// dpsMercOfLocked is the merc of maker's that id names (a merc's id), 0 when
// it names none of maker's. Must be called with the lock held.
func (w *World) dpsMercOfLocked(maker *Player, id EntityID) EntityID {
	if maker == nil || id == 0 {
		return 0
	}
	if n := w.NPCs[id]; n.isMerc() && n.merc.owner != nil && n.merc.owner.ServerID == maker.ServerID {
		return id
	}
	return 0
}

// dpsFigureMercLocked is the merc of maker's that made a figure of kind told to
// maker: none when kind is no merc's; the one whose action tells it
// (dpsByLocked), else maker's only merc out, else none. Must be called with the
// lock held.
func (w *World) dpsFigureMercLocked(maker *Player, kind numberKind) EntityID {
	if maker == nil {
		return 0
	}
	if kind&numberByMerc == 0 {
		if kind&numberByPet != 0 {
			return w.dpsMercOfLocked(maker, w.dpsByPet) // a merc's own pet's (dpsByPetLocked)
		}
		return 0
	}
	if id := w.dpsMercOfLocked(maker, w.dpsBy); id != 0 {
		return id
	}
	if ms := w.mercsOfLocked(maker); len(ms) == 1 {
		return ms[0].npc.ServerID
	}
	return 0
}

// dpsMercLineLocked is merc's own line in fight f, made at its first count:
// its name and class as they are then, maker its owner. Must be called with
// the lock held.
func (w *World) dpsMercLineLocked(f *dpsFight, maker *Player, merc EntityID) *dpsLine {
	if l := f.mercs[merc]; l != nil {
		return &l.dpsLine
	}
	l := &dpsMercLine{owner: maker.ServerID, ownerName: maker.Name}
	if n := w.NPCs[merc]; n != nil {
		l.name, l.class = n.Name, n.Class
		if n.merc != nil {
			l.class, l.slot = n.merc.kit.Class, n.merc.slot
		}
	}
	// the same merc come back in the fight (dead and risen, or called again)
	// under another id: its line goes on, one line a merc
	if l.slot != 0 {
		for _, o := range f.mercs {
			if o.owner == l.owner && o.slot == l.slot && o.name == l.name {
				return &o.dpsLine
			}
		}
	}
	if f.mercs == nil {
		f.mercs = map[EntityID]*dpsMercLine{}
	}
	f.mercs[merc] = l
	return &l.dpsLine
}

// dpsSplitMercs is rows (dpsRowsLocked's, of fight f as p is sent it) with each
// merc's line apart (layout 3): its owner's line less the merc's, and the
// merc's own, flagged 4 (and 8 when it is p's), 2 with its owner's, its owner
// named; in the order of damage again. The wholes are unchanged.
func dpsSplitMercs(rows []dpsRow, f *dpsFight, p *Player) []dpsRow {
	if len(f.mercs) == 0 {
		return rows
	}
	at := map[EntityID]int{}
	for i, r := range rows {
		at[r.id] = i
	}
	for id, ml := range f.mercs {
		i, ok := at[ml.owner]
		if !ok {
			continue // never: an owner's line has its merc's counts
		}
		o := &rows[i].line
		o.damage -= ml.damage
		o.pet = max(o.pet-ml.damage, 0)
		o.heal -= ml.heal
		o.healPet = max(o.healPet-ml.heal, 0)
		o.power -= ml.power
		flags := byte(4) | rows[i].flags&2
		if ml.owner == p.ServerID {
			flags |= 8
		}
		rows = append(rows, dpsRow{id: id, line: ml.dpsLine, flags: flags, owner: ml.ownerName})
	}
	sortDPSRows(rows)
	return rows
}

// dpsMercRows puts each merc's own line of f on its owner's row (layout 5:
// rows[i].mercs, the most damage first), flagged as dpsSplitMercs flags it; the
// owner's row keeps its merc shares (layout 4's). Must be called with the lock
// held.
func dpsMercRows(rows []dpsRow, f *dpsFight, p *Player) {
	if len(f.mercs) == 0 {
		return
	}
	at := map[EntityID]int{}
	for i, r := range rows {
		at[r.id] = i
	}
	for id, ml := range f.mercs {
		i, ok := at[ml.owner]
		if !ok {
			continue
		}
		flags := byte(4) | rows[i].flags&2
		if ml.owner == p.ServerID {
			flags |= 8
		}
		rows[i].mercs = append(rows[i].mercs, dpsRow{id: id, line: ml.dpsLine, flags: flags, owner: ml.ownerName})
	}
	for i := range rows {
		sortDPSRows(rows[i].mercs)
	}
}

// sortDPSRows orders rows as a message sends them: the most damage first, then
// by name, then by id.
func sortDPSRows(rows []dpsRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.line.damage != b.line.damage {
			return a.line.damage > b.line.damage
		}
		if a.line.name != b.line.name {
			return a.line.name < b.line.name
		}
		return a.id < b.id
	})
}

// mercDrainHPLocked gives the hit points a merc's tap took (n, its share) to
// the merc, and for a "Drain Group" one (the necromancer's Blood path: Blood
// Gale to Blood Vortex) to its team in the spell's reach and sight too
// (mercHealTeamLocked, as its group's buffs), as a player's cast heals its
// group (lifetap.go drainLocked; the owner, 2026-11-26, Heirloom petition 3134's
// follow-up: it healed the merc alone). Each heal counts on the meter as the
// merc's healing (its line and its owner's merc share), at what it restored.
// Must be called with the lock held.
func (w *World) mercDrainHPLocked(ms *mercState, s *CastSpell, e SpellEffect, n int) {
	m := ms.npc
	if n <= 0 || m.Dead {
		return
	}
	defer w.dpsBySetLocked(m.ServerID)()
	kind := numberHeal.byMerc()
	healNPC := func(o *NPC) {
		if o.Dead || o.HP >= o.MaxHP {
			return
		}
		was := o.HP
		o.HP = min(o.HP+n, o.MaxHP)
		if o.isMerc() {
			syncMercBody(o.merc)
		}
		w.dpsAddByLocked(ms.owner, dpsHeal, o.HP-was, numberByMerc, m.ServerID, dpsMeterNow())
	}
	if e.Drain != DrainGroup {
		healNPC(m)
		return
	}
	for _, mm := range w.mercHealTeamLocked(ms) {
		if mm.id() != m.ServerID && !w.mercGroupReachesLocked(ms, s, mm) {
			continue
		}
		if p := mm.p; p != nil {
			if !p.IsDead() {
				w.healLocked(ms.body, nil, p, n, kind) // its owner's merc's figure; the meter's merc line (healLocked)
			}
			continue
		}
		healNPC(mm.n)
	}
}

// mercDrainPowerLocked gives the power a merc's Power path spell drained (n,
// the effect's "Drain Group" power) to the merc and its team in the spell's
// reach and sight (mercHealTeamLocked, as its group's buffs), each within its
// most, as a player's cast gives its group (lifetap.go drainLocked): counted
// on the meter as the merc's power given (none its own). Must be called with
// the lock held.
func (w *World) mercDrainPowerLocked(ms *mercState, s *CastSpell, n int) {
	if n <= 0 || ms.npc.Dead {
		return
	}
	m := ms.npc
	for _, mm := range w.mercHealTeamLocked(ms) {
		if mm.id() != m.ServerID && !w.mercGroupReachesLocked(ms, s, mm) {
			continue
		}
		if p := mm.p; p != nil {
			was := p.Power
			p.Power = min(p.Power+n, p.MaxPowerNow())
			p.StatsDirty = true
			w.dpsPowerLocked(m.ServerID, p.ServerID, p.Power-was)
			continue
		}
		o := mm.n
		if o.MaxPower <= 0 {
			continue
		}
		was := o.Power
		o.Power = min(o.Power+n, o.MaxPower)
		if o.isMerc() {
			syncMercBody(o.merc)
		}
		w.dpsPowerLocked(m.ServerID, o.ServerID, o.Power-was)
	}
}
