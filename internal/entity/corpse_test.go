package entity

import (
	"testing"
	"time"
)

// A creature's corpse lies its decay (3 minutes) unless emptied and closed,
// then goes from every view; the creature comes back on its spawn point its
// respawn time (60 s) after the corpse went (F3 20261025-044229).

// corpseWorld is a world with the viewer of slotsFixture and one creature
// beside it, killed at died: its spawn is 5 units off where it lies.
func corpseWorld(t *testing.T, died time.Time) (*World, *NPC) {
	t.Helper()
	w, _ := slotsFixture(t, 0)
	npc := &NPC{ServerID: 20000, Name: "beetle", X: 1003, Z: 1000, ModelID: 0x1234, Level: 2, HP: 0, MaxHP: 20,
		SpawnX: 1008, SpawnZ: 1000, RespawnTime: DefaultRespawnAfter, Dead: true, DeathTime: died}
	w.NPCs[npc.ServerID] = npc
	w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
	return w, npc
}

func TestACorpseLiesItsDecayThenGoesAndComesBackAMinuteLater(t *testing.T) {
	died := time.Now()
	w, npc := corpseWorld(t, died)

	// in view a minute after its death: the old rule hid it after 30 s
	if gone := w.corpseSystem(died.Add(time.Minute)); len(gone) != 0 || !npc.corpseLies(died.Add(time.Minute), w.corpseDecay) {
		t.Fatalf("a minute after its death: gone %v, lies %v; a corpse lies %v", gone, npc.corpseLies(died.Add(time.Minute), w.corpseDecay), DefaultCorpseDecay)
	}
	if npc.corpseLies(died.Add(DefaultCorpseDecay), w.corpseDecay) {
		t.Fatal("still lies at its decay")
	}

	// it decays: told once, then neither told again nor back before a minute more
	at := died.Add(DefaultCorpseDecay)
	if gone := w.corpseSystem(at); len(gone) != 1 || gone[0] != npc.ServerID {
		t.Fatalf("at its decay the system told %v, want [%d]", gone, npc.ServerID)
	}
	if !npc.CorpseGone.Equal(at) {
		t.Fatalf("its corpse went at %v, want its decay %v", npc.CorpseGone, at)
	}
	if gone := w.corpseSystem(at.Add(59 * time.Second)); len(gone) != 0 || !npc.Dead {
		t.Fatalf("59 s after its corpse went: told %v, dead %v; it comes back after 60", gone, npc.Dead)
	}

	// back on its spawn point, alive and whole, its corpse forgotten
	w.corpseSystem(at.Add(DefaultRespawnAfter))
	if npc.Dead || npc.HP != npc.MaxHP || npc.X != npc.SpawnX || npc.Z != npc.SpawnZ || !npc.CorpseGone.IsZero() || !npc.DeathTime.IsZero() {
		t.Fatalf("respawned: dead %v hp %d/%d at (%.0f, %.0f) spawn (%.0f, %.0f), corpse gone %v death %v",
			npc.Dead, npc.HP, npc.MaxHP, npc.X, npc.Z, npc.SpawnX, npc.SpawnZ, npc.CorpseGone, npc.DeathTime)
	}
}

// Emptied and closed, a corpse goes at once, and the minute runs from then:
// not from its death, and not after its decay.
func TestAnEmptiedCorpseGoesAtOnceAndComesBackAMinuteAfter(t *testing.T) {
	died := time.Now().Add(-20 * time.Second)
	w, npc := corpseWorld(t, died)

	if _, ok := w.CorpseLies(npc.ServerID); !ok {
		t.Fatal("a fresh corpse does not lie")
	}
	if w.ClearCorpse(npc.ServerID, died.Add(-time.Hour)) {
		t.Fatal("cleared a corpse of another death")
	}
	if !w.ClearCorpse(npc.ServerID, died) {
		t.Fatal("ClearCorpse refused a corpse that lies")
	}
	cleared := npc.CorpseGone
	if _, ok := w.CorpseLies(npc.ServerID); ok {
		t.Fatal("an emptied and closed corpse still lies")
	}
	if w.ClearCorpse(npc.ServerID, died) {
		t.Fatal("cleared twice")
	}
	if gone := w.corpseSystem(cleared.Add(time.Second)); len(gone) != 0 {
		t.Fatalf("an emptied corpse was told as decayed: %v", gone)
	}
	if w.corpseSystem(cleared.Add(59 * time.Second)); !npc.Dead {
		t.Fatal("back within a minute of its corpse going")
	}
	if w.corpseSystem(cleared.Add(DefaultRespawnAfter)); npc.Dead {
		t.Fatal("not back a minute after its corpse went")
	}
}

// Out of every view once gone: a client is told to delete it at the next
// round, not 30 s after the death, and it comes back as a new entity.
func TestACorpseIsInViewWhileItLies(t *testing.T) {
	w, sess := slotsFixture(t, 0)
	npc := &NPC{ServerID: 20000, Name: "beetle", X: 1003, Z: 1000, ModelID: 0x1234, Level: 2, MaxHP: 20,
		RespawnTime: DefaultRespawnAfter, Dead: true, DeathTime: time.Now().Add(-time.Minute)}
	w.NPCs[npc.ServerID] = npc
	w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)

	if got := visible(w, sess); len(got) != 1 {
		t.Fatalf("a corpse dead a minute: %d in view, want it", len(got))
	}
	npc.CorpseGone = time.Now()
	if got := visible(w, sess); len(got) != 0 {
		t.Fatalf("an emptied corpse: %d in view, want none", len(got))
	}
	npc.CorpseGone = time.Time{}
	npc.DeathTime = time.Now().Add(-DefaultCorpseDecay - time.Second)
	if got := visible(w, sess); len(got) != 0 {
		t.Fatalf("a decayed corpse: %d in view, want none", len(got))
	}
}

// A GM's /spawn leaves the world with its corpse.
func TestAGMSpawnLeavesWithItsCorpse(t *testing.T) {
	died := time.Now()
	w, npc := corpseWorld(t, died)
	npc.Temporary = true
	w.corpseSystem(died.Add(time.Minute))
	if _, ok := w.NPCs[npc.ServerID]; !ok {
		t.Fatal("a spawn left while its corpse lay")
	}
	w.corpseSystem(died.Add(DefaultCorpseDecay))
	if _, ok := w.NPCs[npc.ServerID]; ok {
		t.Fatal("a spawn stayed when its corpse went")
	}
}

// A kill stops a creature where it falls: a walking one's velocity would
// carry the corpse along on every client.
func TestADeathStopsAWalk(t *testing.T) {
	w := NewWorld()
	npc := &NPC{ServerID: 7, Name: "beetle", HP: 10, MaxHP: 10, VelX: 3, VelZ: 4, Loco: LocoWalk}
	w.NPCs[npc.ServerID] = npc
	w.killNPC(npc)
	if npc.moving() || npc.Loco != LocoStand {
		t.Fatalf("a corpse with velocity (%v, %v, %v) and animation %d", npc.VelX, npc.VelY, npc.VelZ, npc.Loco)
	}
	buf := SerializeNPCUpdate(1, npc)
	if buf[40] != 0 || buf[41] != 0 || buf[42] != 0 {
		t.Fatalf("a corpse's channel carries a velocity: % x", buf[40:43])
	}
}

// The settings: a zero keeps the default; the respawn reaches the NPCs
// already in the world.
func TestCorpseTimesFromTheConfig(t *testing.T) {
	w, npc := corpseWorld(t, time.Now())
	w.SetCorpseTimes(0, 0)
	if d, r := w.CorpseTimes(); d != DefaultCorpseDecay || r != DefaultRespawnAfter {
		t.Fatalf("zeros gave %v and %v", d, r)
	}
	w.SetCorpseTimes(30*time.Second, 5*time.Minute)
	if d, r := w.CorpseTimes(); d != 30*time.Second || r != 5*time.Minute || npc.RespawnTime != 5*time.Minute {
		t.Fatalf("set 30s, 5m: %v, %v, the NPC's %v", d, r, npc.RespawnTime)
	}
}
