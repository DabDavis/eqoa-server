package entity

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A flyer's way, every one of them a fault for anyone else: straight up off
// the ground, holding in the air, on through a wall, down through the ground's
// face, and at twice the speed.
func (r *mcRig) flyAway() (refused int) {
	for i := 1; i <= 30; i++ {
		if !r.report(float32(i)*0.9, float32(i), 0, 100) { // up
			refused++
		}
	}
	for i := 0; i < 30; i++ {
		if !r.report(27, 30, 0, 100) { // holding
			refused++
		}
	}
	for i := 1; i <= 110; i++ {
		if !r.report(27+float32(i), 30, 0, 100) { // on through the wall at x 100
			refused++
		}
	}
	for i := 1; i <= 40; i++ {
		if !r.report(137, 30-float32(i), 0, 100) { // down through the ground at 0
			refused++
		}
	}
	return refused
}

// The check leaves a flyer alone, in every mode, in a word: all of it taken,
// nothing moved back, no MOVECHECK line; the walls and the speed apply again
// the moment it does not fly. The control: the same way without #fly is refused
// and moved back in enforce mode and logged in log mode, so the way is one the
// check would judge.
func TestAFlyerIsNotJudged(t *testing.T) {
	var lines bytes.Buffer
	was := log.Writer()
	log.SetOutput(&lines)
	t.Cleanup(func() { log.SetOutput(was) })
	checks := func() int { return strings.Count(lines.String(), "MOVECHECK") }
	for _, mode := range []MoveCheckMode{MoveCheckEnforce, MoveCheckLog} {
		r := newMCRig(t, mode, 0, 0, 0)
		r.w.setFlyingLocked(r.p, true, r.now)
		lines.Reset()
		if n := r.flyAway(); n != 0 || len(r.p.mv.fixes) != 0 || checks() != 0 {
			t.Errorf("%s: flying, %d reports refused, %d moves back, %d lines:\n%s", mode, n, len(r.p.mv.fixes), checks(), lines.String())
		}
		if r.p.Y != -10 || r.p.X != 137 {
			t.Errorf("%s: the place not taken: (%.1f, %.1f)", mode, r.p.X, r.p.Y)
		}

		r = newMCRig(t, mode, 0, 0, 0)
		lines.Reset()
		n := r.flyAway()
		if mode == MoveCheckEnforce && (n == 0 || len(r.p.mv.fixes) == 0) {
			t.Errorf("the control: not flying, %d refused, %d moves back", n, len(r.p.mv.fixes))
		}
		if checks() == 0 {
			t.Errorf("the control, %s: not flying, no MOVECHECK line", mode)
		}
	}
}

// Off, the player falls and walks as normal from where it is: its next reports
// are not judged while its client hears of it, and then they are, from where it
// is. An honest fall from the height is taken whole, and a client that goes on
// holding in the air is moved back, as anyone is.
func TestFlyOffRestoresTheCheck(t *testing.T) {
	fall := func(r *mcRig) (refused int) {
		y := r.p.Y
		for i := 1; i <= 25; i++ { // 2.5 s of the game's fall, its reports at their cadence
			y -= 19.6 * float32(i) * 0.1 * 0.1 * 0.85
			if y < 0 {
				y = 0
			}
			if !r.report(50, y, 0, 100) {
				refused++
			}
		}
		return refused
	}
	r := newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	r.w.setFlyingLocked(r.p, true, r.now)
	for i := 1; i <= 60; i++ {
		r.report(50, float32(i), 0, 100)
	}
	if r.p.Y != 60 {
		t.Fatalf("flown to y %.1f", r.p.Y)
	}
	r.w.setFlyingLocked(r.p, false, r.now)
	// the grace: it may still rise a little, its client not told yet
	if !r.report(50, 60.5, 0, 100) || !r.report(50, 61, 0, 100) {
		t.Fatal("a report on its way down from flight was refused in the grace")
	}
	if !r.p.flyingNow(r.now) {
		t.Fatal("no grace")
	}
	if n := fall(r); n != 0 {
		t.Errorf("an honest fall from the flight: %d reports refused", n)
	}
	if r.p.flyingNow(r.now) || r.p.Flying {
		t.Fatal("still left alone after the grace")
	}
	if !r.p.mv.init {
		t.Error("the check is not back on its reports")
	}

	// the control: off, and the client goes on holding in the air
	r = newMCRig(t, MoveCheckEnforce, 0, 0, 0)
	r.w.setFlyingLocked(r.p, true, r.now)
	for i := 1; i <= 60; i++ {
		r.report(50, float32(i), 0, 100)
	}
	r.w.setFlyingLocked(r.p, false, r.now)
	for i := 0; i < 80; i++ {
		r.report(50, 60, 0, 100)
	}
	if len(r.p.mv.fixes) == 0 {
		t.Error("off, held up in the air for 8 s: not moved back")
	}
}

// Silent in the air is no fault of a flyer's: hovering sends nothing, and the
// check in enforce mode puts down anyone else who does that. The control is
// TestMoveCheckSilentInTheAir.
func TestAFlyerHoversSilently(t *testing.T) {
	r := newMCRig(t, MoveCheckEnforce, 205, 10, 5)
	r.report(211, 9.9, 5, 100) // off the platform's edge
	r.w.setFlyingLocked(r.p, true, r.now)
	r.report(211, 30, 5, 100)
	r.p.mv.init, r.p.mv.judged, r.p.mv.air = true, true, true // as the check last left it
	for i := 0; i < 50; i++ {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.mu.Lock()
		r.w.TickCount++
		r.w.silentAirSystem(r.now)
		r.w.mu.Unlock()
	}
	if r.p.Y != 30 {
		t.Errorf("hovering, moved to y %.2f", r.p.Y)
	}
}

// The walker flags a flyer's client sends are the place's own, so flight alone
// runs no breath down: over the sea, in the air, all of it kept; under it, as
// anyone. In enforce mode the server's own water says the same. The control is
// the same player swimming.
func TestFlightAloneRunsNoBreathDown(t *testing.T) {
	for _, mode := range []MoveCheckMode{MoveCheckEnforce, MoveCheckLog} {
		r := newMCRig(t, mode, 0, 50, 600) // the sea's surface is at 5
		r.w.setFlyingLocked(r.p, true, r.now)
		r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: 0, Y: 50, Z: 600, World: r.p.World, HasWorld: true, At: r.now,
			WalkFlags: 0x0100, HasWalkFlags: true}) // TRACK_HEADING alone
		step := func() {
			r.w.mu.Lock()
			for i := 0; i < 30; i++ { // 18 s: three of breath's steps, and some left
				r.w.TickCount += 6
				r.w.breathSystem(r.now)
			}
			r.w.mu.Unlock()
		}
		step()
		if r.p.Underwater || r.p.Breath != BreathFull {
			t.Errorf("%s: flying over the sea: underwater %v, breath %d", mode, r.p.Underwater, r.p.Breath)
		}
		r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: 0, Y: -10, Z: 600, World: r.p.World, HasWorld: true, At: r.now,
			WalkFlags: 0x0104, HasWalkFlags: true}) // and under it, in the water
		step()
		if !r.p.Underwater || r.p.Breath >= BreathFull {
			t.Errorf("%s: the control: under the sea: underwater %v, breath %d", mode, r.p.Underwater, r.p.Breath)
		}
	}
}

// A logout in flight saves where it last stood, as a logout in a fall does, so
// no login begins in mid-air; a touch of the ground in flight is where it stood.
func TestALogoutInFlightSavesWhereItStood(t *testing.T) {
	r := newMCRig(t, MoveCheckLog, 50, 0, 0)
	report := func(x, y float32, flags uint32) {
		r.now = r.now.Add(100 * time.Millisecond)
		r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: x, Y: y, Z: 0, World: r.p.World, HasWorld: true, At: r.now, WalkFlags: flags, HasWalkFlags: true})
	}
	report(50, 0, 0x0001)
	r.w.setFlyingLocked(r.p, true, r.now)
	report(60, 40, 0)
	x, y, _, back := r.p.SavePlace()
	if !back || x != 50 || y != 0 {
		t.Errorf("logged out flying: (%.1f, %.1f), back %v", x, y, back)
	}
	report(70, 0, 0x0005) // on a floor, under fly
	report(80, 40, 0)
	x, y, _, back = r.p.SavePlace()
	if !back || x != 70 || y != 0 {
		t.Errorf("after a touch of the ground: (%.1f, %.1f), back %v", x, y, back)
	}
}

// A creature's reach goes by the map (chaseLocked), so one under a flyer would
// stand there and hit it 40 up. It stands there and hits nothing, its round's
// hits lost; come down into its reach, it fights. The control: the same place
// with no flight, as anyone standing there would be fought.
func TestACreatureCannotHitAFlyerOverIt(t *testing.T) {
	swings := func(flying bool, up float32) (swung bool, hp int) {
		c := newCaster(t)
		c.npc.Name, c.npc.Level, c.npc.HP, c.npc.MaxHP = "Desert Crab", 34, 2000, 2000
		c.npc.OffMod, c.npc.WeaponDamage = NPCOffMod(34), NPCWeaponDamage(34)
		c.npc.LeashRadius = DefaultLeash
		c.p.HP, c.p.MaxHP = 5000, 5000
		c.p.Y = c.npc.Y + up
		c.p.Flying = flying
		c.npc.CurrentEnemy = c.p.ServerID
		c.npc.Threats = map[EntityID]int64{c.p.ServerID: 10}
		for d := time.Duration(0); d < 8*time.Second; d += 100 * time.Millisecond {
			c.chaseTick(d)
			for _, m := range c.sent() {
				swung = swung || opOfMsg(m) == protocol.OpcodeCastingCombat
			}
		}
		return swung, c.p.HP
	}
	if swung, hp := swings(true, 40); swung || hp != 5000 {
		t.Errorf("flying 40 over it: swung %v, hit points %d", swung, hp)
	}
	if swung, hp := swings(true, 2); !swung || hp == 5000 {
		t.Errorf("flying 2 over it, in its reach: swung %v, hit points %d", swung, hp)
	}
	if swung, hp := swings(false, 40); !swung || hp == 5000 {
		t.Errorf("the control, no flight: swung %v, hit points %d", swung, hp)
	}
}

// Through a restart: its flight and its place go with what is kept, and come
// back on a character its row put where it last stood. The control: one not
// flying keeps its row's place.
func TestKeptPlayerFlies(t *testing.T) {
	for _, flying := range []bool{true, false} {
		old := newKeepRig(t, "a gnoll pup", false)
		now := time.Now()
		old.w.mu.Lock()
		old.p.Flying = flying
		old.p.X, old.p.Y, old.p.Z = 25400, 300.5, 15800
		old.w.mu.Unlock()
		kp, ok := old.w.KeepPlayer(old.sess, now)
		if !ok || kp.Flying != flying {
			t.Fatalf("kept %+v", kp)
		}
		r, sess := through(t, old, "a gnoll pup")
		if !r.w.RestoreKeptPlayer(sess, kp, now) {
			t.Fatal("not restored")
		}
		switch {
		case r.p.Flying != flying:
			t.Errorf("flying %v, want %v", r.p.Flying, flying)
		case flying && (r.p.X != 25400 || r.p.Y != 300.5 || r.p.Z != 15800):
			t.Errorf("back at (%.1f, %.1f, %.1f)", r.p.X, r.p.Y, r.p.Z)
		case flying && (r.p.StoodY != 54.1 || r.p.StoodX != 25375):
			t.Errorf("it last stood at (%.1f, %.1f), the row's", r.p.StoodX, r.p.StoodY)
		case !flying && r.p.Y != 54.1:
			t.Errorf("the control: at y %.1f", r.p.Y)
		}
		if found := r.w.SpatialIndex.QueryRadius(r.p.X, r.p.Z, 1); !containsEntity(found, r.p.ServerID) {
			t.Errorf("flying %v: not in the spatial index where it is", flying)
		}
	}
}

func containsEntity(ids []EntityID, id EntityID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
