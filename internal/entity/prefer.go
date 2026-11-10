package entity

import "strings"

// prefer.go — the foes a creature fights first (database.AbilityPrefer; the GM Jibrish's petition 1438: "If
// the monster Tunare is attacking Mr. Moogie / on his aggro list, he should prioritize her over all other
// mobs first").
//
// Whom a creature fights of its hate list (its threats: what hit it, angered it or it noticed) was always
// its highest threat, chosen again at every hate added (addThreatLocked) and whenever one leaves the list
// (a death, a forgetting, a pet let go). A template's prefer names foes (any case, the first first):
// whenever one of them is on its hate list, alive and in its world, a player or an NPC it can fight, it
// fights that one, whatever the rest's hate, the first named before the second and, of two of one name,
// the higher hate; a taunt does not turn it from one. With none of them on its list it chooses as ever.
// pickEnemyLocked is that choice, every place a creature's enemy is chosen from its list.

// pickEnemyLocked is whom npc fights of its hate list (the file's doc): the first of its preferred foes on
// it, else its highest threat (the first of equal ones met); 0 for none above 0. Must be called with the
// lock held.
func (w *World) pickEnemyLocked(npc *NPC) EntityID {
	if id := w.preferredFoeLocked(npc); id != 0 {
		return id
	}
	var top int64
	var out EntityID
	for id, t := range npc.Threats {
		if t > top {
			top, out = t, id
		}
	}
	return out
}

// preferredFoeLocked is the first of npc's template's preferred foes on its hate list that it can fight:
// of the first name with one there, the highest threat (the lower id of equal ones); 0 for none. Must be
// called with the lock held.
func (w *World) preferredFoeLocked(npc *NPC) EntityID {
	t := npc.tmpl
	if t == nil || len(t.prefer) == 0 || len(npc.Threats) == 0 {
		return 0
	}
	for _, name := range t.prefer {
		var best EntityID
		var top int64
		for id, th := range npc.Threats {
			if th <= 0 || (best != 0 && (th < top || th == top && id > best)) || !w.preferableLocked(npc, id, name) {
				continue
			}
			best, top = id, th
		}
		if best != 0 {
			return best
		}
	}
	return 0
}

// preferableLocked reports id one npc can fight named name (lower case): a living player in its world, or
// a living NPC there (a pet, or a creature it may fight as aiSystem fights one: no dummy). Must be called
// with the lock held.
func (w *World) preferableLocked(npc *NPC, id EntityID, name string) bool {
	if p := w.Players[id]; p != nil {
		return !p.IsDead() && p.World == npc.World && strings.EqualFold(strings.TrimSpace(p.Name), name)
	}
	n := w.NPCs[id]
	return n != nil && n != npc && !n.Dead && n.World == npc.World && (n.Owner != 0 || n.Dummy == NotDummy) &&
		strings.EqualFold(strings.TrimSpace(n.Name), name)
}

// tauntedLocked is whom a taunt by taunter leaves t fighting: a preferred foe of its on its list before
// the taunter (the file's doc), else the taunter. Must be called with the lock held.
func (w *World) tauntedLocked(t *NPC, taunter EntityID) EntityID {
	if id := w.preferredFoeLocked(t); id != 0 {
		return id
	}
	return taunter
}

// preferWords is #stats' words for a template's preferred foes, "" for none.
func preferWords(t *ClassTemplate) string {
	if t == nil || len(t.prefer) == 0 {
		return ""
	}
	return "fights first: " + strings.Join(t.prefer, ", ")
}
