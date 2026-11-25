package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// A Freezeblood mystic (petition 2977's: its Kindred Spirits stood 4.2 and 20 off it, never paired) and the
// template row its pets are copies of, held out of the world; a griffawn beside it summons nothing.
func creaturePetsWorld(t *testing.T, rows ...database.NPCPetRow) (w *World, owner, tmpl, other *NPC) {
	t.Helper()
	w = NewWorld()
	add := func(n *NPC) *NPC {
		n.SpawnX, n.SpawnY, n.SpawnZ, n.SpawnFacing = n.X, n.Y, n.Z, n.Facing
		n.HP, n.MaxHP = 1000, 1000
		w.mu.Lock()
		w.putNPCLocked(n)
		w.mu.Unlock()
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
		return n
	}
	tmpl = add(&NPC{ServerID: 113493, Name: "Kindred Spirit", Level: 21, NPCType: 2, X: 900, Y: 10, Z: 900, SpawnOnly: true})
	owner = add(&NPC{ServerID: 113449, Name: "Freezeblood mystic", Level: 24, NPCType: 2, X: 100, Y: 5, Z: 100})
	other = add(&NPC{ServerID: 113450, Name: "griffawn", Level: 24, NPCType: 2, X: 104, Y: 5, Z: 100})
	if len(rows) == 0 {
		rows = []database.NPCPetRow{{Owner: "freezeblood mystic", Pet: "kindred spirit", TemplateRow: 113493, Count: 1, LevelOffset: -3}}
	}
	w.SetCreaturePets(rows)
	return w, owner, tmpl, other
}

// sweep runs the system as the tick does, at now.
func (w *World) sweepCreaturePets(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cpets.last = time.Time{}
	w.creaturePetsSystem(now)
}

func (w *World) creaturePetsOf(owner *NPC) []*NPC {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []*NPC
	for _, n := range w.NPCs {
		if n.petOf == owner.ServerID {
			out = append(out, n)
		}
	}
	return out
}

func TestACreatureSummonsItsPetBesideIt(t *testing.T) {
	w, owner, tmpl, other := creaturePetsWorld(t)
	now := time.Now()
	w.sweepCreaturePets(now)
	pets := w.creaturePetsOf(owner)
	if len(pets) != 1 {
		t.Fatalf("the mystic's pets: %d, want 1", len(pets))
	}
	p := pets[0]
	if d := distXZ(p.X, p.Z, owner.X, owner.Z); d > 2 || p.Y != owner.Y {
		t.Errorf("its pet stands %.2f off it at height %.1f, want beside it (%.1f) on its floor", d, p.Y, creaturePetBeside)
	}
	if d := distXZ(p.SpawnX, p.SpawnZ, owner.SpawnX, owner.SpawnZ); d > 2 {
		t.Errorf("its pet's spawn %.2f off its owner's", d)
	}
	if p.Name != tmpl.Name || !p.Temporary || p.CopyOf != tmpl.ServerID || p.SpawnOnly || p.Level != 21 {
		t.Errorf("the pet: %q temporary %v copy of %d spawn-only %v level %d; want a copy of the template, in the world, level 24-3",
			p.Name, p.Temporary, p.CopyOf, p.SpawnOnly, p.Level)
	}
	if p.MaxHP != w.balance.NPCMaxHP(21) {
		t.Errorf("the pet's hit points %d, want its level's %d", p.MaxHP, w.balance.NPCMaxHP(21))
	}
	if len(w.creaturePetsOf(other)) != 0 || len(w.creaturePetsOf(tmpl)) != 0 {
		t.Errorf("the griffawn or the template summoned a pet")
	}
	w.mu.Lock()
	w.linkCompanionsLocked()
	paired := w.companionOf[p.ServerID] == owner.ServerID
	w.mu.Unlock()
	if !paired {
		t.Errorf("the pet is not paired with its owner: %v", w.companionOf)
	}
	// once a life: the next sweeps make no more
	w.sweepCreaturePets(now.Add(2 * time.Second))
	w.sweepCreaturePets(now.Add(4 * time.Second))
	if n := len(w.creaturePetsOf(owner)); n != 1 {
		t.Errorf("after more sweeps: %d pets", n)
	}
	// the keep restart and the copies' corpses keep none
	w.mu.Lock()
	p.Dead, p.DeathTime = true, now
	keeps := w.keepsCopyCorpse(p, now)
	w.mu.Unlock()
	if keeps {
		t.Errorf("a summoned pet's corpse is carried over a restart")
	}
}

func TestACreaturesPetFightsAndDiesWithIt(t *testing.T) {
	w, owner, _, _ := creaturePetsWorld(t)
	w.sweepCreaturePets(time.Now())
	pet := w.creaturePetsOf(owner)[0]
	pl := &Player{ServerID: 1001, Name: "Hitter", Level: 30, HP: 900, MaxHP: 900, X: 110, Z: 100}
	w.Players[pl.ServerID] = pl
	w.mu.Lock()
	w.updateAggro(owner, pl.ServerID, 50)
	joined := pet.Threats[pl.ServerID] > 0
	w.killNPC(owner)
	died := pet.Dead
	w.mu.Unlock()
	if !joined || !died {
		t.Errorf("the mystic hit: its pet joined %v; the mystic killed: its pet dead %v", joined, died)
	}
}

// A pet killed alone comes back with its owner's next spawn, not on its own.
func TestAPetKilledAloneComesBackWithItsOwner(t *testing.T) {
	w, owner, _, _ := creaturePetsWorld(t)
	now := time.Now()
	w.sweepCreaturePets(now)
	pet := w.creaturePetsOf(owner)[0]
	w.mu.Lock()
	w.killNPC(pet)
	w.removeNPCLocked(pet) // its corpse gone: a copy leaves (corpse.go)
	w.mu.Unlock()
	w.sweepCreaturePets(now.Add(2 * time.Second))
	if n := len(w.creaturePetsOf(owner)); n != 0 {
		t.Fatalf("its owner alive: %d pets back on their own", n)
	}
	w.mu.Lock()
	w.killNPC(owner)
	w.respawnNPCLocked(owner)
	w.mu.Unlock()
	pets := w.creaturePetsOf(owner)
	if len(pets) != 1 || pets[0].Dead {
		t.Errorf("its owner back: %d pets, want one alive at once", len(pets))
	}
}

// Its owner leaving the world takes its pets; out of view (spawn-only, its hours) too, until it is back.
func TestCreaturePetsLeaveWithTheirOwner(t *testing.T) {
	w, owner, _, _ := creaturePetsWorld(t)
	now := time.Now()
	w.sweepCreaturePets(now)
	w.mu.Lock()
	owner.offHours = true
	w.mu.Unlock()
	w.sweepCreaturePets(now.Add(2 * time.Second))
	if n := len(w.creaturePetsOf(owner)); n != 0 {
		t.Errorf("its owner out of its hours: %d pets stand", n)
	}
	w.mu.Lock()
	owner.offHours = false
	w.mu.Unlock()
	w.sweepCreaturePets(now.Add(4 * time.Second))
	if n := len(w.creaturePetsOf(owner)); n != 1 {
		t.Errorf("its owner back in its hours: %d pets", n)
	}
	w.mu.Lock()
	w.removeNPCLocked(owner)
	w.mu.Unlock()
	if n := len(w.creaturePetsOf(owner)); n != 0 {
		t.Errorf("its owner gone: %d pets stand", n)
	}
}

// One creature's row ("#n") is ahead of its name's; a rule's count; a rule changed takes the old rule's pets away.
func TestCreaturePetsByRowAndCount(t *testing.T) {
	w, owner, _, other := creaturePetsWorld(t,
		database.NPCPetRow{Owner: "freezeblood mystic", Pet: "kindred spirit", TemplateRow: 113493, Count: 1},
		database.NPCPetRow{Owner: "#113450", Pet: "kindred spirit", TemplateRow: 113493, Count: 2, LevelOffset: 1})
	now := time.Now()
	w.sweepCreaturePets(now)
	if a, b := len(w.creaturePetsOf(owner)), len(w.creaturePetsOf(other)); a != 1 || b != 2 {
		t.Errorf("pets: the mystic %d (want 1), the griffawn by its row %d (want 2)", a, b)
	}
	for _, p := range w.creaturePetsOf(other) {
		if p.Level != 25 {
			t.Errorf("the griffawn's pet's level %d, want 24+1", p.Level)
		}
	}
	w.SetCreaturePets([]database.NPCPetRow{{Owner: "#113450", Pet: "kindred spirit", TemplateRow: 113493, Count: 1}})
	w.sweepCreaturePets(now.Add(2 * time.Second))
	if a, b := len(w.creaturePetsOf(owner)), len(w.creaturePetsOf(other)); a != 0 || b != 1 {
		t.Errorf("the rules changed: the mystic %d pets (want 0), the griffawn %d (want 1)", a, b)
	}
}

// A GM's #spawn of an owner brings its pets; its #despawn takes them.
func TestAGMSpawnOfAnOwnerBringsItsPets(t *testing.T) {
	w, owner, tmpl, _ := creaturePetsWorld(t)
	id, _, pets, ok := w.SpawnGMCopy(owner.Name, 500, 5, 500, 0, 0)
	if !ok || len(pets) != 1 {
		t.Fatalf("#spawn: ok %v, its companions %v, want its one pet", ok, pets)
	}
	w.mu.Lock()
	p := w.NPCs[pets[0]]
	w.mu.Unlock()
	if p == nil || p.petOf != id || p.CopyOf != tmpl.ServerID || distXZ(p.X, p.Z, 500, 500) > 2 {
		t.Fatalf("the copy's pet: %+v", p)
	}
	if _, ok := w.DespawnNPC(id); !ok {
		t.Fatal("#despawn refused")
	}
	w.mu.Lock()
	gone := w.NPCs[pets[0]] == nil
	w.mu.Unlock()
	if !gone {
		t.Errorf("the copy despawned: its pet stands")
	}
}
