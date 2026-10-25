package entity

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// charm.go — the enchanter's charm (the Entrancing line, and the bard's
// Entrancing Song): a creature made the caster's pet (F3 20261025-164922,
// jib: "It should check if you currently have a pet. If you do, it should
// block your spellcast and do the fizzle spell animation briefly. If you do
// not, it should check the mobs level against the charms maximum level ...
// When charm hits, it should roll a resist check against the mob. Generally
// charm has a high success rate. When it succeeds ... the monster should
// become your pet with current existing stats. All pet stances should work
// as normal, pet attack ... When the charm pet dies, it should not drop its
// loot.").
//
// What Sandstorm sent (harvests/sandstorm-spells-20261021/pets/, an
// enchanter of 49's Entrancing Captivation; claude-notes/pets-charm-client-
// 20261021.md): on a charm, 0x007E with the creature's OWN id and 0x1003
// putting its name on the non-collide list, as a summoned pet's; the
// creature keeps its channel, its byte 142 turning 3 (player-side), 143 0
// and its npc flags 0x80 (the owner cannot attack it); it follows. On
// undead: "Attempt resisted!" in red, and the creature came at the caster.
//
// Here:
//   - refused at its start, no power taken: with a pet already, "You already
//     have a pet." and a brief cast (the fizzle); a creature above the
//     charm's level, "Your target is too high a level to charm." The level a
//     charm takes is what its words say ("Charms up to level 54 mobs", the
//     bard's "Charm up to level 54"), else the spell's own level (jib: "If the
//     mob is at or below the charm level, it should cast").
//   - at its landing a resist: undead (by name: no record says) and training
//     dummies always, else 10% less the line's CHA share ("Increase 25%
//     based on CHA": 25% of CHA, in tenths of a percent), never under 2%.
//     Resisted, it fights the caster. Not measured: Sandstorm's one charm
//     that took was on a level 1 ant.
//   - taken, the creature is the caster's pet (pet.go) as it is: its level,
//     hit points and swing; it follows, takes the stances and attack, comes
//     along through teleports. Its own spells it has none of: no creature
//     casts on this server.
//   - it is let go when its line's duration ends (Entice's 2 minutes,
//     Entrancing Enticement's 4, Cajole's 2.5; the others give none, and
//     last until it is let go otherwise), when it is dismissed (both turn it
//     on its master), when its master dies, logs out or summons a pet; a
//     creature taken into another world is sent home. One that dies as a pet
//     leaves a corpse no one may loot (OpenCorpse) and comes back itself.

// charmHold is what a charm changed on a creature, to put back.
type charmHold struct {
	npcType int
	faction byte
	world   uint8
	until   time.Time // zero: until it is let go otherwise
	spell   string
}

const (
	// charmNPCType is a charmed creature's npc flags: Sandstorm's charmed
	// ant's 0x80 (0x8000 on the wire, statesync's swap).
	charmNPCType = 0x80
	// charmFizzle is the brief cast a refused charm makes.
	charmFizzle      = 600 * time.Millisecond
	charmResistBase  = 10
	charmResistFloor = 2
)

const (
	charmHavePet  = "You already have a pet."
	charmTooHigh  = "Your target is too high a level to charm."
	charmResisted = "Attempt resisted!" // Sandstorm's
	charmWornOff  = "Your charm spell has worn off."
)

// charmIntn is the charm's dice (a test's own).
var charmIntn = rand.Intn

// charmMaxLevel is the highest level the charm takes.
func charmMaxLevel(spell *CastSpell) int {
	if spell.Def.CharmMax > 0 {
		return spell.Def.CharmMax
	}
	return spell.Level
}

// charmEffect is the spell's charm effect (its CHA share).
func charmEffect(d *SpellDef) SpellEffect {
	for _, e := range d.Effects {
		if e.Kind == EffectCharm {
			return e
		}
	}
	return SpellEffect{Kind: EffectCharm}
}

// charmResistPct is the chance, in percent, that a creature resists the
// charm of a caster with that CHA.
func charmResistPct(e SpellEffect, cha int) int {
	share := 0
	if e.Scale == StatCha {
		share = e.ScalePct * max(cha, 0) / 1000
	}
	return max(charmResistBase-share, charmResistFloor)
}

// undeadWords mark a creature as undead by its name: nothing in this
// server's records does (Sandstorm resisted a charm on "Rotting Flesh").
var undeadWords = []string{"skeleton", "skeletal", "zombie", "ghoul", "ghost", "spectre", "specter", "wraith",
	"lich", "mummy", "rotting", "undead", "decaying", "revenant", "bone"}

func isUndead(npc *NPC) bool {
	n := strings.ToLower(npc.Name)
	for _, w := range undeadWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// charmStartLocked checks a charm as it begins: the refusal's line, and
// whether it fizzles (a brief cast is shown). Must be called with the lock
// held.
func (w *World) charmStartLocked(p *Player, spell *CastSpell, targets []EntityID) (refusal string, fizzle bool) {
	if w.petOfLocked(p) != nil {
		return charmHavePet, true
	}
	for _, id := range targets {
		if npc := w.NPCs[id]; npc != nil && npc.Level > charmMaxLevel(spell) {
			return charmTooHigh, false
		}
	}
	return "", false
}

// charmLandLocked is a charm landing on its target: resisted (the line, and
// the creature at the caster) or taken. Must be called with the lock held.
func (w *World) charmLandLocked(cs *castState, caster *Player, sess *session.Session, npc *NPC, now time.Time) {
	if npc.Dead || npc.Owner != 0 {
		return
	}
	if w.petOfLocked(caster) != nil {
		// a pet came while it was cast
		if sess != nil {
			w.sendColoredLocked(sess, charmHavePet, castRefusalRGB)
		}
		return
	}
	if npc.Dummy != NotDummy || isUndead(npc) || charmIntn(100) < charmResistPct(charmEffect(&cs.spell.Def), caster.StatNow(StatCha)) {
		if sess != nil {
			w.sendColoredLocked(sess, charmResisted, castRefusalRGB)
		}
		caster.LastCombatTime = now
		// hate enough to make the caster its enemy (none counts as none),
		// as Sandstorm's resisting Rotting Flesh came at its caster
		w.updateAggro(npc, caster.ServerID, 1)
		log.Printf("Charm: %s's %q resisted by %s", caster.Name, cs.spell.Name, npc.Name)
		return
	}
	w.charmLocked(caster, sess, npc, &cs.spell, now)
}

// charmLocked makes the creature the caster's pet, as it is. Must be called
// with the lock held.
func (w *World) charmLocked(caster *Player, sess *session.Session, npc *NPC, spell *CastSpell, now time.Time) {
	hold := &charmHold{npcType: npc.NPCType, faction: npc.Faction, world: npc.World, spell: spell.Name}
	if d := spell.Def.Duration; d > 0 {
		hold.until = now.Add(d)
	}
	npc.Charm = hold
	npc.Owner = caster.ServerID
	npc.NPCType = charmNPCType
	npc.Threats, npc.CurrentEnemy = nil, 0
	npc.endRound()
	npc.RoundTarget = 0
	npc.stopMoving()
	npc.roam = roamState{}
	if caster.AttackTarget == npc.ServerID {
		caster.disengage()
	}
	if w.pets == nil {
		w.pets = make(map[EntityID]*petState)
	}
	ps := &petState{owner: caster, npc: npc, stance: petDefensive, last: now}
	w.pets[caster.ServerID] = ps
	if sess != nil {
		w.announcePetLocked(ps, sess)
	}
	lasts := "until it is let go"
	if !hold.until.IsZero() {
		lasts = "for " + spell.Def.Duration.String()
	}
	log.Printf("Charm: %s's %q takes %s (level %d, %d/%d HP), %s", caster.Name, spell.Name, npc.Name, npc.Level, npc.HP, npc.MaxHP, lasts)
}

// releaseCharmLocked lets a charmed pet go: off its master's pet bar
// (0x007F, its non-collide entry off) with the line, the creature its own
// again (unCharmLocked), sent home if it is in another world, and, hostile,
// turned on its master. A dead one keeps the hold until it comes back, so its
// corpse is no one's to loot. sess is the master's, or nil. Must be called
// with the lock held.
func (w *World) releaseCharmLocked(ps *petState, sess *session.Session, text string, hostile bool) {
	npc, owner := ps.npc, ps.owner
	if w.pets[owner.ServerID] == ps {
		delete(w.pets, owner.ServerID)
	}
	ps.target = 0
	npc.endRound()
	npc.RoundTarget = 0
	npc.stopMoving()
	if sess != nil {
		sendClearPet(sess)
		if ps.noCollide {
			sendNoCollideRemove(sess, petNoCollideID)
		}
		if text != "" {
			w.sendChatToSession(sess, text)
		}
	}
	if npc.Dead {
		log.Printf("Charm: %s's charmed %q died", owner.Name, npc.Name)
		return
	}
	home := w.unCharmLocked(npc)
	if npc.World != home {
		oldX, oldZ := npc.X, npc.Z
		npc.World = home
		npc.X, npc.Y, npc.Z, npc.Facing = npc.SpawnX, npc.SpawnY, npc.SpawnZ, npc.SpawnFacing
		w.SpatialIndex.Update(npc.ServerID, oldX, oldZ, npc.X, npc.Z)
		hostile = false
	}
	if hostile && !owner.IsDead() && w.Players[owner.ServerID] == owner {
		w.updateAggro(npc, owner.ServerID, 1)
	}
	log.Printf("Charm: %s's %q goes free (hostile %v)", owner.Name, npc.Name, hostile)
}

// unCharmLocked puts back what a charm changed on the creature and returns
// its own world (its present one when no charm held it).
func (w *World) unCharmLocked(npc *NPC) uint8 {
	h := npc.Charm
	if h == nil {
		return npc.World
	}
	npc.Owner = 0
	npc.NPCType = h.npcType
	npc.Faction = h.faction
	npc.Charm = nil
	return h.world
}
