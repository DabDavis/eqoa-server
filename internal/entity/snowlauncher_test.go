package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// The Automatic Snowball Launcher (snowlauncher.go; the owner, 2026-11-16:
// "lets give zerkziee an automatic snowball launcher, infinite ammo").

// launcherTick runs the launcher's system at d, as the tick does.
func (r *mercRig) launcherTick(d time.Duration) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount++
	r.w.snowLauncherSystem(r.at(d))
}

// target makes the rig's owner target e, by the id its session sees e by.
func (r *mercRig) target(e EntityID) {
	r.seeRig(e)
	r.p.TargetObjectID = 0x01010000 + uint32(e)
}

// Use toggles it; on, at another player targeted: a snowball at once, then
// none until LauncherPlayerGap has gone by (the anti-spam), one again at the
// gap; 1 HP a hit, never below 1; told to both; no PvP flag, fight or hate.
// Off, nothing more.
func TestLauncherFiresAtTheTargetedPlayer(t *testing.T) {
	r := newMercRig(t)
	zsess, z := r.player("Zed", 0, 0)
	r.target(z.ServerID)
	top := z.MaxHPNow()
	z.HP = top
	if got := r.w.ToggleSnowLauncher(r.sess, r.at(0)); got != LauncherOn || !r.w.SnowLauncherOn(r.sess) {
		t.Fatalf("the first use: %v", got)
	}
	r.sess.Queue().Clear()
	var hits []time.Duration
	last := z.HP
	for d := time.Duration(0); d <= 12*time.Second; d += 100 * time.Millisecond {
		r.launcherTick(d)
		if z.HP != last {
			hits, last = append(hits, d), z.HP
		}
	}
	want := []time.Duration{0, LauncherPlayerGap, 2 * LauncherPlayerGap}
	if len(hits) != len(want) {
		t.Fatalf("hits at %v, want %v", hits, want)
	}
	for i := range want {
		if hits[i] != want[i] {
			t.Errorf("hit %d at %v, want %v", i, hits[i], want[i])
		}
	}
	if z.HP != top-3 {
		t.Errorf("HP %d, want %d", z.HP, top-3)
	}
	if !saidTo(zsess, "Owner hits you with a snowball!") || !saidTo(r.sess, "You hit Zed with a snowball!") {
		t.Error("the hit was not told to both")
	}
	if z.pvp.on || r.p.pvp.on || z.AttackTarget != 0 || r.p.AttackTarget != 0 || !z.LastCombatTime.IsZero() || !r.p.LastCombatTime.IsZero() {
		t.Errorf("a fight came of it: pvp %v/%v, targets %d/%d", z.pvp.on, r.p.pvp.on, z.AttackTarget, r.p.AttackTarget)
	}
	// never below 1
	z.HP = 1
	for d := 20 * time.Second; d <= 40*time.Second; d += 100 * time.Millisecond {
		r.launcherTick(d)
	}
	if z.HP != 1 {
		t.Errorf("at 1 HP: %d", z.HP)
	}
	// off: nothing more
	if got := r.w.ToggleSnowLauncher(r.sess, r.at(41*time.Second)); got != LauncherOff || r.w.SnowLauncherOn(r.sess) {
		t.Fatalf("the second use: %v", got)
	}
	z.HP = top
	for d := 41 * time.Second; d <= 60*time.Second; d += 100 * time.Millisecond {
		r.launcherTick(d)
	}
	if z.HP != top {
		t.Errorf("off, a hit: HP %d", z.HP)
	}
}

// Two players hit by turns: each at most once a gap, and the launcher fires no
// oftener than LauncherEvery.
func TestLauncherCadenceAcrossTargets(t *testing.T) {
	r := newMercRig(t)
	_, z := r.player("Zed", 0, 0)
	_, y := r.player("Yara", 0, 0)
	r.seeRig(z.ServerID, y.ServerID)
	z.HP, y.HP = z.MaxHPNow(), y.MaxHPNow()
	r.w.ToggleSnowLauncher(r.sess, r.at(0))
	var shots []time.Duration
	lz, ly := z.HP, y.HP
	for d := time.Duration(0); d <= 20*time.Second; d += 100 * time.Millisecond {
		// he swaps his target every tick
		if (d/(100*time.Millisecond))%2 == 0 {
			r.p.TargetObjectID = 0x01010000 + uint32(z.ServerID)
		} else {
			r.p.TargetObjectID = 0x01010000 + uint32(y.ServerID)
		}
		r.launcherTick(d)
		if z.HP != lz || y.HP != ly {
			shots, lz, ly = append(shots, d), z.HP, y.HP
		}
	}
	for i := 1; i < len(shots); i++ {
		if shots[i]-shots[i-1] < LauncherEvery {
			t.Errorf("two shots %v apart: %v", shots[i]-shots[i-1], shots)
		}
	}
	if len(shots) < 6 {
		t.Errorf("too few shots: %v", shots)
	}
}

// Whom it fires at: the target alive, near, in sight, in his world; never
// himself, no one with nothing targeted. Each in turn is made the one ruled
// out; it waits (no shot) and fires once the target is good again.
func TestLauncherTargets(t *testing.T) {
	r := newMercRig(t)
	_, z := r.player("Zed", 0, 0)
	r.target(z.ServerID)
	r.w.ToggleSnowLauncher(r.sess, r.at(0))
	st := r.w.launchers[r.p.ServerID]
	now := r.at(time.Minute)
	target := func() Entity {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		return r.w.launcherTargetLocked(r.sess, st, now)
	}
	if got := target(); got != z {
		t.Fatalf("the control: %v", got)
	}
	cases := []struct {
		name       string
		set, unset func()
	}{
		{"dead", func() { z.HP = 0 }, func() { z.HP = z.MaxHPNow() }},
		{"far", func() { z.X += SnowballRange + 5 }, func() { z.X -= SnowballRange + 5 }},
		{"another world", func() { z.World = 2 }, func() { z.World = 0 }},
		{"GM hidden", func() { z.GMHidden = true }, func() { z.GMHidden = false }},
		{"nothing targeted", func() { r.p.TargetObjectID = 0 }, func() { r.target(z.ServerID) }},
		{"himself", func() { r.p.TargetObjectID = r.sess.PlayerObjectID() }, func() { r.target(z.ServerID) }},
		{"hit within the gap", func() { st.hit[z.ServerID] = now.Add(-LauncherPlayerGap + time.Second) }, func() { delete(st.hit, z.ServerID) }},
	}
	for _, c := range cases {
		c.set()
		if got := target(); got != nil {
			t.Errorf("%s: %v picked", c.name, got)
		}
		c.unset()
	}
	st.hit[z.ServerID] = now.Add(-LauncherPlayerGap)
	if got := target(); got != z {
		t.Errorf("after the gap: %v", got)
	}
	if _, ok := st.hit[z.ServerID]; ok {
		t.Error("a spent entry was kept")
	}
}

// At a creature: its visual and nothing more (the Snowball has no harm for
// creatures): no HP off, no hate, no fight on either side; the owner told
// once, not every shot.
func TestLauncherAtACreatureIsForTheLook(t *testing.T) {
	r := newMercRig(t)
	crab := r.creature("Desert Crab", 28, 5)
	r.target(crab.ServerID)
	hp := crab.HP
	r.w.ToggleSnowLauncher(r.sess, r.at(0))
	r.sess.Queue().Clear()
	for d := time.Duration(0); d <= 10*time.Second; d += 100 * time.Millisecond {
		r.launcherTick(d)
	}
	told, shots := 0, 0
	for _, m := range r.sess.Queue().GetPending() {
		b := m.Data()
		if bytes.Contains(b, wide("You pelt Desert Crab with snowballs!")) {
			told++
		}
		if opOfMsg(b) == protocol.OpcodeCastingCombat {
			at := 11 + 4*int(b[10])
			if binary.LittleEndian.Uint32(b[6:]) == 0x01010000+uint32(crab.ServerID) && binary.LittleEndian.Uint32(b[at+4:]) == SnowballFX {
				shots++
			}
		}
	}
	if shots != 7 {
		t.Errorf("%d snowballs at the crab in 10 s, want 7 (one each %v)", shots, LauncherEvery)
	}
	if told != 1 {
		t.Errorf("the owner told %d times, want once", told)
	}
	if crab.HP != hp || crab.CurrentEnemy != 0 || len(crab.Threats) != 0 || r.p.AttackTarget != 0 || !r.p.LastCombatTime.IsZero() {
		t.Errorf("harm or a fight: HP %d/%d, enemy %d, threats %d", crab.HP, hp, crab.CurrentEnemy, len(crab.Threats))
	}
	// out of sight range: it waits
	crab.X = r.p.X + SnowballRange + 5
	r.sess.Queue().Clear()
	for d := 11 * time.Second; d <= 15*time.Second; d += 100 * time.Millisecond {
		r.launcherTick(d)
	}
	if n := len(r.castsSeen()); n != 0 {
		t.Errorf("%d snowballs at a crab out of range", n)
	}
}

// It stops by itself, told, on a death, another world, a long move or a world
// loading; a logout stops it unsaid; dead, it will not start.
func TestLauncherStops(t *testing.T) {
	cases := []struct {
		name string
		do   func(r *mercRig)
	}{
		{"dead", func(r *mercRig) { r.p.HP = 0 }},
		{"another world", func(r *mercRig) { r.p.World = 3 }},
		{"moved far", func(r *mercRig) { r.p.X += petLost + 10 }},
		{"loading a world", func(r *mercRig) { r.sess.HoldStateChannels(time.Minute) }},
	}
	for _, c := range cases {
		r := newMercRig(t)
		r.w.ToggleSnowLauncher(r.sess, r.at(0))
		r.launcherTick(0)
		r.sess.Queue().Clear()
		c.do(r)
		r.launcherTick(time.Second)
		if r.w.launchers[r.p.ServerID] != nil {
			t.Errorf("%s: still on", c.name)
		}
		if !saidTo(r.sess, launcherStopLine) {
			t.Errorf("%s: not told", c.name)
		}
	}
	// a logout
	r := newMercRig(t)
	r.w.ToggleSnowLauncher(r.sess, r.at(0))
	r.w.RemovePlayer(r.p.ServerID)
	if len(r.w.launchers) != 0 {
		t.Error("on after a logout")
	}
	// dead, it will not start
	r = newMercRig(t)
	r.p.HP = 0
	if got := r.w.ToggleSnowLauncher(r.sess, r.at(0)); got != LauncherDead || len(r.w.launchers) != 0 {
		t.Errorf("dead: %v", got)
	}
}
