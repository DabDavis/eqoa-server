package entity

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// newQuitRig is a level 20 player at 40 of 300 HP, a bear fighting it.
func newQuitRig(t *testing.T) (*World, *session.Session, *Player, *NPC) {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Jedd", Level: 20, HP: 40, MaxHP: 300, Power: 25, MaxPower: 200,
		X: 1000, Z: 1000, World: 0}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	bear := &NPC{ServerID: 3000, Name: "grizzly bear", Level: 22, HP: 900, MaxHP: 900, ModelID: 0x1234, Size: 1,
		X: 1003, Z: 1000, World: 0, NPCType: 0x0100}
	w.NPCs[bear.ServerID] = bear
	w.SpatialIndex.Insert(bear.ServerID, bear.X, bear.Z)
	w.mu.Lock()
	w.updateAggro(bear, p.ServerID, 10)
	w.mu.Unlock()
	return w, sess, p, bear
}

// A Quit with a creature on the character leaves it in the world, its own
// swing stopped, until the creature stops; QuitLinger at the latest. The
// control: a Quit with nothing on it ends at once.
func TestAQuitInAFightStaysUntilTheFightIsOver(t *testing.T) {
	w, sess, p, bear := newQuitRig(t)
	p.AutoAttack = true
	t0 := time.Unix(1_800_000_000, 0)
	if n := w.QuitInFight(sess); n != 1 {
		t.Fatalf("a quit with the bear on it: %d fighting, want 1", n)
	}
	if p.AutoAttack {
		t.Error("the character left swinging")
	}
	if w.QuitStayOver(sess, t0, t0.Add(5*time.Second)) {
		t.Fatal("its stay ended with the bear still on it")
	}
	if !w.QuitStayOver(sess, t0, t0.Add(QuitLinger)) {
		t.Error("its stay outlasted QuitLinger")
	}
	// the bear leashes: the fight is over
	w.mu.Lock()
	w.dropAggroLocked(p.ServerID)
	w.mu.Unlock()
	if bear.CurrentEnemy != 0 || !w.QuitStayOver(sess, t0, t0.Add(6*time.Second)) {
		t.Error("its stay did not end with the fight")
	}
	// the control: nothing on it
	if n := w.QuitInFight(sess); n != 0 {
		t.Errorf("a quit with nothing on it: %d fighting", n)
	}
}

// Killed in its stay, a character waits for its respawn, past QuitLinger
// too: it leaves the world from its bind point, its debt paid as any death's.
func TestAQuitInAFightDiesAsAnyDeath(t *testing.T) {
	w, sess, p, _ := newQuitRig(t)
	t0 := time.Unix(1_800_000_000, 0)
	w.QuitInFight(sess)
	w.mu.Lock()
	w.playerDiedLocked(nil, p, t0.Add(10*time.Second))
	w.mu.Unlock()
	if p.Debt == 0 {
		t.Error("a death in the stay left no debt")
	}
	if w.QuitStayOver(sess, t0, t0.Add(QuitLinger+5*time.Second)) {
		t.Fatal("its stay ended before its respawn")
	}
	w.mu.Lock()
	due := w.playerRespawnSystem(t0.Add(10*time.Second + PlayerRespawnDelay))
	w.mu.Unlock()
	if len(due) != 1 || p.HP != p.MaxHP {
		t.Fatalf("the respawn: %d due, %d/%d HP", len(due), p.HP, p.MaxHP)
	}
	if !w.QuitStayOver(sess, t0, t0.Add(10*time.Second+PlayerRespawnDelay)) {
		t.Error("its stay did not end after its respawn")
	}
}

// A training dummy is no fight to stay in.
func TestAQuitAtADummyEndsAtOnce(t *testing.T) {
	w, sess, p, bear := newQuitRig(t)
	w.mu.Lock()
	w.dropAggroLocked(p.ServerID)
	w.mu.Unlock()
	bear.Dummy = TrainingDummy
	bear.DummyTarget = p.ServerID
	if n := w.QuitInFight(sess); n != 0 {
		t.Errorf("a quit at a dummy: %d fighting, want 0", n)
	}
}

// What a character leaves with: its hit points and power alive, nothing
// dead (it comes back whole); and they come back within its maxima now, with
// a tick of its HoT and PoT for every RegenInterval it was away (it rested).
func TestVitalsLeaveAndComeBack(t *testing.T) {
	w, sess, p, _ := newQuitRig(t)
	if hp, power, keep, ok := w.Vitals(sess); !ok || !keep || hp != 40 || power != 25 {
		t.Fatalf("kept %d HP %d power (keep %v, ok %v), want 40 25", hp, power, keep, ok)
	}
	p.HP = 0
	if _, _, keep, ok := w.Vitals(sess); !ok || keep {
		t.Errorf("dead: keep %v ok %v, want a whole return", keep, ok)
	}
	p.HP = 300
	if hp, power, ok := w.RestoreVitals(sess, 40, 25, 0); !ok || p.HP != 40 || p.Power != 25 || hp != 40 || power != 25 {
		t.Errorf("back at once: %d/%d HP %d power, want 40 25", p.HP, p.MaxHP, p.Power)
	}
	// a minute away: ten ticks (HoT 6, PoT 4: a 50 of 300 and of 200)
	if w.RestoreVitals(sess, 40, 25, time.Minute+3*time.Second); p.HP != 40+10*p.HoTNow() || p.Power != 25+10*p.PoTNow() {
		t.Errorf("a minute away: %d HP %d power, want %d %d", p.HP, p.Power, 40+10*p.HoTNow(), 25+10*p.PoTNow())
	}
	if p.HoTNow() != 6 || p.PoTNow() != 4 {
		t.Errorf("the rig's HoT %d PoT %d, want 6 4", p.HoTNow(), p.PoTNow())
	}
	// long enough to rest whole
	if w.RestoreVitals(sess, 40, 25, 8*time.Hour); p.HP != p.MaxHPNow() || p.Power != p.MaxPowerNow() {
		t.Errorf("rested: %d/%d HP %d/%d power", p.HP, p.MaxHPNow(), p.Power, p.MaxPowerNow())
	}
	if w.RestoreVitals(sess, 9999, 9999, 0); p.HP != p.MaxHPNow() || p.Power != p.MaxPowerNow() {
		t.Errorf("past its maxima: %d/%d HP %d/%d power", p.HP, p.MaxHPNow(), p.Power, p.MaxPowerNow())
	}
	if w.RestoreVitals(sess, 0, -5, 0); p.HP != 1 || p.Power != 0 {
		t.Errorf("below: %d HP %d power, want 1 0", p.HP, p.Power)
	}
}
