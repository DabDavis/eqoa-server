package entity

import (
	"math"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The chase (chase.go): a creature runs at an enemy out of its reach,
// fights it in reach, turns to it, gives up past its leash and runs home,
// and its wander leaves the fight's movement alone (F3 20261025-165533).

// chaseTick is one AI tick at d into the fixture's clock: the fights and the
// ways home.
func (c *caster) chaseTick(d time.Duration) {
	c.w.mu.Lock()
	c.w.aiSystem(c.at(d))
	c.w.homeSystem(c.at(d))
	c.w.mu.Unlock()
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// A crab pulled from 28 away runs at its puller (the run, its velocity
// toward it, turned to it) and swings at nothing on the way; in reach it
// stands and its rounds begin. The control: until 2026-10-25 the AI dropped
// an enemy more than 10 away at once, and nothing moved the crab.
func TestACreatureRunsAtItsEnemyAndFightsWhenItReaches(t *testing.T) {
	c := newCaster(t)
	c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Desert Crab", 34, 2000, 2000
	c.npc.OffMod, c.npc.WeaponDamage = NPCOffMod(34), NPCWeaponDamage(34)
	c.npc.LeashRadius = DefaultLeash
	c.p.X, c.p.HP, c.p.MaxHP = 130, 5000, 5000 // 28 off the crab at 102
	c.npc.CurrentEnemy = c.p.ServerID
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}

	c.chaseTick(0)
	c.chaseTick(100 * time.Millisecond)
	if c.npc.CurrentEnemy != c.p.ServerID {
		t.Fatal("dropped an enemy 28 away instead of running at it")
	}
	if !c.npc.moving() || c.npc.Loco != LocoRun || !near(c.npc.VelX, NPCRunSpeed) || c.npc.VelZ != 0 {
		t.Errorf("not running at it: velocity (%.2f, %.2f), loco %d", c.npc.VelX, c.npc.VelZ, c.npc.Loco)
	}
	if !near(c.npc.Facing, math.Pi/2) {
		t.Errorf("facing %.3f, want pi/2 (toward +x)", c.npc.Facing)
	}
	if !near(c.npc.X, 102+NPCRunSpeed*0.1) {
		t.Errorf("x %.3f after a tenth of a second", c.npc.X)
	}
	if got := c.sent(); len(got) != 0 {
		t.Errorf("sent %x while closing in", got)
	}

	// it comes in a little inside its reach and stops there
	var swungAt time.Duration
	for d := 200 * time.Millisecond; d <= 5*time.Second && swungAt == 0; d += 100 * time.Millisecond {
		c.chaseTick(d)
		for _, m := range c.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat {
				swungAt = d
			}
		}
	}
	if swungAt == 0 {
		t.Fatalf("never swung: at %.2f, %.2f from its enemy", c.npc.X, c.p.X-c.npc.X)
	}
	// in its reach after a run of about 23.5 at Sandstorm's 9.9 a second (it
	// eases off in the last unit and a quarter), the first tick in reach;
	// this test asked 2.6 to 3 s of the player's base run (8.83) and the
	// easing over 2.5 units until F3 pass 16
	if swungAt < 2300*time.Millisecond || swungAt > 2500*time.Millisecond {
		t.Errorf("swung at %v, want 2.3 to 2.5 s", swungAt)
	}
	// then it eases in to half a step inside its reach, and stands, its round
	// going on meanwhile
	hp := c.p.HP
	var stoodAt time.Duration
	for d := swungAt + 100*time.Millisecond; d <= swungAt+1500*time.Millisecond && stoodAt == 0; d += 100 * time.Millisecond {
		c.chaseTick(d)
		if !c.npc.moving() {
			stoodAt = d
		}
	}
	stop := npcStop(c.npc)
	if gap := c.p.X - c.npc.X; stoodAt == 0 || gap < stop-0.01 || gap > stop+0.13 {
		t.Errorf("stood (at %v) %.2f off its enemy, want %.2f", stoodAt, gap, stop)
	}
	if c.npc.Loco != LocoStand {
		t.Errorf("standing with loco %d", c.npc.Loco)
	}
	for d := stoodAt + 100*time.Millisecond; d <= swungAt+1600*time.Millisecond; d += 100 * time.Millisecond {
		c.chaseTick(d)
	}
	if c.p.HP >= hp {
		t.Error("no hit in the round's first 1.6 s")
	}
}

// A creature following a player who backs off at 3 a second moves with a
// velocity of the pace it moves at, every tick, and the walk or run to match: its
// channel never says it stands while it moves, which CLIENT drew as a slide
// (seen live, 2026-10-25). It keeps hitting in reach. The control: a step to
// a point that stops dead on arriving, as the roam's does.
func TestAFollowingCreatureWalksWithItsVelocity(t *testing.T) {
	c := newCaster(t)
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 18, 900, 900
	c.p.X, c.p.HP, c.p.MaxHP = 105, 5000, 5000 // in reach of the creature at 102
	c.npc.CurrentEnemy = c.p.ServerID
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}
	c.chaseTick(0)
	hp := c.p.HP
	moves, slides := 0, 0
	for d := 100 * time.Millisecond; d <= 8*time.Second; d += 100 * time.Millisecond {
		c.p.X += 0.3
		x := c.npc.X
		c.chaseTick(d)
		if moved := c.npc.X - x; moved > 1e-4 {
			moves++
			want := LocoWalk
			if moved/0.1 >= NPCRunFrom {
				want = LocoRun // the burst that catches it up
			}
			if v := moved / 0.1; !near(c.npc.VelX, v) || c.npc.Loco != want {
				slides++
			}
		}
	}
	if moves < 75 {
		t.Fatalf("moved on %d ticks of 80 after a player backing off: in bursts", moves)
	}
	if slides != 0 {
		t.Errorf("%d of %d ticks moved with a velocity or animation not its own", slides, moves)
	}
	if c.p.HP >= hp {
		t.Error("never hit the player it followed")
	}

	// the player's 0x40 every other tick (its place two ticks' worth at a
	// time, its velocity 3 a second): followed at its pace every tick, not
	// a walk and a stand by turns
	stood := 0
	for d := 8100 * time.Millisecond; d <= 12*time.Second; d += 100 * time.Millisecond {
		if (d/(100*time.Millisecond))%2 == 0 {
			c.p.X += 0.6
		}
		c.p.VelX = 3
		x := c.npc.X
		c.chaseTick(d)
		if c.npc.X-x < 1e-4 || c.npc.Loco != LocoWalk {
			stood++
		}
	}
	if stood != 0 {
		t.Errorf("stood or ran on %d of 40 ticks following a player whose 0x40 came every other tick", stood)
	}

	// a player backing off at 6 a second: followed at about where it would
	// stop, by the lead its velocity gives (on the gap alone it trails 0.9
	// further, at the edge of its reach)
	stop, far := npcStop(c.npc), 0
	for d := 12100 * time.Millisecond; d <= 16*time.Second; d += 100 * time.Millisecond {
		c.p.X += 0.6
		c.p.VelX = 6
		c.chaseTick(d)
		if d > 13*time.Second && c.p.X-c.npc.X > stop+0.3 {
			far++
		}
	}
	if far != 0 {
		t.Errorf("trailed past %.2f on %d ticks following a player backing off at 6 a second", stop+0.3, far)
	}
}

// A creature turns to its enemy as it moves round it in reach, and stands.
// The control: until 2026-10-25 a fighting creature kept the facing it had.
func TestAFightTurnsTheCreatureToItsEnemy(t *testing.T) {
	c := newCaster(t)
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 10, 500, 500
	c.p.HP, c.p.MaxHP = 5000, 5000 // at (100, 100), the crab at (102, 100)
	c.npc.CurrentEnemy = c.p.ServerID
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}
	c.chaseTick(0)
	if !near(c.npc.Facing, 3*math.Pi/2) {
		t.Errorf("facing %.3f, want 3pi/2 (toward -x)", c.npc.Facing)
	}
	c.p.X, c.p.Z = 102, 103
	c.chaseTick(100 * time.Millisecond)
	if !near(c.npc.Facing, 0) || c.npc.X != 102 || c.npc.Z != 100 || c.npc.moving() {
		t.Errorf("facing %.3f at (%.2f, %.2f), want 0 where it stood", c.npc.Facing, c.npc.X, c.npc.Z)
	}
}

// A player who outruns a creature takes it past its leash: it gives up (its
// threats and round dropped) and runs back to where the fight began, a
// creature that does not wander facing as it was spawned. The control:
// with no leash it runs on after its enemy for good.
func TestACreatureGivesUpPastItsLeashAndRunsHome(t *testing.T) {
	c := newCaster(t)
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 10, 500, 500
	c.npc.SpawnX, c.npc.SpawnZ, c.npc.SpawnFacing = 102, 100, 1.25
	c.npc.LeashRadius = DefaultLeash
	c.p.X, c.p.HP, c.p.MaxHP = 110, 5000, 5000
	c.npc.CurrentEnemy = c.p.ServerID
	c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}

	var gaveUp time.Duration
	d := time.Duration(0)
	for ; d <= 60*time.Second; d += 100 * time.Millisecond {
		c.chaseTick(d)
		if c.npc.CurrentEnemy == 0 {
			gaveUp = d
			break
		}
		c.p.X += 1.2 // 12 a second: faster than the creature
	}
	if gaveUp == 0 {
		t.Fatalf("still chasing after 60 s, %.1f from where it began", c.npc.X-102)
	}
	if c.npc.Threats != nil || !c.npc.RoundStart.IsZero() || !c.npc.chase.returning {
		t.Errorf("gave up but kept its threats %v or round, or is not going home", c.npc.Threats)
	}
	if out := c.npc.X - 102; out < DefaultLeash || out > DefaultLeash+1 {
		t.Errorf("gave up %.2f from where it began, want just past %.0f", out, DefaultLeash)
	}
	c.chaseTick(d + 100*time.Millisecond)
	if !c.npc.moving() || c.npc.Loco != LocoRun || c.npc.VelX >= 0 {
		t.Errorf("not running home: velocity (%.2f, %.2f), loco %d", c.npc.VelX, c.npc.VelZ, c.npc.Loco)
	}
	for e := d + 200*time.Millisecond; e <= d+60*time.Second && c.npc.chase.returning; e += 100 * time.Millisecond {
		c.chaseTick(e)
	}
	if c.npc.chase.returning || c.npc.X != 102 || c.npc.Z != 100 || c.npc.moving() {
		t.Errorf("not home: at (%.2f, %.2f), returning %v", c.npc.X, c.npc.Z, c.npc.chase.returning)
	}
	if c.npc.Facing != 1.25 {
		t.Errorf("facing %.3f at its post, want its spawn's 1.25", c.npc.Facing)
	}
	if len(c.w.returning) != 0 {
		t.Errorf("%d still listed on the way home", len(c.w.returning))
	}
}

// A wandering creature that fights: the wander (which runs after the AI in
// a tick) leaves the chase's run alone, so the channel shows a run and not a
// slide. The control: the wander's old stop of a fought creature.
func TestTheWanderLeavesTheChaseAlone(t *testing.T) {
	w := roamWorld(t)
	w.SessionManager = session.NewManager() // where a hit looks for its player's session
	npc := addNPC(w, 100, "beetle", 1020, 1000, creatureModel, 0)
	p := w.Players[1]
	p.HP, p.MaxHP = 500, 500
	npc.CurrentEnemy = p.ServerID
	npc.Threats = map[EntityID]int64{p.ServerID: 5}
	now := time.Now()
	tick := func() {
		w.mu.Lock()
		w.TickCount++
		w.aiSystem(now)
		w.homeSystem(now)
		w.roamSystem(now)
		w.mu.Unlock()
		now = now.Add(100 * time.Millisecond)
	}
	tick()
	tick()
	if !npc.moving() || npc.Loco != LocoRun || npc.VelX >= 0 {
		t.Fatalf("the wander stopped the run: velocity (%.2f, %.2f), loco %d", npc.VelX, npc.VelZ, npc.Loco)
	}
	for i := 0; i < 40; i++ {
		tick()
	}
	if gap := npc.X - p.X; gap > npcReach(npc) {
		t.Errorf("still %.2f from its enemy after 4 s", gap)
	}
}
