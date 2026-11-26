package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// hcrules.go — a Hardcore character is never helped (the owner, 2026-11-18: "so
// HC players aren't helped (can't be power-levelled, can't have high levels kill
// quest mobs for them)"). Where world.hardcore is on (HardcoreOn), for a
// Hardcore character (Player.Hardcore):
//
//   - A creature's fight is earned only by those who alone held its hate
//     (noteHateLocked keeps every holder of it this fight, hcHate): a Hardcore
//     character gets no experience (and so no mastery points), no faction, no
//     quest credit and no loot rights from a kill unless every one that was on
//     its hate list was itself, a Hardcore member of its group, or one of their
//     pets (hcEarnedLocked). Anyone else (a player not Hardcore, a Hardcore one
//     of another group, any player's mercenary or its pet) on the list at any
//     time of the fight taints it for good: killSharersLocked and
//     creditKillLocked leave the Hardcore out (killexp.go, faction.go,
//     questhooks.go), raidKillLocked gives a tainted Hardcore group no rolls
//     (raidrules.go), and the loot rights refuse (handlers/lootrights.go). The
//     record is the creature's life's, kept through its death until it rises
//     again, given up (giveUpLocked: it went home whole) or back to full health
//     with no one on its list.
//   - Groups and raids are all Hardcore or all not (HardcoreMismatch): an
//     invitation either way is refused, and an acceptance (group.go,
//     raidcmds.go).
//   - Help lands only within a Hardcore character's own group (hcAidRefusedLocked:
//     a beneficial spell, an item's, a pet's or a merc's on another player,
//     landOnPlayerLocked): a player not Hardcore cannot aid a Hardcore one,
//     nor a Hardcore one aid anyone outside its own group.
//   - No mercenary is hired, brought back or restored (HireMercAt, RestoreMerc).

// hcHolderOutside marks, in an NPC's hcHate, a holder no Hardcore character
// counts as its own (a mercenary or a merc's pet).
const hcHolderOutside EntityID = 0

// hcHolder is what an NPC keeps of a holder of its hate: the player whose it
// counts as (a pet's owner), and whether that player was Hardcore then.
type hcHolder struct {
	hardcore bool
}

// HardcoreMismatch reports two players who may never group, trade help or
// otherwise deal as one: one Hardcore and one not, on a shard with
// world.hardcore.
func HardcoreMismatch(a, b *Player) bool {
	return HardcoreOn() && a != nil && b != nil && a.Hardcore != b.Hardcore
}

// noteHateLocked keeps id as a holder of npc's hate this fight (its life):
// the player whose it counts as (a pet's owner); a mercenary, or a merc's pet,
// as one outside every Hardcore group. A fight begun on a creature whole and
// with no one on its list starts the record anew. Must be called with the lock
// held.
func (w *World) noteHateLocked(npc *NPC, id EntityID) {
	if npc == nil || id == 0 {
		return
	}
	if len(npc.Threats) == 0 && npc.HP >= npc.MaxHP {
		npc.hcHate = nil
	}
	if npc.hcHate == nil {
		npc.hcHate = map[EntityID]hcHolder{}
	}
	if p := w.Players[id]; p != nil {
		npc.hcHate[id] = hcHolder{hardcore: p.Hardcore}
		w.bestiaryMetLocked(npc, p, time.Now()) // the player met it (bestiary.go)
		return
	}
	n := w.NPCs[id]
	if n == nil {
		return
	}
	if n.isMerc() || n.mercPet != nil {
		npc.hcHate[hcHolderOutside] = hcHolder{}
		return
	}
	if n.Owner != 0 {
		if o := w.Players[n.Owner]; o != nil {
			npc.hcHate[o.ServerID] = hcHolder{hardcore: o.Hardcore}
			w.bestiaryMetLocked(npc, o, time.Now()) // its owner met it (bestiary.go)
			return
		}
		npc.hcHate[hcHolderOutside] = hcHolder{}
	}
	// any other creature (a guard, a creature's own pet): no one's help
}

// forgetHateLocked ends npc's record of its hate's holders: a new life, or its
// fight given up.
func forgetHate(npc *NPC) { npc.hcHate = nil }

// hcEarnedLocked reports whether the Hardcore character m earned npc's kill:
// every holder of its hate this fight m, a Hardcore member of m's group (a
// raid's too), or a pet of one. Must be called with the lock held.
func (w *World) hcEarnedLocked(npc *NPC, m *Player) bool {
	if npc == nil || m == nil {
		return false
	}
	group := map[EntityID]bool{m.ServerID: true}
	for _, g := range w.groupMembersLocked(m) {
		if g.Hardcore {
			group[g.ServerID] = true
		}
	}
	for id, h := range npc.hcHate {
		if id == hcHolderOutside || !h.hardcore || !group[id] {
			return false
		}
	}
	return true
}

// hcBarredLocked reports a Hardcore character barred from npc's kill: one that
// did not earn it (hcEarnedLocked). Anyone else is never barred. Must be
// called with the lock held.
func (w *World) hcBarredLocked(npc *NPC, m *Player) bool {
	if m == nil || !m.HardcoreBound() {
		return false
	}
	if w.hcEarnedLocked(npc, m) {
		return false
	}
	log.Printf("Hardcore: %s gets nothing of %q: another's help was on its hate list", m.Name, npc.Name)
	return true
}

// HardcoreEarned reports whether the Hardcore character's kill of the NPC was
// its own (its group's) alone; anyone else always earns.
func (w *World) HardcoreEarned(npcID EntityID, p *Player) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if p == nil || !p.HardcoreBound() {
		return true
	}
	npc := w.NPCs[npcID]
	return npc != nil && w.hcEarnedLocked(npc, p)
}

// The help refusals' words (ours).
const (
	hcAidCasterWords = "You cannot aid a Hardcore adventurer."
	hcAidHCWords     = "%s tried to aid you: a Hardcore adventurer cannot accept aid."
	hcAidOutWords    = "A Hardcore adventurer may aid only their own group."
)

// hcAidRefusedLocked reports a beneficial landing of caster's on t refused: t
// Hardcore and caster not of its group (or not Hardcore), or caster Hardcore
// and t not of its group. Self help is always fine. tell: say so to both.
// Must be called with the lock held.
func (w *World) hcAidRefusedLocked(caster, t *Player, tell bool) bool {
	if !HardcoreOn() || caster == nil || t == nil || caster == t {
		return false
	}
	if !caster.Hardcore && !t.Hardcore {
		return false
	}
	if caster.Hardcore && t.Hardcore && caster.GroupID != 0 && caster.GroupID == t.GroupID {
		return false
	}
	if caster.Hardcore && t.Hardcore && w.sameRaidLocked(caster, t) {
		return false
	}
	if tell {
		if cs := w.findSessionByEntityIDLocked(caster.ServerID); cs != nil {
			if t.Hardcore {
				w.sendColoredLocked(cs, hcAidCasterWords, castRefusalRGB)
			} else {
				w.sendColoredLocked(cs, hcAidOutWords, castRefusalRGB)
			}
		}
		if t.Hardcore {
			if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
				w.sendColoredLocked(ts, fmt.Sprintf(hcAidHCWords, caster.Name), castRefusalRGB)
			}
		}
	}
	log.Printf("Hardcore: %s's aid on %s refused", caster.Name, t.Name)
	return true
}

// sameRaidLocked reports a and b in one raid (its groups one group here).
func (w *World) sameRaidLocked(a, b *Player) bool {
	for _, m := range w.groupMembersLocked(a) {
		if m == b {
			return true
		}
	}
	return false
}

// The group refusals' words (ours).
const (
	hcGroupWords = "A Hardcore adventurer groups only with Hardcore adventurers."
	hcMercWords  = "A Hardcore adventurer fights without mercenaries."
)

// hcGroupRefusedLocked reports p refused a group (or raid) g of players of
// the other kind, told in red. Must be called with the lock held.
func (w *World) hcGroupRefusedLocked(sess *session.Session, p *Player, g *Group) bool {
	if !HardcoreOn() || g == nil || p == nil {
		return false
	}
	for _, id := range g.Members {
		if m := w.Players[id]; m != nil && m != p && m.Hardcore != p.Hardcore {
			if sess != nil {
				w.sendColoredLocked(sess, hcGroupWords, castRefusalRGB)
			}
			log.Printf("Hardcore: %s refused %s's group %d (one Hardcore, one not)", p.Name, m.Name, g.ID)
			return true
		}
	}
	return false
}

// hcInviteRefusedLocked reports inviter's invitation of target refused (one
// Hardcore, one not), the inviter told. Must be called with the lock held.
func (w *World) hcInviteRefusedLocked(sess *session.Session, inviter, target *Player) bool {
	if !HardcoreMismatch(inviter, target) {
		return false
	}
	if sess != nil {
		w.sendColoredLocked(sess, hcGroupWords, castRefusalRGB)
	}
	log.Printf("Hardcore: %s's invitation of %s refused (one Hardcore, one not)", inviter.Name, target.Name)
	return true
}

// HardcoreBoundSession reports the session's player Hardcore on this shard.
func (w *World) HardcoreBoundSession(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && p.HardcoreBound()
}
