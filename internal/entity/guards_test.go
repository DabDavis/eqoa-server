package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Town guards defend players (guards.go; jib's F3 20261030-063534).

// guardFix is socialFix (Freeport's militia social, Highpass's guard) with a
// wolf fighting the player 3 off it, the player too hardy to die of it.
func newGuardFix(t *testing.T) (*aggroFix, *NPC) {
	t.Helper()
	f := newSocialFix(t)
	f.p.HP, f.p.MaxHP = 1_000_000, 1_000_000
	wolf := f.npc(300, "Ravenous wolf", 20, bearBody, 1003, 1000)
	fightsPlayer(f, wolf)
	return f, wolf
}

// fightsPlayer sets n on the player, as its own notice or a hit would.
func fightsPlayer(f *aggroFix, n *NPC) {
	n.Threats = map[EntityID]int64{f.p.ServerID: 10}
	n.CurrentEnemy = f.p.ServerID
}

// tick is one of the AI's ticks and the way home's, the clock on 100 ms.
func (f *aggroFix) tick() {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.now = f.now.Add(100 * time.Millisecond)
	f.w.aiSystem(f.now)
	f.w.homeSystem(f.now)
}

// A wolf fights the player: the militia guard 20 off the player takes it on.
// The controls: a guard 35 off, one fighting already, a Highpass guard the
// player is KOS to, the baker (no row), a camp's "Jextalan guard" social
// under a faction of its own (a guard by its name alone), and a guard with
// the NPC bit do not; nor does anyone for a militia guard fighting the
// player, a town's business.
func TestTownGuardsTakeOnACreatureFightingAPlayer(t *testing.T) {
	f, wolf := newGuardFix(t)
	f.w.SetSocialRules(map[string]SocialRule{
		"Guard Alinon":   {Social: true, Faction: "The Freeport Militia"},
		"Guard Brenna":   {Social: true, Faction: "The Freeport Militia"},
		"Guard Cadwell":  {Social: true, Faction: "The Freeport Militia"},
		"Guard Ely":      {Social: true, Faction: "The Highpass Guard"},
		"Jextalan guard": {Social: true, Faction: "Jextalan Guards"},
		"Guard Stillman": {Social: true, Faction: "Guards of Rivervale"},
	})
	f.w.SetNPCFactions([]database.NPCFactionRow{{Name: "guard ely", World: -1, Faction: "The Highpass Guard"}})
	f.p.Factions = map[string]int{"The Highpass Guard": -2000}

	near := f.npc(101, "Guard Brenna", 50, humanBody, 1020, 1000)
	far := f.npc(102, "Guard Cadwell", 50, humanBody, 1035, 1000)
	busy := f.npc(100, "Guard Alinon", 50, humanBody, 1010, 1000)
	rat := f.npc(108, "sewer rat", 5, bearBody, 1011, 1000)
	busy.Threats, busy.CurrentEnemy = map[EntityID]int64{rat.ServerID: 9}, rat.ServerID
	kos := f.npc(103, "Guard Ely", 50, humanBody, 1000, 1010)
	baker := f.npc(104, "Baker Dunn", 20, humanBody, 1000, 990)
	camp := f.npc(105, "Jextalan guard", 30, humanBody, 1005, 1005)
	still := f.npc(106, "Guard Stillman", 40, humanBody, 995, 1005)
	still.NPCType |= 0x80

	f.tick()
	if near.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the militia guard 20 off did not take the wolf on: enemy %d", near.CurrentEnemy)
	}
	for _, n := range []*NPC{far, kos, baker, camp, still} {
		if n.CurrentEnemy != 0 {
			t.Errorf("%s took a side (enemy %d); it should not", n.Name, n.CurrentEnemy)
		}
	}
	if busy.CurrentEnemy != rat.ServerID {
		t.Errorf("the busy guard left its own fight with the rat for the wolf (enemy %d)", busy.CurrentEnemy)
	}

	// a militia guard fighting the player is the town's business
	forget(near)
	g := f.npc(107, "Guard Cadwell", 50, humanBody, 1004, 1001)
	fightsPlayer(f, g)
	forget(wolf)
	f.tick()
	if near.CurrentEnemy == g.ServerID {
		t.Error("a militia guard took on a militia guard fighting the player")
	}
}

// The guard runs to the wolf, kills it and walks back to its post; the kill
// is the guards' (GuardsKill). The control: before the guard's first hit the
// wolf is no one's.
func TestAGuardKillsTheCreatureAndGoesBackToItsPost(t *testing.T) {
	f, wolf := newGuardFix(t)
	guard := f.npc(101, "Guard Brenna", 50, humanBody, 1020, 1000)
	guard.HP, guard.MaxHP = 5000, 5000
	f.tick()
	if guard.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the guard did not take the wolf on: enemy %d", guard.CurrentEnemy)
	}
	if f.w.GuardsKill(wolf.ServerID) {
		t.Fatal("the control: the wolf was the guards' before a guard hit it")
	}
	for i := 0; i < 1200 && !wolf.Dead; i++ {
		f.tick()
	}
	if !wolf.Dead {
		t.Fatalf("two minutes on the wolf lives: HP %d/%d, the guard %.1f off it, enemy %d",
			wolf.HP, wolf.MaxHP, distXZ(guard.X, guard.Z, wolf.X, wolf.Z), guard.CurrentEnemy)
	}
	if !f.w.GuardsKill(wolf.ServerID) {
		t.Error("the guard killed the wolf and its corpse is not the guards'")
	}
	for i := 0; i < 600 && distXZ(guard.X, guard.Z, guard.SpawnX, guard.SpawnZ) > 0.5; i++ {
		f.tick()
	}
	if d := distXZ(guard.X, guard.Z, guard.SpawnX, guard.SpawnZ); d > 0.5 || guard.CurrentEnemy != 0 {
		t.Errorf("after the fight the guard stands %.1f off its post, enemy %d", d, guard.CurrentEnemy)
	}
}

// A guard leashed: past its leash from its post it gives the chase up and
// walks back; the wolf goes on with the player.
func TestAGuardGivesUpPastItsLeash(t *testing.T) {
	f, wolf := newGuardFix(t)
	guard := f.npc(101, "Guard Brenna", 50, humanBody, 1020, 1000)
	guard.LeashRadius = 40
	wolf.HP, wolf.MaxHP = 1_000_000, 1_000_000
	f.tick()
	if guard.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the guard did not take the wolf on: enemy %d", guard.CurrentEnemy)
	}
	// the player runs off with the wolf after it, 100 off the post
	f.moveTo(1120, 1000)
	f.w.SpatialIndex.Update(wolf.ServerID, wolf.X, wolf.Z, 1117, 1000)
	wolf.X = 1117
	for i := 0; i < 100 && guard.CurrentEnemy != 0; i++ {
		f.tick()
	}
	if guard.CurrentEnemy != 0 {
		t.Fatalf("the guard chased the wolf past its leash of 40 (%.1f off its post)",
			distXZ(guard.X, guard.Z, guard.SpawnX, guard.SpawnZ))
	}
	if wolf.CurrentEnemy != f.p.ServerID {
		t.Errorf("the wolf left the player (enemy %d)", wolf.CurrentEnemy)
	}
}

// A creature a guard hit gives the player who kills it no experience, no
// faction hits and no quest's kill; the control is the same kill of one no
// guard hit, which gives all three.
func TestAKillTheGuardsHitEarnsNothing(t *testing.T) {
	f, _ := newGuardFix(t)
	f.w.factions.hits[database.NPCClassKey("Ravenous wolf")] = []FactionHit{{"Citizens of Freeport", 5}}
	heard := 0
	f.w.OnKill(func(KillCredit) { heard++ })
	kill := func(n *NPC) {
		f.w.mu.Lock()
		defer f.w.mu.Unlock()
		f.w.spellHitLocked(f.p, f.sess, n, 1_000_000)
	}

	ours := f.npc(301, "Ravenous wolf", 40, bearBody, 1003, 1003)
	fightsPlayer(f, ours)
	ours.guardHit = true
	xp, standing := f.p.TotalXP, f.p.Factions["Citizens of Freeport"]
	kill(ours)
	if !ours.Dead {
		t.Fatal("the spell did not kill the wolf")
	}
	if f.p.TotalXP != xp || f.p.Factions["Citizens of Freeport"] != standing || heard != 0 {
		t.Errorf("the guards' kill earned XP %d -> %d, standing %d -> %d, %d quest kills",
			xp, f.p.TotalXP, standing, f.p.Factions["Citizens of Freeport"], heard)
	}

	theirs := f.npc(302, "Ravenous wolf", 40, bearBody, 1003, 997)
	fightsPlayer(f, theirs)
	kill(theirs)
	if f.p.TotalXP <= xp || f.p.Factions["Citizens of Freeport"] == standing || heard != 1 {
		t.Errorf("the control: the player's own kill earned XP %d -> %d, standing %d -> %d, %d quest kills",
			xp, f.p.TotalXP, standing, f.p.Factions["Citizens of Freeport"], heard)
	}
}

// The mark lasts one fight: a wolf the guard hit that lives through the fight
// (its enemies gone) is no one's again, and a new life starts unmarked.
func TestTheGuardsMarkLastsOneFight(t *testing.T) {
	f, wolf := newGuardFix(t)
	wolf.guardHit = true
	f.tick()
	if !wolf.guardHit {
		t.Fatal("the mark went while the wolf still fought")
	}
	forget(wolf)
	f.tick()
	if wolf.guardHit {
		t.Error("the mark outlived the wolf's fight")
	}
	wolf.guardHit = true
	f.w.mu.Lock()
	f.w.killNPC(wolf)
	f.w.respawnNPCLocked(wolf)
	f.w.mu.Unlock()
	if wolf.guardHit {
		t.Error("the mark outlived the wolf's death")
	}
}

// GuardTown names the towns of the seed's guard factions and nothing else.
func TestGuardTown(t *testing.T) {
	for faction, want := range map[string]string{
		"The Freeport Militia": "Freeport",
		"The Qeynos Guard":     "Qeynos",
		"The Highpass Guard":   "Highpass",
		"Klick`Anon Watch":     "Klick`Anon",
		"Guards of Moradhim":   "Moradhim",
		"Citizens of Freeport": "",
		"House U`Dedne":        "",
		"Jextalan Guards":      "",
		"Deathfist Orcs":       "",
		"":                     "",
	} {
		if got := GuardTown(faction); got != want {
			t.Errorf("GuardTown(%q) = %q, want %q", faction, got, want)
		}
	}
}
