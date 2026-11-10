package entity

import (
	"log"
	"math"
	"regexp"
	"strings"
	"time"
)

// companion.go — a creature and its own pet fight as one (the owner, 2026-11-09:
// "the mobs that have pets dont protect them or attack they are treated as not
// pets").
//
// What ties a pet to an NPC owner on this server: nothing but the rows. No
// table links two NPCs (npcsocial helps by name and faction; campspawns groups
// a camp's kinds), no class template summons (templateKindTakes refuses a
// summoning, and the Charmable Mobs page's "these minions" are summoned by
// nothing here), and an NPC's Owner is only ever a player. What the rows hold
// (eqoaclassic's of 2026-11-09, 61,254 NPCs): 229 rows of 23 names a pet goes
// by (Companion Spirit 172, decaying minion 15, undead minion 10, spectral
// minion 5, Familiar 3, "Goran's pet", "Minion of Krrgar", "Servant of
// Vorin", ...), the captures and dumps they came from having caught a
// caster's pet beside it: 196 of them stand within 10 of a creature that is no
// pet (half within 1.7, nine in ten within 3.1), the other 33 alone. The
// owners so found are the casters the live game gave pets: goblin witch
// doctors, gnoll shamans and spiritists, hobgoblin oracles, froglok shamans
// with their Companion Spirits, necromancers with their minions.
//
// So the pairs are read off the rows at their spawns (linkCompanionsLocked,
// again whenever the world's NPCs change in number): an NPC whose name is a
// pet's (companionName) is the companion of the creature its name names
// ("<owner>'s pet", "Servant of <owner>") within companionNamedReach of its
// spawn, else of the nearest creature within companionReach of it; never one
// that is a pet itself, has the NPC bit (0x80: a merchant or a quest giver,
// who fights no one), is a training dummy, a spawn-only original or a GM's
// copy. The live game's caster fought with its pet beside it, and now:
//   - a hostile act on either one (updateAggro: a hit, a spell, a resisted
//     charm) or either one noticing a player (aggro.go) brings the other into
//     the fight against the same foe (companionJoinLocked), whoever did it: a
//     player, or a player's pet (sent at its owner, as socialaggro.go sends its
//     helpers);
//   - the owner giving up and going home takes its pets home with it
//     (giveUpLocked);
//   - the owner's death takes its pets with it, each laid down dead where it
//     stands (masterdeath.go: its death played, its body gone a few seconds
//     later, a corpse of nothing; until 2026-11-10 gone at once with no death
//     seen), and its return brings them back with it (respawnNPCLocked).
// A pet charmed by a player is the player's while the charm holds and is no one
// else's companion (companionFighter); a player's pets and charmed creatures
// are never paired, and town guards' assist (socialaggro.go) is untouched.

// companionReach is how near a creature's spawn its unnamed pet spawns, and
// companionNamedReach how near one whose name names its owner.
const (
	companionReach      float32 = 4
	companionNamedReach float32 = 30
)

// companionWords are the words a pet's name ends in (any case). "servant"
// only names an owner's ("Moogie's Servant", "Servant of Vorin": the
// companionOwnerName forms): alone it is a kind of creature here (dark
// servants, slith tar servants, tower servants, 70 rows mostly standing alone).
var companionWords = regexp.MustCompile(`(?i)(^|[\s'])(pet|minion|familiar|thrall|companion spirit)$`)

// companionOwnerName is the owner a pet's name names: "Goran's pet" Goran,
// "Minion of Krrgar" Krrgar, "" for none.
var companionOwnerName = regexp.MustCompile(`(?i)^(?:(.+)'s (?:pet|minion|familiar|servant|thrall)|(?:pet|minion|familiar|servant|thrall) of (.+))$`)

// companionName is whether an NPC's name is a pet's.
func companionName(name string) bool {
	name = strings.TrimSpace(name)
	return companionWords.MatchString(name) || companionOwnerName.MatchString(name)
}

// companionFighter is whether an NPC takes part in a pair at all: a creature
// of its own (no player's pet, no charm), that fights (no NPC bit, no dummy),
// and is of a row (no GM's copy, no spawn-only original).
func companionFighter(n *NPC) bool {
	return n != nil && n.Owner == 0 && n.Charm == nil && n.NPCType&0x80 == 0 && n.Dummy == NotDummy && !n.Temporary && !n.SpawnOnly
}

// linkCompanionsLocked reads the pairs off the NPCs' spawns (the file's doc),
// when the world's NPCs changed in number since it last did. Must be called
// with the lock held.
func (w *World) linkCompanionsLocked() {
	if w.companionsAt == len(w.NPCs) && w.companionOf != nil {
		return
	}
	w.companionsAt = len(w.NPCs)
	w.companionOf = map[EntityID]EntityID{}
	w.companions = map[EntityID][]EntityID{}
	type cell struct {
		world uint8
		x, z  int
	}
	grid := map[cell][]*NPC{}
	at := func(n *NPC) cell {
		return cell{n.World, int(math.Floor(float64(n.SpawnX / companionNamedReach))), int(math.Floor(float64(n.SpawnZ / companionNamedReach)))}
	}
	var pets []*NPC
	for _, n := range w.NPCs {
		if !companionFighter(n) {
			continue
		}
		if companionName(n.Name) {
			pets = append(pets, n)
			continue
		}
		grid[at(n)] = append(grid[at(n)], n)
	}
	for _, pet := range pets {
		named := ""
		if m := companionOwnerName.FindStringSubmatch(strings.TrimSpace(pet.Name)); m != nil {
			named = strings.ToLower(strings.TrimSpace(m[1] + m[2]))
		}
		var owner *NPC
		best := float32(math.MaxFloat32)
		c := at(pet)
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				for _, o := range grid[cell{c.world, c.x + dx, c.z + dz}] {
					d := distXZ(pet.SpawnX, pet.SpawnZ, o.SpawnX, o.SpawnZ)
					score := d
					switch {
					case named != "" && d <= companionNamedReach && strings.EqualFold(strings.TrimSpace(o.Name), named):
						score = d - companionNamedReach - companionReach // the owner its name names, before any other
					case d <= companionReach:
					default:
						continue
					}
					if score < best {
						owner, best = o, score
					}
				}
			}
		}
		if owner != nil {
			w.companionOf[pet.ServerID] = owner.ServerID
			w.companions[owner.ServerID] = append(w.companions[owner.ServerID], pet.ServerID)
		}
	}
	log.Printf("Companions: %d creatures' pets beside them, of %d NPCs named as pets", len(w.companionOf), len(pets))
}

// companionPartnersLocked is every NPC fighting as one with npc: its pets, or
// its owner and the owner's other pets; none when it is in no pair. Must be
// called with the lock held.
func (w *World) companionPartnersLocked(npc *NPC) []*NPC {
	w.linkCompanionsLocked()
	owner := npc.ServerID
	if o, ok := w.companionOf[npc.ServerID]; ok {
		owner = o
	}
	ids := append([]EntityID{owner}, w.companions[owner]...)
	if len(ids) == 1 {
		return nil
	}
	var out []*NPC
	for _, id := range ids {
		if n := w.NPCs[id]; id != npc.ServerID && n != nil && companionFighter(n) {
			out = append(out, n)
		}
	}
	return out
}

// companionJoinLocked brings npc's partners into its fight against attacker
// (a player, or a player's pet sent at its owner), each alive, in its world and
// not already fighting that one. Must be called with the lock held.
func (w *World) companionJoinLocked(npc *NPC, attackerID EntityID) {
	if !companionFighter(npc) {
		return
	}
	enemy := w.socialEnemyLocked(attackerID)
	if enemy == 0 {
		return
	}
	for _, p := range w.companionPartnersLocked(npc) {
		if p.Dead || p.World != npc.World || p.Threats[enemy] > 0 {
			continue
		}
		w.addThreatLocked(p, enemy, 1)
		log.Printf("Companions: %s joins %s against %s", p.Name, npc.Name, w.entityNameLocked(enemy))
	}
}

// companionsHomeLocked takes an owner's pets home with it when it gives up a
// fight. Must be called with the lock held.
func (w *World) companionsHomeLocked(owner *NPC) {
	w.linkCompanionsLocked()
	if !companionFighter(owner) || w.companions[owner.ServerID] == nil {
		return
	}
	for _, id := range w.companions[owner.ServerID] {
		if p := w.NPCs[id]; p != nil && companionFighter(p) && !p.Dead && (p.CurrentEnemy != 0 || len(p.Threats) > 0) {
			w.giveUpLocked(p)
		}
	}
}

// companionsDieLocked lays an owner's living pets down dead with it when it
// dies (masterdeath.go's killWithMasterLocked: their deaths played, their
// bodies gone a few seconds later, their respawn time running from then). Must
// be called with the lock held.
func (w *World) companionsDieLocked(owner *NPC, now time.Time) {
	w.linkCompanionsLocked()
	if w.companions[owner.ServerID] == nil || owner.Owner != 0 || owner.Charm != nil {
		return
	}
	for _, id := range w.companions[owner.ServerID] {
		p := w.NPCs[id]
		if p == nil || p.Dead || !companionFighter(p) {
			continue
		}
		w.killWithMasterLocked(p, now)
		log.Printf("Companions: %s dies with %s", p.Name, owner.Name)
	}
}

// companionsBackLocked brings back an owner's pets that went with it, as it
// comes back itself. Must be called with the lock held.
func (w *World) companionsBackLocked(owner *NPC) {
	for _, id := range w.companions[owner.ServerID] {
		if p := w.NPCs[id]; p != nil && p.Dead && !p.CorpseGone.IsZero() && companionFighter(p) {
			w.respawnNPCLocked(p)
		}
	}
}
