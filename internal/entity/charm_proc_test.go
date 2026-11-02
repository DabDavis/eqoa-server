package entity

import (
	"testing"
	"time"
)

// The enchanter's weapon proc on its charmed creature (the user, 2026-11-02:
// "the Damage proc enchanter buff should help").

// Cast at the creature it charmed, the Chaotic line lands on it (a creature
// with an owner is on the players' side), and the creature's own hits proc
// it: the proc's damage as its line gives it, told to the enchanter as its
// pet's. The creature's hit is its own roll (a charmed creature keeps a
// creature's strength: the user, "they can stay mob strength").
func TestTheEnchantersProcGoesOnItsCharmedCreature(t *testing.T) {
	defer func(f, g, h func(int) int) { charmIntn, meleeIntn, procIntn = f, g, h }(charmIntn, meleeIntn, procIntn)
	charmIntn, meleeIntn, procIntn = dice(99), dice(0), dice(99)
	r, _, _ := charmRig(t)
	if began, _ := castCharm(t, r, captivation(), crabID, 0); !began || r.enemy.Owner != r.p.ServerID {
		t.Fatalf("not charmed")
	}
	crab := r.enemy
	foe := &NPC{ServerID: 3400, Name: "sand giant", Level: 33, HP: 50000, MaxHP: 50000, ModelID: 0x55432169, Size: 1,
		X: crab.X + 2, Y: crab.Y, Z: crab.Z, World: 2}
	r.w.NPCs[foe.ServerID] = foe
	r.w.SpatialIndex.Insert(foe.ServerID, foe.X, foe.Z)
	vs := GetOrCreateVisibility(r.sess) // in the enchanter's view: its pet's hits on it are told
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = foe.ServerID, 0x0133E8B0
	ps := r.w.pets[r.p.ServerID]
	ps.target = foe.ServerID

	flux := chaoticFlux()
	if !r.w.StartCast(r.sess, flux, crabID, r.at(20*time.Second)) {
		t.Fatalf("Chaotic Flux on the charmed crab did not begin: %x", r.sent())
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(20*time.Second + flux.CastTime + castLand))
	r.w.mu.Unlock()
	if len(crab.Effects) != 1 || crab.Effects[0].Proc != 64 {
		t.Fatalf("on the charmed crab: %+v", crab.Effects)
	}
	procIntn = dice(0, 99) // procs, not resisted
	plain := petMeleeHit(crab, foe, meleeIntn)
	r.sent()
	r.w.mu.Lock()
	crab.Effects[0].Until = r.at(10 * time.Minute)
	before := foe.HP
	r.w.petHitLocked(ps, foe, r.at(30*time.Second))
	r.w.mu.Unlock()
	if f := figures(r.sess); len(f) != 2 || f[0] != -plain || f[1] != -64 || before-foe.HP != plain+64 {
		t.Errorf("the enchanter was told %v (want -%d, -64); the giant lost %d", f, plain, before-foe.HP)
	}
}

// The simulator's charm play puts its book's weapon procs on the creature it
// charmed, as an enchanter does: a proc of a million ends the fight at the
// creature's first one, where the same book with its procs kept on the
// enchanter (CharmProcsOff: it never swings) fights it out with the
// creature's hits alone; the same seed for both.
func TestSimulatedCharmPutsItsProcsOnTheCreature(t *testing.T) {
	pl := SimPlayer{Class: classEnchanter, Race: 0, Level: 20, Base: SimBaseStats(classEnchanter, 0, 20),
		Play: SimPlay{Buffs: true, Spells: true, Charm: true, CharmDiff: -2}}
	pl.Spells = []CastSpell{
		{SpellID: 700173, DisplayIndex: 1, Level: 20, Scope: 1, Name: "Cajole", CastTime: 5 * time.Second,
			Recast: 30 * time.Second, PowerCost: 175, Range: 30, Def: ParseSpellText("Target, Charm ; Increase 25% based on CHA, 2.5min")},
		{SpellID: 522, DisplayIndex: 2, Level: 16, Scope: 1, Name: "Chaotic Flux", CastTime: time.Second, Recast: time.Second,
			PowerCost: 40, Range: 30, Def: ParseSpell("Target, Decrease Hitpoints by 1000000, 5min", fluxDesc)},
	}
	if !pl.Spells[1].Def.hasProc() {
		t.Fatalf("the stand-in flux is not a proc: %+v", pl.Spells[1].Def)
	}
	on := SimulateFight(pl, SimNPC{Level: 18}, SeedBalance(), 3, 10*time.Minute)
	pl.Play.CharmProcsOff = true
	off := SimulateFight(pl, SimNPC{Level: 18}, SeedBalance(), 3, 10*time.Minute)
	if !on.HadPet || !off.HadPet {
		t.Fatalf("no charmed creature: on %+v, off %+v", on, off)
	}
	t.Logf("procs on the creature: won %v in %v; on the enchanter: won %v in %v", on.Won, on.Time, off.Won, off.Time)
	if !on.Won || (off.Won && on.Time >= off.Time/2) {
		t.Errorf("its procs on the creature: won %v in %v; kept on the enchanter: won %v in %v", on.Won, on.Time, off.Won, off.Time)
	}
}
