package entity

// dpstaken.go — the damage each line took, and a row for every merc out
// (Heirloom petition 3397: "Adding Damage Taken Per Second to the DPS meter so
// tanks can see if adjustments reduce their damage taken ... DPS meter lines
// should match the group/raid count including mercs"; the owner: yes).
//
// A player's damage taken is what it is told it took: each figure of damage
// sent to its own client on itself (sendDamageIndication: a creature's hit or
// spell, a damage over time's tick, the sun, drowning, lava, a duel's hit), as
// the meter's damage is what it is told it did. A merc's is the hit points it
// lost (dpsTakeHPLocked, at each place a creature's hit, spell, shield, proc or
// tick takes them off a pet or a merc: no figure of those is told), counted on
// its own line and on its owner's as its merc share. A summoned or a charmed
// pet's is not counted. Damage taken counts only into a fight under way, as
// healing and power do: it neither starts a fight nor lengthens one, so the
// sun on a vampire or a fall alone is no fight.
//
// Layout 6 (dpsMeterLayoutTaken) sends it, and sends a merc row for every merc
// out of each member in the group (dpsIdleMercRowsLocked), of nothing too, as
// every member already has a line: a cleric merc that has healed no one yet is
// on the meter at 0.

// dpsTakenFigureLocked counts n of damage told to sess's player on itself
// (sendDamageIndication). Must be called with the lock held.
func (w *World) dpsTakenFigureLocked(p *Player, n int) {
	if p == nil || n <= 0 {
		return
	}
	w.dpsAddByLocked(p, dpsTaken, n, 0, 0, dpsMeterNow())
}

// dpsTakeHPLocked is t's hit points after n of damage, counting what it lost
// as damage taken when t is a merc (on its line and its owner's merc share).
// Must be called with the lock held.
func (w *World) dpsTakeHPLocked(t *NPC, n int) int {
	hp := max(t.HP-n, 0)
	if lost := t.HP - hp; lost > 0 && t.isMerc() && t.merc.owner != nil {
		w.dpsAddByLocked(t.merc.owner, dpsTaken, lost, numberByMerc, t.ServerID, dpsMeterNow())
	}
	return hp
}

// dpsIdleMercRowsLocked puts on each member's row of rows (layout 6, after
// dpsMercRows) a row of nothing for each of its mercs out that has no line in
// the fight yet, flagged as dpsMercRows flags a merc's, so every merc in the
// group is on the meter. Must be called with the lock held.
func (w *World) dpsIdleMercRowsLocked(rows []dpsRow, p *Player) {
	for i := range rows {
		r := &rows[i]
		o := w.Players[r.id]
		if r.id == p.ServerID {
			o = p
		}
		if o == nil || r.flags&2 != 0 {
			continue
		}
		added := false
		for _, ms := range w.mercsOfLocked(o) {
			if ms.npc == nil || dpsHasMercRow(r.mercs, ms) {
				continue
			}
			flags := byte(4)
			if o.ServerID == p.ServerID {
				flags |= 8
			}
			r.mercs = append(r.mercs, dpsRow{id: ms.npc.ServerID, flags: flags, owner: o.Name,
				line: dpsLine{name: ms.npc.Name, class: ms.kit.Class}})
			added = true
		}
		if added {
			sortDPSRows(r.mercs)
		}
	}
}

// dpsHasMercRow is whether mercs has ms's row: its id's, or the same merc's
// under an earlier id (dead and risen in the fight: dpsMercLineLocked's one
// line a merc, by its name).
func dpsHasMercRow(mercs []dpsRow, ms *mercState) bool {
	for _, m := range mercs {
		if m.id == ms.npc.ServerID || m.line.name == ms.npc.Name {
			return true
		}
	}
	return false
}
