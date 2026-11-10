package entity

import (
	"testing"
	"time"
)

// Familiars (familiar.go), held to the GM Jibrish's petition 1445: Healbot
// comes by its item's 2 s cast, one at a time; heals its owner 600 below 75%
// of its hit points (not at 76%), at most every 6 s, never one flagged for
// PvP or in a duel; draws no hate, never attacks, regenerates 2,000 a tick;
// goes at its owner's death, a dismissal and a logout.

// healbot is Healbot as tools/seed_familiars.py makes his row.
func healbot() FamiliarDef {
	return FamiliarDef{Item: 715060, Name: "Healbot", Model: 0xFBABEB00, Size: 1, HP: 100000, Regen: 2000, Cast: 2 * time.Second,
		Heal: 600, HealBelow: 75, HealEvery: 6 * time.Second}
}

// callFamiliar uses the familiar's item at at: its cast begun, landed castLand
// after its cast time.
func (r *mercRig) callFamiliar(at time.Duration) *familiarState {
	r.t.Helper()
	d := healbot()
	if !r.w.StartCast(r.sess, FamiliarCast(d), 0xFFFFFFFF, r.at(at)) {
		r.t.Fatal("the familiar's cast did not begin")
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(at + d.Cast + castLand - time.Millisecond))
	early := r.w.familiars[r.p.ServerID]
	r.w.castSystem(r.at(at + d.Cast + castLand))
	r.w.mu.Unlock()
	if early != nil && early.npc.Dead {
		early = nil
	}
	fs := r.w.familiars[r.p.ServerID]
	if fs == nil {
		r.t.Fatal("no familiar after its cast landed")
	}
	return fs
}

// The item's cast calls Healbot as it lands, never before: an NPC players and
// pets cannot attack (the NPC bit), never owned, of 100,000 hit points; a
// second use takes the first's place.
func TestFamiliarComesByItsCast(t *testing.T) {
	r := newMercRig(t)
	d := healbot()
	if !r.w.StartCast(r.sess, FamiliarCast(d), 0xFFFFFFFF, r.at(0)) {
		t.Fatal("the cast did not begin")
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(d.Cast + castLand - time.Millisecond))
	r.w.mu.Unlock()
	if r.w.familiars[r.p.ServerID] != nil {
		t.Fatal("Healbot came before his cast landed")
	}
	if len(r.p.Lockouts) != 0 {
		t.Errorf("an item's familiar locked out %d abilities", len(r.p.Lockouts))
	}
	r.w.mu.Lock()
	r.w.castSystem(r.at(d.Cast + castLand))
	r.w.mu.Unlock()
	fs := r.w.familiars[r.p.ServerID]
	if fs == nil {
		t.Fatal("no Healbot")
	}
	n := fs.npc
	if n.Name != "Healbot" || uint32(n.ModelID) != 0xFBABEB00 || n.HP != 100000 || n.MaxHP != 100000 || n.NPCType != 0x80 ||
		n.Owner != 0 || !n.Temporary {
		t.Errorf("Healbot: %q body %#x HP %d/%d type %#x owner %d temporary %v", n.Name, uint32(n.ModelID), n.HP, n.MaxHP, n.NPCType,
			n.Owner, n.Temporary)
	}
	first := n.ServerID
	r.callFamiliar(10 * time.Second)
	count := 0
	for _, npc := range r.w.NPCs {
		if npc.isFamiliar() {
			count++
		}
	}
	if count != 1 || r.w.NPCs[first] != nil {
		t.Errorf("%d familiars after a second call, the first still there: %v", count, r.w.NPCs[first] != nil)
	}
}

// Healbot heals his owner below 75% of its hit points, 600 at once, at most
// every 6 s, and not at 76% (the control).
func TestHealbotHealsBelowThreeQuarters(t *testing.T) {
	r := newMercRig(t)
	r.callFamiliar(0)
	top := r.p.MaxHPNow()
	r.p.HP = top * 76 / 100
	r.run(5*time.Second, 6*time.Second)
	if r.p.HP != top*76/100 {
		t.Fatalf("at 76%% Healbot healed: %d -> %d", top*76/100, r.p.HP)
	}
	r.p.HP = top * 74 / 100
	before := r.p.HP
	r.tick(7 * time.Second)
	if r.p.HP != min(before+600, top) {
		t.Fatalf("at 74%% Healbot healed %d, want 600", r.p.HP-before)
	}
	r.p.HP = top / 2
	r.run(7*time.Second+100*time.Millisecond, 12*time.Second+900*time.Millisecond)
	if r.p.HP != top/2 {
		t.Errorf("Healbot healed again %v after his last heal", 5900*time.Millisecond)
	}
	r.tick(13 * time.Second)
	if want := min(top/2+600, top); r.p.HP != want {
		t.Errorf("Healbot did not heal again 6 s after: %d, want %d", r.p.HP, want)
	}
}

// Healbot heals no one flagged for PvP and no one in a duel; the control, the
// same owner neither, healed.
func TestHealbotNeverHealsInAFightBetweenPlayers(t *testing.T) {
	r := newMercRig(t)
	r.callFamiliar(0)
	top := r.p.MaxHPNow()
	r.p.HP = top / 2
	r.p.pvp.on = true
	r.run(5*time.Second, 6*time.Second)
	if r.p.HP != top/2 {
		t.Error("Healbot healed a player flagged for PvP")
	}
	r.p.pvp.on = false
	_, foe := r.player("Foe", 0, 0)
	r.w.duels.add(&Duel{ID: 1, A: r.p.ServerID, B: foe.ServerID, who: [2]*Player{r.p, foe}, Phase: duelOn})
	r.run(6*time.Second+100*time.Millisecond, 7*time.Second)
	if r.p.HP != top/2 {
		t.Error("Healbot healed a player in a duel")
	}
	r.w.duels.remove(r.w.duels.of[r.p.ServerID])
	r.tick(7*time.Second + 100*time.Millisecond)
	if r.p.HP != min(top/2+600, top) {
		t.Errorf("out of the duel and unflagged, Healbot did not heal: %d", r.p.HP)
	}
}

// Healbot draws no hate (no creature's list takes him) and never attacks: a
// creature fighting his owner is not touched by him; he regenerates 2,000 a
// tick.
func TestHealbotDrawsNoHateNeverAttacks(t *testing.T) {
	r := newMercRig(t)
	fs := r.callFamiliar(0)
	crab := r.creature("Desert Crab", 28, 3)
	r.w.mu.Lock()
	r.w.addThreatLocked(crab, fs.npc.ServerID, 5000)
	r.w.addThreatLocked(crab, r.p.ServerID, 50)
	r.w.mu.Unlock()
	if _, hated := crab.Threats[fs.npc.ServerID]; hated || crab.CurrentEnemy != r.p.ServerID {
		t.Fatalf("the crab hates Healbot (%v) or fights %d", hated, crab.CurrentEnemy)
	}
	hp := crab.HP
	fs.npc.HP = 50000
	r.run(5*time.Second, 11*time.Second)
	if crab.HP != hp || !fs.npc.RoundStart.IsZero() {
		t.Errorf("Healbot fought: the crab %d -> %d, a round %v", hp, crab.HP, fs.npc.RoundStart)
	}
	if fs.npc.HP < 52000 {
		t.Errorf("Healbot regenerated to %d from 50000 over a regen tick, want 2000 more", fs.npc.HP)
	}
	if _, hated := crab.Threats[fs.npc.ServerID]; hated {
		t.Error("the crab came to hate Healbot")
	}
}

// Healbot goes at his owner's death, at a dismissal and at a logout.
func TestFamiliarGoes(t *testing.T) {
	r := newMercRig(t)
	fs := r.callFamiliar(0)
	r.p.HP = 0
	r.tick(5 * time.Second)
	if r.w.familiars[r.p.ServerID] != nil || r.w.NPCs[fs.npc.ServerID] != nil {
		t.Error("Healbot stayed at his owner's death")
	}
	r.p.HP = r.p.MaxHPNow()
	fs = r.callFamiliar(10 * time.Second)
	if name, ok := r.w.DismissFamiliar(r.sess); !ok || name != "Healbot" || r.w.NPCs[fs.npc.ServerID] != nil {
		t.Errorf("dismissed: %q %v, still in the world %v", name, ok, r.w.NPCs[fs.npc.ServerID] != nil)
	}
	fs = r.callFamiliar(20 * time.Second)
	r.w.RemovePlayer(r.p.ServerID)
	if len(r.w.familiars) != 0 || r.w.NPCs[fs.npc.ServerID] != nil {
		t.Error("Healbot stayed after his owner logged out")
	}
}
