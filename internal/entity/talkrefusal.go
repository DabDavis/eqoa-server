package entity

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The people of some factions will not speak to a player they hold in the red
// bands (Heirloom petition 3111, Packmaster, of the Isle of Dread as he played
// it: Graumord's citizens "will not speak to you at all until they become white
// face"): no talk at all (no town line, no talk tree, no quest offer, step or
// hand-in) and no bank or blacksmith window, one refusal line in their place
// (handlers/talkrefusal.go). Which factions do is a table, factionrefusetalk,
// not a rule for every faction: measured 2026-11-26, a rule for all would
// have every character on both shards refused by a median of 467 NPC-bit
// NPCs (the opposite cities' people), so a faction takes it only when a GM
// lists it. Its members are every NPC of it (npcfaction), with the NPC bit or
// not: Graumord's quests are given by any "Graumord citizen", and fourteen of
// its twenty are attackable. A merchant or coachman of any faction refuses
// the red bands already (Refuses, board #742).

// DefaultTalkRefusal is a refusing NPC's line when its faction's row has none
// of its own (ours, in the game's narration; %n the NPC, %p the player).
const DefaultTalkRefusal = "%n regards you coldly and will not speak with you."

// SetFactionTalkRefusals sets which factions' people will not speak to the
// red bands (the factionrefusetalk table): at the start and when a GM edits it.
// It reports the rows kept and those naming no faction.
func (w *World) SetFactionTalkRefusals(rows []database.FactionTalkRefusalRow) (kept, unknown int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		if _, ok := w.factions.byName[r.Faction]; !ok {
			unknown++
			log.Printf("Factions: factionrefusetalk names %q, which is no faction: no one refuses for it", r.Faction)
			continue
		}
		m[r.Faction] = r.Line
		kept++
	}
	w.factions.talkRefuse = m
	return kept, unknown
}

// RefusesTalk reports whether npc will not speak to the session's player: a
// living NPC of a faction listed in factionrefusetalk whose standing with it
// is at or below FactionKOSAt (refusesLocked: not the player's own genie, and
// nobody with world.faction_aggro off). line is the faction's refusal line,
// "" for DefaultTalkRefusal. A GM is the caller's to exempt.
func (w *World) RefusesTalk(sess *session.Session, npc *NPC) (faction, line string, refuses bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if npc == nil || npc.Dead || len(w.factions.talkRefuse) == 0 {
		return "", "", false
	}
	f := w.npcFactionLocked(npc)
	line, listed := w.factions.talkRefuse[f]
	if f == "" || !listed || !w.refusesLocked(w.sessionPlayerLocked(sess), npc) {
		return "", "", false
	}
	return f, line, true
}
