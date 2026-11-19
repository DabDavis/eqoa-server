package entity

import "time"

// taunthold.go — a taunt holds its creature (the owner, 2026-11-18: "taunt is not keeping hate on
// the tank enough").
//
// What the logs and the code showed: a creature fights its highest threat, chosen again at every
// point of hate anyone adds (addThreatLocked), with no margin to pass and no decay. A taunt
// (taunt.go) puts its taunter at the top plus a margin, the larger of the taunt's hate (Goad's
// ~1,050, 105% for a tank) and a pet's margin at his level (pettauntmargin.go: 1,200 at 60, or 10%
// of the top). Early in a fight, or on anything that is not a raid boss with 100k of hate on it,
// that margin is one to three seconds of a damage dealer's hate: on Heirloom on 2026-11-17 Lord
// Nagafen's fight had Potsgoboom's spells at ~1,100 a second and a tank's damage at
// ~500 a second, Goad every 6 s (its recast 5 s); a heal is its whole figure's hate on every
// creature fighting the one healed (aggro.go healHateLocked). So the creature turned to the
// damage dealer or the healer a moment after each taunt and back at the next.
//
// Now (EQ's and every later game's rule, ours in its figures):
//   - a taunt that lands (a player's: tauntHateLocked, an AoE shout's each creature too; a merc's:
//     merccast.go) holds its creature on the taunter for tauntHoldFor, whatever the others' hate,
//     while the taunter is on its hate list and alive in its world; a preferred foe (prefer.go)
//     still comes first, and another's taunt takes the hold over;
//   - after the hold, a creature fighting the one who last taunted it keeps him until another's
//     threat passes his by tauntStickPct (110%): the tank's own hate keeps it while the damage
//     dealers' stays within a tenth of his. Anyone else the creature fights is chosen as before
//     (the highest), so nothing changes for a fight with no taunt in it.
//
// Not changed: a pet's taunt (pettauntmargin.go: its margin, no hold: a pet taunts off its owner,
// and pettaunttank.go keeps it off a tank), a dummy's hate (dummy.go), a Metal Moogie's
// challenge (metalmoogie.go), a familiar (no hate).

// tauntHoldFor is how long a taunt holds its creature: the taunts' recast (5 s, Taunt to Goad and
// the shouts) and a second for a press after it.
const tauntHoldFor = 6 * time.Second

// tauntStickPct is how far past the taunter's threat another's must go to take a creature held by
// his taunt once the hold has run out, in percent of his.
const tauntStickPct = 110

// holdNow is the hold's clock (a test's own).
var holdNow = time.Now

// tauntHold is a creature's last taunt: by whom and until when it holds.
type tauntHold struct {
	by    EntityID
	until time.Time
}

// tauntHolderLocked is whom npc's taunt holds it on now (the file's doc), 0 for none. Must be
// called with the lock held.
func (w *World) tauntHolderLocked(npc *NPC) EntityID {
	h := npc.taunt
	if h.by == 0 || !holdNow().Before(h.until) || !w.tauntHolderStandsLocked(npc, h.by) {
		return 0
	}
	return h.by
}

// tauntStickLocked is whom npc fights of its list once no hold binds it, top its highest threat
// held by id (0 for none): its enemy still, when he is the one who last taunted it and top is
// within tauntStickPct of his threat. Must be called with the lock held.
func (w *World) tauntStickLocked(npc *NPC, id EntityID, top int64) EntityID {
	cur, by := npc.CurrentEnemy, npc.taunt.by
	if id == 0 || id == cur || cur == 0 || cur != by {
		return id
	}
	mine := npc.Threats[cur]
	if mine <= 0 || !w.tauntHolderStandsLocked(npc, cur) || top*100 > mine*tauntStickPct {
		return id
	}
	return cur
}

// tauntHolderStandsLocked reports a taunter npc can still fight: on its hate list, a living player
// in its world, or a living NPC there. Must be called with the lock held.
func (w *World) tauntHolderStandsLocked(npc *NPC, id EntityID) bool {
	if npc.Threats[id] <= 0 {
		return false
	}
	if p := w.Players[id]; p != nil {
		return !p.IsDead() && w.npcSharesLocked(npc, p)
	}
	n := w.NPCs[id]
	return n != nil && n != npc && !n.Dead && w.npcsShareLocked(n, npc)
}
