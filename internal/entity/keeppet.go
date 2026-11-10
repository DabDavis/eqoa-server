package entity

import (
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/session"
)

// keeppet.go — a player's pet through a restart that keeps the sessions
// (petition 1363, Battaryhog: "i noticed if my mag or enc sets still to long
// the pets despawn, and the charmed ones attack"). A keep restart (keep.go)
// freezes the world a few seconds, and until now ended every pet in it: a
// summoned pet was simply gone, and a charmed creature came back as its row
// makes it, free and whole on its spawn, where its master usually hunts, so it
// attacked him on sight (2026-11-09 05:07, a charmed wicker man on
// eqoaclassic). Ten restarts in four hours made it look like standing still.
//
// Now KeepPlayer writes the pet down beside its owner (KeptPet), and the new
// process gives it back (RestorePet, from handlers/keep.go's ResumeCharacter):
//   - a summoned pet by its summoning spell, made again beside its owner as
//     the spell's landing makes it, with its share of its hit points and its
//     stance;
//   - a charmed creature by its row (an NPC's id is its row's, the same in the
//     new process): the one its row made at the start, where it stood, its
//     share of its hit points, charmed to its master again under the charm it
//     was under, for the time that charm had left (none for one that holds
//     until it is let go) and its own world's grace; no resist is rolled.
// The client is told the pet after its MemoryDump, as at a login (PetDumped,
// petSystem's announce). One that cannot be given back (its spell or its
// creature gone, a creature someone else charmed, its owner dead) is not: the
// charmed one stays its row's own.

// KeptPet is a player's pet as a restart carries it.
type KeptPet struct {
	Name   string     `json:"name"`
	Spell  int        `json:"spell,omitempty"` // a summoned pet's summoning spell
	NPC    uint32     `json:"npc,omitempty"`   // a charmed creature's id (its row's)
	HPPct  float64    `json:"hp_pct"`
	Stance byte       `json:"stance,omitempty"`
	Place  [4]float32 `json:"place"` // x, y, z, facing
	World  uint8      `json:"world"`
	// a charmed creature's charm: the spell's name, its time left (0: until it
	// is let go) and the creature's own world
	Charm  string `json:"charm,omitempty"`
	LeftMS int64  `json:"left_ms,omitempty"`
	Home   uint8  `json:"home,omitempty"`
}

// keptPetLocked is p's living pet as a restart carries it, nil for none or one
// nothing could make again (a summoned pet with no spell). Must be called with
// the lock held.
func (w *World) keptPetLocked(p *Player, now time.Time) *KeptPet {
	ps := w.petStateLocked(p)
	if ps == nil {
		return nil
	}
	pet := ps.npc
	k := &KeptPet{Name: pet.Name, Stance: byte(ps.stance), Place: [4]float32{pet.X, pet.Y, pet.Z, pet.Facing}, World: pet.World,
		HPPct: float64(pet.HP) / float64(max(pet.MaxHP, 1))}
	switch h := pet.Charm; {
	case h != nil:
		k.NPC, k.Charm, k.Home = uint32(pet.ServerID), h.spell, h.world
		if !h.until.IsZero() {
			k.LeftMS = max(h.until.Sub(now).Milliseconds(), 1)
		}
	case ps.spell != 0:
		k.Spell = ps.spell
	default:
		return nil
	}
	return k
}

// RestorePet gives sess's player back the pet a restart carried (the file's
// doc): spell is the summoning spell's record for a summoned one (nil when it
// is gone), unused for a charmed one. It reports whether the pet came back.
func (w *World) RestorePet(sess *session.Session, k KeptPet, spell *CastSpell, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() || p.respawnPending() || w.petStateLocked(p) != nil {
		return false
	}
	if k.NPC != 0 {
		npc := w.NPCs[EntityID(k.NPC)]
		if npc == nil || npc.Dead || npc.Owner != 0 || npc.Charm != nil || npc.Name != k.Name || npc.Temporary {
			log.Printf("Keep: %s's charmed %q (%d) is not there to charm again", p.Name, k.Name, k.NPC)
			return false
		}
		if npc.World == k.World {
			oldX, oldZ := npc.X, npc.Z
			npc.X, npc.Y, npc.Z, npc.Facing = k.Place[0], k.Place[1], k.Place[2], k.Place[3]
			w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		}
		npc.HP = keptShare(k.HPPct, npc.MaxHP)
		hold := &charmHold{npcType: npc.NPCType, faction: npc.Faction, world: k.Home, spell: k.Charm}
		if k.LeftMS > 0 {
			hold.until = now.Add(time.Duration(k.LeftMS) * time.Millisecond)
		}
		w.holdCharmLocked(p, nil, npc, hold, now) // told after its dump (petSystem)
		w.pets[p.ServerID].stance = petStance(k.Stance)
		log.Printf("Keep: %s's charmed %q back (%d/%d HP, %s left)", p.Name, npc.Name, npc.HP, npc.MaxHP,
			time.Duration(k.LeftMS)*time.Millisecond)
		return true
	}
	if spell == nil || spell.SpellID != k.Spell {
		log.Printf("Keep: %s's %q: its spell %d is gone", p.Name, k.Name, k.Spell)
		return false
	}
	w.summonPetLocked(&castState{spell: *spell, caster: p.ServerID}, p, nil, now) // told after its dump (petSystem)
	ps := w.petStateLocked(p)
	if ps == nil {
		return false
	}
	ps.npc.HP = keptShare(k.HPPct, ps.npc.MaxHP)
	if k.Stance != 0 {
		ps.stance = petStance(k.Stance)
	}
	log.Printf("Keep: %s's %q back (%d/%d HP)", p.Name, ps.npc.Name, ps.npc.HP, ps.npc.MaxHP)
	return true
}

// keptShare is a share of max hit points, at least 1.
func keptShare(share float64, max int) int {
	return min(max, int(math.Max(1, math.Round(share*float64(max)))))
}
