package entity

import (
	"testing"
	"time"
)

// The owner's report of 2026-11-09 ("the mobs that have pets dont protect them
// or attack they are treated as not pets"), against a dshinn necromancer and its
// spectral minion 1.4 off it, as eqoaclassic's rows stand: a griffawn 2.5 off,
// no pet, is the control in every part, and a merchant (the NPC bit) at 0.5
// is never an owner.
func companionWorld(t *testing.T) (w *World, owner, pet, other *NPC, p *Player) {
	t.Helper()
	w = NewWorld()
	add := func(n *NPC) *NPC {
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		n.HP, n.MaxHP = 1000, 1000
		w.NPCs[n.ServerID] = n
		w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
		return n
	}
	owner = add(&NPC{ServerID: 4001, Name: "dshinn necromancer", Level: 43, X: 100, Z: 100})
	pet = add(&NPC{ServerID: 4002, Name: "spectral minion", Level: 39, X: 101.4, Z: 100})
	other = add(&NPC{ServerID: 4003, Name: "griffawn", Level: 33, X: 100, Z: 102.5})
	add(&NPC{ServerID: 4004, Name: "Merchant Biba", Level: 30, NPCType: 0x80, X: 101.4, Z: 100.5})
	p = &Player{ServerID: 1001, Name: "Hitter", Level: 50, HP: 900, MaxHP: 900, X: 110, Z: 100}
	w.Players[p.ServerID] = p
	return w, owner, pet, other, p
}

func TestACreatureAndItsPetFightAsOne(t *testing.T) {
	// a hit on the owner brings its pet; not the griffawn beside them
	w, owner, pet, other, p := companionWorld(t)
	w.mu.Lock()
	w.updateAggro(owner, p.ServerID, 50)
	w.mu.Unlock()
	if pet.Threats[p.ServerID] == 0 || pet.CurrentEnemy != p.ServerID {
		t.Errorf("the necromancer hit: its minion's threats %v, enemy %d", pet.Threats, pet.CurrentEnemy)
	}
	if len(other.Threats) != 0 {
		t.Errorf("the control: the griffawn joined (%v)", other.Threats)
	}
	if got := w.companionOf[pet.ServerID]; got != owner.ServerID || len(w.companionOf) != 1 {
		t.Errorf("pairs %v, want the minion the necromancer's alone (not the merchant's, the griffawn's)", w.companionOf)
	}

	// a hit on the pet brings its owner
	w, owner, pet, other, p = companionWorld(t)
	w.mu.Lock()
	w.updateAggro(pet, p.ServerID, 50)
	w.mu.Unlock()
	if owner.Threats[p.ServerID] == 0 || len(other.Threats) != 0 {
		t.Errorf("the minion hit: the necromancer's threats %v, the griffawn's %v", owner.Threats, other.Threats)
	}

	// a player's pet's hit sends them at its owner
	w, owner, pet, _, p = companionWorld(t)
	ppet := &NPC{ServerID: 5001, Name: "Fire Elemental", Level: 40, Owner: p.ServerID, HP: 1, MaxHP: 1, X: 105, Z: 100}
	w.NPCs[ppet.ServerID] = ppet
	w.mu.Lock()
	w.updateAggro(owner, ppet.ServerID, 50)
	w.mu.Unlock()
	if pet.Threats[p.ServerID] == 0 {
		t.Errorf("a player's pet's hit: the minion's threats %v, want its owner's", pet.Threats)
	}

	// the owner gives up: its pet goes home with it; the griffawn keeps its fight
	w, owner, pet, other, p = companionWorld(t)
	w.mu.Lock()
	w.updateAggro(owner, p.ServerID, 50)
	w.addThreatLocked(other, p.ServerID, 5)
	w.giveUpLocked(owner)
	w.mu.Unlock()
	if len(pet.Threats) != 0 || pet.CurrentEnemy != 0 || other.CurrentEnemy != p.ServerID {
		t.Errorf("given up: the minion's threats %v (enemy %d), the griffawn's enemy %d", pet.Threats, pet.CurrentEnemy, other.CurrentEnemy)
	}

	// the owner dies: its pet dies with it where it stands (masterdeath.go: its
	// death played, its body for petDismissedLies, a corpse of nothing; until
	// 2026-11-10 gone at once with no death seen); back with it; the griffawn
	// untouched
	w, owner, pet, other, _ = companionWorld(t)
	w.mu.Lock()
	w.linkCompanionsLocked()
	w.killNPC(owner)
	now := time.Now()
	died := pet.Dead && pet.HP == 0 && pet.CorpseOfNothing() && !other.Dead
	lies, gone := pet.corpseLies(now, w.corpseDecay), !pet.corpseLies(now.Add(petDismissedLies+time.Second), w.corpseDecay)
	w.respawnNPCLocked(owner)
	w.mu.Unlock()
	if !died || !lies || !gone || pet.Dead || pet.CorpseOfNothing() {
		t.Errorf("the necromancer's death: minion dead of nothing %v, lying %v and gone after %v %v, griffawn alive %v; back with it: dead %v, nothing %v",
			died, lies, petDismissedLies, gone, !other.Dead, pet.Dead, pet.CorpseOfNothing())
	}
}

// Never paired: a player's pet or a charmed creature named as a pet, a pet with
// the NPC bit; a name that names its owner takes it over a nearer creature (the
// rows' "Goran's pet", 4.9 off Goran's place in the control below, 1 off a
// serpent guardian).
func TestWhoIsACreaturesPet(t *testing.T) {
	w, _, _, _, p := companionWorld(t)
	add := func(n *NPC) *NPC {
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		w.NPCs[n.ServerID] = n
		return n
	}
	theirs := add(&NPC{ServerID: 4101, Name: "decaying minion", Owner: p.ServerID, X: 100.5, Z: 100})
	charmed := add(&NPC{ServerID: 4102, Name: "undead minion", Owner: p.ServerID, Charm: &charmHold{}, X: 100.2, Z: 100})
	quest := add(&NPC{ServerID: 4103, Name: "village pet", NPCType: 0x80, X: 100.1, Z: 100})
	goran := add(&NPC{ServerID: 4104, Name: "Goran", X: 300, Z: 300})
	add(&NPC{ServerID: 4105, Name: "serpent guardian", X: 304.9, Z: 301})
	gpet := add(&NPC{ServerID: 4106, Name: "Goran's pet", X: 304.9, Z: 300})
	w.mu.Lock()
	w.linkCompanionsLocked()
	w.mu.Unlock()
	for _, n := range []*NPC{theirs, charmed, quest} {
		if _, ok := w.companionOf[n.ServerID]; ok {
			t.Errorf("%q (owner %d, flags %#x) paired", n.Name, n.Owner, n.NPCType)
		}
	}
	if w.companionOf[gpet.ServerID] != goran.ServerID {
		t.Errorf("Goran's pet is %d's, want Goran's %d", w.companionOf[gpet.ServerID], goran.ServerID)
	}
	for name, want := range map[string]bool{"spectral minion": true, "Companion Spirit": true, "Minion of Krrgar": true,
		"Servant of Vorin": true, "maiden's servant": true, "griffawn": false, "Trumpeter": false, "Petal sprite": false, "dark servant": false, "Moogie's Servant": true} {
		if companionName(name) != want {
			t.Errorf("companionName(%q) = %v", name, !want)
		}
	}
}

// The owner, 2026-11-11: "there are many pets not connected to their master".
// A creature that wanders takes its pet with it (a hobgoblin oracle and its
// Companion Spirit 1.5 off it, as Sandstorm's census saw the oracles walk):
// the pet does not wander of itself, keeps within reach of its owner the
// whole walk, and stands beside it at each rest; a pet in a fight stays in it.
func TestACreaturesPetWalksWithIt(t *testing.T) {
	w := roamWorld(t)
	owner := addNPC(w, 100, "wolf", 1005, 1000, creatureModel, 0)
	pet := addNPC(w, 101, "Companion Spirit", 1006.5, 1000, creatureModel, 0)
	w.linkCompanionsLocked()
	if ok, why, _ := w.movementLocked(pet); ok || why != whyCompanion {
		t.Fatalf("the spirit's movement: %v, %q", ok, MoveWhyText(why, ""))
	}
	var far, rested float32
	var restedN int
	run(w, time.Now(), 20*time.Minute, func(now time.Time) {
		d := distXZ(pet.X, pet.Z, owner.X, owner.Z)
		far = max(far, d)
		if !owner.moving() && !pet.moving() {
			rested = max(rested, d)
			restedN++
		}
	})
	if moved := distXZ(owner.X, owner.Z, owner.SpawnX, owner.SpawnZ); moved < 5 && far < 1 {
		t.Fatalf("the wolf never wandered (%.1f off its spawn)", moved)
	}
	if far > companionFollowAt+2 || rested > companionFollowAt+0.5 || restedN == 0 {
		t.Errorf("the spirit: at most %.1f off the wolf walking, %.1f standing (%d ticks); want within %v", far, rested, restedN, companionFollowAt)
	}
	t.Logf("the spirit at most %.2f off the wolf on the walk, %.2f at rest; the wolf %.1f off its spawn", far, rested,
		distXZ(owner.X, owner.Z, owner.SpawnX, owner.SpawnZ))

	// in a fight it is the fight's
	pet.CurrentEnemy = 1
	px, pz := pet.X, pet.Z
	owner.X += 20
	w.companionsFollowLocked(owner, time.Now().Add(time.Hour))
	if pet.X != px || pet.Z != pz {
		t.Errorf("the spirit in a fight followed its owner")
	}
}

// The pets the log counted with no owner (2026-11-11, eqoaclassic's rows):
// a servant names its owner by a word of its name, punctuation aside ("Servant
// of Vorin" by High Priest Vorin 31.8 off, "Servant of Tal Thir" by Tal'Thir
// 36.2 off, "maiden's servant" by the Dawn Maiden 5.8 off), and a kind a pet's
// word names that is no one's pet is none (Zaoran Plateau's Blood Thirsty
// Minions, a village pet).
func TestAPetsNameFindsItsOwner(t *testing.T) {
	for _, c := range []struct {
		named, owner string
		want         bool
	}{{"Vorin", "High Priest Vorin", true}, {"Tal Thir", "Tal'Thir", true}, {"maiden", "Dawn Maiden", true},
		{"Vhi'zan", "Vhi'zan Kaah", true}, {"Goran", "Goran", true}, {"Vorin", "Vorinson", false}, {"Tal", "Tal'Thir", true},
		{"Thir", "Tal'Thir", true}, {"Krrgar", "clan kolbok mystic", false}, {"", "Goran", false}} {
		if got := companionNamesOwner(c.named, c.owner); got != c.want {
			t.Errorf("companionNamesOwner(%q, %q) = %v", c.named, c.owner, got)
		}
	}
	w := NewWorld()
	add := func(id EntityID, name string, x, z float32) *NPC {
		n := &NPC{ServerID: id, Name: name, X: x, Z: z, SpawnX: x, SpawnZ: z}
		w.NPCs[id] = n
		return n
	}
	vorin := add(1, "High Priest Vorin", 5375, 9350)
	add(2, "anointed skeleton", 5375, 9346) // 4.1 off the servant, as on the rows
	servant := add(3, "Servant of Vorin", 5375-31.8, 9350)
	maiden := add(4, "Dawn Maiden", 8434, 5171)
	mserv := add(5, "maiden's servant", 8434+5.8, 5171)
	tal := add(6, "Tal'Thir", 5162, 9158+36.2)
	tserv := add(7, "Servant of Tal Thir", 5162, 9158)
	minion := add(8, "Blood Thirsty Minion", 9000, 9000)
	add(9, "Blood Thirsty Minion", 9001, 9000)
	w.linkCompanionsLocked()
	for pet, want := range map[*NPC]*NPC{servant: vorin, mserv: maiden, tserv: tal} {
		if w.companionOf[pet.ServerID] != want.ServerID {
			t.Errorf("%s is %d's, want %s's", pet.Name, w.companionOf[pet.ServerID], want.Name)
		}
	}
	if companionName(minion.Name) || companionName("village pet") || len(w.companionOf) != 3 {
		t.Errorf("Blood Thirsty Minions as pets: %v %v", companionName(minion.Name), w.companionOf)
	}
}

// Petition 1832 (Waldorf, 2026-11-12): "Same with the Necs and their pets". Guk's Mummified Guardian is a
// necromancer's though its name says no pet: it finds the necromancer 6.8 off (the rows' 166145) over a wizard
// 3.5 off, and one at 1.5 off a "necromaner" (the rows' misspelt kind) still finds that one, the nearest.
func TestAMummifiedGuardianIsANecromancersPet(t *testing.T) {
	w, _, _, _, _ := companionWorld(t)
	add := func(n *NPC) *NPC {
		n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
		w.NPCs[n.ServerID] = n
		return n
	}
	nec := add(&NPC{ServerID: 4201, Name: "froglok tal necromancer", X: 500, Z: 500})
	add(&NPC{ServerID: 4202, Name: "froglok urd wizard", X: 506.8, Z: 503.5})
	g := add(&NPC{ServerID: 4203, Name: "Mummified Guardian", X: 506.8, Z: 500})
	typo := add(&NPC{ServerID: 4204, Name: "ghoul tal necromaner", X: 700, Z: 700})
	g2 := add(&NPC{ServerID: 4205, Name: "Mummified Guardian", X: 701.5, Z: 700})
	w.mu.Lock()
	w.linkCompanionsLocked()
	w.mu.Unlock()
	if w.companionOf[g.ServerID] != nec.ServerID {
		t.Errorf("the guardian is %d's, want the necromancer's %d", w.companionOf[g.ServerID], nec.ServerID)
	}
	if w.companionOf[g2.ServerID] != typo.ServerID {
		t.Errorf("the second guardian is %d's, want the necromaner's %d", w.companionOf[g2.ServerID], typo.ServerID)
	}
	if !companionName("mummified guardian") || companionName("serpent guardian") {
		t.Error("companionName of the guardians")
	}
}
