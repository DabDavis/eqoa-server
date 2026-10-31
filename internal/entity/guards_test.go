package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Town guards keep their posts clear of creatures (guards.go; jib's F3
// 20261030-063534 and the user's rule of 2026-10-31).

// newWatchFix is socialFix with the player far off and the militia's Guard
// Brenna at her post (2000, 10, 2000).
func newWatchFix(t *testing.T) (*aggroFix, *NPC) {
	t.Helper()
	f := newSocialFix(t)
	f.p.HP, f.p.MaxHP = 1_000_000, 1_000_000
	f.moveTo(5000, 5000)
	g := f.npc(101, "Guard Brenna", 50, humanBody, 2000, 2000)
	g.HP, g.MaxHP = 5000, 5000
	return f, g
}

// wanderer is a creature spawned 500 east of (x, z) that has come there.
func (f *aggroFix) wanderer(id EntityID, name string, level int, body uint32, x, z float32) *NPC {
	n := f.npc(id, name, level, body, x, z)
	n.SpawnX = x + 500
	return n
}

// watch is one of the guards' looks (every aggroScanTicks).
func (f *aggroFix) watch() {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.w.TickCount = aggroScanTicks * (f.w.TickCount/aggroScanTicks + 1)
	f.w.guardWatchSystem(f.now)
}

// tick is one of the AI's ticks and the way home's, the clock on 100 ms.
func (f *aggroFix) tick() {
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	f.now = f.now.Add(100 * time.Millisecond)
	f.w.aiSystem(f.now)
	f.w.homeSystem(f.now)
}

// fightsPlayer sets n on the player, as its own notice or a hit would.
func fightsPlayer(f *aggroFix, n *NPC) {
	n.Threats = map[EntityID]int64{f.p.ServerID: 10}
	n.CurrentEnemy = f.p.ServerID
}

// A wolf that wandered to 28 off the guard's post is taken on, no player
// near. Every control stands nearer, so the guard (who takes the nearest)
// would take any of them it did not leave alone: a pet, a charmed creature,
// a town guard on a creature's body, a camp's sentry of a guard faction, a
// creature of the militia's own, an alley cat of Freeport's people, a bandit
// on a players' race body, a sign post's and a blocker's body, one with the
// NPC bit, a training and a sparring dummy, a dead wolf, a GM's peaceful
// minnow, a quest's spawn, one out of the world, and a resident wolf spawned
// 10 off the post; nor one 35 off.
func TestATownGuardTakesOnACreatureThatWandersNear(t *testing.T) {
	f, g := newWatchFix(t)
	f.w.factions.hits[database.NPCClassKey("militia hound")] = []FactionHit{{"Citizens of Freeport", -50}, {"The Freeport Militia", -10}}
	f.w.SetNPCFactions([]database.NPCFactionRow{
		{Name: "jextalan sentry", World: -1, Faction: "Gate Guards of Urglunt"},
		{Name: "freeport alley cat", World: -1, Faction: "Citizens of Freeport"},
	})
	f.w.SetAggroRules(map[string]AggroRule{"cave minnow": {Aggressive: false}})
	wolf := f.wanderer(300, "ravenous wolf", 20, bearBody, 2028, 2000)

	at := float32(2001)
	next := func() float32 { at += 1.3; return at }
	pet := f.wanderer(310, "a pet wolf", 20, bearBody, next(), 2000)
	pet.Owner = f.p.ServerID
	charmed := f.wanderer(311, "ravenous wolf", 20, bearBody, next(), 2000)
	charmed.Charm = &charmHold{}
	f.wanderer(312, "Guard Cadwell", 50, bearBody, next(), 2000)
	f.wanderer(313, "jextalan sentry", 30, bearBody, next(), 2000)
	f.wanderer(314, "militia hound", 20, bearBody, next(), 2000)
	f.wanderer(315, "freeport alley cat", 5, bearBody, next(), 2000)
	f.wanderer(316, "a bandit", 20, humanBody, next(), 2000)
	f.wanderer(317, "Freeport Sign", 1, SignpostBody, next(), 2000)
	f.wanderer(318, "blocker", 1, BlockerBody, next(), 2000)
	tame := f.wanderer(319, "a tame crow", 5, bearBody, next(), 2000)
	tame.NPCType |= 0x80
	training := f.wanderer(320, "Training Dummy Lv20", 20, bearBody, next(), 2000)
	training.Dummy = TrainingDummy
	sparring := f.wanderer(321, "Sparring Dummy", 20, bearBody, next(), 2000)
	sparring.Dummy = SparringDummy
	dead := f.wanderer(322, "ravenous wolf", 20, bearBody, next(), 2000)
	dead.Dead = true
	f.wanderer(323, "cave minnow", 2, bearBody, next(), 2000)
	quest := f.wanderer(324, "ravenous wolf", 20, bearBody, next(), 2000)
	quest.QuestSpawn = true
	kept := f.wanderer(325, "ravenous wolf", 20, bearBody, next(), 2000)
	kept.SpawnOnly = true
	f.npc(326, "ravenous wolf", 20, bearBody, 2010, 2003) // a resident
	f.wanderer(327, "ravenous wolf", 20, bearBody, 2035, 2000)

	f.watch()
	if g.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the guard took on %q (%d), want the wolf that wandered in", f.w.NPCs[g.CurrentEnemy].Name, g.CurrentEnemy)
	}

	// the controls: none of them alone draws it
	forget(g)
	f.w.SpatialIndex.Remove(wolf.ServerID, wolf.X, wolf.Z)
	delete(f.w.NPCs, wolf.ServerID)
	f.watch()
	if g.CurrentEnemy != 0 {
		t.Errorf("with the wolf gone the guard took on %q", f.w.NPCs[g.CurrentEnemy].Name)
	}
}

// The guard looks only while free, and only where a creature can be told
// from a townsperson (race bodies known); a row taken away makes it no
// guard at once (the rules' generation).
func TestWhenAGuardLooks(t *testing.T) {
	f, g := newWatchFix(t)
	wolf := f.wanderer(300, "ravenous wolf", 20, bearBody, 2020, 2000)
	look := func() bool {
		forget(g)
		g.chase.returning = false
		return true
	}
	look()
	g.chase.returning = true
	f.watch()
	if g.CurrentEnemy != 0 {
		t.Error("a guard on its way home took the wolf on")
	}
	look()
	f.w.playerModels = nil
	f.watch()
	if g.CurrentEnemy != 0 {
		t.Error("with no race bodies known a guard took on a creature")
	}
	f.w.playerModels = map[uint32]bool{humanBody: true}
	f.w.SetSocialRules(map[string]SocialRule{"Guard Alinon": {Social: true, Faction: "The Freeport Militia"}})
	f.watch()
	if g.CurrentEnemy != 0 {
		t.Error("a guard whose row was taken away still took the wolf on")
	}
	f.w.SetSocialRules(map[string]SocialRule{"Guard Brenna": {Social: true, Faction: "The Freeport Militia"}})
	f.watch()
	if g.CurrentEnemy != wolf.ServerID {
		t.Errorf("the control: the guard with its row back did not take the wolf on (enemy %d)", g.CurrentEnemy)
	}
}

// The guard runs to the wolf, kills it and walks back to its post; the kill
// is the guards' (GuardsKill). The control: before the guard's first hit the
// wolf is no one's.
func TestAGuardKillsTheCreatureAndGoesBackToItsPost(t *testing.T) {
	f, g := newWatchFix(t)
	wolf := f.wanderer(300, "ravenous wolf", 20, bearBody, 2020, 2000)
	f.watch()
	if g.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the guard did not take the wolf on: enemy %d", g.CurrentEnemy)
	}
	if f.w.GuardsKill(wolf.ServerID) {
		t.Fatal("the control: the wolf was the guards' before a guard hit it")
	}
	for i := 0; i < 1200 && !wolf.Dead; i++ {
		f.tick()
	}
	if !wolf.Dead {
		t.Fatalf("two minutes on the wolf lives: HP %d/%d, the guard %.1f off it, enemy %d",
			wolf.HP, wolf.MaxHP, distXZ(g.X, g.Z, wolf.X, wolf.Z), g.CurrentEnemy)
	}
	if !f.w.GuardsKill(wolf.ServerID) {
		t.Error("the guard killed the wolf and its corpse is not the guards'")
	}
	for i := 0; i < 600 && distXZ(g.X, g.Z, g.SpawnX, g.SpawnZ) > 0.5; i++ {
		f.tick()
	}
	if d := distXZ(g.X, g.Z, g.SpawnX, g.SpawnZ); d > 0.5 || g.CurrentEnemy != 0 {
		t.Errorf("after the fight the guard stands %.1f off its post, enemy %d", d, g.CurrentEnemy)
	}
}

// A guard follows a creature GuardLeash (100) from its post, then gives the
// fight up and walks back. The control: a guard after a player keeps the
// NPC's own leash, past 100.
func TestAGuardGivesUpAHundredFromItsPost(t *testing.T) {
	f, g := newWatchFix(t)
	wolf := f.wanderer(300, "ravenous wolf", 20, bearBody, 2020, 2000)
	wolf.HP, wolf.MaxHP = 1_000_000, 1_000_000
	f.watch()
	if g.CurrentEnemy != wolf.ServerID {
		t.Fatalf("the guard did not take the wolf on: enemy %d", g.CurrentEnemy)
	}
	// the wolf runs off east, ahead of the guard
	var far float32
	for i := 0; i < 300 && g.CurrentEnemy != 0; i++ {
		x := g.X + 15
		f.w.SpatialIndex.Update(wolf.ServerID, wolf.X, wolf.Z, x, 2000)
		wolf.X = x
		f.tick()
		far = max(far, distXZ(g.X, g.Z, g.SpawnX, g.SpawnZ))
	}
	if g.CurrentEnemy != 0 {
		t.Fatalf("the guard still chases the wolf %.1f from its post", distXZ(g.X, g.Z, g.SpawnX, g.SpawnZ))
	}
	if far < 90 || far > GuardLeash+2 {
		t.Errorf("the guard gave up %.1f from its post, want at its leash of %.0f", far, GuardLeash)
	}
	if !g.chase.returning {
		t.Error("the guard gave up and is not on its way back")
	}

	// the control: a guard after a player follows it past 100
	h := f.npc(102, "Guard Cadwell", 50, humanBody, 3000, 3000)
	fightsPlayer(f, h)
	f.moveTo(3150, 3000)
	for i := 0; i < 300 && distXZ(h.X, h.Z, f.p.X, f.p.Z) > 5; i++ {
		f.tick()
	}
	if h.CurrentEnemy != f.p.ServerID || distXZ(h.X, h.Z, h.SpawnX, h.SpawnZ) < 140 {
		t.Errorf("the control: a guard after a player gave up %.1f from its post (enemy %d)",
			distXZ(h.X, h.Z, h.SpawnX, h.SpawnZ), h.CurrentEnemy)
	}
}

// A creature a guard hit gives the player who kills it no experience, no
// faction hits and no quest's kill; the control is the same kill of one no
// guard hit, which gives all three.
func TestAKillTheGuardsHitEarnsNothing(t *testing.T) {
	f := newSocialFix(t)
	f.p.HP, f.p.MaxHP = 1_000_000, 1_000_000
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
	f := newSocialFix(t)
	f.p.HP, f.p.MaxHP = 1_000_000, 1_000_000
	wolf := f.npc(300, "ravenous wolf", 20, bearBody, 1003, 1000)
	fightsPlayer(f, wolf)
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

// GuardTown names the towns of the seed's guard factions and nothing else;
// guardFaction is any guard faction, a camp's too.
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
	for faction, want := range map[string]bool{
		"Jextalan Guards": true, "Guards of Moradhim": true, "The Freeport Militia": true,
		"Citizens of Freeport": false, "Deathfist Orcs": false, "": false,
	} {
		if got := guardFaction(faction); got != want {
			t.Errorf("guardFaction(%q) = %v, want %v", faction, got, want)
		}
	}
}

// The world's tick runs the guards' look (tickLocked): a wolf that wandered
// near is taken on at the next scan tick, whatever the other systems do.
func TestTheTickRunsTheGuardsLook(t *testing.T) {
	f, g := newWatchFix(t)
	wolf := f.wanderer(300, "ravenous wolf", 20, bearBody, 2020, 2000)
	for i := 0; i < aggroScanTicks && g.CurrentEnemy == 0; i++ {
		f.now = f.now.Add(100 * time.Millisecond)
		f.w.tickLocked(f.now)
	}
	if g.CurrentEnemy != wolf.ServerID {
		t.Errorf("%d ticks on the guard has not taken the wolf on (enemy %d)", aggroScanTicks, g.CurrentEnemy)
	}
}
