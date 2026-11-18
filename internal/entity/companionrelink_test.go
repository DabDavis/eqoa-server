package entity

import (
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"

)

// companionrelink_test.go — the companions' relink made cheap (2026-11-18):
// Live's roam turns spiked 300-600 ms on each relink, which every spawn,
// despawn or corpse brings. A name's answer is kept, and only the cells round
// a pet are gridded; the pairs are the same as the relink's before.

// linkCompanionsBefore is linkCompanionsLocked as it was before 2026-11-18
// (build338's: GM spawns' own pairs, the triggers' pets), returning its pairs
// (pet -> owner), each name's rule worked out afresh.
func linkCompanionsBefore(w *World) map[EntityID]EntityID {
	out := map[EntityID]EntityID{}
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
	for _, n := range w.NPCs {
		if !companionFighter(n) {
			continue
		}
		if n.GMSpawn {
			if o := w.NPCs[n.gmCompanionOf]; n.gmCompanionOf != 0 && o != nil && o.GMSpawn && companionFighter(o) {
				out[n.ServerID] = o.ServerID
			}
			continue
		}
		if pet, _, _ := w.companionPetRuleUncachedLocked(n.Name); pet {
			pets = append(pets, n)
			continue
		}
		grid[at(n)] = append(grid[at(n)], n)
	}
	for _, pet := range pets {
		_, named, ownerOnly := w.companionPetRuleUncachedLocked(pet.Name)
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
			out[pet.ServerID] = owner.ServerID
		}
	}
	return out
}

// relinkWorld is a world of n creatures scattered over a few worlds and an
// instance, and pets of every kind among them: named for an owner beside it,
// of a kind (a necromancer's guardian, a shaman's spirit), bare ("goblin
// pet"), far from anyone, a GM's copy, an instance's copy.
func relinkWorld(n int) *World {
	w := NewWorld()
	rnd := rand.New(rand.NewSource(5))
	id := EntityID(1000)
	put := func(name string, world uint8, inst uint32, x, z float32) *NPC {
		id++
		npc := &NPC{ServerID: id, Name: name, World: world, Instance: inst, X: x, Z: z, SpawnX: x, SpawnZ: z,
			ModelID: creatureModel, Level: 10, HP: 50, MaxHP: 50}
		w.NPCs[id] = npc
		return npc
	}
	kinds := []string{"orc pawn", "froglok necromancer", "gnoll shaman", "Goran", "High Priest Vorin", "decaying skeleton", "Tal'Thir",
		"Tunare", "Wrath of the Mother", "Grace of the Mother"}
	for i := 0; i < n; i++ {
		put(kinds[i%len(kinds)], uint8(i%3), uint32(i%17/16), rnd.Float32()*6000, rnd.Float32()*6000)
	}
	pets := []string{"Goran's pet", "Servant of Vorin", "mummified guardian", "elder spirit", "feral spirit", "goblin pet",
		"Minion of Tal Thir", "village pet", "Howling Death"}
	var owners []*NPC
	for _, o := range w.NPCs {
		owners = append(owners, o)
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].ServerID < owners[j].ServerID })
	for i := 0; i < n/50; i++ {
		o := owners[rnd.Intn(len(owners))]
		p := put(pets[i%len(pets)], o.World, o.Instance, o.SpawnX+rnd.Float32()*8-4, o.SpawnZ+rnd.Float32()*8-4)
		switch i % 11 {
		case 3: // far from anyone
			p.SpawnX, p.SpawnZ = 9000+float32(i), 9000
		case 5: // a GM's copy
			p.Temporary = true
		case 7: // a player's
			p.Owner = 1
		}
	}
	// GM spawns: pairs of their own spawning (some broken: an owner gone, one
	// not a GM's spawn), standing among the rows
	for i := 0; i < n/100; i++ {
		o := owners[rnd.Intn(len(owners))]
		gmOwner := put("orc pawn", o.World, o.Instance, o.SpawnX+1, o.SpawnZ+1)
		gmOwner.GMSpawn = true
		gmPet := put("goblin pet", o.World, o.Instance, o.SpawnX+2, o.SpawnZ+1)
		gmPet.GMSpawn = true
		switch i % 4 {
		case 0, 1:
			gmPet.gmCompanionOf = gmOwner.ServerID
		case 2:
			gmPet.gmCompanionOf = o.ServerID // a row's, not a GM's spawn: no pair
		case 3:
			gmPet.gmCompanionOf = 1 // gone
		}
	}
	return w
}

// sameCompanions fails unless the World's pairs are want, and each owner's
// list is its pets.
func sameCompanions(t *testing.T, w *World, want map[EntityID]EntityID) {
	t.Helper()
	if len(w.companionOf) != len(want) {
		t.Fatalf("%d pairs, before %d", len(w.companionOf), len(want))
	}
	byOwner := map[EntityID][]EntityID{}
	for pet, o := range want {
		if w.companionOf[pet] != o {
			t.Fatalf("pet %d (%s): owner %d, before %d", pet, w.NPCs[pet].Name, w.companionOf[pet], o)
		}
		byOwner[o] = append(byOwner[o], pet)
	}
	for o, ps := range byOwner {
		got := append([]EntityID(nil), w.companions[o]...)
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		sort.Slice(ps, func(i, j int) bool { return ps[i] < ps[j] })
		if fmt.Sprint(got) != fmt.Sprint(ps) {
			t.Fatalf("owner %d's pets %v, before %v", o, got, ps)
		}
	}
}

func TestTheRelinkPairsAsBefore(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	w := relinkWorld(20000)
	want := linkCompanionsBefore(w)
	if len(want) < 100 {
		t.Fatalf("only %d pairs: the world pairs too few to say", len(want))
	}
	w.linkCompanionsLocked()
	sameCompanions(t, w, want)
	gmPairs := 0
	for pet := range w.companionOf {
		if w.NPCs[pet].GMSpawn {
			gmPairs++
		}
	}
	if gmPairs == 0 {
		t.Fatal("no GM spawn paired")
	}
	// the triggers' pets: Tunare's resummons pair with her, then not, then
	// the Grace alone (each name's kept answer dropped with them)
	triggerPairs := func() int {
		n := 0
		for pet, o := range w.companionOf {
			if w.NPCs[o].Name == "Tunare" && strings.HasSuffix(w.NPCs[pet].Name, "of the Mother") {
				n++
			}
		}
		return n
	}
	for _, pets := range []map[string]string{
		{"Wrath of the Mother": "Tunare", "Grace of the Mother": "Tunare"},
		{},
		{"Grace of the Mother": "Tunare"},
	} {
		w.SetTriggerCompanions(pets)
		w.linkCompanionsLocked()
		sameCompanions(t, w, linkCompanionsBefore(w))
		if got := triggerPairs(); (len(pets) == 0) != (got == 0) {
			t.Fatalf("triggers %v: %d pairs with Tunare", pets, got)
		}
	}
	// a pet and an owner come and go: relinked, the same again
	for id, n := range w.NPCs {
		if companionName(n.Name) {
			delete(w.NPCs, id)
			break
		}
	}
	w.NPCs[999] = &NPC{ServerID: 999, Name: "goblin pet", SpawnX: 100, SpawnZ: 100, ModelID: creatureModel}
	w.NPCs[998] = &NPC{ServerID: 998, Name: "orc pawn", SpawnX: 101, SpawnZ: 100, ModelID: creatureModel}
	w.linkCompanionsLocked()
	sameCompanions(t, w, linkCompanionsBefore(w))
	if w.companionOf[999] != 998 {
		t.Fatalf("the new goblin pet's owner is %d, want the orc pawn beside it", w.companionOf[999])
	}
}

// A relink's cost follows the pets, not the world's creatures: 20,000 more
// creatures in a world with no pet add next to no allocations to it (before:
// one or more each, Live's 300-600 ms turns over its 65k NPCs).
func TestARelinkCostsThePetsNotTheWorld(t *testing.T) {
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	relinkAllocs := func(w *World) float64 {
		w.linkCompanionsLocked()
		return testing.AllocsPerRun(5, func() {
			w.companionsAt = -1 // a creature came or went
			w.linkCompanionsLocked()
		})
	}
	w := relinkWorld(20000)
	base := relinkAllocs(w)
	pairs := len(w.companionOf)
	rnd := rand.New(rand.NewSource(9))
	for i := 0; i < 20000; i++ {
		id := EntityID(500000 + i)
		x, z := rnd.Float32()*6000, rnd.Float32()*6000
		w.NPCs[id] = &NPC{ServerID: id, Name: fmt.Sprintf("orc pawn %d", i%300), World: 7, X: x, Z: z, SpawnX: x, SpawnZ: z,
			ModelID: creatureModel, Level: 10, HP: 50, MaxHP: 50}
	}
	more := relinkAllocs(w)
	if len(w.companionOf) != pairs {
		t.Fatalf("%d pairs with the creatures, %d without", len(w.companionOf), pairs)
	}
	if more-base > 100 {
		t.Fatalf("20,000 creatures away from every pet add %.0f allocations to a relink (%.0f -> %.0f), want under 100", more-base, base, more)
	}
	t.Logf("a relink: %.0f allocations, %.0f with 20,000 more creatures", base, more)
}
