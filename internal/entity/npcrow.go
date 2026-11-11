package entity

import (
	"strconv"
	"strings"
)

// An NPC's own row where a table keys NPCs by name (the Cazic-Thule trials,
// 2026-11-11; work board 430's code ask 4, 637's ask 1, 640's ask 1): the Isle
// of Dread's Dread and Terror share their names with seven L33 dreads and five
// L41-45 terrors on the mainland, and the trial's catacomb copies of Contagion,
// Festering Jack and Zin share theirs with the outdoor named, so a table keyed
// by name (npctemplate, npcspawnonly) and a spawn that copies an NPC by name
// (a quest's, a GM trigger's, #spawn) could not tell them apart. A key "#n"
// names the npcs row n instead: npctemplate's "#118889" gives that row (and
// every copy of it) its template before its name's; npcspawnonly's "#118877"
// keeps that row alone out of the world; a spawn of "#118877" copies that row.

// RowKey is the key that names the npcs row id in a table keyed by name.
func RowKey(id EntityID) string { return "#" + strconv.FormatUint(uint64(id), 10) }

// ParseRowKey reads a RowKey ("#118877", spaces aside); false for a name.
func ParseRowKey(s string) (EntityID, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '#' {
		return 0, false
	}
	n, err := strconv.ParseUint(s[1:], 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	return EntityID(n), true
}

// row is the npcs row the NPC came from: its own id, or for a copy (a spawn's,
// SpawnNPCCopy) the row it was copied from; 0 for a copy of no row.
func (n *NPC) row() EntityID {
	if n.Temporary {
		return n.CopyOf
	}
	return n.ServerID
}
