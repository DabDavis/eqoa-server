package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// deathFixture is one player in a session, two NPCs on its threat lists and a
// second player one of them also hates.
type deathFixture struct {
	w      *World
	sess   *session.Session
	p      *Player
	other  *Player
	killer *NPC
	bystd  *NPC
	t0     time.Time
}

func newDeathFixture(t *testing.T) *deathFixture {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 7), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Tester", Level: 1, HP: 1, MaxHP: 47, Power: 3, MaxPower: 41,
		Breath: 40, X: 100, Y: 10, Z: 100}
	w.Players[p.ServerID] = p
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x00AA0001)
	other := &Player{ServerID: 1002, Name: "Other", Level: 1, HP: 47, MaxHP: 47, X: 104, Y: 10, Z: 100}
	w.Players[other.ServerID] = other
	// The killer hits hard enough that one blow is death, and hates the
	// other player a little too.
	killer := &NPC{ServerID: 2001, Name: "Training Dummy Lv50", Level: 50, HP: 5000, MaxHP: 5000,
		OffMod: 400, WeaponDamage: 60, X: 102, Y: 10, Z: 100, CurrentEnemy: p.ServerID,
		Threats: map[EntityID]int64{p.ServerID: 500, other.ServerID: 100}}
	bystd := &NPC{ServerID: 2002, Name: "Crab", Level: 2, HP: 30, MaxHP: 30, X: 103, Y: 10, Z: 101,
		CurrentEnemy: p.ServerID, Threats: map[EntityID]int64{p.ServerID: 50}}
	w.NPCs[killer.ServerID] = killer
	w.NPCs[bystd.ServerID] = bystd
	return &deathFixture{w: w, sess: sess, p: p, other: other, killer: killer, bystd: bystd,
		t0: time.Unix(1_700_000_000, 0)}
}

// kill lands the killer's blows until the player is dead, the way the AI's
// pass does (npcAttackPlayer under the lock).
func (f *deathFixture) kill(t *testing.T) {
	t.Helper()
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	for i := 0; i < 20 && !f.p.IsDead(); i++ {
		f.w.npcAttackPlayer(f.killer, f.p.ServerID, f.t0)
	}
	if !f.p.IsDead() {
		t.Fatal("the killer never killed the player")
	}
}

// died counts the 0x005E queued for the player's client.
func (f *deathFixture) died() int {
	n := 0
	for _, m := range f.sess.Queue().PeekPending() {
		d := m.Data()
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCharacterDied {
			n++
		}
	}
	return n
}

// A death drops the dead player off every NPC's threat list, at once: the NPC
// that also hated someone else turns on them (the old AI pass, finding its
// enemy dead, forgot every threat it held), and the one that hated only the
// corpse stands down. The death is told once, whatever hits or drowning
// ticks the corpse would have taken after it.
func TestDeathDropsAggroAndIsToldOnce(t *testing.T) {
	f := newDeathFixture(t)
	f.kill(t)

	if got := f.died(); got != 1 {
		t.Fatalf("%d CharacterDied queued at the death, want 1", got)
	}
	if _, ok := f.killer.Threats[f.p.ServerID]; ok {
		t.Error("the killer still lists the corpse")
	}
	if f.killer.CurrentEnemy != f.other.ServerID {
		t.Errorf("killer's enemy %d, want the other player %d", f.killer.CurrentEnemy, f.other.ServerID)
	}
	if f.killer.Threats[f.other.ServerID] != 100 {
		t.Errorf("killer's threat on the other player %d, want 100 kept", f.killer.Threats[f.other.ServerID])
	}
	if f.bystd.CurrentEnemy != 0 || len(f.bystd.Threats) != 0 {
		t.Errorf("the crab still fights the corpse: enemy %d threats %v", f.bystd.CurrentEnemy, f.bystd.Threats)
	}

	// The AI's next pass turns the killer on the other player instead of
	// clearing it, and nobody hits the corpse.
	f.w.mu.Lock()
	f.w.TickCount = 30
	f.w.aiSystem(f.t0.Add(3 * time.Second))
	f.w.mu.Unlock()
	if f.killer.CurrentEnemy != f.other.ServerID {
		t.Errorf("after the AI pass the killer's enemy is %d, want %d", f.killer.CurrentEnemy, f.other.ServerID)
	}
	if f.p.HP != 0 {
		t.Errorf("the corpse has %d HP", f.p.HP)
	}

	// More blows and a drowning pass on the corpse: still one 0x005E.
	f.w.mu.Lock()
	f.w.npcAttackPlayer(f.killer, f.p.ServerID, f.t0.Add(4*time.Second))
	f.p.Animation = AnimSwimSlow
	f.p.Breath, f.p.DrowningCounter = 0, 54
	f.w.TickCount = 36
	f.w.breathSystem(f.t0.Add(5 * time.Second))
	f.w.mu.Unlock()
	if got := f.died(); got != 1 {
		t.Errorf("%d CharacterDied after more damage to the corpse, want 1", got)
	}
}

// The respawn comes PlayerRespawnDelay after the death, not a tick sooner,
// and only once: HP and Power at their maximum, breath full, the hook handed
// the player and its session after the lock is let go.
func TestPlayerRespawnsAfterTheDelay(t *testing.T) {
	f := newDeathFixture(t)
	var calls []*Player
	f.w.OnPlayerRespawn(func(sess *session.Session, p *Player) {
		if sess != f.sess {
			t.Errorf("hook given session %p, want the player's %p", sess, f.sess)
		}
		// the lock is free: the hook moves the player through locked methods
		if _, ok := f.w.PlayerSnapshot(p.ServerID); !ok {
			t.Error("snapshot under the hook failed")
		}
		calls = append(calls, p)
	})
	f.kill(t)

	f.w.Tick(f.t0.Add(PlayerRespawnDelay - 100*time.Millisecond))
	if len(calls) != 0 || f.p.HP != 0 || !f.p.respawnPending() {
		t.Fatalf("respawned early: calls %d, HP %d", len(calls), f.p.HP)
	}

	f.w.Tick(f.t0.Add(PlayerRespawnDelay))
	if len(calls) != 1 || calls[0] != f.p {
		t.Fatalf("hook calls %d at the delay, want 1 with the player", len(calls))
	}
	if f.p.HP != f.p.MaxHP || f.p.Power != f.p.MaxPower {
		t.Errorf("respawned at %d/%d HP, %d/%d power, want both full", f.p.HP, f.p.MaxHP, f.p.Power, f.p.MaxPower)
	}
	if f.p.Breath != BreathFull || f.p.respawnPending() || f.p.IsDead() {
		t.Errorf("breath %d, pending %v, dead %v after the respawn", f.p.Breath, f.p.respawnPending(), f.p.IsDead())
	}

	f.w.Tick(f.t0.Add(PlayerRespawnDelay + time.Second))
	if len(calls) != 1 {
		t.Errorf("hook called %d times, want once", len(calls))
	}
}

// A corpse does not heal: out of combat long enough to regenerate, it still
// waits at 0 HP for the respawn (the old regen raised it to 1 HP and more,
// alive to the server and dead on its client's screen).
func TestCorpseDoesNotRegenerate(t *testing.T) {
	f := newDeathFixture(t)
	f.kill(t)
	f.p.LastCombatTime = f.t0.Add(-time.Minute)

	f.w.mu.Lock()
	f.w.TickCount = 20
	f.w.regenSystem(f.t0.Add(12 * time.Second))
	f.w.mu.Unlock()
	if f.p.HP != 0 || f.p.Power != 3 {
		t.Errorf("corpse regenerated to %d HP, %d power", f.p.HP, f.p.Power)
	}

	// and a living player beside it does (the skip is the corpse's alone)
	f.other.HP = 10
	f.w.mu.Lock()
	f.w.TickCount = 40
	f.w.regenSystem(f.t0.Add(14 * time.Second))
	f.w.mu.Unlock()
	if f.other.HP <= 10 {
		t.Errorf("the living player did not regenerate: %d HP", f.other.HP)
	}
}
