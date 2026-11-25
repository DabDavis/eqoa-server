package entity

// creaturepets.go — creatures summon their own pets (the owner, 2026-11-25: "any mob with a pet should summon their
// own pets so dont need any pets placed").
//
// Until now a creature's pet was a row of npcs of its own, standing where a capture caught it, paired with the
// creature beside it at the spawns (companion.go). Placement drift broke the pairs: Heirloom petition 2977's Kindred
// Spirits stood 4.2 and 20 off their Freezeblood mystic and were no one's. Now the pets are table npcpets's, by the
// owner (database/npcpets.go): its name (NPCClassKey) or one creature's row ("#n", ahead of its name's; a copy of the
// row too), each a copy of a template row held out of the world (npcspawnonly "#n"), so many of it, its level its
// owner's and the row's offset. tools/creature_pets_20261125.py moved the placed pets there.
//
// A creature's pets are made when it is in the world alive, at each of its spawns: its first, each respawn
// (respawnNPCLocked), a copy's (a dungeon instance's, a GM's #spawn, a quest's spawn), the world loaded or a keep
// restart's creatures restored (the system's sweep, creaturePetsSystem, once a second). Each stands beside it (its
// place round it a pet's own, companionFollowAt's), on the ground's floor there, its spawn as far off its owner's;
// a copy (Temporary, CopyOf its template) whose petOf is its owner, paired with it as companion.go's pairs are
// (linkCompanionsLocked): they fight as one, walk together, go home together, and its owner's death lays it down
// dead with it (masterdeath.go). A pet killed alone leaves with its corpse (a copy does not respawn) and comes back
// with its owner's next spawn, not on its own, as companion.go's pets came back. Its owner leaving the world (a copy
// gone, a GM's #despawn, a row deleted, an instance closed) takes it with it (dropNPCLocked); its owner out of view
// (spawn-only, outside its hours, a ward down) takes it too, until it is back. A keep restart keeps none of them:
// its owner, kept, makes them again where it stands (keepnpc.go, keepdungeon.go, keepgmspawn.go keep no pet).

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/eqoa/server/internal/database"
)

// creaturePetBeside is how far off its owner a pet is made, creaturePetEvery how often the sweep looks.
// creaturePetsMost is the most of one pet a rule makes.
const (
	creaturePetBeside float32 = 1.5
	creaturePetEvery          = time.Second
	creaturePetsMost          = 8
)

// creaturePetRule is one pet of an owner: a copy of the template row, count of them, at the owner's level and
// offset (at least 1).
type creaturePetRule struct {
	Template EntityID
	Count    int
	Offset   int
}

// creaturePetsState is the rules and what was made of them.
type creaturePetsState struct {
	byName    map[string][]creaturePetRule   // by the owner's name's NPCClassKey
	byRow     map[EntityID][]creaturePetRule // by its row ("#n"), ahead of its name's
	templates map[EntityID]bool              // the template rows (no dungeon instance copies them)
	owners    map[EntityID]*creaturePetOwner
	// pending are the NPCs put in the world since the last sweep whose rules want pets (putNPCLocked), scanAll that
	// the next sweep looks at every NPC (the rules read)
	pending []EntityID
	scanAll bool
	last    time.Time
}

// creaturePetOwner is an owner's pets now, and the life they were made for.
type creaturePetOwner struct {
	pets []EntityID
	gen  uint32 // its spawnGen when they were made
	made bool
}

// SetCreaturePets takes table npcpets's rows (database.NPCPets) as the rules. The pets made so far stay; the sweep
// takes away those no rule wants and makes those wanted, at the owners' next spawns (an owner standing now gets its
// new rule's at once, the pets its old one made gone). It reports the owners and the pets they summon.
func (w *World) SetCreaturePets(rows []database.NPCPetRow) (owners, pets int) {
	byName, byRow := map[string][]creaturePetRule{}, map[EntityID][]creaturePetRule{}
	templates := map[EntityID]bool{}
	for _, r := range rows {
		if r.Owner == "" || r.TemplateRow == 0 || r.Count <= 0 {
			continue
		}
		rule := creaturePetRule{Template: EntityID(r.TemplateRow), Count: min(r.Count, creaturePetsMost), Offset: r.LevelOffset}
		if id, ok := ParseRowKey(r.Owner); ok {
			byRow[id] = append(byRow[id], rule)
		} else {
			byName[r.Owner] = append(byName[r.Owner], rule)
		}
		templates[rule.Template] = true
		pets += rule.Count
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cpets.byName, w.cpets.byRow, w.cpets.templates = byName, byRow, templates
	for _, st := range w.cpets.owners {
		st.made = false // looked at again: a rule changed under it
	}
	w.cpets.scanAll, w.cpets.last = true, time.Time{} // the next tick's sweep, over every NPC
	return len(byName) + len(byRow), pets
}

// LoadCreaturePets reads table npcpets into the world: at the start and after a GM's edit of it.
func (w *World) LoadCreaturePets(db *database.SQLiteDB) (owners, pets int, err error) {
	rows, err := db.NPCPets()
	if err != nil {
		return 0, 0, fmt.Errorf("npcpets: %w", err)
	}
	owners, pets = w.SetCreaturePets(rows)
	log.Printf("Creature pets: %d owners summon %d pets (npcpets)", owners, pets)
	return owners, pets, nil
}

// creaturePetRulesLocked is the pets owner summons: its row's ("#n", a copy's row's) ahead of its name's; none for
// a pet of any kind, a player's or charmed creature, one with the NPC bit or a dummy. Must be called with the lock
// held.
func (w *World) creaturePetRulesLocked(owner *NPC) []creaturePetRule {
	if len(w.cpets.byName)+len(w.cpets.byRow) == 0 || owner == nil || owner.petOf != 0 || owner.Owner != 0 || owner.NPCType&0x80 != 0 ||
		owner.Dummy != NotDummy || owner.GenieOf != 0 || owner.PortalOf != 0 || w.cpets.templates[owner.ServerID] {
		return nil
	}
	if r := owner.row(); r != 0 && len(w.cpets.byRow) > 0 {
		if rules, ok := w.cpets.byRow[r]; ok {
			return rules
		}
	}
	return w.cpets.byName[owner.classKey()]
}

// IsCreaturePet reports an NPC an owner summoned (this file's), and its owner.
func (w *World) IsCreaturePet(id EntityID) (EntityID, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if n := w.NPCs[id]; n != nil && n.petOf != 0 {
		return n.petOf, true
	}
	return 0, false
}

// creaturePetsOfLocked is owner's pets in the world now (its list tidied). Must be called with the lock held.
func (w *World) creaturePetsOfLocked(owner EntityID) []*NPC {
	st := w.cpets.owners[owner]
	if st == nil {
		return nil
	}
	var out []*NPC
	keep := st.pets[:0]
	for _, id := range st.pets {
		if p := w.NPCs[id]; p != nil && p.petOf == owner {
			out = append(out, p)
			keep = append(keep, id)
		}
	}
	st.pets = keep
	return out
}

// creaturePetsForLocked makes owner's pets when it is alive in the world and its life has not had them yet, takes
// away those no rule of it wants (and all of them while it stands out of the world), and returns those it made.
// Must be called with the lock held.
func (w *World) creaturePetsForLocked(owner *NPC, now time.Time) []*NPC {
	if owner == nil {
		return nil
	}
	rules := w.creaturePetRulesLocked(owner)
	st := w.cpets.owners[owner.ServerID]
	if st == nil {
		if len(rules) == 0 {
			return nil
		}
		if w.cpets.owners == nil {
			w.cpets.owners = map[EntityID]*creaturePetOwner{}
		}
		st = &creaturePetOwner{}
		w.cpets.owners[owner.ServerID] = st
	}
	have := w.creaturePetsOfLocked(owner.ServerID)
	// out of the world, or about to be (one back outside its hours: corpseSystem holds it after respawnNPCLocked)
	away := owner.SpawnOnly || owner.hiddenAt(now) || w.offHoursLocked(owner, now)
	// the ones no rule wants (or every one, its owner out of the world)
	want := map[EntityID]int{}
	for _, r := range rules {
		want[r.Template] += r.Count
	}
	alive := map[EntityID]int{}
	for _, p := range have {
		if away || want[p.CopyOf] == 0 || !p.Dead && alive[p.CopyOf] >= want[p.CopyOf] {
			w.removeCreaturePetLocked(p)
			continue
		}
		if !p.Dead {
			alive[p.CopyOf]++
		}
	}
	if away {
		st.made = false // made again when it is back
	}
	if len(rules) == 0 {
		if len(st.pets) == 0 {
			delete(w.cpets.owners, owner.ServerID)
		}
		return nil
	}
	if away || owner.Dead || st.made && st.gen == owner.spawnGen || owner.Charm != nil {
		return nil
	}
	st.made, st.gen = true, owner.spawnGen
	var made []*NPC
	slot := len(have)
	for _, r := range rules {
		t := w.NPCs[r.Template]
		if t == nil {
			continue // its template row gone: none (a GM's edit)
		}
		for i := alive[r.Template]; i < r.Count; i++ {
			made = append(made, w.makeCreaturePetLocked(owner, t, r.Offset, slot))
			slot++
		}
	}
	if len(made) > 0 {
		w.companionOf = nil // the pairs read again (companion.go)
	}
	return made
}

// makeCreaturePetLocked makes one pet of owner from template t, the slot'th round it. Must be called with the lock
// held.
func (w *World) makeCreaturePetLocked(owner, t *NPC, offset, slot int) *NPC {
	// beside it, to its left, then its right, then further round
	a := float64(owner.Facing) + math.Pi/2 + float64(slot)*2*math.Pi/5
	if slot%2 == 1 {
		a = float64(owner.Facing) - math.Pi/2 - float64(slot-1)*math.Pi/5
	}
	ox, oz := creaturePetBeside*float32(math.Cos(a)), creaturePetBeside*float32(math.Sin(a))
	x, z := owner.X+ox, owner.Z+oz
	y := w.creaturePetFloorLocked(owner.World, x, owner.Y, z)
	c := w.spawnCopyAsLocked(t, x, y, z, owner.Facing, owner.World, 0)
	// its spawn as far off its owner's (companionsFollowLocked keeps that place by it as it walks)
	c.SpawnX, c.SpawnZ = owner.SpawnX+ox, owner.SpawnZ+oz
	c.SpawnY = w.creaturePetFloorLocked(owner.World, c.SpawnX, owner.SpawnY, c.SpawnZ)
	c.SpawnFacing = owner.SpawnFacing
	c.Instance = owner.Instance
	c.petOf = owner.ServerID
	c.RespawnTime = owner.RespawnTime
	c.EventAggro = owner.EventAggro
	w.setCreaturePetLevelLocked(c, max(1, owner.Level+offset))
	st := w.cpets.owners[owner.ServerID]
	st.pets = append(st.pets, c.ServerID)
	if in := w.dungeons.insts[owner.Instance]; owner.Instance != 0 && owner.World == w.dungeons.world && in != nil {
		in.npcs = append(in.npcs, c.ServerID) // the instance's, closed with it (dungeons.go)
	}
	return c
}

// creaturePetFloorLocked is the ground's floor at (x, z) near height y, or y when none is near.
func (w *World) creaturePetFloorLocked(world uint8, x, y, z float32) float32 {
	if w.ground != nil {
		if fy, ok := w.ground.Floor(world, x, y+3, z); ok && fy-y <= triggerFloorReach && y-fy <= triggerFloorReach {
			return fy
		}
	}
	return y
}

// setCreaturePetLevelLocked gives a pet its level: its combat figures and hit points as a row of that level's
// (fromRecord's, applyBalanceLocked's). Must be called with the lock held.
func (w *World) setCreaturePetLevelLocked(c *NPC, level int) {
	c.Level = level
	c.OffMod, c.DefMod, c.WeaponDamage = NPCOffMod(level), NPCDefMod(level), NPCWeaponDamage(level)
	w.applyBalanceLocked(c)
}

// removeCreaturePetLocked takes a pet out of the world at once. Must be called with the lock held.
func (w *World) removeCreaturePetLocked(p *NPC) {
	w.removeNPCLocked(p)
	w.companionOf = nil
}

// creaturePetsGoneLocked is dropNPCLocked's: a pet's owner forgets it, an owner's pets leave with it. Must be
// called with the lock held.
func (w *World) creaturePetsGoneLocked(npc *NPC) {
	if npc.petOf != 0 {
		return // its owner's list is tidied as it is read (creaturePetsOfLocked)
	}
	st := w.cpets.owners[npc.ServerID]
	if st == nil {
		return
	}
	if w.NPCs[npc.ServerID] != nil {
		return // made again in its place (a GM's edit of its row): its pets are the new one's
	}
	delete(w.cpets.owners, npc.ServerID)
	for _, id := range st.pets {
		if p := w.NPCs[id]; p != nil && p.petOf == npc.ServerID {
			w.removeNPCLocked(p)
		}
	}
	w.companionOf = nil
}

// creaturePetPutLocked is putNPCLocked's: an NPC new in the world (or made again in its place) whose rules want
// pets is looked at by the next sweep. Must be called with the lock held.
func (w *World) creaturePetPutLocked(npc *NPC) {
	if npc.petOf == 0 && len(w.creaturePetRulesLocked(npc)) > 0 || w.cpets.owners[npc.ServerID] != nil {
		w.cpets.pending = append(w.cpets.pending, npc.ServerID)
	}
}

// creaturePetsSystem is the sweep (the file's doc): once a second, every owner known, every NPC put in the world
// since (and every NPC at all once the rules are read) is given its pets at a spawn it has had none for, and its
// pets are looked at. Must be called with the lock held.
func (w *World) creaturePetsSystem(now time.Time) {
	if now.Sub(w.cpets.last) < creaturePetEvery {
		return
	}
	w.cpets.last = now
	for id, st := range w.cpets.owners {
		n := w.NPCs[id]
		switch {
		case n == nil:
			w.creaturePetsGoneLocked(&NPC{ServerID: id})
		case n.SpawnOnly || n.hiddenAt(now):
			if len(st.pets) > 0 {
				w.creaturePetsForLocked(n, now) // out of the world: its pets with it
			}
		case !st.made || st.gen != n.spawnGen:
			w.creaturePetsForLocked(n, now)
		}
	}
	pending := w.cpets.pending
	w.cpets.pending = nil
	for _, id := range pending {
		if n := w.NPCs[id]; n != nil && n.petOf == 0 {
			w.creaturePetsForLocked(n, now)
		}
	}
	if !w.cpets.scanAll {
		return
	}
	w.cpets.scanAll = false
	if len(w.cpets.byName)+len(w.cpets.byRow) == 0 {
		return
	}
	for _, n := range w.NPCs {
		if n.Dead || n.petOf != 0 || w.cpets.owners[n.ServerID] != nil {
			continue
		}
		if len(w.creaturePetRulesLocked(n)) > 0 {
			w.creaturePetsForLocked(n, now)
		}
	}
}
