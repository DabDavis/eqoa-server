package entity

import (
	"encoding/json"
	"testing"
	"time"
)

// soloCorpseWorld is corpseWorld's beetle alive, the rule outside a raid on, killed at died
// (killNPC, as a fight kills it).
func soloCorpseWorld(t *testing.T, died time.Time) (*World, *NPC) {
	t.Helper()
	w, npc := corpseWorld(t, died)
	w.SetSoloCorpse(0, 0, 0)
	npc.Dead, npc.DeathTime, npc.HP = false, time.Time{}, npc.MaxHP
	w.mu.Lock()
	npc.HP = 0
	w.killNPC(npc)
	npc.DeathTime = died
	w.mu.Unlock()
	return w, npc
}

// Heirloom petition 2752: outside a raid a corpse lies 10 minutes as one corpse (no copy
// left at 3 minutes), its creature down with it, and back its respawn time after it went.
// The control: the rule off, the copy at 3 minutes and the creature back after 3 + 1.
func TestASoloCorpseLiesTenMinutesItsCreatureBackAfterIt(t *testing.T) {
	died := time.Now().Add(-time.Second)
	w, npc := soloCorpseWorld(t, died)
	if npc.soloDecay != DefaultSoloCorpseDecay {
		t.Fatalf("the corpse's rule: %v, want 10m", npc.soloDecay)
	}
	if end, ok := w.CorpseDecaysAt(npc.ServerID); !ok || !end.Equal(died.Add(10*time.Minute)) {
		t.Fatalf("it decays at %v, want 10m", end.Sub(died))
	}
	var left []CorpseLeft
	w.OnCorpseLeft(func(l []CorpseLeft) { left = append(left, l...) })
	for _, at := range []time.Duration{DefaultCorpseDecay, DefaultCorpseDecay + DefaultRespawnAfter, 10*time.Minute - time.Second} {
		w.mu.Lock()
		gone := w.corpseSystem(died.Add(at))
		w.mu.Unlock()
		w.runCorpsesGone(gone)
		if len(gone) != 0 || !npc.Dead || !npc.CorpseGone.IsZero() || len(left) != 0 {
			t.Fatalf("at %v: gone %v, dead %v, went %v, left %v", at, gone, npc.Dead, npc.CorpseGone, left)
		}
	}
	at := died.Add(10 * time.Minute)
	w.mu.Lock()
	gone := w.corpseSystem(at)
	w.mu.Unlock()
	if len(gone) != 1 || gone[0] != npc.ServerID || !npc.CorpseGone.Equal(at) {
		t.Fatalf("at 10 minutes: gone %v, went %v", gone, npc.CorpseGone)
	}
	w.mu.Lock()
	w.corpseSystem(at.Add(DefaultRespawnAfter - time.Second))
	w.mu.Unlock()
	if !npc.Dead {
		t.Fatal("back before its respawn time after its corpse went")
	}
	w.mu.Lock()
	w.corpseSystem(at.Add(DefaultRespawnAfter))
	w.mu.Unlock()
	if npc.Dead || npc.soloDecay != 0 || npc.corpseChecks != 0 {
		t.Fatalf("at 10m + its respawn: dead %v, rule %v, checks %d", npc.Dead, npc.soloDecay, npc.corpseChecks)
	}

	// the control: the rule off (a negative decay)
	w2, npc2 := corpseWorld(t, died)
	w2.SetSoloCorpse(-1, 0, 0)
	npc2.Dead, npc2.DeathTime = false, time.Time{}
	w2.mu.Lock()
	w2.killNPC(npc2)
	npc2.DeathTime = died
	w2.corpseSystem(died.Add(DefaultCorpseDecay))
	w2.mu.Unlock()
	if npc2.soloDecay != 0 || npc2.CorpseGone.IsZero() {
		t.Fatalf("the rule off: rule %v, its corpse went %v", npc2.soloDecay, npc2.CorpseGone)
	}
}

// Looted clear, a solo corpse goes at once and its creature's respawn time runs from then.
func TestASoloCorpseLootedClearGoesAndItsRespawnRunsFromThen(t *testing.T) {
	died := time.Now().Add(-2 * time.Minute)
	w, npc := soloCorpseWorld(t, died)
	if !w.ClearCorpse(npc.ServerID, died) {
		t.Fatal("not cleared")
	}
	cleared := npc.CorpseGone
	if cleared.IsZero() || cleared.Sub(died) < 2*time.Minute {
		t.Fatalf("went %v after its death", cleared.Sub(died))
	}
	w.mu.Lock()
	w.corpseSystem(cleared.Add(DefaultRespawnAfter - time.Second))
	w.mu.Unlock()
	if !npc.Dead {
		t.Fatal("back before its respawn time from the clearing")
	}
	w.mu.Lock()
	w.corpseSystem(cleared.Add(DefaultRespawnAfter))
	w.mu.Unlock()
	if npc.Dead {
		t.Fatal("not back its respawn time after the clearing")
	}
}

// Each opening puts a solo corpse's decay back to 10 minutes from then, ten times a death;
// the eleventh changes nothing. A raid's corpse is held 3 minutes from an opening, no limit.
func TestASoloCorpseOpenedTenTimesAtMost(t *testing.T) {
	died := time.Now().Add(-9 * time.Minute)
	w, npc := soloCorpseWorld(t, died)
	for i := 1; i <= DefaultCorpseChecksMax; i++ {
		before := time.Now()
		if !w.RefreshCorpse(npc.ServerID, died) {
			t.Fatalf("opening %d did not put it back", i)
		}
		if end := npc.corpseEnds(w.corpseDecay); end.Before(before.Add(10*time.Minute)) || end.After(time.Now().Add(10*time.Minute)) {
			t.Fatalf("opening %d: it decays %v from now, want 10m", i, time.Until(end))
		}
		if npc.corpseChecks != i {
			t.Fatalf("opening %d counted %d", i, npc.corpseChecks)
		}
	}
	held := npc.corpseHeld
	if w.RefreshCorpse(npc.ServerID, died) || !npc.corpseHeld.Equal(held) || npc.corpseChecks != DefaultCorpseChecksMax {
		t.Fatalf("the eleventh opening: held %v -> %v, checks %d", held, npc.corpseHeld, npc.corpseChecks)
	}
	if w.RefreshCorpse(npc.ServerID, died.Add(time.Second)) {
		t.Fatal("another death's opening held it")
	}

	// a raid's kill: 3 minutes from the opening, as often as it is opened
	w2, npc2 := corpseWorld(t, time.Now().Add(-2*time.Minute))
	w2.SetSoloCorpse(0, 0, 0)
	for i := 0; i < 15; i++ {
		if !w2.RefreshCorpse(npc2.ServerID, npc2.DeathTime) {
			t.Fatalf("a raid's corpse: opening %d not held", i+1)
		}
	}
	if npc2.corpseChecks != 0 || time.Until(npc2.corpseHeld) > DefaultCorpseDecay {
		t.Fatalf("a raid's corpse: checks %d, held %v more", npc2.corpseChecks, time.Until(npc2.corpseHeld))
	}
}

// A solo kill's corpse is anyone's at 7 minutes; a raid's kill keeps the raid's rules: 6
// minutes, and its corpse left as a copy at 3 to lie to 20.
func TestASoloKillOpensAtSevenARaidsKeepsItsRules(t *testing.T) {
	r := newRaidRig(t, 6, 30)
	r.w.SetSoloCorpse(0, 0, 0)
	var heard []KillCredit
	r.w.OnKill(func(k KillCredit) { heard = append(heard, k) })
	kill := func(id EntityID) (*NPC, KillCredit) {
		npc := r.fight(id, 0)
		r.w.mu.Lock()
		npc.Threats, npc.CurrentEnemy = nil, 0
		r.w.killNPC(npc)
		r.w.creditKillLocked(r.sess[0], r.p[0], npc)
		r.w.mu.Unlock()
		return npc, heard[len(heard)-1]
	}
	npc, k := kill(9710)
	if !k.OpenAt.Equal(npc.DeathTime.Add(7*time.Minute)) || npc.soloDecay != 10*time.Minute {
		t.Errorf("a solo kill: anyone's %v after, rule %v", k.OpenAt.Sub(npc.DeathTime), npc.soloDecay)
	}
	r.form(3)
	npc, k = kill(9711)
	if !k.OpenAt.Equal(npc.DeathTime.Add(7*time.Minute)) || npc.soloDecay != 10*time.Minute {
		t.Errorf("a group's kill: anyone's %v after, rule %v", k.OpenAt.Sub(npc.DeathTime), npc.soloDecay)
	}
	for i := 3; i < 6; i++ {
		r.join(0, i)
	}
	if g := r.raidOf(0); g == nil || !raidingLocked(g) {
		t.Fatal("not a raid of six")
	}
	npc, k = kill(9712)
	if !k.OpenAt.Equal(npc.DeathTime.Add(6*time.Minute)) || npc.soloDecay != 0 {
		t.Fatalf("a raid's kill: anyone's %v after, rule %v", k.OpenAt.Sub(npc.DeathTime), npc.soloDecay)
	}
	if end, ok := r.w.CorpseDecaysAt(npc.ServerID); !ok || !end.Equal(npc.DeathTime.Add(DefaultCorpseLinger)) {
		t.Fatalf("a raid's corpse decays %v after, want the linger's 20m", end.Sub(npc.DeathTime))
	}
	died := npc.DeathTime
	r.w.mu.Lock()
	r.w.corpseSystem(died.Add(DefaultCorpseDecay))
	left := r.w.corpsesLeft
	r.w.mu.Unlock()
	if len(left) != 1 || left[0].From != npc.ServerID || !left[0].Died.Equal(died) {
		t.Fatalf("a raid's kill at 3 minutes: corpses left %+v, want its copy", left)
	}
	if c := r.w.NPCs[left[0].To]; c == nil || !c.Dead || c.soloDecay != 0 || c.CorpseDecay != DefaultCorpseLinger {
		t.Fatalf("the raid's corpse left: %+v", c)
	}
}

// The rule comes through a keep restart: the corpse's rule, its openings and its hold; a
// file from before it (no solo_decay) reads as the raid's rule.
func TestASoloCorpseComesThroughARestart(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	old.SetSoloCorpse(0, 0, 0)
	if !old.KillNPCNow(keepWolf) {
		t.Fatal("not killed")
	}
	npc := old.NPCs[keepWolf]
	old.RefreshCorpse(keepWolf, npc.DeathTime)
	old.RefreshCorpse(keepWolf, npc.DeathTime)
	held := npc.corpseHeld
	kept := keepFile(t, old.KeepNPCs())
	var k KeptNPC
	for _, x := range kept {
		if EntityID(x.ID) == keepWolf {
			k = x
		}
	}
	if k.SoloDecay != 10*time.Minute || k.Checks != 2 || !k.CorpseHeld.Equal(held) {
		t.Fatalf("written: %+v", k)
	}

	w := keepNPCProcess(t, db)
	w.SetSoloCorpse(0, 0, 0)
	w.RestoreNPCs(kept)
	back := w.NPCs[keepWolf]
	if !back.Dead || back.soloDecay != 10*time.Minute || back.corpseChecks != 2 || !back.corpseHeld.Equal(held) {
		t.Fatalf("restored: dead %v, rule %v, checks %d, held %v", back.Dead, back.soloDecay, back.corpseChecks, back.corpseHeld)
	}
	w.mu.Lock()
	w.corpseSystem(back.DeathTime.Add(DefaultCorpseDecay + DefaultRespawnAfter))
	w.mu.Unlock()
	if !back.Dead || !back.CorpseGone.IsZero() {
		t.Fatal("after the restart its corpse went at the raid's 3 minutes")
	}

	// an older file: no solo_decay, no checks
	b, _ := json.Marshal(k)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	delete(m, "solo_decay")
	delete(m, "checks")
	b, _ = json.Marshal(m)
	var olderK KeptNPC
	if err := json.Unmarshal(b, &olderK); err != nil {
		t.Fatal(err)
	}
	w3 := keepNPCProcess(t, db)
	w3.SetSoloCorpse(0, 0, 0)
	w3.RestoreNPCs([]KeptNPC{olderK})
	if o := w3.NPCs[keepWolf]; !o.Dead || o.soloDecay != 0 || o.corpseChecks != 0 {
		t.Fatalf("an older file's corpse: dead %v, rule %v, checks %d", o.Dead, o.soloDecay, o.corpseChecks)
	}
}
