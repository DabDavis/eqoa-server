package entity

import (
	"log"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/database"
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
// ("<owner>'s pet", "Servant of <owner>": the name, or its first or last
// words, punctuation aside, companionNamesOwner) within companionNamedReach of
// its spawn, else of the nearest creature within companionReach of it; never one
// that is a pet itself, has the NPC bit (0x80: a merchant or a quest giver,
// who fights no one), is a training dummy, a spawn-only original or a GM's
// copy; and no kind a pet's word names that is no one's pet (companionNotPets).
// A GM trigger's resummon names a pair too ("Tunare calls Wrath of the Mother
// back": gmcompanions.go's SetTriggerCompanions), as a pet's name names its
// owner. A GM's copy of a creature brings copies of its companions, paired with
// it alone (gmcompanions.go).
// The live game's caster fought with its pet beside it, and now:
//   - the pet walks with its owner as it wanders (companionsFollowLocked,
//     2026-11-11), and does not wander of itself (movement.go's whyCompanion);
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
	companionNamedReach float32 = 40
)

// companionFollowAt is the most a pet keeps off its owner as it walks with it
// (companionsFollowLocked): its place beside it at the spawns, nearer when
// that was further; companionCatchUp is how far behind it hurries, half as
// fast again, as a player's pet does (followslot.go's followLocked).
const (
	companionFollowAt float32 = 3
	companionCatchUp  float32 = 8
)

// companionNotPets are the kinds a pet's word names that are no one's pet
// (database.NPCClassKey's keys): Zaoran Plateau's Blood Thirsty Minion camps
// (tools/guidegaps/zp-minions.json: "the new Blood Thirsty Minion camps", a
// creature of their own, 20 rows standing in fives with no caster near), and
// a village's animal (Qeynos hills' "village pet" among the farmers). Read as
// pets, they were the owner's "pets not connected to their master"
// (2026-11-11): a third of the 60 the log counted with none.
var companionNotPets = map[string]bool{
	"blood thirsty minion": true,
	"village pet":          true,
}

// companionWords are the words a pet's name ends in (any case). "servant"
// only names an owner's ("Moogie's Servant", "Servant of Vorin": the
// companionOwnerName forms): alone it is a kind of creature here (dark
// servants, slith tar servants, tower servants, 70 rows mostly standing alone).
var companionWords = regexp.MustCompile(`(?i)(^|[\s'])(pet|minion|familiar|thrall|companion spirit)$`)

// companionOwnerName is the owner a pet's name names: "Goran's pet" Goran,
// "Minion of Krrgar" Krrgar, "" for none.
var companionOwnerName = regexp.MustCompile(`(?i)^(?:(.+)'s (?:pet|minion|familiar|servant|thrall)|(?:pet|minion|familiar|servant|thrall) of (.+))$`)

// companionKinds are the kinds whose every creature is a caster's pet though
// no word of its name says so, by the word its owners' names end in
// (database.NPCClassKey's keys): Guk's Mummified Guardians, the necromancers'
// (Waldorf's petition 1832, 2026-11-12, "same with the Necs and their pets":
// 47 rows, most within 2 of a froglok or ghoul necromancer, a few 6 to 7.4
// off one, so they find their owner as a named pet does, companionNamedReach).
var companionKinds = map[string]string{
	"mummified guardian": "necromancer",
	"elder spirit":       "shaman",
}

// companionOwnerOnly are the companionKinds whose creature is a pet only beside
// an owner its kind's word names (no nearest creature in its stead): Guk's elder
// spirits are its shamans' (Heirloom petition 2291, Waldorf, 2026-11-17, "Elder
// Spirits in Guk Instance are lootable and should not be since they are pets":
// each of the four stands 1.5 to 1.8 off a froglok yun or guk shaman, or within
// 16 of one), while the Tower of Phantoms' two elder spirits stand among its
// spirits with no shaman near, creatures of their own (their robe, 1599).
var companionOwnerOnly = map[string]bool{
	"elder spirit": true,
}

// companionPetKinds are the kinds whose every creature is a pet though no word of its name says so, with no owner's
// word: each the pet of the creature beside it (companionReach), as an unnamed pet's. Feral Spirits are the creature
// shamans' (petition 1846, Waldorf, 2026-11-12, "All feral spirits in Runneye instance our neutral and not
// aggressive": npcaggro had them as players' pets; of 157 rows, 100 stand within 5 of a gnoll, goblin or Tae Ew
// shaman, an oracle, a seer, a mystic or a spiritmaster, the rest alone).
//
// Howling Death is a necromancer's (ticket 2009, Waldorf, 2026-11-13, "Guk instance - Howling Death isn't tethered to
// Rubik and is neutral faced"): all six rows stand 0.7 to 2.05 off their owner (Rubbip in Guk and his instance copy,
// Lothaxe, Amandon, two ambassadors).
//
// And the other pets' names the seed's npcaggro rows named in the instances (the coordinator, 2026-11-13, after ticket
// 2009), each the pet of the creature beside it as it stands in the open world: Kindred Spirits (151 rows, most 1-3 off
// an orc augur or prophet, a spiritfeather aviak, a snowfist, Freezeblood, baga or gnoll mystic, a goblin seer or shaman,
// a Tae Ew shaman), Fire Elementals (3, each 1.1-1.6 off a mage or Lord Usael), Decaying and Wilting Flesh (by lesser
// ghouls) and Swarming Bones (by its froglok bonecaster).
var companionPetKinds = map[string]bool{
	"feral spirit":   true,
	"howling death":  true,
	"kindred spirit": true,
	"fire elemental": true,
	"decaying flesh": true,
	"wilting flesh":  true,
	"swarming bones": true,
}

// companionName is whether an NPC's name is a pet's.
func companionName(name string) bool {
	name = strings.TrimSpace(name)
	key := database.NPCClassKey(name)
	if companionNotPets[key] {
		return false
	}
	if companionKinds[key] != "" || companionPetKinds[key] {
		return true
	}
	return companionWords.MatchString(name) || companionOwnerName.MatchString(name)
}

// companionFold is a name as companionNamesOwner compares it: lower case, its
// letters and digits alone, one space between its words ("Tal'Thir" and
// "Tal Thir" alike).
func companionFold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
			continue
		}
		space = true
	}
	return b.String()
}

// companionNamesOwner is whether a pet's name's owner (named, from
// companionOwnerName) is the creature called owner: the same name, or its
// first or last words ("Servant of Vorin" High Priest Vorin's, "maiden's
// servant" the Dawn Maiden's, "Minion of Vhi'zan" Vhi'zan Kaah's), punctuation
// aside ("Servant of Tal Thir" Tal'Thir's).
func companionNamesOwner(named, owner string) bool {
	n, o := companionFold(named), companionFold(owner)
	if n == "" || o == "" {
		return false
	}
	return n == o || strings.HasPrefix(o, n+" ") || strings.HasSuffix(o, " "+n)
}

// companionFighter is whether an NPC takes part in a pair at all: a creature
// of its own (no player's pet, no charm), that fights (no NPC bit, no dummy),
// and is of a row (no GM's copy, no spawn-only original) or a dungeon
// instance's copy of one (dungeons.go: Waldorf's petition 1832, 2026-11-12,
// "shamans aren't tied to their pets" in Guk's instance; a copy pairs with the
// copies of its own instance alone, linkCompanionsLocked's cell).
//
// A GM's spawn (SpawnGMCopy) takes part too, but only in the pair its spawning
// made (gmCompanionOf: gmcompanions.go), never one read off the spawns.
func companionFighter(n *NPC) bool {
	return n != nil && n.Owner == 0 && n.Charm == nil && n.NPCType&0x80 == 0 && n.Dummy == NotDummy &&
		(!n.Temporary || n.Instance != 0 || n.GMSpawn) && !n.SpawnOnly
}

// companionPetRuleLocked is whether a creature called name is a pet by its name
// (companionName, or a GM trigger's resummon: gmcompanions.go), the owner's name
// its name (or the trigger) names ("" none), and whether it is a pet only beside
// that owner (companionOwnerOnly; a trigger's always). Must be called with the
// lock held.
func (w *World) companionPetRuleLocked(name string) (pet bool, named string, ownerOnly bool) {
	// a name's answer once (companionRules; dropped when the triggers' pets
	// change): its expressions over every creature's name were most of a
	// relink, Live's roam turns of 300-600 ms on each (2026-11-18)
	if r, ok := w.companionRules[name]; ok {
		return r.pet, r.named, r.ownerOnly
	}
	pet, named, ownerOnly = w.companionPetRuleUncachedLocked(name)
	if w.companionRules == nil {
		w.companionRules = map[string]companionRule{}
	}
	w.companionRules[name] = companionRule{pet, named, ownerOnly}
	return pet, named, ownerOnly
}

// companionRule is companionPetRuleLocked's answer for a name.
type companionRule struct {
	pet       bool
	named     string
	ownerOnly bool
}

// companionPetRuleUncachedLocked is companionPetRuleLocked's answer, worked out.
func (w *World) companionPetRuleUncachedLocked(name string) (pet bool, named string, ownerOnly bool) {
	key := database.NPCClassKey(name)
	if by := w.triggerPets[key]; by != "" {
		return true, by, true
	}
	if !companionName(name) {
		return false, "", false
	}
	if m := companionOwnerName.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
		named = strings.TrimSpace(m[1] + m[2])
	} else if k := companionKinds[key]; k != "" {
		named = k
	}
	return true, named, companionOwnerOnly[key]
}

// companionScore is how good an owner o is for pet (lower better), false when it
// is none: the owner its name names within companionNamedReach before any other,
// else (unless ownerOnly) the nearest within companionReach.
func companionScore(pet, o *NPC, named string, ownerOnly bool) (float32, bool) {
	d := distXZ(pet.SpawnX, pet.SpawnZ, o.SpawnX, o.SpawnZ)
	switch {
	case named != "" && d <= companionNamedReach && companionNamesOwner(named, o.Name):
		return d - companionNamedReach - companionReach, true
	case d <= companionReach && !ownerOnly:
		return d, true
	}
	return 0, false
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
		inst  uint32
		x, z  int
	}
	grid := map[cell][]*NPC{}
	at := func(n *NPC) cell {
		return cell{n.World, n.Instance, int(math.Floor(float64(n.SpawnX / companionNamedReach))), int(math.Floor(float64(n.SpawnZ / companionNamedReach)))}
	}
	var pets []*NPC
	others := w.companionScratch[:0] // the other fighters, in the map's order
	for _, n := range w.NPCs {
		if !companionFighter(n) {
			continue
		}
		if n.GMSpawn {
			// a GM's spawn: the pair its spawning made, no other (gmcompanions.go)
			if o := w.NPCs[n.gmCompanionOf]; n.gmCompanionOf != 0 && o != nil && o.GMSpawn && companionFighter(o) {
				w.companionOf[n.ServerID] = o.ServerID
				w.companions[o.ServerID] = append(w.companions[o.ServerID], n.ServerID)
			}
			continue
		}
		if pet, _, _ := w.companionPetRuleLocked(n.Name); pet {
			pets = append(pets, n)
			continue
		}
		others = append(others, n)
	}
	// only the cells a pet looks in (its own and the eight round it) are
	// gridded: the rest of the world's fighters are never asked
	near := make(map[cell]bool, 9*len(pets))
	for _, pet := range pets {
		c := at(pet)
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				near[cell{c.world, c.inst, c.x + dx, c.z + dz}] = true
			}
		}
	}
	for _, n := range others {
		if c := at(n); near[c] {
			grid[c] = append(grid[c], n)
		}
	}
	clear(others) // no NPC kept alive by the scratch
	w.companionScratch = others[:0]
	for _, pet := range pets {
		_, named, ownerOnly := w.companionPetRuleLocked(pet.Name)
		var owner *NPC
		best := float32(math.MaxFloat32)
		c := at(pet)
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				for _, o := range grid[cell{c.world, c.inst, c.x + dx, c.z + dz}] {
					score, ok := companionScore(pet, o, named, ownerOnly)
					if ok && score < best {
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

// companionMasterLocked is the creature npc is the pet of (the pairs, linkCompanionsLocked), nil for none.
// Must be called with the lock held.
func (w *World) companionMasterLocked(npc *NPC) *NPC {
	if !companionFighter(npc) {
		return nil
	}
	w.linkCompanionsLocked()
	o, ok := w.companionOf[npc.ServerID]
	if !ok {
		return nil
	}
	if m := w.NPCs[o]; m != nil && companionFighter(m) {
		return m
	}
	return nil
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
// comes back itself: one that died with it, its body lying yet or gone, or
// one whose corpse went. Must be called with the lock held.
func (w *World) companionsBackLocked(owner *NPC) {
	for _, id := range w.companions[owner.ServerID] {
		if p := w.NPCs[id]; p != nil && p.Dead && (p.withMaster || !p.CorpseGone.IsZero()) && companionFighter(p) {
			w.respawnNPCLocked(p)
		}
	}
}

// companionsFollowLocked walks an owner's pets with it as it wanders (the
// owner, 2026-11-11: "there are many pets not connected to their master"): a
// Companion Spirit stood on its spawn while its hobgoblin oracle wandered up
// to 70 off, 26 pairs of the 178 so. Each living pet of its own, in its
// world, out of any fight, not rooted, makes for its place beside the owner
// (its spawn's offset from the owner's, at most companionFollowAt), at the
// owner's pace or half as fast again once companionCatchUp behind, and stands
// there. Called after each step of the owner's wander (roam.go); a pet does
// not wander of itself (movement.go's whyCompanion). Must be called with the
// lock held.
func (w *World) companionsFollowLocked(owner *NPC, now time.Time) {
	ids := w.companions[owner.ServerID]
	if len(ids) == 0 || !companionFighter(owner) {
		return
	}
	for _, id := range ids {
		pet := w.NPCs[id]
		if pet == nil || !companionFighter(pet) {
			continue
		}
		dt := float32(0.05)
		if !pet.roam.last.IsZero() {
			dt = min(float32(now.Sub(pet.roam.last).Seconds()), 0.5) // a stalled tick does not jump it
		}
		pet.roam.last = now
		if pet.Dead || pet.World != owner.World || pet.CurrentEnemy != 0 || len(pet.Threats) > 0 ||
			pet.chase.on || pet.chase.returning || pet.DummyTarget != 0 || pet.rooted(now) || pet.hiddenAt(now) {
			continue // its fight, its way home, its root: its own
		}
		ox, oz := pet.SpawnX-owner.SpawnX, pet.SpawnZ-owner.SpawnZ
		if l := float32(math.Sqrt(float64(ox*ox + oz*oz))); l > companionFollowAt {
			ox, oz = ox*companionFollowAt/l, oz*companionFollowAt/l
		}
		x, z := owner.X+ox, owner.Z+oz
		d := distXZ(pet.X, pet.Z, x, z)
		if d < 0.5 {
			if pet.moving() {
				pet.stopMoving()
			}
			continue
		}
		speed := NPCWalkSpeed * owner.snareFactor(now)
		if d > companionCatchUp {
			speed *= 1.5
		}
		speed *= pet.snareFactor(now)
		w.stepNPCToward(pet, x, z, speed, dt)
	}
}
